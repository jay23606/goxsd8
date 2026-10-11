package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive xpath20.md §3.9's QuantifiedExpr over one child
// step, in the two body forms the chess assertion of
// saxonData/Assert/assert007.xsd:10 and the attribute test reach: a body
// `$v/@A`, and `$v/following-sibling::*[1][self::M]` (§3.2.1.1, §3.2.2). Both
// are read off the [Tally], with no variable bound at evaluation.

// aqDecides compiles expr with awCompile and evaluates it over nodes.
func aqDecides(t *testing.T, expr string, nodes ...acNode) bool {
	t.Helper()
	return awEvaluate(t, awCompile(t, asRecord(expr)), nodes...)
}

// `every` holds where each bound child satisfies the body and `some` where one
// does, so over no child `every` is true and `some` false (§3.9). The `$c/@a`
// body holds for a c carrying a, whatever its value. Every row declines at
// CompileAssertionTest, and so fails, with exprSingle's quantifiedExpr arm
// removed; the two empty rows pin the polarity of each quantifier.
func TestAssertionQuantifiedOverAttributes(t *testing.T) {
	ca := acElAt(1, "c", "a")
	cb := acElAt(1, "c", "b")
	cab := acElAt(1, "c", "a", "b")
	d := acElAt(1, "d", "a")
	for _, tc := range []struct {
		why   string
		expr  string
		nodes []acNode
		holds bool
	}{
		{"every, all carry a", "every $c in c satisfies $c/@a", []acNode{ca, ca}, true},
		{"every, one lacks a", "every $c in c satisfies $c/@a", []acNode{ca, cb}, false},
		{"every, over no c", "every $c in c satisfies $c/@a", []acNode{d}, true},
		{"every, over nothing at all", "every $c in c satisfies $c/@a", nil, true},
		{"some, one carries a", "some $c in c satisfies $c/@a", []acNode{cb, ca}, true},
		{"some, none carries a", "some $c in c satisfies $c/@a", []acNode{cb, cb, d}, false},
		{"some, over no c", "some $c in c satisfies $c/@a", []acNode{d}, false},
		{"some, over nothing at all", "some $c in c satisfies $c/@a", nil, false},
		{"every, the a on another name only", "every $c in c satisfies $c/@a", []acNode{cb, d}, false},
		{"every over ./c", "every $c in ./c satisfies $c/@a", []acNode{ca}, true},
		{"every, a conjunction held", "every $c in c satisfies $c/@a and $c/@b", []acNode{cab, cab}, true},
		{"every, a conjunction split", "every $c in c satisfies $c/@a and $c/@b", []acNode{cab, ca}, false},
		{"some, a conjunction split across two c", "some $c in c satisfies $c/@a and $c/@b", []acNode{ca, cb}, false},
		{"every under not, by De Morgan", "every $c in c satisfies not($c/@a)", []acNode{cb, cb}, true},
		{"every under not, one carries a", "every $c in c satisfies not($c/@a)", []acNode{cb, ca}, false},
		{"every under not, over no c", "every $c in c satisfies not($c/@a)", nil, true},
		{"some under not, one lacks a", "some $c in c satisfies not($c/@a)", []acNode{ca, cb}, true},
		{"some under not, all carry a", "some $c in c satisfies not($c/@a)", []acNode{ca, ca}, false},
		{"some under not, over no c", "some $c in c satisfies not($c/@a)", nil, false},
		{"fn:not over the whole quantifier", "not(every $c in c satisfies $c/@a)", []acNode{ca, cb}, true},
		{"an IfExpr test", "if (some $c in c satisfies $c/@a) then true() else false()", []acNode{ca}, true},
		{"an IfExpr then-branch", "if (true()) then every $c in c satisfies $c/@a else true()", []acNode{cb}, false},
		{"a parenthesized and operand", "(every $c in c satisfies $c/@a) and count(c) = 2", []acNode{ca, ca}, true},
		{"a parenthesized and operand, count false", "(every $c in c satisfies $c/@a) and count(c) = 2", []acNode{ca}, false},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if got := aqDecides(t, tc.expr, tc.nodes...); got != tc.holds {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.nodes, got, tc.holds)
			}
		})
	}
}

// `$w/following-sibling::*[1][self::white]` selects the next sibling ELEMENT
// of `$w` where it is named white, and nothing where it is named otherwise —
// even with a white later on — or where `$w` is E's last child element. A
// report below a child (depth 2) is no sibling of E's children. The rows run
// assert007's `every … not(…)` and the bare `some` form, which De Morgan's
// law does not rewrite.
//
// Every row declines at CompileAssertionTest, and so fails, with exprSingle's
// quantifiedExpr arm removed. With ctaSuccessionCount.element never resetting
// latestIsName, so a white anywhere after `$w` counts, the rows "white, black,
// white" and "white, black, white, result" fail in both forms, and so does the
// black, white, result, black check below; with it reading a depth-2 report's
// last name as a sibling's, the row "a grandchild white, then a black" fails;
// with count() adding one while latestIsName holds, the rows "white, black,
// white", "a white last child" and "a lone white" fail.
func TestAssertionQuantifiedOverNextSibling(t *testing.T) {
	const noAdjacent = "every $w in white satisfies not($w/following-sibling::*[1][self::white])"
	const someAdjacent = "some $w in white satisfies $w/following-sibling::*[1][self::white]"
	w, b, r := acPath("white"), acPath("black"), acPath("result")
	under := acPath("white", "white")
	underBlack := acPath("black", "white")
	for _, tc := range []struct {
		why   string
		nodes []acNode
		// adjacent is whether some white child's next sibling element is a white.
		adjacent bool
	}{
		{"white, white", []acNode{w, w}, true},
		{"white, black, white", []acNode{w, b, w}, false},
		{"white, black, white, result", []acNode{w, b, w, r}, false},
		{"black, white, white, result", []acNode{b, w, w, r}, true},
		{"a white last child", []acNode{b, w}, false},
		{"a lone white", []acNode{w}, false},
		{"no white", []acNode{b, r}, false},
		{"nothing", nil, false},
		{"a grandchild white, then a black", []acNode{w, under, b}, false},
		{"a grandchild white under the next sibling", []acNode{w, b, underBlack}, false},
		{"a grandchild white, then a white sibling", []acNode{w, under, w}, true},
	} {
		t.Run(tc.why, func(t *testing.T) {
			if got := aqDecides(t, noAdjacent, tc.nodes...); got != !tc.adjacent {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", noAdjacent, tc.nodes, got, !tc.adjacent)
			}
			if got := aqDecides(t, someAdjacent, tc.nodes...); got != tc.adjacent {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", someAdjacent, tc.nodes, got, tc.adjacent)
			}
		})
	}
	// A next sibling of another name than the bound one: `$w`'s next element
	// is a black.
	const toBlack = "some $w in white satisfies $w/following-sibling::*[1][self::black]"
	if !aqDecides(t, toBlack, b, w, b) {
		t.Errorf("Evaluate(%q) over black, white, black = false, want true", toBlack)
	}
	if aqDecides(t, toBlack, b, w, r, b) {
		t.Errorf("Evaluate(%q) over black, white, result, black = true, want false", toBlack)
	}
}

// assert007's whole {test}, in the three spellings of its child names' default
// namespace: a prefix (assert007), an xpathDefaultNamespace on the assert
// (assert008) and one on the schema (assert008a), the last two reaching the
// compile as the record's {default namespace}. Each decides the suite's v1
// game valid and its n1 game invalid, n1 holding two adjacent whites (d4,
// c3). A name in no namespace is another name: the unprefixed spelling without
// a default namespace binds no chess child and holds over n1. With
// rangeBinding, or siblingBody's M, resolved on attributeName's terms instead
// of elementName's, the default-namespace row fails over n1.
func TestAssertionQuantifiedChessSpellings(t *testing.T) {
	const ns = "http://chess/ns/"
	prefixed := `(every $w in chess:white satisfies not($w/following-sibling::*[1][self::chess:white])) and
                       (every $b in chess:black satisfies not($b/following-sibling::*[1][self::chess:black]))`
	unprefixed := `(every $w in white satisfies not($w/following-sibling::*[1][self::white])) and
                       (every $b in black satisfies not($b/following-sibling::*[1][self::black]))`
	chess := func(locals ...string) []acNode {
		var nodes []acNode
		for _, l := range locals {
			nodes = append(nodes, acNode{path: []xsd.QName{{Space: ns, Local: l}}})
		}
		return nodes
	}
	v1 := chess("white", "black", "white", "result")
	n1 := chess("white", "black", "white", "white", "result")
	for _, tc := range []struct {
		why    string
		record xsd.XPathExpression
		v1, n1 bool
	}{
		{"a prefix, assert007", ctaExprRecord(prefixed, "", "chess", ns), true, false},
		{"a default namespace, assert008 and assert008a", ctaExprRecord(unprefixed, ns), true, false},
		{"no namespace at all", ctaExprRecord(unprefixed, ""), true, true},
	} {
		test := awCompile(t, tc.record)
		if got := awEvaluate(t, test, v1...); got != tc.v1 {
			t.Errorf("%s: Evaluate over v1 = %v, want %v", tc.why, got, tc.v1)
		}
		if got := awEvaluate(t, test, n1...); got != tc.n1 {
			t.Errorf("%s: Evaluate over n1 = %v, want %v", tc.why, got, tc.n1)
		}
	}
}

// A quantifier keeps the counters its keys need and shares the binding count
// with `count(N)`: `every` keys the satisfying children and N, `some` the
// satisfying children alone, and the De Morgan rewrite of `every … not(B)` is
// a `some`. Neither form reads a child's value. ReadsAttributesAt(1) is true
// for the `$c/@a` body, whose key reads depth 1's attribute names, and false
// for the sibling body, which reads names alone.
func TestAssertionQuantifiedCounters(t *testing.T) {
	for _, tc := range []struct {
		expr       string
		counters   int
		attributes bool
	}{
		{"every $c in c satisfies $c/@a", 2, true},
		{"some $c in c satisfies $c/@a", 1, true},
		{"count(c) = 1 and (every $c in c satisfies $c/@a)", 2, true},
		{"count(c[@a]) = 1 and (some $c in c satisfies $c/@a)", 1, true},
		{"every $c in c satisfies not($c/@a)", 1, true},
		{"every $c in c satisfies $c/following-sibling::*[1][self::c]", 2, false},
		{"every $c in c satisfies not($c/following-sibling::*[1][self::c])", 1, false},
		{"(some $c in c satisfies $c/following-sibling::*[1][self::d]) and (some $c in c satisfies $c/following-sibling::*[1][self::d])", 1, false},
		{"(some $c in c satisfies $c/following-sibling::*[1][self::d]) and (some $c in c satisfies $c/following-sibling::*[1][self::e])", 2, false},
	} {
		test := awCompile(t, asRecord(tc.expr))
		c := test.Tally()
		if c == nil || len(c.counters) != tc.counters {
			t.Errorf("(%s).Tally() = %v, want %d counters", tc.expr, c, tc.counters)
		}
		if got := c.ReadsAttributesAt(1); got != tc.attributes {
			t.Errorf("(%s).Tally().ReadsAttributesAt(1) = %v, want %v", tc.expr, got, tc.attributes)
		}
		if c.ReadsAttributesAt(0) || c.ReadsAttributesAt(2) {
			t.Errorf("(%s).Tally() counts attributes at depth 0 or 2, want neither", tc.expr)
		}
		for _, name := range []string{"c", "d", "e"} {
			if test.ReadsChild(uq(name)) {
				t.Errorf("(%s).ReadsChild(%s) = true, want false", tc.expr, name)
			}
		}
	}
}

// A range variable is compared by its ·expanded name·: a prefixed one, bound
// in the record, names itself in the body only under a prefix bound to the
// same namespace.
func TestAssertionQuantifiedVariableNames(t *testing.T) {
	ca := acElAt(1, "c", "x")
	if !aqDecides(t, "every $a:w in c satisfies $a:w/@x", ca) {
		t.Errorf("every $a:w … $a:w/@x over a c carrying x: false, want true")
	}
	record := ctaExprRecord("every $a:w in c satisfies $b:w/@x", "", "a", "urn:q", "b", "urn:q")
	if !awEvaluate(t, awCompile(t, record), ca) {
		t.Errorf("every $a:w … $b:w/@x, a and b bound alike, over a c carrying x: false, want true")
	}
}

// Every QuantifiedExpr over a child step outside the admitted shapes declines,
// and so does the range variable outside its body (err:XPST0008). The two
// `preceding::` rows stand outside the child-step predicate #2522 admits and
// belong to #1042. A binding over `$value` is
// TestAssertionQuantifiedOverValueDeclines'.
func TestAssertionQuantifiedDeclines(t *testing.T) {
	for _, tc := range []struct{ expr, why string }{
		{"every $value in c satisfies $value/@a", "a range variable named $value"},
		{"every $c in c, $d in d satisfies $c/@a", "two in-clauses"},
		{"every $c in * satisfies $c/@a", "a wildcard binding"},
		{"every $c in a/b satisfies $c/@a", "a longer binding path"},
		{"every $c in .//c satisfies $c/@a", "a descendant binding"},
		{"every $c in @x satisfies $c/@a", "an attribute binding"},
		{"every $c in . satisfies $c/@a", "the context item as binding"},
		{"every $c in c[@a] satisfies $c/@a", "a filtered binding"},
		{"every $c in c satisfies some $d in d satisfies $d/@a", "a nested quantifier"},
		{"every $c in c satisfies $c/@a or $c/@b", "a disjunctive body"},
		{"every $c in c satisfies $c/@a = 1", "a body reading the attribute's value"},
		{"every $c in c satisfies $c = 1", "a body reading $c's value"},
		{"every $c in c satisfies $c/@*", "a wildcard attribute"},
		{"every $c in c satisfies $c/@a and @b", "a conjunct off E"},
		{"every $c in c satisfies $c/@a and $d/@b", "a conjunct off another variable"},
		{"every $c in c satisfies not($c/@a) and not($c/@b)", "a conjunction of fn:not"},
		{"every $c in c satisfies ($c/@a)", "a parenthesized body"},
		{"every $c in c satisfies $d/@a", "another variable in the body"},
		{"every $a:w in c satisfies $w/@x", "a prefixed variable named unprefixed"},
		{"(every $c in c satisfies $c/@a) and $c/@a", "the variable outside its body"},
		{"every $c in c satisfies $c/following-sibling::c[1]", "a NameTest before [1]"},
		{"every $c in c satisfies $c/following-sibling::*[self::c][1]", "the predicates swapped"},
		{"every $c in c satisfies $c/following-sibling::*[1]", "no [self::M]"},
		{"every $c in c satisfies $c/following-sibling::*[self::c]", "no [1]"},
		{"every $c in c satisfies $c/following-sibling::*", "a bare following-sibling::*"},
		{"every $c in c satisfies $c/following-sibling::*[2][self::c]", "[2]"},
		{"every $c in c satisfies $c/following-sibling::*[0][self::c]", "[0]"},
		{"every $c in c satisfies $c/following-sibling::*[01][self::c]", "[01]"},
		{"every $c in c satisfies $c/following-sibling::*[last()][self::c]", "[last()]"},
		{"every $c in c satisfies $c/following-sibling::*[position() = 1][self::c]", "[position() = 1]"},
		{"every $c in c satisfies $c/following-sibling::*[1][self::*]", "a wildcard self test"},
		{"every $c in c satisfies $c/following-sibling::*[1][self::node()]", "a kind test on self"},
		{"every $c in c satisfies $c/self::c", "self:: outside a predicate"},
		{"every $c in c satisfies $c/following-sibling::*[1][self::c] or $c/@a", "or after the sibling body"},
		{"every $c in c satisfies $c/preceding-sibling::*[1][self::c]", "preceding-sibling::"},
		{"every $c in c satisfies $c/preceding::*[1][self::c]", "preceding:: in a body (#1042)"},
		{"preceding::c", "preceding:: off E (#1042)"},
		{"count(every $c in c satisfies $c/@a) = 1", "a quantifier as a value"},
		{"every $c in c satisfies $c/following-sibling::*[1][self::c] and count(c) = 1", "a body followed by and"},
	} {
		if _, ok := CompileAssertionTest(asRecord(tc.expr), seededTypes, asElementContent(t, false), asUses(t, nil), asNoElems); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
}

// A Type Alternative's {test} has no quantifier, child step, variable or axis
// beyond attribute in its grammar (ta-props-correct clause 2), so each form
// declines there; an assertions facet's binding step would read the absent
// context item and is declined there, and so is a quantifier over `$value`,
// which the facet binds (FacetAssertions' GAP(xpath)), never Holds or Fails.
// The Type Alternative rows over a child step fail with
// ctaTypeAlternativeFacade.quantified admitting the node, and those over
// `$value` decline at ctaTypeAlternativeFacade.variable before rangeScope is
// asked; the facet rows over a child step fail with ctaFacetFacade.quantified
// admitting the node, and those over `$value`, which hold over "x" where
// compiled, with ctaFacetFacade.rangeScope admitting the body's scope.
func TestAssertionQuantifiedOutsideTheAssertionFacade(t *testing.T) {
	forms := []string{
		"every $c in c satisfies $c/@a",
		"some $c in c satisfies $c/@a",
		"every $w in white satisfies not($w/following-sibling::*[1][self::white])",
		"some $w in white satisfies $w/following-sibling::*[1][self::white]",
		"every $x in data($value) satisfies $x = 'x'",
		"some $x in $value satisfies $x = 'x'",
	}
	str := asBuiltin(t, "string")
	for _, expr := range forms {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
		if got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(expr, ""), fcValue(t, str, "x")); got != value.AssertionDeclined {
			t.Errorf("FacetAssertions().Evaluate(%q) = %d, want Declined (%d)", expr, got, value.AssertionDeclined)
		}
	}
	for _, expr := range []string{"preceding::c", "@a and preceding::c"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
}

// A value predicate inside an fn:count argument parses under
// ctaPredicateFacade, which declines a quantifier however it is reached, and
// ctaComparisonRooted refuses one as a predicate's root besides. It is a
// guard: the row fails only with both ctaPredicateFacade.quantified admitting
// the node and ctaComparisonRooted answering true for it, and with either alone
// it still passes.
func TestAssertionQuantifiedInAPredicateDeclines(t *testing.T) {
	elems := func(xsd.QName) (xsd.TypeDefinition, bool) { return asBuiltin(t, "string"), true }
	expr := "count(c[(every $d in d satisfies $d/@a)]) = 0"
	if _, ok := CompileAssertionTest(asRecord(expr), seededTypes, asElementContent(t, false), asUses(t, nil), elems); ok {
		t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
	}
}
