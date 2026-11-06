package parser

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/internal/schemaloc"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// Option configures [Parse], [ParseReport] and [ParseSet]. The zero set of
// options is a complete, usable configuration: options only replace defaults,
// never combine into an invalid one (STYLE T1).
type Option func(*config)

// config is the resolved [ParseSet] configuration. It is always valid — every
// field is non-nil from construction onward, since each Option either installs
// a usable replacement or panics.
type config struct {
	resolver loader.Resolver
	backend  value.Backend
	log      *slog.Logger
}

// newConfig applies opts over the defaults: schema documents are resolved as
// paths under the current directory, values use the spec-exact strict backend,
// and nothing is logged.
func newConfig(opts []Option) config {
	cfg := config{
		resolver: loader.Dir("."),
		backend:  strict.New(),
		log:      slog.New(slog.DiscardHandler),
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithResolver sets the [loader.Resolver] through which every schema document —
// every root location and every <include>d or <import>ed location — is fetched (§4.3.2: how
// schema definitions are located is the processor's, hence the caller's,
// business).
//
// It panics if r is nil: a nil resolver is a caller bug, not a schema-validity
// condition, so it is guarded exactly like [xsd.SchemaBuilder.AddType]'s nil
// component rather than deferred into a nil dereference mid-parse.
func WithResolver(r loader.Resolver) Option {
	if r == nil {
		panic("parser: WithResolver: nil loader.Resolver")
	}
	return func(c *config) { c.resolver = r }
}

// WithBackend sets the [value.Backend] supplying the builtin datatypes' value
// spaces, replacing the default spec-exact builtin/strict backend.
//
// It panics if b is nil, on the same grounds as [WithResolver].
func WithBackend(b value.Backend) Option {
	if b == nil {
		panic("parser: WithBackend: nil value.Backend")
	}
	return func(c *config) { c.backend = b }
}

// WithLogger sets the logger the parser reports assembly progress on, under the
// "parser" group at debug level. A nil logger selects the silent default
// ([slog.DiscardHandler]), so parsing is quiet unless a logger is asked for
// (STYLE L1).
func WithLogger(l *slog.Logger) Option {
	if l == nil {
		l = slog.New(slog.DiscardHandler)
	}
	return func(c *config) { c.log = l }
}

// Parse assembles the schema rooted at location and returns it finalized. It is
// [ParseReport] without the report, for the callers that only want the schema.
func Parse(location string, opts ...Option) (*xsd.Schema, error) {
	schema, _, err := ParseReport(location, opts...)
	return schema, err
}

// ParseReport assembles the schema rooted at location, returns it finalized,
// and reports which schema documents went into it — every document it read, in
// discovery order, and every ·inter-schema-document reference· it could not
// follow to one (see [AssemblyReport]). The report is never nil and is
// populated as far as assembly got even when an error is returned.
//
// Every document of the assembly is ·conditional-inclusion pre-processed·
// (§4.2.2) as it is read, before its own directives are followed, so schema(D)
// below is always read over ci(D) — see parser/doc.go's Conditional inclusion
// section.
//
// It implements §4.2.1's schema(D): the components of the root document plus,
// transitively, those of every schema document reached through an <xs:include>
// child (§4.2.3) — including chameleon inclusion, where a document with no
// targetNamespace of its own contributes its components, and its unqualified
// QName references, to the including namespace (§4.2.3 clause 3.2, §F.1) — or
// through an <xs:import> child (§4.2.6), which brings a DIFFERENT namespace's
// components in unchanged: import never coerces, so an imported document's
// components stay in its own target namespace and a cross-namespace reference
// resolves against them at finalize. That <import> element does double duty: it
// is also what LICENSES the referring document to name that namespace at all
// (src-resolve clause 4.2.2, §4.2.6.1), and the license reaches only the
// document the element sits in — see parser/doc.go's Composition section.
// Documents are read once each, keyed by resolved location AND the namespace
// they were reached under AND the override applied to them, so a diamond or a
// (spec-legal) cycle of <include>s, a namespace imported repeatedly, or a
// document reached both plainly and through an <xs:redefine>, contributes its
// components once and does not trip sch-props-correct (§3.17.6.1) clause 2.
// Reading once skips the re-COMPOSITION alone: every <import> element is still
// judged against src-import clause 3, whether or not it was the one that caused
// D2 to be read.
//
// An <xs:override> child (§4.2.5) is followed too: the document it names is
// composed exactly as an <include>d one — §4.2.5 clause 3.1.2 defines an override
// AS an <include> of the transformed document — after ·override pre-processing·
// substitutes the override's own children for the identically-named source
// declarations of that document and of every document it in turn <include>s or
// <override>s (§F.2, parser/override.go).
//
// An <xs:redefine> child (§4.2.4) is followed too, and is the one directive that
// both adds and subtracts: the document it names is composed as an <include>d
// one, contributing every component it declares EXCEPT those the <redefine>
// explicitly redefines (clause 4.1.2), while the <redefine>'s own children
// contribute the replacements as definitions of THIS document (clause 4.1.1).
// The subtraction is per REDEFINITION, not per document: one also reached
// plainly, or redefined elsewhere for other names, still contributes the
// definitions that reading does not redefine — and contributes them once. A
// self-reference inside a redefining declaration resolves to the definition it
// replaces, not to itself (src-expredef, parser/redefine.go). Two things differ
// from <include>: a non-empty <redefine> whose schemaLocation does not resolve is
// an ERROR (src-redefine clause 1) rather than a silent skip, and a redefining
// <simpleType>/<complexType> is PAIRED with the {name}-·absent· original
// src-expredef clause 1.1 defines, so its self-derivation is a real derivation.
// §4.2.4 marks the mechanism ·deprecated·.
//
// A returned error takes one of four shapes, told apart by the test a caller
// runs:
//
//   - an *[xsderr.Error] that errors.As and [xsderr.RuleOf] reach under a catalog
//     rule (src-include, src-import, src-redefine, src-resolve,
//     sch-props-correct, …): a schema-validity verdict located at the offending
//     construct. A document an <include>, <import>, <redefine> or <override>
//     names that resolves but cannot be read, or is not well-formed, is such a
//     verdict under that directive's rule, with the reader's error still
//     reachable by errors.Is.
//   - an *[xsderr.Error] under [xsderr.RuleXMLWellFormed]: a fault, not a
//     verdict, for a ROOT document that resolves but cannot be read or is not
//     well-formed, with the reader's error reachable by errors.Is. An *Error
//     under [xsderr.RuleComponentInvariant] is no verdict either, and one
//     reaching a ParseReport caller is a parser defect.
//   - a plain error, which errors.As does not reach as an *[xsderr.Error], so
//     RuleOf reports false: a grammar fault against the schema for schema
//     documents (the s4s-grammar class [xsderr]'s package doc names); a root
//     whose document element is not <schema>; a derivation this parser cannot
//     yet map, one that reads the {content type} of a base still being built; a
//     root location the resolver does not resolve, wrapping the resolver's
//     error, so errors.Is(err, [loader.ErrNotFound]) reports a missing root; and
//     a resolver error other than ErrNotFound for a directive's location, wrapped
//     the same way.
//   - a *[builtin.MissingPrimitivesError], which errors.As reaches, when the
//     backend [WithBackend] set does not map every primitive datatype.
//
// Like [Produce], ParseReport returns only the first error.
//
// It is [ParseSet] over the one root [RootAt](location).
func ParseReport(location string, opts ...Option) (*xsd.Schema, *AssemblyReport, error) {
	return ParseSet([]Root{RootAt(location)}, opts...)
}

// ParseSet assembles the schema rooted at every root of roots into ONE schema,
// returns it finalized, and reports which schema documents went into it, exactly
// as [ParseReport] does for one root — ParseReport is ParseSet over a single
// [RootAt]. The report is never nil and is populated as far as assembly got even
// when an error is returned: the roots before the one that failed, and their
// closures, are in it.
//
// Several roots is implementation latitude, not a spec construct: §4.2.1 defines
// schema(D) per single document, and how a processor assembles the components it
// validates against is processor- and application-dependent (§4.3.2). ParseSet
// takes the one assembly §4.2 already defines and enters every root into it, so
// every cross-document rule a single root's closure is held to holds across the
// set too: two roots declaring one expanded name collide under sch-props-correct
// (§3.17.6.1) clause 2, and a QName in one root's document reaches another root's
// namespace only if that document <xs:import>s it (src-resolve clause 4, §4.2.6.1)
// — sharing a set licenses nothing.
//
// Roots are entered in caller order, each root's closure depth-first and
// pre-order before the next root is read, so [AssemblyReport.Documents] lists
// them in that order. Each root is read once per load-once identity, as a
// directive's document is: a root repeated, or already reached through another
// root's closure under the same namespace, is not composed again. How each root
// is read, and what it charges, is its constructor's: see [RootAt] and [HintAt].
// A HintAt root that resolves to no document is legal to skip and is reported by
// [AssemblyReport.UnfollowedRoots]; [AssemblyReport.Unfollowed] keeps only the
// directives of the assembled documents.
//
// An empty roots is a plain error, with an empty report. A nil Root panics, as a
// nil [WithResolver] does: it is a caller bug, not a schema-validity condition.
// Like [Produce], ParseSet returns only the first error.
func ParseSet(roots []Root, opts ...Option) (*xsd.Schema, *AssemblyReport, error) {
	for i, r := range roots {
		if r == nil {
			panic(fmt.Sprintf("parser: ParseSet: roots[%d] is a nil Root", i))
		}
	}
	if len(roots) == 0 {
		// Nothing was assembled: an empty report, not a nil one, so every caller
		// reads the same shape on every path.
		return nil, &AssemblyReport{}, errors.New("parser: ParseSet: no root locations given")
	}
	cfg := newConfig(opts)
	a := newAssembly(cfg)
	for _, r := range roots {
		if err := r.enter(a); err != nil {
			return nil, a.report(), err
		}
	}
	schema, err := a.compile(cfg.backend)
	return schema, a.report(), err
}

// discovered is one schema document of an assembly paired with the EFFECTIVE
// target namespace its components are minted in. That namespace is established
// at discovery time and differs per document, so it cannot be derived from any
// other assembly state and is not redundant with the document's own
// targetNamespace attribute (STYLE D3):
//
//   - every [RootAt] root document, and any <include>d document that declares a
//     targetNamespace, is minted in its own;
//   - a chameleon <include>d document (none of its own) is minted in the
//     INCLUDER's effective namespace (§4.2.3 clause 2.3, §F.1);
//   - an <import>ed document, or a [HintAt] root's, is ALWAYS minted in its
//     own, coerced never — §F.1's transformation belongs to src-include clause
//     2.3 alone, so a bare <import> of a no-namespace document leaves its
//     components in no namespace even when the importing document has one.
//
// Do not collapse this back into a single assembly-wide namespace: that field
// was only ever true of an include-only closure.
type discovered struct {
	doc *Document
	tns string

	// resolved is the location the [loader.Resolver] reported for this document —
	// the load-once identity half of docKey, and what [AssemblyReport] hands a
	// consumer as [AssembledDocument.Location]. It is not derivable from doc:
	// Document.URI is the location as REQUESTED, before the resolver
	// canonicalized it (loader.Dir folds on-disk case), so the two differ exactly
	// where dedup depends on them (STYLE D3).
	resolved string

	// ov is the ·override pre-processing· in force over THIS document's own top
	// level (§4.2.5, §F.2), nil when it was reached plainly. Like tns it is
	// established at discovery time and is a property of the PATH the document was
	// reached by, not of the document (STYLE D3): the same document can be reached
	// once plainly and once under an <override>, and each reading contributes its
	// own component set — which is precisely why §4.2.5 says such a schema "will
	// have duplicate and conflicting versions of some components".
	ov *overrideSet

	// rds are the ·redefinition·s in force over THIS document (§4.2.4,
	// parser/redefine.go) — ONE per reading of it the assembly merged here, in
	// discovery order, each nil when that reading reached the document plainly. A
	// discovered document always carries at least one.
	//
	// It is a slice rather than a single set because every reading of one document
	// under one docKey composes ONCE (see docKey): a top-level definition is
	// contributed unless EVERY reading excepts it, and each reading's own set still
	// records the original it excepted (redefinitions, parser/redefine.go).
	rds redefinitions

	// redefines are the sets read from this document's OWN <redefine> children, in
	// document order. It is the other end of rds — what this document does to
	// others rather than what was done to it — and it is not derivable from the
	// document (STYLE D3): a set exists only for a <redefine> the assembly
	// actually followed, which is what distinguishes an assembled document from
	// one handed to [Produce].
	redefines []*redefineSet

	// unmapped is this discovery's coverage census (producer.census,
	// parser/census.go), taken before any component of the document is built,
	// for [AssembledDocument.Unmapped]. It is a property of the DISCOVERY rather
	// than of the document (STYLE D3): rds and ov except and substitute for
	// declarations another discovery of the same file maps (§4.2.4 clause 4.1.2,
	// §F.2 clause 1), so the same file discovered twice can hold a different
	// census each time, and no field here derives it.
	unmapped []UnmappedConstruct
}

// docKey is the load-once identity of one discovered document: its RESOLVED
// location together with the namespace it was reached under and the ·override
// pre-processing· applied to it. The location alone does not suffice once
// <import> exists, because the SAME document can legitimately be reached twice
// under two different namespaces — once as a chameleon <include> coerced into the
// includer's namespace, once as a bare <import> staying in no namespace — and
// each of those contributes its own component set. The override half is there for
// the same reason: one document overridden two different ways yields two
// different component sets (§4.2.5), while overriding it the same way twice, or
// reaching it again around an <include>/<override> cycle, must contribute its
// components once (§4.2.5's note on sch-props-correct clause 2). In an
// include-only closure both extra components are constant, so neither changes any
// include behavior.
//
// The ·redefinition· applied to a reading is deliberately NOT part of it, so a
// document reached both plainly and as an <xs:redefine> target — or redefined
// twice — is composed ONCE and every definition the redefinitions do not reach is
// minted once (#1349). §4.2.4's construction gives the redefining schema
// "components identical to all the schema components of S2, with the exception of
// those explicitly redefined" (src-redefine clause 4.1.2), which is §4.2.3 clause
// 3.1.2's wording for a plain <include> verbatim; S2 is schema(D2), a pure
// function of D2 (§4.2.1, key-corresponding-schema), so a definition NEITHER
// reading redefines is one component both readings draw in, not two that happen
// to be equal. Which definitions a redefinition subtracts is applied per reading
// at production instead ([discovered].rds).
//
// The extrapolation is stated here because the spec is not explicit for this
// shape: §4.2.4's closing note ("multiple equivalent <redefine>ing of the same
// schema document will not constitute a violation of clause 2 of Schema
// Properties Correct") is scoped to redefine×redefine, and §4.2.5's is the only
// note covering a MIXED pair of mechanisms — for <override>, which it makes a
// genuine conflict. Hence override stays in the key and redefine does not.
type docKey struct {
	resolved  string
	namespace string
	override  string
}

// assembly is the multi-document build context for one [ParseSet] call: the
// <include>/<import> closure of every root, and the single builder every one of
// those documents produces into.
type assembly struct {
	resolver loader.Resolver
	log      *slog.Logger

	// loaded indexes the documents already read, each under the docKey it was
	// discovered as (§4.2.3: "If two <include> elements specify the same schema
	// location (after resolving relative URI references) then they refer to the
	// same schema document"; §4.2.6.2's note wants repeated <import>s of one
	// document not to trip sch-props-correct clause 2 either). It is a LOAD-ONCE
	// index, not a cycle guard (STYLE D4): <include> cycles are spec-legal —
	// §4.2.3 states the same schema corresponds to every document in the cycle —
	// and are not detected, merely loaded once.
	//
	// It indexes the DISCOVERY, not just the fact of it, because a second
	// directive landing on an existing key still has something to contribute: the
	// ·redefinition· it applies is appended to that discovery's readings
	// ([discovered].rds). D2's own targetNamespace is read back off the discovery's
	// document rather than stored beside it (STYLE D3), which is what lets a
	// repeated <import> be judged against src-import clause 3 without re-reading
	// D2 (#275).
	loaded map[docKey]*discovered

	// docs holds every discovered document in discovery order: depth-first,
	// pre-order, over each <schema>'s <include>, <override> and <import> children
	// in a single document-order pass. That order is the order components enter
	// the builder, so it is user-visible in sch-props-correct duplicate reports
	// (STYLE D1/D2) and must not be made to depend on which KIND of directive was
	// seen.
	//
	// The entries are POINTERS, shared with loaded: a discovery is written back to
	// after it is appended — its census, its own <redefine> sets, and the readings
	// a later directive merges into it — and a second copy would silently drop
	// those writes.
	docs []*discovered

	// unfollowed holds every ·inter-schema-document reference· (§4.2.1) that
	// yielded no document, in encounter order, for [AssemblyReport.Unfollowed].
	// It is not redundant with docs (STYLE D3): a directive that named no
	// document contributes nothing TO docs, which is exactly why it needs its own
	// record.
	unfollowed []UnfollowedDirective

	// unfollowedRoots holds every [HintAt] root the resolver returned no document
	// for, in caller order, for [AssemblyReport.UnfollowedRoots]. It is a channel
	// of its own rather than more unfollowed entries because a root has no
	// directive element to be one (STYLE D3): nothing else records it.
	unfollowedRoots []Root
}

// report renders the assembly's discovery state as the [AssemblyReport]
// [ParseSet] returns. It is built from the docs SLICE, in append order, never
// from the loaded index — a map iteration would make the reported order
// nondeterministic (STYLE D2).
func (a *assembly) report() *AssemblyReport {
	docs := make([]AssembledDocument, 0, len(a.docs))
	for _, d := range a.docs {
		docs = append(docs, AssembledDocument{Doc: d.doc, Location: d.resolved, Unmapped: d.unmapped})
	}
	return &AssemblyReport{documents: docs, unfollowed: a.unfollowed, unfollowedRoots: a.unfollowedRoots}
}

// unfollowedAt records that the directive at el yielded no document, for reason.
func (a *assembly) unfollowedAt(el *Element, reason UnfollowedReason) {
	a.unfollowed = append(a.unfollowed, UnfollowedDirective{Reason: reason, At: el.Loc()})
}

// newAssembly returns the empty assembly cfg configures. The load-once index is
// filled by discover alone, the root included: every document of an assembly
// enters it exactly where it is appended to docs, so no key can be claimed for a
// discovery that never happens (STYLE T1).
func newAssembly(cfg config) *assembly {
	return &assembly{
		resolver: cfg.resolver,
		log:      cfg.log,
		loaded:   make(map[docKey]*discovered),
	}
}

// discover appends doc to the assembly under key — the load-once identity this
// reading of it was fetched as — tns — its effective target namespace — ov — the
// ·override pre-processing· in force over it — and rd — the ·redefinition· in
// force over this reading, nil for a plain one. It claims key for the discovery,
// and then, in ONE document-order pass, follows each of the document's top-level
// <include> (§4.2.3: schema(D1) contains immed(D1) plus the components of
// schema(D2) for each <include>d D2), <override> (§4.2.5: plus the components of
// schema(override(E,Dold))) and <import> (§4.2.6.2: plus a set of components
// identical to those of each imported S2) children depth-first. The three
// directive kinds share the pass so that component entry order stays document
// order rather than becoming directive-kind-dependent (STYLE D1).
//
// A later directive resolving to key does not discover the document again; fetch
// merges that reading's redefinition into this discovery instead.
func (a *assembly) discover(doc *Document, key docKey, tns string, ov *overrideSet, rd *redefineSet) error {
	// The discovery this document's own <redefine> sets are recorded on. The
	// recursion below appends further documents, so it is held as the pointer
	// docs and loaded share rather than as a position into either.
	self := &discovered{doc: doc, tns: tns, resolved: key.resolved, ov: ov, rds: redefinitions{rd}}
	a.loaded[key] = self
	a.docs = append(a.docs, self)
	for _, child := range doc.Root().Children() {
		el, ok := child.(*Element)
		if !ok {
			continue
		}
		switch {
		case isXSD(el, "include"):
			// §F.2 clause 3: an <include> inside an overridden document becomes an
			// <override> carrying the same children, so ov CASCADES into the included
			// document — that is what makes §4.2.5's ·target set· transitive over
			// inclusion.
			if err := a.compose(el, tns, ov, nil, ruleSrcInclude); err != nil {
				return err
			}
		case isXSD(el, "override"):
			if err := a.override(el, tns, ov); err != nil {
				return err
			}
		case isXSD(el, "import"):
			// §F.2 clause 5: an <import> is copied unchanged, so ov does NOT cascade
			// through it — §4.2.5's ·target set· "does not include schema documents
			// which are pointed to by <import> or <redefine> elements".
			if err := a.importDocument(el, tns); err != nil {
				return err
			}
		case isXSD(el, "redefine"):
			set, err := a.redefine(el, tns)
			if err != nil {
				return err
			}
			if set != nil {
				self.redefines = append(self.redefines, set)
			}
		}
	}
	return nil
}

// redefine follows one <redefine> element (§4.2.4). It reads the redefining
// declarations the element itself declares and hands them to compose, which
// fetches the redefined document, enforces src-redefine clauses 1 and 2 on it,
// and discovers it under the set — after which the redefined document's producer
// excepts the redefined names (clause 4.1.2) and records each as the original the
// redefinition is paired with (src-expredef).
//
// Clause 2 is src-include clause 2 in every particular — same three sub-clauses,
// same chameleon case (clause 3.3 there, §F.1) — so <redefine> shares the one
// composer with <include> and <override>, told only which rule to charge.
//
// The set returned is the one the REDEFINING document's producer needs, and nil
// for an empty <redefine>, which redefines nothing and is a plain <include>.
//
// ov is deliberately NOT passed on: §F.2 clause 5 copies a <redefine> element
// unchanged, and §4.2.5's ·target set· "does not include schema documents which
// are pointed to by <import> or <redefine> elements", so an override in force
// over the redefining document does not cascade into the redefined one. It DOES
// still substitute for the <redefine>'s own children, which are element
// information items among the children of a <redefine> within the overridden
// document and so fall under §F.2 clause 1 — that half is applied at produce
// time (producer.prescanRedefine).
func (a *assembly) redefine(el *Element, tns string) (*redefineSet, error) {
	set, err := newRedefineSet(el)
	if err != nil {
		return nil, err
	}
	if err := a.compose(el, tns, nil, set, ruleSrcRedefine); err != nil {
		return nil, err
	}
	return set, nil
}

// override follows one <override> element (§4.2.5). It reads the substitutions
// the element itself declares, composes them with ov — whatever override is
// already in force over the containing document — per §F.2 clause 4, and hands
// the result to compose, which fetches the overridden document and enforces
// src-override clauses 1 and 2 on it.
//
// Those two clauses are src-include clauses 1 and 2 in every particular, which is
// no coincidence: §4.2.5 clause 3.1.2 (and 3.2.2 for the chameleon case) define
// the <override> as "replaced by an <include> element pointing to Dold′", with
// "the inclusion … handled as described in [§4.2.3]". Hence one composer for
// both, told only which rule to charge.
//
// The merged set is handed ONLY to the overridden document; the document that
// CONTAINS this <override> keeps ov, the override in force over it. Substitution
// therefore cannot leak back into the overriding document, whatever cycle of
// mutual overrides the schema documents form (PRINCIPLES 16).
func (a *assembly) override(el *Element, tns string, ov *overrideSet) error {
	own, err := newOverrideSet(el)
	if err != nil {
		return err
	}
	return a.compose(el, tns, own.mergedUnder(ov), nil, ruleSrcOverride)
}

// compose follows one <include>, <override> or <redefine> element: it resolves
// the schemaLocation, reads the document it names, enforces clauses 1 and 2 of
// rule — src-include (§4.2.3), src-override (§4.2.5) or src-redefine (§4.2.4),
// whose clauses coincide — on it, and recurses into its own directives under ov
// and rd. tns is the COMPOSING document's effective target namespace, which is
// both what clause 2 is judged against and what the composed document is
// discovered under (§F.1 coercion for the chameleon case).
//
// At most one of ov and rd is ever non-nil: <redefine> is copied unchanged by
// §F.2 clause 5 (so no override cascades through it) and its own children are
// not a further override.
func (a *assembly) compose(el *Element, tns string, ov *overrideSet, rd *redefineSet, rule xsderr.Rule) error {
	hint, ok := el.Attr("schemaLocation")
	if !ok {
		// schemaLocation is REQUIRED on <include>, <override> and <redefine> by the
		// schema for schema documents, and src-include/src-override/src-redefine
		// govern only what its actual value resolves to. A missing required attribute
		// is therefore a grammar fault with no dedicated Schema Representation
		// Constraint, reported as a plain error — the same treatment a <group> with
		// no ref gets in produce_complex.go. It is ALSO a reference this assembly
		// could not follow, and is recorded as one: the error says the assembly
		// stopped, the report says which document set it stopped short of (§4.2.1
		// makes these schemaLocations mandatory, "not hints").
		//
		// Each directive this composer follows IS its own Appendix A production — a
		// global element declaration carrying its content model inline — and each
		// declares schemaLocation use="required": xs:include (xmlschema11-1.md:5535,
		// the attribute at :5543), xs:redefine (:5548, :5560), xs:override (:5567,
		// :5579). Hence the production name below is the element name. <import> is
		// the one directive whose schemaLocation is optional (xs:import, :5595, since
		// §4.2.6.2 admits a bare <import>) and never reaches here: discover routes it
		// to importDocument.
		a.unfollowedAt(el, UnfollowedNoSchemaLocation)
		return fmt.Errorf("parser: <%s> at %s has no schemaLocation attribute, which the schema for schema documents requires: xs:%s declares it use=\"required\"", el.Name().Local(), el.Loc(), el.Name().Local())
	}
	// §4.3.2 clause 4: the location is a URI REFERENCE, resolved against the base
	// URI in scope at the directive element itself (xml:base-aware), not against
	// the document URI.
	requested := schemaloc.Resolve(el.baseURI, hint)

	f, err := a.fetch(requested, tns, ov, rd, directive{el}, rule)
	if err != nil {
		return err
	}
	if !f.exists && rd.mustResolve() {
		// src-redefine clause 1, the one place <redefine> parts company with
		// <include>: "If there are any element information items among the [children]
		// other than <annotation> then the ·actual value· of the schemaLocation
		// [attribute] must successfully resolve". src-include clause 2.4 says the
		// opposite for an <include> — "It is not an error … to fail to resolve at
		// all" — so the two mechanisms are split here rather than sharing fetch's
		// silent-skip outcome. An EMPTY <redefine> has a nil set and takes the
		// <include> path, since clause 1's antecedent does not fire for it.
		return xsderr.New(ruleSrcRedefine, el.Loc(),
			"the schemaLocation %q of this non-empty <redefine> does not resolve to a schema document, but src-redefine clause 1 requires it to resolve successfully whenever the <redefine> has children other than <annotation>", requested)
	}
	if f.doc == nil {
		// Nothing to compose, from either outcome: the location did not resolve —
		// src-include clause 2.4 (and src-override clause 2.4) make that explicitly
		// not an error, D2 does not exist and clause 2's other sub-clauses do not
		// apply — or D2 exists but is already in the assembly under this very key,
		// which §4.2.3's and §4.2.5's notes on sch-props-correct clause 2 want
		// composed exactly once. fetch has already merged rd into that discovery's
		// readings, so a <redefine> taking this path still excepts and pairs.
		//
		// Unlike <import> (#275), the dedup outcome needs no clause 2 re-check here,
		// because the namespace half of the key IS tns and every way an entry lands
		// under {resolved, tns, ov} has already established that the document there
		// declares tns or no targetNamespace at all: a [RootAt] root is discovered under its
		// own (documentRoot.enter); an <include>/<override>/<redefine> read reaching a second
		// directive has passed the clause 2 test below, whose failure aborts the
		// assembly outright; and an <import> or [HintAt] read has passed src-import clause 3,
		// which requires D2's targetNamespace to equal the namespace attribute it is
		// keyed under (clause 3.1) or to be absent when there is none (clause 3.2).
		// Both cases are clause 2.1 (own == tns) or clause 2.2/2.3 (own absent), so
		// re-running the test on a dedup hit could not change the verdict.
		return nil
	}
	if !f.doc.IsSchema() {
		return xsderr.New(rule, el.Loc(),
			"schemaLocation %q resolves to a <%s> document element, but %s clause 1 requires it to resolve to a <schema> element information item", hint, f.doc.Root().Name().Local(), rule)
	}
	// D2 is compared against the composing document's EFFECTIVE namespace rather
	// than its raw targetNamespace attribute. Under chameleon inclusion the §F.1
	// transformation is applied to the whole composing document before its own
	// schema is computed, so its <include>s are evaluated as the coerced
	// document's: §4.2.3's recursion note spells out that "if A includes B and B
	// includes C … the effect is as if A included B' and B' included C'", with B'
	// carrying A's targetNamespace.
	own, hasOwn := f.doc.Root().Attr("targetNamespace")
	if hasOwn && own != tns {
		// src-include clause 2.1 (c-normi) fails (the two targetNamespaces differ,
		// or the composing document has none), 2.2 (c-normi2) fails (D2 has one),
		// 2.3 (c-chami) fails (D2 has one), and 2.4 fails (D2 exists); src-override
		// clause 2.1 (c-o-normir), 2.2 (c-o-normi2r) and 2.3 (c-o-chamir) fail for
		// exactly the same three reasons.
		return xsderr.New(rule, el.Loc(),
			"the schema document %q named by this <%s> has targetNamespace %q, but %s clause 2 requires it to be identical to the composing schema's %q or absent", requested, el.Name().Local(), own, rule, tns)
	}
	// Clause 2.1, 2.2, or — when D2 declares no targetNamespace and the composing
	// document has one — 2.3, whose §F.1 coercion is applied at production time:
	// either way D2 is discovered under the composing document's effective
	// namespace, carrying whatever override or redefinition is in force over it.
	return a.discover(f.doc, f.key, tns, ov, rd)
}

// importDocument follows one <import> element (§4.2.6.2). It enforces
// src-import-noselfimport (clause 1) against tns — the IMPORTING document's
// effective target namespace — resolves the schemaLocation hint if there is one,
// enforces src-import clauses 2 and 3 on what comes back, and discovers D2 under
// D2's OWN target namespace, never a coerced one.
//
// The ·absent· namespace is encoded as the empty string throughout, the same
// sentinel [loader.Resolver] documents, so clause 1's and clause 3's namespace
// comparisons are string comparisons; only the PRESENCE of the namespace
// attribute selects which sub-clause applies, since namespace="" is a legal (if
// unusual) xs:anyURI value and is not the same thing as no attribute at all.
func (a *assembly) importDocument(el *Element, tns string) error {
	namespace, hasNamespace := el.Attr("namespace")
	if err := checkNoSelfImport(el, namespace, hasNamespace, tns); err != nil {
		return err
	}
	hint, hasHint := el.Attr("schemaLocation")
	if !hasHint {
		// A bare <import> — namespace but no schemaLocation — is explicitly legal
		// (§4.2.6.2): it declares that references into namespace are expected
		// (src-resolve clause 4.2.2 / 4.1.2) without naming a document to satisfy
		// them from. There is nothing for the reference strategy to succeed at, so
		// clauses 2 and 3 do not apply and there is no D2 to discover. Deliberately
		// NOT routed through schemaloc.Resolve: that returns the IMPORTING
		// document's own base URI for an empty location, which would make a bare
		// import re-read the importer.
		a.unfollowedAt(el, UnfollowedNoLocation)
		a.log.Debug("import declares a namespace with no schemaLocation hint",
			"rule", string(ruleSrcImport), "namespace", namespace, "at", el.Loc().String())
		return nil
	}
	// §4.3.2 clause 4: a URI REFERENCE resolved against the base URI in scope at
	// the <import> element itself (xml:base-aware).
	requested := schemaloc.Resolve(el.baseURI, hint)

	// The resolver is asked under the IMPORT's namespace, not the importing
	// document's: that pair — target namespace and location hint — is exactly the
	// "application schema reference strategy" input clause 2 describes.
	f, err := a.fetch(requested, namespace, nil, nil, directive{el}, ruleSrcImport)
	if err != nil {
		return err
	}
	if !f.exists {
		// The reference strategy did not succeed, so neither clause 2.1 nor clause
		// 2.2 was satisfied: D2 does not exist and clause 3, which is gated on "If D2
		// exists", does not apply. §4.2.6.2: "It is not an error for the application
		// schema component reference strategy to fail."
		return nil
	}
	if f.doc == nil {
		// D2 exists but was already read under this same (resolved location,
		// namespace) key. §4.2.6.2's note licenses skipping the RE-COMPOSITION of
		// S2's components — that is what keeps repeated <import>s of one document off
		// sch-props-correct clause 2 (c-nmd) — but it does NOT license skipping
		// clause 3: "If D2 exists, that is, clause 2.1 or clause 2.2 above were
		// satisfied" is a fact about the resolved DOCUMENT, not about which <import>
		// element caused it to be read. So clause 3 still runs, against the
		// targetNamespace recorded for D2 when it was read (#275). A chameleon
		// <include> is what makes this reachable: it keys a no-targetNamespace
		// document under the includer's namespace, which a later <import> naming that
		// namespace then lands on.
		return checkImportedNamespace(el.Loc(), directiveWording, requested, namespace, hasNamespace, f.tns)
	}
	if !f.doc.IsSchema() {
		return xsderr.New(ruleSrcImport, el.Loc(),
			"schemaLocation %q resolves to a <%s> document element, but src-import clause 2 requires it to resolve to a <schema> element information item", hint, f.doc.Root().Name().Local())
	}
	if err := checkImportedNamespace(el.Loc(), directiveWording, requested, namespace, hasNamespace, f.tns); err != nil {
		return err
	}
	// Clause 3 holds, so f.tns is the namespace the import declared. D2 is
	// discovered under it: import applies NO §F.1 coercion, which is src-include
	// clause 2.3's alone, and no ·override pre-processing· either, since §F.2
	// clause 5 copies an <import> unchanged.
	return a.discover(f.doc, f.key, f.tns, nil, nil)
}

// checkNoSelfImport enforces src-import clause 1 (src-import-noselfimport,
// §4.2.6.2) on one <import>: a document may not import the namespace it is
// already in.
//
// tns is the importing document's EFFECTIVE target namespace, not its raw
// targetNamespace attribute: §4.2.3's note on chameleon inclusion makes the
// including document's namespace the coerced document's own, so a chameleon
// document importing the includer's namespace is a self-import too. tns is ""
// for the ·absent· namespace — the sentinel [loader.Resolver] documents — so a
// literal targetNamespace="" arrives here as absent and a bare <import> from
// such a document is charged clause 1.2, where a strict-literal reading of "has
// no targetNamespace" would not charge it; tracking literal presence would be a
// second encoding of namespace-absence (STYLE D3).
func checkNoSelfImport(el *Element, namespace string, hasNamespace bool, tns string) error {
	if hasNamespace {
		// Clause 1.1: the namespace attribute's value must NOT match the enclosing
		// <schema>'s effective targetNamespace.
		if namespace != tns {
			return nil
		}
		return xsderr.New(ruleSrcImportNoSelfImport, el.Loc(),
			"<import> names namespace %q, which is the importing schema's own target namespace; src-import clause 1.1 requires them to differ", namespace)
	}
	// Clause 1.2: with no namespace attribute the enclosing <schema> must have a
	// targetNamespace — a bare <import> from a no-namespace document would import
	// the no-namespace it is already in.
	if tns != "" {
		return nil
	}
	return xsderr.New(ruleSrcImportNoSelfImport, el.Loc(),
		"<import> has no namespace attribute, but the importing schema has no target namespace either; src-import clause 1.2 requires one")
}

// checkImportedNamespace enforces src-import clause 3 (§4.2.6.2) on a D2 that
// the reference strategy did produce: the namespace the <import> — or the
// [HintAt] root — declared and the one D2 actually declares must agree. own is
// D2's targetNamespace with the ·absent· namespace encoded as "". A violation is
// charged at at, worded by words.
func checkImportedNamespace(at xsderr.Loc, words importWording, requested, namespace string, hasNamespace bool, own string) error {
	if hasNamespace {
		// Clause 3.1: identical to D2's targetNamespace.
		if own == namespace {
			return nil
		}
		return xsderr.New(ruleSrcImport, at, words.mismatch, requested, own, namespace)
	}
	// Clause 3.2: with no namespace attribute, D2 must have no targetNamespace.
	if own == "" {
		return nil
	}
	return xsderr.New(ruleSrcImport, at, words.unexpected, requested, own)
}

// fetch resolves requested through the assembly's resolver, under namespace, and
// reads the schema document it names. It serves <include>, <override>, <import>
// and a [HintAt] root alike, so it is handed the rule the caller answers to and
// reads everything else that differs between them off ref rather than carrying a
// second, stringly-typed kind.
//
// namespace is what the resolver is asked under AND the namespace half of the
// load-once key: for <include>/<override> the composing document's effective
// target namespace (which is also what the composed document is discovered
// under), for <import> the import's own namespace attribute (which clause 3 then
// requires D2 to agree with). ov is the override half of that key and is nil for
// every directive but <override> (§F.2 clause 5 leaves an <import> unchanged).
//
// It reports the outcome as a [fetched], which distinguishes the two ways a
// directive can end in nothing to compose — outcomes neither the report nor the
// remaining clauses may conflate, which is why only the first is recorded, as an
// [UnfollowedDirective] or, for a hint root, among the unfollowed roots:
//
//   - the location does not resolve at all — §4.2.3: "It is not an error for the
//     actual value of the schemaLocation attribute to fail to resolve at all, in
//     which case the corresponding inclusion must not be performed" (src-include
//     clause 2.4), and §4.2.6.2 equally: "It is not an error for the application
//     schema component reference strategy to fail". D2 does not exist, so no
//     further clause bites, and no document was assembled for this directive, so
//     it is UnfollowedLocationUnresolved;
//   - the document was already loaded under this namespace and this override —
//     the same schema location names the same schema document (§4.2.3), §4.2.6.2's
//     note requires repeated <import>s of one document not to trip
//     sch-props-correct clause 2 either, and §4.2.5's note wants the same of
//     "multiple equivalent overrides of the same schema document", so it
//     contributes its components exactly once. D2 DOES exist here: only its
//     re-COMPOSITION is skipped, and rd is merged into that discovery's readings
//     first, so a <redefine> landing here still excepts and pairs what it names.
//     This directive WAS followed — to a document already in the report — so
//     nothing is recorded: reporting a dedup hit as unfollowed would mark almost
//     every multi-document assembly.
//
// Any OTHER resolver failure is real: a permission or transport error is not
// silently downgraded to "absent". It still yielded no document, so it is
// recorded too, under the same reason — the report says which references came
// back empty, not why the resolver said so.
//
// The load-once key travels back with the document because the RESOLVED location
// in it, not the requested one, is the assembly's document identity and the
// report's [AssembledDocument.Location].
func (a *assembly) fetch(requested, namespace string, ov *overrideSet, rd *redefineSet, ref referrer, rule xsderr.Rule) (fetched, error) {
	rc, resolved, err := a.resolver.Resolve(namespace, requested)
	if errors.Is(err, loader.ErrNotFound) {
		ref.unfollowed(a, UnfollowedLocationUnresolved)
		a.log.Debug("composition skipped: schemaLocation does not resolve",
			append([]any{"rule", string(rule), "namespace", namespace, "location", requested}, ref.logAttrs()...)...)
		return fetched{}, nil
	}
	if err != nil {
		ref.unfollowed(a, UnfollowedLocationUnresolved)
		return fetched{}, fmt.Errorf("parser: resolving %s: %w", ref.describe(requested), err)
	}
	// A read-only reader: a close failure cannot change what was already read, so
	// it cannot affect the parse verdict (STYLE S3).
	defer func() { _ = rc.Close() }()

	key := docKey{resolved: resolved, namespace: namespace, override: ov.key()}
	if prior, done := a.loaded[key]; done {
		// This directive's own ·redefinition· joins the readings of that discovery,
		// so the definitions it excepts are excepted here too and the ones it
		// replaces still record their originals — while the document itself is
		// composed exactly once (see docKey). rd is nil for every directive but a
		// non-empty <redefine>, and a nil reading excepts nothing.
		prior.rds = append(prior.rds, rd)
		a.log.Debug("schema document already loaded",
			append([]any{"namespace", namespace, "location", requested, "resolved", resolved}, ref.logAttrs()...)...)
		return fetched{key: key, tns: attrOr(prior.doc.Root(), "targetNamespace"), exists: true}, nil
	}

	doc, err := ReadDocument(requested, rc)
	if err != nil {
		// src-include clause 1.1 and src-import clause 2 alike require the resolved
		// resource to correspond to a <schema> item "in a well-formed information
		// set", so a well-formedness fault in a document that DID resolve is a
		// composition violation, not a bare XML error. The document is nonetheless
		// one the assembly never read, so the directive is recorded as unfollowed
		// alongside the verdict.
		ref.unfollowed(a, UnfollowedUnreadable)
		return fetched{}, xsderr.Wrap(rule, ref.unreadableAt(requested, err),
			fmt.Errorf("the schema document %q named by this %s is not well-formed, but %s requires a well-formed information set: %w", requested, ref.noun(), rule, err))
	}
	// §4.2.2: every composed document is pre-processed as it is read, on the same
	// terms as the root — "conditional-inclusion pre-processing is always performed
	// first", ahead of the chameleon coercion, redefinition and override
	// pre-processing that this document is about to be discovered under, and ahead
	// of its own directives being followed. A document made empty by version
	// control attributes contributes nothing and <include>s nothing.
	//
	// An src-cip fault here is NOT recorded as an unfollowed directive: the
	// directive was followed to a document that resolved and was read, and the
	// verdict is about that document, not about a reference that led nowhere.
	doc, err = conditionalInclude(doc)
	if err != nil {
		return fetched{}, err
	}
	// The key travels back with the document rather than being claimed here:
	// discover claims it as it appends the discovery a later directive landing on
	// this key merges into. D2's own targetNamespace travels back too, so a later
	// directive can be judged against D2 without re-reading it.
	return fetched{doc: doc, key: key, tns: attrOr(doc.Root(), "targetNamespace"), exists: true}, nil
}

// fetched is the outcome of one [assembly.fetch]: which of the three
// possibilities src-include clause 2 / src-import clause 2 distinguishes the
// reference strategy landed on. It is produced by fetch alone, so the illegal
// combinations (a doc with exists false, a doc that is not the one just read)
// cannot arise elsewhere.
type fetched struct {
	// doc is D2, non-nil ONLY when THIS directive read it. It is nil on a dedup
	// hit: D2 exists and is already in the assembly, and the §4.2.3/§4.2.5/§4.2.6.2
	// notes on sch-props-correct clause 2 are what license not composing its
	// components a second time.
	doc *Document

	// key is the load-once identity this reading of D2 was fetched under, whether
	// this directive read it or a previous one did. discover claims it, and its
	// resolved half is the report's [AssembledDocument.Location].
	key docKey

	// tns is D2's OWN targetNamespace, ·absent· encoded as "", read off D2 whether
	// this directive read it or a previous one did. It is what src-import clause 3
	// compares against, and it is NOT the namespace D2 was fetched under: a
	// chameleon <include> keys a no-targetNamespace document under the includer's
	// namespace (§F.1).
	tns string

	// exists reports whether D2 exists at all — whether src-import clause 2.1 or
	// 2.2 was satisfied, the condition clause 3 is gated on. It is not derivable
	// from doc: a dedup hit has a nil doc and an existing D2, and folding those two
	// into one nil sentinel is precisely what made clause 3 unreachable (#275).
	exists bool
}

// compile produces every discovered document into ONE builder — each under its
// OWN effective target namespace, so an imported document's components land in
// the namespace it declares rather than the root's — and finalizes it. Every
// document's pre-scan runs before any document is produced, so a base= or
// <attributeGroup ref> in one document reaches a definition contributed by
// another (§4.2.3 clause 3.1.2, c-incl-incl); the builtins are seeded once, by
// newSymbols, for the whole assembly. Every document's <defaultOpenContent> is
// judged in that same pre-produce pass (checkDefaultOpenContent), so §3.4.2.3.3
// clause 5.2 can only ever select one whose grammar already holds — including
// through an on-demand base build into a document not yet produced. Every
// document's two *FormDefault attributes are judged in that pass too
// (checkFormDefaults), for the content-independence reason that function's doc
// gives. backend supplies the finalized schema's value space too
// ([value.NewValueSpace]), so the finalize-time {value} comparisons
// (au-props-correct clause 3, loc-testSubP clauses 4.2/5.2.2) and the Simple
// Default Valid checks (a-props-correct clause 2, au-props-correct clause
// 2) decide rather than fail open — see [Produce] for the full statement.
//
// The XML namespace's built-in components are added once beside those
// builtins, by addXMLNamespace, and only HERE rather than at the <import> that
// asks for them: whether they are supplied at all turns on the FINISHED
// document set (supplyXMLNamespace), which no single <import> element knows.
//
// Each document's coverage census (producer.census) is taken in that same
// pre-produce pass, so [AssemblyReport] carries what every discovery holds that
// no dispatch maps whether or not the assembly went on to reject it.
func (a *assembly) compile(backend value.Backend) (*xsd.Schema, error) {
	builder := xsd.NewSchemaBuilder()
	sym, err := newSymbols(builder, backend)
	if err != nil {
		return nil, err
	}
	if err := addXMLNamespace(builder, a.composedDocuments()); err != nil {
		return nil, err
	}
	producers := make([]*producer, 0, len(a.docs))
	for _, d := range a.docs {
		p := newProducer(d.doc, d.tns, d.ov, d.rds, d.redefines, builder, sym)
		// Taken before the first thing that can fail, so a document its own
		// pre-scan rejects still reports what it holds — the report is populated
		// as far as assembly got on every path ([AssemblyReport]).
		d.unmapped = p.census()
		if err := p.prescan(); err != nil {
			return nil, err
		}
		if err := p.checkDefaultOpenContent(); err != nil {
			return nil, err
		}
		if err := p.checkFormDefaults(); err != nil {
			return nil, err
		}
		producers = append(producers, p)
	}
	for i, p := range producers {
		a.log.Debug("producing schema document",
			"uri", a.docs[i].doc.URI(), "targetNamespace", a.docs[i].tns, "chameleon", p.chameleon())
		if err := p.run(); err != nil {
			return nil, err
		}
	}
	return builder.FinalizeWith(value.NewValueSpace(backend), builtin.NewRestrictionChecker(backend))
}

// readRootDocument resolves the location a [RootAt] root names and reads the
// schema document it names, returning the document and its RESOLVED location — the
// load-once dedup key the [loader.Resolver] contract defines, which seeds the
// assembly's index so an <include> pointing back at the root does not re-load
// it. It is the ROOT counterpart of assembly.fetch and deliberately does not
// share its error policy: every failure here is real, because the caller named a
// document that must exist, whereas §4.2.3 makes an unresolvable <include>
// location a no-op.
//
// The resolver is asked under the no-namespace sentinel "": the document's own
// targetNamespace is only knowable after it has been read, so there is nothing
// truthful to pass.
func readRootDocument(r loader.Resolver, location string) (*Document, string, error) {
	rc, resolved, err := r.Resolve("", location)
	if err != nil {
		return nil, "", err
	}
	// Read-only reader; a close failure cannot change what was read (STYLE S3).
	defer func() { _ = rc.Close() }()
	doc, err := ReadDocument(location, rc)
	if err != nil {
		return nil, "", err
	}
	return doc, resolved, nil
}
