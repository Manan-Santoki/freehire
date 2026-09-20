package config

import "testing"

func TestJevEnabled(t *testing.T) {
	cases := []struct {
		name string
		j    Jev
		want bool
	}{
		{"all set", Jev{BaseURL: "u", APIKey: "k", Model: "m"}, true},
		{"empty", Jev{}, false},
		{"missing key", Jev{BaseURL: "u", Model: "m"}, false},
	}
	for _, c := range cases {
		if got := c.j.Enabled(); got != c.want {
			t.Errorf("%s: Enabled()=%v want %v", c.name, got, c.want)
		}
	}
}

func TestLoadJevScoreDefaultsAndRequire(t *testing.T) {
	t.Setenv("JEV_API_KEY", "") // unconfigured
	if _, err := LoadJevScore(); err == nil {
		t.Fatal("LoadJevScore should fail fast when JEV_* unset")
	}
	t.Setenv("JEV_BASE_URL", "https://x")
	t.Setenv("JEV_API_KEY", "key")
	t.Setenv("JEV_MODEL", "jev-latest")
	c, err := LoadJevScore()
	if err != nil {
		t.Fatalf("LoadJevScore: %v", err)
	}
	if c.Concurrency != 4 || c.Version != 1 || c.ApplyMin != 60 || c.MaybeMin != 45 || c.HardBlockMax != 0.5 {
		t.Errorf("defaults wrong: %+v", c)
	}
}
