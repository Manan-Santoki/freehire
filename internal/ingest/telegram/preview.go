package telegram

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// Post is one message parsed from a channel's public web preview page.
type Post struct {
	MsgID    int64
	PostedAt time.Time
	Text     string
	// Links are the message's outbound hyperlinks (anchor text + href), preserved so the
	// extraction step can follow them; nodeText keeps Text itself clean of raw URLs.
	Links []Link
}

// Link is one hyperlink found in a message: its visible anchor text and its href.
type Link struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// ParsePreview extracts the posts from a t.me/s/<channel> preview page. The page
// carries the ~20 latest posts; each is identified by a data-post="<channel>/<id>"
// attribute, with its timestamp in a <time datetime=...> and its body in the
// message-text div. A page with zero parseable posts is an error — it means the
// preview is disabled or the markup drifted, and the caller must fail the channel
// loudly rather than treat it as empty.
func ParsePreview(channel string, page []byte) ([]Post, error) {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil, fmt.Errorf("telegram: parse preview html for %s: %w", channel, err)
	}

	var posts []Post
	prefix := channel + "/"
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if id, ok := postID(n, prefix); ok {
				if p, ok := parseMessage(n, id); ok {
					posts = append(posts, p)
				}
				return // a message node holds no nested messages
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if len(posts) == 0 {
		return nil, fmt.Errorf("telegram: no posts parsed from preview of %s (preview disabled or markup changed)", channel)
	}
	return posts, nil
}

// postID reads a node's data-post attribute, returning the message id when it
// belongs to the given channel (case-insensitive — t.me usernames are).
func postID(n *html.Node, prefix string) (int64, bool) {
	for _, a := range n.Attr {
		if a.Key != "data-post" {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(a.Val), strings.ToLower(prefix)) {
			return 0, false
		}
		id, err := strconv.ParseInt(a.Val[len(prefix):], 10, 64)
		return id, err == nil
	}
	return 0, false
}

// parseMessage extracts the timestamp and text from one message node. A message
// without a timestamp or with an empty body (e.g. photo-only) is skipped.
func parseMessage(n *html.Node, id int64) (Post, bool) {
	var postedAt time.Time
	var text string
	var links []Link

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch {
			case n.Data == "time":
				if dt := sources.Attr(n, "datetime"); dt != "" {
					if t, err := time.Parse(time.RFC3339, dt); err == nil {
						postedAt = t.UTC()
					}
				}
			case sources.HasClass(n, "tgme_widget_message_text"):
				text = nodeText(n)
				links = messageLinks(n)
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	text = strings.TrimSpace(text)
	if postedAt.IsZero() || text == "" {
		return Post{}, false
	}
	return Post{MsgID: id, PostedAt: postedAt, Text: text, Links: links}, true
}

// messageLinks collects the message's anchors as {text, href} in document order. A blank
// href is skipped; the anchor text is flattened the same way as the body so it reads
// cleanly. The hrefs are dropped from nodeText, so this is the only place they survive.
func messageLinks(n *html.Node) []Link {
	var links []Link
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			if href := sources.Attr(n, "href"); href != "" {
				links = append(links, Link{Text: strings.TrimSpace(nodeText(n)), URL: href})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return links
}

// nodeText flattens a node to plain text: entities are already decoded by the
// parser, tags are dropped, and <br> becomes a newline so paragraph structure
// survives for the extraction prompt.
func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			b.WriteString(n.Data)
		case n.Type == html.ElementNode && n.Data == "br":
			b.WriteString("\n")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
