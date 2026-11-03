package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/doc/comment"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// helpCases reach the help path: usage on stdout, nothing on stderr, exit 0.
// Everything after the four bare spellings pins a decision usage or doc.go's
// argument vocabulary states rather than an accident — a help flag before its
// subcommand, a help flag alongside an unrecognized one, -- given no
// end-of-options meaning by the help scan, even where it ends a subcommand's
// options, a help flag after a name outside the vocabulary
// altogether, and `parse -h`, `validate -h` and `gen -h`: those three own the
// flag.FlagSets in the binary, so they are the rows where a subcommand's own
// flag parsing could take -h away from the help path.
var helpCases = [][]string{
	nil,
	{"-h"},
	{"-help"},
	{"--help"},
	{"validate", "-help"},
	{"-help", "validate"},
	{"-xyz", "-help"},
	{"--", "-help"},
	{"validate", "-schema", "s.xsd", "--", "-help"},
	{"frobnicate", "-h"},
	{"parse", "-h"},
	{"validate", "-h"},
	{"validate", "-schema", "testdata/order.xsd", "-h"},
	{"gen", "-h"},
	{"gen", "-schema", "testdata/order.xsd", "-out", "d", "-h"},
}

// dispatchCases is the diagnosis every non-help invocation that reaches no
// subcommand earns: a name outside the vocabulary, a help request carrying a
// value, a flag before the subcommand it qualifies, or no subcommand at all.
// Each is followed on stderr by helpPointer. One encoding of the matrix, driven
// twice — through run below and through the built binary in
// TestBuiltBinaryMatrix.
//
// parse, validate and gen have no row here: all three are built, and
// parse_test.go, validate_test.go and gen_test.go are their matrices.
var dispatchCases = []struct {
	args []string
	want string
}{
	{[]string{"frobnicate"}, `goxsd8: unknown subcommand "frobnicate"`},
	{[]string{"parsee", "a.xsd"}, `goxsd8: unknown subcommand "parsee"`},
	// Case-sensitive matching: a contract name in the wrong case is unknown,
	// not that subcommand.
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
	// A valued help spelling is an answer of its own (#1189): it is a usage error
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

// TestRunDispatch pins #472 and #1189: the four diagnoses are distinct, and
// each is the true one for its input. It supersedes TestDiagnosesAreDistinct
// (removed, #999): that test asserted the same distinctness by comparing
// diagnose's rendered strings, which cannot fail on a collapse between two
// branches whose diagnosis interpolates the input argument —
// unknownSubcommandFmt and leadingFlagFmt both do, so two inputs those branches
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

// The usage: lines a subcommand's exit-2 diagnosis prints, held as literals so
// that a test comparing stderr against them fails when usage loses or changes
// the synopsis line: an expectation read through synopsis would follow usage
// wherever it went (#3110).
const (
	parseUsageLine    = "usage: goxsd8 parse [-q] [-v] <schema.xsd>..."
	validateUsageLine = "usage: goxsd8 validate [-q] [-v] [-format xml|json|ber] [-no-hints] -schema <schema.xsd> [-schema <s2>]... <instance>..."
	genUsageLine      = "usage: goxsd8 gen [-q] [-v] -schema <schema.xsd> -out <dir> [-schema <s2> -out <d2>]..."
)

// TestSubcommandDiagnosisPrintsSynopsis pins #3110: a diagnosis charged inside
// a subcommand prints three lines on stderr, the diagnosis, usage: and that
// subcommand's synopsis line, then helpPointer. dispatchCases pins the
// diagnoses charged before a subcommand is known, which print no synopsis.
func TestSubcommandDiagnosisPrintsSynopsis(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"validate", "x.xml"}, "goxsd8: validate: no schema given\n" + validateUsageLine + "\n" + helpPointer + "\n"},
		{[]string{"gen", "-schema"}, "goxsd8: gen: flag needs an argument: -schema\n" + genUsageLine + "\n" + helpPointer + "\n"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(c.args, &stdout, &stderr); code != exitUsage {
			t.Errorf("run(%q) = %d, want %d", c.args, code, exitUsage)
		}
		if stdout.Len() != 0 {
			t.Errorf("run(%q) stdout = %q, want empty", c.args, stdout.String())
		}
		if stderr.String() != c.want {
			t.Errorf("run(%q) stderr = %q, want %q", c.args, stderr.String(), c.want)
		}
	}
}

// TestStdoutFaultPrintsSynopsis pins the I/O-fault arm of #3110: a failed
// write of parse's summary or of validate's report is charged exit 2 inside
// the subcommand, so its line carries the synopsis too.
func TestStdoutFaultPrintsSynopsis(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"parse", "testdata/order.xsd"}, "goxsd8: parse: writing the summary for testdata/order.xsd: closed pipe\n" + parseUsageLine + "\n" + helpPointer + "\n"},
		{[]string{"validate", "-schema", "testdata/order.xsd", "testdata/order-invalid.xml"}, "goxsd8: validate: writing the report of testdata/order-invalid.xml: closed pipe\n" + validateUsageLine + "\n" + helpPointer + "\n"},
	} {
		var stderr bytes.Buffer
		if code := run(c.args, errWriter{}, &stderr); code != exitUsage {
			t.Errorf("run(%q) = %d, want %d", c.args, code, exitUsage)
		}
		if stderr.String() != c.want {
			t.Errorf("run(%q) stderr = %q, want %q", c.args, stderr.String(), c.want)
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
		t.Errorf("worse(%d, %d) = %d, want %d: exitSeverity ranks exitSchema last only so that every code has a rank", exitSchema, exitUsage, worse(exitSchema, exitUsage), exitSchema)
	}
}

// usageRankRow is a row of usage's exit-code table: its code and its rank,
// a number or none.
var usageRankRow = regexp.MustCompile(`(?m)^  (\d)     (\d|none)  `)

// usagePrecedence is the one-line reading of the rank column under the table:
// the ranked codes, highest first, then the unranked one.
var usagePrecedence = regexp.MustCompile(`Precedence, by that rank: (\d) over (\d) over (\d) over (\d), with validate's (\d) answering alone`)

// TestUsagePrecedenceReadsTheRanks holds usage's precedence line to both the
// rank column it reads and exitSeverity, which worse orders by (#2967): each
// code the line puts over the next one must win over it through worse, in both
// argument orders, and must hold the next rank up in the table. The unranked
// code is exitSchema, which the line ranks against nothing.
func TestUsagePrecedenceReadsTheRanks(t *testing.T) {
	ranks := map[int]string{}
	for _, m := range usageRankRow.FindAllStringSubmatch(usage, -1) {
		code, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("rank row %q: %v", m[0], err)
		}
		ranks[code] = m[2]
	}
	if len(ranks) != len(exitSeverity) {
		t.Fatalf("usage's exit-code table has %d rows, and exitSeverity ranks %d codes", len(ranks), len(exitSeverity))
	}
	for i, code := range exitSeverity {
		want := strconv.Itoa(i + 1)
		if code == exitSchema {
			want = "none"
		}
		if ranks[code] != want {
			t.Errorf("usage's exit-code table ranks %d as %s, and exitSeverity as %s", code, ranks[code], want)
		}
	}

	m := usagePrecedence.FindStringSubmatch(strings.Join(strings.Fields(usage), " "))
	if m == nil {
		t.Fatalf("usage has no line matching %q", usagePrecedence)
	}
	var line []int
	for _, s := range m[1:] {
		code, err := strconv.Atoi(s)
		if err != nil {
			t.Fatalf("precedence code %q: %v", s, err)
		}
		line = append(line, code)
	}
	ranked, alone := line[:len(line)-1], line[len(line)-1]
	if alone != exitSchema {
		t.Errorf("usage's precedence line leaves %d unranked, and the table leaves %d", alone, exitSchema)
	}
	for i := 0; i+1 < len(ranked); i++ {
		hi, lo := ranked[i], ranked[i+1]
		if worse(hi, lo) != hi || worse(lo, hi) != hi {
			t.Errorf("usage's precedence line puts %d over %d, and worse answers %d and %d", hi, lo, worse(hi, lo), worse(lo, hi))
		}
		hiRank, errHi := strconv.Atoi(ranks[hi])
		loRank, errLo := strconv.Atoi(ranks[lo])
		if errHi != nil || errLo != nil || hiRank != loRank+1 {
			t.Errorf("usage's precedence line puts %d over %d, and the table ranks them %s and %s", hi, lo, ranks[hi], ranks[lo])
		}
	}
	if len(ranked) != len(exitSeverity)-1 {
		t.Errorf("usage's precedence line ranks %d codes, and exitSeverity ranks %d besides exitSchema", len(ranked), len(exitSeverity)-1)
	}
}

// usageKindsLead opens usage's list of the kinds of declined check, whose
// items follow it one per "- <kind>:" line.
const usageKindsLead = "A declined check is one of these kinds:\n"

// usageKindItem is one item of that list.
var usageKindItem = regexp.MustCompile(`^        - ([^:]+):`)

// TestUsageListsEveryDeclinedKind holds usage's list of the kinds of declined
// check to the bullet labels of validate.Unevaluated's doc, read from its
// source, which stays their owner (#2967): a kind missing from usage, one usage
// adds, or one out of the doc's order fails it. Labels compare with the doc's
// · marks dropped.
func TestUsageListsEveryDeclinedKind(t *testing.T) {
	want := unevaluatedKinds(t)

	_, rest, ok := strings.Cut(usage, usageKindsLead)
	if !ok {
		t.Fatalf("usage has no %q line", strings.TrimSpace(usageKindsLead))
	}
	var got []string
	for _, line := range strings.Split(rest, "\n") {
		if strings.HasPrefix(line, "          ") {
			continue // an item's continuation line
		}
		m := usageKindItem.FindStringSubmatch(line)
		if m == nil {
			break
		}
		got = append(got, m[1])
	}
	if !slices.Equal(got, want) {
		t.Errorf("usage lists the declined kinds %q, and validate.Unevaluated's doc names %q", got, want)
	}
}

// unevaluatedKinds returns the labels of the first list in the doc comment of
// validate.Unevaluated, parsed from validate/validate.go: each item's text up
// to its first colon, · marks dropped.
func unevaluatedKinds(t *testing.T) []string {
	t.Helper()
	path := filepath.Join("..", "..", "validate", "validate.go")
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	var doc *ast.CommentGroup
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			if spec.(*ast.TypeSpec).Name.Name == "Unevaluated" {
				doc = gen.Doc
			}
		}
	}
	if doc == nil {
		t.Fatalf("%s declares no documented type Unevaluated", path)
	}
	var p comment.Parser
	for _, block := range p.Parse(doc.Text()).Content {
		list, ok := block.(*comment.List)
		if !ok {
			continue
		}
		var kinds []string
		for _, item := range list.Items {
			var text strings.Builder
			for _, b := range item.Content {
				para, ok := b.(*comment.Paragraph)
				if !ok {
					continue
				}
				for _, s := range para.Text {
					if plain, ok := s.(comment.Plain); ok {
						text.WriteString(string(plain))
					}
				}
			}
			label, _, ok := strings.Cut(text.String(), ":")
			if !ok {
				t.Fatalf("validate.Unevaluated's list item %q has no colon ending its kind", text.String())
			}
			kinds = append(kinds, strings.ReplaceAll(label, "·", ""))
		}
		return kinds
	}
	t.Fatalf("validate.Unevaluated's doc has no list of kinds")
	return nil
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
	// shown, so the two cannot become separate lists (STYLE D3/T4). Exactly
	// one synopsis line per name: synopsis panics on any other count, and a
	// prose line naming the subcommand does not count (#3110).
	for _, name := range subcommands {
		if got := synopsisLines(name); len(got) != 1 {
			t.Errorf("usage spells %d synopsis lines for %q (%q), want 1: dispatch reserves it, and subcommandUsageError prints its one line", len(got), name, got)
		}
	}
	// jsonsrcLine's routing of jsonsrc's error channel by class, each class's
	// line shape and exit code, which jsonsrcLine renders (#2721, #2893).
	// Matched over the text with its line breaks folded, as the sentences wrap.
	folded := strings.Join(strings.Fields(usage), " ")
	for _, want := range []string{
		"validate routes each by the class jsonsrc gives it",
		fmt.Sprintf("Malformed JSON and content after the top-level object print <loc>: [%s] <message> and exit 1, as malformed XML does", jsonsrcToken),
		"an I/O fault reading the instance prints the same line and exits 1, as an I/O fault reading an XML instance does",
		fmt.Sprintf("A decline prints <loc>: [%s] undecided: <message> and exits 4", jsonsrcToken),
	} {
		if !strings.Contains(folded, want) {
			t.Errorf("usage does not state how a jsonsrc error routes (%q)", want)
		}
	}
	// gen's sharing between pairs couples a later pair's package to an
	// earlier one's, and nothing carries between invocations (gen.go calls
	// codegen.Generate once per run), so the help names both (#3289).
	for _, want := range []string{
		"The later package imports the earlier by its import path, so deleting or moving the earlier pair's -out breaks the later one's build.",
		"Sharing is among the pairs of one invocation: running gen once per pair declares every type in its own package.",
	} {
		if !strings.Contains(folded, want) {
			t.Errorf("usage does not state gen's sharing coupling or its one-run-per-pair route (%q)", want)
		}
	}
	// The one statement that the undecided: literal marks both withholds,
	// which doc.go's hand-written Diagnostic lines section carries (#2893).
	src, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatalf("reading doc.go: %v", err)
	}
	both := "The literal undecided: marks both withholds on purpose, a declined check and a jsonsrc decline, and the token before it, a rule ID or jsonsrc, tells them apart."
	if !strings.Contains(strings.Join(strings.Fields(strings.ReplaceAll(string(src), "\n//", "\n")), " "), both) {
		t.Errorf("doc.go does not state that undecided: marks both withholds (%q)", both)
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
		// gen's outcomes through the shipped binary; gen_test.go is the matrix.
		{args: []string{"gen", "-schema", flatFixture, "-out", filepath.Join(moduleTempDir(t), "flat")}, code: 0},
		{args: []string{"gen", "-schema", "testdata/broken.xsd", "-out", filepath.Join(moduleTempDir(t), "broken")}, code: 1,
			stderrMatch: "[src-resolve]"},
		{args: []string{"gen", "-schema", flatFixture}, code: 2,
			stderrMatch: "goxsd8: gen: -schema " + flatFixture + " stands without its -out"},
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
