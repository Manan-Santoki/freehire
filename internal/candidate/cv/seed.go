package cv

import "github.com/strelov1/freehire/internal/candidate/resumeextract"

// Seed builds a Document from the user's extracted résumé structure so a new CV starts
// pre-filled instead of blank. It is a pure field mapping: the caller is responsible for
// Sanitize before persisting (the source Structured is already sanitized on extraction).
//
// Extracted skills seed a single "Skills" group; per-language levels are not extracted,
// so language levels are left for the user to fill in.
//
// Every Bullets list is capped at MaxBullets here — not left for the caller's Sanitize
// (which keeps the FIRST N) or for the seed's own source to bound. This is the one place
// a Structured (built from the experience bank, which only ever accumulates and has no
// per-employment ceiling, or from the résumé's own extraction, capped independently and
// smaller by coincidence today) becomes a Document, and it is the Document's write path
// (cvedit's CommitDocument) that refuses to persist anything over the cap — so capping
// anywhere upstream of here leaves this the one input MaxBullets can still be raised or
// lowered without a second place to keep in sync. Kept oldest-first at every OTHER
// reader of the bank (WorkHistory/Professional — fit-analysis scoring, /me/profile —
// deliberately never cap: see internal/candidate/experience/professional.go), so this
// keeps the LAST N (mostRecent) rather than the first: the bank has no confidence signal
// to rank on, and the newest entries are what a candidate most recently confirmed.
func Seed(s resumeextract.Structured) Document {
	// The tagline under the name is the CV's summary. Prefer the extracted summary; fall
	// back to the headline line when the résumé stated no separate summary.
	summary := s.Summary
	if summary == "" {
		summary = s.Headline
	}
	doc := Document{
		Header: Header{
			FullName: s.FullName,
			Email:    s.Email,
			Phone:    s.Phone,
			Location: s.Location,
			Links:    s.Links,
		},
		Summary: summary,
	}

	for _, e := range s.Experience {
		exp := ExperienceItem{
			Role:     e.Title,
			Company:  e.Company,
			Location: e.Location,
			Start:    e.Start,
			End:      e.End,
			Current:  e.Current,
			Summary:  e.Summary,
			Bullets:  mostRecent(e.Highlights, MaxBullets),
			Stack:    e.Stack,
		}
		doc.Experience = append(doc.Experience, exp)
	}

	for _, ed := range s.Education {
		doc.Education = append(doc.Education, EducationItem{
			Institution: ed.Institution,
			Degree:      ed.Degree,
			End:         ed.Year,
		})
	}

	for _, lang := range s.Languages {
		doc.Languages = append(doc.Languages, Language{Name: lang})
	}

	// The extracted skills seed a single unnamed group (the "SKILLS" section heading is
	// enough — a "Skills:" group label under it would be redundant); the user can split
	// them into named groups in the editor. Empty when the CV stated none.
	if len(s.Skills) > 0 {
		doc.Skills = []SkillGroup{{Items: s.Skills}}
	}

	for _, p := range s.Projects {
		doc.Projects = append(doc.Projects, Project{
			Name:    p.Name,
			Link:    p.Link,
			Bullets: mostRecent(p.Highlights, MaxBullets),
		})
	}

	for _, name := range s.Certifications {
		doc.Certifications = append(doc.Certifications, Certification{Name: name})
	}

	return doc
}
