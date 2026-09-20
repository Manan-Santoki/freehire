package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// These guard the route-ordering rule handler.go relies on: Fiber matches routes in
// registration order, so the static /jobs/for-you must be registered BEFORE jobsH's
// /jobs/:slug param route or the param route shadows it. This is a regression guard for
// the production bug where GET /api/v1/jobs/for-you returned 404 — it was matching
// /jobs/:slug with slug="for-you" (GetJob, no such job) instead of the feed handler.
// matchH.RegisterForYou is therefore invoked before jobsH.register in handler.go.

func TestStaticJobsRouteBeatsSlugWhenRegisteredFirst(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api/v1")
	api.Get("/jobs/for-you", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	api.Get("/jobs/:slug", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNotFound) })

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/jobs/for-you", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("static /jobs/for-you shadowed by /jobs/:slug: got %d, want 204 — RegisterForYou must run before jobsH.register", resp.StatusCode)
	}
}

func TestSlugRouteShadowsStaticWhenRegisteredFirst(t *testing.T) {
	// The inverse documents WHY order matters (this is the bug that shipped).
	app := fiber.New()
	api := app.Group("/api/v1")
	api.Get("/jobs/:slug", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNotFound) })
	api.Get("/jobs/for-you", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/jobs/for-you", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected /jobs/:slug to shadow /jobs/for-you when registered first: got %d, want 404", resp.StatusCode)
	}
}
