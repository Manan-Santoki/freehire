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
