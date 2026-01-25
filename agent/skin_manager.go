package agent

import (
	"fmt"
	"path/filepath"
)

// SkinManagerConfig contains configuration for the complete skin management system
type SkinManagerConfig struct {
	// MinecraftVersion is the version to download skins for (e.g., "1.21.5")
	MinecraftVersion string

	// CacheRoot is the base directory for all skin-related files (default: "./skins")
	CacheRoot string

	// AllowNetwork enables fetching skins from Mojang's API
	AllowNetwork bool

	// ClientJarCacheDir is where downloaded client jars are stored (default: "./data/client-cache")
	ClientJarCacheDir string
}

// SkinManager handles downloading, extracting, and serving player skins
type SkinManager struct {
	downloader  *JarDownloader
	fetcher     *SkinFetcher
	skinsDir    string
	initialized bool
}

// NewSkinManager creates a new skin manager with the given configuration
func NewSkinManager(cfg SkinManagerConfig) *SkinManager {
	if cfg.CacheRoot == "" {
		cfg.CacheRoot = "./skins"
	}
	if cfg.ClientJarCacheDir == "" {
		cfg.ClientJarCacheDir = "./data/client-cache"
	}

	downloader := NewJarDownloader(JarDownloaderConfig{
		CacheDir: cfg.ClientJarCacheDir,
	})

	fetcher := NewSkinFetcher(SkinFetcherConfig{
		AllowNetwork: cfg.AllowNetwork,
		CacheRoot:    cfg.CacheRoot,
	})

	return &SkinManager{
		downloader:  downloader,
		fetcher:     fetcher,
		skinsDir:    filepath.Join(cfg.CacheRoot, "extracted"),
		initialized: false,
	}
}

// Initialize ensures skins are downloaded and extracted
func (sm *SkinManager) Initialize(minecraftVersion string) error {
	if sm.initialized {
		return nil
	}

	// Ensure skins are available (download and extract if needed)
	if err := sm.downloader.EnsureSkinsAvailable(minecraftVersion, sm.skinsDir); err != nil {
		return fmt.Errorf("failed to ensure skins available: %w", err)
	}

	sm.initialized = true
	return nil
}

// GetSkinForPlayer returns texture properties for a player
func (sm *SkinManager) GetSkinForPlayer(uuid [16]byte, name string) []ProfileProperty {
	props := sm.fetcher.Get(uuid, name)

	// Convert internal profileProperty to exported ProfileProperty
	result := make([]ProfileProperty, len(props))
	for i, p := range props {
		result[i] = ProfileProperty{
			Name:      p.Name,
			Value:     p.Value,
			Signature: p.Signature,
		}
	}
	return result
}

// GetRandomExtractedSkin returns a random skin from the extracted skins
func (sm *SkinManager) GetRandomExtractedSkin(uuid [16]byte, name string) []ProfileProperty {
	props := sm.fetcher.GetRandomExtractedSkin(sm.skinsDir, uuid, name)

	result := make([]ProfileProperty, len(props))
	for i, p := range props {
		result[i] = ProfileProperty{
			Name:      p.Name,
			Value:     p.Value,
			Signature: p.Signature,
		}
	}
	return result
}

// GetSkinByName returns a specific skin by name and model
func (sm *SkinManager) GetSkinByName(skinName, model string, uuid [16]byte, playerName string) []ProfileProperty {
	props := sm.fetcher.GetExtractedSkinByName(sm.skinsDir, skinName, model, uuid, playerName)

	result := make([]ProfileProperty, len(props))
	for i, p := range props {
		result[i] = ProfileProperty{
			Name:      p.Name,
			Value:     p.Value,
			Signature: p.Signature,
		}
	}
	return result
}

// ListAvailableSkins returns all skins that have been extracted
func (sm *SkinManager) ListAvailableSkins() ([]LocalSkin, error) {
	return sm.fetcher.LoadExtractedSkins(sm.skinsDir)
}
