package tcwh

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen   string       `yaml:"listen"`
	LogDebug bool         `yaml:"log-debug"`
	HTMLPath string       `yaml:"html-path"`
	Auth     AuthConfig   `yaml:"auth"`
	Twitch   TwitchConfig `yaml:"twitch"`
	DB       DBConfig     `yaml:"db"`
}

type AuthConfig struct {
	Secret   string `yaml:"secret"`
	Issuer   string `yaml:"issuer"`
	Audience string `yaml:"audience"`
}

type TwitchConfig struct {
	ClientID      string `yaml:"client-id"`
	ClientSecret  string `yaml:"client-secret"`
	TwitchAuthURL string `yaml:"twitch-auth-url"`
	RedirURI      string `yaml:"redirect-uri"`
	WebhookCBURL  string `yaml:"webhook-cb-url"`
}

type DBConfig struct {
	Path string `yaml:"path"`
}

func (c *Config) Validate() error {
	for k, v := range map[string]string{
		"listen":              c.Listen,
		"html-path":           c.HTMLPath,
		"auth.secret":         c.Auth.Secret,
		"auth.issuer":         c.Auth.Issuer,
		"auth.audience":       c.Auth.Audience,
		"oauth.client-id":     c.Twitch.ClientID,
		"oauth.client-secret": c.Twitch.ClientSecret,
		"oauth.redirect-uri":  c.Twitch.RedirURI,
	} {
		if v == "" {
			return fmt.Errorf("missing required value: %s", k)
		}
	}
	return nil
}

func LoadConfig(cfgPath string) (*Config, error) {
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("reading: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling: %w", err)
	}
	return &cfg, nil
}
