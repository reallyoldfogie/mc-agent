// Package versions provides a central import point for all version-specific packet handlers.
// Importing this package will trigger registration of all supported version handlers.
package versions

import (
	// Import all version-specific packages to trigger their init() functions
	_ "github.com/reallyoldfogie/mc-agent/versions/v1_21_1"
	_ "github.com/reallyoldfogie/mc-agent/versions/v1_21_2"
	_ "github.com/reallyoldfogie/mc-agent/versions/v1_21_3"
	_ "github.com/reallyoldfogie/mc-agent/versions/v1_21_4"
	_ "github.com/reallyoldfogie/mc-agent/versions/v1_21_5"
	_ "github.com/reallyoldfogie/mc-agent/versions/v1_21_6"
	_ "github.com/reallyoldfogie/mc-agent/versions/v1_21_7"
	_ "github.com/reallyoldfogie/mc-agent/versions/v1_21_8"
	_ "github.com/reallyoldfogie/mc-agent/versions/v1_21_9"

	_ "github.com/reallyoldfogie/mc-agent/versions/v1_21_10"
	_ "github.com/reallyoldfogie/mc-agent/versions/v1_21_11"
)
