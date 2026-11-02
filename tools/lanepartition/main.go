// Command lanepartition partitions one lane's banked `fail` lines into
// clusters — by what the suite declares about each case and by the test set it
// comes from, and, fed a conformance run's decline census, by whether the
// engine declined it or decided it wrongly — and ranks the clusters by size,
// so a planning pass reads which single decision holds the most failures
// instead of partitioning an expectation file by hand (#1740).
//
// # What it reads
//
// The lane's committed file, conformance/testdata/expectations/<lane>.txt: its
// `fail` lines are the population, every figure the report prints is a part of
// it, and nothing a run did changes it. The suite catalog, through
// conformance.Catalog, for each case's declared validity; that answers VALID
// or NOT — the catalog folds indeterminate into not-valid — so on its own the
// report splits a lane's fails two ways.
//
// A run's output, with -log: the `-v` output of
//
//	GOXSD_DECLINES=1 go test ./conformance -run TestConformance -count=1 -timeout 30m -v
//
// whose decline census lists, per lane, three sorted case-ID lists
// partitioning that run's failures (conformance/doc.go, "The decline
// census"): decline candidates, indeterminate declines, and decided
// disagreements. Fed one, the report classes each banked fail by the list
// naming it, splits not-valid into invalid and indeterminate by the second
// list, and on a lane whose run charges a rule — the schema lane — clusters
// each decided disagreement by its charge, in the words conformance/doc.go's
// "The decline census" section owns. On a lane whose run names its refusals —
// the instance lane — it clusters each decline by the refusal that declined
// it (#2008); a log whose listing names none partitions as it did before
// refusals were named. A banked fail no list names is one the
// run passed or did not produce; the log is from a tree whose engine differs
// from the file's, and the report counts such cases rather than guessing
// their class.
//
// An issue list on stdin, shaped as docs/ROUTINES.md's "Survey input" writes
// `gapissues.json` — rows carrying number, state and body: each
// cluster names the OPEN issues whose body names its test set or its charged
// rule as a whole token. Empty stdin is a supported mode; the report says it
// reconciled nothing. Every fed row must carry `state`, since an absent one
// would read as open: a list with a row lacking it is discarded whole, the
// report prints without reconciliation, and the run exits 2 naming the first
// such issue (#1604).
//
// # Usage
//
//	go tool lanepartition instance < /dev/null
//	go tool lanepartition -log run.log schema < gapissues.json
//
// It exits 0 for a report, whatever it finds, and 2 for an operational
// error: an unusable command line, an unreadable expectation file, catalog or
// log, a log carrying no census listing for the lane, stdin that does not
// decode as the documented JSON, or a fed row carrying no `state`.
package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kud360/goxsd8/conformance"
	"github.com/kud360/goxsd8/tools/internal/declinecensus"
	"github.com/kud360/goxsd8/tools/internal/suiteroot"
)

// defaultExpectations is the committed expectations directory as written from
// the module root — the path `go tool casejoin` and `go tool lanestatus` read.
const defaultExpectations = "conformance/testdata/expectations"

// instanceLane is the lane whose executor observes "valid" for one shape only:
// its banked fail on a case the suite declares VALID flips only where that
// case is an assessed subtree root (#1841, #1855). Before #1738 such a case
// could not flip at all (#1561).
const instanceLane = "instance"

const usage = `usage: lanepartition [-suite dir] [-expectations dir] [-log file] <lane> [< gapissues.json]
  <lane>     the lane whose banked fails are partitioned
  -log file  a GOXSD_DECLINES=1 conformance run's -v output, splitting declines from decided-and-wrong
  stdin      gh issue list --json number,title,state,body output, or empty to reconcile nothing`

func main() {
	if err := run(os.Stdout, os.Stdin, os.Args[1:]); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "lanepartition: %v\n", err)
		os.Exit(2)
	}
}

// run drives one report end to end. A fed list missing `state` does not stop
// the report: it prints without reconciliation and the error is returned
// after it, as wipsurvey does (#1604).
func run(stdout io.Writer, stdin io.Reader, args []string) error {
	flags := flag.NewFlagSet("lanepartition", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	suite := flags.String("suite", suiteroot.Default, "the suite checkout to read the catalog from")
	expectations := flags.String("expectations", defaultExpectations, "the directory holding the per-lane expectation files")
	logPath := flags.String("log", "", "a GOXSD_DECLINES=1 conformance run's -v output")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%s", err, usage)
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("name exactly one lane\n%s", usage)
	}
	lane := flags.Arg(0)

	issues, feedErr := readIssues(stdin)
	if feedErr != nil && !errors.Is(feedErr, errMissingState) {
		return feedErr
	}
	// A further construction of the lane-file name conformance's unexported
	// laneFileIn owns: rename the convention there and it is renamed here.
	file := filepath.Join(*expectations, lane+".txt")
	banked, err := bankedFails(file)
	if err != nil {
		return err
	}
	entries, err := conformance.Catalog(*suite)
	if err != nil {
		return fmt.Errorf("reading the suite catalog (initialize it with `git submodule update --init %s`): %w", suiteroot.Default, err)
	}
	var census *declinecensus.Census
	if *logPath != "" {
		c, err := declinecensus.Read(*logPath, lane)
		if err != nil {
			return err
		}
		census = &c
	}
	p, err := partition(lane, banked, entries, census)
	if err != nil {
		return err
	}
	render(stdout, p, file, *logPath, issues, feedErr)
	return feedErr
}

// bankedFails reads the case IDs a lane file banks `fail`, in ID order. A
// missing file is an error rather than an empty lane: a partition of a lane
// nobody has banked would print zero and read as a finding.
func bankedFails(file string) ([]string, error) {
	if _, err := os.Stat(file); err != nil {
		return nil, fmt.Errorf("no expectation file at %s: %w", file, err)
	}
	statuses, err := conformance.LoadExpectations(file)
	if err != nil {
		return nil, err
	}
	var ids []string
	for id, st := range statuses {
		if st.IsPass() {
			continue
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}

// A banked fail's declared validity, as the report can know it. notValid is
// the two-way reading the catalog alone gives; fed a log, it splits into
// invalid and indeterminate.
const (
	declaredValid         = "valid"
	declaredInvalid       = "invalid"
	declaredIndeterminate = "indeterminate"
	declaredNotValid      = "invalid|indeterminate"
)

// A banked fail's class. Without a log every case is classBanked; with one it
// is the census list naming it, or classUnlisted.
const (
	classBanked        = "banked fail"
	classDeclined      = "declined"
	classIndeterminate = "indeterminate"
	classDecided       = "decided and wrong"
	classUnlisted      = "in no census list"
)

// classOrder and declaredOrder are the fixed orders the report prints classes
// and declared validities in — the table's rows and columns, and the
// tie-break between clusters of one size.
var (
	classOrder    = []string{classBanked, classDeclined, classIndeterminate, classDecided, classUnlisted}
	declaredOrder = []string{declaredValid, declaredInvalid, declaredIndeterminate, declaredNotValid}
)

// cluster is one set of banked fails sharing a class, a declared validity, a
// test set and a reason.
type cluster struct {
	clusterKey
	ids []string
}

// clusterKey is what the cases of one cluster share. reason is what the run's
// listing names after the case's ID: for a decided case on a lane whose run
// charges one, the charge; for a declined case on a lane whose run names one,
// the refusal; otherwise empty. The class tells the two apart.
type clusterKey struct {
	class, declared, testSet, reason string
}

// partitioned is one completed partition of a lane's banked fails.
type partitioned struct {
	lane   string
	logFed bool
	banked int
	cells  map[[2]string]int // (class, declared) → count; read through the fixed orders only
	chunks []cluster         // largest first
	extra  int               // census-listed IDs this file does not bank fail
}

// charged reports whether the run charged any of this lane's decided cases.
func (p partitioned) charged() bool {
	return slices.ContainsFunc(p.chunks, func(c cluster) bool { return c.class == classDecided && c.reason != "" })
}

// refused reports whether the run named the refusal of any of this lane's
// declined cases.
func (p partitioned) refused() bool {
	return slices.ContainsFunc(p.chunks, func(c cluster) bool { return c.class == classDeclined && c.reason != "" })
}

// partition classes every banked fail and groups them into clusters. A banked
// fail with no catalog entry, or a log listing a case indeterminate that the
// catalog declares valid, is an error: the file, the suite and the log are
// then not one tree's.
func partition(lane string, banked []string, entries []conformance.CatalogEntry, census *declinecensus.Census) (partitioned, error) {
	catalog := make(map[string]conformance.CatalogEntry, len(entries))
	for _, e := range entries {
		catalog[e.ID] = e
	}
	p := partitioned{lane: lane, logFed: census != nil, banked: len(banked), cells: map[[2]string]int{}}
	listed := listingOf(census)
	index := map[clusterKey]int{}
	for _, id := range banked {
		e, ok := catalog[id]
		if !ok {
			return partitioned{}, fmt.Errorf("banked case %s has no catalog entry — the suite checkout is not the one %s.txt was banked against", id, lane)
		}
		class, reason := listed.classOf(id, census)
		declared, err := declaredOf(e, class, census != nil)
		if err != nil {
			return partitioned{}, err
		}
		p.cells[[2]string{class, declared}]++
		testSet, _, _ := strings.Cut(id, "/")
		key := clusterKey{class: class, declared: declared, testSet: testSet, reason: reason}
		i, seen := index[key]
		if !seen {
			i = len(p.chunks)
			index[key] = i
			p.chunks = append(p.chunks, cluster{clusterKey: key})
		}
		p.chunks[i].ids = append(p.chunks[i].ids, id)
	}
	p.extra = listed.unbanked(banked)
	slices.SortStableFunc(p.chunks, compareClusters)
	return p, nil
}

// censusListing is the census inverted to a lookup by case ID.
type censusListing map[string]string

// listingOf inverts the census's lists into one lookup of each listed ID's
// class. A nil census lists nothing.
func listingOf(c *declinecensus.Census) censusListing {
	listed := censusListing{}
	if c == nil {
		return listed
	}
	for id := range c.Declined {
		listed[id] = classDeclined
	}
	for _, id := range c.Indeterminate {
		listed[id] = classIndeterminate
	}
	for id := range c.Decided {
		listed[id] = classDecided
	}
	return listed
}

// classOf is a banked fail's class and its reason: for a decided case its
// charge, for a declined case its refusal.
func (l censusListing) classOf(id string, census *declinecensus.Census) (class, reason string) {
	if census == nil {
		return classBanked, ""
	}
	class, ok := l[id]
	if !ok {
		return classUnlisted, ""
	}
	switch class {
	case classDecided:
		return class, census.Decided[id]
	case classDeclined:
		return class, census.Declined[id]
	}
	return class, ""
}

// unbanked counts the IDs the census lists that the lane file does not bank
// fail: a case the run failed that the file banks pass — a regression — or
// carries no line for.
func (l censusListing) unbanked(banked []string) int {
	n := 0
	for id := range l {
		if _, found := slices.BinarySearch(banked, id); !found {
			n++
		}
	}
	return n
}

// declaredOf is what the suite declares about a banked fail, as far as the
// report can know it: valid or not from the catalog, and — fed a log —
// indeterminate for a case the run declined as such.
func declaredOf(e conformance.CatalogEntry, class string, logFed bool) (string, error) {
	if class == classIndeterminate && e.DeclaredValid {
		return "", fmt.Errorf("the run log lists %s as indeterminate but the catalog declares it valid — the log is not from this suite", e.ID)
	}
	if e.DeclaredValid {
		return declaredValid, nil
	}
	if !logFed {
		return declaredNotValid, nil
	}
	if class == classIndeterminate {
		return declaredIndeterminate, nil
	}
	return declaredInvalid, nil
}

// compareClusters ranks the larger cluster first, and clusters of one size by
// class, declared validity, test set and reason, in that order.
func compareClusters(a, b cluster) int {
	return cmp.Or(
		cmp.Compare(len(b.ids), len(a.ids)),
		cmp.Compare(slices.Index(classOrder, a.class), slices.Index(classOrder, b.class)),
		cmp.Compare(slices.Index(declaredOrder, a.declared), slices.Index(declaredOrder, b.declared)),
		strings.Compare(a.testSet, b.testSet),
		strings.Compare(a.reason, b.reason),
	)
}

// ghIssue is the subset of one `gh issue list --json` row the reconciliation
// reads.
type ghIssue struct {
	Number int     `json:"number"`
	State  *string `json:"state"`
	Body   string  `json:"body"`
}

// errMissingState is what a feed with a row lacking `state` wraps: run
// prints the report without reconciliation and exits 2 on it.
var errMissingState = errors.New(`nothing was reconciled; reshape the input per docs/ROUTINES.md's "Survey input"`)

// readIssues reads the fed issue list, keeping the OPEN issues in number
// order. Empty stdin is the no-feed mode and returns nil with no error.
func readIssues(r io.Reader) ([]ghIssue, error) {
	var raw []ghIssue
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, fmt.Errorf("parsing issue JSON from stdin: %w", err)
	}
	for _, gi := range raw {
		if gi.State == nil {
			return nil, fmt.Errorf("fed issue #%d carries no `state`, so it cannot be told OPEN from CLOSED — %w", gi.Number, errMissingState)
		}
	}
	open := []ghIssue{}
	for _, gi := range raw {
		if !strings.EqualFold(*gi.State, "OPEN") {
			continue
		}
		open = append(open, gi)
	}
	slices.SortFunc(open, func(a, b ghIssue) int { return cmp.Compare(a.Number, b.Number) })
	return open, nil
}

// namesToken reports whether text names term as a whole token: bounded on
// each side by the text's edge or by a character no test-set name or rule ID
// carries, so `src-ct` names `src-ct.5` but not `src-ct-extends`.
func namesToken(text, term string) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], term)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(term)
		if (start == 0 || !tokenByte(text[start-1])) && (end == len(text) || !tokenByte(text[end])) {
			return true
		}
		i = start + 1
	}
}

// tokenByte reports whether b can continue a test-set name or rule ID.
func tokenByte(b byte) bool {
	return b == '-' || b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// naming returns the fed issues whose body names term, as `#N`.
func naming(issues []ghIssue, term string) []string {
	var out []string
	for _, gi := range issues {
		if namesToken(gi.Body, term) {
			out = append(out, fmt.Sprintf("#%d", gi.Number))
		}
	}
	return out
}

// render writes the report: the header and its caveat, the class-by-validity
// table, then every cluster, largest first, each with the open issues naming
// it.
func render(w io.Writer, p partitioned, file, logPath string, issues []ghIssue, feedErr error) {
	_, _ = fmt.Fprintf(w, "lanepartition: lane %s — %d banked fail(s) in %s, %d cluster(s)\n", p.lane, p.banked, file, len(p.chunks))
	_, _ = fmt.Fprintln(w, "  every figure is a part of the committed file's banked fails — the lane's score, which")
	_, _ = fmt.Fprintln(w, "  `go tool lanestatus` prints — and nothing a run did moves it.")
	if p.lane == instanceLane {
		_, _ = fmt.Fprintln(w, "  On this lane a banked fail the suite declares VALID flips only where the executor decides")
		_, _ = fmt.Fprintln(w, "  the document valid, which it does for an assessed subtree root alone, with content or")
		_, _ = fmt.Fprintln(w, "  without (#1841, #1855).")
	}
	renderLogNote(w, p, logPath)

	_, _ = fmt.Fprintln(w, "\n=== Banked fails by class and declared validity ===")
	columns := usedColumns(p)
	_, _ = fmt.Fprintf(w, "  %-20s", "")
	for _, d := range columns {
		_, _ = fmt.Fprintf(w, " %21s", d)
	}
	_, _ = fmt.Fprintf(w, " %8s\n", "total")
	for _, class := range classOrder {
		total := 0
		for _, d := range columns {
			total += p.cells[[2]string{class, d}]
		}
		if total == 0 {
			continue
		}
		_, _ = fmt.Fprintf(w, "  %-20s", class)
		for _, d := range columns {
			_, _ = fmt.Fprintf(w, " %21d", p.cells[[2]string{class, d}])
		}
		_, _ = fmt.Fprintf(w, " %8d\n", total)
	}

	_, _ = fmt.Fprintln(w, "\n=== "+clustersHeading(p)+" ===")
	renderFeedNote(w, issues, feedErr)
	for _, c := range p.chunks {
		_, _ = fmt.Fprintf(w, "  %6d  %s  %s  %s", len(c.ids), c.class, c.declared, c.testSet)
		if c.reason != "" {
			_, _ = fmt.Fprintf(w, "  %s", bracketed(c))
		}
		_, _ = fmt.Fprintln(w, renderIssues(c, issues, feedErr))
	}
}

// clustersHeading names the cluster columns, the refusal one only where the
// run named a refusal, so a log naming none renders as it did before #2008.
func clustersHeading(p partitioned) string {
	if p.refused() {
		return "Clusters, largest first: count  class  declared  test set  [charged] or <refusal>  — open issues naming it"
	}
	return "Clusters, largest first: count  class  declared  test set  [charged]  — open issues naming it"
}

// bracketed renders a cluster's reason: a decided case's charge in square
// brackets, a declined case's refusal in angle brackets, so neither reads as
// the other.
func bracketed(c cluster) string {
	if c.class == classDeclined {
		return "<" + c.reason + ">"
	}
	return "[" + c.reason + "]"
}

// renderLogNote says what the class column means for this report: without a
// log, one class and a two-way validity; with one, the census's classes and
// the two counts that measure how far the log's tree is from the file's.
func renderLogNote(w io.Writer, p partitioned, logPath string) {
	if !p.logFed {
		_, _ = fmt.Fprintln(w, "  No run log fed (-log): declines are not told from decided-and-wrong, and the catalog tells")
		_, _ = fmt.Fprintln(w, "  VALID only from invalid|indeterminate.")
		return
	}
	_, _ = fmt.Fprintf(w, "  Classes read from the GOXSD_DECLINES=1 census in %s. A banked fail in no census\n", logPath)
	_, _ = fmt.Fprintln(w, "  list is one that run passed or did not produce; with the census's own listed cases this")
	_, _ = fmt.Fprintf(w, "  file does not bank fail (%d), it measures how far the log's tree is from the file's.\n", p.extra)
	if p.charged() {
		_, _ = fmt.Fprintln(w, "  A decided case's [charge] is the rule xsderr.RuleOf read off the assembly's error;")
		_, _ = fmt.Fprintln(w, "  (accepted) charges nothing — the assembly succeeded — and (unruled) names no rule.")
	}
	if p.refused() {
		_, _ = fmt.Fprintln(w, "  A declined case's <refusal> names the exit at which the executor declined it, one of")
		_, _ = fmt.Fprintln(w, "  conformance/instance.go's refuse* tokens; one ending (#N) names the open issue owning it,")
		_, _ = fmt.Fprintln(w, "  and unevaluated:<rule>[,<rule>…] names the rules of the checks the walk did not perform.")
	}
}

// usedColumns is the declared validities this report can hold: the two-way
// reading without a log, the three-way one with it.
func usedColumns(p partitioned) []string {
	if !p.logFed {
		return []string{declaredValid, declaredNotValid}
	}
	return []string{declaredValid, declaredInvalid, declaredIndeterminate}
}

// renderFeedNote says, under the heading the reconciliation prints beside,
// whether there was anything to reconcile against.
func renderFeedNote(w io.Writer, issues []ghIssue, feedErr error) {
	if feedErr != nil {
		_, _ = fmt.Fprintf(w, "  (issue list discarded, nothing reconciled: %v)\n", feedErr)
		return
	}
	if issues == nil {
		_, _ = fmt.Fprintln(w, "  (no issue list on stdin: nothing reconciled)")
		return
	}
	_, _ = fmt.Fprintf(w, "  (reconciled against %d open issue(s): a body naming the test set or the charged rule)\n", len(issues))
}

// renderIssues is one cluster's reconciliation: the open issues naming its
// test set, then those naming its charged rule. A parenthesized charge is no
// rule and is not searched for, and neither is a refusal, which is no rule
// either.
func renderIssues(c cluster, issues []ghIssue, feedErr error) string {
	if feedErr != nil || issues == nil {
		return ""
	}
	parts := []string{}
	if found := naming(issues, c.testSet); len(found) > 0 {
		parts = append(parts, c.testSet+": "+strings.Join(found, " "))
	}
	if c.class == classDecided && c.reason != "" && !strings.HasPrefix(c.reason, "(") {
		if found := naming(issues, c.reason); len(found) > 0 {
			parts = append(parts, c.reason+": "+strings.Join(found, " "))
		}
	}
	if len(parts) == 0 {
		return "  — no open issue names it"
	}
	return "  — " + strings.Join(parts, "; ")
}
