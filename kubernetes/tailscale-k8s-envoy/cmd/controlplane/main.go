package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kong"

	// Envoy go-control-plane
	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"

	cs "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"

	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	srv "github.com/envoyproxy/go-control-plane/pkg/server/v3"

	router "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"

	// gRPC
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"

	// Tailscale
	"tailscale.com/tsnet"
)

// CLI defines command-line flags via Kong
type CLI struct {
	Hostname     string `help:"Tailscale node hostname" default:"tsnet-weighted-eds" env:"TS_ENVOY_HOSTNAME"`
	Ephemeral    bool   `help:"Use ephemeral Tailscale node" default:"true" env:"TS_ENVOY_EPHEMERAL"`
	AuthKey      string `help:"Tailscale auth key (tskey-...)" required:"true" env:"TS_ENVOY_AUTHKEY"`
	Port         int    `help:"Port to listen on Tailscale interface" default:"18000" env:"TS_ENVOY_PORT"`
	PollInterval int    `help:"Seconds between Tailscale poll for EDS updates" default:"30" env:"TS_ENVOY_POLL_INTERVAL"`
	Tailnet      string `help:"Tailscale tailnet name (e.g. example.com)" required:"true" env:"TS_ENVOY_TAILNET"`
	APIToken     string `help:"Tailscale Admin API token" required:"true" env:"TS_ENVOY_API_TOKEN"`

	// e.g. --discovery-tags=weight-1,weight-2
	DiscoveryTags []string `help:"Tailscale tags in the form weight-X" env:"TS_ENVOY_DISCOVERY_TAGS"`

	ClusterName string `help:"Envoy cluster name" default:"envoy-cluster" env:"TS_ENVOY_CLUSTER_NAME"`
	NodeId      string `help:"Envoy node ID" default:"envoy-node" env:"TS_ENVOY_NODE_ID"`
}

type TSDevice struct {
	Addresses []string `json:"addresses"`
	Tags      []string `json:"tags"`
}

func main() {
	var cli CLI
	kong.Parse(&cli)

	// Start ephemeral Tailscale node with an auth key
	tsServer := &tsnet.Server{
		Hostname:  cli.Hostname,
		Ephemeral: cli.Ephemeral,
		AuthKey:   cli.AuthKey,
	}
	if err := tsServer.Start(); err != nil {
		log.Fatalf("tsnet.Start failed: %v", err)
	}
	defer tsServer.Close()

	// Listen on Tailscale interface:port
	ln, err := tsServer.Listen("tcp", fmt.Sprintf(":%d", cli.Port))
	if err != nil {
		log.Fatalf("Failed to listen on Tailscale: %v", err)
	}
	defer ln.Close()

	// Build snapshot cache
	snapshotCache := cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil)
	ctx := context.Background()

	// Build initial snapshot
	initSnap, err := buildInitialSnapshot(cli.ClusterName)
	if err != nil {
		log.Fatalf("buildInitialSnapshot error: %v", err)
	}
	if err := snapshotCache.SetSnapshot(ctx, cli.NodeId, initSnap); err != nil {
		log.Fatalf("SetSnapshot failed: %v", err)
	}

	// Create xDS server & register
	xdsServer := srv.NewServer(ctx, snapshotCache, nil)
	grpcServer := grpc.NewServer()

	discoverygrpc.RegisterAggregatedDiscoveryServiceServer(grpcServer, xdsServer)
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, xdsServer)
	cs.RegisterClusterDiscoveryServiceServer(grpcServer, xdsServer)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, xdsServer)
	routeservice.RegisterRouteDiscoveryServiceServer(grpcServer, xdsServer)

	// Serve xDS in background
	go func() {
		log.Printf("Weighted EDS control plane listening on %v ephemeral=%v\n", ln.Addr(), cli.Ephemeral)
		if err := grpcServer.Serve(ln); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	// Poll Tailscale Admin API every PollInterval for devices with "weight-X" tags
	go startAdminAPIEDSLoop(ctx, snapshotCache, cli.NodeId, cli)

	// Block forever
	select {}
}

// buildInitialSnapshot configures Weighted LB + active HTTP health checks
func buildInitialSnapshot(clusterName string) (*cachev3.Snapshot, error) {
	edsCluster := &cluster.Cluster{
		Name: clusterName,
		ClusterDiscoveryType: &cluster.Cluster_Type{
			Type: cluster.Cluster_EDS,
		},
		ConnectTimeout: durationpb.New(1 * time.Second),

		// EDS from ADS
		EdsClusterConfig: &cluster.Cluster_EdsClusterConfig{
			ServiceName: "eds_backend",
			EdsConfig: &core.ConfigSource{
				ResourceApiVersion: resource.DefaultAPIVersion,
				ConfigSourceSpecifier: &core.ConfigSource_Ads{
					Ads: &core.AggregatedConfigSource{},
				},
			},
		},
		CommonLbConfig: &cluster.Cluster_CommonLbConfig{
			// Enable locality weighting
			LocalityConfigSpecifier: &cluster.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
				LocalityWeightedLbConfig: &cluster.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
			},
		},
		// Active HTTP health checks: requests "/" every 5s
		HealthChecks: []*core.HealthCheck{
			{
				Timeout:            durationpb.New(1 * time.Second),
				Interval:           durationpb.New(5 * time.Second),
				UnhealthyThreshold: wrapperspb.UInt32(2),
				HealthyThreshold:   wrapperspb.UInt32(1),
				HealthChecker: &core.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &core.HealthCheck_HttpHealthCheck{
						Host: "127.0.0.1",
						Path: "/",
					},
				},
			},
		},
	}

	// Start with empty EDS
	edsResource := &endpoint.ClusterLoadAssignment{
		ClusterName: "eds_backend",
	}

	// Build listener & route
	listenerRes := buildHTTPListener("main-listener", 10000)
	routeRes := buildRouteConfig("local-route", clusterName)

	// Snapshot version=1
	return cachev3.NewSnapshot(
		"1",
		map[resource.Type][]types.Resource{
			resource.ClusterType:  {edsCluster},
			resource.EndpointType: {edsResource},
			resource.ListenerType: {listenerRes},
			resource.RouteType:    {routeRes},
		},
	)
}

// startAdminAPIEDSLoop -> fetch "weight-X" tags from Tailscale & build localities
func startAdminAPIEDSLoop(
	ctx context.Context,
	snapshotCache cachev3.SnapshotCache,
	nodeID string,
	cli CLI,
) {
	ticker := time.NewTicker(time.Duration(cli.PollInterval) * time.Second)
	defer ticker.Stop()

	version := 2
	edsName := "eds_backend"

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			devices, err := fetchDevices(cli.Tailnet, cli.APIToken)
			if err != nil {
				log.Printf("Error fetching devices: %v", err)
				continue
			}

			// For each discovered tag, parse e.g. "weight-2" => weight=2, build a separate Locality
			var localities []*endpoint.LocalityLbEndpoints

			for _, tag := range cli.DiscoveryTags {
				w := parseTagWeight(tag) // parse "weight-2" => 2

				// Find all devices that have "tagWanted"
				var addrs []string
				for _, d := range devices {
					for _, t := range d.Tags {
						if t == tag {
							addrs = append(addrs, d.Addresses...)
							break
						}
					}
				}
				if len(addrs) == 0 {
					continue
				}
				log.Printf("Found %d addresses for %q => weight=%d\n", len(addrs), tag, w)

				localities = append(localities, &endpoint.LocalityLbEndpoints{
					Locality: &core.Locality{
						Region: tag, // or "weight-x"
					},
					Priority: 0,
					// Weighted LB
					LoadBalancingWeight: wrapperspb.UInt32(w),
					LbEndpoints:         buildLbEndpoints(addrs),
				})
			}

			// Rebuild EDS resource
			edsResource := &endpoint.ClusterLoadAssignment{
				ClusterName: edsName,
				Endpoints:   localities,
			}

			// Update snapshot
			curSnap, _ := snapshotCache.GetSnapshot(nodeID)
			if curSnap == nil {
				log.Printf("EDS loop: no snapshot found for %q", nodeID)
				continue
			}
			edsMap := curSnap.GetResourcesAndTTL(resource.EndpointType)
			if edsMap == nil {
				edsMap = make(map[string]types.ResourceWithTTL)
			}
			edsMap[edsName] = types.ResourceWithTTL{
				Resource: edsResource,
				TTL:      nil,
			}

			clusterMap := curSnap.GetResourcesAndTTL(resource.ClusterType)
			listenerMap := curSnap.GetResourcesAndTTL(resource.ListenerType)
			routeMap := curSnap.GetResourcesAndTTL(resource.RouteType)

			newSnap, err := cachev3.NewSnapshot(
				fmt.Sprintf("%d", version),
				map[resource.Type][]types.Resource{
					resource.ClusterType:  resourceMapWithTtlToSlice(clusterMap),
					resource.RouteType:    resourceMapWithTtlToSlice(routeMap),
					resource.ListenerType: resourceMapWithTtlToSlice(listenerMap),
					resource.EndpointType: resourceMapWithTtlToSlice(edsMap),
				},
			)
			if err != nil {
				log.Printf("EDS loop: building snapshot error: %v", err)
				continue
			}
			version++

			if err := snapshotCache.SetSnapshot(ctx, nodeID, newSnap); err != nil {
				log.Printf("EDS loop: SetSnapshot error: %v", err)
				continue
			}
			log.Printf("EDS updated: %d localities, version=%d\n", len(localities), version)
		}
	}
}

// parseTagWeight => parse "weight-2" => 2, fallback=1
func parseTagWeight(tag string) uint32 {
	parts := strings.Split(tag, "-")
	if len(parts) < 2 {
		return 1
	}
	w, err := strconv.Atoi(parts[1])
	if err != nil || w < 1 {
		return 1
	}
	return uint32(w)
}

// fetchDevices -> Tailscale Admin API => GET /api/v2/tailnet/<tailnet>/devices
func fetchDevices(tailnet, bearerToken string) ([]TSDevice, error) {
	url := fmt.Sprintf("https://api.tailscale.com/api/v2/tailnet/%s/devices", tailnet)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	req.Header.Set("Accept", "application/json")

	httpc := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetchDevices: non-200 status=%v body=%s", resp.Status, string(body))
	}

	// Optionally debug print the entire JSON:
	/*
		bodyBytes, _ := io.ReadAll(resp.Body)
		fmt.Printf("DEBUG RAW: %s\n", string(bodyBytes))
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	*/

	var dr struct {
		Devices []TSDevice `json:"devices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		return nil, err
	}
	return dr.Devices, nil
}

func buildLbEndpoints(addrs []string) []*endpoint.LbEndpoint {
	var out []*endpoint.LbEndpoint
	for _, ip := range addrs {
		out = append(out, &endpoint.LbEndpoint{
			HostIdentifier: &endpoint.LbEndpoint_Endpoint{
				Endpoint: &endpoint.Endpoint{
					Address: &core.Address{
						Address: &core.Address_SocketAddress{
							SocketAddress: &core.SocketAddress{
								Address: ip,
								PortSpecifier: &core.SocketAddress_PortValue{
									PortValue: 8080,
								},
							},
						},
					},
				},
			},
		})
	}
	return out
}

func resourceMapWithTtlToSlice(in map[string]types.ResourceWithTTL) []types.Resource {
	out := make([]types.Resource, 0, len(in))
	for _, rwt := range in {
		out = append(out, rwt.Resource)
	}
	return out
}

func buildHTTPListener(name string, port uint32) *listener.Listener {
	// typed router filter
	routerFilter := &router.Router{}
	routerFilterTyped, err := anypb.New(routerFilter)
	if err != nil {
		panic(err)
	}

	hcmConfig := &hcm.HttpConnectionManager{
		StatPrefix: "ingress_hcm",
		RouteSpecifier: &hcm.HttpConnectionManager_Rds{
			Rds: &hcm.Rds{
				RouteConfigName: "local-route",
				ConfigSource: &core.ConfigSource{
					ResourceApiVersion: resource.DefaultAPIVersion,
					ConfigSourceSpecifier: &core.ConfigSource_Ads{
						Ads: &core.AggregatedConfigSource{},
					},
				},
			},
		},
		HttpFilters: []*hcm.HttpFilter{
			{
				Name: "envoy.filters.http.router",
				ConfigType: &hcm.HttpFilter_TypedConfig{
					TypedConfig: routerFilterTyped,
				},
			},
		},
	}
	anyHCM, err := anypb.New(hcmConfig)
	if err != nil {
		panic(err)
	}

	return &listener.Listener{
		Name: name,
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Address: "0.0.0.0",
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: port,
					},
				},
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: "envoy.filters.network.http_connection_manager",
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: anyHCM,
				},
			}},
		}},
	}
}

func buildRouteConfig(routeName, clusterName string) *route.RouteConfiguration {
	return &route.RouteConfiguration{
		Name: routeName,
		VirtualHosts: []*route.VirtualHost{
			{
				Name:    "weighted_service",
				Domains: []string{"*"},
				Routes: []*route.Route{
					{
						Match: &route.RouteMatch{PathSpecifier: &route.RouteMatch_Prefix{Prefix: "/"}},
						Action: &route.Route_Route{
							Route: &route.RouteAction{
								// All traffic → clusterName
								ClusterSpecifier: &route.RouteAction_Cluster{
									Cluster: clusterName,
								},
							},
						},
					},
				},
			},
		},
	}
}
