package xmltree_test

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/kud360/goxsd8/parser/xmltree"
	"github.com/kud360/goxsd8/xsderr"
)

// collect drains a reader over doc into a slice of nodes, stopping at io.EOF
// and returning the first non-EOF error.
func collect(t *testing.T, uri, doc string) ([]xmltree.Node, error) {
	t.Helper()
	return collectFrom(t, uri, strings.NewReader(doc))
}

// collectFrom is collect over an arbitrary source, for inputs a string cannot
// stand in for: encoded byte streams and readers that fail on demand.
func collectFrom(t *testing.T, uri string, src io.Reader) ([]xmltree.Node, error) {
	t.Helper()
	r := xmltree.NewReader(uri, src)
	var nodes []xmltree.Node
	for {
		n, err := r.Token()
		if errors.Is(err, io.EOF) {
			return nodes, nil
		}
		if err != nil {
			return nodes, err
		}
		nodes = append(nodes, n)
	}
}

// wantWellFormednessError asserts err is a located XML well-formedness fault,
// the one verdict every encoding-layer failure must reach.
func wantWellFormednessError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	rule, ok := xsderr.RuleOf(err)
	if !ok || rule != xsderr.RuleXMLWellFormed {
		t.Errorf("error %v: rule = %q (ok=%v), want %q", err, rule, ok, xsderr.RuleXMLWellFormed)
	}
	if _, ok := xsderr.LocOf(err); !ok {
		t.Errorf("error %v carries no xsderr.Loc", err)
	}
}

// utf16Doc encodes doc as UTF-16 in the given byte order behind the mark XML
// 1.0 §4.3.3 requires of a UTF-16 entity.
func utf16Doc(doc string, bigEndian bool) string {
	var b []byte
	for _, u := range utf16.Encode([]rune("\uFEFF" + doc)) {
		if bigEndian {
			b = append(b, byte(u>>8), byte(u))
			continue
		}
		b = append(b, byte(u), byte(u>>8))
	}
	return string(b)
}

// utf16Units encodes explicit big-endian code units behind a mark, for input
// no Go string can express: lone surrogates and half a code unit.
func utf16Units(units ...uint16) string {
	b := []byte{0xFE, 0xFF}
	for _, u := range units {
		b = append(b, byte(u>>8), byte(u))
	}
	return string(b)
}

// sameNodes asserts two token streams agree in kind, name, data, and location.
// A UTF-16 document decodes to exactly the bytes of its UTF-8 spelling, so
// even locations must match.
func sameNodes(t *testing.T, got, want []xmltree.Node) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d nodes, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Loc() != w.Loc() {
			t.Errorf("node %d loc = %v, want %v", i, got[i].Loc(), w.Loc())
		}
		switch w := w.(type) {
		case *xmltree.StartElement:
			g, ok := got[i].(*xmltree.StartElement)
			if !ok {
				t.Fatalf("node %d = %T, want *StartElement", i, got[i])
			}
			sameStart(t, i, g, w)
		case *xmltree.EndElement:
			g, ok := got[i].(*xmltree.EndElement)
			if !ok {
				t.Fatalf("node %d = %T, want *EndElement", i, got[i])
			}
			if g.Name() != w.Name() {
				t.Errorf("node %d end name = %v, want %v", i, g.Name(), w.Name())
			}
		case *xmltree.CharData:
			g, ok := got[i].(*xmltree.CharData)
			if !ok {
				t.Fatalf("node %d = %T, want *CharData", i, got[i])
			}
			if g.Data() != w.Data() || g.Offset() != w.Offset() {
				t.Errorf("node %d chardata = %q at offset %d, want %q at %d", i, g.Data(), g.Offset(), w.Data(), w.Offset())
			}
		}
	}
}

// sameStart compares one start tag's name and attributes, in order.
func sameStart(t *testing.T, i int, got, want *xmltree.StartElement) {
	t.Helper()
	if got.Name() != want.Name() {
		t.Errorf("node %d start name = %v, want %v", i, got.Name(), want.Name())
	}
	if len(got.Attributes()) != len(want.Attributes()) {
		t.Fatalf("node %d has %d attributes, want %d", i, len(got.Attributes()), len(want.Attributes()))
	}
	for j, a := range want.Attributes() {
		g := got.Attributes()[j]
		if g.Name() != a.Name() || g.Value() != a.Value() {
			t.Errorf("node %d attribute %d = %v=%q, want %v=%q", i, j, g.Name(), g.Value(), a.Name(), a.Value())
		}
	}
}

// byteOrders names the two UTF-16 serializations every decode test runs under.
var byteOrders = []struct {
	name   string
	bigEnd bool
}{
	{"big-endian", true},
	{"little-endian", false},
}

func TestUTF16BOMRoundTrips(t *testing.T) {
	docs := []struct{ name, doc string }{
		// An astral character exercises surrogate-pair decoding.
		{"namespaced multi-line", "<a xmlns=\"urn:D\" x=\"1\">\n  <b:c xmlns:b=\"urn:B\">clef \U0001D11E</b:c>\n</a>"},
		{"xml declaration without encoding", `<?xml version="1.0"?><a><b/></a>`},
	}
	for _, tc := range docs {
		for _, order := range byteOrders {
			t.Run(tc.name+"/"+order.name, func(t *testing.T) {
				want, err := collect(t, "t.xml", tc.doc)
				if err != nil {
					t.Fatalf("UTF-8 baseline: %v", err)
				}
				got, err := collectFrom(t, "t.xml", strings.NewReader(utf16Doc(tc.doc, order.bigEnd)))
				if err != nil {
					t.Fatalf("UTF-16 decode: %v", err)
				}
				sameNodes(t, got, want)
			})
		}
	}
}

// dribbleReader hands out one byte per Read, so every code unit and every
// surrogate pair straddles a fill boundary at some point.
type dribbleReader struct{ s string }

func (d *dribbleReader) Read(p []byte) (int, error) {
	if d.s == "" {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = d.s[0]
	d.s = d.s[1:]
	return 1, nil
}

func TestUTF16StreamsAcrossFillBoundaries(t *testing.T) {
	doc := "<a>" + strings.Repeat("x", 2000) + "\U0001D11E" + "</a>"
	want, err := collect(t, "t.xml", doc)
	if err != nil {
		t.Fatalf("UTF-8 baseline: %v", err)
	}
	sources := []struct {
		name string
		open func(string) io.Reader
	}{
		{"bulk reads", func(s string) io.Reader { return strings.NewReader(s) }},
		{"one byte per read", func(s string) io.Reader { return &dribbleReader{s: s} }},
	}
	for _, src := range sources {
		for _, order := range byteOrders {
			t.Run(src.name+"/"+order.name, func(t *testing.T) {
				got, err := collectFrom(t, "t.xml", src.open(utf16Doc(doc, order.bigEnd)))
				if err != nil {
					t.Fatalf("UTF-16 decode: %v", err)
				}
				sameNodes(t, got, want)
			})
		}
	}
}

func TestUTF16BOMAgreesWithDeclaration(t *testing.T) {
	cases := []struct {
		name     string
		declared string
		bigEnd   bool
	}{
		{"big-endian declares UTF-16", "UTF-16", true},
		{"little-endian declares UTF-16", "UTF-16", false},
		{"big-endian declares UTF-16BE", "UTF-16BE", true},
		{"little-endian declares lowercase utf-16le", "utf-16le", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := `<?xml version="1.0" encoding="` + tc.declared + `"?><a>x</a>`
			nodes, err := collectFrom(t, "t.xml", strings.NewReader(utf16Doc(doc, tc.bigEnd)))
			if err != nil {
				t.Fatalf("decode with encoding=%q: %v", tc.declared, err)
			}
			if len(nodes) != 3 {
				t.Fatalf("got %d nodes, want 3 (start, text, end)", len(nodes))
			}
		})
	}
}

// TestUTF8BOMIsStripped pins XML 1.0 §4.3.3's "encoding signature, not part of
// either the markup or the character data": the root still starts at column 1.
func TestUTF8BOMIsStripped(t *testing.T) {
	nodes, err := collectFrom(t, "t.xml", strings.NewReader("\xEF\xBB\xBF<a>x</a>"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("got %d nodes, want 3", len(nodes))
	}
	wantLoc(t, nodes[0], 1, 1)
}

// TestUTF16IllFormedIsError pins the transcoder's central decision: ill-formed
// UTF-16 is a terminal error, never a U+FFFD substitution. Each case names the
// message it wants, so a substitution mutant cannot hide behind an unrelated
// structural failure — the unpaired surrogate is followed by ordinary
// character data rather than by '<' for exactly that reason.
func TestUTF16IllFormedIsError(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"unpaired surrogate", utf16Units('<', 'a', '>', 0xD834, 'x', '<', '/', 'a', '>'), "unpaired surrogate"},
		{"truncated code unit", utf16Units('<', 'a', '/', '>') + "\x00", "do not form a code unit"},
		{"mismatched end tag", utf16Units('<', 'a', '>', '<', 'b', '>', '<', '/', 'a', '>'), "does not match open element b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collectFrom(t, "t.xml", strings.NewReader(tc.src))
			wantWellFormednessError(t, err)
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestBOMContradictsEncodingDeclaration pins XML 1.0 §4.3.3's fatal error for
// an entity presented in an encoding other than the one it declares.
func TestBOMContradictsEncodingDeclaration(t *testing.T) {
	decl := func(enc string) string {
		return `<?xml version="1.0" encoding="` + enc + `"?><a/>`
	}
	cases := []struct{ name, src string }{
		{"big-endian mark declares UTF-8", utf16Doc(decl("UTF-8"), true)},
		{"little-endian mark declares UTF-16BE", utf16Doc(decl("UTF-16BE"), false)},
		{"big-endian mark declares UTF-16LE", utf16Doc(decl("UTF-16LE"), true)},
		{"no mark declares UTF-16", decl("UTF-16")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collectFrom(t, "t.xml", strings.NewReader(tc.src))
			wantWellFormednessError(t, err)
			if !strings.Contains(err.Error(), "disagrees") {
				t.Errorf("error = %q, want a disagreement message", err)
			}
		})
	}
}

// failFirstReader fails on its very first Read and reports end-of-input on
// every one after: the shape that exposes a dropped Peek error, since the
// retry answers with io.EOF and nothing carries the original cause any more.
type failFirstReader struct {
	err  error
	done bool
}

func (f *failFirstReader) Read([]byte) (int, error) {
	if f.done {
		return 0, io.EOF
	}
	f.done = true
	return 0, f.err
}

// prefixThenFailReader yields prefix, then fails: enough bytes for a mark to
// be ruled out without the peek itself failing, so no error is latched.
type prefixThenFailReader struct {
	prefix string
	err    error
}

func (p *prefixThenFailReader) Read(b []byte) (int, error) {
	if p.prefix == "" {
		return 0, p.err
	}
	n := copy(b, p.prefix)
	p.prefix = p.prefix[n:]
	return n, nil
}

// TestSourceFailureSurfacesItsCause guards the byte-order-mark layer against
// swallowing a read failure. A source that fails before a mark can be read is
// the one bufio.Reader.Peek hands its error to exactly once, so a dropped
// error there turns a broken source into a bare io.EOF — which Token
// documents as the end of a well-formed document.
func TestSourceFailureSurfacesItsCause(t *testing.T) {
	cause := errors.New("boom: original cause")
	cases := []struct {
		name      string
		open      func() io.Reader
		wantNodes int
	}{
		{"fails before a mark can be read", func() io.Reader { return &failFirstReader{err: cause} }, 0},
		{"fails after a mark is ruled out", func() io.Reader { return &prefixThenFailReader{prefix: "<a>", err: cause} }, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nodes, err := collectFrom(t, "t.xml", tc.open())
			if err == nil || errors.Is(err, io.EOF) {
				t.Fatalf("err = %v, want the source's failure, not end-of-document", err)
			}
			wantWellFormednessError(t, err)
			if !errors.Is(err, cause) {
				t.Errorf("error %q does not unwrap to the original cause", err)
			}
			if !strings.Contains(err.Error(), "boom") {
				t.Errorf("error = %q, want it to name the original cause", err)
			}
			if len(nodes) != tc.wantNodes {
				t.Errorf("got %d nodes, want %d", len(nodes), tc.wantNodes)
			}
		})
	}
}

func wantLoc(t *testing.T, n xmltree.Node, line, col int) {
	t.Helper()
	loc := n.Loc()
	if loc.Line != line || loc.Col != col {
		t.Errorf("loc = %d:%d, want %d:%d (node %T)", loc.Line, loc.Col, line, col, n)
	}
}

func TestPositionsMultiLine(t *testing.T) {
	nodes, err := collect(t, "t.xml", "<a>\n  <b>x</b>\n</a>")
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	// START a, CharData "\n  ", START b, CharData "x", END b, CharData "\n", END a
	if len(nodes) != 7 {
		t.Fatalf("got %d nodes, want 7", len(nodes))
	}
	if nodes[0].Loc().URI != "t.xml" {
		t.Errorf("URI = %q, want t.xml", nodes[0].Loc().URI)
	}
	wantLoc(t, nodes[0], 1, 1) // <a>
	wantLoc(t, nodes[1], 1, 4) // "\n  " starts right after '>'
	wantLoc(t, nodes[2], 2, 3) // <b> after two spaces
	wantLoc(t, nodes[3], 2, 6) // 'x'
	wantLoc(t, nodes[4], 2, 7) // </b>
	wantLoc(t, nodes[6], 3, 1) // </a>

	cd, ok := nodes[3].(*xmltree.CharData)
	if !ok {
		t.Fatalf("nodes[3] = %T, want *CharData", nodes[3])
	}
	if cd.Data() != "x" {
		t.Errorf("chardata = %q, want x", cd.Data())
	}
	if cd.Offset() != 9 {
		t.Errorf("offset = %d, want 9", cd.Offset())
	}
}

func TestPositionsCRLF(t *testing.T) {
	// Columns count raw bytes, so the CR contributes a column even though
	// encoding/xml normalizes CRLF to LF in the character data value.
	nodes, err := collect(t, "t.xml", "<a>\r\n<b/>\r\n</a>")
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	wantLoc(t, nodes[0], 1, 1) // <a>
	wantLoc(t, nodes[1], 1, 4) // CharData at the CR, still line 1
	wantLoc(t, nodes[2], 2, 1) // <b/> on line 2
}

func TestNamespaceDefaultAndShadowing(t *testing.T) {
	// Outer default urn:D, inner element rebinds default to urn:E for <c>.
	nodes, err := collect(t, "t.xml", `<a xmlns="urn:D"><b><c xmlns="urn:E"/></b></a>`)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	a := nodes[0].(*xmltree.StartElement)
	if a.Name().Space() != "urn:D" || a.Name().Local() != "a" {
		t.Errorf("a name = %v", a.Name())
	}
	c := nodes[2].(*xmltree.StartElement)
	if c.Name().Space() != "urn:E" {
		t.Errorf("c space = %q, want urn:E (default shadowed)", c.Name().Space())
	}
	// After </c></b>, the closing </a> must still resolve under urn:D.
	last := nodes[len(nodes)-1].(*xmltree.EndElement)
	if last.Name().Space() != "urn:D" || last.Name().Local() != "a" {
		t.Errorf("closing a = %v, want {urn:D}a", last.Name())
	}
}

func TestPrefixShadowing(t *testing.T) {
	// p bound to urn:P at <a>, rebound to urn:Q at <p:b>; <p:c> sees urn:Q.
	nodes, err := collect(t, "t.xml", `<a xmlns:p="urn:P"><p:b xmlns:p="urn:Q"><p:c/></p:b></a>`)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	b := nodes[1].(*xmltree.StartElement)
	if b.Name().Space() != "urn:Q" {
		t.Errorf("b space = %q, want urn:Q", b.Name().Space())
	}
	// LookupPrefix on the inner element yields the shadowing binding.
	if uri, ok := b.LookupPrefix("p"); !ok || uri != "urn:Q" {
		t.Errorf("LookupPrefix(p) on b = (%q,%v), want (urn:Q,true)", uri, ok)
	}
	c := nodes[2].(*xmltree.StartElement)
	if c.Name().Space() != "urn:Q" {
		t.Errorf("c space = %q, want urn:Q", c.Name().Space())
	}
}

func TestAttributesNoDefaultNamespace(t *testing.T) {
	// Default namespace applies to element names, never to attribute names.
	nodes, err := collect(t, "t.xml", `<a xmlns="urn:D" x="1"/>`)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	a := nodes[0].(*xmltree.StartElement)
	attrs := a.Attributes()
	if len(attrs) != 1 {
		t.Fatalf("got %d attrs, want 1 (xmlns is not an attribute)", len(attrs))
	}
	if attrs[0].Name().Space() != "" || attrs[0].Name().Local() != "x" {
		t.Errorf("attr name = %v, want unqualified x", attrs[0].Name())
	}
	if attrs[0].Value() != "1" {
		t.Errorf("attr value = %q, want 1", attrs[0].Value())
	}
}

func TestPrefixedAttributeResolves(t *testing.T) {
	nodes, err := collect(t, "t.xml", `<a xmlns:p="urn:P" p:x="1"/>`)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	a := nodes[0].(*xmltree.StartElement)
	if got := a.Attributes()[0].Name(); got.Space() != "urn:P" || got.Local() != "x" {
		t.Errorf("attr name = %v, want {urn:P}x", got)
	}
}

func TestXMLPrefixImplicit(t *testing.T) {
	// The xml: prefix resolves with no xmlns:xml declaration.
	nodes, err := collect(t, "t.xml", `<r><x xml:lang="en"/></r>`)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	x := nodes[1].(*xmltree.StartElement)
	a := x.Attributes()[0]
	if a.Name().Space() != "http://www.w3.org/XML/1998/namespace" || a.Name().Local() != "lang" {
		t.Errorf("attr name = %v, want the XML namespace lang", a.Name())
	}
	if uri, ok := x.LookupPrefix("xml"); !ok || uri != "http://www.w3.org/XML/1998/namespace" {
		t.Errorf("LookupPrefix(xml) = (%q,%v)", uri, ok)
	}
}

func TestUnboundPrefixIsError(t *testing.T) {
	cases := map[string]string{
		"element":   `<a><u:b/></a>`,
		"attribute": `<a u:x="1"/>`,
		"end tag":   `<a><b></p:b></a>`,
	}
	// Deterministic iteration is irrelevant here (independent subtests).
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := collect(t, "t.xml", doc)
			if err == nil {
				t.Fatalf("want error for %q", doc)
			}
			loc, ok := xsderr.LocOf(err)
			if !ok {
				t.Fatalf("error %v carries no xsderr.Loc", err)
			}
			if loc.Line == 0 || loc.URI != "t.xml" {
				t.Errorf("error loc = %v, want a located t.xml error", loc)
			}
			if !strings.Contains(err.Error(), "unbound namespace prefix") {
				t.Errorf("error = %q, want an unbound-prefix message", err)
			}
		})
	}
}

func TestMismatchedEndTagIsError(t *testing.T) {
	_, err := collect(t, "t.xml", `<a></b>`)
	if err == nil {
		t.Fatal("want error for mismatched end tag")
	}
	if _, ok := xsderr.LocOf(err); !ok {
		t.Fatalf("error %v carries no location", err)
	}
}

func TestUnclosedElementIsError(t *testing.T) {
	_, err := collect(t, "t.xml", `<a><b></b>`)
	if err == nil {
		t.Fatal("want error for unclosed root element")
	}
	if !strings.Contains(err.Error(), "unclosed") {
		t.Errorf("error = %q, want an unclosed-element message", err)
	}
}

func TestMalformedXMLIsErrorNotPanic(t *testing.T) {
	_, err := collect(t, "t.xml", "<a>\n<b>\x00</b></a>")
	if err == nil {
		t.Fatal("want error for control character in content")
	}
	loc, ok := xsderr.LocOf(err)
	if !ok || loc.URI != "t.xml" {
		t.Errorf("malformed-XML error missing located wrap: %v", err)
	}
}

// TestOnlyMiscFollowsTheDocumentElement pins XML 1.0 [1] document ::= prolog
// element Misc*, with [27] Misc ::= Comment | PI | S: non-white-space
// character data after the document element is a well-formedness fault located
// where its character-data token starts in the source — across CRLF line ends
// and character references the decoder replaces — while white space, a comment
// and a PI there are accepted. S is the source's own characters: a CDATA
// section, empty or not, and a character reference are content [43] and no
// Misc, though each decodes to white space (#2089).
func TestOnlyMiscFollowsTheDocumentElement(t *testing.T) {
	for _, tc := range []struct {
		name, doc string
		at        xsderr.Loc // zero: accepted
	}{
		{"text after CRLF lines", "<a>\r\n</a>\r\n\r\n  not well-formed\r\n", xsderr.Loc{URI: "t.xml", Line: 2, Col: 5}},
		{"text on the end tag's line", "<a/> x", xsderr.Loc{URI: "t.xml", Line: 1, Col: 5}},
		{"text after a comment", "<a/>\n<!-- c -->\nx", xsderr.Loc{URI: "t.xml", Line: 2, Col: 11}},
		{"text after a line-feed reference", "<a/>&#10;x", xsderr.Loc{URI: "t.xml", Line: 1, Col: 5}},
		{"text after a space reference", "<a/>&#32;x", xsderr.Loc{URI: "t.xml", Line: 1, Col: 5}},
		{"CDATA section holding a space", "<a/><![CDATA[ ]]>", xsderr.Loc{URI: "t.xml", Line: 1, Col: 5}},
		{"empty CDATA section", "<a/>\n<![CDATA[]]>", xsderr.Loc{URI: "t.xml", Line: 2, Col: 1}},
		{"space reference", "<a/>&#32;", xsderr.Loc{URI: "t.xml", Line: 1, Col: 5}},
		{"line-feed reference", "<a/>&#10;", xsderr.Loc{URI: "t.xml", Line: 1, Col: 5}},
		{"tab reference after white space", "<a/>\n &#x9;", xsderr.Loc{URI: "t.xml", Line: 1, Col: 5}},
		{"white space", "<a/>\r\n \t\n", xsderr.Loc{}},
		{"comment", "<a/>\n<!-- c -->\n", xsderr.Loc{}},
		{"processing instruction", "<a/>\n<?pi data?>\n", xsderr.Loc{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(t, "t.xml", tc.doc)
			if tc.at == (xsderr.Loc{}) {
				if err != nil {
					t.Fatalf("collect: %v, want the document accepted", err)
				}
				return
			}
			wantWellFormednessError(t, err)
			if loc, _ := xsderr.LocOf(err); loc != tc.at {
				t.Errorf("fault at %v, want %v, where the character-data token starts", loc, tc.at)
			}
			want := fmt.Sprintf("t.xml:%d:%d: [xml-wf] character data after the document element", tc.at.Line, tc.at.Col)
			if !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error = %q, want it to open %q", err, want)
			}
		})
	}
}

// TestOnlyMiscPrecedesTheDocumentElement pins XML 1.0 [1] document ::= prolog
// element Misc*, with [22] prolog ::= XMLDecl? Misc* (doctypedecl Misc*)? and
// [27] Misc ::= Comment | PI | S: character data before the document element
// that is not the source's own S — text, a character reference or CDATA
// section decoding to white space, or a U+FEFF after the encoding signature —
// is a well-formedness fault located where its character-data token starts.
// White space, comments, PIs, an XML declaration and a DOCTYPE there, one
// whose internal subset holds a PI with a '>' among them, are accepted.
func TestOnlyMiscPrecedesTheDocumentElement(t *testing.T) {
	const mark = "\xEF\xBB\xBF"
	for _, tc := range []struct {
		name, doc string
		at        xsderr.Loc // zero: accepted
	}{
		{"text", "junk<r/>", xsderr.Loc{URI: "t.xml", Line: 1, Col: 1}},
		{"text after a DOCTYPE", "<!DOCTYPE r>junk<r/>", xsderr.Loc{URI: "t.xml", Line: 1, Col: 13}},
		{"text after an XML declaration", "<?xml version=\"1.0\"?>\r\n x\r\n<r/>", xsderr.Loc{URI: "t.xml", Line: 1, Col: 22}},
		{"space reference", "&#32;<r/>", xsderr.Loc{URI: "t.xml", Line: 1, Col: 1}},
		{"CDATA section holding a space", "\n<![CDATA[ ]]><r/>", xsderr.Loc{URI: "t.xml", Line: 2, Col: 1}},
		{"a second byte-order mark", mark + mark + "<r/>", xsderr.Loc{URI: "t.xml", Line: 1, Col: 1}},
		{"white space, comments and PIs", "\r\n \t<!-- c -->\n<?pi data?>\n<r/>", xsderr.Loc{}},
		{"XML declaration and DOCTYPE", "<?xml version=\"1.0\"?>\n<!DOCTYPE r>\n<r/>", xsderr.Loc{}},
		{"one byte-order mark", mark + "<r/>", xsderr.Loc{}},
		{"internal subset holding a PI with a '>'", "<!DOCTYPE r [<?x a > b?><!NOTATION n SYSTEM 'x'><!ENTITY pic SYSTEM 'u' NDATA n>]><r/>", xsderr.Loc{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(t, "t.xml", tc.doc)
			if tc.at == (xsderr.Loc{}) {
				if err != nil {
					t.Fatalf("collect: %v, want the document accepted", err)
				}
				return
			}
			wantWellFormednessError(t, err)
			want := fmt.Sprintf("t.xml:%d:%d: [xml-wf] character data before the document element", tc.at.Line, tc.at.Col)
			if !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error = %q, want it to open %q", err, want)
			}
		})
	}
}

// TestNoElementFollowsTheDocumentElement pins XML 1.0 [1] document ::= prolog
// element Misc*: a start tag after the document element's end tag is a
// well-formedness fault located at that tag, charged before any fault of the
// tag's own attributes, whatever Misc stands between.
func TestNoElementFollowsTheDocumentElement(t *testing.T) {
	for _, tc := range []struct {
		name, doc, want string
	}{
		{"adjacent", "<a/><b/>", "t.xml:1:5: [xml-wf] element <b> after the document element"},
		{"after Misc", "<a></a>\n<!-- c -->\n<?pi?> <b>x</b>", "t.xml:3:8: [xml-wf] element <b> after the document element"},
		{"unbound element and attribute prefixes", "<a/><p:b q:c='1'/>", "t.xml:1:5: [xml-wf] element <p:b> after the document element"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(t, "t.xml", tc.doc)
			wantWellFormednessError(t, err)
			if !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to open %q", err, tc.want)
			}
		})
	}
}

// TestIllFormedUTF8InMarkupIsError pins XML 1.0 §4.3.3: an ill-formed UTF-8
// code unit sequence in a comment, processing instruction or directive — the
// DOCTYPE and its internal subset included, an unreferenced entity value and
// an NDATA name among them — is a well-formedness fault located at the
// sequence's first byte, and an unparsed entity whose declaration holds one is
// not declared. Well-formed sequences read, U+FFFD spelled as one included:
// the check is UTF-8 validity, not a byte value.
func TestIllFormedUTF8InMarkupIsError(t *testing.T) {
	for _, tc := range []struct {
		name, doc string
		bad       string // the ill-formed sequence; "" when the document reads
	}{
		{"document type name", "<!DOCTYPE r\xff><r/>", "\xff"},
		{"comment in the subset", "<!DOCTYPE r [<!-- \xff -->]><r/>", "\xff"},
		{"NDATA name", "<!DOCTYPE r [<!ENTITY bad SYSTEM 'u' NDATA a\xffb>]><r/>", "\xff"},
		{"unreferenced entity value", "<!DOCTYPE r [<!ENTITY e \"a\xffb\">]><r/>", "\xff"},
		{"system literal", "<!DOCTYPE r SYSTEM 'a\xffb'><r/>", "\xff"},
		{"PI in the subset", "<!DOCTYPE r [<?pi \xff?>]><r/>", "\xff"},
		{"element declaration", "<!DOCTYPE r [<!ELEMENT r\xff ANY>]><r/>", "\xff"},
		{"overlong sequence", "<!DOCTYPE r [<!ENTITY e \"a\xc0\xafb\">]><r/>", "\xc0\xaf"},
		{"surrogate", "<!DOCTYPE r [<!ENTITY e \"a\xed\xa0\x80b\">]><r/>", "\xed\xa0\x80"},
		{"truncated sequence", "<!DOCTYPE r [<!ENTITY e \"a\xe2\x82b\">]><r/>", "\xe2\x82"},
		{"comment before the document element", "<!-- \xff --><r/>", "\xff"},
		{"PI before the document element", "<?pi \xff?><r/>", "\xff"},
		{"comment in content", "<r>\n<!-- \xff --></r>", "\xff"},
		{"PI in content", "<r><?pi \xff?></r>", "\xff"},
		{"directive in content", "<r><!DOCTYPE x\xff></r>", "\xff"},
		{"comment after the document element", "<r/>\n\n<!-- \xff -->", "\xff"},
		{"non-ASCII entity value", "<!DOCTYPE r [<!ENTITY e \"aéb\">]><r/>", ""},
		{"non-ASCII document type name", "<!DOCTYPE ré><ré/>", ""},
		{"supplementary character in a comment and a PI", "<!-- \U0001D11E --><?pi \U0001D11E?><r/>", ""},
		{"U+FFFD in an NDATA name", "<!DOCTYPE r [<!ENTITY bad SYSTEM 'u' NDATA a�b>]><r/>", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := xmltree.NewReader("t.xml", strings.NewReader(tc.doc))
			var err error
			for err == nil {
				_, err = r.Token()
			}
			if tc.bad == "" {
				if !errors.Is(err, io.EOF) {
					t.Fatalf("Token: %v, want the document read to io.EOF", err)
				}
				if strings.Contains(tc.doc, "NDATA") && !r.HasUnparsedEntity("bad") {
					t.Errorf("HasUnparsedEntity(%q) = false, want the well-formed declaration read", "bad")
				}
				return
			}
			wantWellFormednessError(t, err)
			at := strings.Index(tc.doc, tc.bad)
			line := 1 + strings.Count(tc.doc[:at], "\n")
			col := at - strings.LastIndexByte(tc.doc[:at], '\n')
			if loc, _ := xsderr.LocOf(err); loc != (xsderr.Loc{URI: "t.xml", Line: line, Col: col}) {
				t.Errorf("fault at %v, want t.xml:%d:%d, the sequence's first byte", loc, line, col)
			}
			want := fmt.Sprintf("t.xml:%d:%d: [xml-wf] ill-formed UTF-8 byte sequence", line, col)
			if !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error = %q, want it to open %q, at the sequence's first byte", err, want)
			}
			if r.HasUnparsedEntity("bad") {
				t.Errorf("HasUnparsedEntity(%q) = true after the fault, want false", "bad")
			}
		})
	}
}

// TestNonCharInMarkupIsError pins XML 1.0 §2.2 with [2] Char, [15] Comment and
// [16] PI: a well-formed UTF-8 sequence encoding no Char in a comment,
// processing instruction or directive — the DOCTYPE's internal subset, an
// entity value and an ATTLIST default included — is a well-formedness fault
// located at the sequence's first byte, with a message of its own, not
// §4.3.3's. The boundaries of Char and the white space it admits read.
func TestNonCharInMarkupIsError(t *testing.T) {
	for _, tc := range []struct {
		name, doc string
		bad       string // the offending character; "" when the document reads
	}{
		{"comment before the document element", "<!-- \x01 --><r/>", "\x01"},
		{"PI before the document element", "<?pi \x01?><r/>", "\x01"},
		{"comment in content", "<r><!-- \x01 --></r>", "\x01"},
		{"PI in content", "<r><?pi \x01?></r>", "\x01"},
		{"comment in the subset", "<!DOCTYPE r [<!-- \x01 -->]><r/>", "\x01"},
		{"PI in the subset", "<!DOCTYPE r [<?pi \x01?>]><r/>", "\x01"},
		{"entity value", "<!DOCTYPE r [<!ENTITY e \"\x01\">]><r/>", "\x01"},
		{"ATTLIST default", "<!DOCTYPE r [<!ATTLIST r a CDATA \"\x01\">]><r/>", "\x01"},
		{"U+FFFE in a comment", "<!-- ￾ --><r/>", "￾"},
		{"U+FFFF in a PI", "<?pi ￿?><r/>", "￿"},
		{"line ends in a comment", "<!--\t\n\r\n --><r/>", ""},
		{"line ends in a PI", "<?pi \t\n\r\n?><r/>", ""},
		{"Char boundaries in a comment", "<!-- ퟿�\U00010000\U0010FFFF --><r/>", ""},
		{"Char boundaries in a PI", "<?pi ퟿�\U00010000\U0010FFFF?><r/>", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := xmltree.NewReader("t.xml", strings.NewReader(tc.doc))
			var err error
			for err == nil {
				_, err = r.Token()
			}
			if tc.bad == "" {
				if !errors.Is(err, io.EOF) {
					t.Fatalf("Token: %v, want the document read to io.EOF", err)
				}
				return
			}
			wantWellFormednessError(t, err)
			at := strings.Index(tc.doc, tc.bad)
			if loc, _ := xsderr.LocOf(err); loc != (xsderr.Loc{URI: "t.xml", Line: 1, Col: at + 1}) {
				t.Errorf("fault at %v, want t.xml:1:%d, the character's first byte", loc, at+1)
			}
			c, _ := utf8.DecodeRuneInString(tc.bad)
			want := fmt.Sprintf("t.xml:1:%d: [xml-wf] character %U in markup is no Char", at+1, c)
			if !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error = %q, want it to open %q", err, want)
			}
		})
	}
}

// TestNonCharInContentIsDecoderFault pins that a non-Char in character data
// stays the decoder's charge, located where the decoder stops.
func TestNonCharInContentIsDecoderFault(t *testing.T) {
	r := xmltree.NewReader("t.xml", strings.NewReader("<r>\x01</r>"))
	var err error
	for err == nil {
		_, err = r.Token()
	}
	wantWellFormednessError(t, err)
	if loc, _ := xsderr.LocOf(err); loc != (xsderr.Loc{URI: "t.xml", Line: 1, Col: 5}) {
		t.Errorf("fault at %v, want t.xml:1:5", loc)
	}
	if !strings.Contains(err.Error(), "illegal character code U+0001") {
		t.Errorf("error = %q, want the decoder's %q", err, "illegal character code U+0001")
	}
}

func TestEOFIsIdempotent(t *testing.T) {
	r := xmltree.NewReader("t.xml", strings.NewReader("<a/>"))
	for {
		_, err := r.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if _, err := r.Token(); !errors.Is(err, io.EOF) {
		t.Errorf("second post-EOF Token = %v, want io.EOF", err)
	}
}

// TestPrefixUndeclarationIs11Only pins nsc-NoPrefixUndecl: in a document that
// is XML 1.0 — no declaration, or any label but 1.1 — a prefixed namespace
// declaration with an empty value is a located well-formedness fault at that
// declaration's own name, while under a 1.1 label it undeclares the prefix
// (Namespaces in XML 1.1). The default declaration xmlns="" is legal in 1.0.
func TestPrefixUndeclarationIs11Only(t *testing.T) {
	body := "<a xmlns:p='urn:p'>\n  <b xmlns:p=''/>\n</a>"
	for _, tc := range []struct {
		name, doc string
		line      int // 0: accepted
	}{
		{"no declaration", body, 2},
		{"version 1.0", "<?xml version='1.0'?>\n" + body, 3},
		{"version 1.10", "<?xml version='1.10'?>\n" + body, 3},
		{"version 1.1", "<?xml version='1.1'?>\n" + body, 0},
		{"default undeclared in 1.0", "<?xml version='1.0'?>\n<a xmlns='urn:d'>\n  <b xmlns=''/>\n</a>", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(t, "t.xml", tc.doc)
			if tc.line == 0 {
				if err != nil {
					t.Fatalf("collect: %v, want the document accepted", err)
				}
				return
			}
			wantWellFormednessError(t, err)
			if loc, _ := xsderr.LocOf(err); loc != (xsderr.Loc{URI: "t.xml", Line: tc.line, Col: 6}) {
				t.Errorf("fault at %v, want t.xml:%d:6, the name of <b>'s xmlns:p", loc, tc.line)
			}
			if want := fmt.Sprintf("t.xml:%d:6: [xml-wf] namespace declaration xmlns:p has an empty value", tc.line); !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error = %q, want it to open %q", err, want)
			}
		})
	}
}

// TestVersion1xIsReadAs10 pins XML 1.0 §2.8's Note: a document whose
// declaration specifies a 1.x version number other than 1.0 is read as the 1.0
// document it would be under a 1.0 label, nodes and locations alike, in either
// quote style, and with a minor version longer than one digit.
func TestVersion1xIsReadAs10(t *testing.T) {
	body := "\n<a x='1'>\n  <b>t</b>\n</a>"
	cases := []struct{ name, decl, same string }{
		{"double-quoted 1.1", `<?xml version="1.1"?>`, `<?xml version="1.0"?>`},
		{"single-quoted 1.1", `<?xml version='1.1' encoding='UTF-8'?>`, `<?xml version='1.0' encoding='UTF-8'?>`},
		{"spaced Eq", "<?xml\tversion = \"1.9\" ?>", "<?xml\tversion = \"1.0\" ?>"},
		{"two-digit minor version", `<?xml version="1.10" standalone="yes"?>`, `<?xml version="1.0"  standalone="yes"?>`},
		{"two-digit minor version before ?>", `<?xml version="1.01"?>`, `<?xml version="1.0" ?>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect(t, "t.xml", tc.decl+body)
			if err != nil {
				t.Fatalf("collect(%s): %v", tc.decl, err)
			}
			want, err := collect(t, "t.xml", tc.same+body)
			if err != nil {
				t.Fatalf("collect(%s): %v", tc.same, err)
			}
			sameNodes(t, got, want)
		})
	}
}

// TestVersionLabelRejectedOutside1x pins the other side of §2.8: only a 1.x
// VersionNum ([26] '1.' [0-9]+) is read as 1.0, so any other label is still a
// well-formedness fault.
func TestVersionLabelRejectedOutside1x(t *testing.T) {
	for _, decl := range []string{
		`<?xml version="2.0"?>`,
		`<?xml version="1.1a"?>`,
		`<?xml version="1."?>`,
		// The two-digit rewrite would make this one well-formed by supplying
		// the S that [23] requires before encoding.
		`<?xml version="1.10"encoding="UTF-8"?>`,
	} {
		t.Run(decl, func(t *testing.T) {
			_, err := collect(t, "t.xml", decl+"<a/>")
			wantWellFormednessError(t, err)
		})
	}
}

// TestFaultAfterDeclarationLocatedAlikeUnder10And11 pins that the label's
// rewrite moves no location: a fault after the declaration, on its own line
// and on the next, is reported at the same line:col with the same message
// under a 1.0 and a 1.1 label.
func TestFaultAfterDeclarationLocatedAlikeUnder10And11(t *testing.T) {
	for _, body := range []string{"<a></b>", "\n<a>\n  <b></c>\n</a>"} {
		_, err10 := collect(t, "t.xml", `<?xml version="1.0"?>`+body)
		_, err11 := collect(t, "t.xml", `<?xml version="1.1"?>`+body)
		if err10 == nil || err11 == nil {
			t.Fatalf("body %q: errors = %v / %v, want a fault under both labels", body, err10, err11)
		}
		loc10, _ := xsderr.LocOf(err10)
		loc11, ok := xsderr.LocOf(err11)
		if !ok || loc11 != loc10 {
			t.Errorf("body %q: 1.1 fault at %v (located %v), want the 1.0 fault's %v", body, loc11, ok, loc10)
		}
		if err11.Error() != err10.Error() {
			t.Errorf("body %q: 1.1 fault %q, want the 1.0 fault %q", body, err11, err10)
		}
	}
}

// TestNamesAreFifthEdition pins the reader's name check to XML 1.0 5th
// edition [5] Name over [4] NameStartChar and [4a] NameChar (xml.md §2.3): a
// name 5e admits and 4th edition's Appendix B tables do not (U+0133 lies in
// [#xF8-#x2FF]) reads as an element and attribute name, in the document and in
// an internal entity's replacement text; a name [4] does not admit ('-' is
// only a NameChar) is a well-formedness fault, located at the decoder's offset
// after the name in the document and at the reference in replacement text.
func TestNamesAreFifthEdition(t *testing.T) {
	const ref = `<!DOCTYPE r [<!ENTITY e "<-04-29/>">]><r>`
	for _, tc := range []struct {
		name, doc string
		want      string     // the rendered nodes when accepted, else the error's opening
		at        xsderr.Loc // zero: accepted
	}{
		{"5e name in the document", `<Dĳkstra vrĳtag="0"/>`, `<Dĳkstra vrĳtag="0"></Dĳkstra>`, xsderr.Loc{}},
		{"5e name in replacement text", `<!DOCTYPE r [<!ENTITY e "<Dĳkstra vrĳtag='0'/>">]><r>&e;</r>`,
			`<r><Dĳkstra vrĳtag="0"></Dĳkstra></r>`, xsderr.Loc{}},
		{"name outside [4] in the document", `<-04-29/>`,
			"t.xml:1:8: [xml-wf] XML syntax error on line 1: invalid XML name: -04-29",
			xsderr.Loc{URI: "t.xml", Line: 1, Col: 8}},
		{"name outside [4] in replacement text", ref + `&e;</r>`,
			"t.xml:1:42: [xml-wf] in the replacement text of entity e: XML syntax error on line 1: invalid XML name: -04-29",
			xsderr.Loc{URI: "t.xml", Line: 1, Col: len(ref) + 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes, err := collect(t, "t.xml", tc.doc)
			if tc.at == (xsderr.Loc{}) {
				if err != nil {
					t.Fatalf("collect: %v, want the document read", err)
				}
				if got := render(nodes); got != tc.want {
					t.Errorf("read %s\n want %s", got, tc.want)
				}
				return
			}
			wantWellFormednessError(t, err)
			if loc, _ := xsderr.LocOf(err); loc != tc.at {
				t.Errorf("fault at %v, want %v", loc, tc.at)
			}
			if !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to open %q", err, tc.want)
			}
		})
	}
}
