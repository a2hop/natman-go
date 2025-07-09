package radv

import (
	"fmt"
	"strings"

	"natman/config"
)

const RadvdConfPath = "/etc/radvd.conf"

type RadvConfig struct {
	Enabled         bool
	MinAdvInterval  int
	MaxAdvInterval  int
	DefaultLifetime int
	Dhcp            bool
	Prefixes        []PrefixConfig
	Routes          []RouteConfig
	AutoRoutes      []RouteConfig // Auto-generated from netmap6
	RDNSS           []RDNSSConfig // Recursive DNS Server configuration
	Include         []string
}

type PrefixConfig struct {
	Prefix            string
	Mode              string
	OnLink            bool
	Autonomous        bool
	ValidLifetime     int
	PreferredLifetime int
	RouterAddr        bool
}

type RouteConfig struct {
	Prefix     string
	Preference string
	Metric     int
	Lifetime   int
}

type RDNSSConfig struct {
	Servers  []string
	Lifetime int
}

type Config struct {
	Interfaces map[string]*RadvConfig
}

func NewRadvConfig(cfg config.RadvConfig) *RadvConfig {
	radv := &RadvConfig{
		Enabled: cfg.Enabled,
		Dhcp:    cfg.Dhcp,
		Include: cfg.Include,
	}

	// Parse adv-interval [min, max]
	if len(cfg.AdvInterval) >= 2 {
		radv.MinAdvInterval = cfg.AdvInterval[0]
		radv.MaxAdvInterval = cfg.AdvInterval[1]
	} else {
		// Set defaults
		radv.MinAdvInterval = 30
		radv.MaxAdvInterval = 60
	}

	// Set lifetime (renamed from default-lifetime)
	if cfg.Lifetime > 0 {
		radv.DefaultLifetime = cfg.Lifetime
	} else {
		radv.DefaultLifetime = 180
	}

	// Convert prefixes with new compact format
	for _, prefix := range cfg.Prefixes {
		pc := PrefixConfig{
			Prefix:     prefix.Prefix,
			OnLink:     prefix.OnLink,
			Autonomous: prefix.Auto,    // auto -> autonomous
			RouterAddr: prefix.AdvAddr, // adv-addr -> router-addr
		}

		// Parse lifetime [valid, preferred]
		if len(prefix.Lifetime) >= 2 {
			pc.ValidLifetime = prefix.Lifetime[0]
			pc.PreferredLifetime = prefix.Lifetime[1]
		} else {
			// Set defaults
			pc.ValidLifetime = 1800
			pc.PreferredLifetime = 900
		}

		radv.Prefixes = append(radv.Prefixes, pc)
	}

	// Convert routes from array format
	for _, routeArray := range cfg.Routes {
		if len(routeArray.Route) >= 2 {
			prefix := ""
			preference := "medium" // default
			lifetime := 3600       // default

			// Parse prefix (string at index 0)
			if p, ok := routeArray.Route[0].(string); ok {
				prefix = p
			}

			// Parse preference (string at index 1)
			if pref, ok := routeArray.Route[1].(string); ok {
				preference = pref
			}

			// Parse lifetime (optional int at index 2)
			if len(routeArray.Route) >= 3 {
				if l, ok := routeArray.Route[2].(int); ok {
					lifetime = l
				} else if l, ok := routeArray.Route[2].(float64); ok {
					lifetime = int(l)
				}
			}

			if prefix != "" {
				rc := RouteConfig{
					Prefix:     prefix,
					Preference: preference,
					Lifetime:   lifetime,
				}
				radv.Routes = append(radv.Routes, rc)
			}
		}
	}

	// Convert RDNSS configuration
	for _, rdnss := range cfg.RDNSS {
		rc := RDNSSConfig{
			Servers:  rdnss.Server,
			Lifetime: rdnss.Lifetime,
		}

		// Set default lifetime if not specified
		if rc.Lifetime == 0 {
			rc.Lifetime = 300 // 5 minutes default
		}

		radv.RDNSS = append(radv.RDNSS, rc)
	}

	return radv
}

func (r *RadvConfig) GenerateConfig(interfaceName string) string {
	if !r.Enabled {
		return ""
	}

	var config strings.Builder

	config.WriteString(fmt.Sprintf("interface %s {\n", interfaceName))
	config.WriteString("    AdvSendAdvert on;\n")
	config.WriteString(fmt.Sprintf("    MinRtrAdvInterval %d;\n", r.MinAdvInterval))
	config.WriteString(fmt.Sprintf("    MaxRtrAdvInterval %d;\n", r.MaxAdvInterval))
	config.WriteString(fmt.Sprintf("    AdvDefaultLifetime %d;\n", r.DefaultLifetime))

	if r.Dhcp {
		config.WriteString("    AdvManagedFlag on;\n")
		config.WriteString("    AdvOtherConfigFlag on;\n")
	}

	// Add prefixes
	for _, prefix := range r.Prefixes {
		config.WriteString(fmt.Sprintf("    prefix %s {\n", prefix.Prefix))
		config.WriteString(fmt.Sprintf("        AdvOnLink %s;\n", boolToOnOff(prefix.OnLink)))
		config.WriteString(fmt.Sprintf("        AdvAutonomous %s;\n", boolToOnOff(prefix.Autonomous)))
		config.WriteString(fmt.Sprintf("        AdvRouterAddr %s;\n", boolToOnOff(prefix.RouterAddr)))

		// Only include lifetime settings if they differ from defaults
		if prefix.ValidLifetime != 1800 {
			config.WriteString(fmt.Sprintf("        AdvValidLifetime %d;\n", prefix.ValidLifetime))
		}
		if prefix.PreferredLifetime != 900 {
			config.WriteString(fmt.Sprintf("        AdvPreferredLifetime %d;\n", prefix.PreferredLifetime))
		}

		config.WriteString("    };\n")
	}

	// Add manual routes (one-liner format)
	for _, route := range r.Routes {
		config.WriteString(fmt.Sprintf("    route %s { AdvRoutePreference %s; AdvRouteLifetime %d; };\n",
			route.Prefix, route.Preference, route.Lifetime))
	}

	// Add auto-generated routes from netmap6 (one-liner format)
	if len(r.AutoRoutes) > 0 {
		config.WriteString("    # Auto-generated routes from netmap6\n")
		for _, route := range r.AutoRoutes {
			config.WriteString(fmt.Sprintf("    route %s { AdvRoutePreference %s; AdvRouteLifetime %d; };\n",
				route.Prefix, route.Preference, route.Lifetime))
		}
	}

	// Add RDNSS entries
	for _, rdnss := range r.RDNSS {
		if len(rdnss.Servers) > 0 {
			config.WriteString("    RDNSS")
			for _, server := range rdnss.Servers {
				config.WriteString(fmt.Sprintf(" %s", server))
			}
			config.WriteString(fmt.Sprintf(" { AdvRDNSSLifetime %d; };\n", rdnss.Lifetime))
		}
	}

	config.WriteString("};\n\n")

	return config.String()
}

func boolToOnOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
