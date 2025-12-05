package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkinManager_Initialize(t *testing.T) {
	// Use a temporary directory for testing
	tempDir := t.TempDir()

	manager := NewSkinManager(SkinManagerConfig{
		MinecraftVersion:  "1.21.5",
		CacheRoot:         filepath.Join(tempDir, "skins"),
		ClientJarCacheDir: filepath.Join(tempDir, "client-cache"),
		AllowNetwork:      true,
	})

	// Initialize should download and extract skins
	if err := manager.Initialize("1.21.5"); err != nil {
		t.Fatalf("Failed to initialize skin manager: %v", err)
	}

	// Check that skins directory was created
	skinsDir := filepath.Join(tempDir, "skins", "extracted")
	if _, err := os.Stat(skinsDir); os.IsNotExist(err) {
		t.Fatalf("Skins directory was not created: %s", skinsDir)
	}

	// Check for slim and wide directories
	for _, model := range []string{"slim", "wide"} {
		modelDir := filepath.Join(skinsDir, model)
		if _, err := os.Stat(modelDir); os.IsNotExist(err) {
			t.Errorf("Model directory was not created: %s", modelDir)
		}

		// Check that there are some PNG files
		entries, err := os.ReadDir(modelDir)
		if err != nil {
			t.Errorf("Failed to read model directory %s: %v", modelDir, err)
			continue
		}

		hasPNG := false
		for _, entry := range entries {
			if filepath.Ext(entry.Name()) == ".png" {
				hasPNG = true
				break
			}
		}

		if !hasPNG {
			t.Errorf("No PNG files found in model directory: %s", modelDir)
		}
	}
}

func TestSkinManager_ListAvailableSkins(t *testing.T) {
	tempDir := t.TempDir()

	manager := NewSkinManager(SkinManagerConfig{
		MinecraftVersion:  "1.21.5",
		CacheRoot:         filepath.Join(tempDir, "skins"),
		ClientJarCacheDir: filepath.Join(tempDir, "client-cache"),
		AllowNetwork:      true,
	})

	if err := manager.Initialize("1.21.5"); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	skins, err := manager.ListAvailableSkins()
	if err != nil {
		t.Fatalf("Failed to list skins: %v", err)
	}

	if len(skins) == 0 {
		t.Fatal("No skins found")
	}

	t.Logf("Found %d skins:", len(skins))
	for _, skin := range skins {
		t.Logf("  - %s (%s)", skin.Name, skin.Model)
	}

	// Verify we have both slim and wide skins
	hasSlim := false
	hasWide := false
	for _, skin := range skins {
		if skin.Model == "slim" {
			hasSlim = true
		}
		if skin.Model == "wide" {
			hasWide = true
		}
	}

	if !hasSlim {
		t.Error("No slim model skins found")
	}
	if !hasWide {
		t.Error("No wide model skins found")
	}
}

func TestSkinManager_GetRandomSkin(t *testing.T) {
	tempDir := t.TempDir()

	manager := NewSkinManager(SkinManagerConfig{
		MinecraftVersion:  "1.21.5",
		CacheRoot:         filepath.Join(tempDir, "skins"),
		ClientJarCacheDir: filepath.Join(tempDir, "client-cache"),
		AllowNetwork:      true,
	})

	if err := manager.Initialize("1.21.5"); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Get a random skin
	uuid := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	props := manager.GetRandomExtractedSkin(uuid, "TestPlayer")

	if len(props) == 0 {
		t.Fatal("No skin properties returned")
	}

	// Check that we got a textures property
	hasTextures := false
	for _, prop := range props {
		if prop.Name == "textures" {
			hasTextures = true
			if prop.Value == "" {
				t.Error("Textures value is empty")
			}
		}
	}

	if !hasTextures {
		t.Error("No textures property found")
	}
}

func TestSkinManager_GetSkinByName(t *testing.T) {
	tempDir := t.TempDir()

	manager := NewSkinManager(SkinManagerConfig{
		MinecraftVersion:  "1.21.5",
		CacheRoot:         filepath.Join(tempDir, "skins"),
		ClientJarCacheDir: filepath.Join(tempDir, "client-cache"),
		AllowNetwork:      true,
	})

	if err := manager.Initialize("1.21.5"); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Try to get Steve skin
	uuid := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	props := manager.GetSkinByName("steve", "wide", uuid, "TestPlayer")

	if len(props) == 0 {
		t.Fatal("Failed to get steve skin")
	}

	hasTextures := false
	for _, prop := range props {
		if prop.Name == "textures" {
			hasTextures = true
		}
	}

	if !hasTextures {
		t.Error("No textures property found for steve skin")
	}
}

func TestSkinManager_CachedDownload(t *testing.T) {
	tempDir := t.TempDir()

	manager := NewSkinManager(SkinManagerConfig{
		MinecraftVersion:  "1.21.5",
		CacheRoot:         filepath.Join(tempDir, "skins"),
		ClientJarCacheDir: filepath.Join(tempDir, "client-cache"),
		AllowNetwork:      true,
	})

	// First initialization - should download
	if err := manager.Initialize("1.21.5"); err != nil {
		t.Fatalf("Failed first initialization: %v", err)
	}

	// Second initialization - should use cache
	if err := manager.Initialize("1.21.5"); err != nil {
		t.Fatalf("Failed second initialization: %v", err)
	}

	// Verify skins are still available
	skins, err := manager.ListAvailableSkins()
	if err != nil {
		t.Fatalf("Failed to list skins after second init: %v", err)
	}

	if len(skins) == 0 {
		t.Fatal("No skins found after second initialization")
	}
}
