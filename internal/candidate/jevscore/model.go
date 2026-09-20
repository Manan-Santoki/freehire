// Package jevscore is the Jev-backed job-fit decision model: the typed questions, the
// state builder, and the sanitized Score with its verdict. Ported from the validated
// jd_match.py, with fits_level derived per profile.
package jevscore

import (
	"encoding/json"
	"strings"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/jev"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// maxStructuredRunes bounds the résumé JSON in the state, matching matchanalysis.
const maxStructuredRunes = 3000

// matchRubric is the 5-level score rubric; raw score maps raw/(len-1) -> 0..100.
var matchRubric = []string{
	"No meaningful overlap; wrong field or role type",
	"Weak match; a few transferable skills but missing most core requirements",
	"Moderate match; meets several core requirements, notable gaps remain",
	"Strong match; meets most core requirements with only minor gaps",
	"Excellent match; meets or exceeds nearly all requirements",
}

// RoleCategories is the closed choice vocabulary; also the coercion target set.
var RoleCategories = []string{"swe", "ml_ai", "robotics", "qa_test", "data", "devops_infra", "other_tech", "non_technical"}

var roleCriteria = map[string]string{
	"swe":           "General, full-stack, backend, or frontend software engineering",
	"ml_ai":         "Machine learning, AI, LLM, or data science engineering",
	"robotics":      "Robotics, embedded, or controls",
	"qa_test":       "QA, SDET, test automation, or beta testing",
	"data":          "Data engineering, analytics, or data platform",
	"devops_infra":  "DevOps, SRE, platform, or cloud infrastructure",
	"other_tech":    "A technical role not covered by the other options",
	"non_technical": "A non-technical role such as sales, marketing, or management",
}

// Questions builds the five typed questions. fits_level's instruction is derived from
// the profile's seniority levels so the model judges level fit per candidate.
func Questions(seniorities []string) map[string]jev.Question {
	return map[string]jev.Question{
		"match_score":        {Type: jev.Score, Instructions: "Rate how well the RESUME matches the requirements in the JOB DESCRIPTION.", Criteria: matchRubric},
		"role_category":      {Type: jev.Choice, Instructions: "Classify the type of role described in the JOB DESCRIPTION.", Criteria: roleCriteria},
		"has_required_stack": {Type: jev.Noul, Instructions: "The RESUME demonstrates the core technical stack and skills the JOB DESCRIPTION requires."},
		"fits_level":         {Type: jev.Noul, Instructions: fitsLevelInstruction(seniorities)},
		"hard_blocker":       {Type: jev.Noul, Instructions: "The JOB DESCRIPTION states a hard requirement the candidate likely cannot meet, such as US citizenship, an active security clearance, a specific degree not held, or a specialized license."},
	}
}

func fitsLevelInstruction(seniorities []string) string {
	if len(seniorities) == 0 {
		return "The role's seniority and experience level fit the candidate's stated experience level rather than being clearly over- or under-qualified."
	}
	return "The role's seniority and experience level fit a candidate targeting: " +
		strings.Join(seniorities, ", ") + " — not a level clearly above or below those."
}

// BuildState assembles the Jev state: the structured résumé JSON (contacts already
// removed by the Professional projection) and the job text.
func BuildState(r resumeextract.Professional, jobText string) string {
	blob, err := json.Marshal(r)
	if err != nil {
		blob = []byte("{}")
	}
	resume := llm.TruncateRunes(string(blob), maxStructuredRunes)
	return "RESUME:\n" + resume + "\n\n---\n\nJOB DESCRIPTION:\n" + jobText
}
