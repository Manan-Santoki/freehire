package jevscore

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/jev"
)

func TestQuestionsLevelDerived(t *testing.T) {
	qs := Questions([]string{"intern", "junior"})
	if len(qs) != 5 {
		t.Fatalf("want 5 questions, got %d", len(qs))
	}
	if qs["match_score"].Type != jev.Score || qs["role_category"].Type != jev.Choice {
		t.Error("wrong question types")
	}
	if !strings.Contains(qs["fits_level"].Instructions, "intern") || !strings.Contains(qs["fits_level"].Instructions, "junior") {
		t.Errorf("fits_level should name the profile levels: %q", qs["fits_level"].Instructions)
	}
	if _, ok := qs["role_category"].Criteria.(map[string]string)["ml_ai"]; !ok {
		t.Error("role_category criteria missing ml_ai")
	}
}

func TestQuestionsNoLevelsDegrades(t *testing.T) {
	if strings.Contains(Questions(nil)["fits_level"].Instructions, "targeting:") {
		t.Error("no levels should use the generic instruction")
	}
}

func TestBuildState(t *testing.T) {
	s := BuildState(resumeextract.Professional{Headline: "SWE"}, "Backend Engineer")
	if !strings.Contains(s, "RESUME:") || !strings.Contains(s, "JOB DESCRIPTION:") || !strings.Contains(s, "Backend Engineer") {
		t.Errorf("state malformed: %q", s)
	}
}
