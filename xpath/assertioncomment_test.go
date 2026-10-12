package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
)

// The fixtures below drive one comment step, xpath20.md [58] CommentTest,
// `comment()`, `./comment()` or `.//comment()`, as the whole operand of
// fn:exists, fn:empty or an ·effective boolean value·: cvc-assertion clause
// 1.2 excludes comments from the partial ·PSVI· the data model instance is
// built from, so the step selects the empty sequence over any subtree.

// A comment step selects nothing whatever E's subtree holds: over no element,
// element children, and elements two and three levels below E, fn:empty over
// it is true and fn:exists and its ·effective boolean value· false, in each
// admitted spelling. Rows beside an element step read the [Tally] that step
// counts, which the comment step leaves alone. Every row fails with the
// comment branch of ctaParser.selectedStepLength removed, each declining at
// CompileAssertionTest, and with ctaAssertionFacade.comments returning a
// ctaSelectedElements over the descendants named a. With it returning
// ctaNoContextItem the six rows true over some subtree fail — the four
// fn:empty rows and the two fn:not rows — and the other seven pass, an error
// reading false and `exists(.//d)` deciding the `or` row alone.
func TestAssertionCommentStepSelectsNothing(t *testing.T) {
	subtrees := [][]acNode{
		nil,
		{acPath("a"), acPath("b")},
		{acPath("a"), acPath("a", "a"), acPath("a", "a", "d")},
	}
	for _, tc := range []struct {
		expr  string
		holds []bool
	}{
		{"empty(.//comment())", []bool{true, true, true}},
		{"exists(.//comment())", []bool{false, false, false}},
		{"not(.//comment())", []bool{true, true, true}},
		{".//comment()", []bool{false, false, false}},
		{"empty(./comment())", []bool{true, true, true}},
		{"exists(./comment())", []bool{false, false, false}},
		{"./comment()", []bool{false, false, false}},
		{"empty(comment())", []bool{true, true, true}},
		{"exists(comment())", []bool{false, false, false}},
		{"comment()", []bool{false, false, false}},
		{"not(comment ( ))", []bool{true, true, true}},
		{"empty(.//comment()) and exists(.//d)", []bool{false, false, true}},
		{"exists(.//comment()) or exists(.//d)", []bool{false, false, true}},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			test := esCompile(t, tc.expr)
			for i, nodes := range subtrees {
				if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, acTally(test, nodes...), ValueBinding{}, time.Time{}); got != tc.holds[i] {
					t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, nodes, got, tc.holds[i])
				}
			}
		})
	}
}

// A comment step reads nothing: no `.`, no child, and no [Tally] counter of
// its own, so `empty(.//comment())` alone makes no Tally at all. Every row
// fails with the comment branch of ctaParser.selectedStepLength removed, and
// with ctaAssertionFacade.comments returning a ctaSelectedElements over the
// descendants named a.
func TestAssertionCommentStepReadsNothing(t *testing.T) {
	for _, expr := range []string{"empty(.//comment())", "comment()", "exists(./comment())"} {
		t.Run(expr, func(t *testing.T) {
			test := esCompile(t, expr)
			if test.ReadsContextItem() || test.ReadsChild(uq("comment")) || test.Tally() != nil {
				t.Errorf("(%s): ReadsContextItem() = %v, ReadsChild(comment) = %v, Tally() = %v, want false, false, nil",
					expr, test.ReadsContextItem(), test.ReadsChild(uq("comment")), test.Tally())
			}
		})
	}
}

// Every comment step outside the three spellings and the three positions
// still declines on the assertion façade — under fn:count, behind an axis
// spelled out, after a child step, rooted, in a predicate, read for its value,
// with an argument — and so does every other KindTest, [59] PITest among them.
func TestAssertionCommentStepElsewhereDeclines(t *testing.T) {
	for _, expr := range []string{
		"count(.//comment()) = 0", "count(comment()) = 0", "child::comment()", "exists(child::comment())",
		"empty(descendant::comment())", "empty(a/comment())", "empty(//comment())", "a[comment()]",
		".//comment() = 'x'", "string(comment()) = ''", "comment(1)", "empty(.//comment(), 1)",
		"empty(.//processing-instruction())", "processing-instruction()", "empty(.//processing-instruction(x))",
		"empty(.//text())", "empty(.//node())",
	} {
		if _, ok := CompileAssertionTest(asRecord(expr), seededTypes, asElementContent(t, false), asUses(t, map[string]string{"x": "int"}), esElems(t)); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined", expr)
		}
	}
}

// A comment step is the assertion façade's alone. A Type Alternative's {test}
// declines it in every spelling and position (ctaTypeAlternativeFacade.comments):
// with that method returning ctaEmptyValue every Type Alternative row but
// `empty(.//comment())` compiles, which declines at the call, the façade
// calling no library function. An assertions facet's reads its absent context
// item and Fails, never Holds (ctaFacetFacade.comments, err:XPDY0002,
// cvc-assertions-valid clause 1.2), fn:not over it included: every facet row
// is declined with the comment branch of ctaParser.selectedStepLength removed,
// and the three fn:empty and fn:not rows Hold with ctaFacetFacade.comments
// returning ctaEmptyValue.
func TestCommentStepOutsideTheAssertionFacade(t *testing.T) {
	for _, expr := range []string{"comment()", "./comment()", ".//comment()", "not(.//comment())", "empty(.//comment())"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
	str := asBuiltin(t, "string")
	for _, test := range []string{"comment()", "./comment()", ".//comment()", "not(.//comment())", "empty(.//comment())", "exists(.//comment())", "empty(comment())"} {
		if got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(test, ""), fcValue(t, str, "x")); got != value.AssertionFails {
			t.Errorf("FacetAssertions().Evaluate(%q) = %d, want Fails (%d): never Holds", test, got, value.AssertionFails)
		}
	}
}

// A quantifier's body over a typed child step parses under a façade embedding
// the assertion's (ctaChildRangeFacade), where `.` is still E — xpath20.md §3.9
// binds the range variable and leaves the focus alone — so a comment step there
// selects nothing too: over one b child, `some` with fn:empty over it holds and
// with fn:exists does not. Every row fails with the comment branch of
// ctaParser.selectedStepLength removed, and the two rows that hold fail with
// ctaAssertionFacade.comments returning ctaNoContextItem.
func TestAssertionCommentStepInQuantifierBody(t *testing.T) {
	children := asChildren(t, asChild{uq("b"), "string", "x", false})
	for _, tc := range []struct {
		expr  string
		holds bool
	}{
		{"some $v in b satisfies empty(.//comment())", true},
		{"some $v in b satisfies $v = 'x' and exists(comment())", false},
		{"some $v in b satisfies $v = 'x' and not(./comment())", true},
	} {
		t.Run(tc.expr, func(t *testing.T) {
			test := esCompile(t, tc.expr)
			if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), children, test.Tally(), ValueBinding{}, time.Time{}); got != tc.holds {
				t.Errorf("Evaluate(%q) over one b = %v, want %v", tc.expr, got, tc.holds)
			}
		})
	}
}
