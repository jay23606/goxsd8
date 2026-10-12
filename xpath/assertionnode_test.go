package xpath

import (
	"slices"
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive the two functions that take the context item `.`
// as E's NODE rather than its atom: fn:namespace-uri (ctaNamespaceURI) and
// fn:in-scope-prefixes as an operand of `=` (ctaPrefixMember), each reading
// the [ContextElement] Evaluate is handed.

// anEval compiles expr for an E with empty content, or fails the test, and
// evaluates it over e.
func anEval(t *testing.T, expr string, e ContextElement) bool {
	t.Helper()
	test := asCompile(t, expr, asUses(t, nil))
	return test.Evaluate(backend(), seededTypes, e, asValues(t), asNoChildren, nil, ValueBinding{}, time.Time{})
}

// fn:namespace-uri over `.`, or with no argument, is E's [namespace name] as
// ONE xs:anyURI (xpath-functions.md §14.3): `namespace-uri(.) = 'urn:x'`
// holds on an E in urn:x and is false on one in no namespace, whose result is
// the zero-length xs:anyURI — equal to "", one item for fn:exists, false as an
// ·effective boolean value· — never the empty sequence. An xs:anyURI
// compares with an xs:string by B.1's promotion, in a value comparison too.
//
// With libraryCall's namespace-uri arm removed the first row declines. With
// ctaNamespaceURI's arm removed from ctaCarriedType, the exists, count and
// instance-of rows answer false; from ctaEffectiveBoolean.eval, the bare row
// over urn:x answers false; from ctaTypes.instanceItem, the instance-of row
// declines; with the call reading E's local name, both `= 'urn:x'` rows over
// urn:x, `= ""` over no namespace, both bare rows over no namespace, the `eq`,
// sequence and fn:starts-with rows fail.
func TestAssertionNamespaceURIIsTheNamespaceName(t *testing.T) {
	inX := asElement{name: xsd.QName{Space: "urn:x", Local: "e"}}
	inNone := asElement{name: uq("e")}
	for _, tc := range []struct {
		expr string
		e    asElement
		want bool
	}{
		{"namespace-uri(.) = 'urn:x'", inX, true},
		{"namespace-uri(.) = 'urn:x'", inNone, false},
		{"namespace-uri() = 'urn:x'", inX, true},
		{"namespace-uri() = 'urn:x'", inNone, false},
		{"namespace-uri(.) = ''", inNone, true},
		{"namespace-uri(.) = ''", inX, false},
		{"exists(namespace-uri(.))", inNone, true},
		{"count(namespace-uri()) = 1", inNone, true},
		{"namespace-uri(.)", inX, true},
		{"namespace-uri(.)", inNone, false},
		{"not(namespace-uri(.))", inNone, true},
		{"namespace-uri(.) instance of xs:anyURI", inNone, true},
		{"namespace-uri(.) eq 'urn:x'", inX, true},
		{"namespace-uri(.) = ('urn:y', 'urn:x')", inX, true},
		{"starts-with(namespace-uri(.), 'urn:')", inX, true},
	} {
		if got := anEval(t, tc.expr, tc.e); got != tc.want {
			t.Errorf("Evaluate(%q) over %s = %v, want %v", tc.expr, tc.e.name, got, tc.want)
		}
	}
}

// fn:in-scope-prefixes over `.`, as an operand of `=` against string literals
// on either side, holds where some literal is a prefix of E's [in-scope
// namespaces] (xpath-functions.md §11.2.6; xml-infoset.md:169): `a` where E
// binds it, inherited bindings being E's own; `xml` always, whatever E binds;
// the zero-length string exactly where a default namespace is in scope — not
// for an E with none, nor for one whose `xmlns=""` the adapter answers ok
// for; no other prefix whose XML 1.1 `xmlns:p=""` undeclaration the adapter
// answers ok for either; never `xmlns`, nor a string that is no NCName,
// whatever the adapter would answer, and neither is asked of it.
//
// With ctaEval's ctaPrefixMember arm removed every row answering true fails,
// and so does the fn:not row; with booleanExpr not reading prefixMember the
// first row declines; with the literal-left form unread, the first such row
// declines, and with the parenthesized sequence unread, the first sequence
// row; with ctaInScopePrefix's "xml" arm removed the two xml rows fail; with
// it reading ok instead of the uri, the `= ""` rows over no default namespace
// and over `xmlns=""` and the `= 'p'` row over `xmlns:p=""` fail.
func TestAssertionInScopePrefixesAreEsBindings(t *testing.T) {
	bindsA := asElement{name: uq("x"), bindings: map[string]string{"": "http://www.example.org", "a": "http://test"}}
	bindsNone := asElement{name: uq("x")}
	undeclared := asElement{name: uq("x"), bindings: map[string]string{"": ""}}
	undeclaredP := asElement{name: uq("x"), bindings: map[string]string{"p": ""}}
	for _, tc := range []struct {
		expr string
		e    asElement
		want bool
	}{
		{"in-scope-prefixes(.) = 'a'", bindsA, true},
		{"in-scope-prefixes(.) = 'a'", bindsNone, false},
		{"'a' = in-scope-prefixes(.)", bindsA, true},
		{"'a' = in-scope-prefixes(.)", bindsNone, false},
		{"in-scope-prefixes(.) = ('b', 'a')", bindsA, true},
		{"('b', 'a') = in-scope-prefixes(.)", bindsA, true},
		{"in-scope-prefixes(.) = ('b', 'c')", bindsA, false},
		{"in-scope-prefixes(.) = 'xml'", bindsNone, true},
		{"in-scope-prefixes(.) = 'xml'", bindsA, true},
		{"in-scope-prefixes(.) = ''", bindsA, true},
		{"in-scope-prefixes(.) = ''", bindsNone, false},
		{"in-scope-prefixes(.) = ''", undeclared, false},
		{"in-scope-prefixes(.) = 'p'", undeclaredP, false},
		{"not(in-scope-prefixes(.) = 'a')", bindsA, false},
		{"in-scope-prefixes(.) = 'a' and namespace-uri(.) = ''", bindsA, true},
		{"(in-scope-prefixes(.) = 'a')", bindsA, true},
	} {
		if got := anEval(t, tc.expr, tc.e); got != tc.want {
			t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.e.bindings, got, tc.want)
		}
	}
}

// `xmlns` and a string that is no NCName are never members of
// fn:in-scope-prefixes, and the adapter is asked about neither, nor about
// `xml`, which is answered without it: over an adapter that binds every one
// of them, the rows below ask it about exactly `b` and the empty prefix, in
// written order, and the first literal that is a member ends the walk. With
// ctaInScopePrefix's xmlns-and-NCName arm removed the `xmlns`, `p:q` and ` a`
// rows hold and the adapter is asked about each; with its "xml" arm removed
// the adapter is asked about `xml`.
func TestAssertionInScopePrefixesAskNoImpossiblePrefix(t *testing.T) {
	var asked []string
	permissive := asElement{name: uq("x"), bindings: map[string]string{"xmlns": "u", "p:q": "u", " a": "u", "xml": "u", "": "u"}, asked: &asked}
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"in-scope-prefixes(.) = 'xmlns'", false},
		{"in-scope-prefixes(.) = 'p:q'", false},
		{"in-scope-prefixes(.) = ' a'", false},
		{"in-scope-prefixes(.) = ('xml', 'b')", true},
		{"in-scope-prefixes(.) = ('xmlns', 'b', '', 'c')", true},
	} {
		if got := anEval(t, tc.expr, permissive); got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
	if want := []string{"b", ""}; !slices.Equal(asked, want) {
		t.Errorf("the adapter was asked about %q, want %q", asked, want)
	}
}

// Neither function reads E's string value: `.` as their argument is a node,
// compiled through ctaFacade.contextNode and never contextItem, so
// ReadsContextItem stays false for a {test} whose only `.` is such an
// argument, and true once `.` is atomized beside it. Both compile under a nil
// {content type}, where `.` as an atom declines: E's name and bindings do not
// depend on its governing type. With contextNodeArgument compiling `.` through
// contextItem too, the three rows wanting false report true and both
// nil-content rows decline; with ctaAssertionFacade.contextNode declining under
// a nil content, both nil-content rows decline.
func TestAssertionNodeArgumentsReadNoStringValue(t *testing.T) {
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"namespace-uri(.) = 'urn:x'", false},
		{"in-scope-prefixes(.) = 'a'", false},
		{"namespace-uri() = 'urn:x'", false},
		{"namespace-uri(.) = .", true},
	} {
		test := asCompile(t, tc.expr, asUses(t, nil))
		if got := test.ReadsContextItem(); got != tc.want {
			t.Errorf("CompileAssertionTest(%q).ReadsContextItem() = %v, want %v", tc.expr, got, tc.want)
		}
	}
	for _, expr := range []string{"namespace-uri(.) = 'urn:x'", "in-scope-prefixes(.) = 'a'"} {
		test, ok := CompileAssertionTest(asRecord(expr), seededTypes, nil, asUses(t, nil), asNoElems)
		if !ok {
			t.Errorf("CompileAssertionTest(%q) under a nil content: declined, want compiled", expr)
			continue
		}
		if !test.Evaluate(backend(), seededTypes, asElement{name: xsd.QName{Space: "urn:x", Local: "e"}, bindings: map[string]string{"a": "u"}}, asValues(t), asNoChildren, nil, ValueBinding{}, time.Time{}) {
			t.Errorf("Evaluate(%q) under a nil content = false, want true", expr)
		}
	}
	if _, ok := CompileAssertionTest(asRecord(". = 'x'"), seededTypes, nil, asUses(t, nil), asNoElems); ok {
		t.Errorf("CompileAssertionTest(\". = 'x'\") under a nil content: compiled, want declined")
	}
}

// GUARD: every other shape of either function still declines under
// CompileAssertionTest's GAP(xpath) (#1042) — an argument but `.` (a path, a
// variable, `()`), fn:in-scope-prefixes outside `=` against string literals,
// and fn:string over fn:namespace-uri, whose xs:anyURI castsFrom declines as
// it does a typed xs:anyURI attribute's — and both decline in a Type
// Alternative's {test} and inside a value predicate. These rows hold without
// the change too. With ctaNamespaceURI's arm removed from ctaCarriedType, which
// ctaTypes.castSource reads, the `string(namespace-uri(.))` row compiles, and
// with equalsAt reading every general comparator as `=` the `!=` row does.
func TestAssertionNodeFunctionsDeclineOtherShapes(t *testing.T) {
	uses := asUses(t, map[string]string{"a": "string"})
	elems := asElems(map[xsd.QName]xsd.TypeDefinition{uq("c"): asBuiltin(t, "string")})
	for _, expr := range []string{
		"namespace-uri(@a) = ''",
		"namespace-uri(c) = ''",
		"namespace-uri($value) = ''",
		"namespace-uri(()) = ''",
		"namespace-uri((.)) = ''",
		"namespace-uri(., .) = ''",
		"string(namespace-uri(.)) = ''",
		"in-scope-prefixes(.)",
		"exists(in-scope-prefixes(.))",
		"count(in-scope-prefixes(.)) = 1",
		"in-scope-prefixes(.) != 'a'",
		"in-scope-prefixes(.) eq 'a'",
		"in-scope-prefixes(.) = 1",
		"in-scope-prefixes(.) = @a",
		"in-scope-prefixes(.) = in-scope-prefixes(.)",
		"in-scope-prefixes(.) = 'a' cast as xs:string",
		"in-scope-prefixes() = 'a'",
		"in-scope-prefixes(@a) = 'a'",
		"in-scope-prefixes(c) = 'a'",
		"count(c[namespace-uri(.) = '']) = 1",
		"count(c[in-scope-prefixes(.) = 'a']) = 1",
	} {
		if _, ok := CompileAssertionTest(asRecord(expr), seededTypes, xsd.SimpleContent{SimpleType: asBuiltin(t, "string")}, uses, elems); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
	for _, expr := range []string{"namespace-uri(.) = ''", "namespace-uri() = ''", "in-scope-prefixes(.) = 'a'"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
}

// An assertions facet has no context item (cvc-assertions-valid clause 1.2),
// so either function over `.` raises err:XPDY0002 and fails the facet — never
// declined, and never the answer a fabricated element would give, which for
// `namespace-uri() = ""` and `in-scope-prefixes(.) = 'xml'` holds. With
// ctaContextElementOf's ctaAbsentNode arm reading the evaluation's element,
// the first row reads the facet's absent element and panics; with
// ctaFacetFacade.contextNode declining, every row is declined.
func TestFacetNodeFunctionsRaise(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, expr := range []string{
		"namespace-uri(.) = ''",
		"namespace-uri() = ''",
		"not(namespace-uri() = 'x')",
		"in-scope-prefixes(.) = 'xml'",
		"not(in-scope-prefixes(.) = 'a')",
	} {
		if got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(expr, ""), fcValue(t, str, "x")); got != value.AssertionFails {
			t.Errorf("FacetAssertions().Evaluate(%q) = %d, want Fails (%d)", expr, got, value.AssertionFails)
		}
	}
}
