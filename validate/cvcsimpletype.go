package validate

import (
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xpath"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// ruleCvcSimpleType is String Valid (Structures §3.16.4, cvc-simple-type). This
// package charges it only as the verdict a delegating rule wraps — cvc-attribute
// clause 3, cvc-type clause 3.1.3 and cvc-complex-type clauses 1.2 and 4 — and
// only for clause 3: clauses 1 and 2 are Datatype Valid's, whose verdict
// carries cvc-datatype-valid or a facet rule under it.
const ruleCvcSimpleType xsderr.Rule = "cvc-simple-type"

// stringValid runs String Valid (§3.16.4) over lexical against st, for the
// item at loc, mapping lexical under ctx: clauses 1 and 2 through
// value.ValidateLexical, which against w.schema also rejects a NOTATION value
// naming no notation declared in the schema (Datatypes §3.3.19), then clause 3
// ([walk.entitiesDeclared]) over the ·actual value· value.ValidateLexical
// accepted. decided is false where this package withholds a verdict. err is
// the verdict where decided is true — nil for a ·valid· lexical, the rejection
// for an invalid one, which the caller charges under its own rule as the
// wrapped cause — and the withholding cause where decided is false: the
// value.ValidateLexical error that withheld it, which [walk.declineAssertions]
// reads to decline an assertions facet under its own rule, or nil where clause
// 3's ·validating type· is undecidable.
//
// A ·special· st ([xsd.SimpleType.IsSpecial]) passes clause 2 without asking
// ValidateLexical, which no backend answers for one: no clause 1 normalization
// can move a string out of either type's lexical space, and neither type is
// NOTATION or has it in its closure. A value this package reads against one is
// therefore decided, never declined, whatever the backend maps: both lexical
// spaces are every Char sequence and both {facets} are empty (#1788). A
// ValidateLexical error that is not a VERDICT ([value.IsDatatypeVerdict])
// withholds one; each caller states that decline's GAP on its own terms. The
// pipeline evaluates st's assertions facets, and those of every type it
// recurses into, through [xpath.FacetAssertions].
func (w *walk) stringValid(st *xsd.SimpleType, lexical string, ctx value.Context, loc xsderr.Loc) (decided bool, err error) {
	if st.IsSpecial() {
		return w.entitiesDeclared(st, lexical, ctx, loc)
	}
	_, err = value.ValidateLexical(w.backend, w.schema, st, lexical, ctx, xpath.FacetAssertions(w.now))
	if err != nil && !value.IsDatatypeVerdict(err) {
		return false, err
	}
	if err != nil {
		return true, err
	}
	return w.entitiesDeclared(st, lexical, ctx, loc)
}

// entitiesDeclared settles String Valid clause 3 — "Let V be the ·actual value·
// of N with respect to T. Then: Every ·ENTITY value· in V is a ·declared entity
// name·" — over a lexical clauses 1 and 2 have accepted against st. It is the
// ONE check site for both ways the clause fails, and both yield a
// cvc-simple-type verdict at loc:
//
//   - the source does not implement [UnparsedEntities] (walk.entities is nil).
//     Appendix D makes non-support of [unparsedEntities] itself the failure:
//     "all items of type ENTITY or ENTITIES will fail to ·validate·".
//   - the value names no unparsed entity in [unparsedEntities], so it is not a
//     ·declared entity name· (key-vde). Where the source reports through
//     [DeclarationsProcessed] that its DTD was not fully read (walk.declsUnread),
//     the message says so, since the name may be declared where the source did
//     not read; the verdict is the same.
//
// Which values are ·ENTITY values· is key-TYPE-value's, read through the same
// ·validating type· machinery the [ID/IDREF table] uses (roleValues, cvcid.go):
// an item whose ·governing type definition· has no ENTITY or ENTITIES in its
// closure is admitted by no candidacy and costs no member scan, an ENTITIES
// list is checked per item (key-vtype clause 2), and a union of ENTITY and
// string checks only the values its ENTITY member validated. The first value
// that fails is the verdict.
//
// GAP(validate): a ·validating type· this package cannot decide — a candidacy
// or member scan that errors, on validatingType's terms — withholds the verdict
// rather than charging, the same decline idRecord states for the [ID/IDREF
// table]. RULED permanent by #774 (STYLE P3b), on the terms of
// [walk.declaredAttribute]'s ungoverned-type decline: a candidacy or member scan
// that errors is backend or type-scan coverage, not this package's.
func (w *walk) entitiesDeclared(st *xsd.SimpleType, lexical string, ctx value.Context, loc xsderr.Loc) (decided bool, verdict error) {
	candidate, decided := w.candidate(st, valueRole.isEntity)
	if !decided {
		return false, nil
	}
	if !candidate {
		return true, nil
	}
	values, decided := w.roleValues(st, lexical, ctx)
	if !decided {
		return false, nil
	}
	for _, v := range values {
		if !v.role.isEntity() {
			continue
		}
		if w.entities == nil {
			return true, xsderr.New(ruleCvcSimpleType, loc,
				"value %q is not a ·declared entity name·: the source presents no [unparsedEntities] property of the document information item, and Appendix D makes every ·ENTITY value· of such a source fail cvc-simple-type clause 3",
				v.value)
		}
		if w.entities.HasUnparsedEntity(v.value) {
			continue
		}
		if w.declsUnread {
			return true, xsderr.New(ruleCvcSimpleType, loc,
				"value %q names no unparsed entity in the document's [unparsedEntities]: the document's DTD was not fully read — an external DTD subset or a parameter entity went unread — and the name may be declared there, but a declaration that was not read declares nothing, so it is not a ·declared entity name·, which cvc-simple-type clause 3 requires of an ·ENTITY value·",
				v.value)
		}
		return true, xsderr.New(ruleCvcSimpleType, loc,
			"value %q names no unparsed entity in the document's [unparsedEntities], so it is not a ·declared entity name·, which cvc-simple-type clause 3 requires of an ·ENTITY value·",
			v.value)
	}
	return true, nil
}
