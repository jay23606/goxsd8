package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// helpCases reach the help path: usage on stdout, nothing on stderr, exit 0.
// Everything after the four bare spellings pins a decision doc.go's argument
// vocabulary states rather than an accident — a help flag before its
// subcommand, a help flag alongside an unrecognized one, -- with no
// end-of-options meaning, a help flag after a name outside the vocabulary
// altogether, and `parse -h` / `validate -h`: those two own the flag.FlagSets
// in the binary, so they are the rows where a subcommand's own flag parsing
// could take -h away from the help path.
var helpCases = [][]string{
	nil,
	{"-h"},
	{"-help"},
	{"--help"},
	{"validate", "-help"},
	{"-help", "validate"},
	{"-xyz", "-help"},
	{"--", "-help"},
	{"frobnicate", "-h"},
	{"parse", "-h"},
	{"validate", "-h"},
	{"validate", "-schema", "testdata/order.xsd", "-h"},
}

// dispatchCases is the diagnosis every non-help invocation that reaches no
// built subcommand earns: a reserved name, a name outside the vocabulary, a
// help request carrying a value, a flag before the subcommand it qualifies, or
// no subcommand at all. Each is followed on stderr by helpPointer. One
// encoding of the matrix, driven twice — through run below and through the
// built binary in TestBuiltBinaryMatrix.
//
// parse and validate have no row here: both are built, and parse_test.go and
// validate_test.go are their matrices.
var dispatchCases = []struct {
	args []string
	want string
}{
	{[]string{"gen", "-schema", "a.xsd", "-out", "d"}, `goxsd8: gen is not yet implemented`},
	{[]string{"frobnicate"}, `goxsd8: unknown subcommand "frobnicate"`},
	{[]string{"parsee", "a.xsd"}, `goxsd8: unknown subcommand "parsee"`},
	// Case-sensitive matching: a contract name in the wrong case is unknown,
	// not reserved.
	{[]string{"VALIDATE"}, `goxsd8: unknown subcommand "VALIDATE"`},
	// help is not a contract name (doc.go); version is not one either.
	{[]string{"help"}, `goxsd8: unknown subcommand "help"`},
	{[]string{"version"}, `goxsd8: unknown subcommand "version"`},
	// A flag as args[0] with no subcommand anywhere after it is no subcommand
	// at all — not an unknown one. -q is documented and -version is not, and
	// neither is a subcommand name.
	{[]string{"-q"}, noSubcommand},
	{[]string{"-xyz"}, noSubcommand},
	{[]string{"-version"}, noSubcommand},
	{[]string{"-q", "frobnicate"}, noSubcommand},
	// A valued help spelling is the fifth answer (#1189): it is a usage error
	// in every position, and before a subcommand it names the three spellings
	// rather than a rewrite that fails there too. The first row expected
	// noSubcommand until this landed.
	{[]string{"-help=true"}, helpNotAFlagValue},
	{[]string{"-h=1"}, helpNotAFlagValue},
	{[]string{"-help=true", "parse", "a.xsd"}, helpNotAFlagValue},
	{[]string{"-h=1", "parse", "a.xsd"}, helpNotAFlagValue},
	// A flag BEFORE a real subcommand is the one #472 settled: the common
	// flags follow the name they qualify, and the diagnosis says so instead of
	// claiming a subcommand that is right there was never given.
	{[]string{"-q", "parse", "a.xsd"}, `goxsd8: -q must follow the subcommand: goxsd8 parse -q ...`},
	{[]string{"-v", "gen", "-schema", "a.xsd"}, `goxsd8: -v must follow the subcommand: goxsd8 gen -v ...`},
}

// TestRunHelp pins issue #251: a help request — including the bare
// invocation — never reaches a usage error. Usage goes to stdout, stderr
// stays empty, exit code 0.
func TestRunHelp(t *testing.T) {
	for _, args := range helpCases {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr)
		if code != 0 {
			t.Errorf("run(%q) = %d, want 0", args, code)
		}
		if stdout.String() != usage {
			t.Errorf("run(%q) stdout = %q, want the usage contract", args, stdout.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("run(%q) stderr = %q, want empty", args, stderr.String())
		}
	}
}

// TestRunDispatch pins #514, #472 and #1189: the five diagnoses are distinct,
// and each is the true one for its input. It supersedes TestDiagnosesAreDistinct
// (removed, #999): that test asserted the same distinctness by comparing
// diagnose's rendered strings, which cannot fail on a collapse between two
// branches whose diagnosis interpolates the input argument — notImplementedFmt,
// unknownSubcommandFmt and leadingFlagFmt all do, so two inputs those branches
// misclassify as the same kind still render different strings. It stays armed
// against a collapse into noSubcommand, a bare constant with nothing to
// interpolate — which is exactly the collapse a pre-#472 diagnose regresses to,
// and dispatchCases below still catches it.
func TestRunDispatch(t *testing.T) {
	for _, c := range dispatchCases {
		var stdout, stderr bytes.Buffer
		code := run(c.args, &stdout, &stderr)
		if code != 2 {
			t.Errorf("run(%q) = %d, want 2", c.args, code)
		}
		if stdout.Len() != 0 {
			t.Errorf("run(%q) stdout = %q, want empty", c.args, stdout.String())
		}
		want := c.want + "\n" + helpPointer + "\n"
		if stderr.String() != want {
			t.Errorf("run(%q) stderr = %q, want %q", c.args, stderr.String(), want)
		}
	}
}

// TestHelpPointerResolvesOutsideTheModule pins #870's second half: the remedy
// a usage error names must work where an installed binary runs. A `go doc`
// invocation does not — it needs the module tree — so no message may name one.
func TestHelpPointerResolvesOutsideTheModule(t *testing.T) {
	if strings.Contains(helpPointer, "go doc") {
		t.Errorf("helpPointer = %q names go doc, which fails outside the module tree", helpPointer)
	}
	if !strings.Contains(helpPointer, "goxsd8 -help") {
		t.Errorf("helpPointer = %q does not name the binary's own help path", helpPointer)
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("closed pipe") }

// TestRunHelpWriteFailure covers the usage/IO exit code: help the user never
// received is not a success.
func TestRunHelpWriteFailure(t *testing.T) {
	var stderr bytes.Buffer
	if code := run([]string{"-help"}, errWriter{}, &stderr); code != 2 {
		t.Errorf("run with a failing stdout = %d, want 2", code)
	}
}

// TestExitSeverityRanksEveryCode holds exitSeverity to the const block it
// orders. A code missing from it ranks below every code that is in it, so an
// invocation would answer with the outcome that code should have beaten —
// silently, since nothing about a wrong exit code reaches a stream.
func TestExitSeverityRanksEveryCode(t *testing.T) {
	codes := []int{exitOK, exitInvalid, exitUsage, exitSchema, exitUndecided}
	for _, c := range codes {
		if !slices.Contains(exitSeverity, c) {
			t.Errorf("exit code %d is ranked by nothing in exitSeverity", c)
		}
	}
	if len(exitSeverity) != len(codes) {
		t.Errorf("exitSeverity ranks %d codes, and the const block defines %d", len(exitSeverity), len(codes))
	}

	// The ordering the numbers do not carry, in both argument orders: a batch
	// answers with the instance it decided against, not the one it declined
	// to decide.
	if worse(exitInvalid, exitUndecided) != exitInvalid || worse(exitUndecided, exitInvalid) != exitInvalid {
		t.Errorf("worse over %d and %d = %d and %d, want %d both ways", exitInvalid, exitUndecided, worse(exitInvalid, exitUndecided), worse(exitUndecided, exitInvalid), exitInvalid)
	}
	if worse(exitOK, exitUndecided) != exitUndecided {
		t.Errorf("worse(%d, %d) = %d, want %d: undecided is not a clean run", exitOK, exitUndecided, worse(exitOK, exitUndecided), exitUndecided)
	}
	if worse(exitSchema, exitUsage) != exitSchema {
		t.Errorf("worse(%d, %d) = %d, want %d: a schema set that does not compile leaves every instance unassessed", exitSchema, exitUsage, worse(exitSchema, exitUsage), exitSchema)
	}
}

// update makes TestDocRendersUsage rewrite doc.go's # Usage section instead of
// failing on it; main.go's go:generate directive is what passes it.
var update = flag.Bool("update", false, "rewrite doc.go's # Usage section from usage")

// The generated section of doc.go's package comment runs from the # Usage
// heading up to the hand-written heading that follows it.
const (
	docUsageStart = "// # Usage"
	docUsageEnd   = "// # Argument vocabulary"
)

// docUsageBlock renders usage as doc.go's # Usage section, which is the only
// place the mapping between the two lives. The title paragraph is dropped —
// doc.go's opening sentence states it — and the heading line becomes a doc
// comment heading. Every line indented two spaces is a subcommand block and
// renders preformatted, so go doc keeps its layout; every other line is prose
// and renders as it is wrapped in usage, which is why a ragged prose paragraph
// in usage surfaces as a go tool commentwrap finding on doc.go and is fixed by
// reflowing usage, never doc.go.
func docUsageBlock(u string) (string, error) {
	_, rest, ok := strings.Cut(u, "\n\n")
	if !ok {
		return "", errors.New("usage has no title paragraph to drop")
	}
	heading, body, ok := strings.Cut(rest, "\n\n")
	if !ok || strings.Contains(heading, "\n") || !strings.HasSuffix(heading, ":") {
		return "", fmt.Errorf("usage's second paragraph %q is not a one-line heading ending in a colon", heading)
	}
	var b strings.Builder
	b.WriteString("// # " + strings.TrimSuffix(heading, ":") + "\n//\n")
	for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		switch {
		case line == "":
			b.WriteString("//\n")
		case strings.HasPrefix(line, "  "):
			b.WriteString("//\t" + line[2:] + "\n")
		case strings.HasPrefix(line, " "), strings.HasPrefix(line, "\t"):
			return "", fmt.Errorf("usage line %q is neither prose nor indented two spaces as a subcommand block", line)
		default:
			b.WriteString("// " + line + "\n")
		}
	}
	b.WriteString("//\n")
	return b.String(), nil
}

// TestDocRendersUsage holds doc.go's # Usage section to the rendering of usage,
// so the contract is edited in one place (#1772). It compares the section as
// read from doc.go against docUsageBlock(usage), so an edit to either side
// alone fails it. With -update (go generate) it rewrites the section instead.
func TestDocRendersUsage(t *testing.T) {
	want, err := docUsageBlock(usage)
	if err != nil {
		t.Fatalf("rendering usage: %v", err)
	}
	src, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatalf("reading doc.go: %v", err)
	}
	doc := string(src)
	start := strings.Index(doc, "\n"+docUsageStart) + 1
	end := strings.Index(doc, "\n"+docUsageEnd) + 1
	if start == 0 || end <= start {
		t.Fatalf("doc.go has no %q line followed by a %q line to bound the generated section", docUsageStart, docUsageEnd)
	}
	got := doc[start:end]
	if got == want {
		return
	}
	if *update {
		if err := os.WriteFile("doc.go", []byte(doc[:start]+want+doc[end:]), 0o644); err != nil {
			t.Fatalf("writing doc.go: %v", err)
		}
		return
	}
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	i := 0
	for i < len(gotLines) && i < len(wantLines) && gotLines[i] == wantLines[i] {
		i++
	}
	t.Errorf("doc.go's # Usage section is not the rendering of usage: edit usage in main.go and run go generate ./cmd/goxsd8, never doc.go's copy\nfirst difference, section line %d:\n doc.go: %q\n  usage: %q",
		i+1, lineAt(gotLines, i), lineAt(wantLines, i))
}

// lineAt returns lines[i], or a marker when the section ran out first.
func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "(section ended)"
}

// TestUsageCoversContract pins usage against the code it describes, which
// TestDocRendersUsage cannot: that test holds doc.go to usage, and nothing but
// these checks holds usage to dispatch.
func TestUsageCoversContract(t *testing.T) {
	// This pins the vocabulary dispatch reads against the text a user is
	// shown, so the two cannot become separate lists (STYLE D3/T4).
	for _, name := range subcommands {
		if !strings.Contains(usage, "goxsd8 "+name+" ") {
			t.Errorf("usage documents no %q subcommand, but dispatch reserves it", name)
		}
	}
	// The status paragraph states gen's status in the wording the binary
	// prints, not in a second one of its own: both prose copies said "not yet
	// built" where notImplementedFmt printed "not yet implemented", and nothing
	// held them together (#1231). Derived from the constant rather than quoted,
	// so a change to either side fails here.
	verb := strings.TrimPrefix(fmt.Sprintf(notImplementedFmt, "gen"), "goxsd8: gen is ")
	if !strings.Contains(usage, "gen is reserved but "+verb) {
		t.Errorf("usage does not state gen's status in the wording notImplementedFmt prints (%q)", verb)
	}
}

// TestBuiltBinaryMatrix drives the real executable, following #251: exit
// code, stdout and stderr of the shipped binary, not of a seam over run.
func TestBuiltBinaryMatrix(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "goxsd8")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build -o %s .: %v: %s", bin, err, out)
	}

	for _, args := range helpCases {
		stdout, stderr, code := runBinary(t, bin, args)
		if code != 0 {
			t.Errorf("%s %q = %d, want 0 (stderr %q)", bin, args, code, stderr)
		}
		if stdout != usage {
			t.Errorf("%s %q stdout = %q, want the usage contract", bin, args, stdout)
		}
		if stderr != "" {
			t.Errorf("%s %q stderr = %q, want empty", bin, args, stderr)
		}
	}

	for _, c := range dispatchCases {
		stdout, stderr, code := runBinary(t, bin, c.args)
		if code != 2 {
			t.Errorf("%s %q = %d, want 2", bin, c.args, code)
		}
		if stdout != "" {
			t.Errorf("%s %q stdout = %q, want empty", bin, c.args, stdout)
		}
		want := c.want + "\n" + helpPointer + "\n"
		if stderr != want {
			t.Errorf("%s %q stderr = %q, want %q", bin, c.args, stderr, want)
		}
	}

	// parse's and validate's own outcomes, through the shipped executable
	// rather than through run: the exit code, the stream each answer lands on,
	// and the summary's bytes are what a script sees (#251's shape, #472's
	// subject). The six stdin rows are the only place standard input is
	// reachable at all — run takes its writers as arguments and its reader from
	// the process — and the two -format xml ones pin the hint scan's replay as
	// well as the spelling: the scan consumes the document's prefix, so a
	// broken replay would leave the assessment nothing to read.
	for _, c := range []struct {
		args        []string
		stdin       string
		code        int
		stdout      string
		stdoutMatch string
		stderrMatch string
	}{
		{args: []string{"parse", "testdata/order.xsd"}, code: 0, stdout: orderSummary},
		{args: []string{"parse", "-q", "testdata/order.xsd"}, code: 0},
		{args: []string{"parse", "testdata/broken.xsd"}, code: 1, stderrMatch: "[src-resolve]"},
		{args: []string{"parse", "testdata/nosuch.xsd"}, code: 2, stderrMatch: "no such file or directory"},
		{args: []string{"parse"}, code: 2, stderrMatch: "goxsd8: parse: no schema given"},
		// The misplaced flag, through the shipped binary: the summary -q asked
		// to suppress must not reach stdout ahead of the diagnosis (#1290).
		{args: []string{"parse", "testdata/order.xsd", "-q"}, code: 2,
			stderrMatch: "-q stands after a positional argument"},
		{args: []string{"validate", "-schema", orderSchema, validInstance}, code: 0},
		{args: []string{"validate", "-schema", orderSchema, invalidInstance}, code: 1,
			stdoutMatch: invalidInstance + ":5:3: [cvc-attribute]"},
		{args: []string{"validate", "-schema", "testdata/broken.xsd", validInstance}, code: 3,
			stderrMatch: "[src-resolve]"},
		{args: []string{"validate", "-schema", orderSchema, "testdata/nosuch.xml"}, code: 2,
			stderrMatch: "no such file or directory"},
		{args: []string{"validate"}, code: 2, stderrMatch: "goxsd8: validate: no schema given"},
		{args: []string{"validate", "-format", "xml", "-schema", orderSchema, "-"},
			stdin: readFixture(t, validInstance), code: 0},
		{args: []string{"validate", "-format", "xml", "-schema", orderSchema, "-"},
			stdin: readFixture(t, invalidInstance), code: 1, stdoutMatch: "-:5:3: [cvc-attribute]"},
		// - names no source format once xml and json are both assessed, so
		// without -format it exits 2 asking for one, whatever it holds (#2403).
		{args: []string{"validate", "-schema", orderSchema, "-"},
			stdin: readFixture(t, validInstance), code: 2, stderrMatch: "-: standard input has no extension to name a source format"},
		{args: []string{"validate", "-schema", orderSchema, "-"},
			stdin: readFixture(t, invalidJSON), code: 2, stderrMatch: "-: standard input has no extension to name a source format"},
		{args: []string{"validate", "-format", "json", "-schema", orderSchema, "-"},
			stdin: readFixture(t, validJSON), code: 0},
		{args: []string{"validate", "-format", "json", "-schema", orderSchema, "-"},
			stdin: readFixture(t, invalidJSON), code: 1, stdoutMatch: "-:1:19: [cvc-attribute]"},
	} {
		stdout, stderr, code := runBinaryStdin(t, bin, c.args, c.stdin)
		if code != c.code {
			t.Errorf("%s %q = %d, want %d (stderr %q)", bin, c.args, code, c.code, stderr)
		}
		if c.stdoutMatch == "" && stdout != c.stdout {
			t.Errorf("%s %q stdout = %q, want %q", bin, c.args, stdout, c.stdout)
		}
		if c.stdoutMatch != "" && !strings.Contains(stdout, c.stdoutMatch) {
			t.Errorf("%s %q stdout = %q, want it to contain %q", bin, c.args, stdout, c.stdoutMatch)
		}
		if c.stderrMatch == "" && stderr != "" {
			t.Errorf("%s %q stderr = %q, want empty", bin, c.args, stderr)
		}
		if c.stderrMatch != "" && !strings.Contains(stderr, c.stderrMatch) {
			t.Errorf("%s %q stderr = %q, want it to contain %q", bin, c.args, stderr, c.stderrMatch)
		}
	}
}

// readFixture returns the bytes of a testdata file, for the rows that feed one
// to the binary's standard input instead of naming it.
func readFixture(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// runBinary runs bin with args and an empty standard input.
func runBinary(t *testing.T, bin string, args []string) (string, string, int) {
	t.Helper()
	return runBinaryStdin(t, bin, args, "")
}

// runBinaryStdin runs bin with args and stdin, and returns its stdout, stderr
// and exit code. A non-zero exit is an expected outcome here, not a test
// failure — only a failure to run the binary at all is.
func runBinaryStdin(t *testing.T, bin string, args []string, stdin string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	if err != nil && !errors.As(err, &ee) {
		t.Fatalf("%s %q: %v", bin, args, err)
	}
	return stdout.String(), stderr.String(), cmd.ProcessState.ExitCode()
}
