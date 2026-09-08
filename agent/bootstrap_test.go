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
