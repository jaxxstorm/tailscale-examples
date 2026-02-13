package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/lipgloss"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"tailscale.com/tsnet"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	okStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	warnStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
)

type cli struct {
	LogLevel string    `name:"log-level" default:"info" enum:"debug,info,warn,error" help:"Log level for zap output."`
	Server   serverCmd `cmd:"" help:"Run a tsnet HTTP/2 + gRPC server."`
	Client   clientCmd `cmd:"" help:"Send a message over HTTP/2 or gRPC via tsnet."`
}

type serverCmd struct {
	Hostname   string `name:"hostname" default:"tsnet-grpc-server" help:"tsnet hostname for this server node."`
	StateDir   string `name:"state-dir" default:".tsnet-server" help:"Directory to store tsnet state."`
	AuthKey    string `name:"auth-key" help:"Optional Tailscale auth key."`
	ListenAddr string `name:"listen-addr" default:":8443" help:"Tailnet TCP address to listen on."`
	HTTPS      bool   `name:"https" help:"Enable HTTPS certificates via tsnet automatically. Leave off for insecure h2c."`
}

type clientCmd struct {
	Hostname string        `name:"hostname" default:"tsnet-grpc-client" help:"tsnet hostname for this client node."`
	StateDir string        `name:"state-dir" default:".tsnet-client" help:"Directory to store tsnet state."`
	AuthKey  string        `name:"auth-key" help:"Optional Tailscale auth key."`
	Address  string        `name:"address" required:"" help:"Target server address (for example: server.tailnet.ts.net:8443)."`
	Message  string        `name:"message" default:"hello from tsnet" help:"Message payload to send."`
	Mode     string        `name:"mode" default:"http" enum:"http,grpc" help:"Client mode to compare protocols."`
	HTTPS    bool          `name:"https" help:"Use TLS. Leave off to dial insecure (h2c for HTTP mode)."`
	Timeout  time.Duration `name:"timeout" default:"45s" help:"Request timeout."`
}

type runContext struct {
	Logger *zap.Logger
}

func main() {
	cfg := cli{}
	parser, err := kong.New(
		&cfg,
		kong.Name("tsnet-grpc"),
		kong.Description("HTTP/2 and gRPC over tsnet in a single self-contained binary."),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build CLI parser: %v\n", err)
		os.Exit(1)
	}

	kctx, err := parser.Parse(os.Args[1:])
	if err != nil {
		parser.FatalIfErrorf(err)
	}

	logger, err := newLogger(cfg.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = logger.Sync()
	}()

	runCtx := &runContext{Logger: logger}
	kctx.FatalIfErrorf(kctx.Run(runCtx))
}

func (c *serverCmd) Run(run *runContext) error {
	serveCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ts := &tsnet.Server{
		Hostname: c.Hostname,
		Dir:      c.StateDir,
		AuthKey:  c.AuthKey,
	}
	defer ts.Close()

	ln, err := ts.Listen("tcp", c.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}
	defer ln.Close()

	certDomains := []string(nil)
	if c.HTTPS {
		certDomains, err = ensureHTTPSReady(serveCtx, ts)
		if err != nil {
			return err
		}

		lc, err := ts.LocalClient()
		if err != nil {
			return fmt.Errorf("failed getting tailscale local client: %w", err)
		}
		if len(certDomains) > 0 {
			certCtx, cancel := context.WithTimeout(serveCtx, 45*time.Second)
			_, _, err = lc.CertPair(certCtx, certDomains[0])
			cancel()
			if err != nil {
				return fmt.Errorf("failed prefetching tsnet certificate for %q: %w", certDomains[0], err)
			}
		}

		ln = tls.NewListener(ln, &tls.Config{
			MinVersion:     tls.VersionTLS12,
			NextProtos:     []string{"h2", "http/1.1"},
			GetCertificate: lc.GetCertificate,
		})
	}

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(serverUnaryLogger(run.Logger)))
	healthpb.RegisterHealthServer(grpcServer, &loggingHealthServer{logger: run.Logger})

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		message := r.URL.Query().Get("message")
		if message == "" {
			message = "(empty)"
		}
		run.Logger.Info(
			"received http request",
			zap.String("message", message),
			zap.String("proto", r.Proto),
			zap.String("remote_addr", r.RemoteAddr),
		)
		w.Header().Set("content-type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "echo: "+message+"\n")
	})

	mixed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
			return
		}
		httpMux.ServeHTTP(w, r)
	})

	readHeaderTimeout := 10 * time.Second
	if c.HTTPS {
		// Initial certificate retrieval can be slower than a normal request.
		// Keep a larger handshake/header budget for HTTPS.
		readHeaderTimeout = 60 * time.Second
	}
	srv := &http.Server{
		ReadHeaderTimeout: readHeaderTimeout,
		Handler:           mixed,
	}

	go func() {
		<-serveCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	fmt.Println(titleStyle.Render("tsnet HTTP/2 + gRPC server"))
	fmt.Println(okStyle.Render("listening on " + ln.Addr().String()))

	if c.HTTPS {
		domain := "(unknown)"
		if len(certDomains) > 0 {
			domain = certDomains[0]
		}
		run.Logger.Info("serving HTTPS over tsnet", zap.String("primary_domain", domain), zap.String("listen_addr", c.ListenAddr))
		fmt.Println(okStyle.Render("HTTPS enabled; cert domain " + domain))
		err = srv.Serve(ln)
	} else {
		run.Logger.Warn("serving insecure h2c over tsnet; enable --https for TLS")
		fmt.Println(warnStyle.Render("HTTPS disabled; serving insecure h2c"))
		srv.Handler = h2c.NewHandler(mixed, &http2.Server{})
		err = srv.Serve(ln)
	}

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server exited with error: %w", err)
	}
	return nil
}

func (c *clientCmd) Run(run *runContext) error {
	ts := &tsnet.Server{
		Hostname: c.Hostname,
		Dir:      c.StateDir,
		AuthKey:  c.AuthKey,
	}
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()

	upCtx, upCancel := context.WithTimeout(context.Background(), effectiveUpTimeout(c.Timeout))
	defer upCancel()
	if _, err := ts.Up(upCtx); err != nil {
		return fmt.Errorf("tsnet startup/login failed: %w", err)
	}

	fmt.Println(titleStyle.Render("tsnet client"))
	fmt.Println(okStyle.Render(fmt.Sprintf("dialing %s via %s (tls=%t)", c.Address, c.Mode, c.HTTPS)))

	switch c.Mode {
	case "http":
		return c.runHTTP(ctx, run.Logger, ts)
	case "grpc":
		return c.runGRPC(ctx, run.Logger, ts)
	default:
		return fmt.Errorf("unsupported mode %q", c.Mode)
	}
}

func (c *clientCmd) runHTTP(ctx context.Context, logger *zap.Logger, ts *tsnet.Server) error {
	dialAddress := c.Address
	serverName := hostFromAddress(c.Address)
	if c.HTTPS {
		var err error
		dialAddress, serverName, err = resolveTLSAddress(ctx, ts, c.Address)
		if err != nil {
			return err
		}
	}

	transport := &http2.Transport{}
	if c.HTTPS {
		transport.DialTLSContext = func(ctx context.Context, network, addr string, tlsCfg *tls.Config) (net.Conn, error) {
			conn, err := ts.Dial(ctx, network, addr)
			if err != nil {
				return nil, err
			}

			cfg := &tls.Config{MinVersion: tls.VersionTLS12}
			if tlsCfg != nil {
				cfg = tlsCfg.Clone()
				if cfg.MinVersion == 0 {
					cfg.MinVersion = tls.VersionTLS12
				}
			}
			cfg.ServerName = serverName

			tlsConn := tls.Client(conn, cfg)
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				_ = conn.Close()
				return nil, err
			}
			return tlsConn, nil
		}
	} else {
		transport.AllowHTTP = true
		transport.DialTLSContext = func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return ts.Dial(ctx, network, addr)
		}
	}

	scheme := "http"
	if c.HTTPS {
		scheme = "https"
	}

	endpoint := &url.URL{
		Scheme: scheme,
		Host:   dialAddress,
		Path:   "/echo",
	}
	query := endpoint.Query()
	query.Set("message", c.Message)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}

	resp, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response failed: %w", err)
	}

	logger.Info("http response", zap.Int("status_code", resp.StatusCode), zap.String("proto", resp.Proto), zap.ByteString("body", body))
	fmt.Println(okStyle.Render(fmt.Sprintf("http response (%s): %s", resp.Proto, strings.TrimSpace(string(body)))))
	return nil
}

func (c *clientCmd) runGRPC(ctx context.Context, logger *zap.Logger, ts *tsnet.Server) error {
	dialAddress := c.Address
	serverName := hostFromAddress(c.Address)
	if c.HTTPS {
		var err error
		dialAddress, serverName, err = resolveTLSAddress(ctx, ts, c.Address)
		if err != nil {
			return err
		}
	}

	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return ts.Dial(ctx, "tcp", addr)
	}

	dialOpts := []grpc.DialOption{grpc.WithContextDialer(dialer)}
	if c.HTTPS {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: serverName,
		})))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.DialContext(ctx, dialAddress, dialOpts...)
	if err != nil {
		return fmt.Errorf("grpc dial failed: %w", err)
	}
	defer conn.Close()

	healthClient := healthpb.NewHealthClient(conn)
	resp, err := healthClient.Check(ctx, &healthpb.HealthCheckRequest{Service: c.Message})
	if err != nil {
		return fmt.Errorf("grpc health check failed: %w", err)
	}

	logger.Info("grpc response", zap.String("status", resp.Status.String()))
	fmt.Println(okStyle.Render("grpc response: " + resp.Status.String()))
	return nil
}

func newLogger(level string) (*zap.Logger, error) {
	lvl := zapcore.InfoLevel
	if err := lvl.Set(level); err != nil {
		return nil, fmt.Errorf("invalid log-level %q: %w", level, err)
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(lvl)
	cfg.Encoding = "console"
	return cfg.Build()
}

func ensureHTTPSReady(ctx context.Context, ts *tsnet.Server) ([]string, error) {
	upCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	st, err := ts.Up(upCtx)
	if err != nil {
		return nil, fmt.Errorf("failed bringing tsnet up for HTTPS: %w", err)
	}
	if !st.CurrentTailnet.MagicDNSEnabled {
		return nil, errors.New("tailscale MagicDNS must be enabled to use HTTPS certificates")
	}
	if len(st.CertDomains) == 0 {
		return nil, errors.New("tailscale HTTPS certificates are not enabled for this tailnet")
	}
	return append([]string(nil), st.CertDomains...), nil
}

func effectiveUpTimeout(requestTimeout time.Duration) time.Duration {
	if requestTimeout > 30*time.Second {
		return requestTimeout
	}
	return 30 * time.Second
}

func resolveTLSAddress(ctx context.Context, ts *tsnet.Server, address string) (dialAddress, serverName string, err error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", "", fmt.Errorf("address %q must include a port", address)
	}
	if strings.Contains(host, ".") {
		return address, host, nil
	}

	lc, err := ts.LocalClient()
	if err != nil {
		return "", "", fmt.Errorf("failed getting tailscale local client: %w", err)
	}
	if fqdn, ok := lc.ExpandSNIName(ctx, host); ok {
		return net.JoinHostPort(fqdn, port), fqdn, nil
	}

	// Expand bare hostnames (for example "tsnet-grpc-server") to the local
	// tailnet's MagicDNS suffix so TLS SNI/cert validation targets a cert domain.
	status, err := lc.StatusWithoutPeers(ctx)
	if err == nil {
		suffix := status.MagicDNSSuffix
		if status.CurrentTailnet != nil && status.CurrentTailnet.MagicDNSSuffix != "" {
			suffix = status.CurrentTailnet.MagicDNSSuffix
		}
		suffix = strings.TrimSuffix(suffix, ".")
		if suffix != "" {
			fqdn := host + "." + suffix
			return net.JoinHostPort(fqdn, port), fqdn, nil
		}
	}
	return address, host, nil
}

func hostFromAddress(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}
	return addr
}

func serverUnaryLogger(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		logger.Info("grpc request", zap.String("method", info.FullMethod), zap.Any("request", req))
		resp, err := handler(ctx, req)
		if err != nil {
			logger.Error("grpc request failed", zap.String("method", info.FullMethod), zap.Error(err))
			return nil, err
		}
		return resp, nil
	}
}

type loggingHealthServer struct {
	logger *zap.Logger
	healthpb.UnimplementedHealthServer
}

func (s *loggingHealthServer) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	s.logger.Info("received grpc message", zap.String("message", req.GetService()))
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (s *loggingHealthServer) List(context.Context, *healthpb.HealthListRequest) (*healthpb.HealthListResponse, error) {
	return &healthpb.HealthListResponse{
		Statuses: map[string]*healthpb.HealthCheckResponse{
			"": {Status: healthpb.HealthCheckResponse_SERVING},
		},
	}, nil
}

func (s *loggingHealthServer) Watch(_ *healthpb.HealthCheckRequest, _ healthpb.Health_WatchServer) error {
	return status.Error(codes.Unimplemented, "watch is not implemented")
}
