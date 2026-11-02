package xmlsrc

import (
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// entityDoc declares one unparsed entity, pic, in its internal subset, and one
// parsed entity, txt, that is no member of [unparsedEntities].
const entityDoc = `<?xml version="1.0"?>
<!DOCTYPE r [
  <!NOTATION gif SYSTEM "image/gif">
  <!ENTITY pic SYSTEM "pic.gif" NDATA gif>
  <!ENTITY txt "parsed">
]>
<r><c/></r>
`

// Every element of one document answers from its DOCTYPE, the document element
// and a child alike.
func TestElementsAnswerTheDocumentsUnparsedEntities(t *testing.T) {
	root := rootOf(t, entityDoc)
	child, ok := root.Children().Next()
	if !ok {
		t.Fatal("the document element yielded no child")
	}
	c, ok := child.Element()
	if !ok {
		t.Fatal("the first child is not an element")
	}
	for _, e := range []*element{root, c.(*element)} {
		if !e.HasUnparsedEntity("pic") {
			t.Errorf("%s: HasUnparsedEntity(%q) = false, want true", e.Name(), "pic")
		}
		if e.HasUnparsedEntity("txt") {
			t.Errorf("%s: HasUnparsedEntity(%q) = true, want false: txt is a parsed entity", e.Name(), "txt")
		}
	}
}

// entityValidator is a validator over a schema declaring <r> with an optional
// xs:ENTITY attribute @ent and an optional xs:ENTITIES attribute @ents.
func entityValidator(t *testing.T) *validate.Validator {
	t.Helper()
	b := xsd.NewSchemaBuilder()
	seeded, err := builtin.Seed(strict.New())
	if err != nil {
		t.Fatalf("seeding the builtin types: %v", err)
	}
	for _, st := range seeded {
		b.AddType(st)
	}
	var uses []xsd.AttributeUseOrGroupRef
	for _, a := range []struct{ name, typ string }{{"ent", "ENTITY"}, {"ents", "ENTITIES"}} {
		d, err := xsd.NewAttributeDeclaration(xsderr.Loc{}, xsd.QName{Local: a.name},
			xsd.TypeDefinitionRef{Name: xsd.QName{Space: xsd.XMLSchemaNS, Local: a.typ}},
			xsd.NewAttributeGlobalScope(), nil, false)
		if err != nil {
			t.Fatalf("building the %s attribute declaration: %v", a.name, err)
		}
		u, err := xsd.NewAttributeUse(xsderr.Loc{}, false, xsd.LocalAttributeDeclaration{Declaration: d}, nil, nil)
		if err != nil {
			t.Fatalf("building the %s attribute use: %v", a.name, err)
		}
		uses = append(uses, xsd.ResolvedAttributeUse{Use: u})
	}
	ct, err := xsd.NewComplexType(xsderr.Loc{}, xsd.QName{Local: "RType"}, xsd.QName{}, nil,
		xsd.DerivationRestriction, false, uses, nil, nil, xsd.EmptyContent{}, nil, nil)
	if err != nil {
		t.Fatalf("building RType: %v", err)
	}
	b.AddType(ct)
	e, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: "r"},
		xsd.TypeDefinitionRef{Name: xsd.QName{Local: "RType"}}, nil, xsd.NewGlobalScope(),
		nil, false, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("building the r element declaration: %v", err)
	}
	b.AddElement(e)
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the entity schema: %v", err)
	}
	v, err := validate.New(schema, strict.New())
	if err != nil {
		t.Fatalf("validate.New: %v", err)
	}
	return v
}

// Every element answers [all declarations processed] from the one reader: true
// for a DOCTYPE with only an internal subset, false for one naming an external
// subset.
func TestElementsAnswerAllDeclarationsProcessed(t *testing.T) {
	for _, tc := range []struct {
		doc  string
		want bool
	}{
		{entityDoc, true},
		{`<!DOCTYPE r SYSTEM "x.dtd"><r><c/></r>`, false},
	} {
		root := rootOf(t, tc.doc)
		child, ok := root.Children().Next()
		if !ok {
			t.Fatalf("%s: the document element yielded no child", tc.doc)
		}
		c, ok := child.Element()
		if !ok {
			t.Fatalf("%s: the first child is not an element", tc.doc)
		}
		for _, e := range []*element{root, c.(*element)} {
			if got := e.AllDeclarationsProcessed(); got != tc.want {
				t.Errorf("%s: %s: AllDeclarationsProcessed() = %t, want %t", tc.doc, e.Name(), got, tc.want)
			}
		}
	}
}

// What the reader did not read declares nothing, and the charge says the DTD
// was not fully read rather than that the name is undeclared: an entity
// declared only in an external subset, or after a reference to an external
// parameter entity, is charged so. One declared in an internal parameter
// entity's replacement text is declared, and so is one after an unread
// reference in a standalone="yes" document (XML 1.0 §5.1). A name missing from
// a DTD read in full is charged as plainly undeclared.
func TestXMLEntityValuesAndTheUnreadDTD(t *testing.T) {
	const unread = "the document's DTD was not fully read"
	for _, tc := range []struct {
		doc     string
		charged bool
		unread  bool
	}{
		{`<!DOCTYPE r SYSTEM "x.dtd"><r ent="pic"/>`, true, true},
		{`<!DOCTYPE r [<!ENTITY % p "<!ENTITY pic SYSTEM 'u.bin' NDATA n>"> %p; <!NOTATION n SYSTEM 'x'>]><r ent="pic"/>`, false, false},
		{`<!DOCTYPE r [<!ENTITY % x SYSTEM "x.ent"> %x; <!ENTITY pic SYSTEM "u.bin" NDATA n>]><r ent="pic"/>`, true, true},
		{`<?xml version="1.0" standalone="yes"?><!DOCTYPE r [<!ENTITY % x SYSTEM "x.ent"> %x; <!ENTITY pic SYSTEM "u.bin" NDATA n>]><r ent="pic"/>`, false, false},
		{`<!DOCTYPE r [<!ENTITY other SYSTEM "u.bin" NDATA n>]><r ent="pic"/>`, true, false},
	} {
		res, err := Validate(entityValidator(t), strings.NewReader(tc.doc))
		if err != nil {
			t.Fatalf("%s: Validate: %v", tc.doc, err)
		}
		got := res.Violations()
		if !tc.charged {
			if len(got) != 0 {
				t.Errorf("%s: Violations() = %v, want none", tc.doc, got)
			}
			continue
		}
		if len(got) != 1 || !strings.Contains(got[0].Msg, `[cvc-simple-type] value "pic"`) {
			t.Errorf("%s: Violations() = %v, want one String Valid clause 3 charge naming pic", tc.doc, got)
			continue
		}
		if says := strings.Contains(got[0].Msg, unread); says != tc.unread {
			t.Errorf("%s: Msg = %q, says %q: %t, want %t", tc.doc, got[0].Msg, unread, says, tc.unread)
		}
	}
}

// The internal subset's NDATA declarations are what an XML instance's
// ·ENTITY values· are read against, end to end: a declared name passes, and an
// undeclared one — including a parsed entity's name, and an ENTITIES item
// beside declared ones — fails String Valid clause 3. A document with no
// DOCTYPE declares nothing, so every ·ENTITY value· in it fails.
func TestXMLEntityValuesAreReadAgainstTheInternalSubset(t *testing.T) {
	const doctype = `<!DOCTYPE r [<!ENTITY pic SYSTEM "pic.gif" NDATA gif><!ENTITY txt "parsed">]>`
	for _, tc := range []struct {
		doc   string
		ghost string // the undeclared ·ENTITY value· charged, or "" for none
	}{
		{doctype + `<r ent="pic"/>`, ""},
		{doctype + `<r ents="pic pic"/>`, ""},
		{doctype + `<r ent="ghost"/>`, "ghost"},
		{doctype + `<r ent="txt"/>`, "txt"},
		{doctype + `<r ents="pic ghost"/>`, "ghost"},
		{`<r ent="pic"/>`, "pic"},
	} {
		res, err := Validate(entityValidator(t), strings.NewReader(tc.doc))
		if err != nil {
			t.Fatalf("%s: Validate: %v", tc.doc, err)
		}
		got := res.Violations()
		if tc.ghost == "" {
			if len(got) != 0 {
				t.Errorf("%s: Violations() = %v, want none", tc.doc, got)
			}
			continue
		}
		if len(got) != 1 || !strings.Contains(got[0].Msg, "[cvc-simple-type] value \""+tc.ghost+"\"") ||
			!strings.Contains(got[0].Msg, "cvc-simple-type clause 3") {
			t.Errorf("%s: Violations() = %v, want one String Valid clause 3 charge naming %q", tc.doc, got, tc.ghost)
		}
	}
}
