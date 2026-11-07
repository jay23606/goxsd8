package codec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsderr"
)

// qnameSchema declares s:r of one field per site a QName value takes: q, of
// xs:QName; l, a list of xs:QName; n, of nt, an enumeration of xs:NOTATION
// over the notations png and gif; a ref to h, of xs:QName; e, an element of
// complex type whose attribute at, of xs:QName, is resolved against e's own
// bindings; d, an optional xs:QName element with a default, and f, one with a
// fixed value, each written under p bound to urn:schema; and the attribute
// da, of xs:QName, with a default written under the same binding. s:h is a
// simple root too.
const qnameSchema = `<xs:notation name="png" public="image/png"/><xs:notation name="gif" public="image/gif"/>
<xs:simpleType name="nt"><xs:restriction base="xs:NOTATION"><xs:enumeration value="s:png"/><xs:enumeration value="s:gif"/></xs:restriction></xs:simpleType>
<xs:element name="h" type="xs:QName"/>
<xs:element name="r"><xs:complexType><xs:sequence>
<xs:element name="q" type="xs:QName"/><xs:element name="l"><xs:simpleType><xs:list itemType="xs:QName"/></xs:simpleType></xs:element>
<xs:element name="n" type="s:nt"/><xs:element ref="s:h"/>
<xs:element name="e"><xs:complexType><xs:sequence/><xs:attribute name="at" type="xs:QName"/></xs:complexType></xs:element>
<xs:element name="d" type="xs:QName" default="p:dflt" xmlns:p="urn:schema"/><xs:element name="f" type="xs:QName" fixed="p:fix" xmlns:p="urn:schema"/>
</xs:sequence><xs:attribute name="da" type="xs:QName" default="p:att" xmlns:p="urn:schema"/></xs:complexType></xs:element>`

// tQName is the QName datatype struct codegen emits, and qnames, qnameHead
// and qnameE its R, H and E for qnameSchema, which TestQNameTypesAreCodegens
// pins.
type (
	tQName struct {
		Space string
		Local string
	}
	qnames struct {
		Q  tQName
		L  []tQName
		N  tQName
		H  qnameHead
		E  qnameE
		D  tQName
		F  tQName
		Da tQName
	}
	qnameHead tQName
	qnameE    struct{ At *tQName }
)

// TestQNameTypesAreCodegens pins the types above to those codegen emits for
// qnameSchema: QName's datatype struct for xs:QName and for the xs:NOTATION
// type nt alike, []QName for a list of xs:QName (codegen/doc.go "# Simple
// types").
func TestQNameTypesAreCodegens(t *testing.T) {
	gen := generated(t, schemaOf(t, qnameSchema))
	for _, decl := range []string{
		"type QName struct {\n\tSpace string\n\tLocal string\n}\n",
		"type H QName\n",
		"type E struct {\n\tAt *QName\n}\n",
		"type R struct {\n\tQ  QName\n\tL  []QName\n\tN  QName\n\tH  H\n\tE  E\n\tD  QName\n\tF  QName\n\tDa QName\n}\n",
	} {
		if !bytes.Contains(gen, []byte(decl)) {
			t.Errorf("codegen emits\n%s\nwhich does not declare\n%s", gen, decl)
		}
	}
}

// TestDecodeQNames pins the lift of xs:QName and xs:NOTATION (Datatypes
// §3.3.18, §3.3.19) at every site of qnameSchema: each value is the expanded
// name its lexical resolves to, Space the namespace name and Local the local
// part, no prefix kept. The prefix p is bound to urn:outer on the document
// element and rebound to urn:inner on q, and on e, which carries the
// attribute at, so q and at resolve against their own element's binding,
// never the document element's; h's p:x, written under the document
// element's, is urn:outer. A list's items resolve against one set of
// bindings, an unprefixed one under no default namespace to Space "". n's
// s:png is a NOTATION value in QName's struct. d, f and da are supplied by
// their value constraints and resolve against the schema document's binding
// of p, urn:schema, never the instance's urn:outer (value.ConstraintContext).
// CheckDecode returns nil for qnames.
func TestDecodeQNames(t *testing.T) {
	schema := schemaOf(t, qnameSchema)
	data := []byte(`<s:r xmlns:s="urn:s" xmlns:p="urn:outer">` +
		`<s:q xmlns:p="urn:inner">p:x</s:q><s:l> p:a  s:b c </s:l><s:n>s:png</s:n><s:h>p:x</s:h>` +
		`<s:e xmlns:p="urn:inner" at="p:y"/><s:d/><s:f/></s:r>`)
	mustAccept(t, schema, data)
	got, err := Decode[qnames](schema, strict.New(), data)
	if err != nil {
		t.Fatal(err)
	}
	const want = "{Q:{Space:urn:inner Local:x} L:[{Space:urn:outer Local:a} {Space:urn:s Local:b} {Space: Local:c}] N:{Space:urn:s Local:png}" +
		" H:{Space:urn:outer Local:x} E:{At:{Space:urn:inner Local:y}} D:{Space:urn:schema Local:dflt} F:{Space:urn:schema Local:fix} Da:{Space:urn:schema Local:att}}"
	if r := spellValue(reflect.ValueOf(*got)); r != want {
		t.Errorf("Decode =\n%s\nwant\n%s", r, want)
	}
	if err := CheckDecode[qnames](schema, rootName); err != nil {
		t.Errorf("CheckDecode = %v, want nil", err)
	}
}

// TestDecodeQNameRoots pins a simple root of xs:QName, of an xs:NOTATION
// type and of a list of xs:QName: a prefixed lexical, an unprefixed one
// under a default namespace and one under none, Space "" for none, each into
// codegen's struct, and CheckDecode nil for each row's T.
func TestDecodeQNameRoots(t *testing.T) {
	const notation = `<xs:notation name="png" public="image/png"/><xs:element name="r"><xs:simpleType><xs:restriction base="xs:NOTATION"><xs:enumeration value="s:png"/></xs:restriction></xs:simpleType></xs:element>`
	const list = `<xs:element name="r"><xs:simpleType><xs:list itemType="xs:QName"/></xs:simpleType></xs:element>`
	qname := `<xs:element name="r" type="xs:QName"/>`
	rows := []struct {
		name, decls, data string
		into              target
		want              string
	}{
		{"prefixed", qname, `<s:r xmlns:s="urn:s" xmlns:p="urn:p">p:x</s:r>`, into[tQName](), "{Space:urn:p Local:x}"},
		{"unprefixed, under a default namespace", qname, `<s:r xmlns:s="urn:s" xmlns="urn:d"> x </s:r>`, into[tQName](), "{Space:urn:d Local:x}"},
		{"unprefixed, under none", qname, `<s:r xmlns:s="urn:s">x</s:r>`, into[tQName](), "{Space: Local:x}"},
		{"unprefixed, its default namespace undeclared", qname, `<s:r xmlns:s="urn:s" xmlns="">x</s:r>`, into[tQName](), "{Space: Local:x}"},
		{"NOTATION, unprefixed under a default namespace", notation, `<r xmlns="urn:s">png</r>`, into[qnameHead](), "{Space:urn:s Local:png}"},
		{"a list", list, `<s:r xmlns:s="urn:s" xmlns="urn:d">s:x y</s:r>`, into[[]tQName](), "[{Space:urn:s Local:x} {Space:urn:d Local:y}]"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			mustAccept(t, schema, []byte(row.data))
			got, err := row.into.decode(schema, []byte(row.data))
			if err != nil {
				t.Fatal(err)
			}
			if r := spellValue(reflect.ValueOf(got)); r != row.want {
				t.Errorf("Decode = %s, want %s", r, row.want)
			}
			if err := row.into.check(schema); err != nil {
				t.Errorf("CheckDecode = %v, want nil", err)
			}
		})
	}
}

// TestDecodeQNameTypeMismatch pins item 6 over QName's datatype struct: a
// struct whose fields differ from codegen's QName, in order, in number or in
// Go type, and a simple root of string, are each ErrTypeMismatch, no decline;
// the message spells codegen's struct by xs:QName and its fields, for an
// xs:NOTATION type too.
func TestDecodeQNameTypeMismatch(t *testing.T) {
	type swapped struct {
		Local string
		Space string
	}
	type prefixed struct {
		Prefix string
		Space  string
		Local  string
	}
	type bytesLocal struct {
		Space string
		Local []byte
	}
	const qname = "D for the datatype struct D of xs:QName (Space string, Local string)"
	elem := sequenceOf(`<xs:element name="a" type="xs:QName"/>`)
	data := instanceOf("", "<s:a>s:x</s:a>")
	rows := []struct {
		name, decls string
		data        []byte
		into        target
		want        string
	}{
		{"Space and Local swapped", elem, data, into[struct{ A swapped }](), "field 0 of struct { A codec.swapped } is A codec.swapped where codegen emits A " + qname},
		{"a Prefix field", elem, data, into[struct{ A prefixed }](), "field 0 of struct { A codec.prefixed } is A codec.prefixed where codegen emits A " + qname},
		{"Local []byte", elem, data, into[struct{ A bytesLocal }](), "field 0 of struct { A codec.bytesLocal } is A codec.bytesLocal where codegen emits A " + qname},
		{"a simple root of string", `<xs:element name="r" type="xs:QName"/>`, instanceOf("", "s:x"), into[string](), "the underlying type of string is not " + qname},
		{"a NOTATION root of string", `<xs:notation name="png" public="image/png"/><xs:element name="r"><xs:simpleType><xs:restriction base="xs:NOTATION"><xs:enumeration value="s:png"/></xs:restriction></xs:simpleType></xs:element>`,
			instanceOf("", "s:png"), into[string](), "the underlying type of string is not " + qname},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			mustAccept(t, schema, row.data)
			got, err := row.into.decode(schema, row.data)
			if got != nil {
				t.Errorf("Decode = %v, want no value", got)
			}
			if want := ErrTypeMismatch.Error() + ": " + row.want; err == nil || err.Error() != want {
				t.Errorf("error = %v\nwant    %q", err, want)
			}
			var de *DeclineError
			if !errors.Is(err, ErrTypeMismatch) || errors.As(err, &de) {
				t.Errorf("error %v: want ErrTypeMismatch alone", err)
			}
			assertProbeMismatch(t, row.into.check(schema), err)
		})
	}
}

// TestWithLoggerQName pins the conversion record of a QName, read from its
// lexical: rule "dt-lexical-mapping", input the normalized value, "p:x".
func TestWithLoggerQName(t *testing.T) {
	schema := schemaOf(t, sequenceOf(`<xs:element name="a" type="xs:QName"/>`))
	data := instanceOf(` xmlns:p="urn:p"`, "<s:a> p:x </s:a>")
	var buf bytes.Buffer
	if _, err := Decode[struct{ A tQName }](schema, strict.New(), data, WithLogger(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))); err != nil {
		t.Fatal(err)
	}
	var got []traceRecord
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var tr traceRecord
		if err := json.Unmarshal(line, &tr); err != nil {
			t.Fatalf("record %s: %v", line, err)
		}
		if tr.Codec != nil {
			got = append(got, tr)
		}
	}
	const qname = "{http://www.w3.org/2001/XMLSchema}QName"
	loc := fmt.Sprintf("?:1:%d", bytes.Index(data, []byte("<s:a>"))+len("<s:a>")+1)
	want := []traceRecord{
		{"whiteSpace", map[string]string{"rule": "rf-whiteSpace", "type": qname, "input": " p:x ", "loc": loc}},
		{"lexical mapping", map[string]string{"rule": "cvc-datatype-valid", "clause": "2.1", "type": qname, "input": "p:x", "loc": loc}},
		{"conversion", map[string]string{"rule": "dt-lexical-mapping", "type": qname, "input": "p:x", "loc": loc}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("codec records = %+v\nwant %+v", got, want)
	}
}

// TestDecodeQNameStageError pins item 8 over QName's datatype struct, which
// reads no Canonical: a backend whose Parse accepts a lexical that resolves
// to no expanded name against the bindings it was given, here u:x with u
// unbound, fails the conversion, spelling the lexical and no Canonical.
func TestDecodeQNameStageError(t *testing.T) {
	const tail = "; validate accepted the value on the same backend, so this is a fault of the backend, not of the instance"
	unbound := overDatatype("QName", func(base value.Mapping) value.Mapping {
		return value.Mapping{Parse: func(lexical string, ctx value.Context) (value.Value, error) {
			if lexical == "u:x" {
				return base.Parse("s:x", ctx)
			}
			return base.Parse(lexical, ctx)
		}}
	})
	schema := schemaOf(t, sequenceOf(`<xs:element name="a" type="xs:QName"/>`))
	data := instanceOf("", "<s:a>u:x</s:a>")
	offset := bytes.Index(data, []byte("<s:a>")) + len("<s:a>")
	err := decodeErr[struct{ A tQName }](schema, unbound, data)
	var se *stageError
	if !errors.As(err, &se) || se.stage != stageConversion {
		t.Fatalf("Decode error = %v, want a stage error of %s", err, stageConversion.name())
	}
	want := fmt.Sprintf(`codec: ?:1:%d: the conversion to the Go type failed on "u:x", a value of {http://www.w3.org/2001/XMLSchema}QName in the character content of element {urn:s}a at byte offset %d: it resolves to no expanded name against the bindings in scope`, offset+1, offset) + tail
	if err.Error() != want {
		t.Errorf("error = %q\nwant    %q", err, want)
	}
	var xe *xsderr.Error
	if errors.As(err, &xe) || errors.Is(err, ErrTypeMismatch) || errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("error %v wraps something", err)
	}
}
