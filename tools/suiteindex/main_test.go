package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/kud360/goxsd8/tools/internal/suiteroot"
	"github.com/kud360/goxsd8/xsd"
)

// utf16LEDoc encodes doc as UTF-16LE behind the byte-order mark XML 1.0
// §4.3.3 requires of a UTF-16 entity — the encoding 93 of the suite's
// fixtures ship in, and the one a UTF-8 grep reads as interleaved NULs
// (#1239). parser/xmltree's own tests hold an equivalent helper, but a test
// file's helpers are private to its package, so this is a second spelling
// rather than a second decoder.
func utf16LEDoc(doc string) string {
	var b []byte
	for _, u := range utf16.Encode([]rune("\uFEFF" + doc)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return string(b)
}

// The three spellings of one construct the suite actually writes. Every
// fixture below declares the same element — an XSD `element` carrying
// `targetNamespace` — and no two agree on how to spell it.
const (
	unprefixedDoc = `<?xml version="1.0"?>
<schema xmlns="http://www.w3.org/2001/XMLSchema">
  <complexType name="ct">
    <sequence>
      <element name="a" targetNamespace="urn:b"/>
    </sequence>
  </complexType>
</schema>`

	xsPrefixDoc = `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="ct">
    <xs:sequence>
      <xs:element name="a" targetNamespace="urn:b"/>
    </xs:sequence>
  </xs:complexType>
</xs:schema>`

	xsdPrefixDoc = `<?xml version="1.0"?>
<xsd:schema xmlns:xsd="http://www.w3.org/2001/XMLSchema">
  <xsd:complexType name="ct">
    <xsd:sequence>
      <xsd:element name="a" targetNamespace="urn:b"/>
    </xsd:sequence>
  </xsd:complexType>
</xsd:schema>`

	// impostorDoc spells the prefix "xsd" over a namespace that is not the
	// XML Schema one. Its element is not the construct, however it reads.
	impostorDoc = `<?xml version="1.0"?>
<xsd:schema xmlns:xsd="urn:not-the-schema-namespace">
  <xsd:element name="a" targetNamespace="urn:b"/>
</xsd:schema>`

	// instanceNSDoc is an INSTANCE document in the namespace its test targets,
	// which is the shape no default of the element position reaches: not the
	// XML Schema namespace a braceless name means, and not the no-namespace
	// one `{}` means (#1495). boeingData/ipo1/ipo_1.xml is the corpus's own.
	instanceNSDoc = `<?xml version="1.0"?>
<order xmlns="urn:target" xmlns:t="urn:target" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <shipTo xsi:type="t:USAddress"/>
</order>`

	// instanceBareDoc carries the same instance construct in NO namespace —
	// the slice the `{}` form reaches, and the reason its count reads as a
	// population when it is a part of one.
	instanceBareDoc = `<?xml version="1.0"?>
<order xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <shipTo xsi:type="USAddress"/>
</order>`

	// noNamespaceDoc is written in NO namespace throughout, the shape the
	// corpus carries wherever a wrapper declaring no default xmlns holds a
	// schema (msData/additional/test93490_14.xml). Every element in it is the
	// case #1297 turns on, and its one namespaced attribute is the control.
	noNamespaceDoc = `<?xml version="1.0"?>
<root>
  <wrapper xmlns:o="urn:o" id="w" o:keep="x">
    <inner/>
  </wrapper>
</root>`
)

// ns is the XML Schema namespace in the Clark notation the report renders
// every name in.
const ns = "{http://www.w3.org/2001/XMLSchema}"

// xsiNS is the XML Schema instance namespace, and xsiType the attribute half
// of every query below that censuses `xsi:type` — the instance-side construct
// #1494 wanted a population for and #1495 found inexpressible.
const (
	xsiNS   = "http://www.w3.org/2001/XMLSchema-instance"
	xsiType = "@{" + xsiNS + "}type"
	// anyNS is the namespace axis as a query writes it.
	anyNS = "{" + wildcard + "}"
)

// attrPairs renders one hit's matched attributes as "local=value" in the
// order the hit records them, which is what the assertions below compare: a
// []attrHit prints as a wall of struct fields.
func attrPairs(h hit) string {
	var pairs []string
	for _, a := range h.Attrs {
		pairs = append(pairs, a.Name.Local+"="+a.Value)
	}
	return strings.Join(pairs, " ")
}

// mustQuery parses a query or fails the test; the queries below are all
// literals this package's own parser must accept.
func mustQuery(t *testing.T, s string) query {
	t.Helper()
	q, err := parseQuery(s)
	if err != nil {
		t.Fatalf("parseQuery(%q): %v", s, err)
	}
	return q
}

// TestScanFixtureMatchesEveryPrefixSpelling pins the tool's central claim: a
// query names a construct, so all three prefix spellings of one XSD element
// answer it and a same-spelled element in another namespace does not. A
// regression to prefix-text matching fails here.
func TestScanFixtureMatchesEveryPrefixSpelling(t *testing.T) {
	q := mustQuery(t, "element@targetNamespace")
	cases := []struct {
		name string
		doc  string
		want int
	}{
		{"unprefixed under a default binding", unprefixedDoc, 1},
		{"xs prefix", xsPrefixDoc, 1},
		{"xsd prefix", xsdPrefixDoc, 1},
		{"xsd prefix over another namespace", impostorDoc, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scan := scanFixture("t.xsd", strings.NewReader(tc.doc), q)
			if scan.Err != nil {
				t.Fatalf("scanFixture: %v", scan.Err)
			}
			if len(scan.Hits) != tc.want {
				t.Fatalf("got %d hit(s), want %d: %+v", len(scan.Hits), tc.want, scan.Hits)
			}
			if tc.want == 0 {
				return
			}
			if got := scan.Hits[0].Attrs[0].Value; got != "urn:b" {
				t.Errorf("targetNamespace = %q, want urn:b", got)
			}
		})
	}
}

// TestScanFixtureDecodesUTF16 pins the second axis: the same construct, in
// the encoding a UTF-8 grep cannot see, matches identically. The
// unprefixed-under-default-xmlns spelling is the one the acceptance fixture
// s3_2_3si10.xsd uses, so this case crosses both axes at once.
func TestScanFixtureDecodesUTF16(t *testing.T) {
	q := mustQuery(t, "element@targetNamespace")
	scan := scanFixture("t.xsd", strings.NewReader(utf16LEDoc(unprefixedDoc)), q)
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	if len(scan.Hits) != 1 {
		t.Fatalf("got %d hit(s), want 1: %+v", len(scan.Hits), scan.Hits)
	}
	if got := scan.Hits[0].Attrs[0].Value; got != "urn:b" {
		t.Errorf("targetNamespace = %q, want urn:b", got)
	}
	if scan.Hits[0].Line != 5 {
		t.Errorf("Line = %d, want 5 (the decoded stream's line, not the byte's)", scan.Hits[0].Line)
	}
}

// TestScanFixtureRecordsTheParent pins the field #1282 added: each hit names
// the element it is a DIRECT child of, resolved and in document order, so a
// local-versus-top-level census reads off the report instead of being
// hand-written (Appendix A's xs:localElement versus xs:topLevelElement).
func TestScanFixtureRecordsTheParent(t *testing.T) {
	doc := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="top" abstract="true"/>
  <xs:complexType name="ct">
    <xs:sequence>
      <xs:element name="local" abstract="false"/>
    </xs:sequence>
  </xs:complexType>
  <xs:element name="after" abstract="true"/>
</xs:schema>`
	scan := scanFixture("t.xsd", strings.NewReader(doc), mustQuery(t, "element@abstract"))
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	want := []xsd.QName{
		{Space: xsd.XMLSchemaNS, Local: "schema"},
		{Space: xsd.XMLSchemaNS, Local: "sequence"},
		// The third pins the pop: without it "after" inherits the depth the
		// nested declaration left behind.
		{Space: xsd.XMLSchemaNS, Local: "schema"},
	}
	if len(scan.Hits) != len(want) {
		t.Fatalf("got %d hit(s), want %d: %+v", len(scan.Hits), len(want), scan.Hits)
	}
	for i, w := range want {
		if scan.Hits[i].Parent != w {
			t.Errorf("Hits[%d] (%s) Parent = %+v, want %+v", i, scan.Hits[i].Attrs[0].Value, scan.Hits[i].Parent, w)
		}
	}
}

// TestScanFixtureParentOfDocumentElement pins the absent case: a match that is
// the document element has no parent, and the zero QName is how that is said.
func TestScanFixtureParentOfDocumentElement(t *testing.T) {
	doc := `<xs:element xmlns:xs="http://www.w3.org/2001/XMLSchema" name="root" abstract="true"/>`
	scan := scanFixture("t.xsd", strings.NewReader(doc), mustQuery(t, "element@abstract"))
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	if len(scan.Hits) != 1 {
		t.Fatalf("got %d hit(s), want 1: %+v", len(scan.Hits), scan.Hits)
	}
	if got := scan.Hits[0].Parent; got != (xsd.QName{}) {
		t.Errorf("Parent = %+v, want the zero QName (no parent)", got)
	}
}

// TestScanFixtureResolvesTheParentInItsOwnScope pins the hazard a prefix-text
// stack would fall into: the parent's name comes from the bindings in force at
// the PARENT, which its child here rebinds to something else entirely.
func TestScanFixtureResolvesTheParentInItsOwnScope(t *testing.T) {
	doc := `<p:schema xmlns:p="http://www.w3.org/2001/XMLSchema">
  <p:element xmlns:p="urn:decoy" name="x"/>
  <q:element xmlns:q="http://www.w3.org/2001/XMLSchema" name="y" abstract="true"/>
</p:schema>`
	scan := scanFixture("t.xsd", strings.NewReader(doc), mustQuery(t, "element@abstract"))
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	if len(scan.Hits) != 1 {
		t.Fatalf("got %d hit(s), want 1: %+v", len(scan.Hits), scan.Hits)
	}
	want := xsd.QName{Space: xsd.XMLSchemaNS, Local: "schema"}
	if got := scan.Hits[0].Parent; got != want {
		t.Errorf("Parent = %+v, want %+v", got, want)
	}
}

// TestReportNamesEachHitsParent pins that the field reaches the reader: the
// default report prints the parent beside file:line:col, and says so for a
// document element too.
func TestReportNamesEachHitsParent(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "nested.xsd", xsPrefixDoc)
	writeFixture(t, root, "bare.xsd",
		`<xs:element xmlns:xs="http://www.w3.org/2001/XMLSchema" name="r" targetNamespace="urn:b"/>`)

	rep, err := census(root, mustQuery(t, "element@targetNamespace"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	var out strings.Builder
	if err := printReport(&out, rep); err != nil {
		t.Fatalf("printReport: %v", err)
	}
	for _, want := range []string{
		"bare.xsd:1:1 parent=(none) children=[] targetNamespace=\"urn:b\"",
		"nested.xsd:5:7 parent={http://www.w3.org/2001/XMLSchema}sequence children=[] targetNamespace=\"urn:b\"",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report does not carry %q:\n%s", want, out.String())
		}
	}
}

// nestDoc carries every case the containment shape turns on: a match whose
// qualifying ancestor is its PARENT, one whose qualifying ancestor is its
// GRANDparent, a match with no qualifying ancestor at all, and an element
// answering both sides of the query at once — the choice, which is an
// ancestor for what stands under it and is not one for itself.
const nestDoc = `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:choice maxOccurs="unbounded">
    <xs:sequence>
      <xs:element name="deep" maxOccurs="20"/>
    </xs:sequence>
    <xs:element name="shallow" maxOccurs="3"/>
  </xs:choice>
  <xs:element name="lone" maxOccurs="7"/>
</xs:schema>`

// TestScanFixtureMatchesOnlyInsideAMatchingAncestor pins the shape #1585
// added, the way [TestScanFixtureRecordsTheParent] pins the parent field: a
// match counts only when an element matching the ancestor pattern encloses
// it, at ANY depth, and the hit names the innermost such element.
//
// The first case is the one the parent field cannot answer — its qualifying
// ancestor is a grandparent, so an implementation reading only
// `open[len(open)-1]` or `hit.Parent` finds nothing there. The two elements
// the census excludes are the other half: `lone` has no qualifying ancestor,
// and `choice` answers both patterns but is not its own ancestor.
func TestScanFixtureMatchesOnlyInsideAMatchingAncestor(t *testing.T) {
	scan := scanFixture("t.xsd", strings.NewReader(nestDoc), mustQuery(t, "*@maxOccurs//*@maxOccurs"))
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	want := []struct {
		value    string
		parent   xsd.QName
		ancestor xsd.QName
	}{
		{"20", xsdName("sequence"), xsdName("choice")},
		{"3", xsdName("choice"), xsdName("choice")},
	}
	if len(scan.Hits) != len(want) {
		t.Fatalf("got %d hit(s), want %d: %+v", len(scan.Hits), len(want), scan.Hits)
	}
	for i, w := range want {
		h := scan.Hits[i]
		if got := attrPairs(h); got != "maxOccurs="+w.value {
			t.Errorf("Hits[%d] matched %q, want maxOccurs=%s", i, got, w.value)
		}
		if h.Parent != w.parent {
			t.Errorf("Hits[%d] (maxOccurs=%s) Parent = %+v, want %+v", i, w.value, h.Parent, w.parent)
		}
		if h.Ancestor != w.ancestor {
			t.Errorf("Hits[%d] (maxOccurs=%s) Ancestor = %+v, want %+v", i, w.value, h.Ancestor, w.ancestor)
		}
	}
}

// TestScanFixtureCountsNoElementAsItsOwnAncestor pins the exclusion on its
// own, against the query that cannot tell the two readings apart any other
// way: `A//A` over a document holding ONE A answers nothing, where `A` answers
// once. An implementation that consulted the whole open chain including the
// element itself reports the same figure for both.
func TestScanFixtureCountsNoElementAsItsOwnAncestor(t *testing.T) {
	doc := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:choice maxOccurs="unbounded"/>
</xs:schema>`
	nested := scanFixture("t.xsd", strings.NewReader(doc), mustQuery(t, "*@maxOccurs//*@maxOccurs"))
	if nested.Err != nil {
		t.Fatalf("scanFixture: %v", nested.Err)
	}
	if len(nested.Hits) != 0 {
		t.Errorf("`A//A` found %d hit(s) over one A, want 0 — an element is not its own ancestor: %+v",
			len(nested.Hits), nested.Hits)
	}
	plain := scanFixture("t.xsd", strings.NewReader(doc), mustQuery(t, "*@maxOccurs"))
	if len(plain.Hits) != 1 {
		t.Fatalf("`A` found %d hit(s), want 1 — the containment test is what excludes it: %+v",
			len(plain.Hits), plain.Hits)
	}
}

// TestScanFixtureAncestorPatternIsReadOnItsOwn pins that the two sides of
// `//` are separate patterns: the ancestor here carries an attribute the
// match does not, and neither side admits the other's element.
func TestScanFixtureAncestorPatternIsReadOnItsOwn(t *testing.T) {
	doc := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="ct" mixed="true">
    <xs:sequence>
      <xs:element name="in" maxOccurs="2"/>
    </xs:sequence>
  </xs:complexType>
  <xs:element name="out" maxOccurs="2"/>
</xs:schema>`
	scan := scanFixture("t.xsd", strings.NewReader(doc), mustQuery(t, "complexType@mixed//element@maxOccurs"))
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	if len(scan.Hits) != 1 {
		t.Fatalf("got %d hit(s), want the one element inside the mixed complexType: %+v", len(scan.Hits), scan.Hits)
	}
	if got := scan.Hits[0].Attrs[0].Value; got != "2" || scan.Hits[0].Line != 4 {
		t.Errorf("hit = line %d maxOccurs=%q, want line 4 maxOccurs=\"2\"", scan.Hits[0].Line, got)
	}
	if got := scan.Hits[0].Ancestor; got != xsdName("complexType") {
		t.Errorf("Ancestor = %+v, want the complexType that matched the ancestor pattern", got)
	}
}

// TestReportNamesTheQualifyingAncestorAndSaysWhatTheFigureIs pins what the
// containment census prints: the `ancestor=` field beside `parent=` — they
// name different elements on the grandparent line, which is why the field
// exists — and the caveat, whose two directions are the whole reason the
// figure may be quoted at all (#1585).
func TestReportNamesTheQualifyingAncestorAndSaysWhatTheFigureIs(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "nest.xsd", nestDoc)

	rep, err := census(root, mustQuery(t, "*@maxOccurs//*@maxOccurs"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	var out strings.Builder
	if err := printReport(&out, rep); err != nil {
		t.Fatalf("printReport: %v", err)
	}
	for _, want := range []string{
		"suiteindex: 2 occurrence(s) of " + ns + "*@maxOccurs//" + ns + "*@maxOccurs in 1 fixture(s) under ",
		`nest.xsd:5:7 element=` + ns + `element parent=` + ns + `sequence ancestor=` + ns +
			`choice children=[] maxOccurs="20"`,
		`nest.xsd:7:5 element=` + ns + `element parent=` + ns + `choice ancestor=` + ns +
			`choice children=[] maxOccurs="3"`,
		// Both directions, because a figure qualified in one of them is a
		// false statement in the other.
		"bound from above of a LEXICAL nesting population, never as a count",
		"it over-counts, because an ancestor in the document is not a",
		"MISSES a nest that only",
		"this census never resolves a ref",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("containment report does not carry %q:\n%s", want, out.String())
		}
	}
}

// TestReportPrintsNoContainmentFieldsWithoutTheShape pins the other half of
// that rendering, and it is the stronger bar: every query shape that existed
// before #1585 renders the report it always did, so neither the caveat nor
// the `ancestor=` field may appear anywhere in one. A field printed
// unconditionally passes every assertion above and changes the bytes of all
// eight (#1585).
func TestReportPrintsNoContainmentFieldsWithoutTheShape(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "a/pfx.xsd", xsPrefixDoc)
	writeFixture(t, root, "b/axis.xsd", axisDoc)
	writeFixture(t, root, "c/bare.xml", noNamespaceDoc)
	writeFixture(t, root, "d/ns.xml", instanceNSDoc)
	writeFixture(t, root, "e/attr.xsd", `<schema xmlns="http://www.w3.org/2001/XMLSchema">
  <attribute name="c" targetNamespace="urn:b" form="qualified"/>
</schema>`)

	// The eight shapes that predate the containment axis, each over a fixture
	// that answers it.
	for _, q := range []string{
		"element@targetNamespace",
		"attribute@targetNamespace,form",
		"*@mixed|abstract",
		"*@name",
		"{http://www.w3.org/2001/XMLSchema}element@name",
		anyNS + "*" + xsiType,
		"{}wrapper@id",
		"*@*",
	} {
		t.Run(q, func(t *testing.T) {
			rep, err := census(root, mustQuery(t, q))
			if err != nil {
				t.Fatalf("census: %v", err)
			}
			if len(rep.Hits) == 0 {
				t.Fatalf("no hit, so this shape's report says nothing to compare")
			}
			var out strings.Builder
			if err := printReport(&out, rep); err != nil {
				t.Fatalf("printReport: %v", err)
			}
			for _, absent := range []string{"ancestor=", "bound from above", "never resolves a ref"} {
				if strings.Contains(out.String(), absent) {
					t.Errorf("report carries %q, which belongs to the containment shape alone:\n%s",
						absent, out.String())
				}
			}
		})
	}
}

// TestReportSpellsANoNamespaceElementForReEntry pins #1297 at every element
// position a match report prints — the summary line, the `element=` field, the
// parent and the children: a name in no namespace carries an explicit `{}`,
// because a braceless element name re-enters the query language as the XML
// Schema namespace and would name a different element.
func TestReportSpellsANoNamespaceElementForReEntry(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "bare.xml", noNamespaceDoc)

	cases := []struct {
		query string
		want  []string
	}{
		{
			query: "{}wrapper@id",
			want: []string{
				"suiteindex: 1 occurrence(s) of {}wrapper@id in 1 fixture(s) under ",
				`bare.xml:3:3 parent={}root children=[{}inner] id="w"`,
			},
		},
		{
			query: "{}*@id",
			want: []string{
				"suiteindex: 1 occurrence(s) of {}*@id in 1 fixture(s) under ",
				`bare.xml:3:3 element={}wrapper parent={}root children=[{}inner] id="w"`,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			rep, err := census(root, mustQuery(t, tc.query))
			if err != nil {
				t.Fatalf("census: %v", err)
			}
			var out strings.Builder
			if err := printReport(&out, rep); err != nil {
				t.Fatalf("printReport: %v", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("report does not carry %q:\n%s", want, out.String())
				}
			}
		})
	}
}

// TestAxisPairSpellsEachHalfForItsOwnPosition pins where #1297's fix stops:
// the element half of a pair takes the `{}`, the attribute half does not,
// since an attribute position of a query already reads a bare name as no
// namespace. Each half re-enters as the query that reported the pair.
func TestAxisPairSpellsEachHalfForItsOwnPosition(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "bare.xml", noNamespaceDoc)

	cases := []struct{ query, want string }{
		{query: "{}*@*", want: "  {}wrapper @id — 1 occurrence(s) in 1 fixture(s)"},
		{query: "{}*@{urn:o}*", want: "  {}wrapper @{urn:o}keep — 1 occurrence(s) in 1 fixture(s)"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			rep, err := census(root, mustQuery(t, tc.query))
			if err != nil {
				t.Fatalf("census: %v", err)
			}
			var out strings.Builder
			if err := printReport(&out, rep); err != nil {
				t.Fatalf("printReport: %v", err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("axis report does not carry %q:\n%s", tc.want, out.String())
			}
		})
	}
}

// xsdName is one XSD-vocabulary name, for the child lists the tests below
// compare against.
func xsdName(local string) xsd.QName {
	return xsd.QName{Space: xsd.XMLSchemaNS, Local: local}
}

// childNames renders one hit's child list for a failure message, since a
// []xsd.QName prints as a wall of struct fields.
func childNames(h hit) string {
	var got []string
	for _, c := range h.Children {
		got = append(got, c.Local)
	}
	return "[" + strings.Join(got, " ") + "]"
}

// checkChildren compares each hit's child list against want, position by
// position, so a missing repeat fails as loudly as a missing name.
func checkChildren(t *testing.T, scan fixtureScan, want [][]xsd.QName) {
	t.Helper()
	if len(scan.Hits) != len(want) {
		t.Fatalf("got %d hit(s), want %d: %+v", len(scan.Hits), len(want), scan.Hits)
	}
	for i, w := range want {
		got := scan.Hits[i].Children
		if len(got) != len(w) {
			t.Errorf("Hits[%d].Children = %s, want %d name(s): %+v", i, childNames(scan.Hits[i]), len(w), w)
			continue
		}
		for j, name := range w {
			if got[j] != name {
				t.Errorf("Hits[%d].Children[%d] = %+v, want %+v", i, j, got[j], name)
			}
		}
	}
}

// TestScanFixtureRecordsDirectChildren pins the field #1304 added: a hit
// carries the elements DIRECTLY under its match, in document order, with
// repeats kept. The two xs:simpleType children are the multiplicity half of a
// content-model census, which a deduplicated list would discard; the
// xs:documentation is a grandchild and must not appear at all.
func TestScanFixtureRecordsDirectChildren(t *testing.T) {
	doc := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:alternative test="a">
    <xs:annotation><xs:documentation/></xs:annotation>
    <xs:simpleType/>
    <xs:simpleType/>
  </xs:alternative>
  <xs:alternative test="b"/>
  <xs:alternative test="c"></xs:alternative>
</xs:schema>`
	scan := scanFixture("t.xsd", strings.NewReader(doc), mustQuery(t, "alternative@test"))
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	checkChildren(t, scan, [][]xsd.QName{
		{xsdName("annotation"), xsdName("simpleType"), xsdName("simpleType")},
		// Self-closed and empty: no element child, and both fully read.
		nil,
		nil,
	})
	for i, h := range scan.Hits {
		if h.ChildrenUnclosed {
			t.Errorf("Hits[%d] (%s) is marked unclosed, but the fixture reads to the end", i, h.Attrs[0].Value)
		}
	}
}

// TestScanFixtureRecordsChildrenOfNestedMatches pins the stack: a query
// matching at several depths gives each match its own list, and an outer
// match collects only what stands directly under IT — the inner match here is
// a great-grandchild and belongs to neither list.
func TestScanFixtureRecordsChildrenOfNestedMatches(t *testing.T) {
	doc := `<xs:element xmlns:xs="http://www.w3.org/2001/XMLSchema" name="outer" abstract="true">
  <xs:complexType>
    <xs:sequence>
      <xs:element name="inner" abstract="false">
        <xs:annotation/>
        <xs:complexType/>
      </xs:element>
    </xs:sequence>
  </xs:complexType>
</xs:element>`
	scan := scanFixture("t.xsd", strings.NewReader(doc), mustQuery(t, "element@abstract"))
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	checkChildren(t, scan, [][]xsd.QName{
		{xsdName("complexType")},
		{xsdName("annotation"), xsdName("complexType")},
	})
}

// TestScanFixtureMarksOnlyTheMatchesLeftOpen pins the distinction the
// file-level [report.Partial] flag cannot carry (#1304): a fixture whose read
// faults still holds matches that closed ahead of the fault, and their child
// lists are complete. The corpus's own instance is
// msData/additional/test79253.xsd, which faults at its tail: `element@type`
// finds 13 matches in it, every one of them self-closed with a known-empty
// child list, and a file-level flag would report all 13 as unread.
func TestScanFixtureMarksOnlyTheMatchesLeftOpen(t *testing.T) {
	doc := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="closed" abstract="true">
    <xs:complexType/>
  </xs:element>
  <xs:element name="cut" abstract="true">
    <xs:annotation/>
    <xs:whitespace>
</xs:schema>`
	scan := scanFixture("broken.xsd", strings.NewReader(doc), mustQuery(t, "element@abstract"))
	if scan.Err == nil {
		t.Fatal("scanFixture: want a read fault, got none")
	}
	checkChildren(t, scan, [][]xsd.QName{
		{xsdName("complexType")},
		{xsdName("annotation"), xsdName("whitespace")},
	})
	if scan.Hits[0].ChildrenUnclosed {
		t.Error("Hits[0] (closed) is marked unclosed, but its end tag was read before the fault")
	}
	if !scan.Hits[1].ChildrenUnclosed {
		t.Error("Hits[1] (cut) is not marked unclosed, so a prefix of its children reads as the whole list")
	}
}

// TestReportNamesEachHitsChildren pins that the list reaches the reader: the
// default report prints it beside the parent, "[]" for a match with no
// element child, and an "(unclosed)" tail for one the read never closed.
func TestReportNamesEachHitsChildren(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "full.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="a" targetNamespace="urn:b">
    <xs:complexType/>
    <xs:complexType/>
  </xs:element>
  <xs:element name="c" targetNamespace="urn:b"/>
</xs:schema>`)
	writeFixture(t, root, "cut.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="d" targetNamespace="urn:b">
    <xs:annotation>
</xs:schema>`)

	rep, err := census(root, mustQuery(t, "element@targetNamespace"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	var out strings.Builder
	if err := printReport(&out, rep); err != nil {
		t.Fatalf("printReport: %v", err)
	}
	for _, want := range []string{
		"cut.xsd:2:3 parent=" + ns + "schema children=[" + ns + "annotation, (unclosed)]",
		"full.xsd:2:3 parent=" + ns + "schema children=[" + ns + "complexType, " + ns + "complexType]",
		"full.xsd:6:3 parent=" + ns + "schema children=[]",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report does not carry %q:\n%s", want, out.String())
		}
	}
}

// TestScanFixtureRequiresEveryQueriedAttribute pins the AND semantics of a
// multi-attribute query, and that an element missing one named attribute is
// not an occurrence.
func TestScanFixtureRequiresEveryQueriedAttribute(t *testing.T) {
	doc := `<schema xmlns="http://www.w3.org/2001/XMLSchema">
  <attribute name="a" targetNamespace="urn:b"/>
  <attribute name="c" targetNamespace="urn:b" form="qualified"/>
</schema>`
	scan := scanFixture("t.xsd", strings.NewReader(doc), mustQuery(t, "attribute@targetNamespace,form"))
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	if len(scan.Hits) != 1 {
		t.Fatalf("got %d hit(s), want 1: %+v", len(scan.Hits), scan.Hits)
	}
	want := "targetNamespace=urn:b form=qualified"
	if got := attrPairs(scan.Hits[0]); got != want {
		t.Errorf("matched attributes = %q, want %q (query order)", got, want)
	}
}

// axisDoc carries every case the wildcard forms turn on: an element with no
// attribute at all, one whose only attributes are in a namespace, two
// carrying one of the queried names each, and one carrying two of them.
const axisDoc = `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:o="urn:o" o:keep="x">
  <xs:complexType name="ct" mixed="true" abstract="false"/>
  <xs:element name="a" nillable="true" o:extra="y"/>
  <xs:sequence/>
</xs:schema>`

// TestScanFixtureMatchesEveryElement pins the wildcard element: a census over
// an attribute axis is anchored to no element name, which the query language
// had no spelling for at all before #1391. The impostor namespace still does
// not answer — `*` widens the local name, never the namespace.
func TestScanFixtureMatchesEveryElement(t *testing.T) {
	scan := scanFixture("t.xsd", strings.NewReader(axisDoc), mustQuery(t, "*@name"))
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	want := []xsd.QName{xsdName("complexType"), xsdName("element")}
	if len(scan.Hits) != len(want) {
		t.Fatalf("got %d hit(s), want %d: %+v", len(scan.Hits), len(want), scan.Hits)
	}
	for i, w := range want {
		if scan.Hits[i].Element != w {
			t.Errorf("Hits[%d].Element = %+v, want %+v", i, scan.Hits[i].Element, w)
		}
	}
	if impostor := scanFixture("i.xsd", strings.NewReader(impostorDoc), mustQuery(t, "*@name")); len(impostor.Hits) != 0 {
		t.Errorf("got %d hit(s) in another namespace, want 0: %+v", len(impostor.Hits), impostor.Hits)
	}
}

// TestScanFixtureAnyOfTheQueriedAttributes pins the "|" join against the ","
// join above: an element carrying ANY one of the names is an occurrence, and
// the hit records the names it actually carried rather than the query's.
// #456's population is a disjunction over six names, and the "," spelling of
// it is their intersection — a different census, and here an empty one.
func TestScanFixtureAnyOfTheQueriedAttributes(t *testing.T) {
	scan := scanFixture("t.xsd", strings.NewReader(axisDoc), mustQuery(t, "*@mixed|abstract|nillable"))
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	want := []string{"mixed=true abstract=false", "nillable=true"}
	if len(scan.Hits) != len(want) {
		t.Fatalf("got %d hit(s), want %d: %+v", len(scan.Hits), len(want), scan.Hits)
	}
	for i, w := range want {
		if got := attrPairs(scan.Hits[i]); got != w {
			t.Errorf("Hits[%d] matched %q, want %q", i, got, w)
		}
	}
	and := scanFixture("t.xsd", strings.NewReader(axisDoc), mustQuery(t, "*@mixed,abstract,nillable"))
	if len(and.Hits) != 0 {
		t.Errorf("the \",\" spelling found %d hit(s), want 0 — it is the intersection: %+v", len(and.Hits), and.Hits)
	}
}

// TestScanFixtureCensusesEveryAttributeName pins `@*`: every attribute in the
// wildcard's namespace, in the order the tag spells them. A namespace
// declaration is not an attribute and never appears; an element carrying no
// attribute in that namespace is not an occurrence at all.
func TestScanFixtureCensusesEveryAttributeName(t *testing.T) {
	cases := []struct {
		query string
		want  []string
	}{
		{query: "*@*", want: []string{"name=ct mixed=true abstract=false", "name=a nillable=true"}},
		{query: "*@{urn:o}*", want: []string{"keep=x", "extra=y"}},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			scan := scanFixture("t.xsd", strings.NewReader(axisDoc), mustQuery(t, tc.query))
			if scan.Err != nil {
				t.Fatalf("scanFixture: %v", scan.Err)
			}
			if len(scan.Hits) != len(tc.want) {
				t.Fatalf("got %d hit(s), want %d: %+v", len(scan.Hits), len(tc.want), scan.Hits)
			}
			for i, w := range tc.want {
				if got := attrPairs(scan.Hits[i]); got != w {
					t.Errorf("Hits[%d] matched %q, want %q", i, got, w)
				}
			}
		})
	}
}

// TestReportGroupsTheNameAxis pins the shape `@*` reports in, which is the
// one thing #1391 changed about the output: the pairs with their counts, then
// each pair's fixtures, and no per-occurrence section at all. One line per
// occurrence is what the corpus makes unreadable at 172,648 of them.
func TestReportGroupsTheNameAxis(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "a/one.xsd", xsPrefixDoc)
	writeFixture(t, root, "b/two.xsd", utf16LEDoc(unprefixedDoc))
	// Two occurrences of one pair in one fixture: the pair's fixture list
	// names it once and its occurrence count still counts both.
	writeFixture(t, root, "c/three.xsd", `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="p"/>
  <xs:element name="q"/>
</xs:schema>`)

	rep, err := census(root, mustQuery(t, "*@*"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	var out strings.Builder
	if err := printReport(&out, rep); err != nil {
		t.Fatalf("printReport: %v", err)
	}
	for _, want := range []string{
		"=== Attribute-name axis: 3 (element, attribute) pair(s), 8 attribute occurrence(s) ===",
		"  " + ns + "complexType @name — 2 occurrence(s) in 2 fixture(s)",
		"  " + ns + "element @name — 4 occurrence(s) in 3 fixture(s)",
		"  " + ns + "element @targetNamespace — 2 occurrence(s) in 2 fixture(s)",
		"=== Fixtures per pair (path order) ===",
		"  " + ns + "element @name\n      a/one.xsd\n      b/two.xsd\n      c/three.xsd\n",
		"  " + ns + "element @targetNamespace\n      a/one.xsd\n      b/two.xsd\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("axis report does not carry %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "=== Matches") {
		t.Errorf("the axis report also printed the per-occurrence section:\n%s", out.String())
	}
}

// TestReportNamesTheMatchedElementOnlyWhenTheQueryLeavesItOpen pins the one
// thing the match line gained: a query that leaves the element open says
// which element each hit was, and one that fixed it does not repeat itself.
// A union query leaves it open however tightly each alternative is written —
// both names below fix both axes, and the hit's own is still not derivable
// from the query (#1554).
func TestReportNamesTheMatchedElementOnlyWhenTheQueryLeavesItOpen(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "one.xsd", xsPrefixDoc)

	cases := []struct {
		query string
		want  bool
	}{
		{query: "*@targetNamespace", want: true},
		{query: "element@targetNamespace", want: false},
		{query: "element|complexType@targetNamespace", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			rep, err := census(root, mustQuery(t, tc.query))
			if err != nil {
				t.Fatalf("census: %v", err)
			}
			var out strings.Builder
			if err := printReport(&out, rep); err != nil {
				t.Fatalf("printReport: %v", err)
			}
			if got := strings.Contains(out.String(), "element="+ns+"element"); got != tc.want {
				t.Errorf("report names the matched element = %v, want %v:\n%s", got, tc.want, out.String())
			}
		})
	}
}

// TestReportUnionCensusIsAttributablePerElement pins what makes a union
// census readable: its hits are one run of consecutive lines in the report's
// path-then-document order, and the `element=` field is the only record of
// which alternative each line answers. Without it the two halves of a feature
// are counted together and can never be told apart again (#1554).
func TestReportUnionCensusIsAttributablePerElement(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "one.xsd", xsPrefixDoc)

	rep, err := census(root, mustQuery(t, "element|complexType@name"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	if len(rep.Hits) != 2 {
		t.Fatalf("Hits = %+v, want the one element and the one complexType", rep.Hits)
	}
	var out strings.Builder
	if err := printReport(&out, rep); err != nil {
		t.Fatalf("printReport: %v", err)
	}
	summary := "2 occurrence(s) of " + ns + "element|" + ns + "complexType@name in 1 fixture(s)"
	if !strings.Contains(out.String(), summary) {
		t.Errorf("union report does not carry %q:\n%s", summary, out.String())
	}
	// The two hits as whole lines, consecutive, the complexType's position
	// ahead of the element's.
	run := []string{
		`  one.xsd:3:3 element=` + ns + `complexType parent=` + ns + `schema children=[` + ns + `sequence] name="ct"`,
		`  one.xsd:5:7 element=` + ns + `element parent=` + ns + `sequence children=[] name="a"`,
	}
	lines := strings.Split(out.String(), "\n")
	first := slices.Index(lines, run[0])
	if first < 0 || first+1 >= len(lines) || lines[first+1] != run[1] {
		t.Errorf("union report does not carry the run %q:\n%s", run, out.String())
	}
}

// TestReportCensusesEveryNamespaceInTheElementPosition pins the four readings
// of one instance-side census over a tree holding the same construct in a
// target namespace and in none (#1495). Three of them are namespace-RESTRICTED
// and each answers for its own slice — the braceless one for a namespace an
// instance document never uses, which is the reading that printed
// `0 occurrence(s)` as though it were a measurement. The fourth is the axis,
// and it is the only shape whose count is the population's.
//
// Every case asserts the echoed header too: a restricted census names its
// restriction in Clark notation, and the axis names itself as the axis, so the
// two cannot be read for each other off a report line.
func TestReportCensusesEveryNamespaceInTheElementPosition(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "bare.xml", instanceBareDoc)
	writeFixture(t, root, "ns.xml", instanceNSDoc)

	bareHit := `bare.xml:3:3 element={}shipTo parent={}order children=[] type="USAddress"`
	nsHit := `ns.xml:3:3 element={urn:target}shipTo parent={urn:target}order children=[] type="t:USAddress"`
	cases := []struct {
		name   string
		query  string
		want   []string
		absent []string
	}{
		{
			name:   "braceless, so the XML Schema namespace an instance document never carries",
			query:  "*" + xsiType,
			want:   []string{"suiteindex: 0 occurrence(s) of " + ns + "*" + xsiType + " in 0 fixture(s) under "},
			absent: []string{"bare.xml", "ns.xml"},
		},
		{
			name:   "no namespace, a slice of the population and not the whole of it",
			query:  "{}*" + xsiType,
			want:   []string{"suiteindex: 1 occurrence(s) of {}*" + xsiType + " in 1 fixture(s) under ", bareHit},
			absent: []string{"ns.xml"},
		},
		{
			name:   "one named namespace, the other slice",
			query:  "{urn:target}*" + xsiType,
			want:   []string{"suiteindex: 1 occurrence(s) of {urn:target}*" + xsiType + " in 1 fixture(s) under ", nsHit},
			absent: []string{"bare.xml"},
		},
		{
			name:  "the namespace axis, which is the only shape that finds both",
			query: anyNS + "*" + xsiType,
			want: []string{
				"suiteindex: 2 occurrence(s) of " + anyNS + "*" + xsiType + " in 2 fixture(s) under ",
				bareHit,
				nsHit,
			},
		},
		{
			// The axis with the local name FIXED: nothing about the query is a
			// wildcard local part, so a report that keyed its `element=` field
			// on that alone would print these two lines identically (#1495).
			name:  "the namespace axis under a fixed local name",
			query: anyNS + "shipTo" + xsiType,
			want: []string{
				"suiteindex: 2 occurrence(s) of " + anyNS + "shipTo" + xsiType + " in 2 fixture(s) under ",
				bareHit,
				nsHit,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := census(root, mustQuery(t, tc.query))
			if err != nil {
				t.Fatalf("census: %v", err)
			}
			var out strings.Builder
			if err := printReport(&out, rep); err != nil {
				t.Fatalf("printReport: %v", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("report does not carry %q:\n%s", want, out.String())
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(out.String(), absent) {
					t.Errorf("report names %q, which this query's namespace excludes:\n%s", absent, out.String())
				}
			}
		})
	}
}

// TestScanFixtureRecordsTheResolvedAttributeName pins that a hit's attribute
// name is the document's and never the pattern that admitted it: the namespace
// axis is available in the attribute position too, and a pattern open there
// names no namespace for the hit to borrow.
func TestScanFixtureRecordsTheResolvedAttributeName(t *testing.T) {
	scan := scanFixture("ns.xml", strings.NewReader(instanceNSDoc), mustQuery(t, anyNS+"shipTo@"+anyNS+"type"))
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	if len(scan.Hits) != 1 {
		t.Fatalf("got %d hit(s), want 1: %+v", len(scan.Hits), scan.Hits)
	}
	want := xsd.QName{Space: xsiNS, Local: "type"}
	if got := scan.Hits[0].Attrs[0].Name; got != want {
		t.Errorf("Attrs[0].Name = %+v, want %+v (the name the document resolved)", got, want)
	}
}

// TestScanFixtureKeepsHitsAheadOfAFault pins the malformed-fixture contract:
// the suite ships documents that are not well-formed on purpose, and the
// constructs before the fault are evidence, not collateral.
func TestScanFixtureKeepsHitsAheadOfAFault(t *testing.T) {
	doc := `<schema xmlns="http://www.w3.org/2001/XMLSchema">
  <element name="a" targetNamespace="urn:b"/>
  </notopen>`
	scan := scanFixture("broken.xsd", strings.NewReader(doc), mustQuery(t, "element@targetNamespace"))
	if scan.Err == nil {
		t.Fatal("scanFixture: want a read fault, got none")
	}
	if len(scan.Hits) != 1 {
		t.Fatalf("got %d hit(s) ahead of the fault, want 1: %+v", len(scan.Hits), scan.Hits)
	}
	if scan.Elems == 0 {
		t.Error("Elems = 0, so the caller would count a broken fixture as a non-XML file")
	}
}

// writeFixture writes one fixture into a census tree, creating its
// directories.
func writeFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// TestCensusWalksTheWholeTree pins the scope half of the defect: one query
// over a tree returns fixtures from every directory in it, in path order,
// whatever each one's encoding or prefix. Scoping a census to the directory
// the last finding named is what missed target002.n.xsd (#1239).
func TestCensusWalksTheWholeTree(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "saxonData/TargetNS/plain.xsd", xsPrefixDoc)
	writeFixture(t, root, "ibmData/S3_2_3/wide.xsd", utf16LEDoc(unprefixedDoc))
	writeFixture(t, root, "ibmData/S3_2_3/prefixed.xsd", xsdPrefixDoc)
	writeFixture(t, root, "msData/impostor.xsd", impostorDoc)
	writeFixture(t, root, "nistMeta/notes.txt", "no markup here at all")
	writeFixture(t, root, "msData/not-wf.xsd", `<schema xmlns="http://www.w3.org/2001/XMLSchema">`)

	rep, err := census(root, mustQuery(t, "element@targetNamespace"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	want := []string{
		"ibmData/S3_2_3/prefixed.xsd",
		"ibmData/S3_2_3/wide.xsd",
		"saxonData/TargetNS/plain.xsd",
	}
	var got []string
	for _, h := range rep.Hits {
		got = append(got, h.File)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("hits =\n%s\nwant (path order)\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if rep.Walked != 6 {
		t.Errorf("Walked = %d, want 6", rep.Walked)
	}
	if len(rep.NoElement) != 1 || rep.NoElement[0].File != "nistMeta/notes.txt" {
		t.Errorf("NoElement = %+v, want the one file with no markup", rep.NoElement)
	}
	if len(rep.Partial) != 1 || rep.Partial[0].File != "msData/not-wf.xsd" {
		t.Errorf("Partial = %+v, want the one unclosed fixture", rep.Partial)
	}
}

// TestReportAccountsForEveryWalkedFile pins the census's arithmetic: every
// file walked is read to the end, listed as partly read, or listed as holding
// no element — never counted away.
func TestReportAccountsForEveryWalkedFile(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "good.xsd", xsPrefixDoc)
	writeFixture(t, root, "prose.txt", "no markup here at all")
	writeFixture(t, root, "broken.xsd", `<schema xmlns="http://www.w3.org/2001/XMLSchema">`)

	rep, err := census(root, mustQuery(t, "element@targetNamespace"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	var out strings.Builder
	if err := printReport(&out, rep); err != nil {
		t.Fatalf("printReport: %v", err)
	}
	for _, want := range []string{"prose.txt", "broken.xsd", "3 file(s): 1 read to the end"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report does not name %q:\n%s", want, out.String())
		}
	}
}

// TestCensusIsDeterministic pins STYLE D1: two runs over one tree render
// byte-identical reports.
func TestCensusIsDeterministic(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "b/two.xsd", utf16LEDoc(unprefixedDoc))
	writeFixture(t, root, "a/one.xsd", xsPrefixDoc)
	writeFixture(t, root, "c/three.xsd", xsdPrefixDoc)

	var runs []string
	for range 2 {
		rep, err := census(root, mustQuery(t, "element@targetNamespace"))
		if err != nil {
			t.Fatalf("census: %v", err)
		}
		var out strings.Builder
		if err := printReport(&out, rep); err != nil {
			t.Fatalf("printReport: %v", err)
		}
		runs = append(runs, out.String())
	}
	if runs[0] != runs[1] {
		t.Errorf("two runs differ:\n%s\n---\n%s", runs[0], runs[1])
	}
	if !strings.Contains(runs[0], "a/one.xsd") {
		t.Errorf("report names no fixture:\n%s", runs[0])
	}
}

// TestPathsModeSeparatesTheListFromTheReport pins the machine half of the
// paths-only mode: stdout carries the matched fixtures alone, one per line and
// deduplicated, so a tool downstream reads it without parsing a report (#1642);
// everything a human needs — the counts, and the files the census could not
// read whole — goes to stderr, where a pipeline leaves it in front of them.
func TestPathsModeSeparatesTheListFromTheReport(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "b/two.xsd", utf16LEDoc(unprefixedDoc))
	writeFixture(t, root, "a/one.xsd", xsPrefixDoc)
	writeFixture(t, root, "a/none.txt", "not xml at all")

	var files, notes strings.Builder
	if err := run(&files, &notes, []string{"-paths", "element@targetNamespace", root}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if want := "a/one.xsd\nb/two.xsd\n"; files.String() != want {
		t.Errorf("stdout = %q, want exactly %q", files.String(), want)
	}
	for _, want := range []string{"2 occurrence(s)", "2 fixture(s)", "read only partly", "with no XML element"} {
		if !strings.Contains(notes.String(), want) {
			t.Errorf("stderr does not carry %q:\n%s", want, notes.String())
		}
	}
}

// TestPathsModeNamesOneFixtureOnce holds the list to one line per fixture
// however many times the query matched inside it: a join downstream counts
// cases per path, and a path repeated is a case counted twice.
func TestPathsModeNamesOneFixtureOnce(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "a/two-matches.xsd", `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="a" targetNamespace="urn:b"/>
  <xs:element name="c" targetNamespace="urn:d"/>
</xs:schema>`)

	var files, notes strings.Builder
	if err := run(&files, &notes, []string{"-paths", "element@targetNamespace", root}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "a/two-matches.xsd\n"; files.String() != want {
		t.Errorf("stdout = %q, want exactly %q", files.String(), want)
	}
	if !strings.Contains(notes.String(), "2 occurrence(s)") {
		t.Errorf("stderr does not report the occurrences behind the one path:\n%s", notes.String())
	}
}

// TestPathsModeCarriesTheContainmentCaveat keeps the hazard with the figure in
// this mode too: the caveat belongs where the reader holding the count arrives
// (#1279), which in a pipeline is stderr.
func TestPathsModeCarriesTheContainmentCaveat(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "a/one.xsd", xsPrefixDoc)

	var files, notes strings.Builder
	if err := run(&files, &notes, []string{"-paths", "*@targetNamespace//*@name", root}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(notes.String(), "never resolves a ref") {
		t.Errorf("stderr drops the containment caveat:\n%s", notes.String())
	}
	if strings.Contains(files.String(), "bound from above") {
		t.Errorf("the caveat reached the machine-readable list:\n%s", files.String())
	}
}

// TestPathsModeReportsAnAbsentCorpus holds the paths mode to the same
// supported degraded mode the report has (#659): the notice goes to stderr,
// stdout stays empty rather than naming a fixture that is not there, and the
// exit is clean.
func TestPathsModeReportsAnAbsentCorpus(t *testing.T) {
	var files, notes strings.Builder
	absent := filepath.Join(t.TempDir(), "absent")
	if err := run(&files, &notes, []string{"-paths", "element@targetNamespace", absent}); err != nil {
		t.Fatalf("run: %v, want a clean exit", err)
	}
	if files.String() != "" {
		t.Errorf("stdout = %q, want nothing at all", files.String())
	}
	if !strings.Contains(notes.String(), "git submodule update --init") {
		t.Errorf("stderr does not name the command that initializes the suite:\n%s", notes.String())
	}
}

// TestRunCorpusAbsent pins the supported degraded mode: a fresh container has
// no suite submodule (#659), so the tool says the corpus is not there and
// exits 0 rather than failing. Every query form is driven through the guard,
// the axis forms included: the mode is a property of the root and must not
// depend on which report the query would have printed.
func TestRunCorpusAbsent(t *testing.T) {
	queries := []string{"element@targetNamespace", "*@*", "*@mixed|abstract"}
	cases := []struct {
		name string
		root func(t *testing.T) string
		want string
	}{
		{
			name: "missing directory",
			root: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent") },
			want: "does not exist",
		},
		{
			name: "empty directory",
			root: func(t *testing.T) string { return t.TempDir() },
			want: "holds no files",
		},
	}
	for _, tc := range cases {
		for _, q := range queries {
			t.Run(tc.name+" "+q, func(t *testing.T) {
				var out strings.Builder
				if err := run(&out, &out, []string{q, tc.root(t)}); err != nil {
					t.Fatalf("run: %v, want a clean exit", err)
				}
				if !strings.Contains(out.String(), tc.want) {
					t.Errorf("output does not say %q:\n%s", tc.want, out.String())
				}
				if !strings.Contains(out.String(), "git submodule update --init") {
					t.Errorf("output does not name the command that initializes the suite:\n%s", out.String())
				}
			})
		}
	}
}

// TestParseQuery pins the query grammar, including the two defaults that make
// a braceless query mean what a reader expects: an element name is in the XML
// Schema namespace, an attribute name is in none. The wildcard and the two
// joins are pinned here too — the axis forms (#1391) turn on both, and the
// element position takes the ANY join so a feature spelled by two elements is
// one census (#1554).
func TestParseQuery(t *testing.T) {
	cases := []struct {
		in    string
		elem  []namePat
		attrs []namePat
		join  attrJoin
	}{
		{
			in:   "element",
			elem: []namePat{{Space: xsd.XMLSchemaNS, Local: "element"}},
			join: joinAll,
		},
		{
			in:    "attribute@targetNamespace,form",
			elem:  []namePat{{Space: xsd.XMLSchemaNS, Local: "attribute"}},
			attrs: []namePat{{Local: "targetNamespace"}, {Local: "form"}},
			join:  joinAll,
		},
		{
			in:   "{urn:x}thing",
			elem: []namePat{{Space: "urn:x", Local: "thing"}},
			join: joinAll,
		},
		{
			// A namespace may hold the separators; the brace ends the URI.
			in:    "{urn:a@b,c|d}thing@{urn:d@e}attr",
			elem:  []namePat{{Space: "urn:a@b,c|d", Local: "thing"}},
			attrs: []namePat{{Space: "urn:d@e", Local: "attr"}},
			join:  joinAll,
		},
		{
			in:    "{}bare@x",
			elem:  []namePat{{Local: "bare"}},
			attrs: []namePat{{Local: "x"}},
			join:  joinAll,
		},
		{
			// The feature axis: the two elements that spell `{open content}`,
			// each an alternative, in the order the query wrote them (#1554).
			in: "openContent|defaultOpenContent",
			elem: []namePat{
				{Space: xsd.XMLSchemaNS, Local: "openContent"},
				{Space: xsd.XMLSchemaNS, Local: "defaultOpenContent"},
			},
			join: joinAll,
		},
		{
			// An element alternative carries its own namespace and its own
			// wildcards, and the attribute list reads the same beside it.
			in: "{urn:x}thing|{*}*|other@name",
			elem: []namePat{
				{Space: "urn:x", Local: "thing"},
				{Space: wildcard, Local: wildcard},
				{Space: xsd.XMLSchemaNS, Local: "other"},
			},
			attrs: []namePat{{Local: "name"}},
			join:  joinAll,
		},
		{
			// The name axis: every element in the XSD namespace, every
			// attribute in none.
			in:    "*@*",
			elem:  []namePat{{Space: xsd.XMLSchemaNS, Local: wildcard}},
			attrs: []namePat{{Local: wildcard}},
			join:  joinAll,
		},
		{
			// The value axis: any of six names, on any element.
			in:    "*@mixed|abstract",
			elem:  []namePat{{Space: xsd.XMLSchemaNS, Local: wildcard}},
			attrs: []namePat{{Local: "mixed"}, {Local: "abstract"}},
			join:  joinAny,
		},
		{
			in:    "{urn:x}*@{urn:y}*",
			elem:  []namePat{{Space: "urn:x", Local: wildcard}},
			attrs: []namePat{{Space: "urn:y", Local: wildcard}},
			join:  joinAll,
		},
		{
			// The namespace axis, in either position and on either axis of
			// the name: `{*}` is a wildcard and never the URI "*" (#1495).
			in:   "{*}thing",
			elem: []namePat{{Space: wildcard, Local: "thing"}},
			join: joinAll,
		},
		{
			in:    "{*}*@{*}type",
			elem:  []namePat{{Space: wildcard, Local: wildcard}},
			attrs: []namePat{{Space: wildcard, Local: "type"}},
			join:  joinAll,
		},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			q, err := parseQuery(tc.in)
			if err != nil {
				t.Fatalf("parseQuery: %v", err)
			}
			if !slices.Equal(q.Element, tc.elem) {
				t.Errorf("Element = %+v, want %+v", q.Element, tc.elem)
			}
			if q.Join != tc.join {
				t.Errorf("Join = %q, want %q", q.Join, tc.join)
			}
			if len(q.Attrs) != len(tc.attrs) {
				t.Fatalf("Attrs = %+v, want %+v", q.Attrs, tc.attrs)
			}
			for i, want := range tc.attrs {
				if q.Attrs[i] != want {
					t.Errorf("Attrs[%d] = %+v, want %+v", i, q.Attrs[i], want)
				}
			}
		})
	}
}

// TestParseQueryRejects pins the malformed queries that must exit 2 rather
// than censusing something the caller did not ask for. The four
// attribute-list rows are the axis grammar's own rejections: one query means
// one join, and `@*` already names every attribute, so a name beside it says
// nothing under either join. The element-position rows close the `|` join's
// own edges — a join with no name behind it, and the `,` whose refusal
// [TestParseQueryRefusesTheAllJoinOnElementNames] pins by its message.
func TestParseQueryRejects(t *testing.T) {
	for _, in := range []string{
		"", "{urn:x", "@attr", "element@", "element@a@b", "element,a", "{urn:x}a}b",
		"element@a,b|c", "element@a|b,c", "element@*,name", "element@name|*",
		"element|", "element|@name", "openContent|defaultOpenContent,form",
	} {
		t.Run(in, func(t *testing.T) {
			if _, err := parseQuery(in); err == nil {
				t.Errorf("parseQuery(%q) = nil error, want a rejection", in)
			}
		})
	}
}

// TestParseQueryRefusesTheAllJoinOnElementNames pins WHY `,` between element
// names is refused and not merely THAT it is: the join means every name at
// once, which no single element carries. The message it replaces reported the
// query as a missing `@` and sent the reader to an attribute list they had
// not written (#1554); this one names `@` only in a trailing clause, for the
// reader whose `,` was a forgotten `@`. The whole message is pinned, both
// joins in their own halves of it, because swapping the two inside it changes
// no branch and leaves every shorter substring in place (#1048).
func TestParseQueryRefusesTheAllJoinOnElementNames(t *testing.T) {
	_, err := parseQuery("openContent,defaultOpenContent")
	if err == nil {
		t.Fatalf("parseQuery = nil error, want a refusal")
	}
	want := `query "openContent,defaultOpenContent": element names are joined by "|" (any one of them), never by ",": no element carries two names at once, and attribute names follow "@"`
	if err.Error() != want {
		t.Errorf("refusal = %q, want %q", err, want)
	}
}

// TestQueryString pins the canonical echo: the report names the namespace
// that was matched, never the prefix a fixture spelled it with, and echoes
// the join it was given so the two readings are never confused in a report
// header.
func TestQueryString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"attribute@targetNamespace,form", "{http://www.w3.org/2001/XMLSchema}attribute@targetNamespace,form"},
		{"*@*", "{http://www.w3.org/2001/XMLSchema}*@*"},
		{"*@mixed|abstract", "{http://www.w3.org/2001/XMLSchema}*@mixed|abstract"},
		// A union over element names echoes every alternative, in the order
		// it was written: the echo of a feature census is the census (#1554).
		{"openContent|defaultOpenContent", ns + "openContent|" + ns + "defaultOpenContent"},
		// An element name in no namespace keeps the wrapper it was written
		// with: bare, it would echo as a name in the XML Schema namespace
		// (#1297). The attribute half is bare because that position reads a
		// bare name as no namespace already.
		{"{}bare@x", "{}bare@x"},
		// The namespace axis echoes AS the axis, so a reader cannot take the
		// corpus-wide count for a namespace-restricted one (#1495).
		{"{*}*" + xsiType, "{*}*" + xsiType},
		{"{*}shipTo@{*}type", "{*}shipTo@{*}type"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := mustQuery(t, tc.in).String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestQueryStringReEntersAsItself pins what the echo is FOR: the canonical
// form a report prints parses back to the query that produced it, so a reader
// re-runs a census from the line naming it. The no-namespace element name is
// the case that fails without an explicit `{}` — it re-enters as a name in
// the XML Schema namespace, which is a different census (#1297).
func TestQueryStringReEntersAsItself(t *testing.T) {
	for _, in := range []string{
		"element", "{}bare", "{}bare@x", "{urn:x}thing@{urn:y}attr",
		"{}*@*", "*@mixed|abstract", "{}*@{urn:y}*",
		"{*}thing", "{*}*@{urn:y}*", "{*}shipTo@{*}type",
		// A union over element names re-enters as the union, both joins in
		// one query included: the element position's `|` and the attribute
		// list's are read apart (#1554).
		"openContent|defaultOpenContent", "{}bare|{*}thing|{urn:x}*",
		"openContent|defaultOpenContent@mode|appliesToEmpty",
	} {
		t.Run(in, func(t *testing.T) {
			q := mustQuery(t, in)
			back, err := parseQuery(q.String())
			if err != nil {
				t.Fatalf("parseQuery(%q): %v", q.String(), err)
			}
			if !slices.Equal(back.Element, q.Element) {
				t.Errorf("%q re-entered as element %+v, want %+v", q.String(), back.Element, q.Element)
			}
			if back.Join != q.Join {
				t.Errorf("%q re-entered with join %q, want %q", q.String(), back.Join, q.Join)
			}
			if len(back.Attrs) != len(q.Attrs) {
				t.Fatalf("%q re-entered with attrs %+v, want %+v", q.String(), back.Attrs, q.Attrs)
			}
			for i, want := range q.Attrs {
				if back.Attrs[i] != want {
					t.Errorf("%q re-entered with Attrs[%d] = %+v, want %+v", q.String(), i, back.Attrs[i], want)
				}
			}
		})
	}
}

// TestParseQueryAncestorPattern pins the containment grammar: the pattern
// ahead of `//` parses exactly as the one behind it, both sides keep their
// own attribute list and join, and a query without the separator carries no
// ancestor at all. The Clark case is the one the separator's spelling turns
// on — the XML Schema namespace contains `//`, and it is inside the wrapper,
// which [splitName] consumes before it looks for a separator.
func TestParseQueryAncestorPattern(t *testing.T) {
	cases := []struct {
		in       string
		ancestor *pattern
		elem     []namePat
		attrs    []namePat
		join     attrJoin
	}{
		{
			in: "*@maxOccurs//*@maxOccurs",
			ancestor: &pattern{
				Element: []namePat{{Space: xsd.XMLSchemaNS, Local: wildcard}},
				Attrs:   []namePat{{Local: "maxOccurs"}},
				Join:    joinAll,
			},
			elem:  []namePat{{Space: xsd.XMLSchemaNS, Local: wildcard}},
			attrs: []namePat{{Local: "maxOccurs"}},
			join:  joinAll,
		},
		{
			// Each side reads its own join, and the element alternatives of
			// one say nothing about the other's.
			in: "choice|sequence@maxOccurs|minOccurs//element@name,type",
			ancestor: &pattern{
				Element: []namePat{
					{Space: xsd.XMLSchemaNS, Local: "choice"},
					{Space: xsd.XMLSchemaNS, Local: "sequence"},
				},
				Attrs: []namePat{{Local: "maxOccurs"}, {Local: "minOccurs"}},
				Join:  joinAny,
			},
			elem:  []namePat{{Space: xsd.XMLSchemaNS, Local: "element"}},
			attrs: []namePat{{Local: "name"}, {Local: "type"}},
			join:  joinAll,
		},
		{
			in: "{" + xsd.XMLSchemaNS + "}complexType//{*}*@*",
			ancestor: &pattern{
				Element: []namePat{{Space: xsd.XMLSchemaNS, Local: "complexType"}},
				Join:    joinAll,
			},
			elem:  []namePat{{Space: wildcard, Local: wildcard}},
			attrs: []namePat{{Local: wildcard}},
			join:  joinAll,
		},
		{
			// No separator, no ancestor: the shape every query before #1585
			// had, and the nil is what keeps its report unchanged.
			in:   "element@name",
			elem: []namePat{{Space: xsd.XMLSchemaNS, Local: "element"}},

			attrs: []namePat{{Local: "name"}},
			join:  joinAll,
		},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			q, err := parseQuery(tc.in)
			if err != nil {
				t.Fatalf("parseQuery: %v", err)
			}
			if !slices.Equal(q.Element, tc.elem) {
				t.Errorf("Element = %+v, want %+v", q.Element, tc.elem)
			}
			if !slices.Equal(q.Attrs, tc.attrs) {
				t.Errorf("Attrs = %+v, want %+v", q.Attrs, tc.attrs)
			}
			if q.Join != tc.join {
				t.Errorf("Join = %q, want %q", q.Join, tc.join)
			}
			if tc.ancestor == nil {
				if q.Ancestor != nil {
					t.Fatalf("Ancestor = %+v, want none", q.Ancestor)
				}
				return
			}
			if q.Ancestor == nil {
				t.Fatalf("Ancestor = none, want %+v", tc.ancestor)
			}
			if !slices.Equal(q.Ancestor.Element, tc.ancestor.Element) {
				t.Errorf("Ancestor.Element = %+v, want %+v", q.Ancestor.Element, tc.ancestor.Element)
			}
			if !slices.Equal(q.Ancestor.Attrs, tc.ancestor.Attrs) {
				t.Errorf("Ancestor.Attrs = %+v, want %+v", q.Ancestor.Attrs, tc.ancestor.Attrs)
			}
			if q.Ancestor.Join != tc.ancestor.Join {
				t.Errorf("Ancestor.Join = %q, want %q", q.Ancestor.Join, tc.ancestor.Join)
			}
		})
	}
}

// TestParseQueryRejectsAMalformedAncestor pins the containment separator's
// own rejections: a side with no name on it, a lone `/`, and the chain.
func TestParseQueryRejectsAMalformedAncestor(t *testing.T) {
	for _, in := range []string{
		"//", "//element", "element//", "element/element", "element//element//element",
		"element//element,other", "element//element@a,b|c", "element@//element",
	} {
		t.Run(in, func(t *testing.T) {
			if _, err := parseQuery(in); err == nil {
				t.Errorf("parseQuery(%q) = nil error, want a rejection", in)
			}
		})
	}
}

// TestParseQueryRefusesASecondAncestor pins WHY the chain is refused and not
// merely THAT it is: one query names one ancestor, so the reader is told what
// to write instead of being sent to the element position's error. The whole
// message is pinned, because the remainder and the separator swapped inside
// it change no branch and leave every shorter substring in place (#1048).
func TestParseQueryRefusesASecondAncestor(t *testing.T) {
	_, err := parseQuery("choice//sequence//element")
	if err == nil {
		t.Fatalf("parseQuery = nil error, want a refusal")
	}
	want := `query "choice//sequence//element": "//" separates one ancestor from the construct` +
		` it encloses and appears at most once, found a second one at "//element"`
	if err.Error() != want {
		t.Errorf("refusal = %q, want %q", err, want)
	}
}

// TestQueryStringContainmentReEntersAsItself pins the echo for the new shape:
// a containment census's header names both patterns and the separator, and
// parses back to the query that printed it — including the case whose
// namespace holds a `//` of its own.
func TestQueryStringContainmentReEntersAsItself(t *testing.T) {
	cases := []struct{ in, want string }{
		{"*@maxOccurs//*@maxOccurs", ns + "*@maxOccurs//" + ns + "*@maxOccurs"},
		{"{}outer//{*}*@{urn:y}*", "{}outer//{*}*@{urn:y}*"},
		{"choice|sequence@maxOccurs|minOccurs//element@name,type",
			ns + "choice|" + ns + "sequence@maxOccurs|minOccurs//" + ns + "element@name,type"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			q := mustQuery(t, tc.in)
			if got := q.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
			back, err := parseQuery(q.String())
			if err != nil {
				t.Fatalf("parseQuery(%q): %v", q.String(), err)
			}
			if back.String() != q.String() {
				t.Errorf("%q re-entered as %q", q.String(), back.String())
			}
			if back.Ancestor == nil || !slices.Equal(back.Ancestor.Element, q.Ancestor.Element) {
				t.Errorf("%q re-entered with ancestor %+v, want %+v", q.String(), back.Ancestor, q.Ancestor)
			}
		})
	}
}

// TestParseArgs pins the command line: query alone censuses the suite, a
// second argument narrows the tree, anything else is a usage error.
func TestParseArgs(t *testing.T) {
	q, root, err := parseArgs([]string{"element"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if root != suiteroot.Default {
		t.Errorf("root = %q, want %q", root, suiteroot.Default)
	}
	if len(q.Element) != 1 || q.Element[0].Local != "element" {
		t.Errorf("Element = %+v, want the one name element", q.Element)
	}
	if _, root, err = parseArgs([]string{"element", "some/dir"}); err != nil || root != "some/dir" {
		t.Errorf("parseArgs with a dir = %q, %v; want some/dir, nil", root, err)
	}
	for _, args := range [][]string{nil, {"a", "b", "c"}} {
		if _, _, err := parseArgs(args); err == nil {
			t.Errorf("parseArgs(%q) = nil error, want a usage error", args)
		}
	}
}

// suiteRoot locates the W3C suite from this package's directory, or skips:
// the submodule is absent in a fresh container (#659), and a census tool's
// own tests must not be the thing that demands it.
func suiteRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..", suiteroot.Default)
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(entries) == 0) {
		t.Skipf("suite absent: run `git submodule update --init %s`", suiteroot.Default)
	}
	if err != nil {
		t.Fatalf("reading %s: %v", root, err)
	}
	return root
}

// TestSuiteTopLevelCensusOfFinalAndAbstract pins #1205's census, which two
// agents in one session each hand-wrote a namespace-aware walk to take
// because the tool could not answer it (#1282). This predates the query
// language's own `|` join (#1391, #1554); "final= OR abstract=" is still two
// censuses deduped by position here rather than one `element@final|abstract`
// query — that composition is the whole method, and the figures below are
// the pin it reproduces.
func TestSuiteTopLevelCensusOfFinalAndAbstract(t *testing.T) {
	root := suiteRoot(t)
	type where struct {
		File string
		Line int
		Col  int
	}
	seen := map[where]bool{}
	var union []hit
	for _, q := range []string{"element@final", "element@abstract"} {
		rep, err := census(root, mustQuery(t, q))
		if err != nil {
			t.Fatalf("census %s: %v", q, err)
		}
		for _, h := range rep.Hits {
			at := where{h.File, h.Line, h.Col}
			if seen[at] {
				continue
			}
			seen[at] = true
			union = append(union, h)
		}
	}

	// The three parents #1205 grouped as top-level. `redefine` admits no
	// `element` child (xmlschema11-1.md:4078), so a hit under one is an
	// invalid fixture reported verbatim — this tool censuses, it does not
	// correct.
	topLevel := map[xsd.QName]bool{
		{Space: xsd.XMLSchemaNS, Local: "schema"}:   true,
		{Space: xsd.XMLSchemaNS, Local: "redefine"}: true,
		{Space: xsd.XMLSchemaNS, Local: "override"}: true,
	}
	top, local := 0, 0
	for _, h := range union {
		if topLevel[h.Parent] {
			top++
			continue
		}
		local++
	}
	if len(union) != 139 || top != 133 || local != 6 {
		t.Errorf("census = %d hit(s), %d top-level, %d local; want 139, 133, 6", len(union), top, local)
	}
}

// TestSuiteAlternativeContentModel re-derives #1275's census FROM THE TOOL.
// Two agents in one session each hand-wrote a walk to reach it and neither
// scan survived (#1304); this reads it off the hits' own child lists.
// xs:altType's model is "(annotation?, (simpleType | complexType)?)"
// (docs/specs/md/xmlschema11-1.md:5137), and #1275 concluded that no suite
// occurrence carries a child outside it, nor a second simpleType/complexType
// beside a first.
//
// A match whose read broke off first fails here rather than passing quietly:
// a prefix of a child list cannot answer a question about the whole of one.
func TestSuiteAlternativeContentModel(t *testing.T) {
	root := suiteRoot(t)
	rep, err := census(root, mustQuery(t, "alternative"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	if len(rep.Hits) != 232 || countFiles(rep.Hits) != 81 {
		t.Errorf("census = %d occurrence(s) in %d fixture(s); want 232 in 81",
			len(rep.Hits), countFiles(rep.Hits))
	}
	admitted := map[xsd.QName]bool{
		xsdName("annotation"):  true,
		xsdName("simpleType"):  true,
		xsdName("complexType"): true,
	}
	withType := 0
	for _, h := range rep.Hits {
		if h.ChildrenUnclosed {
			t.Errorf("%s:%d:%d — the read stopped inside this match, so %s is a prefix and settles nothing",
				h.File, h.Line, h.Col, childNames(h))
		}
		types := 0
		for _, c := range h.Children {
			if !admitted[c] {
				t.Errorf("%s:%d:%d carries child %s, which xs:altType does not admit", h.File, h.Line, h.Col, c)
			}
			if c.Local == "simpleType" || c.Local == "complexType" {
				types++
			}
		}
		if types > 1 {
			t.Errorf("%s:%d:%d carries %d type children, %s — xs:altType admits at most one",
				h.File, h.Line, h.Col, types, childNames(h))
		}
		if types == 1 {
			withType++
		}
	}
	// The corpus does exercise the inline type child, so the check above is
	// not passing on 232 empty lists: a census that collected no child at all
	// would satisfy every assertion in this test but this one.
	if withType != 43 {
		t.Errorf("%d occurrence(s) carry an inline type child, want 43", withType)
	}
}

// TestSuiteAcceptanceCases pins #1239's two acceptance cases against the real
// corpus: each fixture the three under-predicted landings missed must come
// back from ONE whole-corpus query. si10 is UTF-16LE and unprefixed,
// target002 is plain text and `xs:`-prefixed, and si03 differs from its
// neighbours only in the clause it violates — so a census that scopes,
// decodes, or matches by spelling drops at least one of them.
func TestSuiteAcceptanceCases(t *testing.T) {
	root := suiteRoot(t)
	cases := []struct {
		query string
		want  []string
	}{
		{
			query: "element@targetNamespace",
			want: []string{
				"ibmData/schema_invalid/S3_2_3/s3_2_3si10.xsd",
				"saxonData/TargetNS/target002.n.xsd",
			},
		},
		{
			query: "attribute@targetNamespace",
			want: []string{
				"ibmData/schema_invalid/S3_2_3/s3_2_3si02.xsd",
				"ibmData/schema_invalid/S3_2_3/s3_2_3si03.xsd",
				"ibmData/schema_invalid/S3_2_3/s3_2_3si05.xsd",
				"ibmData/schema_invalid/S3_2_3/s3_2_3si09.xsd",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			rep, err := census(root, mustQuery(t, tc.query))
			if err != nil {
				t.Fatalf("census: %v", err)
			}
			matched := map[string]bool{}
			for _, h := range rep.Hits {
				matched[h.File] = true
			}
			for _, w := range tc.want {
				if !matched[w] {
					t.Errorf("%s carries the construct but the census missed it", w)
				}
			}
		})
	}
}

// TestSuiteAttributeNameAxis re-derives #1369's census FROM THE TOOL. That
// census walked all 15,470 schema documents from a DIAG-gated test written for
// the one landing and deleted with it — the second hand-rolled corpus
// instrument in four days (#1391).
//
// The figures pinned here are the axis itself, roster-free. #1369's own
// 22/16/21 is this census MINUS the 49-entry Appendix A roster that landing
// added, and the subtraction stays the reader's: the roster is
// `parser/produce_s4sattrs.go`'s data, and a copy of it here would be a second
// encoding of one fact (STYLE D3). What this test pins on that side is that
// the pairs the subtraction turns on are IN the census with their fixtures —
// three attribute names Appendix A declares nowhere on the element carrying
// them, one of them a case-mangled `substitutionGroup`.
func TestSuiteAttributeNameAxis(t *testing.T) {
	root := suiteRoot(t)
	rep, err := census(root, mustQuery(t, "*@*"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	groups := groupPairs(rep.Hits)
	occurrences := 0
	for _, g := range groups {
		occurrences += g.Occurrences
	}
	// The second figure below is why the axis report is grouped and the first
	// is what it is grouped into: one line per occurrence is not a report.
	if len(groups) != 185 || occurrences != 172648 {
		t.Errorf("axis = %d pair(s), %d attribute occurrence(s); want 185, 172648", len(groups), occurrences)
	}

	want := map[pairName]pairGroup{
		{Element: xsdName("element"), Attr: xsd.QName{Local: "nullable"}}: {
			Occurrences: 1, Fixtures: []string{"msData/element/elemK007.xsd"},
		},
		{Element: xsdName("element"), Attr: xsd.QName{Local: "SubstitutionGroup"}}: {
			Occurrences: 1, Fixtures: []string{"msData/schema/78029c.xsd"},
		},
		{Element: xsdName("attribute"), Attr: xsd.QName{Local: "value"}}: {
			Occurrences: 5, Fixtures: []string{
				"msData/attribute/attH001.xsd",
				"msData/attribute/attJ017.xsd",
				"msData/simpleType/stB007.xsd",
				"msData/simpleType/stC027.xsd",
				"msData/simpleType/stC028.xsd",
			},
		},
	}
	for _, g := range groups {
		w, ok := want[g.pairName]
		if !ok {
			continue
		}
		delete(want, g.pairName)
		if g.Occurrences != w.Occurrences {
			t.Errorf("%s = %d occurrence(s), want %d", renderPair(g), g.Occurrences, w.Occurrences)
		}
		if strings.Join(g.Fixtures, " ") != strings.Join(w.Fixtures, " ") {
			t.Errorf("%s in %v, want %v", renderPair(g), g.Fixtures, w.Fixtures)
		}
	}
	// The leftovers are drained and sorted before they are reported, because
	// the failure text is output and a map order would spell the same fault
	// differently on every run (STYLE D2).
	missed := make([]pairName, 0, len(want))
	for p := range want {
		missed = append(missed, p)
	}
	sort.Slice(missed, func(i, j int) bool { return lessPair(missed[i], missed[j]) })
	for _, p := range missed {
		t.Errorf("the axis missed %s@%s, which the corpus carries", p.Element, p.Attr)
	}
}

// TestSuiteBooleanAttributeValueCensus re-derives #456's census FROM THE TOOL:
// every occurrence of six boolean-valued attributes in the corpus, partitioned
// on the value's lexical space (xmlschema11-2.md §3.2.2.1 — the boolean
// lexical space is exactly true, false, 1 and 0, and Appendix A fixes
// whiteSpace to collapse, so a padded literal is VALID and the set #456
// improved is the out-of-space partition alone).
//
// The population is a DISJUNCTION over the six names, which is the "|" join;
// the "," join is their intersection and a different census entirely. #456
// hand-rolled this one, reported 10 occurrences in 8 fixtures against 20 out
// of the lexical space, and deleted the script. The occurrence figures
// reproduce; the fixture count does NOT, and the corpus says the tool is
// right — `saxonData/CTA/cta0010.xsd` carries two padded `inheritable`
// attributes, so the padded partition falls in NINE fixtures, not eight.
func TestSuiteBooleanAttributeValueCensus(t *testing.T) {
	root := suiteRoot(t)
	rep, err := census(root, mustQuery(t,
		"*@mixed|abstract|nillable|inheritable|appliesToEmpty|defaultAttributesApply"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	lexical := map[string]bool{"true": true, "false": true, "1": true, "0": true}
	padded, outside := 0, 0
	paddedFixtures := map[string]bool{}
	for _, h := range rep.Hits {
		for _, a := range h.Attrs {
			if lexical[a.Value] {
				continue
			}
			if !lexical[strings.TrimSpace(a.Value)] {
				outside++
				continue
			}
			padded++
			paddedFixtures[h.File] = true
		}
	}
	if padded != 10 || len(paddedFixtures) != 9 || outside != 20 {
		t.Errorf("census = %d padded-but-valid occurrence(s) in %d fixture(s), %d out of the lexical space;"+
			" want 10 in 9, and 20", padded, len(paddedFixtures), outside)
	}
}

// TestSuiteMaxOccursInsideMaxOccurs re-derives #1557's candidate census FROM
// THE TOOL — the population two consecutive grounding rounds answered
// `not derived` for, because no query shape composed an ancestor predicate
// (#1585). The figure bounds a LEXICAL nesting population from above and the
// report says so in its own output; this test pins the census, and the two
// named fixtures pin each direction it is wrong in.
//
// particlesZ015.xsd is the schema of the case arm (b) flipped:
// `<xsd:choice maxOccurs="unbounded">` over two `<xsd:element ref …
// maxOccurs="20"/>`, which this census returns. groupH020.xsd is the other
// direction — a repeating `<xsd:group ref>` over a group that itself holds
// one, repeating inside repeating only AFTER resolution and carrying no
// lexical nest at all, which this census cannot see.
func TestSuiteMaxOccursInsideMaxOccurs(t *testing.T) {
	root := suiteRoot(t)
	rep, err := census(root, mustQuery(t, "*@maxOccurs//*@maxOccurs"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	if len(rep.Hits) != 787 || countFiles(rep.Hits) != 196 {
		t.Errorf("census = %d occurrence(s) in %d fixture(s); want 787 in 196",
			len(rep.Hits), countFiles(rep.Hits))
	}
	matched := map[string]bool{}
	named := 0
	for _, h := range rep.Hits {
		matched[h.File] = true
		if h.Ancestor != h.Parent {
			named++
		}
	}
	if !matched["msData/particles/particlesZ015.xsd"] {
		t.Error("the census missed msData/particles/particlesZ015.xsd, whose maxOccurs nests lexically")
	}
	if matched["msData/group/groupH020.xsd"] {
		t.Error("the census claims msData/group/groupH020.xsd, whose nest exists only after ref resolution")
	}
	// Each of these qualifies on an element the parent field does not even
	// name, so a census reading only `hit.Parent` reports none of them.
	if named != 94 {
		t.Errorf("%d hit(s) name an ancestor their parent field does not, want 94", named)
	}
}

// valueDoc carries every answer a value-test token can give: bound through a
// prefix, unbound, bound to no namespace with no default in scope, bound
// through a prefix declared on the token's OWN start tag, the three shapes
// that are not a QName, an empty value, a list split on a tab, and a list
// joined by an NBSP, which is not XML whitespace and so is one token (#1671).
const valueDoc = `<?xml version="1.0"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" id="s">
  <xs:element name="a" type="xs:ENTITY"/>
  <xs:element name="b" type="u:thing"/>
  <xs:element name="c" type=":lead"/>
  <xs:element name="d" type="bare"/>
  <xs:element name="e" type="p:q:r"/>
  <xs:element name="f" type="tail:"/>
  <xs:element name="g" type=""/>
  <xs:element name="h" xmlns:u="urn:u" type="u:thing"/>
  <xs:union memberTypes="xs:integer&#9;xs:ENTITY"/>
  <xs:union memberTypes="xs:integer&#xA0;xs:ENTITY"/>
  <xs:union memberTypes="xs:integer :lead p:q:r tail:"/>
</xs:schema>`

// resolvedLines renders each hit of a scan as "line:attrs", the attributes in
// the report's own spelling, resolutions included ([renderAttrs]).
func resolvedLines(scan fixtureScan) []string {
	var lines []string
	for _, h := range scan.Hits {
		lines = append(lines, strconv.Itoa(h.Line)+":"+renderAttrs(h.Attrs))
	}
	return lines
}

// TestScanFixtureResolvesAValueTest pins the value test's matching rules, row
// by row over [valueDoc]: `{*}*` matches every BOUND token and never an
// unbound one or one that is no QName; UNBOUND matches only a prefix no
// binding in scope at the token's own start tag declares, so line 10's
// `u:thing` is bound where line 4's is not; an unprefixed token with no
// default namespace is bound to no namespace and answers `{}local`; a list is
// split on XML whitespace only; and both joins are read over the attributes
// that ANSWER, recording only those.
func TestScanFixtureResolvesAValueTest(t *testing.T) {
	const x = "{" + xsd.XMLSchemaNS + "}"
	cases := []struct {
		query string
		want  []string
	}{
		{"*@type={*}*", []string{
			`3: type="xs:ENTITY"->[` + x + `ENTITY]`,
			`6: type="bare"->[{}bare]`,
			`10: type="u:thing"->[{urn:u}thing]`,
		}},
		{"*@type=UNBOUND", []string{
			`4: type="u:thing"->[UNBOUND]`,
		}},
		{"*@type={}bare", []string{
			`6: type="bare"->[{}bare]`,
		}},
		{"*@type={*}*|UNBOUND", []string{
			`3: type="xs:ENTITY"->[` + x + `ENTITY]`,
			`4: type="u:thing"->[UNBOUND]`,
			`6: type="bare"->[{}bare]`,
			`10: type="u:thing"->[{urn:u}thing]`,
		}},
		{"*@memberTypes=" + x + "ENTITY", []string{
			`11: memberTypes="xs:integer\txs:ENTITY"->[` + x + `integer, ` + x + `ENTITY]`,
		}},
		{"*@memberTypes={*}*", []string{
			`11: memberTypes="xs:integer\txs:ENTITY"->[` + x + `integer, ` + x + `ENTITY]`,
			`13: memberTypes="xs:integer :lead p:q:r tail:"->[` + x +
				`integer, (not a QName), (not a QName), (not a QName)]`,
		}},
		// The test applies to every listed attribute: under `,` both must
		// answer, so only the element whose type is in no namespace counts.
		{"element@name,type={}*", []string{
			`6: name="d"->[{}d] type="bare"->[{}bare]`,
		}},
		// Under `|` one answering attribute is enough, and the one that does
		// not answer is not recorded.
		{"element@name|type=" + x + "ENTITY", []string{
			`3: type="xs:ENTITY"->[` + x + `ENTITY]`,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			scan := scanFixture("v.xsd", strings.NewReader(valueDoc), mustQuery(t, tc.query))
			if scan.Err != nil {
				t.Fatalf("scanFixture: %v", scan.Err)
			}
			if got := resolvedLines(scan); !slices.Equal(got, tc.want) {
				t.Errorf("hits =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(tc.want, "\n  "))
			}
		})
	}
}

// TestScanFixtureRecordsNoResolutionWithoutAValueTest pins the invariant
// every other query's report rests on: a hit from a query with no value test
// carries no resolution at all, however QName-shaped its values are, so its
// match line renders the bytes it always did.
func TestScanFixtureRecordsNoResolutionWithoutAValueTest(t *testing.T) {
	scan := scanFixture("v.xsd", strings.NewReader(valueDoc), mustQuery(t, "*@type|memberTypes"))
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	if len(scan.Hits) != 11 {
		t.Fatalf("got %d hit(s), want 11", len(scan.Hits))
	}
	for _, h := range scan.Hits {
		for _, a := range h.Attrs {
			if a.Resolved != nil {
				t.Errorf("line %d: %s carries resolutions %+v without a value test", h.Line, a.Name.Local, a.Resolved)
			}
		}
	}
	if got, want := renderAttrs(scan.Hits[0].Attrs), ` type="xs:ENTITY"`; got != want {
		t.Errorf("renderAttrs = %q, want %q", got, want)
	}
}

// TestScanFixtureAppliesAValueTestOnTheAncestorSide pins that the ancestor
// pattern takes a value test through the one [match] both sides share: only
// the attribute under the complexType whose name resolves to `{}ct` counts.
func TestScanFixtureAppliesAValueTestOnTheAncestorSide(t *testing.T) {
	doc := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:complexType name="ct"><xs:attribute name="x"/></xs:complexType>
  <xs:complexType name="other"><xs:attribute name="y"/></xs:complexType>
</xs:schema>`
	scan := scanFixture("v.xsd", strings.NewReader(doc), mustQuery(t, "complexType@name={}ct//attribute@name"))
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	if got, want := resolvedLines(scan), []string{`2: name="x"`}; !slices.Equal(got, want) {
		t.Errorf("hits = %q, want %q", got, want)
	}
}

// TestScanFixtureTriesEveryAttributeTheNameAdmits pins that a value test is
// applied to every attribute a wildcard name pattern admits, not only to the
// first: `{*}type` admits both `xsi:type` and a plain `type`, and only the
// second answers `{}1`, as in msData/simpleType/test107331_9.xml (#1671).
func TestScanFixtureTriesEveryAttributeTheNameAdmits(t *testing.T) {
	doc := `<root xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <item xsi:type="ct4" type="1"/>
</root>`
	scan := scanFixture("v.xml", strings.NewReader(doc), mustQuery(t, "{*}*@{*}type={}1"))
	if scan.Err != nil {
		t.Fatalf("scanFixture: %v", scan.Err)
	}
	if got, want := resolvedLines(scan), []string{`2: type="1"->[{}1]`}; !slices.Equal(got, want) {
		t.Errorf("hits = %q, want %q", got, want)
	}
}

// valueCaveat is a line of the value-test caveat, which a report prints
// exactly when a side of the query carries a value test.
const valueCaveat = "read this as a bound on DIRECT references, never as a population of resolved types:"

// TestReportPrintsTheResolutionsAndTheValueCaveat pins the value test's
// report: a whole match line, resolution suffix included, and every point of
// the caveat. The caveat prints for a test on either side of `//`, beside the
// containment caveat when both apply.
func TestReportPrintsTheResolutionsAndTheValueCaveat(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "v.xsd", valueDoc)
	for _, q := range []string{"*@type={*}*|UNBOUND", "schema@id={}s//*@type=UNBOUND", "schema@id={}s//*@type"} {
		t.Run(q, func(t *testing.T) {
			rep, err := census(root, mustQuery(t, q))
			if err != nil {
				t.Fatalf("census: %v", err)
			}
			var out strings.Builder
			if err := printReport(&out, rep); err != nil {
				t.Fatalf("printReport: %v", err)
			}
			for _, want := range []string{
				valueCaveat,
				"it follows no derivation chain, so a declaration typed by a named simpleType that",
				"restricts an operand's type is found only by a second query on that type's name, and",
				"it resolves no <element ref>/<group ref>. The query, not any schema, asserts that the",
				"attribute holds QNames, and a token that is not a lexical QName answers no operand,",
				"  UNBOUND included\n",
			} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("report does not carry %q:\n%s", want, out.String())
				}
			}
			if !strings.Contains(out.String(), "\n  v.xsd:4:3 element="+ns+"element parent="+ns+"schema") {
				t.Errorf("report does not carry line 4's hit:\n%s", out.String())
			}
		})
	}
	rep, err := census(root, mustQuery(t, "*@type={*}*|UNBOUND"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	var out strings.Builder
	if err := printReport(&out, rep); err != nil {
		t.Fatalf("printReport: %v", err)
	}
	want := "\n  v.xsd:4:3 element=" + ns + "element parent=" + ns + `schema children=[] type="u:thing"->[UNBOUND]` + "\n"
	if !strings.Contains(out.String(), want) {
		t.Errorf("report does not carry the line %q:\n%s", want, out.String())
	}
	if strings.Contains(out.String(), "bound from above") {
		t.Errorf("report carries the containment caveat without a containment query:\n%s", out.String())
	}
}

// TestReportPrintsNoResolutionWithoutAValueTest pins the other half: a query
// with no value test prints no resolution suffix and no value caveat, and its
// match line is the exact bytes it was before #1671.
func TestReportPrintsNoResolutionWithoutAValueTest(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "v.xsd", valueDoc)
	for _, q := range []string{"*@type", "*@type|memberTypes", "*@*", "*@name//*@type"} {
		t.Run(q, func(t *testing.T) {
			rep, err := census(root, mustQuery(t, q))
			if err != nil {
				t.Fatalf("census: %v", err)
			}
			var out strings.Builder
			if err := printReport(&out, rep); err != nil {
				t.Fatalf("printReport: %v", err)
			}
			for _, absent := range []string{"->[", "DIRECT references"} {
				if strings.Contains(out.String(), absent) {
					t.Errorf("report carries %q, which belongs to the value test alone:\n%s", absent, out.String())
				}
			}
		})
	}
	rep, err := census(root, mustQuery(t, "*@type"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	var out strings.Builder
	if err := printReport(&out, rep); err != nil {
		t.Fatalf("printReport: %v", err)
	}
	want := "\n  v.xsd:4:3 element=" + ns + "element parent=" + ns + `schema children=[] type="u:thing"` + "\n"
	if !strings.Contains(out.String(), want) {
		t.Errorf("report does not carry the line %q:\n%s", want, out.String())
	}
}

// TestParseQueryValueTest pins the value-test grammar: operands after `=`,
// joined by `|`, each a braced name or UNBOUND, on either side of `//`. The
// Clark-named attribute and the URI holding `=`, `|` and `,` are the rows the
// scan turns on — [splitName] consumes a wrapper whole before it looks for a
// separator — and `{}UNBOUND` is a name, never the keyword.
func TestParseQueryValueTest(t *testing.T) {
	cases := []struct {
		in       string
		attrs    []namePat
		join     attrJoin
		value    valueTest
		ancestor valueTest
	}{
		{
			in:    "*@type={urn:x}t",
			attrs: []namePat{{Local: "type"}},
			join:  joinAll,
			value: valueTest{Names: []namePat{{Space: "urn:x", Local: "t"}}},
		},
		{
			in:    "*@type|base={urn:x}t|{*}*|UNBOUND",
			attrs: []namePat{{Local: "type"}, {Local: "base"}},
			join:  joinAny,
			value: valueTest{Names: []namePat{{Space: "urn:x", Local: "t"}, {Space: wildcard, Local: wildcard}}, Unbound: true},
		},
		{
			in:    "*@a,b={}bare",
			attrs: []namePat{{Local: "a"}, {Local: "b"}},
			join:  joinAll,
			value: valueTest{Names: []namePat{{Local: "bare"}}},
		},
		{
			in:    "*@type=UNBOUND|UNBOUND",
			attrs: []namePat{{Local: "type"}},
			join:  joinAll,
			value: valueTest{Unbound: true},
		},
		{
			in:    "*@type={}UNBOUND",
			attrs: []namePat{{Local: "type"}},
			join:  joinAll,
			value: valueTest{Names: []namePat{{Local: unboundOperand}}},
		},
		{
			in:    "{*}*@{" + xsiNS + "}type=UNBOUND",
			attrs: []namePat{{Space: xsiNS, Local: "type"}},
			join:  joinAll,
			value: valueTest{Unbound: true},
		},
		{
			in:    "*@type={urn:a=b|c,d}t",
			attrs: []namePat{{Local: "type"}},
			join:  joinAll,
			value: valueTest{Names: []namePat{{Space: "urn:a=b|c,d", Local: "t"}}},
		},
		{
			in:       "complexType@name={}ct//attribute@type={*}*",
			attrs:    []namePat{{Local: "type"}},
			join:     joinAll,
			value:    valueTest{Names: []namePat{{Space: wildcard, Local: wildcard}}},
			ancestor: valueTest{Names: []namePat{{Local: "ct"}}},
		},
		{
			in:    "*@type",
			attrs: []namePat{{Local: "type"}},
			join:  joinAll,
		},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			q := mustQuery(t, tc.in)
			if !slices.Equal(q.Attrs, tc.attrs) {
				t.Errorf("Attrs = %+v, want %+v", q.Attrs, tc.attrs)
			}
			if q.Join != tc.join {
				t.Errorf("Join = %q, want %q", q.Join, tc.join)
			}
			if !sameValueTest(q.Value, tc.value) {
				t.Errorf("Value = %+v, want %+v", q.Value, tc.value)
			}
			if q.Ancestor != nil && !sameValueTest(q.Ancestor.Value, tc.ancestor) {
				t.Errorf("Ancestor.Value = %+v, want %+v", q.Ancestor.Value, tc.ancestor)
			}
		})
	}
}

// sameValueTest reports whether two value tests hold the same operands in the
// same order.
func sameValueTest(a, b valueTest) bool {
	return slices.Equal(a.Names, b.Names) && a.Unbound == b.Unbound
}

// TestParseQueryRejectsAValueTest pins each refusal of the value-test grammar
// by its whole message, because a message naming the wrong half of a query
// changes no branch (#1048): a test after an element name, on either side of
// `//`; a braceless operand, the keyword's case included, since no default
// namespace is assumed for data; the `,` join between operands; `@*`, whose
// report has no value column; and anything after the operands but `//`.
func TestParseQueryRejectsAValueTest(t *testing.T) {
	braceless := func(q, tok string) string {
		return `query "` + q + `": value operand "` + tok + `" names no namespace: write it out as {uri}local,` +
			` {}local for no namespace or {*}local for any, since no default can be assumed for data, or write UNBOUND`
	}
	for _, tc := range []struct{ in, want string }{
		{"element=x", `query "element=x": a value test follows an attribute list ("@attr=…"), never an element name`},
		{"a//b={*}*", `query "a//b={*}*": a value test follows an attribute list ("@attr=…"), never an element name`},
		{"*@type=ENTITY", braceless("*@type=ENTITY", "ENTITY")},
		{"*@type=unbound", braceless("*@type=unbound", "unbound")},
		{"*@type=UNBOUNDX", braceless("*@type=UNBOUNDX", "UNBOUNDX")},
		{"*@type={urn:x}a,{urn:x}b", `query "*@type={urn:x}a,{urn:x}b": value operands are joined by "|" (any one of them), never by ",": one token is never two names`},
		{"*@*={*}*", `query "*@*={*}*": "@*" names the whole attribute axis and takes no value test`},
		{"*@{urn:y}*=UNBOUND", `query "*@{urn:y}*=UNBOUND": "@*" names the whole attribute axis and takes no value test`},
		{"*@type={urn:x}a@b", `query "*@type={urn:x}a@b": a value test ends the pattern, so only "//" or the end of the query may follow its operands, found "@b"`},
		{"*@type=UNBOUND=x", `query "*@type=UNBOUND=x": a value test ends the pattern, so only "//" or the end of the query may follow its operands, found "=x"`},
		{"*@type=", `query "*@type=": empty value operand`},
		{"*@type={urn:x}a|", `query "*@type={urn:x}a|": empty value operand`},
		{"*@type={urn:x", `query "*@type={urn:x": unterminated "{" in "{urn:x"`},
		{"*@type={urn:x}", `query "*@type={urn:x}": empty local name`},
	} {
		t.Run(tc.in, func(t *testing.T) {
			_, err := parseQuery(tc.in)
			if err == nil {
				t.Fatalf("parseQuery = nil error, want %q", tc.want)
			}
			if err.Error() != tc.want {
				t.Errorf("refusal = %q, want %q", err, tc.want)
			}
		})
	}
}

// TestQueryStringValueTestReEntersAsItself pins the value test's echo: name
// operands first, always braced so a no-namespace name is never read as
// braceless, then UNBOUND — and the echo parses back to the same test on both
// sides of `//` (#1297).
func TestQueryStringValueTestReEntersAsItself(t *testing.T) {
	const x = "{" + xsd.XMLSchemaNS + "}"
	cases := []struct{ in, want string }{
		{"*@type|base|itemType|memberTypes=" + x + "ENTITY|" + x + "ENTITIES",
			ns + "*@type|base|itemType|memberTypes=" + x + "ENTITY|" + x + "ENTITIES"},
		{"{*}*" + xsiType + "=UNBOUND", "{*}*" + xsiType + "=UNBOUND"},
		{"*@type=UNBOUND|{}bare|{*}*", ns + "*@type={}bare|{*}*|UNBOUND"},
		{"*@type={}UNBOUND", ns + "*@type={}UNBOUND"},
		{"complexType@name={}ct//attribute@type={*}*", ns + "complexType@name={}ct//" + ns + "attribute@type={*}*"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			q := mustQuery(t, tc.in)
			if got := q.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
			back, err := parseQuery(q.String())
			if err != nil {
				t.Fatalf("parseQuery(%q): %v", q.String(), err)
			}
			if !sameValueTest(back.Value, q.Value) {
				t.Errorf("%q re-entered with value %+v, want %+v", q.String(), back.Value, q.Value)
			}
			if q.Ancestor != nil && !sameValueTest(back.Ancestor.Value, q.Ancestor.Value) {
				t.Errorf("%q re-entered with ancestor value %+v, want %+v", q.String(), back.Ancestor.Value, q.Ancestor.Value)
			}
		})
	}
}

// TestSuiteEntityReferenceCensus reproduces #773's census FROM THE TOOL: every
// type, base, itemType and memberTypes token resolving to xs:ENTITY or
// xs:ENTITIES. saxonData/Id/id017–id021.xsd are the fixtures behind the nine
// cases #773 banked, each referencing the type directly; id019 does it inside
// a two-token memberTypes, which is the per-token rule. "Names" is an
// at-least bar, so other fixtures may match too.
func TestSuiteEntityReferenceCensus(t *testing.T) {
	root := suiteRoot(t)
	const x = "{" + xsd.XMLSchemaNS + "}"
	rep, err := census(root, mustQuery(t, "*@type|base|itemType|memberTypes="+x+"ENTITY|"+x+"ENTITIES"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	matched := map[string]string{}
	for _, h := range rep.Hits {
		matched[h.File] = renderAttrs(h.Attrs)
	}
	for _, f := range []string{
		"saxonData/Id/id017.xsd", "saxonData/Id/id018.xsd", "saxonData/Id/id019.xsd",
		"saxonData/Id/id020.xsd", "saxonData/Id/id021.xsd",
	} {
		if _, ok := matched[f]; !ok {
			t.Errorf("the census missed %s", f)
		}
	}
	want := ` memberTypes="xs:ENTITY xs:integer"->[` + x + `ENTITY, ` + x + `integer]`
	if got := matched["saxonData/Id/id019.xsd"]; got != want {
		t.Errorf("id019.xsd resolved as %q, want %q", got, want)
	}
}

// TestSuiteUnboundXsiTypeCensus reproduces #1641's census FROM THE TOOL: the
// xsi:type whose prefix no binding in scope declares.
// saxonData/Complex/complex008.n2.xml declares only xmlns:xsi and carries
// xsi:type="unknownPrefix:unknownType".
func TestSuiteUnboundXsiTypeCensus(t *testing.T) {
	root := suiteRoot(t)
	rep, err := census(root, mustQuery(t, "{*}*"+xsiType+"=UNBOUND"))
	if err != nil {
		t.Fatalf("census: %v", err)
	}
	const fixture = "saxonData/Complex/complex008.n2.xml"
	for _, h := range rep.Hits {
		if h.File != fixture {
			continue
		}
		if got, want := renderAttrs(h.Attrs), ` type="unknownPrefix:unknownType"->[UNBOUND]`; got != want {
			t.Errorf("%s resolved as %q, want %q", fixture, got, want)
		}
		return
	}
	t.Errorf("the census missed %s", fixture)
}
