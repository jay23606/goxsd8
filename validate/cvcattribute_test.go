package validate

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The fixtures below drive the charges that need a value space: cvc-attribute
// (§3.2.4.1) clauses 3 and 4, cvc-au (§3.5.4), and cvc-complex-type (§3.4.4.2)
// clause 4. Unlike cvccomplextype_test.go's, every schema here SEEDS the
// builtin simple types, because a declaration whose {type definition} names no
// component in the schema resolves to nothing and declines before any of them
// is reached.
//
// xs:integer is the type throughout: strict maps it, its lexical space rejects
// "abc" outright, and "01" and "1" are one value under two lexical forms —
// which is the whole difference between the value-space comparison these rules
// demand and the lexical one they forbid.

func integerType() xsd.QName { return xsd.QName{Space: xsd.XMLSchemaNS, Local: "integer"} }

// typedUse builds an attribute use over a sibling local declaration of type
// typ, carrying the DECLARATION's {value constraint} (declVC, read by
// cvc-attribute clause 4) and the USE's own (useVC, read by cvc-au). The two
// are separate parameters because the two rules read separate properties, and
// a fixture that could not set them independently could not tell the charges
// apart.
func typedUse(t *testing.T, local string, typ xsd.QName, required bool, declVC, useVC *xsd.ValueConstraint) xsd.AttributeUse {
	t.Helper()
	decl, err := xsd.NewAttributeDeclaration(xsderr.Loc{}, xsd.QName{Local: local},
		xsd.TypeDefinitionRef{Name: typ}, xsd.NewAttributeGlobalScope(), declVC, false)
	if err != nil {
		t.Fatalf("building the %s attribute declaration: %v", local, err)
	}
	u, err := xsd.NewAttributeUse(xsderr.Loc{}, required,
		xsd.LocalAttributeDeclaration{Declaration: decl}, useVC, nil)
	if err != nil {
		t.Fatalf("building the %s attribute use: %v", local, err)
	}
	return u
}

// typedSchema is governedSchema with the builtin simple types seeded, so a
// declaration naming xs:integer resolves to the real component and its facets
// are the spec's.
//
// It finalizes through Finalize and not FinalizeWith: no value space is
// installed, so cos-valid-simple-default (§3.2.6.2) is waved through at
// assembly and a fixture may carry a {value constraint} whose {lexical form} is
// invalid — which is exactly the state cvc-complex-type clause 4 exists to
// catch at assessment time. extra adds the fixture's own named simple types
// beside the builtin ones.
func typedSchema(t *testing.T, uses []xsd.AttributeUse, extra ...*xsd.SimpleType) *xsd.Schema {
	t.Helper()
	ct, err := xsd.NewComplexType(xsderr.Loc{}, xsd.QName{Local: "RootType"}, xsd.QName{}, nil,
		xsd.DerivationRestriction, false, attrContent(uses), nil, nil, xsd.EmptyContent{}, nil, nil)
	if err != nil {
		t.Fatalf("building RootType: %v", err)
	}
	e, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: "root"},
		xsd.TypeDefinitionRef{Name: xsd.QName{Local: "RootType"}}, nil, xsd.NewGlobalScope(),
		nil, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("building the root element declaration: %v", err)
	}
	seeded, err := builtin.Seed(testBackend())
	if err != nil {
		t.Fatalf("seeding the builtin types: %v", err)
	}
	b := xsd.NewSchemaBuilder()
	for _, st := range seeded {
		b.AddType(st)
	}
	for _, st := range extra {
		b.AddType(st)
	}
	b.AddType(ct)
	b.AddElement(e)
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the typed schema: %v", err)
	}
	return schema
}

// valuedRoot is a childless <root> carrying one attribute of the given name and
// lexical value, at the Loc every assertion below cites.
func valuedRoot(name string, lexical string) *testElement {
	return &testElement{
		name:  xsd.QName{Local: "root"},
		attrs: []Attribute{&testAttribute{name: local(name), value: lexical, loc: loc(1, 10)}},
		loc:   loc(1, 1),
	}
}

// assessTyped assesses root against a schema declaring "root" over uses, with
// the builtin types and extra seeded.
func assessTyped(t *testing.T, root Element, uses []xsd.AttributeUse, extra ...*xsd.SimpleType) []*xsderr.Error {
	t.Helper()
	got, _ := assessRecorded(t, typedSchema(t, uses, extra...), root)
	return got
}

// onlyCharge fails unless exactly one violation was charged under rule, and
// returns it.
func onlyCharge(t *testing.T, got []*xsderr.Error, rule xsderr.Rule) *xsderr.Error {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("Violations() = %v, want exactly one %s charge", got, rule)
	}
	if got[0].Rule != rule {
		t.Fatalf("Rule = %q, want %q", got[0].Rule, rule)
	}
	return got[0]
}

// cvc-attribute clause 3: a lexical outside the declaration's {type
// definition}'s lexical space is charged, and one inside it is not. This is the
// charge the whole seam exists for.
func TestAttributeLexicalIsCheckedAgainstItsType(t *testing.T) {
	uses := []xsd.AttributeUse{typedUse(t, "n", integerType(), false, nil, nil)}

	got := onlyCharge(t, assessTyped(t, valuedRoot("n", "abc"), uses), "cvc-attribute")
	if got.Loc != loc(1, 10) {
		t.Errorf("Loc = %s, want the attribute's %s", got.Loc, loc(1, 10))
	}
	if !strings.Contains(got.Msg, "clause 3") || !strings.Contains(got.Msg, "String Valid") {
		t.Errorf("Msg = %q, want clause 3 and its String Valid delegation named", got.Msg)
	}

	wantSilence(t, assessTyped(t, valuedRoot("n", "42"), uses), "42 is an xs:integer")
	// String Valid clause 1 normalizes before Datatype Valid runs, so a
	// lexical the whiteSpace facet collapses to a valid one is valid.
	wantSilence(t, assessTyped(t, valuedRoot("n", "  42  "), uses),
		"whiteSpace normalization precedes datatype validation (String Valid clause 1)")
}

// An attribute declaration with NO @type is xs:anySimpleType (§3.2.2.2's third
// tier), which no backend maps. Datatype Valid holds for every literal against
// it and against xs:anyAtomicType (Datatypes §4.1.4, the ·special· disjunct), so
// clause 3 is DECIDED satisfied: neither charged, which would reject every
// typeless attribute in existence, nor declined, which would leave every such
// document undecided (#1788). The same holds of a ·defaulted attribute·'s
// {lexical form} under cvc-complex-type clause 4.
func TestTypelessAttributeIsDecided(t *testing.T) {
	bare := &testElement{name: xsd.QName{Local: "root"}, loc: loc(1, 1)}
	dflt := xsd.NewValueConstraint(xsd.ValueDefault, "anything at all", nil, nil)
	for _, typ := range []xsd.QName{icBuiltin("anySimpleType"), icBuiltin("anyAtomicType")} {
		got, undecided := assessRecorded(t, typedSchema(t, []xsd.AttributeUse{typedUse(t, "n", typ, false, nil, nil)}),
			valuedRoot("n", "anything at all"))
		wantSilence(t, got, "Datatype Valid holds for every literal against a ·special· datatype")
		wantDeclines(t, undecided)

		got, undecided = assessRecorded(t, typedSchema(t, []xsd.AttributeUse{typedUse(t, "n", typ, false, &dflt, nil)}), bare)
		wantSilence(t, got, "Datatype Valid holds for every default against a ·special· datatype")
		wantDeclines(t, undecided)
	}
}

// Every attribute-side decline is RECORDED as an [Unevaluated] at the item it
// withheld a verdict on, under the rule it would have been charged under: a
// {type definition} the backend does not map (cvc-attribute clause 3), a fixed
// comparison over a ·special· type under a backend that leaves one member of its
// mapping union (here xs:string) unmapped (cvc-attribute clause 4, cvc-au), and a
// ·defaulted attribute· whose {lexical form} the value space cannot read
// (cvc-complex-type clause 4, at the element).
func TestAttributeDeclinesAreRecorded(t *testing.T) {
	decimal := []xsd.AttributeUse{typedUse(t, "n", icBuiltin("decimal"), false, nil, nil)}
	got, undecided := assessRecordedWith(t, gapBackend(icBuiltin("decimal")), typedSchema(t, decimal), valuedRoot("n", "1.5"))
	wantSilence(t, got, "a withheld String Valid verdict charges nothing")
	wantDeclines(t, undecided, Unevaluated{rule: ruleCvcAttribute, loc: loc(1, 10), msg: "cvc-attribute clause 3"})

	fixed := xsd.NewValueConstraint(xsd.ValueFixed, "37", nil, nil)
	both := []xsd.AttributeUse{typedUse(t, "n", icBuiltin("anySimpleType"), false, &fixed, &fixed)}
	got, undecided = assessRecordedWith(t, gapBackend(icBuiltin("string")), typedSchema(t, both), valuedRoot("n", "36"))
	wantSilence(t, got, "an undecided comparison charges nothing")
	wantDeclines(t, undecided,
		Unevaluated{rule: ruleCvcAttribute, loc: loc(1, 10), msg: "cvc-attribute clause 4"},
		Unevaluated{rule: ruleCvcAu, loc: loc(1, 10), msg: "so cvc-au is undecided"})

	dflt := xsd.NewValueConstraint(xsd.ValueDefault, "1.5", nil, nil)
	defaulted := []xsd.AttributeUse{typedUse(t, "n", icBuiltin("decimal"), false, &dflt, nil)}
	got, undecided = assessRecordedWith(t, gapBackend(icBuiltin("decimal")), typedSchema(t, defaulted),
		&testElement{name: xsd.QName{Local: "root"}, loc: loc(1, 1)})
	wantSilence(t, got, "an undecided default charges nothing")
	wantDeclines(t, undecided, Unevaluated{rule: ruleCvcComplexType, loc: loc(1, 1), msg: "cvc-complex-type clause 4"})
}

// Over a ·special· type cvc-attribute clause 4 and cvc-au are DECIDED satisfied
// where the attribute's literal and the fixed {lexical form} are byte-identical:
// one literal maps identically on both sides, however many values it may denote
// (Datatypes §3.2.1.2, §3.2.2.2; #2029). A typeless attribute with a fixed value
// is this case.
func TestSpecialFixedAttributeAgreesOnIdenticalLiterals(t *testing.T) {
	fixed := xsd.NewValueConstraint(xsd.ValueFixed, "x", nil, nil)
	for _, typ := range []xsd.QName{icBuiltin("anySimpleType"), icBuiltin("anyAtomicType")} {
		both := []xsd.AttributeUse{typedUse(t, "n", typ, false, &fixed, &fixed)}
		got, undecided := assessRecorded(t, typedSchema(t, both), valuedRoot("n", "x"))
		wantSilence(t, got, "identical literals agree against xs:"+typ.Local)
		wantDeclines(t, undecided)
	}
}

// Over a ·special· type DIFFERING literals are decided over the type's mapping
// union (Datatypes §3.2.1.2, §3.2.2.2; #2040): "1.0" agrees with a fixed "1",
// both being one xs:decimal, while "36" against a fixed "37" is a value no
// primitive or list type shares, so cvc-attribute clause 4 and cvc-au are each
// charged against the {value constraint} that demands it.
func TestSpecialFixedAttributeIsDecidedOverTheMappingUnion(t *testing.T) {
	one := xsd.NewValueConstraint(xsd.ValueFixed, "1", nil, nil)
	for _, typ := range []xsd.QName{icBuiltin("anySimpleType"), icBuiltin("anyAtomicType")} {
		both := []xsd.AttributeUse{typedUse(t, "n", typ, false, &one, &one)}
		got, undecided := assessRecorded(t, typedSchema(t, both), valuedRoot("n", "1.0"))
		wantSilence(t, got, `"1.0" and "1" are one xs:decimal against xs:`+typ.Local)
		wantDeclines(t, undecided)
	}

	fixed := xsd.NewValueConstraint(xsd.ValueFixed, "37", nil, nil)
	decl := []xsd.AttributeUse{typedUse(t, "n", icBuiltin("anySimpleType"), false, &fixed, nil)}
	got := onlyCharge(t, assessTyped(t, valuedRoot("n", "36"), decl), ruleCvcAttribute)
	if !strings.HasPrefix(got.Msg, `the ·actual value· of the attribute n is neither equal nor identical to the {value} of the fixed {value constraint} "37"`) || !strings.Contains(got.Msg, "clause 4") {
		t.Errorf("Msg = %q, want clause 4 charged against the attribute n and the fixed \"37\"", got.Msg)
	}
	use := []xsd.AttributeUse{typedUse(t, "n", icBuiltin("anySimpleType"), false, nil, &fixed)}
	got = onlyCharge(t, assessTyped(t, valuedRoot("n", "36"), use), ruleCvcAu)
	if !strings.HasPrefix(got.Msg, `the ·actual value· of the attribute n is neither equal nor identical to the {value} of the fixed {value constraint} "37" on its attribute use`) {
		t.Errorf("Msg = %q, want cvc-au charged against the attribute n and the use's fixed \"37\"", got.Msg)
	}
}

// cvc-attribute clause 4 compares ·actual values·: "01" and "1" are one
// xs:integer, so a fixed constraint written one way is satisfied by the other
// spelling, while a genuinely different value is charged.
func TestDeclarationFixedIsComparedInTheValueSpace(t *testing.T) {
	fixed := xsd.NewValueConstraint(xsd.ValueFixed, "1", nil, nil)
	uses := []xsd.AttributeUse{typedUse(t, "n", integerType(), false, &fixed, nil)}

	wantSilence(t, assessTyped(t, valuedRoot("n", "01"), uses),
		`"01" and "1" are one xs:integer value; a lexical comparison would reject this`)

	got := onlyCharge(t, assessTyped(t, valuedRoot("n", "2"), uses), "cvc-attribute")
	if !strings.Contains(got.Msg, "clause 4") || !strings.Contains(got.Msg, "attribute declaration") {
		t.Errorf("Msg = %q, want clause 4 charged against the DECLARATION's {value constraint}", got.Msg)
	}

	// A DEFAULT {value constraint} constrains a present attribute not at all:
	// clause 4 tests the fixed {variety} alone.
	dflt := xsd.NewValueConstraint(xsd.ValueDefault, "1", nil, nil)
	wantSilence(t, assessTyped(t, valuedRoot("n", "2"),
		[]xsd.AttributeUse{typedUse(t, "n", integerType(), false, &dflt, nil)}),
		"a default {value constraint} constrains no attribute that is present")
}

// cvc-au reads the USE's own {value constraint} and cvc-attribute clause 4 the
// DECLARATION's. They are independent rules over two different properties, so a
// use-only fixed value charges cvc-au alone — and an attribute disagreeing with
// both is charged twice, declaration first.
func TestUseFixedChargesCvcAuIndependently(t *testing.T) {
	useFixed := xsd.NewValueConstraint(xsd.ValueFixed, "1", nil, nil)

	got := onlyCharge(t, assessTyped(t, valuedRoot("n", "2"),
		[]xsd.AttributeUse{typedUse(t, "n", integerType(), false, nil, &useFixed)}), "cvc-au")
	if got.Loc != loc(1, 10) {
		t.Errorf("Loc = %s, want the attribute's %s", got.Loc, loc(1, 10))
	}
	if !strings.Contains(got.Msg, "attribute use") {
		t.Errorf("Msg = %q, want the USE's {value constraint} named", got.Msg)
	}

	declFixed := xsd.NewValueConstraint(xsd.ValueFixed, "1", nil, nil)
	both := assessTyped(t, valuedRoot("n", "2"),
		[]xsd.AttributeUse{typedUse(t, "n", integerType(), false, &declFixed, &useFixed)})
	rules := make([]xsderr.Rule, 0, len(both))
	for _, e := range both {
		rules = append(rules, e.Rule)
	}
	if !slices.Equal(rules, []xsderr.Rule{"cvc-attribute", "cvc-au"}) {
		t.Errorf("charged %v, want both rules, the declaration's clause 4 first", rules)
	}
}

// An attribute that is not datatype-valid has no ·actual value·, so the two
// fixed-agreement rules are not also charged for the one defect: clause 3 is
// the whole verdict.
func TestAnInvalidLexicalChargesClauseThreeAlone(t *testing.T) {
	fixed := xsd.NewValueConstraint(xsd.ValueFixed, "1", nil, nil)
	got := assessTyped(t, valuedRoot("n", "abc"),
		[]xsd.AttributeUse{typedUse(t, "n", integerType(), false, &fixed, &fixed)})
	charge := onlyCharge(t, got, "cvc-attribute")
	if !strings.Contains(charge.Msg, "clause 3") {
		t.Errorf("Msg = %q, want clause 3 alone", charge.Msg)
	}
}

// cvc-complex-type clause 4: an OPTIONAL use the instance did not match, whose
// ·effective value constraint· is not ·absent·, has that constraint's own
// {lexical form} validated against the declaration's {type definition}. The
// charge sits on the ELEMENT, since no attribute information item exists to
// carry it.
func TestDefaultedAttributeDefaultIsValidated(t *testing.T) {
	bad := xsd.NewValueConstraint(xsd.ValueDefault, "abc", nil, nil)
	root := &testElement{name: xsd.QName{Local: "root"}, loc: loc(1, 1)}

	got := onlyCharge(t, assessTyped(t, root,
		[]xsd.AttributeUse{typedUse(t, "n", integerType(), false, &bad, nil)}), "cvc-complex-type")
	if got.Loc != loc(1, 1) {
		t.Errorf("Loc = %s, want the element's %s", got.Loc, loc(1, 1))
	}
	if !strings.Contains(got.Msg, "clause 4") {
		t.Errorf("Msg = %q, want clause 4", got.Msg)
	}

	// The use's own {value constraint} wins over the declaration's, which is
	// what makes this the ·effective value constraint· (§3.5.4 key-evc) and not
	// either property read alone.
	good := xsd.NewValueConstraint(xsd.ValueDefault, "1", nil, nil)
	wantSilence(t, assessTyped(t, root,
		[]xsd.AttributeUse{typedUse(t, "n", integerType(), false, &bad, &good)}),
		"the use's own {value constraint} is the effective one")
}

// cvc-complex-type clause 4 maps a QName-valued {lexical form} under the
// namespace bindings its {value constraint} captured (value.ConstraintContext),
// never the element's, and DECIDES it: "p:a" with p bound on the constraint is
// satisfied on an element that binds no p, and with p bound nowhere in the
// schema it is charged, wrapping the Datatype Valid verdict, though the element
// binds p. Through ValidDefault, whose gate 1 answers every QName-governed
// default undecided, both rows record a clause 4 decline instead.
func TestDefaultedQNameAttributeIsDecidedUnderItsOwnBindings(t *testing.T) {
	bound := xsd.NewValueConstraint(xsd.ValueDefault, "p:a", []xsd.NamespaceBinding{xsd.NewNamespaceBinding("p", "urn:a")}, nil)
	unbound := xsd.NewValueConstraint(xsd.ValueDefault, "p:a", nil, nil)
	bindsP := &testElement{name: xsd.QName{Local: "root"}, loc: loc(1, 1), bindings: map[string]string{"p": "urn:a"}}

	got, undecided := assessRecorded(t, typedSchema(t, []xsd.AttributeUse{typedUse(t, "q", icBuiltin("QName"), false, &bound, nil)}),
		&testElement{name: xsd.QName{Local: "root"}, loc: loc(1, 1)})
	wantSilence(t, got, "the constraint's own binding resolves p")
	wantDeclines(t, undecided)

	got, undecided = assessRecorded(t, typedSchema(t, []xsd.AttributeUse{typedUse(t, "q", icBuiltin("QName"), false, &unbound, nil)}), bindsP)
	wantDeclines(t, undecided)
	charge := onlyCharge(t, got, "cvc-complex-type")
	if !strings.HasPrefix(charge.Msg, "the element root carries no attribute information item named q, and the {lexical form} \"p:a\"") || !strings.Contains(charge.Msg, "clause 4") {
		t.Errorf("Msg = %q, want clause 4 charged against the defaulted q", charge.Msg)
	}
	if cause, _ := xsderr.RuleOf(errors.Unwrap(charge)); cause != "cvc-datatype-valid" {
		t.Errorf("the charge's cause carries %q, want cvc-datatype-valid", cause)
	}
}

// The four conjuncts of ·defaulted attribute· that exclude a use from clause 4,
// each on its own. None of these charges, though every one of them carries a
// {lexical form} that is not datatype-valid.
func TestNonDefaultedAttributesEscapeClauseFour(t *testing.T) {
	bad := xsd.NewValueConstraint(xsd.ValueDefault, "abc", nil, nil)
	badFixed := xsd.NewValueConstraint(xsd.ValueFixed, "abc", nil, nil)
	bare := &testElement{name: xsd.QName{Local: "root"}, loc: loc(1, 1)}

	// clause 2: {required} = true. Its own absence is clause 3's charge, not
	// clause 4's, so the assertion is that no CLAUSE 4 charge joins it.
	got := assessTyped(t, bare, []xsd.AttributeUse{typedUse(t, "n", integerType(), true, &bad, nil)})
	charge := onlyCharge(t, got, "cvc-complex-type")
	if !strings.Contains(charge.Msg, "clause 3") {
		t.Errorf("Msg = %q, want clause 3 alone: a {required} use is never a ·defaulted attribute·", charge.Msg)
	}

	// clause 3: an ·absent· ·effective value constraint· — neither property set.
	wantSilence(t, assessTyped(t, bare, []xsd.AttributeUse{typedUse(t, "n", integerType(), false, nil, nil)}),
		"a use with no value constraint at all is not a ·defaulted attribute·")

	// clause 5: the instance DID carry a matching attribute, so the default was
	// never supplied. The present attribute is valid, so nothing else fires.
	wantSilence(t, assessTyped(t, valuedRoot("n", "42"),
		[]xsd.AttributeUse{typedUse(t, "n", integerType(), false, &badFixed, nil)}),
		"a matched declaration supplies no default")
}

// The debug log names cvc-attribute and cvc-au and the clause each settled, so
// a DECLINE is distinguishable from a PASS, which charge the same nothing.
func TestValueChargeOutcomesAreLogged(t *testing.T) {
	fixed := xsd.NewValueConstraint(xsd.ValueFixed, "1", nil, nil)
	log, visits := recordingLogger()
	v, err := New(typedSchema(t, []xsd.AttributeUse{typedUse(t, "n", integerType(), false, &fixed, &fixed)}),
		testBackend(), WithLogger(log))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	v.Assess(valuedRoot("n", "01"))

	want := []string{
		"assessing element validate.name=root validate.loc=instance.xml:1:1",
		"assessing attribute validate.name=n validate.loc=instance.xml:1:10 validate.rule=cvc-attribute validate.clause=3 validate.outcome=satisfied",
		"assessing attribute validate.name=n validate.loc=instance.xml:1:10 validate.rule=cvc-attribute validate.clause=4 validate.outcome=satisfied",
		// cvc-au names no clause: the rule is one undivided sentence.
		"assessing attribute validate.name=n validate.loc=instance.xml:1:10 validate.rule=cvc-au validate.outcome=satisfied",
	}
	if !slices.Equal(*visits, want) {
		t.Errorf("walk logged\n\t%s\nwant\n\t%s",
			strings.Join(*visits, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// cvc-attribute (§3.2.4.1) clause 5 charges an xsi:type attribute whose
// ·actual value· ·resolves· to no type definition, at the ATTRIBUTE's own
// position. It is the built-in declaration for the type attribute (§3.2.7.1)
// charging on its own: no attribute use ever matches xsi:type, so
// [walk.matchedAttribute] never sees the item and none of the clauses above
// reaches it (#1494). The prefix is BOUND, so the resolved name and the lexical
// the message carries are different strings and neither can stand in for the
// other: two arguments swapped inside the fmt.Errorf leaves every asserted
// substring present, and only pinning the opening as a prefix catches it
// (#1048).
func TestUnresolvableXSITypeChargesClauseFive(t *testing.T) {
	schema := eSchema(t, false, nil)
	root := eRoot(map[string]string{"type": "p:Missing"})
	root.bindings = map[string]string{"p": "urn:x"}

	got := cAssess(t, schema, root)

	if len(got) != 1 {
		t.Fatalf("Violations() = %v, want exactly one", got)
	}
	if got[0].Rule != "cvc-attribute" {
		t.Errorf("Rule = %q, want cvc-attribute", got[0].Rule)
	}
	if got[0].Loc != loc(1, 10) {
		t.Errorf("Loc = %s, want the xsi:type attribute's own position %s", got[0].Loc, loc(1, 10))
	}
	const opening = `the xsi:type attribute of the element root has the lexical "p:Missing", ` +
		`and the schema declares no type definition named "{urn:x}Missing" for it to ·resolve· to`
	if !strings.HasPrefix(got[0].Msg, opening) {
		t.Errorf("Msg = %q, want it to open %q", got[0].Msg, opening)
	}
	if !strings.Contains(got[0].Msg, "cvc-attribute clause 5") {
		t.Errorf("Msg = %q, want it to name cvc-attribute clause 5 inline (STYLE E4)", got[0].Msg)
	}

	wantSilence(t, cAssess(t, schema, eRoot(map[string]string{"type": "Derived"}, "a")),
		"an xsi:type that ·resolves· satisfies clause 5")
}

// Clause 5 leaves the ELEMENT where the Note under cvc-elt puts it: assessed
// against its ·selected type definition·, whose own charges arrive beside the
// attribute's and are not displaced by it. Base's {content type} is EMPTY, so
// the <a> is charged by the fallback type exactly as it is without any xsi:type
// at all.
func TestClauseFiveLeavesTheFallbackTypeGoverning(t *testing.T) {
	schema := eSchema(t, false, nil)

	got := cAssess(t, schema, eRoot(map[string]string{"type": "Missing"}, "a"))

	if len(got) != 2 {
		t.Fatalf("Violations() = %v, want two: the clause 5 charge and Base's empty {content type}", got)
	}
	if got[0].Rule != "cvc-attribute" || got[1].Rule != "cvc-complex-type" {
		t.Errorf("Rules = %q, %q, want cvc-attribute then cvc-complex-type against the SELECTED type",
			got[0].Rule, got[1].Rule)
	}
}

// The charge goes through neither arm of cvc-type clause 3, so a SIMPLE
// ·governing type definition· reaches it too — the shape #1494 was filed on,
// where the declared type is xs:string and cvc-complex-type never runs. The
// character content still validates against that fallback type, so clause 5's
// charge is the whole of what the document is told.
func TestClauseFiveChargesUnderASimpleGoverningType(t *testing.T) {
	schema := simpleTypedSchema(t, icBuiltin("string"), nil, false)

	got := cAssess(t, schema, eRoot(map[string]string{"type": "Missing"}, "#hello"))

	if len(got) != 1 {
		t.Fatalf("Violations() = %v, want exactly one: clause 5, with xs:string admitting the content", got)
	}
	if got[0].Rule != "cvc-attribute" || !strings.Contains(got[0].Msg, "cvc-attribute clause 5") {
		t.Errorf("charge = %v, want cvc-attribute clause 5", got[0])
	}
	wantSilence(t, cAssess(t, schema, eRoot(nil, "#hello")),
		"an element carrying no xsi:type at all has no ·actual value· for clause 5 to quantify over")
}

// cvc-attribute (§3.2.4.1) clause 3 charges an xsi:type lexical that is not
// String Valid against xs:QName, the built-in declaration's {type definition}
// (§3.2.7.1), and clause 5 is declined for it: such a lexical has no ·actual
// value· to ·resolve· (#1641). The first three stop at resolveInstanceQName's
// split and were charged by nothing; the last two clear it and were charged
// under clause 5 as QNames naming no type. The charge wraps the Datatype Valid
// verdict as its cause, and its opening is pinned as a prefix so that the
// element name and the lexical cannot trade places unseen (#1048).
func TestNonQNameXSITypeChargesClauseThree(t *testing.T) {
	schema := eSchema(t, false, nil)

	for _, lexical := range []string{
		"p:Derived",   // a prefix with no binding in scope
		"",            // empty
		"a:b:Derived", // a colon structure no QName has
		"23.789",      // an NCName cannot open with a digit
		"not a name",  // nor carry a space
	} {
		got, visits := cAssessLogged(t, schema, eRoot(map[string]string{"type": lexical}))

		if len(got) != 1 {
			t.Errorf("xsi:type=%q: Violations() = %v, want exactly one", lexical, got)
			continue
		}
		if got[0].Rule != "cvc-attribute" || got[0].Loc != loc(1, 10) {
			t.Errorf("xsi:type=%q: charge = %v, want cvc-attribute at the attribute's own %s", lexical, got[0], loc(1, 10))
		}
		opening := `the xsi:type attribute of the element root has the ·initial value· "` + lexical +
			`", which is not ·valid· with respect to {http://www.w3.org/2001/XMLSchema}QName`
		if !strings.HasPrefix(got[0].Msg, opening) {
			t.Errorf("xsi:type=%q: Msg = %q, want it to open %q", lexical, got[0].Msg, opening)
		}
		if !strings.Contains(got[0].Msg, "cvc-attribute clause 3") || strings.Contains(got[0].Msg, "clause 5") {
			t.Errorf("xsi:type=%q: Msg = %q, want it to name cvc-attribute clause 3 and not clause 5", lexical, got[0].Msg)
		}
		if rule, _ := xsderr.RuleOf(got[0].Unwrap()); rule != "cvc-datatype-valid" {
			t.Errorf("xsi:type=%q: cause = %v, want the cvc-datatype-valid verdict wrapped", lexical, got[0].Unwrap())
		}
		want := []string{"3/charged", "5/declined"}
		if outcomes := attributeOutcomes(*visits); !slices.Equal(outcomes, want) {
			t.Errorf("xsi:type=%q: logged %v, want %v", lexical, outcomes, want)
		}
		_, undecided := assessRecorded(t, schema, eRoot(map[string]string{"type": lexical}))
		wantDeclines(t, undecided, Unevaluated{rule: ruleCvcAttribute, loc: loc(1, 10), msg: "so clause 5 is undecided"})
	}
}

// A value backend whose xs:QName mapping reports a fault of the type rather than
// a VERDICT ([value.IsDatatypeVerdict]) leaves cvc-attribute (§3.2.4.1) clause
// 3 undecided for an xsi:type lexical, recorded as [Unevaluated] at the
// attribute, and hands the lexical to clause 5 — #774's GAP(validate) arm of
// [walk.instanceTypeLexical]. A verdict on the same lexical charges clause 3
// instead, so the arm declines faults and not every error. The schema seeds a
// simple xs:QName, so the decline is the fault arm's and not the
// no-simple-xs:QName arm's, which logs and records the same clause.
//
// Deleting the fault arm fails the fault row three times: it logs 3/charged
// 5/declined, records clause 5's "no ·actual value·" decline in place of clause
// 3's, and charges clause 3 in place of clause 5.
func TestQNameMappingFaultDeclinesClauseThree(t *testing.T) {
	schema := eSchema(t, false, nil)
	if _, simple := schema.ResolvedSimpleType(xsd.TypeDefinitionRef{Name: qnameTypeName}); !simple {
		t.Fatal("eSchema carries no simple xs:QName: the row would reach the no-simple-xs:QName arm")
	}
	const lexical = "23.789" // outside xs:QName's lexical space, so a verdict charges it
	for _, row := range []struct {
		name     string
		backend  value.Backend
		outcomes []string
		clause   string
		decline  string
	}{
		{"fault", gapBackend(qnameTypeName), []string{"3/declined", "5/charged"}, "cvc-attribute clause 5", "cvc-attribute clause 3 is undecided"},
		{"verdict", testBackend(), []string{"3/charged", "5/declined"}, "cvc-attribute clause 3", "so clause 5 is undecided"},
	} {
		got, visits := cAssessLoggedWith(t, row.backend, schema, eRoot(map[string]string{"type": lexical}))

		if outcomes := attributeOutcomes(*visits); !slices.Equal(outcomes, row.outcomes) {
			t.Errorf("%s: logged %v, want %v", row.name, outcomes, row.outcomes)
		}
		if len(got) != 1 || got[0].Rule != "cvc-attribute" || got[0].Loc != loc(1, 10) || !strings.Contains(got[0].Msg, row.clause) {
			t.Errorf("%s: Violations() = %v, want exactly one cvc-attribute charge at %s naming %q", row.name, got, loc(1, 10), row.clause)
		}
		_, undecided := assessRecordedWith(t, row.backend, schema, eRoot(map[string]string{"type": lexical}))
		wantDeclines(t, undecided, Unevaluated{rule: ruleCvcAttribute, loc: loc(1, 10), msg: row.decline})
	}
}

// A lexical in xs:QName's lexical space satisfies clause 3 and reaches clause 5
// alone: one naming no type is charged there and nowhere else, and one naming a
// type the schema carries is charged nowhere. The prefix is bound, so clause 3
// maps it under the element's bindings exactly as clause 5 resolves it.
func TestQNameXSITypeSatisfiesClauseThree(t *testing.T) {
	schema := eSchema(t, false, nil)
	root := eRoot(map[string]string{"type": "xs:unknownType"})
	root.bindings = map[string]string{"xs": xsd.XMLSchemaNS}

	got, visits := cAssessLogged(t, schema, root)

	if len(got) != 1 || !strings.Contains(got[0].Msg, "cvc-attribute clause 5") {
		t.Fatalf("Violations() = %v, want exactly one, under cvc-attribute clause 5", got)
	}
	if want := []string{"3/satisfied", "5/charged"}; !slices.Equal(attributeOutcomes(*visits), want) {
		t.Errorf("logged %v, want %v", attributeOutcomes(*visits), want)
	}

	got, visits = cAssessLogged(t, schema, eRoot(map[string]string{"type": "Derived"}, "a"))
	wantSilence(t, got, "an xsi:type that is a QName and ·resolves· satisfies clauses 3 and 5")
	if want := []string{"3/satisfied", "5/satisfied"}; !slices.Equal(attributeOutcomes(*visits), want) {
		t.Errorf("logged %v, want %v", attributeOutcomes(*visits), want)
	}
}

// attributeOutcomes is the clause/outcome pair of every cvc-attribute line the
// walk logged, in order.
func attributeOutcomes(visits []string) []string {
	outcomes := []string{}
	for _, line := range visits {
		if field(line, "validate.rule=") != "cvc-attribute" {
			continue
		}
		outcomes = append(outcomes, field(line, "validate.clause=")+"/"+field(line, "validate.outcome="))
	}
	return outcomes
}

// wildAttrSchema is the fixture for an attribute ·attributed to· an {attribute
// wildcard}: <root>'s RootType carries a ##any {attribute wildcard} whose
// {process contents} is pc, and a {content type} of one optional element
// particle <opaque> — a local declaration with an ·absent· {type definition},
// so a type this package cannot determine — then a lax element wildcard,
// 0..unbounded, that admits an undeclared child to be ·laxly assessed·. Two
// top-level attribute declarations are what a wildcard-attributed name
// ·resolves· to:
//
//	n  xs:integer
//	f  xs:string, fixed "x"
func wildAttrSchema(t *testing.T, pc xsd.ProcessContents) *xsd.Schema {
	t.Helper()
	o, err := xsd.NewUnboundedOccurs(xsderr.Loc{}, 0)
	if err != nil {
		t.Fatalf("NewUnboundedOccurs: %v", err)
	}
	wild, err := xsd.NewParticle(xsderr.Loc{}, o, xsd.ResolvedTerm{Term: *anyWildcard(t, xsd.ProcessLax)})
	if err != nil {
		t.Fatalf("NewParticle: %v", err)
	}
	ct, err := xsd.NewComplexType(xsderr.Loc{}, local("RootType"), xsd.QName{}, nil,
		xsd.DerivationRestriction, false, nil, nil, anyWildcard(t, pc),
		cSequence(t, false, cParticle(t, "opaque", 0, 1), wild), nil, nil)
	if err != nil {
		t.Fatalf("building RootType: %v", err)
	}
	fixed := xsd.NewValueConstraint(xsd.ValueFixed, "x", nil, nil)
	decls := []struct {
		name string
		typ  xsd.QName
		vc   *xsd.ValueConstraint
	}{{"n", integerType(), nil}, {"f", icBuiltin("string"), &fixed}}
	return cSchemaFrom(t, ct, func(b *xsd.SchemaBuilder) {
		for _, st := range icSeeded(t) {
			b.AddType(st)
		}
		for _, d := range decls {
			decl, err := xsd.NewAttributeDeclaration(xsderr.Loc{}, local(d.name),
				xsd.TypeDefinitionRef{Name: d.typ}, xsd.NewAttributeGlobalScope(), d.vc, false)
			if err != nil {
				t.Fatalf("building the top-level %s attribute declaration: %v", d.name, err)
			}
			b.AddAttribute(decl)
		}
	})
}

// wantAttributeCharge fails unless got is exactly one cvc-attribute charge at
// the attribute's position at, whose message OPENS with prefix: pinning the
// opening rather than a substring is what ties the charge to the attribute it
// names (#1048).
func wantAttributeCharge(t *testing.T, got []*xsderr.Error, at xsderr.Loc, prefix string) {
	t.Helper()
	charge := onlyCharge(t, got, ruleCvcAttribute)
	if charge.Loc != at {
		t.Errorf("Loc = %s, want the attribute's %s", charge.Loc, at)
	}
	if !strings.HasPrefix(charge.Msg, prefix) {
		t.Errorf("Msg = %q, want it to open with %q", charge.Msg, prefix)
	}
}

// The openings of the two cvc-attribute charges a wildcard-attributed
// attribute can draw: clause 3 against n, clause 4 against f.
const (
	clause3OnN = "the ·initial value· of the attribute n is not ·valid· with respect to its declaration's {type definition}"
	clause4OnF = `the ·actual value· of the attribute f is neither equal nor identical to the {value} of the fixed {value constraint} "x" on its attribute declaration`
)

// An attribute ·attributed to· a strict or lax {attribute wildcard}
// (cvc-complex-type clause 2.2) has for its ·governing attribute declaration·
// the top-level one its ·expanded name· ·resolves· to (key-governing-ad clause
// 3), and key-sva (§3.3.4.6) clause 2.1 assesses it against that declaration:
// cvc-attribute (§3.2.4.1) clause 3 for a lexical outside its {type definition}
// and clause 4 for a fixed {value constraint} it disagrees with. Under skip the
// item is ·skipped· and nothing is assessed, and a name resolving no
// declaration has no governing declaration under strict either (clause 2.2;
// e-validity clause 1.1.3 names only a ·wildcard particle·).
//
// The charged rows fail with [walk.unmatchedAttribute]'s call to
// [walk.wildcardAttribute] removed; the silent rows guard against
// over-charging (#1891).
func TestWildcardAttributedAttributeIsAssessedAgainstItsResolvedDeclaration(t *testing.T) {
	for _, pc := range []xsd.ProcessContents{xsd.ProcessStrict, xsd.ProcessLax} {
		schema := wildAttrSchema(t, pc)
		t.Run(pc.String()+"/clause 3 charged", func(t *testing.T) {
			wantAttributeCharge(t, icAssess(t, schema, valuedRoot("n", "abc")), loc(1, 10), clause3OnN)
		})
		t.Run(pc.String()+"/clause 4 charged", func(t *testing.T) {
			wantAttributeCharge(t, icAssess(t, schema, valuedRoot("f", "y")), loc(1, 10), clause4OnF)
		})
		t.Run(pc.String()+"/silent", func(t *testing.T) {
			wantSilence(t, icAssess(t, schema, valuedRoot("n", "12")), "12 is an xs:integer")
			wantSilence(t, icAssess(t, schema, valuedRoot("f", "x")), "x agrees with the fixed x")
			wantSilence(t, icAssess(t, schema, valuedRoot("m", "abc")),
				"m resolves no declaration, so it has no ·governing attribute declaration·")
		})
	}

	t.Run("skip/silent", func(t *testing.T) {
		skip := wildAttrSchema(t, xsd.ProcessSkip)
		wantSilence(t, icAssess(t, skip, valuedRoot("n", "abc")), "a ·skipped· attribute is not assessed")
		wantSilence(t, icAssess(t, skip, valuedRoot("f", "y")), "a ·skipped· attribute is not assessed")
	})
}

// An element ·laxly assessed· (key-lva) — an undeclared <unknown> the lax
// element wildcard admitted — is locally validated against xs:anyType, whose
// {attribute wildcard} is lax over every namespace (§3.4.7), so its attributes
// are assessed against the declarations their names resolve to, as under a
// declared type's lax wildcard. <root>'s own {attribute wildcard} is skip here,
// so nothing but xs:anyType's can reach them. An element whose type this
// package could not determine — <opaque> — is not ·laxly assessed· and decides
// nothing about its attributes.
//
// The charged rows fail with [walk.attribute]'s lax arm reduced to the
// undetermined one; the <opaque> row fails with the lax arm taken for every
// element with no complex governing type (#1891).
func TestLaxlyAssessedElementAttributesAreAssessedAgainstTheirResolvedDeclarations(t *testing.T) {
	schema := wildAttrSchema(t, xsd.ProcessSkip)
	root := func(child, attr, value string) *testElement {
		return icElem(xsd.QName{Local: "root"}, 1, nil,
			ElementChild(icElem(xsd.QName{Local: child}, 2, []Attribute{icAttr(local(attr), value, 2)})))
	}

	t.Run("clause 3 charged", func(t *testing.T) {
		wantAttributeCharge(t, icAssess(t, schema, root("unknown", "n", "abc")), loc(2, 2), clause3OnN)
	})
	t.Run("clause 4 charged", func(t *testing.T) {
		wantAttributeCharge(t, icAssess(t, schema, root("unknown", "f", "y")), loc(2, 2), clause4OnF)
	})
	t.Run("silent", func(t *testing.T) {
		wantSilence(t, icAssess(t, schema, root("unknown", "n", "12")), "12 is an xs:integer")
		wantSilence(t, icAssess(t, schema, root("unknown", "m", "abc")),
			"m resolves no declaration, and xs:anyType's wildcard is lax")
	})
	t.Run("undetermined silent", func(t *testing.T) {
		wantSilence(t, icAssess(t, schema, root("opaque", "n", "abc")),
			"an element whose type was not determined decides nothing about its attributes")
	})
}

// xsiNilAttr is an xsi:nil attribute carrying lexical, at 4:20.
func xsiNilAttr(lexical string) Attribute {
	return &testAttribute{name: xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "nil"}, value: lexical, loc: loc(4, 20)}
}

// cvc-attribute (§3.2.4.1) clause 3 charges an xsi:nil lexical outside
// xs:boolean's lexical space, String Valid against the built-in declaration for
// the nil attribute (§3.2.7.2), on an element with no ·governing element
// declaration·: key-governing-ad keeps the attribute governed by that
// declaration however the element is assessed. One row per shape: ·laxly
// assessed· (key-lva); ·strictly assessed· against an xsi:type with no
// ·locally declared type· (key-governing-type-elem clause 8); and an {open
// content} child against its ·locally declared type· or an xsi:type
// ·overriding· it (clauses 7 and 6). Every charged row fails with
// [walk.attributes]'s call to [walk.instanceNilLexical] deleted, no violation
// being charged at all; the silent lexicals guard against charging one
// xs:boolean admits (#2061). The opening is pinned as a prefix so that the
// element name and the lexical cannot trade places unseen (#1048).
func TestDeclarationlessXSINilChargesClauseThree(t *testing.T) {
	typed := func(lexical string) []Attribute { return []Attribute{xsiTypeAttr("xs:int"), xsiNilAttr(lexical)} }
	for _, tc := range []struct {
		why    string
		schema *xsd.Schema
		name   string
		doc    func(lexical string) *testElement
	}{
		{"·laxly assessed· under a lax Wildcard", ldtSchema(t, "xs:date", "xs:date", "lax"), "u",
			func(lexical string) *testElement { return ldtDoc("2008-11-03", "", "u", "", xsiNilAttr(lexical)) }},
		{"xsi:typed under a strict Wildcard", ldtSchema(t, "xs:date", "xs:date", "strict"), "u",
			func(lexical string) *testElement { return ldtDoc("2008-11-03", "", "u", "1", typed(lexical)...) }},
		{"xsi:typed under a lax Wildcard", ldtSchema(t, "xs:date", "xs:date", "lax"), "u",
			func(lexical string) *testElement { return ldtDoc("2008-11-03", "", "u", "1", typed(lexical)...) }},
		{"the ·locally declared type· of an {open content} child", ldtOpenSchema(t, "xs:date", "xs:date"), "e",
			func(lexical string) *testElement {
				return ldtDoc("2008-11-03", "", "e", "2008-11-04", xsiNilAttr(lexical))
			}},
		{"an xsi:type ·overriding· an {open content} child's ·locally declared type·", ldtOpenSchema(t, "xs:date", "xs:date"), "e",
			func(lexical string) *testElement {
				return ldtDoc("2008-11-03", "", "e", "2008-11-04", xsiTypeAttr("xs:date"), xsiNilAttr(lexical))
			}},
	} {
		t.Run(tc.why, func(t *testing.T) {
			got, unevaluated := assessRecorded(t, tc.schema, tc.doc("maybe"))
			wantAttributeCharge(t, got, loc(4, 20), `the xsi:nil attribute of the element `+tc.name+
				` has the ·initial value· "maybe", which is not ·valid· with respect to xs:boolean`)
			if !strings.Contains(got[0].Msg, "cvc-attribute clause 3") {
				t.Errorf("Msg = %q, want it to name cvc-attribute clause 3 inline (STYLE E4)", got[0].Msg)
			}
			if len(unevaluated) != 0 {
				t.Errorf("Unevaluated() = %v, want none", unevaluated)
			}
			for _, lexical := range []string{"false", "0", " \t true \n "} {
				got, unevaluated := assessRecorded(t, tc.schema, tc.doc(lexical))
				wantSilence(t, got, "a collapsed boolean literal has an ·actual value·, and no declaration makes the element ·nilled·")
				if len(unevaluated) != 0 {
					t.Errorf("xsi:nil=%q: Unevaluated() = %v, want none", lexical, unevaluated)
				}
			}
		})
	}
}
