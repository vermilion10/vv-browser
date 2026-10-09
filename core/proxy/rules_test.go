package proxy

import "testing"

func TestDefaultRules(t *testing.T) {
	rules := DefaultRules()
	tests := []struct {
		host string
		want Route
	}{
		{"play-cloud.games.dmm.com:443", Tunnel},
		{"accounts.dmm.com:443", Tunnel},
		{"dmm.com", Tunnel},
		{"www.dmm.co.jp:443", Tunnel},
		{"gw.dmmapis.com:443", Tunnel},
		{"dcgp-game.ugamenow.com:443", Tunnel},
		{"DCGP-GAME.UGAMENOW.COM.", Tunnel},
		{"gc-140-227-189-95.ugamenow.com:23778", Direct},
		{"www.google.com:443", Direct},
		{"notdmm.com:443", Direct},
		{"dmm.com.evil.example:443", Direct},
	}
	for _, tt := range tests {
		if got := rules.Match(tt.host); got != tt.want {
			t.Errorf("Match(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}
