package speaker

import "testing"

func TestPresenceToOnline(t *testing.T) {
	cases := map[string]bool{
		"online":  true,
		"offline": false,
		"":        false,
		"unknown": false,
	}
	for p, want := range cases {
		if got := presenceOnline(p); got != want {
			t.Errorf("presenceOnline(%q)=%v want %v", p, got, want)
		}
	}
}
