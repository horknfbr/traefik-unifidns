package traefikunifidns

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

type UniFiClient struct {
	client    *http.Client
	baseURL   string
	username  string
	password  string
	csrfToken string
}

type DNSEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	ID    string `json:"_id"`
}

func NewUniFiClient(host, username, password string, insecureSkipVerify bool) *UniFiClient {
	// Ensure host doesn't already include a protocol
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = fmt.Sprintf("https://%s", host)
	}

	log.Printf("INFO: Creating new UniFi client for host: %s (insecureSkipVerify: %v)", host, insecureSkipVerify)

	// Create cookie jar for session management
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Printf("ERROR: Failed to create cookie jar: %v", err)
		return nil
	}

	// Create custom transport with TLS configuration
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecureSkipVerify,
		},
	}

	return &UniFiClient{
		client: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
			Jar:       jar,
		},
		baseURL:  host,
		username: username,
		password: password,
	}
}

func (c *UniFiClient) login() error {
	log.Printf("INFO: Logging in to UniFi controller at %s", c.baseURL)

	loginURL := fmt.Sprintf("%s/api/auth/login", c.baseURL)
	payload := map[string]string{
		"username": c.username,
		"password": c.password,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("ERROR: Failed to marshal login payload: %v", err)
		return fmt.Errorf("failed to marshal login payload: %w", err)
	}

	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("ERROR: Failed to create login request: %v", err)
		return fmt.Errorf("failed to create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("ERROR: Failed to send login request: %v", err)
		return fmt.Errorf("failed to send login request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("ERROR: Failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		log.Printf("ERROR: Login failed with status code: %d", resp.StatusCode)
		return fmt.Errorf("login failed with status: %d", resp.StatusCode)
	}

	// Get and store CSRF token
	csrfToken := resp.Header.Get("X-Csrf-Token")
	if csrfToken == "" {
		log.Printf("ERROR: No CSRF token received in login response")
		return fmt.Errorf("no CSRF token received")
	}
	c.csrfToken = csrfToken

	log.Printf("INFO: Successfully logged in to UniFi controller")
	return nil
}

func (c *UniFiClient) GetStaticDNSEntries() ([]DNSEntry, error) {
	log.Printf("INFO: Getting static DNS entries from UniFi controller")

	// Ensure we're logged in and have a CSRF token
	if c.csrfToken == "" {
		if err := c.login(); err != nil {
			return nil, fmt.Errorf("failed to login before getting DNS entries: %w", err)
		}
	}

	dnsURL := fmt.Sprintf("%s/proxy/network/v2/api/site/default/static-dns", c.baseURL)
	req, err := http.NewRequest("GET", dnsURL, nil)
	if err != nil {
		log.Printf("ERROR: Failed to create DNS entries request: %v", err)
		return nil, fmt.Errorf("failed to create DNS entries request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Csrf-Token", c.csrfToken)

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("ERROR: Failed to send DNS entries request: %v", err)
		return nil, fmt.Errorf("failed to send DNS entries request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("ERROR: Failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		log.Printf("ERROR: Failed to get DNS entries with status code: %d", resp.StatusCode)
		return nil, fmt.Errorf("failed to get DNS entries with status: %d", resp.StatusCode)
	}

	var dnsEntries []DNSEntry
	if err := json.NewDecoder(resp.Body).Decode(&dnsEntries); err != nil {
		log.Printf("ERROR: Failed to decode DNS entries response: %v", err)
		return nil, fmt.Errorf("failed to decode DNS entries response: %w", err)
	}

	log.Printf("INFO: Successfully retrieved %d DNS entries", len(dnsEntries))
	return dnsEntries, nil
}

func (c *UniFiClient) updateDNSRecord(hostname, ip string) error {
	log.Printf("INFO: Checking DNS record for %s", hostname)

	// Get existing DNS entries
	entries, err := c.GetStaticDNSEntries()
	if err != nil {
		return fmt.Errorf("failed to get DNS entries before update: %w", err)
	}

	// Check if record exists and if IP has changed
	var existingEntry *DNSEntry
	for _, entry := range entries {
		if entry.Key == hostname {
			existingEntry = &entry
			if entry.Value == ip {
				log.Printf("INFO: DNS record for %s already has IP %s, no update needed", hostname, ip)
				return nil
			}
			log.Printf("INFO: Updating DNS record for %s from %s to %s", hostname, entry.Value, ip)
			break
		}
	}

	// Ensure we're logged in and have a CSRF token
	if c.csrfToken == "" {
		if err := c.login(); err != nil {
			return fmt.Errorf("failed to login before updating DNS: %w", err)
		}
	}

	baseURL := fmt.Sprintf("%s/proxy/network/v2/api/site/default/static-dns", c.baseURL)
	var req *http.Request

	if existingEntry != nil {
		// Update existing record
		updateURL := fmt.Sprintf("%s/%s", baseURL, existingEntry.ID)
		payload := map[string]interface{}{
			"key":         hostname,
			"record_type": "A",
			"value":       ip,
			"enabled":     true,
			"_id":         existingEntry.ID,
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			log.Printf("ERROR: Failed to marshal DNS update payload: %v", err)
			return fmt.Errorf("failed to marshal DNS update payload: %w", err)
		}

		req, err = http.NewRequest("PUT", updateURL, bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("ERROR: Failed to create DNS update request: %v", err)
			return fmt.Errorf("failed to create DNS update request: %w", err)
		}
	} else {
		// Create new record
		log.Printf("INFO: Creating new DNS record for %s with IP %s", hostname, ip)
		payload := map[string]interface{}{
			"key":         hostname,
			"record_type": "A",
			"value":       ip,
			"enabled":     true,
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			log.Printf("ERROR: Failed to marshal DNS create payload: %v", err)
			return fmt.Errorf("failed to marshal DNS create payload: %w", err)
		}

		req, err = http.NewRequest("POST", baseURL, bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("ERROR: Failed to create DNS create request: %v", err)
			return fmt.Errorf("failed to create DNS create request: %w", err)
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Csrf-Token", c.csrfToken)

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("ERROR: Failed to send DNS request: %v", err)
		return fmt.Errorf("failed to send DNS request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("ERROR: Failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		log.Printf("ERROR: DNS operation failed with status code: %d", resp.StatusCode)
		return fmt.Errorf("DNS operation failed with status: %d", resp.StatusCode)
	}

	if existingEntry != nil {
		log.Printf("INFO: Successfully updated DNS record for %s to IP %s", hostname, ip)
	} else {
		log.Printf("INFO: Successfully created new DNS record for %s with IP %s", hostname, ip)
	}
	return nil
}
