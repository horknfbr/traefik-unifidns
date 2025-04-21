package trafikunifidns

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
	if client.apiURL != "http://localhost:8080" {
		t.Errorf("Expected apiURL to be 'http://localhost:8080', got '%s'", client.apiURL)
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
		json.NewEncoder(w).Encode(routers)
	}))
	defer server.Close()

	// Create client with test server URL
	client := &TraefikClient{
		client: &http.Client{},
		apiURL: server.URL,
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
