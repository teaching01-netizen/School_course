// Package normalize provides canonicalization of legacy-system data:
// text and datetime normalization, immutable canonical source models,
// deterministic canonical JSON, and content hashing.
package normalize

import (
	"errors"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// thaiSaraAmExpanded is the canonical decomposition that NFKC applies to
// U+0E33 THAI CHARACTER SARA AM ("ำ"): NIKHAHIT (U+0E4D) + SARA AA
// (U+0E32). It is the only Thai character with a canonical decomposition,
// so it is the only way NFKC can alter well-formed Thai text. We recombine
// it so that Thai text survives byte-exact (see NormalizeText).
const (
	thaiSaraAm         = "\u0e33"
	thaiSaraAmExpanded = "\u0e4d\u0e32"
)

// NormalizeText canonicalizes free text: Unicode NFKC normalization,
// non-breaking/other unicode spaces -> ASCII space, whitespace runs
// collapsed to a single space, trimmed. Preserves Thai text exactly
// (NFKC must not mangle Thai combining marks/vowels).
func NormalizeText(s string) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	s = norm.NFKC.String(s)
	// NFKC expands U+0E33 (SARA AM) to its canonical decomposition
	// (U+0E4D U+0E32); recombine so well-formed Thai text ("น้ำ", "ทำ", ...)
	// is preserved byte-exact. NFKC never produces this pair from anything
	// other than U+0E33, and the result is idempotent.
	s = strings.ReplaceAll(s, thaiSaraAmExpanded, thaiSaraAm)
	return strings.Join(strings.Fields(s), " ")
}

// NormalizeOptional normalizes an optional field: NormalizeText applied,
// then "" and "[NOT SET]" (case-insensitive, trimmed) map to "".
// Returns the normalized value and whether the field is set.
func NormalizeOptional(s string) (string, bool) {
	v := NormalizeText(s)
	if v == "" || strings.EqualFold(v, "[not set]") {
		return "", false
	}
	return v, true
}

// NormalizeID normalizes a legacy identifier: trim whitespace only.
// Leading zeroes are preserved exactly ("0078" -> "0078").
// Empty after trim -> error.
func NormalizeID(s string) (string, error) {
	v := strings.TrimSpace(s)
	if v == "" {
		return "", errors.New("normalize: empty legacy id")
	}
	return v, nil
}
