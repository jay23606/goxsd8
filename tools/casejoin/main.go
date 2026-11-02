// Command casejoin turns suite fixture PATHS into the conformance case IDs
// conformance/testdata/expectations/<lane>.txt is keyed by, and joins them
// against a lane's committed file.
//
// A ratchet PREDICTION starts from a corpus census — `go tool suiteindex`
// reports which fixtures carry a construct — and CLAUDE.md then prescribes
// joining that census through a lane's expectation file. The two sides are
// keyed differently: a census names a DOCUMENT by path, and an expectation
// file names a CASE by `<testSet>/<testGroup>/<kind>/<test-name>`, an ID
// assembled from catalog attribute values that need bear no relation to the
// href beside them. Nothing bridged them, so two grounding rounds running
// predicted `not derived` and the lane then moved anyway (#1494, #1585). This
// is the bridge, and it is a tool because the walk is deterministic and
// error-prone by hand (PRINCIPLES 27).
//
// # The two questions, which are different
//
// ENUMERATE — `casejoin ids` — answers which catalog entries name a path. The
// relation is one-to-many in that direction: `saxonData/Missing/missing001.v1.xml`
// is named by an instanceTest in four different test groups, so it yields four
// case IDs. Every entry the catalog declares is reported, WITHHELD ones
// included: an entry the suite's applicability metadata scopes away from this
// processor is a real catalog entry that merely carries no line in any lane.
// Both modes COUNT distinct entries — an entry naming two of the given paths
// is one entry to either — so the two headers are arithmetic over one input
// and a reader may compare them.
//
// JOIN — `casejoin join <lane>` — answers how many BANKED cases of one lane
// name those paths and could still flip. A withheld entry carries no line and
// is not counted (#1412); neither is an entry no lane selects, nor one already
// banked `pass`, which cannot flip upward. The four Missing entries above
// therefore join to ZERO, which is the right answer to a different question
// from the four the enumeration reports.
//
// Fed a run log with -log — the -v output of a GOXSD_DECLINES=1 conformance
// run — the join also subtracts, on every lane, each banked `fail` that run
// lists under `indeterminate declines`: a case the suite declares
// indeterminate is declined and always recorded `fail`, so it can never flip
// (conformance/doc.go, #277). The catalog alone cannot tell such a case from
// an invalid one, so without a log it stays a candidate and the caveat says so.
//
// # Which document edges are reported
//
// All of them. A case names its documents through two lists, and a path
// matching either one is reported: the document(s) UNDER TEST — a
// schemaTest's ordered <schemaDocument> list, or an instanceTest's one
// <instanceDocument> — and, for an instance case, the schema documents it is
// ASSESSED AGAINST, which are its test group's sibling schemaTest's list
// (conformance.CatalogEntry). Reporting only the first would silently answer
// zero for every schema-side census, whose fixtures are named by the instance
// cases assessed against them. Each reported entry says which edge matched.
//
// # What the count is, and is not
//
// A candidate case count is a BOUND FROM ABOVE on the cases a change to the
// censused construct could flip — never a prediction of flips, and never a
// lane movement. The report says so where the figure is, and names the four
// directions it is wrong in.
//
// # Usage
//
//	go tool suiteindex -paths '{*}*@{http://www.w3.org/2001/XMLSchema-instance}type' | go tool casejoin join instance
//	go tool casejoin -log run.log join instance msData/schema/schA2_a.xsd
//	go tool casejoin ids saxonData/Missing/missing001.v1.xml
//	go tool casejoin -suite other/dir ids < paths.txt
//
// Paths are taken from the command line, or, when none is given, from stdin,
// one per line. Each is a path relative to the suite root — the spelling every
// `suiteindex` report prints — and one still carrying the suite root on its
// front is accepted too.
//
// An absent suite submodule is a supported mode, not a failure (#659): the
// tool says the corpus is not there, names the command that initializes it,
// and exits 0, or 2 when that notice cannot be written. It exits 0 for a
// completed report whatever it finds, even when the report cannot be written,
// and 2 for an operational error: an unusable mode, an unreadable catalog, a
// lane name no expectation file carries, or a -log that cannot be read, is
// given to `ids`, lists the lane's census other than exactly once, or lists a
// case the catalog declares valid as indeterminate.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kud360/goxsd8/conformance"
	"github.com/kud360/goxsd8/tools/internal/declinecensus"
	"github.com/kud360/goxsd8/tools/internal/suiteroot"
)

// defaultExpectations is the committed expectations directory, as written
// from the module root — the same path `go tool lanestatus` reads.
const defaultExpectations = "conformance/testdata/expectations"

// suiteIndexName is the suite's own index, the file a populated submodule
// carries. Its absence is the corpus-absent mode: conformance.Catalog reports
// it as an error, and this tool reports it as a mode (#659), so it is checked
// here before the catalog is read.
//
// It is the SECOND construction of that filename, conformance's own
// (unexported) suiteIndexIn being the first. Rename the file in one and rename
// it in the other, or this tool reports "nothing to join" over a suite that is
// present.
const suiteIndexName = "suite.xml"

const usage = `usage: casejoin [-suite dir] [-expectations dir] [-log file] ids|join <lane> [path...]
  ids          every catalog case ID naming each path, withheld entries included
  join <lane>  the banked cases of <lane> naming those paths that could still flip
  -log file    join only: a GOXSD_DECLINES=1 conformance run's -v output, subtracting its indeterminate declines
paths come from the command line, or from stdin one per line when none is given`

func main() {
	if err := run(os.Stdout, os.Stdin, os.Args[1:]); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "casejoin: %v\n", err)
		os.Exit(2)
	}
}

// run drives one report end to end: parse the command line, read the paths,
// read the catalog, render. It takes its destination, its path source and its
// arguments so a test can drive the whole command without a subprocess.
func run(stdout io.Writer, stdin io.Reader, args []string) error {
	flags := flag.NewFlagSet("casejoin", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	suite := flags.String("suite", suiteroot.Default, "the suite checkout to read the catalog from")
	expectations := flags.String("expectations", defaultExpectations, "the directory holding the per-lane expectation files")
	logPath := flags.String("log", "", "a GOXSD_DECLINES=1 conformance run's -v output")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%s", err, usage)
	}
	mode, lane, rest, err := parseMode(flags.Args())
	if err != nil {
		return err
	}
	if mode == modeIDs && *logPath != "" {
		return fmt.Errorf("-log reads a lane's census, and mode %q names no lane\n%s", modeIDs, usage)
	}
	index := filepath.Join(*suite, suiteIndexName)
	if _, serr := os.Stat(index); errors.Is(serr, fs.ErrNotExist) {
		return suiteroot.PrintAbsent(stdout, "casejoin", "no suite index at "+index, "join")
	}
	paths, err := readPaths(*suite, rest, stdin)
	if err != nil {
		return err
	}
	entries, err := conformance.Catalog(*suite)
	if err != nil {
		return err
	}
	found := namings(entries, paths)
	if mode == modeIDs {
		printIDs(stdout, paths, found)
		return nil
	}
	file := laneFile(*expectations, lane)
	banked, err := loadLane(lane, file)
	if err != nil {
		return err
	}
	var census *declinecensus.Census
	if *logPath != "" {
		c, err := declinecensus.Read(*logPath, lane)
		if err != nil {
			return err
		}
		census = &c
	}
	j, err := joinLane(lane, paths, found, banked, census)
	if err != nil {
		return err
	}
	printJoin(stdout, j, file)
	return nil
}

// The two modes, which answer the two different questions the package doc
// separates.
const (
	modeIDs  = "ids"
	modeJoin = "join"
)

// parseMode reads the mode, the lane a join names, and the paths behind them.
func parseMode(args []string) (mode, lane string, paths []string, err error) {
	if len(args) == 0 {
		return "", "", nil, fmt.Errorf("no mode given\n%s", usage)
	}
	switch args[0] {
	case modeIDs:
		return modeIDs, "", args[1:], nil
	case modeJoin:
		if len(args) < 2 {
			return "", "", nil, fmt.Errorf("mode %q names the lane to join against\n%s", modeJoin, usage)
		}
		return modeJoin, args[1], args[2:], nil
	}
	return "", "", nil, fmt.Errorf("unknown mode %q\n%s", args[0], usage)
}

// readPaths collects the fixture paths to look up: the arguments, or stdin one
// per line when there are none. Blank lines are skipped, every path is
// restated in the catalog's own spelling, and the result is sorted and
// deduplicated so one fixture named twice is one fixture.
func readPaths(suiteDir string, args []string, stdin io.Reader) ([]string, error) {
	raw := args
	if len(raw) == 0 {
		lines, err := readLines(stdin)
		if err != nil {
			return nil, err
		}
		raw = lines
	}
	var paths []string
	for _, r := range raw {
		p := normalizePath(suiteDir, r)
		if p == "" {
			continue
		}
		paths = append(paths, p)
	}
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

// readLines reads stdin as one path per line. Empty input is a supported mode
// — a census that matched nothing produces it — and yields no path rather than
// an error.
func readLines(stdin io.Reader) ([]string, error) {
	var lines []string
	sc := bufio.NewScanner(stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading paths from stdin: %w", err)
	}
	return lines, nil
}

// normalizePath restates one input path in the spelling the catalog reports:
// a slash path relative to the suite root, which is what every `suiteindex`
// report prints. A path pasted with the suite root still on its front is
// accepted and restated, since that is how a path read off a shell line or a
// LOG entry usually arrives; a path that matches nothing after this is
// reported as naming no catalog entry rather than silently dropped.
func normalizePath(suiteDir, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	p := path.Clean(filepath.ToSlash(trimmed))
	root := path.Clean(filepath.ToSlash(suiteDir))
	if root != "." && strings.HasPrefix(p, root+"/") {
		return p[len(root)+1:]
	}
	return p
}

// edge names which of an entry's two document lists a path matched. The
// distinction survives into the report because it is what a reader checks a
// candidate against: a case whose own document was censused, or one merely
// assessed against a censused schema.
type edge string

const (
	edgeUnderTest edge = "under test"
	edgeAssessed  edge = "assessed against"
)

// naming is one catalog entry that names a path, and the edges it named it
// through — both, in the rare case where a case's own document is also the
// schema document it is assessed against.
type naming struct {
	entry conformance.CatalogEntry
	edges []edge
}

// namings reports, per path, the catalog entries naming it, in case-ID order.
// The result is keyed by path and read through the caller's own sorted path
// slice, so no map iteration reaches the output (STYLE D2).
func namings(entries []conformance.CatalogEntry, paths []string) map[string][]naming {
	wanted := map[string]struct{}{}
	for _, p := range paths {
		wanted[p] = struct{}{}
	}
	found := map[string][]naming{}
	for _, e := range entries {
		record(found, wanted, e, e.Docs, edgeUnderTest)
		record(found, wanted, e, e.SchemaDocs, edgeAssessed)
	}
	return found
}

// record files one entry under each wanted document it names. Entries arrive
// in case-ID order, so each path's list is in that order already; an entry
// naming a path twice — the same document listed twice, or matched on both
// edges — is recorded once, with both edges.
func record(found map[string][]naming, wanted map[string]struct{}, e conformance.CatalogEntry, docs []string, at edge) {
	for _, doc := range docs {
		if _, ok := wanted[doc]; !ok {
			continue
		}
		list := found[doc]
		if n := len(list); n > 0 && list[n-1].entry.ID == e.ID {
			if !slices.Contains(list[n-1].edges, at) {
				list[n-1].edges = append(list[n-1].edges, at)
			}
			continue
		}
		found[doc] = append(list, naming{entry: e, edges: []edge{at}})
	}
}

// laneFile is a lane's committed expectation file. It is the one construction
// of that name WITHIN THIS TOOL (STYLE D3): the join reads it and the report
// names it, and a report naming a file the join did not read would be
// unfalsifiable.
//
// It is a SECOND construction of that name, conformance's own (unexported)
// laneFileIn being the first and tools/lanepartition's run another — the same
// split as suiteIndexName above, and for the same reason: this tool cannot
// reach an unexported helper across the package boundary, and one
// filepath.Join does not earn an exported one. Rename the convention in one
// and rename it in the other, or this tool joins against a lane file nobody
// writes.
func laneFile(dir, lane string) string {
	return filepath.Join(dir, lane+".txt")
}

// loadLane reads one lane's committed expectations. A lane file that does not
// exist is an ERROR here and not an empty lane: conformance.LoadExpectations
// is right to read a missing file as a lane with no case, but a join told to
// count against a lane nobody has ever banked would answer zero and look like
// a finding.
func loadLane(lane, file string) (map[string]conformance.Status, error) {
	if _, err := os.Stat(file); err != nil {
		return nil, fmt.Errorf("lane %q has no expectation file at %s: %w", lane, file, err)
	}
	return conformance.LoadExpectations(file)
}

// joined is one completed join: how the entries naming the given paths
// partition against a lane's committed file.
//
// The six classes are DISJOINT and cover every entry, so the rows of the
// report sum to the entry count. Only facts that cannot be recomputed are
// stored — each class holds its case IDs and the report counts them (STYLE
// D3).
type joined struct {
	Lane string
	// LogFed records whether a run log was read, which an empty
	// IndeterminateIDs cannot: without one that class is unknown, not empty.
	LogFed bool
	// Paths are the paths given, and Unnamed those of them no catalog entry
	// names at all.
	Paths   []string
	Unnamed []string
	// WithheldIDs are entries the suite's applicability metadata scopes away.
	// Discovery produces no case for one, so it carries no line in any lane
	// and nothing about it can flip a score (#1412).
	WithheldIDs []string
	// UnscoredIDs are entries this lane's file carries no line for although
	// discovery produced them — the other lane's kind, a case whose line no
	// run has ever banked.
	UnscoredIDs []string
	// BankedPassIDs are entries this lane already records `pass`; they cannot
	// flip upward.
	BankedPassIDs []string
	// IndeterminateIDs are entries banked `fail` that the fed run log lists as
	// indeterminate declines: the suite declares them indeterminate, so the
	// harness declines them and records `fail` whatever the engine does, and
	// none can flip (#277). Empty unless LogFed.
	IndeterminateIDs []string
	// DeclaredValidIDs are candidates, banked `fail` on the instance lane, that
	// the suite declares VALID. They are counted apart from CandidateIDs because
	// that lane's executor decides a document valid for one shape alone — an
	// assessed subtree root, with content or without (#1841, #1855) — so only
	// a case of that shape among them can flip. On any other lane the field is
	// empty and such an entry is in CandidateIDs like any other.
	DeclaredValidIDs []string
	// CandidateIDs are the other banked `fail` entries. Together with
	// DeclaredValidIDs they are the bound from above (candidates).
	CandidateIDs []string
}

// candidates returns every candidate case, in case-ID order: the two banked
// `fail` classes together, derived rather than stored (STYLE D3).
func (j joined) candidates() []string {
	all := slices.Concat(j.DeclaredValidIDs, j.CandidateIDs)
	slices.Sort(all)
	return all
}

// instanceLane is the one lane whose executor observes "valid" for one shape
// only: its banked `fail` on a case the suite declares VALID flips only where
// that case is an assessed subtree root (#1841, #1855). Before #1738 such a case could
// not flip at all and the join subtracted it (#1561); it is now a candidate,
// counted on a row of its own on this lane alone.
const instanceLane = "instance"

// distinctEntries reports the entries naming any of the given paths, each ONCE
// however many of those paths it names, in path order and case-ID order within
// a path. It is the ONE dedupe both modes count through (STYLE D3): an entry
// that names one given path under test and another it is assessed against is
// one entry to the enumeration and one to the join, so the two report the same
// arithmetic over the same input rather than entry–path pairs against distinct
// entries (#1642).
//
// It drops the edges each naming matched through, which belong to one PATH
// and not to the deduplicated entry; the per-path listing reports them from
// the namings themselves.
func distinctEntries(paths []string, found map[string][]naming) []conformance.CatalogEntry {
	seen := map[string]struct{}{}
	var out []conformance.CatalogEntry
	for _, p := range paths {
		for _, n := range found[p] {
			if _, dup := seen[n.entry.ID]; dup {
				continue
			}
			seen[n.entry.ID] = struct{}{}
			out = append(out, n.entry)
		}
	}
	return out
}

// joinLane partitions the entries naming paths against one lane's committed
// file and, when census is not nil, that lane's run-log census, in case-ID
// order throughout (STYLE D1).
func joinLane(lane string, paths []string, found map[string][]naming, banked map[string]conformance.Status, census *declinecensus.Census) (joined, error) {
	j := joined{Lane: lane, LogFed: census != nil, Paths: paths}
	for _, p := range paths {
		if len(found[p]) == 0 {
			j.Unnamed = append(j.Unnamed, p)
		}
	}
	indeterminate := map[string]struct{}{}
	if census != nil {
		for _, id := range census.Indeterminate {
			indeterminate[id] = struct{}{}
		}
	}
	for _, e := range distinctEntries(paths, found) {
		if err := j.classify(e, banked, indeterminate); err != nil {
			return joined{}, err
		}
	}
	slices.Sort(j.WithheldIDs)
	slices.Sort(j.UnscoredIDs)
	slices.Sort(j.BankedPassIDs)
	slices.Sort(j.IndeterminateIDs)
	slices.Sort(j.DeclaredValidIDs)
	slices.Sort(j.CandidateIDs)
	return j, nil
}

// classify files one entry in the single class it belongs to.
//
// Withheld is read BEFORE the lane's file rather than as a reason a line is
// missing: discovery produces no case for a withheld entry, so whether a line
// survives for it decides nothing. A line that does survive one — a sanctioned
// applicability removal a re-pin has just created — is deleted by the next
// ratchet rather than flipped, so it is no candidate either.
//
// A banked `fail` the run log lists as an indeterminate decline is filed
// before the declared-valid test. An entry the catalog declares VALID listed
// so is an error, as `go tool lanepartition` treats it: the log is then not
// from this suite, and subtracting the case would hide a candidate.
func (j *joined) classify(e conformance.CatalogEntry, banked map[string]conformance.Status, indeterminate map[string]struct{}) error {
	if e.Withheld {
		j.WithheldIDs = append(j.WithheldIDs, e.ID)
		return nil
	}
	status, ok := banked[e.ID]
	if !ok {
		j.UnscoredIDs = append(j.UnscoredIDs, e.ID)
		return nil
	}
	if status.IsPass() {
		j.BankedPassIDs = append(j.BankedPassIDs, e.ID)
		return nil
	}
	if _, listed := indeterminate[e.ID]; listed {
		if e.DeclaredValid {
			return fmt.Errorf("the run log lists %s as indeterminate but the catalog declares it valid — the log is not from this suite", e.ID)
		}
		j.IndeterminateIDs = append(j.IndeterminateIDs, e.ID)
		return nil
	}
	if j.Lane == instanceLane && e.DeclaredValid {
		j.DeclaredValidIDs = append(j.DeclaredValidIDs, e.ID)
		return nil
	}
	j.CandidateIDs = append(j.CandidateIDs, e.ID)
	return nil
}

// entries reports how many catalog entries named a path, which is the sum of
// the six classes and is derived rather than stored (STYLE D3).
func (j joined) entries() int {
	return len(j.WithheldIDs) + len(j.UnscoredIDs) + len(j.BankedPassIDs) +
		len(j.IndeterminateIDs) + len(j.DeclaredValidIDs) + len(j.CandidateIDs)
}

// printIDs renders the enumeration: every catalog entry naming each path.
//
// The header counts DISTINCT entries, withheld ones included, which is what
// the join counts too. The listing below it still prints an entry under every
// path it names — that relation is the question this mode answers — so an
// entry naming two of the given paths appears twice there and once in the
// count above.
func printIDs(w io.Writer, paths []string, found map[string][]naming) {
	distinct := distinctEntries(paths, found)
	withheld := 0
	for _, e := range distinct {
		if e.Withheld {
			withheld++
		}
	}
	_, _ = fmt.Fprintf(w, "casejoin: %d path(s), %d catalog entry(ies) naming them, %d withheld\n",
		len(paths), len(distinct), withheld)
	printWithheldNote(w)

	_, _ = fmt.Fprintln(w, "\n=== Catalog entries (path order; case-ID order within a path) ===")
	if len(paths) == 0 {
		_, _ = fmt.Fprintln(w, "(no path given)")
	}
	for _, p := range paths {
		_, _ = fmt.Fprintf(w, "  %s\n", p)
		if len(found[p]) == 0 {
			_, _ = fmt.Fprintln(w, "      (no catalog entry names this path)")
			continue
		}
		for _, n := range found[p] {
			_, _ = fmt.Fprintf(w, "      %s — %s%s\n", n.entry.ID, renderEdges(n.edges), renderWithheld(n.entry))
		}
	}
}

// printWithheldNote states what a WITHHELD entry is, under the header the
// reader holding the figure arrives at (#1279) rather than in a section of its
// own.
func printWithheldNote(w io.Writer) {
	_, _ = fmt.Fprintln(w, "  a WITHHELD entry is one the suite's own applicability metadata scopes away from this")
	_, _ = fmt.Fprintln(w, "  processor: discovery produces no case for it, it carries no line in any lane file, and")
	_, _ = fmt.Fprintln(w, "  no change can flip it (#1412). It is a catalog entry all the same, which is why it is here")
}

// renderEdges names the document lists an entry matched a path through, in
// the fixed order the report always prints them.
func renderEdges(edges []edge) string {
	names := make([]string, 0, len(edges))
	for _, e := range []edge{edgeUnderTest, edgeAssessed} {
		if slices.Contains(edges, e) {
			names = append(names, string(e))
		}
	}
	return strings.Join(names, ", ")
}

// renderWithheld marks a withheld entry and prints nothing for a produced one,
// so the marker is the exception it names.
func renderWithheld(e conformance.CatalogEntry) string {
	if !e.Withheld {
		return ""
	}
	return " — WITHHELD"
}

// printJoin renders the join: the partition, then the candidate IDs
// themselves, then the paths nothing names. file is the lane file the join
// read, named so the figure can be checked against it.
func printJoin(w io.Writer, j joined, file string) {
	_, _ = fmt.Fprintf(w, "casejoin: %d path(s) → %d catalog entry(ies) → %d candidate case(s) in lane %s\n",
		len(j.Paths), j.entries(), len(j.candidates()), j.Lane)
	printCaveat(w, j)

	_, _ = fmt.Fprintf(w, "\n=== Join against %s ===\n", file)
	for _, r := range j.rows() {
		_, _ = fmt.Fprintf(w, "  %-58s %7d\n", r.label, r.count)
	}

	printIDSection(w, "Candidate cases (ID order)", j.candidates())
	printPathSection(w, "Paths no catalog entry names (path order)", j.Unnamed)
}

// row is one line of the partition table.
type row struct {
	label string
	count int
}

// rows renders the partition, indented so the six disjoint classes read as
// the parts of the entry count above them. The indeterminate row is printed
// only when a run log was fed, and then on every lane: without one the class
// is unknown, and a 0 would read as a finding. The declared-valid row is
// printed only on the instance lane, which alone files such a case apart
// (#1738); on any other lane it would stand at zero and read as a claim that
// lane makes no such cases.
func (j joined) rows() []row {
	rows := []row{
		{"paths given", len(j.Paths)},
		{"paths no catalog entry names", len(j.Unnamed)},
		{"catalog entries naming a path", j.entries()},
		{"  withheld — no case produced, nothing to flip (#1412)", len(j.WithheldIDs)},
		{"  no line in " + j.Lane + ".txt — not banked by this lane", len(j.UnscoredIDs)},
		{"  banked pass — cannot flip up", len(j.BankedPassIDs)},
	}
	if j.LogFed {
		rows = append(rows, row{"  banked fail, run log lists it an indeterminate decline — never flips (#277)", len(j.IndeterminateIDs)})
	}
	if j.Lane == instanceLane {
		rows = append(rows, row{"  banked fail, suite declares it VALID — CANDIDATE as the executor's one valid shape (#1841, #1855)", len(j.DeclaredValidIDs)})
	}
	return append(rows, row{"  banked fail — CANDIDATE", len(j.CandidateIDs)})
}

// printCaveat states what the candidate count is and the four directions it
// is wrong in, under the header rather than in a section of its own, because
// that is where the reader holding the figure arrives (#1279). Unqualified the
// figure is a false statement shipped in a tool's own output, which is the
// defect #1585 exists to prevent — and one direction named of four would be
// the same defect with a smaller radius.
func printCaveat(w io.Writer, j joined) {
	_, _ = fmt.Fprintln(w, "  read this as a BOUND FROM ABOVE on the cases a change could flip, never as a count of")
	_, _ = fmt.Fprintln(w, "  flips and never as lane movement. It OVER-counts: a case is a candidate for naming a")
	_, _ = fmt.Fprintln(w, "  censused fixture, whether or not the change reaches it, and the census behind those paths")
	_, _ = fmt.Fprintln(w, "  resolves no <element ref> or <group ref>, so its population is a LEXICAL one and not a")
	_, _ = fmt.Fprintln(w, "  population of resolved components. It UNDER-counts: a fixture that census could read only")
	_, _ = fmt.Fprintln(w, "  partly hides every construct behind the fault, so cases naming it never reach this join —")
	_, _ = fmt.Fprintln(w, "  read the census's own \"Read only partly\" count beside this figure.")
	printIndeterminateCaveat(w, j.LogFed)
	if j.Lane != instanceLane {
		return
	}
	_, _ = fmt.Fprintln(w, "  On this lane a candidate the suite declares VALID flips only where the executor decides the")
	_, _ = fmt.Fprintln(w, "  document valid, which it does for an assessed subtree root alone, with content or without")
	_, _ = fmt.Fprintln(w, "  (#1841, #1855):")
	_, _ = fmt.Fprintln(w, "  the figure includes them, and their own row below counts them, so read that row as the looser")
	_, _ = fmt.Fprintln(w, "  part of the bound.")
}

// printIndeterminateCaveat states the indeterminate direction, on every lane:
// with no run log an over-count, naming the flag that removes it; with one, a
// statement that the row below has removed it, since claiming the over-count
// then would be false.
func printIndeterminateCaveat(w io.Writer, logFed bool) {
	if logFed {
		_, _ = fmt.Fprintln(w, "  Each banked fail the run log lists as an indeterminate decline is on its own row below and")
		_, _ = fmt.Fprintln(w, "  out of the figure: the suite declares it indeterminate, so it is declined and always recorded")
		_, _ = fmt.Fprintln(w, "  fail, and never flips (#277).")
		return
	}
	_, _ = fmt.Fprintln(w, "  It OVER-counts on every lane by the cases the suite declares indeterminate: each is declined")
	_, _ = fmt.Fprintln(w, "  and always recorded fail, so it never flips (#277), and the catalog alone cannot tell one from")
	_, _ = fmt.Fprintln(w, "  an invalid case. Pass -log with a GOXSD_DECLINES=1 conformance run's -v output to subtract them.")
}

// printIDSection lists one class's case IDs under its heading.
func printIDSection(w io.Writer, heading string, ids []string) {
	_, _ = fmt.Fprintf(w, "\n=== %s ===\n", heading)
	if len(ids) == 0 {
		_, _ = fmt.Fprintln(w, "(none)")
	}
	for _, id := range ids {
		_, _ = fmt.Fprintf(w, "  %s\n", id)
	}
}

// printPathSection lists one class's paths under its heading. It is separate
// from printIDSection so neither heading can be printed over the other's
// content.
func printPathSection(w io.Writer, heading string, paths []string) {
	_, _ = fmt.Fprintf(w, "\n=== %s ===\n", heading)
	if len(paths) == 0 {
		_, _ = fmt.Fprintln(w, "(none)")
	}
	for _, p := range paths {
		_, _ = fmt.Fprintf(w, "  %s\n", p)
	}
}
