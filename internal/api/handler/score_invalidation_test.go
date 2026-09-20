package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
)

// fakeScoreInvalidator is a scoreInvalidator that records every call, so a test can assert
// PutProfile/PutResume actually invoke both the cache drop and the outbox cleanup after a
// successful save/upload, rather than just trusting the call site.
type fakeScoreInvalidator struct {
	mu                sync.Mutex
	invalidateCalls   []int64
	deleteOutboxCalls []int64
}

func (f *fakeScoreInvalidator) InvalidateUserScores(_ context.Context, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidateCalls = append(f.invalidateCalls, userID)
	return nil
}

func (f *fakeScoreInvalidator) DeleteUserScoreOutbox(_ context.Context, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteOutboxCalls = append(f.deleteOutboxCalls, userID)
	return nil
}

func (f *fakeScoreInvalidator) calls() (invalidate, deleteOutbox []int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.invalidateCalls...), append([]int64(nil), f.deleteOutboxCalls...)
}

// TestPutProfile_InvalidatesScoresOnSuccess is the regression test for the score-
// invalidation wiring in PutProfile: a successful Save must invalidate the caller's cached
// Jev scores and drain their pending score-outbox entries, both fire-and-forget but both
// still expected to happen. invalidateUserScores runs synchronously on the request
// goroutine (see me_profile.go), so no synchronization beyond the HTTP round trip is
// needed to observe the calls.
func TestPutProfile_InvalidatesScoresOnSuccess(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	repo := &fakeProfileRepo{upsertRet: userprofile.Profile{UserID: 1, Specializations: []string{"backend"}, Skills: []string{"go"}}}
	scores := &fakeScoreInvalidator{}
	h := &profileHandlers{userProfile: userprofile.New(repo), scores: scores}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Put("/me/profile", auth.RequireAuth(iss, testVersions), h.PutProfile)

	resp := doProfile(t, app, fiber.MethodPut, `{"specializations":["backend"],"skills":["go"]}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	invalidate, deleteOutbox := scores.calls()
	if len(invalidate) != 1 || invalidate[0] != 1 {
		t.Errorf("InvalidateUserScores calls = %v, want [1]", invalidate)
	}
	if len(deleteOutbox) != 1 || deleteOutbox[0] != 1 {
		t.Errorf("DeleteUserScoreOutbox calls = %v, want [1]", deleteOutbox)
	}
}

// TestPutProfile_DoesNotInvalidateScoresOnValidationError guards the other half of the
// wiring: an invalid body never reaches Save, so it must never invalidate scores either.
func TestPutProfile_DoesNotInvalidateScoresOnValidationError(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	repo := &fakeProfileRepo{}
	scores := &fakeScoreInvalidator{}
	h := &profileHandlers{userProfile: userprofile.New(repo), scores: scores}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Put("/me/profile", auth.RequireAuth(iss, testVersions), h.PutProfile)

	resp := doProfile(t, app, fiber.MethodPut, `{"specializations":[],"skills":["go"]}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}

	invalidate, deleteOutbox := scores.calls()
	if len(invalidate) != 0 || len(deleteOutbox) != 0 {
		t.Errorf("scores invalidated on a rejected save: invalidate=%v deleteOutbox=%v", invalidate, deleteOutbox)
	}
}

// TestPutResume_InvalidatesScoresOnSuccess is the regression test for the score-
// invalidation wiring in PutResume: a successful résumé upload must invalidate the
// caller's cached Jev scores and drain their pending score-outbox entries. Like
// PutProfile, invalidateUserScores runs synchronously before the response is written (see
// resume.go), before the unrelated background structured-extraction goroutine kicks off.
func TestPutResume_InvalidatesScoresOnSuccess(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	store := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{})
	scores := &fakeScoreInvalidator{}
	h := &resumeHandlers{resume: store, scores: scores}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Put("/me/resume", auth.RequireAuth(iss, testVersions), h.PutResume)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPut, "/me/resume", strings.NewReader(`{"text":"Go and PostgreSQL"}`))
	req.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	invalidate, deleteOutbox := scores.calls()
	if len(invalidate) != 1 || invalidate[0] != 1 {
		t.Errorf("InvalidateUserScores calls = %v, want [1]", invalidate)
	}
	if len(deleteOutbox) != 1 || deleteOutbox[0] != 1 {
		t.Errorf("DeleteUserScoreOutbox calls = %v, want [1]", deleteOutbox)
	}
}

// TestPutResume_DisabledStorageDoesNotInvalidateScores guards the other half: when object
// storage is unconfigured, PutResume 501s before Put ever runs, so scores must stay
// untouched.
func TestPutResume_DisabledStorageDoesNotInvalidateScores(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	store := resume.New(nil, &fakeResumeRepo{}) // nil blob store → storage disabled
	scores := &fakeScoreInvalidator{}
	h := &resumeHandlers{resume: store, scores: scores}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Put("/me/resume", auth.RequireAuth(iss, testVersions), h.PutResume)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPut, "/me/resume", strings.NewReader(`{"text":"Go and PostgreSQL"}`))
	req.Header.Set("Content-Type", fiber.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", resp.StatusCode)
	}

	invalidate, deleteOutbox := scores.calls()
	if len(invalidate) != 0 || len(deleteOutbox) != 0 {
		t.Errorf("scores invalidated on a disabled-storage 501: invalidate=%v deleteOutbox=%v", invalidate, deleteOutbox)
	}
}
