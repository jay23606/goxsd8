package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// The fixtures below drive a cast from a TYPED operand to its own type or to a
// type it is derived from by restriction — F&O §17.2 case 4's identity cast
// and §17.3's cast up the hierarchy, both of which keep the value — which
// ctaTypes.castsFrom admits, and the casts from a typed operand it still
// declines.

// csUses is the attribute types the cast rows read: x an xs:integer, i an
// xs:int, d an xs:decimal, s an xs:string, start an xs:date and dt an
// xs:dateTime.
func csUses(t *testing.T) AttributeTypes {
	t.Helper()
	return asUses(t, map[string]string{"x": "integer", "i": "int", "d": "decimal", "s": "string", "start": "date", "dt": "dateTime"})
}

// csRestriction is a user-defined atomic type in ctaUserNS restricting base
// with no facet of its own.
func csRestriction(t *testing.T, local string, base xsd.QName) *xsd.SimpleType {
	t.Helper()
	st, err := xsd.NewSimpleType(xsderr.Loc{}, xsd.QName{Space: ctaUserNS, Local: local},
		xsd.RestrictionDerivation{}, xsd.SimpleTypeRef{Name: base}, nil, nil)
	if err != nil {
		t.Fatalf("building %s: %v", local, err)
	}
	return st
}

// A cast whose target is the typed operand's own type, or an ancestor of it,
// compiles and keeps the operand's value, over each typed operand [16]
// ta-SimpleValue — the operand of the cast spelling — admits: an attribute,
// `$value` and a child element, the last also as simple content. `xs:integer(@x)
// eq 5` over an xs:integer @x is the identity cast and `xs:decimal(@i) eq 5`
// over an xs:int @i the cast up the hierarchy; `$value gt xs:date(@start)` is
// assert010's {test}, its @startDate renamed. An ABSENT operand is the empty
// sequence under the constructor spelling, whose `?` the cast spelling without
// one turns into err:XPTY0004: §17.3's "always" succeeds only over an item, so
// the fn:not of the second is false. Every row declines at CompileAssertionTest,
// and so fails, with castsFrom admitting the string family alone.
func TestAssertionCastsToItsOwnTypeOrAnAncestor(t *testing.T) {
	five := func(name, typ string) []asTyped { return []asTyped{{uq(name), typ, "5"}} }
	for _, tc := range []struct {
		expr  string
		attrs []asTyped
		want  bool
	}{
		{"xs:integer(@x) eq 5", five("x", "integer"), true},
		{"xs:integer(@x) eq 6", five("x", "integer"), false},
		{"xs:integer(@x) = 5", []asTyped{{uq("x"), "integer", "+05"}}, true},
		{"xs:decimal(@i) eq 5", five("i", "int"), true},
		{"xs:decimal(@i) gt 5", five("i", "int"), false},
		{"@i cast as xs:integer eq 5", five("i", "int"), true},
		{"@i cast as xs:int? eq 5", five("i", "int"), true},
		{"xs:dateTime(@dt) eq xs:dateTime('2000-01-01T00:00:00')", []asTyped{{uq("dt"), "dateTime", "2000-01-01T00:00:00"}}, true},
		{"xs:integer(@x) eq 5", nil, false},
		{"not(xs:integer(@x) eq 5)", nil, true},
		{"@x cast as xs:integer eq 5", nil, false},
		{"not(@x cast as xs:integer eq 5)", nil, false},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			got := asCompile(t, tc.expr, csUses(t)).Evaluate(backend(), seededTypes, asElem, asValues(t, tc.attrs...), asNoChildren, nil, ValueBinding{}, time.Time{})
			if got != tc.want {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.attrs, got, tc.want)
			}
		})
	}

	date := asBuiltin(t, "date")
	for _, tc := range []struct {
		value, start string
		want         bool
	}{
		{"2010-01-02", "2010-01-01", true},
		{"2010-01-01", "2010-01-02", false},
	} {
		test, ok := CompileAssertionTest(asRecord("$value gt xs:date(@start)"), seededTypes, xsd.SimpleContent{SimpleType: date}, csUses(t), asNoElems)
		if !ok {
			t.Errorf("CompileAssertionTest($value gt xs:date(@start)): declined, want compiled")
			continue
		}
		got := test.Evaluate(backend(), seededTypes, asElem, asValues(t, asTyped{uq("start"), "date", tc.start}), asNoChildren, nil, asBind(t, date, tc.value), time.Time{})
		if got != tc.want {
			t.Errorf("Evaluate($value gt xs:date(@start)) over %s, %s = %v, want %v", tc.value, tc.start, got, tc.want)
		}
	}

	for _, expr := range []string{"xs:decimal(n) eq 5", "xs:decimal(c) eq 5", "xs:int(n) eq 5"} {
		test, ok := CompileAssertionTest(asRecord(expr), seededTypes, xsd.EmptyContent{}, asUses(t, nil), asChildTypes(t))
		if !ok {
			t.Errorf("CompileAssertionTest(%q): declined, want compiled", expr)
			continue
		}
		children := asChildren(t, asChild{uq("n"), "int", "5", false}, asChild{uq("c"), "int", "5", false})
		if !test.Evaluate(backend(), seededTypes, asElem, asValues(t), children, nil, ValueBinding{}, time.Time{}) {
			t.Errorf("Evaluate(%q) over n=5, c=5 = false, want true", expr)
		}
	}
}

// A cast from a listed `$value` casts the ONE item it must be: an xs:int list
// of one item casts it, an empty list is the empty sequence the constructor's
// `?` admits — false under eq, true under fn:not — and a list of two or more
// items is err:XPTY0004 (xpath20.md §3.10.2 rule 3), false and false under
// fn:not, never a cast of its first item. With ctaCastItem casting the first
// item of a longer sequence, the two "1 2" rows without fn:not hold.
func TestAssertionCastsAListedValue(t *testing.T) {
	list := asList(t, "IntList", ctaBuiltin("int"))
	content := xsd.SimpleContent{SimpleType: list}
	types := asTypesWith(list)
	for _, tc := range []struct {
		expr, lexical string
		want          bool
	}{
		{"xs:int($value) eq 1", "1", true},
		{"xs:decimal($value) eq 1", "1", true},
		{"xs:int($value) eq 1", "", false},
		{"not(xs:int($value) eq 1)", "", true},
		{"xs:int($value) eq 1", "1 2", false},
		{"not(xs:int($value) eq 1)", "1 2", false},
		{"xs:int($value) = 1", "1 2", false},
		{"not(xs:int($value) = 1)", "1 2", false},
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

// A cast up the hierarchy keeps the VALUE, so it raises where no rendering of
// the value would: the zero of a user restriction of xs:yearMonthDuration has
// no xs:yearMonthDuration ·canonical representation· (§3.4.26.1 Note) and
// renders only through xs:duration's as PT0S, which xs:yearMonthDuration
// rejects; and a zero of ·scale· 99999999999 under a user restriction of
// xs:precisionDecimal has a canonical form beyond the backend's capacity.
// Comparing the first against an xs:yearMonthDuration converts it the same way
// (subtype substitution). With ctaPromote rendering every value whose type is
// not named as the target's (ctaRepresents answering only from == to), every
// row is false.
func TestAssertionCastUpTheHierarchyRendersNothing(t *testing.T) {
	ym := csRestriction(t, "Months", ctaBuiltin("yearMonthDuration"))
	pd := csRestriction(t, "Precise", ctaBuiltin("precisionDecimal"))
	types := asTypesWith(ym, pd)
	typed := map[xsd.QName]*xsd.SimpleType{uq("ym"): ym, uq("p"): pd}
	uses := func(path []xsd.QName, name xsd.QName) (*xsd.SimpleType, bool) {
		if len(path) != 0 {
			return nil, false
		}
		st, ok := typed[name]
		return st, ok
	}
	lexicals := map[xsd.QName]string{uq("ym"): "P0M", uq("p"): "0E-99999999999"}
	attrs := func(yield func(xsd.QName, TypedValue) bool) {
		for _, name := range []xsd.QName{uq("ym"), uq("p")} {
			v, err := value.ValidateLexical(backend(), types, typed[name], lexicals[name], nil, FacetAssertions(time.Time{}))
			if err != nil {
				t.Fatalf("mapping %q against %s: %v", lexicals[name], typed[name].Name(), err)
			}
			if !yield(name, Typed(v)) {
				return
			}
		}
	}
	for _, expr := range []string{
		"xs:yearMonthDuration(@ym) eq xs:yearMonthDuration('P0M')",
		"@ym eq xs:yearMonthDuration('P0M')",
		"exists(xs:precisionDecimal(@p))",
	} {
		test, ok := CompileAssertionTest(asRecord(expr), types, xsd.EmptyContent{}, uses, asNoElems)
		if !ok {
			t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
		}
		if !test.Evaluate(backend(), types, asElem, attrs, asNoChildren, nil, ValueBinding{}, time.Time{}) {
			t.Errorf("Evaluate(%q) = false, want true", expr)
		}
	}
}

// An assertions facet's `xs:date($value)` over an xs:date `$value` is the
// identity cast, decided rather than declined: Override/over010's facet. It
// declines with castsFrom admitting the string family alone.
func TestFacetAssertionsCastTheValue(t *testing.T) {
	date := asBuiltin(t, "date")
	for _, tc := range []struct {
		lexical string
		want    value.AssertionOutcome
	}{
		{"2010-01-02", value.AssertionHolds},
		{"2009-12-31", value.AssertionFails},
	} {
		got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, date, asRecord("xs:date($value) gt xs:date('2010-01-01')"), fcValue(t, date, tc.lexical))
		if got != tc.want {
			t.Errorf("Evaluate(xs:date($value) gt xs:date('2010-01-01'), %q) = %d, want %d", tc.lexical, got, tc.want)
		}
	}
}

// What stays declined is every cast from a typed operand whose type is not the
// target nor derived from it — §17.4's cast down or across a branch and §17.1's
// and §17.5's casts between primitives — keyed on the operand's type BELOW the
// target: `xs:integer(@d)` over an xs:decimal shares its primitive and is
// related by derivation the other way, and `xs:int(@x)` and `xs:dateTimeStamp(@dt)`
// are casts down. An operand [16] ta-SimpleValue does not admit — an fn:count call,
// an arithmetic or function result, a cast — reaches a constructor function on
// these rules, and fn:string's argument, a cast to xs:string, which nothing outside
// the string family is derived from, and which castsFrom admits from a date/time
// primitive alone besides (TestStringOfATypedDateAttribute). A cast of a cast is
// judged by the inner cast's target (castSource), so fn:string over an xs:decimal
// cast of a typed operand declines, while one over a cast of the string family
// compiles as it did before castSource judged a cast. Each of the first four rows
// compiles with castsFrom keyed on a shared primitive, and with it admitting either
// direction; the fn:string row over `xs:decimal(@d)` compiles with castSource's
// ctaCast arm removed, and the `xs:decimal(@s)` row declines with that arm judging a
// cast of the string family too. The `xs:date(@s)` row does not: castsFrom admits a
// cast from xs:date to xs:string whichever way castSource judges it.
func TestCompileAssertionTestDeclinesCastsDownOrAcross(t *testing.T) {
	for _, tc := range []struct{ expr, why string }{
		{"xs:integer(@d) eq 5", "xs:integer is derived from xs:decimal, not the reverse"},
		{"@d cast as xs:integer eq 5", "the cast spelling of the same cast"},
		{"xs:int(@x) eq 5", "a cast down from xs:integer"},
		{"xs:dateTimeStamp(@dt) eq xs:dateTimeStamp('2000-01-01T00:00:00Z')", "a cast down from xs:dateTime"},
		{"string(xs:decimal(@d)) = '5'", "fn:string over a cast of xs:decimal"},
		{"string(count(inner)) = '2'", "fn:string over an xs:integer count"},
		{"string(@i + @i) = '10'", "fn:string over an xs:integer sum"},
		{"string(contains(@s, '5')) = 'true'", "fn:string over an xs:boolean"},
		{"string(string-length(@s)) = '1'", "fn:string over an xs:integer length"},
		{"xs:string(@i) eq '5'", "a cast across primitives"},
		{"xs:date(@dt) eq xs:date('2000-01-01')", "a cast across primitives"},
	} {
		if _, ok := CompileAssertionTest(asRecord(tc.expr), seededTypes, xsd.EmptyContent{}, csUses(t), asNoElems); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
	if !afEval(t, "string(xs:date(@s)) eq '2000-01-01'", []asTyped{{uq("s"), "string", " 2000-01-01 "}}) {
		t.Error("Evaluate(string(xs:date(@s)) eq '2000-01-01') over s=' 2000-01-01 ' = false, want true")
	}
	if !afEval(t, "string(xs:decimal(@s)) eq '5'", []asTyped{{uq("s"), "string", " 5.0 "}}) {
		t.Error("Evaluate(string(xs:decimal(@s)) eq '5') over s=' 5.0 ' = false, want true")
	}
}

// cfFacade evaluates one {test} on one of the three façades, over an E or a
// value it does not read, reporting whether the façade decided it and, where
// it did, the answer: true for FacetAssertions' Holds.
type cfFacade struct {
	name string
	eval func(t *testing.T, expr string) (got, decided bool)
}

// cfFacades is CompileAssertionTest, CompileCTATest and FacetAssertions.
func cfFacades() []cfFacade {
	return []cfFacade{
		{"CompileAssertionTest", func(t *testing.T, expr string) (bool, bool) {
			test, ok := afCompile(t, expr, xsd.EmptyContent{})
			if !ok {
				return false, false
			}
			return test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, nil, ValueBinding{}, time.Time{}), true
		}},
		{"CompileCTATest", func(t *testing.T, expr string) (bool, bool) {
			test, ok := CompileCTATest(asRecord(expr), seededTypes)
			if !ok {
				return false, false
			}
			return test.Evaluate(backend(), seededTypes, ctaAttrs()), true
		}},
		{"FacetAssertions", func(t *testing.T, expr string) (bool, bool) {
			str := asBuiltin(t, "string")
			switch FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, asRecord(expr), fcValue(t, str, "x")) {
			case value.AssertionHolds:
				return true, true
			case value.AssertionFails:
				return false, true
			case value.AssertionDeclined:
				return false, false
			}
			return false, false
		}},
	}
}

// A cast from an xs:float or xs:double operand to a target that is neither
// its own type nor an ancestor of it is never decided through ctaPromote's
// re-validation of the canonical "1.5E0" (castsFrom, literalCastsTo): F&O
// §17.1.2 renders 1.5e0 as the xs:string "1.5" and §17.1.3 casts it to the
// xs:decimal 1.5, so a façade that decides a row answers want, and one that
// declines it withholds. The identity cast `xs:double(1.5e0)` (§17.2 case 4)
// and a DecimalLiteral's cast to xs:string compile and hold on every façade.
// With literalCastsTo's last return answering true, every row but the two
// guards compiles and answers the opposite of want.
func TestCastFromAFloatingOperand(t *testing.T) {
	for _, f := range cfFacades() {
		for _, tc := range []struct {
			expr string
			want bool
		}{
			{"xs:string(1.5e0) = '1.5'", true},
			{"xs:decimal(1.5e0) = 1.5", true},
			{"xs:string(1.5e0) = '1.5E0'", false},
			{"1.5e0 cast as xs:string = '1.5'", true},
		} {
			if got, decided := f.eval(t, tc.expr); decided && got != tc.want {
				t.Errorf("%s(%q) = %v, want %v or declined", f.name, tc.expr, got, tc.want)
			}
		}
		for _, expr := range []string{"xs:string(1.5) = '1.5'", "xs:double(1.5e0) = 1.5e0"} {
			if got, decided := f.eval(t, expr); !decided || !got {
				t.Errorf("%s(%q): decided %v, got %v, want decided and true", f.name, expr, decided, got)
			}
		}
	}
}

// A cast of an IntegerLiteral or a DecimalLiteral is decided through
// ctaPromote's re-validation of the canonical lexical only where F&O §17
// defines the cast that way (castsFrom, literalCastsTo). To xs:integer
// §17.1.3.4 discards the fractional part, so `xs:integer(1.5)` is 1; to
// xs:boolean §17.1.6 makes every non-zero value true; to xs:anyURI §17.1's
// casting table marks the cast N, err:XPTY0004. A façade that decides one of
// those rows answers want, and one that declines it withholds; with
// castsFrom's literal arm removed, the first four compile and answer false
// (`xs:boolean(2) = true()` only where the façade admits fn:true) and the
// xs:anyURI row answers true. The guards are the casts the round trip
// performs exactly: the identity and ancestor casts (§17.2 case 4, §17.3), an
// IntegerLiteral down xs:integer's branch (§17.4), and the casts to xs:string
// (§17.1.2) and xs:double (§17.1.3.2); each compiles and holds on every façade.
func TestCastFromADecimalLiteral(t *testing.T) {
	for _, f := range cfFacades() {
		for _, tc := range []struct {
			expr string
			want bool
		}{
			{"xs:integer(1.5) = 1", true},
			{"1.5 cast as xs:integer = 1", true},
			{"xs:boolean(2) = true()", true},
			{"xs:boolean(2)", true},
			{"xs:anyURI(1.5) = '1.5'", false},
		} {
			if got, decided := f.eval(t, tc.expr); decided && got != tc.want {
				t.Errorf("%s(%q) = %v, want %v or declined", f.name, tc.expr, got, tc.want)
			}
		}
		for _, expr := range []string{
			"xs:integer(2) = 2",
			"xs:decimal(2) = 2",
			"xs:int(2) = 2",
			"xs:string(1.5) = '1.5'",
			"xs:double(1.5) = 1.5e0",
		} {
			if got, decided := f.eval(t, expr); !decided || !got {
				t.Errorf("%s(%q): decided %v, got %v, want decided and true", f.name, expr, decided, got)
			}
		}
	}
}

// fn:string over a cast of a numeric literal is the cast literalCastsTo
// admitted followed by §17.1.2's cast to xs:string, and the two compose to the
// round trip of the literal's own canonical lexical: castSource leaves such a
// cast judged by its target alone (floatingSource), so each row compiles and
// holds on every façade that calls fn:string — CompileCTATest's calls no
// library function. With castSource judging a literal by the type it carries,
// the nested cast is judged xs:decimal or xs:integer, and every row declines on
// both façades.
func TestStringOfACastLiteral(t *testing.T) {
	for _, f := range cfFacades() {
		if f.name == "CompileCTATest" {
			continue
		}
		for _, expr := range []string{
			"string(xs:decimal(5)) = '5'",
			"string(xs:integer(5)) = '5'",
			"string(xs:decimal(1.5)) = '1.5'",
		} {
			if got, decided := f.eval(t, expr); !decided || !got {
				t.Errorf("%s(%q): decided %v, got %v, want decided and true", f.name, expr, decided, got)
			}
		}
	}
}
