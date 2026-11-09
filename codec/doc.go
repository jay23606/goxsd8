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
//	func WithPrecisionDecimalArms[I any](numerical, positiveInfinity, negativeInfinity, notANumber I) Option
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
// that differs, and reads a sealed interface's branch or variant types from
// [WithBranches], and a PrecisionDecimal's arms from
// [WithPrecisionDecimalArms]. Package codec does not import codegen. Nothing
// encodes yet.
//
// # What decodes
//
// Decoded, each construct under the conditions and into the Go type the
// paragraphs after the list state:
//
//   - An atomic simple type of one of the twenty primitives the next
//     paragraph names, xs:precisionDecimal only with
//     [WithPrecisionDecimalArms].
//   - A list simple type.
//   - A union simple type, into a sealed interface.
//   - A top-level element declaration, as the document element's.
//   - A complex type with empty, simple or element-only content.
//   - A sequence or choice, inline or through <group ref>, occurring exactly
//     once.
//   - An element particle, local or <element ref>.
//   - A substitution group, into a sealed interface.
//   - A choice, into a sealed interface.
//   - A wildcard particle, any number of them in one sequence or choice.
//   - An element declaration of type xs:anyType, the document element's or a
//     particle's.
//   - An attribute use.
//   - A value constraint, default or fixed.
//
// A simple type decodes when it is atomic with {primitive type definition}
// xs:string, xs:anyURI, xs:boolean, xs:float, xs:double, xs:decimal,
// xs:dateTime, xs:time, xs:date, xs:gYearMonth, xs:gYear, xs:gMonthDay,
// xs:gDay, xs:gMonth, xs:duration, xs:hexBinary, xs:base64Binary, xs:QName,
// xs:NOTATION or xs:precisionDecimal. So xs:normalizedString, xs:token,
// xs:language, xs:Name, xs:NCName, xs:NMTOKEN, xs:ID, xs:IDREF, xs:ENTITY,
// xs:integer and every builtin derived from it, xs:dateTimeStamp,
// xs:dayTimeDuration, xs:yearMonthDuration, and a user restriction of any of
// them decode. Each decodes into the atomic Go type codegen gives it, by the
// rules of codegen's package doc, "# Simple types", a date, time or duration
// type into its datatype struct, an xs:QName or xs:NOTATION type into
// QName's, the expanded name its lexical resolves to as [Decode]'s doc
// states, and an xs:precisionDecimal type into PrecisionDecimal's, its Value
// the arm [WithPrecisionDecimalArms] supplies for its lexical's value, with
// that lexical's ·scale· and ·sign·, as [Decode]'s doc states. A list type
// decodes when its {item type definition} is such an atomic type or a union
// that decodes, a restriction of a list taking its base's (Structures
// §3.16.2.3), xs:NMTOKENS among them, into []A, A its item's atomic Go type,
// or []I, I its item's sealed interface, one element per list item; the
// empty list is a slice of length 0.
//
// A union type (Datatypes §2.4.1.3) decodes when each of its basic members,
// in codegen's basic-member order (codegen's package doc, "# Simple types"),
// is such an atomic or list type, no two of them get one Go type, and no
// assertions facet lies where its value's identification reads one, item 5
// of [Decode]'s doc stating each. It decodes into codegen's sealed
// interface, the variant of the value's ·active basic member·, as
// [Decode]'s doc states.
//
// A document element decodes when its declaration, a top-level one, has no
// {type table} and its {type definition} is either xs:anyType itself, or,
// with {nillable} false, a simple type that decodes, its value T's own, or a
// complex type that decodes: one with {abstract} false and no {attribute
// wildcard}, and either empty content, simple content whose {simple type
// definition} is a simple type that decodes (Structures §3.4.2.2
// dcl.ctd.ctsc), its value field's, or element-only content with no {open
// content} whose {particle} is a sequence or choice, inline or the {model
// group} of the definition a <group ref> names, occurring exactly once. Each
// particle of that sequence or choice is an element declaration with no {type
// table}, of xs:anyType itself, or, with {nillable} false, of a simple type
// that decodes or of a complex type that decodes by these same rules at any
// depth: a local one, or the top-level one an <element ref> names where
// codegen gives the particle's field that declaration's own type; or the
// particle is an <element ref> where codegen gives its field a sealed
// interface over the referenced declaration's substitution group (codegen's
// package doc, "# Substitution groups"), and each declaration of the group
// with {abstract} false is such a declaration, an abstract head's own
// properties never read; or the particle is a wildcard, whatever its
// {namespace constraint} and {process contents}. No two element particles
// share an expanded name, such an interface's particle taking each of those
// declarations'. Each attribute use is outside the xsi namespace and is typed
// the same way. No element in the instance carries xsi:type, except inside a
// wildcard's match or an element of type xs:anyType's raw content.
//
// Item 6 of [Decode]'s doc states the Go type of each field, naming it V, S
// or I as the next paragraph does. The field of a particle of complex type
// other than xs:anyType is filled from the child element as T is from the
// document element, so a type that reaches itself through an optional or
// repeated particle decodes to the instance's depth.
//
// A choice decodes into codegen's sealed interface, the branch of the one
// alternative the instance matches (§3.8.4.3 cvc-model-group) holding its one
// field: V, or a []V for an alternative whose {max occurs} is above 1 or
// unbounded, its {min occurs} 0 read as 1, or, for an <element ref>
// alternative, the same over the referenced declaration's named type, or, for
// an alternative of complex type, the same over its S or I, or its AnyType for
// one of type xs:anyType. The interface is T itself, or, where the type has an
// attribute use or one occurrence of the choice is emptiable (§3.9.6.3
// cos-group-emptiable), the first field of T, a struct whose further fields
// are the attribute uses'. An empty match is the nil interface. A particle's
// complex type gets its interface or struct by the same rule.
//
// A substitution group's interface holds, for each child, the branch of the
// group's declaration the child's expanded name resolves to (§3.9.4.3
// cvc-accept clause 2.3.2), whose field and alternatives item 6 of
// [Decode]'s doc states.
//
// Each wildcard particle decodes into its own AnyElement field codegen emits,
// or a choice's branch holding one, each child attributed to that particle
// held raw; [Decode]'s doc states the field, which children are attributed to
// it, and the raw content.
//
// An element declaration of type xs:anyType itself (Structures §3.4.7),
// nillable or not, decodes: the document element's into the named type
// codegen emits over AnyType, and a particle's, local or referenced, a
// choice's alternative and a substitution group's member included, into
// codegen's AnyType field, or the named type codegen emits over it, its
// content held raw as a wildcard's match's is; [Decode]'s doc states T and
// the field. A type derived from xs:anyType decodes or declines as its own
// complex type.
//
// Every other construct is a [*DeclineError], never a zero value:
//
//   - A content choice occurring other than exactly once.
//   - A content sequence occurring other than exactly once.
//   - Two alternatives of a choice, or two declarations of a substitution
//     group, whose branch structs could share a Go type; item 5 of
//     [Decode]'s doc states when.
//   - Two element particles taking one expanded name; item 5 of [Decode]'s
//     doc states when.
//   - An all group.
//   - A nested model group, inline or through <group ref>, a choice's
//     model-group alternative included.
//   - Mixed content.
//   - Element-only content with {open content}.
//   - A complex type with {abstract} true.
//   - A complex type with an {attribute wildcard}.
//   - An attribute use in the xsi namespace.
//   - An element declaration with a {type table}.
//   - A nillable element, but for one of type xs:anyType itself; item 5 of
//     [Decode]'s doc states when.
//   - A special type.
//   - A primitive other than those twenty: an ·implementation-defined· one
//     (Datatypes dt-primitive), which xsd.NewPrimitiveType mints and no
//     schema document yields.
//   - A list whose {item type definition} declines, by its item's construct.
//   - A union one of whose basic members declines, by that member's
//     construct.
//   - A union two of whose basic members get one Go type; item 5 of
//     [Decode]'s doc states when.
//   - A union whose value's identification reads an assertions facet; item
//     5 of [Decode]'s doc states where.
//   - An xsi:type.
//
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
// A value constraint of an element declaration or attribute use, default or
// fixed, decodes; [Decode]'s doc states the value it supplies, and that it
// supplies none to an element of type xs:anyType.
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
// other two on each list item, under its {item type definition}. On a union
// value, or a list item of a union, Decode first identifies the ·active
// basic member· (value.ValidatingType), and runs the stages under that
// member. Pattern, facet and assertion stages are validate's, decided on the
// same backend first. A whiteSpace failure, a fault of the simple type, never
// reaches Decode's return: validate runs the same whiteSpace stage first and
// leaves the instance undecided, item 4 of [Decode]'s doc. An identification,
// lexical mapping or conversion that fails is the error [Decode]'s doc states
// in item 8, naming the stage, the type, the value or the list item and its
// position, the instance Loc and, for an element's character content, its
// byte offset. Decode traces each stage it runs on the logger [WithLogger]
// sets, whose doc states each record. No package reads GOXSD_DEBUG.
//
// # Planned decode paths
//
// Decode today is the slice above, validate's facet pipeline followed by the
// stages the Debuggability section names; no value.Emitter exists. Two
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
