package trafikunifidns

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewUniFiClient(t *testing.T) {
	client := NewUniFiClient("192.168.1.1", "admin", "password")
	if client == nil {
		t.Fatal("NewUniFiClient returned nil")
	}
	if client.baseURL != "https://192.168.1.1" {
		t.Errorf("Expected baseURL to be 'https://192.168.1.1', got '%s'", client.baseURL)
	}
	if client.username != "admin" {
		t.Errorf("Expected username to be 'admin', got '%s'", client.username)
	}
	if client.password != "password" {
		t.Errorf("Expected password to be 'password', got '%s'", client.password)
	}
}

func TestUniFiClientLogin(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/api/auth/login" {
			t.Errorf("Expected path '/api/auth/login', got '%s'", r.URL.Path)
		}

		// Check request body
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}
		if payload["username"] != "admin" {
			t.Errorf("Expected username 'admin', got '%s'", payload["username"])
		}
		if payload["password"] != "password" {
			t.Errorf("Expected password 'password', got '%s'", payload["password"])
		}

		// Return a token
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"token": "test-token"})
	}))
	defer server.Close()

	// Create client with test server URL
	client := &UniFiClient{
		client:   &http.Client{},
		baseURL:  server.URL,
		username: "admin",
		password: "password",
	}

	// Test login
	err := client.login()
	if err != nil {
		t.Fatalf("login returned error: %v", err)
	}
	if client.token != "test-token" {
		t.Errorf("Expected token 'test-token', got '%s'", client.token)
	}
}

func TestUniFiClientUpdateDNSRecord(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/api/s/default/rest/dnsrecord" {
			t.Errorf("Expected path '/api/s/default/rest/dnsrecord', got '%s'", r.URL.Path)
		}

		// Check authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Expected Authorization 'Bearer test-token', got '%s'", auth)
		}

		// Check request body
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}
		if payload["name"] != "example.com" {
			t.Errorf("Expected name 'example.com', got '%v'", payload["name"])
		}
		if payload["type"] != "A" {
			t.Errorf("Expected type 'A', got '%v'", payload["type"])
		}
		if payload["content"] != "192.168.1.100" {
			t.Errorf("Expected content '192.168.1.100', got '%v'", payload["content"])
		}
		if payload["ttl"] != float64(300) {
			t.Errorf("Expected ttl 300, got '%v'", payload["ttl"])
		}

		// Return success
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create client with test server URL and token
	client := &UniFiClient{
		client:   &http.Client{},
		baseURL:  server.URL,
		username: "admin",
		password: "password",
		token:    "test-token",
	}

	// Test updateDNSRecord
	err := client.updateDNSRecord("example.com", "192.168.1.100")
	if err != nil {
		t.Fatalf("updateDNSRecord returned error: %v", err)
	}
}
