package strict

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strings"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsderr"
)

// precisionDecimalLexical is the precisionDecimal lexical space (xsd-precisionDecimal
// §3.2, pDecimalRep): an optional-sign numeral with an optional fractional tail and
// optional [Ee] exponent, OR one of exactly four special literals INF/+INF/-INF/NaN.
// The special sub-grammar is deliberately narrow: only 'INF' and 'NaN' are chosen,
// never the IEEE 'INFINITY' spelling or any case variant (§3.2 Note), and the NaN
// alternative carries no sign, so "+NaN"/"-NaN" are rejected while "+INF" is accepted.
// Anchored whole-string; whiteSpace=collapse is a pre-lexical pipeline stage, so
// Parse rejects any stray whitespace here. The production is byte-identical to
// float/double's regex because they share numericalSpecialRep, but it is kept
// separate: they are distinct lexical spaces (§3.2 vs §3.3.4.2/§3.3.5.2) whose spec
// citations must not be coupled through a shared variable.
var precisionDecimalLexical = regexp.MustCompile(`^((\+|-)?([0-9]+(\.[0-9]*)?|\.[0-9]+)([Ee](\+|-)?[0-9]+)?|(\+|-)?INF|NaN)$`)

// pdKind is the discriminated-union tag of a precisionDecimal value: the numeric
// arm carries a (coefficient, scale, sign) triple, while the three special values
// carry NONE of them — the spec's own iff-absence rules (·scale· absent iff
// numericalValue is special; ·sign· absent iff notANumber) become unrepresentable
// illegal states rather than runtime invariants (warden guardrail, STYLE T1).
type pdKind uint8

const (
	// pdNumeric is a numerical value: coefficient/scale/sign are meaningful.
	pdNumeric pdKind = iota
	// pdPosInf is positiveInfinity; scale and sign are absent (sign is implied).
	pdPosInf
	// pdNegInf is negativeInfinity; scale and sign are absent (sign is implied).
	pdNegInf
	// pdNaN is notANumber; scale and sign are both absent.
	pdNaN
)

// pdSign is the ·sign· property of a numeric precisionDecimal value. It is stored
// in its own field, NOT derived from the coefficient, because it is the only fact
// that distinguishes +0 from −0 — a distinction the spec keeps for canonical output
// and for Identical (§3.1: ·sign· is redundant "except when numericalValue is zero").
type pdSign uint8

const (
	// signPositive is ·sign· = positive.
	signPositive pdSign = iota
	// signNegative is ·sign· = negative.
	signNegative
)

// precisionDecimalVal is an xs:precisionDecimal value (xsd-precisionDecimal §3.1),
// the (·numericalValue·, ·scale·, ·sign·) triple. The numericalValue magnitude is
// coefficient × 10^(-scale) with coefficient an integer ≥ 0; scale is preserved
// VERBATIM from the lexical, so 3, 3.0 and 3.00 are distinct values (coefficient/
// scale (3,0), (30,1), (300,2)) that nonetheless compare numerically equal
// (PRINCIPLES 18). ·scale· is an unbounded integer held as a *big.Int (§3.1), so a
// literal whose exponent lies past the host int is decided exactly, never charged
// or wrapped, and no operation materialises 10^·scale·. It is value.Ordered with a
// PARTIAL order (§3.1: NaN incomparable with everything including itself),
// value.Eq, value.Identical (scale-sensitive), value.Scaled and value.DigitCounted
// (totalDigits). It is deliberately NOT value.Lengthed/TimezoneAware: no
// precisionDecimal-applicable facet needs them (§3.3). Nor is it value.Canonical:
// a zero's canonical form can lie beyond this processor's capacity
// (zeroCanonical), which only Mapping.Canonical's error can say.
//
// maxScale/minScale, precisionDecimal's two extension facets (§4.2/§4.3), are
// enforced at instance validation by value/facets.go's scaleFacet, which reads
// ·scale· through the value.Scaled capability this value model implements below
// (cvc-maxScale-valid, cvc-minScale-valid; #133). This mapping supplies the
// value; the facet check lives in the backend-generic pipeline.
type precisionDecimalVal struct {
	kind pdKind
	// coefficient is the integer significand magnitude (≥ 0); numeric arm only.
	coefficient *big.Int
	// scale is ·scale· (aP), kept verbatim from the lexical; numeric arm only.
	// It is an unbounded integer (§3.1 vp-pd-precision): the exponent it derives
	// from is an unbounded noDecimalPtNumeral (§3.2), so it is never narrowed to
	// a host int. Never mutated after parsePrecisionDecimal builds it.
	scale *big.Int
	// sign is ·sign·; numeric arm only. Stored, not derived (distinguishes ±0).
	sign pdSign
}

// parsePrecisionDecimal maps a pDecimalRep to its value (·precisionDecimalLexicalMap·,
// §3.2 steps 1–4): the special literals resolve to their kind, and a numeral splits
// into a magnitude coefficient (intPart+fracPart digits) and a ·scale· of
// len(fracPart) − exponent, so 3.0e2 → (coefficient 30, scale −1) and 3.00 →
// (coefficient 300, scale 2). Trailing/leading zeros are preserved into the scale,
// never stripped: the (coefficient, scale) pair IS the identity.
func parsePrecisionDecimal(lexical string, _ value.Context) (value.Value, error) {
	if !precisionDecimalLexical.MatchString(lexical) {
		return nil, xsderr.New(ruleDatatypeValid, xsderr.Loc{},
			"%q is not in the lexical space of precisionDecimal, which cvc-datatype-valid clause 2.1 requires it to be in", lexical)
	}

	// specialRepValue (step 1, otherwise clause): only these four literals.
	switch lexical {
	case "INF", "+INF":
		return precisionDecimalVal{kind: pdPosInf}, nil
	case "-INF":
		return precisionDecimalVal{kind: pdNegInf}, nil
	case "NaN":
		return precisionDecimalVal{kind: pdNaN}, nil
	}

	sign := signPositive
	body := lexical
	switch body[0] {
	case '+':
		body = body[1:]
	case '-':
		sign = signNegative
		body = body[1:]
	}

	// The exponent is an unbounded noDecimalPtNumeral (pDecimalRep, §3.2), read
	// whole: no exponent the lexical space admits is charged or narrowed.
	exp := new(big.Int)
	if i := strings.IndexAny(body, "Ee"); i >= 0 {
		if _, ok := exp.SetString(body[i+1:], 10); !ok {
			return nil, xsderr.New(ruleDatatypeValid, xsderr.Loc{},
				"%q has no exponent digits, so it is not in the lexical space of precisionDecimal, which cvc-datatype-valid clause 2.1 requires it to be in", lexical)
		}
		body = body[:i]
	}

	intPart, fracPart := body, ""
	if i := strings.IndexByte(body, '.'); i >= 0 {
		intPart, fracPart = body[:i], body[i+1:]
	}

	// The regex guarantees at least one digit across intPart+fracPart, so SetString
	// cannot fail; leading zeros are absorbed by base-10 parsing. The coefficient is
	// the magnitude — the sign is a separate stored fact.
	coeff, ok := new(big.Int).SetString(intPart+fracPart, 10)
	if !ok {
		return nil, xsderr.New(ruleDatatypeValid, xsderr.Loc{},
			"%q has no digits, so it is not in the lexical space of precisionDecimal, which cvc-datatype-valid clause 2.1 requires it to be in", lexical)
	}

	// ·scale· (step 2): decimalPtPrecision (len fracPart) for a plain numeral,
	// scientificPrecision (that count minus the exponent) for scientific notation.
	scale := big.NewInt(int64(len(fracPart)))
	return precisionDecimalVal{kind: pdNumeric, coefficient: coeff, scale: scale.Sub(scale, exp), sign: sign}, nil
}

// signum is the sign of ·numericalValue· (numeric arm only): 0 for a zero of either
// ·sign·, which §3.1 orders equal, else −1 or +1 by the stored ·sign·.
func (p precisionDecimalVal) signum() int {
	if p.coefficient.Sign() == 0 {
		return 0
	}
	if p.sign == signNegative {
		return -1
	}
	return 1
}

// adjusted is the adjusted exponent of a nonzero numeric value: (D − 1) − ·scale·
// for a D-digit coefficient, so 10^adjusted ≤ |numericalValue| < 10^(adjusted+1).
// It is computed without materialising any power of ten (#1849).
func (p precisionDecimalVal) adjusted() *big.Int {
	adj := big.NewInt(int64(len(p.coefficient.String()) - 1))
	return adj.Sub(adj, p.scale)
}

// cmpMagnitude orders |numericalValue| of two nonzero numeric values: by adjusted
// exponent first, and on a tie by coefficient after aligning the scales. Equal
// adjusted exponents force the two scales to differ by exactly the difference of
// the coefficients' digit counts, so the one power of ten built here is bounded by
// the literals' lengths, never by their exponents (#1849).
func (p precisionDecimalVal) cmpMagnitude(o precisionDecimalVal) int {
	if c := p.adjusted().Cmp(o.adjusted()); c != 0 {
		return c
	}
	pc, oc := p.coefficient, o.coefficient
	shift := len(oc.String()) - len(pc.String())
	if shift > 0 {
		pc = new(big.Int).Mul(pc, new(big.Int).Exp(bigTen, big.NewInt(int64(shift)), nil))
	}
	if shift < 0 {
		oc = new(big.Int).Mul(oc, new(big.Int).Exp(bigTen, big.NewInt(int64(-shift)), nil))
	}
	return pc.Cmp(oc)
}

// Cmp is the PARTIAL order on precisionDecimal (§3.1): −INF < every numeric value <
// +INF, numeric values order by ·numericalValue· (scale-blind), and NaN is
// Incomparable with everything, itself included. A non-precisionDecimal argument is
// Incomparable rather than a spurious order (rf-ordered). Two numeric values order
// by signum, then by magnitude (cmpMagnitude), so the comparison costs time in the
// literals' lengths and never in their exponents' magnitudes (#1849).
func (p precisionDecimalVal) Cmp(other value.Value) value.Ordering {
	o, ok := other.(precisionDecimalVal)
	if !ok {
		return value.Incomparable
	}
	if p.kind == pdNaN || o.kind == pdNaN {
		return value.Incomparable
	}
	switch p.kind {
	case pdPosInf:
		if o.kind == pdPosInf {
			return value.Equal
		}
		return value.Greater
	case pdNegInf:
		if o.kind == pdNegInf {
			return value.Equal
		}
		return value.Less
	case pdNumeric, pdNaN:
		// p is numeric (NaN returned above); fall through to the numeric branch.
	}
	switch o.kind {
	case pdPosInf:
		return value.Less
	case pdNegInf:
		return value.Greater
	case pdNumeric, pdNaN:
		// both numeric (o NaN returned above); compare numericalValue below.
	}
	c := cmp.Compare(p.signum(), o.signum())
	if c == 0 && p.signum() != 0 {
		c = p.signum() * p.cmpMagnitude(o)
	}
	switch c {
	case -1:
		return value.Less
	case 1:
		return value.Greater
	}
	return value.Equal
}

// Eq is precisionDecimal equality (§3.1): it coincides with the order's Equal, so it
// is scale-blind (1.0 = 1.00), NaN ≠ NaN (Incomparable ⇒ not Equal) and each infinity
// equals only itself. Scale-sensitive distinctness lives on Identical, not here.
func (p precisionDecimalVal) Eq(other value.Value) bool { return p.Cmp(other) == value.Equal }

// Identical is the precisionDecimal identity relation (PRINCIPLES 18, §2.2.2),
// DISTINCT from Eq: it folds in ·scale· and ·sign·, so 3, 3.0 and 3.00 are three
// distinct values and +0 is not identical to −0, while NaN IS identical to itself
// (the single notANumber value). Enumeration matching (cvc-enumeration-valid,
// §4.3.5.4) reads this, not Eq.
func (p precisionDecimalVal) Identical(other value.Value) bool {
	o, ok := other.(precisionDecimalVal)
	if !ok {
		return false
	}
	if p.kind != o.kind {
		return false
	}
	if p.kind != pdNumeric {
		return true // NaN ≡ NaN, INF ≡ INF, −INF ≡ −INF
	}
	return p.sign == o.sign && p.scale.Cmp(o.scale) == 0 && p.coefficient.Cmp(o.coefficient) == 0
}

// Scale returns ·scale· (§3.1) as a fresh *big.Int the caller owns; ok is false,
// and scale nil, for the special values, whose ·scale· is absent, encoding the
// spec's "absent iff numericalValue is a special value".
func (p precisionDecimalVal) Scale() (*big.Int, bool) {
	if p.kind != pdNumeric {
		return nil, false
	}
	return new(big.Int).Set(p.scale), true
}

// intOf returns n as an int when the host int holds it exactly, and ok false
// otherwise: it never truncates.
func intOf(n *big.Int) (int, bool) {
	if !n.IsInt64() {
		return 0, false
	}
	v := n.Int64()
	if int64(int(v)) != v {
		return 0, false
	}
	return int(v), true
}

// TotalDigits is the value cvc-totalDigits-valid reads (§4.1): for a nonzero numeric
// value with ·numericalValue· nV and ·scale· aP the rule requires
// (aP + 1 + log10(|nV|) div 1) ≤ t. With nV = coefficient × 10^(-aP) and a
// D-digit coefficient, floor(log10(|nV|)) = (D−1) − aP, so the whole expression
// collapses to D — the coefficient's digit count (trailing zeros INCLUDED, unlike
// decimal, which counts the trailing-zero-stripped minimal form). Zero and the
// specials are unconditionally facet-valid (§4.1 clause 1, avoiding log10(0)); they
// report 1 so the pipeline's ≤ t check passes for every t ≥ 1.
func (p precisionDecimalVal) TotalDigits() int {
	if p.kind != pdNumeric || p.coefficient.Sign() == 0 {
		return 1
	}
	return len(p.coefficient.String())
}

// FractionDigits satisfies the value.DigitCounted interface but is inert for
// precisionDecimal: the fractionDigits facet is NOT applicable to this type (§3.3),
// so the facet pipeline never invokes it. It reports ·scale· (0 for the specials) as
// a spec-consistent value rather than a placeholder; a ·scale· past the host int,
// which no caller can reach, reports math.MaxInt or math.MinInt on its sign's side.
func (p precisionDecimalVal) FractionDigits() int {
	if p.kind != pdNumeric {
		return 0
	}
	if n, ok := intOf(p.scale); ok {
		return n
	}
	if p.scale.Sign() < 0 {
		return math.MinInt
	}
	return math.MaxInt
}

// canonicalPrecisionDecimal is the Mapping.Canonical wrapper: it rejects a foreign
// value as an *xsderr.Error rather than panicking (warden guardrail), and passes
// on canonical's beyond-capacity error unchanged.
func canonicalPrecisionDecimal(v value.Value) (string, error) {
	p, ok := v.(precisionDecimalVal)
	if !ok {
		return "", xsderr.New(ruleDatatypeValid, xsderr.Loc{},
			"precisionDecimal canonical: value of type %T is not a strict precisionDecimal", v)
	}
	return p.canonical()
}

// canonical renders the canonical pDecimalRep (·precisionDecimalCanonicalMap·, §6):
// the specials map to their fixed literal (step 2); an integer with ·scale· 0 in
// [1E−6, 1E6] renders as a bare numeral (step 3); a positive ·scale· in range renders
// with the decimal point placed and trailing zeros PADDED to the scale (step 4, so 3
// with scale 2 → "3.00"); everything else — a negative scale, or a magnitude outside
// the range — renders in scientific notation (step 5; a zero by zeroCanonical).
// Every finite form re-parses (f-precDecLexmap) to an Identical value, as
// dt-canonical-mapping requires, where step 5 as written does not: see
// zeroCanonical and scientificCanonical. Trailing zeros are canonical, never
// stripped: that is how the canonical form is quantum-preserving even though Eq is
// quantum-blind.
//
// The one error is zeroCanonical's beyond-capacity one. It is why
// precisionDecimalVal does not implement [value.Canonical]: that string-only
// signature has no outcome for a canonical form this processor will not produce,
// so the form is reachable only through this mapping's [value.Mapping.Canonical].
func (p precisionDecimalVal) canonical() (string, error) {
	switch p.kind {
	case pdNaN:
		return "NaN", nil
	case pdPosInf:
		return "INF", nil
	case pdNegInf:
		return "-INF", nil
	case pdNumeric:
		// Rendered by the numeric algorithm below.
	}

	// f-precDecCanmap never reads ·sign·, so as written 0 and -0 share one form,
	// against dt-canonical-mapping's one-to-one requirement; prepending "-" keeps
	// the ·sign· (vp-pd-sign) that f-precDecLexmap step 3 reads back. This departs
	// from the letter of the algorithm, a candidate erratum, not a reading.
	sign := ""
	if p.sign == signNegative {
		sign = "-"
	}

	// Zero has no log10 (§4.1) and is excluded from steps 3/4 by the 1E−6 lower
	// bound, so it takes step 5 (zeroCanonical).
	if p.coefficient.Sign() == 0 {
		unsigned, err := p.zeroCanonical()
		if err != nil {
			return "", err
		}
		return sign + unsigned, nil
	}

	digits := p.coefficient.String()
	if p.scale.Sign() >= 0 && p.inCanonicalRange() {
		// In range, adjusted = (D − 1) − aP ≥ −6 bounds aP by len(digits) + 5, so
		// intOf always holds it; the scientific form below is the guard's fallback.
		if s, ok := intOf(p.scale); ok {
			return sign + plainCanonical(digits, s), nil
		}
	}
	return sign + p.scientificCanonical(digits), nil
}

// maxZeroCanonicalScale is the largest ·scale· aP whose zero zeroCanonical
// renders, which bounds that canonical form, aP + 4 bytes long, at about 1 MiB.
// Any figure at or above 369, the maxScale a minimally conforming processor must
// support (xsd-precisionDecimal §5.1, implementation-limits), is conformant.
const maxZeroCanonicalScale = 1 << 20

// errPrecisionDecimalCapacity marks a zero whose canonical form lies beyond this
// processor's capacity (xmlschema11-2 §5.4): the value is valid and HAS a
// canonical form, which this processor declines to produce rather than quietly
// change. It is a plain error, never a validity verdict ([value.Mapping]: no
// cvc-* rule reads canonical form, and §5.4 forbids treating the value as
// invalid). It is unexported: no consumer tells it from errNoYearMonthCanonical's
// "no canonical form" yet (STYLE T5), but wrapping it keeps the case
// errors.Is-identifiable inside the package.
var errPrecisionDecimalCapacity = errors.New("precisionDecimal canonical: canonical form beyond this processor's capacity")

// zeroCanonical renders the unsigned canonical form of a zero (step 5), whose
// mantissa and exponent are implementer-supplied: scientificCanonicalMap(0) is
// undefined (log 0), so step 5 as written has no output for any zero. Each form
// re-parses to ·scale· aP (vp-sciPrecision): for aP ≤ 0 the point-less "0E"
// followed by −aP ("0E0", "0E5"), the shortest; for aP ≥ 1 the mantissa "0.0"
// (f = 1) padded with aP − 1 trailing zeros, exponent 0 ("0.0E0", "0.00E0").
//
// A ·scale· above maxZeroCanonicalScale, one past the host int included, is
// errPrecisionDecimalCapacity (xmlschema11-2 §5.4). It is decided on the
// *big.Int before any aP − 1 arithmetic or allocation, so no ·scale· is wrapped
// and no padding proportional to it is built.
func (p precisionDecimalVal) zeroCanonical() (string, error) {
	if p.scale.Sign() <= 0 {
		return "0E" + new(big.Int).Neg(p.scale).String(), nil
	}
	if p.scale.Cmp(big.NewInt(maxZeroCanonicalScale)) > 0 {
		return "", fmt.Errorf("%w: a zero of ·scale· %s, above the bound %d",
			errPrecisionDecimalCapacity, p.scale, maxZeroCanonicalScale)
	}
	return "0.0" + strings.Repeat("0", int(p.scale.Int64())-1) + "E0", nil
}

// plainCanonical renders steps 3 and 4 for a magnitude string digits of ·scale·
// s ≥ 0 in [1E−6, 1E6]: a bare numeral for s = 0, else the decimal point placed s
// digits from the right, left-padded with zeros, trailing zeros preserved.
func plainCanonical(digits string, s int) string {
	if s == 0 {
		return digits // step 3: bare numeral
	}
	for len(digits) <= s { // step 4: decimal point, trailing zeros preserved
		digits = "0" + digits
	}
	return digits[:len(digits)-s] + "." + digits[len(digits)-s:]
}

// inCanonicalRange reports whether |numericalValue| lies in [1E−6, 1E6], the range in
// which precisionDecimalCanonicalMap steps 3 and 4 use plain (non-scientific) forms.
// Called only for a nonzero numeric value with scale ≥ 0. With 10^adjusted ≤ |nV| <
// 10^(adjusted+1): |nV| ≥ 1E−6 ⟺ adjusted ≥ −6, and
// |nV| ≤ 1E6 ⟺ adjusted < 6, or adjusted = 6 with |nV| exactly 1E6 (a coefficient
// that is 1 followed only by zeros). No power of ten is materialised.
func (p precisionDecimalVal) inCanonicalRange() bool {
	adj := p.adjusted()
	if adj.Cmp(big.NewInt(-6)) < 0 {
		return false
	}
	switch adj.Cmp(big.NewInt(6)) {
	case -1:
		return true
	case 0:
		return strings.TrimRight(p.coefficient.String(), "0") == "1"
	}
	return false
}

// scientificCanonical renders precisionDecimalCanonicalMap step 5 for a nonzero
// magnitude: strip the coefficient's trailing zeros to the minimal significand C'
// (a factor of 10^k), form the one-leading-digit mantissa m with exponent
// exp = (len(C')−1) + k − aP, the adjusted exponent, then append aP + exp − f
// trailing zeros (f = m's fractional-digit count) so the printed scale matches
// ·scale·. That pad is (len(C')−1) + k − f = len(digits) − 1 − f, free of aP, so
// it is bounded by the literal's length. It would be −1 exactly when digits is
// one digit: scientificCanonicalMap's forced mantissa "D.0" then carries one
// more fractional digit than ·scale· admits, and step 5's "f will invariably be
// less than or equal to aP+p" fails. That mantissa drops its ".0" (f = 0, an m
// with no decimal point, as step 5's t contemplates), so 3E2 renders "3E2" and
// re-parses Identical. digits is the coefficient magnitude string, which has no
// leading zero. The sign is prepended by the caller.
func (p precisionDecimalVal) scientificCanonical(digits string) string {
	exp := "E" + p.adjusted().String()
	if len(digits) == 1 {
		return digits + exp
	}
	cPrime := strings.TrimRight(digits, "0")

	mantissa, frac := cPrime+".0", 1
	if len(cPrime) > 1 {
		mantissa, frac = cPrime[:1]+"."+cPrime[1:], len(cPrime)-1
	}
	return mantissa + strings.Repeat("0", len(digits)-1-frac) + exp
}
