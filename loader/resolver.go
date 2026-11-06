package loader

import (
	"errors"
	"fmt"
	"io"
)

// ErrNotFound is the sentinel a Resolver returns when it has no answer for
// a (namespace, location) request. Chain uses errors.Is on it to fall
// through to the next resolver, and callers (the M4 parser) use it to
// decide that an unresolved schemaLocation hint is a normal, non-fatal
// outcome per §4.2.6.2 (src-import) and §4.3.2 clause 3 — this package
// does not decide fatality.
var ErrNotFound = errors.New("loader: schema document not found")

// Resolver answers "give me the schema document for (target namespace,
// location hint)". The returned string is the RESOLVED location — the
// dedup key: the loader loads each resolved location once, so a document
// named by several imports/hints composes instead of duplicating.
//
// That makes the string an IDENTITY for the underlying resource, and the
// obligation on it is capability-gated exactly as §4.2.3 (src-include) gates
// it: two different schema locations name two different schema documents
// "unless the implementation is able to determine that the two URIs are
// references to the same resource". So a Resolver that CAN determine two
// location strings name one resource must return the SAME string for both —
// Dir does, respelling each path segment as the filesystem lists it. A
// Resolver that cannot, or does not attempt to, simply returns the location
// as given (FS and HTTP do), and the loader composes that document more than
// once, which §4.2.3 and §4.2.6.2 permit — their Notes encourage collapsing
// spellings, they never require it. A third-party implementation adapted
// through ResolverFunc sits on exactly this footing.
//
// The empty string is the well-defined "no namespace" state (absence of
// targetNamespace, §4.2.6.2 / §4.3.2), so namespace == "" is the sentinel
// for the no-namespace case, not a missing argument.
//
// Which namespace a request carries depends on what made it. Package parser
// makes four kinds of request:
//
//   - an xs:import asks under its namespace attribute, "" when it has none (an
//     xs:import with no schemaLocation asks nothing);
//   - a parser.HintAt root asks under the namespace it was given;
//   - an xs:include, xs:redefine or xs:override asks under the including
//     document's EFFECTIVE target namespace: its declared one or, for a
//     document itself chameleon-included, the namespace it was included under,
//     so a chameleon target is asked under a namespace it does not declare;
//   - a parser.RootAt root asks under "" whatever the document declares.
//
// A resolver keyed on namespace, such as a catalog, must therefore not treat
// "" or a namespace the document does not declare as "not mine" for an
// include-family or RootAt request.
//
// The location argument is ALREADY RESOLVED against the owner element's
// base URI: §4.3.2 clause 4 makes relative-reference resolution the
// caller's (parser's) job, so Resolve is location-hint-in / reader-out and
// performs no URI-relative resolution of its own.
//
// There is deliberately no context.Context parameter: library code here
// runs no concurrency of its own (STYLE D5), and callers bound blocking
// behavior through the injected dependency — for HTTP, the *http.Client's
// Transport timeouts; a caller needing cancellation wraps Resolve in its
// own goroutine, which is its business, not this seam's.
type Resolver interface {
	Resolve(namespace, location string) (io.ReadCloser, string, error)
}

// ResolverFunc adapts an ordinary function to Resolver, in the
// http.HandlerFunc idiom.
type ResolverFunc func(namespace, location string) (io.ReadCloser, string, error)

// Resolve calls f.
func (f ResolverFunc) Resolve(namespace, location string) (io.ReadCloser, string, error) {
	return f(namespace, location)
}

// chainResolver tries its resolvers in slice order; see Chain.
type chainResolver struct {
	resolvers []Resolver
}

// Chain returns a Resolver that tries rs in order. The first resolver that
// does not return ErrNotFound wins — its reader, resolved location, or real
// error is returned immediately.
//
// Only ErrNotFound triggers fall-through: a real (non-ErrNotFound) I/O
// error from any resolver short-circuits the chain and is returned as-is,
// so a permission-denied or malformed-response failure is never masked by a
// later resolver that happens to hold a copy. When every resolver returns
// ErrNotFound the per-resolver failures are aggregated with errors.Join
// (which still satisfies errors.Is(err, ErrNotFound)); an empty chain
// resolves nothing and returns ErrNotFound.
func Chain(rs ...Resolver) Resolver {
	return chainResolver{resolvers: rs}
}

// Resolve implements Resolver.
func (c chainResolver) Resolve(namespace, location string) (io.ReadCloser, string, error) {
	errs := make([]error, 0, len(c.resolvers))
	for _, r := range c.resolvers {
		rc, resolved, err := r.Resolve(namespace, location)
		if err == nil {
			return rc, resolved, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, "", err
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil, "", fmt.Errorf("loader: resolving %q in namespace %q: %w", location, namespace, ErrNotFound)
	}
	return nil, "", errors.Join(errs...)
}
