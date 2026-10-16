package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/internal/schemaloc"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/parser/xmltree"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/jsonsrc"
	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// stdinArg is the instance argument naming standard input. It is an INSTANCE
// spelling alone: runValidate refuses it as a -schema value, because a schema
// document's location is the base URI its own relative <xs:include> and
// <xs:import> references resolve against (§4.3.2 clause 4), which standard
// input has none of. A file genuinely named - is still reached by ./-
// (doc.go's argument vocabulary).
const stdinArg = "-"

// stringList accumulates the values of a repeatable flag in argument order.
// It is what makes `-schema a.xsd -schema b.xsd` the contract's spelling of a
// two-document schema set, and `-schema a.xsd b.xsd` a one-document set
// followed by an instance argument (doc.go).
type stringList []string

// String renders the accumulated values for the flag package's own reporting.
// It is not a spelling the flag reads back: Set appends one whole value per
// occurrence and never splits one.
func (l *stringList) String() string { return strings.Join(*l, " ") }

// Set records one occurrence. Every string is a location, so nothing here
// fails.
func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// sourceFormat is one instance encoding of the contract's -format vocabulary.
type sourceFormat string

const (
	formatXML  sourceFormat = "xml"
	formatJSON sourceFormat = "json"
	formatBER  sourceFormat = "ber"
)

// sourceFormats is the whole -format vocabulary, in the order the contract
// spells it and a diagnosis lists it. Both the flag's validation and the
// extension mapping read this one encoding rather than restating the tokens
// (STYLE D3/T4).
var sourceFormats = []sourceFormat{formatXML, formatJSON, formatBER}

// formatVocabulary renders the valid -format values for a diagnosis, in
// sourceFormats order.
func formatVocabulary() string {
	spellings := make([]string, 0, len(sourceFormats))
	for _, f := range sourceFormats {
		spellings = append(spellings, string(f))
	}
	return strings.Join(spellings, ", ")
}

// runValidate implements `goxsd8 validate`: it compiles the -schema arguments
// into ONE schema set and assesses every positional argument against it,
// writing each violation, and each check the assessment declined, to stdout as
// the contract's "<loc>: [<rule>] <message>", and a JSON instance jsonsrc
// could not assess as jsonsrc's own error line (assessJSON). args excludes the
// subcommand name itself.
//
// Every instance is assessed, in argument order: a violation in one never
// stops the next, so a script gets the whole report from one run. The exit
// code is the worst outcome over the instances (main.go's exit codes), the way
// runParse's is over its schemas.
//
// The flags are parsed here rather than by run, for the reason runParse states.
func runValidate(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	// Discarding the flag set's output silences its own usage rendering, which
	// would print a second, narrower contract next to doc.go's; helpPointer
	// names the real one instead.
	flags.SetOutput(io.Discard)
	var schemas stringList
	flags.Var(&schemas, "schema", "a schema document of the set; repeat it for each")
	format := flags.String("format", "", "force the instance source format: xml, json or ber")
	noHints := flags.Bool("no-hints", false, "ignore the xsi:schemaLocation hints of XML instances")
	verbose := flags.Bool("v", false, "log assembly and assessment at debug level to stderr")
	// -q is defined because the contract makes it common to every subcommand,
	// and is read by nothing: validate writes no informational output, its
	// stdout carrying the report of violations and declined checks doc.go
	// forbids -q to silence.
	_ = flags.Bool("q", false, "accepted; validate has no informational output to suppress")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return usageError(stderr, fmt.Sprintf(helpNotAFlagValueFmt, "validate"))
		}
		return usageError(stderr, fmt.Sprintf("goxsd8: validate: %v", err))
	}
	forced, err := forcedFormat(*format)
	if err != nil {
		return usageError(stderr, fmt.Sprintf("goxsd8: validate: %v", err))
	}
	instances := flags.Args()
	// Ahead of the two missing-argument diagnoses, which are false of a command
	// line that carries the argument in the wrong place: in `validate a.xml
	// -schema a.xsd` the -schema the flag set never saw is the fault to report,
	// not the schema the user did name (#1290).
	if arg, ok := flagShapedIn(instances); ok {
		return usageError(stderr, fmt.Sprintf(flagAfterPositionalFmt, "validate", arg))
	}
	if len(schemas) == 0 {
		return usageError(stderr, "goxsd8: validate: no schema given")
	}
	if len(instances) == 0 {
		return usageError(stderr, "goxsd8: validate: no instance given")
	}

	// A nil logger selects the parser's and the engine's silent defaults, so -v
	// is the whole of the injection and quiet is the default (STYLE L1). -v is
	// all or nothing here too, for the reason parse.go's GAP(cmd) marker
	// records (#1185).
	var log *slog.Logger
	if *verbose {
		log = slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	roots := make([]parser.Root, 0, len(schemas))
	for _, location := range schemas {
		if location == stdinArg {
			// Refused here, before os.Open is reached, because otherwise the
			// contract's "not supported" holds only while no file named - exists
			// in the working directory: one that does would be compiled as the
			// schema set, exit 0, under a spelling that names no file.
			//
			// The refusal is this CLI's policy, not a spec rule. §4.3.2 clause 4
			// resolves a relative schemaLocation against its owner element's base
			// URI, which a stream has none of, and §4.2.3 and §4.2.6.2 make such
			// a reference degrade the assessment rather than reject the document.
			// Only the exact spelling is refused: ./- and any other path ending
			// in - opens through rootLocation like any argument.
			return usageError(stderr, fmt.Sprintf("goxsd8: validate: -schema %s: standard input is not a schema location, a schema document's location being the base URI its own relative <xs:include>, <xs:import> and <xs:override> references resolve against; name the document by path, or a file called %s by ./%s", stdinArg, stdinArg, stdinArg))
		}
		path, err := rootLocation(location)
		if err != nil {
			// An argument that cannot be read is a usage/IO fault, never a
			// verdict about a schema — rootLocation's own reasoning, and
			// parse's contract for the same argument shape.
			return usageError(stderr, fmt.Sprintf("goxsd8: validate: %v", err))
		}
		roots = append(roots, parser.RootAt(path))
	}

	// The backend the set is COMPILED with is the backend it is ASSESSED with:
	// validate.New requires them to be one value, or instance lexicals are read
	// in a value space no facet on the schema was ever checked against.
	backend := strict.New()
	base, report, err := compileSet(roots, backend, log)
	if err != nil {
		// The set's own shortfall stands above the verdict, on parseOne's terms:
		// the assembly discovers the -schema documents in argument order, so a
		// document whose directive resolved to nothing is named whether or not a
		// LATER one collides, or is not a schema document at all — a rejection
		// here used to silence the whole set's notes, including those of documents
		// that composed cleanly (#1312).
		reportUnfollowed(stderr, "validate", assemblyRejected, report)
		// Reported once, before any instance is read: with no schema set there
		// is no assessment to run, and one line beats the same line per
		// instance.
		_, _ = fmt.Fprintln(stderr, violationLine(err))
		return exitSchema
	}
	// Reported once for the same reason, and before any instance is assessed so
	// that the shortfall stands above the report it explains. Every entry names
	// a directive inside the set's own documents: the -schema arguments are
	// roots, which the report never lists among its directives.
	reportUnfollowed(stderr, "validate", assemblyCompiled, report)
	v, err := validate.New(base, backend, validate.WithLogger(log))
	if err != nil {
		return usageError(stderr, fmt.Sprintf("goxsd8: validate: %v", err))
	}

	job := validation{roots: roots, base: v, backend: backend, forced: forced, hints: !*noHints, log: log}
	code := exitOK
	for _, instance := range instances {
		code = worse(code, job.one(instance, stdout, stderr))
	}
	return code
}

// validation is one invocation's resolved configuration: the schema set every
// instance is assessed against, and the policy decisions the flags settled for
// all of them at once.
type validation struct {
	// roots are the -schema documents, in argument order. They are kept beside
	// base because an instance whose hints augment the set is compiled from
	// them again, with the instance's hints appended.
	roots []parser.Root
	// base assesses against the -schema documents alone.
	base *validate.Validator
	// backend is the value space roots were compiled in, and the one every
	// hint-augmented recompilation must reuse (validate.New).
	backend value.Backend
	// forced is the -format value, empty when the flag was not given and the
	// format is derived per instance from its extension.
	forced sourceFormat
	// hints reports whether an XML instance's xsi:schemaLocation hints augment
	// the set, which -no-hints turns off. A JSON instance carries none
	// (assessJSON).
	hints bool
	// log is the debug logger -v installed, nil when it was not given.
	log *slog.Logger
}

// one assesses a single instance argument and reports its exit code.
func (vn *validation) one(instance string, stdout, stderr io.Writer) int {
	if err := refuseDirectory(instance); err != nil {
		return usageError(stderr, fmt.Sprintf("goxsd8: validate: %v", err))
	}
	format, err := formatOf(instance, vn.forced)
	if err != nil {
		return usageError(stderr, fmt.Sprintf("goxsd8: validate: %v", err))
	}
	if format == formatBER {
		// A token the contract reserves and no milestone has built:
		// validate/bersrc carries a doc.go and no Validate, so the honest
		// answer is the usage code and this diagnosis, never a silent pass.
		return usageError(stderr, fmt.Sprintf("goxsd8: validate: %s: -format %s is reserved by the contract and not yet implemented; only %s and %s instances are assessed today", instance, format, formatXML, formatJSON))
	}
	src, closeSrc, err := openInstance(instance)
	if err != nil {
		return usageError(stderr, fmt.Sprintf("goxsd8: validate: %v", err))
	}
	defer closeSrc()

	if format == formatJSON {
		return vn.assessJSON(instance, src, stdout, stderr)
	}
	v, src, code := vn.validatorFor(instance, src, stderr)
	if code != exitOK {
		return code
	}

	result, err := xmlsrc.Validate(v, src, xmlsrc.WithURI(instance))
	if err != nil {
		// No assessment stands: the document is malformed before its document
		// element, holds none at all, or is malformed in what the walk left
		// unread, what follows the document element included. That is a
		// verdict about the instance in the same rendering a violation gets,
		// so it lands on stdout with them and counts as invalid — nothing in
		// the document was shown valid.
		return reportLines(stdout, stderr, instance, []string{violationLine(err)}, exitInvalid)
	}
	lines, code := assessmentLines(result)
	return reportLines(stdout, stderr, instance, lines, code)
}

// assessJSON assesses the JSON instance in src through jsonsrc.Validate,
// against the -schema set alone, and reports its exit code.
//
// It reads no schema location hints: validate/jsonsrc declines
// xsi:schemaLocation and xsi:noNamespaceSchemaLocation members permanently
// (#2627), so a JSON instance carries none, and -no-hints changes nothing for
// it. instanceHints, an XML tokenizer, is never run over JSON bytes.
//
// jsonsrc's error channel is one (nil, err): malformed JSON, content after the
// top-level object, an I/O fault mid-read (which the decoder reports as
// malformed JSON), and every input the mapping declines, which alone comes
// back as a *jsonsrc.DeclineError. jsonsrc's doc makes none of them a validity
// verdict, and nothing tells a shape the mapping rejects from an arm it does
// not implement, so the line — already "jsonsrc: <loc>: <message>", with no
// [<rule>] and no class named — lands on stdout with the report and is
// charged exitUndecided whatever the class: charging exitInvalid would charge
// every decline as invalid. Malformed XML is charged exitInvalid instead, on
// one's reasoning for xmlsrc's fault.
func (vn *validation) assessJSON(instance string, src io.Reader, stdout, stderr io.Writer) int {
	result, err := jsonsrc.Validate(vn.base, src, jsonsrc.WithURI(instance))
	if err != nil {
		return reportLines(stdout, stderr, instance, []string{violationLine(err)}, exitUndecided)
	}
	lines, code := assessmentLines(result)
	return reportLines(stdout, stderr, instance, lines, code)
}

// validatorFor returns the validator instance is assessed against and the
// reader to assess it from, or a non-exitOK code that is the instance's whole
// outcome.
//
// The reader coming back is not always the one going in: reading an instance's
// hints consumes its prefix, so what the assessment reads is the replay
// instanceHints returns. The hinted documents augment THIS instance's set and
// no other's — a hint is a property of the document that carries it, and the
// -schema arguments are the only set every instance shares. Hints that will
// not compose with that set are unusable rather than fatal: they are reported
// against the instance and dropped, and exitSchema stays the answer to a
// -schema set that does not compile. A hint whose location resolves to no
// document composes — it is legal to skip — and is reported too, because
// nothing else in the run says the set this instance was assessed against is
// short of a document the instance itself named (#1251).
func (vn *validation) validatorFor(instance string, src io.Reader, stderr io.Writer) (*validate.Validator, io.Reader, int) {
	if !vn.hints {
		return vn.base, src, exitOK
	}
	base, err := filepath.Abs(instance)
	if err != nil {
		return nil, nil, usageError(stderr, fmt.Sprintf("goxsd8: validate: resolving %s: %v", instance, err))
	}
	found, replay := instanceHints(instance, base, src)
	if len(found) == 0 {
		return vn.base, replay, exitOK
	}
	augmented, report, err := compileSet(append(slices.Clone(vn.roots), found...), vn.backend, vn.log)
	if err != nil {
		// The hints' own shortfall stands above the diagnosis, on
		// reportUnfollowed's terms: it is a fact about the assembly rather than a
		// rider on what that assembly decided (#1312). It says nothing of the
		// rejection, and the line below says nothing of it.
		reportUnfollowedHints(stderr, instance, assemblyRejected, report)
		// A set that stops compiling only once THIS instance's hints are folded
		// in is a fault of the instance, not of the -schema set the invocation
		// named: exitSchema would send a script to a schema set that compiles.
		// §4.3.2 clause 3 obliges a processor to dereference no hint at all, so
		// the honest degradation is the one a hint naming a MISSING document
		// already gets — the -schema set alone decides this instance, which
		// charges cvc-assess-elt where it declares nothing for the root — with
		// the hints reported as unusable rather than silently dropped. A fault
		// against a hint is charged at the hinted document itself (parser.HintAt),
		// so the line keeps its location: that is the file the reader must edit.
		_, _ = fmt.Fprintf(stderr, "goxsd8: validate: %s: ignoring its schema location hints, which do not compose with the -schema set: %s\n", instance, violationLine(err))
		return vn.base, replay, exitOK
	}
	// Before the assessment reads a byte, so that the shortfall stands above the
	// report it explains, exactly as runValidate places the -schema set's.
	reportUnfollowedHints(stderr, instance, assemblyCompiled, report)
	v, err := validate.New(augmented, vn.backend, validate.WithLogger(vn.log))
	if err != nil {
		return nil, nil, usageError(stderr, fmt.Sprintf("goxsd8: validate: %v", err))
	}
	return v, replay, exitOK
}

// reportUnfollowedHints names on stderr, one line per hint, every schema
// location hint of instance whose location resolved to no document — §4.3.2
// clause 3's "failure may cause less than complete ·assessment· outcomes",
// which src-import makes legal to skip and which nothing else in this run
// reports (#1251).
//
// It reads parser.AssemblyReport.UnfollowedRoots, which holds exactly the
// HintAt roots that named no document, each by the location it was built with —
// the absolute path instanceHints resolved. The -schema arguments are RootAt
// roots and never appear there, and a directive inside any document of the set
// is an Unfollowed entry instead, which runValidate already named off the
// -schema set's own assembly (#1260): so no shortfall is named twice, and none
// under a hint that resolved (#1346).
func reportUnfollowedHints(stderr io.Writer, instance string, compiled bool, report *parser.AssemblyReport) {
	for _, r := range report.UnfollowedRoots() {
		// A failed stderr write cannot change the outcome: the exit code is
		// settled either way, and stderr is the only channel this line has.
		_, _ = fmt.Fprintf(stderr, "goxsd8: validate: %s: the schema location hint %s resolved to no document%s\n", instance, r.Location(), shortfallClause(compiled))
	}
}

// assessmentLines renders one assessment's report, in document order — every
// violation it charged, then the source fault that stopped the walk if one
// did, then every check it REACHED and did not perform — together with the
// exit code those lines earn.
//
// A stopped walk is not a violation, but it is a verdict about the instance in
// the same shape — the document was not read to its end, so nothing past the
// fault was assessed — and it is rendered beside them rather than dropped. A
// check the assessment declined is a verdict too, and one no earlier reading
// of a Result could recover from the exit code: validate/doc.go's Contract
// makes an assessment that charged nothing and skipped something undecided
// against exactly the rules those records name, not a pass (#1223).
//
// The code comes back with the lines because it is the one thing their
// rendering does not carry: undecided records print exactly like charges, so
// nothing in the text tells a reader which kind of report this is.
func assessmentLines(result *validate.Result) ([]string, int) {
	violations := result.Violations()
	undecided := result.Unevaluated()
	lines := make([]string, 0, len(violations)+len(undecided)+1)
	for _, v := range violations {
		lines = append(lines, violationLine(v))
	}
	if result.Err() != nil {
		lines = append(lines, violationLine(result.Err()))
	}
	for _, u := range undecided {
		lines = append(lines, undecidedLine(u))
	}

	code := exitOK
	if len(undecided) > 0 {
		code = exitUndecided
	}
	if len(violations) > 0 || result.Err() != nil {
		code = exitInvalid
	}
	return lines, code
}

// undecidedLine renders one check the assessment declined in the rendering a
// violation gets, so that a script scanning stdout for "[<rule>]" sees the
// rules a document stands undecided against alongside the ones it was charged.
//
// It formats the three accessors rather than routing through violationLine,
// because a [validate.Unevaluated] deliberately does not satisfy error and
// must not be made to: it decides nothing about the document, and a consumer
// that could errors.Is it would be one join away from a false reject.
func undecidedLine(u validate.Unevaluated) string {
	return fmt.Sprintf("%s: [%s] %s", u.Loc(), u.Rule(), u.Msg())
}

// reportLines writes one instance's report to stdout and reports the exit code
// the assessment that produced it earned. An empty report writes nothing, so
// a script sees output only when there is something to see.
//
// A failed write is charged the usage/IO code, on parseOne's reasoning: the
// report the user asked for never arrived, which is a fault in this run rather
// than a verdict about the document.
func reportLines(stdout, stderr io.Writer, instance string, lines []string, code int) int {
	if len(lines) == 0 {
		return code
	}
	if _, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n"); err != nil {
		return usageError(stderr, fmt.Sprintf("goxsd8: validate: writing the report of %s: %v", instance, err))
	}
	return code
}

// refuseDirectory charges an instance argument naming a directory as
// unreadable, in rootLocation's words. It answers before formatOf, so a
// directory is diagnosed as a directory whatever its name's extension and
// whatever -format forced, rather than as an extension problem or as malformed
// XML.
//
// The stat follows symbolic links, so a link to a directory is refused too; a
// FIFO or a device is not a directory and passes on to be read. stdinArg is
// never stat'ed: it names standard input, not a file. A stat that fails decides
// nothing here: openInstance reports that argument's fault in the operating
// system's own words, after formatOf has had its say on the extension.
func refuseDirectory(instance string) error {
	if instance == stdinArg {
		return nil
	}
	if info, err := os.Stat(instance); err == nil && info.IsDir() {
		return fmt.Errorf("open %s: is a directory", instance)
	}
	return nil
}

// openInstance opens one instance argument for reading and returns the reader
// together with the close its caller owes. stdinArg names standard input,
// which this process does not own and therefore does not close.
func openInstance(instance string) (io.Reader, func(), error) {
	if instance == stdinArg {
		return os.Stdin, func() {}, nil
	}
	f, err := os.Open(instance)
	if err != nil {
		return nil, nil, err
	}
	// A close error on a read-only handle cannot change what was read.
	return f, func() { _ = f.Close() }, nil
}

// forcedFormat reads the -format value. The empty string is the flag's absence,
// not a token, and leaves the format to each instance's own extension.
//
// Matching is case-sensitive, as the contract states: -format XML is an
// unrecognized token and not a loudly spelled one.
func forcedFormat(token string) (sourceFormat, error) {
	if token == "" {
		return "", nil
	}
	if slices.Contains(sourceFormats, sourceFormat(token)) {
		return sourceFormat(token), nil
	}
	return "", fmt.Errorf("-format %q is not a source format; the values are %s", token, formatVocabulary())
}

// formatOf reports the source format of one instance argument: the -format
// value where the flag was given, and otherwise the format its extension names.
//
// stdinArg has no extension, so without -format it is a usage error: xml and
// json are both assessed, and either reading of standard input could be meant
// (#2403). Any other argument whose extension names no source format is the
// same usage error rather than a guess, so that no input is ever read in a
// format nothing in the invocation asked for.
func formatOf(instance string, forced sourceFormat) (sourceFormat, error) {
	if forced != "" {
		return forced, nil
	}
	if instance == stdinArg {
		return "", fmt.Errorf("%s: standard input has no extension to name a source format; pass -format %s", instance, formatVocabulary())
	}
	ext := filepath.Ext(instance)
	for _, f := range sourceFormats {
		if ext == "."+string(f) {
			return f, nil
		}
	}
	return "", fmt.Errorf("%s: the extension %q names no source format; pass -format %s", instance, ext, formatVocabulary())
}

// compileSet assembles roots into ONE schema set and returns it finalized,
// together with the assembly's report — every document it read, every
// ·inter-schema-document reference· it could not follow to one, and every hint
// root that named no document, which is how a caller observes a set that
// composed short of a document it named.
//
// It is parser.ParseSet over the set, so every cross-document rule the parser
// enforces — src-import clause 3, sch-props-correct clause 2, src-resolve at
// finalize — is enforced over the CLI's set too.
func compileSet(roots []parser.Root, backend value.Backend, log *slog.Logger) (*xsd.Schema, *parser.AssemblyReport, error) {
	// Every root location is an absolute path, so the filesystem resolver is
	// rooted at the filesystem root: parseOne's reasoning, that this process
	// reads the documents its own user named, with that user's privileges. roots
	// is never empty — runValidate requires a -schema — so the first names the
	// volume the whole set resolves under, which on a platform with more than
	// one is the volume they must share.
	return parser.ParseSet(roots,
		parser.WithResolver(loader.Dir(filesystemRoot(roots[0].Location()))),
		parser.WithBackend(backend),
		parser.WithLogger(log))
}

// filesystemRoot is the resolver root under which an absolute path resolves:
// the filesystem root of the volume location names. Every location the CLI
// hands a resolver is absolute, so this is the root that serves all of them.
func filesystemRoot(location string) string {
	return filepath.VolumeName(location) + string(filepath.Separator)
}

// instanceHints reads the schema location hints off the DOCUMENT ELEMENT of
// the XML instance in r, resolved against base, and returns them with a reader
// replaying every byte the scan consumed.
//
// The replay is what lets a hint be read from standard input, which cannot be
// reopened, and it is used for a file too so that both are read exactly once
// and identically.
//
// Reading only the document element is a policy, not a shortfall: §4.3.2
// clause 5 admits a hint on any element and makes its effect global either way,
// clause 3 requires no processor to dereference any of them, and doc.go states
// the scope this one follows.
func instanceHints(uri, base string, r io.Reader) ([]parser.Root, io.Reader) {
	var consumed bytes.Buffer
	reader := xmltree.NewReader(uri, io.TeeReader(r, &consumed))
	replay := func() io.Reader { return io.MultiReader(&consumed, r) }
	for {
		node, err := reader.Token()
		if err != nil {
			// The prefix is malformed, or ends before a document element:
			// there are no hints to read, and no diagnosis to make here. The
			// assessment reads the same bytes off the replay reader and charges
			// the fault at its own location under its own rule, so reporting it
			// here too would be two lines for one fault.
			return nil, replay()
		}
		start, ok := node.(*xmltree.StartElement)
		if !ok {
			continue
		}
		return hintsOf(start, base), replay()
	}
}

// hintsOf reads the §2.7 schema location hints off one element, in the order
// its attributes carry them (STYLE D1/D2), resolving each location against
// base through internal/schemaloc — the same resolution the parser gives an
// <xs:include>, so that a hint and a directive naming one document agree on
// which document that is.
//
// xsi:schemaLocation pairs a namespace with a location; xsi:noNamespaceSchema-
// Location names a location whose document has no target namespace, which is
// parser.HintAt's absent namespace "".
//
// xsi:schemaLocation's type is a list of xs:anyURI (§3.2.7.3), so its items
// are delimited on XML white space alone (xmlSpaceFields).
// xsi:noNamespaceSchemaLocation's is ONE xs:anyURI (§3.2.7.4), not a list: its
// ·actual value· is the whiteSpace = collapse normalization (Datatypes §4.3.6)
// and names one location however many spaces it holds. xs:anyURI admits every
// XML Char, so neither a U+00A0 inside a schemaLocation item nor a #x20 inside
// a noNamespaceSchemaLocation value splits the location it is part of.
func hintsOf(start *xmltree.StartElement, base string) []parser.Root {
	var hints []parser.Root
	for _, a := range start.Attributes() {
		if a.Name().Space() != xsd.XMLSchemaInstanceNS {
			continue
		}
		fields := xmlSpaceFields(a.Value())
		switch a.Name().Local() {
		case "noNamespaceSchemaLocation":
			// An empty value is dropped, as it was when this attribute was
			// read as a list, rather than resolved to base, the instance itself:
			// §4.3.2 clause 3 obliges no processor to dereference a hint.
			if len(fields) > 0 {
				location := strings.Join(fields, " ") // the collapsed ·actual value·
				hints = append(hints, parser.HintAt("", schemaloc.Resolve(base, location)))
			}
		case "schemaLocation":
			// An odd trailing member pairs with nothing and names no document,
			// so it is dropped rather than resolved against an empty namespace.
			for i := 0; i+1 < len(fields); i += 2 {
				hints = append(hints, parser.HintAt(fields[i], schemaloc.Resolve(base, fields[i+1])))
			}
		}
	}
	return hints
}

// xmlSpaceFields splits s into the maximal runs of characters outside the XML
// S production (xml.md [3]: #x20, #x9, #xD, #xA), the only characters a list
// value is delimited on (cvc-datatype-valid, Datatypes §4.1.4 clause 2.2) or
// whiteSpace = collapse normalizes (§4.3.6). strings.Fields is not this split:
// it also breaks on U+00A0, U+2028 and the other Unicode spaces.
func xmlSpaceFields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(" \t\r\n", r) })
}
