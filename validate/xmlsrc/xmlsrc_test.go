package xmlsrc

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

const fixtureURI = "instance.xml"

// fixture is the instance every round-trip assertion below is read against,
// by line and column: a default and a prefixed namespace declared on the
// document element, a prefixed and an unprefixed attribute, text broken into
// runs by a CDATA section and an entity reference, and mixed content.
const fixture = `<?xml version="1.0"?>
<r xmlns="urn:d" xmlns:p="urn:p" id="r1" p:lang="en">top
  <a>in <![CDATA[a]]>&amp;more</a>between
  <p:b><c/></p:b>
</r>
`

// fixtureRoot is the fixture's document element, the ·expanded name· the
// root dispatch resolves against the schema.
var fixtureRoot = xsd.QName{Space: "urn:d", Local: "r"}

// validatorFor is a validator over a schema declaring exactly roots, as
// top-level element declarations. A document whose root is among them is
// walked; one whose root is not is charged cvc-assess-elt before the walk
// begins, so a test that needs the walk to run must name its root here.
func validatorFor(t *testing.T, roots ...xsd.QName) *validate.Validator {
	t.Helper()
	b := xsd.NewSchemaBuilder()
	for _, name := range roots {
		e, err := xsd.NewElementDeclaration(xsderr.Loc{}, name, nil, nil, xsd.NewGlobalScope(),
			nil, false, nil, nil, nil, false, nil)
		if err != nil {
			t.Fatalf("building the %s element declaration: %v", name, err)
		}
		b.AddElement(e)
	}
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the schema: %v", err)
	}
	v, err := validate.New(schema, strict.New())
	if err != nil {
		t.Fatalf("validate.New: %v", err)
	}
	return v
}

func rootOf(t *testing.T, doc string) *element {
	t.Helper()
	w := newWalker(fixtureURI, strings.NewReader(doc))
	root, err := w.root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	return root
}

// trace drains e's subtree the way Assess does — the element, then its
// [[attributes]], then its [[children]] in order, recursively — into one
// line per information item, so document order and every item's Loc are
// asserted as a single expectation.
func trace(t *testing.T, e validate.Element) []string {
	t.Helper()
	out := []string{fmt.Sprintf("element %s @%s", e.Name(), e.Loc())}
	for _, a := range e.Attributes() {
		out = append(out, fmt.Sprintf("attr %s=%q @%s", a.Name(), a.Value(), a.Loc()))
	}
	kids := e.Children()
	for {
		c, ok := kids.Next()
		if !ok {
			break
		}
		if kid, ok := c.Element(); ok {
			out = append(out, trace(t, kid)...)
			continue
		}
		txt, ok := c.Text()
		if !ok {
			t.Fatal("Child holds neither arm")
		}
		out = append(out, fmt.Sprintf("text %q @%s", txt.Data(), txt.Loc()))
	}
	if err := kids.Err(); err != nil {
		t.Fatalf("draining the children of %s: %v", e.Name(), err)
	}
	return out
}

// wantTrace is the fixture's information items in document order. Every Loc
// is the item's own first byte in the fixture text — for a CDATA section,
// the "<" of its opener, for an attribute, the first byte of its name, prefix
// included (xmltree's Attribute.Loc).
var wantTrace = []string{
	`element {urn:d}r @instance.xml:2:1`,
	`attr id="r1" @instance.xml:2:34`,
	`attr {urn:p}lang="en" @instance.xml:2:42`,
	`text "top\n  " @instance.xml:2:54`,
	`element {urn:d}a @instance.xml:3:3`,
	`text "in " @instance.xml:3:6`,
	`text "a" @instance.xml:3:9`,
	`text "&more" @instance.xml:3:22`,
	`text "between\n  " @instance.xml:3:35`,
	`element {urn:p}b @instance.xml:4:3`,
	`element {urn:d}c @instance.xml:4:8`,
	`text "\n" @instance.xml:4:18`,
}

func TestFixtureRoundTripsInDocumentOrder(t *testing.T) {
	got := trace(t, rootOf(t, fixture))
	if len(got) != len(wantTrace) {
		t.Fatalf("traced %d items, want %d:\n%s", len(got), len(wantTrace), strings.Join(got, "\n"))
	}
	for i, want := range wantTrace {
		if got[i] != want {
			t.Errorf("item %d = %s, want %s", i, got[i], want)
		}
	}
}

// TestEveryLocResolvesInTheFixture reads each traced position back out of the
// fixture text: a Loc a diagnostic cites has to name a line and column a
// reader can actually open the file to.
func TestEveryLocResolvesInTheFixture(t *testing.T) {
	lines := strings.Split(fixture, "\n")
	for _, item := range trace(t, rootOf(t, fixture)) {
		at := strings.LastIndex(item, " @")
		var uri string
		var line, col int
		if _, err := fmt.Sscanf(item[at+2:], "%s", &uri); err != nil {
			t.Fatalf("%s: no location: %v", item, err)
		}
		parts := strings.Split(uri, ":")
		if len(parts) != 3 || parts[0] != fixtureURI {
			t.Fatalf("%s: location %q is not %s:line:col", item, uri, fixtureURI)
		}
		if _, err := fmt.Sscanf(parts[1]+" "+parts[2], "%d %d", &line, &col); err != nil {
			t.Fatalf("%s: unreadable line/column: %v", item, err)
		}
		if line < 1 || line > len(lines) {
			t.Errorf("%s: line %d is outside the fixture's %d lines", item, line, len(lines))
			continue
		}
		if col < 1 || col > len(lines[line-1])+1 {
			t.Errorf("%s: column %d is outside line %d (%d bytes)", item, col, line, len(lines[line-1]))
		}
	}
}

// TestNamespaceDeclarationsAreNotAttributes holds the adapter to
// validate.Element's split of Appendix D's [[attributes]] from its
// [[namespace attributes]]: a declaration is scope, reachable only through
// LookupPrefix, and never an attribute — an adapter that reported one would
// have the engine charge cvc-complex-type clause 2 against every namespaced
// document.
func TestNamespaceDeclarationsAreNotAttributes(t *testing.T) {
	root := rootOf(t, fixture)
	for _, a := range root.Attributes() {
		name := a.Name()
		if name.Local == "xmlns" || strings.HasPrefix(name.Local, "xmlns:") || name.Space == "xmlns" {
			t.Errorf("Attributes() reports the namespace declaration %s", name)
		}
	}
	if n := len(root.Attributes()); n != 2 {
		t.Errorf("Attributes() reports %d attributes, want the 2 that are not declarations", n)
	}
	for _, tc := range []struct {
		prefix string
		uri    string
		ok     bool
	}{
		{prefix: "", uri: "urn:d", ok: true},
		{prefix: "p", uri: "urn:p", ok: true},
		{prefix: "unbound"},
	} {
		uri, ok := root.LookupPrefix(tc.prefix)
		if uri != tc.uri || ok != tc.ok {
			t.Errorf("LookupPrefix(%q) = (%q,%v), want (%q,%v)", tc.prefix, uri, ok, tc.uri, tc.ok)
		}
	}
}

// TestUndescendedSubtreeIsNotReportedToItsParent drives the access pattern a
// processContents="skip" wildcard match produces: the engine takes an element
// child and never calls Children on it. The skipped subtree's tokens are
// still in the shared stream, and the parent's cursor must discard them
// rather than report them as its own further children.
func TestUndescendedSubtreeIsNotReportedToItsParent(t *testing.T) {
	const doc = `<r><skip a="1"><deep>x</deep>tail</skip>between<next>z</next></r>`
	kids := rootOf(t, doc).Children()

	skip, ok := kids.Next()
	if !ok {
		t.Fatal("Next() reported no first child")
	}
	skipped, ok := skip.Element()
	if !ok {
		t.Fatal("the first child is not an element")
	}
	if n := skipped.Name().Local; n != "skip" {
		t.Fatalf("the first child is %s, want skip", n)
	}
	// Everything the engine does to a skipped element, and nothing more: no
	// Children call, so its subtree is never pulled by its own cursor.
	if n := len(skipped.Attributes()); n != 1 {
		t.Fatalf("skip reports %d attributes, want 1", n)
	}

	want := []string{`text "between"`, `element next`}
	var got []string
	for {
		c, ok := kids.Next()
		if !ok {
			break
		}
		if e, ok := c.Element(); ok {
			got = append(got, "element "+e.Name().Local)
			if inner := trace(t, e); len(inner) != 2 {
				t.Errorf("draining %s yielded %v, want the element and its one text run", e.Name(), inner)
			}
			continue
		}
		txt, _ := c.Text()
		got = append(got, fmt.Sprintf("text %q", txt.Data()))
	}
	if err := kids.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("after the skipped subtree the parent saw %v, want %v", got, want)
	}
}

// TestCursorStaysExhausted pins the latch: a cursor that has consumed its own
// end tag must not pull again, or it takes the tokens belonging to the next
// sibling of the element it was reading.
func TestCursorStaysExhausted(t *testing.T) {
	root := rootOf(t, `<r><a>x</a><b/></r>`)
	kids := root.Children()
	first, ok := kids.Next()
	if !ok {
		t.Fatal("Next() reported no first child")
	}
	a, _ := first.Element()
	inner := a.Children()
	if _, ok := inner.Next(); !ok {
		t.Fatal("<a>'s cursor reported no text child")
	}
	for i := range 2 {
		if c, ok := inner.Next(); ok {
			t.Fatalf("<a>'s cursor yielded %+v on call %d past its end", c, i+1)
		}
	}
	if err := inner.Err(); err != nil {
		t.Errorf("Err() = %v, want nil at the true end of the children", err)
	}
	// The sibling the exhausted cursor could have stolen is still there.
	next, ok := kids.Next()
	if !ok {
		t.Fatal("the parent cursor lost <b>")
	}
	if e, _ := next.Element(); e.Name().Local != "b" {
		t.Errorf("the parent's next child is %s, want b", e.Name())
	}
}

// TestPrologCharDataIsDropped covers the depth-0 rule alone: white space
// read before the document element belongs to no element, so the root scan
// drops it rather than yielding it to one. Character data after the document
// element's end tag is read by Validate's drain, not the root scan, and is
// deliberately not in this fixture (TestValidateRejectsFaultsPastTheWalk).
func TestPrologCharDataIsDropped(t *testing.T) {
	root := rootOf(t, "\n \n<r>in</r>")
	got := trace(t, root)
	want := []string{`element r @instance.xml:3:1`, `text "in" @instance.xml:3:4`}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("trace = %v, want %v", got, want)
	}
}

// TestChildrenAfterTheWalkLeftTheElementPanics pins the cursor's other
// direction: an engine that descends too LATE is refused, not answered
// wrongly. A cursor keyed on depth alone, opened once the shared stream has
// moved past the element it belongs to, yields whatever now stands at that
// depth — the next sibling's children reported as this element's, or, past
// the document's last token, io.EOF latched into the Result of a well-formed
// document.
func TestChildrenAfterTheWalkLeftTheElementPanics(t *testing.T) {
	for _, tc := range []struct{ name, doc string }{
		{name: "the walk moved on to a sibling", doc: `<r><a><x/></a><b><y/></b></r>`},
		{name: "the walk reached the end of the document", doc: `<r><a><x/></a></r>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kids := rootOf(t, tc.doc).Children()
			first, ok := kids.Next()
			if !ok {
				t.Fatal("Next() reported no first child")
			}
			a, ok := first.Element()
			if !ok {
				t.Fatal("the first child is not an element")
			}
			// The parent advances, which correctly discards <a>'s undrained
			// subtree: after this the stream is past everything <a> owns.
			kids.Next()
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("Children returned a cursor over the tokens of some other element")
				}
				msg, ok := r.(string)
				if !ok || !strings.Contains(msg, "validate/xmlsrc: Children called after the walk left a") {
					t.Errorf("panicked with %v, want the contract violation naming <a>", r)
				}
			}()
			a.Children()
		})
	}
}

func TestValidateAssessesTheDocument(t *testing.T) {
	res, err := Validate(validatorFor(t, fixtureRoot), strings.NewReader(fixture), WithURI(fixtureURI))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Err() != nil {
		t.Errorf("Err() = %v, want nil for a document read to its end", res.Err())
	}
	if v := res.Violations(); v != nil {
		t.Errorf("Violations() = %v, want none for a declared root", v)
	}
}

// TestValidateWithoutURI covers the default: an unnamed document still
// assesses, and its locations render as xsderr.Loc's unknown-URI form.
func TestValidateWithoutURI(t *testing.T) {
	if _, err := Validate(validatorFor(t, fixtureRoot), strings.NewReader(fixture)); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	root := func() *element {
		w := newWalker("", strings.NewReader(fixture))
		root, err := w.root()
		if err != nil {
			t.Fatalf("root: %v", err)
		}
		return root
	}()
	if got := root.Loc().String(); got != "?:2:1" {
		t.Errorf("Loc() = %s, want ?:2:1", got)
	}
}

func TestValidateRejectsNilArguments(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    *validate.Validator
		r    io.Reader
	}{
		{name: "nil validator", r: strings.NewReader(fixture)},
		{name: "nil reader", v: validatorFor(t)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Validate(tc.v, tc.r)
			if err == nil {
				t.Fatal("Validate = nil error, want one")
			}
			if res != nil {
				t.Errorf("Validate returned a %T, want nil: nothing was assessed", res)
			}
			var xerr *xsderr.Error
			if errors.As(err, &xerr) {
				t.Errorf("Validate returned %T: a nil argument is not a verdict about a document", xerr)
			}
		})
	}
}

// TestValidateRejectsDocumentsWithNoRoot covers the second arm of the split:
// the walk never began, so the fault is the returned error and there is no
// Result to carry it.
func TestValidateRejectsDocumentsWithNoRoot(t *testing.T) {
	for _, tc := range []struct{ name, doc string }{
		{name: "empty", doc: ""},
		{name: "whitespace only", doc: "  \n  "},
		{name: "malformed before the root", doc: "</oops>"},
		{name: "text before the root", doc: "junk<r/>"},
		{name: "unbound prefix on the root", doc: `<p:r/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Validate(validatorFor(t), strings.NewReader(tc.doc), WithURI(fixtureURI))
			if err == nil {
				t.Fatal("Validate = nil error, want one")
			}
			if res != nil {
				t.Errorf("Validate returned a %T, want nil: nothing was assessed", res)
			}
			rule, ok := xsderr.RuleOf(err)
			if !ok || rule != xsderr.RuleXMLWellFormed {
				t.Errorf("error %v: rule = %q (ok=%v), want %q", err, rule, ok, xsderr.RuleXMLWellFormed)
			}
			if _, ok := xsderr.LocOf(err); !ok {
				t.Errorf("error %v carries no xsderr.Loc", err)
			}
		})
	}
}

// TestValidateRejectsFaultsPastTheWalk pins that Validate reads the stream to
// its end once the walk returns, and returns the fault it meets there, with no
// Result: after the document element only Misc may stand (XML 1.0 [1]
// document, [27] Misc), and a subtree the walk did not descend into — under a
// root with no declaration, which Assess charges before it reads a child — is
// read too. The last row is the control: Misc after the root reads clean.
func TestValidateRejectsFaultsPastTheWalk(t *testing.T) {
	declared := xsd.QName{Local: "r"}
	for _, tc := range []struct {
		name, doc string
		roots     []xsd.QName
		want      string // the error's opening; "" when the document reads clean
	}{
		{"text after the root", "<r/>junk", []xsd.QName{declared}, "instance.xml:1:5: [xml-wf] character data after the document element"},
		{"a second root", "<r><a/></r>\n<r/>", []xsd.QName{declared}, "instance.xml:2:1: [xml-wf] element <r> after the document element"},
		{"a second root after an undescended one", "<u><x/></u><b/>", nil, "instance.xml:1:12: [xml-wf] element <b> after the document element"},
		{"a fault in an undescended subtree", "<u><x></y></u>", nil, "instance.xml:1:7: [xml-wf] end tag </y> does not match open element x"},
		{"Misc after the root", "<r/>\n<!-- c -->\n<?pi?>\n", []xsd.QName{declared}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Validate(validatorFor(t, tc.roots...), strings.NewReader(tc.doc), WithURI(fixtureURI))
			if tc.want == "" {
				if err != nil || res.Err() != nil {
					t.Fatalf("Validate = %v, Err() = %v, want the document read clean", err, res.Err())
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate = nil error, want one opening %q", tc.want)
			}
			if !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to open %q", err, tc.want)
			}
			if res != nil {
				t.Errorf("Validate returned a %T, want nil: a document that is not well-formed has no assessment", res)
			}
		})
	}
}

// TestValidateReportsMidWalkFaultsInTheResult covers the other side of the
// split: the walk began, so the source fault is the Result's incompleteness
// and not Validate's error. The mismatched tag faults again on the token after
// it, so the row fails if Validate reads on past a walk that stopped.
func TestValidateReportsMidWalkFaultsInTheResult(t *testing.T) {
	for _, doc := range []string{"<r><a>text", "<r><a></b></r>"} {
		t.Run(doc, func(t *testing.T) {
			res, err := Validate(validatorFor(t, xsd.QName{Local: "r"}), strings.NewReader(doc), WithURI(fixtureURI))
			if err != nil {
				t.Fatalf("Validate = %v, want the fault in the Result alone", err)
			}
			if res.Err() == nil {
				t.Fatal("Err() = nil, want the document's fault")
			}
			rule, ok := xsderr.RuleOf(res.Err())
			if !ok || rule != xsderr.RuleXMLWellFormed {
				t.Errorf("Err() = %v: rule = %q (ok=%v), want %q", res.Err(), rule, ok, xsderr.RuleXMLWellFormed)
			}
		})
	}
}

// TestOneFaultIsLatchedForEveryCursor pins the single latch: the fault the
// deepest cursor met is the fault every cursor over that walk reports, so no
// cursor can claim a clean end after the stream has broken.
func TestOneFaultIsLatchedForEveryCursor(t *testing.T) {
	w := newWalker(fixtureURI, strings.NewReader("<r><a>text"))
	root, err := w.root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	outer := root.Children()
	c, ok := outer.Next()
	if !ok {
		t.Fatal("Next() reported no first child")
	}
	a, _ := c.Element()
	inner := a.Children()
	for {
		if _, ok := inner.Next(); !ok {
			break
		}
	}
	if inner.Err() == nil {
		t.Fatal("the inner cursor reports no fault after a truncated document")
	}
	if !errors.Is(outer.Err(), inner.Err()) {
		t.Errorf("outer Err() = %v, inner Err() = %v: one walk latches one fault", outer.Err(), inner.Err())
	}
	if _, ok := outer.Next(); ok {
		t.Error("the outer cursor kept reading after the walk's fault")
	}
}

// errRead is the error readFailure returns.
var errRead = errors.New("the source went away")

// readFailure is an io.Reader whose every Read fails with errRead.
type readFailure struct{}

func (readFailure) Read([]byte) (int, error) { return 0, errRead }

// failingReader yields text and then fails with errRead, so the read fault
// lands wherever text leaves the document standing.
func failingReader(text string) io.Reader {
	return io.MultiReader(strings.NewReader(text), readFailure{})
}

// skipCountSchema declares r, whose children a skip wildcard admits and whose
// one assertion counts the elements below it, so a fault inside a ·skipped·
// child reaches Result.Err through the assertion ancestry's tally of that
// subtree rather than through the walk's own reading of [[children]].
const skipCountSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="r">
    <xs:complexType>
      <xs:sequence><xs:any processContents="skip" minOccurs="0" maxOccurs="unbounded"/></xs:sequence>
      <xs:assert test="count(.//y) ge 0"/>
    </xs:complexType>
  </xs:element>
</xs:schema>`

// TestEverySourceFaultCarriesTheWellFormedRule pins the claim Validate's doc
// makes: every *xsderr.Error it returns, and every Result.Err it leaves,
// carries xsderr.RuleXMLWellFormed, reached through errors.As and
// xsderr.RuleOf, and a fault reading r reaches the reader's own error through
// errors.Is on either channel. The rows cover a fault before the root, after
// the walk, in a subtree the walk did not descend into, in the walk itself, and
// in a ·skipped· subtree a counting assertion tallies.
func TestEverySourceFaultCarriesTheWellFormedRule(t *testing.T) {
	backend := strict.New()
	schema, err := parser.Parse("main.xsd",
		parser.WithBackend(backend),
		parser.WithResolver(loader.Map(map[string]string{"main.xsd": skipCountSchema})))
	if err != nil {
		t.Fatalf("parser.Parse: %v", err)
	}
	v, err := validate.New(schema, backend)
	if err != nil {
		t.Fatalf("validate.New: %v", err)
	}
	for _, tc := range []struct {
		name      string
		r         io.Reader
		inResult  bool // the fault is Result.Err, not the returned error
		readFault bool // the fault is errRead's, which errors.Is must reach
	}{
		{name: "malformed before the root", r: strings.NewReader("</oops>")},
		{name: "a second root", r: strings.NewReader("<r/><r/>")},
		{name: "a fault in an undescended subtree", r: strings.NewReader("<u><x></y></u>")},
		{name: "a read fault before the root", r: failingReader("<?xml version='1.0'?>"), readFault: true},
		{name: "an unclosed child", r: strings.NewReader("<r><x>"), inResult: true},
		{name: "a mismatched tag in a skipped subtree", r: strings.NewReader("<r><x><y></x></r>"), inResult: true},
		{name: "an unbound prefix in a skipped subtree", r: strings.NewReader("<r><x><p:y/></x></r>"), inResult: true},
		{name: "a read fault in a skipped subtree", r: failingReader("<r><x><y>"), inResult: true, readFault: true},
		{name: "a read fault under the root", r: failingReader("<r>"), inResult: true, readFault: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Validate(v, tc.r, WithURI(fixtureURI))
			fault := err
			if tc.inResult {
				if err != nil {
					t.Fatalf("Validate = %v, want the fault in the Result alone", err)
				}
				fault = res.Err()
			}
			if fault == nil {
				t.Fatalf("no fault on the channel the row names (inResult=%v)", tc.inResult)
			}
			var xerr *xsderr.Error
			if !errors.As(fault, &xerr) {
				t.Fatalf("fault %v: errors.As reaches no *xsderr.Error", fault)
			}
			if rule, ok := xsderr.RuleOf(fault); !ok || rule != xsderr.RuleXMLWellFormed {
				t.Errorf("fault %v: rule = %q (ok=%v), want %q", fault, rule, ok, xsderr.RuleXMLWellFormed)
			}
			if got := errors.Is(fault, errRead); got != tc.readFault {
				t.Errorf("fault %v: errors.Is(fault, errRead) = %v, want %v", fault, got, tc.readFault)
			}
		})
	}
}
