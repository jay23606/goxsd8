package jsonsrc

import (
	"slices"

	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/xsd"
)

// This file is §3.3.5.6's [inherited attributes] (e-inherited_attributes) as
// the mapping determines them before the engine sees the infoset: its one
// reader is the selection of a {type table} below the root
// (walker.selector, walker.attributes), whose {test}s read them beside an
// element's own [attributes] (key-cta-ta-select clause 1.1.3).

// inheritor is the four methods of *xsd.Schema walker.handDown reads beyond
// validate.SchemaView: xsd.Schema.ResolvedInheritable, an attribute use's
// {inheritable} (key-p-inherited clause 3.1); xsd.Schema.Attribute, the
// top-level declaration an attribute's name ·resolves· to, whose {inheritable}
// clause 3.2 reads; xsd.Schema.AllowsAttributeWildcardName, whether a skip
// {attribute wildcard} ·skips· an attribute, which leaves it no declaration;
// and xsd.Schema.EffectiveValueConstraint, the value a ·defaulted attribute·
// takes. Reading them through this capability keeps one encoding of each
// (STYLE T4) without widening SchemaView's exported surface, as governor
// does. Under a view without it, which validate.Validator.Schema never
// returns, the mapping determines no [inherited attributes] below the root.
type inheritor interface {
	ResolvedInheritable(u xsd.AttributeUse) bool
	Attribute(name xsd.QName) (xsd.AttributeDeclaration, bool)
	AllowsAttributeWildcardName(w xsd.Wildcard, name xsd.QName) bool
	EffectiveValueConstraint(u xsd.AttributeUse) (xsd.ValueConstraint, bool)
}

// handDown is the [inherited attributes] of every element child of the
// element whose [attributes] are attrs, whose members are classified against
// o, and whose own [inherited attributes] are inherited, where known says the
// mapping determines those; and whether the mapping determines the result.
//
// It determines them where every one of o.governing, the types that may
// govern the element, hands down the same ones (handedUnder), so that the
// one the engine determines does too; and not where o.governing is empty or
// inherited is not known. An element carrying a "$type" hands down under the
// types walker.instanceGoverning finds may govern it, or walker.overrode and
// walker.governOverriding settle once its attributes are read, and not under
// the one its members are classified under before them
// (walker.instanceGoverned), which is the type the "$type" names, standing
// for several, where its declaration's {type table} can select types that
// settle differently.
//
// key-p-inherited clause 2, the same [validation context], holds for every
// pair this is asked of: one mapping builds one infoset under one ·validation
// root·.
//
// handedUnder and inheritable restate validate's walk.handedDown and
// walk.inheritable, which hand down under the one governance the engine has
// determined for an element it is walking. The mapping runs before the engine
// and holds several types that may govern an element, so it composes under
// each and compares, which no export of validate answers (#2791).
func (w *walker) handDown(attrs []validate.Attribute, o *object, inherited []validate.Attribute, known bool) ([]validate.Attribute, bool) {
	s, ok := w.schema.(inheritor)
	if !ok || !known || len(o.governing) == 0 {
		return nil, false
	}
	handed := handedUnder(s, attrs, o.governing[0].typ, inherited)
	for _, gt := range o.governing[1:] {
		if !slices.EqualFunc(handed, handedUnder(s, attrs, gt.typ, inherited), sameAttribute) {
			return nil, false
		}
	}
	return handed, true
}

// handedUnder is the [inherited attributes] of every element child of an
// element governed by t, whose [attributes] are attrs and whose own
// [inherited attributes] are inherited (e-inherited_attributes): first its
// attributes that are ·potentially inherited· (key-p-inherited) — each of
// attrs that is inheritable, then each ·defaulted attribute· (§3.4.5.1) whose
// use's {inheritable} is true, a ·defaulted attribute· being attributed to the
// use that supplies it (clause 3.1) — then each of inherited whose ·expanded
// name· none of those has.
//
// Computed top-down, e-inherited_attributes clause 2 — of two same-named
// candidates, the one whose [[owner element]] is the other's descendant — is
// this one-level composition: the element's own candidates are the nearest
// any child has, and every candidate from further up had its shadowing
// settled when inherited was computed. A non-inheritable attribute of the
// element shadows nothing above it.
func handedUnder(s inheritor, attrs []validate.Attribute, t xsd.TypeDefinition, inherited []validate.Attribute) []validate.Attribute {
	ct, complex := t.(xsd.ComplexType)
	var own []validate.Attribute
	for _, a := range attrs {
		if inheritable(s, ct, complex, a) {
			own = append(own, &attribute{name: a.Name(), value: a.Value()})
		}
	}
	if complex {
		for _, u := range ct.AttributeUses() {
			if !s.ResolvedInheritable(u) {
				continue
			}
			vc, defaulted := defaultedConstraint(s, u, attrs)
			if !defaulted {
				continue
			}
			own = append(own, &attribute{name: u.DeclarationName(), value: vc.LexicalForm()})
		}
	}
	if len(own) == 0 {
		return inherited
	}
	nearest := len(own)
	for _, a := range inherited {
		if slices.ContainsFunc(own[:nearest], func(b validate.Attribute) bool { return b.Name() == a.Name() }) {
			continue
		}
		own = append(own, a)
	}
	return own
}

// inheritable is key-p-inherited clause 3 for one attribute a of an element
// governed by ct, where complex says the governing type is complex: the
// {inheritable} of the attribute use of ct a matches by ·expanded name·
// (clause 3.1, cvc-complex-type clause 2.1); false where a skip {attribute
// wildcard} of ct ·skips· it, leaving it no declaration; and otherwise the
// {inheritable} of the top-level declaration its name ·resolves· to (clause
// 3.2, key-governing-ad clause 3), false where it resolves to none.
func inheritable(s inheritor, ct xsd.ComplexType, complex bool, a validate.Attribute) bool {
	if complex {
		for _, u := range ct.AttributeUses() {
			if u.DeclarationName() == a.Name() {
				return s.ResolvedInheritable(u)
			}
		}
		if wild, ok := ct.AttributeWildcard(); ok && wild.ProcessContents() == xsd.ProcessSkip && s.AllowsAttributeWildcardName(wild, a.Name()) {
			return false
		}
	}
	d, ok := s.Attribute(a.Name())
	return ok && d.Inheritable()
}

// defaultedConstraint reports whether u, an attribute use of the type
// governing an element whose [attributes] are attrs, is a ·defaulted
// attribute· of it (key-dflt-att clauses 2 to 5): not required, its
// ·effective value constraint· present, not in the xsi namespace — no
// declaration in it but the four clause 4 names exists (§3.2.6.4 no-xsi) —
// and no attribute of attrs of its ·expanded name·; and if so, that
// constraint, whose {lexical form} is the value Attribute Default Value
// (§3.4.5.1) supplies.
func defaultedConstraint(s inheritor, u xsd.AttributeUse, attrs []validate.Attribute) (xsd.ValueConstraint, bool) {
	if u.Required() {
		return xsd.ValueConstraint{}, false
	}
	vc, constrained := s.EffectiveValueConstraint(u)
	if !constrained || u.DeclarationName().Space == xsd.XMLSchemaInstanceNS {
		return xsd.ValueConstraint{}, false
	}
	if slices.ContainsFunc(attrs, func(a validate.Attribute) bool { return a.Name() == u.DeclarationName() }) {
		return xsd.ValueConstraint{}, false
	}
	return vc, true
}

// sameAttribute reports whether a and b are of one ·expanded name· and one
// value, all a {test} reads of an inherited attribute.
func sameAttribute(a, b validate.Attribute) bool {
	return a.Name() == b.Name() && a.Value() == b.Value()
}
