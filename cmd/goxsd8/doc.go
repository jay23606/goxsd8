// Command goxsd8 is the command-line interface: schema compilation,
// instance validation, and code generation.
//
// # Usage (contract; subcommands land with their milestones)
//
//	goxsd8 parse [-q] [-v] <schema.xsd>...
//	    Compile each schema argument and print its summary on stdout:
//	    the argument as it was spelled, then a block of lines indented
//	    two spaces and spelled "<label>: <value>". A namespace: line
//	    for each distinct namespace of the components the compilation
//	    declares (the argument document and every one it includes,
//	    imports, overrides or redefines) comes first, in
//	    first-appearance order and none when it declares nothing.
//	    Then one count per kind of declaration those documents make,
//	    all seven kinds always and always in this order: types,
//	    elements, attributes, attribute groups, model groups,
//	    notations, identity constraints. types counts the simple and
//	    the complex definitions together on the one line; model groups
//	    counts the top-level <xs:group> definitions; and the
//	    components: line closing the block is the sum of those seven,
//	    which no namespace line is counted into. Each argument is its
//	    own root document and its own run, in argument order — several
//	    arguments are several compilations, not one set.
//	    Exit 0 when every one compiles; 1 when any is rejected, its
//	    first error on stderr as <loc>: [<rule>] <message> (assembly
//	    stops there, so a rejected schema is one error line); 2 when an
//	    argument cannot be read, which is never a verdict about a
//	    schema. The exit code is the worst of those outcomes.
//	    Some errors have no rule to cite and print the producing
//	    component's own message instead, carrying what location they
//	    have inside the sentence rather than as the <loc>: prefix.
//	    Nothing is stamped on that message — not a rule ID, and not the
//	    goxsd8: <subcommand>: lead-in this binary's own diagnoses open
//	    with — so the line is that message as the component wrote it:
//	    no <loc>: prefix, no [rule], its location named inside the
//	    sentence. A document whose root is not <xs:schema> and a
//	    rejection in the s4s-grammar class (xsderr/doc.go) are
//	    examples, not the whole class: an I/O fault reading a document
//	    the argument REFERENCES prints the same way and is charged 1
//	    like a rejection, though nothing about the schema was decided.
//	    An <xs:include>, <xs:import>, <xs:override> or empty
//	    <xs:redefine> whose schemaLocation resolves to no document is
//	    named on stderr at its own position, with no rule ID and no
//	    change of exit code: the skip is legal, so the summary above is
//	    printed off a schema short of whatever that document declares.
//	    -q does not silence it. The I/O fault above is not that skip:
//	    it is charged 1, though the faulting run names the directive on
//	    stderr too, before printing the fault.
//	    A bare <xs:import>, which names no document, is not reported.
//	    A REJECTED schema is named the same
//	    way, for the directives assembly reached before it stopped, in
//	    a line saying the assembly was rejected rather than compiled
//	    and saying nothing about whether the unread document had a part
//	    in that.
//
//	goxsd8 validate -schema <schema.xsd> [-schema <s2>]... <instance>...
//	    Assess instances against the compiled set; every schema needs
//	    its own -schema, and every positional argument is an instance.
//	    The -schema documents compose into ONE set — several of them are
//	    one compilation, not one each — and - names standard input as an
//	    instance, never as a schema.
//	    A -schema document's own unresolved directive is named on
//	    stderr the same way, once for the set, before any instance is
//	    assessed — and when the set does not compile, for whichever
//	    documents the assembly reached, so one document's rejection
//	    does not silence another's shortfall.
//	    Source format by extension (.xml, .json, .ber) or forced with
//	    -format xml|json|ber, matched case-sensitively and applying to
//	    every instance of the invocation (there is no per-instance
//	    spelling); an unrecognized token, and an instance whose
//	    extension names none of the three, are usage errors listing the
//	    values. Only xml is assessed today: json and ber are reserved,
//	    and an instance in either exits 2 saying so. - has no
//	    extension and is read as xml without -format, as in
//	    goxsd8 validate -schema s.xsd - < i.xml; once a second format
//	    is assessed, - will need -format. An instance argument naming
//	    a directory exits 2 saying it is a directory.
//	    xsi:schemaLocation hints on the document element of an XML
//	    instance augment the schema set for that instance (resolved
//	    relative to the instance; disable with -no-hints). A hint the
//	    set will not compose with is the instance's own fault, not
//	    the schema set's: it is reported on stderr naming that
//	    instance, whose hints are then dropped, and it is assessed
//	    against the -schema documents alone.
//	    An XML instance's DOCTYPE is read for the unparsed entities
//	    an ENTITY value must name: the internal subset, including
//	    its internal parameter entities. The external DTD subset is
//	    never fetched, so a name declared only there is charged with
//	    a cvc-simple-type cause saying the DTD was not fully read, not
//	    that the name is undeclared.
//	    Exit 0 when no instance was charged a violation and none left
//	    a check undecided, 1 invalid, 2 usage or I/O on an argument or
//	    on stdout, and 3 when the schema set does not compile.
//	    An I/O fault reading a document a -schema argument REFERENCES,
//	    through <xs:include>, <xs:import>, <xs:override> or
//	    <xs:redefine>, is charged 3: the schema set does not compile,
//	    though nothing about the schema was decided. Of the I/O faults
//	    reading a schema document, exit 2 covers only a -schema
//	    argument that cannot be read. The fault is not the legal skip
//	    of a schemaLocation resolving to no document, which moves no
//	    exit code, though the faulting run names the directive on
//	    stderr too, before printing the fault. 4 is an instance the
//	    assessment declined to decide: no violation charged, and a
//	    check it reached not performed, so the instance stands
//	    undecided against the rule that check answers to rather than
//	    clean. The code is the worst outcome over the instances, by
//	    severity and not by number: undecided is less severe than
//	    invalid, so a run holding one of each exits 1, and 4 answers a
//	    run where something was left undecided and nothing was worse.
//	    Every instance is assessed — the run never stops at the first
//	    invalid one — and each violation, and each check the assessment
//	    declined, prints one line on stdout:
//	    <loc>: [<rule>] <message>.
//
//	goxsd8 gen -schema <schema.xsd> -out <dir> [-schema <s2> -out <d2>]... [-backend strict|native]
//	    Generate Go types; repeated -schema/-out pairs map schemas to
//	    output directories (multiple schemas, multiple output dirs).
//	    Exit 0 when every pair is generated; 1 when a schema is
//	    rejected, its first error on stderr as <loc>: [<rule>]
//	    <message>; 2 when an argument cannot be read, an output
//	    directory cannot be written, or a -schema stands without its
//	    -out. The exit code is the worst of those outcomes. Those are
//	    the codes gen answers with once M9 builds it: until then every
//	    gen invocation exits 2, reporting that gen is not yet
//	    implemented.
//
// Flags common to all subcommands: -q (quiet), -v (debug logging via
// slog to stderr; scope with GOXSD_DEBUG=parser,validate,codec). They
// qualify a subcommand and follow its name — goxsd8 parse -q a.xsd, not
// goxsd8 -q parse a.xsd. -q suppresses a subcommand's informational
// output, which is parse's summary, and never a diagnosis: neither the
// error lines above nor validate's violations and undecided checks are
// silenced by it.
//
// Implemented today: the help path, parse and validate. With no arguments,
// or with -h, -help or --help in any argument position, goxsd8 prints this
// usage to stdout and exits 0. goxsd8 parse compiles its arguments as above
// and honours -q and -v; goxsd8 validate assesses XML instances as above and
// honours -v, -q silencing nothing there because it writes no informational
// output. GOXSD_DEBUG scopes -v for neither. Every other invocation exits 2,
// reporting on stderr that gen is reserved but not yet implemented, that the
// name is not one of the three, that a help request carries a value, that a
// flag stands before the subcommand it qualifies, or that the first argument
// is a flag and no subcommand was given.
//
// # Argument vocabulary
//
// The subcommand vocabulary is exactly parse, validate and gen, matched
// case-sensitively: goxsd8 VALIDATE is an unknown subcommand, not a
// reserved one spelled loudly.
//
// The bareword help is not a contract name. A help request is spelled as a
// flag, so goxsd8 help is an unknown subcommand.
//
// The help flag is spelled -h, -help or --help, and those three bare tokens
// are the whole vocabulary. The flag-package forms -help=true and -h=1 are not
// help requests wherever they stand: before a subcommand one is a flag where a
// name belongs, and after one it is a flag whose value the subcommand does not
// accept. Both answers are usage errors naming the three spellings, and
// neither reports the flag as one to move: no argument position accepts a
// valued help spelling, so there is nowhere to move it to.
//
// Help is never scoped to a subcommand: a help flag in any argument position
// prints this whole contract and no other argument is examined, so both
// goxsd8 parse -h and goxsd8 -xyz -help print it and exit 0. That scan is
// positional-blind, which is also why -- carries no end-of-options meaning:
// goxsd8 -- -help is a help request.
//
// Everything else is positional. The subcommand name comes first and the
// common flags are its own, following it: goxsd8 parse -q a.xsd, never
// goxsd8 -q parse a.xsd, which is the usage error that says so rather than
// one claiming no subcommand was given. A subcommand's flags in turn precede
// its positional arguments, the flag package stopping at the first of them, so
// a flag-shaped token standing after one is not read as a flag at all: it is
// reported as the misplacement it is, exit 2, rather than taken for a path or
// silently dropped, so in goxsd8 parse a.xsd -q neither is -q a schema
// location nor is the summary it asked to suppress printed. The exact
// spelling - is never a misplaced flag — it is validate's standard-input
// instance argument — and a positional genuinely named -q is spelled ./-q, as
// a file named - is.
//
// A schema argument is a filesystem path, resolved to an absolute one before
// the document is read: an argument spelled absolutely or through "../" works,
// and an error cites a path the reader can open. Relative <xs:include>,
// <xs:import> and <xs:override> locations inside it resolve against that
// document's own directory. That resolution is confined to no subtree: a
// schema document may name any path the invoking user can read, so an
// <xs:include schemaLocation="../../../etc/passwd"> is served like any other.
// A directive whose schemaLocation resolves to no document is skipped rather
// than rejected because src-include clause 2.4 and src-import make that skip
// legal, which is why the usage block names it in a line and not as an error.
//
// validate composes its -schema documents into ONE schema set, each a root of
// one parser.ParseSet assembly read in its own target namespace, and an XML
// instance's schema location hints join that assembly as roots of their own,
// each followed as an <xs:import> of the namespace it pairs. Every
// cross-document rule an ordinary assembly enforces therefore holds over the
// set: two documents colliding on a name are sch-props-correct clause 2, and a
// reference no document of the set supplies is src-resolve. A set that does not
// compile exits 3, which is neither a verdict about an instance (1) nor an
// argument this process could not read (2), so a script tells "your schema is
// wrong" from "your data is wrong" by exit code alone.
//
// An instance the assessment DECLINED to decide exits 4: nothing was charged
// against it, and a check the assessment reached was not performed — for
// example an <xs:assert> whose {test} this engine does not evaluate, a {type
// table} that withheld its ·conditionally selected· type, a value whose simple
// type the value backend cannot decide, a fixed {value constraint} it cannot
// compare, a content model too wide for the matcher to decide, or an ID/IDREF
// or identity-constraint check over a value it could not read — so the
// document stands undecided against exactly the rules those checks answer to,
// which is what [validate.Result.Unevaluated] records (its own doc lists every
// kind, and the few declines it does not record) and validate's own doc.go
// calls not a pass. Each such check prints the "<loc>: [<rule>] <message>" line a
// violation prints, on stdout, so a script scanning for a rule ID reads them
// with the charges. 4 is the least severe outcome after 0 and the codes
// aggregate by severity rather than by number, so a run whose instances mix
// invalid with undecided exits 1: a gate acts on the verdict it has.
//
// validate assesses its instances in argument order. An instance argument
// spelled - is standard input, which has no extension to name a source format;
// without -format it is read as xml, the one format assessed while json and ber
// are reserved, and once a second format is assessed it will need -format
// again. An instance argument naming a directory, by any name and under any
// -format, is charged exit 2 as a directory before its extension is read.
// -schema - is not supported: a schema document's location is the base URI its
// own relative <xs:include>, <xs:import> and <xs:override> references resolve
// against, and standard input has none. That spelling is refused, exit 2,
// rather than opened, so a file which happens to be named - is never compiled
// as the schema set behind it; ./- is what names that file.
//
// validate follows an xsi:schemaLocation or xsi:noNamespaceSchemaLocation hint
// carried by the DOCUMENT ELEMENT of an XML instance, and no other element's.
// §4.3.2 clause 5 admits one on any element and makes its effect global to the
// assessment either way, and clause 3 obliges a processor to dereference none
// of them, so the scope above is the strategy this CLI publishes rather than a
// shortfall. Each hinted location is resolved against the instance's own path
// (clause 4) and joins that instance's schema set alone.
//
// A hint that instance's set will not compose with — one pairing a namespace
// with a document declaring another (src-import clause 3.1), a no-namespace
// hint naming a document that declares one (clause 3.2), or naming a document
// that is not well-formed — is a fault of the INSTANCE that carried it and
// never of the -schema set: clause 3 obliges a processor to dereference no
// hint at all, so the hints of that instance are reported unusable on stderr,
// naming it, and it is assessed against the -schema documents alone. Exit 3
// answers a -schema set that does not compile and nothing else.
//
// A hint naming a document that is NOT THERE is named on stderr too, against
// the instance that carried it and by the location it resolved to, carrying no
// rule ID and moving no exit code: src-import (§4.2.6.2) makes a
// schemaLocation that resolves to nothing legal to skip, so that hint's
// siblings still apply and the set still composes — short of whatever the
// document would have declared, which is the fact the line carries and clause 3
// calls "less than complete ·assessment· outcomes". When the instance's hints
// instead fail to compose with the -schema set, the same shortfall is named
// first, in a line saying the augmented set was rejected rather than compiled
// and saying nothing about whether the unread document had a part in that;
// the hints are then reported unusable and the instance falls back to the
// -schema set alone.
//
// -no-hints turns all of it off — clause 3's "Schema processors should provide
// an option to control whether they do so" — and turning it off is what makes
// an insufficient -schema set fail: a validation root the set declares nothing
// for is charged cvc-assess-elt (§3.3.4.6) instead of the run quietly
// succeeding on a schema document the instance itself named.
//
// gen's -backend strict|native names the value backend, builtin/strict or
// builtin/native, that the generated code is emitted against.
//
// There is no version entry point and none is planned before 1.0: run
// go version -m $(which goxsd8) for the module version of a tagged build.
// -v is not available for one, being already assigned to debug logging.
//
// The CLI is a thin shell over the library for the capabilities it runs
// today: schema compilation and XML instance assessment are reachable through
// parser, xsd, validate and validate/xmlsrc, which reads a DOCTYPE the way
// validate does, and the README shows both routes for them. The capabilities
// this page reserves have no library route either — validate/jsonsrc (M8),
// codegen (M9) with codec (M10) and validate/bersrc (M11) each export nothing
// yet, so for JSON instances, code generation and BER instances there is no
// second route to document until those milestones land. Error output is
// stable and line-oriented for scripting.
//
// # Diagnostic lines
//
// A diagnostic line is <loc>: [<rule>] <message>, on stderr for a schema error
// and on stdout for a violation or a declined check. <loc> is
// <file>:<line>:<col>, ? when unknown, and <rule> is the spec validation rule
// ID. validate renders its violations the way parse renders a schema error,
// the no-rule-to-cite exception included, which on validate's path is the line
// naming the source fault that stopped the walk. A decimal element holding
// "12,50" prints:
//
//	order.xml:3:3: [cvc-type] the ·initial value· of the element amount is not ·valid· with respect to its ·governing type definition· {http://www.w3.org/2001/XMLSchema}decimal, which cvc-type clause 3.1.3 requires as per String Valid (§3.16.4): [cvc-datatype-valid] decimal: "12,50" is not in the lexical space (decimal-lexical-representation, §3.3.3.1)
//
// <rule> is the rule CHARGED, and for the content of an element or attribute
// that is never cvc-datatype-valid: the charge is cvc-type (clause 3.1.3),
// cvc-attribute (clause 3) or cvc-complex-type (clause 1.2), each of which
// delegates through String Valid (§3.16.4) to Datatype Valid and carries that
// verdict as a WRAPPED cause — rendered into the message, as above, and
// reachable as an *xsderr.Error of its own through errors.As and
// xsderr.RuleOf. Key a dispatcher on the outer rule.
package main

// The # Usage section of the package comment above is generated:
// TestDocRendersUsage renders it from usage (main.go) and go generate rewrites
// it. Edit usage, never that section; the rest of the package comment is
// hand-written.
