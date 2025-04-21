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
	client *http.Client
	apiURL string
}

func NewTraefikClient(apiURL string) *TraefikClient {
	return &TraefikClient{
		client: &http.Client{Timeout: 10 * time.Second},
		apiURL: apiURL,
	}
}

func (c *TraefikClient) GetRouters() ([]TraefikRouter, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/http/routers", c.apiURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get routers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get routers: status %d", resp.StatusCode)
	}

	var routers []TraefikRouter
	if err := json.NewDecoder(resp.Body).Decode(&routers); err != nil {
		return nil, fmt.Errorf("failed to decode routers: %w", err)
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
