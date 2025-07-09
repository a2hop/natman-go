package netmap6

import (
	"fmt"
	"natman/config"
	"strconv"
	"strings"
)

// Debug flag
var Debug bool = false

// SetDebug enables debug logging
func SetDebug(debug bool) {
	Debug = debug
}

// DebugPrint prints a message if debug mode is enabled
func DebugPrint(format string, args ...interface{}) {
	if Debug {
		fmt.Printf("[NETMAP6-DEBUG] "+format+"\n", args...)
	}
}

// the object representing a network map
// we will use it later to apply functions and so on
type Netmap6 struct {
	Name    string
	Enabled bool
	PfxPub  string
	PfxPriv string
	Maps    []MapPair
}

type MapPair struct {
	Public  string
	Private string
	Radv    *RadvRoute // Optional radv configuration
}

type RadvRoute struct {
	Preference string
	Metric     int
	Lifetime   int
	Prefix     string // Added field for the correct prefix format
}

func NewNetmap6(name string, cfg config.Netmap6Config) *Netmap6 {
	netmap := &Netmap6{
		Name:    name,
		Enabled: cfg.Enabled,
		PfxPub:  cfg.PfxPub,
		PfxPriv: cfg.PfxPriv,
		Maps:    make([]MapPair, len(cfg.Maps)),
	}

	for i, mapPair := range cfg.Maps {
		if len(mapPair.Pair) < 2 {
			continue // Skip invalid entries
		}

		pair := MapPair{
			Public:  "",
			Private: "",
		}

		// Parse public address (index 0)
		if pub, ok := mapPair.Pair[0].(string); ok {
			pair.Public = pub
		}

		// Parse private address (index 1)
		if priv, ok := mapPair.Pair[1].(string); ok {
			pair.Private = priv
		}

		// Check if radv configuration is present (4 elements total)
		if len(mapPair.Pair) >= 4 {
			preference := "medium" // default
			lifetime := 3600       // default

			// Parse preference (string at index 2)
			if pref, ok := mapPair.Pair[2].(string); ok {
				preference = pref
			}

			// Parse lifetime (int at index 3)
			if l, ok := mapPair.Pair[3].(int); ok {
				lifetime = l
			} else if l, ok := mapPair.Pair[3].(float64); ok {
				lifetime = int(l)
			}

			pair.Radv = &RadvRoute{
				Preference: preference,
				Lifetime:   lifetime,
			}
		}

		netmap.Maps[i] = pair
	}

	return netmap
}

func (n *Netmap6) GenerateIp6tablesRules(interfaceName string) []string {
	if !n.Enabled || interfaceName == "" {
		DebugPrint("Netmap disabled or no interface provided")
		return nil
	}

	var rules []string

	DebugPrint("Generating rules for interface %s with %d mappings", interfaceName, len(n.Maps))
	DebugPrint("Using prefixes - Public: %s, Private: %s", n.PfxPub, n.PfxPriv)

	for i, mapping := range n.Maps {
		// Skip empty mappings
		if mapping.Public == "" || mapping.Private == "" {
			DebugPrint("Mapping %d: Skipping empty mapping", i)
			continue
		}

		DebugPrint("Mapping %d: Public=%s, Private=%s", i, mapping.Public, mapping.Private)

		// Expand addresses using prefixes - Use simpler direct concatenation approach
		publicAddr := n.SimpleConcatAddress(mapping.Public, n.PfxPub)
		privateAddr := n.SimpleConcatAddress(mapping.Private, n.PfxPriv)

		DebugPrint("Expanded addresses - Public: %s, Private: %s", publicAddr, privateAddr)

		// POSTROUTING rule for outgoing traffic (private -> public)
		postrouting := fmt.Sprintf("ip6tables -t nat -A POSTROUTING -o %s -s %s -j NETMAP --to %s",
			interfaceName, privateAddr, publicAddr)
		DebugPrint("Generated POSTROUTING rule: %s", postrouting)

		// PREROUTING rule for incoming traffic (public -> private)
		prerouting := fmt.Sprintf("ip6tables -t nat -A PREROUTING -i %s -d %s -j NETMAP --to %s",
			interfaceName, publicAddr, privateAddr)
		DebugPrint("Generated PREROUTING rule: %s", prerouting)

		rules = append(rules, postrouting, prerouting)
	}

	DebugPrint("Total rules generated: %d", len(rules))
	return rules
}

// SimpleConcatAddress directly concatenates prefix with the address part
// This is a simpler approach that works better for the specific format in the config
func (n *Netmap6) SimpleConcatAddress(addressPart, prefix string) string {
	// If no prefix, return the address as-is
	if prefix == "" {
		return addressPart
	}

	// Handle CIDR notation in addressPart
	var addrPart, cidrSuffix string
	if strings.Contains(addressPart, "/") {
		parts := strings.Split(addressPart, "/")
		addrPart = parts[0]
		cidrSuffix = "/" + parts[1]
	} else {
		addrPart = addressPart
		cidrSuffix = ""
	}

	// Clean up prefix by removing trailing colon
	prefix = strings.TrimRight(prefix, ":")

	// Simple concatenation
	return prefix + ":" + addrPart + cidrSuffix
}

// isValidIPv6Address validates if the string is a proper IPv6 address or CIDR for ip6tables
func isValidIPv6Address(addr string) bool {
	if Debug {
		DebugPrint("Validating IPv6 address: %s", addr)
	}

	// Basic validation for ip6tables
	if addr == "" {
		DebugPrint("Address is empty")
		return false
	}

	// Handle CIDR notation
	var ipPart string
	if strings.Contains(addr, "/") {
		parts := strings.Split(addr, "/")
		if len(parts) != 2 {
			DebugPrint("Invalid CIDR format: wrong number of parts")
			return false
		}
		ipPart = parts[0]

		// Validate prefix length is a number between 0 and 128
		prefixLen := parts[1]
		if pl, err := strconv.Atoi(prefixLen); err != nil || pl < 0 || pl > 128 {
			DebugPrint("Invalid CIDR prefix length: %s", prefixLen)
			return false
		}
		DebugPrint("Valid CIDR prefix length: %s", prefixLen)
	} else {
		ipPart = addr
	}

	result := isValidIPv6AddressFormat(ipPart)
	if Debug {
		if result {
			DebugPrint("Address is valid")
		} else {
			DebugPrint("Address format is invalid")
		}
	}
	return result
}

// isValidIPv6AddressFormat checks if the string has a valid IPv6 address format
func isValidIPv6AddressFormat(ipPart string) bool {
	if Debug {
		DebugPrint("Validating IPv6 address format: %s", ipPart)
	}

	// Must contain at least one colon
	if !strings.Contains(ipPart, ":") {
		DebugPrint("No colons found")
		return false
	}

	// Cannot have three consecutive colons
	if strings.Contains(ipPart, ":::") {
		DebugPrint("Contains invalid ':::'")
		return false
	}

	// Count double-colon (compression) occurrences - at most one is allowed
	doubleColonCount := strings.Count(ipPart, "::")
	if doubleColonCount > 1 {
		DebugPrint("Contains multiple '::' compressions")
		return false
	}

	// Split by colon and validate each segment
	segments := strings.Split(ipPart, ":")
	DebugPrint("Address has %d segments", len(segments))

	// Handle compressed notation with :: at start, end, or middle
	hasCompression := strings.Contains(ipPart, "::")
	if hasCompression {
		// Count segments excluding empty ones from compression
		nonEmptyCount := 0
		for _, seg := range segments {
			if seg != "" {
				nonEmptyCount++
			}
		}
		DebugPrint("Address has compression with %d non-empty segments", nonEmptyCount)

		// With compression, we can have up to 7 non-empty segments
		if nonEmptyCount > 7 {
			DebugPrint("Too many segments (%d) with compression", nonEmptyCount)
			return false
		}
	} else {
		// Without compression, we must have exactly 8 segments
		if len(segments) != 8 {
			DebugPrint("Without compression, expected 8 segments but found %d", len(segments))
			return false
		}
	}

	// Validate each segment is valid hex and correct length
	for i, segment := range segments {
		if segment == "" {
			DebugPrint("Segment %d is empty (part of compression)", i)
			continue // Empty segment is part of :: compression
		}

		if len(segment) > 4 {
			DebugPrint("Segment %d is too long: %s", i, segment)
			return false
		}

		for j, c := range segment {
			if !((c >= '0' && c <= '9') ||
				(c >= 'a' && c <= 'f') ||
				(c >= 'A' && c <= 'F')) {
				DebugPrint("Invalid character in segment %d at position %d: %c", i, j, c)
				return false
			}
		}
	}

	DebugPrint("Address format is valid")
	return true
}

// GetRadvRoutes returns routes that should be automatically added to radvd config
func (n *Netmap6) GetRadvRoutes() []RadvRoute {
	var routes []RadvRoute

	if !n.Enabled {
		return routes
	}

	for _, mapping := range n.Maps {
		if mapping.Radv != nil {
			// Create route for the public prefix/address
			publicAddr := n.SimpleConcatAddress(mapping.Public, n.PfxPub)

			// Ensure it's a valid IPv6 address/prefix
			if isValidIPv6Address(publicAddr) {
				route := RadvRoute{
					Prefix:     publicAddr, // Use the full public address/prefix
					Preference: mapping.Radv.Preference,
					Metric:     mapping.Radv.Metric,
					Lifetime:   mapping.Radv.Lifetime,
				}
				routes = append(routes, route)
			}
		}
	}

	return routes
}
