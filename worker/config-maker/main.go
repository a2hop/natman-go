package configmaker

import (
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// It can scan existing setting nin the system and compose the config from them
func ScanSystemAndGenerateConfig() (string, error) {
	return ScanSystemAndGenerateConfigSlim(false)
}

func ScanSystemAndGenerateConfigSlim(slim bool) (string, error) {
	interfaces, err := scanNetworkInterfaces()
	if err != nil {
		return "", err
	}

	routes, err := scanRoutes()
	if err != nil {
		return "", err
	}

	// Scan existing radvd configuration
	radvdConfig, err := scanRadvdConfig()
	if err != nil {
		// Don't fail if radvd config doesn't exist, just continue without it
		radvdConfig = make(map[string]RadvdInterface)
	}

	// Scan existing netmap rules
	netmapRules, err := scanNetmapRules()
	if err != nil {
		// Don't fail if netmap rules can't be scanned, just continue without them
		netmapRules = make(map[string][]NetmapRule)
	}

	// Scan existing NAT66 rules
	nat66Rules, err := scanNat66Rules()
	if err != nil {
		// Don't fail if NAT66 rules can't be scanned, just continue without them
		nat66Rules = make(map[string][]Nat66Rule)
	}

	config := generateConfigYAML(interfaces, routes, radvdConfig, netmapRules, nat66Rules, slim)
	return config, nil
}

func scanNetworkInterfaces() ([]NetworkInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var interfaces []NetworkInterface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		netIface := NetworkInterface{
			Name: iface.Name,
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipnet.IP.To4() != nil {
					netIface.IPv4Addresses = append(netIface.IPv4Addresses, ipnet.String())
				} else {
					netIface.IPv6Addresses = append(netIface.IPv6Addresses, ipnet.String())
				}
			}
		}

		interfaces = append(interfaces, netIface)
	}

	return interfaces, nil
}

func scanRoutes() ([]Route, error) {
	cmd := exec.Command("ip", "route", "show")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var routes []Route
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "default") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				route := Route{
					Destination: "default",
					Gateway:     fields[2],
					Interface:   fields[4],
				}
				routes = append(routes, route)
			}
		}
	}

	return routes, nil
}

func scanNetmapRules() (map[string][]NetmapRule, error) {
	cmd := exec.Command("ip6tables", "-t", "nat", "-L", "-n", "-v")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseNetmapRulesForConfig(string(output)), nil
}

func parseNetmapRulesForConfig(output string) map[string][]NetmapRule {
	rules := make(map[string][]NetmapRule)
	lines := strings.Split(output, "\n")

	var currentChain string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Detect chain headers
		if strings.HasPrefix(line, "Chain ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentChain = parts[1]
			}
			continue
		}

		// Skip header lines and empty lines
		if line == "" || strings.Contains(line, "pkts bytes target") || strings.Contains(line, "prot opt source") {
			continue
		}

		// Parse NETMAP rules
		if strings.Contains(line, "NETMAP") && currentChain != "" {
			rule := parseNetmapRuleForConfig(line, currentChain)
			if rule.Interface != "" {
				rules[rule.Interface] = append(rules[rule.Interface], rule)
			}
		}
	}

	return rules
}

func parseNetmapRuleForConfig(line, chain string) NetmapRule {
	fields := strings.Fields(line)
	rule := NetmapRule{
		Chain: chain,
	}

	if len(fields) < 8 {
		return rule
	}

	// ip6tables -L -n -v output format:
	// pkts bytes target prot opt in     out    source               destination         [extra options]
	// Example: 2   160 NETMAP all  --  gtwl   any    anywhere             e2::3:0:25:0:0/96    to:b30::20:20:0:0/96
	//          0   1   2      3    4   5      6      7                    8                    9

	// Skip if this is not a NETMAP rule
	if fields[2] != "NETMAP" {
		return rule
	}

	// Extract interface information from correct positions
	inInterface := fields[5]  // "in" interface
	outInterface := fields[6] // "out" interface

	// Extract source and destination
	if len(fields) > 8 {
		rule.Source = fields[7]
		rule.Destination = fields[8]
	}

	// Determine interface and direction based on chain
	if chain == "PREROUTING" {
		// For PREROUTING, packets come IN on the interface
		if inInterface != "any" && inInterface != "*" && inInterface != "--" {
			rule.Interface = inInterface
			rule.Direction = "PREROUTING"
		}
	} else if chain == "POSTROUTING" {
		// For POSTROUTING, packets go OUT on the interface
		if outInterface != "any" && outInterface != "*" && outInterface != "--" {
			rule.Interface = outInterface
			rule.Direction = "POSTROUTING"
		}
	}

	// Extract the "to:" address
	for _, field := range fields[9:] {
		if strings.HasPrefix(field, "to:") {
			rule.ToAddress = strings.TrimPrefix(field, "to:")
			break
		}
	}

	return rule
}

func scanNat66Rules() (map[string][]Nat66Rule, error) {
	cmd := exec.Command("ip6tables", "-t", "nat", "-L", "-n", "-v")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseNat66RulesForConfig(string(output)), nil
}

func parseNat66RulesForConfig(output string) map[string][]Nat66Rule {
	rules := make(map[string][]Nat66Rule)
	lines := strings.Split(output, "\n")

	var currentChain string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Detect chain headers
		if strings.HasPrefix(line, "Chain ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentChain = parts[1]
			}
			continue
		}

		// Skip header lines and empty lines
		if line == "" || strings.Contains(line, "pkts bytes target") || strings.Contains(line, "prot opt source") {
			continue
		}

		// Parse NAT66 rules (MASQUERADE, SNAT, DNAT)
		if (strings.Contains(line, "MASQUERADE") || strings.Contains(line, "SNAT") || strings.Contains(line, "DNAT")) && currentChain != "" {
			rule := parseNat66RuleForConfig(line, currentChain)
			if rule.Interface != "" {
				rules[rule.Interface] = append(rules[rule.Interface], rule)
			}
		}
	}

	return rules
}

func parseNat66RuleForConfig(line, chain string) Nat66Rule {
	fields := strings.Fields(line)
	rule := Nat66Rule{
		Chain: chain,
	}

	if len(fields) < 8 {
		return rule
	}

	// ip6tables -L -n -v output format:
	// pkts bytes target prot opt in     out    source               destination         [extra options]
	// Example: 0   0 MASQUERADE all  --  any    gtwl    anywhere             anywhere
	//          0   1   2         3    4   5      6      7                    8

	// Extract target type
	rule.Target = fields[2]

	// Extract interface information from correct positions
	inInterface := fields[5]  // "in" interface
	outInterface := fields[6] // "out" interface

	// Extract source and destination
	if len(fields) > 8 {
		rule.Source = fields[7]
		rule.Destination = fields[8]
	}

	// Determine interface and direction based on chain
	if chain == "POSTROUTING" {
		// For POSTROUTING, packets go OUT on the interface
		if outInterface != "any" && outInterface != "*" && outInterface != "--" {
			rule.Interface = outInterface
			rule.Direction = "POSTROUTING"
		}
	} else if chain == "PREROUTING" {
		// For PREROUTING, packets come IN on the interface
		if inInterface != "any" && inInterface != "*" && inInterface != "--" {
			rule.Interface = inInterface
			rule.Direction = "PREROUTING"
		}
	}

	return rule
}

func generateConfigYAML(interfaces []NetworkInterface, routes []Route, radvdConfig map[string]RadvdInterface, netmapRules map[string][]NetmapRule, nat66Rules map[string][]Nat66Rule, slim bool) string {
	config := `network:
  links:
`

	// Create a map of all interfaces (from network scan + radvd config + netmap rules)
	allInterfaces := make(map[string]bool)

	// Add interfaces from network scan
	for _, iface := range interfaces {
		allInterfaces[iface.Name] = true
	}

	// Add interfaces from radvd config
	for ifaceName := range radvdConfig {
		allInterfaces[ifaceName] = true
	}

	// Add interfaces from netmap rules
	for ifaceName := range netmapRules {
		allInterfaces[ifaceName] = true
	}

	// Add interfaces from NAT66 rules
	for ifaceName := range nat66Rules {
		allInterfaces[ifaceName] = true
	}

	for ifaceName := range allInterfaces {
		// Get radvd config for this interface
		radvdIface, hasRadvd := radvdConfig[ifaceName]

		// Get netmap rules for this interface
		ifaceNetmapRules, hasNetmap := netmapRules[ifaceName]

		// Get NAT66 rules for this interface
		ifaceNat66Rules, hasNat66 := nat66Rules[ifaceName]

		// Generate route entries based on scanned routes
		routeEntries := ""
		hasRoutes := false
		for _, route := range routes {
			if route.Interface == ifaceName {
				routeEntries = `        - route: ["::/0", "medium", 3600]
`
				hasRoutes = true
				break
			}
		}

		// Extract netmap mappings and correlate with radvd routes
		var netmapMappings []NetmapMapping
		var netmapWithRadv []NetmapMappingWithRadv
		var prefixConfig NetmapPrefixConfig
		if hasNetmap {
			netmapMappings = extractNetmapMappings(ifaceNetmapRules)
			prefixConfig = extractNetmapPrefixes(netmapMappings)

			// Correlate netmap mappings with radvd routes
			if hasRadvd {
				netmapWithRadv = correlateNetmapWithRadvRoutes(netmapMappings, radvdIface.Routes)
			}
		}

		// Generate netmap entries from captured rules with radv information
		netmapEntries := ""
		hasNetmapMaps := false
		if hasNetmap {
			for _, mapping := range netmapWithRadv {
				// Convert full addresses to relative paths if prefixes are available
				publicPath := mapping.Public
				privatePath := mapping.Private

				if prefixConfig.PublicPrefix != "" {
					publicPath = removePrefix(mapping.Public, prefixConfig.PublicPrefix)
				}
				if prefixConfig.PrivatePrefix != "" {
					privatePath = removePrefix(mapping.Private, prefixConfig.PrivatePrefix)
				}

				// Always add radv property with defaults if not already present
				if mapping.HasRadv {
					netmapEntries += `          - pair: ["` + publicPath + `", "` + privatePath + `", "` + mapping.RadvPreference + `", ` + strconv.Itoa(mapping.RadvLifetime) + `]
`
				} else {
					// Add default radv values
					netmapEntries += `          - pair: ["` + publicPath + `", "` + privatePath + `", "high", 3600]
`
				}
				hasNetmapMaps = true
			}

			// Add mappings without radv information
			for _, mapping := range netmapMappings {
				// Check if this mapping was already added with radv
				alreadyAdded := false
				for _, withRadv := range netmapWithRadv {
					if mapping.Public == withRadv.Public && mapping.Private == withRadv.Private {
						alreadyAdded = true
						break
					}
				}

				if !alreadyAdded {
					// Convert full addresses to relative paths if prefixes are available
					publicPath := mapping.Public
					privatePath := mapping.Private

					if prefixConfig.PublicPrefix != "" {
						publicPath = removePrefix(mapping.Public, prefixConfig.PublicPrefix)
					}
					if prefixConfig.PrivatePrefix != "" {
						privatePath = removePrefix(mapping.Private, prefixConfig.PrivatePrefix)
					}

					// Always add default radv values
					netmapEntries += `          - pair: ["` + publicPath + `", "` + privatePath + `", "high", 3600]
`
					hasNetmapMaps = true
				}
			}
		}

		// Generate prefix entries from radvd config
		prefixEntries := ""
		hasPrefixes := false
		if hasRadvd {
			for _, prefix := range radvdIface.Prefixes {
				prefixEntries += `        - prefix: "` + prefix.Prefix + `"
          on-link: ` + boolToString(prefix.OnLink) + `
          auto: ` + boolToString(prefix.Autonomous) + `
          adv-addr: ` + boolToString(prefix.RouterAddr) + `
          lifetime: [1800, 900]
`
				hasPrefixes = true
			}
		}

		// Use radvd settings if available, otherwise defaults
		minInterval := 30
		maxInterval := 60
		defaultLifetime := 180
		dhcp := false

		if hasRadvd {
			if radvdIface.MinRtrAdvInterval > 0 {
				minInterval = radvdIface.MinRtrAdvInterval
			}
			if radvdIface.MaxRtrAdvInterval > 0 {
				maxInterval = radvdIface.MaxRtrAdvInterval
			}
			if radvdIface.AdvDefaultLifetime >= 0 {
				defaultLifetime = radvdIface.AdvDefaultLifetime
			}
			dhcp = radvdIface.AdvManagedFlag
		}

		// Check if nat66 is enabled based on captured rules
		nat66Enabled := hasNat66 && len(ifaceNat66Rules) > 0
		nat44Enabled := false // TODO: Add logic to detect if nat44 is enabled

		// In slim mode, skip interfaces that have no enabled features
		if slim {
			hasEnabledFeatures := (hasNetmap && hasNetmapMaps) || (hasRadvd && (hasPrefixes || hasRoutes)) || nat66Enabled || nat44Enabled
			if !hasEnabledFeatures {
				continue
			}
		}

		config += `    ` + ifaceName + `:
`

		// Generate netmap6 section
		if (hasNetmap && hasNetmapMaps) || (!slim && hasNetmap) {
			config += `      netmap6:
        c1:
          enabled: ` + boolToString(hasNetmap && hasNetmapMaps) + `
`
			// Add prefixes if they were extracted
			if prefixConfig.PublicPrefix != "" {
				config += `          pfx-pub: "` + prefixConfig.PublicPrefix + `"
`
			}
			if prefixConfig.PrivatePrefix != "" {
				config += `          pfx-priv: "` + prefixConfig.PrivatePrefix + `"
`
			}

			if hasNetmapMaps {
				config += `          maps:
` + netmapEntries
			} else if !slim {
				config += `          maps: []
`
			}
		}

		// Generate nat66 section - only if enabled or not slim
		if nat66Enabled || !slim {
			config += `      nat66:
        enabled: ` + boolToString(nat66Enabled) + `
        mss-clamping: false
        mss: 1440
        origins: []
`
		}

		// Generate nat44 section - only if enabled or not slim
		if nat44Enabled || !slim {
			config += `      nat44:
        enabled: ` + boolToString(nat44Enabled) + `
        mss-clamping: false
        mss: 1440
        origins: []
`
		}

		// Generate radv section
		if (hasRadvd && (hasPrefixes || hasRoutes)) || (!slim && hasRadvd) || (!slim && !hasRadvd) {
			config += `      radv:
        enabled: ` + boolToString(hasRadvd) + `
        adv-interval: [` + strconv.Itoa(minInterval) + `, ` + strconv.Itoa(maxInterval) + `]
        lifetime: ` + strconv.Itoa(defaultLifetime) + `
        dhcp: ` + boolToString(dhcp) + `
`

			// Add prefixes - only if they exist or not slim
			if hasPrefixes {
				config += `        prefixes:
` + prefixEntries
			} else if !slim {
				config += `        prefixes: []
`
			}

			// Add routes - only if they exist or not slim
			if hasRoutes {
				config += `        routes:
` + routeEntries
			} else if !slim {
				config += `        routes: []
`
			}
		}
	}

	return config
}

// correlateNetmapWithRadvRoutes matches netmap mappings with radvd routes
func correlateNetmapWithRadvRoutes(mappings []NetmapMapping, routes []RadvdRoute) []NetmapMappingWithRadv {
	var result []NetmapMappingWithRadv

	for _, mapping := range mappings {
		mappingWithRadv := NetmapMappingWithRadv{
			Public:  mapping.Public,
			Private: mapping.Private,
			HasRadv: false,
		}

		// Look for matching radvd route
		for _, route := range routes {
			if route.Prefix == mapping.Public {
				mappingWithRadv.HasRadv = true
				mappingWithRadv.RadvPreference = route.Preference
				mappingWithRadv.RadvLifetime = route.Lifetime
				break
			}
		}

		result = append(result, mappingWithRadv)
	}

	return result
}

func extractNetmapMappings(rules []NetmapRule) []NetmapMapping {
	var mappings []NetmapMapping

	// Create a map to pair PREROUTING and POSTROUTING rules
	preRouting := make(map[string]NetmapRule)
	postRouting := make(map[string]NetmapRule)

	for _, rule := range rules {
		if rule.Direction == "PREROUTING" {
			// Use source as key for PREROUTING rules
			preRouting[rule.Source] = rule
		} else if rule.Direction == "POSTROUTING" {
			// Use source as key for POSTROUTING rules
			postRouting[rule.Source] = rule
		}
	}

	// Try to create mappings from POSTROUTING rules
	for _, postRule := range postRouting {
		if postRule.ToAddress != "" && postRule.Source != "" {
			mapping := NetmapMapping{
				Private: postRule.Source,
				Public:  postRule.ToAddress,
			}
			mappings = append(mappings, mapping)
		}
	}

	// If no POSTROUTING mappings found, try PREROUTING rules
	if len(mappings) == 0 {
		for _, preRule := range preRouting {
			if preRule.ToAddress != "" && preRule.Source != "" {
				mapping := NetmapMapping{
					Private: preRule.ToAddress,
					Public:  preRule.Source,
				}
				mappings = append(mappings, mapping)
			}
		}
	}

	return mappings
}

// extractNetmapPrefixes analyzes netmap mappings to extract common prefixes
func extractNetmapPrefixes(mappings []NetmapMapping) NetmapPrefixConfig {
	var config NetmapPrefixConfig

	if len(mappings) == 0 {
		return config
	}

	// Find common prefixes by analyzing all mappings
	publicPrefix := findCommonIPv6Prefix(mappings, true)
	privatePrefix := findCommonIPv6Prefix(mappings, false)

	config.PublicPrefix = publicPrefix
	config.PrivatePrefix = privatePrefix

	return config
}

// findCommonIPv6Prefix finds the common /64 prefix for public or private addresses
func findCommonIPv6Prefix(mappings []NetmapMapping, isPublic bool) string {
	if len(mappings) == 0 {
		return ""
	}

	var addresses []string
	for _, mapping := range mappings {
		if isPublic {
			addresses = append(addresses, mapping.Public)
		} else {
			addresses = append(addresses, mapping.Private)
		}
	}

	// Find the longest common prefix
	commonPrefix := ""
	if len(addresses) > 0 {
		// Parse the first address to establish base
		firstAddr := addresses[0]
		if strings.Contains(firstAddr, "/") {
			// Remove CIDR notation
			firstAddr = strings.Split(firstAddr, "/")[0]
		}

		// Split by colons to analyze segments

		// Try different prefix lengths to find the optimal one
		segments := strings.Split(firstAddr, ":")
		for prefixLen := 3; prefixLen >= 1; prefixLen-- {
			if len(segments) >= prefixLen {
				candidatePrefix := strings.Join(segments[:prefixLen], ":") + ":"

				// Verify this prefix works for all addresses and results in meaningful reduction
				allMatch := true
				hasReduction := false

				for _, addr := range addresses {
					cleanAddr := addr
					if strings.Contains(cleanAddr, "/") {
						cleanAddr = strings.Split(cleanAddr, "/")[0]
					}

					if !strings.HasPrefix(cleanAddr, candidatePrefix) {
						allMatch = false
						break
					}

					// Check if using this prefix would actually reduce the address
					remaining := strings.TrimPrefix(cleanAddr, candidatePrefix)
					remaining = strings.TrimPrefix(remaining, ":")
					if len(remaining) > 0 && remaining != cleanAddr {
						hasReduction = true
					}
				}

				if allMatch && hasReduction {
					commonPrefix = candidatePrefix
					break // Use the longest valid prefix
				}
			}
		}
	}

	return commonPrefix
}

// removePrefix removes the given prefix from an IPv6 address/range
func removePrefix(address, prefix string) string {
	if prefix == "" || !strings.HasPrefix(address, prefix) {
		return address
	}

	// Remove the prefix and return the remaining part
	remaining := strings.TrimPrefix(address, prefix)

	// Handle cases where the remaining part starts with ":"
	remaining = strings.TrimPrefix(remaining, ":")

	return remaining
}

func scanRadvdConfig() (map[string]RadvdInterface, error) {
	radvdPath := "/etc/radvd.conf"

	// Check if radvd.conf exists
	if _, err := os.Stat(radvdPath); os.IsNotExist(err) {
		return make(map[string]RadvdInterface), nil
	}

	content, err := os.ReadFile(radvdPath)
	if err != nil {
		return nil, err
	}

	return parseRadvdConfig(string(content)), nil
}

func parseRadvdConfig(content string) map[string]RadvdInterface {
	interfaces := make(map[string]RadvdInterface)

	// Remove comments but preserve structure
	content = removeCommentsPreserveStructure(content)

	// Find all interface blocks using a more robust approach
	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmedLine := strings.TrimSpace(line)

		// Check for interface declaration
		if strings.HasPrefix(trimmedLine, "interface ") {
			// Extract interface name
			parts := strings.Fields(trimmedLine)
			if len(parts) < 2 {
				continue
			}

			interfaceName := parts[1]

			// Find the opening brace
			braceStart := i
			if !strings.Contains(trimmedLine, "{") {
				// Look for opening brace on next lines
				for j := i + 1; j < len(lines); j++ {
					if strings.Contains(lines[j], "{") {
						braceStart = j
						break
					}
				}
			}

			// Collect the interface block content
			var blockContent strings.Builder
			braceCount := 0
			startCollecting := false

			for j := braceStart; j < len(lines); j++ {
				currentLine := lines[j]

				// Start counting braces from the line with the first opening brace
				if strings.Contains(currentLine, "{") && !startCollecting {
					startCollecting = true
				}

				if startCollecting {
					braceCount += strings.Count(currentLine, "{") - strings.Count(currentLine, "}")
					blockContent.WriteString(currentLine + "\n")

					// End of interface block
					if braceCount == 0 {
						// Parse the interface block
						radvdIface := parseInterfaceBlock(blockContent.String())
						interfaces[interfaceName] = radvdIface
						i = j // Skip to the end of this interface block
						break
					}
				}
			}
		}
	}

	return interfaces
}

func parseInterfaceBlock(block string) RadvdInterface {
	radvdIface := RadvdInterface{
		AdvSendAdvert:      true, // Default
		AdvManagedFlag:     false,
		MinRtrAdvInterval:  30,
		MaxRtrAdvInterval:  60,
		AdvDefaultLifetime: 180,
	}

	lines := strings.Split(block, "\n")

	// Parse simple key-value pairs first
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse simple key-value pairs
		if strings.Contains(line, "AdvManagedFlag") {
			radvdIface.AdvManagedFlag = strings.Contains(line, "on")
		} else if strings.Contains(line, "MinRtrAdvInterval") {
			if val := extractNumber(line); val >= 0 {
				radvdIface.MinRtrAdvInterval = val
			}
		} else if strings.Contains(line, "MaxRtrAdvInterval") {
			if val := extractNumber(line); val >= 0 {
				radvdIface.MaxRtrAdvInterval = val
			}
		} else if strings.Contains(line, "AdvDefaultLifetime") {
			if val := extractNumber(line); val >= 0 {
				radvdIface.AdvDefaultLifetime = val
			}
		}
	}

	// Parse prefix blocks
	radvdIface.Prefixes = parsePrefixBlocks(block)

	// Parse route blocks
	radvdIface.Routes = parseRouteBlocks(block)

	return radvdIface
}

func parsePrefixBlocks(content string) []RadvdPrefix {
	var results []RadvdPrefix
	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		// Look for prefix declarations
		if strings.HasPrefix(line, "prefix ") {
			// Extract the prefix identifier
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}

			identifier := parts[1]

			// Find the opening brace
			braceStart := i
			if !strings.Contains(line, "{") {
				// Look for opening brace on next lines
				for j := i + 1; j < len(lines); j++ {
					if strings.Contains(lines[j], "{") {
						braceStart = j
						break
					}
				}
			}

			// Collect the block content
			var blockContent strings.Builder
			braceCount := 0

			for j := braceStart; j < len(lines); j++ {
				currentLine := lines[j]
				braceCount += strings.Count(currentLine, "{") - strings.Count(currentLine, "}")

				// Don't include the prefix declaration line in the block content
				if j > i {
					blockContent.WriteString(currentLine + "\n")
				}

				// End of prefix block
				if braceCount == 0 && strings.Contains(currentLine, "}") {
					break
				}
			}

			// Parse the prefix block
			prefix := RadvdPrefix{
				Prefix:     identifier,
				OnLink:     true,  // Default
				Autonomous: true,  // Default
				RouterAddr: false, // Default
			}

			blockLines := strings.Split(blockContent.String(), "\n")
			for _, blockLine := range blockLines {
				blockLine = strings.TrimSpace(blockLine)
				if strings.Contains(blockLine, "AdvOnLink") {
					prefix.OnLink = strings.Contains(blockLine, "on")
				} else if strings.Contains(blockLine, "AdvAutonomous") {
					prefix.Autonomous = strings.Contains(blockLine, "on")
				} else if strings.Contains(blockLine, "AdvRouterAddr") {
					prefix.RouterAddr = strings.Contains(blockLine, "on")
				}
			}

			results = append(results, prefix)
		}
	}

	return results
}

func parseRouteBlocks(content string) []RadvdRoute {
	var results []RadvdRoute
	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		// Look for route declarations
		if strings.HasPrefix(line, "route ") {
			// Extract the route identifier
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}

			identifier := parts[1]

			// Check if this is a single-line route declaration
			if strings.Contains(line, "{") && strings.Contains(line, "}") {
				// Parse inline route options
				route := RadvdRoute{
					Prefix:     identifier,
					Preference: "medium", // Default
					Lifetime:   3600,     // Default
				}

				// Extract options from the same line
				if strings.Contains(line, "AdvRoutePreference") {
					if strings.Contains(line, "high") {
						route.Preference = "high"
					} else if strings.Contains(line, "low") {
						route.Preference = "low"
					}
				}
				if strings.Contains(line, "AdvRouteLifetime") {
					if val := extractNumberFromLine(line, "AdvRouteLifetime"); val >= 0 {
						route.Lifetime = val
					}
				}

				results = append(results, route)
				continue
			}

			// Multi-line route block
			braceStart := i
			if !strings.Contains(line, "{") {
				// Look for opening brace on next lines
				for j := i + 1; j < len(lines); j++ {
					if strings.Contains(lines[j], "{") {
						braceStart = j
						break
					}
				}
			}

			// Collect the block content
			var blockContent strings.Builder
			braceCount := 0

			for j := braceStart; j < len(lines); j++ {
				currentLine := lines[j]
				braceCount += strings.Count(currentLine, "{") - strings.Count(currentLine, "}")

				// Don't include the route declaration line in the block content
				if j > i {
					blockContent.WriteString(currentLine + "\n")
				}

				// End of route block
				if braceCount == 0 && strings.Contains(currentLine, "}") {
					break
				}
			}

			// Parse the route block
			route := RadvdRoute{
				Prefix:     identifier,
				Preference: "medium", // Default
				Lifetime:   3600,     // Default
			}

			blockLines := strings.Split(blockContent.String(), "\n")
			for _, blockLine := range blockLines {
				blockLine = strings.TrimSpace(blockLine)
				if strings.Contains(blockLine, "AdvRoutePreference") {
					if strings.Contains(blockLine, "high") {
						route.Preference = "high"
					} else if strings.Contains(blockLine, "low") {
						route.Preference = "low"
					} else {
						route.Preference = "medium"
					}
				} else if strings.Contains(blockLine, "AdvRouteLifetime") {
					if val := extractNumber(blockLine); val >= 0 {
						route.Lifetime = val
					}
				}
			}

			results = append(results, route)
		}
	}

	return results
}

func extractNumberFromLine(line, keyword string) int {
	// Find the keyword in the line
	idx := strings.Index(line, keyword)
	if idx < 0 {
		return -1
	}

	// Extract the part after the keyword
	remaining := line[idx+len(keyword):]

	// Find the number
	numberRegex := regexp.MustCompile(`\b(\d+)\b`)
	matches := numberRegex.FindStringSubmatch(remaining)
	if len(matches) >= 2 {
		if val, err := strconv.Atoi(matches[1]); err == nil {
			return val
		}
	}
	return -1
}

func removeCommentsPreserveStructure(content string) string {
	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		// Remove comments but preserve the line structure
		if commentIndex := strings.Index(line, "#"); commentIndex >= 0 {
			line = line[:commentIndex]
		}
		// Keep the line even if it's empty to preserve structure
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

func extractNumber(line string) int {
	numberRegex := regexp.MustCompile(`\b(\d+)\b`)
	matches := numberRegex.FindStringSubmatch(line)
	if len(matches) >= 2 {
		if val, err := strconv.Atoi(matches[1]); err == nil {
			return val
		}
	}
	return -1
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

type NetworkInterface struct {
	Name          string
	IPv4Addresses []string
	IPv6Addresses []string
}

type Route struct {
	Destination string
	Gateway     string
	Interface   string
}

type RadvdInterface struct {
	AdvSendAdvert      bool
	AdvManagedFlag     bool
	MinRtrAdvInterval  int
	MaxRtrAdvInterval  int
	AdvDefaultLifetime int
	Prefixes           []RadvdPrefix
	Routes             []RadvdRoute
}

type RadvdPrefix struct {
	Prefix     string
	OnLink     bool
	Autonomous bool
	RouterAddr bool
}

type RadvdRoute struct {
	Prefix     string
	Preference string
	Lifetime   int
}

type NetmapRule struct {
	Interface   string
	Direction   string // PREROUTING or POSTROUTING
	Chain       string // The actual iptables chain name
	Source      string
	Destination string
	ToAddress   string
}

type NetmapMapping struct {
	Public  string
	Private string
}

type NetmapMappingWithRadv struct {
	Public         string
	Private        string
	HasRadv        bool
	RadvPreference string
	RadvLifetime   int
}

type Nat66Rule struct {
	Interface   string
	Direction   string // PREROUTING or POSTROUTING
	Chain       string // The actual iptables chain name
	Target      string // MASQUERADE, SNAT, DNAT
	Source      string
	Destination string
}

type NetmapPrefixConfig struct {
	PublicPrefix  string
	PrivatePrefix string
}

func WriteConfigToFile(config, filePath string) error {
	return os.WriteFile(filePath, []byte(config), 0644)
}
