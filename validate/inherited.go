package validate

import "github.com/kud360/goxsd8/xsd"

// This file is §3.3.5.6's [inherited attributes] (e-inherited_attributes) and
// nothing else. Its one reader is the XDM instance a Type Alternative's {test}
// evaluates against, §3.12.4 key-cta-ta-select clause 1.1.3 (ctaAttributes,
// cta.go).

// inheritedAttribute is one member of an element's [inherited attributes]: the
// ·expanded name· and [[normalized value]] of an attribute information item some
// ancestor carries, which is all clause 1.1.3's copy reads. The item's [[owner
// element]] is not kept: e-inherited_attributes clause 2 is the only rule that
// reads it, and [walk.handedDown] settles that clause by the order it composes
// in, while clause 1.1.3 gives the copy E as its owner.
type inheritedAttribute struct {
	name  xsd.QName
	value string
}

// handedDown is the [inherited attributes] of every element [[child]] of e
// (§3.3.5.6, e-inherited_attributes), where g is e's governance and inherited is
// e's OWN [inherited attributes]: first e's own attributes that are ·potentially
// inherited· (key-p-inherited), then each member of inherited whose ·expanded
// name· none of those has.
//
// key-p-inherited quantifies over "one of E's ancestors" at any depth, and
// e-inherited_attributes clause 2 keeps, of two same-named candidates, the one
// whose [[owner element]] is the descendant of the other's. Computed top-down,
// that reduces to this one-level composition: e's own candidates are the
// nearest any child has, so nothing shadows them, and every candidate from
// further up already had its shadowing by e's ancestors settled when inherited
// was computed for e. The one new edge is e's own candidates over inherited — and
// it is drawn by e's POTENTIALLY INHERITED attributes alone, so an attribute of
// e that is not inheritable shadows nothing above it.
//
// key-p-inherited clause 2, the same [validation context], holds for every pair
// this is asked of: one [Validator.Assess] call has one ·validation root·, which
// is the [validation context] of every item below it, and the root is handed
// nil. Validating two subtrees of one infoset as separate roots in one call
// would make the clause load-bearing, and nothing here could tell them apart.
//
// The property exists only for a child that is not attributed to a skip
// Wildcard, of a parent that is not ·skipped·. Nothing here tests either,
// because neither can arise: [walk.child] stops before [walk.element] for such a
// child, and a ·skipped· parent is never walked at all.
//
// An e whose ·governing type definition· this package could not determine
// hands down a set read off the top-level declarations alone, which a use of
// its real type might contradict. No reader sees it: every element [[child]]
// of such an e, ·nilled· or not, is [walk.child]'s undecided shape
// ([contentCheck.element]), which consults no {type table} and hands its own
// [[children]] the same undecided shape, so no element below e is typed (#2211).
// A ·laxly assessed· e hands down the same top-level reading, and there it is
// the spec's: xs:anyType has no {attribute uses} and a lax {attribute wildcard}
// (§3.4.7), so key-governing-ad clause 3 resolves each attribute by name, and
// the {type table} of a child that ·resolves· reads it ([walk.child]).
func (w *walk) handedDown(e Element, g governance, inherited []inheritedAttribute) []inheritedAttribute {
	attrs := e.Attributes()
	var own []inheritedAttribute
	for _, a := range attrs {
		if w.inheritable(g, a) {
			own = append(own, inheritedAttribute{name: a.Name(), value: a.Value()})
		}
	}
	// key-p-inherited counts an attribute "defaulted as described in Attribute
	// Default Value (§3.4.5.1)" alongside a specified one, and a ·defaulted
	// attribute· is attributed to the use that supplies it, so clause 3.1 reads
	// that use's {inheritable}.
	if ct := g.complexType(); ct != nil {
		for _, u := range ct.AttributeUses() {
			if !w.schema.ResolvedInheritable(u) {
				continue
			}
			vc, defaulted := w.defaultedConstraint(u, attrs)
			if !defaulted {
				continue
			}
			own = append(own, inheritedAttribute{name: u.DeclarationName(), value: vc.LexicalForm()})
		}
	}
	if len(own) == 0 {
		return inherited
	}
	nearest := len(own)
	for _, a := range inherited {
		if inheritedNamed(own[:nearest], a.name) {
			continue
		}
		own = append(own, a)
	}
	return own
}

// inheritable is key-p-inherited clause 3 for one attribute information item a
// of an element whose governance is g: the {inheritable} of the Attribute Use a
// is ·attributed to· (clause 3.1), or where it is attributed to none, the
// {inheritable} of its ·governing attribute declaration· (clause 3.2).
//
// The attribution is [walk.attributeType]'s (cvcid.go), arm for arm but one, and
// reads the same three encodings: attributeUseNamed for clause 2.1's match,
// skippedAttribute for the item key-governing-ad (§3.2.4.2) clause 3 leaves
// with no declaration, and the top-level resolution for every other. The two
// differ in what they read off the result — attributeType the {type
// definition} of the use's resolved declaration, this the use's {inheritable}
// ([xsd.Schema.ResolvedInheritable]), which clause 3.1 names and which may
// differ from its declaration's: a use's own inheritable attribute wins over
// the declaration's (§3.2.2.3 ref.att.local).
//
// The one arm this does not share is attributeType's decline for an element
// whose ·governing type definition· this package could not determine
// ([governance.typeUndetermined]): this reads such an element's attributes by
// ·expanded name·, although key-governing-ad clause 2's ·context-determined
// declaration·, from a use of the undetermined type, may come first. It needs
// no decline, because its one reader is [walk.handedDown], whose result for
// such an element no {type table} reads: every child of it, a ·nilled· one's
// included, is walked undecided ([contentCheck.element], #2211).
func (w *walk) inheritable(g governance, a Attribute) bool {
	if ct := g.complexType(); ct != nil {
		if u, matched := attributeUseNamed(ct.AttributeUses(), a.Name()); matched {
			return w.schema.ResolvedInheritable(u)
		}
	}
	if w.skippedAttribute(g, a) {
		return false
	}
	d, resolved := w.schema.Attribute(a.Name())
	return resolved && d.Inheritable()
}

// inheritedNamed reports whether as holds a member whose ·expanded name· is n.
func inheritedNamed(as []inheritedAttribute, n xsd.QName) bool {
	for _, a := range as {
		if a.name == n {
			return true
		}
	}
	return false
}
