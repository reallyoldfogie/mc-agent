package agent

import "testing"

func TestSeedBlockCoords_FloorsNegativeCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name       string
		x, y, z    float64
		wx, wy, wz int64
	}{
		{"on the floor", 10.5, -60, 3.2, 12, -60, 3},
		{"mid-fall over negative ground", 10.5, -59.5, 3.2, 12, -60, 3},
		{"negative x and z", -0.5, -60, -3.2, 1, -60, -4},
		{"positive coordinates", 4.9, 64.1, 7.5, 6, 64, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, y, z := seedBlockCoords(tc.x, tc.y, tc.z)
			if x != tc.wx || y != tc.wy || z != tc.wz {
				t.Fatalf("seedBlockCoords(%v,%v,%v) = (%d,%d,%d), want (%d,%d,%d)", tc.x, tc.y, tc.z, x, y, z, tc.wx, tc.wy, tc.wz)
			}
		})
	}
}
