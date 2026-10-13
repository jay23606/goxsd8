package validate

import (
	"strings"
	"testing"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The tests below pin that every content decline cvccomplexcontent.go makes is
// RECORDED as an [Unevaluated] and not only logged ([contentCheck.decline]): a
// decline that only logged would leave an empty Result reading as a walk that
// performed every check it reached. The undecided type is xs:decimal under
// gapBackend, a backend that does not map it, so String Valid over it is
// withheld rather than decided. xs:anySimpleType is not one: Datatype Valid
// holds for every literal against a ·special· datatype (Datatypes §4.1.4), so
// the walk decides it though no backend maps it (#1788).

// typeGap is the strict backend with one type unmapped: value.ValidateLexical
// then reports a fault of the TYPE for it and for every type derived from it,
// which is the decline the tests record. The schema is seeded under the strict
// backend, since builtin.Seed maps every primitive; only the assessment reads
// this one.
type typeGap struct {
	value.Backend
	gap xsd.QName
}

// gapBackend is the strict backend with gap unmapped.
func gapBackend(gap xsd.QName) value.Backend { return typeGap{Backend: testBackend(), gap: gap} }

func (b typeGap) Mapping(typ xsd.QName) (value.Mapping, bool) {
	if typ == b.gap {
		return value.Mapping{}, false
	}
	return b.Backend.Mapping(typ)
}

// assessRecorded assesses root against schema and returns what the walk charged
// and what it recorded as unevaluated.
func assessRecorded(t *testing.T, schema *xsd.Schema, root Element) ([]*xsderr.Error, []Unevaluated) {
	t.Helper()
	return assessRecordedWith(t, testBackend(), schema, root)
}

// assessRecordedWith is assessRecorded under backend.
func assessRecordedWith(t *testing.T, backend value.Backend, schema *xsd.Schema, root Element) ([]*xsderr.Error, []Unevaluated) {
	t.Helper()
	v, err := New(schema, backend)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res := v.Assess(root)
	if res.Err() != nil {
		t.Fatalf("Err() = %v, want nil", res.Err())
	}
	return res.Violations(), res.Unevaluated()
}

// wantDeclines fails unless got holds exactly one record per want entry, in
// order, each carrying the entry's rule at its Loc and naming its clause.
func wantDeclines(t *testing.T, got []Unevaluated, want ...Unevaluated) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("Unevaluated() = %v, want %d record(s)", messages(got), len(want))
	}
	for i, w := range want {
		if got[i].Rule() != w.rule {
			t.Errorf("Unevaluated()[%d].Rule() = %q, want %q", i, got[i].Rule(), w.rule)
		}
		if got[i].Loc() != w.loc {
			t.Errorf("Unevaluated()[%d].Loc() = %s, want %s", i, got[i].Loc(), w.loc)
		}
		if !strings.Contains(got[i].Msg(), w.msg) {
			t.Errorf("Unevaluated()[%d].Msg() = %q, want it to name %q", i, got[i].Msg(), w.msg)
		}
	}
}

// cvc-type clause 3.1.3 withheld on a SIMPLE ·governing type definition· is
// recorded at the element, under cvc-type and not cvc-simple-type or
// cvc-datatype-valid: the record names the rule the element would have been
// charged under. The xs:integer control shows the record is the decline's and
// not every simple-typed element's.
func TestWithheldSimpleTypeValueIsRecorded(t *testing.T) {
	schema := simpleTypedSchema(t, icBuiltin("decimal"), nil, false)
	got, undecided := assessRecordedWith(t, gapBackend(icBuiltin("decimal")), schema, cRoot("#1.5"))
	wantSilence(t, got, "a withheld String Valid verdict charges nothing")
	wantDeclines(t, undecided, Unevaluated{rule: ruleCvcType, loc: loc(1, 1), msg: "cvc-type clause 3.1.3"})

	got, undecided = assessRecorded(t, schema, cRoot("#1.5"))
	wantSilence(t, got, "1.5 is an xs:decimal")
	wantDeclines(t, undecided)
}

// A ·special· ·governing type definition· is DECIDED: Datatype Valid holds for
// every literal against it (Datatypes §4.1.4, cvc-datatype-valid), so cvc-type
// clause 3.1.3 is satisfied with neither a charge nor a record, though no
// backend maps either type.
func TestSpecialSimpleTypeValueIsDecided(t *testing.T) {
	for _, typ := range []string{"anySimpleType", "anyAtomicType"} {
		got, undecided := assessRecorded(t, simpleTypedSchema(t, icBuiltin(typ), nil, false), cRoot("#x"))
		wantSilence(t, got, "Datatype Valid holds for every literal against xs:"+typ)
		wantDeclines(t, undecided)
	}
}

// cvc-complex-type clause 1.2's ·initial value· half, withheld over a simple
// {content type}, is recorded at the CONTAINING element.
func TestWithheldSimpleContentValueIsRecorded(t *testing.T) {
	got, undecided := assessRecordedWith(t, gapBackend(icBuiltin("decimal")), simpleContentSchema(t, icBuiltin("decimal")), cRoot("#1.5"))
	wantSilence(t, got, "a withheld String Valid verdict charges nothing")
	wantDeclines(t, undecided, Unevaluated{rule: ruleCvcComplexType, loc: loc(1, 1), msg: "cvc-complex-type clause 1.2"})
}

// An undecided comparison against a fixed {value constraint} (cvc-elt clause
// 5.2.2.2.2) is recorded after the cvc-type clause 3.1.3 record the same
// undecided type makes: a decline sets no charge, so it silences no later
// clause, and each withheld clause is its own record. Over xs:anySimpleType,
// under a backend that leaves xs:string unmapped, a comparison of DIFFERING
// literals alone is undecided — one member of its mapping union cannot answer
// (Datatypes §3.2.1.2) — while clause 3.1.3 is decided.
func TestUndecidedFixedValueComparisonIsRecorded(t *testing.T) {
	fixed := xsd.NewValueConstraint(xsd.ValueFixed, "1.5", nil, nil)
	got, undecided := assessRecordedWith(t, gapBackend(icBuiltin("decimal")),
		simpleTypedSchema(t, icBuiltin("decimal"), &fixed, false), cRoot("#1.5"))
	wantSilence(t, got, "an undecided comparison charges nothing")
	wantDeclines(t, undecided,
		Unevaluated{rule: ruleCvcType, loc: loc(1, 1), msg: "cvc-type clause 3.1.3"},
		Unevaluated{rule: ruleCvcElt, loc: loc(1, 1), msg: "cvc-elt clause 5.2.2.2.2"})

	fixed = xsd.NewValueConstraint(xsd.ValueFixed, "37", nil, nil)
	got, undecided = assessRecordedWith(t, gapBackend(icBuiltin("string")),
		simpleTypedSchema(t, icBuiltin("anySimpleType"), &fixed, false), cRoot("#36"))
	wantSilence(t, got, "an undecided comparison charges nothing")
	wantDeclines(t, undecided, Unevaluated{rule: ruleCvcElt, loc: loc(1, 1), msg: "cvc-elt clause 5.2.2.2.2"})
}

// Over a ·special· ·governing type definition· cvc-elt clause 5.2.2.2.2 is
// DECIDED satisfied where the ·initial value· and the fixed {lexical form} are
// byte-identical: one literal maps identically on both sides, however many
// values it may denote (Datatypes §3.2.1.2, §3.2.2.2; #2029).
func TestSpecialFixedValueAgreesOnIdenticalLiterals(t *testing.T) {
	fixed := xsd.NewValueConstraint(xsd.ValueFixed, "x", nil, nil)
	for _, typ := range []string{"anySimpleType", "anyAtomicType"} {
		got, undecided := assessRecorded(t, simpleTypedSchema(t, icBuiltin(typ), &fixed, false), cRoot("#x"))
		wantSilence(t, got, "identical literals agree against xs:"+typ)
		wantDeclines(t, undecided)
	}
}

// Over a ·special· ·governing type definition· DIFFERING literals are decided
// over the type's mapping union (Datatypes §3.2.1.2, §3.2.2.2; #2040): "1.0"
// satisfies a fixed "1", both being one xs:decimal, and "36" against a fixed
// "37" is charged under cvc-elt clause 5.2.2.2.2.
func TestSpecialFixedValueIsDecidedOverTheMappingUnion(t *testing.T) {
	one := xsd.NewValueConstraint(xsd.ValueFixed, "1", nil, nil)
	for _, typ := range []string{"anySimpleType", "anyAtomicType"} {
		got, undecided := assessRecorded(t, simpleTypedSchema(t, icBuiltin(typ), &one, false), cRoot("#1.0"))
		wantSilence(t, got, `"1.0" and "1" are one xs:decimal against xs:`+typ)
		wantDeclines(t, undecided)
	}

	fixed := xsd.NewValueConstraint(xsd.ValueFixed, "37", nil, nil)
	got, _ := assessRecorded(t, simpleTypedSchema(t, icBuiltin("anySimpleType"), &fixed, false), cRoot("#36"))
	if len(got) != 1 || got[0].Rule != ruleCvcElt ||
		!strings.HasPrefix(got[0].Msg, `the ·actual value· of the element root is neither equal nor identical to the {value} of the fixed {value constraint} "37"`) {
		t.Fatalf("Violations() = %v, want one cvc-elt clause 5.2.2.2.2 charge against root and the fixed \"37\"", got)
	}
}

// declinedContent is a {content type} xsd.Schema.ContentMatcher declines: two
// repeating sibling groups, over a and c, around a b, past the matcher's
// region ceiling (xsd.maxPartitionStates), the shape its GAP(xsd) decline
// names.
func declinedContent(t *testing.T) xsd.ContentType {
	t.Helper()
	group := func(max int, leaf string) xsd.Particle {
		t.Helper()
		unbounded, err := xsd.NewUnboundedOccurs(xsderr.Loc{}, 1)
		if err != nil {
			t.Fatalf("NewUnboundedOccurs: %v", err)
		}
		p, err := xsd.NewParticle(xsderr.Loc{}, unbounded, xsd.ResolvedTerm{Term: cLocal(t, leaf)})
		if err != nil {
			t.Fatalf("NewParticle: %v", err)
		}
		g, err := xsd.NewModelGroup(xsderr.Loc{}, xsd.CompositorSequence, []xsd.Particle{p})
		if err != nil {
			t.Fatalf("NewModelGroup: %v", err)
		}
		o, err := xsd.NewOccurs(xsderr.Loc{}, 1, max)
		if err != nil {
			t.Fatalf("NewOccurs: %v", err)
		}
		gp, err := xsd.NewParticle(xsderr.Loc{}, o, xsd.ResolvedTerm{Term: g})
		if err != nil {
			t.Fatalf("NewParticle: %v", err)
		}
		return gp
	}
	return cSequence(t, false, group(1000, "a"), cParticle(t, "b", 1, 1), group(1000, "c"))
}

// A {content type} xsd.Schema.ContentMatcher declines leaves each element
// [[child]] unmatched, and each is recorded under cvc-complex-content at the
// CHILD's own location — where a charge against it would have sat.
func TestUndecidedContentModelIsRecordedPerChild(t *testing.T) {
	schema := cSchema(t, declinedContent(t))

	got, undecided := assessRecorded(t, schema, cRoot("a", "b"))
	wantSilence(t, got, "an unmatched child charges nothing")
	wantDeclines(t, undecided,
		Unevaluated{rule: ruleCvcComplexContent, loc: loc(2, 1), msg: "cvc-complex-content clause 1"},
		Unevaluated{rule: ruleCvcComplexContent, loc: loc(3, 1), msg: "cvc-complex-content clause 1"})
}

// The same {content type} over an element with no element [[child]] leaves
// the EMPTY sequence unmatched, and that is recorded once, at the element's
// own location: declinedContent's b has {min occurs} 1, so the sequence is not
// ·valid·, and an empty Result there would read as a valid document.
// White-space text, which clause 1.3 admits, changes nothing.
func TestUndecidedContentModelIsRecordedForAnEmptySequence(t *testing.T) {
	schema := cSchema(t, declinedContent(t))
	for _, tc := range []struct {
		name string
		root Element
	}{
		{"no children", cRoot()},
		{"white space only", cRoot("# ")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, undecided := assessRecorded(t, schema, tc.root)
			wantSilence(t, got, "an unmatched empty sequence charges nothing")
			wantDeclines(t, undecided,
				Unevaluated{rule: ruleCvcComplexContent, loc: loc(1, 1), msg: "the element root has no element information item"})
		})
	}
}
