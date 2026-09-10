package config

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLoadFallsBackToDefaultsWhenFileMissing(t *testing.T) {
	settings, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Settings has a slice field (Courier.Servers), so it can no longer use
	// == — reflect.DeepEqual is the direct replacement.
	if !reflect.DeepEqual(settings, Default()) {
		t.Fatalf("Load with missing file = %+v, want Default() = %+v", settings, Default())
	}
}

func TestLoadOverridesOnlyFieldsPresentInFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, path, `{"connection": {"address": "127.0.0.1:25565"}, "rcon": {"address": "localhost:25575"}}`)

	settings, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.Connection.Address != "127.0.0.1:25565" {
		t.Fatalf("Connection.Address = %q, want 127.0.0.1:25565", settings.Connection.Address)
	}
	if settings.RCON.Address != "localhost:25575" {
		t.Fatalf("RCON.Address = %q, want localhost:25575", settings.RCON.Address)
	}
	// Untouched fields keep their defaults.
	if settings.Replay.Generator != Default().Replay.Generator {
		t.Fatalf("Replay.Generator = %q, want default %q", settings.Replay.Generator, Default().Replay.Generator)
	}
	if settings.Auth.ClientID != DefaultClientID {
		t.Fatalf("Auth.ClientID = %q, want default %q", settings.Auth.ClientID, DefaultClientID)
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, path, `not json`)
	if _, err := Load(path); err == nil {
		t.Fatalf("Load with invalid JSON: want error, got nil")
	}
}

func TestEnvSettingsToRlenvConfigConvertsStepTimeoutAndSeeder(t *testing.T) {
	e := EnvSettings{StepTimeoutSeconds: 2.5}
	cfg := e.ToRlenvConfig()
	if want := 2500 * time.Millisecond; cfg.StepTimeout != want {
		t.Fatalf("StepTimeout = %v, want %v", cfg.StepTimeout, want)
	}
	if cfg.Seeder != nil {
		t.Fatalf("Seeder should be nil when SeedEpisodes is false")
	}

	e.SeedEpisodes = true
	cfg = e.ToRlenvConfig()
	if cfg.Seeder == nil {
		t.Fatalf("Seeder should be set when SeedEpisodes is true")
	}
}

func TestEnvSettingsToRlenvConfigConvertsResetOriginAndStuckTimeout(t *testing.T) {
	e := EnvSettings{StuckTimeout: 20}
	cfg := e.ToRlenvConfig()
	if cfg.ResetOrigin != nil {
		t.Fatalf("ResetOrigin should be nil when UseResetOrigin is false")
	}
	if cfg.StuckTimeout != 20 {
		t.Fatalf("StuckTimeout = %d, want 20", cfg.StuckTimeout)
	}

	e.UseResetOrigin = true
	e.ResetOrigin = [3]float64{1, 2, 3}
	cfg = e.ToRlenvConfig()
	if cfg.ResetOrigin == nil || *cfg.ResetOrigin != [3]float64{1, 2, 3} {
		t.Fatalf("ResetOrigin = %v, want &{1 2 3}", cfg.ResetOrigin)
	}
}

func TestEnvSettingsToRlenvConfigConvertsJitter(t *testing.T) {
	e := EnvSettings{Jitter: [3]float64{2, 0, 2}, JitterSeed: 42}
	cfg := e.ToRlenvConfig()
	if cfg.Jitter != [3]float64{2, 0, 2} {
		t.Fatalf("Jitter = %v, want {2 0 2}", cfg.Jitter)
	}
	if cfg.JitterSeed != 42 {
		t.Fatalf("JitterSeed = %d, want 42", cfg.JitterSeed)
	}
}

func TestApplyEnvOverridesNestedFields(t *testing.T) {
	t.Setenv("MCAGENT_CONNECTION_ADDRESS", "10.0.0.1:25566")
	t.Setenv("MCAGENT_RCON_PASSWORD", "hunter2")
	t.Setenv("MCAGENT_ENV_MINE_SEARCH_RADIUS", "7")
	t.Setenv("MCAGENT_MOVEMENT_ENABLE_CLUTCH", "true")

	settings := Default()
	if err := ApplyEnv(&settings, "MCAGENT"); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}

	if settings.Connection.Address != "10.0.0.1:25566" {
		t.Fatalf("Connection.Address = %q, want 10.0.0.1:25566", settings.Connection.Address)
	}
	if settings.RCON.Password != "hunter2" {
		t.Fatalf("RCON.Password = %q, want hunter2", settings.RCON.Password)
	}
	if settings.Env.MineSearchRadius != 7 {
		t.Fatalf("Env.MineSearchRadius = %d, want 7", settings.Env.MineSearchRadius)
	}
	if !settings.Movement.EnableClutch {
		t.Fatalf("Movement.EnableClutch = false, want true")
	}
	// Untouched fields keep their prior value.
	if settings.Connection.Name != "" {
		t.Fatalf("Connection.Name = %q, want empty (no env var set)", settings.Connection.Name)
	}
}

func TestApplyEnvLeavesSettingsUnchangedWhenNoEnvVarsSet(t *testing.T) {
	settings := Default()
	before := settings
	if err := ApplyEnv(&settings, "MCAGENT_UNUSED_PREFIX_TEST"); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if !reflect.DeepEqual(settings, before) {
		t.Fatalf("ApplyEnv with no matching env vars changed settings: got %+v, want %+v", settings, before)
	}
}

func TestApplyEnvSkipsArrayFields(t *testing.T) {
	// EnvSettings.TargetOffset is a [3]float64 — ApplyEnv documents this
	// as unsupported; confirm it doesn't panic or silently misbehave.
	t.Setenv("MCAGENT_ENV_TARGET_OFFSET", "1,2,3")
	settings := Default()
	if err := ApplyEnv(&settings, "MCAGENT"); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if settings.Env.TargetOffset != (Default().Env.TargetOffset) {
		t.Fatalf("TargetOffset changed despite being an unsupported array field: %v", settings.Env.TargetOffset)
	}
}

func TestApplyConnectionFlagsOnlyOverridesExplicitlyPassed(t *testing.T) {
	settings := Default()

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	overrides := RegisterConnectionFlags(fs)
	if err := fs.Parse([]string{"-address=127.0.0.1:25566"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ApplyConnectionFlags(&settings.Connection, fs, overrides)

	if settings.Connection.Address != "127.0.0.1:25566" {
		t.Fatalf("Connection.Address = %q, want 127.0.0.1:25566", settings.Connection.Address)
	}
	if settings.Connection.Name != "" {
		t.Fatalf("Connection.Name = %q, want empty (unset flag must not clobber it)", settings.Connection.Name)
	}
}

func TestRegisterTrainFlagsOverridesConfigPathDefault(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	overrides := RegisterTrainFlags(fs)
	if overrides.ConfigPath != DefaultConfigPath {
		t.Fatalf("ConfigPath = %q, want %q", overrides.ConfigPath, DefaultConfigPath)
	}
}
