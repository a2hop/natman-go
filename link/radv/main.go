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

type Config struct {
	Interfaces map[string]*RadvConfig
}

func NewRadvConfig(cfg config.RadvConfig) *RadvConfig {
	radv := &RadvConfig{
		Enabled:         cfg.Enabled,
		MinAdvInterval:  cfg.MinAdvInterval,
		MaxAdvInterval:  cfg.MaxAdvInterval,
		DefaultLifetime: cfg.DefaultLifetime,
		Dhcp:            cfg.Dhcp,
		Include:         cfg.Include,
	}

	// Set defaults
	if radv.MinAdvInterval == 0 {
		radv.MinAdvInterval = 30
	}
	if radv.MaxAdvInterval == 0 {
		radv.MaxAdvInterval = 60
	}
	if radv.DefaultLifetime == 0 {
		radv.DefaultLifetime = 180
	}

	// Convert prefixes
	for _, prefix := range cfg.Prefixes {
		pc := PrefixConfig{
			Prefix:            prefix.Prefix,
			Mode:              prefix.Mode,
			OnLink:            prefix.OnLink,
			Autonomous:        prefix.Autonomous,
			ValidLifetime:     prefix.ValidLifetime,
			PreferredLifetime: prefix.PreferredLifetime,
			RouterAddr:        prefix.RouterAddr,
		}

		// Set defaults
		if pc.Mode == "" {
			pc.Mode = "slaac"
		}
		if pc.ValidLifetime == 0 {
			pc.ValidLifetime = 1800
		}
		if pc.PreferredLifetime == 0 {
			pc.PreferredLifetime = 900
		}

		radv.Prefixes = append(radv.Prefixes, pc)
	}

	// Convert routes
	for _, route := range cfg.Routes {
		rc := RouteConfig{
			Prefix:     route.Prefix,
			Preference: route.Preference,
			Metric:     route.Metric,
			Lifetime:   route.Lifetime,
		}

		// Set defaults
		if rc.Preference == "" {
			rc.Preference = "medium"
		}
		if rc.Metric == 0 {
			rc.Metric = 100
		}
		if rc.Lifetime == 0 {
			rc.Lifetime = 3600
		}

		radv.Routes = append(radv.Routes, rc)
	}

	return radv
}

func (r *RadvConfig) GenerateConfig(interfaceName string) string {
	if !r.Enabled {
		return ""
	}

	var config strings.Builder

	config.WriteString(fmt.Sprintf("interface %s {\n", interfaceName))
	config.WriteString(fmt.Sprintf("    AdvSendAdvert on;\n"))
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
		config.WriteString(fmt.Sprintf("        AdvValidLifetime %d;\n", prefix.ValidLifetime))
		config.WriteString(fmt.Sprintf("        AdvPreferredLifetime %d;\n", prefix.PreferredLifetime))
		if prefix.RouterAddr {
			config.WriteString("        AdvRouterAddr on;\n")
		}
		config.WriteString("    };\n")
	}

	// Add routes
	for _, route := range r.Routes {
		config.WriteString(fmt.Sprintf("    route %s {\n", route.Prefix))
		config.WriteString(fmt.Sprintf("        AdvRoutePreference %s;\n", route.Preference))
		config.WriteString(fmt.Sprintf("        AdvRouteLifetime %d;\n", route.Lifetime))
		config.WriteString("    };\n")
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
