package trafikunifidns

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
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
	if len(config.Devices) != 0 {
		t.Errorf("Expected empty devices array, got %d devices", len(config.Devices))
	}
}

func TestNew(t *testing.T) {
	config := &Config{
		Devices: []UnifiDeviceConfig{
			{
				Host:     "192.168.1.1",
				Username: "admin",
				Password: "password",
				Pattern:  ".*\\.example\\.com",
			},
		},
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

	// Test with invalid regex pattern
	config.UpdateInterval = "1s"
	config.Devices[0].Pattern = "[" // Invalid regex
	_, err = New(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), config, "test")
	if err == nil {
		t.Fatal("Expected error for invalid pattern, got nil")
	}

	// Test with empty pattern
	config.Devices[0].Pattern = ""
	_, err = New(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), config, "test")
	if err == nil {
		t.Fatal("Expected error for empty pattern, got nil")
	}
}

func TestServeHTTP(t *testing.T) {
	config := &Config{
		Devices: []UnifiDeviceConfig{
			{
				Host:     "192.168.1.1",
				Username: "admin",
				Password: "password",
				Pattern:  ".*\\.example\\.com",
			},
		},
		UpdateInterval: "1s",
		TraefikAPIURL:  "http://localhost:8080",
	}

	handler, err := New(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("test response")); err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
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
			{Rule: "Host(`test.example.com`)"},
			{Rule: "Host('other.domain.com')"},
			{Rule: "PathPrefix(`/api`)"}, // No host rule
		}
		if err := json.NewEncoder(w).Encode(routers); err != nil {
			t.Fatalf("Failed to encode routers: %v", err)
		}
	}))
	defer traefikServer.Close()

	// Create a test server for UniFi API
	unifiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		switch r.URL.Path {
		case "/api/s/default/rest/dnsrecord":
			// Check authorization header for DNS update
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
		case "/api/auth/login":
			// Return a token for login
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]string{"token": "test-token"}); err != nil {
				t.Fatalf("Failed to encode token response: %v", err)
			}
		}
	}))
	defer unifiServer.Close()

	unifiServer2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		switch r.URL.Path {
		case "/api/s/default/rest/dnsrecord":
			// Check authorization header for DNS update
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token" {
				t.Errorf("Expected Authorization 'Bearer test-token', got '%s'", auth)
			}

			// Return success
			w.WriteHeader(http.StatusOK)
		case "/api/auth/login":
			// Return a token for login
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]string{"token": "test-token"}); err != nil {
				t.Fatalf("Failed to encode token response: %v", err)
			}
		}
	}))
	defer unifiServer2.Close()

	// Create a plugin instance with test servers
	config := &Config{
		Devices: []UnifiDeviceConfig{
			{
				Host:     unifiServer.Listener.Addr().String(),
				Username: "admin",
				Password: "password",
				Pattern:  ".*\\.example\\.com",
			},
			{
				Host:     unifiServer2.Listener.Addr().String(),
				Username: "admin",
				Password: "password",
				Pattern:  ".*\\.domain\\.com",
			},
		},
		UpdateInterval: "1s",
		TraefikAPIURL:  traefikServer.URL,
	}

	// Create unifi clients
	unifiClients := make(map[string]*UniFiClient)
	devicePatterns := make(map[string]*regexp.Regexp)

	for i, device := range config.Devices {
		client := NewUniFiClient(device.Host, device.Username, device.Password)
		clientID := fmt.Sprintf("device-%d", i)
		unifiClients[clientID] = client
		pattern, _ := regexp.Compile(device.Pattern)
		devicePatterns[clientID] = pattern
	}

	plugin := &UniFiDNS{
		next:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		name:           "test",
		config:         config,
		unifiClients:   unifiClients,
		devicePatterns: devicePatterns,
		traefikClient:  NewTraefikClient(config.TraefikAPIURL),
		updateInterval: time.Second,
	}

	// Test updateDNS
	err := plugin.updateDNS()
	if err != nil {
		t.Fatalf("updateDNS returned error: %v", err)
	}
}

func TestFindMatchingClient(t *testing.T) {
	config := &Config{
		Devices: []UnifiDeviceConfig{
			{
				Host:     "192.168.1.1",
				Username: "admin1",
				Password: "pass1",
				Pattern:  ".*\\.example\\.com",
			},
			{
				Host:     "192.168.1.2",
				Username: "admin2",
				Password: "pass2",
				Pattern:  ".*\\.domain\\.com",
			},
		},
	}

	// Create unifi clients
	unifiClients := make(map[string]*UniFiClient)
	devicePatterns := make(map[string]*regexp.Regexp)

	for i, device := range config.Devices {
		client := NewUniFiClient(device.Host, device.Username, device.Password)
		clientID := fmt.Sprintf("device-%d", i)
		unifiClients[clientID] = client
		pattern, _ := regexp.Compile(device.Pattern)
		devicePatterns[clientID] = pattern
	}

	plugin := &UniFiDNS{
		config:         config,
		unifiClients:   unifiClients,
		devicePatterns: devicePatterns,
	}

	// Test matching
	tests := []struct {
		hostname   string
		shouldFind bool
		clientHost string
	}{
		{
			hostname:   "test.example.com",
			shouldFind: true,
			clientHost: "192.168.1.1",
		},
		{
			hostname:   "sub.domain.com",
			shouldFind: true,
			clientHost: "192.168.1.2",
		},
		{
			hostname:   "another.site.org",
			shouldFind: false,
			clientHost: "",
		},
	}

	for _, tt := range tests {
		client, found := plugin.findMatchingClient(tt.hostname)
		if found != tt.shouldFind {
			t.Errorf("findMatchingClient(%q): got found=%v, want %v", tt.hostname, found, tt.shouldFind)
		}
		if found && client.baseURL != "https://"+tt.clientHost {
			t.Errorf("findMatchingClient(%q): got client with host %q, want %q",
				tt.hostname, client.baseURL, "https://"+tt.clientHost)
		}
	}
}
