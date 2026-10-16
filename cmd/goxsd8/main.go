package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// subcommands is the contract's whole subcommand vocabulary, in the order
// usage documents it. Diagnosis reads this one encoding rather than restating
// the names (STYLE D3/T4) — which name is built is answered where that name is
// dispatched, in run — and TestUsageCoversContract pins usage's own text to it.
// help and version are deliberately not members; doc.go states why.
var subcommands = []string{"parse", "validate", "gen"}

//go:generate go test -run ^TestDocRendersUsage$ -update .

// usage is the one hand-written source of the CLI's usage contract: the
// subcommand syntax, the common flags and the implementation status. Edit it
// here, then run go generate: doc.go's # Usage section is rendered from it
// (docUsageBlock, main_test.go) and TestDocRendersUsage fails until the two
// match. The rest of doc.go — the argument vocabulary, the diagnostic line
// shape and the CLI's relationship to the library, none of which the help
// path prints — is hand-written there.
const usage = `goxsd8 — XSD 1.1 schema compilation, instance validation, and code generation.

Usage (contract; subcommands land with their milestones):

  goxsd8 parse [-q] [-v] <schema.xsd>...
      Compile each schema argument and print its summary on stdout:
      the argument as it was spelled, then a block of lines indented
      two spaces and spelled "<label>: <value>". A namespace: line
      for each distinct namespace of the components the compilation
      declares (the argument document and every one it includes,
      imports, overrides or redefines) comes first, in
      first-appearance order and none when it declares nothing.
      Then one count per kind of declaration those documents make,
      all seven kinds always and always in this order: types,
      elements, attributes, attribute groups, model groups,
      notations, identity constraints. types counts the simple and
      the complex definitions together on the one line; model groups
      counts the top-level <xs:group> definitions; and the
      components: line closing the block is the sum of those seven,
      which no namespace line is counted into. Each argument is its
      own root document and its own run, in argument order — several
      arguments are several compilations, not one set.
      Exit 0 when every one compiles; 1 when any is rejected, its
      first error on stderr as <loc>: [<rule>] <message> (assembly
      stops there, so a rejected schema is one error line); 2 when an
      argument cannot be read, which is never a verdict about a
      schema. The exit code is the worst of those outcomes.
      Some errors have no rule to cite and print the producing
      component's own message instead, carrying what location they
      have inside the sentence rather than as the <loc>: prefix.
      Nothing is stamped on that message — not a rule ID, and not the
      goxsd8: <subcommand>: lead-in this binary's own diagnoses open
      with — so the line is that message as the component wrote it:
      no <loc>: prefix, no [rule], its location named inside the
      sentence. A document whose root is not <xs:schema> and a
      rejection in the s4s-grammar class (xsderr/doc.go) are
      examples, not the whole class: an I/O fault reading a document
      the argument REFERENCES prints the same way and is charged 1
      like a rejection, though nothing about the schema was decided.
      An <xs:include>, <xs:import>, <xs:override> or empty
      <xs:redefine> whose schemaLocation resolves to no document is
      named on stderr at its own position, with no rule ID and no
      change of exit code: the skip is legal, so the summary above is
      printed off a schema short of whatever that document declares.
      -q does not silence it. Such a directive of a REJECTED schema
      is named the same way, for the directives assembly reached
      before it stopped, in a line saying the assembly was rejected
      rather than compiled and saying nothing about whether the
      unread document had a part in that. The I/O fault above is not
      the legal skip: it is charged 1, though the faulting run names
      the directive on stderr too, before printing the fault.
      A bare <xs:import>, which names no document, is not reported.

  goxsd8 validate [-q] [-v] [-format xml|json|ber] [-no-hints] -schema <schema.xsd> [-schema <s2>]... <instance>...
      For example: goxsd8 validate -schema order.xsd order.xml order.json
      Assess instances against the compiled set; every schema needs
      its own -schema, and every positional argument is an instance.
      The -schema documents compose into ONE set — several of them are
      one compilation, not one each — and - names standard input as an
      instance, never as a schema.
      A -schema document's own unresolved directive is named on
      stderr the same way, once for the set, before any instance is
      assessed — and when the set does not compile, for whichever
      documents the assembly reached, so one document's rejection
      does not silence another's shortfall.
      Source format by extension (.xml, .json, .ber) or forced with
      -format xml|json|ber, matched case-sensitively and applying to
      every instance of the invocation (there is no per-instance
      spelling); an unrecognized token, and an instance whose
      extension names none of the three, are usage errors listing the
      values. xml and json are assessed today; ber is reserved, and
      an instance in it exits 2 saying so. - has no extension and
      needs -format, as in goxsd8 validate -format xml -schema
      s.xsd - < i.xml; without one it exits 2 saying so. An instance
      argument naming a directory exits 2 saying it is a directory.
      xsi:schemaLocation hints on the document element of an XML
      instance augment the schema set for that instance (resolved
      relative to the instance; disable with -no-hints). A hint the
      set will not compose with is the instance's own fault, not
      the schema set's: it is reported on stderr naming that
      instance, whose hints are then dropped, and it is assessed
      against the -schema documents alone. A JSON instance carries
      no hints, so -no-hints changes nothing for it.
      A JSON instance is mapped onto the infoset by validate/jsonsrc
      and assessed against the -schema documents; its violations and
      undecided checks print and exit as an XML instance's do, <loc>
      being jsonsrc's line and byte column. An input jsonsrc's
      mapping declines, malformed JSON, content after the top-level
      object and an I/O fault reading the instance once it is open
      all reach jsonsrc's one error channel, and the line names
      none of them: each prints <loc>: [jsonsrc] <message> on
      stdout, [jsonsrc] standing where [<rule>] stands and naming
      the source that raised the error, not a rule, and exits 4,
      where malformed XML and an I/O fault reading an XML instance
      exit 1. They share 4
      because the line names no class, and nothing tells a shape
      the mapping rejects from an arm it does not implement, so
      charging 1 would charge every decline as invalid.
      The JSON shape in brief: the top-level object has exactly one
      member, whose key names the root element. In an element's
      object a "$xmlns" member, {"prefix":"uri",...}, comes first
      and binds the prefixes of keys and QName content; a "$type"
      member, the element's xsi:type, follows it; both precede every
      other member, and neither is a member of the top-level object.
      An unprefixed key matches a declaration or attribute use by
      local name, and a prefixed key p:local by the namespace its
      "$xmlns" binds p to. A key names an attribute where the
      element's type has an attribute use of that name, and a child
      element where the type contains a child declaration of it; a
      key both carry is declined (exit 4). Attribute members are
      plain keys spelling the attribute's own name — kind, not
      "@kind", "_kind" or "-kind" — and lead the object, before
      every child member. A scalar is the element's simple content,
      null is xsi:nil="true", and an array is one child per item,
      for repeated children. A child key may repeat in one object:
      each member is one more child, in member order, which is how a
      model interleaving repeats (a, b, a) is spelled. A repeated
      attribute key, "$xmlns" or "$type" is declined (exit 4).
      Against a schema declaring {urn:o}order with an attribute kind
      and a repeated child item, this order.json exits 0:
        {"o:order":{"$xmlns":{"o":"urn:o"},"kind":"x","item":["a","b"]}}
      The full mapping, wildcards and every decline included, is
      go doc github.com/kud360/goxsd8/validate/jsonsrc, which needs
      the module source; without it, read validate/jsonsrc/doc.go at
      https://github.com/kud360/goxsd8.
      An XML instance's DOCTYPE is read for the unparsed entities
      an ENTITY value must name: the internal subset, including
      its internal parameter entities. The external DTD subset is
      never fetched, so a name declared only there is charged with
      a cvc-simple-type cause saying the DTD was not fully read, not
      that the name is undeclared.
      Exit 0 when no instance was charged a violation, none left a
      check undecided and none reached jsonsrc's error channel, 1
      invalid, 2 usage or I/O on an argument or on stdout, and 3 when
      the schema set does not compile.
      An I/O fault reading a document a -schema argument REFERENCES,
      through <xs:include>, <xs:import>, <xs:override> or
      <xs:redefine>, is charged 3: the schema set does not compile,
      though nothing about the schema was decided. Of the I/O faults
      reading a schema document, exit 2 covers only a -schema
      argument that cannot be read. The fault is not the legal skip
      of a schemaLocation resolving to no document, which moves no
      exit code, though the faulting run names the directive on
      stderr too, before printing the fault. 4 is an instance the
      assessment declined to decide: no violation charged, and a
      check it reached not performed, so the instance stands
      undecided against the rule that check answers to rather than
      clean; a JSON instance on jsonsrc's error channel is 4 too,
      with no assessment standing. The code is the worst outcome over
      the instances, by severity and not by number: undecided is less
      severe than invalid, so a run holding one of each — a JSON
      instance on the error channel beside an invalid one included —
      exits 1, and 4 answers a run where something was left undecided
      and nothing was worse.
      Every instance is assessed — the run never stops at the first
      invalid one — and each violation, and each check the assessment
      declined, prints one line on stdout:
      <loc>: [<rule>] <message>.

  goxsd8 gen -schema <schema.xsd> -out <dir> [-schema <s2> -out <d2>]... [-backend strict|native]
      Generate Go types; repeated -schema/-out pairs map schemas to
      output directories (multiple schemas, multiple output dirs).
      Exit 0 when every pair is generated; 1 when a schema is
      rejected, its first error on stderr as <loc>: [<rule>]
      <message>; 2 when an argument cannot be read, an output
      directory cannot be written, or a -schema stands without its
      -out. The exit code is the worst of those outcomes. Those are
      the codes gen answers with once M9 builds it: until then every
      gen invocation exits 2, reporting that gen is not yet
      implemented.

Flags common to all subcommands: -q (quiet), -v (debug logging via
slog to stderr; scope with GOXSD_DEBUG=parser,validate,codec). They
qualify a subcommand and follow its name — goxsd8 parse -q a.xsd, not
goxsd8 -q parse a.xsd. -q suppresses a subcommand's informational
output, which is parse's summary, and never a diagnosis: neither the
error lines above nor validate's violations and undecided checks are
silenced by it.

Implemented today: the help path, parse and validate. With no arguments,
or with -h, -help or --help in any argument position, goxsd8 prints this
usage to stdout and exits 0. goxsd8 parse compiles its arguments as above
and honours -q and -v; goxsd8 validate assesses XML and JSON instances as
above and honours -v, -q silencing nothing there because it writes no
informational output. GOXSD_DEBUG scopes -v for neither. Every other
invocation exits 2, reporting on stderr that the subcommand
gen is reserved but not yet implemented, that the name is not one of the
three, that a help flag is spelled with a value, as -help=<value>, that a
flag stands before the subcommand it qualifies, or that the first argument
is a flag and no subcommand was given.
`

// The exit codes the CLI answers with: a clean run, a rejected document, an
// argument list or a file this process could not work with at all, and — for
// validate alone — a schema set that does not compile and an instance the
// assessment declined to decide.
//
// Which fault each code names is per subcommand and stated in doc.go: parse
// charges exitInvalid for the schema it was asked to compile, where validate
// charges it for an instance and answers exitSchema for its schema set.
const (
	exitOK        = 0
	exitInvalid   = 1
	exitUsage     = 2
	exitSchema    = 3
	exitUndecided = 4
)

// exitSeverity orders the codes from least to most severe, so that an
// invocation over several arguments reports the worst outcome over its runs
// (worse). It is a separate encoding from the numbers themselves because the
// two orders differ: exitUndecided takes the next free integer, since the
// published meanings of 0 through 3 do not move (#720), while ranking below
// exitInvalid — a batch mixing an instance shown invalid with one merely left
// undecided answers with the invalid one, which is the verdict a gate acts on.
// exitSchema stays the most severe: it leaves every instance unassessed rather
// than any one of them decided.
var exitSeverity = []int{exitOK, exitUndecided, exitInvalid, exitUsage, exitSchema}

// worse returns the more severe of two exit codes. Both subcommands aggregate
// their argument list through it, which is the one encoding of the contract's
// "the exit code is the worst of those outcomes" (STYLE D3) — and taking the
// numeric maximum instead would answer a mixed batch with exitUndecided.
func worse(a, b int) int {
	if slices.Index(exitSeverity, a) > slices.Index(exitSeverity, b) {
		return a
	}
	return b
}

const (
	// helpRequestSpelling answers the flag-package spellings -h=…/-help=…,
	// which wantsHelp deliberately does not accept: the help vocabulary is the
	// three bare tokens and nothing else (doc.go), so this is a usage error
	// naming the spelling that would have worked.
	helpRequestSpelling = "a help request is spelled -h, -help or --help, with no value"

	// helpNotAFlagValue is that answer before any subcommand, where no flag set
	// has been reached to reject the spelling. It is what a valued help flag
	// earns in either pre-subcommand position, in place of a diagnosis telling
	// the user to move the flag after the subcommand: no position accepts the
	// valued form, so the move the message names fails too (#1189).
	helpNotAFlagValue = "goxsd8: " + helpRequestSpelling

	// helpNotAFlagValueFmt is the same answer from a subcommand's own flag set,
	// which fills the verb because the flag set that rejected the spelling is
	// its own.
	helpNotAFlagValueFmt = "goxsd8: %s: " + helpRequestSpelling

	// helpPointer is the remedy line under every usage error. It names the
	// binary's own help path, which resolves wherever the binary runs; a
	// `go doc <import path>` invocation needs the module tree and fails for
	// an installed binary (#870).
	helpPointer = "run `goxsd8 -help` for the usage contract, or see https://github.com/kud360/goxsd8"

	// notImplementedFmt answers a name the contract reserves and no milestone
	// has built yet. Reserved to those names: its promise that a planned
	// interface documents the name is false for anything else (#514).
	notImplementedFmt = "goxsd8: %s is not yet implemented"

	// unknownSubcommandFmt answers a name outside the vocabulary.
	unknownSubcommandFmt = "goxsd8: unknown subcommand %q"

	// noSubcommand answers a first argument shaped like a flag with no
	// subcommand anywhere after it. The common flags qualify a subcommand and
	// never stand alone, so -q is neither unknown nor unimplemented — it is a
	// subcommand short of an invocation.
	noSubcommand = "goxsd8: no subcommand given"

	// leadingFlagFmt answers a flag placed BEFORE the subcommand it qualifies.
	// The flags are the subcommand's own and follow its name (doc.go's
	// Argument vocabulary), so this invocation names a real subcommand and is
	// still a usage error — one that must not be reported as no subcommand at
	// all, which is what a scan of the first argument alone concluded (#472).
	// A valued help spelling never reaches it, because the rewrite it names
	// would fail (helpNotAFlagValue).
	leadingFlagFmt = "goxsd8: %s must follow the subcommand: goxsd8 %s %s ..."

	// flagAfterPositionalFmt answers a flag-shaped token standing after a
	// subcommand's first positional argument, which flag.FlagSet.Parse stopped
	// at and never read as a flag. Reporting it is what keeps `parse a.xsd -q`
	// from printing the summary -q asked to suppress and then failing to open
	// -q as a schema, and `validate a.xml -schema a.xsd` from reporting the
	// schema the user did name as missing (#1290).
	flagAfterPositionalFmt = "goxsd8: %[1]s: %[2]s stands after a positional argument, where it is no longer read as a flag: the flags of a subcommand come before its arguments, and a file genuinely named %[2]s is named ./%[2]s"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if wantsHelp(args) {
		// A truncated write means the user never got the help they asked for,
		// which is an IO fault and not a success.
		if _, err := fmt.Fprint(stdout, usage); err != nil {
			return exitUsage
		}
		return exitOK
	}
	// args is non-empty here: wantsHelp reports the bare invocation as a help
	// request.
	if args[0] == "parse" {
		return runParse(args[1:], stdout, stderr)
	}
	if args[0] == "validate" {
		return runValidate(args[1:], stdout, stderr)
	}
	return usageError(stderr, diagnose(args))
}

// diagnose names what is wrong with an invocation that neither requests help
// nor names a built subcommand. Matching is case-sensitive per Go CLI
// convention, so goxsd8 VALIDATE is an unknown subcommand rather than a
// reserved one.
//
// It reads past the first argument for one purpose only: a flag standing where
// a subcommand belongs is a different fault depending on whether a subcommand
// follows it, and reporting `goxsd8 -q parse a.xsd` as no subcommand at all
// contradicts the argument list (#472). args is non-empty.
//
// A valued help spelling is answered ahead of that split and without reading
// past it: no argument position accepts one, so there is no position to send
// the user to (#1189).
func diagnose(args []string) string {
	arg := args[0]
	if valuedHelpFlag(arg) {
		return helpNotAFlagValue
	}
	if strings.HasPrefix(arg, "-") {
		if name, ok := subcommandIn(args[1:]); ok {
			return fmt.Sprintf(leadingFlagFmt, arg, name, arg)
		}
		return noSubcommand
	}
	if !slices.Contains(subcommands, arg) {
		return fmt.Sprintf(unknownSubcommandFmt, arg)
	}
	return fmt.Sprintf(notImplementedFmt, arg)
}

// subcommandIn returns the first argument that is a contract subcommand name.
// It reads the same one encoding of the vocabulary dispatch does (STYLE D3).
func subcommandIn(args []string) (string, bool) {
	for _, a := range args {
		if slices.Contains(subcommands, a) {
			return a, true
		}
	}
	return "", false
}

// helpSpellings is the whole help-flag vocabulary the contract publishes, in
// the order doc.go names it. wantsHelp accepts these three bare tokens and
// valuedHelpFlag rejects the same three carrying a flag-package value, so the
// two read one encoding of the vocabulary rather than a list each (STYLE
// D3/T4).
var helpSpellings = []string{"-h", "-help", "--help"}

// wantsHelp accepts a help flag in any argument position, and only in the
// three bare spellings: the scan is deliberately positional-blind, gives --
// no end-of-options meaning, and does not parse -help=true (doc.go).
func wantsHelp(args []string) bool {
	if len(args) == 0 {
		return true
	}
	for _, a := range args {
		if slices.Contains(helpSpellings, a) {
			return true
		}
	}
	return false
}

// valuedHelpFlag reports whether arg is a help spelling carrying a
// flag-package value — -h=1, -help=true. The bare spellings never reach it:
// wantsHelp answers those before dispatch.
func valuedHelpFlag(arg string) bool {
	name, _, ok := strings.Cut(arg, "=")
	return ok && slices.Contains(helpSpellings, name)
}

// flagShapedIn returns the first of a subcommand's positional arguments that
// is spelled like a flag. flag.FlagSet.Parse stopped at the first positional,
// so such a token was never read as a flag and is a misplacement rather than a
// path (doc.go's argument vocabulary).
//
// The exact spelling - is standard input, an instance argument in its own
// right; a file genuinely named -q is reached as ./-q, the escape hatch the
// -schema - refusal already publishes for a file named -.
func flagShapedIn(args []string) (string, bool) {
	for _, a := range args {
		if strings.HasPrefix(a, "-") && a != stdinArg {
			return a, true
		}
	}
	return "", false
}
