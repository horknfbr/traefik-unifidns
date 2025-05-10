package traefikunifidns

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type TraefikRouter struct {
	Rule        string   `json:"rule"`
	Middlewares []string `json:"middlewares"`
	Service     string   `json:"service"`
	Name        string   `json:"name"`
}

type TraefikClient struct {
	client  *http.Client
	baseURL string
}

func NewTraefikClient(apiURL string, insecureSkipVerify bool) *TraefikClient {
	log.Printf("INFO: Creating new Traefik client for API URL: %s (insecureSkipVerify: %v)", apiURL, insecureSkipVerify)

	// Create custom transport with TLS configuration
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecureSkipVerify,
		},
	}

	return &TraefikClient{
		client: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
		baseURL: apiURL,
	}
}

func (c *TraefikClient) GetRouters() ([]TraefikRouter, error) {
	// Get router configurations from the Traefik API using direct HTTP
	url := fmt.Sprintf("%s/api/http/routers", c.baseURL)
	log.Printf("INFO: Fetching routers from Traefik API: %s", url)

	resp, err := c.client.Get(url)
	if err != nil {
		log.Printf("ERROR: Failed to get routers from Traefik API: %v", err)
		return nil, fmt.Errorf("failed to get routers: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// If we already have an error, don't override it
			if err == nil {
				log.Printf("ERROR: Failed to close response body: %v", closeErr)
				err = fmt.Errorf("failed to close response body: %w", closeErr)
			}
		}
	}()

	if resp.StatusCode != http.StatusOK {
		log.Printf("ERROR: Traefik API returned non-OK status code: %d", resp.StatusCode)
		return nil, fmt.Errorf("failed to get routers: status code %d", resp.StatusCode)
	}

	// First decode into a map to validate the structure
	var rawRouters []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawRouters); err != nil {
		log.Printf("ERROR: Failed to decode router response: %v", err)
		return nil, fmt.Errorf("failed to decode router response: %w", err)
	}

	// Convert and validate each router
	var routers []TraefikRouter
	log.Printf("INFO: Processing %d raw routers from API", len(rawRouters))
	for _, raw := range rawRouters {
		router := TraefikRouter{}

		// Validate required fields
		rule, ok := raw["rule"].(string)
		if !ok || rule == "" {
			log.Printf("WARN: Router has invalid or missing rule, skipping")
			continue
		}
		router.Rule = rule

		// Validate middlewares
		middlewares, ok := raw["middlewares"].([]interface{})
		if !ok {
			// Try to handle case where middlewares might be a single string
			if singleMiddleware, ok := raw["middlewares"].(string); ok {
				router.Middlewares = []string{singleMiddleware}
				log.Printf("INFO: Router %s has single middleware: %s", router.Name, singleMiddleware)
			} else {
				log.Printf("WARN: Invalid middlewares format in router data, skipping")
				continue
			}
		} else {
			// Convert middlewares to strings
			for _, m := range middlewares {
				if mStr, ok := m.(string); ok {
					router.Middlewares = append(router.Middlewares, mStr)
				}
			}
			log.Printf("INFO: Router %s has %d middlewares: %v", router.Name, len(router.Middlewares), router.Middlewares)
		}

		// Optional fields
		if name, ok := raw["name"].(string); ok {
			router.Name = name
		}
		if service, ok := raw["service"].(string); ok {
			router.Service = service
		}

		routers = append(routers, router)
		log.Printf("INFO: Added router %s to processing list", router.Name)
	}

	// Filter routers that have the UniFi DNS middleware
	var filteredRouters []TraefikRouter
	log.Printf("INFO: Filtering %d routers for UniFi DNS middleware", len(routers))
	for _, router := range routers {
		log.Printf("INFO: Checking router %s for UniFi DNS middleware", router.Name)
		for _, middleware := range router.Middlewares {
			log.Printf("INFO: Checking middleware: %s", middleware)
			if strings.Contains(middleware, "traefikunifidns") {
				log.Printf("INFO: Found router with UniFi DNS middleware: %s", router.Name)
				filteredRouters = append(filteredRouters, router)
				break
			}
		}
	}

	log.Printf("INFO: Successfully retrieved %d routers with UniFi DNS middleware from Traefik API", len(filteredRouters))
	return filteredRouters, nil
}

// extractHostname extracts the hostname from a Traefik rule
// Example rule: "Host(`example.com`)"
func extractHostname(rule string) string {
	// Match Host(`example.com`) pattern
	re := regexp.MustCompile(`Host\(` + "`" + `([^` + "`" + `]+)` + "`" + `\)`)
	matches := re.FindStringSubmatch(rule)
	if len(matches) > 1 {
		log.Printf("INFO: Extracted hostname from backtick rule: %s", matches[1])
		return strings.TrimSpace(matches[1])
	}

	// Match Host('example.com') pattern
	re = regexp.MustCompile(`Host\('([^']+)'\)`)
	matches = re.FindStringSubmatch(rule)
	if len(matches) > 1 {
		log.Printf("INFO: Extracted hostname from single-quote rule: %s", matches[1])
		return strings.TrimSpace(matches[1])
	}

	// Match Host("example.com") pattern
	re = regexp.MustCompile(`Host\("([^"]+)"\)`)
	matches = re.FindStringSubmatch(rule)
	if len(matches) > 1 {
		log.Printf("INFO: Extracted hostname from double-quote rule: %s", matches[1])
		return strings.TrimSpace(matches[1])
	}

	log.Printf("INFO: No hostname found in rule: %s", rule)
	return ""
}
