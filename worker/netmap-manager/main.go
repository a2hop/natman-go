package netmapmanager

import (
	"crypto/sha256"
	"fmt"
	"os/exec"
	"strings"

	"natman/link"
)

// It needs to be able to generate the mappings
// check if they already exist (if so - then skip)
// remove mappings that no longer exist in the config
// if any new mappings are added, then add them to the ip6tables
// make sure we do not have duplicates

// it also needs to be able to print the mappings in a human readable format

// Debug flag
var Debug bool = false

// SetDebug enables debug logging
func SetDebug(debug bool) {
	Debug = debug
}

// DebugPrint prints a message if debug mode is enabled
func DebugPrint(format string, args ...interface{}) {
	if Debug {
		fmt.Printf("[NETMAP-DEBUG] "+format+"\n", args...)
	}
}

func ApplyNetmapRules(links map[string]*link.Link) error {
	// Get current rules
	DebugPrint("Getting current netmap rules")
	currentRules, err := getCurrentNetmapRules()
	if err != nil {
		return fmt.Errorf("failed to get current rules: %v", err)
	}
	DebugPrint("Found %d current netmap rules", len(currentRules))

	// Generate new rules from config
	var newRules []string
	for linkName, linkObj := range links {
		for netmapName, netmap := range linkObj.Netmap6 {
			DebugPrint("Generating rules for link %s, netmap %s (enabled: %t)",
				linkName, netmapName, netmap.Enabled)

			rules := netmap.GenerateIp6tablesRules(linkName)
			DebugPrint("Generated %d rules for link %s, netmap %s",
				len(rules), linkName, netmapName)

			// Dump mappings if debug is enabled
			if Debug && len(rules) == 0 && netmap.Enabled {
				DebugPrint("Detailed mapping dump for %s.%s:", linkName, netmapName)
				for i, mapping := range netmap.Maps {
					DebugPrint("  Map[%d]: Public=%s, Private=%s", i, mapping.Public, mapping.Private)
					pubExp := netmap.SimpleConcatAddress(mapping.Public, netmap.PfxPub)
					privExp := netmap.SimpleConcatAddress(mapping.Private, netmap.PfxPriv)
					DebugPrint("    Expanded: Public=%s, Private=%s", pubExp, privExp)
				}
			}

			if len(rules) == 0 && netmap.Enabled && len(netmap.Maps) > 0 {
				fmt.Printf("Warning: No valid netmap rules generated for enabled netmap %s on interface %s\n",
					netmapName, linkName)

				// Try a direct rule creation as a fallback
				for _, mapping := range netmap.Maps {
					publicAddr := netmap.SimpleConcatAddress(mapping.Public, netmap.PfxPub)
					privateAddr := netmap.SimpleConcatAddress(mapping.Private, netmap.PfxPriv)

					// Create direct rules as a fallback
					postrouting := fmt.Sprintf("ip6tables -t nat -A POSTROUTING -o %s -s %s -j NETMAP --to %s",
						linkName, privateAddr, publicAddr)
					prerouting := fmt.Sprintf("ip6tables -t nat -A PREROUTING -i %s -d %s -j NETMAP --to %s",
						linkName, publicAddr, privateAddr)

					fmt.Printf("Trying fallback direct rule: %s\n", postrouting)
					if err := executeIp6tablesRule(postrouting); err != nil {
						fmt.Printf("Error: Failed to add direct rule: %v\n", err)
					} else {
						fmt.Printf("Successfully added direct rule\n")
					}

					fmt.Printf("Trying fallback direct rule: %s\n", prerouting)
					if err := executeIp6tablesRule(prerouting); err != nil {
						fmt.Printf("Error: Failed to add direct rule: %v\n", err)
					} else {
						fmt.Printf("Successfully added direct rule\n")
					}
				}
			}

			newRules = append(newRules, rules...)
		}
	}
	DebugPrint("Generated %d total new rules", len(newRules))

	// Check if we have any rules to apply
	if len(newRules) == 0 {
		fmt.Println("No valid netmap rules to apply - check your configuration and debug logs")
		return nil
	}

	// Normalize rules for better comparison
	normalizedCurrent := normalizeRules(currentRules)
	normalizedNew := normalizeRules(newRules)

	// Debug normalized rules
	if Debug {
		DebugPrint("=== Normalized current rules ===")
		for i, rule := range normalizedCurrent {
			DebugPrint("Current[%d]: %s", i, rule)
		}
		DebugPrint("=== Normalized new rules ===")
		for i, rule := range normalizedNew {
			DebugPrint("New[%d]: %s", i, rule)
		}
	}

	// Calculate rules to add and remove using normalized comparison
	rulesToAdd := smartDifference(newRules, currentRules, normalizedNew, normalizedCurrent)
	rulesToRemove := smartDifference(currentRules, newRules, normalizedCurrent, normalizedNew)

	DebugPrint("Rules to add: %d", len(rulesToAdd))
	DebugPrint("Rules to remove: %d", len(rulesToRemove))

	// Show rules to add in verbose mode
	for i, rule := range rulesToAdd {
		DebugPrint("Rule to add %d: %s", i+1, rule)
	}

	// Remove old rules
	for i, rule := range rulesToRemove {
		removeRule := strings.Replace(rule, "-A ", "-D ", 1)
		DebugPrint("Removing rule %d: %s", i, removeRule)
		if err := executeIp6tablesRule(removeRule); err != nil {
			fmt.Printf("Warning: failed to remove rule %s: %v\n", removeRule, err)
			DebugPrint("Failed to remove rule: %v", err)
		}
	}

	// Add new rules
	var failedRules []string
	for i, rule := range rulesToAdd {
		DebugPrint("Adding rule %d: %s", i, rule)
		if err := executeIp6tablesRule(rule); err != nil {
			fmt.Printf("Error: Failed to add netmap rule: %s\n", rule)
			fmt.Printf("  Error details: %v\n", err)
			failedRules = append(failedRules, rule)
		}
	}

	if len(failedRules) > 0 {
		return fmt.Errorf("failed to add %d netmap rules out of %d total rules",
			len(failedRules), len(rulesToAdd))
	}

	return nil
}

// normalizeRules converts rules to a normalized format for comparison
func normalizeRules(rules []string) []string {
	var normalized []string
	for _, rule := range rules {
		normalized = append(normalized, normalizeRule(rule))
	}
	return normalized
}

// normalizeRule converts a rule to a standardized format for comparison
func normalizeRule(rule string) string {
	// Parse the rule into components
	parts := strings.Fields(rule)
	if len(parts) < 6 {
		return rule
	}

	var chain, iface, direction, source, dest, target, toAddr string

	// Extract basic components
	for i, part := range parts {
		switch part {
		case "-A":
			if i+1 < len(parts) {
				chain = parts[i+1]
			}
		case "-i":
			if i+1 < len(parts) {
				iface = parts[i+1]
				direction = "input"
			}
		case "-o":
			if i+1 < len(parts) {
				iface = parts[i+1]
				direction = "output"
			}
		case "-s":
			if i+1 < len(parts) {
				source = parts[i+1]
			}
		case "-d":
			if i+1 < len(parts) {
				dest = parts[i+1]
			}
		case "-j":
			if i+1 < len(parts) {
				target = parts[i+1]
			}
		case "--to":
			if i+1 < len(parts) {
				toAddr = parts[i+1]
			}
		}
	}

	// Build normalized string with consistent ordering
	// Format: chain|direction|iface|source|dest|target|toAddr
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s",
		strings.ToLower(chain),
		direction,
		iface,
		source,
		dest,
		strings.ToLower(target),
		toAddr)
}

// smartDifference compares rules using both original and normalized forms
func smartDifference(originalSlice1, _, normalizedSlice1, normalizedSlice2 []string) []string {
	// Create a set of normalized rules from slice2
	normalizedSet := make(map[string]bool)
	for _, item := range normalizedSlice2 {
		normalizedSet[item] = true
	}

	var result []string
	for i, item := range normalizedSlice1 {
		// Check if normalized version exists in the set
		if !normalizedSet[item] {
			// If not found, add the original rule to result
			if i < len(originalSlice1) {
				result = append(result, originalSlice1[i])
			}
		} else {
			// Rule exists, log it for debugging
			DebugPrint("Rule already exists (normalized match): %s", originalSlice1[i])
		}
	}

	return result
}

func getCurrentNetmapRules() ([]string, error) {
	// Try using -S first (saves format)
	cmd := exec.Command("ip6tables", "-t", "nat", "-S")
	output, err := cmd.Output()
	if err == nil {
		rules := parseNetmapRulesFromSaves(string(output))
		if len(rules) > 0 {
			return rules, nil
		}
	}

	// Fallback to -L -n format if -S doesn't work or returns no rules
	cmd = exec.Command("ip6tables", "-t", "nat", "-L", "-n")
	output, err = cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseNetmapRulesFromList(string(output)), nil
}

// parseNetmapRulesFromSaves - update to normalize the output format
func parseNetmapRulesFromSaves(output string) []string {
	var rules []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "NETMAP") && strings.HasPrefix(line, "-A ") {
			// Normalize the rule format to match our generated rules
			rule := normalizeRuleFormat(line)
			if rule != "" {
				rules = append(rules, rule)
			}
		}
	}

	return rules
}

// normalizeRuleFormat converts ip6tables rule to consistent parameter order
func normalizeRuleFormat(line string) string {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return ""
	}

	chain := parts[1]
	var iface, direction, source, dest, toAddr string
	var isInput bool

	// Parse all parameters
	for i := 2; i < len(parts); i++ {
		switch parts[i] {
		case "-i":
			if i+1 < len(parts) {
				iface = parts[i+1]
				direction = "-i"
				isInput = true
				i++
			}
		case "-o":
			if i+1 < len(parts) {
				iface = parts[i+1]
				direction = "-o"
				isInput = false
				i++
			}
		case "-s":
			if i+1 < len(parts) {
				source = parts[i+1]
				i++
			}
		case "-d":
			if i+1 < len(parts) {
				dest = parts[i+1]
				i++
			}
		case "--to":
			if i+1 < len(parts) {
				toAddr = parts[i+1]
				i++
			}
		}
	}

	// Rebuild rule with consistent parameter order
	var rule strings.Builder
	rule.WriteString("ip6tables -t nat -A ")
	rule.WriteString(chain)

	// Add interface (always before source/dest)
	if direction != "" && iface != "" {
		rule.WriteString(" ")
		rule.WriteString(direction)
		rule.WriteString(" ")
		rule.WriteString(iface)
	}

	// Add source/dest in consistent order
	if isInput && dest != "" {
		rule.WriteString(" -d ")
		rule.WriteString(dest)
	} else if !isInput && source != "" {
		rule.WriteString(" -s ")
		rule.WriteString(source)
	}

	// Add NETMAP target
	rule.WriteString(" -j NETMAP")

	// Add --to parameter
	if toAddr != "" {
		rule.WriteString(" --to ")
		rule.WriteString(toAddr)
	}

	return rule.String()
}

func parseNetmapRulesFromList(output string) []string {
	var rules []string
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
		if line == "" || strings.Contains(line, "pkts bytes target") {
			continue
		}

		// Parse NETMAP rules
		if strings.Contains(line, "NETMAP") {
			rule := parseNetmapRuleFromListLine(line, currentChain)
			if rule != "" {
				rules = append(rules, rule)
			}
		}
	}

	return rules
}

func parseNetmapRuleFromListLine(line, chain string) string {
	fields := strings.Fields(line)

	if len(fields) < 9 {
		return ""
	}

	// Skip if not a NETMAP rule
	if fields[2] != "NETMAP" {
		return ""
	}

	// Extract fields
	protocol := fields[3]
	inInterface := fields[5]
	outInterface := fields[6]
	source := fields[7]
	destination := fields[8]

	// Find "to:" address
	var toAddress string
	for i := 9; i < len(fields); i++ {
		if strings.HasPrefix(fields[i], "to:") {
			toAddress = strings.TrimPrefix(fields[i], "to:")
			break
		}
	}

	// Build the ip6tables command
	var rule strings.Builder
	rule.WriteString("ip6tables -t nat -A ")
	rule.WriteString(chain)

	// Add protocol
	if protocol != "" && protocol != "all" && protocol != "--" {
		rule.WriteString(" -p ")
		rule.WriteString(protocol)
	}

	// Add input interface for PREROUTING
	if chain == "PREROUTING" && inInterface != "" && inInterface != "any" && inInterface != "*" && inInterface != "--" {
		rule.WriteString(" -i ")
		rule.WriteString(inInterface)
	}

	// Add output interface for POSTROUTING
	if chain == "POSTROUTING" && outInterface != "" && outInterface != "any" && outInterface != "*" && outInterface != "--" {
		rule.WriteString(" -o ")
		rule.WriteString(outInterface)
	}

	// Add source
	if source != "" && source != "anywhere" && source != "0.0.0.0/0" && source != "::/0" {
		rule.WriteString(" -s ")
		rule.WriteString(source)
	}

	// Add destination
	if destination != "" && destination != "anywhere" && destination != "0.0.0.0/0" && destination != "::/0" {
		rule.WriteString(" -d ")
		rule.WriteString(destination)
	}

	// Add target and to address
	rule.WriteString(" -j NETMAP")
	if toAddress != "" {
		rule.WriteString(" --to ")
		rule.WriteString(toAddress)
	}

	return rule.String()
}

func executeIp6tablesRule(rule string) error {
	parts := strings.Fields(rule)
	if len(parts) == 0 {
		return fmt.Errorf("empty rule")
	}

	DebugPrint("Executing command: %s", rule)
	cmd := exec.Command(parts[0], parts[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		DebugPrint("Command failed: %v, output: %s", err, string(output))
		return fmt.Errorf("command failed: %v, output: %s", err, string(output))
	}
	DebugPrint("Command executed successfully")

	return nil
}

// PrintNetmapRules prints the current netmap configuration from the links
func PrintNetmapRules(links map[string]*link.Link) error {
	fmt.Println("Current Netmap6 Configuration:")
	fmt.Println("==============================")

	for linkName, linkObj := range links {
		fmt.Printf("\nInterface: %s\n", linkName)

		for setName, netmap := range linkObj.Netmap6 {
			if !netmap.Enabled {
				continue
			}

			fmt.Printf("  Set: %s\n", setName)
			fmt.Printf("    Public Prefix: %s\n", netmap.PfxPub)
			fmt.Printf("    Private Prefix: %s\n", netmap.PfxPriv)
			fmt.Printf("    Mappings:\n")

			for _, mapping := range netmap.Maps {
				fmt.Printf("      %s <-> %s\n", mapping.Private, mapping.Public)
			}
		}
	}

	return nil
}

func PrintCurrentNetmapRules() error {
	fmt.Println("Current ip6tables NETMAP Rules:")
	fmt.Println("===============================")

	rules, err := getCurrentNetmapRules()
	if err != nil {
		return fmt.Errorf("failed to get current rules: %v", err)
	}

	if len(rules) == 0 {
		fmt.Println("No NETMAP rules found")
		return nil
	}

	for i, rule := range rules {
		fmt.Printf("%d. %s\n", i+1, rule)
	}

	return nil
}

func CaptureNetmapRulesFromSystem() (map[string][]string, error) {
	rules, err := getCurrentNetmapRules()
	if err != nil {
		return nil, err
	}

	// Group rules by interface
	rulesByInterface := make(map[string][]string)

	for _, rule := range rules {
		// Extract interface name from rule
		var iface string
		if strings.Contains(rule, "-o ") {
			// POSTROUTING rule
			parts := strings.Fields(rule)
			for i, part := range parts {
				if part == "-o" && i+1 < len(parts) {
					iface = parts[i+1]
					break
				}
			}
		} else if strings.Contains(rule, "-i ") {
			// PREROUTING rule
			parts := strings.Fields(rule)
			for i, part := range parts {
				if part == "-i" && i+1 < len(parts) {
					iface = parts[i+1]
					break
				}
			}
		}

		if iface != "" {
			rulesByInterface[iface] = append(rulesByInterface[iface], rule)
		}
	}

	return rulesByInterface, nil
}

func GetNetmapHash(links map[string]*link.Link) string {
	var rules []string
	for linkName, linkObj := range links {
		for _, netmap := range linkObj.Netmap6 {
			netmapRules := netmap.GenerateIp6tablesRules(linkName)
			rules = append(rules, netmapRules...)
		}
	}

	combined := strings.Join(rules, "\n")
	hash := sha256.Sum256([]byte(combined))
	return fmt.Sprintf("%x", hash)
}
