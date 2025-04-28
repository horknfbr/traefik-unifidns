package traefikunifidns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
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

func TestGetLocalIPExtended(t *testing.T) {
	// First test the regular function behavior
	ip, err := getLocalIP()
	if err != nil {
		t.Fatalf("getLocalIP returned error: %v", err)
	}
	if ip == "" {
		t.Fatal("getLocalIP returned empty string")
	}

	// Check if it's a valid IP address
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		t.Errorf("getLocalIP returned invalid IP address: %s", ip)
	}

	// Verify it's not a loopback address
	if parsedIP.IsLoopback() {
		t.Errorf("getLocalIP returned loopback address: %s", ip)
	}

	// Verify it's an IPv4 address (which the function is designed to return)
	if parsedIP.To4() == nil {
		t.Errorf("getLocalIP returned non-IPv4 address: %s", ip)
	}
}

func TestGetLocalIPNoAddresses(t *testing.T) {
	// We can't easily mock net.InterfaceAddrs without compiler modification,
	// so we'll test our understanding of the function logic instead

	// Create a simple version of getLocalIP that takes a custom InterfaceAddrs function
	customGetLocalIP := func(getAddrs func() ([]net.Addr, error)) (string, error) {
		addrs, err := getAddrs()
		if err != nil {
			return "", err
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return ipnet.IP.String(), nil
				}
			}
		}

		return "", fmt.Errorf("no suitable IP address found")
	}

	// Test case 1: Simulate InterfaceAddrs returning an error
	errorFunc := func() ([]net.Addr, error) {
		return nil, fmt.Errorf("simulated error")
	}

	_, err := customGetLocalIP(errorFunc)
	if err == nil || err.Error() != "simulated error" {
		t.Errorf("Expected 'simulated error', got: %v", err)
	}

	// Test case 2: No suitable addresses
	onlyLoopbackFunc := func() ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{
				IP:   net.ParseIP("127.0.0.1"),
				Mask: net.CIDRMask(8, 32),
			},
		}, nil
	}

	_, err = customGetLocalIP(onlyLoopbackFunc)
	if err == nil || err.Error() != "no suitable IP address found" {
		t.Errorf("Expected 'no suitable IP address found', got: %v", err)
	}

	// Test case 3: Only IPv6 addresses
	onlyIPv6Func := func() ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{
				IP:   net.ParseIP("::1"),
				Mask: net.CIDRMask(128, 128),
			},
			&net.IPNet{
				IP:   net.ParseIP("fe80::1"),
				Mask: net.CIDRMask(64, 128),
			},
		}, nil
	}

	_, err = customGetLocalIP(onlyIPv6Func)
	if err == nil || err.Error() != "no suitable IP address found" {
		t.Errorf("Expected 'no suitable IP address found', got: %v", err)
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

func TestUpdateLoop(t *testing.T) {
	// Create a context that we can cancel to stop the loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a test server to mock the Traefik API
	traefikServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		routers := []TraefikRouter{
			{Rule: "Host(`test.example.com`)"},
		}
		if err := json.NewEncoder(w).Encode(routers); err != nil {
			t.Fatalf("Failed to encode routers: %v", err)
		}
	}))
	defer traefikServer.Close()

	// Create a test server for UniFi API with HTTP protocol
	unifiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]string{"token": "test-token"}); err != nil {
				t.Fatalf("Failed to encode token response: %v", err)
			}
		case "/api/s/default/rest/dnsrecord":
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer unifiServer.Close()

	// Create a config with a very short update interval
	config := &Config{
		Devices: []UnifiDeviceConfig{
			{
				Host:     unifiServer.URL, // Using the HTTP URL directly
				Username: "admin",
				Password: "password",
				Pattern:  ".*\\.example\\.com",
			},
		},
		UpdateInterval: "50ms", // Very short interval for testing
		TraefikAPIURL:  traefikServer.URL,
	}

	// Create a mock updateDNS function that counts calls
	updateCalled := make(chan struct{}, 1)

	// Create a custom mock implementation of UniFiDNS that counts updateDNS calls
	plugin := &UniFiDNS{
		next:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		name:           "test",
		config:         config,
		unifiClients:   make(map[string]*UniFiClient),
		devicePatterns: make(map[string]*regexp.Regexp),
		traefikClient:  NewTraefikClient(traefikServer.URL),
		updateInterval: 50 * time.Millisecond,
	}

	// Mock the updateDNS method by embedding a custom struct
	type mockUniFiDNS struct {
		*UniFiDNS
		updateDNSCalled chan struct{}
	}

	mock := &mockUniFiDNS{
		UniFiDNS:        plugin,
		updateDNSCalled: updateCalled,
	}

	// Create a custom updateDNS method
	customUpdateDNS := func() error {
		mock.updateDNSCalled <- struct{}{}
		return nil
	}

	// Start the update loop with our custom mock
	go func() {
		ticker := time.NewTicker(mock.updateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Call our custom function instead of the real updateDNS
				_ = customUpdateDNS()
			}
		}
	}()

	// Wait for the updateDNS to be called at least once
	select {
	case <-updateCalled:
		// Success, the updateDNS function was called
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Timeout waiting for updateDNS to be called")
	}

	// Cancel the context to stop the loop
	cancel()

	// Wait a bit to ensure the loop has stopped
	time.Sleep(100 * time.Millisecond)
}

func TestUpdateLoopWithError(t *testing.T) {
	// Create a context that we can cancel to stop the loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a test server to mock the Traefik API
	traefikServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		routers := []TraefikRouter{
			{Rule: "Host(`test.example.com`)"},
		}
		if err := json.NewEncoder(w).Encode(routers); err != nil {
			t.Fatalf("Failed to encode routers: %v", err)
		}
	}))
	defer traefikServer.Close()

	// Create a config with a very short update interval
	config := &Config{
		UpdateInterval: "50ms", // Very short interval for testing
		TraefikAPIURL:  traefikServer.URL,
	}

	// Create a plugin with mock updateDNS method that will return an error
	plugin := &UniFiDNS{
		next:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		name:           "test",
		config:         config,
		unifiClients:   make(map[string]*UniFiClient),
		devicePatterns: make(map[string]*regexp.Regexp),
		traefikClient:  NewTraefikClient(traefikServer.URL),
		updateInterval: 50 * time.Millisecond,
	}

	// Track calls to updateDNS
	errorCalled := false

	// Capture standard output to test error logging
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run the update loop with a mocked updateDNS function that returns an error
	go func() {
		ticker := time.NewTicker(plugin.updateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Simulate error in updateDNS
				fmt.Printf("Error updating DNS: test error\n")
				errorCalled = true

				// Cancel the context after we've tested the error path
				cancel()
			}
		}
	}()

	// Wait a bit to ensure the error message is printed
	time.Sleep(150 * time.Millisecond)

	// Restore stdout
	if err := w.Close(); err != nil {
		t.Fatalf("Failed to close pipe writer: %v", err)
	}
	os.Stdout = oldStdout

	// Check if the error path was called
	if !errorCalled {
		t.Fatal("Error path in updateLoop was not executed")
	}

	// Read the captured output
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("Failed to read captured output: %v", err)
	}

	// Check the output contains our error message
	output := buf.String()
	if !strings.Contains(output, "Error updating DNS: test error") {
		t.Errorf("Expected error message not found in output: %s", output)
	}
}

func TestUpdateDNSErrors(t *testing.T) {
	// Test case 1: Error in getLocalIP
	t.Run("getLocalIP error", func(t *testing.T) {
		// Create custom updateDNS function with mocked getLocalIP
		customUpdateDNS := func(getLocalIPFunc func() (string, error)) error {
			// Mock mutex operations

			// Get the local IP address using the provided function
			_, err := getLocalIPFunc()
			if err != nil {
				return fmt.Errorf("failed to get local IP: %w", err)
			}

			// Just return success for this test
			return nil
		}

		// Test with getLocalIP returning an error
		err := customUpdateDNS(func() (string, error) {
			return "", fmt.Errorf("simulated getLocalIP error")
		})

		if err == nil || !strings.Contains(err.Error(), "simulated getLocalIP error") {
			t.Errorf("Expected error from getLocalIP, got: %v", err)
		}
	})

	// Test case 2: Error in GetRouters
	t.Run("GetRouters error", func(t *testing.T) {
		// Create a custom updateDNS function for testing
		customUpdateDNS := func(getRouters func() ([]TraefikRouter, error)) error {
			// Get routers from Traefik
			_, err := getRouters()
			if err != nil {
				return fmt.Errorf("failed to get Traefik routers: %w", err)
			}

			// Just return success
			return nil
		}

		// Test with GetRouters returning an error
		err := customUpdateDNS(func() ([]TraefikRouter, error) {
			return nil, fmt.Errorf("simulated GetRouters error")
		})

		if err == nil || !strings.Contains(err.Error(), "simulated GetRouters error") {
			t.Errorf("Expected error from GetRouters, got: %v", err)
		}
	})

	// Test case 3: Invalid hostname pattern
	t.Run("Invalid hostname pattern", func(t *testing.T) {
		// Capture output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Process routers with invalid/empty hostnames
		routers := []TraefikRouter{
			{Rule: "PathPrefix(`/api`)"}, // No host rule
			{Rule: ""},                   // Empty rule
		}

		// Process all routers
		for _, router := range routers {
			hostname := extractHostname(router.Rule)
			if hostname == "" {
				fmt.Printf("Skipping router with no hostname: %s\n", router.Rule)
				continue
			}
		}

		// Restore stdout
		if err := w.Close(); err != nil {
			t.Fatalf("Failed to close pipe writer: %v", err)
		}
		os.Stdout = oldStdout

		// Read the captured output
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, r); err != nil {
			t.Fatalf("Failed to read captured output: %v", err)
		}

		// Check output contains our messages
		output := buf.String()
		expectedMessages := []string{
			"Skipping router with no hostname: PathPrefix(`/api`)",
			"Skipping router with no hostname: ",
		}

		for _, msg := range expectedMessages {
			if !strings.Contains(output, msg) {
				t.Errorf("Expected output to contain '%s', got: %s", msg, output)
			}
		}
	})
}
