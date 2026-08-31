package following

import (
	"encoding/hex"
	"testing"
)

// TestDeriveOfflineUUID pins deriveOfflineUUID against a real offline-mode
// server's actual assigned UUID for the name "ChestAccess", confirmed live
// (see git history/commit message for the diagnostic). The previous
// implementation used uuid.NewMD5(uuid.NameSpaceOID, ...) - an RFC4122
// "UUIDv3 with namespace" derivation - which does not match vanilla's own
// offline-mode UUID assignment (Java's plain
// UUID.nameUUIDFromBytes("OfflinePlayer:"+name), a raw MD5 digest with no
// namespace prefix) and silently never matched any real server, breaking
// FindPlayerByName's fallback path whenever the primary player-list lookup
// failed or hadn't caught up yet.
func TestDeriveOfflineUUID(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "ChestAccess", want: "3b6989dee58e3833ab4893c135ed6b52"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveOfflineUUID(tt.name)
			gotHex := hex.EncodeToString(got[:])
			if gotHex != tt.want {
				t.Errorf("deriveOfflineUUID(%q) = %s, want %s", tt.name, gotHex, tt.want)
			}
		})
	}
}
