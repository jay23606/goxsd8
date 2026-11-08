package xsd

import (
	"strconv"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsderr"
)

// facetBase builds a base type carrying ownFacets, rooted on a fresh primitive
// so each case is independent.
func facetBase(t *testing.T, ownFacets ...Facet) *SimpleType {
	t.Helper()
	prim := mustPrim(t, "string")
	base, err := newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "base"},
		RestrictionDerivation{}, prim, ownFacets, nil)
	if err != nil {
		t.Fatalf("build base: %v", err)
	}
	return base
}

// deriveFacets restricts base with ownFacets, returning the construction error.
func deriveFacets(base *SimpleType, ownFacets ...Facet) error {
	_, err := newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "derived"},
		RestrictionDerivation{}, base, ownFacets, nil)
	return err
}

// wantRule asserts err is an *xsderr.Error charging rule.
func wantRule(t *testing.T, err error, rule xsderr.Rule) {
	t.Helper()
	if err == nil {
		t.Fatalf("want rejection charging %s, got nil", rule)
	}
	got, ok := xsderr.RuleOf(err)
	if !ok || got != rule {
		t.Fatalf("rejection charges %q (ok=%v), want %q; err=%v", got, ok, rule, err)
	}
}

// TestCountFacetValidRestriction covers all five count-valued valid-restriction
// SCCs in both polarities: the narrowing direction each rule permits must
// construct, and the widening direction must be rejected under that rule's own
// ID. length is the odd one out — §4.3.1.4 makes it an equality, so BOTH
// directions are violations.
func TestCountFacetValidRestriction(t *testing.T) {
	cases := []struct {
		name     string
		kind     FacetKind
		baseVal  string
		ownVal   string
		wantRule xsderr.Rule // empty means the restriction must be accepted
	}{
		{"length equal accepted", FacetLength, "5", "5", ""},
		{"length narrowed rejected", FacetLength, "5", "4", ruleLengthValidRestriction},
		{"length widened rejected", FacetLength, "5", "6", ruleLengthValidRestriction},
		{"minLength raised accepted", FacetMinLength, "2", "4", ""},
		{"minLength equal accepted", FacetMinLength, "2", "2", ""},
		{"minLength lowered rejected", FacetMinLength, "2", "1", ruleMinLengthValidRestriction},
		{"maxLength lowered accepted", FacetMaxLength, "9", "4", ""},
		{"maxLength raised rejected", FacetMaxLength, "9", "10", ruleMaxLengthValidRestriction},
		{"totalDigits lowered accepted", FacetTotalDigits, "6", "3", ""},
		{"totalDigits raised rejected", FacetTotalDigits, "6", "7", ruleTotalDigitsValidRestriction},
		{"fractionDigits lowered accepted", FacetFractionDigits, "4", "0", ""},
		{"fractionDigits raised rejected", FacetFractionDigits, "4", "5", ruleFractionDigitsValidRestriction},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base := facetBase(t, NewFacet(c.kind, []string{c.baseVal}, false))
			err := deriveFacets(base, NewFacet(c.kind, []string{c.ownVal}, false))
			if c.wantRule == "" {
				if err != nil {
					t.Fatalf("restriction %s %s -> %s rejected: %v", c.kind, c.baseVal, c.ownVal, err)
				}
				return
			}
			wantRule(t, err, c.wantRule)
		})
	}
}

// TestCountFacetRestrictionVacuousWithoutBaseFacet proves the rules are keyed
// off the BASE's {facets}: a restriction introducing a count facet the base does
// not carry is unconstrained by them, in either direction.
func TestCountFacetRestrictionVacuousWithoutBaseFacet(t *testing.T) {
	base := facetBase(t)
	if err := deriveFacets(base, NewFacet(FacetMaxLength, []string{"100"}, false)); err != nil {
		t.Fatalf("introducing maxLength on a base without one: %v", err)
	}
	if err := deriveFacets(base, NewFacet(FacetLength, []string{"3"}, false)); err != nil {
		t.Fatalf("introducing length on a base without one: %v", err)
	}
}

// TestCountFacetRestrictionTransitive proves the comparison rides
// EffectiveFacets: a facet inherited unchanged through an intermediate level is
// still the operand two levels down, with no manual ancestor walk.
func TestCountFacetRestrictionTransitive(t *testing.T) {
	base := facetBase(t, NewFacet(FacetMaxLength, []string{"8"}, false))
	middle, err := newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "middle"},
		RestrictionDerivation{}, base, nil, nil)
	if err != nil {
		t.Fatalf("build middle: %v", err)
	}
	_, err = newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "leaf"},
		RestrictionDerivation{}, middle, []Facet{NewFacet(FacetMaxLength, []string{"9"}, false)}, nil)
	wantRule(t, err, ruleMaxLengthValidRestriction)
}

// TestWhiteSpaceValidRestriction encodes §4.3.6.4's two numbered error
// conditions and only those: the restrictiveness order is preserve < replace <
// collapse, so moving up or staying put is legal and moving down is not. The
// replace-over-preserve row is the one the spec Note's own (contradictory)
// "preserve, collapse, replace" listing would wrongly reject.
func TestWhiteSpaceValidRestriction(t *testing.T) {
	cases := []struct {
		baseVal  string
		ownVal   string
		rejected bool
	}{
		{"preserve", "preserve", false},
		{"preserve", "replace", false},
		{"preserve", "collapse", false},
		{"replace", "replace", false},
		{"replace", "collapse", false},
		{"replace", "preserve", true}, // §4.3.6.4 condition 2
		{"collapse", "collapse", false},
		{"collapse", "replace", true},  // §4.3.6.4 condition 1
		{"collapse", "preserve", true}, // §4.3.6.4 condition 1
	}
	for _, c := range cases {
		t.Run(c.baseVal+"->"+c.ownVal, func(t *testing.T) {
			base := facetBase(t, NewFacet(FacetWhiteSpace, []string{c.baseVal}, false))
			err := deriveFacets(base, NewFacet(FacetWhiteSpace, []string{c.ownVal}, false))
			if !c.rejected {
				if err != nil {
					t.Fatalf("whiteSpace %s -> %s rejected: %v", c.baseVal, c.ownVal, err)
				}
				return
			}
			wantRule(t, err, ruleWhiteSpaceValidRestriction)
		})
	}
}

// TestTimezoneValidRestriction covers §4.3.14.4: a base {value} of optional may
// be narrowed to anything, while required or prohibited must be repeated
// verbatim, and the check is vacuous on a {value} outside the domain.
func TestTimezoneValidRestriction(t *testing.T) {
	cases := []struct {
		baseVal  string
		ownVal   string
		rejected bool
	}{
		{"optional", "required", false},
		{"optional", "prohibited", false},
		{"optional", "optional", false},
		{"required", "required", false},
		{"required", "optional", true},
		{"required", "prohibited", true},
		{"prohibited", "required", true},
		// A {value} outside the three-token domain on either side is not this
		// SCC's to charge (#3495).
		{"bogus", "required", false},
		{"required", "bogus", false},
	}
	for _, c := range cases {
		t.Run(c.baseVal+"->"+c.ownVal, func(t *testing.T) {
			base := facetBase(t, NewFacet(FacetExplicitTimezone, []string{c.baseVal}, false))
			err := deriveFacets(base, NewFacet(FacetExplicitTimezone, []string{c.ownVal}, false))
			if !c.rejected {
				if err != nil {
					t.Fatalf("explicitTimezone %s -> %s rejected: %v", c.baseVal, c.ownVal, err)
				}
				return
			}
			wantRule(t, err, ruleTimezoneValidRestriction)
		})
	}
}

// TestCountFacetConsistency covers the same-type consistency SCCs, which are
// not restriction-specific: they constrain the type's OWN effective {facets},
// so they fire on a single type declaring both members of a pair.
func TestCountFacetConsistency(t *testing.T) {
	prim := mustPrim(t, "string")
	cases := []struct {
		name     string
		facets   []Facet
		wantRule xsderr.Rule
	}{
		{
			"minLength above maxLength",
			[]Facet{NewFacet(FacetMinLength, []string{"5"}, false), NewFacet(FacetMaxLength, []string{"3"}, false)},
			ruleMinLengthLEMaxLength,
		},
		{
			"minLength equal to maxLength accepted",
			[]Facet{NewFacet(FacetMinLength, []string{"3"}, false), NewFacet(FacetMaxLength, []string{"3"}, false)},
			"",
		},
		{
			"fractionDigits above totalDigits",
			[]Facet{NewFacet(FacetFractionDigits, []string{"4"}, false), NewFacet(FacetTotalDigits, []string{"2"}, false)},
			ruleFractionDigitsLETotalDigits,
		},
		{
			"minLength above length",
			[]Facet{NewFacet(FacetLength, []string{"4"}, false), NewFacet(FacetMinLength, []string{"5"}, false)},
			ruleLengthMinLengthMaxLength,
		},
		{
			"length above maxLength",
			[]Facet{NewFacet(FacetLength, []string{"6"}, false), NewFacet(FacetMaxLength, []string{"5"}, false)},
			ruleLengthMinLengthMaxLength,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "t"},
				RestrictionDerivation{}, prim, c.facets, nil)
			if c.wantRule == "" {
				if err != nil {
					t.Fatalf("consistent facets rejected: %v", err)
				}
				return
			}
			wantRule(t, err, c.wantRule)
		})
	}
}

// TestCountFacetValueMalformed proves a count facet's {value} is parsed as an
// xs:nonNegativeInteger and charged under that facet's own rule when it is not
// one — reached only where a comparison actually needs the number.
func TestCountFacetValueMalformed(t *testing.T) {
	base := facetBase(t, NewFacet(FacetMaxLength, []string{"8"}, false))
	err := deriveFacets(base, NewFacet(FacetMaxLength, []string{"-1"}, false))
	wantRule(t, err, ruleMaxLengthValidRestriction)
	if !strings.Contains(err.Error(), "nonNegativeInteger") {
		t.Errorf("message %q does not name the expected {value} space", err.Error())
	}
}

// TestListApplicableFacets covers the list applicable-facet check: the set is the
// §4.1.5 literal, so length/minLength/maxLength/pattern/enumeration/whiteSpace/
// assertions pass and anything else — here a bound facet — is rejected. The
// rejection is charged under the clause the §3.16.6.2 case split SELECTS, which
// differs between the two list branches: a CONSTRUCTED list (B is
// xs:anySimpleType) is in branch 2.2.1, so 2.2.1.2; a list RESTRICTING a real
// list is in branch 2.2.2, so 2.2.2.4.
//
// The applicable-facet POSITIVE case is therefore a RESTRICTING list: clause
// 2.2.1.2 admits nothing but whiteSpace = collapse fixed on a constructed one,
// so an applicable minLength can only ever appear at a second derivation step —
// the same two-step shape xs:NMTOKENS itself has (§3.4.5).
func TestListApplicableFacets(t *testing.T) {
	item := mustPrim(t, "string")
	constructed, err := newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "constructedlist"},
		listOf(item), anySimpleType, constructedListFacets(), nil)
	if err != nil {
		t.Fatalf("constructed list rejected: %v", err)
	}
	baseList, err := newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "goodlist"},
		listOf(item), constructed,
		[]Facet{NewFacet(FacetMinLength, []string{"1"}, false)}, nil)
	if err != nil {
		t.Fatalf("list with an applicable facet rejected: %v", err)
	}

	_, err = newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "badconstructedlist"},
		listOf(item), anySimpleType,
		[]Facet{NewFacet(FacetMaxInclusive, []string{"5"}, false)}, nil)
	wantRule(t, err, ruleCosSTRestricts)
	if !strings.Contains(err.Error(), "2.2.1.2") {
		t.Errorf("constructed-list message %q does not name clause 2.2.1.2", err.Error())
	}

	_, err = newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "badrestrictedlist"},
		listOf(item), baseList,
		[]Facet{NewFacet(FacetMaxInclusive, []string{"5"}, false)}, nil)
	wantRule(t, err, ruleCosSTRestricts)
	if !strings.Contains(err.Error(), "2.2.2.4") {
		t.Errorf("restricted-list message %q does not name clause 2.2.2.4", err.Error())
	}
}

// TestUnionApplicableFacets covers cos-st-restricts clause 3.2.2.4: only
// pattern, enumeration and assertions are applicable to a union, so a facet
// applicable to a LIST but not a union (whiteSpace) is still rejected. The
// clause lives in the RESTRICTED branch of the §3.16.6.2 case split, so both
// types here restrict a real union base; a union constructed straight from
// xs:anySimpleType is governed by clause 3.2.1.2 instead
// (TestUnionConstructedFacetsMustBeEmpty).
func TestUnionApplicableFacets(t *testing.T) {
	baseUnion := mustUnion(t, "baseunion")
	member := mustMembers(baseUnion)[0]
	_, err := newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "goodunion"},
		unionOf(member), baseUnion,
		[]Facet{NewFacet(FacetPattern, []string{"a+"}, false)}, nil)
	if err != nil {
		t.Fatalf("union with an applicable facet rejected: %v", err)
	}

	_, err = newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "badunion"},
		unionOf(member), baseUnion,
		[]Facet{NewFacet(FacetWhiteSpace, []string{"collapse"}, true)}, nil)
	wantRule(t, err, ruleCosSTRestricts)
	if !strings.Contains(err.Error(), "3.2.2.4") {
		t.Errorf("message %q does not name clause 3.2.2.4", err.Error())
	}
}

// mustUnion builds a freshly-constructed (B is xs:anySimpleType) union over one
// string member, carrying no facets — the shape clause 3.2.1.2 demands.
func mustUnion(t *testing.T, local string) *SimpleType {
	t.Helper()
	member := mustPrim(t, local+"member")
	u, err := newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: local},
		unionOf(member), anySimpleType, nil, nil)
	if err != nil {
		t.Fatalf("build union %s: %v", local, err)
	}
	return u
}

// TestUnionConstructedFacetsMustBeEmpty covers cos-st-restricts clause 3.2.1.2:
// a union whose {base type definition} is xs:anySimpleType is FRESHLY
// CONSTRUCTED, and its {facets} must be empty — not merely drawn from the union
// applicable set, so even a pattern (applicable to a union under §4.1.5) is
// rejected. Nothing inherits into such a type (xs:anySimpleType carries no
// facets), so {facets} is exactly the own facet set.
func TestUnionConstructedFacetsMustBeEmpty(t *testing.T) {
	member := mustPrim(t, "cufmember")
	if _, err := newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "emptyunion"},
		unionOf(member), anySimpleType, nil, nil); err != nil {
		t.Fatalf("facet-free constructed union rejected: %v", err)
	}

	_, err := newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "facetedunion"},
		unionOf(member), anySimpleType,
		[]Facet{NewFacet(FacetPattern, []string{"a+"}, false)}, nil)
	wantRule(t, err, ruleCosSTRestricts)
	if !strings.Contains(err.Error(), "3.2.1.2") {
		t.Errorf("message %q does not name clause 3.2.1.2", err.Error())
	}
}

// TestAtomicVarietyApplicableFacetsNotCharged pins the placement decision: the
// ATOMIC applicable-facet clause (1.3.1) is NOT charged in this package — it
// needs the generated per-primitive table — so an inapplicable-looking facet on
// an atomic type still constructs here and is rejected at finalize instead, by
// the installed SimpleTypeRestrictionChecker.
func TestAtomicVarietyApplicableFacetsNotCharged(t *testing.T) {
	prim := mustPrim(t, "string")
	if _, err := newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "atomic"},
		RestrictionDerivation{}, prim,
		[]Facet{NewFacet(FacetMaxInclusive, []string{"5"}, false)}, nil); err != nil {
		t.Fatalf("atomic applicability must not be charged in package xsd, got: %v", err)
	}
}

// lengthChain builds a base chain of restriction steps over a fresh string
// primitive — one SimpleType per element of steps, each carrying that step's own
// facets — and returns the construction error of the first step that is
// rejected, or nil when the whole chain constructs. It exists because
// length-minLength-maxLength clauses 1.2/2.2 are a derivation-HISTORY predicate:
// unlike the same-{facets} clauses 1.1/2.1, they cannot be exercised on one type.
func lengthChain(t *testing.T, steps ...[]Facet) error {
	t.Helper()
	cur := mustPrim(t, "string")
	for i, own := range steps {
		next, err := newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "step" + strconv.Itoa(i)},
			RestrictionDerivation{}, cur, own, nil)
		if err != nil {
			return err
		}
		cur = next
	}
	return nil
}

// TestLengthDerivationHistory covers length-minLength-maxLength clauses 1.2 and
// 2.2: length and a coexisting minLength/maxLength may not be specified together
// at one derivation step. Every case here satisfies clauses 1.1/2.1 (the value
// ordering), so a rejection can only come from the derivation-history half —
// checkLengthCoexistence would accept all of them.
func TestLengthDerivationHistory(t *testing.T) {
	length := func(v string) Facet { return NewFacet(FacetLength, []string{v}, false) }
	minLen := func(v string) Facet { return NewFacet(FacetMinLength, []string{v}, false) }
	maxLen := func(v string) Facet { return NewFacet(FacetMaxLength, []string{v}, false) }

	cases := []struct {
		name       string
		steps      [][]Facet
		wantClause string // empty means the chain must construct
	}{
		{
			"length and minLength at one step",
			[][]Facet{{length("4"), minLen("1")}},
			"clause 1.2",
		},
		{
			"length and maxLength at one step",
			[][]Facet{{length("4"), maxLen("10")}},
			"clause 2.2",
		},
		{
			"minLength inherited at the same value, length added below",
			[][]Facet{{minLen("4")}, {length("4")}},
			"",
		},
		{
			"maxLength inherited at the same value, length added below",
			[][]Facet{{maxLen("8")}, {length("4")}},
			"",
		},
		{
			"minLength raised at the step that adds length",
			[][]Facet{{minLen("1")}, {minLen("3"), length("5")}},
			"clause 1.2",
		},
		{
			"maxLength lowered at the step that adds length",
			[][]Facet{{maxLen("9")}, {maxLen("6"), length("5")}},
			"clause 2.2",
		},
		{
			"both inherited two steps up, length added at the leaf",
			[][]Facet{{minLen("2")}, {maxLen("9")}, {length("4")}},
			"",
		},
		{
			"maxLength added below the step that introduced length",
			[][]Facet{{length("5")}, {maxLen("5")}},
			"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := lengthChain(t, c.steps...)
			if c.wantClause == "" {
				if err != nil {
					t.Fatalf("chain rejected: %v", err)
				}
				return
			}
			wantRule(t, err, ruleLengthMinLengthMaxLength)
			if !strings.Contains(err.Error(), c.wantClause) {
				t.Errorf("message %q does not cite %s", err.Error(), c.wantClause)
			}
		})
	}
}

// TestSameStepBoundPairs covers maxInclusive-maxExclusive (§4.3.8.4) and
// minInclusive-minExclusive (§4.3.9.4): both bounds of one side specified at ONE
// derivation step are rejected under that side's rule, and the same two kinds
// split across steps are accepted. The split rows are the negative control for
// reading the overlaid {facets} instead of the step's own facets: every leaf
// there has both kinds of the pair in its EffectiveFacets, so a check reading
// them rejects it. builtin's TestSameStepBoundPairsFullCheck runs the same
// shapes, with value-consistent {value}s, through the whole restriction checker.
func TestSameStepBoundPairs(t *testing.T) {
	f := func(kind FacetKind, v string) Facet { return NewFacet(kind, []string{v}, false) }
	cases := []struct {
		name     string
		steps    [][]Facet // own facets per derivation step, outermost base first
		wantRule xsderr.Rule
	}{
		{"maxInclusive and maxExclusive at one step",
			[][]Facet{{f(FacetMaxInclusive, "5"), f(FacetMaxExclusive, "10")}}, ruleMaxInclusiveMaxExclusive},
		{"minInclusive and minExclusive at one step",
			[][]Facet{{f(FacetMinInclusive, "5"), f(FacetMinExclusive, "1")}}, ruleMinInclusiveMinExclusive},
		{"inherited maxInclusive, own maxExclusive",
			[][]Facet{{f(FacetMaxInclusive, "10")}, {f(FacetMaxExclusive, "5")}}, ""},
		{"inherited maxExclusive, own maxInclusive",
			[][]Facet{{f(FacetMaxExclusive, "10")}, {f(FacetMaxInclusive, "5")}}, ""},
		{"inherited minInclusive, own minExclusive",
			[][]Facet{{f(FacetMinInclusive, "1")}, {f(FacetMinExclusive, "5")}}, ""},
		{"inherited minExclusive, own minInclusive",
			[][]Facet{{f(FacetMinExclusive, "1")}, {f(FacetMinInclusive, "5")}}, ""},
		{"both inherited from two ancestor steps",
			[][]Facet{{f(FacetMaxInclusive, "10")}, {f(FacetMaxExclusive, "5")}, nil}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := mustPrim(t, "decimal")
			var err error
			for i, own := range c.steps {
				st, err = newCheckedSimpleType(xsderr.Loc{}, QName{Space: "urn:test", Local: "step" + strconv.Itoa(i)},
					RestrictionDerivation{}, st, own, nil)
				if err != nil && i < len(c.steps)-1 {
					t.Fatalf("ancestor step %d rejected: %v", i, err)
				}
			}
			if c.wantRule == "" {
				if err != nil {
					t.Fatalf("pair split across derivation steps rejected: %v", err)
				}
				return
			}
			wantRule(t, err, c.wantRule)
			if !strings.Contains(err.Error(), "same derivation step") {
				t.Errorf("message %q does not name the same-step condition", err.Error())
			}
		})
	}
}
