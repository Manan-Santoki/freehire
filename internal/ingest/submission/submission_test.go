package submission_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/ingest/submission"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeRepo records the domain inputs it is handed and returns canned rows, so the service
// tests run without a database (the moderation_test.go precedent).
type fakeRepo struct {
	created         moderation.CreateInput
	createSubmitter int64
	createCalled    bool
	createErr       error
	createRet       submission.Submission

	getRet submission.Submission
	getErr error

	claimID       int64
	claimReviewer int64
	claimCalled   bool
	claimErr      error
	claimRet      submission.Submission

	attachID     int64
	attachJobID  int64
	attachCalled bool
	attachErr    error
	attachRet    submission.Submission

	rejectID       int64
	rejectReviewer int64
	rejectReason   string
	rejectCalled   bool
	rejectErr      error
	rejectRet      submission.Submission

	isHostBlockedHost   string
	isHostBlockedCalled bool
	isHostBlockedRet    bool
	isHostBlockedErr    error

	blockID       int64
	blockHost     string
	blockReviewer int64
	blockReason   string
	blockCalled   bool
	blockErr      error
	blockRet      submission.Submission

	// order, when set, records the sequence of side-effecting calls this repo and a
	// fakeMinter sharing the same pointer make ("claim", "mint", "attach"), so a test can
	// assert Approve's call order without guessing at timing.
	order *[]string
}

func (f *fakeRepo) Create(_ context.Context, submittedBy int64, in moderation.CreateInput) (submission.Submission, error) {
	f.created, f.createSubmitter, f.createCalled = in, submittedBy, true
	return f.createRet, f.createErr
}

func (f *fakeRepo) Get(_ context.Context, _ int64) (submission.Submission, error) {
	return f.getRet, f.getErr
}

func (f *fakeRepo) ListPending(_ context.Context) ([]submission.PendingSubmission, error) {
	return nil, nil
}

func (f *fakeRepo) ListByUser(_ context.Context, _ int64) ([]submission.UserSubmission, error) {
	return nil, nil
}

func (f *fakeRepo) ClaimForApproval(_ context.Context, id, reviewerID int64) (submission.Submission, error) {
	f.claimID, f.claimReviewer, f.claimCalled = id, reviewerID, true
	if f.order != nil {
		*f.order = append(*f.order, "claim")
	}
	return f.claimRet, f.claimErr
}

func (f *fakeRepo) AttachJob(_ context.Context, id, jobID int64) (submission.Submission, error) {
	f.attachID, f.attachJobID, f.attachCalled = id, jobID, true
	if f.order != nil {
		*f.order = append(*f.order, "attach")
	}
	return f.attachRet, f.attachErr
}

func (f *fakeRepo) MarkRejected(_ context.Context, id, reviewerID int64, reason string) (submission.Submission, error) {
	f.rejectID, f.rejectReviewer, f.rejectReason, f.rejectCalled = id, reviewerID, reason, true
	return f.rejectRet, f.rejectErr
}

func (f *fakeRepo) IsHostBlocked(_ context.Context, host string) (bool, error) {
	f.isHostBlockedHost, f.isHostBlockedCalled = host, true
	return f.isHostBlockedRet, f.isHostBlockedErr
}

func (f *fakeRepo) RejectAndBlockHost(_ context.Context, id int64, host string, reviewerID int64, reason string) (submission.Submission, error) {
	f.blockID, f.blockHost, f.blockReviewer, f.blockReason, f.blockCalled = id, host, reviewerID, reason, true
	return f.blockRet, f.blockErr
}

// fakeMinter stands in for moderation.Service: it records the approve-time mint call.
type fakeMinter struct {
	actorID int64
	in      moderation.CreateInput
	called  bool
	ret     job.Job
	err     error

	// order, when set, records this call as "mint" — see fakeRepo.order.
	order *[]string
}

func (m *fakeMinter) Create(_ context.Context, actorID int64, in moderation.CreateInput) (job.Job, job.Extras, error) {
	m.actorID, m.in, m.called = actorID, in, true
	if m.order != nil {
		*m.order = append(*m.order, "mint")
	}
	return m.ret, job.Extras{}, m.err
}

// mustJob hydrates a db row into the aggregate for the minted-job fixture.
func mustJob(t *testing.T, r db.Job) job.Job {
	t.Helper()
	j, _, err := job.FromRow(r)
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	return j
}

func validInput() moderation.CreateInput {
	return moderation.CreateInput{
		URL:      "https://acme.example/jobs/1",
		Title:    "Senior Go Developer",
		Company:  "Acme",
		Location: "Berlin",
		Remote:   true,
	}
}

func TestSubmit_PersistsPendingWithOwner(t *testing.T) {
	repo := &fakeRepo{createRet: submission.Submission{ID: 1, Status: "pending"}}
	svc := submission.New(repo, &fakeMinter{})

	_, err := svc.Submit(context.Background(), 7, validInput())
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !repo.createCalled {
		t.Fatal("repo.Create was not called")
	}
	if repo.createSubmitter != 7 {
		t.Errorf("submittedBy = %d, want 7", repo.createSubmitter)
	}
	got := repo.created
	if got.URL != "https://acme.example/jobs/1" || got.Title != "Senior Go Developer" || got.Company != "Acme" {
		t.Errorf("content not carried through: %+v", got)
	}
	if got.Location != "Berlin" || !got.Remote {
		t.Errorf("optional fields not carried: location=%q remote=%v", got.Location, got.Remote)
	}
}

func TestSubmit_SanitizesDescriptionBeforePersist(t *testing.T) {
	repo := &fakeRepo{createRet: submission.Submission{ID: 1, Status: "pending"}}
	in := validInput()
	in.Description = "Build it<script>alert(1)</script>"

	_, err := submission.New(repo, &fakeMinter{}).Submit(context.Background(), 7, in)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !repo.createCalled {
		t.Fatal("repo.Create was not called")
	}
	if strings.Contains(repo.created.Description, "<script") {
		t.Errorf("repo.Create received unsanitized description: %q", repo.created.Description)
	}
	if repo.created.Description != "Build it" {
		t.Errorf("description = %q, want sanitized %q", repo.created.Description, "Build it")
	}
}

func TestSubmit_ValidatesBeforePersist(t *testing.T) {
	cases := []struct {
		name string
		in   moderation.CreateInput
	}{
		{"missing url", moderation.CreateInput{Title: "T", Company: "C"}},
		{"missing title", moderation.CreateInput{URL: "https://x/1", Company: "C"}},
		{"missing company", moderation.CreateInput{URL: "https://x/1", Title: "T"}},
		{"non-http url", moderation.CreateInput{URL: "ftp://x/1", Title: "T", Company: "C"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, err := submission.New(repo, &fakeMinter{}).Submit(context.Background(), 7, tc.in)
			if !errors.Is(err, moderation.ErrInvalid) {
				t.Errorf("err = %v, want moderation.ErrInvalid", err)
			}
			if repo.createCalled {
				t.Error("repo.Create should not be called on invalid input")
			}
		})
	}
}

func TestSubmit_PropagatesDuplicatePending(t *testing.T) {
	repo := &fakeRepo{createErr: submission.ErrDuplicatePending}
	_, err := submission.New(repo, &fakeMinter{}).Submit(context.Background(), 7, validInput())
	if !errors.Is(err, submission.ErrDuplicatePending) {
		t.Errorf("err = %v, want ErrDuplicatePending", err)
	}
}

func TestApprove_ClaimsThenMintsThenAttaches(t *testing.T) {
	sub := submission.Submission{ID: 5, SubmittedBy: 7, Status: "pending", URL: "https://x/1", Source: "workatastartup", Title: "Dev", Company: "Acme", Location: "Berlin", Remote: true, Description: "Build <b>it</b>"}
	claimed := sub
	claimed.Status = "approved"
	var order []string
	repo := &fakeRepo{getRet: sub, claimRet: claimed, attachRet: submission.Submission{ID: 5, Status: "approved"}, order: &order}
	minter := &fakeMinter{ret: mustJob(t, db.Job{ID: 99}), order: &order}
	svc := submission.New(repo, minter)

	_, err := svc.Approve(context.Background(), 3, 5)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if !repo.claimCalled {
		t.Fatal("repo.ClaimForApproval was not called")
	}
	if repo.claimID != 5 || repo.claimReviewer != 3 {
		t.Errorf("claim params = id=%d reviewer=%d, want id=5 reviewer=3", repo.claimID, repo.claimReviewer)
	}
	if !minter.called {
		t.Fatal("minter.Create was not called")
	}
	if minter.actorID != 7 {
		t.Errorf("mint actorID = %d, want 7 (the submitter, as job author)", minter.actorID)
	}
	// Every content field must travel from the (claimed) submission to the mint input —
	// especially Description, the one field the minter sanitizes, and Source, which the
	// minter defaults.
	if minter.in.URL != "https://x/1" || minter.in.Source != "workatastartup" ||
		minter.in.Company != "Acme" || !minter.in.Remote || minter.in.Description != "Build <b>it</b>" {
		t.Errorf("mint input not built from submission: %+v", minter.in)
	}
	if !repo.attachCalled {
		t.Fatal("repo.AttachJob was not called")
	}
	if repo.attachID != 5 || repo.attachJobID != 99 {
		t.Errorf("attach params = id=%d job=%d, want id=5 job=99", repo.attachID, repo.attachJobID)
	}
	// The claim must precede the mint — that ordering is the whole fix, closing the race
	// where a concurrent Reject could flip the status between the mint and the mark — and
	// the mint must precede the attach, since there is no job to attach before it exists.
	want := []string{"claim", "mint", "attach"}
	if !slices.Equal(order, want) {
		t.Errorf("call order = %v, want %v", order, want)
	}
}

func TestApprove_NotFound(t *testing.T) {
	repo := &fakeRepo{getErr: submission.ErrSubmissionNotFound}
	minter := &fakeMinter{}
	_, err := submission.New(repo, minter).Approve(context.Background(), 3, 5)
	if !errors.Is(err, submission.ErrSubmissionNotFound) {
		t.Errorf("err = %v, want ErrSubmissionNotFound", err)
	}
	if minter.called {
		t.Error("minter.Create should not be called when the submission is missing")
	}
}

func TestApprove_AlreadyDecided(t *testing.T) {
	jobID := int64(9)
	repo := &fakeRepo{getRet: submission.Submission{ID: 5, Status: "approved", JobID: &jobID}}
	minter := &fakeMinter{}
	_, err := submission.New(repo, minter).Approve(context.Background(), 3, 5)
	if !errors.Is(err, submission.ErrAlreadyDecided) {
		t.Errorf("err = %v, want ErrAlreadyDecided", err)
	}
	if minter.called || repo.claimCalled || repo.attachCalled {
		t.Error("a fully decided submission must not be claimed, minted, or attached")
	}
}

// TestApprove_ClaimLosesRace_PropagatesErrAlreadyDecided covers the race the whole
// claim-first design exists to close: Get sees 'pending' (a snapshot that is already
// stale by the time this runs), but the atomic ClaimForApproval loses to a concurrent
// decision — exactly what a real Reject's own status='pending'-guarded update would cause,
// by flipping the row to 'rejected' between this call's Get and its claim. The claim must
// never be allowed to appear to succeed once it has actually lost.
func TestApprove_ClaimLosesRace_PropagatesErrAlreadyDecided(t *testing.T) {
	repo := &fakeRepo{getRet: submission.Submission{ID: 5, Status: "pending"}, claimErr: submission.ErrAlreadyDecided}
	minter := &fakeMinter{}
	_, err := submission.New(repo, minter).Approve(context.Background(), 3, 5)
	if !errors.Is(err, submission.ErrAlreadyDecided) {
		t.Errorf("err = %v, want ErrAlreadyDecided", err)
	}
	if !repo.claimCalled {
		t.Error("repo.ClaimForApproval should have been attempted")
	}
	if minter.called || repo.attachCalled {
		t.Error("a lost claim must never mint or attach a job")
	}
}

// TestApprove_ResumesClaimedButUnattachedSubmission covers a process that died between the
// claim and the attach on an earlier call: Get now sees status='approved' with no job_id
// yet. Approve must mint and attach without claiming again — the claim's own
// status='pending' guard would no longer match a row this call's own earlier attempt
// already moved to 'approved'.
func TestApprove_ResumesClaimedButUnattachedSubmission(t *testing.T) {
	sub := submission.Submission{ID: 5, SubmittedBy: 7, Status: "approved", JobID: nil, URL: "https://x/1", Source: "workatastartup", Title: "Dev", Company: "Acme"}
	repo := &fakeRepo{getRet: sub, attachRet: submission.Submission{ID: 5, Status: "approved"}}
	minter := &fakeMinter{ret: mustJob(t, db.Job{ID: 99})}
	svc := submission.New(repo, minter)

	_, err := svc.Approve(context.Background(), 3, 5)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if repo.claimCalled {
		t.Error("a resumed approval must not call ClaimForApproval again")
	}
	if !minter.called {
		t.Fatal("minter.Create was not called")
	}
	if minter.actorID != 7 {
		t.Errorf("mint actorID = %d, want 7 (the submitter, as job author)", minter.actorID)
	}
	if !repo.attachCalled {
		t.Fatal("repo.AttachJob was not called")
	}
	if repo.attachID != 5 || repo.attachJobID != 99 {
		t.Errorf("attach params = id=%d job=%d, want id=5 job=99", repo.attachID, repo.attachJobID)
	}
}

func TestReject_MarksWithReason(t *testing.T) {
	repo := &fakeRepo{getRet: submission.Submission{ID: 5, Status: "pending"}, rejectRet: submission.Submission{Status: "rejected"}}
	minter := &fakeMinter{}
	_, err := submission.New(repo, minter).Reject(context.Background(), 3, 5, "duplicate", false)
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if !repo.rejectCalled {
		t.Fatal("repo.MarkRejected was not called")
	}
	if repo.rejectID != 5 || repo.rejectReviewer != 3 || repo.rejectReason != "duplicate" {
		t.Errorf("reject params = id=%d reviewer=%d reason=%q, want id=5 reviewer=3 reason=duplicate", repo.rejectID, repo.rejectReviewer, repo.rejectReason)
	}
	if minter.called {
		t.Error("reject must not mint a job")
	}
	if repo.blockCalled {
		t.Error("a plain reject must not touch the blocklist")
	}
}

func TestReject_AlreadyDecided(t *testing.T) {
	repo := &fakeRepo{getRet: submission.Submission{ID: 5, Status: "rejected"}}
	_, err := submission.New(repo, &fakeMinter{}).Reject(context.Background(), 3, 5, "", false)
	if !errors.Is(err, submission.ErrAlreadyDecided) {
		t.Errorf("err = %v, want ErrAlreadyDecided", err)
	}
	if repo.rejectCalled {
		t.Error("a decided submission must not be re-marked")
	}
}

func TestReject_AlreadyDecided_BlockDomainNotAttempted(t *testing.T) {
	repo := &fakeRepo{getRet: submission.Submission{ID: 5, Status: "approved"}}
	_, err := submission.New(repo, &fakeMinter{}).Reject(context.Background(), 3, 5, "", true)
	if !errors.Is(err, submission.ErrAlreadyDecided) {
		t.Errorf("err = %v, want ErrAlreadyDecided", err)
	}
	if repo.blockCalled {
		t.Error("a decided submission must not trigger a blocklist write")
	}
}

func TestReject_BlockDomain_UsesNormalizedHostOfSubmissionURL(t *testing.T) {
	repo := &fakeRepo{
		getRet:   submission.Submission{ID: 5, Status: "pending", URL: "https://WWW.Gridnaut.Site/jobs/1"},
		blockRet: submission.Submission{ID: 5, Status: "rejected"},
	}
	_, err := submission.New(repo, &fakeMinter{}).Reject(context.Background(), 3, 5, "referral spam", true)
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if !repo.blockCalled {
		t.Fatal("repo.RejectAndBlockHost was not called")
	}
	if repo.blockID != 5 || repo.blockReviewer != 3 || repo.blockReason != "referral spam" {
		t.Errorf("block params = id=%d reviewer=%d reason=%q, want id=5 reviewer=3 reason=%q", repo.blockID, repo.blockReviewer, repo.blockReason, "referral spam")
	}
	if repo.blockHost != "gridnaut.site" {
		t.Errorf("blockHost = %q, want normalized host %q", repo.blockHost, "gridnaut.site")
	}
	if repo.rejectCalled {
		t.Error("block_domain must use RejectAndBlockHost, not the plain MarkRejected path")
	}
}

func TestSubmit_RefusesBlockedHost(t *testing.T) {
	repo := &fakeRepo{isHostBlockedRet: true}
	_, err := submission.New(repo, &fakeMinter{}).Submit(context.Background(), 7, validInput())
	if !errors.Is(err, submission.ErrBlockedDomain) {
		t.Errorf("err = %v, want ErrBlockedDomain", err)
	}
	if repo.createCalled {
		t.Error("a blocked host must not be persisted")
	}
}

func TestSubmit_ChecksNormalizedHost(t *testing.T) {
	repo := &fakeRepo{}
	in := validInput()
	in.URL = "https://WWW.Acme.Example/jobs/1"
	if _, err := submission.New(repo, &fakeMinter{}).Submit(context.Background(), 7, in); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if repo.isHostBlockedHost != "acme.example" {
		t.Errorf("isHostBlockedHost = %q, want normalized host %q", repo.isHostBlockedHost, "acme.example")
	}
}

func TestSubmit_AllowsNonBlockedHost(t *testing.T) {
	repo := &fakeRepo{isHostBlockedRet: false, createRet: submission.Submission{ID: 1, Status: "pending"}}
	_, err := submission.New(repo, &fakeMinter{}).Submit(context.Background(), 7, validInput())
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !repo.createCalled {
		t.Error("a non-blocked host must be persisted as usual")
	}
}
