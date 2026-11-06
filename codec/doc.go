// Package codec is the dataset serializer/deserializer: schema-directed
// decode of instance data into Go values and canonical encode back out —
// built for minimal allocation.
//
// # Decode
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
// xs:string, xs:anyURI, xs:boolean, xs:float, xs:double, xs:decimal,
// xs:dateTime, xs:time, xs:date, xs:gYearMonth, xs:gYear, xs:gMonthDay,
// xs:gDay, xs:gMonth or xs:duration. So xs:normalizedString, xs:token,
// xs:language, xs:Name, xs:NCName, xs:NMTOKEN, xs:ID, xs:IDREF, xs:ENTITY,
// xs:integer and every builtin derived from it, xs:dateTimeStamp,
// xs:dayTimeDuration, xs:yearMonthDuration, and a user restriction of any of
// them decode. Each decodes into the atomic Go type codegen gives it, by the
// rules of codegen's package doc, "# Simple types", a date, time or duration
// type into its datatype struct. A list type decodes when its {item type
// definition} is such an atomic type, a restriction of a list taking its
// base's (Structures §3.16.2.3), xs:NMTOKENS among them, into []A, A its
// item's atomic Go type, one element per list item; the empty list is a
// slice of length 0.
//
// A document element decodes when its declaration, a top-level one, has no
// {type table} and {nillable} false, and its {type definition} is either a
// simple type that decodes, its value T's own, or a complex type that
// decodes: one with element-only content and no {open content} or {attribute
// wildcard}, whose {particle} is an inline sequence or choice occurring
// exactly once. Each particle of that sequence or choice is an element
// declaration with no {type table} and {nillable} false, of a simple type
// that decodes or of a complex type that decodes, other than xs:anyType and
// with {abstract} false, by these same rules at any depth: a local one, or the
// top-level one an <element ref> names where codegen gives the particle's
// field that declaration's own type; or the particle is an <element ref>
// where codegen gives its field a sealed interface over the referenced
// declaration's substitution group (codegen's package doc, "# Substitution
// groups"), and each declaration of the group with {abstract} false is such
// a declaration, an abstract head's own properties never read. No two
// particles share an expanded name, such an interface's particle taking each
// of those declarations'. Each attribute use is outside the xsi namespace and
// is typed the same way. No element in the instance carries xsi:type.
//
// Item 6 of [Decode]'s doc states the Go type of each field, naming it V, S
// or I as the next paragraph does. The field of a particle of complex type
// is filled from the child element as T is from the document element, so a
// type that reaches itself through an optional or repeated particle decodes
// to the instance's depth.
//
// A choice decodes into codegen's sealed interface, the branch of the one
// alternative the instance matches (§3.8.4.3 cvc-model-group) holding its one
// field: V, or a []V for an alternative whose {max occurs} is above 1 or
// unbounded, its {min occurs} 0 read as 1, or, for an <element ref>
// alternative, the same over the referenced declaration's named type, or, for
// an alternative of complex type, the same over its S or I.
// The interface is T itself, or, where the type has an attribute use or one
// occurrence of the choice is emptiable (§3.9.6.3 cos-group-emptiable), the
// first field of T, a struct whose further fields are the attribute uses'. An
// empty match is the nil interface. A particle's complex type gets its
// interface or struct by the same rule.
//
// A substitution group's interface holds, for each child, the branch of the
// group's declaration the child's expanded name resolves to (§3.9.4.3
// cvc-accept clause 2.3.2), whose field and alternatives item 6 of
// [Decode]'s doc states.
//
// Every other construct is a [*DeclineError], never a zero value: a content
// choice occurring other than exactly once, two branches of a sealed
// interface, a choice's alternatives or a substitution group's declarations,
// whose structs have one shape or which share a name where either is of
// complex type, holds a substitution group's interface or holds a datatype
// struct or a list of one, an all group, a nested model group or <group ref>,
// a choice's model-group alternative included, a wildcard, mixed, simple or
// empty content, a particle's element declaration of type xs:anyType or of a
// complex type with {abstract} true, local or referenced, a nillable element,
// a union, a special type, a primitive other than those fifteen, a list whose
// {item type definition} is a union or of such a primitive, declining by its
// item's construct, and an xsi:type. Two of those are rulings rather than
// coverage still to come:
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
// type. On a list value whiteSpace runs once, under the list type, and the
// other two on each list item, under its {item type definition}. Pattern,
// facet and assertion stages are validate's, decided on the same backend
// first, and a whiteSpace failure is a fault of the simple type, a plain
// error. A lexical mapping or conversion that fails is the error [Decode]'s
// doc states in item 8, naming the stage, the type, the normalized value or
// the list item and its position, the instance Loc and, for an element's
// character content, its byte offset. Decode traces each stage it runs on the
// logger [WithLogger] sets, whose doc states each record. No package reads
// GOXSD_DEBUG.
//
// # Planned decode paths
//
// Decode today is the slice above, validate's facet pipeline followed by the
// three stages the Debuggability section names; no value.Emitter exists. Two
// paths are planned, neither implemented: a runtime path, always available,
// of the facet pipeline and value.Mapping driven by the compiled schema,
// general, reflective and allocation-tolerant, whose hot-path APIs follow the
// appender convention (AppendCanonical(dst []byte, v) []byte;
// ParseBytes([]byte)) so that it too is allocation-frugal; and a generated
// fast path, code emitted at codegen time via value.Emitter, with no
// intermediate strings, no boxed values, facet checks inlined and
// zero-allocation scalar decode on the native backend. Both implement the
// SAME pipeline stages with the SAME spec rule IDs: differential tests feed
// identical input to both and require identical values and identical error
// rule IDs, testing.AllocsPerRun benchmarks pin the fast path's budget, and a
// fast path that disagrees with the runtime path is wrong by definition.
// Generated fast paths preserve what the Debuggability section states and map
// back to their emitting backend and schema construct via header comments.
package codec
