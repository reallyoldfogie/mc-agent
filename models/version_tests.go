package models

import "os"

// VersionTest represents a Minecraft version to test against.
// This is used by both integration and unit tests.
type VersionTest struct {
	Name      string
	MCVersion string
}

// StandardVersionTests provides the standard set of Minecraft versions to test against.
// default to minimal set of versions for faster testing.  Set TEST_ALL_VERSIONS=1 to test all versions.
var StandardVersionTests = minimalStandardVersionTests

// full set of versions for comprehensive testing. Add new versions here.
var fullStandardVersionTests = []VersionTest{
	{
		Name:      "1.21.1",
		MCVersion: "1.21.1",
	},
	{
		Name:      "1.21.2",
		MCVersion: "1.21.2",
	},
	{
		Name:      "1.21.3",
		MCVersion: "1.21.3",
	},
	{
		Name:      "1.21.4",
		MCVersion: "1.21.4",
	},
	{
		Name:      "1.21.5",
		MCVersion: "1.21.5",
	},
	{
		Name:      "1.21.6",
		MCVersion: "1.21.6",
	},
	{
		Name:      "1.21.7",
		MCVersion: "1.21.7",
	},
	{
		Name:      "1.21.8",
		MCVersion: "1.21.8",
	},
}

// use minimal set of versions for faster testing. If you add a new version, change the latest version to it.
// If major changes in a protocol happen, consider adding more versions to this minimal set.
var minimalStandardVersionTests = []VersionTest{
	{
		Name:      "1.21.1",
		MCVersion: "1.21.1",
	},
	{
		Name:      "1.21.5",
		MCVersion: "1.21.5",
	},
	{
		Name:      "1.21.8",
		MCVersion: "1.21.8",
	},
}

func init() {
	if os.Getenv("TEST_ALL_VERSIONS") != "" {
		StandardVersionTests = fullStandardVersionTests
	}
}
