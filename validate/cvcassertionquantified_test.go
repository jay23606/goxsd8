package validate

import (
	"testing"

	"github.com/kud360/goxsd8/xsd"
)

// The fixtures below drive a quantified {test} over <e1>'s children (xpath20.md
// §3.9) through the walk, on ccSchemaText's RootType: the sibling form
// `$c/following-sibling::*[1][self::M]`, which the walk's reports at depth 1
// decide, and the attribute form `$c/@a`, which reads their attribute names.

// ccSpace is a whitespace-only run of text at line, which RootType's
// element-only content admits.
func ccSpace(line int) Child { return TextChild(&testText{data: "\n  ", loc: loc(line, 1)}) }

// `following-sibling::*` selects ELEMENT siblings alone (§3.2.1.2), so a text
// run between two e1, or after the last one, changes nothing: e1, text, e1
// still holds two adjacent e1 and is charged, and an e1 followed by text alone
// is E's last child element and is not. A child of an e1 is no sibling. An
// undetermined <u> or a ·skipped· {urn:skip}y is an element sibling like any
// other, and the sibling form reads no attribute name, so neither declines
// it. The validator drops comments before the walk (Child), so a trailing
// comment is the trailing-text row's report stream. Every row is declined
// instead, and fails, with xpath's quantifier declining; with
// ctaChildrenFollowedBy.selectsAttributesAt true at depth 1, the three rows
// holding a u are declined instead, and fail.
func TestAssertionQuantifiedSiblingsThroughTheWalk(t *testing.T) {
	const noAdjacent = "every $c in e1 satisfies not($c/following-sibling::*[1][self::e1])"
	e1, u, y := local("e1"), local("u"), xsd.QName{Space: "urn:skip", Local: "y"}
	k := ccAttr(local("k"), "1", 3)
	for _, tc := range []struct {
		why, test string
		root      *testElement
		charged   bool
	}{
		{"e1, text, e1", noAdjacent,
			ccRoot(nil, ccNode(e1, 2, nil), ccSpace(3), ccNode(e1, 4, nil)), true},
		{"e1, e1", noAdjacent,
			ccRoot(nil, ccNode(e1, 2, nil), ccNode(e1, 3, nil)), true},
		{"an e1 last, then trailing text", noAdjacent,
			ccRoot(nil, ccNode(e1, 2, nil), ccSpace(3)), false},
		{"an e1 whose own child is an e1, then u", noAdjacent,
			ccRoot(nil, ccNode(e1, 2, nil, ccNode(e1, 3, nil)), ccNode(u, 4, []Attribute{k})), false},
		{"e1, then an undetermined u", noAdjacent,
			ccRoot(nil, ccNode(e1, 2, nil), ccNode(u, 3, []Attribute{k})), false},
		{"e1 followed by u, decided", "some $c in e1 satisfies $c/following-sibling::*[1][self::u]",
			ccRoot(nil, ccNode(e1, 2, nil), ccNode(u, 3, []Attribute{k})), false},
		{"e1 followed by a skipped y", "some $c in e1 satisfies $c/following-sibling::*[1][self::s:y]",
			ccRoot(nil, ccNode(e1, 2, nil), ccSpace(3), ccNode(y, 4, nil)), false},
		{"e1, e1, then a skipped y: the first e1 is not followed by y", "every $c in e1 satisfies $c/following-sibling::*[1][self::s:y]",
			ccRoot(nil, ccNode(e1, 2, nil), ccNode(e1, 3, nil), ccNode(y, 4, nil)), true},
	} {
		t.Run(tc.why, func(t *testing.T) {
			ccDecided(t, aAssess(t, ccSchema(t, tc.test), tc.root), tc.charged, tc.test)
		})
	}
}

// `$c/@a` reads the attribute names of each e1 child, carried or ·defaulted·,
// so it holds where every e1 carries a and is charged where one lacks it; an
// undetermined <u> among the children has ·defaulted attributes· the walk
// cannot know at depth 1, where the Tally counts attribute names, so the
// assertion is declined (Tally.ReadsAttributesAt). The decided rows are
// declined instead, and fail, with xpath's quantifier declining.
func TestAssertionQuantifiedAttributesThroughTheWalk(t *testing.T) {
	const every = "every $c in e1 satisfies $c/@a"
	e1, u, a := local("e1"), local("u"), local("a")
	for _, tc := range []struct {
		why     string
		root    *testElement
		charged bool
	}{
		{"every e1 carries a", ccRoot(nil, ccNode(e1, 2, []Attribute{ccAttr(a, "x", 2)}), ccNode(e1, 3, []Attribute{ccAttr(a, "", 3)})), false},
		{"one e1 lacks a", ccRoot(nil, ccNode(e1, 2, []Attribute{ccAttr(a, "x", 2)}), ccNode(e1, 3, nil)), true},
		{"no e1 at all", ccRoot(nil), false},
	} {
		t.Run(tc.why, func(t *testing.T) {
			ccDecided(t, aAssess(t, ccSchema(t, every), tc.root), tc.charged, every)
		})
	}
	k := ccAttr(local("k"), "1", 3)
	ccDeclinedAmong(t, aAssess(t, ccSchema(t, every), ccRoot(nil, ccNode(e1, 2, []Attribute{ccAttr(a, "x", 2)}), ccNode(u, 3, []Attribute{k}))),
		"the element u at instance.xml:3:3, in the subtree of the element e1 whose nodes a {test} counts, cannot be counted exactly for the data model instance cvc-assertion clause 1 builds: its ·governing type definition· was not determined")
}
