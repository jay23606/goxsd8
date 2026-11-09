package goshape

import (
	"slices"

	"github.com/kud360/goxsd8/xsd"
)

// SubstitutionSum reports whether a field over the element declaration decl
// holds a sum, and returns its members: the declarations of decl's
// ·substitution group· (§3.3.6.4 key-eq, [xsd.Schema.SubstitutionGroup]) with
// {abstract} false (§3.3.4.3 cvc-elt clause 2), in group order. sum is true
// exactly when decl is top-level and members are not decl alone, so an
// abstract decl whose group holds no other declaration with {abstract} false
// answers a sum with no members, and a decl with {abstract} false whose other
// declarations are all abstract answers none. The group is the whole of
// [xsd.Schema.SubstitutionGroup]'s, whose doc states when decl is alone in it
// (cos-equiv-derived-ok-rec clauses 1, 2.1 and 2.3) and that a local
// declaration heads none (cvc-accept clause 2.3.2); codegen/doc.go
// "# Substitution groups" states the branches of the members it holds through
// an abstract declaration (cos-equiv-derived-ok-rec clause 2.2).
func SubstitutionSum(schema *xsd.Schema, decl xsd.ElementDeclaration) (members []xsd.ElementDeclaration, sum bool) {
	if decl.ScopeVariety() != xsd.ScopeGlobal {
		return nil, false
	}
	members = slices.DeleteFunc(schema.SubstitutionGroup(decl.Name()), xsd.ElementDeclaration.Abstract)
	if len(members) == 1 && members[0].Name() == decl.Name() {
		return nil, false
	}
	return members, true
}

// BasicMembers returns the basic members of the union simple type st
// (Datatypes §2.4.1.3 dt-basicmember) in codegen's basic-member order
// (codegen/doc.go "# Simple types"), reading each union's {member type
// definitions} (§3.16.1) through [xsd.SimpleType.Members], which takes a
// restriction's from its base (§3.16.2.1), and keeping each member at its
// first position by identity. No union reaches itself through its members
// (cos-st-restricts clause 3.3 no-self-membership, which finalize checks), so
// the walk carries no visited set.
func BasicMembers(r xsd.TypeResolver, st *xsd.SimpleType) ([]*xsd.SimpleType, error) {
	members, err := st.Members(r)
	if err != nil {
		return nil, err
	}
	var basic []*xsd.SimpleType
	for _, m := range members {
		variety, err := m.Variety(r)
		if err != nil {
			return nil, err
		}
		reached := []*xsd.SimpleType{m}
		// {variety} is a closed sum, so the assertion is STYLE T2's sealed-sum
		// exception.
		if _, isUnion := variety.(xsd.Union); isUnion {
			reached, err = BasicMembers(r, m)
			if err != nil {
				return nil, err
			}
		}
		for _, b := range reached {
			if !slices.Contains(basic, b) {
				basic = append(basic, b)
			}
		}
	}
	return basic, nil
}
