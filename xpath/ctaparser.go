package xpath

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kud360/goxsd8/regex"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// This file is the lexer and the recursive-descent parser for the §3.12.6
// required subset, one of each (STYLE T4, xpath/doc.go's "There is never a
// second, lenient parser"), plus the productions beyond it an assertion's
// {test} reaches: xpath20.md [23] ValueComp, [44] VarRef, an abbreviated
// child-axis step whose NodeTest is a QName ([31] AbbrevForwardStep, §3.2.1.1),
// a "/" or "//" opening [25] PathExpr over one such step or one attribute step,
// a [26] RelativePathExpr of two or more such steps, the last of which may be
// the [37] Wildcard `*`, one element step `N`, `./N` or `.//N`, or a child step
// filtered by a [40] Predicate that is a conjunction of child steps or one
// `preceding::` step, standing as the whole operand of fn:exists, fn:empty or
// an ·effective boolean value· (childPath, selectedElements, childrenHaving,
// childrenPreceded), an fn:count call ([48] FunctionCall) over one counted
// path, a [40] Predicate on a child step in it (predicate) or a [21] UnionExpr
// of such paths (countArgument), or over an operand a library call takes as its
// argument (countCall), a call to one of the F&O string and sequence functions
// (libraryCall) whose arguments are additive expressions or `()`, a constructor
// function whose operand is such an argument (constructorOperand), a call to
// fn:namespace-uri over `.` or with no argument (namespaceURICall) and, as an
// operand of `=` against string literals, to fn:in-scope-prefixes over `.`
// (prefixMember), each `.` there E as a node (contextNodeArgument), the binary
// operators of [13] AdditiveExpr and [14] MultiplicativeExpr, [47]
// ContextItemExpr `.`, which the assertion and facet façades admit, and the
// predicate façade reads as its candidate (valuePredicate), [7] IfExpr wherever
// an ExprSingle stands whole in a boolean position (exprSingle), [6]
// QuantifiedExpr over one child step whose body tests the bound child's
// attributes or next sibling element, over one typed child step, filtered by
// `[position() lt last()]` or not (rangeBinding), whose body is any ExprSingle
// reading the bound child and `$v/following-sibling::M[1]` (childQuantified,
// boundSibling), or over the items of a typed `$value` whose body is any
// ExprSingle, in the same positions (quantifiedExpr), [18] CastableExpr's
// `castable as` tail (castableTail), [16] InstanceofExpr's `instance of` tail
// with an atomic SequenceType, over an fn:data call among others
// (instanceofExpr), and, as a general comparison's operand, an integer
// sequence, [11] RangeExpr or §3.3.1's comma sequence over IntegerLiterals
// (integerSequence), or a string sequence, §3.3.1's comma sequence over
// StringLiterals (stringSequence; ctasequence.go) — each behind the façade
// (ctaFacade.comparesValues, ctaFacade.variable, ctaFacade.child,
// ctaFacade.childPath, ctaFacade.childrenHaving, ctaFacade.childrenPreceded,
// ctaFacade.elements, ctaFacade.quantified, ctaFacade.rangeScope,
// ctaFacade.childScope, ctaFacade.boundSibling, ctaFacade.rooted,
// ctaFacade.count, ctaFacade.callsLibrary, ctaFacade.computes,
// ctaFacade.contextItem, ctaFacade.contextNode, ctaFacade.focus,
// ctaFacade.conditional, ctaFacade.constructsSequences, ctaFacade.castable,
// ctaFacade.instanceOf), so a Type Alternative's {test} reaches none of them.
// Every method below is named for the production it parses, and the whole
// grammar is both reached and evaluated: no method here is a stub, and the
// production-level declines are those twenty-two façade methods'. xpath/doc.go
// owns the enumeration of what declines; every other decline reaching this file
// is ctaTypes answering ctaTypeDeclined for a comparison type, a cast target or
// a cast operand it will not serve, ctaTypes.arithmetic declining an operand
// pair, ctaTypes.instanceItem and ctaTypes.itemMatches declining an `instance
// of` operand or AtomicType, a library call of an arity its function does not
// have, a predicate or a union operand outside the shapes predicate,
// valuePredicate, childrenHaving, childrenPreceded and ctaUnionOf admit, a
// quantified expression outside the shapes quantifiedExpr admits, `.` or
// another node standing as a node (booleanExpr, presenceCall, instanceofExpr),
// a sequence sequenceLength does not measure or integerSequence does not build,
// or the façade declining a NameTest, a variable's type or a settled comparison
// type, which the production that asked propagates unchanged.

// ctaFunctionNS is the default function namespace of a {test}'s static context
// (xpath-valid clause 2.2.4, §3.13.6.2), which an unprefixed [12]
// ta-BooleanFunction name resolves in — so a bare not(...) is in the subset
// without any binding in the record's {namespace bindings}.
const ctaFunctionNS = "http://www.w3.org/2005/xpath-functions"

// ctaNotFunction is the ONE function name [12] ta-BooleanFunction may carry:
// §3.12.6 clause 3, "Any strings matching the BooleanFunction production are
// function calls to fn:not".
var ctaNotFunction = xsd.QName{Space: ctaFunctionNS, Local: "not"}

// ctaCountFunction is fn:count (xpath-functions.md §15.4.1), which a [14]
// ta-ValueExpr calls on the assertion and facet façades (ctaParser.countCall);
// the other functions it calls but the constructors are
// ctaParser.libraryCall's.
var ctaCountFunction = xsd.QName{Space: ctaFunctionNS, Local: "count"}

// ctaDataFunction is fn:data (xpath-functions.md §2.4), which the assertion
// and facet façades call as the operand of `instance of`
// (ctaParser.dataInstanceOf), and over `$value` as a quantifier's binding
// sequence (ctaParser.valueBinding), and nowhere else.
var ctaDataFunction = xsd.QName{Space: ctaFunctionNS, Local: "data"}

// ctaInScopePrefixesFunction is fn:in-scope-prefixes (xpath-functions.md
// §11.2.6), which the assertion and facet façades call over `.` as one operand
// of `=` and nowhere else (ctaParser.prefixMember).
var ctaInScopePrefixesFunction = xsd.QName{Space: ctaFunctionNS, Local: "in-scope-prefixes"}

// ctaNames holds the {namespace bindings} and the {default namespace} of one
// XPath Expression property record, the bindings indexed by prefix. The map is
// internal and never iterated into output (STYLE D2) — it is read by prefix and
// nothing else, and the diagnostic an unbound prefix produces names the prefix
// the parse walk reached, never one this map yielded.
//
// The record's {default namespace} is the default ELEMENT/TYPE namespace, so
// it answers for exactly two productions here: [15] ta-CastExpr's target QName,
// which xpath20.md §3.10.2 puts in it ("if the target type has no namespace
// prefix, it is considered to be in the default element/type namespace"), and
// the NameTest of a child-axis step (elementName). An absent {default
// namespace} is the empty string, which is the no-namespace answer both cases
// want then. An unprefixed attribute NameTest and an unprefixed function name
// take their own answers, and neither is this one.
type ctaNames struct {
	prefixes         map[string]string
	defaultNamespace string
}

// ctaUnresolvedName is the ·expanded name· an UNBOUND prefix resolves to, so
// that the parse continues far enough to decide whether the rest of the
// expression is a complete [8] ta-Test — which is the whole of what tells a
// static error apart from an unsupported construct.
//
// It never escapes into an evaluable tree: recording the defect and building
// this name are one step, compileCTATest reports that defect, and
// [CompileCTATest] withholds on any defect at all. No QName the grammar can
// write equals it either, since ctaScanNameTest admits no empty local part.
// The wildcard NameTest has no such uninhabited value to take and uses a sum
// arm instead (ctaUnresolvedTest).
var ctaUnresolvedName = xsd.QName{}

// attributeName resolves the QName arm of [17] ta-AttrName's NameTest to an
// ·expanded name·. An unprefixed one is in NO namespace: the attribute axis's
// principal node kind is attribute, never element, so xpath20.md §3.2.1.2's
// "otherwise, it has no namespace URI" applies and the {default namespace} is
// not consulted (PRINCIPLES 15).
func (p *ctaParser) attributeName(text string) xsd.QName {
	prefix, local, prefixed := strings.Cut(text, ":")
	if !prefixed {
		return xsd.QName{Local: text}
	}
	return p.prefixedName(prefix, local)
}

// wildcardTest resolves the WILDCARD arm of [17] ta-AttrName's NameTest —
// xpath20.md's [37], all three spellings — into the matcher it names.
//
// Only `NCName ':' '*'` resolves a prefix, and so only it can be err:XPST0081;
// `*` and `'*' ':' NCName` name no prefix at all and match across namespaces
// (§3.2.1.2).
func (p *ctaParser) wildcardTest(text string) ctaNameTest {
	prefix, local, prefixed := strings.Cut(text, ":")
	if !prefixed {
		return ctaAnyName{}
	}
	if prefix == "*" {
		return ctaAnySpace{local: local}
	}
	space, bound := p.prefixedSpace(prefix)
	if !bound {
		return ctaUnresolvedTest{}
	}
	return ctaAnyLocal{space: space}
}

// elementName resolves the QName NameTest of a child-axis step to an ·expanded
// name·. An unprefixed one takes the {default namespace}: the child axis's
// principal node kind is element, so xpath20.md §3.2.1.2 puts an unprefixed
// name test in "the default element/type namespace in the expression context",
// which xpath-valid clause 2.2.3 fixes as the {default namespace} — an
// assertion's xpathDefaultNamespace, already resolved (PRINCIPLES 15).
func (p *ctaParser) elementName(text string) xsd.QName {
	prefix, local, prefixed := strings.Cut(text, ":")
	if !prefixed {
		return xsd.QName{Space: p.names.defaultNamespace, Local: text}
	}
	return p.prefixedName(prefix, local)
}

// typeName resolves the QName naming a datatype on attributeName's terms,
// except that its unprefixed form takes the default ELEMENT/TYPE namespace
// (xpath20.md §3.10.2) rather than the no-namespace answer an attribute
// NameTest gets or the function-namespace one a function name gets.
//
// The err:XPST0081 an unbound prefix records here is always DISCARDED, because
// ctaTypes.castTarget resolves no type under ctaUnresolvedName and so declines
// the whole expression before the parse can reach the end of a [8] ta-Test.
// That is one more cast-target static condition this engine does not charge,
// under the marker ctaTypes.castTarget carries.
func (p *ctaParser) typeName(text string) xsd.QName {
	prefix, local, prefixed := strings.Cut(text, ":")
	if !prefixed {
		return xsd.QName{Space: p.names.defaultNamespace, Local: text}
	}
	return p.prefixedName(prefix, local)
}

// functionName resolves a function QName on attributeName's terms, except that
// its unprefixed form takes the default FUNCTION namespace (xpath-valid clause
// 2.2.4) rather than the no-namespace answer an attribute NameTest gets.
func (p *ctaParser) functionName(text string) xsd.QName {
	prefix, local, prefixed := strings.Cut(text, ":")
	if !prefixed {
		return xsd.QName{Space: ctaFunctionNS, Local: text}
	}
	return p.prefixedName(prefix, local)
}

// prefixedName resolves a PREFIXED QName against the {namespace bindings},
// recording err:XPST0081 for a prefix with no binding among them and yielding
// ctaUnresolvedName in its place.
func (p *ctaParser) prefixedName(prefix, local string) xsd.QName {
	space, bound := p.prefixedSpace(prefix)
	if !bound {
		return ctaUnresolvedName
	}
	return xsd.QName{Space: space, Local: local}
}

// prefixedSpace resolves a prefix to the NAMESPACE it is bound to, recording
// err:XPST0081 for one with no binding on prefixedName's terms and reporting ok
// false — [37]'s `NCName ':' '*'` carries no local part to pair it with, and no
// namespace URI is uninhabited, so the answer is the reported false and never a
// sentinel URI a real attribute could match (ctaUnresolvedTest).
func (p *ctaParser) prefixedSpace(prefix string) (string, bool) {
	space, bound := p.names.prefixes[prefix]
	if !bound {
		p.recordUnbound(prefix)
		return "", false
	}
	return space, true
}

// recordUnbound records the err:XPST0081 an unbound prefix is (xpath20.md
// Appendix G: "a static error if a QName used in an expression contains a
// namespace prefix that cannot be expanded into a namespace URI") and lets the
// parse go on. The code is the recorded verdict's own rule (ruleXPST0081) and
// its [xsderr.Loc] is the zero one, this package holding no schema position.
//
// The FIRST unbound prefix decides the message, so the answer is the one the
// walk reaches first in expression order and not a map's (STYLE D2).
//
// The record is PROVISIONAL: compileCTATest keeps it only where the parse then
// reached the end of a complete [8] ta-Test, because an expression outside the
// required subset is declined rather than charged, however its names resolve.
func (p *ctaParser) recordUnbound(prefix string) {
	if p.defect.kind != ctaNoDefect {
		return
	}
	p.defect = ctaDefect{
		kind: ctaStaticError,
		static: xsderr.New(ruleXPST0081, xsderr.Loc{},
			"no statically-known namespace binding for prefix %q", prefix),
	}
}

// ctaKind identifies one token of the subset's lexical structure.
type ctaKind byte

const (
	// ctaEOF is the sentinel past the last token; the token slice never holds
	// one, and ctaParser.peek synthesizes it.
	ctaEOF ctaKind = iota
	// ctaNameTok is an NCName or a prefixed QName, whose text is as written.
	// The keywords 'or', 'and', 'cast', 'castable', 'as' and the 'attribute'
	// axis name are this kind too: XPath has no reserved words, so what a name
	// means is the parser's to decide from position.
	ctaNameTok
	// ctaWildcardTok is one [37] Wildcard — `*`, `NCName ':' '*'` or
	// `'*' ':' NCName` — whose text is as written. It is its own kind and not a
	// ctaNameTok carrying a `*`, because ctaNameTok also carries the keywords and
	// the axis name and a wildcard reaches none of those positions. Five
	// productions read it: attrName as a NameTest, childPathLength as the bare
	// `*` NameTest of a child path's last step, after a '/', siblingBody as the
	// bare `*` after `following-sibling::`, and multiplicativeOperator and
	// occurrenceIndicator as a bare `*` following a complete operand or a
	// SequenceType's ItemType, positions no NameTest can take.
	ctaWildcardTok
	// ctaStringTok is a StringLiteral, whose text is its VALUE — quotes
	// stripped, doubled quotes folded to one.
	ctaStringTok
	// ctaNumberTok is a NumericLiteral, whose text is as written.
	ctaNumberTok
	ctaLParen
	ctaRParen
	// ctaAtTok is '@', the abbreviation ta-props-correct clause 2.1 spells
	// [17] ta-AttrName with.
	ctaAtTok
	// ctaAxisTok is '::', which only the unabbreviated attribute-axis form
	// clause 2.2 admits reaches, the preceding step childrenPreceded reads,
	// and, in a quantifier's body, the following-sibling and self steps
	// siblingBody reads and the following-sibling step boundSibling reads.
	ctaAxisTok
	// ctaQuestionTok is the '?' occurrence indicator of [15] ta-CastExpr.
	ctaQuestionTok
	// ctaCompTok is one operator of [13] ta-Comparator, whose text is its
	// spelling.
	ctaCompTok
	// ctaDollarTok is the '$' opening xpath20.md [44] VarRef, which only the
	// assertion and facet façades' `$value` reaches (ctaFacade.variable), and
	// which opens an fn:count argument that is no path (ctaParser.countsItems),
	// and a quantifier's range variable, where it is bound and where its body
	// names it (ctaParser.quantifiedExpr) — through the façade inside a body
	// over `$value` or a typed child step (ctaRangeFacade.variable,
	// ctaChildRangeFacade.variable), and opening `$v/following-sibling::M[1]`
	// there (ctaParser.boundSibling) — and `$value` as such a quantifier's
	// binding sequence (ctaParser.valueBinding).
	ctaDollarTok
	// ctaSlashTok is '/' and ctaSlashSlashTok is '//'. Each is read only where
	// it opens a [25] PathExpr (ctaParser.rootedPath) or follows the `.`
	// opening an fn:count argument or an element step whose existence is
	// asked (ctaParser.countPath), and a '/' also between the steps of a
	// child path (ctaParser.childPath) and after a quantifier's range variable
	// in its body (ctaParser.variableStep, ctaParser.boundSibling); anywhere
	// else it is a token no production takes, so `a//b`, and `a/b` outside a
	// child path's three positions, are not expressions here.
	ctaSlashTok
	ctaSlashSlashTok
	// ctaDotTok is a '.' that opens no NumericLiteral: the [47]
	// ContextItemExpr, which the assertion, facet and predicate façades admit
	// (ctaFacade.contextItem), or the context item an fn:count argument's or
	// an element step's `./` or `.//` opens with (ctaParser.countPath).
	// '..', the abbreviated parent step, is not tokenized at all.
	ctaDotTok
	// ctaPlusTok is '+' and ctaMinusTok is '-', the two operators of xpath20.md
	// [13] AdditiveExpr (ctaParser.additiveOperator). Neither can open a name or
	// a number, so no other token is read across one; a '-' INSIDE a name is an
	// NCName character and never this token, so `a-1` is one name.
	ctaPlusTok
	ctaMinusTok
	// ctaCommaTok is ',', which separates the arguments of a call to one of the
	// F&O functions the façade admits (ctaParser.arguments) and the members of
	// an integer or string sequence (ctaParser.sequence), and is read nowhere
	// else, so a comma in any other position is a token no production takes.
	ctaCommaTok
	// ctaLBracketTok is '[' and ctaRBracketTok is ']', which open and close
	// xpath20.md [40] Predicate, read only after a child step of an fn:count
	// argument (ctaParser.predicate), after the child step of a child-existence
	// conjunction (ctaParser.childrenHaving), after the child step a
	// `preceding::` step filters and after that step
	// (ctaParser.childrenPreceded), after the following-sibling step of a
	// quantifier's body (ctaParser.siblingBody, ctaParser.boundSibling), and
	// after a quantifier's binding step (ctaParser.positionWindow).
	ctaLBracketTok
	ctaRBracketTok
	// ctaBarTok is '|', xpath20.md [21] UnionExpr's operator, read only between
	// the operands of an fn:count argument (ctaParser.countArgument).
	ctaBarTok
)

// ctaToken is one token, identified by kind. text carries the source spelling
// for the three kinds that have one (name, number) or the decoded value
// (string), and is empty for the punctuation kinds.
type ctaToken struct {
	kind ctaKind
	text string
}

// ctaTokenize splits an {expression} into tokens, reporting false for text
// whose lexical structure the subset does not admit at all.
//
// White space between tokens is skipped per §3.12.6's Note ("[XPath 2.0]
// allows whitespace to be used between tokens ... even though this is not
// explicitly shown in the grammar"), and so are XPath comments, which nest
// (xpath20.md §A.2.4.1) and are legal wherever white space is.
func ctaTokenize(s string) ([]ctaToken, bool) {
	var toks []ctaToken
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case unicode.IsSpace(r):
			i += size
		case strings.HasPrefix(s[i:], "(:"):
			j, ok := ctaSkipComment(s, i)
			if !ok {
				return nil, false
			}
			i = j
		case r == '(':
			toks = append(toks, ctaToken{kind: ctaLParen})
			i++
		case r == ')':
			toks = append(toks, ctaToken{kind: ctaRParen})
			i++
		case r == '@':
			toks = append(toks, ctaToken{kind: ctaAtTok})
			i++
		case r == '?':
			toks = append(toks, ctaToken{kind: ctaQuestionTok})
			i++
		case r == '$':
			toks = append(toks, ctaToken{kind: ctaDollarTok})
			i++
		case r == '+':
			toks = append(toks, ctaToken{kind: ctaPlusTok})
			i++
		case r == '-':
			toks = append(toks, ctaToken{kind: ctaMinusTok})
			i++
		case r == ',':
			toks = append(toks, ctaToken{kind: ctaCommaTok})
			i++
		case r == '[':
			toks = append(toks, ctaToken{kind: ctaLBracketTok})
			i++
		case r == ']':
			toks = append(toks, ctaToken{kind: ctaRBracketTok})
			i++
		case r == '|':
			toks = append(toks, ctaToken{kind: ctaBarTok})
			i++
		case strings.HasPrefix(s[i:], "//"):
			toks = append(toks, ctaToken{kind: ctaSlashSlashTok})
			i += 2
		case r == '/':
			toks = append(toks, ctaToken{kind: ctaSlashTok})
			i++
		case r == ':':
			if !strings.HasPrefix(s[i:], "::") {
				return nil, false
			}
			toks = append(toks, ctaToken{kind: ctaAxisTok})
			i += 2
		case r == '=', r == '!', r == '<', r == '>':
			tok, j, ok := ctaScanComparator(s, i)
			if !ok {
				return nil, false
			}
			toks = append(toks, tok)
			i = j
		case r == '\'', r == '"':
			text, j, ok := ctaScanString(s, i)
			if !ok {
				return nil, false
			}
			toks = append(toks, ctaToken{kind: ctaStringTok, text: text})
			i = j
		case r >= '0' && r <= '9', r == '.':
			j := ctaScanNumber(s, i)
			if j > i {
				toks = append(toks, ctaToken{kind: ctaNumberTok, text: s[i:j]})
				i = j
				continue
			}
			if r != '.' || strings.HasPrefix(s[i:], "..") {
				return nil, false
			}
			toks = append(toks, ctaToken{kind: ctaDotTok})
			i++
		default:
			kind, j := ctaScanNameTest(s, i)
			if j == i {
				return nil, false
			}
			toks = append(toks, ctaToken{kind: kind, text: s[i:j]})
			i = j
		}
	}
	return toks, true
}

// ctaSkipComment reports the index past the comment opening at i, or false for
// one left unclosed. Comments nest, so the scan counts depth rather than
// stopping at the first ":)".
func ctaSkipComment(s string, i int) (int, bool) {
	depth := 0
	for i < len(s) {
		switch {
		case strings.HasPrefix(s[i:], "(:"):
			depth++
			i += 2
		case strings.HasPrefix(s[i:], ":)"):
			depth--
			i += 2
			if depth == 0 {
				return i, true
			}
		default:
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
		}
	}
	return 0, false
}

// ctaScanComparator reads one [13] ta-Comparator at i, longest first so '<='
// is never read as '<' followed by '='. '!' opens only '!='.
func ctaScanComparator(s string, i int) (ctaToken, int, bool) {
	for _, op := range []string{"!=", "<=", ">=", "=", "<", ">"} {
		if strings.HasPrefix(s[i:], op) {
			return ctaToken{kind: ctaCompTok, text: op}, i + len(op), true
		}
	}
	return ctaToken{}, 0, false
}

// ctaScanString reads the StringLiteral opening at i (xpath20.md [74]),
// returning its VALUE: the quote character may appear inside the literal only
// doubled, and a doubled pair denotes one such character.
func ctaScanString(s string, i int) (string, int, bool) {
	quote := s[i]
	var b strings.Builder
	for j := i + 1; j < len(s); {
		if s[j] != quote {
			_, size := utf8.DecodeRuneInString(s[j:])
			b.WriteString(s[j : j+size])
			j += size
			continue
		}
		if j+1 < len(s) && s[j+1] == quote {
			b.WriteByte(quote)
			j += 2
			continue
		}
		return b.String(), j + 1, true
	}
	return "", 0, false
}

// ctaScanNumber reports the end of the NumericLiteral starting at i, or i
// itself where none does. It admits xpath20.md's [71] IntegerLiteral, [72]
// DecimalLiteral and [73] DoubleLiteral, which share one scan: digits, an
// optional fractional part, an optional exponent — with at least one digit in
// the mantissa.
//
// No sign is admitted, and that is the grammar's doing rather than an
// omission: [16] ta-SimpleValue reaches a Literal directly, with no unary
// operator production between them, so "-1" is a ctaMinusTok and a number,
// and no production takes a ctaMinusTok where an operand opens.
func ctaScanNumber(s string, i int) int {
	j := ctaScanDigits(s, i)
	if j < len(s) && s[j] == '.' {
		j = ctaScanDigits(s, j+1)
	}
	if j == i || (j == i+1 && s[i] == '.') {
		return i
	}
	if j >= len(s) || (s[j] != 'e' && s[j] != 'E') {
		return j
	}
	k := j + 1
	if k < len(s) && (s[k] == '+' || s[k] == '-') {
		k++
	}
	end := ctaScanDigits(s, k)
	if end == k {
		return j // 'e' with no exponent digits is not part of the literal
	}
	return end
}

// ctaScanDigits reports the end of the run of ASCII digits at i.
func ctaScanDigits(s string, i int) int {
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i
}

// ctaScanNameTest reports the kind and the end of the [36] NameTest starting at
// i, or i itself where none does. One scanner reads both arms (STYLE T4),
// because they share their first NCName and differ only past the ':':
//
//   - `NCName` and `NCName ':' NCName` are ctaNameTok, the QName arm — and the
//     spelling every keyword and the axis name arrives under.
//   - `*`, `NCName ':' '*'` and `'*' ':' NCName` are ctaWildcardTok, [37]'s
//     three arms.
//
// The ':' is consumed only when a name or a `*` really follows it, so
// 'attribute::x' leaves the '::' for the axis token and 'attribute::*' leaves
// it for the axis token followed by a bare wildcard. No white space may sit
// inside a NameTest, which is what keeps `@p : name` out of the grammar.
func ctaScanNameTest(s string, i int) (ctaKind, int) {
	if s[i] == '*' {
		return ctaWildcardTok, ctaScanWildcardTail(s, i+1)
	}
	j := ctaScanNCName(s, i)
	if j == i || j >= len(s) || s[j] != ':' {
		return ctaNameTok, j
	}
	if j+1 < len(s) && s[j+1] == '*' {
		return ctaWildcardTok, j + 2
	}
	k := ctaScanNCName(s, j+1)
	if k == j+1 {
		return ctaNameTok, j
	}
	return ctaNameTok, k
}

// ctaScanWildcardTail reports the end of the optional `':' NCName` following a
// leading '*', or i itself where none does.
func ctaScanWildcardTail(s string, i int) int {
	if i >= len(s) || s[i] != ':' {
		return i
	}
	j := ctaScanNCName(s, i+1)
	if j == i+1 {
		return i
	}
	return j
}

// ctaNCNamePattern is the pattern Datatypes §3.4.7.1 fixes for xs:NCName,
// "[\i-[:]][\c-[:]]*", behind the '^' a prefix scan needs. It is a local copy
// of the generated builtin row's value rather than a production import of
// builtin, and TestCTANCNamePatternPinned fails if the two diverge (PRINCIPLES
// 26/27).
const ctaNCNamePattern = `^[\i-[:]][\c-[:]]*`

// ctaNCNameRE matches the longest NCName at the START of the string it is
// applied to — [Namespaces in XML] production [4], spelled as
// ctaNCNamePattern. It is translated and compiled once here through
// [regex.Translate], so the code points behind \i and \c are the ones the
// regex package owns and not a second table (PRINCIPLES 26/27; parser and
// builtin/strict reach the same class the same way, and regex/class.go records
// which edition of XML supplies it).
//
// The flavor is FO because only FO's '^' is a real anchor: FlavorXSD anchors
// the WHOLE string, which cannot express a prefix scan. This pattern carries
// no construct the two flavors read differently.
var ctaNCNameRE = func() *regexp.Regexp {
	goRE, err := regex.Translate(ctaNCNamePattern, regex.FlavorFO, "")
	if err != nil {
		panic("xpath: translating the NCName pattern: " + err.Error())
	}
	return regexp.MustCompile(goRE)
}()

// ctaScanNCName reports the end of the NCName starting at i, or i where none
// does. Its character classes are EXACT and not an approximation of the
// grammar: Datatypes §G.4.2.5 defines \i and \c by direct reference to XML's
// NameStartChar and NameChar, so the boundary reported here is the boundary
// XML draws — no character that terminates a name is read into one, and none
// that continues a name terminates it.
//
// The match is anchored at i, so its length IS the end it reports. A miss and
// an empty match are the same "" here, and mean the same thing: the pattern
// requires a NameStartChar, so it never matches empty.
func ctaScanNCName(s string, i int) int {
	return i + len(ctaNCNameRE.FindString(s[i:]))
}

// ctaParser is the recursive-descent parser over the token stream. It carries
// no visited set and no depth guard (STYLE D4): the grammar has no back edge,
// so a descent over a finite token slice terminates by construction.
type ctaParser struct {
	toks  []ctaToken
	pos   int
	names ctaNames
	types ctaTypes
	// facade compiles each [17] ta-AttrName, which is the one production whose
	// node differs between the façades, and admits each settled comparison type
	// (compileCTATest).
	facade ctaFacade
	// defect is what name resolution found wrong, which is the one defect kind
	// the walk itself has to carry: every other one is the false a production
	// returns. It lives here rather than on ctaNames because a value receiver
	// cannot keep it.
	defect ctaDefect
}

// peek reports the token at offset ahead of the cursor, or the EOF sentinel.
func (p *ctaParser) peek(ahead int) ctaToken {
	if p.pos+ahead >= len(p.toks) {
		return ctaToken{kind: ctaEOF}
	}
	return p.toks[p.pos+ahead]
}

// at reports whether the cursor sits on a token of kind k.
func (p *ctaParser) at(k ctaKind) bool { return p.peek(0).kind == k }

// atName reports whether the cursor sits on the unprefixed name text, which is
// how the keywords 'or', 'and', 'cast', 'castable', 'as' and the 'attribute'
// axis are recognized.
func (p *ctaParser) atName(text string) bool {
	return p.peek(0).kind == ctaNameTok && p.peek(0).text == text
}

// advance moves the cursor past one token.
func (p *ctaParser) advance() { p.pos++ }

// test parses [8] ta-Test, which is one OrExpr and then the end of the
// expression: trailing tokens are not a Test, however well the prefix parsed.
func (p *ctaParser) test() (ctaExpr, bool) {
	x, ok := p.exprSingle()
	if !ok {
		return nil, false
	}
	if !p.at(ctaEOF) {
		return nil, false
	}
	return x, true
}

// exprSingle parses xpath20.md [3] ExprSingle in each position it stands whole
// in a boolean position: the {test} itself, a parenthesized expression, fn:not's
// argument, and the test and branches of an IfExpr (ctaIf). Its [7] IfExpr arm
// opens with the unprefixed name `if` followed by '(' — a reserved function
// name (xpath20.md A.3), so that pair never opens a call — and every other opening
// is [9] ta-OrExpr's, but for [6] QuantifiedExpr's, which opens with the
// unprefixed name `some` or `every` followed by '$' (quantifiedExpr) — a pair
// no other production opens with. The comma of [2] Expr is no token this
// grammar takes, so an Expr of two or more ExprSingles declines.
func (p *ctaParser) exprSingle() (ctaExpr, bool) {
	if p.atName("if") && p.peek(1).kind == ctaLParen {
		return p.ifExpr()
	}
	if (p.atName("some") || p.atName("every")) && p.peek(1).kind == ctaDollarTok {
		return p.quantifiedExpr()
	}
	return p.orExpr()
}

// quantifiedExpr parses xpath20.md [6] QuantifiedExpr, `("some" | "every" )
// "$" VarName "in" ExprSingle "satisfies" ExprSingle`, whose keyword the
// cursor is on, over ONE in-clause, in one of two forms the binding sequence
// decides:
//
//   - over `data($value)` or `$value` (valueBinding), each item of a typed
//     `$value`, the body is one ExprSingle parsed under the façade
//     ctaFacade.rangeScope answers, the range variable in scope as an item of
//     `$value`'s item type, so `every $x in data($value) satisfies $x gt 0 and
//     $x lt 9` is one body; the node is ctaQuantifiedValue;
//   - over a child step (rangeBinding) unfiltered, whose body opens, after an
//     optional fn:not, as rangeBody's two forms do (atTalliedBody), the body
//     rangeBody desugars to the key counting the children satisfying it,
//     the fn:not moved outside by De Morgan's law: `every $v in N satisfies
//     not(B)` is `not(some $v in N satisfies B)` (ctaQuantifier.dual). The
//     node is p.facade's, which may decline it (ctaFacade.quantified);
//   - over a child step filtered by `[position() lt last()]`, or unfiltered
//     under any other body, the body is one ExprSingle parsed under the
//     façade ctaFacade.childScope answers, the range variable in scope as a
//     node of the step's type (childQuantified); the node is
//     ctaQuantifiedChildren.
//
// The VarName is a QName, resolved on varRef's terms, so an unprefixed one is
// in no namespace, and the body names it by its ·expanded name·. Its scope is
// the body alone (§3.9): the binding sequence is parsed under the outer
// façade, and so is everything after the body, so `$x` outside the body is
// still the err:XPST0008 that declines. Under a body rangeBody desugars it is
// bound in no static context, never resolved through ctaFacade.variable. A
// body ExprSingle runs as far as the grammar lets it, so over a child step
// `every $v in N satisfies $v/@a and @b` is one body, which declines: the
// parse stops before the `and`, a token no caller of exprSingle takes after
// an ExprSingle.
//
// GAP(xpath): every other QuantifiedExpr declines — a range variable named
// `$value`, which would shadow cvc-assertion clause 2.3's; two or more
// in-clauses; a binding sequence neither valueBinding nor rangeBinding
// parses, a binding predicate other than `[position() lt last()]` — `<`,
// `ne`, `!=`, `[last()]` among them — and one over a `$value` that is
// statically empty or ·special·; over a child step, a body opening as
// rangeBody's forms do that rangeBody does not parse, `or` among them, and a
// windowed binding under such a body; over a child step under any other body,
// a binding or `following-sibling::M[1]` child of mixed content, ·special·,
// list, union or no declared type, which no typed node reads
// (ctaFacade.child), and a path over the range variable other than
// `following-sibling::M[1]`; over `$value`, a body the range scope declines,
// a path over the range variable and `instance of` over it among them; and a
// nested quantifier in any body. The direction is the withhold
// [CompileAssertionTest] reports. (#1042)
func (p *ctaParser) quantifiedExpr() (ctaExpr, bool) {
	q := ctaSome
	if p.atName("every") {
		q = ctaEvery
	}
	p.advance() // 'some' or 'every'
	p.advance() // '$'
	if !p.at(ctaNameTok) {
		return nil, false
	}
	variable := p.attributeName(p.peek(0).text)
	p.advance()
	if variable == ctaValueName || !p.atName("in") {
		return nil, false
	}
	p.advance()
	if p.at(ctaDollarTok) || (p.at(ctaNameTok) && p.peek(1).kind == ctaLParen && p.functionName(p.peek(0).text) == ctaDataFunction) {
		return p.valueQuantified(q, variable)
	}
	name, window, ok := p.rangeBinding()
	if !ok || !p.atName("satisfies") {
		return nil, false
	}
	p.advance()
	if window || !p.atTalliedBody() {
		return p.childQuantified(q, variable, name, window)
	}
	negated := p.at(ctaNameTok) && p.peek(1).kind == ctaLParen && p.functionName(p.peek(0).text) == ctaNotFunction
	if negated {
		p.advance() // 'not'
		p.advance() // '('
		q = q.dual()
	}
	satisfying, ok := p.rangeBody(variable, name)
	if !ok {
		return nil, false
	}
	if negated {
		if !p.at(ctaRParen) {
			return nil, false
		}
		p.advance()
	}
	x, ok := p.facade.quantified(q, satisfying)
	if !ok || !negated {
		return x, ok
	}
	return ctaNot{operand: x}, true
}

// valueQuantified parses the rest of a quantifier over `$value`, q its
// quantifier and variable its range variable, from its binding sequence
// (valueBinding) on: `satisfies`, then the body under the façade
// ctaFacade.rangeScope answers for variable, which p.facade is again once the
// body is parsed.
func (p *ctaParser) valueQuantified(q ctaQuantifier, variable xsd.QName) (ctaExpr, bool) {
	over, ok := p.valueBinding()
	if !ok || !p.atName("satisfies") {
		return nil, false
	}
	p.advance()
	scoped, ok := p.facade.rangeScope(variable, over)
	if !ok {
		return nil, false
	}
	outer := p.facade
	p.facade = scoped
	body, parsed := p.exprSingle()
	p.facade = outer
	if !parsed {
		return nil, false
	}
	return ctaQuantifiedValue{q: q, over: over, body: body}, true
}

// valueBinding parses a quantifier's binding sequence where it is a call to
// fn:data over the [44] VarRef `$value` or that VarRef bare, compiled under
// p.facade (ctaFacade.variable), and reports the node `$value` compiled to.
// fn:data over an atomic sequence is that sequence (xpath-functions.md §2.4,
// xpath20.md §2.4.2), so both spellings are the one node. Every other VarRef,
// and a `$value` that is not a typed ctaValueVar — the statically empty one
// under a {content type} that is not simple, the ·special· one — declines
// under quantifiedExpr's GAP(xpath), and so does an fn:data call of any other
// argument or arity.
func (p *ctaParser) valueBinding() (ctaValueVar, bool) {
	called := p.at(ctaNameTok)
	if called {
		p.advance() // 'data'
		p.advance() // '('
	}
	if !p.at(ctaDollarTok) {
		return ctaValueVar{}, false
	}
	v, ok := p.varRef()
	if !ok {
		return ctaValueVar{}, false
	}
	if called {
		if !p.at(ctaRParen) {
			return ctaValueVar{}, false
		}
		p.advance()
	}
	over, typed := v.(ctaValueVar)
	return over, typed
}

// rangeBinding parses a quantifier's binding sequence as far as it is one
// abbreviated child-axis step with a QName NameTest, `N` or `./N`, resolved on
// elementName's terms like every child step, optionally filtered by the one
// predicate ctaPositionWindow spells, and reports N and whether that
// predicate is present. Anything else — a wildcard, `.//N`, `@N`, `.`, a longer
// path or any other predicate — leaves a token other than `satisfies` after
// it, which quantifiedExpr declines.
//
// The predicate is read as a fixed spelling and never through
// ctaFacade.focus, so fn:position and fn:last outside it keep their façade's
// answer: the assertion façade's decline, the facet façade's err:XPDY0002.
func (p *ctaParser) rangeBinding() (name xsd.QName, window, ok bool) {
	if p.at(ctaDotTok) {
		if p.peek(1).kind != ctaSlashTok {
			return xsd.QName{}, false, false
		}
		p.advance() // '.'
		p.advance() // '/'
	}
	if !p.at(ctaNameTok) {
		return xsd.QName{}, false, false
	}
	name = p.elementName(p.peek(0).text)
	p.advance()
	return name, p.positionWindow(), true
}

// ctaPositionWindow is the predicate rangeBinding reads after a binding step:
// `[position() lt last()]`, which selects every child the step selects but the
// last (xpath20.md §3.2.2; xpath-functions.md §16.1, §16.2).
var ctaPositionWindow = []ctaToken{
	{kind: ctaLBracketTok}, {kind: ctaNameTok, text: "position"}, {kind: ctaLParen}, {kind: ctaRParen},
	{kind: ctaNameTok, text: "lt"}, {kind: ctaNameTok, text: "last"}, {kind: ctaLParen}, {kind: ctaRParen},
	{kind: ctaRBracketTok},
}

// positionWindow consumes ctaPositionWindow where the tokens at the cursor
// spell it and reports whether they did, consuming nothing otherwise.
func (p *ctaParser) positionWindow() bool {
	for i, want := range ctaPositionWindow {
		if p.peek(i) != want {
			return false
		}
	}
	p.pos += len(ctaPositionWindow)
	return true
}

// atTalliedBody reports whether the body at the cursor opens, after an
// optional fn:not, `'$' VarName '/' '@'` or `'$' VarName '/'
// 'following-sibling' '::' '*'` — the openings of the two forms rangeBody
// desugars to [Tally] keys. Every other body is childQuantified's. Nothing is
// consumed.
func (p *ctaParser) atTalliedBody() bool {
	at := 0
	if p.at(ctaNameTok) && p.peek(1).kind == ctaLParen && p.functionName(p.peek(0).text) == ctaNotFunction {
		at = 2
	}
	if p.peek(at).kind != ctaDollarTok || p.peek(at+1).kind != ctaNameTok || p.peek(at+2).kind != ctaSlashTok {
		return false
	}
	if p.peek(at+3).kind == ctaAtTok {
		return true
	}
	return p.peek(at+3) == ctaSiblingSteps[0] && p.peek(at+4) == ctaSiblingSteps[1] && p.peek(at+5) == ctaSiblingSteps[2]
}

// childQuantified parses the rest of a quantifier over a child step, q its
// quantifier, variable its range variable, name the binding step's ·expanded
// name· and window whether `[position() lt last()]` filters it, from its body
// on: one ExprSingle parsed under the façade ctaFacade.childScope answers for
// variable over the step, which p.facade is again once the body is parsed. The
// step is compiled as a typed child step (ctaFacade.child), and declines
// unless it is a ctaTypedChild: its children are bound as nodes whose typed
// values the body may read. The node is ctaQuantifiedChildren.
func (p *ctaParser) childQuantified(q ctaQuantifier, variable, name xsd.QName, window bool) (ctaExpr, bool) {
	step, typed := p.facade.child(ctaExactName{name: name}, p.types)
	over, isTyped := step.(ctaTypedChild)
	if !typed || !isTyped {
		return nil, false
	}
	scoped, ok := p.facade.childScope(variable, over)
	if !ok {
		return nil, false
	}
	outer := p.facade
	p.facade = scoped
	body, parsed := p.exprSingle()
	p.facade = outer
	if !parsed {
		return nil, false
	}
	return ctaQuantifiedChildren{q: q, over: over, dropLast: window, body: body}, true
}

// rangeBody parses a quantifier's body over variable, bound to each child of
// E named name, into the key counting the children for which it is true, in
// one of two forms, each a path opening `$v/` (variableStep):
//
//   - `$v/@A`, or a conjunction `$v/@A and $v/@B …`, each a QName NameTest
//     resolved on attributeName's terms, whose ·effective boolean value· is
//     whether `$v` carries the attribute (xpath20.md §2.4.3 rule 2): the
//     children carrying every one, the key `count(N[@A and @B])` counts under
//     (ctaFilteredChildren);
//   - `$v/following-sibling::*[1][self::M]` (siblingBody).
func (p *ctaParser) rangeBody(variable, name xsd.QName) (ctaRangeKey, bool) {
	if !p.variableStep(variable) {
		return nil, false
	}
	if !p.at(ctaAtTok) {
		return p.siblingBody(name)
	}
	var required []xsd.QName
	for {
		if !p.at(ctaAtTok) || p.peek(1).kind != ctaNameTok {
			return nil, false
		}
		p.advance() // '@'
		required = append(required, p.attributeName(p.peek(0).text))
		p.advance()
		if !p.atName("and") || p.peek(1).kind != ctaDollarTok {
			break
		}
		p.advance() // 'and'
		if !p.variableStep(variable) {
			return nil, false
		}
	}
	return ctaFilteredChildrenOf(name, required)
}

// variableStep parses `'$' VarName '/'` where VarName resolves, on varRef's
// terms, to variable, reporting false for any other token or name.
func (p *ctaParser) variableStep(variable xsd.QName) bool {
	if !p.at(ctaDollarTok) || p.peek(1).kind != ctaNameTok || p.peek(2).kind != ctaSlashTok {
		return false
	}
	if p.attributeName(p.peek(1).text) != variable {
		return false
	}
	p.advance() // '$'
	p.advance() // VarName
	p.advance() // '/'
	return true
}

// ctaSiblingSteps is the spelling siblingBody reads after `$v/` and before the
// name M: `following-sibling::*[1][self::`. Its first three tokens are the
// opening atTalliedBody tells that body by, and its first two the
// `following-sibling::` atBoundSibling reads.
var ctaSiblingSteps = []ctaToken{
	{kind: ctaNameTok, text: "following-sibling"}, {kind: ctaAxisTok}, {kind: ctaWildcardTok, text: "*"},
	{kind: ctaLBracketTok}, {kind: ctaNumberTok, text: "1"}, {kind: ctaRBracketTok},
	{kind: ctaLBracketTok}, {kind: ctaNameTok, text: "self"}, {kind: ctaAxisTok},
}

// siblingBody parses the body `following-sibling::*[1][self::M]` after `$v/`,
// M a QName NameTest resolved on elementName's terms — the self axis's
// principal node kind is element (xpath20.md §3.2.1.2) — into the children
// named name whose next sibling element is named M (ctaChildrenFollowedBy).
// The predicates are read in that order and no other: §3.2.2 makes `[1]`
// positional, the first element sibling, and `[self::M]` an ·effective
// boolean value· filter of it, where `following-sibling::M[1]` and
// `*[self::M][1]` select the first M sibling however far.
//
// GAP(xpath): every other spelling declines — a position other than the
// IntegerLiteral `1`, `[01]` and `[1.0]` among them, `[last()]` and
// `[position() = 1]`; either predicate alone or the two swapped; a wildcard or
// kind test on either step; `self::` outside a predicate; and every other
// axis, `preceding::` and `preceding-sibling::` among them. The direction is
// the withhold [CompileAssertionTest] reports. (#1042)
func (p *ctaParser) siblingBody(name xsd.QName) (ctaRangeKey, bool) {
	for _, want := range ctaSiblingSteps {
		if p.peek(0) != want {
			return nil, false
		}
		p.advance()
	}
	if !p.at(ctaNameTok) || p.peek(1).kind != ctaRBracketTok {
		return nil, false
	}
	next := p.elementName(p.peek(0).text)
	p.advance() // M
	p.advance() // ']'
	return ctaChildrenFollowedBy{name: name, next: next}, true
}

// ifExpr parses xpath20.md [7] IfExpr, `"if" "(" Expr ")" "then" ExprSingle
// "else" ExprSingle`, whose `if (` the cursor is on, where the façade admits it
// (ctaFacade.conditional), so a Type Alternative's {test} declines it as
// outside its required subset. Both branches are parsed whatever the test
// will select, so a construct this engine declines in either declines the
// {test}, and a static error in the branch evaluation never selects still
// applies (xpath20.md §2.3.4: an expression is not rewritten to remove one).
func (p *ctaParser) ifExpr() (ctaExpr, bool) {
	if !p.facade.conditional() {
		return nil, false
	}
	p.advance() // 'if'
	p.advance() // '('
	test, ok := p.exprSingle()
	if !ok || !p.at(ctaRParen) {
		return nil, false
	}
	p.advance()
	if !p.atName("then") {
		return nil, false
	}
	p.advance()
	then, ok := p.exprSingle()
	if !ok || !p.atName("else") {
		return nil, false
	}
	p.advance()
	otherwise, ok := p.exprSingle()
	if !ok {
		return nil, false
	}
	return ctaIf{test: test, then: then, otherwise: otherwise}, true
}

// orExpr parses [9] ta-OrExpr. A single operand yields that operand rather
// than a one-armed ctaOr, so the tree holds no node that decides nothing.
func (p *ctaParser) orExpr() (ctaExpr, bool) {
	first, ok := p.andExpr()
	if !ok {
		return nil, false
	}
	operands := []ctaExpr{first}
	for p.atName("or") {
		p.advance()
		next, ok := p.andExpr()
		if !ok {
			return nil, false
		}
		operands = append(operands, next)
	}
	if len(operands) == 1 {
		return first, true
	}
	return ctaOr{operands: operands}, true
}

// andExpr parses [10] ta-AndExpr, on orExpr's terms.
func (p *ctaParser) andExpr() (ctaExpr, bool) {
	first, ok := p.booleanExpr()
	if !ok {
		return nil, false
	}
	operands := []ctaExpr{first}
	for p.atName("and") {
		p.advance()
		next, ok := p.booleanExpr()
		if !ok {
			return nil, false
		}
		operands = append(operands, next)
	}
	if len(operands) == 1 {
		return first, true
	}
	return ctaAnd{operands: operands}, true
}

// booleanExpr parses [11] ta-BooleanExpr's three arms. The first arm's
// parenthesized OrExpr is read as an ExprSingle (exprSingle), so an IfExpr
// stands inside the parentheses where the façade admits one; an `if (` in
// this production's own position is no ExprSingle and reaches
// constructorFunction, which declines it.
//
// The second and third arms both admit `QName '('` — [12] ta-BooleanFunction
// and [18] ta-ConstructorFunction — and §3.12.6 clause 3's Note resolves that
// ambiguity by FUNCTION NAME, not by argument shape: a name that is fn:not
// takes the BooleanFunction arm, and every other name is a constructor call
// and therefore a ValueExpr of the third arm. There is no fourth reading in
// which an unknown name is its own error.
//
// The third arm with its Comparator absent also takes a child path
// (childPath), a child step filtered by its children's existence
// (childrenHaving) or by a `preceding::` step (childrenPreceded) or one element
// step (selectedElements), whose ·effective boolean value· is whether it
// selects a node, where the path is the WHOLE ValueExpr: the token after it
// ends the BooleanExpr (closesBoolean). Followed by anything else — a
// comparator, an operator, a cast — it is left to additiveExpr, whose one child
// step leaves a '/' after it a token no production takes, and declines, and so
// does a `.` before a '/' or '//'; one step is childStep's, whose value is
// read.
//
// A left operand that is an integer or string sequence (sequenceLength) is read
// as one where a general comparator follows it, ahead of the `(` arm, which
// would read its parenthesis as a boolean one. Ahead of both, a comparison of
// fn:in-scope-prefixes against string literals is prefixMember's, whose left
// operand may be such a sequence.
//
// GAP(xpath): the context item `.` with its Comparator absent declines, so `.`
// and `not(.)` do: there it is a node, whose ·effective boolean value· is
// rule 2's (xpath20.md §2.4.3), and never that of the atom ctaContextAtom
// reads. The direction is the withhold [CompileAssertionTest] reports. (#1042)
func (p *ctaParser) booleanExpr() (ctaExpr, bool) {
	if p.prefixMemberLength() > 0 {
		return p.prefixMember()
	}
	if n := p.sequenceLength(0); n > 0 && p.peek(n).kind == ctaCompTok {
		left, ok := p.sequence(n)
		if !ok {
			return nil, false
		}
		op, compared := p.comparator()
		if !compared {
			return nil, false
		}
		return p.generalComparison(op, left)
	}
	if p.at(ctaLParen) {
		p.advance()
		x, ok := p.exprSingle()
		if !ok {
			return nil, false
		}
		if !p.at(ctaRParen) {
			return nil, false
		}
		p.advance()
		return x, true
	}
	if p.at(ctaNameTok) && p.peek(1).kind == ctaLParen && p.functionName(p.peek(0).text) == ctaNotFunction {
		return p.booleanFunction()
	}
	if n := p.childPathLength(0); n > 0 && p.closesBoolean(n) {
		path, ok := p.childPath(n)
		if !ok {
			return nil, false
		}
		return ctaEffectiveBoolean{operand: path}, true
	}
	if n := p.childrenHavingLength(0); n > 0 && p.closesBoolean(n) {
		having, ok := p.childrenHaving()
		if !ok {
			return nil, false
		}
		return ctaEffectiveBoolean{operand: having}, true
	}
	if n := p.childrenPrecededLength(0); n > 0 && p.closesBoolean(n) {
		preceded, ok := p.childrenPreceded()
		if !ok {
			return nil, false
		}
		return ctaEffectiveBoolean{operand: preceded}, true
	}
	if n := p.selectedStepLength(0); n > 0 && p.closesBoolean(n) {
		step, ok := p.selectedElements()
		if !ok {
			return nil, false
		}
		return ctaEffectiveBoolean{operand: step}, true
	}
	left, ok := p.additiveExpr()
	if !ok {
		return nil, false
	}
	if op, isValue := p.valueComparator(); isValue {
		return p.valueComparison(op, left)
	}
	op, compared := p.comparator()
	if !compared {
		if _, isContext := left.(ctaContextAtom); isContext {
			return nil, false
		}
		return ctaEffectiveBoolean{operand: left}, true
	}
	return p.generalComparison(op, left)
}

// generalComparison parses the right operand of a general comparison
// (xpath20.md §3.5.2) whose left operand and operator are already read —
// an integer or string sequence or an additiveExpr (generalOperand) — and builds its
// node, typed by ctaTypes.comparison: a type it cannot be compared in is the
// err:XPTY0004 ctaTypeError, and a declined one declines.
func (p *ctaParser) generalComparison(op ctaComparator, left ctaValue) (ctaExpr, bool) {
	right, ok := p.generalOperand()
	if !ok {
		return nil, false
	}
	comparison, typing := p.types.comparison(op, left, right)
	if typing == ctaTypeDeclined {
		return nil, false
	}
	if typing == ctaTypeErrored {
		return ctaTypeError{}, true
	}
	return ctaCompare{op: op, comparison: comparison, left: left, right: right}, true
}

// booleanFunction parses [12] ta-BooleanFunction, whose name the caller has
// already resolved to fn:not. Its argument is an ExprSingle (exprSingle), as
// xpath20.md [48] FunctionCall's is.
func (p *ctaParser) booleanFunction() (ctaExpr, bool) {
	p.advance() // the function name
	p.advance() // '('
	arg, ok := p.exprSingle()
	if !ok {
		return nil, false
	}
	if !p.at(ctaRParen) {
		return nil, false
	}
	p.advance()
	return ctaNot{operand: arg}, true
}

// comparator reads one [13] ta-Comparator, reporting false where the cursor is
// on something else — which is [11]'s optional-comparator arm, not an error.
func (p *ctaParser) comparator() (ctaComparator, bool) {
	if !p.at(ctaCompTok) {
		return ctaEqual, false
	}
	text := p.peek(0).text
	p.advance()
	switch text {
	case "=":
		return ctaEqual, true
	case "!=":
		return ctaNotEqual, true
	case "<":
		return ctaLess, true
	case "<=":
		return ctaLessEqual, true
	case ">":
		return ctaGreater, true
	case ">=":
		return ctaGreaterEqual, true
	}
	return ctaEqual, false
}

// valueComparator reads one xpath20.md [23] ValueComp, reporting false where
// the cursor is on anything else. Each spelling is the operator whose B.2 rows
// it reads — `eq` the row `A eq B`, and so on for the other five — which are
// the rows the [13] ta-Comparator spellings reach through §3.5.2's definition
// of a general comparison, so one operator type serves both.
//
// The six spellings are NCNames, and XPath has no reserved words, so what makes
// one an operator is its position: right after a [14] ta-ValueExpr no other
// production opens with a name but 'cast', 'castable', 'and' and 'or', none
// of which is spelled like one.
func (p *ctaParser) valueComparator() (ctaComparator, bool) {
	if !p.at(ctaNameTok) {
		return ctaEqual, false
	}
	var op ctaComparator
	switch p.peek(0).text {
	case "eq":
		op = ctaEqual
	case "ne":
		op = ctaNotEqual
	case "lt":
		op = ctaLess
	case "le":
		op = ctaLessEqual
	case "gt":
		op = ctaGreater
	case "ge":
		op = ctaGreaterEqual
	default:
		return ctaEqual, false
	}
	p.advance()
	return op, true
}

// valueComparison parses the right operand of a value comparison whose left
// operand and operator are already read, and builds its node: xpath20.md [10]
// ComparisonExpr with its ValueComp arm, which §3.12.6's grammar does not
// have. The façade decides whether it is admitted at all
// (ctaFacade.comparesValues), so a Type Alternative's {test} declines it as
// outside its required subset.
//
// [10] admits ONE comparison per ComparisonExpr, so `@a eq @b eq @c` is not an
// expression; here the trailing `eq` is a token no production takes, and the
// parse ends unsupported as any other unparsed tail does.
//
// The type both operands are compared in is settled here, on §3.5.1's terms
// (ctaTypes.valueComparison), and a type it cannot be compared in is the
// err:XPTY0004 node a general comparison builds too, and a settled type is
// evaluated exactly as a general comparison's is (ctaHoldsPair).
func (p *ctaParser) valueComparison(op ctaComparator, left ctaValue) (ctaExpr, bool) {
	if !p.facade.comparesValues() {
		return nil, false
	}
	right, ok := p.additiveExpr()
	if !ok {
		return nil, false
	}
	comparison, typing := p.types.valueComparison(op, left, right)
	if typing == ctaTypeDeclined {
		return nil, false
	}
	if typing == ctaTypeErrored {
		return ctaTypeError{}, true
	}
	return ctaValueCompare{op: op, comparison: comparison, left: left, right: right}, true
}

// additiveExpr parses xpath20.md [13] AdditiveExpr, `MultiplicativeExpr ( ("+"
// | "-") MultiplicativeExpr )*`, left-associatively, in each position [11]
// ta-BooleanExpr and a value comparison read an operand in. A single operand
// yields that operand rather than an arithmetic node, so a {test} with no
// operator compiles to the tree it always did.
func (p *ctaParser) additiveExpr() (ctaValue, bool) {
	left, ok := p.multiplicativeExpr()
	if !ok {
		return nil, false
	}
	for {
		op, isOp := p.additiveOperator()
		if !isOp {
			return left, true
		}
		right, ok := p.multiplicativeExpr()
		if !ok {
			return nil, false
		}
		if left, ok = p.arithmetic(op, left, right); !ok {
			return nil, false
		}
	}
}

// additiveOperator reads one [13] AdditiveExpr operator, reporting false where
// the cursor is on anything else.
func (p *ctaParser) additiveOperator() (ctaArithOp, bool) {
	switch p.peek(0).kind {
	case ctaPlusTok:
		p.advance()
		return ctaAdd, true
	case ctaMinusTok:
		p.advance()
		return ctaSubtract, true
	default:
		return ctaAdd, false
	}
}

// multiplicativeExpr parses xpath20.md [14] MultiplicativeExpr, `UnionExpr (
// ("*" | "div" | "idiv" | "mod") UnionExpr )*`, on additiveExpr's terms. Each
// operand is a [16] InstanceofExpr (instanceofExpr): the productions between
// UnionExpr and InstanceofExpr are reached only through their one-operand
// arms, so a union — but in an fn:count argument (countArgument) — an
// `intersect`, an `except`, a `treat` and a unary sign leave a token no
// production takes and decline.
func (p *ctaParser) multiplicativeExpr() (ctaValue, bool) {
	left, ok := p.instanceofExpr()
	if !ok {
		return nil, false
	}
	for {
		op, isOp := p.multiplicativeOperator()
		if !isOp {
			return left, true
		}
		right, ok := p.instanceofExpr()
		if !ok {
			return nil, false
		}
		if left, ok = p.arithmetic(op, left, right); !ok {
			return nil, false
		}
	}
}

// multiplicativeOperator reads one [14] MultiplicativeExpr operator, reporting
// false where the cursor is on anything else. `*` is a bare ctaWildcardTok,
// which right after a complete operand no NameTest can be; `div`, `idiv` and
// `mod` are NCNames, operators by position on valueComparator's terms.
func (p *ctaParser) multiplicativeOperator() (ctaArithOp, bool) {
	tok := p.peek(0)
	var op ctaArithOp
	switch {
	case tok.kind == ctaWildcardTok && tok.text == "*":
		op = ctaMultiply
	case tok.kind == ctaNameTok && tok.text == "div":
		op = ctaDivide
	case tok.kind == ctaNameTok && tok.text == "idiv":
		op = ctaIntegerDivide
	case tok.kind == ctaNameTok && tok.text == "mod":
		op = ctaModulus
	default:
		return ctaAdd, false
	}
	p.advance()
	return op, true
}

// instanceofExpr parses xpath20.md [16] InstanceofExpr, `TreatExpr (
// "instance" "of" SequenceType )?`, whose TreatExpr is reached through its
// one-operand arm down to a [14] ta-ValueExpr (valueExpr), where the façade
// admits the tail (ctaFacade.instanceOf), so a Type Alternative's {test}
// declines it as outside its required subset. A call to fn:data opening it is
// dataInstanceOf's. The operand is matched ATOMIZED (ctaInstanceOf), so an
// operand that is a node — an attribute or child step, a path, `.` — is
// matched only as fn:data's argument; a node itself never matches an
// AtomicType (§2.5.4.2).
//
// GAP(xpath): a node operand under `instance of` without fn:data, `@d
// instance of xs:date`, declines rather than answering for the node (false
// over a node, true for the empty sequence under `?` or `*`). The direction is
// the withhold [CompileAssertionTest] reports. (#1042)
func (p *ctaParser) instanceofExpr() (ctaValue, bool) {
	if p.facade.instanceOf() && p.at(ctaNameTok) && p.peek(1).kind == ctaLParen && p.functionName(p.peek(0).text) == ctaDataFunction {
		return p.dataInstanceOf()
	}
	v, ok := p.valueExpr()
	if !ok {
		return nil, false
	}
	if !p.atName("instance") {
		return v, true
	}
	if !p.facade.instanceOf() {
		return nil, false
	}
	if _, isStep := v.(ctaStep); isStep {
		return nil, false
	}
	if _, isContext := v.(ctaContextAtom); isContext {
		return nil, false
	}
	return p.instanceTail(v)
}

// dataInstanceOf parses a call to fn:data with its one `item()*` argument
// (xpath-functions.md §2.4), whose name the cursor is on, as the operand of
// the `instance of` tail that must follow it (instanceTail): fn:data returns
// "the result of atomizing a sequence" (xpath20.md §2.4.2), which is the
// sequence ctaInstanceOf matches, so the call is no node of its own and its
// argument is the tail's operand. Every other arity declines (err:XPST0017).
//
// GAP(xpath): fn:data anywhere but as the operand of `instance of`, or over
// `$value` as a quantifier's binding sequence (valueBinding), declines, so
// `data(@d) = 1` and `string(data(.))` do: elsewhere a node operand is read
// atomized already, and its ·effective boolean value· and fn:exists would
// still read the node, which fn:data does not return. The direction is the
// withhold [CompileAssertionTest] reports. (#1042)
func (p *ctaParser) dataInstanceOf() (ctaValue, bool) {
	args, ok := p.arguments()
	if !ok || len(args) != 1 || !p.atName("instance") {
		return nil, false
	}
	return p.instanceTail(args[0])
}

// instanceTail parses the `"instance" "of" SequenceType` tail over v, whose
// `instance` the cursor is on, into its node, reporting false where v's static
// type does not decide its items' dynamic type (ctaTypes.instanceItem) or the
// SequenceType declines (sequenceType). An fn:sum call, whose dynamic type its
// static type does not decide, is sumInstanceTail's. An absent indicator, `?`,
// `*` and `+` are xpath20.md [51] OccurrenceIndicator; xpath20.md A.1.2's
// occurrence-indicators constraint makes a `*` or `+` after the AtomicType an
// indicator and never an operator.
func (p *ctaParser) instanceTail(v ctaValue) (ctaValue, bool) {
	p.advance() // 'instance'
	if !p.atName("of") {
		return nil, false
	}
	p.advance()
	if sum, isSum := v.(ctaSum); isSum {
		return p.sumInstanceTail(sum)
	}
	item, derived, decides := p.types.instanceItem(v)
	if !decides {
		return nil, false
	}
	matches, ok := p.sequenceType(item)
	if !ok {
		return nil, false
	}
	// GAP(xpath): over a typed child, an AtomicType its compiled type does not
	// derive from may still match the child's own type annotation, derived
	// from it by an xsi:type (ctaTypes.instanceItem), so the tail declines
	// rather than answer false. The direction is the withhold
	// [CompileAssertionTest] reports. (#1042)
	if derived && !matches {
		return nil, false
	}
	occurrence := p.occurrenceIndicator()
	boolean, resolved := p.types.simple(ctaBuiltin("boolean"))
	if !resolved {
		return nil, false
	}
	read := p.types.str
	if typed, isTyped := item.(ctaTyped); isTyped {
		read = typed.st
	}
	return ctaInstanceOf{operand: v, read: read, matches: matches, occurrence: occurrence, st: boolean}, true
}

// sumInstanceTail parses the SequenceType of an `instance of` tail over sum, an
// fn:sum call, whose `of` the cursor has passed, into its ctaSumInstance. The
// call's value's DYNAMIC type depends on how many items its argument yields
// (ctaSum.total), so whether it matches is decided at evaluation and never
// from sum's static type: only the AtomicType is settled here, on
// sequenceType's terms, admitted where itemMatches admits it. The call always
// returns exactly one item, which every occurrence indicator admits, so the
// indicator is read and decides nothing.
func (p *ctaParser) sumInstanceTail(sum ctaSum) (ctaValue, bool) {
	if !p.at(ctaNameTok) || p.peek(1).kind == ctaLParen {
		return nil, false
	}
	name := p.typeName(p.peek(0).text)
	p.advance()
	if _, admitted := p.types.itemMatches(ctaTyped{st: sum.st}, name); !admitted {
		return nil, false
	}
	p.occurrenceIndicator()
	boolean, resolved := p.types.simple(ctaBuiltin("boolean"))
	if !resolved {
		return nil, false
	}
	return ctaSumInstance{sum: sum, atomic: name, st: boolean}, true
}

// sequenceType parses the ItemType of xpath20.md [50] SequenceType where it is
// [53] AtomicType, a QName resolved as a TYPE name on singleType's terms, and
// reports whether an item of type item matches it (ctaTypes.itemMatches).
//
// GAP(xpath): every other SequenceType declines — [54] KindTest, such as
// assert013's `attribute(*, xs:dateTime)`, `item()` and `empty-sequence()`,
// each a name followed by '(' — and so does an AtomicType itemMatches does not
// admit. The direction is the withhold [CompileAssertionTest] reports. (#1042)
func (p *ctaParser) sequenceType(item ctaStatic) (matches, ok bool) {
	if !p.at(ctaNameTok) || p.peek(1).kind == ctaLParen {
		return false, false
	}
	name := p.typeName(p.peek(0).text)
	p.advance()
	return p.types.itemMatches(item, name)
}

// occurrenceIndicator reads an optional xpath20.md [51] OccurrenceIndicator,
// consuming it, or reports ctaExactlyOne where the cursor is on none. A `*`
// is a bare ctaWildcardTok, on multiplicativeOperator's terms.
func (p *ctaParser) occurrenceIndicator() ctaOccurrence {
	tok := p.peek(0)
	var o ctaOccurrence
	switch {
	case tok.kind == ctaQuestionTok:
		o = ctaZeroOrOne
	case tok.kind == ctaWildcardTok && tok.text == "*":
		o = ctaZeroOrMore
	case tok.kind == ctaPlusTok:
		o = ctaOneOrMore
	default:
		return ctaExactlyOne
	}
	p.advance()
	return o
}

// arithmetic builds the node of one binary arithmetic operator over two
// operands already parsed (xpath20.md §3.4), declining where the façade
// computes nothing (ctaFacade.computes) or ctaTypes.arithmetic will not type
// the pair.
func (p *ctaParser) arithmetic(op ctaArithOp, left, right ctaValue) (ctaValue, bool) {
	if !p.facade.computes() {
		return nil, false
	}
	return p.types.arithmetic(op, left, right)
}

// valueExpr parses [14] ta-ValueExpr, dispatching on whether a function call
// opens it, and on which function it calls: fn:count, which the assertion
// and facet façades call (countCall), one of the F&O functions a façade that calls the
// library admits (ctaFacade.callsLibrary, libraryCall), or a constructor.
//
// The gate is asked before the name is: where the façade calls no library
// function, every name but fn:count is a constructor call, so a Type
// Alternative's {test} reaches castTarget with it and declines, and its grammar
// stays at fn:not (§3.12.6 clause 3).
func (p *ctaParser) valueExpr() (ctaValue, bool) {
	if !p.at(ctaNameTok) || p.peek(1).kind != ctaLParen {
		return p.castExpr()
	}
	name := p.functionName(p.peek(0).text)
	if name == ctaCountFunction {
		return p.countCall()
	}
	if p.facade.callsLibrary() && name.Space == ctaFunctionNS {
		return p.libraryCall(name.Local)
	}
	return p.constructorFunction()
}

// libraryCall parses a call, xpath20.md [48] FunctionCall, to the function
// named local in the default function namespace, where it is one of the F&O
// functions the assertion and facet façades admit (ctafunc.go), and is a
// constructorFunction for every other local, where castTarget declines it.
// The name is matched here and nowhere else, and no node stores it.
func (p *ctaParser) libraryCall(local string) (ctaValue, bool) {
	switch local {
	case "contains":
		return p.matchCall(ctaContains)
	case "starts-with":
		return p.matchCall(ctaStartsWith)
	case "ends-with":
		return p.matchCall(ctaEndsWith)
	case "string-length":
		return p.unaryStringCall(ctaStringLength)
	case "normalize-space":
		return p.unaryStringCall(ctaNormalizeSpace)
	case "string":
		return p.stringCall()
	case "concat":
		return p.concatCall()
	case "empty":
		return p.presenceCall(ctaEmptyTest)
	case "exists":
		return p.presenceCall(ctaExistsTest)
	case "distinct-values":
		return p.distinctValuesCall()
	case "true", "false":
		return p.constantCall(local)
	case "current-date":
		return p.currentDateCall()
	case "position", "last":
		return p.focusCall()
	case "namespace-uri":
		return p.namespaceURICall()
	case "sum":
		return p.sumCall()
	}
	return p.constructorFunction()
}

// sumCall parses a call to fn:sum with its one `xs:anyAtomicType*` argument
// (xpath-functions.md §15.4.5), whose name the cursor is on, where that
// argument is a path through one or more child steps to an attribute step,
// `N1/…/@A` (attributePathLength), whose node p.facade builds
// (ctaFacade.attributePath) and which ctaTypes.sum types the call over.
//
// GAP(xpath): every other call to fn:sum declines — the two-argument form
// `sum($arg, $zero)`, whose empty result is `$zero` and never the xs:integer
// 0 of this one, and every other argument: E's own attribute `sum(@p)`, a
// child or element path read for its value `sum(c)` or `sum(a/c)`, `$value`,
// a wildcard or `//` step, a predicate, and any other expression — as do
// fn:avg, fn:min and fn:max, which reach constructorFunction. The direction is
// the withhold [CompileAssertionTest] reports. (#1042)
func (p *ctaParser) sumCall() (ctaValue, bool) {
	n := p.attributePathLength(2)
	if n == 0 || p.peek(2+n).kind != ctaRParen {
		return nil, false
	}
	p.advance() // the function name
	p.advance() // '('
	var steps []xsd.QName
	for end := p.pos + n - 2; p.pos < end; p.advance() {
		if p.at(ctaSlashTok) {
			continue
		}
		steps = append(steps, p.elementName(p.peek(0).text))
	}
	p.advance() // '@'
	test := p.attributeTest()
	p.advance()
	p.advance() // ')'
	arg, ok := p.facade.attributePath(steps, test, p.types)
	if !ok {
		return nil, false
	}
	return p.types.sum(arg)
}

// attributeTest resolves the NameTest the cursor is on, after an attribute
// step's '@': a QName on attributeName's terms, and a [37] Wildcard on
// wildcardTest's. Nothing is consumed.
func (p *ctaParser) attributeTest() ctaNameTest {
	text := p.peek(0).text
	if p.at(ctaWildcardTok) {
		return p.wildcardTest(text)
	}
	return ctaExactName{name: p.attributeName(text)}
}

// attributePathLength is how many tokens, from offset at ahead of the cursor,
// spell a relative path of one or more abbreviated child-axis steps with QName
// NameTests followed by one abbreviated attribute step whose NameTest is a
// QName or a [37] Wildcard, `QName ('/' QName)* '/' '@' NameTest`, and 0 where
// they spell none. A name followed by anything but '/' — a '(', a '::', a '['
// — is no such step. Nothing is consumed.
func (p *ctaParser) attributePathLength(at int) int {
	n := 0
	for {
		if p.peek(at+n).kind != ctaNameTok || p.peek(at+n+1).kind != ctaSlashTok {
			return 0
		}
		n += 2
		if p.peek(at+n).kind == ctaAtTok {
			break
		}
	}
	if last := p.peek(at + n + 1).kind; last != ctaNameTok && last != ctaWildcardTok {
		return 0
	}
	return n + 2
}

// arguments parses the parenthesized argument list of a call whose name the
// cursor is on, xpath20.md [48] FunctionCall's `"(" (ExprSingle ("," ExprSingle)*)?
// ")"`, each argument as argument parses it. The arity is the caller's to
// check, before it builds a node.
func (p *ctaParser) arguments() ([]ctaValue, bool) {
	p.advance() // the function name
	p.advance() // '('
	if p.at(ctaRParen) {
		p.advance()
		return nil, true
	}
	var args []ctaValue
	for {
		arg, ok := p.argument()
		if !ok {
			return nil, false
		}
		args = append(args, arg)
		if p.at(ctaRParen) {
			p.advance()
			return args, true
		}
		if !p.at(ctaCommaTok) {
			return nil, false
		}
		p.advance()
	}
}

// argument parses one argument of a library call: an additiveExpr operand, or
// the empty sequence `()` (xpath20.md [46] ParenthesizedExpr with no Expr),
// which is admitted in argument position only and compiles to the statically
// empty ctaEmptyValue. Every other parenthesized expression declines here, as
// it does in an arithmetic operand's position.
func (p *ctaParser) argument() (ctaValue, bool) {
	if p.at(ctaLParen) && p.peek(1).kind == ctaRParen {
		p.advance()
		p.advance()
		return ctaEmptyValue{}, true
	}
	return p.additiveExpr()
}

// matchCall parses a call to fn:contains, fn:starts-with or fn:ends-with (op)
// with its two xs:string? arguments (xpath-functions.md §7.5.1–7.5.3), whose
// result is xs:boolean. Every other arity declines (err:XPST0017, withheld on
// [CompileAssertionTest]'s terms), and so does a boolean that does not
// resolve.
//
// GAP(xpath): the three-argument form, whose third argument names a collation
// (§7.3.1), declines rather than being evaluated — and is never evaluated as
// the two-argument form, which compares under the default collation alone. The
// direction is the withhold [CompileAssertionTest] reports. (#1042)
func (p *ctaParser) matchCall(op ctaMatchOp) (ctaValue, bool) {
	args, ok := p.arguments()
	if !ok || len(args) != 2 {
		return nil, false
	}
	boolean, resolved := p.types.simple(ctaBuiltin("boolean"))
	if !resolved {
		return nil, false
	}
	return ctaMatch{op: op, left: p.types.stringArgument(args[0]), right: p.types.stringArgument(args[1]), st: boolean}, true
}

// unaryStringCall parses a call to fn:string-length or fn:normalize-space (op)
// with its one xs:string? argument, or with none, which F&O makes the string
// value of the context item: `f(fn:string(.))` (xpath-functions.md §7.4.4,
// §7.4.5), whose `.` is argumentOrDot's and whose fn:string is stringOf's. Two
// or more arguments decline, and so does a result type that does not resolve.
func (p *ctaParser) unaryStringCall(op ctaUnaryStringOp) (ctaValue, bool) {
	args, ok := p.arguments()
	if !ok || len(args) > 1 {
		return nil, false
	}
	arg, ok := p.argumentOrDot(args)
	if !ok {
		return nil, false
	}
	if len(args) == 0 {
		if arg, ok = p.stringOf(arg); !ok {
			return nil, false
		}
	}
	st, resolved := p.types.simple(op.resultType())
	if !resolved {
		return nil, false
	}
	return ctaUnaryString{op: op, arg: p.types.stringArgument(arg), st: st}, true
}

// argumentOrDot is the one argument args holds, or, where it holds none, the
// context item `.`, which is p.facade's (ctaFacade.contextItem): the assertion
// façade's is E's string value under every non-nil {content type} and
// declines under a nil one, and the facet façade's raises err:XPDY0002.
func (p *ctaParser) argumentOrDot(args []ctaValue) (ctaValue, bool) {
	if len(args) == 1 {
		return args[0], true
	}
	return p.facade.contextItem()
}

// stringCall parses a call to fn:string with its one `item()?` argument, or
// with none, which is the context item `.` (xpath-functions.md §2.3). Two or
// more arguments decline.
func (p *ctaParser) stringCall() (ctaValue, bool) {
	args, ok := p.arguments()
	if !ok || len(args) > 1 {
		return nil, false
	}
	arg, ok := p.argumentOrDot(args)
	if !ok {
		return nil, false
	}
	return p.stringOf(arg)
}

// stringOf builds fn:string over arg: "the same string as is returned by the
// expression "$arg cast as xs:string"" for an atomic value (xpath-functions.md
// §2.3), which is a ctaCast to xs:string admitted on castsFrom's terms and
// allowing the empty sequence, which ctaStringFunction maps to the zero-length
// string. A node's string-value is the same string wherever castsFrom admits
// the node: the string-value of an attribute or of an element of simple type is
// its [schema normalized value] (xpath-datamodel :1562, :1307), which for the
// xs:string family is its typed value. For an xs:date, xs:dateTime or xs:time
// node this engine takes it to be the ·canonical representation· of the typed
// value, which castsFrom's third rule admits: this engine's TypedValue holds
// the typed value alone (tvTyped, xpath/assertion.go), so the data model's
// licence condition is met — "If an implementation stores only the typed value
// of an attribute, it may use any valid lexical representation of the typed
// value for the string-value property" (xpath-datamodel §6.3.4 :1567, and
// §6.2.4 :1309 for an element). Every other typed node, and every xs:float or
// xs:double argument, a literal included, declines under castsFrom's
// GAP(xpath).
func (p *ctaParser) stringOf(arg ctaValue) (ctaStringFunction, bool) {
	if !p.types.castsFrom(arg, p.types.str) {
		return ctaStringFunction{}, false
	}
	return ctaStringFunction{cast: ctaCast{operand: arg, target: p.types.str, allowsEmpty: true}}, true
}

// concatCall parses a call to fn:concat with two or more `xs:anyAtomicType?`
// arguments (xpath-functions.md §7.4.1), whose result is xs:string: each
// argument is "cast to xs:string" and the empty sequence "is treated as the
// zero-length string", which is fn:string's reading of an atomic argument, so
// each is held as the fn:string call stringOf builds over it and an argument
// stringOf declines declines the call. Fewer than two arguments match no
// signature (err:XPST0017, xpath20.md §3.1.5) and decline.
func (p *ctaParser) concatCall() (ctaValue, bool) {
	args, ok := p.arguments()
	if !ok || len(args) < 2 {
		return nil, false
	}
	parts := make([]ctaStringFunction, 0, len(args))
	for _, arg := range args {
		part, admitted := p.stringOf(arg)
		if !admitted {
			return nil, false
		}
		parts = append(parts, part)
	}
	return ctaConcat{args: parts, st: p.types.str}, true
}

// presenceCall parses a call to fn:empty or fn:exists (op) with its one
// `item()*` argument (xpath-functions.md §15.1.4, §15.1.5), which is not
// atomized (presenceArgument), and whose result is xs:boolean. A boolean that
// does not resolve declines.
//
// GAP(xpath): the context item `.` as the whole argument declines: there it is
// a node, E, which fn:exists and fn:empty do not atomize, and never the atom
// ctaContextAtom reads. The direction is the withhold [CompileAssertionTest]
// reports. (#1042)
func (p *ctaParser) presenceCall(op ctaPresenceOp) (ctaValue, bool) {
	operand, ok := p.presenceArgument()
	if !ok {
		return nil, false
	}
	if _, isContext := operand.(ctaContextAtom); isContext {
		return nil, false
	}
	boolean, resolved := p.types.simple(ctaBuiltin("boolean"))
	if !resolved {
		return nil, false
	}
	return ctaPresence{op: op, operand: operand, st: boolean}, true
}

// presenceArgument parses the parenthesized argument list of the fn:empty or
// fn:exists call whose name the cursor is on: a child path (childPath), a
// child step filtered by its children's existence (childrenHaving) or by a
// `preceding::` step (childrenPreceded) or one element step (selectedElements)
// where it is the whole list, closed by the call's ')', and otherwise the list
// arguments parses, of which exactly one argument is admitted — every other
// arity declines (err:XPST0017).
func (p *ctaParser) presenceArgument() (ctaValue, bool) {
	if n := p.childPathLength(2); n > 0 && p.peek(2+n).kind == ctaRParen {
		p.advance() // the function name
		p.advance() // '('
		path, ok := p.childPath(n)
		p.advance() // ')'
		return path, ok
	}
	if n := p.childrenHavingLength(2); n > 0 && p.peek(2+n).kind == ctaRParen {
		p.advance() // the function name
		p.advance() // '('
		having, ok := p.childrenHaving()
		p.advance() // ')'
		return having, ok
	}
	if n := p.childrenPrecededLength(2); n > 0 && p.peek(2+n).kind == ctaRParen {
		p.advance() // the function name
		p.advance() // '('
		preceded, ok := p.childrenPreceded()
		p.advance() // ')'
		return preceded, ok
	}
	if n := p.selectedStepLength(2); n > 0 && p.peek(2+n).kind == ctaRParen {
		p.advance() // the function name
		p.advance() // '('
		step, ok := p.selectedElements()
		p.advance() // ')'
		return step, ok
	}
	args, ok := p.arguments()
	if !ok || len(args) != 1 {
		return nil, false
	}
	return args[0], true
}

// distinctValuesCall parses a call to fn:distinct-values with its one
// `xs:anyAtomicType*` argument (xpath-functions.md §15.1.6), which is atomized
// (ctaDistinctValues), and whose result is that argument's distinct items. A
// statically empty argument compiles to ctaEmptyValue, which is the empty
// sequence the call returns over it. The node's st is the type its items are
// compared in: the argument's own static type, or xs:string for an
// xs:untypedAtomic one, which declines where p.types resolves no xs:string.
// No argument declines (err:XPST0017).
//
// GAP(xpath): the two-argument form, whose second argument names a collation
// (§7.3.1), declines rather than being evaluated — and is never evaluated as
// the one-argument form, which compares under the default collation alone. The
// direction is the withhold [CompileAssertionTest] reports. (#1042)
func (p *ctaParser) distinctValuesCall() (ctaValue, bool) {
	args, ok := p.arguments()
	if !ok || len(args) != 1 {
		return nil, false
	}
	if ctaIsEmpty(args[0]) {
		return ctaEmptyValue{}, true
	}
	if typed, isTyped := ctaStaticOf(args[0]).(ctaTyped); isTyped {
		return ctaDistinctValues{operand: args[0], st: typed.st}, true
	}
	str, resolved := p.types.simple(ctaBuiltin("string"))
	if !resolved {
		return nil, false
	}
	return ctaDistinctValues{operand: args[0], st: str}, true
}

// constantCall parses a call to fn:true or fn:false, named local, with no
// argument (xpath-functions.md §9.1.1, §9.1.2), into the ctaLiteral of the
// xs:boolean it returns, whose lexical local is. An argument declines, and so
// does a boolean that does not resolve.
func (p *ctaParser) constantCall(local string) (ctaValue, bool) {
	args, ok := p.arguments()
	if !ok || len(args) != 0 {
		return nil, false
	}
	boolean, resolved := p.types.simple(ctaBuiltin("boolean"))
	if !resolved {
		return nil, false
	}
	return ctaLiteral{text: local, st: boolean}, true
}

// currentDateCall parses a call to fn:current-date with no argument
// (xpath-functions.md §16.4) into its ctaCurrentDate, whose result is
// xs:date. An argument declines (err:XPST0017), and so does an xs:date that
// does not resolve.
func (p *ctaParser) currentDateCall() (ctaValue, bool) {
	args, ok := p.arguments()
	if !ok || len(args) != 0 {
		return nil, false
	}
	date, resolved := p.types.simple(ctaBuiltin("date"))
	if !resolved {
		return nil, false
	}
	return ctaCurrentDate{st: date}, true
}

// focusCall parses a call to fn:position or fn:last with no argument
// (xpath-functions.md §16.1, §16.2), whose result is xs:integer, into the node
// the façade compiles a read of its focus to (ctaFacade.focus). An argument
// declines (err:XPST0017), and so does an xs:integer that does not resolve.
func (p *ctaParser) focusCall() (ctaValue, bool) {
	args, ok := p.arguments()
	if !ok || len(args) != 0 {
		return nil, false
	}
	integer, resolved := p.types.simple(ctaBuiltin("integer"))
	if !resolved {
		return nil, false
	}
	return p.facade.focus(integer)
}

// namespaceURICall parses a call to fn:namespace-uri over `.` or with no
// argument, which "defaults to the context node (.)" (xpath-functions.md
// §14.3), into its ctaNamespaceURI over the node the façade compiles `.` to
// (contextNodeArgument), whose result is xs:anyURI. An xs:anyURI that does not
// resolve declines.
//
// GAP(xpath): any other argument declines — a path such as `@a` or `a`, a
// variable, `()`, and `.` written any other way, `(.)` or `./self::node()` —
// rather than reading the namespace name of the node it selects. The
// direction is the withhold [CompileAssertionTest] reports. (#1042)
func (p *ctaParser) namespaceURICall() (ctaValue, bool) {
	node, ok := p.contextNodeArgument(true)
	if !ok {
		return nil, false
	}
	anyURI, resolved := p.types.simple(ctaBuiltin("anyURI"))
	if !resolved {
		return nil, false
	}
	return ctaNamespaceURI{context: node, st: anyURI}, true
}

// contextNodeArgument parses the argument list of the call whose name the
// cursor is on where it is `(.)`, or `()` where admitsNone, into the node the
// façade compiles `.` to as a NODE (ctaFacade.contextNode) — never through
// ctaFacade.contextItem, whose atom a node parameter does not take, so the
// call records no read of E's string value ([AssertionTest.ReadsContextItem]).
// Every other list declines.
func (p *ctaParser) contextNodeArgument(admitsNone bool) (ctaNodeArg, bool) {
	n := p.contextNodeCallLength(0, admitsNone)
	if n == 0 {
		return nil, false
	}
	p.pos += n
	return p.facade.contextNode()
}

// contextNodeCallLength is how many tokens, from offset at ahead of the cursor
// where a function name stands, spell a call whose argument list is `(.)` —
// or `()` where admitsNone — and 0 where they spell none. Nothing is consumed.
func (p *ctaParser) contextNodeCallLength(at int, admitsNone bool) int {
	if p.peek(at+1).kind != ctaLParen {
		return 0
	}
	if p.peek(at+2).kind == ctaDotTok && p.peek(at+3).kind == ctaRParen {
		return 4
	}
	if admitsNone && p.peek(at+2).kind == ctaRParen {
		return 3
	}
	return 0
}

// prefixMemberLength is how many tokens at the cursor spell a general
// comparison by `=` of a call to fn:in-scope-prefixes over `.`,
// `in-scope-prefixes(.)`, against a StringLiteral or a parenthesized sequence
// of them (stringOperandLength), the call on either side, and 0 where they
// spell none or the façade calls no library function (ctaFacade.callsLibrary).
// Nothing is consumed, and the lookahead records only the err:XPST0081 of an
// unbound prefix that the parse itself records at the same token
// (prefixesCallLength).
func (p *ctaParser) prefixMemberLength() int {
	if !p.facade.callsLibrary() {
		return 0
	}
	if call := p.prefixesCallLength(0); call > 0 {
		literals := p.stringOperandLength(call + 1)
		if literals == 0 || !p.equalsAt(call) {
			return 0
		}
		return call + 1 + literals
	}
	literals := p.stringOperandLength(0)
	if literals == 0 || !p.equalsAt(literals) {
		return 0
	}
	call := p.prefixesCallLength(literals + 1)
	if call == 0 {
		return 0
	}
	return literals + 1 + call
}

// prefixesCallLength is how many tokens, from offset at ahead of the cursor,
// spell `in-scope-prefixes(.)`, the name resolved in the function namespace
// (functionName), and 0 where they spell anything else. Nothing is consumed,
// but functionName records the err:XPST0081 of an unbound prefix; that is
// only the one the parse itself records at the same token, since a prefixed
// name followed by `(` is a FunctionCall whatever this answers and the parser
// never backtracks.
func (p *ctaParser) prefixesCallLength(at int) int {
	tok := p.peek(at)
	if tok.kind != ctaNameTok || p.peek(at+1).kind != ctaLParen || p.functionName(tok.text) != ctaInScopePrefixesFunction {
		return 0
	}
	return p.contextNodeCallLength(at, false)
}

// equalsAt reports whether the token at offset at ahead of the cursor is the
// general comparator `=`.
func (p *ctaParser) equalsAt(at int) bool {
	tok := p.peek(at)
	return tok.kind == ctaCompTok && tok.text == "="
}

// stringOperandLength is how many tokens, from offset at ahead of the cursor,
// spell a StringLiteral or a parenthesized sequence of them
// (stringSequenceLength), and 0 where they spell neither.
func (p *ctaParser) stringOperandLength(at int) int {
	if p.peek(at).kind == ctaStringTok {
		return 1
	}
	return p.stringSequenceLength(at)
}

// prefixMember parses the comparison prefixMemberLength measured at the cursor
// into its ctaPrefixMember (xpath-functions.md §11.2.6, xpath20.md §3.5.2):
// the call's `.` compiled as a node (contextNodeArgument), and the literals in
// written order. A token after it that ends no [11] ta-BooleanExpr, as in
// `in-scope-prefixes(.) = 'a' cast as xs:string`, is left to the enclosing
// production, which takes none and declines.
//
// GAP(xpath): fn:in-scope-prefixes in every other shape declines — a
// free-standing call, `count(in-scope-prefixes(.))`, an operator but `=`,
// `!=` and `eq` among them, an operand that is neither a StringLiteral nor a
// parenthesized sequence of them, and any argument but `.` — because
// evaluating it there needs the sequence of E's prefixes, which only a
// listing of E's [in-scope namespaces] builds, and [ContextElement] answers
// one prefix at a time as validate.Element's LookupPrefix does. The direction
// is the withhold [CompileAssertionTest] reports. (#1042)
func (p *ctaParser) prefixMember() (ctaExpr, bool) {
	if p.prefixesCallLength(0) > 0 {
		node, ok := p.contextNodeArgument(false)
		if !ok {
			return nil, false
		}
		p.advance() // '='
		return ctaPrefixMember{context: node, literals: p.stringOperand()}, true
	}
	literals := p.stringOperand()
	p.advance() // '='
	node, ok := p.contextNodeArgument(false)
	if !ok {
		return nil, false
	}
	return ctaPrefixMember{context: node, literals: literals}, true
}

// stringOperand parses the StringLiteral, or the parenthesized sequence of
// them, stringOperandLength measured at the cursor into their values, in
// written order.
func (p *ctaParser) stringOperand() []string {
	if p.at(ctaStringTok) {
		text := p.peek(0).text
		p.advance()
		return []string{text}
	}
	return p.stringSequence(p.stringSequenceLength(0)).texts
}

// countCall parses an fn:count call, xpath20.md [48] FunctionCall with one
// argument, whose name the caller has already resolved to fn:count. Where the
// façade calls the library (ctaFacade.callsLibrary) and the argument opens as
// no path can (countsItems), the argument is one a library call takes
// (argument) — `$value`, `()`, a function call, a literal — whose items are
// counted, and the node is their ctaCount (ctaCountedItems, ctaCountOf) on
// every such façade. Every other argument is a path countArgument parses,
// whose node is p.facade's (ctaFacade.count), which may decline it, as is the
// node of a rooted argument; an argument outside those shapes declines.
func (p *ctaParser) countCall() (ctaValue, bool) {
	p.advance() // the function name
	p.advance() // '('
	if p.facade.callsLibrary() && p.countsItems() {
		operand, ok := p.argument()
		if !ok || !p.at(ctaRParen) {
			return nil, false
		}
		p.advance()
		return ctaCountOf(ctaCountedItems{operand: operand}, p.types)
	}
	arg, ok := p.countArgument()
	if !ok || !p.at(ctaRParen) {
		return nil, false
	}
	p.advance()
	return p.facade.count(arg, p.types)
}

// countsItems reports whether the cursor opens an fn:count argument no path
// opens with: a VarRef's '$', a '(' — `()` among them — a literal, or a
// function call, a name followed by '('. A path opens with "/", "//", ".",
// '@', a wildcard, an axis or a name standing alone. Nothing is consumed.
func (p *ctaParser) countsItems() bool {
	if p.at(ctaNameTok) {
		return p.peek(1).kind == ctaLParen
	}
	return p.at(ctaDollarTok) || p.at(ctaLParen) || p.at(ctaStringTok) || p.at(ctaNumberTok)
}

// countArgument parses fn:count's argument: one operand (countOperand), or
// xpath20.md [21] UnionExpr over two or more, joined by `|` or `union`
// (§3.3.3), whose node ctaUnionOf builds and which declines a rooted operand
// and one filtered by a predicate that reads the child's value.
func (p *ctaParser) countArgument() (ctaCounted, bool) {
	first, ok := p.countOperand()
	if !ok {
		return nil, false
	}
	if !p.atUnion() {
		return first, true
	}
	operands := []ctaCounted{first}
	for p.atUnion() {
		p.advance()
		next, ok := p.countOperand()
		if !ok {
			return nil, false
		}
		operands = append(operands, next)
	}
	return ctaUnionOf(operands)
}

// atUnion reports whether the cursor sits on a [21] UnionExpr operator, `|` or
// `union`, which right after a complete operand no other production opens
// with.
func (p *ctaParser) atUnion() bool { return p.at(ctaBarTok) || p.atName("union") }

// countOperand parses one operand of fn:count's argument: a path countPath
// parses, and, where a '[' follows a child step `N` or `./N`, the one
// predicate filtering it (predicate). A predicate after any other step — `.//N`,
// `@N`, a rooted step — and a second predicate leave a token no production
// takes, and decline.
func (p *ctaParser) countOperand() (ctaCounted, bool) {
	arg, ok := p.countPath()
	if !ok || !p.at(ctaLBracketTok) {
		return arg, ok
	}
	step, isPath := arg.(ctaCountPath)
	if !isPath || step.axis != ctaCountChildren {
		return nil, false
	}
	return p.predicate(step.name)
}

// predicate parses xpath20.md [40] Predicate, `"[" Expr "]"`, filtering the
// child step that selects E's children named name, as far as an fn:count
// argument admits it: a conjunction of attribute-existence tests with QName
// NameTests, `@A` or `@A1 and @A2 …`, resolved on attributeName's terms, which
// is ctaFilteredChildren, and otherwise a predicate over the child's value
// (valuePredicate).
//
// GAP(xpath): every other predicate declines — an attribute test in a
// disjunction, under fn:not, with a wildcard or atomized (`c[@a = 1]`), a
// numeric one (`c[1]`), and any predicate outside an fn:count argument but the
// conjunction of child steps childrenHaving parses and the `preceding::` step
// childrenPreceded parses where a child step's existence is asked — so a
// predicate on any other axis, `ancestor::` and `preceding-sibling::` among
// them, declines, and so does `count(N[preceding::M])`. The direction is the
// withhold [CompileAssertionTest] reports. (#1042)
func (p *ctaParser) predicate(name xsd.QName) (ctaCounted, bool) {
	p.advance() // '['
	n := p.existenceLength()
	if n == 0 || p.peek(n).kind != ctaRBracketTok {
		return p.valuePredicate(name)
	}
	var required []xsd.QName
	for end := p.pos + n; p.pos < end; p.advance() {
		if p.at(ctaAtTok) {
			p.advance()
			required = append(required, p.attributeName(p.peek(0).text))
		}
	}
	p.advance() // ']'
	return ctaFilteredChildrenOf(name, required)
}

// valuePredicate parses the predicate whose '[' the cursor has passed, over
// the children named name, as a predicate over each child's value
// (ctaMatchingChildren): an [8] ta-Test production read with `.` the child
// (ctaCandidate), typed as p.facade types a child step naming name, so a child
// that step would decline — the Type Alternative's and the facet's every one,
// and an assertion's of no single simple typed value — declines here too, and
// so does one that step reads untyped, a child of mixed content
// (ctaUntypedChild): a candidate is typed, and an untyped one would need an arm
// of its own. Every other production reads ctaPredicateFacade, which declines
// each read of a node — an attribute, a child or element step, a path, `$value`
// — an fn:count call and every F&O function. The root must be a comparison, or
// and, or and fn:not over comparisons (ctaComparisonRooted): a bare value may
// be numeric, which selects by position.
func (p *ctaParser) valuePredicate(name xsd.QName) (ctaCounted, bool) {
	step, typed := p.facade.child(ctaExactName{name: name}, p.types)
	child, isChild := step.(ctaTypedChild)
	if !typed || !isChild {
		return nil, false
	}
	outer := p.facade
	p.facade = ctaPredicateFacade{candidate: ctaCandidate{st: child.st}}
	pred, parsed := p.orExpr()
	p.facade = outer
	if !parsed || !p.at(ctaRBracketTok) || !ctaComparisonRooted(pred) {
		return nil, false
	}
	p.advance() // ']'
	return ctaMatchingChildren{name: name, pred: pred}, true
}

// ctaPredicateFacade is the façade a value predicate's own expression parses
// under (ctaParser.valuePredicate): its context item is candidate, the child
// the predicate filters, so every production that reads another node
// declines — an attribute of the candidate would need its own type, a step
// below it a subtree this engine does not keep — and so does `$value`, an
// fn:count call, and every F&O function. Comparisons and arithmetic are
// admitted over what remains, the candidate, literals and casts.
type ctaPredicateFacade struct{ candidate ctaCandidate }

func (ctaPredicateFacade) ctaFacade() {}

func (ctaPredicateFacade) attribute(ctaNameTest, ctaTypes) (ctaValue, bool) { return nil, false }

// comparesValues is true, on ctaAssertionFacade.comparesValues' terms.
func (ctaPredicateFacade) comparesValues() bool { return true }

func (ctaPredicateFacade) variable(xsd.QName, ctaTypes) (ctaValue, bool) { return nil, false }

func (ctaPredicateFacade) child(ctaNameTest, ctaTypes) (ctaValue, bool) { return nil, false }

func (ctaPredicateFacade) childPath([]xsd.QName, ctaElementTest) (ctaValue, bool) {
	return nil, false
}

// attributePath declines. It is never reached: callsLibrary is false, so
// `sum(…)` in a predicate reaches constructorFunction and declines there.
func (ctaPredicateFacade) attributePath([]xsd.QName, ctaNameTest, ctaTypes) (ctaValue, bool) {
	return nil, false
}

func (ctaPredicateFacade) childrenHaving(xsd.QName, []xsd.QName) (ctaValue, bool) {
	return nil, false
}

func (ctaPredicateFacade) childrenPreceded(ctaChildrenPreceded) (ctaValue, bool) {
	return nil, false
}

func (ctaPredicateFacade) elements(ctaCountPath) (ctaValue, bool) { return nil, false }

func (ctaPredicateFacade) quantified(ctaQuantifier, ctaRangeKey) (ctaExpr, bool) { return nil, false }

// rangeScope declines, on quantified's terms. It is never reached: variable
// declines `$value` before the binding sequence is compiled.
func (ctaPredicateFacade) rangeScope(xsd.QName, ctaValueVar) (ctaFacade, bool) { return nil, false }

// childScope declines, on quantified's terms. It is never reached: child
// declines the binding step before the body's scope is asked for.
func (ctaPredicateFacade) childScope(xsd.QName, ctaTypedChild) (ctaFacade, bool) { return nil, false }

// boundSibling declines, on variable's terms: no variable is in scope.
func (ctaPredicateFacade) boundSibling(xsd.QName, ctaNameTest, ctaTypes) (ctaValue, bool) {
	return nil, false
}

func (ctaPredicateFacade) rooted() (ctaValue, bool) { return nil, false }

// contextItem compiles `.` to the candidate child (ctaCandidate).
func (f ctaPredicateFacade) contextItem() (ctaValue, bool) { return f.candidate, true }

func (ctaPredicateFacade) count(ctaCounted, ctaTypes) (ctaValue, bool) { return nil, false }

// contextNode declines: inside a predicate `.` is the candidate child and not
// E. It is never reached, callsLibrary being false.
func (ctaPredicateFacade) contextNode() (ctaNodeArg, bool) { return nil, false }

// focus declines. It is never reached: callsLibrary is false, so `position()`
// and `last()` in a predicate reach constructorFunction and decline there,
// under ctaParser.predicate's GAP(xpath).
func (ctaPredicateFacade) focus(*xsd.SimpleType) (ctaValue, bool) { return nil, false }

// computes is true, on ctaAssertionFacade.computes' terms.
func (ctaPredicateFacade) computes() bool { return true }

// callsLibrary is false: a library call in a predicate declines at
// constructorFunction, so `string-length(.)`, position() and last() never
// reach a node.
func (ctaPredicateFacade) callsLibrary() bool { return false }

// conditional is false: a branch may be a bare value, which a predicate's root
// may not be (ctaComparisonRooted).
func (ctaPredicateFacade) conditional() bool { return false }

// constructsSequences is false: a value predicate compares the candidate
// against literals, casts and arithmetic alone, and `N[. = (1 to 3)]` declines
// with every other predicate outside that shape (ctaParser.predicate).
func (ctaPredicateFacade) constructsSequences() bool { return false }

// castable is false, on constructsSequences' terms: `N[. castable as xs:int
// = 'true']` declines with every other predicate outside that shape.
func (ctaPredicateFacade) castable() bool { return false }

// instanceOf is false, on castable's terms: `N[. instance of xs:int]`
// declines with every other predicate outside that shape.
func (ctaPredicateFacade) instanceOf() bool { return false }

// existenceLength is how many tokens at the cursor spell a conjunction of
// attribute-existence tests, `'@' QName ('and' '@' QName)*`, and 0 where they
// spell none. Nothing is consumed.
func (p *ctaParser) existenceLength() int {
	n := 0
	for {
		if p.peek(n).kind != ctaAtTok || p.peek(n+1).kind != ctaNameTok {
			return 0
		}
		n += 2
		if tok := p.peek(n); tok.kind != ctaNameTok || tok.text != "and" {
			return n
		}
		n++
	}
}

// countPath parses one counted path: a rooted path (rootedPath), whose node
// p.facade builds and which is an argument only where it is ctaCounted —
// ctaNoDocumentRoot, which raises before it selects a node — or a relative
// path of one step with a QName NameTest, `N`, `@N`, or either behind `./` or
// `.//` (countStep). A longer path, a wildcard and a bare `.` leave a token no
// production takes, or none at all, and decline. selectedElements parses its
// element steps too.
func (p *ctaParser) countPath() (ctaCounted, bool) {
	if p.at(ctaSlashTok) || p.at(ctaSlashSlashTok) {
		rooted, ok := p.rootedPath()
		if !ok {
			return nil, false
		}
		counted, isCounted := rooted.(ctaCounted)
		return counted, isCounted
	}
	if !p.at(ctaDotTok) {
		return p.countStep(ctaCountChildren, ctaCountOwnAttributes)
	}
	p.advance()
	if p.at(ctaSlashSlashTok) {
		p.advance()
		return p.countStep(ctaCountDescendants, ctaCountSubtreeAttributes)
	}
	if !p.at(ctaSlashTok) {
		return nil, false
	}
	p.advance()
	return p.countStep(ctaCountChildren, ctaCountOwnAttributes)
}

// countStep parses the one step of a counted relative path: `'@' QName`,
// resolved on attributeName's terms and counted on the axis attributes, or a
// QName, resolved on elementName's terms and counted on the axis elements. The
// step is never typed: fn:count does not atomize it (ctaCount).
func (p *ctaParser) countStep(elements, attributes ctaCountAxis) (ctaCounted, bool) {
	if p.at(ctaAtTok) {
		p.advance()
		if !p.at(ctaNameTok) {
			return nil, false
		}
		name := p.attributeName(p.peek(0).text)
		p.advance()
		return ctaCountPath{axis: attributes, name: name}, true
	}
	if !p.at(ctaNameTok) {
		return nil, false
	}
	name := p.elementName(p.peek(0).text)
	p.advance()
	return ctaCountPath{axis: elements, name: name}, true
}

// castExpr parses [15] ta-CastExpr: a SimpleValue and an optional cast tail —
// or, in its place, xpath20.md [18] CastableExpr's `castable as` tail
// (castableTail).
//
// The tail's QName is resolved as a TYPE name and classified before the node
// is built, so a target this engine does not cast to declines the whole
// expression here rather than deciding anything at evaluation time
// (ctaTypes.castTarget). §3.12.6 clause 4 fixes what an admitted one is: "Any
// explicit casts (i.e. any strings which match the optional "cast as" QName in
// the CastExpr production) are casts to built-in datatypes."
//
// GAP(xpath): a `castable as` tail after a `cast as` one, `E cast as T
// castable as U`, which [18]'s CastExpr operand admits, leaves `castable` a
// token no production takes, and declines; so does a `castable as` tail over
// any operand but a [16] ta-SimpleValue, such as `xs:date(@d) castable as
// xs:string`. The direction is the withhold [CompileAssertionTest] reports.
// (#1042)
func (p *ctaParser) castExpr() (ctaValue, bool) {
	v, ok := p.simpleValue()
	if !ok {
		return nil, false
	}
	if p.atName("castable") {
		return p.castableTail(v)
	}
	if !p.atName("cast") {
		return v, true
	}
	p.advance()
	cast, ok := p.singleType(v)
	if !ok {
		return nil, false
	}
	return cast, true
}

// castableTail parses xpath20.md [18] CastableExpr's `"castable" "as"
// SingleType` tail over v, whose `castable` the cursor is on, where the façade
// admits it (ctaFacade.castable), so a Type Alternative's {test} declines it
// as outside its required subset. It holds the cast `v cast as` the same
// SingleType builds (singleType), so a target or operand castTarget or
// castsFrom declines for the cast declines for `castable` too, and no second
// casting rule exists to disagree with the cast's (ctaCastable).
func (p *ctaParser) castableTail(v ctaValue) (ctaValue, bool) {
	if !p.facade.castable() {
		return nil, false
	}
	p.advance() // 'castable'
	cast, ok := p.singleType(v)
	if !ok {
		return nil, false
	}
	boolean, resolved := p.types.simple(ctaBuiltin("boolean"))
	if !resolved {
		return nil, false
	}
	return ctaCastable{cast: cast, st: boolean}, true
}

// singleType parses the `'as' QName '?'?` following a `cast` or `castable`
// keyword — xpath20.md [49] SingleType — into the cast of v to the type the
// QName names, reporting false where the tail is malformed, castTarget does
// not admit the target or castsFrom does not admit the cast.
func (p *ctaParser) singleType(v ctaValue) (ctaCast, bool) {
	if !p.atName("as") {
		return ctaCast{}, false
	}
	p.advance()
	if !p.at(ctaNameTok) {
		return ctaCast{}, false
	}
	text := p.peek(0).text
	p.advance()
	allowsEmpty := false
	if p.at(ctaQuestionTok) {
		p.advance()
		allowsEmpty = true
	}
	target, admitted := p.types.castTarget(p.typeName(text))
	if !admitted || !p.types.castsFrom(v, target) {
		return ctaCast{}, false
	}
	return ctaCast{operand: v, target: target, allowsEmpty: allowsEmpty}, true
}

// constructorFunction parses [18] ta-ConstructorFunction.
//
// §3.12.6 clause 3 makes every QName '(' SimpleValue ')' whose name is not
// fn:not a constructor call for a built-in datatype, so this one production
// covers both the constructor spelling of a cast and the "unknown boolean
// function" reading — there is no third case to distinguish. The calls beyond
// §3.12.6 are fn:count and, on a façade that calls the library, the F&O
// functions libraryCall names, which valueExpr hands to countCall and
// libraryCall instead. The name is a FUNCTION name, resolved as one, and it
// names the datatype at the same time: an unprefixed int(...) is fn:int,
// which declares no constructor, and never xs:int.
//
// The node is castExpr's, with allowsEmpty TRUE unconditionally: xpath20.md
// §3.10.4 defines T($arg) as (($arg) cast as T?), and the `?` is part of that
// equivalence however [18]'s own production is written — so a constructor call
// over an absent attribute is the empty sequence where the same cast written
// without `?` would be err:XPTY0004.
//
// The operand is a [16] ta-SimpleValue on a façade that calls no library
// function, the Type Alternative's and a value predicate's, and otherwise any
// argument a library call takes (argument): xpath20.md §3.1.5 makes a
// constructor function's argument an ExprSingle, and [18]'s `SimpleValue`
// restricts the Type Alternative subset alone (§3.12.6), so on the assertion
// and facet façades `xs:date(concat(string($value), '!!!'))` is the cast of a
// function result, judged by castsFrom on its static type.
func (p *ctaParser) constructorFunction() (ctaValue, bool) {
	name := p.functionName(p.peek(0).text)
	p.advance() // the function name
	p.advance() // '('
	arg, ok := p.constructorOperand()
	if !ok {
		return nil, false
	}
	if !p.at(ctaRParen) {
		return nil, false
	}
	p.advance()
	target, admitted := p.types.castTarget(name)
	if !admitted || !p.types.castsFrom(arg, target) {
		return nil, false
	}
	return ctaCast{operand: arg, target: target, allowsEmpty: true}, true
}

// constructorOperand parses the operand of [18] ta-ConstructorFunction on
// constructorFunction's terms: argument where the façade calls the library
// (ctaFacade.callsLibrary), and simpleValue where it does not.
func (p *ctaParser) constructorOperand() (ctaValue, bool) {
	if p.facade.callsLibrary() {
		return p.argument()
	}
	return p.simpleValue()
}

// simpleValue parses [16] ta-SimpleValue's two arms, the arms the assertion
// façade adds — [44] VarRef (varRef), a child-axis step (childStep), and a
// rooted path (rootedPath) — and [47] ContextItemExpr, which the assertion,
// facet and predicate façades add. A name opens the unabbreviated attribute axis only
// where `::` follows the name `attribute`, and a child-axis step otherwise.
func (p *ctaParser) simpleValue() (ctaValue, bool) {
	switch p.peek(0).kind {
	case ctaAtTok:
		return p.attrName()
	case ctaNameTok:
		if p.atName("attribute") && p.peek(1).kind == ctaAxisTok {
			return p.attrName()
		}
		return p.childStep()
	case ctaSlashTok, ctaSlashSlashTok:
		return p.rootedPath()
	case ctaDollarTok:
		if p.atBoundSibling() {
			return p.boundSibling()
		}
		return p.varRef()
	case ctaDotTok:
		p.advance()
		return p.facade.contextItem()
	case ctaStringTok:
		text := p.peek(0).text
		p.advance()
		return ctaLiteral{text: text, st: p.types.str}, true
	case ctaNumberTok:
		text := p.peek(0).text
		p.advance()
		return ctaLiteral{text: text, st: p.types.literal(text)}, true
	default:
		return nil, false
	}
}

// varRef parses xpath20.md [44] VarRef, `'$' VarName`, whose [45] VarName is a
// QName. An unprefixed one is in NO namespace, which is where cvc-assertion
// clause 2.3 puts `$value` ("no namespace URI and ... "value" as the local
// name"); a prefixed one resolves against the {namespace bindings} on
// attributeName's terms. The node is p.facade's, which declines every name but
// the one variable its static context holds.
func (p *ctaParser) varRef() (ctaValue, bool) {
	p.advance() // '$'
	if !p.at(ctaNameTok) {
		return nil, false
	}
	text := p.peek(0).text
	p.advance()
	return p.facade.variable(p.attributeName(text), p.types)
}

// atBoundSibling reports whether the tokens at the cursor spell
// `'$' VarName '/' 'following-sibling' '::' QName '[' '1' ']'`, a path over a
// variable that boundSibling parses. Nothing is consumed.
func (p *ctaParser) atBoundSibling() bool {
	return p.peek(0).kind == ctaDollarTok && p.peek(1).kind == ctaNameTok && p.peek(2).kind == ctaSlashTok &&
		p.peek(3) == ctaSiblingSteps[0] && p.peek(4) == ctaSiblingSteps[1] && p.peek(5).kind == ctaNameTok &&
		p.peek(6) == ctaToken{kind: ctaLBracketTok} && p.peek(7) == ctaToken{kind: ctaNumberTok, text: "1"} &&
		p.peek(8) == ctaToken{kind: ctaRBracketTok}
}

// boundSibling parses `$v/following-sibling::M[1]`, which atBoundSibling
// found at the cursor, the VarName resolved on varRef's terms and M on
// elementName's — the following-sibling axis's principal node kind is element
// (xpath20.md §3.2.1.2) — into the node p.facade compiles it to
// (ctaFacade.boundSibling), which may decline it. `[1]` is the IntegerLiteral
// `1` alone: §3.2.2 makes it positional, the first M sibling however far.
func (p *ctaParser) boundSibling() (ctaValue, bool) {
	variable := p.attributeName(p.peek(1).text)
	next := ctaExactName{name: p.elementName(p.peek(5).text)}
	p.pos += 9
	return p.facade.boundSibling(variable, next, p.types)
}

// childStep parses one abbreviated child-axis step, xpath20.md [31]
// AbbrevForwardStep without its '@' — "If the axis name is omitted from an
// axis step, the default axis is child" (§3.2.4) — whose NodeTest is a QName
// NameTest, resolved on elementName's terms. The node is p.facade's, which
// may decline it. A [37] Wildcard NameTest is a ctaWildcardTok and never
// reaches here, and `child::` spelled out is a name followed by a `::` no
// production takes: both decline.
func (p *ctaParser) childStep() (ctaValue, bool) {
	text := p.peek(0).text
	p.advance()
	return p.facade.child(ctaExactName{name: p.elementName(text)}, p.types)
}

// childPathLength is how many tokens, from offset at ahead of the cursor,
// spell a relative path of two or more abbreviated child-axis steps, each but
// the last with a QName NameTest and the last with a QName or the bare [37]
// Wildcard `*`, `QName ('/' QName)* '/' (QName | '*')`, and 0 where they spell
// none. A name followed by '(' or '::' is a function call or an axis spelled
// out, never such a step, and so is a '/' followed by anything but a name or
// `*`: each answers 0, as does one step alone, which is childStep's. A `*` ends
// the path, and `*:N` and `p:*` are no `*` here. Nothing is consumed.
func (p *ctaParser) childPathLength(at int) int {
	n := 0
	for {
		if p.peek(at+n).kind != ctaNameTok {
			return 0
		}
		next := p.peek(at + n + 1).kind
		if next == ctaLParen || next == ctaAxisTok {
			return 0
		}
		n++
		if next != ctaSlashTok {
			break
		}
		n++
		if tok := p.peek(at + n); tok.kind == ctaWildcardTok && tok.text == "*" {
			n++
			break
		}
	}
	if n < 3 {
		return 0
	}
	return n
}

// closesBoolean reports whether the token at offset at ahead of the cursor ends
// a [11] ta-BooleanExpr: the end of the expression, the ')' of a parenthesized
// OrExpr, of fn:not or of an IfExpr's test, the 'and' or 'or' of the expression
// enclosing it, or the 'else' ending an IfExpr's then-branch (ifExpr). An
// 'else' outside an IfExpr is a token no production takes, so the parse ends
// unsupported after the path as it would before it.
func (p *ctaParser) closesBoolean(at int) bool {
	tok := p.peek(at)
	if tok.kind == ctaEOF || tok.kind == ctaRParen {
		return true
	}
	return tok.kind == ctaNameTok && (tok.text == "and" || tok.text == "or" || tok.text == "else")
}

// childPath parses the n tokens childPathLength measured at the cursor as
// xpath20.md [26] RelativePathExpr, `StepExpr ("/" StepExpr)+`, each step an
// abbreviated child-axis step (§3.2.1.1, §3.2.4) whose QName NameTest is
// resolved on elementName's terms, so an unprefixed one takes the {default
// namespace}, and whose last step may instead be the [37] Wildcard `*`, which
// on the child axis matches every element (ctaAnyName, §3.2.1.2). It is the
// ONE place a ctaChildPath is built, and it is reached from two positions
// alone: the ·effective boolean value· arm of booleanExpr and fn:exists or
// fn:empty's argument (presenceArgument), each where the path is the whole
// operand — the three positions that ask only whether the path selects a
// node, and never atomize it. simpleValue and additiveExpr never reach it, so
// no comparison, operator, cast or other call takes one, and childStep stays
// one step; one step in the same positions is selectedElements'. The node is
// p.facade's, which may decline it.
//
// GAP(xpath): a path in any other position, or with a wildcard on a step but
// the last or a `*:N` or `p:*` one on any step, a predicate, an axis spelled
// out, a '//' between steps or an attribute step, declines; the direction is
// the withhold [CompileAssertionTest] reports. (#1042)
func (p *ctaParser) childPath(n int) (ctaValue, bool) {
	var parents []xsd.QName
	end := p.pos + n - 1
	for ; p.pos < end; p.advance() {
		if p.at(ctaSlashTok) {
			continue
		}
		parents = append(parents, p.elementName(p.peek(0).text))
	}
	var last ctaElementTest = ctaAnyName{}
	if p.at(ctaNameTok) {
		last = ctaExactName{name: p.elementName(p.peek(0).text)}
	}
	p.advance()
	return p.facade.childPath(parents, last)
}

// childrenHavingLength is how many tokens, from offset at ahead of the cursor,
// spell a child step with a QName NameTest filtered by one [40] Predicate that
// is a conjunction of child steps with QName NameTests, `QName '[' QName ('and'
// QName)* ']'`, and 0 where they spell none. A name inside the brackets
// followed by anything but 'and' or ']' — a '/', a '(', a '::', a comparator —
// spells none. Nothing is consumed.
func (p *ctaParser) childrenHavingLength(at int) int {
	if p.peek(at).kind != ctaNameTok || p.peek(at+1).kind != ctaLBracketTok {
		return 0
	}
	n := 2
	for {
		if p.peek(at+n).kind != ctaNameTok {
			return 0
		}
		n++
		tok := p.peek(at + n)
		if tok.kind == ctaRBracketTok {
			return n + 1
		}
		if tok.kind != ctaNameTok || tok.text != "and" {
			return 0
		}
		n++
	}
}

// childrenHaving parses the tokens childrenHavingLength measured at the cursor
// as a child step filtered by xpath20.md [40] Predicate, `N[a and b …]`, each
// name resolved on elementName's terms, into the node p.facade builds for it
// (ctaFacade.childrenHaving), which may decline it. The predicate is no number,
// so it filters each child named N by its ·effective boolean value· (§3.2.2),
// and each conjunct is a child step of that child, whose ·effective boolean
// value· is whether it selects a node (§2.4.3 rule 2). It is the ONE place a
// ctaChildrenHaving is built, reached from childPath's two positions alone and
// on its terms.
func (p *ctaParser) childrenHaving() (ctaValue, bool) {
	name := p.elementName(p.peek(0).text)
	p.advance() // the step's name
	p.advance() // '['
	required := []xsd.QName{p.elementName(p.peek(0).text)}
	p.advance()
	// childrenHavingLength measured a name after each 'and', and ']' after the
	// last name.
	for p.atName("and") {
		p.advance()
		required = append(required, p.elementName(p.peek(0).text))
		p.advance()
	}
	p.advance() // ']'
	return p.facade.childrenHaving(name, required)
}

// childrenPrecededLength is how many tokens, from offset at ahead of the
// cursor, spell a child step with a QName NameTest filtered by one [40]
// Predicate that is one `preceding::` step with a QName NameTest, itself
// filtered by at most one predicate that is a QName child step or a call of
// one name over one, `QName '[' 'preceding' '::' QName ( '[' QName ']' | '['
// QName '(' QName ')' ']' )? ']'`, and 0 where they spell none. Nothing is
// consumed, and the called name is not resolved: childrenPreceded declines
// any but fn:not.
func (p *ctaParser) childrenPrecededLength(at int) int {
	if p.peek(at).kind != ctaNameTok || p.peek(at+1).kind != ctaLBracketTok {
		return 0
	}
	if tok := p.peek(at + 2); tok.kind != ctaNameTok || tok.text != "preceding" {
		return 0
	}
	if p.peek(at+3).kind != ctaAxisTok || p.peek(at+4).kind != ctaNameTok {
		return 0
	}
	n := 5
	if p.peek(at+n).kind == ctaLBracketTok {
		n += p.precedingFilterLength(at + n)
	}
	if p.peek(at+n).kind != ctaRBracketTok {
		return 0
	}
	return n + 1
}

// precedingFilterLength is how many tokens, from offset at ahead of the cursor
// where a '[' stands, spell `'[' QName ']'` or `'[' QName '(' QName ')' ']'`,
// and 0 where they spell neither.
func (p *ctaParser) precedingFilterLength(at int) int {
	if p.peek(at+1).kind != ctaNameTok {
		return 0
	}
	if p.peek(at+2).kind == ctaRBracketTok {
		return 3
	}
	called := p.peek(at+2).kind == ctaLParen && p.peek(at+3).kind == ctaNameTok &&
		p.peek(at+4).kind == ctaRParen && p.peek(at+5).kind == ctaRBracketTok
	if !called {
		return 0
	}
	return 6
}

// childrenPreceded parses the tokens childrenPrecededLength measured at the
// cursor as a child step filtered by xpath20.md [40] Predicate,
// `N[preceding::M]`, `N[preceding::M[P]]` or `N[preceding::M[not(P)]]`, each
// name resolved on elementName's terms — the preceding axis's principal node
// kind is element (§3.2.1.2) — and the called name as a function's, into the
// node p.facade builds for it (ctaFacade.childrenPreceded), which may decline
// it. Each predicate is no number, so it filters by its ·effective boolean
// value· (§3.2.2): the outer one whether the preceding step selects a node, the
// inner one whether M has a child named P, or, under fn:not, has none (§2.4.3
// rule 2). It is the ONE place a ctaChildrenPreceded is built, reached from
// childPath's two positions alone and on its terms.
//
// GAP(xpath): every other step in such a predicate declines — `ancestor::`,
// `ancestor-or-self::`, `preceding-sibling::`, `following::`,
// `following-sibling::`, `descendant::` and every other axis spelled out, a
// wildcard or kind test on the preceding step, a predicate on it that is no
// QName child step or fn:not over one — `[not(b) and c]`, `[b/c]`, `[@b]`,
// `[*]`, `[1]` — a second predicate on it, a step after it, and a call of any
// function but fn:not; so does a preceding step on `./N`, `.//N` or a longer
// path, under fn:count, or read for its value. The direction is the withhold
// [CompileAssertionTest] reports. (#1042)
func (p *ctaParser) childrenPreceded() (ctaValue, bool) {
	name := p.elementName(p.peek(0).text)
	p.advance() // the step's name
	p.advance() // '['
	p.advance() // 'preceding'
	p.advance() // '::'
	preceded := ctaChildrenPreceded{name: name, preceding: p.elementName(p.peek(0).text), test: ctaAnyPreceding{}}
	p.advance()
	if p.at(ctaLBracketTok) {
		p.advance() // '['
		negated := p.peek(1).kind == ctaLParen
		if negated {
			if p.functionName(p.peek(0).text) != ctaNotFunction {
				return nil, false
			}
			p.advance() // 'not'
			p.advance() // '('
		}
		preceded.test = ctaPrecedingChild{child: p.elementName(p.peek(0).text), negated: negated}
		p.advance()
		if negated {
			p.advance() // ')'
		}
		p.advance() // ']'
	}
	p.advance() // ']'
	return p.facade.childrenPreceded(preceded)
}

// selectedStepLength is how many tokens, from offset at ahead of the cursor,
// spell one of countPath's element steps — a QName, `N`, alone or behind
// `./` or `.//` — and 0 where they spell none. Nothing is consumed, and nothing
// after the name is read: a name followed by '(' or '::' is a function call or
// an axis spelled out, which the caller's check that the token after the step
// closes the operand turns away.
func (p *ctaParser) selectedStepLength(at int) int {
	n := 0
	if p.peek(at).kind == ctaDotTok {
		if next := p.peek(at + 1).kind; next != ctaSlashTok && next != ctaSlashSlashTok {
			return 0
		}
		n = 2
	}
	if p.peek(at+n).kind != ctaNameTok {
		return 0
	}
	return n + 1
}

// selectedElements parses the step selectedStepLength measured at the cursor
// on countPath's grammar — `N` and `./N` the element children of E named
// N, `.//N` every element below E so named (xpath20.md §3.2.4) — into the node
// p.facade builds for it (ctaFacade.elements), which may decline it. It is the
// ONE place a ctaSelectedElements is built, reached from childPath's two
// positions alone and on its terms: the ·effective boolean value· arm of
// booleanExpr and fn:exists or fn:empty's argument (presenceArgument), where
// the step is the whole operand, whatever type the step's name has. Everywhere
// else a step is childStep's, whose value is read, and `./N` and `.//N`
// decline.
func (p *ctaParser) selectedElements() (ctaValue, bool) {
	arg, ok := p.countPath()
	if !ok {
		return nil, false
	}
	path, relative := arg.(ctaCountPath)
	if !relative {
		return nil, false
	}
	return p.facade.elements(path)
}

// rootedPath parses xpath20.md [25] PathExpr's two rooted arms, "/"
// RelativePathExpr and "//" RelativePathExpr, as far as ONE step with a QName
// NameTest: a child-axis one, or an abbreviated attribute one, `'@' QName`; a
// longer path, a wildcard, and a bare "/" leave a token no production takes.
// The step is resolved — an unbound prefix in it is err:XPST0081 like any other,
// on elementName's or attributeName's terms — and never typed: the node is
// p.facade's, and each façade that admits it builds a node that raises before
// any step is taken (ctaNoDocumentRoot, ctaNoContextItem), so what the step
// would select is never asked.
func (p *ctaParser) rootedPath() (ctaValue, bool) {
	p.advance() // '/' or '//'
	attribute := p.at(ctaAtTok)
	if attribute {
		p.advance()
	}
	if !p.at(ctaNameTok) {
		return nil, false
	}
	p.rootedStepName(attribute, p.peek(0).text)
	p.advance()
	return p.facade.rooted()
}

// rootedStepName resolves the NameTest text of rootedPath's one step, on
// attributeName's terms where attribute is true and elementName's otherwise.
// The name is discarded: resolving it is what records an unbound prefix.
func (p *ctaParser) rootedStepName(attribute bool, text string) {
	if attribute {
		p.attributeName(text)
		return
	}
	p.elementName(text)
}

// attrName parses [17] ta-AttrName in BOTH spellings ta-props-correct clause 2
// admits, which is disjunctive: clause 2.1's abbreviated `'@' NameTest`, and
// clause 2.2's "XPath expression involving the attribute axis whose
// abbreviated form is as given above", i.e. `attribute::NameTest`.
//
// BOTH arms of [36] NameTest are admitted, in both spellings: the QName one,
// and xpath20.md's [37] Wildcard, which [17] reaches because it names NameTest
// rather than a QName. ctaWildcardTok is accepted as a NameTest HERE and as
// the bare `*` ending a child path (childPathLength), and in no other NameTest
// position.
//
// The node the resolved NameTest becomes is p.facade's, which may decline it.
func (p *ctaParser) attrName() (ctaValue, bool) {
	switch {
	case p.at(ctaAtTok):
		p.advance()
	case p.atName("attribute") && p.peek(1).kind == ctaAxisTok:
		p.advance()
		p.advance()
	default:
		return nil, false
	}
	text := p.peek(0).text
	switch p.peek(0).kind {
	case ctaNameTok:
		p.advance()
		return p.facade.attribute(ctaExactName{name: p.attributeName(text)}, p.types)
	case ctaWildcardTok:
		p.advance()
		return p.facade.attribute(p.wildcardTest(text), p.types)
	default:
		return nil, false
	}
}
