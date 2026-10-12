package xpath

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// This file evaluates the F&O functions the assertion and facet façades call
// beyond fn:not and fn:count (ctaFacade.callsLibrary), each of which a Type
// Alternative's {test} declines: fn:contains, fn:starts-with and fn:ends-with
// (xpath-functions.md §7.5.1–7.5.3, ctaMatch), fn:string-length and
// fn:normalize-space (§7.4.4, §7.4.5, ctaUnaryString), fn:empty and fn:exists
// (§15.1.4, §15.1.5, ctaPresence), fn:distinct-values (§15.1.6,
// ctaDistinctValues), fn:string (§2.3, ctaStringFunction), fn:concat
// (§7.4.1, ctaConcat), fn:substring-after (§7.5.5, ctaSubstringAfter) and
// fn:number (§14.4, ctaNumber) — and fn:count over an argument that is no path
// (§15.4.1, ctaCountedItems), whose items are counted as fn:empty and
// fn:exists count them, and fn:sum with one argument over a path to an
// attribute below E (§15.4.5, ctaSum), whose values the [Tally] collects.
// fn:true and fn:false (§9.1.1, §9.1.2) are constants, which compile to the
// ctaLiteral of their xs:boolean (ctaParser.constantCall). fn:current-date
// (§16.4) reads the dynamic context's current dateTime (ctaCurrentDate), and
// fn:position and fn:last (§16.1, §16.2) its focus, which only the facet
// façade compiles, to the err:XPDY0002 of an absent one (ctaNoFocus,
// ctaFacade.focus). fn:namespace-uri (§14.3, ctaNamespaceURI) and
// fn:in-scope-prefixes (§11.2.6), the latter as an operand of `=` against
// string literals (ctaPrefixMember), read E itself, `.` taken as a node and
// never atomized (ctaContextNode, ctaFacade.contextNode), through the
// [ContextElement] the evaluation carries.
//
// An argument whose parameter is xs:string? is converted by xpath20.md
// §3.1.5's function conversion rules, as far as the static type settles them,
// at compile time (ctaTypes.stringArgument), and read here by ctaStringOf. The
// strings are compared under the default collation, which xpath-valid clause
// 2.2.10 fixes as the Unicode codepoint collation, so a match is a match of
// code points — which a match of UTF-8 bytes is.

// ctaMatchOp is one of the three functions xpath-functions.md §7.5 matches one
// xs:string against another with: fn:contains, fn:starts-with and
// fn:ends-with.
type ctaMatchOp byte

const (
	ctaContains ctaMatchOp = iota
	ctaStartsWith
	ctaEndsWith
)

// ctaUnaryStringOp is one of the two functions of one xs:string? argument this
// grammar calls: fn:string-length (§7.4.4) and fn:normalize-space (§7.4.5).
type ctaUnaryStringOp byte

const (
	ctaStringLength ctaUnaryStringOp = iota
	ctaNormalizeSpace
)

// ctaPresenceOp is one of the two functions of one `item()*` argument this
// grammar calls: fn:empty (§15.1.4) and fn:exists (§15.1.5).
type ctaPresenceOp byte

const (
	ctaEmptyTest ctaPresenceOp = iota
	ctaExistsTest
)

// ctaStringArgument is one argument of an F&O function whose parameter is
// xs:string?, as ctaTypes.stringArgument converted it at compile time: operand
// yields items of the xs:string family or none — an xs:untypedAtomic operand
// is held under its cast to xs:string — and mistyped marks an operand of any
// other type, whose every item is the err:XPTY0004 xpath20.md §3.1.5 raises.
// ctaStringOf reads it.
type ctaStringArgument struct {
	operand  ctaValue
	mistyped bool
}

// ctaMatch is a call to fn:contains, fn:starts-with or fn:ends-with (op) with
// two arguments, whose result is st, xs:boolean.
type ctaMatch struct {
	op          ctaMatchOp
	left, right ctaStringArgument
	st          *xsd.SimpleType
}

// ctaUnaryString is a call to fn:string-length or fn:normalize-space (op) with
// one argument, written or implicit (ctaParser.unaryStringCall), whose result
// is st: xs:integer for fn:string-length and xs:string for fn:normalize-space
// (resultType).
type ctaUnaryString struct {
	op  ctaUnaryStringOp
	arg ctaStringArgument
	st  *xsd.SimpleType
}

// ctaPresence is a call to fn:empty or fn:exists (op), whose result is st,
// xs:boolean. The operand is `item()*` and is not atomized: a step's nodes are
// counted whatever their values, a ·nilled· child among them
// (ctaSequenceLength).
type ctaPresence struct {
	op      ctaPresenceOp
	operand ctaValue
	st      *xsd.SimpleType
}

// ctaStringFunction is a call to fn:string, which is its argument cast to
// xs:string (xpath-functions.md §2.3, ctaParser.stringOf) with the empty
// sequence mapped to the zero-length string. Its static type is cast.target.
type ctaStringFunction struct{ cast ctaCast }

// ctaConcat is a call to fn:concat with two or more arguments
// (xpath-functions.md §7.4.1, ctaParser.concatCall), whose result is st,
// xs:string: the strings of args, in written order, each argument held as
// the fn:string call over it, which casts it to xs:string and maps the empty
// sequence to the zero-length string.
type ctaConcat struct {
	args []ctaStringFunction
	st   *xsd.SimpleType
}

// ctaSubstringAfter is a call to fn:substring-after with two arguments
// (xpath-functions.md §7.5.5, ctaParser.substringAfterCall), whose result is
// st, xs:string: the part of left's string after the first occurrence of
// right's, under the codepoint collation (ctaSubstringAfterItem).
type ctaSubstringAfter struct {
	left, right ctaStringArgument
	st          *xsd.SimpleType
}

// ctaNumber is a call to fn:number with one argument, written or implicit
// (xpath-functions.md §14.4, ctaParser.numberCall), whose result is st,
// xs:double: the operand's one item cast to xs:double (§17.1.3.2), and NaN
// where the operand is empty or the cast fails (ctaNumberItem). read is the
// type the operand's items are read in before that cast: the operand's own
// static type where it is typed, and xs:string otherwise — an xs:untypedAtomic
// item cast to xs:string keeps its string, and the statically empty operand
// holds no item to read.
type ctaNumber struct {
	operand ctaValue
	read    *xsd.SimpleType
	st      *xsd.SimpleType
}

// ctaDistinctValues is a call to fn:distinct-values with its one
// `xs:anyAtomicType*` argument (xpath-functions.md §15.1.6): the atomized
// operand with every item eq to an earlier one dropped (ctaDistinctValues.eval).
// Its static type is the operand's — an item survives under its own type, and
// an xs:untypedAtomic one stays xs:untypedAtomic (ctaStaticOf). st is the type
// its items are read in to be compared, which ctaParser.distinctValuesCall
// resolves: the operand's own static type where it is typed, and xs:string
// where it is xs:untypedAtomic, which §15.1.6 compares "as if it were of type
// xs:string". A statically empty argument is never one: distinctValuesCall
// compiles it to ctaEmptyValue.
type ctaDistinctValues struct {
	operand ctaValue
	st      *xsd.SimpleType
}

// ctaCountedItems is an fn:count argument that is no path (ctaCounted): a
// `$value`, `()`, a function call or any other operand a library call takes
// as its argument (ctaParser.argument), whose items fn:count counts without
// atomizing them (xpath-functions.md §15.4.1, `item()*`) — ctaSequenceLength,
// fn:empty's and fn:exists' reading. It is no counter key (ctaTallied) and no
// union operand: what it counts is read off the operand, never off the
// [Tally], so it keys only what its operand counts.
type ctaCountedItems struct{ operand ctaValue }

// ctaCurrentDate is a call to fn:current-date with no argument
// (xpath-functions.md §16.4), whose result is st, xs:date: "xs:date(
// fn:current-dateTime())", the date of the dynamic context's current dateTime
// (xpath20.md §2.1.2) in that dateTime's own timezone, which the cast keeps
// (§17.1.5) — never the implicit timezone, which F&O §10.4 assumes only on a
// compared operand that has none. The instant is ctaTypedInput.now, so every
// call in one evaluation, and every evaluation handed the same instant,
// returns the same date (§16.4 "stable"; cvc-xpath clause 6).
type ctaCurrentDate struct{ st *xsd.SimpleType }

// ctaNoFocus is a call to fn:position or fn:last with no argument
// (xpath-functions.md §16.1, §16.2), whose result is st, xs:integer, where
// the focus is absent — an assertions facet's {test}, which has no context
// item, context position or context size (cvc-assertions-valid clauses 1.2
// and 1.3). Each function raises err:XPDY0002 "If the context item is
// undefined", so evaluating the node raises whichever it calls; its static
// type is the xs:integer either would return, so `position() le 50` compiles
// to a comparison that raises and not to the err:XPTY0004 an untyped operand
// would make of it. Only the facet façade builds it (ctaFacetFacade.focus).
type ctaNoFocus struct{ st *xsd.SimpleType }

// ctaNodeArg is the sealed sum of what the [47] ContextItemExpr `.` compiles
// to as the NODE argument of fn:namespace-uri and fn:in-scope-prefixes, whose
// parameter is node() and element() (xpath-functions.md §14.3, §11.2.6), never
// atomized (ctaFacade.contextNode): E itself (ctaContextNode) or an assertions
// facet's absent context item (ctaAbsentNode). It is no ctaValue: no item is
// ever read of it, and ctaContextElementOf is its one reader.
type ctaNodeArg interface{ ctaNodeArg() }

// ctaContextNode is `.` over E taken as a node, which only the assertion
// façade builds (ctaAssertionFacade.contextNode). ctaContextElementOf reads it
// as the evaluation's [ContextElement].
type ctaContextNode struct{}

// ctaAbsentNode is `.` taken as a node where there is no context item, which
// only the facet façade builds (ctaFacetFacade.contextNode): either function
// over it raises err:XPDY0002 (xpath20.md §3.1.4; cvc-assertions-valid clause
// 1.2), on ctaNoContextItem's terms.
type ctaAbsentNode struct{}

func (ctaContextNode) ctaNodeArg() {}
func (ctaAbsentNode) ctaNodeArg()  {}

// ctaNamespaceURI is a call to fn:namespace-uri over E (xpath-functions.md
// §14.3), written `namespace-uri(.)` or `namespace-uri()`, whose argument
// "defaults to the context node (.)": one item of st, xs:anyURI, E's
// [namespace name] — for an E in no namespace "the xs:anyURI corresponding to
// the zero-length string", never the empty sequence. context is what the
// façade compiles `.` to as a node (ctaFacade.contextNode): ctaContextNode,
// or an assertions facet's ctaAbsentNode, over which the call raises
// err:XPDY0002.
type ctaNamespaceURI struct {
	context ctaNodeArg
	st      *xsd.SimpleType
}

func (ctaMatch) ctaValue()          {}
func (ctaUnaryString) ctaValue()    {}
func (ctaPresence) ctaValue()       {}
func (ctaStringFunction) ctaValue() {}
func (ctaConcat) ctaValue()         {}
func (ctaSubstringAfter) ctaValue() {}
func (ctaNumber) ctaValue()         {}
func (ctaDistinctValues) ctaValue() {}
func (ctaCurrentDate) ctaValue()    {}
func (ctaNoFocus) ctaValue()        {}
func (ctaNamespaceURI) ctaValue()   {}
func (ctaSum) ctaValue()            {}
func (ctaSumInstance) ctaValue()    {}

// ctaSum is a call to fn:sum with its one `xs:anyAtomicType*` argument
// (xpath-functions.md §15.4.5, ctaParser.sumCall), arg, whose types
// ctaTypes.sum settled: item is the type arg's items are read in — their own
// type, or xs:double where they are xs:untypedAtomic, which §15.4.5 casts to
// xs:double — st the call's static type, which an enclosing operator converts
// its value into, and integer xs:integer, the type of the 0 an empty argument
// sums to.
//
// The value's DYNAMIC type is not st alone (ctaSum.total): the xs:integer 0
// for an empty argument, the one item itself for an argument of one, and st
// for two or more. Each derives from st where st is xs:integer or xs:decimal,
// so converting it into st is subtype substitution. Where st is xs:float or
// xs:double the 0 does not, which is why such a call is no arithmetic operand
// and is compared with an xs:decimal operand only where that cannot tell the
// two types apart (ctaTypes.arithmeticOperand, ctaTypes.sumCompares), and why
// `instance of` over any call reads the dynamic type (ctaSumInstance).
type ctaSum struct {
	arg     ctaValue
	item    *xsd.SimpleType
	st      *xsd.SimpleType
	integer *xsd.SimpleType
}

// ctaSumInstance is `E instance of T` (xpath20.md §3.10.1) over an fn:sum
// call sum (ctaParser.sumInstanceTail), whose result is st, xs:boolean: true
// where the DYNAMIC type of the call's one item (ctaSum.total) is the
// AtomicType named atomic or derives from it (§2.5.4.2), which only the
// evaluation decides. Every occurrence indicator admits the one item, so none
// is kept.
type ctaSumInstance struct {
	sum    ctaSum
	atomic xsd.QName
	st     *xsd.SimpleType
}

// floating reports whether n's static type is xs:float or xs:double, from
// neither of which the xs:integer 0 of an empty argument derives.
func (n ctaSum) floating() bool {
	return n.st.Name() == ctaBuiltin("float") || n.st.Name() == ctaBuiltin("double")
}

// ctaFloatingSum is the fn:sum call whose static type v takes, where that call
// is floating (ctaSum.floating): v itself, or the operand of an
// fn:distinct-values call over one, which keeps its operand's static type
// (ctaStaticOf) and so its operand's xs:integer 0 over an empty argument. It
// is false for every other v.
func ctaFloatingSum(v ctaValue) (ctaSum, bool) {
	switch n := v.(type) {
	case ctaSum:
		return n, n.floating()
	case ctaDistinctValues:
		return ctaFloatingSum(n.operand)
	}
	return ctaSum{}, false
}

// total evaluates n (xpath-functions.md §15.4.5): its value and that value's
// dynamic type, or false where it raises. The argument's items are read in
// n.item — so an xs:untypedAtomic item is cast to xs:double, and one that
// does not cast raises err:FORG0001 — in document order. Then:
//
//   - none: the xs:integer 0;
//   - any, where n.item's {primitive type definition} is not numeric, so it
//     does not support addition: err:FORG0006 — the duration types, which do,
//     ctaTypes.sum declines;
//   - one: that item, of type n.item;
//   - two or more: their sum, of type n.st, added in document order through
//     each item's ·canonical representation· on ctaArith's terms, so a sum of
//     xs:decimal items is exact (§6.2) and one containing NaN is NaN. §15.4.5
//     lets the items be added in any order; this one fixes it (STYLE D1).
func (n ctaSum) total(env ctaEnv) (value.Value, *xsd.SimpleType, bool) {
	atoms, read := ctaItemOf(n.arg, n.item, env).(ctaAtoms)
	if !read {
		return nil, nil, false
	}
	if len(atoms.vs) == 0 {
		zero, validated := ctaValidated(ctaValidate("0", n.integer, env))
		return zero, n.integer, validated
	}
	p, err := n.item.Primitive(env.types)
	if err != nil || p == nil {
		return nil, nil, false
	}
	kind, numeric := ctaNumericKindOf(p)
	if !numeric {
		return nil, nil, false // err:FORG0006
	}
	if len(atoms.vs) == 1 {
		return atoms.vs[0], n.item, true
	}
	adding := ctaArith{op: ctaAdd, kind: kind}
	total, rendered := ctaCanonical(atoms.vs[0], n.item, env)
	if !rendered {
		return nil, nil, false
	}
	for _, v := range atoms.vs[1:] {
		lexical, rendered := ctaCanonical(v, n.item, env)
		if !rendered {
			return nil, nil, false
		}
		if total, rendered = adding.compute(total, lexical); !rendered {
			return nil, nil, false
		}
	}
	sum, validated := ctaValidated(ctaValidate(total, n.st, env))
	return sum, n.st, validated
}

// ctaSumItem is n's value converted into c on ctaTypedAttrItem's terms, from
// its dynamic type (ctaSum.total): one item, or ctaRaised where the call
// raises.
func ctaSumItem(n ctaSum, c *xsd.SimpleType, env ctaEnv) ctaItem {
	v, dynamic, ok := n.total(env)
	if !ok {
		return ctaRaised{}
	}
	return ctaPromote(v, dynamic, c, env)
}

// ctaSumInstanceItem evaluates n and converts the xs:boolean it returns into
// c on ctaMatchItem's terms: whether the dynamic type of the fn:sum call's
// value is n.atomic or derived from it, walked up its {base type definition}
// chain at evaluation (ctaTypes.ancestor, over the evaluation's resolver). A
// call that raises raises, as evaluating any operand does.
func ctaSumInstanceItem(n ctaSumInstance, c *xsd.SimpleType, env ctaEnv) ctaItem {
	_, dynamic, ok := n.sum.total(env)
	if !ok {
		return ctaRaised{}
	}
	at, err := ctaTypes{resolver: env.types}.ancestor(dynamic, n.atomic)
	if err != nil {
		return ctaRaised{}
	}
	return ctaConvert(strconv.FormatBool(at != nil), n.st, c, env)
}

// ctaPrefixMember is a general comparison by `=` (xpath20.md §3.5.2) one of
// whose operands is a call to fn:in-scope-prefixes over E (xpath-functions.md
// §11.2.6), `in-scope-prefixes(.)`, and the other a StringLiteral or a
// parenthesized sequence of them, on either side (ctaParser.prefixMember): the
// existential over the pairs, decided as whether any of literals is one of
// the prefixes the call returns (ctaInScopePrefix) — the call's items are
// xs:NCName, which B.2 compares with an xs:string under the codepoint
// collation, so a pair holds exactly where the two strings are equal. context
// is what the façade compiles `.` to as a node, on ctaNamespaceURI's terms.
//
// It is a boolean node of its own, and the call is no ctaValue, because the
// sequence the call returns is never built: [ContextElement] answers whether
// one prefix is bound and lists none.
type ctaPrefixMember struct {
	context  ctaNodeArg
	literals []string
}

func (ctaPrefixMember) ctaExpr() {}

// eval decides n: true where some literal is a prefix of E's [in-scope
// namespaces], in written order, and the err:XPDY0002 of a facet's absent
// context item.
func (n ctaPrefixMember) eval(env ctaEnv) ctaAnswer {
	e, present := ctaContextElementOf(n.context, env)
	if !present {
		return ctaError
	}
	for _, p := range n.literals {
		if ctaInScopePrefix(e, p) {
			return ctaTrue
		}
	}
	return ctaFalse
}

// ctaInScopePrefix reports whether p is one of the prefixes fn:in-scope-prefixes
// returns for e (xpath-functions.md §11.2.6), which are those of e's [in-scope
// namespaces] (xml-infoset.md:169), its inherited bindings included:
//
//   - "xml" always: it is in every element's [in-scope namespaces], answered
//     here and never asked of e;
//   - "xmlns" never, nor any other non-empty string that is no NCName, neither
//     of which a binding can be under; e is not asked about either;
//   - any other prefix, the zero-length one (the default namespace) included,
//     where e answers it a non-empty namespace name. No prefix is ever bound
//     to a zero-length one, and the empty prefix answers ok true with no
//     default namespace in scope, so uri alone decides and ok is not read.
func ctaInScopePrefix(e ContextElement, p string) bool {
	switch {
	case p == "xml":
		return true
	case p == "xmlns" || ctaScanNCName(p, 0) != len(p):
		return false
	}
	uri, _ := e.LookupPrefix(p)
	return uri != ""
}

// ctaContextElementOf is E as fn:namespace-uri and fn:in-scope-prefixes read
// it: the evaluation's [ContextElement] where context is ctaContextNode, and
// false — err:XPDY0002 — where it is an assertions facet's ctaAbsentNode,
// whose evaluation holds no element. The input is ctaTypedInput by
// construction: only the assertion and facet façades call the library
// (ctaFacade.callsLibrary), and the other arm holds no element and raises,
// unreachably. The default arm is unreachable, ctaNodeArg being sealed over
// the two arms named, and raises.
func ctaContextElementOf(context ctaNodeArg, env ctaEnv) (ContextElement, bool) {
	switch context.(type) {
	case ctaContextNode:
		in, typed := env.input.(ctaTypedInput)
		if !typed {
			return nil, false
		}
		return in.node, true
	case ctaAbsentNode:
		return nil, false // err:XPDY0002
	default:
		return nil, false
	}
}

// ctaNamespaceURIItem is n's xs:anyURI, E's [namespace name], converted into c
// on ctaMatchItem's terms, and the error its context raises.
func ctaNamespaceURIItem(n ctaNamespaceURI, c *xsd.SimpleType, env ctaEnv) ctaItem {
	e, present := ctaContextElementOf(n.context, env)
	if !present {
		return ctaRaised{}
	}
	return ctaConvert(e.Name().Space, n.st, c, env)
}

// resultType is the ·expanded name· of the type op returns.
func (op ctaUnaryStringOp) resultType() xsd.QName {
	if op == ctaStringLength {
		return ctaBuiltin("integer")
	}
	return ctaBuiltin("string")
}

// holds reports whether s matches sub as op asks, code point by code point:
// anywhere for fn:contains, at the start for fn:starts-with and at the end for
// fn:ends-with. A zero-length sub matches every s, and no other sub matches a
// zero-length s, which are the two empty-string rules §7.5.1–7.5.3 state, in
// the order they state them.
func (op ctaMatchOp) holds(s, sub string) bool {
	switch op {
	case ctaContains:
		return strings.Contains(s, sub)
	case ctaStartsWith:
		return strings.HasPrefix(s, sub)
	case ctaEndsWith:
		return strings.HasSuffix(s, sub)
	}
	return false // op is one of the three constants; never reached
}

// ctaStringOf is the one xs:string a evaluates to, reporting false where it
// raises: the empty sequence is the zero-length string, which is how every
// xs:string? parameter of this grammar reads it (§7.4.4, §7.4.5, §7.5.1–7.5.3);
// two or more items are err:XPTY0004, and so is any item of a mistyped
// argument (xpath20.md §3.1.5); an operand that raised stays raised. The string
// is the item's ·canonical representation·, read through [value.Canonical] on
// holdsCollated's terms — the identity for the xs:string family and for
// xs:anyURI, which §3.1.5's URI promotion makes an xs:string.
//
// The operand is read in its own static type, so no item is converted here:
// stringArgument cast an xs:untypedAtomic one to xs:string at compile time. The
// statically empty operand holds no item. An xs:untypedAtomic static type is
// raised, since no argument stringArgument built has one.
func ctaStringOf(a ctaStringArgument, env ctaEnv) (string, bool) {
	switch s := ctaStaticOf(a.operand).(type) {
	case ctaEmptySequence:
		return "", true
	case ctaTyped:
		atoms, converted := ctaItemOf(a.operand, s.st, env).(ctaAtoms)
		if !converted {
			return "", false
		}
		if len(atoms.vs) == 0 {
			return "", true
		}
		if len(atoms.vs) > 1 || a.mistyped {
			return "", false // err:XPTY0004
		}
		str, renders := atoms.vs[0].(value.Canonical)
		if !renders {
			return "", false
		}
		return str.Canonical(), true
	case ctaUntypedAtomic:
		return "", false
	}
	return "", false // ctaStatic has the three arms above; never reached
}

// ctaMatchItem evaluates n, its left argument first, and converts the
// xs:boolean it returns into c: an argument that raises is the error, and the
// result is validated against n.st and promoted, on ctaArithItem's terms.
func ctaMatchItem(n ctaMatch, c *xsd.SimpleType, env ctaEnv) ctaItem {
	s, ok := ctaStringOf(n.left, env)
	if !ok {
		return ctaRaised{}
	}
	sub, ok := ctaStringOf(n.right, env)
	if !ok {
		return ctaRaised{}
	}
	return ctaConvert(strconv.FormatBool(n.op.holds(s, sub)), n.st, c, env)
}

// ctaUnaryStringItem evaluates n and converts its result into c on
// ctaMatchItem's terms: fn:string-length counts CODE POINTS ("the length in
// characters", §7.4.4), and fn:normalize-space strips leading and trailing
// whitespace and folds each inner run into one #x20, the whitespace being XML's
// S production alone (§7.4.5, ctaNormalizedSpace).
func ctaUnaryStringItem(n ctaUnaryString, c *xsd.SimpleType, env ctaEnv) ctaItem {
	s, ok := ctaStringOf(n.arg, env)
	if !ok {
		return ctaRaised{}
	}
	if n.op == ctaStringLength {
		return ctaConvert(strconv.Itoa(utf8.RuneCountInString(s)), n.st, c, env)
	}
	return ctaConvert(ctaNormalizedSpace(s), n.st, c, env)
}

// ctaNormalizedSpace is fn:normalize-space over s: its runs of XML S — #x20,
// #x9, #xD and #xA, and nothing else, so neither U+0085 nor U+00A0 is
// whitespace here — dropped at either end and folded to one #x20 between.
func ctaNormalizedSpace(s string) string {
	return strings.Join(strings.FieldsFunc(s, ctaXMLSpace), " ")
}

// ctaXMLSpace reports whether r is one of the four characters of XML 1.0's S
// production.
func ctaXMLSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

// ctaPresenceItem evaluates n and converts the xs:boolean it returns into c on
// ctaMatchItem's terms: fn:empty is true and fn:exists false exactly where the
// operand is the empty sequence. An operand that raises is the error.
func ctaPresenceItem(n ctaPresence, c *xsd.SimpleType, env ctaEnv) ctaItem {
	length, ok := ctaSequenceLength(n.operand, env)
	if !ok {
		return ctaRaised{}
	}
	return ctaConvert(strconv.FormatBool((length == 0) == (n.op == ctaEmptyTest)), n.st, c, env)
}

// ctaStringFunctionItem evaluates fn:string's cast and converts the xs:string
// it returns into c on ctaMatchItem's terms: the cast of the empty sequence is
// the empty sequence, which fn:string returns as the zero-length string
// (§2.3), and every other outcome — one value, or the err:XPTY0004 of two or
// more items (ctaCastItem) — is the cast's.
func ctaStringFunctionItem(n ctaStringFunction, c *xsd.SimpleType, env ctaEnv) ctaItem {
	cast, converted := ctaCastItem(n.cast, n.cast.target, env).(ctaAtoms)
	if !converted {
		return ctaRaised{}
	}
	if len(cast.vs) == 0 {
		return ctaConvert("", n.cast.target, c, env)
	}
	return ctaPromote(cast.vs[0], n.cast.target, c, env)
}

// ctaConcatItem evaluates n and converts the xs:string it returns into c on
// ctaMatchItem's terms: each argument's string, read by ctaStringOf from the
// fn:string call it is held as, joined in written order. An argument that
// raises — err:XPTY0004 for two or more items, the error of its own operand —
// is the error, and the arguments after it are not evaluated.
func ctaConcatItem(n ctaConcat, c *xsd.SimpleType, env ctaEnv) ctaItem {
	var b strings.Builder
	for _, arg := range n.args {
		s, ok := ctaStringOf(ctaStringArgument{operand: arg}, env)
		if !ok {
			return ctaRaised{}
		}
		b.WriteString(s)
	}
	return ctaConvert(b.String(), n.st, c, env)
}

// ctaSubstringAfterItem evaluates n, its left argument first, and converts the
// xs:string it returns into c on ctaMatchItem's terms: an empty argument is the
// zero-length string (ctaStringOf), a zero-length right yields left whole, a
// right left does not contain yields the zero-length string, and otherwise the
// result is what follows right's first occurrence in left, matched code point
// by code point (xpath-functions.md §7.5.5 under the default collation).
func ctaSubstringAfterItem(n ctaSubstringAfter, c *xsd.SimpleType, env ctaEnv) ctaItem {
	s, ok := ctaStringOf(n.left, env)
	if !ok {
		return ctaRaised{}
	}
	sub, ok := ctaStringOf(n.right, env)
	if !ok {
		return ctaRaised{}
	}
	_, after, found := strings.Cut(s, sub)
	if !found {
		after = ""
	}
	return ctaConvert(after, n.st, c, env)
}

// ctaNumberItem evaluates n and converts the xs:double it returns into c on
// ctaMatchItem's terms (xpath-functions.md §14.4): the operand's items, read
// in n.read, raise where the operand raises, and two or more are err:XPTY0004
// (xpath20.md §3.1.5); none is NaN, and so is one whose cast to xs:double
// (§17.1.3.2, ctaPromote) fails — "If the conversion to xs:double fails, the
// xs:double value NaN is returned".
func ctaNumberItem(n ctaNumber, c *xsd.SimpleType, env ctaEnv) ctaItem {
	atoms, ok := ctaItemOf(n.operand, n.read, env).(ctaAtoms)
	if !ok || len(atoms.vs) > 1 {
		return ctaRaised{}
	}
	if len(atoms.vs) == 0 {
		return ctaConvert("NaN", n.st, c, env)
	}
	d, converted := ctaValidated(ctaPromote(atoms.vs[0], n.read, n.st, env))
	if !converted {
		return ctaConvert("NaN", n.st, c, env)
	}
	return ctaPromote(d, n.st, c, env)
}

// ctaCurrentDateLayout renders a [time.Time] as the xs:date lexical of its
// date in its own UTC offset: the timezoneFrag is Z for the zero offset and
// ±hh:mm otherwise, which is §17.1.5's cast of an xs:dateTime to xs:date.
const ctaCurrentDateLayout = "2006-01-02Z07:00"

// ctaCurrentDateItem evaluates n — the date of the evaluation's current
// dateTime, as ctaCurrentDateInstant settles its offset, rendered by
// ctaCurrentDateLayout — and converts it into c on ctaMatchItem's terms. The
// input is ctaTypedInput by construction: only the assertion and facet
// façades call the library (ctaFacade.callsLibrary), and the other arm holds
// no instant and raises, unreachably.
func ctaCurrentDateItem(n ctaCurrentDate, c *xsd.SimpleType, env ctaEnv) ctaItem {
	in, typed := env.input.(ctaTypedInput)
	if !typed {
		return ctaRaised{}
	}
	return ctaConvert(ctaCurrentDateInstant(in.now).Format(ctaCurrentDateLayout), n.st, c, env)
}

// ctaCurrentDateInstant is now where a timezoneFrag spells its offset — a whole
// number of minutes from -14:00 to +14:00 (xmlschema11-2 timezoneFrag) — and
// now.UTC() otherwise, so an unspellable offset renders no wrong or invalid
// lexical.
func ctaCurrentDateInstant(now time.Time) time.Time {
	_, off := now.Zone()
	if off%60 != 0 || off > 14*3600 || off < -14*3600 {
		return now.UTC()
	}
	return now
}

// ctaDistinctValuesItem evaluates n (ctaDistinctValues.eval) and converts
// each surviving item, read in n.st, into c on ctaPromoted's terms: the
// identity where c is that type, and for an xs:untypedAtomic operand the cast
// of its xs:string to c, which is the cast of the xs:untypedAtomic item itself.
func ctaDistinctValuesItem(n ctaDistinctValues, c *xsd.SimpleType, env ctaEnv) ctaItem {
	vs, ok := n.eval(env)
	if !ok {
		return ctaRaised{}
	}
	return ctaPromoted(vs, n.st, c, env)
}

// eval is fn:distinct-values over n's operand (xpath-functions.md §15.1.6):
// its items read in n.st, in order, each kept unless it is the same as
// one kept before it, reporting false where the operand raises. Which of two
// equal items survives, and in what order, is ·implementation dependent·;
// this keeps the first, in the operand's order (STYLE D1).
//
// Two items are the same where eq holds between them in that type's {primitive
// type definition} (ctaHoldsPair) — so 0 and -0 are one, a date or time without a
// timezone is compared under the implicit timezone, and xs:string items under
// the codepoint collation, which is the default collation xpath-valid clause
// 2.2.10 fixes — and where both are NaN, each unequal to itself ([value.Eq]),
// which §15.1.6 makes one item although `NaN eq NaN` is false. A pair eq does
// not decide, as for a value with no equality, is two items: §15.1.6 makes
// values eq is not defined for distinct, never an error.
func (n ctaDistinctValues) eval(env ctaEnv) ([]value.Value, bool) {
	p, err := n.st.Primitive(env.types)
	if err != nil || p == nil {
		return nil, false
	}
	atoms, converted := ctaItemOf(n.operand, n.st, env).(ctaAtoms)
	if !converted {
		return nil, false
	}
	var kept, keys []value.Value
	for _, v := range atoms.vs {
		key, ok := ctaValidated(ctaPromote(v, n.st, p, env))
		if !ok {
			return nil, false
		}
		if ctaHoldsAny(keys, key, p, env) {
			continue
		}
		kept = append(kept, v)
		keys = append(keys, key)
	}
	return kept, true
}

// ctaHoldsAny reports whether key, a value of the primitive p, is the same as
// any of keys on ctaDistinctValues.eval's terms.
func ctaHoldsAny(keys []value.Value, key value.Value, p *xsd.SimpleType, env ctaEnv) bool {
	for _, k := range keys {
		if ctaHoldsPair(ctaEqual, p, k, key, env) == ctaTrue || (ctaNaN(k) && ctaNaN(key)) {
			return true
		}
	}
	return false
}

// ctaNaN reports whether v is unequal to itself, which [value.Eq] states of
// NaN alone, on ctaBooleanOf's terms.
func ctaNaN(v value.Value) bool {
	eq, comparable := v.(value.Eq)
	return comparable && !eq.Eq(v)
}

// nodes is how many items c's operand evaluates to (ctaSequenceLength), which
// is what fn:count returns for it.
func (c ctaCountedItems) nodes(env ctaEnv) (int, bool) { return ctaSequenceLength(c.operand, env) }

// ctaSequenceLength is how many items v evaluates to, without atomizing it —
// the question fn:count, fn:empty and fn:exists ask of their `item()*`
// argument — reporting false where it raises. A step is the number of nodes it
// selects, which ctaStep.nodes answers for both this and the ·effective
// boolean value·; `$value` over ·special· content is one xs:untypedAtomic value
// or, unbound, none (ctaUntypedBinding); an fn:distinct-values call is the
// items it keeps, whatever its static type (ctaDistinctValues.eval); the
// context item `.` (ctaContextAtom) is one item, though it is statically
// untyped; the statically empty sequence is none; and every other operand is
// the length of its items in its own static type, which converts none of them.
func ctaSequenceLength(v ctaValue, env ctaEnv) (int, bool) {
	if step, isStep := v.(ctaStep); isStep {
		return step.nodes(env)
	}
	if distinct, isDistinct := v.(ctaDistinctValues); isDistinct {
		vs, ok := distinct.eval(env)
		return len(vs), ok
	}
	if _, untyped := v.(ctaUntypedValue); untyped {
		_, bound, ok := ctaUntypedBinding(env)
		if !bound {
			return 0, ok
		}
		return 1, ok
	}
	if _, isContext := v.(ctaContextAtom); isContext {
		return 1, true
	}
	typed, isTyped := ctaStaticOf(v).(ctaTyped)
	if !isTyped {
		return 0, true // ctaEmptyValue, the statically empty sequence
	}
	atoms, ok := ctaItemOf(v, typed.st, env).(ctaAtoms)
	return len(atoms.vs), ok
}

// ctaStep is a ctaValue that evaluates to a sequence of NODES rather than of
// atomic values: an attribute step, untyped (ctaAttr) or typed (ctaTypedAttr),
// a child-axis step, typed (ctaTypedChild) or over children of mixed content
// (ctaUntypedChild), an element step whose existence is asked
// (ctaSelectedElements), a path of child steps (ctaChildPath), a child step
// filtered by its children's existence (ctaChildrenHaving) or by a
// `preceding::` step (ctaChildrenPreceded), the candidate `.` inside a value
// predicate (ctaCandidate), the range variable of a quantifier over a typed
// child step and `$v/following-sibling::M[1]` over it (ctaBoundChild,
// ctaBoundSibling), an attribute step over either (ctaBoundAttribute), and the
// three that raise before they select a node, a rooted path
// (ctaNoDocumentRoot), a read of an absent context item (ctaNoContextItem) and
// an fn:doc call (ctaNoDocument). Its nodes method is the ONE reading of node
// existence, which the ·effective boolean value· of a step
// (ctaEffectiveBoolean.eval) and fn:empty and fn:exists (ctaSequenceLength)
// both take; every other operand's items are atomic values the caller reads
// its own way. A quantifier's range variable over `$value` (ctaRangeItem) is
// one such atomic value and no step.
type ctaStep interface {
	ctaValue
	// nodes is how many nodes the step selects, a ·nilled· child counting as a
	// node, reporting false where it raises: a rooted path (err:XPDY0050), a
	// read of an absent context item (err:XPDY0002), an fn:doc call
	// (err:FODC0005), a matched value breaking the caller's obligation
	// ([TypedAttributes], [ChildElements]), and a [Tally] holding no counter
	// for an element step or a child path, which [AssertionTest.Evaluate]
	// refuses before the tree is read.
	nodes(env ctaEnv) (int, bool)
}

func (s ctaAttr) nodes(env ctaEnv) (int, bool) {
	matched, ok := ctaMatchedAttributes(s, env)
	return len(matched), ok
}

func (s ctaTypedAttr) nodes(env ctaEnv) (int, bool) {
	matched, ok := ctaMatchedTyped(s, env)
	return len(matched), ok
}

func (s ctaTypedChild) nodes(env ctaEnv) (int, bool) {
	_, nodes, ok := ctaMatchedChildren(s, env)
	return nodes, ok
}

func (s ctaUntypedChild) nodes(env ctaEnv) (int, bool) {
	_, nodes, ok := ctaMatchedUntypedChildren(s, env)
	return nodes, ok
}

// nodes is the counter the evaluation's [Tally] holds for s (ctaTalliedNodes).
func (s ctaChildPath) nodes(env ctaEnv) (int, bool) { return ctaTalliedNodes(s, env) }

// nodes is the counter the evaluation's [Tally] holds for h (ctaTalliedNodes).
func (h ctaChildrenHaving) nodes(env ctaEnv) (int, bool) { return ctaTalliedNodes(h, env) }

// nodes is the counter the evaluation's [Tally] holds for p (ctaTalliedNodes).
func (p ctaChildrenPreceded) nodes(env ctaEnv) (int, bool) { return ctaTalliedNodes(p, env) }

// nodes is the counter the evaluation's [Tally] holds for s's path
// (ctaTalliedNodes).
func (s ctaSelectedElements) nodes(env ctaEnv) (int, bool) { return ctaTalliedNodes(s.path, env) }

// nodes is the counter the evaluation's [Tally] holds for p (ctaTalliedNodes),
// which is how many nodes fn:count over p counts.
func (p ctaCountPath) nodes(env ctaEnv) (int, bool) { return ctaTalliedNodes(p, env) }

// nodes is ctaCountPath.nodes'.
func (f ctaFilteredChildren) nodes(env ctaEnv) (int, bool) { return ctaTalliedNodes(f, env) }

// nodes is ctaCountPath.nodes'.
func (u ctaUnion) nodes(env ctaEnv) (int, bool) { return ctaTalliedNodes(u, env) }

// nodes is how many of E's children named m.name its predicate is true for,
// each child in document order the context item (ctaEnv.candidate): its typed
// value, or none for a ·nilled· one (ctaEachChild). A predicate that raises
// over any child raises for the whole count, and so does a yielded value
// breaking the obligation [ChildElements] states.
func (m ctaMatchingChildren) nodes(env ctaEnv) (int, bool) {
	n := 0
	ok := ctaEachChild(m.name, env, func(vs []value.Value) bool {
		inner := env
		inner.candidate = vs
		switch ctaEval(m.pred, inner) {
		case ctaTrue:
			n++
		case ctaError:
			return false
		case ctaFalse:
		}
		return true
	})
	return n, ok
}

// nodes is 1: the candidate is one node, ·nilled· or not.
func (ctaCandidate) nodes(ctaEnv) (int, bool) { return 1, true }

// nodes is 1: the bound child is one node, ·nilled· or not (ctaBoundNodes).
func (ctaBoundChild) nodes(env ctaEnv) (int, bool) { return ctaBoundNodes(env.rangeChild.bound()) }

// nodes is 1 where a later child bears the step's name and 0 where none does
// (ctaBoundNodes).
func (s ctaBoundSibling) nodes(env ctaEnv) (int, bool) {
	return ctaBoundNodes(env.rangeChild.next(s.step.name))
}

// nodes is 1 where the child the owner selects has the attribute, and 0 where
// the owner selects none or the child has none so named
// (ctaBoundAttributeValues).
func (s ctaBoundAttribute) nodes(env ctaEnv) (int, bool) {
	vs, ok := ctaBoundAttributeValues(s, env)
	return len(vs), ok
}

// ctaTalliedNodes is the counter the evaluation's [Tally] holds for key, which
// the caller filled with E's subtree. The input is ctaTypedInput by
// construction (ctaInput); the other arm holds no Tally and raises,
// unreachably.
func ctaTalliedNodes(key ctaKey, env ctaEnv) (int, bool) {
	in, typed := env.input.(ctaTypedInput)
	if !typed {
		return 0, false
	}
	return in.counts.count(key)
}

func (ctaNoDocumentRoot) nodes(ctaEnv) (int, bool) { return 0, false } // err:XPDY0050
func (ctaNoContextItem) nodes(ctaEnv) (int, bool)  { return 0, false } // err:XPDY0002
func (ctaNoDocument) nodes(ctaEnv) (int, bool)     { return 0, false } // err:FODC0005
