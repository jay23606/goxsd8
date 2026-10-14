package validate

import (
	"fmt"
	"log/slog"
	"slices"

	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// Option configures [New]. The zero set of options is a complete, usable
// configuration: options only replace defaults, never combine into an
// invalid one (STYLE T1).
type Option func(*config)

// config is the resolved [New] configuration. Every field is non-nil from
// construction onward, since each Option installs a usable replacement.
type config struct {
	log *slog.Logger
}

// newConfig applies opts over the defaults: nothing is logged.
func newConfig(opts []Option) config {
	cfg := config{log: slog.New(slog.DiscardHandler)}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithLogger sets the logger the validator reports assessment progress on,
// at debug level, under the "validate" group [New] installs (STYLE L1). A
// nil logger selects the silent default ([slog.DiscardHandler]), so
// assessment is quiet unless a logger is asked for.
func WithLogger(l *slog.Logger) Option {
	if l == nil {
		l = slog.New(slog.DiscardHandler)
	}
	return func(c *config) { c.log = l }
}

// Validator assesses instance documents against one compiled schema.
//
// It is immutable and reusable by layout, not by promise: it holds the
// schema, the backend and the logger and nothing else — no cache of a
// governing mapping, no compiled facets (STYLE D3; package value recomputes
// both by design) — and every piece of walk state lives in a struct
// [Validator.Assess] creates and drops, so a second assessment cannot
// observe the first one's state.
type Validator struct {
	schema  *xsd.Schema
	backend value.Backend
	log     *slog.Logger
}

// New returns a [Validator] assessing instances against schema, reading
// instance lexicals in backend's value space. It takes the one finalized
// [xsd.Schema] parser.Parse assembles — the §3.17.1 Schema component is
// already multi-namespace-capable, so there is no separate schema-set type.
//
// backend MUST be the backend schema was compiled with. Its facets were
// checked in the value space [xsd.SchemaBuilder.FinalizeWith] installed at
// assembly, so assessing with a different one parses instance lexicals in a
// space no facet on the schema was ever checked against: a bound facet
// admitting a value the instance mapping cannot represent, or representing
// it differently, turns into a rejection of a document the schema admits.
// Nothing here can detect that — the schema does not carry the capability —
// so it is the caller's to honor. parser.Parse's own backend option is the
// one to pass.
//
// A nil backend panics, on [value.NewValueSpace]'s and parser.WithBackend's
// grounds: it is a caller bug, not a validity verdict about any document,
// and a parameter cannot be forgotten the way an option can. There is no
// default — installing one would either assess a schema compiled elsewhere
// under the wrong value space with no error, or make "a Validator that
// charges no datatype violation" a representable state whose empty [Result]
// is indistinguishable from a valid document.
//
// It returns an error on exactly one condition, a nil schema, and that
// error is a plain one rather than an [xsderr.Error]: a missing schema is a
// caller's unchecked [xsd.SchemaBuilder.Finalize] result, not a validity
// verdict about any document.
func New(schema *xsd.Schema, backend value.Backend, opts ...Option) (*Validator, error) {
	if backend == nil {
		panic("validate: New: nil value.Backend")
	}
	if schema == nil {
		return nil, fmt.Errorf("validate: New: nil *xsd.Schema")
	}
	cfg := newConfig(opts)
	return &Validator{schema: schema, backend: backend, log: cfg.log.WithGroup("validate")}, nil
}

// SchemaView is the read-only view of the compiled schema a source adapter
// reads to build the infoset it hands [Validator.Assess] (STYLE T3): a root
// element declaration resolved by expanded name, the top-level declarations
// to choose one from when the source names a root by local name alone, a
// declaration's {type definition} resolved to its component, and the
// declarations and wildcards a complex type's content model contains, against
// which an adapter whose source spells no namespace classifies a child by
// local name, or names one only a wildcard admits.
//
// Each method is the [xsd.Schema] method of the same name and answers on its
// terms.
type SchemaView interface {
	xsd.ElementResolver
	Elements() []xsd.ElementDeclaration
	ResolvedType(ref xsd.TypeDefinitionOrRef) (xsd.TypeDefinition, bool)
	ContainedElementDeclarations(within xsd.ComplexType) []xsd.ElementDeclaration
	ContainedWildcards(within xsd.ComplexType) []xsd.Wildcard
}

var _ SchemaView = (*xsd.Schema)(nil)

// Schema returns the [SchemaView] of the compiled schema v assesses against.
// The walk reads the *xsd.Schema itself and not this narrowing, so widening
// the view is the job of a CALLER that needs more, never of a rule inside the
// package.
func (v *Validator) Schema() SchemaView { return v.schema }

// Result is the outcome of one [Validator.Assess] call.
type Result struct {
	violations  []*xsderr.Error
	unevaluated []Unevaluated
	err         error
}

// Violations returns the violations the assessment charged, in document
// order. The slice is copied: mutating the result does not affect r. No
// violation yields nil.
func (r *Result) Violations() []*xsderr.Error {
	if len(r.violations) == 0 {
		return nil
	}
	return append([]*xsderr.Error(nil), r.violations...)
}

// Unevaluated is one check the assessment REACHED and did not perform: the
// spec rule that check answers to, the instance location it was reached at,
// and what was skipped. It is produced by the assessment alone, the way a
// [Result] is, so its fields are unexported and it has no exported
// constructor.
//
// It is not a violation and deliberately does NOT satisfy error: it has no
// Error method, and must not grow one. [xsderr.Error]'s own doc fixes that
// interface's meaning in this module — every schema- or instance-validity
// violation is an *xsderr.Error — so a value that decides nothing about a
// document must not be one, or a consumer can errors.Is it, join it into a
// violation list, or append it to the violations of a Result and turn a
// skipped check into a false reject.
//
// Every site is reached by the walk, one record per withheld check at the item
// it withheld a verdict on:
//
//   - assertions (cvcassertion.go): each {assertions} member of a complex
//     ·governing type definition· left unevaluated (cvc-assertion), and the
//     first assertions-facet {test} the value pipeline declined in place of
//     the String Valid decline it causes (cvc-assertions-valid);
//   - the ·governing type definition· (assess.go, cta.go): each withheld one,
//     not one per alternative the {type table} scan never tried — a {test}
//     outside the §3.12.6 subset (key-cta-ta-select), a selected {type
//     definition} that is nil (cvc-elt clause 1, §5.3), and an undecided
//     ·override· of the ·selected type definition· (cvc-elt clause 4) or of a
//     ·locally declared type· (key-sva (§3.3.4.6) clause 1.2);
//   - content (cvccomplexcontent.go): String Valid (§3.16.4) over an element's
//     ·initial value· (cvc-type clause 3.1.3, cvc-complex-type clause 1.2), a
//     fixed {value constraint} comparison (cvc-elt clause 5.2.2.2.2), and,
//     where xsd.Schema.ContentMatcher does not decide an element's {content
//     type}, each element [[child]] left unmatched, or the element itself where
//     it has none (cvc-complex-content);
//   - attributes (cvcattribute.go): cvc-attribute clauses 3, 4 and 5, cvc-au,
//     and a ·defaulted attribute·'s {lexical form} (cvc-complex-type clause 4);
//   - the ID/IDREF table (cvcid.go): each item that could have been in the
//     ·eligible item set· and could not be read (cvc-id clause 1), an element
//     whose ·governing type definition· was withheld among them;
//   - identity constraints (cvcidentityconstraint.go): a path outside the
//     subset, a field node with no readable value, and an undecided
//     ·key-sequence· comparison (cvc-identity-constraint clauses 3 and 4).
//
// One decline is NOT recorded, so an empty Unevaluated is not by itself a claim
// that the walk performed every check it reached: an unresolvable {attribute
// declaration}, which no *xsd.Schema that exists carries.
type Unevaluated struct {
	rule xsderr.Rule
	loc  xsderr.Loc
	msg  string
}

// newUnevaluated builds one record, mirroring [xsderr.New]'s shape so a site
// that charges and a site that declines are written the same way.
func newUnevaluated(rule xsderr.Rule, loc xsderr.Loc, format string, args ...any) Unevaluated {
	return Unevaluated{rule: rule, loc: loc, msg: fmt.Sprintf(format, args...)}
}

// Rule returns the spec rule the check answers to. For a check whose failure is
// a charge, that is the ID it would have been charged under — cvc-assertion,
// cvc-assertions-valid and each content site's cvc-type, cvc-complex-type,
// cvc-elt or cvc-complex-content all are, the clause going in [Unevaluated.Msg].
// It is NOT always a chargeable rule: §3.12.4 gives conditional type assignment
// no Validation Rule of its own, so a withheld Type Alternative {test} is
// recorded under the [Definition:] anchor key-cta-ta-select (cta.go), whose
// failure selects no type rather than charging one.
func (u Unevaluated) Rule() xsderr.Rule { return u.rule }

// Loc returns the INSTANCE location the check was reached at: the element for
// a check on an element information item, the attribute for one on an
// attribute information item. The zero Loc means unknown.
func (u Unevaluated) Loc() xsderr.Loc { return u.loc }

// Msg returns the human-readable statement of which check was skipped and on
// which component.
func (u Unevaluated) Msg() string { return u.msg }

// Unevaluated returns the checks the assessment reached and did not perform,
// in document order. The slice is copied: mutating the result does not affect
// r. No such check yields nil.
//
// An empty [Result.Violations] together with a non-empty result here is NOT a
// pass; doc.go's Contract section states what that combination means.
func (r *Result) Unevaluated() []Unevaluated {
	if len(r.unevaluated) == 0 {
		return nil
	}
	return append([]Unevaluated(nil), r.unevaluated...)
}

// Err reports the fault in the source that stopped the walk, or nil when
// the walk reached the end of the document. A non-nil Err means the
// assessment is INCOMPLETE, not that the document is invalid: an invalid
// document is reported through [Result.Violations].
func (r *Result) Err() error { return r.err }

// resultMark is a position in a [Result]: how many violations and how many
// [Unevaluated] it held when [Result.mark] took it. [walk.element] reads what
// was recorded since its caller's mark to know whether its element is ·valid·
// in the partial ·PSVI· (cvc-assertion clauses 1.2 and 2.3.1.1), and every
// caller takes that mark BEFORE the element's governance is decided, so a
// cvc-elt clause 4 charge or decline [walk.instanceOverride] records counts as
// the element's own.
type resultMark struct{ violations, unevaluated int }

// mark is r's present position.
func (r *Result) mark() resultMark {
	return resultMark{violations: len(r.violations), unevaluated: len(r.unevaluated)}
}

// violatedSince reports whether r charged a violation after m.
func (r *Result) violatedSince(m resultMark) bool { return len(r.violations) > m.violations }

// recordedSince reports whether r charged a violation or recorded an
// [Unevaluated] after m.
func (r *Result) recordedSince(m resultMark) bool {
	return r.violatedSince(m) || len(r.unevaluated) > m.unevaluated
}

// skipping is m with the records between from and to, two later marks, no
// longer counted as recorded since it: a span about another item that falls
// between m and the reader of m.
func (m resultMark) skipping(from, to resultMark) resultMark {
	return resultMark{
		violations:  m.violations + to.violations - from.violations,
		unevaluated: m.unevaluated + to.unevaluated - from.unevaluated,
	}
}

// causedBy builds the [xsderr.Error] for one violation, carrying cause — the
// verdict of the rule this one DELEGATES to — in both places a consumer looks
// for it, from ONE argument (STYLE D3): appended to the formatted Msg after
// ": ", and set as the wrapped cause so errors.Unwrap, errors.Is and errors.As
// reach the inner *xsderr.Error and the rule ID it carries. A reader holding
// only the rendered string has no chain to walk; a reader holding the error
// should not have to scrape a message for a rule ID; and taking one argument
// is what stops the two from naming different errors.
//
// format therefore stops at the delegating rule's own sentence, which leads
// with its own fact and closes on its own citation (STYLE E5) — the ": " and
// the cause's own rendering are this function's to append. A cause that is
// itself an *xsderr.Error with the zero Loc renders from its fields as
// "[rule] msg", since its Error() would print the unknown position as "?: "
// inside a line the outer Loc already places (#1844); every other cause
// renders as %v renders it. A nil cause yields the plain [xsderr.New] result,
// Msg unadorned and Unwrap nil, and reaches here only through
// [contentCheck.charge]: every other non-delegating charge site in the package
// calls [xsderr.New] itself.
func causedBy(rule xsderr.Rule, loc xsderr.Loc, cause error, format string, args ...any) *xsderr.Error {
	if cause == nil {
		return xsderr.New(rule, loc, format, args...)
	}
	var rendered any = cause
	//nolint:errorlint // only the cause ITSELF renders from its fields: a wrapper around a zero-Loc *xsderr.Error keeps its own %v rendering.
	if inner, ok := cause.(*xsderr.Error); ok && inner.Loc == (xsderr.Loc{}) {
		rendered = fmt.Sprintf("[%s] %s", inner.Rule, inner.Msg)
	}
	v := xsderr.New(rule, loc, format+": %v", append(slices.Clone(args), rendered)...)
	v.Err = cause
	return v
}
