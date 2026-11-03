package codegen

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/kud360/goxsd8/xsderr"
)

// A DeclineError is a construct [Generate] declines; Generate's doc states
// how it is returned.
type DeclineError struct {
	construct construct
	loc       xsderr.Loc
	// detail is the decline's text before its Loc: the construct's format
	// over the arguments the declining site passed.
	detail string
}

// Construct returns the decline's Construct, as Generate's doc defines it.
func (e *DeclineError) Construct() Construct { return Construct{e.construct} }

// Loc returns the Loc of the component the decline is charged to: the element
// declaration, for a property of that declaration, of its simple type or of a
// simple type its {type table} selects, or for its reaching an element or
// complex type again through elements that each occur exactly once; the
// complex type, for a property of that type, its content or its particles;
// and for an attribute use, the attribute declaration it resolves to, the
// global one for an <attribute ref>.
//
// Loc is the zero xsderr.Loc exactly when that component's Loc is zero: a
// component built through xsd.SchemaBuilder with a zero Loc. Every other decline from a
// parsed schema carries the declining component's location in its document.
func (e *DeclineError) Loc() xsderr.Loc { return e.loc }

// Error names the construct and the Loc; it does not name the target.
func (e *DeclineError) Error() string {
	return e.detail + " at " + e.loc.String() + " is not supported by codegen, a generator refusal, not a schema fault"
}

// Unwrap returns errors.ErrUnsupported.
func (e *DeclineError) Unwrap() error { return errors.ErrUnsupported }

// A Construct is a construct Generate declines: two declines of the same
// construct have equal Constructs; the zero Construct names none. A Construct
// is comparable: == reports whether two name the same construct, and it may be
// used as a map key. The package exports no Construct value, because a
// release that lifts a construct stops returning its Construct; compare a
// Construct only with another obtained from [DeclineError.Construct]. The
// package doc's "Coverage" section states what declines.
type Construct struct{ c construct }

// String returns the construct's name for display, distinct for each
// Construct; it is not the decline's message. The names are not stable across
// releases: a release may rename a construct, split one into several, or stop
// returning a name once the generator covers its construct, and no name is
// reserved. To tell constructs apart, use the Constructs themselves, as
// [Construct] states, never a name.
func (c Construct) String() string {
	if c.c == 0 || int(c.c) >= len(constructs) {
		return "Construct(" + strconv.Itoa(int(c.c)) + ")"
	}
	return constructs[c.c].name
}

// construct is the closed set of constructs the generator declines, one per
// declining site but constructVacuousParticle, which field's wildcard, model
// group and element paths, contentGroup, reach and vacuousBasic each
// decline. The zero value is invalid (STYLE T1).
type construct uint8

// The construct values, in the order of their declining sites in emit.go.
const (
	constructUncoveredPrimitive construct = iota + 1
	constructVacuousParticle
	constructAbstractLocal
	constructValueCycle
)

// constructs holds, indexed by construct, each construct's name, which
// Construct.String returns, and the format its decline's detail is rendered
// from. Index 0, the invalid construct, holds neither.
var constructs = [...]struct{ name, format string }{
	constructUncoveredPrimitive: {"uncovered primitive", "%s, whose {primitive type definition} is %s"},
	constructVacuousParticle:    {"particle with {max occurs} 0", "a particle of %s with {max occurs} 0 in %s"},
	constructAbstractLocal:      {"local element declaration with {abstract} true", "%s with {abstract} true in %s"},
	constructValueCycle:         {"struct-value recursion", "%s reaching %s again through elements that each occur exactly once"},
}

// decline returns the *DeclineError for construct c, which the generator does
// not cover, at loc, its detail c's format over args. No schema rule is
// violated, so it carries no xsderr rule (STYLE E2). Its text already says the
// construct is unsupported, so DeclineError.Error prints none of
// errors.ErrUnsupported's text.
func decline(loc xsderr.Loc, c construct, args ...any) error {
	return &DeclineError{construct: c, loc: loc, detail: fmt.Sprintf(constructs[c].format, args...)}
}
