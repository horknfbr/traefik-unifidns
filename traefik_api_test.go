package traefikunifidns

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewTraefikClient(t *testing.T) {
	// Test with default settings
	client := NewTraefikClient("http://localhost:8080", false)
	if client == nil {
		t.Fatal("NewTraefikClient returned nil")
	}
	if client.baseURL != "http://localhost:8080" {
		t.Errorf("Expected baseURL to be 'http://localhost:8080', got '%s'", client.baseURL)
	}
	if client.client.Timeout != 10*time.Second {
		t.Error("Expected client timeout to be 10 seconds")
	}

	// Test with HTTPS and insecure skip verify
	client = NewTraefikClient("https://localhost:8080", true)
	if client == nil {
		t.Fatal("NewTraefikClient returned nil")
	}
	if client.baseURL != "https://localhost:8080" {
		t.Errorf("Expected baseURL to be 'https://localhost:8080', got '%s'", client.baseURL)
	}
}

func TestGetRouters(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http/routers" {
			t.Errorf("Expected path '/api/http/routers', got '%s'", r.URL.Path)
		}

		// Return sample router configurations
		routers := []TraefikRouter{
			{
				Name:        "router1",
				Rule:        "Host(`example.com`)",
				Service:     "service1",
				Middlewares: []string{"traefikunifidns"},
			},
			{
				Name:        "router2",
				Rule:        "Host(`test.com`)",
				Service:     "service2",
				Middlewares: []string{"other-middleware"},
			},
			{
				Name:        "router3",
				Rule:        "Host(`domain.com`)",
				Service:     "service3",
				Middlewares: []string{"traefikunifidns", "other-middleware"},
			},
			{
				Name:        "router4",
				Rule:        "Host(`multiple.com`)",
				Service:     "service4",
				Middlewares: []string{"traefikunifidns", "traefikunifidns"}, // Duplicate middleware
			},
		}
		json.NewEncoder(w).Encode(routers)
	}))
	defer server.Close()

	// Create client with test server URL
	client := &TraefikClient{
		client:  &http.Client{},
		baseURL: server.URL,
	}

	// Test GetRouters
	routers, err := client.GetRouters()
	if err != nil {
		t.Fatalf("GetRouters returned error: %v", err)
	}

	// Should only get routers with traefikunifidns middleware
	if len(routers) != 3 {
		t.Errorf("Expected 3 routers with UniFi DNS middleware, got %d", len(routers))
	}

	// Check first router
	if routers[0].Name != "router1" {
		t.Errorf("Expected router name 'router1', got '%s'", routers[0].Name)
	}
	if routers[0].Rule != "Host(`example.com`)" {
		t.Errorf("Expected rule 'Host(`example.com`)', got '%s'", routers[0].Rule)
	}
	if routers[0].Service != "service1" {
		t.Errorf("Expected service 'service1', got '%s'", routers[0].Service)
	}

	// Check second router
	if routers[1].Name != "router3" {
		t.Errorf("Expected router name 'router3', got '%s'", routers[1].Name)
	}
	if routers[1].Rule != "Host(`domain.com`)" {
		t.Errorf("Expected rule 'Host(`domain.com`)', got '%s'", routers[1].Rule)
	}
	if routers[1].Service != "service3" {
		t.Errorf("Expected service 'service3', got '%s'", routers[1].Service)
	}

	// Check third router (with duplicate middleware)
	if routers[2].Name != "router4" {
		t.Errorf("Expected router name 'router4', got '%s'", routers[2].Name)
	}
	if routers[2].Rule != "Host(`multiple.com`)" {
		t.Errorf("Expected rule 'Host(`multiple.com`)', got '%s'", routers[2].Rule)
	}
	if routers[2].Service != "service4" {
		t.Errorf("Expected service 'service4', got '%s'", routers[2].Service)
	}
}

func TestGetRoutersErrors(t *testing.T) {
	// Test case 1: HTTP request error
	t.Run("HTTP request error", func(t *testing.T) {
		client := &TraefikClient{
			client:  &http.Client{},
			baseURL: "http://invalid-url-that-will-fail:12345",
		}

		_, err := client.GetRouters()
		if err == nil {
			t.Error("Expected error for invalid URL, got nil")
		}
	})

	// Test case 2: Non-200 status code
	t.Run("Non-200 status code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		client := &TraefikClient{
			client:  &http.Client{},
			baseURL: server.URL,
		}

		_, err := client.GetRouters()
		if err == nil {
			t.Error("Expected error for non-200 status code, got nil")
		}
	})

	// Test case 3: Invalid JSON response
	t.Run("Invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		client := &TraefikClient{
			client:  &http.Client{},
			baseURL: server.URL,
		}

		_, err := client.GetRouters()
		if err == nil {
			t.Error("Expected error for invalid JSON, got nil")
		}
	})

	// Test case 4: Empty routers list
	t.Run("Empty routers list", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode([]TraefikRouter{})
		}))
		defer server.Close()

		client := &TraefikClient{
			client:  &http.Client{},
			baseURL: server.URL,
		}

		routers, err := client.GetRouters()
		if err != nil {
			t.Fatalf("GetRouters returned error: %v", err)
		}
		if len(routers) != 0 {
			t.Errorf("Expected 0 routers, got %d", len(routers))
		}
	})

	// Test case 5: Malformed router data
	t.Run("Malformed router data", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"rule": "Host(` + "`" + `example.com` + "`" + `)", "middlewares": "invalid"}]`))
		}))
		defer server.Close()

		client := NewTraefikClient(server.URL, false)
		routers, err := client.GetRouters()
		if err != nil {
			t.Errorf("Expected no error for malformed router data, got %v", err)
		}
		if len(routers) != 0 {
			t.Errorf("Expected 0 routers for malformed data, got %d", len(routers))
		}
	})

	// Test case 6: Response body close error
	t.Run("Response body close error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set content length to force a body close error
			w.Header().Set("Content-Length", "1")
			w.Write([]byte("{"))
		}))
		defer server.Close()

		client := &TraefikClient{
			client:  &http.Client{},
			baseURL: server.URL,
		}

		_, err := client.GetRouters()
		if err == nil {
			t.Error("Expected error for response body close error, got nil")
		}
	})
}

func TestExtractHostname(t *testing.T) {
	testCases := []struct {
		name     string
		rule     string
		expected string
	}{
		{
			name:     "Backtick hostname",
			rule:     "Host(`example.com`)",
			expected: "example.com",
		},
		{
			name:     "Single quote hostname",
			rule:     "Host('test.com')",
			expected: "test.com",
		},
		{
			name:     "Double quote hostname",
			rule:     "Host(\"domain.com\")",
			expected: "domain.com",
		},
		{
			name:     "No hostname",
			rule:     "Path(`/api`)",
			expected: "",
		},
		{
			name:     "Empty rule",
			rule:     "",
			expected: "",
		},
		{
			name:     "Invalid host rule",
			rule:     "Host(example.com)",
			expected: "",
		},
		{
			name:     "Multiple host rules",
			rule:     "Host(`example.com`) && Path(`/api`)",
			expected: "example.com",
		},
		{
			name:     "Host rule with spaces",
			rule:     "Host(` example.com `)",
			expected: "example.com",
		},
		{
			name:     "Host rule with special characters",
			rule:     "Host(`example.com:8080`)",
			expected: "example.com:8080",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractHostname(tc.rule)
			if result != tc.expected {
				t.Errorf("Expected hostname '%s', got '%s'", tc.expected, result)
			}
		})
	}
}
