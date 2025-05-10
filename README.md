# Traefik UniFi DNS Plugin

[![CI](https://github.com/horknfbr/traefikunifidns/actions/workflows/testcover.yml/badge.svg)](https://github.com/horknfbr/traefikunifidns/actions/workflows/testcover.yml) [![Go Linter](https://github.com/horknfbr/traefikunifidns/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/horknfbr/traefikunifidns/actions/workflows/golangci-lint.yml)

![Traefik UniFi DNS Plugin](/.assets/icon.png)

This Traefik plugin automatically updates DNS records on UniFi devices based on the routers configured in Traefik and domain name patterns.

## Features

- Automatically detects hostnames from Traefik router rules
- Updates DNS records on multiple UniFi devices based on hostname patterns
- Only updates DNS records when IP address has changed
- Configurable update interval
- Secure authentication with UniFi devices using username and password
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
    traefikunifidns:
      plugin:
        traefikunifidns:
          devices:
            - host: "192.168.1.1"
              username: "admin"
              password: "your-password"
              pattern: ".*\\.example\\.com"
              insecureSkipVerifyTLS: true  # For devices with self-signed certificates
            - host: "192.168.1.2"
              username: "admin"
              password: "your-password"
              pattern: ".*\\.domain\\.com"
          updateInterval: "5m"
          traefikApiUrl: "http://localhost:8080"
          insecureSkipVerifyTLS: false  # Global setting for all connections
```

### Configuration Options

- `devices`: Array of UniFi device configurations:
  - `host`: The hostname or IP address of your UniFi device
  - `username`: Username for UniFi authentication (typically "admin")
  - `password`: Password for UniFi authentication
  - `pattern`: Regular expression to match hostnames to this device (e.g., ".*\\.example\\.com")
  - `insecureSkipVerifyTLS`: (Optional) Skip TLS certificate verification for this device (useful for self-signed certificates). Defaults to `false`
- `updateInterval`: How often to check for and update DNS records (default: "5m")
- `traefikApiUrl`: URL of the Traefik API (default: `http://localhost:8080`)
- `insecureSkipVerifyTLS`: (Optional) Skip TLS certificate verification for all connections (useful for self-signed certificates). Defaults to `false`

### Authentication

The plugin uses username and password authentication to connect to your UniFi devices. This is the standard authentication method supported by the UniFi API.

Note: Store your credentials securely using environment variables or secrets management. Never commit passwords to version control.

## How it Works

The plugin performs an immediate DNS update when it starts up, ensuring your DNS records are current right away. After the initial update, it continues with regular interval-based updates based on your configuration.

The plugin checks all Traefik routers for Host rules, extracts the domain names, and compares them against the configured regex patterns. When a domain matches a pattern, the plugin checks if the DNS record needs to be updated by comparing the current IP with the existing record. Updates only occur when:

1. The plugin starts up (immediate update)
2. A new domain is detected that matches a device pattern
3. An existing domain's IP address has changed
4. The domain exists but doesn't have a DNS record yet

This ensures minimal API calls to your UniFi devices and prevents unnecessary updates.

For example, with the configuration above:

- `test.example.com` would be checked against device at 192.168.1.1 and updated only if needed
- `blog.domain.com` would be checked against device at 192.168.1.2 and updated only if needed
- Domains that don't match any pattern will be logged but not updated

The plugin logs all actions, including:

- When a record is checked but doesn't need updating
- When a record is updated with a new IP
- Any errors that occur during the process
- Initial update status on startup

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

- Store credentials securely using environment variables or secrets management
- Use dedicated service accounts with minimal required permissions for each device
- Ensure proper network security between Traefik and UniFi devices
- Consider using different credentials for each device
- Regularly rotate passwords for service accounts

## License

[MIT License](LICENSE)
