// Command goxsd8 is the command-line interface: schema compilation,
// instance validation, and code generation.
//
// # Usage (contract; subcommands land with their milestones)
//
//	Exit codes. A run over several arguments exits with its highest-ranked
//	outcome: rank, not number, orders the codes.
//
//	code  rank  meaning
//	0     1     parse: every schema compiled. validate: every instance's
//	            assessment ran to its end, charging no violation and
//	            leaving no check undecided; that is not proven invalid,
//	            which is not proven valid. gen: every pair generated.
//	            Under -strict, in each of the three, no schemaLocation
//	            was named resolving to no document either.
//	1     3     parse and gen: a schema rejected, an I/O fault reading a
//	            document it references included. validate: an instance
//	            invalid, not well-formed XML, its walk stopped by a fault,
//	            malformed JSON, or a fault reading a JSON instance once it
//	            is open.
//	2     4     usage, or an I/O fault on an argument or on stdout, gen's
//	            refusal of an -out included: never a verdict about a
//	            schema or an instance. A line a subcommand prints about
//	            the command line's shape (an unknown or misplaced flag, a
//	            missing argument, a bad flag value, an instance whose
//	            name gives no format) is followed by usage: and that
//	            subcommand's synopsis line below, then by the line
//	            pointing to this help. Every other line a subcommand
//	            charges 2 (an I/O fault, gen's refusal of an -out, an
//	            instance in the ber format) is printed alone.
//	3     none  validate only: the -schema set does not compile, an I/O
//	            fault reading a document it references included. Unranked:
//	            it answers before any instance is opened, so no instance's
//	            4, 1 or 2 is reached, and a 2 raised before compiling (a
//	            flag, a missing or misplaced argument, a -schema that is
//	            - or cannot be read) answers instead of it.
//	4     2     validate: an instance left undecided, by a check the
//	            assessment declined (validate below names its kinds)
//	            or by a jsonsrc decline. Nothing was
//	            proven invalid and something was not decided: not a pass,
//	            so a gate that must fail closed treats 4 as a failure.
//	            gen: a construct the generator declines, a generator
//	            refusal and not a schema fault, so nothing is written.
//	            Nothing was proven wrong and something was not done: not
//	            a pass either. And under -strict, in parse, validate and
//	            gen alike: a schemaLocation named on stderr resolving to
//	            no document, a directive's or an instance's hint's. That
//	            4 is this CLI's policy, not a spec verdict: nothing was
//	            proven wrong and a document was not read. It ranks as
//	            any 4 does, so a rejection's 1 or a fault's 2 answers
//	            over it, and validate's 3 answers alone.
//
//	Precedence, by that rank: 2 over 1 over 4 over 0, with validate's 3
//	answering alone, before any instance is opened.
//
//	Streams: stdout carries this help text, parse's summary and each
//	instance's report lines — its violations, its undecided checks, the
//	line of the fault that stopped its walk, and the line of an instance
//	not well-formed or unreadable once open — and stderr carries every
//	other line: schema rejections (parse and gen 1, validate 3), usage
//	errors and the I/O faults charged 2, gen's refusals and its exit-4
//	declines, unresolved-directive and hint notes, and -v logging.
//
//	Paths, in parse, validate and gen alike: a line about an argument
//	itself names it as it was typed, relative or ./ or absolute, and
//	standard input as -. That is the I/O fault opening an argument,
//	gen's -schema <s> -out <d> pair, the instance a hint note names,
//	and every line of an instance's report, its <loc> included. A
//	location in a schema document is absolute whatever the argument's
//	spelling: the location a schema rejection names (parse and gen 1,
//	validate 3), a gen decline's, an unresolved-directive note's, and
//	every schemaLocation or hint the run resolved. So an instance's
//	lines repeat wherever the same command line runs, and a schema's
//	lines carry the running host's own path.
//
//	goxsd8 parse [-q] [-v] [-strict] <schema.xsd>...
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
//	    A rejected schema prints its first error on stderr as <loc>:
//	    [<rule>] <message> (assembly stops there, so a rejected schema
//	    is one error line). An argument that cannot be read stops no
//	    run but its own: the arguments after it are still compiled.
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
//	    skipped and named on stderr at its own position, by its kind
//	    and its schemaLocation value and an <xs:import> by its
//	    namespace too, with no rule ID and, without -strict, no change
//	    of exit code: no rule charges the skip — src-include and
//	    src-import make it legal for an <xs:import> and for an
//	    <xs:include> outside every <xs:override>'s target set, and the
//	    spec states nothing either way for an <xs:override>, an empty
//	    <xs:redefine>, or an <xs:include> inside such a target set,
//	    which §F.2 rewrites into an <xs:override> — so the summary
//	    above is printed off a schema short of whatever that document
//	    declares. -q does not silence it.
//	    Under -strict the summary is printed all the same and the
//	    argument is charged 4. Such a directive of a REJECTED schema
//	    is named the same way, for the directives assembly reached
//	    before it stopped, in a line saying the assembly was rejected
//	    rather than compiled and saying nothing about whether the
//	    unread document had a part in that. The I/O fault above is not
//	    that skip: it is charged 1, though the faulting run names the
//	    directive on stderr too, before printing the fault.
//	    A bare <xs:import>, which names no document, is not reported.
//
//	goxsd8 validate [-q] [-v] [-strict] [-format xml|json|ber] [-no-hints] -schema <schema.xsd> [-schema <s2>]... <instance>...
//	    For example: goxsd8 validate -schema order.xsd order.xml order.json
//	    Assess instances against the compiled set; every schema needs
//	    its own -schema, and every positional argument is an instance.
//	    The -schema documents compose into ONE set — several of them are
//	    one compilation, not one each — and - names standard input as an
//	    instance, never as a schema.
//	    A -schema document's own unresolved directive is named on
//	    stderr the same way, once for the set, before any instance is
//	    assessed — and when the set does not compile, for whichever
//	    documents the assembly reached, so one document's rejection
//	    does not silence another's shortfall. Under -strict that
//	    line charges the run 4 and every instance is still assessed,
//	    and an XML instance's xsi:schemaLocation hint (below) that
//	    resolves to no document, named on stderr against that
//	    instance, charges that instance 4 likewise.
//	    Source format by extension (.xml, .json, .ber) or forced with
//	    -format xml|json|ber, the extension and the token both matched
//	    case-sensitively (a.XML names no format, as -format XML names
//	    none), and -format applying to every instance of the
//	    invocation (there is no per-instance spelling); an
//	    unrecognized token, and an instance whose extension names none
//	    of the three, are usage errors listing the values. xml and
//	    json are assessed today; ber is reserved, and an instance in
//	    it exits 2 saying so. - has no extension and
//	    needs -format, as in goxsd8 validate -format xml -schema
//	    s.xsd - < i.xml; without one it exits 2 saying so. An instance
//	    argument naming a directory exits 2 saying it is a directory.
//	    xsi:schemaLocation hints on the document element of an XML
//	    instance augment the schema set for that instance (resolved
//	    relative to the instance; disable with -no-hints). A hint the
//	    set will not compose with is the instance's own fault, not
//	    the schema set's: it is reported on stderr naming that
//	    instance, whose hints are then dropped, and it is assessed
//	    against the -schema documents alone. A JSON instance carries
//	    no hints, so -no-hints changes nothing for it.
//	    A JSON instance is mapped onto the infoset by validate/jsonsrc
//	    and assessed against the -schema documents; its violations and
//	    undecided checks print and exit as an XML instance's do, <loc>
//	    being jsonsrc's line and byte column. A violation's message
//	    can end with a clause jsonsrc appends naming a member in JSON
//	    terms, such as ; JSON member "<key>" ... object expected; go
//	    doc github.com/kud360/goxsd8/validate/jsonsrc Validate lists
//	    each clause and the one charge it goes on. Malformed JSON,
//	    content after the top-level object, an I/O fault reading the
//	    instance once it is open and an input jsonsrc's mapping
//	    declines all reach jsonsrc's one error channel, and validate
//	    routes each by the class jsonsrc gives it, printing one line on
//	    stdout with [jsonsrc] standing where [<rule>] stands and naming
//	    the source that raised the error, not a rule. Malformed JSON
//	    and content after the top-level object print <loc>: [jsonsrc]
//	    <message> and exit 1, as malformed XML does; an I/O fault
//	    reading the instance prints the same line and exits 1, as an
//	    I/O fault reading an XML instance does. A decline prints
//	    <loc>: [jsonsrc] undecided: <message> and exits 4: the
//	    instance stands undecided, not invalid.
//	    Where -format forces xml or json on an instance whose
//	    extension names the other, a line saying it is not well-formed
//	    in the forced format keeps its token and exit code and ends
//	    with a clause naming the format forced and the one the
//	    extension names, ; -format F read <instance> as F, but its
//	    extension names G: drop -format, or pass -format G. Under
//	    -format json that is every malformed JSON line: malformed
//	    JSON, no JSON value and content after the top-level object
//	    alike. Under -format xml it is every [xml-wf] line,
//	    wherever in the document its fault lies. A line reporting a
//	    fault reading the instance gets no clause.
//	    The JSON shape in brief: the top-level object has exactly one
//	    member, whose key names the root element. In an element's
//	    object a "$xmlns" member, {"prefix":"uri",...}, comes first
//	    and binds the prefixes of keys and QName content; a "$type"
//	    member, the element's xsi:type, follows it; both precede every
//	    other member, and neither is a member of the top-level object.
//	    An unprefixed key matches a declaration or attribute use by
//	    local name, and a prefixed key p:local by the namespace its
//	    "$xmlns" binds p to. A key names an attribute where the
//	    element's type has an attribute use of that name, and a child
//	    element where the type contains a child declaration of it; a
//	    key both carry is declined (exit 4). Attribute members are
//	    plain keys spelling the attribute's own name — kind, not
//	    "@kind", "_kind" or "-kind" — and lead the object, before
//	    every child member. A scalar is the element's simple content,
//	    null is xsi:nil="true", and an array is one child per item,
//	    for repeated children. A number's lexical form is its source
//	    digits, never a float64 round trip (1.0 stays 1.0, 1e2 stays
//	    1e2), and true and false are those literals, so a JSON boolean
//	    is an xs:boolean lexical form. A child key may repeat in one object:
//	    each member is one more child, in member order, which is how a
//	    model interleaving repeats (a, b, a) is spelled. A repeated
//	    attribute key, "$xmlns" or "$type" is declined (exit 4).
//	    Against a schema declaring {urn:o}order with an attribute kind
//	    and a repeated child item, this order.json exits 0:
//	      {"o:order":{"$xmlns":{"o":"urn:o"},"kind":"x","item":["a","b"]}}
//	    The full mapping, wildcards and every decline included, is
//	    go doc github.com/kud360/goxsd8/validate/jsonsrc, which needs
//	    the module source; without it, read validate/jsonsrc/doc.go at
//	    https://github.com/kud360/goxsd8.
//	    An XML instance's DOCTYPE is read for the unparsed entities
//	    an ENTITY value must name: the internal subset, including
//	    its internal parameter entities. The external DTD subset is
//	    never fetched, so a name declared only there is charged with
//	    a cvc-simple-type cause saying the DTD was not fully read, not
//	    that the name is undeclared.
//	    The -schema set is compiled before any instance is opened,
//	    since without a set there is nothing to assess an instance
//	    against; the exit-code table's 3 row says what answers before
//	    it and what it preempts. A 3's error prints on stderr once.
//	    An I/O fault reading a document a -schema argument REFERENCES,
//	    through <xs:include>, <xs:import>, <xs:override> or
//	    <xs:redefine>, is charged 3: the schema set does not compile,
//	    though nothing about the schema was decided. Of the I/O faults
//	    reading a schema document, exit 2 covers only a -schema
//	    argument that cannot be read. The fault is not the skip of a
//	    schemaLocation resolving to no document, which moves no exit
//	    code without -strict, though the faulting run names the
//	    directive on stderr too, before printing the fault.
//	    A declined check is one of these kinds:
//	      - assertions: an <xs:assert> {test}, or an assertions
//	        facet's, left unevaluated;
//	      - the governing type definition: an element's type
//	        withheld, by an <xs:alternative> {test} left unevaluated
//	        among other causes;
//	      - content: an element's value, its fixed value, or its
//	        children against its content model, left undecided;
//	      - attributes: an attribute's value against its declaration
//	        or use, or a defaulted attribute's value, left undecided;
//	      - the ID/IDREF table: an item that might hold an ID or
//	        IDREF and could not be read;
//	      - identity constraints: a selector or field path outside
//	        the evaluated subset, an unreadable field value, or an
//	        undecided key comparison.
//	    The type a declined <xs:alternative> withholds can leave the
//	    checks needing it undecided too, each on an undecided line of
//	    its own. A {test} this build evaluates is checked as usual and
//	    prints no undecided line. Whether a {test} is evaluated turns
//	    on the XPath forms it uses, so one assertion can be decided in
//	    one spelling and declined in another; go doc
//	    github.com/kud360/goxsd8/xpath lists which forms are evaluated,
//	    under # Forms each entry point evaluates. An XPath function
//	    this build does not evaluate is not a kind of its own but a
//	    cause inside assertions and the governing type definition, not
//	    necessarily only those. The authority on
//	    the kinds is go doc github.com/kud360/goxsd8/validate
//	    Unevaluated, which needs the module source; without it, read
//	    type Unevaluated in validate/validate.go at
//	    https://github.com/kud360/goxsd8.
//	    Every instance is assessed — the run never stops at the first
//	    invalid one, nor at an instance argument charged 2, whose 2
//	    stands beside the verdicts of the rest — and each violation
//	    prints one line on stdout, <loc>: [<rule>] <message>, and
//	    each check the assessment declined one line beside them,
//	    carrying the literal undecided: after the rule: <loc>: [<rule>]
//	    undecided: <message>. An instance's violations print first,
//	    then the line of the fault that stopped its walk if one did,
//	    then its undecided checks.
//
//	goxsd8 gen [-q] [-v] [-strict] -schema <schema.xsd> -out <dir> [-schema <s2> -out <d2>]...
//	    For example: goxsd8 gen -schema order.xsd -out ./gen/order
//	    Generate Go types, one package per -schema/-out pair: the file
//	    xsd_gen.go in the -out directory, which is created with its
//	    parents when absent, declaring a package named by the
//	    directory's last path element (./gen/order/ is package order).
//	    Every -out lies in a Go module, which gives its package an
//	    import path: the module path of the nearest go.mod at or above
//	    the -out, then the -out's path below that go.mod's directory.
//	    Each -schema needs the -out after it and each -out the -schema
//	    before it; gen takes no positional argument. Each -schema is
//	    compiled alone, as a parse argument is: its path is resolved
//	    absolute before it is read. The package declares a type for
//	    every top-level element declaration of the -schema's composed
//	    set, an imported or included document's among them. A set
//	    declaring no top-level element gives a package with no type.
//	    The comments of the generated file name each document by its
//	    path relative to the -out directory, climbing with ../ out of
//	    it. A rejected -schema prints its first error on stderr as
//	    parse's does, the no-rule-to-cite lines and the
//	    unresolved-directive lines included. Under -strict an
//	    unresolved-directive line of a compiled -schema is a failure
//	    to compile charged 4, so nothing is written.
//	    Pairs whose -out lies under the same nearest go.mod share a
//	    document whose generated types agree, the declarations each
//	    would emit for it being the same text: the earliest such
//	    pair's package declares them, and each later pair's package
//	    imports it and aliases those types. Any other pair declares
//	    them itself. The later package imports the earlier by its
//	    import path, so deleting or moving the earlier pair's -out
//	    breaks the later one's build. Sharing is among the pairs of
//	    one invocation: running gen once per pair declares every type
//	    in its own package.
//	    Every -out is checked before any -schema is compiled. The
//	    generator's refusals of a pair exit 2 whatever any pair
//	    compiles or declines, each on one stderr line naming the pair
//	    as -schema <s> -out <d>, in argument order: among them a last
//	    path element that is not a Go package name, as with -out
//	    my-pkg or -out ., an -out repeating an earlier pair's, an -out
//	    that exists and is not a directory, an existing xsd_gen.go
//	    whose first line does not mark it generated, which gen never
//	    overwrites, an -out with no go.mod in it or any directory above
//	    it, one whose nearest go.mod cannot be read or declares no
//	    module, and an -out whose import path repeats an earlier
//	    pair's. Then every pair is compiled, and every failure to
//	    compile reported after those lines, before anything is
//	    written; then either every pair is generated or none is,
//	    except that a fault while writing can leave some pairs written
//	    and others not. A fault creating or writing the output exits
//	    2, named against its pair the same way.
//	    Each construct the generator declines prints on one stderr
//	    line naming the pair, the construct and its location, in
//	    argument order and then document order, at most one for each
//	    top-level element declaration of the pair's -schema, and gen
//	    then exits 4; go doc github.com/kud360/goxsd8/codegen says
//	    what it accepts, which needs the module source; without it,
//	    read codegen/doc.go at https://github.com/kud360/goxsd8. gen
//	    writes nothing on stdout.
//
// Flags common to all subcommands: -q (quiet), -v (debug logging via
// slog to stderr; scope with GOXSD_DEBUG=parser,validate,codec),
// -strict (exit 4, by the exit-code table's 4 row, when the run names a
// schemaLocation resolving to no document; off, such a line moves no
// exit code). They qualify a subcommand and follow its name — goxsd8
// parse -q a.xsd, not goxsd8 -q parse a.xsd. -q suppresses a
// subcommand's informational output, which is parse's summary, and
// never a diagnosis: neither the error lines above nor validate's
// violations and undecided checks are silenced by it.
//
// Implemented today: the help path, parse, validate and gen. With no
// arguments, or with -h, -help or --help in any argument position, goxsd8
// prints this usage to stdout and exits 0, the whole of it even after a
// subcommand name, because a per-subcommand copy could drift from this one.
// goxsd8 parse compiles its arguments as above and honours -q, -v and
// -strict; goxsd8 validate assesses XML and JSON instances and goxsd8 gen
// generates as above, and both honour -v and -strict, -q silencing nothing
// there because neither writes informational output. GOXSD_DEBUG scopes
// -v for none of them. Every other invocation exits 2, reporting on
// stderr that the name is not one of the three, that a help flag is
// spelled with a value, as -help=<value>, that a flag stands before the
// subcommand it qualifies, or that the first argument is a flag and no
// subcommand was given.
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
// help requests wherever they stand, and where that is decides the answer.
// Before a subcommand one is a flag where a name belongs, and among a
// subcommand's flags it is a flag whose value the subcommand does not accept:
// both are usage errors naming the three spellings, and neither reports the
// flag as one to move, since no argument position accepts a valued help
// spelling. After a subcommand's first positional it is a misplaced
// flag-shaped token, the exit-2 diagnosis stated below, which names none of
// the spellings. After a -- that ended a subcommand's options it is a
// positional like any other there: a path to parse and validate, and to gen
// the positional it takes none of.
//
// The usage above owns where a help flag is read and why help is unscoped.
// No argument beside a help flag is examined, so goxsd8 -xyz -help prints
// the usage and exits 0 as well. That scan is positional-blind and gives --
// no end-of-options meaning: goxsd8 -- -help and goxsd8 validate -schema s.xsd
// -- -help are help requests, exit 0, so an argument spelled -h, -help or
// --help is reached as ./-help even after --.
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
// A -- standing in a flag's place, before a subcommand's first positional, is
// read by the flag package as the end of that subcommand's options: every
// argument after it is a positional, flag-shaped or not, so goxsd8 validate
// -schema s.xsd -- -weird.xml -q.xml assesses the files -weird.xml and
// -q.xml. A -- standing as a flag's value, as in validate -schema -- a.xml,
// is that value and ends nothing, and a -- after a positional is itself a
// misplaced flag-shaped token, exit 2.
//
// A schema argument is a filesystem path, resolved to an absolute one before
// the document is read, so an argument spelled absolutely or through "../"
// works; the usage block's Paths paragraph says how a line names it. Relative
// <xs:include>, <xs:import> and <xs:override> locations inside it resolve
// against that document's own directory. That resolution is confined to no
// subtree: a schema document may name any path the invoking user can read, so
// an <xs:include schemaLocation="../../../etc/passwd"> is served like any
// other. A directive whose schemaLocation resolves to no document is skipped
// rather than rejected, and the usage block names it in a line and not as an
// error, for the reason that block gives: no rule charges the skip.
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
// calls not a pass. Each such check prints a line on stdout in the shape
// Diagnostic lines below states for a declined check, so a script scanning
// for a rule ID reads them with the charges and a reader tells them apart.
//
// A JSON instance on validate/jsonsrc's error channel has no Unevaluated
// record and no assessment standing. That channel is one error carrying
// malformed JSON, content after the top-level object, an I/O fault reading the
// instance once it is open (which jsonsrc reports as a fault reading the
// input) or an input jsonsrc's mapping declines, and validate routes it by the
// class jsonsrc gives it in a *jsonsrc.Error. Its one line is on stdout with
// the report, in the shapes Diagnostic lines below states:
//
//   - Malformed JSON, class jsonsrc.Malformed, exits 1, as malformed XML does:
//     <loc>: [jsonsrc] <message>, ending with the -format clause the usage
//     block states where it applies.
//   - A fault reading the input, class jsonsrc.ReadFault, exits 1, as an I/O
//     fault reading an XML instance does: <loc>: [jsonsrc] <message>.
//   - A decline, class jsonsrc.Decline, exits 4: <loc>: [jsonsrc] undecided:
//     <message>. A decline is a withhold, never a verdict, so the instance
//     stands undecided and not invalid, and it ranks as any 4 does.
//
// validate assesses its instances in argument order. An instance argument
// spelled - is standard input, which has no extension to name a source format,
// so it needs -format: xml and json are both assessed, and without the flag it
// exits 2 saying so (#2403). An instance argument naming a directory, by any
// name and under any -format, is charged exit 2 as a directory before its
// extension is read. -schema - is not supported: a schema document's location
// is the base URI its own relative <xs:include>, <xs:import> and <xs:override>
// references resolve against, and standard input has none. That spelling is
// refused, exit 2, rather than opened, so a file which happens to be named -
// is never compiled as the schema set behind it; ./- is what names that file.
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
// rule ID and, without -strict, moving no exit code: src-import (§4.2.6.2)
// makes a schemaLocation that resolves to nothing legal to skip, so that hint's
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
// There is no version entry point and none is planned before 1.0: run
// go version -m $(which goxsd8) for the module version of a tagged build.
// -v is not available for one, being already assigned to debug logging.
//
// The CLI is a thin shell over the library for the capabilities it runs
// today: schema compilation and XML instance assessment are reachable through
// parser, xsd, validate and validate/xmlsrc, which reads a DOCTYPE the way
// validate does, and the README shows both routes for them; JSON instance
// assessment is validate/jsonsrc's Validate, which -format json calls. That
// mapping is not whole: the inputs it still declines (its doc's Every decline)
// reach the exit-4 decline line above rather than a verdict. Code generation is
// codegen.Generate, which gen calls once with one Target per -schema/-out pair;
// it emits Go types for M9's first slice of schema shapes, and the constructs
// outside that slice reach gen's exit-4 decline line. The capabilities this
// page reserves are not whole library routes either: codec (M10) decodes a
// narrow slice through codec.Decode and encodes nothing, and validate/bersrc
// (M11) exports nothing yet, so no subcommand decodes, encodes or reads a BER
// instance until those milestones land. Error output is stable and
// line-oriented for scripting.
//
// # Diagnostic lines
//
// A diagnostic line is <loc>: [<rule>] <message>, on stderr for a schema error
// and on stdout for a violation. A check the assessment declined prints on
// stdout too, with the literal undecided: after [<rule>], which tells it from
// a charge on the line itself while keeping the split every other line has:
//
//	<loc>: [<rule>] undecided: <message>
//
// <loc> is <file>:<line>:<col>, ? when unknown, and <rule> is the spec
// validation rule ID. validate renders its violations the way parse renders a
// schema error, the no-rule-to-cite exception included, which on validate's
// path is the line naming the source fault that stopped the walk. A JSON
// instance on validate/jsonsrc's error channel is no exception: jsonsrc, the
// source that raised the error and no rule ID, stands where <rule> stands, so
// the line splits like every other, unmarked for malformed JSON and a fault
// reading the input and marked for a decline:
//
//	<loc>: [jsonsrc] <message>
//	<loc>: [jsonsrc] undecided: <message>
//
// The literal undecided: marks both withholds on purpose, a declined check and
// a jsonsrc decline, and the token before it, a rule ID or jsonsrc, tells them
// apart. The line does not tell malformed JSON from a fault reading the input,
// as an [xml-wf] line does not tell malformed XML from a fault reading an XML
// instance.
//
// A decimal element holding "12,50" prints:
//
//	order.xml:1:1: [cvc-type] element amount's value "12,50" is not a valid {http://www.w3.org/2001/XMLSchema}decimal, which cvc-type clause 3.1.3 requires: [cvc-datatype-valid] "12,50" is not in the lexical space of decimal, which cvc-datatype-valid clause 2.1 requires it to be in
//
// <rule> is the rule CHARGED, and for the content of an element or attribute
// that is never cvc-datatype-valid: the charge is cvc-type clause 3.1.3,
// cvc-attribute clause 3 or cvc-complex-type clause 1.2, each of which
// delegates through cvc-simple-type (String Valid) to cvc-datatype-valid and
// carries that verdict as a WRAPPED cause — rendered into the message, as
// above, and reachable as an *xsderr.Error of its own through errors.As and
// xsderr.RuleOf. Key a dispatcher on the outer rule.
package main

// The # Usage section of the package comment above is generated:
// TestDocRendersUsage renders it from usage (main.go) and go generate rewrites
// it. Edit usage, never that section; the rest of the package comment is
// hand-written.
