package agent

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/reallyoldfogie/mc-protocol-go/models"
)

const (
	versionManifestV2 = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
)

// JarDownloaderConfig controls where files are cached and how long to keep them
type JarDownloaderConfig struct {
	CacheDir   string
	HTTPClient *http.Client
	TTLDays    int // how many days before re-downloading
}

// JarDownloader downloads and extracts Minecraft client jars
type JarDownloader struct {
	cacheDir   string
	httpClient *http.Client
	ttlDays    int
}

// NewJarDownloader creates a new downloader with the given configuration
func NewJarDownloader(cfg JarDownloaderConfig) *JarDownloader {
	cacheDir := cfg.CacheDir
	if cacheDir == "" {
		cacheDir = "./data/client-cache"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	ttlDays := cfg.TTLDays
	if ttlDays <= 0 {
		ttlDays = 30 // default 30 days
	}
	return &JarDownloader{
		cacheDir:   cacheDir,
		httpClient: client,
		ttlDays:    ttlDays,
	}
}

// GetVersionMetadata fetches metadata for a specific Minecraft version from Mojang
func (d *JarDownloader) GetVersionMetadata(mcVersion string) (*models.VersionMetaData, error) {
	var manifest struct {
		Latest struct {
			Release string `json:"release"`
		} `json:"latest"`
		Versions []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"versions"`
	}

	manifestRes, err := d.httpClient.Get(versionManifestV2)
	if err != nil {
		return nil, fmt.Errorf("could not reach version manifest: %w", err)
	}
	defer manifestRes.Body.Close()

	if err := json.NewDecoder(manifestRes.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("could not decode manifest JSON: %w", err)
	}

	if strings.ToLower(mcVersion) == "latest" {
		mcVersion = manifest.Latest.Release
	}

	var versionURL string
	for _, v := range manifest.Versions {
		if mcVersion == v.ID {
			versionURL = v.URL
			break
		}
	}
	if versionURL == "" {
		return nil, fmt.Errorf("could not find version %s in manifest", mcVersion)
	}

	var versionMetadata models.VersionMetaData
	versionRes, err := d.httpClient.Get(versionURL)
	if err != nil {
		return nil, fmt.Errorf("could not reach versionURL: %w", err)
	}
	defer versionRes.Body.Close()

	if err := json.NewDecoder(versionRes.Body).Decode(&versionMetadata); err != nil {
		return nil, fmt.Errorf("could not decode version JSON: %w", err)
	}

	return &versionMetadata, nil
}

// DownloadClientJar downloads the client jar for a specific version if not already cached
func (d *JarDownloader) DownloadClientJar(mcVersion string) (string, error) {
	versionDir := filepath.Join(d.cacheDir, mcVersion)
	clientJarPath := filepath.Join(versionDir, "client.jar")

	// Check if already cached and still valid
	if d.isCacheValid(clientJarPath) {
		return clientJarPath, nil
	}

	// Create cache directory
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Get version metadata
	metadata, err := d.GetVersionMetadata(mcVersion)
	if err != nil {
		return "", fmt.Errorf("failed to get version metadata: %w", err)
	}

	// Download client jar
	if err := d.downloadFile(metadata.Downloads.Client.URL, clientJarPath); err != nil {
		return "", fmt.Errorf("failed to download client.jar: %w", err)
	}

	return clientJarPath, nil
}

// ExtractSkins extracts player skin files from the client jar to the specified directory
func (d *JarDownloader) ExtractSkins(clientJarPath, targetDir string) error {
	// Check if skins are already extracted
	if d.areSkinsExtracted(targetDir) {
		return nil
	}

	// Open the jar file (it's a zip)
	r, err := zip.OpenReader(clientJarPath)
	if err != nil {
		return fmt.Errorf("failed to open client jar: %w", err)
	}
	defer r.Close()

	// Extract files from assets/minecraft/textures/entity/player/
	skinPrefix := "assets/minecraft/textures/entity/player/"
	extracted := 0

	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, skinPrefix) || !strings.HasSuffix(f.Name, ".png") {
			continue
		}

		// Get relative path after the prefix
		relPath := strings.TrimPrefix(f.Name, skinPrefix)
		targetPath := filepath.Join(targetDir, relPath)

		// Create directory structure
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		// Extract file
		if err := d.extractFile(f, targetPath); err != nil {
			return fmt.Errorf("failed to extract %s: %w", f.Name, err)
		}
		extracted++
	}

	if extracted == 0 {
		return fmt.Errorf("no skin files found in client jar")
	}

	return nil
}

// EnsureSkinsAvailable downloads the client jar and extracts skins if they don't exist
func (d *JarDownloader) EnsureSkinsAvailable(mcVersion, skinsDir string) error {
	// Check if skins already exist
	if d.areSkinsExtracted(skinsDir) {
		return nil
	}

	// Download client jar
	clientJarPath, err := d.DownloadClientJar(mcVersion)
	if err != nil {
		return fmt.Errorf("failed to download client jar: %w", err)
	}

	// Extract skins
	if err := d.ExtractSkins(clientJarPath, skinsDir); err != nil {
		return fmt.Errorf("failed to extract skins: %w", err)
	}

	return nil
}

// downloadFile downloads a file from a URL to the target path
func (d *JarDownloader) downloadFile(url, targetPath string) error {
	resp, err := d.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to get url (%s): %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	outFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	return nil
}

// extractFile extracts a single file from a zip archive
func (d *JarDownloader) extractFile(f *zip.File, targetPath string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("failed to open file in archive: %w", err)
	}
	defer rc.Close()

	outFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, rc); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}

// isCacheValid checks if a cached file exists and is within the TTL
func (d *JarDownloader) isCacheValid(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	ttlDuration := time.Duration(d.ttlDays*24) * time.Hour
	return time.Since(info.ModTime()) < ttlDuration
}

// areSkinsExtracted checks if skins have been extracted to the target directory
func (d *JarDownloader) areSkinsExtracted(skinsDir string) bool {
	// Check if both slim and wide directories exist with at least one skin each
	slimDir := filepath.Join(skinsDir, "slim")
	wideDir := filepath.Join(skinsDir, "wide")

	slimEntries, slimErr := os.ReadDir(slimDir)
	wideEntries, wideErr := os.ReadDir(wideDir)

	if slimErr != nil || wideErr != nil {
		return false
	}

	// Check if there are any .png files
	hasSkinFiles := func(entries []os.DirEntry) bool {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".png") {
				return true
			}
		}
		return false
	}

	return hasSkinFiles(slimEntries) && hasSkinFiles(wideEntries)
}
