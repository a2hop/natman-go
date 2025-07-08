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

func ApplyNetmapRules(links map[string]*link.Link) error {
	// Get current rules
	currentRules, err := getCurrentNetmapRules()
	if err != nil {
		return fmt.Errorf("failed to get current rules: %v", err)
	}

	// Generate new rules from config
	var newRules []string
	for linkName, linkObj := range links {
		for _, netmap := range linkObj.Netmap6 {
			rules := netmap.GenerateIp6tablesRules(linkName)
			newRules = append(newRules, rules...)
		}
	}

	// Calculate rules to add and remove
	rulesToAdd := difference(newRules, currentRules)
	rulesToRemove := difference(currentRules, newRules)

	// Remove old rules
	for _, rule := range rulesToRemove {
		removeRule := strings.Replace(rule, "-A ", "-D ", 1)
		if err := executeIp6tablesRule(removeRule); err != nil {
			fmt.Printf("Warning: failed to remove rule %s: %v\n", removeRule, err)
		}
	}

	// Add new rules
	for _, rule := range rulesToAdd {
		if err := executeIp6tablesRule(rule); err != nil {
			return fmt.Errorf("failed to add rule %s: %v", rule, err)
		}
	}

	return nil
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

func parseNetmapRulesFromSaves(output string) []string {
	var rules []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "NETMAP") && strings.HasPrefix(line, "-A ") {
			// Extract the chain and rule details
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				chain := parts[1]
				// Convert to full command format
				rule := "ip6tables -t nat -A " + chain
				for i := 2; i < len(parts); i++ {
					rule += " " + parts[i]
				}
				rules = append(rules, rule)
			}
		}
	}

	return rules
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

	cmd := exec.Command(parts[0], parts[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %s, output: %s", err, string(output))
	}

	return nil
}

func difference(slice1, slice2 []string) []string {
	set := make(map[string]bool)
	for _, item := range slice2 {
		set[item] = true
	}

	var result []string
	for _, item := range slice1 {
		if !set[item] {
			result = append(result, item)
		}
	}

	return result
}

func PrintNetmapRules(links map[string]*link.Link) {
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
