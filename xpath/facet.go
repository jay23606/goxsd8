package xpath

import (
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// This file is the FACET façade over the grammar compileCTATest parses (STYLE
// T4): an assertions facet's {test} (Datatypes §4.3.13, an Assertion's {test})
// written in the assertion façade's grammar is compiled and evaluated against
// one value, which Assertions Valid (§4.3.13.3, cvc-assertions-valid) asks of
// it. It differs from the assertion façade in the one way the rule's
// conditions differ from cvc-assertion's: there is no context item (clause
// 1.2), so every expression that reads one — [47] ContextItemExpr `.`, the
// node fn:namespace-uri and fn:in-scope-prefixes take among them
// (ctaFacetFacade.contextNode), an attribute or child-axis step, a "/" or "//"
// opening a path, and the implicit argument of fn:string, fn:string-length,
// fn:normalize-space and fn:namespace-uri called with none — raises
// err:XPDY0002 (ctaNoContextItem; ctaAbsentNode for the node), and no context
// position or size (clause 1.3), so fn:position and fn:last raise it too
// (ctaNoFocus); and `$value`, bound to the XDM representation of the value
// under the facet's type, or under its ·active basic member· where that type
// is a union (clause 1.4, dt-xdmrep clause 4), is the whole of what a {test}
// can read, arithmetic and the F&O string and sequence functions over it
// included (ctaFacetFacade.computes, ctaFacetFacade.callsLibrary), `castable
// as` and `instance of` over it (ctaFacetFacade.castable,
// ctaFacetFacade.instanceOf), and fn:count over it or over any other operand
// that is no path. An fn:count call over a path declines
// (ctaFacetFacade.count), and so does a quantified expression, over a child
// step (ctaFacetFacade.quantified, and ctaParser.childQuantified for one
// windowed or reading its bound children, whose binding step compiles to no
// typed child) or over `$value` (ctaFacetFacade.rangeScope).

// FacetAssertions is the [value.AssertionEvaluator] for an assertions facet's
// {test}s: it compiles the {test} under the facet's static context and
// evaluates it with no context item, `$value` bound to v ([value.AssertionEvaluator]
// states the contract). Its consumer is validate, which passes it at every
// value.ValidateLexical, value.ValidatingType and value.ConstraintMatches call,
// so a value's Datatype Valid verdict — and a union's ·validating type· and a
// fixed-value comparison with it — includes its assertions facets.
//
// Each {test} answers one of three outcomes:
//
//   - [value.AssertionHolds], where it evaluates to true;
//   - [value.AssertionFails], where it evaluates to false or raises a dynamic or
//     type error, which cvc-assertions-valid treats alike — err:XPDY0002 for a
//     read of the absent context item among them, arithmetic, `castable as`
//     and fn:data under `instance of` over one and fn:string, fn:string-length
//     and fn:normalize-space with no argument included, and of the absent
//     focus by fn:position and fn:last, and the err:FOAR0001 and
//     err:FOAR0002 of arithmetic ([AssertionTest.Evaluate] lists them);
//   - [value.AssertionDeclined], where this engine does not evaluate it: a
//     {test} [CompileAssertionTest] would decline over a simple {content type}
//     of the same type, on that function's terms — the grammar is the same and
//     so is every decline it states — a {test} calling fn:count over a
//     path, `count(@a)`, which `count($value)` is not, one holding a
//     quantified expression, over a child step, `every $c in c satisfies
//     $c/@a`, or over `$value`, `every $x in data($value) satisfies $x gt 0`,
//     and one calling fn:doc over a StringLiteral, `exists(doc('x.xml'))`,
//     which `empty(doc(()))` is not.
//
// A union's own assertions facet is evaluated like any other: the pipeline
// hands this evaluator the union's ·active basic member· as st, the type under
// which `$value` is the value's XDM representation (dt-xdmrep clause 4,
// cvc-assertions-valid clause 1.4), so a {test} is compiled against that
// member and declines only where it would over the member itself.
//
// GAP(xpath): an fn:count call over a path is declined, whose argument would
// raise err:XPDY0002 over the absent context item (ctaFacetFacade.count), and
// so is a quantified expression, whose binding step would raise it
// (ctaFacetFacade.quantified, ctaParser.childQuantified), and one over
// `$value`, which would not (ctaFacetFacade.rangeScope), and an fn:doc call
// over a StringLiteral, which [CompileAssertionTest] admits and which would
// raise err:FODC0005 (ctaFacetFacade.document). Every other decline
// is [CompileAssertionTest]'s, under its GAP(xpath). The direction is the
// withhold: the caller declines the value's Datatype Valid verdict, never
// charging it and never showing it satisfied. (#1042)
//
// now is the dynamic context's current dateTime, which every {test} the
// evaluator decides reads on [AssertionTest.Evaluate]'s terms: fn:current-date
// is the xs:date of now in now's own UTC offset, or in UTC where no
// timezoneFrag spells that offset. A zero now is the instant
// 0001-01-01T00:00:00Z on those terms, never the current time. validate builds
// one evaluator per call site from the one instant it reads at the start of
// each Validator.Assess, so every facet of one assessment episode sees the
// same current dateTime (cvc-xpath clause 6).
//
// Nothing is cached: each call compiles its {test} afresh (STYLE D3), and b and
// r are read as [AssertionTest.Evaluate] reads them and stored nowhere.
func FacetAssertions(now time.Time) value.AssertionEvaluator { return facetAssertions{now: now} }

// facetAssertions is [FacetAssertions]' implementation, fusing the compile and
// the evaluation so no compiled facet tree crosses the package boundary. now
// is the current dateTime it was built with.
type facetAssertions struct{ now time.Time }

// Evaluate decides test against v, a value of st, on [FacetAssertions]' terms.
func (f facetAssertions) Evaluate(b value.Backend, r xsd.TypeResolver, st *xsd.SimpleType, test xsd.XPathExpression, v value.Value) value.AssertionOutcome {
	root, defect := compileCTATest(test, r, ctaFacetFacade{st: st})
	if defect.kind != ctaNoDefect {
		return value.AssertionDeclined
	}
	// The string value bound is "" and unread, and no element is bound: the
	// facet tree compiles `.` to ctaNoContextItem, and to ctaAbsentNode as a
	// node (cvc-assertions-valid clause 1.2).
	in := ctaTypedInput{attrs: noTypedAttributes, children: noChildElements, value: BindValue("", Typed(v)), now: f.now}
	if ctaEval(root, ctaEnv{backend: b, types: r, input: in}) != ctaTrue {
		return value.AssertionFails
	}
	return value.AssertionHolds
}

// ctaAssertionsDeclined is the [value.AssertionEvaluator] a Type
// Alternative's casts validate with (ctaLexicalInput.facets). It declines every
// {test}: a Type Alternative's evaluation holds no current dateTime for
// fn:current-date to read, and deciding at a made-up one would answer from a
// value nothing supplied. It twins value's unexported assertionsUndecided,
// which this package cannot name; ctaValidate's GAP(xpath) owns what a
// decline costs.
type ctaAssertionsDeclined struct{}

// Evaluate declines every {test}.
func (ctaAssertionsDeclined) Evaluate(value.Backend, xsd.TypeResolver, *xsd.SimpleType, xsd.XPathExpression, value.Value) value.AssertionOutcome {
	return value.AssertionDeclined
}

// noTypedAttributes and noChildElements are the empty inputs a facet
// evaluation carries: a facet tree holds no attribute or child step to read
// either, since ctaFacetFacade compiles each to ctaNoContextItem.
func noTypedAttributes(func(xsd.QName, TypedValue) bool) {}

func noChildElements(func(ChildElement) bool) {}

// ctaFacetFacade is the facet façade compileCTATest parses for: `$value` is
// typed by st, the type [value.AssertionEvaluator] hands over — the simple type
// the assertions facet is effective on, or the ·active basic member· where that
// type is a union (dt-xdmrep clause 4) — and every read of the context item
// compiles to ctaNoContextItem.
type ctaFacetFacade struct{ st *xsd.SimpleType }

func (ctaFacetFacade) ctaFacade() {}

// attribute compiles any attribute step to ctaNoContextItem: the step's
// context node is the context item (xpath20.md §3.2.1), which is absent.
func (ctaFacetFacade) attribute(ctaNameTest, ctaTypes) (ctaValue, bool) {
	return ctaNoContextItem{}, true
}

// comparesValues is true: the {test} is full XPath 2.0, on
// ctaAssertionFacade.comparesValues' terms.
func (ctaFacetFacade) comparesValues() bool { return true }

// computes is true, on ctaAssertionFacade.computes' terms. An operand that
// reads the absent context item is ctaNoContextItem, whose err:XPDY0002 the
// arithmetic over it raises, so `. + 1` fails the facet rather than declining
// (cvc-assertions-valid clause 1.2's Note).
func (ctaFacetFacade) computes() bool { return true }

// callsLibrary is true, on ctaAssertionFacade.callsLibrary's terms. A call
// whose argument, written or implicit, reads the absent context item raises
// the err:XPDY0002 that argument's ctaNoContextItem does, so
// `string-length() > 0` fails the facet rather than declining.
func (ctaFacetFacade) callsLibrary() bool { return true }

// conditional is true, on ctaAssertionFacade.conditional's terms.
func (ctaFacetFacade) conditional() bool { return true }

// constructsSequences is true, on ctaAssertionFacade.constructsSequences'
// terms.
func (ctaFacetFacade) constructsSequences() bool { return true }

// castable is true, on ctaAssertionFacade.castable's terms. An operand that
// reads the absent context item is ctaNoContextItem, whose err:XPDY0002
// evaluating it raises, so `. castable as xs:date` fails the facet rather than
// answering false: xpath20.md §3.10.3's "If evaluation of E fails with a
// dynamic error, the castable expression as a whole fails" (ctaCastableItem).
func (ctaFacetFacade) castable() bool { return true }

// instanceOf is true, on ctaAssertionFacade.instanceOf's terms. fn:data over
// a read of the absent context item raises its ctaNoContextItem's
// err:XPDY0002, so `data(.) instance of xs:untypedAtomic` fails the facet
// rather than answering (ctaInstanceOfItem).
//
// GAP(xpath): an AttributeTest over an attribute step, `@d instance of
// attribute()`, declines rather than raising the err:XPDY0002 its step's
// ctaNoContextItem would: ctaParser.attributeInstanceOf admits an attribute
// node alone. The direction is the withhold [CompileAssertionTest] reports.
// (#1042)
func (ctaFacetFacade) instanceOf() bool { return true }

// variable compiles `$value` (clause 1.1: "no namespace URI and ... 'value' as
// the local name") against st as ctaTypes.valueVariable classifies it, the XDM
// representation of a value of st (dt-xdmrep), and declines every other name,
// which is not in the static context (err:XPST0008), on
// ctaAssertionFacade.variable's terms.
func (f ctaFacetFacade) variable(name xsd.QName, types ctaTypes) (ctaValue, bool) {
	if name != ctaValueName {
		return nil, false
	}
	return types.valueVariable(f.st)
}

// child compiles any child-axis step to ctaNoContextItem, on attribute's
// terms.
func (ctaFacetFacade) child(ctaNameTest, ctaTypes) (ctaValue, bool) {
	return ctaNoContextItem{}, true
}

// childPath compiles any path of child steps to ctaNoContextItem, on
// attribute's terms: its first step's context node is the absent context item.
func (ctaFacetFacade) childPath([]xsd.QName, ctaElementTest) (ctaValue, bool) {
	return ctaNoContextItem{}, true
}

// attributePath compiles any path through child steps to an attribute to
// ctaNoContextItem, on childPath's terms, so `sum(a/@p) = 0` fails the facet
// rather than declining.
func (ctaFacetFacade) attributePath([]xsd.QName, ctaNameTest, ctaTypes) (ctaValue, bool) {
	return ctaNoContextItem{}, true
}

// elementPath compiles any path of child steps read for its values to
// ctaNoContextItem, on childPath's terms, so `data(a/b) instance of xs:date*`
// fails the facet rather than declining.
func (ctaFacetFacade) elementPath([]xsd.QName, ctaTypes) (ctaValue, bool) {
	return ctaNoContextItem{}, true
}

// childrenHaving compiles any child step filtered by its children's existence
// to ctaNoContextItem, on childPath's terms: the step its predicate filters
// reads the absent context item before the predicate is evaluated.
func (ctaFacetFacade) childrenHaving(xsd.QName, []xsd.QName) (ctaValue, bool) {
	return ctaNoContextItem{}, true
}

// childrenPreceded compiles any child step filtered by a `preceding::` step to
// ctaNoContextItem, on childrenHaving's terms.
func (ctaFacetFacade) childrenPreceded(ctaChildrenPreceded) (ctaValue, bool) {
	return ctaNoContextItem{}, true
}

// elements compiles any element step, `N`, `./N` or `.//N`, to
// ctaNoContextItem, on childPath's terms: `N` is a child step of the absent
// context item, and `./N` and `.//N` open with a read of it, `.`.
func (ctaFacetFacade) elements(ctaCountPath) (ctaValue, bool) {
	return ctaNoContextItem{}, true
}

// comments compiles any comment step, `comment()`, `./comment()` or
// `.//comment()`, to ctaNoContextItem, on elements' terms: what the step would
// select is never asked, the context item it steps from being absent
// (cvc-assertions-valid clause 1.2), so `empty(.//comment())` fails the facet.
func (ctaFacetFacade) comments() (ctaValue, bool) {
	return ctaNoContextItem{}, true
}

// quantified declines every quantified expression. Its binding sequence is a
// child step of the absent context item and would raise err:XPDY0002, but the
// body is counted off a [Tally] the facet evaluation has none of; that decline
// is [FacetAssertions]' withhold, under its GAP(xpath), on count's terms.
func (ctaFacetFacade) quantified(ctaQuantifier, ctaRangeKey) (ctaExpr, bool) {
	return nil, false
}

// rangeScope declines every quantifier over `$value`, although `$value` is
// bound here and §3.13 admits full XPath 2.0 in a facet's {test}: the decline
// is [FacetAssertions]' withhold, under its GAP(xpath). (#1042)
func (ctaFacetFacade) rangeScope(xsd.QName, ctaValueVar) (ctaFacade, bool) {
	return nil, false
}

// childScope declines every quantifier over a child step, on quantified's
// terms. It is never reached: child compiles the binding step to
// ctaNoContextItem, which is no ctaTypedChild, so ctaParser.childQuantified
// declines before the body's scope is asked for.
func (ctaFacetFacade) childScope(xsd.QName, ctaTypedChild) (ctaFacade, bool) {
	return nil, false
}

// boundSibling declines: `$value` is the one variable in scope, bound to
// atomic values (clause 1.1), from which no axis steps.
func (ctaFacetFacade) boundSibling(xsd.QName, ctaNameTest, ctaTypes) (ctaValue, bool) {
	return nil, false
}

// rooted compiles a path opening with "/" or "//" to ctaNoContextItem: the
// path begins at `fn:root(self::node())` (xpath20.md §3.2), a read of the
// context item that raises err:XPDY0002 before the `treat as` whose
// err:XPDY0050 ctaNoDocumentRoot is.
func (ctaFacetFacade) rooted() (ctaValue, bool) {
	return ctaNoContextItem{}, true
}

// contextItem compiles `.` to ctaNoContextItem (xpath20.md §3.1.4).
func (ctaFacetFacade) contextItem() (ctaValue, bool) {
	return ctaNoContextItem{}, true
}

// contextNode compiles `.` as the node fn:namespace-uri and
// fn:in-scope-prefixes take to ctaAbsentNode, on contextItem's terms: the
// call raises err:XPDY0002 (xpath-functions.md §14.3), so `namespace-uri() =
// ""` fails the facet rather than declining or holding.
func (ctaFacetFacade) contextNode() (ctaNodeArg, bool) {
	return ctaAbsentNode{}, true
}

// document declines every call to fn:doc whose argument is a StringLiteral.
// cvc-assertions-valid clause 1 evaluates the {test} on cvc-xpath's rules with
// modifications that leave the available documents alone, which reads as
// clause 8's empty set and err:FODC0005; no grounding has ruled on it, so the
// call is withheld rather than charged. `doc(())` never reaches here: it is
// the empty sequence on every façade (ctaParser.docCall).
//
// GAP(xpath): the call declines where it would raise. The direction is
// [FacetAssertions]' withhold. (#1042)
func (ctaFacetFacade) document() (ctaValue, bool) {
	return nil, false
}

// focus compiles a call to fn:position or fn:last to ctaNoFocus, typed st:
// there is no context position or size (cvc-assertions-valid clause 1.3) and
// no context item (clause 1.2), so the call raises err:XPDY0002
// (xpath-functions.md §16.1, §16.2) and `position() le 50` fails the facet
// rather than declining.
func (ctaFacetFacade) focus(st *xsd.SimpleType) (ctaValue, bool) {
	return ctaNoFocus{st: st}, true
}

// count declines every fn:count call over a path, which reads the absent
// context item and would raise err:XPDY0002 but is counted off a [Tally] the
// facet evaluation has none of; that decline is [FacetAssertions]' withhold,
// under its GAP(xpath). An fn:count call over an operand that is no path,
// `count($value)`, never reaches it (ctaParser.countCall).
func (ctaFacetFacade) count(ctaCounted, ctaTypes) (ctaValue, bool) {
	return nil, false
}
