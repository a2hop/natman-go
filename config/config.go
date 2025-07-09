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
	Pair []interface{} `yaml:"pair"` // [public, private] or [public, private, preference, lifetime]
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
	Enabled     bool                  `yaml:"enabled"`
	AdvInterval []int                 `yaml:"adv-interval"` // [min, max]
	Lifetime    int                   `yaml:"lifetime"`
	Dhcp        bool                  `yaml:"dhcp"`
	Prefixes    []PrefixConfigCompact `yaml:"prefixes"`
	Routes      []RouteArray          `yaml:"routes"`
	RDNSS       []RDNSSConfigCompact  `yaml:"rdnss"`
	Include     []string              `yaml:"include"`
}

type PrefixConfigCompact struct {
	Prefix   string `yaml:"prefix"`
	OnLink   bool   `yaml:"on-link"`
	Auto     bool   `yaml:"auto"`     // changed from autonomous
	AdvAddr  bool   `yaml:"adv-addr"` // changed from router-addr
	Lifetime []int  `yaml:"lifetime"` // [valid, preferred]
}

type RouteArray struct {
	Route []interface{} `yaml:"route"` // [prefix, preference, lifetime]
}

type RDNSSConfigCompact struct {
	Server   []string `yaml:"server"`
	Lifetime int      `yaml:"lifetime"`
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
