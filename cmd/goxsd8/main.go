package main

import (
	"flag"
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

  Exit codes. A run over several arguments exits with its highest-ranked
  outcome: rank, not number, orders the codes.

  code  rank  meaning
  0     1     parse: every schema compiled. validate: every instance's
              assessment ran to its end, charging no violation and
              leaving no check undecided; that is not proven invalid,
              which is not proven valid. gen: every pair generated.
              Under -strict, in each of the three, no schemaLocation
              was named resolving to no document either.
  1     3     parse and gen: a schema rejected, an I/O fault reading a
              document it references included. validate: an instance
              invalid, not well-formed XML, its walk stopped by a fault,
              malformed JSON, or a fault reading a JSON instance once it
              is open.
  2     4     usage, or an I/O fault on an argument or on stdout, gen's
              refusal of an -out included: never a verdict about a
              schema or an instance. A line a subcommand prints about
              the command line's shape (an unknown or misplaced flag, a
              missing argument, a bad flag value, an instance whose
              name gives no format) is followed by usage: and that
              subcommand's synopsis line below, then by the line
              pointing to this help. Every other line a subcommand
              charges 2 (an I/O fault, gen's refusal of an -out, an
              instance in the ber format) is printed alone.
  3     none  validate only: the -schema set does not compile, an I/O
              fault reading a document it references included. Unranked:
              it answers before any instance is opened, so no instance's
              4, 1 or 2 is reached, and a 2 raised before compiling (a
              flag, a missing or misplaced argument, an instance whose
              name gives no format, a -schema that is - or cannot be
              read) answers instead of it.
  4     2     validate: an instance left undecided, by a check the
              assessment declined (validate below names its kinds)
              or by a jsonsrc decline. Nothing was
              proven invalid and something was not decided: not a pass,
              so a gate that must fail closed treats 4 as a failure.
              gen: a construct the generator declines, a generator
              refusal and not a schema fault, so nothing is written.
              Nothing was proven wrong and something was not done: not
              a pass either. And under -strict, in parse, validate and
              gen alike: a schemaLocation named on stderr resolving to
              no document, a directive's or an instance's hint's. That
              4 is this CLI's policy, not a spec verdict: nothing was
              proven wrong and a document was not read. It ranks as
              any 4 does, so a rejection's 1 or a fault's 2 answers
              over it, and validate's 3 answers alone.

  Precedence, by that rank: 2 over 1 over 4 over 0, with validate's 3
  answering alone, before any instance is opened.

  Streams: stdout carries this help text, parse's summary and each
  instance's report lines — its violations, its undecided checks, the
  line of the fault that stopped its walk, and the line of an instance
  not well-formed or unreadable once open — and stderr carries every
  other line: schema rejections (parse and gen 1, validate 3), usage
  errors and the I/O faults charged 2, gen's refusals and its exit-4
  declines, unresolved-directive and hint notes, and -v logging.

  Paths, in parse, validate and gen alike: a line about an argument
  itself names it as it was typed, relative or ./ or absolute, and
  standard input as -. That is the I/O fault opening an argument,
  gen's -schema <s> -out <d> pair, the instance a hint note names,
  and every line of an instance's report, its <loc> included. A
  location in a schema document is absolute whatever the argument's
  spelling: the location a schema rejection names (parse and gen 1,
  validate 3), a gen decline's, an unresolved-directive note's, and
  every schemaLocation or hint the run resolved. So an instance's
  lines repeat wherever the same command line runs, and a schema's
  lines carry the running host's own path.

  goxsd8 parse [-q] [-v] [-strict] <schema.xsd>...
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
      A rejected schema prints its first error on stderr as <loc>:
      [<rule>] <message> (assembly stops there, so a rejected schema
      is one error line). An argument that cannot be read stops no
      run but its own: the arguments after it are still compiled.
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
      skipped and named on stderr at its own position, by its kind
      and its schemaLocation value and an <xs:import> by its
      namespace too, with no rule ID and, without -strict, no change
      of exit code: no rule charges the skip — src-include and
      src-import make it legal for an <xs:import> and for an
      <xs:include> outside every <xs:override>'s target set,
      src-redefine clause 1 requires an <xs:redefine>'s
      schemaLocation to resolve only when it has children other than
      <xs:annotation>, so an empty one's skip breaks no rule though
      none calls it legal, no normative clause states it either way
      for an <xs:override>, and src-include's closing sentence covers
      an <xs:include> inside such a target set as written, but this
      processor reads §F.2's rewrite of that include into an
      <xs:override> as putting it under the override's silence
      instead — so the summary above is printed off a schema short
      of whatever that document declares. -q does not silence it.
      Under -strict the summary is printed all the same and the
      argument is charged 4. Such a directive of a REJECTED schema
      is named the same way, for the directives assembly reached
      before it stopped, in a line saying the assembly was rejected
      rather than compiled and saying nothing about whether the
      unread document had a part in that. The I/O fault above is not
      that skip: it is charged 1, though the faulting run names the
      directive on stderr too, before printing the fault.
      A bare <xs:import>, which names no document, is not reported.

  goxsd8 validate [-q] [-v] [-strict] [-format xml|json|ber] [-no-hints] -schema <schema.xsd> [-schema <s2>]... <instance>...
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
      does not silence another's shortfall. Under -strict that
      line charges the run 4 and every instance is still assessed,
      and an XML instance's xsi:schemaLocation hint (below) that
      resolves to no document, named on stderr against that
      instance, charges that instance 4 likewise.
      Source format by extension (.xml, .json, .ber) or forced with
      -format xml|json|ber, the extension and the token both matched
      case-sensitively (a.XML names no format, as -format XML names
      none), and -format applying to every instance of the
      invocation (there is no per-instance spelling); an
      unrecognized token, and an instance whose extension names none
      of the three, are usage errors listing the values. xml and
      json are assessed today; ber is reserved, and an instance in
      it exits 2 saying so. - has no extension and
      needs -format, as in goxsd8 validate -format xml -schema
      s.xsd - < i.xml; without one it exits 2 saying so. An instance
      whose name gives no format refuses the whole invocation before
      any schema is compiled: one line for each such instance, in
      argument order, then usage: and the synopsis once, and no
      instance is assessed. An instance argument naming a directory
      is not one of them whatever its name: it exits 2 saying it is
      a directory, and stops no instance but its own.
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
      being jsonsrc's line and byte column. A violation's message
      can end with a clause jsonsrc appends naming a member in JSON
      terms, such as ; JSON member "<key>" ... object expected; go
      doc github.com/kud360/goxsd8/validate/jsonsrc Validate lists
      each clause and the one charge it goes on. Malformed JSON,
      content after the top-level object, an I/O fault reading the
      instance once it is open and an input jsonsrc's mapping
      declines all reach jsonsrc's one error channel, and validate
      routes each by the class jsonsrc gives it, printing one line on
      stdout with [jsonsrc] standing where [<rule>] stands and naming
      the source that raised the error, not a rule. Malformed JSON
      and content after the top-level object print <loc>: [jsonsrc]
      <message> and exit 1, as malformed XML does; an I/O fault
      reading the instance prints the same line and exits 1, as an
      I/O fault reading an XML instance does. A decline prints
      <loc>: [jsonsrc] undecided: <message> and exits 4: the
      instance stands undecided, not invalid.
      Where -format forces xml or json on an instance whose
      extension names the other, a line saying it is not well-formed
      in the forced format keeps its token and exit code and ends
      with a clause naming the format forced and the one the
      extension names, ; -format F read <instance> as F, but its
      extension names G: drop -format, or pass -format G. Under
      -format json that is every malformed JSON line: malformed
      JSON, no JSON value and content after the top-level object
      alike. Under -format xml it is every [xml-wf] line,
      wherever in the document its fault lies. A line reporting a
      fault reading the instance gets no clause.
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
      for repeated children. A number's lexical form is its source
      digits, never a float64 round trip (1.0 stays 1.0, 1e2 stays
      1e2), and true and false are those literals, so a JSON boolean
      is an xs:boolean lexical form. A child key may repeat in one object:
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
      The -schema set is compiled before any instance is opened,
      since without a set there is nothing to assess an instance
      against; the exit-code table's 3 row says what answers before
      it and what it preempts. A 3's error prints on stderr once.
      An I/O fault reading a document a -schema argument REFERENCES,
      through <xs:include>, <xs:import>, <xs:override> or
      <xs:redefine>, is charged 3: the schema set does not compile,
      though nothing about the schema was decided. Of the I/O faults
      reading a schema document, exit 2 covers only a -schema
      argument that cannot be read. The fault is not the skip of a
      schemaLocation resolving to no document, which moves no exit
      code without -strict, though the faulting run names the
      directive on stderr too, before printing the fault.
      A declined check is one of these kinds:
        - assertions: an <xs:assert> {test}, or an assertions
          facet's, left unevaluated;
        - the governing type definition: an element's type
          withheld, by an <xs:alternative> {test} left unevaluated
          among other causes;
        - content: an element's value, its fixed value, or its
          children against its content model, left undecided;
        - attributes: an attribute's value against its declaration
          or use, or a defaulted attribute's value, left undecided;
        - the ID/IDREF table: an item that might hold an ID or
          IDREF and could not be read;
        - identity constraints: a selector or field path outside
          the evaluated subset, an unreadable field value, or an
          undecided key comparison.
      The type a declined <xs:alternative> withholds can leave the
      checks needing it undecided too, each on an undecided line of
      its own. A {test} this build evaluates is checked as usual and
      prints no undecided line. Whether a {test} is evaluated turns
      on the XPath forms it uses, so one assertion can be decided in
      one spelling and declined in another; go doc
      github.com/kud360/goxsd8/xpath lists which forms are evaluated,
      under # Forms each entry point evaluates. An XPath function
      this build does not evaluate is not a kind of its own but a
      cause inside assertions and the governing type definition, not
      necessarily only those. The authority on
      the kinds is go doc github.com/kud360/goxsd8/validate
      Unevaluated, which needs the module source; without it, read
      type Unevaluated in validate/validate.go at
      https://github.com/kud360/goxsd8.
      Every instance is assessed — the run never stops at the first
      invalid one, nor at an instance argument charged 2, whose 2
      stands beside the verdicts of the rest — and each violation
      prints one line on stdout, <loc>: [<rule>] <message>, and
      each check the assessment declined one line beside them,
      carrying the literal undecided: after the rule: <loc>: [<rule>]
      undecided: <message>. An instance's violations print first,
      then the line of the fault that stopped its walk if one did,
      then its undecided checks.

  goxsd8 gen [-q] [-v] [-strict] -schema <schema.xsd> -out <dir> [-schema <s2> -out <d2>]...
      For example: goxsd8 gen -schema order.xsd -out ./gen/order
      Generate Go types, one package per -schema/-out pair: the file
      xsd_gen.go in the -out directory, which is created with its
      parents when absent, declaring a package named by the
      directory's last path element (./gen/order/ is package order).
      Every -out lies in a Go module, which gives its package an
      import path: the module path of the nearest go.mod at or above
      the -out, then the -out's path below that go.mod's directory.
      Each -schema needs the -out after it and each -out the -schema
      before it; gen takes no positional argument. Each -schema is
      compiled alone, as a parse argument is: its path is resolved
      absolute before it is read. The package declares a type for
      every top-level element declaration of the -schema's composed
      set, an imported or included document's among them. A set
      declaring no top-level element gives a package with no type.
      The comments of the generated file name each document by its
      path relative to the -out directory, climbing with ../ out of
      it. A rejected -schema prints its first error on stderr as
      parse's does, the no-rule-to-cite lines and the
      unresolved-directive lines included. Under -strict an
      unresolved-directive line of a compiled -schema is a failure
      to compile charged 4, so nothing is written.
      Pairs whose -out lies under the same nearest go.mod share a
      document whose generated types agree, the declarations each
      would emit for it being the same text: the earliest such
      pair's package declares them, and each later pair's package
      imports it and aliases those types. Any other pair declares
      them itself. The later package imports the earlier by its
      import path, so deleting or moving the earlier pair's -out
      breaks the later one's build. Sharing is among the pairs of
      one invocation: running gen once per pair declares every type
      in its own package.
      Every -out is checked before any -schema is compiled. The
      generator's refusals of a pair exit 2 whatever any pair
      compiles or declines, each on one stderr line naming the pair
      as -schema <s> -out <d>, in argument order: among them a last
      path element that is not a Go package name, as with -out
      my-pkg or -out ., an -out repeating an earlier pair's, an -out
      that exists and is not a directory or whose path runs through
      a non-directory, an existing xsd_gen.go whose first line does not mark
      it generated, which gen never overwrites, an -out with no
      go.mod in it or any directory above it, one whose nearest
      go.mod cannot be read or declares no module, and an -out whose
      import path repeats an earlier pair's. A refusal stops no
      compilation: every pair is then compiled, a refused pair's
      -schema included, and each line a compilation prints follows
      the refusal lines, every failure to compile and every
      unresolved-directive line, with or without -strict; after a
      refusal nothing is written. Without one, every failure to
      compile is reported before anything is written; then either
      every pair is generated or none is,
      except that a fault while writing can leave some pairs written
      and others not. A fault creating or writing the output exits
      2, named against its pair the same way.
      Each construct the generator declines prints on one stderr
      line naming the pair, the construct and its location, in
      argument order and then document order, at most one for each
      top-level element declaration of the pair's -schema, and gen
      then exits 4; go doc github.com/kud360/goxsd8/codegen says
      what it accepts, which needs the module source; without it,
      read codegen/doc.go at https://github.com/kud360/goxsd8. gen
      writes nothing on stdout.

Flags common to all subcommands: -q (quiet), -v (debug logging via
slog to stderr; scope with GOXSD_DEBUG=parser,validate,codec),
-strict (exit 4, by the exit-code table's 4 row, when the run names a
schemaLocation resolving to no document; off, such a line moves no
exit code). They qualify a subcommand and follow its name — goxsd8
parse -q a.xsd, not goxsd8 -q parse a.xsd. -q suppresses a
subcommand's informational output, which is parse's summary, and
never a diagnosis: neither the error lines above nor validate's
violations and undecided checks are silenced by it.

Implemented today: the help path, parse, validate and gen. With no
arguments, or with -h, -help or --help in any argument position, goxsd8
prints this usage to stdout and exits 0, the whole of it even after a
subcommand name, because a per-subcommand copy could drift from this one.
goxsd8 parse compiles its arguments as above and honours -q, -v and
-strict; goxsd8 validate assesses XML and JSON instances and goxsd8 gen
generates as above, and both honour -v and -strict, -q silencing nothing
there because neither writes informational output. GOXSD_DEBUG scopes
-v for none of them. Every other invocation exits 2, reporting on
stderr that the name is not one of the three, that a help flag is
spelled with a value, as -help=<value>, that a flag stands before the
subcommand it qualifies, or that the first argument is a flag and no
subcommand was given.
`

// synopsisIndent is the indent of a subcommand's synopsis line in usage. The
// prose lines that open with goxsd8 and a name sit in column 0 (goxsd8 parse
// compiles ...), so anchoring on these exact two spaces is what keeps
// synopsisLines off them.
const synopsisIndent = "  "

// synopsisLines returns every line of usage spelled as name's synopsis line,
// its indent stripped. TestUsageCoversContract pins exactly one per name in
// subcommands.
func synopsisLines(name string) []string {
	prefix := synopsisIndent + "goxsd8 " + name + " "
	var lines []string
	for line := range strings.Lines(usage) {
		if strings.HasPrefix(line, prefix) {
			lines = append(lines, strings.TrimSuffix(strings.TrimPrefix(line, synopsisIndent), "\n"))
		}
	}
	return lines
}

// synopsis returns name's synopsis line as usage spells it, read from usage
// itself so that the line has one encoding (STYLE D3). A name with no such
// line, or with several, is a broken usage rather than an input fault, and
// panics rather than printing an empty or an arbitrary usage: line.
func synopsis(name string) string {
	lines := synopsisLines(name)
	if len(lines) != 1 {
		panic(fmt.Sprintf("goxsd8: usage spells %d synopsis lines for %s, want 1", len(lines), name))
	}
	return lines[0]
}

// The exit codes the CLI answers with: a clean run, a rejected document, an
// argument list or a file this process could not work with at all, a schema set
// that does not compile (validate alone), and an instance the assessment
// declined to decide, a schema the generator declined (validate and gen) or,
// under -strict, a schemaLocation named resolving to no document (skipCode).
//
// Which fault each code names is per subcommand and stated in doc.go: parse and
// gen charge exitInvalid for the schema they were asked to compile, where
// validate charges it for an instance and answers exitSchema for its schema
// set.
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
// exitSchema ranks last only so that every code has a rank: no caller passes it
// to worse, runValidate answering it before any instance is assessed, which is
// why usage's exit-code table gives it none.
var exitSeverity = []int{exitOK, exitUndecided, exitInvalid, exitUsage, exitSchema}

// worse returns the more severe of two exit codes. Every subcommand aggregates
// its argument list through it, which is the one encoding of the rank column
// of usage's exit-code table (STYLE D3) — and taking the numeric maximum
// instead would answer a mixed batch with exitUndecided.
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
	if args[0] == "gen" {
		return runGen(args[1:], stderr)
	}
	return usageError(stderr, diagnose(args))
}

// diagnose names what is wrong with an invocation that neither requests help
// nor names a subcommand: every name in subcommands is dispatched by run
// before diagnose is reached. Matching is case-sensitive per Go CLI
// convention, so goxsd8 VALIDATE is an unknown subcommand rather than
// validate.
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
	return fmt.Sprintf(unknownSubcommandFmt, arg)
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
// is spelled like a flag, once flags.Parse(args) has succeeded. Where
// flag.FlagSet.Parse stopped at the first positional, such a token was never
// read as a flag and is a misplacement rather than a path (doc.go's argument
// vocabulary). Where it stopped at a -- it consumed as the end of options,
// every positional is a path, -weird.xml and -q included, and none is
// returned (#3332).
//
// The exact spelling - is standard input, an instance argument in its own
// right; a file genuinely named -q is reached as ./-q, the escape hatch the
// -schema - refusal already publishes for a file named -.
func flagShapedIn(flags *flag.FlagSet, args []string) (string, bool) {
	if endedOptions(flags, args) {
		return "", false
	}
	for _, a := range flags.Args() {
		if strings.HasPrefix(a, "-") && a != stdinArg {
			return a, true
		}
	}
	return "", false
}

// endedOptions reports whether flags.Parse(args), which succeeded, consumed a
// -- as the end of options. It reads the tokens Parse consumed, because the
// operands flags.Args() returns are the same for a -- that is a flag's
// value: validate -schema -- a.xml -q leaves [a.xml -q] as
// validate -schema s.xsd -- a.xml -q does, and only the second -- ends
// options.
//
// It walks the tokens the flag package consumed as that package does: a
// flag spelled with = or naming a boolean flag is one token, any other flag
// takes the next token as its value, whatever that token is spelled. A -- met
// where a flag stands is the terminator, and is necessarily the last token
// consumed. Every name it looks up is one Parse accepted, so Lookup finds it.
func endedOptions(flags *flag.FlagSet, args []string) bool {
	consumed := args[:len(args)-flags.NArg()]
	for i := 0; i < len(consumed); i++ {
		if consumed[i] == "--" {
			return true
		}
		name := strings.TrimPrefix(strings.TrimPrefix(consumed[i], "-"), "-")
		if strings.Contains(name, "=") {
			continue
		}
		if b, ok := flags.Lookup(name).Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
			continue
		}
		i++
	}
	return false
}
