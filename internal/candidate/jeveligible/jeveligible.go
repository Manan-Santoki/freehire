// Package jeveligible is the deterministic gate that bounds which (user, job) pairs Jev
// ever scores. It reproduces the profile's feed filter (specialization, seniority,
// exclusions) plus the obvious hard-constraint blockers (work authorization, location /
// work mode), so an ineligible pair never reaches the model.
package jeveligible

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/job/jobfacts"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// Candidate is the profile-side of the gate for one user.
type Candidate struct {
	Profile          userprofile.Profile
	Resume           resumeextract.Professional
	Loc              userprofile.LocationPreferences
	DerivedCountries []string
}

// Eligible reports whether job should be Jev-scored for this candidate.
func Eligible(job db.Job, c Candidate) bool {
	if job.Category == "" || !slices.Contains(c.Profile.Specializations, job.Category) {
		return false
	}
	if len(c.Profile.Seniorities) > 0 && (job.Seniority == "" || !slices.Contains(c.Profile.Seniorities, job.Seniority)) {
		return false
	}
	if slices.Contains(c.Profile.ExcludedCompanies, strings.ToLower(job.CompanySlug)) ||
		slices.Contains(c.Profile.ExcludedCompanies, strings.ToLower(job.Company)) {
		return false
	}
	if slices.Contains(c.Profile.ExcludedSources, strings.ToLower(job.Source)) {
		return false
	}
	// Obvious hard blockers: work authorization and location/work-mode only.
	jr, ev := hardInputs(job, c)
	for _, b := range hardconstraint.Evaluate(jr, ev) {
		if b.Met {
			continue
		}
		if b.Category == hardconstraint.CategoryWorkAuth || b.Category == hardconstraint.CategoryLocationWorkMode {
			return false
		}
	}
	return true
}

// hardInputs mirrors internal/api/handler.buildHardConstraintInputs.
func hardInputs(job db.Job, c Candidate) (hardconstraint.JobRequirements, hardconstraint.CVEvidence) {
	jr := hardconstraint.JobRequirements{
		ExperienceYearsMin:     pgconv.IntPtr(job.ExperienceYearsMin),
		EducationLevel:         job.EducationLevel,
		DegreeOptional:         jobfacts.DegreeOptional(job.Description),
		EnglishLevel:           job.EnglishLevel,
		VisaSponsorship:        jobVisa(job.Enrichment),
		WorkMode:               job.WorkMode,
		Countries:              job.Countries,
		RequiredCertifications: jobfacts.RequiredCertifications(job.Description),
	}
	ev := hardconstraint.CVEvidence{
		TotalYears:     c.Resume.TotalYears,
		Degrees:        degreeNames(c.Resume.Education),
		Languages:      c.Resume.Languages,
		Certifications: c.Resume.Certifications,
		CountryCode:    candidateCountry(c.Loc.Base.Country, c.DerivedCountries),
		PrefersRemote:  prefersRemote(c.Loc.WorkModes),
	}
	return jr, ev
}

func jobVisa(raw json.RawMessage) *bool {
	if len(raw) == 0 {
		return nil
	}
	var e enrich.Enrichment
	if json.Unmarshal(raw, &e) != nil {
		return nil
	}
	return e.VisaSponsorship
}

func degreeNames(edu []resumeextract.Education) []string {
	out := make([]string, 0, len(edu))
	for _, e := range edu {
		if e.Degree != "" {
			out = append(out, e.Degree)
		}
	}
	return out
}

func candidateCountry(asserted string, derived []string) string {
	if asserted != "" {
		return asserted
	}
	if len(derived) == 1 {
		return derived[0]
	}
	return ""
}

func prefersRemote(workModes []string) bool {
	var remote, onsiteOrHybrid bool
	for _, m := range workModes {
		switch strings.ToLower(m) {
		case "remote":
			remote = true
		case "onsite", "hybrid":
			onsiteOrHybrid = true
		}
	}
	return remote && !onsiteOrHybrid
}
