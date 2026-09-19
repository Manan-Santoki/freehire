package applyform

import (
	"encoding/json"
	"testing"
)

// doverFixture mirrors the application_questions[] captured live from a real Dover posting
// (qompyl/f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5, 2026-09-18).
const doverFixture = `[
  {"id": "ddaa0e05-172c-4ee5-a17b-0cf2d965f70b", "question": "LinkedIn Profile URL",
   "input_type": "SHORT_ANSWER", "required": true, "question_type": "LINKEDIN_URL",
   "multiple_choice_options": null, "max_selections": null, "hidden": false, "order_index": 0},
  {"id": "5ec7ee02-58f0-4643-8e77-1dbf0b9c65f9", "question": "Phone Number",
   "input_type": "SHORT_ANSWER", "required": false, "question_type": "PHONE_NUMBER",
   "multiple_choice_options": null, "max_selections": null, "hidden": false, "order_index": 1},
  {"id": "40423ba7-fbbc-4802-a1e0-7e48722d9c7a",
   "question": "Just to confirm - are you aware this role is equity-only at this stage (no salary yet)?",
   "input_type": "MULTIPLE_CHOICE", "required": true, "question_type": "CUSTOM",
   "multiple_choice_options": ["Yes, I understand", "No, I wasn't aware", "I'd like to know more about how that works"],
   "max_selections": null, "hidden": false, "order_index": 2},
  {"id": "14360a61-b515-4b42-97f4-22a96f353d8b", "question": "What motivates you to join an early-stage, equity-based startup?",
   "input_type": "LONG_ANSWER", "required": false, "question_type": "CUSTOM",
   "multiple_choice_options": null, "max_selections": null, "hidden": false, "order_index": 3},
  {"id": "17b7dbca-05fb-4f3f-8ee2-22d82540ab83", "question": "Resume Upload",
   "input_type": "FILE_UPLOAD", "required": true, "question_type": "RESUME",
   "multiple_choice_options": null, "max_selections": null, "hidden": false, "order_index": null},
  {"id": "hidden-1", "question": "Internal routing tag",
   "input_type": "SHORT_ANSWER", "required": false, "question_type": "CUSTOM",
   "multiple_choice_options": null, "max_selections": null, "hidden": true, "order_index": 5}
]`

func decodeDover(t *testing.T) Form {
	t.Helper()
	var questions []DoverApplicationQuestion
	if err := json.Unmarshal([]byte(doverFixture), &questions); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return FromDover(questions)
}

func TestFromDoverMapsFieldShapes(t *testing.T) {
	form := decodeDover(t)
	if form.Provider != "dover" {
		t.Errorf("Provider = %q, want dover", form.Provider)
	}
	// The hidden question must not appear: it is not part of what applying costs.
	if len(form.Fields) != 5 {
		t.Fatalf("want 5 visible fields, got %d: %+v", len(form.Fields), form.Fields)
	}

	byID := map[string]Field{}
	for _, f := range form.Fields {
		byID[f.ID] = f
	}

	linkedin := byID["ddaa0e05-172c-4ee5-a17b-0cf2d965f70b"]
	if linkedin.Type != TypeText || !linkedin.Required || linkedin.Label != "LinkedIn Profile URL" {
		t.Errorf("linkedin field = %+v", linkedin)
	}

	phone := byID["5ec7ee02-58f0-4643-8e77-1dbf0b9c65f9"]
	if phone.Type != TypeText || phone.Required {
		t.Errorf("phone field = %+v", phone)
	}

	choice := byID["40423ba7-fbbc-4802-a1e0-7e48722d9c7a"]
	if choice.Type != TypeSelect || len(choice.Options) != 3 {
		t.Fatalf("choice field = %+v", choice)
	}
	if choice.Options[0].Label != "Yes, I understand" || choice.Options[0].Value != "Yes, I understand" {
		t.Errorf("choice option[0] = %+v", choice.Options[0])
	}

	long := byID["14360a61-b515-4b42-97f4-22a96f353d8b"]
	if long.Type != TypeTextarea {
		t.Errorf("long-answer field = %+v", long)
	}

	resume := byID["17b7dbca-05fb-4f3f-8ee2-22d82540ab83"]
	if resume.Type != TypeFile {
		t.Errorf("resume field = %+v", resume)
	}

	if _, ok := byID["hidden-1"]; ok {
		t.Errorf("hidden question must be excluded, got %+v", byID)
	}
}
