package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const DefaultClientID = "88650e7e-efee-4857-b9a9-cf580a00ef43" // token from github.com/maxsupermanhd/go-mc-ms-auth

type AppConfig struct {
	ClientID string `yaml:"client_id"`
}

// Load reads the config from the specified file path.
// If the file doesn't exist or doesn't contain a clientID, returns the default clientID.
func Load(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return default
			return DefaultClientID, nil
		}
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("failed to parse config file: %w", err)
	}

	// If clientID is empty in config, use default
	if cfg.ClientID == "" {
		return DefaultClientID, nil
	}

	return cfg.ClientID, nil
}
