package sources

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// hackernews adapts Hacker News' monthly "Ask HN: Who is hiring?" threads through the Algolia
// HN Search API, which serves the whole thread — every top-level comment with its HTML — as
// one keyless JSON document. Each top-level comment is one employer's post; replies are
// discussion and are not read.
//
// # Boardless, and what a crawl reads
//
// There is no board: the "whoishiring" account posts one hiring thread a month, and a crawl
// reads the two newest (this month's, still filling, and last month's, still live in the
// first weeks of the new one). The thread search is filtered by that account's own author tag
// rather than by free text, because a free-text query is ranked and lets an unrelated recent
// story outrank the thread; the account also posts the sibling "Who wants to be hired?" and
// "Freelancer?" threads, so the title is checked against hackernewsHiringTitle. Both threads
// are read whole and any failure fails the crawl, so the provider is a fullCatalog: a post
// from a thread that has aged out of the window is genuinely no longer offered, and the
// source-scoped sweep closes it.
//
// # The post format is a convention, not a schema
//
// The thread's own instructions ask for "Company | Role | Location | ..." on the first line,
// and most posts follow it, with extra segments for commitment, salary, remote policy and an
// apply link. The first paragraph (up to the first <p>) is read as that header: the first
// pipe segment is the employer and the second the role; a post with fewer than two segments
// names no employer this catalogue could file it under and is dropped. The location is the
// first later segment that is not a commitment word, a URL or a salary. The whole comment,
// header included, is the body. The apply link is the first anchor in the post, falling back
// to the comment's own permalink.
type hackernews struct {
	http JSONGetter
}

// NewHackerNews builds the adapter over the given HTTP client.
func NewHackerNews(c JSONGetter) Source { return hackernews{http: c} }

func (hackernews) Provider() string { return "hackernews" }

// One global thread, no per-tenant board.
func (hackernews) boardless() {}

// Every post names its own employer.
func (hackernews) aggregator() {}

// Both threads are read whole every run and a failed read fails the crawl.
func (hackernews) fullCatalog() {}

const (
	// hackernewsThreadSearchURL lists the whoishiring account's newest stories. Ten covers the
	// two hiring threads wanted plus their sibling threads.
	hackernewsThreadSearchURL = "https://hn.algolia.com/api/v1/search_by_date?tags=story,author_whoishiring&hitsPerPage=10"
	// hackernewsItemURL is one story with its full comment tree.
	hackernewsItemURL = "https://hn.algolia.com/api/v1/items/%d"
	// hackernewsPermalink is a comment's own page, the fallback link for a post without one.
	hackernewsPermalink = "https://news.ycombinator.com/item?id=%d"
	// hackernewsThreads is how many of the newest hiring threads a crawl reads.
	hackernewsThreads = 2
)

var (
	hackernewsHiringTitle = regexp.MustCompile(`(?i)who is hiring`)
	// hackernewsParagraph splits a comment's HTML into paragraphs. HN emits an unclosed <p>
	// between paragraphs and nothing before the first one.
	hackernewsParagraph = regexp.MustCompile(`(?i)<p\b[^>]*>`)
	hackernewsURL       = regexp.MustCompile(`https?://\S+`)
	// hackernewsCommitment is a header segment that names a commitment rather than a place.
	hackernewsCommitment = regexp.MustCompile(`(?i)^(full[ -]?time|part[ -]?time|contract(or)?|intern(ship)?s?|permanent|freelance)$`)
)

// hackernewsSearch is the story search response; only the id and title are read.
type hackernewsSearch struct {
	Hits []struct {
		ObjectID string `json:"objectID"`
		Title    string `json:"title"`
	} `json:"hits"`
}

// hackernewsItem is a story or a comment as the items API serves it. Text is null on a deleted
// comment, and the tree recurses through Children.
type hackernewsItem struct {
	ID        int64            `json:"id"`
	Title     string           `json:"title"`
	Text      string           `json:"text"`
	CreatedAt string           `json:"created_at"`
	Children  []hackernewsItem `json:"children"`
}

// threads finds the newest hiring threads' ids, newest first.
func (s hackernews) threads(ctx context.Context) ([]int64, error) {
	var resp hackernewsSearch
	if err := s.http.GetJSON(ctx, hackernewsThreadSearchURL, &resp); err != nil {
		return nil, fmt.Errorf("hackernews: thread search: %w", err)
	}
	var ids []int64
	for _, h := range resp.Hits {
		if !hackernewsHiringTitle.MatchString(h.Title) {
			continue
		}
		id, err := strconv.ParseInt(h.ObjectID, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		ids = append(ids, id)
		if len(ids) == hackernewsThreads {
			break
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("hackernews: no \"Who is hiring?\" thread among the newest %d whoishiring stories", len(resp.Hits))
	}
	return ids, nil
}

func (s hackernews) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	ids, err := s.threads(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, id := range ids {
		var thread hackernewsItem
		if err := s.http.GetJSON(ctx, fmt.Sprintf(hackernewsItemURL, id), &thread); err != nil {
			// fullCatalog: a thread that could not be read must fail the crawl, never shrink it.
			return nil, fmt.Errorf("hackernews: thread %d: %w", id, err)
		}
		for _, c := range thread.Children {
			if job, ok := c.toJob(); ok {
				jobs = append(jobs, job)
			}
		}
	}
	return jobs, nil
}

// hackernewsHeader is a post's parsed first line.
type hackernewsHeader struct {
	Company, Title, Location string
	Remote                   bool
}

// hackernewsParseHeader reads the pipe-delimited first paragraph of a post. ok is false for a
// post with no second segment: nothing then separates the employer from the role.
func hackernewsParseHeader(text string) (hackernewsHeader, bool) {
	head := text
	if loc := hackernewsParagraph.FindStringIndex(text); loc != nil {
		head = text[:loc[0]]
	}
	plain := strings.TrimSpace(hackernewsText(head))
	parts := strings.Split(plain, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) < 2 || parts[0] == "" {
		return hackernewsHeader{}, false
	}
	title := strings.TrimSpace(hackernewsURL.ReplaceAllString(parts[1], ""))
	if title == "" {
		return hackernewsHeader{}, false
	}
	h := hackernewsHeader{
		Company: parts[0],
		Title:   title,
		Remote:  isRemote(strings.Join(parts[1:], " | ")),
	}
	for _, seg := range parts[2:] {
		seg = strings.TrimSpace(hackernewsURL.ReplaceAllString(seg, ""))
		if seg == "" || hackernewsCommitment.MatchString(seg) || strings.ContainsAny(seg[:1], "$€£") {
			continue
		}
		h.Location = seg
		break
	}
	return h, true
}

// hackernewsText renders an HTML fragment as plain text, entities decoded.
func hackernewsText(fragment string) string {
	root, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return ""
	}
	return textContent(root)
}

// hackernewsLink is the first absolute http(s) anchor in a post, or "".
func hackernewsLink(fragment string) string {
	root, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return ""
	}
	link := ""
	walk(root, func(n *html.Node) bool {
		if link != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			if href := strings.TrimSpace(attr(n, "href")); strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
				link = href
				return false
			}
		}
		return true
	})
	return link
}

// toJob maps one top-level comment to a Job. ok is false for a deleted or empty comment and
// for one whose header names no employer and role.
func (c hackernewsItem) toJob() (Job, bool) {
	text := strings.TrimSpace(c.Text)
	if c.ID == 0 || text == "" {
		return Job{}, false
	}
	h, ok := hackernewsParseHeader(text)
	if !ok {
		return Job{}, false
	}
	link := hackernewsLink(text)
	if link == "" {
		link = fmt.Sprintf(hackernewsPermalink, c.ID)
	}
	workMode := ""
	if h.Remote {
		workMode = "remote"
	}
	return Job{
		ExternalID:  strconv.FormatInt(c.ID, 10),
		URL:         link,
		Title:       h.Title,
		Company:     h.Company,
		Location:    h.Location,
		Description: sanitizeHTML(text),
		Remote:      h.Remote,
		WorkMode:    workMode,
		PostedAt:    NotFuture(parseRFC3339(c.CreatedAt)),
	}, true
}
