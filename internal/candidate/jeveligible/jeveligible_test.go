package jeveligible

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/db"
)

func techJob() db.Job {
	return db.Job{
		Category: "ml_ai", Seniority: "intern", Description: "AI intern role",
		IsTech: pgtype.Bool{Bool: true, Valid: true},
		Company: "Acme", CompanySlug: "acme", Source: "greenhouse",
	}
}

func prof() userprofile.Profile {
	return userprofile.Profile{
		Specializations: []string{"ml_ai", "swe"},
		Seniorities:     []string{"intern", "junior"},
	}
}

func TestEligibleHappyPath(t *testing.T) {
	if !Eligible(techJob(), Candidate{Profile: prof()}) {
		t.Error("expected eligible")
	}
}

func TestIneligibleWrongCategory(t *testing.T) {
	j := techJob()
	j.Category = "sales_role_not_in_specs"
	if Eligible(j, Candidate{Profile: prof()}) {
		t.Error("category not in specializations -> ineligible")
	}
}

func TestIneligibleWrongSeniority(t *testing.T) {
	j := techJob()
	j.Seniority = "principal"
	if Eligible(j, Candidate{Profile: prof()}) {
		t.Error("seniority not in profile levels -> ineligible")
	}
}

func TestEmptyProfileSeniorityAllowsAny(t *testing.T) {
	p := prof()
	p.Seniorities = nil
	j := techJob()
	j.Seniority = "principal"
	if !Eligible(j, Candidate{Profile: p}) {
		t.Error("empty profile seniority = no seniority constraint")
	}
}

func TestExcludedCompany(t *testing.T) {
	p := prof()
	p.ExcludedCompanies = []string{"acme"}
	if Eligible(techJob(), Candidate{Profile: p}) {
		t.Error("excluded company -> ineligible")
	}
}
