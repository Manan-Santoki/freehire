// Package submission is the public job-submission queue: any authenticated user can
// submit a vacancy for review, and a moderator approves it into the live catalogue or
// rejects it. It owns validation (shared with internal/ingest/moderation) and the review state
// machine; the Repository owns persistence. Approval mints the live job by delegating to
// the moderation use case (the Minter), so derivation, dedup, and the enrichment enqueue
// are not duplicated here.
package submission

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrSubmissionNotFound is the missing review target (mapped to 404).
	ErrSubmissionNotFound = errors.New("submission: not found")
	// ErrDuplicatePending is a second submission of a URL already awaiting review
	// (the partial unique index; mapped to 409).
	ErrDuplicatePending = errors.New("submission: a pending submission for this URL already exists")
	// ErrAlreadyDecided is an approve/reject of a submission that is no longer pending
	// (mapped to 409).
	ErrAlreadyDecided = errors.New("submission: already decided")
	// ErrBlockedDomain is a submission whose URL host is on the blocklist (mapped to 403).
	// No row is written — see Service.Submit.
	ErrBlockedDomain = errors.New("submission: this domain is not accepted")
)

// Submission is a stored queue entry: the package domain type, decoupled from the generated
// db row. SubmittedBy is kept (unlike the report queue's dropped ownership columns) because
// Approve mints the live job attributed to the original submitter; it is never on the wire.
// created_at/reviewed_at/posted_at are *time.Time because the handler serializes them.
type Submission struct {
	ID           int64
	SubmittedBy  int64
	URL          string
	Source       string
	Title        string
	Company      string
	Location     string
	Remote       bool
	Description  string
	PostedAt     *time.Time
	Status       string
	ReviewReason string
	ReviewedAt   *time.Time
	// JobID is the minted job's id once the submission is approved (nil otherwise, and
	// still nil for the brief window between ClaimForApproval and AttachJob — see
	// Service.Approve's resume branch).
	JobID     *int64
	CreatedAt *time.Time

	// The structured facets the submitter stated, retained so the moderator sees them
	// and Approve can carry them onto the minted job (see the Approve mint below).
	Skills         []string
	Regions        []string
	Cities         []string
	WorkMode       string
	EmploymentType string
	Seniority      string
	SalaryMin      *int
	SalaryMax      *int
	SalaryCurrency string
	SalaryPeriod   string
}

// PendingSubmission is a moderator-queue row: a Submission plus the joined submitter email
// the reviewer needs.
type PendingSubmission struct {
	Submission
	SubmitterEmail string
}

// UserSubmission is a "my submissions" row: a Submission plus the minted job's public slug,
// set only on an approved submission (empty otherwise) so the UI can link to /jobs/<slug>.
type UserSubmission struct {
	Submission
	JobSlug string
}

// Minter mints a live vacancy from validated content. internal/ingest/moderation.Service
// satisfies it; the seam keeps the service testable without a database.
type Minter interface {
	Create(ctx context.Context, actorID int64, in moderation.CreateInput) (job.Job, job.Extras, error)
}

// Repository is the persistence contract for the submission queue, expressed in the package
// domain types rather than the generated db rows. Create takes the validated content plus the
// owning user; the mark methods take primitive ids; the adapter builds the db params and maps
// the rows back.
type Repository interface {
	Create(ctx context.Context, submittedBy int64, in moderation.CreateInput) (Submission, error)
	Get(ctx context.Context, id int64) (Submission, error)
	ListPending(ctx context.Context) ([]PendingSubmission, error)
	ListByUser(ctx context.Context, userID int64) ([]UserSubmission, error)
	// ClaimForApproval atomically flips a pending submission to approved, recording the
	// reviewing moderator; job_id stays unset until AttachJob records the mint. Scoped to
	// status='pending', so this is the guarded transition a concurrent Reject always loses
	// (see Service.Approve).
	ClaimForApproval(ctx context.Context, id, reviewerID int64) (Submission, error)
	// AttachJob records the minted job on a submission ClaimForApproval already claimed.
	// Scoped to status='approved'.
	AttachJob(ctx context.Context, id, jobID int64) (Submission, error)
	MarkRejected(ctx context.Context, id, reviewerID int64, reason string) (Submission, error)
	// IsHostBlocked reports whether a normalized host (see normalizeHost) is on the
	// submission-domain blocklist.
	IsHostBlocked(ctx context.Context, host string) (bool, error)
	// RejectAndBlockHost adds host to the blocklist (attributed to reviewerID/reason) and,
	// in the same action, rejects id plus every other still-pending submission whose URL
	// host normalizes to host. Returns the target (id)'s resulting row; a target no longer
	// pending by the time this runs is ErrAlreadyDecided, matching MarkRejected.
	RejectAndBlockHost(ctx context.Context, id int64, host string, reviewerID int64, reason string) (Submission, error)
}

// Service implements the submission use cases.
type Service struct {
	repo   Repository
	minter Minter
}

// New creates a Service backed by the given Repository and Minter.
func New(repo Repository, minter Minter) *Service {
	return &Service{repo: repo, minter: minter}
}

// Submit validates contributed content against the same contract a moderator create uses,
// refuses a URL whose host is on the submission-domain blocklist (ErrBlockedDomain, no row
// written), and otherwise stores it as a pending submission owned by the given user. A
// second submission of a URL already pending surfaces ErrDuplicatePending (the repository
// maps the unique violation).
//
// The description is sanitized to the same allowlist moderation.Service.Create uses, before
// it is ever persisted — a pending or rejected submission is never re-sanitized, and the
// review UI already renders it with {@html}, so waiting for approval would leave raw HTML in
// the database (stored XSS). Sanitizing here rather than re-sanitizing is also idempotent:
// Approve carries this already-clean value into moderation.CreateInput, and
// htmltext.Sanitize is safe to run twice.
func (s *Service) Submit(ctx context.Context, submittedBy int64, in moderation.CreateInput) (Submission, error) {
	if err := in.Validate(); err != nil {
		return Submission{}, err
	}
	blocked, err := s.repo.IsHostBlocked(ctx, hostOf(in.URL))
	if err != nil {
		return Submission{}, err
	}
	if blocked {
		return Submission{}, ErrBlockedDomain
	}
	in.Description = htmltext.Sanitize(in.Description)
	return s.repo.Create(ctx, submittedBy, in)
}

// ListMine returns the given user's submissions, newest first. Each row carries the
// minted job's slug (when approved) so the UI can link to the live vacancy.
func (s *Service) ListMine(ctx context.Context, userID int64) ([]UserSubmission, error) {
	return s.repo.ListByUser(ctx, userID)
}

// ListPending returns the moderator review queue (with submitter emails), newest first.
func (s *Service) ListPending(ctx context.Context) ([]PendingSubmission, error) {
	return s.repo.ListPending(ctx)
}

// Approve claims a pending submission, mints a live vacancy from its fields (attributed to
// the submitter), and attaches the minted job to the now-approved submission. A missing
// submission is ErrSubmissionNotFound; one that is no longer pending (and not a resumable
// claim — see below) is ErrAlreadyDecided.
//
// The claim runs BEFORE the mint, not after: it atomically flips the status to 'approved'
// under the same status='pending' guard Reject's own mark uses, so whichever call reaches
// its guarded update first wins the row and the other gets ErrAlreadyDecided on its own
// side. That closes the race the previous mint-then-mark order left open, where a
// concurrent Reject could flip the status between the mint and the mark: the job would
// exist live while the submission stayed 'rejected' with no job_id pointing at it.
//
// A submission already claimed but not yet attached (status='approved', JobID nil) is a
// resumed attempt — the process died between the claim and the attach on an earlier call —
// and is minted and attached without claiming again, since the claim is not repeatable
// (its own status='pending' guard would no longer match). Because the moderation upsert is
// idempotent on the URL, re-minting on resume is safe.
func (s *Service) Approve(ctx context.Context, reviewerID, id int64) (Submission, error) {
	sub, err := s.repo.Get(ctx, id)
	if err != nil {
		return Submission{}, err
	}
	switch {
	case sub.Status == statusPending:
		sub, err = s.repo.ClaimForApproval(ctx, id, reviewerID)
		if err != nil {
			return Submission{}, err
		}
	case sub.Status == statusApproved && sub.JobID == nil:
		// Resume: already claimed by an earlier, interrupted call. Mint and attach below
		// without claiming again.
	default:
		return Submission{}, ErrAlreadyDecided
	}
	mintedJob, _, err := s.minter.Create(ctx, sub.SubmittedBy, moderation.CreateInput{
		URL:            sub.URL,
		Source:         sub.Source,
		Title:          sub.Title,
		Company:        sub.Company,
		Location:       sub.Location,
		Remote:         sub.Remote,
		Description:    sub.Description,
		PostedAt:       sub.PostedAt,
		Skills:         sub.Skills,
		Regions:        sub.Regions,
		Cities:         sub.Cities,
		WorkMode:       sub.WorkMode,
		EmploymentType: sub.EmploymentType,
		Seniority:      sub.Seniority,
		SalaryMin:      sub.SalaryMin,
		SalaryMax:      sub.SalaryMax,
		SalaryCurrency: sub.SalaryCurrency,
		SalaryPeriod:   sub.SalaryPeriod,
	})
	if err != nil {
		return Submission{}, err
	}
	return s.repo.AttachJob(ctx, id, mintedJob.Fields().ID)
}

// Reject marks a pending submission rejected with an optional reason, recording the
// reviewing moderator. No job is created. A missing submission is ErrSubmissionNotFound;
// one that is no longer pending is ErrAlreadyDecided.
//
// When blockDomain is set, the submission's URL host is added to the submission-domain
// blocklist and every other still-pending submission on that host is rejected in the same
// action (see Repository.RejectAndBlockHost) — a moderator clearing one spam submission
// clears the whole domain's backlog and refuses it going forward, in one call.
func (s *Service) Reject(ctx context.Context, reviewerID, id int64, reason string, blockDomain bool) (Submission, error) {
	sub, err := s.repo.Get(ctx, id)
	if err != nil {
		return Submission{}, err
	}
	if sub.Status != statusPending {
		return Submission{}, ErrAlreadyDecided
	}
	if blockDomain {
		return s.repo.RejectAndBlockHost(ctx, id, hostOf(sub.URL), reviewerID, reason)
	}
	return s.repo.MarkRejected(ctx, id, reviewerID, reason)
}

// statusPending is the only status that can be approved or rejected; statusApproved is the
// claimed-but-maybe-not-yet-attached state Approve resumes from. The closed vocabulary
// lives in the migration's CHECK.
const (
	statusPending  = "pending"
	statusApproved = "approved"
)

// hostOf extracts the normalized host from a URL already known to parse (Validate, or a
// stored submission's own URL, guarantees this). A parse failure — unreachable in practice
// — yields "", which matches no blocklist entry.
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return normalizeHost(u.Host)
}

// normalizeHost lowercases a URL host and strips one leading "www." prefix, so
// gridnaut.site and www.GridNaut.site match the same blocklist entry. Deliberately no
// wildcard or subdomain/suffix matching: exact-host is enough for the observed pattern and
// keeps this a single indexed lookup with no false-positive risk.
func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.TrimPrefix(host, "www.")
}
