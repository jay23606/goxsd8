package codec

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/kud360/goxsd8/internal/goname"
	"github.com/kud360/goxsd8/internal/goshape"
	"github.com/kud360/goxsd8/parser/xmltree"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// ErrTypeMismatch is the error [Decode] wraps when T does not match the type
// codegen emits for the document element's declaration; Decode's doc, item 6,
// states when.
var ErrTypeMismatch = errors.New("codec: T is not the type codegen emits for the document element")

// Decode decodes the XML instance data into a new value of T, the type
// package codegen emits for the declaration of data's document element, and
// returns it. A value is returned exactly when the error is nil.
//
// T is the caller's choice: nothing dispatches a document element to its
// generated type, and item 6 below states which T match. A caller decoding
// documents of several roots therefore reads the document element's name
// itself, through parser/xmltree or encoding/xml, before choosing T.
//
// The slice Decode covers is the package doc's "# What decodes". Decode first
// assesses data against schema with package validate and decodes only an
// instance whose Result has no source fault and stands
// validate.NotProvenInvalid; it then reads data a second time with
// parser/xmltree. Each decoded value, a field's or, for a document element of
// simple type, T's own, is read from the item's ·normalized value·
// (Structures §3.1.4 key-nv) under its declaration's simple type, for the
// value field of a complex type with simple content the {content type}'s
// {simple type definition} (§3.4.4.2 cvc-complex-type clause 1.2): the string
// [value.ApplyWhiteSpace] returns for the item's characters, an attribute's
// as XML 1.0 §3.3.3 normalized them, an element's the concatenation of its
// character children. Where the post-schema-validation infoset supplies a
// value it is read from that value, the {lexical form} of a value constraint
// under the same normalization: for an absent attribute whose use has an
// effective value constraint (§3.5.4 key-evc; §3.4.4.2 key-dflt-att, §3.4.5.1
// sic-attrDefault), and for an element with neither element nor character
// children whose declaration has a {value constraint} (§3.3.4.3 cvc-elt
// clause 5.1, §3.3.5.4 sic-eltType), of simple type or with simple content,
// whose value field holds it. For a simple type whose atomic Go type
// is string the value is that string. For every other it is the value
// backend's governing mapping of the type ([value.GoverningMapping]) maps
// that string to, through Parse with the item's namespace bindings in scope
// (Datatypes §4.1.4 cvc-datatype-valid clause 2.1): an element's own for its
// content, its owner element's for an attribute, and for a supplied value
// those in scope at the schema document element that wrote the value
// constraint ([value.ConstraintContext]), never the instance's; read back
// through that mapping's Canonical and converted to the Go type: a boolean's
// canonical form exactly "true" or "false" (§E.4 f-booleanCanmap), a float's
// and a double's by strconv.ParseFloat at the Go type's width, a decimal's
// into big.Rat, big.Int or one of the bounded Go integers int64, int32,
// int16, int8, uint64, uint32, uint16 and uint8 at its width, a hexBinary's
// into []byte by encoding/hex, either letter case read (§E.4.1
// f-hexBinaryMap), a base64Binary's into []byte by encoding/base64's
// StdEncoding (§3.3.16.2), and a date, time or duration type's into its
// datatype struct, by that datatype's lexical grammar (§3.3.6.2,
// §3.3.7.2–§3.3.14.2, §D.2.2), each property the local value the form shows,
// never converted to UTC (§D.2.1), the timezoneOffset in minutes, and a
// duration's Months and Seconds both of the form's one sign (§3.3.6.1, §E.2).
// A type whose datatype struct is yearMonthDuration's alone is converted from
// the string Parse accepted, so that zero, which has no canonical
// representation (§3.4.26.1), decodes as Months 0; its Canonical is never
// consulted. A type whose datatype struct is QName's, an xs:QName or
// xs:NOTATION type, neither of which has a canonical representation (§3.3.18,
// §3.3.19), is the expanded name the string Parse accepted resolves to
// against the same bindings Parse was given, Space its namespace name, "" for
// none, and Local its local part, no prefix kept; its Canonical is never
// consulted either. A type whose datatype struct is PrecisionDecimal's, an
// xs:precisionDecimal type, is converted from the string Parse accepted by
// ·precisionDecimalLexicalMap· (precisionDecimal §6 f-precDecLexmap), since
// its canonical mapping keeps neither the ·scale· of zero nor its ·sign·
// (f-precDecCanmap); its Canonical is never consulted. Its Value holds the
// arm [WithPrecisionDecimalArms] supplies for the value's ·numericalValue·
// (§3.1 vp-pd-numVal): INF and +INF the positive infinity's, -INF the
// negative infinity's and NaN the not-a-number's, each a struct of no field;
// and a numeral the numerical arm, its Coefficient the numeral's digits read
// as one decimal integer, the value's magnitude times 10^Scale, its Scale the
// numeral's ·scale· (vp-pd-precision), its digits after the '.' less its
// exponent, and its Negative true exactly when the numeral opens with '-'
// (vp-pd-sign). So 3 and 3.00 decode to Scale 0 and 2, 0 and -0 to Negative
// false and true, and 3E2 to Coefficient 3 and Scale -2. A float or double
// literal beyond the type's range is the INF or -INF its mapping rounds it to
// (·floatingPointRound·, Datatypes §E.1), so it decodes as ±Inf where the
// backend's Canonical renders that, as builtin/strict's does, and is no item
// 8 error. backend's governing mapping of each such type but those three
// kinds must therefore have a non-nil Canonical, as builtin/strict's do; item
// 8 states the error where one fails. For a list type, whose whiteSpace is
// collapse (Datatypes §4.3.6, §2.4.1.2), the normalized string is split at
// each U+0020 into its list items before any is mapped, an empty one into
// none (§4.1.1 dt-list), and each item is read as above under the list's
// {item type definition}, a restriction of a list taking its base's
// (Structures §3.16.2.3); the value is the []A of the items in order. For a
// union type, which has no whiteSpace of its own, so that its normalized
// string is the characters themselves (Datatypes §4.3.6), the value is the
// variant of its ·active basic member· (dt-active-member,
// dt-active-basic-member): the member [value.ValidatingType] identifies on
// that string, under the same bindings as Parse, the first member in order
// that accepts it, descended through a member that is itself a union, a
// {lexical form} included for a supplied value. The variant is the type
// [WithBranches] supplies for the union's interface whose Go type is that
// member's (item 6), and it holds the member's value read as above, from the
// string normalized by the member's own whiteSpace (cvc-datatype-valid clause
// 2.3, the note on its pre-lexical facets), under the member as a list or an
// atomic type. A list of a union is []I, I the item's interface, each list
// item's member identified on the item. So the value is the instance's own
// even where a fixed value constraint holds it to an equal one that is not
// identical (§3.5.4 cvc-au, cvc-elt clause 5.2.2.2.2): -0 against fixed 0 on
// xs:double decodes to negative zero. The field of an optional attribute use
// with an effective value constraint is the Go type codegen gives its simple
// type itself, as codegen emits it, holding the supplied value when the
// attribute is absent; a whitespace-only element has a character child and
// holds its own value; and an element absent from the instance supplies
// nothing, its field nil or not appended to. An element of type xs:anyType
// holds no such value: its field holds its content raw (item 6), and its
// {value constraint} supplies none, since its {content type} is mixed and it
// has no [schema normalized value] (§3.3.5.4 sic-eltType), so an empty one's
// Text is "" whatever its default or fixed value.
//
// Every instance Loc an error of Decode carries — a source fault's, a
// violation's, an undecided check's, a decline's and item 8's — names the
// document [WithURI] gives; without it, or with an empty uri, the URI
// renders "?".
//
// The first of these that applies is returned:
//
//  1. A nil schema: validate.New's plain error. A nil backend panics, on
//     validate.New's grounds. backend must be the one schema was compiled
//     with, as validate.New states.
//  2. A source fault, in either channel validate/xmlsrc.Validate carries one
//     in, whatever the Result's Standing: that error as it is, an
//     *xsderr.Error with rule xml-wf that xsderr.RuleOf reaches.
//  3. A validate.Charged Result: an error wrapping every violation, each an
//     *xsderr.Error, in the order Result.Violations lists them. errors.As to
//     an *xsderr.Error, and xsderr.RuleOf, reach the first violation alone.
//     [ViolationsOf] returns every one. Decode never succeeds where validate
//     does not accept.
//  4. A validate.Undecided Result: an [*UndecidedError] whose Unevaluated is
//     the first validate.Unevaluated. It wraps errors.ErrUnsupported, as a
//     *DeclineError does, so errors.Is does not tell the two apart; errors.As
//     does.
//  5. A construct outside the slice of the document element's declaration or
//     type, or of a declaration or type its particles reach at any depth: a
//     *DeclineError at the document element's instance Loc, whose
//     [DeclineError.Construct] is that construct's, the first in the order
//     the list "Item 5 examines" below states. A component of a declaration
//     or type this item examines that does not resolve, or a particle with
//     {max occurs} 0, is a plain error in place of a decline, at its point in
//     that order.
//  6. T does not match the type codegen emits for the declaration, as Decode
//     derives it from the schema by the comparisons the list "Item 6
//     compares" below states: an error errors.Is reports as
//     [ErrTypeMismatch], no *DeclineError, so that a T that does not match
//     the schema is never filled.
//  7. An element carrying xsi:type, the document element, any descendant
//     outside a wildcard's match, and any element outside an element of type
//     xs:anyType's content, the element of type xs:anyType itself among
//     them: a *DeclineError at that element's instance Loc, the first in
//     document order, whose [DeclineError.Construct] is xsi:type's. Inside
//     a wildcard's match and inside an element of type xs:anyType, xsi:type
//     is one of an element's raw attributes (item 6). A document element no top-level
//     declaration declares, which validation accepts only through its
//     xsi:type (§3.3.4.6 cvc-assess-elt), declines so in place of item 5.
//  8. A union's identification of its ·active basic member·, the lexical
//     mapping or the conversion to the Go type failing on a value validate
//     accepted on the same backend: value.ValidatingType returns an error for
//     the union and the value, or a type that is no basic member of the
//     union, under codec's bindings; or no mapping of backend governs
//     the type, its Parse rejects the normalized value, or its Canonical is
//     nil, fails, or renders a form the Go type does not read or a finite
//     form beyond the Go type's range, or, for yearMonthDuration's datatype
//     struct and PrecisionDecimal's, which read no Canonical, Parse accepts
//     a form that struct does not read, PrecisionDecimal's reading the
//     pDecimalRep production alone (precisionDecimal §3.2), or, for QName's,
//     which reads none either, Parse accepts a lexical that resolves to no
//     expanded name against the bindings it was given; a datatype struct
//     reads no form denoting no value of its datatype, such as a month of 13,
//     an hour of 24 or a dateTimeStamp without a timezone. For a list, the
//     type is its {item type definition} and the value one list item; for a
//     union's other stages, the type is its ·active basic member·. It is a
//     fault of the backend, not of the instance. The error names the stage,
//     the type — its QName, or for an anonymous type the declaration it is
//     local to and that declaration's schema Loc, and for a list's item type
//     the list's too, and for a union's basic member the union's too — the
//     normalized value, for an identification the characters it was made on,
//     or the list item and its 1-based position among the value's items, and
//     the value's source at its instance Loc, for a list the whole value's and
//     never an item's: an element's character content, at its first character
//     data, with that data's byte offset; a present attribute, at its Loc
//     (xmltree.Attribute.Loc), with no offset; or a supplied value, at its
//     element's start, with the declaration whose value constraint supplied
//     it. It wraps nothing: errors.As finds no *xsderr.Error, *DeclineError or
//     *UndecidedError in it, xsderr.RuleOf no rule, and errors.Is neither
//     errors.ErrUnsupported nor [ErrTypeMismatch]; and [ViolationsOf] returns
//     nil for it.
//
// Items 7 and 8 are both found while filling T, and the first in document
// order is returned: an element's xsi:type, then its attributes' values,
// present attributes in start-tag order and then the supplied values of
// absent ones in attribute-use order, then, for simple content, its value
// field's value, or else its children in document order,
// each child of complex type filled in this same order before the next
// child, and a child attributed to a wildcard particle, and an element of
// type xs:anyType past its own xsi:type, read raw, which finds neither. A
// supplied value whose attribute is present is never mapped.
//
// Each child element fills the field of the particle validation attributes
// it to (§3.4.4.4 key-att-to): of a content model with no wildcard particle,
// the particle of its expanded name; of one with a wildcard particle, the one
// cvc-accept attributes it to, an element particle taking a child before the
// wildcard does (§3.8.6.4 cos-nonambig), a substitution group's member
// filling its head's particle, so of sequence(a, b?, any?) the children
// <a/><b/><b/> give A, B and an E holding the second <b/>.
//
// Items 5 and 6 read no instance; [CheckDecode] returns them for a
// declaration before an instance arrives.
//
// Item 5 examines the document element's declaration in this order; the
// fourth and fifth bullets say how the particle step reads a substitution
// group's <element ref> and a choice's alternatives:
//
//   - The declaration's {type table} and {nillable}.
//   - Its {type definition}: a simple type's {variety} and {primitive type
//     definition}, for a list its {item type definition}'s, whose construct
//     the list declines by, after which nothing further is examined; for a
//     union, or a list's union item, each basic member in codegen's
//     basic-member order examined as an atomic or list type is, the first
//     declining member's construct the union's, then a basic member whose Go
//     type an earlier one's is, as item 6 compares a variant's, two lists of
//     a union or the same A or []A, such as xs:string's and xs:token's, which
//     declines as a union two of whose basic members get one Go type, then an
//     assertions facet effective on the union, on any of its {member type
//     definitions} at any depth, an intervening union's included, or on a
//     list member's {item type definition} or what that reaches, which
//     declines as a union with an assertions facet, since Decode identifies
//     the ·active basic member· deciding no {test}; or a complex type's
//     {abstract}, then its content, then, for simple content, its {simple
//     type definition}, examined as the document element's simple type is,
//     whose construct the content declines by, or, for element-only content,
//     its {particle}'s model group, an all group declining whatever it occurs,
//     and then the occurrence of its sequence or choice: of a <group ref>,
//     the compositor is the referenced definition's {model group}'s and the
//     occurrence the referring particle's (§3.7.2 declare-namedModelGroup).
//   - Each particle of the complex type's content in order, the whole of one
//     before the next. A nested model group declines first, and so does a
//     wildcard particle after the content model's first, which gives the
//     field item 6 states and is examined no further. Then the particle's
//     element declaration's {type table} and {nillable}, and its {type
//     definition}: a simple one is examined as the document element's is;
//     xs:anyType itself, known by its QName, gives the field item 6 states and
//     is examined no further; and any other complex one, a type derived from
//     xs:anyType included, is examined as the document element's complex type
//     is, from its {abstract} to its {attribute wildcard}. Then the particle
//     declines by an expanded name an earlier particle's field takes, a
//     particle's field taking its element's expanded name, or, for a
//     substitution group's interface, each declaration the next bullet
//     examines.
//   - Of an <element ref> particle whose field holds a substitution group's
//     sealed interface (item 6), in place of the referenced declaration's own
//     properties and type: each declaration of the group with {abstract}
//     false in group order, the head among them only where it has {abstract}
//     false, examined as a particle's element declaration is, each then
//     declining by a branch field of an earlier one's shape as a choice's
//     alternatives do, and then the first of their expanded names in group
//     order an earlier particle's field takes.
//   - Of a choice, each alternative's own constructs, then a branch field of
//     the name an earlier alternative's has where either alternative is of a
//     simple type whose Go type is a datatype struct or a list of one, where
//     one alternative is codegen's struct and the other a wildcard, of type
//     xs:anyType, or of a simple type whose atomic Go type is big.Rat or
//     big.Int, where both alternatives are of type xs:anyType, occur alike,
//     and both hold X or both an N over X (item 6), or where both
//     alternatives are of another complex type or hold a substitution group's
//     interface, occur alike, and are both a struct or both a sealed
//     interface, and otherwise of the name and Go type, as item 6 reads it,
//     which declines as two branches of a sealed interface whose structs
//     have one shape.
//   - Each of the complex type's attribute uses in order, and its {attribute
//     wildcard}.
//
// The complex type of a top-level declaration, the document element's, one
// an <element ref> names, or a member of that one's substitution group, a
// local declaration's named complex {type definition}, and the anonymous
// complex type of a local declaration in a model group definition, which a
// <group ref> reaches, are each examined at the first reach of that
// declaration or that name only, and contribute nothing when reached again,
// so a type that reaches itself is examined once. A complex type with simple
// content, which reaches no particle, is examined at each reach, its value
// field holding the reaching declaration's {value constraint}; no decline
// differs, since one examination's answer is every other's.
//
// Item 6 compares T with the type codegen emits by shape alone: neither T's,
// a sealed interface's, a branch's nor a variant's name is compared. Decode
// never reads the fields from T, so for a type codegen emitted from this
// schema an item 6 error is a defect of Decode's, or a branch or variant set
// [WithBranches] or an arm set [WithPrecisionDecimalArms] did not supply as
// codegen emitted it, and for any other T it is a T that does not match.
// Item 6 compares:
//
//   - T of a complex {type definition}: T is a struct whose fields are
//     exactly those codegen emits, in name, order and Go type, each field's
//     Go type as the bullets below read it, for simple content the value
//     field first, named from the local name value, holding V, the Go type
//     codegen gives the {content type}'s {simple type definition}, matched as
//     a local particle's V of simple type is, never a pointer, and then the
//     attribute fields, or, where codegen emits the choice's sealed interface
//     as the type itself, T is an interface, and Decode then returns a *T
//     pointing at it. So a T of the same shape, such as the struct codegen
//     emits for another top-level element of the same type, passes and
//     decodes as the declaration's own struct does, an error only where that
//     one's is.
//   - T of a simple {type definition}: T's underlying type alone is the Go
//     type codegen gives it (the Simple types section of codegen's package
//     doc): its atomic Go type A, one of string, bool, float32, float64,
//     big.Rat, big.Int, int64, int32, int16, int8, uint64, uint32, uint16,
//     uint8, []byte and a datatype struct D, a date, time or duration
//     datatype's, QName's, which an xs:NOTATION type shares, or
//     PrecisionDecimal's, or for a union its sealed interface I, or for a
//     list []A or []I, A or I its {item type definition}'s. So any T over
//     that Go type passes, such as the type codegen emits for another
//     document element of the same simple type. Codegen declares D and I in
//     the caller's package, so each is matched by shape: D a struct of
//     exactly D's exported fields, in name, order and Go type, struct tags
//     aside, whatever its name, PrecisionDecimal's exactly one field Value,
//     not embedded, whose type is an interface of one unexported method, and
//     for a list a slice, named or not, of such a struct; I any interface of
//     one unexported method, whatever its name, so the named type codegen
//     emits over I for a document element passes, held to I's variants as the
//     union bullet below states, and for a list a slice, named or not, of
//     such an interface.
//   - The field of a particle or attribute use of simple type: for a local
//     element declaration's particle and an attribute use it is V, *V or []V
//     by its occurrence, an attribute's by its use's {required} and its
//     effective value constraint (§3.5.4 key-evc), V the Go type codegen
//     gives that simple type, A or []A; for an <element ref> particle it is
//     N, *N or []N, N the named type codegen emits for the referenced
//     declaration, matched as any type other than V whose underlying type is
//     V, never by name; V itself, a local particle's type, does not pass, so
//     a ref's field of simple type and a local one's never accept one Go type,
//     except where V is a datatype struct D or a union's interface I: V and N
//     are then each a struct of D's shape, or each any interface of one
//     unexported method, as T is, so either field accepts both. For a list of
//     D or of a union a local field's V is an unnamed slice of such a struct
//     or interface and a ref's N a defined slice type over one. A union's
//     optional field, a particle's or an attribute use's, is the bare
//     interface, never a pointer, nil meaning absent.
//   - The field of a particle whose element declaration, local or
//     referenced, has a complex {type definition} other than xs:anyType
//     itself: S, *S or []S by its occurrence, S a struct held to that type
//     as T is to the document element's, its fields exactly codegen's and
//     its name never compared, or, where codegen emits the type's choice's
//     sealed interface as the type itself, I or []I, I that interface, held
//     as T is, the bare interface for an optional one and never a pointer,
//     nil meaning absent.
//   - The field of a wildcard particle, named as if from local name any
//     (codegen's package doc, "# Naming"), and a choice's wildcard
//     alternative's branch field: E, *E or []E by its occurrence, a choice's
//     {min occurs} 0 read as 1, E a struct of the shape of codegen's
//     AnyElement, which codegen declares in the caller's package, so E is
//     matched by shape, every struct's name never compared, whatever codegen
//     numbers it, such as AnyElement2 or AnyType2: E's fields exactly Space
//     and Local, each a string, and Content, X, in that order; X's, AnyType's,
//     exactly Attributes, a slice of A, Text, a string, and Children, a slice
//     of C; A's, AnyAttribute's, exactly Space, Local and Value, each a
//     string; and C's, AnyChild's, exactly Element and Tail, a string, where
//     Element is of E itself, not another struct of E's shape. Fields are
//     matched in name, order and Go type, none embedded, struct tags aside,
//     and a slice named or not. Each child attributed to the wildcard (the
//     paragraph after item 8) is one E, holding its expanded name and its content
//     raw: Attributes its attributes in start-tag order, xsi: ones included
//     and namespace declarations not, each value as the XML reader normalized
//     it, any prefix in it unresolved; Text the character data before its
//     first child element; and Children each child element, read the same
//     way, as Element, and as Tail the character data after it, before the
//     next child or the end, each run of character data concatenated across
//     the comments and processing instructions that break it. Mixed content
//     declines (item 5), so no field holds AnyChild itself.
//   - The field of a particle whose element declaration has {type definition}
//     xs:anyType itself (Structures §3.4.7 any-type-itself), and a choice's
//     such alternative's branch field, a type derived from xs:anyType being
//     examined by the complex-particle bullet above: for a local element
//     declaration's particle, X, *X or []X by its occurrence, a choice's {min
//     occurs} 0 read as 1, X a struct of the shape the bullet above states
//     for AnyType, matched through E, the Go type of its Children's Element;
//     for an <element ref> particle, N, *N or []N, N the named type codegen
//     emits for the referenced declaration over X, matched as the simple-type
//     bullet above matches a ref's N over V, X itself not passing. The field
//     holds the element's content raw, as the bullet above states an E's
//     Content, so an xsi:type inside it is one of its raw attributes, the
//     element's own declining (item 7).
//   - The field of an <element ref> particle that codegen gives a sealed
//     interface over the referenced declaration's substitution group
//     (§3.3.6.4 key-eq; codegen's package doc, "# Substitution groups"
//     states which): in place of all these, I or []I, I that interface, the
//     bare interface for an optional one and never a pointer, nil meaning
//     absent, one interface value per child for []I; its alternatives are the
//     group's declarations with {abstract} false, in group order, each one's
//     branch field named from its local name and holding what the field of an
//     <element ref> particle occurring once to that declaration would hold
//     were its own substitution group that declaration alone: N, S or I, N
//     over X for a declaration of type xs:anyType, so a group with no such
//     declaration has no alternative and its field is nil.
//   - A sealed interface, T, a struct's first field or a nested element's, a
//     choice's or a substitution group's, whose branch types reach Decode
//     only through [WithBranches]: it does not match when it is not an
//     interface whose one method is unexported, or when its branch set, as
//     WithBranches supplied it, is not one struct of one field per
//     alternative, that field the name and Go type of the alternative's
//     branch field: when no WithBranches supplies it for an interface with
//     alternatives, or its count differs, or it holds a nil value, a type
//     that is no struct, a type twice, or a branch matching no alternative
//     an earlier branch did not match. A branch is held to the one
//     alternative its field could be: of that alternative's name, its Go
//     type V or []V by the alternative's occurrence, V the alternative's Go
//     type for a simple type, any struct for codegen's struct, any interface
//     for a sealed interface, and the E, X or N its bullet above matches for
//     a wildcard or an element of type xs:anyType. A branch whose field could
//     be no alternative is held to the first alternative of its field's name
//     of complex type, a wildcard or a substitution group's interface, and
//     the error is that alternative's mismatch.
//   - A union's sealed interface I, T's own or a field's, at any depth, a
//     list's element and a variant's element included, whose variants reach
//     Decode only through [WithBranches], whose doc states which set serves
//     I: it does not match when its set is not one variant per basic member:
//     when no WithBranches supplies it for a union with a basic
//     member, or its count differs, or it holds a nil value, a type twice, or
//     a variant matching no basic member an earlier variant did not match. A
//     variant matches the first basic member, in codegen's basic-member order,
//     whose Go type is its underlying type, by shape alone, as T is matched to
//     a simple type above: the member's atomic Go type A, a struct of D's
//     shape for a datatype struct, its arms then held as a PrecisionDecimal's
//     are, []A for a list member, or []J for a list member whose item is a
//     union, J any interface of one unexported method, then held to that
//     union's variants by this same bullet. A union with no basic member
//     takes the empty set.
//   - The Value interface I of a PrecisionDecimal, T's own or a field's, at
//     any depth, a list's element included, whose arms reach Decode only
//     through [WithPrecisionDecimalArms]: it does not match when no
//     WithPrecisionDecimalArms supplies I's arms, or it supplies a nil arm, a
//     numerical arm that is not a struct of exactly Coefficient and Scale,
//     each a big.Int, and Negative, a bool, in that order, none embedded,
//     struct tags aside, a positive infinity, negative infinity or
//     not-a-number arm that is no struct of no field, or one type as two
//     arms.
func Decode[T any](schema *xsd.Schema, backend value.Backend, data []byte, opts ...Option) (*T, error) {
	cfg := newConfig(opts)
	v, err := validate.New(schema, backend, validate.WithLogger(cfg.log))
	if err != nil {
		return nil, err
	}
	res, err := xmlsrc.Validate(v, bytes.NewReader(data), xmlsrc.WithURI(cfg.uri))
	if err != nil {
		return nil, err
	}
	if err := verdict(res); err != nil {
		return nil, err
	}
	r := xmltree.NewReader(cfg.uri, bytes.NewReader(data))
	root, err := documentElement(r)
	if err != nil {
		return nil, err
	}
	b, err := bind(&pipeline{schema: schema, backend: backend, log: cfg.log.WithGroup("codec"), decimals: cfg.decimals, branches: cfg.branches}, root)
	if err != nil {
		return nil, err
	}
	f, err := b.check(reflect.TypeFor[T](), newCheckContext(cfg))
	if err != nil {
		return nil, err
	}
	out := new(T)
	if err := f.fill(r, root, reflect.ValueOf(out).Elem()); err != nil {
		return nil, err
	}
	return out, nil
}

// CheckDecode returns, reading no instance, the error items 5 and 6 of
// [Decode]'s doc state for a document element of the top-level element
// declaration name in schema decoded into T under opts, or nil when neither
// applies. Of opts it reads only the sets [WithBranches] and
// [WithPrecisionDecimalArms] supply. The first of these that applies is
// returned:
//
//  1. A nil schema: a plain error.
//  2. No top-level element declaration of schema has the expanded name name:
//     a plain error naming name.
//  3. The declaration has {abstract} true: a plain error, since no document
//     element validates against it (Structures §3.3.4.3 cvc-elt clause 2).
//  4. A construct outside the decoded slice, item 5 of Decode's doc: its
//     *DeclineError, charged to no instance Loc ([DeclineError.Loc]), or the
//     plain error it states in a decline's place.
//  5. A T that is not the type codegen emits for the declaration, item 6 of
//     Decode's doc: its error, which errors.Is reports as [ErrTypeMismatch].
//
// A plain error is no *DeclineError, wraps neither [ErrTypeMismatch] nor
// errors.ErrUnsupported, and carries no xsderr rule. nil means Decode returns
// neither its item 5 nor its item 6 for a document element of name; Decode's
// items 2 to 4, 7 and 8 depend on the instance.
func CheckDecode[T any](schema *xsd.Schema, name xsd.QName, opts ...Option) error {
	if schema == nil {
		return errors.New("codec: CheckDecode: nil *xsd.Schema")
	}
	decl, ok := schema.Element(name)
	if !ok {
		return fmt.Errorf("codec: no top-level element declaration has the expanded name %q", name.String())
	}
	// An answer about the schema, no violation of an instance, so it carries
	// no xsderr rule (STYLE E2).
	if decl.Abstract() {
		return fmt.Errorf("codec: %s has {abstract} true, so no document element validates against it (cvc-elt clause 2)", declSubject(decl))
	}
	b, err := bindDecl(&pipeline{schema: schema}, decl)
	if err != nil {
		return err
	}
	_, err = b.check(reflect.TypeFor[T](), newCheckContext(newConfig(opts)))
	return err
}

// verdict returns nil for a Result Decode decodes, and otherwise the error
// Decode's doc states for it, items 2 to 4.
func verdict(res *validate.Result) error {
	if err := res.Err(); err != nil {
		return err
	}
	switch res.Standing() {
	case validate.NotProvenInvalid:
		return nil
	case validate.Charged:
		return &chargedError{violations: res.Violations()}
	case validate.Undecided:
		return &UndecidedError{u: res.Unevaluated()[0]}
	}
	return fmt.Errorf("codec: the validator returned Standing %d, which is no arm", res.Standing())
}

// ViolationsOf returns every violation of the charged error [Decode]
// returns for a validate.Charged Result (Decode's doc, item 3), in the
// order Result.Violations lists them, however many layers wrap that error.
// It returns nil for every other error, a nil err included. The slice is
// fresh on each call: mutating it does not affect err.
func ViolationsOf(err error) []*xsderr.Error {
	var ce *chargedError
	if !errors.As(err, &ce) {
		return nil
	}
	return slices.Clone(ce.violations)
}

// chargedError is the error Decode's doc states in item 3; violations is the
// Result's, never empty.
type chargedError struct{ violations []*xsderr.Error }

// Error opens with codec's statement of the charge and then gives each
// violation's text, one per line.
func (e *chargedError) Error() string {
	return "codec: the instance is invalid against the schema: " + errors.Join(e.Unwrap()...).Error()
}

// Unwrap returns the violations, each as an error, for the errors.As and
// xsderr.RuleOf reads Decode's doc states in item 3.
func (e *chargedError) Unwrap() []error {
	errs := make([]error, len(e.violations))
	for i, v := range e.violations {
		errs[i] = v
	}
	return errs
}

// An UndecidedError is the error [Decode] returns for a validate.Undecided
// Result (Decode's doc, item 4): the validator reached a check it did not
// perform, so the instance is not proven valid. It wraps
// errors.ErrUnsupported, as a [*DeclineError] does; errors.As to each type
// tells the two apart. It carries only the first, in document order, of the
// Result's validate.Unevaluated checks; the full list is
// validate.Result.Unevaluated, which a caller reaches by validating with
// package validate directly (validate.New, then validate/xmlsrc.Validate).
type UndecidedError struct{ u validate.Unevaluated }

// Unevaluated returns the one check e carries: its rule, its instance Loc and
// its message.
func (e *UndecidedError) Unevaluated() validate.Unevaluated { return e.u }

// Error names the Unevaluated's instance Loc and rule.
func (e *UndecidedError) Error() string {
	return fmt.Sprintf("codec: %s: the validator left %s undecided, so the instance is not proven valid: %v", e.u.Loc(), e.u.Rule(), errors.ErrUnsupported)
}

// Unwrap returns errors.ErrUnsupported.
func (e *UndecidedError) Unwrap() error { return errors.ErrUnsupported }

// documentElement returns the start tag of r's document element.
func documentElement(r *xmltree.Reader) (*xmltree.StartElement, error) {
	for {
		tok, err := r.Token()
		if err != nil {
			return nil, err
		}
		if start, ok := tok.(*xmltree.StartElement); ok {
			return start, nil
		}
	}
}

// shape is the occurrence a field holds its value in, by codegen's
// occurrence rules: one value, an optional one, or a repeated one. The zero
// value is invalid (STYLE T1).
type shape uint8

const (
	shapeOne      shape = iota + 1 // V
	shapeOptional                  // *V
	shapeRepeated                  // []V
)

// elem returns the Go type V that t holds by s, t itself for shapeOne and the
// element type of a pointer for shapeOptional and of a slice for
// shapeRepeated, and false when t is no type of shape s.
func (s shape) elem(t reflect.Type) (reflect.Type, bool) {
	switch s {
	case shapeOne:
		return t, true
	case shapeOptional:
		if t.Kind() != reflect.Pointer {
			return nil, false
		}
		return t.Elem(), true
	case shapeRepeated:
		if t.Kind() != reflect.Slice {
			return nil, false
		}
		return t.Elem(), true
	}
	// The zero shape is an internal bug, never input.
	panic(fmt.Sprintf("codec: invalid shape %d", s))
}

// over returns the Go type of shape s over v: V, *V or []V.
func (s shape) over(v reflect.Type) reflect.Type {
	switch s {
	case shapeOne:
		return v
	case shapeOptional:
		return reflect.PointerTo(v)
	case shapeRepeated:
		return reflect.SliceOf(v)
	}
	// The zero shape is an internal bug, never input.
	panic(fmt.Sprintf("codec: invalid shape %d", s))
}

// prefix spells what s puts before V in an ErrTypeMismatch message: nothing,
// "*" or "[]".
func (s shape) prefix() string {
	switch s {
	case shapeOne:
	case shapeOptional:
		return "*"
	case shapeRepeated:
		return "[]"
	}
	return ""
}

// store stores v, a decoded value of a type that converts to the V a field of
// shape s holds, or a decimalValue or a list of them that p builds one from,
// in dst, a value of a Go type of shape s: a repeated one appends it, an
// optional one points at a copy of it, each converted to dst's own element
// type, and shapeOne sets dst to it converted to dst's type, each conversion
// convertTo's.
func (s shape) store(p *pipeline, dst, v reflect.Value) {
	switch s {
	case shapeOptional:
		ptr := reflect.New(dst.Type().Elem())
		ptr.Elem().Set(convertTo(p, v, dst.Type().Elem()))
		dst.Set(ptr)
	case shapeRepeated:
		dst.Set(reflect.Append(dst, convertTo(p, v, dst.Type().Elem())))
	default:
		dst.Set(convertTo(p, v, dst.Type()))
	}
}

// convertTo returns v converted to t: a decimalValue, or a list of them, as
// p.decimal builds the caller's PrecisionDecimal from it, and a unionValue, as
// p.union builds the caller's variant from it, since neither converts to a
// caller type; a slice whose element type is not t's, a list of codec's
// mirror of a datatype struct or of unionValues, which converts to no slice
// of the caller's type, element by element into a new slice of t of v's
// length, each element as convertTo converts it; and any other value by
// Convert.
func convertTo(p *pipeline, v reflect.Value, t reflect.Type) reflect.Value {
	if v.Type() == decimalValueType || (v.Kind() == reflect.Slice && v.Type().Elem() == decimalValueType) {
		return p.decimal(v, t)
	}
	if v.Type() == unionValueType {
		return p.union(v, t)
	}
	if v.Kind() != reflect.Slice || v.Type().Elem() == t.Elem() {
		return v.Convert(t)
	}
	out := reflect.MakeSlice(t, v.Len(), v.Len())
	for i := range v.Len() {
		out.Index(i).Set(convertTo(p, v.Index(i), t.Elem()))
	}
	return out
}

// valueType is how the Go type V a field's shape holds its value in relates
// to the Go type codegen gives the field's simple type (scalar.goType), or
// to codegen's AnyType for an element of type xs:anyType (anyTypeArm), by
// codegen's field rules: that type itself for a local element declaration's
// particle and an attribute use, and, for an <element ref> particle and a
// substitution group's member's branch field, the named type codegen emits
// for the referenced top-level declaration, whose underlying type it is. The
// zero value is invalid (STYLE T1).
type valueType uint8

const (
	valueBare  valueType = iota + 1 // the simple type's Go type, or AnyType, itself
	valueNamed                      // a named type over the simple type's Go type, or over AnyType
)

// field is one field a particle gives a struct codegen emits: a complex
// type's struct's, or a choice's branch struct's. It holds its Go name, its
// shape, and arm, the V its shape holds.
type field struct {
	name  string
	shape shape
	arm   fieldArm
}

// fieldArm is the V a field's shape holds, as Decode reads it, a closed sum:
// a scalarArm, the Go type codegen gives a simple type or a named type over
// it; a structArm, a nested complex type's struct; an ifaceArm, a nested
// complex type's bare sealed interface; a groupArm, the sealed interface
// over a substitution group; a rawArm, a wildcard particle's AnyElement; or
// an anyTypeArm, an element of type xs:anyType's AnyType or a named type
// over it.
type fieldArm interface {
	// bind holds v, the V the field's shape holds at at, to the arm, item 6 of
	// Decode's doc, and returns the elementReader that reads the field's
	// element into a V, or at's mismatch.
	bind(v reflect.Type, at fieldAt, cx *checkContext) (elementReader, error)
	// admits reports whether v could be the V the arm binds: for a scalarArm,
	// a rawArm and an anyTypeArm, exactly the V bind accepts, but that bind
	// also holds a PrecisionDecimal scalarArm's V to the arms checkContext
	// supplies for its Value interface; for a structArm, a struct; for an
	// ifaceArm and a groupArm, an interface. bind rejects every v the arm does
	// not admit before it checks anything further, so such a v stores no
	// checkContext.bound entry.
	admits(v reflect.Type) bool
	// describe spells V for an ErrTypeMismatch message.
	describe() string
}

// scalarArm is a field's V where the field's element or attribute is of a
// simple type: value, how V relates to the Go type codegen gives that type,
// and the scalar the value decodes as.
type scalarArm struct {
	value valueType
	scalar
}

// admits reports whether v is the V codegen gives a, as Decode's doc, item 6,
// reads it: the Go type codegen gives a's simple type (scalar.goType) itself
// for valueBare, and for valueNamed a type other than it over it
// (scalar.underlies), its name never compared. A datatype struct D, a
// PrecisionDecimal included, is declared in the caller's package, so codec
// holds no D to compare v with: for valueBare and valueNamed alike v is a
// struct of D's shape, and for a list []D, an unnamed slice of such a struct,
// for valueBare, and a defined slice type over one, N for type N []D, for
// valueNamed. A union's interface I is declared in the caller's package too,
// and N over I is an interface of I's method set, so for valueBare and
// valueNamed alike v is any interface of one unexported method, and for a
// list of a union, []I, an unnamed slice of one for valueBare, and a defined
// slice type over one for valueNamed.
func (a scalarArm) admits(v reflect.Type) bool {
	if a.callerDeclared() {
		if !a.underlies(v) {
			return false
		}
		if a.item == nil {
			return true
		}
		return (v.Name() == "") == (a.value == valueBare)
	}
	want := a.goType()
	switch a.value {
	case valueBare:
		return v == want
	case valueNamed:
		return v != want && a.underlies(v)
	}
	// The zero valueType is an internal bug, never input.
	panic(fmt.Sprintf("codec: invalid value type %d", a.value))
}

// bind returns a, which reads the field's element, when a admits v and, for
// a PrecisionDecimal or a union, cx holds the sets of the interfaces it holds
// (scalar.holdSets), and otherwise at's mismatch, or the sets' mismatch.
func (a scalarArm) bind(v reflect.Type, at fieldAt, cx *checkContext) (elementReader, error) {
	if !a.admits(v) {
		return nil, at.mismatch()
	}
	if err := a.holdSets(v, at.site(), cx); err != nil {
		return nil, err
	}
	return a, nil
}

// read reads the character children of the element start from r up to its
// end tag and returns their value, or the supplied one's, as elementValue
// does.
func (a scalarArm) read(p *pipeline, r *xmltree.Reader, start *xmltree.StartElement) (reflect.Value, error) {
	return elementValue(r, start, p, a.scalar)
}

// describe spells V: the Go type codegen gives a's simple type, A or for a
// list []A, as scalar.spell spells it, or N for a named type N over it, which
// a datatype struct D and a union's interface I, of no list, are not told
// apart from (admits).
func (a scalarArm) describe() string {
	if a.value == valueNamed && (!a.callerDeclared() || a.item != nil) {
		return "N for a named type N over " + a.spell()
	}
	return a.spell()
}

// structArm is a field's V where the field's element is of a complex type
// codegen gives a struct, unit's: S, matched by its fields as T is, its name
// never compared, whether the particle is local or an <element ref>.
type structArm struct{ unit *binding }

// bind holds v to a's struct, and returns the reader that fills a V from the
// field's element.
func (a structArm) bind(v reflect.Type, at fieldAt, cx *checkContext) (elementReader, error) {
	if !a.admits(v) {
		return nil, at.mismatch()
	}
	f, err := a.unit.check(v, cx)
	if err != nil {
		return nil, err
	}
	return nestedReader{t: v, fill: f}, nil
}

// admits reports whether v is a struct, the kind of every S.
func (structArm) admits(v reflect.Type) bool { return v.Kind() == reflect.Struct }

// describe spells V.
func (structArm) describe() string { return "S for a struct S" }

// ifaceArm is a field's V where the field's element is of a complex type
// codegen gives a bare sealed interface, unit's: I, held to unit's choice as
// T is, its name never compared. Its field's shape is never shapeOptional
// (choiceRoot.field).
type ifaceArm struct{ unit *choiceRoot }

// bind holds v to a's interface, and returns the reader that fills a V from
// the field's element.
func (a ifaceArm) bind(v reflect.Type, at fieldAt, cx *checkContext) (elementReader, error) {
	if !a.admits(v) {
		return nil, at.mismatch()
	}
	f, err := a.unit.checkAt(v, at.site(), cx)
	if err != nil {
		return nil, err
	}
	return nestedReader{t: v, fill: f}, nil
}

// admits reports whether v is an interface, the kind of every I.
func (ifaceArm) admits(v reflect.Type) bool { return v.Kind() == reflect.Interface }

// describe spells V.
func (ifaceArm) describe() string { return "I for a sealed interface I" }

// groupArm is a field's V where the field is an <element ref> particle's to a
// head whose substitution group codegen gives a sealed interface (groupField):
// I, held to sealed's branches as a choice's interface is, its name never
// compared. Its field's shape is never shapeOptional.
type groupArm struct{ sealed *sealed }

// bind holds v to a's interface, and returns the reader that reads one
// member's element into a fresh V.
func (a groupArm) bind(v reflect.Type, at fieldAt, cx *checkContext) (elementReader, error) {
	if !a.admits(v) {
		return nil, at.mismatch()
	}
	bc, err := a.sealed.checkSite(v, at.site(), cx)
	if err != nil {
		return nil, err
	}
	return groupReader{t: v, bound: bc}, nil
}

// admits reports whether v is an interface, the kind of every I.
func (groupArm) admits(v reflect.Type) bool { return v.Kind() == reflect.Interface }

// describe spells V.
func (groupArm) describe() string { return "I for a substitution group's sealed interface I" }

// groupReader reads a member's element into a new value of t, the interface
// a groupArm bound, through bound, so a repeated field holds one interface
// value per child.
type groupReader struct {
	t     reflect.Type
	bound *boundChoice
}

// read returns a new value of g.t holding the branch of start's member, the
// one whose expanded name is start's.
func (g groupReader) read(_ *pipeline, r *xmltree.Reader, start *xmltree.StartElement) (reflect.Value, error) {
	i, ok := g.bound.sealed.children[expanded(start.Name())]
	if !ok {
		return reflect.Value{}, unparticled(start)
	}
	v := reflect.New(g.t).Elem()
	if err := g.bound.set(r, start, i, v); err != nil {
		return reflect.Value{}, err
	}
	return v, nil
}

// rawArm is a field's V where the field is a wildcard particle's: E, a
// struct of the shape of codegen's AnyElement, as rawShapeOf matches it, its
// name never compared (codegen/doc.go "# Element declarations").
type rawArm struct{}

// bind returns the rawShape of v, which reads the field's element, when v is
// a struct of AnyElement's shape, and otherwise at's mismatch.
func (rawArm) bind(v reflect.Type, at fieldAt, _ *checkContext) (elementReader, error) {
	s, ok := rawShapeOf(v)
	if !ok {
		return nil, at.mismatch()
	}
	return s, nil
}

// admits reports whether v is a struct of AnyElement's shape, exactly the V
// bind accepts.
func (rawArm) admits(v reflect.Type) bool {
	_, ok := rawShapeOf(v)
	return ok
}

// describe spells V.
func (rawArm) describe() string { return "E for a struct E of codegen's AnyElement shape" }

// anyTypeArm is a field's V where the field's element is of type xs:anyType
// itself (§3.4.7 any-type-itself): for valueBare, X, a struct of the shape of
// codegen's AnyType, and for valueNamed, N, a type other than X whose
// underlying type is X's, the named type codegen emits for a top-level
// declaration of type xs:anyType (codegen/doc.go "# xs:anyType"); X is
// matched through contentShapeOf, and no struct's or N's name is compared.
type anyTypeArm struct{ value valueType }

// bind returns the reader of v's raw content, when a admits v, and otherwise
// at's mismatch.
func (a anyTypeArm) bind(v reflect.Type, at fieldAt, _ *checkContext) (elementReader, error) {
	s, ok := a.shapeOf(v)
	if !ok {
		return nil, at.mismatch()
	}
	return rawContent{shape: s}, nil
}

// admits reports whether v is X for valueBare or N for valueNamed, exactly
// the V bind accepts.
func (a anyTypeArm) admits(v reflect.Type) bool {
	_, ok := a.shapeOf(v)
	return ok
}

// shapeOf returns the rawShape contentShapeOf reaches from v, when v is the V
// a holds: that shape's content itself for valueBare, and for valueNamed a
// type other than it that converts to it, so of its underlying type.
func (a anyTypeArm) shapeOf(v reflect.Type) (rawShape, bool) {
	s, ok := contentShapeOf(v)
	if !ok {
		return rawShape{}, false
	}
	switch a.value {
	case valueBare:
		return s, v == s.content
	case valueNamed:
		return s, v != s.content && v.ConvertibleTo(s.content)
	}
	// The zero valueType is an internal bug, never input.
	panic(fmt.Sprintf("codec: invalid value type %d", a.value))
}

// describe spells V.
func (a anyTypeArm) describe() string {
	if a.value == valueNamed {
		return "N for a named type N over a struct of AnyType's shape"
	}
	return "X for a struct X of codegen's AnyType shape"
}

// contentShapeOf returns the rawShape reached from x through its Children's
// element type C and C's Element, E, as rawShapeOf matches E, when x is a
// struct of exactly the fields Attributes, Text and Children, Children a
// slice, and C a struct of exactly Element and Tail. Its caller holds x to
// the shape's content; these guards only keep the walk from panicking.
func contentShapeOf(x reflect.Type) (rawShape, bool) {
	if !fieldsNamed(x, "Attributes", "Text", "Children") || x.Field(2).Type.Kind() != reflect.Slice {
		return rawShape{}, false
	}
	c := x.Field(2).Type.Elem()
	if !fieldsNamed(c, "Element", "Tail") {
		return rawShape{}, false
	}
	return rawShapeOf(c.Field(0).Type)
}

// rawContent reads an element of type xs:anyType into a new value of
// shape.content, raw, as readContent reads it; a valueNamed field's
// shape.store converts it to N.
type rawContent struct{ shape rawShape }

// read returns the content of the element start, read from r up to its end
// tag.
func (c rawContent) read(_ *pipeline, r *xmltree.Reader, start *xmltree.StartElement) (reflect.Value, error) {
	return c.shape.readContent(r, start)
}

// rawShape is the four raw-content structs codegen emits (codegen/doc.go "#
// Element declarations"), as a T holds them: element, E, AnyElement's;
// content, X, AnyType's; attribute, A, AnyAttribute's; and child, C,
// AnyChild's. rawShapeOf states their fields.
type rawShape struct {
	element, content, attribute, child reflect.Type
}

// rawShapeOf returns the rawShape whose element is e, when e is a struct of
// AnyElement's shape: e's fields exactly Space and Local, each a string, and
// Content, X, in that order; X's exactly Attributes, a slice of A, Text, a
// string, and Children, a slice of C; A's exactly Space, Local and Value,
// each a string; and C's exactly Element, of e itself, and Tail, a string.
// Each field is matched by name, order and Go type, none embedded, struct
// tags aside, and no struct by its name, so codegen's AnyElement2 and
// AnyType2 match; a slice may be named or not. C's Element is the one field
// that reaches back, and it must be e, so the match is finite.
func rawShapeOf(e reflect.Type) (rawShape, bool) {
	str := reflect.TypeFor[string]()
	if !fieldsNamed(e, "Space", "Local", "Content") || e.Field(0).Type != str || e.Field(1).Type != str {
		return rawShape{}, false
	}
	x := e.Field(2).Type
	if !fieldsNamed(x, "Attributes", "Text", "Children") || x.Field(1).Type != str ||
		x.Field(0).Type.Kind() != reflect.Slice || x.Field(2).Type.Kind() != reflect.Slice {
		return rawShape{}, false
	}
	a, c := x.Field(0).Type.Elem(), x.Field(2).Type.Elem()
	if !fieldsNamed(a, "Space", "Local", "Value") || a.Field(0).Type != str || a.Field(1).Type != str || a.Field(2).Type != str {
		return rawShape{}, false
	}
	if !fieldsNamed(c, "Element", "Tail") || c.Field(0).Type != e || c.Field(1).Type != str {
		return rawShape{}, false
	}
	return rawShape{element: e, content: x, attribute: a, child: c}, true
}

// fieldsNamed reports whether t is a struct whose fields are exactly names,
// in order, none embedded.
func fieldsNamed(t reflect.Type, names ...string) bool {
	if t.Kind() != reflect.Struct || t.NumField() != len(names) {
		return false
	}
	for i, name := range names {
		if f := t.Field(i); f.Name != name || f.Anonymous {
			return false
		}
	}
	return true
}

// read reads the element start, which a wildcard particle matched, from r up
// to its end tag into a new value of s.element, raw: Space and Local its
// expanded name, and Content its content as readContent reads it. Nothing in
// it is assessed again: no xsi:type is read and no declaration consulted, at
// any depth.
func (s rawShape) read(_ *pipeline, r *xmltree.Reader, start *xmltree.StartElement) (reflect.Value, error) {
	e := reflect.New(s.element).Elem()
	e.Field(0).SetString(start.Name().Space())
	e.Field(1).SetString(start.Name().Local())
	content, err := s.readContent(r, start)
	if err != nil {
		return reflect.Value{}, err
	}
	e.Field(2).Set(content)
	return e, nil
}

// readContent reads the content of the element start from r up to its end
// tag into a new value of s.content: Attributes, one A per attribute of
// start in start-tag order, xsi: ones included and namespace declarations
// not (xmltree.StartElement.Attributes), each its expanded name and its
// value as the reader normalized it, a prefix inside it unresolved, nil for
// none; Text, the character data before the first child element, every
// adjacent run concatenated, "" for none; and Children, one C per child
// element in document order, its Element the child as read reads it and its
// Tail the character data after it, up to the next child element or start's
// end tag, nil for none. Comments and processing instructions are no
// tokens of r, so they hold no place.
func (s rawShape) readContent(r *xmltree.Reader, start *xmltree.StartElement) (reflect.Value, error) {
	x := reflect.New(s.content).Elem()
	attrs := x.Field(0)
	for _, a := range start.Attributes() {
		v := reflect.New(s.attribute).Elem()
		v.Field(0).SetString(a.Name().Space())
		v.Field(1).SetString(a.Name().Local())
		v.Field(2).SetString(a.Value())
		attrs.Set(reflect.Append(attrs, v))
	}
	children := x.Field(2)
	// text gathers the character data due at tail: X's Text, then the last
	// child's Tail.
	var text bytes.Buffer
	tail := x.Field(1)
	for {
		tok, err := r.Token()
		if err != nil {
			return reflect.Value{}, err
		}
		switch tok := tok.(type) {
		case *xmltree.CharData:
			text.WriteString(tok.Data())
		case *xmltree.StartElement:
			tail.SetString(text.String())
			text.Reset()
			el, err := s.read(nil, r, tok)
			if err != nil {
				return reflect.Value{}, err
			}
			c := reflect.New(s.child).Elem()
			c.Field(0).Set(el)
			children.Set(reflect.Append(children, c))
			tail = children.Index(children.Len() - 1).Field(1)
		case *xmltree.EndElement:
			tail.SetString(text.String())
			return x, nil
		}
	}
}

// nestedReader reads an element of complex type into a new value of t, the
// V a structArm or ifaceArm bound, through fill.
type nestedReader struct {
	t    reflect.Type
	fill filler
}

// read returns a new value of n.t that fill sets from the element start.
func (n nestedReader) read(_ *pipeline, r *xmltree.Reader, start *xmltree.StartElement) (reflect.Value, error) {
	v := reflect.New(n.t).Elem()
	if err := n.fill.fill(r, start, v); err != nil {
		return reflect.Value{}, err
	}
	return v, nil
}

// An elementReader reads the element start, whose start tag r has just
// returned, from r up to its end tag, and returns its value, of the V a field
// holds, under p.
type elementReader interface {
	read(p *pipeline, r *xmltree.Reader, start *xmltree.StartElement) (reflect.Value, error)
}

// fieldAt is field index of the struct owner, the site item 6 holds want to.
type fieldAt struct {
	owner reflect.Type
	index int
	want  field
}

// mismatch is the ErrTypeMismatch error for the field at a, which is not
// a.want as codegen emits it.
func (a fieldAt) mismatch() error {
	got := a.owner.Field(a.index)
	return fmt.Errorf("%w: field %d of %s is %s %s where codegen emits %s %s", ErrTypeMismatch, a.index, a.owner, got.Name, got.Type, a.want.name, a.want.describe())
}

// site names the field at a by its Go name and owner, as a sealed
// interface's ErrTypeMismatch message names its site.
func (a fieldAt) site() string {
	return fmt.Sprintf("field %s of %s", a.owner.Field(a.index).Name, a.owner)
}

// check holds field index of owner to f, item 6 of Decode's doc: the same
// name, not embedded, a Go type of f's shape, and a V f's arm binds. It
// returns the elementReader that reads f's element.
func (f field) check(owner reflect.Type, index int, cx *checkContext) (elementReader, error) {
	at := fieldAt{owner: owner, index: index, want: f}
	got := owner.Field(index)
	v, shaped := f.shape.elem(got.Type)
	if !shaped || got.Name != f.name || got.Anonymous {
		return nil, at.mismatch()
	}
	return f.arm.bind(v, at, cx)
}

// admits reports whether got, a branch struct's one field, could be f as
// check reads it: f's name, not embedded, a Go type of f's shape, and a V f's
// arm admits. Every got that check holds to f, f admits; a got f does not
// admit, check rejects before its arm checks anything further, so it stores
// no checkContext.bound entry.
func (f field) admits(got reflect.StructField) bool {
	if got.Name != f.name || got.Anonymous {
		return false
	}
	v, shaped := f.shape.elem(got.Type)
	return shaped && f.arm.admits(v)
}

// sharesBranch reports whether f and g, two branch fields of one sealed
// interface, a choice's alternatives' or a substitution group's members', may
// admit a common Go type, as field.admits reads one, so that no branch type
// could tell them apart (sealed.alternative). It reads their names, shapes,
// arm classes, scalar elements and valueTypes only, never a nested unit's
// content, which may be unbound or reach itself here, and the first of these
// that applies answers:
//
//   - Different names: false.
//   - Either arm a scalarArm of a datatype struct, or a list of one: true,
//     which is conservative, since its V is a struct matched by shape
//     (scalarArm.admits), which another datatype struct's or codegen's
//     struct S may share.
//   - Either arm a scalarArm of a union, or of a list of one: true exactly
//     when both arms hold an interface (holdsInterface), which is
//     conservative, since their shapes and units are not compared; beside any
//     other arm, false, since a structArm's, a rawArm's and an anyTypeArm's V
//     is a struct and another scalar's A or []A holds no interface.
//   - Either arm an anyTypeArm: for two anyTypeArms, true exactly when they
//     have the same valueType and one shape, a valueBare arm's X and a
//     valueNamed arm's N never one Go type (anyTypeArm.admits); beside a
//     structArm, true, which is conservative, since a structArm admits any
//     struct and X and N are structs; and beside a rawArm, a scalarArm, an
//     ifaceArm or a groupArm, false, since contentShapeOf admits only a
//     struct of the three exported fields Attributes, Text and Children,
//     which no rawArm's E is, no scalar's V is but a datatype struct's, which
//     the bullet above answers, and no interface is.
//   - Either arm a rawArm: for two rawArms, true exactly when they have one
//     shape; beside a structArm, true, which is conservative, since a
//     structArm admits any struct and a rawArm's V is one; and beside a
//     scalarArm, an ifaceArm or a groupArm, false, since rawShapeOf admits
//     only a struct of three exported fields, which no scalar's V is but a
//     datatype struct's, which the bullet above answers (big.Rat's and
//     big.Int's fields are unexported), and no interface.
//   - Both arms scalarArms: for two valueBare arms, true exactly when they
//     have the same Go type of shape over scalar.goType, a repeated xs:int
//     and a list of xs:int each []int32; for two valueNamed arms, true
//     exactly when they have the same shape and the same goType separately,
//     since the named type sits between them and []N over int32 is no N over
//     []int32; and for one of each, true exactly when the valueBare field is
//     repeated and the valueNamed one occurs once over a list of the first's
//     Go type, since a defined slice type over []A is both a []A's
//     element-wise slice and an N over []A.
//   - One arm a scalarArm, the other nested: true exactly when the nested arm
//     is a structArm and the scalar's atomic Go type is a struct, big.Rat or
//     big.Int, which is conservative, since a structArm admits any struct. No
//     scalar's V the bullets above leave is an interface, and none holds one
//     but a PrecisionDecimal's, which the datatype-struct bullet answers, so
//     an ifaceArm or a groupArm gives false.
//   - Both nested: true exactly when both are structArms, or neither is,
//     each then an ifaceArm or a groupArm, and they have one shape, which is
//     conservative, since their units' contents are not compared. A branch
//     field occurs once or repeated, its shapeOptional read as shapeOne, and
//     a nested arm's V is a struct or an interface, never a slice, so any
//     other pair admits no common Go type.
func (f field) sharesBranch(g field) bool {
	if f.name != g.name {
		return false
	}
	// fieldArm is a closed sum, so the assertions are STYLE T2's sealed-sum
	// exception.
	fa, fScalar := f.arm.(scalarArm)
	ga, gScalar := g.arm.(scalarArm)
	if (fScalar && fa.elem.datatypeStruct()) || (gScalar && ga.elem.datatypeStruct()) {
		return true
	}
	_, fUnion := fa.elem.(*unionScalar)
	_, gUnion := ga.elem.(*unionScalar)
	if fUnion || gUnion {
		return holdsInterface(f.arm) && holdsInterface(g.arm)
	}
	_, fStruct := f.arm.(structArm)
	_, gStruct := g.arm.(structArm)
	fx, fAny := f.arm.(anyTypeArm)
	gx, gAny := g.arm.(anyTypeArm)
	if fAny || gAny {
		if fAny && gAny {
			return fx.value == gx.value && f.shape == g.shape
		}
		return fStruct || gStruct
	}
	_, fRaw := f.arm.(rawArm)
	_, gRaw := g.arm.(rawArm)
	if fRaw || gRaw {
		if fRaw && gRaw {
			return f.shape == g.shape
		}
		return fStruct || gStruct
	}
	if fScalar != gScalar {
		a := fa
		if gScalar {
			a = ga
		}
		return (fStruct || gStruct) && a.elem.bare().Kind() == reflect.Struct
	}
	if !fScalar {
		return fStruct == gStruct && f.shape == g.shape
	}
	if fa.value != ga.value {
		bare, named, bareShape, namedShape := fa, ga, f.shape, g.shape
		if fa.value == valueNamed {
			bare, named, bareShape, namedShape = ga, fa, g.shape, f.shape
		}
		return bareShape == shapeRepeated && namedShape == shapeOne && shapeRepeated.over(bare.goType()) == named.goType()
	}
	if fa.value == valueNamed {
		return f.shape == g.shape && fa.goType() == ga.goType()
	}
	return f.shape.over(fa.goType()) == g.shape.over(ga.goType())
}

// holdsInterface reports whether a's V is an interface or a slice of one: an
// ifaceArm's, a groupArm's, or a scalarArm's of a union or a list of one.
// fieldArm is a closed sum, so the switch is STYLE T2's sealed-sum exception.
func holdsInterface(a fieldArm) bool {
	switch a := a.(type) {
	case ifaceArm, groupArm:
		return true
	case scalarArm:
		_, isUnion := a.elem.(*unionScalar)
		return isUnion
	}
	return false
}

// describe spells the Go type codegen gives f for an ErrTypeMismatch message:
// V, *V or []V, V as f's arm spells it.
func (f field) describe() string {
	return f.shape.prefix() + f.arm.describe()
}

// attribute is the field an attribute use gives a complex type's struct: its
// Go name, its shape, and the scalar its value decodes as; its V is the Go
// type codegen gives the attribute's simple type itself.
type attribute struct {
	name  string
	shape shape
	scalar
}

// field returns a as item 6 of Decode's doc reads a field.
func (a attribute) field() field {
	return field{name: a.name, shape: a.shape, arm: scalarArm{value: valueBare, scalar: a.scalar}}
}

// A binder is the type codegen emits for one document element declaration,
// as Decode reads it: check holds T to it, item 6 of Decode's doc, reading
// each sealed interface's branch types from cx, and returns the filler that
// sets a value of T from the document element. A *binding binds a complex
// {type definition} codegen gives a struct, a *choiceRoot one it gives a bare
// sealed interface, and a *simpleRoot a simple one.
type binder interface {
	check(t reflect.Type, cx *checkContext) (filler, error)
}

// A complexUnit is the type codegen emits for a complex {type definition}, a
// document element's or a particle's element declaration's, as Decode reads
// it, a closed sum: a *binding, a struct, or a *choiceRoot, a bare sealed
// interface. It binds T as a binder does where it is the document element's.
type complexUnit interface {
	binder
	// field returns the field, but for its name, of a particle of an element
	// of the unit's type occurring in shape s: its V the unit's type, codegen's
	// shape for it.
	field(s shape) field
}

// checkContext is what one item-6 check of T reads beside the binder: sets,
// the branch types [WithBranches] supplied, decimals, the arm types
// [WithPrecisionDecimalArms] supplied, and bound, the filler already bound
// for each complex unit and Go type the check has reached.
type checkContext struct {
	sets     branchSets
	decimals decimalArmSets
	// bound ties the check's knot (STYLE D4): a unit is held to a Go type once,
	// its entry stored before its fields are checked, and a later reach of the
	// same pair, through a recursive type, links to that entry. It lives for
	// one check only, a lookup index never iterated (STYLE D2); an entry left
	// by a check that fails is never read, since the failure ends the check.
	bound map[boundKey]filler
}

// newCheckContext returns the checkContext of a check under cfg's branch and
// arm sets.
func newCheckContext(cfg config) *checkContext {
	return &checkContext{sets: cfg.branches, decimals: cfg.decimals, bound: map[boundKey]filler{}}
}

// boundKey is a complex unit and the Go type item 6 holds it to.
type boundKey struct {
	unit complexUnit
	t    reflect.Type
}

// A filler sets out, a value of the T a binder's check held, from the
// element root, whose start tag r has just returned, and from r's tokens up
// to its end tag, or returns the xsi:type decline Decode's doc states, item 7.
type filler interface {
	fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error
}

// binding is the struct codegen emits for a complex type, a document
// element's or a particle's element declaration's, as Decode reads it: its
// fields in codegen's order, its content's first, then each attribute use's.
type binding struct {
	p       *pipeline
	content elementContent
	attrs   []attribute
	// attributes maps an attribute's expanded name to its field's index in
	// attrs; it is a lookup index only and never iterated (STYLE D2).
	attributes map[xsd.QName]int
}

// field returns the field of shape s whose V is b's struct.
func (b *binding) field(s shape) field { return field{shape: s, arm: structArm{unit: b}} }

// elementContent is the content of the struct a *binding binds, the fields
// that precede its attribute fields, a closed sum: a *sequence, the fields of
// a sequence's particles, or of none for empty content, a *choice, the one
// field holding a choice's sealed interface, or a *simpleValue, simple
// content's value field.
type elementContent interface {
	// width is how many of the struct's fields the content's are.
	width() int
	// check holds t's first width fields to the content's, item 6 of Decode's
	// doc, and returns the contentFiller that sets them.
	check(t reflect.Type, cx *checkContext) (contentFiller, error)
}

// A contentFiller sets the content's fields of out, the struct an
// elementContent's check held, from the content of the element root, whose
// start tag r has just returned, reading r up to its end tag: each child to
// the field eachChild attributes it to, or, for simple content, the character
// children to the value field.
type contentFiller interface {
	fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error
}

// bind returns the binder of root's declaration in p's schema, or the first
// decline Decode's doc states for it, item 5, charged to root's instance Loc.
// Item 5 is bindDecl's, which CheckDecode calls too.
func bind(p *pipeline, root *xmltree.StartElement) (binder, error) {
	name := expanded(root.Name())
	decl, ok := p.schema.Element(name)
	if !ok {
		// Only an xsi:type lets such a document element be assessed
		// (cvc-assess-elt), so it declines as one, in place of item 5.
		if err := checkXsiType(root); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("codec: %s: the document element %s has no top-level declaration", root.Loc(), name)
	}
	b, err := bindDecl(p, decl)
	// bindDecl returns its declines unwrapped and uncharged, and no other
	// error of its wraps one, so the charged copy replaces err whole.
	var de *DeclineError
	if errors.As(err, &de) {
		return nil, de.at(root.Loc())
	}
	return b, err
}

// bindDecl returns the binder of decl, a top-level element declaration in p's
// schema, or the first decline Decode's doc states for it, item 5, charged to
// no instance Loc, or the plain error item 5 states in a decline's place. It
// reads only p's schema.
func bindDecl(p *pipeline, decl xsd.ElementDeclaration) (binder, error) {
	if d := declarationDecline(decl); d != 0 {
		return nil, decline(d, declSubject(decl))
	}
	td, ok := p.schema.ResolvedType(decl.TypeDefinition())
	if !ok {
		return nil, fmt.Errorf("codec: the {type definition} of %s does not resolve", declSubject(decl))
	}
	// TypeDefinition is a closed sum of the two, so the assertions are STYLE
	// T2's sealed-sum exception.
	if st, ok := td.(*xsd.SimpleType); ok {
		return bindSimpleRoot(p, decl, st, typeSubject(decl, td))
	}
	ct, _ := td.(xsd.ComplexType)
	run := &bindRun{p: p, elements: unitIndex[xsd.QName]{}, types: unitIndex[xsd.QName]{}, groups: unitIndex[groupPath]{}}
	return bindComplex(run, decl, ct, run.elements, decl.Name())
}

// typeSubject names td, the {type definition} of decl, as a decline of one
// of td's constructs labels it: by decl and its schema Loc, then td's.
func typeSubject(decl xsd.ElementDeclaration, td xsd.TypeDefinition) string {
	return fmt.Sprintf("the {type definition} of %s, a type defined at %s", declSubject(decl), td.Loc())
}

// bindRun is one item-5 bind of a document element declaration: p, and the
// complex units it has reached by name. Codec descends into two kinds of
// term, an inline local element declaration and an <element ref>, the
// latter to the referenced declaration or to each member of its
// substitution group, and into one kind of model group, a content
// particle's, inline or through a <group ref>; a local declaration's type is
// either inline, a tree it owns, or named. So every recursion through types
// passes one of the three keys of codegen's emitted types: a top-level
// element declaration, the document element's or one an <element ref>
// reaches, keyed by its expanded name in elements; the named {type
// definition} of a local element declaration, keyed by its QName in types;
// and the anonymous type of a local element declaration whose {scope}'s
// {parent} is a model group definition, which only a <group ref> reaches,
// keyed by its groupPath in groups. Every other inline anonymous type is
// bound in place, under no key, inside the unit of the type or group that
// declares it, which is bound once.
type bindRun struct {
	p        *pipeline
	elements unitIndex[xsd.QName]
	types    unitIndex[xsd.QName]
	groups   unitIndex[groupPath]
}

// groupPath keys the anonymous complex type of element, a local element
// declaration whose {scope}'s {parent} is the model group definition group
// (§3.3.2.3 dcl.elt.local): the path of expanded names from group, which a
// <group ref> re-roots at its group, one step long, since an anonymous type
// nested deeper is bound in place in its ancestor's unit. It mirrors codegen's
// nestedKey.
type groupPath struct {
	group, element xsd.QName
}

// unitIndex maps a key to the complex unit bound for it. Each unit's entry
// is stored when the unit is allocated, before its content is bound, so a
// later reach of the key, a recursive one included, links to the unit and
// examines nothing again: this is STYLE D4's named placeholder, no visited
// set. It lives for one bindRun only and is a lookup index never iterated
// (STYLE D2). No entry is removed when a bind fails: any item-5 error ends
// the whole bind.
type unitIndex[K comparable] map[K]complexUnit

// bindComplex returns the complex unit of ct, the {type definition} of decl,
// labelled by typeSubject, the one index already holds for key where it holds
// one, or else the first decline Decode's doc states for ct, item 5, in its
// order: {abstract} true, ct's content, for simple content its {simple type
// definition} (simpleValueOf), for element-only content the occurrence of its
// sequence or choice and each particle in order, then each attribute use in
// order, and its {attribute wildcard}. Empty content binds the struct codegen
// emits for it, with no field ahead of the attribute fields, and simple
// content the struct whose one field ahead of them is the value field
// (codegen/doc.go "# Complex types"). A nil index binds an inline anonymous ct
// under no key. A ct of simple content is stored under no key either: it
// reaches no particle, so nothing inside it reaches it again, and its value
// field holds decl's own {value constraint} (cvc-elt clause 5.1), which
// another declaration of a named ct does not share.
func bindComplex[K comparable](run *bindRun, decl xsd.ElementDeclaration, ct xsd.ComplexType, index unitIndex[K], key K) (complexUnit, error) {
	if u, ok := index[key]; ok {
		return u, nil
	}
	typeSubject := typeSubject(decl, ct)
	// Codegen emits no struct for an abstract type: an element of one holds
	// its governing types' interface, or the struct of its one member type
	// (codegen/doc.go "# Element declarations").
	if ct.Abstract() {
		return nil, decline(constructAbstractTypeElement, typeSubject)
	}
	// names is the struct's Namer: the content's fields claim from it in
	// particle order, then the attribute fields (codegen/doc.go "# Particles").
	// A bare sealed interface has no struct and discards it.
	names := goname.NewNamer()
	b := &binding{p: run.p, attributes: map[xsd.QName]int{}}
	// ContentType is a closed sum, so the switch is STYLE T2's sealed-sum
	// exception.
	switch c := ct.ContentType().(type) {
	case xsd.SimpleContent:
		value, err := simpleValueOf(run.p, decl, c.SimpleType, typeSubject, names)
		if err != nil {
			return nil, err
		}
		b.content = value
	case xsd.EmptyContent:
		// A valid instance has no character or element child here
		// (cvc-complex-type clause 1.1), so the sequence of no particle reads
		// none.
		index.store(key, b)
		b.content = &sequence{p: run.p, children: map[xsd.QName]int{}}
	case xsd.ElementContent:
		content, group, d, err := contentGroup(run.p.schema, c)
		if err != nil {
			return nil, err
		}
		if d != 0 {
			return nil, decline(d, typeSubject)
		}
		// codegen/emit.go's shape: a choice occurring once, in a type with no
		// attribute field, is the type itself unless it has {particles} and one
		// occurrence of it is emptiable (cos-group-emptiable). Every attribute
		// use gives a field or declines, and so does every particle an
		// alternative, so the counts decide before either is bound.
		if group.Compositor() == xsd.CompositorChoice && len(ct.AttributeUses()) == 0 &&
			(len(group.Particles()) == 0 || !run.p.schema.ParticleEmptiable(content)) {
			root := &choiceRoot{}
			index.store(key, root)
			ch, err := bindChoice(run, ct, group.Particles(), names, typeSubject)
			if err != nil {
				return nil, err
			}
			if _, has := ct.AttributeWildcard(); has {
				return nil, decline(constructAttributeWildcard, typeSubject)
			}
			root.sealed, root.wild = ch.sealed, ch.wild
			return root, nil
		}
		index.store(key, b)
		b.content, err = bindContent(run, ct, group, names, typeSubject)
		if err != nil {
			return nil, err
		}
	}
	for _, u := range ct.AttributeUses() {
		a, err := attributeUse(run.p.schema, u)
		if err != nil {
			return nil, err
		}
		a.name = names.Claim(u.DeclarationName().Local)
		b.attributes[u.DeclarationName()] = len(b.attrs)
		b.attrs = append(b.attrs, a)
	}
	if _, has := ct.AttributeWildcard(); has {
		return nil, decline(constructAttributeWildcard, typeSubject)
	}
	return b, nil
}

// store stores u as key's unit, unless index is nil, that of an anonymous
// type bound under no key.
func (index unitIndex[K]) store(key K, u complexUnit) {
	if index != nil {
		index[key] = u
	}
}

// nested returns the complex unit of ct, the {type definition} of el, a
// particle's element declaration, other than xs:anyType, which declField
// binds in its place, or the first decline Decode's doc states for it, item
// 5: the constructs of ct in the order bindComplex states, keyed by el's
// expanded name for a top-level el, by ct's QName for a local el whose {type
// definition} names it, by its groupPath for an inline one declared in a
// model group definition, and under no key for any other inline one, a ct of
// simple content stored under none of these (bindComplex).
func (run *bindRun) nested(el xsd.ElementDeclaration, ct xsd.ComplexType) (complexUnit, error) {
	if el.ScopeVariety() == xsd.ScopeGlobal {
		return bindComplex(run, el, ct, run.elements, el.Name())
	}
	// TypeDefinition and the {scope}'s {parent} are closed sums, so the
	// assertions are STYLE T2's sealed-sum exception; a local declaration's
	// {type definition} is never a SubstitutionGroupHeadTypeRef.
	if ref, named := el.TypeDefinition().(xsd.TypeDefinitionRef); named {
		return bindComplex(run, el, ct, run.types, ref.Name)
	}
	if parent, has := el.Scope().Parent(); has {
		if group, inGroup := parent.(xsd.ModelGroupScopeParent); inGroup {
			return bindComplex(run, el, ct, run.groups, groupPath{group: group.Name, element: el.Name()})
		}
	}
	return bindComplex(run, el, ct, unitIndex[xsd.QName](nil), xsd.QName{})
}

// bindContent returns the elementContent of group, the content sequence or
// choice of ct, its fields named from names, or the first decline Decode's
// doc states for its particles, item 5, typeSubject naming the type.
func bindContent(run *bindRun, ct xsd.ComplexType, group xsd.ModelGroup, names *goname.Namer, typeSubject string) (elementContent, error) {
	if group.Compositor() == xsd.CompositorChoice {
		return bindChoice(run, ct, group.Particles(), names, typeSubject)
	}
	return bindSequence(run, ct, group.Particles(), names, typeSubject)
}

// simpleValue is simple content as the struct codegen emits holds it: the
// value field, the one field ahead of the attribute fields, named from the
// local name value, which holds V, the Go type codegen gives the {content
// type}'s {simple type definition}, never a pointer to it (codegen/doc.go "#
// Complex types").
type simpleValue struct {
	p     *pipeline
	value field
}

// simpleValueOf returns the simpleValue of st, the {simple type definition}
// of the simple content of decl's {type definition}, which typeSubject names,
// its field named from names, or the decline simpleDecline returns for st,
// item 5, naming st after typeSubject (contentSubject). Its value is read as
// a simple-typed element's is, from the element's ·normalized value· under st
// (§3.4.4.2 cvc-complex-type clause 1.2, §3.1.4 key-nv), or, for an element
// with no character or element child, decl's {value constraint}'s {lexical
// form} (§3.3.4.3 cvc-elt clause 5.1.2), the supplied value naming decl.
func simpleValueOf(p *pipeline, decl xsd.ElementDeclaration, st *xsd.SimpleType, typeSubject string, names *goname.Namer) (*simpleValue, error) {
	subject := contentSubject(typeSubject, st)
	g, err := simpleDecline(p.schema, st, subject)
	if err != nil {
		return nil, err
	}
	s := newScalar(g, st, subject, xsd.ValueConstraint{}, false)
	vc, has := decl.ValueConstraint()
	s.supplied = supply(vc, has, declSubject(decl))
	return &simpleValue{p: p, value: field{name: names.Claim("value"), shape: shapeOne, arm: scalarArm{value: valueBare, scalar: s}}}, nil
}

// contentSubject names, after typeSubject, st, the {simple type definition}
// of that type's simple content, and st's schema Loc.
func contentSubject(typeSubject string, st *xsd.SimpleType) string {
	if st.Name() == (xsd.QName{}) {
		return fmt.Sprintf("%s, through its {content type}'s anonymous {simple type definition}, a type defined at %s", typeSubject, st.Loc())
	}
	return fmt.Sprintf("%s, through its {content type}'s {simple type definition} %s, a type defined at %s", typeSubject, st.Name(), st.Loc())
}

// width is the value field.
func (s *simpleValue) width() int { return 1 }

// check holds t's first field to the value field, and returns the
// boundValue that sets it.
func (s *simpleValue) check(t reflect.Type, cx *checkContext) (contentFiller, error) {
	read, err := s.value.check(t, 0, cx)
	if err != nil {
		return nil, err
	}
	return boundValue{value: s, read: read}, nil
}

// boundValue is simple content whose check held, read the value field's
// elementReader.
type boundValue struct {
	value *simpleValue
	read  elementReader
}

// fill sets the value field of out, the struct, from root's character
// children, or its supplied value, as elementValue reads them.
func (b boundValue) fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error {
	v, err := b.read.read(b.value.p, r, root)
	if err != nil {
		return err
	}
	b.value.value.shape.store(b.value.p, out.Field(0), v)
	return nil
}

// sequence is a sequence's particles as the struct codegen emits holds them:
// one field per particle, in particle order.
type sequence struct {
	p      *pipeline
	fields []field
	// children maps a child's expanded name to its field's index; it is a
	// lookup index only and never iterated (STYLE D2).
	children map[xsd.QName]int
	// wild is the sequence's wildcard particle, nil for none.
	wild *wildcardSlot
}

// bindSequence returns the sequence of particles, the content sequence of
// ct, each field named from names in particle order, an element particle's
// by its element's local name and a wildcard particle's as if from local
// name any (codegen/doc.go "# Naming"), or the first decline Decode's doc
// states for them, item 5, typeSubject naming the type: a wildcard particle
// after the sequence's first declines as a wildcard particle.
func bindSequence(run *bindRun, ct xsd.ComplexType, particles []xsd.Particle, names *goname.Namer, typeSubject string) (*sequence, error) {
	s := &sequence{p: run.p, children: map[xsd.QName]int{}}
	for _, p := range particles {
		el, d, err := particleElement(run.p.schema, p)
		if err != nil {
			return nil, err
		}
		if d == constructWildcardParticle && s.wild == nil {
			f, err := wildcardField(p, typeSubject)
			if err != nil {
				return nil, err
			}
			f.name = names.Claim("any")
			s.wild = &wildcardSlot{ct: ct, index: len(s.fields)}
			s.fields = append(s.fields, f)
			continue
		}
		if d != 0 {
			return nil, decline(d, "a particle of "+typeSubject)
		}
		f, takes, err := elementField(run, s.children, el, p.Occurs())
		if err != nil {
			return nil, err
		}
		f.name = names.Claim(el.Name().Local)
		for _, n := range takes {
			s.children[n] = len(s.fields)
		}
		s.fields = append(s.fields, f)
	}
	if _, err := s.wild.matcher(run.p.schema); err != nil {
		return nil, err
	}
	return s, nil
}

// wildcardField returns the field, but for its name, which its caller
// claims, of p, a wildcard particle of the type typeSubject names: codegen's
// AnyElement (rawArm) in occursShape's shape, or occursShape's plain error.
func wildcardField(p xsd.Particle, typeSubject string) (field, error) {
	s, err := occursShape(p.Occurs(), "the wildcard particle of "+typeSubject)
	if err != nil {
		return field{}, err
	}
	return field{shape: s, arm: rawArm{}}, nil
}

// wildcardSlot is the one wildcard particle of a content sequence or choice
// that decodes: ct, the complex type whose {content type} holds it, and
// index, the index of its field in the sequence or of its alternative in the
// choice.
type wildcardSlot struct {
	ct    xsd.ComplexType
	index int
}

// matcher returns a fresh Matcher over w.ct's {content type}, which
// attributes one element's children (eachChild), or nil for a nil w, no
// wildcard particle. The binds call it once all of a content model's
// particles are bound, so that CheckDecode returns what Decode does, and on
// a premise: ContentMatcher's answer depends on ct alone, and declines only
// a {content type} that is not element-only, which contentGroup has declined,
// or one too wide to track, which needs a repeating model group, and the
// particle Decode binds is one sequence or choice occurring once of element
// and wildcard particles, every nested group declined. A decline here is a
// defect of that premise, a plain error and no decline.
func (w *wildcardSlot) matcher(schema *xsd.Schema) (*xsd.Matcher, error) {
	if w == nil {
		return nil, nil
	}
	m, ok := schema.ContentMatcher(w.ct)
	if !ok {
		return nil, fmt.Errorf("codec: xsd.Schema.ContentMatcher declines the complex type defined at %s, whose children Decode attributes through one", w.ct.Loc())
	}
	return m, nil
}

// width is the particle count.
func (s *sequence) width() int { return len(s.fields) }

// check holds t's first fields to s's, in name, order and Go type, and
// returns the boundSequence that sets them.
func (s *sequence) check(t reflect.Type, cx *checkContext) (contentFiller, error) {
	readers := make([]elementReader, len(s.fields))
	for i, want := range s.fields {
		r, err := want.check(t, i, cx)
		if err != nil {
			return nil, err
		}
		readers[i] = r
	}
	return &boundSequence{sequence: s, readers: readers}, nil
}

// boundSequence is a sequence whose fields check held: readers[i] reads the
// element of sequence.fields[i].
type boundSequence struct {
	sequence *sequence
	readers  []elementReader
}

// fill sets, from each child element, the field eachChild attributes it to,
// from the value that field's elementReader reads from r up to the child's
// end tag.
func (b *boundSequence) fill(r *xmltree.Reader, _ *xmltree.StartElement, out reflect.Value) error {
	s := b.sequence
	return eachChild(r, s.p.schema, s.children, s.wild, func(start *xmltree.StartElement, i int) error {
		v, err := b.readers[i].read(s.p, r, start)
		if err != nil {
			return err
		}
		s.fields[i].shape.store(s.p, out.Field(i), v)
		return nil
	})
}

// sealed is a sealed interface as codegen emits it, whose one method is
// unexported, and one branch struct per alternative, in order, each holding
// the one field alts holds for it, as Decode reads them.
type sealed struct {
	p    *pipeline
	alts []field
	// children maps a child's expanded name to its alternative's index in
	// alts; it is a lookup index only and never iterated (STYLE D2).
	children map[xsd.QName]int
}

// choice is a choice occurring once as codegen emits it: a sealed interface
// with one branch struct per alternative, in particle order, each holding the
// one field its alternative's particle gives, its {min occurs} 0 read as 1
// and its {max occurs} kept (codegen/doc.go "# Complex types"). The interface
// is the type itself (*choiceRoot) or the first field of the type's struct
// (*binding), named name.
type choice struct {
	name   string
	sealed *sealed
	// wild is the choice's wildcard alternative, nil for none.
	wild *wildcardSlot
}

// bindChoice returns the choice of particles, the content choice of ct, its
// field named from names by the local name choice before any alternative is
// read (codegen/doc.go "# Particles"), each branch field named as a
// sequence's field is from a fresh Namer, or the first decline Decode's doc
// states for them, item 5, typeSubject naming the type: each alternative's
// as a sequence's particle declines, a wildcard alternative after the
// choice's first among them, then an alternative whose branch struct has an
// earlier one's shape, which no branch type could tell apart: one whose
// branch field may admit a Go type the earlier one's does
// (field.sharesBranch).
func bindChoice(run *bindRun, ct xsd.ComplexType, particles []xsd.Particle, names *goname.Namer, typeSubject string) (*choice, error) {
	c := &choice{name: names.Claim("choice"), sealed: &sealed{p: run.p, children: map[xsd.QName]int{}}}
	alts := c.sealed
	for _, p := range particles {
		el, d, err := particleElement(run.p.schema, p)
		if err != nil {
			return nil, err
		}
		var f field
		var takes []xsd.QName
		var subject string
		switch {
		case d == constructWildcardParticle && c.wild == nil:
			f, err = wildcardField(p, typeSubject)
			if err != nil {
				return nil, err
			}
			f.name = goname.NewNamer().Claim("any")
			c.wild = &wildcardSlot{ct: ct, index: len(alts.alts)}
			subject = "the wildcard particle of " + typeSubject
		case d != 0:
			return nil, decline(d, "a particle of "+typeSubject)
		default:
			f, takes, err = elementField(run, alts.children, el, p.Occurs())
			if err != nil {
				return nil, err
			}
			f.name = goname.NewNamer().Claim(el.Name().Local)
			subject = declSubject(el)
		}
		// A branch's {min occurs} 0 reads as 1: the empty match is the nil
		// interface, so no branch holds an absent value (cos-group-emptiable).
		if f.shape == shapeOptional {
			f.shape = shapeOne
		}
		for _, earlier := range alts.alts {
			if earlier.sharesBranch(f) {
				return nil, decline(constructSharedBranch, subject)
			}
		}
		for _, n := range takes {
			alts.children[n] = len(alts.alts)
		}
		alts.alts = append(alts.alts, f)
	}
	if _, err := c.wild.matcher(run.p.schema); err != nil {
		return nil, err
	}
	return c, nil
}

// width is the one field holding the interface.
func (c *choice) width() int { return 1 }

// check holds t's first field to the one holding c's interface, and returns
// the boundContentChoice that sets it.
func (c *choice) check(t reflect.Type, cx *checkContext) (contentFiller, error) {
	got := t.Field(0)
	if got.Name != c.name || got.Anonymous {
		return nil, fmt.Errorf("%w: field 0 of %s is %s %s where codegen emits %s, a sealed interface", ErrTypeMismatch, t, got.Name, got.Type, c.name)
	}
	bc, err := c.sealed.checkSite(got.Type, fieldAt{owner: t, index: 0}.site(), cx)
	if err != nil {
		return nil, err
	}
	return boundContentChoice{choice: c, bound: bc}, nil
}

// boundContentChoice is a content choice whose check held, bound, the
// contentFiller of the struct's first field.
type boundContentChoice struct {
	choice *choice
	bound  *boundChoice
}

// fill sets the choice's field of out, the struct, from each child element,
// to the alternative eachChild attributes it to.
func (b boundContentChoice) fill(r *xmltree.Reader, _ *xmltree.StartElement, out reflect.Value) error {
	alts := b.choice.sealed
	return eachChild(r, alts.p.schema, alts.children, b.choice.wild, func(start *xmltree.StartElement, i int) error {
		return b.bound.set(r, start, i, out.Field(0))
	})
}

// checkSite holds it, the Go type at the site site names, to c's sealed
// interface, item 6 of Decode's doc: an interface whose one method is
// unexported, whose branch types cx's sets hold as one struct per alternative
// of the alternative's branch shape. It returns c bound to those branch
// types. Neither the interface's nor a branch's name is compared.
func (c *sealed) checkSite(it reflect.Type, site string, cx *checkContext) (*boundChoice, error) {
	if it.Kind() != reflect.Interface {
		return nil, fmt.Errorf("%w: %s is %s, no interface, where codegen emits a sealed interface", ErrTypeMismatch, site, it)
	}
	if it.NumMethod() != 1 || it.Method(0).IsExported() {
		return nil, fmt.Errorf("%w: %s is %s, which is not an interface of one unexported method, codegen's sealed interface", ErrTypeMismatch, site, it)
	}
	branches, supplied := cx.sets.of(it)
	if !supplied && len(c.alts) > 0 {
		return nil, fmt.Errorf("%w: no WithBranches supplies the branches of %s, the interface of %s, where codegen emits %d", ErrTypeMismatch, it, site, len(c.alts))
	}
	if len(branches) != len(c.alts) {
		return nil, fmt.Errorf("%w: WithBranches supplies %d branches of %s where codegen emits %d", ErrTypeMismatch, len(branches), it, len(c.alts))
	}
	// As many branches as alternatives, each matching one no earlier branch
	// matched, leave no entry of matched without its branch type.
	matched := make([]boundBranch, len(c.alts))
	for i, bt := range branches {
		if bt == nil {
			return nil, fmt.Errorf("%w: WithBranches supplies a nil branch of %s", ErrTypeMismatch, it)
		}
		if bt.Kind() != reflect.Struct {
			return nil, fmt.Errorf("%w: WithBranches supplies %s, no struct, as a branch of %s", ErrTypeMismatch, bt, it)
		}
		if slices.Contains(branches[:i], bt) {
			return nil, fmt.Errorf("%w: WithBranches supplies %s twice as a branch of %s", ErrTypeMismatch, bt, it)
		}
		j, read, err := c.alternative(bt, cx)
		if err != nil {
			return nil, err
		}
		if j < 0 || matched[j].t != nil {
			return nil, fmt.Errorf("%w: branch %s of %s has the shape of no branch struct codegen emits that an earlier branch did not match", ErrTypeMismatch, bt, it)
		}
		matched[j] = boundBranch{t: bt, read: read}
	}
	return &boundChoice{sealed: c, branches: matched}, nil
}

// alternative returns the index in c.alts of the alternative whose branch
// struct bt is, with the elementReader of its one field, or -1 for none, or
// that alternative's ErrTypeMismatch error: the first alternative whose
// branch field admits bt's one field (field.admits), which sharesBranch keeps
// alone of those admitting any one Go type; or else, where none admits it,
// the first alternative whose arm is no scalarArm and whose field bt's one
// field is named like, whose check then fails storing no checkContext.bound
// entry (fieldArm.admits). The one alternative selected is the only one
// checked, its check's error the check's. So no check is tried and discarded.
func (c *sealed) alternative(bt reflect.Type, cx *checkContext) (int, elementReader, error) {
	if bt.NumField() != 1 {
		return -1, nil, nil
	}
	got := bt.Field(0)
	j := slices.IndexFunc(c.alts, func(a field) bool { return a.admits(got) })
	if j < 0 {
		// fieldArm is a closed sum, so the assertion is STYLE T2's sealed-sum
		// exception.
		j = slices.IndexFunc(c.alts, func(a field) bool {
			_, scalar := a.arm.(scalarArm)
			return !scalar && a.name == got.Name
		})
	}
	if j < 0 {
		return -1, nil, nil
	}
	read, err := c.alts[j].check(bt, 0, cx)
	return j, read, err
}

// boundChoice is a sealed interface checkSite held: branches[i] is the
// branch of sealed.alts[i], none without its type.
type boundChoice struct {
	sealed   *sealed
	branches []boundBranch
}

// boundBranch is one branch struct t of a choice's interface, read its one
// field's elementReader.
type boundBranch struct {
	t    reflect.Type
	read elementReader
}

// set sets dst, a value of the sealed interface, from the child element
// start, read from r up to its end tag, which its caller attributes to
// alternative i: the branch of that alternative holds the value its field's
// elementReader reads, a repeated alternative's appended to the branch dst
// already holds.
func (bc *boundChoice) set(r *xmltree.Reader, start *xmltree.StartElement, i int, dst reflect.Value) error {
	alt, bb := bc.sealed.alts[i], bc.branches[i]
	branch := reflect.New(bb.t).Elem()
	if !dst.IsNil() {
		// Validation charges a child of a second alternative first
		// (cvc-model-group); this is the defensive check.
		if dst.Elem().Type() != bb.t {
			return fmt.Errorf("codec: %s: element %s matches a second alternative of the choice", start.Loc(), expanded(start.Name()))
		}
		branch.Set(dst.Elem())
	}
	v, err := bb.read.read(bc.sealed.p, r, start)
	if err != nil {
		return err
	}
	alt.shape.store(bc.sealed.p, branch.Field(0), v)
	dst.Set(branch)
	return nil
}

// choiceRoot is the bare sealed interface codegen emits for a complex type,
// a document element's or a particle's element declaration's, whose content
// is a choice and which has no attribute field, as Decode reads it.
type choiceRoot struct {
	sealed *sealed
	// wild is the choice's wildcard alternative, nil for none.
	wild *wildcardSlot
}

// field returns the field of shape s whose V is c's interface: an optional
// one is the bare interface, nil meaning absent, never a pointer to it
// (codegen/doc.go "# Particles").
func (c *choiceRoot) field(s shape) field {
	if s == shapeOptional {
		s = shapeOne
	}
	return field{shape: s, arm: ifaceArm{unit: c}}
}

// check holds t, T itself, to the choice's sealed interface, and returns the
// boundChoiceRoot that fills it.
func (c *choiceRoot) check(t reflect.Type, cx *checkContext) (filler, error) {
	return c.checkAt(t, "T", cx)
}

// checkAt holds t, the Go type at the site site names, to the choice's sealed
// interface, and returns the boundChoiceRoot that fills it, the one cx
// already bound for c and t where it holds one.
func (c *choiceRoot) checkAt(t reflect.Type, site string, cx *checkContext) (filler, error) {
	key := boundKey{unit: c, t: t}
	if f, ok := cx.bound[key]; ok {
		return f, nil
	}
	bc := &boundChoiceRoot{root: c}
	cx.bound[key] = bc
	choice, err := c.sealed.checkSite(t, site, cx)
	if err != nil {
		return nil, err
	}
	bc.choice = choice
	return bc, nil
}

// boundChoiceRoot is root whose check held, choice its bound interface.
type boundChoiceRoot struct {
	root   *choiceRoot
	choice *boundChoice
}

// fill sets out, a value of the interface, as a filler does, from each child
// element, to the alternative eachChild attributes it to. An empty match
// leaves out nil.
func (c *boundChoiceRoot) fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error {
	if err := checkXsiType(root); err != nil {
		return err
	}
	if err := noAttributes(root); err != nil {
		return err
	}
	alts := c.root.sealed
	return eachChild(r, alts.p.schema, alts.children, c.root.wild, func(start *xmltree.StartElement, i int) error {
		return c.choice.set(r, start, i, out)
	})
}

// bindSimpleRoot returns the simpleRoot of decl, a top-level element
// declaration whose {type definition} is st, or the decline Decode's doc
// states for st, item 5, subject naming st.
func bindSimpleRoot(p *pipeline, decl xsd.ElementDeclaration, st *xsd.SimpleType, subject string) (binder, error) {
	g, err := simpleDecline(p.schema, st, subject)
	if err != nil {
		return nil, err
	}
	vc, has := decl.ValueConstraint()
	return &simpleRoot{p: p, scalar: newScalar(g, st, declSubject(decl), vc, has)}, nil
}

// declarationDecline returns the construct of an element declaration's own
// properties Decode declines, in the order its doc states, or 0.
func declarationDecline(decl xsd.ElementDeclaration) construct {
	if _, has := decl.TypeTable(); has {
		return constructTypeTable
	}
	if decl.Nillable() {
		return constructNillable
	}
	return 0
}

// contentGroup returns the {particle} of content, a complex type's
// element-only or mixed content, its {term} an inline sequence or choice or
// a <group ref>, and its model group, the inline one or the {model group} of
// the definition the ref names (§3.7.2 declare-namedModelGroup), or the
// construct Decode declines in their place, in its doc's item 5 order. A
// <group ref> naming no definition is modelGroupTerm's plain error.
func contentGroup(schema *xsd.Schema, content xsd.ElementContent) (xsd.Particle, xsd.ModelGroup, construct, error) {
	if content.Mixed {
		return xsd.Particle{}, xsd.ModelGroup{}, constructMixedContent, nil
	}
	if content.OpenContent != nil {
		return xsd.Particle{}, xsd.ModelGroup{}, constructOpenContent, nil
	}
	group, _, err := modelGroupTerm(schema, content.Particle.Term())
	if err != nil {
		return xsd.Particle{}, xsd.ModelGroup{}, 0, err
	}
	occurs := content.Particle.Occurs()
	upper, bounded := occurs.Max()
	once := occurs.Min() == 1 && bounded && upper == 1
	switch group.Compositor() {
	case xsd.CompositorChoice:
		if !once {
			return xsd.Particle{}, xsd.ModelGroup{}, constructChoiceOccurs, nil
		}
		return content.Particle, group, 0, nil
	case xsd.CompositorAll:
		return xsd.Particle{}, xsd.ModelGroup{}, constructAllContent, nil
	case xsd.CompositorSequence:
		if !once {
			return xsd.Particle{}, xsd.ModelGroup{}, constructSequenceOccurs, nil
		}
		return content.Particle, group, 0, nil
	}
	// The {particle} of element-only content is a model group (§3.4.2.3.3), so
	// the zero ModelGroup, no compositor, is a component no schema document
	// yields; decline it with the nearest construct rather than read it.
	return xsd.Particle{}, xsd.ModelGroup{}, constructNestedGroup, nil
}

// modelGroupTerm returns the model group term holds and true: an inline
// sequence, choice or all group, or the {model group} of the top-level model
// group definition a <group ref> names (§3.7.2 declare-namedModelGroup),
// followed through [xsd.Schema.ModelGroup]. It returns false for an element
// declaration, an <element ref> or a wildcard. A <group ref> naming no
// definition is a plain error, never a decline: Finalize resolves every one
// ([xsd.ModelGroupRef]). The term is a closed sum, so the switch is STYLE
// T2's sealed-sum exception.
func modelGroupTerm(schema *xsd.Schema, term xsd.TermOrRef) (xsd.ModelGroup, bool, error) {
	switch term := term.(type) {
	case xsd.ModelGroupRef:
		def, ok := schema.ModelGroup(term.Name)
		if !ok {
			return xsd.ModelGroup{}, false, fmt.Errorf("codec: the <group ref> particle to %s does not resolve", term.Name)
		}
		return def.ModelGroup(), true, nil
	case xsd.ResolvedTerm:
		group, isGroup := term.Term.(xsd.ModelGroup)
		return group, isGroup, nil
	}
	return xsd.ModelGroup{}, false, nil
}

// particleElement returns the element declaration p's term holds, a local
// one or the top-level one an <element ref> names (§3.3.2.4
// ref.elt.global), followed through [xsd.Schema.Element], or the construct
// Decode declines in its place: a model group, inline or through <group ref>,
// is a nested model group particle, and a wildcard a wildcard particle,
// which bindSequence and bindChoice bind in its place where it is their
// content model's first. An <element ref> naming no top-level declaration is
// a plain error.
func particleElement(schema *xsd.Schema, p xsd.Particle) (xsd.ElementDeclaration, construct, error) {
	switch term := p.Term().(type) {
	case xsd.ElementDeclarationRef:
		decl, ok := schema.Element(term.Name)
		if !ok {
			return xsd.ElementDeclaration{}, 0, fmt.Errorf("codec: the <element ref> particle to %s does not resolve", term.Name)
		}
		return decl, 0, nil
	case xsd.ResolvedTerm:
		switch inner := term.Term.(type) {
		case xsd.ElementDeclaration:
			return inner, 0, nil
		case xsd.Wildcard:
			return xsd.ElementDeclaration{}, constructWildcardParticle, nil
		}
	}
	return xsd.ElementDeclaration{}, constructNestedGroup, nil
}

// elementField returns the field, but for its name, which its caller claims,
// of the element declaration el in a particle occurring as occurs, and the
// expanded names of the instance children that field takes, or as the error
// the *DeclineError Decode's doc states for it, item 5, or the plain error
// that item states in a decline's place, in the order that doc states. For a
// top-level el, which an <element ref> named, whose substitution group
// codegen gives a sealed interface (goshape.SubstitutionSum), it is groupField's.
// Otherwise it is declField's for el, the field taking el's name alone, and
// then an expanded name an earlier particle of its model group shares,
// children indexing the earlier particles' names, declines; a local el of
// simple type's field holds the Go type codegen gives its type, and a
// top-level one's the named type codegen emits for el over it (valueNamed).
func elementField(run *bindRun, children map[xsd.QName]int, el xsd.ElementDeclaration, occurs xsd.Occurs) (field, []xsd.QName, error) {
	if members, sum := goshape.SubstitutionSum(run.p.schema, el); sum {
		return groupField(run, children, el, members, occurs)
	}
	value := valueBare
	if el.ScopeVariety() == xsd.ScopeGlobal {
		value = valueNamed
	}
	f, err := declField(run, el, value, func() (shape, error) {
		return particleShape(children, []xsd.ElementDeclaration{el}, el, occurs)
	})
	if err != nil {
		return field{}, nil, err
	}
	return f, []xsd.QName{el.Name()}, nil
}

// groupField returns the field, but for its name, of an <element ref>
// particle to head occurring as occurs, where codegen gives it the sealed
// interface over head's substitution group, members being
// goshape.SubstitutionSum's for head, and the expanded names of members, the
// instance children that field takes, in group order (codegen/doc.go "#
// Substitution groups"). The interface has one branch per member, in group
// order, holding the one field declField gives the member, of the member's
// own type even where the member heads a group of its own, and occurring
// once. A head with {abstract} true is no member, so its own properties and
// type are not examined, and with no member the interface has no branch. The
// first of these in this order declines: each member in group order, by
// declField's constructs and then by a branch field that may admit a Go type
// an earlier member's does (field.sharesBranch); then a member's expanded
// name an earlier particle of the model group shares, children indexing the
// earlier particles' names, the first in group order; then occursShape's
// plain error. The field's shape is occursShape's, an optional one the bare
// interface, nil meaning absent, as choiceRoot.field's is.
func groupField(run *bindRun, children map[xsd.QName]int, head xsd.ElementDeclaration, members []xsd.ElementDeclaration, occurs xsd.Occurs) (field, []xsd.QName, error) {
	group := &sealed{p: run.p, children: map[xsd.QName]int{}}
	names := make([]xsd.QName, 0, len(members))
	for _, m := range members {
		f, err := declField(run, m, valueNamed, func() (shape, error) { return shapeOne, nil })
		if err != nil {
			return field{}, nil, err
		}
		f.name = goname.NewNamer().Claim(m.Name().Local)
		for _, earlier := range group.alts {
			if earlier.sharesBranch(f) {
				return field{}, nil, decline(constructSharedBranch, declSubject(m))
			}
		}
		group.children[m.Name()] = len(group.alts)
		group.alts = append(group.alts, f)
		names = append(names, m.Name())
	}
	s, err := particleShape(children, members, head, occurs)
	if err != nil {
		return field{}, nil, err
	}
	if s == shapeOptional {
		s = shapeOne
	}
	return field{shape: s, arm: groupArm{sealed: group}}, names, nil
}

// declField returns the field, but for its name, of the element declaration
// el, its V by el's own type, or as the error the *DeclineError Decode's doc
// states for it, item 5, naming el, or the plain error that item states in a
// decline's place, in the order that doc states: el's own properties, then
// its type, then shapeOf's error, shapeOf returning the field's shape. It
// never examines el's substitution group. A field of simple type holds the Go
// type codegen gives el's type, or the named type over it (value); one of
// type xs:anyType itself holds codegen's AnyType, or the named type over it
// (value), the element's content raw (anyTypeArm), its {value constraint}
// never read, as Decode's doc states; and one of any other complex type
// holds the type codegen emits for that type, the unit run.nested binds. An
// optional field of a union, whose Go type is a sealed interface
// (simpleGo.sealed), is the bare interface, nil meaning absent, never a
// pointer to it (codegen/doc.go "# Particles").
func declField(run *bindRun, el xsd.ElementDeclaration, value valueType, shapeOf func() (shape, error)) (field, error) {
	schema := run.p.schema
	subject := declSubject(el)
	if d := declarationDecline(el); d != 0 {
		return field{}, decline(d, subject)
	}
	td, ok := schema.ResolvedType(el.TypeDefinition())
	if !ok {
		return field{}, fmt.Errorf("codec: the {type definition} of %s does not resolve", subject)
	}
	// TypeDefinition is a closed sum of the two, so the assertion is STYLE T2's
	// sealed-sum exception.
	st, simple := td.(*xsd.SimpleType)
	if !simple {
		ct, _ := td.(xsd.ComplexType)
		// xs:anyType is known by name, as codegen's isAnyType knows it, so a
		// type derived from it binds as its own complex type.
		if ct.Name() == (xsd.QName{Space: xsd.XMLSchemaNS, Local: "anyType"}) {
			s, err := shapeOf()
			if err != nil {
				return field{}, err
			}
			return field{shape: s, arm: anyTypeArm{value: value}}, nil
		}
		u, err := run.nested(el, ct)
		if err != nil {
			return field{}, err
		}
		s, err := shapeOf()
		if err != nil {
			return field{}, err
		}
		return u.field(s), nil
	}
	g, err := simpleDecline(schema, st, subject)
	if err != nil {
		return field{}, err
	}
	s, err := shapeOf()
	if err != nil {
		return field{}, err
	}
	if s == shapeOptional && g.sealed() {
		s = shapeOne
	}
	vc, has := el.ValueConstraint()
	return field{shape: s, arm: scalarArm{value: value, scalar: newScalar(g, st, subject, vc, has)}}, nil
}

// particleShape returns the shape of the field of el's particle occurring as
// occurs, which takes the instance children named by decls, or the decline of
// the first of decls whose expanded name an earlier particle shares, children
// indexing the earlier particles' names, or occursShape's plain error, in
// that order.
func particleShape(children map[xsd.QName]int, decls []xsd.ElementDeclaration, el xsd.ElementDeclaration, occurs xsd.Occurs) (shape, error) {
	for _, d := range decls {
		if _, shared := children[d.Name()]; shared {
			return 0, decline(constructSharedName, declSubject(d))
		}
	}
	return occursShape(occurs, "the particle of "+declSubject(el))
}

// attributeUse returns the attribute field, but for its name, which its
// caller claims, of attribute use u, or as the error the *DeclineError
// Decode's doc states for it, item 5, naming its attribute declaration, or a
// plain error. The field's shape is codegen's, as the package doc's "# What
// decodes" states, a pointer exactly for a use neither required nor with an
// effective value constraint whose Go type is no sealed interface
// (simpleGo.sealed), and needs no xsi: test here: an xsi: use declines before
// its shape is picked.
func attributeUse(schema *xsd.Schema, u xsd.AttributeUse) (attribute, error) {
	decl, ok := schema.ResolvedAttributeDeclaration(u)
	if !ok {
		return attribute{}, fmt.Errorf("codec: the {attribute declaration} of attribute use %s does not resolve", u.DeclarationName())
	}
	subject := fmt.Sprintf("attribute %s declared at %s", decl.Name(), decl.Loc())
	if decl.Name().Space == xsd.XMLSchemaInstanceNS {
		return attribute{}, decline(constructXsiAttribute, subject)
	}
	st, ok := schema.ResolvedSimpleType(decl.TypeDefinition())
	if !ok {
		return attribute{}, fmt.Errorf("codec: the {type definition} of %s does not resolve", subject)
	}
	g, err := simpleDecline(schema, st, subject)
	if err != nil {
		return attribute{}, err
	}
	shape := shapeOptional
	vc, has := schema.EffectiveValueConstraint(u)
	if u.Required() || has || g.sealed() {
		shape = shapeOne
	}
	return attribute{shape: shape, scalar: newScalar(g, st, subject, vc, has)}, nil
}

// simpleDecline returns the Go type codegen gives the simple type st of the
// item subject names (codegen/doc.go "# Simple types"), or else an error:
// the *DeclineError Decode's doc states for st, item 5, subject naming it,
// or a plain error. For an atomic st the decline is atomicDecline's; for a
// union, unionDecline's; for a list, that of its {item type definition}
// (Structures §3.16.2.3, a restriction of a list taking its base's), which
// cos-st-restricts clause 2.1 (checkListGraph) holds to an atomic or union
// type, so its construct always answers, the detail naming the item after
// subject. {variety} is a closed sum, so the assertions are STYLE T2's
// sealed-sum exception.
func simpleDecline(r xsd.TypeResolver, st *xsd.SimpleType, subject string) (simpleGo, error) {
	variety, err := st.Variety(r)
	if err != nil {
		return simpleGo{}, err
	}
	if _, isUnion := variety.(xsd.Union); isUnion {
		u, err := unionDecline(r, st, subject)
		if err != nil {
			return simpleGo{}, err
		}
		return simpleGo{elem: u}, nil
	}
	if _, isList := variety.(xsd.List); !isList {
		atom, d, err := atomicDecline(r, st)
		if err != nil {
			return simpleGo{}, err
		}
		if d != 0 {
			return simpleGo{}, decline(d, subject)
		}
		return simpleGo{elem: atom}, nil
	}
	item, err := st.Item(r)
	if err != nil {
		return simpleGo{}, err
	}
	if item == nil {
		return simpleGo{}, fmt.Errorf("codec: the list type of %s has no {item type definition}", subject)
	}
	itemVariety, err := item.Variety(r)
	if err != nil {
		return simpleGo{}, err
	}
	if _, isUnion := itemVariety.(xsd.Union); isUnion {
		u, err := unionDecline(r, item, itemSubject(subject, item))
		if err != nil {
			return simpleGo{}, err
		}
		return simpleGo{elem: u, item: item}, nil
	}
	atom, d, err := atomicDecline(r, item)
	if err != nil {
		return simpleGo{}, err
	}
	if d != 0 {
		return simpleGo{}, decline(d, itemSubject(subject, item))
	}
	return simpleGo{elem: atom, item: item}, nil
}

// itemSubject names, after subject, item, the {item type definition} of the
// list type whose item declines, and item's schema Loc.
func itemSubject(subject string, item *xsd.SimpleType) string {
	if item.Name() == (xsd.QName{}) {
		return fmt.Sprintf("%s, through the list type's anonymous {item type definition}, a type defined at %s", subject, item.Loc())
	}
	return fmt.Sprintf("%s, through the list type's {item type definition} %s, a type defined at %s", subject, item.Name(), item.Loc())
}

// atomicDecline returns the atomic Go type codegen gives the simple type st,
// an atomic type whose {primitive type definition} is one of primitiveAtomics' or
// datatypeAtomics', as codegen/doc.go "# Simple types" picks it: from st's {base
// type definition} chain (chainAtomic), which meets a date, time or duration
// primitive, xs:QName or xs:precisionDecimal at its top, failing that from its
// primitive. Or else it returns the construct Decode declines st for: a special
// type's, or that of its primitive. A union or list st is a plain error:
// simpleDecline routes a union to unionDecline and a list to its {item type
// definition}, which cos-st-restricts clause 2.1 holds to no list, routing a
// union item to unionDecline too, whose basic members are no unions. {variety}
// is a closed sum, so the switch is STYLE T2's sealed-sum exception.
func atomicDecline(r xsd.TypeResolver, st *xsd.SimpleType) (atomic, construct, error) {
	variety, err := st.Variety(r)
	if err != nil {
		return 0, 0, err
	}
	switch variety.(type) {
	case xsd.Union:
		return 0, 0, fmt.Errorf("codec: the simple type defined at %s is a union where an atomic type is required", st.Loc())
	case xsd.List:
		return 0, 0, fmt.Errorf("codec: the simple type defined at %s is a list where an atomic or union type is required, as a list's {item type definition} (cos-st-restricts clause 2.1)", st.Loc())
	}
	primitive, err := st.Primitive(r)
	if err != nil {
		return 0, 0, err
	}
	if primitive == nil {
		// xs:anySimpleType, of absent {variety}, and xs:anyAtomicType.
		return 0, constructSpecial, nil
	}
	atom, ok, err := chainAtomic(r, st)
	if err != nil {
		return 0, 0, err
	}
	if ok {
		return atom, 0, nil
	}
	if atom, ok := primitiveAtomics[primitive.Name()]; ok {
		return atom, 0, nil
	}
	return 0, constructOtherPrimitive, nil
}

// occursShape returns the shape codegen gives the field of the particle
// subject names, occurring as occurs, by codegen's package doc "# Particles"
// pointer and slice rules. Decode's check of T is what holds the two to one
// answer.
func occursShape(occurs xsd.Occurs, subject string) (shape, error) {
	upper, bounded := occurs.Max()
	switch {
	case !bounded || upper > 1:
		return shapeRepeated, nil
	case upper == 0:
		return 0, fmt.Errorf("codec: %s has {max occurs} 0, for which codegen emits no type", subject)
	case occurs.Min() == 0:
		return shapeOptional, nil
	}
	return shapeOne, nil
}

// check returns the boundBinding that fills t when t is the struct codegen
// emits for b, the one cx already bound for b and t where it holds one, and
// otherwise the ErrTypeMismatch error Decode's doc states, item 6. Its fields
// are compared, never its name.
func (b *binding) check(t reflect.Type, cx *checkContext) (filler, error) {
	key := boundKey{unit: b, t: t}
	if f, ok := cx.bound[key]; ok {
		return f, nil
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w: %s is no struct", ErrTypeMismatch, t)
	}
	width := b.content.width()
	if t.NumField() != width+len(b.attrs) {
		return nil, fmt.Errorf("%w: %s has %d fields where codegen emits %d", ErrTypeMismatch, t, t.NumField(), width+len(b.attrs))
	}
	bb := &boundBinding{binding: b}
	cx.bound[key] = bb
	cf, err := b.content.check(t, cx)
	if err != nil {
		return nil, err
	}
	for i, want := range b.attrs {
		if _, err := want.field().check(t, width+i, cx); err != nil {
			return nil, err
		}
	}
	bb.content = cf
	return bb, nil
}

// boundBinding is a binding whose T check held, content its content's
// contentFiller.
type boundBinding struct {
	binding *binding
	content contentFiller
}

// fill sets out's fields as a filler does, in the order Decode's doc states
// for items 7 and 8: root's xsi:type, its present attributes in start-tag
// order, the supplied values of the absent ones in attribute-use order, then
// its content, its value field's value or its children. An attribute field
// with a supplied value holds it unless its attribute is present, and its
// supplied value is then never decoded.
func (b *boundBinding) fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error {
	if err := checkXsiType(root); err != nil {
		return err
	}
	bd := b.binding
	width := bd.content.width()
	present := make([]bool, len(bd.attrs))
	for _, a := range root.Attributes() {
		i, ok := bd.attributes[expanded(a.Name())]
		if !ok {
			if a.Name().Space() != xsd.XMLSchemaInstanceNS {
				return unattributed(a)
			}
			continue
		}
		v, err := bd.attrs[i].decode(bd.p, a.Value(), attributeSource{a: a, owner: root})
		if err != nil {
			return err
		}
		bd.attrs[i].shape.store(bd.p, out.Field(width+i), v)
		present[i] = true
	}
	for i, f := range bd.attrs {
		if present[i] || f.supplied == nil {
			continue
		}
		v, err := f.decode(bd.p, f.supplied.vc.LexicalForm(), suppliedSource{start: root, supplied: f.supplied})
		if err != nil {
			return err
		}
		f.shape.store(bd.p, out.Field(width+i), v)
	}
	return b.content.fill(r, root, out)
}

// eachChild reads r's tokens up to the end tag of the element whose start tag
// r has just returned, and calls child for each child element's start tag
// with the index of the field or alternative it is attributed to, which reads
// r up to that child's end tag. The child carries no xsi:type, or eachChild
// returns the xsi:type decline Decode's doc states, item 7, unless it is
// attributed to the wildcard particle w, whose match is read raw. With no
// wildcard particle, a nil w, the child is attributed by its expanded name to
// children's index; with one, by one fresh Matcher of w's, fed every child
// in document order (§3.4.4.4 key-att-to), which puts each name to the
// element particles before the wildcard (cvc-accept; §3.8.6.4 cos-nonambig):
// a child attributed to an element declaration to its expanded name's index
// in children, the declaration being a substitution group's head where the
// child is a member, and one attributed to the wildcard to w.index. Decode
// validated the instance first, so every child is attributed, and to a
// particle; the plain error unparticled returns where one is not is a
// defect of Decode's.
func eachChild(r *xmltree.Reader, schema *xsd.Schema, children map[xsd.QName]int, w *wildcardSlot, child func(start *xmltree.StartElement, i int) error) error {
	m, err := w.matcher(schema)
	if err != nil {
		return err
	}
	for {
		tok, err := r.Token()
		if err != nil {
			return err
		}
		switch tok := tok.(type) {
		case *xmltree.EndElement:
			return nil
		case *xmltree.StartElement:
			i, attributed := attributeChild(m, w, children, tok)
			if wild := attributed && w != nil && i == w.index; !wild {
				if err := checkXsiType(tok); err != nil {
					return err
				}
			}
			if !attributed {
				return unparticled(tok)
			}
			if err := child(tok, i); err != nil {
				return err
			}
		}
	}
}

// attributeChild returns the index in children, or w's, of the field or
// alternative that the child element start is attributed to, as eachChild
// states, m being w's Matcher over the children before start, nil for a nil
// w, and false where it is attributed to none, or to no particle, an {open
// content}. Attribution is a closed sum, so the switch is STYLE T2's
// sealed-sum exception.
func attributeChild(m *xsd.Matcher, w *wildcardSlot, children map[xsd.QName]int, start *xmltree.StartElement) (int, bool) {
	name := expanded(start.Name())
	if w != nil {
		a, _ := m.Next(name)
		if _, wild := a.(xsd.Wildcard); wild {
			return w.index, true
		}
		if _, element := a.(xsd.ElementDeclaration); !element {
			return 0, false
		}
	}
	i, ok := children[name]
	return i, ok
}

// unparticled is the error for the child element start when no particle of
// its parent's content attributes it.
func unparticled(start *xmltree.StartElement) error {
	return fmt.Errorf("codec: %s: element %s is attributed to no particle", start.Loc(), expanded(start.Name()))
}

// elementValue reads the character children of the element start from r up
// to its end tag and returns s's value of their characters, or of s's
// supplied {lexical form} when it is not nil and there are none, the value
// the element's {value constraint} supplies (cvc-elt clause 5.1), as Decode's
// doc states.
func elementValue(r *xmltree.Reader, start *xmltree.StartElement, p *pipeline, s scalar) (reflect.Value, error) {
	var text bytes.Buffer
	var first *xmltree.CharData
	for {
		tok, err := r.Token()
		if err != nil {
			return reflect.Value{}, err
		}
		switch tok := tok.(type) {
		case *xmltree.CharData:
			if first == nil {
				first = tok
			}
			text.WriteString(tok.Data())
		case *xmltree.StartElement:
			return reflect.Value{}, fmt.Errorf("codec: %s: element %s of simple type or simple content holds element %s", tok.Loc(), expanded(start.Name()), expanded(tok.Name()))
		case *xmltree.EndElement:
			if s.supplied != nil && text.Len() == 0 {
				return s.decode(p, s.supplied.vc.LexicalForm(), suppliedSource{start: start, supplied: s.supplied})
			}
			return s.decode(p, text.String(), contentSource{start: start, first: first})
		}
	}
}

// simpleRoot is the type codegen emits for a document element declaration
// whose {type definition} is a simple type Decode decodes, as Decode reads
// it: a type over the Go type codegen gives that simple type (scalar.goType).
type simpleRoot struct {
	p *pipeline
	scalar
}

// check returns s, which fills t, when t is a type over the Go type codegen
// gives s's simple type (scalar.underlies) and, for a PrecisionDecimal or a
// union, cx holds the sets of the interfaces it holds (scalar.holdSets), and
// otherwise the ErrTypeMismatch error Decode's doc states, item 6. t's name is
// never compared.
func (s *simpleRoot) check(t reflect.Type, cx *checkContext) (filler, error) {
	if !s.underlies(t) {
		return nil, fmt.Errorf("%w: the underlying type of %s is not %s", ErrTypeMismatch, t, s.spell())
	}
	if err := s.holdSets(t, "T", cx); err != nil {
		return nil, err
	}
	return s, nil
}

// fill sets out, whose underlying type is the Go type codegen gives s's
// simple type, as a filler does.
func (s *simpleRoot) fill(r *xmltree.Reader, root *xmltree.StartElement, out reflect.Value) error {
	if err := checkXsiType(root); err != nil {
		return err
	}
	if err := noAttributes(root); err != nil {
		return err
	}
	v, err := elementValue(r, root, s.p, s.scalar)
	if err != nil {
		return err
	}
	shapeOne.store(s.p, out, v)
	return nil
}

// noAttributes returns the error for the first attribute of the element root
// outside the xsi namespace, for a type with no attribute use.
func noAttributes(root *xmltree.StartElement) error {
	for _, a := range root.Attributes() {
		if a.Name().Space() != xsd.XMLSchemaInstanceNS {
			return unattributed(a)
		}
	}
	return nil
}

// unattributed is the error for the attribute a of an element when no
// attribute use of its declaration's type attributes it.
func unattributed(a xmltree.Attribute) error {
	return fmt.Errorf("codec: %s: attribute %s is attributed to no attribute use", a.Loc(), expanded(a.Name()))
}

// checkXsiType returns the xsi:type decline for start when it carries one.
// It matches the attribute by expanded name alone: no QName is resolved.
func checkXsiType(start *xmltree.StartElement) error {
	for _, a := range start.Attributes() {
		if a.Name().Space() == xsd.XMLSchemaInstanceNS && a.Name().Local() == "type" {
			return decline(constructXsiType, fmt.Sprintf("element %s with xsi:type %q", expanded(start.Name()), a.Value())).at(start.Loc())
		}
	}
	return nil
}

// declSubject names the element declaration decl and its schema Loc.
func declSubject(decl xsd.ElementDeclaration) string {
	return fmt.Sprintf("element %s declared at %s", decl.Name(), decl.Loc())
}

// expanded returns the expanded name of an instance name.
func expanded(n xmltree.Name) xsd.QName {
	return xsd.QName{Space: n.Space(), Local: n.Local()}
}
