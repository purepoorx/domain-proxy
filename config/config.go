package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Rule struct {
	Source string `yaml:"source"`
	Target string `yaml:"target,omitempty"`
}

type RewriteTarget struct {
	Host         string
	InjectHeader bool // true = inject X-Target-Host with original source
}

type TLSConfig struct {
	CACert string `yaml:"ca_cert"`
	CAKey  string `yaml:"ca_key"`
}

type ProxyConfig struct {
	Addr string `yaml:"addr"`
}

type Config struct {
	Proxy         ProxyConfig `yaml:"proxy"`
	TLS           TLSConfig   `yaml:"tls"`
	DefaultTarget string      `yaml:"default_target"`
	Rules         []Rule      `yaml:"rules"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config file: %w", err)
	}

	if cfg.Proxy.Addr == "" {
		cfg.Proxy.Addr = "127.0.0.1:1080"
	}
	if cfg.TLS.CACert == "" {
		cfg.TLS.CACert = "./certs/ca.crt"
	}
	if cfg.TLS.CAKey == "" {
		cfg.TLS.CAKey = "./certs/ca.key"
	}
	if cfg.DefaultTarget == "" {
		cfg.DefaultTarget = "domain-proxy.okkkk.tk"
	}

	return cfg, nil
}

func (c Config) BuildRuleMap() map[string]RewriteTarget {
	m := make(map[string]RewriteTarget, len(c.Rules))
	for _, r := range c.Rules {
		if r.Target == "" {
			m[r.Source] = RewriteTarget{
				Host:         c.DefaultTarget,
				InjectHeader: true,
			}
		} else {
			m[r.Source] = RewriteTarget{
				Host:         r.Target,
				InjectHeader: false,
			}
		}
	}
	return m
}
