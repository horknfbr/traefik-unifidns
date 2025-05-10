package traefikunifidns

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewUniFiClient(t *testing.T) {
	client := NewUniFiClient("192.168.1.1", "admin", "password", false)
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
	if client.client.Jar == nil {
		t.Error("Expected cookie jar to be initialized")
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

		// Set CSRF token in response header
		w.Header().Set("X-Csrf-Token", "test-csrf-token")
		w.WriteHeader(http.StatusOK)
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
	if client.csrfToken != "test-csrf-token" {
		t.Errorf("Expected CSRF token 'test-csrf-token', got '%s'", client.csrfToken)
	}
}

func TestUniFiClientLoginErrors(t *testing.T) {
	// Test case 1: HTTP request error
	t.Run("HTTP request error", func(t *testing.T) {
		client := &UniFiClient{
			client:   &http.Client{},
			baseURL:  "http://invalid-url-that-will-fail:12345",
			username: "admin",
			password: "password",
		}

		err := client.login()
		if err == nil {
			t.Error("Expected error for invalid URL, got nil")
		}
	})

	// Test case 2: Non-200 status code
	t.Run("Non-200 status code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		client := &UniFiClient{
			client:   &http.Client{},
			baseURL:  server.URL,
			username: "admin",
			password: "password",
		}

		err := client.login()
		if err == nil {
			t.Error("Expected error for non-200 status code, got nil")
		}
	})

	// Test case 3: Missing CSRF token
	t.Run("Missing CSRF token", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			// No CSRF token in header
		}))
		defer server.Close()

		client := &UniFiClient{
			client:   &http.Client{},
			baseURL:  server.URL,
			username: "admin",
			password: "password",
		}

		err := client.login()
		if err == nil {
			t.Error("Expected error for missing CSRF token, got nil")
		}
	})
}

func TestGetStaticDNSEntries(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			// Check login request
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Failed to decode login request body: %v", err)
			}
			if payload["username"] != "admin" || payload["password"] != "password" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("X-Csrf-Token", "test-csrf-token")
			w.WriteHeader(http.StatusOK)

		case "/proxy/network/v2/api/site/default/static-dns":
			// Check headers
			if r.Header.Get("X-Csrf-Token") != "test-csrf-token" {
				t.Errorf("Expected CSRF token 'test-csrf-token', got '%s'", r.Header.Get("X-Csrf-Token"))
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type 'application/json', got '%s'", r.Header.Get("Content-Type"))
			}

			// Return sample DNS entries
			entries := []DNSEntry{
				{Key: "example.com", Value: "192.168.1.100", ID: "1"},
				{Key: "test.com", Value: "192.168.1.101", ID: "2"},
			}
			json.NewEncoder(w).Encode(entries)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("Unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create client with test server URL
	client := &UniFiClient{
		client:   &http.Client{},
		baseURL:  server.URL,
		username: "admin",
		password: "password",
	}

	// Test GetStaticDNSEntries
	entries, err := client.GetStaticDNSEntries()
	if err != nil {
		t.Fatalf("GetStaticDNSEntries returned error: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("Expected 2 DNS entries, got %d", len(entries))
	}

	expectedEntries := []DNSEntry{
		{Key: "example.com", Value: "192.168.1.100", ID: "1"},
		{Key: "test.com", Value: "192.168.1.101", ID: "2"},
	}

	for i, entry := range entries {
		if entry != expectedEntries[i] {
			t.Errorf("Entry %d mismatch: expected %+v, got %+v", i, expectedEntries[i], entry)
		}
	}
}

func TestGetStaticDNSEntriesErrors(t *testing.T) {
	// Test case 1: HTTP request error
	t.Run("HTTP request error", func(t *testing.T) {
		client := &UniFiClient{
			client:   &http.Client{},
			baseURL:  "http://invalid-url-that-will-fail:12345",
			username: "admin",
			password: "password",
		}

		_, err := client.GetStaticDNSEntries()
		if err == nil {
			t.Error("Expected error for invalid URL, got nil")
		}
	})

	// Test case 2: Non-200 status code when getting DNS entries
	t.Run("Non-200 status code when getting DNS entries", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/auth/login":
				w.Header().Set("X-Csrf-Token", "test-csrf-token")
				w.WriteHeader(http.StatusOK)
			case "/proxy/network/v2/api/site/default/static-dns":
				w.WriteHeader(http.StatusBadRequest)
			default:
				t.Errorf("Unexpected path: %s", r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		client := &UniFiClient{
			client:   &http.Client{},
			baseURL:  server.URL,
			username: "admin",
			password: "password",
		}

		_, err := client.GetStaticDNSEntries()
		if err == nil {
			t.Error("Expected error for non-200 status code, got nil")
		}
	})
}

func TestUniFiClientUpdateDNSRecord(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			// Check login request
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Failed to decode login request body: %v", err)
			}
			if payload["username"] != "admin" || payload["password"] != "password" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("X-Csrf-Token", "test-csrf-token")
			w.WriteHeader(http.StatusOK)

		case "/proxy/network/v2/api/site/default/static-dns":
			// Check headers
			if r.Header.Get("X-Csrf-Token") != "test-csrf-token" {
				t.Errorf("Expected CSRF token 'test-csrf-token', got '%s'", r.Header.Get("X-Csrf-Token"))
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type 'application/json', got '%s'", r.Header.Get("Content-Type"))
			}

			// Handle GET request for existing DNS entries
			if r.Method == "GET" {
				// For empty DNS entries test
				if r.Header.Get("X-Test-Empty-DNS") == "true" {
					json.NewEncoder(w).Encode([]DNSEntry{})
					w.WriteHeader(http.StatusOK)
					return
				}

				// For invalid JSON response test
				if r.Header.Get("X-Test-Invalid-JSON") == "true" {
					w.Write([]byte("invalid json"))
					w.WriteHeader(http.StatusOK)
					return
				}

				entries := []DNSEntry{
					{Key: "example.com", Value: "192.168.1.100", ID: "1"},
					{Key: "test.com", Value: "192.168.1.101", ID: "2"},
				}
				json.NewEncoder(w).Encode(entries)
				w.WriteHeader(http.StatusOK)
				return
			}

			// Handle POST request for new records
			if r.Method == "POST" {
				var payload map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("Failed to decode DNS create request body: %v", err)
				}

				// Check common fields
				if payload["record_type"] != "A" {
					t.Errorf("Expected record_type 'A', got '%v'", payload["record_type"])
				}
				if payload["enabled"] != true {
					t.Errorf("Expected enabled true, got '%v'", payload["enabled"])
				}

				// For newdomain.com test case
				if payload["key"] == "newdomain.com" {
					if payload["value"] != "192.168.1.200" {
						t.Errorf("Expected value '192.168.1.200', got '%v'", payload["value"])
					}
				}

				// Return success
				w.WriteHeader(http.StatusOK)
				return
			}
		case "/proxy/network/v2/api/site/default/static-dns/1":
			// Handle PUT request for updating existing records
			if r.Method == "PUT" {
				var payload map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("Failed to decode DNS update request body: %v", err)
				}

				// Check common fields
				if payload["record_type"] != "A" {
					t.Errorf("Expected record_type 'A', got '%v'", payload["record_type"])
				}
				if payload["enabled"] != true {
					t.Errorf("Expected enabled true, got '%v'", payload["enabled"])
				}

				// For example.com test case
				if payload["key"] != "example.com" {
					t.Errorf("Expected key 'example.com', got '%v'", payload["key"])
				}
				if payload["value"] != "192.168.1.200" {
					t.Errorf("Expected value '192.168.1.200', got '%v'", payload["value"])
				}
				if payload["_id"] != "1" {
					t.Errorf("Expected _id '1', got '%v'", payload["_id"])
				}

				// Return success
				w.WriteHeader(http.StatusOK)
				return
			}
		default:
			t.Errorf("Unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create client with test server URL
	client := &UniFiClient{
		client:   &http.Client{},
		baseURL:  server.URL,
		username: "admin",
		password: "password",
	}

	// Test case 1: Update existing record with new IP
	t.Run("Update existing record with new IP", func(t *testing.T) {
		err := client.updateDNSRecord("example.com", "192.168.1.200")
		if err != nil {
			t.Fatalf("updateDNSRecord returned error: %v", err)
		}
	})

	// Test case 2: No update needed (same IP)
	t.Run("No update needed - same IP", func(t *testing.T) {
		err := client.updateDNSRecord("example.com", "192.168.1.100")
		if err != nil {
			t.Fatalf("updateDNSRecord returned error: %v", err)
		}
	})

	// Test case 3: Update non-existent record
	t.Run("Update non-existent record", func(t *testing.T) {
		err := client.updateDNSRecord("newdomain.com", "192.168.1.200")
		if err != nil {
			t.Fatalf("updateDNSRecord returned error: %v", err)
		}
	})

	// Test case 4: Empty DNS entries
	t.Run("Empty DNS entries", func(t *testing.T) {
		client.client.Transport = &headerTransport{
			headers: map[string]string{"X-Test-Empty-DNS": "true"},
			base:    http.DefaultTransport,
		}
		err := client.updateDNSRecord("example.com", "192.168.1.200")
		if err != nil {
			t.Fatalf("updateDNSRecord returned error: %v", err)
		}
	})

	// Test case 5: Invalid JSON response
	t.Run("Invalid JSON response", func(t *testing.T) {
		client.client.Transport = &headerTransport{
			headers: map[string]string{"X-Test-Invalid-JSON": "true"},
			base:    http.DefaultTransport,
		}
		err := client.updateDNSRecord("example.com", "192.168.1.200")
		if err == nil {
			t.Fatal("Expected error for invalid JSON response, got nil")
		}
		if !strings.Contains(err.Error(), "failed to decode DNS entries response") {
			t.Errorf("Expected error to contain 'failed to decode DNS entries response', got: %v", err)
		}
	})
}

// headerTransport is a custom transport that adds headers to requests
type headerTransport struct {
	headers map[string]string
	base    http.RoundTripper
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range t.headers {
		req.Header.Add(k, v)
	}
	return t.base.RoundTrip(req)
}

func TestUniFiClientUpdateDNSRecordErrors(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			// Check login request
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Failed to decode login request body: %v", err)
			}
			if payload["username"] != "admin" || payload["password"] != "password" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("X-Csrf-Token", "test-csrf-token")
			w.WriteHeader(http.StatusOK)

		case "/proxy/network/v2/api/site/default/static-dns":
			// Check headers
			if r.Header.Get("X-Csrf-Token") != "test-csrf-token" {
				t.Errorf("Expected CSRF token 'test-csrf-token', got '%s'", r.Header.Get("X-Csrf-Token"))
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type 'application/json', got '%s'", r.Header.Get("Content-Type"))
			}

			// Handle GET request for existing DNS entries
			if r.Method == "GET" {
				// For HTTP request error test
				if r.Header.Get("X-Test-HTTP-Error") == "true" {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				// For non-200 status code test
				if r.Header.Get("X-Test-Non-200") == "true" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				entries := []DNSEntry{
					{Key: "example.com", Value: "192.168.1.100", ID: "1"},
					{Key: "test.com", Value: "192.168.1.101", ID: "2"},
				}
				json.NewEncoder(w).Encode(entries)
				w.WriteHeader(http.StatusOK)
				return
			}

			// Handle POST request for new records
			if r.Method == "POST" {
				// For non-200 status code test
				if r.Header.Get("X-Test-Non-200") == "true" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				var payload map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("Failed to decode DNS create request body: %v", err)
				}

				// Return success
				w.WriteHeader(http.StatusOK)
				return
			}
		case "/proxy/network/v2/api/site/default/static-dns/1":
			// Handle PUT request for updating existing records
			if r.Method == "PUT" {
				// For non-200 status code test
				if r.Header.Get("X-Test-Non-200") == "true" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				var payload map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("Failed to decode DNS update request body: %v", err)
				}

				// Return success
				w.WriteHeader(http.StatusOK)
				return
			}
		default:
			t.Errorf("Unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create client with test server URL
	client := &UniFiClient{
		client:   &http.Client{},
		baseURL:  server.URL,
		username: "admin",
		password: "password",
	}

	// Test case 1: HTTP request error
	t.Run("HTTP request error", func(t *testing.T) {
		client.client.Transport = &headerTransport{
			headers: map[string]string{"X-Test-HTTP-Error": "true"},
			base:    http.DefaultTransport,
		}
		err := client.updateDNSRecord("example.com", "192.168.1.200")
		if err == nil {
			t.Fatal("Expected error for HTTP request error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get DNS entries before update") {
			t.Errorf("Expected error to contain 'failed to get DNS entries before update', got: %v", err)
		}
	})

	// Test case 2: Non-200 status code when getting DNS entries
	t.Run("Non-200 status code when getting DNS entries", func(t *testing.T) {
		client.client.Transport = &headerTransport{
			headers: map[string]string{"X-Test-Non-200": "true"},
			base:    http.DefaultTransport,
		}
		err := client.updateDNSRecord("example.com", "192.168.1.200")
		if err == nil {
			t.Fatal("Expected error for non-200 status code, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get DNS entries before update") {
			t.Errorf("Expected error to contain 'failed to get DNS entries before update', got: %v", err)
		}
	})

	// Test case 3: Non-200 status code when updating DNS
	t.Run("Non-200 status code when updating DNS", func(t *testing.T) {
		client.client.Transport = &headerTransport{
			headers: map[string]string{"X-Test-Non-200": "true"},
			base:    http.DefaultTransport,
		}
		err := client.updateDNSRecord("example.com", "192.168.1.200")
		if err == nil {
			t.Fatal("Expected error for non-200 status code, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get DNS entries before update") {
			t.Errorf("Expected error to contain 'failed to get DNS entries before update', got: %v", err)
		}
	})
}
