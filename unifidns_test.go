package trafikunifidns

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCreateConfig(t *testing.T) {
	config := CreateConfig()
	if config == nil {
		t.Fatal("CreateConfig returned nil")
	}
	if config.UpdateInterval != "5m" {
		t.Errorf("Expected UpdateInterval to be '5m', got '%s'", config.UpdateInterval)
	}
	if config.TraefikAPIURL != "http://localhost:8080" {
		t.Errorf("Expected TraefikAPIURL to be 'http://localhost:8080', got '%s'", config.TraefikAPIURL)
	}
}

func TestNew(t *testing.T) {
	config := &Config{
		UDMProHost:     "192.168.1.1",
		UDMProUsername: "admin",
		UDMProPassword: "password",
		UpdateInterval: "1s",
		TraefikAPIURL:  "http://localhost:8080",
	}

	handler, err := New(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), config, "test")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if handler == nil {
		t.Fatal("New returned nil handler")
	}

	// Test with invalid interval
	config.UpdateInterval = "invalid"
	_, err = New(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), config, "test")
	if err == nil {
		t.Fatal("Expected error for invalid interval, got nil")
	}
}

func TestServeHTTP(t *testing.T) {
	config := &Config{
		UDMProHost:     "192.168.1.1",
		UDMProUsername: "admin",
		UDMProPassword: "password",
		UpdateInterval: "1s",
		TraefikAPIURL:  "http://localhost:8080",
	}

	handler, err := New(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}), config, "test")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	if rr.Body.String() != "test response" {
		t.Errorf("Expected body 'test response', got '%s'", rr.Body.String())
	}
}

func TestGetLocalIP(t *testing.T) {
	ip, err := getLocalIP()
	if err != nil {
		t.Fatalf("getLocalIP returned error: %v", err)
	}
	if ip == "" {
		t.Fatal("getLocalIP returned empty string")
	}
	// Check if it's a valid IP address
	if net.ParseIP(ip) == nil {
		t.Errorf("getLocalIP returned invalid IP address: %s", ip)
	}
}

func TestUpdateDNS(t *testing.T) {
	// Create a test server for Traefik API
	traefikServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/http/routers" {
			t.Errorf("Expected path '/api/http/routers', got '%s'", r.URL.Path)
		}

		// Return test routers
		w.Header().Set("Content-Type", "application/json")
		routers := []TraefikRouter{
			{Rule: "Host(`example.com`)"},
			{Rule: "Host('test.com')"},
			{Rule: "PathPrefix(`/api`)"}, // No host rule
		}
		json.NewEncoder(w).Encode(routers)
	}))
	defer traefikServer.Close()

	// Create a test server for UniFi API
	unifiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		// Check authorization header for DNS update
		if r.URL.Path == "/api/s/default/rest/dnsrecord" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token" {
				t.Errorf("Expected Authorization 'Bearer test-token', got '%s'", auth)
			}

			// Check request body
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Failed to decode request body: %v", err)
			}
			if payload["type"] != "A" {
				t.Errorf("Expected type 'A', got '%v'", payload["type"])
			}
			if payload["ttl"] != float64(300) {
				t.Errorf("Expected ttl 300, got '%v'", payload["ttl"])
			}

			// Return success
			w.WriteHeader(http.StatusOK)
		} else if r.URL.Path == "/api/auth/login" {
			// Return a token for login
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"token": "test-token"})
		}
	}))
	defer unifiServer.Close()

	// Create a plugin instance with test servers
	config := &Config{
		UDMProHost:     unifiServer.Listener.Addr().String(),
		UDMProUsername: "admin",
		UDMProPassword: "password",
		UpdateInterval: "1s",
		TraefikAPIURL:  traefikServer.URL,
	}

	plugin := &UniFiDNS{
		next:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		name:           "test",
		config:         config,
		unifiClient:    NewUniFiClient(config.UDMProHost, config.UDMProUsername, config.UDMProPassword),
		traefikClient:  NewTraefikClient(config.TraefikAPIURL),
		updateInterval: time.Second,
	}

	// Test updateDNS
	err := plugin.updateDNS()
	if err != nil {
		t.Fatalf("updateDNS returned error: %v", err)
	}
}
