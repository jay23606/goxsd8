package codec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/codec/testdata/readme/order"
	"github.com/kud360/goxsd8/codegen"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/parser/xmltree"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// parseString returns the schema of the one schema document doc, located
// s.xsd.
func parseString(t *testing.T, doc string) *xsd.Schema {
	t.Helper()
	resolve := loader.ResolverFunc(func(_, location string) (io.ReadCloser, string, error) {
		if location != "s.xsd" {
			return nil, "", fmt.Errorf("no document %q", location)
		}
		return io.NopCloser(strings.NewReader(doc)), location, nil
	})
	schema, err := parser.Parse("s.xsd", parser.WithResolver(resolve))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return schema
}

// schemaOf returns the schema of a document in target namespace urn:s, its
// elements qualified and xsi bound, holding decls.
func schemaOf(t *testing.T, decls string) *xsd.Schema {
	t.Helper()
	return parseString(t, `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:s="urn:s" targetNamespace="urn:s" elementFormDefault="qualified">`+decls+`</xs:schema>`)
}

// instanceOf returns a document element s:r in urn:s, xs and xsi bound, with
// attrs on its start tag and content inside it.
func instanceOf(attrs, content string) []byte {
	return []byte(`<s:r xmlns:s="urn:s" xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` + attrs + `>` + content + `</s:r>`)
}

// TestDecodeReadmeOrder pins README's order example end to end: the golden
// package order holds the file codegen writes for testdata/readme/order.xsd,
// byte for byte, and README's order.xml decodes into its Order.
func TestDecodeReadmeOrder(t *testing.T) {
	readme := filepath.Join("..", "testdata", "readme")
	schema, err := parser.Parse("order.xsd", parser.WithResolver(loader.Dir(readme)))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "order")
	if err := codegen.Generate([]codegen.Target{{Schema: schema, Dir: dir, Package: "order"}}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "xsd_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "readme", "order", "xsd_gen.go")
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("codegen writes for order.xsd, which %s must hold byte for byte:\n%s", golden, got)
	}

	data, err := os.ReadFile(filepath.Join(readme, "order.xml"))
	if err != nil {
		t.Fatal(err)
	}
	o, err := Decode[order.Order](schema, strict.New(), data)
	if err != nil {
		t.Fatal(err)
	}
	kind := "x"
	if wantOrder := (order.Order{Item: []string{"a", "b"}, Kind: &kind}); !reflect.DeepEqual(*o, wantOrder) {
		t.Errorf("Decode(order.xml) = %+v (Kind %v), want Item [a b], Kind x", *o, o.Kind)
	}
}

// shapesSchema declares s:r with one local element per Go shape in the slice
// and a required and an optional attribute. The optional attribute shares
// the local name opt with an element, so the two fields' names pin
// codegen's claim order.
const shapesSchema = `
  <xs:element name="r">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="one" type="xs:string"/>
        <xs:element name="opt" type="xs:string" minOccurs="0"/>
        <xs:element name="rep" type="xs:string" minOccurs="0" maxOccurs="unbounded"/>
        <xs:element name="tok" type="xs:token"/>
      </xs:sequence>
      <xs:attribute name="req" type="xs:string" use="required"/>
      <xs:attribute name="opt" type="xs:normalizedString"/>
    </xs:complexType>
  </xs:element>`

// shapes is the struct codegen emits for shapesSchema's s:r, which
// TestShapesIsCodegens pins.
type shapes struct {
	One  string
	Opt  *string
	Rep  []string
	Tok  string
	Req  string
	Opt2 *string
}

// TestShapesIsCodegens pins shapes to the struct codegen emits for
// shapesSchema, so that the rows decoding into it decode into codegen's type.
func TestShapesIsCodegens(t *testing.T) {
	schema := schemaOf(t, shapesSchema)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "s")
	if err := codegen.Generate([]codegen.Target{{Schema: schema, Dir: dir, Package: "s"}}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "xsd_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	const want = "type R struct {\n\tOne  string\n\tOpt  *string\n\tRep  []string\n\tTok  string\n\tReq  string\n\tOpt2 *string\n}\n"
	if !bytes.Contains(got, []byte(want)) {
		t.Errorf("codegen emits for shapesSchema\n%s\nwhich does not declare R as shapes declares its fields:\n%s", got, want)
	}
}

// TestDecodeShapes pins each Go shape of the slice: a required element
// (string), an optional one (*string) present and absent, a repeated one
// ([]string), a required attribute (string) and an optional one (*string)
// present and absent, each value the item's ·normalized value· (key-nv): an
// xs:token element's collapsed, an xs:normalizedString attribute's replaced,
// and an xs:string element's preserved.
func TestDecodeShapes(t *testing.T) {
	schema := schemaOf(t, shapesSchema)
	str := func(s string) *string { return &s }
	rows := []struct {
		name  string
		attrs string
		body  string
		want  shapes
	}{
		{
			name:  "every field present",
			attrs: ` req=" r " opt="a&#9;b"`,
			body:  "<s:one> x\ty </s:one><s:opt>o</s:opt><s:rep>1</s:rep><s:rep> 2 </s:rep><s:tok>\n  a   b \n</s:tok>",
			want:  shapes{One: " x\ty ", Opt: str("o"), Rep: []string{"1", " 2 "}, Tok: "a b", Req: " r ", Opt2: str("a b")},
		},
		{
			name:  "a comment and a processing instruction split no value",
			attrs: ` req="r"`,
			body:  "<s:one>a<!--c-->b<?p x?>c</s:one><s:tok>t<!--c--> u</s:tok>",
			want:  shapes{One: "abc", Tok: "t u", Req: "r"},
		},
		{
			name:  "optional and repeated fields absent",
			attrs: ` req="r"`,
			body:  "<s:one></s:one><s:tok>t</s:tok>",
			want:  shapes{Tok: "t", Req: "r"},
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			got, err := Decode[shapes](schema, strict.New(), instanceOf(row.attrs, row.body))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*got, row.want) {
				t.Errorf("Decode = %s, want %s", render(*got), render(row.want))
			}
		})
	}
}

// render prints s with its pointers followed.
func render(s shapes) string {
	deref := func(p *string) string {
		if p == nil {
			return "nil"
		}
		return fmt.Sprintf("&%q", *p)
	}
	return fmt.Sprintf("{One:%q Opt:%s Rep:%q Tok:%q Req:%q Opt2:%s}", s.One, deref(s.Opt), s.Rep, s.Tok, s.Req, deref(s.Opt2))
}

// TestDecodeRefusesWhatValidateCharges pins that Decode never succeeds where
// validate charges the instance: a missing required child (cvc-complex-content
// clause 1, its first violation) and an undeclared attribute (cvc-complex-type
// clause 2.2) each return an error carrying the validator's rule and the
// instance Loc, and no value.
func TestDecodeRefusesWhatValidateCharges(t *testing.T) {
	schema := schemaOf(t, shapesSchema)
	rows := []struct {
		name  string
		attrs string
		body  string
		rule  xsderr.Rule
	}{
		{"missing required child", ` req="r"`, "<s:one>x</s:one>", "cvc-complex-content"},
		{"undeclared attribute", ` req="r" bogus="b"`, "<s:one>x</s:one><s:tok>t</s:tok>", "cvc-complex-type"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			got, err := Decode[shapes](schema, strict.New(), instanceOf(row.attrs, row.body))
			if got != nil || err == nil {
				t.Fatalf("Decode = (%v, %v), want no value and an error", got, err)
			}
			if rule, _ := xsderr.RuleOf(err); rule != row.rule {
				t.Errorf("rule = %q, want %s: %v", rule, row.rule, err)
			}
			if loc, _ := xsderr.LocOf(err); loc.Line != 1 || loc.Col != 1 {
				t.Errorf("Loc = %s, want the document element's, 1:1: %v", loc, err)
			}
			if !strings.HasPrefix(err.Error(), "codec: the instance is invalid against the schema: ") {
				t.Errorf("error %q does not open as Decode's Charged error", err)
			}
			var de *DeclineError
			if errors.Is(err, errors.ErrUnsupported) || errors.As(err, &de) {
				t.Errorf("error %v is a decline or unsupported, want a violation", err)
			}
		})
	}
}

// TestDecodeAbstractRoot pins that an abstract document element never reaches
// a decline: the validator charges it (cvc-elt clause 2) and Decode returns
// that charge with no value.
func TestDecodeAbstractRoot(t *testing.T) {
	schema := schemaOf(t, `<xs:element name="r" abstract="true"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence></xs:complexType></xs:element>`)
	got, err := Decode[struct{ A string }](schema, strict.New(), instanceOf("", "<s:a>x</s:a>"))
	if got != nil || err == nil {
		t.Fatalf("Decode = (%v, %v), want no value and an error", got, err)
	}
	if rule, _ := xsderr.RuleOf(err); rule != "cvc-elt" {
		t.Errorf("rule = %q, want cvc-elt: %v", rule, err)
	}
}

// TestDecodeUndecided pins item 4 of Decode's doc: an in-slice instance whose
// complex type carries an assertion the XPath subset declines is Undecided
// (cvc-assertion), and Decode returns an error naming that rule and the
// instance Loc, wrapping errors.ErrUnsupported, which is no *DeclineError.
func TestDecodeUndecided(t *testing.T) {
	schema := schemaOf(t, undecidedSchema)
	got, err := Decode[struct{ A string }](schema, strict.New(), instanceOf("", "<s:a>x</s:a>"))
	if got != nil || err == nil {
		t.Fatalf("Decode = (%v, %v), want no value and an error", got, err)
	}
	const want = "codec: ?:1:1: the validator left cvc-assertion undecided, so the instance is not proven valid: unsupported operation"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
	var de *DeclineError
	if !errors.Is(err, errors.ErrUnsupported) || errors.As(err, &de) {
		t.Errorf("error %v: want errors.ErrUnsupported and no *DeclineError", err)
	}
}

// TestDecodeSourceFault pins item 2 of Decode's doc: an ill-formed instance
// returns the reader's xml-wf error as it is.
func TestDecodeSourceFault(t *testing.T) {
	schema := schemaOf(t, shapesSchema)
	got, err := Decode[shapes](schema, strict.New(), []byte(`<s:r xmlns:s="urn:s" req="r"><s:one>x</s:one>`))
	if got != nil || err == nil {
		t.Fatalf("Decode = (%v, %v), want no value and an error", got, err)
	}
	if rule, _ := xsderr.RuleOf(err); rule != xsderr.RuleXMLWellFormed {
		t.Errorf("rule = %q, want %s: %v", rule, xsderr.RuleXMLWellFormed, err)
	}
	if strings.HasPrefix(err.Error(), "codec:") {
		t.Errorf("error %q is wrapped, want the source fault as it is", err)
	}
}

// TestDecodeNilArguments pins item 1 of Decode's doc: a nil schema is
// validate.New's plain error, and a nil backend panics.
func TestDecodeNilArguments(t *testing.T) {
	got, err := Decode[shapes](nil, strict.New(), instanceOf("", ""))
	if got != nil || err == nil {
		t.Fatalf("Decode(nil schema) = (%v, %v), want no value and an error", got, err)
	}
	if _, isRule := xsderr.RuleOf(err); isRule {
		t.Errorf("Decode(nil schema) = %v, want a plain error", err)
	}
	_, want := validate.New(nil, strict.New())
	if err.Error() != want.Error() {
		t.Errorf("Decode(nil schema) = %q, want validate.New's %q", err, want)
	}
	defer func() {
		if recover() == nil {
			t.Error("Decode(nil backend) did not panic")
		}
	}()
	_, _ = Decode[shapes](schemaOf(t, shapesSchema), nil, instanceOf("", ""))
}

// TestDecodeTypeMismatch pins item 6 of Decode's doc: a T that is not the
// type codegen emits is an error errors.Is reports as ErrTypeMismatch, no
// decline and no rule, and nothing is filled, for a document element of
// complex type and, a T whose underlying type is not string, of simple type;
// an error that is no T mismatch is not reported as one.
func TestDecodeTypeMismatch(t *testing.T) {
	schema := schemaOf(t, shapesSchema)
	data := instanceOf(` req="r"`, "<s:one>x</s:one><s:tok>t</s:tok>")
	simple := schemaOf(t, `<xs:element name="r" type="xs:string"/>`)
	simpleData := instanceOf("", "x")
	type count int
	type renamed struct {
		One  string
		Opt  *string
		Rep  []string
		Tok  string
		Req  string
		Opt3 *string
	}
	type retyped struct {
		One  string
		Opt  string
		Rep  []string
		Tok  string
		Req  string
		Opt2 *string
	}
	type short struct {
		One string
	}
	rows := []struct {
		name   string
		decode func() (any, error)
		want   string
	}{
		{"renamed field", func() (any, error) { return Decode[renamed](schema, strict.New(), data) },
			ErrTypeMismatch.Error() + ": field 5 of codec.renamed is Opt3 *string where codegen emits Opt2 *string"},
		{"retyped field", func() (any, error) { return Decode[retyped](schema, strict.New(), data) },
			ErrTypeMismatch.Error() + ": field 1 of codec.retyped is Opt string where codegen emits Opt *string"},
		{"field count", func() (any, error) { return Decode[short](schema, strict.New(), data) },
			ErrTypeMismatch.Error() + ": codec.short has 1 fields where codegen emits 6"},
		{"no struct", func() (any, error) { return Decode[string](schema, strict.New(), data) },
			ErrTypeMismatch.Error() + ": string is no struct"},
		{"a struct for a simple root", func() (any, error) { return Decode[struct{ A string }](simple, strict.New(), simpleData) },
			ErrTypeMismatch.Error() + ": the underlying type of struct { A string } is not string"},
		{"a type over int for a simple root", func() (any, error) { return Decode[count](simple, strict.New(), simpleData) },
			ErrTypeMismatch.Error() + ": the underlying type of codec.count is not string"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			got, err := row.decode()
			if !reflect.ValueOf(got).IsNil() || err == nil {
				t.Fatalf("Decode = (%v, %v), want no value and an error", got, err)
			}
			if err.Error() != row.want {
				t.Errorf("error = %q, want %q", err, row.want)
			}
			if !errors.Is(err, ErrTypeMismatch) {
				t.Errorf("error %v is not ErrTypeMismatch", err)
			}
			var de *DeclineError
			if _, isRule := xsderr.RuleOf(err); isRule || errors.Is(err, errors.ErrUnsupported) || errors.As(err, &de) {
				t.Errorf("error %v is a rule or a decline, want ErrTypeMismatch alone", err)
			}
		})
	}
	// A document element with no top-level declaration and no xsi:type is
	// charged by the validator (cvc-assess-elt) before Decode binds it, so
	// the row calls bind, whose error Decode returns as it is.
	empty := schemaOf(t, "")
	_, err := Decode[struct{}](empty, strict.New(), instanceOf("", ""))
	if rule, _ := xsderr.RuleOf(err); rule != "cvc-assess-elt" {
		t.Fatalf("Decode(undeclared document element) = %v, want the validator's cvc-assess-elt", err)
	}
	start, err := documentElement(xmltree.NewReader("", bytes.NewReader(instanceOf("", ""))))
	if err != nil {
		t.Fatal(err)
	}
	_, err = bind(empty, start)
	const want = "codec: ?:1:1: the document element {urn:s}r has no top-level declaration"
	if err == nil || err.Error() != want || errors.Is(err, ErrTypeMismatch) {
		t.Errorf("bind(undeclared document element) = %v, want %q, no ErrTypeMismatch", err, want)
	}
}

// TestDecodeSimpleRoot pins a document element of simple type (cvc-type
// clause 3.1): it decodes into a T whose underlying type is string, its name
// never compared, holding the element's ·normalized value· under the
// declaration's type (Datatypes §4.3.6): an xs:string's preserved, an
// xs:token's and an xs:anyURI's collapsed, and a user restriction of
// xs:string under its own whiteSpace facet.
func TestDecodeSimpleRoot(t *testing.T) {
	type label string
	rows := []struct {
		name    string
		decls   string
		content string
		decode  func(*xsd.Schema, []byte) (string, error)
		want    string
	}{
		{"xs:string, into string", `<xs:element name="r" type="xs:string"/>`, " a\tb ", decodeAs[string], " a\tb "},
		{"xs:string, into a named type", `<xs:element name="r" type="xs:string"/>`, " a\tb ", decodeAs[label], " a\tb "},
		{"xs:token", `<xs:element name="r" type="xs:token"/>`, "\n  a   b \n", decodeAs[label], "a b"},
		{"xs:anyURI", `<xs:element name="r" type="xs:anyURI"/>`, "  http://e.x/a \t b  ", decodeAs[label], "http://e.x/a b"},
		{"a user restriction of xs:string", `<xs:simpleType name="code"><xs:restriction base="xs:string"><xs:whiteSpace value="collapse"/><xs:maxLength value="4"/></xs:restriction></xs:simpleType>
			<xs:element name="r" type="s:code"/>`, " a   b ", decodeAs[label], "a b"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			data := instanceOf("", row.content)
			mustAccept(t, schema, data)
			got, err := row.decode(schema, data)
			if err != nil {
				t.Fatal(err)
			}
			if got != row.want {
				t.Errorf("Decode = %q, want %q", got, row.want)
			}
		})
	}
}

// decodeAs decodes data against schema into T, whose underlying type is
// string, and returns the value as a string.
func decodeAs[T ~string](schema *xsd.Schema, data []byte) (string, error) {
	v, err := Decode[T](schema, strict.New(), data)
	if err != nil {
		return "", err
	}
	return string(*v), nil
}

// TestDecodeStringDerivedTypes pins the package doc's "# What decodes" list
// of the string-derived builtins and a user restriction: each element of
// one decodes, collapsed where its whiteSpace is collapse (Datatypes §4.3.6).
func TestDecodeStringDerivedTypes(t *testing.T) {
	schema := schemaOf(t, `<xs:simpleType name="code"><xs:restriction base="xs:NCName"><xs:maxLength value="4"/></xs:restriction></xs:simpleType>
		<xs:element name="r"><xs:complexType><xs:sequence>
		<xs:element name="ns" type="xs:normalizedString"/><xs:element name="lang" type="xs:language"/>
		<xs:element name="name" type="xs:Name"/><xs:element name="ncname" type="xs:NCName"/>
		<xs:element name="nmtoken" type="xs:NMTOKEN"/><xs:element name="id" type="xs:ID"/>
		<xs:element name="idref" type="xs:IDREF"/><xs:element name="entity" type="xs:ENTITY"/>
		<xs:element name="code" type="s:code"/>
		</xs:sequence></xs:complexType></xs:element>`)
	type derived struct {
		Ns, Lang, Name, Ncname, Nmtoken, Id, Idref, Entity, Code string
	}
	data := []byte(`<!DOCTYPE s:r [<!NOTATION n SYSTEM "n"><!ENTITY e SYSTEM "e" NDATA n>]>` + string(instanceOf("",
		"<s:ns>a\tb</s:ns><s:lang> en </s:lang><s:name> p:q </s:name><s:ncname> q </s:ncname><s:nmtoken> 1a </s:nmtoken>"+
			"<s:id> i1 </s:id><s:idref> i1 </s:idref><s:entity> e </s:entity><s:code> ab </s:code>")))
	got, err := Decode[derived](schema, strict.New(), data)
	if err != nil {
		t.Fatal(err)
	}
	want := derived{Ns: "a b", Lang: "en", Name: "p:q", Ncname: "q", Nmtoken: "1a", Id: "i1", Idref: "i1", Entity: "e", Code: "ab"}
	if *got != want {
		t.Errorf("Decode = %+v, want %+v", *got, want)
	}
}

// derivedSchema declares document elements whose field order is not the
// declaration's own: r1's type extends base, adding an attribute use, r2's
// restricts it, and r4 brings its attribute uses by attribute group.
const derivedSchema = `
  <xs:complexType name="base"><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence><xs:attribute name="x" type="xs:string"/></xs:complexType>
  <xs:complexType name="ext"><xs:complexContent><xs:extension base="s:base"><xs:attribute name="y" type="xs:string"/></xs:extension></xs:complexContent></xs:complexType>
  <xs:complexType name="res"><xs:complexContent><xs:restriction base="s:base"><xs:sequence><xs:element name="a" type="xs:token"/></xs:sequence></xs:restriction></xs:complexContent></xs:complexType>
  <xs:element name="r1" type="s:ext"/>
  <xs:element name="r2" type="s:res"/>
  <xs:element name="r4"><xs:complexType><xs:sequence><xs:element name="a" type="xs:string"/></xs:sequence><xs:attributeGroup ref="s:g"/></xs:complexType></xs:element>
  <xs:attributeGroup name="g"><xs:attribute name="z" type="xs:string"/><xs:attribute name="w" type="xs:string" use="required"/></xs:attributeGroup>`

// TestDecodeDerivedTypes pins Decode's field list to codegen's where the
// attribute uses come from a base type or an attribute group: each struct
// below is the one codegen emits for derivedSchema, which the test checks,
// and each document element decodes into it.
func TestDecodeDerivedTypes(t *testing.T) {
	schema := schemaOf(t, derivedSchema)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "s")
	if err := codegen.Generate([]codegen.Target{{Schema: schema, Dir: dir, Package: "s"}}); err != nil {
		t.Fatal(err)
	}
	gen, err := os.ReadFile(filepath.Join(dir, "xsd_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range []string{
		"type R1 struct {\n\tA string\n\tY *string\n\tX *string\n}\n",
		"type R2 struct {\n\tA string\n\tX *string\n}\n",
		"type R4 struct {\n\tA string\n\tZ *string\n\tW string\n}\n",
	} {
		if !bytes.Contains(gen, []byte(decl)) {
			t.Errorf("codegen emits for derivedSchema\n%s\nwhich does not declare\n%s", gen, decl)
		}
	}
	str := func(s string) *string { return &s }
	instance := func(name, attrs, content string) []byte {
		return []byte(`<s:` + name + ` xmlns:s="urn:s"` + attrs + `>` + content + `</s:` + name + `>`)
	}
	type R1 struct {
		A string
		Y *string
		X *string
	}
	r1, err := Decode[R1](schema, strict.New(), instance("r1", ` x="1" y="2"`, "<s:a>a</s:a>"))
	if err != nil || !reflect.DeepEqual(*r1, R1{A: "a", Y: str("2"), X: str("1")}) {
		t.Errorf("Decode(r1) = %v, %v", r1, err)
	}
	type R2 struct {
		A string
		X *string
	}
	r2, err := Decode[R2](schema, strict.New(), instance("r2", "", "<s:a> a  b </s:a>"))
	if err != nil || !reflect.DeepEqual(*r2, R2{A: "a b"}) {
		t.Errorf("Decode(r2) = %v, %v", r2, err)
	}
	type R4 struct {
		A string
		Z *string
		W string
	}
	r4, err := Decode[R4](schema, strict.New(), instance("r4", ` w="w"`, "<s:a>a</s:a>"))
	if err != nil || !reflect.DeepEqual(*r4, R4{A: "a", W: "w"}) {
		t.Errorf("Decode(r4) = %v, %v", r4, err)
	}
}

// generated returns the file codegen writes for schema.
func generated(t *testing.T, schema *xsd.Schema) []byte {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "s")
	if err := codegen.Generate([]codegen.Target{{Schema: schema, Dir: dir, Package: "s"}}); err != nil {
		t.Fatal(err)
	}
	gen, err := os.ReadFile(filepath.Join(dir, "xsd_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	return gen
}

// The choice schemas, each declaring s:r of an anonymous complex type whose
// content is a choice occurring once: pickSchema's alternatives are a and a
// repeated b, so codegen gives the bare interface pick; optSchema's b has
// {min occurs} 0, so the choice is emptiable and codegen gives the struct
// opt; attributedSchema adds an attribute named choice and a required one,
// so codegen gives the struct attributed, whose field Choice2 pins that the
// choice's field claims its name first.
const (
	pickSchema       = `<xs:element name="r"><xs:complexType><xs:choice><xs:element name="a" type="xs:string"/><xs:element name="b" type="xs:token" maxOccurs="3"/></xs:choice></xs:complexType></xs:element>`
	optSchema        = `<xs:element name="r"><xs:complexType><xs:choice><xs:element name="a" type="xs:string"/><xs:element name="b" type="xs:string" minOccurs="0"/></xs:choice></xs:complexType></xs:element>`
	attributedSchema = `<xs:element name="r"><xs:complexType><xs:choice><xs:element name="a" type="xs:string"/><xs:element name="b" type="xs:string"/></xs:choice>
		<xs:attribute name="choice" type="xs:string"/><xs:attribute name="tag" type="xs:string" use="required"/></xs:complexType></xs:element>`
)

// pick, pickA and pickB are the types codegen emits for pickSchema, R, RA and
// RB, which TestChoiceTypesAreCodegens pins; pickC, of a shape codegen emits
// no branch in, and pickA2, of pickA's, are implementers codegen does not
// emit.
type (
	pick   interface{ isPick() }
	pickA  struct{ A string }
	pickB  struct{ B []string }
	pickC  struct{ C string }
	pickA2 struct{ A string }
)

func (pickA) isPick()  {}
func (pickB) isPick()  {}
func (pickC) isPick()  {}
func (pickA2) isPick() {}

// opt, optChoice, optA and optB are optSchema's R, RChoice, RChoiceA and
// RChoiceB.
type (
	opt       struct{ Choice optChoice }
	optChoice interface{ isOptChoice() }
	optA      struct{ A string }
	optB      struct{ B string }
)

func (optA) isOptChoice() {}
func (optB) isOptChoice() {}

// attributed, attributedChoice, attributedA and attributedB are
// attributedSchema's R, RChoice, RChoiceA and RChoiceB.
type (
	attributed struct {
		Choice  attributedChoice
		Choice2 *string
		Tag     string
	}
	attributedChoice interface{ isAttributedChoice() }
	attributedA      struct{ A string }
	attributedB      struct{ B string }
)

func (attributedA) isAttributedChoice() {}
func (attributedB) isAttributedChoice() {}

// The branch sets of the three schemas' interfaces.
var (
	pickBranches       = WithBranches[pick](pickA{}, pickB{})
	optBranches        = WithBranches[optChoice](optA{}, optB{})
	attributedBranches = WithBranches[attributedChoice](attributedA{}, attributedB{})
)

// TestChoiceTypesAreCodegens pins the types above to those codegen emits for
// each choice schema, in shape, so that the rows decoding into them decode
// into codegen's types.
func TestChoiceTypesAreCodegens(t *testing.T) {
	rows := []struct {
		decls string
		want  []string
	}{
		{pickSchema, []string{
			"type R interface {\n\tisR()\n}\n",
			"type RA struct {\n\tA string\n}\n\nfunc (RA) isR() {}\n",
			"type RB struct {\n\tB []string\n}\n\nfunc (RB) isR() {}\n",
		}},
		{optSchema, []string{
			"type R struct {\n\tChoice RChoice\n}\n",
			"type RChoice interface {\n\tisRChoice()\n}\n",
			"type RChoiceA struct {\n\tA string\n}\n\nfunc (RChoiceA) isRChoice() {}\n",
			"type RChoiceB struct {\n\tB string\n}\n\nfunc (RChoiceB) isRChoice() {}\n",
		}},
		{attributedSchema, []string{
			"type R struct {\n\tChoice  RChoice\n\tChoice2 *string\n\tTag     string\n}\n",
			"type RChoice interface {\n\tisRChoice()\n}\n",
			"type RChoiceA struct {\n\tA string\n}\n\nfunc (RChoiceA) isRChoice() {}\n",
			"type RChoiceB struct {\n\tB string\n}\n\nfunc (RChoiceB) isRChoice() {}\n",
		}},
	}
	for _, row := range rows {
		gen := generated(t, schemaOf(t, row.decls))
		for _, decl := range row.want {
			if !bytes.Contains(gen, []byte(decl)) {
				t.Errorf("codegen emits\n%s\nwhich does not declare\n%s", gen, decl)
			}
		}
	}
}

// TestDecodeChoice pins the choice lift (cvc-model-group: one alternative
// matches): each branch of the bare interface, a repeated alternative
// appending into its one branch, the emptiable choice's empty match as the
// nil interface (cos-group-emptiable) and its {min occurs} 0 alternative's
// branch holding a string, and the struct holding the choice beside its
// attribute fields. Each value is its element's ·normalized value·: b's
// xs:token collapses.
func TestDecodeChoice(t *testing.T) {
	str := func(s string) *string { return &s }
	rows := []struct {
		name   string
		decls  string
		attrs  string
		body   string
		decode func(*xsd.Schema, []byte) (any, error)
		want   any
	}{
		{"bare interface, first branch", pickSchema, "", "<s:a> x </s:a>", decodeInto[pick](pickBranches), pick(pickA{A: " x "})},
		{"bare interface, repeated branch", pickSchema, "", "<s:b> 1  2 </s:b><s:b>3</s:b><s:b/>", decodeInto[pick](pickBranches), pick(pickB{B: []string{"1 2", "3", ""}})},
		{"emptiable choice, empty match", optSchema, "", "", decodeInto[opt](optBranches), opt{}},
		{"emptiable choice, its optional alternative present", optSchema, "", "<s:b>v</s:b>", decodeInto[opt](optBranches), opt{Choice: optB{B: "v"}}},
		{"emptiable choice, its required alternative", optSchema, "", "<s:a>w</s:a>", decodeInto[opt](optBranches), opt{Choice: optA{A: "w"}}},
		{"attributes beside the choice", attributedSchema, ` tag="t" choice="c"`, "<s:b>v</s:b>", decodeInto[attributed](attributedBranches),
			attributed{Choice: attributedB{B: "v"}, Choice2: str("c"), Tag: "t"}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			data := instanceOf(row.attrs, row.body)
			mustAccept(t, schema, data)
			got, err := row.decode(schema, data)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, row.want) {
				t.Errorf("Decode = %#v, want %#v", got, row.want)
			}
		})
	}
}

// decodeInto returns a function decoding data against schema into T under
// opts and returning the value T holds, or nil with Decode's error.
func decodeInto[T any](opts ...Option) func(*xsd.Schema, []byte) (any, error) {
	return func(schema *xsd.Schema, data []byte) (any, error) {
		v, err := Decode[T](schema, strict.New(), data, opts...)
		if err != nil {
			return nil, err
		}
		return *v, nil
	}
}

// TestDecodeChoiceTypeMismatch pins item 6 of Decode's doc for a choice: a T
// or a branch set that is not codegen's is ErrTypeMismatch, no decline, and
// nothing is filled.
func TestDecodeChoiceTypeMismatch(t *testing.T) {
	type exported interface{ IsR() }
	type twoMethods interface {
		isPick()
		isOther()
	}
	type stringChoice struct {
		Choice  string
		Choice2 *string
		Tag     string
	}
	type misnamed struct {
		C       attributedChoice
		Choice2 *string
		Tag     string
	}
	type attributeShort struct {
		Choice attributedChoice
		Tag    string
		X      *string
	}
	pickData := instanceOf("", "<s:a>x</s:a>")
	tagged := instanceOf(` tag="t"`, "<s:a>x</s:a>")
	rows := []struct {
		name   string
		decls  string
		data   []byte
		decode func(*xsd.Schema, []byte) (any, error)
		want   string
	}{
		{"no WithBranches", pickSchema, pickData, decodeInto[pick](),
			"no WithBranches supplies the branches of codec.pick, the interface of T, where codegen emits 2"},
		{"one branch short", pickSchema, pickData, decodeInto[pick](WithBranches[pick](pickA{})),
			"WithBranches supplies 1 branches of codec.pick where codegen emits 2"},
		{"one extra branch", pickSchema, pickData, decodeInto[pick](WithBranches[pick](pickA{}, pickB{}, pickC{})),
			"WithBranches supplies 3 branches of codec.pick where codegen emits 2"},
		{"a pointer branch", pickSchema, pickData, decodeInto[pick](WithBranches[pick](&pickA{}, pickB{})),
			"WithBranches supplies *codec.pickA, no struct, as a branch of codec.pick"},
		{"a nil branch", pickSchema, pickData, decodeInto[pick](WithBranches[pick](pickA{}, nil)),
			"WithBranches supplies a nil branch of codec.pick"},
		{"a branch twice", pickSchema, pickData, decodeInto[pick](WithBranches[pick](pickA{}, pickA{})),
			"WithBranches supplies codec.pickA twice as a branch of codec.pick"},
		{"a branch of no alternative's shape", pickSchema, pickData, decodeInto[pick](WithBranches[pick](pickA{}, pickC{})),
			"branch codec.pickC of codec.pick has the shape of no branch struct codegen emits that an earlier branch did not match"},
		{"two branches of one alternative's shape", pickSchema, pickData, decodeInto[pick](WithBranches[pick](pickA{}, pickA2{})),
			"branch codec.pickA2 of codec.pick has the shape of no branch struct codegen emits that an earlier branch did not match"},
		{"a struct where codegen emits the bare interface", pickSchema, pickData, decodeInto[opt](optBranches),
			"T is codec.opt, no interface, where codegen emits a sealed interface"},
		{"the bare interface where codegen emits a struct", optSchema, pickData, decodeInto[optChoice](optBranches),
			"codec.optChoice is no struct"},
		{"an interface whose method is exported", pickSchema, pickData, decodeInto[exported](),
			"T is codec.exported, which is not an interface of one unexported method, codegen's sealed interface"},
		{"an interface of two methods", pickSchema, pickData, decodeInto[twoMethods](),
			"T is codec.twoMethods, which is not an interface of one unexported method, codegen's sealed interface"},
		{"the choice's field not an interface", attributedSchema, tagged, decodeInto[stringChoice](),
			"field Choice of codec.stringChoice is string, no interface, where codegen emits a sealed interface"},
		{"the choice's field misnamed", attributedSchema, tagged, decodeInto[misnamed](attributedBranches),
			"field 0 of codec.misnamed is C codec.attributedChoice where codegen emits Choice, a sealed interface"},
		{"an attribute field after the choice's", attributedSchema, tagged, decodeInto[attributeShort](attributedBranches),
			"field 1 of codec.attributeShort is Tag string where codegen emits Choice2 *string"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			mustAccept(t, schema, row.data)
			got, err := row.decode(schema, row.data)
			if got != nil || err == nil {
				t.Fatalf("Decode = (%v, %v), want no value and an error", got, err)
			}
			if want := ErrTypeMismatch.Error() + ": " + row.want; err.Error() != want {
				t.Errorf("error = %q, want %q", err, want)
			}
			var de *DeclineError
			if !errors.Is(err, ErrTypeMismatch) || errors.As(err, &de) {
				t.Errorf("error %v: want ErrTypeMismatch alone", err)
			}
		})
	}
}

// TestWithBranches pins WithBranches' option contract: a set for an
// interface T does not hold, or for a type that is no interface, is ignored;
// a later set for the same interface replaces an earlier one, in either
// order; and the branches may come in any order.
func TestWithBranches(t *testing.T) {
	schema := schemaOf(t, pickSchema)
	data := instanceOf("", "<s:b>x</s:b>")
	want := pick(pickB{B: []string{"x"}})
	accepted := []struct {
		name string
		opts []Option
	}{
		{"a set for an interface T does not hold", []Option{optBranches, pickBranches, attributedBranches}},
		{"a set for a type that is no interface", []Option{WithBranches[string]("x"), pickBranches}},
		{"a later set replaces a short one", []Option{WithBranches[pick](pickA{}), pickBranches}},
		{"branches permuted", []Option{WithBranches[pick](pickB{}, pickA{})}},
	}
	for _, row := range accepted {
		t.Run(row.name, func(t *testing.T) {
			got, err := Decode[pick](schema, strict.New(), data, row.opts...)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*got, want) {
				t.Errorf("Decode = %#v, want %#v", *got, want)
			}
		})
	}
	_, err := Decode[pick](schema, strict.New(), data, pickBranches, WithBranches[pick](pickA{}))
	if want := ErrTypeMismatch.Error() + ": WithBranches supplies 1 branches of codec.pick where codegen emits 2"; err == nil || err.Error() != want {
		t.Errorf("Decode under a full set then a short one = %v, want %q: the later set replaces the earlier", err, want)
	}
}

// TestChoiceSharedBranchIsCodegens pins that the shared-branch decline guards
// a shape codegen does emit: two alternatives whose expanded names differ give
// two branch structs of one shape, which no branch type tells apart.
func TestChoiceSharedBranchIsCodegens(t *testing.T) {
	gen := generated(t, schemaOf(t, root(`<xs:choice><xs:element name="x" type="xs:string"/><xs:element name="x" form="unqualified" type="xs:string"/></xs:choice>`)))
	for _, decl := range []string{
		"type RX struct {\n\tX string\n}\n",
		"type RX2 struct {\n\tX string\n}\n",
	} {
		if !bytes.Contains(gen, []byte(decl)) {
			t.Errorf("codegen emits\n%s\nwhich does not declare\n%s", gen, decl)
		}
	}
}
