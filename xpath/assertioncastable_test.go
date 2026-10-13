package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// `$value castable as xs:double` over an xs:string `$value` is true exactly
// where `$value cast as xs:double` succeeds (xpath20.md §3.10.3): over "23.5",
// " 10e4 " (xs:double's whiteSpace collapse comes first, F&O §17.1.1) and
// "INF", and false over "abc" and "10f4", whose cast raises err:FORG0001 —
// never an error, so fn:not over it holds. The unprefixed `double` resolves in
// the {default namespace}, as assert-simple003's xpathDefaultNamespace puts
// it. Every row declines with ctaParser.castExpr's `castable` arm removed.
func TestAssertionCastableOverValue(t *testing.T) {
	str := asBuiltin(t, "string")
	content := xsd.SimpleContent{SimpleType: str}
	for _, tc := range []struct {
		expr, defaultNS, lexical string
		want                     bool
	}{
		{"$value castable as xs:double", "", "23.5", true},
		{"$value castable as xs:double", "", " 10e4 ", true},
		{"$value castable as xs:double", "", "INF", true},
		{"$value castable as xs:double", "", "abc", false},
		{"$value castable as xs:double", "", "10f4", false},
		{"not($value castable as xs:double)", "", "abc", true},
		{"not($value castable as xs:double)", "", "23.5", false},
		{"$value castable as double", xsd.XMLSchemaNS, "10e4", true},
		{"$value castable as double", xsd.XMLSchemaNS, "10f4", false},
		{"$value castable as xs:double and $value castable as xs:integer", "", "5", true},
		{"$value castable as xs:double and $value castable as xs:integer", "", "5.5", false},
		{"$value castable as xs:boolean = true()", "", "1", true},
		{"$value castable as xs:boolean = true()", "", "2", false},
	} {
		test, ok := CompileAssertionTest(ctaExprRecord(tc.expr, tc.defaultNS, "xs", xsd.XMLSchemaNS), seededTypes, content, asUses(t, nil), asNoElems)
		if !ok {
			t.Errorf("CompileAssertionTest(%q): declined, want compiled", tc.expr)
			continue
		}
		if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, nil, asBind(t, str, tc.lexical), time.Time{}); got != tc.want {
			t.Errorf("Evaluate(%q) over %q = %v, want %v", tc.expr, tc.lexical, got, tc.want)
		}
	}
}

// `@a castable as xs:date` over an attribute whose type is ·special·, read as
// xs:untypedAtomic, validates its string as an xs:date lexical (F&O §17.1.1):
// true over 2008-01-01 and " 2008-01-01 ", false over 2008-13-01. An ABSENT @a
// is the empty sequence, which xpath20.md §3.10.2 rule 3 casts only under `?`:
// false under xs:date, true under xs:date?. Two attributes a wildcard would
// select are out of reach here (ctaAssertionFacade declines a wildcard), so the
// sequence of two is `$value`'s below. Every row declines with
// ctaParser.castExpr's `castable` arm removed; the absent row under `?` is
// false with castableTail forcing its cast's allowsEmpty false, and the
// absent rows without `?` flip with it forced true.
func TestAssertionCastableOverAnUntypedAttribute(t *testing.T) {
	uses := asUses(t, map[string]string{"a": "anySimpleType"})
	at := func(lexical string) []asTyped { return []asTyped{{uq("a"), "anySimpleType", lexical}} }
	for _, tc := range []struct {
		expr  string
		attrs []asTyped
		want  bool
	}{
		{"@a castable as xs:date", at("2008-01-01"), true},
		{"@a castable as xs:date", at(" 2008-01-01 "), true},
		{"@a castable as xs:date", at("2008-13-01"), false},
		{"not(@a castable as xs:date)", at("2008-13-01"), true},
		{"@a castable as xs:date", nil, false},
		{"not(@a castable as xs:date)", nil, true},
		{"@a castable as xs:date?", nil, true},
		{"@a castable as xs:date?", at("2008-13-01"), false},
	} {
		got := asCompile(t, tc.expr, uses).Evaluate(backend(), seededTypes, asElem, asValues(t, tc.attrs...), asNoChildren, nil, ValueBinding{}, time.Time{})
		if got != tc.want {
			t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.attrs, got, tc.want)
		}
	}
}

// A `$value` of two or more items is not castable — the cast raises
// err:XPTY0004 (xpath20.md §3.10.2 rule 3) — so `castable` is false and fn:not
// over it true, never an error; one item is castable and the empty list is
// under `?` alone.
func TestAssertionCastableOverAListedValue(t *testing.T) {
	list := asList(t, "IntList", ctaBuiltin("int"))
	content := xsd.SimpleContent{SimpleType: list}
	types := asTypesWith(list)
	for _, tc := range []struct {
		expr, lexical string
		want          bool
	}{
		{"$value castable as xs:int", "1", true},
		{"$value castable as xs:int", "1 2", false},
		{"not($value castable as xs:int)", "1 2", true},
		{"$value castable as xs:int", "", false},
		{"$value castable as xs:int?", "", true},
	} {
		test, ok := CompileAssertionTest(asRecord(tc.expr), types, content, asUses(t, nil), asNoElems)
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

// An assertions facet decides `$value castable as T` over the value, and an
// error EVALUATING the operand is the castable expression's (xpath20.md
// §3.10.3: "If evaluation of E fails with a dynamic error, the castable
// expression as a whole fails"): `. castable as xs:date` reads the absent
// context item, err:XPDY0002, and fails the facet, and so does fn:not over it,
// which a castable answering false would make hold. The fn:not row holds with
// ctaCastableItem's ctaSequenceLength check removed; the `$value` rows decline
// with ctaFacetFacade.castable answering false.
func TestFacetAssertionsCastable(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, tc := range []struct {
		test, lexical string
		want          value.AssertionOutcome
	}{
		{"$value castable as xs:integer", "12", value.AssertionHolds},
		{"$value castable as xs:integer", "1.5", value.AssertionFails},
		{"not($value castable as xs:integer)", "1.5", value.AssertionHolds},
		{". castable as xs:date", "2008-01-01", value.AssertionFails},
		{"not(. castable as xs:date)", "2008-01-01", value.AssertionFails},
		{"@a castable as xs:date?", "2008-01-01", value.AssertionFails},
	} {
		got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(tc.test, "", "xs", xsd.XMLSchemaNS), fcValue(t, str, tc.lexical))
		if got != tc.want {
			t.Errorf("Evaluate(%q, %q) = %d, want %d", tc.test, tc.lexical, got, tc.want)
		}
	}
}

// Guard: what `castable as` does not admit declines rather than answering. A
// Type Alternative's {test} declines it — §3.12.6's [15] ta-CastExpr has no
// `castable` tail. In an assertion, a cast castsFrom or castTarget declines
// for `cast as` declines for `castable as` too: xs:integer from an xs:decimal
// @d (F&O §17.1.3.4's truncation), xs:QName from an untyped @a (§17.1's
// err:XPTY0004), xs:NOTATION (err:XPST0080), and a target that names no type.
// So does a `castable` tail over a cast, and one inside a value predicate.
func TestCastableDeclines(t *testing.T) {
	for _, expr := range []string{"@a castable as xs:date", "not(@a castable as xs:date)", "(@a castable as xs:date)"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, "", "xs", xsd.XMLSchemaNS), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
	uses := asUses(t, map[string]string{"d": "decimal", "a": "anySimpleType"})
	for _, expr := range []string{
		"@d cast as xs:integer = 1",
		"@d castable as xs:integer",
		"@a castable as xs:QName",
		"@a castable as xs:NOTATION",
		"@a castable as xs:nonesuch",
		"@a cast as xs:string castable as xs:date",
		"xs:string(@a) castable as xs:date",
		"count(n[. castable as xs:int = . castable as xs:int]) = 1",
	} {
		if _, ok := CompileAssertionTest(asRecord(expr), seededTypes, xsd.EmptyContent{}, uses, asChildTypes(t)); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
}
