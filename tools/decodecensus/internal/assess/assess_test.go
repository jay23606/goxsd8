package assess

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/codec"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// fixtureSchema declares a, in the slice codec decodes, c, whose all
// content codec declines, p, a choice codec decodes into a sealed interface,
// and u, a's shape with an assertion whose for expression the XPath subset
// declines, so validate leaves it undecided (cvc-assertion).
const fixtureSchema = `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="a"><xs:complexType><xs:sequence><xs:element name="b" type="xs:string"/></xs:sequence></xs:complexType></xs:element>
  <xs:element name="c"><xs:complexType><xs:all><xs:element name="d" type="xs:string"/></xs:all></xs:complexType></xs:element>
  <xs:element name="p"><xs:complexType><xs:choice><xs:element name="q" type="xs:string"/><xs:element name="r" type="xs:string"/></xs:choice></xs:complexType></xs:element>
  <xs:element name="u"><xs:complexType><xs:sequence><xs:element name="b" type="xs:string"/></xs:sequence>
    <xs:assert test="(for $x in b return string-length($x)) = 1"/></xs:complexType></xs:element>
</xs:schema>
`

// p, pq and pr are the sealed interface and branches codegen emits for
// element p.
type (
	p  interface{ isP() }
	pq struct{ Q string }
	pr struct{ R string }
)

func (pq) isP() {}
func (pr) isP() {}

// The fixture's instances: valid.xml, all.xml and choice.xml are valid, invalid.xml
// lacks a's required child b, so validate charges it, and undecided.xml is
// Undecided.
var fixtureFiles = map[string]string{
	"s.xsd":         fixtureSchema,
	"valid.xml":     `<a><b>x</b></a>`,
	"invalid.xml":   `<a/>`,
	"all.xml":       `<c><d>x</d></c>`,
	"choice.xml":    `<p><r>x</r></p>`,
	"undecided.xml": `<u><b>x</b></u>`,
}

// a is the struct codegen emits for element a: Decode compares its fields
// alone, so a struct of this shape decodes as codegen's own.
type a struct{ B string }

// mismatched is a struct whose fields are not codegen's for element a.
type mismatched struct{ X int }

// writeSuite writes fixtureFiles under a fresh directory and returns it.
func writeSuite(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range fixtureFiles {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	return root
}

// drive runs Drive over one group of the fixture schema holding cases, with
// decoders, and returns its Summary.
func drive(t *testing.T, decoders []Decoder, cases ...Case) Summary {
	t.Helper()
	req, err := json.Marshal(Request{Root: writeSuite(t), Groups: []Group{{Schema: []string{"s.xsd"}, Cases: cases}}})
	if err != nil {
		t.Fatalf("encoding the request: %v", err)
	}
	var out bytes.Buffer
	if err := Drive(bytes.NewReader(req), &out, decoders); err != nil {
		t.Fatalf("Drive: %v", err)
	}
	var sum Summary
	if err := json.Unmarshal(out.Bytes(), &sum); err != nil {
		t.Fatalf("reading the summary %q: %v", out.String(), err)
	}
	return sum
}

// accepting is a Decoder returning no error whatever the instance, as a
// codec that decodes an instance validate does not accept would.
func accepting(*xsd.Schema, value.Backend, []byte, ...codec.Option) error { return nil }

// TestDriveCountsEachBucket holds Drive, the loop the census's driver runs,
// to one count per bucket of the classifier, each row a case that lands in
// one bucket alone. The rows decoded, declined, undecided, rejected and
// placeholder mismatch each fail with their bucket's arm of classify removed,
// and failed other, classify's last return, with its arm of tallyOutcome
// removed; the row differential fails with assess's differential always false,
// and the row after it with the differential set on every decoded case.
func TestDriveCountsEachBucket(t *testing.T) {
	own := []Decoder{DecoderFor[a]()}
	cases := []struct {
		name     string
		decoders []Decoder
		c        Case
		want     Summary
	}{
		{"decoded", own, Case{"valid.xml", 0}, Summary{Decoded: 1}},
		{"declined", []Decoder{DecoderFor[a]()}, Case{"all.xml", 0},
			Summary{Declines: []Tally{{Construct: "all content", Count: 1}}}},
		{"undecided", own, Case{"undecided.xml", 0}, Summary{Undecided: 1}},
		{"rejected", []Decoder{DecoderFor[mismatched]()}, Case{"valid.xml", 0}, Summary{Rejected: 1}},
		{"failed other", own, Case{"invalid.xml", 0}, Summary{FailedOther: 1}},
		{"placeholder mismatch", nil, Case{"valid.xml", -1}, Summary{FailedOther: 1, PlaceholderMismatch: 1}},
		{"differential", []Decoder{accepting}, Case{"invalid.xml", 0}, Summary{Decoded: 1, Differential: 1}},
		{"no differential where validate accepts", []Decoder{accepting}, Case{"valid.xml", 0}, Summary{Decoded: 1}},
		{"decoded under bound branches", []Decoder{DecoderFor[p](codec.WithBranches[p](pq{}, pr{}))}, Case{"choice.xml", 0}, Summary{Decoded: 1}},
		{"rejected without them", []Decoder{DecoderFor[p]()}, Case{"choice.xml", 0}, Summary{Rejected: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := drive(t, tc.decoders, tc.c); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Summary = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestDriveTalliesDeclinesByConstruct pins the decline table's key: two
// cases declining one construct are one Tally of 2.
func TestDriveTalliesDeclinesByConstruct(t *testing.T) {
	got := drive(t, []Decoder{DecoderFor[a]()}, Case{"all.xml", 0}, Case{"valid.xml", 0}, Case{"all.xml", 0})
	want := Summary{Decoded: 1, Declines: []Tally{{Construct: "all content", Count: 2}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Summary = %+v, want %+v", got, want)
	}
}

// TestDriveRefusesAnUnknownDecoder holds a decoder index outside the
// decoders, and below the placeholder's -1, to an error.
func TestDriveRefusesAnUnknownDecoder(t *testing.T) {
	for _, i := range []int{1, -2} {
		req, err := json.Marshal(Request{Root: writeSuite(t), Groups: []Group{{Schema: []string{"s.xsd"}, Cases: []Case{{"valid.xml", i}}}}})
		if err != nil {
			t.Fatalf("encoding the request: %v", err)
		}
		var out bytes.Buffer
		if err := Drive(bytes.NewReader(req), &out, []Decoder{DecoderFor[a]()}); err == nil {
			t.Errorf("Drive with decoder index %d: no error, summary %s", i, out.String())
		}
	}
}

// TestAssembleEntersEachUnreadFurtherDocument pins Assemble's further
// documents: r.xsd, which s.xsd redefines, is in the first document's closure
// and is not entered again, and o.xsd, which nothing names, is.
func TestAssembleEntersEachUnreadFurtherDocument(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"s.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:redefine schemaLocation="r.xsd"><xs:simpleType name="t"><xs:restriction base="t"><xs:maxLength value="3"/></xs:restriction></xs:simpleType></xs:redefine>
</xs:schema>`,
		"r.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:simpleType name="t"><xs:restriction base="xs:string"/></xs:simpleType>
  <xs:element name="r" type="t"/>
</xs:schema>`,
		"o.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:element name="o" type="xs:string"/></xs:schema>`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	schema, err := Assemble(root, []string{"s.xsd", "r.xsd", "o.xsd"}, strict.New())
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	for _, local := range []string{"r", "o"} {
		if _, ok := schema.Element(xsd.QName{Local: local}); !ok {
			t.Errorf("the schema declares no element %s", local)
		}
	}
}
