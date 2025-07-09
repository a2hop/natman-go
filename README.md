# natman-go

Modern NAT manager for Linux written in Go. Natman-go provides automated configuration and management of IPv4/IPv6 NAT, network mapping (NETMAP), and router advertisement (RADV) services.

## Features

- **IPv6 Network Mapping (NETMAP)**: Automated 1:1 IPv6 address translation using ip6tables NETMAP target
- **NAT44/NAT66**: IPv4 and IPv6 masquerading with MSS clamping support
- **Router Advertisement (RADV)**: Automated radvd configuration management
- **System Discovery**: Scan existing network configuration and generate natman config
- **Configuration Validation**: Validate configuration files and system state
- **Rule Management**: Intelligent rule addition/removal without duplicates

## Installation

### Prerequisites

- Linux system with iptables/ip6tables support
- NETMAP target support in kernel/iptables (for IPv6 network mapping)
- radvd package (for router advertisement functionality)
- Root privileges for network configuration

### Build from Source

```bash
git clone https://github.com/a2hop/natman-go.git
cd natman-go
go build -o natman .
sudo cp natman /usr/local/bin/
```

### Create Configuration Directory

```bash
sudo mkdir -p /etc/natman
```

## Quick Start

### 1. Generate Configuration from System

Scan your current network setup and generate a configuration file:

```bash
# Generate full configuration
sudo natman config-capture

# Generate minimal configuration (only enabled features)
sudo natman config-capture --slim

# Custom config path
sudo natman config-capture -c /path/to/config.yaml
```

### 2. Validate Configuration

```bash
sudo natman validate
```

### 3. Apply Configuration

```bash
sudo natman
```

## Usage

### Command Line Options

```bash
natman [options] [command]
```

#### Global Options

- `-c, --c=PATH`: Configuration file path (default: `/etc/natman/config.yaml`)
- `--quiet, -q`: Suppress non-essential output
- `--debug, -d`: Enable debug output
- `--slim`: Generate minimal configuration (config-capture only)
- `-h, --help`: Show help message

#### Commands

- **No command**: Apply configuration (default behavior)
- `config-capture`: Scan system and generate configuration file
- `status`: Show current system status and configuration
- `validate`: Validate configuration file
- `show-netmap`: Display current NETMAP rules
- `show-nat`: Display current NAT rules
- `capture-rules`: Capture and display all current rules

### Examples

```bash
# Apply configuration with custom path
sudo natman -c /home/user/my-config.yaml

# Generate configuration and save to custom location
sudo natman config-capture -c /home/user/natman.yaml --slim

# Check status quietly
sudo natman status --quiet

# Show current NAT rules
sudo natman show-nat

# Enable debug mode
sudo natman --debug
```

## Configuration

### Configuration File Structure

The configuration file uses YAML format:

```yaml
network:
  links:
    eth0:
      netmap6:
        set1:
          enabled: true
          pfx-pub: "2001:db8:1::"
          pfx-priv: "fd00:1::"
          maps:
            - pair: ["100", "100"]
            - pair: ["101", "101", "high", 7200]
      
      nat66:
        enabled: true
        mss-clamping: true
        mss: 1440
        origins:
          - "fd00::/16"
      
      nat44:
        enabled: true
        mss-clamping: true
        mss: 1440
        origins:
          - "192.168.1.0/24"
      
      radv:
        enabled: true
        adv-interval: [30, 60]
        lifetime: 180
        dhcp: false
        prefixes:
          - prefix: "2001:db8:1::/64"
            on-link: true
            auto: true
            adv-addr: false
            lifetime: [1800, 900]
        routes:
          - route: ["::/0", "medium", 3600]
```

### Configuration Sections

#### Network Mapping (netmap6)

Maps IPv6 addresses 1:1 using NETMAP target:

```yaml
netmap6:
  set_name:
    enabled: true
    pfx-pub: "2001:db8:1::"     # Public prefix (optional)
    pfx-priv: "fd00:1::"        # Private prefix (optional)
    maps:
      - pair: ["public_addr", "private_addr"]
      - pair: ["public_addr", "private_addr", "preference", lifetime]
```

The `pair` array supports two formats:
- `[public, private]`: Basic mapping
- `[public, private, preference, lifetime]`: With router advertisement settings

#### NAT Configuration

IPv4 and IPv6 masquerading:

```yaml
nat44:
  enabled: true
  mss-clamping: true          # Enable MSS clamping
  mss: 1440                   # MSS value
  origins:                    # Source networks for policy routing
    - "192.168.0.0/16"

nat66:
  enabled: true
  mss-clamping: false
  mss: 1440
  origins:
    - "fd00::/16"
```

#### Router Advertisement (radv)

Configures radvd for IPv6 router advertisements:

```yaml
radv:
  enabled: true
  adv-interval: [30, 60]      # [min, max] advertisement interval (seconds)
  lifetime: 180               # Default route lifetime (seconds)
  dhcp: false                 # Enable managed/other config flags
  
  prefixes:
    - prefix: "2001:db8:1::/64"
      on-link: true
      auto: true              # Autonomous configuration
      adv-addr: false         # Advertise router address
      lifetime: [1800, 900]   # [valid, preferred] lifetime
  
  routes:
    - route: ["::/0", "medium", 3600]  # [prefix, preference, lifetime]
  
  include:                    # Include external config files
    - "/etc/radvd.conf.d/custom.conf"
```

Route preference can be: `"high"`, `"medium"`, or `"low"`

## System Integration

### Systemd Service

Create `/etc/systemd/system/natman.service`:

```ini
[Unit]
Description=Natman Network Manager
After=network.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/natman --quiet
RemainAfterExit=yes
User=root

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable natman
sudo systemctl start natman
```

### Network Interface Integration

For automatic configuration on interface changes, create `/etc/systemd/system/natman@.service`:

```ini
[Unit]
Description=Natman for interface %i
After=network.target
BindsTo=sys-subsystem-net-devices-%i.device

[Service]
Type=oneshot
ExecStart=/usr/local/bin/natman --quiet
RemainAfterExit=yes
User=root

[Install]
WantedBy=sys-subsystem-net-devices-%i.device
```

## Troubleshooting

### Check System Status

```bash
sudo natman status
```

### Validate Configuration

```bash
sudo natman validate
```

### View Current Rules

```bash
# Show all NAT rules
sudo natman show-nat

# Show all NETMAP rules
sudo natman show-netmap

# Capture all current rules
sudo natman capture-rules
```

### Debug Mode

Enable debug output for detailed troubleshooting:

```bash
sudo natman --debug
```

### Common Issues

#### NETMAP Target Not Available

Ensure your kernel and iptables support the NETMAP target:

```bash
# Check kernel module
sudo modprobe ip6table_nat
sudo lsmod | grep ip6table_nat

# Test NETMAP availability
sudo ip6tables -t nat -A OUTPUT -j NETMAP --to ::/0 2>/dev/null && echo "NETMAP available"
```

#### radvd Service Issues

Check radvd configuration and service status:

```bash
# Validate generated radvd config
sudo radvd -c -C /etc/radvd.conf

# Check service status
sudo systemctl status radvd

# View radvd logs
sudo journalctl -u radvd
```

#### Configuration Issues

Use debug mode to see detailed processing:

```bash
# Debug configuration parsing and rule generation
sudo natman --debug

# Validate configuration syntax
sudo natman validate
```

#### Permission Issues

Natman requires root privileges for network configuration:

```bash
sudo natman [command]
```

## Development

### Project Structure

```
natman-go/
├── config/           # Configuration parsing
├── link/            # Network link abstraction
│   ├── netmap6/     # IPv6 network mapping
│   └── radv/        # Router advertisement
├── worker/          # Core functionality modules
│   ├── config-maker/     # System scanning and config generation
│   ├── nat-manager/      # NAT rule management
│   ├── netmap-manager/   # NETMAP rule management
│   └── radvd-manager/    # radvd configuration management
└── main.go          # Main application entry point
```

### Building

```bash
go build -o natman .
```

### Testing

```bash
go test ./...
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Support

- Report issues: [GitHub Issues](https://github.com/your-repo/natman-go/issues)
- Documentation: [Wiki](https://github.com/your-repo/natman-go/wiki)
- Discussions: [GitHub Discussions](https://github.com/your-repo/natman-go/discussions)
