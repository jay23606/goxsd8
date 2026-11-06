// Package xsderr is the error currency of the module: structured errors
// that carry a spec rule ID and a source location.
//
// # Using xsderr
//
// A caller holding an error from this module reaches its structured part
// with errors.As to an *xsderr.Error, which yields the Rule, the Loc and the
// Msg together; RuleOf and LocOf read the Rule or the Loc alone. An *Error
// holds no clause apart from Msg. By convention, not by check, Rule carries
// no part number — never the rule-plus-part-number name Appendix B
// recommends for reports — because one charge may cite several clauses, or a
// clause of another rule; by the same convention Msg names the clause a
// charge makes beside its Rule. Switch on Rule, and read a clause as prose.
// Error's doc says how a caller tells an *Error that is a validity verdict
// from one that is a fault.
//
// One validity class is no *Error at all: the s4s-grammar class below,
// which the spec catalogs nowhere, is a plain error that errors.As does not
// reach and RuleOf reports false for.
//
// # Design
//
// It is a pure leaf — stdlib imports only. Every schema- or
// instance-validity violation the spec CATALOGS is an *Error carrying that
// catalog's rule ID.
//
// # The s4s-grammar class
//
// A schema document that is simply not valid against the schema for schema
// documents — a prohibited or missing attribute, an unprefixed one the
// element's grammar declares nowhere, a child its content model does not
// admit, one repeated past its maxOccurs, one out of order —
// violates §2.4 clause 1 (sd-valid). That clause is anchored but
// uncataloged: it is absent from Appendix B's three tables (B.1 cvc-*, B.3
// src-*, B.4 cos-*), which are the spec's own exhaustive lists of citable
// codes, and §5.1's restatement of it carries no anchor at all. No src-*
// covers the grammar either, by construction: §2.3's gloss-src defines Schema
// Representation Constraints as constraints "beyond those which are expressed
// in" that grammar, and every src-* with sub-clauses opens "In addition to the
// conditions imposed ... by the schema for schema documents" (src-ct, src-ta).
//
// So a rejection in this class carries no Rule. It is a plain error whose
// message names the offending item, its location and the Appendix A
// production it violates. Do not charge it sd-valid, which is not a catalog
// ID and would read as one in the Rule position, and do not borrow the
// nearest src-*, which states a different constraint. Nor does the class get
// a sentinel Rule of its own: it is a real validity verdict the spec declines
// to catalog, not a non-validity fault, and a Rule minted for it would be read
// as a citation of a rule the spec does not state.
//
// # The rule catalog
//
// Rule values come from a catalog generated from the local specs, and
// IsValidRule tests membership in it.
package xsderr
