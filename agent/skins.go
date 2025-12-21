package agent

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// profileProperty represents a single game profile property such as "textures".
type profileProperty struct {
	name      string
	value     string
	signature string
}

// Name returns the property name.
func (p profileProperty) Name() string { return p.name }

// Value returns the property value.
func (p profileProperty) Value() string { return p.value }

// Signature returns the property signature if present.
func (p profileProperty) Signature() string { return p.signature }

// SkinFetcherConfig controls how skins are resolved and cached.
type SkinFetcherConfig struct {
	AllowNetwork bool
	CacheRoot    string
	HTTPClient   *http.Client
}

// SkinFetcher implements SkinProvider with local caching and optional network lookups.
type SkinFetcher struct {
	allowNetwork bool
	cacheRoot    string
	httpClient   *http.Client
	rng          *rand.Rand
}

// NewSkinFetcher builds a SkinFetcher using the provided configuration.
func NewSkinFetcher(cfg SkinFetcherConfig) *SkinFetcher {
	cacheRoot := cfg.CacheRoot
	if cacheRoot == "" {
		cacheRoot = "skins"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &SkinFetcher{
		allowNetwork: cfg.AllowNetwork,
		cacheRoot:    cacheRoot,
		httpClient:   client,
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Get returns texture properties for the given player, loading from cache when possible
// and falling back to network or extracted skins.
func (s *SkinFetcher) Get(uuid [16]byte, name string) []profileProperty {
	// 1) cached player skin by uuid
	if props := s.loadCachedSkin(filepath.Join(s.cacheRoot, "players"), uuid, name); len(props) > 0 {
		return props
	}

	// 2) try network (if allowed)
	if s.allowNetwork {
		if props := s.fetchFromMojang(uuid, name); len(props) > 0 {
			s.persistSkin(filepath.Join(s.cacheRoot, "players"), uuid, name, props)
			return props
		}
	}

	// 3) extracted skins from client jar (final fallback)
	extractedSkinsDir := filepath.Join(s.cacheRoot, "extracted")
	if props := s.GetRandomExtractedSkin(extractedSkinsDir, uuid, name); len(props) > 0 {
		return props
	}

	// If we get here, skins need to be downloaded via Initialize()
	return nil
}

// loadCachedSkin loads properties for a specific uuid from disk.
func (s *SkinFetcher) loadCachedSkin(dir string, uuid [16]byte, name string) []profileProperty {
	uhex := hex.EncodeToString(uuid[:])
	path := filepath.Join(dir, uhex+"-"+name+".json")
	props, err := s.readPropertyFile(path)
	if err == nil && len(props) > 0 {
		return props
	}
	// also try name-based cache if uuid not set
	if uhex == strings.Repeat("00", 16) && name != "" {
		path = filepath.Join(dir, strings.ToLower(name)+".json")
		props, err = s.readPropertyFile(path)
		if err == nil && len(props) > 0 {
			return props
		}
	}
	return nil
}

// persistSkin writes properties to the players cache.
func (s *SkinFetcher) persistSkin(dir string, uuid [16]byte, name string, props []profileProperty) {
	if len(props) == 0 {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	uhex := hex.EncodeToString(uuid[:])
	filename := uhex + "-" + name
	_ = s.writePropertyFile(filepath.Join(dir, filename+".json"), props)
}

// fetchFromMojang resolves textures via Mojang session servers.
func (s *SkinFetcher) fetchFromMojang(uuid [16]byte, name string) []profileProperty {
	targetUUID := uuidHexNoDashes(uuid)
	if targetUUID == strings.Repeat("00", 32) && name != "" {
		// Resolve UUID by name first.
		u, err := s.lookupUUIDByName(name)
		if err != nil || u == "" {
			return nil
		}
		targetUUID = u
	}
	if targetUUID == strings.Repeat("00", 32) {
		return nil
	}
	url := fmt.Sprintf("https://sessionserver.mojang.com/session/minecraft/profile/%s?unsigned=false", targetUUID)
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var payload struct {
		Properties []struct {
			Name      string `json:"name"`
			Value     string `json:"value"`
			Signature string `json:"signature"`
		} `json:"properties"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil
	}
	var props []profileProperty
	for _, p := range payload.Properties {
		if p.Name == "" || p.Value == "" {
			continue
		}
		props = append(props, profileProperty{name: p.Name, value: p.Value, signature: p.Signature})
	}
	return props
}

func (s *SkinFetcher) lookupUUIDByName(name string) (string, error) {
	url := fmt.Sprintf("https://api.mojang.com/users/profiles/minecraft/%s", name)
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.ID == "" {
		return "", errors.New("empty id")
	}
	return payload.ID, nil
}

func (s *SkinFetcher) readPropertyFile(path string) ([]profileProperty, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var stored struct {
		Properties []struct {
			Name      string `json:"name"`
			Value     string `json:"value"`
			Signature string `json:"signature"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}
	var props []profileProperty
	for _, p := range stored.Properties {
		if p.Name == "" || p.Value == "" {
			continue
		}
		props = append(props, profileProperty{name: p.Name, value: p.Value, signature: p.Signature})
	}
	return props, nil
}

func (s *SkinFetcher) writePropertyFile(path string, props []profileProperty) error {
	payload := struct {
		Properties []profileProperty `json:"properties"`
	}{Properties: props}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, fs.FileMode(0o644))
}

// uuidHexNoDashes converts a UUID byte array to a hex string without dashes
func uuidHexNoDashes(uuid [16]byte) string {
	return hex.EncodeToString(uuid[:])
}

// LocalSkin represents a skin loaded from a local PNG file
type LocalSkin struct {
	Name  string // e.g., "steve", "alex"
	Model string // "slim" or "wide"
	Path  string // full path to the PNG file
}

// LoadExtractedSkins scans the skins directory for extracted PNG files
func (s *SkinFetcher) LoadExtractedSkins(skinsDir string) ([]LocalSkin, error) {
	var skins []LocalSkin

	// Scan both slim and wide directories
	for _, model := range []string{"slim", "wide"} {
		modelDir := filepath.Join(skinsDir, model)
		entries, err := os.ReadDir(modelDir)
		if err != nil {
			// Directory doesn't exist, skip
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".png") {
				continue
			}

			skinName := strings.TrimSuffix(entry.Name(), ".png")
			skins = append(skins, LocalSkin{
				Name:  skinName,
				Model: model,
				Path:  filepath.Join(modelDir, entry.Name()),
			})
		}
	}

	return skins, nil
}

// GetRandomExtractedSkin returns a random skin from the extracted skins directory
func (s *SkinFetcher) GetRandomExtractedSkin(skinsDir string, uuid [16]byte, playerName string) []profileProperty {
	skins, err := s.LoadExtractedSkins(skinsDir)
	if err != nil || len(skins) == 0 {
		return nil
	}

	// Pick a random skin
	skin := skins[s.rng.Intn(len(skins))]

	// Load the skin data and create a texture property
	return s.createPropertyFromLocalSkin(skin, uuid, playerName)
}

// createPropertyFromLocalSkin creates a texture property from a local skin PNG file
func (s *SkinFetcher) createPropertyFromLocalSkin(skin LocalSkin, uuid [16]byte, playerName string) []profileProperty {
	// Read the PNG file
	data, err := os.ReadFile(skin.Path)
	if err != nil {
		return nil
	}

	// Encode as base64
	base64Data := base64.StdEncoding.EncodeToString(data)

	// Create data URL
	dataURL := "data:image/png;base64," + base64Data

	// Build textures JSON
	profileID := uuidHexNoDashes(uuid)
	if profileID == "" || profileID == strings.Repeat("00", 32) {
		profileID = strings.Repeat("0", 32)
	}

	texturesObj := map[string]any{
		"timestamp":   time.Now().UnixMilli(),
		"profileId":   profileID,
		"profileName": playerName,
		"textures": map[string]any{
			"SKIN": map[string]any{
				"url": dataURL,
				"metadata": map[string]string{
					"model": skin.Model,
				},
			},
		},
	}

	texturesJSON, err := json.Marshal(texturesObj)
	if err != nil {
		return nil
	}

	texturesValue := base64.StdEncoding.EncodeToString(texturesJSON)

	return []profileProperty{{
		name:  "textures",
		value: texturesValue,
	}}
}

// GetExtractedSkinByName returns a specific skin by name and model type
func (s *SkinFetcher) GetExtractedSkinByName(skinsDir, skinName, model string, uuid [16]byte, playerName string) []profileProperty {
	skinPath := filepath.Join(skinsDir, model, skinName+".png")

	// Check if file exists
	if _, err := os.Stat(skinPath); err != nil {
		return nil
	}

	skin := LocalSkin{
		Name:  skinName,
		Model: model,
		Path:  skinPath,
	}

	return s.createPropertyFromLocalSkin(skin, uuid, playerName)
}
