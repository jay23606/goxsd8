package value

import (
	"errors"
	"testing"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// TestFacetStageMessagesWhole pins, by equality, the whole Msg of one error from
// every facet-stage site #2412 rewrote to STYLE E5: the offending item first,
// then the rule its site charges as a clause of the same sentence. A
// facetPrecondition or assertionDeclined Msg carries none of its sentinel's
// text, though the sentinel still rides the chain: each row also pins the three
// classification predicates, which read the chain alone.
func TestFacetStageMessagesWhole(t *testing.T) {
	q := func(local string) xsd.QName { return xsd.QName{Space: "urn:t", Local: local} }
	collapse := xsd.NewFacet(xsd.FacetWhiteSpace, []string{"collapse"}, true)
	prim := func(local string, facets ...xsd.Facet) *xsd.SimpleType {
		t.Helper()
		st, err := xsd.NewPrimitiveType(xsderr.Loc{}, q(local), append([]xsd.Facet{collapse}, facets...), nil)
		if err != nil {
			t.Fatalf("NewPrimitiveType(%s): %v", local, err)
		}
		return st
	}
	validate := func(b Backend, st *xsd.SimpleType, a AssertionEvaluator) error {
		_, err := ValidateLexical(b, noSchema{}, st, "1", nil, a)
		return err
	}
	second := func(_ any, err error) error { return err }

	unordered := prim("unordered", xsd.NewFacet(xsd.FacetMaxInclusive, []string{"5"}, false))
	unmapped := prim("unmapped")
	enumerated := prim("enumerated", xsd.NewEnumerationFacet([]xsd.EnumerationMember{xsd.NewEnumerationMember("1", nil, nil)}))
	bounded := prim("bounded", xsd.NewFacet(xsd.FacetMinInclusive, []string{"0"}, false))
	twoBounds := prim("twoBounds", xsd.NewFacet(xsd.FacetMaxInclusive, []string{"1", "2"}, false))
	// An heir is mapped while the ancestor whose facet it inherits is not, so
	// the fault names both.
	heir := func(local string, base *xsd.SimpleType) *xsd.SimpleType {
		t.Helper()
		st, err := newCheckedSimpleType(xsderr.Loc{}, q(local), xsd.RestrictionDerivation{}, base, nil, nil)
		if err != nil {
			t.Fatalf("NewSimpleType(%s): %v", local, err)
		}
		return st
	}
	enumHeir := heir("enumHeir", enumerated)
	boundHeir := heir("boundHeir", bounded)
	bare, err := xsd.NewPrimitiveType(xsderr.Loc{}, q("bare"), nil, nil)
	if err != nil {
		t.Fatalf("NewPrimitiveType(bare): %v", err)
	}
	num := primType(t, "numeric", "collapse")
	union, err := newCheckedSimpleType(xsderr.Loc{}, q("u"), unionOf(unmapped), xsd.AnySimpleType(), nil, nil)
	if err != nil {
		t.Fatalf("NewSimpleType(union): %v", err)
	}
	numUnion, err := newCheckedSimpleType(xsderr.Loc{}, q("nu"), unionOf(num), xsd.AnySimpleType(), nil, nil)
	if err != nil {
		t.Fatalf("NewSimpleType(union of numeric): %v", err)
	}
	bound := boundFacet{limit: intValue(1), lexical: "+1", kind: xsd.FacetMaxInclusive}
	length := lengthFacet{limit: "2", kind: xsd.FacetLength}
	digits := digitsFacet{limit: "3", kind: xsd.FacetTotalDigits}
	scale := scaleFacet{limit: "-2", kind: xsd.FacetMaxScale}
	timezone := explicitTimezoneFacet{requirement: tzProhibited}
	twoValues := func(kind xsd.FacetKind) xsd.Facet { return xsd.NewFacet(kind, []string{"1", "2"}, false) }
	oneValue := func(kind xsd.FacetKind, v string) xsd.Facet { return xsd.NewFacet(kind, []string{v}, false) }
	intBase, intB := restrictionBase(t, twoValues(xsd.FacetMaxInclusive))

	const bareCandidate = "no capabilities"
	for _, tc := range []struct {
		name                           string
		err                            error
		rule                           xsderr.Rule
		precondition, verdict, decline bool
		msg                            string
	}{
		// facetPrecondition: the five CheckValue sites open with the facet and the
		// {value} the checker holds, newBoundFacet with the facet, its {value} and
		// the type, effectiveWhiteSpace with the type.
		{"bound facet, incapable candidate", bound.CheckValue(bareCandidate), ruleCosApplicableFacets, true, false, false,
			`the maxInclusive facet, whose {value} is "+1", is not applicable to the type of a candidate that is not Ordered (string), but cos-applicable-facets allows in {facets} only the facets applicable to the type`},
		{"length facet, incapable candidate", length.CheckValue(bareCandidate), ruleCosApplicableFacets, true, false, false,
			`the length facet, whose {value} is "2", is not applicable to the type of a candidate that is not Lengthed (string), but cos-applicable-facets allows in {facets} only the facets applicable to the type`},
		{"digits facet, incapable candidate", digits.CheckValue(bareCandidate), ruleCosApplicableFacets, true, false, false,
			`the totalDigits facet, whose {value} is "3", is not applicable to the type of a candidate that is not DigitCounted (string), but cos-applicable-facets allows in {facets} only the facets applicable to the type`},
		{"scale facet, incapable candidate", scale.CheckValue(bareCandidate), ruleCosApplicableFacets, true, false, false,
			`the maxScale facet, whose {value} is "-2", is not applicable to the type of a candidate that is not Scaled (string), but cos-applicable-facets allows in {facets} only the facets applicable to the type`},
		{"explicitTimezone facet, incapable candidate", timezone.CheckValue(bareCandidate), ruleCosApplicableFacets, true, false, false,
			`the explicitTimezone facet, whose {value} is "prohibited", is not applicable to the type of a candidate that is not TimezoneAware (string), but cos-applicable-facets allows in {facets} only the facets applicable to the type`},
		{"bound facet, unordered {value}", validate(plainBackend{unordered.Name(): true}, unordered, assertionsUndecided{}), ruleCosApplicableFacets, true, false, false,
			`the maxInclusive facet of the simple type {urn:t}unordered, whose {value} "5" parses to a string that is not Ordered, is not applicable to that type, but cos-applicable-facets allows in {facets} only the facets applicable to the type`},
		{"no whiteSpace mode in force", second(effectiveWhiteSpace(noSchema{}, bare)), xsderr.RuleComponentInvariant, true, false, false,
			`the simple type {urn:t}bare has no whiteSpace {value} in force that is exactly one of preserve/replace/collapse`},

		// assertionDeclined opens with the assertion.
		{"declined assertion", validate(memberBackend{num.Name(): allDigits}, asserting(t, "asserted", num, "hold", "decline"), scripted(nil)), ruleCvcAssertionsValid, false, false, true,
			`assertion 2 of 2 in the {value} of the assertions facet of the simple type {urn:test}asserted, whose {test} is "decline", was not evaluated, so it is undecided whether the value is facet-valid with respect to it, which cvc-assertions-valid requires`},

		// The census sites: an unmapped type, a {value} of the wrong count, a
		// {value} outside its space, and the active-basic-member scan.
		{"atomic type with no backend mapping", validate(plainBackend{}, unmapped, assertionsUndecided{}), ruleCvcDatatypeValid, false, false, false,
			`the simple type {urn:t}unmapped has no governing backend mapping, so the backend cannot decide cvc-datatype-valid for it`},
		{"union with no backend mapping", validate(plainBackend{}, union, assertionsUndecided{}), ruleCvcDatatypeValid, false, false, false,
			`the simple type {urn:t}u has no governing backend mapping, so the backend cannot decide cvc-datatype-valid for it`},
		{"enumeration declared on an unmapped type", validate(plainBackend{}, enumerated, assertionsUndecided{}), ruleCvcEnumerationValid, false, false, false,
			`the simple type {urn:t}enumerated, which declares the enumeration facet, has no governing backend mapping, so cvc-enumeration-valid cannot be decided against the facet's {value}`},
		{"bound declared on an unmapped type", validate(plainBackend{}, bounded, assertionsUndecided{}), ruleCvcMinInclusiveValid, false, false, false,
			`the simple type {urn:t}bounded, which declares the minInclusive facet, has no governing backend mapping, so cvc-minInclusive-valid cannot be decided against the facet's {value}`},
		{"enumeration inherited from an unmapped type", validate(plainBackend{enumHeir.Name(): true}, enumHeir, assertionsUndecided{}), ruleCvcEnumerationValid, false, false, false,
			`the simple type {urn:t}enumerated, which declares the enumeration facet that the simple type {urn:t}enumHeir inherits, has no governing backend mapping, so cvc-enumeration-valid cannot be decided against the facet's {value}`},
		{"bound inherited from an unmapped type", validate(plainBackend{boundHeir.Name(): true}, boundHeir, assertionsUndecided{}), ruleCvcMinInclusiveValid, false, false, false,
			`the simple type {urn:t}bounded, which declares the minInclusive facet that the simple type {urn:t}boundHeir inherits, has no governing backend mapping, so cvc-minInclusive-valid cannot be decided against the facet's {value}`},
		{"bound facet with two values", validate(plainBackend{twoBounds.Name(): true}, twoBounds, assertionsUndecided{}), ruleCvcMaxInclusiveValid, false, false, false,
			`the maxInclusive facet of the simple type {urn:t}twoBounds carries 2 values rather than one, so cvc-maxInclusive-valid cannot be decided against its {value}`},
		{"explicitTimezone facet with two values", second(newExplicitTimezoneFacet(twoValues(xsd.FacetExplicitTimezone))), ruleCvcExplicitTimezoneValid, false, true, false,
			`the explicitTimezone facet carries 2 values rather than one, so cvc-explicitTimezone-valid cannot be decided against its {value}`},
		{"explicitTimezone facet with an unknown value", second(newExplicitTimezoneFacet(oneValue(xsd.FacetExplicitTimezone, "maybe"))), ruleCvcExplicitTimezoneValid, false, true, false,
			`the explicitTimezone facet's {value} "maybe" is not required, prohibited or optional, the only three values cvc-explicitTimezone-valid clauses 1 to 3 decide`},
		{"scale facet with two values", second(newScaleFacet(twoValues(xsd.FacetMinScale))), ruleCvcMinScaleValid, false, true, false,
			`the minScale facet carries 2 values rather than one, so cvc-minScale-valid cannot be decided against its {value}`},
		{"scale facet with a non-integer value", second(newScaleFacet(oneValue(xsd.FacetMaxScale, "1.5"))), ruleCvcMaxScaleValid, false, true, false,
			`the maxScale facet's {value} "1.5" is not an integer, so cvc-maxScale-valid cannot be decided against it`},
		{"digits facet with two values", second(newDigitsFacet(twoValues(xsd.FacetFractionDigits))), ruleCvcFractionDigitsValid, false, true, false,
			`the fractionDigits facet carries 2 values rather than one, so cvc-fractionDigits-valid cannot be decided against its {value}`},
		{"digits facet with a negative value", second(newDigitsFacet(oneValue(xsd.FacetTotalDigits, "-1"))), ruleCvcTotalDigitsValid, false, true, false,
			`the totalDigits facet's {value} "-1" is not a nonNegativeInteger, so cvc-totalDigits-valid cannot be decided against it`},
		{"base bound facet with two values", restrict(t, intB, intBase, oneValue(xsd.FacetMaxInclusive, "0")), ruleMaxInclusiveValidRestriction, false, true, false,
			`the maxInclusive facet carries 2 values rather than one, so maxInclusive-valid-restriction cannot be decided for it`},
		{"literal no member identifies", second(activeBasicMember(memberBackend{num.Name(): allDigits}, noSchema{}, numUnion, "x", nil, assertionsUndecided{})), ruleCvcDatatypeValid, false, false, false,
			`the literal "x" is Datatype Valid against none of the union's 1 {member type definitions}, though validateUnion already accepted it, so it has no ·active basic member·, which cvc-datatype-valid clause 2.3 requires`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var xe *xsderr.Error
			if !errors.As(tc.err, &xe) || xe.Rule != tc.rule {
				t.Fatalf("err = %v, want an *xsderr.Error charging %s", tc.err, tc.rule)
			}
			if xe.Msg != tc.msg {
				t.Errorf("Msg =\n%s\nwant\n%s", xe.Msg, tc.msg)
			}
			if got := IsFacetPrecondition(tc.err); got != tc.precondition {
				t.Errorf("IsFacetPrecondition = %v, want %v", got, tc.precondition)
			}
			if got := IsDatatypeVerdict(tc.err); got != tc.verdict {
				t.Errorf("IsDatatypeVerdict = %v, want %v", got, tc.verdict)
			}
			if got := IsAssertionDeclined(tc.err); got != tc.decline {
				t.Errorf("IsAssertionDeclined = %v, want %v", got, tc.decline)
			}
		})
	}
}
