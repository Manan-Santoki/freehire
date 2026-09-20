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
