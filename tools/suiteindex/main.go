// Command suiteindex takes a census of the W3C xsdtests corpus: which
// fixtures carry a given XML construct, across every file in the tree and in
// every encoding the corpus ships.
//
// A grounding round predicting ratchet movement needs the set of fixtures
// exercising a construct. A grep misses members of that set by mechanisms
// that share no fix: a UTF-16LE fixture read as interleaved NULs by a UTF-8
// grep, a fixture outside the single directory a search named, and a fixture
// found by matching one clause's SHAPE instead of the construct it carries
// (#1239). PRINCIPLES 27 turns that shape of repeated, deterministic work
// into a tool.
//
// # What it matches
//
// A query names a construct, never a spelling of one. Element names match on
// the resolved (namespace URI, local name) pair, so `<xsd:element>`,
// `<xs:element>` and a bare `<element>` under
// `xmlns="http://www.w3.org/2001/XMLSchema"` are one construct and all three
// answer one query. Matching prefix text is the defect this tool retires:
// the corpus writes all three spellings, sometimes within one directory.
//
// Attribute names match by local name in no namespace, which is what an
// unprefixed attribute resolves to — XSD's own vocabulary attributes
// (targetNamespace, form, ref) are always written unprefixed. A namespace
// declaration (xmlns, xmlns:p) is not an attribute and never matches
// ([xmltree.StartElement.Attributes]).
//
// A local name may be written `*`, which stands for every local name in its
// namespace: `*` is every element in the XML Schema namespace and `@*` is
// every attribute in no namespace. The wildcard is what makes a census
// UNANCHORED — an axis over every element, or over every attribute name,
// rather than a lookup of one construct — and it needs no escape, since `*`
// is not an NCName and no document can spell a name that collides with it.
//
// A NAMESPACE may be written `{*}`, which stands for every namespace at once,
// the no-namespace one included: `{*}root` is every `root` element in the
// corpus whatever namespace it sits in, and `{*}*` is every element there is.
// The element position needs it because a braceless element name means the
// XML Schema namespace — right for a schema document, wrong for an INSTANCE
// one, whose elements sit in whatever namespace its test targets — so an
// instance-side census was inexpressible and answered `0 occurrence(s)`
// rather than saying so (#1495). It cannot be confused with `{}local`, which
// is the one namespace that has no name, and it costs no census that worked
// before: `{*}local` parsed as the namespace literally named `*`, which
// nothing in this corpus declares.
//
// Attribute names are joined by `,` (an element must carry EVERY one to count
// as an occurrence) or by `|` (ANY one of them is enough). One query uses one
// join, never both, and `@*` stands alone: it already names the whole
// attribute axis. A query that names its attributes prints the values it
// matched, in query order.
//
// ELEMENT names take the `|` join only, which is how a FEATURE spelled by
// more than one element is censused in one query:
// `openContent|defaultOpenContent` censuses `{open content}`, and CLAUDE.md's
// suiteindex paragraph says how far that figure holds (#1554).
// The `,` join is refused in that position rather than matching nothing — no
// element carries two names at once — and a query naming more than one of
// them always prints the element each hit was, since the query no longer
// fixes it.
//
// # Containment
//
// A query may require its occurrences to lie INSIDE another matching element,
// at any depth, by writing that element's pattern ahead of `//`:
// `*@maxOccurs//*@maxOccurs` is every element carrying `maxOccurs` that
// stands inside another one carrying it (#1585). Both sides of `//` are whole
// patterns — element alternatives, an attribute list and its join each — and
// each side is read on its own, so `A//B` finds a B under an A and never a B
// under a B.
//
// An element is not its own ancestor: a start tag answering both patterns
// qualifies nothing but its own descendants, which is the entire difference
// between `A//A` and `A`. A hit records the INNERMOST enclosing element that
// matched, since that is the one a reader checks the nest against first.
//
// One query names one ancestor. A second `//` is refused rather than read as
// a chain: the ancestor side is a pattern and takes no ancestor of its own, so
// a chain is unrepresentable rather than silently half-applied.
//
// Containment is the DOCUMENT's own nesting. This census never resolves an
// `<element ref>` or a `<group ref>`, so a containment figure answers for a
// lexical population and not for a population of resolved components; the
// report says so in its own output, in both directions it can be wrong
// ([printCaveat]).
//
// # Value tests
//
// Write `=` and operands after an attribute list to select occurrences by
// what a QName-valued attribute RESOLVES to, against the bindings in scope at
// its own start tag:
// `*@type|base|itemType|memberTypes={http://www.w3.org/2001/XMLSchema}ENTITY`
// is every reference to xs:ENTITY, however each fixture prefixes it (#1671).
// The test applies to every attribute in the list, and an attribute answers
// it when the element carries it and at least one of its tokens satisfies at
// least one operand; the list's join is then read over the attributes that
// answer.
//
// An operand is a Clark name with its wrapper always written — `{uri}local`,
// `{}local` for no namespace, `{*}` and `*` wildcarding either axis as they do
// in the element position — or the keyword UNBOUND, and operands take the `|`
// join only. A braceless name is refused: a value's namespace is data, so no
// default is assumed for it. `@*` takes no value test.
//
// Every tested value is split on XML whitespace (#x20, #x9, #xD, #xA) into
// tokens, whatever attribute carries it, so a list such as memberTypes is
// matched per token and a scalar QName is one token. Each token splits on its
// first colon; the prefix, or the default namespace for an unprefixed token,
// resolves in scope. A token whose prefix is bound nowhere answers UNBOUND,
// and `{*}*` never matches it: `{*}*|UNBOUND` is every QName-shaped token. An
// unprefixed token with no default namespace in scope is BOUND, to no
// namespace, and answers `{}local`. A token with an empty prefix, an empty
// local part or a second colon is not a QName and answers no operand at all.
// NCName character validity is not checked: this census resolves bindings and
// does not validate a lexical space.
//
// A match line of a value-test query follows each tested attribute's raw
// value with `->[…]`, one resolution per token in token order: the braced
// name, UNBOUND, or `(not a QName)`. The report prints a caveat under its
// header, because the figure is a bound on DIRECT references and not a
// population of resolved types ([printValueCaveat]).
//
// # Encoding
//
// Encoding is this tool's problem, not the caller's. Every fixture is read
// through [xmltree.Reader], which detects a byte-order mark and transcodes
// UTF-16 to UTF-8 before any token reaches the matcher (XML 1.0 §4.3.3).
// The suite's UTF-16 fixtures all carry a mark, so xmltree's markless-UTF-16
// gap (#361) does not reach this corpus.
//
// # What it reports
//
// A query that names the attributes it looks for reports its MATCHES, sorted
// by path with document order preserved within each file. A query that
// wildcards them (`@*`) reports the attribute-NAME AXIS instead: the
// (element, attribute) pairs, then the fixtures carrying each pair. Both end
// with every file the census could not read all the way through as XML: the
// ones that broke off partway, and the ones that held no element at all.
//
// The axis is grouped because ungrouped it is not a report anyone reads:
// `*@*` over the suite is 172,648 attribute occurrences, which one line each
// renders unreadable, and 185 pairs, which a table renders in a screenful
// (#1391). Its first section is that table — pair, occurrence count, fixture
// count, in name order — and its second lists each pair's fixtures in path
// order, deduplicated, so a pair's own fixtures are read off the section
// rather than by re-querying it.
//
// A match line carries its position, then the matched element's name where
// the query does not already fix it — a wildcard local part, the `{*}`
// namespace axis, or more than one element alternative — and never where it
// does, then `parent=` naming the element it is a direct child of — `(none)`
// for a document element — then `children=[…]` listing the elements directly
// under it, then the matched attributes' values. The parent is what
// separates a local occurrence from a top-level one, which no query over an
// element's OWN attributes can express (#1282). It is the parent the
// document actually spells, never one corrected against the grammar: an
// `xs:element` under `xs:redefine` is reported there, wrong though that
// document is, and its children are reported the same way.
//
// A containment query prints its caveat ahead of the body, and each match
// line carries an `ancestor=` field naming the innermost enclosing element
// that matched the ancestor pattern. That is not the `parent=` field: a
// qualifying ancestor stands any number of levels up, and the two name
// different elements as soon as the nest is deeper than one. Neither the
// caveat nor the field appears for a query without an ancestor pattern, whose
// report is the same bytes it always was.
//
// An element name the report prints is spelled so that it re-enters as a
// query naming that same element, which is why a parent or child in NO
// namespace prints as `{}name` rather than bare: bare is the XML Schema
// namespace on the way back in (#1297).
//
// The child list is in document order and keeps repeats, because a
// content-model census turns on multiplicity: "no child outside
// `annotation | simpleType | complexType`" survives deduplication and "nor a
// second `simpleType` beside a first" does not (#1304). `children=[]` is an
// occurrence with no element child at all. A list whose last entry is
// `(unclosed)` is one whose read broke off before the match's own end tag, so
// the names ahead of the marker are a prefix rather than the whole list — a
// fact about that MATCH and not about its file, since a fixture can fault at
// its tail with every match in it already closed and every child list
// complete.
//
// `-paths` prints the matched fixtures ALONE on stdout, one path per line and
// nothing else, for a tool downstream: `go tool casejoin` turns that list into
// the conformance case IDs a lane's expectation file is keyed by, which is the
// join CLAUDE.md prescribes for a ratchet prediction (#1642). The header, its
// counts and any caveat go to stderr in that mode, so a pipeline still leaves
// them in front of the human; the per-file listings below are left to a plain
// run of the same query.
//
// The corpus ships deliberately malformed fixtures, so a parse fault is
// content rather than a failure: the matches found ahead of it are kept and
// the file is listed, because a construct BEHIND the fault is invisible here
// and the reader is the one who gets to judge that. Both groups are listed in
// full and nothing is elided — most of the second group is the suite's
// images, prose and stylesheets, but a census that counts what it could not
// read instead of naming it under-reports exactly as the greps did.
//
// # Usage
//
//	go tool suiteindex element@targetNamespace
//	go tool suiteindex attribute@targetNamespace,form
//	go tool suiteindex '{http://www.w3.org/1999/XSL/Transform}stylesheet'
//	go tool suiteindex '*@*'
//	go tool suiteindex '*@mixed|abstract|nillable'
//	go tool suiteindex 'openContent|defaultOpenContent'
//	go tool suiteindex '{*}*@{http://www.w3.org/2001/XMLSchema-instance}type'
//	go tool suiteindex '*@maxOccurs//*@maxOccurs'
//	go tool suiteindex '*@type|base|itemType|memberTypes={http://www.w3.org/2001/XMLSchema}ENTITY|{http://www.w3.org/2001/XMLSchema}ENTITIES'
//	go tool suiteindex '{*}*@{http://www.w3.org/2001/XMLSchema-instance}type=UNBOUND'
//	go tool suiteindex element@targetNamespace testdata/xsdtests/ibmData
//	go tool suiteindex -paths '{*}*@{http://www.w3.org/2001/XMLSchema-instance}type' | go tool casejoin join instance
//
// The query is `[pattern//]local[|local…][@attr[,attr…][=operand[|operand…]]]`
// with `|` in place of `,` for the ANY join over attributes, and any local
// part may be `*`. The pattern before `//` has that same shape and names the
// ancestor an occurrence lies inside. Any name may be written in Clark notation
// (`{uri}local`) to name its namespace outright, or `{*}local` to census
// every namespace at once; a braceless element name is in the XML Schema
// namespace and a braceless attribute name is in no namespace. The second
// argument is the tree to walk, defaulting to the suite at [suiteroot.Default];
// narrowing it is for reading one directory's output, never for taking the
// census the whole corpus answers.
//
// An absent or fixture-free root is a supported mode, not a failure: the
// submodule is absent in a fresh container (#659), so the tool says the
// corpus is not there, names the command that initializes it, and exits 0.
//
// It exits 0 for a completed census whatever it finds, and 2 for an
// operational error: an unparseable query, or a file or directory it cannot
// read.
package main

import (
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

	"github.com/kud360/goxsd8/parser/xmltree"
	"github.com/kud360/goxsd8/tools/internal/suiteroot"
	"github.com/kud360/goxsd8/xsd"
)

// usage is printed for any argument the tool cannot act on.
const usage = `usage: suiteindex [-paths] <[pattern//]local[|local...][@attr[,attr...][=operand[|operand...]]]> [dir]; "," joins attribute names as all, "|" joins either position's names as any, "//" admits a match only inside an element the pattern before it matches, at any depth, any local name may be "*", and "{uri}" before one fixes its namespace — "{*}" censuses every namespace, "{}" the one that has none; "=" after an attribute list admits only an attribute with a token resolving to one of its "|"-joined operands, each "{uri}local" or UNBOUND; -paths prints the matched fixture paths alone, for a tool downstream`

func main() {
	if err := run(os.Stdout, os.Stderr, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "suiteindex: %v\n", err)
		os.Exit(2)
	}
}

// run drives the census end to end: parse the query, walk the corpus, render
// the report. It takes its destinations and arguments so a test can drive the
// whole command without a subprocess.
func run(stdout, stderr io.Writer, args []string) error {
	flags := flag.NewFlagSet("suiteindex", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pathsOnly := flags.Bool("paths", false, "print the matched fixture paths alone, one per line, for a tool downstream")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%s", err, usage)
	}
	q, root, err := parseArgs(flags.Args())
	if err != nil {
		return err
	}
	rep, err := census(root, q)
	if err != nil {
		return err
	}
	if *pathsOnly {
		return printPaths(stdout, stderr, rep)
	}
	return printReport(stdout, rep)
}

// parseArgs splits the command line's positional arguments into the query and
// the tree to walk.
func parseArgs(args []string) (query, string, error) {
	if len(args) == 0 || len(args) > 2 {
		return query{}, "", errors.New(usage)
	}
	q, err := parseQuery(args[0])
	if err != nil {
		return query{}, "", err
	}
	if len(args) == 2 {
		return q, args[1], nil
	}
	return q, suiteroot.Default, nil
}

// wildcard is what stands for every name on the axis it is written on: as a
// local part, every local name in its namespace; inside a Clark wrapper
// (`{*}`), every namespace at once. It needs no escape and can never be
// ambiguous on the local axis: "*" is not an NCName (Namespaces in XML §4),
// so no name a document spells collides with it. On the namespace axis it is
// reserved rather than impossible — `{*}name` parsed before as the namespace
// literally named "*", a query no fixture in this corpus could ever answer
// (#1495) — so a namespace URI of "*" is the one URI this query language
// cannot name.
const wildcard = "*"

// containment separates the ancestor pattern from the construct that must lie
// inside it, and is the only axis this query language has. The spelling is
// available because `/` is not an NCName character (Namespaces in XML §4), so
// no local name a document writes collides with it, and a namespace URI
// carrying slashes is safe too: [splitName] consumes the Clark wrapper whole
// before it scans for a separator, so the `//` in `{http://…}element` is
// inside the URI and never a separator.
const containment = "//"

// elementSpace and attrSpace are the namespace a braceless name means in each
// position of a query: the vocabulary every schema fixture in this corpus is
// written in for an element, and no namespace for an attribute, which is what
// an unprefixed attribute resolves to. Neither default fits an INSTANCE
// document, whose elements sit in whatever namespace its test targets, which
// is what `{*}` is for ([wildcard]). The parser and the renderers read the
// same two constants, so the report cannot drift into spelling a name the way
// the other position reads it (STYLE D3, #1297).
const (
	elementSpace = xsd.XMLSchemaNS
	attrSpace    = ""
)

// namePat is one name a query matches: a namespace URI or [wildcard], and a
// local name or [wildcard]. Each axis carries its wildcard in the field
// itself rather than in a flag beside it — one fact, one encoding (STYLE D3),
// and an illegal "wildcard named foo" is unrepresentable.
type namePat struct {
	Space string
	Local string
}

// isAny reports whether p stands for every local name in its namespace, which
// is what makes a census an axis rather than a lookup.
func (p namePat) isAny() bool {
	return p.Local == wildcard
}

// anySpace reports whether p stands for its local name in EVERY namespace,
// no namespace included. It is the axis an instance-side census runs on: the
// namespaces the fixtures use are what such a census is trying to discover,
// so it cannot name one (#1495).
func (p namePat) anySpace() bool {
	return p.Space == wildcard
}

// isOpen reports whether p leaves either axis of the name open, so a hit p
// admitted is not named by p alone ([query.elementOpen]).
func (p namePat) isOpen() bool {
	return p.isAny() || p.anySpace()
}

// matches reports whether n is one of the names p stands for. The comparison
// is on namespace URI and local part, never on the prefix a document spelled —
// that equivalence is the tool's whole point.
func (p namePat) matches(n xsd.QName) bool {
	return (p.anySpace() || p.Space == n.Space) && (p.isAny() || p.Local == n.Local)
}

// render spells p for a query position whose braceless names mean
// defaultSpace, wildcards and all (`{uri}*`, `{*}name`), so the reader sees
// the axis that was censused ([renderIn]). The namespace wildcard is written
// out here rather than left to [xsd.QName], which would spell the sentinel as
// though it were a URI by coincidence of its own rules.
func (p namePat) render(defaultSpace string) string {
	if p.anySpace() {
		return "{" + wildcard + "}" + p.Local
	}
	return renderIn(xsd.QName{Space: p.Space, Local: p.Local}, defaultSpace)
}

// attrJoin is what a query's attribute list means. Each constant IS its own
// separator, so the query language, the parser and [query.String] hold one
// spelling of it between them rather than a mapping table.
type attrJoin string

const (
	// joinAll admits only an element carrying EVERY name in the list.
	joinAll attrJoin = ","
	// joinAny admits one carrying at least one of them.
	joinAny attrJoin = "|"
)

// pattern is one element test: the element names it admits, the attribute
// names an occurrence carries, how that list is read, and the test on their
// values. A query is one pattern, plus the ancestor pattern an occurrence must
// lie inside.
type pattern struct {
	// Element holds the element names an occurrence may carry, each an
	// alternative: a feature spelled by more than one element is one census
	// and not two to be added up by hand (#1554). It is never empty, and
	// alternatives are held in the order the query wrote them so the echo
	// re-enters as itself.
	Element []namePat
	Attrs   []namePat
	Join    attrJoin
	// Value is the test every attribute in Attrs must also pass to count as
	// carried ([valueTest]). It is inactive for a pattern with no attribute
	// list, and never active beside `@*`: the parser refuses both.
	Value valueTest
}

// valueTest is a test on the VALUE of an attribute, written after the
// attribute list as `=` and operands joined by `|`: an attribute answers it
// when at least one of its whitespace-separated tokens, resolved as a QName
// against the bindings in scope at its own start tag, satisfies at least one
// operand (#1671). Names and Unbound are independent alternatives, and the
// test is inactive when neither is set ([valueTest.active]).
type valueTest struct {
	// Names are the `{uri}local` operands, in query order, each wildcarded
	// on either axis exactly as [namePat] is anywhere else. They match a
	// BOUND token only.
	Names []namePat
	// Unbound is the [unboundOperand] operand: it matches a token whose
	// prefix no in-scope binding declares. It is a bool and not a count, so a
	// repeated keyword changes nothing.
	Unbound bool
}

// unboundOperand is the operand matching a token whose prefix is unbound. It
// cannot collide with a name operand, every one of which is brace-led: a
// no-namespace name spelled UNBOUND is written `{}UNBOUND`.
const unboundOperand = "UNBOUND"

// valueSep introduces a pattern's [valueTest]. `=` is not an NCName
// character, so no attribute name a document writes collides with it.
const valueSep = "="

// active reports whether v tests anything at all. It is derived from the
// operands rather than held beside them (STYLE D3).
func (v valueTest) active() bool {
	return len(v.Names) > 0 || v.Unbound
}

// admits reports whether one resolved token satisfies an operand of v. A
// token that is not a lexical QName satisfies none, [unboundOperand] and
// `{*}*` included: no prefix was found for either to answer about.
//
// The switch is exhaustive over the sealed sum; the default arm asserts the
// invariant and is unreachable, since [tokenRes] is unexported and this file
// declares every arm.
func (v valueTest) admits(r tokenRes) bool {
	switch r := r.(type) {
	case boundToken:
		for _, n := range v.Names {
			if n.matches(r.Name) {
				return true
			}
		}
		return false
	case unboundToken:
		return v.Unbound
	case notQNameToken:
		return false
	default:
		panic("suiteindex: valueTest.admits: non-exhaustive tokenRes switch")
	}
}

// String renders v's operands in the canonical form — name operands first,
// in query order and always braced ([bracedName]), then [unboundOperand] —
// so the echo re-enters as the same test (#1297). It is empty for an
// inactive test.
func (v valueTest) String() string {
	if !v.active() {
		return ""
	}
	ops := make([]string, 0, len(v.Names)+1)
	for _, n := range v.Names {
		ops = append(ops, bracedName(n.Space, n.Local))
	}
	if v.Unbound {
		ops = append(ops, unboundOperand)
	}
	return valueSep + strings.Join(ops, string(joinAny))
}

// bracedName spells a name in Clark notation with the wrapper ALWAYS written,
// `{}local` for no namespace and `{*}` for the namespace axis. A value
// operand has no default namespace, so neither [xsd.QName.String] nor
// [renderIn], which leave a no-namespace name bare, can spell one.
func bracedName(space, local string) string {
	return "{" + space + "}" + local
}

// query is one census: the construct to look for, and the ancestor an
// occurrence of it must lie inside.
type query struct {
	pattern
	// Ancestor is the pattern an occurrence must lie INSIDE, at any depth, or
	// nil for a census that asks no containment question. It is a [pattern]
	// and not a second query, so a chained `A//B//C` cannot be built at all
	// (STYLE T1) — the parser refuses it, and nothing downstream carries a
	// branch for a chain it might have admitted.
	Ancestor *pattern
}

// matchesElement reports whether n is one of the element names p admits.
func (p pattern) matchesElement(n xsd.QName) bool {
	for _, e := range p.Element {
		if e.matches(n) {
			return true
		}
	}
	return false
}

// elementOpen reports whether a hit's own element name is NOT derivable from
// p, so the report must print it ([renderMatched]). Anything but exactly one
// alternative leaves it open, however tightly each alternative is written:
// two elements' hits are one undifferentiated run of lines otherwise, which
// is the whole point of censusing them together (#1554).
func (p pattern) elementOpen() bool {
	if len(p.Element) != 1 {
		return true
	}
	return p.Element[0].isOpen()
}

// anyAttr reports whether the pattern censuses the attribute-NAME axis —
// every attribute in a namespace rather than named ones. It is derived from
// the attribute list, which parsing has already restricted to a lone wildcard
// (STYLE D3): `@*` names the whole axis and cannot be joined with anything.
func (p pattern) anyAttr() bool {
	return len(p.Attrs) == 1 && p.Attrs[0].isAny()
}

// String renders one pattern in the canonical form: every name in Clark
// notation, so the reader sees the namespace that was matched rather than the
// prefix some fixture happened to spell it with.
func (p pattern) String() string {
	var b strings.Builder
	for i, e := range p.Element {
		if i > 0 {
			b.WriteString(string(joinAny))
		}
		b.WriteString(e.render(elementSpace))
	}
	for i, a := range p.Attrs {
		sep := string(p.Join)
		if i == 0 {
			sep = "@"
		}
		b.WriteString(sep)
		b.WriteString(a.render(attrSpace))
	}
	b.WriteString(p.Value.String())
	return b.String()
}

// String renders the query in the canonical form the report echoes, the
// ancestor pattern and its separator included. The echo is itself a query —
// [parseQuery] takes it back to this query, whatever namespaces it names
// (#1297).
func (q query) String() string {
	if q.Ancestor == nil {
		return q.pattern.String()
	}
	return q.Ancestor.String() + containment + q.pattern.String()
}

// parseQuery parses `[pattern//]local[|local…][@attr[,attr…]]` — `|` in place
// of `,` for the ANY join over attributes — where any name may carry a Clark
// `{uri}` wrapper and either part of it may be [wildcard]: `{*}` in the
// wrapper's place is every namespace, `*` in the local part's is every local
// name. A braceless element name is in the XML Schema namespace — the
// vocabulary every schema fixture in this corpus is written in — and a
// braceless attribute name is in no namespace, which is what an unprefixed
// attribute resolves to. A [valueSep] after the attribute list starts a
// [valueTest]. A [containment] separator makes the pattern ahead of it the
// ancestor an occurrence must lie inside.
func parseQuery(s string) (query, error) {
	first, rest, err := parsePattern(s, s)
	if err != nil {
		return query{}, err
	}
	if rest == "" {
		return query{pattern: first}, nil
	}
	if !strings.HasPrefix(rest, containment) {
		return query{}, fmt.Errorf("query %q: the ancestor separator is %q, found %q", s, containment, rest)
	}
	inner, rest, err := parsePattern(s, rest[len(containment):])
	if err != nil {
		return query{}, err
	}
	if rest != "" {
		return query{}, fmt.Errorf("query %q: %q separates one ancestor from the construct it encloses and appears at most once, found a second one at %q", s, containment, rest)
	}
	return query{pattern: inner, Ancestor: &first}, nil
}

// parsePattern consumes one pattern from the head of s — an element position,
// then an attribute list if an `@` follows and its value test if a
// [valueSep] follows that — and returns it with whatever is
// left, which is the [containment] separator or nothing at all. whole is the
// query the caller was given, which every message names, so a rejection reads
// against what was typed rather than against the fragment that failed.
func parsePattern(whole, s string) (pattern, string, error) {
	elems, rest, err := parseElements(whole, s)
	if err != nil {
		return pattern{}, "", err
	}
	p := pattern{Element: elems, Join: joinAll}
	if strings.HasPrefix(rest, valueSep) {
		return pattern{}, "", fmt.Errorf("query %q: a value test follows an attribute list (%q), never an element name", whole, "@attr"+valueSep+"…")
	}
	if !strings.HasPrefix(rest, "@") {
		return p, rest, nil
	}
	rest = rest[1:]
	for {
		attr, more, err := splitName(rest, attrSpace)
		if err != nil {
			return pattern{}, "", fmt.Errorf("query %q: %w", whole, err)
		}
		p.Attrs = append(p.Attrs, attr)
		if strings.HasPrefix(more, valueSep) {
			p.Value, more, err = parseOperands(whole, more[len(valueSep):])
			if err != nil {
				return pattern{}, "", err
			}
			if more != "" && !strings.HasPrefix(more, containment) {
				return pattern{}, "", fmt.Errorf("query %q: a value test ends the pattern, so only %q or the end of the query may follow its operands, found %q", whole, containment, more)
			}
		}
		if more == "" || strings.HasPrefix(more, containment) {
			closed, err := closeAttrs(whole, p)
			if err != nil {
				return pattern{}, "", err
			}
			return closed, more, nil
		}
		join := attrJoin(more[:1])
		if join != joinAll && join != joinAny {
			return pattern{}, "", fmt.Errorf("query %q: expected %q or %q between attribute names, found %q", whole, joinAll, joinAny, more)
		}
		if len(p.Attrs) > 1 && join != p.Join {
			return pattern{}, "", fmt.Errorf("query %q: attribute names are joined by %q (every one) or by %q (any one), never both", whole, joinAll, joinAny)
		}
		p.Join = join
		rest = more[1:]
	}
}

// parseElements consumes a pattern's element position — one name, or several
// joined by [joinAny] — and returns them with whatever follows, the `@` of an
// attribute list included.
//
// [joinAll] is REFUSED here rather than parsed or left to the attribute
// list's own error: it means every name at once, which no single element
// carries, and the position's old error reported it as a missing `@`, which
// says nothing about why the census cannot be taken.
func parseElements(whole, s string) ([]namePat, string, error) {
	var elems []namePat
	rest := s
	for {
		p, more, err := splitName(rest, elementSpace)
		if err != nil {
			return nil, "", fmt.Errorf("query %q: %w", whole, err)
		}
		elems = append(elems, p)
		if strings.HasPrefix(more, string(joinAll)) {
			return nil, "", fmt.Errorf("query %q: element names are joined by %q (any one of them), never by %q: no element carries two names at once, and attribute names follow %q", whole, joinAny, joinAll, "@")
		}
		if !strings.HasPrefix(more, string(joinAny)) {
			return elems, more, nil
		}
		rest = more[1:]
	}
}

// closeAttrs finishes a parsed attribute list, rejecting the combinations
// the grammar admits and the matcher cannot mean: a wildcard beside another
// name, or beside a value test. `@*` already stands for every attribute, so
// joining it to a second name says nothing under either join, and it selects
// the name-axis report, which has no value column to print a resolution in.
func closeAttrs(whole string, p pattern) (pattern, error) {
	for _, a := range p.Attrs {
		if !a.isAny() {
			continue
		}
		if len(p.Attrs) > 1 {
			return pattern{}, fmt.Errorf("query %q: %q names the whole attribute axis and stands alone", whole, "@"+wildcard)
		}
		if p.Value.active() {
			return pattern{}, fmt.Errorf("query %q: %q names the whole attribute axis and takes no value test", whole, "@"+wildcard)
		}
	}
	return p, nil
}

// parseOperands consumes a value test's operands from the head of s — the
// text after [valueSep] — and returns them with whatever follows. Operands
// take the [joinAny] join only, so a `|` here never starts another attribute
// name, and a [joinAll] is refused: one token is never two names at once.
//
// Every operand is [unboundOperand] or brace-led. A braceless name is
// refused rather than given a default namespace, as the element and
// attribute positions do: a value's namespace is data the query cannot
// assume, so it is written out — `{uri}local`, `{}local` or `{*}local`.
func parseOperands(whole, s string) (valueTest, string, error) {
	var v valueTest
	rest := s
	for {
		more, err := parseOperand(whole, rest, &v)
		if err != nil {
			return valueTest{}, "", err
		}
		if strings.HasPrefix(more, string(joinAll)) {
			return valueTest{}, "", fmt.Errorf("query %q: value operands are joined by %q (any one of them), never by %q: one token is never two names", whole, joinAny, joinAll)
		}
		if !strings.HasPrefix(more, string(joinAny)) {
			return v, more, nil
		}
		rest = more[len(joinAny):]
	}
}

// parseOperand consumes one operand from the head of s into v and returns
// what follows it.
func parseOperand(whole, s string, v *valueTest) (string, error) {
	if strings.HasPrefix(s, "{") {
		n, more, err := splitName(s, "")
		if err != nil {
			return "", fmt.Errorf("query %q: %w", whole, err)
		}
		v.Names = append(v.Names, n)
		return more, nil
	}
	tok, more := s, ""
	if i := strings.IndexAny(s, nameStops); i >= 0 {
		tok, more = s[:i], s[i:]
	}
	if tok == "" {
		return "", fmt.Errorf("query %q: empty value operand", whole)
	}
	if tok != unboundOperand {
		return "", fmt.Errorf("query %q: value operand %q names no namespace: write it out as {uri}local, {}local for no namespace or {*}local for any, since no default can be assumed for data, or write %s", whole, tok, unboundOperand)
	}
	v.Unbound = true
	return more, nil
}

// nameStops are the characters that end a local part in a query: the start of
// an attribute list or a value test, either join, and the [containment]
// separator.
const nameStops = "@" + valueSep + string(joinAll) + string(joinAny) + containment

// splitName consumes one name from the head of s — an optional Clark
// `{uri}` wrapper, then a local part — and returns it with whatever follows.
// The URI is taken as everything up to the closing brace, so a namespace
// containing a separator survives the separators around it — the `//` of
// `{http://…}element` included — and a wrapper holding [wildcard] alone is
// the namespace axis rather than a URI.
func splitName(s, defaultSpace string) (p namePat, rest string, err error) {
	space := defaultSpace
	if strings.HasPrefix(s, "{") {
		end := strings.Index(s, "}")
		if end < 0 {
			return namePat{}, "", fmt.Errorf("unterminated \"{\" in %q", s)
		}
		space = s[1:end]
		s = s[end+1:]
	}
	local := s
	if i := strings.IndexAny(s, nameStops); i >= 0 {
		local, rest = s[:i], s[i:]
	}
	if local == "" {
		return namePat{}, "", errors.New("empty local name")
	}
	if strings.ContainsAny(local, "{}") {
		return namePat{}, "", fmt.Errorf("local name %q contains a brace: write a namespace as a leading {uri}", local)
	}
	return namePat{Space: space, Local: local}, rest, nil
}

// attrHit is one attribute an occurrence carried and the query matched: the
// name as the document resolved it, and its value. The name is the hit's own
// and not the query's, because a query can leave it open — `@*` names no
// attribute at all, `{*}` leaves its namespace open, and the `|` join names
// more than the element carries.
type attrHit struct {
	Name  xsd.QName
	Value string
	// Resolved holds one resolution per whitespace-separated token of Value,
	// in token order — every token, not only the ones that satisfied the
	// test, since the others are what a reader checks the hit against. It is
	// nil exactly when the query has no value test ([valueTest]), which is
	// what keeps every other query's match lines the bytes they always were.
	Resolved []tokenRes
}

// tokenRes is what one token of an attribute value resolved to as a QName. It
// is a SEALED SUM (STYLE T1/T2), closed by the QName lexical space and sealed
// by the tokenRes marker method, with one arm per answer:
//
//   - boundToken — its prefix, or the default namespace for an unprefixed
//     token, is bound in scope, and it names an expanded name;
//   - unboundToken — its prefix is bound nowhere in scope;
//   - notQNameToken — it is not a lexical QName at all, so it has no prefix
//     to be bound or unbound.
type tokenRes interface{ tokenRes() }

// boundToken is a token that resolved to Name.
type boundToken struct{ Name xsd.QName }

// unboundToken is a token whose prefix no in-scope binding declares.
type unboundToken struct{}

// notQNameToken is a token that is not a lexical QName.
type notQNameToken struct{}

func (boundToken) tokenRes()    {}
func (unboundToken) tokenRes()  {}
func (notQNameToken) tokenRes() {}

// hit is one occurrence of the queried construct.
type hit struct {
	// File is the fixture's slash-separated path relative to the census root.
	File string
	Line int
	Col  int
	// Element is the resolved name of the matched element itself. It repeats
	// the query for a query that fixes one element name, and is the only
	// record of which element was hit for one that leaves it open — a
	// wildcard, the `{*}` axis, or several alternatives ([query.elementOpen])
	// — which is what the axis report groups on.
	Element xsd.QName
	// Parent is the resolved name of the element this match is a direct child
	// of, taken from the document as written and never reconciled with the
	// grammar. The zero QName means the match is the document element and has
	// no parent at all — a distinct value rather than a second field, because
	// no name has an empty local part ([xsd.QName]).
	Parent xsd.QName
	// Ancestor is the resolved name of the innermost enclosing element that
	// matched the query's ancestor pattern, and the zero QName for a query
	// that has none. It is not derivable from Parent: a qualifying ancestor
	// stands any number of levels up, and the pattern that admitted it can be
	// open on either axis of the name.
	Ancestor xsd.QName
	// Children are the resolved names of the elements directly under this
	// match, in document order and with repeats kept. The multiplicity is the
	// point: a content-model census asks whether a second xs:simpleType stands
	// beside a first, which a deduplicated list or a distinct-child tally
	// cannot answer (#1304).
	Children []xsd.QName
	// ChildrenUnclosed reports that the read ended before this match's own end
	// tag, so Children is a prefix of the real list rather than the whole of
	// it. It is a fact about the MATCH, which [report.Partial] — a fact about
	// the file — cannot carry: a fixture that faults at its tail can hold
	// matches that all closed cleanly, and a file-level flag would report
	// their complete child lists as unread.
	ChildrenUnclosed bool
	// Attrs holds the attributes the query matched, in query order — in
	// document order for `@*`, which fixes no order of its own.
	Attrs []attrHit
}

// fileNote is one file the census could not read all the way through as XML,
// with the fault that stopped it — nil for a file that ended cleanly without
// ever holding an element.
type fileNote struct {
	File string
	Err  error
}

// report is one completed census. Only facts that cannot be recomputed are
// stored: the counts of fully read files and of matched fixtures are derived
// where they are printed (STYLE D3).
//
// Every walked file lands in exactly one of three places — read to the end,
// [report.Partial], or [report.NoElement] — and the two latter groups are
// listed rather than counted. A census that silently drops the files it could
// not read is the defect this tool exists to retire, one layer down.
type report struct {
	Query query
	Root  string
	// RootPresent is whether the root directory exists at all. It is not
	// derivable from Walked, which is 0 for an empty directory too, and the
	// two cases want different advice (#659).
	RootPresent bool
	// Walked is how many regular files the census opened.
	Walked int
	Hits   []hit
	// Partial holds files whose read broke off after at least one start tag,
	// so a construct behind the fault is invisible to the census.
	Partial []fileNote
	// NoElement holds files that yielded no start tag at all: the corpus's
	// images, stylesheets and prose, plus any fixture that faulted ahead of
	// its root element.
	NoElement []fileNote
}

// pairName is the attribute-name axis's unit: one element name and one
// attribute name found on it.
type pairName struct {
	Element xsd.QName
	Attr    xsd.QName
}

// pairGroup is what the census found for one pair — how many occurrences, and
// the fixtures they fall in, each named once and in path order.
type pairGroup struct {
	pairName
	Occurrences int
	Fixtures    []string
}

// groupPairs collects the axis from the hits, in name order. Hits arrive in
// path order, so a pair's fixture list is deduplicated by comparing with the
// last path appended to it and needs no set, exactly as [countFiles] does.
//
// The map is an index into the slice being built and is never ranged over;
// the report's order is the sort below (STYLE D2).
func groupPairs(hits []hit) []pairGroup {
	at := map[pairName]int{}
	var groups []pairGroup
	for _, h := range hits {
		for _, a := range h.Attrs {
			name := pairName{Element: h.Element, Attr: a.Name}
			i, ok := at[name]
			if !ok {
				groups = append(groups, pairGroup{pairName: name})
				i = len(groups) - 1
				at[name] = i
			}
			g := &groups[i]
			g.Occurrences++
			if len(g.Fixtures) == 0 || g.Fixtures[len(g.Fixtures)-1] != h.File {
				g.Fixtures = append(g.Fixtures, h.File)
			}
		}
	}
	sort.Slice(groups, func(i, j int) bool { return lessPair(groups[i].pairName, groups[j].pairName) })
	return groups
}

// lessPair orders the axis by element name and then attribute name, each
// namespace before local part, which is the order the pairs are printed in.
func lessPair(a, b pairName) bool {
	if a.Element != b.Element {
		return a.Element.Space < b.Element.Space ||
			(a.Element.Space == b.Element.Space && a.Element.Local < b.Element.Local)
	}
	return a.Attr.Space < b.Attr.Space ||
		(a.Attr.Space == b.Attr.Space && a.Attr.Local < b.Attr.Local)
}

// census walks root and matches q against every file under it. Files are
// scanned in path-sorted order and each file's hits stay in document order,
// so the whole report is deterministic (STYLE D1).
func census(root string, q query) (report, error) {
	paths, present, err := walkFiles(root)
	if err != nil {
		return report{}, err
	}
	rep := report{Query: q, Root: root, RootPresent: present, Walked: len(paths)}
	for _, rel := range paths {
		scan, err := scanFile(filepath.Join(root, filepath.FromSlash(rel)), rel, q)
		if err != nil {
			return report{}, err
		}
		rep.Hits = append(rep.Hits, scan.Hits...)
		if scan.Elems == 0 {
			rep.NoElement = append(rep.NoElement, fileNote{File: rel, Err: scan.Err})
			continue
		}
		if scan.Err != nil {
			rep.Partial = append(rep.Partial, fileNote{File: rel, Err: scan.Err})
		}
	}
	return rep, nil
}

// walkFiles lists every regular file under root as a slash path relative to
// it, sorted. present is false when root does not exist — the fresh-container
// case, which is a mode rather than an error.
func walkFiles(root string) (paths []string, present bool, err error) {
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
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relating %s to %s: %w", path, root, err)
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, true, fmt.Errorf("walking %s: %w", root, err)
	}
	sort.Strings(paths)
	return paths, true, nil
}

// fixtureScan is what one fixture yielded: its matches, how many start tags
// it held at all, and the fault that ended the read early, if any. Err is a
// property of the fixture's own bytes — a malformed document is content in
// this corpus, not an operational failure — which is why it is a field here
// and not a returned error.
type fixtureScan struct {
	Hits  []hit
	Elems int
	Err   error
}

// scanFile opens path and scans it as uri. It is the only filesystem-touching
// step of the match pipeline; [scanFixture] itself reads a stream. Its error
// is operational — a file that could not be opened or closed — and aborts the
// census rather than being counted as an unreadable fixture.
func scanFile(path, uri string, q query) (scan fixtureScan, err error) {
	f, err := os.Open(path)
	if err != nil {
		return fixtureScan{}, fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing %s: %w", path, cerr)
		}
	}()
	return scanFixture(uri, f, q), nil
}

// scanFixture reports every start tag in r matching q, in document order.
// [fixtureScan.Elems] counts the start tags seen at all, which is how the
// caller tells a fixture that broke off partway from a file that was never
// XML.
//
// A read fault comes back WITH the hits found ahead of it rather than in
// place of them: the suite ships deliberately malformed fixtures, and the
// constructs before the fault are evidence the census must keep.
//
// open is the chain of elements enclosing the token in hand, pushed and
// popped over this one stream rather than recovered by a second parse. Each
// entry also remembers where its own hit lives, which is the whole mechanism
// on the child side: the element a start tag is directly under is the top of
// the stack, so appending the name there needs no depth arithmetic and a
// match nested inside another match is not a special case. A hit is appended
// at its START tag, which is what keeps the report in document order, and its
// child list fills in as the children arrive.
//
// The containment test reads that same chain, which is why it needs no second
// pass over the fixture: each entry also remembers whether it matched the
// ancestor pattern, recorded at its own start tag because its attributes are
// gone by the time a descendant arrives, and a match qualifies when any entry
// STRICTLY outside it says yes.
//
// The pop cannot underflow: xmltree emits an end tag only for an element it
// holds open under a matching name, and any other end tag is a fault that
// returns above.
func scanFixture(uri string, r io.Reader, q query) fixtureScan {
	var scan fixtureScan
	var open []openElem
	rd := xmltree.NewReader(uri, r)
	for {
		node, err := rd.Token()
		if err != nil {
			// A match still open when the stream ends never saw its end tag,
			// so its child list stops where the read did.
			markUnclosed(scan.Hits, open)
			if !errors.Is(err, io.EOF) {
				// Kept, not wrapped: an xmltree error already names the
				// document and the position of the fault (STYLE E3), and the
				// report prints the path beside it.
				scan.Err = err
			}
			return scan
		}
		if _, ok := node.(*xmltree.EndElement); ok {
			open = open[:len(open)-1]
			continue
		}
		start, ok := node.(*xmltree.StartElement)
		if !ok {
			continue
		}
		// Both reads happen before the push, so the top of the stack is the
		// enclosing element rather than this one. Each entry is the name the
		// reader resolved in that element's OWN scope, so a parent binding its
		// prefix differently from its child still resolves correctly.
		name := qnameOf(start.Name())
		parent := innermost(open)
		recordChild(scan.Hits, open, name)
		open = append(open, openElem{Name: name, Hit: noHit, Ancestor: matchesAncestor(start, q)})
		scan.Elems++
		attrs, ok := match(start, q.pattern)
		if !ok {
			continue
		}
		ancestor, ok := enclosingMatch(open[:len(open)-1], q)
		if !ok {
			continue
		}
		loc := start.Loc()
		scan.Hits = append(scan.Hits, hit{File: uri, Line: loc.Line, Col: loc.Col, Element: name, Parent: parent, Ancestor: ancestor, Attrs: attrs})
		open[len(open)-1].Hit = len(scan.Hits) - 1
	}
}

// openElem is one element the walk is inside: the name a child of it reports
// as its parent, where its own hit lives in [fixtureScan.Hits] so that child
// can be recorded against it, and whether it matched the query's ancestor
// pattern so a match under it qualifies.
type openElem struct {
	Name xsd.QName
	Hit  int
	// Ancestor is false throughout a census that asks no containment
	// question, which is what [enclosingMatch] reads it against.
	Ancestor bool
}

// noHit is [openElem.Hit] for an element that is not itself a match, so
// nothing collects what stands under it.
const noHit = -1

// innermost is the name of the enclosing element the open chain ends with, or
// the zero QName at the document level, where there is none.
func innermost(open []openElem) xsd.QName {
	if len(open) == 0 {
		return xsd.QName{}
	}
	return open[len(open)-1].Name
}

// matchesAncestor reports whether start is an occurrence of q's ancestor
// pattern, so matches found under it qualify. A query that asks no
// containment question marks nothing: its matches stand wherever they like.
func matchesAncestor(start *xmltree.StartElement, q query) bool {
	if q.Ancestor == nil {
		return false
	}
	_, ok := match(start, *q.Ancestor)
	return ok
}

// enclosingMatch reports whether q's containment test admits a match whose
// enclosing chain is open, and names the innermost element of that chain
// which matched the ancestor pattern.
//
// open is the chain STRICTLY outside the match — an element is not its own
// ancestor, which is the whole difference between `A//A` and `A` when one
// start tag answers both patterns.
//
// A query with no ancestor pattern admits every match and names no element,
// so the hit's zero QName is the record of a census that asked no containment
// question rather than of one that found no ancestor.
func enclosingMatch(open []openElem, q query) (xsd.QName, bool) {
	if q.Ancestor == nil {
		return xsd.QName{}, true
	}
	for i := len(open) - 1; i >= 0; i-- {
		if open[i].Ancestor {
			return open[i].Name, true
		}
	}
	return xsd.QName{}, false
}

// recordChild writes name into the child list of the match the open chain
// ends with, when that element is one. It writes through hits, which the
// caller holds as [fixtureScan.Hits].
func recordChild(hits []hit, open []openElem, name xsd.QName) {
	if len(open) == 0 {
		return
	}
	at := open[len(open)-1].Hit
	if at == noHit {
		return
	}
	hits[at].Children = append(hits[at].Children, name)
}

// markUnclosed records on every match still open that its child list is a
// prefix of the real one. Every enclosing match is marked and not just the
// innermost, because the read reached none of their end tags.
func markUnclosed(hits []hit, open []openElem) {
	for _, e := range open {
		if e.Hit == noHit {
			continue
		}
		hits[e.Hit].ChildrenUnclosed = true
	}
}

// match reports whether start is an occurrence of p's construct — its
// resolved name is one of p's element names, and its attributes satisfy p's
// list under p's join — and the attributes that matched. Both a query's own
// pattern and its ancestor pattern are tested with it, which is what makes
// `A//B`'s two sides read alike (STYLE T4).
//
// An element carrying none of the attribute names a `|` query lists is not an
// occurrence, and neither is one carrying no attribute at all under `@*`: an
// attribute census has nothing to say about an element with no attribute.
// Under a value test an attribute counts as carried only when it answers the
// test, so both joins read "answers" where they otherwise read "carries".
func match(start *xmltree.StartElement, p pattern) ([]attrHit, bool) {
	if !p.matchesElement(qnameOf(start.Name())) {
		return nil, false
	}
	if p.anyAttr() {
		return axisAttrs(start, p.Attrs[0])
	}
	var got []attrHit
	for _, want := range p.Attrs {
		a, ok := attrOn(start, want, p.Value)
		if !ok && p.Join == joinAll {
			return nil, false
		}
		if !ok {
			continue
		}
		got = append(got, a)
	}
	if p.Join == joinAny && len(got) == 0 {
		return nil, false
	}
	return got, true
}

// axisAttrs is the attribute-name axis at one start tag: every attribute in
// the wildcard's namespace, in the order the tag spells them, which is the
// only order a query naming no attribute leaves.
func axisAttrs(start *xmltree.StartElement, want namePat) ([]attrHit, bool) {
	var got []attrHit
	for _, a := range start.Attributes() {
		name := qnameOf(a.Name())
		if !want.matches(name) {
			continue
		}
		got = append(got, attrHit{Name: name, Value: a.Value()})
	}
	return got, len(got) > 0
}

// attrOn returns start's want attribute as the hit records it — the name the
// document resolved, never the pattern that admitted it, which can be open on
// either axis. Attributes are tried in the order the tag spells them.
//
// With no value test the first attribute want admits wins, and its hit
// carries no resolution at all. A pattern fixing both namespace and local
// name has at most one candidate, since no start tag carries two attributes
// with one expanded name; a `{*}` or `*` pattern can admit several, and
// nothing but order tells them apart.
//
// Under an active value test the first admitted attribute that ANSWERS the
// test wins, with every token's resolution recorded: an admitted attribute
// that does not answer is skipped and the next one tried, so `{*}type` still
// finds a plain `type` after an `xsi:type` that fails the test.
func attrOn(start *xmltree.StartElement, want namePat, v valueTest) (attrHit, bool) {
	for _, a := range start.Attributes() {
		name := qnameOf(a.Name())
		if !want.matches(name) {
			continue
		}
		h := attrHit{Name: name, Value: a.Value()}
		if !v.active() {
			return h, true
		}
		h.Resolved = resolveTokens(start, h.Value)
		if slices.ContainsFunc(h.Resolved, v.admits) {
			return h, true
		}
	}
	return attrHit{}, false
}

// resolveTokens resolves every token of value as a QName at start. The value
// is split on XML whitespace whatever attribute carries it: the query, not a
// schema, asserts that the attribute holds QNames, and a scalar QName holds
// no whitespace, so it is one token. An empty value has none, so it answers
// no operand.
func resolveTokens(start *xmltree.StartElement, value string) []tokenRes {
	var res []tokenRes
	for _, tok := range strings.FieldsFunc(value, isXMLSpace) {
		res = append(res, resolveToken(start, tok))
	}
	return res
}

// resolveToken resolves one token against the bindings in scope at start
// (Datatypes §3.3.18 via [xmltree.StartElement.LookupPrefix]): an unprefixed
// token takes the default namespace, or no namespace when none is in scope,
// and is bound either way. The token splits on its FIRST colon (Namespaces in
// XML, PrefixedName / UnprefixedName); an empty prefix, an empty local part or
// a second colon makes it no QName. NCName character validity is not checked:
// this census resolves bindings and does not validate a lexical space.
func resolveToken(start *xmltree.StartElement, tok string) tokenRes {
	prefix, local, prefixed := strings.Cut(tok, ":")
	if !prefixed {
		prefix, local = "", tok
	}
	if prefixed && (prefix == "" || local == "" || strings.Contains(local, ":")) {
		return notQNameToken{}
	}
	uri, ok := start.LookupPrefix(prefix)
	if !ok {
		return unboundToken{}
	}
	return boundToken{Name: xsd.QName{Space: uri, Local: local}}
}

// isXMLSpace reports whether r is XML whitespace (#x20, #x9, #xD, #xA), the
// only characters a list value is split on. [strings.Fields] would also
// split on NBSP and the other Unicode spaces, which XSD's whiteSpace
// collapse does not treat as separators.
func isXMLSpace(r rune) bool {
	return strings.ContainsRune("\x20\t\r\n", r)
}

// qnameOf restates a name the reader resolved in the form the query language
// and the report both speak.
func qnameOf(n xmltree.Name) xsd.QName {
	return xsd.QName{Space: n.Space(), Local: n.Local()}
}

// printReport renders rep in the fixed layout the package doc describes.
// Formatting lives here, apart from [census], so the matching logic stays
// testable without string-matching rendered text.
func printReport(dst io.Writer, rep report) error {
	w := &latchWriter{w: dst}
	printReportTo(w, rep)
	if w.err != nil {
		return fmt.Errorf("writing report: %w", w.err)
	}
	return nil
}

// printReportTo does the rendering itself. Its writes go to a [latchWriter],
// which is why they are not individually checked — [suiteroot.PrintAbsent]'s
// returned error included, which is the error w latches.
func printReportTo(w io.Writer, rep report) {
	if !rep.RootPresent {
		_ = suiteroot.PrintAbsent(w, "suiteindex", rep.Root+" does not exist", "census")
		return
	}
	if rep.Walked == 0 {
		_ = suiteroot.PrintAbsent(w, "suiteindex", rep.Root+" holds no files", "census")
		return
	}

	printHeader(w, rep)

	printBody(w, rep)

	printNotes(w, "Read only partly: a match behind the fault is invisible to this census", rep.Partial)
	printNotes(w, "No XML element: the corpus's images, prose and stylesheets, and any"+
		" fixture that faulted ahead of its root", rep.NoElement)
}

// printHeader renders what the census found and what it could not read, plus
// the caveat a containment figure owes. It is the whole of what the
// paths-only mode reports beside its paths ([printPaths]), so the two modes
// cannot drift into stating the figure differently (STYLE D3).
func printHeader(w io.Writer, rep report) {
	_, _ = fmt.Fprintf(w, "suiteindex: %d occurrence(s) of %s in %d fixture(s) under %s\n",
		len(rep.Hits), rep.Query, countFiles(rep.Hits), rep.Root)
	_, _ = fmt.Fprintf(w, "  walked %d file(s): %d read to the end, %d read only partly, %d with no XML element\n",
		rep.Walked, rep.Walked-len(rep.NoElement)-len(rep.Partial), len(rep.Partial), len(rep.NoElement))
	printCaveat(w, rep.Query)
}

// printPaths renders the paths-only mode: the fixtures the census matched, one
// per line and nothing else, on stdout — a list a tool downstream reads, `go
// tool casejoin` being the one this exists for (#1642) — while the header and
// its caveat go to stderr, where a pipeline leaves them in front of the human.
//
// The per-file listings of what the census could NOT read are left to the
// plain run: their COUNTS are in the header printed here, and a reader holding
// a figure that depends on them is told to go read them there.
func printPaths(stdout, stderr io.Writer, rep report) error {
	notes := &latchWriter{w: stderr}
	if !rep.RootPresent || rep.Walked == 0 {
		printReportTo(notes, rep)
		return pathsErr(notes, nil)
	}
	printHeader(notes, rep)
	_, _ = fmt.Fprintln(notes, "  paths-only mode: read the files it could not parse whole with the same query and no -paths")

	files := &latchWriter{w: stdout}
	for _, f := range matchedFiles(rep.Hits) {
		_, _ = fmt.Fprintln(files, f)
	}
	return pathsErr(notes, files)
}

// pathsErr reports the first write failure of either destination, naming which
// one failed so a broken pipe downstream is not reported as a failure to print
// the notes.
func pathsErr(notes, files *latchWriter) error {
	if files != nil && files.err != nil {
		return fmt.Errorf("writing matched paths: %w", files.err)
	}
	if notes.err != nil {
		return fmt.Errorf("writing census notes: %w", notes.err)
	}
	return nil
}

// printCaveat states what a figure does not resolve, for each shape that
// owes a caveat: containment and the value test. The two are independent, and
// a query carrying both prints both; one carrying neither prints nothing.
//
// It sits under the header rather than in a section of its own because that
// is where the reader holding the figure arrives (#1279).
func printCaveat(w io.Writer, q query) {
	printContainmentCaveat(w, q)
	printValueCaveat(w, q)
}

// printContainmentCaveat states which directions a containment figure is
// wrong in, and prints nothing at all for a query that asked no containment
// question. The census walks the document's own nesting and resolves nothing,
// so the figure is a ceiling of a lexical population and neither a count nor
// a ceiling of the resolved-component population a reader is usually after;
// unqualified, it would be a false statement shipped in the tool's own output
// (#1585).
func printContainmentCaveat(w io.Writer, q query) {
	if q.Ancestor == nil {
		return
	}
	_, _ = fmt.Fprintln(w, "  read this as a bound from above of a LEXICAL nesting population, never as a count")
	_, _ = fmt.Fprintln(w, "  of a resolved one: it over-counts, because an ancestor in the document is not a")
	_, _ = fmt.Fprintln(w, "  particle after <element ref>/<group ref> resolution, and it MISSES a nest that only")
	_, _ = fmt.Fprintln(w, "  that resolution creates, because this census never resolves a ref")
}

// printValueCaveat states what a value-test figure does not resolve, when
// either side of the query carries a value test (#1671). The test resolves a
// value's prefix and nothing further, so the figure counts the attributes
// that NAME an operand directly, which is not the population of declarations
// whose type is that operand.
func printValueCaveat(w io.Writer, q query) {
	if !q.Value.active() && (q.Ancestor == nil || !q.Ancestor.Value.active()) {
		return
	}
	_, _ = fmt.Fprintln(w, "  read this as a bound on DIRECT references, never as a population of resolved types:")
	_, _ = fmt.Fprintln(w, "  it follows no derivation chain, so a declaration typed by a named simpleType that")
	_, _ = fmt.Fprintln(w, "  restricts an operand's type is found only by a second query on that type's name, and")
	_, _ = fmt.Fprintln(w, "  it resolves no <element ref>/<group ref>. The query, not any schema, asserts that the")
	_, _ = fmt.Fprintln(w, "  attribute holds QNames, and a token that is not a lexical QName answers no operand,")
	_, _ = fmt.Fprintln(w, "  UNBOUND included")
}

// printBody renders the half of the report the query's shape chooses: the
// attribute-name axis for a query that wildcards its attributes, the matches
// themselves for one that names them.
func printBody(w io.Writer, rep report) {
	if rep.Query.anyAttr() {
		printAxis(w, groupPairs(rep.Hits))
		return
	}
	printMatches(w, rep)
}

// printMatches renders one line per occurrence, which is the whole report for
// every query that names the attributes it looks for.
func printMatches(w io.Writer, rep report) {
	_, _ = fmt.Fprintln(w, "\n=== Matches (path order, document order within a file) ===")
	if len(rep.Hits) == 0 {
		_, _ = fmt.Fprintln(w, "(none)")
	}
	for _, h := range rep.Hits {
		_, _ = fmt.Fprintf(w, "  %s:%d:%d %sparent=%s %schildren=%s%s\n",
			h.File, h.Line, h.Col, renderMatched(rep.Query, h), renderName(h.Parent),
			renderAncestor(rep.Query, h), renderChildren(h), renderAttrs(h.Attrs))
	}
}

// printAxis renders the attribute-name axis in its two sections: the pairs
// with their counts, then each pair's fixtures. The counts come first because
// that section is the one that fits on a screen; the listing under it runs to
// a line per (pair, fixture), 101,897 of them over the suite (#1391).
func printAxis(w io.Writer, groups []pairGroup) {
	occurrences := 0
	for _, g := range groups {
		occurrences += g.Occurrences
	}
	_, _ = fmt.Fprintf(w, "\n=== Attribute-name axis: %d (element, attribute) pair(s), %d attribute occurrence(s) ===\n",
		len(groups), occurrences)
	if len(groups) == 0 {
		_, _ = fmt.Fprintln(w, "(none)")
	}
	for _, g := range groups {
		_, _ = fmt.Fprintf(w, "  %s — %d occurrence(s) in %d fixture(s)\n",
			renderPair(g), g.Occurrences, len(g.Fixtures))
	}

	_, _ = fmt.Fprintln(w, "\n=== Fixtures per pair (path order) ===")
	if len(groups) == 0 {
		_, _ = fmt.Fprintln(w, "(none)")
	}
	for _, g := range groups {
		_, _ = fmt.Fprintf(w, "  %s\n", renderPair(g))
		for _, f := range g.Fixtures {
			_, _ = fmt.Fprintf(w, "      %s\n", f)
		}
	}
}

// printNotes renders one group of unread files under its heading. Both groups
// are listed in full: what a census could not read is the reader's to judge,
// and counting it instead is how a corpus survey under-reports (#1239).
func printNotes(w io.Writer, heading string, notes []fileNote) {
	_, _ = fmt.Fprintf(w, "\n=== %s ===\n", heading)
	if len(notes) == 0 {
		_, _ = fmt.Fprintln(w, "(none)")
	}
	for _, n := range notes {
		if n.Err == nil {
			_, _ = fmt.Fprintf(w, "  %s — read to the end, no element in it\n", n.File)
			continue
		}
		_, _ = fmt.Fprintf(w, "  %s — %v\n", n.File, n.Err)
	}
}

// countFiles reports how many distinct fixtures the hits fall in. It counts
// the list [matchedFiles] names rather than counting separately, so the header
// figure and the paths-only mode's output can never disagree (STYLE D3).
func countFiles(hits []hit) int {
	return len(matchedFiles(hits))
}

// matchedFiles names the distinct fixtures the hits fall in, in path order.
// Hits arrive path-sorted, so distinctness is a neighbour comparison and needs
// no set.
func matchedFiles(hits []hit) []string {
	var files []string
	for i, h := range hits {
		if i == 0 || h.File != files[len(files)-1] {
			files = append(files, h.File)
		}
	}
	return files
}

// renderIn spells n for a query position whose braceless names mean
// defaultSpace: Clark notation, except that a name in NO namespace printed
// where braceless means something else carries the empty wrapper `{}` that
// [splitName] already reads as no namespace. Without it the report's `root`
// and a query's `root` are two different elements, since Clark notation
// leaves a no-namespace name bare and an element position of a query reads
// bare as the XML Schema namespace (#1297).
func renderIn(n xsd.QName, defaultSpace string) string {
	if n.Space == "" && defaultSpace != "" {
		return "{}" + n.Local
	}
	return n.String()
}

// renderName names one element QName — a hit's parent, one of its children,
// the element a wildcard query matched, or an axis pair's element half — in
// the spelling an element position of a query takes, so the name re-enters
// as a query naming it.
//
// The zero QName is the parent of a document element, which has none; a child
// is never zero. "(none)" can never collide with a name: parentheses are not
// NCName characters.
func renderName(n xsd.QName) string {
	if n.Local == "" {
		return "(none)"
	}
	return renderIn(n, elementSpace)
}

// renderChildren lists the elements directly under a hit, in document order
// and with repeats kept, as "[a, b]" — "[]" for a match with no element
// child. A list ending in "(unclosed)" is one whose read stopped before the
// match's end tag, so the names ahead of the marker are a prefix; the marker
// cannot be read as a name for the same reason "(none)" cannot.
func renderChildren(h hit) string {
	names := make([]string, 0, len(h.Children)+1)
	for _, c := range h.Children {
		names = append(names, renderName(c))
	}
	if h.ChildrenUnclosed {
		names = append(names, "(unclosed)")
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// renderAttrs formats the attributes a hit matched, each named as the hit
// records it. The local part alone is what a reader recognizes, and no
// ambiguity follows it: a query naming two attributes of one local name in
// different namespaces is not one anybody writes.
//
// An attribute a value test resolved is followed, with no space, by
// `->[r1, r2, …]`: one resolution per token, in token order ([renderToken]).
// One without a resolution prints nothing more, so a query with no value test
// renders the bytes it always did.
func renderAttrs(attrs []attrHit) string {
	var b strings.Builder
	for _, a := range attrs {
		fmt.Fprintf(&b, " %s=%q", a.Name.Local, a.Value)
		if a.Resolved == nil {
			continue
		}
		toks := make([]string, 0, len(a.Resolved))
		for _, r := range a.Resolved {
			toks = append(toks, renderToken(r))
		}
		b.WriteString("->[" + strings.Join(toks, ", ") + "]")
	}
	return b.String()
}

// renderToken spells one token's resolution: a bound token as the braced
// operand that matches it exactly ([bracedName]), so it re-enters as an
// operand; an unbound one as [unboundOperand]; and one that is no QName as
// "(not a QName)", which cannot be read as a name for the same reason
// "(none)" cannot.
//
// The switch is exhaustive over the sealed sum; the default arm asserts the
// invariant and is unreachable, since [tokenRes] is unexported and this file
// declares every arm.
func renderToken(r tokenRes) string {
	switch r := r.(type) {
	case boundToken:
		return bracedName(r.Name.Space, r.Name.Local)
	case unboundToken:
		return unboundOperand
	case notQNameToken:
		return "(not a QName)"
	default:
		panic("suiteindex: renderToken: non-exhaustive tokenRes switch")
	}
}

// renderMatched names the element a hit matched, for a query that does not
// already fix that name — a wildcard local part, the `{*}` namespace axis, or
// more than one element alternative — and nothing at all for one that fixes
// it, since the report's own header echoes the query. Without this a query
// whose element is open prints every element's hits as one undifferentiated
// run of lines (#1495, #1554). The trailing space belongs to the field, so
// the line closes up when there is none.
func renderMatched(q query, h hit) string {
	if !q.elementOpen() {
		return ""
	}
	return "element=" + renderName(h.Element) + " "
}

// renderAncestor names the enclosing element that qualified a hit, for a
// containment query and for no other: a census that asked no containment
// question has nothing to say in this field, and its absence is what leaves
// every other query's report the bytes it always was (#1585). The trailing
// space belongs to the field, so the line closes up when there is none.
func renderAncestor(q query, h hit) string {
	if q.Ancestor == nil {
		return ""
	}
	return "ancestor=" + renderName(h.Ancestor) + " "
}

// renderPair names one axis pair in the form the two axis sections share, so
// a pair's heading in the second is greppable from its line in the first.
// Each half is spelled for its own position: an attribute in no namespace
// stays bare, because that is already what an attribute position of a query
// means by a bare name.
func renderPair(g pairGroup) string {
	return renderName(g.Element) + " @" + renderIn(g.Attr, attrSpace)
}

// latchWriter is an [io.Writer] that remembers its first failure and drops
// every write after it. A report is a long run of small writes to one
// destination, so checking each in place would bury the layout in error
// handling; the caller checks err once, at the end.
type latchWriter struct {
	w   io.Writer
	err error
}

func (l *latchWriter) Write(p []byte) (int, error) {
	if l.err != nil {
		return 0, l.err
	}
	n, err := l.w.Write(p)
	l.err = err
	return n, err
}
