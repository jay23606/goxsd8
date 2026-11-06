package codec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// decimalOver is strict.New() with xs:decimal's mapping, which governs every
// integer type, replaced by m(strict's own).
func decimalOver(m func(base value.Mapping) value.Mapping) value.Backend {
	decimal := xsd.QName{Space: xsd.XMLSchemaNS, Local: "decimal"}
	base, _ := strict.New().Mapping(decimal)
	return value.Override(strict.New(), oneMapping{name: decimal, m: m(base)})
}

// oneMapping is a value.Backend mapping name alone.
type oneMapping struct {
	name xsd.QName
	m    value.Mapping
}

func (o oneMapping) Mapping(typ xsd.QName) (value.Mapping, bool) {
	if typ == o.name {
		return o.m, true
	}
	return value.Mapping{}, false
}

// The backends of TestDecodeStageErrors. noContext's Parse rejects, when
// handed no Context, each literal rowLiterals holds: validate hands Parse the
// instance's Context for an instance value and the schema document's for a
// value constraint's, and Decode none (value.Context's nil), so validate
// accepts what Decode's lexical mapping rejects. A nil Context also reaches
// Parse for the {value} of xs:int's bound facets, which rowLiterals does not
// hold. The others break the governing mapping's Canonical: nil, failing,
// rendering a form int32 does not read, or, xs:float's in beyondFloat32,
// rendering a finite form beyond float32's range.
var (
	rowLiterals = []string{"01", "1", "2", "3", "5"}
	noContext   = decimalOver(func(base value.Mapping) value.Mapping {
		return value.Mapping{Parse: func(lexical string, ctx value.Context) (value.Value, error) {
			if ctx == nil && slices.Contains(rowLiterals, lexical) {
				return nil, xsderr.New("cvc-datatype-valid", xsderr.Loc{}, "no Context")
			}
			return base.Parse(lexical, ctx)
		}, Canonical: base.Canonical}
	})
	nilCanonical = decimalOver(func(base value.Mapping) value.Mapping {
		return value.Mapping{Parse: base.Parse}
	})
	failingCanonical = decimalOver(func(base value.Mapping) value.Mapping {
		return value.Mapping{Parse: base.Parse, Canonical: func(value.Value) (string, error) { return "", errors.New("no canonical form") }}
	})
	fractionCanonical = decimalOver(func(base value.Mapping) value.Mapping {
		return value.Mapping{Parse: base.Parse, Canonical: func(value.Value) (string, error) { return "1.5", nil }}
	})
	beyondFloat32 = func() value.Backend {
		float := xsd.QName{Space: xsd.XMLSchemaNS, Local: "float"}
		base, _ := strict.New().Mapping(float)
		return value.Override(strict.New(), oneMapping{name: float, m: value.Mapping{Parse: base.Parse, Canonical: func(value.Value) (string, error) { return "1e39", nil }}})
	}()
)

// TestDecodeStageErrors pins item 8 of Decode's doc: a lexical mapping or a
// conversion to the Go type that fails on a value validate accepted on the
// same backend is a stage error naming the stage, the type (its QName, or the
// declaration an anonymous one is local to), the normalized lexical, and the
// value's source: an element's character content with its first character
// data's Loc and byte offset, a present attribute's Loc, or a supplied
// value's element Loc and the declaration whose value constraint supplied it.
// It is no decline, no undecided error and no charge, and it unwraps to
// nothing.
func TestDecodeStageErrors(t *testing.T) {
	const tail = "; validate accepted the value on the same backend, so this is a fault of the backend, not of the instance"
	const intQName = "{http://www.w3.org/2001/XMLSchema}int"
	element := sequenceOf(`<xs:element name="a" type="xs:int"/>`)
	elementData := instanceOf("", "<s:a> 01 </s:a>")
	offset := bytes.Index(elementData, []byte("<s:a>")) + len("<s:a>")
	attributes := root(`<xs:sequence><xs:element name="z" type="xs:string" minOccurs="0"/></xs:sequence><xs:attribute name="b" type="xs:int"/><xs:attribute name="c" type="xs:int" default=" 3 "/>`)
	rows := []struct {
		name, decls string
		data        []byte
		backend     value.Backend
		decode      func(*xsd.Schema, value.Backend, []byte) error
		stage       stage
		want        string
	}{
		{"lexical mapping, character content", element, elementData, noContext, decodeErr[struct{ A int32 }], stageLexicalMapping,
			fmt.Sprintf(`codec: ?:1:%d: the lexical mapping failed on "01", a value of %s in the character content of element {urn:s}a at byte offset %d: ?: [cvc-datatype-valid] no Context`, offset+1, intQName, offset)},
		{"lexical mapping, a present attribute", attributes, instanceOf(` b=" 2 " c="4"`, ""), noContext, decodeErr[attributed3], stageLexicalMapping,
			fmt.Sprintf(`codec: ?:1:%d: the lexical mapping failed on "2", a value of %s in attribute b: ?: [cvc-datatype-valid] no Context`, bytes.Index(instanceOf(` b=" 2 " c="4"`, ""), []byte(" b="))+2, intQName)},
		{"lexical mapping, a supplied value", attributes, instanceOf("", ""), noContext, decodeErr[attributed3], stageLexicalMapping,
			`codec: ?:1:1: the lexical mapping failed on "3", a value of ` + intQName + ` in the value the value constraint of attribute c declared at s.xsd:1:336 supplies on element {urn:s}r: ?: [cvc-datatype-valid] no Context`},
		{"lexical mapping, an anonymous type", sequenceOf(`<xs:element name="a"><xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType></xs:element>`), elementData, noContext,
			decodeErr[struct{ A int32 }], stageLexicalMapping,
			fmt.Sprintf(`codec: ?:1:%d: the lexical mapping failed on "01", a value of the anonymous simple type of element {urn:s}a declared at s.xsd:1:231 in the character content of element {urn:s}a at byte offset %d: ?: [cvc-datatype-valid] no Context`, offset+1, offset)},
		{"lexical mapping, a simple root", `<xs:element name="r" type="xs:int"/>`, instanceOf("", "5"), noContext, decodeErr[int32], stageLexicalMapping,
			fmt.Sprintf(`codec: ?:1:%d: the lexical mapping failed on "5", a value of %s in the character content of element {urn:s}r at byte offset %d: ?: [cvc-datatype-valid] no Context`, len(instanceOf("", ""))-len("</s:r>")+1, intQName, len(instanceOf("", ""))-len("</s:r>"))},
		{"lexical mapping, an anonymous simple root", `<xs:element name="r"><xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType></xs:element>`, instanceOf("", "5"), noContext, decodeErr[int32], stageLexicalMapping,
			fmt.Sprintf(`codec: ?:1:%d: the lexical mapping failed on "5", a value of the anonymous simple type of element {urn:s}r declared at s.xsd:1:181 in the character content of element {urn:s}r at byte offset %d: ?: [cvc-datatype-valid] no Context`, len(instanceOf("", ""))-len("</s:r>")+1, len(instanceOf("", ""))-len("</s:r>"))},
		{"conversion, no Canonical", element, elementData, nilCanonical, decodeErr[struct{ A int32 }], stageConversion,
			fmt.Sprintf(`codec: ?:1:%d: the conversion to the Go type failed on "01", a value of %s in the character content of element {urn:s}a at byte offset %d: the governing mapping has no canonical mapping`, offset+1, intQName, offset)},
		{"conversion, Canonical fails", element, elementData, failingCanonical, decodeErr[struct{ A int32 }], stageConversion,
			fmt.Sprintf(`codec: ?:1:%d: the conversion to the Go type failed on "01", a value of %s in the character content of element {urn:s}a at byte offset %d: no canonical form`, offset+1, intQName, offset)},
		{"conversion, a form the Go type does not read", element, elementData, fractionCanonical, decodeErr[struct{ A int32 }], stageConversion,
			fmt.Sprintf(`codec: ?:1:%d: the conversion to the Go type failed on "01", a value of %s in the character content of element {urn:s}a at byte offset %d: canonical form "1.5" is no int32: strconv.ParseInt: parsing "1.5": invalid syntax`, offset+1, intQName, offset)},
		{"conversion, a finite form beyond the Go type's range", sequenceOf(`<xs:element name="a" type="xs:float"/>`), elementData, beyondFloat32, decodeErr[struct{ A float32 }], stageConversion,
			fmt.Sprintf(`codec: ?:1:%d: the conversion to the Go type failed on "01", a value of {http://www.w3.org/2001/XMLSchema}float in the character content of element {urn:s}a at byte offset %d: canonical form "1e39" is no float32: strconv.ParseFloat: parsing "1e39": value out of range`, offset+1, offset)},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			err := row.decode(schemaOf(t, row.decls), row.backend, row.data)
			var se *stageError
			if !errors.As(err, &se) || se.stage != row.stage {
				t.Fatalf("Decode error = %v, want a stage error of %s", err, row.stage.name())
			}
			if want := row.want + tail; err.Error() != want {
				t.Errorf("error = %q\nwant    %q", err, want)
			}
			var de *DeclineError
			var ue *UndecidedError
			var xe *xsderr.Error
			if errors.As(err, &de) || errors.As(err, &ue) || errors.As(err, &xe) || ViolationsOf(err) != nil {
				t.Errorf("error %v is a decline, an undecided error, an *xsderr.Error or a charge", err)
			}
			if _, isRule := xsderr.RuleOf(err); isRule || errors.Is(err, errors.ErrUnsupported) || errors.Is(err, ErrTypeMismatch) {
				t.Errorf("error %v carries a rule, ErrUnsupported or ErrTypeMismatch", err)
			}
		})
	}
}

// attributed3 is the struct codegen emits for TestDecodeStageErrors'
// attributes schema, by the shapes TestAtomicTypesAreCodegens pins.
type attributed3 struct {
	Z *string
	B *int32
	C int32
}

// decodeErr decodes data against schema on backend into T and returns
// Decode's error.
func decodeErr[T any](schema *xsd.Schema, backend value.Backend, data []byte) error {
	got, err := Decode[T](schema, backend, data)
	if got != nil {
		return fmt.Errorf("Decode returned a value, %v, with error %w", *got, err)
	}
	return err
}

// TestDecodeStageErrorOrder pins the order of item 8 against item 7: the
// first in document order is returned, an element's xsi:type before its
// attributes' values, a present attribute's before an absent one's supplied
// value, and an attribute's before a child's.
func TestDecodeStageErrorOrder(t *testing.T) {
	schema := schemaOf(t, `<xs:complexType name="rt"><xs:sequence><xs:element name="a" type="xs:int"/></xs:sequence>`+
		`<xs:attribute name="b" type="xs:int" default="3"/><xs:attribute name="c" type="xs:int"/></xs:complexType><xs:element name="r" type="s:rt"/>`)
	type r struct {
		A int32
		B int32
		C *int32
	}
	rows := []struct {
		name, attrs, body string
		want              string
	}{
		{"xsi:type before an attribute's value", ` c="1" xsi:type="s:rt"`, "<s:a>1</s:a>", "codec does not decode an element carrying xsi:type"},
		{"a present attribute before a supplied value", ` c="1"`, "<s:a>1</s:a>", `failed on "1", a value of {http://www.w3.org/2001/XMLSchema}int in attribute c:`},
		{"a supplied value before a child", "", "<s:a>1</s:a>", `failed on "3", a value of {http://www.w3.org/2001/XMLSchema}int in the value the value constraint of attribute b`},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			err := decodeErr[r](schema, noContext, instanceOf(row.attrs, row.body))
			if err == nil || !strings.Contains(err.Error(), row.want) {
				t.Errorf("error = %v, want one holding %q", err, row.want)
			}
		})
	}
	// A supplied value whose attribute is present is never mapped: noContext
	// rejects b's supplied 3 and maps every value this instance carries.
	got, err := Decode[r](schema, noContext, instanceOf(` b="4"`, "<s:a>7</s:a>"))
	if err != nil || *got != (r{A: 7, B: 4}) {
		t.Errorf("Decode = %v, %v; want {7 4 nil}", got, err)
	}
}

// traceRecord is one codec record a JSON handler writes.
type traceRecord struct {
	Msg   string            `json:"msg"`
	Codec map[string]string `json:"codec"`
}

// TestWithLogger pins WithLogger's contract: Decode logs one Debug record per
// stage it runs on each value under the "codec" group, with the stage's rule,
// the type, the stage's input and the instance Loc, a string value its
// whiteSpace stage alone; it hands the logger to validate, whose records sit
// under "validate"; a later WithLogger replaces an earlier, and nil is
// silent.
func TestWithLogger(t *testing.T) {
	schema := schemaOf(t, sequenceOf(`<xs:element name="a" type="xs:int"/><xs:element name="b" type="xs:string"/>`))
	data := instanceOf("", "<s:a> 01 </s:a><s:b>x</s:b>")
	var earlier, buf bytes.Buffer
	debug := &slog.HandlerOptions{Level: slog.LevelDebug}
	if _, err := Decode[struct {
		A int32
		B string
	}](schema, strict.New(), data, WithLogger(slog.New(slog.NewJSONHandler(&earlier, debug))), WithLogger(slog.New(slog.NewJSONHandler(&buf, debug)))); err != nil {
		t.Fatal(err)
	}
	if earlier.Len() != 0 {
		t.Errorf("the replaced logger was written:\n%s", earlier.String())
	}
	var got []traceRecord
	validateLines := 0
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("record %s: %v", line, err)
		}
		if _, ok := rec["validate"]; ok {
			validateLines++
		}
		if _, ok := rec["codec"]; !ok {
			continue
		}
		var tr traceRecord
		if err := json.Unmarshal(line, &tr); err != nil {
			t.Fatalf("record %s: %v", line, err)
		}
		got = append(got, tr)
	}
	const xsInt, xsString = "{http://www.w3.org/2001/XMLSchema}int", "{http://www.w3.org/2001/XMLSchema}string"
	aLoc := fmt.Sprintf("?:1:%d", bytes.Index(data, []byte("<s:a>"))+len("<s:a>")+1)
	bLoc := fmt.Sprintf("?:1:%d", bytes.Index(data, []byte("<s:b>"))+len("<s:b>")+1)
	want := []traceRecord{
		{"whiteSpace", map[string]string{"rule": "rf-whiteSpace", "type": xsInt, "input": " 01 ", "loc": aLoc}},
		{"lexical mapping", map[string]string{"rule": "cvc-datatype-valid", "clause": "2.1", "type": xsInt, "input": "01", "loc": aLoc}},
		{"conversion", map[string]string{"rule": "dt-canonical-mapping", "type": xsInt, "input": "1", "loc": aLoc}},
		{"whiteSpace", map[string]string{"rule": "rf-whiteSpace", "type": xsString, "input": "x", "loc": bLoc}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("codec records = %+v\nwant %+v", got, want)
	}
	if validateLines == 0 {
		t.Errorf("no record under the validate group: the logger did not reach validate")
	}
}

// TestDecodeLogsNothingByDefault pins that Decode with no WithLogger, or with
// WithLogger(nil), writes nothing, not even to slog's default logger.
func TestDecodeLogsNothingByDefault(t *testing.T) {
	var buf bytes.Buffer
	defer slog.SetDefault(slog.Default())
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	schema := schemaOf(t, sequenceOf(`<xs:element name="a" type="xs:int"/>`))
	data := instanceOf("", "<s:a>1</s:a>")
	for _, opts := range [][]Option{nil, {WithLogger(nil)}} {
		if _, err := Decode[struct{ A int32 }](schema, strict.New(), data, opts...); err != nil {
			t.Fatal(err)
		}
	}
	if buf.Len() != 0 {
		t.Errorf("Decode wrote to slog's default logger:\n%s", buf.String())
	}
}
