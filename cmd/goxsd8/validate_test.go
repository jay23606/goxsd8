package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The fixtures every outcome test turns on: the schema parse's own tests
// compile, an instance of it that charges nothing, one whose sku lexical is
// outside its Sku pattern, a second namespace's schema paired with an
// instance that names it through xsi:schemaLocation and nothing else, and a
// schema carrying an <xs:assert> whose instance charges nothing and leaves
// the assertion unevaluated, and a schema whose own <xs:include> names a
// document that is not there with an instance of it either way.
const (
	validInstance        = "testdata/order-valid.xml"
	invalidInstance      = "testdata/order-invalid.xml"
	orderSchema          = "testdata/order.xsd"
	hintedInstance       = "testdata/hinted.xml"
	hintedSchema         = "testdata/hinted.xsd"
	assertedSchema       = "testdata/asserted.xsd"
	assertedInstance     = "testdata/asserted.xml"
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

// TestValidateRendersDelegatedVerdictWithoutPlaceholder pins the whole line a
// delegating charge prints: the String Valid verdict it wraps carries no
// position of its own, and renders as "[rule] msg" after the outer charge's
// sentence rather than behind the zero Loc's "?: " (#1844).
func TestValidateRendersDelegatedVerdictWithoutPlaceholder(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"validate", "-schema", orderSchema, invalidInstance}, &stdout, &stderr); code != exitInvalid {
		t.Fatalf("code = %d, want %d (stderr %q)", code, exitInvalid, stderr.String())
	}
	const want = invalidInstance + `:5:3: [cvc-attribute] the ·initial value· of the attribute sku is not ·valid· with respect to its declaration's {type definition} {http://example.com/order}Sku, which cvc-attribute clause 3 requires as per String Valid (§3.16.4): [cvc-pattern-valid] "nope" matches no member of the pattern facet of the simple type {http://example.com/order}Sku, whose {value} holds "[A-Z]{3}-[0-9]{4}", but cvc-pattern-valid requires one to match`
	if line, _, _ := strings.Cut(stdout.String(), "\n"); line != want {
		t.Errorf("first line =\n%s\nwant\n%s", line, want)
	}
}

// TestValidateRendersALexicalSpaceFaultWhole pins the whole line validate
// prints for <amount>12,50</amount> against an element of type xs:decimal:
// the cvc-datatype-valid cause the strict backend builds opens with the
// lexical and the type and closes with its rule inline (STYLE E5), with no
// trailing production-and-section parenthetical (#2371). The outer cvc-type
// sentence is pinned as validate renders it today; #2370 owns its wording.
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
	want := instance + `:1:1: [cvc-type] the ·initial value· of the element amount is not ·valid· with respect to its ·governing type definition· {http://www.w3.org/2001/XMLSchema}decimal, which cvc-type clause 3.1.3 requires as per String Valid (§3.16.4): [cvc-datatype-valid] "12,50" is not in the lexical space of decimal, which cvc-datatype-valid clause 2.1 requires it to be in` + "\n"
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
// type's pattern rejection names the pattern it failed and the type that
// declares it — xs:integer's [\-+]?[0-9]+ — rather than reading as a pattern
// the schema author wrote (#2310).
func TestValidateNamesTheBuiltinPatternAndItsDeclaringType(t *testing.T) {
	_, code, stdout, stderr := runIntPattern(t)
	if code != exitInvalid {
		t.Fatalf("code = %d, want %d (stdout %q, stderr %q)", code, exitInvalid, stdout, stderr)
	}
	const want = `[cvc-pattern-valid] "x3" matches no member of the pattern facet of the simple type {http://www.w3.org/2001/XMLSchema}integer, whose {value} holds "[\-+]?[0-9]+", but cvc-pattern-valid requires one to match`
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout =\n%s\nwant it to carry\n%s", stdout, want)
	}
}

// TestValidateIntPatternRuleAndExitCode characterizes the rule a lexical
// outside xs:int's inherited pattern is charged under: exit 1, cvc-type
// clause 3.1.3 wrapping cvc-pattern-valid. The lexical also fails xs:int's
// lexical mapping (cvc-datatype-valid clause 2.1) and the spec orders neither
// clause first, so this pins the repository's choice — the pattern stage
// runs before the mapping — against a re-charge under another rule (#2310).
func TestValidateIntPatternRuleAndExitCode(t *testing.T) {
	instance, code, stdout, stderr := runIntPattern(t)
	if code != exitInvalid {
		t.Fatalf("code = %d, want %d (stdout %q, stderr %q)", code, exitInvalid, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	line, _, _ := strings.Cut(stdout, "\n")
	if prefix := instance + ":1:1: [cvc-type] "; !strings.HasPrefix(line, prefix) {
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

// TestValidateUndecidedInstanceHasItsOwnExitCode is this issue's witness: an
// instance charged no violation whose assessment DECLINED a check it reached
// is not the clean pass exit 0 names, and the CLI must say so on the same
// stream and in the same shape a violation gets. asserted.xml charges nothing
// and leaves the <xs:assert> of its governing type unevaluated, which
// validate/doc.go's Contract calls undecided rather than a pass.
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
	note := "goxsd8: validate: " + abs + ":10:3: this schemaLocation resolved to no document; the rejected assembly is short of whatever that document declares\n"

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
	want := "goxsd8: validate: " + base + ":3:3: this schemaLocation resolved to no document, which is legal and skipped; the compiled schema is short of whatever that document declares\n"

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
// a doc.go stub, fails cleanly instead of passing silently.
func TestValidateFormatVocabulary(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"unrecognized token", []string{"-format", "yaml"}, `-format "yaml" is not a source format; the values are xml, json, ber`},
		{"wrong case", []string{"-format", "XML"}, `-format "XML" is not a source format`},
		{"forced ber is reserved", []string{"-format", "ber"}, "-format ber is reserved by the contract and not yet implemented"},
		{"extension is the default", []string{}, "the extension \".txt\" names no source format; pass -format xml, json, ber"},
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
		})
	}
}

// TestValidateJSONInstance pins a .json instance's route through
// validate/jsonsrc: its verdicts print the "<loc>: [<rule>] <message>" line an
// XML instance's do, at jsonsrc's line and byte column, and earn the same
// codes, while jsonsrc's one error channel — malformed JSON and every decline
// alike — prints "<loc>: [jsonsrc] <message>", the source name standing where
// a rule ID stands, and exits 4: it is no validity verdict. Every row runs
// with and without -no-hints and must answer the same, because a JSON instance
// carries no hints.
func TestValidateJSONInstance(t *testing.T) {
	cases := []struct {
		name      string
		schema    string
		instances []string
		code      int
		// lines are the report's whole lines, in order; a line ending in
		// "] " is a prefix, the message after it being the engine's.
		lines []string
	}{
		{"valid", orderSchema, []string{validJSON}, exitOK, nil},
		{"invalid", orderSchema, []string{invalidJSON}, exitInvalid, []string{
			invalidJSON + ":1:19: [cvc-attribute] ",
			invalidJSON + ":1:11: [cvc-identity-constraint] ",
		}},
		{"unevaluated", assertedSchema, []string{assertedJSON}, exitUndecided, []string{
			assertedJSON + ":1:2: [cvc-assertion] ",
		}},
		{"malformed", orderSchema, []string{malformedJSON}, exitUndecided, []string{
			malformedJSON + ":1:18: [jsonsrc] malformed JSON: unexpected EOF",
		}},
		{"declined", orderSchema, []string{declinedJSON}, exitUndecided, []string{
			declinedJSON + `:1:2: [jsonsrc] member "z:nosuch": no "$xmlns" in scope binds its prefix "z"; a prefixed key names the expanded name its prefix's binding gives it, so bind "z" in a "$xmlns" member, {"z":"<namespace name>"}, leading the object of the element the key names or is an attribute of, or of an ancestor`,
		}},
		// A decline is less severe than a violation, as an undecided check is.
		{"declined beside invalid", orderSchema, []string{declinedJSON, invalidJSON}, exitInvalid, []string{
			declinedJSON + `:1:2: [jsonsrc] member "z:nosuch": no "$xmlns" in scope binds its prefix "z"; a prefixed key names the expanded name its prefix's binding gives it, so bind "z" in a "$xmlns" member, {"z":"<namespace name>"}, leading the object of the element the key names or is an attribute of, or of an ancestor`,
			invalidJSON + ":1:19: [cvc-attribute] ",
			invalidJSON + ":1:11: [cvc-identity-constraint] ",
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
					if got[i] == want || (strings.HasSuffix(want, "] ") && strings.HasPrefix(got[i], want)) {
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

// TestValidateJSONReadFaultIsUndecided pins what the contract promises for an
// I/O fault reading a JSON instance after it is opened: jsonsrc reports it as
// a fault reading the input on its one error channel, so it exits 4, where
// the same fault under an XML instance is an [xml-wf] verdict exiting 1.
// /proc/self/mem opens, and its first read faults (EIO at offset 0).
func TestValidateJSONReadFaultIsUndecided(t *testing.T) {
	const faulting = "/proc/self/mem"
	if _, err := os.ReadFile(faulting); err == nil || !strings.Contains(err.Error(), "input/output error") {
		t.Skipf("reading %s gives %v here, not the EIO this test needs", faulting, err)
	}
	for _, c := range []struct {
		format string
		code   int
		prefix string
	}{
		{"json", exitUndecided, faulting + ":1:1: [jsonsrc] fault reading the input: read " + faulting + ": input/output error"},
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
	if code := run(args, &stdout, &stderr); code != exitUndecided {
		t.Errorf("run(%q) = %d, want %d (stdout %q, stderr %q)", args, code, exitUndecided, stdout.String(), stderr.String())
	}
	want := instance + ":1:18: [jsonsrc] malformed JSON: unexpected EOF\n"
	if stdout.String() != want {
		t.Errorf("run(%q) stdout = %q, want %q", args, stdout.String(), want)
	}
	if n := mustCiteOpenableLocations(t, "stdout", stdout.String()); n != 1 {
		t.Errorf("stdout cites %d locations, want 1:\n%s", n, stdout.String())
	}
}

// TestJSONErrorLine pins jsonErrorLine on the spellings no instance the CLI
// reads produces: a location with no line, and an error not located at the
// URI at all, which is located at the URI with its own text.
func TestJSONErrorLine(t *testing.T) {
	for _, c := range []struct{ uri, err, want string }{
		{"a.json", "jsonsrc: a.json: no line", "a.json: [jsonsrc] no line"},
		{"a.json", "jsonsrc: a.json:3: line only", "a.json:3: [jsonsrc] line only"},
		{"a.json", "jsonsrc: Validate: nil io.Reader", "a.json: [jsonsrc] Validate: nil io.Reader"},
		{"a", "jsonsrc: a.json:1:2: elsewhere", "a: [jsonsrc] a.json:1:2: elsewhere"},
	} {
		if got := jsonErrorLine(c.uri, errors.New(c.err)); got != c.want {
			t.Errorf("jsonErrorLine(%q, %q) = %q, want %q", c.uri, c.err, got, c.want)
		}
	}
}

// TestValidateJSONScalarWhereObjectExpected pins the clause jsonsrc appends to
// the cvc-complex-type charge at a scalar member whose element's type has
// element-only content: README's order.xsd, readme-order.xsd here, on
// readme-text.json, {"order":"just text"}, which exits 1 on one line opening
// at the scalar's location and naming "order" in the clause.
func TestValidateJSONScalarWhereObjectExpected(t *testing.T) {
	const instance = "testdata/readme-text.json"
	args := []string{"validate", "-schema", "testdata/readme-order.xsd", instance}
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
// readme-order.xsd here: an unknown key, which lists the keys the type
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
			args := []string{"validate", "-schema", "testdata/readme-order.xsd", instance}
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
// them collapse into one line.
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
	}{
		{"no schema", []string{"validate", validInstance}, "goxsd8: validate: no schema given"},
		// The schema named after the instance (#1290). The seen map below is
		// what this row is for: this answer was byte-identical to the row above
		// it, so a user who named a schema was told they had not.
		{"schema after the instance", []string{"validate", validInstance, "-schema", orderSchema}, fmt.Sprintf(flagAfterPositionalFmt, "validate", "-schema")},
		{"no instance", []string{"validate", "-schema", orderSchema}, "goxsd8: validate: no instance given"},
		{"dangling -schema", []string{"validate", "-schema"}, "flag needs an argument: -schema"},
		{"undefined flag", []string{"validate", "-out", dir, "-schema", orderSchema, validInstance}, "flag provided but not defined: -out"},
		{"missing schema", []string{"validate", "-schema", "testdata/nosuch.xsd", validInstance}, "no such file or directory"},
		{"schema is a directory", []string{"validate", "-schema", dir, validInstance}, "is a directory"},
		{"schema from stdin", []string{"validate", "-schema", "-", validInstance}, "standard input is not a schema location"},
		{"missing instance", []string{"validate", "-schema", orderSchema, "testdata/nosuch.xml"}, "no such file or directory"},
		// - names no source format, and xml and json are both assessed, so
		// standard input needs -format (#2403).
		{"stdin without -format", []string{"validate", "-schema", orderSchema, "-"}, "goxsd8: validate: -: standard input has no extension to name a source format; pass -format xml, json, ber"},
		// A directory instance is charged as a directory before formatOf reads
		// its extension, whatever its name and whatever -format forced (#2403):
		// "." and the extension-less directory answered with the extension
		// message without the check, and the .xml-named and forced ones with an
		// exit-1 [xml-wf] read failure.
		{"instance is .", []string{"validate", "-schema", orderSchema, "."}, "goxsd8: validate: open .: is a directory"},
		{"instance is a directory", []string{"validate", "-schema", orderSchema, plainDir}, "goxsd8: validate: open " + plainDir + ": is a directory"},
		{"instance is a directory named .xml", []string{"validate", "-schema", orderSchema, xmlDir}, "goxsd8: validate: open " + xmlDir + ": is a directory"},
		{"instance is a directory under -format", []string{"validate", "-format", "xml", "-schema", orderSchema, forcedDir}, "goxsd8: validate: open " + forcedDir + ": is a directory"},
		{"instance is a link to a directory", []string{"validate", "-schema", orderSchema, linkDir}, "goxsd8: validate: open " + linkDir + ": is a directory"},
		// -help=true is not one of the three help spellings, at any position
		// (doc.go's argument vocabulary); after a subcommand it is a flag whose
		// value that subcommand does not accept.
		{"help as a flag value", []string{"validate", "-help=true"}, fmt.Sprintf(helpNotAFlagValueFmt, "validate")},
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
		if !strings.HasSuffix(stderr.String(), helpPointer+"\n") {
			t.Errorf("%s: stderr = %q, want it to end with the remedy line", c.name, stderr.String())
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
