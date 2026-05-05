package versions

import (
	"testing"

	v1_21_1 "github.com/reallyoldfogie/mc-agent/handler_versions/v1_21_1"
	v1_21_10 "github.com/reallyoldfogie/mc-agent/handler_versions/v1_21_10"
	v1_21_11 "github.com/reallyoldfogie/mc-agent/handler_versions/v1_21_11"
	v1_21_2 "github.com/reallyoldfogie/mc-agent/handler_versions/v1_21_2"
	v1_21_3 "github.com/reallyoldfogie/mc-agent/handler_versions/v1_21_3"
	v1_21_4 "github.com/reallyoldfogie/mc-agent/handler_versions/v1_21_4"
	v1_21_5 "github.com/reallyoldfogie/mc-agent/handler_versions/v1_21_5"
	v1_21_6 "github.com/reallyoldfogie/mc-agent/handler_versions/v1_21_6"
	v1_21_7 "github.com/reallyoldfogie/mc-agent/handler_versions/v1_21_7"
	v1_21_8 "github.com/reallyoldfogie/mc-agent/handler_versions/v1_21_8"
	v1_21_9 "github.com/reallyoldfogie/mc-agent/handler_versions/v1_21_9"
	"github.com/reallyoldfogie/mc-protocol-go/data/versions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandlerProtocolVersions verifies that each version handler's ProtocolVersion
// constant matches the corresponding version in mc-protocol-go's VersionProtocol map.
func TestHandlerProtocolVersions(t *testing.T) {
	tests := []struct {
		name    string
		version string
		handler interface {
			ProtocolVersion() uint
		}
	}{
		{"1.21.1", "1.21.1", v1_21_1.NewHandler(nil)},
		{"1.21.2", "1.21.2", v1_21_2.NewHandler(nil)},
		{"1.21.3", "1.21.3", v1_21_3.NewHandler(nil)},
		{"1.21.4", "1.21.4", v1_21_4.NewHandler(nil)},
		{"1.21.5", "1.21.5", v1_21_5.NewHandler(nil)},
		{"1.21.6", "1.21.6", v1_21_6.NewHandler(nil)},
		{"1.21.7", "1.21.7", v1_21_7.NewHandler(nil)},
		{"1.21.8", "1.21.8", v1_21_8.NewHandler(nil)},
		{"1.21.9", "1.21.9", v1_21_9.NewHandler(nil)},
		{"1.21.10", "1.21.10", v1_21_10.NewHandler(nil)},
		{"1.21.11", "1.21.11", v1_21_11.NewHandler(nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectedProtocolVersion, exists := versions.VersionProtocol[tt.version]
			require.True(t, exists, "Version %s not found in mc-protocol-go VersionProtocol map", tt.version)

			actualProtocolVersion := tt.handler.ProtocolVersion()

			assert.Equal(t, expectedProtocolVersion, actualProtocolVersion,
				"ProtocolVersion mismatch for %s", tt.version)
		})
	}
}
