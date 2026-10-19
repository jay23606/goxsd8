package conformance

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/kud360/goxsd8/internal/schemaloc"
	"github.com/kud360/goxsd8/internal/xmltok"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// This file gives the instance lane the schema location hints its instance
// carries, xsi:schemaLocation and xsi:noNamespaceSchemaLocation (§2.7.3,
// §4.3.2), assembled through parser.ParseSet and parser.HintAt. §4.3.2 clause 3
// lets a processor dereference hints and obliges it to dereference none. For an
// instanceTest whose test group declares no schemaTest (groupSchemaDocs yields
// none) the hints of every element, the first per namespace, and every inline
// xs:schema the instance carries locate the whole schema (#2013, #2171,
// #2180), and every shape the lane cannot read completely DECLINES rather than
// deciding against a partial schema. A document carrying neither is assessed
// against the built-in components alone where its root's xsi:type names a
// built-in type definition (builtinsSchema, #2151). For a group that declares
// one, the root's hints alone only add namespaces the group's documents leave
// out, and never decline (groupSchema, #771); an inline xs:schema is not read
// there.
//
// The reader is a second copy of cmd/goxsd8's instanceHints/hintsOf, narrowed
// to what the lane can decide, and on the no-schemaTest path it reads the hints
// below the document element, and the inline xs:schema documents, that the
// CLI's copy ignores; #755 owns the single home both should share (STYLE T4).

// caseSchema assembles the schema c is assessed against, with assembleCase's
// schema and error, its decidable false reported as the refusal that declined
// the case: the group's schema documents where discovery attached them, widened by
// the hints groupSchema adds, otherwise the schema the instance's own hints and
// inline xs:schema documents locate (#2013, #2180, §4.3.2 clauses 3-5), and
// for a case with none of these — no group schema, no hint AND no inline
// xs:schema — the built-in components where builtinsSchema finds the case
// decidable against them.
func caseSchema(backend value.Backend, c caseSpec) (*xsd.Schema, refusal, error) {
	if c.schemaDoc != "" {
		return groupSchema(backend, c)
	}
	root, hints, inline, why := instanceHints(c.doc)
	if why != "" {
		return nil, why, nil
	}
	if len(hints) == 0 && len(inline) == 0 {
		return builtinsSchema(backend, root)
	}
	return assembleHints(backend, c.doc, expandedName(root.Name), hints, inline)
}

// groupSchema is what caseSchema answers for a case whose group declares a
// schemaTest: the schema assembleCase assembles from the group's documents
// (refuseGroupAssembly where it declines), widened by the hints of c's instance
// root for namespaces that assembly leaves uncovered (#771, §4.3.2 clauses 3-5).
//
// A hint is added only where its namespace — "" for
// xsi:noNamespaceSchemaLocation — is the targetNamespace of no document the
// group's closure consumed (closureNamespaces). One for a covered namespace is
// left out: a stale hint naming a document that redeclares the group's
// components would make the union a schema error (sch-props-correct clause 2),
// and §4.2.6.2's Note lets a processor keep the components it already has for
// a namespace. A chameleon-included document takes its includer's namespace,
// which the includer's own targetNamespace already covers.
//
// A hint that cannot be followed, or a widened assembly that declines or errs,
// never declines the case and never decides it invalid; a followed hint's
// components are assessed like any other. §4.3.2 clause 3 makes a hint that
// cannot be followed no error, so every failure falls back to the group's own
// schema — where the group's assembly errs (its perr is what the caller reads,
// as without hints), the instance has a shape instanceHints refuses or carries
// an inline xs:schema, no root hint names an uncovered namespace (a hint below
// the root is never read on this path), or the widened assembly declines or
// errs (src-import clause 3.1, a hinted document whose targetNamespace is not
// the hint's). A hint resolving to no document is skipped by parser.ParseSet
// itself (parser.AssemblyReport.UnfollowedRoots), and the widened assembly is
// used without it.
func groupSchema(backend value.Backend, c caseSpec) (*xsd.Schema, refusal, error) {
	schema, report, decidable, perr := assembleCase(backend, c.schemaDoc, c.schemaExtraDocs)
	if !decidable {
		return nil, refuseGroupAssembly, nil
	}
	if perr != nil {
		return schema, "", perr
	}
	hints := uncoveredHints(c, report)
	if len(hints) == 0 {
		return schema, "", nil
	}
	hinted, _, decidable, perr := assembleCaseWith(backend, c.schemaDoc, c.schemaExtraDocs, hints)
	if decidable && perr == nil {
		return hinted, "", nil
	}
	return schema, "", nil
}

// uncoveredHints returns, as parser.HintAt roots, the hints of c's instance root
// whose namespace no document of report covers (closureNamespaces), and none
// where instanceHints refuses the instance or the instance carries an inline
// xs:schema, which is never read on this path. A hint below the root is not
// read here either (#771's GAP(conformance) in instance.go). Each location is
// resolved against the instance's path relative to c.schemaDoc's directory, the
// one assembleCaseWith's resolver serves, so a location climbing above it
// resolves to no document.
func uncoveredHints(c caseSpec, report *parser.AssemblyReport) []parser.Root {
	root, _, inline, why := instanceHints(c.doc)
	if why != "" || len(inline) > 0 {
		return nil
	}
	rel, err := filepath.Rel(filepath.Dir(c.schemaDoc), c.doc)
	if err != nil {
		return nil
	}
	// hintsOf's one refusal, an odd member count, depends on root alone, and
	// instanceHints has just read root's hints without it, so this read names
	// none.
	hints, _ := hintsOf(root, filepath.ToSlash(rel), closureNamespaces(report))
	return hints
}

// closureNamespaces is the set of targetNamespace values of the documents
// report lists, an absent attribute read as "": the namespaces the assembly
// already holds components for. A chameleon-included document has no
// targetNamespace and so adds "" too, which can only withhold an
// xsi:noNamespaceSchemaLocation hint and leave the group's own schema.
func closureNamespaces(report *parser.AssemblyReport) map[string]bool {
	covered := map[string]bool{}
	for _, d := range report.Documents() {
		tns, _ := d.Doc.Root().Attr("targetNamespace")
		covered[tns] = true
	}
	return covered
}

// builtinsSchemaDoc is the package-relative path of a schema document with no
// children and no targetNamespace: the schema assembled from it holds the
// built-in components alone, which every schema contains — the built-in type
// definitions (§3.4.7, §3.16.7), all in the XSD namespace, and the four xsi:
// attribute declarations (§3.2.7) — and no element declaration.
const builtinsSchemaDoc = "testdata/builtins/builtins.xsd"

// builtinsSchema is what caseSchema answers for a case with no group schema
// whose instance, its document element root, carries no hint and no inline
// xs:schema on any element: the schema assembled from builtinsSchemaDoc
// through assembleCase, handed out only where the spec decides the case
// against the built-in components, and otherwise the refusal naming the arm
// that leaves it undecided — the caller DECLINES (#2151). With no hint to
// follow, and §4.3.2 clause 3 obliging a processor to follow none, the
// built-in components every schema holds are the whole schema. The arms:
//
//   - root's xsi:type, resolved against the namespace declarations root
//     carries (resolveQName) — its whole [in-scope namespaces] but xml:, root
//     being the document element — names a type definition of that schema,
//     so a built-in one in the XSD namespace. With no ·governing element
//     declaration·, it is root's ·governing type definition·
//     (key-governing-type-elem clause 8, key-itd), root is ·strictly assessed·
//     against it (cvc-assess-elt clause 1), and cvc-type decides root. The
//     schema is handed out.
//   - root's xsi:type resolves to no QName, or to one naming no built-in type
//     definition (refuseNoHintXsiTypeUnresolved). key-itd clause 3 fails, so
//     root has no ·governing type definition·, is ·laxly assessed·
//     (cvc-assess-elt clause 3), and its [validity] is notKnown (e-validity
//     clause 2). cvc-attribute clause 5 charges the xsi:type attribute, not
//     root.
//   - root carries no xsi:type, and an attribute in the xsi namespace whose
//     local name is none of §3.2.7's four (unknownXsi, refuseNoHintUnknownXsi).
//     Root is ·laxly assessed· against xs:anyType, whose lax attribute wildcard
//     admits the attribute, and no declaration exists to assess it (key-sva
//     (§3.3.4.6) clause 2.2): no rule charges it, so a suite expectation of
//     invalid is a suite expectation (PRINCIPLES 25), not a verdict.
//   - root carries neither (refuseNoHint): a ·laxly assessed· root, its
//     [validity] notKnown (#2013).
//
// Handed the schema, the walk would decide either undecided arm on its
// cvc-assess-elt charge for a root with no top-level declaration and no
// resolving xsi:type, a ·strict assessment· the spec does not make there.
func builtinsSchema(backend value.Backend, root xml.StartElement) (*xsd.Schema, refusal, error) {
	i := slices.IndexFunc(root.Attr, func(a xml.Attr) bool { return a.Name == xsiType })
	if i < 0 {
		if slices.ContainsFunc(root.Attr, unknownXsi) {
			return nil, refuseNoHintUnknownXsi, nil
		}
		return nil, refuseNoHint, nil
	}
	name, ok := resolveQName(root.Attr, root.Attr[i].Value)
	if !ok {
		return nil, refuseNoHintXsiTypeUnresolved, nil
	}
	schema, _, decidable, perr := assembleCase(backend, builtinsSchemaDoc, nil)
	if !decidable {
		return nil, refuseBuiltinsAssembly, nil
	}
	if perr != nil {
		return schema, "", perr
	}
	if _, ok := schema.Type(name); !ok {
		return nil, refuseNoHintXsiTypeUnresolved, nil
	}
	return schema, "", nil
}

// unknownXsi reports whether a is an attribute in the xsi namespace whose
// local name is none of the four §3.2.7 declares, compared case-sensitively,
// so xsi:Type is one.
func unknownXsi(a xml.Attr) bool {
	return a.Name.Space == xsd.XMLSchemaInstanceNS && notExcepted(a)
}

// instanceHints reads the schema location hints of the instance at doc, in
// document order, the document element's first, together with that element's
// start tag and the instance's inline xs:schema elements, in document order.
// Each location is resolved against the instance's base URI through
// internal/schemaloc (§4.3.2 clause 4). §4.3.2 clause 5 makes a hint on any
// element global to the assessment, so every element's hints are read before
// assessment begins, a whole-document reading §4.1 lets stand for the lazy one
// (#2171). The first element to hint a namespace — "" for
// xsi:noNamespaceSchemaLocation — supplies it, and a later element's hint for
// that namespace is left out (hintsOf), so it never reaches parser.ParseSet:
// clause 3 makes following any hint optional, and sch-props-correct clause 2
// would make a second document redeclaring the first one's components a schema
// error. The namespace is the hint's, never that of the element carrying it.
//
// An inline xs:schema element, the document element included, is read as a
// schema document of the case's schema (#2180, inlineSchema). §4.3.2's
// preamble leaves the means of locating schema documents to the processor, a
// document so located is an ordinary schema document (§5.1), and clause 5
// makes its effect as global as a hint's, so the order-independent reading
// §4.1 and clause 5 require reads every one before assessment begins. It takes
// no part in the first-wins reading of hints: it is no hint clause 3 lets the
// processor leave unfollowed but a document the instance itself carries, so
// one for a namespace a hint supplied is read beside that hint's document, and
// a component both declare is the schema error sch-props-correct clause 2
// makes it. The element itself stays in the instance, where no declaration
// governs it and it is ·laxly assessed· (cvc-assess-elt clause 3).
//
// It names a refusal — the caller DECLINES — when the hints read may not be the
// whole of what clause 5 makes global, or may not resolve where clause 4 says:
//
//   - the document will not resolve or decode, or carries a DOCTYPE whose DTD
//     could default a hint onto any element: one with an external subset, an
//     <!ATTLIST or a parameter-entity reference (rootStart);
//   - the root carries xml:base, or an element carrying a hint, or an inline
//     xs:schema, has xml:base on itself or an ancestor: it moves the base URI
//     clause 4 resolves that element's locations against;
//   - an xsi:schemaLocation value has an odd member count, so one location
//     pairs with no namespace;
//   - an inline xs:schema sits inside another, in its annotation say, where
//     whether it is a schema document of the assessment or content of the
//     outer one's is a reading the lane does not make (refuseInlineSchema).
//
// A document carrying no hint and no inline xs:schema names no refusal and
// returns neither; caseSchema hands that shape to builtinsSchema.
//
// The reading decides four suite-invalid cases of the MS-Additional ordering
// family (test group addB156-addB166) VALID, each an expected divergence
// (PRINCIPLES 25): addB159 (test93490_5.xml), addB161 (test93490_7.xml),
// addB162 (test93490_8.xml) and addB164 (test93490_10.xml). Each hints, below
// the document element, a namespace an earlier hint already supplied, naming a
// different document, and the family's description charges that ordering
// ("schema location or inline schema seen after item from schema target
// namespace seen"). No rule of the Recommendation does: §4.3.2 clause 5 gives
// a hint on any element global effect and names no error for a late one, and
// clause 3 makes following it optional, so the later document is left out and
// the instance is valid against the earlier one's components. addB156 and
// addB158 have the same shape, and the suite expects both valid under 1.1.
// addB166 (test93490_12.xml) is a fifth, suite status queried: a late hint and
// a late inline xs:schema for a namespace the root's hint supplied, the inline
// one declaring nothing, so no rule charges it either.
func instanceHints(doc string) (root xml.StartElement, hints []parser.Root, inline []inlineSchema, why refusal) {
	rc, _, err := loader.Dir(filepath.Dir(doc)).Resolve("", filepath.Base(doc))
	if err != nil {
		return xml.StartElement{}, nil, nil, refuseHintsUnresolved
	}
	defer func() { _ = rc.Close() }() // read-only handle: close error cannot affect the verdict
	r := hintReader{base: filepath.Base(doc), supplied: map[string]bool{}, seen: &window{line: 1, col: 1}}
	r.dec = recordingDecoder(rc, r.seen)
	start, why := rootStart(r.dec)
	if why != "" {
		return xml.StartElement{}, nil, nil, why
	}
	if slices.ContainsFunc(start.Attr, isXMLBase) {
		return xml.StartElement{}, nil, nil, refuseXMLBase
	}
	if why := r.read(start); why != "" {
		return xml.StartElement{}, nil, nil, why
	}
	return start, r.hints, r.inline, ""
}

// hintsOf reads el's hints, resolving each location against base, the
// instance's base URI as the resolver serving it spells it: xsi:schemaLocation
// pairs a namespace with a location, and xsi:noNamespaceSchemaLocation names a
// location whose document has no target namespace, parser.HintAt's "". It
// leaves out every hint whose namespace covered holds, then adds the namespace
// of every hint it returns to covered, so a later element's hint for that
// namespace is left out too (first-wins); el's own hints for one namespace are
// all returned. It refuses an odd xsi:schemaLocation member count
// (refuseOddLocation).
func hintsOf(el xml.StartElement, base string, covered map[string]bool) (hints []parser.Root, why refusal) {
	var namespaces []string
	for _, a := range el.Attr {
		if !isLocationHint(a) {
			continue
		}
		fields := strings.Fields(a.Value)
		if a.Name.Local == "noNamespaceSchemaLocation" {
			if covered[""] {
				continue
			}
			for _, location := range fields {
				hints = append(hints, parser.HintAt("", schemaloc.Resolve(base, location)))
				namespaces = append(namespaces, "")
			}
			continue
		}
		if len(fields)%2 != 0 {
			return nil, refuseOddLocation
		}
		for i := 0; i < len(fields); i += 2 {
			if covered[fields[i]] {
				continue
			}
			hints = append(hints, parser.HintAt(fields[i], schemaloc.Resolve(base, fields[i+1])))
			namespaces = append(namespaces, fields[i])
		}
	}
	for _, ns := range namespaces {
		covered[ns] = true
	}
	return hints, ""
}

// hintReader is instanceHints' one pass over the instance: dec, a
// recordingDecoder writing to seen, base, the instance's base URI as its
// resolver spells it, supplied, the namespaces hints have supplied so far
// (hintsOf), and what the pass has read — hints, inline, and pending, the
// inline xs:schema whose end tag it has yet to reach.
type hintReader struct {
	dec      *xmltok.Decoder
	seen     *window
	base     string
	supplied map[string]bool
	open     []scope
	hints    []parser.Root
	inline   []inlineSchema
	pending  *pendingSchema
}

// scope is what hintReader keeps of one open element: whether an xml:base is
// in scope there, and the namespace declarations the element carries itself.
type scope struct {
	based bool
	decls []xml.Attr
}

// pendingSchema is an inline xs:schema hintReader has read the start tag of:
// depth, the number of elements open outside it, from and tag, the offsets of
// its '<' and of the byte after its start tag, and bindings, the namespace
// declarations it inherits (inheritedBindings).
type pendingSchema struct {
	depth    int
	from     int64
	tag      int64
	bindings string
}

// inlineSchema is an inline xs:schema of the instance as a schema document:
// location, the one assembleHints serves it under (inlineLocation), and text,
// the document.
type inlineSchema struct {
	location string
	text     string
}

// read reads, from the document element, whose start tag is root and which
// dec is positioned just past, to the end of the document, every element's
// hints and every inline xs:schema. It names the refusal where dec does not
// decode (refuseDecode), and passes on element's.
func (r *hintReader) read(root xml.StartElement) refusal {
	if why := r.element(root); why != "" {
		return why
	}
	for {
		if r.pending == nil {
			r.seen.advance(r.dec.InputOffset())
		}
		tok, err := r.dec.Token()
		if errors.Is(err, io.EOF) {
			return ""
		}
		if err != nil {
			return refuseDecode
		}
		if _, ok := tok.(xml.EndElement); ok {
			r.open = r.open[:len(r.open)-1]
			if r.pending != nil && len(r.open) == r.pending.depth {
				r.closeSchema()
			}
			continue
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if why := r.element(start); why != "" {
			return why
		}
	}
}

// element reads the element whose start tag, start, dec has just returned. It
// opens the element's scope, reads its hints (hintsOf) and, where it is an
// inline xs:schema, begins that document (pendingSchema). It names the refusal
// where an element carrying a hint, or an inline xs:schema, has xml:base on
// itself or an ancestor (refuseXMLBase), or an inline xs:schema sits inside
// another (refuseInlineSchema), and passes on hintsOf's.
func (r *hintReader) element(start xml.StartElement) refusal {
	based := slices.ContainsFunc(start.Attr, isXMLBase)
	if len(r.open) > 0 {
		based = based || r.open[len(r.open)-1].based
	}
	if isInlineSchema(start.Name) {
		if r.pending != nil {
			return refuseInlineSchema
		}
		if based {
			return refuseXMLBase
		}
		tag := r.dec.InputOffset()
		r.pending = &pendingSchema{
			depth:    len(r.open),
			from:     r.seen.lastOpen(tag),
			tag:      tag,
			bindings: inheritedBindings(r.open, start),
		}
	}
	r.open = append(r.open, scope{based: based, decls: slices.DeleteFunc(slices.Clone(start.Attr), notNamespaceDeclaration)})
	if !slices.ContainsFunc(start.Attr, isLocationHint) {
		return ""
	}
	if based {
		return refuseXMLBase
	}
	got, why := hintsOf(start, r.base, r.supplied)
	if why != "" {
		return why
	}
	r.hints = append(r.hints, got...)
	return ""
}

// closeSchema ends the pending inline xs:schema at the decoder's offset, just
// past its end tag, and adds it to inline as a schema document whose every
// line is the instance's, and every column but those after its start tag on
// that tag's last line: the text before its '<' is blanked to newlines and
// spaces, and the bindings it inherits are added at the close of its start
// tag, after every attribute it carries itself. Cutting the element
// out alone would leave a prefix its ancestors bind unbound, so a name or a
// QName value using it would not read as it reads in the instance, whose
// [in-scope namespaces] the inline document's elements keep.
func (r *hintReader) closeSchema() {
	p := r.pending
	r.pending = nil
	tag := r.seen.bytes(p.from, p.tag)
	closing := ">"
	if bytes.HasSuffix(tag, []byte("/>")) {
		closing = "/>"
	}
	line, col := r.seen.position(p.from)
	var b strings.Builder
	b.WriteString(strings.Repeat("\n", line-1))
	b.WriteString(strings.Repeat(" ", col-1))
	b.Write(tag[:len(tag)-len(closing)])
	b.WriteString(p.bindings)
	b.WriteString(closing)
	b.Write(r.seen.bytes(p.tag, r.dec.InputOffset()))
	r.inline = append(r.inline, inlineSchema{location: inlineLocation(r.base, len(r.inline)+1), text: b.String()})
}

// inheritedBindings is the namespace declarations, as attribute text each led
// by a space, that bind in el's scope a prefix, or the default namespace,
// which open, the scopes of el's ancestors outermost first, declare and el
// itself does not: el's [in-scope namespaces] but its own and xml:. A
// declaration binding to "" undeclares and is left out. They are in the order
// of the outermost declaration of each prefix.
func inheritedBindings(open []scope, el xml.StartElement) string {
	var prefixes []string
	bound := map[string]string{}
	for _, s := range open {
		for _, a := range s.decls {
			p := declaredPrefix(a)
			if _, ok := bound[p]; !ok {
				prefixes = append(prefixes, p)
			}
			bound[p] = a.Value
		}
	}
	var b strings.Builder
	for _, p := range prefixes {
		redeclared := slices.ContainsFunc(el.Attr, func(a xml.Attr) bool {
			return isNamespaceDeclaration(a) && declaredPrefix(a) == p
		})
		if bound[p] == "" || redeclared {
			continue
		}
		b.WriteString(" xmlns")
		if p != "" {
			b.WriteString(":" + p)
		}
		b.WriteString(`="` + attrEscaper.Replace(bound[p]) + `"`)
	}
	return b.String()
}

// attrEscaper escapes a namespace name for a double-quoted attribute value:
// the markup characters, and the white space attribute-value normalization
// would otherwise turn to spaces (XML 1.0 §3.3.3).
var attrEscaper = strings.NewReplacer(`&`, "&amp;", `<`, "&lt;", `"`, "&quot;", "\t", "&#9;", "\n", "&#10;", "\r", "&#13;")

// declaredPrefix is the prefix the namespace declaration a binds, "" for the
// default namespace.
func declaredPrefix(a xml.Attr) string {
	if a.Name.Space == "xmlns" {
		return a.Name.Local
	}
	return ""
}

// notNamespaceDeclaration is isNamespaceDeclaration's negation.
func notNamespaceDeclaration(a xml.Attr) bool { return !isNamespaceDeclaration(a) }

// inlineFragment leads the fragment identifier of every inline location.
const inlineFragment = "#inline-"

// inlineLocation is the location the n-th inline xs:schema of the instance
// whose base URI is base is served under, n counting from 1 in document order:
// base with a fragment identifier, so each has its own load-once identity, and
// a location it names resolves against base's directory exactly as one the
// instance names (§4.3.2 clause 4, schemaloc.Resolve), its base URI being the
// instance's. An error in it cites that location, and the instance's own line
// and column (closeSchema).
func inlineLocation(base string, n int) string {
	return base + inlineFragment + strconv.Itoa(n)
}

// window is an io.Writer holding the bytes written to it from offset base on,
// and the 1-based line and column, in bytes, of base: hintReader's record of
// the decoded instance, advanced past every token it no longer needs, so it
// holds at most the decoder's read-ahead and the inline xs:schema being read.
// Lines are counted at line feeds, as parser/xmltree counts them.
type window struct {
	base      int64
	buf       []byte
	line, col int
}

// Write implements io.Writer. It never fails.
func (w *window) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

// advance drops the bytes before offset off.
func (w *window) advance(off int64) {
	w.line, w.col = w.position(off)
	w.buf = w.buf[off-w.base:]
	w.base = off
}

// position is the line and column of offset off.
func (w *window) position(off int64) (line, col int) {
	line, col = w.line, w.col
	for _, c := range w.buf[:off-w.base] {
		col++
		if c == '\n' {
			line, col = line+1, 1
		}
	}
	return line, col
}

// bytes is the bytes from offset from up to offset to.
func (w *window) bytes(from, to int64) []byte {
	return w.buf[from-w.base : to-w.base]
}

// lastOpen is the offset of the last '<' before offset end: for an end just
// past a start tag, that tag's own, since no attribute value holds a literal
// '<' (XML 1.0 §3.1 [10] AttValue). The window holds it: it was advanced no
// further than the end of the token before.
func (w *window) lastOpen(end int64) int64 {
	return w.base + int64(bytes.LastIndexByte(w.bytes(w.base, end), '<'))
}

// isXMLBase reports whether a is xml:base.
func isXMLBase(a xml.Attr) bool {
	return a.Name.Space == xmlPrefixNS && a.Name.Local == "base"
}

// isInlineSchema reports whether an element named n is an xs:schema.
func isInlineSchema(n xml.Name) bool {
	return n.Space == xsd.XMLSchemaNS && n.Local == "schema"
}

// assembleHints assembles the schema the hints and the inline xs:schema
// documents locate, the instance lane's counterpart of assembleCase for a
// case with no group schema, and names the refusal where the outcome may not
// be read on assembleCase's terms — the caller DECLINES — which is when:
//
//   - a hint resolved to no document (parser.AssemblyReport.UnfollowedRoots,
//     legal to skip under §4.2.6.2), so the schema is short of a document the
//     instance named (refuseHintUnfollowed);
//   - assemblyDeclined, assembleCase's own gate, refuses the outcome: the
//     closure leaves the decidable subset (closureDecidable) and the assembly
//     did not fail with a grammarRejection, or the rejection is one an
//     unfollowed directive could have fabricated (fabricatedRejection)
//     (refuseHintAssembly);
//   - the assembly succeeded and declares no top-level element for root
//     (refuseHintUndeclared). A root no declaration governs is laxly
//     assessed (cvc-assess-elt clause 3), and §3.3.5.1 gives a root not
//     strictly assessed [validity] notKnown, not invalid; no ruling
//     licenses deciding such a case "not valid".
//
// It declines too where an inline xs:schema does not read as a schema document
// (rootReadable, refuseInlineUnread): parser.ParseSet answers that with a plain
// error, which no verdict may rest on, as for assembleCase's own roots.
//
// The resolver is assembleCase's, pinnedResolver over a loader.Dir rooted at
// the instance's own directory, which every hint location is relative to
// (instanceHints), so a location climbing above it is refused as unresolved,
// behind a loader.Map serving each inline xs:schema at its inlineLocation. The
// hints enter parser.ParseSet in document order, then each inline xs:schema as
// a parser.RootAt root, in document order: either way the assembly is one
// schema (§4.3.2 clause 5).
func assembleHints(backend value.Backend, doc string, root xsd.QName, hints []parser.Root, inline []inlineSchema) (*xsd.Schema, refusal, error) {
	texts := make(map[string]string, len(inline))
	roots := slices.Clone(hints)
	for _, s := range inline {
		texts[s.location] = s.text
		roots = append(roots, parser.RootAt(s.location))
	}
	resolver := loader.Chain(loader.Map(texts), pinnedResolver{dir: loader.Dir(filepath.Dir(doc))})
	for _, s := range inline {
		if _, ok := rootReadable(resolver, s.location); !ok {
			return nil, refuseInlineUnread, nil
		}
	}
	schema, report, perr := parser.ParseSet(roots, parser.WithResolver(resolver), parser.WithBackend(backend))
	if len(report.UnfollowedRoots()) > 0 {
		return nil, refuseHintUnfollowed, nil
	}
	if assemblyDeclined(report, perr) {
		return nil, refuseHintAssembly, nil
	}
	if perr != nil {
		return schema, "", perr
	}
	if _, ok := schema.Element(root); !ok {
		return nil, refuseHintUndeclared, nil
	}
	return schema, "", nil
}
