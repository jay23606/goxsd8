package validate

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xpath"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// This file settles cvc-complex-type (§3.4.4.2) clause 6 — each assertion in
// the {assertions} of a complex ·governing type definition· — by EVALUATING its
// {test} wherever xpath compiles it, and declines the assertions facets of a
// simple type whose {test} the value pipeline could not decide. Both halves
// decline through [walk.decline], so every site this file does not decide is
// recorded and logged at the item it was reached at, and the two
// GAP(validate) markers below state, separately, what each of the two still
// withholds; the GAP(xpath) markers on the string-value collector state which
// descendants of a child of mixed content leave its string-value undecided.
//
// The two rules are DISTINCT and are never conflated. cvc-assertion
// (§3.13.4.1) is the complex-type variety, reached from cvc-complex-type
// (§3.4.4.2) clause 6 and from nowhere else. cvc-assertions-valid (Datatypes
// §4.3.13.3) is the simple-type one, reached from cvc-datatype-valid (§4.1.4)
// clause 3 (dv_vfacets) over the assertions facet of a simple type. One record
// type carries both, discriminated by [Unevaluated.Rule].
//
// The simple-type facets are evaluated where Datatype Valid is decided, inside
// package value's pipeline, through [xpath.FacetAssertions] — on the type and
// on every list item and union member the cvc-datatype-valid recursion
// PRINCIPLES 12 quantifies over reaches, and on no other. A failed {test} is
// part of the verdict the caller charges under its own rule; a declined one is
// a non-verdict, which the recording sites decline under cvc-assertions-valid
// ([walk.declineAssertions]): cvc-attribute clause 3 ([walk.declaredAttribute],
// for an attribute matched by an {attribute use} and for one ·attributed to· a
// strict or lax {attribute wildcard} alike), and cvc-type clause 3.1.3 /
// cvc-complex-type clause 1.2 over an element's ·initial value·
// ([contentCheck.stringValid]), and cvc-complex-type clause 4 over a
// ·defaulted attribute·'s {lexical form} ([walk.defaultedAttribute]). cvcid.go
// and cvcidentityconstraint.go re-run the datatype pipeline over lexicals those
// sites already decided, and record only declines of their own.

// ruleCvcAssertion is Assertion Satisfied (Structures §3.13.4.1,
// cvc-assertion), whose single caller is cvc-complex-type clause 6. The clause
// charged goes in the message on ruleCvcElt's terms: the catalog carries the
// bare name.
const ruleCvcAssertion xsderr.Rule = "cvc-assertion"

// ruleCvcAssertionsValid is Assertions Valid (Datatypes §4.3.13.3,
// cvc-assertions-valid), the assertions facet's own per-facet specification
// under Facet Valid. A simple type's assertions answer to this rule and never
// to ruleCvcAssertion: the two differ in what they bind $value to and in what
// context item they evaluate against, so one ID standing for both would
// misreport every simple-type assertion.
const ruleCvcAssertionsValid xsderr.Rule = "cvc-assertions-valid"

// elementAssertions settles cvc-complex-type (§3.4.4.2) clause 6 for e: "E is
// ·valid· with respect to each of the assertions in T.{assertions} as per
// Assertion Satisfied (§3.13.4.1)", T being e's ·governing type definition·. A
// governing type that is not a Complex Type Definition has no {assertions}
// property at all — a simple one's assertions are facets, and reach
// ruleCvcAssertionsValid instead.
//
// asserts is e's clause 6 state, opened when e was entered ([walk.compileAssertions],
// which compiles every {test}) and nil where T has no {assertions}. Each compiled
// test is evaluated with the binding [walk.assertionValue] gives `.` and `$value`:
// invalid says whether e is known to be invalid in the partial ·PSVI· by now (clause
// 2.3.1.1) — a violation recorded for e, the deciding of its governance included, or
// below it ([walk.element]'s mark) — and content is e's own content check, exhausted,
// which holds the ·initial value· and the ·nilled· answer, and own is e's own
// string-value frame ([ownStringValue]), nil where e opened none.
//
// Each assertion takes exactly one of three outcomes: DECLINED, where
// [xpath.CompileAssertionTest] reported false or e's attributes, the children
// a {test} reads, or `$value` cannot be read; CHARGED under cvc-assertion, where
// [xpath.AssertionTest.Evaluate] reports false — the {test} was false or raised
// a dynamic or type error, which cvc-assertion's opening sentence treats alike
// ("evaluates to true ... without raising any dynamic error or type error");
// and SATISFIED otherwise. The log names no clause: cvc-assertion's verdict is
// that opening sentence, and its numbered clauses only build the evaluation's
// context.
//
// GAP(validate): an assertion this package does not evaluate is DECLINED —
// recorded as an [Unevaluated] under cvc-assertion at e through [walk.decline],
// never charged and never shown satisfied. The residue is: a {test} xpath declines,
// whose GAP(xpath) markers name the grammar and type residue (paths, the function
// library beyond what [xpath.CompileAssertionTest] lists); and every assertion of
// an e one of whose attributes matching an {attribute use}, carried or ·defaulted·,
// has no ·actual value·, one of whose element [[children]] a {test} reads has no
// typed value this package reads ([walk.keepChild]) — a child of mixed content
// among them whose string-value spans a descendant whose contribution is undecided
// ([stringValueFrame.lacking]) — or one of whose attributes a {test} reads
// through a range variable bound to that child has none under the type the
// {test} was compiled to read, or whose ·defaulted attributes· are unknown
// ([walk.childAttributes]), whose `$value` is undecided ([walk.assertionValues]),
// whose own string value, which `.` reads, spans such a descendant, or was not
// gathered because e has simple content and element [[children]], or is ·nilled·
// and has [[children]] and simple content or a {test} reading `.`
// ([walk.assertionValue]), or one of whose {test}s counts the attribute nodes of,
// filters children by the attribute names of, or reads the attribute values of, an
// element of its subtree whose ·governing type definition· this package could not
// determine, so that its ·defaulted attributes· are unknown ([walk.tallyElement]),
// or reads through a path the typed value of an attribute below e that has none
// under the type the {test} was compiled to read — its element ·skipped·, its own
// {attribute use}'s type neither that type nor validly derived from it, no use
// matching it, or no ·actual value· ([walk.tallyAttributes]) — or the typed value
// of an element below e, read through a path of child steps, that has none under
// that type: the element ·skipped·, not known ·valid·, governed by no type or by
// one neither that type nor validly derived from it, or its value not mapping under
// it ([walk.tallyValue]); a ·skipped· subtree is counted, by name
// ([assertionAncestry.tallySkipped]). Fail-open: the withheld value is clause 6's
// own verdict, whose whole consumer set inside this package is w.res.violations and
// its one reader [Result.Violations], which charge on a violation PRESENT, so a
// decline can only cost a rejection and can manufacture none. (#1042)
func (w *walk) elementAssertions(e Element, asserts *assertionCheck, content *contentCheck, own *stringValueFrame, invalid bool) {
	if asserts == nil {
		return
	}
	attrs := e.Attributes()
	in, lack := w.assertionValues(e, attrs, asserts, content, own, invalid)
	for i, c := range asserts.tests {
		site := fmt.Sprintf("assertion %d of %d in the {assertions} of the ·governing type definition· %s, whose {test} is %q,",
			i+1, len(asserts.tests), typeName(asserts.ct), c.a.Test().Expression())
		if lack != nil {
			w.decline("assessing element", e.Name(), e.Loc(), ruleCvcAssertion, "",
				"%s was not evaluated: %s, so whether the element is ·valid· with respect to it, as cvc-complex-type clause 6 requires, is undecided",
				site, lack.declined(e.Name()))
			continue
		}
		test := c.test
		if test == nil {
			w.decline("assessing element", e.Name(), e.Loc(), ruleCvcAssertion, "",
				"%s was not evaluated: this engine's XPath evaluator declined it, so whether the element %s is ·valid· with respect to it, as cvc-complex-type clause 6 requires (Assertion Satisfied, §3.13.4.1), is undecided",
				site, e.Name())
			continue
		}
		if test.Evaluate(w.backend, w.schema, e, in.yield, asserts.yieldChildren, c.tally, in.value, w.now) {
			w.logDecision("assessing element", e.Name(), e.Loc(), ruleCvcAssertion, "", "satisfied")
			continue
		}
		w.res.violations = append(w.res.violations, xsderr.New(ruleCvcAssertion, e.Loc(),
			"the element %s is not ·valid· with respect to %s which did not evaluate to true without raising a dynamic or type error, as Assertion Satisfied (§3.13.4.1) requires of each assertion cvc-complex-type clause 6 quantifies over",
			e.Name(), site))
		w.logDecision("assessing element", e.Name(), e.Loc(), ruleCvcAssertion, "", "charged")
	}
}

// assertionCheck is one element's cvc-complex-type clause 6 state, opened when
// the element is entered ([walk.compileAssertions]) and settled once its
// [[children]] are exhausted ([walk.elementAssertions]): the ·governing type
// definition· ct whose {assertions} are evaluated, and each of them with its
// compiled {test}, in {assertions} order.
//
// Between the two it collects, from the element's own [[children]] as each is
// exhausted ([walk.keepChild]), what the compiled tests read of them: children
// holds one [xpath.ChildElement] per element [[child]] some test reads, in
// arrival order, which is document order (STYLE D1); and lack is the first such
// child that has no typed value this package reads, nil while there is none —
// a read child of mixed content taking it as well from a descendant whose
// contribution to its string-value is undecided ([stringValueFrame.lacking]),
// and a child read through a range variable taking it from an attribute a test
// reads that has no typed value under the type the test was compiled to read
// ([walk.childAttributes]) — or the first element of the element's subtree a
// test's [xpath.Tally] cannot be told of exactly: one whose ·governing type
// definition· was not determined, at whose depth some test reads attribute
// names or values ([assertionCheck.readsAttributesAt], [walk.tallyElement]), or
// one carrying an attribute whose typed value a test reads and which has none
// under the type the test was compiled to read ([walk.tallyAttributes],
// [assertionAncestry.tallySkipped]), or one whose own typed value a test reads
// through a path of child steps and which has none under that type
// ([walk.tallyValue]). The lack is the whole check's, never one test's. Each
// test's Tally collects the counts, and the typed values of the attributes a
// path to an attribute reads and of the elements a path of child steps reads,
// from the whole subtree as it is walked — a ·skipped· part of it by name
// ([assertionAncestry.tallySkipped]) — and no node of it is kept.
type assertionCheck struct {
	ct       xsd.ComplexType
	tests    []assertionTest
	children []xpath.ChildElement
	lack     assertionLack
}

// counts reports whether some compiled test of c counts nodes of the element's
// subtree, holding an [xpath.Tally]. A nil c — an element with no {assertions}
// — counts none.
func (c *assertionCheck) counts() bool {
	if c == nil {
		return false
	}
	for _, t := range c.tests {
		if t.tally != nil {
			return true
		}
	}
	return false
}

// readsAttributesAt reports whether the attribute names reported for an
// element depth levels below c's element, 0 being c's element itself, can
// change a count, or a value read, some compiled test of c holds
// ([xpath.Tally.ReadsAttributesAt]). A test with no Tally reads none.
func (c *assertionCheck) readsAttributesAt(depth int) bool {
	for _, t := range c.tests {
		if t.tally.ReadsAttributesAt(depth) {
			return true
		}
	}
	return false
}

// readsAttribute reports whether some compiled test of c reads the typed value
// of the attribute named name of each element whose chain of names below c's
// element is path ([xpath.Tally.ReadsAttribute]). A test with no Tally reads
// none.
func (c *assertionCheck) readsAttribute(path []xsd.QName, name xsd.QName) bool {
	for _, t := range c.tests {
		if t.tally.ReadsAttribute(path, name) {
			return true
		}
	}
	return false
}

// attribute reports v, the typed value of the attribute named name of the
// element just reported at path ([assertionCheck.tally]), to the Tally of
// each test of c that reads it ([xpath.Tally.Attribute]).
func (c *assertionCheck) attribute(path []xsd.QName, name xsd.QName, v xpath.TypedValue) {
	for _, t := range c.tests {
		if t.tally.ReadsAttribute(path, name) {
			t.tally.Attribute(name, v)
		}
	}
}

// readsElementValue reports whether some compiled test of c reads the typed
// value of each element whose chain of names below c's element is path
// ([xpath.Tally.ReadsElementValue]). A test with no Tally reads none.
func (c *assertionCheck) readsElementValue(path []xsd.QName) bool {
	for _, t := range c.tests {
		if t.tally.ReadsElementValue(path) {
			return true
		}
	}
	return false
}

// elementValue reports v, the typed value of the element at path whose subtree
// was just walked ([walk.tallyValue]), to the Tally of each test of c that
// reads it ([xpath.Tally.ElementValue]).
func (c *assertionCheck) elementValue(path []xsd.QName, v xpath.TypedValue) {
	for _, t := range c.tests {
		if t.tally.ReadsElementValue(path) {
			t.tally.ElementValue(v)
		}
	}
}

// tally reports one element of c's element's subtree to each test's Tally, in
// one [xpath.Tally.Element] call: by path, the names of the elements from c's
// element's child down to it inclusive, with attrs, the names of its attribute
// nodes. The empty path is c's own element, whose element node selects
// nothing and whose attributes alone count. A test with no Tally takes nothing
// ([xpath.Tally]'s nil receiver).
func (c *assertionCheck) tally(path []xsd.QName, attrs []xsd.QName) {
	for _, t := range c.tests {
		t.tally.Element(path, attrs)
	}
}

// readsContextItem reports whether some compiled test of c reads E's string
// value, which `.` atomizes to ([xpath.AssertionTest.ReadsContextItem]),
// under a {content type} that is not simple, whose string value is collected
// by frame ([ownStringValue]); under a simple one it is the ·initial value·
// and is never collected. A {test} reading `$value` alone reads none of it:
// `$value` is the empty sequence there (clause 2.3.2). A nil c — an element
// with no {assertions} — reads nothing.
func (c *assertionCheck) readsContextItem() bool {
	if c == nil {
		return false
	}
	if _, simple := c.ct.ContentType().(xsd.SimpleContent); simple {
		return false
	}
	for _, t := range c.tests {
		if t.test != nil && t.test.ReadsContextItem() {
			return true
		}
	}
	return false
}

// reads reports whether some compiled test of c reads a child element named
// name ([xpath.AssertionTest.ReadsChild]). A nil c — an element with no
// {assertions} — reads none.
func (c *assertionCheck) reads(name xsd.QName) bool {
	if c == nil {
		return false
	}
	for _, t := range c.tests {
		if t.test != nil && t.test.ReadsChild(name) {
			return true
		}
	}
	return false
}

// childAttributesRead is the names of the attributes of a child element named
// name that some compiled test of c reads through a range variable bound to it
// ([xpath.AssertionTest.ReadsChildAttributes]), the union over c's tests, each
// name possibly more than once. A nil c reads none.
func (c *assertionCheck) childAttributesRead(name xsd.QName) []xsd.QName {
	if c == nil {
		return nil
	}
	var names []xsd.QName
	for _, t := range c.tests {
		if t.test != nil {
			names = append(names, t.test.ReadsChildAttributes(name)...)
		}
	}
	return names
}

// lacking records l as c's lack where c has none yet, so the first lack the
// walk meets is the one a decline names: a counted node's on entering it
// ([walk.tallyElement]), a read child's once its assessment is over
// ([walk.keepChild]), both in document order.
func (c *assertionCheck) lacking(l assertionLack) {
	if c.lack == nil {
		c.lack = l
	}
}

// assertionAncestry is what the {assertions} of an element's ancestors read of
// it, handed down the walk beside its identity-constraint state: parent, the
// clause 6 state of its parent, nil at the ·validation root· and wherever the
// parent has no {assertions}, which reads its typed value ([walk.keepChild]);
// counting, the nearest ancestor at any depth one of whose {test}s counts nodes
// of its subtree, which is told of the element ([walk.tallyElement]);
// collecting, the innermost open string-value frame among its ancestors — an
// ancestor whose string-value a {test} of its own parent, or of its own, reads
// — to every open frame of which the element's text runs are appended
// ([stringValueFrame]); and path, the ·expanded names· of the element's
// ancestors from the ·validation root· down, the root included, so len(path)
// is the element's depth below the root, 0 at the root.
//
// It holds O(depth) frames, one per counting ancestor and up to two per
// collecting one, O(depth) names, the text of each collecting ancestor's
// subtree so far, and no node.
//
// Every element's path shares its parent's backing array: below appends the
// element's own name in place, so a later sibling's name overwrites an earlier
// one's at the same index, and a descendant's those below it. That is safe
// because the walk is recursive — an element's subtree is done before its next
// sibling's name is written — and because path is read only during
// [walk.tallyElement]'s calls, which [xpath.Tally.Element] retains nothing of.
type assertionAncestry struct {
	parent     *assertionCheck
	counting   *tallyFrame
	collecting *stringValueFrame
	path       []xsd.QName
}

// tallyFrame is one counting ancestor: its clause 6 state, its depth below the
// ·validation root·, and the next counting ancestor above it, nil where there
// is none.
type tallyFrame struct {
	check *assertionCheck
	depth int
	outer *tallyFrame
}

// stringValueFrame is the string-value (xpath-datamodel §6.2.4) of the element
// whose content check is owner, which a {test} reads, whose clause 6 state is
// reader. It is one of two kinds. A CHILD frame (own false) is an element
// [[child]] a {test} of its parent reads, reader being the parent's: a child
// whose ·locally declared type· and own ·governing type definition· both have
// mixed content, and which is not ·nilled· ([walk.stringValue]). An OWN frame
// (own true) is E's string value, which `.` atomizes to and which reader, E's
// own clause 6 state, reads: an E whose {content type} is not simple, which is
// not ·nilled·, and one of whose compiled {test}s reads `.`
// ([ownStringValue], [assertionCheck.readsContextItem]).
//
// text collects the concatenation of every Text Node of owner's subtree in
// document order, as the partial ·PSVI· cvc-assertion clause 1.2 builds them:
// each character run arrives in every open frame
// ([assertionAncestry.collectText]) — whole where it is the owner's own, the
// owner being annotated xs:anyType there if E and mixed if a read child, and
// as xpath-datamodel §6.7.4 builds the Text Node of a descendant otherwise.
// outer is the next open frame enclosing it, nil where there is none.
//
// It holds the text of one subtree while that subtree is walked, the only
// data a string-value is made of, and nothing else of it.
type stringValueFrame struct {
	text   strings.Builder
	owner  *contentCheck
	reader *assertionCheck
	own    bool
	outer  *stringValueFrame
}

// lacking declines the reader of f and of every frame enclosing it, for the
// reason why: a descendant whose contribution to the innermost string-value is
// undecided leaves every string-value spanning it undecided too.
func (f *stringValueFrame) lacking(why string) {
	for ; f != nil; f = f.outer {
		f.reader.lacking(f.lack(why))
	}
}

// lack is what f's reader lacks for the reason why: E's own string value for
// an own frame ([lackingStringValue]), and a read child's typed value for a
// child frame ([lackingChild]).
func (f *stringValueFrame) lack(why string) assertionLack {
	if f.own {
		return lackingStringValue{why: why}
	}
	return lackingChild{name: f.owner.e.Name(), loc: f.owner.e.Loc(), why: why}
}

// below is the ancestry of each element [[child]] of the element named name
// whose ancestry is up and whose own clause 6 state is own: own is their
// parent, name ends their ancestors' path, the counting ancestors are up's,
// with own's element innermost where some test of own counts, and the
// innermost open string-value frame is inner, the innermost the element
// itself opened or up's where it opened none ([walk.element]).
func (up assertionAncestry) below(name xsd.QName, own *assertionCheck, inner *stringValueFrame) assertionAncestry {
	next := assertionAncestry{parent: own, counting: up.counting, collecting: inner, path: append(up.path, name)}
	if own.counts() {
		next.counting = &tallyFrame{check: own, depth: len(up.path), outer: up.counting}
	}
	return next
}

// stringValue opens the string-value frame of e, an element whose ancestry is
// up, governed by g, ·nilled· where nilled is true, and is nil where it opens
// none: e opens one exactly where some {test} of its parent reads its name, its
// ·locally declared type· within the parent's ·governing type definition· —
// the lookup [walk.assertionElementTypes] answers the compile — has mixed
// content, its own ·governing type definition· does too, and it is not
// ·nilled·. Every other read child's value is [walk.childValue]'s to decide,
// which lacks or reads it with no string-value. content is e's content check.
func (w *walk) stringValue(e Element, g governance, content *contentCheck, up assertionAncestry) *stringValueFrame {
	if content.nilled || !mixedContent(g.typ) || !up.parent.reads(e.Name()) {
		return nil
	}
	ldt, local := w.schema.LocallyDeclaredElementType(up.parent.ct, e.Name())
	if !local || !mixedContent(ldt) {
		return nil
	}
	return &stringValueFrame{owner: content, reader: up.parent, outer: up.collecting}
}

// ownStringValue opens the own string-value frame of the element whose clause
// 6 state is asserts and whose content check is content — the frame
// [walk.assertionValue] binds `.` from — inside outer, the innermost frame
// open around its [[children]] otherwise; it is nil where the element opens
// none. It opens one exactly where the element is not ·nilled· and some
// compiled {test} reads `.` under a {content type} that is not simple
// ([assertionCheck.readsContextItem]): under simple content `.` is the
// ·initial value· the content check gathers, and a ·nilled· element binds `.`
// without its [[children]]'s text.
func ownStringValue(asserts *assertionCheck, content *contentCheck, outer *stringValueFrame) *stringValueFrame {
	if content.nilled || !asserts.readsContextItem() {
		return nil
	}
	return &stringValueFrame{owner: content, reader: asserts, own: true, outer: outer}
}

// mixedContent reports whether td is a complex type whose {content
// type}.{variety} is mixed — xs:anyType among them (§3.4.7) — whose element's
// typed value is its string-value as xs:untypedAtomic (xpath-datamodel
// §6.2.4).
func mixedContent(td xsd.TypeDefinition) bool {
	ct, isComplex := td.(xsd.ComplexType)
	return isComplex && ct.ContentType().Variety() == xsd.ContentMixed
}

// collectElement settles what e, an element whose ancestry is up, governed by g
// and ·nilled· where nilled is true, contributes to the string-value of every
// open frame enclosing it, before its own [[children]] arrive: a ·nilled· one
// contributes nothing of its own, whatever its type, and a run among its
// [[children]] declines every such frame ([assertionAncestry.collectText]);
// every other one contributes its text runs as collectText reads them off, and
// what [collectEnd] settles once they are exhausted.
//
// GAP(xpath): an element whose ·governing type definition· this package could
// not determine declines the reader of every frame enclosing it: it may have
// simple content, element-only content or mixed content, each of which builds
// its Text Nodes differently (§6.7.4). The direction is the withhold: the
// assertion is recorded as unevaluated, never charged and never shown
// satisfied. (#1042)
func (up assertionAncestry) collectElement(e Element, g governance, nilled bool) {
	if up.collecting == nil || nilled || !g.typeUndetermined() {
		return
	}
	up.collecting.lacking(fmt.Sprintf("the element %s at %s below it has a ·governing type definition· that was not determined, so what its text contributes to the string-value is undecided", e.Name(), e.Loc()))
}

// collectText appends t, one character run [[child]] of the element whose
// content check is content, to every open string-value frame of up. To a frame
// whose owner that element is, the run is appended whole: E is annotated
// xs:anyType in the partial ·PSVI· (cvc-assertion clause 1.2), and a read
// child's frame is opened only over mixed content, so neither drops a run of
// white space. To every other frame it is appended as the Text Node
// xpath-datamodel §6.7.4 builds from it under that element's governance
// (descendantRun): under mixed content, or ·laxly assessed· and so annotated
// xs:anyType, the raw run; under simple content the raw run as well, which
// [collectEnd] declines where it is not the [schema normalized value]; and
// under element-only or empty content nothing for a run that is white space
// alone, which is no Text Node there.
//
// GAP(xpath): a run whose Text Node a frame would not hold whole declines the
// reader of every frame instead. A run that is white space alone under simple
// content is no Text Node where the data model reads the raw characters and
// part of one where it reads the [schema normalized value] (xpath-datamodel
// Appendix J.2, "may"). Any other run under element-only or empty content is a
// Text Node of its own (§6.7.4) that cvc-complex-type clause 1.1 or 1.3
// charged; and a run of a ·nilled· element, which cvc-elt clause 3.2.3.1
// charged, leaves dm:nilled false (§6.2.4), so it stands in the string-value as
// a Text Node built under that element's {content type}. This walk reads
// neither into the string-value: a read child's frame spanning one is declined
// in [walk.childValue] all the same, and an own frame has no such guard. A run
// of an element whose ·governing type definition· this package could not
// determine declines too, on [assertionAncestry.collectElement]'s terms. The
// direction is the withhold: the assertion is recorded as unevaluated, never
// charged and never shown satisfied. (#1042)
func (up assertionAncestry) collectText(content *contentCheck, t Text) {
	if up.collecting == nil || t.Data() == "" {
		return
	}
	if content.nilled {
		up.collecting.lacking(fmt.Sprintf("a character run at %s below it is in a ·nilled· element, which cvc-elt clause 3.2.3.1 charged, so its Text Node is not read into the string-value", t.Loc()))
		return
	}
	run, why := descendantRun(content.g, t)
	if why != "" {
		up.collecting.lacking(why)
		return
	}
	for f := up.collecting; f != nil; f = f.outer {
		if f.owner == content {
			f.text.WriteString(t.Data())
			continue
		}
		f.text.WriteString(run)
	}
}

// descendantRun is what t, a character run [[child]] of a non-·nilled·
// element governed by g, contributes to the string-value of an ancestor, on
// [assertionAncestry.collectText]'s terms, or why that is undecided.
func descendantRun(g governance, t Text) (run, why string) {
	if g.typeUndetermined() {
		return "", fmt.Sprintf("a character run at %s below it is in an element whose ·governing type definition· was not determined", t.Loc())
	}
	if g.laxlyAssessed() || mixedContent(g.typ) {
		return t.Data(), ""
	}
	if g.valueType() == nil && isXMLWhitespace(t.Data()) {
		return "", ""
	}
	if g.valueType() == nil {
		return "", fmt.Sprintf("a character run at %s below it is not white space alone in an element of element-only or empty content, which cvc-complex-type clause 1.1 or 1.3 charged, so its Text Node is not read into the string-value", t.Loc())
	}
	if isXMLWhitespace(t.Data()) {
		return "", fmt.Sprintf("a character run at %s below it is white space alone in an element of simple content, which the data model makes no Text Node of where it reads the raw characters and part of one where it reads the [schema normalized value], so the string-value is undecided", t.Loc())
	}
	return t.Data(), ""
}

// collectEnd settles what e, an element governed by g with the content check
// content, contributes to the string-value of every open frame around its
// [[children]] — inner being the innermost, e's own among them, nil where none
// is open — once those [[children]] are exhausted, beyond the runs
// [assertionAncestry.collectText] appended. A ·nilled· e contributes nothing.
// r resolves the whiteSpace facet of e's simple type.
//
// GAP(xpath): two contributions are a processor's choice the data model leaves
// open, and each declines the reader of every frame from inner out rather
// than guess. An element of mixed content whose ·initial value· cvc-elt clause
// 5.1 substituted from its {value constraint}: xpath-datamodel §6.2.4 (:1265)
// lets a processor add a Text Node for the default, not requiring it. And an
// element of simple type or simple content whose raw characters are not its
// [schema normalized value], which Appendix J.2 lets its Text Node hold
// instead (:1259, "may"): one with element [[children]], which cvc-complex-type
// clause 1.2 or cvc-type clause 3.1.2 charged; one that took a non-empty
// default of its {value constraint}, of which J.2 makes a Text Node no raw
// character holds; and one whose ·initial value· its whiteSpace facet
// changes, or whose facet this package does not read (normalizedLexical).
// Where the raw characters are the [schema normalized value], both readings
// build the one Text Node collectText appended. The direction is the
// withhold: the assertion is recorded as unevaluated, never charged and never
// shown satisfied. (#1042)
func collectEnd(r xsd.TypeResolver, e Element, inner *stringValueFrame, g governance, content *contentCheck) {
	if inner == nil || content.nilled {
		return
	}
	if why := undecidedContribution(r, e, g, content); why != "" {
		inner.lacking(why)
	}
}

// undecidedContribution is why what e, governed by g with the content check
// content and not ·nilled·, contributes to a string-value spanning it is
// undecided, on [collectEnd]'s terms, and "" where it is decided.
func undecidedContribution(r xsd.TypeResolver, e Element, g governance, content *contentCheck) string {
	vc, defaulted := content.defaulted()
	if mixedContent(g.typ) && defaulted {
		return fmt.Sprintf("the element %s at %s, of mixed content, took the default of its {value constraint}, which the data model may or may not make a Text Node", e.Name(), e.Loc())
	}
	st := g.valueType()
	if st == nil {
		return ""
	}
	if content.sawElement {
		return fmt.Sprintf("the element %s at %s below it has simple content and element [[children]], so what its text contributes to the string-value is undecided", e.Name(), e.Loc())
	}
	if defaulted && vc.LexicalForm() != "" {
		return fmt.Sprintf("the element %s at %s below it, of simple content, took the default %q of its {value constraint}, which the data model makes a Text Node where it reads the [schema normalized value] and not where it reads the raw characters, so the string-value is undecided", e.Name(), e.Loc(), vc.LexicalForm())
	}
	lexical, _ := content.assessed()
	normalized, ok := normalizedLexical(r, st, lexical)
	if !ok || normalized != lexical {
		return fmt.Sprintf("the element %s at %s below it has simple content whose raw characters %q are not its [schema normalized value], and the data model lets its Text Node hold either, so the string-value is undecided", e.Name(), e.Loc(), lexical)
	}
	return ""
}

// skipped hands e, an element [[child]] ·skipped· by key-sva clause 3.2 and
// whose ancestry is up, to the {assertions} of its ancestors: as a lack of its
// parent where some test of the parent reads e's name — a ·skipped· element is
// not ·assessed·, so the partial ·PSVI· gives it no type to read its value
// under — and, where some ancestor counts, as e's subtree told to every
// counting ancestor's Tallies ([assertionAncestry.tallySkipped]). It reports
// the fault in the source that stopped that subtree, and nil where none did;
// where no ancestor counts, nothing below e is read and it reports nil.
//
// GAP(xpath): a ·skipped· e inside an open string-value frame declines the
// reader of every enclosing frame ([stringValueFrame.lacking]) rather than
// reading its text into the string-value, which [assertionAncestry.tallySkipped]
// does not walk for. The direction is the withhold: the assertion is recorded
// as unevaluated, never charged and never shown satisfied. (#1042)
func (up assertionAncestry) skipped(e Element) error {
	if up.parent.reads(e.Name()) {
		up.parent.lacking(lackingChild{name: e.Name(), loc: e.Loc(), why: "it is ·skipped·, so it is not ·assessed·"})
	}
	if up.collecting != nil {
		up.collecting.lacking(fmt.Sprintf("the element %s at %s below it is ·skipped·, so its text is not read into the string-value", e.Name(), e.Loc()))
	}
	if up.counting == nil {
		return nil
	}
	return up.tallySkipped(e)
}

// tallySkipped tells every counting ancestor in up of e, an element of a
// ·skipped· subtree whose ancestry is up, and then of each element below e, in
// document order: each ancestor's Tallies count an element by its chain of
// names below that ancestor, on [walk.tallyElement]'s terms, with the names of
// the attributes the element carries at that chain's length. A ·skipped·
// element and everything below it is governed by no type (key-skipped,
// key-governing-type-elem item 5), so it has no ·defaulted attribute·
// (key-dflt-att), and its attribute nodes are exactly the [[attributes]] it
// carries, xsi ones included ([Element.Attributes]); whether a {test} counts
// attribute nodes at its depth decides nothing here. An attribute whose typed
// value a {test} of a counting ancestor reads through a path to it
// ([assertionCheck.readsAttribute]) has a typed value of xs:untypedAtomic
// under no type, never the one that {test} was compiled to read, so that
// ancestor takes the lack ([lackingAttributeBelow]) and its Tallies, left
// owing the value, are never read; so does one a {test} of which reads the
// typed value of the element itself through a path of child steps
// ([assertionCheck.readsElementValue]), on the same terms
// ([lackingElementBelow]).
//
// It reads names and nothing else: no element below the skip is ·assessed·
// (cvc-assess-elt clause 2), so nothing is fed to an identity constraint or the
// ID table, no [inherited attributes] are handed down, no value is kept for a
// {test} ([walk.keepChild]), and nothing is logged. It opens e's [[children]]
// ([Element.Children]) while the source still stands at e — the caller's
// cursor has not advanced past it — and drains that cursor before it returns,
// as [Children] requires; a text child counts nothing and is passed over. It
// reports the fault in the source that stopped a cursor, wrapped once with the
// element whose [[children]] it was reading, and nil otherwise.
func (up assertionAncestry) tallySkipped(e Element) error {
	// chain is on [walk.tallyElement]'s shared-array terms: each child's call
	// writes the slot after it, and returns before the next child's does.
	chain := append(up.path, e.Name())
	var attrs []xsd.QName
	for _, a := range e.Attributes() {
		attrs = append(attrs, a.Name())
	}
	for f := up.counting; f != nil; f = f.outer {
		path := chain[f.depth+1:]
		f.check.tally(path, attrs)
		if f.check.readsElementValue(path) {
			f.check.lacking(lackingElementBelow{name: e.Name(), loc: e.Loc(), why: "it is ·skipped·, so its typed value is xs:untypedAtomic, never the type the {test} was compiled to read"})
		}
		for _, a := range attrs {
			if f.check.readsAttribute(path, a) {
				f.check.lacking(lackingAttributeBelow{attr: a, name: e.Name(), loc: e.Loc(), why: "its element is ·skipped·, so its typed value is xs:untypedAtomic, never the type the {test} was compiled to read"})
				break
			}
		}
	}
	below := assertionAncestry{counting: up.counting, path: chain}
	kids := e.Children()
	for {
		c, ok := kids.Next()
		if !ok {
			break
		}
		child, isElement := c.Element()
		if !isElement {
			continue
		}
		if err := below.tallySkipped(child); err != nil {
			return err
		}
	}
	if err := kids.Err(); err != nil {
		return fmt.Errorf("reading the children of %s at %s: %w", e.Name(), e.Loc(), err)
	}
	return nil
}

// tallyElement tells every counting ancestor in up, and e's own clause 6 state
// own, of e: each ancestor's Tallies count e by its chain of names below that
// ancestor — the names in up's path below the ancestor's depth, then e's own —
// with e's attribute nodes ([walk.attributeNodes]) at that chain's length, and
// own's count e's attribute nodes with the empty chain, depth 0. g is e's
// governance. Each ancestor's Tallies that read the typed value of an
// attribute of e through a path to it ([xpath.Tally.ReadsAttribute]) are then
// told that value ([walk.tallyAttributes]). Where e's attribute nodes are
// undecided — its ·governing type definition· was not determined, so its
// ·defaulted attributes· are unknown — an ancestor some test of which reads
// attribute names at e's depth below it ([assertionCheck.readsAttributesAt])
// — counting attribute nodes, filtering children by their attributes, or
// reading their values — takes the lack instead, which declines its
// assertions, since a count or a sum missing them could fabricate a charge;
// every other ancestor counts e's element node alone, which no attribute name
// it could miss changes.
//
// It runs on every element [walk.element] enters — invalid, ·nilled·, ·laxly
// assessed· or undecided alike — once, on entry and before [walk.element]
// walks e's [[children]], which is the number and order of reports
// [xpath.Tally] obliges its caller to. A ·skipped· element is not entered,
// and [assertionAncestry.tallySkipped] reports it and its subtree in its
// place. The reports withheld from an ancestor that takes the lack instead
// can leave that ancestor's Tally out of that order; its assertions are
// declined on the lack, and that Tally is never read.
func (w *walk) tallyElement(e Element, g governance, up assertionAncestry, own *assertionCheck) {
	if up.counting == nil && !own.counts() {
		return
	}
	attrs, decided := w.attributeNodes(e, g)
	tell := func(c *assertionCheck, path []xsd.QName) {
		if decided {
			c.tally(path, attrs)
			w.tallyAttributes(c, path, e, g)
			return
		}
		if c.readsAttributesAt(len(path)) {
			c.lacking(lackingCount{name: e.Name(), loc: e.Loc(), why: "its ·governing type definition· was not determined, so its ·defaulted attributes·, whose names or values a {test} reads, are unknown"})
			return
		}
		c.tally(path, nil)
	}
	// chain is e's ancestors' names from the root down, then e's own, on
	// assertionAncestry's shared-array terms: e's own subtree, which would
	// overwrite the slot, has not been walked yet.
	chain := append(up.path, e.Name())
	for f := up.counting; f != nil; f = f.outer {
		tell(f.check, chain[f.depth+1:])
	}
	if own.counts() {
		tell(own, chain[len(chain):])
	}
}

// tallyAttributes tells the Tallies of c, an ancestor's clause 6 state to which
// e was just reported at path ([assertionCheck.tally]), the typed value of each
// attribute node of e one of them reads ([assertionCheck.readsAttribute]), on
// [walk.attributesRead]'s terms. Where one has none, c takes the lack instead
// ([lackingAttributeBelow]) and nothing more is told: its assertions are
// declined, and its Tallies, left owing a value, are never read.
func (w *walk) tallyAttributes(c *assertionCheck, path []xsd.QName, e Element, g governance) {
	reads := func(name xsd.QName) bool { return c.readsAttribute(path, name) }
	tell := func(name xsd.QName, v xpath.TypedValue) { c.attribute(path, name, v) }
	if lack := w.attributesRead(c.ct, path, e, g, reads, tell); lack != nil {
		c.lacking(lack)
	}
}

// attributesRead hands sink, in order, the typed value of each attribute node
// of e that reads reports a {test} of the element whose ·governing type
// definition· is ct reads at path below it: each attribute e carries, in
// document order, then each ·defaulted attribute· under g's complex
// ·governing type definition·, in {attribute uses} order, the order
// [walk.attributeNodes] names them in. Each value is [walk.attributeBelow]'s.
// It stops at the first that has none and returns that lack
// ([lackingAttributeBelow]), sink having been handed only those before it.
func (w *walk) attributesRead(ct xsd.ComplexType, path []xsd.QName, e Element, g governance, reads func(xsd.QName) bool, sink func(xsd.QName, xpath.TypedValue)) assertionLack {
	attrs := e.Attributes()
	var uses []xsd.AttributeUse
	if own := g.complexType(); own != nil {
		uses = own.AttributeUses()
	}
	for _, a := range attrs {
		if !reads(a.Name()) {
			continue
		}
		u, matched := attributeUseNamed(uses, a.Name())
		if !matched {
			return lackingAttributeBelow{attr: a.Name(), name: e.Name(), loc: e.Loc(), why: "no {attribute use} of its element's ·governing type definition· matches it, so its type is not the one the {test} was compiled to read"}
		}
		v, why := w.attributeBelow(ct, path, u, a.Value(), elementContext{owner: e}, a.Loc())
		if why != "" {
			return lackingAttributeBelow{attr: a.Name(), name: e.Name(), loc: e.Loc(), why: why}
		}
		sink(a.Name(), v)
	}
	for _, u := range uses {
		vc, defaulted := w.defaultedConstraint(u, attrs)
		if !defaulted || !reads(u.DeclarationName()) {
			continue
		}
		v, why := w.attributeBelow(ct, path, u, vc.LexicalForm(), value.ConstraintContext(vc), e.Loc())
		if why != "" {
			return lackingAttributeBelow{attr: u.DeclarationName(), name: e.Name(), loc: e.Loc(), why: why}
		}
		sink(u.DeclarationName(), v)
	}
	return nil
}

// attributeBelow is the typed value of an attribute of an element at path below
// the element whose ·governing type definition· is ct, as a {test} of that
// element reads it through a path to the attribute, or why it has none. u is
// the attribute's own {attribute use}, within its element's own ·governing
// type definition·; lexical is its [schema normalized value], read at loc and
// mapped under ctx. The value is mapped under T, the type [walk.assertionTypes]
// answers for (path, u's name) — the one closure the {test} was compiled
// against, so the compiled type and the mapped type cannot disagree, which
// [xpath.Tally.Attribute] makes the caller's obligation — on
// [walk.childValue]'s pattern: normalized under u's own type T′ and mapped
// under T. It reports why instead where:
//
//   - T′ is neither T nor, for a T that is not ·special·, validly derived from
//     it (cos-st-derived-ok, [xsd.Schema.ValidlySubstitutable] with no blocking
//     keyword): T is STATIC, read off the ·locally declared type· chain, and a
//     type an element is governed by — a {type table}'s selection, an
//     xsi:type, a declaration a wildcard resolves to — can give the attribute
//     another type. A ·special· T is read as xs:untypedAtomic, so T′ must be
//     T itself, on [walk.childValue]'s terms for a child;
//   - u's declaration or type does not resolve, or String Valid
//     ([walk.stringValid]) over lexical under T′ is rejected or withheld, so it
//     has no ·actual value·, on [walk.assertionTyped]'s terms;
//   - its normalized value does not map under T.
//
// The relation is tested on the attribute use's type directly, never through
// the element's ·governing type definition· being substitutable for the
// ·locally declared type·.
func (w *walk) attributeBelow(ct xsd.ComplexType, path []xsd.QName, u xsd.AttributeUse, lexical string, ctx value.Context, loc xsderr.Loc) (xpath.TypedValue, string) {
	compiled, typed := w.assertionTypes(ct)(path, u.DeclarationName())
	if !typed {
		// Unreachable: a {test} reads the value only where the compile typed it.
		return nil, "the {test} was compiled with no type for it"
	}
	own, resolved := w.assertionType(u)
	if !resolved {
		return nil, "its {attribute declaration} or its type does not resolve"
	}
	if !sameType(own, compiled) {
		if compiled.IsSpecial() {
			return nil, fmt.Sprintf("its type %s is not %s, which the {test} reads as xs:untypedAtomic", typeName(own), typeName(compiled))
		}
		derived, err := w.schema.ValidlySubstitutable(own, compiled, nil)
		if err != nil || !derived {
			return nil, fmt.Sprintf("its type %s is neither %s, the type the {test} was compiled to read, nor validly derived from it", typeName(own), typeName(compiled))
		}
	}
	decided, verdict := w.stringValid(own, lexical, ctx, loc)
	if !decided || verdict != nil {
		return nil, fmt.Sprintf("%q has no ·actual value· under its type %s", lexical, typeName(own))
	}
	if compiled.IsSpecial() {
		return xpath.Untyped(lexical), ""
	}
	normalized, ok := normalizedLexical(w.schema, own, lexical)
	if !ok {
		return nil, fmt.Sprintf("%q has no [schema normalized value] under its type %s", lexical, typeName(own))
	}
	v, err := value.ValidateLexical(w.backend, w.schema, compiled, normalized, ctx, xpath.FacetAssertions(w.now))
	if err != nil {
		return nil, fmt.Sprintf("its [schema normalized value] %q has no ·actual value· under %s", normalized, typeName(compiled))
	}
	return xpath.Typed(v), ""
}

// tallyValue tells every counting ancestor in up, one of whose {test}s reads
// the typed value of e through a path of child steps
// ([assertionCheck.readsElementValue]), that value, once e's subtree is walked
// and its own assessment is over ([xpath.Tally.ElementValue] is owed after the
// reports of e's subtree). g is e's governance, content its content check, and
// recorded whether any violation or [Unevaluated] was recorded for e, the
// deciding of its governance included, or below it ([walk.element]'s mark).
// Each value is [walk.elementBelow]'s; where e has none, the ancestor takes the
// lack instead ([lackingElementBelow]) and is told nothing, so its assertions
// are declined and its Tallies, left owing the value, are never read.
func (w *walk) tallyValue(e Element, g governance, content *contentCheck, up assertionAncestry, recorded bool) {
	// chain is on [walk.tallyElement]'s shared-array terms: e's subtree, which
	// wrote the slots after e's, is walked, and e's own slot still holds its
	// name.
	chain := append(up.path, e.Name())
	for f := up.counting; f != nil; f = f.outer {
		path := chain[f.depth+1:]
		if !f.check.readsElementValue(path) {
			continue
		}
		v, why := w.elementBelow(f.check.ct, path, e, g, content, recorded)
		if why != "" {
			f.check.lacking(lackingElementBelow{name: e.Name(), loc: e.Loc(), why: why})
			continue
		}
		f.check.elementValue(path, v)
	}
}

// elementBelow is the typed value (xpath-datamodel §6.2.4) of e, an element at
// path below the element whose ·governing type definition· is ct, as a {test}
// of that element reads it through a path of child steps, or why it has none.
// The value is mapped under T, the type [walk.assertionElementTypes] answers
// for path — the one closure the {test} was compiled against, so the compiled
// type and the mapped type cannot disagree, which [xpath.Tally.ElementValue]
// makes the caller's obligation — on [walk.childValue]'s pattern: e's ·initial
// value· normalized under its own type and mapped under the simple type T has
// its value in. A ·nilled· e is nil, the empty sequence. It reports why
// instead where:
//
//   - recorded: e is not known to be ·valid·, on [walk.childValue]'s terms;
//   - e has no ·governing type definition· — ·laxly assessed·, or one this
//     package could not determine;
//   - e's own ·governing type definition· is neither T nor validly derived
//     from it ([xsd.Schema.ValidlySubstitutable] with no blocking keyword): T
//     is STATIC, read off the ·locally declared type· chain, and a type an
//     element is governed by — an xsi:type, a {type table}'s selection, a
//     declaration a wildcard or substitution group resolves to — can differ
//     from it, the more so below a child whose own type differs from the
//     chain's. The relation is tested on e's own type directly, never through
//     the ·locally declared type· within its parent's own type;
//   - its [schema normalized value] under its own type does not map under T.
func (w *walk) elementBelow(ct xsd.ComplexType, path []xsd.QName, e Element, g governance, content *contentCheck, recorded bool) (xpath.TypedValue, string) {
	if recorded {
		return nil, "a violation or an unevaluated check was recorded for it or below it, so it is not known to be ·valid·"
	}
	compiled, typed := w.assertionElementTypes(ct)(path)
	answered := valueTypeOf(compiled)
	if !typed || answered == nil {
		// Unreachable: a {test} reads the value only where the compile typed it.
		return nil, "the {test} was compiled with no simple type for it"
	}
	if g.typ == nil {
		return nil, "it has no ·governing type definition·"
	}
	if !sameType(g.typ, compiled) {
		derived, err := w.schema.ValidlySubstitutable(g.typ, compiled, nil)
		if err != nil || !derived {
			return nil, fmt.Sprintf("its ·governing type definition· %s is neither %s, the type the {test} was compiled to read, nor validly derived from it", typeName(g.typ), typeName(compiled))
		}
	}
	if content.nilled {
		return nil, ""
	}
	own := g.valueType()
	if own == nil {
		return nil, "its ·governing type definition· has no simple type its value is read under"
	}
	lexical, ctx := content.assessed()
	normalized, ok := normalizedLexical(w.schema, own, lexical)
	if !ok {
		return nil, "its ·initial value· has no [schema normalized value] under its own type"
	}
	v, err := value.ValidateLexical(w.backend, w.schema, answered, normalized, ctx, xpath.FacetAssertions(w.now))
	if err != nil {
		return nil, fmt.Sprintf("its [schema normalized value] has no ·actual value· under %s", typeName(answered))
	}
	return xpath.Typed(v), ""
}

// attributeNodes is the names of e's attribute nodes in the data model instance
// cvc-assertion clause 1 builds: each attribute e carries — xsi ones included,
// namespace declarations being none ([Element.Attributes]) — then each of its
// ·defaulted attributes· (key-dflt-att, [walk.defaultedConstraint]) under g's
// complex ·governing type definition·. decided is false where this package
// could not determine that type ([governance.typeUndetermined]), so which
// uses default is unknown. An element whose type is simple, or which is ·laxly
// assessed· and so ·governed by· no type, has no ·defaulted attribute·.
func (w *walk) attributeNodes(e Element, g governance) (names []xsd.QName, decided bool) {
	if g.typeUndetermined() {
		return nil, false
	}
	attrs := e.Attributes()
	for _, a := range attrs {
		names = append(names, a.Name())
	}
	ct := g.complexType()
	if ct == nil {
		return names, true
	}
	for _, u := range ct.AttributeUses() {
		if _, defaulted := w.defaultedConstraint(u, attrs); defaulted {
			names = append(names, u.DeclarationName())
		}
	}
	return names, true
}

// yieldChildren is the kept children as an [xpath.ChildElements].
func (c *assertionCheck) yieldChildren(yield func(xpath.ChildElement) bool) {
	for _, child := range c.children {
		if !yield(child) {
			return
		}
	}
}

// assertionTest is one member of ct.{assertions}, its {test} as
// [xpath.CompileAssertionTest] compiled it, nil where that declined, and the
// [xpath.Tally] that test's evaluation reads ([xpath.AssertionTest.Tally]),
// nil where it counts nothing.
type assertionTest struct {
	a     xsd.Assertion
	test  *xpath.AssertionTest
	tally *xpath.Tally
}

// compileAssertions compiles every assertion of g's complex ·governing type
// definition· when e is entered, and is nil where there is none to compile: g
// is not complex, or its {assertions} is empty. Every input a compile reads is
// STATIC — the {test}, T's {content type}, T's {attribute uses} and the
// ·locally declared types· within T ([walk.assertionElementTypes]) — so
// nothing the [[children]] carry can change a compiled tree, and compiling
// before they arrive is what lets the walk know which of them a {test} reads
// ([assertionCheck.reads]) and which {test}s count nodes of e's subtree, each
// of which gets its Tally here, fresh for e ([assertionCheck.counts]).
//
// {assertions} is read whole and its base chain is never walked: cos-ct-extends
// clause 1.7 and derivation-ok-restriction clause 5 both make B.{assertions} a
// prefix of T.{assertions}, so unioning the chain here would report every
// inherited assertion once per derivation step. Every one of them, inherited
// or not, is compiled against T's OWN {attribute uses} ([walk.assertionTypes]),
// because the instance it reads is e's as T types it: a restriction that
// narrows an attribute's type narrows it for the base's assertions too, and
// against T's {content type}, which fixes `$value`'s static type
// (cvc-assertion clause 2.3.1.3). Each is compiled per element and cached
// nowhere.
func (w *walk) compileAssertions(g governance) *assertionCheck {
	ct := g.complexType()
	if ct == nil || len(ct.Assertions()) == 0 {
		return nil
	}
	check := &assertionCheck{ct: *ct}
	for _, a := range ct.Assertions() {
		check.tests = append(check.tests, w.compileAssertion(a, *ct))
	}
	return check
}

// compileAssertion compiles a's {test} for an element whose ·governing type
// definition· is ct, on [walk.compileAssertions]' terms, under ct's {content
// type}. Whether the compiled {test} reads `.`, and so has E's own string value
// collected for it ([ownStringValue]), is
// [xpath.AssertionTest.ReadsContextItem]'s answer, which
// [assertionCheck.readsContextItem] reads.
func (w *walk) compileAssertion(a xsd.Assertion, ct xsd.ComplexType) assertionTest {
	c := assertionTest{a: a}
	if test, compiled := xpath.CompileAssertionTest(a.Test(), w.schema, ct.ContentType(), w.assertionTypes(ct), w.assertionElementTypes(ct)); compiled {
		c.test = &test
		c.tally = test.Tally()
	}
	return c
}

// assertionType is the {type definition} of the {attribute declaration} of
// the use u, which an assertion {test} reads that use's attribute as. resolved
// is false where the declaration or the type does not resolve to a simple
// type. It is the ONE lookup [walk.assertionTypes] and [walk.assertionValues]
// share, so the type a {test} is compiled against and the arm and type its
// values are yielded under cannot disagree, which [xpath.TypedAttributes] makes
// the caller's obligation.
func (w *walk) assertionType(u xsd.AttributeUse) (st *xsd.SimpleType, resolved bool) {
	d, resolved := w.schema.ResolvedAttributeDeclaration(u)
	if !resolved {
		return nil, false
	}
	st, simple := w.schema.ResolvedSimpleType(d.TypeDefinition())
	if !simple {
		return nil, false
	}
	return st, true
}

// assertionTypes is the [xpath.AttributeTypes] of an element whose
// ·governing type definition· is ct: the type of the {attribute use} matching
// the name (cvc-complex-type clause 2.1's match, attributeUseNamed) of ct for
// the empty path, and of the STATIC type of the element at a non-empty path
// below it ([walk.staticTypeBelow]), on [walk.assertionType]'s terms. A path
// with no such static type, and a name no use matches — only an {attribute
// wildcard} can admit it — have no type fixed at compile time and answer
// false, as does a use whose declaration or type does not resolve.
//
// The answer does not depend on whether the element carries the attribute: a
// carried one and a ·defaulted attribute· (key-dflt-att) are both in the
// partial ·PSVI· cvc-assertion clause 1.2 builds from, and both are yielded
// ([walk.assertionValues]) or, below the element, reported
// ([walk.tallyAttributes]); a use neither carried nor defaulted is absent, and
// the empty sequence is what the {test} reads. It is the one closure the
// compile and [walk.attributeBelow]'s mapping both read.
func (w *walk) assertionTypes(ct xsd.ComplexType) xpath.AttributeTypes {
	return func(path []xsd.QName, name xsd.QName) (*xsd.SimpleType, bool) {
		owner, static := w.staticTypeBelow(ct, path)
		if !static {
			return nil, false
		}
		u, matched := attributeUseNamed(owner.AttributeUses(), name)
		if !matched {
			return nil, false
		}
		return w.assertionType(u)
	}
}

// staticTypeBelow is the STATIC type of the element at path below an element
// whose ·governing type definition· is ct: ct itself for the empty path, and
// otherwise the ·locally declared type· (key-ldt-elem,
// [xsd.Schema.LocallyDeclaredElementType]) of path[0] within ct, then of each
// later step within the type found for the step before. It is false where a
// step's type is ·absent· or is not a complex type, within which no later step
// and no {attribute use} has a type.
func (w *walk) staticTypeBelow(ct xsd.ComplexType, path []xsd.QName) (xsd.ComplexType, bool) {
	for _, step := range path {
		ldt, local := w.schema.LocallyDeclaredElementType(ct, step)
		if !local {
			return xsd.ComplexType{}, false
		}
		next, isComplex := ldt.(xsd.ComplexType)
		if !isComplex {
			return xsd.ComplexType{}, false
		}
		ct = next
	}
	return ct, true
}

// assertionElementTypes is the [xpath.ElementTypes] of an element whose
// ·governing type definition· is ct: the ·locally declared type·
// ([xsd.Schema.LocallyDeclaredElementType], key-ldt-elem) of the path's last
// step within the STATIC type [walk.staticTypeBelow] reaches through the steps
// before it — ct itself for a one-step path, so a child's answer is its
// ·locally declared type· within ct — and false where that is ·absent·, where
// a step before the last has no complex static type, and for the empty path.
// The last step's type may be simple or complex. [walk.keepChild] reads the
// same lookup for a child, and [walk.tallyValue] reads this closure for an
// element below one, so the type a {test} is compiled against and the one a
// value is mapped under cannot disagree, which [xpath.ChildElements] and
// [xpath.Tally.ElementValue] make the caller's obligation.
func (w *walk) assertionElementTypes(ct xsd.ComplexType) xpath.ElementTypes {
	return func(path []xsd.QName) (xsd.TypeDefinition, bool) {
		if len(path) == 0 {
			return nil, false
		}
		owner, static := w.staticTypeBelow(ct, path[:len(path)-1])
		if !static {
			return nil, false
		}
		return w.schema.LocallyDeclaredElementType(owner, path[len(path)-1])
	}
}

// keepChild hands e, an element [[child]] of the element whose clause 6 state
// is parent, to that state once e's own assessment is over, where some {test}
// of the parent reads e's value or, through a range variable bound to e, its
// attributes: one [xpath.ChildElement] carrying e's typed value ([walk.childValue])
// where a {test} reads it and nil where none does, and the typed values of the
// attributes a {test} reads ([walk.childAttributes]), or the lack that declines the
// parent's assertions. A child read for its attributes alone is never asked its value,
// so a value childValue could not give declines nothing there. frame is e's
// string-value frame, nil where e opened none ([walk.stringValue]). recorded reports
// whether any violation or [Unevaluated] was recorded for e — the deciding of its
// governance included, so a cvc-elt clause 4 charge counts ([walk.element]'s mark)
// — or anywhere below it.
//
// A child no {test} reads is kept nowhere, and a child that is read is kept as
// ONE value and the values of the attributes of its own a {test} reads, never a
// subtree: no value step xpath compiles reaches below a child but through the
// string-value of a child of mixed content, which its frame collected as the
// subtree streamed past and which is one string here, so the walk holds at
// most one value, and one per read attribute, per element [[child]] of an
// element whose {assertions} read it, for the data model instance cvc-assertion clause
// 1 builds from the parent; an fn:count over a child step filtered by a
// predicate over its value reads it here too, and counts the kept values. A
// {test} that only counts a child, filtered by its attributes or not, or only
// asks its existence or that of a child path through it, reads its [xpath.Tally]
// instead ([walk.tallyElement]), and the child never comes through here for it.
func (w *walk) keepChild(parent *assertionCheck, e Element, g governance, content *contentCheck, frame *stringValueFrame, recorded bool) {
	valued := parent.reads(e.Name())
	names := parent.childAttributesRead(e.Name())
	if !valued && len(names) == 0 {
		return
	}
	var v xpath.TypedValue
	if valued {
		var lack assertionLack
		if v, lack = w.childValue(parent.ct, e, g, content, frame, recorded); lack != nil {
			parent.lacking(lack)
			return
		}
	}
	attrs, lack := w.childAttributes(parent, names, e, g)
	if lack != nil {
		parent.lacking(lack)
		return
	}
	parent.children = append(parent.children, xpath.ChildWithAttributes(e.Name(), v, assertionInput{attrs: attrs}.yield))
}

// childAttributes is the typed value of each attribute of e, an element
// [[child]] of the element whose clause 6 state is parent, that some {test}
// of the parent reads through a range variable bound to e (names,
// [assertionCheck.childAttributesRead]), on [walk.attributesRead]'s terms at
// the one-step path of e's name, which is the lookup the {test} was compiled
// against ([xpath.ChildElements]). An attribute e does not have is not among
// them. It reports the lack instead where one has no typed value under that
// type, or where g's type was not determined, so that which uses default is
// unknown.
func (w *walk) childAttributes(parent *assertionCheck, names []xsd.QName, e Element, g governance) ([]assertionValue, assertionLack) {
	if len(names) == 0 {
		return nil, nil
	}
	if g.typeUndetermined() {
		return nil, lackingChild{name: e.Name(), loc: e.Loc(), why: "its ·governing type definition· was not determined, so its ·defaulted attributes·, whose values a {test} reads, are unknown"}
	}
	var read []assertionValue
	reads := func(name xsd.QName) bool { return slices.Contains(names, name) }
	keep := func(name xsd.QName, v xpath.TypedValue) { read = append(read, assertionValue{name: name, v: v}) }
	if lack := w.attributesRead(parent.ct, []xsd.QName{e.Name()}, e, g, reads, keep); lack != nil {
		return nil, lack
	}
	return read, nil
}

// childValue is e's typed value (xpath-datamodel §6.2.4) as a {test} of its
// parent, whose ·governing type definition· is ct, reads it: under the simple
// type the ·locally declared type· of e within ct has its value in
// ([walk.assertionElementTypes], valueTypeOf), which is the type the {test}
// was compiled against — or, where that type has mixed content, as
// [xpath.Untyped] of e's string-value, which frame collected
// ([walk.stringValue]). It reports a lackingChild instead where e's own
// assessment leaves that value unread:
//
//   - recorded: a violation makes e invalid in the partial ·PSVI·, and an
//     [Unevaluated] — String Valid over its ·initial value· withheld among
//     them — leaves its validity undecided;
//   - e has no ·governing type definition· — ·laxly assessed·, or one this
//     package could not determine — or one that is neither the same as nor
//     ·validly substitutable· ·without limitation· for the ·locally declared
//     type· ([xsd.Schema.ValidlySubstitutable] with no blocking keyword). That
//     is cvc-complex-type clause 5's own test, repeated here because
//     [walk.locallyDeclaredType] decides that clause in [walk.child], before
//     e's frame opens, so its charge is not among those recorded covers. Once
//     it holds, the simple type g.valueType() answers is the answered type
//     or validly derived from it (cos-st-derived-ok), whose lexical mapping is
//     a subset of the answered type's: an extension keeps its base's {simple
//     type definition} (cos-ct-extends clauses 1.4.1 and 2.1), and a
//     restriction's is validly derived from its base's (derivation-ok-restriction
//     clause 2.2.2.1; its clause 2.2.2.2 needs a mixed base, which answers no
//     simple type);
//   - the [schema normalized value] under e's own type does not map under the
//     answered type;
//   - the ·locally declared type· has mixed content and e's own ·governing
//     type definition· has none — an xsi:type naming a simple or an
//     element-only type — so e's typed value is not the string-value the
//     {test} was compiled to read.
//
// A ·nilled· e's is nil, the empty sequence, whatever its type (xpath20.md
// §2.5.2 item 4.1). A mixed one's is [xpath.Untyped] of frame's text,
// in which a descendant whose contribution is undecided has already lacked the
// parent ([stringValueFrame.lacking]). Otherwise the value is e's ·initial
// value· ([contentCheck.assessed], the {value constraint}'s {lexical form}
// where cvc-elt clause 5.1 substituted it), normalized under e's own type's
// whiteSpace (normalizedLexical) — an extension keeps it and a restriction
// never weakens it — and mapped under the answered type, in the namespace
// context assessed pairs it with.
func (w *walk) childValue(ct xsd.ComplexType, e Element, g governance, content *contentCheck, frame *stringValueFrame, recorded bool) (xpath.TypedValue, assertionLack) {
	lacking := func(why string) (xpath.TypedValue, assertionLack) {
		return nil, lackingChild{name: e.Name(), loc: e.Loc(), why: why}
	}
	if recorded {
		return lacking("a violation or an unevaluated check was recorded for it or below it, so it is not known to be ·valid·")
	}
	ldt, local := w.schema.LocallyDeclaredElementType(ct, e.Name())
	mixed := local && mixedContent(ldt)
	answered := valueTypeOf(ldt)
	if !local || !mixed && answered == nil {
		return lacking("its ·locally declared type· has neither a simple type its value is read under nor mixed content")
	}
	if g.typ == nil {
		return lacking("it has no ·governing type definition·")
	}
	if !sameType(g.typ, ldt) {
		substitutable, err := w.schema.ValidlySubstitutable(g.typ, ldt, nil)
		if err != nil || !substitutable {
			return lacking(fmt.Sprintf("its ·governing type definition· %s is neither its ·locally declared type· %s nor ·validly substitutable· for it", typeName(g.typ), typeName(ldt)))
		}
	}
	if content.nilled {
		return nil, nil
	}
	if mixed {
		if !mixedContent(g.typ) {
			return lacking(fmt.Sprintf("its ·governing type definition· %s has no mixed content, so its typed value is not the string-value its ·locally declared type· %s is read as", typeName(g.typ), typeName(ldt)))
		}
		if frame == nil {
			// Unreachable: walk.element opens a frame for every child a {test}
			// reads whose ·locally declared type· and ·governing type definition·
			// both have mixed content and which is not ·nilled·
			// ([walk.stringValue]).
			return lacking("its string-value was not collected")
		}
		return xpath.Untyped(frame.text.String()), nil
	}
	own := g.valueType()
	if own == nil {
		return lacking("its ·governing type definition· has no simple type its value is read under")
	}
	lexical, ctx := content.assessed()
	normalized, ok := normalizedLexical(w.schema, own, lexical)
	if !ok {
		return lacking("its ·initial value· has no [schema normalized value] under its own type")
	}
	v, err := value.ValidateLexical(w.backend, w.schema, answered, normalized, ctx, xpath.FacetAssertions(w.now))
	if err != nil {
		return lacking(fmt.Sprintf("its [schema normalized value] has no ·actual value· under %s", typeName(answered)))
	}
	return xpath.Typed(v), nil
}

// assertionValue is one attribute's typed value as an assertion {test} reads
// it.
type assertionValue struct {
	name xsd.QName
	v    xpath.TypedValue
}

// assertionInput is what one element's assertions read: its typed attributes,
// in document order, and its string value and the binding cvc-assertion clause
// 2.3 gives `$value` ([xpath.ValueBinding]).
type assertionInput struct {
	attrs []assertionValue
	value xpath.ValueBinding
}

// yield is the attributes as an [xpath.TypedAttributes].
func (in assertionInput) yield(yield func(xsd.QName, xpath.TypedValue) bool) {
	for _, a := range in.attrs {
		if !yield(a.name, a.v) {
			return
		}
	}
}

// assertionLack is why an element's assertions cannot be evaluated at all: an
// input every {test} of the element would be evaluated over is undecided, and
// [xpath.AssertionTest.Evaluate] has no undecided answer to give. The arms are
// this file's, so the decline text is a capability of the lack and the one
// decline site never switches over them (STYLE T2).
type assertionLack interface {
	// declined states what is missing for the element named e, as the clause
	// the decline message completes.
	declined(e xsd.QName) string
}

// lackingAttribute is an attribute of the instance, matching an {attribute
// use}, that has no ·actual value· ([walk.assertionValues]).
type lackingAttribute struct{ a Attribute }

// lackingDefault is a ·defaulted attribute·, which has no attribute of the
// instance to carry, whose use's ·effective value constraint· has no ·actual
// value· ([walk.assertionValues]). The use is the whole of it: the name is its
// declaration's, and the {lexical form} its ·effective value constraint·'s.
type lackingDefault struct{ u xsd.AttributeUse }

// lackingValue is a simple {content type} whose `$value` is undecided
// ([walk.assertionValue]).
type lackingValue struct{}

// lackingText is a simple {content type} over an element with element
// [[children]], whose string value, which `.` reads, this walk did not gather
// ([walk.assertionValue]).
type lackingText struct{}

// lackingNilledText is a ·nilled· element with [[children]], whose string
// value, which `.` reads, this walk did not gather ([walk.assertionValue]).
type lackingNilledText struct{}

// lackingStringValue is an element whose string value, which `.` reads, spans
// a descendant whose contribution to it is undecided, for the reason why
// ([stringValueFrame.lacking]).
type lackingStringValue struct{ why string }

// lackingChild is an element [[child]], named name at loc, that a {test} reads
// and that has no typed value this package reads, for the reason why
// ([walk.childValue]).
type lackingChild struct {
	name xsd.QName
	loc  xsderr.Loc
	why  string
}

func (l lackingAttribute) declined(e xsd.QName) string {
	return fmt.Sprintf("the attribute %s of the element %s has no ·actual value· for the data model instance cvc-assertion clause 1 builds", l.a.Name(), e)
}

func (l lackingDefault) declined(e xsd.QName) string {
	return fmt.Sprintf("the ·defaulted attribute· %s of the element %s has no ·actual value· for the data model instance cvc-assertion clause 1 builds", l.u.DeclarationName(), e)
}

// lackingCount is an element, named name at loc, of the subtree of an element
// one of whose {test}s counts its nodes, which the counting [xpath.Tally]
// cannot be told of exactly, for the reason why: its ·governing type
// definition· was not determined, and a {test} counts its attribute nodes
// ([walk.tallyElement]).
type lackingCount struct {
	name xsd.QName
	loc  xsderr.Loc
	why  string
}

// lackingAttributeBelow is the attribute named attr of an element, named name
// at loc, of the subtree of an element one of whose {test}s reads that
// attribute's typed value through a path to it, which has no typed value that
// {test} reads, for the reason why ([walk.tallyAttributes],
// [walk.attributeBelow], [assertionAncestry.tallySkipped]).
type lackingAttributeBelow struct {
	attr xsd.QName
	name xsd.QName
	loc  xsderr.Loc
	why  string
}

// lackingElementBelow is an element, named name at loc, of the subtree of an
// element one of whose {test}s reads its typed value through a path of child
// steps, which has no typed value that {test} reads, for the reason why
// ([walk.tallyValue], [walk.elementBelow], [assertionAncestry.tallySkipped]).
type lackingElementBelow struct {
	name xsd.QName
	loc  xsderr.Loc
	why  string
}

func (l lackingElementBelow) declined(e xsd.QName) string {
	return fmt.Sprintf("the element %s at %s, in the subtree of the element %s, whose typed value a {test} reads, has no typed value for the data model instance cvc-assertion clause 1 builds: %s", l.name, l.loc, e, l.why)
}

func (l lackingAttributeBelow) declined(e xsd.QName) string {
	return fmt.Sprintf("the attribute %s of the element %s at %s, in the subtree of the element %s, whose typed value a {test} reads, has no typed value for the data model instance cvc-assertion clause 1 builds: %s", l.attr, l.name, l.loc, e, l.why)
}

func (l lackingCount) declined(e xsd.QName) string {
	return fmt.Sprintf("the element %s at %s, in the subtree of the element %s whose nodes a {test} counts, cannot be counted exactly for the data model instance cvc-assertion clause 1 builds: %s", l.name, l.loc, e, l.why)
}

func (l lackingChild) declined(e xsd.QName) string {
	return fmt.Sprintf("the child element %s of the element %s at %s, which a {test} reads, has no typed value for the data model instance cvc-assertion clause 1 builds: %s", l.name, e, l.loc, l.why)
}

func (lackingText) declined(e xsd.QName) string {
	return fmt.Sprintf("the element %s has simple content and element [[children]], which cvc-complex-type clause 1.2 charged, so its string value, the text of all its descendants, was not gathered", e)
}

func (lackingNilledText) declined(e xsd.QName) string {
	return fmt.Sprintf("the element %s has xsi:nil = true and [[children]], which cvc-elt clause 3.2.3.1 charged, so its string value, the text of its descendants and never the zero-length string while dm:nilled is false in the partial ·PSVI·, was not gathered", e)
}

func (l lackingStringValue) declined(e xsd.QName) string {
	return fmt.Sprintf("the string value of the element %s, which `.` reads in the data model instance cvc-assertion clause 1 builds, is undecided: %s", e, l.why)
}

func (lackingValue) declined(e xsd.QName) string {
	return fmt.Sprintf("the element %s has simple content whose [schema actual value], which cvc-assertion clause 2.3.1 binds to $value, is undecided: String Valid over its ·initial value· was withheld", e)
}

// assertionValues is the input of e's assertions: the typed value
// ([walk.assertionTyped]) of each attribute of attrs that matches an {attribute
// use} of ct, in document order, then of each ·defaulted attribute· of e
// ([walk.defaultedConstraint], key-dflt-att) in the order of ct.{attribute
// uses}, each under the type [walk.assertionType] resolves for its use, and
// the binding of `.` and `$value` ([walk.assertionValue]).
//
// A ·defaulted attribute· is read as its use's ·effective value constraint·
// supplies it: its {lexical form} is the [schema normalized value], and the
// ·actual value· is mapped from it under [value.ConstraintContext], the bindings
// in scope where the schema document wrote it (Datatypes §3.3.18), never e's.
// The partial ·PSVI· cvc-assertion clause 1.2 builds from holds it — clause 1.1
// sets aside only cvc-complex-type clause 6, not the attribute defaulting
// key-dflt-att's PSVI contribution makes — so reading it as the empty sequence
// instead could fabricate a charge.
//
// It reports the lack, carrying the first attribute lacking one, where any
// such attribute has no ·actual value·: its declaration or {type definition}
// does not resolve, or String Valid ([walk.stringValid]) over its lexical is
// rejected or withheld — charged or declined by cvc-attribute clause 3 for a
// carried attribute and by cvc-complex-type clause 4 for a defaulted one
// ([walk.defaultedAttribute]), the same check re-run here because the walk
// keeps no ·actual values·. Omitting such an attribute instead would make `@a`
// the empty sequence and could fabricate a charge. After the attributes it
// reports asserts' own lack: an element [[child]] a {test} reads that has no
// typed value, or an attribute of it a {test} reads that has none
// ([walk.keepChild]), a node of e's subtree a {test} counts that
// its Tally could not be told of ([walk.tallyElement]), or a descendant whose
// contribution to e's own string value, which own collected, is undecided
// ([stringValueFrame.lacking]).
//
// An attribute matching no use is not read: no {test}
// [xpath.CompileAssertionTest] admits can name it ([walk.assertionTypes]), so
// its own ·actual value· decides nothing here.
//
// A lack declines every assertion of e, including one whose {test} never reads
// what is lacking: whether a {test} reads `$value` is not something the
// compiled test reports, and a child's lack, like a count's, is kept per
// element and not per {test}.
func (w *walk) assertionValues(e Element, attrs []Attribute, asserts *assertionCheck, content *contentCheck, own *stringValueFrame, invalid bool) (assertionInput, assertionLack) {
	ct := asserts.ct
	var in assertionInput
	for _, a := range attrs {
		u, matched := attributeUseNamed(ct.AttributeUses(), a.Name())
		if !matched {
			continue
		}
		v, read := w.assertionTyped(u, a.Value(), elementContext{owner: e}, a.Loc())
		if !read {
			return assertionInput{}, lackingAttribute{a: a}
		}
		in.attrs = append(in.attrs, assertionValue{name: a.Name(), v: v})
	}
	for _, u := range ct.AttributeUses() {
		vc, defaulted := w.defaultedConstraint(u, attrs)
		if !defaulted {
			continue
		}
		v, read := w.assertionTyped(u, vc.LexicalForm(), value.ConstraintContext(vc), e.Loc())
		if !read {
			return assertionInput{}, lackingDefault{u: u}
		}
		in.attrs = append(in.attrs, assertionValue{name: u.DeclarationName(), v: v})
	}
	if asserts.lack != nil {
		return assertionInput{}, asserts.lack
	}
	bound, lack := w.assertionValue(e, asserts, content, own, invalid)
	if lack != nil {
		return assertionInput{}, lack
	}
	in.value = bound
	return in, nil
}

// assertionTyped is the typed value of an attribute of the use u whose
// [schema normalized value] is lexical, read at loc and mapped under ctx,
// reporting false where it has no ·actual value·: u's declaration or {type
// definition} does not resolve ([walk.assertionType]), String Valid
// ([walk.stringValid]) over lexical is rejected or withheld, or the mapping
// errors.
//
// The value is [xpath.Untyped] of lexical where the type is ·special·
// ([xsd.SimpleType.IsSpecial]): xpath-datamodel §3.3.1.2 makes it the [schema
// normalized value] as xs:untypedAtomic. A carried attribute's [[normalized
// value]] is that value unchanged, because key-nv normalizes under
// xs:anySimpleType "as in the preserve case" and xs:anyAtomicType carries no
// whiteSpace facet either; a defaulted one's {lexical form} is it by
// key-dflt-att. The value is [xpath.Typed] of the ·actual value· mapped under
// the type otherwise.
func (w *walk) assertionTyped(u xsd.AttributeUse, lexical string, ctx value.Context, loc xsderr.Loc) (xpath.TypedValue, bool) {
	st, resolved := w.assertionType(u)
	if !resolved {
		return nil, false
	}
	decided, verdict := w.stringValid(st, lexical, ctx, loc)
	if !decided || verdict != nil {
		return nil, false
	}
	if st.IsSpecial() {
		return xpath.Untyped(lexical), true
	}
	v, err := value.ValidateLexical(w.backend, w.schema, st, lexical, ctx, xpath.FacetAssertions(w.now))
	if err != nil {
		return nil, false
	}
	return xpath.Typed(v), true
}

// assertionValue is what e's assertions read of e itself — its string value,
// which `.` atomizes to, and the value cvc-assertion clause 2.3 binds to
// `$value` ([xpath.BindValue]) — or the lack that leaves either undecided.
//
// Under a {content type} that is not simple, the string value is the text own
// collected ([ownStringValue]): the concatenation of the string-values of all
// e's Text Node descendants in document order (xpath-datamodel §6.2.4, :1304)
// — every character [[child]] of e, white space alone included, e being
// annotated xs:anyType in the partial ·PSVI· (cvc-assertion clause 1.2), and
// each Text Node of a descendant as [assertionAncestry.collectText] builds it,
// a descendant whose contribution is undecided having lacked e's check
// already ([stringValueFrame.lacking]). An invalid e has a string value all
// the same, so it is bound beside clause 2.3.2's empty `$value`. own is nil
// there only where no compiled {test} reads `.`
// ([assertionCheck.readsContextItem]), and the binding is then the zero
// [xpath.ValueBinding]: [xpath.AssertionTest.ReadsContextItem] never
// under-reports, so no {test} evaluated over that binding reads its text, and
// its `$value` is clause 2.3.2's empty sequence whatever the binding.
//
// Under simple content, the string value is the ·initial value·
// [contentCheck.assessed] answers, or the {value constraint}'s {lexical form}
// cvc-elt clause 5.1 substitutes for an empty e, unnormalized. It is bound
// whatever `$value` is, an invalid e's included, but for the e below: `.` is
// E's string value whatever its [validity].
//
// GAP(xpath): E's string value under simple content is a processor's choice
// the data model leaves open, and this walk makes one reading of it (#2455).
// xpath-datamodel Appendix J.2's children rule builds a Text Node from the
// character [[children]], and also says a processor "may" build instead one
// Text Node from the [schema normalized value]; §6.2.4's string-value gives an
// EMPTY element the zero-length string. This walk binds the raw ·initial value·
// for an e with character [[children]], J.2's first reading, which the
// xs:anyType annotation of the partial ·PSVI· points at, and the {value
// constraint}'s {lexical form} for a defaulted e with none — the [schema
// normalized value] cvc-elt clause 5.1 supplies, J.2's "may" — and not the
// zero-length string §6.2.4 gives an empty element. The direction is
// unestablished: a {test} reading `.` can be charged or satisfied under either
// reading.
//
// A ·nilled· e binds the zero [xpath.ValueBinding] where it has no [[children]]
// ([contentCheck.empty]): the zero-length string, an empty element's string
// value (xpath-datamodel §6.2.4), and the empty `$value` of clause 2.3.1.2.
// dm:nilled is false in the partial ·PSVI· — [validity] is never valid there —
// so a ·nilled· e WITH [[children]], which cvc-elt clause 3.2.3.1 charged as
// the first arrived, has the text of its descendants as its string value, and
// this walk gathers none of it ([contentCheck.gathers]). Its assertions are
// declined ([lackingNilledText]) on the terms [walk.elementAssertions] states
// under simple content, a {test} reading only `$value` among them, since the
// lack is kept per element and not per {test}; and under any other {content
// type} where some compiled {test} reads `.`
// ([assertionCheck.readsContextItem]). Where none does, the binding is the zero
// [xpath.ValueBinding] as for an e that opened no own frame, whose `$value` is
// clause 2.3.2's empty sequence.
//
// An e under simple content that has element [[children]] — which
// cvc-complex-type clause 1.2 charged as the first arrived, so invalid is true
// — has a string value this walk never gathered: the text of its descendants,
// and every run after that charge ([contentCheck.text]). Its assertions are
// declined ([lackingText]) on the terms [walk.elementAssertions] states, never
// evaluated over the partial text.
//
// Clause 2.3.2's empty sequence — a nil `$value` — is decided from three facts
// this walk holds: ct's {content type} is not simple, e is ·nilled· (clause
// 2.3.1.2), or e is already known to be invalid (clause 2.3.1.1: the partial
// ·PSVI·'s [validity] "is given the value invalid if and only if the element is
// known to be invalid").
//
// Over a ·special· {simple type definition} ([xsd.SimpleType.IsSpecial])
// `$value` is [xpath.Untyped] of the ·initial value· [contentCheck.assessed]
// answers, which is e's [schema normalized value] unchanged — key-nv
// normalizes under xs:anySimpleType "as in the preserve case" — and whose XDM
// representation is that value as xs:untypedAtomic (Datatypes dt-xdmrep clause
// 1). String Valid accepts every lexical against either ·special· type, so it
// is not re-run.
//
// Otherwise `$value` is e's [schema actual value]: the ·initial value·, or the
// {value constraint}'s {lexical form} cvc-elt clause 5.1 substitutes for an
// empty e ([contentCheck.assessed]), mapped under the {simple type definition},
// in the namespace context assessed pairs it with, by String Valid
// ([walk.stringValid]) re-run as [walk.assertionValues] re-runs it for an
// attribute. It is undecided where String Valid is withheld. A rejection is
// clause 2.3.2 again, which cvc-complex-type clause 1.2 has charged by now.
//
// A DECLINED check of e's own elsewhere leaves e's [validity] undecided
// between invalid and notKnown, and the actual value is bound all the same:
// were e invalid, e is rejected whatever its assertions answer, so no answer
// they give turns a valid document invalid or an invalid one valid.
func (w *walk) assertionValue(e Element, asserts *assertionCheck, content *contentCheck, own *stringValueFrame, invalid bool) (xpath.ValueBinding, assertionLack) {
	simple, isSimple := asserts.ct.ContentType().(xsd.SimpleContent)
	if content.nilled && !content.empty() && (isSimple || asserts.readsContextItem()) {
		return xpath.ValueBinding{}, lackingNilledText{}
	}
	if content.nilled {
		return xpath.ValueBinding{}, nil
	}
	if !isSimple && own == nil {
		return xpath.ValueBinding{}, nil
	}
	if !isSimple {
		return xpath.BindValue(own.text.String(), nil), nil
	}
	if content.sawElement {
		return xpath.ValueBinding{}, lackingText{}
	}
	lexical, ctx := content.assessed()
	if invalid {
		return xpath.BindValue(lexical, nil), nil
	}
	if simple.SimpleType.IsSpecial() {
		return xpath.BindValue(lexical, xpath.Untyped(lexical)), nil
	}
	decided, verdict := w.stringValid(simple.SimpleType, lexical, ctx, e.Loc())
	if !decided {
		return xpath.ValueBinding{}, lackingValue{}
	}
	if verdict != nil {
		return xpath.BindValue(lexical, nil), nil
	}
	v, err := value.ValidateLexical(w.backend, w.schema, simple.SimpleType, lexical, ctx, xpath.FacetAssertions(w.now))
	if err != nil {
		return xpath.ValueBinding{}, lackingValue{}
	}
	return xpath.BindValue(lexical, xpath.Typed(v)), nil
}

// declineAssertions records err, the non-verdict [walk.stringValid] withheld a
// verdict on, as a decline under cvc-assertions-valid at the item named name at
// loc, under event, where it is an assertions-facet decline
// ([value.IsAssertionDeclined]), and reports whether it was one. withheld names
// the caller's clause the decline leaves undecided. The message is the one
// value's decline carries, naming the assertion, the simple type its facet is
// effective on and its {test}: an assertion component carries no Loc of its
// own (#35).
//
// cvc-assertions-valid (§4.3.13.3) is evaluated inside String Valid: the value
// pipeline runs each assertions facet it reaches through
// [xpath.FacetAssertions], so a failed {test} is part of the Datatype Valid
// verdict (cvc-datatype-valid clause 3) the caller charges under its own rule,
// and a union's ·validating type· — which [walk.validatingType],
// [walk.roleValues] and [walk.keyMember] read — is the first member whose
// facets, assertions included, accept the value (dt-active-member).
//
// GAP(validate): a {test} [xpath.FacetAssertions] declines — one outside the
// grammar [xpath.CompileAssertionTest] admits, over the facet's type or, for a
// union's own assertions facet, over its ·active basic member· — leaves the
// value's Datatype Valid verdict undecided, and is DECLINED here. The cvcid.go
// and cvcidentityconstraint.go re-runs reach the same non-verdict and decline
// on it, and the union dispatch stops at a member that declined rather than
// handing the value to a later one, so no ·validating type· is chosen past a
// {test} nobody decided. Fail-open: the withheld verdict's whole consumer set
// is w.res.violations and its one reader [Result.Violations], which charge on a
// violation PRESENT, so a decline can only cost a rejection and can manufacture
// none. (#1042)
func (w *walk) declineAssertions(err error, event string, name xsd.QName, loc xsderr.Loc, withheld string) bool {
	if !value.IsAssertionDeclined(err) {
		return false
	}
	w.decline(event, name, loc, ruleCvcAssertionsValid, "",
		"%s; that facet is reached from cvc-datatype-valid clause 3, so %s is undecided", declinedAssertion(err), withheld)
	return true
}

// declinedAssertion is the message of the *xsderr.Error value's decline err
// carries, which names the assertion it declined, or err's own rendering where
// it carries none.
func declinedAssertion(err error) string {
	var xe *xsderr.Error
	if errors.As(err, &xe) {
		return xe.Msg
	}
	return err.Error()
}
