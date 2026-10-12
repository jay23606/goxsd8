package xpath

import (
	"testing"
	"time"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive the assertion façade's fn:count call
// (xpath-functions.md §15.4.1): an assertion counts nodes of the subtree of E,
// the root of the instance cvc-assertion clause 1.3 builds, through the [Tally]
// its caller fills, and never reads a counted node's value.

// acNode is one node the caller reports to a Tally: an element node, or an
// attribute node where attribute is true, named name and depth levels below E.
// An element node is reported by its chain below E: path where it is non-nil,
// and otherwise depth-1 ancestors named acFiller above name, the empty chain —
// E itself — at a depth below 1; with attrs, the names of its attribute nodes.
// An attribute node is reported alone on an element whose chain is depth
// acFiller names, the empty chain — E itself — at depth 0.
type acNode struct {
	attribute bool
	depth     int
	name      xsd.QName
	path      []xsd.QName
	attrs     []xsd.QName
}

// acFiller names the ancestors acTally invents for an element reported by
// depth, a name no counted path below writes.
var acFiller = uq("anc")

// chain is the element chain n is reported by.
func (n acNode) chain() []xsd.QName {
	if n.path != nil {
		return n.path
	}
	var path []xsd.QName
	if n.attribute {
		for range n.depth {
			path = append(path, acFiller)
		}
		return path
	}
	for range n.depth - 1 {
		path = append(path, acFiller)
	}
	if n.depth >= 1 {
		path = append(path, n.name)
	}
	return path
}

// reported is the attribute names n is reported with.
func (n acNode) reported() []xsd.QName {
	if n.attribute {
		return []xsd.QName{n.name}
	}
	return n.attrs
}

// acPath is an element node reported by the chain of locals below E, each in no
// namespace.
func acPath(locals ...string) acNode {
	path := []xsd.QName{}
	for _, l := range locals {
		path = append(path, uq(l))
	}
	return acNode{path: path}
}

// acEl is an element node named local in no namespace, depth levels below E.
func acEl(depth int, local string) acNode { return acNode{depth: depth, name: uq(local)} }

// acAt is an attribute node named local in no namespace, belonging to the
// element depth levels below E.
func acAt(depth int, local string) acNode {
	return acNode{attribute: true, depth: depth, name: uq(local)}
}

// acTally is test's Tally with nodes reported to it, in the order written,
// each after the ancestors of its chain the reports before it leave unopened —
// those below the longest prefix its parent's chain shares with the chain
// reported last — reported with no attribute names, so the sequence is one
// pre-order of a tree, as [Tally] obliges: a chain a node shares with the
// report before it, or extends by one name, opens nothing. A second instance
// of an element already open, a sibling of the same name, is written as a node
// of its own.
func acTally(test AssertionTest, nodes ...acNode) *Tally {
	c := test.Tally()
	var last []xsd.QName
	for _, n := range nodes {
		chain := n.chain()
		for k := acSharedPrefix(last, chain) + 1; k < len(chain); k++ {
			c.Element(chain[:k], nil)
		}
		c.Element(chain, n.reported())
		last = chain
	}
	return c
}

// acSharedPrefix is how many leading names chain's parent chain, all of chain
// but its last name, shares with last.
func acSharedPrefix(last, chain []xsd.QName) int {
	k := 0
	for k < len(last) && k < len(chain)-1 && last[k] == chain[k] {
		k++
	}
	return k
}

// acCompile compiles expr for an E with element-only content whose attribute
// length is an xs:int and which declares no child element at all: a counted
// step consults neither AttributeTypes nor ElementTypes, so asNoElems types no
// name it counts.
func acCompile(t *testing.T, record xsd.XPathExpression) AssertionTest {
	t.Helper()
	test, ok := CompileAssertionTest(record, seededTypes, xsd.ElementContent{}, asUses(t, map[string]string{"length": "int"}), asNoElems)
	if !ok {
		t.Fatalf("CompileAssertionTest(%q): declined, want compiled", record.Expression())
	}
	return test
}

// fn:count counts the nodes its path selects in E's subtree (xpath20.md
// §3.2.4): `N` and `./N` the element children, `.//N` every element below E
// and never E itself, `@N` and `./@N` E's own attributes, and `.//@N` the
// attributes of E and of every element below it. `@length eq count(./ele)` is
// satisfied where length counts the children and charged where it does not —
// the satisfied row tells a count from a decline read as false. `.//e1` over an
// E itself named e1 counts its e1 child and grandchild and not E (reported at
// depth 0, which an element report selects nothing at). `.//@a` counts E's own
// @a with a grandchild's, so the count is 2. A name a counted step selects is
// typed nowhere: asNoElems declines every child step, and `zz` counts all the
// same. Every row declines at CompileAssertionTest, and fails, with fn:count
// declining (ctaParser.valueExpr handing it to constructorFunction).
func TestAssertionCountsSubtreeNodes(t *testing.T) {
	length := func(lexical string) TypedAttributes {
		return asValues(t, asTyped{uq("length"), "int", lexical})
	}
	for _, tc := range []struct {
		why    string
		expr   string
		attrs  TypedAttributes
		nodes  []acNode
		holds  bool
		record xsd.XPathExpression
	}{
		{why: "length counts the children", expr: "@length eq count(./ele)", attrs: length("2"),
			nodes: []acNode{acEl(1, "ele"), acEl(1, "ele")}, holds: true},
		{why: "length does not count the children", expr: "@length eq count(./ele)", attrs: length("3"),
			nodes: []acNode{acEl(1, "ele"), acEl(1, "ele")}},
		{why: "a grandchild is a descendant, E is not", expr: "count(.//e1) eq 2",
			nodes: []acNode{acEl(0, "e1"), acEl(1, "e1"), acEl(2, "e1")}, holds: true},
		{why: "E named e1 is not its own descendant", expr: "count(.//e1) eq 2",
			nodes: []acNode{acEl(0, "e1"), acEl(1, "e1")}},
		{why: "E's own attribute is in .//@a", expr: "count(.//@a) eq 2",
			nodes: []acNode{acAt(0, "a"), acAt(2, "a")}, holds: true},
		{why: "a grandchild's attribute alone is one", expr: "count(.//@a) eq 2",
			nodes: []acNode{acAt(2, "a")}},
		{why: "a child step counts no grandchild", expr: "count(e1) eq 1",
			nodes: []acNode{acEl(1, "e1"), acEl(2, "e1")}, holds: true},
		{why: "an attribute step counts E's own alone", expr: "count(@a) eq 1",
			nodes: []acNode{acAt(0, "a"), acAt(1, "a")}, holds: true},
		{why: "./@a is @a", expr: "count(./@a) eq 1",
			nodes: []acNode{acAt(0, "a"), acAt(1, "a")}, holds: true},
		{why: "an attribute named e1 is no element", expr: "count(e1) eq 0",
			nodes: []acNode{acAt(1, "e1")}, holds: true},
		{why: "an element named a is no attribute", expr: "count(.//@a) eq 0",
			nodes: []acNode{acEl(1, "a")}, holds: true},
		{why: "two paths, two counters", expr: "count(e1) eq 1 and count(.//e1) eq 2",
			nodes: []acNode{acEl(1, "e1"), acEl(2, "e1")}, holds: true},
		{why: "one path written twice, one counter", expr: "count(e1) eq 1 and count(./e1) = 1",
			nodes: []acNode{acEl(1, "e1")}, holds: true},
		{why: "an untyped name counts", expr: "count(zz) eq 1",
			nodes: []acNode{acEl(1, "zz")}, holds: true},
		{why: "a general comparison", expr: "count(e1) > 1",
			nodes: []acNode{acEl(1, "e1"), acEl(1, "e1")}, holds: true},
		{why: "a count's effective boolean value, nonzero", expr: "count(e1)",
			nodes: []acNode{acEl(1, "e1")}, holds: true},
		{why: "a count's effective boolean value, zero", expr: "count(e1)"},
		{why: "fn:not over a count", expr: "not(count(e1) eq 1)", holds: true},
		{why: "a present default namespace names the qualified child",
			record: ctaExprRecord("count(e1) eq 1", "urn:t"),
			nodes:  []acNode{{depth: 1, name: xsd.QName{Space: "urn:t", Local: "e1"}}}, holds: true},
		{why: "a present default namespace misses the unqualified child",
			record: ctaExprRecord("count(e1) eq 1", "urn:t"), nodes: []acNode{acEl(1, "e1")}},
		{why: "an unprefixed attribute step is in no namespace whatever the default",
			record: ctaExprRecord("count(@a) eq 1", "urn:t"), nodes: []acNode{acAt(0, "a")}, holds: true},
		{why: "a prefix names its binding",
			record: ctaExprRecord("count(.//t:e1) eq 1", "", "t", "urn:t"),
			nodes:  []acNode{{depth: 3, name: xsd.QName{Space: "urn:t", Local: "e1"}}}, holds: true},
	} {
		t.Run(tc.why, func(t *testing.T) {
			record := tc.record
			if tc.expr != "" {
				record = asRecord(tc.expr)
			}
			attrs := tc.attrs
			if attrs == nil {
				attrs = asValues(t)
			}
			test := acCompile(t, record)
			if got := test.Evaluate(backend(), seededTypes, asElem, attrs, asNoChildren, acTally(test, tc.nodes...), ValueBinding{}, time.Time{}); got != tc.holds {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", record.Expression(), tc.nodes, got, tc.holds)
			}
		})
	}
}

// A rooted fn:count argument raises err:XPDY0050 (xpath20.md §3.2): E has no
// document node above it, so `count(//e1) eq 2` is false over an E with
// exactly two e1 descendants, `count(//@a) eq 1` over one @a, and their fn:not
// too, the error propagating. Such a test counts nothing, so its Tally is nil.
// A rooted attribute step is admitted outside fn:count as well and raises the
// same. Every row declines, and fails, with fn:count declining; the `//@a`
// rows also with ctaParser.rootedPath admitting no attribute step.
func TestAssertionRootedCountRaises(t *testing.T) {
	attrs := asValues(t, asTyped{uq("a"), "string", "a"})
	children := asChildren(t, asChild{uq("e1"), "string", "present", false}, asChild{uq("e1"), "string", "present", false})
	for _, expr := range []string{
		"count(//e1) eq 2", "count(/e1) eq 1", "not(count(//e1) eq 2)", "count(//e1)",
		"count(//@a) eq 1", "count(/@a) eq 1", "not(count(//@a) eq 1)",
		"/@a = 'a'", "//@a", "not(//@a)", "//@a eq 'a'",
	} {
		test, ok := CompileAssertionTest(asRecord(expr), seededTypes, xsd.ElementContent{}, asUses(t, map[string]string{"a": "string"}), asChildTypes(t))
		if !ok {
			t.Errorf("CompileAssertionTest(%q): declined, want compiled", expr)
			continue
		}
		if c := test.Tally(); c != nil {
			t.Errorf("(%q).Tally() = %v, want nil: a rooted path counts nothing", expr, c)
		}
		if test.Evaluate(backend(), seededTypes, asElem, attrs, children, nil, ValueBinding{}, time.Time{}) {
			t.Errorf("Evaluate(%q) = true, want false: the leading slash raises err:XPDY0050", expr)
		}
	}
}

// Evaluate answers false for a Tally the test did not make, whatever the tree
// holds: `count(e1) eq 0` is satisfied with its own empty Tally and false with
// none, with one counting another path, and with one counting a further path;
// `'a' = 'a'`, which counts nothing, is satisfied with no Tally and false with
// one, a zero-value Tally included. The last three false rows are satisfied
// instead with Tally.fits' check removed from Evaluate, and the zero-value row
// with only fits' empty-paths guard removed; the other two stay false without
// the check, the count node finding no counter for its path (ctaCountItem).
func TestAssertionEvaluateRefusesAMismatchedTally(t *testing.T) {
	counting := acCompile(t, asRecord("count(e1) eq 0"))
	other := acCompile(t, asRecord("count(e2) eq 0"))
	wider := acCompile(t, asRecord("count(e1) eq 0 and count(e2) eq 0"))
	plain := acCompile(t, asRecord("'a' = 'a'"))
	for _, tc := range []struct {
		why    string
		test   AssertionTest
		counts *Tally
		holds  bool
	}{
		{"its own Tally", counting, counting.Tally(), true},
		{"no Tally where it counts", counting, nil, false},
		{"a Tally counting another path", counting, other.Tally(), false},
		{"a Tally counting a further path", counting, wider.Tally(), false},
		{"no Tally where it counts nothing", plain, nil, true},
		{"a Tally where it counts nothing", plain, counting.Tally(), false},
		{"a zero-value Tally where it counts nothing", plain, &Tally{}, false},
	} {
		if got := tc.test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, tc.counts, ValueBinding{}, time.Time{}); got != tc.holds {
			t.Errorf("%s: Evaluate = %v, want %v", tc.why, got, tc.holds)
		}
	}
}

// AssertionTest.Tally is nil for a test that counts nothing and fresh on every
// call otherwise, so filling one Tally leaves the next untouched and the
// compiled test immutable. Tally's methods are total: an empty element chain
// selects no element, and a nil Tally takes any report.
func TestAssertionTallyIsFreshAndTotal(t *testing.T) {
	if c := acCompile(t, asRecord("@length eq 1")).Tally(); c != nil {
		t.Errorf("Tally() of a test with no fn:count = %v, want nil", c)
	}
	test := acCompile(t, asRecord("count(.//e1) eq 0 and count(.//@a) eq 0"))
	acTally(test, acEl(1, "e1"), acAt(0, "a"))
	fresh := acTally(test, acEl(0, "e1"), acEl(-1, "e1"))
	if !test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, fresh, ValueBinding{}, time.Time{}) {
		t.Error("Evaluate over a fresh Tally given only out-of-range depths = false, want true")
	}
	var none *Tally
	none.Element([]xsd.QName{uq("e1")}, []xsd.QName{uq("a")})
}

// ReadsAttributesAt answers per counted path: `@a` selects attributes of E
// alone, `.//@a` those of E and of every element below it, and `e1`, `.//e1`
// and the child path `e1/e1` select none at any depth. `c[@a]` reads the
// attribute names of E's children, at depth 1 alone, and a union reads them
// wherever an operand does. A test that counts nothing has a nil Tally, which
// answers false, and so does every Tally at a depth below 0. The two `c[@a]`
// rows fail at depth 1 with ctaFilteredChildren.selectsAttributesAt false, and
// the `c | @a` row at depth 0 with ctaUnion.selectsAttributesAt asking its
// first operand alone.
func TestTallyReadsAttributesAt(t *testing.T) {
	for _, tc := range []struct {
		expr string
		want map[int]bool
	}{
		{"count(@a) eq 0", map[int]bool{-1: false, 0: true, 1: false, 2: false}},
		{"count(.//@a) eq 0", map[int]bool{-1: false, 0: true, 1: true, 2: true}},
		{"count(e1) eq 0", map[int]bool{-1: false, 0: false, 1: false, 2: false}},
		{"count(.//e1) eq 0", map[int]bool{-1: false, 0: false, 1: false, 2: false}},
		{"exists(e1/e1)", map[int]bool{-1: false, 0: false, 1: false, 2: false}},
		{"count(c[@a]) eq 0", map[int]bool{-1: false, 0: false, 1: true, 2: false}},
		{"count(c | e1) eq 0", map[int]bool{-1: false, 0: false, 1: false, 2: false}},
		{"count(c | @a) eq 0", map[int]bool{-1: false, 0: true, 1: false, 2: false}},
		{"count(c[@a] | e1) eq 0", map[int]bool{-1: false, 0: false, 1: true, 2: false}},
		{"@length eq 1", map[int]bool{-1: false, 0: false, 1: false, 2: false}},
	} {
		c := acCompile(t, asRecord(tc.expr)).Tally()
		for depth := -1; depth <= 2; depth++ {
			if got := c.ReadsAttributesAt(depth); got != tc.want[depth] {
				t.Errorf("Tally of %q: ReadsAttributesAt(%d) = %v, want %v", tc.expr, depth, got, tc.want[depth])
			}
		}
	}
}

// fn:count's path argument is one QName step, a rooted one, a child step
// filtered by one predicate, or a union (the predicate and union rows below):
// every other path declines, a positional predicate among them, and so does
// `.//` or a child step behind `./` in a comparison operand — an attribute
// step behind `./` is `@N` there (TestAssertionOwnAttributeStep) — and a cast
// from a count, which [16] ta-SimpleValue, the operand of the cast spelling,
// does not admit.
func TestCompileAssertionTestDeclinesCounts(t *testing.T) {
	for _, tc := range []struct{ expr, why string }{
		{"count(*) = 0", "a wildcard NameTest"},
		{"count(@*) = 0", "an attribute wildcard"},
		{"count(.//*) = 0", "a descendant wildcard"},
		{"count(//*) = 0", "a rooted wildcard"},
		{"count(//@*) = 0", "a rooted attribute wildcard"},
		{"count(e1/e1) = 0", "a path of two steps"},
		{"count(e1//e1) = 0", "a path through descendant-or-self"},
		{"count(//e1/e1) = 0", "a rooted path of two steps"},
		{"count(.) = 1", "the context item alone"},
		{"count(..) = 1", "the parent step"},
		{"count(e1[1]) = 1", "a positional predicate"},
		{"count(child::e1) = 1", "the unabbreviated child axis"},
		{"count(attribute::a) = 1", "the unabbreviated attribute axis"},
		{"count() = 0", "no argument"},
		{"count(e1, e1) = 0", "two arguments"},
		{"count(./) = 0", "a slash with no step after it"},
		{"count(.//) = 0", "a double slash with no step after it"},
		{"count(/) = 0", "a bare slash"},
		{"count(@p:a) = 0", "an unbound prefix"},
		{".//e1 = 'a'", "a descendant step in a comparison"},
		{"./e1 = 'a'", "a child step behind ./ in a comparison"},
		{"count(e1) cast as xs:string = '1'", "a cast from an xs:integer count"},
	} {
		if _, ok := CompileAssertionTest(asRecord(tc.expr), seededTypes, xsd.ElementContent{}, asUses(t, map[string]string{"length": "int"}), asChildTypes(t)); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
}

// fn:count over a path is the assertion façade's alone: a Type Alternative's
// {test} declines it under §3.12.6 clause 3, and an assertions facet's declines
// it (ctaFacetFacade.count) rather than failing. A rooted attribute step reads the
// facet's absent context item and fails it (err:XPDY0002), and a Type
// Alternative declines it as it declines every rooted path. The relative count
// rows compile, and fail, with either façade's count admitting the call; the
// rooted one is declined there by the rooted path first. The `//@a` facet row
// declines instead, and fails, with ctaParser.rootedPath admitting no
// attribute step.
func TestCountDeclinesOutsideTheAssertionFacade(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, expr := range []string{"count(@a) = 0", "count(.//e1) = 0", "count(//e1) = 0", "//@a = 'x'"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
	}
	for _, tc := range []struct {
		test string
		want value.AssertionOutcome
	}{
		{"count(@a) = 0", value.AssertionDeclined},
		{"count(.//e1) = 0", value.AssertionDeclined},
		{"count(//e1) = 0", value.AssertionDeclined},
		{"//@a = 'x'", value.AssertionFails},
	} {
		if got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(tc.test, ""), fcValue(t, str, "x")); got != tc.want {
			t.Errorf("FacetAssertions().Evaluate(%q) = %d, want %d", tc.test, got, tc.want)
		}
	}
}

// acElAt is an element node named local in no namespace, depth levels below E,
// whose attribute nodes are named attrs, each in no namespace.
func acElAt(depth int, local string, attrs ...string) acNode {
	n := acEl(depth, local)
	for _, a := range attrs {
		n.attrs = append(n.attrs, uq(a))
	}
	return n
}

// acE is E itself, reported by the empty chain, whose attribute nodes are
// named attrs, each in no namespace.
func acE(attrs ...string) acNode {
	n := acPath()
	for _, a := range attrs {
		n.attrs = append(n.attrs, uq(a))
	}
	return n
}

// acDecides fails unless expr compiles under acCompile and evaluates to want
// over a Tally given nodes.
func acDecides(t *testing.T, expr string, want bool, nodes ...acNode) {
	t.Helper()
	test := acCompile(t, asRecord(expr))
	if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), asNoChildren, acTally(test, nodes...), ValueBinding{}, time.Time{}); got != want {
		t.Errorf("Evaluate(%q) over %v = %v, want %v", expr, nodes, got, want)
	}
}

// acCounters fails unless expr compiles under acCompile to a test whose Tally
// holds want counters.
func acCounters(t *testing.T, expr string, want int) {
	t.Helper()
	if c := acCompile(t, asRecord(expr)).Tally(); c == nil || len(c.counters) != want {
		t.Errorf("(%s).Tally() = %v, want %d counters", expr, c, want)
	}
}

// A predicate that is a conjunction of attribute-existence tests filters a
// counted child step (xpath20.md §3.2.2): `c[@a and @b]` selects the children
// named c whose attribute nodes include both, which the Tally reads off the
// names each Tally.Element report carries. A c carrying @a alone is not
// selected; neither is a grandchild c, E itself, or an element a carrying an
// attribute c. A further attribute is no matter, nor is the order the
// conjunction names them in, and a name written twice is required once.
// `c[@n]` is node-valued, so it is the existence test and not a positional
// one. Every row declines, and fails, with ctaParser.predicate declining every
// predicate; the "only @a", "required whole" and "one of two children" rows
// fail with ctaFilteredChildren.selectsElement asking for any one required
// name rather than all.
func TestAssertionCountsChildrenFilteredByAttributes(t *testing.T) {
	for _, tc := range []struct {
		why   string
		expr  string
		nodes []acNode
		holds bool
	}{
		{"both attributes", "count(c[@a and @b]) = 1", []acNode{acElAt(1, "c", "a", "b")}, true},
		{"only @a", "count(c[@a and @b]) = 1", []acNode{acElAt(1, "c", "a")}, false},
		{"the conjunction is required whole", "count(c[@a and @b]) = 0", []acNode{acElAt(1, "c", "b"), acElAt(1, "c", "a")}, true},
		{"a further attribute", "count(c[@a and @b]) = 1", []acNode{acElAt(1, "c", "x", "b", "a")}, true},
		{"one of two children", "count(c[@a and @b]) = 1", []acNode{acElAt(1, "c", "a", "b"), acElAt(1, "c", "b")}, true},
		{"two children", "count(c[@a and @b]) = 2", []acNode{acElAt(1, "c", "a", "b"), acElAt(1, "c", "b", "a")}, true},
		{"a grandchild is no child", "count(c[@a and @b]) = 0", []acNode{acElAt(2, "c", "a", "b")}, true},
		{"E itself is no child", "count(c[@a and @b]) = 0", []acNode{acE("a", "b")}, true},
		{"an element a carrying an attribute c", "count(c[@a]) = 0", []acNode{acElAt(1, "a", "c")}, true},
		{"the order of the conjunction", "count(c[@b and @a]) = 1", []acNode{acElAt(1, "c", "a", "b")}, true},
		{"a name written twice", "count(c[@a and @a]) = 1", []acNode{acElAt(1, "c", "a")}, true},
		{"./c filters as c does", "count(./c[@a]) = 1", []acNode{acElAt(1, "c", "a"), acElAt(1, "c")}, true},
		{"c[@n] is an existence test", "count(c[@n]) = 1", []acNode{acElAt(1, "c", "n"), acElAt(1, "c")}, true},
	} {
		t.Run(tc.why, func(t *testing.T) { acDecides(t, tc.expr, tc.holds, tc.nodes...) })
	}
}

// One counter serves a filtered step however its conjunction is ordered or
// repeated, and a filtered step and the bare one keep two. The first two rows
// hold two counters with ctaFilteredChildren.same reading its names in order
// or ctaFilteredChildrenOf keeping a repeated one.
func TestAssertionFilteredStepsShareCounters(t *testing.T) {
	acCounters(t, "count(c[@a and @b]) = 1 and count(c[@b and @a]) = 1", 1)
	acCounters(t, "count(c[@a]) = 1 and count(c[@a and @a]) = 1", 1)
	acCounters(t, "count(c[@a]) = 1 and count(c) = 1", 2)
	acCounters(t, "count(c[@a]) = 1 and count(c[@b]) = 1", 2)
	acCounters(t, "count(c[@a]) = 1 and count(d[@a]) = 1", 2)
}

// A union in fn:count's argument (xpath20.md §3.3.3) counts each node ONCE,
// whichever operands select it: `count(@a | @b) = 1` holds with one of the two
// and not with both; `count(e | .//e)` over an e child and an e grandchild is
// 2, where summing per-operand counts would be 3; `count(e[@a] | e[@b])` over
// one e carrying both is 1. `union` is `|`. The real fixtures' shapes compile
// and decide too. Every row declines, and fails, with ctaParser.countArgument
// reading one operand; the two overlap rows are false instead with ctaSelected
// summing ctaUnion's operands rather than asking whether any selects a node.
func TestAssertionCountsAUnionOnce(t *testing.T) {
	for _, tc := range []struct {
		why   string
		expr  string
		nodes []acNode
		holds bool
	}{
		{"one of two attributes", "count(@a | @b) = 1", []acNode{acE("a")}, true},
		{"the other of two attributes", "count(@a | @b) = 1", []acNode{acE("b", "x")}, true},
		{"both attributes", "count(@a | @b) = 1", []acNode{acE("a", "b")}, false},
		{"neither attribute", "count(@a | @b) = 0", []acNode{acE("x"), acElAt(1, "a", "a")}, true},
		{"a child and a grandchild selected by two operands", "count(e | .//e) = 2", []acNode{acEl(1, "e"), acPath("e", "e")}, true},
		{"one child selected by two filtered operands", "count(e[@a] | e[@b]) = 1", []acNode{acElAt(1, "e", "a", "b")}, true},
		{"two children each selected by one filtered operand", "count(e[@a] | e[@b]) = 2", []acNode{acElAt(1, "e", "a"), acElAt(1, "e", "b"), acElAt(1, "e")}, true},
		{"an element and an attribute", "count(e | @a) = 2", []acNode{acE("a"), acEl(1, "e")}, true},
		{"union is |", "count(@a union @b) = 2", []acNode{acE("a", "b")}, true},
		{"three operands", "count(@a | @b | e) = 3", []acNode{acE("a", "b"), acEl(1, "e")}, true},
		{"d4_3_15 timer, one", "count(@time | @iterations) = 1", []acNode{acE("time")}, true},
		{"d4_3_15 timer, two", "count(@time | @iterations) = 1", []acNode{acE("time", "iterations")}, false},
		{"d4_3_15 parent, a grandchild child element", "count(child[@name and @dob] | grandchild[@name and @dob]) = 1",
			[]acNode{acElAt(1, "grandchild", "name", "dob")}, true},
		{"d4_3_15 parent, both", "count(child[@name and @dob] | grandchild[@name and @dob]) = 1",
			[]acNode{acElAt(1, "child", "name", "dob"), acElAt(1, "grandchild", "dob", "name")}, false},
		{"d4_3_15 parent, one without dob", "count(child[@name and @dob] | grandchild[@name and @dob]) = 1",
			[]acNode{acElAt(1, "child", "name", "dob"), acElAt(1, "grandchild", "name")}, true},
	} {
		t.Run(tc.why, func(t *testing.T) { acDecides(t, tc.expr, tc.holds, tc.nodes...) })
	}
}

// A union keeps one counter per distinct operand SET: `e | e` is `e` itself and
// shares its counter, `@a | @b` and `@b | @a` are one, `e | @a | e` is `@a |
// e`; no operand is read for what it selects, so `e | .//e` and `.//e` keep
// two. Rows hold a counter more with ctaUnionOf keeping a repeated operand
// (the first and third), not collapsing to a single one (the first), or with
// ctaUnion.same reading its operands in order (the second to fourth).
func TestAssertionUnionCounters(t *testing.T) {
	acCounters(t, "count(e | e) = 1 and count(e) = 1", 1)
	acCounters(t, "count(@a | @b) = 1 and count(@b | @a) = 1", 1)
	acCounters(t, "count(e | @a | e) = 1 and count(@a | e) = 1", 1)
	acCounters(t, "count(e[@a] | e[@b]) = 1 and count(e[@b] | e[@a]) = 1", 1)
	acCounters(t, "count(e | .//e) = 1 and count(.//e) = 1", 2)
}

// A predicate fn:count admits is an attribute-existence conjunction on a child
// step: every other one declines, and so does a union operand that is rooted
// or a path of two steps, and a predicate or a union outside fn:count. A
// numeric predicate is positional, which an order-free Tally cannot decide,
// and position() and last() decline in a predicate, whose façade calls no
// library function (guard).
func TestCompileAssertionTestDeclinesPredicatesAndUnions(t *testing.T) {
	for _, tc := range []struct{ expr, why string }{
		{"count(c[@a or @b]) = 1", "a disjunction"},
		{"count(c[not(@a)]) = 1", "an fn:not"},
		{"count(c[@a = 1]) = 1", "an attribute atomized"},
		{"count(c[@a + 0]) = 1", "an attribute in arithmetic"},
		{"count(c[@*]) = 1", "an attribute wildcard"},
		{"count(c[@p:a]) = 1", "an unbound prefix"},
		{"count(c[1]) = 1", "a numeric predicate"},
		{"count(c[1 + 0]) = 1", "a numeric arithmetic predicate"},
		{"count(c[position() = 1]) = 1", "position()"},
		{"count(c[last()]) = 1", "last()"},
		{"count(c[@a][@b]) = 1", "two predicates"},
		{"count(c[]) = 1", "an empty predicate"},
		{"count(c[@a) = 1", "an unclosed predicate"},
		{"count(.//c[@a]) = 1", "a predicate on a descendant step"},
		{"count(@a[@b]) = 1", "a predicate on an attribute step"},
		{"count(a/c[@a]) = 1", "a predicate after a path"},
		{"count(/c[@a]) = 1", "a predicate on a rooted step"},
		{"count(/e | e) = 1", "a rooted union operand"},
		{"count(e | //e) = 1", "a rooted second union operand"},
		{"count(a/b | e) = 1", "a child path union operand"},
		{"count(e |) = 1", "a union with no right operand"},
		{"count(| e) = 1", "a union with no left operand"},
		{"exists(c[@a])", "a predicate in fn:exists"},
		{"c[@a]", "a predicate as an effective boolean value"},
		{"exists(@a | @b)", "a union in fn:exists"},
		{"@a | @b", "a union as an effective boolean value"},
	} {
		if _, ok := CompileAssertionTest(asRecord(tc.expr), seededTypes, xsd.ElementContent{}, asUses(t, map[string]string{"a": "int", "b": "int"}), asChildTypes(t)); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
}

// vpElems is E's child element types for the value-predicate rows: white is
// whiteType, e1 an xs:string, n an xs:int.
func vpElems(t *testing.T, whiteType xsd.TypeDefinition) ElementTypes {
	t.Helper()
	return asElems(map[xsd.QName]xsd.TypeDefinition{
		uq("white"): whiteType, uq("e1"): asBuiltin(t, "string"), uq("n"): asBuiltin(t, "int"),
	})
}

// vpCompile compiles expr under vpElems(whiteType), for an E with element-only
// content and an xs:int attribute a, reporting whether it compiled.
func vpCompile(t *testing.T, expr string, whiteType xsd.TypeDefinition) (AssertionTest, bool) {
	t.Helper()
	return CompileAssertionTest(asRecord(expr), seededTypes, xsd.ElementContent{}, asUses(t, map[string]string{"a": "int"}), vpElems(t, whiteType))
}

// vpWhite is a child named white of type typ holding lexical.
func vpWhite(typ, lexical string) asChild { return asChild{uq("white"), typ, lexical, false} }

// A predicate that reads the candidate's VALUE filters a counted child step
// (xpath20.md §3.2.2): `.` inside it is each white child in turn, typed as a
// child step naming white is, so `count(white[. = 'oo']) lt 2` is false over
// two whites holding "oo" and true where one holds anything else. The count
// reads ChildElements, not the Tally: ReadsChild(white) is true and the test
// has no Tally. A child of another name is not a candidate. A ·nilled· white
// is a node with no value (xpath-datamodel §6.2.4), so `. = 'oo'` is false
// for it and its fn:not true, no error raised. and, or, fn:not, a value
// comparison, arithmetic and a cast are admitted over the candidate. Every
// row declines, and fails, with ctaParser.predicate declining a predicate that
// is not an attribute-existence conjunction, and so does the ReadsChild check
// with ctaMatchingChildren.readsChild false; every row whose predicate is
// false for some candidate — "one of two", the ·nilled· rows, or, and, the
// value comparison, the arithmetic and "beside a counted step" — fails with
// ctaMatchingChildren.nodes counting every candidate.
func TestAssertionCountsChildrenFilteredByValue(t *testing.T) {
	str := asBuiltin(t, "string")
	nilled := asChild{name: uq("white"), nilled: true}
	for _, tc := range []struct {
		why      string
		expr     string
		children []asChild
		holds    bool
	}{
		{"two oo", "count(white[. = 'oo']) lt 2", []asChild{vpWhite("string", "oo"), vpWhite("string", "oo")}, false},
		{"one of two", "count(white[. = 'oo']) lt 2", []asChild{vpWhite("string", "oo"), vpWhite("string", "o")}, true},
		{"none", "count(white[. = 'oo']) lt 2", nil, true},
		{"another name is no candidate", "count(white[. = 'oo']) = 1",
			[]asChild{{uq("e1"), "string", "oo", false}, vpWhite("string", "oo")}, true},
		{"./white is white", "count(./white[. = 'oo']) = 2", []asChild{vpWhite("string", "oo"), vpWhite("string", "oo")}, true},
		{"a nilled white is not counted", "count(white[. = 'oo']) = 1", []asChild{nilled, vpWhite("string", "oo")}, true},
		{"a nilled white under fn:not", "count(white[not(. = 'oo')]) = 1", []asChild{nilled, vpWhite("string", "oo")}, true},
		{"or", "count(white[. = 'oo' or . = 'x']) = 2", []asChild{vpWhite("string", "x"), vpWhite("string", "oo"), vpWhite("string", "y")}, true},
		{"and", "count(white[. != 'oo' and . != 'x']) = 1", []asChild{vpWhite("string", "x"), vpWhite("string", "oo"), vpWhite("string", "y")}, true},
		{"a value comparison", "count(white[. eq 'oo']) = 1", []asChild{vpWhite("string", "x"), vpWhite("string", "oo")}, true},
		{"a cast", "count(white[xs:string(.) = 'oo']) = 1", []asChild{vpWhite("string", "oo")}, true},
		{"arithmetic over an xs:int", "count(n[. + 1 = 3]) = 1", []asChild{{uq("n"), "int", "2", false}, {uq("n"), "int", "3", false}}, true},
		{"beside a counted step", "count(white[. = 'oo']) = 1 and count(white) = 2", []asChild{vpWhite("string", "oo"), vpWhite("string", "x")}, true},
	} {
		t.Run(tc.why, func(t *testing.T) {
			test, ok := vpCompile(t, tc.expr, str)
			if !ok {
				t.Fatalf("CompileAssertionTest(%q): declined, want compiled", tc.expr)
			}
			counts := test.Tally()
			for _, c := range tc.children {
				counts.Element([]xsd.QName{c.name}, nil)
			}
			if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), asChildren(t, tc.children...), counts, ValueBinding{}, time.Time{}); got != tc.holds {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.children, got, tc.holds)
			}
		})
	}
	test, ok := vpCompile(t, "count(white[. = 'oo']) lt 2", str)
	if !ok {
		t.Fatal("CompileAssertionTest(count(white[. = 'oo']) lt 2): declined, want compiled")
	}
	if !test.ReadsChild(uq("white")) || test.ReadsChild(uq("e1")) || test.Tally() != nil {
		t.Errorf("ReadsChild(white) = %v, ReadsChild(e1) = %v, Tally() = %v, want true, false and nil",
			test.ReadsChild(uq("white")), test.ReadsChild(uq("e1")), test.Tally())
	}
}

// A predicate that raises over any candidate makes the whole count raise, and
// the assertion false — never a candidate left uncounted (cvc-assertion,
// §3.13.4.1). Over an xs:int white, `. = 'oo'` is err:XPTY0004 (B.2 has no
// xs:int and xs:string row): one white makes `count(white[. = 'oo']) lt 2`
// false, none leaves it true, since no candidate is evaluated. `. div 0` over
// an xs:int is err:FOAR0001, so `count(n[. div 0 = 1]) = 0` is false over one
// n, where an error read as no match would hold, and so is its fn:not. The
// first and third rows hold instead with ctaMatchingChildren.nodes passing
// over a candidate whose predicate raised.
func TestAssertionValuePredicateErrorRaisesTheCount(t *testing.T) {
	i := asBuiltin(t, "int")
	for _, tc := range []struct {
		why      string
		expr     string
		children []asChild
		holds    bool
	}{
		{"an xs:int white against 'oo'", "count(white[. = 'oo']) lt 2", []asChild{vpWhite("int", "3")}, false},
		{"no white", "count(white[. = 'oo']) lt 2", nil, true},
		{"a division by zero", "count(n[. div 0 = 1]) = 0", []asChild{{uq("n"), "int", "2", false}}, false},
		{"a division by zero under fn:not", "not(count(n[. div 0 = 1]) = 0)", []asChild{{uq("n"), "int", "2", false}}, false},
		{"a ·nilled· n raises nothing", "count(n[. div 0 = 1]) = 0", []asChild{{name: uq("n"), nilled: true}}, true},
	} {
		t.Run(tc.why, func(t *testing.T) {
			test, ok := vpCompile(t, tc.expr, i)
			if !ok {
				t.Fatalf("CompileAssertionTest(%q): declined, want compiled", tc.expr)
			}
			if got := test.Evaluate(backend(), seededTypes, asElem, asValues(t), asChildren(t, tc.children...), nil, ValueBinding{}, time.Time{}); got != tc.holds {
				t.Errorf("Evaluate(%q) over %v = %v, want %v", tc.expr, tc.children, got, tc.holds)
			}
		})
	}
}

// A value predicate types `.` as a child step naming the candidate does, so a
// white with no single simple typed value declines — element-only, mixed,
// empty, ·special·, list or union — and so does every other value predicate
// outside the admitted shape: one not directly on a child step, outside
// fn:count, in a union, mixing `.` with an attribute, reading another node,
// `$value` or an fn:count, calling a library function, or whose root is not a
// comparison, which may be numeric and so positional. The type rows and `zz`
// compile, and fail, with ctaParser.valuePredicate reading `.` as xs:string
// where the child step declines or reads the child untyped, as a mixed one; the
// five bare-value rows but the library call's with ctaComparisonRooted
// admitting ctaEffectiveBoolean; the two union rows with ctaUnionOf admitting a
// value-filtered operand. The rest are guards.
func TestCompileAssertionTestDeclinesValuePredicates(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, tc := range []struct {
		why   string
		white xsd.TypeDefinition
	}{
		{"element-only", asComplex(t, "ElementOnly", asElementContent(t, false))},
		{"mixed", asComplex(t, "Mixed", asElementContent(t, true))},
		{"empty", asComplex(t, "Empty", xsd.EmptyContent{})},
		{"·special·", asBuiltin(t, "anySimpleType")},
		{"list", asList(t, "Ints", ctaBuiltin("int"))},
		{"union", asUnion(t)},
	} {
		if _, ok := vpCompile(t, "count(white[. = 'oo']) lt 2", tc.white); ok {
			t.Errorf("CompileAssertionTest over the %s white: compiled, want declined", tc.why)
		}
	}
	for _, tc := range []struct{ expr, why string }{
		{"count(.//white[. = 'oo']) = 1", "a descendant step"},
		{"count(a/white[. = 'oo']) = 1", "a path of two steps"},
		{"exists(white[. = 'oo'])", "fn:exists"},
		{"white[. = 'oo']", "an effective boolean value"},
		{"white[. = 'oo'] = 'oo'", "a comparison operand"},
		{"count(white[. = 'x'] | e1)", "a union operand"},
		{"count(e1 | white[. = 'x'])", "a second union operand"},
		{"count(white[. = 'x' and @a])", "`.` with an attribute"},
		{"count(white[. = e1])", "a child step"},
		{"count(white[. = $value])", "$value"},
		{"count(white[count(e1) = 1])", "an fn:count"},
		{"count(white[. = count(white[. = 'x'])])", "a nested predicate"},
		{"count(white[string-length(.) = 2])", "a library function"},
		{"count(white[.])", "a bare `.`"},
		{"count(white[string-length(.)])", "a bare library call"},
		{"count(white[1])", "a numeric literal"},
		{"count(n[. + 0])", "bare arithmetic"},
		{"count(white[. = 'x' and .])", "a bare `.` under and"},
		{"count(white[not(.)])", "a bare `.` under fn:not"},
		{"count(white[./x])", "a step below the candidate"},
		{"count(white[position() = 1])", "position()"},
		{"count(white[last()])", "last()"},
		{"count(zz[. = 'x'])", "a child with no type"},
		{".", "`.` at the top level"},
	} {
		if _, ok := vpCompile(t, tc.expr, str); ok {
			t.Errorf("CompileAssertionTest(%q): compiled, want declined (%s)", tc.expr, tc.why)
		}
	}
}

// A predicate is the assertion façade's alone: a Type Alternative's {test}
// declines every one, as it declines fn:count, and an assertions facet's
// declines a counted one, reaching no predicate it could evaluate.
func TestPredicatesDeclineOutsideTheAssertionFacade(t *testing.T) {
	str := asBuiltin(t, "string")
	for _, expr := range []string{"count(c[@a]) = 0", "count(white[. = 'x']) = 0", "count(@a | @b) = 0"} {
		if _, ok := CompileCTATest(ctaExprRecord(expr, ""), seededTypes); ok {
			t.Errorf("CompileCTATest(%q): compiled, want declined", expr)
		}
		if got := FacetAssertions(time.Time{}).Evaluate(backend(), seededTypes, str, ctaExprRecord(expr, ""), fcValue(t, str, "x")); got != value.AssertionDeclined {
			t.Errorf("FacetAssertions().Evaluate(%q) = %d, want %d", expr, got, value.AssertionDeclined)
		}
	}
}
