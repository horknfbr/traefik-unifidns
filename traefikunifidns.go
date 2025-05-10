package traefikunifidns

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"sync"
	"time"
)

// UnifiDeviceConfig represents configuration for a single UniFi device
type UnifiDeviceConfig struct {
	Host                  string `json:"host"`
	Username              string `json:"username"`
	Password              string `json:"password"`
	Pattern               string `json:"pattern"` // Regex pattern to match domain names
	InsecureSkipVerifyTLS bool   `json:"insecureSkipVerifyTLS,omitempty"`
}

// Config the plugin configuration.
type Config struct {
	Devices               []UnifiDeviceConfig `json:"devices"`
	UpdateInterval        string              `json:"updateInterval,omitempty"`
	TraefikAPIURL         string              `json:"traefikApiUrl"`
	InsecureSkipVerifyTLS bool                `json:"insecureSkipVerifyTLS,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		UpdateInterval:        "5m",
		TraefikAPIURL:         "http://localhost:8080",
		Devices:               []UnifiDeviceConfig{},
		InsecureSkipVerifyTLS: false,
	}
}

// UniFiDNS a UniFi DNS plugin.
type UniFiDNS struct {
	next           http.Handler
	name           string
	config         *Config
	unifiClients   map[string]*UniFiClient
	devicePatterns map[string]*regexp.Regexp
	traefikClient  *TraefikClient
	updateInterval time.Duration
	mu             sync.RWMutex
	lastUpdate     time.Time
}

// New created a new UniFi DNS plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	interval, err := time.ParseDuration(config.UpdateInterval)
	if err != nil {
		log.Printf("ERROR: Invalid update interval: %v", err)
		return nil, fmt.Errorf("invalid update interval: %w", err)
	}

	// Initialize UnifiClients and compile patterns
	unifiClients := make(map[string]*UniFiClient)
	devicePatterns := make(map[string]*regexp.Regexp)

	for i, device := range config.Devices {
		if device.Pattern == "" {
			log.Printf("ERROR: Device %d is missing a pattern", i)
			return nil, fmt.Errorf("device %d is missing a pattern", i)
		}

		// Compile the regex pattern
		re, err := regexp.Compile(device.Pattern)
		if err != nil {
			log.Printf("ERROR: Invalid pattern for device %d: %v", i, err)
			return nil, fmt.Errorf("invalid pattern for device %d: %w", i, err)
		}

		// Create a client for this device
		skipVerify := device.InsecureSkipVerifyTLS || config.InsecureSkipVerifyTLS
		client := NewUniFiClient(device.Host, device.Username, device.Password, skipVerify)
		clientID := fmt.Sprintf("device-%d", i)
		unifiClients[clientID] = client
		devicePatterns[clientID] = re
	}

	u := &UniFiDNS{
		next:           next,
		name:           name,
		config:         config,
		unifiClients:   unifiClients,
		devicePatterns: devicePatterns,
		traefikClient:  NewTraefikClient(config.TraefikAPIURL, config.InsecureSkipVerifyTLS),
		updateInterval: interval,
	}

	// Run initial update
	if err := u.updateDNS(); err != nil {
		log.Printf("ERROR: Initial DNS update failed: %v", err)
	}

	// Start the update goroutine
	go u.updateLoop(ctx)
	log.Printf("INFO: Plugin initialized with update interval: %s", interval)

	return u, nil
}

func (u *UniFiDNS) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	u.next.ServeHTTP(rw, req)
	log.Printf("INFO: Served HTTP request: %s %s", req.Method, req.URL.Path)
}

func (u *UniFiDNS) updateLoop(ctx context.Context) {
	log.Printf("INFO: Starting DNS update loop with interval: %s", u.updateInterval)
	ticker := time.NewTicker(u.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := u.updateDNS(); err != nil {
				log.Printf("ERROR: DNS update failed: %v", err)
			}
		case <-ctx.Done():
			log.Printf("INFO: Stopping DNS update loop")
			return
		}
	}
}

// findMatchingClient returns the unifi client that matches the given hostname
func (u *UniFiDNS) findMatchingClient(hostname string) (*UniFiClient, bool) {
	for clientID, pattern := range u.devicePatterns {
		if pattern.MatchString(hostname) {
			log.Printf("INFO: Found matching client for hostname: %s", hostname)
			return u.unifiClients[clientID], true
		}
	}
	return nil, false
}

func (u *UniFiDNS) updateDNS() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	log.Printf("INFO: Starting DNS update cycle")

	// Get the local IP address
	localIP, err := getLocalIP()
	if err != nil {
		log.Printf("ERROR: Failed to get local IP: %v", err)
		return fmt.Errorf("failed to get local IP: %w", err)
	}
	log.Printf("INFO: Using local IP: %s", localIP)

	// Get current Traefik routers from the API
	routers, err := u.traefikClient.GetRouters()
	if err != nil {
		log.Printf("ERROR: Failed to get Traefik routers: %v", err)
		return fmt.Errorf("failed to get Traefik routers: %w", err)
	}
	log.Printf("INFO: Retrieved %d routers from Traefik API", len(routers))

	// Update DNS records for each router
	for _, router := range routers {
		if router.Rule == "" {
			continue
		}

		// Extract hostname from rule (assuming format "Host(`example.com`)"))
		hostname := extractHostname(router.Rule)
		if hostname == "" {
			continue
		}

		log.Printf("INFO: Processing hostname: %s", hostname)

		// Find the matching UniFi client for this hostname
		client, found := u.findMatchingClient(hostname)
		if !found {
			log.Printf("WARN: No matching UniFi device found for hostname: %s", hostname)
			continue
		}

		// Update DNS record
		if err := client.updateDNSRecord(hostname, localIP); err != nil {
			log.Printf("ERROR: Failed to update DNS record for %s: %v", hostname, err)
			continue
		}
		log.Printf("INFO: Successfully updated DNS record for %s", hostname)
	}

	u.lastUpdate = time.Now()
	log.Printf("INFO: Completed DNS update cycle. Last update: %s", u.lastUpdate.Format(time.RFC3339))
	return nil
}

func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no suitable IP address found")
}
