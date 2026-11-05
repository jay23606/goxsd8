// Package xmltree is a streaming, position-tracking XML reader: the
// origin of every xsderr.Loc in an XML document.
//
// It is independent of the rest of the module (leaf besides xsderr and the
// internal/ leaves internal/xmldecl, internal/xmlenc, internal/xmlname,
// internal/xmlchar and internal/xmltok) and used for both schema documents
// (parser) and XML instances (validate/xmlsrc).
//
// # Contract (implemented in M2)
//
//   - Streaming with bounded memory: wraps the io.Reader, never
//     io.ReadAll (STYLE P4). Line/column mapping uses an offset index
//     over newline positions (sort-searched on demand), not retained
//     document content.
//   - Namespace-scoped: prefixes resolve against in-scope bindings at
//     each node; unbound prefixes are reported as errors with location,
//     never passed through as if they were namespaces. A start tag's
//     whole set of PREFIXED in-scope bindings is enumerable in a
//     deterministic order (StartElement.InScopePrefixes), for consumers
//     that must carry a namespace context forward rather than resolve one
//     name; the default namespace stays a separate LookupPrefix("") fact.
//   - Every node (element, attribute, character data) answers Loc()
//     (URI, line, column) and, for character content, the byte offset —
//     decode errors downstream cite it.
//   - Nodes are immutable once produced: private fields, getter methods
//     (STYLE T1).
//   - Byte-order marks are honoured per XML 1.0 §4.3.3: a UTF-16 mark
//     (FE FF, FF FE) selects a streaming transcode to UTF-8, a UTF-8
//     mark is dropped as the encoding signature it is (both
//     internal/xmlenc, shared with the conformance harness's raw
//     re-reads), and an encoding declaration that disagrees with the
//     mark is that section's fatal error, reported as
//     RuleXMLWellFormed. Locations are offsets into the decoded UTF-8
//     stream, not into the source bytes.
//   - An ill-formed UTF-8 code unit sequence anywhere in the decoded
//     stream is §4.3.3's fatal error, reported as RuleXMLWellFormed: in
//     character data, attribute values and names by the decoder
//     (internal/xmltok), and in a comment, processing instruction or
//     directive, the DOCTYPE and its internal subset included, by the
//     reader at the sequence's first byte.
//   - A well-formed UTF-8 sequence encoding no XML 1.0 [2] Char in a
//     comment, processing instruction or directive, the DOCTYPE and its
//     internal subset included, is rejected as RuleXMLWellFormed at the
//     sequence's first byte (§2.2, [15], [16]); the decoder charges one
//     in character data and attribute values.
//   - GAP(xml): UTF-16 without a mark, declared only by encoding=, is
//     not decoded — it fails well-formedness rather than being read.
//     Tracked by #361.
//   - Element, attribute and PI target names and entity references, in
//     the document and in an entity's replacement text, are checked
//     against XML 1.0 5th edition's [5] Name (internal/xmltok); a name
//     outside it is RuleXMLWellFormed. So is a DOCTYPE whose document type
//     name is missing or outside [5] (XML 1.0 [28] doctypedecl, checked
//     against internal/xmlname at the directive), and, at the directive, a
//     directive outside the document element that is no doctypedecl: a
//     keyword other than "DOCTYPE" in that case, <!FOO> or <!doctype r>, or
//     one with no S after it, <!DOCTYPEr> ([22] prolog, [27] Misc, [28]).
//   - A document whose XML declaration specifies a 1.x version number
//     other than 1.0 is read as a 1.0 document (XML 1.0 §2.8 Note), by a
//     same-length rewrite of that number (internal/xmldecl), so locations
//     are the source's own. Only the label is admitted: a 1.1-only
//     feature, such as a C0 character reference, is read as XML 1.0 reads
//     it. The one exception is prefix undeclaration: xmlns:p="" undeclares
//     p in a document labelled version="1.1" (Namespaces in XML 1.1), and
//     is rejected as RuleXMLWellFormed in any other document, which is XML
//     1.0 (nsc-NoPrefixUndecl). The default declaration xmlns="" is legal
//     in both.
//   - Outside the document element only Misc — comments, processing
//     instructions and white space — may appear, and before it an XML
//     declaration and a DOCTYPE too (XML 1.0 [1] document, [22] prolog,
//     [27] Misc). A character-data run before or after the document
//     element must be literal S in the source, or it is rejected as
//     RuleXMLWellFormed at the run's start, the character after the
//     preceding markup. A character reference, a CDATA section or a
//     U+FEFF after the encoding signature is not S, whatever it decodes
//     to. Also rejected as RuleXMLWellFormed: a second top-level element,
//     at its start tag; a DOCTYPE after the document element or after
//     another DOCTYPE, at the directive, declaring nothing ([22] admits
//     one, before the element); and a processing instruction whose target
//     is "xml" in any case anywhere but as the XML declaration, spelled in
//     lower case at the document's first character, after no white space,
//     comment or other declaration ([17] PITarget, [23] XMLDecl), at the
//     instruction, so a misplaced standalone="yes" is never read.
//   - Inside the document element, and in the replacement text of an
//     internal entity referenced there (§4.3.2), a directive and a
//     processing instruction whose target is "xml" in any case are
//     rejected as RuleXMLWellFormed ([43] content, [17] PITarget), at the
//     token or, in replacement text, at the reference. Replacement text
//     never referenced is not checked for them. An external parsed entity
//     is never read, so its TextDecl is never charged.
//   - The DOCTYPE's internal subset is read for its general entity
//     declarations, the first declaration of a name binding (XML 1.0 §4.2),
//     and for its attribute-list declarations' attribute types (see the
//     attribute-value bullets below). Its unparsed entities (<!ENTITY name
//     SYSTEM|PUBLIC ... NDATA notation>) are the document's [unparsed
//     entities] property, answered by Reader.HasUnparsedEntity and final once
//     the document element's start tag is read; the subset itself surfaces as
//     no Node, a prolog fact not being part of the element/character-data
//     stream. Internal parameter entities are expanded, bounded in depth and
//     size. Rejected as RuleXMLWellFormed at the directive, in the subset or
//     in an expanded parameter entity's replacement text: text between
//     declarations that is neither S nor a PEReference — a '%' run that is no
//     PEReference, or a '<!' opening no markup declaration, among it ([28b]
//     intSubset, [28a] DeclSep, [69] PEReference, WFC: PE Between
//     Declarations); a comment holding "--" ([15]); a processing instruction
//     whose target is no Name or is "xml" in any case ([16], [17]); a notation
//     declaration that is no [82] NotationDecl; a '<' outside a markup
//     declaration's literals ([45], [52], [70], [82]); a parameter-entity
//     reference inside a markup declaration, an entity value literal included
//     (WFC: PEs in Internal Subset), and any other '%' in an entity value
//     literal ([9]); an <!ENTITY> that is no [70] EntityDecl — no S after its
//     keyword, a declared name, general or parameter, that is no Name ([71],
//     [72], [5]), a missing definition, one that is neither one EntityValue
//     nor an ExternalID ([73], [74], [75], [11], [12], [13]), an NDataDecl in
//     a parameter entity's PEDef ([74]) or anything but one after a general
//     entity's ExternalID ([76]), and an entity value literal whose '&' opens
//     no Reference ([66]–[68]) or whose character reference names no Char
//     (WFC: Legal Character), referenced or not; an <!ELEMENT> that is no [45]
//     elementdecl, its contentspec no 'EMPTY', 'ANY', Mixed or children
//     ([46]–[51]); an <!ATTLIST> that is no [52] AttlistDecl, an AttDef
//     missing its S, AttType or DefaultDecl ([53]–[60]), or a default value
//     that is no [10] AttValue, holding a '<', a '&' that opens no Reference
//     or a character reference naming no Char, or an entity reference that
//     breaks WFC: Entity Declared (in a document that is standalone="yes" or
//     has neither an external subset nor a parameter-entity reference, a
//     default value outside every parameter entity referencing directly a
//     name no general entity declaration before the <!ATTLIST> declares, or
//     through another entity's replacement text a name no declaration
//     declares, a declaration in a parameter entity's replacement text
//     counting for neither, and a reference in the replacement text of an
//     entity whose binding declaration stands in one occurring within it)
//     or, directly or through other entities' replacement text, WFC: Parsed
//     Entity, No Recursion, No External Entity References or No < in
//     Attribute Values, or a '&' in such replacement text that begins no
//     Reference (§4.4.5, [67]), whether or not the default is ever applied —
//     a validity constraint on either declaration is no fault; replacement
//     text ending inside a comment, processing instruction or markup
//     declaration; and a '<' in the DOCTYPE header, a subset no ']' closes,
//     or text other than S between that ']' and '>' ([28] doctypedecl). A
//     processing instruction between the subset's declarations runs to the
//     "?>" that alone closes it ([16] PI), whatever '>', '<' or quote it
//     holds (internal/xmltok), and one no "?>" closes fails as the decoder's
//     syntax error at the end of input. The external subset is never read,
//     by design (#1668), nor is an external parameter entity; after a
//     parameter-entity reference that is not read, unless standalone="yes",
//     the rest of the internal subset is checked for well-formedness alone,
//     binding no parameter entity, declaring no general entity, defining no
//     attribute and expanding no reference (XML 1.0 §5.1). An entity declared
//     only where the reader did not read is not a member, and
//     Reader.AllDeclarationsProcessed, the [all declarations processed]
//     property, then reports false.
//   - Every attribute value, namespace declarations included, is its
//     normalized value per XML 1.0 §3.3.3, steps 1-3 (Attribute.Value): a
//     character reference is the character it names, so &#9; stays a tab,
//     while each literal #x9, #xA or #xD is #x20, a line end (§2.11) one
//     #x20.
//   - An attribute the internal subset's <!ATTLIST> declarations define
//     with an AttType other than CDATA — tokenized, NOTATION or an
//     enumeration ([54]–[59]) — then has leading and trailing #x20
//     discarded and each run of #x20 replaced by one (§3.3.3's paragraph
//     after step 3), so a namespace declaration binds the collapsed
//     value; one defined as CDATA, or not at all, is read as CDATA. A
//     definition applies to the element type and attribute names as the
//     declaration and the tag spell them, prefix included, and the first
//     definition of an attribute of an element type binds (§3.3). One
//     after a parameter-entity reference that is not read, outside a
//     standalone="yes" document, is not processed (§5.1).
//   - A reference to an internal general entity is replaced by its
//     replacement text (XML 1.0 §4.4.2, §4.4.5): in content, parsed as
//     content in the scope in force at the reference, its nodes located at
//     the reference; in an attribute value, normalized per §3.3.3 together
//     with the rest of that value. Nested references expand at inclusion
//     (§4.5). A recursive reference, a '<' in replacement text an attribute
//     value includes, a '&' in included replacement text that begins no
//     Reference (a literal's `&#38;` puts one there; §4.4.2, [67]),
//     replacement text that is not balanced content, and, in a
//     standalone="yes" document, a reference to an entity declared only in a
//     parameter entity's replacement text, internal, external or unparsed
//     (WFC: Entity Declared), are RuleXMLWellFormed faults; a reference in
//     the replacement text of an entity whose binding declaration (§4.2)
//     stands in a parameter entity occurs within it, and is not charged. A
//     reference, directly or through replacement text, to an unparsed entity
//     whose declaration the reader read (WFC: Parsed Entity), in content or
//     in an attribute value, and a reference in an attribute value to an
//     external entity whose declaration the reader read (WFC: No External
//     Entity References) are RuleXMLWellFormed faults too. A reference past
//     the reader's bound on nesting depth or on replacement text included per
//     document is refused, as are a reference in content to an external
//     parsed entity, which the reader does not include (§4.4.3), and a
//     reference to an entity whose declaration the reader did not read,
//     wrapping a cause: the reader does not decide whether the document is
//     well-formed.
//   - Every error the reader returns but io.EOF is a RuleXMLWellFormed
//     *xsderr.Error, and whether it wraps a cause says what it decides. One wrapping no
//     cause is a charge the reader makes itself and a definite fault whatever
//     the document's DTD declares — an entity it includes or charges is one whose
//     internal-subset declaration it read, which binds before the external
//     subset's (XML 1.0 §2.8, §4.2) — except the unbound-prefix charge: an
//     <!ATTLIST can default the namespace declaration that binds the prefix
//     (§3.3.2), a non-validating processor must supply that default from the
//     internal subset (§5.1), and the reader applies no attribute default.
//     A WFC Parsed Entity charge against an unparsed entity declared after a
//     reference to a parameter entity the reader did not read, which may
//     have held an overriding declaration, is made only in a standalone="yes" document,
//     and is definite there: "when `standalone="yes"`, processors MUST process these
//     declarations" (§5.1), so that declaration binds (§4.2; #2459). One wrapping a
//     cause — an encoding/xml syntax error, among them its refusal of an encoding
//     declaration the reader does not decode, an I/O fault, a start tag whose
//     attributes the reader's re-read of its source does not find as the decoder
//     read them, or one of the refusals above — may be a limit of this reader.
//
// Fuzz targets guard the reader against panics on malformed input
// (PRINCIPLES 24); malformed XML is an error value, never a crash.
package xmltree
