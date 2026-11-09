package strict_test

import (
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

func pdMapping(t *testing.T) value.Mapping {
	t.Helper()
	m, ok := strict.New().Mapping(xsd.QName{Space: xsd.XMLSchemaNS, Local: "precisionDecimal"})
	if !ok {
		t.Fatal("strict backend does not map xs:precisionDecimal")
	}
	return m
}

func parsePD(t *testing.T, m value.Mapping, lexical string) value.Value {
	t.Helper()
	v, err := m.Parse(lexical, nil)
	if err != nil {
		t.Fatalf("Parse(%q): unexpected error %v", lexical, err)
	}
	return v
}

// TestPrecisionDecimalCanonical exercises the ·precisionDecimalCanonicalMap· (§6)
// with the spec's own authoritative example table (§3.2), plus the four special
// literals. The headline property is trailing-zero preservation: 3.00 canonicalizes
// to "3.00", NOT "3" (quantum-preserving canonical), and negative-scale values have
// only a scientific form.
func TestPrecisionDecimalCanonical(t *testing.T) {
	m := pdMapping(t)
	cases := map[string]string{
		// §3.2 example table (lexical → canonical).
		"3":      "3",
		"3.00":   "3.00",
		"03.00":  "3.00",
		"300":    "300",
		"3.00e2": "300",
		"3.0e2":  "3.0E2",
		"30e1":   "3.0E2",
		".30e3":  "3.0E2",
		"-1.23":  "-1.23",
		"+1.0":   "1.0", // scale preserved, unlike decimal's "1"
		"0.05":   "0.05",
		// The four special literals map to their canonical form.
		"INF":  "INF",
		"+INF": "INF",
		"-INF": "-INF",
		"NaN":  "NaN",
	}
	for lex, want := range cases {
		v := parsePD(t, m, lex)
		got, err := m.Canonical(v)
		if err != nil {
			t.Errorf("Canonical(%q): unexpected error %v", lex, err)
			continue
		}
		if got != want {
			t.Errorf("Canonical(%q) = %q, want %q", lex, got, want)
		}
	}
}

// TestPrecisionDecimalCanonicalRoundTrips pins dt-canonical-mapping's
// lexicalMap(canonicalMap(v)) = v for finite values: each lexical's canonical
// form is the string given and re-parses (f-precDecLexmap) to an Identical
// value — the same ·numericalValue·, ·scale· (vp-sciPrecision) and ·sign·
// (vp-pd-sign). The single-digit-coefficient rows of negative ·scale· or out of
// range, and the zeros of ·scale· ≤ 0, are the ones f-precDecCanmap step 5 as
// written gives no Identical form for (#3515); the rest pin forms it does give.
func TestPrecisionDecimalCanonicalRoundTrips(t *testing.T) {
	m := pdMapping(t)
	cases := []struct{ lexical, want string }{
		// Zeros: scientificCanonicalMap(0) is undefined, so the form is ours.
		{"0", "0E0"},
		{"-0", "-0E0"},
		{"0E5", "0E5"},
		{"-0E5", "-0E5"},
		{"0E1", "0E1"},
		{"0.0", "0.0E0"},
		{"0.00", "0.00E0"},
		// One-digit coefficients that reach step 5.
		{"3E2", "3E2"},
		{"1E7", "1E7"},
		{"-1E7", "-1E7"},
		{"1E-7", "1E-7"},
		{"1E6", "1E6"},
		{"5E1", "5E1"},
		// Forms step 5 and steps 3-4 already give.
		{"3", "3"},
		{"3.00", "3.00"},
		{"03.00", "3.00"},
		{"300", "300"},
		{"3.00e2", "300"},
		{"3.0e2", "3.0E2"},
		{"30e1", "3.0E2"},
		{".30e3", "3.0E2"},
		{"-1.23", "-1.23"},
		{"+1.0", "1.0"},
		{"0.05", "0.05"},
		{"1.5E-3", "0.0015"},
	}
	for _, c := range cases {
		v := parsePD(t, m, c.lexical).(value.Identical)
		got, err := m.Canonical(v)
		if err != nil {
			t.Errorf("Canonical(%q): unexpected error %v", c.lexical, err)
			continue
		}
		if got != c.want {
			t.Errorf("Canonical(%q) = %q, want %q", c.lexical, got, c.want)
		}
		if !v.Identical(parsePD(t, m, got)) {
			t.Errorf("Canonical(%q) = %q, which re-parses to a value not Identical to %q", c.lexical, got, c.lexical)
		}
	}
}

// TestPrecisionDecimalCanonicalContrastsDecimal pins the distinction from xs:decimal:
// decimal collapses precision (1.00 → "1"), precisionDecimal preserves scale
// (1.00 → "1.00"). A regression that reused decimal's canonical would break this.
func TestPrecisionDecimalCanonicalContrastsDecimal(t *testing.T) {
	pd := pdMapping(t)
	got, err := pd.Canonical(parsePD(t, pd, "1.00"))
	if err != nil {
		t.Fatalf("Canonical(1.00): %v", err)
	}
	if got != "1.00" {
		t.Errorf("precisionDecimal Canonical(1.00) = %q, want %q", got, "1.00")
	}
}

// TestPrecisionDecimalReject covers the lexical-space boundary (§3.2): the special
// literals are EXACTLY INF/+INF/-INF/NaN — no IEEE "INFINITY", no case variants, no
// signed NaN — and whitespace/garbage is out of the space.
func TestPrecisionDecimalReject(t *testing.T) {
	m := pdMapping(t)
	for _, lex := range []string{
		"INFINITY", "+INFINITY", "inf", "Inf", "nan", "NAN", "+NaN", "-NaN",
		"", "+", "-", ".", " 1", "1 ", "1.2.3", "abc", "0x1", "1,000", "E2", "1e", "1e+",
	} {
		_, err := m.Parse(lex, nil)
		if err == nil {
			t.Errorf("Parse(%q): want lexical-space error, got nil", lex)
			continue
		}
		if rule, ok := xsderr.RuleOf(err); !ok || rule != "cvc-datatype-valid" {
			t.Errorf("Parse(%q): rule = %q (ok=%v), want cvc-datatype-valid", lex, rule, ok)
		}
	}
}

// TestPrecisionDecimalEqIsQuantumBlind proves Eq/Cmp look only at ·numericalValue·
// (§3.1): 1.0 and 1.00 (same numericalValue, different scale) are EQUAL, and the
// partial order places −INF below and +INF above every numeric value.
func TestPrecisionDecimalEqIsQuantumBlind(t *testing.T) {
	m := pdMapping(t)
	oneScale1 := parsePD(t, m, "1.0").(value.Ordered)
	oneScale2 := parsePD(t, m, "1.00")
	bigger := parsePD(t, m, "10")
	posInf := parsePD(t, m, "INF")
	negInf := parsePD(t, m, "-INF")

	if got := oneScale1.Cmp(oneScale2); got != value.Equal {
		t.Errorf("Cmp(1.0, 1.00) = %v, want Equal (quantum-blind)", got)
	}
	if !oneScale1.Eq(oneScale2) {
		t.Error("Eq(1.0, 1.00) = false, want true (quantum-blind)")
	}
	if got := oneScale1.Cmp(bigger); got != value.Less {
		t.Errorf("Cmp(1.0, 10) = %v, want Less", got)
	}
	if got := oneScale1.Cmp(posInf); got != value.Less {
		t.Errorf("Cmp(1.0, INF) = %v, want Less", got)
	}
	if got := oneScale1.Cmp(negInf); got != value.Greater {
		t.Errorf("Cmp(1.0, -INF) = %v, want Greater", got)
	}
	if got := posInf.(value.Ordered).Cmp(posInf); got != value.Equal {
		t.Errorf("Cmp(INF, INF) = %v, want Equal", got)
	}
	// A foreign value is Incomparable / unequal, not a panic.
	if got := oneScale1.Cmp("not a precisionDecimal"); got != value.Incomparable {
		t.Errorf("Cmp(precisionDecimal, string) = %v, want Incomparable", got)
	}
	if oneScale1.Eq(42) {
		t.Error("Eq(precisionDecimal, int) = true, want false")
	}
}

// TestPrecisionDecimalIdenticalIsScaleSensitive proves Identical is DISTINCT from Eq
// (PRINCIPLES 18): 3, 3.0 and 3.00 are three distinct identities and +0 is not
// identical to −0, even though every pair compares Eq-equal.
func TestPrecisionDecimalIdenticalIsScaleSensitive(t *testing.T) {
	m := pdMapping(t)
	three := parsePD(t, m, "3").(value.Identical)
	threeS1 := parsePD(t, m, "3.0")
	threeS2 := parsePD(t, m, "3.00")
	threeAgain := parsePD(t, m, "03") // same triple (3, 0, positive) as "3"

	if three.Identical(threeS1) {
		t.Error("Identical(3, 3.0) = true, want false (scales 0 vs 1)")
	}
	if three.Identical(threeS2) {
		t.Error("Identical(3, 3.00) = true, want false (scales 0 vs 2)")
	}
	if !three.Identical(threeAgain) {
		t.Error("Identical(3, 03) = false, want true (same value triple)")
	}
	// Yet all are Eq-equal: identity and equality genuinely diverge.
	if !three.(value.Eq).Eq(threeS2) {
		t.Error("Eq(3, 3.00) = false, want true")
	}

	// +0 and −0 are Eq-equal but NOT identical (sign distinguishes them).
	posZero := parsePD(t, m, "0").(value.Identical)
	negZero := parsePD(t, m, "-0")
	if posZero.Identical(negZero) {
		t.Error("Identical(+0, -0) = true, want false (distinct signs)")
	}
	if !posZero.(value.Eq).Eq(negZero) {
		t.Error("Eq(+0, -0) = false, want true")
	}
}

// TestPrecisionDecimalNaN pins NaN's split personality: Incomparable with everything
// under Cmp (including itself, so never Eq), yet Identical to itself (§3.1 order vs
// PRINCIPLES 18 identity). A foreign argument is likewise handled without panic.
func TestPrecisionDecimalNaN(t *testing.T) {
	m := pdMapping(t)
	nan := parsePD(t, m, "NaN")
	nan2 := parsePD(t, m, "NaN")
	one := parsePD(t, m, "1")

	if got := nan.(value.Ordered).Cmp(nan2); got != value.Incomparable {
		t.Errorf("Cmp(NaN, NaN) = %v, want Incomparable", got)
	}
	if got := nan.(value.Ordered).Cmp(one); got != value.Incomparable {
		t.Errorf("Cmp(NaN, 1) = %v, want Incomparable", got)
	}
	if nan.(value.Eq).Eq(nan2) {
		t.Error("Eq(NaN, NaN) = true, want false (NaN never equals itself)")
	}
	if !nan.(value.Identical).Identical(nan2) {
		t.Error("Identical(NaN, NaN) = false, want true (single notANumber value)")
	}
	if nan.(value.Identical).Identical(one) {
		t.Error("Identical(NaN, 1) = true, want false")
	}
}

// TestPrecisionDecimalScale proves the value carries value.Scaled with ·scale·
// present for numerics and ABSENT (ok=false) for the special values (§3.1: scale is
// absent iff numericalValue is special).
func TestPrecisionDecimalScale(t *testing.T) {
	m := pdMapping(t)
	cases := map[string]int64{"3": 0, "3.00": 2, "3.0e2": -1, "0.05": 2}
	for lex, want := range cases {
		s, ok := parsePD(t, m, lex).(value.Scaled)
		if !ok {
			t.Fatalf("value of %q does not implement value.Scaled", lex)
		}
		got, present := s.Scale()
		if !present {
			t.Errorf("Scale(%q): ok=false, want a present scale", lex)
			continue
		}
		if !got.IsInt64() || got.Int64() != want {
			t.Errorf("Scale(%q) = %d, want %d", lex, got, want)
		}
	}
	for _, lex := range []string{"INF", "-INF", "NaN"} {
		s := parsePD(t, m, lex).(value.Scaled)
		if got, present := s.Scale(); present || got != nil {
			t.Errorf("Scale(%q) = (%v, %v), want (nil, false) for a special value", lex, got, present)
		}
	}
}

// TestPrecisionDecimalTotalDigits pins cvc-totalDigits-valid's value (§4.1): the
// coefficient's digit count INCLUDING trailing zeros (distinct from decimal), with
// zero and the specials special-cased to 1 (unconditionally facet-valid, avoiding
// log10(0)).
func TestPrecisionDecimalTotalDigits(t *testing.T) {
	m := pdMapping(t)
	cases := map[string]int{
		"3.00":  3, // coefficient 300 → 3 digits (trailing zeros count)
		"3.0e2": 2, // coefficient 30 → 2 digits
		"300":   3,
		"0.05":  1, // coefficient 5
		"0":     1, // zero convention (unconditionally valid)
		"0.00":  1, // zero, any scale
		"INF":   1,
		"NaN":   1,
	}
	for lex, want := range cases {
		v := parsePD(t, m, lex).(value.DigitCounted)
		if got := v.TotalDigits(); got != want {
			t.Errorf("TotalDigits(%q) = %d, want %d", lex, got, want)
		}
	}
}

// TestPrecisionDecimalCanonicalForeign proves the canonical wrapper rejects a foreign
// value as an *xsderr.Error rather than panicking (warden guardrail).
func TestPrecisionDecimalCanonicalForeign(t *testing.T) {
	m := pdMapping(t)
	_, err := m.Canonical("not a precisionDecimal")
	if err == nil {
		t.Fatal("Canonical(foreign): want error, got nil")
	}
	if rule, ok := xsderr.RuleOf(err); !ok || rule != "cvc-datatype-valid" {
		t.Errorf("Canonical(foreign): rule = %q (ok=%v), want cvc-datatype-valid", rule, ok)
	}
}
