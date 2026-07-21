// Command healthcheck is a zero-dependency liveness probe for the distroless
// runtime image, where no shell or curl is available for Docker HEALTHCHECK.
package main

import (
	"net/http"
	"os"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil || resp.StatusCode >= 300 {
		os.Exit(1)
	}
	os.Exit(0)
}
