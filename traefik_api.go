package trafikunifidns

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

type TraefikRouter struct {
	Rule string `json:"rule"`
}

type TraefikClient struct {
	client  *http.Client
	baseURL string
}

func NewTraefikClient(apiURL string) *TraefikClient {
	return &TraefikClient{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: apiURL,
	}
}

func (c *TraefikClient) GetRouters() ([]TraefikRouter, error) {
	// Get router configurations from the Traefik API using direct HTTP
	url := fmt.Sprintf("%s/api/http/routers", c.baseURL)
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get routers: %w", err)
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
		return nil, fmt.Errorf("failed to get routers: status code %d", resp.StatusCode)
	}

	// Decode router information from JSON response
	var routers []TraefikRouter
	if err := json.NewDecoder(resp.Body).Decode(&routers); err != nil {
		return nil, fmt.Errorf("failed to decode router response: %w", err)
	}

	return routers, nil
}

// extractHostname extracts the hostname from a Traefik rule
// Example rule: "Host(`example.com`)"
func extractHostname(rule string) string {
	// Match Host(`example.com`) pattern
	re := regexp.MustCompile(`Host\(` + "`" + `([^` + "`" + `]+)` + "`" + `\)`)
	matches := re.FindStringSubmatch(rule)
	if len(matches) > 1 {
		return matches[1]
	}

	// Match Host('example.com') pattern
	re = regexp.MustCompile(`Host\('([^']+)'\)`)
	matches = re.FindStringSubmatch(rule)
	if len(matches) > 1 {
		return matches[1]
	}

	// Match Host("example.com") pattern
	re = regexp.MustCompile(`Host\("([^"]+)"\)`)
	matches = re.FindStringSubmatch(rule)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}
