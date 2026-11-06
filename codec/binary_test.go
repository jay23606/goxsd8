package codec

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
)

// The binary schemas: binarySchema's s:r holds one field per site a
// hexBinary or base64Binary value takes, x and b of each type, o an optional
// hexBinary, l a list of hexBinary, a ref to h, of hexBinary, a nested type's
// particle v of base64Binary, and the attribute at, of base64Binary;
// binaryChoiceSchema's choice holds x, of hexBinary, and y, of base64Binary;
// binaryRefSchema's sequence holds a ref to h alone, of the primitive it
// names.
const (
	binarySchema = `<xs:element name="h" type="xs:hexBinary"/>
<xs:element name="r"><xs:complexType><xs:sequence>
<xs:element name="x" type="xs:hexBinary"/><xs:element name="b" type="xs:base64Binary"/><xs:element name="o" type="xs:hexBinary" minOccurs="0"/>
<xs:element name="l"><xs:simpleType><xs:list itemType="xs:hexBinary"/></xs:simpleType></xs:element><xs:element ref="s:h"/>
<xs:element name="n"><xs:complexType><xs:sequence><xs:element name="v" type="xs:base64Binary"/></xs:sequence></xs:complexType></xs:element>
</xs:sequence><xs:attribute name="at" type="xs:base64Binary"/></xs:complexType></xs:element>`
	binaryChoiceSchema = `<xs:element name="r"><xs:complexType><xs:choice><xs:element name="x" type="xs:hexBinary"/><xs:element name="y" type="xs:base64Binary"/></xs:choice></xs:complexType></xs:element>`
)

func binaryRefSchema(primitive string) string {
	return `<xs:element name="h" type="xs:` + primitive + `"/><xs:element name="r"><xs:complexType><xs:sequence><xs:element ref="s:h"/></xs:sequence></xs:complexType></xs:element>`
}

// binaries to binaryRef are the types codegen emits for the binary schemas:
// binarySchema's R, H and N, binaryChoiceSchema's R, RX and RY, and
// binaryRefSchema's R, which TestBinaryTypesAreCodegens pins.
type (
	binaries struct {
		X  []byte
		B  []byte
		O  *[]byte
		L  [][]byte
		H  binaryHead
		N  binaryNested
		At *[]byte
	}
	binaryHead     []byte
	binaryNested   struct{ V []byte }
	binaryChoice   interface{ isBinaryChoice() }
	binaryChoiceX  struct{ X []byte }
	binaryChoiceY  struct{ Y []byte }
	binaryRef      struct{ H binaryHead }
	binaryRoot     []byte
	binaryListRoot [][]byte
)

func (binaryChoiceX) isBinaryChoice() {}
func (binaryChoiceY) isBinaryChoice() {}

var binaryChoiceBranches = WithBranches[binaryChoice](binaryChoiceX{}, binaryChoiceY{})

// TestBinaryTypesAreCodegens pins the types above to those codegen emits for
// each binary schema: []byte for either primitive, [][]byte for a list of
// one (codegen/doc.go "# Simple types").
func TestBinaryTypesAreCodegens(t *testing.T) {
	rows := []struct {
		decls string
		want  []string
	}{
		{binarySchema, []string{"type H []byte\n", "type N struct {\n\tV []byte\n}\n",
			"type R struct {\n\tX  []byte\n\tB  []byte\n\tO  *[]byte\n\tL  [][]byte\n\tH  H\n\tN  N\n\tAt *[]byte\n}\n"}},
		{binaryChoiceSchema, []string{
			"type R interface {\n\tisR()\n}\n",
			"type RX struct {\n\tX []byte\n}\n\nfunc (RX) isR() {}\n",
			"type RY struct {\n\tY []byte\n}\n\nfunc (RY) isR() {}\n",
		}},
		{binaryRefSchema("hexBinary"), []string{"type H []byte\n", "type R struct {\n\tH H\n}\n"}},
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

// TestDecodeBinaries pins the lift of xs:hexBinary and xs:base64Binary
// (Datatypes §3.3.15, §3.3.16): a value decodes into codegen's []byte, its
// octets the lexical's, a hexBinary in either letter case (§E.4.1
// f-hexBinaryMap), a base64Binary with and without padding and with the
// spaces its lexical space admits and the whitespace collapse removes
// (§3.3.16.2), and the empty value of each, no octets, a slice of length 0
// (codegen/doc.go "# Simple types": nil and the empty slice are one value).
// The two "4142" rows tell the primitives apart: that literal is in both
// lexical spaces, of different octets. CheckDecode returns nil for each row's
// T.
func TestDecodeBinaries(t *testing.T) {
	rows := []struct {
		typ, lexical string
		want         []byte
	}{
		{"hexBinary", "0aff", []byte{0x0a, 0xff}},
		{"hexBinary", "0AFF", []byte{0x0a, 0xff}},
		{"hexBinary", "0aFf", []byte{0x0a, 0xff}},
		{"hexBinary", "4142", []byte("AB")},
		{"hexBinary", " \n4142\t ", []byte("AB")},
		{"hexBinary", "", nil},
		{"base64Binary", "4142", []byte{0xe3, 0x5e, 0x36}},
		{"base64Binary", "QUI=", []byte("AB")},
		{"base64Binary", "QQ==", []byte("A")},
		{"base64Binary", "QUJD", []byte("ABC")},
		{"base64Binary", "Q U J D", []byte("ABC")},
		{"base64Binary", "QU I =", []byte("AB")},
		{"base64Binary", "Q Q = =", []byte("A")},
		{"base64Binary", "\n QUJD\tRA==  \n", []byte("ABCD")},
		{"base64Binary", "", nil},
	}
	for _, row := range rows {
		t.Run(row.typ+" "+row.lexical, func(t *testing.T) {
			schema := schemaOf(t, sequenceOf(`<xs:element name="a" type="xs:`+row.typ+`"/>`))
			data := instanceOf("", "<s:a>"+row.lexical+"</s:a>")
			mustAccept(t, schema, data)
			got, err := Decode[struct{ A []byte }](schema, strict.New(), data)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.A, row.want) || len(got.A) != len(row.want) {
				t.Errorf("A = %#v, want the octets %#v", got.A, row.want)
			}
			if err := CheckDecode[struct{ A []byte }](schema, rootName); err != nil {
				t.Errorf("CheckDecode = %v, want nil", err)
			}
		})
	}
}

// TestDecodeBinarySites pins []byte and [][]byte at each site that holds
// one: against codegen's R for binarySchema, an element of each primitive, an
// optional one, present with no octets as a non-nil pointer to a slice of
// length 0, a list of hexBinary, a ref into the head's named type, a nested
// type's particle and an attribute use, present with no octets likewise; a
// choice's alternative of each primitive; and a simple document element of
// hexBinary and of a list of it, named or not. CheckDecode returns nil for
// each T.
func TestDecodeBinarySites(t *testing.T) {
	schema := schemaOf(t, binarySchema)
	data := instanceOf(` at=""`, "<s:x>0a</s:x><s:b>QUI=</s:b><s:o/><s:l> 0A  ff \n</s:l><s:h>BEEF</s:h><s:n><s:v>QQ==</s:v></s:n>")
	mustAccept(t, schema, data)
	got, err := Decode[binaries](schema, strict.New(), data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.X, []byte{0x0a}) || !bytes.Equal(got.B, []byte("AB")) || !bytes.Equal(got.H, []byte{0xbe, 0xef}) || !bytes.Equal(got.N.V, []byte("A")) {
		t.Errorf("Decode = %#v, want X 0a, B AB, H beef, N.V A", *got)
	}
	if len(got.L) != 2 || !bytes.Equal(got.L[0], []byte{0x0a}) || !bytes.Equal(got.L[1], []byte{0xff}) {
		t.Errorf("Decode's L = %#v, want [0a ff]", got.L)
	}
	if got.O == nil || len(*got.O) != 0 || got.At == nil || len(*got.At) != 0 {
		t.Errorf("Decode's O = %#v, At = %#v, want each a non-nil pointer to a slice of length 0", got.O, got.At)
	}
	if err := CheckDecode[binaries](schema, rootName); err != nil {
		t.Errorf("CheckDecode = %v, want nil", err)
	}

	choice := schemaOf(t, binaryChoiceSchema)
	for _, row := range []struct {
		body string
		want binaryChoice
	}{
		{"<s:x>4142</s:x>", binaryChoiceX{X: []byte("AB")}},
		{"<s:y>4142</s:y>", binaryChoiceY{Y: []byte{0xe3, 0x5e, 0x36}}},
	} {
		data := instanceOf("", row.body)
		mustAccept(t, choice, data)
		v, err := Decode[binaryChoice](choice, strict.New(), data, binaryChoiceBranches)
		if err != nil {
			t.Errorf("Decode %s: %v", row.body, err)
			continue
		}
		if !choiceBytesEqual(*v, row.want) {
			t.Errorf("Decode %s = %#v, want %#v", row.body, *v, row.want)
		}
	}
	if err := CheckDecode[binaryChoice](choice, rootName, binaryChoiceBranches); err != nil {
		t.Errorf("CheckDecode of the choice = %v, want nil", err)
	}

	root := schemaOf(t, `<xs:element name="r" type="xs:hexBinary"/>`)
	if v, err := Decode[binaryRoot](root, strict.New(), instanceOf("", "cafe")); err != nil || !bytes.Equal(*v, []byte{0xca, 0xfe}) {
		t.Errorf("Decode[binaryRoot] = %v, %v, want ca fe", v, err)
	}
	if v, err := Decode[[]byte](root, strict.New(), instanceOf("", "")); err != nil || len(*v) != 0 {
		t.Errorf("Decode[[]byte] of the empty value = %v, %v, want a slice of length 0", v, err)
	}
	if err := CheckDecode[binaryRoot](root, rootName); err != nil {
		t.Errorf("CheckDecode[binaryRoot] = %v, want nil", err)
	}
	list := schemaOf(t, `<xs:element name="r"><xs:simpleType><xs:list itemType="xs:hexBinary"/></xs:simpleType></xs:element>`)
	if v, err := Decode[binaryListRoot](list, strict.New(), instanceOf("", "01 0203")); err != nil || len(*v) != 2 || !bytes.Equal((*v)[0], []byte{1}) || !bytes.Equal((*v)[1], []byte{2, 3}) {
		t.Errorf("Decode[binaryListRoot] = %v, %v, want [01 0203]", v, err)
	}
	if v, err := Decode[[][]byte](list, strict.New(), instanceOf("", "ff")); err != nil || len(*v) != 1 || !bytes.Equal((*v)[0], []byte{0xff}) {
		t.Errorf("Decode[[][]byte] = %v, %v, want [ff]", v, err)
	}
	if err := CheckDecode[[][]byte](list, rootName); err != nil {
		t.Errorf("CheckDecode[[][]byte] = %v, want nil", err)
	}
}

// TestConvertBinaries pins the conversion of a canonical form to []byte
// under each binary arm, whatever backend rendered it: hexBinary's in either
// letter case, its case never compared with f-hexBinaryCanonical's upper
// case (Datatypes §E.4.1), and base64Binary's padded form; a form neither
// reads is an error, item 8's.
func TestConvertBinaries(t *testing.T) {
	for _, row := range []struct {
		atom atomic
		form string
		want []byte
	}{
		{atomicHexBinary, "0aff", []byte{0x0a, 0xff}},
		{atomicHexBinary, "0AFF", []byte{0x0a, 0xff}},
		{atomicBase64Binary, "QUI=", []byte("AB")},
	} {
		v, err := row.atom.convert(row.form)
		if err != nil || !bytes.Equal(v.Bytes(), row.want) {
			t.Errorf("%s.convert(%q) = %v, %v, want %#v", row.atom.spell(), row.form, v, err, row.want)
		}
	}
	for _, row := range []struct {
		atom atomic
		form string
	}{
		{atomicHexBinary, "0"},
		{atomicBase64Binary, "QUI"},
	} {
		if _, err := row.atom.convert(row.form); err == nil {
			t.Errorf("convert(%q) = nil error, want one", row.form)
		}
	}
}

// choiceBytesEqual reports whether a and b are the same branch holding the
// same octets.
func choiceBytesEqual(a, b binaryChoice) bool {
	switch x := a.(type) {
	case binaryChoiceX:
		y, ok := b.(binaryChoiceX)
		return ok && bytes.Equal(x.X, y.X)
	case binaryChoiceY:
		y, ok := b.(binaryChoiceY)
		return ok && bytes.Equal(x.Y, y.Y)
	}
	return false
}

// TestDecodeBinaryRefTypeMismatch pins item 6 of Decode's doc for an
// <element ref> to a hexBinary or base64Binary declaration: its field is N, a
// defined type over []byte, which binaryRef's passes; the bare []byte, a local
// particle's V, and string are each ErrTypeMismatch, whose message spells
// codegen's type as []byte, never reflect's []uint8 (atomic.spell); T's own
// field type is reflect's spelling.
func TestDecodeBinaryRefTypeMismatch(t *testing.T) {
	for _, p := range []struct {
		primitive string
		want      []byte
	}{
		{"hexBinary", []byte("AB")},
		{"base64Binary", []byte{0xe3, 0x5e, 0x36}},
	} {
		t.Run(p.primitive, func(t *testing.T) { assertBinaryRef(t, binaryRefSchema(p.primitive), p.want) })
	}
}

// assertBinaryRef fails the test unless the ref schema decls decodes its
// instance into binaryRef, H holding want, and mismatches a bare []byte H
// and a string one as TestDecodeBinaryRefTypeMismatch states.
func assertBinaryRef(t *testing.T, decls string, want []byte) {
	t.Helper()
	schema := schemaOf(t, decls)
	data := instanceOf("", "<s:h>4142</s:h>")
	mustAccept(t, schema, data)
	got, err := Decode[binaryRef](schema, strict.New(), data)
	if err != nil || !bytes.Equal(got.H, want) {
		t.Errorf("Decode[binaryRef] = %v, %v, want H %#v", got, err, want)
	}
	for _, row := range []struct {
		name   string
		decode func() error
		check  error
	}{
		{"bare []byte", func() error { _, err := Decode[struct{ H []byte }](schema, strict.New(), data); return err },
			CheckDecode[struct{ H []byte }](schema, rootName)},
		{"string", func() error { _, err := Decode[struct{ H string }](schema, strict.New(), data); return err },
			CheckDecode[struct{ H string }](schema, rootName)},
	} {
		err := row.decode()
		if !errors.Is(err, ErrTypeMismatch) {
			t.Errorf("%s: Decode = %v, want ErrTypeMismatch", row.name, err)
			continue
		}
		if want := "where codegen emits H N for a named type N over []byte"; !strings.HasSuffix(err.Error(), want) {
			t.Errorf("%s: Decode = %q, want it to end %q", row.name, err, want)
		}
		assertProbeMismatch(t, row.check, err)
	}
}
