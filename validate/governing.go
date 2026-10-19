package validate

import "github.com/kud360/goxsd8/xsd"

// This file is the one encoding of an element's governance (Structures
// §3.3.4.6): which ·governing element declaration· and ·governing type
// definition· a child, or the ·validation root·, is assessed against, and
// whether it is assessed at all. [walk.childGoverning] reads it and charges
// each exit ([walk.charged]); [Validator.Governing] reads it for a source
// adapter that must know the governance before the engine sees the element.

// governed is the outcome decideGovernance reaches for one element, a sealed
// sum (STYLE T2) with one arm per exit the walk charges, declines or takes
// without a word. Each arm carries what its reader needs and nothing a later
// comparison could recover.
type governed interface{ governed() }

// skippedGovernance is an element ·attributed to· a skip Wildcard, or to an
// {open content} whose {wildcard} is skip: it is not ·assessed·
// (cvc-assess-elt clause 2), and neither is any element below it
// (key-skipped). The {open content} half is a reading, which
// [Validator.Governing] states.
type skippedGovernance struct{}

// decidedGovernance is an element whose ·governing type definition· typ is
// determined: hasDecl says whether decl is its ·governing element
// declaration·, and instance whether typ is its ·instance-specified type
// definition· ([governance.instance]). typ is never nil.
type decidedGovernance struct {
	decl     xsd.ElementDeclaration
	hasDecl  bool
	typ      xsd.TypeDefinition
	instance bool
}

// notOverridingGovernance is an element governed by decl whose
// ·instance-specified type definition· instance does not ·override· its
// ·selected type definition· selected under decl's {disallowed
// substitutions}: selected governs it (key-governing-type-elem clause 4), and
// the walk charges cvc-elt clause 4 ([walk.instanceOverride]).
type notOverridingGovernance struct {
	decl               xsd.ElementDeclaration
	selected, instance xsd.TypeDefinition
}

// unresolvedGovernance is an element with no ·governing element declaration·,
// its ·expanded name· ·resolving· to none, and no ·instance-specified type
// definition·: ·laxly assessed· (key-lva) below the root, and charged
// cvc-assess-elt at it ([Validator.Assess]).
type unresolvedGovernance struct{}

// absentAffiliationGovernance is an element whose ·governing element
// declaration· has an ·absent· member of its {substitution group
// affiliations}, the first one being head: §5.3 makes it ·laxly assessed· as
// if cvc-elt clause 1 had failed ([walk.absentAffiliation]).
type absentAffiliationGovernance struct{ head xsd.QName }

// absentHeadGovernance is an element whose ·selected type definition· is
// ·absent·, the slot it is read off naming a substitution-group head with no
// declaration: §5.3 makes it ·laxly assessed· as if cvc-elt clause 1 had
// failed ([walk.absentSelection]). The walk names the slot from its own
// selection ([walk.conditionallySelected]).
type absentHeadGovernance struct{}

// unresolvedSlotGovernance is an element governed by decl whose ·selected
// type definition· slot is nil, which resolves to no type: no ·governing type
// definition· is determined ([walk.resolvedSelection]'s decline).
type unresolvedSlotGovernance struct{ decl xsd.ElementDeclaration }

// withheldGovernance is an element governed by decl, which carries a {type
// table} whose selection the selection callback declined: no ·governing type
// definition· is determined. The walk's callback records the decline itself
// ([walk.conditionallySelected]).
type withheldGovernance struct{ decl xsd.ElementDeclaration }

// undecidedOverrideGovernance is an element whose ·instance-specified type
// definition· instance may or may not ·override· against, its ·selected type
// definition· under decl where hasDecl, and its ·locally declared type·
// otherwise: [xsd.Schema.ValidlySubstitutable] answered err instead
// ([walk.instanceOverride], [walk.localGovernance]).
type undecidedOverrideGovernance struct {
	decl              xsd.ElementDeclaration
	hasDecl           bool
	instance, against xsd.TypeDefinition
	err               error
}

func (skippedGovernance) governed()           {}
func (decidedGovernance) governed()           {}
func (notOverridingGovernance) governed()     {}
func (unresolvedGovernance) governed()        {}
func (absentAffiliationGovernance) governed() {}
func (absentHeadGovernance) governed()        {}
func (unresolvedSlotGovernance) governed()    {}
func (withheldGovernance) governed()          {}
func (undecidedOverrideGovernance) governed() {}

// decideGovernance is the governance of an element E named name, whose
// ·instance-specified type definition· (key-itd) is instance, nil where it has
// none, and which parent's content model ·attributes· to a (§3.4.4.4,
// key-att-to), nil where it attributes E to nothing; parent is the complex
// ·governing type definition· of E's parent, and nil where it has none and at
// the ·validation root·. selected answers the {type definition} slot a {type
// table} ·conditionally selects· for E (key-cta-select), or false where it
// withholds it; it is called at most once, and only for a declaration that
// carries a table.
//
// The ·governing element declaration· is key-governing-ed's, one case per
// [xsd.Attribution] variant (STYLE T2's closed-sum exception); this package
// stipulates none (clause 1):
//
//   - clause 2, the ·context-determined declaration·: an ElementDeclaration
//     attribution whose ·expanded name· is E's own is the {term} of the element
//     particle E was ·attributed to·.
//   - clause 2 through the ·substituting declaration·: an ElementDeclaration
//     attribution with ANOTHER name is the particle's own D, which cvc-accept
//     clause 2.3.2 admitted E under as a member of D's ·substitution group·.
//     The ·context-determined declaration· is then that member and never D
//     ([xsd.Attribution]), and a member is top-level by construction
//     (§3.3.6.4, cos-equiv-class), so it is the resolution below.
//   - clause 3, for a strict or a lax Wildcard: the declaration name ·resolves·
//     to among the schema's top-level element declarations
//     (cvc-resolve-instance, §3.17.6.3). A skip Wildcard ·skips· E instead
//     (cvc-assess-elt clause 2).
//   - clause 4, "otherwise", for an [*xsd.OpenContent] whose {wildcard} is
//     strict or lax and for a nil attribution: no declaration where E's
//     ·locally declared type· within parent
//     ([xsd.Schema.LocallyDeclaredElementType], key-ldt-elem §3.4.4.1) is
//     non-·absent· (clause 4.3), that type, or an xsi:type ·overriding· it,
//     governing instead (governedLocally); otherwise the same resolution as
//     clause 3. A skip {wildcard} ·skips· E (clause 4.1).
//
// A resolved name leads to governedByDeclaration, and one that resolves to
// none leaves E with no declaration: instance governs it where it has one
// (key-governing-type-elem clause 8), and it is unresolvedGovernance, ·laxly
// assessed· (key-lva), where it has none.
func decideGovernance(s *xsd.Schema, parent *xsd.ComplexType, a xsd.Attribution, name xsd.QName, instance xsd.TypeDefinition, selected func(xsd.TypeTable) (xsd.TypeDefinitionOrRef, bool)) governed {
	switch t := a.(type) {
	case xsd.ElementDeclaration:
		if t.Name() == name {
			return governedByDeclaration(s, t, instance, selected)
		}
		return governedByName(s, name, instance, selected)
	case xsd.Wildcard:
		if t.ProcessContents() == xsd.ProcessSkip {
			return skippedGovernance{}
		}
		return governedByName(s, name, instance, selected)
	case *xsd.OpenContent:
		if t.Wildcard().ProcessContents() == xsd.ProcessSkip {
			return skippedGovernance{}
		}
	}
	if parent != nil {
		if ldt, local := s.LocallyDeclaredElementType(*parent, name); local {
			return governedLocally(s, ldt, instance)
		}
	}
	return governedByName(s, name, instance, selected)
}

// governedByName is the governance of an element E named name whose
// ·governing element declaration· is the one name ·resolves· to, where one
// does (governedByDeclaration); where none does, instance governs E with no
// declaration (key-governing-type-elem clause 8), and with no instance E is
// unresolvedGovernance.
func governedByName(s *xsd.Schema, name xsd.QName, instance xsd.TypeDefinition, selected func(xsd.TypeTable) (xsd.TypeDefinitionOrRef, bool)) governed {
	d, found := s.Element(name)
	if found {
		return governedByDeclaration(s, d, instance, selected)
	}
	if instance == nil {
		return unresolvedGovernance{}
	}
	return decidedGovernance{typ: instance, instance: true}
}

// governedByDeclaration is the governance of an element E whose ·governing
// element declaration· is d and whose ·instance-specified type definition· is
// instance, nil where it has none. The order is encoded here once: an ·absent·
// {substitution group affiliations} member (§5.3), then the ·selected type
// definition· S (key-selected-type: clause 1, the slot d's {type table}
// ·conditionally selects·, through selected; clause 2, d's {type definition}),
// then the slot's resolution, then the ·override·.
//
// The ·overriding· test is key-overrides clause 2 against S under d's
// {disallowed substitutions} and nothing else: the target type's own
// {prohibited substitutions} joins inside key-val-sub-type, for the
// complex/complex case alone, which is [xsd.Schema.ValidlySubstitutable]'s to
// apply. instance governs where it ·overrides· S (key-governing-type-elem
// clause 3), and S otherwise (clause 4), the not-overriding case being
// cvc-elt clause 4's violation. The ·selected type definition· is settled
// before the ·override· because ·overriding· is a relation TO it, so an
// undetermined or ·absent· one decides nothing about an xsi:type.
//
// The exits that leave d without a ·governing type definition· are
// withheldGovernance, the callback declining a {type table}'s selection;
// unresolvedSlotGovernance, a nil slot — the declaration's own (clause 2),
// the one of the alternative that ·successfully selects·, or the {default
// type definition}'s; and undecidedOverrideGovernance, an error deciding the
// ·override·. The exits to ·lax assessment· are absentAffiliationGovernance
// and absentHeadGovernance (§5.3, Missing Sub-components).
func governedByDeclaration(s *xsd.Schema, d xsd.ElementDeclaration, instance xsd.TypeDefinition, selected func(xsd.TypeTable) (xsd.TypeDefinitionOrRef, bool)) governed {
	if head, absent := missingAffiliation(s, d); absent {
		return absentAffiliationGovernance{head: head}
	}
	ref := d.TypeDefinition()
	if table, tabled := d.TypeTable(); tabled {
		slot, decided := selected(table)
		if !decided {
			return withheldGovernance{decl: d}
		}
		ref = slot
	}
	sel, resolved := s.ResolvedType(ref)
	if !resolved && missingHead(s, ref) {
		return absentHeadGovernance{}
	}
	if !resolved {
		return unresolvedSlotGovernance{decl: d}
	}
	if instance == nil {
		return decidedGovernance{decl: d, hasDecl: true, typ: sel}
	}
	overrides, err := s.ValidlySubstitutable(instance, sel, d.DisallowedSubstitutions())
	if err != nil {
		return undecidedOverrideGovernance{decl: d, hasDecl: true, instance: instance, against: sel, err: err}
	}
	if overrides {
		return decidedGovernance{decl: d, hasDecl: true, typ: instance, instance: true}
	}
	return notOverridingGovernance{decl: d, selected: sel, instance: instance}
}

// governedLocally is the governance of an element with no ·governing element
// declaration· because a non-·absent· ·locally declared type· ldt exists for it
// (key-governing-ed clause 4.3): key-governing-type-elem clause 6, an
// ·instance-specified type definition· that ·overrides· ldt, and otherwise
// clause 7, ldt itself. With no ·governing element declaration· known,
// ·overriding· is ·validly substitutable without limitation· (key-overrides
// clause 2), the EMPTY set of blocking keywords (key-val-sub-type-absolute),
// and no cvc-elt clause is live to charge an xsi:type that does not override.
func governedLocally(s *xsd.Schema, ldt, instance xsd.TypeDefinition) governed {
	if instance == nil {
		return decidedGovernance{typ: ldt}
	}
	overrides, err := s.ValidlySubstitutable(instance, ldt, nil)
	if err != nil {
		return undecidedOverrideGovernance{instance: instance, against: ldt, err: err}
	}
	if overrides {
		return decidedGovernance{typ: instance, instance: true}
	}
	return decidedGovernance{typ: ldt}
}

// missingAffiliation reports the first member of d's {substitution group
// affiliations}, in document order, that names no element declaration in the
// schema. Finalize keeps such a name as an ·absent· member (§5.3 Missing
// Sub-components, xsd's resolveElementDecl).
func missingAffiliation(s *xsd.Schema, d xsd.ElementDeclaration) (xsd.QName, bool) {
	for _, head := range d.SubstitutionGroupAffiliationNames() {
		if _, found := s.Element(head); !found {
			return head, true
		}
	}
	return xsd.QName{}, false
}

// missingHead reports whether ref is a [xsd.SubstitutionGroupHeadTypeRef]
// whose head names no element declaration in the schema: §5.3's ·absent·
// head, as against a head that exists and whose own slot is nil. Finalize
// accepts it (xsd's resolveTypeDefinitionSlot); the parser never builds it,
// giving the member of an unresolvable substitutionGroup head xs:anyType.
func missingHead(s *xsd.Schema, ref xsd.TypeDefinitionOrRef) bool {
	head, isHead := ref.(xsd.SubstitutionGroupHeadTypeRef)
	if !isHead {
		return false
	}
	_, found := s.Element(head.Head)
	return !found
}

// Governing is the governance [Validator.Assess] decides for an element E
// named name (§3.3.4.6): its ·governing element declaration· decl, where
// declared, and its ·governing type definition· typ, where typ is not nil, or
// assessed false where E is ·skipped·. It is exported for a source adapter
// that must know an element's governance before the engine sees the element,
// to settle which members of its source are attributes and children by it;
// validate/jsonsrc is the in-module instance.
//
// parent is the complex ·governing type definition· of E's parent, nil where
// it has none and at the ·validation root·; narrow a simple or undetermined
// one to nil. a is what parent's content model ·attributes· E to (§3.4.4.4,
// key-att-to), as an [xsd.Matcher] over it answers, and nil where it
// attributes E to nothing and at the root. instance is E's
// ·instance-specified type definition· (key-itd), the top-level type its
// xsi:type ·resolves· to, and nil where it has none. selected answers the
// {type definition} slot E's ·governing element declaration·'s {type table}
// ·conditionally selects· (key-cta-select), or false where it withholds the
// selection; [Validator.ConditionallySelected], partly applied to E's
// [attributes] and [inherited attributes], is the selection Assess makes.
// selected must not be nil: Governing panics on nil, as Assess does on a nil
// root. Neither Governing nor Assess stipulates a declaration or a type
// (key-governing-ed clause 1, key-governing-type-elem clauses 1 and 2).
//
// The answer, row by row:
//
//   - Skipped. a is a skip Wildcard, or an [*xsd.OpenContent] whose {wildcard}
//     is skip: assessed false and every other result zero (cvc-assess-elt
//     clause 2). selected is not called. assessed is false nowhere else where
//     err is nil. The {open content} half is a reading and not a quotation
//     (#1969, reversing #1576): key-skipped, clause 3.2 of ·strictly assessed·
//     and cvc-wildcard name a skip Wildcard and never an Open Content, and
//     key-att-to ·attributes· the item to the {open content} record. The
//     ·default binding· of cos-content-act-restrict (§3.4.6.4,
//     key-dft-binding) clause 6 binds an item ·attributed· to "an Open Content
//     with a skip Wildcard" to the keyword skip, beside a skip ·wildcard
//     particle·'s, where a declaration key-governing-ed clause 4 resolved
//     would have bound it to that declaration by clause 1 instead.
//     cos-element-consistent clause 2.2 likewise counts only an {open content}
//     "with a strict or lax Wildcard", which is evidence of the exclusion and
//     not a rule of assessment.
//   - A ·governing element declaration· D: an ElementDeclaration a of E's
//     name (key-governing-ed clause 2); the top-level declaration name
//     ·resolves· to for an ElementDeclaration a of another name (the
//     ·substituting declaration·), for a strict or lax Wildcard (clause 3),
//     and for nil or a non-skip OpenContent a where no non-·absent· ·locally
//     declared type· of E exists within parent (clause 4, including 4.3).
//     Where D has an ·absent· {substitution group affiliations} member,
//     declared is false and typ nil: §5.3's ·lax assessment·, which Assess
//     charges under cvc-elt clause 1, and selected is not called. Otherwise S,
//     the ·selected type definition· (key-selected-type), is the slot
//     selected(table) answers where D has a {type table}, called exactly once,
//     and D's {type definition} where it has none. Where selected answers
//     false, or the slot is nil, decl is D, declared true and typ nil: Assess
//     determines no ·governing type definition· and records an [Unevaluated].
//     Where the slot names a substitution-group head with no declaration,
//     declared is false and typ nil, §5.3's ·lax assessment· again. Otherwise
//     typ is instance where instance is not nil and ·overrides· S under D's
//     {disallowed substitutions} (key-governing-type-elem clause 3), and S in
//     every other case (clause 4); where instance is not nil and typ is S,
//     Assess charges cvc-elt clause 4. An instance type is top-level, so
//     named, and a type always ·overrides· itself, so typ is the type instance
//     names exactly where instance ·overrides· S.
//   - No declaration, a non-·absent· ·locally declared type· L of E within
//     parent (key-ldt-elem), for a nil or non-skip OpenContent a
//     (key-governing-ed clause 4.3): declared false, and typ instance where it
//     ·overrides· L ·without limitation· (key-governing-type-elem clause 6),
//     and L otherwise (clause 7). Nothing is charged.
//   - No declaration, name ·resolving· to none: declared false, typ instance
//     where it is not nil (clause 8), and otherwise nil, E being ·laxly
//     assessed· (key-lva). At the ·validation root·, parent and a both nil,
//     Assess charges cvc-assess-elt instead where declared is false and typ
//     nil, but for an ·absent· affiliation or head above.
//
// err is non-nil only where [xsd.Schema.ValidlySubstitutable] cannot settle
// the ·override·, an exit unreachable for a finalized Schema (RULED permanent
// by #2174); every other result is then zero, and Assess records an
// [Unevaluated].
func (v *Validator) Governing(parent *xsd.ComplexType, a xsd.Attribution, name xsd.QName, instance xsd.TypeDefinition, selected func(xsd.TypeTable) (xsd.TypeDefinitionOrRef, bool)) (decl xsd.ElementDeclaration, declared bool, typ xsd.TypeDefinition, assessed bool, err error) {
	if selected == nil {
		panic("validate: Governing: nil selected")
	}
	switch g := decideGovernance(v.schema, parent, a, name, instance, selected).(type) {
	case skippedGovernance:
		return xsd.ElementDeclaration{}, false, nil, false, nil
	case decidedGovernance:
		return g.decl, g.hasDecl, g.typ, true, nil
	case notOverridingGovernance:
		return g.decl, true, g.selected, true, nil
	case unresolvedGovernance, absentAffiliationGovernance, absentHeadGovernance:
		return xsd.ElementDeclaration{}, false, nil, true, nil
	case unresolvedSlotGovernance:
		return g.decl, true, nil, true, nil
	case withheldGovernance:
		return g.decl, true, nil, true, nil
	case undecidedOverrideGovernance:
		return xsd.ElementDeclaration{}, false, nil, false, g.err
	}
	panic("validate: Governing: unknown governed arm")
}
