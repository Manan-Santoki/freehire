package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The regression guard for the 2026-09-20 enrichment outage: a non-streaming call must
// say `"stream": false` on the WIRE. langchaingo omits the field when false, and the
// 9router gateway reads an omitted stream as a request to stream, answering with an SSE
// body the client cannot decode.
func TestNonStreamingCallPinsStreamFalse(t *testing.T) {
	rec := newBodyRecorder(t)

	if _, err := rec.client(t).GenerateJSON(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	got, ok := rec.last(t)["stream"]
	if !ok {
		t.Fatal("stream is absent from the request; the gateway will stream the reply")
	}
	if string(got) != "false" {
		t.Errorf("stream = %s, want false", got)
	}
}

// A streaming call already says `"stream": true`; the pin must not touch it.
func TestStreamingCallKeepsStreamTrue(t *testing.T) {
	var got map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(raw, &got)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"{}\"}}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	c, err := New(srv.URL, "service-key", "model-x")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.GenerateJSONStream(context.Background(), "sys", "usr", func(string) {}); err != nil {
		t.Fatalf("GenerateJSONStream: %v", err)
	}

	if string(got["stream"]) != "true" {
		t.Errorf("stream = %s, want true", got["stream"])
	}
}

func TestWithStreamPinned(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"absent is pinned false", `{"model":"m"}`, `{"model":"m","stream":false}`},
		{"present true is kept", `{"model":"m","stream":true}`, `{"model":"m","stream":true}`},
		{"present false is kept", `{"model":"m","stream":false}`, `{"model":"m","stream":false}`},
		{"non-object passes through", `[1,2]`, `[1,2]`},
		{"null passes through", `null`, `null`},
		{"garbage passes through", `not json`, `not json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := withStreamPinned(json.RawMessage(tc.in))
			if err != nil {
				t.Fatalf("withStreamPinned: %v", err)
			}
			if strings.TrimSpace(string(got)) != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}
