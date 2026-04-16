package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
)

type infoResponse struct {
	Server       string `json:"server"`
	Scenario     string `json:"scenario"`
	Hostname     string `json:"hostname"`
	Port         string `json:"port"`
	Version      string `json:"version"`
	HealthStatus int    `json:"health_status"`
}

func main() {
	server := envOrDefault("SERVER_NAME", "test-server")
	scenario := envOrDefault("SCENARIO_NAME", "unknown")
	port := envOrDefault("PORT", "8080")
	version := envOrDefault("SERVER_VERSION", "v1")
	healthStatus := envIntOrDefault("HEALTH_STATUS", http.StatusOK)

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("hostname: %v", err)
	}

	info := infoResponse{
		Server:       server,
		Scenario:     scenario,
		Hostname:     hostname,
		Port:         port,
		Version:      version,
		HealthStatus: healthStatus,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, healthStatus, map[string]interface{}{
			"ok":            healthStatus >= 200 && healthStatus < 300,
			"server":        server,
			"scenario":      scenario,
			"health_status": healthStatus,
		})
	})
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, info)
	})

	log.Printf("test server %s listening on :%s", server, port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envIntOrDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
