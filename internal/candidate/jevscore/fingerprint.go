package jevscore

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
)

// ProfileFingerprint stably hashes the profile-side inputs that decide the fine
// eligibility gate (jeveligible.Eligible): specializations, seniorities, skills, and the
// raw location_preferences JSONB block. It is the second staleness stamp beside the CV
// upload time: EnqueueJevScoresForProfile's freshness check and UpsertUserJobScore's
// stamped write must both call this on the SAME profile fields, or a coarse re-enqueue
// pass would either never fire (a real profile edit that hashes the same) or fire on
// every run (two call sites that hash differently) — see
// internal/platform/db/queries/user_job_scores.sql.
//
// locationPreferences is the profile's raw JSONB column (json.RawMessage), taken
// verbatim rather than re-marshaled: normalizeLocationPreferences (userprofile.Service.
// Save) is the only writer and always produces it from the same struct in the same
// field order, so two saves of an equal location always serialize identically.
func ProfileFingerprint(specializations, seniorities, skills []string, locationPreferences []byte) string {
	h := sha256.New()
	writeStrings(h, specializations)
	writeStrings(h, skills)
	writeStrings(h, seniorities)
	h.Write([]byte{0})
	h.Write(locationPreferences)
	return hex.EncodeToString(h.Sum(nil))
}

// writeStrings feeds a null-separated, then list-terminated encoding of values into h, so
// {"a", "bc"} and {"ab", "c"} never collide the way plain concatenation would.
func writeStrings(h hash.Hash, values []string) {
	for _, v := range values {
		_, _ = h.Write([]byte(v))
		_, _ = h.Write([]byte{0})
	}
	_, _ = h.Write([]byte{0xFF})
}
