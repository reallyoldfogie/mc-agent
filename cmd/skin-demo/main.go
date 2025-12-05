package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/reallyoldfogie/mc-agent/agent"
)

func main() {
	version := flag.String("version", "1.21.5", "Minecraft version")
	listOnly := flag.Bool("list", false, "Only list available skins, don't test loading")
	skinName := flag.String("skin", "", "Load a specific skin by name (e.g., 'steve', 'alex')")
	model := flag.String("model", "wide", "Skin model: 'slim' or 'wide'")
	flag.Parse()

	// Create skin manager
	log.Println("Creating skin manager...")
	skinMgr := agent.NewSkinManager(agent.SkinManagerConfig{
		MinecraftVersion:  *version,
		CacheRoot:         "./skins",
		AllowNetwork:      true,
		ClientJarCacheDir: "./data/client-cache",
	})

	// Initialize (downloads and extracts if needed)
	log.Printf("Initializing skin system for Minecraft %s...", *version)
	if err := skinMgr.Initialize(*version); err != nil {
		log.Fatalf("Failed to initialize skin manager: %v", err)
	}
	log.Println("✓ Skin system initialized successfully")

	// List available skins
	skins, err := skinMgr.ListAvailableSkins()
	if err != nil {
		log.Fatalf("Failed to list skins: %v", err)
	}

	fmt.Printf("\n=== Available Skins (%d total) ===\n", len(skins))
	slimCount := 0
	wideCount := 0
	for _, skin := range skins {
		fmt.Printf("  • %s (%s) - %s\n", skin.Name, skin.Model, skin.Path)
		if skin.Model == "slim" {
			slimCount++
		} else {
			wideCount++
		}
	}
	fmt.Printf("\nSummary: %d slim, %d wide\n", slimCount, wideCount)

	if *listOnly {
		os.Exit(0)
	}

	// Test loading skins
	fmt.Println("\n=== Testing Skin Loading ===")
	testUUID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	testName := "TestPlayer"

	if *skinName != "" {
		// Load specific skin
		fmt.Printf("\nLoading specific skin: %s (%s)\n", *skinName, *model)
		props := skinMgr.GetSkinByName(*skinName, *model, testUUID, testName)
		if len(props) == 0 {
			log.Printf("❌ Failed to load skin %s (%s)", *skinName, *model)
		} else {
			fmt.Printf("✓ Loaded skin successfully\n")
			printSkinProperties(props)
		}
	} else {
		// Load random skin
		fmt.Println("\nLoading random skin...")
		props := skinMgr.GetRandomExtractedSkin(testUUID, testName)
		if len(props) == 0 {
			log.Println("❌ Failed to load random skin")
		} else {
			fmt.Println("✓ Loaded random skin successfully")
			printSkinProperties(props)
		}
	}

	fmt.Println("\n=== Demo Complete ===")
}

func printSkinProperties(props []agent.ProfileProperty) {
	for _, prop := range props {
		fmt.Printf("\nProperty: %s\n", prop.Name)
		fmt.Printf("  Value Length: %d bytes\n", len(prop.Value))
		if prop.Signature != "" {
			fmt.Printf("  Signature: %s\n", prop.Signature)
		}
		// Don't print the full value as it's very long (base64-encoded PNG)
		if len(prop.Value) > 100 {
			fmt.Printf("  Value (preview): %s...\n", prop.Value[:100])
		} else {
			fmt.Printf("  Value: %s\n", prop.Value)
		}
	}
}
