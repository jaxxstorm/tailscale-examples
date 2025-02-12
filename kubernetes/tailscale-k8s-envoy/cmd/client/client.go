// client.go
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <url> <n_requests>\n", os.Args[0])
		os.Exit(1)
	}
	url := os.Args[1]
	nReq, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid n_requests: %v\n", err)
		os.Exit(1)
	}

	counts := make(map[string]int)
	var fails int

	for i := 0; i < nReq; i++ {
		resp, err := http.Get(url)
		if err != nil {
			fails++
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			fails++
			continue
		}
		key := strings.TrimSpace(string(body))
		counts[key]++
	}

	for k, c := range counts {
		percent := float64(c) / float64(nReq) * 100.0
		fmt.Printf("%s: %.1f%% (%d)\n", k, percent, c)
	}
	fmt.Printf("Failed: %d\n", fails)
}
