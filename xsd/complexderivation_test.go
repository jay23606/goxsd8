package xsd

import (
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsderr"
)

// These tests are package-internal: checkComplexDerivations runs inside
// SchemaBuilder.Finalize and is unexported (STYLE T5), so the assertions are made
// on the error Finalize returns. The component builders come from
// particleattribution_test.go and effectivetotalrange_test.go — one set of
// helpers, not three (STYLE T4).

// dAnyType is the ur-type xs:anyType (§3.4.7) as the producer seeds it: a mixed
// complex type over a lax ##any element wildcard, with a lax ##any attribute
// wildcard, and itself as its {base type definition}. Phase D needs it present
// whenever a type under test names it as a base, and its presence is also what
// makes derivation-ok-restriction clause 2.1 reachable in these tests.
func dAnyType(t *testing.T) ComplexType {
	t.Helper()
	w := uWildcard(t, NamespaceConstraintAny, nil, ProcessLax)
	inner := uParticle(t, uUnbounded(t, 0), ResolvedTerm{Term: w})
	seq := uGroup(t, CompositorSequence, inner)
	ct, err := NewComplexType(xsderr.Loc{}, anyTypeName, anyTypeName, nil, DerivationRestriction, false,
		nil, nil, &w, ElementContent{Mixed: true, Particle: uOne(t, ResolvedTerm{Term: seq})}, nil, nil)
	if err != nil {
		t.Fatalf("NewComplexType(xs:anyType): %v", err)
	}
	return ct
}

// dOwnInline builds the element declaration that OWNS ct, an ANONYMOUS complex
// type a helper here already minted the {context} identity for. It reads that
// identity back off the type and hands it to NewElementDeclarationOwningTypes,
// the only entry point for this shape, so the two halves of §3.4.2.1
// dcl.ctd.common agree by construction. A caller that mints the identity itself
// calls the constructor directly instead — this helper exists for the tests
// whose subject is something other than the identity (STYLE T4).
func dOwnInline(t *testing.T, name QName, ct ComplexType, scope Scope, affiliations ...QName) ElementDeclaration {
	t.Helper()
	context, ok := ct.Context()
	if !ok {
		t.Fatalf("dOwnInline(%s): the complex type is NAMED (%s), so it is not owned by a declaration", name, ct.Name())
	}
	e, err := NewElementDeclarationOwningTypes(xsderr.Loc{}, context.ID(), name, InlineTypeDefinition{Definition: ct}, nil, scope,
		nil, false, nil, affiliations, nil, false, nil)
	if err != nil {
		t.Fatalf("NewElementDeclarationOwningTypes(%s): %v", name, err)
	}
	return e
}

// dType builds a complex type restricting base, with the given {content type},
// {attribute uses} and {attribute wildcard}. An absent name selects
// NewAnonymousComplexType, whose §3.4.1 tableau makes {context} Required; the
// callers that pass one build an inline type under an element declaration, so
// a freshly minted ElementDeclarationContext is the honest arm — dOwnInline
// hands that same identity to the declaration.
func dType(t *testing.T, name, base QName, content ContentType, uses []AttributeUse, wildcard *Wildcard) ComplexType {
	t.Helper()
	return dTypeAt(t, xsderr.Loc{}, name, base, content, uses, wildcard)
}

// dTypeAt is dType positioned at loc, for a test that asserts which component a
// rejection is charged to.
func dTypeAt(t *testing.T, loc xsderr.Loc, name, base QName, content ContentType, uses []AttributeUse, wildcard *Wildcard) ComplexType {
	t.Helper()
	if name.Local == "" {
		ct, err := NewAnonymousComplexType(loc, ElementDeclarationContext{Component: NewComponentID()},
			base, nil, DerivationRestriction, false, attributeUseMembers(uses), nil, wildcard, content, nil, nil)
		if err != nil {
			t.Fatalf("NewAnonymousComplexType: %v", err)
		}
		return ct
	}
	ct, err := NewComplexType(loc, name, base, nil, DerivationRestriction, false,
		attributeUseMembers(uses), nil, wildcard, content, nil, nil)
	if err != nil {
		t.Fatalf("NewComplexType(%s): %v", name, err)
	}
	return ct
}

// dFinal builds a complex type whose {final} carries the given derivation
// methods.
func dFinal(t *testing.T, name, base QName, final []DerivationMethod) ComplexType {
	t.Helper()
	ct, err := NewComplexType(xsderr.Loc{}, name, base, final, DerivationRestriction, false,
		nil, nil, nil, EmptyContent{}, nil, nil)
	if err != nil {
		t.Fatalf("NewComplexType(%s): %v", name, err)
	}
	return ct
}

// dAssertType builds an empty-content named complex type deriving from base by
// method and carrying the given {assertions}. It is the ONLY way to assemble the
// state derivation-ok-restriction clause 5 and cos-ct-extends clause 1.7 exist to
// reject: §3.4.2.1 clause 1's fold (parser/produce_xpath.go) makes the prefix
// hold by construction for every type the producer maps, so the rejecting cases
// have to be built through the exported constructor, which folds nothing.
func dAssertType(t *testing.T, name, base QName, method DerivationMethod, assertions []Assertion) ComplexType {
	t.Helper()
	ct, err := NewComplexType(xsderr.Loc{}, name, base, nil, method, false,
		nil, nil, nil, EmptyContent{}, nil, assertions)
	if err != nil {
		t.Fatalf("NewComplexType(%s): %v", name, err)
	}
	return ct
}

// dAssert builds an Assertion over the given XPath text, the only property that
// distinguishes two Assertions for the prefix relation (assertionprefix.go).
func dAssert(test string) Assertion {
	return NewAssertion(NewXPathExpression(test, nil, nil, nil))
}

// dSimple builds a named atomic simple type restricting base. It declares a
// plain ·restriction·, so its {variety} and {primitive type definition} derive
// from base the way the XML mapping gives them (§3.16.2.1). base must not be
// xs:anyAtomicType, which only a primitive datatype may name as its {base type
// definition} — dPrimitive builds those.
func dSimple(t *testing.T, name QName, base *SimpleType) *SimpleType {
	t.Helper()
	st, err := newCheckedSimpleType(xsderr.Loc{}, name, RestrictionDerivation{}, base, nil, nil)
	if err != nil {
		t.Fatalf("newCheckedSimpleType(%s): %v", name, err)
	}
	return st
}

// dPrimitive builds a named primitive datatype: the {base type definition} is
// xs:anyAtomicType and the {primitive type definition} is the node itself
// (§3.16.1). It is what a fixture needing a plain atomic type to hang
// declarations off must build, since st-props-correct clause 1 admits no other
// shape directly on xs:anyAtomicType.
func dPrimitive(t *testing.T, name QName) *SimpleType {
	t.Helper()
	st, err := newCheckedPrimitiveType(xsderr.Loc{}, name, nil, nil)
	if err != nil {
		t.Fatalf("newCheckedPrimitiveType(%s): %v", name, err)
	}
	return st
}

// dAttr builds an optional attribute use over a sibling local declaration of the
// given name and type.
func dAttr(t *testing.T, name, typeName QName) AttributeUse {
	t.Helper()
	return dAttrUse(t, name, typeName, false, nil)
}

// dAttrUse builds an attribute use over a sibling local declaration, with the
// given {required} and the declaration's {value constraint}.
func dAttrUse(t *testing.T, name, typeName QName, required bool, vc *ValueConstraint) AttributeUse {
	t.Helper()
	decl, err := NewAttributeDeclaration(xsderr.Loc{}, name, TypeDefinitionRef{Name: typeName}, aLocalScope(t), vc, false)
	if err != nil {
		t.Fatalf("NewAttributeDeclaration(%s): %v", name, err)
	}
	u, err := NewAttributeUse(xsderr.Loc{}, required, LocalAttributeDeclaration{Declaration: decl}, nil, nil)
	if err != nil {
		t.Fatalf("NewAttributeUse(%s): %v", name, err)
	}
	return u
}

// dFinalize assembles a schema carrying xs:anyType, the two named simple types
// the attribute tests use, and whatever build adds, then finalizes it.
func dFinalize(t *testing.T, build func(*SchemaBuilder)) error {
	t.Helper()
	b := NewSchemaBuilder()
	b.AddType(dAnyType(t))
	b.AddType(uNamedType(t, uq("T")))
	str := dPrimitive(t, uq("str"))
	b.AddType(str)
	b.AddType(dSimple(t, uq("narrow"), str))
	b.AddType(dPrimitive(t, uq("other")))
	build(b)
	_, err := b.Finalize()
	return err
}

// dElementContent wraps a model group as an element-only or mixed {content type}.
func dElementContent(t *testing.T, mixed bool, g ModelGroup) ElementContent {
	t.Helper()
	return ElementContent{Mixed: mixed, Particle: uOne(t, ResolvedTerm{Term: g})}
}

// TestDerivationOKRestrictionClause1 pins clause 1: a base whose {final}
// contains restriction may not be restricted.
func TestDerivationOKRestrictionClause1(t *testing.T) {
	err := dFinalize(t, func(b *SchemaBuilder) {
		b.AddType(dFinal(t, uq("base"), anyTypeName, []DerivationMethod{DerivationRestriction}))
		b.AddType(dType(t, uq("derived"), uq("base"), EmptyContent{}, nil, nil))
	})
	expectRule(t, err, ruleDerivationOKRestriction)
}

// TestDerivationOKRestrictionClause1Passes is the control: an {final} of
// extension alone does not block restriction.
func TestDerivationOKRestrictionClause1Passes(t *testing.T) {
	err := dFinalize(t, func(b *SchemaBuilder) {
		b.AddType(dFinal(t, uq("base"), anyTypeName, []DerivationMethod{DerivationExtension}))
		b.AddType(dType(t, uq("derived"), uq("base"), EmptyContent{}, nil, nil))
	})
	if err != nil {
		t.Fatalf("restricting a base final only for extension was rejected: %v", err)
	}
}

// TestDerivationOKRestrictionClause21 pins clause 2.1: a base that IS xs:anyType
// discharges clause 2 outright, whatever the derived {content type} is.
func TestDerivationOKRestrictionClause21(t *testing.T) {
	err := dFinalize(t, func(b *SchemaBuilder) {
		b.AddType(dType(t, uq("derived"), anyTypeName,
			dElementContent(t, false, uGroup(t, CompositorSequence, rElem(t, 1, 1))), nil, nil))
	})
	if err != nil {
		t.Fatalf("restricting xs:anyType was rejected: %v", err)
	}
}

// TestDerivationOKRestrictionClause23 pins clause 2.3 — an EMPTY restriction of
// an element-only base is valid exactly when the base's {particle} is
// ·emptiable·. This is the case that exercises cos-group-emptiable /
// cos-seq-range / cos-choice-range end to end.
func TestDerivationOKRestrictionClause23(t *testing.T) {
	for _, tc := range []struct {
		name      string
		baseModel ModelGroup
		wantOK    bool
	}{
		{"base with a mandatory element is not emptiable",
			uGroup(t, CompositorSequence, rElem(t, 1, 1)), false},
		{"base whose only member is optional is emptiable",
			uGroup(t, CompositorSequence, rElem(t, 0, 1)), true},
		{"base sequence summing to a mandatory member is not emptiable",
			uGroup(t, CompositorSequence, rNamedElem(t, "e1", 0, 1), rNamedElem(t, "e2", 1, 1)), false},
		{"base choice with one skippable branch is emptiable",
			uGroup(t, CompositorChoice, rNamedElem(t, "e1", 1, 1), rNamedElem(t, "e2", 0, 1)), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := dFinalize(t, func(b *SchemaBuilder) {
				b.AddType(dType(t, uq("base"), anyTypeName, dElementContent(t, false, tc.baseModel), nil, nil))
				b.AddType(dType(t, uq("derived"), uq("base"), EmptyContent{}, nil, nil))
			})
			if tc.wantOK && err != nil {
				t.Fatalf("emptying an ·emptiable· base was rejected: %v", err)
			}
			if !tc.wantOK {
				expectRule(t, err, ruleDerivationOKRestriction)
			}
		})
	}
}

// TestDerivationOKRestrictionClause22 pins clause 2.2: a simple-content
// restriction's {simple type definition} must be validly derived from the base's
// (2.2.2.1).
func TestDerivationOKRestrictionClause22(t *testing.T) {
	for _, tc := range []struct {
		name       string
		derivedSTs string
		wantOK     bool
	}{
		{"narrowing to a derived simple type is valid", "narrow", true},
		{"widening to an unrelated simple type is not", "other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := dFinalize(t, func(b *SchemaBuilder) {
				str := dPrimitive(t, uq("str2"))
				narrow := dSimple(t, uq("narrow2"), str)
				other := dPrimitive(t, uq("other2"))
				b.AddType(str)
				b.AddType(narrow)
				b.AddType(other)
				derived := narrow
				if tc.derivedSTs == "other" {
					derived = other
				}
				b.AddType(dType(t, uq("base"), anyTypeName, SimpleContent{SimpleType: str}, nil, nil))
				b.AddType(dType(t, uq("derived"), uq("base"), SimpleContent{SimpleType: derived}, nil, nil))
			})
			if tc.wantOK && err != nil {
				t.Fatalf("a valid simple-content restriction was rejected: %v", err)
			}
			if !tc.wantOK {
				expectRule(t, err, ruleDerivationOKRestriction)
			}
		})
	}
}

// TestDerivationOKRestrictionClause241 pins clause 2.4.1: a MIXED restriction of
// an element-only base fails the variety match, and no other branch of clause 2
// rescues it. This is the case that must survive the 2.4.2 seam returning true —
// if the seam ever leaked into 2.4.1 this test would go green wrongly.
func TestDerivationOKRestrictionClause241(t *testing.T) {
	err := dFinalize(t, func(b *SchemaBuilder) {
		b.AddType(dType(t, uq("base"), anyTypeName,
			dElementContent(t, false, uGroup(t, CompositorSequence, rElem(t, 1, 1))), nil, nil))
		b.AddType(dType(t, uq("derived"), uq("base"),
			dElementContent(t, true, uGroup(t, CompositorSequence, rElem(t, 1, 1))), nil, nil))
	})
	expectRule(t, err, ruleDerivationOKRestriction)
}

// TestDerivationOKRestrictionClause242 pins that clause 2.4.2 is now DECIDED
// (cos-content-act-restrict, contentrestricts.go) rather than provisionally
// accepted: an element-only restriction of an element-only base passes 2.4.1 and
// is then rejected because its content model admits sequences the base does not.
// The shape is the one the pre-#263 seam test asserted must be accepted, so the
// landing shows up as a deliberate change here.
func TestDerivationOKRestrictionClause242(t *testing.T) {
	err := dFinalize(t, func(b *SchemaBuilder) {
		b.AddType(dType(t, uq("base"), anyTypeName,
			dElementContent(t, false, uGroup(t, CompositorSequence, rElem(t, 1, 1))), nil, nil))
		b.AddType(dType(t, uq("derived"), uq("base"),
			dElementContent(t, false, uGroup(t, CompositorSequence, rElem(t, 5, 9))), nil, nil))
	})
	expectRule(t, err, ruleDerivationOKRestriction)
}

// TestDerivationOKRestrictionClause3UnknownAttribute pins clause 3 (c-ran): a
// restriction may not introduce an attribute the base neither declares nor
// admits through an {attribute wildcard}.
func TestDerivationOKRestrictionClause3UnknownAttribute(t *testing.T) {
	err := dFinalize(t, func(b *SchemaBuilder) {
		b.AddType(dType(t, uq("base"), anyTypeName, EmptyContent{}, nil, nil))
		b.AddType(dType(t, uq("derived"), uq("base"), EmptyContent{},
			[]AttributeUse{dAttr(t, uq("a"), uq("str"))}, nil))
	})
	expectRule(t, err, ruleDerivationOKRestriction)
}

// TestDerivationOKRestrictionClause3WildcardAdmits is the control for the
// previous test: a base with an {attribute wildcard} admitting the name gives a
// ·default binding·, so the restriction is accepted.
func TestDerivationOKRestrictionClause3WildcardAdmits(t *testing.T) {
	err := dFinalize(t, func(b *SchemaBuilder) {
		w := uWildcard(t, NamespaceConstraintAny, nil, ProcessLax)
		b.AddType(dType(t, uq("base"), anyTypeName, EmptyContent{}, nil, &w))
		b.AddType(dType(t, uq("derived"), uq("base"), EmptyContent{},
			[]AttributeUse{dAttr(t, uq("a"), uq("str"))}, nil))
	})
	if err != nil {
		t.Fatalf("an attribute admitted by the base's {attribute wildcard} was rejected: %v", err)
	}
}

// TestDerivationOKRestrictionClause3InheritedAttribute pins the §3.4.2.4 clause
// 3 fold clause 3 (c-ran) reads B.{attribute uses} through: in the chain
// A(@x) ← B(inheriting @x, re-declaring nothing) ← C, x IS a member of
// B.{attribute uses}, even though the producer maps only B's own <attribute>
// children onto B. Charging C for it would be a false reject.
//
// The table's rejecting rows are what keep the passing rows honest: a widened
// type is still charged (so the inherited use is really found and compared under
// loc-testSubP clause 5, not waved through), and a name absent from the WHOLE
// chain is still charged (so the walk does not simply admit everything once it
// reaches xs:anyType).
func TestDerivationOKRestrictionClause3InheritedAttribute(t *testing.T) {
	for _, tc := range []struct {
		name   string
		use    AttributeUse
		wantOK bool
	}{
		{"re-declaring an attribute the base only inherits is valid",
			dAttr(t, uq("x"), uq("str")), true},
		{"re-declaring the inherited attribute as required is valid",
			dAttrUse(t, uq("x"), uq("str"), true, nil), true},
		{"narrowing the inherited attribute's type is valid",
			dAttr(t, uq("x"), uq("narrow")), true},
		{"widening the inherited attribute's type is still charged",
			dAttr(t, uq("x"), uq("other")), false},
		{"a name absent from the whole base chain is still charged",
			dAttr(t, uq("z"), uq("str")), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := dFinalize(t, func(b *SchemaBuilder) {
				b.AddType(dType(t, uq("A"), anyTypeName, EmptyContent{},
					[]AttributeUse{dAttr(t, uq("x"), uq("str"))}, nil))
				b.AddType(dType(t, uq("B"), uq("A"), EmptyContent{}, nil, nil))
				b.AddType(dType(t, uq("C"), uq("B"), EmptyContent{}, []AttributeUse{tc.use}, nil))
			})
			if tc.wantOK && err != nil {
				t.Fatalf("an attribute the base inherits was rejected: %v", err)
			}
			if !tc.wantOK {
				expectRule(t, err, ruleDerivationOKRestriction)
			}
		})
	}
}

// TestDerivationOKRestrictionClause3WidenedType pins clause 3 via loc-testSubP
// clause 5.1: a restriction may narrow an attribute's type but not widen it.
func TestDerivationOKRestrictionClause3WidenedType(t *testing.T) {
	for _, tc := range []struct {
		name        string
		derivedType QName
		wantOK      bool
	}{
		{"narrowing the attribute type is valid", uq("narrow"), true},
		{"widening it to an unrelated type is not", uq("other"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := dFinalize(t, func(b *SchemaBuilder) {
				b.AddType(dType(t, uq("base"), anyTypeName, EmptyContent{},
					[]AttributeUse{dAttr(t, uq("a"), uq("str"))}, nil))
				b.AddType(dType(t, uq("derived"), uq("base"), EmptyContent{},
					[]AttributeUse{dAttr(t, uq("a"), tc.derivedType)}, nil))
			})
			if tc.wantOK && err != nil {
				t.Fatalf("narrowing an attribute's type was rejected: %v", err)
			}
			if !tc.wantOK {
				expectRule(t, err, ruleDerivationOKRestriction)
			}
		})
	}
}

// TestDerivationOKRestrictionClause3RelaxedRequired pins the cvc-complex-type
// clause 3 half of c-ran: an attribute the base REQUIRES may not be relaxed to
// optional by a restriction.
func TestDerivationOKRestrictionClause3RelaxedRequired(t *testing.T) {
	err := dFinalize(t, func(b *SchemaBuilder) {
		b.AddType(dType(t, uq("base"), anyTypeName, EmptyContent{},
			[]AttributeUse{dAttrUse(t, uq("a"), uq("str"), true, nil)}, nil))
		b.AddType(dType(t, uq("derived"), uq("base"), EmptyContent{},
			[]AttributeUse{dAttrUse(t, uq("a"), uq("str"), false, nil)}, nil))
	})
	expectRule(t, err, ruleDerivationOKRestriction)
}

// TestDerivationOKRestrictionClause3DroppedFixed pins loc-testSubP clause 5.2: a
// restriction may not drop the base's ·effective value constraint· of {variety}
// fixed.
func TestDerivationOKRestrictionClause3DroppedFixed(t *testing.T) {
	fixed := NewValueConstraint(ValueFixed, "7", nil, nil)
	for _, tc := range []struct {
		name      string
		derivedVC *ValueConstraint
		wantOK    bool
	}{
		{"keeping the same fixed value is valid", &fixed, true},
		{"dropping the fixed value constraint is not", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := dFinalize(t, func(b *SchemaBuilder) {
				b.AddType(dType(t, uq("base"), anyTypeName, EmptyContent{},
					[]AttributeUse{dAttrUse(t, uq("a"), uq("str"), false, &fixed)}, nil))
				b.AddType(dType(t, uq("derived"), uq("base"), EmptyContent{},
					[]AttributeUse{dAttrUse(t, uq("a"), uq("str"), false, tc.derivedVC)}, nil))
			})
			if tc.wantOK && err != nil {
				t.Fatalf("preserving the base's fixed value was rejected: %v", err)
			}
			if !tc.wantOK {
				expectRule(t, err, ruleDerivationOKRestriction)
			}
		})
	}
}

// TestDerivationOKRestrictionClause4Element pins clause 4 (c-vs-ctd-r) over
// elements: the ·locally declared type· of an element name within the
// restriction must be ·validly substitutable· for its ·locally declared type·
// within the base.
func TestDerivationOKRestrictionClause4Element(t *testing.T) {
	for _, tc := range []struct {
		name        string
		derivedType QName
		wantOK      bool
	}{
		{"narrowing an element's type is valid", uq("narrow"), true},
		{"retyping it to an unrelated type is not", uq("other"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := dFinalize(t, func(b *SchemaBuilder) {
				baseModel := uGroup(t, CompositorSequence,
					uOne(t, ResolvedTerm{Term: uLocal(t, uq("e"), uq("str"))}))
				derivedModel := uGroup(t, CompositorSequence,
					uOne(t, ResolvedTerm{Term: uLocal(t, uq("e"), tc.derivedType)}))
				b.AddType(dType(t, uq("base"), anyTypeName, dElementContent(t, false, baseModel), nil, nil))
				b.AddType(dType(t, uq("derived"), uq("base"), dElementContent(t, false, derivedModel), nil, nil))
			})
			if tc.wantOK && err != nil {
				t.Fatalf("narrowing an element's declared type was rejected: %v", err)
			}
			if !tc.wantOK {
				expectRule(t, err, ruleDerivationOKRestriction)
			}
		})
	}
}

// TestDerivationOKRestrictionClause4AnyTypeBase pins clause 4 against a base
// that leaves an element untyped, which declares it xs:anyType (§3.3.2.1 case
// 4): a simple type is validly substitutable for xs:anyType under {extension,
// list, union}, since cos-st-derived-ok reads restriction alone; a
// simpleContent EXTENSION of one is not, since cos-ct-derived-ok clause 1 reads
// extension on its step. Rejecting the first would false-reject the commonest
// restriction shape there is.
//
// The rejection surfaces as clause 2, not clause 4: clause 2.4.2's
// ctr-child-type-subsumption reads the same relation first, through loc-testSubP
// clause 4.5 (declaredTypeRestricts), and checkDerivationOKRestriction reports
// the first failing clause.
func TestDerivationOKRestrictionClause4AnyTypeBase(t *testing.T) {
	for _, tc := range []struct {
		name        string
		derivedType QName
		wantOK      bool
	}{
		{"a simple type for an element the base left as xs:anyType", uq("str"), true},
		{"a simpleContent extension for an element the base left as xs:anyType", uq("ext"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := dFinalize(t, func(b *SchemaBuilder) {
				xstr := dPrimitive(t, uq("xstr"))
				b.AddType(xstr)
				ext, cerr := NewComplexType(xsderr.Loc{}, uq("ext"), uq("xstr"), nil, DerivationExtension, false,
					nil, nil, nil, SimpleContent{SimpleType: xstr}, nil, nil)
				if cerr != nil {
					t.Fatalf("NewComplexType(ext): %v", cerr)
				}
				b.AddType(ext)
				baseModel := uGroup(t, CompositorSequence,
					uOne(t, ResolvedTerm{Term: uLocal(t, uq("e"), anyTypeName)}))
				derivedModel := uGroup(t, CompositorSequence,
					uOne(t, ResolvedTerm{Term: uLocal(t, uq("e"), tc.derivedType)}))
				b.AddType(dType(t, uq("base"), anyTypeName, dElementContent(t, false, baseModel), nil, nil))
				b.AddType(dType(t, uq("derived"), uq("base"), dElementContent(t, false, derivedModel), nil, nil))
			})
			if tc.wantOK && err != nil {
				t.Fatalf("typing an element the base left as xs:anyType was rejected: %v", err)
			}
			if !tc.wantOK {
				expectRule(t, err, ruleDerivationOKRestriction)
				if !strings.Contains(err.Error(), "any branch of derivation-ok-restriction clause 2 ") {
					t.Fatalf("want the clause 2.4.2 charge, which checkDerivationOKRestriction reaches before clause 4: %v", err)
				}
			}
		})
	}
}

// TestDerivationOKRestrictionClause5 pins clause 5 — B.{assertions} is a prefix
// of T.{assertions} — in both directions. §3.4.2.1 clause 1's fold makes the
// relation hold by construction for a type the producer mapped (see
// parser/produce_xpath.go's assertionsWithBase and the parser-side test that
// pins it), so every rejecting case here is assembled through the exported
// constructor instead (dAssertType), which folds nothing.
func TestDerivationOKRestrictionClause5(t *testing.T) {
	baseAssert, ownAssert := dAssert("true()"), dAssert("@a > 0")
	for _, tc := range []struct {
		name    string
		derived []Assertion
		wantOK  bool
	}{
		{"the fold's own output: the base's assertion, then the type's own", []Assertion{baseAssert, ownAssert}, true},
		{"the base's assertions alone are a prefix of themselves", []Assertion{baseAssert}, true},
		{"a restriction that keeps only its own assertion", []Assertion{ownAssert}, false},
		{"a restriction carrying no assertions at all", nil, false},
		{"a restriction that puts its own assertion FIRST", []Assertion{ownAssert, baseAssert}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := dFinalize(t, func(b *SchemaBuilder) {
				b.AddType(dAssertType(t, uq("base"), anyTypeName, DerivationRestriction, []Assertion{baseAssert}))
				b.AddType(dAssertType(t, uq("derived"), uq("base"), DerivationRestriction, tc.derived))
			})
			if tc.wantOK {
				if err != nil {
					t.Fatalf("a restriction whose {assertions} extend the base's was rejected: %v", err)
				}
				return
			}
			expectRule(t, err, ruleDerivationOKRestriction)
		})
	}
}

// TestDerivationOKRestrictionClause5DistinguishesTests is the mutation guard on
// the identity relation clause 5's prefix is taken under (assertionprefix.go): a
// derived type that repeats the base's assertion COUNT with a different {test}
// is not a prefix, and would slip through a comparison that only counted.
func TestDerivationOKRestrictionClause5DistinguishesTests(t *testing.T) {
	err := dFinalize(t, func(b *SchemaBuilder) {
		b.AddType(dAssertType(t, uq("base"), anyTypeName, DerivationRestriction, []Assertion{dAssert("true()")}))
		b.AddType(dAssertType(t, uq("derived"), uq("base"), DerivationRestriction, []Assertion{dAssert("false()")}))
	})
	expectRule(t, err, ruleDerivationOKRestriction)
}

// TestCTPropsCorrectSimpleBase pins ct-props-correct clause 2: a simple {base
// type definition} forces {derivation method} = extension, so a restriction of
// one is rejected — and charged ct-props-correct, not the coarser
// derivation-ok-restriction clause 1.
func TestCTPropsCorrectSimpleBase(t *testing.T) {
	err := dFinalize(t, func(b *SchemaBuilder) {
		b.AddType(dType(t, uq("derived"), uq("str"), EmptyContent{}, nil, nil))
	})
	expectRule(t, err, ruleCTPropsCorrect)
}

// TestCTPropsCorrectDuplicateAttributeUses pins ct-props-correct clause 4: two
// {attribute uses} may not share an {attribute declaration} expanded name.
func TestCTPropsCorrectDuplicateAttributeUses(t *testing.T) {
	err := dFinalize(t, func(b *SchemaBuilder) {
		b.AddType(dType(t, uq("ct"), anyTypeName, EmptyContent{},
			[]AttributeUse{dAttr(t, uq("a"), uq("str")), dAttr(t, uq("a"), uq("str"))}, nil))
	})
	expectRule(t, err, ruleCTPropsCorrect)
}

// TestCTPropsCorrectDistinctAttributeUsesPass is the control: two uses of
// different names are fine.
func TestCTPropsCorrectDistinctAttributeUsesPass(t *testing.T) {
	err := dFinalize(t, func(b *SchemaBuilder) {
		b.AddType(dType(t, uq("ct"), anyTypeName, EmptyContent{},
			[]AttributeUse{dAttr(t, uq("a"), uq("str")), dAttr(t, uq("b"), uq("str"))}, nil))
	})
	if err != nil {
		t.Fatalf("two distinctly named attribute uses were rejected: %v", err)
	}
}

// TestDerivedOKComplexHonoursBlockingSet pins that cos-ct-derived-ok's blocking
// subset is load-bearing on the complex side (warden's caveat to confirmation
// 1): the same pair is derived-OK under the empty set and not under {restriction}.
func TestDerivedOKComplexHonoursBlockingSet(t *testing.T) {
	b := NewSchemaBuilder()
	b.AddType(dAnyType(t))
	b.AddType(dType(t, uq("base"), anyTypeName, EmptyContent{}, nil, nil))
	b.AddType(dType(t, uq("derived"), uq("base"), EmptyContent{}, nil, nil))
	s, err := b.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	base, _ := s.Type(uq("base"))
	derived, _ := s.Type(uq("derived"))
	d, ok := derived.(ComplexType)
	if !ok {
		t.Fatalf("derived is not a ComplexType")
	}
	if ok, err := s.derivedOKComplex(d, base, nil); err != nil || !ok {
		t.Fatalf("derived is not OK from base under the empty blocking set")
	}
	if ok, err := s.derivedOKComplex(d, base, []DerivationMethod{DerivationRestriction}); err != nil || ok {
		t.Fatalf("derived was accepted from base with restriction in the blocking set")
	}
	if ok, err := s.derivedOKComplex(d, base, restrictionBlockingKeywords); err != nil || !ok {
		t.Fatalf("clause 4's {extension, list, union} set must not block a restriction")
	}
}

// dSimpleContentSchema finalizes a schema carrying xs:anyType, a primitive str,
// narrow restricting str, and two simpleContent extensions: ext of str and ext2
// of narrow.
func dSimpleContentSchema(t *testing.T) *Schema {
	t.Helper()
	b := NewSchemaBuilder()
	b.AddType(dAnyType(t))
	str := dPrimitive(t, uq("str"))
	narrow := dSimple(t, uq("narrow"), str)
	b.AddType(str)
	b.AddType(narrow)
	for _, ext := range []struct {
		name, base QName
		st         *SimpleType
	}{{uq("ext"), uq("str"), str}, {uq("ext2"), uq("narrow"), narrow}} {
		ct, err := NewComplexType(xsderr.Loc{}, ext.name, ext.base, nil, DerivationExtension, false,
			nil, nil, nil, SimpleContent{SimpleType: ext.st}, nil, nil)
		if err != nil {
			t.Fatalf("NewComplexType(%s): %v", ext.name, err)
		}
		b.AddType(ct)
	}
	s, err := b.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return s
}

// TestDeclaredTypeRestrictsAnyTypeBase pins loc-testSubP clause 4.5 (c-vs-ct)
// where G's declared type is xs:anyType: key-val-sub-type-restricts' constant
// set {extension, list, union} admits a simple type and a restriction of
// xs:anyType, and turns away a simpleContent extension, whose own step is
// extension (cos-ct-derived-ok clause 1).
func TestDeclaredTypeRestrictsAnyTypeBase(t *testing.T) {
	s := dSimpleContentSchema(t)
	general := uLocal(t, uq("a"), anyTypeName)
	for _, tc := range []struct {
		name     string
		specific QName
		want     bool
	}{
		{"xs:anyType itself", anyTypeName, true},
		{"a simple type", uq("narrow"), true},
		{"a simpleContent extension", uq("ext"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.declaredTypeRestricts(uLocal(t, uq("a"), tc.specific), general); got != tc.want {
				t.Fatalf("declaredTypeRestricts(%s, xs:anyType) = %v, want %v", tc.specific, got, tc.want)
			}
		})
	}
}

// TestValidlySubstitutableAnyType pins key-val-sub-type against the super
// ·xs:anyType·, which no blocking set is short-circuited for: each step of a
// complex sub's chain reads cos-ct-derived-ok clause 1, and a simple sub reads
// cos-st-derived-ok clause 2.1 up to the xs:anySimpleType -> xs:anyType hop.
func TestValidlySubstitutableAnyType(t *testing.T) {
	s := dSimpleContentSchema(t)
	anyType, _ := s.Type(anyTypeName)
	extension := []DerivationMethod{DerivationExtension}
	restriction := []DerivationMethod{DerivationRestriction}
	for _, tc := range []struct {
		name    string
		sub     TypeDefinition
		blocked []DerivationMethod
		want    bool
	}{
		{"simpleContent extension, empty set", mustType(t, s, "ext"), nil, true},
		{"simpleContent extension, {extension} (cos-ct-derived-ok 1)", mustType(t, s, "ext"), extension, false},
		{"simpleContent extension, {restriction} (cos-ct-derived-ok 2.3.2.2)", mustType(t, s, "ext"), restriction, false},
		{"simple type, {extension} (cos-st-derived-ok reads restriction alone)", mustType(t, s, "narrow"), extension, true},
		{"simple type, {restriction} (cos-st-derived-ok 2.1)", mustType(t, s, "narrow"), restriction, false},
		{"xs:anySimpleType, {restriction} (2.1 at the hop)", anySimpleType, restriction, false},
		{"xs:anyType itself, {extension, restriction} (identity)", anyType, []DerivationMethod{DerivationExtension, DerivationRestriction}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := s.ValidlySubstitutable(tc.sub, anyType, tc.blocked); err != nil || got != tc.want {
				t.Fatalf("ValidlySubstitutable(%s, xs:anyType, %v) = %v (err %v), want %v", typeDefinitionLabel(tc.sub), tc.blocked, got, err, tc.want)
			}
		})
	}
}

// mustType looks a named type definition up in s or fails the test.
func mustType(t *testing.T, s *Schema, local string) TypeDefinition {
	t.Helper()
	td, ok := s.Type(uq(local))
	if !ok {
		t.Fatalf("%s is not in the schema", local)
	}
	return td
}

// TestSimpleDerivedOK pins cos-st-derived-ok (§3.16.6.3) for a simple D against a
// B that may be complex: ·xs:anyType· is reached through xs:anySimpleType, whose
// own hop to it reads clause 2.1 too, and no other complex type is reached.
func TestSimpleDerivedOK(t *testing.T) {
	s := dSimpleContentSchema(t)
	restriction := []DerivationMethod{DerivationRestriction}
	for _, tc := range []struct {
		name    string
		d       *SimpleType
		b       QName
		blocked []DerivationMethod
		want    bool
	}{
		{"xs:anySimpleType from xs:anyType, empty set (2.2.1)", anySimpleType, anyTypeName, nil, true},
		{"xs:anySimpleType from xs:anyType, {restriction} (2.1)", anySimpleType, anyTypeName, restriction, false},
		{"str from xs:anyType, empty set (2.2.2)", simpleOf(t, s, "str"), anyTypeName, nil, true},
		{"str from xs:anyType, {extension, list, union} (2.1 vacuous)", simpleOf(t, s, "str"), anyTypeName, restrictionBlockingKeywords, true},
		{"str from xs:anyType, {restriction} (2.1)", simpleOf(t, s, "str"), anyTypeName, restriction, false},
		{"narrow from str, {restriction} (2.1)", simpleOf(t, s, "narrow"), uq("str"), restriction, false},
		{"str from a complex type other than xs:anyType", simpleOf(t, s, "str"), uq("ext"), nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bt, ok := s.Type(tc.b)
			if !ok {
				t.Fatalf("%s is not in the schema", tc.b)
			}
			if got, err := s.simpleDerivedOK(tc.d, bt, tc.blocked); err != nil || got != tc.want {
				t.Fatalf("simpleDerivedOK(%s, %s, %v) = %v (err %v), want %v", tc.d.Name(), tc.b, tc.blocked, got, err, tc.want)
			}
		})
	}
}

// simpleOf looks a named simple type up in s or fails the test.
func simpleOf(t *testing.T, s *Schema, local string) *SimpleType {
	t.Helper()
	st, ok := mustType(t, s, local).(*SimpleType)
	if !ok {
		t.Fatalf("%s is not a simple type of the schema", local)
	}
	return st
}

// TestDerivedOKComplexSimpleBase pins cos-ct-derived-ok clause 2.3.2.2: a complex
// D whose {base type definition} is simple hands the SAME subset to
// cos-st-derived-ok, whose clause 2.1 reads restriction in it — and B may be
// ·xs:anyType·, which a simple type reaches through xs:anySimpleType. The
// anyType rows are the shape an xsi:type naming a simpleContent type takes under
// an untyped declaration.
func TestDerivedOKComplexSimpleBase(t *testing.T) {
	s := dSimpleContentSchema(t)
	extension := []DerivationMethod{DerivationExtension}
	restriction := []DerivationMethod{DerivationRestriction}
	for _, tc := range []struct {
		name    string
		d, b    QName
		blocked []DerivationMethod
		want    bool
	}{
		{"ext from xs:anyType, empty set", uq("ext"), anyTypeName, nil, true},
		{"ext from xs:anyType, {extension} (clause 1)", uq("ext"), anyTypeName, extension, false},
		{"ext from xs:anyType, {restriction} (2.3.2.2, cos-st-derived-ok 2.1)", uq("ext"), anyTypeName, restriction, false},
		{"ext from its own base, {restriction} (2.2)", uq("ext"), uq("str"), restriction, true},
		{"ext2 from str, empty set (2.3.2.2)", uq("ext2"), uq("str"), nil, true},
		{"ext2 from str, {restriction} (2.3.2.2, cos-st-derived-ok 2.1)", uq("ext2"), uq("str"), restriction, false},
		{"ext2 from str, {list, union} (2.3.2.2, cos-st-derived-ok reads neither)", uq("ext2"), uq("str"),
			[]DerivationMethod{DerivationList, DerivationUnion}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dt, _ := s.Type(tc.d)
			d, ok := dt.(ComplexType)
			if !ok {
				t.Fatalf("%s is not a ComplexType", tc.d)
			}
			bt, ok := s.Type(tc.b)
			if !ok {
				t.Fatalf("%s is not in the schema", tc.b)
			}
			if got, err := s.derivedOKComplex(d, bt, tc.blocked); err != nil || got != tc.want {
				t.Fatalf("derivedOKComplex(%s, %s, %v) = %v (err %v), want %v", tc.d, tc.b, tc.blocked, got, err, tc.want)
			}
		})
	}
}

// ldtSchema finalizes the key-ldt-elem fixture: base B, a sequence of an
// optional local e of type str and a lax ##any wildcard; T, a restriction of
// B whose sequence holds only a reference to the top-level head h (str); and
// the top-level e (narrow) in h's ·substitution group·. T's content model
// therefore ·implicitly contains· the top-level e (key-impl-cont) while B's
// contains the local one, and the two carry different types.
func ldtSchema(t *testing.T) *Schema {
	t.Helper()
	b := NewSchemaBuilder()
	b.AddType(dAnyType(t))
	str := dPrimitive(t, uq("str"))
	b.AddType(str)
	b.AddType(dSimple(t, uq("narrow"), str))
	b.AddElement(uGlobal(t, uq("h"), uq("str")))
	b.AddElement(uGlobal(t, uq("e"), uq("narrow"), uq("h")))
	lax := uWildcard(t, NamespaceConstraintAny, nil, ProcessLax)
	baseModel := uGroup(t, CompositorSequence,
		uParticle(t, uOccurs(t, 0, 1), ResolvedTerm{Term: uLocal(t, uq("e"), uq("str"))}),
		uOne(t, ResolvedTerm{Term: lax}))
	derivedModel := uGroup(t, CompositorSequence, uOne(t, ElementDeclarationRef{Name: uq("h")}))
	b.AddType(dType(t, uq("B"), anyTypeName, dElementContent(t, false, baseModel), nil, nil))
	b.AddType(dType(t, uq("T"), uq("B"), dElementContent(t, false, derivedModel), nil, nil))
	s, err := b.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return s
}

// TestLocallyDeclaredElementType pins key-ldt-elem's three cases on the
// exported query, and the implicit-containment half that sets it apart from
// the derivation check's explicitOnly walk: within T the name e is
// ·implicitly contained· through h, so case 2 answers T's narrow at T itself
// and never steps to B's str. Leaving the implicit gather out answers str,
// which is the row "T, e" catching it.
func TestLocallyDeclaredElementType(t *testing.T) {
	s := ldtSchema(t)
	complexType := func(name QName) ComplexType {
		t.Helper()
		td, ok := s.Type(name)
		if !ok {
			t.Fatalf("%s is not a type of the schema", name)
		}
		ct, ok := td.(ComplexType)
		if !ok {
			t.Fatalf("%s is not a ComplexType", name)
		}
		return ct
	}
	for _, tc := range []struct {
		why    string
		within QName
		name   QName
		want   QName // the zero QName: ·absent·
	}{
		{"case 2, implicitly contained through the head h", uq("T"), uq("e"), uq("narrow")},
		{"case 2, a directly contained reference", uq("T"), uq("h"), uq("str")},
		{"case 2, a directly contained local declaration", uq("B"), uq("e"), uq("str")},
		{"case 3 then case 1: no declaration on the chain", uq("T"), uq("x"), QName{}},
		{"case 1, xs:anyType", anyTypeName, uq("e"), QName{}},
	} {
		t.Run(tc.why, func(t *testing.T) {
			got, ok := s.LocallyDeclaredElementType(complexType(tc.within), tc.name)
			if tc.want == (QName{}) {
				if ok {
					t.Fatalf("LocallyDeclaredElementType(%s, %s) = %s, want ·absent·", tc.within, tc.name, typeDefinitionLabel(got))
				}
				return
			}
			if !ok {
				t.Fatalf("LocallyDeclaredElementType(%s, %s) is ·absent·, want %s", tc.within, tc.name, tc.want)
			}
			if got.Name() != tc.want {
				t.Fatalf("LocallyDeclaredElementType(%s, %s) = %s, want %s", tc.within, tc.name, typeDefinitionLabel(got), tc.want)
			}
		})
	}
	// The derivation check's walk leaves implicit containment out and so steps
	// past T to B's local e.
	got, ok := s.locallyDeclaredElementType(complexType(uq("T")), uq("e"), explicitOnly)
	if !ok || got.Name() != uq("str") {
		t.Fatalf("locallyDeclaredElementType(T, e, explicitOnly) = %v, %v, want str from B", got, ok)
	}
}

// TestContainedElementDeclarations pins the set key-ldt-elem case 2 reads, in
// the order the export promises: T's reference to h first, then the top-level
// e it ·implicitly contains· through h's ·substitution group·. B contains its
// local e and not the wildcard beside it, xs:anyType's wildcard-only content
// contains nothing, and a simple-content type contains nothing.
func TestContainedElementDeclarations(t *testing.T) {
	s := ldtSchema(t)
	complexType := func(s *Schema, name QName) ComplexType {
		t.Helper()
		td, ok := s.Type(name)
		if !ok {
			t.Fatalf("%s is not a type of the schema", name)
		}
		ct, ok := td.(ComplexType)
		if !ok {
			t.Fatalf("%s is not a ComplexType", name)
		}
		return ct
	}
	for _, tc := range []struct {
		why    string
		s      *Schema
		within QName
		want   []ScopeVariety // one per declaration, in order; names below
		names  []QName
	}{
		{"explicit then implicit", s, uq("T"), []ScopeVariety{ScopeGlobal, ScopeGlobal}, []QName{uq("h"), uq("e")}},
		{"a local declaration, the wildcard beside it left out", s, uq("B"), []ScopeVariety{ScopeLocal}, []QName{uq("e")}},
		{"wildcard-only content", s, anyTypeName, nil, nil},
		{"simple content", dSimpleContentSchema(t), uq("ext"), nil, nil},
	} {
		t.Run(tc.why, func(t *testing.T) {
			got := tc.s.ContainedElementDeclarations(complexType(tc.s, tc.within))
			if tc.names == nil {
				if got != nil {
					t.Fatalf("ContainedElementDeclarations(%s) = %d declarations, want nil", tc.within, len(got))
				}
				return
			}
			if len(got) != len(tc.names) {
				t.Fatalf("ContainedElementDeclarations(%s) = %d declarations, want %d", tc.within, len(got), len(tc.names))
			}
			for i, d := range got {
				if d.Name() != tc.names[i] || d.ScopeVariety() != tc.want[i] {
					t.Errorf("declaration %d = %s (%v), want %s (%v)", i, d.Name(), d.ScopeVariety(), tc.names[i], tc.want[i])
				}
			}
		})
	}
}

// TestContainedElementDeclarationsOnePerComponent pins the de-duplication: a
// sequence referencing the top-level h twice contains ONE declaration, while
// two local declarations sharing the name e are two components and both stay.
func TestContainedElementDeclarationsOnePerComponent(t *testing.T) {
	b := NewSchemaBuilder()
	b.AddType(dAnyType(t))
	str := dPrimitive(t, uq("str"))
	b.AddType(str)
	b.AddElement(uGlobal(t, uq("h"), uq("str")))
	model := uGroup(t, CompositorSequence,
		uOne(t, ElementDeclarationRef{Name: uq("h")}),
		uOne(t, ResolvedTerm{Term: uLocal(t, uq("e"), uq("str"))}),
		uOne(t, ElementDeclarationRef{Name: uq("h")}),
		uOne(t, ResolvedTerm{Term: uLocal(t, uq("e"), uq("str"))}))
	b.AddType(dType(t, uq("T"), anyTypeName, dElementContent(t, false, model), nil, nil))
	s, err := b.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	td, _ := s.Type(uq("T"))
	ct, ok := td.(ComplexType)
	if !ok {
		t.Fatalf("T is not a ComplexType")
	}
	got := s.ContainedElementDeclarations(ct)
	var names []QName
	for _, d := range got {
		names = append(names, d.Name())
	}
	want := []QName{uq("h"), uq("e"), uq("e")}
	if !slices.Equal(names, want) {
		t.Fatalf("ContainedElementDeclarations(T) names = %v, want %v", names, want)
	}
}

// TestContainedWildcards pins ContainedWildcards' contract: every Wildcard
// {term} the content model contains, in document order, one per particle
// occurrence — a <group ref> written twice contributes its wildcard twice —
// whatever its {process contents}, the skip one included, and never the
// {open content} wildcard.
func TestContainedWildcards(t *testing.T) {
	a, b, o := NamespaceName("urn:a"), NamespaceName("urn:b"), NamespaceName("urn:o")
	b0 := NewSchemaBuilder()
	b0.AddType(dAnyType(t))
	str := dPrimitive(t, uq("str"))
	b0.AddType(str)
	g, err := NewModelGroupDefinition(xsderr.Loc{}, uq("G"),
		uGroup(t, CompositorSequence, uOne(t, ResolvedTerm{Term: uWildcard(t, NamespaceConstraintEnumeration, []Namespace{b}, ProcessSkip)})))
	if err != nil {
		t.Fatalf("NewModelGroupDefinition(G): %v", err)
	}
	b0.AddModelGroup(g)
	model := uGroup(t, CompositorSequence,
		uOne(t, ResolvedTerm{Term: uWildcard(t, NamespaceConstraintEnumeration, []Namespace{a}, ProcessStrict)}),
		uOne(t, ModelGroupRef{Name: uq("G")}),
		uOne(t, ResolvedTerm{Term: uLocal(t, uq("e"), uq("str"))}),
		uOne(t, ModelGroupRef{Name: uq("G")}))
	oc, err := NewOpenContent(xsderr.Loc{}, OpenContentSuffix, uWildcard(t, NamespaceConstraintEnumeration, []Namespace{o}, ProcessLax))
	if err != nil {
		t.Fatalf("NewOpenContent: %v", err)
	}
	content := dElementContent(t, false, model)
	content.OpenContent = &oc
	b0.AddType(dType(t, uq("T"), anyTypeName, content, nil, nil))
	s, err := b0.Finalize()
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	td, _ := s.Type(uq("T"))
	ct, ok := td.(ComplexType)
	if !ok {
		t.Fatalf("T is not a ComplexType")
	}
	type row struct {
		ns Namespace
		pc ProcessContents
	}
	var got []row
	for _, w := range s.ContainedWildcards(ct) {
		got = append(got, row{w.NamespaceConstraint().Namespaces()[0], w.ProcessContents()})
	}
	want := []row{{a, ProcessStrict}, {b, ProcessSkip}, {b, ProcessSkip}}
	if !slices.Equal(got, want) {
		t.Fatalf("ContainedWildcards(T) = %v, want %v", got, want)
	}
	simple := dSimpleContentSchema(t)
	std, _ := simple.Type(uq("ext"))
	if got := simple.ContainedWildcards(std.(ComplexType)); got != nil {
		t.Fatalf("ContainedWildcards(simple content) = %d wildcards, want nil", len(got))
	}
}
