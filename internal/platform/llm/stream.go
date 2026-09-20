package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// streamPin writes `"stream": false` onto an outgoing chat request that says nothing
// about streaming, and is a pass-through otherwise.
//
// It exists because the two sides of the wire disagree about what silence means.
// langchaingo tags the field `json:"stream,omitempty"`, so a non-streaming call — every
// GenerateJSON, every enrichment — omits it entirely. The OpenAI contract reads that
// omission as false. The 9router gateway does not: measured 2026-09-20, every provider
// behind it except the Codex (cx/) routes answered an omitted `stream` with
// `Content-Type: text/event-stream` and a `data: {...}` body, which the client then tried
// to decode as one JSON object and reported as `invalid character 'd' looking for
// beginning of value`. The enrich worker failed 5,977 calls in a row that way after
// LLM_MODEL moved to a Kiro route, and with nothing enriched nothing was eligible for
// Jev scoring. The same request with an explicit `"stream": false` came back as plain
// JSON on every route probed.
//
// On the transport rather than in the request builder because langchaingo owns that
// builder (the same seam, and for the same reason, as reasoningInjector next to it). A
// request that already carries `stream` — the streaming entry points set it to true —
// is left exactly as written.
type streamPin struct {
	next http.RoundTripper
}

func (t *streamPin) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}
	if req.Body == nil {
		return next.RoundTrip(req)
	}

	// A RoundTripper must close the request body on every path, not only the one that
	// reaches the wire.
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: read request body: %w", err)
	}

	patched, err := withStreamPinned(body)
	if err != nil {
		return nil, err
	}

	clone := req.Clone(req.Context())
	clone.Body = io.NopCloser(bytes.NewReader(patched))
	clone.ContentLength = int64(len(patched))
	clone.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(patched)), nil
	}

	return next.RoundTrip(clone)
}

// withStreamPinned sets the stream member of a chat request to false when it is absent,
// leaving every other member — and a present stream, whatever its value — as langchaingo
// wrote it. A body that is not a JSON object is returned unchanged: the transport is
// installed on every client, and nothing guarantees only chat requests reach it.
func withStreamPinned(body json.RawMessage) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &fields); err != nil {
		return body, nil //nolint:nilerr // not a JSON object: not ours to rewrite
	}
	// A literal `null` unmarshals into a nil map without error; see withReasoningEffort
	// for why a write to it here would be fatal rather than merely wrong.
	if fields == nil {
		return body, nil
	}
	if _, present := fields["stream"]; present {
		return body, nil
	}
	fields["stream"] = json.RawMessage("false")

	patched, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("llm: encode patched request: %w", err)
	}

	return patched, nil
}
