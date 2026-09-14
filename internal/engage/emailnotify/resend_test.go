package emailnotify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

func TestResendTransportSendsTheMessage(t *testing.T) {
	var got resendRequest
	var auth, contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		contentType = r.Header.Get("Content-Type")
		if r.Method != http.MethodPost || r.URL.Path != "/emails" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"mail_1"}`))
	}))
	defer srv.Close()

	c := NewResendClient("re_test")
	c.resend.baseURL = srv.URL
	err := c.Send(context.Background(), Message{
		From:           "freehire <no-reply@example.test>",
		To:             "someone@example.test",
		Subject:        "Weekly digest",
		HTML:           "<p>hi</p>",
		Text:           "hi",
		Group:          emailprefs.GroupAlerts,
		UnsubscribeURL: "https://example.test/unsubscribe/abc",
		ReplyTo:        "founder@example.test",
		Attachments:    []Attachment{{Filename: "invite.ics", ContentType: "text/calendar; method=REQUEST", Content: []byte("BEGIN:VCALENDAR")}},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if auth != "Bearer re_test" || contentType != "application/json" {
		t.Errorf("headers: auth=%q content-type=%q", auth, contentType)
	}
	if got.From != "freehire <no-reply@example.test>" || len(got.To) != 1 || got.To[0] != "someone@example.test" {
		t.Errorf("addresses: %+v", got)
	}
	if got.Subject != "Weekly digest" || got.HTML != "<p>hi</p>" || got.Text != "hi" {
		t.Errorf("content: %+v", got)
	}
	if len(got.ReplyTo) != 1 || got.ReplyTo[0] != "founder@example.test" {
		t.Errorf("reply_to: %v", got.ReplyTo)
	}
	if got.Headers["List-Unsubscribe"] != "<https://example.test/unsubscribe/abc>" || got.Headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" {
		t.Errorf("unsubscribe headers: %v", got.Headers)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Filename != "invite.ics" || got.Attachments[0].Content != "QkVHSU46VkNBTEVOREFS" || got.Attachments[0].ContentType != "text/calendar; method=REQUEST" {
		t.Errorf("attachments: %+v", got.Attachments)
	}
}

func TestResendTransportReportsAPIErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"statusCode":403,"name":"validation_error","message":"domain is not verified"}`))
	}))
	defer srv.Close()

	c := NewResendClient("re_test")
	c.resend.baseURL = srv.URL
	err := c.Send(context.Background(), Message{From: "a@example.test", To: "b@example.test", Subject: "x", Text: "x", Group: emailprefs.GroupEssential})
	if err == nil {
		t.Fatal("expected an error for a 403")
	}
	if want := "HTTP 403"; !contains(err.Error(), want) || !contains(err.Error(), "domain is not verified") {
		t.Errorf("error %q should carry the status and the API message", err)
	}
}

func TestNewClientPrefersResendWhenConfigured(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "re_test")
	c, err := NewClient(context.Background(), "")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.resend == nil || c.ses != nil {
		t.Errorf("expected a Resend-backed client, got %+v", c)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
