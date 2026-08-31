package agent

import "testing"

func TestParseEntityPosRegex(t *testing.T) {
	tests := []struct {
		name   string
		resp   string
		wantX  float64
		wantY  float64
		wantZ  float64
		wantOK bool
	}{
		{
			name:  "typical response",
			resp:  "ChestAccess has the following entity data: [1.5d, 64.0d, -2.5d]",
			wantX: 1.5, wantY: 64.0, wantZ: -2.5,
			wantOK: true,
		},
		{
			name:  "negative and integer-looking values",
			resp:  "SomeBot has the following entity data: [-150.0d, 0.0d, 148.5d]",
			wantX: -150.0, wantY: 0.0, wantZ: 148.5,
			wantOK: true,
		},
		{
			name:   "player not found",
			resp:   "No entity was found",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := entityPosRe.FindStringSubmatch(tt.resp)
			if !tt.wantOK {
				if m != nil {
					t.Errorf("expected no match, got %v", m)
				}
				return
			}
			if m == nil {
				t.Fatalf("expected a match, got none")
			}
		})
	}
}

func TestLooksLikeCommandFailure(t *testing.T) {
	tests := []struct {
		resp string
		want bool
	}{
		{resp: "Set ChestAccessCam's game mode to Spectator Mode", want: false},
		{resp: "Incorrect argument for command", want: true},
		{resp: "No entity was found", want: true},
		{resp: "Unknown command. Did you mean...", want: true},
	}
	for _, tt := range tests {
		if got := looksLikeCommandFailure(tt.resp); got != tt.want {
			t.Errorf("looksLikeCommandFailure(%q) = %v, want %v", tt.resp, got, tt.want)
		}
	}
}
