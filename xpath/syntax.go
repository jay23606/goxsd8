package xpath

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// This file is the RECOGNIZER for the full XPath 2.0 grammar, xpath20.md
// Appendix A: A.1's productions [1] XPath through [70] TypeName, under A.1.2's
// extra-grammatical constraints (leading-lone-slash, reserved-function-names,
// occurrence-indicators) and A.2's lexical structure. It answers two questions
// and builds nothing: no tree, no static type. The first is whether a string
// is an XPath 2.0 Expr at all; a failure is err:XPST0003 (xpath20.md §2.2.3.1:
// "an expression is not a valid instance of the grammar defined in A.1 EBNF"),
// which [CTATestStaticError] reports for a Type Alternative {test} the subset
// parser (ctaparser.go) declined. The second, asked only with a static context
// (syntaxStatic), is the first static error in expression order the names of
// such a {test} carry, which [CTATestStaticError] reports where the subset
// parser declined the text with none proved (ctaFullStaticError).
//
// It is not the subset parser's rival: that parser builds the one tree the
// engine evaluates and admits a strict SUBSET of what this recognizer accepts,
// so the two never disagree about a text the subset admits. What this one adds
// is the other direction — telling a legal full-XPath {test} the subset merely
// declines (a withhold) from text that is no XPath at all (a charge).
//
// Its lexical pieces are the subset lexer's own where the two read the same
// terminal (STYLE T4): ctaScanNCName for [79] NCName, ctaScanString for [74]
// StringLiteral, ctaScanNumber for [71]-[73] NumericLiteral, ctaSkipComment for
// [77] Comment, ctaScanNameTest for [78] QName and [37]'s `NCName ':' '*'`, and
// ctaScanWildcardTail for [37]'s `'*' ':' NCName`. White space
// is XML's S, #x20 #x9 #xD #xA (A.2.4.1), and nothing else.

// syntaxKind identifies one terminal of A.2's lexical structure as the
// recognizer reads it.
type syntaxKind byte

const (
	// syntaxEOF ends every token stream, so a production reading past the
	// last terminal sees a kind no production takes.
	syntaxEOF syntaxKind = iota
	// syntaxName is a [78] QName, the spelling every keyword arrives under:
	// keywords are not reserved (A.2), so which ones a name is depends on the
	// position the recognizer reads it in.
	syntaxName
	// syntaxWildcard is [37]'s `NCName ':' '*'` or `'*' ':' NCName`. A bare
	// `*` is syntaxStar, which is also the multiplication operator.
	syntaxWildcard
	syntaxStar
	syntaxNumber
	syntaxString
	syntaxDollar
	syntaxLParen
	syntaxRParen
	syntaxLBracket
	syntaxRBracket
	syntaxComma
	syntaxDot
	syntaxDotDot
	syntaxSlash
	syntaxSlashSlash
	syntaxAxis // '::'
	syntaxAt
	syntaxQuestion
	syntaxPlus
	syntaxMinus
	syntaxBar
	// syntaxComp is a [22] GeneralComp or the '<<' and '>>' of [24] NodeComp;
	// the word-spelled comparators of [23] ValueComp and NodeComp's 'is' are
	// syntaxName.
	syntaxComp
)

// syntaxToken is one terminal: its kind, its source spelling for a name and for
// an `NCName ':' '*'` wildcard — the one wildcard spelling whose prefix the
// static pass resolves — and the byte offset it starts at, which is what a
// failure reports. A name is prefixed exactly when its text holds a ':', so no
// keyword list below — none of whose words holds one — ever matches a prefixed
// name.
type syntaxToken struct {
	kind syntaxKind
	text string
	at   int
}

// ruleXPST0003 is the XPath static error a text outside A.1's grammar is
// (xpath20.md §2.2.3.1: "an expression is not a valid instance of the grammar
// defined in A.1 EBNF"), carried as the [xsderr.Rule] of the error
// [CTATestStaticError] returns for one and read off the wrapped cause exactly
// as ruleXPST0081 is.
const ruleXPST0003 xsderr.Rule = "err:XPST0003"

// syntaxError is the err:XPST0003 verdict over expr, or nil where expr is an
// XPath 2.0 expression. The message names the expression and where the
// grammar stopped: a byte offset into it, or its end.
func syntaxError(expr string) *xsderr.Error {
	at, bad := xpathSyntaxError(expr)
	if !bad {
		return nil
	}
	where := fmt.Sprintf("takes nothing at byte offset %d", at)
	if at == len(expr) {
		where = "needs more than its end"
	}
	return xsderr.New(ruleXPST0003, xsderr.Loc{},
		"%q is not an XPath 2.0 expression: the grammar %s, which err:XPST0003 forbids", expr, where)
}

// xpathSyntaxError reports whether s is NOT an XPath 2.0 expression — no
// derivation of [1] XPath from A.1's grammar covers it — and, when it is not,
// the byte offset of the first terminal (or character, for a lexical failure)
// no derivation can take; len(s) means the text ended too soon.
func xpathSyntaxError(s string) (at int, bad bool) {
	toks, at, ok := syntaxTokenize(s)
	if !ok {
		return at, true
	}
	r := syntaxRecognizer{toks: toks}
	if r.expr() && r.peek().kind == syntaxEOF {
		return 0, false
	}
	return r.peek().at, true
}

// syntaxTokenize splits s into A.2's terminals, reporting false and the
// offending offset for text no terminal covers: a character outside every
// terminal and outside XML's S, an unclosed StringLiteral or Comment, a ':'
// outside a QName, a Wildcard or '::', a '!' outside '!=', and two
// non-delimiting terminals with no separator between them where A.2.2 needs
// one (`10div 3`, `1.2.3`).
//
// The longest match is taken (A.2), which is A.2.4.1's `foo-foo` as one QName
// and `foo- foo` as the QName `foo-` and another. Comments are separators and
// produce no token; a Comment can never sit inside a name, a number, a string
// or a Wildcard (ws: explicit), because each of those is scanned as one token.
func syntaxTokenize(s string) ([]syntaxToken, int, bool) {
	var toks []syntaxToken
	emit := func(kind syntaxKind, at int) { toks = append(toks, syntaxToken{kind: kind, at: at}) }
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case strings.HasPrefix(s[i:], "(:"):
			j, ok := ctaSkipComment(s, i)
			if !ok {
				return nil, i, false
			}
			i = j
		case strings.HasPrefix(s[i:], "::"):
			emit(syntaxAxis, i)
			i += 2
		case strings.HasPrefix(s[i:], ".."):
			emit(syntaxDotDot, i)
			i += 2
		case strings.HasPrefix(s[i:], "//"):
			emit(syntaxSlashSlash, i)
			i += 2
		case c == '!' || c == '=' || c == '<' || c == '>':
			n := syntaxComparatorLen(s[i:])
			if n == 0 {
				return nil, i, false
			}
			emit(syntaxComp, i)
			i += n
		case c == '"' || c == '\'':
			_, j, ok := ctaScanString(s, i)
			if !ok {
				return nil, i, false
			}
			emit(syntaxString, i)
			i = j
		case c >= '0' && c <= '9', c == '.' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9':
			j := ctaScanNumber(s, i)
			// A.2.2: a numeric literal is non-delimiting, so a name starting
			// at its end would need a separator, and "." needs one beside it.
			if j < len(s) && (s[j] == '.' || ctaScanNCName(s, j) > j) {
				return nil, j, false
			}
			emit(syntaxNumber, i)
			i = j
		case c == '*':
			j := ctaScanWildcardTail(s, i+1)
			if j > i+1 {
				emit(syntaxWildcard, i)
				i = j
				continue
			}
			emit(syntaxStar, i)
			i++
		default:
			kind, ok := syntaxPunctuation(c)
			if ok {
				emit(kind, i)
				i++
				continue
			}
			// A ':' the NameTest scanner leaves behind — `p:` with no NCName or
			// '*' after it — reaches this arm next and fails here, at the ':'.
			nameKind, j := ctaScanNameTest(s, i)
			if j == i {
				return nil, i, false
			}
			tok := syntaxToken{kind: syntaxName, text: s[i:j], at: i}
			if nameKind == ctaWildcardTok {
				tok.kind = syntaxWildcard
			}
			toks = append(toks, tok)
			i = j
		}
	}
	return append(toks, syntaxToken{kind: syntaxEOF, at: len(s)}), 0, true
}

// syntaxPunctuation reports the kind of the one-character delimiting terminal
// c is, if it is one the switch in syntaxTokenize does not read itself.
func syntaxPunctuation(c byte) (syntaxKind, bool) {
	switch c {
	case '$':
		return syntaxDollar, true
	case '(':
		return syntaxLParen, true
	case ')':
		return syntaxRParen, true
	case '[':
		return syntaxLBracket, true
	case ']':
		return syntaxRBracket, true
	case ',':
		return syntaxComma, true
	case '.':
		return syntaxDot, true
	case '/':
		return syntaxSlash, true
	case '@':
		return syntaxAt, true
	case '?':
		return syntaxQuestion, true
	case '+':
		return syntaxPlus, true
	case '-':
		return syntaxMinus, true
	case '|':
		return syntaxBar, true
	}
	return 0, false
}

// syntaxComparatorLen reports the length of the [22] GeneralComp or [24]
// NodeComp symbol at the start of s, longest first, or 0 where none is.
func syntaxComparatorLen(s string) int {
	for _, op := range []string{"!=", "<=", ">=", "<<", ">>", "=", "<", ">"} {
		if strings.HasPrefix(s, op) {
			return len(op)
		}
	}
	return 0
}

// syntaxRecognizer is the recursive descent over A.1, one method per
// production it needs a decision point for. Every method reports whether its
// production matched, consuming as it goes; on false the token at pos is the
// one no derivation takes, because no decision backtracks — A.1 is decidable
// here with one or two tokens of lookahead, and the keywords that open
// ExprSingle are told from names by the token after them.
//
// It carries no depth guard (STYLE D4): every loop consumes a token per turn
// and every recursion consumes one before it recurses.
type syntaxRecognizer struct {
	toks []syntaxToken
	pos  int
	// static is the static context the names are resolved against, or nil for
	// the recognizer syntaxError runs, which resolves nothing.
	static *syntaxStatic
}

// syntaxStatic is the static context of a Type Alternative {test}, xpath-valid
// clause 2.2's, as the full-grammar static pass reads it, and what that pass
// found.
type syntaxStatic struct {
	// names are the statically known namespaces (2.2.2) and the default
	// element/type namespace (2.2.3), read exactly as the subset parser reads
	// them.
	names ctaNames
	// types are the in-scope schema definitions (2.2.5).
	types ctaTypes
	// vars are the range variables in scope at the recognizer's position,
	// innermost last. A ForExpr or QuantifiedExpr binds every one of them: the
	// in-scope variables of the static context are the empty set (2.2.6).
	vars []xsd.QName
	// first is the first static error in expression order, or nil.
	first *xsderr.Error
}

// ctaFullStaticError is the first static error, in expression order, that a
// Type Alternative {test} inside the full XPath 2.0 grammar carries, or nil:
// the static pass of [CTATestStaticError] for a text the subset parser
// declined with no static error proved, wherever in the text the error sits
// and whether or not ctaTokenize reads the text at all. It answers nil for a
// text outside the grammar, whose err:XPST0003 is syntaxError's, and for
// types that hold no builtins (ctaResolveTypes), against which no name
// resolves.
//
// The static errors it proves are:
//
//   - err:XPST0081 for a QName — a NameTest, a wildcard's NCName prefix, a
//     function name, a VarName, a SingleType or AtomicType, and a name or a
//     TypeName inside a KindTest — whose prefix the {namespace bindings} do not
//     bind (xpath20.md Appendix G);
//   - err:XPST0008 for a VarRef naming no variable a ForExpr or QuantifiedExpr
//     around it bound (xpath20.md §3.1.2: "It is a static error [err:XPST0008]
//     to reference a variable that is not in scope"), `$value` included, since
//     xpath-valid clause 2.2.6 leaves the in-scope variables empty;
//   - err:XPST0051 or err:XPST0080 for the SingleType of a `cast as` or a
//     `castable as` (ctaTypes.castTarget, xpath20.md §3.10.2 and §3.10.3);
//   - err:XPST0051 for the AtomicType of an `instance of` or `treat as`
//     SequenceType (ctaTypes.sequenceAtomicType, xpath20.md §2.5.4.2);
//   - err:XPST0008 for the TypeName of an ElementTest or an AttributeTest
//     outside the in-scope schema types (kindTypeName, ctaTypes.inScopeType),
//     for every schema-element name (schemaElementName), and for a
//     schema-attribute name other than the four §3.2.7 xsi declarations
//     (schemaAttributeName), xpath20.md §2.5.4.3–§2.5.4.6. An ElementName or
//     AttributeName in an ElementTest or AttributeTest is exempt (testName).
//
// GAP(xpath): a function name or arity no signature has, err:XPST0017, is not
// charged; [CTATestStaticError] answers nil for a text carrying nothing else.
// xpath-valid clause 2.2.8 leaves the function signatures
// ·implementation-defined·, and #2690 decides whether that charges or
// permanently withholds it. The oracle's #894 ruling, on clause 2.2.5's
// in-scope schema types, states that it does not extend to 2.2.8.
func ctaFullStaticError(expr xsd.XPathExpression, types xsd.TypeResolver) *xsderr.Error {
	toks, _, ok := syntaxTokenize(expr.Expression())
	if !ok {
		return nil
	}
	known, ok := ctaResolveTypes(types)
	if !ok {
		return nil
	}
	static := &syntaxStatic{names: ctaNamesOf(expr), types: known}
	r := syntaxRecognizer{toks: toks, static: static}
	if !r.expr() || r.peek().kind != syntaxEOF {
		return nil
	}
	return static.first
}

// record keeps d where it is the first static error the static pass found;
// it keeps nothing for the recognizer syntaxError runs, nor for a defect of
// another kind.
func (r *syntaxRecognizer) record(d ctaDefect) {
	if r.static == nil || r.static.first != nil || d.kind != ctaStaticError {
		return
	}
	r.static.first = d.static
}

// qname resolves the QName or `NCName ':' '*'` wildcard t spells, an
// unprefixed one into the namespace unprefixed, recording err:XPST0081 for an
// unbound prefix. It reports false for an unbound prefix and for the
// recognizer syntaxError runs, which resolves nothing; t's spelling is empty
// for `*` and `'*' ':' NCName`, which name no prefix and resolve trivially.
func (r *syntaxRecognizer) qname(t syntaxToken, unprefixed string) (xsd.QName, bool) {
	if r.static == nil {
		return xsd.QName{}, false
	}
	prefix, local, prefixed := strings.Cut(t.text, ":")
	if !prefixed {
		return xsd.QName{Space: unprefixed, Local: t.text}, true
	}
	space, bound := r.static.names.prefixes[prefix]
	if !bound {
		r.record(ctaUnboundPrefix(prefix))
		return xsd.QName{}, false
	}
	return xsd.QName{Space: space, Local: local}, true
}

// typeName resolves the QName t spells on qname's terms as a type name, an
// unprefixed one into the default element/type namespace (xpath-valid clause
// 2.2.3).
func (r *syntaxRecognizer) typeName(t syntaxToken) (xsd.QName, bool) {
	if r.static == nil {
		return xsd.QName{}, false
	}
	return r.qname(t, r.static.names.defaultNamespace)
}

// scope is the number of range variables in scope, which restoreScope takes
// back to once the ForExpr or QuantifiedExpr that bound more ends.
func (r *syntaxRecognizer) scope() int {
	if r.static == nil {
		return 0
	}
	return len(r.static.vars)
}

func (r *syntaxRecognizer) restoreScope(n int) {
	if r.static == nil {
		return
	}
	r.static.vars = r.static.vars[:n]
}

func (r *syntaxRecognizer) peek() syntaxToken { return r.toks[r.pos] }

// peekAfter is the token after the next one, or the EOF token.
func (r *syntaxRecognizer) peekAfter() syntaxToken {
	if r.pos+1 < len(r.toks) {
		return r.toks[r.pos+1]
	}
	return r.toks[len(r.toks)-1]
}

func (r *syntaxRecognizer) accept(kind syntaxKind) bool {
	if r.peek().kind != kind {
		return false
	}
	r.pos++
	return true
}

// isKeyword reports whether t is the unprefixed name word, which is how every
// keyword is spelled (A.2: keywords are case sensitive).
func isKeyword(t syntaxToken, word string) bool {
	return t.kind == syntaxName && t.text == word
}

func (r *syntaxRecognizer) acceptKeyword(word string) bool {
	if !isKeyword(r.peek(), word) {
		return false
	}
	r.pos++
	return true
}

// acceptKeywordPair consumes the two-word operators `instance of`, `treat
// as`, `castable as` and `cast as`, and nothing when only the first matches.
func (r *syntaxRecognizer) acceptKeywordPair(first, second string) bool {
	if !isKeyword(r.peek(), first) || !isKeyword(r.peekAfter(), second) {
		return false
	}
	r.pos += 2
	return true
}

// expr is [2] Expr.
func (r *syntaxRecognizer) expr() bool {
	if !r.exprSingle() {
		return false
	}
	for r.accept(syntaxComma) {
		if !r.exprSingle() {
			return false
		}
	}
	return true
}

// exprSingle is [3] ExprSingle. `for`, `some` and `every` open [4] ForExpr
// and [6] QuantifiedExpr only before '$', and `if` opens [7] IfExpr only
// before '(' — which reserved-function-names makes unambiguous; elsewhere each
// is a NameTest.
func (r *syntaxRecognizer) exprSingle() bool {
	t, next := r.peek(), r.peekAfter()
	switch {
	case (isKeyword(t, "for") || isKeyword(t, "some") || isKeyword(t, "every")) && next.kind == syntaxDollar:
		r.pos++
		body := "satisfies"
		if isKeyword(t, "for") {
			body = "return"
		}
		outer := r.scope()
		ok := r.bindings() && r.acceptKeyword(body) && r.exprSingle()
		r.restoreScope(outer)
		return ok
	case isKeyword(t, "if") && next.kind == syntaxLParen:
		r.pos += 2
		return r.expr() && r.accept(syntaxRParen) &&
			r.acceptKeyword("then") && r.exprSingle() &&
			r.acceptKeyword("else") && r.exprSingle()
	}
	return r.orExpr()
}

// bindings is the `"$" VarName "in" ExprSingle ("," ...)*` [5]
// SimpleForClause and [6] QuantifiedExpr share. Each variable comes into
// scope after its own `in` ExprSingle, for the bindings after it and the
// `return` or `satisfies` body (xpath20.md §3.7, §3.9); exprSingle takes them
// out of scope again.
func (r *syntaxRecognizer) bindings() bool {
	for {
		if !r.accept(syntaxDollar) || r.peek().kind != syntaxName {
			return false
		}
		name, resolved := r.qname(r.peek(), "")
		r.pos++
		if !r.acceptKeyword("in") || !r.exprSingle() {
			return false
		}
		if resolved {
			r.static.vars = append(r.static.vars, name)
		}
		if !r.accept(syntaxComma) {
			return true
		}
	}
}

// varRef is [44] VarRef as a PrimaryExpr, a reference to a variable: one no
// enclosing binding put in scope is err:XPST0008. An unprefixed VarName is in
// no namespace (xpath20.md §3.1.2).
func (r *syntaxRecognizer) varRef() bool {
	if !r.accept(syntaxDollar) || r.peek().kind != syntaxName {
		return false
	}
	t := r.peek()
	r.pos++
	name, resolved := r.qname(t, "")
	if resolved && !slices.Contains(r.static.vars, name) {
		r.record(ctaStaticDefect(ruleXPST0008, "the variable reference $%s names no in-scope variable, which err:XPST0008 forbids", t.text))
	}
	return true
}

// orExpr through intersectExcept are [8] OrExpr through [15]
// IntersectExceptExpr, each a left-associative chain over the next.
func (r *syntaxRecognizer) orExpr() bool {
	return r.chain(r.andExpr, func() bool { return r.acceptKeyword("or") })
}

func (r *syntaxRecognizer) andExpr() bool {
	return r.chain(r.comparisonExpr, func() bool { return r.acceptKeyword("and") })
}

// comparisonExpr is [10] ComparisonExpr, which takes ONE comparator at most:
// `1 = 2 = 3` is no derivation.
func (r *syntaxRecognizer) comparisonExpr() bool {
	if !r.rangeExpr() {
		return false
	}
	if !r.accept(syntaxComp) && !r.acceptComparatorWord() {
		return true
	}
	return r.rangeExpr()
}

// acceptComparatorWord consumes a [23] ValueComp or [24] NodeComp's 'is'.
func (r *syntaxRecognizer) acceptComparatorWord() bool {
	for _, w := range []string{"eq", "ne", "lt", "le", "gt", "ge", "is"} {
		if r.acceptKeyword(w) {
			return true
		}
	}
	return false
}

// rangeExpr is [11] RangeExpr, one `to` at most.
func (r *syntaxRecognizer) rangeExpr() bool {
	if !r.additiveExpr() {
		return false
	}
	if !r.acceptKeyword("to") {
		return true
	}
	return r.additiveExpr()
}

func (r *syntaxRecognizer) additiveExpr() bool {
	return r.chain(r.multiplicativeExpr, func() bool {
		return r.accept(syntaxPlus) || r.accept(syntaxMinus)
	})
}

func (r *syntaxRecognizer) multiplicativeExpr() bool {
	return r.chain(r.unionExpr, func() bool {
		return r.accept(syntaxStar) || r.acceptKeyword("div") || r.acceptKeyword("idiv") || r.acceptKeyword("mod")
	})
}

func (r *syntaxRecognizer) unionExpr() bool {
	return r.chain(r.intersectExceptExpr, func() bool {
		return r.accept(syntaxBar) || r.acceptKeyword("union")
	})
}

func (r *syntaxRecognizer) intersectExceptExpr() bool {
	return r.chain(r.instanceofExpr, func() bool {
		return r.acceptKeyword("intersect") || r.acceptKeyword("except")
	})
}

// chain is `operand (operator operand)*`; operator consumes the operator
// when it is there and reports whether it was.
func (r *syntaxRecognizer) chain(operand, operator func() bool) bool {
	if !operand() {
		return false
	}
	for operator() {
		if !operand() {
			return false
		}
	}
	return true
}

// instanceofExpr through castExpr are [16] InstanceofExpr through [19]
// CastExpr, each an operand with one optional typed tail.
func (r *syntaxRecognizer) instanceofExpr() bool {
	if !r.treatExpr() {
		return false
	}
	if !r.acceptKeywordPair("instance", "of") {
		return true
	}
	return r.sequenceType()
}

func (r *syntaxRecognizer) treatExpr() bool {
	if !r.castableExpr() {
		return false
	}
	if !r.acceptKeywordPair("treat", "as") {
		return true
	}
	return r.sequenceType()
}

func (r *syntaxRecognizer) castableExpr() bool {
	if !r.castExpr() {
		return false
	}
	if !r.acceptKeywordPair("castable", "as") {
		return true
	}
	return r.singleType()
}

func (r *syntaxRecognizer) castExpr() bool {
	if !r.unaryExpr() {
		return false
	}
	if !r.acceptKeywordPair("cast", "as") {
		return true
	}
	return r.singleType()
}

// unaryExpr is [20] UnaryExpr over [21] ValueExpr, which is [25] PathExpr.
func (r *syntaxRecognizer) unaryExpr() bool {
	for r.accept(syntaxPlus) || r.accept(syntaxMinus) {
	}
	return r.pathExpr()
}

// pathExpr is [25] PathExpr under xgc: leading-lone-slash: a '/' is the whole
// path only where the next token cannot open a RelativePathExpr, so `/ * 5`
// reads `/*` and then fails at `5`.
func (r *syntaxRecognizer) pathExpr() bool {
	if r.accept(syntaxSlash) {
		if !opensRelativePath(r.peek()) {
			return true
		}
		return r.relativePathExpr()
	}
	r.accept(syntaxSlashSlash)
	return r.relativePathExpr()
}

// opensRelativePath reports whether t can be the first terminal of a [26]
// RelativePathExpr: of a [28] AxisStep or of a [38] FilterExpr's [41]
// PrimaryExpr.
func opensRelativePath(t syntaxToken) bool {
	switch t.kind {
	case syntaxName, syntaxWildcard, syntaxStar, syntaxAt, syntaxDotDot, syntaxDot,
		syntaxDollar, syntaxLParen, syntaxNumber, syntaxString:
		return true
	default:
		return false
	}
}

// relativePathExpr is [26] RelativePathExpr.
func (r *syntaxRecognizer) relativePathExpr() bool {
	return r.chain(r.stepExpr, func() bool {
		return r.accept(syntaxSlash) || r.accept(syntaxSlashSlash)
	})
}

// stepExpr is [27] StepExpr — [28] AxisStep or [38] FilterExpr — with the
// [39] PredicateList both end in. A name before '::' is an axis; an
// unprefixed name before '(' is a KindTest when it names one, never a
// FunctionCall when A.3 reserves it, and a FunctionCall otherwise.
func (r *syntaxRecognizer) stepExpr() bool {
	t, next := r.peek(), r.peekAfter()
	var ok bool
	switch {
	case t.kind == syntaxDotDot:
		r.pos++
		ok = true
	case t.kind == syntaxAt:
		r.pos++
		ok = r.nodeTest()
	case t.kind == syntaxName && next.kind == syntaxAxis:
		ok = isAxis(t) && r.advance(2) && r.nodeTest()
	case t.kind == syntaxName && next.kind == syntaxLParen && isKindTestName(t):
		ok = r.kindTest()
	case t.kind == syntaxName && next.kind == syntaxLParen:
		ok = !isReservedFunctionName(t) && r.functionCall()
	case t.kind == syntaxName || t.kind == syntaxWildcard || t.kind == syntaxStar:
		ok = r.nodeTest()
	default:
		ok = r.primaryExpr()
	}
	return ok && r.predicateList()
}

// advance consumes n tokens and reports true, for use inside a conjunction.
func (r *syntaxRecognizer) advance(n int) bool {
	r.pos += n
	return true
}

// isAxis reports whether t names a [30] ForwardAxis or [33] ReverseAxis.
func isAxis(t syntaxToken) bool {
	switch t.text {
	case "child", "descendant", "attribute", "self", "descendant-or-self",
		"following-sibling", "following", "namespace",
		"parent", "ancestor", "preceding-sibling", "preceding", "ancestor-or-self":
		return true
	}
	return false
}

// isKindTestName reports whether t is the keyword opening one of [54]
// KindTest's nine arms.
func isKindTestName(t syntaxToken) bool {
	switch t.text {
	case "document-node", "element", "attribute", "schema-element", "schema-attribute",
		"processing-instruction", "comment", "text", "node":
		return true
	}
	return false
}

// isReservedFunctionName reports whether t is one of A.3's names no unprefixed
// FunctionCall may use (xgc: reserved-function-names). It is the whole list;
// stepExpr reads the KindTest names among it as a KindTest first.
func isReservedFunctionName(t syntaxToken) bool {
	switch t.text {
	case "empty-sequence", "if", "item", "typeswitch":
		return true
	}
	return isKindTestName(t)
}

// nodeTest is [35] NodeTest. A NameTest's namespace is no static error's
// concern beyond its prefix, so qname resolves it with no default namespace.
func (r *syntaxRecognizer) nodeTest() bool {
	t := r.peek()
	if t.kind == syntaxName && r.peekAfter().kind == syntaxLParen && isKindTestName(t) {
		return r.kindTest()
	}
	if t.kind != syntaxName && t.kind != syntaxWildcard && t.kind != syntaxStar {
		return false
	}
	r.qname(t, "")
	r.pos++
	return true
}

// predicateList is [39] PredicateList.
func (r *syntaxRecognizer) predicateList() bool {
	for r.accept(syntaxLBracket) {
		if !r.expr() || !r.accept(syntaxRBracket) {
			return false
		}
	}
	return true
}

// primaryExpr is [41] PrimaryExpr less [48] FunctionCall, which stepExpr
// reads.
func (r *syntaxRecognizer) primaryExpr() bool {
	switch r.peek().kind {
	case syntaxNumber, syntaxString, syntaxDot:
		r.pos++
		return true
	case syntaxDollar:
		return r.varRef()
	case syntaxLParen:
		r.pos++
		if r.accept(syntaxRParen) {
			return true
		}
		return r.expr() && r.accept(syntaxRParen)
	default:
		return false
	}
}

// functionCall is [48] FunctionCall. Its name is resolved for its prefix
// alone: whether a function of that name and arity exists is err:XPST0017,
// withheld (ctaFullStaticError).
func (r *syntaxRecognizer) functionCall() bool {
	r.qname(r.peek(), ctaFunctionNS)
	r.pos += 2 // the QName and '('
	if r.accept(syntaxRParen) {
		return true
	}
	if !r.exprSingle() {
		return false
	}
	for r.accept(syntaxComma) {
		if !r.exprSingle() {
			return false
		}
	}
	return r.accept(syntaxRParen)
}

// singleType is [49] SingleType: one QName, then '?' or nothing. The QName is
// the target of a `cast as` or `castable as`, in the default element/type
// namespace when unprefixed (xpath20.md §3.10.2), and castTarget decides its
// static error.
func (r *syntaxRecognizer) singleType() bool {
	t := r.peek()
	if !r.accept(syntaxName) {
		return false
	}
	if name, resolved := r.typeName(t); resolved {
		_, defect := r.static.types.castTarget(name)
		r.record(defect)
	}
	r.accept(syntaxQuestion)
	return true
}

// sequenceType is [50] SequenceType. Under xgc: occurrence-indicators a '?',
// '*' or '+' right after the ItemType is always its [51] OccurrenceIndicator,
// so `4 treat as item() + 5` fails at `5`.
func (r *syntaxRecognizer) sequenceType() bool {
	if isKeyword(r.peek(), "empty-sequence") && r.peekAfter().kind == syntaxLParen {
		r.pos += 2
		return r.accept(syntaxRParen)
	}
	if !r.itemType() {
		return false
	}
	_ = r.accept(syntaxQuestion) || r.accept(syntaxStar) || r.accept(syntaxPlus)
	return true
}

// itemType is [52] ItemType. A bare QName is an [53] AtomicType, in the
// default element/type namespace when unprefixed, and sequenceAtomicType
// decides its static error.
func (r *syntaxRecognizer) itemType() bool {
	t := r.peek()
	if t.kind == syntaxName && r.peekAfter().kind == syntaxLParen && isKindTestName(t) {
		return r.kindTest()
	}
	if isKeyword(t, "item") && r.peekAfter().kind == syntaxLParen {
		r.pos += 2
		return r.accept(syntaxRParen)
	}
	if !r.accept(syntaxName) {
		return false
	}
	if name, resolved := r.typeName(t); resolved {
		r.record(r.static.types.sequenceAtomicType(name))
	}
	return true
}

// acceptName consumes a QName a KindTest carries and hands it to resolve,
// which decides its static error for the role it plays, reporting whether one
// was there.
func (r *syntaxRecognizer) acceptName(resolve func(syntaxToken)) bool {
	t := r.peek()
	if !r.accept(syntaxName) {
		return false
	}
	resolve(t)
	return true
}

// testName resolves the ElementName of an ElementTest or the AttributeName of
// an AttributeTest for its prefix alone: neither need be declared, which the
// err:XPST0008 entry of xpath20.md Appendix G exempts by name.
func (r *syntaxRecognizer) testName(t syntaxToken) {
	r.qname(t, "")
}

// kindTypeName resolves the TypeName of an ElementTest or an AttributeTest in
// the default element/type namespace (typeName) and records err:XPST0008 where
// it is no type in the in-scope schema types (ctaTypes.inScopeType): "TypeName
// must be present in the in-scope schema types [err:XPST0008]" (xpath20.md
// §2.5.4.3, §2.5.4.5).
func (r *syntaxRecognizer) kindTypeName(t syntaxToken) {
	name, resolved := r.typeName(t)
	if !resolved || r.static.types.inScopeType(name) {
		return
	}
	r.record(ctaStaticDefect(ruleXPST0008, "the KindTest TypeName %q is no type in the in-scope schema types, which err:XPST0008 forbids", name))
}

// schemaElementName resolves the name of a SchemaElementTest in the default
// element/type namespace, an element name's position (xpath20.md §2.5.4), and
// records err:XPST0008 for every name it resolves: "If the ElementName
// specified in the SchemaElementTest is not found in the in-scope element
// declarations, a static error is raised [err:XPST0008]" (§2.5.4.4), and
// xpath-valid clause 2.2.5 puts no element declaration in scope — §3.2.7,
// §3.4.7 and §3.16.7 hold none.
func (r *syntaxRecognizer) schemaElementName(t syntaxToken) {
	name, resolved := r.typeName(t)
	if !resolved {
		return
	}
	r.record(ctaStaticDefect(ruleXPST0008, "the schema-element name %q is no in-scope element declaration, which err:XPST0008 forbids", name))
}

// schemaAttributeName resolves the name of a SchemaAttributeTest, an
// unprefixed one into no namespace (xpath20.md §2.5.4: "An unprefixed
// attribute QName is in no namespace"), and records err:XPST0008 where it is
// none of the in-scope attribute declarations (ctaInScopeAttribute, §2.5.4.6).
func (r *syntaxRecognizer) schemaAttributeName(t syntaxToken) {
	name, resolved := r.qname(t, "")
	if !resolved || ctaInScopeAttribute(name) {
		return
	}
	r.record(ctaStaticDefect(ruleXPST0008, "the schema-attribute name %q is no in-scope attribute declaration, which err:XPST0008 forbids", name))
}

// ctaInScopeAttribute reports whether name is one of the in-scope attribute
// declarations xpath-valid clause 2.2.5 fixes: the four Built-in Attribute
// Declarations of Structures §3.2.7, xsi:type, xsi:nil, xsi:schemaLocation and
// xsi:noNamespaceSchemaLocation. The parser's seedInstanceAttributes builds
// the same four as components and validate's isInstanceAttribute excepts them
// by name. Both packages import this one, so neither encoding is reachable
// from here, and the set is fixed by the spec (STYLE T4's accepted third
// encoding).
func ctaInScopeAttribute(name xsd.QName) bool {
	if name.Space != xsd.XMLSchemaInstanceNS {
		return false
	}
	switch name.Local {
	case "type", "nil", "schemaLocation", "noNamespaceSchemaLocation":
		return true
	}
	return false
}

// kindTest is [54] KindTest, entered on its keyword with '(' next.
func (r *syntaxRecognizer) kindTest() bool {
	word := r.peek().text
	r.pos += 2 // the keyword and '('
	switch word {
	case "document-node":
		// [56]: an ElementTest or a SchemaElementTest, or nothing.
		t := r.peek()
		if (isKeyword(t, "element") || isKeyword(t, "schema-element")) && r.peekAfter().kind == syntaxLParen {
			if !r.kindTest() {
				return false
			}
		}
	case "element":
		// [64]: (ElementNameOrWildcard ("," TypeName "?"?)?)?
		if r.acceptName(r.testName) || r.accept(syntaxStar) {
			if r.accept(syntaxComma) {
				if !r.acceptName(r.kindTypeName) {
					return false
				}
				r.accept(syntaxQuestion)
			}
		}
	case "attribute":
		// [60]: (AttribNameOrWildcard ("," TypeName)?)?
		if r.acceptName(r.testName) || r.accept(syntaxStar) {
			if r.accept(syntaxComma) && !r.acceptName(r.kindTypeName) {
				return false
			}
		}
	case "schema-element":
		// [66]: exactly one QName.
		if !r.acceptName(r.schemaElementName) {
			return false
		}
	case "schema-attribute":
		// [62]: exactly one QName.
		if !r.acceptName(r.schemaAttributeName) {
			return false
		}
	case "processing-instruction":
		// [59]: an NCName or a StringLiteral, or nothing.
		if !strings.Contains(r.peek().text, ":") {
			_ = r.accept(syntaxName) || r.accept(syntaxString)
		}
	}
	// [55] node(), [57] text() and [58] comment() take nothing.
	return r.accept(syntaxRParen)
}
