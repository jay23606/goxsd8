package parser_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsderr"
)

// TestProduceNonIDValueRejected pins that an XSD element's id attribute whose
// ·actual value· is outside xs:ID's ·lexical space· (NCName's, Datatypes
// §3.4.8) is charged cvc-datatype-valid at that id attribute, whatever position
// the element is written in. One row per kind of position the walk reaches: a
// top-level declaration, a local particle, an identity-constraint element, and
// the <schema> root itself.
func TestProduceNonIDValueRejected(t *testing.T) {
	// A slice, not a map: subtest order is output (STYLE D2). Every body puts the
	// offending id attribute on the line named by wantLine.
	cases := []struct {
		name     string
		doc      string
		wantMsg  string
		wantLine int
	}{
		{
			name:     `top-level <element> id="123"`,
			doc:      wrap("", "\n"+`<xs:element name="e" id="123" type="xs:string"/>`),
			wantMsg:  `<element> id "123" is not in`,
			wantLine: 2,
		},
		{
			name: `local <any> id="25"`,
			doc: wrap("", "\n"+`<xs:complexType name="CT"><xs:sequence>`+"\n"+
				`<xs:any id="25"/>`+"\n"+
				`</xs:sequence></xs:complexType>`),
			wantMsg:  `<any> id "25" is not in`,
			wantLine: 3,
		},
		{
			name: `<key> id=""`,
			doc: wrap("", "\n"+`<xs:element name="root">`+"\n"+
				`<xs:key name="k" id=""><xs:selector xpath="."/><xs:field xpath="@a"/></xs:key>`+"\n"+
				`</xs:element>`),
			wantMsg:  `<key> id "" is not in`,
			wantLine: 3,
		},
		{
			name: `<schema> id="a:b"`,
			doc: `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"` + "\n" +
				`id="a:b"/>`,
			wantMsg:  `<schema> id "a:b" is not in`,
			wantLine: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, tc.doc)
			assertRule(t, err, "cvc-datatype-valid")
			assertIDFault(t, err, tc.wantMsg, tc.wantLine)
		})
	}
}

// TestProduceDuplicateIDRejected pins cvc-id clause 2 (§3.3.4.5) over one
// schema document: the SECOND element carrying a value is charged, at its own
// id attribute, the message names the first carrier and cites the clause inline
// (STYLE E4). The padded row compares ·actual values·: xs:ID's whiteSpace =
// collapse makes " a " and "a" one ID.
func TestProduceDuplicateIDRejected(t *testing.T) {
	cases := []struct {
		name    string
		first   string
		second  string
		wantMsg string
	}{
		{name: "identical", first: "a", second: "a", wantMsg: `<attribute> id "a" duplicates the id of the <element> at ` + produceURI + ":2:"},
		{name: "padded", first: " a ", second: "a", wantMsg: `<attribute> id "a" duplicates the id of the <element> at ` + produceURI + ":2:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := produce(t, wrap("", "\n"+`<xs:element name="e" id="`+tc.first+`" type="xs:string"/>`+"\n"+
				`<xs:attribute name="a" id="`+tc.second+`" type="xs:string"/>`))
			assertRule(t, err, "cvc-id")
			assertIDFault(t, err, tc.wantMsg, 3)
			if !strings.Contains(err.Error(), "cvc-id clause 2") {
				t.Fatalf("message = %q, want the clause cited inline as \"cvc-id clause 2\" (STYLE E4)", err)
			}
		})
	}
}

// TestProducePaddedIDAccepted pins the other half of collapse: id=" a " alone
// is a valid xs:ID, not a non-NCName.
func TestProducePaddedIDAccepted(t *testing.T) {
	if _, err := produce(t, wrap("", `<xs:element name="e" id=" a " type="xs:string"/>`)); err != nil {
		t.Fatalf("Produce: %v, want id=\" a \" accepted as the xs:ID \"a\"", err)
	}
}

// TestProduceAppinfoContentIDsNotJudged pins that an id inside <appinfo> or
// <documentation> content is no part of the document's ID table: that content
// is lax (xmlschema11-1.md:5727, :5740) and §5.1 lets a processor treat invalid
// <annotation> descendants as valid, so neither a non-NCName there nor a value
// repeating one outside is charged.
func TestProduceAppinfoContentIDsNotJudged(t *testing.T) {
	_, err := produce(t, wrap("", `<xs:element name="e" id="a" type="xs:string">`+
		`<xs:annotation><xs:appinfo><xs:element id="1"/><xs:element id="a"/></xs:appinfo>`+
		`<xs:documentation><xs:sequence id="a"/></xs:documentation></xs:annotation>`+
		`</xs:element>`))
	if err != nil {
		t.Fatalf("Produce: %v, want ids inside <appinfo>/<documentation> content left unjudged", err)
	}
}

// TestParseSameIDAcrossDocumentsAccepted pins the validation root as the schema
// DOCUMENT: two documents of one assembly may carry the same id, as the W3C
// suite's catalog-valid cross-document pairs (ctA029, idA003, …) require.
func TestParseSameIDAcrossDocumentsAccepted(t *testing.T) {
	_, err := parseMap(t, "main.xsd", map[string]string{
		"main.xsd": wrap("", `<xs:include schemaLocation="other.xsd"/>`+
			`<xs:element name="a" id="foo" type="xs:string"/>`),
		"other.xsd": wrap("", `<xs:element name="b" id="foo" type="xs:string"/>`),
	})
	if err != nil {
		t.Fatalf("Parse: %v, want the same id in two schema documents accepted", err)
	}
}

// assertIDFault pins err's message as opening with wantMsg — the offending
// element, its value and, for a duplicate, the first carrier — and its position
// as produceURI line wantLine with a column (E3).
func assertIDFault(t *testing.T, err error, wantMsg string, wantLine int) {
	t.Helper()
	var xe *xsderr.Error
	if !errors.As(err, &xe) {
		t.Fatalf("error %v is not an *xsderr.Error", err)
	}
	if !strings.HasPrefix(xe.Msg, wantMsg) {
		t.Fatalf("message = %q, want prefix %q", xe.Msg, wantMsg)
	}
	if xe.Loc.URI != produceURI || xe.Loc.Line != wantLine || xe.Loc.Col == 0 {
		t.Fatalf("position = %s:%d:%d, want %s:%d with a column", xe.Loc.URI, xe.Loc.Line, xe.Loc.Col, produceURI, wantLine)
	}
}
