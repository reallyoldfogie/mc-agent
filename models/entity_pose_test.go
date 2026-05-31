package models

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntityPoseRegistry_Fallback(t *testing.T) {
	reg := NewEntityPoseRegistryFromFallback()
	require.NotNil(t, reg)
	assert.True(t, reg.UsedFallback())
	assert.Equal(t, len(fallbackPoseNames), reg.Count())

	name, ok := reg.Name(0)
	assert.True(t, ok)
	assert.Equal(t, "standing", name)

	// 'sitting' is at ordinal 10 in the 1.21.10 enum.
	ord, ok := reg.Ordinal("sitting")
	assert.True(t, ok)
	assert.Equal(t, int32(10), ord)
}

func TestEntityPoseRegistry_UnknownOrdinal(t *testing.T) {
	reg := NewEntityPoseRegistryFromFallback()
	name, ok := reg.Name(999)
	assert.False(t, ok)
	assert.Equal(t, "unknown_999", name)

	name, ok = reg.Name(-1)
	assert.False(t, ok)
	assert.Equal(t, "unknown_-1", name)
}

func TestEntityPoseRegistry_OrdinalCaseInsensitive(t *testing.T) {
	reg := NewEntityPoseRegistryFromFallback()
	ord, ok := reg.Ordinal("SITTING")
	assert.True(t, ok)
	assert.Equal(t, int32(10), ord)

	_, ok = reg.Ordinal("not_a_pose")
	assert.False(t, ok)
}

func TestLoadEntityPoseRegistry_MissingFileUsesFallback(t *testing.T) {
	dir := t.TempDir()
	reg, err := LoadEntityPoseRegistry(dir, "1.21.99")
	require.NoError(t, err)
	require.NotNil(t, reg)
	assert.True(t, reg.UsedFallback())
	assert.Equal(t, len(fallbackPoseNames), reg.Count())
}

func TestLoadEntityPoseRegistry_ReadsMapForm(t *testing.T) {
	dir := t.TempDir()
	versionDir := filepath.Join(dir, "1.99.0")
	require.NoError(t, os.MkdirAll(versionDir, 0o755))
	// Canonical on-disk format: explicit ordinal->name map. Note that the keys
	// are deliberately out of order to confirm the loader uses the key, not
	// the JSON iteration order.
	payload := []byte(`{"2":"sitting","0":"standing","1":"fall_flying"}`)
	require.NoError(t, os.WriteFile(filepath.Join(versionDir, "poses.json"), payload, 0o644))

	reg, err := LoadEntityPoseRegistry(dir, "1.99.0")
	require.NoError(t, err)
	require.NotNil(t, reg)
	assert.False(t, reg.UsedFallback())
	assert.Equal(t, 3, reg.Count())

	ord, ok := reg.Ordinal("sitting")
	assert.True(t, ok)
	assert.Equal(t, int32(2), ord, "loader must trust the explicit key, not file position")

	name, ok := reg.Name(1)
	assert.True(t, ok)
	assert.Equal(t, "fall_flying", name)
}

func TestLoadEntityPoseRegistry_ReadsSparseMapForm(t *testing.T) {
	// Verify gaps in the enum (e.g. Mojang deprecates an intermediate pose)
	// don't make the registry mis-index neighbors.
	dir := t.TempDir()
	versionDir := filepath.Join(dir, "1.99.0")
	require.NoError(t, os.MkdirAll(versionDir, 0o755))
	payload := []byte(`{"0":"standing","2":"sitting","17":"inhaling"}`)
	require.NoError(t, os.WriteFile(filepath.Join(versionDir, "poses.json"), payload, 0o644))

	reg, err := LoadEntityPoseRegistry(dir, "1.99.0")
	require.NoError(t, err)
	assert.Equal(t, 3, reg.Count())

	name, ok := reg.Name(17)
	assert.True(t, ok)
	assert.Equal(t, "inhaling", name)

	// Ordinal 1 was deliberately omitted — must resolve as unknown, not
	// silently map to whatever was next in iteration order.
	name, ok = reg.Name(1)
	assert.False(t, ok)
	assert.Equal(t, "unknown_1", name)
}

func TestLoadEntityPoseRegistry_BackCompatArrayForm(t *testing.T) {
	// The very first revision of the exporter shipped a positional array.
	// Keep that path working so existing dumps don't break.
	dir := t.TempDir()
	versionDir := filepath.Join(dir, "1.99.0")
	require.NoError(t, os.MkdirAll(versionDir, 0o755))
	payload := []byte(`["standing","fall_flying","sitting"]`)
	require.NoError(t, os.WriteFile(filepath.Join(versionDir, "poses.json"), payload, 0o644))

	reg, err := LoadEntityPoseRegistry(dir, "1.99.0")
	require.NoError(t, err)
	assert.Equal(t, 3, reg.Count())

	ord, ok := reg.Ordinal("sitting")
	assert.True(t, ok)
	assert.Equal(t, int32(2), ord)
}

func TestLoadEntityPoseRegistry_EmptyMapFallsBack(t *testing.T) {
	dir := t.TempDir()
	versionDir := filepath.Join(dir, "1.99.0")
	require.NoError(t, os.MkdirAll(versionDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(versionDir, "poses.json"), []byte(`{}`), 0o644))

	reg, err := LoadEntityPoseRegistry(dir, "1.99.0")
	require.NoError(t, err)
	require.NotNil(t, reg)
	assert.True(t, reg.UsedFallback(), "empty file should not silently disable pose lookups")
}

func TestLoadEntityPoseRegistry_NonIntegerKeyErrors(t *testing.T) {
	dir := t.TempDir()
	versionDir := filepath.Join(dir, "1.99.0")
	require.NoError(t, os.MkdirAll(versionDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(versionDir, "poses.json"), []byte(`{"standing":"standing"}`), 0o644))

	_, err := LoadEntityPoseRegistry(dir, "1.99.0")
	assert.Error(t, err, "non-integer keys must fail loudly — silently dropping them would hide exporter bugs")
}

func TestLoadEntityPoseRegistry_MalformedJSONErrors(t *testing.T) {
	dir := t.TempDir()
	versionDir := filepath.Join(dir, "1.99.0")
	require.NoError(t, os.MkdirAll(versionDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(versionDir, "poses.json"), []byte(`not json`), 0o644))

	_, err := LoadEntityPoseRegistry(dir, "1.99.0")
	assert.Error(t, err)
}
