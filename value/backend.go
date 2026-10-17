package value

import "github.com/kud360/goxsd8/xsd"

// Context supplies the dynamic information a lexical mapping needs beyond the
// literal itself. QName and NOTATION mapping is context-dependent: it resolves
// a prefix against the in-scope namespace bindings at the point of the literal
// (Datatypes §3.3.18/§3.3.19, xmlschema11-2.md — prose-only, with no Appendix E
// hfn), which is why [Mapping.Parse] receives one (PRINCIPLES 19).
//
// A nil Context binds nothing, the empty prefix included: by repository
// policy, a host with no XML namespace context of its own supplies the
// bindings it means, so every QName and NOTATION lexical is rejected under
// cvc-datatype-valid clause 2.1. It is safe for every other type.
type Context interface {
	// LookupNamespace resolves prefix against the namespace bindings in scope.
	// The empty prefix yields the default namespace with ok true: namespace is
	// "" where none is in scope or `xmlns=""` undeclares it. Any other prefix
	// yields its namespace name with ok true where it is bound, and ("",
	// false) where it is unbound; "xml" is always bound. Whether "xmlns" is
	// bound is the implementer's.
	//
	// The QName and NOTATION mappings reject a lexical whose prefix answers ok
	// false (cvc-datatype-valid clause 2.1), so a Context that answers ("",
	// false) for the empty prefix rejects every unprefixed one.
	LookupNamespace(prefix string) (namespace string, ok bool)
}

// Mapping is a type's lexical↔value mapping.
//
// Parse turns a normalized lexical form into a [Value], using ctx for the
// context-dependent types. A lexical that the wider-space facet checks accept
// but a narrow derived representation cannot hold is a mapping error on the
// derived type — Parse returns it as an *xsderr.Error (the concrete rule ID is
// chosen by the concrete backend that produces the value; no such backend
// exists yet, so the rule is TBD until one lands), never a false validity
// verdict.
//
// Canonical renders a value back to its canonical lexical form. It MAY be nil:
// some types have no canonical mapping at all (QName's is context-dependent and
// the spec defines none per dt-canonical-representation). Callers must treat a
// nil Canonical as "this whole type has no canonical form", not as an error.
//
// Distinct from that whole-type case: even when Canonical is non-nil, it may
// return a plain (non-*xsderr.Error) error for one specific VALID value that has
// no canonical lexical form of its own. A canonical mapping's domain is only
// "(where possible)" the entire value space (dt-canonical-mapping, §2.3.1), so a
// type that inherits a base's canonical mapping under a narrowing pattern facet
// can hold a value whose would-be canonical form falls outside its own lexical
// space (e.g. yearMonthDuration's ·months·=0∧·seconds·=0, whose duration
// canonical "PT0S" is outside the [^DT]* lexical space, §3.4.26.1 Note; any type
// restricting a base's canonical mapping by a pattern facet can hit the same
// trap). Unlike the Parse-side mapping error above, this is NEVER a validity
// verdict — no cvc-* rule reads canonical form (§2.3.1 Note) — so it is a plain
// fmt.Errorf, not an *xsderr.Error. This per-value "valid, no canonical form"
// case stays textually separate from the whole-type "no Canonical func" case.
//
// The same plain error also reports a VALID value whose canonical form exists
// but lies beyond the backend's capacity (Datatypes §5.4: indicated, never
// quietly changed, never treated as invalid) — e.g. the strict backend's
// precisionDecimal zero of a huge ·scale·, whose canonical form pads ·scale· − 1
// zeros.
type Mapping struct {
	// Parse maps a normalized lexical form to a value, or returns an
	// *xsderr.Error describing why the lexical is not in the type's space.
	// Parse MUST be non-nil: a Mapping is meaningless without it.
	Parse func(lexical string, ctx Context) (Value, error)
	// Canonical maps a value back to its canonical lexical form. It is nil for
	// types that have no canonical mapping (whole-type case); when non-nil it may
	// still return a plain non-*xsderr.Error for one valid value that has no
	// canonical form of its own (partial-domain case, §2.3.1's "(where possible)")
	// or whose canonical form is beyond the backend's capacity (§5.4). Such an
	// error is not a validity verdict — see the [Mapping] doc.
	Canonical func(v Value) (string, error)
}

// Backend supplies the lexical↔value mappings for builtin types. A backend
// MUST cover the primitives (directly or via composition); it MAY also map
// derived builtins to give them their own, typically narrower, representation.
//
// A derived type's own mapping governs only the VALUE the application receives,
// never the space in which inherited facet checks run: enumeration and bound
// facets are always compared in the value space of the type that DECLARES the
// facet, parsed by that type's governing mapping (its own, or its nearest
// mapped ancestor's — ultimately the primitive's, which is the widest). Using a
// narrow derived representation for base-chain semantics would corrupt them
// (overflow, collapsed precision, different ordering). Comparison and facet
// capabilities are not backend methods; they live on the values a Mapping
// produces (see [Ordered], [DigitCounted], …).
type Backend interface {
	// Mapping returns the mapping for typ; ok is false when this backend does
	// not map typ. That is the expected answer for a type the backend does not
	// map directly, not a fault: package builtin/strict maps only the types its
	// "Current coverage" section names, so it answers false for xs:integer. The
	// zero Mapping returned beside ok == false has a nil Parse.
	//
	// [ValidateLexical] reaches the value of such a type that is not ·special·
	// (§4.1): an atomic type's through the governing mapping the [Backend] doc
	// names, a list's through its {item type definition}, and a union's through
	// its {member type definitions}.
	Mapping(typ xsd.QName) (Mapping, bool)
}

// Override composes two backends per type: it returns a [Backend] that yields
// partial's mapping for every type partial defines, and base's mapping
// otherwise. This is how a program backs only xs:decimal with a money type and
// keeps the rest of base unchanged.
//
// The widest-space rule (see [Backend]) still governs: an override changes only
// the value an application receives for the overridden type, not the space in
// which any type's inherited facet checks run.
//
// Both base and partial MUST be non-nil.
func Override(base, partial Backend) Backend {
	return overrideBackend{base: base, partial: partial}
}

// overrideBackend layers partial over base for [Override].
type overrideBackend struct {
	base    Backend
	partial Backend
}

func (o overrideBackend) Mapping(typ xsd.QName) (Mapping, bool) {
	if m, ok := o.partial.Mapping(typ); ok {
		return m, true
	}
	return o.base.Mapping(typ)
}

// LexicalFacet is a facet checked on the normalized lexical form, before the
// lexical mapping runs (the whiteSpace and pattern stages of the pipeline;
// ARCHITECTURE.md). A failure is an *xsderr.Error carrying the facet's rule ID.
type LexicalFacet interface {
	// CheckLexical validates the normalized lexical form, returning an
	// *xsderr.Error on rejection.
	CheckLexical(normalized string) error
}

// ValueFacet is a facet checked on the parsed value, after the lexical mapping
// runs (the value-facet stage of the pipeline; ARCHITECTURE.md). A failure is
// an *xsderr.Error carrying the facet's rule ID. Its name pairs with
// [LexicalFacet] to name the two pipeline stages; the "Facet" suffix is
// load-bearing, so the pairing is kept despite the value.ValueFacet stutter.
//
//nolint:revive // ValueFacet/LexicalFacet name the pipeline stages; suffix is load-bearing.
type ValueFacet interface {
	// CheckValue validates the parsed value, returning an *xsderr.Error on
	// rejection.
	CheckValue(v Value) error
}
