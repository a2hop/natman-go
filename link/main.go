package link

import (
	"natman/config"
	"natman/link/netmap6"
	"natman/link/radv"
)

// Abstract object representing a network link.
type Link struct {
	Name    string
	Config  config.LinkConfig
	Netmap6 map[string]*netmap6.Netmap6
	Nat66   *Nat66
	Nat44   *Nat44
	Radv    *radv.RadvConfig
}

type Nat66 struct {
	Enabled     bool
	MssClamping bool
	Mss         int
	Origins     []string
}

type Nat44 struct {
	Enabled     bool
	MssClamping bool
	Mss         int
	Origins     []string
}

func NewLink(name string, cfg config.LinkConfig) *Link {
	link := &Link{
		Name:    name,
		Config:  cfg,
		Netmap6: make(map[string]*netmap6.Netmap6),
	}

	// Initialize netmap6 configurations
	for setName, netmapCfg := range cfg.Netmap6 {
		link.Netmap6[setName] = netmap6.NewNetmap6(setName, netmapCfg)
	}

	// Initialize NAT66 if configured
	if cfg.Nat66 != nil {
		link.Nat66 = &Nat66{
			Enabled:     cfg.Nat66.Enabled,
			MssClamping: cfg.Nat66.MssClamping,
			Mss:         cfg.Nat66.Mss,
			Origins:     cfg.Nat66.Origins,
		}
	}

	// Initialize NAT44 if configured
	if cfg.Nat44 != nil {
		link.Nat44 = &Nat44{
			Enabled:     cfg.Nat44.Enabled,
			MssClamping: cfg.Nat44.MssClamping,
			Mss:         cfg.Nat44.Mss,
			Origins:     cfg.Nat44.Origins,
		}
	}

	// Initialize RADV if configured
	if cfg.Radv != nil {
		link.Radv = radv.NewRadvConfig(*cfg.Radv)
	}

	return link
}

func BuildLinks(cfg *config.Config) map[string]*Link {
	links := make(map[string]*Link)

	for linkName, linkCfg := range cfg.Network.Links {
		links[linkName] = NewLink(linkName, linkCfg)
	}

	return links
}
