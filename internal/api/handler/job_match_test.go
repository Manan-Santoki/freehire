package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeJobMatchStore is a jobMatchStore that returns canned rows, so the Jev/coverage
// fallback in JobMatch is exercised without Postgres.
type fakeJobMatchStore struct {
	job db.Job

	scoreRow db.GetUserJobScoreRow
	scoreErr error // pgx.ErrNoRows for "never scored"; nil to serve scoreRow
}

func (f fakeJobMatchStore) GetJobBySlug(context.Context, string) (db.Job, error) {
	return f.job, nil
}

func (f fakeJobMatchStore) GetUserJobScore(context.Context, db.GetUserJobScoreParams) (db.GetUserJobScoreRow, error) {
	if f.scoreErr != nil {
		return db.GetUserJobScoreRow{}, f.scoreErr
	}
	return f.scoreRow, nil
}

// jobMatchApp mounts the read-only /jobs/:slug/match route behind RequireAuth on a
// matchHandlers backed by the given fake store and profile repo.
func jobMatchApp(store jobMatchStore, repo userprofile.Repository) (*fiber.App, *auth.Issuer) {
	h := &matchHandlers{
		store:       store,
		userProfile: userprofile.New(repo),
	}
	iss := auth.NewIssuer("test-secret", time.Hour)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/:slug/match", auth.RequireAuth(iss, testVersions), h.JobMatch)
	return app, iss
}

type jobMatchBody struct {
	Data struct {
		CoveragePercent int              `json:"coverage_percent"`
		Jev             *json.RawMessage `json:"jev"`
		JevStale        bool             `json:"jev_stale"`
	} `json:"data"`
}

func TestJobMatch_ServesJevScoreWhenFresh(t *testing.T) {
	job := db.Job{
		ID:          42,
		Skills:      []string{"go", "postgres"},
		ContentHash: pgtype.Text{String: "hash-1", Valid: true},
	}
	store := fakeJobMatchStore{
		job: job,
		scoreRow: db.GetUserJobScoreRow{
			MatchPct:         85,
			MatchRaw:         4,
			MatchConfidence:  0.9,
			RoleCategory:     "backend",
			RoleConfidence:   0.8,
			HasRequiredStack: 1,
			FitsLevel:        1,
			HardBlocker:      0,
			Verdict:          "APPLY",
			JobContentHash:   pgtype.Text{String: "hash-1", Valid: true},
		},
	}
	repo := &fakeProfileRepo{getRet: userprofile.Profile{Skills: []string{"go"}}}
	app, iss := jobMatchApp(store, repo)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/jobs/some-job/match", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET match: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Data struct {
			CoveragePercent int `json:"coverage_percent"`
			Jev             struct {
				Verdict  string `json:"verdict"`
				MatchPct int    `json:"match_pct"`
			} `json:"jev"`
			JevStale bool `json:"jev_stale"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Jev.Verdict != "APPLY" || body.Data.Jev.MatchPct != 85 {
		t.Errorf("jev = %+v, want verdict=APPLY match_pct=85", body.Data.Jev)
	}
	if body.Data.JevStale {
		t.Error("jev_stale = true, want false (content hash matches)")
	}
	// Coverage stays present alongside the Jev score.
	if body.Data.CoveragePercent != 50 {
		t.Errorf("coverage_percent = %d, want 50 (go matched, postgres missing)", body.Data.CoveragePercent)
	}
}

func TestJobMatch_StaleWhenJobContentHashMoved(t *testing.T) {
	job := db.Job{
		ID:          42,
		Skills:      []string{"go"},
		ContentHash: pgtype.Text{String: "hash-2", Valid: true},
	}
	store := fakeJobMatchStore{
		job: job,
		scoreRow: db.GetUserJobScoreRow{
			Verdict:        "APPLY",
			JobContentHash: pgtype.Text{String: "hash-1", Valid: true},
		},
	}
	repo := &fakeProfileRepo{getRet: userprofile.Profile{Skills: []string{"go"}}}
	app, iss := jobMatchApp(store, repo)
	token, _ := iss.Issue(1, testTokenVersion)

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/jobs/some-job/match", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET match: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Data struct {
			JevStale bool `json:"jev_stale"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Data.JevStale {
		t.Error("jev_stale = false, want true (content hash moved)")
	}
}

func TestJobMatch_FallsBackToCoverageWithoutAScore(t *testing.T) {
	job := db.Job{
		ID:          42,
		Skills:      []string{"go", "kafka"},
		ContentHash: pgtype.Text{String: "hash-1", Valid: true},
	}
	store := fakeJobMatchStore{job: job, scoreErr: pgx.ErrNoRows}
	repo := &fakeProfileRepo{getRet: userprofile.Profile{Skills: []string{"go"}}}
	app, iss := jobMatchApp(store, repo)
	token, _ := iss.Issue(1, testTokenVersion)

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/jobs/some-job/match", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET match: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body jobMatchBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Jev != nil {
		t.Errorf("jev = %s, want absent (omitempty, no scored row)", *body.Data.Jev)
	}
	if body.Data.JevStale {
		t.Error("jev_stale = true, want false/absent without a score")
	}
	if body.Data.CoveragePercent != 50 {
		t.Errorf("coverage_percent = %d, want 50 (go matched, kafka missing)", body.Data.CoveragePercent)
	}
}
