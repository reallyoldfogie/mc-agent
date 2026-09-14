package agent

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/config"
)

func TestCredCachePathPrefersExplicitCredCacheFile(t *testing.T) {
	got := credCachePath(config.AuthSettings{CacheDir: ".", CredCacheFile: "/secure/daze.credCacheFile"})
	if want := "/secure/daze.credCacheFile"; got != want {
		t.Fatalf("credCachePath() = %q, want %q (explicit CredCacheFile must win over CacheDir)", got, want)
	}
}

func TestCredCachePathFallsBackToCacheDirWhenUnset(t *testing.T) {
	got := credCachePath(config.AuthSettings{CacheDir: "/some/dir"})
	if want := "/some/dir/.credCacheFile"; got != want {
		t.Fatalf("credCachePath() = %q, want %q", got, want)
	}
}

func TestResolveAuthOfflineSkipsCredCacheEntirely(t *testing.T) {
	auth, err := ResolveAuth(true, "Daze", "uuid-1", "token-1", config.AuthSettings{})
	if err != nil {
		t.Fatalf("ResolveAuth offline: %v", err)
	}
	if auth.Name != "Daze" || auth.UUID != "uuid-1" || auth.AccessToken != "token-1" {
		t.Fatalf("ResolveAuth offline = %+v, want name/uuid/token passed through unchanged", auth)
	}
}

// TestResolveAuthOfflineRejectsNameOverMaxLength regression-tests
// docs/bugs/offline-login-hello-packet-decode-disconnect.md's actual root
// cause directly: "RSITrainSmokeTest" (17 characters, one over
// usernameMaxLength) is the exact name that produced the bug's second
// occurrence.
func TestResolveAuthOfflineRejectsNameOverMaxLength(t *testing.T) {
	_, err := ResolveAuth(true, "RSITrainSmokeTest", "uuid-1", "token-1", config.AuthSettings{})
	if err == nil {
		t.Fatalf("ResolveAuth offline with a 17-character name: want error, got nil")
	}
}

// TestResolveAuthOfflineAcceptsNameAtMaxLength guards the boundary the
// other way — usernameMaxLength itself (16 characters) must still be
// accepted, not rejected off-by-one.
func TestResolveAuthOfflineAcceptsNameAtMaxLength(t *testing.T) {
	name := "SixteenCharsLong" // exactly 16 characters
	if got := len([]rune(name)); got != usernameMaxLength {
		t.Fatalf("test fixture %q is %d characters, want exactly %d", name, got, usernameMaxLength)
	}
	auth, err := ResolveAuth(true, name, "uuid-1", "token-1", config.AuthSettings{})
	if err != nil {
		t.Fatalf("ResolveAuth offline with a %d-character name: %v", usernameMaxLength, err)
	}
	if auth.Name != name {
		t.Fatalf("ResolveAuth offline = %+v, want name passed through unchanged", auth)
	}
}
