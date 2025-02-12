package main

import (
	"fmt"
	"github.com/alecthomas/kong"
	"log"
	"net/http"
	"sync"
)

var (
	isHealthy = true
	mu        sync.RWMutex
)

type CLI struct {
	Host     string `help:"Hostname" required:"true" env:"SERVER_HOST"`
	Port     int    `help:"Port to listen on" default:"8080" env:"SERVER_PORT"`
	Locality string `help:"Locality" env:"SERVER_LOCALITY"`
}

func main() {

	var cli CLI
	kong.Parse(&cli)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()
		if isHealthy {
			fmt.Fprintf(w, "Hello from %s! in %s\n", cli.Host, cli.Locality)
		} else {
			http.Error(w, "Unhealthy", http.StatusServiceUnavailable)
		}
	})

	// Toggle to healthy
	http.HandleFunc("/healthy", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		isHealthy = true
		mu.Unlock()
		fmt.Fprintf(w, "[%s] Set to healthy\n", cli.Host)
	})

	// Toggle to unhealthy
	http.HandleFunc("/unhealthy", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		isHealthy = false
		mu.Unlock()
		fmt.Fprintf(w, "[%s] Set to unhealthy\n", cli.Host)
	})

	log.Printf("Server starting on :%d, host = %s", cli.Port, cli.Host)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", cli.Port), nil))
}
