package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/jsonsrc"
)

// The fixtures every outcome test turns on: the schema parse's own tests
// compile, an instance of it that charges nothing, one whose sku lexical is
// outside its Sku pattern, a second namespace's schema paired with an
// instance that names it through xsi:schemaLocation and nothing else, and a
// schema carrying an <xs:assert> whose instance charges nothing and leaves
// the assertion unevaluated, with two more instances of it that leave the
// assertion unevaluated beside a charge and beside a stopped walk, and a
// schema whose own <xs:include> names a document that is not there with an
// instance of it either way.
const (
	validInstance        = "testdata/order-valid.xml"
	invalidInstance      = "testdata/order-invalid.xml"
	orderSchema          = "testdata/order.xsd"
	hintedInstance       = "testdata/hinted.xml"
	hintedSchema         = "testdata/hinted.xsd"
	assertedSchema       = "testdata/asserted.xsd"
	assertedInstance     = "testdata/asserted.xml"
	assertedCharged      = "testdata/asserted-charged.xml"
	assertedUnclosed     = "testdata/asserted-unclosed.xml"
	brokenSchema         = "testdata/broken.xsd"
	shortSchema          = "testdata/unresolved-include.xsd"
	shortValidInstance   = "testdata/unresolved-include-valid.xml"
	shortInvalidInstance = "testdata/unresolved-include-invalid.xml"
)

// The JSON fixtures: order-valid.xml's and order-invalid.xml's documents and
// asserted.xml's as jsonsrc maps them, a document that ends inside its root's
// object, and one whose root member names no top-level declaration of
// order.xsd, which jsonsrc declines.
const (
	validJSON     = "testdata/order-valid.json"
	invalidJSON   = "testdata/order-invalid.json"
	assertedJSON  = "testdata/asserted.json"
	malformedJSON = "testdata/malformed.json"
	declinedJSON  = "testdata/declined.json"
)

// citedLocation matches a "<file>:<line>:<col>:" location anywhere in a line,
// capturing the file.
var citedLocation = regexp.MustCompile(`([^\s"]+):\d+:\d+:`)

// mustCiteOpenableLocations fails unless every "<file>:<line>:<col>:" location
// text cites names a file that exists — the positive form of "no line cites a
// document the reader cannot open" (STYLE E3), which a location inside a
// synthesized in-memory document fails (#1368). It returns how many locations
// it checked, so a caller that expects one can say so.
func mustCiteOpenableLocations(t *testing.T, stream, text string) int {
	t.Helper()
	matches := citedLocation.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if _, err := os.Stat(m[1]); err != nil {
			t.Errorf("%s cites %s, which names no file the reader can open (%v):\n%s", stream, m[0], err, text)
		}
	}
	return len(matches)
}

// TestValidateCleanInstance pins the quiet outcome: an instance that charges
// nothing writes nothing on either stream and exits 0, so a script can run
// validate in a pipeline and see output only when there is something to see.
func TestValidateCleanInstance(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", "-schema", orderSchema, validInstance}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("stdout = %q, stderr = %q, want both empty", stdout.String(), stderr.String())
	}
}

// TestValidateViolationsGoToStdout pins the stream and the rendering #1066
// decided and this landing implements: validate's product is the violation
// report a script pipes into grep, so it lands on stdout in the
// "<loc>: [<rule>] <message>" shape parse gives a schema error on stderr, and
// stderr stays empty because nothing went wrong with the run.
func TestValidateViolationsGoToStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", "-schema", orderSchema, invalidInstance}, &stdout, &stderr)
	if code != exitInvalid {
		t.Fatalf("code = %d, want %d (stderr %q)", code, exitInvalid, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty: an invalid instance is a verdict, not a fault", stderr.String())
	}
	line, _, _ := strings.Cut(stdout.String(), "\n")
	if !strings.HasPrefix(line, invalidInstance+":") {
		t.Errorf("first line = %q, want it to open with the instance location", line)
	}
	if !strings.Contains(line, "[cvc-attribute]") {
		t.Errorf("first line = %q, want the charged rule ID in brackets", line)
	}
}

// TestValidatePathRendering pins usage's Paths paragraph from a known working
// directory: every line of an instance's report opens with the instance
// argument as it was typed, whichever of the four spellings that is, and a
// schema rejection's location is absolute whichever spelling named the schema.
func TestValidatePathRendering(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{orderSchema, invalidInstance, brokenSchema} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "sub", filepath.Base(f)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	instanceXML, err := os.ReadFile(invalidInstance)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	const schema, instance, broken = "sub/order.xsd", "sub/order-invalid.xml", "sub/broken.xsd"
	absInstance := filepath.Join(dir, instance)
	absBroken := filepath.Join(dir, broken)

	t.Run("instance", func(t *testing.T) {
		for _, c := range []struct{ arg, opening string }{
			{instance, instance + ":5:"},
			{"./" + instance, "./" + instance + ":5:"},
			{absInstance, absInstance + ":5:"},
			{stdinArg, stdinArg + ":5:"},
		} {
			if c.arg == stdinArg {
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := w.Write(instanceXML); err != nil {
					t.Fatal(err)
				}
				if err := w.Close(); err != nil {
					t.Fatal(err)
				}
				old := os.Stdin
				os.Stdin = r
				t.Cleanup(func() { os.Stdin = old; _ = r.Close() })
			}
			args := []string{"validate", "-format", "xml", "-schema", schema, c.arg}
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != exitInvalid {
				t.Fatalf("run(%q) = %d, want %d (stderr %q)", args, code, exitInvalid, stderr.String())
			}
			lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
			if len(lines) != 2 {
				t.Errorf("run(%q) stdout =\n%s\nwant 2 lines", args, stdout.String())
			}
			for _, line := range lines {
				if !strings.HasPrefix(line, c.opening) {
					t.Errorf("run(%q) line = %q, want it to open %q", args, line, c.opening)
				}
			}
		}
	})

	t.Run("schema", func(t *testing.T) {
		for _, arg := range []string{broken, "./" + broken, absBroken} {
			args := []string{"validate", "-schema", arg, instance}
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != exitSchema {
				t.Fatalf("run(%q) = %d, want %d (stderr %q)", args, code, exitSchema, stderr.String())
			}
			if want := absBroken + ":9:3: [src-resolve] "; !strings.HasPrefix(stderr.String(), want) {
				t.Errorf("run(%q) stderr = %q, want it to open %q", args, stderr.String(), want)
			}
		}
	})
}

// TestValidateRendersDelegatedVerdictWithoutPlaceholder pins the whole line a
// delegating charge prints: the String Valid verdict it wraps carries no
// position of its own, and renders as "[rule] msg" after the outer charge's
// sentence rather than behind the zero Loc's "?: " (#1844).
func TestValidateRendersDelegatedVerdictWithoutPlaceholder(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"validate", "-schema", orderSchema, invalidInstance}, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("code = %d, want %d (stderr %q)", code, exitInvalid, stderr.String())
	}
	const want = invalidInstance + `:5:11: [cvc-attribute] attribute sku's value "nope" is not a valid {http://example.com/order}Sku, which cvc-attribute clause 3 requires: [cvc-pattern-valid] "nope" matches no member of the pattern facet of the simple type {http://example.com/order}Sku, whose {value} holds "[A-Z]{3}-[0-9]{4}", but cvc-pattern-valid requires one to match`
	if line, _, _ := strings.Cut(stdout.String(), "\n"); line != want {
		t.Errorf("first line =\n%s\nwant\n%s", line, want)
	}
}

// TestValidateRendersALexicalSpaceFaultWhole pins the whole line validate
// prints for <amount>12,50</amount> against an element of type xs:decimal:
// the cvc-datatype-valid cause the strict backend builds opens with the
// lexical and the type and closes with its rule inline (STYLE E5), with no
// trailing production-and-section parenthetical (#2371), and the outer cvc-type
// sentence it wraps opens with the element and its value (#2370).
func TestValidateRendersALexicalSpaceFaultWhole(t *testing.T) {
	dir := t.TempDir()
	schema := filepath.Join(dir, "s.xsd")
	instance := filepath.Join(dir, "a.xml")
	if err := os.WriteFile(schema, []byte(`<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:element name="amount" type="xs:decimal"/></xs:schema>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(instance, []byte(`<amount>12,50</amount>`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"validate", "-schema", schema, instance}, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("code = %d, want %d (stdout %q, stderr %q)", code, exitInvalid, stdout.String(), stderr.String())
	}
	want := instance + `:1:1: [cvc-type] element amount's value "12,50" is not a valid {http://www.w3.org/2001/XMLSchema}decimal, which cvc-type clause 3.1.3 requires: [cvc-datatype-valid] "12,50" is not in the lexical space of decimal, which cvc-datatype-valid clause 2.1 requires it to be in` + "\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout =\n%s\nwant\n%s", got, want)
	}
}

// runIntPattern validates <n>x3</n> against an element of type xs:int, a
// lexical outside xs:integer's built-in pattern, which xs:int inherits
// (Datatypes integer.pattern), and returns the instance path and the run's
// outcome.
func runIntPattern(t *testing.T) (instance string, code int, stdout, stderr string) {
	t.Helper()
	dir := t.TempDir()
	schema := filepath.Join(dir, "s.xsd")
	instance = filepath.Join(dir, "i.xml")
	if err := os.WriteFile(schema, []byte(`<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:element name="n" type="xs:int"/></xs:schema>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(instance, []byte(`<n>x3</n>`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code = run([]string{"validate", "-schema", schema, instance}, &out, &errOut)
	return instance, code, out.String(), errOut.String()
}

// TestValidateNamesTheBuiltinPatternAndItsDeclaringType pins that a built-in
// type's pattern rejection names the pattern it failed, the type that declares
// it — xs:integer's [\-+]?[0-9]+ — and that xs:int inherits it, rather than
// reading as a pattern the schema author wrote (#2310, #3335).
func TestValidateNamesTheBuiltinPatternAndItsDeclaringType(t *testing.T) {
	_, code, stdout, stderr := runIntPattern(t)
	if code != exitInvalid {
		t.Fatalf("code = %d, want %d (stdout %q, stderr %q)", code, exitInvalid, stdout, stderr)
	}
	const want = `[cvc-pattern-valid] "x3" matches no member of the pattern facet that the simple type {http://www.w3.org/2001/XMLSchema}int inherits from the simple type {http://www.w3.org/2001/XMLSchema}integer, whose {value} holds "[\-+]?[0-9]+", but cvc-pattern-valid requires one to match`
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout =\n%s\nwant it to carry\n%s", stdout, want)
	}
}

// TestValidateIntPatternRuleAndExitCode characterizes the rule a lexical
// outside xs:int's inherited pattern is charged under: exit 1, cvc-type
// clause 3.1.3 wrapping cvc-pattern-valid, the outer sentence pinned whole up
// to the ": " before its cause, in STYLE E5's order: the element and its value,
// the type it is not a valid one of, then the citation (#2370). The lexical
// also fails xs:int's lexical mapping (cvc-datatype-valid clause 2.1) and the
// spec orders neither clause first, so this pins the repository's choice — the
// pattern stage runs before the mapping — against a re-charge under another
// rule (#2310).
func TestValidateIntPatternRuleAndExitCode(t *testing.T) {
	instance, code, stdout, stderr := runIntPattern(t)
	if code != exitInvalid {
		t.Fatalf("code = %d, want %d (stdout %q, stderr %q)", code, exitInvalid, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	line, _, _ := strings.Cut(stdout, "\n")
	if prefix := instance + `:1:1: [cvc-type] element n's value "x3" is not a valid {http://www.w3.org/2001/XMLSchema}int, which cvc-type clause 3.1.3 requires: [cvc-pattern-valid] `; !strings.HasPrefix(line, prefix) {
		t.Errorf("first line = %q, want it to open %q", line, prefix)
	}
	if !strings.Contains(line, `: [cvc-pattern-valid] "x3" `) {
		t.Errorf("first line = %q, want the cvc-type charge to wrap a cvc-pattern-valid verdict on \"x3\"", line)
	}
}

// TestValidateQuietDoesNotSuppressViolations pins the constraint #16 still
// carries and doc.go states: -q suppresses a subcommand's INFORMATIONAL
// output, and validate's violations are its product, not information about it.
func TestValidateQuietDoesNotSuppressViolations(t *testing.T) {
	var quiet, loud bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"validate", "-q", "-schema", orderSchema, invalidInstance}, &quiet, &stderr); code != exitInvalid {
		t.Fatalf("code = %d, want %d (stderr %q)", code, exitInvalid, stderr.String())
	}
	stderr.Reset()
	if code := run([]string{"validate", "-schema", orderSchema, invalidInstance}, &loud, &stderr); code != exitInvalid {
		t.Fatalf("code = %d, want %d (stderr %q)", code, exitInvalid, stderr.String())
	}
	if quiet.String() != loud.String() {
		t.Errorf("-q changed the violation report:\n%s\nwant\n%s", quiet.String(), loud.String())
	}
	if quiet.Len() == 0 {
		t.Error("-q suppressed the whole report")
	}
}

// TestValidateReportsEveryInstance is the half #1066 left to this issue: the
// run does not stop at the first invalid instance. Both files' violations are
// on stdout, in argument order, from one invocation.
func TestValidateReportsEveryInstance(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"validate", "-no-hints", "-schema", orderSchema, invalidInstance, validInstance, hintedInstance}
	if code := run(args, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("code = %d, want %d (stderr %q)", code, exitInvalid, stderr.String())
	}
	first := strings.Index(stdout.String(), invalidInstance)
	last := strings.Index(stdout.String(), hintedInstance)
	if first < 0 || last < 0 {
		t.Fatalf("stdout =\n%s\nwant a line for both %s and %s", stdout.String(), invalidInstance, hintedInstance)
	}
	if first > last {
		t.Errorf("stdout =\n%s\nwant the instances reported in argument order", stdout.String())
	}
	if !strings.Contains(stdout.String(), "[cvc-assess-elt]") {
		t.Errorf("stdout =\n%s\nwant the third instance's own charge, which a short-circuit would have skipped", stdout.String())
	}
}

// TestValidateSchemaFaultHasItsOwnExitCode is this issue's first Acceptance
// bullet: a broken schema and broken data do not share a code, so a CI script
// branches on "your schema is wrong" without reading a message. The schema
// fault is also reported ONCE, before any instance is read, and on stderr —
// stdout carries verdicts about instances alone.
func TestValidateSchemaFaultHasItsOwnExitCode(t *testing.T) {
	// The code is its own before any run proves a schema set earns it: a
	// contract that promised the split and spent an existing code on it would
	// leave every assertion below passing.
	if exitSchema == exitOK || exitSchema == exitInvalid || exitSchema == exitUsage {
		t.Fatalf("exitSchema = %d, want a code of its own beside %d, %d and %d", exitSchema, exitOK, exitInvalid, exitUsage)
	}

	var stdout, stderr bytes.Buffer
	args := []string{"validate", "-schema", "testdata/broken.xsd", validInstance, invalidInstance}
	code := run(args, &stdout, &stderr)
	if code != exitSchema {
		t.Fatalf("code = %d, want %d", code, exitSchema)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if got := strings.Count(stderr.String(), "\n"); got != 1 {
		t.Errorf("stderr = %q, want one line however many instances were named", stderr.String())
	}
	if !strings.Contains(stderr.String(), "[src-resolve]") {
		t.Errorf("stderr = %q, want the rule the assembly charged", stderr.String())
	}
}

// TestValidateSchemaFaultPrecedesInstanceUsage pins the order usage's exit-code
// table states for 3, which never passes through worse (#1232): a set that does
// not compile answers 3 beside an instance argument that is itself a usage
// error, since no instance is examined, and a 2 raised before compiling — a
// bad -format, an unreadable -schema, a -schema of -, a flag after a
// positional, an instance whose name gives no format (an existing order.txt,
// or - without -format) — answers instead of the 3. Each instance argument of
// the 3 loop is first shown to answer 2 against a set that compiles, so the 3
// is not an argument that was never a fault.
func TestValidateSchemaFaultPrecedesInstanceUsage(t *testing.T) {
	dir := t.TempDir()
	unnamed := filepath.Join(dir, "order.txt")
	if err := os.WriteFile(unnamed, []byte("<order/>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, instance := range []string{"testdata/nosuch.xml", dir} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"validate", "-schema", orderSchema, instance}, &stdout, &stderr); code != exitUsage {
			t.Fatalf("instance %q against a set that compiles = %d, want %d: it is no usage error to preempt (stderr %q)", instance, code, exitUsage, stderr.String())
		}
		stdout.Reset()
		stderr.Reset()
		if code := run([]string{"validate", "-schema", brokenSchema, instance}, &stdout, &stderr); code != exitSchema {
			t.Errorf("instance %q beside a set that does not compile = %d, want %d (stderr %q)", instance, code, exitSchema, stderr.String())
		}
		if got := strings.Count(stderr.String(), "\n"); got != 1 || !strings.Contains(stderr.String(), "[src-resolve]") {
			t.Errorf("instance %q: stderr = %q, want the set's one src-resolve line and nothing about the instance", instance, stderr.String())
		}
	}
	for _, args := range [][]string{
		{"validate", "-format", "bogus", "-schema", brokenSchema, validInstance},
		{"validate", "-schema", brokenSchema, "-schema", "testdata/nosuch.xsd", validInstance},
		{"validate", "-schema", "testdata/nosuch.xsd", "-schema", brokenSchema, validInstance},
		{"validate", "-schema", brokenSchema, validInstance, "-q"},
		{"validate", "-schema", brokenSchema, "-schema", stdinArg, validInstance},
		{"validate", "-schema", brokenSchema, unnamed},
		{"validate", "-schema", brokenSchema, stdinArg},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != exitUsage {
			t.Errorf("run(%q) = %d, want %d: a fault read before compiling answers before the set does (stderr %q)", args, code, exitUsage, stderr.String())
		}
		if strings.Contains(stderr.String(), "[src-resolve]") {
			t.Errorf("run(%q): stderr = %q, want no compilation reached", args, stderr.String())
		}
	}
}

// TestValidateUndecidedInstanceHasItsOwnExitCode is this issue's witness: an
// instance charged no violation whose assessment DECLINED a check it reached
// is not the clean pass exit 0 names, and the CLI must say so on the same
// stream a violation gets, opening "<loc>: [<rule>] " as a violation line
// does. asserted.xml charges nothing and leaves the <xs:assert> of its
// governing type unevaluated, which validate/doc.go's Contract calls
// undecided rather than a pass.
func TestValidateUndecidedInstanceHasItsOwnExitCode(t *testing.T) {
	// The code is its own before any run earns it, on the reasoning
	// TestValidateSchemaFaultHasItsOwnExitCode states.
	if slices.Contains([]int{exitOK, exitInvalid, exitUsage, exitSchema}, exitUndecided) {
		t.Fatalf("exitUndecided = %d, want a code of its own beside %d, %d, %d and %d", exitUndecided, exitOK, exitInvalid, exitUsage, exitSchema)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", "-schema", assertedSchema, assertedInstance}, &stdout, &stderr)
	if code != exitUndecided {
		t.Fatalf("code = %d, want %d (stdout %q, stderr %q)", code, exitUndecided, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty: a check the assessment declined is a verdict about the instance, not a fault of the run", stderr.String())
	}
	line, _, _ := strings.Cut(stdout.String(), "\n")
	if !strings.HasPrefix(line, assertedInstance+":") {
		t.Errorf("first line = %q, want it to open with the instance location", line)
	}
	if !strings.Contains(line, "[cvc-assertion]") {
		t.Errorf("first line = %q, want the declined check's rule ID in brackets, as a violation line carries the charged one", line)
	}
}

// TestValidateUndecidedIsLessSevereThanInvalid pins the severity ordering the
// exit codes are aggregated in, which is not their numeric order: a batch
// holding an instance shown invalid and one merely left undecided answers
// with the invalid one, whatever order the arguments came in.
func TestValidateUndecidedIsLessSevereThanInvalid(t *testing.T) {
	for _, args := range [][]string{
		{"validate", "-schema", orderSchema, "-schema", assertedSchema, invalidInstance, assertedInstance},
		{"validate", "-schema", orderSchema, "-schema", assertedSchema, assertedInstance, invalidInstance},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != exitInvalid {
			t.Errorf("run(%v) = %d, want %d (stdout %q, stderr %q)", args, code, exitInvalid, stdout.String(), stderr.String())
		}
		if !strings.Contains(stdout.String(), "[cvc-assertion]") {
			t.Errorf("run(%v) stdout =\n%s\nwant the undecided instance reported too, not swallowed by the invalid one", args, stdout.String())
		}
	}
}

// TestStandingCode pins the exit code each [validate.Standing] arm earns, and
// that a Standing no arm matches never answers exitOK.
func TestStandingCode(t *testing.T) {
	for _, c := range []struct {
		name     string
		standing validate.Standing
		want     int
	}{
		{"Charged", validate.Charged, exitInvalid},
		{"Undecided", validate.Undecided, exitUndecided},
		{"NotProvenInvalid", validate.NotProvenInvalid, exitOK},
	} {
		if got := standingCode(c.standing); got != c.want {
			t.Errorf("standingCode(%s) = %d, want %d", c.name, got, c.want)
		}
	}
	for _, c := range []struct {
		name     string
		standing validate.Standing
	}{
		{"Standing(0)", validate.Standing(0)},
		{"Standing(NotProvenInvalid+1)", validate.NotProvenInvalid + 1},
	} {
		if got := standingCode(c.standing); got == exitOK {
			t.Errorf("standingCode(%s) = exitOK, want any other code: a Standing no arm matches must not pass a gate", c.name)
		}
	}
}

// TestValidateUndecidedBesideInvalidIsInvalid pins that one instance whose
// Result carries an undecided check beside a charge, or beside a stopped walk,
// exits exitInvalid: the undecided check does not soften the instance's code.
// The charged row reaches exitInvalid through standingCode's Charged arm; the
// unclosed row's Standing is Undecided, and only assessmentLines raising it
// through worse for result.Err() reaches exitInvalid. Each row asserts the
// charge or fault line, then the [cvc-assertion] line, in that order, and that
// the two render apart from the same run: the undecided line carries
// "<loc>: [cvc-assertion] undecided: " and the charge line carries no
// "undecided: " after its rule.
func TestValidateUndecidedBesideInvalidIsInvalid(t *testing.T) {
	for _, c := range []struct {
		name     string
		instance string
		first    string
	}{
		{"charge", assertedCharged, assertedCharged + ":4:56: [cvc-complex-type] "},
		{"stopped walk", assertedUnclosed, assertedUnclosed + ":4:1: [xml-wf] "},
	} {
		var stdout, stderr bytes.Buffer
		code := run([]string{"validate", "-schema", assertedSchema, c.instance}, &stdout, &stderr)
		if code != exitInvalid {
			t.Errorf("%s: code = %d, want %d (stdout %q, stderr %q)", c.name, code, exitInvalid, stdout.String(), stderr.String())
		}
		lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
		if len(lines) != 2 {
			t.Errorf("%s: stdout =\n%s\nwant two lines, the %q one then the [cvc-assertion] one", c.name, stdout.String(), c.first)
			continue
		}
		if !strings.HasPrefix(lines[0], c.first) {
			t.Errorf("%s: first line = %q, want prefix %q", c.name, lines[0], c.first)
		}
		if strings.HasPrefix(lines[0], c.first+"undecided: ") {
			t.Errorf("%s: first line = %q, want no undecided: mark: it is a charge", c.name, lines[0])
		}
		if !strings.HasPrefix(lines[1], c.instance+":") || !strings.Contains(lines[1], ": [cvc-assertion] undecided: ") {
			t.Errorf("%s: second line = %q, want the instance's \"<loc>: [cvc-assertion] undecided: \" line: the declined check marks itself apart from the charge", c.name, lines[1])
		}
	}
}

// TestValidateQuietDoesNotSuppressUndecidedChecks holds the new stream to the
// settled scope of -q: it silences a subcommand's informational output, and
// the checks the assessment declined are a verdict about the instance.
func TestValidateQuietDoesNotSuppressUndecidedChecks(t *testing.T) {
	var quiet, loud, stderr bytes.Buffer
	if code := run([]string{"validate", "-q", "-schema", assertedSchema, assertedInstance}, &quiet, &stderr); code != exitUndecided {
		t.Fatalf("code = %d, want %d (stderr %q)", code, exitUndecided, stderr.String())
	}
	stderr.Reset()
	if code := run([]string{"validate", "-schema", assertedSchema, assertedInstance}, &loud, &stderr); code != exitUndecided {
		t.Fatalf("code = %d, want %d (stderr %q)", code, exitUndecided, stderr.String())
	}
	if quiet.String() != loud.String() {
		t.Errorf("-q changed the report:\n%s\nwant\n%s", quiet.String(), loud.String())
	}
	if quiet.Len() == 0 {
		t.Error("-q suppressed the whole report")
	}
}

// TestValidateSchemasComposeIntoOneSet pins the contract sentence a user can
// learn nowhere else: several -schema arguments are ONE compilation, not one
// each. hinted.xml's root is declared only by hinted.xsd, so it can be
// assessed with hints OFF exactly when the two documents compose.
func TestValidateSchemasComposeIntoOneSet(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"validate", "-no-hints", "-schema", orderSchema, "-schema", hintedSchema, hintedInstance}
	if code := run(args, &stdout, &stderr); code != exitOK {
		t.Fatalf("code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout.String(), stderr.String())
	}

	// The same instance against the same partial set alone: the composition,
	// not the order.xsd argument, is what made the run above succeed.
	stdout.Reset()
	stderr.Reset()
	partial := []string{"validate", "-no-hints", "-schema", orderSchema, hintedInstance}
	if code := run(partial, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("partial set: code = %d, want %d", code, exitInvalid)
	}
}

// TestValidateSchemasShareATargetNamespace is the composition case a wrapper
// that <include>d everything would get wrong and the one the persona's Story C
// describes: two documents in ONE target namespace, one declaring the element
// and the other the type it references. They compose only if the set is a real
// assembly, since neither document alone resolves the reference.
func TestValidateSchemasShareATargetNamespace(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	declaration := write("element.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:x="urn:x" targetNamespace="urn:x"><xs:element name="root" type="x:Count"/></xs:schema>`)
	definition := write("type.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x"><xs:simpleType name="Count"><xs:restriction base="xs:int"/></xs:simpleType></xs:schema>`)
	valid := write("valid.xml", `<root xmlns="urn:x">42</root>`)
	invalid := write("invalid.xml", `<root xmlns="urn:x">forty-two</root>`)

	var stdout, stderr bytes.Buffer
	args := []string{"validate", "-no-hints", "-schema", declaration, "-schema", definition, valid}
	if code := run(args, &stdout, &stderr); code != exitOK {
		t.Fatalf("code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout.String(), stderr.String())
	}

	// The set still decides: composing is not the same as accepting.
	stdout.Reset()
	stderr.Reset()
	args = []string{"validate", "-no-hints", "-schema", declaration, "-schema", definition, invalid}
	if code := run(args, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("invalid instance: code = %d, want %d (stderr %q)", code, exitInvalid, stderr.String())
	}

	// And the reference is unresolved without the second document, which is a
	// schema fault rather than a verdict about the instance.
	stdout.Reset()
	stderr.Reset()
	args = []string{"validate", "-no-hints", "-schema", declaration, valid}
	if code := run(args, &stdout, &stderr); code != exitSchema {
		t.Fatalf("half the set: code = %d, want %d (stderr %q)", code, exitSchema, stderr.String())
	}
	if !strings.Contains(stderr.String(), "[src-resolve]") {
		t.Errorf("stderr = %q, want the unresolved reference charged", stderr.String())
	}
}

// TestValidateNamesAnUnresolvedSchemaSideDirective is #1260's acceptance for
// the other subcommand: a -schema document's own <xs:include> naming a
// document that is not there is named on stderr, once, before any instance is
// assessed — and nothing else moves. The set composes (src-include clause
// 2.4), so exitSchema is not the answer, and assessment runs normally against
// the SHORT set: the clean instance charges nothing and the invalid one is
// charged against the declaration the argument document does make, which an
// unassessed or empty set could not produce.
//
// The line belongs to the -schema set alone. A hinted document's own
// unfollowed directives are #1251's, and every location a line cites must name
// a document the reader can open (STYLE E3).
func TestValidateNamesAnUnresolvedSchemaSideDirective(t *testing.T) {
	abs, err := filepath.Abs(shortSchema)
	if err != nil {
		t.Fatal(err)
	}
	// The <xs:include> the fixture carries on line 10.
	at := abs + ":10:3:"

	var stdout, stderr bytes.Buffer
	if code := run([]string{"validate", "-schema", shortSchema, shortValidInstance}, &stdout, &stderr); code != exitOK {
		t.Fatalf("clean instance: code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty — the instance charges nothing", stdout.String())
	}
	if !strings.Contains(stderr.String(), at) {
		t.Errorf("stderr = %q, want the unfollowed directive's location %q", stderr.String(), at)
	}
	mustCiteOpenableLocations(t, "stderr", stderr.String())
	if strings.Contains(stderr.String(), "[") {
		t.Errorf("stderr = %q, want no rule brackets — the skip is not a violation", stderr.String())
	}

	// One line for the set, not one per instance, and the assessment still
	// decides every one of them.
	stdout.Reset()
	stderr.Reset()
	args := []string{"validate", "-schema", shortSchema, shortValidInstance, shortInvalidInstance}
	if code := run(args, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("two instances: code = %d, want %d (stdout %q, stderr %q)", code, exitInvalid, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "[cvc-type]") {
		t.Errorf("stdout = %q, want the invalid instance charged against the short set", stdout.String())
	}
	if got := strings.Count(stderr.String(), at); got != 1 {
		t.Errorf("stderr names the directive %d times, want 1 — the set is compiled once:\n%s", got, stderr.String())
	}
}

// TestValidateNamesAnUnresolvedDirectiveOfARejectedSet is #1312's ruling on
// the set-wide arm. compileSet composes every -schema document as ONE
// assembly, so reading its report only when that assembly compiled let one
// document's unrelated rejection suppress every other document's shortfall:
// here the src-resolve fault is broken.xsd's alone, and the unfollowed
// <xs:include> belongs to a document that composed cleanly.
//
// Both are named, in the wording a rejected assembly earns, and the shortfall
// stands above the verdict — which is neither charged against it nor cleared of
// it, the report saying nothing either way.
func TestValidateNamesAnUnresolvedDirectiveOfARejectedSet(t *testing.T) {
	abs, err := filepath.Abs(shortSchema)
	if err != nil {
		t.Fatal(err)
	}
	// The <xs:include> the fixture carries on line 10, in the rejected
	// assembly's wording.
	note := "goxsd8: validate: " + abs + `:10:3: this <include>'s schemaLocation "nosuchdocument.xsd" resolved to no document; the rejected assembly is short of whatever that document declares` + "\n"

	var stdout, stderr bytes.Buffer
	args := []string{"validate", "-no-hints", "-schema", shortSchema, "-schema", brokenSchema, shortValidInstance}
	if code := run(args, &stdout, &stderr); code != exitSchema {
		t.Fatalf("code = %d, want %d — the set does not compile (stdout %q, stderr %q)", code, exitSchema, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), note) {
		t.Errorf("stderr =\n%s\nwant the clean document's own note %q", stderr.String(), note)
	}
	if !strings.Contains(stderr.String(), "[src-resolve]") {
		t.Errorf("stderr =\n%s\nwant the other document's rejection charged", stderr.String())
	}
	if strings.Contains(stderr.String(), "compiled schema") {
		t.Errorf("stderr =\n%s\nwant no line calling the rejected set a compiled schema", stderr.String())
	}
	if strings.Index(stderr.String(), note) > strings.Index(stderr.String(), "[src-resolve]") {
		t.Errorf("stderr =\n%s\nwant the note before the error line", stderr.String())
	}
}

// TestValidateIOFaultOnReferencedDocumentIsASchemaFault is #1419's ruling on
// validate's side of the fault TestParseIOFaultOnReferencedDocumentCarriesNoRule
// pins for parse: an I/O fault reading a document a -schema argument
// REFERENCES comes back out of compileSet and is charged exitSchema, not the
// exitUsage an unreadable argument earns — the code the contract now states.
//
// The instance argument is there to reach compileSet at all: with none, the
// run stops on "no instance given" and exits 2 before composing (#1432).
// Compilation fails before any instance is read, so which one it is does not
// matter. The ENOTDIR trigger and its platform probe are the parse test's, for
// its reason: a platform answering ENOENT would take the legal-skip arm.
func TestValidateIOFaultOnReferencedDocumentIsASchemaFault(t *testing.T) {
	const schema = "testdata/notdir-include.xsd"
	abs, err := filepath.Abs(schema)
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
	if code := run([]string{"validate", "-schema", schema, validInstance}, &stdout, &stderr); code != exitSchema {
		t.Fatalf("code = %d, want %d — the set does not compile (stderr %q)", code, exitSchema, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty — no instance was assessed", stdout.String())
	}
	lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("stderr = %q, want two lines: the unfollowed-directive shortfall, then the fault", stderr.String())
	}
	if want := "goxsd8: validate: " + abs + ":12:3: "; !strings.HasPrefix(lines[0], want) {
		t.Errorf("stderr line 1 = %q, want reportUnfollowed's line for the directive, prefix %q", lines[0], want)
	}
	prefix := fmt.Sprintf("parser: resolving <include> schemaLocation %q at %s:12:3: loader: opening %q under %q: ",
		inner, abs, inner, filesystemRoot(abs))
	if !strings.HasPrefix(lines[1], prefix) {
		t.Errorf("stderr line 2 = %q, want prefix %q", lines[1], prefix)
	}
}

// TestValidateHints is the -no-hints Acceptance bullet, both halves: a partial
// -schema set plus the instance's own xsi:schemaLocation succeeds, and the
// same set with -no-hints fails under a rule ID that names the fault —
// cvc-assess-elt, a validation root the set declares nothing for — rather than
// a generic error.
func TestValidateHints(t *testing.T) {
	var stdout, stderr bytes.Buffer
	followed := []string{"validate", "-schema", orderSchema, hintedInstance}
	if code := run(followed, &stdout, &stderr); code != exitOK {
		t.Fatalf("hints followed: code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	ignored := []string{"validate", "-no-hints", "-schema", orderSchema, hintedInstance}
	if code := run(ignored, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("-no-hints: code = %d, want %d (stderr %q)", code, exitInvalid, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[cvc-assess-elt]") {
		t.Errorf("-no-hints stdout = %q, want the cvc-assess-elt charge", stdout.String())
	}
}

// TestValidateNoNamespaceHint pins the other half of §2.7.3: the
// no-namespace hint names one location and no namespace to pair it with, so it
// is followed as parser.HintAt's absent namespace — src-import clause 3.2,
// which a no-namespace document satisfies.
func TestValidateNoNamespaceHint(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write("plain.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:element name="plain" type="xs:int"/></xs:schema>`)
	valid := write("valid.xml", `<plain xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="plain.xsd">7</plain>`)
	invalid := write("invalid.xml", `<plain xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="plain.xsd">seven</plain>`)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"validate", "-schema", orderSchema, valid}, &stdout, &stderr); code != exitOK {
		t.Fatalf("code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout.String(), stderr.String())
	}

	// The hinted document decides, so a wrong lexical is still charged.
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"validate", "-schema", orderSchema, invalid}, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("invalid instance: code = %d, want %d (stderr %q)", code, exitInvalid, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[cvc-type]") {
		t.Errorf("stdout = %q, want the hinted type's charge", stdout.String())
	}

	// And -no-hints leaves the root undeclared, as it does for a namespaced
	// hint.
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"validate", "-no-hints", "-schema", orderSchema, valid}, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("-no-hints: code = %d, want %d (stderr %q)", code, exitInvalid, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[cvc-assess-elt]") {
		t.Errorf("-no-hints stdout = %q, want the cvc-assess-elt charge", stdout.String())
	}
}

// TestValidateUnusableHintIsTheInstancesFault pins the attribution a hint
// failure gets: a hint is the INSTANCE's own advisory claim (§4.3.2 clause 3),
// so a set that stops compiling once that instance's hints are folded in is a
// fault of the instance, never of the -schema set the invocation named. The
// three ways a hint can be unusable — pairing a namespace with a document that
// declares another, naming a document that is not well-formed, naming one that
// is not there — therefore degrade identically: the -schema set alone decides,
// which charges cvc-assess-elt here, and exitSchema stays the answer to a
// -schema set that does not compile.
//
// All three are also NAMED on stderr, against the instance and by the hinted
// document (#1251). Only the first two are faults: a schemaLocation that
// resolves to nothing is legal to skip, so that case's line carries no rule ID.
//
// Every location a line cites must name a document the reader can open (STYLE
// E3): a charge against a hint is charged at the hinted document itself.
func TestValidateUnusableHintIsTheInstancesFault(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	// The document the mis-paired hint names: its own targetNamespace is not
	// the one the hint pairs it with, which is src-import clause 3.1.
	write("actual.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://example.com/actual"><xs:element name="note" type="xs:string"/></xs:schema>`)
	write("junk.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"`)
	instance := func(name, location string) string {
		return write(name, `<h:note xmlns:h="http://example.com/hinted" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="http://example.com/hinted `+location+`">ok</h:note>`)
	}
	cases := []struct {
		name string
		path string
		// want is the hinted document the diagnosis must name.
		want string
		// composes reports whether the augmented set still compiles, which a
		// hint naming a document that is not there does not stop: its line is a
		// shortfall rather than a charge, so it carries no rule ID.
		composes bool
	}{
		{"namespace mis-paired", instance("mispaired.xml", "actual.xsd"), "actual.xsd", false},
		{"hinted document malformed", instance("malformed-hint.xml", "junk.xsd"), "junk.xsd", false},
		{"hinted document missing", instance("missing-hint.xml", "nosuch.xsd"), "nosuch.xsd", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{"validate", "-schema", orderSchema, c.path}, &stdout, &stderr)
			if code != exitInvalid {
				t.Fatalf("code = %d, want %d (stdout %q, stderr %q)", code, exitInvalid, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), "[cvc-assess-elt]") {
				t.Errorf("stdout = %q, want the -schema set's own charge for an undeclared root", stdout.String())
			}
			mustCiteOpenableLocations(t, "stderr", stderr.String())
			mustCiteOpenableLocations(t, "stdout", stdout.String())
			if !strings.Contains(stderr.String(), c.path) {
				t.Errorf("stderr = %q, want the instance that carried the hint named", stderr.String())
			}
			if !strings.Contains(stderr.String(), c.want) {
				t.Errorf("stderr = %q, want the hinted document %s named", stderr.String(), c.want)
			}
			if charged := strings.Contains(stderr.String(), "["); charged == c.composes {
				t.Errorf("stderr = %q, want a rule bracket exactly where the augmented set does not compile (composes = %v)", stderr.String(), c.composes)
			}
		})
	}
}

// TestValidateNamesAMissingHintedDocument is #1251's acceptance: a hint whose
// location resolves to no document is legal to skip (src-import, src-include),
// so the augmented set still composes and the assessment still runs — and the
// operator is told, on stderr, which hinted document the set went without.
// Before this the whole shortfall was §4.3.2 clause 3's "less than complete
// ·assessment· outcomes" and nothing on either stream.
//
// The instance carries TWO hints so that the line is pinned to the hint that
// FAILED rather than to a position among the hints: correlating the report's
// entries to the hints by ordinal would name the resolving sibling here, the
// entries being only the hints that failed.
func TestValidateNamesAMissingHintedDocument(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write("good.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:good"><xs:element name="root" type="xs:int"/></xs:schema>`)
	// The document a mis-paired hint names: its own targetNamespace is not the
	// one that hint pairs it with, which is src-import clause 3.1.
	write("actual.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:actual"><xs:element name="note" type="xs:string"/></xs:schema>`)
	instance := func(name, pairs string) string {
		return write(name, `<g:root xmlns:g="urn:good" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="`+pairs+`">7</g:root>`)
	}
	missing := filepath.Join(dir, "nosuch.xsd")
	// The hint that resolves comes FIRST, so the failing one is hints[1] and an
	// ordinal read of the single report entry would name good.xsd instead.
	composing := instance("composing.xml", "urn:good good.xsd urn:gone nosuch.xsd")
	note := "goxsd8: validate: " + composing + ": the schema location hint " + missing + " resolved to no document, which is legal and skipped; the compiled schema is short of whatever that document declares\n"

	var stdout, stderr bytes.Buffer
	if code := run([]string{"validate", "-schema", orderSchema, composing}, &stdout, &stderr); code != exitOK {
		t.Fatalf("code = %d, want %d — the surviving hint still decides (stdout %q, stderr %q)", code, exitOK, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty — the shortfall is not a verdict about the instance", stdout.String())
	}
	if stderr.String() != note {
		t.Errorf("stderr = %q, want exactly %q", stderr.String(), note)
	}

	// -q silences no diagnosis (#1066).
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"validate", "-q", "-schema", orderSchema, composing}, &stdout, &stderr); code != exitOK {
		t.Fatalf("-q: code = %d, want %d (stderr %q)", code, exitOK, stderr.String())
	}
	if stderr.String() != note {
		t.Errorf("-q stderr = %q, want exactly %q", stderr.String(), note)
	}

	// And the other assembly outcome, on reportUnfollowed's terms (#1312): the
	// missing hint is named there too, in the wording that claims neither
	// legality nor any part in the rejection — here the mis-paired sibling's.
	// The missing hint comes FIRST this time, so both positions are exercised.
	stdout.Reset()
	stderr.Reset()
	rejected := instance("rejected.xml", "urn:gone nosuch.xsd urn:wrong actual.xsd")
	rejectedNote := "goxsd8: validate: " + rejected + ": the schema location hint " + missing + " resolved to no document; the rejected assembly is short of whatever that document declares\n"
	if code := run([]string{"validate", "-schema", orderSchema, rejected}, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("rejected: code = %d, want %d (stdout %q, stderr %q)", code, exitInvalid, stdout.String(), stderr.String())
	}
	if !strings.HasPrefix(stderr.String(), rejectedNote) {
		t.Errorf("rejected stderr =\n%s\nwant it to open with %q", stderr.String(), rejectedNote)
	}
	if !strings.Contains(stderr.String(), "[src-import]") {
		t.Errorf("rejected stderr =\n%s\nwant the mis-paired hint charged", stderr.String())
	}
	if !strings.Contains(stdout.String(), "[cvc-assess-elt]") {
		t.Errorf("rejected stdout = %q, want the -schema set's own charge for an undeclared root", stdout.String())
	}
	if n := mustCiteOpenableLocations(t, "rejected stderr", stderr.String()); n == 0 {
		t.Errorf("rejected stderr =\n%s\nwant the mis-paired hint's charge located in the document it names", stderr.String())
	}
}

// TestValidateStrictChargesASkip pins -strict on validate (#1845), for both
// lines that name a schemaLocation resolving to no document. The -schema set's
// directive line charges the run 4, beside every instance's own outcome, a 1
// outranking it. An instance's hint line charges that instance 4, on both
// assembly outcomes of its hints: composing, and rejected by a mis-paired
// sibling hint, where the instance falls back to the -schema set alone. Every
// row without a line is charged nothing, -no-hints reading no hint to name.
func TestValidateStrictChargesASkip(t *testing.T) {
	dir := t.TempDir()
	valid, err := os.ReadFile(validInstance)
	if err != nil {
		t.Fatal(err)
	}
	// validInstance with hints on its document element, so that the -schema
	// set alone decides it clean whatever its hints compose to.
	instance := func(name, pairs string) string {
		body := strings.Replace(string(valid), `createdBy="ada"`, `createdBy="ada" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="`+pairs+`"`, 1)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if err := os.WriteFile(filepath.Join(dir, "actual.xsd"), []byte(`<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:actual"><xs:element name="note" type="xs:string"/></xs:schema>`), 0o644); err != nil {
		t.Fatal(err)
	}
	composing := instance("composing.xml", "urn:gone nosuch.xsd")
	rejected := instance("rejected.xml", "urn:gone nosuch.xsd urn:wrong actual.xsd")

	for _, c := range []struct {
		name       string
		args       []string
		want       int
		wantStderr string // a substring every row's stderr carries, "" for none
	}{
		{"set directive", []string{"-strict", "-schema", shortSchema, shortValidInstance}, exitUndecided, "resolved to no document and was skipped"},
		{"set directive beside an invalid instance", []string{"-strict", "-schema", shortSchema, shortValidInstance, shortInvalidInstance}, exitInvalid, "resolved to no document and was skipped"},
		{"set directive, default", []string{"-schema", shortSchema, shortValidInstance}, exitOK, "resolved to no document and was skipped"},
		{"hint, composing", []string{"-strict", "-schema", orderSchema, composing}, exitUndecided, "resolved to no document, which is legal and skipped"},
		{"hint, composing, beside a clean instance", []string{"-strict", "-schema", orderSchema, validInstance, composing}, exitUndecided, "the schema location hint"},
		{"hint, rejected", []string{"-strict", "-schema", orderSchema, rejected}, exitUndecided, "ignoring its schema location hints"},
		{"hint, rejected, default", []string{"-schema", orderSchema, rejected}, exitOK, "the schema location hint"},
		{"hint, no-hints", []string{"-strict", "-no-hints", "-schema", orderSchema, composing}, exitOK, ""},
		{"no skip", []string{"-strict", "-schema", orderSchema, validInstance}, exitOK, ""},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(append([]string{"validate"}, c.args...), &stdout, &stderr); code != c.want {
			t.Errorf("%s: validate %q = %d, want %d (stdout %q, stderr %q)", c.name, c.args, code, c.want, stdout.String(), stderr.String())
		}
		if c.wantStderr == "" && stderr.Len() != 0 || !strings.Contains(stderr.String(), c.wantStderr) {
			t.Errorf("%s: stderr = %q, want %q", c.name, stderr.String(), c.wantStderr)
		}
	}
}

// TestValidateSchemaDirectiveIsNotReportedAsAHint pins that the two shortfall
// reporters stay disjoint on one invocation (#1346): a -schema document's OWN
// unresolved directive is named exactly once, by runValidate's reportUnfollowed
// at its own file:line:column, and not again by the hint reporter — which would
// both double-report it and name a hint that RESOLVED as having resolved to
// nothing. The include sits on line 3 and the instance carries one resolving
// hint, the pair #1346 found reported twice.
func TestValidateSchemaDirectiveIsNotReportedAsAHint(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	base := write("base.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:base">`+"\n"+
		`  <xs:element name="b" type="xs:string"/>`+"\n"+
		`  <xs:include schemaLocation="nosuch-nested.xsd"/>`+"\n"+
		`</xs:schema>`)
	write("good.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://example.com/hinted"><xs:element name="note" type="xs:string"/></xs:schema>`)
	instance := write("inst.xml", `<h:note xmlns:h="http://example.com/hinted" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="http://example.com/hinted good.xsd">ok</h:note>`)
	want := "goxsd8: validate: " + base + `:3:3: this <include>'s schemaLocation "nosuch-nested.xsd" resolved to no document and was skipped; the compiled schema is short of whatever that document declares` + "\n"

	var stdout, stderr bytes.Buffer
	if code := run([]string{"validate", "-schema", base, instance}, &stdout, &stderr); code != exitOK {
		t.Fatalf("code = %d, want %d — the resolving hint decides (stdout %q, stderr %q)", code, exitOK, stdout.String(), stderr.String())
	}
	if stderr.String() != want {
		t.Errorf("stderr =\n%s\nwant exactly\n%s", stderr.String(), want)
	}
}

// TestValidateNonSchemaRootCarriesNoRule is TestParseNonSchemaRootCarriesNoRule
// on validate's side (#1368): a -schema argument whose document element is not
// <xs:schema> is rejected in the very line parse prints for it — no rule ID and
// no location naming a document the reader cannot open — since both reach
// parser's own root check. The exit code is validate's for a set that does not
// compile, which parse's single-schema contract does not share.
func TestValidateNonSchemaRootCarriesNoRule(t *testing.T) {
	var parseOut, parseErr bytes.Buffer
	if code := run([]string{"parse", "testdata/notschema.xml"}, &parseOut, &parseErr); code != exitInvalid {
		t.Fatalf("parse: code = %d, want %d (stderr %q)", code, exitInvalid, parseErr.String())
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"validate", "-schema", "testdata/notschema.xml", validInstance}, &stdout, &stderr); code != exitSchema {
		t.Fatalf("validate: code = %d, want %d (stderr %q)", code, exitSchema, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty — no instance was assessed", stdout.String())
	}
	if stderr.String() != parseErr.String() {
		t.Errorf("validate stderr = %q, want parse's line for the same document, %q", stderr.String(), parseErr.String())
	}
	mustCiteOpenableLocations(t, "stderr", stderr.String())
}

// TestValidateHintRepeatsASchemaArgument is the case a naive union would break
// on and the resolver's dedup contract covers: the instance hints the very
// document -schema already named, so the set holds it twice by location and
// once by identity. Composing it twice would collide every component under
// sch-props-correct clause 2.
func TestValidateHintRepeatsASchemaArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"validate", "-schema", hintedSchema, hintedInstance}
	if code := run(args, &stdout, &stderr); code != exitOK {
		t.Fatalf("code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout.String(), stderr.String())
	}
}

// TestValidateHintsResolveAgainstTheInstance pins §4.3.2 clause 4 as the CLI
// applies it: a relative hint names a document beside the INSTANCE, not beside
// the working directory, so an instance moved into another directory carries
// its schema with it.
func TestValidateHintsResolveAgainstTheInstance(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"hinted.xml", "hinted.xsd"} {
		body, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	args := []string{"validate", "-schema", orderSchema, filepath.Join(dir, "hinted.xml")}
	if code := run(args, &stdout, &stderr); code != exitOK {
		t.Fatalf("code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout.String(), stderr.String())
	}
}

// TestValidateFormatVocabulary pins the -format enum this issue narrowed: the
// three tokens are matched case-sensitively, an unrecognized one is a usage
// error LISTING the valid values, an instance whose extension names none of
// them is the same error rather than a guess, and ber, whose adapter is still
// a doc.go stub, fails cleanly instead of passing silently: a capability the
// build lacks, not a fault in the command line, so its line stands alone with
// no synopsis under it (#3333).
func TestValidateFormatVocabulary(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
		// shape marks a diagnosis of the command line's shape, which
		// validateUsageLine and helpPointer follow; any other row's line stands
		// alone (#3333).
		shape bool
	}{
		{"unrecognized token", []string{"-format", "yaml"}, `-format "yaml" is not a source format; pass one of xml, json or ber`, true},
		{"wrong case", []string{"-format", "XML"}, `-format "XML" is not a source format`, true},
		{"forced ber is reserved", []string{"-format", "ber"}, "-format ber is reserved by the contract and not yet implemented", false},
		{"extension is the default", []string{}, "the extension \".txt\" names no source format; pass -format with one of xml, json or ber", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := append([]string{"validate"}, c.args...)
			args = append(args, "-schema", orderSchema, "instance.txt")
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != exitUsage {
				t.Fatalf("code = %d, want %d", code, exitUsage)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), c.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), c.want)
			}
			if synopsis := strings.HasSuffix(stderr.String(), "\n"+validateUsageLine+"\n"+helpPointer+"\n"); synopsis != c.shape {
				t.Errorf("stderr = %q, synopsis and helpPointer under it = %t, want %t", stderr.String(), synopsis, c.shape)
			}
			if !c.shape && strings.Count(stderr.String(), "\n") != 1 {
				t.Errorf("stderr = %q, want one line", stderr.String())
			}
		})
	}
}

// TestValidateJSONInstance pins a .json instance's route through
// validate/jsonsrc: its violations print the "<loc>: [<rule>] <message>" line
// an XML instance's do, and its declined checks the "<loc>: [<rule>]
// undecided: <message>" one, at jsonsrc's line and byte column, and earn the
// same codes, while jsonsrc's one error channel, the source name standing
// where a rule ID stands, is routed by class: malformed JSON prints
// "<loc>: [jsonsrc] <message>" and exits 1, as malformed XML does, and a
// decline prints "<loc>: [jsonsrc] undecided: <message>" and exits 4. Every
// row runs with and without -no-hints and must answer the same, because a JSON
// instance carries no hints.
func TestValidateJSONInstance(t *testing.T) {
	cases := []struct {
		name      string
		schema    string
		instances []string
		code      int
		// lines are the report's whole lines, in order; a line ending in
		// "] " or "] undecided: " is a prefix, the message after it being
		// the engine's.
		lines []string
	}{
		{"valid", orderSchema, []string{validJSON}, exitOK, nil},
		{"invalid", orderSchema, []string{invalidJSON}, exitInvalid, []string{
			invalidJSON + ":1:19: [cvc-attribute] ",
			invalidJSON + ":1:11: [cvc-identity-constraint] ",
		}},
		{"unevaluated", assertedSchema, []string{assertedJSON}, exitUndecided, []string{
			assertedJSON + ":1:2: [cvc-assertion] undecided: ",
		}},
		{"malformed", orderSchema, []string{malformedJSON}, exitInvalid, []string{
			malformedJSON + ":1:18: [jsonsrc] malformed JSON: unexpected EOF",
		}},
		{"declined", orderSchema, []string{declinedJSON}, exitUndecided, []string{
			declinedJSON + `:1:2: [jsonsrc] undecided: member "z:nosuch": no "$xmlns" in scope binds its prefix "z"; a prefixed key names the expanded name its prefix's binding gives it, so bind "z" in a "$xmlns" member, {"z":"<namespace name>"}, leading the object of the element the key names or is an attribute of, or of an ancestor`,
		}},
		// A decline is less severe than a violation, as an undecided check is.
		{"declined beside invalid", orderSchema, []string{declinedJSON, invalidJSON}, exitInvalid, []string{
			declinedJSON + `:1:2: [jsonsrc] undecided: member "z:nosuch": no "$xmlns" in scope binds its prefix "z"; a prefixed key names the expanded name its prefix's binding gives it, so bind "z" in a "$xmlns" member, {"z":"<namespace name>"}, leading the object of the element the key names or is an attribute of, or of an ancestor`,
			invalidJSON + ":1:19: [cvc-attribute] ",
			invalidJSON + ":1:11: [cvc-identity-constraint] ",
		}},
		// Malformed JSON ranks as invalid, above a decline beside it.
		{"declined beside malformed", orderSchema, []string{declinedJSON, malformedJSON}, exitInvalid, []string{
			declinedJSON + `:1:2: [jsonsrc] undecided: member "z:nosuch": no "$xmlns" in scope binds its prefix "z"; a prefixed key names the expanded name its prefix's binding gives it, so bind "z" in a "$xmlns" member, {"z":"<namespace name>"}, leading the object of the element the key names or is an attribute of, or of an ancestor`,
			malformedJSON + ":1:18: [jsonsrc] malformed JSON: unexpected EOF",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var reports []string
			for _, flags := range [][]string{nil, {"-no-hints"}} {
				args := append(append([]string{"validate"}, flags...), "-schema", c.schema)
				args = append(args, c.instances...)
				var stdout, stderr bytes.Buffer
				if code := run(args, &stdout, &stderr); code != c.code {
					t.Errorf("run(%q) = %d, want %d (stdout %q, stderr %q)", args, code, c.code, stdout.String(), stderr.String())
				}
				if stderr.Len() != 0 {
					t.Errorf("run(%q) stderr = %q, want empty: the report is on stdout", args, stderr.String())
				}
				got := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
				if len(c.lines) == 0 {
					got = nil
				}
				if len(got) != len(c.lines) {
					t.Fatalf("run(%q) stdout =\n%s\nwant %d lines", args, stdout.String(), len(c.lines))
				}
				for i, want := range c.lines {
					prefix := strings.HasSuffix(want, "] ") || strings.HasSuffix(want, "] undecided: ")
					if got[i] == want || (prefix && strings.HasPrefix(got[i], want)) {
						continue
					}
					t.Errorf("run(%q) line %d = %q, want %q", args, i+1, got[i], want)
				}
				mustCiteOpenableLocations(t, "stdout", stdout.String())
				reports = append(reports, stdout.String())
			}
			if reports[0] != reports[1] {
				t.Errorf("-no-hints changed a JSON instance's report:\n%s\nwant\n%s", reports[1], reports[0])
			}
		})
	}
}

// TestValidateJSONReadFaultIsInvalid pins what the contract promises for an
// I/O fault reading a JSON instance after it is opened: jsonsrc reports it as
// a fault reading the input, class jsonsrc.ReadFault, and it exits 1 on an
// unmarked [jsonsrc] line, as the same fault under an XML instance exits 1 on
// an [xml-wf] line. The xml row is the parity pin: the json row's code moves
// with it. /proc/self/mem opens, and its first read faults (EIO at offset 0).
func TestValidateJSONReadFaultIsInvalid(t *testing.T) {
	const faulting = "/proc/self/mem"
	if _, err := os.ReadFile(faulting); err == nil || !strings.Contains(err.Error(), "input/output error") {
		t.Skipf("reading %s gives %v here, not the EIO this test needs", faulting, err)
	}
	for _, c := range []struct {
		format string
		code   int
		prefix string
	}{
		{"json", exitInvalid, faulting + ":1:1: [jsonsrc] fault reading the input: read " + faulting + ": input/output error"},
		{"xml", exitInvalid, faulting + ":1:1: [xml-wf] read " + faulting + ": input/output error"},
	} {
		var stdout, stderr bytes.Buffer
		args := []string{"validate", "-format", c.format, "-schema", orderSchema, faulting}
		if code := run(args, &stdout, &stderr); code != c.code {
			t.Errorf("run(%q) = %d, want %d (stdout %q, stderr %q)", args, code, c.code, stdout.String(), stderr.String())
		}
		if stdout.String() != c.prefix+"\n" {
			t.Errorf("run(%q) stdout = %q, want %q", args, stdout.String(), c.prefix+"\n")
		}
	}
}

// TestValidateJSONErrorLineSplitsByURI pins that a jsonsrc error line's
// location is split off by the instance's own name, so a path carrying a colon
// keeps it whole and the line stays openable.
func TestValidateJSONErrorLineSplitsByURI(t *testing.T) {
	body, err := os.ReadFile(malformedJSON)
	if err != nil {
		t.Fatal(err)
	}
	instance := filepath.Join(t.TempDir(), "order:1.json")
	if err := os.WriteFile(instance, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{"validate", "-schema", orderSchema, instance}
	if code := run(args, &stdout, &stderr); code != exitInvalid {
		t.Errorf("run(%q) = %d, want %d (stdout %q, stderr %q)", args, code, exitInvalid, stdout.String(), stderr.String())
	}
	want := instance + ":1:18: [jsonsrc] malformed JSON: unexpected EOF\n"
	if stdout.String() != want {
		t.Errorf("run(%q) stdout = %q, want %q", args, stdout.String(), want)
	}
	if n := mustCiteOpenableLocations(t, "stdout", stdout.String()); n != 1 {
		t.Errorf("stdout cites %d locations, want 1:\n%s", n, stdout.String())
	}
}

// TestAssessJSONHintFollowsClass pins that the class of the jsonsrc error, and
// not whether src returned an error, decides formatHint's clause: bytes src
// returns that are malformed ahead of src's error are malformed JSON and take
// it, and a document src's error cuts off is a fault reading the input and
// does not (#2792). Each instance name's extension names xml, under -format
// json.
func TestAssessJSONHintFollowsClass(t *testing.T) {
	path, err := rootLocation(orderSchema)
	if err != nil {
		t.Fatal(err)
	}
	backend := strict.New()
	schema, _, err := compileSet([]parser.Root{parser.RootAt(path)}, backend, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, err := validate.New(schema, backend)
	if err != nil {
		t.Fatal(err)
	}
	vn := &validation{base: v, backend: backend, forced: formatJSON}
	boom := errors.New("disk on fire")
	for _, c := range []struct {
		name string
		src  io.Reader
		want string
	}{
		{"malformed bytes, then a fault", io.MultiReader(strings.NewReader(`{"order",`), iotest.ErrReader(boom)),
			"order.xml:1:10: [jsonsrc] malformed JSON: invalid character ',' after object key; -format json read order.xml as json, but its extension names xml: drop -format, or pass -format xml\n"},
		{"a document a fault cuts off", io.MultiReader(strings.NewReader(`{"order":{"si`), iotest.ErrReader(boom)),
			"order.xml:1:11: [jsonsrc] fault reading the input: disk on fire\n"},
	} {
		var stdout, stderr bytes.Buffer
		if code := vn.assessJSON("order.xml", c.src, &stdout, &stderr); code != exitInvalid {
			t.Errorf("%s: assessJSON = %d, want %d (stderr %q)", c.name, code, exitInvalid, stderr.String())
		}
		if stdout.String() != c.want {
			t.Errorf("%s: stdout = %q, want %q", c.name, stdout.String(), c.want)
		}
	}
}

// TestJSONSrcLineUntaughtClassIsDeclined pins jsonsrcLine's fallbacks: a
// *jsonsrc.Error whose Class() has no arm there, the zero value being the one
// a caller can build, and an error that is no *jsonsrc.Error each print the
// marked decline line and exit 4, never charging the instance. Under -format
// json on an xml-named instance, so a fallback taking the Malformed arm would
// also add formatHint's clause.
func TestJSONSrcLineUntaughtClassIsDeclined(t *testing.T) {
	vn := &validation{forced: formatJSON}
	for _, c := range []struct {
		name string
		err  error
		want string
	}{
		{"zero Class", &jsonsrc.Error{}, "?: [jsonsrc] undecided: "},
		{"no *jsonsrc.Error", errors.New("jsonsrc: Validate: nil io.Reader"), "order.xml: [jsonsrc] undecided: jsonsrc: Validate: nil io.Reader"},
	} {
		line, code := vn.jsonsrcLine("order.xml", c.err)
		if code != exitUndecided {
			t.Errorf("%s: jsonsrcLine code = %d, want %d", c.name, code, exitUndecided)
		}
		if line != c.want {
			t.Errorf("%s: jsonsrcLine line = %q, want %q", c.name, line, c.want)
		}
	}
}

// readmeOrderXSD is README's order.xsd, as the README examples run it.
const readmeOrderXSD = "../../testdata/readme/order.xsd"

// TestValidateJSONScalarWhereObjectExpected pins the clause jsonsrc appends to
// the cvc-complex-type charge at a scalar member whose element's type has
// element-only content: README's order.xsd, readmeOrderXSD, on
// readme-text.json, {"order":"just text"}, which exits 1 on one line opening
// at the scalar's location and naming "order" in the clause.
func TestValidateJSONScalarWhereObjectExpected(t *testing.T) {
	const instance = "testdata/readme-text.json"
	args := []string{"validate", "-schema", readmeOrderXSD, instance}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != exitInvalid {
		t.Errorf("run(%q) = %d, want %d (stdout %q, stderr %q)", args, code, exitInvalid, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("run(%q) stderr = %q, want empty: the report is on stdout", args, stderr.String())
	}
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("run(%q) stdout =\n%s\nwant 1 line", args, stdout.String())
	}
	const prefix = instance + ":1:10: [cvc-complex-type] "
	const clause = `; JSON member "order" has the scalar value "just text", but the type of its element has element-only content: object expected, {"order":{...}}`
	if !strings.HasPrefix(lines[0], prefix) || !strings.HasSuffix(lines[0], clause) {
		t.Errorf("run(%q) line = %q, want it to open %q and end %q", args, lines[0], prefix, clause)
	}
	mustCiteOpenableLocations(t, "stdout", stdout.String())
}

// TestValidateJSONMemberClauses pins the clauses jsonsrc appends to the charge
// a parent's content takes at a child member, on README's order.xsd,
// readmeOrderXSD: an unknown key, which lists the keys the type
// carries; "_kind", which also names kind as the attribute's key; and an
// object under the simple-typed item, which says a scalar is expected. Each
// instance exits 1 on one line opening at the member's location and rule and
// ending with the whole clause.
func TestValidateJSONMemberClauses(t *testing.T) {
	for _, c := range []struct {
		name, doc, at, clause string
	}{
		{"unknown key", `{"order":{"kind":"x","colour":"red","item":"a"}}`, ":1:22: [cvc-complex-content] ",
			`; JSON member "colour" names no child element declaration and no attribute use of the type of "order", whose members are keyed "kind", "item"`},
		{"attribute key behind _", `{"order":{"_kind":"x","item":"a"}}`, ":1:11: [cvc-complex-content] ",
			`; JSON member "_kind" names no child element declaration and no attribute use of the type of "order", whose members are keyed "kind", "item"; an attribute member is keyed by the attribute's own name, "kind", not "_kind"`},
		{"object where a scalar is expected", `{"order":{"item":"a","item":{"y":"b"}}}`, ":1:30: [cvc-type] ",
			`; JSON member "y" is a member of the object of "item", whose type is a simple type: scalar expected, {"item":<string, number or boolean>}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			instance := filepath.Join(t.TempDir(), "order.json")
			if err := os.WriteFile(instance, []byte(c.doc), 0o600); err != nil {
				t.Fatal(err)
			}
			args := []string{"validate", "-schema", readmeOrderXSD, instance}
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != exitInvalid {
				t.Errorf("run(%q) = %d, want %d (stdout %q, stderr %q)", args, code, exitInvalid, stdout.String(), stderr.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("run(%q) stderr = %q, want empty: the report is on stdout", args, stderr.String())
			}
			lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
			if len(lines) != 1 {
				t.Fatalf("run(%q) stdout =\n%s\nwant 1 line", args, stdout.String())
			}
			prefix := instance + c.at
			if !strings.HasPrefix(lines[0], prefix) || !strings.HasSuffix(lines[0], c.clause) {
				t.Errorf("run(%q) line = %q, want it to open %q and end %q", args, lines[0], prefix, c.clause)
			}
			mustCiteOpenableLocations(t, "stdout", stdout.String())
		})
	}
}

// TestValidateFormatMismatchClause pins the clause formatHint ends a line
// with, on README's order.xsd (readmeOrderXSD) and order.json, with a
// well-formed order.xml: -format json over order.xml and -format xml over
// order.json, each keeping its token and exit code. Three more rows carry it:
// content after the top-level object and no JSON value at all under -format
// json, and under -format xml a fault the walk meets inside the document
// element, which reaches validate.Result.Err. The other rows print no
// clause, each under the guard named in its name: the forced format matching
// the extension, no -format, an extension naming no format and standard input,
// an extension naming the reserved ber, a decline, and a read fault under each
// format. want is the whole of stdout.
func TestValidateFormatMismatchClause(t *testing.T) {
	dir := t.TempDir()
	at := func(name string) string { return filepath.Join(dir, name) }
	const orderXML = `<o:order xmlns:o="urn:o" kind="x"><o:item>a</o:item></o:order>` + "\n"
	for name, body := range map[string]string{
		"order.json":   `{"o:order":{"$xmlns":{"o":"urn:o"},"kind":"x","item":["a","b"]}}` + "\n",
		"order.xml":    orderXML,
		"order.txt":    orderXML,
		"order.ber":    orderXML,
		"hello.json":   "hello\n",
		"declined.xml": `{"z:nosuch":{}}` + "\n",
		"after.xml":    `{"o:order":{"$xmlns":{"o":"urn:o"},"kind":"x","item":["a"]}} {}` + "\n",
		"empty.xml":    "",
		"mid.json":     `<o:order xmlns:o="urn:o"><o:item>a</o:order>` + "\n",
	} {
		if err := os.WriteFile(at(name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const notJSON = ":1:1: [jsonsrc] malformed JSON: invalid character '<' looking for beginning of value"
	for _, c := range []struct {
		name  string
		args  []string
		stdin string
		code  int
		want  string
	}{
		{"json forced on xml", []string{"-format", "json", at("order.xml")}, "", exitInvalid,
			at("order.xml") + notJSON + "; -format json read " + at("order.xml") + " as json, but its extension names xml: drop -format, or pass -format xml\n"},
		{"xml forced on json", []string{"-format", "xml", at("order.json")}, "", exitInvalid,
			at("order.json") + ":1:1: [xml-wf] character data before the document element: only an XML declaration, a DOCTYPE, comments, processing instructions and white space may precede it (XML 1.0 [1] document, [22] prolog, [27] Misc); -format xml read " + at("order.json") + " as xml, but its extension names json: drop -format, or pass -format json\n"},
		{"json forced on xml, content after the top-level object", []string{"-format", "json", at("after.xml")}, "", exitInvalid,
			at("after.xml") + ":1:62: [jsonsrc] content follows the top-level object; -format json read " + at("after.xml") + " as json, but its extension names xml: drop -format, or pass -format xml\n"},
		{"json forced on xml, no JSON value", []string{"-format", "json", at("empty.xml")}, "", exitInvalid,
			at("empty.xml") + `:1:1: [jsonsrc] the document holds no JSON value; a JSON instance is one object whose one member names the validation root, {"<root>":...}; -format json read ` + at("empty.xml") + " as json, but its extension names xml: drop -format, or pass -format xml\n"},
		{"xml forced on json, fault the walk meets", []string{"-format", "xml", at("mid.json")}, "", exitInvalid,
			at("mid.json") + ":1:35: [xml-wf] end tag </o:order> does not match open element {urn:o}item; -format xml read " + at("mid.json") + " as xml, but its extension names json: drop -format, or pass -format json\n"},
		{"forced format named by the extension", []string{"-format", "json", malformedJSON}, "", exitInvalid,
			malformedJSON + ":1:18: [jsonsrc] malformed JSON: unexpected EOF\n"},
		{"no -format", []string{at("hello.json")}, "", exitInvalid,
			at("hello.json") + ":1:1: [jsonsrc] malformed JSON: invalid character 'h' looking for beginning of value\n"},
		{"extension naming no format", []string{"-format", "json", at("order.txt")}, "", exitInvalid,
			at("order.txt") + notJSON + "\n"},
		{"standard input", []string{"-format", "json", stdinArg}, orderXML, exitInvalid,
			stdinArg + notJSON + "\n"},
		{"extension naming the reserved ber", []string{"-format", "json", at("order.ber")}, "", exitInvalid,
			at("order.ber") + notJSON + "\n"},
		{"decline", []string{"-format", "json", at("declined.xml")}, "", exitUndecided,
			at("declined.xml") + `:1:2: [jsonsrc] undecided: member "z:nosuch": no "$xmlns" in scope binds its prefix "z"; a prefixed key names the expanded name its prefix's binding gives it, so bind "z" in a "$xmlns" member, {"z":"<namespace name>"}, leading the object of the element the key names or is an attribute of, or of an ancestor` + "\n"},
		{"read fault under json", []string{"-format", "json", at("fault.xml")}, "", exitInvalid,
			at("fault.xml") + ":1:1: [jsonsrc] fault reading the input: read " + at("fault.xml") + ": input/output error\n"},
		{"read fault under xml", []string{"-format", "xml", at("fault.json")}, "", exitInvalid,
			at("fault.json") + ":1:1: [xml-wf] read " + at("fault.json") + ": input/output error\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if strings.HasPrefix(c.name, "read fault") {
				// /proc/self/mem opens, and its first read faults (EIO at offset
				// 0), as TestValidateJSONReadFaultIsInvalid reads it; the link
				// gives it an extension naming the other format.
				const faulting = "/proc/self/mem"
				if _, err := os.ReadFile(faulting); err == nil || !strings.Contains(err.Error(), "input/output error") {
					t.Skipf("reading %s gives %v here, not the EIO this row needs", faulting, err)
				}
				if err := os.Symlink(faulting, c.args[len(c.args)-1]); err != nil {
					t.Fatal(err)
				}
			}
			if c.stdin != "" {
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := w.WriteString(c.stdin); err != nil {
					t.Fatal(err)
				}
				if err := w.Close(); err != nil {
					t.Fatal(err)
				}
				old := os.Stdin
				os.Stdin = r
				t.Cleanup(func() { os.Stdin = old; _ = r.Close() })
			}
			args := append([]string{"validate", "-schema", readmeOrderXSD}, c.args...)
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != c.code {
				t.Errorf("run(%q) = %d, want %d (stderr %q)", args, code, c.code, stderr.String())
			}
			if stdout.String() != c.want {
				t.Errorf("run(%q) stdout =\n%q\nwant\n%q", args, stdout.String(), c.want)
			}
		})
	}
}

// reportLine is one line a report is expected to hold: its opening, the
// "<loc>: [<rule>] " a line's own location and rule spell, and whether it ends
// with formatHint's clause.
type reportLine struct {
	opening string
	clause  bool
}

// mustReport fails unless report holds exactly want's lines, in want's order,
// each opening with its opening and holding clause, at its end, exactly when
// its clause is set. The message text between them is the library's, which
// these rows do not pin.
func mustReport(t *testing.T, report, clause string, want []reportLine) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(report, "\n"), "\n")
	if len(lines) != len(want) || !strings.HasSuffix(report, "\n") {
		t.Fatalf("report =\n%s\nwant %d lines", report, len(want))
	}
	for i, w := range want {
		if !strings.HasPrefix(lines[i], w.opening) {
			t.Errorf("line %d = %q, want it to open %q", i+1, lines[i], w.opening)
		}
		if strings.Contains(lines[i], clause) != w.clause {
			t.Errorf("line %d = %q holds %q: %v, want %v", i+1, lines[i], clause, !w.clause, w.clause)
		}
		if w.clause && !strings.HasSuffix(lines[i], clause) {
			t.Errorf("line %d = %q, want it to end %q", i+1, lines[i], clause)
		}
	}
}

// TestAssessXMLReadFaultTheWalkMeetsTakesNoClause pins that a fault reading
// the instance keeps formatHint's clause off its [xml-wf] line when the walk
// meets it inside the document element, where it reaches validate.Result.Err
// rather than xmlsrc.Validate's error. The reader serves one byte at a time,
// so instanceHints' read of the start tag never reaches the fault, and the
// undecided lines after the fault line, one per box the walk assessed before
// it, are what only the Result.Err arm prints. -format xml over a json-named
// instance, so a clause the guard let through would end the fault line.
func TestAssessXMLReadFaultTheWalkMeetsTakesNoClause(t *testing.T) {
	path, err := rootLocation(assertedSchema)
	if err != nil {
		t.Fatal(err)
	}
	roots := []parser.Root{parser.RootAt(path)}
	backend := strict.New()
	schema, _, err := compileSet(roots, backend, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, err := validate.New(schema, backend)
	if err != nil {
		t.Fatal(err)
	}
	vn := &validation{roots: roots, base: v, backend: backend, forced: formatXML, hints: true}
	const instance = "midfault.json"
	const doc = `<a:boxes xmlns:a="http://example.com/asserted"><a:box width="3"/>` + "\n" + `<a:box width="3"/>`
	src := iotest.OneByteReader(io.MultiReader(strings.NewReader(doc), iotest.ErrReader(errors.New("disk on fire"))))
	var stdout, stderr bytes.Buffer
	if code := vn.assessXML(instance, src, &stdout, &stderr); code != exitInvalid {
		t.Errorf("assessXML = %d, want %d (stderr %q)", code, exitInvalid, stderr.String())
	}
	mustReport(t, stdout.String(), "; -format xml read "+instance, []reportLine{
		{instance + ":2:19: [xml-wf] disk on fire", false},
		{instance + ":1:48: [cvc-assertion] undecided: ", false},
		{instance + ":2:1: [cvc-assertion] undecided: ", false},
	})
}

// TestValidateViolationsBeforeAWalkMetFaultTakeNoClause pins that formatHint's
// clause ends the [xml-wf] line of the fault the walk met and no other: the
// violations charged before it and the undecided lines after it print without
// it. -format xml over a json-named instance of asserted.xsd whose first box
// is charged an undeclared attribute, whose second box's width is no int, and
// whose boxes is closed by a mismatched end tag. The lines are pinned in
// order, so the row pins the report's order too: violations, then the fault,
// then the undecided checks.
func TestValidateViolationsBeforeAWalkMetFaultTakeNoClause(t *testing.T) {
	instance := filepath.Join(t.TempDir(), "boxes.json")
	const doc = `<a:boxes xmlns:a="http://example.com/asserted"><a:box width="3" extra="1"/><a:box width="x"/></a:wrong>` + "\n"
	if err := os.WriteFile(instance, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"validate", "-format", "xml", "-schema", assertedSchema, instance}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != exitInvalid {
		t.Errorf("run(%q) = %d, want %d (stderr %q)", args, code, exitInvalid, stderr.String())
	}
	mustReport(t, stdout.String(), "; -format xml read "+instance+" as xml, but its extension names json: drop -format, or pass -format json", []reportLine{
		{instance + ":1:65: [cvc-complex-type] ", false},
		{instance + ":1:83: [cvc-attribute] ", false},
		{instance + ":1:94: [xml-wf] end tag </a:wrong> does not match open element {http://example.com/asserted}boxes", true},
		{instance + ":1:48: [cvc-assertion] undecided: ", false},
		{instance + ":1:76: [cvc-assertion] undecided: ", false},
	})
	mustCiteOpenableLocations(t, "stdout", stdout.String())
}

// TestValidateSchemaFlagIsRepeatable pins the grammar doc.go states and the
// flag.Value implements: every schema needs its own -schema, so a second
// location standing beside the first is a positional argument — an instance —
// and not a second schema.
func TestValidateSchemaFlagIsRepeatable(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"validate", "-schema", orderSchema, hintedSchema, validInstance}
	if code := run(args, &stdout, &stderr); code != exitUsage {
		t.Fatalf("code = %d, want %d (stderr %q)", code, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), hintedSchema) {
		t.Errorf("stderr = %q, want %s reported as an instance argument", stderr.String(), hintedSchema)
	}
}

// TestValidateUsageErrors covers the usage/IO code's whole surface for this
// subcommand: each diagnosis reaches stderr, stdout stays empty, and no two of
// them collapse into one line. A diagnosis of the command line's shape has
// validateUsageLine and helpPointer under it; an argument that cannot be read
// is its one line alone (#3333).
func TestValidateUsageErrors(t *testing.T) {
	dir := t.TempDir()
	xmlDir := filepath.Join(dir, "d.xml")
	forcedDir := filepath.Join(dir, "forced")
	plainDir := filepath.Join(dir, "plain")
	linkDir := filepath.Join(dir, "link.xml")
	for _, d := range []string{xmlDir, forcedDir, plainDir} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(xmlDir, linkDir); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
		want string
		// shape marks a diagnosis of the command line's shape, which
		// validateUsageLine and helpPointer follow; any other row's line stands
		// alone (#3333).
		shape bool
	}{
		{"no schema", []string{"validate", validInstance}, "goxsd8: validate: no schema given", true},
		// The schema named after the instance (#1290). The seen map below is
		// what this row is for: this answer was byte-identical to the row above
		// it, so a user who named a schema was told they had not.
		{"schema after the instance", []string{"validate", validInstance, "-schema", orderSchema}, fmt.Sprintf(flagAfterPositionalFmt, "validate", "-schema"), true},
		{"no instance", []string{"validate", "-schema", orderSchema}, "goxsd8: validate: no instance given", true},
		{"dangling -schema", []string{"validate", "-schema"}, "flag needs an argument: -schema", true},
		{"undefined flag", []string{"validate", "-out", dir, "-schema", orderSchema, validInstance}, "flag provided but not defined: -out", true},
		{"missing schema", []string{"validate", "-schema", "testdata/nosuch.xsd", validInstance}, "no such file or directory", false},
		{"schema is a directory", []string{"validate", "-schema", dir, validInstance}, "is a directory", false},
		{"schema from stdin", []string{"validate", "-schema", "-", validInstance}, "standard input is not a schema location", true},
		{"missing instance", []string{"validate", "-schema", orderSchema, "testdata/nosuch.xml"}, "no such file or directory", false},
		// - names no source format, and xml and json are both assessed, so
		// standard input needs -format (#2403).
		{"stdin without -format", []string{"validate", "-schema", orderSchema, "-"}, "goxsd8: validate: -: standard input has no extension to name a source format; pass -format with one of xml, json or ber", true},
		// A directory instance is charged as a directory before formatOf reads
		// its extension, whatever its name and whatever -format forced (#2403):
		// "." and the extension-less directory answered with the extension
		// message without the check, and the .xml-named and forced ones with an
		// exit-1 [xml-wf] read failure.
		{"instance is .", []string{"validate", "-schema", orderSchema, "."}, "goxsd8: validate: open .: is a directory", false},
		{"instance is a directory", []string{"validate", "-schema", orderSchema, plainDir}, "goxsd8: validate: open " + plainDir + ": is a directory", false},
		{"instance is a directory named .xml", []string{"validate", "-schema", orderSchema, xmlDir}, "goxsd8: validate: open " + xmlDir + ": is a directory", false},
		{"instance is a directory under -format", []string{"validate", "-format", "xml", "-schema", orderSchema, forcedDir}, "goxsd8: validate: open " + forcedDir + ": is a directory", false},
		{"instance is a link to a directory", []string{"validate", "-schema", orderSchema, linkDir}, "goxsd8: validate: open " + linkDir + ": is a directory", false},
		// -help=true is not one of the three help spellings, at any position
		// (doc.go's argument vocabulary); after a subcommand it is a flag whose
		// value that subcommand does not accept.
		{"help as a flag value", []string{"validate", "-help=true"}, fmt.Sprintf(helpNotAFlagValueFmt, "validate"), true},
	}
	seen := make(map[string]string)
	for _, c := range cases {
		var stdout, stderr bytes.Buffer
		if code := run(c.args, &stdout, &stderr); code != exitUsage {
			t.Errorf("%s: run(%q) = %d, want %d", c.name, c.args, code, exitUsage)
		}
		if stdout.Len() != 0 {
			t.Errorf("%s: stdout = %q, want empty", c.name, stdout.String())
		}
		if !strings.Contains(stderr.String(), c.want) {
			t.Errorf("%s: stderr = %q, want it to contain %q", c.name, stderr.String(), c.want)
		}
		if synopsis := strings.HasSuffix(stderr.String(), "\n"+validateUsageLine+"\n"+helpPointer+"\n"); synopsis != c.shape {
			t.Errorf("%s: stderr = %q, synopsis and helpPointer under it = %t, want %t", c.name, stderr.String(), synopsis, c.shape)
		}
		if !c.shape && strings.Count(stderr.String(), "\n") != 1 {
			t.Errorf("%s: stderr = %q, want one line", c.name, stderr.String())
		}
		if other, ok := seen[stderr.String()]; ok {
			t.Errorf("%s and %s produce the same diagnosis %q", c.name, other, stderr.String())
		}
		seen[stderr.String()] = c.name
	}
}

// TestValidateSchemaFileNamedDash pins the escape hatch the -schema -
// refusal leaves open: the exact spelling - is refused and nothing else is, so
// a file genuinely named - is still compiled as a schema document when a path
// names it — ./- from its own directory, or an absolute path ending in it.
//
// The instance it rejects is asserted too, because an argument that was read
// and an argument that was ignored both exit 0 on a clean instance.
func TestValidateSchemaFileNamedDash(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	dash := write("-", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:x"><xs:element name="root" type="xs:int"/></xs:schema>`)
	valid := write("valid.xml", `<root xmlns="urn:x">42</root>`)
	invalid := write("invalid.xml", `<root xmlns="urn:x">forty-two</root>`)

	// ./- names that file only from the directory holding it, so the run happens
	// there; the absolute path names the same file from anywhere.
	t.Chdir(dir)
	for _, schema := range []string{dash, "./-"} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"validate", "-schema", schema, valid}, &stdout, &stderr); code != exitOK {
			t.Errorf("-schema %s: code = %d, want %d (stdout %q, stderr %q)", schema, code, exitOK, stdout.String(), stderr.String())
		}
		stdout.Reset()
		stderr.Reset()
		if code := run([]string{"validate", "-schema", schema, invalid}, &stdout, &stderr); code != exitInvalid {
			t.Errorf("-schema %s on the invalid instance: code = %d, want %d (stdout %q, stderr %q)", schema, code, exitInvalid, stdout.String(), stderr.String())
		}
	}
}

// TestValidateAdversarialArguments is the robustness bar the stub set and this
// landing must not fall below (#472, the 2026-08-12 cliuser pass): no panic,
// an exit code in the contract's four, and the streams kept apart, whatever
// the argument list. None of these names a readable schema set and a readable
// instance, so every one is a usage or IO fault.
func TestValidateAdversarialArguments(t *testing.T) {
	lists := [][]string{
		{"validate", ""},
		{"validate", "--"},
		{"validate", "\x00\x01"},
		{"validate", "-schema", "", validInstance},
		{"validate", "-schema", "\x00\x01", validInstance},
		{"validate", "-schema", "--", validInstance},
		{"validate", "-schema"},
		{"validate", "-format"},
		{"validate", "-out"},
		{"validate", "-no-hints"},
		{"validate", "-schema", orderSchema, ""},
		{"validate", "-schema", orderSchema, "\x00\x01.xml"},
		{"validate", "-schema", strings.Repeat("a/", 200) + "x.xsd", validInstance},
		{"validate", "-schema", orderSchema, strings.Repeat("a/", 200) + "x.xml"},
	}
	for _, args := range lists {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != exitUsage {
			t.Errorf("run(%q) = %d, want %d", args, code, exitUsage)
		}
		if stdout.Len() != 0 {
			t.Errorf("run(%q) stdout = %q, want empty", args, stdout.String())
		}
		if stderr.Len() == 0 {
			t.Errorf("run(%q) reported nothing on stderr", args)
		}
	}
}

// TestValidateFlagAfterPositional pins the same consequence parse's own test
// does, and the same reversal (#1290): a subcommand's flags precede its
// positional arguments, and a trailing -no-hints is now reported as a
// misplaced flag rather than read as an instance argument. Until then this
// test asserted the trailing token reaching the source reader as a path.
func TestValidateFlagAfterPositional(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"validate", "-schema", orderSchema, hintedInstance, "-no-hints"}
	if code := run(args, &stdout, &stderr); code != exitUsage {
		t.Fatalf("code = %d, want %d — the trailing -no-hints is a misplaced flag", code, exitUsage)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty: nothing is assessed once an argument is diagnosed", stdout.String())
	}
	want := fmt.Sprintf(flagAfterPositionalFmt, "validate", "-no-hints")
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
	}
}

// TestValidateEndOfOptions pins doc.go's reading of -- for validate (#3332):
// one the flag package consumed before the first instance argument ends
// validate's options, so every later argument is an instance, a second
// flag-shaped one included. A -- that is -schema's value ends nothing, so a
// flag-shaped token after the first instance is still misplaced, and so is a
// -- standing after an instance. The files named like flags are valid
// instances, so a row reading them as paths exits 0.
func TestValidateEndOfOptions(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []struct{ name, from string }{
		{"order.xsd", orderSchema}, {"a.xml", validInstance}, {"-weird.xml", validInstance}, {"-q.xml", validInstance},
	} {
		name := f.name
		body, err := os.ReadFile(f.from)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	misplaced := func(arg string) string {
		return fmt.Sprintf(flagAfterPositionalFmt, "validate", arg) + "\n" + validateUsageLine + "\n" + helpPointer + "\n"
	}
	rows := []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{"operand after --", []string{"-schema", "order.xsd", "--", "-weird.xml"}, exitOK, ""},
		{"two flag-shaped operands after --", []string{"-schema", "order.xsd", "--", "-weird.xml", "-q.xml"}, exitOK, ""},
		{"-- after a boolean flag", []string{"-schema", "order.xsd", "-no-hints", "--", "-weird.xml"}, exitOK, ""},
		{"-- after a flag spelled with =", []string{"-schema=order.xsd", "--", "-weird.xml"}, exitOK, ""},
		{"-- as -schema's value", []string{"-schema", "--", "a.xml", "-weird.xml"}, exitUsage, misplaced("-weird.xml")},
		{"-- after a positional", []string{"-schema", "order.xsd", "a.xml", "--", "-weird.xml"}, exitUsage, misplaced("--")},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(append([]string{"validate"}, r.args...), &stdout, &stderr); code != r.code {
				t.Errorf("validate %q = %d, want %d", r.args, code, r.code)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if stderr.String() != r.stderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), r.stderr)
			}
		})
	}
}

// TestValidateExtensionCaseSensitive pins usage's claim that the extension is
// matched case-sensitively, as -format's token is: a.XML names no source
// format and is the usage error listing the values (#1318).
func TestValidateExtensionCaseSensitive(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"validate", "-schema", orderSchema, "a.XML"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("code = %d, want %d (stderr %q)", code, exitUsage, stderr.String())
	}
	want := `goxsd8: validate: a.XML: the extension ".XML" names no source format; pass -format with one of xml, json or ber` + "\n"
	if !strings.HasPrefix(stderr.String(), want) {
		t.Errorf("stderr = %q, want it to begin %q", stderr.String(), want)
	}
}

// TestValidateMalformedInstanceIsAVerdict pins where a source fault lands: an
// instance the reader cannot finish was not shown valid, so the fault is
// rendered beside the violations on stdout and charged the invalid code — it
// is a verdict about the document, not a fault in the run.
func TestValidateMalformedInstanceIsAVerdict(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"validate", "-schema", orderSchema, "testdata/malformed.xml"}
	if code := run(args, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("code = %d, want %d (stderr %q)", code, exitInvalid, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "[xml-wf]") {
		t.Errorf("stdout = %q, want the well-formedness fault reported", stdout.String())
	}
}

// TestValidateNeverReadsTheExternalDTDSubset pins the policy usage publishes:
// the external DTD subset is never fetched, so an ENTITY value whose unparsed
// entity is declared only there is charged — with a cvc-simple-type cause
// saying the DTD was not fully read, not that the name is undeclared — and the
// report is the same byte for byte whether the external subset's file exists
// beside the instance or not. An entity declared through an internal parameter
// entity is read.
func TestValidateNeverReadsTheExternalDTDSubset(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	schema := write("ent.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:element name="r"><xs:complexType><xs:attribute name="ent" type="xs:ENTITY"/></xs:complexType></xs:element></xs:schema>`)
	external := write("external.xml", `<!DOCTYPE r SYSTEM "x.dtd"><r ent="pic"/>`)
	internal := write("internal.xml", `<!DOCTYPE r [<!ENTITY % p "<!ENTITY pic SYSTEM 'u.bin' NDATA n>"> %p; <!NOTATION n SYSTEM "x">]><r ent="pic"/>`)

	report := func() string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := run([]string{"validate", "-schema", schema, external}, &stdout, &stderr); code != exitInvalid {
			t.Fatalf("code = %d, want %d (stdout %q, stderr %q)", code, exitInvalid, stdout.String(), stderr.String())
		}
		return stdout.String()
	}
	absent := report()
	if !strings.Contains(absent, "[cvc-attribute]") || !strings.Contains(absent, "[cvc-simple-type]") || !strings.Contains(absent, "the document's DTD was not fully read") {
		t.Errorf("stdout = %q, want a cvc-attribute charge with a cvc-simple-type cause saying the DTD was not fully read", absent)
	}
	write("x.dtd", `<!NOTATION n SYSTEM "x"><!ENTITY pic SYSTEM "u.bin" NDATA n>`)
	if present := report(); present != absent {
		t.Errorf("with x.dtd present, stdout = %q, want it identical to the absent case's %q", present, absent)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"validate", "-schema", schema, internal}, &stdout, &stderr); code != exitOK {
		t.Errorf("internal parameter entity: code = %d, want %d (stdout %q, stderr %q)", code, exitOK, stdout.String(), stderr.String())
	}
}

// TestValidateStrayDTDTextIsNotWellFormed pins that text between internal
// subset declarations which is neither S nor a PEReference — at depth 0, in a
// parameter entity's replacement text, or a '%' run that is no PEReference
// under standalone="yes" — makes the instance not well-formed, charged the
// invalid code with an [xml-wf] verdict, though the ENTITY value names an
// unparsed entity declared beside it; the same subset without the stray text
// validates.
func TestValidateStrayDTDTextIsNotWellFormed(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	schema := write("ent.xsd", `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:element name="r"><xs:complexType><xs:attribute name="ent" type="xs:ENTITY"/></xs:complexType></xs:element></xs:schema>`)
	const decls = `<!NOTATION n SYSTEM 'x'><!ENTITY pic SYSTEM 'u' NDATA n>`
	for _, tc := range []struct {
		doc  string
		want int
	}{
		{`<!DOCTYPE r [` + decls + `]><r ent="pic"/>`, exitOK},
		{`<!DOCTYPE r [` + decls + ` junk]><r ent="pic"/>`, exitInvalid},
		{`<!DOCTYPE r [<!NOTATION n SYSTEM 'x'><!ENTITY % p "<!ENTITY pic SYSTEM 'u' NDATA n> junk"> %p;]><r ent="pic"/>`, exitInvalid},
		{`<?xml version="1.0" standalone="yes"?><!DOCTYPE r [%1x; ` + decls + `]><r ent="pic"/>`, exitInvalid},
	} {
		var stdout, stderr bytes.Buffer
		code := run([]string{"validate", "-schema", schema, write("i.xml", tc.doc)}, &stdout, &stderr)
		if code != tc.want {
			t.Errorf("%s: code = %d, want %d (stdout %q, stderr %q)", tc.doc, code, tc.want, stdout.String(), stderr.String())
			continue
		}
		if tc.want == exitInvalid && !strings.Contains(stdout.String(), "[xml-wf]") {
			t.Errorf("%s: stdout = %q, want an [xml-wf] verdict", tc.doc, stdout.String())
		}
	}
}

// TestValidateLocatesAnAttributeChargeAtTheAttribute pins where a charge on an
// attribute is located: at the first byte of the attribute's name, on
// whatever line of the start tag it stands, while a charge on the element for
// an attribute it lacks stays at the start tag's "<".
func TestValidateLocatesAnAttributeChargeAtTheAttribute(t *testing.T) {
	t.Chdir(t.TempDir())
	const schema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:element name="r"><xs:complexType><xs:attribute name="id" type="xs:int" use="required"/></xs:complexType></xs:element></xs:schema>`
	if err := os.WriteFile("s.xsd", []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ doc, opening string }{
		{`<r id="1x"/>`, `i.xml:1:4: [cvc-attribute] attribute id's value "1x" `},
		{"<r\n\t id = '1x'/>", `i.xml:2:3: [cvc-attribute] attribute id's value "1x" `},
		{`<r/>`, `i.xml:1:1: [cvc-complex-type] `},
	} {
		if err := os.WriteFile("i.xml", []byte(c.doc), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		if code := run([]string{"validate", "-schema", "s.xsd", "i.xml"}, &stdout, &stderr); code != exitInvalid {
			t.Fatalf("%q: code = %d, want %d (stdout %q, stderr %q)", c.doc, code, exitInvalid, stdout.String(), stderr.String())
		}
		if line, _, _ := strings.Cut(stdout.String(), "\n"); !strings.HasPrefix(line, c.opening) {
			t.Errorf("%q: first line = %q, want it to open %q", c.doc, line, c.opening)
		}
	}
}
