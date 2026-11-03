package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// runParse implements `goxsd8 parse`: it compiles each schema argument and
// writes its summary to stdout, one run per argument in argument order.
// args excludes the subcommand name itself.
//
// The flags are parsed here rather than by run, which is what lets the
// contract state that the common flags qualify a subcommand and follow it
// (doc.go's Argument vocabulary). Nothing before the subcommand name reaches
// this function, and a help request never does: run answers one before it
// dispatches, so wiring a flag set here cannot take -h away from the help
// path.
func runParse(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("parse", flag.ContinueOnError)
	// Discarding the flag set's output silences its own usage rendering, which
	// would print a second, narrower contract next to doc.go's; helpPointer
	// names the real one instead.
	flags.SetOutput(io.Discard)
	quiet := flags.Bool("q", false, "suppress the summary")
	verbose := flags.Bool("v", false, "log the parser's assembly at debug level to stderr")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return subcommandUsageError(stderr, "parse", fmt.Sprintf(helpNotAFlagValueFmt, "parse"))
		}
		return subcommandUsageError(stderr, "parse", fmt.Sprintf("goxsd8: parse: %v", err))
	}
	locations := flags.Args()
	// Before anything is compiled: a flag written after the first schema
	// argument was never read as a flag, and compiling the arguments around it
	// would print a summary the misplaced flag may have asked to suppress
	// (#1290).
	if arg, ok := flagShapedIn(flags, args); ok {
		return subcommandUsageError(stderr, "parse", fmt.Sprintf(flagAfterPositionalFmt, "parse", arg))
	}
	if len(locations) == 0 {
		return subcommandUsageError(stderr, "parse", "goxsd8: parse: no schema given")
	}

	// A nil logger selects parser's silent default, so -v is the whole of the
	// injection and quiet is the default (STYLE L1).
	//
	// GAP(cmd): GOXSD_DEBUG does not scope -v. The contract (usage) names
	// GOXSD_DEBUG=parser,validate,codec and nothing reads the variable, so -v
	// is all or nothing. #1185 owns wiring the scoping or dropping the mention.
	var log *slog.Logger
	if *verbose {
		log = slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	code := exitOK
	for _, location := range locations {
		code = worse(code, parseOne(location, *quiet, log, stdout, stderr))
	}
	return code
}

// parseOne compiles one schema argument and reports its exit code. It never
// stops the run: every argument is compiled and reported, so one broken schema
// does not hide the next argument's verdict.
func parseOne(location string, quiet bool, log *slog.Logger, stdout, stderr io.Writer) int {
	schema, code := compileArgument("parse", location, log, stderr)
	if schema == nil {
		return code
	}
	if quiet {
		return exitOK
	}
	if _, err := io.WriteString(stdout, summarize(location, schema)); err != nil {
		// The summary the user asked for never arrived; that is an IO fault,
		// not a verdict about the schema.
		return subcommandFault(stderr, fmt.Sprintf("goxsd8: parse: writing the summary for %s: %v", location, err))
	}
	return exitOK
}

// compileArgument compiles one schema argument as its own root document, the
// way parse and gen both do, and returns the schema, or nil and the exit code
// its failure is charged: 2 for an argument that cannot be read, 1 for a
// rejection, whose first error it prints on stderr. verb is the subcommand the
// diagnoses are charged to. Every directive the assembly reached whose
// schemaLocation resolved to no document is named on stderr either way, ahead
// of the error on the rejected path.
func compileArgument(verb, location string, log *slog.Logger, stderr io.Writer) (*xsd.Schema, int) {
	root, err := rootLocation(location)
	if err != nil {
		return nil, subcommandFault(stderr, fmt.Sprintf("goxsd8: %s: %v", verb, err))
	}
	// The resolver is rooted at the filesystem root and the document is named
	// by its absolute path, so that an argument spelled absolutely or through
	// "../" resolves at all, a relative <xs:include> in it resolves against
	// its own directory, and the location an error cites is a path the reader
	// can open. loader.Dir's confinement to a subtree is the library default
	// for an embedding that treats a schemaLocation hint as attacker-supplied;
	// this process reads the documents its own user named, with that user's
	// privileges.
	schema, report, err := parser.ParseReport(root,
		parser.WithResolver(loader.Dir(filesystemRoot(root))),
		parser.WithLogger(log))
	if err != nil {
		// Above the verdict, and reported on this path for the reason
		// reportUnfollowed states: the shortfall is a fact about the assembly the
		// error came out of, and this report carries it (#1312). A root document
		// that could not be read returns an empty report, so that third error
		// shape still prints nothing here.
		reportUnfollowed(stderr, verb, assemblyRejected, report)
		// Exit 1 covers more than a schema verdict. rootLocation already opened
		// the ARGUMENT, so an argument that cannot be read is charged 2 and
		// never reaches here; an I/O fault reading a document that argument
		// REFERENCES does reach here and is charged 1 like a rejection, though
		// nothing about the schema was decided. It stays 1 (#1419): the
		// contract scopes exit 2 to the argument, and a Resolver error cannot
		// in general be told apart from a resolver declining to serve the
		// document. Errors reach stderr whatever -q says, so a script can grep
		// them.
		_, _ = fmt.Fprintln(stderr, violationLine(err))
		return nil, exitInvalid
	}
	// Before the caller writes anything and so ahead of parse's -q gate: the
	// summary is parse's informational output, and a directive the assembly
	// behind it could not follow is a diagnosis, which doc.go forbids -q to
	// silence.
	reportUnfollowed(stderr, verb, assemblyCompiled, report)
	return schema, exitOK
}

// rootLocation opens location to prove it is a readable file and returns its
// absolute path. Opening it here is what lets an unreadable argument be
// reported in the operating system's own words and charged exit 2, rather than
// reaching the parser and coming back as an assembly error indistinguishable
// in shape from a verdict about a schema's content.
func rootLocation(location string) (string, error) {
	f, err := os.Open(location)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }() // read-only handle: a close error cannot change what was read
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("open %s: is a directory", location)
	}
	return filepath.Abs(location)
}

// violationLine renders one schema error as the contract's
// "<loc>: [<rule>] <message>". The rendering is (*xsderr.Error).Error()'s own
// and is never re-derived here; the error is unwrapped to reach it, because
// the parser wraps some rejections in assembly context that would otherwise
// prefix the line.
//
// An error carrying no *xsderr.Error prints its own message instead. That is
// the whole membership rule, and the class it admits is open: nothing here
// invents a rule ID for a member, because an invented one would read as a
// citation (STYLE E2). Two kinds arrive, named as examples rather than
// enumerated. A rejection the spec catalogs no rule for — a document whose
// root is not <xs:schema>, or the s4s-grammar class (xsderr/doc.go) — is a
// real verdict with no rule ID to cite. An I/O or transport fault reading a
// REFERENCED document is no spec class at all: parser's fetch wraps every
// resolver error other than loader.ErrNotFound in plain assembly context and
// each hop back returns it unwrapped, so it lands here with nothing charged
// (parser/parse.go). A schemaLocation that merely resolves to no document is
// the ErrNotFound arm instead — a legal skip under src-include clause 2.4,
// reported by reportUnfollowed and never reaching this function.
func violationLine(err error) string {
	var e *xsderr.Error
	if errors.As(err, &e) {
		return e.Error()
	}
	return err.Error()
}

// The two assembly outcomes shortfallClause words a line for, named at the call
// sites so that neither reads as a bare true or false.
const (
	assemblyCompiled = true
	assemblyRejected = false
)

// reportUnfollowed names on stderr, one line per directive at its own
// position, every ·inter-schema-document reference· the assembly reached whose
// schemaLocation resolved to no document. verb is the subcommand the diagnosis
// is charged to, and compiled says whether that assembly went on to yield a
// schema.
//
// BOTH outcomes are reported (#1312). The shortfall is a fact about the
// assembly, which parser.AssemblyReport carries "as far as assembly got even
// when ParseReport returns an error", so reading it only on the success path
// withheld the line exactly where an operator holding an error needs to know
// the set was short. A rejected assembly names the directives it reached before
// stopping, which is all the report holds — one that failed on its root
// document holds none and prints nothing.
//
// The line carries no rule ID and moves no exit code, because this is not a
// violation: src-include clause 2.4 (§4.2.3) and src-import (§4.2.6.2) both
// make an unresolved schemaLocation legal to skip, and src-override (§4.2.5)
// clause 1 is vacuously satisfied by the same failure, so rendering it through
// violationLine's "<loc>: [<rule>] <message>" would publish a non-error as a
// spec charge (STYLE E2). What it buys is that a summary, or an assessment,
// answered off a SHORT assembly is distinguishable from one answered off a
// complete one — the fact nothing but -v carried before (#1260).
//
// Only parser.UnfollowedLocationUnresolved is named. A bare <import>
// (parser.UnfollowedNoLocation) names no document to have failed to reach:
// §4.2.6.2 makes it the spelling for "references into this namespace are
// expected", which another document of the set may supply, so reporting it
// would charge a complete set with a shortfall. The other two — a directive
// with no schemaLocation at all (parser.UnfollowedNoSchemaLocation) and a
// document that resolved but could not be read (parser.UnfollowedUnreadable) —
// do reach here now that the error path reports too, and stay unnamed: each
// arrives with the error that charges it, which says more than this line could.
func reportUnfollowed(stderr io.Writer, verb string, compiled bool, report *parser.AssemblyReport) {
	for _, u := range report.Unfollowed() {
		if u.Reason != parser.UnfollowedLocationUnresolved {
			continue
		}
		// A failed stderr write cannot change the outcome: the exit code is
		// settled either way, and stderr is the only channel this line has.
		_, _ = fmt.Fprintf(stderr, "goxsd8: %s: %s: this schemaLocation resolved to no document%s\n", verb, u.At, shortfallClause(compiled))
	}
}

// shortfallClause is what a line naming an unfollowed schemaLocation says about
// the assembly it came out of, which is not the same sentence on both outcomes:
// two of the compiled wording's claims hold only when the assembly compiled,
// and every caller reporting an unfollowed directive says it the same way
// (STYLE D3).
//
// That the skip was LEGAL: parser.UnfollowedLocationUnresolved also records the
// two unresolved locations that are faults — a non-empty <xs:redefine>'s
// (src-redefine clause 1) and a resolver that failed rather than reported
// absence — and each arrives with the verdict charging it, which a line calling
// it legal would contradict. And that the shortfall is all that is wrong: on
// the error path nothing in the report says whether the unread document had any
// part in the rejection, so the line says nothing of it either.
func shortfallClause(compiled bool) string {
	if compiled {
		return ", which is legal and skipped; the compiled schema is short of whatever that document declares"
	}
	return "; the rejected assembly is short of whatever that document declares"
}

// usageError reports a usage fault charged before any subcommand is known: the
// message, then the remedy, on stderr, and exit 2. Nothing reaches stdout,
// which carries verdicts alone. A fault charged inside a subcommand goes
// through subcommandUsageError instead.
func usageError(stderr io.Writer, msg string) int {
	// A failed stderr write cannot change the outcome — the exit code is 2
	// either way, and stderr is the only channel left to report it on.
	_, _ = fmt.Fprintf(stderr, "%s\n%s\n", msg, helpPointer)
	return exitUsage
}

// subcommandUsageError reports a diagnosis of the command line's shape charged
// inside the subcommand name — an unknown or misplaced flag, a missing
// argument, a bad flag value. It prints the message, then usage: and name's
// synopsis line, then the remedy, on stderr, and returns exit 2 (#3110). Every
// other exit-2 line a subcommand prints goes through subcommandFault instead
// (#3333).
func subcommandUsageError(stderr io.Writer, name, msg string) int {
	// A failed stderr write cannot change the outcome — the exit code is 2
	// either way, and stderr is the only channel left to report it on.
	_, _ = fmt.Fprintf(stderr, "%s\nusage: %s\n%s\n", msg, synopsis(name), helpPointer)
	return exitUsage
}

// subcommandFault reports an exit-2 fault charged inside a subcommand that the
// command line's shape did not cause — an I/O fault on an argument or on
// stdout, gen's refusal of an -out or a non-decline failure of Generate, a
// capability the build lacks. It prints the message alone, on stderr, and
// returns exit 2 (#3333). No synopsis or remedy follows it: retyping the
// command line does not cure it, and an argument's fault stops no run but its
// own, so a synopsis would land between the reports of the arguments around
// it.
func subcommandFault(stderr io.Writer, msg string) int {
	// A failed stderr write cannot change the outcome, for the reason
	// subcommandUsageError states.
	_, _ = fmt.Fprintln(stderr, msg)
	return exitUsage
}

// absentNamespace renders the ·absent· target namespace (§2.2), which a QName
// carries as an empty Space. It is spelled rather than left blank so that a
// no-namespace schema produces a namespace line like any other.
const absentNamespace = "(absent)"

// summarize renders the parse summary of one compiled schema: the namespaces
// its components are in, then the count of each §3.17.1 property, then their
// total. location is echoed as the argument spelled it.
//
// Every fact comes from *xsd.Schema's published enumeration accessors, each of
// which returns document order, and the buckets are visited in a fixed order,
// so the whole block is byte-identical across runs on identical input (STYLE
// D1/D2).
//
// The namespaces are the ones the compiled set's own components are in, in
// first-appearance order. That is not quite "the target namespace of each
// schema document": §3.17.1 gives the Schema component no {target namespace}
// property, so a document's targetNamespace is observable only through the
// components it declares, and one that declares nothing contributes no line.
func summarize(location string, s *xsd.Schema) string {
	buckets := []bucket{
		{"types", declaredNames(s.Types())},
		{"elements", declaredNames(s.Elements())},
		{"attributes", declaredNames(s.Attributes())},
		{"attribute groups", declaredNames(s.AttributeGroups())},
		{"model groups", declaredNames(s.ModelGroups())},
		{"notations", declaredNames(s.Notations())},
		{"identity constraints", declaredNames(s.IdentityConstraints())},
	}

	var b strings.Builder
	b.WriteString(location + "\n")
	for _, ns := range namespacesOf(buckets) {
		b.WriteString("  namespace: " + ns + "\n")
	}
	total := 0
	for _, bk := range buckets {
		total += len(bk.names)
		fmt.Fprintf(&b, "  %s: %d\n", bk.label, len(bk.names))
	}
	fmt.Fprintf(&b, "  components: %d\n", total)
	return b.String()
}

// bucket is one §3.17.1 property as the summary reports it: the label it
// prints under, and the names of the components a schema document declared
// into it.
type bucket struct {
	label string
	names []xsd.QName
}

// namespacesOf collects the distinct namespaces the named components are in,
// in first-appearance order over the buckets. The map decides membership only;
// the slice decides the order, so no output order comes from map iteration
// (STYLE D2).
//
// An anonymous component contributes nothing: its {name} is the zero QName, so
// it carries no target namespace to report — reading its empty Space as the
// ·absent· namespace would invent a namespace line for a schema that has none.
func namespacesOf(buckets []bucket) []string {
	var order []string
	seen := make(map[string]bool)
	for _, bk := range buckets {
		for _, name := range bk.names {
			if name == (xsd.QName{}) || seen[name.Space] {
				continue
			}
			seen[name.Space] = true
			order = append(order, namespaceLabel(name.Space))
		}
	}
	return order
}

// namespaceLabel renders one namespace name for the summary.
func namespaceLabel(space string) string {
	if space == "" {
		return absentNamespace
	}
	return space
}

// component is the shape every §3.17.1 property's members share: an expanded
// name and a source position. Both are on the eight component kinds already,
// so the summary reads all seven buckets it counts through one constraint
// rather than seven near-copies (STYLE T4).
type component interface {
	Name() xsd.QName
	Loc() xsderr.Loc
}

// declaredNames reduces one §3.17.1 property to the names of the components a
// SCHEMA DOCUMENT declared.
//
// The zero Loc is the discriminator, and xsd's package doc fixes its meaning:
// it "is the correct value for a component with no schema document behind it —
// parser.Produce's synthesized xs:anyType and §3.2.7 xsi: attribute
// declarations, the four xml: attribute declarations the parser supplies for an
// <import> of the §1.3.2 XML namespace it cannot fetch, and package builtin's
// seeded built-in datatypes, are the legitimate zero-Loc producers". Those are
// what every parse puts into {type definitions} and {attribute declarations}
// before reading a document, so counting them would report the same fifty-odd
// types and the same four xsi: attributes for every schema — and, for a schema
// that imports the XML namespace, four more it never wrote — burying what the
// schema itself declares.
func declaredNames[T component](components []T) []xsd.QName {
	names := make([]xsd.QName, 0, len(components))
	for _, c := range components {
		if c.Loc() == (xsderr.Loc{}) {
			continue
		}
		names = append(names, c.Name())
	}
	return names
}
