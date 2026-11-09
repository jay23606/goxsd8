package xsderr

import (
	"errors"
	"fmt"
)

// Rule is a spec rule ID, such as "cvc-complex-type", "cos-st-restricts", or
// "derivation-ok-restriction". Exactly one Rule identifies each Error. It is
// usually a VALIDATION rule; where the spec names a check but states no rule
// for it, the definition anchor naming that check is the ID instead
// (key-cta-ta-select, validate/cta.go). Every Rule constructed in this module
// is expected to be present in the generated catalog (see IsValidRule).
type Rule string

// RuleXMLWellFormed is the sentinel Rule for XML well-formedness faults
// (unbound namespace prefix, mismatched or unclosed tag, malformed XML) that
// are not XSD schema- or instance-validity violations and so have no
// spec-defined cvc-*/src-*/cos-*/sic-* rule ID. It lets such an Error carry a
// recognizable, non-empty Rule instead of "" (which would be indistinguishable
// from a caller that simply forgot to set one). IsValidRule accepts it as a
// documented, non-spec exemption from the generated catalog.
const RuleXMLWellFormed Rule = "xml-wf"

// RuleComponentInvariant is the sentinel Rule for a representation-invariant
// fault: a component constructor, or a later reader of a component such as a
// resolver or the value pipeline, rejecting a state the spec never names as a
// numbered clause because the XML mapping layer above it makes the state
// unrepresentable — an absent (zero) QName in a slot that exists only to carry
// a present reference, for example. Such a rejection is a
// representation-invariant fault of the caller, not a schema- or
// instance-validity verdict, so citing a cvc-*/src-*/*-props-correct rule would
// misclassify it for RuleOf/errors.As consumers. Like RuleXMLWellFormed it lets
// the Error carry a recognizable, non-empty Rule instead of "", and IsValidRule
// accepts it as a documented, non-spec exemption from the generated catalog.
//
// UNREACHABILITY IS THE PRECONDITION, and it is the standing convention for
// every future guard (#343): a rejection may cite RuleComponentInvariant only
// when the state it rejects is genuinely unreachable from a schema document —
// when the XML mapping layer above the constructor cannot produce it. If a human
// could type the schema document that produces the state, the producer owes it a
// real spec rule first, charged at the construct the author wrote, and the
// constructor guard is then a BACKSTOP for the programmatic caller rather than
// the primary charge. Reaching this sentinel from a document is therefore a
// defect in the producer, not a verdict about the document: it tells a
// RuleOf/errors.As consumer that the library built something illegal, which is
// the wrong thing to say about an author's mistake.
const RuleComponentInvariant Rule = "component-invariant"

// Loc identifies where an offending construct lives — the schema document or
// the instance document. Its fields are threaded from the positions the
// reader of that document reports — parser/xmltree for XML, validate/jsonsrc
// for JSON — never reconstructed (STYLE E3). The zero Loc means the location
// is unknown, and should be rare.
type Loc struct {
	// URI is the document the construct was read from.
	URI string
	// Line is the 1-based line number, or 0 when unknown.
	Line int
	// Col is the 1-based column, counted in bytes from the start of the line
	// — not in runes or UTF-16 code units — or 0 when unknown. Which byte
	// stream it counts, the source or a decoding of it, is the producing
	// reader's to state (parser/xmltree.NewReader, validate/jsonsrc's
	// "Locations" section).
	Col int
}

// String renders a Loc one dimension at a time, as "uri:line:col". The uri is
// URI, or "?" when URI is empty. ":line" follows only when Line is known (at
// least 1), and ":col" only when both Line and Col are known, so a known Col
// without a known Line is not rendered, and the zero Loc renders "?".
func (l Loc) String() string {
	uri := l.URI
	if uri == "" {
		uri = "?"
	}
	if l.Line < 1 {
		return uri
	}
	if l.Col < 1 {
		return fmt.Sprintf("%s:%d", uri, l.Line)
	}
	return fmt.Sprintf("%s:%d:%d", uri, l.Line, l.Col)
}

// Error is the module's structured error currency: an error carrying the spec
// Rule it concerns and the source Loc of the offending construct. Not every
// *Error is a validity verdict.
//
// An *Error whose Rule is RuleXMLWellFormed is an XML well-formedness fault,
// and one whose Rule is RuleComponentInvariant is a representation-invariant
// fault such as a component constructor's rejection; neither is a verdict, and
// comparing Rule with those two sentinels identifies those two classes.
// IsValidRule is not that test, because it accepts both sentinels. Nor is the
// comparison sufficient: an *Error under a catalog Rule can still be a fault
// rather than a verdict — a facet-pipeline precondition charged under
// cos-applicable-facets, or a foreign value handed to a canonical mapping.
// Whether such an *Error is a verdict is the producing API's to document;
// value.ValidateLexical's caller tests value.IsDatatypeVerdict. A validate
// caller reads the split from two channels, validate.Result.Err and
// validate/xmlsrc.Validate's returned error; xmlsrc.Validate's doc comment owns
// which fault reaches which.
type Error struct {
	// Rule is the spec rule the error concerns, or one of the two sentinels.
	Rule Rule
	// Loc is where the offending construct lives; the zero Loc means unknown.
	Loc Loc
	// Msg is the human-readable explanation.
	Msg string
	// Err is an optional wrapped cause; Unwrap returns it.
	Err error
}

// Error renders the error as "loc: [rule] msg".
func (e *Error) Error() string {
	return e.Loc.String() + ": " + e.ruleMsg()
}

// ruleMsg renders e without its Loc, as "[rule] msg".
func (e *Error) ruleMsg() string {
	return fmt.Sprintf("[%s] %s", e.Rule, e.Msg)
}

// CauseText renders err as it reads when quoted as the cause inside another
// message. An err that is itself an *Error — its own dynamic type, not an
// *Error found further down its chain — with the zero Loc renders as
// "[rule] msg", without the "?: " its Error would print for the unknown
// position; every other err, nil and a wrapper around a zero-Loc *Error
// included, renders as fmt's %v renders it.
func CauseText(err error) string {
	//nolint:errorlint // only err ITSELF renders from its fields: a wrapper around a zero-Loc *Error keeps its own %v rendering.
	if e, ok := err.(*Error); ok && e != nil && e.Loc == (Loc{}) {
		return e.ruleMsg()
	}
	return fmt.Sprint(err)
}

// Unwrap returns the wrapped cause so errors.Is and errors.As reach through the
// wrapping; a wrapper that hid its cause would break sentinel detection up the
// chain.
func (e *Error) Unwrap() error {
	return e.Err
}

// New builds an *Error attributing a formatted message to rule at loc, with no
// wrapped cause.
func New(rule Rule, loc Loc, format string, args ...any) *Error {
	return &Error{Rule: rule, Loc: loc, Msg: fmt.Sprintf(format, args...)}
}

// Wrap attaches rule and loc to a deeper error, keeping its identity (Unwrap
// returns err so errors.Is/As reach the wrapped cause) and taking Msg from
// CauseText(err). Wrap returns nil when err is nil.
func Wrap(rule Rule, loc Loc, err error) *Error {
	if err == nil {
		return nil
	}
	return &Error{Rule: rule, Loc: loc, Msg: CauseText(err), Err: err}
}

// RuleOf reports the Rule of the first *Error in err's chain. The second result
// is false when the chain holds no *Error.
func RuleOf(err error) (Rule, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Rule, true
	}
	return "", false
}

// LocOf reports the Loc of the first *Error in err's chain. The second result
// is false when the chain holds no *Error.
func LocOf(err error) (Loc, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Loc, true
	}
	return Loc{}, false
}

// IsValidRule reports whether r is in the generated spec catalog (ruleCatalog,
// emitted by tools/rulecat into catalog.go) or is one of the two hand-added
// non-spec sentinels, RuleXMLWellFormed and RuleComponentInvariant, which are
// deliberately outside the catalog because the faults they name have no
// spec-defined rule ID. It lives here rather than in the generated catalog.go
// so the sentinel exemptions survive `go generate`.
//
// Membership is exact: no prefix-trimming, no clause-suffix (dotted) leniency.
// The generated catalog holds the rule IDs tools/rulecat extracts from the
// specs ("cvc-complex-type", "cvc-elt"); they carry no part number, save two
// dotted names that spec prose spells, cos-ct-extends.1.2 and
// src-simple-type.1. So both IsValidRule("cvc-complex-type.2.1") and
// IsValidRule("cvc-elt.1") are false, and stay false. Those two dotted names
// pass, though no non-test construction site cites them: this check is a floor
// under the no-part-number convention the package doc's "Using xsderr" states,
// not the convention itself. Citing a clause-level rule means changing that
// convention and extending tools/rulecat to emit the name, never weakening this
// check.
//
// The sentinel set stays closed at two; the package doc's "The s4s-grammar
// class" says why that class gets none.
func IsValidRule(r Rule) bool {
	if r == RuleXMLWellFormed || r == RuleComponentInvariant {
		return true
	}
	_, ok := ruleCatalog[r]
	return ok
}
