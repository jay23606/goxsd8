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
// cases decode (a.xml, twice, f.xml, of simple type, and g.xml, a choice,
// through the branch set the census binds), decline with another primitive,
// xs:precisionDecimal (e.xml, twice) and all content (c.xml), and fail otherwise, charged
// (invalid.xml) and with no declaration for the document element (u.xml), which
// decodes into the placeholder; its case old is withheld. Group rec's schema
// assembles and does not generate, group bad's does not assemble, and group none
// declares no schemaTest.
var censusSuite = map[string]string{
	"suite.xml": `<testSuite xmlns:xlink="http://www.w3.org/1999/xlink"><testSetRef xlink:href="sets/s.testSet"/></testSuite>`,
	"sets/s.testSet": `<testSet name="S" xmlns:xlink="http://www.w3.org/1999/xlink">
  <testGroup name="g">` + schemaTest("s.xsd") +
		instanceTest("decoded", "a.xml", "") + instanceTest("decoded2", "a.xml", "") +
		instanceTest("simple", "e.xml", "") + instanceTest("simple2", "e.xml", "") + instanceTest("simpledecoded", "f.xml", "") +
		instanceTest("all", "c.xml", "") + instanceTest("choice", "g.xml", "") + instanceTest("invalid", "invalid.xml", "") +
		instanceTest("undeclared", "u.xml", "") + instanceTest("old", "a.xml", ` version="1.0"`) + `</testGroup>
  <testGroup name="rec">` + schemaTest("rec.xsd") + instanceTest("i", "a.xml", "") + `</testGroup>
  <testGroup name="bad">` + schemaTest("bad.xsd") + instanceTest("i", "a.xml", "") + `</testGroup>
  <testGroup name="none">` + instanceTest("i", "a.xml", "") + `</testGroup>
</testSet>`,
	"d/s.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="a"><xs:complexType><xs:sequence><xs:element name="b" type="xs:string"/></xs:sequence></xs:complexType></xs:element>
  <xs:element name="c"><xs:complexType><xs:all><xs:element name="d" type="xs:string"/></xs:all></xs:complexType></xs:element>
  <xs:element name="e" type="xs:precisionDecimal"/>
  <xs:element name="f" type="xs:string"/>
  <xs:element name="g"><xs:complexType><xs:choice><xs:element name="h" type="xs:string"/><xs:element name="i" type="xs:string"/></xs:choice></xs:complexType></xs:element>
</xs:schema>`,
	"d/rec.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="a"><xs:complexType><xs:sequence><xs:element ref="a"/></xs:sequence></xs:complexType></xs:element>
</xs:schema>`,
	"d/bad.xsd":       `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:element name="a" type="undeclared"/></xs:schema>`,
	"d/a.xml":         `<a><b>x</b></a>`,
	"d/c.xml":         `<c><d>x</d></c>`,
	"d/e.xml":         `<e>1.0</e>`,
	"d/f.xml":         `<f>x</f>`,
	"d/g.xml":         `<g><i>x</i></g>`,
	"d/invalid.xml":   `<a/>`,
	"d/u.xml":         `<u/>`,
	"d/notes.txt":     "not a document",
	"d/sub/other.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"/>`,
}

// wantCounts is the report's head over censusSuite: every count, and its
// construct lines.
const wantCounts = `decodecensus: 12 instance case(s) under %ROOT%
  schema generates: 9
  decoded: 4
  declined: 3
  undecided: 0
  rejected codegen's own type: 0
  failed other: 2
  decoded where validate rejects: 0

=== First decline, by construct: 2 construct(s) ===
       2  an atomic simple type whose primitive is none of xs:string, xs:anyURI, xs:boolean, xs:float, xs:double, xs:decimal, xs:dateTime, xs:time, xs:date, xs:gYearMonth, xs:gYear, xs:gMonthDay, xs:gDay, xs:gMonth, xs:duration, xs:hexBinary, xs:base64Binary, xs:QName and xs:NOTATION
       1  all content

=== Caveats ===
`

// wantPaths is the -paths sections over censusSuite.
const wantPaths = `
=== Cases, by construct ===
an atomic simple type whose primitive is none of xs:string, xs:anyURI, xs:boolean, xs:float, xs:double, xs:decimal, xs:dateTime, xs:time, xs:date, xs:gYearMonth, xs:gYear, xs:gMonthDay, xs:gDay, xs:gMonth, xs:duration, xs:hexBinary, xs:base64Binary, xs:QName and xs:NOTATION (2):
  S/g/instance/simple
  S/g/instance/simple2
all content (1):
  S/g/instance/all

=== Cases, by listed count ===
rejected codegen's own type (0):
decoded where validate rejects (0):
Decode's ErrTypeMismatch on the placeholder (0):
`

// withCaveats returns head followed by the caveats' lines and the caveat
// placeholderCaveat renders for rep, as printReport closes its report
// before any -paths section.
func withCaveats(head string, rep report) string {
	var b strings.Builder
	b.WriteString(head)
	for _, c := range caveats {
		b.WriteString("  - " + c + "\n")
	}
	b.WriteString("  - " + placeholderCaveat(rep) + "\n")
	return b.String()
}

// TestCensusCounts runs the whole command over censusSuite, a driver built
// and run included, without and with -paths, and compares each whole report:
// every count the report prints, the decline table, every caveat and the
// placeholder's, and with -paths each declined construct's case IDs in byte
// order. Without -paths the report holds nothing after the placeholder's
// caveat. It fails with instanceCases' withheld test removed, with plan's
// generates count removed, and with the decoded, declined or failed other
// arm of the driver's tallyOutcome removed, and with driverSource's bound
// branch list dropped, which moves g.xml from decoded to rejected; undecided,
// rejected and the differential, which no schema document makes codec
// return, are TestReportPrintsEachMergedCount's and package assess's.
func TestCensusCounts(t *testing.T) {
	root := writeTree(t, censusSuite)
	plain := withCaveats(strings.ReplaceAll(wantCounts, "%ROOT%", root), report{placeholder: 1})
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"without -paths", []string{root}, plain},
		{"with -paths", []string{"-paths", root}, plain + wantPaths},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := run(&out, tc.args); err != nil {
				t.Fatalf("run: %v", err)
			}
			if out.String() != tc.want {
				t.Errorf("report\n%s\nwant\n%s", out.String(), tc.want)
			}
		})
	}
}

// TestReportPrintsEachMergedCount merges two drivers' summaries, every count
// of each non-zero, one construct in both and each listed count's cases split
// across both, and compares the whole report printReport renders with
// -paths: each count is the sum of the two, and each case ID is printed
// under its own heading, in byte order where the merge met them in reverse,
// so a count or list merge or printReport drops, or prints under another
// label or unsorted, fails here.
func TestReportPrintsEachMergedCount(t *testing.T) {
	rep := report{root: "r", considered: 40, generates: 30, placeholder: 9, unresolved: 1}
	rep.merge(assess.Summary{Decoded: 1, Undecided: 2, FailedOther: 4,
		Rejected:            []string{"S/r/instance/2"},
		Differential:        []string{"S/d/instance/3", "S/d/instance/2"},
		PlaceholderMismatch: []string{"S/m/instance/4", "S/m/instance/3"},
		Declines:            []assess.Tally{{Construct: "x", Cases: []string{"S/x/instance/1"}}, {Construct: "y", Cases: []string{"S/y/instance/3"}}}})
	rep.merge(assess.Summary{Decoded: 10, Undecided: 20, FailedOther: 40,
		Rejected:            []string{"S/r/instance/1"},
		Differential:        []string{"S/d/instance/1"},
		PlaceholderMismatch: []string{"S/m/instance/2", "S/m/instance/1"},
		Declines:            []assess.Tally{{Construct: "y", Cases: []string{"S/y/instance/2", "S/y/instance/1"}}}})
	var out bytes.Buffer
	if err := printReport(&out, rep, true); err != nil {
		t.Fatalf("printReport: %v", err)
	}
	want := withCaveats(`decodecensus: 40 instance case(s) under r
  schema generates: 30
  decoded: 11
  declined: 4
  undecided: 22
  rejected codegen's own type: 2
  failed other: 44
  decoded where validate rejects: 3

=== First decline, by construct: 2 construct(s) ===
       3  y
       1  x

=== Caveats ===
`, rep) + `
=== Cases, by construct ===
y (3):
  S/y/instance/1
  S/y/instance/2
  S/y/instance/3
x (1):
  S/x/instance/1

=== Cases, by listed count ===
rejected codegen's own type (2):
  S/r/instance/1
  S/r/instance/2
decoded where validate rejects (3):
  S/d/instance/1
  S/d/instance/2
  S/d/instance/3
Decode's ErrTypeMismatch on the placeholder (4):
  S/m/instance/1
  S/m/instance/2
  S/m/instance/3
  S/m/instance/4
`
	if out.String() != want {
		t.Errorf("report\n%s\nwant\n%s", out.String(), want)
	}
	if !strings.Contains(out.String(), "Decode's ErrTypeMismatch on the placeholder, 4 case(s)") {
		t.Errorf("the placeholder's caveat in\n%s\ncounts other than the 4 merged cases", out.String())
	}
}

// TestRankedOrdersByCountThenString pins ranked's order on three tallies
// whose first-seen order, count order with a first-seen tie-break and count
// order with the String tie-break the tool owes are three different orders.
func TestRankedOrdersByCountThenString(t *testing.T) {
	x := assess.Tally{Construct: "x", Cases: []string{"1"}}
	m := assess.Tally{Construct: "m", Cases: []string{"1", "2"}}
	b := assess.Tally{Construct: "b", Cases: []string{"1", "2"}}
	tallies := []assess.Tally{x, m, b}
	want := []assess.Tally{b, m, x}
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
	file, err := parseGenerated(filepath.Join(dir, "xsd_gen.go"))
	if err != nil {
		t.Fatalf("parseGenerated: %v", err)
	}
	types := generatedTypes(file)
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
	file, err := parseGenerated(filepath.Join(dir, "xsd_gen.go"))
	if err != nil {
		t.Fatalf("parseGenerated: %v", err)
	}
	types := generatedTypes(file)
	if typ, _, ok := resolveCase(schema, types, xsd.QName{Local: "a"}, dir); typ != "A" || !ok {
		t.Errorf("resolveCase(a) = %q, %v, want \"A\", true; types %v", typ, ok, types)
	}
}

// TestDriverSourceBindsBranches pins the census's branch-set supply: a
// generated file declaring two interfaces, one with two marker types and one
// with a union's non-struct variant, beside a method that is no marker, a
// pointer-receiver method of a marker's name and a generic interface, yields
// sealedInterfaces' list, and a driver source declaring one WithBranches per
// non-generic interface, in file order, bound to each decoder of that package
// alone.
func TestDriverSourceBindsBranches(t *testing.T) {
	const src = `package p0

type R interface {
	isR()
}

type RA struct{ A string }

func (RA) isR() {}

type RB struct{ B string }

func (RB) isR() {}

func (RB) String() string { return "" }

type U interface {
	isU()
}

type UInt int32

func (UInt) isU() {}

func (*RA) isU() {}

type Nillable[T, A any] interface {
	isNillable(T, A)
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "xsd_gen.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := parseGenerated(path)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealedInterfaces(file)
	want := []sealedDecl{{name: "R", branches: []string{"RA", "RB"}}, {name: "U", branches: []string{"UInt"}}}
	if !reflect.DeepEqual(sealed, want) {
		t.Errorf("sealedInterfaces = %+v, want %+v", sealed, want)
	}
	got := string(driverSource(batch{packages: []genPackage{{name: "p0", sealed: sealed}, {name: "p1"}}, decoders: []string{"p0.R", "p1.S", "p0.X"}}))
	for _, line := range []string{
		"\t\"github.com/kud360/goxsd8/codec\"\n",
		"var p0Branches = []codec.Option{\n\tcodec.WithBranches[p0.R](*new(p0.RA), *new(p0.RB)),\n\tcodec.WithBranches[p0.U](*new(p0.UInt)),\n}\n",
		"\tassess.DecoderFor[p0.R](p0Branches...),\n\tassess.DecoderFor[p1.S](),\n\tassess.DecoderFor[p0.X](p0Branches...),\n",
	} {
		if !strings.Contains(got, line) {
			t.Errorf("driver source\n%s\nlacks\n%s", got, line)
		}
	}
	if strings.Contains(got, "p1Branches") {
		t.Errorf("driver source\n%s\ndeclares branches for p1, which declares no interface", got)
	}
}
