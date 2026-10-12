package parser

import (
	"errors"
	"fmt"

	"github.com/kud360/goxsd8/xsderr"
)

// Root is one root location of the document set [ParseSet] assembles: a schema
// document the caller names outright ([RootAt]) or a schema location hint, a
// location paired with the namespace it is expected to declare ([HintAt]). It
// is a closed set (STYLE T2's sealed-sum exception), so the two constructors
// are the only way to build one.
//
// Which composition a root gets is derived from which constructor built it,
// never from a flag: a RootAt root is read exactly as [ParseReport] reads its
// root, in its own target namespace, and a HintAt root is followed exactly as an
// <xs:import> of that namespace is (§4.2.6.2), checked against src-import clause
// 3.
//
// Root values are comparable, and [AssemblyReport.UnfollowedRoots] hands back
// the caller's own values, so a caller identifies a root it passed by comparing
// it with ==, or names it by Location.
type Root interface {
	// Location returns the location the root was constructed with, as the
	// caller spelled it.
	Location() string

	// enter adds the root's document, and its closure, to a. It is the seal as
	// well as the behavior: nothing outside this package can implement it.
	enter(a *assembly) error
}

// RootAt returns the Root naming the schema document at location — resolved
// through the [loader.Resolver] under the no-namespace sentinel and read as
// [ParseReport] reads its root: conditional inclusion first (§4.2.2), then a
// document element that must be <schema>, whose own targetNamespace its
// components are minted in. A failure to resolve or read it is a plain I/O error,
// as it is for [ParseReport], since the caller named a document that must exist;
// it is therefore never among [AssemblyReport.UnfollowedRoots].
func RootAt(location string) Root { return documentRoot{location: location} }

// HintAt returns the Root naming the schema document at location as a schema
// location hint for namespace — §4.3.2 clause 3's warrant, the (namespace,
// location) pair xsi:schemaLocation carries, with "" standing for the ·absent·
// namespace xsi:noNamespaceSchemaLocation implies.
//
// It is followed exactly as an <xs:import> naming that namespace and location
// (§4.2.6.2), with no importing document: the resolver is asked under namespace,
// a location that resolves to no document is legal to skip and is recorded among
// [AssemblyReport.UnfollowedRoots], a document that cannot be read or whose
// element is not <schema> is src-import clause 2, and one whose targetNamespace
// disagrees is src-import clause 3.1 (namespace non-empty) or 3.2 (namespace "")
// — judged even when the document is already in the assembly, as for a repeated
// <xs:import>. Clause 1 does not apply: there is no importing document to import
// its own namespace.
func HintAt(namespace, location string) Root {
	return hintRoot{namespace: namespace, location: location}
}

// documentRoot is the Root [RootAt] builds.
type documentRoot struct {
	location string
}

// Location returns the location RootAt was given.
func (r documentRoot) Location() string { return r.location }

// enter reads the document at r.location and discovers it under its own
// effective namespace, with the nil (identity) override set and the nil
// (identity) redefinition: nothing substitutes for, or excepts, its own
// declarations.
//
// A root whose key is already loaded — a repeated root, or a document another
// root's closure already reached plainly under the same namespace — is a dedup
// hit, exactly as a directive landing on that key is in fetch: its plain reading
// joins the discovery's readings, so a document first reached as an
// <xs:redefine> target keeps the definitions this reading does not except
// (#1349), and is not composed a second time.
func (r documentRoot) enter(a *assembly) error {
	doc, resolved, err := readRootDocument(a.resolver, r.location)
	if err != nil {
		return fmt.Errorf("parser: reading root schema document %q: %w", r.location, err)
	}
	// §4.2.2: the root is pre-processed the moment it is read, before its own
	// directives are followed and before any rule is read against it. The verdict
	// travels back unwrapped — an src-cip fault is a schema-validity verdict about
	// a document that WAS read, not the plain I/O error a root that cannot be
	// reached returns above.
	doc, err = conditionalInclude(doc)
	if err != nil {
		return err
	}
	if !doc.IsSchema() {
		return fmt.Errorf("parser: <%s> at %s is not a <schema> document root: assembling a schema requires one", doc.Root().Name().Local(), doc.Root().Loc())
	}
	// The root document's effective target namespace is its own: there is no
	// including document to borrow one from (§4.2.3 clause 2.3 needs one).
	tns := attrOr(doc.Root(), "targetNamespace")
	key := docKey{resolved: resolved, namespace: tns}
	if prior, done := a.loaded[key]; done {
		prior.rds = append(prior.rds, nil)
		a.log.Debug("root schema document already loaded", "location", r.location, "resolved", resolved)
		return nil
	}
	return a.discover(doc, key, tns, nil, nil)
}

// hintRoot is the Root [HintAt] builds. namespace is "" for the ·absent·
// namespace, which selects src-import clause 3.2 over 3.1.
type hintRoot struct {
	namespace string
	location  string
}

// Location returns the location HintAt was given.
func (r hintRoot) Location() string { return r.location }

// enter follows the hint as an <xs:import> of r.namespace from no document at
// all — importDocument's path from its fetch onward, sharing fetch and
// checkImportedNamespace rather than restating them (STYLE T4).
//
// Every verdict is charged at the hinted document's own element: there is no
// directive element to charge it at, and the hinted document is the one a reader
// can open (STYLE E3). On a dedup hit that is the element of the discovery the
// hint landed on.
func (r hintRoot) enter(a *assembly) error {
	f, err := a.fetch(r.location, r.namespace, nil, nil, r, ruleSrcImport)
	if err != nil {
		return err
	}
	if !f.exists {
		// §4.2.6.2: "It is not an error for the application schema component
		// reference strategy to fail." fetch has recorded the root.
		return nil
	}
	if f.doc == nil {
		// Already in the assembly under this (resolved location, namespace) key:
		// only the re-composition is skipped, and clause 3 still runs (#275).
		return checkImportedNamespace(a.loaded[f.key].doc.Root().Loc(), hintWording, r.location, r.namespace, r.namespace != "", f.tns)
	}
	at := f.doc.Root().Loc()
	if !f.doc.IsSchema() {
		return xsderr.New(ruleSrcImport, at,
			"the schema location hint %q resolves to a <%s> document element, but src-import clause 2 requires it to resolve to a <schema> element information item", r.location, f.doc.Root().Name().Local())
	}
	if err := checkImportedNamespace(at, hintWording, r.location, r.namespace, r.namespace != "", f.tns); err != nil {
		return err
	}
	return a.discover(f.doc, f.key, f.tns, nil, nil)
}

// describe names the hint for a resolver fault's context.
func (r hintRoot) describe(requested string) string {
	return fmt.Sprintf("schema location hint %q for namespace %q", requested, r.namespace)
}

// noun names the hint inside a verdict's message.
func (hintRoot) noun() string { return "schema location hint" }

// logAttrs are the hint's attributes on a debug line.
func (r hintRoot) logAttrs() []any { return []any{"hint", r.location} }

// unreadableAt charges a hinted document that could not be read at the reader's
// own fault position when it has one — a well-formedness fault inside the
// hinted document — and at the document itself otherwise.
func (hintRoot) unreadableAt(requested string, err error) xsderr.Loc {
	var e *xsderr.Error
	if errors.As(err, &e) {
		return e.Loc
	}
	return xsderr.Loc{URI: requested}
}

// unfollowed records the hint among [AssemblyReport.UnfollowedRoots] when the
// resolver returned no document for it. A document that resolved and could not
// be read is not recorded: it yields src-import clause 2, which names it.
func (r hintRoot) unfollowed(a *assembly, reason UnfollowedReason) {
	if reason != UnfollowedLocationUnresolved {
		return
	}
	a.unfollowedRoots = append(a.unfollowedRoots, r)
}

// referrer is the reference assembly.fetch follows: a directive element of a
// document already in the assembly ([directive]) or a [HintAt] root of the
// caller's set (hintRoot). fetch reads through it everything that differs
// between the two — how the reference is named in an error or a log line, where
// an unreadable document is charged, and which report channel records a
// reference that yielded no document — so both share one fetch (STYLE T4).
type referrer interface {
	// describe names the reference, resolved to requested, for a resolver
	// fault's context.
	describe(requested string) string
	// noun names the reference inside a verdict's message.
	noun() string
	// logAttrs are the reference's own attributes on a debug line.
	logAttrs() []any
	// unreadableAt is where src-include clause 1.1 / src-import clause 2 is
	// charged against the document at requested, which err says could not be
	// read.
	unreadableAt(requested string, err error) xsderr.Loc
	// unfollowed records on a that the reference yielded no document, for reason.
	unfollowed(a *assembly, reason UnfollowedReason)
}

// directive is the referrer for an <include>, <override>, <redefine> or
// <import> element: everything fetch says about the reference is said at the
// element, and a reference that yielded no document is an [UnfollowedDirective].
type directive struct {
	el *Element
}

// describe names the directive and its position.
func (d directive) describe(requested string) string {
	return fmt.Sprintf("<%s> schemaLocation %q at %s", d.el.Name().Local(), requested, d.el.Loc())
}

// noun names the directive element.
func (d directive) noun() string { return "<" + d.el.Name().Local() + ">" }

// logAttrs are the directive's kind and position.
func (d directive) logAttrs() []any {
	return []any{"directive", d.el.Name().Local(), "at", d.el.Loc().String()}
}

// unreadableAt charges the directive element itself.
func (d directive) unreadableAt(string, error) xsderr.Loc { return d.el.Loc() }

// unfollowed records the directive among [AssemblyReport.Unfollowed].
func (d directive) unfollowed(a *assembly, reason UnfollowedReason) {
	a.unfollowedAt(d.el, reason)
}

// importWording is how src-import clause 3's two verdicts word what declared the
// namespace — an <import> element's namespace attribute, or the namespace a
// hint pairs its location with. Each is a format string taking the requested
// location and D2's own targetNamespace, and mismatch then the declared
// namespace.
type importWording struct {
	mismatch   string
	unexpected string
}

// The two importWordings, one per referrer that reaches clause 3.
var (
	directiveWording = importWording{
		mismatch:   "<import>ed schema document %q has target namespace %q, but src-import clause 3.1 requires it to be identical to the namespace attribute's %q",
		unexpected: "<import> has no namespace attribute, but the schema document %q it names has target namespace %q; src-import clause 3.2 requires it to have none",
	}
	hintWording = importWording{
		mismatch:   "the schema document %q this schema location hint names has target namespace %q, but src-import clause 3.1 requires it to be identical to the hint's namespace %q",
		unexpected: "the schema document %q this schema location hint names without a namespace has target namespace %q; src-import clause 3.2 requires it to have none",
	}
)
