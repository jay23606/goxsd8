package xmltree

import (
	"encoding/xml"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/kud360/goxsd8/internal/xmlchar"
	"github.com/kud360/goxsd8/internal/xmldecl"
	"github.com/kud360/goxsd8/internal/xmlenc"
	"github.com/kud360/goxsd8/internal/xmltok"
	"github.com/kud360/goxsd8/xsderr"
)

// Reader is a streaming, namespace-scoped XML token reader. It wraps an
// io.Reader and yields resolved Nodes one at a time from Token, holding only
// bounded state — a namespace-scope chain, an open-element stack, and an
// index of newline offsets — never the whole document (STYLE P4).
//
// A Reader is single-use and not safe for concurrent use.
type Reader struct {
	uri string
	dec *xmltok.Decoder
	pos *posReader
	// decl is the version-label rewrite the decoder reads through; it keeps
	// the source's own label, which prefix undeclaration depends on.
	decl *xmldecl.Reader

	// bom is what the document's byte-order mark said its encoding is — the
	// evidence an encoding declaration must agree with (XML 1.0 §4.3.3).
	bom xmlenc.Mark

	// stack holds one frame per currently-open element, so end tags match
	// their starts and nested elements resolve against the right scope.
	stack []frame
	// ended records that the document element's end tag has been read, after
	// which only Misc may appear: no start tag (see classify), no character
	// data but S (see outsideRootFault), and no DOCTYPE (see declareEntities).
	ended bool
	// doctype records that the DOCTYPE has been read, after which no other may
	// appear (see declareEntities).
	doctype bool
	// eof latches io.EOF so repeated Token calls keep returning it.
	eof bool

	// entities maps each general entity name the DOCTYPE's internal subset
	// declares to that name's binding declaration and whether a declaration
	// of it stands outside every parameter entity (see boundEntity). It is a
	// lookup index only, never iterated. dec.Entity names every one of them
	// to the decoder, which refuses a reference to any other name (see
	// included), so that Reader.reference decides a reference to an external
	// or unparsed one: it charges WFC Entity Declared in a standalone="yes"
	// document, then WFC Parsed Entity, in content or in an attribute value,
	// and, in an attribute value, No External Entity References, and refuses
	// any other.
	entities map[string]boundEntity
	// tokenized maps each (element type, attribute) name pair an <!ATTLIST>
	// of the internal subset defines, as the declaration spells them, to
	// whether the pair's FIRST definition, which binds (XML 1.0 §3.3), gives
	// an AttType other than CDATA: an attribute it maps to true has its
	// normalized value trimmed and collapsed (see expandAttrs). It is a lookup
	// index only, never iterated.
	tokenized map[attName]bool
	// spent counts the bytes of replacement text included so far, against
	// maxGEExpansion.
	spent int
	// pending holds the nodes one reference's inclusion produced beyond the
	// first, which Token returns before reading on.
	pending []Node
	// declsUnread is the inverse of [all declarations processed]: the DOCTYPE
	// named an external subset, or its internal subset referenced a parameter
	// entity that was not read (see doctypeEntities). Inverted so that the
	// zero value — no DOCTYPE at all — is the right answer.
	declsUnread bool
	// standalone records the XML declaration's standalone="yes" (XML 1.0
	// §2.9), which the DOCTYPE after it is read under. Only the declaration
	// at the document's first character sets it: a processing instruction
	// targeting "xml" anywhere else is a fault (see xmlDeclFault).
	standalone bool
}

// boundEntity is what the reader knows of one general entity name. binding is
// the name's FIRST declaration, which binds (XML 1.0 §4.2): a later NDATA
// declaration of a name already declared parsed declares no unparsed entity,
// and a later literal gives an internal entity no second replacement text;
// binding.inPE is where that declaration stands, and so whether a reference
// its replacement text holds occurs within a parameter entity (see
// Reader.withinPE). onlyInPE reports that every declaration of the name read
// so far stands in a parameter entity's replacement text: WFC Entity Declared
// counts every declaration outside every parameter entity, not only the
// binding one (see Reader.reference).
type boundEntity struct {
	binding  entityDecl
	onlyInPE bool
}

// attName is an attribute name and the element type name it is defined on,
// each as raw source spells it, prefix included: the key an <!ATTLIST>
// definition and a start tag's attribute meet on, before any prefix resolves.
type attName struct {
	elem, name string
}

// frame is one open element: its resolved name (to match the end tag), the
// scope in force for its content, and the location of its start tag (so an
// unclosed-element error points at the tag left open, not at end-of-stream).
type frame struct {
	name  Name
	scope *scope
	loc   xsderr.Loc
}

// NewReader returns a Reader over r. uri names the document for locations
// (xsderr.Loc.URI); it is not opened or resolved here — it is only threaded
// into every Loc the reader emits.
//
// A document whose XML declaration specifies a 1.x version number other than
// 1.0 is read as a 1.0 document (XML 1.0 §2.8 Note), through
// xmldecl.As10's same-length rewrite of that number, so locations are
// unchanged by it. The label still decides one constraint: only a document
// labelled 1.1 may undeclare a prefix (see checkUndeclarations).
//
// A leading byte-order mark is honoured per XML 1.0 §4.3.3: a UTF-16 document
// is decoded to UTF-8 before the XML decoder sees it, and a UTF-8 mark is
// dropped as the encoding signature it is (internal/xmlenc). Locations are
// therefore offsets into the decoded UTF-8 stream, not into the source bytes.
func NewReader(uri string, r io.Reader) *Reader {
	body, bom := xmlenc.Decode(r)
	decl := xmldecl.As10(body)
	pos := &posReader{r: decl}
	dec := xmltok.NewDecoder(pos)
	dec.CharsetReader = bom.CharsetReader
	return &Reader{
		uri:  uri,
		dec:  dec,
		pos:  pos,
		decl: decl,
		bom:  bom,
	}
}

// Token advances to the next element or character-data node and returns it.
// It returns io.EOF at the end of a well-formed document. Comments, processing
// instructions, and the DOCTYPE directive are skipped, once checked for
// ill-formed UTF-8 and for characters outside [2] Char (checkChars) and then
// for their place: a processing instruction targeting "xml" in any case
// anywhere but as the XML declaration at the document's first character (see
// xmlDeclFault), a directive inside the document element, and a DOCTYPE after
// it or after another are errors. The DOCTYPE's entity declarations are read on
// the way past (see HasUnparsedEntity), and a reference to an internal entity
// one declares is replaced by the nodes its replacement text parses to (see
// included). Malformed input, unbound namespace prefixes, and mismatched or
// unclosed tags are returned as errors carrying an xsderr.Loc — never as a panic
// (see the fuzz target).
func (r *Reader) Token() (Node, error) {
	if len(r.pending) > 0 {
		node := r.pending[0]
		r.pending = r.pending[1:]
		return node, nil
	}
	if r.eof {
		return nil, io.EOF
	}
	for {
		// InputOffset before RawToken is the offset of the token's first
		// byte; RawToken then advances the decoder past it.
		off := r.dec.InputOffset()
		r.pos.release(off)
		tok, err := r.dec.RawToken()
		if err != nil {
			return r.handleReadErr(err)
		}
		node, emit, err := r.classify(tok, off)
		if err != nil {
			return nil, err
		}
		if emit {
			return node, nil
		}
	}
}

// handleReadErr maps a RawToken error to the reader's contract: io.EOF ends a
// well-formed document (unclosed elements are an error instead); any other
// error is a malformed-XML failure wrapped with the current location.
func (r *Reader) handleReadErr(err error) (Node, error) {
	if errors.Is(err, io.EOF) {
		r.eof = true
		if len(r.stack) > 0 {
			open := r.stack[len(r.stack)-1]
			return nil, xsderr.New(xsderr.RuleXMLWellFormed, open.loc, "unexpected end of document: element %s left unclosed", qname(open.name))
		}
		return nil, io.EOF
	}
	return nil, xsderr.Wrap(xsderr.RuleXMLWellFormed, r.locAt(r.dec.InputOffset()), err)
}

// classify resolves one raw token. It returns (node, true, nil) to emit a
// node, (nil, false, nil) to skip (comment/PI/directive/duplicate
// whitespace), or an error.
func (r *Reader) classify(tok xml.Token, off int64) (Node, bool, error) {
	loc := r.locAt(off)
	switch t := tok.(type) {
	case xml.StartElement:
		if r.ended {
			return nil, false, xsderr.New(xsderr.RuleXMLWellFormed, loc, "element <%s> after the document element: only comments, processing instructions and white space may follow it (XML 1.0 [1] document, [27] Misc)", rawName(t.Name))
		}
		attrs, err := r.expandAttrs(t, r.source(off), true, func(i int) int64 { return off + int64(i) }, loc, nil)
		if err != nil {
			return nil, false, err
		}
		node, err := r.startElement(t.Name, attrs, loc)
		if err != nil {
			return nil, false, err
		}
		return node, true, nil
	case xml.EndElement:
		node, err := r.endElement(t, loc)
		if err != nil {
			return nil, false, err
		}
		return node, true, nil
	case xml.CharData:
		if r.dec.Entity != nil {
			if raw := r.source(off); refersToEntity(raw) {
				return r.included(raw, off, loc)
			}
		}
		if len(r.stack) == 0 {
			if err := r.outsideRootFault(r.source(off), loc); err != nil {
				return nil, false, err
			}
		}
		return &CharData{data: string(t), offset: off, loc: loc}, true, nil
	case xml.ProcInst:
		if err := r.checkChars(r.source(off), off); err != nil {
			return nil, false, err
		}
		if !strings.EqualFold(t.Target, "xml") {
			return nil, false, nil
		}
		if err := xmlDeclFault(t.Target, off, loc); err != nil {
			return nil, false, err
		}
		r.standalone = pseudoAttr(string(t.Inst), "standalone") == "yes"
		return nil, false, r.checkDeclaration(t, loc)
	case xml.Directive:
		raw := r.source(off)
		if err := r.checkChars(raw, off); err != nil {
			return nil, false, err
		}
		if len(r.stack) > 0 {
			return nil, false, xsderr.New(xsderr.RuleXMLWellFormed, loc, "directive %q inside element %s, where XML 1.0 [43] content admits no directive", directiveName(raw), qname(r.stack[len(r.stack)-1].name))
		}
		return nil, false, r.declareEntities(raw, loc)
	default:
		// xml.Comment: not part of the element/character-data stream the
		// parser consumes.
		return nil, false, r.checkChars(r.source(off), off)
	}
}

// xmlDeclFault charges a processing instruction whose target, target, is
// "xml" in some case, located at loc and offset off, as RuleXMLWellFormed
// unless it is the XML declaration: [17] PITarget excludes the name in every
// case, and [23] XMLDecl, which spells it in lower case, stands only first in
// [22] prolog, at the document entity's first character. That is offset 0 of
// the decoded stream, a byte-order mark having been dropped as the encoding
// signature it is (XML 1.0 §4.3.3), so white space, a comment or another
// declaration before it makes it no XMLDecl.
func xmlDeclFault(target string, off int64, loc xsderr.Loc) error {
	if target != "xml" {
		return xsderr.New(xsderr.RuleXMLWellFormed, loc, "processing instruction target %q is \"xml\" in another case, which XML 1.0 [17] PITarget excludes", target)
	}
	if off != 0 {
		return xsderr.New(xsderr.RuleXMLWellFormed, loc, "processing instruction target \"xml\" after the document's first character, which XML 1.0 [17] PITarget excludes everywhere but in the [23] XMLDecl that opens [22] prolog")
	}
	return nil
}

// directiveName is the keyword of raw, a directive's source "<!" through '>',
// as doctypeEntities quotes it: "<!" and the excerpt of what follows.
func directiveName(raw string) string {
	body, _ := strings.CutPrefix(raw, "<!")
	body, _ = strings.CutSuffix(body, ">")
	return "<!" + excerpt(body)
}

// checkChars checks raw, the source of a comment, processing instruction or
// directive whose first byte is at offset off, and returns a RuleXMLWellFormed
// fault located at the first offending sequence's first byte. The decoder
// checks character data, attribute values and names itself and the bodies of
// these three tokens never. It charges two faults, each with its own message:
//
//   - An ill-formed UTF-8 code unit sequence, which XML 1.0 §4.3.3 makes a
//     fatal error in an entity encoded in UTF-8. The test is UTF-8 validity,
//     not a byte value: a byte decodes as utf8.RuneError of width 1 exactly
//     where utf8.ValidString fails. It is a test of its own because U+FFFD
//     is itself a Char: the Char test alone would pass the byte.
//   - A well-formed sequence encoding no [2] Char. A parsed entity is a
//     sequence of characters (§2.2), [15] Comment and [16] PI are built from
//     Char, and the internal subset, its entity value literals included, is
//     text of the document entity, so the document is not well-formed
//     (dt-wellformed). No WFC names this fault: WFC: Legal Character covers
//     character references only.
func (r *Reader) checkChars(raw string, off int64) error {
	for i := 0; i < len(raw); {
		c, n := utf8.DecodeRuneInString(raw[i:])
		if c == utf8.RuneError && n == 1 {
			return xsderr.New(xsderr.RuleXMLWellFormed, r.locAt(off+int64(i)), "ill-formed UTF-8 byte sequence in markup (XML 1.0 §4.3.3)")
		}
		if !xmlchar.IsChar(c) {
			return xsderr.New(xsderr.RuleXMLWellFormed, r.locAt(off+int64(i)), "character U+%04X in markup is no Char (XML 1.0 [2] Char, §2.2)", c)
		}
		i += n
	}
	return nil
}

// declareEntities records the general entity declarations and the <!ATTLIST>
// attribute definitions of a DOCTYPE directive at the document level, keeping
// for each entity name its first declaration and whether one outside every
// parameter entity is read (boundEntity), the first definition of each
// attribute of an element type (XML 1.0 §4.2, §3.3), and whether any
// declaration went unread. raw is the directive's source, "<!"
// through '>': the subset is read from it rather than from the decoder's
// Directive token, which replaces each comment with one space, so that a
// comment's own grammar can be checked (XML 1.0 [15] Comment). The directive
// stands at the document level, classify having charged one inside an element.
// A directive that is no doctypedecl, a DOCTYPE that is not well-formed where
// doctypeEntities checks it, and a DOCTYPE after the document element or after
// another DOCTYPE, which [22] prolog admits once and only before the document
// element, is a RuleXMLWellFormed fault at loc that declares nothing.
func (r *Reader) declareEntities(raw string, loc xsderr.Loc) error {
	body, _ := strings.CutPrefix(raw, "<!")
	body, _ = strings.CutSuffix(body, ">")
	decls, atts, unread, err := doctypeEntities(body, r.standalone, loc)
	if err != nil {
		return err
	}
	if r.ended {
		return xsderr.New(xsderr.RuleXMLWellFormed, loc, "DOCTYPE after the document element, where XML 1.0 [27] Misc admits none")
	}
	if r.doctype {
		return xsderr.New(xsderr.RuleXMLWellFormed, loc, "second DOCTYPE, where XML 1.0 [22] prolog admits only one")
	}
	r.doctype = true
	if unread {
		r.declsUnread = true
	}
	for _, att := range atts {
		key := attName{elem: att.elem, name: att.name}
		if _, bound := r.tokenized[key]; bound {
			continue
		}
		if r.tokenized == nil {
			r.tokenized = make(map[attName]bool)
		}
		r.tokenized[key] = att.tokenized
	}
	for _, decl := range decls {
		if bound, ok := r.entities[decl.name]; ok {
			if bound.onlyInPE && !decl.inPE {
				bound.onlyInPE = false
				r.entities[decl.name] = bound
			}
			continue
		}
		if r.entities == nil {
			r.entities = make(map[string]boundEntity)
		}
		r.entities[decl.name] = boundEntity{binding: decl, onlyInPE: decl.inPE}
		if r.dec.Entity == nil {
			r.dec.Entity = make(map[string]string)
		}
		r.dec.Entity[decl.name] = ""
	}
	return nil
}

// HasUnparsedEntity reports whether name is the name of an unparsed entity —
// one declared <!ENTITY name SYSTEM|PUBLIC ... NDATA notation> — in the
// document's DOCTYPE internal subset: the [unparsed entities] property of the
// document information item, which an ·ENTITY value· must name (Structures
// §3.16.4 key-vde, Appendix D).
//
// The answer is final once Token has returned the document element's
// StartElement: XML 1.0 places the doctypedecl before the document element,
// so no declaration can arrive later. Asked earlier, it reports only what has
// been read so far.
//
// The internal subset is read with its internal parameter entities expanded;
// the external DTD subset is never read, by design (#1668), nor is an external
// parameter entity, and a declaration after a parameter-entity reference that
// was not read is not processed unless the document is standalone="yes" (XML
// 1.0 §5.1). An unparsed entity declared only where the reader did not read
// is reported false, and AllDeclarationsProcessed then reports false too.
func (r *Reader) HasUnparsedEntity(name string) bool {
	return r.entities[name].binding.unparsed
}

// AllDeclarationsProcessed reports the document information item's [all
// declarations processed] (XML Infoset §2.1): whether the reader read the
// complete DTD. It is false when the DOCTYPE names an external subset, which
// is never read, or when the internal subset references a parameter entity
// the reader did not read (see HasUnparsedEntity), whatever the document's
// standalone declaration says.
//
// The answer is final once Token has returned the document element's
// StartElement, on HasUnparsedEntity's terms.
func (r *Reader) AllDeclarationsProcessed() bool {
	return !r.declsUnread
}

// checkDeclaration enforces XML 1.0 §4.3.3's fatal error: "it is a fatal error
// for an entity including an encoding declaration to be presented to the XML
// processor in an encoding other than that named in the declaration". The
// byte-order mark is the evidence of the encoding the entity was presented in.
//
// It catches the direction the XML decoder cannot: a declaration naming UTF-8
// is the decoder's default and never reaches the mark's CharsetReader, so a
// UTF-16 mark contradicting it would otherwise pass unnoticed. pi is the
// document's XML declaration (see xmlDeclFault).
func (r *Reader) checkDeclaration(pi xml.ProcInst, loc xsderr.Loc) error {
	name := pseudoAttr(string(pi.Inst), "encoding")
	if name == "" || r.bom.AgreesWith(name) {
		return nil
	}
	// xmlenc.Mark.String names the mark itself, so the message states the
	// entity's actual encoding rather than repeating "byte-order mark".
	return xsderr.New(xsderr.RuleXMLWellFormed, loc, "encoding declaration %q disagrees with the entity's actual encoding: %s", name, r.bom)
}

// startElement resolves an element's name, tag, and attributes, attrs, against
// the scope its declarations establish, pushes an open-element frame, and
// returns the node.
func (r *Reader) startElement(tag xml.Name, attrs []rawAttr, loc xsderr.Loc) (*StartElement, error) {
	bindings := bindingsOf(attrs)
	if err := r.checkUndeclarations(bindings, loc); err != nil {
		return nil, err
	}
	child := r.currentScope().child(bindings)

	if tag.Space == xmlnsPrefix {
		return nil, xsderr.New(xsderr.RuleXMLWellFormed, loc, "%q is a reserved prefix and cannot name an element", xmlnsPrefix)
	}
	space, ok := child.lookup(tag.Space)
	if !ok {
		return nil, xsderr.New(xsderr.RuleXMLWellFormed, loc, "unbound namespace prefix %q on element <%s>", tag.Space, rawName(tag))
	}
	name := Name{space: space, local: tag.Local}

	resolved, err := resolveAttrs(attrs, child, loc)
	if err != nil {
		return nil, err
	}

	r.stack = append(r.stack, frame{name: name, scope: child, loc: loc})
	return &StartElement{name: name, attrs: resolved, scope: child, loc: loc}, nil
}

// checkUndeclarations enforces Namespaces in XML 1.0's nsc-NoPrefixUndecl: "In
// a namespace declaration for a prefix ... the attribute value MUST NOT be
// empty." Undeclaring a prefix with xmlns:p="" is a Namespaces in XML 1.1
// feature, so the constraint holds unless the document is labelled
// version="1.1"; a document with no XML declaration is XML 1.0. The default
// namespace declaration xmlns="" is exempt (xml-names §6.2).
func (r *Reader) checkUndeclarations(bindings []binding, loc xsderr.Loc) error {
	if r.decl.Version() == "1.1" {
		return nil
	}
	for _, b := range bindings {
		if b.prefix != "" && b.uri == "" {
			return xsderr.New(xsderr.RuleXMLWellFormed, loc, "namespace declaration xmlns:%s has an empty value: only an XML 1.1 document may undeclare a prefix (nsc-NoPrefixUndecl)", b.prefix)
		}
	}
	return nil
}

// endElement matches an end tag against the open element and pops it.
func (r *Reader) endElement(t xml.EndElement, loc xsderr.Loc) (*EndElement, error) {
	if len(r.stack) == 0 {
		return nil, xsderr.New(xsderr.RuleXMLWellFormed, loc, "unexpected end tag </%s> with no open element", rawName(t.Name))
	}
	top := r.stack[len(r.stack)-1]
	space, ok := top.scope.lookup(t.Name.Space)
	if !ok {
		return nil, xsderr.New(xsderr.RuleXMLWellFormed, loc, "unbound namespace prefix %q on end tag </%s>", t.Name.Space, rawName(t.Name))
	}
	got := Name{space: space, local: t.Name.Local}
	if got != top.name {
		return nil, xsderr.New(xsderr.RuleXMLWellFormed, loc, "end tag </%s> does not match open element %s", rawName(t.Name), qname(top.name))
	}
	r.stack = r.stack[:len(r.stack)-1]
	r.ended = len(r.stack) == 0
	return &EndElement{name: got, loc: loc}, nil
}

// outsideRootFault enforces XML 1.0 §2.1's well-formedness clause 1 for the
// character data outside the document element: [1] document ::= prolog element
// Misc*, [22] prolog ::= XMLDecl? Misc* (doctypedecl Misc*)?, and [27] Misc ::=
// Comment | PI | S, so character data before or after it must be S [3]. raw is
// the SOURCE of one character-data token read at the document level, starting
// at loc, and the test reads raw, never the decoded text: a character
// reference and a CDATA section are content [43] and match no Misc, whatever
// they decode to, and a U+FEFF past the encoding signature (§4.3.3) is no S.
// The fault is located at loc, the token's own start, before the document
// element or after its end tag (r.ended), each with its own message.
func (r *Reader) outsideRootFault(raw string, loc xsderr.Loc) error {
	if strings.Trim(raw, declSpace) == "" {
		return nil
	}
	if r.ended {
		return xsderr.New(xsderr.RuleXMLWellFormed, loc, "character data after the document element: only comments, processing instructions and white space may follow it (XML 1.0 [1] document, [27] Misc)")
	}
	return xsderr.New(xsderr.RuleXMLWellFormed, loc, "character data before the document element: only an XML declaration, a DOCTYPE, comments, processing instructions and white space may precede it (XML 1.0 [1] document, [22] prolog, [27] Misc)")
}

// currentScope is the scope in force for the innermost open element, or nil
// (the empty base scope) at the document level.
func (r *Reader) currentScope() *scope {
	if len(r.stack) == 0 {
		return nil
	}
	return r.stack[len(r.stack)-1].scope
}

// bindingsOf extracts the namespace declarations from an element's raw
// attributes, in document order. RawToken reports xmlns:p as {Space:"xmlns",
// Local:"p"} and the default xmlns as {Space:"", Local:"xmlns"}.
func bindingsOf(attrs []rawAttr) []binding {
	var bs []binding
	for _, ra := range attrs {
		a := ra.attr
		if a.Name.Space == xmlnsPrefix {
			bs = append(bs, binding{prefix: a.Name.Local, uri: a.Value})
			continue
		}
		if a.Name.Space == "" && a.Name.Local == xmlnsPrefix {
			bs = append(bs, binding{prefix: "", uri: a.Value})
		}
	}
	return bs
}

// resolveAttrs resolves the non-declaration attributes of an element against
// scope s, in document order (STYLE D2: order is preserved, no map). An
// unprefixed attribute is in no namespace; a prefixed one whose prefix is
// unbound is an error, located at loc, the start tag. Each Attribute keeps the
// position of its own name (rawAttr).
func resolveAttrs(attrs []rawAttr, s *scope, loc xsderr.Loc) ([]Attribute, error) {
	var out []Attribute
	for _, ra := range attrs {
		a := ra.attr
		if isDeclaration(a) {
			continue
		}
		if a.Name.Space == xmlnsPrefix {
			return nil, xsderr.New(xsderr.RuleXMLWellFormed, loc, "%q is a reserved prefix and cannot name an attribute", xmlnsPrefix)
		}
		space := ""
		if a.Name.Space != "" {
			resolved, ok := s.lookup(a.Name.Space)
			if !ok {
				return nil, xsderr.New(xsderr.RuleXMLWellFormed, loc, "unbound namespace prefix %q on attribute %s", a.Name.Space, rawName(a.Name))
			}
			space = resolved
		}
		out = append(out, Attribute{name: Name{space: space, local: a.Name.Local}, value: a.Value, loc: ra.loc})
	}
	return out, nil
}

// isDeclaration reports whether a raw attribute is a namespace declaration
// (xmlns or xmlns:p) rather than a real attribute.
func isDeclaration(a xml.Attr) bool {
	if a.Name.Space == xmlnsPrefix {
		return true
	}
	return a.Name.Space == "" && a.Name.Local == xmlnsPrefix
}

// rawName renders a RawToken name (Space holds the raw prefix) as it appeared
// in the source, for error messages.
func rawName(n xml.Name) string {
	if n.Space == "" {
		return n.Local
	}
	return n.Space + ":" + n.Local
}

// qname renders a resolved Name for error messages as "{uri}local", or just
// local for a name in no namespace.
func qname(n Name) string {
	if n.space == "" {
		return n.local
	}
	return "{" + n.space + "}" + n.local
}

// locAt maps a byte offset to a 1-based (line, column) using the newline
// index, sort-searched on demand (STYLE P4: no retained document content).
// Column counts bytes from the start of the line.
func (r *Reader) locAt(off int64) xsderr.Loc {
	if off < 0 {
		off = 0
	}
	nls := r.pos.newlines
	before := sort.Search(len(nls), func(i int) bool { return nls[i] >= off })
	lastNL := int64(-1)
	if before > 0 {
		lastNL = nls[before-1]
	}
	return xsderr.Loc{URI: r.uri, Line: before + 1, Col: int(off - lastNL)}
}

// posReader wraps the input, counting bytes and recording the offset of every
// newline so line/column can be derived without keeping the content. It grows
// with the number of lines, not the document size.
//
// It also keeps the source of the token being read: raw holds every byte read
// from offset base on, and release drops the bytes before a token's start. The
// decoder reads through a buffer of its own, so raw holds one token and at
// most that buffer's read-ahead past it.
type posReader struct {
	r        io.Reader
	off      int64
	newlines []int64
	raw      []byte
	base     int64
}

// Read reads from the underlying reader, recording newline offsets and the
// source as bytes pass through.
func (p *posReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	for i := 0; i < n; i++ {
		if b[i] == '\n' {
			p.newlines = append(p.newlines, p.off+int64(i))
		}
	}
	p.raw = append(p.raw, b[:n]...)
	p.off += int64(n)
	return n, err
}

// release drops the source before offset off, the start of the next token.
func (p *posReader) release(off int64) {
	p.raw = p.raw[off-p.base:]
	p.base = off
}

// span returns the source from offset from to offset to, both within the
// token being read.
func (p *posReader) span(from, to int64) string {
	return string(p.raw[from-p.base : to-p.base])
}
