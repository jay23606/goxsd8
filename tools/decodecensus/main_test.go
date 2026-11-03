package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/codegen"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/tools/decodecensus/internal/assess"
	"github.com/kud360/goxsd8/xsd"
)

// writeTree writes files, slash paths relative to a fresh directory, and
// returns that directory.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
	return root
}

// instanceTest is one instanceTest of the fixture catalog over the document
// doc, under d/.
func instanceTest(name, doc, attrs string) string {
	return `<instanceTest name="` + name + `"` + attrs + `><instanceDocument xlink:href="../d/` + doc + `"/><expected validity="valid"/></instanceTest>`
}

// schemaTest is the schemaTest of a fixture group over the document doc,
// under d/.
func schemaTest(doc string) string {
	return `<schemaTest name="s"><schemaDocument xlink:href="../d/` + doc + `"/><expected validity="valid"/></schemaTest>`
}

// censusSuite is the fixture suite. Group g's schema generates, and its
// cases decode (a.xml, twice), decline with simple root (e.xml, twice) and
// choice content (c.xml), and fail otherwise, charged (invalid.xml) and with
// no declaration for the document element (u.xml), which decodes into the
// placeholder; its case old is withheld. Group rec's schema assembles and
// does not generate, group bad's does not assemble, and group none declares
// no schemaTest.
var censusSuite = map[string]string{
	"suite.xml": `<testSuite xmlns:xlink="http://www.w3.org/1999/xlink"><testSetRef xlink:href="sets/s.testSet"/></testSuite>`,
	"sets/s.testSet": `<testSet name="S" xmlns:xlink="http://www.w3.org/1999/xlink">
  <testGroup name="g">` + schemaTest("s.xsd") +
		instanceTest("decoded", "a.xml", "") + instanceTest("decoded2", "a.xml", "") +
		instanceTest("simple", "e.xml", "") + instanceTest("simple2", "e.xml", "") +
		instanceTest("choice", "c.xml", "") + instanceTest("invalid", "invalid.xml", "") +
		instanceTest("undeclared", "u.xml", "") + instanceTest("old", "a.xml", ` version="1.0"`) + `</testGroup>
  <testGroup name="rec">` + schemaTest("rec.xsd") + instanceTest("i", "a.xml", "") + `</testGroup>
  <testGroup name="bad">` + schemaTest("bad.xsd") + instanceTest("i", "a.xml", "") + `</testGroup>
  <testGroup name="none">` + instanceTest("i", "a.xml", "") + `</testGroup>
</testSet>`,
	"d/s.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="a"><xs:complexType><xs:sequence><xs:element name="b" type="xs:string"/></xs:sequence></xs:complexType></xs:element>
  <xs:element name="c"><xs:complexType><xs:choice><xs:element name="d" type="xs:string"/></xs:choice></xs:complexType></xs:element>
  <xs:element name="e" type="xs:string"/>
</xs:schema>`,
	"d/rec.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="a"><xs:complexType><xs:sequence><xs:element ref="a"/></xs:sequence></xs:complexType></xs:element>
</xs:schema>`,
	"d/bad.xsd":       `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:element name="a" type="undeclared"/></xs:schema>`,
	"d/a.xml":         `<a><b>x</b></a>`,
	"d/c.xml":         `<c><d>x</d></c>`,
	"d/e.xml":         `<e>x</e>`,
	"d/invalid.xml":   `<a/>`,
	"d/u.xml":         `<u/>`,
	"d/notes.txt":     "not a document",
	"d/sub/other.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"/>`,
}

// wantCounts is the report's head over censusSuite: every count, and its
// construct lines.
const wantCounts = `decodecensus: 10 instance case(s) under %ROOT%
  schema generates: 7
  decoded: 2
  declined: 3
  undecided: 0
  rejected codegen's own struct: 0
  failed other: 2
  decoded where validate rejects: 0

=== First decline, by construct: 2 construct(s) ===
       2  a document element of simple type
       1  choice content

=== Caveats ===
`

// TestCensusCounts runs the whole command over censusSuite, a driver built
// and run included, and pins every count the report prints, the decline
// table, every caveat and the placeholder's. It fails with instanceCases'
// withheld test removed, with plan's generates count removed, and with the
// decoded, declined or failed other arm of the driver's tallyOutcome removed;
// undecided, rejected and the differential, which no schema document makes
// codec return, are TestReportPrintsEachMergedCount's and package assess's.
func TestCensusCounts(t *testing.T) {
	root := writeTree(t, censusSuite)
	var out bytes.Buffer
	if err := run(&out, []string{root}); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := strings.ReplaceAll(wantCounts, "%ROOT%", root)
	if !strings.HasPrefix(out.String(), want) {
		t.Errorf("report opens\n%s\nwant\n%s", out.String(), want)
	}
	for _, c := range caveats {
		if !strings.Contains(out.String(), "  - "+c+"\n") {
			t.Errorf("report lacks the caveat %q", c)
		}
	}
	placeholder := placeholderCaveat(report{placeholder: 1})
	if !strings.HasSuffix(out.String(), "  - "+placeholder+"\n") {
		t.Errorf("report closes\n%s\nwant the caveat %q", out.String(), placeholder)
	}
}

// TestReportPrintsEachMergedCount merges two drivers' summaries, every count
// of each non-zero and one construct in both, and pins each line of the
// report's head and table: each count is the sum of the two, so a count
// merge or printReport drops, or prints under another label, fails here.
func TestReportPrintsEachMergedCount(t *testing.T) {
	rep := report{root: "r", considered: 40, generates: 30}
	rep.merge(assess.Summary{Decoded: 1, Undecided: 2, Rejected: 3, FailedOther: 4, Differential: 5, PlaceholderMismatch: 1,
		Declines: []assess.Tally{{Construct: "x", Count: 1}, {Construct: "y", Count: 1}}})
	rep.merge(assess.Summary{Decoded: 10, Undecided: 20, Rejected: 30, FailedOther: 40, Differential: 50, PlaceholderMismatch: 2,
		Declines: []assess.Tally{{Construct: "y", Count: 6}}})
	var out bytes.Buffer
	if err := printReport(&out, rep); err != nil {
		t.Fatalf("printReport: %v", err)
	}
	want := `decodecensus: 40 instance case(s) under r
  schema generates: 30
  decoded: 11
  declined: 8
  undecided: 22
  rejected codegen's own struct: 33
  failed other: 44
  decoded where validate rejects: 55

=== First decline, by construct: 2 construct(s) ===
       7  y
       1  x
`
	if !strings.HasPrefix(out.String(), want) {
		t.Errorf("report opens\n%s\nwant\n%s", out.String(), want)
	}
	if rep.sum.PlaceholderMismatch != 3 {
		t.Errorf("merged PlaceholderMismatch = %d, want 3", rep.sum.PlaceholderMismatch)
	}
}

// TestRankedOrdersByCountThenString pins ranked's order on three tallies
// whose first-seen order, count order with a first-seen tie-break and count
// order with the String tie-break the tool owes are three different orders.
func TestRankedOrdersByCountThenString(t *testing.T) {
	tallies := []assess.Tally{{Construct: "x", Count: 1}, {Construct: "m", Count: 2}, {Construct: "b", Count: 2}}
	want := []assess.Tally{{Construct: "b", Count: 2}, {Construct: "m", Count: 2}, {Construct: "x", Count: 1}}
	if got := ranked(tallies); !reflect.DeepEqual(got, want) {
		t.Errorf("ranked = %v, want %v", got, want)
	}
}

// TestCensusAbsentSuiteIsAMode holds an absent tree, and one with no suite
// index, to the supported mode: a report saying so, and no error.
func TestCensusAbsentSuiteIsAMode(t *testing.T) {
	cases := []struct{ name, root string }{
		{"absent", filepath.Join(t.TempDir(), "absent")},
		{"no index", writeTree(t, map[string]string{"notes.txt": ""})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := run(&out, []string{tc.root}); err != nil {
				t.Fatalf("run: %v", err)
			}
			want := "decodecensus: no suite index at " + filepath.Join(tc.root, "suite.xml") + " — corpus-absent mode, nothing to census."
			if !strings.HasPrefix(out.String(), want) {
				t.Errorf("report = %q, want it to open %q", out.String(), want)
			}
		})
	}
}

// failWriter refuses every write.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("refused") }

// TestCensusAbsentNoticeUnwrittenIsAnError holds a corpus-absent notice that
// cannot be written to run's error, which main exits non-zero on.
func TestCensusAbsentNoticeUnwrittenIsAnError(t *testing.T) {
	if err := run(failWriter{}, []string{filepath.Join(t.TempDir(), "absent")}); err == nil {
		t.Error("run wrote its notice to a refusing writer and returned no error")
	}
}

// ownTypeSchema declares, beside a, an element A whose type's name collides
// with a's and takes A2; x, whose choice's branches XA and XH, each over an
// <element ref>, record a and h in their headers; h and its substitution
// group member m, whose interface's branches HSubstitutionGroupH and
// HSubstitutionGroupM record h and m; tt, a sealed interface over its {type
// table}; s, one over the members of its abstract {type definition}; and the
// abstract abs, which gets no type.
const ownTypeSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="a"><xs:complexType><xs:sequence><xs:element name="b" type="xs:string"/></xs:sequence></xs:complexType></xs:element>
  <xs:element name="A" type="xs:string"/>
  <xs:element name="x"><xs:complexType><xs:choice><xs:element ref="a"/><xs:element ref="h"/></xs:choice></xs:complexType></xs:element>
  <xs:element name="h" type="xs:string"/>
  <xs:element name="m" substitutionGroup="h" type="xs:string"/>
  <xs:element name="tt" type="xs:anyType"><xs:alternative test="@k" type="xs:string"/></xs:element>
  <xs:complexType name="shape" abstract="true"/>
  <xs:complexType name="circle"><xs:complexContent><xs:extension base="shape"/></xs:complexContent></xs:complexType>
  <xs:complexType name="square"><xs:complexContent><xs:extension base="shape"/></xs:complexContent></xs:complexType>
  <xs:element name="s" type="shape"/>
  <xs:element name="abs" abstract="true" type="xs:string"/>
</xs:schema>`

// TestOwnTypeFindsEachTopLevelElementsType generates ownTypeSchema and pins
// the type resolveCase finds for each top-level element. Without namedFrom's
// test, a, h and m each match a branch beside their own type and resolve to
// none; with ownProvenances' plain entry alone, tt and s resolve to none.
func TestOwnTypeFindsEachTopLevelElementsType(t *testing.T) {
	root := writeTree(t, map[string]string{"s.xsd": ownTypeSchema, "out/go.mod": "module census\n"})
	schema, err := parser.Parse("s.xsd", parser.WithResolver(loader.Dir(root)))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	dir := filepath.Join(root, "out", "p")
	if errs := codegen.Generate([]codegen.Target{{Schema: schema, Dir: dir, Package: "p"}}); errs != nil {
		t.Fatalf("Generate: %v", errs)
	}
	types, err := generatedTypes(filepath.Join(dir, "xsd_gen.go"))
	if err != nil {
		t.Fatalf("generatedTypes: %v", err)
	}
	cases := []struct {
		local, typ string
		decl, ok   bool
	}{
		{"a", "A", true, true},
		{"A", "A2", true, true},
		{"x", "X", true, true},
		{"h", "H", true, true},
		{"m", "M", true, true},
		{"tt", "Tt", true, true},
		{"s", "S", true, true},
		{"abs", "", false, false},
		{"undeclared", "", false, false},
	}
	for _, tc := range cases {
		typ, decl, ok := resolveCase(schema, types, xsd.QName{Local: tc.local}, dir)
		if typ != tc.typ || decl != tc.decl || ok != tc.ok {
			t.Errorf("resolveCase(%s) = %q, %v, %v, want %q, %v, %v; types %v", tc.local, typ, decl, ok, tc.typ, tc.decl, tc.ok, types)
		}
	}
}

// TestOwnTypeRendersAnAbsoluteLoc parses the schema under its absolute path,
// so that its declarations' Loc URIs are absolute: resolveCase finds a's type
// only through headerLoc's branch for one, and fails with that branch
// removed.
func TestOwnTypeRendersAnAbsoluteLoc(t *testing.T) {
	root := writeTree(t, map[string]string{"s.xsd": ownTypeSchema, "out/go.mod": "module census\n"})
	schema, err := parser.Parse(filepath.Join(root, "s.xsd"), parser.WithResolver(loader.Dir(string(filepath.Separator))))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	decl, ok := schema.Element(xsd.QName{Local: "a"})
	if !ok || !filepath.IsAbs(decl.Loc().URI) {
		t.Fatalf("element a's Loc %v is not absolute", decl.Loc())
	}
	dir := filepath.Join(root, "out", "p")
	if errs := codegen.Generate([]codegen.Target{{Schema: schema, Dir: dir, Package: "p"}}); errs != nil {
		t.Fatalf("Generate: %v", errs)
	}
	types, err := generatedTypes(filepath.Join(dir, "xsd_gen.go"))
	if err != nil {
		t.Fatalf("generatedTypes: %v", err)
	}
	if typ, _, ok := resolveCase(schema, types, xsd.QName{Local: "a"}, dir); typ != "A" || !ok {
		t.Errorf("resolveCase(a) = %q, %v, want \"A\", true; types %v", typ, ok, types)
	}
}
