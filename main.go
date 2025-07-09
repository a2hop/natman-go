package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"natman/config"
	"natman/link"
	configmaker "natman/worker/config-maker"
	natmanager "natman/worker/nat-manager"
	netmapmanager "natman/worker/netmap-manager"
	radvdmanager "natman/worker/radvd-manager"
)

// Global debug flag
var Debug bool = false

// SetDebug sets the debug mode for verbose output
func SetDebug(debug bool) {
	Debug = debug

	// Also set debug for component managers
	netmapmanager.SetDebug(debug)
}

// DebugPrint prints a message if debug mode is enabled
func DebugPrint(format string, args ...interface{}) {
	if Debug {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}

func main() {
	// Parse command line arguments manually to handle flags after commands
	var configPath string = "/etc/natman/config.yaml" // default
	var slim bool = false                             // default
	var quiet bool = false                            // default
	var debug bool = false                            // default
	var command string

	// Parse arguments manually
	args := os.Args[1:]
	var nonFlagArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if strings.HasPrefix(arg, "--c=") {
			configPath = strings.TrimPrefix(arg, "--c=")
		} else if arg == "--c" || arg == "-c" {
			if i+1 < len(args) {
				configPath = args[i+1]
				i++ // skip next arg as it's the value
			}
		} else if arg == "--slim" {
			slim = true
		} else if arg == "--quiet" || arg == "-q" {
			quiet = true
		} else if arg == "--debug" || arg == "-d" {
			debug = true
		} else if arg == "-h" || arg == "--help" {
			showHelp()
			return
		} else if strings.HasPrefix(arg, "-") {
			// Handle other flags that might be added later
			continue
		} else {
			// Non-flag argument (likely the command)
			nonFlagArgs = append(nonFlagArgs, arg)
		}
	}

	// Set global debug flag
	SetDebug(debug)

	// Get command from non-flag arguments
	if len(nonFlagArgs) > 0 {
		command = nonFlagArgs[0]
	}

	// Handle special commands
	switch command {
	case "config-capture":
		if err := runConfigCapture(configPath, slim); err != nil {
			fmt.Printf("Error in config capture: %v\n", err)
			os.Exit(1)
		}
		return
	case "status":
		if err := runStatus(configPath); err != nil {
			fmt.Printf("Error getting status: %v\n", err)
			os.Exit(1)
		}
		return
	case "validate":
		if err := runValidate(configPath); err != nil {
			fmt.Printf("Validation failed: %v\n", err)
			os.Exit(1)
		}
		return
	case "show-netmap":
		if err := runShowNetmap(); err != nil {
			fmt.Printf("Error showing netmap rules: %v\n", err)
			os.Exit(1)
		}
		return
	case "show-nat":
		if err := runShowNat(); err != nil {
			fmt.Printf("Error showing NAT rules: %v\n", err)
			os.Exit(1)
		}
		return
	case "capture-rules":
		if err := runCaptureRules(); err != nil {
			fmt.Printf("Error capturing rules: %v\n", err)
			os.Exit(1)
		}
		return
	case "show-radvd":
		if err := runShowRadvd(); err != nil {
			fmt.Printf("Error showing radvd settings: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Normal flow: parse config and apply configuration
	if err := runNormalFlow(configPath, quiet); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Println("natman-go - Modern NAT manager for Linux")
	fmt.Println("")
	fmt.Println("USAGE:")
	fmt.Println("    natman [OPTIONS] [COMMAND]")
	fmt.Println("")
	fmt.Println("OPTIONS:")
	fmt.Println("    -c, --c=PATH     Configuration file path (default: /etc/natman/config.yaml)")
	fmt.Println("    -q, --quiet      Suppress non-essential output")
	fmt.Println("    -d, --debug      Enable debug output")
	fmt.Println("    -h, --help       Show this help message")
	fmt.Println("")
	fmt.Println("COMMANDS:")
	fmt.Println("    config-capture   Scan system and generate configuration file")
	fmt.Println("                     Use --slim to generate minimal configuration")
	fmt.Println("    status           Show current system status and configuration")
	fmt.Println("    validate         Validate configuration file")
	fmt.Println("    show-netmap      Display current NETMAP rules")
	fmt.Println("    show-nat         Display current NAT rules")
	fmt.Println("    show-radvd       Display current radvd settings and routes")
	fmt.Println("    capture-rules    Capture and display all current rules")
	fmt.Println("")
	fmt.Println("EXAMPLES:")
	fmt.Println("    natman                                    # Apply configuration")
	fmt.Println("    natman config-capture                    # Generate config from system")
	fmt.Println("    natman config-capture --slim             # Generate minimal config")
	fmt.Println("    natman -c /path/to/config.yaml validate  # Validate custom config")
	fmt.Println("    natman status --quiet                    # Check status quietly")
}

func runConfigCapture(configPath string, slim bool) error {
	fmt.Println("Running config capture...")

	// Scan system and generate config
	var configContent string
	var err error

	if slim {
		configContent, err = configmaker.ScanSystemAndGenerateConfigSlim(true)
	} else {
		configContent, err = configmaker.ScanSystemAndGenerateConfig()
	}

	if err != nil {
		return fmt.Errorf("failed to scan system: %v", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	// Write config to file
	if err := configmaker.WriteConfigToFile(configContent, configPath); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	fmt.Printf("Configuration written to %s\n", configPath)
	return nil
}

func runStatus(configPath string) error {
	fmt.Println("System Status:")
	fmt.Println("==============")

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("Config file not found: %s\n", configPath)
		return nil
	}

	// Parse config
	cfg, err := config.ParseConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	// Build links
	links := link.BuildLinks(cfg)

	// Print netmap status if function exists
	fmt.Println("\nNETMAP Status:")
	if err := netmapmanager.PrintNetmapRules(links); err != nil {
		fmt.Printf("  Error displaying NETMAP rules: %v\n", err)
	}

	// Check radvd status
	fmt.Println("\nRadvd Service Status:")
	active, err := radvdmanager.GetRadvdStatus()
	if err != nil {
		fmt.Printf("  Error checking radvd status: %v\n", err)
	} else {
		if active {
			fmt.Println("  Status: Active")
		} else {
			fmt.Println("  Status: Inactive")
		}
	}

	return nil
}

func runValidate(configPath string) error {
	fmt.Println("Validating configuration...")

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", configPath)
	}

	// Parse config
	_, err := config.ParseConfig(configPath)
	if err != nil {
		return fmt.Errorf("config validation failed: %v", err)
	}

	// Validate radvd config if it exists
	if err := radvdmanager.ValidateRadvdConfig(); err != nil {
		fmt.Printf("Warning: radvd config validation failed: %v\n", err)
	}

	fmt.Println("Configuration is valid")
	return nil
}

func runNormalFlow(configPath string, quiet bool) error {
	if !quiet {
		fmt.Printf("Starting natman with config: %s\n", configPath)
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", configPath)
	}

	// Parse config file
	DebugPrint("Parsing config file: %s", configPath)
	cfg, err := config.ParseConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	// Validate config has links
	if len(cfg.Network.Links) == 0 {
		return fmt.Errorf("no links configured in config file")
	}

	if !quiet {
		fmt.Printf("Loaded configuration with %d links\n", len(cfg.Network.Links))
	}
	DebugPrint("Configuration loaded with %d links", len(cfg.Network.Links))

	// Build the link model
	links := link.BuildLinks(cfg)
	if len(links) == 0 {
		return fmt.Errorf("no valid links found after building link models")
	}
	if !quiet {
		fmt.Println("Built link models")
	}
	DebugPrint("Built link models with %d links", len(links))

	// Dump link configuration in debug mode
	if Debug {
		for name, linkObj := range links {
			DebugPrint("Link: %s", name)
			for netmapName, netmapObj := range linkObj.Netmap6 {
				DebugPrint("  Netmap6: %s (enabled: %t)", netmapName, netmapObj.Enabled)
				DebugPrint("    PfxPub: %s", netmapObj.PfxPub)
				DebugPrint("    PfxPriv: %s", netmapObj.PfxPriv)
				DebugPrint("    Maps: %d entries", len(netmapObj.Maps))
				for i, mapping := range netmapObj.Maps {
					DebugPrint("      Map[%d]: Public=%s, Private=%s", i, mapping.Public, mapping.Private)
				}
			}
		}
	}

	// Run natmaker (NAT44/NAT66 configuration)
	if !quiet {
		fmt.Println("Applying NAT rules...")
	}
	DebugPrint("Applying NAT rules")
	if err := natmanager.ApplyNatRules(links); err != nil {
		return fmt.Errorf("failed to apply NAT rules: %v", err)
	}
	if !quiet {
		fmt.Println("NAT rules applied successfully")
	}
	DebugPrint("NAT rules applied successfully")

	// Run netmapmaker (IPv6 network mapping)
	if !quiet {
		fmt.Println("Applying netmap rules...")
	}
	DebugPrint("Applying netmap rules")
	if err := netmapmanager.ApplyNetmapRules(links); err != nil {
		return fmt.Errorf("failed to apply netmap rules: %v", err)
	}
	if !quiet {
		fmt.Println("Netmap rules applied successfully")
	}
	DebugPrint("Netmap rules applied successfully")

	// Run radvdmaker (Router Advertisement configuration)
	if !quiet {
		fmt.Println("Updating radvd configuration...")
	}
	DebugPrint("Updating radvd configuration")
	if err := radvdmanager.CreateRadvdConfig(links); err != nil {
		return fmt.Errorf("failed to update radvd config: %v", err)
	}
	if !quiet {
		fmt.Println("Radvd configuration updated successfully")
	}
	DebugPrint("Radvd configuration updated successfully")

	if !quiet {
		fmt.Println("All configurations applied successfully!")
	}
	DebugPrint("All configurations applied successfully")
	return nil
}

func runShowNetmap() error {
	fmt.Println("Showing current netmap rules from system...")
	return netmapmanager.PrintCurrentNetmapRules()
}

func runShowNat() error {
	fmt.Println("Showing current NAT rules from system...")
	return natmanager.PrintCurrentNatRules()
}

func runCaptureRules() error {
	fmt.Println("Capturing current rules from system...")

	// Capture netmap rules
	fmt.Println("\nCapturing NETMAP rules...")
	netmapRules, err := netmapmanager.CaptureNetmapRulesFromSystem()
	if err != nil {
		return fmt.Errorf("failed to capture netmap rules: %v", err)
	}

	fmt.Println("NETMAP rules by interface:")
	for iface, rules := range netmapRules {
		fmt.Printf("  Interface %s: %d rules\n", iface, len(rules))
		for _, rule := range rules {
			fmt.Printf("    %s\n", rule)
		}
	}

	// Capture NAT rules
	fmt.Println("\nCapturing NAT rules...")
	natRules, err := natmanager.CaptureNatRulesFromSystem()
	if err != nil {
		return fmt.Errorf("failed to capture NAT rules: %v", err)
	}

	fmt.Println("NAT rules:")
	if ipv4Rules, ok := natRules["ipv4"].(map[string][]string); ok {
		fmt.Println("  IPv4 rules by interface:")
		for iface, rules := range ipv4Rules {
			fmt.Printf("    Interface %s: %d rules\n", iface, len(rules))
			for _, rule := range rules {
				fmt.Printf("      %s\n", rule)
			}
		}
	}

	if ipv6Rules, ok := natRules["ipv6"].(map[string][]string); ok {
		fmt.Println("  IPv6 rules by interface:")
		for iface, rules := range ipv6Rules {
			fmt.Printf("    Interface %s: %d rules\n", iface, len(rules))
			for _, rule := range rules {
				fmt.Printf("      %s\n", rule)
			}
		}
	}

	return nil
}

func runShowRadvd() error {
	fmt.Println("Showing current radvd settings with focus on routes...")

	// Check if radvd service is running
	active, err := radvdmanager.GetRadvdStatus()
	if err != nil {
		return fmt.Errorf("failed to check radvd status: %v", err)
	}

	fmt.Printf("Radvd Service Status: ")
	if active {
		fmt.Println("✓ Active")
	} else {
		fmt.Println("✗ Inactive")
	}

	// Read and display radvd configuration file
	configPath := "/etc/radvd.conf"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("Radvd config file not found: %s\n", configPath)
		return nil
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read radvd config: %v", err)
	}

	fmt.Printf("\nRadvd Configuration Analysis (%s):\n", configPath)
	fmt.Println(strings.Repeat("=", 50))

	// Parse configuration and extract meaningful information
	lines := strings.Split(string(content), "\n")
	var currentInterface string
	var interfaceCount int
	var prefixCount int
	var routeCount int

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse interface blocks
		if strings.Contains(line, "interface") && strings.Contains(line, "{") {
			interfaceCount++
			// Extract interface name
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentInterface = strings.TrimSuffix(parts[1], "{")
			}
			fmt.Printf("\n📡 Interface: %s\n", currentInterface)

			// Look ahead for configuration in this interface block
			j := i + 1
			braceCount := 1
			var advSendAdvert, advDefaultRoute bool
			var minInterval, maxInterval, defaultLifetime string

			for j < len(lines) && braceCount > 0 {
				innerLine := strings.TrimSpace(lines[j])
				if strings.Contains(innerLine, "{") {
					braceCount++
				}
				if strings.Contains(innerLine, "}") {
					braceCount--
				}

				if strings.Contains(innerLine, "AdvSendAdvert") {
					if strings.Contains(innerLine, "on") {
						advSendAdvert = true
					}
				}
				if strings.Contains(innerLine, "AdvDefaultRoute") {
					if strings.Contains(innerLine, "on") {
						advDefaultRoute = true
					}
				}
				if strings.Contains(innerLine, "MinRtrAdvInterval") {
					parts := strings.Fields(innerLine)
					if len(parts) >= 2 {
						minInterval = strings.TrimSuffix(parts[1], ";")
					}
				}
				if strings.Contains(innerLine, "MaxRtrAdvInterval") {
					parts := strings.Fields(innerLine)
					if len(parts) >= 2 {
						maxInterval = strings.TrimSuffix(parts[1], ";")
					}
				}
				if strings.Contains(innerLine, "AdvDefaultLifetime") {
					parts := strings.Fields(innerLine)
					if len(parts) >= 2 {
						defaultLifetime = strings.TrimSuffix(parts[1], ";")
					}
				}
				j++
			}

			// Display interface summary
			fmt.Printf("   ├─ Router Advertisements: %s\n", boolToStatus(advSendAdvert))
			fmt.Printf("   ├─ Default Route Advertisement: %s\n", boolToStatus(advDefaultRoute))
			if minInterval != "" && maxInterval != "" {
				fmt.Printf("   ├─ Advertisement Interval: %s-%s seconds\n", minInterval, maxInterval)
			}
			if defaultLifetime != "" {
				fmt.Printf("   ├─ Default Route Lifetime: %s seconds\n", defaultLifetime)
			}
		}

		// Parse prefix blocks
		if strings.Contains(line, "prefix") && strings.Contains(line, "{") {
			prefixCount++
			// Extract prefix
			parts := strings.Fields(line)
			var prefix string
			if len(parts) >= 2 {
				prefix = strings.TrimSuffix(parts[1], "{")
			}
			fmt.Printf("   ├─ 🌐 Prefix: %s\n", prefix)

			// Look ahead for prefix configuration
			j := i + 1
			braceCount := 1
			var advOnLink, advAutonomous, advRouterAddr bool
			var validLifetime, preferredLifetime string

			for j < len(lines) && braceCount > 0 {
				innerLine := strings.TrimSpace(lines[j])
				if strings.Contains(innerLine, "{") {
					braceCount++
				}
				if strings.Contains(innerLine, "}") {
					braceCount--
				}

				if strings.Contains(innerLine, "AdvOnLink") {
					advOnLink = strings.Contains(innerLine, "on")
				}
				if strings.Contains(innerLine, "AdvAutonomous") {
					advAutonomous = strings.Contains(innerLine, "on")
				}
				if strings.Contains(innerLine, "AdvRouterAddr") {
					advRouterAddr = strings.Contains(innerLine, "on")
				}
				if strings.Contains(innerLine, "AdvValidLifetime") {
					parts := strings.Fields(innerLine)
					if len(parts) >= 2 {
						validLifetime = strings.TrimSuffix(parts[1], ";")
					}
				}
				if strings.Contains(innerLine, "AdvPreferredLifetime") {
					parts := strings.Fields(innerLine)
					if len(parts) >= 2 {
						preferredLifetime = strings.TrimSuffix(parts[1], ";")
					}
				}
				j++
			}

			fmt.Printf("   │  ├─ On-Link: %s\n", boolToStatus(advOnLink))
			fmt.Printf("   │  ├─ Autonomous Config: %s\n", boolToStatus(advAutonomous))
			fmt.Printf("   │  ├─ Router Address: %s\n", boolToStatus(advRouterAddr))
			if validLifetime != "" {
				fmt.Printf("   │  ├─ Valid Lifetime: %s\n", validLifetime)
			}
			if preferredLifetime != "" {
				fmt.Printf("   │  └─ Preferred Lifetime: %s\n", preferredLifetime)
			}
		}

		// Parse route blocks
		if strings.Contains(line, "route") && strings.Contains(line, "{") {
			routeCount++
			// Extract route
			parts := strings.Fields(line)
			var route string
			if len(parts) >= 2 {
				route = strings.TrimSuffix(parts[1], "{")
			}
			fmt.Printf("   ├─ 🛣️  Route: %s\n", route)

			// Look ahead for route configuration
			j := i + 1
			braceCount := 1
			var routeLifetime, routePreference string

			for j < len(lines) && braceCount > 0 {
				innerLine := strings.TrimSpace(lines[j])
				if strings.Contains(innerLine, "{") {
					braceCount++
				}
				if strings.Contains(innerLine, "}") {
					braceCount--
				}

				if strings.Contains(innerLine, "AdvRouteLifetime") {
					parts := strings.Fields(innerLine)
					if len(parts) >= 2 {
						routeLifetime = strings.TrimSuffix(parts[1], ";")
					}
				}
				if strings.Contains(innerLine, "AdvRoutePreference") {
					parts := strings.Fields(innerLine)
					if len(parts) >= 2 {
						routePreference = strings.TrimSuffix(parts[1], ";")
					}
				}
				j++
			}

			if routeLifetime != "" {
				fmt.Printf("   │  ├─ Route Lifetime: %s\n", routeLifetime)
			}
			if routePreference != "" {
				fmt.Printf("   │  └─ Route Preference: %s\n", routePreference)
			}
		}
	}

	// Summary
	fmt.Printf("\n📊 Summary:\n")
	fmt.Printf("   ├─ Interfaces configured: %d\n", interfaceCount)
	fmt.Printf("   ├─ IPv6 prefixes announced: %d\n", prefixCount)
	fmt.Printf("   └─ Additional routes announced: %d\n", routeCount)

	return nil
}

func boolToStatus(b bool) string {
	if b {
		return "✓ Enabled"
	}
	return "✗ Disabled"
}
