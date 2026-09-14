package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type captureTransport struct{ body string }

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(req.Body)
	c.body = string(b)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{}}, nil
}

func effortSent(t *testing.T, ctx context.Context) (string, bool) {
	t.Helper()
	cap := &captureTransport{}
	tr := &reasoningInjector{next: cap}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://gateway/v1/chat/completions", strings.NewReader(`{"model":"m","messages":[]}`))
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("round trip: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cap.body), &fields); err != nil {
		t.Fatalf("decode body %q: %v", cap.body, err)
	}
	raw, ok := fields["reasoning_effort"]
	if !ok {
		return "", false
	}
	var v string
	_ = json.Unmarshal(raw, &v)
	return v, true
}

func TestEnvDefaultAppliesWhenTheCallNamesNoEffort(t *testing.T) {
	t.Setenv("LLM_REASONING_EFFORT", "xhigh")
	if got, ok := effortSent(t, context.Background()); !ok || got != "xhigh" {
		t.Errorf("want reasoning_effort=xhigh from the environment, got %q (present=%v)", got, ok)
	}
}

func TestExplicitEffortWinsOverTheEnvDefault(t *testing.T) {
	t.Setenv("LLM_REASONING_EFFORT", "xhigh")
	ctx := withReasoning(context.Background(), ReasoningNone)
	if got, ok := effortSent(t, ctx); !ok || got != "none" {
		t.Errorf("want the call's own \"none\" to survive, got %q (present=%v)", got, ok)
	}
}

func TestNoEnvNoEffortSendsNothing(t *testing.T) {
	t.Setenv("LLM_REASONING_EFFORT", "")
	if got, ok := effortSent(t, context.Background()); ok {
		t.Errorf("expected no reasoning_effort field, got %q", got)
	}
}
