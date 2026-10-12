package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive xpath20.md §3.9's QuantifiedExpr over a typed child
// step whose body reads its bound children's values, the ascending-order
// assertion of ibmData/mixed/assertions/namespace/ns2.xsd:23, `every $y in
// y[position() lt last()] satisfies ($y lt $y/following-sibling::y[1])`: the
// binding step filtered by `[position() lt last()]` selects every y but the
// last (§3.2.2), `$y/following-sibling::y[1]` is the first y after `$y`
// however many other children intervene (§3.2.1.1), and `lt` compares the
// children's TYPED values (§3.5.1, cvc-assertion clause 1.2).

// aqcAscending is ns2.xsd:23's {test}.
const aqcAscending = "every $y in y[position() lt last()] satisfies ($y lt $y/following-sibling::y[1])"

// aqcElems is E's child element types: y is xs:integer, s is xs:string, z is
// xs:string, m has mixed content and l is a list of xs:int.
func aqcElems(t *testing.T) ElementTypes {
	t.Helper()
	return asElems(map[xsd.QName]xsd.TypeDefinition{
		uq("y"): asBuiltin(t, "integer"), uq("s"): asBuiltin(t, "string"), uq("z"): asBuiltin(t, "string"),
		uq("m"): asComplex(t, "Mixed", asElementContent(t, true)),
		uq("l"): asList(t, "IntList", ctaBuiltin("int")),
	})
}

// aqcTypes is the seeded builtins plus aqcElems' list type.
func aqcTypes(t *testing.T) xsd.TypeResolver {
	t.Helper()
	return asTypesWith(asList(t, "IntList", ctaBuiltin("int")))
}

// aqcCompile compiles expr for an element with element-only content whose
// children aqcElems types, reporting whether it compiled.
func aqcCompile(t *testing.T, expr string) (AssertionTest, bool) {
	t.Helper()
	return CompileAssertionTest(asRecord(expr), aqcTypes(t), asElementContent(t, false), asUses(t, nil), aqcElems(t))
}

// aqcDecides compiles expr, failing the test where it declines, and evaluates
// it over children in the order written.
func aqcDecides(t *testing.T, expr string, children ...asChild) bool {
	t.Helper()
	test, ok := aqcCompile(t, expr)
	if !ok {
		t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
	}
	return test.Evaluate(backend(), aqcTypes(t), asElem, asValues(t), asChildren(t, children...), nil, ValueBinding{}, time.Time{})
}

// aqcYs is one xs:integer y child per lexical, in order, and an xs:string z
// child for each lexical "z", a nilled y for each "nil".
func aqcYs(lexicals ...string) []asChild {
	out := make([]asChild, 0, len(lexicals))
	for _, l := range lexicals {
		switch l {
		case "z":
			out = append(out, asChild{uq("z"), "string", "x", false})
		case "nil":
			out = append(out, asChild{name: uq("y"), nilled: true})
		default:
			out = append(out, asChild{uq("y"), "integer", l, false})
		}
	}
	return out
}

// `y[position() lt last()]` as a binding sequence binds every y child but the
// last, in document order — so a 9 last is never bound and `every … $y lt 3`
// holds over 1 2 9 where the unfiltered binding does not, and a 2 last is never
// found by `some … $y = 2` — and over one y it binds none, so `every` holds and
// `some` fails (§3.9). The unwindowed rows pin the bare binding the same node
// takes. Every row declines at CompileAssertionTest, and so fails, with
// quantifiedExpr routing no body to childQuantified. With
// ctaQuantifiedChildren.eval ignoring dropLast, the rows "1 2 9, windowed",
// "1 2, the 2 last", "a lone y, every", "a lone y, some" and "a z between,
// windowed over ./y" fail.
func TestAssertionQuantifiedChildrenWindow(t *testing.T) {
	for _, tc := range []struct {
		why   string
		expr  string
		ys    []string
		holds bool
	}{
		{"1 2 9, windowed", "every $y in y[position() lt last()] satisfies $y lt 3", []string{"1", "2", "9"}, true},
		{"1 2 9, unwindowed", "every $y in y satisfies $y lt 3", []string{"1", "2", "9"}, false},
		{"1 2, unwindowed", "every $y in y satisfies $y lt 3", []string{"1", "2"}, true},
		{"1 2, the 2 last", "some $y in y[position() lt last()] satisfies $y = 2", []string{"1", "2"}, false},
		{"2 1, the 1 last", "some $y in y[position() lt last()] satisfies $y = 2", []string{"2", "1"}, true},
		{"a lone y, every", "every $y in y[position() lt last()] satisfies $y lt 0", []string{"5"}, true},
		{"a lone y, some", "some $y in y[position() lt last()] satisfies $y = 5", []string{"5"}, false},
		{"no y, every", "every $y in y[position() lt last()] satisfies $y lt 0", nil, true},
		{"no y, some", "some $y in ./y[position() lt last()] satisfies $y = 5", nil, false},
		{"a z between, windowed over ./y", "every $y in ./y[position() lt last()] satisfies $y lt 3", []string{"1", "z", "2", "9"}, true},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if got := aqcDecides(t, tc.expr, aqcYs(tc.ys...)...); got != tc.holds {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.ys, got, tc.holds)
			}
		})
	}
}

// ns2.xsd:23's {test} holds over ascending y children and fails where one is
// not below the next y; `following-sibling::y[1]` is the first y sibling
// however many z children intervene, so 1 z 2 holds. The comparison is
// numeric over the typed xs:integer values: 2 10 holds, where the xs:string
// comparison of the same lexicals would not. The suite's ns2.xml, 2 4 6 8 10,
// holds. A ·nilled· y is a node whose typed value is empty (xpath-datamodel
// §6.2.4): bound, it counts for the window, and `lt` over it answers the empty
// sequence, whose ·effective boolean value· is false (§3.5.1, §2.4.3), so
// `every` fails whether the nilled y is bound or is the next y.
//
// Every row declines at CompileAssertionTest, and so fails, with
// atBoundSibling answering false. With ctaChildBinding.next returning the next
// child of any name, "1 z 2" fails; with it starting its scan at the bound
// child itself, "1 2 3", "1 z 2", "2 10, numeric" and "ns2.xml" fail.
func TestAssertionQuantifiedChildrenAscending(t *testing.T) {
	for _, tc := range []struct {
		why   string
		ys    []string
		holds bool
	}{
		{"1 2 3", []string{"1", "2", "3"}, true},
		{"1 3 2", []string{"1", "3", "2"}, false},
		{"1 z 2", []string{"1", "z", "2"}, true},
		{"1 z 3 z z 2", []string{"1", "z", "3", "z", "z", "2"}, false},
		{"2 10, numeric", []string{"2", "10"}, true},
		{"10 9", []string{"10", "9"}, false},
		{"equal, not below", []string{"4", "4"}, false},
		{"ns2.xml", []string{"2", "4", "6", "8", "10"}, true},
		{"no y", nil, true},
		{"a lone y", []string{"7"}, true},
		{"a nilled y next", []string{"1", "nil", "3"}, false},
		{"a nilled y last", []string{"1", "2", "nil"}, false},
		{"a nilled y bound first", []string{"nil", "2"}, false},
		{"a lone nilled y", []string{"nil"}, true},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if got := aqcDecides(t, aqcAscending, aqcYs(tc.ys...)...); got != tc.holds {
				t.Errorf("Evaluate over %v = %v, want %v", tc.ys, got, tc.holds)
			}
		})
	}
}

// The comparison runs in the bound children's own type: over xs:string s
// children the same body compares by codepoints, so "2" "10" fails where the
// xs:integer y children of TestAssertionQuantifiedChildrenAscending hold. The
// bound child is a node: as a bare body its ·effective boolean value· is true,
// a ·nilled· one's included (xpath20.md §2.4.3 rule 2), where the xs:integer
// 0's fn:boolean would be false; and `$y/following-sibling::y[1]` as a bare
// body asks whether a later y exists. A value of the wrong arm breaks the
// obligation [ChildElements] states and raises, whether or not the body reads
// the bound child. With ctaBoundChild dropped from ctaStep, "0 bare", "nilled
// bare" and "0 under not" fail; with ctaQuantifiedChildren.eval's arm check
// removed, the Untyped row under `every $y in y satisfies true()` fails, every
// other body raising at its own read (ctaBoundItem, ctaBoundNodes); with
// dropLast ignored, the xs:string row over "10" "2" fails.
func TestAssertionQuantifiedChildrenTyped(t *testing.T) {
	strings := "every $s in s[position() lt last()] satisfies ($s lt $s/following-sibling::s[1])"
	if aqcDecides(t, strings, asChild{uq("s"), "string", "2", false}, asChild{uq("s"), "string", "10", false}) {
		t.Errorf("Evaluate(%q) over \"2\" \"10\" = true, want false: xs:string compares by codepoints", strings)
	}
	if !aqcDecides(t, strings, asChild{uq("s"), "string", "10", false}, asChild{uq("s"), "string", "2", false}) {
		t.Errorf("Evaluate(%q) over \"10\" \"2\" = false, want true", strings)
	}
	for _, tc := range []struct {
		why   string
		expr  string
		ys    []string
		holds bool
	}{
		{"a bare binding reading the value", "every $y in y satisfies $y = 1", []string{"1", "1"}, true},
		{"a bare binding reading the value, one off", "every $y in y satisfies $y = 1", []string{"1", "2"}, false},
		{"0 bare", "every $y in y satisfies $y", []string{"0"}, true},
		{"nilled bare", "every $y in y satisfies $y", []string{"nil"}, true},
		{"0 under not", "some $y in y satisfies not($y)", []string{"0"}, false},
		{"a later y, bare", "some $y in y satisfies $y/following-sibling::y[1]", []string{"1", "z", "2"}, true},
		{"no later y, bare", "every $y in y satisfies $y/following-sibling::y[1]", []string{"1", "z", "2"}, false},
		{"a later z", "some $y in y satisfies $y/following-sibling::z[1] = 'x'", []string{"1", "z"}, true},
		{"no later z", "some $y in y satisfies $y/following-sibling::z[1] = 'x'", []string{"z", "1"}, false},
		{"$value beside the bound child", "every $y in y satisfies empty($value)", []string{"1"}, true},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if got := aqcDecides(t, tc.expr, aqcYs(tc.ys...)...); got != tc.holds {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.ys, got, tc.holds)
			}
		})
	}
	wrong := func(yield func(ChildElement) bool) {
		_ = yield(Child(uq("y"), Untyped("1"))) && yield(Child(uq("y"), Untyped("2")))
	}
	for _, expr := range []string{aqcAscending, "not(" + aqcAscending + ")", "every $y in y satisfies $y", "every $y in y satisfies true()"} {
		test, ok := aqcCompile(t, expr)
		if !ok {
			t.Fatalf("CompileAssertionTest(%q): declined, want compiled", expr)
		}
		if test.Evaluate(backend(), aqcTypes(t), asElem, asValues(t), wrong, nil, ValueBinding{}, time.Time{}) {
			t.Errorf("Evaluate(%q) over Untyped y children = true, want false: the read raises", expr)
		}
	}
}

// The quantifier reads its binding step's children, and the body's sibling
// step's, off [ChildElements] and counts nothing: ReadsChild reports y, and z
// where `following-sibling::z[1]` reads it, and Tally is nil. With
// ctaQuantifiedChildren.readsChild reporting the body's reads alone, y fails in
// the last three rows (the first row's sibling step reads y itself); with
// ctaBoundSibling.readsChild answering false, the second row's z fails.
func TestAssertionQuantifiedChildrenReads(t *testing.T) {
	for _, tc := range []struct {
		expr  string
		reads []string
		not   []string
	}{
		{aqcAscending, []string{"y"}, []string{"z", "s"}},
		{"every $y in y satisfies $y/following-sibling::z[1] = 'x'", []string{"y", "z"}, []string{"s"}},
		{"every $y in y satisfies $y lt 3", []string{"y"}, []string{"z"}},
		{"every $y in y satisfies s = 'x'", []string{"y", "s"}, []string{"z"}},
	} {
		test, ok := aqcCompile(t, tc.expr)
		if !ok {
			t.Fatalf("CompileAssertionTest(%q): declined, want compiled", tc.expr)
		}
		for _, name := range tc.reads {
			if !test.ReadsChild(uq(name)) {
				t.Errorf("(%s).ReadsChild(%s) = false, want true", tc.expr, name)
			}
		}
		for _, name := range tc.not {
			if test.ReadsChild(uq(name)) {
				t.Errorf("(%s).ReadsChild(%s) = true, want false", tc.expr, name)
			}
		}
		if c := test.Tally(); c != nil {
			t.Errorf("(%s).Tally() = %v, want nil", tc.expr, c)
		}
	}
}

// Every form outside the admitted one declines. A binding or sibling child of
// mixed content (m) or of a list type (l) has no typed node to bind; a
// predicate on the binding step other than `[position() lt last()]` — `<`,
// `ne`, `[last()]`, `[1]` — and an element step over the range variable other
// than `following-sibling::M[1]` decline, as does any quantifier nested in the
// body. Two rows step to an attribute of the bound child: windowed, `$y/@a`
// declines here only because aqcCompile's attrs (asUses(t, nil)) answer no
// type for y's a; unwindowed, `$y/@a = 1` opens as the [Tally]'s body
// (ctaParser.atTalliedBody), which declines it whatever a's type.
// TestAssertionBoundAttributesDecline states what such a step admits now and
// what it declines. fn:position and fn:last outside that one predicate keep the
// assertion façade's decline (ctaAssertionFacade.focus), in the body and out
// of it, the window being a fixed spelling and no read of the focus.
func TestAssertionQuantifiedChildrenDeclines(t *testing.T) {
	for _, tc := range []struct{ expr, why string }{
		{"every $m in m[position() lt last()] satisfies ($m lt $m/following-sibling::m[1])", "a mixed binding child"},
		{"every $m in m satisfies $m = 'x'", "a mixed binding child, unwindowed"},
		{"every $l in l satisfies $l = 1", "a list-typed binding child"},
		{"every $y in y satisfies $y lt $y/following-sibling::m[1]", "a mixed sibling child"},
		{"every $y in y satisfies $y lt $y/following-sibling::l[1]", "a list-typed sibling child"},
		{"every $y in y satisfies $y lt $y/following-sibling::q[1]", "a sibling of no declared type"},
		{"every $y in y satisfies $y lt $y/following-sibling::*[1]", "a wildcard sibling"},
		{"every $y in y satisfies $y lt $y/following-sibling::y[2]", "[2] on the sibling"},
		{"every $y in y satisfies $y lt $y/following-sibling::y", "no [1] on the sibling"},
		{"every $y in y satisfies $y lt $y/preceding-sibling::y[1]", "preceding-sibling::"},
		{"every $y in y satisfies $y lt $y/following::y[1]", "following::"},
		{"every $y in y satisfies $y lt $z/following-sibling::y[1]", "another variable's sibling"},
		{"every $y in y[position() < last()] satisfies $y lt 3", "< in the window"},
		{"every $y in y[position() ne last()] satisfies $y lt 3", "ne in the window"},
		{"every $y in y[last()] satisfies $y lt 3", "[last()]"},
		{"every $y in y[1] satisfies $y lt 3", "[1]"},
		{"every $y in y[position() lt last()][1] satisfies $y lt 3", "a second predicate"},
		{"every $y in y[position() lt last()] satisfies $y/@a", "an attribute attrs answers no type for"},
		{"every $y in y[position() lt last()] satisfies $y/following-sibling::*[1][self::y]", "a windowed binding under a sibling body"},
		{"every $y in y satisfies (some $z in y[position() lt last()] satisfies $z = $y)", "a nested windowed quantifier"},
		{"every $y in y satisfies (some $z in y satisfies $z/@a)", "a nested Tally quantifier"},
		{"every $y in y satisfies (some $x in data($value) satisfies $x = $y)", "a nested quantifier over $value"},
		{"every $y in y satisfies $y/@a = 1", "a body opening as the Tally's"},
		{"every $y in y satisfies data($y) instance of xs:integer", "instance of over the bound child (ctaTypes.instanceItem)"},
		{"every $y in y satisfies data($y/following-sibling::y[1]) instance of xs:integer", "instance of over the sibling (ctaTypes.instanceItem)"},
		{"every $y in y satisfies position() = 1", "position() in the body"},
		{"every $y in y satisfies last() = 1", "last() in the body"},
		{"position() lt last()", "position() and last() at the root"},
		{"$value/following-sibling::y[1] = 1", "$value is no node"},
		{"(every $y in y satisfies $y lt 3) and $y = 1", "the variable outside its body"},
	} {
		if _, ok := aqcCompile(t, tc.expr); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
}

// A quantifier over a typed child step nested in the body of one over a typed
// `$value` declines (ctaRangeFacade.childScope), and so does
// `$x/following-sibling::y[1]` over that body's range variable, an atomic
// value from which no axis steps. The windowed row fails with
// ctaRangeFacade.childScope answering a ctaChildRangeFacade.
func TestAssertionQuantifiedChildrenInAValueBodyDeclines(t *testing.T) {
	content := xsd.SimpleContent{SimpleType: asBuiltin(t, "integer")}
	for _, expr := range []string{
		"every $x in data($value) satisfies (" + aqcAscending + ")",
		"some $x in $value satisfies (every $y in y satisfies $y = $x)",
		"some $x in $value satisfies $x/following-sibling::y[1] = 1",
	} {
		if _, ok := CompileAssertionTest(asRecord(expr), aqcTypes(t), content, asUses(t, nil), aqcElems(t)); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
}
