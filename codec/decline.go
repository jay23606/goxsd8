package codec

import (
	"errors"
	"strconv"

	"github.com/kud360/goxsd8/xsderr"
)

// A DeclineError is a construct [Decode] does not decode; it wraps
// errors.ErrUnsupported, and Decode's and [CheckDecode]'s docs state when
// each returns one.
type DeclineError struct {
	construct construct
	// loc is the instance Loc the decline is charged to, nil for none.
	loc *xsderr.Loc
	// detail is the text Error's doc states after the construct.
	detail string
}

// Construct returns the construct the decline declines.
func (e *DeclineError) Construct() Construct { return Construct{e.construct} }

// Loc returns the instance Loc of the element the decline is charged to: the
// document element for a construct [Decode]'s item 5 finds, of its declaration or type
// or of a declaration or type its particles reach at any depth, and the element carrying
// it for xsi:type, and true. For a decline [CheckDecode] returns, which reads no
// instance, it returns the zero Loc and false.
func (e *DeclineError) Loc() (xsderr.Loc, bool) {
	if e.loc == nil {
		return xsderr.Loc{}, false
	}
	return *e.loc, true
}

// Error names the instance Loc, where Loc reports one, and the construct,
// then the declining declaration, the document element's, a particle's
// element declaration's, local or a top-level one an <element ref> names, or
// a member of that one's substitution group, at any depth, or an attribute's,
// and, for a construct of a complex type, the document element's or one a
// particle's element declaration has at any depth, or of the document
// element's simple type, that type labelled as its declaration's {type
// definition}, with no path from the document element, and, for a construct of
// a list type's {item type definition}, that item, by its QName where it has
// one, each with its schema Loc, which is the zero Loc for a built-in
// component; for xsi:type, it names the element and the type the xsi:type
// names.
func (e *DeclineError) Error() string {
	text := "codec does not decode " + e.construct.name() + ": " + e.detail +
		"; a decoder refusal, not a schema or instance fault"
	if e.loc == nil {
		return text
	}
	return e.loc.String() + ": " + text
}

// Unwrap returns errors.ErrUnsupported.
func (e *DeclineError) Unwrap() error { return errors.ErrUnsupported }

// A Construct is a construct Decode declines: two declines of the same
// construct have equal Constructs; the zero Construct names none. A Construct
// is comparable: == reports whether two name the same construct, and it may be
// used as a map key. The package exports no Construct value, because a
// release that lifts a construct stops returning its Construct; compare a
// Construct only with another obtained from [DeclineError.Construct]. The
// package doc's "What decodes" section states what declines.
type Construct struct{ c construct }

// String returns the construct's name for display, distinct for each
// Construct; it is not the decline's message. The names are not stable across
// releases: a release may rename a construct, split one into several, or stop
// returning a name once Decode covers its construct, and no name is reserved.
// To tell constructs apart, use the Constructs themselves, as [Construct]
// states, never a name.
func (c Construct) String() string { return c.c.name() }

// construct is the closed set of constructs Decode declines, one per
// declining site. The zero value is invalid (STYLE T1).
type construct uint8

// The construct values, in the order Decode finds them: the document
// element's declaration, its type, its content, each particle in order, its
// attribute uses in order, its attribute wildcard, and the instance. A
// particle's complex type, examined at the particle, declines by the same
// constructs from constructSimpleContent on, and by
// constructAbstractTypeElement, which sits with the particle's though the
// document element's type declines by it too, ahead of its content. The three
// of a simple type, constructUnion to constructOtherPrimitive, sit with the
// particle's, though a document element of simple type, an attribute use, and
// a list type, by its {item type definition}, decline by them too.
const (
	constructTypeTable construct = iota + 1
	constructNillable
	constructSimpleContent
	constructMixedContent
	constructOpenContent
	constructGroupRefContent
	constructChoiceOccurs
	constructAllContent
	constructSequenceOccurs
	constructNestedGroup
	constructWildcardParticle // a wildcard particle after its sequence's or choice's first
	constructAbstractTypeElement
	constructSharedName
	constructSharedBranch
	constructUnion
	constructSpecial
	constructOtherPrimitive
	constructXsiAttribute
	constructAttributeWildcard
	constructXsiType
)

// constructNames holds, indexed by construct, each construct's name.
var constructNames = [...]string{
	constructTypeTable:           "an element declaration with a {type table}",
	constructNillable:            "an element declaration with {nillable} true",
	constructSimpleContent:       "simple content",
	constructMixedContent:        "mixed content",
	constructOpenContent:         "{open content}",
	constructGroupRefContent:     "a <group ref> content particle",
	constructChoiceOccurs:        "a content choice occurring other than exactly once",
	constructAllContent:          "all content",
	constructSequenceOccurs:      "a content sequence occurring other than exactly once",
	constructNestedGroup:         "a nested model group particle",
	constructWildcardParticle:    "a wildcard particle",
	constructAbstractTypeElement: "an element declaration whose complex {type definition} has {abstract} true",
	constructSharedName:          "two particles taking one expanded name, each by its own element declaration or a member of its substitution group",
	constructSharedBranch:        "two branches of a sealed interface whose structs have one shape",
	constructUnion:               "a union simple type",
	constructSpecial:             "a special simple type",
	constructOtherPrimitive:      "an atomic simple type whose primitive is none of xs:string, xs:anyURI, xs:boolean, xs:float, xs:double, xs:decimal, xs:dateTime, xs:time, xs:date, xs:gYearMonth, xs:gYear, xs:gMonthDay, xs:gDay, xs:gMonth, xs:duration, xs:hexBinary, xs:base64Binary, xs:QName and xs:NOTATION",
	constructXsiAttribute:        "an attribute use in the xsi namespace",
	constructAttributeWildcard:   "an {attribute wildcard}",
	constructXsiType:             "an element carrying xsi:type",
}

// name returns the construct's name.
func (c construct) name() string {
	if c == 0 || int(c) >= len(constructNames) {
		return "construct(" + strconv.Itoa(int(c)) + ")"
	}
	return constructNames[c]
}

// decline returns the *DeclineError for construct c, charged to no instance
// Loc, detail the text Error's doc states after the construct. No schema or
// instance rule is violated, so it carries no xsderr rule (STYLE E2).
func decline(c construct, detail string) *DeclineError {
	return &DeclineError{construct: c, detail: detail}
}

// at returns a copy of e charged to the instance Loc loc.
func (e *DeclineError) at(loc xsderr.Loc) *DeclineError {
	charged := *e
	charged.loc = &loc
	return &charged
}
