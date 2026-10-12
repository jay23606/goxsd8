package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// orderSummary is the whole stdout block `goxsd8 parse testdata/order.xsd`
// prints, pinned byte for byte. It is what makes "print a summary" a contract:
// the namespace line, one count per §3.17.1 property the schema document
// declares into, and their total. The type count is 2 and not 54 — the seeded
// built-in datatypes carry no source position and are not the schema's own
// declarations (see declaredNames).
const orderSummary = `testdata/order.xsd
  namespace: http://example.com/order
  types: 2
  elements: 1
  attributes: 1
  attribute groups: 1
  model groups: 1
  notations: 1
  identity constraints: 1
  components: 8
`

// importsSummary is the whole stdout block `goxsd8 parse testdata/imports.xsd`
// prints. Its two namespace lines are what makes namespacesOf's ordering
// observable at all: with one namespace the order slice never holds two
// entries, so ranging the seen map instead would print the same bytes.
//
// The order is first appearance over the buckets, which is the root document's
// namespace and then the imported one — not sorted, which would put
// .../imported first.
const importsSummary = `testdata/imports.xsd
  namespace: http://example.com/root
  namespace: http://example.com/imported
  types: 0
  elements: 2
  attributes: 0
  attribute groups: 0
  model groups: 0
  notations: 0
  identity constraints: 0
  components: 2
`

// noComponentsSummary and includedNamespaceSummary are the two halves of the
// namespace derivation's scope, each the whole stdout block below the header
// its fixture echoes from a temporary path. They differ by one <xs:include>
// and disagree about the namespace line, which is what makes the derivation
// observably a property of the compilation rather than of the argument
// document. Both are pinned as blocks, like orderSummary and importsSummary,
// so a summary that regressed to printing nothing fails here too.
const noComponentsSummary = `  types: 0
  elements: 0
  attributes: 0
  attribute groups: 0
  model groups: 0
  notations: 0
  identity constraints: 0
  components: 0
`

const includedNamespaceSummary = `  namespace: http://example.com/ns
  types: 0
  elements: 1
  attributes: 0
  attribute groups: 0
  model groups: 0
  notations: 0
  identity constraints: 0
  components: 1
`

// brokenRule is the src-resolve charge testdata/broken.xsd earns. The line is
// asserted by its shape and this substring rather than in full: the message is
// the parser's and the location is absolute, so pinning either here would make
// this test a copy of parser's wording.
const brokenRule = "[src-resolve]"

// malformedRule is the charge testdata/malformed.xsd earns. Unlike broken.xsd's
// src-resolve, the parser returns this one WRAPPED in the assembly context it
// was read under, so it is the fixture that reaches violationLine's unwrap.
const malformedRule = "[xml-wf]"

func TestParseSummary(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"parse", "testdata/order.xsd"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("parse of a valid schema = %d, want 0 (stderr %q)", code, stderr.String())
	}
	if stdout.String() != orderSummary {
		t.Errorf("summary =\n%s\nwant\n%s", stdout.String(), orderSummary)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

// TestParseNamespaceOrder pins the one order the summary derives rather than
// reads off an enumeration: namespacesOf's first-appearance order over the
// buckets. A two-namespace set is what makes it assertable — with a single
// namespace, ranging the seen map produces the same bytes.
func TestParseNamespaceOrder(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"parse", "testdata/imports.xsd"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("parse of an importing schema = %d, want 0 (stderr %q)", code, stderr.String())
	}
	if stdout.String() != importsSummary {
		t.Errorf("summary =\n%s\nwant\n%s", stdout.String(), importsSummary)
	}
}

// TestParseSummaryIsDeterministic is the acceptance bar, not a nicety (STYLE
// D1/D2): the same input must produce the same bytes, run after run. A summary
// assembled by ranging a map would pass TestParseSummary most of the time and
// fail here — but only on the importing fixture, whose set carries the two
// namespaces map iteration has something to permute between.
func TestParseSummaryIsDeterministic(t *testing.T) {
	for _, location := range []string{"testdata/order.xsd", "testdata/imports.xsd"} {
		var first bytes.Buffer
		if code := run([]string{"parse", location}, &first, &bytes.Buffer{}); code != 0 {
			t.Fatalf("parse %s = %d, want 0", location, code)
		}
		for i := range 20 {
			var again bytes.Buffer
			if code := run([]string{"parse", location}, &again, &bytes.Buffer{}); code != 0 {
				t.Fatalf("parse %s = %d, want 0", location, code)
			}
			if again.String() != first.String() {
				t.Fatalf("%s run %d differs from run 0:\n%s\nvs\n%s", location, i+1, again.String(), first.String())
			}
		}
	}
}

// TestParseSchemaErrors pins the failure contract: exit 1, nothing on stdout,
// and the rejected schema's first error on stderr in xsderr.Error's own
// rendering — a location a reader can open, the rule ID in brackets, then the
// message. Assembly stops at that error, so one rejected argument is one error
// line.
func TestParseSchemaErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"parse", "testdata/broken.xsd"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("parse of a rejected schema = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty — a rejected schema has no summary", stdout.String())
	}
	line := strings.TrimSuffix(stderr.String(), "\n")
	if strings.Contains(line, "\n") {
		t.Errorf("stderr = %q, want one line", stderr.String())
	}
	if !strings.Contains(line, brokenRule) {
		t.Errorf("stderr = %q, want the charged rule in brackets", line)
	}
	abs, err := filepath.Abs("testdata/broken.xsd")
	if err != nil {
		t.Fatal(err)
	}
	// The location must be openable from anywhere, and 1-indexed: the
	// offending element sits on line 9 of the fixture.
	if !strings.HasPrefix(line, abs+":9:") {
		t.Errorf("stderr = %q, want it to open with %q", line, abs+":9:")
	}
}

// TestParseWrappedSchemaError pins violationLine's unwrap, which no
// src-resolve fixture can reach: the parser returns that class unwrapped.
// A document that is not well-formed XML comes back WRAPPED in the assembly
// context it was read under, and the contract's first field is a location a
// reader can open, not the package that did the reading. Deleting the
// errors.As branch fails here and nowhere else.
func TestParseWrappedSchemaError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"parse", "testdata/malformed.xsd"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("parse of a malformed document = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty — a rejected schema has no summary", stdout.String())
	}
	line := strings.TrimSuffix(stderr.String(), "\n")
	if strings.Contains(line, "\n") {
		t.Errorf("stderr = %q, want one line", stderr.String())
	}
	if !strings.Contains(line, malformedRule) {
		t.Errorf("stderr = %q, want the charged rule in brackets", line)
	}
	abs, err := filepath.Abs("testdata/malformed.xsd")
	if err != nil {
		t.Fatal(err)
	}
	// The unclosed start tag runs to line 9 of the fixture.
	if !strings.HasPrefix(line, abs+":9:") {
		t.Errorf("stderr = %q, want it to open with %q", line, abs+":9:")
	}
	if strings.HasPrefix(line, "parser:") {
		t.Errorf("stderr = %q, want xsderr.Error's own rendering, unwrapped", line)
	}
}

// TestParseS4SGrammarRejectionCarriesNoRule pins the exception the contract
// states beside the "<loc>: [<rule>] <message>" shape (#1313): a document not
// valid against the schema for schema documents is rejected with no rule to
// cite (xsderr/doc.go), so its line is the producing component's own message
// and its location sits inside the sentence rather than as the <loc>: prefix.
//
// Both fixtures are asserted in the one test, because the claim is a
// DIFFERENCE between two rejections that both exit 1 with one stderr line —
// asserting the no-rule shape alone would pass against a renderer that had
// dropped brackets from every line.
//
// The opening "parser: <redefine> at <loc>" is pinned as a prefix, which is
// what makes the assertion fail on a message whose subject or location moved
// rather than merely on one missing a bracket (#1048).
func TestParseS4SGrammarRejectionCarriesNoRule(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"parse", "testdata/s4s-redefine.xsd"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("parse of a document invalid against the schema for schema documents = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty — a rejected schema has no summary", stdout.String())
	}
	line := strings.TrimSuffix(stderr.String(), "\n")
	if strings.Contains(line, "\n") {
		t.Errorf("stderr = %q, want one error line", stderr.String())
	}
	if strings.Contains(line, "[") {
		t.Errorf("stderr = %q, want no [<rule>]: the spec catalogs none for this class, and inventing one would read as a citation", line)
	}
	abs, err := filepath.Abs("testdata/s4s-redefine.xsd")
	if err != nil {
		t.Fatal(err)
	}
	// The <xs:redefine> sits on line 10 of the fixture, and its location is
	// carried INSIDE the sentence: a line opening with abs+":" would be the
	// contract's <loc>: prefix, which this class does not print.
	if !strings.HasPrefix(line, "parser: <redefine> at "+abs+":10:") {
		t.Errorf("stderr = %q, want it to open with %q", line, "parser: <redefine> at "+abs+":10:")
	}

	// The rejection that DOES carry a rule, for the difference: same exit
	// code, same single line, and both halves of the published shape.
	var ruledOut, ruledErr bytes.Buffer
	if code := run([]string{"parse", "testdata/broken.xsd"}, &ruledOut, &ruledErr); code != 1 {
		t.Fatalf("parse of a rejected schema = %d, want 1", code)
	}
	ruled := strings.TrimSuffix(ruledErr.String(), "\n")
	if !strings.Contains(ruled, brokenRule) {
		t.Errorf("stderr = %q, want the charged rule in brackets", ruled)
	}
	brokenAbs, err := filepath.Abs("testdata/broken.xsd")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ruled, brokenAbs+":") {
		t.Errorf("stderr = %q, want the <loc>: prefix the s4s-grammar line does not print", ruled)
	}
}

// TestParseNonSchemaRootCarriesNoRule pins the second member of the no-rule-ID
// class the contract names beside the "<loc>: [<rule>] <message>" shape
// (#1313): a well-formed document whose root is not <xs:schema> is a caller
// precondition fault rather than a schema-validity verdict (parser/produce.go),
// so no rule governs it and its line is the producing component's own message —
// the likeliest operator mistake, pointing parse at an instance document.
//
// It is exit 1 and not exit 2: rootLocation has already proved the file
// readable, so the rejection is a verdict about what the document IS.
//
// The whole message is pinned, not a bracket-free shape: it carries its
// location as an s4s-grammar message does, <file>:<line>:<col> inside the
// sentence, so an assertion that stopped at "no [" would pass against a line
// that had lost its subject, its file or its position (#1048).
func TestParseNonSchemaRootCarriesNoRule(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"parse", "testdata/notschema.xml"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("parse of a document whose root is not <xs:schema> = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty — a rejected schema has no summary", stdout.String())
	}
	line := strings.TrimSuffix(stderr.String(), "\n")
	if strings.Contains(line, "\n") {
		t.Errorf("stderr = %q, want one error line", stderr.String())
	}
	if strings.Contains(line, "[") {
		t.Errorf("stderr = %q, want no [<rule>]: no rule governs the document handed to a compiler, and inventing one would read as a citation", line)
	}
	abs, err := filepath.Abs("testdata/notschema.xml")
	if err != nil {
		t.Fatal(err)
	}
	// The location is inside the sentence: a line opening with abs+":" would
	// be the contract's <loc>: prefix, which this rejection does not print.
	want := "parser: <foo> at " + abs + ":5:1 is not a <schema> document root: assembling a schema requires one"
	if line != want {
		t.Errorf("stderr = %q, want %q", line, want)
	}
}

// TestParseIOFaultOnReferencedDocumentCarriesNoRule pins the third kind
// violationLine's own-message branch admits (#1354), and the one that is no spec
// class at all: an I/O fault reading a document the argument REFERENCES.
// parser's fetch wraps every resolver error other than loader.ErrNotFound in
// plain assembly context, every hop back returns it unwrapped, and it reaches
// stderr with no rule charged. Removing that member from the doc, or closing
// the enumeration around the two rejections again, leaves this unaccounted
// for.
//
// The trigger is a non-directory path segment — an <xs:include> whose
// schemaLocation runs through the including document itself as if it were a
// directory, so os.Open answers ENOTDIR. A permission bit would not do: this
// suite runs as root often enough that a mode-0000 fixture opens anyway and
// the test would pass without ever entering the branch. The probe below is
// what makes the trigger's PLATFORM assumption explicit rather than assumed:
// a system that answers a path-through-a-file with ENOENT maps it to
// loader.ErrNotFound, which is the legal-skip arm and a different test.
//
// Two stderr lines, not one, and the order is the contract's: reportUnfollowed
// names the shortfall first (fetch records UnfollowedLocationUnresolved before
// returning), then the violation line. It is the LAST line that is pinned.
//
// Exit 1, not 2 — the code the contract reserves for a schema verdict, though
// nothing here was decided about the schema. #1419 ruled it stays 1, and the
// contract now says so beside the legal skip it is not;
// TestValidateIOFaultOnReferencedDocumentIsASchemaFault pins validate's 3.
//
// The pin stops where the operating system's own words begin. Everything
// through `under "<root>": ` is goxsd8's, and pinning it as a PREFIX pins the
// subject, both wrapper layers and the location together, so a line that had
// lost its directive or its position could not pass (#1048).
func TestParseIOFaultOnReferencedDocumentCarriesNoRule(t *testing.T) {
	abs, err := filepath.Abs("testdata/notdir-include.xsd")
	if err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(abs, "inner.xsd")
	probe, probeErr := os.Open(inner)
	if probeErr == nil {
		_ = probe.Close()
	}
	if probeErr == nil || os.IsNotExist(probeErr) {
		t.Skipf("open %q = %v, want a non-not-exist error: this platform does not answer a path through a regular file with ENOTDIR", inner, probeErr)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"parse", "testdata/notdir-include.xsd"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("parse of a schema whose <xs:include> cannot be opened = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty — a rejected assembly has no summary", stdout.String())
	}
	lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("stderr = %q, want two lines: the unfollowed-directive shortfall, then the violation", stderr.String())
	}
	if !strings.Contains(lines[0], "this schemaLocation resolved to no document") {
		t.Errorf("stderr line 1 = %q, want reportUnfollowed's shortfall line", lines[0])
	}
	line := lines[1]
	if strings.Contains(line, "[") {
		t.Errorf("stderr = %q, want no [<rule>]: no XSD rule governs a document that could not be read, and inventing one would read as a citation", line)
	}
	// The location is inside the sentence: a line opening with abs+":" would
	// be the contract's <loc>: prefix, which this fault does not print.
	prefix := fmt.Sprintf("parser: resolving <include> schemaLocation %q at %s:12:3: loader: opening %q under %q: ",
		inner, abs, inner, filesystemRoot(abs))
	if !strings.HasPrefix(line, prefix) {
		t.Errorf("stderr = %q, want prefix %q", line, prefix)
	}
	if len(line) <= len(prefix) {
		t.Errorf("stderr = %q, want the operating system's own words after %q", line, prefix)
	}
}

// TestParseNamesAnUnresolvedDirective is #1260's acceptance: a schema argument
// whose own directive names a document that is not there compiles — src-include
// clause 2.4 makes the skip legal — so nothing about the summary or the exit
// code moves, and the shortfall behind them is named on stderr rather than
// reachable only behind -v.
//
// Both fixtures are the NON-ERROR class. An EMPTY <xs:redefine> keeps
// <xs:include>'s skip, src-redefine clause 1 being antecedent on children other
// than <annotation>; a NON-EMPTY one whose location does not resolve violates
// that clause and is a rejected schema — the exit-1 path parseOne already had,
// which this reporting leaves untouched.
func TestParseNamesAnUnresolvedDirective(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
	}{
		{"include", "testdata/unresolved-include.xsd"},
		{"empty redefine", "testdata/unresolved-empty-redefine.xsd"},
	}
	for _, c := range cases {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"parse", c.fixture}, &stdout, &stderr); code != exitOK {
			t.Errorf("%s: parse = %d, want %d — an unresolved schemaLocation is not an error", c.name, code, exitOK)
		}
		if want := "  elements: 1\n"; !strings.Contains(stdout.String(), want) {
			t.Errorf("%s: stdout =\n%s\nwant %q — the summary is unchanged", c.name, stdout.String(), want)
		}
		line := strings.TrimSuffix(stderr.String(), "\n")
		if strings.Contains(line, "\n") {
			t.Errorf("%s: stderr = %q, want one line — the fixture carries one directive", c.name, stderr.String())
		}
		abs, err := filepath.Abs(c.fixture)
		if err != nil {
			t.Fatal(err)
		}
		// The directive's own position, which is the whole point of reporting
		// it: an operator must be able to open the document and find the line.
		// Both fixtures carry theirs on line 10.
		if !strings.Contains(line, abs+":10:3:") {
			t.Errorf("%s: stderr = %q, want the directive's location %q", c.name, line, abs+":10:3:")
		}
		// No rule ID, in the brackets a charge is rendered with: the skip is
		// legal, so a script keying on "[<rule>]" must not read it as one.
		if strings.Contains(line, "[") {
			t.Errorf("%s: stderr = %q, want no rule brackets — this is not a violation", c.name, line)
		}
	}
}

// shortBrokenSchema carries #1312's shape: the same unresolved <xs:include>
// the fixtures above carry, in a document that also earns brokenRule.
const shortBrokenSchema = "testdata/unresolved-include-broken.xsd"

// TestParseNamesAnUnresolvedDirectiveOfARejectedSchema is #1312's ruling —
// option (a), report and then return: the shortfall is a fact about the
// assembly the error came out of, so a schema that fails to compile is named
// for it exactly as one that compiles is. It was dropped before, because
// reportUnfollowed stood after the error return, which withheld the line where
// an operator reading a rejection most needs it. reportUnfollowed's own doc
// comment states why the wording differs by outcome; this test does not
// restate it.
func TestParseNamesAnUnresolvedDirectiveOfARejectedSchema(t *testing.T) {
	abs, err := filepath.Abs(shortBrokenSchema)
	if err != nil {
		t.Fatal(err)
	}
	note := "goxsd8: parse: " + abs + ":10:3: this schemaLocation resolved to no document; the rejected assembly is short of whatever that document declares\n"

	var stdout, stderr bytes.Buffer
	if code := run([]string{"parse", shortBrokenSchema}, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("parse = %d, want %d — the schema is rejected (stderr %q)", code, exitInvalid, stderr.String())
	}
	if !strings.Contains(stderr.String(), note) {
		t.Errorf("stderr =\n%s\nwant the note %q", stderr.String(), note)
	}
	if !strings.Contains(stderr.String(), brokenRule) {
		t.Errorf("stderr =\n%s\nwant the error line too, charging %s", stderr.String(), brokenRule)
	}
	if strings.Contains(stderr.String(), "compiled schema") {
		t.Errorf("stderr =\n%s\nwant no line calling this a compiled schema", stderr.String())
	}
	// The shortfall stands above the verdict it may or may not explain, which
	// is the order both subcommands report in.
	if strings.Index(stderr.String(), note) > strings.Index(stderr.String(), brokenRule) {
		t.Errorf("stderr =\n%s\nwant the note before the error line", stderr.String())
	}

	// Per-argument independence, which the ruling leaves untouched: each
	// argument is its own parseOne, so the compiling one keeps its own note AND
	// its own wording beside the rejected one's.
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"parse", shortSchema, shortBrokenSchema}, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("two arguments: parse = %d, want %d (stderr %q)", code, exitInvalid, stderr.String())
	}
	shortAbs, err := filepath.Abs(shortSchema)
	if err != nil {
		t.Fatal(err)
	}
	compiledNote := "goxsd8: parse: " + shortAbs + ":10:3: this schemaLocation resolved to no document, which is legal and skipped; the compiled schema is short of whatever that document declares\n"
	if !strings.Contains(stderr.String(), compiledNote) {
		t.Errorf("stderr =\n%s\nwant the compiling argument's own note %q", stderr.String(), compiledNote)
	}
	if !strings.Contains(stderr.String(), note) {
		t.Errorf("stderr =\n%s\nwant the rejected argument's note %q", stderr.String(), note)
	}
}

// TestParseQuietDoesNotSuppressAnUnresolvedDirective holds the new line to
// -q's published scope: -q suppresses the summary, which is parse's
// informational output, and never a diagnosis. A gate spelled
// `goxsd8 parse -q` is the case #1260 was filed from, so a -q that swallowed
// this would leave the silence exactly where it was found.
func TestParseQuietDoesNotSuppressAnUnresolvedDirective(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"parse", "-q", "testdata/unresolved-include.xsd"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("parse -q = %d, want %d (stderr %q)", code, exitOK, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("parse -q stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "resolved to no document") {
		t.Errorf("parse -q stderr = %q, want the unfollowed directive named", stderr.String())
	}
}

// TestParseSaysNothingOfABareImport pins the other side of the line #1260
// draws: an <xs:import> with no schemaLocation names no document to have
// failed to reach — §4.2.6.2 makes it the spelling for "references into this
// namespace are expected", which another document of a set may supply — so
// reporting it would charge a complete assembly with a shortfall.
func TestParseSaysNothingOfABareImport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bare.xsd")
	body := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://example.com/ns">` +
		`<xs:import namespace="http://example.com/other"/><xs:element name="a" type="xs:string"/></xs:schema>`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"parse", path}, &stdout, &stderr); code != exitOK {
		t.Fatalf("parse = %d, want %d (stderr %q)", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty — a bare <xs:import> is not a shortfall", stderr.String())
	}
}

// TestParseQuiet pins -q's scope, which binds validate too: it suppresses the
// summary and nothing else. A -q that swallowed the error lines would break
// every grep-based script, which is why the second half of this test is the
// load-bearing one.
func TestParseQuiet(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"parse", "-q", "testdata/order.xsd"}, &stdout, &stderr); code != 0 {
		t.Errorf("parse -q of a valid schema = %d, want 0 (stderr %q)", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("parse -q stdout = %q, want empty", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"parse", "-q", "testdata/broken.xsd"}, &stdout, &stderr); code != 1 {
		t.Errorf("parse -q of a rejected schema = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), brokenRule) {
		t.Errorf("parse -q stderr = %q, want the error line — -q never silences a diagnosis", stderr.String())
	}
}

// TestParseVerbose pins the other half of STYLE L1: silent by default, and -v
// is the whole of the injection. The parser logs under its own group, so any
// output at all proves the logger reached parser.WithLogger.
func TestParseVerbose(t *testing.T) {
	var quietErr bytes.Buffer
	run([]string{"parse", "testdata/order.xsd"}, &bytes.Buffer{}, &quietErr)
	if quietErr.Len() != 0 {
		t.Errorf("parse without -v wrote %q to stderr, want silence", quietErr.String())
	}
	var loudErr bytes.Buffer
	run([]string{"parse", "-v", "testdata/order.xsd"}, &bytes.Buffer{}, &loudErr)
	if loudErr.Len() == 0 {
		t.Error("parse -v wrote nothing to stderr, want the parser's debug log")
	}
}

// TestParseMultipleSchemas pins the multi-schema decision: one run per
// argument, in argument order, every argument reported however the one before
// it went, and the worst outcome as the exit code.
func TestParseMultipleSchemas(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"parse", "testdata/order.xsd", "testdata/broken.xsd", "testdata/order.xsd"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("parse of a valid and a rejected schema = %d, want 1", code)
	}
	if stdout.String() != orderSummary+orderSummary {
		t.Errorf("stdout =\n%s\nwant the valid schema's summary twice", stdout.String())
	}
	if !strings.Contains(stderr.String(), brokenRule) {
		t.Errorf("stderr = %q, want the rejected schema's error", stderr.String())
	}

	// An unreadable argument outranks a schema verdict: 2 says no verdict was
	// reached, and it must not be masked by the 1 an earlier argument earned.
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"parse", "testdata/broken.xsd", "testdata/nosuch.xsd"}, &stdout, &stderr); code != 2 {
		t.Errorf("parse of a rejected and an unreadable schema = %d, want 2", code)
	}
}

// TestParseUsageErrors pins the exit-2 half of the split this issue owns: a
// fault in the command line or in reading a file is never a verdict about a
// schema. Each answer is distinct, lands on stderr with helpPointer under it,
// and leaves stdout empty.
func TestParseUsageErrors(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no schema", []string{"parse"}, "goxsd8: parse: no schema given"},
		{"missing file", []string{"parse", "testdata/nosuch.xsd"}, "no such file or directory"},
		{"a directory", []string{"parse", dir}, "is a directory"},
		{"undefined flag", []string{"parse", "-bogus", "testdata/order.xsd"}, "flag provided but not defined: -bogus"},
		// -help=true is not one of the three help spellings, at any position
		// (doc.go's argument vocabulary); after a subcommand it is a flag
		// whose value that subcommand does not accept.
		{"help as a flag value", []string{"parse", "-help=true"}, fmt.Sprintf(helpNotAFlagValueFmt, "parse")},
	}
	seen := make(map[string]string)
	for _, c := range cases {
		var stdout, stderr bytes.Buffer
		if code := run(c.args, &stdout, &stderr); code != 2 {
			t.Errorf("%s: run(%q) = %d, want 2", c.name, c.args, code)
		}
		if stdout.Len() != 0 {
			t.Errorf("%s: stdout = %q, want empty", c.name, stdout.String())
		}
		if !strings.Contains(stderr.String(), c.want) {
			t.Errorf("%s: stderr = %q, want it to contain %q", c.name, stderr.String(), c.want)
		}
		if !strings.HasSuffix(stderr.String(), helpPointer+"\n") {
			t.Errorf("%s: stderr = %q, want helpPointer under it", c.name, stderr.String())
		}
		if prior, dup := seen[stderr.String()]; dup {
			t.Errorf("%s answers exactly what %s answers: %q", c.name, prior, stderr.String())
		}
		seen[stderr.String()] = c.name
	}
}

// TestParseFaultsAreDistinguishable is the cliuser criterion this issue was
// filed to satisfy: a script must be able to tell "your command line is wrong"
// from "that subcommand does nothing yet" from "your schema is invalid",
// without parsing prose. The exit code separates the last from the first two,
// and the stderr line separates those two from each other.
func TestParseFaultsAreDistinguishable(t *testing.T) {
	kinds := []struct {
		name string
		args []string
		code int
	}{
		{"no schema argument", []string{"parse"}, 2},
		{"unreadable schema", []string{"parse", "testdata/nosuch.xsd"}, 2},
		{"reserved subcommand", []string{"validate", "a.xml"}, 2},
		{"invalid schema", []string{"parse", "testdata/broken.xsd"}, 1},
		{"valid schema", []string{"parse", "testdata/order.xsd"}, 0},
	}
	seen := make(map[string]string)
	for _, k := range kinds {
		var stdout, stderr bytes.Buffer
		if code := run(k.args, &stdout, &stderr); code != k.code {
			t.Errorf("%s: run(%q) = %d, want %d", k.name, k.args, code, k.code)
		}
		if prior, dup := seen[stderr.String()]; dup {
			t.Errorf("%s is byte-identical on stderr to %s: %q", k.name, prior, stderr.String())
		}
		seen[stderr.String()] = k.name
	}
}

// TestParsePathSpellings pins what the resolver rooting buys: an argument
// spelled absolutely, or reaching out of the working directory through "..",
// names the same schema as the relative spelling and compiles the same.
func TestParsePathSpellings(t *testing.T) {
	abs, err := filepath.Abs("testdata/order.xsd")
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// The same file reached by climbing out of the working directory and back
	// in: loader.Dir's default confinement to "." would refuse this spelling.
	climbing := filepath.Join("..", filepath.Base(cwd), "testdata", "order.xsd")
	for _, given := range []string{abs, climbing} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"parse", given}, &stdout, &stderr); code != 0 {
			t.Errorf("parse %q = %d, want 0 (stderr %q)", given, code, stderr.String())
		}
		// The header echoes the argument as spelled; the counts do not move.
		want := given + strings.TrimPrefix(orderSummary, "testdata/order.xsd")
		if stdout.String() != want {
			t.Errorf("parse %q stdout =\n%s\nwant\n%s", given, stdout.String(), want)
		}
	}
}

// TestParseIncludeResolvesAgainstItsOwnDocument pins the other half of the
// rooting: a relative schemaLocation inside a schema is served from that
// schema's directory, not from the process's working directory.
func TestParseIncludeResolvesAgainstItsOwnDocument(t *testing.T) {
	dir := t.TempDir()
	const ns = `xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://example.com/inc"`
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("common.xsd", `<xs:schema `+ns+`><xs:element name="shared" type="xs:string"/></xs:schema>`)
	write("main.xsd", `<xs:schema `+ns+`><xs:include schemaLocation="common.xsd"/><xs:element name="root" type="xs:string"/></xs:schema>`)

	main := filepath.Join(dir, "main.xsd")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"parse", main}, &stdout, &stderr); code != 0 {
		t.Fatalf("parse %q = %d, want 0 (stderr %q)", main, code, stderr.String())
	}
	// Two elements only if the <include> resolved: main.xsd declares one.
	if want := "  elements: 2\n"; !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout =\n%s\nwant %q — the include did not compose", stdout.String(), want)
	}
}

// TestParseAbsentNamespace pins the summary's rendering of the ·absent· target
// namespace (§2.2), which a QName carries as an empty Space: a no-namespace
// schema gets a namespace line like any other rather than a blank one.
func TestParseAbsentNamespace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nons.xsd")
	body := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:element name="a" type="xs:string"/></xs:schema>`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"parse", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("parse = %d, want 0 (stderr %q)", code, stderr.String())
	}
	if want := "  namespace: " + absentNamespace + "\n"; !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout =\n%s\nwant %q", stdout.String(), want)
	}
}

// TestParseNamespaceWithNoComponents pins the other end of the same
// derivation: §3.17.1 gives the Schema component no {target namespace}
// property, so a targetNamespace is observable only through the components a
// compilation declares, and one that declares none reports no namespace at
// all — not the ·absent· one, which would claim a schema with no namespace.
func TestParseNamespaceWithNoComponents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nocomponents.xsd")
	body := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://example.com/ns">` +
		`<xs:annotation><xs:documentation>declares nothing</xs:documentation></xs:annotation></xs:schema>`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"parse", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("parse = %d, want 0 (stderr %q)", code, stderr.String())
	}
	if want := path + "\n" + noComponentsSummary; stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
}

// TestParseNamespaceThroughInclusion pins the half the contract's scope turns
// on: the derivation runs over the whole compilation, so the namespace of a
// component an <xs:include>d document declares reaches the summary even though
// the argument document declares nothing. One <xs:include> is the whole
// difference from TestParseNamespaceWithNoComponents, and it reverses the
// outcome — which is why "the components the compilation declares" and "the
// components that document declares" are not the same contract.
func TestParseNamespaceThroughInclusion(t *testing.T) {
	dir := t.TempDir()
	const ns = `xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://example.com/ns"`
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("child.xsd", `<xs:schema `+ns+`><xs:element name="a" type="xs:string"/></xs:schema>`)
	write("root.xsd", `<xs:schema `+ns+`><xs:include schemaLocation="child.xsd"/></xs:schema>`)

	root := filepath.Join(dir, "root.xsd")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"parse", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("parse %q = %d, want 0 (stderr %q)", root, code, stderr.String())
	}
	if want := root + "\n" + includedNamespaceSummary; stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
}

// TestParseAdversarialArguments is the robustness bar the stub set and this
// landing must not fall below (#472, the 2026-08-12 cliuser pass): no panic,
// an exit code in the contract's three, and the streams kept apart, whatever
// the argument list.
func TestParseAdversarialArguments(t *testing.T) {
	lists := [][]string{
		{"parse", ""},
		{"parse", "--"},
		{"parse", "\x00\x01"},
		{"parse", "-"},
		{"parse", "-q", "-q"},
		{"parse", "-v", ""},
		{"parse", strings.Repeat("a/", 200) + "x.xsd"},
	}
	for _, args := range lists {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Errorf("run(%q) = %d, want 2 — none of these names a readable schema", args, code)
		}
		if stdout.Len() != 0 {
			t.Errorf("run(%q) stdout = %q, want empty", args, stdout.String())
		}
		if stderr.Len() == 0 {
			t.Errorf("run(%q) reported nothing on stderr", args)
		}
	}
}

// TestParseFlagAfterPositional pins the consequence doc.go's argument
// vocabulary states, which #1290 REVERSED: a subcommand's flags precede its
// positional arguments, and a trailing -q is now reported as the misplacement
// it is rather than opened as a schema location. Until then this test asserted
// the opposite — the summary printed, then "open -q: no such file or
// directory" — so the stdout assertion is the load-bearing half: reporting the
// misplacement after the run would answer with the same code and the same
// stderr line.
func TestParseFlagAfterPositional(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"parse", "testdata/order.xsd", "-q"}, &stdout, &stderr)
	if code != exitUsage {
		t.Errorf("run = %d, want %d — the trailing -q is a misplaced flag", code, exitUsage)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty: nothing is compiled once an argument is diagnosed", stdout.String())
	}
	want := fmt.Sprintf(flagAfterPositionalFmt, "parse", "-q")
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
	}
}

// TestParseFileNamedLikeAFlag pins the escape hatch the diagnosis above leaves
// open, the one doc.go names beside ./- : a schema document whose file name
// begins with - is compiled when a path reaches it, so the misplacement rule
// costs no argument that a user can otherwise spell.
func TestParseFileNamedLikeAFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "-q")
	body, err := os.ReadFile("testdata/order.xsd")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	// ./-q names that file only from the directory holding it, so the run
	// happens there; the absolute path names the same file from anywhere.
	t.Chdir(dir)
	for _, location := range []string{path, "./-q"} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"parse", location}, &stdout, &stderr); code != exitOK {
			t.Errorf("parse %s: code = %d, want %d (stderr %q)", location, code, exitOK, stderr.String())
		}
		if !strings.Contains(stdout.String(), "components: 8") {
			t.Errorf("parse %s: stdout = %q, want the summary of the schema that path names", location, stdout.String())
		}
	}
}
