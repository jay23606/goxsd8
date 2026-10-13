package xpath

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// ctaExpr builds an XPath Expression property record over expr, binding the
// prefix/namespace pairs as its {namespace bindings}. defaultNS, when
// non-empty, is its {default namespace} — the ABSENT property otherwise, which
// is what every test passing "" here gets. It answers for one production only,
// [15] ta-CastExpr's unprefixed target QName, which is why
// TestCompileResolvesCastTargetInDefaultNamespace is the one test in this file
// that passes it a value.
func ctaExprRecord(expr, defaultNS string, bindings ...string) xsd.XPathExpression {
	var nb []xsd.NamespaceBinding
	for i := 0; i+1 < len(bindings); i += 2 {
		nb = append(nb, xsd.NewNamespaceBinding(bindings[i], bindings[i+1]))
	}
	var def *string
	if defaultNS != "" {
		def = &defaultNS
	}
	return xsd.NewXPathExpression(expr, nb, def, nil)
}

// uq is an ·expanded name· in no namespace, which is what every unprefixed
// attribute NameTest resolves to.
func uq(local string) xsd.QName { return xsd.QName{Local: local} }

// ctaAttribute is one attribute of the element a {test} is evaluated against.
type ctaAttribute struct {
	name    xsd.QName
	lexical string
}

// ctaAttrs is the [Attributes] over a fixed attribute list, yielding it in the
// order WRITTEN, which stands in for the source order a real element's
// attributes arrive in. A map would yield them in a different order per run
// (STYLE D2), which a wildcard NameTest matching several of them makes
// observable in the answer.
func ctaAttrs(attrs ...ctaAttribute) Attributes {
	return func(yield func(xsd.QName, string) bool) {
		for _, a := range attrs {
			if !yield(a.name, a.lexical) {
				return
			}
		}
	}
}

// at is one attribute in no namespace, which is what an unprefixed NameTest
// matches.
func at(local, lexical string) ctaAttribute {
	return ctaAttribute{name: uq(local), lexical: lexical}
}

// atNS is one attribute in the namespace space.
func atNS(space, local, lexical string) ctaAttribute {
	return ctaAttribute{name: xsd.QName{Space: space, Local: local}, lexical: lexical}
}

// seededTypes and seededBackend are the type knowledge and the value spaces
// every test here compiles and evaluates against: the builtin datatypes
// builtin.Seed produces, indexed by ·expanded name· exactly as a parsed
// schema's {type definitions} index them. They are built ONCE and together —
// Seed compiles each type's facets against the backend it is handed, so a
// compile and the evaluation that follows it must see the same instance.
//
// A failed Seed is a broken fixture rather than a failed assertion, and it
// would break every test in the file identically, so it panics here instead of
// reporting through a *testing.T none of these helpers holds.
var seededTypes, seededBackend = seedBuiltins()

// ctaTestTypes is the minimal [xsd.TypeResolver] the seeded builtins fill.
type ctaTestTypes map[xsd.QName]xsd.TypeDefinition

func (t ctaTestTypes) Type(name xsd.QName) (xsd.TypeDefinition, bool) {
	td, declared := t[name]
	return td, declared
}

func seedBuiltins() (ctaTestTypes, value.Backend) {
	b := strict.New()
	types, err := builtin.Seed(b)
	if err != nil {
		panic("builtin.Seed: " + err.Error())
	}
	resolver := make(ctaTestTypes, len(types))
	for _, st := range types {
		resolver[st.Name()] = st
	}
	return resolver, b
}

// backend is the value spaces the comparisons and the casts run in.
func backend() value.Backend { return seededBackend }

// compile compiles expr or fails the test, so an evaluation case never
// silently degrades into a decline case.
func compile(t *testing.T, expr string, bindings ...string) CTATest {
	t.Helper()
	c, ok := CompileCTATest(ctaExprRecord(expr, "", bindings...), seededTypes)
	if !ok {
		t.Fatalf("CompileCTATest(%q): declined, want compiled", expr)
	}
	return c
}

// TestCompileAdmitsRequiredSubset pins that every production [8]-[18] this
// engine evaluates parses: the two AttrName spellings ta-props-correct clause
// 2 admits, both Literal kinds, all six Comparators, the three BooleanExpr
// arms, and the or/and connectives with their nesting.
func TestCompileAdmitsRequiredSubset(t *testing.T) {
	for _, expr := range []string{
		"@kind",
		"@kind = 'book'",
		"@kind='book'",
		"attribute::kind = 'book'",
		"@kind != 'book'",
		"@n < 5",
		"@n <= 5",
		"@n > 5",
		"@n >= 5",
		"@n = 1.5",
		"@n = 1.5e2",
		"@n = .5",
		"'a' = 'a'",
		"1 = 1",
		"not(@kind = 'book')",
		"fn:not(@kind = 'book')",
		"(@kind = 'book')",
		"@a = 'x' or @b = 'y'",
		"@a = 'x' and @b = 'y'",
		"@a = 'x' and (@b = 'y' or @c = 'z')",
		"@a  =  'x'   or   @b = 'y'",
		"(: a comment :) @kind = 'book'",
		"@kind = 'it''s'",
		"@kind = \"it's\"",
		"@a.b = 'x'",
	} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, "", "fn", ctaFunctionNS), seededTypes); !ok {
			t.Errorf("CompileCTATest(%q): declined, want compiled", expr)
		}
	}
}

// TestCompileResolvesUnprefixedFunctionInDefaultFunctionNamespace pins
// xpath-valid clause 2.2.4: a bare not(...) IS fn:not, with no binding needed,
// while a prefix bound elsewhere is a different function and declines.
func TestCompileResolvesUnprefixedFunctionInDefaultFunctionNamespace(t *testing.T) {
	if _, ok := CompileCTATest(ctaExprRecord("not(@k = 'x')", ""), seededTypes); !ok {
		t.Error("an unprefixed not(...) declined; it is fn:not by clause 2.2.4")
	}
	elsewhere := ctaExprRecord("p:not(@k = 'x')", "", "p", "http://example.com/p")
	if _, ok := CompileCTATest(elsewhere, seededTypes); ok {
		t.Error("p:not bound outside the F&O namespace compiled as fn:not")
	}
}

// TestCompileDeclines pins the withhold direction over its whole membership:
// text that is no Test production at all (which is what legal full XPath 2.0
// looks like here), an unbound prefix, and a wildcard NameTest. The cast
// TARGETS that decline have their own test below, because they decline for a
// reason the grammar cannot see.
func TestCompileDeclines(t *testing.T) {
	for _, tc := range []struct{ expr, why string }{
		{"", "an empty {expression} is no Test production"},
		{"1 idiv string-length(@type) gt 0", "full XPath 2.0, outside the subset"},
		{"self::message", "an axis step the grammar has no production for"},
		{"a:b('123')", "a constructor call naming a type the {type definitions} do not declare"},
		{"(@type cast as xs:float)='float'", "[11]'s parenthesised arm is a BooleanExpr, which no Comparator may follow"},
		{"double('3' cast as float > 2)", "a constructor argument is a SimpleValue, never a comparison"},
		{"3 cast as 3", "a cast target is a QName, not a Literal"},
		{"cast as decimal 3", "'cast' where a SimpleValue must open [15]"},
		{"3 cast 'as' decimal", "the 'as' keyword is a name token, not a string literal"},
		{"6 > cast as decimal", "a Comparator's right operand is a ValueExpr"},
		{"3 cast as @a:kind > 1", "a cast target is a QName, not an AttrName"},
		{"@p:kind = 'x'", "a prefix with no binding is err:XPST0081"},
		{"p:not(@kind = 'x')", "an unbound prefix on the function name too"},
		// The three WILDCARD NameTests were pinned here until they compiled
		// (#859). They evaluate now — TestEvaluateWildcardNameTest — and only
		// the one spelling that resolves a prefix can still withhold, which is
		// the err:XPST0081 row below and not a decline for the wildcard's shape.
		{"@p:* = 'x'", "a prefixed wildcard NameTest whose prefix has no binding is err:XPST0081"},
		{"* = 'x'", "a wildcard reaches this grammar only through [17] ta-AttrName"},
		{"1 * 2 = 2", "and a Type Alternative's façade admits no arithmetic operator"},
		{"@kind = 'x' extra", "trailing tokens are not part of a Test"},
		{"@kind = ", "a Comparator with no right operand"},
		{"(@kind = 'x'", "an unclosed parenthesis"},
		{"@kind = 'x", "an unclosed string literal"},
		{"(: unclosed", "an unclosed comment"},
		{"@n = -1", "no unary minus production reaches a Literal"},
		{"@n - 1 = 2", "a Type Alternative's façade admits no arithmetic operator"},
		{"@a = 'x' or", "an 'or' with no right operand"},
		// B.1 rule 1.1's xs:float to xs:double promotion, which this engine
		// withholds rather than answering through a canonical round-trip that
		// lands on a different xs:double (ctaWider, #889). Both sites: the
		// typed pair, and clause 2.1's untypedAtomic-against-numeric arm.
		{"@f cast as xs:float = 1e-1", "an xs:float operand against an xs:double one needs B.1 rule 1.1"},
		{"@f cast as xs:float > 1e-1", "the same pair under an ordering comparator"},
		{"1e-1 = @f cast as xs:float", "the same pair with the operands the other way round"},
		{"@a = @f cast as xs:float", "clause 2.1 answers xs:double, so the xs:float operand needs rule 1.1"},
		{"@a < @f cast as xs:float", "the same clause 2.1 pair under an ordering comparator"},
	} {
		bindings := []string{"a", "http://example.com/a", "xs", xsd.XMLSchemaNS}
		if _, ok := CompileCTATest(ctaExprRecord(tc.expr, "", bindings...), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
}

// TestCTATestStaticErrorReportsUnboundPrefix pins the static direction of
// ta-props-correct clause 2: an unbound prefix inside a COMPLETE ta-Test is
// err:XPST0081, reported as a fact about the schema — and CompileCTATest goes on
// withholding the same expression, because the tree it built holds a name that
// resolved to nothing.
//
// The code travels as the verdict's own [xsderr.Rule] and the message holds the
// fact alone, which is what lets the assembler wrapping this read err:XPST0081
// off the cause rather than scraping the rendering for it (#978).
func TestCTATestStaticErrorReportsUnboundPrefix(t *testing.T) {
	for _, tc := range []struct{ expr, prefix string }{
		{"@p:kind = 'x'", "p"},
		{"attribute::p:kind = 'x'", "p"},
		{"@p:kind", "p"},
		{"not(@p:kind = 'x')", "p"},
		{"@a:kind = 'x' or @p:kind = 'y'", "p"},
		{"@q:kind = @p:kind", "q"},
		{"(@p:kind = 'x') and @a:kind = 'y'", "p"},
		// A cast tail no longer declines the expression that carries it (#858),
		// so the ta-Test around an unbound prefix COMPLETES here where it once
		// died at the tail, and the same defect is reported rather than dropped.
		{"@p:kind cast as xs:string", "p"},
		{"@p:kind cast as xs:string = 'x'", "p"},
		{"xs:string(@p:kind) = 'x'", "p"},
		// [37]'s NCName ':' '*' is the one wildcard spelling that resolves a
		// prefix, so it is the one that can carry this static error (#859). The
		// other two name no prefix and appear in no row here.
		{"@p:* = 'x'", "p"},
		{"attribute::p:* = 'x'", "p"},
		{"@p:*", "p"},
		{"not(@p:* = 'x')", "p"},
		{"@p:* cast as xs:string = 'x'", "p"},
	} {
		record := ctaExprRecord(tc.expr, "", "a", "http://example.com/a", "xs", xsd.XMLSchemaNS)
		err := CTATestStaticError(record, seededTypes)
		if err == nil {
			t.Errorf("CTATestStaticError(%q) = nil, want err:XPST0081", tc.expr)
			continue
		}
		var verdict *xsderr.Error
		if !errors.As(err, &verdict) {
			t.Errorf("CTATestStaticError(%q) = %v, which is not an *xsderr.Error: the assembler wraps this verdict and reads its rule off it", tc.expr, err)
			continue
		}
		if verdict.Rule != ruleXPST0081 {
			t.Errorf("CTATestStaticError(%q) reports rule %q, want %q", tc.expr, verdict.Rule, ruleXPST0081)
		}
		if want := `no statically-known namespace binding for prefix "` + tc.prefix + `"`; verdict.Msg != want {
			t.Errorf("CTATestStaticError(%q).Msg = %q, want %q", tc.expr, verdict.Msg, want)
		}
		if verdict.Loc != (xsderr.Loc{}) {
			t.Errorf("CTATestStaticError(%q) reports location %v, want the zero Loc: this package reads no schema document", tc.expr, verdict.Loc)
		}
		if _, ok := CompileCTATest(record, seededTypes); ok {
			t.Errorf("CompileCTATest(%q) compiled; a tree holding an unresolved name must never reach Evaluate", tc.expr)
		}
	}
}

// TestCompileAdmitsCasts pins the three cast-shaped constructs of the required
// subset, which are the whole of what this issue's landing added to the
// admitted set: [15] ta-CastExpr's `cast as QName` tail, its `?` occurrence
// indicator, and [18] ta-ConstructorFunction — the last being §3.12.6 clause
// 3's reading of any QName '(' SimpleValue ')' whose name is not fn:not.
func TestCompileAdmitsCasts(t *testing.T) {
	for _, expr := range []string{
		"@kind cast as xs:string",
		"@kind cast as xs:string = 'x'",
		"@n cast as xs:integer ?",
		"@n cast as xs:integer? = 3",
		"xs:int(@length) > xs:int(@width)",
		"@length cast as xs:int > @width cast as xs:int",
		"xs:int(@length) = @width cast as xs:int",
		"xs:date('2026-01-01') = @d cast as xs:date",
		"not(@n cast as xs:integer > 3)",
	} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, "", "xs", xsd.XMLSchemaNS), seededTypes); !ok {
			t.Errorf("CompileCTATest(%q): declined, want compiled", expr)
		}
	}
}

// TestCTATestStaticErrorIsSilentForUnsupported pins UNSUPPORTED DOMINATES
// STATIC. Every expression here is legal XPath 2.0 syntax (xpath20.md A.1),
// and none of them is a schema fault the engine proves: the subset ones
// compile, and charging one outside the subset would reject a schema
// §3.12.6 clause 2's Note says a processor may decline but must not refuse.
func TestCTATestStaticErrorIsSilentForUnsupported(t *testing.T) {
	for _, tc := range []struct{ expr, why string }{
		{"@p:a = 'x' and count(@b) > 1", "a complete-looking prefix inside an expression a later constructor call declines"},
		{"p:not(@kind = 'x')", "a constructor call whose argument is no SimpleValue — a prefixed name is never fn:not"},
		{"count(//p:a) > 1", "full XPath 2.0, outside the subset"},
		{"self::message", "an axis step the grammar has no production for"},
		{"@kind = 'book'", "a complete Test whose every name resolved"},
		{"@a:kind = 'book'", "a complete Test whose prefix is bound"},
		// The cast-target static conditions the engine does not prove: the
		// target declines, the parse dies at the tail, and the defect the
		// unbound prefix recorded on the way in dies with it.
		{"@p:kind cast as q:Missing", "an unbound prefix on the cast target itself"},
		{"@p:kind cast as xs:Missing", "err:XPST0051, folded into the withhold"},
		{"@p:kind cast as xs:anyAtomicType", "err:XPST0080, folded into the withhold"},
	} {
		bindings := []string{"a", "http://example.com/a", "xs", xsd.XMLSchemaNS}
		if err := CTATestStaticError(ctaExprRecord(tc.expr, "", bindings...), seededTypes); err != nil {
			t.Errorf("CTATestStaticError(%q) = %v, want nil (%s)", tc.expr, err, tc.why)
		}
	}
}

// TestCTATestStaticErrorReportsSyntaxError pins err:XPST0003: a {test} no
// derivation of xpath20.md A.1's grammar covers is a static error, so
// xpath-valid clause 2 fails and ta-props-correct clause 2 is charged over it.
// The first six rows are the suite's s3_12si04, s3_12si05 and s3_12si06
// expressions; the precedence row carries an unbound prefix the subset parse
// records before it dies, and reports err:XPST0003 over it.
//
// The assertion and facet façades share the traversal and must still DECLINE
// each one, never charge it: only CTATestStaticError asks the grammar.
func TestCTATestStaticErrorReportsSyntaxError(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, tc := range []struct{ expr, msg string }{
		{")(", `")(" is not an XPath 2.0 expression: the grammar takes nothing at byte offset 0, which err:XPST0003 forbids`},
		{">", `">" is not an XPath 2.0 expression: the grammar takes nothing at byte offset 0, which err:XPST0003 forbids`},
		{"((7>=6)", `"((7>=6)" is not an XPath 2.0 expression: the grammar needs more than its end, which err:XPST0003 forbids`},
		{"@6='hi'", `"@6='hi'" is not an XPath 2.0 expression: the grammar takes nothing at byte offset 1, which err:XPST0003 forbids`},
		{"@numberOfChildren < 5 AND @numberOfChildren > 0", `"@numberOfChildren < 5 AND @numberOfChildren > 0" is not an XPath 2.0 expression: the grammar takes nothing at byte offset 22, which err:XPST0003 forbids`},
		{"(@type cast as xs1::double)='double'", `"(@type cast as xs1::double)='double'" is not an XPath 2.0 expression: the grammar takes nothing at byte offset 18, which err:XPST0003 forbids`},
		{"@p:kind = 'x' extra", `"@p:kind = 'x' extra" is not an XPath 2.0 expression: the grammar takes nothing at byte offset 14, which err:XPST0003 forbids`},
		{"@p:kind = ", `"@p:kind = " is not an XPath 2.0 expression: the grammar needs more than its end, which err:XPST0003 forbids`},
		{"", `"" is not an XPath 2.0 expression: the grammar needs more than its end, which err:XPST0003 forbids`},
	} {
		record := ctaExprRecord(tc.expr, "", "a", "http://example.com/a", "xs", xsd.XMLSchemaNS)
		err := CTATestStaticError(record, seededTypes)
		if err == nil {
			t.Errorf("CTATestStaticError(%q) = nil, want err:XPST0003", tc.expr)
			continue
		}
		var verdict *xsderr.Error
		if !errors.As(err, &verdict) {
			t.Errorf("CTATestStaticError(%q) = %v, which is not an *xsderr.Error", tc.expr, err)
			continue
		}
		if verdict.Rule != ruleXPST0003 {
			t.Errorf("CTATestStaticError(%q) reports rule %q, want %q", tc.expr, verdict.Rule, ruleXPST0003)
		}
		if verdict.Msg != tc.msg {
			t.Errorf("CTATestStaticError(%q).Msg = %q, want %q", tc.expr, verdict.Msg, tc.msg)
		}
		if verdict.Loc != (xsderr.Loc{}) {
			t.Errorf("CTATestStaticError(%q) reports location %v, want the zero Loc", tc.expr, verdict.Loc)
		}
		if _, ok := CompileCTATest(record, seededTypes); ok {
			t.Errorf("CompileCTATest(%q) compiled, want declined", tc.expr)
		}
		if _, ok := CompileAssertionTest(record, seededTypes, xsd.EmptyContent{}, asUses(t, nil), asNoElems); ok {
			t.Errorf("CompileAssertionTest(%q) compiled, want declined", tc.expr)
		}
		if got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, record, fcValue(t, str, "x")); got != value.AssertionDeclined {
			t.Errorf("FacetAssertions.Evaluate(%q) = %d, want AssertionDeclined: a facet {test} outside the grammar is declined, never failed", tc.expr, got)
		}
	}
}

// ctaUserNS is the namespace of the two NON-builtin type definitions the cast
// target classification has to tell apart from a builtin.
const ctaUserNS = "http://example.com/a"

// ctaFixtureTypes is the seeded builtins plus a user-defined ATOMIC simple type
// and a COMPLEX type, both in ctaUserNS. Neither is a legal cast target, and
// the two are illegal for different reasons the classification keeps apart:
// a:Kind is valid XPath outside §3.12.6's required subset, a:Box is
// err:XPST0051.
func ctaFixtureTypes(t *testing.T) ctaTestTypes {
	t.Helper()
	types := make(ctaTestTypes, len(seededTypes)+2)
	for name, td := range seededTypes {
		types[name] = td
	}
	str, declared := seededTypes.Type(ctaBuiltin("string"))
	if !declared {
		t.Fatal("the seeded builtins hold no xs:string")
	}
	kind, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Space: ctaUserNS, Local: "Kind"},
		xsd.RestrictionDerivation{}, xsd.OwnedSimpleType{Definition: str.(*xsd.SimpleType)}, nil, nil)
	if err != nil {
		t.Fatalf("building the user-defined atomic type: %v", err)
	}
	types[kind.Name()] = kind
	box, err := xsd.NewComplexType(xsderr.Loc{}, xsd.QName{Space: ctaUserNS, Local: "Box"},
		xsd.QName{}, nil, xsd.DerivationRestriction, false, nil, nil, nil, xsd.EmptyContent{}, nil, nil)
	if err != nil {
		t.Fatalf("building the complex type: %v", err)
	}
	types[box.Name()] = box
	return types
}

// TestCompileClassifiesCastTargets pins the THREE-way split §3.12.6 clause 4
// and xpath20.md §3.10.2 make of a cast target, over both spellings:
//
//   - (a) a builtin atomic datatype is the required subset's own case and
//     compiles;
//   - (b) another in-scope atomic type is valid XPath outside the required
//     subset, which §3.12.6's Note licenses declining;
//   - (c) an unresolvable, complex, non-atomic or excluded target is
//     err:XPST0051/err:XPST0080, a STATIC error, which shares (b)'s decline
//     because it shares its consequence at the caller.
//
// The two declines are one encoding on purpose; what this test pins is that
// every member of both lands in it and that (a) does not.
func TestCompileClassifiesCastTargets(t *testing.T) {
	types := ctaFixtureTypes(t)
	bindings := []string{"a", ctaUserNS, "xs", xsd.XMLSchemaNS}
	for _, expr := range []string{
		"@k cast as xs:string",
		"@k cast as xs:integer",
		"xs:int(@k)",
		"@k cast as xs:date",
	} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, "", bindings...), types); !ok {
			t.Errorf("CompileCTATest(%q): declined, want compiled — (a) is a builtin atomic datatype", expr)
		}
	}
	for _, tc := range []struct{ expr, why string }{
		{"@k cast as a:Kind", "(b) a user-defined atomic type is outside the required subset"},
		{"a:Kind(@k)", "(b) in the constructor spelling too"},
		{"@k cast as a:Box", "(c) a complex type is no atomic type: err:XPST0051"},
		{"@k cast as xs:nosuchtype", "(c) an unresolvable target: err:XPST0051"},
		{"@k cast as xs:anySimpleType", "(c) an ·absent· {variety}: err:XPST0051"},
		{"@k cast as xs:IDREFS", "(c) a list builtin is no atomic type: err:XPST0051"},
		{"@k cast as xs:anyAtomicType", "(c) excluded by name: err:XPST0080"},
		{"@k cast as xs:NOTATION", "(c) excluded by name: err:XPST0080"},
		{"@k cast as xs:QName", "an xs:untypedAtomic operand to xs:QName is err:XPTY0004 (F&O §17.1), declined under castsFrom's marker"},
		{"@k cast as Kind", "an unprefixed target is in the {default namespace}, which declares none"},
	} {
		if _, ok := CompileCTATest(ctaExprRecord(tc.expr, "", bindings...), types); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
}

// TestCompileResolvesCastTargetInDefaultNamespace pins xpath20.md §3.10.2's
// one use of the {default namespace} in this grammar: "if the target type has
// no namespace prefix, it is considered to be in the default element/type
// namespace". The same unprefixed name declines against a record carrying no
// {default namespace}, so the test would notice a compiler that ignored the
// property.
func TestCompileResolvesCastTargetInDefaultNamespace(t *testing.T) {
	if _, ok := CompileCTATest(ctaExprRecord("@k cast as string", xsd.XMLSchemaNS), seededTypes); !ok {
		t.Error("an unprefixed cast target did not resolve in the {default namespace}")
	}
	if _, ok := CompileCTATest(ctaExprRecord("@k cast as string", ""), seededTypes); ok {
		t.Error("an unprefixed cast target resolved with no {default namespace} to take")
	}
}

// TestEvaluateCastToBuiltin pins that a cast DECIDES: the operand is validated
// against the target datatype and the comparison then runs in that type's
// value space rather than over lexicals.
func TestEvaluateCastToBuiltin(t *testing.T) {
	attrs := ctaAttrs(at("n", "4"), at("frac", "3.5"), at("length", "10"), at("width", "9"), at("d", "2026-03-01"), at("k", "book"))
	for _, tc := range []struct {
		expr string
		want bool
		why  string
	}{
		{"@n cast as xs:integer > 3", true, "the Goal's own example"},
		{"@n cast as xs:integer > 4", false, "a decided false"},
		{"@n cast as xs:integer = 4", true, "and equality in the numeric space"},
		{"xs:int(@length) > xs:int(@width)", true, "10 > 9 numerically, though \"10\" sorts before \"9\""},
		{"@length cast as xs:int > @width cast as xs:int", true, "the same pair in the cast spelling"},
		{"xs:int(@length) = @width cast as xs:int", false, "the two spellings mix in one comparison"},
		{"@d cast as xs:date = xs:date('2026-03-01')", true, "a constructor over a StringLiteral"},
		{"@d cast as xs:date > xs:date('2026-01-01')", true, "B.2 gives xs:date all six operators"},
		{"@d cast as xs:date < xs:date('2026-01-01')", false, "and orders them the right way round"},
		{"@k cast as xs:string = 'book'", true, "a cast to xs:string is the identity"},
	} {
		got := compile(t, tc.expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v (%s)", tc.expr, got, tc.want, tc.why)
		}
	}
}

// TestEvaluateFailedCastIsFalseNotDecline pins §3.12.4 clause 2 for the cast
// productions: a cast whose operand is not in the target type's ·lexical
// space· raises err:FORG0001, which makes the {test} false — not a decline,
// and not a node-local false that fn:not could invert.
func TestEvaluateFailedCastIsFalseNotDecline(t *testing.T) {
	attrs := ctaAttrs(at("frac", "3.5"), at("k", "book"), at("n", "4"))
	for _, tc := range []struct {
		expr string
		want bool
		why  string
	}{
		{"@frac cast as xs:integer > 3", false, "3.5 is not in xs:integer's lexical space"},
		{"not(@frac cast as xs:integer > 3)", false, "raised, so fn:not raises the same error"},
		{"@k cast as xs:integer = 1", false, "nor is a word"},
		{"xs:int(@k) = 1", false, "the constructor spelling raises identically"},
		{"not(xs:int(@k) = 1)", false, "and propagates identically"},
		{"@frac cast as xs:decimal > 3", true, "the discriminating row: a cast that SUCCEEDS still decides"},
		{"not(@n cast as xs:integer > 3)", false, "and a decided true still inverts"},
	} {
		got := compile(t, tc.expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v (%s)", tc.expr, got, tc.want, tc.why)
		}
	}
}

// TestEvaluateCastOfAbsentAttribute pins xpath20.md §3.10.2 rule 3, which the
// `?` occurrence indicator splits in two: with `?` the cast of an absent
// attribute IS the empty sequence, which forms no pair and is a DECIDED false;
// without `?` it is err:XPTY0004, which is a RAISED false. fn:not is what
// tells the two apart, and §3.10.4's T($arg) ≡ (($arg) cast as T?) is why the
// constructor spelling always takes the first.
func TestEvaluateCastOfAbsentAttribute(t *testing.T) {
	attrs := ctaAttrs(at("other", "x"))
	for _, tc := range []struct {
		expr string
		want bool
		why  string
	}{
		{"@n cast as xs:integer = 3", false, "err:XPTY0004: an empty operand with no `?`"},
		{"not(@n cast as xs:integer = 3)", false, "which is RAISED, so fn:not raises it too"},
		{"@n cast as xs:integer? = 3", false, "with `?` the cast is the empty sequence"},
		{"not(@n cast as xs:integer? = 3)", true, "which forms no pair: a decided false, and it inverts"},
		{"xs:integer(@n) = 3", false, "a constructor call over an absent attribute"},
		{"not(xs:integer(@n) = 3)", true, "carries the `?` of §3.10.4's equivalence"},
	} {
		got := compile(t, tc.expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v (%s)", tc.expr, got, tc.want, tc.why)
		}
	}
}

// TestEvaluateCastAgainstStringLiteral pins the grounding's worked case: a
// cast operand compared against a plain StringLiteral has no xpath20.md B.2
// row, because a literal is xs:string and shares no primitive with xs:float,
// and §3.5.2's untypedAtomic casting rules never fire for it. That is
// err:XPTY0004 and so a RAISED false, which is not the same answer as a
// comparison that ran.
func TestEvaluateCastAgainstStringLiteral(t *testing.T) {
	attrs := ctaAttrs(at("f", "1.5"), at("type", "float"))
	for _, tc := range []struct {
		expr string
		want bool
		why  string
	}{
		{"@f cast as xs:float = 'float'", false, "err:XPTY0004, though the cast itself succeeds"},
		{"not(@f cast as xs:float = 'float')", false, "raised, so fn:not raises it"},
		{"@type cast as xs:string = 'float'", true, "the discriminating row: xs:string against xs:string compares"},
	} {
		got := compile(t, tc.expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v (%s)", tc.expr, got, tc.want, tc.why)
		}
	}
}

// TestEvaluateCastNormalizesWhiteSpace pins xpath-functions.md §17.1.1's
// "whitespace normalization is applied as indicated by the whiteSpace facet
// for the datatype": a cast is the whole datatype validation, so casting to
// xs:token collapses the lexical before the comparison sees it, and the same
// operand uncast does not.
func TestEvaluateCastNormalizesWhiteSpace(t *testing.T) {
	attrs := ctaAttrs(at("k", "a  b"), at("pad", "  padded  "))
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"@k cast as xs:token = 'a b'", true},
		{"@k = 'a b'", false},
		{"@pad cast as xs:token = 'padded'", true},
		{"@pad = 'padded'", false},
	} {
		got := compile(t, tc.expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestEvaluateEffectiveBooleanValueOfACast pins fn:boolean's rules over a bare
// cast in a boolean position (xpath20.md §2.4.3): rule 1 for the empty
// sequence, rule 3 for xs:boolean, rule 4 for the string family, rule 5 for
// the numeric one — NaN included, which is false and not an error — and rule
// 6's err:FORG0006 for every other type.
func TestEvaluateEffectiveBooleanValueOfACast(t *testing.T) {
	attrs := ctaAttrs(at("zero", "0"), at("five", "5"), at("nan", "NaN"), at("empty", ""), at("word", "book"), at("yes", "true"), at("no", "0"), at("d", "2026-03-01"))
	for _, tc := range []struct {
		expr string
		want bool
		why  string
	}{
		{"@zero cast as xs:integer", false, "rule 5: numerically zero"},
		{"@five cast as xs:integer", true, "rule 5: anything else"},
		{"@nan cast as xs:double", false, "rule 5 names NaN outright"},
		{"not(@nan cast as xs:double)", true, "and it is a DECIDED false, so it inverts"},
		{"@empty cast as xs:string", false, "rule 4: zero length"},
		{"@word cast as xs:string", true, "rule 4: anything else"},
		{"@yes cast as xs:boolean", true, "rule 3: the value unchanged"},
		{"@no cast as xs:boolean", false, "including the lexical 0"},
		{"@d cast as xs:date", false, "rule 6: err:FORG0006 on every other type"},
		{"not(@d cast as xs:date)", false, "which is RAISED, so fn:not raises it"},
		{"xs:integer(@missing)", false, "rule 1: the empty sequence"},
		{"not(xs:integer(@missing))", true, "a decided false, and it inverts"},
	} {
		got := compile(t, tc.expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v (%s)", tc.expr, got, tc.want, tc.why)
		}
	}
}

// TestEvaluateStringComparators pins all six operators over the untypedAtomic
// attribute against a string literal, which §3.5.2 rule 2.4 compares as
// xs:string and B.2 decides through the default (codepoint) collation.
func TestEvaluateStringComparators(t *testing.T) {
	attrs := ctaAttrs(at("k", "book"))
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"@k = 'book'", true},
		{"@k = 'cd'", false},
		{"@k != 'cd'", true},
		{"@k != 'book'", false},
		{"@k < 'cd'", true},
		{"@k < 'book'", false},
		{"@k <= 'book'", true},
		{"@k > 'a'", true},
		{"@k >= 'book'", true},
		{"@k >= 'cd'", false},
	} {
		if got := compile(t, tc.expr).Evaluate(backend(), seededTypes, attrs); got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestEvaluateNumericComparators pins §3.5.2 clause 2.1: an untypedAtomic
// operand meeting a numeric one is cast to xs:double, whatever the literal's
// own numeric type, and compared in that space — so "3" and "3.0" and "03" are
// one answer and not three lexical ones.
func TestEvaluateNumericComparators(t *testing.T) {
	attrs := ctaAttrs(at("n", "3"), at("wide", "03.0"), at("bad", "three"), at("pad", "  3  "), at("big", "10"))
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"@n = 3", true},
		{"@n = 3.0", true},
		{"@n = 3e0", true},
		{"@n != 4", true},
		{"@n < 5", true},
		{"@n < 3", false},
		{"@n <= 3", true},
		{"@n > 0", true},
		{"@n >= 4", false},
		// "10" sorts BEFORE "9" under the codepoint collation and after it
		// in xs:double, so these two fail for an evaluator that compared the
		// lexicals instead of casting.
		{"@big > 9", true},
		{"@big < 9", false},
		{"@wide = 3", true},
		{"@pad = 3", true},
		{"@bad = 3", false},
		{"@bad != 3", false},
		{"@bad < 3", false},
	} {
		if got := compile(t, tc.expr).Evaluate(backend(), seededTypes, attrs); got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestEvaluateBadCastIsFalseNotDecline pins §3.12.4 clause 2 over its whole
// operator set: a cast that raises err:FORG0001 makes the {test} false rather
// than an error, and `!=` is false too — no pair of atomic values was formed,
// so nothing is unequal either.
func TestEvaluateBadCastIsFalseNotDecline(t *testing.T) {
	attrs := ctaAttrs(at("n", "not a number"))
	for _, expr := range []string{"@n = 1", "@n != 1", "@n < 1", "@n >= 1"} {
		if compile(t, expr).Evaluate(backend(), seededTypes, attrs) {
			t.Errorf("Evaluate(%q) = true, want false", expr)
		}
	}
}

// TestEvaluateAbsentAttributeIsFalse pins the empty-sequence rule of §3.5.2:
// no pair can be formed against an absent attribute, so EVERY comparator is
// false — including `!=`, which a naive reading would make true.
func TestEvaluateAbsentAttributeIsFalse(t *testing.T) {
	attrs := ctaAttrs(at("other", "x"))
	for _, expr := range []string{
		"@k = 'book'", "@k != 'book'", "@k < 'book'", "@k <= 'book'",
		"@k > 'book'", "@k >= 'book'", "@k = 1", "@k != 1",
	} {
		if compile(t, expr).Evaluate(backend(), seededTypes, attrs) {
			t.Errorf("Evaluate(%q) with no such attribute = true, want false", expr)
		}
	}
}

// TestEvaluateUnprefixedNameTestIsInNoNamespace pins xpath20.md:1521 against
// the trap it exists for: the record carries a {default namespace}, and the
// unprefixed NameTest still resolves to no namespace, because the attribute
// axis's principal node kind is never element (PRINCIPLES 15).
func TestEvaluateUnprefixedNameTestIsInNoNamespace(t *testing.T) {
	const ns = "http://example.com/d"
	c, ok := CompileCTATest(ctaExprRecord("@k = 'book'", ns), seededTypes)
	if !ok {
		t.Fatal("CompileCTATest: declined")
	}
	if !c.Evaluate(backend(), seededTypes, ctaAttrs(at("k", "book"))) {
		t.Error("an unprefixed NameTest did not match the no-namespace attribute")
	}
	inDefault := ctaAttrs(atNS(ns, "k", "book"))
	if c.Evaluate(backend(), seededTypes, inDefault) {
		t.Error("an unprefixed NameTest matched an attribute in the {default namespace}")
	}
}

// TestEvaluatePrefixedNameTest pins that a bound prefix resolves against the
// record's {namespace bindings}.
func TestEvaluatePrefixedNameTest(t *testing.T) {
	const ns = "http://example.com/c2"
	c := compile(t, "@c2:min = 1", "c2", ns)
	if !c.Evaluate(backend(), seededTypes, ctaAttrs(atNS(ns, "min", "1"))) {
		t.Error("a prefixed NameTest did not match its expanded name")
	}
	if c.Evaluate(backend(), seededTypes, ctaAttrs(at("min", "1"))) {
		t.Error("a prefixed NameTest matched a no-namespace attribute")
	}
}

// TestEvaluateAttributeAxisForm pins ta-props-correct clause 2.2: the
// unabbreviated attribute-axis spelling is the same AttrName as clause 2.1's
// abbreviation, and decides the same way.
func TestEvaluateAttributeAxisForm(t *testing.T) {
	attrs := ctaAttrs(at("k", "book"))
	for _, expr := range []string{"attribute::k = 'book'", "attribute :: k = 'book'"} {
		if !compile(t, expr).Evaluate(backend(), seededTypes, attrs) {
			t.Errorf("Evaluate(%q) = false, want true", expr)
		}
	}
	if compile(t, "attribute::k = 'cd'").Evaluate(backend(), seededTypes, attrs) {
		t.Error("attribute::k = 'cd' = true, want false")
	}
}

// ctaWildcardNS is the namespace the wildcard tests bind their prefix to, and
// the one their namespaced attributes carry.
const ctaWildcardNS = "http://example.com/w"

// TestCompileAdmitsWildcardNameTest pins that [17] ta-AttrName's NameTest is
// xpath20.md's [36] whole: all three [37] Wildcard spellings compile, in both
// the abbreviated and the attribute-axis form ta-props-correct clause 2 admits,
// and in every position a ta-AttrName reaches.
func TestCompileAdmitsWildcardNameTest(t *testing.T) {
	for _, expr := range []string{
		"@*",
		"@* = 'x'",
		"@w:* = 'x'",
		"@*:kind = 'x'",
		"attribute::* = 'x'",
		"attribute::w:* = 'x'",
		"attribute::*:kind = 'x'",
		"attribute :: * = 'x'",
		"'x' = @*",
		"@* = @*:kind",
		"not(@* = 'x')",
		"@* cast as xs:integer = 1",
		"xs:integer(@*) = 1",
		"@* = 'x' or @*:kind = 'y'",
	} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, "", "w", ctaWildcardNS, "xs", xsd.XMLSchemaNS), seededTypes); !ok {
			t.Errorf("CompileCTATest(%q): declined, want compiled", expr)
		}
	}
}

// TestEvaluateWildcardNameTest pins which attributes each [37] Wildcard arm
// selects (xpath20.md §3.2.1.2): `*` every one of them, `NCName:*` those in the
// namespace the prefix is bound to whatever their local name, and `*:NCName`
// those with that local name in ANY namespace or none.
//
// Each arm is paired with an attribute set it must NOT match, so an
// implementation that answered true for every wildcard would fail here.
func TestEvaluateWildcardNameTest(t *testing.T) {
	attrs := ctaAttrs(
		at("kind", "book"),
		atNS(ctaWildcardNS, "kind", "disc"),
		atNS(ctaWildcardNS, "size", "large"),
		atNS("http://example.com/other", "rank", "1"),
	)
	for _, tc := range []struct {
		expr string
		want bool
		why  string
	}{
		{"@* = 'book'", true, "`*` reaches the no-namespace attribute"},
		{"@* = 'large'", true, "and every namespaced one"},
		{"@* = 'missing'", false, "and matches no value it does not carry"},
		{"@w:* = 'disc'", true, "NCName:* takes any local name in the bound namespace"},
		{"@w:* = 'large'", true, "including the other one"},
		{"@w:* = 'book'", false, "and never the no-namespace attribute"},
		{"@w:* = '1'", false, "nor one in a different namespace"},
		{"@*:kind = 'book'", true, "*:NCName takes the local name in no namespace"},
		{"@*:kind = 'disc'", true, "and in any namespace"},
		{"@*:kind = 'large'", false, "and never another local name"},
		{"attribute::* = 'book'", true, "clause 2.2's spelling is the same NameTest"},
		{"attribute::w:* = 'disc'", true, "in the prefixed arm too"},
		{"attribute::*:kind = 'disc'", true, "and in the local-name one"},
	} {
		got := compile(t, tc.expr, "w", ctaWildcardNS).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v (%s)", tc.expr, got, tc.want, tc.why)
		}
	}
}

// TestEvaluateWildcardIsExistential pins xpath20.md §3.5.2 over the sequence a
// wildcard selects: "the result of the comparison is true if and only if there
// is a pair of atomic values ... that have the required magnitude relationship.
// Otherwise the result of the comparison is false."
//
// A wildcard matching SEVERAL attributes is the whole point — an evaluator that
// took only the first would answer false for the second row and true for the
// fifth. The fn:not rows separate that false from a raised error: an empty
// match set forms no pair, which is a DECIDED false and inverts.
func TestEvaluateWildcardIsExistential(t *testing.T) {
	three := ctaAttrs(at("a", "x"), at("b", "y"), at("c", "z"))
	for _, tc := range []struct {
		attrs Attributes
		expr  string
		want  bool
		why   string
	}{
		{three, "@* = 'x'", true, "the FIRST attribute satisfies it"},
		{three, "@* = 'z'", true, "the LAST one does, which a first-item reading would miss"},
		{three, "@* = 'q'", false, "no attribute does"},
		{three, "@* != 'x'", true, "some pair is unequal, though one is equal"},
		{three, "@* < 'y'", true, "the ordering comparators quantify the same way"},
		{three, "@* < 'a'", false, "and are false where no pair relates"},
		{three, "@* = @*", true, "both operands are sequences"},
		{ctaAttrs(), "@* = 'x'", false, "an element carrying NO attribute forms no pair"},
		{ctaAttrs(), "not(@* = 'x')", true, "which is a decided false, not an error, so it inverts"},
		{ctaAttrs(at("k", "book")), "@w:* = 'book'", false, "a wildcard matching nothing is the same empty sequence"},
		{ctaAttrs(at("k", "book")), "not(@w:* = 'book')", true, "and decides the same way"},
	} {
		got := compile(t, tc.expr, "w", ctaWildcardNS).Evaluate(backend(), seededTypes, tc.attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v (%s)", tc.expr, got, tc.want, tc.why)
		}
	}
}

// TestEvaluateWildcardOperandConvertsWhole pins that §3.5.2 clause 2's cast is
// applied to the WHOLE matched sequence: one attribute that does not cast into
// the comparison type raises for the operand, on §3.5.2's "may raise a dynamic
// error as soon as it encounters an error in evaluating either operand", rather
// than dropping out of the sequence.
//
// The fn:not rows are the assertion. Both halves false is the raised error; a
// per-item reading would decide the first row true and invert the second.
func TestEvaluateWildcardOperandConvertsWhole(t *testing.T) {
	mixed := ctaAttrs(at("n", "7"), at("k", "book"))
	numeric := ctaAttrs(at("n", "7"), at("m", "2"))
	for _, tc := range []struct {
		attrs Attributes
		expr  string
		want  bool
		why   string
	}{
		{mixed, "@* > 5", false, "err:FORG0001: \"book\" is no xs:double"},
		{mixed, "not(@* > 5)", false, "raised, so fn:not raises it too"},
		{ctaAttrs(at("k", "book"), at("n", "7")), "@* > 5", false, "wherever in the sequence the uncastable item sits"},
		{numeric, "@* > 5", true, "the discriminating row: every item casts, and 7 > 5"},
		{numeric, "@* > 9", false, "a decided false"},
		{numeric, "not(@* > 9)", true, "which inverts"},
	} {
		got := compile(t, tc.expr).Evaluate(backend(), seededTypes, tc.attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v (%s)", tc.expr, got, tc.want, tc.why)
		}
	}
}

// TestEvaluateWildcardUnderACast pins xpath20.md §3.10.2 rule 3 over the one
// operand length only a wildcard can produce: a cast admits "exactly one atomic
// value", and the `?` occurrence indicator widens that to AT MOST one, so TWO
// OR MORE matched attributes are err:XPTY0004 under either spelling and never
// the first of them.
func TestEvaluateWildcardUnderACast(t *testing.T) {
	one := ctaAttrs(at("n", "3"))
	two := ctaAttrs(at("n", "3"), at("m", "4"))
	for _, tc := range []struct {
		attrs Attributes
		expr  string
		want  bool
		why   string
	}{
		{one, "@* cast as xs:integer = 3", true, "a singleton sequence casts"},
		{two, "@* cast as xs:integer = 3", false, "err:XPTY0004: two items under a cast"},
		{two, "not(@* cast as xs:integer = 3)", false, "raised, so fn:not raises it"},
		{two, "@* cast as xs:integer? = 3", false, "`?` widens rule 3 to at most one, not to more"},
		{two, "not(@* cast as xs:integer? = 3)", false, "so this raises too"},
		{two, "xs:integer(@*) = 3", false, "and §3.10.4's constructor equivalence carries the same `?`"},
		{ctaAttrs(), "@* cast as xs:integer = 3", false, "err:XPTY0004: no items and no `?`"},
		{ctaAttrs(), "not(@* cast as xs:integer = 3)", false, "which is raised"},
		{ctaAttrs(), "not(@* cast as xs:integer? = 3)", true, "while `?` makes it the empty sequence, a decided false"},
	} {
		got := compile(t, tc.expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, tc.attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v (%s)", tc.expr, got, tc.want, tc.why)
		}
	}
}

// TestEvaluateWildcardEffectiveBooleanValue pins fn:boolean's rule 2 over a
// bare wildcard AttrName (xpath20.md §2.4.3): a sequence whose first item is a
// node is true whatever its length, so a wildcard in a boolean position asks
// only whether E carries an attribute its NameTest selects.
func TestEvaluateWildcardEffectiveBooleanValue(t *testing.T) {
	for _, tc := range []struct {
		attrs Attributes
		expr  string
		want  bool
	}{
		{ctaAttrs(at("a", "x"), at("b", "y")), "@*", true},
		{ctaAttrs(at("a", "")), "@*", true},
		{ctaAttrs(), "@*", false},
		{ctaAttrs(), "not(@*)", true},
		{ctaAttrs(at("a", "x")), "@w:*", false},
		{ctaAttrs(atNS(ctaWildcardNS, "a", "x")), "@w:*", true},
		{ctaAttrs(at("kind", "x")), "@*:kind", true},
		{ctaAttrs(at("kind", "x")), "@*:other", false},
	} {
		got := compile(t, tc.expr, "w", ctaWildcardNS).Evaluate(backend(), seededTypes, tc.attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestEvaluateConnectives pins [9], [10], [11]'s parenthesised arm and the
// fn:not the [12] production is fixed to, including that grouping changes the
// answer (so the test would notice an evaluator that ignored the parens).
func TestEvaluateConnectives(t *testing.T) {
	attrs := ctaAttrs(at("a", "x"), at("b", "y"), at("c", "z"))
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"@a = 'x' or @b = 'no'", true},
		{"@a = 'no' or @b = 'no'", false},
		{"@a = 'x' and @b = 'y'", true},
		{"@a = 'x' and @b = 'no'", false},
		{"not(@a = 'x')", false},
		{"not(@a = 'no')", true},
		{"fn:not(@a = 'no')", true},
		{"@a = 'no' and @b = 'no' or @c = 'z'", true},
		{"@a = 'no' and (@b = 'no' or @c = 'z')", false},
		{"not(@a = 'no' or @b = 'no')", true},
	} {
		if got := compile(t, tc.expr, "fn", ctaFunctionNS).Evaluate(backend(), seededTypes, attrs); got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestEvaluateRaisedErrorFalsifiesTheWholeTest pins key-cta-ta-select clause 2
// (§3.12.4) at the granularity its subject fixes: "the {test} is treated as if
// it had evaluated (without error) to false" — the {test}, not the node that
// raised. fn:not is a function, and xpath20.md §3.6 makes it raise its
// operand's error rather than absorb it, so no number of fn:not calls turns a
// raised error into a true {test}.
//
// The two error sources are separate: @n's failed cast to xs:double is
// err:FORG0001 (a dynamic error), and a string literal against a numeric one
// has no xpath20.md B.2 row at all, which is err:XPTY0004 (a type error).
// Clause 2 names both.
func TestEvaluateRaisedErrorFalsifiesTheWholeTest(t *testing.T) {
	attrs := ctaAttrs(at("n", "abc"), at("k", "x"))
	for _, tc := range []struct {
		expr string
		want bool
		why  string
	}{
		{"@n = 5", false, "err:FORG0001: a PRESENT @n that does not cast to xs:double"},
		{"not(@n = 5)", false, "fn:not raises the same error; it does not invert it"},
		{"not(not(@n = 5))", false, "two fn:not calls propagate it twice"},
		{"not(@n != 5)", false, "every comparator reaches the same failed cast"},
		{"not(@n < 5)", false, "including the ordering ones"},
		{"'a' = 5", false, "err:XPTY0004: no B.2 row for a string against a numeric"},
		{"not('a' = 5)", false, "the type error propagates through fn:not too"},
		{"not(not('a' = 5))", false, "and stays raised however deep the nesting"},
		// The discriminating rows: fn:not still inverts an answer that was
		// DECIDED, so the fix cannot be "not(...) is always false".
		{"not(@missing = 5)", true, "an absent attribute forms no pair — false, not an error"},
		{"not(@k = 'no')", true, "a decided false inverts"},
		{"not(@k = 'x')", false, "a decided true inverts"},
	} {
		if got := compile(t, tc.expr).Evaluate(backend(), seededTypes, attrs); got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v (%s)", tc.expr, got, tc.want, tc.why)
		}
	}
}

// TestEvaluateRaisedErrorUnderConnectives pins xpath20.md §3.6's truth tables
// over an operand that RAISES, read with XPath 1.0 compatibility mode false
// (xpath-valid clause 2.2.1). Every bare row is the answer this evaluator
// already gave before the error became a state of its own, so a landing that
// moves one has changed something it should not have; the fn:not rows are
// where the tables become observable, because only there does the difference
// between "false" and "raised" survive to the {test}.
func TestEvaluateRaisedErrorUnderConnectives(t *testing.T) {
	attrs := ctaAttrs(at("n", "abc"), at("k", "x"))
	for _, tc := range []struct {
		expr string
		want bool
		cell string
	}{
		{"@n = 5 or @k = 'x'", true, "error or true: the cell permits either, and true is the standing answer"},
		{"@k = 'x' or @n = 5", true, "true or error: same cell, other order"},
		{"@n = 5 or @k = 'no'", false, "error or false: the error, which clause 2 makes false"},
		{"@n = 5 and @k = 'x'", false, "error and true: the error"},
		{"@k = 'no' and @n = 5", false, "false and error: the false decides, and the raising operand is never evaluated"},
		{"@n = 5 and @k = 'no'", false, "error and false: the cell permits either, and this evaluator takes the false"},
		{"@k = 'x' and @n = 5", false, "true and error: the error"},
		{"not(@n = 5 or @k = 'no')", false, "the or raised, so fn:not raises: NOT true"},
		{"not(@k = 'x' and @n = 5)", false, "the and raised, so fn:not raises: NOT true"},
		{"not(@n = 5 or @k = 'x')", false, "the or is a decided true, which inverts"},
		{"not(@k = 'no' and @n = 5)", true, "the and is a decided false, which inverts"},
		{"not(@n = 5 and @k = 'no')", true, "error and false: this evaluator takes the cell's false, so it inverts"},
	} {
		if got := compile(t, tc.expr).Evaluate(backend(), seededTypes, attrs); got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v (%s)", tc.expr, got, tc.want, tc.cell)
		}
	}
}

// TestEvaluateEffectiveBooleanValue pins [11]'s third arm with its Comparator
// ABSENT: a bare ValueExpr in a boolean position takes its ·effective boolean
// value· (xpath20.md §2.4.3), which for an AttrName is presence and for a
// Literal is fn:boolean's rules 4 and 5.
func TestEvaluateEffectiveBooleanValue(t *testing.T) {
	attrs := ctaAttrs(at("a", "x"), at("empty", ""))
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"@a", true},
		{"@empty", true}, // rule 2: a node, whatever its string value
		{"@missing", false},
		{"@a and @empty", true},
		{"@a and @missing", false},
		{"not(@missing)", true},
		{"'x'", true},
		{"''", false},
		{"1", true},
		{"0", false},
		{"0.0", false},
		{"0e0", false},
		{"1.5", true},
	} {
		if got := compile(t, tc.expr).Evaluate(backend(), seededTypes, attrs); got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestEvaluateLiteralOnlyComparisons pins the pairs with no untypedAtomic
// operand: two literals of one space compare in it, a decimal literal meeting
// a double one promotes, and a string literal against a numeric one is
// err:XPTY0004 — which §3.12.4 clause 2 decides as false rather than
// declining.
func TestEvaluateLiteralOnlyComparisons(t *testing.T) {
	attrs := ctaAttrs()
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"'a' = 'a'", true},
		{"'a' = 'b'", false},
		{"'a' < 'b'", true},
		{"1 = 1", true},
		{"1 = 1.0", true},
		{"1 = 1e0", true},
		{"1 < 2", true},
		{"1.5 > 1", true},
		{"'1' = 1", false},
		{"'1' != 1", false},
		{"1 = 'a'", false},
	} {
		if got := compile(t, tc.expr).Evaluate(backend(), seededTypes, attrs); got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestEvaluateStringLiteralEscapes pins xpath20.md [74]: the quote character
// appears inside a literal only doubled, and denotes one such character.
func TestEvaluateStringLiteralEscapes(t *testing.T) {
	attrs := ctaAttrs(at("k", "it's"))
	for _, expr := range []string{"@k = 'it''s'", "@k = \"it's\""} {
		if !compile(t, expr).Evaluate(backend(), seededTypes, attrs) {
			t.Errorf("Evaluate(%q) = false, want true", expr)
		}
	}
	if compile(t, "@k = 'it''''s'").Evaluate(backend(), seededTypes, attrs) {
		t.Error("a doubled pair was not folded to one quote")
	}
}

// TestEvaluateZeroTestDecides pins that the zero CTATest — which no successful
// CompileCTATest produces — still decides rather than panicking.
func TestEvaluateZeroTestDecides(t *testing.T) {
	var zero CTATest
	if zero.Evaluate(backend(), seededTypes, ctaAttrs()) {
		t.Error("the zero CTATest evaluated true")
	}
}

// TestNumericLiteralType pins which builtin datatype each NumericLiteral kind
// carries, because it is what decides whether two literals are compared as
// xs:decimal or promoted to xs:double.
func TestNumericLiteralType(t *testing.T) {
	known := compileTypes(t)
	for _, tc := range []struct {
		text string
		want string
	}{
		{"1", "decimal"},
		{"1.5", "decimal"},
		{".5", "decimal"},
		{"1e0", "double"},
		{"1.5E-2", "double"},
	} {
		if got := known.literal(tc.text).Name(); got != ctaBuiltin(tc.want) {
			t.Errorf("literal(%q) = %v, want xs:%s", tc.text, got, tc.want)
		}
	}
}

// castNode is a compiled `@a cast as xs:<local>` node, which is the only way a
// TYPED operand other than a Literal reaches a comparison.
func castNode(t *testing.T, known ctaTypes, local string) ctaValue {
	t.Helper()
	target, admitted := known.castTarget(ctaBuiltin(local))
	if !admitted {
		t.Fatalf("castTarget(xs:%s): declined, want admitted", local)
	}
	return ctaCast{operand: ctaAttr{test: ctaExactName{name: uq("a")}}, target: target}
}

// compileTypes is the compile-time type knowledge over the seeded builtins.
func compileTypes(t *testing.T) ctaTypes {
	t.Helper()
	known, ok := ctaResolveTypes(seededTypes)
	if !ok {
		t.Fatal("ctaResolveTypes over the seeded builtins: declined")
	}
	return known
}

// TestComparisonType pins §3.5.2's casting rules and B.1's promotions as a
// table, so the type a pair is compared in is checked directly and not only
// through an evaluation that could agree by accident. It re-derives the table
// the deleted ctaComparisonSpace pinned, over datatypes rather than over the
// three-valued space that function could name.
//
// All three ctaTyping outcomes are in the table, because they are three
// different directions: a settled type, err:XPTY0004 (a decided false), and
// the withhold #889 owns.
//
// It reads ctaTypes.converted rather than ctaTypes.comparison, which is the
// same table plus B.2's per-operator rows; those are
// TestComparisonOperatorLegality's subject.
func TestComparisonType(t *testing.T) {
	known := compileTypes(t)
	attr := ctaAttr{test: ctaExactName{name: uq("a")}}
	str := ctaLiteral{text: "x", st: known.str}
	dec := ctaLiteral{text: "1", st: known.decimal}
	dbl := ctaLiteral{text: "1e0", st: known.double}
	for _, tc := range []struct {
		name        string
		left, right ctaValue
		// want is the comparison type's local name, and is empty for the two
		// outcomes that settle none.
		want   string
		typing ctaTyping
	}{
		{"two attributes are clause 1's xs:string", attr, attr, "string", ctaTypeSettled},
		{"attribute vs string literal is clause 2.4", attr, str, "string", ctaTypeSettled},
		{"attribute vs decimal literal is clause 2.1", attr, dec, "double", ctaTypeSettled},
		{"decimal literal vs attribute is clause 2.1", dec, attr, "double", ctaTypeSettled},
		{"attribute vs double literal is clause 2.1", attr, dbl, "double", ctaTypeSettled},
		{"two decimal literals stay xs:decimal", dec, dec, "decimal", ctaTypeSettled},
		{"decimal vs double literal promotes", dec, dbl, "double", ctaTypeSettled},
		{"two string literals stay xs:string", str, str, "string", ctaTypeSettled},
		{"string vs numeric literal has no B.2 row", str, dec, "", ctaTypeErrored},
		{"a cast vs a literal of its own primitive", castNode(t, known, "int"), dec, "decimal", ctaTypeSettled},
		{"an attribute vs a numeric cast is clause 2.1", attr, castNode(t, known, "int"), "double", ctaTypeSettled},
		{"an attribute vs a date cast is clause 2.4", attr, castNode(t, known, "date"), "date", ctaTypeSettled},
		{"an attribute vs a dayTimeDuration cast is clause 2.2", attr, castNode(t, known, "dayTimeDuration"), "dayTimeDuration", ctaTypeSettled},
		{"an attribute vs a yearMonthDuration cast is clause 2.3", attr, castNode(t, known, "yearMonthDuration"), "yearMonthDuration", ctaTypeSettled},
		{"two date casts share their primitive", castNode(t, known, "date"), castNode(t, known, "date"), "date", ctaTypeSettled},
		// The two duration subtypes keep their NAMES where both operands
		// carry one, because B.2 writes their ordering rows under those names
		// and gives their xs:duration primitive none (ctaTypes.sharedNamed).
		{"two dayTimeDuration casts keep the named subtype", castNode(t, known, "dayTimeDuration"), castNode(t, known, "dayTimeDuration"), "dayTimeDuration", ctaTypeSettled},
		{"two yearMonthDuration casts keep the named subtype", castNode(t, known, "yearMonthDuration"), castNode(t, known, "yearMonthDuration"), "yearMonthDuration", ctaTypeSettled},
		{"the two duration subtypes share only their primitive", castNode(t, known, "dayTimeDuration"), castNode(t, known, "yearMonthDuration"), "duration", ctaTypeSettled},
		{"two duration casts share their primitive", castNode(t, known, "duration"), castNode(t, known, "duration"), "duration", ctaTypeSettled},
		{"a date cast vs a string literal shares nothing", castNode(t, known, "date"), str, "", ctaTypeErrored},
		{"xs:boolean pairs are admitted for every comparator", castNode(t, known, "boolean"), castNode(t, known, "boolean"), "boolean", ctaTypeSettled},
		{"a token cast vs a string literal is xs:string", castNode(t, known, "token"), str, "string", ctaTypeSettled},
		// B.1's URI promotion is applied at the value comparison and not to
		// the comparison TYPE, so an xs:anyURI pair stays xs:anyURI: clause
		// 2.4 must cast the untypedAtomic operand to xs:anyURI, whose
		// whiteSpace facet is collapse, and answering xs:string here would
		// preserve it instead.
		{"an attribute vs an anyURI cast is clause 2.4", attr, castNode(t, known, "anyURI"), "anyURI", ctaTypeSettled},
		{"two anyURI casts share their primitive", castNode(t, known, "anyURI"), castNode(t, known, "anyURI"), "anyURI", ctaTypeSettled},
		{"an anyURI cast vs a string literal is B.1's URI promotion", castNode(t, known, "anyURI"), str, "string", ctaTypeSettled},
		// B.1 rule 1.2 is "created by casting" and stays admitted in both
		// directions; rule 1.1 is the withhold. (#889)
		{"a float cast vs a decimal literal promotes to the wider", castNode(t, known, "float"), dec, "float", ctaTypeSettled},
		{"a double cast vs a decimal literal promotes to the wider", castNode(t, known, "double"), dec, "double", ctaTypeSettled},
		{"two float casts share their primitive", castNode(t, known, "float"), castNode(t, known, "float"), "float", ctaTypeSettled},
		{"a float cast vs a double literal needs rule 1.1", castNode(t, known, "float"), dbl, "", ctaTypeDeclined},
		{"a double literal vs a float cast needs rule 1.1", dbl, castNode(t, known, "float"), "", ctaTypeDeclined},
		{"a float cast vs a double cast needs rule 1.1", castNode(t, known, "float"), castNode(t, known, "double"), "", ctaTypeDeclined},
		{"an attribute vs a float cast needs rule 1.1 under clause 2.1", attr, castNode(t, known, "float"), "", ctaTypeDeclined},
		{"a float cast vs an attribute needs rule 1.1 under clause 2.1", castNode(t, known, "float"), attr, "", ctaTypeDeclined},
	} {
		got, typing := known.converted(tc.left, tc.right)
		if typing != tc.typing {
			t.Errorf("%s: typing = %v, want %v", tc.name, typing, tc.typing)
			continue
		}
		if typing == ctaTypeSettled && got.Name() != ctaBuiltin(tc.want) {
			t.Errorf("%s: comparison type = %v, want xs:%s", tc.name, got.Name(), tc.want)
		}
	}
}

// TestComparisonOperatorLegality pins xpath20.md B.2's PER-OPERATOR rows: a
// comparison is admitted exactly where B.2 holds a row for that operator over
// the type both operands are converted into, and is err:XPTY0004 otherwise
// (§3.5.1), whatever the operand values are then able to answer.
//
// The table names both directions B.2 disagrees with a value-space judgment
// in. The types with eq and ne rows and no ordering rows are xs:duration, the
// five Gregorian types, xs:hexBinary and xs:base64Binary; xs:boolean has all
// six, its ordering rows included, and xs:QName has eq and ne alone. xs:NOTATION
// is the eq/ne-only type the table cannot reach, being declined as a cast
// target before a comparison over it can be built (ctaTypes.castTarget).
func TestComparisonOperatorLegality(t *testing.T) {
	known := compileTypes(t)
	equality := []ctaComparator{ctaEqual, ctaNotEqual}
	ordering := []ctaComparator{ctaLess, ctaLessEqual, ctaGreater, ctaGreaterEqual}
	all := []ctaComparator{ctaEqual, ctaNotEqual, ctaLess, ctaLessEqual, ctaGreater, ctaGreaterEqual}
	for _, tc := range []struct {
		name   string
		local  string
		ops    []ctaComparator
		typing ctaTyping
	}{
		{"xs:duration has eq and ne", "duration", equality, ctaTypeSettled},
		{"xs:duration has no ordering row", "duration", ordering, ctaTypeErrored},
		{"xs:gYear has eq and ne", "gYear", equality, ctaTypeSettled},
		{"xs:gYear has no ordering row", "gYear", ordering, ctaTypeErrored},
		{"xs:gYearMonth has no ordering row", "gYearMonth", ordering, ctaTypeErrored},
		{"xs:gMonthDay has no ordering row", "gMonthDay", ordering, ctaTypeErrored},
		{"xs:gDay has no ordering row", "gDay", ordering, ctaTypeErrored},
		{"xs:gMonth has no ordering row", "gMonth", ordering, ctaTypeErrored},
		{"xs:hexBinary has eq and ne", "hexBinary", equality, ctaTypeSettled},
		{"xs:hexBinary has no ordering row", "hexBinary", ordering, ctaTypeErrored},
		{"xs:base64Binary has eq and ne", "base64Binary", equality, ctaTypeSettled},
		{"xs:base64Binary has no ordering row", "base64Binary", ordering, ctaTypeErrored},
		{"xs:boolean has all six", "boolean", all, ctaTypeSettled},
		{"xs:QName has eq and ne", "QName", equality, ctaTypeSettled},
		{"xs:QName has no ordering row", "QName", ordering, ctaTypeErrored},
		// The two duration subtypes carry the ordering rows their primitive
		// lacks, and reach eq and ne through it by subtype substitution.
		{"xs:dayTimeDuration has all six", "dayTimeDuration", all, ctaTypeSettled},
		{"xs:yearMonthDuration has all six", "yearMonthDuration", all, ctaTypeSettled},
		// A subtype of one reaches the same rows, which is what makes the
		// lookup a walk of the {base type definition} chain and not a name
		// match: xs:dateTimeStamp is derived from xs:dateTime.
		{"xs:dateTimeStamp reaches xs:dateTime's rows", "dateTimeStamp", all, ctaTypeSettled},
		{"xs:date has all six", "date", all, ctaTypeSettled},
		{"xs:time has all six", "time", all, ctaTypeSettled},
		{"xs:string has all six", "string", all, ctaTypeSettled},
		{"xs:int reaches numeric's rows", "int", all, ctaTypeSettled},
		// xs:precisionDecimal is the ONE cast target this engine admits that
		// B.2 holds no row of any kind for: XPath 2.0's operator mapping
		// predates it and derives it from xs:anyAtomicType, so no ancestor
		// carries a row and every comparison over it is err:XPTY0004.
		{"xs:precisionDecimal has no row at all", "precisionDecimal", all, ctaTypeErrored},
	} {
		for _, op := range tc.ops {
			left := castNode(t, known, tc.local)
			right := castNode(t, known, tc.local)
			got, typing := known.comparison(op, left, right)
			if typing != tc.typing {
				t.Errorf("%s: comparison(%v) typing = %v, want %v", tc.name, op, typing, tc.typing)
				continue
			}
			if typing == ctaTypeSettled && got == nil {
				t.Errorf("%s: comparison(%v) settled on no type", tc.name, op)
			}
		}
	}
}

// TestEvaluateOrderingWithoutB2Row is the first defect direction's witness at
// the evaluation surface: an ordering comparison over a type B.2 gives eq and
// ne rows but no ordering rows is err:XPTY0004, which key-cta-ta-select clause
// 2 (§3.12.4) makes the {test} false.
//
// Each case is paired with the same expression under fn:not, and that pairing
// is the whole assertion: a DECIDED false inverts to true, while a raised
// error propagates through fn:not and is still false at the root. Both halves
// false is therefore the error and not a comparison that merely came out
// false — the xs:duration and Gregorian values these expressions compare are
// value.Ordered under the strict backend, so a comparator reading the value
// space would answer true for every ordering row here.
func TestEvaluateOrderingWithoutB2Row(t *testing.T) {
	attrs := ctaAttrs(at("a", "P2D"), at("b", "P1D"), at("y", "2001"), at("z", "2000"), at("h", "0F"), at("i", "0A"))
	for _, expr := range []string{
		"@a cast as xs:duration > @b cast as xs:duration",
		"@a cast as xs:duration >= @b cast as xs:duration",
		"@b cast as xs:duration < @a cast as xs:duration",
		"@b cast as xs:duration <= @a cast as xs:duration",
		"@y cast as xs:gYear > @z cast as xs:gYear",
		"@z cast as xs:gYear < @y cast as xs:gYear",
		"@h cast as xs:hexBinary > @i cast as xs:hexBinary",
		"@h cast as xs:base64Binary > @h cast as xs:base64Binary",
	} {
		if got := compile(t, expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs); got {
			t.Errorf("Evaluate(%q) = true, want false (err:XPTY0004)", expr)
		}
		negated := "not(" + expr + ")"
		if got := compile(t, negated, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs); got {
			t.Errorf("Evaluate(%q) = true, want false: the error propagates through fn:not", negated)
		}
	}
}

// TestEvaluateEqualityWithoutOrdering pins the other half of the same rows:
// the types B.2 denies an ordering row still compare for equality, so the fix
// narrowed the operators and not the operand set.
func TestEvaluateEqualityWithoutOrdering(t *testing.T) {
	attrs := ctaAttrs(at("a", "P2D"), at("b", "P1D"), at("y", "2001"), at("z", "2000"), at("h", "0F"))
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"@a cast as xs:duration = @a cast as xs:duration", true},
		{"@a cast as xs:duration = @b cast as xs:duration", false},
		{"@a cast as xs:duration != @b cast as xs:duration", true},
		{"@y cast as xs:gYear = @y cast as xs:gYear", true},
		{"@y cast as xs:gYear = @z cast as xs:gYear", false},
		{"@h cast as xs:hexBinary = @h cast as xs:hexBinary", true},
	} {
		got := compile(t, tc.expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestEvaluateBooleanComparisons is the second defect direction's witness:
// B.2 gives xs:boolean all six rows, so an ordering comparison over two
// xs:boolean operands DECIDES — through op:boolean-less-than's own definition,
// "false is less than true" (xpath-functions.md §9.2.2) — rather than raising
// because the value space carries no order.
//
// The fn:not rows are what separate a decided false from a raised error: they
// invert, which an error would not.
func TestEvaluateBooleanComparisons(t *testing.T) {
	attrs := ctaAttrs(at("t", "true"), at("f", "0"))
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"@f cast as xs:boolean < @t cast as xs:boolean", true},
		{"@t cast as xs:boolean < @f cast as xs:boolean", false},
		{"not(@t cast as xs:boolean < @f cast as xs:boolean)", true},
		{"@t cast as xs:boolean > @f cast as xs:boolean", true},
		{"@f cast as xs:boolean > @t cast as xs:boolean", false},
		{"@t cast as xs:boolean <= @t cast as xs:boolean", true},
		{"@t cast as xs:boolean <= @f cast as xs:boolean", false},
		{"@f cast as xs:boolean >= @f cast as xs:boolean", true},
		{"@f cast as xs:boolean >= @t cast as xs:boolean", false},
		{"@t cast as xs:boolean = @t cast as xs:boolean", true},
		{"@t cast as xs:boolean != @f cast as xs:boolean", true},
	} {
		got := compile(t, tc.expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestEvaluateDateTimeAtImplicitTimezone pins F&O §10.4 on the Type Alternative
// façade: an untyped @d cast to xs:date against a date (§3.5.2 clause 2.4) and
// compared with a timezoned one is decided under the implicit timezone,
// ctaImplicitTimezone (Z). The tautology is true whatever the constant is, and
// the `=` rows are true because it is Z; every mixed row fails with the
// date/time arm of ctaHoldsPair removed, which answers the tautology false.
// Two untimezoned operands, and two timezoned ones, compare as before the arm,
// and a g* ordering stays err:XPTY0004 (B.2), which fn:not does not invert.
func TestEvaluateDateTimeAtImplicitTimezone(t *testing.T) {
	attrs := ctaAttrs(at("d", "2000-01-01"), at("z", "2000-01-01Z"), at("g", "---01"))
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"@d < xs:date('2000-01-01Z') or @d >= xs:date('2000-01-01Z')", true},
		{"@d = xs:date('2000-01-01Z')", true},
		{"@d != xs:date('2000-01-01Z')", false},
		{"@d <= xs:date('2000-01-01Z')", true},
		{"@d > xs:date('2000-01-01+14:00')", true},
		{"@g cast as xs:gDay = xs:gDay('---01Z')", true},
		{"@d = xs:date('2000-01-01')", true},
		{"@d < xs:date('2000-01-02')", true},
		{"@z = xs:date('2000-01-01+14:00')", false},
		{"@z > xs:date('2000-01-01+14:00')", true},
		{"@g cast as xs:gDay < xs:gDay('---02')", false},
		{"not(@g cast as xs:gDay < xs:gDay('---02'))", false},
	} {
		got := compile(t, tc.expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestEvaluateDurationSubtypeOrdering guards the regression the B.2 lookup
// invites: B.2 writes the four ordering rows under xs:yearMonthDuration and
// xs:dayTimeDuration and none under their xs:duration primitive, so reducing
// two operands of one subtype to that primitive would turn a comparison B.2
// admits into err:XPTY0004 (ctaTypes.sharedNamed).
//
// The last two rows are the pairing that legitimately does NOT order: one
// operand of each subtype shares only xs:duration, which has eq and ne and no
// ordering row.
func TestEvaluateDurationSubtypeOrdering(t *testing.T) {
	attrs := ctaAttrs(at("a", "P2D"), at("b", "PT1H"), at("y", "P1Y"), at("m", "P1M"))
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"@a cast as xs:dayTimeDuration > @b cast as xs:dayTimeDuration", true},
		{"@b cast as xs:dayTimeDuration > @a cast as xs:dayTimeDuration", false},
		{"not(@b cast as xs:dayTimeDuration > @a cast as xs:dayTimeDuration)", true},
		{"@b cast as xs:dayTimeDuration <= @a cast as xs:dayTimeDuration", true},
		{"@y cast as xs:yearMonthDuration > @m cast as xs:yearMonthDuration", true},
		{"@m cast as xs:yearMonthDuration >= @y cast as xs:yearMonthDuration", false},
		{"not(@m cast as xs:yearMonthDuration >= @y cast as xs:yearMonthDuration)", true},
		{"@y cast as xs:yearMonthDuration = @y cast as xs:yearMonthDuration", true},
		{"@a cast as xs:dayTimeDuration > @y cast as xs:yearMonthDuration", false},
		{"not(@a cast as xs:dayTimeDuration > @y cast as xs:yearMonthDuration)", false},
	} {
		got := compile(t, tc.expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestEvaluateYearMonthDurationZeroPromotes pins ctaCanonical's walk past a
// mapping that cannot render the value: xs:yearMonthDuration's own canonical
// mapping has no form for P0M (§3.4.26.1 Note), so the promotion into
// xs:duration renders it through xs:duration's mapping, "PT0S", and the
// comparison decides. Stopping the walk at the nearest mapped type raises
// instead, and both rows answer false.
func TestEvaluateYearMonthDurationZeroPromotes(t *testing.T) {
	attrs := ctaAttrs(at("z", "P0M"), at("d", "PT0S"))
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"@z cast as xs:yearMonthDuration = @d cast as xs:duration", true},
		{"not(@z cast as xs:yearMonthDuration != @d cast as xs:duration)", true},
	} {
		got := compile(t, tc.expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestPromotePrecisionDecimalZeroBeyondCapacity calls ctaPromote directly — no
// compiled test reaches it with an xs:precisionDecimal operand, which has no
// B.2 row (TestComparisonOperatorLegality) — with a zero whose ·scale·
// 99999999999 has a canonical form of about 100 GB. The promotion raises on
// the backend's beyond-capacity error (xmlschema11-2 §5.4) inside the test's
// budget and well under ctaPromoteAllocBound, where rendering it would pad
// 99999999999 zeros. The ·scale· 2 row is the control: the same call renders a
// small zero and casts it into xs:string.
func TestPromotePrecisionDecimalZeroBeyondCapacity(t *testing.T) {
	// ctaPromoteAllocBound is far above the walk's and the error's own
	// allocations and far below the 100 GB the padded form needs.
	const ctaPromoteAllocBound = 1 << 20
	env := ctaEnv{backend: backend(), types: seededTypes, input: ctaLexicalInput{}}
	types := compileTypes(t)
	from, _ := types.castTarget(ctaBuiltin("precisionDecimal"))
	to, _ := types.castTarget(ctaBuiltin("string"))
	m, _ := backend().Mapping(ctaBuiltin("precisionDecimal"))

	control, err := m.Parse("0.00", nil)
	if err != nil {
		t.Fatalf("Parse(0.00): %v", err)
	}
	cast, converted := ctaValidated(ctaPromote(control, from, to, env))
	if !converted {
		t.Fatal("ctaPromote(0.00 → xs:string) raised, want the canonical 0.0E0 padded to ·scale· 2")
	}
	if s, _ := cast.(value.Canonical); s == nil || s.Canonical() != "0.00E0" {
		t.Fatalf("ctaPromote(0.00 → xs:string) = %v, want 0.00E0", cast)
	}

	huge, err := m.Parse("0E-99999999999", nil)
	if err != nil {
		t.Fatalf("Parse(0E-99999999999): %v", err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got := ctaPromote(huge, from, to, env)
	runtime.ReadMemStats(&after)
	if _, raised := got.(ctaRaised); !raised {
		t.Fatalf("ctaPromote(0E-99999999999 → xs:string) = %#v, want ctaRaised", got)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated >= ctaPromoteAllocBound {
		t.Errorf("ctaPromote(0E-99999999999 → xs:string) allocated %d bytes, want below %d", allocated, ctaPromoteAllocBound)
	}
}

// TestEvaluateAnyURIComparisons pins B.1's URI promotion at the place it is
// applied — ctaCompare.eval's collation route — and the two answers that
// route decides.
//
// The equality row is finding 2's witness: clause 2.4 casts the
// xs:untypedAtomic operand to xs:anyURI, whose whiteSpace facet COLLAPSES the
// surrounding space, so the two operands are the same xs:anyURI value.
// Promoting the comparison type to xs:string instead would cast @u to
// xs:string, whose whiteSpace facet is preserve, and decide false.
//
// The ordering row needs the collation route to decide at all: xs:anyURI
// values are deliberately not value.Ordered (§3.3.17.3, ordered=false), so a
// comparison type of xs:anyURI reaching holdsBetween raises err:XPTY0004 and
// the {test} is false whichever way the two URIs sort.
func TestEvaluateAnyURIComparisons(t *testing.T) {
	attrs := ctaAttrs(at("u", " http://a "), at("v", "http://a"), at("w", "http://b"))
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"@u = @v cast as xs:anyURI", true},
		{"@u != @v cast as xs:anyURI", false},
		{"@u = @w cast as xs:anyURI", false},
		{"@v cast as xs:anyURI < @w cast as xs:anyURI", true},
		{"@w cast as xs:anyURI < @v cast as xs:anyURI", false},
		{"@v cast as xs:anyURI <= @v cast as xs:anyURI", true},
		{"@w cast as xs:anyURI > @v cast as xs:anyURI", true},
		{"@v cast as xs:anyURI >= @w cast as xs:anyURI", false},
	} {
		got := compile(t, tc.expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestEvaluateFloatComparisons pins that the withhold #889 owns did not take
// the xs:float pairs B.1 rule 1.2 and the identity arm DO reach: an xs:float
// operand still compares against an xs:decimal literal and against another
// xs:float operand, and both decide.
func TestEvaluateFloatComparisons(t *testing.T) {
	attrs := ctaAttrs(at("f", "0.5"), at("g", "0.25"))
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"@f cast as xs:float = 0.5", true},
		{"@f cast as xs:float = 0.25", false},
		{"@f cast as xs:float > 0.25", true},
		{"@f cast as xs:float > @g cast as xs:float", true},
		{"@f cast as xs:float = @g cast as xs:float", false},
	} {
		got := compile(t, tc.expr, "xs", xsd.XMLSchemaNS).Evaluate(backend(), seededTypes, attrs)
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestScanNCNameIsTheXMLNameClass pins the NCName scan against XML's
// NameStartChar and NameChar — the classes Datatypes §G.4.2.5 defines \i and \c
// to be — and not against Unicode's letter and digit categories. The four rows
// written as code-point escapes are the boundaries the two classes draw
// differently, and each is a TOKEN boundary: what the scan stops before is what
// the tokenizer reads next.
func TestScanNCNameIsTheXMLNameClass(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want int
		why  string
	}{
		{"kind", 4, "an ASCII name is the whole of it"},
		{"a.b-c_d", 7, "'.', '-' and '_' continue a name"},
		{"_x", 2, "'_' opens one"},
		{"élan", 5, "U+00E9 is a NameStartChar and a letter both"},
		{"日本", 6, "and so is U+65E5"},
		{"9x", 0, "a digit opens no name"},
		{".x", 0, "nor does '.'"},
		{"", 0, "nor does the empty string"},
		{"p:local", 1, `':' is subtracted from both classes: [\i-[:]][\c-[:]]*`},
		{"\u00b5x", 0, "U+00B5 MICRO SIGN is a Unicode letter and NO NameStartChar"},
		{"a\u00aa", 1, "U+00AA is a Unicode letter and NO NameChar"},
		{"a\u00b7b", 4, "U+00B7 MIDDLE DOT is a NameChar and neither letter nor digit"},
		{"a\u0301b", 4, "and so is U+0301 COMBINING ACUTE ACCENT"},
	} {
		if got := ctaScanNCName(tc.s, 0); got != tc.want {
			t.Errorf("ctaScanNCName(%q, 0) = %d, want %d (%s)", tc.s, got, tc.want, tc.why)
		}
	}
}

// TestCTANCNamePatternPinned fails when ctaNCNamePattern stops being xs:NCName's
// pattern facet as the generated builtin row carries it (Datatypes §3.4.7.1),
// behind the '^' the FO prefix scan adds.
func TestCTANCNamePatternPinned(t *testing.T) {
	if want := "^" + builtin.NCNamePattern(); ctaNCNamePattern != want {
		t.Errorf("ctaNCNamePattern = %q, want %q (\"^\" + builtin.NCNamePattern())", ctaNCNamePattern, want)
	}
}

// TestCompileFollowsTheXMLNameClass is the same boundary reaching the parser:
// an AttrName the class does not admit is no [17] ta-AttrName at all, and one
// it does admit is a single name token however Unicode's categories read it.
func TestCompileFollowsTheXMLNameClass(t *testing.T) {
	if _, ok := CompileCTATest(ctaExprRecord("@\u00b5 = 'x'", ""), seededTypes); ok {
		t.Error(`CompileCTATest("@µ = 'x'"): compiled, want declined — U+00B5 opens no NCName`)
	}
	attrs := ctaAttrs(at("a\u00b7b", "x"))
	if got := compile(t, "@a\u00b7b = 'x'").Evaluate(backend(), seededTypes, attrs); !got {
		t.Error(`Evaluate("@a·b = 'x'") = false, want true — U+00B7 continues the name`)
	}
}
