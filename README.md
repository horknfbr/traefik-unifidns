# Traefik UniFi DNS Plugin

[![CI](https://github.com/horknfbr/traefikunifidns/actions/workflows/testcover.yml/badge.svg)](https://github.com/horknfbr/traefikunifidns/actions/workflows/testcover.yml) [![Go Linter](https://github.com/horknfbr/traefikunifidns/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/horknfbr/traefikunifidns/actions/workflows/golangci-lint.yml)

![Traefik UniFi DNS Plugin](/.assets/icon.png)

This Traefik plugin automatically updates DNS records on UniFi devices based on the routers configured in Traefik and domain name patterns.

## Features

- Automatically detects hostnames from Traefik router rules
- Updates DNS records on multiple UniFi devices based on hostname patterns
- Configurable update interval
- Secure authentication with UniFi devices
- Support for regex pattern matching to route hostnames to appropriate devices

## Configuration

### Static Configuration

```yaml
experimental:
  plugins:
    traefikunifidns:
      moduleName: github.com/horknfbr/traefikunifidns
```

### Dynamic Configuration

```yaml
http:
  middlewares:
    unifidns:
      plugin:
        unifidns:
          devices:
            - host: "192.168.1.1"
              username: "admin"
              password: "your-password"
              pattern: ".*\\.example\\.com"
            - host: "192.168.1.2"
              username: "admin"
              password: "your-password"
              pattern: ".*\\.domain\\.com"
          updateInterval: "5m"
          traefikApiUrl: "http://localhost:8080"
```

### Configuration Options

- `devices`: Array of UniFi device configurations:
  - `host`: The hostname or IP address of your UniFi device
  - `username`: Username for UniFi authentication
  - `password`: Password for UniFi authentication
  - `pattern`: Regular expression to match hostnames to this device (e.g., ".\*\\.example\\.com")
- `updateInterval`: How often to check for and update DNS records (default: "5m")
- `traefikApiUrl`: URL of the Traefik API (default: `http://localhost:8080`)

## How it Works

The plugin checks all Traefik routers for Host rules, extracts the domain names, and compares them against the configured regex patterns. When a domain matches a pattern, the plugin updates the DNS record on the corresponding UniFi device.

For example, with the configuration above:

- `test.example.com` would be updated on device at 192.168.1.1
- `blog.domain.com` would be updated on device at 192.168.1.2
- Domains that don't match any pattern will be logged but not updated

## Usage

1. Install the plugin in your Traefik configuration
2. Configure the middleware with your UniFi credentials and domain patterns
3. Apply the middleware to your routers

Example router configuration:

```yaml
http:
  routers:
    my-service:
      rule: "Host(`example.com`)"
      service: my-service
      middlewares:
        - unifidns
```

## Security Considerations

- Store sensitive credentials (username/password) securely
- Consider using environment variables or secrets management
- Ensure proper network security between Traefik and UniFi devices

## License

[MIT License](LICENSE)
