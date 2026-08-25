package world

import "testing"

func TestVersionRequiresCalculatedDataLen(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"1.21.4", false},
		{"1.21.5", true},
		{"1.21.11", true},
		{"26.1", true},
		{"not-a-version", false},
	}
	for _, tt := range tests {
		if got := versionRequiresCalculatedDataLen(tt.version); got != tt.want {
			t.Errorf("versionRequiresCalculatedDataLen(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}

func TestVersionHasFluidCount(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"1.21.11", false},
		{"1.21.4", false},
		{"26.1", true},
		{"not-a-version", false},
	}
	for _, tt := range tests {
		if got := versionHasFluidCount(tt.version); got != tt.want {
			t.Errorf("versionHasFluidCount(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}
