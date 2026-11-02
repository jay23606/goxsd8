package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// fixtureSuite is a miniature suite catalog carrying every shape the join
// turns on: one instance document named by three test groups, one of them
// withheld; a schema document named both by its own schemaTest and by the
// instance case assessed against it — once inside an applicable group and
// once inside a withheld one, so both counts have a duplicate to collapse;
// the declared outcomes the instance lane files its declared-valid row on;
// and, in g5, a schema case and an instance case the suite declares
// indeterminate, which a fed run log lists as indeterminate declines.
var fixtureSuite = map[string]string{
	"suite.xml": `<testSuite xmlns:xlink="http://www.w3.org/1999/xlink">
  <testSetRef xlink:href="sets/s.testSet"/>
</testSuite>`,

	"sets/s.testSet": `<testSet name="S" xmlns:xlink="http://www.w3.org/1999/xlink">
  <testGroup name="g1">
    <schemaTest name="s1">
      <schemaDocument xlink:href="../docs/a.xsd"/>
      <expected validity="valid"/>
    </schemaTest>
    <instanceTest name="i1">
      <instanceDocument xlink:href="../docs/a1.xml"/>
      <expected validity="invalid"/>
    </instanceTest>
  </testGroup>
  <testGroup name="g2">
    <instanceTest name="i2">
      <instanceDocument xlink:href="../docs/a1.xml"/>
      <expected validity="valid"/>
    </instanceTest>
  </testGroup>
  <testGroup name="g3" version="1.0">
    <schemaTest name="s3">
      <schemaDocument xlink:href="../docs/c.xsd"/>
      <expected validity="valid"/>
    </schemaTest>
    <instanceTest name="i3">
      <instanceDocument xlink:href="../docs/a1.xml"/>
      <expected validity="invalid"/>
    </instanceTest>
  </testGroup>
  <testGroup name="g4">
    <schemaTest name="s4">
      <schemaDocument xlink:href="../docs/b.xsd"/>
      <expected validity="valid"/>
    </schemaTest>
    <instanceTest name="i4">
      <instanceDocument xlink:href="../docs/b1.xml"/>
      <expected validity="invalid"/>
    </instanceTest>
  </testGroup>
  <testGroup name="g5">
    <schemaTest name="s5">
      <schemaDocument xlink:href="../docs/d.xsd"/>
      <expected validity="indeterminate"/>
    </schemaTest>
    <instanceTest name="i5">
      <instanceDocument xlink:href="../docs/d1.xml"/>
      <expected validity="indeterminate"/>
    </instanceTest>
  </testGroup>
</testSet>`,
}

// fixtureLanes are the committed expectations the fixture suite is joined
// against: one case per class the report partitions into.
//
// S/g3/instance/i3 is WITHHELD and carries a line anyway — the state a suite
// re-pin leaves behind, where the line is a sanctioned applicability removal
// the next ratchet deletes rather than a score that could flip. The join must
// classify it withheld and not as a banked failure.
var fixtureLanes = map[string]string{
	"instance.txt": `S/g1/instance/i1 fail
S/g2/instance/i2 fail
S/g3/instance/i3 fail
S/g4/instance/i4 pass
S/g5/instance/i5 fail
`,
	"schema.txt": `S/g1/schema/s1 fail
S/g4/schema/s4 fail
S/g5/schema/s5 fail
`,
}

// fixtureLog is a GOXSD_DECLINES=1 run's census over the fixture lanes, inside
// the noise a real -v log carries around it: each lane lists its g5 case as an
// indeterminate decline.
const fixtureLog = `=== RUN   TestConformance
    conformance_test.go:245: lane schema: decline candidates: [S/g1/schema/s1]
    conformance_test.go:246: lane schema: indeterminate declines: [S/g5/schema/s5]
    conformance_test.go:247: lane schema: decided disagreements: [S/g4/schema/s4=(accepted)]
    conformance_test.go:245: lane instance: decline candidates: [S/g1/instance/i1 S/g2/instance/i2]
    conformance_test.go:246: lane instance: indeterminate declines: [S/g5/instance/i5]
    conformance_test.go:247: lane instance: decided disagreements: []
--- PASS: TestConformance (1.00s)
`

// logFile writes body to a temp file and returns its path.
func logFile(t *testing.T, body string) string {
	t.Helper()
	return filepath.Join(writeTree(t, map[string]string{"run.log": body}), "run.log")
}

// writeTree materializes a name-to-body map under a fresh temp directory and
// returns its root.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
	return root
}

// runFixture drives the whole command against the fixture suite and lanes,
// with args behind the two directory flags, and returns what it printed.
func runFixture(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	var out strings.Builder
	full := append([]string{"-suite", writeTree(t, fixtureSuite), "-expectations", writeTree(t, fixtureLanes)}, args...)
	if err := run(&out, strings.NewReader(stdin), full); err != nil {
		t.Fatalf("run %q: %v", args, err)
	}
	return out.String()
}

// wantLines fails unless every wanted substring is present, naming the output
// once so a failure is readable.
func wantLines(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("output does not carry %q:\n%s", w, got)
			return
		}
	}
}

// wantRow fails unless the report carries the named partition row with the
// given count. It compares the label and the number and not the padding
// between them, so a column width is not a test to maintain.
func wantRow(t *testing.T, got, label string, count int) {
	t.Helper()
	for line := range strings.SplitSeq(got, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, label) {
			continue
		}
		fields := strings.Fields(trimmed)
		if last := fields[len(fields)-1]; last != strconv.Itoa(count) {
			t.Errorf("row %q counts %s, want %d", label, last, count)
		}
		return
	}
	t.Errorf("no row %q in:\n%s", label, got)
}

// TestIDsNamesEveryCatalogEntryOfAPathIncludingWithheldOnes is the direction
// the relation is one-to-many in: one document, three test groups, three case
// IDs — and the withheld one is reported and marked rather than dropped,
// because it is a catalog entry that merely carries no line in any lane
// (#1412, #1642).
func TestIDsNamesEveryCatalogEntryOfAPathIncludingWithheldOnes(t *testing.T) {
	got := runFixture(t, "", "ids", "docs/a1.xml")
	wantLines(t, got,
		"casejoin: 1 path(s), 3 catalog entry(ies) naming them, 1 withheld",
		"S/g1/instance/i1 — under test\n",
		"S/g2/instance/i2 — under test\n",
		"S/g3/instance/i3 — under test — WITHHELD\n",
	)
}

// TestIDsReportsBothDocumentEdges holds the reader to the two lists a case
// names documents through: a schema document is named by its own schemaTest
// and by the instance case assessed against it, and a tool reporting only the
// first answers zero for every schema-side census.
func TestIDsReportsBothDocumentEdges(t *testing.T) {
	got := runFixture(t, "", "ids", "docs/b.xsd")
	wantLines(t, got,
		"S/g4/instance/i4 — assessed against\n",
		"S/g4/schema/s4 — under test\n",
	)
}

// TestIDsReportsAPathNoCatalogEntryNames keeps a path the catalog never
// mentions visible: a census fixture no case names is a fact about the corpus,
// not an empty line to swallow.
func TestIDsReportsAPathNoCatalogEntryNames(t *testing.T) {
	got := runFixture(t, "", "ids", "docs/unreferenced.xml")
	wantLines(t, got, "(no catalog entry names this path)")
}

// TestJoinCountsOnlyTheBankedFailuresThatCouldFlip is the join itself: of the
// three entries naming the path, the withheld one carries no line, and the two
// banked failures are both candidates — the one the suite declares valid
// counted on its own row, since the instance lane's executor decides a
// document valid for an assessed subtree root (#1841, #1855) and it may be one.
func TestJoinCountsOnlyTheBankedFailuresThatCouldFlip(t *testing.T) {
	got := runFixture(t, "", "join", "instance", "docs/a1.xml")
	wantLines(t, got,
		"casejoin: 1 path(s) → 3 catalog entry(ies) → 2 candidate case(s) in lane instance",
		"\n  S/g1/instance/i1\n  S/g2/instance/i2\n",
		"which it does for an assessed subtree root alone, with content or without\n  (#1841, #1855)",
	)
	wantRow(t, got, "withheld — no case produced, nothing to flip (#1412)", 1)
	wantRow(t, got, "banked fail, suite declares it VALID — CANDIDATE as the executor's one valid shape (#1841, #1855)", 1)
	wantRow(t, got, "banked fail — CANDIDATE", 1)
}

// TestJoinSeparatesSuiteValidCasesOnTheInstanceLaneOnly holds the
// declared-valid row to the lane it is a fact about. S/g4/schema/s4 is
// declared valid and banked fail: on the schema lane it is a candidate like any
// other, with no row of its own.
func TestJoinSeparatesSuiteValidCasesOnTheInstanceLaneOnly(t *testing.T) {
	got := runFixture(t, "", "join", "schema", "docs/b.xsd")
	wantLines(t, got,
		"→ 1 candidate case(s) in lane schema",
		"\n  S/g4/schema/s4\n",
	)
	wantRow(t, got, "banked fail — CANDIDATE", 1)
	if strings.Contains(got, "suite declares it VALID") || strings.Contains(got, "#1738") {
		t.Errorf("the instance lane's declared-valid row or caveat is printed for lane schema:\n%s", got)
	}
}

// TestJoinDoesNotCountAPassOrAnUnbankedCase holds the two remaining classes
// apart from the candidates: a case this lane already passes cannot flip up,
// and a case this lane's file carries no line for is not this lane's to count
// (#1412).
func TestJoinDoesNotCountAPassOrAnUnbankedCase(t *testing.T) {
	got := runFixture(t, "", "join", "instance", "docs/b.xsd", "docs/b1.xml")
	wantLines(t, got, "→ 0 candidate case(s) in lane instance")
	wantRow(t, got, "catalog entries naming a path", 2)
	wantRow(t, got, "no line in instance.txt — not banked by this lane", 1)
	wantRow(t, got, "banked pass — cannot flip up", 1)
}

// TestJoinCountsAnEntryOnceHoweverManyGivenPathsItNames pins the deduplication
// the bound depends on: S/g4/instance/i4 names one path under test and another
// it is assessed against, and counting it twice would inflate every figure it
// appears in.
func TestJoinCountsAnEntryOnceHoweverManyGivenPathsItNames(t *testing.T) {
	got := runFixture(t, "docs/b.xsd\ndocs/b1.xml\n", "join", "instance")
	wantRow(t, got, "catalog entries naming a path", 2)
}

// entryCount reads the entry count out of either mode's header sentence, in
// both of which it is the number immediately before "catalog entry(ies)".
func entryCount(t *testing.T, got string) int {
	t.Helper()
	head, _, ok := strings.Cut(got, " catalog entry(ies)")
	if !ok {
		t.Fatalf("no entry count in:\n%s", got)
	}
	fields := strings.Fields(head)
	n, err := strconv.Atoi(fields[len(fields)-1])
	if err != nil {
		t.Fatalf("entry count %q: %v", fields[len(fields)-1], err)
	}
	return n
}

// TestIDsAndJoinCountTheSameDistinctEntries holds the two modes to one
// arithmetic over one input. S/g3/instance/i3 names docs/a1.xml under test and
// docs/c.xsd as the schema it is assessed against, so counting entry–path
// PAIRS in one mode and distinct entries in the other prints two different
// entry counts for one set of paths and a reader cannot tell which number
// means what (#1642). The withheld count dedupes with it: i3 and s3 are both
// withheld, and i3 is named by both paths.
func TestIDsAndJoinCountTheSameDistinctEntries(t *testing.T) {
	ids := runFixture(t, "", "ids", "docs/a1.xml", "docs/c.xsd")
	join := runFixture(t, "", "join", "instance", "docs/a1.xml", "docs/c.xsd")
	if got, want := entryCount(t, ids), entryCount(t, join); got != want {
		t.Errorf("mode ids counts %d catalog entry(ies), mode join %d, over the same paths", got, want)
	}
	wantLines(t, ids, "casejoin: 2 path(s), 4 catalog entry(ies) naming them, 2 withheld")
	wantRow(t, join, "catalog entries naming a path", 4)
	wantRow(t, join, "withheld — no case produced, nothing to flip (#1412)", 2)
}

// TestJoinCaveatNamesAllFourDirections holds the figure's caveat to every
// direction it is wrong in. A caveat naming one of four is the false
// statement #1585 exists to prevent, with a smaller radius.
func TestJoinCaveatNamesAllFourDirections(t *testing.T) {
	got := runFixture(t, "", "join", "instance", "docs/a1.xml")
	wantLines(t, got,
		"BOUND FROM ABOVE",
		"OVER-counts",
		"<element ref> or <group ref>",
		"UNDER-counts",
		"Read only partly",
		"declares VALID",
		indeterminateOverCount,
	)
	header := strings.Index(got, "casejoin: ")
	caveat := strings.Index(got, "BOUND FROM ABOVE")
	body := strings.Index(got, "=== Join against")
	if header >= caveat || caveat >= body {
		t.Errorf("the caveat must sit under the header, ahead of the body (#1279):\n%s", got)
	}
}

// indeterminateOverCount and indeterminateSubtracted open the caveat's
// indeterminate direction without a run log and with one.
const (
	indeterminateOverCount  = "It OVER-counts on every lane by the cases the suite declares indeterminate"
	indeterminateSubtracted = "Each banked fail the run log lists as an indeterminate decline is on its own row below"
)

// indeterminateRow is the label of the row a fed run log's indeterminate
// declines are counted on.
const indeterminateRow = "banked fail, run log lists it an indeterminate decline — never flips (#277)"

// TestJoinCaveatNamesTheIndeterminateDirectionOnEveryLane holds the fourth
// direction off the instance lane's early return: on the schema lane, with no
// log fed, the caveat names the over-count and the flag and log that remove
// it, and prints no indeterminate row, since an unread class is unknown and a
// 0 there would read as a finding.
func TestJoinCaveatNamesTheIndeterminateDirectionOnEveryLane(t *testing.T) {
	got := runFixture(t, "", "join", "schema", "docs/d.xsd")
	wantLines(t, got,
		indeterminateOverCount,
		"Pass -log with a GOXSD_DECLINES=1 conformance run's -v output",
		"→ 1 candidate case(s) in lane schema",
	)
	wantRow(t, got, "banked fail — CANDIDATE", 1)
	if strings.Contains(got, "indeterminate decline —") {
		t.Errorf("an indeterminate row is printed with no run log fed:\n%s", got)
	}
}

// TestJoinFedALogSubtractsIndeterminateDeclines is the row itself, on both
// lanes: the case the run log lists as an indeterminate decline leaves the
// candidates for its own row, and the caveat stops claiming the over-count
// the row has removed.
func TestJoinFedALogSubtractsIndeterminateDeclines(t *testing.T) {
	for _, tc := range []struct{ lane, path, id string }{
		{"instance", "docs/d1.xml", "S/g5/instance/i5"},
		{"schema", "docs/d.xsd", "S/g5/schema/s5"},
	} {
		t.Run(tc.lane, func(t *testing.T) {
			got := runFixture(t, "", "-log", logFile(t, fixtureLog), "join", tc.lane, tc.path)
			wantLines(t, got,
				"→ 0 candidate case(s) in lane "+tc.lane,
				indeterminateSubtracted,
			)
			wantRow(t, got, indeterminateRow, 1)
			wantRow(t, got, "banked fail — CANDIDATE", 0)
			if strings.Contains(got, indeterminateOverCount) {
				t.Errorf("the caveat claims the over-count the row removed:\n%s", got)
			}
			if strings.Contains(got, "\n  "+tc.id+"\n") {
				t.Errorf("%s is listed as a candidate:\n%s", tc.id, got)
			}
		})
	}
}

// TestJoinFedALogStillCountsTheOtherBankedFails holds the subtraction to the
// indeterminate list alone: S/g1/instance/i1 and S/g2/instance/i2 are listed
// as ordinary decline candidates and stay candidates, and the row prints its
// 0 because a log was read. A log whose instance decline candidates name
// their refusals (`<id>=<refusal>`, #2008) reads the same.
func TestJoinFedALogStillCountsTheOtherBankedFails(t *testing.T) {
	reasoned := strings.Replace(fixtureLog, "decline candidates: [S/g1/instance/i1 S/g2/instance/i2]",
		"decline candidates: [S/g1/instance/i1=abstract S/g2/instance/i2=open-content-ldt(#2080)]", 1)
	for _, tc := range []struct{ name, log string }{{"without refusals", fixtureLog}, {"naming refusals", reasoned}} {
		t.Run(tc.name, func(t *testing.T) {
			got := runFixture(t, "", "-log", logFile(t, tc.log), "join", "instance", "docs/a1.xml")
			wantLines(t, got, "→ 2 candidate case(s) in lane instance")
			wantRow(t, got, indeterminateRow, 0)
		})
	}
}

// TestJoinRefusesAnUnusableLog holds every -log fault to an operational error
// (exit 2), each opening with its subject (#1048): a log that cannot be read,
// one with no listing for the lane, one listing it twice, a -log given to the
// mode that names no lane, and a log listing as indeterminate a case the
// catalog declares valid.
func TestJoinRefusesAnUnusableLog(t *testing.T) {
	noInstance := strings.Join(slices.DeleteFunc(strings.Split(fixtureLog, "\n"), func(l string) bool {
		return strings.Contains(l, "lane instance:")
	}), "\n")
	validListed := strings.Replace(fixtureLog, "indeterminate declines: [S/g5/instance/i5]", "indeterminate declines: [S/g2/instance/i2 S/g5/instance/i5]", 1)
	join := []string{"join", "instance", "docs/a1.xml"}
	cases := []struct {
		name, prefix, carries string
		args                  []string
	}{
		{"unreadable", "reading run log", "no such file", append([]string{"-log", filepath.Join(t.TempDir(), "absent.log")}, join...)},
		{"no listing for the lane", "run log ", `carries no "decline candidates" listing for lane instance`, append([]string{"-log", logFile(t, noInstance)}, join...)},
		{"listed twice", "run log ", `lane instance lists "decline candidates" twice`, append([]string{"-log", logFile(t, fixtureLog+fixtureLog)}, join...)},
		{"given to ids", "-log reads a lane's census", `mode "ids" names no lane`, []string{"-log", logFile(t, fixtureLog), "ids", "docs/a1.xml"}},
		{"valid case listed indeterminate", "the run log lists S/g2/instance/i2 as indeterminate", "the catalog declares it valid", append([]string{"-log", logFile(t, validListed)}, join...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			full := append([]string{"-suite", writeTree(t, fixtureSuite), "-expectations", writeTree(t, fixtureLanes)}, tc.args...)
			err := run(&out, strings.NewReader(""), full)
			if err == nil || !strings.HasPrefix(err.Error(), tc.prefix) || !strings.Contains(err.Error(), tc.carries) {
				t.Errorf("run error = %v, want one opening %q and carrying %q; output:\n%s", err, tc.prefix, tc.carries, out.String())
			}
		})
	}
}

// TestPathsAreTakenFromStdinAndRestated pins the input contract: paths arrive
// one per line when none is given on the command line, a path still carrying
// the suite root on its front is restated rather than silently matching
// nothing, blanks are skipped, and one fixture named twice is one fixture.
func TestPathsAreTakenFromStdinAndRestated(t *testing.T) {
	suite := writeTree(t, fixtureSuite)
	var out strings.Builder
	stdin := strings.NewReader("\n" + filepath.ToSlash(filepath.Join(suite, "docs/a1.xml")) + "\n./docs/a1.xml\n")
	args := []string{"-suite", suite, "-expectations", writeTree(t, fixtureLanes), "ids"}
	if err := run(&out, stdin, args); err != nil {
		t.Fatalf("run: %v", err)
	}
	wantLines(t, out.String(), "casejoin: 1 path(s), 3 catalog entry(ies) naming them, 1 withheld")
}

// TestJoinRefusesALaneWithNoExpectationFile keeps a mistyped lane from
// answering zero: LoadExpectations is right to read a missing file as an empty
// lane, and a join that counted against one would report a finding.
func TestJoinRefusesALaneWithNoExpectationFile(t *testing.T) {
	var out strings.Builder
	args := []string{"-suite", writeTree(t, fixtureSuite), "-expectations", writeTree(t, fixtureLanes), "join", "instnace", "docs/a1.xml"}
	err := run(&out, strings.NewReader(""), args)
	if err == nil {
		t.Fatalf("join against a lane with no file = nil error; output:\n%s", out.String())
	}
	if !strings.HasPrefix(err.Error(), `lane "instnace" has no expectation file`) {
		t.Errorf("error %q does not open by naming the lane it refused: the file path behind it "+
			"carries the same word, so only the subject position pins it (#1048)", err)
	}
}

// TestUnusableModeIsAnOperationalError holds the two mode errors to a
// non-zero exit rather than an empty report.
func TestUnusableModeIsAnOperationalError(t *testing.T) {
	for _, args := range [][]string{{}, {"jion", "instance"}, {"join"}} {
		var out strings.Builder
		if err := run(&out, strings.NewReader(""), args); err == nil {
			t.Errorf("run(%q) = nil error, want a usage error", args)
		}
	}
}

// TestCorpusAbsentModeReportsAndExitsCleanly holds the fresh-container case to
// a report and a clean exit (#659): the submodule is absent until it is
// initialized, which is a state to name, not a failure.
func TestCorpusAbsentModeReportsAndExitsCleanly(t *testing.T) {
	var out strings.Builder
	args := []string{"-suite", filepath.Join(t.TempDir(), "absent"), "join", "instance", "docs/a1.xml"}
	if err := run(&out, strings.NewReader(""), args); err != nil {
		t.Fatalf("run over an absent corpus: %v, want a clean exit", err)
	}
	wantLines(t, out.String(), "corpus-absent mode", "git submodule update --init")
}

// refusingWriter fails every write, standing in for a closed stdout.
type refusingWriter struct{}

var errRefused = errors.New("refused")

func (refusingWriter) Write([]byte) (int, error) { return 0, errRefused }

// TestCorpusAbsentWriteErrorIsReturned holds casejoin's corpus-absent mode to
// the write-error policy the suiteroot package doc states.
func TestCorpusAbsentWriteErrorIsReturned(t *testing.T) {
	args := []string{"-suite", filepath.Join(t.TempDir(), "absent"), "ids", "docs/a1.xml"}
	if err := run(refusingWriter{}, strings.NewReader(""), args); !errors.Is(err, errRefused) {
		t.Errorf("run over an absent corpus into a failing stdout = %v, want %v", err, errRefused)
	}
}

// TestJoinRowsPartitionTheEntries holds the table to its own arithmetic: the
// six classes are disjoint and cover every entry, so a reader may subtract
// one row from another — and the indeterminate class is out of candidates().
func TestJoinRowsPartitionTheEntries(t *testing.T) {
	j := joined{
		Lane:             instanceLane,
		LogFed:           true,
		WithheldIDs:      []string{"a"},
		UnscoredIDs:      []string{"b", "c"},
		BankedPassIDs:    []string{"d"},
		IndeterminateIDs: []string{"h", "i"},
		DeclaredValidIDs: []string{"e", "f"},
		CandidateIDs:     []string{"g"},
	}
	if j.entries() != 9 {
		t.Errorf("entries() = %d, want 9", j.entries())
	}
	if got := j.candidates(); !slices.Equal(got, []string{"e", "f", "g"}) {
		t.Errorf("candidates() = %v, want the declared-valid and candidate classes only", got)
	}
	sum := 0
	for _, r := range j.rows() {
		if strings.HasPrefix(r.label, "  ") {
			sum += r.count
		}
	}
	if sum != j.entries() {
		t.Errorf("the indented rows sum to %d, want the entry count %d", sum, j.entries())
	}
}
