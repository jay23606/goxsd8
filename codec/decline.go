package codec

import (
	"errors"
	"strconv"

	"github.com/kud360/goxsd8/xsderr"
)

// A DeclineError is a construct [Decode] does not decode; it wraps
// errors.ErrUnsupported, and Decode's doc states when it is returned.
type DeclineError struct {
	construct construct
	loc       xsderr.Loc
	// detail is the text Error's doc states after the construct.
	detail string
}

// Construct returns the construct the decline declines.
func (e *DeclineError) Construct() Construct { return Construct{e.construct} }

// Loc returns the instance Loc of the element the decline is charged to: the
// document element for a construct of its declaration or its type, and the
// element carrying it for xsi:type.
func (e *DeclineError) Loc() xsderr.Loc { return e.loc }

// Error names the instance Loc and the construct, then the declining
// declaration, the document element's, a local element's or an attribute's,
// and, for a construct of the document element's type, that type
// labelled as one, each with its schema Loc, which is the zero Loc for a
// built-in component; for xsi:type, it names the element and the type the
// xsi:type names.
func (e *DeclineError) Error() string {
	return e.loc.String() + ": codec does not decode " + e.construct.name() + ": " + e.detail +
		"; a decoder refusal, not a schema or instance fault"
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
// attribute uses in order, its attribute wildcard, and the instance.
const (
	constructTypeTable construct = iota + 1
	constructNillable
	constructSimpleRoot
	constructEmptyContent
	constructSimpleContent
	constructMixedContent
	constructOpenContent
	constructGroupRefContent
	constructChoiceContent
	constructAllContent
	constructSequenceOccurs
	constructNestedGroup
	constructWildcardParticle
	constructElementRef
	constructComplexElement
	constructSharedName
	constructList
	constructUnion
	constructSpecial
	constructOtherPrimitive
	constructXsiAttribute
	constructAttributeWildcard
	constructXsiType
)

// constructNames holds, indexed by construct, each construct's name.
var constructNames = [...]string{
	constructTypeTable:         "an element declaration with a {type table}",
	constructNillable:          "an element declaration with {nillable} true",
	constructSimpleRoot:        "a document element of simple type",
	constructEmptyContent:      "empty content",
	constructSimpleContent:     "simple content",
	constructMixedContent:      "mixed content",
	constructOpenContent:       "{open content}",
	constructGroupRefContent:   "a <group ref> content particle",
	constructChoiceContent:     "choice content",
	constructAllContent:        "all content",
	constructSequenceOccurs:    "a content sequence occurring other than exactly once",
	constructNestedGroup:       "a nested model group particle",
	constructWildcardParticle:  "a wildcard particle",
	constructElementRef:        "an <element ref> particle",
	constructComplexElement:    "a local element declaration of complex type",
	constructSharedName:        "two particles sharing an expanded name",
	constructList:              "a list simple type",
	constructUnion:             "a union simple type",
	constructSpecial:           "a special simple type",
	constructOtherPrimitive:    "an atomic simple type whose primitive is neither xs:string nor xs:anyURI",
	constructXsiAttribute:      "an attribute use in the xsi namespace",
	constructAttributeWildcard: "an {attribute wildcard}",
	constructXsiType:           "an element carrying xsi:type",
}

// name returns the construct's name.
func (c construct) name() string {
	if c == 0 || int(c) >= len(constructNames) {
		return "construct(" + strconv.Itoa(int(c)) + ")"
	}
	return constructNames[c]
}

// decline returns the *DeclineError for construct c at the instance Loc loc,
// detail the text Error's doc states after the construct. No schema or
// instance rule is violated, so it carries no xsderr rule (STYLE E2).
func decline(loc xsderr.Loc, c construct, detail string) *DeclineError {
	return &DeclineError{construct: c, loc: loc, detail: detail}
}
