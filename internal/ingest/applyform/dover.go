package applyform

// DoverApplicationQuestion is one control from Dover's `application_questions[]`, the same
// array a posting's own detail endpoint (`inbound/application-portal-job/{id}`) carries —
// there is no separate form endpoint to capture from.
type DoverApplicationQuestion struct {
	ID                    string   `json:"id"`
	Question              string   `json:"question"`
	InputType             string   `json:"input_type"`
	Required              bool     `json:"required"`
	MultipleChoiceOptions []string `json:"multiple_choice_options"`
	Hidden                bool     `json:"hidden"`
}

// doverInputTypes maps Dover's input_type enum onto the normalized vocabulary. The set was
// measured against a live posting; anything outside it deliberately has no entry.
var doverInputTypes = map[string]FieldType{
	"SHORT_ANSWER":    TypeText,
	"LONG_ANSWER":     TypeTextarea,
	"MULTIPLE_CHOICE": TypeSelect,
	"FILE_UPLOAD":     TypeFile,
}

// FromDover captures the application form described by a Dover posting's own
// application_questions[]. A hidden question is excluded, the same rule FromAshby applies to
// a hidden section: a control the candidate never sees is not part of what applying costs.
func FromDover(questions []DoverApplicationQuestion) Form {
	form := Form{Provider: "dover"}
	for _, q := range questions {
		if q.Hidden {
			continue
		}
		f := Field{
			ID:       q.ID,
			Label:    q.Question,
			Type:     doverInputTypes[q.InputType],
			RawType:  q.InputType,
			Required: q.Required,
		}
		for _, opt := range q.MultipleChoiceOptions {
			f.Options = append(f.Options, Option{Label: opt, Value: opt})
		}
		form.Fields = append(form.Fields, f)
	}
	return form
}
