package codec

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/xsd"
)

// root declares s:r of an anonymous complex type whose content is body.
func root(body string) string {
	return `<xs:element name="r"><xs:complexType>` + body + `</xs:complexType></xs:element>`
}

// sequenceOf is a root whose content is a sequence of particles.
func sequenceOf(particles string) string {
	return root(`<xs:sequence>` + particles + `</xs:sequence>`)
}

// TestDecodeDeclines pins one row per construct Decode declines: each
// schema and instance is valid, so validation accepts, and Decode returns
// the construct's *DeclineError at the declining element's instance Loc,
// wrapping errors.ErrUnsupported, and no value. Each row decodes into a
// struct with no field, so a decline is never reported as the T check's
// ErrTypeMismatch: the declines precede it.
func TestDecodeDeclines(t *testing.T) {
	const a = `<s:a>x</s:a>`
	rows := []struct {
		name      string
		decls     string
		attrs     string
		content   string
		construct construct
		line, col int
	}{
		{"type table", `<xs:complexType name="rt"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>
			<xs:element name="r" type="s:rt"><xs:alternative test="@k" type="s:rt"/></xs:element>`, "", a, constructTypeTable, 1, 1},
		{"nillable", `<xs:element name="r" nillable="true"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType></xs:element>`,
			"", a, constructNillable, 1, 1},
		{"nillable local element with a fixed value", sequenceOf(`<xs:element name="a" type="xs:string" nillable="true" fixed="x"/>`), "", `<s:a>x</s:a>`, constructNillable, 1, 1},
		{"document element of mixed content with a default", `<xs:element name="r" default="d"><xs:complexType mixed="true"><xs:sequence><xs:element name="a" type="xs:string" minOccurs="0"/></xs:sequence></xs:complexType></xs:element>`,
			"", "", constructMixedContent, 1, 1},
		{"simple root", `<xs:element name="r" type="xs:string"/>`, "", "x", constructSimpleRoot, 1, 1},
		{"empty content", root(``), "", "", constructEmptyContent, 1, 1},
		{"simple content", root(`<xs:simpleContent><xs:extension base="xs:string"/></xs:simpleContent>`), "", "x", constructSimpleContent, 1, 1},
		{"mixed content", `<xs:element name="r"><xs:complexType mixed="true"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType></xs:element>`,
			"", a, constructMixedContent, 1, 1},
		{"open content", root(`<xs:openContent><xs:any namespace="##other" processContents="lax"/></xs:openContent><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence>`),
			"", a, constructOpenContent, 1, 1},
		{"group ref content", `<xs:group name="g"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:group>` + root(`<xs:group ref="s:g"/>`),
			"", a, constructGroupRefContent, 1, 1},
		{"choice content", root(`<xs:choice><xs:element name="a" type="xs:string"/></xs:choice>`), "", a, constructChoiceContent, 1, 1},
		{"all content", root(`<xs:all><xs:element name="a" type="xs:string"/></xs:all>`), "", a, constructAllContent, 1, 1},
		{"content sequence occurring twice", root(`<xs:sequence maxOccurs="2"><xs:element name="a" type="xs:string"/></xs:sequence>`), "", a, constructSequenceOccurs, 1, 1},
		{"nested model group", sequenceOf(`<xs:choice><xs:element name="a" type="xs:string"/></xs:choice>`), "", a, constructNestedGroup, 1, 1},
		{"nested <group ref>", `<xs:group name="g"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:group>` + sequenceOf(`<xs:group ref="s:g"/>`),
			"", a, constructNestedGroup, 1, 1},
		{"wildcard particle", sequenceOf(`<xs:any processContents="lax"/>`), "", a, constructWildcardParticle, 1, 1},
		{"element ref", `<xs:element name="a" type="xs:string"/>` + sequenceOf(`<xs:element ref="s:a"/>`), "", a, constructElementRef, 1, 1},
		{"local element of complex type", `<xs:complexType name="ct"><xs:sequence/></xs:complexType>` + sequenceOf(`<xs:element name="a" type="s:ct"/>`),
			"", `<s:a/>`, constructComplexElement, 1, 1},
		{"local element of xs:anyType", sequenceOf(`<xs:element name="a"/>`), "", a, constructComplexElement, 1, 1},
		{"local element of emptiable mixed content with a default", sequenceOf(`<xs:element name="a" default="d"><xs:complexType mixed="true"><xs:sequence><xs:element name="b" type="xs:string" minOccurs="0"/></xs:sequence></xs:complexType></xs:element>`),
			"", `<s:a/>`, constructComplexElement, 1, 1},
		{"particles sharing an expanded name", sequenceOf(`<xs:element name="a" type="xs:string"/><xs:element name="b" type="xs:string"/><xs:element name="a" type="xs:string"/>`),
			"", a + `<s:b>y</s:b>` + a, constructSharedName, 1, 1},
		{"list", sequenceOf(`<xs:element name="a"><xs:simpleType><xs:list itemType="xs:string"/></xs:simpleType></xs:element>`), "", a, constructList, 1, 1},
		{"union", sequenceOf(`<xs:element name="a"><xs:simpleType><xs:union memberTypes="xs:string xs:int"/></xs:simpleType></xs:element>`), "", a, constructUnion, 1, 1},
		{"xs:anySimpleType", sequenceOf(`<xs:element name="a" type="xs:anySimpleType"/>`), "", a, constructSpecial, 1, 1},
		{"xs:anyAtomicType", sequenceOf(`<xs:element name="a" type="xs:anyAtomicType"/>`), "", a, constructSpecial, 1, 1},
		{"other primitive", sequenceOf(`<xs:element name="a" type="xs:int"/>`), "", `<s:a>1</s:a>`, constructOtherPrimitive, 1, 1},
		{"attribute use's type", root(`<xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence><xs:attribute name="b" type="xs:boolean"/>`),
			` b="true"`, a, constructOtherPrimitive, 1, 1},
		{"attribute use in the xsi namespace", `<xs:import namespace="http://www.w3.org/2001/XMLSchema-instance"/>` +
			root(`<xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence><xs:attribute ref="xsi:nil"/>`), "", a, constructXsiAttribute, 1, 1},
		{"attribute wildcard", root(`<xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence><xs:anyAttribute processContents="lax"/>`),
			"", a, constructAttributeWildcard, 1, 1},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			data := instanceOf(row.attrs, row.content)
			mustAccept(t, schema, data)
			got, err := Decode[struct{}](schema, strict.New(), data)
			if got != nil {
				t.Fatalf("Decode = %v, want no value", got)
			}
			assertDecline(t, err, row.construct, row.line, row.col)
		})
	}
}

// TestDecodeXsiTypeDeclines pins the xsi:type decline (#3202's ruling,
// refused permanently where the selected type is concrete): an element
// carrying xsi:type, the document element or a child, declines at its own
// instance Loc after the T check, even where the xsi:type names the declared
// type itself. The child row is the one whose decoding into the base's field
// would lose the derived type's value: xs:token collapses what xs:string
// preserves.
func TestDecodeXsiTypeDeclines(t *testing.T) {
	schema := schemaOf(t, `<xs:complexType name="rt"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>
		<xs:element name="r" type="s:rt"/>`)
	rows := []struct {
		name      string
		attrs     string
		content   string
		line, col int
	}{
		{"the document element, naming its declared type", ` xsi:type="s:rt"`, `<s:a>x</s:a>`, 1, 1},
		{"a child, naming a type derived from its concrete one", "", `<s:a xsi:type="xs:token"> x  y </s:a>`, 1, 120},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			data := instanceOf(row.attrs, row.content)
			mustAccept(t, schema, data)
			got, err := Decode[struct{ A string }](schema, strict.New(), data)
			if got != nil {
				t.Fatalf("Decode = %v, want no value", got)
			}
			assertDecline(t, err, constructXsiType, row.line, row.col)
		})
	}
}

// TestDecodeUndeclaredRootXsiType pins that a document element no top-level
// declaration declares, which validate accepts through its xsi:type alone
// (§3.3.4.6 cvc-assess-elt), declines as xsi:type at its own Loc.
func TestDecodeUndeclaredRootXsiType(t *testing.T) {
	schema := schemaOf(t, `<xs:complexType name="rt"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType>`)
	data := instanceOf(` xsi:type="s:rt"`, `<s:a>x</s:a>`)
	mustAccept(t, schema, data)
	got, err := Decode[struct{ A string }](schema, strict.New(), data)
	if got != nil {
		t.Fatalf("Decode = %v, want no value", got)
	}
	assertDecline(t, err, constructXsiType, 1, 1)
}

// TestDeclineConstruct pins DeclineError.Construct's contract: two declines
// of one construct, from two schemas and declining sites, have equal
// Constructs, and declines of two different constructs have different ones.
// One row per construct group: the declaration, the content, a particle, an
// attribute use and xsi:type.
func TestDeclineConstruct(t *testing.T) {
	const a = `<s:a>x</s:a>`
	xsiImport := `<xs:import namespace="http://www.w3.org/2001/XMLSchema-instance"/>`
	type decl struct{ decls, attrs, content string }
	rows := []struct {
		group string
		pair  [2]decl
	}{
		{"declaration", [2]decl{
			{`<xs:element name="r" nillable="true"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType></xs:element>`, "", a},
			{sequenceOf(`<xs:element name="a" type="xs:string" nillable="true"/>`), "", a},
		}},
		{"content", [2]decl{
			{root(`<xs:choice><xs:element name="a" type="xs:string"/></xs:choice>`), "", a},
			{`<xs:complexType name="ct"><xs:choice><xs:element name="b" type="xs:string"/><xs:element name="a" type="xs:string"/></xs:choice></xs:complexType><xs:element name="r" type="s:ct"/>`, "", a},
		}},
		{"particle", [2]decl{
			{sequenceOf(`<xs:any processContents="lax"/>`), "", a},
			{sequenceOf(`<xs:element name="b" type="xs:string"/><xs:any processContents="skip"/>`), "", `<s:b>y</s:b><s:c/>`},
		}},
		{"attribute use", [2]decl{
			{xsiImport + root(`<xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence><xs:attribute ref="xsi:nil"/>`), "", a},
			{xsiImport + root(`<xs:sequence><xs:element name="b" type="xs:string"/></xs:sequence><xs:attribute name="c" type="xs:string"/><xs:attribute ref="xsi:schemaLocation"/>`), "", `<s:b>y</s:b>`},
		}},
		{"xsi:type", [2]decl{
			{`<xs:complexType name="rt"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType><xs:element name="r" type="s:rt"/>`, ` xsi:type="s:rt"`, a},
			{sequenceOf(`<xs:element name="a" type="xs:string"/>`), "", `<s:a xsi:type="xs:token">x</s:a>`},
		}},
	}
	constructs := make([]Construct, len(rows))
	for i, row := range rows {
		var got [2]Construct
		for j, d := range row.pair {
			schema := schemaOf(t, d.decls)
			data := instanceOf(d.attrs, d.content)
			mustAccept(t, schema, data)
			_, err := Decode[struct{ A string }](schema, strict.New(), data)
			var de *DeclineError
			if !errors.As(err, &de) {
				t.Fatalf("%s %d: Decode error = %v, want a *DeclineError", row.group, j, err)
			}
			got[j] = de.Construct()
		}
		if got[0] != got[1] {
			t.Errorf("%s: two declines of one construct have Constructs %s and %s, want equal", row.group, got[0], got[1])
		}
		constructs[i] = got[0]
	}
	for i := range rows {
		for j := i + 1; j < len(rows); j++ {
			if constructs[i] == constructs[j] {
				t.Errorf("the %s and %s declines have equal Constructs %s, want different", rows[i].group, rows[j].group, constructs[i])
			}
		}
	}
}

// TestDeclineErrorText pins a decline's whole message for a construct of the
// document element's type: the declaration's schema Loc, then the type's,
// labelled as the type's, then the refusal tail.
func TestDeclineErrorText(t *testing.T) {
	schema := schemaOf(t, root(`<xs:choice><xs:element name="a" type="xs:string"/></xs:choice>`))
	_, err := Decode[struct{}](schema, strict.New(), instanceOf("", `<s:a>x</s:a>`))
	const want = "?:1:1: codec does not decode choice content: the {type definition} of element {urn:s}r declared at s.xsd:1:181, a type defined at s.xsd:1:202; a decoder refusal, not a schema or instance fault"
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
}

// TestDeclineErrorTextSimpleRoot pins the message of a decline of the
// document element's type when that type is simple and built in.
func TestDeclineErrorTextSimpleRoot(t *testing.T) {
	schema := schemaOf(t, `<xs:element name="r" type="xs:string"/>`)
	_, err := Decode[string](schema, strict.New(), instanceOf("", `x`))
	const want = "?:1:1: codec does not decode a document element of simple type: the {type definition} of element {urn:s}r declared at s.xsd:1:181, a type defined at ?; a decoder refusal, not a schema or instance fault"
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
}

// mustAccept fails the test unless validate accepts data against schema.
func mustAccept(t *testing.T, schema *xsd.Schema, data []byte) {
	t.Helper()
	v, err := validate.New(schema, strict.New())
	if err != nil {
		t.Fatal(err)
	}
	res, err := xmlsrc.Validate(v, bytes.NewReader(data))
	if err != nil || res.Err() != nil || res.Standing() != validate.NotProvenInvalid {
		t.Fatalf("the row's instance is not accepted: %v %v %v", err, res.Violations(), res.Unevaluated())
	}
}

// assertDecline fails the test unless err is the *DeclineError of
// construct c at instance line and col, wrapping errors.ErrUnsupported.
func assertDecline(t *testing.T, err error, c construct, line, col int) {
	t.Helper()
	var de *DeclineError
	if !errors.As(err, &de) {
		t.Fatalf("Decode error = %v, want a *DeclineError", err)
	}
	if got := de.Construct(); got != (Construct{c}) || got.String() != c.name() {
		t.Errorf("Construct() = %s, want %s", got, c.name())
	}
	if de.Loc().Line != line || de.Loc().Col != col {
		t.Errorf("Loc = %s, want %d:%d", de.Loc(), line, col)
	}
	if !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("error %v does not wrap errors.ErrUnsupported", err)
	}
	if want := de.Loc().String() + ": codec does not decode " + c.name() + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error %q does not open with %q", err, want)
	}
}

// TestDeclineErrorTextOtherPrimitive pins the message of a decline of a
// local element whose type's primitive is neither xs:string nor xs:anyURI,
// the two primitives simpleDecline admits.
func TestDeclineErrorTextOtherPrimitive(t *testing.T) {
	schema := schemaOf(t, sequenceOf(`<xs:element name="a" type="xs:int"/>`))
	_, err := Decode[struct{}](schema, strict.New(), instanceOf("", `<s:a>1</s:a>`))
	const want = "?:1:1: codec does not decode an atomic simple type whose primitive is neither xs:string nor xs:anyURI: element {urn:s}a declared at s.xsd:1:231; a decoder refusal, not a schema or instance fault"
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
}
