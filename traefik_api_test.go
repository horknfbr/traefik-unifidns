package traefikunifidns

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewTraefikClient(t *testing.T) {
	client := NewTraefikClient("http://localhost:8080")
	if client == nil {
		t.Fatal("NewTraefikClient returned nil")
	}
	if client.baseURL != "http://localhost:8080" {
		t.Errorf("Expected baseURL to be 'http://localhost:8080', got '%s'", client.baseURL)
	}
}

func TestTraefikClientGetRouters(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			{Rule: "Host(\"domain.com\")"},
			{Rule: "PathPrefix(`/api`)"}, // No host rule
		}
		if err := json.NewEncoder(w).Encode(routers); err != nil {
			t.Fatalf("Failed to encode routers: %v", err)
		}
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
	if len(routers) != 4 {
		t.Errorf("Expected 4 routers, got %d", len(routers))
	}
}

func TestTraefikClientGetRoutersWithErrors(t *testing.T) {
	// Test case 1: Error on HTTP request
	t.Run("HTTP request error", func(t *testing.T) {
		// Create a client with an invalid URL to force a request error
		client := &TraefikClient{
			client:  &http.Client{},
			baseURL: "http://invalid-url-that-will-fail:12345",
		}

		// Test GetRouters with an invalid URL
		_, err := client.GetRouters()
		if err == nil {
			t.Error("Expected error for invalid URL, got nil")
		}
	})

	// Test case 2: Non-200 status code
	t.Run("Non-200 status code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
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
			w.Header().Set("Content-Type", "application/json")
			// Return invalid JSON
			_, err := w.Write([]byte("{invalid json"))
			if err != nil {
				t.Fatalf("Failed to write response: %v", err)
			}
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
}

func TestExtractHostname(t *testing.T) {
	tests := []struct {
		name     string
		rule     string
		expected string
	}{
		{
			name:     "Backtick format",
			rule:     "Host(`example.com`)",
			expected: "example.com",
		},
		{
			name:     "Single quote format",
			rule:     "Host('test.com')",
			expected: "test.com",
		},
		{
			name:     "Double quote format",
			rule:     "Host(\"domain.com\")",
			expected: "domain.com",
		},
		{
			name:     "No host rule",
			rule:     "PathPrefix(`/api`)",
			expected: "",
		},
		{
			name:     "Empty rule",
			rule:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractHostname(tt.rule)
			if result != tt.expected {
				t.Errorf("extractHostname(%q) = %q, want %q", tt.rule, result, tt.expected)
			}
		})
	}
}
