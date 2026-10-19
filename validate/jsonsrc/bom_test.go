package jsonsrc

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

// readmeSchema is README's order.xsd: order holds one or more item strings
// and carries an optional kind attribute.
const readmeSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
           targetNamespace="urn:o" elementFormDefault="qualified">
  <xs:element name="order">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="item" type="xs:string" maxOccurs="unbounded"/>
      </xs:sequence>
      <xs:attribute name="kind" type="xs:string"/>
    </xs:complexType>
  </xs:element>
</xs:schema>`

// README's order.json and twice.json.
const (
	readmeOrder = `{"o:order":{"$xmlns":{"o":"urn:o"},"kind":"x","item":["a","b"]}}`
	readmeTwice = `{"o:order":{"$xmlns":{"o":"urn:o"},"kind":"x","kind":"y","item":["a"]}}`
)

const bom = "\xEF\xBB\xBF"

// outcome is what Validate answers for one document, flattened so two runs
// compare: the violations' rules and locations, or the error's text and
// whether it is a decline.
func outcome(t *testing.T, r io.Reader, schema string) string {
	t.Helper()
	res, err := Validate(validatorOver(t, schema), r, WithURI(jsonURI))
	var d *DeclineError
	if errors.As(err, &d) {
		return "error " + err.Error() + " (decline)"
	}
	if err != nil {
		return "error " + err.Error()
	}
	var b strings.Builder
	b.WriteString("result")
	for _, v := range res.Violations() {
		b.WriteString(" " + string(v.Rule) + "@" + v.Loc.String())
	}
	return b.String()
}

// TestByteOrderMarkIgnored pins one leading UTF-8 byte order mark ignored
// (RFC 8259 §8.1, Validate): a document opening with it answers exactly what
// the document without it answers, at every location, whatever sizes the
// reader returns, the mark split across reads included.
func TestByteOrderMarkIgnored(t *testing.T) {
	for _, tc := range []struct {
		name, schema, doc string
		want              string // the BOM-less document's outcome
	}{
		{"README order.json", readmeSchema, readmeOrder, "result"},
		{"README twice.json", readmeSchema, readmeTwice,
			`error jsonsrc: instance.json:1:47: attribute member "kind" repeats in the object of {urn:o}order; an element carries one attribute of each expanded name, so give it once (decline)`},
		{"violations on later lines", orderSchema,
			"{\n  \"order\": {\n    \"size\": \"x\",\n    \"item\": [ {\"sku\": \"ABC\"},\n      {\"sku\": 7} ]\n  }\n}\n",
			"result cvc-type@instance.json:3:5 cvc-type@instance.json:5:8"},
		{"violation on line 1", orderSchema, `{"order":{"size":"x","item":{"sku":"ABC"}}}`,
			"result cvc-type@instance.json:1:11"},
		{"U+FEFF inside a string is content", readmeSchema,
			`{"o:order":{"$xmlns":{"o":"urn:o"},"item":["` + bom + `a"]}}`, "result"},
		{"only the mark", readmeSchema, "",
			`error jsonsrc: instance.json:1:1: the document holds no JSON value; a JSON instance is one object whose one member names the validation root, {"<root>":...}`},
		{"the mark and a line feed", readmeSchema, "\n",
			`error jsonsrc: instance.json:1:1: the document holds no JSON value; a JSON instance is one object whose one member names the validation root, {"<root>":...}`},
		{"truncated, then a line feed", readmeSchema, `{"r":"1"` + "\n",
			"error jsonsrc: instance.json:1:9: malformed JSON: unexpected EOF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := outcome(t, strings.NewReader(tc.doc), tc.schema); got != tc.want {
				t.Fatalf("without the mark: %s\nwant %s: the fixture does not say what the test claims", got, tc.want)
			}
			for _, rd := range []struct {
				name string
				wrap func(io.Reader) io.Reader
			}{
				{"whole", func(r io.Reader) io.Reader { return r }},
				{"one byte", iotest.OneByteReader},
				{"half", iotest.HalfReader},
				{"data with EOF", iotest.DataErrReader},
			} {
				if got := outcome(t, rd.wrap(strings.NewReader(bom+tc.doc)), tc.schema); got != tc.want {
					t.Errorf("with the mark, %s reads: %s\nwant %s, as without it", rd.name, got, tc.want)
				}
			}
		})
	}
}

// TestByteOrderMarkElsewhere pins that only one mark, as the document's first
// three bytes, is ignored: a second mark, a mark after white space, between
// tokens or after the top-level value, and a UTF-16 mark are malformed JSON,
// each at its own location, and so are the mark's first two bytes, whatever
// follows them.
func TestByteOrderMarkElsewhere(t *testing.T) {
	for _, tc := range []struct {
		name, doc, prefix string
	}{
		{"two marks", bom + bom + readmeOrder, "jsonsrc: instance.json:1:1: malformed JSON: "},
		{"after white space", " " + bom + readmeOrder, "jsonsrc: instance.json:1:2: malformed JSON: "},
		{"between tokens", `{"o:order":` + bom + readmeOrder[len(`{"o:order":`):], "jsonsrc: instance.json:1:12: malformed JSON: "},
		{"after the top-level value", readmeOrder + bom, "jsonsrc: instance.json:1:65: malformed JSON: "},
		{"part of a mark", "\xEF\xBB" + readmeOrder, "jsonsrc: instance.json:1:1: malformed JSON: "},
		{"part of a mark, then the end", "\xEF\xBB", "jsonsrc: instance.json:1:1: malformed JSON: "},
		{"UTF-16LE mark", "\xFF\xFE" + readmeOrder, "jsonsrc: instance.json:1:1: malformed JSON: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := outcome(t, strings.NewReader(tc.doc), readmeSchema)
			if !strings.HasPrefix(got, "error "+tc.prefix) || strings.HasSuffix(got, "(decline)") {
				t.Errorf("Validate = %s\nwant a plain error opening %q", got, tc.prefix)
			}
		})
	}
}

// onceErr returns err on its first Read and io.EOF after it, so a fault is
// reported only if the first reader to see it keeps it.
type onceErr struct{ err error }

func (o *onceErr) Read([]byte) (int, error) {
	err := o.err
	o.err = nil
	if err == nil {
		return 0, io.EOF
	}
	return 0, err
}

// TestByteOrderMarkReadFault pins a fault reading r while the mark is being
// read, or right after it, reported as a fault reading the input, at 1:1,
// with errors.Is reaching r's error: never malformed JSON or an empty
// document, the beginning of a mark the fault cut off included (bomless.rule).
func TestByteOrderMarkReadFault(t *testing.T) {
	boom := errors.New("disk on fire")
	for _, lead := range []string{"", "\xEF", "\xEF\xBB", bom} {
		_, err := Validate(validatorOver(t, readmeSchema), io.MultiReader(strings.NewReader(lead), &onceErr{boom}), WithURI(jsonURI))
		const want = "jsonsrc: instance.json:1:1: fault reading the input: disk on fire"
		if err == nil || err.Error() != want || !errors.Is(err, boom) {
			t.Errorf("Validate over %q then a fault = %v, want %q wrapping the reader's error", lead, err, want)
		}
	}
}
