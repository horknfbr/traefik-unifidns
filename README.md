# Traefik UniFi DNS Plugin

This Traefik plugin automatically updates DNS records on a UniFi Dream Machine Pro based on the routers configured in Traefik.

## Features

- Automatically detects hostnames from Traefik router rules
- Updates DNS records on UniFi Dream Machine Pro
- Configurable update interval
- Secure authentication with UniFi UDM Pro

## Configuration

### Static Configuration

```yaml
experimental:
  plugins:
    unifidns:
      moduleName: github.com/horknfbr/trafik-unifidns
```

### Dynamic Configuration

```yaml
http:
  middlewares:
    unifidns:
      plugin:
        unifidns:
          udmProHost: "192.168.1.1"
          udmProUsername: "admin"
          udmProPassword: "your-password"
          updateInterval: "5m"
          traefikApiUrl: "http://localhost:8080"
```

### Configuration Options

- `udmProHost`: The hostname or IP address of your UniFi Dream Machine Pro
- `udmProUsername`: Username for UniFi authentication
- `udmProPassword`: Password for UniFi authentication
- `updateInterval`: How often to check for and update DNS records (default: "5m")
- `traefikApiUrl`: URL of the Traefik API (default: "http://localhost:8080")

## Usage

1. Install the plugin in your Traefik configuration
2. Configure the middleware with your UniFi credentials
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
- Ensure proper network security between Traefik and UDM Pro

## License

MIT License
