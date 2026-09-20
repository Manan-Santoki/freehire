package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeForYouStore is a forYouStore that filters/orders/paginates a fixed set of rows the
// same way the SQL does, so ForYou's handling of the params and the envelope is exercised
// without Postgres.
type fakeForYouStore struct {
	rows []db.ForYouFeedRow
}

// filtered mimics the SQL's WHERE clause and its ORDER BY match_pct DESC (ties don't
// matter for these tests, so the fake doesn't replicate the "j.id DESC" tiebreak).
func (f fakeForYouStore) filtered(arg db.CountForYouParams) []db.ForYouFeedRow {
	var out []db.ForYouFeedRow
	for _, r := range f.rows {
		if arg.Verdict != "" && r.Verdict != arg.Verdict {
			continue
		}
		if r.MatchPct < int16(arg.MinPct) {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].MatchPct > out[j].MatchPct })
	return out
}

func (f fakeForYouStore) ForYouFeed(_ context.Context, arg db.ForYouFeedParams) ([]db.ForYouFeedRow, error) {
	rows := f.filtered(db.CountForYouParams{UserID: arg.UserID, Verdict: arg.Verdict, MinPct: arg.MinPct})
	off, lim := int(arg.Off), int(arg.Lim)
	if off >= len(rows) {
		return []db.ForYouFeedRow{}, nil
	}
	end := off + lim
	if end > len(rows) {
		end = len(rows)
	}
	return rows[off:end], nil
}

func (f fakeForYouStore) CountForYou(_ context.Context, arg db.CountForYouParams) (int64, error) {
	return int64(len(f.filtered(arg))), nil
}

// forYouApp mounts the cookie-auth /jobs/for-you route on a matchHandlers backed by the
// given fake store.
func forYouApp(store forYouStore) (*fiber.App, *auth.Issuer) {
	h := &matchHandlers{forYou: store}
	iss := auth.NewIssuer("test-secret", time.Hour)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/for-you", auth.RequireAuth(iss, testVersions), h.ForYou)
	return app, iss
}

type forYouBody struct {
	Data []ForYouJob `json:"data"`
	Meta struct {
		Total  int64 `json:"total"`
		Limit  int   `json:"limit"`
		Offset int   `json:"offset"`
	} `json:"meta"`
}

func getForYou(t *testing.T, app *fiber.App, iss *auth.Issuer, query string) forYouBody {
	t.Helper()
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/jobs/for-you"+query, nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET for-you: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body forYouBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func seedRows() []db.ForYouFeedRow {
	return []db.ForYouFeedRow{
		{PublicSlug: "job-a", Title: "Backend Engineer", MatchPct: 60, Verdict: "MAYBE"},
		{PublicSlug: "job-b", Title: "Staff Engineer", MatchPct: 90, Verdict: "APPLY"},
		{PublicSlug: "job-c", Title: "Junior Engineer", MatchPct: 20, Verdict: "SKIP"},
	}
}

func TestForYou_OrdersByMatchPctDescending(t *testing.T) {
	store := fakeForYouStore{rows: seedRows()}
	app, iss := forYouApp(store)

	body := getForYou(t, app, iss, "")

	if len(body.Data) != 3 {
		t.Fatalf("len(data) = %d, want 3", len(body.Data))
	}
	want := []string{"job-b", "job-a", "job-c"}
	for i, slug := range want {
		if body.Data[i].Slug != slug {
			t.Errorf("data[%d].slug = %q, want %q (order = %+v)", i, body.Data[i].Slug, slug, body.Data)
		}
	}
	if body.Meta.Total != 3 {
		t.Errorf("meta.total = %d, want 3", body.Meta.Total)
	}
}

func TestForYou_FiltersByVerdict(t *testing.T) {
	store := fakeForYouStore{rows: seedRows()}
	app, iss := forYouApp(store)

	body := getForYou(t, app, iss, "?verdict=APPLY")

	if len(body.Data) != 1 || body.Data[0].Slug != "job-b" {
		t.Fatalf("data = %+v, want only job-b", body.Data)
	}
	if body.Meta.Total != 1 {
		t.Errorf("meta.total = %d, want 1", body.Meta.Total)
	}
}

func TestForYou_FiltersByMinMatchPct(t *testing.T) {
	store := fakeForYouStore{rows: seedRows()}
	app, iss := forYouApp(store)

	body := getForYou(t, app, iss, "?min=50")

	if len(body.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2 (job-a, job-b)", len(body.Data))
	}
	for _, job := range body.Data {
		if job.Slug == "job-c" {
			t.Errorf("data contains job-c (match_pct=20), want filtered out by min=50")
		}
	}
	if body.Meta.Total != 2 {
		t.Errorf("meta.total = %d, want 2", body.Meta.Total)
	}
}

func TestForYou_PaginationEnvelope(t *testing.T) {
	store := fakeForYouStore{rows: seedRows()}
	app, iss := forYouApp(store)

	body := getForYou(t, app, iss, "?limit=1&offset=1")

	if len(body.Data) != 1 || body.Data[0].Slug != "job-a" {
		t.Fatalf("data = %+v, want only job-a (second-best match, offset 1)", body.Data)
	}
	if body.Meta.Total != 3 {
		t.Errorf("meta.total = %d, want 3 (unfiltered total, not page size)", body.Meta.Total)
	}
	if body.Meta.Limit != 1 {
		t.Errorf("meta.limit = %d, want 1", body.Meta.Limit)
	}
	if body.Meta.Offset != 1 {
		t.Errorf("meta.offset = %d, want 1", body.Meta.Offset)
	}
}
