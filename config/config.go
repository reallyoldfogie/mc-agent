package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const DefaultClientID = "88650e7e-efee-4857-b9a9-cf580a00ef43" // token from github.com/maxsupermanhd/go-mc-ms-auth

type AppConfig struct {
	ClientID string `yaml:"client_id"`
	CacheDir string `yaml:"cache_dir"`
}

// Load reads the config from the specified file path.
// If the file doesn't exist or doesn't contain a clientID, returns the default clientID.
func Load(filePath string) (AppConfig, error) {
	var cfg AppConfig
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return default
			return AppConfig{ClientID: DefaultClientID, CacheDir: "."}, nil
		}
		return cfg, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse config file: %w", err)
	}

	if cfg.CacheDir == "" {
		cfg.CacheDir = "."
	}

	if cfg.CacheDir != "." {
		if err := os.MkdirAll(cfg.CacheDir, 0700); err != nil {
			return cfg, fmt.Errorf("failed to create cache directory: %w", err)
		}
	}

	// If clientID is empty in config, use default
	if cfg.ClientID == "" {
		cfg.ClientID = DefaultClientID
	}

	return cfg, nil
}
