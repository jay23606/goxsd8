package conformance

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/validate"
	"github.com/kud360/goxsd8/validate/xmlsrc"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsderr"
)

// This file activates the instance lane (issue #713) by giving the instance
// entry of defaultLanes a real executor, on the precedent schema.go set for the
// schema lane (#175). It touches nothing else in the runner (the #6 seam, STYLE
// T2): the lane's selector stays selectsKind(kindInstance), so the executor is
// handed EVERY instanceTest case and either decides it or honestly DECLINES it
// (records a Fail gap) — a case it cannot decide for the right reason never
// flips to pass. It is package-internal conformance support: it exports nothing
// and no library code imports it.
//
// # What an instanceTest asserts, and against which schema
//
// An instanceTest asks: is THIS instance document valid against the schema of
// its test group? The catalog names only the instance document, so discovery
// carries the group's schema documents alongside it (caseSpec.schemaDoc and
// schemaExtraDocs, groupSchemaDocs in conformance/runner.go) — the
// <schemaDocument> list of the group's sibling schemaTest. A group declaring any
// number of schemaTests other than exactly one yields no schema reference at
// all, which is not a hypothetical shape: 55 groups of the pinned suite declare
// NONE. Such a case is assessed against the schema its xsi:schemaLocation /
// xsi:noNamespaceSchemaLocation hints locate, read off every element in document
// order with the first hint for a namespace winning (caseSchema,
// instancehints.go, #2013, #2171, §4.3.2 clauses 3-5), together with every
// inline xs:schema it carries, each read as a schema document (#2180). One
// carrying neither is assessed against the built-in components alone where its
// root's xsi:type names a built-in type definition (key-governing-type-elem
// clause 8), and DECLINES otherwise (builtinsSchema, #2151).
//
// The group's schema is assembled by assembleCase, the very gate the schema lane
// decides its own cases with, widened by the root's hints for every namespace
// that assembly's documents leave uncovered (groupSchema, instancehints.go,
// #771), and this lane declines wherever that gate does — plus one condition the
// schema lane does not have: it declines a schema document set the assembly
// REJECTED. A schema the parser found schema-invalid is not the schema the suite
// declared, so an assessment against whatever partial components survived would
// decide a different question. (The suite's own metadata is not consulted for
// this: the assembly's verdict is the fact, and a group whose schemaTest
// declares a non-valid expectation therefore declines through the same test as
// any other failed assembly.) One rejection is decided "not valid" rather than
// declined: a schema document the assembly retrieved and the reader charged not
// well-formed (wellFormednessFault), a harness convention execInstanceCase
// states (#2058).
//
// # The only outcomes this slice can DECIDE
//
// validate.Validator.Assess charges exactly ten rules, about the ·validation
// root· and about any descendant the descent reaches (see "Charges at depth"
// below):
//
//  1. cvc-assess-elt (§3.3.4.6), when the root has no top-level element
//     declaration AND no xsi:type ·resolving· to a top-level type definition
//     (#716). It then determines neither a ·governing element declaration· nor
//     a ·governing type definition·, so key-sva clause 1 fails and §3.3.5.1's
//     e-validity gives it notKnown rather than valid — which under §5.2 ·strict
//     wildcard validation·, the mode this processor committed to in #712, is
//     what "the invoking process ... will otherwise report an error to its
//     environment" names. The document is NOT VALID, whatever else it holds.
//  2. cvc-elt (§3.3.4.3) clause 2, when the root's declaration was found and its
//     {abstract} is true; clause 3, when it carries an xsi:nil its {nillable}
//     forbids (3.1), an xsi:nil with no ·actual value· (3.2), or is ·nilled· and
//     carries [[children]] (3.2.3.1) or a fixed {value constraint} (3.2.3.2);
//     clause 4, when its xsi:type ·resolves· to a type definition that does not
//     ·override· the ·selected type definition·; and clause 5.2.2, when its
//     declaration's fixed {value constraint} disagrees with the [[children]] it
//     has (#716). The root is ·strictly assessed· and locally INVALID, so
//     e-validity clause 1.1.1.1 fails and its [validity] is invalid. The
//     document is NOT VALID, whatever else it holds.
//  3. cvc-complex-type (§3.4.4.2) clause 1, clause 2, clause 3 or clause 4, when
//     the root's ·governing type definition· was determinable and complex and
//     one of its [[attributes]] matches neither an attribute use nor an
//     {attribute wildcard} (#714), a {required} use has no attribute at all
//     (#714), a ·defaulted attribute·'s own {lexical form} is not datatype-valid
//     (#766), its [[children]] hold a character or element information item the
//     {content type}.{variety} admits none of — clauses 1.1, 1.2 and 1.3
//     (#715) — or its ·initial value· under a simple {content type} is not
//     ·valid· per String Valid against that {content type}'s {simple type
//     definition}, the other half of clause 1.2 (#775). The root is then not
//     locally ·valid· with respect to that type, so cvc-type clause 3.2 fails, so
//     cvc-elt clause 5 fails, and e-validity clause 1.1.1.1 gives the root
//     [validity] invalid exactly as case 2 does.
//  4. cvc-attribute (§3.2.4.1), against an attribute of the root that has a
//     ·governing attribute declaration·: clause 3, when its lexical is not
//     ·valid· per String Valid (§3.16.4) against the declaration's {type
//     definition}, and clause 4, when its ·actual value· disagrees with a fixed
//     {value constraint} on that declaration (#766). That declaration is the one
//     a matched attribute use carries; the top-level one a name ·attributed to·
//     a strict or lax {attribute wildcard} ·resolves· to (cvc-complex-type clause
//     2.2, key-governing-ad clause 3), clause 3 and clause 4 both (#1891); the
//     built-in one for xsi:type (§3.2.7.1), clause 3 when its lexical is no
//     xs:QName and clause 5 when the QName ·resolves· to no type definition; and
//     the built-in one for xsi:nil (§3.2.7.2), clause 3 when its lexical is no
//     xs:boolean, on an element with no ·governing element declaration· — under
//     one, cvc-elt clause 3 (case 2) charges that lexical instead (#2061). A
//     charged attribute's [validity] is invalid, and e-validity's conjunction
//     (§3.3.5.1 clause 1.1.1.2) fails for the root on an invalid attribute of
//     its own whatever else holds. None of them is charged against an element
//     validate leaves undecided — one whose governance it could not decide,
//     whose attributes the true schema may ·skip· — which validate reaches only
//     beside a decline already in the Result (#2159, #1892).
//  5. cvc-au (§3.5.4), when a matched attribute's ·actual value· disagrees with
//     a fixed {value constraint} on the attribute USE — a different property
//     from case 4's, which is why both can be charged for one attribute (#766).
//     cvc-complex-type clause 2.1 reads it, so the root is not locally ·valid·
//     and case 3's chain applies unchanged.
//  6. cvc-complex-content (§3.4.4.3) clause 1, when the root's ·governing type
//     definition· was determinable and complex, its {content type} holds a
//     particle, and the sequence of element information items in its
//     [[children]] is not ·accepted· by that particle — an item no particle
//     admits at its position, or a sequence that ends before a {min occurs} is
//     met (#715). cvc-complex-type clause 1.4 reads it, so case 3's chain
//     applies unchanged.
//  7. cvc-identity-constraint (§3.11.4), when a key, unique or keyref declared
//     on an element the descent typed is not satisfied over that element's
//     subtree — a field selecting more than one valued node (clause 3), two
//     ·qualified node set· members sharing a ·key-sequence· (clauses 4.1 and
//     4.2.2), a key whose ·target node set· is wider than its ·qualified node
//     set· (clause 4.2.1), a key-sequence element member from a {nillable}
//     declaration (clause 4.2.3), or a keyref matching no entry of the node
//     table its {referenced key} has in that element's own [identity-constraint
//     table] (clause 4.3, #718). cvc-elt clause 6 reads it, so case 3's chain
//     applies unchanged.
//  8. cvc-id (§3.3.4.5), when the ·validation root·'s [ID/IDREF table] holds a
//     binding with more than one member (clause 2, a multiply-defined ID) or —
//     only where no item of the subtree was declined — an empty one (clause 1,
//     a reference to an undefined ID) (#718). cvc-elt clause 7 reads it AT THE
//     ROOT ALONE, and it makes the root not locally ·valid· exactly as case 2
//     does.
//  9. cvc-type (§3.3.4.4) clause 2, when an element's ·governing type
//     definition· is a Complex Type Definition whose {abstract} is true
//     (#2095); clause 3.1, when it is a Simple Type Definition and the element
//     carries an attribute beyond the four xsi: names (3.1.1), an element
//     information item [[child]] (3.1.2), or — where it is not ·nilled· — an
//     ·initial value· that is not ·valid· per String Valid against that type
//     (3.1.3) (#913). The element is then not locally ·valid· with respect to
//     its ·governing type definition·, so case 3's chain applies unchanged.
//  10. cvc-assertion (§3.13.4.1), when an assertion in the {assertions} of an
//     element's complex ·governing type definition· has a {test} validate's
//     XPath evaluator compiles and that {test} is false or raises a dynamic or
//     type error, which the rule treats alike (#2232). cvc-complex-type clause
//     6 reads it, so case 3's chain applies unchanged. An assertion whose
//     {test} the evaluator declines is recorded, never charged. A simple
//     type's assertions facet is evaluated as well, and a false {test} is
//     charged — but folded into Datatype Valid (cvc-datatype-valid clause 3)
//     and so under cvc-complex-type, cvc-attribute or cvc-type, cases 3, 4
//     and 9, never as a top-level cvc-assertions-valid (#2246).
//
// All ten are unconditional: no verdict here can be overturned by anything in
// the rest of the document, which is what makes them decidable while the engine
// leaves most of the document undecided. They are every "not valid" this lane
// observes of an assessment, the not-well-formed schema document above being
// the one "not valid" it records without one; its "valid" is the one gated
// shape that charges none of them ("Why an EMPTY Result is evidence of validity
// for ONE shape only" below).
//
// # Charges at depth
//
// Cases 2 to 7, case 9 and case 10 are charged against a DESCENDANT on the same
// terms as against the root (#790, #913), and stay unconditional there. The lane's
// "valid" is §2.5's key-deep-valid-doc (#1911): the root's [validity] is valid
// AND no element or attribute anywhere in the document has [validity] invalid
// (clauses 3 and 4). key-sva (§3.3.4.6) clause 3.1 has a child assessed with
// respect to its ·governing element declaration· — the one the parent's content
// model ·attributed· it to, or, under a lax wildcard or a ·laxly assessed·
// parent, the top-level one its name ·resolves· to (#1823) — so a child
// validate charges is one it ·strictly assessed· against a declaration it
// really has, and its [validity] is invalid (§3.3.5.1 e-validity clause 1.1.1);
// an attribute validate charges has a ·governing attribute declaration·, on a
// ·laxly assessed· element too (#1891), and its [validity] is invalid likewise.
// Either makes the document not deep-valid, whatever the unassessed rest of it
// holds. Clause 1.1.2 makes a ·strictly assessed· ancestor with an invalid
// [[child]] or [[attribute]] invalid in turn, but stops at the first ·laxly
// assessed· one (cvc-assess-elt clause 3, key-lva): its [validity] is notKnown
// (e-validity clause 2), never invalid (§2.5's Note), so the root may stay
// valid and the document root-valid. decidedNotValid reads a charge at or below
// a ·laxly assessed· element as "not valid" all the same, which is deep-valid's
// reading and not root-valid's.
//
// Every {type table} a declaration carries IS built (#851), so validate never
// guesses a tableless declaration's type: it ·conditionally selects· through the
// table, and withholds the element's ·governing type definition· only where the
// §3.12.6 required-subset evaluator declines one of the {test}s it had to try
// (validate/cta.go).
//
// Cases 3 to 6 and case 9 rest on conditions validate checks rather than this
// file assuming them: the attribute half of cvc-complex-type is reached only where
// the governing type was determinable — the ·selected type definition·, which
// a {type table} may have ·conditionally selected·, or the ·instance-specified·
// one that ·overrides· it — and only for attributes clause 2 quantifies over
// (the four xsi: names are excepted). The content half adds its own: an element
// that is ·nilled· (clause 1 applies only where it is not, and cvc-elt clause
// 3.2.3.1 decides its [[children]] instead), and a {content type} whose shape
// xsd.Schema.ContentMatcher declines — a nested repetition whose occurrence
// ranges admit more partitions than the walk carries cursors, an all group
// holding an all group — each of which withholds clause 1.4 entirely rather than
// matching part of a sequence. The value charges add their own: a declaration whose
// {type definition} does not resolve to a simple type, and — the one that would
// otherwise reject every value of a type the backend does not map — a
// value.ValidateLexical error that is a fault of the type or of the backend rather than
// a verdict about the lexical (value.IsDatatypeVerdict). Each of those is a DECLINE
// inside validate, and a declined attribute charges nothing at all, so it cannot arrive
// here.
//
// Case 2 fires through an ASSEMBLED schema: producer.produceElement maps
// {abstract} from the top-level <element>'s abstract attribute (§3.3.2.1
// dcl.elt.common, #761), so a document declaring an abstract root reaches the
// cvc-elt charge here. Case 2 and case 3 can be charged for ONE root together —
// the cvc-elt branch keeps walking after charging, so an abstract root whose
// governing type is determinable is assessed for its attributes too — which is
// among the reasons decidedNotValid pins no violation count.
//
// # Why an EMPTY Result is evidence of validity for ONE shape only
//
// In general an empty Result — no violation charged — is UNDECIDABLE IN BOTH
// DIRECTIONS and DECLINES. e-validity is a conjunction: local validity, AND no
// [[children]] or [[attributes]] whose [validity] is invalid, AND none
// attributed to a strict ·wildcard particle· and left notKnown. Being
// ·strictly assessed· at all (key-sva, §3.3.4.6) is itself a three-clause
// definition whose clauses 2 and 3 dispatch assessment recursively into every
// attribute and child, which Assess follows only as far as it can type: where a
// descendant's declaration or type is not determinable — a withheld {type
// table} selection, an unresolvable {type definition}, a ·skipped· subtree —
// the element and everything below it is decided against nothing, and one of
// validate's declines records nothing in Result.Unevaluated
// (validate.Unevaluated's own doc names it). The spec has no category for
// "this processor did not implement that check" stronger than notKnown, so
// outside the shape below an empty Result licenses no "valid" claim; equally it
// licenses no "invalid" one, so an expected-invalid case declines exactly as an
// expected-valid one does.
//
// The one shape it DOES license "valid" for is an ASSESSED SUBTREE ROOT
// (assessedSubtreeRoot, subtreeroot.go, #1841), with content or without
// (#1855), and only where the walk recorded nothing in Result.Unevaluated.
// key-sva (§3.3.4.6) clauses 2 and 3 dispatch assessment into every attribute
// and child, and sic-e-outcome (§3.3.5.1) clause 1.1 recurses through them, so
// the gate holds EVERY element of the subtree to its conditions, not the root
// alone. It re-reads the instance and re-derives each child's ·attribution·
// with xsd.Schema.ContentMatcher, independently of the walk, and discharges at
// every element each clause the walk does not record deciding:
//
//   - key-sva clause 1 and cvc-elt clause 1: the root's declaration is the
//     top-level one its name resolves to, and every other element's is the
//     one the walk's childGoverning takes too: the {term} of the element
//     particle its parent's {content type} ·attributes· it to, carrying the
//     element's own name — the ·context-determined declaration· (§3.3.4.6
//     key-governing-ed clause 2). A ·substitution group· member cvc-accept
//     clause 2.3.2 attributes to an element particle of another name takes the
//     top-level declaration its name ·resolves· to, which the Matcher held
//     ·substitutable· for the particle's {term} (cos-equiv-derived-ok-rec, whose
//     clause 2.3 counts a simple restriction step as restriction, #1942). A child
//     ·attributed to· a strict or lax Wildcard or {open content} takes
//     the top-level declaration its name ·resolves· to (clauses 3 and 4),
//     cvc-complex-type clause 5 being the walk's for it
//     (subtreeGate.resolvedChild). An {open content}'s child whose ·locally
//     declared type· is non-·absent· has no declaration, resolved name or not
//     (clause 4.3): that type governs it, or an xsi:type ·overriding· it
//     (key-governing-type-elem clauses 6 and 7), and the gate holds it and its
//     subtree to every condition below against that type, never ·nilled·, as
//     the walk's localGovernance assesses them (subtreeGate.localTyped, #2080).
//     A strict ·wildcard particle·'s child resolving to none, with no
//     xsi:type, is admitted with its subtree unread: the walk charges e-validity
//     clause 1.1.3 for its parent, so no empty Result carries it. A skip
//     Wildcard's child is admitted with its subtree unread: it is ·skipped·
//     (key-sva clause 3.2, cvc-assess-elt clause 2) and has no [validity] to
//     block its parent's (sic-e-outcome clause 1.1). So is a skip {open
//     content}'s child, which validate reads as ·skipped· too (#1969). A lax
//     Wildcard's or {open content}'s child resolving to none, with no xsi:type,
//     is ·laxly assessed· (cvc-assess-elt clause 3, key-lva): the gate holds it
//     and its subtree to every condition below against ·xs:anyType·, as the walk
//     assesses them, and it is never ·nilled·, key-nilled being relative to a
//     declaration it lacks (subtreeGate.laxlyAssessed, #1823, #1891). Its
//     [validity] is notKnown, which blocks no ancestor's valid (e-validity clause
//     1.1.3 names a strict ·wildcard particle· alone) and leaves the document
//     deep-valid (§2.5 Note, #1911). A child resolving to none whose xsi:type
//     names a type definition, under strict and lax alike, has that type as its
//     ·governing type definition· and is ·strictly assessed· against it
//     (cvc-assess-elt clause 1), as the walk assesses it: the gate holds it and
//     its subtree to every condition below against that type, never ·nilled·,
//     there being no declaration (subtreeGate.instanceTyped, #1978). That type
//     governs by key-governing-type-elem clause 8 where the child's ·locally
//     declared type· is ·absent·, and by clause 6 where a Wildcard's child has a
//     non-·absent· one: with no declaration, ·overriding· it is cvc-complex-type
//     clause 5's condition, which the walk charges, so an empty Result implies
//     clause 6 holds. One whose xsi:type names none is refused. A root
//     resolving to none whose xsi:type names a type definition has it as its
//     ·governing type definition· by clause 8 and is held to the same
//     conditions against it, never ·nilled· (subtreeGate.undeclaredRoot,
//     #2156); a root resolving to none with no such xsi:type is refused. No
//     element of a subtree whose Result is empty is therefore assessed against
//     no type.
//   - cvc-elt clauses 2 to 6, at every element: {abstract} false, and a
//     ·selected type definition· the gate determines itself (§3.3.4.1
//     key-selected-type): the {type definition}, or the type a {type table}
//     ·conditionally selects· (key-cta-select), evaluating each {test} it
//     reaches as the walk does, over the element's [[attributes]] and its
//     [inherited attributes] (§3.3.5.6), and refusing where a {test} it reaches
//     is one the evaluator declines (subtreeGate.selectedType). The walk records
//     that decline in Result.Unevaluated, so no empty Result reaches the gate
//     with it. A ·nilled· element ·xs:error· governs is refused too: the walk
//     charges cvc-type clause 3.1.3 for one that is not ·nilled· alone
//     (§3.16.7.3, key-error). An xsi:type is admitted where the gate resolves it
//     and xsd.Schema.ValidlySubstitutable answers that it ·overrides· the
//     selected type (clause 4, §3.3.4.2 key-overrides), which is the walk's
//     decision too; the gate then follows that type as the ·governing type
//     definition· (key-governing-type-elem clause 3) through every condition
//     below. It refuses an error from that predicate
//     (subtreeGate.governingType), which validate's instanceOverride records
//     in Result.Unevaluated. {identity-constraint definitions} are
//     admitted, at every depth: clause 6 (cvc-identity-constraint, §3.11.4) is
//     the walk's, which reads a ·defaulted attribute· field node as it reads a
//     present one (validate's icCheck.fieldDefaultedAttributes) and records each
//     check it declines in Result.Unevaluated. {nillable} and xsi:nil are
//     admitted, at every depth (#2053): clause 3 is the walk's — validate's
//     nilCheck charges 3.1, an xsi:nil on a declaration whose {nillable} is
//     false, 3.2, one with no ·actual value·, and 3.2.3.2, a ·nilled· element
//     under a fixed {value constraint}, and its contentCheck charges 3.2.3.1, a
//     ·nilled· element's character or element [[child]]. An xsi:nil false under
//     a {nillable} declaration is 3.2.2, read as if absent. The gate refuses an
//     xsi:nil with no ·actual value· on a declared element itself
//     (subtreeGate's nilValue), behind the walk's 3.1 or 3.2 charge for it. A
//     {value constraint} of either variety is admitted, at every
//     depth: clause 5.1 substitutes its {lexical form} for the ·normalized
//     value· of an element with neither element nor character [[children]], and
//     the walk assesses cvc-type over that substituted value and settles 5.1.1
//     (validate's contentCheck.assessed and contentCheck.defaultValid); over an
//     element that has [[children]], a fixed one is clause 5.2.2's, which the
//     walk settles too — 5.2.2.1, no element [[children]], and 5.2.2.2, the
//     ·initial value· agreeing with it (validate's contentCheck.fixedValue) —
//     recording in Result.Unevaluated the one comparison value.ConstraintMatches
//     does not decide; clause 5 is gated on an element that is not ·nilled·, as
//     the walk reads it. Clause 2 is the walk's, at every element: it charges
//     an element whose ·governing element declaration· has {abstract} true
//     (validate's walk.abstractDeclaration), so no empty Result carries one.
//   - cvc-type clause 2 (§3.3.4.4): the walk's, at every element. It charges
//     an element whose ·governing type definition· is a complex type with
//     {abstract} true, however that type was determined (validate's
//     walk.abstractType), so no empty Result carries one.
//   - cvc-type clause 3.1 (§3.3.4.4), for a Simple Type Definition: 3.1.1, the
//     element carries no attribute beyond namespace declarations and the four
//     xsi: names, and 3.1.2, it has no element [[children]] — the gate refuses
//     both, a second check on the walk's charges. 3.1.3 — the ·initial value·,
//     or clause 5.1's substitute, String Valid against the type — is the
//     walk's, the empty string of a content-less element included; its one
//     decline (validate's contentCheck.simpleTypeValue, String Valid withheld)
//     is RECORDED in Result.Unevaluated, under cvc-assertions-valid where an
//     assertions facet of the type's closure has a {test} validate's facet
//     evaluator declines; one it evaluates false is part of the 3.1.3 verdict.
//   - cvc-elt clause 7 (cvc-id, §3.3.4.5): the walk's, at every depth. It
//     reads each element's and attribute's item into the [ID/IDREF table]
//     (validate's walk.idAttributes, walk.idDefaultedAttributes and
//     walk.idElement, through walk.idRecord) and charges both clauses once, at
//     the root (idTable.charge). Every item it cannot read is recorded through
//     walk.declineID except at two sites that set ids.declined and record
//     nothing, neither of which arises under this gate:
//     walk.idDefaultedAttributes' unresolved {attribute declaration}
//     ("Unreachable on a *xsd.Schema that exists"), which the gate refuses
//     besides, since each use's declaration must resolve (refuseAttributeUse);
//     and walk.child's undecided child, which arises only below a parent whose
//     {content type} xsd.Schema.ContentMatcher declines, which the gate
//     refuses (refuseContentMatcher), or below a parent whose ·governing type
//     definition· is undetermined, which the gate refuses too, admitting no
//     element whose type it cannot determine — or, below a ·laxly assessed·
//     parent, the walk attributes the child to ·xs:anyType·'s lax wildcard
//     ahead of that site, as the gate's matcher over ·xs:anyType· does. A
//     third site, walk.localGovernance's undecided exit, sets ids.declined
//     beside its own walk.decline record and is unreachable for a finalized
//     Schema, so it is no site that records nothing. Keep refuseContentMatcher
//     and refuseAttributeUse when editing the gate: the gate's reading of
//     cvc-id clause 1 rests on them. String Valid clause 3's ·declared entity
//     name· check (key-vde) is the walk's too, decided per ENTITY value by
//     walk.entitiesDeclared and recorded as an [Unevaluated] by its callers
//     where undecided (walk.declineAttribute, walk.declineDefaulted,
//     contentCheck.decline). So is String Valid clause 2's NOTATION half,
//     NOTATION's ·value space· being "the set of QNames of notations declared
//     in the current schema" (Datatypes §3.3.19): value.ValidateLexical,
//     handed the schema, rejects a value naming no declared notation under
//     cvc-datatype-valid, at every depth; a value outside a NOTATION type's
//     enumeration is the backend's cvc-enumeration-valid verdict (Datatypes
//     §4.3.5.4), which Datatype Valid entails, a ·defaulted attribute·'s
//     {lexical form} included (walk.defaultedAttribute). The gate therefore
//     reads no simple type's closure: finalize's src-resolve pass (xsd's
//     resolveSimpleType) resolves every {base type definition}, {item type
//     definition} and {member type definitions} reference of every simple type
//     a Schema holds, so no closure the walk reads is unreadable, and a String
//     Valid the backend withholds is recorded by the callers above. Each use's
//     declaration must resolve to a simple type (recordedAttributeType), so an
//     unresolvable {attribute declaration} is refused.
//   - cvc-complex-type clause 2: every attribute beyond namespace declarations
//     and the four xsi: names matches an attribute use (2.1) or is
//     ·attributed to· the {attribute wildcard} (2.2, cvc-wildcard §3.10.4.1)
//     (#1860). Under skip it is ·skipped· and not assessed (key-sva clause
//     2.2); under lax or strict a name that ·resolves· to a top-level
//     declaration is assessed against it (key-sva clause 2.1), and one that
//     resolves to none under lax is not assessed. That declaration's {type
//     definition} must resolve to a simple type, as a use's must. The gate
//     refuses a strict wildcard's name that resolves to none, which the walk
//     charges nothing for and records nothing of (#1912), and a resolved name
//     whose ·locally declared type· is not ·absent·, for clause 5 below
//     (subtreeGate.wildcardAttribute). Clause 2.1's cvc-attribute and cvc-au,
//     clause 2.2's cvc-attribute clauses 3 and 4 against the resolved declaration,
//     and clauses 3 and 4 — a {required} use the element lacks, and a ·defaulted
//     attribute· for each use it does not carry — are the walk's, which records
//     each check it withholds, on an element with no [[attributes]] as on any
//     other. A wildcard-resolved declaration supplies no ·defaulted attribute·
//     (key-dflt-att ranges over {attribute uses}).
//   - key-sva clause 2 for the xsi: attributes the gate admits: xsi:nil is
//     ·valid· against its built-in declaration's xs:boolean (§3.2.7) wherever
//     it has an ·actual value·, and one without is the walk's (cvc-elt clause
//     3.1 or 3.2 under a declaration, cvc-attribute clause 3 on an element with
//     none, key-governing-ad); xsi:type is the walk's (cvc-attribute clauses 3 and 5,
//     charged or recorded wherever it is no QName or ·resolves· to no type).
//     Neither cvc-attribute charge reaches an element the walk leaves
//     undecided (case 4 above), and none needs to: such an element arrives
//     only beside a decline, so no empty Result carries one; and xsi:schemaLocation
//     and xsi:noNamespaceSchemaLocation are each ·valid· against their built-in
//     declaration's anyURI or list-of-anyURI type (§3.2.7), whose lexical spaces
//     admit every string (Datatypes §3.3.17).
//   - cvc-complex-type clause 5: the walk's for every element [[child]],
//     which charges one whose ·governing type definition· is neither the same
//     as nor ·validly substitutable· ·without limitation· for its non-·absent·
//     ·locally declared type· (key-ldt-elem, walking the {base type
//     definition} chain, ·implicitly contained· ·substitution group· members
//     included: validate's walk.locallyDeclaredType over
//     xsd.Schema.LocallyDeclaredElementType) and records the comparison it
//     cannot settle. The attribute half the walk never checks, and it is
//     vacuous for every attribute the gate admits: an attribute matching a use
//     has that use's {attribute declaration}'s {type definition} for both
//     (key-ldt-att case 2, clause 2.1), and one resolved through the
//     {attribute wildcard} is admitted only where its ·locally declared type·
//     is ·absent· (key-ldt-att, walking the {base type definition} chain:
//     subtreeGate.locallyDeclaredAttribute). e-validity clause 1.1.3 is the
//     walk's, charged for a strict ·wildcard particle·'s child resolving to
//     none (validate's walk.unresolvedStrictWildcardChild).
//   - cvc-complex-type clause 6: the walk's, at every element. validate's
//     elementAssertions charges cvc-assertion for a member of a governing
//     type's {assertions} whose {test} it evaluates and finds not true (case
//     10), and records an Unevaluated for every member it does not evaluate, so
//     an empty Result shows every member evaluated and satisfied.
//   - cvc-complex-type clause 1 (§3.4.4.2): the walk's, for an element with no
//     [[children]] as for any other. An empty {content type} meets clause 1.1
//     wherever the walk charged no character or element [[child]]; a simple one
//     is clause 1.2's String Valid, decided and recorded as cvc-type clause
//     3.1.3 is above; an element-only or mixed one is clause 1.4
//     (cvc-complex-content), whose empty sequence is ·valid· only where the
//     particle is emptiable (cvc-particle, "possibly empty"). The gate refuses
//     a {content type} ContentMatcher declines and a child sequence it rejects
//     or does not accept, a second check on the walk's record or charge. Clause 1
//     applies only to an element that is not ·nilled·: a ·nilled· one's
//     [[children]] are cvc-elt clause 3.2.3.1's, which the walk charges, and
//     the gate reads them without its ContentMatcher, refusing an element
//     [[child]] (subtreeGate.complex).
//
// Every element but a ·laxly assessed· one is then ·strictly assessed· against
// a type the walk determined, through a declaration or, for a wildcard's child
// resolving none, through its xsi:type, and its [validity] is valid exactly
// where the walk charged nothing for it, its attributes or its descendants.
//
// The TRUST BOUNDARY is value.ValidateLexical: its verdict on every ·initial
// value· and every attribute value in the subtree is taken as Datatype Valid
// (Datatypes §4.1.4). The datatypes lane is what grounds that verdict; this
// lane does not re-check it. A value of xs:anySimpleType or xs:anyAtomicType
// never reaches it: Datatype Valid holds for every literal against a ·special·
// datatype, and validate decides so itself (#1788).
//
// Two declines sit outside the clauses altogether. The gate declines any
// assembly one of whose documents carries an attribute in the versioning
// namespace, which no hazard now backs: closureVersioned's GAP(conformance)
// marker owns that decline. It declines a DOCTYPE too whose DTD could default
// an attribute the gate does not see: one with an external subset, an
// <!ATTLIST or a parameter-entity reference (rootStart).
//
// The one shape that looks like case 1 and is not: a root with no top-level
// declaration whose xsi:type ·resolves·. Its ·instance-specified type
// definition· is its ·governing type definition· (key-governing-type-elem clause
// 8), so it is ·strictly assessed· against that type, cvc-assess-elt is not
// charged, and it lands in the undecidable bucket above on the same terms as any
// other root that charges nothing.
//
// # Why no false pass is possible
//
// Every "not valid" observation this lane emits comes from one of the ten
// charges above, each of which is unconditional, or from a schema document the
// reader charged not well-formed, recorded before any assessment under
// execInstanceCase's harness convention. Its "valid" observation
// is an empty Result — no violation, no unevaluated record — on the gated
// shape, whose every applicable clause the section above names the decider of;
// every other empty Result declines. So the lane can record a still-failing
// gap for a suite-invalid case it cannot see the defect in, and for a
// suite-valid case outside that shape, but it cannot score a pass on a
// document it did not really reject, nor on one it did not really decide valid
// — at the root or at any depth, the charges being the same ten either way.
// The bound on the valid side is exactly as wide as the gate is correct and
// ValidateLexical is right: a clause the gate should have excluded and did
// not, or a Datatype Valid verdict the backend gets wrong, is where a false
// pass could come from.
//
// Case 3's ATTRIBUTE clauses are the ones whose unconditionality depends on a
// schema COMPONENT being complete rather than on the instance alone: an
// under-reported {attribute uses} or {attribute wildcard} would make a valid
// attribute look unmatched. Both properties are the spec's for every governing
// type validate can reach, an ANONYMOUS one included: §3.4.2.4 clause 3 and
// §3.4.2.5 clause 2 fold a declaration-owned type through the slot that owns it
// (xsd/ownedtypefold.go, #414), which is why validate reads them with no
// anonymity test at all. The completeness has to hold in the LIBRARY
// rather than here, because assembleCase's decidability gate bounds what THIS
// lane assembles and says nothing about the library's other callers.
//
// Cases 7 and 8 carry their own, and validate declines rather than charging at
// every one: an identity constraint whose {selector} or {fields} fall outside
// the §3.11.6.2/§3.11.6.3 path subset, a field node whose ·governing type
// definition· was not determinable, a ·key-sequence· member pair with no
// ·primitive· value space to compare in, and — for cvc-id clause 1 alone — an
// [ID/IDREF table] any item of the subtree was declined for. Each is a DECLINE
// inside validate, so it cannot arrive here.
//
// Case 3's clause 1 and case 6 do not share that dependency, and are not
// declined with it: no finalize pass folds a {content type} at all, so a complex
// type's particle is whatever its producer built for it whether the type is
// named or anonymous.
//
// The cvc-assess-elt charge does carry one hazard of its own: a root is equally
// undeclared when an <import>/<include> the assembly did not follow took the
// declaring components with it, which is not the defect the suite meant to test.
// assembleCase's fabricatedRejection bounds that for the SCHEMA lane, where the
// fabricated verdict shows up as a failed parse, and does not transfer here,
// where the parse succeeds and the charge lands anyway. The same holds of a
// component only the instance's own hints locate: groupSchema adds a root hint's
// document to the group's schema where no document of the group's closure has
// the hint's namespace, and withholds every other hint — one for a covered
// namespace, every hint below the root, every hint of an instance
// instanceHints refuses or that carries an inline xs:schema (uncoveredHints),
// and every hint where the widened assembly declines or errs — so an element
// only such a hint declares is still charged as undeclared. GAP(conformance):
// each such hint is withheld — one for a covered namespace, one below the root,
// every hint of a refused instance or of one carrying an inline xs:schema,
// every hint where the widened assembly declines or errs, and an
// xsi:noNamespaceSchemaLocation hint withheld because a chameleon-included
// document counts "" as covered (closureNamespaces). RULED permanent by #771
// (STYLE P3b): §4.3.2 clause 3 makes every hint optional, and sch-props-correct
// clause 2 with §4.2.6.2's Note permits keeping the components already held for
// a namespace. A schema assembled from the instance's hints alone, for a group
// with no schemaTest, never reaches this charge at the root: assembleHints
// declines a hinted schema declaring no top-level element for it. Nor does the
// built-ins schema: builtinsSchema hands it only to a root whose xsi:type
// resolves in it. A root undeclared for that reason whose xsi:type resolves is
// not charged at all: it is ·strictly assessed· against that type
// (assessedSubtreeRoot, #2156), and a "valid" the missing declaration's
// constraints would have overturned is the other direction, which can only cost
// wins: the lane observes "valid" where the suite says "invalid".
//
// Three further declines close the ways a NON-verdict could reach that
// comparison. An instance document that will not resolve or read is a recorded
// gap, on ReadDocument's own reasoning in schema.go — a reader limitation is not
// a well-formedness verdict. A validate.Result whose Err is non-nil is a walk
// that STOPPED on a source fault mid-document, so what it did or did not charge
// records how far the walk got and not what the document holds: the walk keeps
// going after a charge such as an abstract declaration's, so a Result can carry
// BOTH a decidable violation and a truncated walk. And a violation set that
// holds any rule outside decidableRules declines rather than being read as
// a verdict a later slice's wider Assess might charge under an approximation;
// the COUNT is not a condition, since one root can honestly carry several
// charges (see decidedNotValid). An EMPTY violation set declines unless the
// shape's conditions above all hold.

// The instance lane's refusals: one token per exit at which execInstanceCase
// declines a case, which the GOXSD_DECLINES=1 listing writes after the case's
// ID (#2008). Each names the condition the function it is returned from states
// in its doc comment; a token ending in (#N) names the open issue that owns the
// arm. refuseDecode alone is returned at several sites, every one a decoder
// error in an internal/xmltok re-read of the instance. refuseUnevaluated alone
// is never written bare: unevaluatedRefusal suffixes it with the rules of the
// records that caused it (#2106).
const (
	// caseSchema (instancehints.go).
	refuseGroupAssembly   refusal = "group-assembly"       // assembleCase declined the group's schema
	refuseHintsUnresolved refusal = "hints-unresolved"     // instanceHints: the instance will not resolve
	refuseInlineSchema    refusal = "inline-schema"        // hintReader: an inline xs:schema inside another
	refuseInlineUnread    refusal = "inline-schema-unread" // assembleHints: an inline xs:schema rootReadable refuses
	refuseXMLBase         refusal = "xml-base"             // instanceHints, hintReader: xml:base on the root, or in scope of a hint or an inline xs:schema
	refuseOddLocation     refusal = "odd-schemaLocation"   // hintsOf: an odd xsi:schemaLocation member count
	refuseHintUnfollowed  refusal = "hint-unfollowed"      // assembleHints: a hint resolved to no document
	refuseHintAssembly    refusal = "hint-assembly"        // assembleHints: assemblyDeclined refused the outcome
	refuseHintUndeclared  refusal = "hint-undeclared-root" // assembleHints: no top-level declaration for the root
	refuseDoctype         refusal = "doctype"              // rootStart: a directive defaultsNoAttribute refuses
	refuseProlog          refusal = "prolog-text"          // rootStart: character data before the root that is not white space
	refuseDecode          refusal = "decode"               // an internal/xmltok decoder error, at any re-read site

	// builtinsSchema (instancehints.go): no group schema, and the instance
	// carries no hint and no inline xs:schema.
	refuseNoHint                  refusal = "no-hint"                       // no xsi:type, and no unknownXsi attribute
	refuseNoHintXsiTypeUnresolved refusal = "no-hint-xsi-type-unresolved"   // an xsi:type naming no built-in type definition
	refuseNoHintUnknownXsi        refusal = "no-hint-unknown-xsi-attribute" // no xsi:type, and an unknownXsi attribute
	refuseBuiltinsAssembly        refusal = "builtins-assembly"             // assembleCase declined builtinsSchemaDoc

	// execInstanceCase and assessInstance.
	refuseUnboundPrefix      refusal = "unbound-prefix(#2073)" // perr: a schema document's unbound-prefix charge
	refuseSchemaError        refusal = "schema-error"          // perr: any other assembly error
	refuseValidator          refusal = "validator"             // validate.New failed
	refuseInstanceUnresolved refusal = "instance-unresolved"   // assessInstance: the instance will not resolve
	refuseInstanceUnread     refusal = "instance-unread"       // assessInstance: xmlsrc.Validate failed
	refuseWalkStopped        refusal = "walk-stopped"          // assessInstance: validate.Result.Err
	refuseUndecidedRule      refusal = "undecided-rule"        // decidedNotValid: a charge outside decidableRules
	refuseUnevaluated        refusal = "unevaluated"           // validate.Result.Unevaluated is not empty; see unevaluatedRefusal

	// assessedSubtreeRoot's pre-gate refusals (subtreeroot.go).
	refuseVersioned       refusal = "versioned"        // closureVersioned
	refuseGateUnresolved  refusal = "gate-unresolved"  // the instance will not resolve for the re-read
	refuseEpilogDirective refusal = "epilog-directive" // documentEnd: a directive after the root

	// subtreeGate and the free functions it calls (subtreeroot.go).
	refuseUndeclaredRoot      refusal = "undeclared-root"             // undeclaredRoot: no declaration, no resolving xsi:type
	refuseNilLexical          refusal = "nil-lexical"                 // nilValue: no ·actual value·, at element
	refuseTypeTableUndecided  refusal = "type-table-undecided"        // selectedType: CompileCTATest declines a {test} it reached
	refuseTypeTableWhitespace refusal = "type-table-whitespace"       // selectedType: ctaAttributes cannot normalize a value
	refuseTypeUnresolved      refusal = "type-unresolved"             // selectedType: the selected {type definition}
	refuseErrorType           refusal = "error-type"                  // element: ·xs:error· governs
	refuseXsiTypeUnresolved   refusal = "xsi-type-unresolved"         // governingType, instanceTyped, localType: no type of that name
	refuseXsiTypeUndecided    refusal = "xsi-type-undecided"          // governingType, localType: ValidlySubstitutable errs
	refuseXsiTypeNotOverride  refusal = "xsi-type-not-overriding"     // governingType: T does not ·override·
	refuseSimpleAttribute     refusal = "simple-attribute"            // governed: an attribute on a simple-typed element
	refuseTypeKind            refusal = "type-kind"                   // governed: neither simple nor complex
	refuseAttributeUse        refusal = "attribute-use-unresolved"    // complex: an {attribute uses} member
	refuseAttributeType       refusal = "attribute-type"              // recordedAttributeType
	refuseContentMatcher      refusal = "content-matcher"             // complex: ContentMatcher declines
	refuseContentType         refusal = "content-type"                // complex: an unknown {content type}
	refuseAttributeUnadmitted refusal = "attribute-unadmitted"        // wildcardAttribute: no wildcard admits it
	refuseStrictAttribute     refusal = "strict-attribute-unresolved" // wildcardAttribute: strict, no declaration
	refuseAttributeLDT        refusal = "attribute-ldt"               // wildcardAttribute: locallyDeclaredAttribute
	refuseElementChild        refusal = "element-child"               // leaf: an element child
	refuseContentIncomplete   refusal = "content-incomplete"          // children: m does not accept
	refuseContentRejected     refusal = "content-rejected"            // child: m.Next refuses
	refuseMemberUnresolved    refusal = "member-unresolved"           // child: a substitution-group member
	refuseAttribution         refusal = "attribution"                 // child: an unknown attribution
	refuseAnyType             refusal = "any-type"                    // laxlyAssessed: no ·xs:anyType·
)

// newInstanceExec builds the instance lane's executor. The strict backend is
// built once here, exactly as newSchemaExec does it: it maps all 20 primitives,
// so parser.Parse's internal builtin.Seed precondition holds for every case.
func newInstanceExec() laneExecutor {
	backend := strict.New()
	return func(c caseSpec) (Status, refusal) {
		return execInstanceCase(backend, c)
	}
}

// execInstanceCase decides one instanceTest case, or honestly declines it
// (Fail, with the refusal naming the exit): it assembles the case's schema
// (caseSchema), assesses the instance document against it, and reads the
// assessment only where the answer is unconditional: a set of the ten
// decidable charges is "not valid", and an empty Result on an assessed subtree
// root (assessedSubtreeRoot) is "valid". An assembly rejected for a
// not-well-formed schema document is "not valid" without an assessment; every
// other rejected assembly declines. A decided case carries no refusal.
func execInstanceCase(backend value.Backend, c caseSpec) (Status, refusal) {
	schema, report, why, perr := caseSchema(backend, c)
	if why != "" {
		return Fail(), why
	}
	// A schema document the assembly RETRIEVED and the reader rejected as not
	// well-formed (wellFormednessFault) is recorded "not valid". That is a harness
	// convention, not a spec verdict: §4.3.2 item 3 makes a failed attempt to
	// dereference a schema location hint no error, but this document was retrieved
	// and is in error, and §5.1 requires a conforming processor to report an error
	// in a schema document used in constructing a schema while leaving any further
	// operation, assessment included, out of scope. Not-valid is how the harness
	// records that report, as the schema lane's fabricatedRejection read arm does
	// for a composed document. Every other perr declines: a resolver fault
	// (#1201), a read failure wrapping a cause, which may be a reader limitation,
	// a prefix bound only by the namespace declaration an internal-subset ATTLIST
	// defaults, and every rejection of a document that did read (schA8.i's
	// src-import clause 3.1). A genuinely unbound prefix declines too, the gap
	// wellFormednessFault's GAP(parser) marker tracks (#2073).
	if wellFormednessFault(perr) {
		return decideAgreement(false, c.expect.wantsValid()), ""
	}
	if unboundPrefix(perr) {
		return Fail(), refuseUnboundPrefix
	}
	if perr != nil {
		return Fail(), refuseSchemaError
	}
	v, err := validate.New(schema, backend)
	if err != nil {
		// Only a nil schema reaches here, which a nil perr should have excluded;
		// declining rather than trusting it keeps the lane's verdicts honest.
		return Fail(), refuseValidator
	}
	result, why := assessInstance(v, c.doc)
	if why != "" {
		return Fail(), why
	}
	if violations := result.Violations(); len(violations) > 0 {
		if !decidedNotValid(violations) {
			return Fail(), refuseUndecidedRule
		}
		return decideAgreement(false, c.expect.wantsValid()), ""
	}
	// An empty Result is "valid" for the gated shape alone, and only where the
	// walk recorded no check it reached and did not perform.
	if unevaluated := result.Unevaluated(); len(unevaluated) > 0 {
		return Fail(), unevaluatedRefusal(unevaluated)
	}
	if why := assessedSubtreeRoot(backend, schema, report, c.doc); why != "" {
		return Fail(), why
	}
	return decideAgreement(true, c.expect.wantsValid()), ""
}

// unevaluatedRefusal is refuseUnevaluated's token for records, the checks an
// assessment reached and did not perform: `unevaluated:<rule>[,<rule>…]`,
// naming each distinct Unevaluated.Rule once, in the order records first lists
// it (#2106). The token carries no whitespace and no `=`, so the
// GOXSD_DECLINES=1 listing's `<id>=<refusal>` entry still parses as one field.
func unevaluatedRefusal(records []validate.Unevaluated) refusal {
	var rules []string
	for _, u := range records {
		if rule := string(u.Rule()); !slices.Contains(rules, rule) {
			rules = append(rules, rule)
		}
	}
	return refuseUnevaluated + refusal(":"+strings.Join(rules, ","))
}

// assessInstance reads the instance document at doc and assesses it against v,
// or declines, naming the refusal. It declines on three conditions, none of
// which is a verdict about the document: a document that will not resolve or
// that the reader rejects for any reason, a caller fault or a document
// malformed before its document element or in what the walk left unread, what
// follows the document element included (xmlsrc.Validate's own error channel),
// and a walk that STOPPED on a source fault mid-document (validate.Result.Err),
// whose empty violation list records how far the walk got rather than what the
// document holds.
//
// The resolver is a loader.Dir rooted at the instance document's own directory,
// mirroring assembleCase's read of its root, so a case fixture is reached the
// same way whichever lane reaches it.
func assessInstance(v *validate.Validator, doc string) (*validate.Result, refusal) {
	resolver := loader.Dir(filepath.Dir(doc))
	rc, _, err := resolver.Resolve("", filepath.Base(doc))
	if err != nil {
		return nil, refuseInstanceUnresolved
	}
	defer func() { _ = rc.Close() }() // read-only handle: close error cannot affect the verdict
	result, err := xmlsrc.Validate(v, rc, xmlsrc.WithURI(doc))
	if err != nil {
		return nil, refuseInstanceUnread
	}
	if result.Err() != nil {
		return nil, refuseWalkStopped
	}
	return result, ""
}

// These are the ten rules validate.Validator.Assess charges, and with
// cvc-assertions-valid (decidableRules) the whole of what this lane may read as
// a verdict. All of them are catalog IDs in their BARE form: the charged clause
// lives in the message text, not in a dotted rule ID, so matching the rule
// alone is the only stable match — and it is the right one, since a root
// failing ANY clause of any of the ten is not locally valid and so not valid
// (§3.3.5.1 e-validity clause 1.1.1.1).
const (
	ruleCvcAssessElt      xsderr.Rule = "cvc-assess-elt"
	ruleCvcElt            xsderr.Rule = "cvc-elt"
	ruleCvcType           xsderr.Rule = "cvc-type"
	ruleCvcComplexType    xsderr.Rule = "cvc-complex-type"
	ruleCvcComplexContent xsderr.Rule = "cvc-complex-content"
	ruleCvcAttribute      xsderr.Rule = "cvc-attribute"
	ruleCvcAu             xsderr.Rule = "cvc-au"

	ruleCvcIdentityConstraint xsderr.Rule = "cvc-identity-constraint"
	ruleCvcID                 xsderr.Rule = "cvc-id"

	ruleCvcAssertion xsderr.Rule = "cvc-assertion"

	// ruleCvcAssertionsValid is Assertions Valid (Datatypes §4.3.13.3), which
	// Assess never charges at top level: see decidableRules.
	ruleCvcAssertionsValid xsderr.Rule = "cvc-assertions-valid"
)

// decidableRules collects them for the membership test below, so growing the
// set is one edit and the test reads the same however long it gets — a chain of
// != comparisons silently admits a rule someone forgot to add to it.
//
// cvc-assertions-valid is admitted beside the ten and is NOT load-bearing: a
// false assertions-facet {test} makes the value not Datatype Valid
// (cvc-datatype-valid clause 3), which validate charges under the rule of the
// item it decides — cvc-attribute, cvc-type or cvc-complex-type, each already
// here — with the facet's verdict as the wrapped cause, and decidedNotValid
// reads the top-level rule alone. No violation carries it at top level today;
// were one to, it would be unconditional, as each of the ten is.
var decidableRules = []xsderr.Rule{
	ruleCvcAssessElt, ruleCvcElt, ruleCvcType, ruleCvcComplexType, ruleCvcComplexContent,
	ruleCvcAttribute, ruleCvcAu, ruleCvcIdentityConstraint, ruleCvcID, ruleCvcAssertion,
	ruleCvcAssertionsValid,
}

// decidedNotValid reports whether the violations one assessment charged
// establish that the document is not valid, unconditionally and whatever the
// unassessed rest of it holds.
//
// At least one violation, with every violation carrying one of the enumerated
// rules, is that evidence. Each enumerated rule is an unconditional "not valid"
// on its own, so a set of them is one too, however many. No violation at all
// leaves e-validity's conjunction unevaluated and declines (see the file
// comment); so does a set holding any OTHER rule — that is a shape this slice
// was not written against, and a later, wider Assess may charge one under a
// fail-open approximation, which reading as a verdict would be trusting it sight
// unseen.
//
// The count is not pinned at one because the clauses quantify independently —
// cvc-complex-type over the attributes present, the uses required and the
// ·defaulted attributes·; cvc-attribute and cvc-au over one attribute from two
// different {value constraint} sources — so one root can carry several charges
// honestly.
func decidedNotValid(violations []*xsderr.Error) bool {
	if len(violations) == 0 {
		return false
	}
	for _, v := range violations {
		if !slices.Contains(decidableRules, v.Rule) {
			return false
		}
	}
	return true
}
