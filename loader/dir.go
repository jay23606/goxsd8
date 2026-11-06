package loader

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// dirResolver serves locations as paths under a filesystem root; see Dir.
type dirResolver struct {
	root string
}

// Dir returns a Resolver that serves each location as a path relative to
// the root directory path.
//
// location is treated as an untrusted, possibly attacker-supplied hint
// (xsi:schemaLocation flows through the same seam), so Dir refuses any
// location that would escape root via ".." segments, mapping such attempts
// to ErrNotFound, and joins an absolute location beneath root like any
// other: "/tmp/a.xsd" names root/tmp/a.xsd. This traversal defense is an
// engineering decision, NOT a spec rule — no XSD text governs it.
//
// Choosing to confine is engineering; how a consumer must treat the refusal
// that results is not. §4.2.6.2 (src-import) names URI-to-document resolution
// "the application schema component reference strategy" — Dir's confined-to-root
// serving IS that strategy for its callers — and rules that "it is not an error
// for the application schema component reference strategy to fail"; §4.2.3
// (src-include) clause 2.4 says the same of a schemaLocation that "does not
// resolve successfully", enumerating no reasons and distinguishing none. A
// caller may therefore treat a refusal here exactly as it treats a document that
// is genuinely absent — compose nothing, report no error — which is what
// parser's assembly.fetch and the conformance closure walk both do (issue #257).
//
// The resolved location Dir returns is CANONICAL in on-disk case: on a
// case-insensitive filesystem two location hints that differ only in case
// open the same file, and Dir reports the same identity string for both, so
// Resolver's dedup contract holds and the document composes once instead of
// twice. Like the traversal defense above this is engineering, not a spec
// rule: §4.2.3 (src-include) and §4.2.6.2 (src-import) mandate only that
// lexically identical locations name the same document, and their Notes
// merely encourage — never require — an implementation to recognize when
// two different location strings name one resource.
func Dir(path string) Resolver {
	return dirResolver{root: path}
}

// Resolve implements Resolver. The resolved location it returns is the
// joined path with each segment spelled as it is on disk (see canonicalCase),
// so differently-cased hints for one file share one identity.
func (d dirResolver) Resolve(namespace, location string) (io.ReadCloser, string, error) {
	root := filepath.Clean(d.root)
	full := filepath.Join(root, location)

	// Confinement check (engineering, not spec-derived): the cleaned join
	// must remain inside root. filepath.Join has already collapsed ".."
	// segments, so a location that climbs out yields a rel starting "..".
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, "", fmt.Errorf("loader: location %q escapes root %q: %w", location, d.root, ErrNotFound)
	}

	f, err := os.Open(full)
	if os.IsNotExist(err) {
		return nil, "", fmt.Errorf("loader: %q not found under %q: %w", location, d.root, ErrNotFound)
	}
	if err != nil {
		return nil, "", fmt.Errorf("loader: opening %q under %q: %w", location, d.root, err)
	}
	// f streams from disk; the caller closes it (P4 — no buffering here).
	// rel is reused, not recomputed: it is the confinement check's own output.
	return f, canonicalCase(root, rel), nil
}

// canonicalCase returns root joined with rel, each of rel's segments spelled
// as the directory holding it spells it on disk. Two location strings naming
// one file through different case — which a case-insensitive filesystem opens
// alike — therefore produce the same identity string, the dedup key Resolver
// documents and parser's docKey relies on.
//
// rel is ALREADY the confined, non-escaping relative path filepath.Rel
// produced, so the confinement check has run: segment count and the absence of
// "." / ".." segments are invariant here, and only a segment's SPELLING is
// ever refined. Substitutes come from the parent directory's own listing,
// which never yields "." or "..", so confinement cannot be reopened.
//
// It is best-effort and total — no error return. It degrades to rel's own
// casing, unchanged, whenever the on-disk name cannot be confirmed: a
// directory that cannot be listed (a search-only 0111 mode is
// openable-through but not listable) or a Unicode fold that open(2) accepted
// but [strings.EqualFold] does not agree with (Turkish dotless i, HFS+
// normalization). The caller has already opened this path successfully, so
// returning its own casing on degradation is the previously-correct answer,
// never a new failure mode.
func canonicalCase(root, rel string) string {
	if rel == "" || rel == "." {
		return root
	}
	segs := strings.Split(rel, string(filepath.Separator))
	dir := root
	for i, seg := range segs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			// Degrade: keep this and every remaining segment as given.
			return filepath.Join(append([]string{dir}, segs[i:]...)...)
		}
		dir = filepath.Join(dir, matchEntryName(entries, seg))
	}
	return dir
}

// matchEntryName returns the name entries spells want with: an EXACT match
// wins, otherwise the FIRST case-insensitive match in entries' own order
// ([os.ReadDir] sorts by filename), and want unchanged when nothing matches
// (canonicalCase's degrade path). Scanning in that order rather than indexing
// a map keyed by folded name keeps the pick deterministic (STYLE D1/D2): on a
// case-sensitive filesystem two distinct entries can fold-equal, and a map
// would make which one wins depend on iteration order.
func matchEntryName(entries []os.DirEntry, want string) string {
	for _, e := range entries {
		if e.Name() == want {
			return want
		}
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), want) {
			return e.Name()
		}
	}
	return want
}
