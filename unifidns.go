package trafikunifidns

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// Config the plugin configuration.
type Config struct {
	UDMProHost     string `json:"udmProHost"`
	UDMProUsername string `json:"udmProUsername"`
	UDMProPassword string `json:"udmProPassword"`
	UpdateInterval string `json:"updateInterval,omitempty"`
	TraefikAPIURL  string `json:"traefikApiUrl"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		UpdateInterval: "5m",
		TraefikAPIURL:  "http://localhost:8080",
	}
}

// UniFiDNS a UniFi DNS plugin.
type UniFiDNS struct {
	next           http.Handler
	name           string
	config         *Config
	unifiClient    *UniFiClient
	traefikClient  *TraefikClient
	updateInterval time.Duration
	mu             sync.RWMutex
	lastUpdate     time.Time
}

// New created a new UniFi DNS plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	interval, err := time.ParseDuration(config.UpdateInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid update interval: %w", err)
	}

	u := &UniFiDNS{
		next:           next,
		name:           name,
		config:         config,
		unifiClient:    NewUniFiClient(config.UDMProHost, config.UDMProUsername, config.UDMProPassword),
		traefikClient:  NewTraefikClient(config.TraefikAPIURL),
		updateInterval: interval,
	}

	// Start the update goroutine
	go u.updateLoop(ctx)

	return u, nil
}

func (u *UniFiDNS) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	u.next.ServeHTTP(rw, req)
}

func (u *UniFiDNS) updateLoop(ctx context.Context) {
	ticker := time.NewTicker(u.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := u.updateDNS(); err != nil {
				fmt.Printf("Error updating DNS: %v\n", err)
			}
		}
	}
}

func (u *UniFiDNS) updateDNS() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	// Get the local IP address
	localIP, err := getLocalIP()
	if err != nil {
		return fmt.Errorf("failed to get local IP: %w", err)
	}

	// Get current Traefik routers from the API
	routers, err := u.traefikClient.GetRouters()
	if err != nil {
		return fmt.Errorf("failed to get Traefik routers: %w", err)
	}

	// Update DNS records for each router
	for _, router := range routers {
		if router.Rule == "" {
			continue
		}

		// Extract hostname from rule (assuming format "Host(`example.com`)")
		hostname := extractHostname(router.Rule)
		if hostname == "" {
			continue
		}

		// Update DNS record
		if err := u.unifiClient.updateDNSRecord(hostname, localIP); err != nil {
			fmt.Printf("Failed to update DNS record for %s: %v\n", hostname, err)
			continue
		}
	}

	u.lastUpdate = time.Now()
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
