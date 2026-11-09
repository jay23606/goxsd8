package strict

import (
	"errors"
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xpath"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// pdDeadline bounds each huge-exponent row: a comparison that materialises
// 10^|scale| does not finish inside it (#1849), while one by adjusted exponent
// takes microseconds.
const pdDeadline = 10 * time.Second

// withinDeadline runs f and fails the test if it has not returned in pdDeadline.
// A row that times out leaves f running; the test binary exits with it.
func withinDeadline(t *testing.T, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(pdDeadline):
		t.Fatalf("did not finish within %s", pdDeadline)
	}
}

// pdValue parses lexical through strict's precisionDecimal mapping.
func pdValue(t *testing.T, lexical string) precisionDecimalVal {
	t.Helper()
	v, err := parsePrecisionDecimal(lexical, nil)
	if err != nil {
		t.Fatalf("Parse(%q): %v", lexical, err)
	}
	return v.(precisionDecimalVal)
}

// TestPrecisionDecimalCmpHugeExponent pins the §3.1 order on values whose
// exponents differ by about 10^9 (#1849): Cmp orders them by ·numericalValue·
// and finishes inside pdDeadline.
func TestPrecisionDecimalCmpHugeExponent(t *testing.T) {
	cases := []struct {
		a, b string
		want value.Ordering
	}{
		{"1E999999999", "10", value.Greater},
		{"10", "1E999999999", value.Less},
		{"1E-999999999", "10", value.Less},
		{"-1E999999999", "10", value.Less},
		{"-1E999999999", "-10", value.Less},
		{"-1E-999999999", "-10", value.Greater},
		{"0", "1E-999999999", value.Less},
		{"-0E999999999", "0E-999999999", value.Equal},
		{"1E999999999", "10E999999998", value.Equal},
		{"1.5E999999999", "15E999999998", value.Equal},
		{"1.5E999999999", "1.4999E999999999", value.Greater},
		{"1.4999E999999999", "1.5E999999999", value.Less},
		{"-1.4999E999999999", "-1.5E999999999", value.Greater},
		{"9.9E999999998", "1E999999999", value.Less},
	}
	for _, c := range cases {
		t.Run(c.a+" vs "+c.b, func(t *testing.T) {
			a, b := pdValue(t, c.a), pdValue(t, c.b)
			var got value.Ordering
			withinDeadline(t, func() { got = a.Cmp(b) })
			if got != c.want {
				t.Errorf("Cmp(%s, %s) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

// TestPrecisionDecimalFacetsHugeExponent pins the bound and enumeration facets
// against a literal whose exponent dwarfs the facet's (#1849): each rejection
// charges its own rule (cvc-maxInclusive-valid, cvc-minInclusive-valid,
// cvc-enumeration-valid, §4.3) inside pdDeadline.
func TestPrecisionDecimalFacetsHugeExponent(t *testing.T) {
	pd := newPrim(t, "precisionDecimal")
	cases := []struct {
		name    string
		facet   xsd.Facet
		lexical string
		rule    xsderr.Rule
	}{
		{"maxInclusive 10", xsd.NewFacet(xsd.FacetMaxInclusive, []string{"10"}, false), "1E999999999", "cvc-maxInclusive-valid"},
		{"minInclusive 10", xsd.NewFacet(xsd.FacetMinInclusive, []string{"10"}, false), "1E-999999999", "cvc-minInclusive-valid"},
		{"enumeration 10", xsd.NewEnumerationFacet([]xsd.EnumerationMember{xsd.NewEnumerationMember("10", nil, nil)}), "1E999999999", "cvc-enumeration-valid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := derive(t, "huge", pd, c.facet)
			var err error
			withinDeadline(t, func() {
				_, err = value.ValidateLexical(New(), noSchema{}, st, c.lexical, nil, xpath.FacetAssertions(time.Time{}))
			})
			wantRule(t, err, c.rule)
		})
	}
}

// TestPrecisionDecimalUnboundedExponent pins that an exponent past the host int
// is read whole (#1848): the literal matches pDecimalRep (§3.2), so it is never
// charged cvc-datatype-valid, and its ·scale· (vp-sciPrecision, §3.1) is exact.
func TestPrecisionDecimalUnboundedExponent(t *testing.T) {
	cases := []struct{ lexical, scale string }{
		{"1E-99999999999999999999", "99999999999999999999"},
		{"1E99999999999999999999", "-99999999999999999999"},
		{"1E-9223372036854775808", "9223372036854775808"},
		{"1.5E-9223372036854775807", "9223372036854775808"},
		{"1.25E-9223372036854775806", "9223372036854775808"},
		{"-0.0E+9223372036854775809", "-9223372036854775808"},
	}
	pd := newPrim(t, "precisionDecimal")
	for _, c := range cases {
		t.Run(c.lexical, func(t *testing.T) {
			if _, err := value.ValidateLexical(New(), noSchema{}, pd, c.lexical, nil, xpath.FacetAssertions(time.Time{})); err != nil {
				t.Fatalf("ValidateLexical(xs:precisionDecimal, %q) = %v, want accept", c.lexical, err)
			}
			got, ok := pdValue(t, c.lexical).Scale()
			if !ok || got.String() != c.scale {
				t.Errorf("Scale(%q) = (%v, %v), want (%s, true)", c.lexical, got, ok, c.scale)
			}
		})
	}
}

// TestPrecisionDecimalScaleIsACopy pins Scale's ownership contract
// (value.Scaled): mutating the returned ·scale· leaves the value unchanged.
func TestPrecisionDecimalScaleIsACopy(t *testing.T) {
	v := pdValue(t, "3.00")
	s, _ := v.Scale()
	s.SetInt64(7)
	if again, _ := v.Scale(); again.Int64() != 2 {
		t.Errorf("Scale after mutating an earlier result = %v, want 2", again)
	}
}

// TestPrecisionDecimalFacetsUnboundedScale pins the facets against a ·scale· past
// the host int (#1848): maxScale and minScale (cvc-maxScale-valid,
// cvc-minScale-valid, xsd-precisionDecimal §4.2.3/§4.3.3) compare it whole rather
// than wrapped, and the bound facets (cvc-maxInclusive-valid,
// cvc-minExclusive-valid) order a value of magnitude 10^-9223372036854775808
// below 0.5.
func TestPrecisionDecimalFacetsUnboundedScale(t *testing.T) {
	pd := newPrim(t, "precisionDecimal")
	maxScale0 := xsd.NewFacet(xsd.FacetMaxScale, []string{"0"}, false)
	cases := []struct {
		name    string
		facet   xsd.Facet
		lexical string
		rule    xsderr.Rule // "" means accepted
	}{
		{"maxScale 0 scale MaxInt+1", maxScale0, "1E-9223372036854775808", "cvc-maxScale-valid"},
		{"maxScale 0 scale 1+MaxInt", maxScale0, "1.5E-9223372036854775807", "cvc-maxScale-valid"},
		{"maxScale 0 scale 2+(MaxInt-1)", maxScale0, "1.25E-9223372036854775806", "cvc-maxScale-valid"},
		{"maxScale 0 scale -huge", maxScale0, "1E99999999999999999999", ""},
		{"minScale 0 scale MinInt-1", xsd.NewFacet(xsd.FacetMinScale, []string{"0"}, false), "1E9223372036854775809", "cvc-minScale-valid"},
		{"maxInclusive 0.5", xsd.NewFacet(xsd.FacetMaxInclusive, []string{"0.5"}, false), "1E-9223372036854775808", ""},
		{"minExclusive 0.5", xsd.NewFacet(xsd.FacetMinExclusive, []string{"0.5"}, false), "1E-9223372036854775808", "cvc-minExclusive-valid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := derive(t, "scaled", pd, c.facet)
			_, err := value.ValidateLexical(New(), noSchema{}, st, c.lexical, nil, xpath.FacetAssertions(time.Time{}))
			if c.rule == "" {
				wantAccept(t, err)
				return
			}
			wantRule(t, err, c.rule)
		})
	}
}

// TestPrecisionDecimalCanonicalUnboundedExponent pins Mapping.Canonical
// (canonicalPrecisionDecimal) on a value whose ·scale· is huge (§6): a nonzero
// value's scientific exponent is printed whole, while a zero above
// maxZeroCanonicalScale — one whose ·scale· lies past the host int included —
// is the beyond-capacity error (xmlschema11-2 §5.4), never a padded form or a
// "0.0E-(aP−1)" spelling, and never a validity verdict. A zero AT the bound
// still renders, and round-trips.
func TestPrecisionDecimalCanonicalUnboundedExponent(t *testing.T) {
	cases := []struct {
		lexical, want string // want "" is the beyond-capacity error
	}{
		{"1E-99999999999999999999", "1E-99999999999999999999"},
		{"-1.50E99999999999999999999", "-1.50E99999999999999999999"},
		{"0E-1048577", ""},
		{"0E-99999999999", ""},
		{"0E-9223372036854775809", ""},
		{"0E-99999999999999999999", ""},
		{"-0.0E-99999999999999999999", ""},
	}
	for _, c := range cases {
		t.Run(c.lexical, func(t *testing.T) {
			got, err := canonicalPrecisionDecimal(pdValue(t, c.lexical))
			if c.want != "" {
				if err != nil || got != c.want {
					t.Fatalf("Canonical(%q) = %q, %v; want %q", c.lexical, got, err, c.want)
				}
				return
			}
			if !errors.Is(err, errPrecisionDecimalCapacity) || got != "" {
				t.Fatalf("Canonical(%q) = %d bytes %.40q…, %v; want the beyond-capacity error", c.lexical, len(got), got, err)
			}
			if rule, verdict := xsderr.RuleOf(err); verdict {
				t.Errorf("Canonical(%q): error carries rule %s; a beyond-capacity decline is no validity verdict", c.lexical, rule)
			}
		})
	}

	t.Run("0E-1048576 renders at the bound", func(t *testing.T) {
		v := pdValue(t, "0E-1048576")
		got, err := canonicalPrecisionDecimal(v)
		if err != nil {
			t.Fatalf("Canonical(0E-1048576): %v", err)
		}
		if len(got) != maxZeroCanonicalScale+4 || !pdValue(t, got).Identical(v) {
			t.Errorf("Canonical(0E-1048576) has length %d and does not round-trip to the value", len(got))
		}
	})
}
