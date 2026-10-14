// Package bersrc maps BER-encoded (ASN.1 Basic Encoding Rules)
// documents onto the validate infoset so BER instances can be assessed
// against a compiled schema set.
//
// The engine never imports a BER decoder; this adapter walks the TLV
// stream (streaming, definite and indefinite lengths, byte offsets
// retained for Loc) and builds infoset values.
//
// # Planned mapping (M11 — not yet implemented; contract fixed now)
//
//   - The caller supplies the tag ↔ element correspondence (a schema-
//     derived tag map produced at compile time), since BER carries tags,
//     not names; constructed encodings become element children in
//     encounter order, primitive encodings become simple content in the
//     value space the schema type expects.
//   - Verdicts come from the same engine with the same cvc-* rule IDs
//     as XML and JSON; the ber conformance lane pins the mapping with
//     curated cases.
//   - BER has no DTD-shaped declaration, so the adapter will not
//     implement [github.com/kud360/goxsd8/validate.UnparsedEntities] and
//     every ·ENTITY value· will fail cvc-simple-type clause 3, as that
//     interface's doc states.
//
// Open questions to settle in the M11 design issue (with oracle
// grounding): canonical lexical bridging for binary-native values
// (integers, octet strings) and the treatment of application/context
// tag classes. This contract fixes the package's seam — TLV in, infoset
// out, engine untouched — not those answers.
package bersrc
