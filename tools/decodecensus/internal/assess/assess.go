// Package assess is the half of `go tool decodecensus` that runs inside its
// driver: the program the census compiles, in a temporary module, beside the
// packages codegen emitted, so that codec.Decode can be instantiated with each
// generated type. Its importers are that driver's generated main package,
// which calls [Drive] with one [DecoderFor] Decoder per type, and the
// census command, which calls [Assemble] and writes the [Request] Drive reads.
// The command's package doc owns the population, the assembly and the report;
// this package owns how one instance is classified, so that the tests that
// pin each class run the classifier the driver runs.
package assess

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/codec"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// Assemble assembles the schema of one test group from docs, its
// <schemaDocument> list as slash paths relative to the suite at root, with
// backend: parser.ParseReport of the first document through loader.Dir over
// that document's own directory, under its base name, then, when a further
// document is one that closure did not read, parser.ParseSet of the first and
// each such further one, each named by its path relative to the first's
// directory. A further document outside that directory's tree does not
// resolve, and fails the assembly.
func Assemble(root string, docs []string, backend value.Backend) (*xsd.Schema, error) {
	if len(docs) == 0 {
		return nil, errors.New("assess: no schema document to assemble")
	}
	first := filepath.Join(root, filepath.FromSlash(docs[0]))
	dir := filepath.Dir(first)
	resolver := loader.Dir(dir)
	location := filepath.Base(first)
	schema, report, err := parser.ParseReport(location, parser.WithResolver(resolver), parser.WithBackend(backend))
	if err != nil {
		return nil, err
	}
	roots := []parser.Root{parser.RootAt(location)}
	for _, doc := range docs[1:] {
		rel, err := filepath.Rel(dir, filepath.Join(root, filepath.FromSlash(doc)))
		if err != nil {
			return nil, fmt.Errorf("assess: relating %s to %s: %w", doc, dir, err)
		}
		extra := filepath.ToSlash(rel)
		reached, err := closureReached(resolver, report, extra)
		if err != nil {
			return nil, err
		}
		if !reached {
			roots = append(roots, parser.RootAt(extra))
		}
	}
	if len(roots) == 1 {
		return schema, nil
	}
	schema, _, err = parser.ParseSet(roots, parser.WithResolver(resolver), parser.WithBackend(backend))
	return schema, err
}

// closureReached reports whether the document resolver resolves location to
// is one the assembly report read.
func closureReached(resolver loader.Resolver, report *parser.AssemblyReport, location string) (bool, error) {
	rc, resolved, err := resolver.Resolve("", location)
	if err != nil {
		return false, fmt.Errorf("assess: resolving schema document %s: %w", location, err)
	}
	if err := rc.Close(); err != nil {
		return false, fmt.Errorf("assess: closing schema document %s: %w", location, err)
	}
	for _, d := range report.Documents() {
		if d.Location == resolved {
			return true, nil
		}
	}
	return false, nil
}

// A Decoder decodes the instance data against schema, compiled with backend,
// into the Go type its maker chose, and returns codec.Decode's error alone.
type Decoder func(schema *xsd.Schema, backend value.Backend, data []byte, opts ...codec.Option) error

// DecoderFor returns the Decoder into T: codec.Decode[T], its value
// discarded, under bound and then the options each call passes. The census
// binds every codec.WithBranches set of T's generated package.
func DecoderFor[T any](bound ...codec.Option) Decoder {
	return func(schema *xsd.Schema, backend value.Backend, data []byte, opts ...codec.Option) error {
		_, err := codec.Decode[T](schema, backend, data, append(slices.Clone(bound), opts...)...)
		return err
	}
}

// placeholder is the T a case decodes into when the census resolved no
// generated type for its document element ([Case]).
type placeholder struct{}

// bucket is the class of one Decode result. The zero value is invalid (STYLE
// T1).
type bucket uint8

const (
	// decoded: Decode returned a value.
	decoded bucket = iota + 1
	// declined: an error errors.As retrieves a *codec.DeclineError from,
	// Decode's doc items 5 and 7.
	declined
	// undecided: an error errors.As retrieves a *codec.UndecidedError from,
	// Decode's doc item 4.
	undecided
	// rejected: an error errors.Is reports as codec.ErrTypeMismatch, Decode's
	// doc item 6, on the type codegen emitted for the document element's
	// declaration.
	rejected
	// placeholderMismatch: an error errors.Is reports as
	// codec.ErrTypeMismatch on the placeholder, counted under failed other.
	placeholderMismatch
	// failedOther: every other error.
	failedOther
)

// outcome is one case's classification: its bucket, the construct of a
// declined one, and whether Decode returned a value where the census's own
// validation did not accept the instance.
type outcome struct {
	bucket       bucket
	construct    codec.Construct
	differential bool
}

// classify returns the bucket of err, a Decoder's result, and for a decline
// its construct. own reports whether the Decoder's T is the type codegen
// emitted for the document element's declaration: an ErrTypeMismatch on any
// other T is no codec defect.
func classify(err error, own bool) (bucket, codec.Construct) {
	if err == nil {
		return decoded, codec.Construct{}
	}
	var de *codec.DeclineError
	if errors.As(err, &de) {
		return declined, de.Construct()
	}
	if errors.Is(err, codec.ErrTypeMismatch) {
		if !own {
			return placeholderMismatch, codec.Construct{}
		}
		return rejected, codec.Construct{}
	}
	var ue *codec.UndecidedError
	if errors.As(err, &ue) {
		return undecided, codec.Construct{}
	}
	return failedOther, codec.Construct{}
}

// accepts reports whether validation accepts data against schema: it is
// accepted when validate.New and xmlsrc.Validate succeed and the Result has
// no source fault and stands validate.NotProvenInvalid.
func accepts(schema *xsd.Schema, backend value.Backend, data []byte, uri string) bool {
	v, err := validate.New(schema, backend)
	if err != nil {
		return false
	}
	res, err := xmlsrc.Validate(v, bytes.NewReader(data), xmlsrc.WithURI(uri))
	if err != nil || res.Err() != nil {
		return false
	}
	return res.Standing() == validate.NotProvenInvalid
}

// assess classifies decode's result on the instance data, named uri: its
// bucket by classify, and differential when decode returned no error where
// the census's own validation, never decode's error, does not accept data.
func assess(schema *xsd.Schema, backend value.Backend, data []byte, uri string, decode Decoder, own bool) outcome {
	b, c := classify(decode(schema, backend, data, codec.WithURI(uri)), own)
	return outcome{bucket: b, construct: c, differential: b == decoded && !accepts(schema, backend, data, uri)}
}

// A Request is what [Drive] reads: the suite root every path is relative to,
// and the test groups whose cases it assesses.
type Request struct {
	// Root is the suite root, absolute.
	Root string
	// Groups are the test groups, each with the schema its cases share.
	Groups []Group
}

// A Group is one test group's schema documents, assembled by [Assemble], and
// the cases assessed against that schema.
type Group struct {
	Schema []string
	Cases  []Case
}

// A Case is one instance case: its instance document, a slash path relative
// to the suite root, and the index into Drive's decoders of the Decoder into
// the type codegen emitted for its document element's declaration, or -1 for
// the placeholder, which an ErrTypeMismatch is no codec defect for.
type Case struct {
	Instance string
	Decoder  int
}

// A Summary is what [Drive] writes: each bucket's count but declined's, which
// is the sum of Declines' counts (STYLE D3), the count of decoded cases the
// census's validation did not accept, and of failed other the ErrTypeMismatch
// count on the placeholder.
type Summary struct {
	Decoded             int
	Undecided           int
	Rejected            int
	FailedOther         int
	Differential        int
	PlaceholderMismatch int
	// Declines holds one Tally per declined construct, in the order the
	// cases first met each.
	Declines []Tally
}

// A Tally is one declined construct, by its codec.Construct's String, and
// how many cases declined it first.
type Tally struct {
	Construct string
	Count     int
}

// Drive reads one JSON [Request] from r, assesses each of its cases with the
// Decoder decoders holds at the case's index, or the placeholder's, and
// writes one JSON [Summary] to w. Within the one process it keys each decline
// on its codec.Construct, and writes the Construct's String, which codec
// makes distinct for each Construct, only into the Summary. A group that does
// not assemble, or an instance that does not read, is an error: the census
// generated each group's package from the same assembly, and read each
// instance's document element, before it wrote the Request.
func Drive(r io.Reader, w io.Writer, decoders []Decoder) error {
	var req Request
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return fmt.Errorf("assess: reading the request: %w", err)
	}
	backend := strict.New()
	var sum Summary
	// index maps a construct to its position in sum.Declines, for lookup
	// only; nothing ranges over it (STYLE D2).
	index := map[codec.Construct]int{}
	for _, g := range req.Groups {
		schema, err := Assemble(req.Root, g.Schema, backend)
		if err != nil {
			return fmt.Errorf("assess: assembling %v: %w", g.Schema, err)
		}
		for _, c := range g.Cases {
			data, err := os.ReadFile(filepath.Join(req.Root, filepath.FromSlash(c.Instance)))
			if err != nil {
				return fmt.Errorf("assess: reading instance %s: %w", c.Instance, err)
			}
			decode, own, err := decoderAt(decoders, c.Decoder)
			if err != nil {
				return fmt.Errorf("assess: case %s: %w", c.Instance, err)
			}
			tallyOutcome(&sum, index, assess(schema, backend, data, c.Instance, decode, own))
		}
	}
	if err := json.NewEncoder(w).Encode(sum); err != nil {
		return fmt.Errorf("assess: writing the summary: %w", err)
	}
	return nil
}

// decoderAt returns decoders[i] and true, or for i -1 the placeholder's
// Decoder and false.
func decoderAt(decoders []Decoder, i int) (Decoder, bool, error) {
	if i == -1 {
		return DecoderFor[placeholder](), false, nil
	}
	if i < 0 || i >= len(decoders) {
		return nil, false, fmt.Errorf("decoder index %d is outside the %d decoders", i, len(decoders))
	}
	return decoders[i], true, nil
}

// tallyOutcome adds o, one case's outcome, to sum, index mapping a construct
// to its Tally's position in sum.Declines.
func tallyOutcome(sum *Summary, index map[codec.Construct]int, o outcome) {
	if o.differential {
		sum.Differential++
	}
	switch o.bucket {
	case decoded:
		sum.Decoded++
	case declined:
		i, seen := index[o.construct]
		if !seen {
			i = len(sum.Declines)
			index[o.construct] = i
			sum.Declines = append(sum.Declines, Tally{Construct: o.construct.String()})
		}
		sum.Declines[i].Count++
	case undecided:
		sum.Undecided++
	case rejected:
		sum.Rejected++
	case placeholderMismatch:
		sum.FailedOther++
		sum.PlaceholderMismatch++
	case failedOther:
		sum.FailedOther++
	}
}
