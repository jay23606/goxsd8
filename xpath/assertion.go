package xpath

import (
	"slices"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// This file is the ASSERTION façade over the §3.12.6 grammar compileCTATest
// parses (STYLE T4): an {assertions} member's {test} (§3.13.1) written in that
// grammar is compiled and evaluated against the element it guards, which
// cvc-assertion (§3.13.4.1) asks of it. It holds the first slices of tier 2
// (doc.go) and nothing wider: the grammar is the Type Alternative one, widened
// only where the façade admits a production (ctaFacade) — an assertion's
// instance is TYPED.
//
// cvc-assertion clause 1 builds the XDM instance from the partial ·PSVI· of E,
// so each attribute carries the type annotation that ·PSVI· gives it
// (xpath-datamodel §3.3.1.1): the type its ·governing attribute declaration·
// gave it where it is ·valid· there, and §3.13.2's own example relies on that:
// in `@min le @max` over two xs:int attributes "the typed values of the
// attributes are available for comparison; it is not necessary to cast". A
// Type Alternative's instance is untyped instead (key-cta-ta-select clause 1's
// Note), which is why the two façades build different nodes for one [17]
// ta-AttrName over a typed attribute and take different attribute inputs, and
// why neither tree can be fed the other's input. An attribute annotated
// ·special· is the one an assertion reads untyped too — one whose type is
// ·special·, or one invalid in the partial ·PSVI·, which is annotated
// xs:anySimpleType: its typed value is xs:untypedAtomic (xpath-datamodel
// §3.3.1.2), so both façades build the same node for it.
//
// The assertion façade widens the grammar by NINE productions the Type
// Alternative façade declines: the eq/ne/lt/le/gt/ge value comparisons
// (xpath20.md §3.5.1, [23] ValueComp), in [11] ta-BooleanExpr's comparator
// position; three more arms of [16] ta-SimpleValue — the variable reference
// `$value` (xpath20.md [44] VarRef), which cvc-assertion clause 2.2 adds to an
// assertion's static context where ta-props-correct clause 2's grammar names
// no variable; an abbreviated child-axis step with a QName NameTest
// (§3.2.1.1), which reads E's element [[children]], whose typed values
// cvc-assertion clause 1.2's partial ·PSVI· holds; and a path opening with "/"
// or "//" over one child or attribute step, which raises err:XPDY0050 over
// that instance (§3.2); a relative path of two or more such child steps (§3.2,
// [26] RelativePathExpr), the last of which may be the [37] Wildcard `*`
// (§3.2.1.2), one element step `N`, `./N` or `.//N` (§3.2.4), or one child
// step filtered by a [40] Predicate that is a conjunction of child steps,
// `N[a and b]` (§3.2.2), or one `preceding::` step, `N[preceding::M[not(P)]]`
// (§3.2.1.1), as the whole operand of fn:exists, fn:empty or an ·effective
// boolean value·, whether it selects a node of E's subtree being what a
// [Tally] carries whatever the selected elements' types (ctaFacade.childPath,
// ctaFacade.elements, ctaFacade.childrenHaving, ctaFacade.childrenPreceded); an
// fn:count call (xpath-functions.md §15.4.1) in [14] ta-ValueExpr's position,
// over one counted path of E's subtree, whose counts a [Tally] carries, or over
// an operand that is no path, such as `$value`, whose items it counts; inside
// that call's argument, xpath20.md [40] Predicate, one on a child step, testing
// the existence of the child's attributes, which the Tally reads too, or
// comparing the child's own value, read off [ChildElements] (§3.2.2); and [21]
// UnionExpr over such operands (§3.3.3), each node counted once; and, in [14]'s
// position, a call to one of the F&O string, sequence and date functions
// [CompileAssertionTest] lists (ctaFacade.callsLibrary, ctafunc.go), whose
// argument may also be the empty sequence `()`, or to fn:namespace-uri, and
// fn:in-scope-prefixes as an operand of `=`, each over E as a node
// ([ContextElement], ctaAssertionFacade.contextNode), or to fn:sum, whose one
// argument is a relative path of child steps ending in an attribute step,
// `a/b/@p` (§3.2), the typed values of whose attributes below E a [Tally]
// carries (ctaFacade.attributePath), or to fn:data under `instance of`, whose
// one argument is a relative path of two or more child steps, `a/b`, the typed
// values of whose elements below E a [Tally] carries (ctaFacade.elementPath).
// It also admits xpath20.md §3.4's binary arithmetic operators over numeric
// operands (ctaFacade.computes), in each comparison operand's position; §3.8's
// [7] IfExpr wherever an ExprSingle stands whole in a boolean position
// (ctaFacade.conditional), and there §3.9's [6] QuantifiedExpr over one child
// step, whose body tests the bound child's attributes or its next sibling
// element, its counts read off the [Tally] (ctaFacade.quantified), or over one
// typed child step, filtered by `[position() lt last()]` or not (§3.2.2), each
// child bound in turn to its range variable off [ChildElements], whose body
// reads its typed value, `$v/following-sibling::M[1]` and the attributes of
// either, `$v/@A`, each of which a function call may end as the path's last
// step, `$v/@A/F(…)` (ctaFacade.childScope, ctaFacade.boundSibling,
// ctaFacade.boundAttribute), or over the items of a typed `$value`, each bound in
// turn to its range variable (ctaFacade.rangeScope); [47] ContextItemExpr `.`,
// as E's string value (ctaAssertionFacade.contextItem); and, as a general
// comparison's operand, §3.3.1's integer and string sequences
// (ctaFacade.constructsSequences).

// AttributeTypes answers, for the element information item E whose assertions
// are being compiled, the {type definition} an attribute with the ·expanded
// name· name of the element at path is typed by. path holds the ·expanded
// names· of the elements from E's child down to the one that owns the
// attribute, and is read during the call and never kept.
//
// For the empty path the element is E, and the answer is the type annotation
// E's attribute so named carries in the partial ·PSVI· cvc-assertion clause
// 1.2 builds (xpath-datamodel §3.3.1.1), its {attribute use} being the one of
// E's ·governing type definition· matching that name: the type of that use's
// {attribute declaration} where the attribute is ·valid· there, and
// xs:anySimpleType where it is invalid. For a non-empty path it is the type of the
// {attribute use} named name of that element's STATIC type: the ·locally
// declared type· (key-ldt-elem) of path[0] within E's ·governing type
// definition·, then of each later step within the type found for the step
// before. ok false means no type is fixed for the name at compile time — a
// step's type is ·absent· or not a complex type, no use matches, a name only
// an {attribute wildcard} can admit, or a use whose declaration or type does
// not resolve — and [CompileAssertionTest] declines a {test} naming it.
//
// The answer for the empty path is per E: it depends on the [validity] of the
// attribute E carries, so its caller compiles per E. It does not depend on
// whether E carries the attribute, or a ·defaulted attribute· (key-dflt-att)
// stands in for it, both of which [TypedAttributes] yields. The answer for a
// non-empty path is STATIC in the sense the compile needs: it does not depend
// on the instance — whether the element carries the attribute or a
// ·defaulted attribute· stands in for it, both of which [Tally.Attribute]
// takes, a {type table}'s selection, an xsi:type, or the attribute's
// [validity]. So the ·governing type definition· of an element below E can
// differ from the type the compile read, and [Tally.Attribute] states which
// values agree with the answer. Its one consumer is validate's cvc-assertion
// site (validate/cvcassertion.go), which builds it, [TypedAttributes] and what
// it reports to [Tally.Attribute] from one lookup so they cannot disagree.
type AttributeTypes func(path []xsd.QName, name xsd.QName) (*xsd.SimpleType, bool)

// ElementTypes answers, for the element information item E whose assertions are
// being compiled, the {type definition} an element below E is read under. path
// holds the ·expanded names· of the elements from E's child down to the one
// whose type is asked, and is read during the call and never kept.
//
// For a path of one step, [N], the answer is the ·locally declared type· of a
// child named N within E's ·governing type definition· (key-ldt-elem). For a
// longer path, every step but the last follows the chain [AttributeTypes]
// follows for a non-empty path, and the answer is the ·locally declared type·
// of the last step within the type that chain reaches; it may be a simple or a
// complex type. ok false means no type is fixed at compile time — a step's type
// is ·absent·, or a step before the last is not a complex type — and
// [CompileAssertionTest] declines a {test} reading it. The empty path is E
// itself, whose value is `$value`'s and `.`'s ([BindValue]) and never this
// lookup's: it answers false.
//
// It is STATIC, on the terms [AttributeTypes] states for a non-empty path: the
// answer does not depend on which elements E's subtree carries, on a {type
// table}'s selection, or on the type an xsi:type gives one, so the ·governing
// type definition· of an element below E can differ from the type the compile
// reads, below a child even without being substitutable for it, and
// [ChildElements] and [Tally.ElementValue] state which values agree with it.
// Its one consumer is validate's cvc-assertion site
// (validate/cvcassertion.go), which builds it and maps the values it reports
// from one lookup, so they cannot disagree.
type ElementTypes func(path []xsd.QName) (xsd.TypeDefinition, bool)

// ChildElement is one element [[child]] of E as an assertion {test} reads it:
// its ·expanded name·, its typed value (xpath-datamodel §6.2.4) and the typed
// values of the attributes of its own a compiled step reads through a range
// variable bound to it. It is opaque, so its constructors are [Child] and
// [ChildWithAttributes].
//
// attrs holds those attributes in the order [ChildWithAttributes] drained
// them, each once; an attribute the child does not have is not among them, so
// the step reading it selects nothing.
type ChildElement struct {
	name  xsd.QName
	v     TypedValue
	attrs []childAttribute
}

// childAttribute is one attribute of a [ChildElement]: its ·expanded name· and
// its typed value.
type childAttribute struct {
	name xsd.QName
	v    TypedValue
}

// Child is the child element named name whose typed value is v, on the terms
// [ChildElements] states, carrying no attribute. Child(name, nil) is a
// ·nilled· child, whose typed value is the empty sequence whatever its type
// (xpath-datamodel §3.3.1.2; xpath20.md §2.5.2 item 4.1 for mixed content): a
// node all the same, which a {test} naming it finds.
func Child(name xsd.QName, v TypedValue) ChildElement { return ChildElement{name: name, v: v} }

// ChildWithAttributes is Child(name, v) that also carries the typed values
// attrs yields of the child's own attributes, which a compiled step reads
// through a range variable bound to this child: `$x/@a`,
// `$x/following-sibling::x[1]/@a`. attrs is read during the call and never
// kept. It must be non-nil.
func ChildWithAttributes(name xsd.QName, v TypedValue, attrs TypedAttributes) ChildElement {
	c := Child(name, v)
	attrs(func(a xsd.QName, av TypedValue) bool {
		c.attrs = append(c.attrs, childAttribute{name: a, v: av})
		return true
	})
	return c
}

// ChildElements yields E's element [[children]] a compiled {test} reads, in
// DOCUMENT ORDER (STYLE D1), each as a [ChildElement]. It must be non-nil, and
// a yield reporting false ends the walk. A child no compiled step names need
// not be yielded: [AssertionTest.ReadsChild] reports which names one reads for
// their values, or binds for their places alone, and
// [AssertionTest.ReadsChildAttributes] which attributes of a
// child of a name one reads through a range variable bound to it. Every child
// of a name either reports — true, or a non-empty slice — must be yielded,
// once, in its document-order place among the others yielded: a quantifier
// reads a child's position among its namesakes, the last of them, and the next
// child of a name after it off that order, and `data(N[k])` the k-th child
// named N (xpath20.md §3.2.1.1, §3.2.2).
//
// Each value's arm is fixed by the type [ElementTypes] answered for the
// one-step path [N], N its name, when the [AssertionTest] being evaluated was
// compiled; a ·nilled· child's is nil whatever that type. A complex type whose
// {content type}.{variety} is mixed — xs:anyType among them — gives [Untyped]
// of the child's string-value (xpath-datamodel §6.2.4), which spans the text
// of all its descendants and not only its own. Any other type gives [Typed] of
// a value of EXACTLY the simple type the compile read off it — that type
// itself, or the {simple type definition} of its simple {content type} — which
// [Typed] says how validate maps. That agreement is the caller's obligation,
// on the terms [TypedAttributes] states for an attribute's value: validate
// maps the child's [schema normalized value] under its OWN ·governing type
// definition· into that simple type, takes a string-value only where that
// governing type has mixed content too, and withholds the assertion where the
// child's own type cannot give the arm the compile read or is neither the
// compiled type nor validly derived from it — which an `instance of` over the
// child relies on, matching an AtomicType the compiled type derives from
// without reading the child's own type. A value breaking it — the other arm —
// is a dynamic error wherever the tree reads it, which
// [AssertionTest.Evaluate] answers false. The value of a child of a name
// ReadsChild reports false for is never read, so it may be nil whether or not
// the child is ·nilled·.
//
// A child of a name ReadsChildAttributes reports a non-empty slice for is
// built by [ChildWithAttributes], whose attrs yields each attribute of the
// child named in that slice, carried or ·defaulted·, on [TypedAttributes]'
// terms with [AttributeTypes] answering for the one-step path [N]: [Typed] of a
// value of EXACTLY that type — the compile reads no attribute whose type is
// ·special· this way — and the attribute not yielded at all where the child
// does not have it. A value of the other arm, a nil value or a name yielded
// twice breaks that obligation, a dynamic error wherever the tree reads it.
//
// It carries the children whose typed values a compiled step reads and nothing
// below them: those a value step names, and those an fn:count argument filters
// by a predicate over their value, `count(N[. = 'x'])`, which is counted over
// them here, and those a quantifier binds whose body reads the bound child's
// value, its attributes or its next namesake, `every $v in N[position() lt
// last()] satisfies $v lt $v/following-sibling::M[1]` — the children named N
// and those named M — with the typed values of the attributes of its own such
// a body reads, `number($x/@a) lt number($x/following-sibling::x[1]/@a)`,
// tied to the child that has them. A node a {test} only counts — a child, a
// descendant, an attribute, a child filtered by its attributes' existence — or
// only asks the existence of through a path, `a`, `.//a`, `a/b`, `a/*` or
// `a[b]`, is reported to the [Tally] instead, and never yielded here for that
// sake; so is the typed value of an attribute of a child or a deeper
// descendant read through a path from E, `sum(a/b/@p)` ([Tally.Attribute]),
// and the typed value of an element below a child that a path of two or more
// steps reads, `data(a/b)` ([Tally.ElementValue]): it carries E's DIRECT
// children alone, and [AssertionTest.ReadsChild] is false for both a and b of
// `data(a/b)`.
type ChildElements func(yield func(ChildElement) bool)

// TypedValue is the typed value of one attribute node, of one element node of
// simple type, simple content or mixed content (xpath-datamodel §6.2.4), or of
// `$value`, in the data model instance cvc-assertion clause 1 builds: an
// ·actual value· of a type fixed at compile time ([Typed]), or an
// xs:untypedAtomic value ([Untyped]) — the [schema normalized value] under
// xs:anySimpleType or xs:anyAtomicType (xpath-datamodel §3.3.1.2), or an
// element's string-value under mixed content (§6.2.4). Datatypes dt-xdmrep
// clause 1 gives a ·special· type's value that dynamic type. It is a sealed
// sum of exactly those two arms, so no value carries both a lexical and a
// typed value (STYLE T1).
type TypedValue interface{ typedValue() }

// tvTyped is [Typed]'s arm: an ·actual value· of the type the tree holds.
type tvTyped struct{ v value.Value }

// tvUntyped is [Untyped]'s arm: an xs:untypedAtomic value, by its lexical.
type tvUntyped struct{ lexical string }

func (tvTyped) typedValue()   {}
func (tvUntyped) typedValue() {}

// Typed is the typed value v, an ·actual value· mapped under a type whose
// typed value is not xs:untypedAtomic. Typed(nil) is nil, so a nil value has
// one encoding.
//
// v is the [value.Value] that [value.ValidateLexical] returns for the [schema
// normalized value] under the simple type the compile fixed for the node: the
// type [AttributeTypes] answered for an attribute; for an element below E, the
// simple type [ElementTypes] answered, or the {simple type definition} of the
// simple {content type} of the complex type it answered; for `$value`, the
// {simple type definition} [BindValue] names. That call, with
// [FacetAssertions](now) as its evaluator, is what package validate passes.
func Typed(v value.Value) TypedValue {
	if v == nil {
		return nil
	}
	return tvTyped{v: v}
}

// Untyped is lexical as an instance of xs:untypedAtomic. lexical must be what
// the data model makes that typed value of: the [schema normalized value] of
// an attribute, an element or `$value` whose type is xs:anySimpleType or
// xs:anyAtomicType (xpath-datamodel §3.3.1.2), or the string-value of an
// element whose type is a complex type with mixed content (§6.2.4).
func Untyped(lexical string) TypedValue { return tvUntyped{lexical: lexical} }

// TypedAttributes yields E's attributes that matched an {attribute use} of its
// ·governing type definition·, each as its ·expanded name· and its typed
// value: those E carries, in DOCUMENT ORDER (STYLE D1), then its ·defaulted
// attributes· (key-dflt-att), which the partial ·PSVI· cvc-assertion clause
// 1.2 builds from holds too, in an order the caller fixes. It must be non-nil,
// and a yield reporting false ends the walk.
//
// Each value's arm is fixed by the type [AttributeTypes] answered for its name
// at the empty path when the [AssertionTest] being evaluated was compiled:
// [Untyped] of the attribute's [schema normalized value] exactly when that type
// is ·special· — xs:anySimpleType or xs:anyAtomicType — and [Typed] of a value
// of EXACTLY that type otherwise, which the tree holds and converts the value
// from, and which [Typed] says how validate maps. That agreement is the
// caller's obligation, and it is the same one [BindValue] places on `$value`'s
// value against the {simple type definition} of the content type the test was
// compiled for. A value breaking it — nil, or the other arm — is a dynamic
// error wherever the tree reads it, which [AssertionTest.Evaluate] answers
// false.
//
// Unlike [Attributes], it carries no [inherited attributes]: the XDM instance
// cvc-assertion clause 1.3 builds contains E's own [[attributes]] and nothing
// from outside E. It carries E's attributes alone: an attribute of a child or a
// deeper descendant whose value a path reads is reported to the [Tally]
// ([Tally.Attribute]).
type TypedAttributes func(yield func(name xsd.QName, v TypedValue) bool)

// ValueBinding is what one evaluation reads of E itself, two independent facts
// [BindValue] binds: E's string value, which the [47] ContextItemExpr `.`
// atomizes to under every {content type}, and the value cvc-assertion clause
// 2.3 binds to `$value`.
//
// The string value is always one xs:untypedAtomic value: E's annotation in the
// partial ·PSVI· cvc-assertion clause 1.2 builds is xs:anyType (xpath-datamodel §3.3.1.1,
// [validation attempted] partial), a complex type with mixed content, whose typed value is
// its string value as xs:untypedAtomic (xpath20.md §2.5.2) — "its atomized value will be a
// single atomic value of type untypedAtomic", as clause 2.3.1's Note says, whatever E's
// {content type} — element-only, empty and mixed as well as simple, whatever the {simple
// type definition}'s variety. It has no absent state: an empty E's string value is the
// zero-length string (xpath-datamodel §6.2.4), which is xs:untypedAtomic "" and never the
// empty sequence. A ·nilled· E is no exception either way: dm:nilled is false in the
// partial ·PSVI·, whose [validity] is never valid, so a ·nilled· E with [[children]] has
// their text as its string value, not "".
//
// A nil `$value` is clause 2.3.2's empty sequence — for an E that is invalid
// in the partial ·PSVI·, ·nilled·, or under a {content type} that is not
// simple. The zero ValueBinding is BindValue("", nil), what a ·nilled· E with
// no [[children]] binds; an invalid E binds its string value beside a nil
// `$value`.
//
// It has no third state for an E whose value is UNDECIDED: [AssertionTest.Evaluate]
// always decides, so a caller that cannot tell which of clause 2.3's cases E is
// in, or what E's string value is, declines the assertion itself, as it does
// an attribute with no ·actual value·.
type ValueBinding struct {
	text string
	v    TypedValue
}

// BindValue binds E's string value text, which `.` reads, and `$value`'s
// value v, on the terms [ValueBinding] states. Each fact is the caller's to
// supply, and neither is derived from the other.
//
// text is E's string value as the data model instance holds it: the
// concatenation of its text node children (xpath-datamodel §6.2.4), which
// Appendix J.2's children rule builds from the character [[children]] of E in
// document order — its ·initial value· — or, where cvc-elt clause 5.1
// supplied a default because E has neither element nor character
// [[children]], the {value constraint}'s {lexical form}, the one text node
// J.2's "may" lets the data model build from the [schema normalized value] of
// a defaulted element. Each is a processor's choice J.2 leaves open, and
// validate's caller makes it (the GAP(xpath) at its cvc-assertion site,
// validate/cvcassertion.go). It is never re-normalized under the {simple type
// definition}'s whiteSpace: `.` over xs:integer content "0030" is "0030",
// where `$value` is 30. Under every other {content type} it is the
// concatenation of the string values of all E's Text Node descendants in
// document order (xpath-datamodel.md:1304): each character [[child]] of E,
// white space alone included — E is annotated xs:anyType, whose content is
// mixed (cvc-assertion clause 1.2) — and each of a descendant, except a run
// that is white space alone under a descendant whose {content type} is
// element-only or empty, which is no Text Node (:2202). An invalid E has a
// string value all the same, so text is bound where v is nil.
//
// v is the typed value of E's [schema actual value] (cvc-assertion clause
// 2.3.1), whose arm and type the {simple type definition} of the
// [xsd.SimpleContent] the [AssertionTest] was compiled for fixes, on the terms
// [TypedAttributes] states for an attribute's value: [Untyped] of E's [schema
// normalized value] where it is ·special· — the representation of that value
// xpath-datamodel §3.3.1.2 fixes, a ·special· type's lexical mapping not being
// a function (Datatypes §3.2.1.2) — and [Typed] of a value of exactly it
// otherwise, which [Typed] says how validate maps. A list {simple type
// definition}'s value must carry [value.Listed], whose items are the flattened
// sequence Datatypes dt-xdmrep makes its XDM representation. A nil v is the
// empty sequence of clause 2.3.2, so the empty sequence has one encoding.
func BindValue(text string, v TypedValue) ValueBinding { return ValueBinding{text: text, v: v} }

// ContextElement is E, the element information item cvc-xpath makes the
// context item of an assertion's {test}, as the functions that take `.` as a
// NODE read it: fn:namespace-uri reads its [namespace name] off Name, and
// fn:in-scope-prefixes asks LookupPrefix whether a prefix is in E's [in-scope
// namespaces]. It must be non-nil, even for a {test} that reads neither:
// [AssertionTest.Evaluate] states what a nil one does. validate.Element
// satisfies it as it stands. A parser/xmltree StartElement answers
// LookupPrefix by this rule already, but its Name returns an xmltree.Name, not
// an xsd.QName, so it needs a wrapper; validate/xmlsrc's element is one.
type ContextElement interface {
	// Name is E's ·expanded name·; its Space is E's [namespace name], "" for
	// none.
	Name() xsd.QName
	// LookupPrefix resolves prefix against E's [in-scope namespaces], the
	// bindings E inherits included. The empty prefix yields the default
	// namespace with ok true: uri is "" where none is in scope or `xmlns=""`
	// undeclares it. Any other prefix yields its namespace name with ok true
	// where it is bound, and ("", false) where it is unbound or `xmlns:p=""`
	// undeclares it; "xml" is always bound.
	//
	// fn:in-scope-prefixes reads uri alone: a prefix is in scope exactly where
	// uri is non-empty. validate reads ok as well, so an implementation
	// answers both as stated.
	LookupPrefix(prefix string) (uri string, ok bool)
}

// Tally is the subtree input of ONE evaluation of ONE [AssertionTest] over
// the element E: for each relative path the {test} counts over with fn:count
// (xpath-functions.md §15.4.1) — a step, a child step filtered by the
// existence of its attributes, or a union of those (xpath20.md §3.2.2,
// §3.3.3) — or asks the existence of with fn:exists, fn:empty or an ·effective
// boolean value· (§15.1.4, §15.1.5, xpath20.md §2.4.3) — among them a child
// step filtered by the existence of its own children, and one filtered by a
// `preceding::` step (§3.2.1.1) — or a quantified expression over a child step
// ranges over (§3.9) — the children it binds, and those its body holds for:
// those carrying an attribute, or followed by a sibling element of a name —
// how many nodes of E's subtree it selects. A child step filtered by its VALUE,
// and a quantifier reading its bindings' values, are not among them:
// [ChildElements] answers both. [AssertionTest.Tally] makes one, the caller
// reports E's subtree to it while that subtree streams past, and
// [AssertionTest.Evaluate] reads it. It keeps one counter per distinct path and
// no node: what the caller holds for a count is one integer per path, the chain
// of names of the latest report, which is O(depth), for a child step filtered
// by its children's existence, which of those children the latest child named
// by the step has shown so far, for one filtered by a `preceding::` step,
// whether an element it keeps has been reported and, for each element of the
// latest report's chain, whether it bears the preceding step's name and has
// shown the child its filter names, which is O(depth) too, and, for the
// children followed by a sibling of a name, whether the latest child of E
// reported bears the name they are drawn from — whatever the subtree's size.
//
// It also carries, for each path to an attribute whose values the {test}
// reads — fn:sum's argument `N1/…/@A` (xpath-functions.md §15.4.5, xpath20.md
// §3.2), atomized (§2.4.2) — the typed value of every attribute node the path
// selects, kept as a slice in the order the caller reported them
// ([Tally.Attribute]). Every node one such path selects sits at the one depth
// its steps fix, so that report order is document order (STYLE D1). The
// values are kept and nothing derived from them — the sum is computed when
// [AssertionTest.Evaluate] reads them (STYLE D3) — so what the Tally holds for
// such a path is O(nodes it selects), which is the size of the sequence the
// {test} reads. So it carries, for each path of two or more child steps whose
// elements' typed values the {test} reads — fn:data's argument `N1/…/Nk`
// (xpath-functions.md §2.4, xpath20.md §3.2) — the typed value of every
// element the path selects, kept as a slice in the order the caller reported
// them ([Tally.ElementValue]), on the same terms: every element one such path
// selects sits at the depth its steps fix, so none is another's ancestor, and
// the post-order in which their values arrive is document order too.
//
// The caller's obligation is to report EVERY node of E's subtree in the data
// model instance cvc-assertion clause 1 builds, exactly once, by one
// [Tally.Element] call per element, E itself included, in PRE-ORDER DOCUMENT
// ORDER: E first, then every element of E's subtree, each before its
// [[children]] and those in document order, so that each report at a depth
// of one or more below E belongs to the latest earlier report one level up.
// After each element's report and before the next, it reports the typed value
// of each of that element's attribute nodes whose value a path reads
// ([Tally.ReadsAttribute]), by one [Tally.Attribute] call each. The typed
// value of an element whose value a path reads ([Tally.ReadsElementValue]) is
// owed once, by one [Tally.ElementValue] call, AFTER that element's report and
// the reports of its whole subtree, and before the next report at its depth
// or above and before [AssertionTest.Evaluate] reads the Tally: an element's
// typed value is known only once its own subtree has been assessed.
// Each element is reported by the chain of ·expanded names· from E's child
// down to it — the empty chain for E — whatever its validity, whether it is
// ·nilled·, and whether it was ·strictly· or ·laxly assessed· or ·skipped·,
// with the names of ALL its attribute nodes: those it carries, xsi:type and
// the other xsi attributes among them, and its ·defaulted attributes·
// (key-dflt-att), which the partial ·PSVI· clause 1.2 builds from holds too;
// never a namespace declaration, which is not an attribute node.
//
// A report the order rules out — one at depth d of one or more whose chain's
// first d−1 names are not the first d−1 names of the report before it, a
// grandchild before its parent among them — breaks that obligation, on the
// terms [TypedAttributes] states for an attribute's value: the Tally drops
// every counter and collector, and [AssertionTest.Evaluate] answers false
// for it. So do the breaches of [Tally.Attribute]'s ordering that call
// states, an attribute value a read named in the latest report never got
// among them, and those of [Tally.ElementValue]'s, an element value owed when
// a report at its element's depth or above arrives, or when Evaluate reads the
// Tally, among them. A subtree reported short, an element reported under a
// wrong name in an order that still nests, or an element's value given before
// the reports of its subtree rather than after them, is not detected, and
// yields a wrong count or a value out of place.
//
// A ·skipped· element and everything below it is governed by no type
// (key-skipped, key-governing-type-elem item 5), so it has no ·defaulted
// attribute·: its attribute nodes are exactly those it carries, xsi ones
// included, and the typed value of each is xs:untypedAtomic, never the type
// [AttributeTypes] answered for a read of it. An element whose ·governing type
// definition· the caller could not determine has ·defaulted attributes· it
// cannot know, and may be reported with no attribute names only at a depth
// where [Tally.ReadsAttributesAt] is false; a caller that cannot report a
// subtree exactly otherwise, or cannot give a read attribute or a read element
// the value [Tally.Attribute] or [Tally.ElementValue] obliges — a ·skipped·
// one's is xs:untypedAtomic, never the type [ElementTypes] answered — declines
// the assertion itself, on the terms [ValueBinding] states for an undecided
// `$value`, and never reports a stand-in. Under-reporting would make a count
// too small, or a sum or sequence short, and could fabricate a charge.
//
// Its consumer is validate's cvc-assertion site (validate/cvcassertion.go),
// which reports each element it walks, and each element of a ·skipped· subtree
// by name, to the Tally of every enclosing element's {assertions} that has one.
// Its methods exist for package validate, and a caller outside the module is
// not expected to drive them.
type Tally struct {
	// counters holds one counter per counted key, in the order
	// [AssertionTest.Tally] made them (fits).
	counters []ctaCounter
	// collectors holds one collector per collected key, in the order
	// [AssertionTest.Tally] made them (fits).
	collectors []ctaCollector
	chain      []xsd.QName
	// pending is the names, in the latest report's order, of the attribute
	// nodes of the latest report whose value a path reads and which have not
	// had their [Tally.Attribute] call yet: a slice, never a map, so no state
	// is read in an order the reports did not give it.
	pending []xsd.QName
	// owed is the depths of the reported elements whose value a path reads
	// and which have not had their [Tally.ElementValue] call yet, innermost
	// last: each is an element of the latest report's chain, so it is
	// O(depth).
	owed []int
}

// ctaCounter is the sealed sum of the counters a [Tally] keeps, one per
// counted key (ctaCountedKey.counter): ctaReportCount, which adds what each
// report selects, ctaInstanceCount, which counts the children of E whose own
// children the reports after them show, ctaPrecedenceCount, which counts the
// children of E some element reported before which the reports after it
// kept, and ctaSuccessionCount, which counts the children of E whose next
// sibling element the next report at depth 1 names. The keys close the set
// (STYLE T2's schema-closed-set exception). Each arm is a pointer, which
// element updates in place. No arm is also a ctaCollector.
type ctaCounter interface {
	ctaCounter()
	// key is the key the counter counts under.
	key() ctaKey
	// element takes one [Tally.Element] report, already checked to follow the
	// report before it in pre-order (ctaNests).
	element(path, attrs []xsd.QName)
	// count is how many nodes the key selects among the reports taken so far.
	count() int
}

// ctaCollector is the sealed sum of the collectors a [Tally] keeps, one per
// collected key (ctaCollectedKey.collector): ctaAttributeCollector, which
// keeps the typed values of the attribute nodes a path to an attribute
// selects, and ctaElementCollector, which keeps those of the elements a path of
// child steps read for its value selects. The keys close the set (STYLE T2's
// schema-closed-set exception). Each arm is a pointer, which the value intake
// updates in place; the intake reaches its own arm by a switch over this sum.
// No arm is also a ctaCounter.
type ctaCollector interface {
	ctaCollector()
	// key is the key the collector collects under.
	key() ctaKey
	// collected is the typed values collected so far, in the order they were
	// reported.
	collected() []TypedValue
}

// ctaAttributeCollector keeps, for a ctaAttributePath key, the typed value of
// each attribute node it selects, in the order [Tally.Attribute] reported
// them. The values arrive by [Tally.Attribute] alone: an element report names
// its attribute nodes and carries none of their values.
type ctaAttributeCollector struct {
	read   ctaAttributePath
	values []TypedValue
}

func (*ctaAttributeCollector) ctaCollector() {}

func (a *ctaAttributeCollector) key() ctaKey { return a.read }

func (a *ctaAttributeCollector) collected() []TypedValue { return a.values }

// ctaElementCollector keeps, for a ctaElementPath key, the typed value of each
// element it selects, in the order [Tally.ElementValue] reported them, a
// ·nilled· one's nil among them. The values arrive by [Tally.ElementValue]
// alone: an element report carries none.
type ctaElementCollector struct {
	read   ctaElementPath
	values []TypedValue
}

func (*ctaElementCollector) ctaCollector() {}

func (e *ctaElementCollector) key() ctaKey { return e.read }

func (e *ctaElementCollector) collected() []TypedValue { return e.values }

// ctaReportCount counts a key each report decides alone (ctaTallied): n is how
// many of the nodes reported so far tallied selects.
type ctaReportCount struct {
	tallied ctaTallied
	n       int
}

// ctaInstanceCount counts a ctaChildrenHaving key, `N[a and b]`: n is how many
// children of E named N, among those whose instance is closed — a later child
// of E has been reported — have shown a child named each required name; open
// reports whether the latest child of E reported is named N; and shown holds,
// in required's written order, which of the required names that child's own
// children reported so far carry.
type ctaInstanceCount struct {
	having ctaChildrenHaving
	n      int
	open   bool
	shown  []bool
}

// ctaSuccessionCount counts a ctaChildrenFollowedBy key,
// `$v/following-sibling::*[1][self::M]` over the children named N: n is how
// many children of E named N have been followed, as the next child of E
// reported, by one named M; and latestIsName reports whether the latest child
// of E reported is named N, false before any is.
type ctaSuccessionCount struct {
	followed     ctaChildrenFollowedBy
	n            int
	latestIsName bool
}

// ctaPrecedenceCount counts a ctaChildrenPreceded key, `N[preceding::M[…]]`:
// n is how many children of E named N have been reported after an element
// named M that the key's test keeps; kept reports whether such an element has
// closed — a report at its own depth or above has followed it — so far; and
// open holds, for each element of the latest report's chain from E's child
// down, whether it is named M and whether a child the test marks has been
// reported below it. Only a closed element is decided, and every element
// reported before a child of E is closed by that child's report: the
// pre-order [Tally] obliges makes its chain the child alone.
type ctaPrecedenceCount struct {
	preceded ctaChildrenPreceded
	n        int
	kept     bool
	open     []ctaOpenElement
}

// ctaOpenElement is one element of a ctaPrecedenceCount's open chain: named
// reports whether it is named M, and shown whether a child its key's test
// marks has been reported below it.
type ctaOpenElement struct {
	named bool
	shown bool
}

func (*ctaReportCount) ctaCounter()     {}
func (*ctaInstanceCount) ctaCounter()   {}
func (*ctaPrecedenceCount) ctaCounter() {}
func (*ctaSuccessionCount) ctaCounter() {}

func (r *ctaReportCount) key() ctaKey     { return r.tallied }
func (i *ctaInstanceCount) key() ctaKey   { return i.having }
func (p *ctaPrecedenceCount) key() ctaKey { return p.preceded }
func (s *ctaSuccessionCount) key() ctaKey { return s.followed }

// element closes every open element at the report's depth or below it,
// noting whether the test keeps one named M; counts a report of a child of E
// named N where one was kept before it, every element reported before it
// being closed and none its ancestor (xpath20.md §3.2.1.1); marks the
// report's parent where the test marks the report's name; and opens the
// report. A report at depth 0, E itself, closes everything and opens nothing:
// E is the ancestor of every element and never preceding.
func (p *ctaPrecedenceCount) element(path, _ []xsd.QName) {
	d := len(path)
	stays := max(d-1, 0)
	for _, closed := range p.open[stays:] {
		if closed.named && p.preceded.test.keeps(closed.shown) {
			p.kept = true
		}
	}
	p.open = p.open[:stays]
	if d == 0 {
		return
	}
	if d == 1 && p.kept && path[0] == p.preceded.name {
		p.n++
	}
	if d >= 2 && p.preceded.test.marks(path[d-1]) {
		p.open[d-2].shown = true
	}
	p.open = append(p.open, ctaOpenElement{named: path[d-1] == p.preceded.preceding})
}

// count is n: whether a child of E is selected is decided at its own report,
// which closes every element before it.
func (p *ctaPrecedenceCount) count() int { return p.n }

// element reads a report of a child of E as the next sibling element of the
// child reported before it, counting that one where it is named N and this
// one M, and then notes whether this one is named N. A report at depth 0, E
// itself, or at depth 2 or more, below a child, is no sibling of E's children
// and decides nothing: the pre-order [Tally] obliges makes the next report at
// depth 1 the next element child of E whatever is reported between.
func (s *ctaSuccessionCount) element(path, _ []xsd.QName) {
	if len(path) != 1 {
		return
	}
	if s.latestIsName && path[0] == s.followed.next {
		s.n++
	}
	s.latestIsName = path[0] == s.followed.name
}

// count is n: the last child of E has no next sibling element, so an N
// reported last is never counted.
func (s *ctaSuccessionCount) count() int { return s.n }

// element adds the nodes the report carries that tallied selects
// (ctaSelected).
func (r *ctaReportCount) element(path, attrs []xsd.QName) {
	r.n += ctaSelected(r.tallied, path, attrs)
}

// count is n.
func (r *ctaReportCount) count() int { return r.n }

// element reads a report of a child of E as the start of the next instance,
// closing the open one — counted where satisfied — and opening one where the
// child is named N; and a report of a grandchild of E, whose parent the
// pre-order makes the latest child of E, as showing the required name it
// carries. Every other report decides nothing.
func (i *ctaInstanceCount) element(path, _ []xsd.QName) {
	switch len(path) {
	case 1:
		if i.satisfied() {
			i.n++
		}
		i.open = path[0] == i.having.name
		clear(i.shown)
	case 2:
		for k, r := range i.having.required {
			if path[1] == r {
				i.shown[k] = true
			}
		}
	}
}

// count is n, plus one where the open instance is satisfied: no later report
// takes back a required name it has shown.
func (i *ctaInstanceCount) count() int {
	if i.satisfied() {
		return i.n + 1
	}
	return i.n
}

// satisfied reports whether an instance is open and has shown every required
// name.
func (i *ctaInstanceCount) satisfied() bool {
	return i.open && !slices.Contains(i.shown, false)
}

// Element reports one element of E's subtree, by path, and its attribute
// nodes, by attrs. path is the ·expanded names· of the elements from E's child
// down to the reported one inclusive, so len(path) is its depth below E — 0
// for E itself, 1 for a child, 2 for a grandchild, and so on. attrs is the
// names of ALL its attribute nodes, carried or ·defaulted·, on the terms
// [Tally] states, which also fix the order the reports come in. Neither slice
// is retained: both are read during the call, and path is copied.
//
// Every path the {test} counts selecting the element counts it — `N` where
// path is [N], `.//N` where path ends in N at any depth, since xpath20.md
// §3.2.4 makes `.//N` `./descendant-or-self::node()/child::N`, which never
// selects E itself, `N1/N2/…` where path is exactly its steps (§3.2), and
// `N1/…/*` where path is the steps before the `*` followed by any one name,
// `*` on the child axis matching every element (§3.2.1.2) — and every path
// selecting one of its attribute nodes counts that node: `@N` at depth 0
// only, and `.//@N` at every depth, E's own included, since §3.2.4 makes
// `.//@N` `./descendant-or-self::node()/attribute::N`. `N[a and …]` counts a
// child named N once the reports at depth 2 that follow it, before the next
// report at depth 1, include one named each of a, … (§3.2.2).
// `N[preceding::M]` counts a child named N where some report before it but
// E's is named M, at any depth — `N[preceding::M[P]]` where that M's reports
// at the next depth down, before a report at its own depth or above, include
// one named P, and `N[preceding::M[not(P)]]` where they include none
// (§3.2.1.1, §3.2.2; cvc-assertion clause 1.3 roots the tree at E).
// `$v/following-sibling::*[1][self::M]`, over each child named N a quantifier
// binds to `$v`, counts a child named N once the next report at depth 1 after
// it, its next sibling element, is named M (§3.2.1.1, §3.2.2), whatever is
// reported below it in between, and never the last child of E. The empty path
// is E, whose element node no path selects. Every report to a nil Tally
// selects nothing.
//
// Each name of attrs whose value a path reads at path ([Tally.ReadsAttribute])
// is owed one [Tally.Attribute] call before the next report, and, where a path
// reads the reported element's own value ([Tally.ReadsElementValue]), the
// element is owed one [Tally.ElementValue] call once its subtree is reported.
// A report arriving while a value of the latest one is still owed, or at the
// depth of an element still owing its value or above it, breaks [Tally]'s
// obligation, on the terms a report out of pre-order does.
func (c *Tally) Element(path []xsd.QName, attrs []xsd.QName) {
	if c == nil {
		return
	}
	if len(c.pending) != 0 || !ctaNests(c.chain, path) || len(c.owed) != 0 && len(path) <= c.owed[len(c.owed)-1] {
		c.breach()
	}
	c.chain = append(c.chain[:0], path...)
	if c.ReadsElementValue(path) {
		c.owed = append(c.owed, len(path))
	}
	for _, counter := range c.counters {
		counter.element(path, attrs)
	}
	c.pending = c.pending[:0]
	for _, a := range attrs {
		if c.ReadsAttribute(path, a) {
			c.pending = append(c.pending, a)
		}
	}
}

// ReadsAttribute reports whether the {test} c was made for reads the typed
// value of the attribute node named name of each element whose chain of names
// below E, from E's child down to the element itself, is path — a path to an
// attribute, `N1/…/@A`, whose steps are path and whose attribute step names
// name. It is false for a nil Tally, for an empty path — E's own attributes
// are [TypedAttributes]' alone — and for every name and path no such path
// reads.
//
// Its consumer is validate's walk (validate/cvcassertion.go), which reports
// the value of each attribute node of a reported element it is true for with
// [Tally.Attribute].
func (c *Tally) ReadsAttribute(path []xsd.QName, name xsd.QName) bool {
	if c == nil || len(path) == 0 {
		return false
	}
	for _, collector := range c.collectors {
		if a, isAttribute := collector.(*ctaAttributeCollector); isAttribute && a.read.reads(path, name) {
			return true
		}
	}
	return false
}

// Attribute reports v, the typed value of the attribute node named name of the
// element the latest [Tally.Element] call reported. A value travels with its
// element's report — after that call and before the next — and does not repeat
// the path. It is owed for each attribute node of that report for which
// [Tally.ReadsAttribute] is true, exactly once; nothing else may be reported.
//
// Its arm and type are fixed by the type [AttributeTypes] answered for (path,
// name) when the [AssertionTest] being evaluated was compiled, path being the
// latest report's, on the terms [TypedAttributes] states for an attribute of
// E — [Untyped] of its [schema normalized value] exactly where that type is
// ·special·, and [Typed] of a value of EXACTLY that type otherwise, which
// [Typed] says how validate maps. That agreement is the caller's obligation,
// and a value breaking it — nil, or the other arm — is a dynamic error
// wherever the tree reads it, which [AssertionTest.Evaluate] answers false.
// [AttributeTypes] answers statically for a non-empty path, so the
// attribute's own type can differ from the one compiled: validate maps the
// attribute's [schema normalized value] under its OWN attribute use's type
// into the compiled one, and withholds the assertion where its own type is
// neither the compiled type nor validly derived from it, or, for a ·special·
// compiled type, is not that type itself.
//
// A call that breaks the ordering above breaks [Tally]'s obligation, on the
// terms a report out of pre-order does: one before any report, after E's own
// report (the empty path), for a name not among the latest report's attribute
// names, for one [Tally.ReadsAttribute] is false for, and a second call for one
// name of one report. Each value is kept, never v's place in a running total.
// A call to a nil Tally takes nothing.
func (c *Tally) Attribute(name xsd.QName, v TypedValue) {
	if c == nil {
		return
	}
	owed := slices.Index(c.pending, name)
	if owed < 0 {
		c.breach()
		return
	}
	c.pending = slices.Delete(c.pending, owed, owed+1)
	for _, collector := range c.collectors {
		if a, isAttribute := collector.(*ctaAttributeCollector); isAttribute && a.read.reads(c.chain, name) {
			a.values = append(a.values, v)
		}
	}
}

// ReadsElementValue reports whether the {test} c was made for reads the typed
// value of each element whose chain of names below E, from E's child down to
// the element itself, is path — a path of two or more child steps, `N1/…/Nk`,
// read for its value as fn:data's argument. It is false for a nil Tally and
// for every path shorter than two: a child's value is [ChildElements]' alone,
// and E's is [ValueBinding]'s.
//
// Its consumer is validate's walk (validate/cvcassertion.go), which reports
// the value of each element it is true for with [Tally.ElementValue], once
// that element's subtree is walked.
func (c *Tally) ReadsElementValue(path []xsd.QName) bool {
	if c == nil || len(path) < 2 {
		return false
	}
	for _, collector := range c.collectors {
		if e, isElement := collector.(*ctaElementCollector); isElement && e.read.reads(path) {
			return true
		}
	}
	return false
}

// ElementValue reports v, the typed value of the innermost element reported so
// far that still owes one: an element for which [Tally.ReadsElementValue] was
// true at its [Tally.Element] report and whose value has not arrived. It takes
// no path, so a value for any other element cannot be expressed, and a value
// the caller skipped leaves its element owing, which the next report at its
// depth or above, or Evaluate, finds. The value is owed once, after the
// reports of the element's whole subtree, on the terms [Tally] states.
//
// Its arm and type are fixed by the type [ElementTypes] answered for the
// element's path when the [AssertionTest] being evaluated was compiled, on the
// terms [ChildElements] states for a child's value read TYPED: [Typed] of a
// value of EXACTLY the simple type the compile read off it — that type, or the
// {simple type definition} of its simple {content type} — which [Typed] says
// how validate maps. A nil v is a ·nilled· element's empty typed value
// (xpath-datamodel §6.2.4), as [Child](name, nil) is: it contributes no item,
// and is no breach. A value of the other arm is a dynamic error wherever the
// tree reads it, which [AssertionTest.Evaluate] answers false. [ElementTypes]
// answers statically, so the element's own type can differ from the one
// compiled: validate maps the element's [schema normalized value] under its
// OWN ·governing type definition· into the compiled one, and withholds the
// assertion where its own type is neither the compiled type nor validly
// derived from it, never testing it against the ·locally declared type· within
// its parent's own type.
//
// A call while nothing is owed — before any report, after a report of E or
// of a child of E that nothing below owes, or a second value for one element —
// and a call while an [Tally.Attribute] value of the latest report is still
// owed break [Tally]'s obligation, on the terms a report out of pre-order
// does. Each value is kept. A call to a nil Tally takes nothing.
func (c *Tally) ElementValue(v TypedValue) {
	if c == nil {
		return
	}
	if len(c.owed) == 0 || len(c.pending) != 0 {
		c.breach()
		return
	}
	path := c.chain[:c.owed[len(c.owed)-1]]
	c.owed = c.owed[:len(c.owed)-1]
	for _, collector := range c.collectors {
		if e, isElement := collector.(*ctaElementCollector); isElement && e.read.reads(path) {
			e.values = append(e.values, v)
		}
	}
}

// breach drops every counter and collector c holds: a report or value breaking
// [Tally]'s obligation, which [AssertionTest.Evaluate] then answers false for
// (fits). It is the one site of that drop. The values owed go with the
// collectors, so every depth still owed is one of the latest report's chain.
func (c *Tally) breach() {
	c.counters = nil
	c.collectors = nil
	c.owed = nil
}

// settled reports whether c owes no [Tally.Attribute] call for the latest
// report and no [Tally.ElementValue] call, which a nil Tally never does:
// [AssertionTest.Evaluate] reads a Tally still owing one as broken.
func (c *Tally) settled() bool { return c == nil || len(c.pending) == 0 && len(c.owed) == 0 }

// values is the typed values c collected for key, false where it holds no
// collector for it. It reads collectors alone: no count is a value.
func (c *Tally) values(key ctaKey) ([]TypedValue, bool) {
	if c == nil {
		return nil, false
	}
	for _, collector := range c.collectors {
		if collector.key().same(key) {
			return collector.collected(), true
		}
	}
	return nil, false
}

// ctaNests reports whether a report by path may follow one by prev in the
// pre-order [Tally] obliges: E's report, the empty path, at any point, and a
// report at depth d of one or more where prev's first d−1 names are path's.
func ctaNests(prev, path []xsd.QName) bool {
	d := len(path)
	if d == 0 {
		return true
	}
	return len(prev) >= d-1 && slices.Equal(prev[:d-1], path[:d-1])
}

// ctaSelected is how many of the nodes one [Tally.Element] report carries — the
// element at path, and its attribute nodes attrs — key selects, each node
// counted once.
func ctaSelected(key ctaTallied, path, attrs []xsd.QName) int {
	n := 0
	if key.selectsElement(path, attrs) {
		n++
	}
	for _, a := range attrs {
		if key.selectsAttribute(len(path), a) {
			n++
		}
	}
	return n
}

// ReadsAttributesAt reports whether the attribute names reported for an
// element depth levels below E, 0 being E, can change any count or any value
// read c holds: `@N` selects attribute nodes at depth 0 only, and `.//@N` at
// every depth from 0, while `N[@A]`, and a quantifier's `$v/@A` over the
// children named N, select by the attribute names of depth 1, and a path to an
// attribute, `N1/…/@A`, reads the values of the attribute nodes at the depth
// its steps fix, len(steps); a path read for its elements' values,
// `data(N1/…/Nk)`, reads no attribute name at any depth. It is false for a nil
// Tally and for a depth below 0, where no [Tally.Element] report stands.
//
// Its consumer is validate's walk (validate/cvcassertion.go), which asks it of
// an element whose ·defaulted attributes· it cannot know: where it is false,
// no attribute name reported for that element could change any count or any
// value read, and the element is reported with none.
func (c *Tally) ReadsAttributesAt(depth int) bool {
	if c == nil {
		return false
	}
	for _, counter := range c.counters {
		if counter.key().selectsAttributesAt(depth) {
			return true
		}
	}
	for _, collector := range c.collectors {
		if collector.key().selectsAttributesAt(depth) {
			return true
		}
	}
	return false
}

// count is the count c holds for key, false where it holds no counter for it.
// It reads counters alone: no collector counts.
func (c *Tally) count(key ctaKey) (int, bool) {
	if c == nil {
		return 0, false
	}
	for _, counter := range c.counters {
		if counter.key().same(key) {
			return counter.count(), true
		}
	}
	return 0, false
}

// fits reports whether c holds exactly one counter for each of keys.counted and
// one collector for each of keys.collected, each in that order — which a Tally
// [AssertionTest.Tally] made from the test's keys holds until a report breaks
// the order [Tally] obliges — and is nil where both are empty, since a test
// that neither counts nor collects has no Tally and any non-nil c given to it
// is a breach.
func (c *Tally) fits(keys ctaKeys) bool {
	if len(keys.counted) == 0 && len(keys.collected) == 0 {
		return c == nil
	}
	if c == nil || len(c.counters) != len(keys.counted) || len(c.collectors) != len(keys.collected) {
		return false
	}
	for i, counter := range c.counters {
		if !counter.key().same(keys.counted[i]) {
			return false
		}
	}
	for i, collector := range c.collectors {
		if !collector.key().same(keys.collected[i]) {
			return false
		}
	}
	return true
}

// AssertionTest is a compiled assertion {test}: the expression tree
// [CompileAssertionTest] admitted for one element's attribute and child
// element types. It is a distinct type from [CTATest] so a tree typed for an
// assertion can never be evaluated over a Type Alternative's [Attributes],
// nor a Type Alternative tree over [TypedAttributes].
//
// The zero AssertionTest, which [CompileAssertionTest] returns with ok false,
// holds no {test} and is safe to use: [AssertionTest.ReadsContextItem] is
// false, [AssertionTest.ReadsChild] is false for every name,
// [AssertionTest.Tally] is nil, and [AssertionTest.Evaluate] answers false for
// every non-nil e.
type AssertionTest struct {
	root             ctaExpr
	readsContextItem bool
}

// CompileAssertionTest compiles an assertion's {test} (§3.13.1, an
// [xsd.XPathExpression] property record) for the element whose ·governing
// type definition· has the {content type} content, whose attribute types
// attrs answers and the types of whose child elements, and of the elements
// below them, elems answers by path, reporting ok false for a {test} this
// engine cannot evaluate. Its consumer is validate's cvc-assertion site
// (validate/cvcassertion.go), which declines the assertion on ok false.
//
// content fixes `$value`'s static type, which is what cvc-assertion clause
// 2.3.1.3 reads: under an [xsd.SimpleContent] it is the {simple type
// definition}, whose value [BindValue] binds at evaluation; under every other
// {content type} it is the empty sequence (clause 2.3.2), whatever the binding.
// A nil content declines every {test} naming `$value`. content also decides
// `.`: under every {content type} it is E's string value, one xs:untypedAtomic
// value [BindValue] binds beside `$value`, whatever the {simple type
// definition}'s variety, and a nil content declines it. Whether the compiled
// {test} reads `.` is [AssertionTest.ReadsContextItem]'s answer. `.` as the
// node fn:namespace-uri and fn:in-scope-prefixes take is E itself, which
// [AssertionTest.Evaluate] is handed, under every content, a nil one included.
//
// The grammar is [CompileCTATest]'s with the value comparisons, `$value`, an abbreviated
// child-axis step, an attribute step behind `./`, `./@a` or `./attribute::a`, which steps
// from `.`, E, and is E's attribute `@a` (xpath20.md §3.2, §3.2.4), a "/" or "//" opening
// one child or attribute step, a relative path of two or more abbreviated child-axis steps
// with QName NameTests, `a/b`, the last of which may instead be the wildcard `*`, `a/*`,
// which selects every element child (§3.2.1.2), one element step, `a`, `./a` or `.//a`, one
// step whose NodeTest is [58] CommentTest, `comment()`, `./comment()` or `.//comment()`, or
// a child step filtered by one predicate that is a conjunction of child steps with QName
// NameTests, `a[b and c]`, which selects each child a that has a child of each name
// (§3.2.2), or by one predicate that is one `preceding::` step with a QName NameTest,
// unfiltered or filtered by one QName child step or fn:not over one, `a[preceding::a]`,
// `a[preceding::a[b]]` or `a[preceding::a[not(b)]]`, which selects each child a with an
// element so named and so filtered before it in E's subtree, at any depth, E never among
// them (§3.2.1.1, cvc-assertion clause 1.3), standing as the whole operand of fn:exists,
// fn:empty or an ·effective boolean value· (xpath20.md §3.2, §3.2.4, §2.4.3), an fn:count
// call, whose argument may filter a child step by one predicate or join operands with `|`
// or `union` (§3.2.2, §3.3.3; the bullets below bound both), or be an operand that is no
// path — `$value`, `()`, a literal or a function call, read as any argument of the
// functions below is — the binary arithmetic operators `+`, `-`, `*`, `div`, `idiv` and
// `mod` (xpath20.md §3.4), and a call to one of the F&O string, sequence and date functions —
// fn:contains, fn:starts-with and fn:ends-with with two arguments, fn:string-length,
// fn:normalize-space and fn:string with one or none, the implicit argument being `.`,
// fn:concat with two or more, each cast to xs:string, fn:substring-after with two, under
// the codepoint collation, fn:number with one or none, the implicit argument being `.`, its
// one item cast to xs:double and NaN over none or where that cast fails, fn:empty,
// fn:exists and fn:distinct-values with one, fn:true, fn:false and fn:current-date with
// none, fn:year-from-date with one, the year of its xs:date's ·localized value·, an
// xs:untypedAtomic argument cast to xs:date and one of any other type err:XPTY0004, and
// fn:doc with one that is a StringLiteral or statically empty, either followed by a relative path of
// QName child or attribute steps (xpath-functions.md §7.5.1–7.5.3, §7.4.4, §7.4.5, §2.3, §7.4.1,
// §7.5.5, §14.4, §15.1.4, §15.1.5, §15.1.6, §9.1.1, §9.1.2, §16.4, §10.5.14, §15.5.4), any argument
// of which may be the empty sequence `()`, and any of which a constructor function may take as its
// operand, as it may any other argument (xpath20.md §3.1.5), and fn:sum with one argument that is a
// relative path of one or more child steps and one attribute step, each with a QName NameTest,
// `sum(a/b/@p)` (§15.4.5, xpath20.md §3.2), the attribute typed as attrs answers for the path — over
// the typed values of the attributes it selects below E in document order, xs:untypedAtomic ones cast
// to xs:double, the xs:integer 0 over none, the one item over one, and over more their sum in their
// numeric type (§6.2), exact over xs:decimal ones, err:FORG0006 over items whose type is not numeric,
// and an `instance of` over the call reading the type of the value it returns rather than its static
// type — and fn:namespace-uri with `.` or no argument (§14.3), E's [namespace name] as one xs:anyURI,
// the zero-length one for no namespace, and fn:in-scope-prefixes with `.` (§11.2.6) as one operand of
// `=` whose other is a StringLiteral or a parenthesized comma sequence of them, on either side, true
// where one of those is a prefix of E's [in-scope namespaces] (`xml` always, the zero-length string
// where a default namespace is in scope, never `xmlns`), each `.` there E as a node — the conditional
// `if (Expr) then ExprSingle else ExprSingle` (xpath20.md §3.8) as the whole {test}, inside
// parentheses, as fn:not's argument or as an operand of another, whose test's ·effective boolean
// value· selects the one branch evaluated, so a dynamic error in the other is never raised, and both
// of whose branches are compiled, so a decline in either declines the {test}; the [47]
// ContextItemExpr `.`, atomized (§3.1.4, §2.4.2); [18] CastableExpr's `castable as` tail over a [16]
// ta-SimpleValue, `$value castable as xs:double`, an xs:boolean true exactly where the same `cast as`
// yields a value and false where that cast raises — the empty sequence without `?`, two or more
// items, a lexical or facet mismatch — while an error evaluating its operand raises (§3.10.3), and
// which declines wherever that cast does; [16] InstanceofExpr's `instance of` tail with an atomic
// SequenceType, a builtin [53] AtomicType or xs:untypedAtomic and an optional `?`, `*` or `+`
// (§3.10.1), over an operand that is no node or over an fn:data call, `data(@d) instance of
// xs:date*`, whose argument it atomizes (xpath-functions.md §2.4) — a relative path of two or more
// child steps with QName NameTests among them, `data(a/b) instance of xs:date*`, its last step typed
// as elems answers for the path on a child's terms, over the typed values of the elements it selects
// below E in document order, a ·nilled· one contributing none, and one typed child step, its axis
// abbreviated or spelled out, `data(child::d)`, filtered by one IntegerLiteral predicate or by none,
// `data(d[1]) instance of xs:date`, the k-th d child in document order or none (§3.2.2, §3.2.4) —
// true where the item count is one the indicator admits and each item's type derives from the
// AtomicType (§2.5.4), never casting, so `data(.) instance of xs:untypedAtomic` holds over every
// {content type} and `$value instance of xs:date` over an xs:date `$value`, while an error evaluating
// its operand raises; the same tail with an [60] AttributeTest, `attribute()`, `attribute(N)`,
// `attribute(*, T)` or `attribute(N, T)` for T a builtin simple type or xs:untypedAtomic, over one
// attribute step of E, `@d`, `attribute::d` or `./@d`, true where the count of attributes it selects
// is one the indicator admits and the attribute is named N and annotated with the type attrs answers
// for it at the empty path, deriving from T (§2.5.4.5), so assert013's `./@event instance of
// attribute(*, xs:dateTime)` holds over an xs:dateTime @event; [6] QuantifiedExpr (xpath20.md §3.9)
// wherever the conditional stands, `some` or `every` over one in-clause binding a range variable `$v`
// to each child of E a step `N` or `./N` with a QName NameTest selects, whose body is `$v/@A`, a
// conjunction `$v/@A and $v/@B …`, or `$v/following-sibling::*[1][self::M]` — the next sibling
// element of `$v` where it is named M, empty after E's last child element (§3.2.1.1, §3.2.2) — or one
// of those inside fn:not, or binding `$v` to each child such a step selects whose type elems answers
// is one a child read for its value is admitted under as a TYPED value, or to each but the last of
// them under the one predicate `[position() lt last()]` (§3.2.2), whose body is any one ExprSingle
// this function admits, `$v` in scope in it alone as a node, ·nilled· or not, whose typed value is
// that child's, and `$v/following-sibling::M[1]` the first later child named M however many of other
// names lie between, or none (§3.2.1.1), typed on the same terms, and `$v/@A` and
// `$v/following-sibling::M[1]/@A` the attribute A of that child, or none where it has no A, typed as
// attrs answers for A at the one-step path of the child's name — never ·special· — either one ending
// in a function call, `$v/@A/F(…)`, evaluated once with the attribute node as its context item `.`,
// or the empty sequence where there is no such node (xpath20.md §3.2, [27] StepExpr's FilterExpr
// arm), or binding a range variable `$x` to each item of a typed `$value`, atomic or list, written
// `data($value)` or `$value` (xpath-functions.md §2.4), whose body is any one ExprSingle this
// function admits, the range variable in scope in it alone as an atomic value of `$value`'s item
// type, `some` true where a binding satisfies the body and `every` where all do, so over no binding —
// an empty list, or the nil `$value` clause 2.3.2 binds — `some` is false and `every` true, and over
// `$value` or a typed child step a binding whose body raises is the answer unless another binding
// decides the quantifier (§3.6's tables); and, as an operand of a general comparison, an integer
// sequence: [11] RangeExpr `I to J` over two IntegerLiterals, bare or parenthesized, or a
// parenthesized comma sequence of IntegerLiterals and such ranges, `(1 to 10, 20, 30)` (§3.3.1), or a
// string sequence, a parenthesized comma sequence of StringLiterals, `('a', 'b')`. A string
// sequence's items are xs:string. Against a typed operand outside the xs:string family, an xs:integer
// `$value` or child among them, xpath20.md §B.2 defines no comparison and it raises err:XPTY0004,
// while `.`, which is not typed, casts to xs:string (§3.5.2 rule 2.4) — added, and every decline
// [CompileCTATest] states is this one's too, under the same static context (xpath-valid clause 2.2)
// augmented with `$value` (cvc-assertion clause 2.2), plus these, each of which is the same withhold:
//
//   - an attribute NameTest that is not a QName: a [37] Wildcard can match an
//     attribute ·attributed to· an {attribute wildcard}, whose type is not
//     fixed at compile time;
//   - a QName for which attrs reports false;
//   - an attribute whose type is not ·special· and does not atomize to one
//     atomic value of a type known at compile time — a list or union {variety}
//     — or whose {primitive type definition} is xs:NOTATION
//     (ctaTypes.typedAtomic). The variety is classified here, not trusted to
//     attrs. An attribute attrs annotates ·special· is read as
//     xs:untypedAtomic and is admitted;
//   - a `$value` over an [xsd.SimpleContent] whose {simple type definition} the
//     bullet above declines as an attribute's type, unless it is a list whose
//     {item type definition} that bullet admits — so a union, a list of a
//     union, and an xs:NOTATION primitive or item type decline. A
//     ·special· one is read as xs:untypedAtomic and is admitted;
//   - any variable but `$value` and, inside a quantifier's body, its range
//     variable, which is not in the static context at all (err:XPST0008) — so
//     `$v` outside the body declines;
//   - a quantified expression of any other shape: a range variable named
//     `$value`, which would shadow clause 2.3's; two or more in-clauses; a
//     binding sequence that is not one QName child step `N` or `./N` — `*`,
//     `a/b`, `.//N`, `@x`, `.` or a predicate but `[position() lt last()]`
//     — `<`, `ne` or `!=` for `lt`, `[last()]`, `[1]`, a second predicate —
//     nor `data($value)` or `$value` over a typed `$value`, so a `$value`
//     under a {content type} that is not simple or over a ·special· one
//     declines as a binding; a nested quantifier in any body; over `$value`, a
//     path over the range variable, `$x/@a`, and `instance of` over it; over a
//     child step whose body is none of the forms read off the [Tally], a
//     child step N a child read for its value would decline, or of mixed
//     content, a path over `$v` but `$v/following-sibling::M[1]`, an
//     attribute step over either and a function call after that —
//     `$v/following-sibling::M`, `[2]`, a wildcard M, `preceding-sibling::`,
//     `$v/@*`, an attribute attrs answers no type, a ·special· one or one of
//     no fixed atomic type for, a call after it that carries no type, reads
//     any node but `.` or the focus, and any step after that call — and such
//     an M a child read for its value would decline, or of mixed content; over
//     `N[position() lt last()]`, a body read off the [Tally]; over a child
//     step whose body opens as one read off the [Tally] does, `$v/@` or
//     `$v/following-sibling::*`, a body of any other form — `or`,
//     `$v/following-sibling::*[self::M][1]`, which selects the first M sibling
//     however far, a position other than the IntegerLiteral `1`, `[last()]`,
//     `[position() = 1]`, either predicate alone, a wildcard or kind test on
//     either step — a body followed by `and` or `or` inside the same
//     ExprSingle, and a quantifier anywhere but where the conditional stands;
//   - a child-axis step whose NameTest is not a QName — but the `*` ending a
//     child path whose existence is asked — or spells its axis out but as
//     fn:data's whole argument under `instance of`, and, where
//     the step's value is read — anywhere but as the whole operand of
//     fn:exists, fn:empty or an ·effective boolean value· — one naming a child
//     for which elems reports false;
//   - a child read for its value whose type elems answers is none of: a simple
//     type the bullets above admit as an attribute's, a complex type whose
//     simple {content type} is one, or a complex type whose {content
//     type}.{variety} is mixed — xs:anyType among them — which is read as
//     xs:untypedAtomic. A ·special· simple type declines, because an xsi:type
//     can give the child a typed value where the compile read an
//     xs:untypedAtomic one; a mixed one does not, because the caller withholds
//     a child whose own type has no mixed content ([ChildElements]). Every
//     element-only and empty {content type} declines;
//   - a path of child steps read as fn:data's argument whose last step elems
//     answers false for the path, or answers a type a child read for its value
//     would decline as above, or one of mixed content, whose string-value the
//     caller collects only for a child of E, or whose last step is `*`;
//   - a path of more than one step anywhere but as the whole operand of
//     fn:exists, fn:empty or an ·effective boolean value· or, ending in an
//     attribute step, as fn:sum's argument, or, of child steps alone, as
//     fn:data's argument under `instance of`, and there any step
//     on another axis, with a wildcard but a bare `*` on the last step, with a
//     predicate or a kind test, and any "//" between steps — so `a/b = 1`,
//     `count(a/b)`, `string(a/b)`, `a/@b`, `a//b`, `a/* = 1`, `a/*/b`,
//     `a/*:b` and `a/p:*` decline — a rooted path of more than one step, and a
//     "/" with no step after it;
//   - a call to fn:sum with two arguments, `sum(a/b/@p, 0)`, or over any
//     other argument — `sum(@p)`, `sum(c)`, `sum(a/c)`, `sum(a/b/@*)`,
//     `sum(a//b/@p)`, `sum($value)` — over attributes of no fixed type, of a
//     list or union type or of a type whose {primitive type definition} is a
//     duration, and one whose items are xs:float, xs:double or
//     xs:untypedAtomic, whose empty value, the xs:integer 0, is not of its
//     static type, bare or under fn:distinct-values, as an arithmetic
//     operand, or compared with an operand typed with an xs:decimal primitive
//     but a DecimalLiteral or IntegerLiteral that is zero exactly where its
//     rounding to that type is; and a call to fn:avg, fn:min or fn:max;
//   - an fn:count argument that is not one QName step — `N`, `@N`, either of
//     them behind `./` or `.//`, or a rooted one — nor such a child step `N`
//     or `./N` filtered by one predicate the next two bullets admit, nor a `|`
//     or `union` of relative such operands, nor an operand that opens as no
//     path does — `$`, `(`, a literal, or a name followed by `(` — and that a
//     function argument admits — so a wildcard, a longer path, a bare `.`, a
//     union with `$value`, a predicate on any other step, a second predicate,
//     and a union operand that is rooted or filtered by its value decline, and
//     so does a step behind `./` or `.//` anywhere but in an fn:count call or
//     as the whole operand of fn:exists, fn:empty or an ·effective boolean
//     value· — but an attribute step behind `./`, `./@N`, which is `@N`
//     wherever an attribute step stands — and a predicate or a union anywhere
//     but in an fn:count argument, but the child-existence and `preceding::`
//     predicates the next bullet admits;
//   - outside an fn:count argument, a predicate on any step but one child
//     step standing as the whole operand of fn:exists, fn:empty or an
//     ·effective boolean value·, or as fn:data's whole argument under
//     `instance of`; in the first, one that is neither a conjunction
//     of child steps with QName NameTests, `N[a]` or `N[a1 and a2 …]`, nor one
//     `preceding::` step with a QName NameTest, `N[preceding::M]`, filtered by
//     at most one QName child step or fn:not over one, `N[preceding::M[P]]` or
//     `N[preceding::M[not(P)]]` — so `N[a or b]`, `N[not(a)]`, `N[a/b]`,
//     `N[*]`, `N[@a and b]`, `N[a][b]`, `N[a]/b`, `N[a] = 1`, `N[1]`, every
//     other axis — `N[ancestor::M]`, `N[preceding-sibling::M]`,
//     `N[following::M]` — `N[preceding::*]`, `N[preceding::M[@P]]`,
//     `N[preceding::M[not(P) and Q]]`, `N[preceding::M[P][Q]]` and
//     `count(N[preceding::M])` decline; in the second, one that is no
//     IntegerLiteral, `data(N[last()])` or `data(N[1.0])`, or that filters a
//     child of mixed content;
//   - in an fn:count argument, a predicate that is not a conjunction of
//     attribute-existence tests with QName NameTests, `N[@A]` or `N[@A1 and
//     @A2 …]`, nor a comparison, or and, or and fn:not over comparisons, of the
//     candidate `.`, literals, casts and arithmetic — so `count(N[a])`,
//     `N[@a or @b]`, `N[not(@a)]`, `N[@*]`, an attribute atomized or beside
//     `.`, any other node, `$value`, fn:count and every other function call
//     decline — and a predicate whose root is a bare value, `N[1]`, `N[1 + 0]`
//     or `N[.]`, which a numeric value would make positional (§3.2.2);
//     position() and last() decline here, the predicate calling no library
//     function (ctaPredicateFacade.callsLibrary), and outside a predicate too
//     (ctaAssertionFacade.focus);
//   - a predicate over a child whose type elems answers is not one a child
//     read for its value is admitted under as a TYPED value, on that bullet's
//     terms — so a predicate over a mixed child, `count(body[. = 'x'])`,
//     declines — unless the predicate tests attribute existence alone;
//   - a cast whose operand is a typed attribute, a typed child, `$value` or,
//     under the constructor spelling, a typed count, arithmetic or function
//     result outside the xs:string family, to a target that operand's type is
//     neither nor derived from by restriction (F&O §17.4, §17.1, §17.5) — so
//     `xs:integer(@d)` over an xs:decimal @d declines and `xs:decimal(@i)`
//     over an xs:int @i, §17.3's cast, does not, nor does a cast of an
//     xs:date, xs:dateTime or xs:time operand to xs:string itself, §17.1.2's
//     local value, while one to xs:token declines — and a cast of an xs:float
//     or xs:double operand, a DoubleLiteral or (as fn:string's argument) a
//     cast to either, to any target but its own type (F&O §17.1.2, §17.1.3
//     and §17.1.6 over the value where §17.1's casting table marks the target
//     Y or M, §17.1's err:XPTY0004 where it marks N), so `xs:string(1.5e0)`
//     and `xs:date(1.5e0)` decline and `xs:double(1.5e0)` does not — and a
//     cast of an IntegerLiteral or a DecimalLiteral to any target but one
//     whose primitive is xs:string, xs:float or xs:double, xs:decimal, and for
//     an IntegerLiteral xs:integer's branch (F&O §17.1.3.4 and §17.1.6 over
//     the value, §17.1's err:XPTY0004), so `xs:integer(1.5)`,
//     `xs:boolean(2)` and `xs:anyURI(1.5)` decline and `xs:integer(2)` and
//     `xs:string(1.5)` do not;
//   - an arithmetic operand whose {primitive type definition} is not
//     xs:decimal, xs:float or xs:double — so the duration and date/time
//     arithmetic B.2 defines declines, and so does the err:XPTY0004 of any
//     other type — and an xs:float operand against an xs:double or
//     xs:untypedAtomic one, which needs B.1 rule 1.1 (#889); a unary `+` or
//     `-`, and a parenthesized operand, which [11]'s `(` arm reads as a
//     boolean expression;
//   - a conditional anywhere but in the positions listed above — so a
//     function's argument, `contains(if (@a) then 'x' else 'y', 'x')`, and
//     a parenthesized comparison or arithmetic operand,
//     `(if (@a) then 1 else 2) = 1`, decline — and a conditional inside a
//     predicate;
//   - a call to fn:contains, fn:starts-with, fn:ends-with or
//     fn:substring-after with a third, collation argument, or to
//     fn:distinct-values with a second (§7.3.1), which is never read as
//     the form without it, and a call to any of the functions above with
//     an arity it does not have (err:XPST0017);
//   - `.` under a nil content, but as the argument of fn:namespace-uri or
//     fn:in-scope-prefixes, and `.` as the whole operand of an ·effective
//     boolean value·, fn:not over one, fn:exists, fn:empty or fn:count, where it
//     is a node and not the atom it is read as elsewhere — so `.`, `not(.)`,
//     `exists(.)` and `count(.)` decline — and fn:string-length,
//     fn:normalize-space and fn:string with no argument wherever `.` declines;
//   - fn:namespace-uri with any argument but `.` — a path, a variable, `()`,
//     `(.)` — and fn:in-scope-prefixes over any argument but `.`, or anywhere
//     but as one operand of `=` against a StringLiteral or a parenthesized
//     comma sequence of them — so `in-scope-prefixes(.)`,
//     `count(in-scope-prefixes(.))`, `in-scope-prefixes(.) != 'a'`,
//     `in-scope-prefixes(.) eq 'a'` and `in-scope-prefixes(.) = @a` decline;
//   - fn:doc over any argument but a StringLiteral or a statically empty one
//     — `doc(@a)`, `doc($value)` over simple content, `doc(1)` — and with a
//     path after it holding a wildcard, an axis spelled out, a predicate or a
//     "/" with no step after it, `doc('a')/*`, `doc('a')[1]`;
//   - an integer or string sequence anywhere but as a general comparison's
//     operand — `. eq (1 to 3)`, `(1 to 3) + 1`, `exists(('a', 'b'))` — a
//     range operand that is not an IntegerLiteral, `1 to .`, a member that is
//     none of an IntegerLiteral, such a range and a StringLiteral, a sequence
//     mixing StringLiterals with the other two, `('a', 1)`, a nested or empty
//     parenthesis, a misplaced comma, `('a',)`, an IntegerLiteral beyond
//     int64, and a sequence of more than 4096 items;
//   - fn:number over a typed operand outside the xs:string family whose
//     type is neither xs:double nor derived from it — an xs:decimal, an
//     xs:integer, an xs:boolean or an xs:date operand — whose cast to
//     xs:double the cast bullet above declines;
//   - fn:string, and an fn:concat argument, over a typed attribute, a typed
//     child, an fn:count call, an arithmetic result, `$value`, a function
//     result or a cast of one of them outside the xs:string family and the
//     xs:date, xs:dateTime and xs:time primitives, which is the cast to
//     xs:string the bullet above declines, and fn:string over any argument
//     whose {primitive type definition} is xs:float or xs:double, a literal
//     or a cast included, which is the cast to xs:string that bullet's
//     floating clause declines;
//   - a `castable as` tail over any operand but a [16] ta-SimpleValue — so
//     `@d cast as xs:string castable as xs:date` and `xs:string(@d) castable
//     as xs:date` decline — and one inside a value predicate, and every
//     `castable as T` whose `cast as T` the bullets above decline;
//   - an `instance of` tail whose SequenceType is not atomic — a KindTest
//     such as `element()`, `item()`, `empty-sequence()`, and an AttributeTest
//     over anything but one attribute step, `data(@d)` among them, or whose
//     TypeName is user-defined (err:XPST0008), xs:anyType, or xs:anyAtomicType
//     over an attribute of ·special· type — or whose AtomicType is
//     user-defined or no atomic type at all (a list, xs:anySimpleType,
//     err:XPST0051), one over a node, `@d instance of xs:date`, or over a
//     numeric literal or an arithmetic result, whose static type here is not
//     its dynamic one, one over a typed child, or fn:distinct-values over one,
//     whose AtomicType the child's ·locally declared type· does not derive
//     from — `data(e) instance of xs:token` over an xs:string e, which an
//     xsi:type can make an xs:token — and so over a path of child steps read
//     for its values, `data(a/b)`, against the type elems answered for it, one
//     inside a value predicate, and fn:data anywhere but as the operand of
//     `instance of` or as a quantifier's binding over `$value`.
//
// An xs:string? argument — of every function above but fn:empty, fn:exists,
// fn:distinct-values, fn:string, fn:concat and fn:number — of any type outside the
// xs:string and xs:anyURI families is not a decline: xpath20.md §3.1.5's
// function conversion raises err:XPTY0004 for each item it yields, an absent
// attribute's empty sequence being the zero-length string, and so does a
// `$value` of two or more items.
//
// A counted step consults neither attrs nor elems: fn:count does not atomize
// its argument (xpath-functions.md §15.4.1, `$arg as item()*`), so the step's
// type decides nothing and a node of any type, or matched by a wildcard,
// counts. Nor does a child path of two or more steps, `*` ending it or not, one
// element step, or a child step filtered by the existence of its children or
// by a `preceding::` step, as the whole operand of fn:exists, fn:empty or an
// ·effective boolean value·, on the same terms: fn:exists and fn:empty take
// `item()*` (§15.1.4, §15.1.5) and an ·effective boolean value· asks only
// whether the first item is a node (xpath20.md §2.4.3 rule 2), so a node of any
// type, ·nilled· or not, is selected — `a` over an element-only a is decided
// where `a = 1` declines. Nor does an attribute-existence predicate, whose
// ·effective boolean value· asks only whether the attribute node exists (§2.4.3
// rule 2), so `c[@a]` counts a c carrying an empty a, and neither does a
// quantified expression over a child step whose body is read off the [Tally],
// whose binding step is never atomized and whose body asks only whether its
// step selects a node, so a child of any type is bound and `$v/@a` holds for a
// `$v` carrying an empty a. One whose body is any other form binds typed
// children off [ChildElements], on a value step's terms. What the {test} counts
// is read off the [Tally] its evaluation carries ([AssertionTest.Tally]). A
// predicate reading `.` is the one counted argument elems types: it atomizes
// each candidate child, typed as a child step naming it reads a TYPED value,
// and counts over [ChildElements]; a dynamic or type error over any candidate
// raises for the whole count. An argument that is no path is counted off its
// own items, never the [Tally], on fn:exists' terms: `count($value)` is the
// number of items of `$value` — one for an atomic or ·special· value, each item
// of a list (Datatypes dt-xdmrep clause 3), and none for an empty list or the
// empty sequence clause 2.3.2 binds.
//
// fn:distinct-values atomizes its argument and drops each item eq to an
// earlier one (xpath-functions.md §15.1.6), compared in the items' {primitive
// type definition}: under the codepoint collation, so "a" and "A" are two; at
// the implicit timezone, Z, for a date or time without one; with 0 and -0 one
// value and NaN one item although `NaN eq NaN` is false; an xs:untypedAtomic
// item as xs:string; and two items eq does not relate as two, never an error.
// Which of two equal items survives is the first.
//
// An XPath STATIC error is declined too and never reported: the
// static-error question about an assertion is the schema assembler's, and
// [CTATestStaticError] answers it for a Type Alternative only.
//
// A leading "/" or "//" followed by one child or attribute step is not a
// decline, inside an fn:count call or outside one: E is the root of the data
// model instance, with no document node above it (cvc-assertion clause 1.3),
// so the path raises err:XPDY0050 (xpath20.md §3.2) before it selects a node,
// and the {test} is false whatever E is named and whatever its subtree holds.
//
// Nor is a call to fn:doc whose argument is a StringLiteral: cvc-xpath clause
// 8 makes the available documents the empty set, so the call raises
// err:FODC0005 (xpath-functions.md §15.5.4) and the {test} is false whatever E
// carries. Over a statically empty argument, `doc(())`, the call is the empty
// sequence. A path after the call, `doc('a.xml')//event/@name`, steps from a
// call that raises first or from the empty sequence, so no step of it is
// evaluated and no document node is built: its names are resolved, an unbound
// prefix declining, and discarded.
//
// Nor is one step whose NodeTest is xpath20.md [58] CommentTest, a [54]
// KindTest — `comment()`, `./comment()` or `.//comment()` — as the whole
// operand of fn:exists, fn:empty or an ·effective boolean value·: cvc-assertion
// clause 1.2 excludes comments from the partial ·PSVI· clause 1.3 builds the
// data model instance from, retaining them only ·at user option·, which this
// engine offers none of, so the step selects the empty sequence whatever E
// carries, `empty(.//comment())` is true and `exists(.//comment())` false.
// Anywhere else — under fn:count, behind an axis spelled out,
// `child::comment()`, after a child step, in a predicate — it declines, under
// the GAP(xpath) below, and so does every other KindTest, [59] PITest
// `processing-instruction()` among them, which clause 1.2 excludes on the same
// terms but whose argument, an NCName or a StringLiteral that raises
// err:XPTY0004 where its normalized value is no NCName (xpath20.md §3.2.1.2),
// this grammar does not parse. A Type Alternative's {test} declines the step
// in every position, and an assertions facet's, which has no context item to
// step from, raises err:XPDY0002 over it and fails ([FacetAssertions]).
//
// GAP(xpath): unlike a Type Alternative's, an assertion's {test} has no
// required subset to stop at — §3.13 admits full XPath 2.0 — so every decline
// above is this engine's limit and not the spec's license: paths of more than
// one step outside fn:exists, fn:empty, an ·effective boolean value·, fn:sum's
// argument and fn:data's under `instance of`, axes beyond the attribute step,
// one child step, the child steps of such a path, the one element step whose
// existence is asked, the one counted step, the preceding step and its child
// step in a predicate on a child step whose existence is asked — `ancestor::`
// and `preceding-sibling::` among the axes beyond them — the following-sibling
// and self steps of a quantifier's sibling body, and the
// `following-sibling::M[1]` step over a range variable bound to a typed child
// and the attribute step over it or over that step, quantified expressions
// beyond one in-clause over a child step with the two body forms read off the
// [Tally], over a typed child step, filtered by `[position() lt last()]` or
// not, whose body reads its bound children, or over a typed `$value`, wildcards
// but a `*` ending a child path whose existence is asked, KindTests but the
// comment step whose existence is asked, predicates beyond the two kinds on a
// counted child step and the child-existence conjunction and the `preceding::`
// step on a child step whose existence is asked and the IntegerLiteral one on a
// child step fn:data reads under `instance of`, positional ones among them,
// unions outside fn:count or over other operands, children read for their value
// whose type is element-only, empty or ·special·, elements a path of child
// steps reads for their value whose type is mixed, a value predicate over a
// mixed child, arithmetic outside the numeric operands and the binary
// operators, conditionals whose value is read as an item rather than for its
// ·effective boolean value·, `.` under a nil content or where it is a node, but
// as the argument of the two functions below, sequence expressions beyond the
// integer and string sequences of a general comparison's operand, `castable as`
// over any operand but a [16] ta-SimpleValue or inside a value predicate,
// `instance of` beyond an atomic SequenceType over the operands above, the
// collation argument, fn:sum with two arguments, over any argument but a path
// to an attribute below E, over duration items, or over floating items, bare or
// under fn:distinct-values, as an arithmetic operand or against an xs:decimal
// operand, fn:avg, fn:min and fn:max, and every F&O function but fn:count,
// those listed above, fn:namespace-uri over `.` or with no argument, and
// fn:in-scope-prefixes over `.` only as an operand of `=`, among them —
// fn:in-scope-prefixes elsewhere because its sequence needs a listing of E's
// [in-scope namespaces], a capability validate.Element lacks, [ContextElement]
// answering one prefix at a time. The direction is the withhold: the caller
// records the assertion as unevaluated and neither charges it nor shows it
// satisfied (PRINCIPLES 20). (#1042)
//
// types is read as [CompileCTATest] reads it and stored nowhere.
func CompileAssertionTest(expr xsd.XPathExpression, types xsd.TypeResolver, content xsd.ContentType, attrs AttributeTypes, elems ElementTypes) (AssertionTest, bool) {
	var readsContextItem bool
	root, defect := compileCTATest(expr, types, ctaAssertionFacade{content: content, attrs: attrs, elems: elems, readsContextItem: &readsContextItem})
	if defect.kind != ctaNoDefect {
		return AssertionTest{}, false
	}
	return AssertionTest{root: root, readsContextItem: readsContextItem}, true
}

// ReadsContextItem reports whether the compiled {test} reads E's string value,
// which the [47] ContextItemExpr `.` atomizes to and [BindValue]'s text
// supplies: true wherever the compile admitted `.`, written or implicit — the
// zero-argument fn:string, fn:string-length and fn:normalize-space read it
// (xpath-functions.md §2.3, §7.4.4, §7.4.5) — and false for a {test} reading
// only `$value`, attributes or children, whose evaluation reads no text of
// the [ValueBinding]. A `.` inside an fn:count argument's predicate is the
// candidate child and no read here; that child's value is
// [AssertionTest.ReadsChild]'s. Nor is the `.` fn:namespace-uri or
// fn:in-scope-prefixes takes, which is E as a node and reads E's name and
// bindings ([ContextElement]), never its string value, so a {test} whose only
// `.` is such an argument reports false.
//
// The answer is recorded as the compile admits each `.`, not read back off the
// tree, so it may over-report — a `.` admitted on a parse the compile then
// abandoned — and never under-reports: a {test} for which it is false
// evaluates to the same answer over every binding's text. Its consumer is
// validate's walk, which gathers E's string value under a {content type} that
// is not simple only for an element one of whose {test}s reads it, and binds
// the zero-length string otherwise.
func (t AssertionTest) ReadsContextItem() bool { return t.readsContextItem }

// Evaluate reports whether the compiled {test} evaluates to true for the
// element whose attributes attrs yields, whose element [[children]] children
// yields, whose subtree counts has counted and whose string value and `$value`
// v binds ([ValueBinding]), WITHOUT raising a dynamic or type error — the
// whole of what cvc-assertion (§3.13.4.1) asks: "An element information item E
// is locally ·valid· with respect to an assertion if and only if the {test}
// evaluates to true (see below) without raising any dynamic error or type
// error." Clause 3 converts the result "as if by a call to the XPath
// fn:boolean function", which a boolean-rooted tree already is.
//
// So false is ONE answer for two outcomes the caller treats alike — the {test}
// was false, or it raised (err:FORG0001, err:XPTY0004, which a function
// argument's conversion or cardinality raises too, err:FORG0006,
// err:XPDY0050, err:FODC0005 for an fn:doc call, err:FOAR0001 for a `div`, `idiv`
// or `mod` by zero over xs:decimal or xs:integer operands and for any `idiv` by
// zero, err:FOAR0002 for an `idiv` over a NaN operand or an infinite dividend or
// whose quotient overflows to an infinity) — and either way E is not ·valid· with
// respect to the assertion. A processor that raises a type error dynamically
// "will treat the expression as having evaluated to false" (cvc-xpath,
// §3.13.4.2). Every decline happened at [CompileAssertionTest].
//
// The bool carries no diagnostic; validate makes the one its caller reads. It
// charges a false answer as an entry of validate's Result.Violations() under
// cvc-assertion, at E's location. An assertion it does not evaluate — one whose
// {test} [CompileAssertionTest] declined, or one validate itself declines —
// never reaches Evaluate, and is an entry of Result.Unevaluated() under
// cvc-assertion instead.
//
// The dynamic context is cvc-xpath's — context item E, position and size 1 —
// with the one variable cvc-assertion clause 2.3 adds to it, `$value`, bound
// to v's typed value; `.` atomizes to v's string value. A tree compiled for a
// {content type} that is not simple never reads v's typed value, `$value`
// being the empty sequence there. b and types are read as [CTATest.Evaluate]
// reads them.
//
// now is the dynamic context's current dateTime (xpath20.md §2.1.2), which
// fn:current-date reads as the xs:date of now in now's own UTC offset
// (xpath-functions.md §16.4, §17.1.5); the implicit timezone stays Z whatever
// now's offset is. An offset no timezoneFrag can spell — not a whole number of
// minutes, or outside -14:00 to +14:00 (xmlschema11-2 timezoneFrag) — is
// replaced by UTC: the date is that of now.UTC(), with timezone Z. cvc-xpath
// clause 6 makes the current dateTime constant during an assessment episode,
// so the caller hands every evaluation of one episode the same instant. Its
// consumer is validate, which reads the clock once per Validator.Assess and
// passes that instant here and to [FacetAssertions]. This package reads no
// clock: the zero [time.Time] is the instant 0001-01-01T00:00:00Z like any
// other and never stands for the current time, so fn:current-date over it is
// 0001-01-01Z.
//
// counts is the [Tally] t.Tally() made for this evaluation, filled with E's
// subtree on the terms that type states, and nil exactly where t.Tally() is
// nil. Any other counts — a nil one where t counts, a non-nil one where it
// counts nothing, or one made by a test counting other paths — breaks that
// obligation, on the terms [TypedAttributes] states for an attribute's value,
// and Evaluate answers false for it whatever the tree holds; so does a Tally
// still owing a [Tally.Attribute] call for its latest report, or a
// [Tally.ElementValue] call. A Tally made by another test counting the same
// paths cannot be told from t's own and is read as one; handing over the one
// filled for this E is the caller's part.
//
// e is E itself, the context item cvc-xpath clause 1 names, and must be
// non-nil: a nil e panics, whatever the {test} reads. It is read only by the
// two functions that take `.` as a NODE and not atomized: fn:namespace-uri,
// over `.` or with no argument, reads E's [namespace name] off e.Name, and
// fn:in-scope-prefixes, over `.` as an operand of `=`
// ([CompileAssertionTest]), asks e.LookupPrefix about each prefix the other
// operand names. The compiled tree holds no element, so one [AssertionTest]
// serves every E for which attrs gave the same answers.
func (t AssertionTest) Evaluate(b value.Backend, types xsd.TypeResolver, e ContextElement, attrs TypedAttributes, children ChildElements, counts *Tally, v ValueBinding, now time.Time) bool {
	if e == nil {
		panic("xpath: AssertionTest.Evaluate: nil ContextElement")
	}
	if !counts.fits(t.countedPaths()) || !counts.settled() {
		return false
	}
	in := ctaTypedInput{node: e, attrs: attrs, children: children, counts: counts, value: v, now: now}
	return ctaEval(t.root, ctaEnv{backend: b, types: types, input: in}) == ctaTrue
}

// Tally returns a fresh, empty [Tally] for one evaluation of t, holding one
// counter for each distinct relative path an fn:count call in t counts over — a
// step, a child step filtered by attribute existence, or a union, one counter
// serving `c[@a and @b]` and `c[@b and @a]`, and `e | e` and `e` — and each
// distinct path — one element step, a child path of two or more steps, its last
// step a QName or `*`, a child step filtered by its children's existence, one
// counter serving `N[a and b]` and `N[b and a]`, or one filtered by a
// `preceding::` step, `N[preceding::M[not(P)]]` — whose existence t asks
// (fn:exists, fn:empty, an ·effective boolean value·), one counter serving a
// path both counted and asked — and, for each quantified expression over a
// child step whose body is read off the Tally, the children satisfying its
// body, `N[@A]` or the N followed by an M sibling, and, under `every`, the
// children it binds, `N`, one counter serving `count(N)` and `every $v in N
// satisfies …` — and one collector for each distinct path to an attribute whose
// values t reads, fn:sum's argument `a/b/@p`, and for each distinct path of
// child steps whose elements' values t reads, fn:data's argument `a/b` — or nil
// exactly where t neither counts nor reads a value below E — a {test} with none
// of these, or one whose every fn:count argument is rooted and raises, is a
// child step filtered by its value, which [ChildElements] answers, or is no
// path and counts none, as `count($value)` counts the items of `$value`. t
// itself is not changed, so one compiled test serves any number of evaluations,
// each with its own Tally.
//
// Its consumer is validate's walk (validate/cvcassertion.go), which reads the
// nil as its gate: only an element one of whose {test}s has a Tally reports its
// subtree to one, and which children a value step reads stays
// [AssertionTest.ReadsChild]'s answer alone. The Tally's methods exist for
// package validate, and a caller outside the module is not expected to drive
// them.
func (t AssertionTest) Tally() *Tally {
	keys := t.countedPaths()
	if len(keys.counted) == 0 && len(keys.collected) == 0 {
		return nil
	}
	c := &Tally{counters: make([]ctaCounter, len(keys.counted)), collectors: make([]ctaCollector, len(keys.collected))}
	for i, key := range keys.counted {
		c.counters[i] = key.counter()
	}
	for i, key := range keys.collected {
		c.collectors[i] = key.collector()
	}
	return c
}

// countedPaths is each distinct key t counts under — an fn:count argument, an
// element step, child path or filtered child step whose existence it asks,
// the keys of a quantified expression over a child step read off the Tally —
// and, apart, each distinct key it collects values under — a path to an
// attribute whose values it reads — each in written order, read off the tree
// itself. Each key files itself ([ctaKey].file), so the split is total by
// construction.
func (t AssertionTest) countedPaths() ctaKeys {
	var keys ctaKeys
	if t.root == nil {
		// The zero AssertionTest, which no successful CompileAssertionTest
		// produces, holds nothing.
		return keys
	}
	for _, key := range t.root.counted(nil) {
		key.file(&keys)
	}
	return keys
}

// ctaKeys is the distinct keys of one compiled test, split by what a [Tally]
// keeps under each: counted under a ctaCounter, collected under a
// ctaCollector, each in written order.
type ctaKeys struct {
	counted   []ctaCountedKey
	collected []ctaCollectedKey
}

// count appends key to k.counted unless a key the same as it is there.
func (k *ctaKeys) count(key ctaCountedKey) {
	if !ctaHoldsKey(k.counted, key) {
		k.counted = append(k.counted, key)
	}
}

// collect appends key to k.collected unless a key the same as it is there.
func (k *ctaKeys) collect(key ctaCollectedKey) {
	if !ctaHoldsKey(k.collected, key) {
		k.collected = append(k.collected, key)
	}
}

// ctaHoldsKey reports whether keys holds a key the same as key.
func ctaHoldsKey[K ctaKey](keys []K, key ctaKey) bool {
	for _, k := range keys {
		if k.same(key) {
			return true
		}
	}
	return false
}

// ReadsChild reports whether the compiled {test} holds a child-axis step that
// selects a child element named name for its VALUE, which is what
// [AssertionTest.Evaluate]'s [ChildElements] must yield: a value step, `N[k]`
// behind fn:data among them, whose k-th child is found among all those named N; a child
// step an fn:count argument filters by a predicate over its value, `count(N[. = 'x'])`,
// which is counted over the children yielded; or the binding step of a quantifier whose
// body reads its bound children's values, or reads neither their values nor their
// attributes, so that the children are yielded for their places alone, and the
// `following-sibling::M[1]` step over its range variable read for its value. A child read
// only for its own attributes through a range variable bound to it, `$x/@a` or
// `$x/following-sibling::x[1]/@a`, is no read here: [AssertionTest.ReadsChildAttributes]
// answers it, and a binding step whose body reads its children's attributes and not their
// values reads none of their values. Its consumer is validate's walk, which keeps a
// child's value only where some {test} of its parent reads it. Any other fn:count path
// reads no value and is no read here — an argument that is no path reads what its operand
// reads, `count($value)` nothing — nor is an element step, a child path of two or more
// steps or a child step filtered by its children's existence or by a `preceding::` step
// whose existence alone is asked, whatever the type of the child it names: what each
// counts is [AssertionTest.Tally]'s. Nor is a path to an attribute, `sum(a/@p)`, which
// reads the attribute's value and never the child's: that value is the [Tally]'s too
// ([Tally.Attribute]). Nor is a path of child steps read for its elements' values,
// `data(a/b)`, which reads neither a's value nor any child's: b's values are the
// [Tally]'s ([Tally.ElementValue]). The answer is read off the tree itself.
func (t AssertionTest) ReadsChild(name xsd.QName) bool {
	if t.root == nil {
		// The zero AssertionTest, which no successful CompileAssertionTest
		// produces, holds nothing.
		return false
	}
	return t.root.readsChild(name)
}

// ReadsChildAttributes is the names of the attributes of a child named name
// that the compiled {test} reads through a range variable bound to that child,
// `$x/@a`, or to the child before it, `$x/following-sibling::x[1]/@a` — those
// [AssertionTest.Evaluate]'s [ChildElements] must carry on each child so named
// ([ChildWithAttributes]) — in written order, each once; empty where it reads
// none. The slice is the caller's. Its consumer is validate's walk, which keeps
// a child, and those of its attributes, where some {test} of its parent reads
// them. An attribute of a child read through a path from E, `sum(a/@p)`, is
// none of them: its value is the [Tally]'s ([Tally.Attribute]). The answer is
// read off the tree itself.
func (t AssertionTest) ReadsChildAttributes(name xsd.QName) []xsd.QName {
	if t.root == nil {
		// The zero AssertionTest, which no successful CompileAssertionTest
		// produces, holds nothing.
		return nil
	}
	var names []xsd.QName
	for _, a := range t.root.childAttributes(name, nil) {
		if !slices.Contains(names, a) {
			names = append(names, a)
		}
	}
	return names
}

// readsChild reports whether any of operands holds a child value step naming
// name, at any depth.
func (n ctaOr) readsChild(name xsd.QName) bool { return ctaAnyReadsChild(n.operands, name) }

// readsChild is ctaOr.readsChild's.
func (n ctaAnd) readsChild(name xsd.QName) bool { return ctaAnyReadsChild(n.operands, name) }

// readsChild reports whether the operand holds a child value step naming name.
func (n ctaNot) readsChild(name xsd.QName) bool { return n.operand.readsChild(name) }

// readsChild reports whether either operand is, or casts, a child value step
// naming name.
func (n ctaCompare) readsChild(name xsd.QName) bool {
	return n.left.readsChild(name) || n.right.readsChild(name)
}

// readsChild is ctaCompare.readsChild's.
func (n ctaValueCompare) readsChild(name xsd.QName) bool {
	return n.left.readsChild(name) || n.right.readsChild(name)
}

// readsChild reports whether the operand is, or casts, a child value step
// naming name.
func (n ctaEffectiveBoolean) readsChild(name xsd.QName) bool { return n.operand.readsChild(name) }

// readsChild is false: the node holds no operand.
func (ctaTypeError) readsChild(xsd.QName) bool { return false }

// readsChild is false, on ctaChildPath's terms: neither the binding step nor
// the body reads a value, and what each counts is the [Tally]'s.
func (ctaQuantified) readsChild(xsd.QName) bool { return false }

// readsChild reports whether the body holds a child value step naming name:
// the binding sequence, `$value`, reads no child.
func (n ctaQuantifiedValue) readsChild(name xsd.QName) bool { return n.body.readsChild(name) }

// readsChild reports whether the body reads the value of a child named name —
// the bound child's (ctaBoundChild.readsChild) or `$v/following-sibling::M[1]`'s
// M among them — or name is the binding step's and the body reads no attribute
// of a child so named: its children are then bound off [ChildElements] for
// their positions and the last of them alone, which are read off the children
// yielded for their values. A body reading the bound children's attributes
// has them yielded for those (ReadsChildAttributes), and reads their values
// only where it says so.
func (n ctaQuantifiedChildren) readsChild(name xsd.QName) bool {
	return n.body.readsChild(name) || n.over.name == name && len(n.body.childAttributes(name, nil)) == 0
}

// readsChild reports whether the test or either branch holds a child value
// step naming name: which branch is evaluated is not known until E is.
func (n ctaIf) readsChild(name xsd.QName) bool {
	return n.test.readsChild(name) || n.then.readsChild(name) || n.otherwise.readsChild(name)
}

// ctaAnyReadsChild is readsChild over each of operands.
func ctaAnyReadsChild(operands []ctaExpr, name xsd.QName) bool {
	for _, o := range operands {
		if o.readsChild(name) {
			return true
		}
	}
	return false
}

// readsChild reports whether the step selects children named name. It and
// ctaUntypedChild are the child value steps every readsChild here looks for.
func (n ctaTypedChild) readsChild(name xsd.QName) bool { return n.name == name }

// readsChild is ctaTypedChild.readsChild's.
func (n ctaUntypedChild) readsChild(name xsd.QName) bool { return n.name == name }

// readsChild reports whether the cast's operand is a child value step naming
// name.
func (n ctaCast) readsChild(name xsd.QName) bool { return n.operand.readsChild(name) }

// readsChild is its cast's.
func (n ctaCastable) readsChild(name xsd.QName) bool { return n.cast.readsChild(name) }

// readsChild is its operand's: fn:data over a child step reads each child's
// typed value.
func (n ctaInstanceOf) readsChild(name xsd.QName) bool { return n.operand.readsChild(name) }

// readsChild reports whether either operand of the arithmetic holds a
// child value step naming name.
func (n ctaArith) readsChild(name xsd.QName) bool {
	return n.left.readsChild(name) || n.right.readsChild(name)
}

// readsChild reports whether either argument holds a child value step naming
// name.
func (n ctaMatch) readsChild(name xsd.QName) bool {
	return n.left.operand.readsChild(name) || n.right.operand.readsChild(name)
}

// readsChild reports whether the argument holds a child value step naming name.
func (n ctaUnaryString) readsChild(name xsd.QName) bool { return n.arg.operand.readsChild(name) }

// readsChild reports whether the operand holds a child value step naming name:
// fn:exists over a cast of a child step reads its value off [ChildElements].
func (n ctaPresence) readsChild(name xsd.QName) bool { return n.operand.readsChild(name) }

// readsChild reports whether the cast fn:string is holds a child value step
// naming name.
func (n ctaStringFunction) readsChild(name xsd.QName) bool { return n.cast.readsChild(name) }

// readsChild reports whether any argument holds a child value step naming
// name.
func (n ctaConcat) readsChild(name xsd.QName) bool {
	for _, arg := range n.args {
		if arg.readsChild(name) {
			return true
		}
	}
	return false
}

// readsChild reports whether either argument holds a child value step naming
// name.
func (n ctaSubstringAfter) readsChild(name xsd.QName) bool {
	return n.left.operand.readsChild(name) || n.right.operand.readsChild(name)
}

// readsChild reports whether the argument holds a child value step naming name.
func (n ctaYearFromDate) readsChild(name xsd.QName) bool { return n.arg.operand.readsChild(name) }

// readsChild reports whether the argument holds a child value step naming name.
func (n ctaNumber) readsChild(name xsd.QName) bool { return n.operand.readsChild(name) }

// readsChild reports whether the argument holds a child value step naming name.
func (n ctaDistinctValues) readsChild(name xsd.QName) bool { return n.operand.readsChild(name) }

// readsChild is false for each of these: none is a child-axis step or holds an
// operand but the node E is, which no child is.
func (ctaAttr) readsChild(xsd.QName) bool           { return false }
func (ctaTypedAttr) readsChild(xsd.QName) bool      { return false }
func (ctaNoDocumentRoot) readsChild(xsd.QName) bool { return false }
func (ctaNoContextItem) readsChild(xsd.QName) bool  { return false }
func (ctaNoDocument) readsChild(xsd.QName) bool     { return false }
func (ctaLiteral) readsChild(xsd.QName) bool        { return false }
func (ctaValueVar) readsChild(xsd.QName) bool       { return false }
func (ctaEmptyValue) readsChild(xsd.QName) bool     { return false }
func (ctaUntypedValue) readsChild(xsd.QName) bool   { return false }
func (ctaCurrentDate) readsChild(xsd.QName) bool    { return false }
func (ctaNoFocus) readsChild(xsd.QName) bool        { return false }
func (ctaContextAtom) readsChild(xsd.QName) bool    { return false }
func (ctaNamespaceURI) readsChild(xsd.QName) bool   { return false }
func (ctaPrefixMember) readsChild(xsd.QName) bool   { return false }

// readsChild reports whether the call's argument reads the value of a child
// named name.
func (n ctaCount) readsChild(name xsd.QName) bool { return n.arg.readsChild(name) }

// readsChild is false for each of these: a counted path reads no child's
// value, and what it counts is the [Tally]'s.
func (ctaCountPath) readsChild(xsd.QName) bool        { return false }
func (ctaFilteredChildren) readsChild(xsd.QName) bool { return false }
func (ctaUnion) readsChild(xsd.QName) bool            { return false }

// readsChild reports whether the counted operand holds a child value step
// naming name: `count($value)` reads none.
func (c ctaCountedItems) readsChild(name xsd.QName) bool { return c.operand.readsChild(name) }

// readsChild reports whether m filters children named name, whose values its
// predicate reads: they are counted over [ChildElements], never the [Tally].
func (m ctaMatchingChildren) readsChild(name xsd.QName) bool { return m.name == name }

// readsChild is false: the candidate's value is the enclosing
// ctaMatchingChildren's read, which reports its name.
func (ctaCandidate) readsChild(xsd.QName) bool { return false }

// readsChild is false: the range variable is an item of `$value`, no child.
func (ctaRangeItem) readsChild(xsd.QName) bool { return false }

// readsChild reports whether the bound children are named name: the range
// variable read as an operand reads its child's value.
func (n ctaBoundChild) readsChild(name xsd.QName) bool { return n.name == name }

// readsChild is false: the attribute's value is no child's
// (ReadsChildAttributes).
func (ctaBoundAttribute) readsChild(xsd.QName) bool { return false }

// readsChild is its call's, which reads no child (ctaStepFacade).
func (n ctaBoundAttributeCall) readsChild(name xsd.QName) bool { return n.call.readsChild(name) }

// readsChild reports whether the step selects children named name, read off
// [ChildElements] beside the bound one.
func (n ctaBoundSibling) readsChild(name xsd.QName) bool { return n.step.name == name }

// readsChild reports whether the filtered step selects children named name:
// which of them is the k-th is read off [ChildElements], so each is yielded.
func (n ctaChildAt) readsChild(name xsd.QName) bool { return n.step.readsChild(name) }

// readsChild is false, on ctaCount's terms: a child path reads no node's value,
// and how many nodes it selects is the [Tally]'s.
func (ctaChildPath) readsChild(xsd.QName) bool { return false }

// readsChild is false: the attribute values a path to an attribute reads are
// the [Tally]'s, never a child's ([ChildElements]).
func (ctaAttributePath) readsChild(xsd.QName) bool { return false }

// readsChild is false, on ctaAttributePath's terms: the element values a path
// of child steps reads are the [Tally]'s ([Tally.ElementValue]).
func (ctaElementPath) readsChild(xsd.QName) bool { return false }

// readsChild is its argument's.
func (n ctaSum) readsChild(name xsd.QName) bool { return n.arg.readsChild(name) }

// readsChild is its fn:sum call's.
func (n ctaSumInstance) readsChild(name xsd.QName) bool { return n.sum.readsChild(name) }

// readsChild is false, on ctaChildPath's terms.
func (ctaSelectedElements) readsChild(xsd.QName) bool { return false }

// readsChild is false, on ctaChildPath's terms: neither the child step nor a
// step of its predicate reads a value.
func (ctaChildrenHaving) readsChild(xsd.QName) bool { return false }

// readsChild is false, on ctaChildrenHaving's terms.
func (ctaChildrenPreceded) readsChild(xsd.QName) bool { return false }

// childAttributes appends to into the name of each attribute of a child named
// name that a ctaBoundAttribute in the tree reads, at any depth, in written
// order, and returns the extended slice ([AssertionTest.ReadsChildAttributes]).
func (n ctaOr) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return ctaAnyChildAttributes(n.operands, name, into)
}

// childAttributes is ctaOr.childAttributes'.
func (n ctaAnd) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return ctaAnyChildAttributes(n.operands, name, into)
}

// ctaAnyChildAttributes is childAttributes over each of operands, in written
// order.
func ctaAnyChildAttributes(operands []ctaExpr, name xsd.QName, into []xsd.QName) []xsd.QName {
	for _, o := range operands {
		into = o.childAttributes(name, into)
	}
	return into
}

// childAttributes is its operand's.
func (n ctaNot) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.operand.childAttributes(name, into)
}

// childAttributes is either operand's, the left one first.
func (n ctaCompare) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.right.childAttributes(name, n.left.childAttributes(name, into))
}

// childAttributes is ctaCompare.childAttributes'.
func (n ctaValueCompare) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.right.childAttributes(name, n.left.childAttributes(name, into))
}

// childAttributes is its operand's.
func (n ctaEffectiveBoolean) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.operand.childAttributes(name, into)
}

// childAttributes is the test's, then either branch's, on readsChild's terms.
func (n ctaIf) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.otherwise.childAttributes(name, n.then.childAttributes(name, n.test.childAttributes(name, into)))
}

// childAttributes is its body's.
func (n ctaQuantifiedValue) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.body.childAttributes(name, into)
}

// childAttributes is its body's: the binding step reads no attribute.
func (n ctaQuantifiedChildren) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.body.childAttributes(name, into)
}

// childAttributes is its operand's.
func (n ctaCast) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.operand.childAttributes(name, into)
}

// childAttributes is its cast's.
func (n ctaCastable) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.cast.childAttributes(name, into)
}

// childAttributes is its operand's.
func (n ctaInstanceOf) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.operand.childAttributes(name, into)
}

// childAttributes is either operand's, the left one first.
func (n ctaArith) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.right.childAttributes(name, n.left.childAttributes(name, into))
}

// childAttributes is either argument's, the left one first.
func (n ctaMatch) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.right.operand.childAttributes(name, n.left.operand.childAttributes(name, into))
}

// childAttributes is its argument's.
func (n ctaUnaryString) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.arg.operand.childAttributes(name, into)
}

// childAttributes is its operand's.
func (n ctaPresence) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.operand.childAttributes(name, into)
}

// childAttributes is its cast's.
func (n ctaStringFunction) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.cast.childAttributes(name, into)
}

// childAttributes is each argument's, in written order.
func (n ctaConcat) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	for _, arg := range n.args {
		into = arg.childAttributes(name, into)
	}
	return into
}

// childAttributes is either argument's, the left one first.
func (n ctaSubstringAfter) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.right.operand.childAttributes(name, n.left.operand.childAttributes(name, into))
}

// childAttributes is its argument's.
func (n ctaYearFromDate) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.arg.operand.childAttributes(name, into)
}

// childAttributes is its argument's.
func (n ctaNumber) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.operand.childAttributes(name, into)
}

// childAttributes is its argument's.
func (n ctaDistinctValues) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.operand.childAttributes(name, into)
}

// childAttributes is its argument's.
func (n ctaCount) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.arg.childAttributes(name, into)
}

// childAttributes is its counted operand's.
func (c ctaCountedItems) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return c.operand.childAttributes(name, into)
}

// childAttributes is its argument's.
func (n ctaSum) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.arg.childAttributes(name, into)
}

// childAttributes is its fn:sum call's.
func (n ctaSumInstance) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.sum.childAttributes(name, into)
}

// childAttributes appends the attribute's name where its owner, the bound
// child or the sibling after it, is named name.
func (n ctaBoundAttribute) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	if n.owner.element() != name {
		return into
	}
	return append(into, n.name)
}

// childAttributes is the attribute step's, then its call's.
func (n ctaBoundAttributeCall) childAttributes(name xsd.QName, into []xsd.QName) []xsd.QName {
	return n.call.childAttributes(name, n.attr.childAttributes(name, into))
}

// childAttributes appends nothing for each of these: none holds a
// ctaBoundAttribute, a predicate's façade and the [Tally]'s keys reading no
// attribute through a range variable.
func (ctaTypeError) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName        { return into }
func (ctaQuantified) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName       { return into }
func (ctaPrefixMember) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName     { return into }
func (ctaTypedChild) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName       { return into }
func (ctaUntypedChild) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName     { return into }
func (ctaAttr) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName             { return into }
func (ctaTypedAttr) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName        { return into }
func (ctaNoDocumentRoot) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName   { return into }
func (ctaNoContextItem) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName    { return into }
func (ctaNoDocument) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName       { return into }
func (ctaLiteral) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName          { return into }
func (ctaValueVar) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName         { return into }
func (ctaEmptyValue) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName       { return into }
func (ctaUntypedValue) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName     { return into }
func (ctaCurrentDate) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName      { return into }
func (ctaNoFocus) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName          { return into }
func (ctaContextAtom) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName      { return into }
func (ctaNamespaceURI) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName     { return into }
func (ctaCountPath) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName        { return into }
func (ctaFilteredChildren) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName { return into }
func (ctaUnion) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName            { return into }
func (ctaMatchingChildren) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName { return into }
func (ctaCandidate) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName        { return into }
func (ctaRangeItem) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName        { return into }
func (ctaBoundChild) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName       { return into }
func (ctaBoundSibling) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName     { return into }
func (ctaChildAt) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName          { return into }
func (ctaChildPath) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName        { return into }
func (ctaAttributePath) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName    { return into }
func (ctaElementPath) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName      { return into }
func (ctaSelectedElements) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName { return into }
func (ctaChildrenHaving) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName   { return into }
func (ctaChildrenPreceded) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName { return into }
func (ctaIntegerRanges) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName    { return into }
func (ctaStringSequence) childAttributes(_ xsd.QName, into []xsd.QName) []xsd.QName   { return into }

// counted appends each path any of operands counts over.
func (n ctaOr) counted(into []ctaKey) []ctaKey { return ctaAnyCounted(n.operands, into) }

// counted is ctaOr.counted's.
func (n ctaAnd) counted(into []ctaKey) []ctaKey { return ctaAnyCounted(n.operands, into) }

// counted appends each path the operand counts over.
func (n ctaNot) counted(into []ctaKey) []ctaKey { return n.operand.counted(into) }

// counted appends each path either operand counts over, the left one first.
func (n ctaCompare) counted(into []ctaKey) []ctaKey {
	return n.right.counted(n.left.counted(into))
}

// counted is ctaCompare.counted's.
func (n ctaValueCompare) counted(into []ctaKey) []ctaKey {
	return n.right.counted(n.left.counted(into))
}

// counted appends each path the operand counts over.
func (n ctaEffectiveBoolean) counted(into []ctaKey) []ctaKey {
	return n.operand.counted(into)
}

// counted appends each path the test, then either branch, counts over, on
// readsChild's terms.
func (n ctaIf) counted(into []ctaKey) []ctaKey {
	return n.otherwise.counted(n.then.counted(n.test.counted(into)))
}

// counted appends each path the cast's operand counts over.
func (n ctaCast) counted(into []ctaKey) []ctaKey { return n.operand.counted(into) }

// counted is its cast's.
func (n ctaCastable) counted(into []ctaKey) []ctaKey { return n.cast.counted(into) }

// counted is its operand's.
func (n ctaInstanceOf) counted(into []ctaKey) []ctaKey { return n.operand.counted(into) }

// counted appends each path either operand of the arithmetic counts over, the
// left one first.
func (n ctaArith) counted(into []ctaKey) []ctaKey {
	return n.right.counted(n.left.counted(into))
}

// counted appends each path either argument counts over, the left one first.
func (n ctaMatch) counted(into []ctaKey) []ctaKey {
	return n.right.operand.counted(n.left.operand.counted(into))
}

// counted appends each path the argument counts over.
func (n ctaUnaryString) counted(into []ctaKey) []ctaKey {
	return n.arg.operand.counted(into)
}

// counted appends each path the operand counts over.
func (n ctaPresence) counted(into []ctaKey) []ctaKey { return n.operand.counted(into) }

// counted appends each path the cast fn:string is counts over.
func (n ctaStringFunction) counted(into []ctaKey) []ctaKey {
	return n.cast.counted(into)
}

// counted appends each path any argument counts over, in written order.
func (n ctaConcat) counted(into []ctaKey) []ctaKey {
	for _, arg := range n.args {
		into = arg.counted(into)
	}
	return into
}

// counted appends each path either argument counts over, the left one first.
func (n ctaSubstringAfter) counted(into []ctaKey) []ctaKey {
	return n.right.operand.counted(n.left.operand.counted(into))
}

// counted appends each path the argument counts over.
func (n ctaYearFromDate) counted(into []ctaKey) []ctaKey { return n.arg.operand.counted(into) }

// counted appends each path the argument counts over.
func (n ctaNumber) counted(into []ctaKey) []ctaKey { return n.operand.counted(into) }

// counted appends each path the argument counts over.
func (n ctaDistinctValues) counted(into []ctaKey) []ctaKey {
	return n.operand.counted(into)
}

// counted appends the path the call's argument counts over, if any.
func (n ctaCount) counted(into []ctaKey) []ctaKey { return n.arg.counted(into) }

// counted appends the key itself: the [Tally] counts the nodes it selects.
func (p ctaCountPath) counted(into []ctaKey) []ctaKey        { return append(into, p) }
func (f ctaFilteredChildren) counted(into []ctaKey) []ctaKey { return append(into, f) }
func (u ctaUnion) counted(into []ctaKey) []ctaKey            { return append(into, u) }

// counted appends each path the counted operand counts over, and never c
// itself: its items are read off the operand, so `count($value)` keys nothing
// in the [Tally].
func (c ctaCountedItems) counted(into []ctaKey) []ctaKey { return c.operand.counted(into) }

// counted appends nothing: m is counted over [ChildElements], never keyed in
// the [Tally], and its predicate counts nothing (ctaPredicateFacade.count).
func (ctaMatchingChildren) counted(into []ctaKey) []ctaKey { return into }

// counted appends nothing: the candidate is no fn:count call.
func (ctaCandidate) counted(into []ctaKey) []ctaKey { return into }

// counted appends nothing: the range variable is no fn:count call.
func (ctaRangeItem) counted(into []ctaKey) []ctaKey { return into }

// counted appends nothing for each of these: each is read off [ChildElements],
// never keyed in the [Tally].
func (ctaBoundChild) counted(into []ctaKey) []ctaKey     { return into }
func (ctaBoundSibling) counted(into []ctaKey) []ctaKey   { return into }
func (ctaBoundAttribute) counted(into []ctaKey) []ctaKey { return into }
func (ctaChildAt) counted(into []ctaKey) []ctaKey        { return into }

// counted is its call's, which counts no path (ctaStepFacade).
func (n ctaBoundAttributeCall) counted(into []ctaKey) []ctaKey { return n.call.counted(into) }

// counted appends the path itself: the [Tally] counts the nodes it selects.
func (n ctaChildPath) counted(into []ctaKey) []ctaKey { return append(into, n) }

// counted appends the path itself: the [Tally] collects the values it reads.
func (n ctaAttributePath) counted(into []ctaKey) []ctaKey { return append(into, n) }

// counted appends the path itself: the [Tally] collects the values it reads.
func (n ctaElementPath) counted(into []ctaKey) []ctaKey { return append(into, n) }

// counted is its argument's.
func (n ctaSum) counted(into []ctaKey) []ctaKey { return n.arg.counted(into) }

// counted is its fn:sum call's.
func (n ctaSumInstance) counted(into []ctaKey) []ctaKey { return n.sum.counted(into) }

// counted appends the key itself, on ctaChildPath's terms.
func (n ctaChildrenHaving) counted(into []ctaKey) []ctaKey { return append(into, n) }

// counted appends the key itself, on ctaChildPath's terms.
func (n ctaChildrenPreceded) counted(into []ctaKey) []ctaKey { return append(into, n) }

// counted appends the key counting the bindings that satisfy the body, and,
// for `every`, the key counting every binding, the one `count(N)` counts under
// too, so a {test} writing both keeps one counter for it.
func (n ctaQuantified) counted(into []ctaKey) []ctaKey {
	into = append(into, n.satisfying)
	if n.q != ctaEvery {
		return into
	}
	return append(into, n.satisfying.bound())
}

// counted appends each path the body counts over: the binding sequence,
// `$value`, keys nothing, and the quantifier none of its own.
func (n ctaQuantifiedValue) counted(into []ctaKey) []ctaKey { return n.body.counted(into) }

// counted appends each path the body counts over: the binding step is read
// off [ChildElements], keying nothing, and the quantifier keys none of its
// own.
func (n ctaQuantifiedChildren) counted(into []ctaKey) []ctaKey { return n.body.counted(into) }

// counted appends the step's path, the key an fn:count over the same step
// counts under too.
func (n ctaSelectedElements) counted(into []ctaKey) []ctaKey {
	return append(into, n.path)
}

// ctaAnyCounted is counted over each of operands, in written order.
func ctaAnyCounted(operands []ctaExpr, into []ctaKey) []ctaKey {
	for _, o := range operands {
		into = o.counted(into)
	}
	return into
}

// counted appends nothing for each of these: none is an fn:count call or holds
// an operand but the node E is, which counts nothing.
func (ctaTypeError) counted(into []ctaKey) []ctaKey      { return into }
func (ctaAttr) counted(into []ctaKey) []ctaKey           { return into }
func (ctaTypedAttr) counted(into []ctaKey) []ctaKey      { return into }
func (ctaTypedChild) counted(into []ctaKey) []ctaKey     { return into }
func (ctaUntypedChild) counted(into []ctaKey) []ctaKey   { return into }
func (ctaNoDocumentRoot) counted(into []ctaKey) []ctaKey { return into }
func (ctaNoContextItem) counted(into []ctaKey) []ctaKey  { return into }
func (ctaNoDocument) counted(into []ctaKey) []ctaKey     { return into }
func (ctaLiteral) counted(into []ctaKey) []ctaKey        { return into }
func (ctaValueVar) counted(into []ctaKey) []ctaKey       { return into }
func (ctaEmptyValue) counted(into []ctaKey) []ctaKey     { return into }
func (ctaUntypedValue) counted(into []ctaKey) []ctaKey   { return into }
func (ctaContextAtom) counted(into []ctaKey) []ctaKey    { return into }
func (ctaCurrentDate) counted(into []ctaKey) []ctaKey    { return into }
func (ctaNoFocus) counted(into []ctaKey) []ctaKey        { return into }
func (ctaNamespaceURI) counted(into []ctaKey) []ctaKey   { return into }
func (ctaPrefixMember) counted(into []ctaKey) []ctaKey   { return into }

// ctaAssertionFacade is the assertion façade compileCTATest parses for: its
// attribute nodes are typed by attrs, its child element nodes by elems, its
// `$value` by content, a nil one of which also declines `.`, and it declines
// the comparison types it cannot yet decide. readsContextItem, never nil, is
// set each time contextItem admits `.` ([AssertionTest.ReadsContextItem]).
type ctaAssertionFacade struct {
	content          xsd.ContentType
	attrs            AttributeTypes
	elems            ElementTypes
	readsContextItem *bool
}

func (ctaAssertionFacade) ctaFacade() {}

// comparesValues is true: an assertion's {test} is full XPath 2.0 (§3.13), so
// [23] ValueComp is in its grammar, and the §3.13.2 example `@min le @max`
// writes one.
func (ctaAssertionFacade) comparesValues() bool { return true }

// computes is true, on comparesValues' terms: §3.4's arithmetic is in full
// XPath 2.0.
func (ctaAssertionFacade) computes() bool { return true }

// callsLibrary is true, on comparesValues' terms: the F&O function library is
// in full XPath 2.0.
func (ctaAssertionFacade) callsLibrary() bool { return true }

// conditional is true, on comparesValues' terms: §3.8's IfExpr is in full
// XPath 2.0.
func (ctaAssertionFacade) conditional() bool { return true }

// constructsSequences is true, on comparesValues' terms: §3.3.1's sequence
// expressions are in full XPath 2.0.
func (ctaAssertionFacade) constructsSequences() bool { return true }

// castable is true, on comparesValues' terms: §3.10.3's CastableExpr is in
// full XPath 2.0.
func (ctaAssertionFacade) castable() bool { return true }

// instanceOf is true, on comparesValues' terms: §3.10.1's InstanceofExpr and
// fn:data are in full XPath 2.0.
func (ctaAssertionFacade) instanceOf() bool { return true }

// ctaValueName is the ·expanded name· of the one variable an assertion's
// static context holds (cvc-assertion clause 2.3): "no namespace URI and ...
// "value" as the local name".
var ctaValueName = xsd.QName{Local: "value"}

// variable compiles the [44] VarRef `$value` against content (cvc-assertion
// clause 2.3), and declines every other name, which is not in the static
// context (err:XPST0008) — a static error, withheld on [CompileAssertionTest]'s
// terms.
//
// Clause 2.3 is decided on content's {variety} first: anything but an
// [xsd.SimpleContent] binds the empty sequence (2.3.2), statically, which is
// ctaEmptyValue. A simple one binds the XDM representation of E's [schema
// actual value] under its {simple type definition} (2.3.1), whose static type
// ctaTypes.valueVariable classifies. A nil content declines.
func (f ctaAssertionFacade) variable(name xsd.QName, types ctaTypes) (ctaValue, bool) {
	if name != ctaValueName || f.content == nil {
		return nil, false
	}
	simple, isSimple := f.content.(xsd.SimpleContent)
	if !isSimple {
		return ctaEmptyValue{}, true
	}
	return types.valueVariable(simple.SimpleType)
}

// attribute compiles a QName NameTest whose name attrs types with a ·special·
// type ([xsd.SimpleType.IsSpecial]) to a ctaAttr, the untyped attribute node a Type
// Alternative builds — the typed value of such an attribute is its [schema
// normalized value] as xs:untypedAtomic (xpath-datamodel §3.3.1.2; Datatypes
// dt-xdmrep clause 1 for that dynamic type), which §3.5.2 casts as it casts an
// untyped one. A name attrs types with a type this engine reads as one atomic
// value (ctaTypes.typedAtomic) compiles to a ctaTypedAttr, and every other
// NameTest declines.
//
// An unbound prefix's ctaUnresolvedName reaches attrs like any other name; no
// attribute use can carry it, so the façade declines it and the parse ends
// unsupported, which an assertion withholds on exactly as it would on the
// static error.
func (f ctaAssertionFacade) attribute(test ctaNameTest, types ctaTypes) (ctaValue, bool) {
	exact, isExact := test.(ctaExactName)
	if !isExact {
		return nil, false
	}
	st, typed := f.attrs(nil, exact.name)
	if !typed {
		return nil, false
	}
	if st.IsSpecial() {
		return ctaAttr{test: exact}, true
	}
	if !types.typedAtomic(st) {
		return nil, false
	}
	return ctaTypedAttr{name: exact.name, st: st}, true
}

// attributePath compiles a path through the child steps steps to an attribute
// step whose NameTest is a QName to the ctaAttributePath over them, typed by
// the type attrs answers for (steps, name), on attribute's terms: a ·special·
// type is read as xs:untypedAtomic, and every other must be one
// ctaTypes.typedAtomic reads as one atomic value. Every other NameTest and
// type declines. No child step is typed: none is atomized, and the static
// type of the element each selects is attrs' to follow (key-ldt-elem).
//
// GAP(xpath): a wildcard NameTest declines, on attribute's terms, and so does
// a list or union type, whose atomized value is a sequence or of a type only
// the instance decides (ctaTypes.typedAtomic). The direction is the withhold
// [CompileAssertionTest] reports. (#1042)
func (f ctaAssertionFacade) attributePath(steps []xsd.QName, test ctaNameTest, types ctaTypes) (ctaValue, bool) {
	exact, isExact := test.(ctaExactName)
	if !isExact {
		return nil, false
	}
	st, typed := f.attrs(steps, exact.name)
	if !typed {
		return nil, false
	}
	if !st.IsSpecial() && !types.typedAtomic(st) {
		return nil, false
	}
	return ctaAttributePath{steps: steps, name: exact.name, st: st}, true
}

// child compiles a QName NameTest on the child axis whose one-step path elems
// types to a ctaTypedChild, over the simple type a child of that type has its
// typed value in (ctaChildValueType): xpath-datamodel §6.2.4 makes the typed
// value of an element whose type is a simple type, or a complex type with
// simple content, the one §3.3.1.2 computes for that simple type. The simple
// type must be one ctaTypes.typedAtomic reads as one atomic value. A name
// elems types with a complex type whose {content type}.{variety} is mixed —
// xs:anyType among them, never singled out by name — compiles to a
// ctaUntypedChild instead: §6.2.4 makes such an element's typed value its
// string-value as xs:untypedAtomic. Every other NameTest declines, and so does
// every other type:
//
//   - a ·special· simple type, or simple content over one: the compiled type is
//     static, and an xsi:type can make the child's own type a typed one, so
//     reading it as xs:untypedAtomic could charge a valid child. A mixed type
//     carries no such hazard: the caller withholds a child whose own type has
//     no mixed content ([ChildElements]);
//   - a complex type whose {content type} is element-only (atomizing it is a
//     type error) or empty (its typed value is the empty sequence) — each read
//     the way xpath-datamodel §6.2.4 says, but by its own node this engine does
//     not build.
//
// A step standing as the whole operand of fn:exists, fn:empty or an ·effective
// boolean value· never reaches here: its value is not read, and elements
// builds its node whatever the type.
func (f ctaAssertionFacade) child(test ctaNameTest, types ctaTypes) (ctaValue, bool) {
	exact, isExact := test.(ctaExactName)
	if !isExact {
		return nil, false
	}
	td, typed := f.elems([]xsd.QName{exact.name})
	if !typed {
		return nil, false
	}
	if ctaMixedType(td) {
		return ctaUntypedChild(exact), true
	}
	st, simple := ctaChildValueType(td)
	if !simple || st.IsSpecial() || !types.typedAtomic(st) {
		return nil, false
	}
	return ctaTypedChild{name: exact.name, st: st}, true
}

// elementPath compiles a path of two or more child steps read for its
// elements' typed values to the ctaElementPath over steps, typed by the type
// elems answers for steps, on child's terms for a child read TYPED: the simple
// type that type has its typed value in (ctaChildValueType), which must be one
// ctaTypes.typedAtomic reads as one atomic value and not ·special·. No step but
// the last is typed: none is atomized, and the static type of the element
// each selects is elems' to follow (key-ldt-elem).
//
// GAP(xpath): a last step of mixed content declines, xs:anyType among them:
// its typed value is its string-value (xpath-datamodel §6.2.4), which validate
// collects only for a child of E (its string-value frames). So does a ·special·
// simple type, on child's terms. The direction is the withhold
// [CompileAssertionTest] reports. (#1042) A last step of element-only or empty
// content declines too, not as a gap: fn:data over an element-only one raises
// err:FOTY0012 (xpath-functions.md §2.4), which no node here raises, and must
// never become a silent false.
func (f ctaAssertionFacade) elementPath(steps []xsd.QName, types ctaTypes) (ctaValue, bool) {
	td, typed := f.elems(steps)
	if !typed || ctaMixedType(td) {
		return nil, false
	}
	st, simple := ctaChildValueType(td)
	if !simple || st.IsSpecial() || !types.typedAtomic(st) {
		return nil, false
	}
	return ctaElementPathOf(steps, st)
}

// childPath compiles a relative path of two or more child-axis steps to a
// ctaChildPath over parents and last, consulting neither attrs nor elems: the
// path stands only where fn:exists, fn:empty or an ·effective boolean value·
// asks whether it selects a node, none of which atomizes it
// (xpath-functions.md §15.1.4, §15.1.5, `$arg as item()*`; xpath20.md §2.4.3
// rule 2), so no step's type decides anything and a node of any type, ·nilled·
// or not, is selected — on count's terms. What the path selects is read off
// the [Tally].
func (ctaAssertionFacade) childPath(parents []xsd.QName, last ctaElementTest) (ctaValue, bool) {
	path, admitted := ctaChildPathOf(parents, last)
	if !admitted {
		return nil, false
	}
	return path, true
}

// childrenHaving compiles a child step filtered by a conjunction of child
// steps to a ctaChildrenHaving over name and required, consulting neither
// attrs nor elems, on childPath's terms: each step's ·effective boolean value·
// asks only whether it selects a node (xpath20.md §2.4.3 rule 2). What the
// step selects is read off the [Tally].
func (ctaAssertionFacade) childrenHaving(name xsd.QName, required []xsd.QName) (ctaValue, bool) {
	having, admitted := ctaChildrenHavingOf(name, required)
	if !admitted {
		return nil, false
	}
	return having, true
}

// childrenPreceded compiles a child step filtered by a `preceding::` step to
// preceded itself, consulting neither attrs nor elems, on childrenHaving's
// terms. What it selects is read off the [Tally].
func (ctaAssertionFacade) childrenPreceded(preceded ctaChildrenPreceded) (ctaValue, bool) {
	return preceded, true
}

// elements compiles one element step, `N`, `./N` or `.//N`, to a
// ctaSelectedElements over path, consulting neither attrs nor elems, on
// childPath's terms: the step stands only where its existence is asked, so N's
// type decides nothing — an element-only, mixed, empty or ·special· one is
// selected as a simple one is — and what it selects is read off the [Tally].
// An attribute axis declines; the parser builds none here.
func (ctaAssertionFacade) elements(path ctaCountPath) (ctaValue, bool) {
	selected, admitted := ctaSelectedElementsOf(path)
	if !admitted {
		return nil, false
	}
	return selected, true
}

// comments compiles one comment step, `comment()`, `./comment()` or
// `.//comment()`, to ctaEmptyValue, consulting neither attrs nor elems:
// cvc-assertion clause 1.2 excludes comments from the partial ·PSVI· clause
// 1.3 builds the data model instance from, retaining them only ·at user
// option·, which this engine offers none of, so E's subtree holds no comment
// node and the step selects the empty sequence whatever E carries. Nothing is
// counted and nothing read.
func (ctaAssertionFacade) comments() (ctaValue, bool) {
	return ctaEmptyValue{}, true
}

// quantified compiles a quantified expression over one child step to its
// ctaQuantified, consulting neither attrs nor elems, on childPath's terms:
// neither the binding step nor the body is atomized — the body's ·effective
// boolean value· asks only whether its step selects a node (xpath20.md
// §2.4.3 rule 2) — so a child of any type, ·nilled· or not, is bound. What
// each counts is read off the [Tally].
func (ctaAssertionFacade) quantified(q ctaQuantifier, satisfying ctaRangeKey) (ctaExpr, bool) {
	return ctaQuantified{q: q, satisfying: satisfying}, true
}

// rangeScope is f with variable in scope as the range variable over the items
// of over (ctaRangeFacade), each of over's item type: the body of a
// quantifier over a typed `$value` parses under it.
func (f ctaAssertionFacade) rangeScope(variable xsd.QName, over ctaValueVar) (ctaFacade, bool) {
	return ctaRangeFacade{ctaAssertionFacade: f, name: variable, item: ctaRangeItem{st: over.atom}}, true
}

// childScope is f with variable in scope as the range variable over the
// children over selects (ctaChildRangeFacade), each a node whose typed value is
// of over's type: the body of a quantifier over a typed child step parses
// under it.
func (f ctaAssertionFacade) childScope(variable xsd.QName, over ctaTypedChild) (ctaFacade, bool) {
	return ctaChildRangeFacade{ctaAssertionFacade: f, name: variable, item: ctaBoundChild(over)}, true
}

// boundAttribute declines, on boundSibling's terms: outside a quantifier's
// body over a child step no variable is a node an attribute step could step
// from.
func (ctaAssertionFacade) boundAttribute(ctaValue, ctaNameTest, ctaTypes) (ctaBoundAttribute, bool) {
	return ctaBoundAttribute{}, false
}

// boundSibling declines: outside a quantifier's body over a child step no
// variable is a node `following-sibling::` could step from, `$value` included
// (cvc-assertion clause 2.3 binds it to atomic values).
func (ctaAssertionFacade) boundSibling(xsd.QName, ctaNameTest, ctaTypes) (ctaValue, bool) {
	return nil, false
}

// ctaRangeFacade is the façade the body of an assertion's quantifier over a
// typed `$value` parses under (ctaParser.quantifiedExpr): the assertion
// façade, with the range variable, whose ·expanded name· is name, added to
// its in-scope variables (xpath20.md §3.9), compiled to item. It embeds the concrete
// ctaAssertionFacade, so no other façade's scope can hold a range variable,
// and every production it does not override is the assertion's own — a `.`
// in the body records its read through the embedded readsContextItem.
//
// Its scope is the body alone: the parser swaps it in for the body and the
// outer façade back after, so `$x` beyond the body is the err:XPST0008 the
// outer façade declines.
type ctaRangeFacade struct {
	ctaAssertionFacade
	name xsd.QName
	item ctaRangeItem
}

// variable compiles the range variable, name, to item, and every other name as the
// assertion façade does, `$value` among them.
func (f ctaRangeFacade) variable(name xsd.QName, types ctaTypes) (ctaValue, bool) {
	if name == f.name {
		return f.item, true
	}
	return f.ctaAssertionFacade.variable(name, types)
}

// quantified declines a quantifier over a child step inside the body.
//
// GAP(xpath): a nested quantifier declines, here, in rangeScope and in
// childScope, and so does one nested in a body over a typed child step
// (ctaChildRangeFacade). The direction is the withhold [CompileAssertionTest]
// reports. (#1042)
func (ctaRangeFacade) quantified(ctaQuantifier, ctaRangeKey) (ctaExpr, bool) {
	return nil, false
}

// rangeScope declines a quantifier over `$value` inside the body, under
// quantified's GAP(xpath).
func (ctaRangeFacade) rangeScope(xsd.QName, ctaValueVar) (ctaFacade, bool) {
	return nil, false
}

// childScope declines a quantifier over a typed child step inside the body,
// under quantified's GAP(xpath).
func (ctaRangeFacade) childScope(xsd.QName, ctaTypedChild) (ctaFacade, bool) {
	return nil, false
}

// ctaChildRangeFacade is the façade the body of an assertion's quantifier over
// a typed child step parses under (ctaParser.childQuantified): the assertion
// façade, with the range variable, whose ·expanded name· is name, added to its
// in-scope variables (xpath20.md §3.9), compiled to item, and the paths over
// it this engine reads, `$v/following-sibling::M[1]` (boundSibling) and an
// attribute step over either (boundAttribute). It embeds the concrete
// ctaAssertionFacade on ctaRangeFacade's terms, and its scope is the body
// alone on the same terms.
type ctaChildRangeFacade struct {
	ctaAssertionFacade
	name xsd.QName
	item ctaBoundChild
}

// variable compiles the range variable, name, to item, and every other name as
// the assertion façade does, `$value` among them.
func (f ctaChildRangeFacade) variable(name xsd.QName, types ctaTypes) (ctaValue, bool) {
	if name == f.name {
		return f.item, true
	}
	return f.ctaAssertionFacade.variable(name, types)
}

// boundSibling compiles `$v/following-sibling::M[1]` over the range variable,
// name, to the ctaBoundSibling over the step M as the assertion façade's child
// compiles it, so its typed value is of the type M's ·locally declared type·
// gives it. Every other variable declines, and so does an M that child does
// not compile to a ctaTypedChild — one whose type child declines, or one of
// mixed content, whose untyped value no bound node reads — under
// ctaParser.quantifiedExpr's GAP(xpath).
func (f ctaChildRangeFacade) boundSibling(variable xsd.QName, next ctaNameTest, types ctaTypes) (ctaValue, bool) {
	if variable != f.name {
		return nil, false
	}
	step, typed := f.child(next, types)
	sibling, isTyped := step.(ctaTypedChild)
	if !typed || !isTyped {
		return nil, false
	}
	return ctaBoundSibling{step: sibling}, true
}

// boundAttribute compiles an attribute step whose NameTest is a QName over
// owner, the range variable (ctaBoundChild) or `$v/following-sibling::M[1]`
// over it (ctaBoundSibling), to the ctaBoundAttribute typed by the type attrs
// answers for the one-step path of the owner's name, on
// ctaAssertionFacade.attributePath's terms but for a ·special· type. Every
// other owner — `$value`, a range variable over `$value`, no node at all —
// declines, and so does every other NameTest and type.
//
// GAP(xpath): a wildcard NameTest declines, on attributePath's terms, and so do
// a ·special· type, whose xs:untypedAtomic value no ctaCandidate reads as the
// context item of a call after the step, and a list or union type
// (ctaTypes.typedAtomic). The direction is the withhold [CompileAssertionTest]
// reports. (#1042)
func (f ctaChildRangeFacade) boundAttribute(owner ctaValue, test ctaNameTest, types ctaTypes) (ctaBoundAttribute, bool) {
	bound, isBound := owner.(ctaBoundOwner)
	exact, isExact := test.(ctaExactName)
	if !isBound || !isExact {
		return ctaBoundAttribute{}, false
	}
	st, typed := f.attrs([]xsd.QName{bound.element()}, exact.name)
	if !typed || st.IsSpecial() || !types.typedAtomic(st) {
		return ctaBoundAttribute{}, false
	}
	return ctaBoundAttribute{owner: bound, name: exact.name, st: st}, true
}

// quantified declines a quantifier over a child step inside the body, under
// ctaRangeFacade.quantified's GAP(xpath).
func (ctaChildRangeFacade) quantified(ctaQuantifier, ctaRangeKey) (ctaExpr, bool) {
	return nil, false
}

// rangeScope declines a quantifier over `$value` inside the body, under
// ctaRangeFacade.quantified's GAP(xpath).
func (ctaChildRangeFacade) rangeScope(xsd.QName, ctaValueVar) (ctaFacade, bool) {
	return nil, false
}

// childScope declines a quantifier over a typed child step inside the body,
// under ctaRangeFacade.quantified's GAP(xpath).
func (ctaChildRangeFacade) childScope(xsd.QName, ctaTypedChild) (ctaFacade, bool) {
	return nil, false
}

// ctaChildValueType is the simple type the typed value of an element of type
// td is computed in (xpath-datamodel §6.2.4): td itself where it is simple,
// its {simple type definition} where it is complex with a simple {content
// type}, and false otherwise. The default arm is unreachable:
// [xsd.TypeDefinition] is a sealed sum of the two variants named (STYLE T2's
// schema-closed-set exception).
func ctaChildValueType(td xsd.TypeDefinition) (*xsd.SimpleType, bool) {
	switch t := td.(type) {
	case *xsd.SimpleType:
		return t, true
	case xsd.ComplexType:
		sc, simple := t.ContentType().(xsd.SimpleContent)
		return sc.SimpleType, simple
	}
	return nil, false
}

// ctaMixedType reports whether td is a complex type whose {content
// type}.{variety} is mixed, whose element's typed value is its string-value as
// xs:untypedAtomic (xpath-datamodel §6.2.4) — xs:anyType among them, whose
// {variety} is mixed (§3.4.7), and which is not told apart by its name.
func ctaMixedType(td xsd.TypeDefinition) bool {
	ct, isComplex := td.(xsd.ComplexType)
	return isComplex && ct.ContentType().Variety() == xsd.ContentMixed
}

// rooted compiles a path opening with "/" or "//" to ctaNoDocumentRoot: E is
// the root of the instance cvc-assertion clause 1.3 builds, and no document
// node is above it, so the path raises err:XPDY0050 (xpath20.md §3.2).
func (ctaAssertionFacade) rooted() (ctaValue, bool) {
	return ctaNoDocumentRoot{}, true
}

// count compiles an fn:count call over the path arg to its ctaCount (ctaCountOf). arg
// is never typed against attrs or elems: fn:count does not atomize it
// ([CompileAssertionTest]).
func (ctaAssertionFacade) count(arg ctaCounted, types ctaTypes) (ctaValue, bool) {
	return ctaCountOf(arg, types)
}

// ctaCountOf is the ctaCount of xs:integer over arg, the type
// xpath-functions.md §15.4.1 gives fn:count's result, false where types
// resolves no xs:integer.
func ctaCountOf(arg ctaCounted, types ctaTypes) (ctaValue, bool) {
	integer, resolved := types.simple(ctaBuiltin("integer"))
	if !resolved {
		return nil, false
	}
	return ctaCount{arg: arg, st: integer}, true
}

// contextItem compiles the [47] ContextItemExpr `.` to ctaContextAtom under
// every {content type}. The context item is E (cvc-xpath clause 1), annotated
// xs:anyType in the partial ·PSVI· (cvc-assertion clause 1.2; the suite's
// saxonMeta/Assert.testSet:692 reads it so), so `.` atomizes to one
// xs:untypedAtomic value, E's string value (xpath-datamodel.md:488, :1327) —
// under element-only, empty and mixed content as under simple, and under a
// list or union {simple type definition} as under an atomic one, the variety
// deciding `$value`'s type alone. Which text that string value is, is
// [BindValue]'s caller's to supply, and contextItem records that it is read
// (f.readsContextItem), which [AssertionTest.ReadsContextItem] answers.
//
// GAP(xpath): a nil content, whose governing type was not determined, declines
// `.`. The direction is the withhold [CompileAssertionTest] reports. (#1042)
func (f ctaAssertionFacade) contextItem() (ctaValue, bool) {
	if f.content == nil {
		return nil, false
	}
	*f.readsContextItem = true
	return ctaContextAtom{}, true
}

// contextNode compiles `.` as the node fn:namespace-uri and
// fn:in-scope-prefixes take to ctaContextNode, E itself (cvc-xpath clause 1),
// under every {content type} and under a nil one: E's name and in-scope
// namespaces are E's own whatever its governing type, and neither function
// reads E's string value, so contextNode records no read of it
// (f.readsContextItem) and [AssertionTest.ReadsContextItem] stays false for a
// {test} whose only `.` is such an argument.
func (ctaAssertionFacade) contextNode() (ctaNodeArg, bool) {
	return ctaContextNode{}, true
}

// focus declines every call to fn:position and fn:last. The focus is
// defined — context item E, context position and context size 1 (cvc-xpath
// clauses 1–3) — so each call has an answer, and it neither raises
// err:XPDY0002 nor reads E's value, which contextItem's ctaContextAtom is.
//
// GAP(xpath): `position()` and `last()` outside a predicate decline rather
// than evaluating to 1. The direction is the withhold [CompileAssertionTest]
// reports. (#1042)
func (ctaAssertionFacade) focus(*xsd.SimpleType) (ctaValue, bool) {
	return nil, false
}

// document compiles a call to fn:doc whose argument is a StringLiteral to
// ctaNoDocument: cvc-xpath clause 8 makes the available documents empty, so
// the call raises err:FODC0005 (xpath-functions.md §15.5.4) and the {test}
// does not evaluate to true (cvc-assertion).
func (ctaAssertionFacade) document() (ctaValue, bool) {
	return ctaNoDocument{}, true
}
