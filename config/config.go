package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Network NetworkConfig `yaml:"network"`
}

type NetworkConfig struct {
	Links map[string]LinkConfig `yaml:"links"`
}

type LinkConfig struct {
	Netmap6 map[string]Netmap6Config `yaml:"netmap6"`
	Nat66   *Nat66Config             `yaml:"nat66,omitempty"`
	Nat44   *Nat44Config             `yaml:"nat44,omitempty"`
	Radv    *RadvConfig              `yaml:"radv,omitempty"`
}

type Netmap6Config struct {
	Enabled bool      `yaml:"enabled"`
	PfxPub  string    `yaml:"pfx-pub,omitempty"`
	PfxPriv string    `yaml:"pfx-priv,omitempty"`
	Maps    []MapPair `yaml:"maps"`
}

type MapPair struct {
	Pair [2]string `yaml:"pair"`
}

type Nat66Config struct {
	Enabled     bool     `yaml:"enabled"`
	MssClamping bool     `yaml:"mss-clamping"`
	Mss         int      `yaml:"mss"`
	Origins     []string `yaml:"origins"`
}

type Nat44Config struct {
	Enabled     bool     `yaml:"enabled"`
	MssClamping bool     `yaml:"mss-clamping"`
	Mss         int      `yaml:"mss"`
	Origins     []string `yaml:"origins"`
}

type RadvConfig struct {
	Enabled         bool           `yaml:"enabled"`
	MinAdvInterval  int            `yaml:"min-adv-interval"`
	MaxAdvInterval  int            `yaml:"max-adv-interval"`
	DefaultLifetime int            `yaml:"default-lifetime"`
	Dhcp            bool           `yaml:"dhcp"`
	Prefixes        []PrefixConfig `yaml:"prefixes"`
	Routes          []RouteConfig  `yaml:"routes"`
	Include         []string       `yaml:"include"`
}

type PrefixConfig struct {
	Prefix            string `yaml:"prefix"`
	Mode              string `yaml:"mode"`
	OnLink            bool   `yaml:"on-link"`
	Autonomous        bool   `yaml:"autonomous"`
	ValidLifetime     int    `yaml:"valid-lifetime"`
	PreferredLifetime int    `yaml:"preferred-lifetime"`
	RouterAddr        bool   `yaml:"router-addr"`
}

type RouteConfig struct {
	Prefix     string `yaml:"prefix"`
	Preference string `yaml:"preference"`
	Metric     int    `yaml:"metric"`
	Lifetime   int    `yaml:"lifetime"`
}

func ParseConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
