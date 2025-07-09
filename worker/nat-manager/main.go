package natmanager

import (
	"fmt"
	"os/exec"
	"strings"

	"natman/link"
)

// Global quiet mode flag
var QuietMode bool = false

// SetQuietMode sets the quiet mode for suppressing non-essential output
func SetQuietMode(quiet bool) {
	QuietMode = quiet
}

// it generates and applies NAT66 and NAT44 configuration
// including masquerading and policy based routing

// make sure we do not duplicate the entries
// check for entries present in the iptables but removed from config
// remove them from iptables

func ApplyNatRules(links map[string]*link.Link) error {
	// Apply NAT44 rules
	if err := applyNat44Rules(links); err != nil {
		return fmt.Errorf("failed to apply NAT44 rules: %v", err)
	}

	// Apply NAT66 rules
	if err := applyNat66Rules(links); err != nil {
		return fmt.Errorf("failed to apply NAT66 rules: %v", err)
	}

	return nil
}

func applyNat44Rules(links map[string]*link.Link) error {
	// Get current NAT44 rules
	currentRules, err := getCurrentNat44Rules()
	if err != nil {
		return err
	}

	// Debug: Print current rules only if not in quiet mode
	if !QuietMode && len(currentRules) > 0 {
		fmt.Println("Current NAT44 rules:")
		for _, rule := range currentRules {
			fmt.Printf("  - %s\n", rule)
		}
	}

	// Generate new rules
	var newRules []string
	for linkName, linkObj := range links {
		if linkObj.Nat44 != nil && linkObj.Nat44.Enabled {
			rules := generateNat44Rules(linkName, linkObj.Nat44)
			newRules = append(newRules, rules...)
		}
	}

	// Debug: Print new rules only if not in quiet mode
	if !QuietMode && len(newRules) > 0 {
		fmt.Println("New NAT44 rules to apply:")
		for _, rule := range newRules {
			fmt.Printf("  - %s\n", rule)
		}
	}

	// Apply rule changes
	return applyRuleChanges(currentRules, newRules)
}

func applyNat66Rules(links map[string]*link.Link) error {
	// Get current NAT66 rules
	currentRules, err := getCurrentNat66Rules()
	if err != nil {
		return err
	}

	// Generate new rules
	var newRules []string
	for linkName, linkObj := range links {
		if linkObj.Nat66 != nil && linkObj.Nat66.Enabled {
			rules := generateNat66Rules(linkName, linkObj.Nat66)
			newRules = append(newRules, rules...)
		}
	}

	// Apply rule changes
	return applyRuleChanges(currentRules, newRules)
}

func generateNat44Rules(interfaceName string, nat44 *link.Nat44) []string {
	var rules []string

	if nat44 == nil || !nat44.Enabled {
		return rules
	}

	// Validate interface name
	if interfaceName == "" {
		return rules
	}

	// Basic masquerading rule
	masqRule := fmt.Sprintf("iptables -t nat -A POSTROUTING -o %s -j MASQUERADE", interfaceName)
	rules = append(rules, masqRule)

	// MSS clamping if enabled
	if nat44.MssClamping && nat44.Mss > 0 {
		mssRule := fmt.Sprintf("iptables -t mangle -A FORWARD -o %s -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --set-mss %d",
			interfaceName, nat44.Mss)
		rules = append(rules, mssRule)
	}

	// Policy-based routing for origins
	for _, origin := range nat44.Origins {
		if origin != "" {
			pbrRule := fmt.Sprintf("iptables -t nat -A POSTROUTING -s %s -o %s -j MASQUERADE", origin, interfaceName)
			rules = append(rules, pbrRule)
		}
	}

	return rules
}

func generateNat66Rules(interfaceName string, nat66 *link.Nat66) []string {
	var rules []string

	if nat66 == nil || !nat66.Enabled {
		return rules
	}

	// Validate interface name
	if interfaceName == "" {
		return rules
	}

	// Basic masquerading rule for IPv6
	masqRule := fmt.Sprintf("ip6tables -t nat -A POSTROUTING -o %s -j MASQUERADE", interfaceName)
	rules = append(rules, masqRule)

	// MSS clamping if enabled
	if nat66.MssClamping && nat66.Mss > 0 {
		mssRule := fmt.Sprintf("ip6tables -t mangle -A FORWARD -o %s -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --set-mss %d",
			interfaceName, nat66.Mss)
		rules = append(rules, mssRule)
	}

	// Policy-based routing for origins
	for _, origin := range nat66.Origins {
		if origin != "" {
			pbrRule := fmt.Sprintf("ip6tables -t nat -A POSTROUTING -s %s -o %s -j MASQUERADE", origin, interfaceName)
			rules = append(rules, pbrRule)
		}
	}

	return rules
}

func getCurrentNat44Rules() ([]string, error) {
	return getCurrentNatRules("iptables")
}

func getCurrentNat66Rules() ([]string, error) {
	return getCurrentNatRules("ip6tables")
}

func getCurrentNatRules(iptablesCmd string) ([]string, error) {
	var rules []string

	// Get NAT table rules
	cmd := exec.Command(iptablesCmd, "-t", "nat", "-S")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for MASQUERADE, SNAT, or DNAT rules
		if (strings.Contains(line, "MASQUERADE") || strings.Contains(line, "SNAT") || strings.Contains(line, "DNAT")) && strings.HasPrefix(line, "-A ") {
			// Extract the chain and rule details
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				chain := parts[1]
				// Convert to full command format using the iptablesCmd parameter
				rule := iptablesCmd + " -t nat -A " + chain
				for i := 2; i < len(parts); i++ {
					rule += " " + parts[i]
				}
				rules = append(rules, rule)
			}
		}
	}

	// Get mangle table rules for MSS clamping using the same iptablesCmd
	cmd = exec.Command(iptablesCmd, "-t", "mangle", "-S")
	output, err = cmd.Output()
	if err != nil {
		return rules, nil // Don't fail if mangle table query fails
	}

	lines = strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "TCPMSS") && strings.HasPrefix(line, "-A ") {
			// Extract the chain and rule details
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				chain := parts[1]
				// Convert to full command format using the iptablesCmd parameter
				rule := iptablesCmd + " -t mangle -A " + chain
				for i := 2; i < len(parts); i++ {
					rule += " " + parts[i]
				}
				rules = append(rules, rule)
			}
		}
	}

	return rules, nil
}

func applyRuleChanges(currentRules, newRules []string) error {
	// Normalize rules for comparison
	normalizedCurrent := make([]string, len(currentRules))
	normalizedNew := make([]string, len(newRules))

	for i, rule := range currentRules {
		normalizedCurrent[i] = normalizeRule(rule)
	}
	for i, rule := range newRules {
		normalizedNew[i] = normalizeRule(rule)
	}

	// Calculate rules to add and remove using normalized versions
	rulesToAdd := difference(normalizedNew, normalizedCurrent)
	rulesToRemove := difference(normalizedCurrent, normalizedNew)

	// Map back to original rules
	addMap := make(map[string]string)
	for i, norm := range normalizedNew {
		addMap[norm] = newRules[i]
	}
	removeMap := make(map[string]string)
	for i, norm := range normalizedCurrent {
		removeMap[norm] = currentRules[i]
	}

	// Remove old rules
	for _, normRule := range rulesToRemove {
		if origRule, ok := removeMap[normRule]; ok {
			removeRule := strings.Replace(origRule, "-A ", "-D ", 1)
			if err := executeIptablesRule(removeRule); err != nil {
				if !QuietMode {
					fmt.Printf("Warning: failed to remove rule %s: %v\n", removeRule, err)
				}
			}
		}
	}

	// Add new rules
	for _, normRule := range rulesToAdd {
		if origRule, ok := addMap[normRule]; ok {
			if err := executeIptablesRule(origRule); err != nil {
				return fmt.Errorf("failed to add rule %s: %v", origRule, err)
			}
		}
	}

	return nil
}

// normalizeRule normalizes a rule for comparison by removing variations in formatting
func normalizeRule(rule string) string {
	// Split and rejoin to normalize whitespace
	parts := strings.Fields(rule)

	// Normalize common variations
	for i, part := range parts {
		// Normalize IP address representations
		if part == "0.0.0.0/0" || part == "anywhere" {
			parts[i] = "0.0.0.0/0"
		} else if part == "::/0" {
			parts[i] = "::/0"
		}
	}

	return strings.Join(parts, " ")
}

func executeIptablesRule(rule string) error {
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

func PrintCurrentNatRules() error {
	if QuietMode {
		return nil
	}

	fmt.Println("Current NAT Rules:")
	fmt.Println("==================")

	// Print IPv4 NAT rules
	fmt.Println("\nIPv4 NAT Rules (iptables):")
	ipv4Rules, err := getCurrentNat44Rules()
	if err != nil {
		fmt.Printf("Error getting IPv4 rules: %v\n", err)
	} else if len(ipv4Rules) == 0 {
		fmt.Println("No IPv4 NAT rules found")
	} else {
		for i, rule := range ipv4Rules {
			fmt.Printf("  %d. %s\n", i+1, rule)
		}
	}

	// Print IPv6 NAT rules
	fmt.Println("\nIPv6 NAT Rules (ip6tables):")
	ipv6Rules, err := getCurrentNat66Rules()
	if err != nil {
		fmt.Printf("Error getting IPv6 rules: %v\n", err)
	} else if len(ipv6Rules) == 0 {
		fmt.Println("No IPv6 NAT rules found")
	} else {
		for i, rule := range ipv6Rules {
			fmt.Printf("  %d. %s\n", i+1, rule)
		}
	}

	return nil
}

func CaptureNatRulesFromSystem() (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Capture IPv4 rules
	ipv4Rules, err := getCurrentNat44Rules()
	if err != nil {
		return nil, fmt.Errorf("failed to get IPv4 NAT rules: %v", err)
	}

	// Capture IPv6 rules
	ipv6Rules, err := getCurrentNat66Rules()
	if err != nil {
		return nil, fmt.Errorf("failed to get IPv6 NAT rules: %v", err)
	}

	// Group rules by interface and type
	ipv4ByInterface := groupNatRulesByInterface(ipv4Rules)
	ipv6ByInterface := groupNatRulesByInterface(ipv6Rules)

	result["ipv4"] = ipv4ByInterface
	result["ipv6"] = ipv6ByInterface

	return result, nil
}

func groupNatRulesByInterface(rules []string) map[string][]string {
	rulesByInterface := make(map[string][]string)

	for _, rule := range rules {
		var iface string

		// Extract interface name from rule
		if strings.Contains(rule, "-o ") {
			parts := strings.Fields(rule)
			for i, part := range parts {
				if part == "-o" && i+1 < len(parts) {
					iface = parts[i+1]
					break
				}
			}
		}

		if iface != "" {
			rulesByInterface[iface] = append(rulesByInterface[iface], rule)
		}
	}

	return rulesByInterface
}

func FlushNatRules(ipVersion string) error {
	var iptablesCmd string

	switch ipVersion {
	case "ipv4":
		iptablesCmd = "iptables"
	case "ipv6":
		iptablesCmd = "ip6tables"
	case "both":
		// Flush both
		if err := FlushNatRules("ipv4"); err != nil {
			return err
		}
		return FlushNatRules("ipv6")
	default:
		return fmt.Errorf("invalid IP version: %s (use 'ipv4', 'ipv6', or 'both')", ipVersion)
	}

	// Flush NAT table
	cmd := exec.Command(iptablesCmd, "-t", "nat", "-F")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to flush NAT table: %v", err)
	}

	// Flush mangle table FORWARD chain (for MSS clamping)
	cmd = exec.Command(iptablesCmd, "-t", "mangle", "-F", "FORWARD")
	if err := cmd.Run(); err != nil {
		if !QuietMode {
			fmt.Printf("Warning: failed to flush mangle FORWARD chain: %v\n", err)
		}
	}

	if !QuietMode {
		fmt.Printf("Flushed %s NAT rules\n", ipVersion)
	}
	return nil
}
