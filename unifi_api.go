package traefikunifidns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type UniFiClient struct {
	client   *http.Client
	baseURL  string
	username string
	password string
	token    string
}

func NewUniFiClient(host, username, password string) *UniFiClient {
	// Ensure host doesn't already include a protocol
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = fmt.Sprintf("https://%s", host)
	}

	return &UniFiClient{
		client:   &http.Client{Timeout: 10 * time.Second},
		baseURL:  host,
		username: username,
		password: password,
	}
}

func (c *UniFiClient) login() error {
	loginURL := fmt.Sprintf("%s/api/auth/login", c.baseURL)

	payload := map[string]string{
		"username": c.username,
		"password": c.password,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal login payload: %w", err)
	}

	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send login request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// If we already have an error, don't override it
			if err == nil {
				err = fmt.Errorf("failed to close response body: %w", closeErr)
			}
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed with status: %d", resp.StatusCode)
	}

	// Extract token from response
	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode login response: %w", err)
	}

	c.token = result.Token
	return nil
}

func (c *UniFiClient) updateDNSRecord(hostname, ip string) error {
	if c.token == "" {
		if err := c.login(); err != nil {
			return fmt.Errorf("failed to login: %w", err)
		}
	}

	updateURL := fmt.Sprintf("%s/api/s/default/rest/dnsrecord", c.baseURL)

	payload := map[string]interface{}{
		"name":    hostname,
		"type":    "A",
		"content": ip,
		"ttl":     300,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal DNS update payload: %w", err)
	}

	req, err := http.NewRequest("POST", updateURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create DNS update request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send DNS update request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// If we already have an error, don't override it
			if err == nil {
				err = fmt.Errorf("failed to close response body: %w", closeErr)
			}
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("DNS update failed with status: %d", resp.StatusCode)
	}

	return nil
}
