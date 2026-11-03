// Command gencensus measures codegen over the W3C suite: how many of its
// schema documents codegen.Generate emits, and, for the rest, the first
// construct each one declines, counted by construct and largest first. It is
// M9's measure, the way a lane score is a validator's: `/backlog` ranks
// codegen lifting slices by it, and a slice's `## Acceptance` states the
// documents it lifts (#3030).
//
// Every `.xsd` under the tree is taken as a root in sorted path order and
// assembled with parser.Parse through loader.Dir over its own directory, so an
// <include> or <import> reaches that directory's tree and nothing above it. A
// document that assembles is handed to codegen.Generate as one Target writing
// into a temporary directory, removed before the tool exits. A document that
// does not assemble is counted and set aside: the suite is full of schemas
// that are invalid on purpose.
//
// A decline is counted by the codegen.Construct of the *codegen.DeclineError
// that errors.As retrieves from the first element of Generate's result that
// holds one, and by nothing parsed from its text; a construct's line prints
// its Construct's String. A result holding no decline is counted on a line of
// its own, never under a construct; a failure that follows a document's
// declines in Generate's result is not counted.
//
// Usage:
//
//	go tool gencensus [-paths] [dir]
//
// dir defaults to the suite at [suiteroot.Default]. -paths adds each construct's
// documents, and each document whose Generate result holds no decline with
// that result's failures, one per line as errors.Join prints them. An absent or
// document-free tree is a supported mode, not a failure: the submodule is absent
// in a fresh container (#659), so the tool says so, names the command that
// initializes it, and exits 0. It exits 0 for a completed census whatever it
// finds, and 2 for an operational error.
package main

import (
	"bufio"
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/kud360/goxsd8/codegen"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/tools/internal/suiteroot"
)

// usage is printed for any argument the tool cannot act on.
const usage = "usage: gencensus [-paths] [dir]; -paths lists each construct's documents"

func main() {
	if err := run(os.Stdout, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "gencensus: %v\n", err)
		os.Exit(2)
	}
}

// run drives the census end to end: walk the tree, assemble and generate each
// document, render the report. It takes its destination and arguments so a
// test can drive the whole command without a subprocess.
func run(stdout io.Writer, args []string) (err error) {
	flags := flag.NewFlagSet("gencensus", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	withPaths := flags.Bool("paths", false, "list each construct's documents")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%s", err, usage)
	}
	if flags.NArg() > 1 {
		return errors.New(usage)
	}
	root := suiteroot.Default
	if flags.NArg() == 1 {
		root = flags.Arg(0)
	}
	docs, present, err := walkSchemas(root)
	if err != nil {
		return err
	}
	if !present {
		return suiteroot.PrintAbsent(stdout, "gencensus", root+" does not exist", "census")
	}
	if len(docs) == 0 {
		return suiteroot.PrintAbsent(stdout, "gencensus", root+" holds no .xsd document", "census")
	}
	out, err := os.MkdirTemp("", "gencensus-")
	if err != nil {
		return fmt.Errorf("creating the output directory: %w", err)
	}
	defer func() {
		if rerr := os.RemoveAll(out); rerr != nil {
			err = errors.Join(err, fmt.Errorf("removing the output directory %s: %w", out, rerr))
		}
	}()
	// Generate refuses a Dir outside a Go module (codegen.Target.Dir).
	if err := os.WriteFile(filepath.Join(out, "go.mod"), []byte("module gencensus\n"), 0o644); err != nil {
		return fmt.Errorf("writing the output directory's go.mod: %w", err)
	}
	rep := census(root, docs, out)
	return printReport(stdout, rep, *withPaths)
}

// tally is one declined construct's documents, in path order.
type tally struct {
	construct codegen.Construct
	docs      []string
}

// failure is a document whose Generate result holds no decline.
type failure struct {
	doc  string
	errs []*codegen.TargetError
}

// report is what the census found. The assembled and declined counts are not
// stored: each is a sum of what is (STYLE D3).
type report struct {
	root      string
	found     int
	generated int
	// tallies holds one entry per declined construct, in the order the walk
	// first met each.
	tallies []tally
	other   []failure
}

// declined is how many documents Generate declined.
func (r report) declined() int {
	n := 0
	for _, t := range r.tallies {
		n += len(t.docs)
	}
	return n
}

// assembled is how many documents parser.Parse assembled: each one Generate
// was called on.
func (r report) assembled() int { return r.generated + r.declined() + len(r.other) }

// census assembles and generates each of docs, slash paths relative to root,
// writing into out, a directory in a Go module. A document's outcome is the
// census's data, never an operational error, so census returns none.
func census(root string, docs []string, out string) report {
	rep := report{root: root, found: len(docs)}
	// index maps a construct to its position in rep.tallies, for lookup only;
	// nothing ranges over it (STYLE D2).
	index := map[codegen.Construct]int{}
	for _, doc := range docs {
		path := filepath.Join(root, filepath.FromSlash(doc))
		schema, err := parser.Parse(filepath.Base(path), parser.WithResolver(loader.Dir(filepath.Dir(path))))
		if err != nil {
			continue
		}
		errs := codegen.Generate([]codegen.Target{{Schema: schema, Dir: out, Package: "census", Name: doc}})
		if errs == nil {
			rep.generated++
			continue
		}
		var de *codegen.DeclineError
		for _, e := range errs {
			if errors.As(e, &de) {
				break
			}
		}
		if de == nil {
			rep.other = append(rep.other, failure{doc: doc, errs: errs})
			continue
		}
		i, seen := index[de.Construct()]
		if !seen {
			i = len(rep.tallies)
			index[de.Construct()] = i
			rep.tallies = append(rep.tallies, tally{construct: de.Construct()})
		}
		rep.tallies[i].docs = append(rep.tallies[i].docs, doc)
	}
	return rep
}

// ranked returns the tallies largest first, then by the construct's String.
func ranked(tallies []tally) []tally {
	r := slices.Clone(tallies)
	slices.SortStableFunc(r, func(a, b tally) int {
		if c := cmp.Compare(len(b.docs), len(a.docs)); c != 0 {
			return c
		}
		return strings.Compare(a.construct.String(), b.construct.String())
	})
	return r
}

// walkSchemas lists every `.xsd` file under root as a slash path relative to
// it, sorted. present is false when root does not exist, the fresh-container
// case, which is a mode rather than an error.
func walkSchemas(root string) (docs []string, present bool, err error) {
	info, err := os.Stat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reading census root %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, false, fmt.Errorf("census root %s is not a directory", root)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Dot entries are the submodule's git plumbing, never fixtures.
		if path != root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || filepath.Ext(path) != ".xsd" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relating %s to %s: %w", path, root, err)
		}
		docs = append(docs, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, true, fmt.Errorf("walking %s: %w", root, err)
	}
	sort.Strings(docs)
	return docs, true, nil
}

// caveats are the limits on reading any figure the census prints.
var caveats = []string{
	"One decline per document: the census counts the first decline in Generate's result, so a document counts once, under its first decline.",
	"Documents, not cases: every .xsd is taken as a root, once, however many test cases name it and whether or not any case uses it as one.",
	"The suite's schemas exercise the spec; they do not resemble an adopter's, so a count ranks a construct within this corpus only.",
	"No count is a yield: lifting a construct exposes each of its documents' next decline, so its count bounds from above the documents lifting it alone makes generate.",
}

// printReport renders the census: the counts, the declined constructs largest
// first, the caveats, and with withPaths each construct's documents and each
// document whose Generate result holds no decline. A bufio.Writer keeps its
// first write error and Flush returns it, so checking Flush checks every
// write.
func printReport(dst io.Writer, rep report, withPaths bool) error {
	w := bufio.NewWriter(dst)
	_, _ = fmt.Fprintf(w, "gencensus: %d schema document(s) under %s\n", rep.found, rep.root)
	_, _ = fmt.Fprintf(w, "  assembled: %d\n", rep.assembled())
	_, _ = fmt.Fprintf(w, "  generated: %d\n", rep.generated)
	_, _ = fmt.Fprintf(w, "  declined:  %d\n", rep.declined())
	_, _ = fmt.Fprintf(w, "  Generate error other than a decline: %d\n", len(rep.other))
	order := ranked(rep.tallies)
	_, _ = fmt.Fprintf(w, "\n=== First decline, by construct: %d construct(s) ===\n", len(order))
	for _, t := range order {
		_, _ = fmt.Fprintf(w, "  %6d  %s\n", len(t.docs), t.construct)
	}
	_, _ = fmt.Fprintln(w, "\n=== Caveats ===")
	for _, c := range caveats {
		_, _ = fmt.Fprintf(w, "  - %s\n", c)
	}
	if withPaths {
		printPaths(w, rep, order)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the report: %w", err)
	}
	return nil
}

// printPaths renders the -paths sections: each construct's documents, in
// order's order, then each document whose Generate result holds no decline,
// with its failures as the package doc's -paths states.
func printPaths(w *bufio.Writer, rep report, order []tally) {
	_, _ = fmt.Fprintln(w, "\n=== Documents, by construct ===")
	for _, t := range order {
		_, _ = fmt.Fprintf(w, "%s (%d):\n", t.construct, len(t.docs))
		for _, doc := range t.docs {
			_, _ = fmt.Fprintf(w, "  %s\n", doc)
		}
	}
	_, _ = fmt.Fprintf(w, "\n=== Generate errors other than a decline: %d ===\n", len(rep.other))
	for _, f := range rep.other {
		texts := make([]string, len(f.errs))
		for i, e := range f.errs {
			texts[i] = e.Error()
		}
		_, _ = fmt.Fprintf(w, "  %s — %s\n", f.doc, strings.Join(texts, "\n"))
	}
}
