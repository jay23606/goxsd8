package validate

import (
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The fixtures below drive the ATTRIBUTE-EXISTENCE half of cvc-complex-type
// (§3.4.4.2) — clauses 2 and 3, which need no value space — over
// the validation root, which is the only element whose ·governing type
// definition· this package determines. Every schema is built through the
// exported constructors: the producer is not involved, so a shape it cannot
// yet emit (an attribute wildcard) is still reachable here. No declaration
// here names a {type definition}, which is what keeps the value charges out —
// they live in cvcattribute_test.go, over schemas that seed the builtins.

// aUse builds an attribute use over a sibling local declaration named local
// in no namespace, with the given {required} and the use's own {value
// constraint} — the one cvc-au (§3.5.4) reads, not the declaration's. The
// declaration carries NO {type definition}, so every value charge declines on
// it; typedUse (cvcattribute_test.go) is the one that names a type.
func aUse(t *testing.T, local string, required bool, vc *xsd.ValueConstraint) xsd.AttributeUse {
	t.Helper()
	decl, err := xsd.NewAttributeDeclaration(xsderr.Loc{}, xsd.QName{Local: local}, nil,
		xsd.NewAttributeGlobalScope(), nil, false)
	if err != nil {
		t.Fatalf("building the %s attribute declaration: %v", local, err)
	}
	u, err := xsd.NewAttributeUse(xsderr.Loc{}, required,
		xsd.LocalAttributeDeclaration{Declaration: decl}, vc, nil)
	if err != nil {
		t.Fatalf("building the %s attribute use: %v", local, err)
	}
	return u
}

// anyWildcard is the ·complete wildcard· of the given {process contents}:
// {variety} any, so clause 2.2.1 is satisfied for every name and only
// cvc-wildcard's own clauses could reject. {process contents} is a parameter
// because it decides what an admitted attribute is assessed against
// (§3.10.4.1), which is a different question from admission and the one
// icWildcardSchema varies.
func anyWildcard(t *testing.T, pc xsd.ProcessContents) *xsd.Wildcard {
	t.Helper()
	c, err := xsd.NewNamespaceConstraint(xsderr.Loc{}, xsd.NamespaceConstraintAny, nil, nil, nil)
	if err != nil {
		t.Fatalf("building the namespace constraint: %v", err)
	}
	w, err := xsd.NewWildcard(xsderr.Loc{}, c, pc)
	if err != nil {
		t.Fatalf("building the %s wildcard: %v", pc, err)
	}
	return &w
}

// nsWildcard is a wildcard whose {namespace constraint} admits the listed
// namespace names and nothing else ({variety} enumeration), so cvc-wildcard
// clause 1 rejects every other ·expanded name·.
func nsWildcard(t *testing.T, pc xsd.ProcessContents, uris ...string) *xsd.Wildcard {
	t.Helper()
	spaces := make([]xsd.Namespace, 0, len(uris))
	for _, u := range uris {
		spaces = append(spaces, xsd.NamespaceName(u))
	}
	c, err := xsd.NewNamespaceConstraint(xsderr.Loc{}, xsd.NamespaceConstraintEnumeration, spaces, nil, nil)
	if err != nil {
		t.Fatalf("building the enumeration namespace constraint: %v", err)
	}
	w, err := xsd.NewWildcard(xsderr.Loc{}, c, pc)
	if err != nil {
		t.Fatalf("building the %s wildcard: %v", pc, err)
	}
	return &w
}

// definedKeywordWildcard is the ·complete wildcard· carrying ##defined in its
// {disallowed names}, which cvc-wildcard clause 2.2 resolves against the
// finalized schema's top-level {attribute declarations}.
func definedKeywordWildcard(t *testing.T, pc xsd.ProcessContents) *xsd.Wildcard {
	t.Helper()
	c, err := xsd.NewNamespaceConstraint(xsderr.Loc{}, xsd.NamespaceConstraintAny, nil, nil,
		[]xsd.DisallowedNameKeyword{xsd.DisallowedNameDefined})
	if err != nil {
		t.Fatalf("building the ##defined namespace constraint: %v", err)
	}
	w, err := xsd.NewWildcard(xsderr.Loc{}, c, pc)
	if err != nil {
		t.Fatalf("building the %s wildcard: %v", pc, err)
	}
	return &w
}

// governedSchema declares "root" governed by a complex type carrying uses and
// wildcard. The type's {base type definition} is left ·absent·: the fold that
// would inherit a base's uses is finalize's, and a type with no base carries
// exactly the uses named here.
func governedSchema(t *testing.T, uses []xsd.AttributeUse, wildcard *xsd.Wildcard) *xsd.Schema {
	t.Helper()
	return governedSchemaDeclaring(t, uses, wildcard)
}

// governedSchemaDeclaring is governedSchema plus a top-level Attribute
// Declaration for each name, which is what cvc-wildcard clause 2.2's ##defined
// keyword ·resolves· against.
func governedSchemaDeclaring(t *testing.T, uses []xsd.AttributeUse, wildcard *xsd.Wildcard, declared ...xsd.QName) *xsd.Schema {
	t.Helper()
	ct, err := xsd.NewComplexType(xsderr.Loc{}, xsd.QName{Local: "RootType"}, xsd.QName{}, nil,
		xsd.DerivationRestriction, false, attrContent(uses), nil, wildcard, xsd.EmptyContent{}, nil, nil)
	if err != nil {
		t.Fatalf("building RootType: %v", err)
	}
	e, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: "root"},
		xsd.TypeDefinitionRef{Name: xsd.QName{Local: "RootType"}}, nil, xsd.NewGlobalScope(),
		nil, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("building the root element declaration: %v", err)
	}
	b := xsd.NewSchemaBuilder()
	b.AddType(ct)
	b.AddElement(e)
	for _, n := range declared {
		d, err := xsd.NewAttributeDeclaration(xsderr.Loc{}, n, nil, xsd.NewAttributeGlobalScope(), nil, false)
		if err != nil {
			t.Fatalf("building the top-level %s attribute declaration: %v", n, err)
		}
		b.AddAttribute(d)
	}
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the governed schema: %v", err)
	}
	return schema
}

// attributedRoot is a childless <root> carrying the named attributes in
// source order, each at a distinct Loc.
func attributedRoot(names ...xsd.QName) *testElement {
	e := &testElement{name: xsd.QName{Local: "root"}, loc: loc(1, 1)}
	for i, n := range names {
		e.attrs = append(e.attrs, &testAttribute{name: n, value: "v", loc: loc(1, 10+i)})
	}
	return e
}

func local(name string) xsd.QName { return xsd.QName{Local: name} }

// assessRoot assesses root against a schema declaring "root" over uses and
// wildcard, and returns the violations charged.
func assessRoot(t *testing.T, root Element, uses []xsd.AttributeUse, wildcard *xsd.Wildcard) []*xsderr.Error {
	t.Helper()
	return assessSchema(t, governedSchema(t, uses, wildcard), root)
}

// assessSchema is assessRoot over a schema the caller built, for the fixtures
// that need more in it than governedSchema's element and type.
func assessSchema(t *testing.T, schema *xsd.Schema, root Element) []*xsderr.Error {
	t.Helper()
	v, err := New(schema, testBackend())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res := v.Assess(root)
	if res.Err() != nil {
		t.Fatalf("Err() = %v, want nil", res.Err())
	}
	return res.Violations()
}

// wantSilence fails unless the assessment charged nothing. It is the
// assertion every DECLINE shares with every PASS — the two are
// indistinguishable in a Result, which is exactly why conformance/instance.go
// reads an empty Result as a decline and never as "valid".
func wantSilence(t *testing.T, got []*xsderr.Error, why string) {
	t.Helper()
	if len(got) != 0 {
		t.Errorf("Violations() = %v, want none: %s", got, why)
	}
}

// assessOutcomes assesses root and returns the clause/outcome pair the debug
// log recorded for each attribute, in visit order. It is how a DECLINE is
// told apart from a PASS at all: both charge nothing, so the log is the only
// place the difference is observable (STYLE L1) and the only place a test can
// pin which branch ran.
func assessOutcomes(t *testing.T, root Element, uses []xsd.AttributeUse, wildcard *xsd.Wildcard) []string {
	t.Helper()
	log, visits := recordingLogger()
	v, err := New(governedSchema(t, uses, wildcard), testBackend(), WithLogger(log))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	v.Assess(root)
	outcomes := []string{}
	for _, line := range *visits {
		if !strings.HasPrefix(line, "assessing attribute ") {
			continue
		}
		clause, outcome := field(line, "validate.clause="), field(line, "validate.outcome=")
		outcomes = append(outcomes, clause+"/"+outcome)
	}
	return outcomes
}

// field pulls one space-delimited key=value out of a recorded log line, or
// "-" where the line does not carry that key.
func field(line, key string) string {
	for _, f := range strings.Fields(line) {
		if after, ok := strings.CutPrefix(f, key); ok {
			return after
		}
	}
	return "-"
}

// wantCharge fails unless the assessment charged exactly one
// cvc-complex-type violation naming clause and citing at.
func wantCharge(t *testing.T, got []*xsderr.Error, clause string, at xsderr.Loc, names ...string) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("Violations() = %v, want exactly one", got)
	}
	if got[0].Rule != "cvc-complex-type" {
		t.Errorf("Rule = %q, want cvc-complex-type", got[0].Rule)
	}
	if !strings.Contains(got[0].Msg, clause) {
		t.Errorf("Msg = %q, want it to name %q — the catalog cannot carry a dotted rule ID", got[0].Msg, clause)
	}
	if got[0].Loc != at {
		t.Errorf("Loc = %s, want %s", got[0].Loc, at)
	}
	for _, n := range names {
		if !strings.Contains(got[0].Msg, n) {
			t.Errorf("Msg = %q, want it to name %s", got[0].Msg, n)
		}
	}
}

// Clause 3 is an existence test over ·expanded names·: a required use with a
// matching attribute information item present is satisfied, and clause 2.1
// then settles that attribute through a cvc-au that constrains nothing.
func TestRequiredAttributePresentIsNotCharged(t *testing.T) {
	got := assessRoot(t, attributedRoot(local("id")), []xsd.AttributeUse{aUse(t, "id", true, nil)}, nil)
	wantSilence(t, got, "a required attribute use with its attribute present satisfies clauses 2.1 and 3")
}

// Clause 3: a {required} use with no attribute information item of that
// ·expanded name· is charged, against the ELEMENT's position — there is no
// missing attribute to cite one for.
func TestRequiredAttributeAbsentChargesClauseThree(t *testing.T) {
	got := assessRoot(t, attributedRoot(), []xsd.AttributeUse{aUse(t, "id", true, nil)}, nil)
	wantCharge(t, got, "clause 3", loc(1, 1), "id", "root")
}

// An OPTIONAL use with no matching attribute is silent: clause 3 quantifies
// over {required} uses alone, and the ·defaulted attribute· an absent
// optional use produces is a §3.4.5.1 infoset contribution, not a violation.
func TestOptionalAttributeAbsentIsNotCharged(t *testing.T) {
	got := assessRoot(t, attributedRoot(), []xsd.AttributeUse{aUse(t, "id", false, nil)}, nil)
	wantSilence(t, got, "clause 3 says nothing about an optional use")
}

// Clause 2 has no third case: an attribute matching no attribute use falls
// outside case 2.1 into case 2.2, and with no {attribute wildcard} on its type
// that case fails at its first conjunct, clause 2.2.1, so clause 2 is violated
// outright — decidable with no datatype backend at all. The charge cites the
// ATTRIBUTE's position and names cvc-complex-type clause 2.2.1, the conjunct
// that fails.
func TestUndeclaredAttributeWithNoWildcardChargesClauseTwo(t *testing.T) {
	got := assessRoot(t, attributedRoot(local("stray")), []xsd.AttributeUse{aUse(t, "id", false, nil)}, nil)
	wantCharge(t, got, "cvc-complex-type clause 2.2.1", loc(1, 10), "stray")
}

// The same attribute against a type carrying an {attribute wildcard} that
// ADMITS its ·expanded name· satisfies clause 2.2 and is not charged: clause 2.2
// is a conjunction, 2.2.1 holds on the wildcard's presence and 2.2.2 on
// cvc-wildcard (§3.10.4.1), which the ·complete wildcard· here passes for every
// name. The control row is what keeps the silence honest — the identical
// document IS charged when the wildcard is gone, so silence here comes from the
// wildcard and not from an accidental match.
func TestUndeclaredAttributeAdmittedByWildcardIsNotCharged(t *testing.T) {
	uses := []xsd.AttributeUse{aUse(t, "id", false, nil)}
	got := assessRoot(t, attributedRoot(local("stray")), uses, anyWildcard(t, xsd.ProcessStrict))
	wantSilence(t, got, "clause 2.2 is satisfied: the wildcard admits the name")
	if outcomes := assessOutcomes(t, attributedRoot(local("stray")), uses, anyWildcard(t, xsd.ProcessStrict)); !slices.Equal(outcomes, []string{"2.2/satisfied"}) {
		t.Errorf("assessed %v, want the attribute SATISFIED under clause 2.2 — not matched to a use", outcomes)
	}

	control := assessRoot(t, attributedRoot(local("stray")), uses, nil)
	if len(control) != 1 {
		t.Fatalf("the control charged %v, want exactly one: the silence above proves nothing otherwise", control)
	}
}

// An attribute the {attribute wildcard} does NOT admit is charged clause 2:
// 2.2.1 holds on the wildcard's presence and 2.2.2 fails on cvc-wildcard
// (§3.10.4.1) clause 1, the ·expanded name· being outside the {namespace
// constraint}. The charge cites the attribute's own position and names 2.2.2, so
// it is distinguishable from the no-wildcard charge, which names 2.2.1 (#717).
func TestUndeclaredAttributeOutsideTheWildcardChargesClauseTwoTwoTwo(t *testing.T) {
	uses := []xsd.AttributeUse{aUse(t, "id", false, nil)}
	wild := nsWildcard(t, xsd.ProcessStrict, "urn:elsewhere")
	stray := xsd.QName{Space: "urn:here", Local: "stray"}

	got := assessRoot(t, attributedRoot(stray), uses, wild)
	wantCharge(t, got, "clause 2.2.2", loc(1, 10), "stray")

	// The same wildcard admits the name it was built for, so the charge above
	// comes from the ·expanded name· and not from the wildcard's mere shape.
	wantSilence(t, assessRoot(t, attributedRoot(xsd.QName{Space: "urn:elsewhere", Local: "stray"}), uses, wild),
		"the wildcard's {namespace constraint} admits this name")
}

// cvc-wildcard clause 2.2 is decided too, not only clause 1: an attribute
// wildcard whose {disallowed names} carries the ##defined keyword does not admit
// a name that ·resolves· to a top-level attribute declaration, so clause 2.2.2
// of cvc-complex-type fails for exactly the names the schema declares
// (xsd.Schema.AllowsAttributeWildcardName).
func TestWildcardWithDefinedKeywordExcludesADeclaredAttributeName(t *testing.T) {
	uses := []xsd.AttributeUse{aUse(t, "id", false, nil)}
	wild := definedKeywordWildcard(t, xsd.ProcessStrict)
	schema := governedSchemaDeclaring(t, uses, wild, local("top"))

	wantCharge(t, assessSchema(t, schema, attributedRoot(local("top"))), "clause 2.2.2", loc(1, 10), "top")
	wantSilence(t, assessSchema(t, schema, attributedRoot(local("undeclared"))),
		"##defined excludes only the names the schema declares at the top level")
}

// A wildcard does not silence clause 3: the missing required use is charged
// whatever the type admits beyond its {attribute uses}, since clause 3 reads
// only the uses.
func TestWildcardDoesNotSilenceClauseThree(t *testing.T) {
	got := assessRoot(t, attributedRoot(), []xsd.AttributeUse{aUse(t, "id", true, nil)}, anyWildcard(t, xsd.ProcessStrict))
	wantCharge(t, got, "clause 3", loc(1, 1), "id")
}

// aUse's declaration carries no {type definition} at all, so every attribute
// matched to one declines at cvc-attribute clause 3 — before cvc-au is
// reached, whatever {value constraint} the use carries. The fixed and default
// rows are the same silence for that one reason, which is what makes this the
// control for the typed charges in cvcattribute_test.go: those fire only
// because their declaration names a type this backend governs. The decline is
// RECORDED as an [Unevaluated] at the attribute, and not only logged.
func TestMatchedAttributeWithoutATypeIsDeclined(t *testing.T) {
	fixed := xsd.NewValueConstraint(xsd.ValueFixed, "1", nil, nil)
	dflt := xsd.NewValueConstraint(xsd.ValueDefault, "1", nil, nil)
	for _, vc := range []*xsd.ValueConstraint{nil, &fixed, &dflt} {
		uses := []xsd.AttributeUse{aUse(t, "id", true, vc)}
		got, undecided := assessRecorded(t, governedSchema(t, uses, nil), attributedRoot(local("id")))
		wantSilence(t, got, "a declaration with no {type definition} yields no ·actual value· to judge")
		wantDeclines(t, undecided, Unevaluated{rule: ruleCvcAttribute, loc: loc(1, 10), msg: "cvc-attribute clause 3"})
		if outcomes := assessOutcomes(t, attributedRoot(local("id")), uses, nil); !slices.Equal(outcomes, []string{"3/declined"}) {
			t.Errorf("assessed %v, want cvc-attribute clause 3 DECLINED", outcomes)
		}
	}
}

// The four xsi: attributes of §3.2.7 are excepted from clause 2's quantifier
// by its own text, so one present but declared by no attribute use is not
// charged — the arm that would charge it never runs.
func TestInstanceAttributesAreExemptFromClauseTwo(t *testing.T) {
	xsi := func(name string) xsd.QName {
		return xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: name}
	}
	// All FOUR are excepted, xsi:type and xsi:nil included: each is ·attributed
	// to· nothing at all, whatever it goes on to decide elsewhere. xsi:type
	// records two lines before that exemption and not in place of it — its own
	// cvc-attribute clauses 3 and 5, which are the built-in declaration's and
	// leave the item as unmatched by clause 2 as the other three. Clause 3
	// declines: this schema seeds no xs:QName for it to read the lexical
	// against.
	for _, n := range []string{"type", "nil", "schemaLocation", "noNamespaceSchemaLocation"} {
		want := []string{"2/exempt"}
		if n == "type" {
			want = []string{"3/declined", "5/charged", "2/exempt"}
		}
		uses := []xsd.AttributeUse{aUse(t, "id", false, nil)}
		if outcomes := assessOutcomes(t, attributedRoot(xsi(n)), uses, nil); !slices.Equal(outcomes, want) {
			t.Errorf("assessed xsi:%s as %v, want %v — EXEMPT from clause 2 rather than matched", n, outcomes, want)
		}
	}
	// xsi:type's clause 3 decline is recorded at the attribute, not only logged.
	_, undecided := assessRecorded(t, governedSchema(t, []xsd.AttributeUse{aUse(t, "id", false, nil)}, nil),
		attributedRoot(xsi("type")))
	wantDeclines(t, undecided, Unevaluated{rule: ruleCvcAttribute, loc: loc(1, 10), msg: "cvc-attribute clause 3"})
	// Two of them charge nothing anywhere, and the other two are charged by
	// rules that are not clause 2: this root's declaration is not {nillable},
	// so cvc-elt clause 3.1 charges xsi:nil's mere presence (cvcelt_test.go),
	// and attributedRoot's "v" ·resolves· to no type definition, so
	// cvc-attribute clause 5 charges xsi:type (cvcattribute_test.go).
	for _, n := range []string{"schemaLocation", "noNamespaceSchemaLocation"} {
		uses := []xsd.AttributeUse{aUse(t, "id", false, nil)}
		wantSilence(t, assessRoot(t, attributedRoot(xsi(n)), uses, nil), "xsi:"+n+" is excepted from clause 2")
	}
	// An xsi:-namespaced attribute that is NOT one of the four is inside the
	// quantifier and charged like any other, so the exemption is by name and
	// not by namespace.
	got := assessRoot(t, attributedRoot(xsi("invented")), []xsd.AttributeUse{aUse(t, "id", false, nil)}, nil)
	wantCharge(t, got, "clause 2", loc(1, 10), "invented")
}

// An xsi:type naming a type nothing declares ·resolves· to no
// ·instance-specified type definition· at all, so the ·selected type definition·
// stays the ·governing· one (key-governing-type-elem clause 4) and the attribute
// half runs against it unchanged — the fallback the Note under cvc-elt states.
// The attribute's OWN cvc-attribute clause 5 charge rides alongside that
// fallback and does not displace it (cvcattribute_test.go).
func TestUnresolvableXSITypeStillAssessesAgainstTheDeclaredType(t *testing.T) {
	root := attributedRoot(xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "type"}, local("stray"))
	got := assessRoot(t, root, []xsd.AttributeUse{aUse(t, "id", false, nil)}, nil)
	if len(got) != 2 {
		t.Fatalf("Violations() = %v, want two: cvc-attribute clause 5 and the fallback type's clause 2", got)
	}
	if got[0].Rule != "cvc-attribute" {
		t.Errorf("Rule = %q, want the first charge to be cvc-attribute against the xsi:type item", got[0].Rule)
	}
	wantCharge(t, got[1:], "clause 2", loc(1, 11), "stray")
}

// A declaration carrying a {type table} whose {test} falls outside the
// §3.12.6 required subset withholds the assessment: the ·selected type
// definition· is whichever <alternative> ·conditionally selects· one for this
// instance (§3.3.4.2), that one cannot be decided, and D.{type definition} is
// only the table's fallback and not its answer. A {test} the engine CAN
// evaluate selects a type instead (validate/cta_test.go).
func TestUnevaluableTypeTableDeclinesTheAttributeHalf(t *testing.T) {
	test := xsd.NewXPathExpression("count(@id) > 0", nil, nil, nil)
	alt := namedTypeAlternative(t, &test, xsd.QName{Local: "RootType"})
	table, err := xsd.NewTypeTable(xsderr.Loc{}, []xsd.TypeAlternative{alt},
		namedTypeAlternative(t, nil, xsd.QName{Local: "RootType"}))
	if err != nil {
		t.Fatalf("building the type table: %v", err)
	}
	ct, err := xsd.NewComplexType(xsderr.Loc{}, xsd.QName{Local: "RootType"}, xsd.QName{}, nil,
		xsd.DerivationRestriction, false, attrContent([]xsd.AttributeUse{aUse(t, "id", true, nil)}), nil, nil,
		xsd.EmptyContent{}, nil, nil)
	if err != nil {
		t.Fatalf("building RootType: %v", err)
	}
	e, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: "root"},
		xsd.TypeDefinitionRef{Name: xsd.QName{Local: "RootType"}}, &table, xsd.NewGlobalScope(),
		nil, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("building the root element declaration: %v", err)
	}
	b := xsd.NewSchemaBuilder()
	b.AddType(ct)
	b.AddElement(e)
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the tabled schema: %v", err)
	}
	v, err := New(schema, testBackend())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// The same root against the same type WITHOUT the table is charged clause
	// 3 for the missing @id, so the silence here is the table's doing.
	wantSilence(t, v.Assess(attributedRoot()).Violations(),
		"an unevaluable {test} leaves the ·selected type definition· undetermined")
	wantCharge(t, assessRoot(t, attributedRoot(), []xsd.AttributeUse{aUse(t, "id", true, nil)}, nil),
		"clause 3", loc(1, 1), "id")
}

// A SIMPLE-typed root's attributes reach cvc-complex-type through no clause of
// it: cvc-type clause 3.2 dispatches to cvc-complex-type only for a complex T,
// and a simple T is clause 3.1.1's much narrower question instead (the charge
// itself is cvctype_test.go's).
func TestSimpleTypedRootReachesNoComplexTypeClause(t *testing.T) {
	st, err := xsd.NewPrimitiveType(xsderr.Loc{}, xsd.QName{Local: "RootType"}, nil, nil)
	if err != nil {
		t.Fatalf("building RootType: %v", err)
	}
	e, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: "root"},
		xsd.TypeDefinitionRef{Name: xsd.QName{Local: "RootType"}}, nil, xsd.NewGlobalScope(),
		nil, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("building the root element declaration: %v", err)
	}
	b := xsd.NewSchemaBuilder()
	b.AddType(st)
	b.AddElement(e)
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the simple-typed schema: %v", err)
	}
	v, err := New(schema, testBackend())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := v.Assess(attributedRoot(local("stray"))).Violations()
	if len(got) != 1 || got[0].Rule != "cvc-type" {
		t.Fatalf("Violations() = %v, want exactly one cvc-type charge and no cvc-complex-type one", got)
	}
}

// A descendant's governing type comes from the particle it is ·attributed to·
// in its parent's {content type} (key-sva (§3.3.4.6) clause 3.1), and a child
// its parent decidedly attributed to NOTHING is governed per key-governing-ed
// clause 4: <child> has no ·locally declared type· and its name ·resolves· to
// no declaration, so it is ·laxly assessed· against xs:anyType, whose lax
// {attribute wildcard} admits the same stray attribute that is charged on the
// root (#1892).
//
// The child here is that shape because it is charged: governedSchema's type has
// an empty {content type}, which admits no element information item
// [[children]] at all (clause 1.1). That charge is the root's, at the child's
// position, and it names no attribute.
func TestDescendantAttributesOfAnUnattributedChildAreLaxlyAssessed(t *testing.T) {
	child := &testElement{
		name:  xsd.QName{Local: "child"},
		attrs: []Attribute{&testAttribute{name: local("stray"), value: "v", loc: loc(2, 3)}},
		loc:   loc(2, 1),
	}
	root := attributedRoot()
	root.kids = []Child{ElementChild(child)}

	got := assessRoot(t, root, []xsd.AttributeUse{aUse(t, "id", false, nil)}, nil)

	if len(got) != 1 || got[0].Loc != loc(2, 1) {
		t.Fatalf("Violations() = %v, want the one clause 1.1 charge at the child's own location", got)
	}
	if strings.Contains(got[0].Msg, "stray") {
		t.Errorf("Msg = %q, want no charge about a descendant's attribute", got[0].Msg)
	}
}

// The two clauses charge independently and in the order they are found:
// clause 2 over the attributes present, in source order, then clause 3 over
// the uses required. A Result therefore carries several honest charges for
// one root, which is why conformance's decidedNotValid does not pin the
// count at one.
func TestClauseTwoAndThreeChargeInDiscoveryOrder(t *testing.T) {
	root := attributedRoot(local("strayA"), local("strayB"))
	got := assessRoot(t, root, []xsd.AttributeUse{aUse(t, "req", true, nil)}, nil)
	if len(got) != 3 {
		t.Fatalf("Violations() = %v, want three", got)
	}
	wantLocs := []xsderr.Loc{loc(1, 10), loc(1, 11), loc(1, 1)}
	gotLocs := make([]xsderr.Loc, 0, len(got))
	for _, e := range got {
		gotLocs = append(gotLocs, e.Loc)
		if e.Rule != "cvc-complex-type" {
			t.Errorf("Rule = %q, want cvc-complex-type", e.Rule)
		}
	}
	if !slices.Equal(gotLocs, wantLocs) {
		t.Errorf("charged at %v, want %v: the attributes present in source order, then the uses required", gotLocs, wantLocs)
	}
	if !strings.Contains(got[2].Msg, "clause 3") {
		t.Errorf("the last charge is %q, want clause 3", got[2].Msg)
	}
}

// The debug log names the rule and clause that settled each attribute (STYLE
// L1), and says nothing about a rule where nothing was settled.
func TestAttributeLogNamesTheRuleAndClause(t *testing.T) {
	log, visits := recordingLogger()
	v, err := New(governedSchema(t, []xsd.AttributeUse{aUse(t, "id", false, nil)}, nil), testBackend(), WithLogger(log))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	v.Assess(attributedRoot(local("id"), local("stray")))

	want := []string{
		"assessing element validate.name=root validate.loc=instance.xml:1:1",
		"assessing attribute validate.name=id validate.loc=instance.xml:1:10 validate.rule=cvc-attribute validate.clause=3 validate.outcome=declined",
		"assessing attribute validate.name=stray validate.loc=instance.xml:1:11 validate.rule=cvc-complex-type validate.clause=2 validate.outcome=charged",
	}
	if !slices.Equal(*visits, want) {
		t.Errorf("walk logged\n\t%s\nwant\n\t%s",
			strings.Join(*visits, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// anonymousRootSchema declares "root" governed by the ANONYMOUS complex type
// it owns (§3.3.2.1 dcl.elt.common clause 1), derived from base by derivation
// and carrying uses. Base is a named type carrying one attribute use and an
// ##any wildcard, which the folds give the anonymous type both of under an
// EXTENSION and the use alone under a restriction (§3.4.2.5 clause 2.1).
// It is the component shape parser.Parse builds for an inline <complexType>
// child of an <element>, reached here through the constructors because this
// package imports no parser.
func anonymousRootSchema(t *testing.T, base xsd.QName, derivation xsd.DerivationMethod, uses []xsd.AttributeUse) *xsd.Schema {
	t.Helper()
	id := xsd.NewComponentID()
	ct, err := xsd.NewAnonymousComplexType(xsderr.Loc{}, xsd.ElementDeclarationContext{Component: id},
		base, nil, derivation, false, attrContent(uses), nil, nil, xsd.EmptyContent{}, nil, nil)
	if err != nil {
		t.Fatalf("building the anonymous root type: %v", err)
	}
	e, err := xsd.NewElementDeclarationOwningTypes(xsderr.Loc{}, id, xsd.QName{Local: "root"}, xsd.InlineTypeDefinition{Definition: ct}, nil,
		xsd.NewGlobalScope(), nil, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("building the root element declaration: %v", err)
	}
	baseType, err := xsd.NewComplexType(xsderr.Loc{}, xsd.QName{Local: "Base"}, xsd.QName{}, nil,
		xsd.DerivationRestriction, false, attrContent([]xsd.AttributeUse{aUse(t, "fromBase", false, nil)}), nil,
		anyWildcard(t, xsd.ProcessStrict), xsd.EmptyContent{}, nil, nil)
	if err != nil {
		t.Fatalf("building Base: %v", err)
	}
	// xs:anyType is the one complex type permitted to be its own base (§3.4.7)
	// and carries the ##any wildcard that section gives it — which a
	// RESTRICTION of it does not inherit (§3.4.2.5 clause 2.1), so its presence
	// here cannot be what makes the assessed case below silent.
	anyType, err := xsd.NewComplexType(xsderr.Loc{}, xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyType"},
		xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyType"}, nil, xsd.DerivationRestriction, false,
		nil, nil, anyWildcard(t, xsd.ProcessStrict), xsd.EmptyContent{}, nil, nil)
	if err != nil {
		t.Fatalf("building xs:anyType: %v", err)
	}
	b := xsd.NewSchemaBuilder()
	b.AddElement(e)
	b.AddType(baseType)
	b.AddType(anyType)
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the anonymous-root schema: %v", err)
	}
	return schema
}

// assessAgainst assesses root against schema and returns what was charged.
func assessAgainst(t *testing.T, schema *xsd.Schema, root Element) []*xsderr.Error {
	t.Helper()
	v, err := New(schema, testBackend())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res := v.Assess(root)
	if res.Err() != nil {
		t.Fatalf("Err() = %v, want nil", res.Err())
	}
	return res.Violations()
}

// An ANONYMOUS governing type derived from a named base decides the attribute
// half exactly as a named one does (#1116): §3.4.2.4 clause 3 and §3.4.2.5
// clause 2 are "the same for all complex type definitions", and both folds
// reach a type an element declaration OWNS (xsd/ownedtypefold.go), so Base's
// @fromBase use is among this type's {attribute uses} under either derivation
// method.
//
// The two documents separate the clauses. @fromBase is the INHERITED use and
// @own the type's own, so clause 2.1 matches both and charges nothing; the
// RESTRICTION arm is what pins the use fold, its {attribute wildcard} being the
// ·complete wildcard· of a type carrying no <anyAttribute> (§3.4.2.5 clause
// 2.1), so nothing but a matched use can keep @fromBase from clause 2. The
// empty document leaves the {required} @own with no attribute information item
// of its name, which is clause 3's charge and lands at the element.
func TestAnonymousDerivedRootDecidesTheAttributeHalf(t *testing.T) {
	for _, derivation := range []xsd.DerivationMethod{xsd.DerivationExtension, xsd.DerivationRestriction} {
		schema := anonymousRootSchema(t, xsd.QName{Local: "Base"}, derivation,
			[]xsd.AttributeUse{aUse(t, "own", true, nil)})
		wantSilence(t, assessAgainst(t, schema, attributedRoot(local("fromBase"), local("own"))),
			"the folded {attribute uses} hold Base's use as well as the type's own ("+derivation.String()+")")
		wantCharge(t, assessAgainst(t, schema, attributedRoot()), "clause 3", loc(1, 1), "own", "root")
	}
}

// The anonymous shape on which both folds are provably the identity: a
// restriction of xs:anyType, the §3.4.2.3.2 implicit-content form — §3.4.7
// makes xs:anyType's {attribute uses} empty so clause 3 inherits nothing, and
// §3.4.2.5 clause 2 unions the base's wildcard for an EXTENSION only. It is
// assessed on the same terms as the derived shape above rather than on any of
// its own, which is the whole claim: no assessment here reads {name} or
// {context}.
func TestAnonymousRestrictionOfAnyTypeIsAssessed(t *testing.T) {
	schema := anonymousRootSchema(t, xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyType"},
		xsd.DerivationRestriction, []xsd.AttributeUse{aUse(t, "id", true, nil)})
	wantCharge(t, assessAgainst(t, schema, attributedRoot(local("id"), local("stray"))),
		"clause 2", loc(1, 11), "stray")
	wantCharge(t, assessAgainst(t, schema, attributedRoot()), "clause 3", loc(1, 1), "id")
}

// attrContent lifts attribute uses into the attribute content a complex-type
// constructor takes, one xsd.ResolvedAttributeUse each: a type with no
// <attributeGroup ref>, whose content Finalize folds to exactly these uses.
func attrContent(uses []xsd.AttributeUse) []xsd.AttributeUseOrGroupRef {
	content := make([]xsd.AttributeUseOrGroupRef, len(uses))
	for i, u := range uses {
		content[i] = xsd.ResolvedAttributeUse{Use: u}
	}
	return content
}
