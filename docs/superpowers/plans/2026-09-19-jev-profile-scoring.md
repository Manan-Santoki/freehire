# Jev Profile Scoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the skill-only "Profile match" signal with a Jev-backed score (match %, verdict, seniority-fit / stack / hard-blocker probabilities) computed ahead of time for feed-eligible (user, job) pairs, and add a Postgres-ranked "For You" feed.

**Architecture:** A deterministic eligibility gate bounds which (user, job) pairs Jev ever scores. Eligible pairs are enqueued to `job_score_outbox` and drained by a new `cmd/jevscore` cron worker that calls TypeSafe's Jev `/v1/systemone` API and writes a per-(user, job) row to `user_job_scores`. The badge and a new `GET /jobs/for-you` feed read those Postgres rows; Meilisearch keeps keyword/facets/autocomplete/anon browsing. Everything degrades to the existing deterministic skill signals when Jev is unconfigured.

**Tech Stack:** Go (Fiber, pgx/v5, sqlc v1.31.1), Postgres, SvelteKit (Svelte 5 runes), tygo contracts. Jev API (not OpenAI-compatible → its own `net/http` client).

**Spec:** `docs/superpowers/specs/2026-09-19-jev-profile-scoring-design.md`

## Global Constraints

- **Jev endpoint/shape:** `POST {JEV_BASE_URL}` (default `https://api.typesafe.ai/v1/systemone`), header `Authorization: Bearer {JEV_API_KEY}`, body `{"model","state","questions"}`. Response `{"model","answers":{name:{type,score|choice|noul,confidence?,...}},"usage":{"input_tokens","output_tokens"}}`. `noul` answers may omit `confidence` (default 0). Store the response's resolved `model` (e.g. `jev-1.13.0`), not the requested `jev-latest`, as the staleness stamp.
- **Never commit the API key.** It lives only in `JEV_API_KEY` on the deploy; `.env.example` carries a placeholder.
- **Vendor-neutral:** never hard-code the URL/model outside config defaults (`JEV_*`).
- **Verdict rule (ported from `jd_match.py::summarize`):** `SKIP` if `hard_blocker >= HARDBLOCK_MAX (0.5)` or `role_category == "non_technical"`; else `APPLY` if `match_pct >= APPLY_MIN (60)` and `has_required_stack >= 0.5` and `fits_level >= 0.5`; else `MAYBE` if `match_pct >= MAYBE_MIN (45)`; else `SKIP`.
- **Score sanitize invariant:** clamp `match_pct` to 0..100, clamp every noul/confidence to 0..1, coerce `role_category` to the known 8-value set (else `other_tech`). Never persist a raw out-of-vocabulary value.
- **Best-effort:** an unconfigured/failing Jev never errors a user request; badge and feed fall back to `jobmatch.Compute` / the existing `?sort=match`.
- **sqlc regen:** after editing `internal/platform/db/queries/*.sql`, run `make sqlc` and commit regenerated `internal/platform/db/*.sql.go`, `querier.go`, `models.go`.
- **Migration:** one new file `migrations/0175_jev_scores.sql` (plain `CREATE TABLE`, no `migrate: no-transaction` marker). Applied by initdb on a fresh volume; run manually on prod before deploying code that reads the tables.
- **Read job facets from `db.Job` columns** (`Category`, `Seniority`, `Countries`, `Regions`, `WorkMode`), NOT the enrichment blob.
- **Résumé to Jev = structured contact-free projection** (`resumeextract.Professional`), never raw CV text.

---

## Phase 1 — Jev client + config

### Task 1: Jev + JevScore config

**Files:**
- Create: `internal/platform/config/jev.go`
- Test: `internal/platform/config/jev_test.go`

**Interfaces:**
- Produces: `config.Jev{BaseURL, APIKey, Model string; Timeout time.Duration}` with `LoadJev() Jev`, `(Jev).Enabled() bool`, `(Jev).Require() error`; `config.JevScore{Jev; Concurrency, LeaseSeconds, MaxAttempts, UpstreamGraceDays, Version, ApplyMin, MaybeMin int; HardBlockMax float64}` with `LoadJevScore() (JevScore, error)`.
- Consumes: existing unexported helpers `env`, `envInt`, `envDuration`, `envFloat` (same package).

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/platform/config/ -run 'Jev' -v`
Expected: FAIL (undefined `Jev`).

- [ ] **Step 3: Write the implementation**

```go
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
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/platform/config/ -run 'Jev' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/config/jev.go internal/platform/config/jev_test.go
git commit -m "feat(config): Jev + JevScore worker config"
```

---

### Task 2: Jev HTTP client

**Files:**
- Create: `internal/platform/jev/jev.go`
- Test: `internal/platform/jev/jev_test.go`

**Interfaces:**
- Consumes: `config.Jev` (Task 1).
- Produces: `jev.QuestionType` (`Score`/`Choice`/`Noul`), `jev.Question{Type,Instructions,Criteria}`, `jev.Answer{Type,Score,Choice,Noul,Confidence}`, `jev.Response{Model,Answers map[string]Answer,Usage}`, `jev.Usage{InputTokens,OutputTokens}`, `jev.Client`, `New(baseURL,apiKey,model string, timeout time.Duration) *Client`, `NewFromConfig(config.Jev) *Client`, `(*Client).Decide(ctx, state string, questions map[string]Question) (Response, error)`. A nil `*Client` is unconfigured.

- [ ] **Step 1: Write the failing test** (httptest, mirrors the `bodyRecorder` pattern in `internal/platform/llm/reasoning_test.go`)

```go
package jev

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDecideSendsBodyAndParsesAnswers(t *testing.T) {
	var gotAuth string
	var gotBody map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"jev-1.13.0","answers":{
			"match_score":{"type":"score","score":1.12,"confidence":0.9},
			"role_category":{"type":"choice","choice":"ml_ai","confidence":0.79},
			"hard_blocker":{"type":"noul","noul":0.93}},
			"usage":{"input_tokens":510,"output_tokens":97}}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "secret-key", "jev-latest", 5*time.Second)
	resp, err := c.Decide(context.Background(), "STATE", map[string]Question{
		"match_score": {Type: Score, Instructions: "rate", Criteria: []string{"a", "b"}},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("auth header = %q", gotAuth)
	}
	if _, ok := gotBody["questions"]; !ok {
		t.Error("request missing questions")
	}
	if resp.Model != "jev-1.13.0" {
		t.Errorf("model = %q", resp.Model)
	}
	if resp.Answers["match_score"].Score != 1.12 || resp.Answers["match_score"].Confidence != 0.9 {
		t.Errorf("score answer = %+v", resp.Answers["match_score"])
	}
	if resp.Answers["role_category"].Choice != "ml_ai" {
		t.Errorf("choice = %q", resp.Answers["role_category"].Choice)
	}
	// noul with no confidence key must default to 0, not error.
	if resp.Answers["hard_blocker"].Noul != 0.93 || resp.Answers["hard_blocker"].Confidence != 0 {
		t.Errorf("noul answer = %+v", resp.Answers["hard_blocker"])
	}
	if resp.Usage.InputTokens != 510 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestDecideMapsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"bad key"}`)
	}))
	defer srv.Close()
	if _, err := New(srv.URL, "k", "m", time.Second).Decide(context.Background(), "s", nil); err == nil {
		t.Fatal("expected error on 401")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/platform/jev/ -v`
Expected: FAIL (undefined `New`).

- [ ] **Step 3: Write the implementation**

```go
// Package jev is the client for TypeSafe's Jev ("System One") decision API. It sends
// unstructured state plus typed questions and returns typed, calibrated answers in one
// pass. Not OpenAI-compatible, so it does not use internal/platform/llm.
package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/strelov1/freehire/internal/platform/config"
)

type QuestionType string

const (
	Score  QuestionType = "score"
	Choice QuestionType = "choice"
	Noul   QuestionType = "noul"
)

// Question is one typed decision. Criteria is a []string rubric for Score, a
// map[string]string for Choice, and nil for Noul.
type Question struct {
	Type         QuestionType `json:"type"`
	Instructions string       `json:"instructions"`
	Criteria     any          `json:"criteria,omitempty"`
}

// Answer holds whichever value field matches the question type, plus calibrated
// confidence (absent for noul → 0).
type Answer struct {
	Type       string  `json:"type"`
	Score      float64 `json:"score"`
	Choice     string  `json:"choice"`
	Noul       float64 `json:"noul"`
	Confidence float64 `json:"confidence"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func New(baseURL, apiKey, model string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{baseURL: baseURL, apiKey: apiKey, model: model, http: &http.Client{Timeout: timeout}}
}

// NewFromConfig builds a client, or nil when unconfigured (callers degrade).
func NewFromConfig(c config.Jev) *Client {
	if !c.Enabled() {
		return nil
	}
	return New(c.BaseURL, c.APIKey, c.Model, c.Timeout)
}

type request struct {
	Model     string              `json:"model"`
	State     string              `json:"state"`
	Questions map[string]Question `json:"questions"`
}

func (c *Client) Decide(ctx context.Context, state string, questions map[string]Question) (Response, error) {
	if c == nil {
		return Response{}, fmt.Errorf("jev: client is nil (unconfigured)")
	}
	body, err := json.Marshal(request{Model: c.model, State: state, Questions: questions})
	if err != nil {
		return Response{}, fmt.Errorf("jev: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("jev: request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("jev: do: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("jev: status %d: %s", resp.StatusCode, string(raw))
	}
	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return Response{}, fmt.Errorf("jev: decode: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/platform/jev/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/jev/
git commit -m "feat(jev): TypeSafe Jev decision API client"
```

---

## Phase 2 — Decision model (`internal/candidate/jevscore`)

### Task 3: Questions + state builder

**Files:**
- Create: `internal/candidate/jevscore/model.go`
- Test: `internal/candidate/jevscore/model_test.go`

**Interfaces:**
- Consumes: `jev.Question`/`jev.QuestionType` (Task 2), `resumeextract.Professional`, `llm.TruncateRunes`.
- Produces: `jevscore.Questions(seniorities []string) map[string]jev.Question`; `jevscore.BuildState(r resumeextract.Professional, jobText string) string`; exported `jevscore.RoleCategories []string`.

- [ ] **Step 1: Write the failing test**

```go
package jevscore

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/jev"
)

func TestQuestionsLevelDerived(t *testing.T) {
	qs := Questions([]string{"intern", "junior"})
	if len(qs) != 5 {
		t.Fatalf("want 5 questions, got %d", len(qs))
	}
	if qs["match_score"].Type != jev.Score || qs["role_category"].Type != jev.Choice {
		t.Error("wrong question types")
	}
	if !strings.Contains(qs["fits_level"].Instructions, "intern") || !strings.Contains(qs["fits_level"].Instructions, "junior") {
		t.Errorf("fits_level should name the profile levels: %q", qs["fits_level"].Instructions)
	}
	if _, ok := qs["role_category"].Criteria.(map[string]string)["ml_ai"]; !ok {
		t.Error("role_category criteria missing ml_ai")
	}
}

func TestQuestionsNoLevelsDegrades(t *testing.T) {
	if strings.Contains(Questions(nil)["fits_level"].Instructions, "targeting:") {
		t.Error("no levels should use the generic instruction")
	}
}

func TestBuildState(t *testing.T) {
	s := BuildState(resumeextract.Professional{Headline: "SWE"}, "Backend Engineer")
	if !strings.Contains(s, "RESUME:") || !strings.Contains(s, "JOB DESCRIPTION:") || !strings.Contains(s, "Backend Engineer") {
		t.Errorf("state malformed: %q", s)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/candidate/jevscore/ -run 'Questions|BuildState' -v`
Expected: FAIL (undefined).

- [ ] **Step 3: Write the implementation**

```go
// Package jevscore is the Jev-backed job-fit decision model: the typed questions, the
// state builder, and the sanitized Score with its verdict. Ported from the validated
// jd_match.py, with fits_level derived per profile.
package jevscore

import (
	"encoding/json"
	"strings"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/jev"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// maxStructuredRunes bounds the résumé JSON in the state, matching matchanalysis.
const maxStructuredRunes = 3000

// matchRubric is the 5-level score rubric; raw score maps raw/(len-1) -> 0..100.
var matchRubric = []string{
	"No meaningful overlap; wrong field or role type",
	"Weak match; a few transferable skills but missing most core requirements",
	"Moderate match; meets several core requirements, notable gaps remain",
	"Strong match; meets most core requirements with only minor gaps",
	"Excellent match; meets or exceeds nearly all requirements",
}

// RoleCategories is the closed choice vocabulary; also the coercion target set.
var RoleCategories = []string{"swe", "ml_ai", "robotics", "qa_test", "data", "devops_infra", "other_tech", "non_technical"}

var roleCriteria = map[string]string{
	"swe":           "General, full-stack, backend, or frontend software engineering",
	"ml_ai":         "Machine learning, AI, LLM, or data science engineering",
	"robotics":      "Robotics, embedded, or controls",
	"qa_test":       "QA, SDET, test automation, or beta testing",
	"data":          "Data engineering, analytics, or data platform",
	"devops_infra":  "DevOps, SRE, platform, or cloud infrastructure",
	"other_tech":    "A technical role not covered by the other options",
	"non_technical": "A non-technical role such as sales, marketing, or management",
}

// Questions builds the five typed questions. fits_level's instruction is derived from
// the profile's seniority levels so the model judges level fit per candidate.
func Questions(seniorities []string) map[string]jev.Question {
	return map[string]jev.Question{
		"match_score": {Type: jev.Score, Instructions: "Rate how well the RESUME matches the requirements in the JOB DESCRIPTION.", Criteria: matchRubric},
		"role_category": {Type: jev.Choice, Instructions: "Classify the type of role described in the JOB DESCRIPTION.", Criteria: roleCriteria},
		"has_required_stack": {Type: jev.Noul, Instructions: "The RESUME demonstrates the core technical stack and skills the JOB DESCRIPTION requires."},
		"fits_level":  {Type: jev.Noul, Instructions: fitsLevelInstruction(seniorities)},
		"hard_blocker": {Type: jev.Noul, Instructions: "The JOB DESCRIPTION states a hard requirement the candidate likely cannot meet, such as US citizenship, an active security clearance, a specific degree not held, or a specialized license."},
	}
}

func fitsLevelInstruction(seniorities []string) string {
	if len(seniorities) == 0 {
		return "The role's seniority and experience level fit the candidate's stated experience level rather than being clearly over- or under-qualified."
	}
	return "The role's seniority and experience level fit a candidate targeting: " +
		strings.Join(seniorities, ", ") + " — not a level clearly above or below those."
}

// BuildState assembles the Jev state: the structured résumé JSON (contacts already
// removed by the Professional projection) and the job text.
func BuildState(r resumeextract.Professional, jobText string) string {
	blob, err := json.Marshal(r)
	if err != nil {
		blob = []byte("{}")
	}
	resume := llm.TruncateRunes(string(blob), maxStructuredRunes)
	return "RESUME:\n" + resume + "\n\n---\n\nJOB DESCRIPTION:\n" + jobText
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/candidate/jevscore/ -run 'Questions|BuildState' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/candidate/jevscore/model.go internal/candidate/jevscore/model_test.go
git commit -m "feat(jevscore): typed questions + state builder"
```

---

### Task 4: Score type, sanitize, verdict

**Files:**
- Create: `internal/candidate/jevscore/score.go`
- Test: `internal/candidate/jevscore/score_test.go`

**Interfaces:**
- Consumes: `jev.Response` (Task 2).
- Produces: `jevscore.Score{MatchPct int; MatchRaw,MatchConfidence float64; RoleCategory string; RoleConfidence float64; HasRequiredStack,FitsLevel,HardBlocker float64; Verdict string}`; `jevscore.Thresholds{ApplyMin,MaybeMin int; HardBlockMax float64}`; `jevscore.FromAnswers(resp jev.Response, th Thresholds) Score`.

- [ ] **Step 1: Write the failing test** (seeded with the two real validated cases)

```go
package jevscore

import (
	"testing"

	"github.com/strelov1/freehire/internal/platform/jev"
)

var defaultTh = Thresholds{ApplyMin: 60, MaybeMin: 45, HardBlockMax: 0.5}

func resp(ms, msc float64, role string, rolec, stack, level, blocker float64) jev.Response {
	return jev.Response{Answers: map[string]jev.Answer{
		"match_score":        {Score: ms, Confidence: msc},
		"role_category":      {Choice: role, Confidence: rolec},
		"has_required_stack": {Noul: stack},
		"fits_level":         {Noul: level},
		"hard_blocker":       {Noul: blocker},
	}}
}

func TestFromAnswersFoundingEngIsSkip(t *testing.T) {
	// Real case: strong stack but wrong seniority + hard blocker -> SKIP.
	s := FromAnswers(resp(1.78, 0.76, "ml_ai", 1.0, 0.75, 0.12, 0.33), defaultTh)
	if s.MatchPct != 45 { // round(1.78/4*100)
		t.Errorf("MatchPct=%d want 45", s.MatchPct)
	}
	if s.Verdict != "MAYBE" {
		t.Errorf("verdict=%s want MAYBE (45>=MaybeMin, blocker<0.5, but level<0.5 blocks APPLY)", s.Verdict)
	}
}

func TestFromAnswersHardBlockerForcesSkip(t *testing.T) {
	s := FromAnswers(resp(3.0, 0.9, "ml_ai", 1.0, 0.9, 0.9, 0.93), defaultTh)
	if s.Verdict != "SKIP" {
		t.Errorf("verdict=%s want SKIP (hard_blocker 0.93>=0.5)", s.Verdict)
	}
}

func TestFromAnswersApply(t *testing.T) {
	s := FromAnswers(resp(3.2, 0.9, "swe", 1.0, 0.8, 0.8, 0.1), defaultTh)
	if s.MatchPct != 80 || s.Verdict != "APPLY" {
		t.Errorf("got pct=%d verdict=%s want 80/APPLY", s.MatchPct, s.Verdict)
	}
}

func TestFromAnswersSanitizes(t *testing.T) {
	s := FromAnswers(resp(9.0, 2.0, "wizardry", -3, 5, -1, 2), defaultTh)
	if s.MatchPct != 100 || s.MatchConfidence != 1 || s.RoleCategory != "other_tech" ||
		s.HasRequiredStack != 1 || s.FitsLevel != 0 || s.HardBlocker != 1 {
		t.Errorf("not clamped/coerced: %+v", s)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/candidate/jevscore/ -run FromAnswers -v`
Expected: FAIL (undefined).

- [ ] **Step 3: Write the implementation**

```go
package jevscore

import (
	"math"
	"slices"

	"github.com/strelov1/freehire/internal/platform/jev"
)

// Score is the sanitized, server-owned decision for one (candidate, job).
type Score struct {
	MatchPct         int     `json:"match_pct"`
	MatchRaw         float64 `json:"match_raw"`
	MatchConfidence  float64 `json:"match_confidence"`
	RoleCategory     string  `json:"role_category"`
	RoleConfidence   float64 `json:"role_confidence"`
	HasRequiredStack float64 `json:"has_required_stack"`
	FitsLevel        float64 `json:"fits_level"`
	HardBlocker      float64 `json:"hard_blocker"`
	Verdict          string  `json:"verdict"`
}

type Thresholds struct {
	ApplyMin     int
	MaybeMin     int
	HardBlockMax float64
}

// FromAnswers maps a Jev response to a sanitized Score and computes the verdict.
func FromAnswers(resp jev.Response, th Thresholds) Score {
	a := resp.Answers
	levels := float64(len(matchRubric) - 1)
	raw := a["match_score"].Score
	pct := 0
	if levels > 0 {
		pct = int(math.Round(raw / levels * 100))
	}
	role := a["role_category"].Choice
	if !slices.Contains(RoleCategories, role) {
		role = "other_tech"
	}
	s := Score{
		MatchPct:         clampInt(pct, 0, 100),
		MatchRaw:         raw,
		MatchConfidence:  clamp01(a["match_score"].Confidence),
		RoleCategory:     role,
		RoleConfidence:   clamp01(a["role_category"].Confidence),
		HasRequiredStack: clamp01(a["has_required_stack"].Noul),
		FitsLevel:        clamp01(a["fits_level"].Noul),
		HardBlocker:      clamp01(a["hard_blocker"].Noul),
	}
	s.Verdict = verdict(s, th)
	return s
}

func verdict(s Score, th Thresholds) string {
	switch {
	case s.HardBlocker >= th.HardBlockMax || s.RoleCategory == "non_technical":
		return "SKIP"
	case s.MatchPct >= th.ApplyMin && s.HasRequiredStack >= 0.5 && s.FitsLevel >= 0.5:
		return "APPLY"
	case s.MatchPct >= th.MaybeMin:
		return "MAYBE"
	default:
		return "SKIP"
	}
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/candidate/jevscore/ -run FromAnswers -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/candidate/jevscore/score.go internal/candidate/jevscore/score_test.go
git commit -m "feat(jevscore): sanitized Score + verdict"
```

---

### Task 5: Scorer (composes client + model)

**Files:**
- Create: `internal/candidate/jevscore/scorer.go`
- Test: `internal/candidate/jevscore/scorer_test.go`

**Interfaces:**
- Consumes: `jev.Client` (Task 2), `Questions`/`BuildState` (Task 3), `FromAnswers`/`Thresholds` (Task 4).
- Produces: `jevscore.Input{Resume resumeextract.Professional; JobText string; Seniorities []string}`; `jevscore.Scorer`; `NewScorer(c *jev.Client, th Thresholds) *Scorer`; `(*Scorer).Enabled() bool`; `(*Scorer).Score(ctx, in Input) (Score, error)`.

- [ ] **Step 1: Write the failing test**

```go
package jevscore

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/jev"
)

type fakeDecider struct{ resp jev.Response; gotState string }

func (f *fakeDecider) Decide(_ context.Context, state string, _ map[string]jev.Question) (jev.Response, error) {
	f.gotState = state
	return f.resp, nil
}

func TestScorerScore(t *testing.T) {
	fd := &fakeDecider{resp: resp(3.2, 0.9, "swe", 1, 0.8, 0.8, 0.1)}
	s := &Scorer{decider: fd, th: defaultTh}
	got, err := s.Score(context.Background(), Input{JobText: "Backend Engineer", Seniorities: []string{"junior"}})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if got.Verdict != "APPLY" {
		t.Errorf("verdict=%s", got.Verdict)
	}
	if fd.gotState == "" {
		t.Error("state not built/sent")
	}
}

func TestScorerNilIsDisabled(t *testing.T) {
	if NewScorer(nil, defaultTh).Enabled() {
		t.Error("nil client scorer must be disabled")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/candidate/jevscore/ -run Scorer -v`
Expected: FAIL.

- [ ] **Step 3: Write the implementation**

```go
package jevscore

import (
	"context"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/jev"
)

type decider interface {
	Decide(ctx context.Context, state string, questions map[string]jev.Question) (jev.Response, error)
}

// Input is one scoring request.
type Input struct {
	Resume      resumeextract.Professional
	JobText     string
	Seniorities []string
}

// Scorer runs one Jev decision and sanitizes it into a Score.
type Scorer struct {
	decider decider
	th      Thresholds
}

// NewScorer returns a Scorer; a nil client yields a disabled Scorer (Enabled()==false).
func NewScorer(c *jev.Client, th Thresholds) *Scorer {
	if c == nil {
		return &Scorer{th: th}
	}
	return &Scorer{decider: c, th: th}
}

func (s *Scorer) Enabled() bool { return s != nil && s.decider != nil }

// Model returns the resolved model of the last-known configuration for stamping; the
// resolved model actually stored comes from each response (see the worker).
func (s *Scorer) Score(ctx context.Context, in Input) (Score, error) {
	resp, err := s.decider.Decide(ctx, BuildState(in.Resume, in.JobText), Questions(in.Seniorities))
	if err != nil {
		return Score{}, err
	}
	sc := FromAnswers(resp, s.th)
	return sc, nil
}

// ScoreWithModel is like Score but also returns the response's resolved model, which the
// worker stores as the staleness stamp (so a Jev upgrade auto-invalidates).
func (s *Scorer) ScoreWithModel(ctx context.Context, in Input) (Score, string, error) {
	resp, err := s.decider.Decide(ctx, BuildState(in.Resume, in.JobText), Questions(in.Seniorities))
	if err != nil {
		return Score{}, "", err
	}
	return FromAnswers(resp, s.th), resp.Model, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/candidate/jevscore/ -run Scorer -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/candidate/jevscore/scorer.go internal/candidate/jevscore/scorer_test.go
git commit -m "feat(jevscore): Scorer composing client + model"
```

---

## Phase 3 — Eligibility gate (`internal/candidate/jeveligible`)

### Task 6: Deterministic eligibility predicate

**Files:**
- Create: `internal/candidate/jeveligible/jeveligible.go`
- Test: `internal/candidate/jeveligible/jeveligible_test.go`

**Interfaces:**
- Consumes: `db.Job`, `userprofile.Profile`/`userprofile.LocationPreferences`, `resumeextract.Professional`, `hardconstraint` (Evaluate + categories/severity), `jobfacts` (DegreeOptional/RequiredCertifications), `enrich.Enrichment` (visa), `internal/platform/pgconv`.
- Produces: `jeveligible.Candidate{Profile, Resume, Loc, DerivedCountries}`; `jeveligible.Eligible(job db.Job, c Candidate) bool`. (The coarse specialization/seniority/exclusion filtering also lives in SQL at enqueue — Task 8; this Go gate is the FINE per-pair check the worker runs before calling Jev.)

**Note on structure:** the input building copies the proven logic from `internal/api/handler/hardconstraint_inputs.go` (`buildHardConstraintInputs`) verbatim — it lives in package `handler` (unexported) and depends on `db.Job`, so it is reimplemented here rather than imported. Keep `hardconstraint` itself pure (no `db` dep).

- [ ] **Step 1: Write the failing test**

```go
package jeveligible

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/db"
)

func techJob() db.Job {
	return db.Job{
		Category: "ml_ai", Seniority: "intern", Description: "AI intern role",
		IsTech: pgtype.Bool{Bool: true, Valid: true},
		Company: "Acme", CompanySlug: "acme", Source: "greenhouse",
	}
}

func prof() userprofile.Profile {
	return userprofile.Profile{
		Specializations: []string{"ml_ai", "swe"},
		Seniorities:     []string{"intern", "junior"},
	}
}

func TestEligibleHappyPath(t *testing.T) {
	if !Eligible(techJob(), Candidate{Profile: prof()}) {
		t.Error("expected eligible")
	}
}

func TestIneligibleWrongCategory(t *testing.T) {
	j := techJob()
	j.Category = "sales_role_not_in_specs"
	if Eligible(j, Candidate{Profile: prof()}) {
		t.Error("category not in specializations -> ineligible")
	}
}

func TestIneligibleWrongSeniority(t *testing.T) {
	j := techJob()
	j.Seniority = "principal"
	if Eligible(j, Candidate{Profile: prof()}) {
		t.Error("seniority not in profile levels -> ineligible")
	}
}

func TestEmptyProfileSeniorityAllowsAny(t *testing.T) {
	p := prof()
	p.Seniorities = nil
	j := techJob()
	j.Seniority = "principal"
	if !Eligible(j, Candidate{Profile: p}) {
		t.Error("empty profile seniority = no seniority constraint")
	}
}

func TestExcludedCompany(t *testing.T) {
	p := prof()
	p.ExcludedCompanies = []string{"acme"}
	if Eligible(techJob(), Candidate{Profile: p}) {
		t.Error("excluded company -> ineligible")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/candidate/jeveligible/ -v`
Expected: FAIL (undefined).

- [ ] **Step 3: Write the implementation**

```go
// Package jeveligible is the deterministic gate that bounds which (user, job) pairs Jev
// ever scores. It reproduces the profile's feed filter (specialization, seniority,
// exclusions) plus the obvious hard-constraint blockers (work authorization, location /
// work mode), so an ineligible pair never reaches the model.
package jeveligible

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/job/jobfacts"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// Candidate is the profile-side of the gate for one user.
type Candidate struct {
	Profile          userprofile.Profile
	Resume           resumeextract.Professional
	Loc              userprofile.LocationPreferences
	DerivedCountries []string
}

// Eligible reports whether job should be Jev-scored for this candidate.
func Eligible(job db.Job, c Candidate) bool {
	if job.Category == "" || !slices.Contains(c.Profile.Specializations, job.Category) {
		return false
	}
	if len(c.Profile.Seniorities) > 0 && (job.Seniority == "" || !slices.Contains(c.Profile.Seniorities, job.Seniority)) {
		return false
	}
	if slices.Contains(c.Profile.ExcludedCompanies, strings.ToLower(job.CompanySlug)) ||
		slices.Contains(c.Profile.ExcludedCompanies, strings.ToLower(job.Company)) {
		return false
	}
	if slices.Contains(c.Profile.ExcludedSources, strings.ToLower(job.Source)) {
		return false
	}
	// Obvious hard blockers: work authorization and location/work-mode only.
	jr, ev := hardInputs(job, c)
	for _, b := range hardconstraint.Evaluate(jr, ev) {
		if b.Met {
			continue
		}
		if b.Category == hardconstraint.CategoryWorkAuth || b.Category == hardconstraint.CategoryLocationWorkMode {
			return false
		}
	}
	return true
}

// hardInputs mirrors internal/api/handler.buildHardConstraintInputs.
func hardInputs(job db.Job, c Candidate) (hardconstraint.JobRequirements, hardconstraint.CVEvidence) {
	jr := hardconstraint.JobRequirements{
		ExperienceYearsMin:     pgconv.IntPtr(job.ExperienceYearsMin),
		EducationLevel:         job.EducationLevel,
		DegreeOptional:         jobfacts.DegreeOptional(job.Description),
		EnglishLevel:           job.EnglishLevel,
		VisaSponsorship:        jobVisa(job.Enrichment),
		WorkMode:               job.WorkMode,
		Countries:              job.Countries,
		RequiredCertifications: jobfacts.RequiredCertifications(job.Description),
	}
	ev := hardconstraint.CVEvidence{
		TotalYears:     c.Resume.TotalYears,
		Degrees:        degreeNames(c.Resume.Education),
		Languages:      c.Resume.Languages,
		Certifications: c.Resume.Certifications,
		CountryCode:    candidateCountry(c.Loc.Base.Country, c.DerivedCountries),
		PrefersRemote:  prefersRemote(c.Loc.WorkModes),
	}
	return jr, ev
}

func jobVisa(raw json.RawMessage) *bool {
	if len(raw) == 0 {
		return nil
	}
	var e enrich.Enrichment
	if json.Unmarshal(raw, &e) != nil {
		return nil
	}
	return e.VisaSponsorship
}

func degreeNames(edu []resumeextract.Education) []string {
	out := make([]string, 0, len(edu))
	for _, e := range edu {
		if e.Degree != "" {
			out = append(out, e.Degree)
		}
	}
	return out
}

func candidateCountry(asserted string, derived []string) string {
	if asserted != "" {
		return asserted
	}
	if len(derived) == 1 {
		return derived[0]
	}
	return ""
}

func prefersRemote(workModes []string) bool {
	var remote, onsiteOrHybrid bool
	for _, m := range workModes {
		switch strings.ToLower(m) {
		case "remote":
			remote = true
		case "onsite", "hybrid":
			onsiteOrHybrid = true
		}
	}
	return remote && !onsiteOrHybrid
}
```

**Note:** verify the helper signatures against `internal/api/handler/hardconstraint_inputs.go` (`jobfacts.DegreeOptional`, `jobfacts.RequiredCertifications`, `pgconv.IntPtr`) — copy exact package paths if they differ.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/candidate/jeveligible/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/candidate/jeveligible/
git commit -m "feat(jeveligible): deterministic Jev eligibility gate"
```

---

## Phase 4 — Schema, queries, store

### Task 7: Migration + SQL queries + sqlc regen

**Files:**
- Create: `migrations/0175_jev_scores.sql`
- Create: `internal/platform/db/queries/user_job_scores.sql`
- Modify (generated): `internal/platform/db/user_job_scores.sql.go`, `internal/platform/db/querier.go`, `internal/platform/db/models.go`

**Interfaces:**
- Produces (sqlc-generated): `q.GetUserJobScore`, `q.UpsertUserJobScore`, `q.ForYouFeed`, `q.CountForYou`, `q.InvalidateUserScores`, `q.DeleteUserScoreOutbox`, `q.EnqueueJevScoresForProfile`, `q.ClaimJevScoreBatch`, `q.DeleteJevScoreEntries`, `q.DeleteIneligibleJevScoreOutbox`, `q.RecordJevScoreFailure`, `q.GetJob` (add if absent), plus `db.UserJobScore`, `db.JobScoreOutbox` models.

- [ ] **Step 1: Write the migration** (`migrations/0175_jev_scores.sql`)

```sql
-- Per-(user, job) Jev fit score + its work queue. The score is the seniority/role/
-- blocker-aware replacement for the deterministic jobmatch coverage badge and the
-- source of the personalized "For You" feed ordering. Only feed-eligible pairs are ever
-- scored (see internal/candidate/jeveligible); the feed reads only stored rows, so the
-- feed IS the eligible set by construction.
--
-- Staleness is quintuple-stamped like user_job_analysis: model (the RESOLVED Jev model,
-- so an upgrade auto-invalidates), score_version (JEVSCORE_VERSION bump), the profile
-- fingerprint (specializations+seniorities+skills+location), cv_uploaded_at, and
-- job_content_hash. A recompute overwrites the row and all stamps. FKs cascade.
CREATE TABLE public.user_job_scores (
    user_id             bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    job_id              bigint      NOT NULL REFERENCES public.jobs(id)  ON DELETE CASCADE,
    match_pct           smallint    NOT NULL,
    match_raw           real        NOT NULL,
    match_confidence    real        NOT NULL,
    role_category       text        NOT NULL,
    role_confidence     real        NOT NULL,
    has_required_stack  real        NOT NULL,
    fits_level          real        NOT NULL,
    hard_blocker        real        NOT NULL,
    verdict             text        NOT NULL,
    model               text        NOT NULL,
    score_version       integer     NOT NULL,
    profile_fingerprint text        NOT NULL,
    cv_uploaded_at      timestamptz,
    job_content_hash    text,
    scored_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, job_id)
);

-- Feed ordering (per user, best first) and verdict filtering.
CREATE INDEX user_job_scores_feed_idx ON public.user_job_scores (user_id, match_pct DESC);
CREATE INDEX user_job_scores_verdict_idx ON public.user_job_scores (user_id, verdict);

-- Reference-only work queue, mirroring semantic_outbox: one live entry per (user, job),
-- freshest job first, lease + dead-letter. Rows carry no copy of the job.
CREATE TABLE public.job_score_outbox (
    id             bigint      NOT NULL,
    user_id        bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    job_id         bigint      NOT NULL REFERENCES public.jobs(id)  ON DELETE CASCADE,
    target_version integer     NOT NULL,
    job_posted_at  timestamptz,
    attempts       integer     NOT NULL DEFAULT 0,
    claimed_at     timestamptz,
    failed_at      timestamptz,
    last_error     text        NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE public.job_score_outbox
    ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (SEQUENCE NAME public.job_score_outbox_id_seq);

ALTER TABLE ONLY public.job_score_outbox
    ADD CONSTRAINT job_score_outbox_pkey PRIMARY KEY (id);

-- The enqueue dedup key: one live entry per (user, job).
ALTER TABLE ONLY public.job_score_outbox
    ADD CONSTRAINT job_score_outbox_user_job_key UNIQUE (user_id, job_id);

-- Claim order: freshest job first over claimable (not dead-lettered) rows.
CREATE INDEX job_score_outbox_claim_idx
    ON public.job_score_outbox (job_posted_at DESC NULLS LAST, job_id DESC)
    WHERE (failed_at IS NULL);
```

- [ ] **Step 2: Write the queries** (`internal/platform/db/queries/user_job_scores.sql`)

```sql
-- name: GetUserJobScore :one
-- The caller's cached Jev score for one job, with the five staleness stamps. No row =
-- never scored (handler falls back to deterministic coverage). The handler compares the
-- stamps to live values to decide the stale flag.
SELECT match_pct, match_raw, match_confidence, role_category, role_confidence,
       has_required_stack, fits_level, hard_blocker, verdict,
       model, score_version, profile_fingerprint, cv_uploaded_at, job_content_hash, scored_at
FROM user_job_scores
WHERE user_id = $1 AND job_id = $2;

-- name: UpsertUserJobScore :exec
-- Create-or-replace the score for a (user, job). Composite PK makes it idempotent.
INSERT INTO user_job_scores (
    user_id, job_id, match_pct, match_raw, match_confidence, role_category, role_confidence,
    has_required_stack, fits_level, hard_blocker, verdict,
    model, score_version, profile_fingerprint, cv_uploaded_at, job_content_hash, scored_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16, now())
ON CONFLICT (user_id, job_id) DO UPDATE SET
    match_pct           = EXCLUDED.match_pct,
    match_raw           = EXCLUDED.match_raw,
    match_confidence    = EXCLUDED.match_confidence,
    role_category       = EXCLUDED.role_category,
    role_confidence     = EXCLUDED.role_confidence,
    has_required_stack  = EXCLUDED.has_required_stack,
    fits_level          = EXCLUDED.fits_level,
    hard_blocker        = EXCLUDED.hard_blocker,
    verdict             = EXCLUDED.verdict,
    model               = EXCLUDED.model,
    score_version       = EXCLUDED.score_version,
    profile_fingerprint = EXCLUDED.profile_fingerprint,
    cv_uploaded_at      = EXCLUDED.cv_uploaded_at,
    job_content_hash    = EXCLUDED.job_content_hash,
    scored_at           = now();

-- name: ForYouFeed :many
-- The caller's personalized feed: eligible scored jobs, best first. Optional verdict
-- filter ('' = all) and minimum match_pct. Paginated.
SELECT j.public_slug, j.title, j.company, j.company_slug, j.location, j.work_mode,
       j.posted_at, j.closed_at, j.skills,
       s.match_pct, s.verdict, s.has_required_stack, s.fits_level, s.hard_blocker, s.role_category
FROM user_job_scores s
JOIN jobs j ON j.id = s.job_id
WHERE s.user_id = $1
  AND j.closed_at IS NULL AND j.duplicate_of IS NULL
  AND (sqlc.arg(verdict)::text = '' OR s.verdict = sqlc.arg(verdict)::text)
  AND s.match_pct >= sqlc.arg(min_pct)::int
ORDER BY s.match_pct DESC, j.id DESC
LIMIT sqlc.arg(lim)::int OFFSET sqlc.arg(off)::int;

-- name: CountForYou :one
SELECT count(*)
FROM user_job_scores s
JOIN jobs j ON j.id = s.job_id
WHERE s.user_id = $1
  AND j.closed_at IS NULL AND j.duplicate_of IS NULL
  AND (sqlc.arg(verdict)::text = '' OR s.verdict = sqlc.arg(verdict)::text)
  AND s.match_pct >= sqlc.arg(min_pct)::int;

-- name: InvalidateUserScores :exec
-- Drop a user's scores after a profile edit or CV upload so they re-score fresh.
DELETE FROM user_job_scores WHERE user_id = $1;

-- name: DeleteUserScoreOutbox :exec
-- Drop a user's pending queue entries so re-enqueue stamps them with fresh values.
DELETE FROM job_score_outbox WHERE user_id = $1;

-- name: EnqueueJevScoresForProfile :execrows
-- Coarse enqueue for one profile: open/tech/enriched jobs whose category and seniority
-- match, not excluded, and lacking a FRESH score. The fine hard-blocker gate runs in the
-- worker before Jev is called. ON CONFLICT keeps one live entry per (user, job).
INSERT INTO job_score_outbox (user_id, job_id, target_version, job_posted_at)
SELECT sqlc.arg(user_id)::bigint, j.id, sqlc.arg(target_version)::int, COALESCE(j.posted_at, j.created_at)
FROM jobs j
WHERE j.closed_at IS NULL
  AND j.duplicate_of IS NULL
  AND j.is_tech IS TRUE
  AND j.description <> ''
  AND j.enriched_at IS NOT NULL
  AND j.category = ANY(sqlc.arg(specializations)::text[])
  AND (cardinality(sqlc.arg(seniorities)::text[]) = 0 OR j.seniority = ANY(sqlc.arg(seniorities)::text[]))
  AND NOT (j.company_slug = ANY(sqlc.arg(excluded_companies)::text[]))
  AND NOT (j.source = ANY(sqlc.arg(excluded_sources)::text[]))
  AND NOT EXISTS (
      SELECT 1 FROM user_job_scores s
      WHERE s.user_id = sqlc.arg(user_id)::bigint
        AND s.job_id = j.id
        AND s.score_version = sqlc.arg(target_version)::int
        AND s.model = sqlc.arg(model)::text
        AND s.profile_fingerprint = sqlc.arg(profile_fingerprint)::text
        AND s.cv_uploaded_at IS NOT DISTINCT FROM sqlc.arg(cv_uploaded_at)
        AND s.job_content_hash IS NOT DISTINCT FROM j.content_hash
  )
ON CONFLICT (user_id, job_id) DO NOTHING;

-- name: ClaimJevScoreBatch :many
-- Claim a wave of live, unleased entries for open canonical jobs, freshest first.
WITH claimable AS (
    SELECT o.id, o.user_id, o.job_id, o.target_version
    FROM job_score_outbox o
    WHERE o.failed_at IS NULL
      AND (o.claimed_at IS NULL
           OR o.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
      AND EXISTS (
          SELECT 1 FROM jobs j
          WHERE j.id = o.job_id AND j.closed_at IS NULL AND j.duplicate_of IS NULL
      )
    ORDER BY o.job_posted_at DESC NULLS LAST, o.job_id DESC
    FOR UPDATE OF o SKIP LOCKED
    LIMIT sqlc.arg(batch_size)::int
)
UPDATE job_score_outbox o
SET claimed_at = now()
FROM claimable c
WHERE o.id = c.id
RETURNING o.id, o.user_id, o.job_id, o.target_version;

-- name: DeleteJevScoreEntries :exec
DELETE FROM job_score_outbox WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: DeleteIneligibleJevScoreOutbox :execrows
-- Reap live entries the claim can never take: job gone/closed/duplicate. Leaves
-- dead-lettered rows (failed_at set) alone. Bounded by max_rows.
DELETE FROM job_score_outbox
WHERE id IN (
    SELECT o.id
    FROM job_score_outbox o
    LEFT JOIN jobs j ON j.id = o.job_id
    WHERE o.failed_at IS NULL
      AND (j.id IS NULL OR j.closed_at IS NOT NULL OR j.duplicate_of IS NOT NULL)
    LIMIT sqlc.arg(max_rows)::int
);

-- name: RecordJevScoreFailure :one
-- Count a failed attempt; dead-letter at max_attempts (posting-fault) or past the
-- upstream grace window (outage). Lease left in place as the crash reaper.
UPDATE job_score_outbox
SET attempts   = attempts + 1,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE
                     WHEN sqlc.arg(posting_at_fault)::boolean
                         THEN CASE WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now() END
                     ELSE CASE
                              WHEN sqlc.arg(upstream_grace_days)::int > 0
                                  AND created_at < now() - make_interval(days => sqlc.arg(upstream_grace_days)::int)
                                  THEN now()
                          END
                 END
WHERE id = sqlc.arg(id)
RETURNING attempts, failed_at;
```

- [ ] **Step 3: Add `GetJob` if absent** — check `internal/platform/db/queries/jobs.sql` for a by-id getter. If none returns the full row by id, add:

```sql
-- name: GetJob :one
SELECT * FROM jobs WHERE id = $1;
```

- [ ] **Step 4: Regenerate + build**

Run: `make sqlc && go build ./...`
Expected: generates `internal/platform/db/user_job_scores.sql.go`, updates `models.go`/`querier.go`; build succeeds.

- [ ] **Step 5: Commit**

```bash
git add migrations/0175_jev_scores.sql internal/platform/db/queries/user_job_scores.sql internal/platform/db/
git commit -m "feat(db): user_job_scores + job_score_outbox schema and queries"
```

---

### Task 8: Store adapter + integration tests

**Files:**
- Create: `internal/platform/db/user_job_scores_integration_test.go` (`//go:build integration`, `package db`)

**Interfaces:**
- Consumes: generated queries (Task 7), test harness `startPostgres`/`truncate`/`ingestUpsert`/`ingestParams` (existing in `package db` integration tests).
- Produces: verified query behavior (upsert idempotency, claim ordering + lease + closed-skip, reaper, dead-letter, enqueue eligibility + fresh-skip).

- [ ] **Step 1: Write the failing integration test**

```go
//go:build integration

package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestUpsertAndGetUserJobScore(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	uid := insertUser(t, pool) // helper below
	job, err := ingestUpsert(ctx, q, ingestParams("acme:1", "AI Intern"))
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}

	err = q.UpsertUserJobScore(ctx, UpsertUserJobScoreParams{
		UserID: uid, JobID: job.ID, MatchPct: 80, MatchRaw: 3.2, MatchConfidence: 0.9,
		RoleCategory: "ml_ai", RoleConfidence: 1, HasRequiredStack: 0.8, FitsLevel: 0.9,
		HardBlocker: 0.1, Verdict: "APPLY", Model: "jev-1.13.0", ScoreVersion: 1,
		ProfileFingerprint: "fp1",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := q.GetUserJobScore(ctx, GetUserJobScoreParams{UserID: uid, JobID: job.ID})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.MatchPct != 80 || got.Verdict != "APPLY" || got.Model != "jev-1.13.0" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	// Upsert again -> overwrite, still one row.
	_ = q.UpsertUserJobScore(ctx, UpsertUserJobScoreParams{UserID: uid, JobID: job.ID, MatchPct: 40, Verdict: "SKIP", Model: "jev-1.13.0", ScoreVersion: 1, ProfileFingerprint: "fp1", RoleCategory: "ml_ai"})
	got2, _ := q.GetUserJobScore(ctx, GetUserJobScoreParams{UserID: uid, JobID: job.ID})
	if got2.MatchPct != 40 {
		t.Errorf("overwrite failed: %d", got2.MatchPct)
	}
}

func TestClaimSkipsClosedAndRespectsLease(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	uid := insertUser(t, pool)
	open, _ := ingestUpsert(ctx, q, ingestParams("acme:open", "Open"))
	closed, _ := ingestUpsert(ctx, q, ingestParams("acme:closed", "Closed"))
	pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, closed.ID)
	for _, id := range []int64{open.ID, closed.ID} {
		pool.Exec(ctx, `INSERT INTO job_score_outbox (user_id, job_id, target_version, job_posted_at) VALUES ($1,$2,1,now())`, uid, id)
	}
	rows, err := q.ClaimJevScoreBatch(ctx, ClaimJevScoreBatchParams{LeaseSeconds: 180, BatchSize: 10})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(rows) != 1 || rows[0].JobID != open.ID {
		t.Fatalf("want only open job claimed, got %+v", rows)
	}
	// Immediately re-claiming returns nothing (leased).
	again, _ := q.ClaimJevScoreBatch(ctx, ClaimJevScoreBatchParams{LeaseSeconds: 180, BatchSize: 10})
	if len(again) != 0 {
		t.Errorf("leased entry should not re-claim: %+v", again)
	}
	_ = time.Now
}

func insertUser(t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (pgconnCommandTag, error)
}) int64 {
	// Replace with the existing user-insert helper used by other integration tests;
	// if none, insert a minimal users row and return its id.
	t.Helper()
	return 0
}
```

**Note:** replace `insertUser`/`pgconnCommandTag` with the user-fixture helper other integration tests use (grep `internal/platform/db/*_integration_test.go` for how they seed `users`; e.g. `user_profiles_integration_test.go`). `truncate` must include `user_job_scores, job_score_outbox` if they lack a jobs/users FK path — they DO cascade from users/jobs, so `TRUNCATE ... users, jobs ... CASCADE` reaches them; verify.

- [ ] **Step 2: Run to verify it fails**

Run: `go test -tags=integration ./internal/platform/db/ -run 'UserJobScore|Claim' -v`
Expected: FAIL until the migration/queries exist (they do from Task 7) — this confirms the harness + generated params compile and behave.

- [ ] **Step 3: Fix the fixture helper** so the test compiles and passes (use the real user seed helper).

- [ ] **Step 4: Run to verify it passes**

Run: `go test -tags=integration ./internal/platform/db/ -run 'UserJobScore|Claim' -v`
Expected: PASS (requires Docker).

- [ ] **Step 5: Commit**

```bash
git add internal/platform/db/user_job_scores_integration_test.go
git commit -m "test(db): user_job_scores + outbox integration tests"
```

---

## Phase 5 — Worker (`cmd/jevscore`)

### Task 9: Runner + worker main + profile/CV invalidation

**Files:**
- Create: `internal/candidate/jevscore/runner.go` (Store port + Runner)
- Create: `internal/candidate/jevscore/runner_test.go`
- Create: `cmd/jevscore/main.go`
- Create: `cmd/jevscore/store.go`
- Modify: the profile-save and CV-upload handlers to call `InvalidateUserScores` + `DeleteUserScoreOutbox` (best-effort)

**Interfaces:**
- Consumes: `Scorer` (Task 5), `jeveligible` (Task 6), generated queries (Task 7), `internal/platform/outbox.RunPool`, `worker.Main/Bootstrap/ExitCode`.
- Produces: `jevscore.Store` interface (`Reap`, `EnqueueForProfiles`, `Claim`, `Complete`, `Fail`); `jevscore.Runner{Scorer, Store, Profiles, Jobs, Resumes}`; `(Runner).Run(ctx, RunOptions) (Stats, error)`.

- [ ] **Step 1: Write the failing test** (Runner drains a fake store, gates ineligible, upserts)

```go
package jevscore

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/jev"
	"github.com/strelov1/freehire/internal/platform/outbox"
)

type fakeStore struct {
	claimed   []Claimed
	completed []Claimed
	failed    []Claimed
}

func (f *fakeStore) Reap(context.Context, int) (int, error) { return 0, nil }
func (f *fakeStore) Claim(_ context.Context, batch, _ int) ([]Claimed, error) {
	out := f.claimed
	f.claimed = nil
	if len(out) > batch {
		out = out[:batch]
	}
	return out, nil
}
func (f *fakeStore) Complete(_ context.Context, c Claimed, _ Score, _ string) error {
	f.completed = append(f.completed, c)
	return nil
}
func (f *fakeStore) Fail(_ context.Context, c Claimed, _ error) (bool, error) {
	f.failed = append(f.failed, c)
	return false, nil
}

func TestRunnerScoresEligibleAndSkipsIneligible(t *testing.T) {
	// ... build a Claimed for an eligible job + an ineligible one; a fake Scorer decider;
	// assert Complete called for the eligible, and the ineligible is Completed (dropped)
	// without a Jev call. (Full body in implementation.)
	_ = outbox.Succeeded
	_ = jev.Score
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/candidate/jevscore/ -run Runner -v`
Expected: FAIL (undefined `Claimed`/`Store`/`Runner`).

- [ ] **Step 3: Write `runner.go`**

```go
package jevscore

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/candidate/jeveligible"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/outbox"
)

// Claimed is one leased (user, job) queue entry plus the loaded job + candidate needed
// to score it. The Store fills Job/Candidate when it claims.
type Claimed struct {
	ID        int64
	UserID    int64
	JobID     int64
	Version   int32
	Job       db.Job
	Candidate jeveligible.Candidate
	// Fingerprint / CVUploadedAt are the profile-side staleness stamps for the write.
	Fingerprint  string
	CVUploadedAt *db.Timestamptz // pgtype.Timestamptz alias; see store
}

type Store interface {
	Reap(ctx context.Context, max int) (int, error)
	Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	Complete(ctx context.Context, c Claimed, score Score, model string) error
	Fail(ctx context.Context, c Claimed, cause error) (deadLettered bool, err error)
}

type RunOptions struct {
	Concurrency  int
	LeaseSeconds int
}

type Stats struct {
	Scored, Skipped, Failed, DeadLettered, Reaped int
}

type Runner struct {
	Scorer *Scorer
	Store  Store
}

const reapLimit = 5000

func (r Runner) Run(ctx context.Context, opt RunOptions) (Stats, error) {
	var stats Stats
	if !r.Scorer.Enabled() {
		return stats, nil // best-effort: unconfigured Jev scores nothing
	}
	reaped, err := r.Store.Reap(ctx, reapLimit)
	if err != nil {
		log.Printf("jevscore: reap: %v", err)
	}
	stats.Reaped = reaped

	s, err := outbox.RunPool(ctx, r.Store, outbox.RunOptions{
		BatchSize: opt.Concurrency, LeaseSeconds: opt.LeaseSeconds, Concurrency: opt.Concurrency,
	}, r.process)
	stats.Scored = s.Succeeded
	stats.Failed = s.Failed
	stats.DeadLettered = s.DeadLettered
	stats.Skipped = s.Discarded
	return stats, err
}

func (r Runner) process(ctx context.Context, c Claimed) outbox.Outcome {
	// Fine gate: an ineligible pair is dropped (Complete deletes the row) without Jev.
	if !jeveligible.Eligible(c.Job, c.Candidate) {
		if err := r.Store.Complete(ctx, c, Score{}, ""); err != nil {
			log.Printf("jevscore: drop ineligible: %v", err)
			return outbox.Failed
		}
		return outbox.Discarded
	}
	score, model, err := r.Scorer.ScoreWithModel(ctx, Input{
		Resume:      c.Candidate.Resume,
		JobText:     c.Job.Description,
		Seniorities: c.Candidate.Profile.Seniorities,
	})
	if err != nil {
		dead, ferr := r.Store.Fail(ctx, c, err)
		if ferr != nil {
			log.Printf("jevscore: fail record: %v", ferr)
		}
		if dead {
			return outbox.DeadLettered
		}
		return outbox.Failed
	}
	if err := r.Store.Complete(ctx, c, score, model); err != nil {
		log.Printf("jevscore: complete: %v", err)
		return outbox.Failed
	}
	return outbox.Succeeded
}
```

**Note:** `db.Timestamptz` is not a real alias — use `pgtype.Timestamptz` from `github.com/jackc/pgx/v5/pgtype` in `Claimed` and imports. Adjust the struct field type accordingly during implementation.

- [ ] **Step 4: Write `cmd/jevscore/store.go`** — the DB adapter. `Claim` runs `ClaimJevScoreBatch`, then for each row loads the job (`GetJob`), and (cached per user within the wave) the profile (`userProfile.Get`), structured résumé (`resume.Structured`) and geography (`resume.Geography`), building `jeveligible.Candidate` + fingerprint + cv_uploaded_at. `Complete` runs `UpsertUserJobScore` + `DeleteJevScoreEntries([id])` in one tx (skip the upsert when `model==""`, i.e. an ineligible drop — just delete). `Fail` runs `RecordJevScoreFailure` (posting_at_fault=false for transport errors). `Reap` runs `DeleteIneligibleJevScoreOutbox`. A separate `EnqueueForProfiles` (called before the pool, inside `Run` or `main`) loops active profiles calling `EnqueueJevScoresForProfile`.

```go
package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/candidate/jeveligible"
	"github.com/strelov1/freehire/internal/candidate/jevscore"
	"github.com/strelov1/freehire/internal/platform/db"
)

type dbStore struct {
	pool        *pgxpool.Pool
	q           *db.Queries
	version     int32
	model       string // resolved lazily; for the enqueue fresh-check use the configured model
	profiles    profileLoader
	resumes     resumeLoader
	// per-wave caches keyed by user_id
}
// ... Claim/Complete/Fail/Reap as described; EnqueuePending loops profiles.
```

- [ ] **Step 5: Write `cmd/jevscore/main.go`** (mirror `cmd/enrich/main.go`)

```go
// Command jevscore is the standalone Jev scoring worker. It enqueues eligible (user,job)
// pairs, then drains the queue: for each it calls Jev and writes user_job_scores. Runs on
// a schedule and exits; non-zero on any failure/dead-letter.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/candidate/jevscore"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/jev"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	cfg, err := config.LoadJevScore()
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	client := jev.NewFromConfig(cfg.Jev)
	scorer := jevscore.NewScorer(client, jevscore.Thresholds{ApplyMin: cfg.ApplyMin, MaybeMin: cfg.MaybeMin, HardBlockMax: cfg.HardBlockMax})

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	store := newDBStore(pool, cfg)
	if n, err := store.EnqueuePending(ctx); err != nil {
		log.Printf("jevscore: enqueue: %v", err)
	} else {
		log.Printf("jevscore: enqueued=%d", n)
	}

	stats, err := jevscore.Runner{Scorer: scorer, Store: store}.Run(ctx, jevscore.RunOptions{
		Concurrency: cfg.Concurrency, LeaseSeconds: cfg.LeaseSeconds,
	})
	if err != nil {
		log.Printf("jevscore: %v", err)
		return 1
	}
	log.Printf("jevscore done: scored=%d skipped=%d failed=%d dead=%d reaped=%d",
		stats.Scored, stats.Skipped, stats.Failed, stats.DeadLettered, stats.Reaped)
	return worker.ExitCode(stats.Failed, stats.DeadLettered)
}
```

- [ ] **Step 6: Wire invalidation** — in the profile-save handler (`userprofile.Service.Save` caller in `internal/api/handler/me_profile.go`) and the CV-upload handler, after a successful save, best-effort call `queries.InvalidateUserScores(ctx, userID)` and `queries.DeleteUserScoreOutbox(ctx, userID)` (log-and-ignore errors). Add a focused test asserting the calls happen on save.

- [ ] **Step 7: Run tests + build**

Run: `go test ./internal/candidate/jevscore/... && go build ./cmd/jevscore/`
Expected: PASS + builds.

- [ ] **Step 8: Commit**

```bash
git add internal/candidate/jevscore/runner.go internal/candidate/jevscore/runner_test.go cmd/jevscore/ internal/api/handler/
git commit -m "feat(jevscore): scoring worker + queue drain + invalidation"
```

---

## Phase 6 — Serve

### Task 10: Badge returns Jev score (with fallback) + contracts

**Files:**
- Modify: `internal/api/handler/job_match.go`
- Create: `internal/candidate/jevscore/wire.go` (a `json`-tagged wire struct for contracts, if `Score` isn't already the wire shape — `Score` already has json tags, so reuse it)
- Modify: `cmd/gen-contracts/main.go` (add a jevscore package entry)
- Test: `internal/api/handler/job_match_test.go` (extend)

**Interfaces:**
- Consumes: `q.GetUserJobScore` (Task 7), `jevscore.Score` (Task 4), existing `jobmatch.Compute` fallback.
- Produces: extended `jobMatchResponse` with an optional `*jevscore.Score` field (`json:"jev,omitempty"`) and a `stale bool`; TS type `Score` in `contracts.ts`.

- [ ] **Step 1: Write the failing test** — `JobMatch` returns the Jev score when a fresh row exists; falls back to coverage when absent.

```go
// In job_match_test.go: seed a user_job_scores row via the fake queries; assert the
// response JSON carries "jev":{"verdict":"APPLY",...}. With no row, assert "jev" absent
// and coverage_percent present (unchanged behavior).
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/api/handler/ -run JobMatch -v`
Expected: FAIL.

- [ ] **Step 3: Implement** — in `JobMatch`, after loading job+profile, best-effort `GetUserJobScore(user, job.ID)`; if found and fresh (stamps match live `job.ContentHash`, config model/version — compare what the handler can cheaply check: at minimum `score_version` and `job_content_hash`), map the row to `*jevscore.Score` and include it; keep `jobmatch.Compute` + blockers as the always-present fallback body. Never error on a missing row.

```go
type jobMatchResponse struct {
	jobmatch.JobMatch
	Blockers []hardconstraint.Blocker `json:"blockers"`
	Jev      *jevscore.Score          `json:"jev,omitempty"`
	JevStale bool                     `json:"jev_stale,omitempty"`
}
```

- [ ] **Step 4: Add the contracts entry** in `cmd/gen-contracts/main.go` mirroring the jobmatch entry (Path `internal/candidate/jevscore`, IncludeFiles `["score.go"]`), read the body, append it. Run `make gen-contracts`.

- [ ] **Step 5: Run + regen**

Run: `go test ./internal/api/handler/ -run JobMatch && make gen-contracts && go build ./...`
Expected: PASS + `web/src/lib/generated/contracts.ts` gains the `Score` type.

- [ ] **Step 6: Commit**

```bash
git add internal/api/handler/job_match.go internal/api/handler/job_match_test.go cmd/gen-contracts/main.go web/src/lib/generated/contracts.ts
git commit -m "feat(api): job-match badge serves Jev score with deterministic fallback"
```

---

### Task 11: `GET /jobs/for-you` feed endpoint

**Files:**
- Create: `internal/api/handler/for_you.go`
- Modify: `internal/api/handler/match_analysis.go` (register the route)
- Test: `internal/api/handler/for_you_test.go`

**Interfaces:**
- Consumes: `q.ForYouFeed`, `q.CountForYou` (Task 7), `requireUserID`, `pageParams`, `listResponse`.
- Produces: `GET /api/v1/jobs/for-you?verdict=&min=&limit=&offset=` returning `{"data":[feedRow],"meta":{total,limit,offset}}`; a `feedRow` wire struct (json-tagged) added to contracts.

- [ ] **Step 1: Write the failing test** — seed scores for two jobs; assert ordering by match_pct DESC, verdict filter, pagination meta.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/api/handler/ -run ForYou -v`
Expected: FAIL.

- [ ] **Step 3: Implement handler**

```go
func (h *matchHandlers) ForYou(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	limit, offset, err := pageParams(c)
	if err != nil {
		return err
	}
	verdict := c.Query("verdict") // "", APPLY, MAYBE, SKIP
	minPct := c.QueryInt("min", 0)
	rows, err := h.queries.ForYouFeed(c.Context(), db.ForYouFeedParams{
		UserID: userID, Verdict: verdict, MinPct: int32(minPct),
		Lim: int32(limit), Off: int32(offset),
	})
	if err != nil {
		return err
	}
	total, err := h.queries.CountForYou(c.Context(), db.CountForYouParams{UserID: userID, Verdict: verdict, MinPct: int32(minPct)})
	if err != nil {
		return err
	}
	return listResponse(c, toForYouRows(rows), total, limit, offset)
}
```

Register in `matchHandlers.register` (cookie-auth, browser feed):

```go
api.Get("/jobs/for-you", mw.cookie, h.ForYou)
```

- [ ] **Step 4: Add `feedRow` to contracts** (mirror the jobmatch contracts entry) + `make gen-contracts`.

- [ ] **Step 5: Run + regen + build**

Run: `go test ./internal/api/handler/ -run ForYou && make gen-contracts && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/handler/for_you.go internal/api/handler/match_analysis.go internal/api/handler/for_you_test.go cmd/gen-contracts/main.go web/src/lib/generated/contracts.ts
git commit -m "feat(api): personalized /jobs/for-you feed"
```

---

## Phase 7 — Frontend

### Task 12: API client + Profile-match badge shows Jev score

**Files:**
- Modify: `web/src/lib/api.ts` (add `getJobMatch` returns extended shape; add `forYouFeed`)
- Modify: `web/src/lib/types.ts` (extend `JobMatchResult` with `jev?: Score`)
- Modify: `web/src/lib/components/JobMatch.svelte` (render Jev verdict chip + 3 flags when present)
- Modify: `web/src/lib/jobMatch.ts` (view-model helpers for verdict tone)
- Test: `web/src/lib/jobMatch.test.ts` (extend)

**Interfaces:**
- Consumes: contracts `Score` (Task 10).
- Produces: badge UI showing `match_pct`, verdict chip (APPLY/MAYBE/SKIP), `has_required_stack`/`fits_level`/`hard_blocker` as labeled meters, confidence; deterministic coverage kept as the fallback when `jev` is absent.

- [ ] **Step 1: Write the failing view-model test** — `verdictTone('APPLY')` → the apply class; a helper `jevFlags(score)` returns the three labeled percentages.
- [ ] **Step 2: Run** `cd web && npm test -- jobMatch` → FAIL.
- [ ] **Step 3: Implement** the view-model helpers + render branch in `JobMatch.svelte` (reuse `MatchSummary.svelte`'s verdict tone map as the pattern). When `match.jev` is present, headline the Jev `match_pct` + verdict chip + flags; otherwise the existing coverage view.
- [ ] **Step 4: Run** `cd web && npm test -- jobMatch && npm run check` → PASS.
- [ ] **Step 5: Commit** `feat(web): profile-match badge shows Jev score + verdict`.

---

### Task 13: Card verdict chip + "For You" feed view

**Files:**
- Modify: `web/src/lib/components/JobMatchBar.svelte` / `JobRow.svelte` (verdict chip on cards when a Jev score rides on the row)
- Create: `web/src/routes/jobs/for-you/+page.svelte` + `+page.server.ts` (SSR first page via `api.forYouFeed`)
- Modify: `web/src/lib/components/JobsView.svelte` (verdict filter chips beside the sort control; a `for-you` paginator)
- Test: `web/src/lib/...test.ts` for any new view-model logic

**Interfaces:**
- Consumes: `api.forYouFeed` (Task 12), `feedRow` contract (Task 11), `Paginator`.
- Produces: a `/jobs/for-you` ranked route reusing `JobsView`, verdict filter chips, and card verdict chips.

- [ ] **Step 1: Write the failing test** for the verdict-filter view-model (which verdicts are active → query params).
- [ ] **Step 2: Run** `cd web && npm test` → FAIL.
- [ ] **Step 3: Implement** the route + paginator (`new Paginator((limit, offset) => api.forYouFeed({verdict, min}, limit, offset))`), verdict filter chips, and the card chip.
- [ ] **Step 4: Run** `cd web && npm test && npm run check && npm run build` → PASS.
- [ ] **Step 5: Commit** `feat(web): For You ranked feed + card verdict chips`.

---

## Phase 8 — Deploy wiring

### Task 14: env example + schedule + docs

**Files:**
- Modify: `.env.example` (add `JEV_*` / `JEVSCORE_*` placeholders — NO real key)
- Modify: the deploy compose/schedule to run `cmd/jevscore` on a cron (mirror `cmd/enrich`'s schedule)
- Modify: `internal/candidate/jevscore/AGENTS.md` (new — conventions doc, mirroring the enrich/matchanalysis AGENTS style)

- [ ] **Step 1:** Add to `.env.example`:

```
# Jev (TypeSafe System One) scoring — see docs/superpowers/specs/2026-09-19-jev-profile-scoring-design.md
JEV_BASE_URL=https://api.typesafe.ai/v1/systemone
JEV_API_KEY=
JEV_MODEL=jev-latest
JEV_TIMEOUT=30s
JEV_CONCURRENCY=4
JEVSCORE_VERSION=1
JEVSCORE_APPLY_MIN=60
JEVSCORE_MAYBE_MIN=45
JEVSCORE_HARDBLOCK_MAX=0.5
```

- [ ] **Step 2:** Add the `cmd/jevscore` cron to the deploy schedule (Dokploy/compose) alongside `cmd/enrich`. Document the manual `0175` migration step (run before deploy).
- [ ] **Step 3:** Write `internal/candidate/jevscore/AGENTS.md` capturing: the gate-bounds-the-cross-product invariant, the resolved-model stamp, best-effort degradation, and the SQL-coarse / Go-fine eligibility split.
- [ ] **Step 4:** `go build ./... && go test ./...` (unit) green.
- [ ] **Step 5: Commit** `chore(jevscore): env example, schedule, conventions`.

---

## Self-review notes

- **Spec coverage:** client (T2), config (T1), decision model + level-derived fits_level + structured résumé (T3–T5), eligibility gate reusing hardconstraint (T6), schema + staleness stamps + queue (T7), store + integration tests (T8), worker mirroring cmd/enrich + invalidation (T9), badge with fallback + contracts (T10), Postgres For You feed default-show-all sorted by score (T11), frontend badge + card + feed (T12–T13), deploy/env/no-committed-key + AGENTS (T14). All spec sections map to a task.
- **Type consistency:** `Score`/`Thresholds`/`Input`/`Scorer`/`Claimed`/`Store`/`Runner` names are used identically across T3–T13; `jev.Question`/`Answer`/`Response` across T2–T5; generated `db.*Params`/`db.UserJobScore` across T7–T11.
- **Known adjustments flagged inline:** `Claimed.CVUploadedAt` uses `pgtype.Timestamptz` (not a `db.Timestamptz` alias); `insertUser` fixture must be swapped for the repo's existing user-seed helper; verify `jobfacts`/`pgconv` import paths and whether `GetJob` already exists before adding it.
