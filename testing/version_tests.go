package testing

import "os"

// standardVersionTests provides the standard set of Minecraft versions to test against.
// This should be used by all integration tests that spawn servers/agents.
// Version handlers are automatically detected for each version.
type versionTest struct {
	name      string
	mcVersion string
}

// default to minimal set of versions for faster testing.  Set TEST_ALL_VERSIONS=1 to test all versions.
var standardVersionTests = minimalStandardVersionTests

// full set of versions for comprehensive testing. Add new versions here.
var fullStandardVersionTests = []versionTest{
	{
		name:      "1.21.1",
		mcVersion: "1.21.1",
	},
	{
		name:      "1.21.2",
		mcVersion: "1.21.2",
	},
	{
		name:      "1.21.3",
		mcVersion: "1.21.3",
	},
	{
		name:      "1.21.4",
		mcVersion: "1.21.4",
	},
	{
		name:      "1.21.5",
		mcVersion: "1.21.5",
	},
	{
		name:      "1.21.6",
		mcVersion: "1.21.6",
	},
	{
		name:      "1.21.7",
		mcVersion: "1.21.7",
	},
	{
		name:      "1.21.8",
		mcVersion: "1.21.8",
	},
}

// use minimal set of versions for faster testing. If you add a new version, change the latest version to it.
// If major changes in a protocol happen, consider adding more versions to this minimal set.
var minimalStandardVersionTests = []versionTest{
	{
		name:      "1.21.1",
		mcVersion: "1.21.1",
	},
	{
		name:      "1.21.5",
		mcVersion: "1.21.5",
	},
	{
		name:      "1.21.8",
		mcVersion: "1.21.8",
	},
}

func init() {
	if os.Getenv("TEST_ALL_VERSIONS") != "" {
		standardVersionTests = fullStandardVersionTests
	}
}
