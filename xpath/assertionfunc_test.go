package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive the F&O string and sequence functions the assertion
// and facet façades call (ctaFacade.callsLibrary, ctafunc.go) and a Type
// Alternative's {test} declines.

// afUses is the attribute types every function row reads: s an xs:string, k an
// xs:token, u an xs:anyURI, i an xs:integer, and w an xs:anySimpleType, whose
// typed value is xs:untypedAtomic.
func afUses(t *testing.T) AttributeTypes {
	t.Helper()
	return asUses(t, map[string]string{"s": "string", "k": "token", "u": "anyURI", "i": "integer", "w": "anySimpleType"})
}

// afCompile compiles expr for an E whose {content type} is content over afUses
// and asChildTypes, reporting whether it compiled.
func afCompile(t *testing.T, expr string, content xsd.ContentType) (AssertionTest, bool) {
	t.Helper()
	return CompileAssertionTest(asRecord(expr), seededTypes, content, afUses(t), asChildTypes(t))
}

// afEval compiles expr for an E with empty content, or fails the test, and
// evaluates it over attrs and children.
func afEval(t *testing.T, expr string, attrs []asTyped, children ...asChild) bool {
	t.Helper()
	test, ok := afCompile(t, expr, xsd.EmptyContent{})
	if !ok {
		t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
	}
	return test.Evaluate(backend(), seededTypes, asElem, asValues(t, attrs...), asChildren(t, children...), nil, ValueBinding{}, time.Time{})
}

// afEvalValue compiles expr for an E with simple content of type st, or fails
// the test, and evaluates it with `$value` bound to lexical under st.
func afEvalValue(t *testing.T, expr string, st *xsd.SimpleType, lexical string) bool {
	t.Helper()
	test, ok := afCompile(t, expr, xsd.SimpleContent{SimpleType: st})
	if !ok {
		t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
	}
	return test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, nil, asBind(t, st, lexical), time.Time{})
}

// Each string function decides both ways over an attribute, `$value` and a
// child (xpath-functions.md §7.4.4, §7.4.5, §7.5.1–7.5.3, §2.3), comparing
// code points under the default collation (xpath-valid clause 2.2.10):
// fn:contains is case-sensitive, fn:string-length counts the 11 code points of
// a 13-byte string, and fn:normalize-space folds XML's S alone — a no-break
// space survives it. An xs:token and an xs:anyURI attribute are xs:string?
// arguments by subtype substitution and URI promotion — the xs:token read as
// its typed value, whose whiteSpace is collapsed — and an xs:untypedAtomic one
// is cast to xs:string (xpath20.md §3.1.5). Every row is declined at
// CompileAssertionTest, and fails, with ctaFacade.callsLibrary false on the
// assertion façade.
func TestAssertionStringFunctions(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, tc := range []struct {
		expr  string
		attrs []asTyped
		want  bool
	}{
		{"string-length(@s) le 10", []asTyped{{uq("s"), "string", "short"}}, true},
		{"string-length(@s) le 10", []asTyped{{uq("s"), "string", "abcdefghijk"}}, false},
		{"string-length(@s) eq 11", []asTyped{{uq("s"), "string", "héllo wörld"}}, true},
		{"starts-with(@s, 'p')", []asTyped{{uq("s"), "string", "prefix"}}, true},
		{"starts-with(@s, 'p')", []asTyped{{uq("s"), "string", "suffix"}}, false},
		{"starts-with(@k, 'a b')", []asTyped{{uq("k"), "token", "  a   b "}}, true},
		{"starts-with(@k, 'a b')", []asTyped{{uq("k"), "token", "b a"}}, false},
		{"starts-with(@u, 'http:')", []asTyped{{uq("u"), "anyURI", "http://a"}}, true},
		{"ends-with(@w, 'z')", []asTyped{{uq("w"), "anySimpleType", "xyz"}}, true},
		{"ends-with(@w, 'z')", []asTyped{{uq("w"), "anySimpleType", "zyx"}}, false},
	} {
		if got := afEval(t, tc.expr, tc.attrs); got != tc.want {
			t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.attrs, got, tc.want)
		}
	}
	for _, tc := range []struct {
		expr, lexical string
		want          bool
	}{
		{"ends-with($value, 'xyz')", "abcxyz", true},
		{"ends-with($value, 'xyz')", "xyzabc", false},
		{"contains($value, 'E')", "an ELEMENT", true},
		{"contains($value, 'E')", "an element", false},
		{"normalize-space($value) = '100'", " \t100\n ", true},
		{"normalize-space($value) = '1 00'", "1 \r\n 00", true},
		{"normalize-space($value) = '100'", " 1 00 ", false},
		{"normalize-space($value) = '100'", " 100", false},
	} {
		if got := afEvalValue(t, tc.expr, str, tc.lexical); got != tc.want {
			t.Errorf("Evaluate(%q) over $value %q = %v, want %v", tc.expr, tc.lexical, got, tc.want)
		}
	}
	for _, tc := range []struct {
		lexical string
		want    bool
	}{
		{"hello", true},
		{"bye", false},
	} {
		got := afEval(t, "string(e1) = 'hello'", nil, asChild{uq("e1"), "string", tc.lexical, false})
		if got != tc.want {
			t.Errorf("Evaluate(string(e1) = 'hello') over e1 %q = %v, want %v", tc.lexical, got, tc.want)
		}
	}
}

// fn:empty and fn:exists test their `item()*` argument for the empty sequence
// without atomizing it (§15.1.4, §15.1.5): an attribute present or absent, a
// cast of a child present or absent, `$value` over non-simple content — which
// assert019 reads — and over simple content bound and unbound. A rooted path
// raises err:XPDY0050 under either, which fn:not propagates. fn:true() and
// fn:false() compile, and are constants whatever E carries. Every row is
// declined at CompileAssertionTest, and fails, with ctaFacade.callsLibrary
// false on the assertion façade.
func TestAssertionPresenceFunctions(t *testing.T) {
	s := []asTyped{{uq("s"), "string", "x"}}
	for _, tc := range []struct {
		expr     string
		attrs    []asTyped
		children []asChild
		want     bool
	}{
		{"exists(@s)", s, nil, true},
		{"exists(@s)", nil, nil, false},
		{"empty(@s)", s, nil, false},
		{"empty(@s)", nil, nil, true},
		{"exists(e1 cast as xs:string)", nil, []asChild{{uq("e1"), "string", "", false}}, true},
		{"exists(e1 cast as xs:string)", nil, nil, false},
		{"empty(xs:string(e1))", nil, []asChild{{uq("e1"), "string", "", false}}, false},
		{"exists(/r)", nil, nil, false},
		{"not(exists(/r))", nil, nil, false},
		{"not(empty(//@s))", s, nil, false},
		{"empty($value)", nil, nil, true},
		{"exists($value)", nil, nil, false},
		{"true()", nil, nil, true},
		{"true()", s, nil, true},
		{"false()", nil, nil, false},
		{"false()", s, nil, false},
		{"not(false())", s, nil, true},
	} {
		if got := afEval(t, tc.expr, tc.attrs, tc.children...); got != tc.want {
			t.Errorf("Evaluate(%q) over %v, %v = %v, want %v", tc.expr, tc.attrs, tc.children, got, tc.want)
		}
	}
	str := asBuiltin(t, "string")
	test, ok := afCompile(t, "exists($value)", xsd.SimpleContent{SimpleType: str})
	if !ok {
		t.Fatal("CompileAssertionTest(exists($value)) over simple content: declined, want compiled")
	}
	for _, tc := range []struct {
		bound ValueBinding
		want  bool
	}{
		{asBind(t, str, ""), true},
		{ValueBinding{}, false},
	} {
		if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, nil, tc.bound, time.Time{}); got != tc.want {
			t.Errorf("Evaluate(exists($value)) over %v = %v, want %v", tc.bound, got, tc.want)
		}
	}
	special := asBuiltin(t, "anySimpleType")
	untyped, ok := afCompile(t, "exists($value)", xsd.SimpleContent{SimpleType: special})
	if !ok {
		t.Fatal("CompileAssertionTest(exists($value)) over ·special· content: declined, want compiled")
	}
	if !untyped.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, nil, BindValue("", Untyped("")), time.Time{}) {
		t.Error("Evaluate(exists($value)) over an xs:untypedAtomic $value = false, want true")
	}
	if untyped.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, nil, ValueBinding{}, time.Time{}) {
		t.Error("Evaluate(exists($value)) over the unbound xs:untypedAtomic $value = true, want false")
	}
}

// The empty sequence `()` is an argument, and F&O's empty-sequence rules
// apply: fn:string-length(()) is 0 (§7.4.4's own example), a zero-length
// second argument matches everything and is decided before a zero-length
// first one matches nothing (§7.5.1–7.5.3), and fn:normalize-space(()) and
// fn:string(()) are the zero-length string. Every row is declined, and fails,
// with ctaFacade.callsLibrary false; the fn:string row also with
// ctaStringFunctionItem answering the cast's empty sequence as itself.
func TestAssertionEmptySequenceArguments(t *testing.T) {
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"string-length(()) = 0", true},
		{"starts-with((), '')", true},
		{"starts-with((), ())", true},
		{"contains('', ())", true},
		{"starts-with((), 'a')", false},
		{"ends-with('a', ())", true},
		{"normalize-space(()) = ''", true},
		{"string(()) = ''", true},
		{"exists(())", false},
		{"empty(())", true},
		{"string-length(@s) = 0", true},
		{"string-length(@i) = 0", true},
	} {
		if got := afEval(t, tc.expr, nil); got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// An xs:string? argument of any other type is err:XPTY0004 (xpath20.md
// §3.1.5): an xs:integer attribute, an xs:int `$value`, and the xs:integer
// fn:string-length returns. The error makes the assertion false and stays an
// error under fn:not, where a false-by-comparison would not; an ABSENT such
// attribute matches xs:string? all the same (TestAssertionEmptySequenceArguments).
// So does a `$value` of two or more items over a list type, where one item is a
// string. With ctaStringOf ignoring mistyped, the xs:integer rows hold and their
// fn:not rows fail; with it ignoring the item count, the list rows do.
func TestAssertionStringArgumentTypeErrors(t *testing.T) {
	i := []asTyped{{uq("i"), "integer", "5"}}
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"string-length(@i) = 1", false},
		{"not(string-length(@i) = 1)", false},
		{"contains(@i, '5')", false},
		{"not(contains(@i, '5'))", false},
		{"string-length(string-length(@i)) = 1", false},
		{"not(string-length(string-length(@s)) = 1)", false},
	} {
		if got := afEval(t, tc.expr, i); got != tc.want {
			t.Errorf("Evaluate(%q) over i=5 = %v, want %v", tc.expr, got, tc.want)
		}
	}
	intType := asBuiltin(t, "int")
	for _, expr := range []string{"contains($value, '1')", "not(contains($value, '1'))"} {
		if afEvalValue(t, expr, intType, "1") {
			t.Errorf("Evaluate(%q) over an xs:int $value = true, want false (err:XPTY0004)", expr)
		}
	}
	list := asList(t, "StringList", ctaBuiltin("string"))
	types := asTypesWith(list)
	for _, tc := range []struct {
		expr, lexical string
		want          bool
	}{
		{"string-length($value) = 3", "abc", true},
		{"string-length($value) = 3", "abc def", false},
		{"not(string-length($value) = 3)", "abc def", false},
		{"string-length($value) = 0", "", true},
		{"exists($value)", "abc def", true},
	} {
		test, ok := CompileAssertionTest(asRecord(tc.expr), types, xsd.SimpleContent{SimpleType: list}, afUses(t), asNoElems)
		if !ok {
			t.Fatalf("CompileAssertionTest(%q) over a list: declined, want compiled", tc.expr)
		}
		v, err := value.ValidateLexical(backend(), types, list, tc.lexical, nil, FacetAssertions(time.Time{}))
		if err != nil {
			t.Fatalf("mapping %q against the list: %v", tc.lexical, err)
		}
		if got := test.Evaluate(backend(), types, asElem, asValues(t), asNoChildren, nil, BindValue("", Typed(v)), time.Time{}); got != tc.want {
			t.Errorf("Evaluate(%q) over %q = %v, want %v", tc.expr, tc.lexical, got, tc.want)
		}
	}
}

// Each new node's static type, item, ·effective boolean value· and cast
// admission is its own (ctaStaticOf, ctaItemOf, ctaEffectiveBoolean.eval,
// castsFrom). The static rows compare a result against an operand of another
// type, which is err:XPTY0004 — false, and false under fn:not — where reading
// the result as xs:untypedAtomic would cast the other operand and hold; the
// bare rows are each result's effective boolean value; the cast rows decline
// fn:string over a non-string result and admit it over fn:normalize-space's.
// Removing one arm's case from ctaItemOf fails that arm's row of the matching
// table. Removing it from ctaCarriedType, the static type ctaStaticOf and
// castsFrom both read and the type ctaEffectiveBoolean.eval decides a bare
// result in, fails the arm's static and bare rows and, for the three
// non-string results, its item and decline rows; castsFrom's answer for
// ctaStringFunction's xs:string is what it gives an unjudged operand, so no
// row tells that part apart.
func TestStringFunctionArmsInEverySwitch(t *testing.T) {
	s := []asTyped{{uq("s"), "string", "1"}}
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"contains(@s, '1') = 'true'", false},
		{"not(contains(@s, '1') = 'true')", false},
		{"string-length(@s) = '1'", false},
		{"not(string-length(@s) = '1')", false},
		{"exists(@s) = 'true'", false},
		{"not(exists(@s) = 'true')", false},
		{"string(@s) = 1", false},
		{"not(string(@s) = 1)", false},
	} {
		if got := afEval(t, tc.expr, s); got != tc.want {
			t.Errorf("static: Evaluate(%q) over s=1 = %v, want %v", tc.expr, got, tc.want)
		}
	}
	for _, expr := range []string{
		"contains(@s, '1') eq true()", "string-length(@s) eq 1", "normalize-space(@s) eq '1'",
		"exists(@s) eq true()", "string(@s) eq '1'",
	} {
		if !afEval(t, expr, s) {
			t.Errorf("item: Evaluate(%q) over s=1 = false, want true", expr)
		}
	}
	for _, expr := range []string{"contains(@s, '1')", "string-length(@s)", "normalize-space(@s)", "exists(@s)", "string(@s)"} {
		if !afEval(t, expr, s) {
			t.Errorf("effective boolean value: Evaluate(%q) over s=1 = false, want true", expr)
		}
	}
	for _, expr := range []string{"string(contains(@s, '1')) = 'true'", "string(string-length(@s)) = '1'", "string(exists(@s)) = 'true'"} {
		if _, ok := afCompile(t, expr, xsd.EmptyContent{}); ok {
			t.Errorf("castsFrom: CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
	for _, expr := range []string{"string(normalize-space(@s)) = '1'", "string(string(@s)) = '1'"} {
		if !afEval(t, expr, s) {
			t.Errorf("castsFrom: Evaluate(%q) over s=1 = false, want true", expr)
		}
	}
}

// stringArgument and ctaStringOf, through atomicArgument and ctaArgumentAtom,
// each name every ctaStatic arm: the statically empty operand is held as it is
// and not mistyped, an xs:untypedAtomic one is held under its cast to
// xs:string, and ctaStringOf raises for an operand whose static type is
// xs:untypedAtomic, which atomicArgument never stores. Without
// atomicArgument's ctaEmptySequence case the empty row is mistyped, and with
// ctaArgumentAtom's ctaUntypedAtomic case answering the empty sequence the
// raised row fails.
func TestStringArgumentNamesEveryStaticArm(t *testing.T) {
	known := compileTypes(t)
	if got := known.stringArgument(ctaEmptyValue{}); got != (ctaAtomicArgument{operand: ctaEmptyValue{}}) {
		t.Errorf("stringArgument(()) = %+v, want the operand held, not mistyped", got)
	}
	untyped := known.stringArgument(ctaUntypedValue{})
	if _, cast := untyped.operand.(ctaCast); !cast || untyped.mistyped {
		t.Errorf("stringArgument($value untyped) = %+v, want its cast to xs:string, not mistyped", untyped)
	}
	env := ctaEnv{backend: backend(), types: seededTypes}
	if s, ok := ctaStringOf(ctaAtomicArgument{operand: ctaUntypedValue{}}, env); ok {
		t.Errorf("ctaStringOf(xs:untypedAtomic operand) = %q, want raised", s)
	}
}

// A new node reads what its arguments read: a typed child under each is
// reported by ReadsChild — fn:exists over a cast of one reads its value off
// ChildElements — and an fn:count call under one is counted by the Tally, so
// `exists(count(inner))` holds. With an arm's readsChild or counted answering
// nothing for its arguments, that arm's row fails.
func TestStringFunctionsReadTheirArguments(t *testing.T) {
	for _, expr := range []string{"contains('x', e1)", "string-length(e1) = 0", "exists(e1 cast as xs:string)", "string(e1) = ''", "distinct-values(e1) = ''"} {
		test, ok := afCompile(t, expr, xsd.EmptyContent{})
		if !ok {
			t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
		}
		if !test.ReadsChild(uq("e1")) {
			t.Errorf("(%s).ReadsChild(e1) = false, want true", expr)
		}
	}
	for _, expr := range []string{"exists(count(inner))", "contains('1', count(inner))", "string-length(count(inner)) = 1", "string(normalize-space(count(inner))) = ''", "distinct-values(count(inner)) = 1"} {
		if acCompile(t, asRecord(expr)).Tally() == nil {
			t.Errorf("(%s).Tally() = nil, want a counter for inner", expr)
		}
	}
	counted := acCompile(t, asRecord("exists(count(inner))"))
	if !counted.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, acTally(counted, acEl(1, "inner")), ValueBinding{}, time.Time{}) {
		t.Error("Evaluate(exists(count(inner))) = false, want true")
	}
}

// fn:concat casts each argument to xs:string and reads the empty sequence as
// the zero-length string (xpath-functions.md §7.4.1): `concat(@s, 'x')` is
// "ax" over s="a" and "x" over an absent @s, an xs:untypedAtomic @w is cast
// like any other, and `()` is an argument. A constructor function over the
// call casts its xs:string (§5, §17.1.1), so `xs:date(concat('2008-01-0',
// '1'))` is that date, and one over an fn:count call casts its xs:integer, the
// identity cast castsFrom's ancestor rule admits. Without libraryCall's concat
// arm the first row declines at CompileAssertionTest, which stops the test;
// with constructorFunction parsing simpleValue alone the xs:date row declines,
// and with that row removed the fn:count one does.
func TestAssertionConcat(t *testing.T) {
	a := []asTyped{{uq("s"), "string", "a"}}
	for _, tc := range []struct {
		expr  string
		attrs []asTyped
		want  bool
	}{
		{"concat(@s, 'x') = 'ax'", a, true},
		{"concat(@s, 'x') = 'x'", a, false},
		{"concat(@s, 'x') = 'x'", nil, true},
		{"concat(@s, 'x') = 'ax'", nil, false},
		{"concat('<', @w, '>', ()) = '<v>'", []asTyped{{uq("w"), "anySimpleType", "v"}}, true},
		{"xs:date(concat('2008-01-0', '1')) eq xs:date('2008-01-01')", nil, true},
	} {
		if got := afEval(t, tc.expr, tc.attrs); got != tc.want {
			t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.attrs, got, tc.want)
		}
	}
	counted := acCompile(t, asRecord("xs:integer(count(inner)) eq 1"))
	if !counted.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, acTally(counted, acEl(1, "inner")), ValueBinding{}, time.Time{}) {
		t.Error("Evaluate(xs:integer(count(inner)) eq 1) over one inner = false, want true")
	}
}

// ctaConcat answers in each switch over the ctaValue sum, its static type
// xs:string. The static row compares the result with a numeric literal, a
// B.2 mismatch, err:XPTY0004 — false — where an xs:untypedAtomic reading
// would cast "1" to the number and hold; the instance row asks its static
// type; the item row reads its string; the bare rows are its ·effective
// boolean value·; the ReadsChild and Tally rows ask what its arguments read.
// With the ctaConcat arm deleted from ctaCarriedType the static, instance and
// `concat('a', 'b')` rows fail; from ctaTypes.instanceItem the instance row
// declines; with ctaEffectiveBoolean.eval's ctaCarriedType read deleted
// `concat('a', 'b')` fails; from ctaItemOf the instance, item and
// `concat('a', 'b')` rows fail; with ctaConcat.readsChild answering false the
// ReadsChild row fails, and with ctaConcat.counted appending nothing the
// Tally row does.
func TestConcatArmsInEverySwitch(t *testing.T) {
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"concat('1', '') = 1", false},
		{"concat('a', 'b') instance of xs:string", true},
		{"concat('a', 'b') eq 'ab'", true},
		{"concat('a', 'b')", true},
		{"concat((), ())", false},
	} {
		if got := afEval(t, tc.expr, nil); got != tc.want {
			t.Errorf("Evaluate(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
	test, ok := afCompile(t, "concat('x', e1) = 'x'", xsd.EmptyContent{})
	if !ok {
		t.Fatal("CompileAssertionTest(concat('x', e1) = 'x'): declined, want compiled")
	}
	if !test.ReadsChild(uq("e1")) {
		t.Error("(concat('x', e1) = 'x').ReadsChild(e1) = false, want true")
	}
	if acCompile(t, asRecord("concat('', normalize-space(count(inner))) = ''")).Tally() == nil {
		t.Error("(concat('', normalize-space(count(inner))) = '').Tally() = nil, want a counter for inner")
	}
}

// What stays declined: a function libraryCall does not name, the three-argument
// collation form of fn:contains and its kin (never read as the two-argument
// form), every other arity — fn:concat with fewer than two arguments among
// them (§7.4.1) — and fn:string, or an fn:concat argument, over a typed node or
// value outside the xs:string family and the date/time primitives (castsFrom),
// or over an xs:float or xs:double, a literal (literalCastsTo) or a cast to
// either over a string-family operand (castsFrom's floatingSource shape)
// included, whose cast to xs:string §17.1.2 does not render canonically — an
// xs:decimal and an xs:boolean literal still compile. The zero-argument string
// forms compile over simple content, whose implicit argument is E's string
// value `.` reads (TestAssertionContextItemIsTheStringValue evaluates them).
// With matchCall admitting three arguments the collation row compiles, with
// literalCastsTo's last return answering true the `string(1.5e0)` row does, and
// with castSource's ctaCast exit answering (nil, false) in place of
// floatingSource the two rows over a cast to xs:float or xs:double do; with
// literalCastsTo's xs:decimal or xs:boolean arm answering false, `string(1.5)`
// or `string(true())` declines.
func TestCompileAssertionTestDeclinesFunctions(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, expr := range []string{
		"upper-case(@s) = 'X'",
		"concat(@s) = 'x'",
		"concat() = ''",
		"concat(@i, 'x') = '5x'",
		"contains(@s, 'x','http://www.w3.org/2005/xpath-functions/collation/codepoint')",
		"starts-with(@s, 'x', 'http://www.w3.org/2005/xpath-functions/collation/codepoint')",
		"contains(@s)",
		"empty()",
		"exists(@s, @s)",
		"true(1)",
		"string(@s, @s) = ''",
		"string-length(@s, @s) = 0",
		"string(@i) = '5'",
		"string(n) = '5'",
		"string(c) = '5'",
		"string($value) = '5'",
		"string(1.5e0) = '1.5'",
		"string(xs:float('1.5')) = '1.5'",
		"string(xs:double(@s)) = '1.5'",
		"exists(a/@b)",
		"exists(a/b, a/b)",
		"contains(@s, ('x'))",
	} {
		if _, ok := afCompile(t, expr, xsd.SimpleContent{SimpleType: asBuiltin(t, "int")}); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
	if _, ok := afCompile(t, "string($value) = 'x'", xsd.SimpleContent{SimpleType: str}); !ok {
		t.Error("CompileAssertionTest(string($value) = 'x') over xs:string content: declined, want compiled")
	}
	for _, expr := range []string{"string(1.5) = '1.5'", "string(true()) = 'true'"} {
		if _, ok := afCompile(t, expr, xsd.SimpleContent{SimpleType: str}); !ok {
			t.Errorf("CompileAssertionTest(%s): declined, want compiled", expr)
		}
	}
}

// A Type Alternative's {test} declines every function libraryCall names — the
// CTA grammar admits fn:not alone (§3.12.6 clause 3) — which CompileCTATest
// itself shows, CTATestStaticError answering nil for a decline and a pass
// alike; `not(@a = 'x')` still compiles. Its constructor function takes a [16]
// ta-SimpleValue alone ([18]), so a constructor over a function call or
// another constructor declines too; `xs:string(@a) = 'x'` still compiles.
// With ctaTypeAlternativeFacade answering callsLibrary true, every declined
// row compiles; with constructorOperand parsing an argument on every façade,
// `xs:string(xs:string(@a)) = 'x'` does.
func TestCompileCTATestDeclinesLibraryFunctions(t *testing.T) {
	for _, expr := range []string{
		"contains(@a, 'x')", "starts-with(@a, 'x')", "ends-with(@a, 'x')",
		"string-length(@a) > 0", "normalize-space(@a) = 'x'", "string(@a) = 'x'",
		"empty(@a)", "exists(@a)", "distinct-values(@a) = 'x'", "true()", "false()", "not(true())",
		"current-date() = current-date()", "concat(@a, 'x') = 'ax'", "year-from-date(@a) eq 2008",
	} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
	for _, expr := range []string{"xs:string(xs:string(@a)) = 'x'", "xs:date(concat(@a, '')) = xs:date('2008-01-01')"} {
		if _, ok := CompileCTATest(asRecord(expr), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
	for _, expr := range []string{"not(@a = 'x')", "xs:string(@a) = 'x'"} {
		if _, ok := CompileCTATest(asRecord(expr), seededTypes); !ok {
			t.Errorf("CompileCTATest(%q): declined, want compiled", expr)
		}
	}
}

// An assertions facet calls the same functions over `$value`
// (cvc-assertions-valid): `ends-with($value, 'xyz')` holds and fails, a list
// `$value` of two items is err:XPTY0004 under an xs:string? parameter and as an
// fn:concat argument, which casts it (§7.4.1), and the zero-argument string
// forms read the absent context item and raise err:XPDY0002 — failing the
// facet, under fn:not too, rather than declining (clause 1.2's Note) — as
// fn:position does as an fn:concat argument. With ctaFacetFacade.callsLibrary
// false every row declines; with ctaConcatItem reading a raising argument as
// the zero-length string, `concat($value, "") = ""` over "a b" and the
// fn:position row hold.
func TestFacetStringFunctions(t *testing.T) {
	str := asBuiltin(t, "string")
	list := asList(t, "StringList", ctaBuiltin("string"))
	types := asTypesWith(list)
	listValue := func(lexical string) value.Value {
		t.Helper()
		v, err := value.ValidateLexical(backend(), types, list, lexical, nil, FacetAssertions(time.Time{}))
		if err != nil {
			t.Fatalf("mapping %q against the list: %v", lexical, err)
		}
		return v
	}
	for _, tc := range []struct {
		test string
		st   *xsd.SimpleType
		v    value.Value
		want value.AssertionOutcome
	}{
		{"ends-with($value, 'xyz')", str, fcValue(t, str, "abcxyz"), value.AssertionHolds},
		{"ends-with($value, 'xyz')", str, fcValue(t, str, "xyzabc"), value.AssertionFails},
		{"not(starts-with($value, '0')) and string-length($value) le 10", str, fcValue(t, str, "123"), value.AssertionHolds},
		{"contains($value, 'a')", list, listValue("a"), value.AssertionHolds},
		{"contains($value, 'a')", list, listValue("a b"), value.AssertionFails},
		{"not(contains($value, 'a'))", list, listValue("a b"), value.AssertionFails},
		{"concat($value, '') = 'a'", list, listValue("a"), value.AssertionHolds},
		{"concat($value, '') = ''", list, listValue("a b"), value.AssertionFails},
		{"not(concat($value, '') = '')", list, listValue("a b"), value.AssertionFails},
		{"concat('x', position()) = 'x'", str, fcValue(t, str, "x"), value.AssertionFails},
		{"string-length() > 0", str, fcValue(t, str, "x"), value.AssertionFails},
		{"not(string-length() > 0)", str, fcValue(t, str, "x"), value.AssertionFails},
		{"normalize-space() = 'x'", str, fcValue(t, str, "x"), value.AssertionFails},
		{"string() = 'x'", str, fcValue(t, str, "x"), value.AssertionFails},
		{"not(string() = 'x')", str, fcValue(t, str, "x"), value.AssertionFails},
		{"exists(@a)", str, fcValue(t, str, "x"), value.AssertionFails},
		{"not(empty(.))", str, fcValue(t, str, "x"), value.AssertionFails},
	} {
		if got := FacetAssertions(time.Time{}).Evaluate(backend(), types, tc.st, ctaExprRecord(tc.test, ""), tc.v); got != tc.want {
			t.Errorf("Evaluate(%q over %s) = %d, want %d", tc.test, tc.st.Name(), got, tc.want)
		}
	}
}
