package agent

// Import versions package to register version handlers for tests.
// This ensures that common.GetVersionHandler() can find handlers when tests specify a version.
import (
	_ "github.com/reallyoldfogie/mc-agent/versions"
)
