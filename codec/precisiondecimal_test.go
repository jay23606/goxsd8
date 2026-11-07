package codec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsderr"
)

// decimalSchema declares s:r of one field per site a precisionDecimal value
// takes: a, repeated; l, a list; refs to h, of xs:precisionDecimal, and hl, a
// list of it; o, optional; c, a choice whose alternative x is of
// xs:precisionDecimal; and the attributes at, optional, and d, with a
// default. s:h and s:hl are simple roots too.
const decimalSchema = `<xs:element name="h" type="xs:precisionDecimal"/>
<xs:element name="hl"><xs:simpleType><xs:list itemType="xs:precisionDecimal"/></xs:simpleType></xs:element>
<xs:element name="r"><xs:complexType><xs:sequence>
<xs:element name="a" type="xs:precisionDecimal" maxOccurs="unbounded"/>
<xs:element name="l"><xs:simpleType><xs:list itemType="xs:precisionDecimal"/></xs:simpleType></xs:element>
<xs:element ref="s:h"/><xs:element ref="s:hl"/>
<xs:element name="o" type="xs:precisionDecimal" minOccurs="0"/>
<xs:element name="c"><xs:complexType><xs:choice><xs:element name="x" type="xs:precisionDecimal"/><xs:element name="y" type="xs:string"/></xs:choice></xs:complexType></xs:element>
</xs:sequence><xs:attribute name="at" type="xs:precisionDecimal"/><xs:attribute name="d" type="xs:precisionDecimal" default="1.50"/></xs:complexType></xs:element>`

// tPrecisionDecimal, its Value interface tPDValue and the four arms are the
// datatype struct codegen emits for xs:precisionDecimal, and decimals, tH,
// tHl, tC, tCX and tCY its R, H, Hl, C, CX and CY for decimalSchema, which
// TestDecimalTypesAreCodegens pins.
type (
	tPrecisionDecimal struct{ Value tPDValue }
	tPDValue          interface{ isPrecisionDecimalValue() }
	tPDNumerical      struct {
		Coefficient big.Int
		Scale       big.Int
		Negative    bool
	}
	tPDPositiveInfinity struct{}
	tPDNegativeInfinity struct{}
	tPDNotANumber       struct{}
	decimals            struct {
		A  []tPrecisionDecimal
		L  []tPrecisionDecimal
		H  tH
		Hl tHl
		O  *tPrecisionDecimal
		C  tC
		At *tPrecisionDecimal
		D  tPrecisionDecimal
	}
	tH  tPrecisionDecimal
	tHl []tPrecisionDecimal
	tC  interface{ isC() }
	tCX struct{ X tPrecisionDecimal }
	tCY struct{ Y string }
)

func (tPDNumerical) isPrecisionDecimalValue()        {}
func (tPDPositiveInfinity) isPrecisionDecimalValue() {}
func (tPDNegativeInfinity) isPrecisionDecimalValue() {}
func (tPDNotANumber) isPrecisionDecimalValue()       {}
func (tCX) isC()                                     {}
func (tCY) isC()                                     {}

// pdArms is the WithPrecisionDecimalArms of tPDValue, its arms in their
// roles.
var pdArms = WithPrecisionDecimalArms[tPDValue](tPDNumerical{}, tPDPositiveInfinity{}, tPDNegativeInfinity{}, tPDNotANumber{})

// TestDecimalTypesAreCodegens pins the types above to those codegen emits for
// decimalSchema (codegen/doc.go "# Simple types").
func TestDecimalTypesAreCodegens(t *testing.T) {
	gen := generated(t, schemaOf(t, decimalSchema))
	for _, decl := range []string{
		"type PrecisionDecimal struct {\n\tValue PrecisionDecimalValue\n}\n",
		"type PrecisionDecimalValue interface {\n\tisPrecisionDecimalValue()\n}\n",
		"type PrecisionDecimalValueNumerical struct {\n\tCoefficient big.Int\n\tScale       big.Int\n\tNegative    bool\n}\n",
		"type PrecisionDecimalValuePositiveInfinity struct{}\n",
		"type PrecisionDecimalValueNegativeInfinity struct{}\n",
		"type PrecisionDecimalValueNotANumber struct{}\n",
		"type H PrecisionDecimal\n", "type Hl []PrecisionDecimal\n",
		"type R struct {\n\tA  []PrecisionDecimal\n\tL  []PrecisionDecimal\n\tH  H\n\tHl Hl\n\tO  *PrecisionDecimal\n\tC  C\n\tAt *PrecisionDecimal\n\tD  PrecisionDecimal\n}\n",
		"type CX struct {\n\tX PrecisionDecimal\n}\n", "type CY struct {\n\tY string\n}\n",
	} {
		if !bytes.Contains(gen, []byte(decl)) {
			t.Errorf("codegen emits\n%s\nwhich does not declare\n%s", gen, decl)
		}
	}
}

// spellDecimal spells v, a tPrecisionDecimal or a type over one, by its
// arm: N(Coefficient,Scale) for a numerical value, a leading '-' when
// Negative, and +INF, -INF, NaN or nil.
func spellDecimal(v reflect.Value) string {
	switch arm := v.Field(0).Interface().(type) {
	case tPDNumerical:
		sign := ""
		if arm.Negative {
			sign = "-"
		}
		return fmt.Sprintf("%sN(%s,%s)", sign, arm.Coefficient.String(), arm.Scale.String())
	case tPDPositiveInfinity:
		return "+INF"
	case tPDNegativeInfinity:
		return "-INF"
	case tPDNotANumber:
		return "NaN"
	case nil:
		return "nil"
	}
	return fmt.Sprintf("%T", v.Field(0).Interface())
}

// spellDecimals spells v, a slice of PrecisionDecimal structs, by
// spellDecimal, in brackets.
func spellDecimals(v reflect.Value) string {
	parts := make([]string, v.Len())
	for i := range parts {
		parts[i] = spellDecimal(v.Index(i))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// TestDecodePrecisionDecimalValues pins the lexical reading of
// xs:precisionDecimal (precisionDecimal §6 f-precDecLexmap) on a simple root:
// Coefficient the digits, Scale the ·scale·, which may be negative, Negative
// the ·sign·, so 3 and 3.00 differ in Scale and 0 and -0 in Negative, and a
// zero of scale 0 and 3E2 of scale -2, which strict's Canonical form gets
// wrong; INF and +INF are one arm, -INF and NaN their own. CheckDecode
// returns nil for the root's T.
func TestDecodePrecisionDecimalValues(t *testing.T) {
	schema := schemaOf(t, `<xs:element name="r" type="xs:precisionDecimal"/>`)
	rows := []struct{ lexical, want string }{
		{"3", "N(3,0)"},
		{"3.00", "N(300,2)"},
		{"0", "N(0,0)"},
		{"-0", "-N(0,0)"},
		{"0E5", "N(0,-5)"},
		{"3E2", "N(3,-2)"},
		{"3.0e2", "N(30,-1)"},
		{"30e1", "N(30,-1)"},
		{"1E7", "N(1,-7)"},
		{"-1.5E-3", "-N(15,4)"},
		{"1.5E+2", "N(15,-1)"},
		{"+.50", "N(50,2)"},
		{"7.", "N(7,0)"},
		{" 12345678901234567890.1 ", "N(123456789012345678901,1)"},
		{"INF", "+INF"},
		{"+INF", "+INF"},
		{"-INF", "-INF"},
		{"NaN", "NaN"},
	}
	for _, row := range rows {
		t.Run(row.lexical, func(t *testing.T) {
			data := instanceOf("", row.lexical)
			mustAccept(t, schema, data)
			got, err := Decode[tPrecisionDecimal](schema, strict.New(), data, pdArms)
			if err != nil {
				t.Fatal(err)
			}
			if s := spellDecimal(reflect.ValueOf(*got)); s != row.want {
				t.Errorf("Decode(%q) = %s, want %s", row.lexical, s, row.want)
			}
		})
	}
	if err := CheckDecode[tPrecisionDecimal](schema, rootName, pdArms); err != nil {
		t.Errorf("CheckDecode = %v, want nil", err)
	}
}

// TestDecodePrecisionDecimals pins every site of decimalSchema: a repeated
// local element, a list, refs to an element and to a list, an optional
// element, a choice's alternative, a present attribute and a defaulted
// absent one, and the simple roots of h, its N, and of hl, a list.
// CheckDecode returns nil for each T.
func TestDecodePrecisionDecimals(t *testing.T) {
	schema := schemaOf(t, decimalSchema)
	opts := []Option{pdArms, WithBranches[tC](tCX{}, tCY{})}
	data := instanceOf(` at="-0"`, `<s:a>3</s:a><s:a>3.00</s:a><s:l> -INF 0 .5 </s:l><s:h>NaN</s:h><s:hl>INF 3E2</s:hl><s:o>-1</s:o><s:c><s:x>+INF</s:x></s:c>`)
	mustAccept(t, schema, data)
	got, err := Decode[decimals](schema, strict.New(), data, opts...)
	if err != nil {
		t.Fatal(err)
	}
	x, _ := got.C.(tCX)
	r := reflect.ValueOf(*got)
	gotSpelled := fmt.Sprintf("A:%s L:%s H:%s Hl:%s O:%s C.X:%s At:%s D:%s", spellDecimals(r.Field(0)), spellDecimals(r.Field(1)), spellDecimal(r.Field(2)),
		spellDecimals(r.Field(3)), spellDecimal(r.Field(4).Elem()), spellDecimal(reflect.ValueOf(x.X)), spellDecimal(r.Field(6).Elem()), spellDecimal(r.Field(7)))
	const want = "A:[N(3,0) N(300,2)] L:[-INF N(0,0) N(5,1)] H:NaN Hl:[+INF N(3,-2)] O:-N(1,0) C.X:+INF At:-N(0,0) D:N(150,2)"
	if gotSpelled != want {
		t.Errorf("Decode =\n%s\nwant\n%s", gotSpelled, want)
	}
	if err := CheckDecode[decimals](schema, rootName, opts...); err != nil {
		t.Errorf("CheckDecode = %v, want nil", err)
	}
	roots := []struct {
		name, decls, content string
		into                 target
		spell                func(reflect.Value) string
		want                 string
	}{
		{"h's N", `<xs:element name="r" type="xs:precisionDecimal"/>`, "-3.10", into[tH](pdArms), spellDecimal, "-N(310,2)"},
		{"a restriction", `<xs:element name="r"><xs:simpleType><xs:restriction base="xs:precisionDecimal"><xs:maxScale value="3"/></xs:restriction></xs:simpleType></xs:element>`,
			"2.500", into[tPrecisionDecimal](pdArms), spellDecimal, "N(2500,3)"},
		{"a list", `<xs:element name="r"><xs:simpleType><xs:list itemType="xs:precisionDecimal"/></xs:simpleType></xs:element>`, "1 NaN", into[tHl](pdArms), spellDecimals, "[N(1,0) NaN]"},
		{"an empty list", `<xs:element name="r"><xs:simpleType><xs:list itemType="xs:precisionDecimal"/></xs:simpleType></xs:element>`, "", into[[]tPrecisionDecimal](pdArms), spellDecimals, "[]"},
	}
	for _, row := range roots {
		t.Run(row.name, func(t *testing.T) {
			schema := schemaOf(t, row.decls)
			data := instanceOf("", row.content)
			mustAccept(t, schema, data)
			v, err := row.into.decode(schema, data)
			if err != nil {
				t.Fatal(err)
			}
			if s := row.spell(reflect.ValueOf(v)); s != row.want {
				t.Errorf("Decode = %s, want %s", s, row.want)
			}
			if err := row.into.check(schema); err != nil {
				t.Errorf("CheckDecode = %v, want nil", err)
			}
		})
	}
}

// TestDecodePrecisionDecimalTypeMismatch pins item 6 over PrecisionDecimal: a
// struct of a field of another name or Go type, or whose Value is no
// interface of one unexported method, a simple root of string, no
// WithPrecisionDecimalArms for the Value interface, a WithBranches set in its
// place, a nil arm, a numerical arm of another shape, a special arm with a
// field, and one type as two arms are each ErrTypeMismatch, no decline,
// CheckDecode's error too.
func TestDecodePrecisionDecimalTypeMismatch(t *testing.T) {
	type renamed struct{ V tPDValue }
	type valueInt struct{ Value int }
	type valueAny struct{ Value any }
	type exported interface{ IsValue() }
	type valueExported struct{ Value exported }
	const pd = "D for the datatype struct D of xs:precisionDecimal (Value I, I a sealed interface whose arms WithPrecisionDecimalArms supplies)"
	const iface = "codec.tPDValue"
	elem := sequenceOf(`<xs:element name="a" type="xs:precisionDecimal"/>`)
	data := instanceOf("", "<s:a>1.0</s:a>")
	field := func(typ string) string {
		return "field 0 of struct { A " + typ + " } is A " + typ + " where codegen emits A " + pd
	}
	site := "field A of struct { A codec.tPrecisionDecimal }"
	rows := []struct {
		name, decls string
		data        []byte
		into        target
		want        string
	}{
		{"a field V", elem, data, into[struct{ A renamed }](pdArms), field("codec.renamed")},
		{"a Value int", elem, data, into[struct{ A valueInt }](pdArms), field("codec.valueInt")},
		{"a Value of no method", elem, data, into[struct{ A valueAny }](pdArms), field("codec.valueAny")},
		{"a Value of an exported method", elem, data, into[struct{ A valueExported }](pdArms), field("codec.valueExported")},
		{"a simple root of string", `<xs:element name="r" type="xs:precisionDecimal"/>`, instanceOf("", "1.0"), into[string](pdArms), "the underlying type of string is not " + pd},
		{"no arms", elem, data, into[struct{ A tPrecisionDecimal }](),
			"no WithPrecisionDecimalArms supplies the arms of " + iface + ", the Value interface of " + site},
		{"a WithBranches set in place of the arms", elem, data,
			into[struct{ A tPrecisionDecimal }](WithBranches[tPDValue](tPDNumerical{}, tPDPositiveInfinity{}, tPDNegativeInfinity{}, tPDNotANumber{})),
			"no WithPrecisionDecimalArms supplies the arms of " + iface + ", the Value interface of " + site},
		{"no arms, at a simple root", `<xs:element name="r" type="xs:precisionDecimal"/>`, instanceOf("", "1.0"), into[tPrecisionDecimal](),
			"no WithPrecisionDecimalArms supplies the arms of " + iface + ", the Value interface of T"},
		{"no arms, for a list's element", sequenceOf(`<xs:element name="a"><xs:simpleType><xs:list itemType="xs:precisionDecimal"/></xs:simpleType></xs:element>`), data,
			into[struct{ A []tPrecisionDecimal }](), "no WithPrecisionDecimalArms supplies the arms of " + iface + ", the Value interface of field A of struct { A []codec.tPrecisionDecimal }"},
		{"a nil notANumber arm", elem, data,
			into[struct{ A tPrecisionDecimal }](WithPrecisionDecimalArms[tPDValue](tPDNumerical{}, tPDPositiveInfinity{}, tPDNegativeInfinity{}, nil)),
			"WithPrecisionDecimalArms supplies a nil notANumber arm of " + iface},
		{"a numerical arm of swapped fields", elem, data,
			into[struct{ A tPrecisionDecimal }](WithPrecisionDecimalArms[tPDValue](swappedArm{}, tPDPositiveInfinity{}, tPDNegativeInfinity{}, tPDNotANumber{})),
			"WithPrecisionDecimalArms supplies codec.swappedArm as the numerical arm of " + iface + ", where codegen emits a struct of Coefficient big.Int, Scale big.Int and Negative bool"},
		{"a numerical arm of an int64 Scale", elem, data,
			into[struct{ A tPrecisionDecimal }](WithPrecisionDecimalArms[tPDValue](signedArm{}, tPDPositiveInfinity{}, tPDNegativeInfinity{}, tPDNotANumber{})),
			"WithPrecisionDecimalArms supplies codec.signedArm as the numerical arm of " + iface + ", where codegen emits a struct of Coefficient big.Int, Scale big.Int and Negative bool"},
		{"a negativeInfinity arm with a field", elem, data,
			into[struct{ A tPrecisionDecimal }](WithPrecisionDecimalArms[tPDValue](tPDNumerical{}, tPDPositiveInfinity{}, flaggedArm{}, tPDNotANumber{})),
			"WithPrecisionDecimalArms supplies codec.flaggedArm as the negativeInfinity arm of " + iface + ", where codegen emits a struct of no field"},
		{"one type as two arms", elem, data,
			into[struct{ A tPrecisionDecimal }](WithPrecisionDecimalArms[tPDValue](tPDNumerical{}, tPDPositiveInfinity{}, tPDPositiveInfinity{}, tPDNotANumber{})),
			"WithPrecisionDecimalArms supplies codec.tPDPositiveInfinity as both the positiveInfinity and the negativeInfinity arm of " + iface},
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

// swappedArm, signedArm and flaggedArm are arms of tPDValue that are not
// codegen's: Scale before Coefficient, an int64 Scale, and a field-less
// role's arm with a field.
type (
	swappedArm struct {
		Scale       big.Int
		Coefficient big.Int
		Negative    bool
	}
	signedArm struct {
		Coefficient big.Int
		Scale       int64
		Negative    bool
	}
	flaggedArm struct{ Negative bool }
)

func (swappedArm) isPrecisionDecimalValue() {}
func (signedArm) isPrecisionDecimalValue()  {}
func (flaggedArm) isPrecisionDecimalValue() {}

// TestPrecisionDecimalArmsApart pins the two sets' separation: arms for a
// choice's interface are no branch set of it, so a choice root under
// WithPrecisionDecimalArms alone is item 6's missing WithBranches; and the
// misuse WithPrecisionDecimalArms' doc states Decode cannot report, the
// field-less arms passed in each other's roles, decodes INF into the arm
// passed as negativeInfinity.
func TestPrecisionDecimalArmsApart(t *testing.T) {
	choice := schemaOf(t, `<xs:element name="r"><xs:complexType><xs:choice><xs:element name="x" type="xs:precisionDecimal"/><xs:element name="y" type="xs:string"/></xs:choice></xs:complexType></xs:element>`)
	data := instanceOf("", "<s:x>1</s:x>")
	mustAccept(t, choice, data)
	_, err := Decode[tC](choice, strict.New(), data, pdArms, WithPrecisionDecimalArms[tC](tCX{}, tCY{}, tCX{}, tCY{}))
	if want := ErrTypeMismatch.Error() + ": no WithBranches supplies the branches of codec.tC, the interface of T, where codegen emits 2"; err == nil || err.Error() != want {
		t.Errorf("error = %v\nwant    %q", err, want)
	}
	root := schemaOf(t, `<xs:element name="r" type="xs:precisionDecimal"/>`)
	data = instanceOf("", "INF")
	mustAccept(t, root, data)
	swapped := WithPrecisionDecimalArms[tPDValue](tPDNumerical{}, tPDNegativeInfinity{}, tPDPositiveInfinity{}, tPDNotANumber{})
	got, err := Decode[tPrecisionDecimal](root, strict.New(), data, swapped)
	if err != nil {
		t.Fatal(err)
	}
	if s := spellDecimal(reflect.ValueOf(*got)); s != "-INF" {
		t.Errorf("Decode(INF) under swapped arms = %s, want -INF", s)
	}
}

// TestWithLoggerPrecisionDecimal pins the conversion record of a
// precisionDecimal, read from its lexical: rule "dt-lexical-mapping", input
// the normalized value, "3.00".
func TestWithLoggerPrecisionDecimal(t *testing.T) {
	schema := schemaOf(t, sequenceOf(`<xs:element name="a" type="xs:precisionDecimal"/>`))
	data := instanceOf("", "<s:a> 3.00 </s:a>")
	var buf bytes.Buffer
	if _, err := Decode[struct{ A tPrecisionDecimal }](schema, strict.New(), data, pdArms, WithLogger(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))); err != nil {
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
	const pd = "{http://www.w3.org/2001/XMLSchema}precisionDecimal"
	loc := fmt.Sprintf("?:1:%d", bytes.Index(data, []byte("<s:a>"))+len("<s:a>")+1)
	want := []traceRecord{
		{"whiteSpace", map[string]string{"rule": "rf-whiteSpace", "type": pd, "input": " 3.00 ", "loc": loc}},
		{"lexical mapping", map[string]string{"rule": "cvc-datatype-valid", "clause": "2.1", "type": pd, "input": "3.00", "loc": loc}},
		{"conversion", map[string]string{"rule": "dt-lexical-mapping", "type": pd, "input": "3.00", "loc": loc}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("codec records = %+v\nwant %+v", got, want)
	}
}

// TestDecodePrecisionDecimalStageError pins item 8 over PrecisionDecimal,
// which reads no Canonical: a backend whose Parse accepts a lexical that is
// no pDecimalRep, here 1,5, fails the conversion, spelling the lexical and no
// Canonical.
func TestDecodePrecisionDecimalStageError(t *testing.T) {
	const tail = "; validate accepted the value on the same backend, so this is a fault of the backend, not of the instance"
	comma := overDatatype("precisionDecimal", func(base value.Mapping) value.Mapping {
		return value.Mapping{Parse: func(lexical string, ctx value.Context) (value.Value, error) {
			if lexical == "1,5" {
				return base.Parse("1.5", ctx)
			}
			return base.Parse(lexical, ctx)
		}}
	})
	schema := schemaOf(t, sequenceOf(`<xs:element name="a" type="xs:precisionDecimal"/>`))
	data := instanceOf("", "<s:a>1,5</s:a>")
	offset := bytes.Index(data, []byte("<s:a>")) + len("<s:a>")
	got, err := Decode[struct{ A tPrecisionDecimal }](schema, comma, data, pdArms)
	if got != nil {
		t.Errorf("Decode = %v, want no value", *got)
	}
	var se *stageError
	if !errors.As(err, &se) || se.stage != stageConversion {
		t.Fatalf("Decode error = %v, want a stage error of %s", err, stageConversion.name())
	}
	want := fmt.Sprintf(`codec: ?:1:%d: the conversion to the Go type failed on "1,5", a value of {http://www.w3.org/2001/XMLSchema}precisionDecimal in the character content of element {urn:s}a at byte offset %d: it is no D for the datatype struct D of xs:precisionDecimal (Value I, I a sealed interface whose arms WithPrecisionDecimalArms supplies): ",5" follows the value at byte 1`, offset+1, offset) + tail
	if err.Error() != want {
		t.Errorf("error = %q\nwant    %q", err, want)
	}
	var xe *xsderr.Error
	if errors.As(err, &xe) || errors.Is(err, ErrTypeMismatch) || errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("error %v wraps something", err)
	}
}

// TestConvertDecimalRejects pins convertDecimal's refusals of lexicals that
// are no pDecimalRep (precisionDecimal §3.2), which a backend's Parse could
// accept: no digit, an exponent without one, a sign twice, and trailing
// text, inf and nan in another case included.
func TestConvertDecimalRejects(t *testing.T) {
	for _, lexical := range []string{"", ".", "+", "-", "E5", "1E", "1E+", "+-1", "--1", "1e5.0", "1 2", "inf", "nan", "+NaN", "-NaN", "INFINITY", "0x10"} {
		if v, err := convertDecimal(lexical); err == nil {
			t.Errorf("convertDecimal(%q) = %v, want an error", lexical, v)
		}
	}
}
