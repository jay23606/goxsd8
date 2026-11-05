// Package codec is the dataset serializer/deserializer: schema-directed
// decode of instance data into Go values and canonical encode back out —
// built for minimal allocation.
//
// # Decode (M10's first slice)
//
//	func Decode[T any](schema *xsd.Schema, backend value.Backend, data []byte, opts ...Option) (*T, error)
//	func CheckDecode[T any](schema *xsd.Schema, name xsd.QName, opts ...Option) error
//	type Option func(*config)
//	func WithURI(uri string) Option
//	func WithBranches[I any](branches ...I) Option
//	func WithLogger(l *slog.Logger) Option
//	type DeclineError struct{ ... }
//	func (e *DeclineError) Loc() (xsderr.Loc, bool)
//	func (e *DeclineError) Error() string
//	func (e *DeclineError) Unwrap() error
//	func (e *DeclineError) Construct() Construct
//	type Construct struct{ ... }
//	func (c Construct) String() string
//	var ErrTypeMismatch error
//	func ViolationsOf(err error) []*xsderr.Error
//	type UndecidedError struct{ ... }
//	func (e *UndecidedError) Unevaluated() validate.Unevaluated
//	func (e *UndecidedError) Error() string
//	func (e *UndecidedError) Unwrap() error
//
// [Decode] decodes a valid XML instance into the type package codegen emits
// for its document element's declaration, and [Decode]'s doc states what it
// returns in every other case; [CheckDecode]'s states what it returns for a
// declaration before an instance arrives. It reads the schema, not T:
// codegen's structs carry no tags, so Decode derives each field's name, order
// and Go type from the declaration by codegen's own rules and refuses a T
// that differs, and reads a sealed interface's branch types from
// [WithBranches]. Package codec does not import codegen. Nothing encodes yet.
//
// # What decodes
//
// A simple type decodes when it is atomic with {primitive type definition}
// xs:string, xs:anyURI, xs:boolean, xs:float, xs:double or xs:decimal. So
// xs:normalizedString, xs:token, xs:language, xs:Name, xs:NCName, xs:NMTOKEN,
// xs:ID, xs:IDREF, xs:ENTITY, xs:integer and every builtin derived from it,
// and a user restriction of any of them decode. Each decodes into the atomic
// Go type codegen gives it, by the rules of codegen's package doc, "# Simple
// types".
//
// A document element decodes when its declaration, a top-level one, has no
// {type table} and {nillable} false, and its {type definition} is either a
// simple type that decodes, its value T's own, or a complex type with
// element-only content and no {open content} or {attribute wildcard}, whose
// {particle} is an inline sequence or choice occurring exactly once. Each
// particle of that sequence or choice is an element declaration with no {type
// table} and {nillable} false, of a simple type that decodes, no two
// particles sharing an expanded name: a local one, or the top-level one an
// <element ref> names where codegen gives the particle's field that
// declaration's own type, not a sealed interface over its substitution group
// (codegen's package doc, "# Substitution groups"). Each attribute
// use is outside the xsi namespace and is typed the same way. Each field is
// A, a *A or a []A by its occurrence, A the atomic Go type of its simple
// type, an <element ref>'s the same over the named type over A codegen emits
// for the referenced declaration, an attribute's by its use's {required} and,
// as [Decode]'s doc states, its effective value constraint (§3.5.4 key-evc).
// No element in the instance carries xsi:type.
//
// A choice decodes into codegen's sealed interface, the branch of the one
// alternative the instance matches (§3.8.4.3 cvc-model-group) holding its one
// field: A, or a []A for an alternative whose {max occurs} is above 1 or
// unbounded, its {min occurs} 0 read as 1, or, for an <element ref>
// alternative, the same over the referenced declaration's named type.
// The interface is T itself, or, where the type has an attribute use or one
// occurrence of the choice is emptiable (§3.9.6.3 cos-group-emptiable), the
// first field of T, a struct whose further fields are the attribute uses'. An
// empty match is the nil interface.
//
// Every other construct is a [*DeclineError], never a zero value: a content
// choice occurring other than exactly once, two choice alternatives whose
// branch structs have one shape, an all group, a nested model group or
// <group ref>, a choice's model-group alternative included, a wildcard, an
// <element ref> to an abstract declaration or to one whose substitution group
// holds another declaration with {abstract} false, mixed, simple or empty
// content, a particle's element declaration of complex type, local or
// referenced, xs:anyType included, a nillable element, a list, a union, a
// special type, a primitive other than those six, and an xsi:type. Two of those are
// rulings rather than coverage still to come:
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
// A value constraint of an element declaration or attribute use, default or
// fixed, decodes; [Decode]'s doc states the value it supplies.
//
// Every error about the instance carries its instance Loc, under the URI
// [WithURI] names: a violation's, an undecided check's, and a decline's,
// which also names its construct and, for a construct of the schema, the
// declining component's schema Loc; and [DeclineError.Construct] returns its
// construct.
//
// # Debuggability
//
// Decode runs three stages on a value: whiteSpace, then, for an atomic Go
// type other than string, the lexical mapping and the conversion to the Go
// type. Pattern, facet and assertion stages are validate's, decided on the
// same backend first, and a whiteSpace failure is a fault of the simple type,
// a plain error. A lexical mapping or conversion that fails is the error
// [Decode]'s doc states in item 8, naming the stage, the type, the normalized
// value, the instance Loc and, for an element's character content, its byte
// offset. [WithLogger] traces each stage per value under the "codec" group:
// whiteSpace under rf-whiteSpace (Datatypes §4.3.6) with the raw text, the
// lexical mapping under cvc-datatype-valid clause 2.1 with the normalized
// value, and the conversion under dt-canonical-mapping (§2.3.1) with the
// canonical form, each record naming the type and the instance Loc. No
// package reads GOXSD_DEBUG; its scoping is #1185's.
//
// # Planned decode paths, one semantics (M10 — not yet implemented)
//
// Decode today is the slice above, validate's facet pipeline followed by
// the three stages the Debuggability section names; no value.Emitter
// exists. Each path below is planned.
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
// Generated fast paths preserve what the Debuggability section states and
// map back to their emitting backend and schema construct via header
// comments.
package codec
