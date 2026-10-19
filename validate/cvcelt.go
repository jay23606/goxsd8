package validate

import (
	"strings"

	"github.com/kud360/goxsd8/internal/xmlqname"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// This file holds the two instance attributes that change what the rest of an
// assessment reads — xsi:type, which can displace the ·governing type
// definition· (§2.7.1), and xsi:nil, which can make an element ·nilled·
// (§2.7.2) — together with the cvc-elt (§3.3.4.3) clauses that charge them.
//
// Neither is ever ·attributed to· an attribute use or a wildcard:
// cvc-complex-type clause 2 excepts both by ·expanded name·, along with
// xsi:schemaLocation and xsi:noNamespaceSchemaLocation, and isInstanceAttribute
// (assess.go) is the one reading of that exception.
//
// Clause 3's three failures are charged here (nilCheck); clause 4's is charged
// where the governing type is settled ([walk.instanceOverride], assess.go), because
// the same resolution decides the charge and the type. Clause 5's two arms live
// with the [[children]] that decide which of them applies (cvccomplexcontent.go).
// Clause 1 is charged for the two ·absent· components §5.3 routes to it, and
// the element is then ·laxly assessed· ([governedByDeclaration]): an ·absent·
// {substitution group affiliations} member, before the selected type is read
// ([walk.absentAffiliation], assess.go), and an ·absent· selected type, where
// it is read ([walk.absentSelection], cta.go).

// nilled reports whether e is ·nilled· with respect to its ·governing element
// declaration· (§3.3.4.3, key-nilled): E has xsi:nil = true AND D.{nillable} =
// true. An element with no ·governing element declaration· is ·nilled· with
// respect to nothing, so it is never nilled here.
//
// The ·actual value· is read directly rather than through the injected
// value.Backend, on the same grounds cvc-resolve-instance's QName is
// (instanceTypeDefinition): xsi:nil is one of the Built-in Attribute
// Declarations §3.2.7 fixes as xs:boolean-typed and §3.2.6.4 no-xsi
// forbids a schema to redeclare, so its lexical space is the four literals of
// boolean-lexical-mapping (Datatypes §3.3.2.2) after the whiteSpace = collapse
// its type fixes, and no backend may widen or narrow it. A lexical outside those
// four yields false here and is charged by nilCheck under clause 3.2 instead.
func nilled(e Element, g governance) bool {
	if !g.hasDecl || !g.decl.Nillable() {
		return false
	}
	v, present := instanceNil(e)
	return present && v == instanceBooleanTrue
}

// instanceBoolean is the ·actual value· of an xs:boolean-typed instance
// attribute as this package reads it, with the third state a validity rule
// needs: a lexical outside the type's lexical space has no ·actual value· at
// all (§3.3.5.4 clause 1.3) and is neither true nor false.
type instanceBoolean uint8

const (
	// instanceBooleanUnreadable is a lexical outside the four literals of
	// boolean-lexical-mapping.
	instanceBooleanUnreadable instanceBoolean = iota
	// instanceBooleanFalse is the "false" and "0" literals.
	instanceBooleanFalse
	// instanceBooleanTrue is the "true" and "1" literals.
	instanceBooleanTrue
)

// instanceNil reports the ·actual value· of e's xsi:nil attribute, and whether
// e carries one at all.
func instanceNil(e Element) (instanceBoolean, bool) {
	a, present := instanceAttribute(e, "nil")
	if !present {
		return instanceBooleanUnreadable, false
	}
	return instanceBooleanOf(a.Value()), true
}

// instanceBooleanOf maps lexical, an xs:boolean-typed instance attribute's
// ·initial value·, to its ·actual value·: boolean-lexical-mapping's four
// literals (Datatypes §3.3.2.2) after the whiteSpace = collapse xs:boolean
// fixes, and instanceBooleanUnreadable for every other lexical.
func instanceBooleanOf(lexical string) instanceBoolean {
	switch collapseXMLWhitespace(lexical) {
	case "true", "1":
		return instanceBooleanTrue
	case "false", "0":
		return instanceBooleanFalse
	}
	return instanceBooleanUnreadable
}

// nilCheck settles cvc-elt clause 3 for one element and reports whether e is
// ·nilled·, which clauses 1 of cvc-complex-type and 5 of cvc-elt both read.
//
// Clause 3 is a disjunction over D.{nillable}, and each arm has exactly one way
// to fail:
//
//   - 3.1, D.{nillable} = false: E must have no xsi:nil attribute at all. The
//     ·actual value· is not read — clause 3.1 quantifies over PRESENCE, so
//     xsi:nil="false" on a non-nillable declaration violates it exactly as
//     xsi:nil="true" does.
//   - 3.2, D.{nillable} = true: one of no attribute (3.2.1), the value false
//     (3.2.2) or the value true (3.2.3) must hold. A lexical outside xs:boolean's
//     lexical space satisfies none of the three, and clause 3.2 is charged with
//     no sub-clause, there being no sub-clause that a readable value would have
//     failed.
//   - 3.2.3.1 and 3.2.3.2, under a ·nilled· element: no character or element
//     information item [[children]], and no {value constraint} with {variety} =
//     fixed. The second is decidable here; the first is not, since the
//     [[children]] have not been read, and is charged as they arrive
//     (cvccomplexcontent.go).
//
// An element with no ·governing element declaration· reaches no arm: cvc-elt is
// a rule ABOUT a declaration, and one that is ·absent· charges nothing. An
// xsi:nil with no ·actual value· on such an element is charged against the
// attribute instead, under cvc-attribute clause 3 ([walk.instanceNilLexical],
// cvcattribute.go).
//
// Whether E is ·nilled· is [nilled]'s to decide and is not re-derived from the
// arms above (STYLE T4): the arms charge, and the one fact they all report is
// read from key-nilled's single encoding.
func (w *walk) nilCheck(e Element, g governance) bool {
	if !g.hasDecl {
		return false
	}
	v, present := instanceNil(e)
	if !g.decl.Nillable() {
		if present {
			w.res.violations = append(w.res.violations, xsderr.New(ruleCvcElt, e.Loc(),
				"the element %s carries an xsi:nil attribute, but the {nillable} of its ·governing element declaration· is false, and cvc-elt clause 3.1 admits no xsi:nil attribute on such an element whatever its ·actual value·",
				e.Name()))
		}
		return false
	}
	if present && v == instanceBooleanUnreadable {
		w.res.violations = append(w.res.violations, xsderr.New(ruleCvcElt, e.Loc(),
			"the xsi:nil attribute of the element %s has no ·actual value·: its lexical is outside the lexical space of the xs:boolean the Built-in Attribute Declarations (§3.2.7) type it by, so none of the three arms of cvc-elt clause 3.2 — no attribute (3.2.1), the value false (3.2.2), the value true (3.2.3) — holds, and the clause has no fourth",
			e.Name()))
		return false
	}
	isNilled := nilled(e, g)
	if f, fixed := elementFixed(g); fixed && isNilled {
		w.res.violations = append(w.res.violations, xsderr.New(ruleCvcElt, e.Loc(),
			"the element %s has xsi:nil = true, so it is ·nilled·, but its ·governing element declaration· carries the fixed {value constraint} %q, and cvc-elt clause 3.2.3.2 admits no {value constraint} with {variety} = fixed on a ·nilled· element",
			e.Name(), f.LexicalForm()))
	}
	return isNilled
}

// elementDefault applies cvc-elt clause 5's case split and reports the
// {value constraint} clause 5.1 substitutes: D has a {value constraint}, E has
// neither element nor character information item [[children]], and E is not
// ·nilled· with respect to D. Where it holds, what clause 5.1.2 assesses is
// "the element information item with D.{value constraint}.{lexical form} used as
// its ·normalized value·" and never the empty [[children]] E actually has; where
// it fails, clause 5.2 assesses E itself and this reports false.
//
// Clause 5.1.2 stipulates a ·normalized value· and cvc-type clause 3.1.3 reads
// an ·initial value·, and this hands the one on as the other: the {lexical form}
// becomes the string String Valid (§3.16.4) normalizes under the ·governing type
// definition·'s whiteSpace facet. It is NOT pre-normalized against any type —
// [xsd.NewValueConstraint] stores it verbatim and parser's valueConstraintOf
// hands it the raw default=/fixed= attribute value — so the composition
// normalizes it exactly once, under the type doing the validating. That is the
// same composition [xsd.ValueSpace.ValidDefault] applies wherever a {value
// constraint}'s lexical is read — a-props-correct clause 2, au-props-correct
// clause 2 and e-props-correct clause 2 — and that cvc-complex-type clause 4
// ([walk.defaultedAttribute]) applies through [walk.stringValid]: all predate
// this rule and all compose it the same way, so clause 5.1.1 and clause 5.1.2
// cannot disagree about the lexical either.
//
// GAP(validate): where the {lexical form} is not already a fixed point of that
// facet, normalizing once is a READING of clause 5.1.2 and not a transcription
// of it — §3.3.5.4 clause 1.1 makes the {lexical form} the [schema normalized
// value] outright, with no second normalization, and key-nv (§3.3) defines a
// ·normalized value· only relative to the type validating it. #1119 owns this
// residual. The four readers of the value this hands on split on which way that
// cuts, so the direction is stated per reader and not once (STYLE P3a):
//
//   - [contentCheck.simpleTypeValue] (cvc-type 3.1.3) and
//     [contentCheck.initialValue] (cvc-complex-type 1.2) CHARGE on a lexical
//     outside the type's lexical space, and the direction for these two is
//     UNESTABLISHED. Collapsing a padded lexical into a type's lexical space
//     withholds a charge, but the pattern stage runs on the normalized string
//     (value/facets.go), so a {pattern} matching only strings whose white space
//     collapse deletes is charged HERE and satisfied under a literal reading —
//     the same normalization cuts both ways and no one direction covers the
//     reader set.
//   - [walk.idRecord] (§3.17.5.2) and [walk.keyMember] (§3.11.4 clause 3) read
//     an ·actual value· off it, and for these two normalizing is not a
//     permission at all but the only way to have a value: key-nv derives the
//     ·actual value· from the NORMALIZED lexical, so skipping it would map the
//     wrong string or none, shortening a ·key-sequence· and dropping an ·ID
//     value·.
//
// The emptiness is the caller's to establish, since only the walk that consumed
// E.[[children]] knows it ([contentCheck.empty]). Both varieties are returned:
// clause 5.1 quantifies over "a {value constraint}" with no {variety} condition,
// which is what makes it the arm an empty element with a FIXED constraint takes
// (elementFixed's is clause 5.2.2's narrower reading, on the other arm).
func elementDefault(g governance, empty, isNilled bool) (xsd.ValueConstraint, bool) {
	if !empty || isNilled || !g.hasDecl {
		return xsd.ValueConstraint{}, false
	}
	return g.decl.ValueConstraint()
}

// elementFixed reports D.{value constraint} where its {variety} is fixed — the
// shape cvc-elt clauses 3.2.3.2 and 5.2.2 both read — for an element whose
// ·governing element declaration· is known.
func elementFixed(g governance) (xsd.ValueConstraint, bool) {
	if !g.hasDecl {
		return xsd.ValueConstraint{}, false
	}
	vc, has := g.decl.ValueConstraint()
	if !has || vc.Kind() != xsd.ValueFixed {
		return xsd.ValueConstraint{}, false
	}
	return vc, true
}

// instanceTypeDefinition is the ·instance-specified type definition· of e
// (§3.3.4.1, key-itd): the type definition e's xsi:type attribute ·resolves· to.
// It is false where any of the definition's three conjuncts fails — no xsi:type
// attribute, a ·normalized value· that is not a QName, or a QName resolving to
// no top-level type definition — because the definition is one conjunction and a
// failure anywhere in it leaves E with no ·instance-specified type definition·
// AT ALL, which is what makes cvc-elt clause 4 vacuous rather than violated for
// an unresolvable xsi:type (the Note under cvc-elt, resolving W3C issue 11764).
//
// That vacuity is the ELEMENT's alone. The ATTRIBUTE is charged separately
// under cvc-attribute (§3.2.4.1), clause 3 for a lexical outside xs:QName's
// lexical space and clause 5 for a QName that names nothing, the second reading
// the same resolution through [walk.resolveInstanceType] without changing what
// this returns ([walk.instanceTypeResolves], cvcattribute.go).
//
// The resolution is cvc-resolve-instance (§3.17.6.3) and stays at the Structures
// level: the prefix is resolved against the in-scope namespace bindings at E
// ([Element.LookupPrefix], PRINCIPLES 19) and the result looked up among the
// schema's top-level {type definitions}. It does not go through the injected
// value.Backend, which maps the DATATYPE xs:QName: no backend decides which
// component a name denotes, and routing this through one would make the
// component model's own symbol table backend-dependent.
func (w *walk) instanceTypeDefinition(e Element) (xsd.TypeDefinition, bool) {
	a, present := instanceAttribute(e, "type")
	if !present {
		return nil, false
	}
	_, t, outcome := w.resolveInstanceType(e, a)
	return t, outcome == instanceTypeResolved
}

// instanceTypeOutcome is how far an xsi:type attribute got in ·resolving· to a
// type definition, which the two rules reading it divide differently: key-itd
// conjunct 3 folds both failures into "E has no ·instance-specified type
// definition·" ([walk.instanceTypeDefinition]), while cvc-attribute clause 5
// charges the second alone ([walk.instanceTypeResolves], cvcattribute.go).
//
// The outcome does not decide whether the lexical is a QName, which is
// cvc-attribute clause 3's question ([walk.instanceTypeLexical]): a lexical
// whose parts are no NCName reaches instanceTypeUnresolved, and clause 5 reads
// the outcome only for a lexical clause 3 did not reject.
type instanceTypeOutcome uint8

const (
	// instanceTypeNoValue is a lexical [resolveInstanceQName] turns away at the
	// SPLIT: empty, a colon structure no QName has, or a prefix with no binding
	// in scope at E. Every such lexical has no ·actual value·.
	instanceTypeNoValue instanceTypeOutcome = iota
	// instanceTypeUnresolved is a lexical that cleared the split and yields a
	// name no top-level type definition of the schema carries.
	instanceTypeUnresolved
	// instanceTypeResolved is a lexical that cleared the split and yields the
	// name of one.
	instanceTypeResolved
)

// resolveInstanceType ·resolves· the xsi:type attribute a of e against the
// schema's top-level {type definitions}, on cvc-resolve-instance's terms
// (§3.17.6.3), and reports which of its three outcomes that reached. It is the
// one encoding of that walk (STYLE T4); the two readers differ only in which
// outcomes they act on.
//
// The first result is the name the split and the prefix binding made of the
// lexical, which the clause 5 charge names in its message beside it so that a
// prefix bound to a namespace the author did not expect is visible. It is A's
// ·actual value· only where the lexical is a QName, which this does not decide
// (see instanceTypeOutcome). It is the zero QName on instanceTypeNoValue.
func (w *walk) resolveInstanceType(e Element, a Attribute) (xsd.QName, xsd.TypeDefinition, instanceTypeOutcome) {
	name, named := resolveInstanceQName(e, a.Value())
	if !named {
		return xsd.QName{}, nil, instanceTypeNoValue
	}
	t, resolved := w.schema.Type(name)
	if !resolved {
		return name, nil, instanceTypeUnresolved
	}
	return name, t, instanceTypeResolved
}

// resolveInstanceQName splits a QName lexical, A.[[normalized value]], and
// resolves its prefix against the bindings in scope at e, which is the whole
// of cvc-resolve-instance (§3.17.6.3) up to the component lookup itself. It is
// false for an empty lexical, a colon structure no QName has, and a prefix with
// no binding, each of which leaves the ·actual value· ·absent·.
//
// The split, the whiteSpace = collapse xs:QName fixes and the prefix lookup
// are [xmlqname.Resolve]'s, the one encoding validate/jsonsrc's "$type" member
// resolves through too (STYLE T4). It checks the two halves for EMPTINESS and
// not against the NCName production, so true does not make the lexical a
// QName: whether it is one is cvc-attribute clause 3's to decide, through the
// backend's xs:QName mapping ([walk.instanceTypeLexical], cvcattribute.go). A
// name that is not an NCName cannot be the {name} of any component, so the
// lookup in [walk.instanceTypeDefinition] turns it away exactly as it turns
// away a well-formed name nothing declares.
func resolveInstanceQName(e Element, lexical string) (xsd.QName, bool) {
	space, local, ok := xmlqname.Resolve(lexical, e.LookupPrefix)
	if !ok {
		return xsd.QName{}, false
	}
	return xsd.QName{Space: space, Local: local}, true
}

// instanceAttribute reports e's attribute information item in the XML Schema
// instance namespace with the given [[local name]], and whether e carries one.
// The four names it is asked for are the Built-in Attribute Declarations of
// §3.2.7, which §3.2.6.4 no-xsi forbids a schema to redeclare, so the
// ·expanded name· settles which item is meant with no resolution of its own.
func instanceAttribute(e Element, local string) (Attribute, bool) {
	name := xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: local}
	for _, a := range e.Attributes() {
		if a.Name() == name {
			return a, true
		}
	}
	return nil, false
}

// collapseXMLWhitespace is the whiteSpace = collapse normalization (Datatypes
// §4.3.6): every leading and trailing space, tab, carriage return and line feed
// removed, and every internal run of them replaced by a single space. It is
// applied to xsi:nil, whose type xs:boolean fixes that facet; xsi:type's
// xs:QName is collapsed inside [xmlqname.Resolve] (resolveInstanceQName).
//
// The white-space set is xmlWhitespace and deliberately not unicode.IsSpace: a
// vertical tab is not white space to a schema processor, and collapsing one
// would move a lexical into a lexical space that does not contain it.
func collapseXMLWhitespace(s string) string {
	return strings.Join(strings.FieldsFunc(s, isXMLSpace), " ")
}

// isXMLSpace reports whether r is one of xmlWhitespace's four characters.
func isXMLSpace(r rune) bool {
	return strings.ContainsRune(xmlWhitespace, r)
}
