package config

import (
	"fmt"
	"strings"
	"time"
)

// Jev is the connection config for TypeSafe's Jev decision API. Separate from LLM:
// Jev is not OpenAI-compatible, so it does not share the LLM_* vars or client.
type Jev struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

// LoadJev reads the values permissively; policy (require vs degrade) is the caller's.
func LoadJev() Jev {
	return Jev{
		BaseURL: env("JEV_BASE_URL", "https://api.typesafe.ai/v1/systemone"),
		APIKey:  env("JEV_API_KEY", ""),
		Model:   env("JEV_MODEL", "jev-latest"),
		Timeout: envDuration("JEV_TIMEOUT", 30*time.Second),
	}
}

// Enabled reports whether a Jev client can be built.
func (j Jev) Enabled() bool {
	return j.BaseURL != "" && j.APIKey != "" && j.Model != ""
}

// Require fails fast for worker entrypoints, naming every missing var.
func (j Jev) Require() error {
	var missing []string
	for _, v := range []struct{ key, value string }{
		{"JEV_BASE_URL", j.BaseURL}, {"JEV_API_KEY", j.APIKey}, {"JEV_MODEL", j.Model},
	} {
		if v.value == "" {
			missing = append(missing, v.key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("config: missing required env: %s", strings.Join(missing, ", "))
	}
	return nil
}

// JevScore is the cmd/jevscore worker config: the connection plus queue + verdict knobs.
type JevScore struct {
	Jev

	Concurrency       int
	LeaseSeconds      int
	MaxAttempts       int
	UpstreamGraceDays int
	Version           int

	ApplyMin     int
	MaybeMin     int
	HardBlockMax float64
}

func LoadJevScore() (JevScore, error) {
	c := JevScore{
		Jev:               LoadJev(),
		Concurrency:       envInt("JEV_CONCURRENCY", 4),
		LeaseSeconds:      envInt("JEV_LEASE_SECONDS", 300),
		MaxAttempts:       envInt("JEVSCORE_MAX_ATTEMPTS", 3),
		UpstreamGraceDays: envInt("JEVSCORE_UPSTREAM_GRACE_DAYS", 14),
		Version:           envInt("JEVSCORE_VERSION", 1),
		ApplyMin:          envInt("JEVSCORE_APPLY_MIN", 60),
		MaybeMin:          envInt("JEVSCORE_MAYBE_MIN", 45),
		HardBlockMax:      envFloat("JEVSCORE_HARDBLOCK_MAX", 0.5),
	}
	if c.Concurrency < 1 {
		c.Concurrency = 1
	}
	if err := c.Require(); err != nil {
		return JevScore{}, err
	}
	return c, nil
}
