// Package validate assesses instance documents against a compiled schema
// set, over an abstract infoset so XML, JSON, and BER sources plug in as
// adapters.
//
// # The abstract infoset
//
// The engine consumes [Element], [Attribute], [Text] and the [Children]
// cursor over the [Child] sum — never a concrete decoder's types
// (PRINCIPLES 8). They carry the Appendix D properties a cvc- rule reads
// off an information item, plus the Loc a diagnostic cites. Adapters
// construct infoset values and hand them over:
//
//	validate/xmlsrc   XML instances via parser/xmltree      implemented
//	validate/jsonsrc  JSON instances                        implemented
//	validate/bersrc   BER-encoded instances                 planned (M11), unbuilt
//
// No package of this module in the engine's import closure imports
// encoding/xml, encoding/json, or a BER decoder; only the adapters do.
// (log/slog carries encoding/json into every closure in the module for
// its JSONHandler — the test that pins this boundary says which form of
// the ban each path is held to.)
//
// Assessment is streaming: [Children] is a pull cursor, so a source
// yields one child at a time and the walk never holds a document.
//
// A later infoset property arrives as a NEW optional capability
// interface the engine narrows to, and NEVER as a method added to
// Element, Attribute or Text (PRINCIPLES 3). Those three are implemented
// by adapter packages outside this module, so a method added to one
// breaks every adapter at once. [UnparsedEntities] and
// [DeclarationsProcessed] are the capabilities that have shipped: the
// document's [unparsedEntities] and [all declarations processed], each
// narrowed once off the root [Validator.Assess] is handed.
//
// Each capability's own doc comment decides what its absence means, and
// the contracts differ. [unparsedEntities] absent REJECTS: Appendix
// D makes every ENTITY or ENTITIES value of a source that does not
// support the property fail String Valid clause 3. [all declarations
// processed] absent is taken as TRUE, and only words a diagnostic: a
// name missing from [unparsedEntities] is reported as plainly undeclared
// rather than as possibly declared where the source did not read.
// [[base URI]] absent would FALL BACK: a planned interface{ BaseURI()
// string }, not yet built, would answer Loc().URI where an adapter does
// not implement it. [[prefix]], [[base URI]] and [[attribute type]] are
// the Appendix D properties still waiting on that route: no cvc- rule
// reads one today.
//
// # Assessment semantics designed in from the start
//
//   - Content-model matching NEVER BACKTRACKS — UPA fixes which particle
//     takes each item, and a bounded set of live partitions decides which
//     iteration of a repeated ancestor it falls in — and explicit content
//     beats an open-content wildcard at the current state (PRINCIPLES 14).
//     The matcher is xsd's pull walk driver.
//   - Empty content is stricter than element-only: a type whose particle
//     can never match an element admits no character content at all, not
//     even whitespace (PRINCIPLES 13).
//   - Parent element context is threaded through the whole chain: ID
//     harvesting under value constraints, EDC's post-xsi:type governing
//     type, and namespace context for identity constraints all need it. An
//     xsi:type displaces the ·governing type definition· at every depth, not
//     only at the ·validation root·.
//   - Conditional type assignment: a declaration's {type table}
//     ·conditionally selects· the ·selected type definition· an xsi:type
//     then ·overrides· or does not, through xpath's §3.12.6
//     required-subset evaluator (cta.go). A {test} reads the element's own
//     [[attributes]] and, behind them, its [inherited attributes]
//     (§3.3.5.6), which the descent computes top-down and threads to every
//     child it does not skip (inherited.go). A {test} the evaluator
//     declines withholds the element's ·governing type definition·
//     instead of falling back to the declared type, and is recorded as an
//     [Unevaluated] under key-cta-ta-select (§3.12.4) — nothing else
//     in the [Result] reports it, since a withheld type charges the
//     element nothing. A dynamic or type error inside a {test} the
//     evaluator DOES compile is recorded by nothing: clause 2 of that rule
//     makes it a decided false, so the alternative was tried and the scan
//     continues.
//   - Identity constraints: node tables propagate UPWARD — a keyref on
//     element E resolves only against key sequences sourced within E's
//     own subtree; selector/field paths honor xpathDefaultNamespace for
//     element steps and never for attribute steps (PRINCIPLES 15).
//   - Union values validate against DirectMembers in order, with the
//     validating member's whiteSpace driving pattern normalization
//     (PRINCIPLES 11).
//   - Assertions are VISITED at every variety level. A complex type's
//     {assertions} member is EVALUATED where xpath compiles its {test}
//     over the element's typed attributes and children, and charged under
//     cvc-assertion (§3.13.4.1) where it is false or raises; one xpath does
//     not compile is recorded as an [Unevaluated] under cvc-assertion. List
//     item and list, union member and union, atomic: a simple type's
//     assertions facet is EVALUATED inside String Valid by
//     xpath.FacetAssertions wherever the cvc-datatype-valid recursion reaches
//     it, a failed {test} being part of the Datatype Valid verdict the caller
//     charges and choosing a union's ·validating type· (dt-active-member); one
//     it declines is recorded as an [Unevaluated] under cvc-assertions-valid
//     (§4.3.13.3). The same evaluator maps both sides of a fixed-value
//     comparison (cvc-attribute clause 4, cvc-au, cvc-elt clause 5.2.2.2.2,
//     value.ConstraintMatches), where a {test} it declines or fails leaves
//     that clause's own [Unevaluated]. No unevaluated site is ever reported as
//     satisfied (PRINCIPLES 12, cvcassertion.go). Every {test} of one
//     [Validator.Assess] call, of either kind, is evaluated at one current
//     dateTime, which fn:current-date reads: the clock is read once, at the
//     start of the call, and held constant for the assessment episode
//     (cvc-xpath clause 6, §3.13.4.2).
//   - A *Validator is safe for concurrent use by multiple goroutines: [New]
//     builds it once from an already-finalized [xsd.Schema] and a
//     [value.Backend], and every [Validator.Assess] call builds and drops
//     its own walk state (see the Validator doc), so two goroutines
//     assessing through the same *Validator never observe each other's
//     progress. This holds only as far as the backend does: a
//     value.Backend implementation with its own unsynchronized per-call
//     state is the caller's to make safe, not this package's.
//
// # Contract (M5, landing rule by rule)
//
// [Result] carries every violation charged so far as an *xsderr.Error
// (cvc-* rule + instance and/or schema Loc), in document order. Ten rules
// are charged today, at the ·validation root· and at every descendant the
// descent assesses — against a ·governing element declaration·, against a
// ·governing type definition· alone, or, ·laxly assessed·, against xs:anyType.
//
// Read the verdict off [Result.Violations]; [Result.Err] reports whether the
// walk finished, not whether the document is valid. An empty Violations means
// NOT PROVEN INVALID rather than valid: [Validator.Assess] decides local
// validity alone, which is clause 1.1.1 of the [validity] property
// (§3.3.5.1), plus clause 1.1.3 for an element [[child]] ·attributed· to a
// strict ·wildcard particle· whose own validity is notKnown, charged at the
// ENCLOSING element (assess.go). It evaluates neither clause 1.1.2 — an
// invalid descendant or attribute — nor clause 2, which makes an item that was
// not ·strictly assessed· notKnown rather than valid. A consumer that may
// accept only what has been shown valid cannot read an empty result as that
// showing.
//
// An empty [Result.Violations] with a non-empty [Result.Unevaluated] is not a
// pass either: each entry there is a check the assessment REACHED and did not
// perform, so the document stands undecided against exactly the rules those
// entries name.
//
// All four charges below that DELEGATE to String Valid (§3.16.4,
// cvc-simple-type) carry the delegated verdict as their wrapped cause, so
// errors.Unwrap/Is/As reach the inner *xsderr.Error and [xsderr.RuleOf] reads
// the inner rule ID off it instead of a consumer scraping the message for one.
// cvc-attribute clause 3 (but for xsi:nil's, below), cvc-type clause 3.1.3,
// cvc-complex-type clause 1.2 and cvc-complex-type clause 4 over a ·defaulted
// attribute·'s {lexical form} each wrap the String Valid verdict. Where clause
// 2 fails, that is the Datatype Valid (Datatypes §4.1.4) verdict, whose own
// rule is cvc-datatype-valid or one of the facet rules under it, and no
// intermediate cvc-simple-type node sits between. Where clause 3 fails — an
// ·ENTITY value· that is not a ·declared entity name· — the verdict is
// cvc-simple-type's own, since that clause delegates to nothing. All four read
// clause 2's verdict off value.ValidateLexical directly, clause 4 mapping the
// {lexical form} under the bindings its {value constraint} captured
// (value.ConstraintContext). Among those verdicts is a NOTATION value whose
// QName names no notation declaration of the schema, outside NOTATION's ·value
// space· (Datatypes §3.3.19): value.ValidateLexical decides it against the
// schema it is handed, under cvc-datatype-valid. Error() still renders each
// wrapped verdict into the message as well, for a reader who holds only the
// string.
//
// Whether a cause is there is read off Unwrap and never off the rule ID:
// clauses 1.2 and 4 both charge under cvc-complex-type and both wrap the
// verdict; the rule's other clauses wrap nothing.
//
// The first comes from [Validator.Assess]'s dispatch on the root's ·governing
// element declaration·: cvc-assess-elt (§3.3.4.6) for a root that determines
// neither a declaration nor a ·governing type definition·. cvc-assess-elt
// carries one charge more, at every element the descent types: e-validity
// (§3.3.5.1) clause 1.1.3, for an element [[child]] ·attributed to· a strict
// ·wildcard particle· that ·resolves· to no declaration, or to one §5.3 sends
// to ·lax assessment·. e-validity has no catalog ID of its own and none is
// minted for it, so the message names the clause against e-validity while the
// error carries cvc-assess-elt.
//
// The second is cvc-elt (§3.3.4.3), in five clauses, at the root and at every
// descendant the descent types. Clause 1 is charged as Structures §5.3
// (Missing Sub-components) directs, for an element whose ·governing element
// declaration· has a {substitution group affiliations} member, or whose
// ·selected type definition· names a substitution group head, that no
// declaration carries, and the element then falls back to ·lax assessment·
// against xs:anyType; a nil {type definition} on a directly built declaration is
// recorded as an [Unevaluated] instead (cta.go). Clause 2 decides {abstract}: an
// element whose ·governing element declaration· is abstract, whether the root's
// top-level declaration, a particle's, or the one a strict or lax wildcard's
// child ·resolves· to. Clause 3 decides xsi:nil: an xsi:nil attribute on a
// declaration whose {nillable} is false (3.1), one whose lexical is outside
// xs:boolean's lexical space (3.2), and a ·nilled· element carrying character or
// element [[children]] (3.2.3.1) or a fixed {value constraint} (3.2.3.2). Clause
// 4 decides xsi:type: an ·instance-specified type definition· that ·resolves·
// but is not ·validly substitutable· for the ·selected type definition· subject
// to the declaration's {disallowed substitutions}
// ([xsd.Schema.ValidlySubstitutable]). One that resolves AND overrides becomes
// the ·governing type definition· the rest of the assessment reads, and one that
// does not resolve at all charges no clause of cvc-elt and leaves the selected
// type governing — the Note under cvc-elt is explicit that the two failures
// share that fallback and differ only in the charge. That second failure is
// charged against the ATTRIBUTE instead, under cvc-attribute clause 3 or 5
// below, which leaves the fallback exactly as it is. Clause 5 is a case split on
// whether the element is EMPTY and its declaration carries a {value constraint},
// and both arms are decided. Clause 5.2's, for an element that HAS [[children]]:
// 5.2.1's ordinary cvc-type dispatch, and 5.2.2 for a fixed constraint — no
// element [[children]] (5.2.2.1), and an ·initial value· matching the {lexical
// form} under a mixed {content type} (5.2.2.2.1) or an ·actual value· equal or
// identical to the {value} under a simple one (5.2.2.2.2). Clause 5.1's, for an
// empty one: the item assessed is the one carrying D.{value constraint}.{lexical
// form} as its ·normalized value·, which 5.1.2 sends to the same cvc-type
// dispatch and which the [ID/IDREF table] and a ·key-sequence· read in place of
// the empty ·initial value·, each mapping that {lexical form} under the
// namespace bindings in scope where the schema document wrote it
// (value.ConstraintContext, Datatypes §3.3.18) and never under the element's;
// and 5.1.1 charges Element Default Valid (Immediate) (§3.3.6.2) over that
// constraint where the ·governing type definition· is an ·instance-specified·
// one, which is the xsi:type case alone.
//
// The third is cvc-type (§3.3.4.4) clause 3.1, the arm taken where the
// ·governing type definition· is a Simple Type Definition rather than a complex
// one. Clause 3.1.1 admits no [[attributes]] beyond xsi:type, xsi:nil,
// xsi:schemaLocation and xsi:noNamespaceSchemaLocation, charged per offending
// attribute at its own Loc; clause 3.1.2 admits no element information item
// [[children]] at all; and clause 3.1.3 has the element's ·initial value·
// validated against that type per String Valid (§3.16.4), through the same
// value.Backend and with the same declines as the simple-content charge below.
// The first two apply to a ·nilled· element as much as to any other; the third
// alone is skipped for one. Clause 2 is charged under the same rule, at every
// element whose ·governing type definition· is a complex type with {abstract}
// true, whichever clause of key-governing-type-elem settled that type; the
// element is still assessed against it. Clause 1 is not evaluated.
//
// Three more are the root's attribute half, against its ·governing
// type definition·'s {attribute uses} and {attribute wildcard}.
// cvc-complex-type (§3.4.4.2) clauses 2 and 3 decide EXISTENCE and need no
// value space. Clause 4 and the two rules clause 2.1 dispatches to —
// cvc-attribute (§3.2.4.1) clauses 3 and 4 and cvc-au (§3.5.4) — decide
// VALUES, and read them through the value.Backend [New] takes: an
// attribute's lexical against its declaration's {type definition} per
// String Valid (§3.16.4), its ·actual value· against a fixed {value
// constraint} on the declaration and on the use (two independent rules over
// two properties, both charged), and a ·defaulted attribute·'s own {lexical
// form} against its type.
//
// cvc-attribute clauses 3 and 4 are charged on the same terms against an
// attribute ·attributed to· a strict or lax {attribute wildcard} under clause
// 2.2 — the governing type's, or xs:anyType's lax one on a ·laxly assessed·
// element (§3.4.7) — with the top-level declaration its ·expanded name·
// ·resolves· to as its ·governing attribute declaration· (key-governing-ad
// clause 3). A ·skipped· attribute, attributed to a skip wildcard, and one
// whose name resolves no declaration have none, under strict as under lax, and
// are not assessed (key-sva (§3.3.4.6) clause 2.2).
//
// cvc-attribute is also charged against the built-in declaration for the type
// attribute (§3.2.7.1), at the xsi:type attribute's own Loc, and that charge
// sits outside cvc-type clause 3's dispatch: the declaration governs the item
// whatever the element's ·governing type definition· is — simple, complex or
// undetermined — and cvc-complex-type clause 2 excepts xsi:type by name, so
// neither clause 2.1 nor clause 2.2 reaches it. Clause 3 reads its lexical
// through the same value.Backend against that declaration's xs:QName {type
// definition}, so an empty lexical, a colon structure no QName has, a part
// that is no NCName and a prefix with no binding in scope are each charged
// there. Clause 5 charges a lexical clause 3 accepts whose ·actual value·
// ·resolves· to no type definition, and needs no value space. Clause 3 is
// charged on the same terms against the built-in declaration for the nil
// attribute (§3.2.7.2), at the xsi:nil attribute's own Loc, for a lexical
// outside xs:boolean's four literals on an element with no ·governing element
// declaration· — ·laxly assessed·, or ·strictly assessed· against a type alone.
// Under a declaration the same lexical is cvc-elt clause 3's charge above, and
// is not charged twice. That lexical is read as cvc-elt clause 3 reads it, off
// the four literals and not through the value.Backend, so this is the one
// cvc-attribute clause 3 charge that wraps no String Valid verdict as its
// cause. Neither built-in declaration's charge reaches an undecided element —
// one whose governance this package could not decide, walked against nothing
// below a decline — since the true schema may ·skip· that element's attributes
// (key-governing-ad clause 3 and its Note).
//
// The seventh is the root's content half, against the same type's {content
// type}. cvc-complex-type clause 1 decides what its {variety} admits —
// no [[children]] at all for empty, no element ones for simple, no
// non-white-space character ones for element-only — and clause 1.4 sends
// the sequence of element information items to cvc-complex-content
// (§3.4.4.3) over xsd.Schema.ContentMatcher, which charges an item no
// particle admits at its position against that item's own Loc, and a
// sequence ending short of a {min occurs} against the root's. Where the
// {content type}'s {open content} is present, an item the {particle}
// cannot take goes to that record's {wildcard} instead and is charged
// only where the wildcard does not admit it either — clause 2 for {mode}
// suffix and clause 3 for interleave, which is the clause each charge
// names. Clause 1.2 additionally reads a VALUE, through the same backend the
// attribute charges use: a simple {content type} has the root's ·initial
// value· — every character information item [[child]] concatenated in order
// — validated against its {simple type definition} per String Valid, charged
// against the root's own Loc. Clause 5 asks each element [[child]]'s
// ·governing type definition· to be the same as, or ·validly substitutable·
// ·without limitation· for, its ·locally declared type· within the root's type
// (key-ldt-elem, xsd.Schema.LocallyDeclaredElementType), charged against the
// child's Loc; the clause's [[attributes]] half (key-ldt-att) is not evaluated.
//
// Everything not decidable is left undecided rather than guessed at: a
// ·governing type definition· that is not determinable, a {content type} whose
// shape xsd.Schema.ContentMatcher declines, a declaration whose {type
// definition} is not a simple type, a value whose ·validating type· String Valid
// clause 3 cannot decide, and — the decline that matters most — a
// value.ValidateLexical error that is a fault of the type or of the backend
// rather than a verdict about the lexical (value.IsDatatypeVerdict), which is
// what keeps a value of a type this backend does not map from being rejected by
// every document that carries one. The two ·special· datatypes are decided
// instead ([xsd.SimpleType.IsSpecial]), so a typeless attribute (§3.2.2.2) is
// satisfied. Which declines are recorded as [Unevaluated], and which are not, is
// [Unevaluated]'s own doc to say. An {attribute wildcard} carries an obligation
// that is this layer's ALONE: where the wildcard's {process contents} is strict
// or lax and it does not carry ##defined, §3.4.6.4 key-dft-binding case 3 binds
// an item ·attributed· to it to a SYNTHESIZED Attribute Use over the ·governing
// attribute declaration· its ·expanded name· ·resolves· to, which only an
// assessment episode can ·resolve· — so xsd's static c-ran rendering reports the
// keyword there by a ruling, not by an omission for this layer's carve to repair
// (#267). A skip {attribute wildcard} is outside that obligation as squarely as
// a ##defined one, and this layer owes it no case-3 rendering at all:
// key-governing-ad (§3.2.4.2) clause 3 resolves by name only "provided the
// attribute is not ·skipped·" and key-skipped makes such an item ·skipped·, so
// it has no ·governing attribute declaration· to bind and case 6's keyword is
// already the whole binding. cvcid.go's skippedAttribute encodes that reading
// for the attribute side, as the paragraph below does for the element side.
//
// Every one of those charges reaches a DESCENDANT on the same terms, against
// the ·governing type definition· the particle its parent's {content type}
// ·attributes· it to supplies (key-sva (§3.3.4.6) clause 3.1): an element
// particle's {term}, or — for a strict or lax ·wildcard particle· or {open
// content} {wildcard}, and for an item admitted as a member of a ·substitution
// group· — the top-level declaration its ·expanded name· ·resolves· to. An item
// ·attributed to· the {open content} whose ·locally declared type· is
// non-·absent· is the exception: key-governing-ed clause 4.3 gives it no
// declaration, and that type, or an xsi:type ·overriding· it, governs it,
// resolved name or not. Two shapes stop it: a child ·attributed to· a skip
// ·wildcard particle· or to an {open content} with a skip {wildcard}, which is
// ·skipped· along with every element beneath it (cvc-assess-elt clause 2,
// key-sva (§3.3.4.6) clause 3.2) — for the {open content} a reading and not a
// quotation, key-skipped naming a skip wildcard and never an Open Content,
// which the ·default binding· of cos-content-act-restrict clause 6 settles
// (#1969) — and an undecided child, whose governance this package could not
// decide at all: a child of a parent whose own type it could not determine,
// ·nilled· or not (#2211), or whose clause 1.4 xsd.Schema.ContentMatcher does
// not decide, which is assessed against nothing along with its whole subtree
// and withholds cvc-id clause 1 (#1892).
//
// A child its parent DECIDEDLY ·attributes· to nothing — a ·nilled· parent
// whose type this package determined, a simple-typed parent, an empty or
// simple {content type}, an item no particle or {open content} admits, and,
// by this package's reading rather than the spec's, any child after the
// parent's content is charged, the matcher not being advanced past a charge
// — is governed per key-governing-ed clause 4, which carries no attribution
// condition: by its ·locally declared type· within the parent's complex type
// where that is non-·absent· (clause 4.3; key-governing-type-elem clauses 6
// and 7), else by the declaration its name ·resolves· to, else by its
// xsi:type alone (key-governing-type-elem clause 8), else it is ·laxly
// assessed·. A child whose name ·resolves· to no declaration, and that
// exception does not reach, is likewise ·laxly assessed· against xs:anyType
// (key-lva, walk.child, #1823).
//
// The eighth is cvc-identity-constraint (§3.11.4), over the {identity-constraint
// definitions} of the ·governing element declaration· of every element the
// descent types. Its {selector} and {fields} are evaluated as the restricted
// path subset §3.11.6.2 and §3.11.6.3 define — the icpath package's grammar,
// directly and never through the XPath engine — and its clause 4 charges a
// duplicate ·key-sequence· (clauses 4.1 and 4.2.2), a key whose ·target node
// set· is wider than its ·qualified node set· (4.2.1), an element member from a
// {nillable} declaration (4.2.3), and a keyref matching no entry of its
// {referenced key}'s node table (4.3), with clause 3 charging a field that
// selects more than one node whose ·governing type definition· is a simple type
// definition or a complex type definition with {variety} simple, whatever their
// values, or any node whose ·governing type definition· is neither. Two members
// validated against different types, or against a union, are compared in the value
// spaces of their ·primitive· datatypes, a union-typed member and each item of a
// list of unions read by its own ·validating type· (§3.16.4). The node tables
// clause 4.3 reads are §3.11.5's, assembled bottom-up as the walk leaves each
// element and conflict-resolved on the way.
//
// The ninth is cvc-id (§3.3.4.5), charged at the ·validation root· alone
// (cvc-elt clause 7): the [ID/IDREF table] of §3.17.5.2 is assembled across the
// whole subtree from every attribute and element whose ·governing type
// definition· is ID, IDREF or IDREFS or is ·derived· or ·constructed· from one
// of them — a ·defaulted attribute· among them, at its ·effective value
// constraint·'s {lexical form} — each value classified by its own ·validating
// type· (§3.16.4) so that a union contributes what the member that validated it
// makes it and nothing for the rest; a binding with more than one member is
// charged clause 2, an empty one clause 1.
//
// Both of the last two decline rather than charge wherever this package could
// not read what the rule quantifies over — a path outside the subset, a field
// node or an ID-bearing item with no determinable ·governing type definition·,
// a ·key-sequence· member pair this package could not compare in one value
// space (a value with neither equality nor identity, or a type chain or
// ·validating type· that does not resolve), and a pair with a member of type
// xs:anySimpleType or xs:anyAtomicType, whose lexical names no one ·actual
// value·, save two such members with byte-identical lexicals, which are the
// same — and cvc-id clause 1 additionally declines for the whole document once
// any item of the subtree did, since an unread declaration is exactly what an
// empty binding would misreport. What a ·nilled· element is to each rule is
// walk.idElement's doc to say for cvc-id and icCheck.fill's for
// cvc-identity-constraint.
//
// The tenth is cvc-assertion (§3.13.4.1), cvc-complex-type clause 6, at every
// element whose ·governing type definition· is a complex type with
// {assertions}, once its [[children]] are exhausted: each member whose {test}
// xpath compiles when the element is entered — in the grammar
// xpath.CompileAssertionTest states, which owns it — over the element itself,
// whose name and in-scope namespaces xpath reads off an xpath.ContextElement,
// its attributes, carried or ·defaulted·, read TYPED by their {attribute uses}'
// types (as xs:untypedAtomic where that type is ·special·, and where cvc-attribute
// made a carried one invalid, which the partial ·PSVI· annotates xs:anySimpleType,
// xpath-datamodel §3.3.1.1), the element [[children]] a {test} names, read TYPED by
// their ·locally declared types· once each child's own assessment is over — as the
// string-value of the child's subtree, xs:untypedAtomic, where that type has mixed
// content (xpath-datamodel §6.2.4) — with the attributes of such a child, carried or
// ·defaulted·, a {test} reads through a range variable bound to it, read TYPED as an
// attribute below it is, its value read only where a {test} reads that too, the
// counts of the nodes of its subtree, reported to each counting {test}'s xpath.Tally
// as the walk passes them, each element by its chain of names below the counting
// element, a ·skipped· subtree's by name alone, with the typed values of the
// attributes below it a {test} reads through a path, and of the elements below it a
// {test} reads through a path of child steps, each once its own assessment is over,
// its own string value — its ·initial value· under simple content, and the text of
// its subtree under any other {content type} — and its simple content's ·actual
// value· — is evaluated over those values and charged where it is false or raises a
// dynamic or type error. A member xpath declines, and every member of an element one
// of whose use-matched or ·defaulted· attributes has no ·actual value·, or, carried,
// a [validity] this package did not decide or, made invalid by cvc-attribute clause
// 4, a [schema normalized value] this package does not read, one of whose read
// children is not known ·valid· or not ·validly substitutable· for its ·locally
// declared type·, or, read for its string-value, has no mixed content of its own, a
// string-value of which — a read child's, or its own a {test} reads — spans a
// ·skipped· element, one of simple content whose raw characters are not its [schema
// normalized value] or that holds a run of white space alone, one of a type not
// determined, a mixed one that took its {value constraint} default, a ·nilled· one
// with character [[children]], or a run that is not white space alone in one of
// element-only or empty content, whose counted subtree holds an element whose
// ·governing type definition· was not determined and whose attribute nodes a {test}
// counts or reads, or an attribute, or an element, a {test} reads through a path, or
// an attribute of a read child a {test} reads through a range variable, that has no
// typed value under the type it was compiled to read — an attribute cvc-attribute did
// not settle ·valid· among them — whose simple content's ·actual value· is undecided,
// or whose string value was not gathered because it has simple content and element
// [[children]] or is ·nilled· and has any [[children]] under simple content or a
// {test} reading `.`, are recorded as [Unevaluated] under cvc-assertion instead
// (cvcassertion.go).
//
// The rest of the cvc- decisions land on the walk [Validator.Assess]
// already makes. Non-fatal warnings get an accessor of their own the day
// something produces one.
package validate
