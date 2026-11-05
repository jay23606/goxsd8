package codec

import (
	"bytes"
	"errors"
	"math"
	"math/big"
	"reflect"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/xsderr"
)

// atomicSchema declares s:r of one local element per atomic Go type the
// slice decodes, the eight bounded integers among them and a user restriction
// of xs:int, an optional and a repeated one, refs to the heads h, of xs:int,
// and d, of xs:decimal, and an attribute use per shape: optional, required,
// and with a default; s:lr, of that restriction, is a simple root; s:c's
// choice holds local a and A, one Go field name over two atomic Go types.
const atomicSchema = `<xs:simpleType name="lo"><xs:restriction base="xs:int"><xs:minInclusive value="0"/></xs:restriction></xs:simpleType>
<xs:element name="h" type="xs:int"/><xs:element name="d" type="xs:decimal"/><xs:element name="lr" type="s:lo"/>
<xs:element name="r"><xs:complexType><xs:sequence>
<xs:element name="b" type="xs:boolean"/><xs:element name="f" type="xs:float"/><xs:element name="g" type="xs:double"/>
<xs:element name="dec" type="xs:decimal"/><xs:element name="big" type="xs:integer"/><xs:element name="nn" type="xs:nonNegativeInteger"/>
<xs:element name="i8" type="xs:byte"/><xs:element name="i16" type="xs:short"/><xs:element name="i32" type="xs:int"/><xs:element name="i64" type="xs:long"/>
<xs:element name="u8" type="xs:unsignedByte"/><xs:element name="u16" type="xs:unsignedShort"/><xs:element name="u32" type="xs:unsignedInt"/><xs:element name="u64" type="xs:unsignedLong"/>
<xs:element name="lo" type="s:lo"/><xs:element name="opt" type="xs:int" minOccurs="0"/><xs:element name="rep" type="xs:double" maxOccurs="unbounded"/>
<xs:element ref="s:h"/><xs:element ref="s:d" minOccurs="0"/>
</xs:sequence>
<xs:attribute name="ab" type="xs:boolean"/><xs:attribute name="ar" type="xs:decimal" use="required"/><xs:attribute name="af" type="xs:float" default="1.5"/>
</xs:complexType></xs:element>
<xs:element name="c"><xs:complexType><xs:choice><xs:element name="a" type="xs:int"/><xs:element name="A" type="xs:boolean"/></xs:choice></xs:complexType></xs:element>`

// atomics to atomicCA2 are the types codegen emits for atomicSchema: R, H, D,
// Lr, C, CA and CA2, which TestAtomicTypesAreCodegens pins.
type (
	atomics struct {
		B   bool
		F   float32
		G   float64
		Dec big.Rat
		Big big.Int
		Nn  big.Int
		I8  int8
		I16 int16
		I32 int32
		I64 int64
		U8  uint8
		U16 uint16
		U32 uint32
		U64 uint64
		Lo  int32
		Opt *int32
		Rep []float64
		H   atomicH
		D   *atomicD
		Ab  *bool
		Ar  big.Rat
		Af  float32
	}
	atomicH   int32
	atomicD   big.Rat
	atomicLr  int32
	atomicC   interface{ isC() }
	atomicCA  struct{ A int32 }
	atomicCA2 struct{ A bool }
)

func (atomicCA) isC()  {}
func (atomicCA2) isC() {}

var atomicCBranches = WithBranches[atomicC](atomicCA{}, atomicCA2{})

// TestAtomicTypesAreCodegens pins the types above to those codegen emits for
// atomicSchema, in shape, so that the rows decoding into them decode into
// codegen's types.
func TestAtomicTypesAreCodegens(t *testing.T) {
	gen := generated(t, schemaOf(t, atomicSchema))
	for _, decl := range []string{
		"type H int32\n", "type D big.Rat\n", "type Lr int32\n",
		"type R struct {\n\tB   bool\n\tF   float32\n\tG   float64\n\tDec big.Rat\n\tBig big.Int\n\tNn  big.Int\n" +
			"\tI8  int8\n\tI16 int16\n\tI32 int32\n\tI64 int64\n\tU8  uint8\n\tU16 uint16\n\tU32 uint32\n\tU64 uint64\n" +
			"\tLo  int32\n\tOpt *int32\n\tRep []float64\n\tH   H\n\tD   *D\n\tAb  *bool\n\tAr  big.Rat\n\tAf  float32\n}\n",
		"type C interface {\n\tisC()\n}\n",
		"type CA struct {\n\tA int32\n}\n\nfunc (CA) isC() {}\n",
		"type CA2 struct {\n\tA bool\n}\n\nfunc (CA2) isC() {}\n",
	} {
		if !bytes.Contains(gen, []byte(decl)) {
			t.Errorf("codegen emits\n%s\nwhich does not declare\n%s", gen, decl)
		}
	}
}

// atomicBody is an s:r instance body of atomicSchema whose every value's
// lexical form differs from the string of its value.
const atomicBody = "<s:b>1</s:b><s:f> 1e0 </s:f><s:g>1E-1</s:g><s:dec>010.50</s:dec><s:big>+0012345678901234567890123</s:big><s:nn>007</s:nn>" +
	"<s:i8>-0128</s:i8><s:i16>+32767</s:i16><s:i32> 01 </s:i32><s:i64>-09223372036854775808</s:i64>" +
	"<s:u8>0255</s:u8><s:u16>+65535</s:u16><s:u32>04294967295</s:u32><s:u64>018446744073709551615</s:u64>" +
	"<s:lo>07</s:lo><s:opt>02</s:opt><s:rep>1e0</s:rep><s:rep> 2.5E0 </s:rep><s:h>-03</s:h><s:d>1.50</s:d>"

// TestDecodeAtomics pins the lift of xs:boolean, xs:float, xs:double and
// xs:decimal (Datatypes §3.3.2–§3.3.5, §3.4.13–§3.4.25): each value is the
// one its lexical mapping gives its ·normalized value·, in the atomic Go type
// codegen emits, through each site that holds one: a sequence field of each
// type, a user restriction, the optional and repeated shapes, an <element
// ref> field of a named type over int32 and an optional one over big.Rat,
// and an attribute optional, required, and absent with a default, its
// supplied value mapped, or present.
func TestDecodeAtomics(t *testing.T) {
	schema := schemaOf(t, atomicSchema)
	rows := []struct {
		name, attrs string
		af          float32
	}{
		{"the defaulted attribute absent", ` ab="0" ar="+.5"`, 1.5},
		{"the defaulted attribute present", ` ab="0" ar="+.5" af="2e0"`, 2},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			data := instanceOf(row.attrs, atomicBody)
			mustAccept(t, schema, data)
			got, err := Decode[atomics](schema, strict.New(), data)
			if err != nil {
				t.Fatal(err)
			}
			big0, _ := new(big.Int).SetString("12345678901234567890123", 10)
			two, f := int32(2), false
			checks := []struct {
				field string
				ok    bool
			}{
				{"B", got.B},
				{"F", got.F == 1},
				{"G", got.G == 0.1},
				{"Dec", got.Dec.Cmp(big.NewRat(21, 2)) == 0},
				{"Big", got.Big.Cmp(big0) == 0},
				{"Nn", got.Nn.Cmp(big.NewInt(7)) == 0},
				{"I8", got.I8 == math.MinInt8},
				{"I16", got.I16 == math.MaxInt16},
				{"I32", got.I32 == 1},
				{"I64", got.I64 == math.MinInt64},
				{"U8", got.U8 == math.MaxUint8},
				{"U16", got.U16 == math.MaxUint16},
				{"U32", got.U32 == math.MaxUint32},
				{"U64", got.U64 == math.MaxUint64},
				{"Lo", got.Lo == 7},
				{"Opt", reflect.DeepEqual(got.Opt, &two)},
				{"Rep", reflect.DeepEqual(got.Rep, []float64{1, 2.5})},
				{"H", got.H == -3},
				{"D", got.D != nil && (*big.Rat)(got.D).Cmp(big.NewRat(3, 2)) == 0},
				{"Ab", reflect.DeepEqual(got.Ab, &f)},
				{"Ar", got.Ar.Cmp(big.NewRat(1, 2)) == 0},
				{"Af", got.Af == row.af},
			}
			for _, c := range checks {
				if !c.ok {
					t.Errorf("field %s = %v", c.field, reflect.ValueOf(*got).FieldByName(c.field))
				}
			}
		})
	}
}

// TestDecodeAtomicRootsAndChoice pins a simple root of each kind a named T
// takes, a named type over int32 for a user restriction of xs:int, bool
// itself for xs:boolean and a named type over big.Rat for xs:decimal, and a
// choice whose two alternatives share a Go field name and shape over two
// atomic Go types: each decodes, the choice not declining as one branch shape.
func TestDecodeAtomicRootsAndChoice(t *testing.T) {
	schema := schemaOf(t, atomicSchema)
	lr, err := Decode[atomicLr](schema, strict.New(), []byte(`<s:lr xmlns:s="urn:s"> 01 </s:lr>`))
	if err != nil || *lr != 1 {
		t.Errorf("Decode[atomicLr] = %v, %v; want 1", lr, err)
	}
	boolean := schemaOf(t, `<xs:element name="r" type="xs:boolean"/>`)
	b, err := Decode[bool](boolean, strict.New(), instanceOf("", "1"))
	if err != nil || !*b {
		t.Errorf("Decode[bool] = %v, %v; want true", b, err)
	}
	d, err := Decode[atomicD](schema, strict.New(), []byte(`<s:d xmlns:s="urn:s">-.250</s:d>`))
	if err != nil || (*big.Rat)(d).Cmp(big.NewRat(-1, 4)) != 0 {
		t.Errorf("Decode[atomicD] = %v, %v; want -1/4", d, err)
	}
	for _, row := range []struct {
		body string
		want atomicC
	}{
		{"<s:a> 01 </s:a>", atomicCA{A: 1}},
		{"<s:A>1</s:A>", atomicCA2{A: true}},
	} {
		data := []byte(`<s:c xmlns:s="urn:s">` + row.body + `</s:c>`)
		mustAccept(t, schema, data)
		got, err := Decode[atomicC](schema, strict.New(), data, atomicCBranches)
		if err != nil || !reflect.DeepEqual(*got, row.want) {
			t.Errorf("Decode[atomicC](%s) = %v, %v; want %#v", row.body, got, err, row.want)
		}
	}
}

// TestDecodeFixedNegativeZero pins that a value equal to, but not identical
// with, its fixed {value constraint} (§3.5.4 cvc-au, §3.3.4.3 cvc-elt clause
// 5.2.2.2.2: equal or identical) decodes as the instance's own value: -0
// against fixed 0 decodes to negative zero, on an attribute, whose field is
// the value type itself (#3344), and on an element.
func TestDecodeFixedNegativeZero(t *testing.T) {
	schema := schemaOf(t, root(`<xs:sequence><xs:element name="e" type="xs:double" fixed="0"/></xs:sequence><xs:attribute name="z" type="xs:float" fixed="0"/>`))
	data := instanceOf(` z="-0"`, "<s:e>-0</s:e>")
	mustAccept(t, schema, data)
	got, err := Decode[struct {
		E float64
		Z float32
	}](schema, strict.New(), data)
	if err != nil {
		t.Fatal(err)
	}
	if got.E != 0 || !math.Signbit(got.E) {
		t.Errorf("E = %v, want negative zero", got.E)
	}
	if got.Z != 0 || !math.Signbit(float64(got.Z)) {
		t.Errorf("Z = %v, want negative zero", got.Z)
	}
}

// TestDecodeAtomicTypeMismatch pins item 6 of Decode's doc over the atomic Go
// types: a field, a simple root's T and a ref's named type of the wrong Go
// integer width, and a ref's named type over big.Int where codegen emits one
// over big.Rat, are ErrTypeMismatch, no decline.
func TestDecodeAtomicTypeMismatch(t *testing.T) {
	type wideH int64
	type intD big.Int
	type width struct{ A int64 }
	type refWidth struct{ H wideH }
	type refUnderlying struct{ D intD }
	rows := []struct {
		name, decls string
		data        []byte
		into        target
		want        string
	}{
		{"a field's width", sequenceOf(`<xs:element name="a" type="xs:int"/>`), instanceOf("", "<s:a>1</s:a>"),
			into[width](), "field 0 of codec.width is A int64 where codegen emits A int32"},
		{"a simple root's width", `<xs:element name="r" type="xs:int"/>`, instanceOf("", "1"),
			into[int64](), "the underlying type of int64 is not int32"},
		{"a ref's named type's width", `<xs:element name="h" type="xs:int"/>` + sequenceOf(`<xs:element ref="s:h"/>`), instanceOf("", "<s:h>1</s:h>"),
			into[refWidth](), "field 0 of codec.refWidth is H codec.wideH where codegen emits H N for a named type N over int32"},
		{"a ref's named type's underlying type", `<xs:element name="d" type="xs:decimal"/>` + sequenceOf(`<xs:element ref="s:d"/>`), instanceOf("", "<s:d>1</s:d>"),
			into[refUnderlying](), "field 0 of codec.refUnderlying is D codec.intD where codegen emits D N for a named type N over big.Rat"},
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
				t.Errorf("error = %v, want %q", err, want)
			}
			var de *DeclineError
			if !errors.Is(err, ErrTypeMismatch) || errors.As(err, &de) {
				t.Errorf("error %v: want ErrTypeMismatch alone", err)
			}
			if _, isRule := xsderr.RuleOf(err); isRule {
				t.Errorf("error %v carries a rule", err)
			}
			assertProbeMismatch(t, row.into.check(schema), err)
		})
	}
}
