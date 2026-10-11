package xpath

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// This file evaluates the RESTRICTED expression subset a Type Alternative's
// {test} is written in — the "Test XPath expressions" grammar ta-props-correct
// clause 2.1 (§3.12.6) fixes, productions [8] ta-Test through [18]
// ta-ConstructorFunction — and nothing wider, but for the two productions the
// assertion façade adds and the Type Alternative one declines: [11]'s
// Comparator position also takes xpath20.md [23] ValueComp ('eq' | 'ne' | 'lt'
// | 'le' | 'gt' | 'ge'), evaluated as §3.5.1's value comparison
// (ctaValueCompare), and [16] SimpleValue also takes [44] VarRef ('$' VarName),
// of which only `$value` is in scope (cvc-assertion clause 2.2), a child-axis
// step naming one of E's element [[children]] (ctaTypedChild, or ctaUntypedChild for a
// child of mixed content), and a "/" or "//" opening a path, which raises (ctaNoDocumentRoot);
// the whole operand of fn:exists, fn:empty or an ·effective boolean value· also takes a
// relative path of two or more such steps, the last of which may be `*` (ctaChildPath), one
// element step `N`, `./N` or `.//N` whatever N's type (ctaSelectedElements), one child step
// filtered by a conjunction of child steps, `N[a and b]` (ctaChildrenHaving), and one child
// step filtered by one `preceding::` step, `N[preceding::M[not(P)]]` (ctaChildrenPreceded); an
// ExprSingle standing whole in a boolean position also takes [6] QuantifiedExpr over one child
// step, whose body tests the bound child's attributes or its next sibling element
// (ctaQuantified), or over one typed child step, filtered by `[position() lt last()]` or not,
// whose body is any ExprSingle over its range variable, a node, and
// `$v/following-sibling::M[1]` (ctaQuantifiedChildren, ctaBoundChild, ctaBoundSibling), or
// over the items of a typed `$value`, whose body is any ExprSingle over its range variable
// (ctaQuantifiedValue, ctaRangeItem); and [14] ValueExpr also takes an fn:count call over one
// counted path (ctaCount) — a child step in it filtered by one [40] Predicate testing
// attribute existence (ctaFilteredChildren) or comparing the child's value
// (ctaMatchingChildren, whose `.` is ctaCandidate), and two or more such paths joined by [21]
// UnionExpr (ctaUnion) — or over an operand that is no path, such as `$value`, whose items it
// counts (ctaCountedItems), and a call to one of the F&O string and sequence functions or to
// fn:namespace-uri over E (ctaNamespaceURI), evaluated in ctafunc.go, and [18]'s operand any
// argument of such a call (ctaParser.constructorOperand), and [11] ta-BooleanExpr a comparison
// by `=` of fn:in-scope-prefixes over E against string literals (ctaPrefixMember), whose `.`
// is E as a node (ctaContextNode); and each comparison operand may be xpath20.md [13]
// AdditiveExpr over [14] MultiplicativeExpr, whose operators are evaluated in ctaarith.go
// (ctaArith); [47] ContextItemExpr `.`, atomized to E's string value (ctaContextAtom);
// xpath20.md [18] CastableExpr's `castable as` tail in place of [15]'s `cast as` one
// (ctaCastable); xpath20.md [16] InstanceofExpr's `instance of` tail with an atomic
// SequenceType over a [14] ta-ValueExpr or an fn:data call (ctaInstanceOf); and a general
// comparison's operand may be an integer sequence, xpath20.md [11] RangeExpr or §3.3.1's comma
// sequence over IntegerLiterals, or a string sequence, §3.3.1's comma sequence over
// StringLiterals, evaluated in ctasequence.go (ctaIntegerRanges, ctaStringSequence). The facet
// façade (ctaFacetFacade) takes the assertion façade's grammar but fn:count over a path, and
// compiles every read of the context item — `.`, an attribute or child step, a rooted path —
// to the err:XPDY0002 an assertions facet's absent context item raises (ctaNoContextItem;
// ctaAbsentNode for `.` as a node). It is not a stage of a general XPath 2.0 evaluator: the
// productions below reach no axis but attribute, one child step, the child-step paths and the
// one descendant step whose existence is asked, the descendant steps fn:count counts over, the
// preceding step of ctaChildrenPreceded and the child step filtering it, and the
// following-sibling and self steps of a quantifier's body and the following-sibling step over
// its range variable, no predicate or union but those in an fn:count argument, the
// child-existence conjunction, the two of ctaChildrenPreceded, the two of that body, the `[1]`
// of that following-sibling step and the `[position() lt last()]` filtering a quantifier's
// binding step, no variable but `$value` and a quantifier's range variable inside its body,
// and no function but fn:not, fn:count, the ctaParser.libraryCall names, fn:in-scope-prefixes
// as an operand of `=` (ctaParser.prefixMember) and fn:data as the operand of `instance of`
// (ctaParser.instanceofExpr) or over `$value` as a quantifier's binding sequence
// (ctaParser.valueBinding), so evaluating them directly is exact where a fail-open delegation
// to a general engine would be a guess.
//
//	[8]  Test                ::= OrExpr
//	[9]  OrExpr              ::= AndExpr ( 'or' AndExpr )*
//	[10] AndExpr             ::= BooleanExpr ( 'and' BooleanExpr )*
//	[11] BooleanExpr         ::= '(' OrExpr ')' | BooleanFunction |
//	                             ValueExpr ( Comparator ValueExpr )?
//	[12] BooleanFunction     ::= QName '(' OrExpr ')'
//	[13] Comparator          ::= '=' | '!=' | '<' | '<=' | '>' | '>='
//	[14] ValueExpr           ::= CastExpr | ConstructorFunction
//	[15] CastExpr            ::= SimpleValue ( 'cast' 'as' QName '?'? )?
//	[16] SimpleValue         ::= AttrName | Literal
//	[17] AttrName            ::= '@' NameTest
//	[18] ConstructorFunction ::= QName '(' SimpleValue ')'
//
// THREE ERROR DIRECTIONS MEET HERE AND NO TWO OF THEM ARE ONE.
//
// A test the engine cannot evaluate is declined at COMPILE time
// ([CompileCTATest] reporting false), and its consequence — the caller
// withholding the element's ·governing type definition· — is argued at that
// caller (validate/cta.go). A dynamic or type error inside a test the engine
// CAN evaluate is a decided false, because key-cta-ta-select clause 2
// (§3.12.4) says so: "If a dynamic error or a type error is raised during
// evaluation, then the {test} is treated as if it had evaluated (without
// error) to false." That is spec-normative behavior and not this module's
// fail-open contract, so it carries no GAP marker and never reaches the
// caller as a decline. An XPath STATIC error is neither: it is a
// ta-props-correct clause 2 (§3.12.6) violation the assembler charges before
// any instance exists, reported by [CTATestStaticError] and by nothing else in
// this package.
//
// The nodes of the XDM instance a {test} runs against are UNTYPED
// (key-cta-ta-select clause 1's Note), so an UNCAST attribute operand
// atomizes to xs:untypedAtomic and xpath20.md §3.5.2's untypedAtomic casting
// rules are the normal path of every comparison here rather than an edge
// case. The Note also names the remedy the grammar gives a test that needs
// type information — "explicit casts will sometimes be necessary" — which is
// [15]'s cast tail and [18]'s constructor function, both evaluated here.
//
// EVERY CAST IS A DATATYPE VALIDATION. xpath-functions.md §17.1.1 fixes that
// outright for an xs:string or xs:untypedAtomic operand: "Whitespace
// normalization is applied as indicated by the whiteSpace facet for the
// datatype. The resulting whitespace-normalized string must be a valid
// lexical form for the datatype. The semantics of casting are identical to
// XML Schema validation." So a cast is [value.ValidateLexical] — the whole
// facet pipeline, whiteSpace and pattern and value facets included — and
// never a lexical parser of this package's own (STYLE T4).

// Attributes yields the attributes of the element information item E whose
// ·selected type definition· is being decided — §3.12.4 clause 1.1.2's copy of
// E.[[attributes]], plus clause 1.1.3's non-clashing [[inherited attributes]] —
// each as its ·expanded name· and its [[normalized value]], in DOCUMENT ORDER.
// It must be non-nil, and a yield reporting false ends the walk.
//
// It is the ONE way this package reads an attribute, whatever NameTest [17]
// ta-AttrName carries: a QName NameTest filters this sequence to the one
// ·expanded name· it names, and each [37] Wildcard arm filters it its own way,
// so no second entry point encodes "read an attribute" a second time (STYLE
// D4).
//
// The sequence a NameTest matches is the operand xpath20.md §3.5.2 quantifies
// a general comparison over: true only when "there is a pair of atomic values,
// one in the first operand sequence and the other in the second", so a
// comparison matching NO attribute is false; §3.10.2 rule 3 makes that same
// empty sequence a type error (err:XPTY0004) under a `cast as T` written
// without the `?` occurrence indicator, and a matched sequence of two or more
// is err:XPTY0004 under any cast at all. That is neither a decline nor an error
// to the caller in any of those cases.
//
// The lexical is the attribute's [[normalized value]]: XML 1.0 §3.3.3
// attribute-value normalization applied, whiteSpace-facet normalization NOT —
// the latter belongs to the type a cast targets, and [CTATest.Evaluate]
// applies it per target type.
type Attributes func(yield func(name xsd.QName, lexical string) bool)

// CTATest is a compiled Type Alternative {test}: the §3.12.6 required-subset
// expression tree [CompileCTATest] admitted, ready to be evaluated against any
// number of element information items.
type CTATest struct{ root ctaExpr }

// CompileCTATest compiles the {test} of a Type Alternative (§3.12.1, an
// [xsd.XPathExpression] property record) into an evaluable [CTATest],
// reporting ok false for an {expression} this engine cannot evaluate.
//
// types resolves every QName the expression names a datatype with — [15]
// ta-CastExpr's target, [18] ta-ConstructorFunction's function name, and the
// builtin types §3.5.2's casting rules answer with — and is read here and
// stored nowhere: the compiled tree holds the resolved *[xsd.SimpleType]
// components themselves, which is why [CTATest.Evaluate] takes a resolver
// again rather than reading one off the tree.
//
// ok false is the WITHHOLD direction and covers every such {expression} under
// one encoding, because they all have one consequence at the caller:
//
//   - text that is not a [8] ta-Test production at all. A legal full XPath 2.0
//     {test} looks exactly like this to a restricted-subset parser, and
//     ta-props-correct and xpath-valid (§3.13.6.2) are SCHEMA component
//     constraints charged at assembly by their owner, which reads this module
//     through [CTATestStaticError] and never through this entry point — so
//     nothing upstream has narrowed what arrives here and an error return would
//     be a false reject of every schema using full XPath in conditional type
//     assignment;
//   - a prefix with no binding in the record's {namespace bindings}
//     (err:XPST0081), which is a STATIC error and so not clause 2's false;
//   - a comparison whose two operands need a type promotion this engine cannot
//     perform, which today is exactly B.1 rule 1.1's xs:float to xs:double
//     (ctaWider). Declining is what keeps the withhold and the DECIDED false
//     apart: a promotion that cannot be performed is not err:XPTY0004 and must
//     not become one, because that error is a decided false the caller reads as
//     "this alternative did not select";
//   - a cast target this engine does not cast to, which is TWO conditions
//     sharing one encoding deliberately. A target naming an in-scope atomic
//     type outside the XSD namespace is valid XPath outside §3.12.6's
//     required subset, which that section's own Note licenses a processor to
//     decline ("Conforming processors may but are not required to support
//     XPath expressions not belonging to the required subset of XPath"); a
//     target that resolves to nothing, to a complex type, to a non-atomic
//     type or to xs:anyAtomicType/xs:NOTATION is err:XPST0051/err:XPST0080, a
//     STATIC error and so an xpath-valid clause 2 failure. The encoding
//     tracks the CONSEQUENCE, and both consequences are the one withhold
//     validate/cta.go's conditionallySelected argues: the element's
//     ·governing type definition· is not determined, and no type is assessed
//     against in its place;
//   - a cast of a numeric literal that F&O §17 does not define over its
//     ·canonical representation·, the one cast this façade declines for its
//     OPERAND (ctaTypes.castsFrom, literalCastsTo): a DoubleLiteral to any
//     target but xs:double, such as `xs:string(1.5e0)`, and an IntegerLiteral
//     or a DecimalLiteral to a target outside the string family, xs:float,
//     xs:double, xs:decimal and — for an IntegerLiteral — xs:integer's
//     branch, such as `xs:integer(1.5)` or `xs:boolean(2)`. To a target F&O
//     §17.1's casting table marks Y or M from the literal's type, §17.1.2,
//     §17.1.3 and §17.1.6 define the cast over the value, not over the
//     canonical lexical this engine would re-validate, and it is valid XPath
//     with a defined result; to a target it marks N, such as
//     `xs:date(1.5e0)` or `xs:anyURI(1.5)`, §17.1 raises err:XPTY0004, a type
//     error, which this façade declines too rather than raising — a withhold,
//     as the defined result's decline is.
//
// It never returns an error: a decline is not a verdict about the schema. The
// verdict the STATIC errors two of those bullets name does carry is
// [CTATestStaticError]'s to report, over the same traversal and disagreeing
// with nothing here — this entry point still withholds them, because a tree
// built over an unresolved name or an unadmitted cast target is not evaluable
// whatever the schema's fate. Of the two, only the unbound prefix is reported
// today; the cast-target conditions are folded into this withhold and reach no
// charge, under the marker ctaTypes.castTarget carries.
//
// The static context is xpath-valid clause 2.2's: XPath 1.0 compatibility mode
// false (2.2.1), statically known namespaces from expr.NamespaceBindings()
// (2.2.2), the default element/type namespace from expr.DefaultNamespace()
// (2.2.3) and the default function namespace
// http://www.w3.org/2005/xpath-functions (2.2.4). The default element/type
// namespace is read only where [xsd.XPathExpression.DefaultNamespace] reports
// it present, because that accessor's own doc makes the first result "not
// meaningful" otherwise; an ABSENT one leaves ctaNames.defaultNamespace the
// empty string, which is the no-namespace answer §3.10.2 wants. A Type
// Alternative's {test} consults it for an unprefixed cast TARGET and for
// nothing else (xpath20.md §3.10.2: "If the target type has no namespace
// prefix, it is considered to be in the default element/type namespace"): [17]
// ta-AttrName makes every NameTest its grammar reaches an attribute-axis one,
// whose principal node kind is never element, so an unprefixed NameTest is
// always in no namespace (xpath20.md §3.2.1.2, PRINCIPLES 15), and a child-axis
// step, whose NameTest would read it, is the assertion façade's and declined
// here (ctaFacade.child).
func CompileCTATest(expr xsd.XPathExpression, types xsd.TypeResolver) (CTATest, bool) {
	root, defect := compileCTATest(expr, types, ctaTypeAlternativeFacade{})
	if defect.kind != ctaNoDefect {
		return CTATest{}, false
	}
	return CTATest{root: root}, true
}

// ruleXPST0081 is the XPath static error an unbound namespace prefix is
// (xpath20.md Appendix G: "It is a static error if a QName used in an
// expression contains a namespace prefix that cannot be expanded into a
// namespace URI by using the statically known namespaces"), carried as the
// [xsderr.Rule] of the error [CTATestStaticError] returns.
//
// The CHARGE is the assembler's and the CODE is this package's (STYLE E2): a
// consumer reads this rule off the wrapped cause with [xsderr.RuleOf], never
// off a message it scraped, and reads the schema-side verdict —
// ta-props-correct — off the wrapper the assembler minted over it.
const ruleXPST0081 xsderr.Rule = "err:XPST0081"

// CTATestStaticError reports the XPath STATIC error the {test} of a Type
// Alternative carries, and nil for every other {expression} — including one
// this engine merely cannot evaluate.
//
// It answers the one question ta-props-correct clause 2 (§3.12.6) asks of a
// {test}: whether it "satisfies the constraint XPath Valid (§3.13.6.2)", whose
// clause 2 is "X does not produce any static error". That is a Schema Component
// Constraint, decided when the component is assembled and independent of
// whether any instance is ever ·assessed·, so the charge belongs to the
// assembler: it mints ta-props-correct at the <alternative>'s location and
// wraps what this returns as the cause. The CODE is this package's, because
// err:XPST0081 is XPath's static error and not the assembler's rule, so the
// result is an *[xsderr.Error] carrying it (ruleXPST0081) and a consumer reads
// it with [xsderr.RuleOf] after one errors.Unwrap of the charge.
//
// UNSUPPORTED DOMINATES STATIC. A static error is reported only where the token
// stream parsed to its end as a complete [8] ta-Test production and name
// resolution was the sole defect. `count(//p:a) > 1` with p unbound is legal
// full XPath 2.0 that this engine declines under §3.12.6 clause 2's Note
// ("Conforming processors may but are not required to support XPath
// expressions not belonging to the required subset of XPath"), and reading its
// unbound prefix in isolation would turn that decline into the rejection of a
// conforming schema. Under-charging is a rejection this engine can take later;
// over-charging is a false reject now.
//
// types is the IN-SCOPE SCHEMA DEFINITIONS of the static context this question
// is asked in, which xpath-valid clause 2.2.5 fixes as "those components that
// are present in every schema by definition" — the built-ins of §3.2.7, §3.4.7
// and §3.16.7, and NOT the schema's own {type definitions}, which that clause
// does not put in scope. It is read exactly as [CompileCTATest] reads it and
// stored nowhere.
//
// The rule is the XPath error code and the message is the fact, so the result
// renders as
//
//	?: [err:XPST0081] no statically-known namespace binding for prefix "p"
//
// naming the FIRST unbound prefix in expression order (STYLE D2). The
// [xsderr.Loc] is the zero one, which renders as that `?`: this package reads
// no schema document and so has no position to report — the real one is the
// <alternative>'s, attached by the assembler that charges over this.
func CTATestStaticError(expr xsd.XPathExpression, types xsd.TypeResolver) error {
	_, defect := compileCTATest(expr, types, ctaTypeAlternativeFacade{})
	if defect.kind != ctaStaticError {
		// An untyped nil, never a nil *xsderr.Error in an error interface: a
		// caller's `!= nil` must mean what it says.
		return nil
	}
	return defect.static
}

// compileCTATest is the ONE traversal the entry points above and
// [CompileAssertionTest] are façades over (STYLE T4): it tokenizes the
// {expression}, resolves the builtin datatypes the compiler names, indexes the
// {namespace bindings} and parses [8] ta-Test, reporting the tree and what — if
// anything — was wrong with it.
//
// facade is the only thing the façades differ in: what one [17] ta-AttrName
// compiles to, and which settled comparison types they evaluate —
// ctaTypeAlternativeFacade for a Type Alternative, whose instance is untyped,
// and ctaAssertionFacade for an assertion, whose attributes are typed.
//
// Name resolution never fails a parse — it records and carries on — so a parse
// that failed at all failed for another reason, and is ctaUnsupported with
// whatever an unbound prefix recorded along the way DISCARDED. That is where
// "unsupported dominates static" is enforced, once, for every façade.
func compileCTATest(expr xsd.XPathExpression, types xsd.TypeResolver, facade ctaFacade) (ctaExpr, ctaDefect) {
	toks, ok := ctaTokenize(expr.Expression())
	if !ok {
		return nil, ctaDefect{kind: ctaUnsupported}
	}
	known, ok := ctaResolveTypes(types)
	if !ok {
		return nil, ctaDefect{kind: ctaUnsupported}
	}
	names := ctaNames{prefixes: make(map[string]string)}
	for _, b := range expr.NamespaceBindings() {
		names.prefixes[b.Prefix()] = b.Namespace()
	}
	if defaultNS, present := expr.DefaultNamespace(); present {
		names.defaultNamespace = defaultNS
	}
	p := ctaParser{toks: toks, names: names, types: known, facade: facade}
	root, ok := p.test()
	if !ok {
		return nil, ctaDefect{kind: ctaUnsupported}
	}
	return root, p.defect
}

// ctaDefect is what compileCTATest found wrong with an {expression}. The three
// states are sealed inside this package and none of them crosses the boundary:
// each exported entry point answers about ONE of them, so no caller ever
// classifies a defect and no caller can mistake a decline for a verdict.
//
// The zero value is "nothing wrong", which is the only state that yields an
// evaluable tree.
type ctaDefect struct {
	kind ctaDefectKind
	// static is the verdict of a ctaStaticError defect, carrying the XPath error
	// code it violates as its [xsderr.Rule] — the error [CTATestStaticError]
	// hands the assembler to wrap. The other kinds leave it nil because they
	// have nothing to say: an {expression} outside what this engine evaluates is
	// not a verdict about the schema, so there is no fact to report about it.
	static *xsderr.Error
}

// ctaDefectKind is the kind of a [ctaDefect].
type ctaDefectKind byte

const (
	// ctaNoDefect is a complete [8] ta-Test production this engine evaluates.
	ctaNoDefect ctaDefectKind = iota
	// ctaUnsupported is an {expression} this engine cannot evaluate — full XPath
	// 2.0 outside the required subset, or one of the subset constructs whose own
	// production declines it. It is the fail-open direction (PRINCIPLES 20) and
	// never a schema fault.
	ctaUnsupported
	// ctaStaticError is a complete ta-Test production carrying an XPath static
	// error, which xpath-valid clause 2 (§3.13.6.2) forbids outright.
	ctaStaticError
)

// Evaluate reports whether the compiled {test} holds for the element
// information item whose attributes attrs yields, which is the whole of what
// key-cta-ta-select (§3.12.4) asks of a {test}: "A.{test} evaluates to true".
//
// It always decides. Every way an evaluation can go wrong inside the compiled
// subset — a cast that fails (err:FORG0001), an absent operand under a cast
// written without `?` or operand types the operator does not accept
// (err:XPTY0004) — is a dynamic or type error, which clause 2 makes a false
// rather than an error or a decline. The declines all happened at
// [CompileCTATest] but one.
//
// GAP(xpath): an assertions facet on a cast's target is declined at evaluation
// (ctaValidate), and the decline reads as false like an error, where clause 2
// makes only an error false (#2533).
//
// Clause 2's subject is "the {test}", not the sub-expression that raised, so
// this is the ONE place the substitution happens: a raised error travels up
// the expression tree under XPath's own rules for each operator and becomes
// false here, at the root. An error under an fn:not is therefore a false
// {test} and never the inversion of one.
//
// b supplies the value spaces the comparisons and the casts run in and must be
// non-nil. types resolves the {base type definition} chain of every type the
// compiled tree holds — the same capability [CompileCTATest] classified those
// types with, threaded as a parameter and stored nowhere (ARCHITECTURE), so
// one compiled [CTATest] serves any resolver that answers for its components.
func (t CTATest) Evaluate(b value.Backend, types xsd.TypeResolver, attrs Attributes) bool {
	return ctaEval(t.root, ctaEnv{backend: b, types: types, input: ctaLexicalInput{attrs: attrs}}) == ctaTrue
}

// ctaEnv is the dynamic context of one [CTATest.Evaluate] or
// [AssertionTest.Evaluate] call. cvc-xpath (§3.13.4.2) fixes the rest of it —
// context item E, context position and size 1, no variable values but the
// `$value` cvc-assertion clause 2.3 adds — and of that only `$value`, the
// context item's string value, its own attributes and element [[children]],
// and the counts of the nodes of its subtree fn:count selects are reachable in
// this grammar, beside the current dateTime fn:current-date reads (xpath20.md
// §2.1.2), so the attributes, the children, the counts, the [ValueBinding],
// the instant, the value spaces and the type knowledge the casts need are the
// whole of what evaluation reads. A facet {test} [FacetAssertions] evaluates
// has no context item at all (cvc-assertions-valid clause 1.2), so it reads
// nothing of its input but `$value` and the instant.
//
// candidate is the typed value of the context item inside a predicate — the
// child ctaMatchingChildren.nodes evaluates its predicate for, one value or
// none for a ·nilled· child — which only a ctaCandidate reads, and which only
// a predicate's tree holds.
//
// rangeItem is the item a quantifier over `$value` binds its range variable to
// — the one item of the binding sequence ctaQuantifiedValue.eval evaluates its
// body for — which only a ctaRangeItem reads, and which only such a body's
// tree holds.
//
// rangeChild is the child a quantifier over a typed child step binds its range
// variable to (ctaQuantifiedChildren.eval), which only a ctaBoundChild and a
// ctaBoundSibling read, and which only such a body's tree holds. It is a field
// of its own and not an arm of one sum with rangeItem: a body binds one, and
// the scopes that could nest the two decline (ctaChildRangeFacade,
// ctaRangeFacade), but nothing makes them exclusive.
type ctaEnv struct {
	backend    value.Backend
	types      xsd.TypeResolver
	input      ctaInput
	candidate  []value.Value
	rangeItem  value.Value
	rangeChild ctaChildBinding
}

// ctaChildBinding is one binding of a quantifier over a typed child step:
// children[at], children being every child [ChildElements] yielded for the
// evaluation, in document order, read once per evaluation of the quantifier
// (ctaQuantifiedChildren.eval) and never kept beyond it. The zero value binds
// nothing, which only a tree outside such a body carries, and which reads no
// child.
type ctaChildBinding struct {
	children []ChildElement
	at       int
}

// bound is the bound child, as the one-child sequence `$v` is, or none for the
// zero value.
func (b ctaChildBinding) bound() []ChildElement {
	if b.at >= len(b.children) {
		return nil
	}
	return b.children[b.at : b.at+1]
}

// next is `$v/following-sibling::M[1]` for M name: the first child after the
// bound one named name, in document order, whatever children of other names
// lie between (xpath20.md §3.2.1.1, §3.2.2), or none where no later child is
// so named.
func (b ctaChildBinding) next(name xsd.QName) []ChildElement {
	for i := b.at + 1; i < len(b.children); i++ {
		if b.children[i].name == name {
			return b.children[i : i+1]
		}
	}
	return nil
}

// ctaInput is the sealed sum of the two attribute inputs an evaluation reads,
// one per façade: E's attributes as LEXICALS for a Type Alternative, whose
// instance is untyped (key-cta-ta-select clause 1's Note), and as TYPED values
// for an assertion (cvc-assertion clause 1, §3.13.4.1). The two are one field
// and not two nil-able ones, so no environment carries both.
//
// Each façade pairs its own tree with its own input: [CTATest.Evaluate] builds
// ctaLexicalInput over a tree of ctaAttr nodes, and [AssertionTest.Evaluate]
// builds ctaTypedInput over a tree of ctaTypedAttr nodes and of ctaAttr nodes
// for the attributes whose type is ·special·, so a ctaTypedAttr never meets a
// lexical input. A ctaAttr reads either: every lexical a ctaLexicalInput
// yields, or each [Untyped] value a ctaTypedInput yields, whose other arm is a
// breach of [TypedAttributes]' obligation that the node raises on
// (ctaMatchedAttributes).
//
// Each arm answers facets, the [value.AssertionEvaluator] a cast it makes
// validates an assertions facet with (ctaValidate).
type ctaInput interface {
	ctaInput()
	facets() value.AssertionEvaluator
}

// ctaLexicalInput is a Type Alternative's attribute input.
type ctaLexicalInput struct{ attrs Attributes }

// ctaTypedInput is an assertion's input: E itself as the functions taking `.`
// as a node read it ([ContextElement], ctaContextElementOf), its typed
// attributes, its element [[children]], the counts of the nodes its fn:count
// calls select, E's string value and the value cvc-assertion clause 2.3 binds
// to `$value` ([ValueBinding]), and now, the dynamic context's current dateTime
// (xpath20.md §2.1.2) fn:current-date reads (ctaCurrentDate). The element, the
// children, the counts, the binding and the instant live here and on no other
// arm, so a Type Alternative's evaluation cannot carry any of them. counts is
// nil where the tree counts nothing, which a facet evaluation's never does,
// and node is nil in a facet evaluation, whose tree compiles `.` to
// ctaNoContextItem, and to ctaAbsentNode as a node, and so never reads it.
type ctaTypedInput struct {
	node     ContextElement
	attrs    TypedAttributes
	children ChildElements
	counts   *Tally
	value    ValueBinding
	now      time.Time
}

func (ctaLexicalInput) ctaInput() {}
func (ctaTypedInput) ctaInput()   {}

// facets is ctaAssertionsDeclined: the input holds no current dateTime.
func (ctaLexicalInput) facets() value.AssertionEvaluator { return ctaAssertionsDeclined{} }

// facets is [FacetAssertions] at the evaluation's own instant, now.
func (in ctaTypedInput) facets() value.AssertionEvaluator { return FacetAssertions(in.now) }

// ctaExpr is the sealed sum of the BOOLEAN-valued nodes of the compiled tree.
// The grammar closes the set (STYLE T2's schema-closed-set exception), so
// consumers type-switch over the branches and no further branch is
// representable outside this package. Every branch answers readsChild
// ([AssertionTest.ReadsChild]) and counted ([AssertionTest.Tally]) as methods,
// so a branch added without them does not compile.
type ctaExpr interface {
	ctaExpr()
	readsChild(name xsd.QName) bool
	// counted appends to into each path an fn:count call in the node counts, at
	// any depth, in written order, and returns the extended slice.
	counted(into []ctaKey) []ctaKey
}

// ctaOr is [9] ta-OrExpr: existential over its operands, in written order.
// A one-operand OrExpr is never built — the parser returns the operand itself
// — so an ctaOr always holds two or more.
type ctaOr struct{ operands []ctaExpr }

// ctaAnd is [10] ta-AndExpr, universal over its operands on ctaOr's terms.
type ctaAnd struct{ operands []ctaExpr }

// ctaNot is the fn:not call [12] ta-BooleanFunction is fixed to by §3.12.6
// clause 3 ("Any strings matching the BooleanFunction production are function
// calls to fn:not").
type ctaNot struct{ operand ctaExpr }

// ctaCompare is [11] ta-BooleanExpr's third arm with its Comparator present:
// a general comparison (xpath20.md §3.5.2) whose comparison type — the one
// type §3.5.2's casting rules convert BOTH operands into — was settled at
// compile time by ctaTypes.comparison.
type ctaCompare struct {
	op         ctaComparator
	comparison *xsd.SimpleType
	left       ctaValue
	right      ctaValue
}

// ctaValueCompare is a value comparison (xpath20.md §3.5.1), [10]
// ComparisonExpr's ValueComp arm, which only the assertion and facet façades
// admit (ctaFacade.comparesValues). Its comparison type — the one type §3.5.1
// converts both atomized operands into — was settled at compile time by
// ctaTypes.valueComparison, so the node is B.2-legal by construction as a
// ctaCompare is.
//
// It is a node of its own and not a ctaCompare with a flag, because the two
// quantify differently over the same operand sequences: a general comparison is
// existential over them, and a value comparison admits at most one item per
// operand and answers the EMPTY SEQUENCE for an empty one (ctaValueCompare.eval).
type ctaValueCompare struct {
	op         ctaComparator
	comparison *xsd.SimpleType
	left       ctaValue
	right      ctaValue
}

// ctaEffectiveBoolean is [11] ta-BooleanExpr's third arm with its Comparator
// ABSENT — a bare ValueExpr standing in a boolean position, whose value is its
// ·effective boolean value· (xpath20.md §2.4.3, fn:boolean).
type ctaEffectiveBoolean struct{ operand ctaValue }

// ctaTypeError is a comparison whose operand types are not a valid combination
// for its operator under xpath20.md B.2 — no one type serves both operands, or
// B.2 holds no row for that operator over the type that does, both of which
// ctaTypes.comparison decides. XPath raises err:XPTY0004 for it, so the node
// evaluates to ctaError: a raised error the enclosing operators carry, not a
// decline and not a node-local false.
type ctaTypeError struct{}

// ctaIf is xpath20.md [7] IfExpr, `if (Expr) then ExprSingle else ExprSingle`
// (§3.8), which only the assertion and facet façades admit
// (ctaFacade.conditional): the ·effective boolean value· of test selects then
// or otherwise, and only the selected branch is evaluated (ctaIf.eval).
//
// Its branches are boolean nodes and not item-valued ones because the
// parser builds it only where an ExprSingle stands whole in a boolean
// position (ctaParser.exprSingle) — the {test} itself, a parenthesized
// expression, fn:not's argument, and the three operands of another IfExpr —
// whose consumer takes the ·effective boolean value· of the IfExpr's value,
// which is the ·effective boolean value· of the selected branch. A branch that
// is a bare value is a ctaEffectiveBoolean over it, as it is at the root.
type ctaIf struct {
	test      ctaExpr
	then      ctaExpr
	otherwise ctaExpr
}

// ctaQuantified is xpath20.md [6] QuantifiedExpr, `some|every $v in N
// satisfies B`, over one in-clause binding `$v` to each child of E named N —
// over one child step, and over nothing else — which only the assertion façade
// admits (ctaFacade.quantified). §3.9 binds `$v` to each item of the binding
// sequence in turn and takes B's ·effective boolean value· per binding: `some`
// is true where one binding satisfies B, `every` where all do, so over no
// binding at all `some` is false and `every` true.
//
// No variable is bound at evaluation. The parser desugars B to the key
// counting the bindings that satisfy it (ctaParser.rangeBody), satisfying, so
// `some` is that count above 0 and `every` that count equal to the count of
// every binding, satisfying.bound(), both read off the [Tally]. A body
// fn:not(B) is rewritten by De Morgan's law to ctaNot over the dual quantifier
// of B (ctaParser.quantifiedExpr), so no key carries a negation.
//
// A quantifier over `$value` is ctaQuantifiedValue, an arm of its own and not
// this one with a flag: the two decide on different channels, this one off
// [Tally] keys with nothing bound, that one by binding each item at
// evaluation. So is a quantifier over a child step whose body reads its bound
// children's values or `$v/following-sibling::M[1]`, ctaQuantifiedChildren,
// which binds each child at evaluation off [ChildElements].
type ctaQuantified struct {
	q          ctaQuantifier
	satisfying ctaRangeKey
}

// ctaQuantifiedValue is xpath20.md [6] QuantifiedExpr over one in-clause whose
// binding sequence is a typed `$value`, `some|every $x in data($value)
// satisfies B` — fn:data over an atomic sequence is the identity
// (xpath-functions.md §2.4, xpath20.md §2.4.2), so `$value` bare is the same
// node — which only the assertion façade admits (ctaFacade.rangeScope). §3.9
// binds `$x` to each item of over in turn, each the ctaRangeItem body reads,
// and takes body's ·effective boolean value· per binding:
// ctaQuantifiedValue.eval.
//
// over is the concrete ctaValueVar, so a binding sequence other than a typed
// `$value`, atomic or list, is not representable: the statically empty and the
// ·special· `$value` decline at compile time (ctaParser.valueBinding).
type ctaQuantifiedValue struct {
	q    ctaQuantifier
	over ctaValueVar
	body ctaExpr
}

// ctaQuantifiedChildren is xpath20.md [6] QuantifiedExpr over one in-clause
// whose binding sequence is a typed child step, `some|every $v in N satisfies
// B`, or that step filtered by `[position() lt last()]` where dropLast is
// true — every child named N but the last, since inside the predicate the
// context position and size count the step's own result (§3.2.2, §2.1.2;
// xpath-functions.md §16.1, §16.2) — which only the assertion façade admits
// (ctaFacade.childScope). §3.9 binds `$v` to each such child in document order
// (ctaEnv.rangeChild), each the ctaBoundChild and ctaBoundSibling nodes of
// body read, and takes body's ·effective boolean value· per binding:
// ctaQuantifiedChildren.eval.
//
// over is the concrete ctaTypedChild, so a binding step of mixed content or of
// a type no typed step reads is not representable: ctaFacade.child declines
// the second and builds ctaUntypedChild for the first, which
// ctaParser.childQuantified declines. It is an arm of its own and not
// ctaQuantified with a flag, on ctaQuantifiedValue's terms: it decides by
// binding each child at evaluation, where ctaQuantified reads [Tally] keys.
type ctaQuantifiedChildren struct {
	q        ctaQuantifier
	over     ctaTypedChild
	dropLast bool
	body     ctaExpr
}

// ctaQuantifier is the keyword of a ctaQuantified, a ctaQuantifiedValue or a
// ctaQuantifiedChildren, `some` or `every`. Its zero value is no quantifier,
// and no node holds it: ctaParser.quantifiedExpr builds ctaSome or ctaEvery
// from the keyword read.
type ctaQuantifier byte

const (
	// ctaSome is `some`: true where at least one binding satisfies the body.
	ctaSome ctaQuantifier = iota + 1
	// ctaEvery is `every`: true where every binding satisfies the body.
	ctaEvery
)

// dual is the quantifier De Morgan's law trades q for under fn:not: `every $v
// satisfies not(B)` is `not(some $v satisfies B)`, and `some $v satisfies
// not(B)` is `not(every $v satisfies B)`. The zero quantifier, which no node
// holds, is its own.
func (q ctaQuantifier) dual() ctaQuantifier {
	switch q {
	case ctaSome:
		return ctaEvery
	case ctaEvery:
		return ctaSome
	}
	return q
}

func (ctaOr) ctaExpr()                 {}
func (ctaAnd) ctaExpr()                {}
func (ctaNot) ctaExpr()                {}
func (ctaCompare) ctaExpr()            {}
func (ctaValueCompare) ctaExpr()       {}
func (ctaEffectiveBoolean) ctaExpr()   {}
func (ctaTypeError) ctaExpr()          {}
func (ctaIf) ctaExpr()                 {}
func (ctaQuantified) ctaExpr()         {}
func (ctaQuantifiedValue) ctaExpr()    {}
func (ctaQuantifiedChildren) ctaExpr() {}

// ctaValue is the sealed sum of the ITEM-valued nodes: the arms of [16]
// ta-SimpleValue — its AttrName arm in the untyped and the typed form
// (ctaFacade.attribute), its Literal arm, and the assertion façade's `$value`
// in its three static forms (ctaFacade.variable), child-axis step
// (ctaFacade.child), child path (ctaFacade.childPath), child step filtered by
// its children's existence (ctaFacade.childrenHaving), child step filtered by
// a `preceding::` step (ctaFacade.childrenPreceded), element step whose
// existence is asked (ctaFacade.elements) and rooted path (ctaFacade.rooted),
// and the facet façade's read of an absent context item (ctaNoContextItem) —
// the cast that [15] ta-CastExpr's tail and [18] ta-ConstructorFunction both
// build over one of them — the latter, on a façade that calls the library,
// over any other branch an argument parses too — an fn:count call
// (ctaFacade.count), a binary arithmetic operator over two of them (ctaArith,
// ctaFacade.computes), the `castable as` tail over one of them (ctaCastable,
// ctaFacade.castable), the `instance of` tail over one of them or over an
// fn:data call (ctaInstanceOf, ctaFacade.instanceOf), and a call to an F&O
// string or sequence function over them (ctaMatch, ctaUnaryString,
// ctaPresence, ctaDistinctValues, ctaStringFunction, ctaConcat;
// ctaFacade.callsLibrary), to fn:current-date (ctaCurrentDate) or, over an
// absent focus, to fn:position or fn:last (ctaNoFocus, ctaFacade.focus), or to
// fn:namespace-uri (ctaNamespaceURI), the assertion façade's `.`
// (ctaContextAtom, ctaFacade.contextItem), and an integer or string sequence
// (ctaIntegerRanges, ctaStringSequence, ctaFacade.constructsSequences), and
// the range variable of a quantifier over `$value` inside its body
// (ctaRangeItem, ctaFacade.rangeScope), and the range variable of a quantifier
// over a typed child step inside its body and `$v/following-sibling::M[1]`
// over it (ctaBoundChild, ctaBoundSibling; ctaFacade.childScope,
// ctaFacade.boundSibling). `.` as the node fn:namespace-uri and
// fn:in-scope-prefixes take is no branch: it is ctaNodeArg
// (ctaFacade.contextNode). Every branch answers readsChild and counted on
// ctaExpr's terms.
type ctaValue interface {
	ctaValue()
	readsChild(name xsd.QName) bool
	counted(into []ctaKey) []ctaKey
}

// ctaAttr is [17] ta-AttrName over an UNTYPED attribute: the attribute step
// whose NameTest selects a SEQUENCE of E's attributes, in document order, out
// of what [Attributes] yields — or, in an assertion, the one attribute an exact
// NameTest names whose type is ·special·, whose typed value is xs:untypedAtomic
// (ctaAssertionFacade.attribute), out of what [TypedAttributes] yields.
//
// The NameTest is settled at compile time, so evaluation carries no axis and no
// prefix of its own — every name it could resolve is already an ·expanded name·
// (ctaNameTest).
type ctaAttr struct{ test ctaNameTest }

// ctaTypedAttr is [17] ta-AttrName over a TYPED instance, which is an
// assertion's (ctaAssertionFacade): the attribute E has under the ·expanded
// name· name — carried, or ·defaulted· ([TypedAttributes]) — at most one, whose
// typed value is of type st — the {type definition} [AttributeTypes] answered
// for that name at compile time, which is why the node carries it and the
// operand's static type is st rather than xs:untypedAtomic.
//
// Only a QName NameTest builds one: a [37] Wildcard arm can match an attribute
// ·attributed to· an {attribute wildcard}, whose type is not fixed at compile
// time, so ctaAssertionFacade declines it.
type ctaTypedAttr struct {
	name xsd.QName
	st   *xsd.SimpleType
}

// ctaTypedChild is an abbreviated child-axis step over a TYPED instance, which
// is an assertion's (ctaAssertionFacade.child): the sequence of E's element
// [[children]] under the ·expanded name· name, in document order
// ([ChildElements]), the typed value of each of which is of type st — the
// simple type ctaAssertionFacade.child read off the ·locally declared type·
// [ElementTypes] answered for that name at compile time, which is why the node
// carries it and the operand's static type is st (xpath-datamodel §6.2.4,
// §3.3.1.2).
//
// A ·nilled· child is a node of the sequence whose typed value is the empty
// sequence (xpath-datamodel §6.2.4): it counts for the step's node existence
// (ctaStep.nodes), and contributes no atom when the sequence is atomized. A
// step standing where its existence alone is asked is ctaSelectedElements and
// never this node.
//
// It is a node of its own and not a ctaTypedAttr with a flag: the two read
// different inputs, and an element step matches any number of nodes where an
// exact attribute step matches at most one.
type ctaTypedChild struct {
	name xsd.QName
	st   *xsd.SimpleType
}

// ctaUntypedChild is an abbreviated child-axis step over an assertion's
// instance whose children under the ·expanded name· name have a ·locally
// declared type· — the one [ElementTypes] answered at compile time — that is a
// complex type whose {content type}.{variety} is mixed, xs:anyType among them
// (ctaAssertionFacade.child): the sequence of E's element [[children]] under
// that name, in document order ([ChildElements]), the typed value of each of
// which is its string-value as one xs:untypedAtomic value (xpath-datamodel
// §6.2.4), so the operand's static type is xs:untypedAtomic, as a ctaAttr's is.
// A ·nilled· child is a node whose typed value is the empty sequence, on
// ctaTypedChild's terms (xpath20.md §2.5.2 item 4.1).
//
// It is a node of its own and not a ctaTypedChild with a flag: it holds no
// type, and it reads the [Untyped] arm of [ChildElements] where ctaTypedChild
// reads the [Typed] one.
type ctaUntypedChild struct{ name xsd.QName }

// ctaCandidate is the [47] ContextItemExpr `.` inside a predicate that filters
// a child step (ctaMatchingChildren): the child the predicate is evaluated for
// (xpath20.md §3.2.2: "the context item is the item currently being tested
// against the predicate"), one node whose typed value is of type st — the type
// ctaAssertionFacade.child reads off the ·locally declared type· of the step's
// children, so a child of a type that declines there declines here, and so
// does a child of mixed content, which that step reads untyped — or the
// empty sequence for a ·nilled· child (xpath-datamodel §6.2.4). The value is
// the evaluation's ctaEnv.candidate. Only ctaPredicateFacade.contextItem
// compiles it, in a value predicate's scope (ctaParser.valuePredicate): the
// assertion façade reads `.` outside a predicate as E's string value
// (ctaContextAtom), and every other façade declines `.` or raises over it.
type ctaCandidate struct{ st *xsd.SimpleType }

// ctaRangeItem is the range variable `$x` of a quantifier over `$value`
// (ctaQuantifiedValue) inside its body: one atomic item of type st, the
// `$value` item type the binding sequence carries, whose value is the
// evaluation's ctaEnv.rangeItem. Only ctaRangeFacade.variable compiles it, in
// the body's scope (ctaParser.quantifiedExpr).
//
// It is no ctaStep and no ctaCandidate: a step's ·effective boolean value· is
// node existence, and `$x` is an atomic value whose ·effective boolean value·
// is fn:boolean's over it (ctaBoolean), so `some $x in data($value) satisfies
// $x` over `0` is false. st is ctaValueVar.atom, copied at the one site
// building this node (ctaAssertionFacade.rangeScope): it is the static type the
// body's operators read (ctaCarriedType).
type ctaRangeItem struct{ st *xsd.SimpleType }

// ctaBoundChild is the range variable `$v` of a quantifier over a typed child
// step (ctaQuantifiedChildren) inside its body: one child of E, the evaluation's
// ctaEnv.rangeChild, whose typed value is of type st — the binding step's
// ctaTypedChild.st, copied at the one site building this node
// (ctaAssertionFacade.childScope) — or the empty sequence for a ·nilled· one
// (xpath-datamodel §6.2.4). Only ctaChildRangeFacade.variable compiles it.
//
// It is a ctaStep and no ctaRangeItem: `$v` is a NODE, so its ·effective
// boolean value· is true whatever its value, a ·nilled· child's included
// (xpath20.md §2.4.3 rule 2), and it atomizes to its typed value where an
// operator reads it (§2.4.2).
type ctaBoundChild struct{ st *xsd.SimpleType }

// ctaBoundSibling is `$v/following-sibling::M[1]` over the range variable of a
// quantifier over a typed child step (ctaQuantifiedChildren) inside its body:
// the first element following the bound child, in document order, among E's
// element [[children]] named step.name, however many of other names
// intervene (xpath20.md §3.2.1.1; §3.2.2 makes `[1]` count the
// `following-sibling::M` step's own result), or nothing after the last one —
// the empty sequence, which a value comparison answers empty and so false
// (§3.5.1, §2.4.3 rule 1). Its typed value is of type step.st, the type
// ctaAssertionFacade.child reads off M's ·locally declared type·, or the empty
// sequence for a ·nilled· child. Only ctaChildRangeFacade.boundSibling compiles
// it.
//
// It is a ctaStep: it selects zero or one node, on ctaBoundChild's terms.
type ctaBoundSibling struct{ step ctaTypedChild }

// ctaNoDocumentRoot is a path opening with "/" or "//", which begins at the
// root of the tree containing the context node through `(fn:root(self::node())
// treat as document-node())` (xpath20.md §3.2) — and the root of the data
// model instance cvc-assertion clause 1.3 builds is E, an element node, with
// no document node above it: "if the root node above the context node is not a
// document node, a dynamic error is raised [err:XPDY0050]". So it raises
// whatever E is named and whatever step follows. It is a node of its own: no
// other arm raises err:XPDY0050, and ctaTypeError is a comparison's
// err:XPTY0004.
type ctaNoDocumentRoot struct{}

// ctaNoContextItem is an expression that reads the context item where there is
// none, which only an assertions facet's {test} is evaluated under
// (cvc-assertions-valid clause 1.2: "There is no context item"; its Note: "the
// expression '.', or any implicit or explicit reference to the context item,
// will raise a dynamic error"): the [47] ContextItemExpr `.`, an attribute or
// child-axis step, and a path opening with "/" or "//". Each raises
// err:XPDY0002 (xpath20.md §2.1.2, §3.1.4: "If the context item is undefined,
// a context item expression raises a dynamic error") whatever it names, and only
// the facet façade builds it (ctaFacetFacade). It is a node of its own and not
// ctaNoDocumentRoot: that one's err:XPDY0050 needs a context node whose root is
// not a document node, which no context item at all cannot supply.
type ctaNoContextItem struct{}

// ctaValueVar is `$value` over a simple {content type} (cvc-assertion clause
// 2.3.1): the XDM representation of E's [schema actual value], read from the
// [ValueBinding] the evaluation carries. atom is the type of each item — the
// {simple type definition} itself, or its {item type definition} where listed
// is true and the value is a list, whose items Datatypes dt-xdmrep flattens
// into the sequence. Both were settled at compile time (ctaTypes.valueVariable),
// which is why the operand's static type is atom.
type ctaValueVar struct {
	atom   *xsd.SimpleType
	listed bool
}

// ctaEmptyValue is `$value` under any {content type} that is not simple
// (cvc-assertion clause 2.3.2): the empty sequence, decided at compile time,
// so the evaluation's [ValueBinding] is never read. It is also the node of
// arithmetic over such a `$value` (ctaTypes.arithmetic), whose result §3.4
// makes the empty sequence at compile time all the same, of the empty sequence
// `()` written as a library call's argument (ctaParser.argument), and of an
// integer sequence holding no item, `(1 to 0)` (ctaParser.integerSequence).
type ctaEmptyValue struct{}

// ctaUntypedValue is `$value` over a simple {content type} whose {simple type
// definition} is ·special· (cvc-assertion clause 2.3.1): its XDM representation
// is E's [schema normalized value] as one xs:untypedAtomic value (Datatypes
// dt-xdmrep clause 1), read from the [ValueBinding] as an [Untyped] value, or
// the empty sequence where the binding's `$value` is nil (clause 2.3.2). It is an
// arm of its own and not a ctaValueVar with a flag, because it holds no type:
// its operand's static type is xs:untypedAtomic, as an untyped attribute's is.
type ctaUntypedValue struct{}

// ctaContextAtom is the [47] ContextItemExpr `.` over E, an element whose
// ·governing type definition· has any {content type}
// (ctaAssertionFacade.contextItem), ATOMIZED (xpath20.md §2.4.2): one
// xs:untypedAtomic value holding E's string value, read from the
// [ValueBinding] the evaluation carries — never `$value`'s typed value, and
// never the empty sequence ([ValueBinding] states why). It is an arm of its
// own and not a ctaUntypedValue, because the two read different facts of the
// binding: `$value` over a ·special· type is the empty sequence where E is
// invalid or ·nilled·, and `.` is E's string value there too.
//
// It stands only where it is atomized. As the whole operand of an ·effective
// boolean value·, fn:exists or fn:empty `.` is a NODE, and
// ctaParser.booleanExpr and ctaParser.presenceCall decline it there rather
// than read the atom; fn:count over it is a path, which ctaParser.countPath
// declines; and as the argument of fn:namespace-uri or fn:in-scope-prefixes
// it is ctaContextNode, never this.
type ctaContextAtom struct{}

// ctaFacade is the sealed sum of the façades compileCTATest parses for — one
// value, so the attribute node a façade builds and the comparisons it admits
// can never come from two different façades (STYLE T1). The grammar's three
// consumers — a Type Alternative, an assertion and an assertions facet — the
// scope a value predicate's expression parses in inside an assertion's
// fn:count argument (ctaPredicateFacade), the scope the body of an assertion's
// quantifier over `$value` parses in (ctaRangeFacade), and the one the body of
// its quantifier over a typed child step parses in (ctaChildRangeFacade) close
// the set (STYLE T2's schema-closed-set exception).
type ctaFacade interface {
	ctaFacade()
	// attribute compiles one [17] ta-AttrName whose NameTest resolved to test
	// into its node, reporting false where the façade declines it — which
	// declines the whole expression on [CompileCTATest]'s withhold terms. The
	// types are the compile's own, for a façade that classifies the type it
	// reads.
	attribute(test ctaNameTest, types ctaTypes) (ctaValue, bool)
	// comparesValues reports whether the façade admits xpath20.md [23]
	// ValueComp at all, which §3.12.6's grammar has no production for.
	comparesValues() bool
	// variable compiles one [44] VarRef naming name into its node, reporting
	// false where the façade declines it — every name not in its static
	// context, which is a static error (err:XPST0008) withheld on the same
	// terms as attribute's decline.
	variable(name xsd.QName, types ctaTypes) (ctaValue, bool)
	// child compiles one abbreviated child-axis step whose NameTest resolved
	// to test into its node, reporting false where the façade declines it, on
	// attribute's terms.
	child(test ctaNameTest, types ctaTypes) (ctaValue, bool)
	// childPath compiles a relative path of two or more child-axis steps — the
	// steps before the last, whose QName NameTests resolved to parents, and the
	// last, whose NameTest resolved to last — standing as the whole operand of
	// fn:exists, fn:empty or an ·effective boolean value·
	// (ctaParser.childPath), into its node, reporting false where the façade
	// declines it, on attribute's terms. No step is typed: none is atomized.
	childPath(parents []xsd.QName, last ctaElementTest) (ctaValue, bool)
	// attributePath compiles a relative path of one or more child-axis steps
	// whose QName NameTests resolved to steps, followed by one attribute step
	// whose NameTest resolved to test, `N1/…/@A`, standing as fn:sum's
	// argument (ctaParser.sumCall), into its node, reporting false where the
	// façade declines it, on attribute's terms. The attribute step is typed
	// with the type types classifies, as attribute's is: its nodes are
	// atomized.
	attributePath(steps []xsd.QName, test ctaNameTest, types ctaTypes) (ctaValue, bool)
	// childrenHaving compiles a child step whose QName NameTest resolved to
	// name, filtered by a predicate that is a conjunction of child steps whose
	// QName NameTests resolved to required, `N[a and b]`, standing where
	// childPath's path stands (ctaParser.childrenHaving), into its node,
	// reporting false where the façade declines it, on attribute's terms. No
	// step is typed, on childPath's terms.
	childrenHaving(name xsd.QName, required []xsd.QName) (ctaValue, bool)
	// childrenPreceded compiles a child step filtered by a predicate that is
	// one `preceding::` step, `N[preceding::M[not(P)]]`, whose names the parser
	// resolved into preceded, standing where childPath's path stands
	// (ctaParser.childrenPreceded), into its node, reporting false where the
	// façade declines it, on attribute's terms. No step is typed, on
	// childPath's terms.
	childrenPreceded(preceded ctaChildrenPreceded) (ctaValue, bool)
	// elements compiles one element step, `N`, `./N` or `.//N`, whose QName
	// NameTest resolved into path, standing as the whole operand of fn:exists,
	// fn:empty or an ·effective boolean value· (ctaParser.selectedElements),
	// into its node, reporting false where the façade declines it, on
	// attribute's terms. The step is not typed, on childPath's terms.
	elements(path ctaCountPath) (ctaValue, bool)
	// quantified compiles xpath20.md [6] QuantifiedExpr over one child step,
	// and only that form, `some|every $v in N satisfies B`, quantifier q, whose
	// body B the parser desugared to the key counting the children satisfying
	// it (ctaParser.quantifiedExpr), into its node, reporting false where the
	// façade declines it, on attribute's terms. Neither N nor B is typed: no
	// step is atomized. A quantifier over `$value` is rangeScope's, and one
	// over a child step whose body is neither form, or which `[position() lt
	// last()]` filters, childScope's.
	quantified(q ctaQuantifier, satisfying ctaRangeKey) (ctaExpr, bool)
	// rangeScope is the façade the body of xpath20.md [6] QuantifiedExpr over
	// a typed `$value`, `some|every $x in data($value) satisfies B`, parses
	// under: this one with the range variable named variable in scope, each
	// item of over (§3.9: the range variable is added to the body's in-scope
	// variables), reporting false where the façade declines the quantifier, on
	// attribute's terms (ctaParser.quantifiedExpr).
	rangeScope(variable xsd.QName, over ctaValueVar) (ctaFacade, bool)
	// childScope is the façade the body of xpath20.md [6] QuantifiedExpr over
	// a typed child step, `some|every $v in N satisfies B` or over
	// `N[position() lt last()]`, parses under: this one with the range variable
	// named variable in scope, each child over selects (§3.9), reporting false
	// where the façade declines the quantifier, on attribute's terms
	// (ctaParser.childQuantified).
	childScope(variable xsd.QName, over ctaTypedChild) (ctaFacade, bool)
	// boundSibling compiles `$v/following-sibling::M[1]`, variable the
	// ·expanded name· `$v` resolved to and next the NameTest M resolved to, into
	// its node, reporting false where the façade declines it, on attribute's
	// terms: every façade but the one childScope answers declines it, and that
	// one declines every variable but its range variable (ctaParser.simpleValue).
	boundSibling(variable xsd.QName, next ctaNameTest, types ctaTypes) (ctaValue, bool)
	// rooted compiles a path opening with "/" or "//" into its node, reporting
	// false where the façade declines it, on attribute's terms.
	rooted() (ctaValue, bool)
	// contextItem compiles the [47] ContextItemExpr `.` into its node,
	// reporting false where the façade declines it, on attribute's terms.
	contextItem() (ctaValue, bool)
	// contextNode compiles the [47] ContextItemExpr `.` as the NODE the
	// argument of fn:namespace-uri and fn:in-scope-prefixes takes, never
	// atomized (ctaParser.contextNodeArgument), into its node, reporting false
	// where the façade declines it, on attribute's terms. Only a façade that
	// calls the library reaches it (ctaParser.libraryCall,
	// ctaParser.prefixMember).
	contextNode() (ctaNodeArg, bool)
	// focus compiles a call to fn:position or fn:last with no argument
	// (xpath-functions.md §16.1, §16.2), a read of the context position or
	// size whose result is st, xs:integer, into its node, reporting false
	// where the façade declines it, on attribute's terms. Only a façade that
	// calls the library reaches it (ctaParser.libraryCall).
	focus(st *xsd.SimpleType) (ctaValue, bool)
	// count compiles an fn:count call over a path, compiled to arg, into its
	// node, reporting false where the façade declines it, on attribute's
	// terms. An argument that is no path never reaches it: ctaParser.countCall
	// compiles that call itself (ctaCountedItems).
	count(arg ctaCounted, types ctaTypes) (ctaValue, bool)
	// computes reports whether the façade admits xpath20.md §3.4's binary
	// arithmetic operators at all ([13] AdditiveExpr, [14]
	// MultiplicativeExpr), which §3.12.6's grammar has no production for.
	computes() bool
	// callsLibrary reports whether the façade admits a call to the F&O
	// functions ctaParser.libraryCall parses — fn:contains, fn:starts-with,
	// fn:ends-with, fn:string-length, fn:normalize-space, fn:string,
	// fn:concat, fn:empty, fn:exists, fn:distinct-values, fn:true, fn:false,
	// fn:current-date, fn:position and fn:last (ctaFacade.focus) and
	// fn:namespace-uri (ctaFacade.contextNode) — and fn:in-scope-prefixes as an
	// operand of `=` (ctaParser.prefixMember) at all, which §3.12.6 clause 3
	// pins out of [12] ta-BooleanFunction (fn:not alone) and [18]
	// ta-ConstructorFunction (constructors alone), an fn:count argument that is
	// no path (ctaParser.countCall), and a constructor function's operand
	// beyond [18]'s [16] ta-SimpleValue (ctaParser.constructorOperand).
	callsLibrary() bool
	// conditional reports whether the façade admits xpath20.md [7] IfExpr at
	// all (ctaParser.ifExpr), which §3.12.6's grammar has no production for.
	conditional() bool
	// constructsSequences reports whether the façade admits, as an operand of
	// a general comparison, xpath20.md [11] RangeExpr `to` and the
	// parenthesized comma sequence of §3.3.1 over IntegerLiterals or over
	// StringLiterals (ctaParser.sequence), which §3.12.6's grammar has no
	// production for.
	constructsSequences() bool
	// castable reports whether the façade admits xpath20.md [18]
	// CastableExpr's `castable as` tail at all (ctaParser.castableTail), which
	// §3.12.6's grammar has no production for.
	castable() bool
	// instanceOf reports whether the façade admits xpath20.md [16]
	// InstanceofExpr's `instance of` tail, and fn:data as its operand, at all
	// (ctaParser.instanceofExpr), which §3.12.6's grammar has no production
	// for.
	instanceOf() bool
}

// ctaTypeAlternativeFacade is a Type Alternative's façade: every attribute
// NameTest is admitted and reads E's attributes untyped, and no production
// beyond §3.12.6's grammar is admitted.
type ctaTypeAlternativeFacade struct{}

func (ctaTypeAlternativeFacade) ctaFacade() {}

func (ctaTypeAlternativeFacade) attribute(test ctaNameTest, _ ctaTypes) (ctaValue, bool) {
	return ctaAttr{test: test}, true
}

// comparesValues is false: [13] ta-Comparator spells the general comparators
// alone, and a value comparison is outside the required subset §3.12.6's Note
// licenses a processor to decline.
func (ctaTypeAlternativeFacade) comparesValues() bool { return false }

// variable declines every name: ta-props-correct clause 2's grammar has no
// VarRef, and xpath-valid clause 2.2.6 leaves a Type Alternative's in-scope
// variables empty, so `$value` there is err:XPST0008 — a static error this
// engine declines rather than charges, on the "unsupported dominates static"
// terms [CTATestStaticError] states.
func (ctaTypeAlternativeFacade) variable(xsd.QName, ctaTypes) (ctaValue, bool) {
	return nil, false
}

// child declines every child-axis step: [17] ta-AttrName is the only step
// ta-props-correct clause 2's grammar has, and a {test} outside that grammar
// is what §3.12.6's Note licenses a processor to decline.
func (ctaTypeAlternativeFacade) child(ctaNameTest, ctaTypes) (ctaValue, bool) {
	return nil, false
}

// childPath declines every path of child steps, on child's terms: §3.12.4
// key-cta-ta-select clause 1.2 gives the instance a Type Alternative's {test}
// is evaluated over no [children], so `N/*` would select nothing there, and
// declining it is what §3.12.6's Note licenses.
func (ctaTypeAlternativeFacade) childPath([]xsd.QName, ctaElementTest) (ctaValue, bool) {
	return nil, false
}

// attributePath declines every path through child steps to an attribute, on
// childPath's terms. It is never reached: callsLibrary is false, so `sum(…)`
// reaches constructorFunction and declines there.
func (ctaTypeAlternativeFacade) attributePath([]xsd.QName, ctaNameTest, ctaTypes) (ctaValue, bool) {
	return nil, false
}

// childrenHaving declines every filtered child step, on childPath's terms.
func (ctaTypeAlternativeFacade) childrenHaving(xsd.QName, []xsd.QName) (ctaValue, bool) {
	return nil, false
}

// childrenPreceded declines every child step filtered by a `preceding::`
// step, on childPath's terms: key-cta-ta-select clause 1.2's instance has no
// [children] for the step to select.
func (ctaTypeAlternativeFacade) childrenPreceded(ctaChildrenPreceded) (ctaValue, bool) {
	return nil, false
}

// elements declines every element step, on child's terms.
func (ctaTypeAlternativeFacade) elements(ctaCountPath) (ctaValue, bool) {
	return nil, false
}

// quantified declines every quantified expression: ta-props-correct clause
// 2's grammar has no QuantifiedExpr, and the one it would range over is a
// child step, which child declines.
func (ctaTypeAlternativeFacade) quantified(ctaQuantifier, ctaRangeKey) (ctaExpr, bool) {
	return nil, false
}

// rangeScope declines every quantifier over `$value`, on quantified's terms.
// It is never reached: variable declines `$value` before the binding
// sequence is compiled.
func (ctaTypeAlternativeFacade) rangeScope(xsd.QName, ctaValueVar) (ctaFacade, bool) {
	return nil, false
}

// childScope declines every quantifier over a child step, on quantified's
// terms. It is never reached: child declines the binding step before the body's
// scope is asked for.
func (ctaTypeAlternativeFacade) childScope(xsd.QName, ctaTypedChild) (ctaFacade, bool) {
	return nil, false
}

// boundSibling declines, on child's terms: ta-props-correct clause 2's grammar
// has no VarRef and no axis but attribute.
func (ctaTypeAlternativeFacade) boundSibling(xsd.QName, ctaNameTest, ctaTypes) (ctaValue, bool) {
	return nil, false
}

// rooted declines every rooted path, on child's terms.
func (ctaTypeAlternativeFacade) rooted() (ctaValue, bool) {
	return nil, false
}

// contextItem declines `.`, on child's terms: ta-props-correct clause 2's
// grammar has no ContextItemExpr.
func (ctaTypeAlternativeFacade) contextItem() (ctaValue, bool) {
	return nil, false
}

// count declines every fn:count call, on child's terms: §3.12.6 clause 3 makes
// every [18] ta-ConstructorFunction a constructor for a built-in datatype, and
// no other function but fn:not is in the grammar.
func (ctaTypeAlternativeFacade) count(ctaCounted, ctaTypes) (ctaValue, bool) {
	return nil, false
}

// contextNode declines, on contextItem's terms. It is never reached:
// callsLibrary is false, so `namespace-uri(.)` reaches
// ctaParser.constructorFunction and declines there, and
// `in-scope-prefixes(.) = 'a'` is never read as ctaParser.prefixMember's.
func (ctaTypeAlternativeFacade) contextNode() (ctaNodeArg, bool) {
	return nil, false
}

// focus declines, on count's terms. It is never reached: callsLibrary is
// false, so `position()` and `last()` reach ctaParser.constructorFunction and
// decline there.
func (ctaTypeAlternativeFacade) focus(*xsd.SimpleType) (ctaValue, bool) {
	return nil, false
}

// computes is false, on comparesValues' terms: ta-props-correct clause 2's
// grammar has no arithmetic operator.
func (ctaTypeAlternativeFacade) computes() bool { return false }

// callsLibrary is false: §3.12.6 clause 3 makes every function call a call to
// fn:not or to a constructor, so every other name reaches
// ctaParser.constructorFunction and declines there, and [18] makes a
// constructor's operand a [16] ta-SimpleValue.
func (ctaTypeAlternativeFacade) callsLibrary() bool { return false }

// conditional is false, on comparesValues' terms: ta-props-correct clause 2's
// grammar ([8]–[18]) has no IfExpr, and §3.12.6's Note licenses a processor to
// decline a {test} outside it.
func (ctaTypeAlternativeFacade) conditional() bool { return false }

// constructsSequences is false, on comparesValues' terms: ta-props-correct
// clause 2's grammar has no RangeExpr and no comma.
func (ctaTypeAlternativeFacade) constructsSequences() bool { return false }

// castable is false, on comparesValues' terms: ta-props-correct clause 2's
// [15] ta-CastExpr has a `cast as` tail and no `castable as` one.
func (ctaTypeAlternativeFacade) castable() bool { return false }

// instanceOf is false, on castable's terms: ta-props-correct clause 2's
// grammar has no [16] InstanceofExpr, and clause 3 calls no function but
// fn:not and the constructors.
func (ctaTypeAlternativeFacade) instanceOf() bool { return false }

// ctaNameTest is the sealed sum of [36] NameTest's arms as [17] ta-AttrName
// and a child-axis step reach them, matching one ·expanded name· at a time.
// The axis is the parser's: the name an unprefixed QName resolves to depends
// on the axis's principal node kind (xpath20.md §3.2.1.2), and is resolved
// before a test is built. The grammar closes the set (STYLE T2's
// schema-closed-set exception), so no further arm is representable outside
// this package.
type ctaNameTest interface {
	ctaNameTest()
	// matches reports whether a node with this ·expanded name· is selected by
	// the test.
	matches(name xsd.QName) bool
}

// ctaExactName is [36]'s QName arm, resolved: a prefixed NameTest against the
// {namespace bindings}; an unprefixed one to NO namespace on the attribute
// axis, whose principal node kind is never element (ctaParser.attributeName),
// and to the {default namespace} on the child axis, whose principal node kind
// is element (ctaParser.elementName) (PRINCIPLES 15).
//
// A NameTest whose prefix has no binding holds ctaUnresolvedName instead, and
// no such node is ever evaluated: it comes with a ctaStaticError defect, on
// which [CompileCTATest] withholds.
type ctaExactName struct{ name xsd.QName }

// ctaAnyName is [37] Wildcard's `*` arm: "a node test * is true for any node of
// the principal node kind of the step axis" (xpath20.md §3.2.1.2), which on the
// attribute axis is every attribute of E, and on the child axis, ending a child
// path (ctaElementTest), every element child of the step's context node.
type ctaAnyName struct{}

// ctaAnyLocal is [37]'s `NCName ':' '*'` arm, whose prefix is ALREADY resolved
// against the {namespace bindings}: "true for any node ... whose expanded QName
// has the namespace URI to which the prefix is bound, regardless of the local
// part of the name".
//
// The empty space is a real matcher — every attribute in no namespace — and not
// a sentinel, which is why an unbound prefix takes ctaUnresolvedTest instead.
type ctaAnyLocal struct{ space string }

// ctaAnySpace is [37]'s `'*' ':' NCName` arm: "true for any node ... whose local
// name matches the given NCName, regardless of its namespace or lack of a
// namespace". No prefix is resolved, so this arm has no err:XPST0081 exposure.
type ctaAnySpace struct{ local string }

// ctaUnresolvedTest is the NameTest an UNBOUND prefix in [37]'s `NCName ':' '*'`
// resolves to. It matches nothing and is never evaluated: it comes with a
// ctaStaticError defect, on which [CompileCTATest] withholds — ctaUnresolvedName's
// role for a QName NameTest, as a TYPE rather than as a value, because no
// namespace URI is uninhabited and so none can stand in for one that resolved
// to nothing.
type ctaUnresolvedTest struct{}

func (ctaExactName) ctaNameTest()      {}
func (ctaAnyName) ctaNameTest()        {}
func (ctaAnyLocal) ctaNameTest()       {}
func (ctaAnySpace) ctaNameTest()       {}
func (ctaUnresolvedTest) ctaNameTest() {}

func (t ctaExactName) matches(name xsd.QName) bool { return name == t.name }

func (ctaAnyName) matches(xsd.QName) bool { return true }

// ctaElementTest is the sealed sum of the NameTests the last step of a child
// path takes (ctaChildPath): [36]'s QName arm (ctaExactName) and [37]'s `*` arm
// (ctaAnyName), which on the child axis, whose principal node kind is element
// (xpath20.md §3.2.1.1), is true for every element child and for no other node
// (§3.2.1.2). It is a narrower sum than ctaNameTest over two of that sum's own
// arms: `*:N` and `p:*` are no arm of it, so a path ending in either cannot be
// built (ctaParser.childPathLength never measures one). The grammar closes the
// set (STYLE T2's schema-closed-set exception).
type ctaElementTest interface {
	ctaNameTest
	ctaElementTest()
}

func (ctaExactName) ctaElementTest() {}
func (ctaAnyName) ctaElementTest()   {}

func (t ctaAnyLocal) matches(name xsd.QName) bool { return name.Space == t.space }

func (t ctaAnySpace) matches(name xsd.QName) bool { return name.Local == t.local }

func (ctaUnresolvedTest) matches(xsd.QName) bool { return false }

// ctaLiteral is the Literal arm of [16] ta-SimpleValue, carrying the builtin
// datatype its XPath literal kind fixes: a StringLiteral is xs:string, an
// IntegerLiteral or DecimalLiteral is xs:decimal (xs:integer's primitive base,
// which is the type two such literals are compared in; a cast tells them apart
// by text, literalCastsTo), and a DoubleLiteral is xs:double. A call to a
// zero-argument constant function compiles to one too: fn:true() and
// fn:false() are the xs:boolean of the lexical "true" and "false"
// (ctaParser.constantCall).
type ctaLiteral struct {
	text string
	st   *xsd.SimpleType
}

// ctaCast is [15] ta-CastExpr's `cast as QName '?'?` tail and, equivalently,
// [18] ta-ConstructorFunction: xpath20.md §3.10.4 DEFINES the constructor
// function call T($arg) as (($arg) cast as T?), so one node serves both and a
// second implementation would be STYLE T4 duplication.
//
// target is a builtin datatype, which §3.12.6 clause 4 fixes for the cast
// spelling ("Any explicit casts ... are casts to built-in datatypes") and
// clause 3 for the constructor spelling ("Any strings matching the
// ConstructorFunction production are function calls to constructor functions
// for the built-in datatypes").
//
// allowsEmpty is the `?` occurrence indicator, and it is load-bearing rather
// than cosmetic: xpath20.md §3.10.2 rule 3 makes an empty operand the result
// of the cast when `?` is present and err:XPTY0004 when it is not. §3.10.4's
// equivalence carries the `?` — "This example is equivalent to ("2000-01-01"
// cast as xs:date?)" — so the constructor spelling always allows it, whatever
// [18]'s own production says.
type ctaCast struct {
	operand     ctaValue
	target      *xsd.SimpleType
	allowsEmpty bool
}

// ctaCastable is xpath20.md [18] CastableExpr's `castable as SingleType` tail,
// `E castable as T`, whose result is st, xs:boolean: §3.10.3 makes it true
// exactly where `E cast as T` succeeds, so it holds that cast itself and
// evaluates it (ctaCastableItem) — one node, the cast's, decides both, and no
// second casting rule exists to disagree with it (STYLE D3). A cast's own
// failure — the empty sequence without `?`, two or more items, a lexical or
// facet mismatch — is false; an error evaluating E is the castable
// expression's error.
type ctaCastable struct {
	cast ctaCast
	st   *xsd.SimpleType
}

// ctaInstanceOf is xpath20.md [16] InstanceofExpr's `instance of SequenceType`
// tail, `E instance of T` with T an AtomicType and an optional [51]
// OccurrenceIndicator, whose result is st, xs:boolean: §3.10.1 makes it true
// "if the value of its first operand matches the SequenceType in its second
// operand, according to the rules for SequenceType matching" (§2.5.4), and it
// never casts. operand is E ATOMIZED: an fn:data call's argument
// (xpath-functions.md §2.4), or an operand that is already atomic
// (ctaParser.instanceofExpr). Whether one item matches T — "An AtomicType
// AtomicType matches an atomic value whose actual type is AT if
// derives-from(AT, AtomicType) is true" (§2.5.4.2) — is settled at compile
// time from the operand's static type, which the parser admits only where it
// decides every item's match (ctaTypes.instanceItem): it is every item's
// dynamic type, or, over a typed child, the type every item's dynamic type is
// or derives from and T is one it derives from too. So matches holds that
// answer and the evaluation counts the items alone (ctaInstanceOfItem). read is
// the type the items are read in to be counted: the operand's own type, or
// xs:string for an xs:untypedAtomic operand, as ctaDistinctValues reads one.
type ctaInstanceOf struct {
	operand    ctaValue
	read       *xsd.SimpleType
	matches    bool
	occurrence ctaOccurrence
	st         *xsd.SimpleType
}

// ctaOccurrence is xpath20.md [51] OccurrenceIndicator, absent included: how
// many items a SequenceType admits (§2.5.4.1).
type ctaOccurrence byte

const (
	// ctaExactlyOne is an ItemType with no indicator: "exactly one item".
	ctaExactlyOne ctaOccurrence = iota
	// ctaZeroOrOne is `?`.
	ctaZeroOrOne
	// ctaZeroOrMore is `*`.
	ctaZeroOrMore
	// ctaOneOrMore is `+`.
	ctaOneOrMore
)

// admits reports whether a sequence of n items has the length o admits:
// §2.5.4.1's "any sequence type whose OccurrenceIndicator is * or ? matches a
// value that is an empty sequence", and `?` and the absent indicator admit no
// second item.
func (o ctaOccurrence) admits(n int) bool {
	switch o {
	case ctaZeroOrOne:
		return n <= 1
	case ctaZeroOrMore:
		return true
	case ctaOneOrMore:
		return n >= 1
	default:
		return n == 1
	}
}

// ctaCount is an fn:count call (xpath-functions.md §15.4.1, `fn:count($arg as
// item()*) as xs:integer`), which the assertion façade admits over every
// argument and the facet façade over one that is no path (ctaParser.countCall):
// the number of items its argument evaluates to, as one value of st,
// xs:integer. The argument is not atomized — the signature's item()* asks for
// none — so a counted step is never typed and reads no value: what it selects
// is read off the [Tally] the evaluation carries, which the caller fills with
// E's subtree, and never off [TypedAttributes]. The path argument that reads a
// value is a child step filtered by a predicate over it (ctaMatchingChildren),
// which atomizes each candidate inside the predicate and is counted over
// [ChildElements] instead; an argument that is no path, such as `$value`, is
// counted off its own items (ctaCountedItems).
type ctaCount struct {
	arg ctaCounted
	st  *xsd.SimpleType
}

// ctaCounted is the sealed sum of what an fn:count argument compiles to: a
// relative path a [Tally] counts — one step (ctaCountPath), a child step
// filtered by attribute existence (ctaFilteredChildren), or a union of those
// (ctaUnion) — a child step filtered by its value, which the evaluation
// counts over [ChildElements] (ctaMatchingChildren), a rooted path, which
// raises err:XPDY0050 before it selects a node and so before fn:count sees a
// sequence (ctaNoDocumentRoot, xpath20.md §3.2), or an operand that is no
// path, whose own items are counted (ctaCountedItems). The grammar closes the set
// (STYLE T2's schema-closed-set exception). Every arm answers readsChild and
// counted on ctaExpr's terms, and nodes, how many items it evaluates to — the
// nodes a path selects — reporting false where it raises.
type ctaCounted interface {
	ctaCounted()
	readsChild(name xsd.QName) bool
	counted(into []ctaKey) []ctaKey
	nodes(env ctaEnv) (int, bool)
}

// ctaCountPath is one relative path fn:count counts over: the nodes on axis
// named name. It is one arm of the keys a [Tally] keeps one counter per
// distinct one of (ctaTallied).
type ctaCountPath struct {
	axis ctaCountAxis
	name xsd.QName
}

// ctaCountAxis is which nodes of E's subtree a ctaCountPath selects, by kind
// and by depth below E (xpath20.md §3.2.4's abbreviations, E the context node).
type ctaCountAxis byte

const (
	// ctaCountChildren is `N` or `./N`: the element children of E.
	ctaCountChildren ctaCountAxis = iota
	// ctaCountDescendants is `.//N`, `./descendant-or-self::node()/child::N`:
	// every element below E at any depth, and never E itself.
	ctaCountDescendants
	// ctaCountOwnAttributes is `@N` or `./@N`: the attributes of E.
	ctaCountOwnAttributes
	// ctaCountSubtreeAttributes is `.//@N`,
	// `./descendant-or-self::node()/attribute::N`: the attributes of E itself
	// and of every element below it.
	ctaCountSubtreeAttributes
)

func (ctaCountPath) ctaCounted()        {}
func (ctaFilteredChildren) ctaCounted() {}
func (ctaUnion) ctaCounted()            {}
func (ctaMatchingChildren) ctaCounted() {}
func (ctaNoDocumentRoot) ctaCounted()   {}
func (ctaCountedItems) ctaCounted()     {}

// ctaMatchingChildren is a child step with a QName NameTest filtered by a
// predicate that reads the child's VALUE, `N[. = 'x']` (xpath20.md §3.2.2),
// which only an fn:count argument takes (ctaParser.valuePredicate): the
// element children of E named name for which pred is true, each child in turn
// the context item pred reads as `.` (ctaCandidate). pred is a comparison, or
// and, or and fn:not over comparisons, never a bare value: a numeric one would
// select by position, which needs an order this engine does not keep, so the
// parser builds no other (ctaComparisonRooted).
//
// It is no counter key: what it selects depends on each child's typed value,
// which the [Tally] never sees, so it is counted over [ChildElements] at
// evaluation (ctaMatchingChildren.nodes) and reports name as a child it reads
// ([AssertionTest.ReadsChild]). A predicate that raises over any child makes
// the whole count raise, never a child left uncounted.
type ctaMatchingChildren struct {
	name xsd.QName
	pred ctaExpr
}

// ctaComparisonRooted reports whether x is a comparison, or ctaAnd, ctaOr or
// ctaNot over such — the roots ctaParser.valuePredicate admits. A comparison
// whose operand types B.2 rejects is ctaTypeError, still a comparison, which
// raises over each child. A bare value is ctaEffectiveBoolean, whose value may
// be numeric and so positional (§3.2.2), and is refused whatever its static
// type.
func ctaComparisonRooted(x ctaExpr) bool {
	switch n := x.(type) {
	case ctaCompare, ctaValueCompare, ctaTypeError:
		return true
	case ctaAnd:
		return ctaAllComparisonRooted(n.operands)
	case ctaOr:
		return ctaAllComparisonRooted(n.operands)
	case ctaNot:
		return ctaComparisonRooted(n.operand)
	case ctaEffectiveBoolean:
		return false
	case ctaIf:
		// A branch may be a bare value, and ctaPredicateFacade.conditional
		// declines every IfExpr before one is built.
		return false
	case ctaQuantified:
		// No comparison, and ctaPredicateFacade.quantified declines every
		// QuantifiedExpr before one is built.
		return false
	case ctaQuantifiedValue:
		// Never built here: ctaPredicateFacade.rangeScope declines every
		// quantifier over `$value` before one is.
		return false
	case ctaQuantifiedChildren:
		// Never built here: ctaPredicateFacade.childScope declines every
		// quantifier over a typed child step before one is.
		return false
	}
	return false
}

// ctaAllComparisonRooted is ctaComparisonRooted over each of operands.
func ctaAllComparisonRooted(operands []ctaExpr) bool {
	for _, o := range operands {
		if !ctaComparisonRooted(o) {
			return false
		}
	}
	return true
}

// ctaFilteredChildren is a child step with a QName NameTest filtered by a
// predicate that is a conjunction of attribute-existence tests, `N[@A]`,
// `N[@A1 and @A2 …]` (xpath20.md §3.2.2), which only an fn:count argument
// takes (ctaParser.predicate): the element children of E named name whose
// attribute nodes include every name in required. A predicate whose value is
// not numeric is decided by its ·effective boolean value· (§3.2.2), and an
// attribute step's is whether it selects a node (§2.4.3 rule 2), so no
// attribute's value — an empty one included — decides anything and none is
// typed. It is a counter key (ctaTallied) of its own, whose [Tally] reads the
// attribute names each [Tally.Element] report carries.
//
// Its one constructor is ctaFilteredChildrenOf, which holds required as a
// non-empty set: duplicates dropped, the rest in written order.
type ctaFilteredChildren struct {
	name     xsd.QName
	required []xsd.QName
}

// ctaFilteredChildrenOf is the ctaFilteredChildren over the children named
// name carrying every attribute in required, false where required is empty.
func ctaFilteredChildrenOf(name xsd.QName, required []xsd.QName) (ctaFilteredChildren, bool) {
	distinct := ctaDistinctNames(required)
	if len(distinct) == 0 {
		return ctaFilteredChildren{}, false
	}
	return ctaFilteredChildren{name: name, required: distinct}, true
}

// ctaUnion is a union of two or more counted operands, `A | B` or `A union B`
// (xpath20.md §3.3.3), which only an fn:count argument takes: every node any
// operand selects, each ONCE — the operator eliminates duplicates by node
// identity — so `count(e | .//e)` is the number of e below E and `count(@a |
// @b)` is 0, 1 or 2. Each operand is a ctaCountPath or a ctaFilteredChildren.
// It is a counter key (ctaTallied) of its own, whose [Tally] counts a reported
// node once where any operand selects it, and never sums per-operand counts.
//
// Its one constructor is ctaUnionOf, which drops an operand the same as one
// before it and collapses to the one operand left, so `count(e | e)` is
// `count(e)` and shares its counter.
type ctaUnion struct{ operands []ctaTallied }

// ctaUnionOf is the union of operands, false where one is not an operand a
// [Tally] counts in a union: a rooted path, which raises, and a child step
// filtered by a predicate that reads the child's value. operands is never
// empty: countArgument passes two or more.
func ctaUnionOf(operands []ctaCounted) (ctaCounted, bool) {
	var distinct []ctaTallied
	add := func(k ctaTallied) {
		if !ctaHoldsPath(distinct, k) {
			distinct = append(distinct, k)
		}
	}
	for _, o := range operands {
		switch k := o.(type) {
		case ctaCountPath:
			add(k)
		case ctaFilteredChildren:
			add(k)
		default:
			return nil, false
		}
	}
	if len(distinct) == 1 {
		single, counted := distinct[0].(ctaCounted)
		return single, counted
	}
	return ctaUnion{operands: distinct}, true
}

// ctaChildPath is a relative path of two or more abbreviated child-axis steps,
// `N1/N2/…` or `N1/…/*` (xpath20.md [26] RelativePathExpr, §3.2.1.1), which
// only the assertion façade admits (ctaFacade.childPath) and only as the whole
// operand of fn:exists, fn:empty or an ·effective boolean value·
// (ctaParser.childPath). parents holds the resolved ·expanded names· of the
// steps before the last, in written order, each with a QName NameTest; last is
// the last step's NameTest, a QName or the [37] Wildcard `*`
// (ctaElementTest).
//
// Each step is evaluated once per node the steps before it select (§3.2:
// "E1/E2 … evaluates E2 once for each node of E1", duplicates removed), so the
// path selects the elements whose chain of ancestors below E, from E's child
// down to the element itself, is parents followed by one name last matches —
// each a distinct node, and how many there are is what the three positions
// read: none or some, never a value. No step is atomized (fn:exists, fn:empty
// `item()*`, §2.4.3 rule 2), so no step is typed and a node of any type,
// ·nilled· or not, is selected; the count is read off the [Tally] the
// evaluation carries, which is why the node is a counter key (ctaTallied) and
// not a ctaCounted arm: fn:count over it declines.
//
// Its one constructor is ctaChildPathOf, which admits one parent or more: a
// one-step path is ctaTypedChild or ctaUntypedChild where its value is read,
// ctaSelectedElements where only its existence is, and ctaCountPath where it
// is counted.
type ctaChildPath struct {
	parents []xsd.QName
	last    ctaElementTest
}

// ctaChildPathOf is the ctaChildPath over parents and last, false where
// parents is empty. parents is held, not copied: the parser builds it fresh.
func ctaChildPathOf(parents []xsd.QName, last ctaElementTest) (ctaChildPath, bool) {
	if len(parents) == 0 {
		return ctaChildPath{}, false
	}
	return ctaChildPath{parents: parents, last: last}, true
}

// ctaAttributePath is a relative path of one or more abbreviated child-axis
// steps with QName NameTests followed by one abbreviated attribute step with a
// QName NameTest, `N1/…/@A` (xpath20.md [26] RelativePathExpr, §3.2.1.1,
// §3.2.4), which only the assertion façade admits (ctaFacade.attributePath) and
// only as fn:sum's argument (ctaParser.sumCall): the attribute nodes named name
// of the elements whose chain of names below E, from E's child down to the
// element itself, is steps — each a distinct node, in document order (§3.2) —
// atomized to their typed values (§2.4.2). Each value is of type st, the type
// [AttributeTypes] answered for (steps, name) at compile time, or
// xs:untypedAtomic where st is ·special· (xpath-datamodel §3.3.1.2).
//
// The values are read off the [Tally] the evaluation carries, which is why
// the node is a counter key (ctaKey) whose counter collects them
// (ctaAttributeCollector) rather than reading [TypedAttributes] or
// [ChildElements], neither of which reaches below E's children.
type ctaAttributePath struct {
	steps []xsd.QName
	name  xsd.QName
	st    *xsd.SimpleType
}

// ctaChildrenHaving is a child step with a QName NameTest filtered by a
// predicate that is a conjunction of child steps with QName NameTests, `N[a]`
// or `N[a and b …]` (xpath20.md §3.2.2), which only the assertion façade admits
// (ctaFacade.childrenHaving), standing where a ctaChildPath stands
// (ctaParser.childrenHaving): the element children of E named name each of
// which has, among its own element children, one named each of required.
//
// The predicate is evaluated once per child named name, that child the context
// item (§3.2.2 rule 2), and its value is no number, so it keeps the child where
// its ·effective boolean value· is true: each conjunct is a step, whose
// ·effective boolean value· is whether it selects a node (§2.4.3 rule 2, §3.6).
// So two N children that split the required names between them select
// nothing, where `N/a and N/b`, which hoists each conjunct to E, is true. No
// step is atomized, so none is typed.
//
// It is a counter key (ctaKey), but no ctaTallied arm: whether one N child is
// selected is decided by the reports of ITS children, which arrive after the
// child's own report, so no single [Tally.Element] report answers it. Its
// counter is ctaInstanceCount, which reads the reports in the pre-order
// [Tally] obliges its caller to. It is no ctaFilteredChildren either: that key
// decides a child from the attribute names its own report carries.
//
// Its one constructor is ctaChildrenHavingOf, which holds required as a
// non-empty set: duplicates dropped, the rest in written order.
type ctaChildrenHaving struct {
	name     xsd.QName
	required []xsd.QName
}

// ctaChildrenHavingOf is the ctaChildrenHaving over the children named name
// having a child named each of required, false where required is empty.
func ctaChildrenHavingOf(name xsd.QName, required []xsd.QName) (ctaChildrenHaving, bool) {
	distinct := ctaDistinctNames(required)
	if len(distinct) == 0 {
		return ctaChildrenHaving{}, false
	}
	return ctaChildrenHaving{name: name, required: distinct}, true
}

// ctaChildrenPreceded is a child step with a QName NameTest filtered by a
// predicate that is one `preceding::` step with a QName NameTest, itself
// optionally filtered by one child step or fn:not over one, `N[preceding::M]`,
// `N[preceding::M[P]]` or `N[preceding::M[not(P)]]` (xpath20.md §3.2.1.1,
// §3.2.2), which only the assertion façade admits (ctaFacade.childrenPreceded),
// standing where a ctaChildPath stands (ctaParser.childrenPreceded): the
// element children of E named name before each of which, in document order,
// stands an element named preceding that test keeps.
//
// The predicate is evaluated once per child named name, that child the context
// node (§3.2.2), and its ·effective boolean value· is whether the step selects
// a node (§2.4.3 rule 2). §3.2.1.1 makes `preceding::` "all nodes that are
// descendants of the root of the tree in which the context node is found, are
// not ancestors of the context node, and occur before the context node in
// document order"; cvc-assertion clause 1.3 roots that tree at E, so the step
// never leaves E's subtree, and a child of E has no ancestor there but E, which
// is never preceding. So the preceding elements of a child of E are every
// element of E's subtree before it but E, at any depth: an element nested in
// an earlier sibling among them. No step is atomized, so none is typed.
//
// It is a counter key (ctaKey), but no ctaTallied arm: whether an element
// named preceding is kept is decided by the reports of ITS children, which
// arrive after its own. Its counter is ctaPrecedenceCount.
type ctaChildrenPreceded struct {
	name      xsd.QName
	preceding xsd.QName
	test      ctaPrecedingTest
}

// ctaPrecedingTest is the sealed sum of the filters on the `preceding::M`
// step of a ctaChildrenPreceded: none (ctaAnyPreceding), and one child step or
// fn:not over one (ctaPrecedingChild). The grammar ctaParser.childrenPreceded
// parses closes the set (STYLE T2's schema-closed-set exception). Each arm is
// comparable, so two ctaChildrenPreceded compare with ==.
type ctaPrecedingTest interface {
	ctaPrecedingTest()
	// marks reports whether a child named name of an M decides the test.
	marks(name xsd.QName) bool
	// keeps reports whether the test keeps an M, given whether one of the
	// children it marks was reported below that M.
	keeps(shown bool) bool
}

// ctaAnyPreceding is `preceding::M` unfiltered: it keeps every M.
type ctaAnyPreceding struct{}

// ctaPrecedingChild is `preceding::M[P]`, or `preceding::M[not(P)]` where
// negated: it keeps an M one of whose element children is named child, or,
// negated, one none of whose element children is (xpath20.md §2.4.3 rule 2,
// xpath-functions.md §9.3.1).
type ctaPrecedingChild struct {
	child   xsd.QName
	negated bool
}

func (ctaAnyPreceding) ctaPrecedingTest()   {}
func (ctaPrecedingChild) ctaPrecedingTest() {}

// marks is false: no child decides an unfiltered step.
func (ctaAnyPreceding) marks(xsd.QName) bool { return false }

// keeps is true.
func (ctaAnyPreceding) keeps(bool) bool { return true }

// marks reports whether name is c's child.
func (c ctaPrecedingChild) marks(name xsd.QName) bool { return name == c.child }

// keeps is shown, or its negation where c is negated.
func (c ctaPrecedingChild) keeps(shown bool) bool { return shown != c.negated }

// ctaChildrenFollowedBy is the element children of E named name whose next
// sibling ELEMENT is named next: the children a quantifier binds to `$v` for
// which `$v/following-sibling::*[1][self::M]` selects a node, M resolving to
// next (ctaParser.siblingBody). xpath20.md §3.2.1.1 makes following-sibling
// the later children of `$v`'s parent, E, and `*` keeps the elements among
// them (§3.2.1.2); §3.2.2 makes `[1]` the first of those in document order, a
// forward axis's, and `[self::M]` keeps it where it is named M. A text or
// comment sibling is no element and is never a step's node. name == next is
// legal, and both are exact ·expanded names·: a wildcard or kind test builds
// none.
//
// It is a counter key (ctaKey), but no ctaTallied arm: whether one child is
// selected is decided by the report of the next child of E, which arrives
// after its own. Its counter is ctaSuccessionCount. It is a ctaRangeKey too,
// over the children named name it is drawn from.
type ctaChildrenFollowedBy struct {
	name xsd.QName
	next xsd.QName
}

// ctaRangeKey is the sealed sum of the keys a node-ranging quantifier counts
// the bindings satisfying its body under (ctaQuantified): `$v/@A`, the
// children carrying the attributes (ctaFilteredChildren), and
// `$v/following-sibling::*[1][self::M]`, the children followed by an M
// (ctaChildrenFollowedBy). The two body forms ctaParser.rangeBody parses close
// the set (STYLE T2's schema-closed-set exception).
type ctaRangeKey interface {
	ctaKey
	ctaRangeKey()
	// bound is the key counting every binding the quantifier ranges over, E's
	// children named as the key's: `N` of `every $v in N satisfies …`. It is
	// derived from the key and stored nowhere.
	bound() ctaCountPath
}

func (ctaFilteredChildren) ctaRangeKey()   {}
func (ctaChildrenFollowedBy) ctaRangeKey() {}

// bound is `N` over f's name.
func (f ctaFilteredChildren) bound() ctaCountPath {
	return ctaCountPath{axis: ctaCountChildren, name: f.name}
}

// bound is `N` over s's name.
func (s ctaChildrenFollowedBy) bound() ctaCountPath {
	return ctaCountPath{axis: ctaCountChildren, name: s.name}
}

// ctaDistinctNames is names with each name after its first occurrence dropped,
// the rest in written order.
func ctaDistinctNames(names []xsd.QName) []xsd.QName {
	var distinct []xsd.QName
	for _, n := range names {
		if !slices.Contains(distinct, n) {
			distinct = append(distinct, n)
		}
	}
	return distinct
}

// ctaSelectedElements is one element step, `N`, `./N` or `.//N` with a QName
// NameTest (xpath20.md §3.2.1.1, §3.2.4), which only the assertion façade
// admits (ctaFacade.elements) and only as the whole operand of fn:exists,
// fn:empty or an ·effective boolean value· (ctaParser.selectedElements): the
// element children of E named N, or every element below E named N and never E
// itself, as path's axis says. Like ctaChildPath it is never atomized
// (fn:exists, fn:empty `item()*`, §2.4.3 rule 2), so the step is never typed,
// a node of any type, ·nilled· or not, is selected, and how many there are is
// read off the [Tally] — under path itself, so `count(a) ge 1 and a` keeps one
// counter for both. It is not a counter key of its own (ctaTallied): path is.
//
// Its one constructor is ctaSelectedElementsOf, which refuses an attribute
// axis: an attribute step in those positions is ctaAttr or ctaTypedAttr.
type ctaSelectedElements struct{ path ctaCountPath }

// ctaSelectedElementsOf is the ctaSelectedElements over p, false where p's axis
// selects attributes.
func ctaSelectedElementsOf(p ctaCountPath) (ctaSelectedElements, bool) {
	switch p.axis {
	case ctaCountChildren, ctaCountDescendants:
		return ctaSelectedElements{path: p}, true
	case ctaCountOwnAttributes, ctaCountSubtreeAttributes:
		return ctaSelectedElements{}, false
	}
	return ctaSelectedElements{}, false
}

// ctaKey is the sealed sum of the keys a [Tally] keeps a counter under: the
// keys one [Tally.Element] report decides alone (ctaTallied), a child step
// filtered by its children's existence (ctaChildrenHaving), which the reports
// below the child decide, a child step filtered by a `preceding::` step
// (ctaChildrenPreceded), which the reports before the child decide, the
// children followed by a sibling of a name (ctaChildrenFollowedBy), which the
// report of the next child decides, and a path to an attribute
// (ctaAttributePath), whose values the [Tally.Attribute] calls after each
// report bring. The grammar closes the set (STYLE T2's schema-closed-set
// exception). Two keys are compared by same and never with ==, which a
// slice-holding arm cannot take.
type ctaKey interface {
	ctaKey()
	// selectsAttributesAt reports whether the key selects, or reads the value
	// of, an attribute node of any name of the element depth levels below E, 0
	// being E ([Tally.ReadsAttributesAt]).
	selectsAttributesAt(depth int) bool
	// same reports whether the key and other select the same nodes, which is
	// what makes one counter serve both.
	same(other ctaKey) bool
	// counter is a fresh counter for the key, which has counted nothing.
	counter() ctaCounter
}

// ctaTallied is the sealed sum of the keys whose every [Tally.Element] report
// is decided by that report alone, counted by ctaReportCount: an fn:count path
// (ctaCountPath), a filtered child step (ctaFilteredChildren), a union
// (ctaUnion), and a child path (ctaChildPath). The grammar closes the set
// (STYLE T2's schema-closed-set exception).
type ctaTallied interface {
	ctaKey
	ctaTallied()
	// selectsElement reports whether the key selects the element node whose
	// chain below E is path, from E's child down to the node inclusive, and
	// whose attribute nodes are named attrs ([Tally.Element]). An empty path
	// is E itself, which no key selects.
	selectsElement(path, attrs []xsd.QName) bool
	// selectsAttribute reports whether the key selects an attribute node named
	// name of the element depth levels below E, 0 being E.
	selectsAttribute(depth int, name xsd.QName) bool
}

func (ctaCountPath) ctaKey()        {}
func (ctaFilteredChildren) ctaKey() {}
func (ctaUnion) ctaKey()            {}
func (ctaChildPath) ctaKey()        {}
func (ctaChildrenHaving) ctaKey()   {}
func (ctaChildrenPreceded) ctaKey() {}

func (ctaChildrenFollowedBy) ctaKey() {}

func (ctaAttributePath) ctaKey() {}

func (ctaCountPath) ctaTallied()        {}
func (ctaFilteredChildren) ctaTallied() {}
func (ctaUnion) ctaTallied()            {}
func (ctaChildPath) ctaTallied()        {}

// counter is a fresh ctaReportCount over p.
func (p ctaCountPath) counter() ctaCounter { return &ctaReportCount{tallied: p} }

// counter is a fresh ctaReportCount over f.
func (f ctaFilteredChildren) counter() ctaCounter { return &ctaReportCount{tallied: f} }

// counter is a fresh ctaReportCount over u.
func (u ctaUnion) counter() ctaCounter { return &ctaReportCount{tallied: u} }

// counter is a fresh ctaReportCount over p.
func (p ctaChildPath) counter() ctaCounter { return &ctaReportCount{tallied: p} }

// counter is a fresh ctaInstanceCount over h, with no instance open.
func (h ctaChildrenHaving) counter() ctaCounter {
	return &ctaInstanceCount{having: h, shown: make([]bool, len(h.required))}
}

// selectsAttributesAt is false: h selects element children, and no
// attribute name decides which.
func (ctaChildrenHaving) selectsAttributesAt(int) bool { return false }

// same reports whether other is a ctaChildrenHaving over h's name requiring
// the same set of children, in any order, on ctaFilteredChildren.same's terms.
func (h ctaChildrenHaving) same(other ctaKey) bool {
	o, isHaving := other.(ctaChildrenHaving)
	return isHaving && o.name == h.name && len(o.required) == len(h.required) && ctaContainsAll(o.required, h.required)
}

// counter is a fresh ctaPrecedenceCount over p, with no element reported.
func (p ctaChildrenPreceded) counter() ctaCounter { return &ctaPrecedenceCount{preceded: p} }

// selectsAttributesAt is false at every depth: p selects element children by
// the names of the elements reported, and no attribute name decides which.
func (ctaChildrenPreceded) selectsAttributesAt(int) bool { return false }

// same reports whether other is a ctaChildrenPreceded over p's names and test.
func (p ctaChildrenPreceded) same(other ctaKey) bool {
	o, isPreceded := other.(ctaChildrenPreceded)
	return isPreceded && o == p
}

// counter is a fresh ctaSuccessionCount over s, with no child of E reported.
func (s ctaChildrenFollowedBy) counter() ctaCounter { return &ctaSuccessionCount{followed: s} }

// selectsAttributesAt is false at every depth: s selects element children by
// the names of the children reported, and no attribute name decides which.
func (ctaChildrenFollowedBy) selectsAttributesAt(int) bool { return false }

// same reports whether other is a ctaChildrenFollowedBy over s's name and next
// name.
func (s ctaChildrenFollowedBy) same(other ctaKey) bool {
	o, isFollowed := other.(ctaChildrenFollowedBy)
	return isFollowed && o == s
}

// selectsElement reports whether p selects the element whose chain below E is
// path: one named p.name, at depth 1 for `N` and at any depth from 1 for
// `.//N`, whatever its attributes. An attribute axis selects no element.
func (p ctaCountPath) selectsElement(path, _ []xsd.QName) bool {
	if len(path) == 0 || path[len(path)-1] != p.name {
		return false
	}
	switch p.axis {
	case ctaCountChildren:
		return len(path) == 1
	case ctaCountDescendants:
		return true
	case ctaCountOwnAttributes, ctaCountSubtreeAttributes:
		return false
	}
	return false
}

// selectsAttribute reports whether p selects an attribute named name of the
// element depth levels below E: E's own for `@N`, and E's or any element's
// below it for `.//@N`. An element axis selects no attribute.
func (p ctaCountPath) selectsAttribute(depth int, name xsd.QName) bool {
	if name != p.name {
		return false
	}
	switch p.axis {
	case ctaCountOwnAttributes:
		return depth == 0
	case ctaCountSubtreeAttributes:
		return depth >= 0
	case ctaCountChildren, ctaCountDescendants:
		return false
	}
	return false
}

// selectsAttributesAt reports whether p selects attributes of the element
// depth levels below E: `@N` E's own, at depth 0, and `.//@N` E's or any
// element's below it, at every depth from 0. An element axis selects none.
func (p ctaCountPath) selectsAttributesAt(depth int) bool {
	switch p.axis {
	case ctaCountOwnAttributes:
		return depth == 0
	case ctaCountSubtreeAttributes:
		return depth >= 0
	case ctaCountChildren, ctaCountDescendants:
		return false
	}
	return false
}

// same reports whether other is a ctaCountPath equal to p.
func (p ctaCountPath) same(other ctaKey) bool {
	o, isCount := other.(ctaCountPath)
	return isCount && o == p
}

// counter is a fresh ctaAttributeCollector over p, which holds no value.
func (p ctaAttributePath) counter() ctaCounter { return &ctaAttributeCollector{read: p} }

// selectsAttributesAt is true at the depth of the elements p's steps select,
// whose attribute names decide which values p reads, and false at every other.
func (p ctaAttributePath) selectsAttributesAt(depth int) bool { return depth == len(p.steps) }

// same reports whether other is a ctaAttributePath over p's steps and name:
// both were compiled against one [AttributeTypes], so their types agree.
func (p ctaAttributePath) same(other ctaKey) bool {
	o, isPath := other.(ctaAttributePath)
	return isPath && o.name == p.name && slices.Equal(o.steps, p.steps)
}

// reads reports whether p reads the value of the attribute named name of the
// element whose chain below E is path.
func (p ctaAttributePath) reads(path []xsd.QName, name xsd.QName) bool {
	return name == p.name && slices.Equal(path, p.steps)
}

// selectsElement reports whether path is p's parents followed by one name its
// last step matches, whatever the element's attributes.
func (p ctaChildPath) selectsElement(path, _ []xsd.QName) bool {
	depth := len(p.parents)
	return len(path) == depth+1 && slices.Equal(path[:depth], p.parents) && p.last.matches(path[depth])
}

// selectsAttribute is false: every step of p is on the child axis.
func (ctaChildPath) selectsAttribute(int, xsd.QName) bool { return false }

// selectsAttributesAt is false: every step of p is on the child axis.
func (ctaChildPath) selectsAttributesAt(int) bool { return false }

// same reports whether other is a ctaChildPath with p's parents and a last
// step of the same arm and name (ctaElementTest): `a/*` and `a/b` are two keys.
func (p ctaChildPath) same(other ctaKey) bool {
	o, isChild := other.(ctaChildPath)
	return isChild && slices.Equal(o.parents, p.parents) && o.last == p.last
}

// selectsElement reports whether the element whose chain below E is path is a
// child of E named f.name whose attribute nodes, attrs, include every name f
// requires.
func (f ctaFilteredChildren) selectsElement(path, attrs []xsd.QName) bool {
	return len(path) == 1 && path[0] == f.name && ctaContainsAll(attrs, f.required)
}

// selectsAttribute is false: f selects element children.
func (ctaFilteredChildren) selectsAttribute(int, xsd.QName) bool { return false }

// selectsAttributesAt is true at depth 1, E's children, whose attribute names
// decide whether f selects them, and false at every other depth.
func (ctaFilteredChildren) selectsAttributesAt(depth int) bool { return depth == 1 }

// same reports whether other is a ctaFilteredChildren over f's name requiring
// the same set of attributes, in any order: each holds its set without
// duplicates (ctaFilteredChildrenOf), so equal lengths and containment one way
// are set equality.
func (f ctaFilteredChildren) same(other ctaKey) bool {
	o, isFiltered := other.(ctaFilteredChildren)
	return isFiltered && o.name == f.name && len(o.required) == len(f.required) && ctaContainsAll(o.required, f.required)
}

// ctaContainsAll reports whether set holds every one of names.
func ctaContainsAll(set, names []xsd.QName) bool {
	for _, n := range names {
		if !slices.Contains(set, n) {
			return false
		}
	}
	return true
}

// selectsElement reports whether any operand of u selects the element.
func (u ctaUnion) selectsElement(path, attrs []xsd.QName) bool {
	for _, o := range u.operands {
		if o.selectsElement(path, attrs) {
			return true
		}
	}
	return false
}

// selectsAttribute reports whether any operand of u selects the attribute.
func (u ctaUnion) selectsAttribute(depth int, name xsd.QName) bool {
	for _, o := range u.operands {
		if o.selectsAttribute(depth, name) {
			return true
		}
	}
	return false
}

// selectsAttributesAt reports whether any operand of u reads the attribute
// names reported at depth.
func (u ctaUnion) selectsAttributesAt(depth int) bool {
	for _, o := range u.operands {
		if o.selectsAttributesAt(depth) {
			return true
		}
	}
	return false
}

// same reports whether other is a ctaUnion over the same operands, in any
// order: each holds its operands without duplicates (ctaUnionOf), so equal
// lengths and containment one way are set equality. No key is read for the
// nodes it selects, so `e | .//e` and `.//e` are two keys.
func (u ctaUnion) same(other ctaKey) bool {
	o, isUnion := other.(ctaUnion)
	if !isUnion || len(o.operands) != len(u.operands) {
		return false
	}
	for _, k := range u.operands {
		if !ctaHoldsPath(o.operands, k) {
			return false
		}
	}
	return true
}

func (ctaAttr) ctaValue()             {}
func (ctaTypedAttr) ctaValue()        {}
func (ctaTypedChild) ctaValue()       {}
func (ctaUntypedChild) ctaValue()     {}
func (ctaCandidate) ctaValue()        {}
func (ctaRangeItem) ctaValue()        {}
func (ctaBoundChild) ctaValue()       {}
func (ctaBoundSibling) ctaValue()     {}
func (ctaChildPath) ctaValue()        {}
func (ctaAttributePath) ctaValue()    {}
func (ctaChildrenHaving) ctaValue()   {}
func (ctaChildrenPreceded) ctaValue() {}
func (ctaSelectedElements) ctaValue() {}
func (ctaNoDocumentRoot) ctaValue()   {}
func (ctaNoContextItem) ctaValue()    {}
func (ctaLiteral) ctaValue()          {}
func (ctaCast) ctaValue()             {}
func (ctaCastable) ctaValue()         {}
func (ctaInstanceOf) ctaValue()       {}
func (ctaCount) ctaValue()            {}
func (ctaValueVar) ctaValue()         {}
func (ctaEmptyValue) ctaValue()       {}
func (ctaUntypedValue) ctaValue()     {}
func (ctaContextAtom) ctaValue()      {}

// ctaStatic is one operand's static type, which is what xpath20.md §3.5.2's
// casting rules dispatch on. It is a sealed sum of the three states this
// grammar can produce and not a datatype: an uncast UNTYPED attribute has no
// type ANNOTATION at all, because key-cta-ta-select clause 1 labels every node
// of the constructed instance untyped — and an assertion's attribute whose type
// is ·special· has a typed value of xs:untypedAtomic all the same
// (xpath-datamodel §3.3.1.2) — and the statically empty `$value` has no item to
// carry one.
type ctaStatic interface{ ctaStatic() }

// ctaUntypedAtomic is an uncast attribute operand, which atomizes to a single
// xs:untypedAtomic value (§3.13.4.1's note on the same "labeled as untyped"
// condition: "its atomized value will be a single atomic value of type
// untypedAtomic"; xpath-datamodel §3.3.1.2 for a ·special· type).
type ctaUntypedAtomic struct{}

// ctaTyped is an operand carrying a datatype: a Literal, the result of a cast,
// a constructor function, a castable or an instance-of expression, a typed
// attribute, an fn:count call, an arithmetic result, the result of an F&O
// string or sequence function or of fn:current-date, or each item of `$value`.
// It carries the COMPONENT alone — st.Name() is the name, and storing both
// would be two encodings of one fact (STYLE D3).
type ctaTyped struct{ st *xsd.SimpleType }

// ctaEmptySequence is the statically empty operand, ctaEmptyValue: it yields
// no item, so no operator is applied to it and no type is involved
// (ctaTypes.againstEmpty).
type ctaEmptySequence struct{}

func (ctaUntypedAtomic) ctaStatic() {}
func (ctaTyped) ctaStatic()         {}
func (ctaEmptySequence) ctaStatic() {}

// ctaStaticOf reports the static type of one [14] ta-ValueExpr. ctaAttr,
// ctaUntypedChild, ctaUntypedValue, ctaContextAtom and a ctaAttributePath over
// a ·special· type are the untyped arms, and so is an fn:distinct-values call
// over one, whose static type is its operand's; so are ctaNoDocumentRoot and
// ctaNoContextItem: each raises before any item exists, so its static type
// decides only whether a comparison over it compiles, never an answer. A
// ctaChildPath, ctaChildrenHaving, ctaChildrenPreceded or ctaSelectedElements
// never reaches here: ctaParser.childPath, ctaParser.childrenHaving,
// ctaParser.childrenPreceded and ctaParser.selectedElements build them only
// where no static type is asked. Every other operand is typed by the type it
// carries (ctaCarriedType).
func ctaStaticOf(v ctaValue) ctaStatic {
	switch n := v.(type) {
	case ctaDistinctValues:
		return ctaStaticOf(n.operand)
	case ctaEmptyValue:
		return ctaEmptySequence{}
	}
	if st, typed := ctaCarriedType(v); typed {
		return ctaTyped{st: st}
	}
	return ctaUntypedAtomic{}
}

// ctaCarriedType is the simple type a typed operand carries — its static type,
// which ctaStaticOf reports and ctaTypes.castSource judges a cast by — or false
// for one that carries none: an untyped or statically empty operand, and an
// fn:distinct-values call, whose static type is its operand's. A cast carries
// its target, an fn:string call its cast's, `$value` the type of each of its
// items, and a range variable over `$value` that same type (ctaRangeItem); a
// range variable over a typed child step carries the step's type
// (ctaBoundChild), and `$v/following-sibling::M[1]` M's (ctaBoundSibling). A
// path to an attribute carries its compiled type, unless that type is ·special·
// and its values xs:untypedAtomic; an fn:sum call carries its static type
// (ctaSum), and `instance of` over one xs:boolean (ctaSumInstance).
func ctaCarriedType(v ctaValue) (*xsd.SimpleType, bool) {
	switch n := v.(type) {
	case ctaAttributePath:
		if n.st.IsSpecial() {
			return nil, false
		}
		return n.st, true
	case ctaSum:
		return n.st, true
	case ctaSumInstance:
		return n.st, true
	case ctaLiteral:
		return n.st, true
	case ctaCast:
		return n.target, true
	case ctaCastable:
		return n.st, true
	case ctaInstanceOf:
		return n.st, true
	case ctaTypedAttr:
		return n.st, true
	case ctaTypedChild:
		return n.st, true
	case ctaCandidate:
		return n.st, true
	case ctaRangeItem:
		return n.st, true
	case ctaBoundChild:
		return n.st, true
	case ctaBoundSibling:
		return n.step.st, true
	case ctaCount:
		return n.st, true
	case ctaArith:
		return n.st, true
	case ctaMatch:
		return n.st, true
	case ctaUnaryString:
		return n.st, true
	case ctaPresence:
		return n.st, true
	case ctaStringFunction:
		return n.cast.target, true
	case ctaConcat:
		return n.st, true
	case ctaCurrentDate:
		return n.st, true
	case ctaNoFocus:
		return n.st, true
	case ctaNamespaceURI:
		return n.st, true
	case ctaValueVar:
		return n.atom, true
	case ctaIntegerRanges:
		return n.st, true
	case ctaStringSequence:
		return n.st, true
	}
	return nil, false
}

// ctaComparator is one of the six comparison operators: a [13] ta-Comparator
// spelling in a general comparison (xpath20.md §3.5.2, ctaCompare), or a [23]
// ValueComp spelling in a value comparison (§3.5.1, ctaValueCompare). Each is
// named for the B.2 rows both spellings of it read — `=` and `eq` the
// `A eq B` row — and the node it sits on is what says how it quantifies.
type ctaComparator byte

const (
	ctaEqual ctaComparator = iota
	ctaNotEqual
	ctaLess
	ctaLessEqual
	ctaGreater
	ctaGreaterEqual
)

// ctaAnswer is what one node of a compiled {test} evaluates to: an XPath
// boolean, or ctaError — the state key-cta-ta-select clause 2 (§3.12.4)
// describes as "a dynamic error or a type error is raised during evaluation".
//
// The third state exists because clause 2's subject is "the {test}" and not
// the node that raised. A node that raises therefore reports it rather than
// deciding for the whole expression: fn:not propagates it and xpath20.md
// §3.6's truth tables say what and/or do with it, and only
// [CTATest.Evaluate] and [AssertionTest.Evaluate] substitute false for it —
// the second because cvc-assertion's subject is the {test} too.
//
// ctaFalse is the zero value, which is what the nil root of a zero CTATest or
// AssertionTest answers.
type ctaAnswer byte

const (
	ctaFalse ctaAnswer = iota
	ctaTrue
	ctaError
)

// ctaAnswerOf lifts a decided boolean into a ctaAnswer.
func ctaAnswerOf(b bool) ctaAnswer {
	if b {
		return ctaTrue
	}
	return ctaFalse
}

// negated is fn:not (xpath20.md §3.6): it inverts a boolean and propagates a
// raised error unchanged — "If an error is encountered in finding the
// effective boolean value of its operand, fn:not raises the same error."
// fn:not is a function and not a logical operator, so no truth table lets it
// absorb its operand's error into a boolean; only the {test} as a whole
// absorbs it, at [CTATest.Evaluate] or [AssertionTest.Evaluate].
func (a ctaAnswer) negated() ctaAnswer {
	if a == ctaError {
		return ctaError
	}
	return ctaAnswerOf(a == ctaFalse)
}

// ctaEval evaluates one boolean-valued node.
//
// The default arm covers exactly one shape: the nil root of a zero CTATest,
// which no successful [CompileCTATest] produces. Every branch of the sum above
// is named.
func ctaEval(x ctaExpr, env ctaEnv) ctaAnswer {
	switch n := x.(type) {
	case ctaOr:
		return n.eval(env)
	case ctaAnd:
		return n.eval(env)
	case ctaNot:
		return ctaEval(n.operand, env).negated()
	case ctaCompare:
		return n.eval(env)
	case ctaValueCompare:
		return n.eval(env)
	case ctaEffectiveBoolean:
		return n.eval(env)
	case ctaTypeError:
		return ctaError
	case ctaIf:
		return n.eval(env)
	case ctaPrefixMember:
		return n.eval(env)
	case ctaQuantified:
		return n.eval(env)
	case ctaQuantifiedValue:
		return n.eval(env)
	case ctaQuantifiedChildren:
		return n.eval(env)
	default:
		return ctaFalse
	}
}

// eval decides a quantified expression over `$value` on xpath20.md §3.9's
// terms, binding the range variable to each item of the binding sequence in
// sequence order (ctaEnv.rangeItem) and evaluating the body for it, the
// answers aggregated by ctaQuantify. A binding sequence that raises is the
// error. Over no binding — an empty list, or the nil `$value` cvc-assertion
// clause 2.3.2 binds — `every` is true and `some` false.
func (n ctaQuantifiedValue) eval(env ctaEnv) ctaAnswer {
	items, bound := ctaItemOf(n.over, n.over.atom, env).(ctaAtoms)
	if !bound {
		return ctaError
	}
	return ctaQuantify(n.q, len(items.vs), func(i int) ctaAnswer {
		inner := env
		inner.rangeItem = items.vs[i]
		return ctaEval(n.body, inner)
	})
}

// eval decides a quantified expression over a typed child step on xpath20.md
// §3.9's terms: the children [ChildElements] yields are read once, in document
// order, and the range variable is bound to each named n.over.name in turn
// (ctaEnv.rangeChild) — all but the last of them where n.dropLast is true, the
// `[position() lt last()]` window (§3.2.2) — and the body evaluated for it, the
// answers aggregated by ctaQuantify. So over one child the window is empty, and
// `every` true. A child of that name whose value is neither [Typed] nor nil
// breaks the obligation [ChildElements] states and raises before any binding
// is evaluated, on ctaEachChild's terms; a body's read of a child of another
// name checks that child's arm itself (ctaBoundItem, ctaBoundNodes).
//
// The input is ctaTypedInput by construction (ctaInput); the other arm carries
// no children and binds nothing, unreachably.
func (n ctaQuantifiedChildren) eval(env ctaEnv) ctaAnswer {
	in, typed := env.input.(ctaTypedInput)
	if !typed {
		return ctaError
	}
	var children []ChildElement
	var bindings []int
	broken := false
	in.children(func(c ChildElement) bool {
		if c.name == n.over.name {
			if !ctaTypedOrNilled(c) {
				broken = true
				return false
			}
			bindings = append(bindings, len(children))
		}
		children = append(children, c)
		return true
	})
	if broken {
		return ctaError
	}
	if n.dropLast && len(bindings) > 0 {
		bindings = bindings[:len(bindings)-1]
	}
	return ctaQuantify(n.q, len(bindings), func(i int) ctaAnswer {
		inner := env
		inner.rangeChild = ctaChildBinding{children: children, at: bindings[i]}
		return ctaEval(n.body, inner)
	})
}

// ctaTypedOrNilled reports whether c's value is of the arm a typed child step
// reads: [Typed], or nil for a ·nilled· child ([ChildElements]).
func ctaTypedOrNilled(c ChildElement) bool {
	if c.v == nil {
		return true
	}
	_, isTyped := c.v.(tvTyped)
	return isTyped
}

// ctaBoundItem atomizes selected, the zero or one child a ctaBoundChild or a
// ctaBoundSibling selects (ctaChildBinding), each value of type st, and
// converts the atoms into c on ctaTypedChildItem's terms: a ·nilled· child
// contributes none, and a value of the other arm raises, breaking the
// obligation [ChildElements] states.
func ctaBoundItem(selected []ChildElement, st, c *xsd.SimpleType, env ctaEnv) ctaItem {
	vs := make([]value.Value, 0, len(selected))
	for _, child := range selected {
		if child.v == nil {
			continue
		}
		tv, isTyped := child.v.(tvTyped)
		if !isTyped {
			return ctaRaised{}
		}
		vs = append(vs, tv.v)
	}
	return ctaPromoted(vs, st, c, env)
}

// ctaBoundNodes is how many nodes selected holds, a ·nilled· child counting as
// one, reporting false for a value of the other arm, on ctaBoundItem's terms.
func ctaBoundNodes(selected []ChildElement) (int, bool) {
	for _, child := range selected {
		if !ctaTypedOrNilled(child) {
			return 0, false
		}
	}
	return len(selected), true
}

// ctaQuantify is the answer of a quantifier q over bindings bindings, each(i)
// being the body's answer for the i-th, walked in binding order.
//
// The answer is xpath20.md §3.6's over the bindings' answers, `some` on
// ctaOr.eval's terms and `every` on ctaAnd.eval's: a decisive binding — true
// for `some`, false for `every` — decides the whole and stops the walk,
// whether or not an earlier binding raised, and otherwise an error survives to
// the end. §3.9 leaves the evaluation order and so which of an error and a
// decisive answer wins implementation-dependent, and this policy answers the
// same whatever the order (STYLE D1). Over no binding `every` is true and
// `some` false.
func ctaQuantify(q ctaQuantifier, bindings int, each func(i int) ctaAnswer) ctaAnswer {
	decisive := ctaAnswerOf(q == ctaSome)
	answer := decisive.negated()
	for i := range bindings {
		got := each(i)
		if got == decisive {
			return decisive
		}
		if got == ctaError {
			answer = ctaError
		}
	}
	return answer
}

// eval decides a quantified expression on xpath20.md §3.9's terms over the
// counts the evaluation's [Tally] holds: `some` is true where the bindings
// satisfying the body number one or more, and `every` where they number as
// many as the bindings — so `every` over no binding is true and `some` false.
// A count the Tally does not hold raises, which [AssertionTest.Evaluate]
// refuses before the tree is read (ctaTalliedNodes).
func (n ctaQuantified) eval(env ctaEnv) ctaAnswer {
	satisfied, ok := ctaTalliedNodes(n.satisfying, env)
	if !ok {
		return ctaError
	}
	switch n.q {
	case ctaSome:
		return ctaAnswerOf(satisfied > 0)
	case ctaEvery:
		bound, ok := ctaTalliedNodes(n.satisfying.bound(), env)
		if !ok {
			return ctaError
		}
		return ctaAnswerOf(satisfied == bound)
	}
	// The zero quantifier, which no node holds.
	return ctaError
}

// eval decides a conditional expression on xpath20.md §3.8's terms: the
// ·effective boolean value· of the test selects the branch whose value is the
// expression's, and the other branch is NOT evaluated, so a dynamic error it
// would raise is never raised ("the conditional expression ignores (does not
// raise) any dynamic errors encountered in the else-expression"). An error
// the test raises — err:FORG0006 among them (§2.4.3) — or the selected branch
// raises is the expression's, which the enclosing operators carry on
// ctaAnswer's terms.
func (n ctaIf) eval(env ctaEnv) ctaAnswer {
	test := ctaEval(n.test, env)
	if test == ctaError {
		return ctaError
	}
	if test == ctaTrue {
		return ctaEval(n.then, env)
	}
	return ctaEval(n.otherwise, env)
}

// eval decides an or-expression against xpath20.md §3.6's or-table, read with
// XPath 1.0 compatibility mode false (xpath-valid clause 2.2.1).
//
// A true operand determines the whole expression and stops the walk, whether
// or not an earlier operand raised: the table's two cells pairing an error
// with a true permit either true or the error, and true is the answer an
// or-expression whose determining operand succeeded has always had. Every
// other cell holding an error is the error, so an error survives to the end of
// the loop when nothing decides.
func (n ctaOr) eval(env ctaEnv) ctaAnswer {
	answer := ctaFalse
	for _, o := range n.operands {
		got := ctaEval(o, env)
		if got == ctaTrue {
			return ctaTrue
		}
		if got == ctaError {
			answer = ctaError
		}
	}
	return answer
}

// eval decides an and-expression on ctaOr.eval's terms, against §3.6's
// and-table: a false operand determines the expression and stops the walk, and
// an error otherwise survives to the end.
func (n ctaAnd) eval(env ctaEnv) ctaAnswer {
	answer := ctaTrue
	for _, o := range n.operands {
		got := ctaEval(o, env)
		if got == ctaFalse {
			return ctaFalse
		}
		if got == ctaError {
			answer = ctaError
		}
	}
	return answer
}

// eval decides one general comparison (xpath20.md §3.5.2), which is
// EXISTENTIAL over the two operand sequences: "the result of the comparison is
// true if and only if there is a pair of atomic values, one in the first
// operand sequence and the other in the second operand sequence, that have the
// required magnitude relationship. Otherwise the result of the comparison is
// false." An operand matching no attribute forms no pair and so decides false,
// which is not an error.
//
// A pair that raises decides for the whole comparison, on the same latitude the
// operands take below; the pairs are walked in document order, so which one
// that is does not vary between runs (STYLE D1).
//
// A RAISED operand decides first, whatever the other operand is: §3.5.2 says
// a general comparison "may raise a dynamic error as soon as it encounters an
// error in evaluating either operand" — which is also why an operand is
// converted whole, so one item of a matched sequence failing §3.5.2 clause 2's
// cast raises for the operand rather than being dropped from it.
//
// Both operands reach the comparison already converted into c.comparison,
// which is where §3.5.2 clause 2's casts happened, so what is left here is
// clause 3's value comparison — and B.1's URI promotion, which is applied
// HERE and not in the comparison type: an xs:anyURI comparison type is decided
// through the default collation exactly as an xs:string one is, because B.1
// rule 2 promotes xs:anyURI to xs:string and B.2 then gives it the same six
// fn:compare rows. Promoting the TYPE instead would change what clause 2.4
// casts an xs:untypedAtomic operand to and so change the answer
// (ctaTypes.comparison).
//
// xs:boolean takes a third route for the same kind of reason: the operator
// functions B.2 names for it are defined over the two values themselves and
// not over an order the value space carries (holdsBoolean). The date/time
// family takes a fourth, because F&O §10.4's operator functions compare under
// the implicit timezone and the value space carries none
// (holdsAtImplicitTimezone).
func (c ctaCompare) eval(env ctaEnv) ctaAnswer {
	l, leftAtoms := ctaItemOf(c.left, c.comparison, env).(ctaAtoms)
	r, rightAtoms := ctaItemOf(c.right, c.comparison, env).(ctaAtoms)
	if !leftAtoms || !rightAtoms {
		return ctaError // ctaRaised, the sum's only other arm
	}
	for _, lv := range l.vs {
		for _, rv := range r.vs {
			if got := ctaHoldsPair(c.op, c.comparison, lv, rv, env); got != ctaFalse {
				return got
			}
		}
	}
	return ctaFalse
}

// ctaHoldsPair decides op between two values both already converted into the
// comparison type c. It is the one decision both comparison nodes reach: one
// PAIR of a general comparison's existential is §3.5.2 clause 3's value
// comparison, and a value comparison (§3.5.1) is that same application of the
// operator to its two singletons.
func ctaHoldsPair(op ctaComparator, c *xsd.SimpleType, l, r value.Value, env ctaEnv) ctaAnswer {
	if ctaStringLike(c) {
		return op.holdsCollated(l, r)
	}
	if c.Name() == ctaBuiltin("boolean") {
		return op.holdsBoolean(l, r, c, env)
	}
	if ctaDateTimeFamily(c) {
		return op.holdsAtImplicitTimezone(l, r, c, env)
	}
	return op.holdsBetween(l, r)
}

// ctaDateTimeFamily reports whether c, a comparison type, is one of the eight
// primitives F&O §10.4's date and time comparison functions are defined over:
// xs:dateTime (and so xs:dateTimeStamp, by subtype substitution), xs:time,
// xs:date, xs:gYearMonth, xs:gYear, xs:gMonthDay, xs:gDay and xs:gMonth.
// Every comparison type in the family that reaches a pair is the primitive
// itself, because ctaTypes.shared and ctaTypes.untypedAgainst answer only the
// two duration subtypes below their primitive, so the name decides.
// ctaTypes.againstEmpty can settle xs:dateTimeStamp or a user date subtype —
// `@dts = ()` — but over the empty sequence, which forms no pair.
func ctaDateTimeFamily(c *xsd.SimpleType) bool {
	switch c.Name() {
	case ctaBuiltin("dateTime"), ctaBuiltin("time"), ctaBuiltin("date"),
		ctaBuiltin("gYearMonth"), ctaBuiltin("gYear"), ctaBuiltin("gMonthDay"),
		ctaBuiltin("gDay"), ctaBuiltin("gMonth"):
		return true
	}
	return false
}

// eval decides one value comparison (xpath20.md §3.5.1), whose steps are
// applied to each operand in order, the left one first — the order is
// implementation-dependent, and fixing it fixes which answer a pair of faulty
// operands gets (STYLE D1):
//
//   - an operand that raised is the error;
//   - step 2, an EMPTY atomized operand: "the result of the value comparison is
//     an empty sequence";
//   - step 3, an operand of more than one item: err:XPTY0004.
//
// Step 4's xs:untypedAtomic cast and the conversion to the least common type
// both happened in converting each operand into c.comparison
// (ctaTypes.valueComparison), so what remains is the operator, on the one pair.
//
// The empty sequence is answered as ctaFalse, and that is exact rather than an
// approximation: every consumer of an [11] ta-BooleanExpr takes its ·effective
// boolean value· — §3.6's and/or and fn:not over their operands, and clause 3
// of cvc-assertion over the {test} — and the effective boolean value of the
// empty sequence is false (§2.4.3 rule 1). So `not(@x eq 1)` over an E without
// @x is true, where an err:XPTY0004 under the same fn:not would stay an error.
func (c ctaValueCompare) eval(env ctaEnv) ctaAnswer {
	l, settled, single := ctaSingletonOperand(c.left, c.comparison, env)
	if !single {
		return settled
	}
	r, settled, single := ctaSingletonOperand(c.right, c.comparison, env)
	if !single {
		return settled
	}
	return ctaHoldsPair(c.op, c.comparison, l, r, env)
}

// ctaSingletonOperand evaluates one value-comparison operand into c, reporting
// its one atomic value, or false with the answer the whole comparison takes
// without applying its operator: ctaError for a raised operand or one of two
// or more items (§3.5.1 step 3, err:XPTY0004), and ctaFalse — the empty
// sequence, read as ctaValueCompare.eval states — for an empty one (step 2).
func ctaSingletonOperand(v ctaValue, c *xsd.SimpleType, env ctaEnv) (value.Value, ctaAnswer, bool) {
	atoms, converted := ctaItemOf(v, c, env).(ctaAtoms)
	if !converted {
		return nil, ctaError, false
	}
	if len(atoms.vs) == 0 {
		return nil, ctaFalse, false
	}
	if len(atoms.vs) > 1 {
		return nil, ctaError, false // err:XPTY0004
	}
	return atoms.vs[0], ctaFalse, true
}

// eval decides the ·effective boolean value· of a bare ValueExpr (xpath20.md
// §2.4.3, the fn:boolean rules quoted there).
//
// An AttrName, untyped or typed, a child-axis or element step and a child path
// evaluate to a sequence of NODES rather than to atomic values, so each takes
// rule 2 ("a sequence whose first item is a node") whenever its NameTest
// matches at all and rule 1 (the empty sequence) when it matches nothing, and
// no type of its own is involved — a ·nilled· child is a node all the same.
// Rule 2 holds whatever the sequence's LENGTH, which is what a wildcard
// NameTest and a repeated child make observable. A rooted path raises
// err:XPDY0050, and a read of an absent context item err:XPDY0002.
// ctaStep.nodes is that reading, which fn:empty and fn:exists share. `$value`
// is atomic values and no node: the statically empty one is rule 1's false, the
// bound typed one is decided by ctaBoolean, a list of two or more items
// included, and the untyped one by rule 4 (ctaUntypedBoolean). An
// fn:distinct-values call is decided by ctaBoolean over the items it keeps,
// read in the type it compares them in — xs:string for an xs:untypedAtomic
// operand, whose rule 4 is xs:string's. Every other operand is a singleton
// atomic value or the empty sequence, which ctaBoolean decides.
func (e ctaEffectiveBoolean) eval(env ctaEnv) ctaAnswer {
	if step, isStep := e.operand.(ctaStep); isStep {
		nodes, ok := step.nodes(env)
		if !ok {
			return ctaError
		}
		return ctaAnswerOf(nodes != 0)
	}
	switch n := e.operand.(type) {
	case ctaLiteral:
		return ctaBoolean(e.operand, n.st, env)
	case ctaCast:
		return ctaBoolean(e.operand, n.target, env)
	case ctaCastable:
		return ctaBoolean(e.operand, n.st, env)
	case ctaInstanceOf:
		return ctaBoolean(e.operand, n.st, env)
	case ctaCount:
		return ctaBoolean(e.operand, n.st, env)
	case ctaArith:
		return ctaBoolean(e.operand, n.st, env)
	case ctaMatch:
		return ctaBoolean(e.operand, n.st, env)
	case ctaUnaryString:
		return ctaBoolean(e.operand, n.st, env)
	case ctaPresence:
		return ctaBoolean(e.operand, n.st, env)
	case ctaStringFunction:
		return ctaBoolean(e.operand, n.cast.target, env)
	case ctaConcat:
		return ctaBoolean(e.operand, n.st, env)
	case ctaCurrentDate:
		return ctaBoolean(e.operand, n.st, env)
	case ctaNoFocus:
		return ctaBoolean(e.operand, n.st, env)
	case ctaNamespaceURI:
		return ctaBoolean(e.operand, n.st, env)
	case ctaDistinctValues:
		return ctaBoolean(e.operand, n.st, env)
	case ctaSum:
		return ctaBoolean(e.operand, n.st, env)
	case ctaSumInstance:
		return ctaBoolean(e.operand, n.st, env)
	case ctaValueVar:
		return ctaBoolean(e.operand, n.atom, env)
	case ctaRangeItem:
		return ctaBoolean(e.operand, n.st, env)
	case ctaEmptyValue:
		return ctaFalse
	case ctaUntypedValue:
		return ctaUntypedBoolean(env)
	default:
		return ctaFalse
	}
}

// ctaBoolean is fn:boolean over a singleton atomic operand of type st
// (xpath20.md §2.4.3 rules 1 and 3 through 6), which it decides in st's
// {primitive type definition}: the rules quantify over "xs:string ... or a
// type derived from one of these", and the primitive is what answers that for
// every builtin at once instead of a name list per rule.
//
// Rule 6 — "in all other cases, fn:boolean raises a type error [err:FORG0006]"
// — is the fallthrough, and it is reachable: this grammar can cast to any
// builtin atomic datatype, and only the boolean, string, anyURI and numeric
// families have a rule.
//
// An unresolvable primitive is ctaError on the same terms as any other type
// fault (ctaValidate). It is unreachable for a tree this package compiled,
// which resolved the primitive of every literal and every cast target before
// admitting it.
//
// A sequence of TWO OR MORE items is rule 6 as well — fn:boolean is defined
// over "a sequence whose first item is not a node" of length one, and raises
// err:FORG0006 otherwise. A `$value` over a list {simple type definition}
// reaches it with its items; a cast can put a wildcard-matched sequence here
// too, but raises err:XPTY0004 for that length before this is reached.
func ctaBoolean(v ctaValue, st *xsd.SimpleType, env ctaEnv) ctaAnswer {
	p, err := st.Primitive(env.types)
	if err != nil || p == nil {
		return ctaError
	}
	atoms, converted := ctaItemOf(v, p, env).(ctaAtoms)
	if !converted {
		return ctaError
	}
	if len(atoms.vs) == 0 {
		return ctaFalse
	}
	if len(atoms.vs) > 1 {
		return ctaError // err:FORG0006
	}
	return ctaBooleanOf(atoms.vs[0], p, env)
}

// ctaBooleanOf applies fn:boolean's per-type rules to one atomic value of
// primitive type p, through the capability interfaces the value itself carries
// (STYLE T2) and never a type switch over a backend's concrete types.
//
//   - Rule 3, xs:boolean: the value unchanged, read as equality against the
//     value of the lexical "true".
//   - Rule 4, xs:string and xs:anyURI: false iff the value has zero length,
//     which is [value.Lengthed]'s answer and the same one the length facet
//     reads.
//   - Rule 5, the numeric types: false iff NaN or numerically zero. NaN is
//     detected as a value UNEQUAL TO ITSELF, which [value.Eq] states outright
//     for float and double, so no NaN literal or backend-specific probe is
//     needed.
//   - Rule 6, everything else: err:FORG0006, hence ctaError.
func ctaBooleanOf(v value.Value, p *xsd.SimpleType, env ctaEnv) ctaAnswer {
	eq, comparable := v.(value.Eq)
	switch p.Name() {
	case ctaBuiltin("boolean"):
		return ctaTruth(v, p, env)
	case ctaBuiltin("string"), ctaBuiltin("anyURI"):
		measured, lengthed := v.(value.Lengthed)
		if !lengthed {
			return ctaError
		}
		return ctaAnswerOf(measured.Len() != 0)
	case ctaBuiltin("decimal"), ctaBuiltin("float"), ctaBuiltin("double"):
		if !comparable {
			return ctaError
		}
		if !eq.Eq(v) {
			return ctaFalse
		}
		return ctaEqualsLexical(eq, p, "0", env).negated()
	}
	return ctaError
}

// ctaEqualsLexical reports whether eq equals the value of lexical in type p,
// which is how fn:boolean's rules 3 and 5 reach the two constants they name
// (true, and numeric zero) without any backend-specific value construction.
// A lexical outside p's value space is ctaError, and neither of the two is:
// "true" is in xs:boolean's lexical space and "0" in every numeric one.
func ctaEqualsLexical(eq value.Eq, p *xsd.SimpleType, lexical string, env ctaEnv) ctaAnswer {
	against, validated := ctaValidated(ctaValidate(lexical, p, env))
	if !validated {
		return ctaError
	}
	return ctaAnswerOf(eq.Eq(against))
}

// ctaItem is what one [14] ta-ValueExpr contributes to the operator above it,
// and it is a sequence or a raised error rather than a value and a presence
// flag: xpath20.md §3.10.2 rule 3 splits an EMPTY operand into the empty
// sequence (with `?`) and a raised err:XPTY0004 (without it), and those two are
// the opposite of each other under fn:not. It is a sealed sum (STYLE T1/T2) so
// no third state is representable.
type ctaItem interface{ ctaItem() }

// ctaAtoms is the sequence of atomic values an operand yields, in document
// order, already in the type the enclosing operator asked for. A zero-length
// slice IS the empty sequence — the XDM's own encoding, which a [17]
// ta-AttrName whose NameTest matches nothing produces and which a cast written
// with `?` produces over it — so no separate absence marker exists to disagree
// with it (STYLE D3).
type ctaAtoms struct{ vs []value.Value }

// ctaRaised is a dynamic or type error raised while producing the item —
// err:FORG0001 from a lexical or facet mismatch, err:XPTY0004 from an empty
// operand under a cast written without `?`, from a cast over a sequence of two
// or more items, from a cast this processor does not support, or from a
// function argument xpath20.md §3.1.5's conversion does not match
// (ctaStringOf), and an arithmetic operator's err:FOAR0001 and err:FOAR0002
// (ctaArithItem). Which of them it was is not carried: key-cta-ta-select
// clause 2 (§3.12.4) gives them all the same consequence, as cvc-assertion
// (§3.13.4.1) does for an assertion, and the {test} is where that consequence
// is applied.
type ctaRaised struct{}

func (ctaAtoms) ctaItem()  {}
func (ctaRaised) ctaItem() {}

// ctaSingleton is the one-item sequence a datatype validation yields.
func ctaSingleton(v value.Value) ctaItem { return ctaAtoms{vs: []value.Value{v}} }

// ctaValidated is the single value a ctaValidate or ctaPromote result carries,
// reporting false where it raised: both answer a SINGLETON sequence or
// ctaRaised, and their callers want the value itself.
func ctaValidated(i ctaItem) (value.Value, bool) {
	atoms, converted := i.(ctaAtoms)
	if !converted || len(atoms.vs) != 1 {
		return nil, false
	}
	return atoms.vs[0], true
}

// ctaItemOf evaluates one [14] ta-ValueExpr to the sequence it yields,
// converted into type c — the type the enclosing operator compares or reads it
// in.
//
// The arms are the ways an item acquires a type:
//
//   - an UNTYPED attribute, and each child of mixed content, is
//     xs:untypedAtomic, which §3.5.2's casting rules cast STRAIGHT to c
//     (clause 1's xs:string, or clause 2's type chosen from the other
//     operand). No intermediate type exists to cast through.
//   - `.` is E's string value as xs:untypedAtomic, cast
//     straight to c as an untyped attribute is.
//   - a TYPED attribute, each typed child, a LITERAL, an fn:count call's
//     xs:integer, each item of an integer or string sequence, each item of
//     `$value` and the item a quantifier's range variable is bound to carry
//     their own type and are converted to c, which is a no-op
//     wherever the two coincide; the statically empty `$value` yields nothing
//     to convert.
//   - an F&O function's result is of its own result type — an
//     fn:distinct-values call's items of the type it compares them in, and an
//     fn:sum call's of the type its value carries (ctaSum.total) — and
//     converted to c on the typed operands' terms, once the function has
//     been applied to its arguments (ctafunc.go).
//   - a path to an attribute below E yields each value the [Tally] collected
//     for it, typed or xs:untypedAtomic, on the typed attribute's or the
//     untyped attribute's terms (ctaAttributePathItem).
//   - a rooted path raises err:XPDY0050 before it yields anything, and a read
//     of an absent context item, or of an absent focus by fn:position or
//     fn:last, err:XPDY0002.
//   - an ARITHMETIC result is of its own result type and converted to c on
//     the typed operands' terms, once its operands have been converted into
//     its operation type and computed (ctaArithItem).
//   - a CAST evaluates its operand IN THE TARGET TYPE first, because that cast
//     is the expression the author wrote and its failure is the author's
//     err:FORG0001, and only then converts the result to c. Evaluating it
//     straight into c instead would let `@n cast as xs:integer` accept "3.5"
//     whenever the comparison happened to run in xs:double.
//   - a CASTABLE expression evaluates its cast in the target type, as a cast
//     does, and converts the xs:boolean of whether it raised to c
//     (ctaCastableItem).
//   - an INSTANCE OF expression counts its atomized operand's items and
//     converts the xs:boolean of whether they match to c (ctaInstanceOfItem).
//
// The default arm is unreachable: every branch of the ctaValue sum is named.
func ctaItemOf(v ctaValue, c *xsd.SimpleType, env ctaEnv) ctaItem {
	switch n := v.(type) {
	case ctaAttr:
		return ctaAttrItem(n, c, env)
	case ctaTypedAttr:
		return ctaTypedAttrItem(n, c, env)
	case ctaTypedChild:
		return ctaTypedChildItem(n, c, env)
	case ctaUntypedChild:
		return ctaUntypedChildItem(n, c, env)
	case ctaCandidate:
		return ctaPromoted(env.candidate, n.st, c, env)
	case ctaRangeItem:
		return ctaPromote(env.rangeItem, n.st, c, env)
	case ctaBoundChild:
		return ctaBoundItem(env.rangeChild.bound(), n.st, c, env)
	case ctaBoundSibling:
		return ctaBoundItem(env.rangeChild.next(n.step.name), n.step.st, c, env)
	case ctaChildPath:
		// Never reached: ctaParser.childPath builds the node only where its
		// nodes are counted and no item is read (ctaStep.nodes).
		return ctaRaised{}
	case ctaChildrenHaving:
		// Never reached, on ctaChildPath's terms
		// (ctaParser.childrenHaving).
		return ctaRaised{}
	case ctaChildrenPreceded:
		// Never reached, on ctaChildPath's terms
		// (ctaParser.childrenPreceded).
		return ctaRaised{}
	case ctaSelectedElements:
		// Never reached, on ctaChildPath's terms
		// (ctaParser.selectedElements).
		return ctaRaised{}
	case ctaNoDocumentRoot:
		return ctaRaised{} // err:XPDY0050
	case ctaNoContextItem:
		return ctaRaised{} // err:XPDY0002
	case ctaNoFocus:
		return ctaRaised{} // err:XPDY0002
	case ctaLiteral:
		return ctaConvert(n.text, n.st, c, env)
	case ctaCast:
		return ctaCastItem(n, c, env)
	case ctaCastable:
		return ctaCastableItem(n, c, env)
	case ctaInstanceOf:
		return ctaInstanceOfItem(n, c, env)
	case ctaCount:
		return ctaCountItem(n, c, env)
	case ctaArith:
		return ctaArithItem(n, c, env)
	case ctaMatch:
		return ctaMatchItem(n, c, env)
	case ctaUnaryString:
		return ctaUnaryStringItem(n, c, env)
	case ctaPresence:
		return ctaPresenceItem(n, c, env)
	case ctaStringFunction:
		return ctaStringFunctionItem(n, c, env)
	case ctaConcat:
		return ctaConcatItem(n, c, env)
	case ctaCurrentDate:
		return ctaCurrentDateItem(n, c, env)
	case ctaNamespaceURI:
		return ctaNamespaceURIItem(n, c, env)
	case ctaDistinctValues:
		return ctaDistinctValuesItem(n, c, env)
	case ctaAttributePath:
		return ctaAttributePathItem(n, c, env)
	case ctaSum:
		return ctaSumItem(n, c, env)
	case ctaSumInstance:
		return ctaSumInstanceItem(n, c, env)
	case ctaValueVar:
		return ctaValueItem(n, c, env)
	case ctaEmptyValue:
		return ctaAtoms{}
	case ctaUntypedValue:
		return ctaUntypedValueItem(c, env)
	case ctaContextAtom:
		return ctaContextAtomItem(c, env)
	case ctaIntegerRanges:
		return ctaIntegerRangesItem(n, c, env)
	case ctaStringSequence:
		return ctaStringSequenceItem(n, c, env)
	default:
		return ctaAtoms{}
	}
}

// ctaContextAtomItem casts E's string value, the [ValueBinding]'s text, into c,
// which §3.5.2's casting rules — and §3.5.1 step 4's, and §3.4's for an
// arithmetic operand — do to an xs:untypedAtomic operand, on ctaAttrItem's
// terms: one item, whatever `$value` is bound to.
//
// The input is ctaTypedInput by construction (ctaInput); the other arm binds
// nothing and is unreachable, and raises.
func ctaContextAtomItem(c *xsd.SimpleType, env ctaEnv) ctaItem {
	in, typed := env.input.(ctaTypedInput)
	if !typed {
		return ctaRaised{}
	}
	return ctaValidate(in.value.text, c, env)
}

// ctaUntypedValueItem casts `$value`'s [Untyped] binding into c, which
// §3.5.2's casting rules — and §3.5.1 step 4's — do to an xs:untypedAtomic
// operand, on ctaAttrItem's terms. A nil `$value` binding is the empty
// sequence (cvc-assertion clause 2.3.2).
//
// The input is ctaTypedInput by construction (ctaInput); the other arm binds
// nothing and is unreachable. A [Typed] binding breaks the obligation
// [BindValue] states and is ctaRaised, unreachable for a caller that keeps it.
func ctaUntypedValueItem(c *xsd.SimpleType, env ctaEnv) ctaItem {
	lexical, bound, ok := ctaUntypedBinding(env)
	if !ok {
		return ctaRaised{}
	}
	if !bound {
		return ctaAtoms{}
	}
	return ctaValidate(lexical, c, env)
}

// ctaUntypedBoolean is fn:boolean over `$value`'s [Untyped] binding: xpath20.md
// §2.4.3 rule 4 makes an xs:untypedAtomic value false iff it has zero length,
// and rule 1 makes the empty sequence — a nil `$value` binding — false. A
// [Typed] binding is ctaError, on ctaUntypedValueItem's terms.
func ctaUntypedBoolean(env ctaEnv) ctaAnswer {
	lexical, bound, ok := ctaUntypedBinding(env)
	if !ok {
		return ctaError
	}
	return ctaAnswerOf(bound && lexical != "")
}

// ctaUntypedBinding reads `$value`'s binding as an [Untyped] value: its
// lexical, with bound false for a nil `$value` binding, and ok false for a
// binding of the other arm, which breaks the obligation [BindValue] states.
func ctaUntypedBinding(env ctaEnv) (lexical string, bound, ok bool) {
	in, typed := env.input.(ctaTypedInput)
	if !typed || in.value.v == nil {
		return "", false, true
	}
	untyped, isUntyped := in.value.v.(tvUntyped)
	if !isUntyped {
		return "", false, false
	}
	return untyped.lexical, true, true
}

// ctaValueItem converts `$value`'s binding into c on ctaTypedAttrItem's terms
// (ctaPromote). A nil `$value` binding is the empty sequence (cvc-assertion
// clause 2.3.2). A listed n ranges the bound value's [value.Listed] items in
// order, each of type n.atom — the flattened sequence Datatypes dt-xdmrep makes
// a list value's XDM representation, so an empty list is the empty sequence and
// a list of two or more items is a sequence of that length.
//
// The input is ctaTypedInput by construction (ctaInput); the other arm binds
// nothing and is unreachable. A binding that is not [Typed], and a listed one
// whose value does not carry [value.Listed], break the obligation [BindValue]
// states and are ctaRaised, unreachable for a caller that keeps it.
func ctaValueItem(n ctaValueVar, c *xsd.SimpleType, env ctaEnv) ctaItem {
	in, typed := env.input.(ctaTypedInput)
	if !typed || in.value.v == nil {
		return ctaAtoms{}
	}
	bound, isTyped := in.value.v.(tvTyped)
	if !isTyped {
		return ctaRaised{}
	}
	if !n.listed {
		return ctaPromote(bound.v, n.atom, c, env)
	}
	list, isList := bound.v.(value.Listed)
	if !isList {
		return ctaRaised{}
	}
	var vs []value.Value
	for item := range list.Items() {
		converted, ok := ctaValidated(ctaPromote(item, n.atom, c, env))
		if !ok {
			return ctaRaised{}
		}
		vs = append(vs, converted)
	}
	return ctaAtoms{vs: vs}
}

// ctaMatchedAttributes is the [[normalized value]]s of the attributes n's
// NameTest selects, in the DOCUMENT ORDER [Attributes] yields them in — which
// is the order of the operand sequence itself and so of every answer decided
// over it (STYLE D1).
//
// The whole sequence is walked: a QName NameTest matches at most one attribute
// because no element carries two of one ·expanded name·, and a [37] Wildcard
// arm has no such bound.
//
// Over a ctaTypedInput — an assertion's attribute whose type is ·special· — the
// matched values are the [Untyped] arms' [schema normalized value]s, and ok is
// false where a matched value is not [Untyped], which breaks the obligation
// [TypedAttributes] states and which every reader raises on (ctaInput). The
// default arm is unreachable: ctaInput is sealed over the two arms named.
func ctaMatchedAttributes(n ctaAttr, env ctaEnv) (matched []string, ok bool) {
	switch in := env.input.(type) {
	case ctaLexicalInput:
		in.attrs(func(name xsd.QName, lexical string) bool {
			if n.test.matches(name) {
				matched = append(matched, lexical)
			}
			return true
		})
		return matched, true
	case ctaTypedInput:
		ok = true
		in.attrs(func(name xsd.QName, v TypedValue) bool {
			if !n.test.matches(name) {
				return true
			}
			untyped, isUntyped := v.(tvUntyped)
			if !isUntyped {
				ok = false
				return false
			}
			matched = append(matched, untyped.lexical)
			return true
		})
		return matched, ok
	default:
		return nil, true
	}
}

// ctaMatchedTyped is ctaMatchedAttributes for a typed attribute: the typed
// values [TypedAttributes] yields under n's ·expanded name·, at most one, each
// of type n.st by the caller's obligation that type states. ok is false where
// a matched value is not [Typed], which breaks that obligation and which every
// reader raises on.
//
// The input is ctaTypedInput by construction (ctaInput); the other arm matches
// nothing and is unreachable.
func ctaMatchedTyped(n ctaTypedAttr, env ctaEnv) (matched []value.Value, ok bool) {
	in, typed := env.input.(ctaTypedInput)
	if !typed {
		return nil, true
	}
	ok = true
	in.attrs(func(name xsd.QName, v TypedValue) bool {
		if name != n.name {
			return true
		}
		tv, isTyped := v.(tvTyped)
		if !isTyped {
			ok = false
			return false
		}
		matched = append(matched, tv.v)
		return true
	})
	return matched, ok
}

// ctaTypedAttrItem converts the matched typed value into c on ctaPromote's
// terms, which is B.1's promotion or §3.5.2's conversion into the comparison
// type — never a re-validation of the attribute's lexical, which has none here.
func ctaTypedAttrItem(n ctaTypedAttr, c *xsd.SimpleType, env ctaEnv) ctaItem {
	matched, ok := ctaMatchedTyped(n, env)
	if !ok {
		return ctaRaised{}
	}
	return ctaPromoted(matched, n.st, c, env)
}

// ctaAttributePathItem atomizes the attribute nodes n selects (xpath20.md
// §2.4.2) to the typed values the evaluation's [Tally] collected for n, in the
// order they were reported, which is document order ([Tally.Attribute]), and
// converts them into c: cast as ctaAttrItem casts an untyped attribute's
// where n's type is ·special·, and on ctaTypedAttrItem's terms otherwise. A
// value of the other arm than n's type fixes, or nil, breaks the obligation
// [Tally.Attribute] states and raises, as does a Tally holding no collector
// for n, which [AssertionTest.Evaluate] refuses before the tree is read. The
// input is ctaTypedInput by construction (ctaInput); the other arm carries no
// Tally and raises.
func ctaAttributePathItem(n ctaAttributePath, c *xsd.SimpleType, env ctaEnv) ctaItem {
	in, typed := env.input.(ctaTypedInput)
	if !typed {
		return ctaRaised{}
	}
	collected, held := in.counts.values(n)
	if !held {
		return ctaRaised{}
	}
	if n.st.IsSpecial() {
		lexicals := make([]string, 0, len(collected))
		for _, v := range collected {
			untyped, isUntyped := v.(tvUntyped)
			if !isUntyped {
				return ctaRaised{}
			}
			lexicals = append(lexicals, untyped.lexical)
		}
		return ctaUntypedItems(lexicals, c, env)
	}
	vs := make([]value.Value, 0, len(collected))
	for _, v := range collected {
		tv, isTyped := v.(tvTyped)
		if !isTyped {
			return ctaRaised{}
		}
		vs = append(vs, tv.v)
	}
	return ctaPromoted(vs, n.st, c, env)
}

// ctaEachChild hands each of E's element [[children]] named name, in the
// DOCUMENT ORDER [ChildElements] yields them in, to each as its typed value:
// one value, or none for a ·nilled· child, which is a node with no value. It
// reports false, and stops, where each does, and where a child's value is
// neither [Typed] nor nil, which breaks the obligation [ChildElements] states
// and which every reader raises on.
//
// The input is ctaTypedInput by construction (ctaInput); the other arm carries
// no children, so each is never called, and is unreachable.
func ctaEachChild(name xsd.QName, env ctaEnv, each func(vs []value.Value) bool) bool {
	in, typed := env.input.(ctaTypedInput)
	if !typed {
		return true
	}
	ok := true
	in.children(func(c ChildElement) bool {
		if c.name != name {
			return true
		}
		if c.v == nil {
			ok = each(nil)
			return ok
		}
		tv, isTyped := c.v.(tvTyped)
		if !isTyped {
			ok = false
			return false
		}
		ok = each([]value.Value{tv.v})
		return ok
	})
	return ok
}

// ctaMatchedChildren is the typed values of E's element [[children]] n's
// NameTest selects (ctaEachChild), each of type n.st by the caller's
// obligation that type states; and nodes, how many children it selects — a
// ·nilled· one included. ok is false where ctaEachChild reports false.
func ctaMatchedChildren(n ctaTypedChild, env ctaEnv) (vs []value.Value, nodes int, ok bool) {
	ok = ctaEachChild(n.name, env, func(child []value.Value) bool {
		nodes++
		vs = append(vs, child...)
		return true
	})
	return vs, nodes, ok
}

// ctaTypedChildItem atomizes the selected children (xpath20.md §2.4.2) and
// converts the atoms into c on ctaTypedAttrItem's terms: each child's typed
// value, in document order, a ·nilled· child contributing none.
func ctaTypedChildItem(n ctaTypedChild, c *xsd.SimpleType, env ctaEnv) ctaItem {
	matched, _, ok := ctaMatchedChildren(n, env)
	if !ok {
		return ctaRaised{}
	}
	return ctaPromoted(matched, n.st, c, env)
}

// ctaEachUntypedChild is ctaEachChild for a child of mixed content
// (ctaUntypedChild): it hands each of E's element [[children]] named name, in
// the DOCUMENT ORDER [ChildElements] yields them in, to each as the lexical of
// its string-value: one, or none for a ·nilled· child. It reports false, and
// stops, where each does, and where a child's value is neither [Untyped] nor
// nil — a [Typed] one breaks the obligation [ChildElements] states, on
// ctaMatchedAttributes' terms for a typed input, and every reader raises on it.
//
// The input is ctaTypedInput by construction (ctaInput); the other arm carries
// no children, so each is never called, and is unreachable.
func ctaEachUntypedChild(name xsd.QName, env ctaEnv, each func(lexicals []string) bool) bool {
	in, typed := env.input.(ctaTypedInput)
	if !typed {
		return true
	}
	ok := true
	in.children(func(c ChildElement) bool {
		if c.name != name {
			return true
		}
		if c.v == nil {
			ok = each(nil)
			return ok
		}
		untyped, isUntyped := c.v.(tvUntyped)
		if !isUntyped {
			ok = false
			return false
		}
		ok = each([]string{untyped.lexical})
		return ok
	})
	return ok
}

// ctaMatchedUntypedChildren is the string-values of E's element [[children]]
// n's NameTest selects (ctaEachUntypedChild), and nodes, how many children it
// selects — a ·nilled· one included. ok is false where ctaEachUntypedChild
// reports false.
func ctaMatchedUntypedChildren(n ctaUntypedChild, env ctaEnv) (lexicals []string, nodes int, ok bool) {
	ok = ctaEachUntypedChild(n.name, env, func(child []string) bool {
		nodes++
		lexicals = append(lexicals, child...)
		return true
	})
	return lexicals, nodes, ok
}

// ctaUntypedChildItem atomizes the selected children (xpath20.md §2.4.2), each
// to its string-value as xs:untypedAtomic in document order, a ·nilled· child
// contributing none, and casts the atoms into c on ctaAttrItem's terms.
func ctaUntypedChildItem(n ctaUntypedChild, c *xsd.SimpleType, env ctaEnv) ctaItem {
	matched, _, ok := ctaMatchedUntypedChildren(n, env)
	if !ok {
		return ctaRaised{}
	}
	return ctaUntypedItems(matched, c, env)
}

// ctaPromoted converts each of vs, values of type from, into c on ctaPromote's
// terms, in order, raising for the whole sequence where one does not convert.
func ctaPromoted(vs []value.Value, from, c *xsd.SimpleType, env ctaEnv) ctaItem {
	converted := make([]value.Value, 0, len(vs))
	for _, v := range vs {
		cv, ok := ctaValidated(ctaPromote(v, from, c, env))
		if !ok {
			return ctaRaised{}
		}
		converted = append(converted, cv)
	}
	return ctaAtoms{vs: converted}
}

// ctaCountItem is the xs:integer fn:count returns for n, converted into c on
// ctaTypedAttrItem's terms: how many items n's argument evaluates to
// (ctaCounted.nodes) — the counter the evaluation's [Tally] holds for its
// path, the children its value predicate is true for, or the items of an
// operand that is no path (ctaCountedItems) — through the lexical
// of that integer, which is a datatype validation as every value this package
// builds is. An argument whose predicate raised over a candidate raises.
//
// A rooted argument raises err:XPDY0050 before fn:count is applied
// (ctaNoDocumentRoot). A Tally with no counter for the path is a caller breach
// [AssertionTest.Evaluate] answers false before the tree is read, so the
// ctaRaised it is here is unreachable through that entry point. The input is
// ctaTypedInput by construction (ctaInput); the other arm counts nothing and is
// unreachable too.
func ctaCountItem(n ctaCount, c *xsd.SimpleType, env ctaEnv) ctaItem {
	count, ok := n.arg.nodes(env)
	if !ok {
		return ctaRaised{}
	}
	v, validated := ctaValidated(ctaValidate(strconv.Itoa(count), n.st, env))
	if !validated {
		return ctaRaised{}
	}
	return ctaPromote(v, n.st, c, env)
}

// ctaAttrItem casts the matched attributes into c, which §3.5.2's casting rules
// do to an xs:untypedAtomic operand.
//
// The conversion is EAGER over the whole sequence: one item that does not cast
// raises for the OPERAND, on §3.5.2's own latitude ("may raise a dynamic error
// as soon as it encounters an error in evaluating either operand"), rather than
// dropping out of it. So `@* = 3` over an element carrying n="3" and s="abc"
// raises and is false, where a per-pair conversion would answer true.
func ctaAttrItem(n ctaAttr, c *xsd.SimpleType, env ctaEnv) ctaItem {
	matched, ok := ctaMatchedAttributes(n, env)
	if !ok {
		return ctaRaised{}
	}
	return ctaUntypedItems(matched, c, env)
}

// ctaUntypedItems casts each of lexicals, the lexicals of a sequence of
// xs:untypedAtomic values in order, into c on ctaAttrItem's eager terms.
func ctaUntypedItems(lexicals []string, c *xsd.SimpleType, env ctaEnv) ctaItem {
	vs := make([]value.Value, 0, len(lexicals))
	for _, lexical := range lexicals {
		v, validated := ctaValidated(ctaValidate(lexical, c, env))
		if !validated {
			return ctaRaised{}
		}
		vs = append(vs, v)
	}
	return ctaAtoms{vs: vs}
}

// ctaCastItem evaluates one cast and converts its result into c.
//
// The empty-sequence rules are xpath20.md §3.10.2's rule 3, both halves: with
// the `?` occurrence indicator the cast of an empty operand IS the empty
// sequence, and without it the cast raises err:XPTY0004. That difference is
// the whole reason the `?` is carried on the node.
//
// TWO OR MORE items raise err:XPTY0004 whether or not the `?` is written: rule
// 3 admits "exactly one atomic value" and `?` widens that to AT MOST one, never
// to more, so a cast over a wildcard NameTest matching several attributes is a
// type error and not the first of them.
func ctaCastItem(n ctaCast, c *xsd.SimpleType, env ctaEnv) ctaItem {
	inner, converted := ctaItemOf(n.operand, n.target, env).(ctaAtoms)
	if !converted {
		return ctaRaised{}
	}
	if len(inner.vs) == 0 {
		if n.allowsEmpty {
			return ctaAtoms{}
		}
		return ctaRaised{}
	}
	if len(inner.vs) > 1 {
		return ctaRaised{}
	}
	return ctaPromote(inner.vs[0], n.target, c, env)
}

// ctaCastableItem evaluates `E castable as T` (xpath20.md §3.10.3) and
// converts the xs:boolean it returns into c on ctaMatchItem's terms. E is
// evaluated first, uncast (ctaSequenceLength), and an error there raises: "If
// evaluation of E fails with a dynamic error, the castable expression as a
// whole fails" — so `. castable as xs:date` in an assertions facet raises the
// err:XPDY0002 its ctaNoContextItem does. Otherwise the result is whether n's
// cast yields a value rather than raising (ctaCastItem): the empty sequence
// with `?` is true, without it err:XPTY0004 and false, as are two or more
// items and a lexical or facet mismatch, err:FORG0001.
func ctaCastableItem(n ctaCastable, c *xsd.SimpleType, env ctaEnv) ctaItem {
	if _, ok := ctaSequenceLength(n.cast.operand, env); !ok {
		return ctaRaised{}
	}
	_, raised := ctaCastItem(n.cast, n.cast.target, env).(ctaRaised)
	return ctaConvert(strconv.FormatBool(!raised), n.st, c, env)
}

// ctaInstanceOfItem evaluates `E instance of T` (xpath20.md §3.10.1) and
// converts the xs:boolean it returns into c on ctaMatchItem's terms. E's
// atomized items are read in n.read, never cast to T, and an error producing
// them raises, as evaluating any operand does — so `data(.) instance of
// xs:untypedAtomic` in an assertions facet raises the err:XPDY0002 its
// ctaNoContextItem does. Otherwise the result is §2.5.4.1's: the item count
// is one n.occurrence admits, and every item matches T, which n.matches
// settled at compile time — vacuously so for the empty sequence.
func ctaInstanceOfItem(n ctaInstanceOf, c *xsd.SimpleType, env ctaEnv) ctaItem {
	atoms, ok := ctaItemOf(n.operand, n.read, env).(ctaAtoms)
	if !ok {
		return ctaRaised{}
	}
	matched := n.occurrence.admits(len(atoms.vs)) && (len(atoms.vs) == 0 || n.matches)
	return ctaConvert(strconv.FormatBool(matched), n.st, c, env)
}

// ctaConvert casts lexical, a value of type from, into type to.
func ctaConvert(lexical string, from, to *xsd.SimpleType, env ctaEnv) ctaItem {
	v, validated := ctaValidated(ctaValidate(lexical, from, env))
	if !validated {
		return ctaRaised{}
	}
	return ctaPromote(v, from, to, env)
}

// ctaPromote converts one atomic value of type from into type to, which is
// what §3.5.2 clause 2's cast and B.1's type promotions ask of an operand
// whose own type is not the type the comparison runs in, and what F&O §17.2
// case 4 and §17.3 ask of a cast to from itself or to an ancestor of it.
//
// Where v is already a value of to (ctaRepresents) — to is from, or an
// ancestor of from whose values come from the mapping v came from — the result
// is v itself. That is subtype substitution and §17.3's cast alike, "The result
// will have the same value as the original", and it renders nothing, so it
// raises nothing where §17.3 says the cast always succeeds: not over the zero
// of a user restriction of xs:yearMonthDuration, which renders only through
// xs:duration's mapping, as a lexical xs:yearMonthDuration rejects, and not
// over a zero of huge ·scale· under a user restriction of xs:precisionDecimal.
//
// Every other conversion goes through the value's ·canonical representation·,
// which is the one lexical the spec guarantees maps back to that same value
// (Datatypes §2.3.1), so a conversion is one more datatype validation and
// never a backend-specific value translation this package would have to know
// the representations for. ctaCanonical renders it. A value it cannot render
// cannot be converted, which is ctaRaised on either of two terms:
//
//   - no canonical mapping on from's chain, which is err:XPTY0004 — the
//     "casting is not supported" arm of xpath-functions.md §17. That is
//     unreachable for the target set [CompileCTATest] admits: the canonical
//     mapping is absent only for xs:QName and xs:NOTATION (value.Mapping),
//     and a cast whose target has either as its primitive is declined at
//     compile time.
//   - a canonical form beyond the backend's capacity (xmlschema11-2 §5.4,
//     the strict backend's precisionDecimal zero of a huge ·scale·), a
//     dynamic error raised as an implementation limit — indicated, never
//     rendered as a padded or substitute lexical. fn:string over a cast to
//     xs:precisionDecimal reaches it. A cast from a typed precisionDecimal
//     operand does not: castsFrom admits one only to its own type or an
//     ancestor of it, which ctaRepresents answers without rendering.
func ctaPromote(v value.Value, from, to *xsd.SimpleType, env ctaEnv) ctaItem {
	if ctaRepresents(from, to, env) {
		return ctaSingleton(v)
	}
	lexical, rendered := ctaCanonical(v, from, env)
	if !rendered {
		return ctaRaised{}
	}
	return ctaValidate(lexical, to, env)
}

// ctaRepresents reports whether a value of type from is already a value of
// type to: to is from itself, or an ancestor of from on its {base type
// definition} chain with no type from from up to but excluding to carrying a
// [value.Mapping] of its own in env.backend. A value's representation is its
// nearest mapped ancestor's (value.Backend's nearest-mapped-ancestor rule,
// which value.ValidateLexical applies), so a mapped type between the two —
// xs:int under a backend that maps it — means v is in a representation to's
// values are not, and the conversion renders it. An unwalkable chain reports
// false, which renders too.
//
// TERMINATION: the walk carries no visited set, on ctaTypes.ancestor's terms.
func ctaRepresents(from, to *xsd.SimpleType, env ctaEnv) bool {
	for at := from; at != nil; {
		if at.Name() == to.Name() {
			return true
		}
		if _, mapped := env.backend.Mapping(at.Name()); mapped {
			return false
		}
		base, err := at.Base(env.types)
		if err != nil {
			return false
		}
		at = base
	}
	return false
}

// ctaCanonical renders v, a value of type from, through the backend's
// [value.Mapping.Canonical] — the (string, error) channel, because
// [value.Canonical]'s string-only one has no outcome for a form beyond the
// backend's capacity, and a value whose type can reach one does not carry it.
//
// The mapping is the nearest one on from's {base type definition} chain, from
// itself included, which is the mapping that produced v (value.Backend's
// nearest-mapped-ancestor rule, which value.ValidateLexical applies).
// [value.Mapping] is keyed by name, so an anonymous or user-derived from
// reaches its builtin ancestor's. A mapping whose Canonical is nil, or errors
// on v, does not end the walk: its error is per-value (value.Mapping), and a
// partial-domain one says only that v has no form in THAT type's lexical
// space, so the next mapped ancestor's wider canonical mapping is asked —
// xs:yearMonthDuration's P0M has none of its own (§3.4.26.1 Note) and renders
// through xs:duration's as PT0S. Reporting false when no mapping on the chain
// renders v, or the chain cannot be walked, is the caller's ctaRaised.
func ctaCanonical(v value.Value, from *xsd.SimpleType, env ctaEnv) (string, bool) {
	for at := from; at != nil; {
		m, mapped := env.backend.Mapping(at.Name())
		if mapped && m.Canonical != nil {
			lexical, err := m.Canonical(v)
			if err == nil {
				return lexical, true
			}
		}
		base, err := at.Base(env.types)
		if err != nil {
			return "", false
		}
		at = base
	}
	return "", false
}

// ctaValidate casts lexical into st, which xpath-functions.md §17.1.1 makes
// one datatype validation: "The semantics of casting are identical to XML
// Schema validation", whiteSpace normalization included. [value.ValidateLexical]
// is that validation, so this package holds no lexical parser of its own
// (STYLE T4).
//
// The [value.Context] is nil, and there is nothing for it to carry: only the
// QName and NOTATION mappings are context-dependent (PRINCIPLES 19), and a
// cast whose target has either as its primitive is declined at compile time.
//
// A failure is ctaRaised either way, and the branch is what says WHICH error
// it is (STYLE E2). A [value.IsDatatypeVerdict] error is a verdict about the
// lexical — err:FORG0001, "it is not possible to cast the input value into
// the value space of the target type". Anything else but a declined facet
// assertion ([value.IsAssertionDeclined], the gap below) is a fault of the type
// or of the backend and says nothing about the lexical, so it is not that
// verdict; it is the one xpath-functions.md §17 gives an ST/TT pair this
// processor cannot cast between at all, err:XPTY0004. key-cta-ta-select
// clause 2 makes the {test} false for both.
//
// An assertions facet of st is decided by the evaluator env.input supplies
// (ctaInput.facets). An assertion's is [FacetAssertions] at the evaluation's
// own current dateTime, ctaTypedInput.now, so a cast inside an assertion and
// the assertion itself read one instant (cvc-xpath clause 6). A Type
// Alternative's input holds no instant, and its evaluator,
// ctaAssertionsDeclined, decides no {test} rather than read one at an instant
// it would have to make up.
//
// GAP(xpath): a declined facet assertion is not an XPath error, yet it maps to
// ctaRaised like one, so the {test} reads false where key-cta-ta-select
// clause 2 makes only a dynamic or type error false: that false is this
// repo's choice and not the spec's, and withholding instead needs a three-way
// [CTATest.Evaluate] (#2533). Its readers, validate's
// walk.conditionallySelected and conformance's subtree-root evaluation, then
// try the next alternative or the {default type definition}, so the direction
// at the instance is unestablished. It is unreached today: a Type
// Alternative's casts target builtins alone (ctaTypes.castTarget), and no
// builtin has an assertions facet.
func ctaValidate(lexical string, st *xsd.SimpleType, env ctaEnv) ctaItem {
	v, err := value.ValidateLexical(env.backend, env.types, st, lexical, nil, env.input.facets())
	if err == nil {
		return ctaSingleton(v)
	}
	if value.IsDatatypeVerdict(err) {
		return ctaRaised{} // err:FORG0001
	}
	if value.IsAssertionDeclined(err) {
		return ctaRaised{} // declined, not an error: the gap above
	}
	return ctaRaised{} // err:XPTY0004
}

// holdsSign reports whether op holds for two operands whose collation
// comparison yielded sign, which is how xpath20.md B.2 defines every one of
// the six operators over xs:string: through fn:compare against 0.
func (op ctaComparator) holdsSign(sign int) bool {
	switch op {
	case ctaEqual:
		return sign == 0
	case ctaNotEqual:
		return sign != 0
	case ctaLess:
		return sign < 0
	case ctaLessEqual:
		return sign <= 0
	case ctaGreater:
		return sign > 0
	case ctaGreaterEqual:
		return sign >= 0
	}
	return false
}

// holdsCollated decides op between two xs:string values under the DEFAULT
// COLLATION, which xpath-valid clause 2.2.10 fixes as the Unicode codepoint
// collation — a static-context property no backend owns, which is why this
// one comparison is decided here rather than through a value capability.
// xpath20.md B.2 routes all six operators on xs:string (and, through B.1's
// promotion, on xs:anyURI) through fn:compare, so one sign decides them all.
//
// The compared string is the value's ·canonical representation·, which is the
// identity for xs:string and xs:anyURI (f-stringCanmap, f-anyURICanmap) and so
// is the string the cast produced — whiteSpace-collapsed where the operand's
// own type collapses, as `cast as xs:token` does. It is read through
// [value.Canonical], not ctaCanonical's mapping walk: an identity canonical
// form has no beyond-capacity case, and the string-only capability needs no
// type to find a mapping by. A value carrying none is err:XPTY0004, the
// "casting is not supported" arm ctaPromote also raises on.
func (op ctaComparator) holdsCollated(l, r value.Value) ctaAnswer {
	ls, lRenders := l.(value.Canonical)
	rs, rRenders := r.(value.Canonical)
	if !lRenders || !rRenders {
		return ctaError
	}
	return ctaAnswerOf(op.holdsSign(strings.Compare(ls.Canonical(), rs.Canonical())))
}

// holdsBetween reports whether op holds between two values of one comparison
// type, through the capability interfaces the values themselves carry (STYLE
// T2) and never a type switch over a backend's concrete types.
//
// A value carrying neither capability is ctaError, and that is a fault of the
// BACKEND rather than a judgment about the operator: which operators an
// operand type admits is xpath20.md B.2's answer, settled when the node was
// built (ctaTypes.admitsComparison), so every pair reaching here is one B.2
// holds a row for.
func (op ctaComparator) holdsBetween(l, r value.Value) ctaAnswer {
	if op == ctaEqual || op == ctaNotEqual {
		eq, comparable := l.(value.Eq)
		if !comparable {
			return ctaError
		}
		return ctaAnswerOf(eq.Eq(r) == (op == ctaEqual))
	}
	ord, ordered := l.(value.Ordered)
	if !ordered {
		return ctaError
	}
	return ctaAnswerOf(op.holdsOrdering(ord.Cmp(r)))
}

// ctaImplicitTimezone is the implicit timezone of every evaluation's dynamic
// context (xpath20.md dt-timezone), spelled as the timezoneFrag it appends to
// an untimezoned lexical: Z, the zero offset PT0S. Structures §3.13.4.2
// cvc-xpath clause 7 makes it ·implementation-defined· — the
// implementation-defined list's item 12 for XPath evaluation generally — and
// constant during an ·assessment· episode; a package constant is that.
const ctaImplicitTimezone = "Z"

// holdsAtImplicitTimezone decides op between two values of c, a date/time
// comparison type (ctaDateTimeFamily), as F&O §10.4 defines its comparison
// functions: "If either operand to a comparison function on date or time
// values does not have an (explicit) timezone then, for the purpose of the
// operation, an implicit timezone, provided by the dynamic context ..., is
// assumed to be present as part of the value." Each operand without one is
// given ctaImplicitTimezone (ctaAtImplicitTimezone), and the two timezoned
// values are then decided by holdsBetween, which is total over them.
//
// The value space's own partial order is left as it is, because the facets
// read it: a mixed pair is never equal there and is [value.Incomparable]
// within fourteen hours, which is right for minInclusive and wrong for XPath,
// so the substitution happens here and per operand, and never by reading the
// pair's Incomparable.
//
// Which operators reach here is B.2's answer, settled at compile time: the g*
// types have eq and ne alone, so an ordering over them is the err:XPTY0004
// node and never this decision.
func (op ctaComparator) holdsAtImplicitTimezone(l, r value.Value, c *xsd.SimpleType, env ctaEnv) ctaAnswer {
	left, lPlaced := ctaAtImplicitTimezone(l, c, env)
	if !lPlaced {
		return ctaError
	}
	right, rPlaced := ctaAtImplicitTimezone(r, c, env)
	if !rPlaced {
		return ctaError
	}
	return op.holdsBetween(left, right)
}

// ctaAtImplicitTimezone is v, a value of the date/time primitive c, with
// ctaImplicitTimezone in place of a missing timezone: v itself where it has
// one, and otherwise the value of its ·canonical representation· with
// ctaImplicitTimezone appended, validated against c — the round-trip
// ctaPromote converts through, so this package translates no backend value.
// It reports false, the caller's ctaError, for a value that is not
// [value.TimezoneAware], one whose canonical form does not render, and one
// whose timezoned lexical c does not validate: each is a fault of the backend,
// since every untimezoned lexical of the family takes a timezoneFrag.
func ctaAtImplicitTimezone(v value.Value, c *xsd.SimpleType, env ctaEnv) (value.Value, bool) {
	tz, aware := v.(value.TimezoneAware)
	if !aware {
		return nil, false
	}
	if tz.HasTimezone() {
		return v, true
	}
	lexical, rendered := ctaCanonical(v, c, env)
	if !rendered {
		return nil, false
	}
	return ctaValidated(ctaValidate(lexical+ctaImplicitTimezone, c, env))
}

// holdsBoolean decides op between two xs:boolean values, which B.2 gives all
// six rows: eq and ne through op:boolean-equal, and the four ordering
// operators through op:boolean-less-than and op:boolean-greater-than.
//
// Those two functions are defined over the VALUES and not over an order —
// "Returns true if $arg1 is false and $arg2 is true. Otherwise, returns false"
// (xpath-functions.md §9.2.2) — so the ordering is decided through
// [value.Eq] against the value of the lexical "true", exactly as fn:boolean
// reads an xs:boolean operand (ctaTruth). It is deliberately NOT decided
// through [value.Ordered]: that capability is the datatype's own order, which
// xs:boolean does not have ("The XML Schema datatype xs:boolean is not
// ordered", xpath-functions.md §9.2), and B.2's rows are about which operators
// an operand type admits rather than about the value space.
func (op ctaComparator) holdsBoolean(l, r value.Value, st *xsd.SimpleType, env ctaEnv) ctaAnswer {
	if op == ctaEqual || op == ctaNotEqual {
		return op.holdsBetween(l, r)
	}
	left := ctaTruth(l, st, env)
	right := ctaTruth(r, st, env)
	if left == ctaError || right == ctaError {
		return ctaError
	}
	return ctaAnswerOf(op.holdsOrdering(ctaBooleanOrdering(left == ctaTrue, right == ctaTrue)))
}

// ctaTruth reads one xs:boolean value of type st as an answer, which is
// equality against the value of the lexical "true" — fn:boolean's rule 3 for
// an xs:boolean operand (xpath20.md §2.4.3), and the reading
// op:boolean-less-than and op:boolean-greater-than are written in terms of.
func ctaTruth(v value.Value, st *xsd.SimpleType, env ctaEnv) ctaAnswer {
	eq, comparable := v.(value.Eq)
	if !comparable {
		return ctaError
	}
	return ctaEqualsLexical(eq, st, "true", env)
}

// ctaBooleanOrdering orders two xs:boolean values as op:boolean-less-than and
// op:boolean-greater-than order them: "false is less than true" and nothing
// else is related, which over two values is a total order and so never
// [value.Incomparable].
func ctaBooleanOrdering(l, r bool) value.Ordering {
	if l == r {
		return value.Equal
	}
	if !l {
		return value.Less
	}
	return value.Greater
}

// holdsOrdering reports whether op holds for two values that compared as o.
// [value.Incomparable] is false for all four ordering operators: no magnitude
// relationship holds between values no order relates, which is §3.5.2's
// "otherwise the result of the comparison is false".
func (op ctaComparator) holdsOrdering(o value.Ordering) bool {
	switch o {
	case value.Less:
		return op == ctaLess || op == ctaLessEqual
	case value.Equal:
		return op == ctaLessEqual || op == ctaGreaterEqual
	case value.Greater:
		return op == ctaGreater || op == ctaGreaterEqual
	case value.Incomparable:
		return false
	}
	return false
}
