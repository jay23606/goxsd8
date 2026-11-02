// Package codec is the dataset serializer/deserializer: schema-directed
// decode of instance data into Go values and canonical encode back out —
// built for minimal allocation.
//
// # Decode (M10's first slice)
//
//	func Decode[T any](schema *xsd.Schema, backend value.Backend, data []byte, opts ...Option) (*T, error)
//	type Option func(*config)
//	func WithURI(uri string) Option
//	type DeclineError struct{ ... }
//	func (e *DeclineError) Loc() xsderr.Loc
//	func (e *DeclineError) Error() string
//	func (e *DeclineError) Unwrap() error
//	func (e *DeclineError) Construct() Construct
//	type Construct struct{ ... }
//	func (c Construct) String() string
//	var ErrTypeMismatch error
//
// [Decode] decodes a valid XML instance into the struct package codegen
// emits for its document element's declaration, and [Decode]'s doc states
// what it returns in every other case. It reads the schema, not the struct:
// codegen's structs carry no tags, so Decode derives each field's name, order
// and Go type from the declaration by codegen's own rules and refuses a T
// that differs. Package codec does not import codegen. Nothing encodes yet.
//
// # What decodes
//
// A document element decodes when its declaration, a top-level one, has no
// {type table} and {nillable} false, and its {type definition} is a complex
// type with element-only content and no {open content} or {attribute
// wildcard}, whose {particle} is an inline sequence occurring exactly once.
// Each particle of that sequence is a local element declaration with no
// {type table} and {nillable} false, of a simple type that is atomic with
// {primitive type definition} xs:string or xs:anyURI, no two particles
// sharing an expanded name; each attribute use is outside the xsi namespace
// and is typed the same way. So xs:normalizedString, xs:token, xs:language,
// xs:Name, xs:NCName, xs:NMTOKEN, xs:ID, xs:IDREF, xs:ENTITY, xs:anyURI and a
// user restriction of any of them decode, each field a string, a *string or a
// []string by its occurrence. No element in the instance carries xsi:type.
//
// Every other construct is a [*DeclineError], never a zero value: a choice,
// an all group, a nested model group or <group ref>, a wildcard, an <element
// ref> or substitution group, mixed, simple or empty content, a local
// element of complex type, xs:anyType included, a nillable element, a list,
// a union, a special type, any other primitive, and an xsi:type.
// Two of those are rulings rather than coverage still to come:
//
//   - A declaration with a {type table} declines. The slice that decodes it
//     takes the branch from the type the validator selected
//     (validate.Validator.ConditionallySelected) and never evaluates a {test}
//     itself.
//   - An element carrying xsi:type declines, whatever type it names. Where
//     the ·selected type definition· is not abstract, codegen emits no
//     branch for a type derived from it (Structures §3.3.4.3 cvc-elt clause
//     4), and decoding the instance into the selected type's field would
//     lose what the derived type governs: under xsi:type="xs:token" an
//     xs:string element's value collapses. That refusal is permanent.
//
// A value constraint of an element or attribute use, default or fixed,
// decodes; [Decode]'s doc states the value its field holds.
//
// Every error about the instance carries its instance Loc, under the URI
// [WithURI] names: a violation's, an undecided check's, and a decline's,
// which also names its construct and, for a construct of the schema, the
// declining component's schema Loc; and [DeclineError.Construct] returns its
// construct.
//
// # Planned decode paths, one semantics (M10 — not yet implemented)
//
// Decode reads no value space: every value in the slice is the string its
// whiteSpace stage leaves (value.ApplyWhiteSpace). Each path below is
// planned.
//
//   - Runtime path (always available): the facet pipeline +
//     value.Mapping, driven by the compiled schema. General, reflective,
//     allocation-tolerant. Hot-path APIs follow the appender convention
//     (AppendCanonical(dst []byte, v) []byte; ParseBytes([]byte)) so
//     even this path is allocation-frugal.
//   - Generated fast path: code emitted at codegen time via
//     value.Emitter — no intermediate strings, no boxed values, facet
//     checks inlined, zero-allocation scalar decode on the native
//     backend.
//
// Both paths implement the SAME pipeline stages with the SAME spec rule
// IDs. Differential tests feed identical input to both and require
// identical values and identical error rule IDs; testing.AllocsPerRun
// benchmarks pin the fast path's budget. A fast path that disagrees with
// the runtime path is wrong by definition.
//
// # Planned debuggability (M10 — not yet implemented)
//
// Every decode error a pipeline stage raises carries the stage that
// rejected (whitespace / pattern / lexical-map / facet / assertion), the
// type QName, the offending input fragment, and the instance Loc + byte
// offset. GOXSD_DEBUG=codec traces stage transitions per value through the
// injected slog logger. Generated fast paths preserve all of it and map
// back to their emitting backend and schema construct via header comments.
// No stage of Decode rejects a value today: the validator has accepted
// every value it reads.
package codec
