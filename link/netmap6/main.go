package netmap6

import (
	"fmt"
	"natman/config"
	"strings"
)

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
		netmap.Maps[i] = MapPair{
			Public:  mapPair.Pair[0],
			Private: mapPair.Pair[1],
		}
	}

	return netmap
}

func (n *Netmap6) GenerateIp6tablesRules(interfaceName string) []string {
	if !n.Enabled || interfaceName == "" {
		return nil
	}

	var rules []string

	for _, mapping := range n.Maps {
		// Skip empty mappings
		if mapping.Public == "" || mapping.Private == "" {
			continue
		}

		// Build complete addresses using prefixes if they exist
		publicAddr := mapping.Public
		privateAddr := mapping.Private

		// If we have prefixes and the mapping doesn't already include full IPv6 addresses, prepend them
		if n.PfxPub != "" && !strings.Contains(publicAddr, ":") {
			publicAddr = n.PfxPub + publicAddr
		}
		if n.PfxPriv != "" && !strings.Contains(privateAddr, ":") {
			privateAddr = n.PfxPriv + privateAddr
		}

		// Validate the addresses have colons (basic IPv6 validation)
		if !strings.Contains(publicAddr, ":") || !strings.Contains(privateAddr, ":") {
			continue
		}

		// POSTROUTING rule for outgoing traffic (private -> public)
		postrouting := fmt.Sprintf("ip6tables -t nat -A POSTROUTING -o %s -s %s -j NETMAP --to %s",
			interfaceName, privateAddr, publicAddr)

		// PREROUTING rule for incoming traffic (public -> private)
		prerouting := fmt.Sprintf("ip6tables -t nat -A PREROUTING -i %s -d %s -j NETMAP --to %s",
			interfaceName, publicAddr, privateAddr)

		rules = append(rules, postrouting, prerouting)
	}

	return rules
}
