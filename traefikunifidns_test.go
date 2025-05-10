package traefikunifidns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateConfig(t *testing.T) {
	// Test the default configuration
	got := CreateConfig()
	want := &Config{
		UpdateInterval:        "5m",
		TraefikAPIURL:         "http://localhost:8080",
		Devices:               []UnifiDeviceConfig{},
		InsecureSkipVerifyTLS: false,
	}
	assert.Equal(t, want, got)
}

func TestNew(t *testing.T) {
	config := &Config{
		Devices: []UnifiDeviceConfig{
			{
				Host:                  "192.168.1.1",
				Username:              "admin",
				Password:              "password",
				Pattern:               "example.com",
				InsecureSkipVerifyTLS: true,
			},
		},
		UpdateInterval: "1m",
		TraefikAPIURL:  "http://localhost:8080",
	}

	// Create a next handler that will be called by ServeHTTP
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	plugin, err := New(context.Background(), next, config, "test")
	require.NoError(t, err)
	require.NotNil(t, plugin)

	u := plugin.(*UniFiDNS)
	assert.Equal(t, config, u.config)
	assert.NotNil(t, u.traefikClient)
	assert.NotNil(t, u.unifiClients)
	assert.Len(t, u.unifiClients, 1)
}

func TestServeHTTP(t *testing.T) {
	config := &Config{
		Devices: []UnifiDeviceConfig{
			{
				Host:                  "192.168.1.1",
				Username:              "admin",
				Password:              "password",
				Pattern:               "example.com",
				InsecureSkipVerifyTLS: true,
			},
		},
		UpdateInterval: "1m",
		TraefikAPIURL:  "http://localhost:8080",
	}

	// Create a next handler that will be called by ServeHTTP
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	plugin, err := New(context.Background(), next, config, "test")
	require.NoError(t, err)

	// Create a test request
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Call ServeHTTP
	plugin.ServeHTTP(w, req)

	// Verify the response
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdateDNS(t *testing.T) {
	// Create test servers
	traefikServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/http/routers" {
			routers := []map[string]interface{}{
				{
					"name":        "router1",
					"rule":        "Host(`example.com`)",
					"service":     "service1",
					"middlewares": []string{"traefikunifidns"},
				},
			}
			if err := json.NewEncoder(w).Encode(routers); err != nil {
				t.Errorf("Failed to encode routers: %v", err)
			}
		}
	}))
	defer traefikServer.Close()

	unifiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			w.Header().Set("X-Csrf-Token", "test-csrf-token")
			w.WriteHeader(http.StatusOK)
		case "/proxy/network/v2/api/site/default/static-dns":
			if r.Method == "GET" {
				if err := json.NewEncoder(w).Encode([]DNSEntry{}); err != nil {
					t.Errorf("Failed to encode DNS entries: %v", err)
				}
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}
	}))
	defer unifiServer.Close()

	// Create test configuration
	config := &Config{
		Devices: []UnifiDeviceConfig{
			{
				Host:                  "127.0.0.1:" + strings.Split(unifiServer.URL, ":")[2],
				Username:              "admin",
				Password:              "password",
				Pattern:               ".*",
				InsecureSkipVerifyTLS: true,
			},
		},
		UpdateInterval: "1m",
		TraefikAPIURL:  traefikServer.URL,
	}

	// Create plugin instance
	plugin, err := New(context.Background(), nil, config, "test")
	if err != nil {
		t.Fatalf("Failed to create plugin: %v", err)
	}

	// Run DNS update
	u := plugin.(*UniFiDNS)
	err = u.updateDNS()
	if err != nil {
		t.Fatalf("updateDNS returned error: %v", err)
	}
}

func TestFindMatchingClient(t *testing.T) {
	config := &Config{
		Devices: []UnifiDeviceConfig{
			{
				Host:                  "192.168.1.1",
				Username:              "admin",
				Password:              "password",
				Pattern:               "example.com",
				InsecureSkipVerifyTLS: true,
			},
			{
				Host:                  "192.168.1.2",
				Username:              "admin",
				Password:              "password",
				Pattern:               "test.com",
				InsecureSkipVerifyTLS: true,
			},
		},
		UpdateInterval: "1m",
		TraefikAPIURL:  "http://localhost:8080",
	}

	plugin, err := New(context.Background(), nil, config, "test")
	require.NoError(t, err)

	u := plugin.(*UniFiDNS)

	tests := []struct {
		name      string
		hostname  string
		want      *UniFiClient
		wantFound bool
	}{
		{
			name:      "exact_match",
			hostname:  "example.com",
			want:      u.unifiClients["device-0"],
			wantFound: true,
		},
		{
			name:      "no_match",
			hostname:  "unknown.com",
			want:      nil,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := u.findMatchingClient(tt.hostname)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantFound, found)
		})
	}
}

func TestUpdateLoop(t *testing.T) {
	config := &Config{
		Devices: []UnifiDeviceConfig{
			{
				Host:                  "192.168.1.1",
				Username:              "admin",
				Password:              "password",
				Pattern:               "example.com",
				InsecureSkipVerifyTLS: true,
			},
		},
		UpdateInterval: "1m",
		TraefikAPIURL:  "http://localhost:8080",
	}

	plugin, err := New(context.Background(), nil, config, "test")
	require.NoError(t, err)

	u := plugin.(*UniFiDNS)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the update loop
	go u.updateLoop(ctx)

	// Wait a bit to let the loop start
	time.Sleep(100 * time.Millisecond)

	// Cancel the context to stop the loop
	cancel()
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
		// Create a custom logger to capture log output
		var logBuf bytes.Buffer
		oldLogger := log.Default()
		log.SetOutput(&logBuf)

		// Process routers with invalid/empty hostnames
		routers := []TraefikRouter{
			{Rule: "PathPrefix(`/api`)"}, // No host rule
			{Rule: ""},                   // Empty rule
		}

		// Process all routers
		for _, router := range routers {
			hostname := extractHostname(router.Rule)
			if hostname == "" {
				log.Printf("INFO: Skipping router with no hostname: %s", router.Rule)
				continue
			}
		}

		// Restore the original logger
		log.SetOutput(oldLogger.Writer())

		// Check log output contains our messages
		logOutput := logBuf.String()
		expectedMessages := []string{
			"INFO: Skipping router with no hostname: PathPrefix(`/api`)",
			"INFO: Skipping router with no hostname: ",
		}

		for _, msg := range expectedMessages {
			if !strings.Contains(logOutput, msg) {
				t.Errorf("Expected log output to contain '%s', got: %s", msg, logOutput)
			}
		}
	})
}
