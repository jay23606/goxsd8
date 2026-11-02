package validate

import (
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The fixtures below drive cvc-type (§3.3.4.4) clause 3.1 — the arm taken where
// an element's ·governing type definition· is a Simple Type Definition rather
// than a complex one. They reuse simpleTypedSchema (cvcelt_test.go) for a root
// whose declaration names a builtin simple type, and build their own fixtures
// for the two shapes that one cannot reach: a DESCENDANT typed by its parent's
// content model, and an element with no ·governing element declaration· at all
// whose xsi:type supplies the type instead.

// tWantCharge fails unless the violations are exactly one cvc-type charge
// naming clause inline (STYLE E4) and citing at.
func tWantCharge(t *testing.T, got []*xsderr.Error, clause string, at xsderr.Loc) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("Violations() = %v, want exactly one", got)
	}
	if got[0].Rule != "cvc-type" {
		t.Fatalf("Rule = %q, want cvc-type", got[0].Rule)
	}
	if !strings.Contains(got[0].Msg, "cvc-type clause "+clause) {
		t.Errorf("Msg = %q, want it to name cvc-type clause %s inline (STYLE E4)", got[0].Msg, clause)
	}
	if got[0].Loc != at {
		t.Errorf("Loc = %s, want the offending position %s", got[0].Loc, at)
	}
}

// Clause 3.1.3: the ·initial value· of an element whose ·governing type
// definition· IS a simple type is ·valid· with respect to that type per String
// Valid (§3.16.4). This is the leaf spelling of the very case the
// simpleContent-wrapped form already charged under cvc-complex-type clause 1.2
// (#913).
func TestSimpleTypeChargesAnInvalidInitialValue(t *testing.T) {
	schema := simpleTypedSchema(t, icBuiltin("decimal"), nil, false)

	wantSilence(t, cAssess(t, schema, cRoot("#12.50")), "12.50 is an xs:decimal")

	got := cAssess(t, schema, cRoot("#12,50"))
	tWantCharge(t, got, "3.1.3", loc(1, 1))
	if want := `element root's value "12,50" is not a valid {` + xsd.XMLSchemaNS + `}decimal, which cvc-type clause 3.1.3 requires: [cvc-datatype-valid] `; !strings.HasPrefix(got[0].Msg, want) {
		t.Errorf("Msg = %q, want the prefix %q: the element, its value and the ·governing type definition·, then the delegated verdict", got[0].Msg, want)
	}
	// The charge names the ·governing type definition· ITSELF, never the
	// {simple type definition} of a {content type} that is not there.
	if strings.Contains(got[0].Msg, "{content type}") {
		t.Errorf("Msg = %q, want no {content type} — clause 3.1's T has none", got[0].Msg)
	}
	if !strings.Contains(got[0].Msg, "decimal") {
		t.Errorf("Msg = %q, want the ·governing type definition· named", got[0].Msg)
	}
}

// Clause 3.1.3 tests the runs CONCATENATED, on the ·initial value·'s own terms
// (Glossary), and String Valid clause 1 normalizes before Datatype Valid runs.
func TestSimpleTypeReadsTheWholeInitialValue(t *testing.T) {
	schema := simpleTypedSchema(t, icBuiltin("integer"), nil, false)

	wantSilence(t, cAssess(t, schema, cRoot("#-", "#42")), "the ·initial value· is -42")
	wantSilence(t, cAssess(t, schema, cRoot("#  42 ")),
		"whiteSpace normalization precedes datatype validation (String Valid clause 1)")

	tWantCharge(t, cAssess(t, schema, cRoot("#4", "#-2")), "3.1.3", loc(1, 1))
}

// An element with NO character information item [[child]] and no {value
// constraint} on its declaration is on cvc-elt clause 5.2's arm, so clause 5.2.1
// sends E ITSELF to cvc-type and clause 3.1.3 charges its empty ·initial value·
// against a type whose lexical space excludes "".
func TestSimpleTypeChargesAnEmptyElement(t *testing.T) {
	schema := simpleTypedSchema(t, icBuiltin("integer"), nil, false)

	tWantCharge(t, cAssess(t, schema, cRoot()), "3.1.3", loc(1, 1))
}

// Clause 3.1.1 admits xsi:type, xsi:nil, xsi:schemaLocation and
// xsi:noNamespaceSchemaLocation and NOTHING else — not an unprefixed attribute,
// not one in a foreign namespace, not one the schema itself declares elsewhere.
// Each offending item is charged at its own location.
func TestSimpleTypeChargesEveryNonInstanceAttribute(t *testing.T) {
	schema := simpleTypedSchema(t, icBuiltin("integer"), nil, false)

	for i, name := range []xsd.QName{
		local("stray"),
		{Space: "http://example.com/other", Local: "foreign"},
		{Space: xsd.XMLSchemaNS, Local: "type"}, // the schema namespace, not the instance one
	} {
		root := attributedRoot(name)
		root.kids = []Child{TextChild(&testText{data: "42", loc: loc(2, 1)})}

		got := cAssess(t, schema, root)
		tWantCharge(t, got, "3.1.1", loc(1, 10))
		if !strings.Contains(got[0].Msg, name.Local) {
			t.Errorf("case %d: Msg = %q, want it to name the offending attribute", i, got[0].Msg)
		}
	}
}

// The four §3.2.7 instance attributes clause 3.1.1 excepts by name are silent,
// and the exception is by ·expanded name· and not by prefix.
func TestSimpleTypeExemptsTheFourInstanceAttributes(t *testing.T) {
	schema := simpleTypedSchema(t, icBuiltin("integer"), nil, false)

	instance := func(name string) xsd.QName {
		return xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: name}
	}
	root := attributedRoot(instance("schemaLocation"), instance("noNamespaceSchemaLocation"))
	root.kids = []Child{TextChild(&testText{data: "42", loc: loc(2, 1)})}

	wantSilence(t, cAssess(t, schema, root), "clause 3.1.1 excepts all four xsi: attributes by name")
}

// Clause 3.1.2 admits no element information item [[child]] at all, charged at
// the child's own position.
func TestSimpleTypeChargesAnElementChild(t *testing.T) {
	schema := simpleTypedSchema(t, icBuiltin("integer"), nil, false)

	got := cAssess(t, schema, cRoot("#42", "a"))
	tWantCharge(t, got, "3.1.2", loc(3, 1))
	if !strings.Contains(got[0].Msg, "a") {
		t.Errorf("Msg = %q, want it to name the offending child", got[0].Msg)
	}
}

// A ·nilled· element skips clause 3.1.3 and nothing else: the clause carries
// clause 3.1's only "if E is not ·nilled·" gate, so an empty nilled element
// carrying the nil attribute alone charges nothing at all.
func TestNilledSimpleTypedElementSkipsTheValueCheck(t *testing.T) {
	schema := simpleTypedSchema(t, icBuiltin("integer"), nil, true)

	nilled := func() *testElement {
		e := cRoot()
		e.attrs = []Attribute{&testAttribute{
			name:  xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "nil"},
			value: "true", loc: loc(1, 10)}}
		return e
	}

	wantSilence(t, cAssess(t, schema, nilled()), "clause 3.1.3 is gated on E not being ·nilled·")

	// Clause 3.1.1 is NOT gated on ·nilled·, and still charges the stray.
	withStray := nilled()
	withStray.attrs = append(withStray.attrs,
		&testAttribute{name: local("stray"), value: "v", loc: loc(1, 20)})
	tWantCharge(t, cAssess(t, schema, withStray), "3.1.1", loc(1, 20))
}

// Clause 3.1 reaches a DESCENDANT on the same terms as the root: the child here
// is typed by the element declaration its parent's content model ·attributed·
// it to (key-sva (§3.3.4.6) clause 3.1), and its own ·initial value· is charged
// against its own type, at its own location.
func TestSimpleTypeChargesADescendant(t *testing.T) {
	schema := descendantSimpleTypeSchema(t)

	root := &testElement{name: xsd.QName{Local: "root"}, loc: loc(1, 1)}
	child := &testElement{name: xsd.QName{Local: "amount"}, loc: loc(2, 3)}
	child.kids = []Child{TextChild(&testText{data: "12,50", loc: loc(2, 11)})}
	root.kids = []Child{ElementChild(child)}

	tWantCharge(t, cAssess(t, schema, root), "3.1.3", loc(2, 3))
}

// descendantSimpleTypeSchema declares "root" over an element-only complex type
// whose one particle is a local <amount> declaration of type xs:decimal, so the
// SIMPLE governing type is reached only through the descent.
func descendantSimpleTypeSchema(t *testing.T) *xsd.Schema {
	t.Helper()
	scope, err := xsd.NewLocalScope(xsderr.Loc{}, xsd.ComplexTypeScopeParent{Name: xsd.QName{Local: "RootType"}})
	if err != nil {
		t.Fatalf("NewLocalScope: %v", err)
	}
	amount, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: "amount"},
		xsd.TypeDefinitionRef{Name: icBuiltin("decimal")}, nil, scope,
		nil, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("building the amount element declaration: %v", err)
	}
	occurs, err := xsd.NewOccurs(xsderr.Loc{}, 1, 1)
	if err != nil {
		t.Fatalf("NewOccurs: %v", err)
	}
	p, err := xsd.NewParticle(xsderr.Loc{}, occurs, xsd.ResolvedTerm{Term: amount})
	if err != nil {
		t.Fatalf("NewParticle: %v", err)
	}
	ct, err := xsd.NewComplexType(xsderr.Loc{}, xsd.QName{Local: "RootType"}, xsd.QName{}, nil,
		xsd.DerivationRestriction, false, nil, nil, nil, cSequence(t, false, p), nil, nil)
	if err != nil {
		t.Fatalf("building the governing type: %v", err)
	}
	return cSchemaFrom(t, ct, func(b *xsd.SchemaBuilder) {
		seeded, err := builtin.Seed(testBackend())
		if err != nil {
			t.Fatalf("seeding the builtin types: %v", err)
		}
		for _, st := range seeded {
			b.AddType(st)
		}
	})
}

// The other entry point: key-sva (§3.3.4.6) clause 1.2, an element with no
// ·governing element declaration· at all whose xsi:type ·resolves· to a simple
// type. key-governing-type-elem clause 8 makes that the ·governing type
// definition·, so clause 3.1 applies with no declaration behind it.
func TestXSITypeAloneReachesClause31(t *testing.T) {
	schema := untypedSchema(t)

	root := &testElement{
		name:     xsd.QName{Local: "loose"},
		loc:      loc(1, 1),
		bindings: map[string]string{"xs": xsd.XMLSchemaNS},
		attrs: []Attribute{&testAttribute{
			name:  xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "type"},
			value: "xs:decimal", loc: loc(1, 10)}},
	}
	root.kids = []Child{TextChild(&testText{data: "12.50", loc: loc(1, 40)})}
	wantSilence(t, cAssess(t, schema, root), "12.50 is an xs:decimal, whatever supplied the type")

	root.kids = []Child{TextChild(&testText{data: "12,50", loc: loc(1, 40)})}
	tWantCharge(t, cAssess(t, schema, root), "3.1.3", loc(1, 1))
}

// untypedSchema holds the builtin types and NO element declaration, so a root
// determines its ·governing type definition· from xsi:type or from nothing.
func untypedSchema(t *testing.T) *xsd.Schema {
	t.Helper()
	b := xsd.NewSchemaBuilder()
	seeded, err := builtin.Seed(testBackend())
	if err != nil {
		t.Fatalf("seeding the builtin types: %v", err)
	}
	for _, st := range seeded {
		b.AddType(st)
	}
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the declarationless schema: %v", err)
	}
	return schema
}
