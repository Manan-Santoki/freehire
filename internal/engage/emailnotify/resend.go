package emailnotify

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// resendTransport delivers through Resend's HTTP API
// (https://resend.com/docs/api-reference/emails/send-email) for deployments that have
// no AWS account. Setting RESEND_API_KEY makes NewClient hand back a Client that sends
// this way; every caller keeps composing the same Message and the SES path is
// untouched. The From address must belong to a domain verified in the Resend
// dashboard, exactly as SES requires a verified identity.
type resendTransport struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

const resendBaseURL = "https://api.resend.com"

// NewResendClient builds a Client backed by Resend. Prefer NewClient, which picks this
// transport by itself when RESEND_API_KEY is set.
func NewResendClient(apiKey string) *Client {
	return &Client{resend: &resendTransport{
		apiKey:  apiKey,
		baseURL: resendBaseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}}
}

type resendRequest struct {
	From        string             `json:"from"`
	To          []string           `json:"to"`
	Subject     string             `json:"subject"`
	HTML        string             `json:"html,omitempty"`
	Text        string             `json:"text,omitempty"`
	ReplyTo     []string           `json:"reply_to,omitempty"`
	Headers     map[string]string  `json:"headers,omitempty"`
	Attachments []resendAttachment `json:"attachments,omitempty"`
}

type resendAttachment struct {
	Filename    string `json:"filename"`
	Content     string `json:"content"`
	ContentType string `json:"content_type,omitempty"`
}

func (t *resendTransport) request(m Message) resendRequest {
	req := resendRequest{From: m.From, To: []string{m.To}, Subject: m.Subject, HTML: m.HTML, Text: m.Text}
	if m.ReplyTo != "" {
		req.ReplyTo = []string{m.ReplyTo}
	}
	for _, h := range m.unsubscribeHeaders() {
		if req.Headers == nil {
			req.Headers = map[string]string{}
		}
		req.Headers[aws.ToString(h.Name)] = aws.ToString(h.Value)
	}
	for _, a := range m.Attachments {
		req.Attachments = append(req.Attachments, resendAttachment{
			Filename:    a.Filename,
			Content:     base64.StdEncoding.EncodeToString(a.Content),
			ContentType: a.ContentType,
		})
	}
	return req
}

func (t *resendTransport) send(ctx context.Context, m Message) error {
	body, err := json.Marshal(t.request(m))
	if err != nil {
		return fmt.Errorf("emailnotify: resend encode: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("emailnotify: resend request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+t.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := t.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("emailnotify: resend send to %s: %w", m.To, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("emailnotify: resend send to %s: HTTP %d: %s", m.To, resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	return nil
}
