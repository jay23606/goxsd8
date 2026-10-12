package parser_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// parseSet assembles roots over an in-memory document set and returns the
// report together with the parse outcome; the report is never nil.
func parseSet(t *testing.T, roots []parser.Root, docs map[string]string) (*xsd.Schema, *parser.AssemblyReport, error) {
	t.Helper()
	s, report, err := parser.ParseSet(roots, parser.WithResolver(loader.Map(docs)))
	if report == nil {
		t.Fatal("ParseSet returned a nil *AssemblyReport")
	}
	return s, report, err
}

// TestParseSetAttributesEachShortfallToItsRoot is per-root attribution
// (#1283, #1251): given several roots, a caller learns WHICH hint named no
// document, by the value it passed in, and a directive inside a document of the
// set that named no document stays an [parser.UnfollowedDirective] at its own
// position. The resolving hint sits between the two failing ones, so an ordinal
// correlation or a lost order both go red; a root leaking into Unfollowed, or a
// directive into UnfollowedRoots, changes a length.
func TestParseSetAttributesEachShortfallToItsRoot(t *testing.T) {
	docs := map[string]string{
		// The <xs:include> is on line 3, column 1.
		"a.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:a">` + "\n" +
			`<xs:element name="a" type="xs:string"/>` + "\n" +
			`<xs:include schemaLocation="nosuch-nested.xsd"/>` + "\n" +
			`</xs:schema>`,
		"good.xsd": wrap("urn:n2", `<xs:element name="g" type="xs:string"/>`),
	}
	missing1 := parser.HintAt("urn:n1", "missing1.xsd")
	good := parser.HintAt("urn:n2", "good.xsd")
	missing2 := parser.HintAt("urn:n3", "missing2.xsd")
	s, report, err := parseSet(t, []parser.Root{parser.RootAt("a.xsd"), missing1, good, missing2}, docs)
	if err != nil {
		t.Fatalf("ParseSet: %v", err)
	}
	if got, want := report.UnfollowedRoots(), []parser.Root{missing1, missing2}; !slices.Equal(got, want) {
		t.Errorf("UnfollowedRoots() = %v, want %v", got, want)
	}
	for i, want := range []string{"missing1.xsd", "missing2.xsd"} {
		if got := report.UnfollowedRoots()[i].Location(); got != want {
			t.Errorf("UnfollowedRoots()[%d].Location() = %q, want %q", i, got, want)
		}
	}
	wantDirective := []parser.UnfollowedDirective{{
		Reason: parser.UnfollowedLocationUnresolved,
		At:     xsderr.Loc{URI: "a.xsd", Line: 3, Col: 1},
	}}
	if got := report.Unfollowed(); !slices.Equal(got, wantDirective) {
		t.Errorf("Unfollowed() = %v, want exactly a.xsd's own <xs:include> %v", got, wantDirective)
	}
	if got, want := locations(report), []string{"a.xsd", "good.xsd"}; !slices.Equal(got, want) {
		t.Errorf("Documents() locations = %v, want %v", got, want)
	}
	for _, name := range []xsd.QName{{Space: "urn:a", Local: "a"}, {Space: "urn:n2", Local: "g"}} {
		if _, ok := s.Element(name); !ok {
			t.Errorf("assembled schema lacks element %v", name)
		}
	}
}

// TestParseSetReportsAsFarAsAssemblyGot pins the error path's report: the roots
// before the one that failed, and their shortfalls, are in it, and a
// non-schema RootAt root is ParseReport's own plain error.
func TestParseSetReportsAsFarAsAssemblyGot(t *testing.T) {
	docs := map[string]string{
		"a.xsd":   wrap("urn:a", `<xs:element name="a" type="xs:string"/>`),
		"foo.xml": `<foo/>`,
	}
	missing := parser.HintAt("urn:m", "missing.xsd")
	_, report, err := parseSet(t, []parser.Root{parser.RootAt("a.xsd"), missing, parser.RootAt("foo.xml"), parser.HintAt("urn:z", "after.xsd")}, docs)
	if err == nil {
		t.Fatal("ParseSet accepted a root whose element is not <schema>")
	}
	if want := `parser: <foo> at foo.xml:1:1 is not a <schema> document root: assembling a schema requires one`; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
	if got, want := locations(report), []string{"a.xsd"}; !slices.Equal(got, want) {
		t.Errorf("Documents() locations = %v, want %v", got, want)
	}
	if got, want := report.UnfollowedRoots(), []parser.Root{missing}; !slices.Equal(got, want) {
		t.Errorf("UnfollowedRoots() = %v, want %v — the hint after the failure was never reached", got, want)
	}
}

// TestParseSetEmptyAndNil pins the two caller faults: no roots is a plain
// error with an empty report, and a nil Root panics.
func TestParseSetEmptyAndNil(t *testing.T) {
	_, report, err := parseSet(t, nil, nil)
	if err == nil {
		t.Error("ParseSet(nil) returned no error")
	}
	if len(report.Documents()) != 0 || len(report.Unfollowed()) != 0 || len(report.UnfollowedRoots()) != 0 {
		t.Errorf("ParseSet(nil) report = %+v, want empty", report)
	}
	defer func() {
		if recover() == nil {
			t.Error("ParseSet with a nil Root did not panic")
		}
	}()
	_, _, _ = parser.ParseSet([]parser.Root{parser.RootAt("a.xsd"), nil})
}

// TestParseSetRootsShareOneAssembly pins what composing several roots into one
// assembly buys: two roots declaring one expanded name collide under
// sch-props-correct clause 2, while a root named twice, or a namespaced root
// hinted again under its own namespace, is composed once.
func TestParseSetRootsShareOneAssembly(t *testing.T) {
	docs := map[string]string{
		"a.xsd": wrap("urn:x", `<xs:element name="e" type="xs:string"/>`),
		"b.xsd": wrap("urn:x", `<xs:element name="e" type="xs:int"/>`),
	}
	_, _, err := parseSet(t, []parser.Root{parser.RootAt("a.xsd"), parser.RootAt("b.xsd")}, docs)
	var xe *xsderr.Error
	if !errors.As(err, &xe) || xe.Rule != "sch-props-correct" {
		t.Fatalf("two roots declaring {urn:x}e: error = %v, want sch-props-correct", err)
	}

	for _, roots := range [][]parser.Root{
		{parser.RootAt("a.xsd"), parser.RootAt("a.xsd")},
		{parser.RootAt("a.xsd"), parser.HintAt("urn:x", "a.xsd")},
		{parser.HintAt("urn:x", "a.xsd"), parser.RootAt("a.xsd")},
	} {
		_, report, err := parseSet(t, roots, docs)
		if err != nil {
			t.Errorf("ParseSet(%v): %v, want the repeat composed once", roots, err)
			continue
		}
		if got, want := locations(report), []string{"a.xsd"}; !slices.Equal(got, want) {
			t.Errorf("ParseSet(%v) Documents() = %v, want %v", roots, got, want)
		}
	}
}

// TestParseSetPlainRootAfterRedefineKeepsItsReading pins the dedup hit's merge:
// a document first reached as an <xs:redefine> target and then named as a plain
// root joins that discovery with a plain reading, which excepts nothing — so the
// original T it contributes collides with the redefinition, exactly as the
// same document reached plainly through an <xs:include> does (#1349). Dropping
// the plain reading would leave the redefine's exception in force alone and
// accept the set.
func TestParseSetPlainRootAfterRedefineKeepsItsReading(t *testing.T) {
	docs := map[string]string{
		"a.xsd": wrap("urn:x", `<xs:redefine schemaLocation="b.xsd">`+
			`<xs:simpleType name="T"><xs:restriction base="tns:T"><xs:maxLength value="3"/></xs:restriction></xs:simpleType>`+
			`</xs:redefine>`),
		"b.xsd": wrap("urn:x", `<xs:simpleType name="T"><xs:restriction base="xs:string"/></xs:simpleType>`),
	}
	if _, _, err := parseSet(t, []parser.Root{parser.RootAt("a.xsd")}, docs); err != nil {
		t.Fatalf("the redefinition alone: %v", err)
	}
	_, _, err := parseSet(t, []parser.Root{parser.RootAt("a.xsd"), parser.RootAt("b.xsd")}, docs)
	var xe *xsderr.Error
	if !errors.As(err, &xe) || xe.Rule != "sch-props-correct" {
		t.Fatalf("error = %v, want sch-props-correct: the plain root contributes the original T", err)
	}
}

// TestParseSetHintIsCheckedAsAnImport pins HintAt's src-import verdicts, each
// charged at the hinted document itself — the document a reader can open — and
// never recorded among UnfollowedRoots, since the location did resolve.
func TestParseSetHintIsCheckedAsAnImport(t *testing.T) {
	docs := map[string]string{
		"x.xsd":    wrap("urn:x", `<xs:element name="e" type="xs:string"/>`),
		"none.xsd": wrap("", `<xs:element name="n" type="xs:string"/>`),
		"junk.xsd": "<xs:schema xmlns:xs=\"http://www.w3.org/2001/XMLSchema\">\n<oops",
		"foo.xml":  `<foo/>`,
		"c.xsd":    wrap("urn:c", `<xs:include schemaLocation="none.xsd"/>`),
	}
	cases := []struct {
		name  string
		roots []parser.Root
		// at is where the verdict is charged; clause names the clause the message
		// cites.
		at     xsderr.Loc
		clause string
	}{
		{"namespace mis-paired", []parser.Root{parser.HintAt("urn:y", "x.xsd")}, xsderr.Loc{URI: "x.xsd", Line: 1, Col: 1}, "clause 3.1"},
		// c.xsd includes none.xsd as a chameleon, keying it under urn:c, so the
		// hint for urn:c lands on that discovery and must still fail clause 3.1
		// against none.xsd's own absent namespace (#275).
		{"mis-paired on a dedup hit", []parser.Root{parser.RootAt("c.xsd"), parser.HintAt("urn:c", "none.xsd")}, xsderr.Loc{URI: "none.xsd", Line: 1, Col: 1}, "clause 3.1"},
		{"absent namespace, namespaced document", []parser.Root{parser.HintAt("", "x.xsd")}, xsderr.Loc{URI: "x.xsd", Line: 1, Col: 1}, "clause 3.2"},
		{"not a schema", []parser.Root{parser.HintAt("urn:x", "foo.xml")}, xsderr.Loc{URI: "foo.xml", Line: 1, Col: 1}, "clause 2"},
		{"not well-formed", []parser.Root{parser.HintAt("urn:x", "junk.xsd")}, xsderr.Loc{URI: "junk.xsd", Line: 2}, "well-formed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, report, err := parseSet(t, c.roots, docs)
			var xe *xsderr.Error
			if !errors.As(err, &xe) || xe.Rule != "src-import" {
				t.Fatalf("error = %v, want src-import", err)
			}
			if xe.Loc.URI != c.at.URI || xe.Loc.Line != c.at.Line || (c.at.Col != 0 && xe.Loc.Col != c.at.Col) {
				t.Errorf("charged at %s, want %s", xe.Loc, c.at)
			}
			if !strings.Contains(xe.Msg, c.clause) || !strings.Contains(xe.Msg, "schema location hint") {
				t.Errorf("message = %q, want it to cite %s against the schema location hint", xe.Msg, c.clause)
			}
			if got := report.UnfollowedRoots(); len(got) != 0 {
				t.Errorf("UnfollowedRoots() = %v, want none: the hint resolved", got)
			}
		})
	}

	// An absent-namespace hint to a no-namespace document composes.
	if _, _, err := parseSet(t, []parser.Root{parser.HintAt("", "none.xsd")}, docs); err != nil {
		t.Errorf("absent-namespace hint to a no-namespace document: %v", err)
	}
}
