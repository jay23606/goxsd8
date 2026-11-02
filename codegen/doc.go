// Package codegen emits Go type declarations from a compiled schema set. It
// emits no decode or encode code: decoding into the declarations is package
// codec's, whose package doc states which of them it decodes today.
//
// # Entry point
//
//	type Target struct { Schema *xsd.Schema; Dir, Package, Name string }
//	func Generate(targets []Target) error
//	func CheckTargets(targets []Target) []error
//
// Each target carries one finalized [xsd.Schema] — what parser.Parse
// assembles from a root location and its composition closure; the
// §3.17.1 Schema component is already multi-namespace-capable, so there
// is no separate schema-set type. Each target emits one package into its
// own directory. [Generate] states the phases, the decline and destination
// rules, and the file it writes; [CheckTargets] states which targets it
// refuses whatever their Schemas hold.
//
//	s, err := parser.Parse("testdata/example.xsd")
//	// ...
//	err = codegen.Generate([]codegen.Target{{Schema: s, Dir: "gen/order", Package: "order"}})
//
// ExampleGenerate, in codegen/example_test.go, runs those calls, into a
// temporary directory, and prints the whole file Generate writes for that
// schema, which is inside the slice "Coverage" states. Plain go doc does not
// print Example functions: read it in that file, or on pkg.go.dev beside this
// documentation.
//
// # What you get
//
// The emitted structs carry no struct tags and bind no XML name; do not decode
// them with encoding/xml: decoding is codec's, as the opening paragraph states.
// The types are declarations only: validate an instance against the Schema
// with package validate, never through the structs. codec.Decode fills the
// structs of the slice codec's package doc states, and you fill every other.
// Package validate is today what enforces the constraints the types do not:
// facets, {assertions}, identity constraints, value constraints and wildcard
// constraints, the last four as "Complex types", "Particles", "Open content",
// "Attributes" and "Element declarations" state.
//
// Each supported construct gets the Go shape below, a summary of the rules
// each row quotes, in the sections from "Simple types" to "Element
// declarations", by a rule's opening or by a sentence of one, which state its
// exceptions.
//
//   - complex type → a struct ("A sequence or an all group gives"), or its
//     content choice's sealed interface ("A choice gives")
//   - element → the type of its {type definition}, for a simple type a named Go
//     type over the value type ("An element whose {type definition}")
//   - choice occurring once → a sealed interface, one branch struct per
//     alternative ("A choice gives")
//   - {min occurs} 0 with {max occurs} 1 → a nil-able field, a pointer or a bare
//     interface; {max occurs} above 1 → a slice ("{max occurs} above 1")
//   - decimal → big.Rat, xs:integer and the unbounded integer types → big.Int,
//     the bounded ones → the Go integer of their range ("For an atomic type",
//     "Failing that")
//   - union → a sealed interface, one variant per basic member ("A union type")
//   - facet, an enumeration included → no change of Go type: a user restriction
//     of an atomic or list type gets the Go type its base gets, a restriction
//     of string giving string, and a user restriction of a union gets its own
//     interface over its base's basic members ("Facets are not represented",
//     "Each union")
//   - precisionDecimal → the struct PrecisionDecimal over a sealed value sum
//     ("PrecisionDecimal holds one")
//   - <element ref> to a head that is abstract, or whose group holds another
//     non-abstract declaration → a sealed interface, one branch per
//     non-abstract member ("An <element ref> particle to a head")
//   - element with other than one governing type (an abstract complex type's
//     members, or the types its {type table} selects) → a sealed interface,
//     one branch per governing type ("An element declaration's governing types")
//   - repeated model group with a repeating edge, repeated emptiable sequence,
//     or choice with an emptiable model group alternative → a slice of a
//     sealed interface, one branch per basic particle ("A model group particle
//     meeting")
//
// # Coverage
//
// Supported, each construct emitted from or accepted as the rules below state:
//
//   - element declarations, top-level, and local with {abstract} false,
//     nillable or not, with or without a {type table}
//   - <element ref> particles and substitution groups
//   - the special types xs:anySimpleType and xs:anyAtomicType
//   - atomic simple types, of every primitive Datatypes defines and of
//     precisionDecimal
//   - list simple types
//   - union simple types
//   - xs:anyType
//   - complex types, named or anonymous, derived by restriction or extension
//   - complex types with {abstract} true, as an element declaration's {type
//     definition} or a type its {type table} selects
//   - empty, simple, element-only and mixed content, the last two with or
//     without {open content}
//   - sequences, choices with or without {particles}, and all groups, inline or
//     through <group ref>
//   - optional and repeated particles
//   - a repeated model group with a repeating edge, a repeated emptiable
//     sequence, and a choice with an emptiable model group alternative, each
//     as a flattened field
//   - wildcard particles
//   - attribute uses and attribute wildcards, beside any content above
//   - facets, {assertions}, value constraints and identity constraints
//
// No mode generates the part of a schema that avoids a declined construct;
// [Generate] states what a decline does, and "Declined today" below states
// each construct's reason.
//
// Declined today, each construct named as [Construct.String] prints it for a
// [DeclineError]'s [DeclineError.Construct] (see [Construct] for comparing
// them), each a permanent decline: uncovered primitive, local element
// declaration with {abstract} true and struct-value recursion, for the reasons
// the paragraph after the list and, for struct-value recursion, "Particles"
// state; and particle with {max occurs} 0, since an element, group reference,
// model group or wildcard with minOccurs=maxOccurs=0 maps to no component at
// all (Structures §3.3.2, §3.7.2, §3.8.2, §3.10.2).
//
//   - uncovered primitive
//   - particle with {max occurs} 0
//   - local element declaration with {abstract} true
//   - struct-value recursion
//
// A list or union declines with the Construct its item type or member declines
// with. An uncovered primitive is a primitive other than the nineteen Datatypes
// defines (Datatypes §3.3) and precisionDecimal: an ·implementation-defined·
// one (Datatypes dt-primitive), which xsd.NewPrimitiveType mints and no schema
// document yields. Nor does any schema document yield a local element
// declaration with {abstract} true, since xs:localElement prohibits abstract
// (Structures Appendix A, the schema for schemas), though
// xsd.NewElementDeclaration builds one; no element is valid against it
// (Structures §3.3.4.3 cvc-elt clause 2).
//
// Generate emits, in document order:
//
//   - one Go type per top-level element declaration with {abstract} false
//   - one per anonymous complex type nested in such a declaration's type
//     definition, in a named complex type a local element declaration, a
//     governing type or an extension's {base type definition} reaches, or in a
//     model group definition a <group ref> particle names
//   - one per named complex type a local element declaration reaches
//   - one per place a struct's or a branch struct's particles hold a choice or a
//     sequence that is not spliced; a sequence in a sequence or as a choice's
//     alternative, and an all group in an all group, spliced by the occurrence
//     rules below, give the places of their own particles instead
//   - one for a complex type's own content particle, a sequence, a choice or an
//     all group, only when it is a sequence or an all group that is not
//     spliced, or a choice that occurs other than exactly once, that has
//     {particles} one occurrence of which is emptiable, or whose type has mixed
//     content, an attribute use other than one of xsi:nil, an {attribute
//     wildcard} or {open content}
//   - one branch struct per alternative of each choice it emits as a sealed
//     interface
//   - one branch struct per basic particle a flattened model group particle
//     reaches, after the sealed interface the two items above give that
//     particle's place in place of its occurrence type
//   - one sealed interface per head an <element ref> particle names that has
//     {abstract} true or whose substitution group holds another declaration
//     with {abstract} false, with one branch struct per declaration of its
//     group with {abstract} false
//   - one sealed interface per element declaration with other than one
//     governing type, in place of that declaration's type, with one branch
//     struct per governing type, each followed by the types its field reaches,
//     the struct of a member of an abstract complex type among them
//
// By the item for each place a struct's or a branch struct's particles hold a
// choice or a sequence, and the item for a complex type's own content particle,
// a particle that a model group definition or an extension's base holds gets
// one in each struct that holds it, each shared as an anonymous complex type
// nested beside it is.
//
// An <element ref> particle gets no type of its own: its field holds the type
// emitted for the top-level element declaration it names, or, for a head the
// list above names, that head's sealed interface.
//
// Then, after every other declaration and in the order the walk first needs
// each, Generate emits:
//
//   - one datatype struct per date, time or duration datatype, one for QName
//     and NOTATION together, and PrecisionDecimal followed by its sealed
//     interface and that interface's four arms, each that a declaration's type
//     or a value field is declared over
//   - the four raw-content structs once an element's type is xs:anyType, a
//     model group holds a wildcard particle, or a type has an {attribute
//     wildcard} or {open content}
//   - the generic struct MixedChild once a field of mixed content holds an
//     element
//   - the generic sealed interface Nillable and its arms Nilled and Content
//     once a field holds a nillable element or the package defines the type
//     of a nillable top-level declaration other than one over AnyType or a
//     governing types' interface; an aliased one's nilled value is its
//     owner's Nillable (Generate)
//   - one attributes struct per complex type with attribute fields a nillable
//     element has
//   - one sealed interface per union simple type a declaration's type, a value
//     field, a list's item or a union's member is declared over, each followed
//     by one variant type per basic member
//
// Generate declines everything outside this slice, which the rules in the
// eight sections that follow, "Simple types" to "Element declarations", state.
//
// # Simple types
//
//   - An element whose {type definition} is a simple type gets a named type
//     over string for a special type (Datatypes §2.4.2 dt-special),
//     xs:anySimpleType or xs:anyAtomicType, known by identity whatever its
//     {variety}, and otherwise over the atomic Go type an atomic simple type
//     gets, over a slice of one, or over the union's sealed interface. A
//     special type's string holds the literal as a string: neither a value of
//     the type nor an xsi:type naming another type is represented.
//   - For an atomic type, the nearest of the eight bounded integer types on the
//     type's {base type definition} chain (§3.16.1), the type itself first,
//     gives the Go integer whose range is exactly its value space (Datatypes
//     §3.4.16–§3.4.19, §3.4.21–§3.4.24): long gives int64, int int32, short
//     int16, byte int8, unsignedLong uint64, unsignedInt uint32, unsignedShort
//     uint16, unsignedByte uint8. Failing one, a chain that reaches xs:integer
//     gives big.Int, which holds every integer (Datatypes §3.4.13): xs:integer,
//     the four unbounded integer types and every type derived from them other
//     than through the eight bounded ones; and a chain that reaches
//     dateTimeStamp, dayTimeDuration or yearMonthDuration gives that builtin's
//     datatype struct.
//   - Failing that, the type's {primitive type definition} (Datatypes §4.1.1)
//     chooses: string and anyURI give string, boolean gives bool, float gives
//     float32 and double float64, whose IEEE 754 binary32 and binary64 value
//     spaces (Datatypes §3.3.4.1, §3.3.5.1) those Go types hold exactly,
//     decimal gives big.Rat, which holds every decimal value exactly (Datatypes
//     §3.3.3), hexBinary and base64Binary give []byte, which holds every finite
//     sequence of octets exactly (Datatypes §3.3.15.1, §3.3.16.1), nil and the
//     empty slice being one value, no octets, and dateTime, time, date,
//     gYearMonth, gYear, gMonthDay, gDay, gMonth and duration each give their
//     datatype struct, QName and NOTATION give QName's (Datatypes §3.3.18,
//     §3.3.19), and precisionDecimal gives its datatype struct PrecisionDecimal
//     (precisionDecimal §3.1). That Go integer, string, bool, float32, float64,
//     big.Int, big.Rat, []byte or datatype struct is the type's atomic Go type.
//   - A date, time or duration datatype struct has one field per property its
//     datatype's values carry (Datatypes §3.3.6.1, §3.3.7.1–§3.3.14.1,
//     §3.4.26–§3.4.28), in the order of the seven-property model (§D.2.1) or of
//     duration's two properties: Year a big.Int; Month, Day, Hour and Minute a
//     uint8; Second a big.Rat; TimezoneOffset, in minutes, an int16 for
//     dateTimeStamp, whose values always carry one, and a *int16, nil when
//     absent, for the others; Months a big.Int and Seconds a big.Rat, duration
//     having both, dayTimeDuration Seconds alone and yearMonthDuration Months
//     alone.
//   - A date, time or duration datatype struct holds every value of its
//     datatype exactly, in the local property values the model stores rather
//     than converted to UTC (§D.2.1). It also holds struct values no value maps
//     to, such as a Month of 13, a Day beyond its month's last, or Months and
//     Seconds of opposite signs, and it represents neither its datatype's
//     equality nor its order.
//   - QName's datatype struct, which NOTATION shares, holds the expanded name a
//     literal resolves to (Datatypes §3.3.18, §3.3.19): Space is the namespace
//     name, "" for none, and Local the local part; no prefix is represented. It
//     also holds struct values no value maps to, such as a Local that is not an
//     NCName or, for NOTATION, a name no <notation> declares.
//   - PrecisionDecimal holds one precisionDecimal value (precisionDecimal §3.1
//     sec-vs-pD) as its one field Value, of the sealed interface
//     PrecisionDecimalValue, the closed-sum exception as a choice's is: one
//     method, unexported, "is" plus the interface's name, and one arm per case
//     of the value's ·numericalValue· (vp-pd-numVal), emitted after the
//     interface in this order. PrecisionDecimalValueNumerical holds Coefficient,
//     a big.Int, the value's magnitude, never negative; Scale, a big.Int, its
//     ·scale· (vp-pd-precision); and Negative, its ·sign· (vp-pd-sign), so that
//     the value is Coefficient × 10^−Scale, negative when Negative.
//     PrecisionDecimalValuePositiveInfinity,
//     PrecisionDecimalValueNegativeInfinity and PrecisionDecimalValueNotANumber,
//     the special values (dt-specialvalue), have no field: their ·scale· is
//     absent, and their ·sign· absent for notANumber and given by the
//     infinities. So 3 and 3.00 are two values, and 0 and −0 two, while the
//     literals of one value, such as 3.0e2 and 30e1, or INF and +INF, are one:
//     no lexical form is represented.
//   - PrecisionDecimal holds every value of its datatype exactly. It also holds
//     struct values no value maps to, a nil Value or a negative Coefficient,
//     and it represents neither its datatype's equality nor its order: 0 and −0
//     are equal, and NaN is incomparable with every value, itself included
//     (precisionDecimal §3.1).
//   - The generated file imports math/big exactly when a declaration
//     names it: a big.Int or big.Rat, a datatype struct, all but
//     GMonthDay, GDay, GMonth, QName and PrecisionDecimal holding one, or
//     PrecisionDecimalValueNumerical.
//   - A list type (Datatypes §2.4.1.2) gives a slice of the Go type its {item
//     type definition} (§3.16.1) gets, an atomic Go type or a union's sealed
//     interface, a restriction of a list taking its base's item (§3.16.2.3);
//     the list declines exactly when its item does, by the item's construct.
//     Wherever a field holds a list, the pointer and slice rules below apply to
//     that slice as to any atomic Go type; in a field that is not a pointer,
//     nil and the empty slice are one value, the empty list. Facets are not
//     represented and do not change the Go type a simple type gets, and a named
//     user simple type other than a union gets no Go type of its own.
//   - A union type (Datatypes §2.4.1.3) gives a sealed interface whose one
//     method is unexported, "is" plus the interface's name, and one variant
//     type per basic member (dt-basicmember), emitted after the interface in
//     basic-member order: its {member type definitions} (§3.16.1), a
//     restriction of a union taking its base's (§3.16.2.1), in order, each
//     member that is itself a union replaced by its own basic members, and each
//     member kept at its first position only, a list member being basic. Each
//     variant is a named type over the Go type its member gets, an atomic Go
//     type or a list's slice, and implements the method; consumers type-switch
//     over the variants.
//   - Each union, named or anonymous, gets one interface however many
//     declarations reach it, and a restriction of a union gets its own. The
//     union declines exactly when one of its basic members does, by that
//     member's construct, at the first declining member in basic-member order.
//   - A union's interface does not enforce the ·active basic member·, the first
//     member in order that accepts a literal: with xs:int first, a string
//     variant can hold "5", which xs:int accepts. Facets on the union itself
//     are not represented. A union with no basic member, which §3.16.1 admits
//     though no schema document yields one
//     (src-union-memberTypes-or-simpleTypes), gets its interface alone, which
//     no variant implements.
//
// # xs:anyType
//
//   - An element whose {type definition} is xs:anyType (§3.4.7
//     any-type-itself), named by its type attribute or the default for an
//     element declared with no type and no substitutionGroup (§3.3.2), holds
//     its raw content. A top-level element gets a named type over the struct
//     AnyType. A local element's field, and a choice alternative's branch,
//     holds AnyType, by the occurrence rules below. This is xs:anyType's alone:
//     a user complex type of the same shape, or derived from xs:anyType, is a
//     complex type as the complex-type bullets below state.
//   - AnyType holds the element's [attributes], xsi: ones included, in document
//     order as AnyAttribute values of Space, Local and Value, at most one per
//     expanded name, which the struct does not enforce; then its mixed content
//     as Text, the characters before the first child element, "" for none, and
//     Children, one AnyChild per child element in order, holding that child as
//     Element and as Tail the characters after it, before the next child or the
//     end, so that each gap between children is one string.
//   - An AnyElement holds a child's expanded name, Space and Local, and its own
//     content as an AnyType: a wildcard's match is held raw, whether or not a
//     declaration or an xsi:type would govern it. Namespace declarations are
//     not represented, so a prefix inside a value, such as an xsi:type's, does
//     not resolve from the struct. AnyType reaches itself only through the
//     slice Children, so no element of type xs:anyType closes a struct-value
//     recursion, and none of the four names math/big.
//
// # Complex types
//
//   - An element whose {type definition} is a complex type other than
//     xs:anyType with {abstract} false, named or anonymous, gets a type of
//     its own whatever its derivation, restriction or extension, of complex or
//     simple content (§3.4.2.2 dcl.ctd.ctsc, §3.4.2.3 dcl.ctd.ctcc). The type
//     is built from its own {content type} (§3.4.2.3.3 dcl.ctd.ctcc.common,
//     §3.4.2.2 dcl.ctd.ctsc) and {attribute uses}, which hold the uses it
//     inherits, all of an extension's base's and those of a restriction's base
//     it neither overrides nor prohibits (§3.4.2.4 dcl.ctd.attuses), and has
//     no Go-level relation to its base's: no embedding, no conversion.
//   - A complex type's {assertions}, inherited ones included (§3.4.2.1
//     dcl.ctd.common), are accepted and not represented. An assertion is one
//     more condition an instance must satisfy and adds no content or attribute
//     (§3.4.4.2 cvc-complex-type clause 6). So the type emits exactly as it
//     would without them, and enforcing them is the validator's.
//   - The complex type has one of four contents: empty content ({variety} empty,
//     §3.4.1); simple content ({variety} simple), which gives a struct of the
//     value field stated below and the attribute fields; or element-only or
//     mixed content (§3.4.2.3.3 dcl.ctd.ctcc.common), with or without {open
//     content}, whose {particle}, which carries the <sequence>'s, <choice>'s or
//     <all>'s own {min occurs} and {max occurs} (§3.9.1), or a <group ref>'s,
//     whose {term} is the referenced definition's {model group} (§3.7.2), holds
//     a sequence, a choice or an all group of element, wildcard (§3.10.2.1
//     dcl.wc.p) and model group particles, each model group particle, inline or
//     a <group ref>, a sequence, choice or all group that holds the same:
//     element particles are local element declarations, and <element ref>
//     particles, whose {term} is the top-level declaration the ref names
//     (§3.3.2.4).
//   - A sequence or an all group gives a struct, and so does empty content,
//     with no particle field; a struct with no field is spelled struct{}.
//   - Mixed content ({variety} mixed, §3.4.1) gives the struct element-only
//     content of the same {particle} gives, with two changes. Its first field,
//     ahead of every field the rules from "Simple types" to "Element
//     declarations" give, is Text, a string holding the characters before the
//     first child element, "" for none. And every field its particles give for
//     an element particle, in the struct, in an occurrence type, or in a
//     choice's or a flattened interface's branch struct, holds MixedChild[T] in
//     place of the T it would hold, whatever T is, a sealed interface included:
//     MixedChild holds that child as Element and as Tail the characters after
//     it, before the next child or the end, and the occurrence rules below apply
//     to MixedChild[T] as to any struct, so {min occurs} 0 with {max occurs} 1
//     gives *MixedChild[T]. A wildcard particle's field holds AnyChild in place
//     of AnyElement. A field holding a model group's occurrence type, a choice's
//     interface or a flattened field's slice is not wrapped: it is not a child
//     element.
//   - So the instance's character data round-trips: Text, then each child in
//     the order the struct holds it, each followed by its Tail. The characters
//     are raw and of no simple type (§3.4.4.2 cvc-complex-type clause 1.4
//     validates the element children alone); mixed content with an empty
//     {particle} gives Text and the attribute fields alone. A present
//     MixedChild whose Element is a nil sealed interface is not a child.
//   - Mixedness is read from the type's own {content type}, never its base's: a
//     restriction of a mixed base whose own content is element-only gets
//     neither Text nor MixedChild, and an extension of a mixed base is mixed
//     (§3.4.2.3.3 clause 4.2, cos-ct-extends clause 1.4.3.2.2.1), its base's
//     particle fields wrapped as its own are.
//   - Simple content gives a struct whose first field, the value field, is
//     named from the local name value and holds a special type's string, the
//     atomic Go type, the slice, or the union's sealed interface its {simple
//     type definition} gives as an element's {type definition} does, never a
//     pointer to it; the value field declines exactly when that type does, by
//     the type's construct. That type is the base itself for an extension of a
//     simple type, the base's own for an extension of a complex type, and for a
//     restriction an anonymous restriction of the base's own or of an inline
//     <simpleType>, whose facets are not represented (§3.4.2.2 dcl.ctd.ctsc).
//   - A choice gives a sealed interface, the closed-sum exception, which a
//     complex type's content {particle} gives as the type itself unless a rule
//     below gives a struct holding it: an interface whose one method is
//     unexported, "is" plus the interface's name, and one branch struct per
//     alternative in particle order, emitted after the interface, each holding
//     the fields the alternative's particle gives as a particle of a sequence
//     would, its {min occurs} 0 read as 1 and its {max occurs} kept — an
//     element or a wildcard its one field, a sequence occurring exactly once
//     the fields of its own particles, any other model group its one field —
//     and implementing the method; consumers type-switch over the branches,
//     and "N pointer fields, exactly one non-nil" never appears. A choice with
//     no {particles}, which matches no sequence of children, the empty one
//     included (§3.8.4.3 cvc-model-group), gets the interface alone, which no
//     branch implements, so its one value is nil, and which a content
//     {particle} gives as the type itself, or a field gives, as it does any
//     choice.
//   - One occurrence of a choice with {particles} matches nothing exactly when
//     one of its alternatives is emptiable (§3.8.6.6 cos-choice-range,
//     §3.9.6.3 cos-group-emptiable), and the nil interface is the one value for
//     that empty match: no branch holds it, an element or a wildcard
//     alternative with {min occurs} 0 and {max occurs} 1 giving a value field,
//     never a pointer. A repeated choice's slice holds one item per occurrence
//     that matches something, so it does not count occurrences that match
//     nothing.
//   - A choice particle one of whose alternatives' term is a model group,
//     inline or through <group ref>, one occurrence of which is emptiable
//     ([xsd.Schema.ParticleEmptiable]), whatever the alternative's own {min
//     occurs} and the choice's own occurrence, the content {particle}
//     included, is flattened, as stated below: a branch over that alternative
//     would hold the empty match.
//
// # Substitution groups
//
//   - An <element ref> particle to a head that has {abstract} true, or whose
//     substitution group ([xsd.Schema.SubstitutionGroup], §3.3.6.4 key-eq)
//     holds a declaration with {abstract} false other than the head, gives a
//     field holding a sealed interface over the group's declarations with
//     {abstract} false, the closed-sum exception as a choice's is: one method,
//     unexported, "is" plus the interface's name, and one branch struct per
//     such declaration, the head included when it has {abstract} false, in the
//     order [xsd.Schema.Elements] returns them, emitted after the interface,
//     each holding one field, named from the declaration's local name, of the
//     type emitted for that top-level declaration, and implementing the method.
//     The field takes the occurrence rules below.
//   - A substitution group's declarations' own types are emitted unchanged,
//     each at its own position, and one interface is emitted per head however
//     many particles reach it.
//   - A head with {abstract} false whose {disallowed substitutions} contains
//     substitution (cos-equiv-derived-ok-rec clause 2.1), or whose group holds
//     no other declaration with {abstract} false, gives the field of an
//     <element ref> particle to it the head's own type. {abstract} is read per
//     declaration once the group is taken: an abstract declaration gets no
//     branch, and the members the group holds through it
//     (cos-equiv-derived-ok-rec clause 2.2) get theirs. An abstract head whose
//     group holds no other declaration with {abstract} false gets the interface
//     alone, whose one value is nil. Where the interface is first needed, each
//     branch's declaration is checked in group order as at an <element ref>
//     particle to it, its properties and the shape of its type, so a declining
//     one declines the interface there.
//
// # Particles
//
//   - A sequence or all group {particle} that is not spliced, a choice
//     {particle} that occurs other than exactly once, or a choice {particle}
//     that occurs exactly once and either has {particles} one occurrence of
//     which is emptiable or whose type has mixed content, an attribute use
//     other than one of xsi:nil, an {attribute wildcard} or {open content}
//     (§3.4.2.4 dcl.ctd.attuses, §3.4.2.5 dcl.ctd.anyatt, §3.4.2.3.3
//     dcl.ctd.ctcc.common), inherited ones included, gives a struct whose first
//     field, after mixed content's Text, holds what one occurrence of its model
//     group gives — a struct of the sequence's or all group's particle fields
//     alone, or the choice's sealed interface — by the occurrence rules below,
//     followed by the open-content field and the attribute fields; the field
//     and that type are named as if from the local name sequence, choice or all
//     ("Naming"). A model group particle in a sequence or as a choice's
//     alternative, a choice or a sequence that is not spliced, gives one such
//     field in the same way, a choice occurring exactly once giving the bare
//     interface. The occurrence's fields lie behind that slice, pointer or
//     interface, so no struct-value recursion passes through it. A flattened
//     particle gives in place of either field the flattened field stated
//     below.
//   - Each particle of a sequence gives one field, in particle order, but a
//     sequence particle spliced by the occurrence rules below, inline or a
//     <group ref> to a sequence group, which gives in its place the fields its
//     own particles give, so an extension (§3.4.2.3.3 clause 4.2.3.3) gives its base
//     particle's fields and then its own: a local element with a simple type
//     gets a special type's string, the atomic Go type, the slice, or the
//     interface an element's type is declared over, one with an anonymous
//     complex type of the same shape gets a struct or interface of its own,
//     emitted after its parent.
//   - A local element with a named complex type of the same shape gets the
//     struct or interface emitted for that type, once per expanded name, after
//     the parent of the first local element of that type, and shared by every
//     local element of that type in any parent, the type's own included; a
//     top-level element of that type keeps the type named from the element.
//   - A type nested in a named complex type is emitted once per path of
//     expanded names from that type, after the first element whose walk reaches
//     it, and every element of that type or of a type extending it, top-level
//     or local, shares it; a type nested in a model group definition is emitted
//     once per path of expanded names from that group, after the first element
//     whose walk reaches it, and every <group ref> particle to the group shares
//     it, each element the group declares being one component (§3.7.2); two
//     sibling local elements whose expanded names differ only in {target
//     namespace} (§3.3.2.3), a qualified x beside an unqualified x, get a type
//     each.
//   - An <element ref> particle gets the type emitted for the top-level element
//     declaration it names, named from that element whatever its type
//     definition, or the sealed interface over its substitution group as stated
//     above, and its field is named from that declaration's local name.
//   - A wildcard particle gives one field named Any by "Naming", holding
//     AnyElement, or AnyChild in mixed content, by the occurrence rules
//     below. Neither its {namespace constraint} nor its {process contents}
//     (§3.10.1) is represented: the field holds elements the wildcard would
//     not admit (cvc-wildcard), and holds every match raw whether a strict or
//     lax wildcard would assess it; enforcing either is the validator's.
//   - A wildcard particle's field, in a struct, an occurrence type or a
//     choice's branch, holds the children attributed to the wildcard: a child
//     that an element particle could take in its place, every child before it
//     attributed as it is, is the element particle's (§3.8.4.2
//     key-competing-paths, key-vpath; §3.8.6.4 cos-nonambig), which the
//     struct does not enforce. In a sequence or a choice, whose fields hold
//     their children in the order the instance does, a value holding such a
//     child in the wildcard's field is one no validation-path yields: it
//     writes either the children of another value, the validation-path's, or
//     children no validation-path accepts, as of sequence(any?, a) an Any
//     holding <a/> and A write <a/><a/>. A child no element particle can take
//     at its point is the wildcard's whatever its name: of sequence(a, b?,
//     any?), the children <a/><b/> give A and B, never A and an Any holding
//     <b/>, and <a/><b/><b/> give A, B and an Any holding the second <b/>. In
//     an all group, whose fields hold no order, the attribution is of the
//     children the value writes: of all(a, any), the children <a/><a/> give A
//     and an Any holding the second <a/>, but a value holding in the
//     wildcard's field a child that an element particle whose field is below
//     its {max occurs} would take is one no validation-path yields, as of
//     all(a?, any) an Any holding <a/> beside no A writes <a/>, which no
//     validation-path accepts.
//   - A named complex type, a top-level element through <element ref>
//     particles, or a local element of an anonymous type through <group ref>
//     particles to the group declaring it, that reaches itself through elements
//     that each occur exactly once in a sequence or all group that is a
//     {particle} occurring exactly once or in a sequence or all group spliced
//     into one, and whose types are not choices, declines: its Go type would
//     hold itself as a struct value. Such a type has no finite valid instance:
//     an instance valid against it holds another (§3.8.4.3 cvc-model-group,
//     cvc-complex-type, cvc-elt). Every recursion a finite valid instance can
//     close already passes a pointer, a slice or an interface, so the decline
//     needs no pointer break. A field holding a substitution group's
//     interface, a governing types' interface, or a nillable element's
//     Nillable, holds no struct value, so no recursion passes it; a type a
//     branch of a governing types' interface holds is checked for one from
//     that branch.
//   - {max occurs} above 1 or unbounded gives a slice whatever {min occurs} is,
//     of a list's slice too ([][]T); {min occurs} 0 with {max occurs} 1 gives a
//     pointer to a struct, atomic Go type, or list's slice (*[]T), and the bare
//     interface, a choice's, a union's, a substitution group's, a governing
//     types' or a nillable element's Nillable, never a pointer to one, nil
//     meaning absent.
//   - A sequence or all group particle, the content {particle} or nested, is
//     spliced, giving in its place the fields its own particles give and no
//     field of its own, when it occurs exactly once, or with {min occurs} 0
//     and {max occurs} 1 when one occurrence of its model group can match
//     nothing (§3.9.6.3 cos-group-emptiable, §3.8.6.5 cos-seq-range): each of
//     its particles then has a value for matching nothing, so its absence and
//     its empty match are one value, never a nil and a non-nil pointer. A
//     sequence particle with {max occurs} above 1 or unbounded one occurrence
//     of which can match nothing is flattened, as stated below, whatever its
//     {min occurs}: a slice of its occurrences would hold an empty match both
//     as no item and as items that match nothing. So is a model group
//     particle with {max occurs} above 1 or unbounded whose model group has,
//     inline or through <group ref>, a particle that repeats — its {max
//     occurs} above 1 or unbounded and other than its {min occurs} — at both
//     its edges. A particle is at both edges of a model group when it is one
//     of the group's particles and, if the group is a sequence, each other
//     particle of that sequence is emptiable (xsd.Schema.ParticleEmptiable); or
//     when it is at both edges of the model group of such a particle that does
//     not repeat. Every alternative of a choice is at both edges. A slice of
//     its occurrences would hold one child sequence under more than one
//     grouping into occurrences (§3.9.4.3 cvc-accept).
//   - A model group particle meeting one of those three gates — a choice
//     holding an emptiable model group alternative, a repeated emptiable
//     sequence, a repeated model group with a repeating edge — is flattened
//     when it is the first such particle on the walk from the content
//     {particle} inward: the walk tests a particle before the particles of its
//     model group, and the model groups inside a flattened one are its
//     reach's, each given no field, no type and no gate test of its own. A
//     flattened particle, the content {particle} or nested, inline or a
//     <group ref>, gives one field, named as if from the local name sequence
//     or choice ("Naming"), holding a slice of a sealed interface whatever the
//     particle's occurrence, a choice occurring exactly once included, nil and
//     the empty slice being one value, no child. The interface is the
//     closed-sum exception as a choice's is: one method, unexported, "is" plus
//     the interface's name, and one branch struct per basic particle
//     (Structures §2.2.3.2 key-basic-particle) the particle's model group
//     reaches, emitted after the interface in reach order, each implementing
//     the method. The reach is a depth-first walk of the model group's
//     {particles} in order, an element particle, local or an <element ref>, or
//     a wildcard particle being one basic particle, and a model group
//     particle, inline or a <group ref>, giving in its place its own model
//     group's; it never enters an element's type. Each point of that walk is a
//     branch of its own, so a <group ref> reaching one group twice, or two
//     local declarations of one name, give two branches. A model group
//     reaching no basic particle gives the interface alone, which no branch
//     implements. Inside it a particle with {max occurs} 0 declines at any
//     depth, before any branch is emitted, and a local element declaration
//     with {abstract} true at its branch.
//   - A flattened interface's branch struct holds one field, the one its basic
//     particle would give occurring exactly once as a particle of a sequence,
//     never a pointer or a slice: an element's type, through a substitution
//     group's or governing types' interface and Nillable as stated below, and
//     MixedChild in mixed content; or a wildcard's AnyElement, AnyChild in
//     mixed content. The slice holds one item per child element the particle
//     matches, in document order, each in a branch whose basic particle
//     accepts it; a nil item is not a child. The grouping of those children
//     into occurrences of the model group, and of each group inside it, is not
//     represented: the spec fixes no partition of them into occurrences
//     (§3.9.4.3 cvc-accept clause 3 and its Note). The slice also holds child
//     sequences the particle does not accept, in another order, count or
//     grouping, which the struct does not enforce. Under Unique Particle
//     Attribution a child sequence has at most one validation-path (§3.8.4.2
//     key-vpath), but a slice may hold a child in a branch other than the one
//     that path takes, as two branches whose particles both accept it can;
//     attributing each child to its particle is the validator's and the
//     codec's, as for a wildcard particle's field. The field lies behind its
//     slice, so no struct-value recursion passes through it.
//   - An all group gives the fields a sequence of the same particles gives, in
//     particle order, by the same occurrence rules, an all group particle
//     occurring exactly once in it, a <group ref> to an all group (§3.8.6.2
//     cos-all-limited clause 1.3), giving in its place the fields its own
//     particles give. The order in which its children occur in the instance is
//     not represented: each field holds, in order, the children its particle
//     accepts, and how one particle's children interleave with another's is
//     lost. Every such interleaving is valid (§3.8.4.1.3 all-mg: an all group's
//     language is the interleave of its particles').
//
// # Open content
//
//   - A complex type's {open content} (§3.4.2.3.3 dcl.ctd.ctcc.common clauses 5
//     and 6), read from its own {content type}, so one an extension inherits
//     counts (clause 4.2.3.3) and an <openContent mode="none"> adds none of its
//     own, so an extension keeps the {open content} its base's {content type}
//     carries (clause 6.1), gives one field in the struct the type itself
//     gives, never in an occurrence type or a branch struct, after the particle
//     fields and before the attribute fields, named as if from the local name
//     openContent ("Naming"), so that beside an element openContent, which
//     keeps OpenContent, the field is OpenContent2. The field is a slice of
//     AnyElement, or of AnyChild in mixed content, never a pointer, nil and the
//     empty slice being one value. Empty explicit content that clause 6.2 makes
//     element-only, with a sequence of no {particles}, gives a struct holding
//     the field alone.
//   - The open-content field holds, in order, the children attributed to the
//     open content (§3.4.4.4): a child that the {particle} could take at that
//     point in the sequence is the particle's, which the struct does not
//     enforce (§3.4.4.3 cvc-complex-content clauses 2.3 and 3.3). Each field
//     holds its own children in order, and how the open content's children
//     interleave with the particle's is not represented: under {mode}
//     interleave they may occur anywhere among them, and under suffix every
//     one follows them all (clauses 2 and 3). Neither the {mode} nor the
//     {wildcard}'s {namespace constraint} or {process contents} (§3.10.1) is
//     represented; enforcing either is the validator's.
//
// # Attributes
//
//   - Each attribute use of a type emitted as a struct, but one whose
//     {attribute declaration} is the built-in xsi:nil, gives one field after
//     the particle fields and the open-content field, or the value field, if
//     any, in the order [xsd.ComplexType.AttributeUses] returns its {attribute
//     uses} (§3.4.2.4). The field is named from the attribute declaration's
//     local name, so a qualified and an unqualified attribute of one local
//     name, or an attribute named like an element of the struct, like the
//     open-content field or like the value field, take the suffix "Naming"
//     states, the later field taking it: an attribute value beside the value
//     field gives Value2.
//   - An attribute use's {type definition}, xs:anySimpleType for a declaration
//     with no type attribute and no <simpleType> child (§3.2.2.1
//     dcl.att.global, §3.2.2.2 dcl.att.local), gives a special type's string,
//     an atomic Go type, a slice, or a union's sealed interface, as an
//     element's does; {required} true gives that type, false a pointer to it
//     (*[]T for a list), but the bare interface for a union, nil meaning
//     absent from the post-schema-validation infoset, which supplies an
//     attribute whose use has an effective value constraint (below).
//   - The use's effective value constraint (§3.5.4 key-evc), default or fixed,
//     whether the use or a global declaration it references carries it, is
//     accepted and changes neither the field's Go type nor its name. The field
//     holds the attribute of the post-schema-validation infoset, so for an
//     optional use whose declaration is not a built-in xsi: one, an absent
//     attribute's field holds the constraint's value (§3.4.4.2 key-dflt-att,
//     §3.4.5.1 sic-attrDefault). Enforcing a fixed value is the validator's.
//   - An {attribute wildcard} of a type emitted as a struct, its own, one an
//     extension inherits, or one an attribute group reference or the schema's
//     defaultAttributes brings (§3.4.2.5 dcl.ctd.anyatt), gives one field after
//     every attribute field, the struct's last, named as if from the local name
//     anyAttribute ("Naming"), so that an attribute anyAttribute gives the
//     wildcard's field AnyAttribute2.
//   - An {attribute wildcard}'s field is a slice of AnyAttribute, never a
//     pointer, nil and the empty slice being one value, and holds the element's
//     [attributes] that no attribute use's declaration names, in document
//     order, but xsi:type, xsi:nil, xsi:schemaLocation and
//     xsi:noNamespaceSchemaLocation (cvc-complex-type clause 2, a use taking
//     precedence by clause 2.1), at most one per expanded name and none named
//     like a declared use, which the struct does not enforce; AnyType's
//     Attributes, by contrast, holds the xsi: ones.
//   - Neither an {attribute wildcard}'s {namespace constraint} nor its {process
//     contents} (§3.10.1) is represented: the field holds attributes the
//     wildcard would not admit (cvc-wildcard, cvc-complex-type clause 2.2.2),
//     and holds every match raw whether a strict or lax wildcard would assess
//     it; enforcing either is the validator's.
//
// # Element declarations
//
//   - An element declaration with {nillable} true (§3.3.1 ed-nillable) and no
//     {value constraint} of {variety} fixed is nillable: a field over it holds,
//     in place of the T the rules from "Simple types" to "Element declarations"
//     give it, Nillable[T, A], a generic sealed interface, the closed-sum
//     exception as a choice's is, whose one method is unexported and takes T
//     and A, so that an arm implements it for its own type arguments alone. Its
//     arms are Nilled[T, A], holding as Attributes the attributes of a nilled
//     element, which has no character or element children (§3.3.4.3 cvc-elt
//     clause 3.2.3) and keeps its attributes (§3.4.4.2 cvc-complex-type clauses
//     2 to 4), and Content[T, A], holding T as Content for an element that is
//     not nilled. The three are emitted once per package. The field takes the
//     occurrence rules above as a sealed interface: the bare interface, nil
//     meaning absent, which a required element's field admits though no valid
//     instance gives it, as a required choice's does; a slice of it when
//     repeated; MixedChild over it in mixed content.
//   - A is struct{} when T is a simple type's Go type or a complex type's with
//     no attribute field, and otherwise T's attributes struct, one per T,
//     shared by every nillable element of that type: T's attribute fields and
//     then its {attribute wildcard} field, named as T names them, so an
//     attribute value beside the value field keeps Value2, and no value, Text,
//     particle or open-content field. Neither T nor A holds xsi:nil: an {attribute
//     wildcard}'s field excludes it, as stated above, and an attribute use
//     whose {attribute declaration} is the built-in xsi:nil (§3.2.7.2) gives no
//     field, whatever the element's {nillable}. An interface T keeps its own
//     nil inside Content, so an absent element, a nilled one and an emptiable
//     choice's empty match are three values.
//   - No field holding AnyType is wrapped: xs:anyType's AnyType holds xsi:nil
//     among its Attributes. Nor is a field holding a substitution group's
//     interface or a governing types' interface: each branch of the former
//     wraps its field when its own declaration, the head's when it has a
//     branch, is nillable, and each branch of the latter when the declaration
//     whose governing types it holds is. A nillable top-level declaration's
//     type is emitted unchanged, Nillable over it being a nilled root's value.
//     A declaration with {nillable} true and a fixed {value constraint} emits
//     as if {nillable} were false: every nilled instance of it is invalid
//     (cvc-elt clause 3.2.3.2).
//   - A top-level element declaration with {abstract} true (§3.3.1
//     ed-abstract) gets no type of its own and no branch: no element is
//     valid against it, as a root or in a content model (§3.3.4.3 cvc-elt
//     clause 2), and a field over an <element ref> particle to it holds its
//     substitution group's interface, stated above. Any other member of a
//     substitution group is emitted as every top-level declaration is, and
//     enters the interface of each head whose group holds it.
//   - An element declaration's governing types, which it is emitted from, are the types that can
//     govern a valid instance of it. Their slots are, with a {type table} (§3.12), the types the
//     table can select (§3.3.4.1 key-selected-type): the {type definition} of each of its
//     {alternatives} in order, then that of its {default type definition}, which is the declared
//     {type definition} when the last <alternative> has a test (§3.3.2.1), xs:error, which no
//     instance is valid against (§2.2.1.1 key-error, §3.16.7.3), omitted; and without one, its
//     {type definition}. A complex type with {abstract} true, which no instance is valid against
//     (§3.4.1 ctd-abstract, §3.3.4.4 cvc-type clause 2), gives in its place its members: the named
//     complex types with {abstract} false of the Schema's {type definitions}, in
//     [xsd.Schema.Types] order, ·validly substitutable· for it ([xsd.Schema.ValidlySubstitutable])
//     subject to the declaration's {disallowed substitutions} and its own {prohibited
//     substitutions}, the types an xsi:type that ·overrides· it may name (§3.3.4.2 key-overrides
//     clause 2, §3.3.4.3 cvc-elt clause 4). So a derived type that is itself abstract is no
//     member, though a type derived from it can be; an anonymous one is none; and a member need
//     not be any element's type. Each type is kept at its first slot only: two slots hold one type
//     exactly when both name a type and the names are equal, so two anonymous types are never one.
//     When one type remains, the declaration emits as if that type were its {type definition}.
//     Otherwise it gets, in place of the type it would get, a sealed interface, the closed-sum
//     exception as a choice's is: one method, unexported, "is" plus the interface's name, and one
//     branch struct per type in that order, emitted after the interface, each holding one field of
//     the type a local element of that type occurring exactly once holds, and implementing the
//     method; with no type remaining, the interface alone, whose one value is nil. A field over
//     the declaration takes the occurrence rules below; a top-level declaration's interface is
//     shared by every <element ref> particle to it, and a local one's as an anonymous complex type
//     in its place would be. The types the branches hold are emitted unchanged and carry no
//     method.
//   - Neither an alternative's {test} nor an xsi:type naming a type that is
//     not a governing type (cvc-elt clause 4) is represented: a branch holds a
//     value of its type whatever selected it, and selecting the branch an
//     instance takes is the codec's. Where a slot's type is abstract, its
//     members are the types an xsi:type may name in its place; an xsi:type
//     naming a type derived from a governing type with {abstract} false is
//     not represented.
//   - An element declaration's {value constraint}, default or fixed, and its
//     {identity-constraint definitions} are accepted and not represented. A
//     value constraint supplies a value only for an empty element and changes
//     neither its content nor the type its value is drawn from (§3.3.1). An
//     identity constraint restricts the values an instance holds and adds no
//     content or attribute (§3.11.1). So the element emits exactly as it would
//     without either, but for the fixed {value constraint} of a declaration
//     with {nillable} true, stated above, and a field over it holds the
//     post-schema-validation infoset's value, the constraint's for an empty
//     element that is not nilled (cvc-elt clause 5.1); enforcing a fixed value
//     or an identity constraint is the validator's.
//
// # Determinism
//
// The output is a function of the schema and of each document's location
// relative to the target's Dir, so two trees laid out alike under different
// absolute roots produce byte-identical output; "Naming" states where a
// header's location is not relative. Under Generate's import rule it is also
// a function of the earlier targets of the same call: their slice order, their
// Schemas, each owner's Package and import path, and which of them have
// the target's nearest go.mod. No map iteration anywhere near the emitter;
// emission order is the walk's, which document order alone fixes, as
// "Coverage" states (datatype structs, PrecisionDecimal's interface and arms
// with it, raw-content structs, MixedChild, Nillable with its arms, attributes
// structs, and union interfaces with their variants last, in the order the
// walk first needs each). Golden-file tests pin the output.
//
// # Naming
//
// A single namer component owns every XSD-name → Go-identifier decision. Each
// XSD local name becomes an exported identifier: '_' and every rune Go does
// not admit in an identifier break words, every word starts upper-case, and a
// result with no upper-case first letter is prefixed with X. That upper-case
// start is also what keeps every identifier clear of the Go keywords. A
// top-level element's type is named from the element; a named complex type a
// local element reaches, and a named union's interface, from the type; a
// datatype struct, PrecisionDecimal included, from its datatype; the
// raw-content structs, MixedChild, and Nillable and its arms Nilled and
// Content, from fixed local names; and every other type, such as an anonymous
// type, an occurrence type, a branch, a variant, or PrecisionDecimal's
// interface and arms, by a path: a name it lies under, followed by local names
// of its own. A residual collision — two names folding to one identifier,
// among a package's types or one struct's fields — is disambiguated
// deterministically by document order: the later name takes the smallest free
// numeric suffix from 2, so a top-level element ct and a complex type ct
// reached locally take Ct in the order they are first reached.
//
// A type named by a path from a declaration — an anonymous complex type, an
// occurrence type and a flattened group's interface, a local declaration's
// governing types' interface, and an anonymous union but one under a variant —
// takes the shortest suffix of that path that starts at a declared step and is
// unique, and the whole path when no shorter suffix is. A suffix is a run of
// whole steps, each one local name compared as its identifier, so shipTo,
// ShipTo and ship_to are one step. A declared step is the path's root, an
// element, an attribute or its owner, or a named list; a generated word —
// sequence, choice or all, a {type table} branch's step such as alternative2Type
// or defaultType, value, union — starts no suffix. A suffix is unique when no
// other type's whole path ends with it and its identifier is no fixed name's:
// the whole path of a branch, a variant, an attributes struct or an arm is the
// whole path of the name it extends followed by its own local names, and a fixed
// name is one named from an element, a named type or a datatype,
// PrecisionDecimal's interface, a substitution group's interface, or the fixed
// local names above. Uniqueness is measured over every type name the package
// claims, the whole composed Schema's, so a declaration in one document can
// lengthen the name of a type declared in another. Each such type takes the
// suffix of its own whole path, whatever the type it lies under is named
// (element address under element shipTo under element purchaseOrder → Address,
// when no other path ends in address). A branch, a variant, an attributes
// struct, an arm, and an anonymous union under a variant are never shortened:
// each extends the final name of what it extends (OrderLine shortened to Line
// gives the branch LineAlternative2; PriceAttributes follows Price). Shortening
// runs first, and the numeric suffix then runs over the shortened names in
// claim order. Under element order, local elements address, shipTo and billTo,
// and an element address in each of the last two, all of anonymous types, give
// OrderAddress, ShipTo, ShipToAddress, BillTo and BillToAddress, three paths
// ending in address, and a choice in shipTo's type gives ShipToChoice. Each
// arrow below to a path-named type, written → path, gives its whole path.
//
// A package that imports a document's types from an earlier target (Generate)
// claims their names as if it defined them, so its uniqueness is measured over
// the same names, imported ones included. Each alias carries its owner's name,
// which agreement (Generate) makes the name the importer's own composition
// gives it. The fields of an imported type name its owner's types, never the
// importer's.
//
// Generated identifiers are not stable before v1.0: a release may rename any
// type or field this section names.
//
// A top-level element's type is named from the element, its name claimed when the walk first
// reaches the element: at its own position, at the first <element ref> particle to it met before
// that, or at the branch of the first substitution-group interface holding it. A named complex
// type a local element reaches is named from the type (complex type ct → Ct), whichever local
// elements have type ct. An anonymous type nested in either is named by the path of names from
// the named complex type that declares its element, when one does (element x in complex type ct →
// path CtX, whichever elements have type ct or a type extending ct, which reaches it through ct's
// own particle), and otherwise from its nearest named ancestor: the top-level element when the
// element's own type is anonymous (element shipTo under element purchaseOrder → path
// PurchaseOrderShipTo). The original of a complex type a <redefine> redefines has no name (§4.2.4
// src-expredef clause 1.1), so the types of the local elements it declares are named from the
// nearest named ancestor: its element x → path TX for elements of type t, path UX for a type u
// extending t. An anonymous type of a local element a model group definition declares, and every
// type nested in it, is named by the path of names from that group (element e in group g → path
// GE), whichever <group ref> particles reach it. Each such name, a named complex type's or a
// nested anonymous type's, is claimed when the walk first reaches a field holding its type, ahead
// of the names that type's own walk claims (beside a top-level element x, element x in complex
// type ct, holding an <element ref> to a later top-level element ctX → path CtX, and CtX2 for
// ctX). The type for one occurrence of a model group particle is named by the path of names of
// the complex type whose content model holds it — Ct for a named complex type ct, the path an
// anonymous one is named by above, GE for the anonymous type of element e in group g — never from
// a name its interface or a branch claimed, followed by the local name sequence, choice or all of
// each occurrence type enclosing it and then its own (path CtSequence; a choice in that sequence
// → path CtSequenceChoice; a repeated sequence in a branch of named choice type alt, reached by
// element fifth → path AltSequence, never FifthSequenceSequence), and the types nested in its
// particles as if it were absent; it is named and keyed under that complex type whichever <group
// ref> particle brought its group, so two complex types referencing one group get an occurrence
// type each. Its name is claimed when the walk first reaches its field, ahead of the names its
// particles claim (the repeated sequence of complex type rows, holding an <element ref> to a
// later top-level element rowsSequence → path RowsSequence, and RowsSequence2 for rowsSequence).
// A flattened model group particle's sealed interface takes the name its occurrence type would
// take, claimed where that name would be, and is shared as it would be (the repeated sequence
// content particle of element many's anonymous type → path ManySequence); each of its branch
// structs is named from the interface's name plus the basic particle's word: the element
// declaration's local name, the referenced declaration's for an <element ref>, or any for a
// wildcard, the model groups between contributing none, so that two basic particles of one word
// take the numeric suffix in reach order (an unqualified and a qualified element x of
// ManySequence → ManySequenceX, ManySequenceX2). The branches' names are claimed right after the
// interface's, in reach order, each followed by the names its field's type first claims, and the
// types nested in their elements are named as if the flattened group were absent. A choice's
// branch struct is named from its interface's name plus the alternative's local name, its
// compositor for a model group alternative, or any for a wildcard alternative (alternative a of
// interface First → FirstA, a sequence alternative → FirstSequence, a wildcard alternative →
// FirstAny), so two top-level elements of one named choice type get distinct interfaces and
// branches.
//
// A substitution group's interface is named from its head's local name plus
// SubstitutionGroup (head h → HSubstitutionGroup), and each branch from the
// interface's name plus its declaration's local name (member m1 →
// HSubstitutionGroupM1); the interface's name is claimed at the first <element
// ref> particle to the head, then each branch's in group order, each followed,
// when the walk has not reached its declaration yet, by the names that
// declaration's type claims as at an <element ref> particle to it. A governing
// types' interface, a {type table}'s or an abstract complex type's alike, takes
// the name the declaration's own type would take, claimed where that name
// would be: from the element for a top-level declaration, and by the path an
// anonymous complex type of a local one would take (element line in complex
// type order → path OrderLine). Each of its branches is named from the interface's
// name plus the branch's local name: the governing type's local name when it
// is named, a member of an abstract type included (member circle of element
// shape → ShapeCircle), and otherwise alternative and the 1-based position of
// its alternative among the {alternatives}, or default for the {default type
// definition} (a type book, and an anonymous fourth alternative, of interface
// Product → ProductBook and ProductAlternative4; an anonymous default of Memo →
// MemoDefault). An anonymous type a branch holds is named by the interface's
// path followed by the branch's local name and the local name type, every type
// nested in it extending that path, and an anonymous union with union last
// (paths ProductAlternative4Type, OrderLineAlternative2TypeNote,
// UAlternative1TypeUnion). The branches' names are claimed after the
// interface's, at a top-level declaration's own position, in branch order, each
// followed by the names its field's type first claims. Each choice branch's
// name is claimed when the walk reaches its alternative, after the types every
// earlier alternative first needs and ahead of its own, so the raw-content
// structs a wildcard alternative first needs follow its branch and every
// earlier one and precede every later one: alternatives type, a wildcard and
// element of interface Any name the branches AnyType, AnyAny and AnyElement2,
// and the raw-content structs AnyType2, AnyAttribute, AnyChild and AnyElement.
//
// A datatype struct is named from its datatype's local name (dateTime →
// DateTime, gYearMonth → GYearMonth, dayTimeDuration → DayTimeDuration, QName
// → QName, which an xs:NOTATION type shares), claimed when the walk first
// needs it: for a top-level element of a simple type, ahead of that element's
// own type, so that a top-level element date of type xs:date names its type
// Date2; for a field of a struct, the value field and attribute fields
// included, after that struct's own name, so that a top-level element date
// whose complex type extends xs:date names its type Date and the datatype
// struct Date2. PrecisionDecimal's interface is named from the path
// precisionDecimal, value, and each arm from the interface's name plus
// numerical, positiveInfinity, negativeInfinity or notANumber
// (PrecisionDecimalValueNotANumber), the five claimed in that order right
// after PrecisionDecimal's own name. The raw-content structs are named
// AnyType, AnyAttribute, AnyChild and AnyElement, claimed together in that
// order when the walk first reaches a declaration of type xs:anyType or a
// wildcard particle, ahead of that declaration's own type, so that a top-level
// element anyType with no type names its type AnyType2; or when it first
// reaches the {open content} of a type emitted as a struct, after that
// struct's own name and the types its particle fields first need, or its
// {attribute wildcard}, after the types its other fields first need, so that a
// top-level element anyType whose anonymous complex type has an <openContent>
// or an <anyAttribute> names its type AnyType and the raw-content structs
// AnyType2, AnyAttribute, AnyChild and AnyElement. A wildcard particle's field
// is named as if from local name any, a model group particle's, holding its
// occurrence type or flattened, as if from its compositor's local name,
// sequence, choice or all, an {open content}'s as if from local name
// openContent, and an {attribute wildcard}'s as if from local name
// anyAttribute. Mixed content's Text field is named as if from local name
// text, ahead of the particle fields, so that an element text in it gives the
// field Text2. The generic struct MixedChild is named from local name
// mixedChild, claimed when the walk first needs it as a datatype struct is:
// for an element field of mixed content, after that struct's own name and
// ahead of the names the field's type first claims, so that an <element ref>
// particle there to a top-level element mixedChild that the walk has not
// reached names the generic struct MixedChild and that element's type
// MixedChild2. The generic sealed interface Nillable and its arms Nilled and
// Content are named from the local names nillable, nilled and content, claimed
// together in that order when the walk first needs them: for a field over a
// nillable element, after the names the field's type first claims, so after
// MixedChild's in mixed content; and at a nillable top-level declaration's own
// position, after the names its type claims. A complex type's attributes
// struct is named from that type's Go name followed by the local name
// attributes (Price → PriceAttributes), claimed right after Nillable's names
// would be at the same point.
//
// A union's interface is named from the type when it is named (simple type
// size → Size), claimed when the walk first needs it as a datatype struct is:
// for a top-level element of a union type, ahead of that element's own type,
// so that a top-level element size of type size names its type Size2; for a
// field of a struct, after that struct's own name. An anonymous union is named
// by the path of names an anonymous complex type would take in its place, with
// the local name union last: a top-level element's (element pin → path PinUnion), a
// local element's (element code in complex type box → path BoxCodeUnion), an
// attribute's from the global attribute declaration, the named complex type or
// attribute group definition declaring it, or the path of the struct holding
// it for an anonymous complex type (attribute mode in attribute group shared →
// path SharedModeUnion, attribute kind of element tag → path TagKindUnion), and a value
// field's from its struct's path with value before union (complex type narrow
// → path NarrowValueUnion). A list's item is named from the list whose <list>
// declares it: from that list's local name when it is named (simple type codes
// → path CodesUnion, for codes and every restriction of it), and from the path the
// list takes otherwise. An anonymous union's interface but one under a variant,
// a list's item included, is claimed as a named union's is, a struct's fields
// claiming in field order, wherever its <simpleType> lies: attributes kind and Kind of
// element root, each of an anonymous union → paths RootKindUnion and RootKindUnion2;
// the item of list codes, declared first in the document, claimed at a top-level
// element of type codes that follows a top-level element codesUnion → path CodesUnion2.
// A variant is named from its interface's name plus its member's local name: an
// anonymous list member's is list, and any other anonymous member's is that of the
// nearest named type on its {base type definition} chain, the member itself first
// (xs:int in Size → SizeInt, an anonymous restriction of xs:string → SizeString). The
// variants are claimed after their interface in basic-member order, each ahead of its
// member's own Go type, which takes the variant's name as its path (an anonymous union
// item of the list variant MixedList → MixedListUnion).
//
// Every other generated type's header comment but a union interface's or
// variant's, or MixedChild's, Nillable's, Nilled's or Content's, records the
// QName and the schema Loc of the declaration it is named from: the element
// declaration, or the complex type definition for a type named from one, an
// attributes struct's being its type's, followed by a second line naming that
// type. A union interface's header records the simple type it is generated
// from, by its QName or as (anonymous), and that type's schema Loc, and a
// variant's header its member the same way, a builtin member, having no schema
// document, by its QName alone. A datatype struct's header records its
// datatype's QName alone, a builtin having no schema document, then a second
// line stating what its fields' names do not, when there is any:
// TimezoneOffset's unit and whether it may be absent, that duration's Months
// and Seconds share one sign, or that QName's Space is "" for no namespace;
// PrecisionDecimal's interface and arms record the same QName, the numerical
// arm then a second line stating its value's formula and that Coefficient is
// never negative. A raw-content struct's header records xs:anyType's QName
// alone, then, for AnyType and AnyChild, a second line stating what Text or
// Tail holds. MixedChild's header records no declaration, since every mixed
// content shares it, and states that it holds one child element and the
// characters after it, then a second line stating what Tail holds. Nillable's,
// Nilled's and Content's headers record no declaration, since every nillable
// element shares them, and state what each holds.
//
// A Loc whose URI filepath.IsAbs accepts renders relative to the target's Dir
// made absolute against the working directory, slash-separated and climbing
// with ".." out of Dir. A document outside the tree laid out alike, such as
// one an absolute schemaLocation names, renders a climbing path that differs
// with that tree's depth below the filesystem root. Any other URI renders as
// is, a URL or a relative loader key alike, as does one with no path relative
// to Dir, such as one on another Windows volume. Symbolic links are not
// resolved.
//
// # Planned (M10 — not yet implemented)
//
// Generated readers/views expose the narrowest interface a consumer needs.
//
// # The Emitter seam (value.Emitter; M10 — not yet implemented)
//
// Backends contribute specialized decode/encode source for their value
// representations — parsing straight from the reader's byte window into
// the target field, facet checks inlined, no intermediate string, no
// boxed value. A backend without an Emitter falls back to emitting calls
// into codec's runtime path. Emitted fast paths carry the same pipeline
// stage and rule-ID metadata as the runtime path; codec's differential
// tests hold the two to identical behavior.
package codegen
