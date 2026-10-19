package conformance

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// This file activates the schema lane (issue #175) by giving the schema entry of
// defaultLanes a real executor. It touches nothing else in the runner (the #6
// seam, STYLE T2): the lane's selector stays selectsKind(kindSchema), so the
// executor is handed EVERY schemaTest case and either decides it or honestly
// DECLINES it (records a Fail gap) — a case it cannot decide for the right
// reason never flips to pass. It is package-internal conformance support: it
// exports nothing and no library code imports it.
//
// The gate this file documents is assembleCase, and it is SHARED: the instance
// lane (conformance/instance.go, #713) needs the very same assurance before it
// may assess anything against the schema an assembly built, so it asks the same
// question of the same closure and declines on the same answers.
//
// # What a schemaTest asserts
//
// A schemaTest asks: is THIS schema document itself schema-valid? The lane
// decides that with the end-to-end assembler (parser.Parse, issue #179, which
// subsumes the producer and Finalize of issues #174/#176/#177/#178). Parse
// implements §4.2.1's schema(D) — the root document's components plus,
// transitively, those of every document reached through an <xs:include> child
// (§4.2.3), chameleon coercion included (§F.1), an <xs:override> child (§4.2.5,
// which composes the overridden document as an include of its §F.2-transformed
// self), or an <xs:import> child (§4.2.6.2, which coerces nothing: the imported
// document keeps its own namespace) — and maps top-level
// <simpleType>/<element>/<attribute>/<attributeGroup>/<group>/<notation> and the
// decidable subset of <complexType> (implicit content, and both alternants of
// each of <complexContent> and <simpleContent>; its particles including <group
// ref>, local element/attribute declarations, attribute uses including
// <attributeGroup ref>, wildcards, <openContent> and the schema-level
// <defaultOpenContent> it falls back to, and <assert> assertions) into xsd
// components, maps the name= identity constraints of global and local <element>s
// (#178), seeds the ur-type xs:anyType, resolves cross-references, and rejects
// duplicate top-level names within a kind. The ref= identity-constraint form and
// the inline anonymous local complexType forms outside the implicit-content
// shape are SILENTLY SKIPPED or declined (§3.1.2 permits ignoring a
// not-yet-produced representation), NOT rejected.
//
// The EXTENSION forms are DECIDED as of #336: Parse builds <complexContent>
// <extension> and <simpleContent> <extension> types (#228; §3.4.2.3.3 clause 4.2,
// §3.4.2.2 cases 3-5), cos-ct-extends (§3.4.6.2) judges them (#264), and every
// §3.4.2 base fold its case-1 clauses read is now done — clause 1.2's
// {attribute uses} (§3.4.2.4 clause 3, #401), clause 1.3's {attribute wildcard}
// (§3.4.2.5 clause 2.2, #265) and clause 1.7's {assertions} (§3.4.2.1 clause 1,
// #346). ONE cos-ct-extends clause cannot reject inside the admitted shape:
// clause 1.5.
//
// Clause 1.5 (two-step derivability) is an ENGINE approximation: it is proven
// only for a pure-extension chain, and a chain that mixes extension and
// restriction steps is accepted unconditionally (GAP(xsd) in
// xsd/complexextension.go, follow-up #392). That marker, not this comment,
// carries its retirement. Case 2 (simple base) is complete.
//
// It is an UNDER-rejection — the lane can report "valid" for a schema a complete
// processor rejects, never "invalid" for a valid one — so it is a recorded gap
// of the same shape as the ones the admitted <restriction> path already carries,
// not a fabricated verdict.
//
// # Why "Parse returns nil" is not, by itself, evidence of validity
//
// Because the producer silently skips the representations it does not yet build,
// a document whose top-level content includes (say) an invalid <group> or an
// undecidable <complexType> form alongside valid simpleType/element/attribute
// would still assemble with no error — a FALSE ACCEPT. §3.1.2's licence
// to ignore a representation is an implementation choice about what to BUILD; it
// does not make the spec consider such a document valid: the invalid complexType
// still makes the document schema-INVALID under sch-props-correct clause 1
// (§3.17.6.1), whichever cvc-complex-type/cos-* rule it violates (oracle
// grounding, issue #175). So "Parse returns nil" is genuine evidence of validity
// ONLY when the top-level content is PROVABLY CONFINED to what the producer
// actually processes.
//
// Since #242 that qualifier binds over a CLOSURE, not one document. The assembly
// reads the whole <include>/<override>/<import> closure, so a composed document
// holding a skipped representation false-accepts exactly as a root one would. So
// the harness runs parser.ParseReport (parser.ParseSet for several roots), which
// reports the DOCUMENT SET it assembled, and gates every document of that set on
// the allowlist below (closureDecidable, conformance/schema_closure.go). Until
// #272 the harness instead re-walked §4.2's composition edges itself, because
// parser.Parse could not be asked which documents it read; the gated set is now
// the assembled set by construction rather than by two walks agreeing, which is
// what closes the under-gating hazard — a document the harness missed but the
// parser read would be a document whose shape was never gated, the false accept
// back again.
//
// # The decidable shape (the strict top-level allowlist)
//
// assembleCase therefore admits an ACCEPTED assembly only after confirming the
// whole shape of every document in its closure is confined to what the producer
// checks, and the lane DECLINES (Fail) anything else. A REJECTED assembly is
// held to the same confinement unless its error is a grammarRejection, which no
// out-of-subset construct can fabricate (step 3, #2076):
//
//  1. Readability. parser.ReadDocument is run on every declared root document,
//     and the assembly reads every composed document through it. ANY error
//     DECLINES the case (Fail), never a validity verdict: a ReadDocument error
//     does not distinguish a genuine XML well-formedness fault from a reader
//     LIMITATION. A well-formed document declaring an encoding the reader does
//     not decode without a byte-order mark (#361), or referencing a general
//     entity its DOCTYPE internal subset declares, is rejected as xml-wf, so
//     treating that as observed-invalid would fabricate an "invalid" verdict for
//     a well-formed document — a wrong-reason pass that would flip pass→fail
//     once the reader lifts the limitation. So malformed XML is NOT a
//     claimed schema-well-formedness sub-cohort here; it is a declined recorded
//     gap. For a COMPOSED document the decline comes through the report: a
//     document that resolved but could not be read is one the assembly never took
//     in, recorded as parser.UnfollowedUnreadable, and the error it also returns
//     completes the conjunction in step 4 — unless that error is a
//     well-formedness fault the reader charged itself of a kind no reader
//     limitation produces, which step 4 decides (wellFormednessFault, #2067).
//  2. Root identity. If the root is not <schema> (IsSchema false) the case is
//     DECLINED: §3.17.2 explicitly does NOT require <schema> to be the document
//     root, so Parse's error there is a plain non-xsderr Go precondition fault,
//     not a sch-props-correct rejection — not decidable for this lane. Inventing
//     a "root must be <schema>" rejection would overreach (oracle grounding). An
//     INCLUDED document that is not a <schema> is the opposite case and is NOT
//     declined: src-include clause 1 makes that a genuine rejection, which the
//     assembly emits, and the document never enters the report to be gated.
//  3. Top-level allowlist, unless the document is UNCONDITIONALLY REJECTED
//     first. A document holding a MISPLACED <xs:notation> — one whose parent is
//     neither <xs:schema> nor <xs:override>, the only two content models that
//     reach the xs:schemaTop group <notation> is declared in — is admitted
//     whatever else it holds (#945, holdsMisplacedNotation). Nothing in it can
//     be silently skipped into a vacuous accept, because the parse cannot return
//     nil: rejectS4SFaults walks the whole document before any producer runs and
//     charges the fault wherever it stands. The verdict it buys is "invalid",
//     which §5.1's first bullet (xmlschema11-1.md:4296) makes the right one for
//     such a document however its remaining shape reads, and no later slice can
//     flip it — that permanence is what separates this from the ReadDocument
//     limitation step 1 declines. Only a construct the producer rejects
//     GENUINELY and UNCONDITIONALLY may be admitted this way.
//
//     The allowlist below is a guard against a skipped construct, so it binds
//     an assembly the parser ACCEPTED, or rejected with an error a skip could
//     have produced; it does not bind every non-nil error (#2076). A closure
//     outside it is still decided when the assembly fails with a
//     grammarRejection: an error carrying no rule — this processor's spelling of
//     a §2.4 clause 1 sd-valid fault, which §5.1's first bullet makes the
//     document's own — with no directive left unfollowed. A skip leaves a
//     component missing, and the reference to it is charged src-resolve
//     (§3.17.6.2, §5.3), a ruled error that still declines. grammarRejection
//     lists every decline trigger by identifier, with whether the producer
//     skips or rejects it.
//
//     Every top-level child element must otherwise be xsd:annotation,
//     xsd:include, xsd:import, xsd:override, xsd:simpleType, xsd:element,
//     xsd:attribute, xsd:complexType, xsd:attributeGroup (named definition),
//     xsd:group (named definition), xsd:defaultOpenContent (in every shape, as
//     of #352: the two its declaration forbids are rejected by the producer for
//     any document that declares one, so they are real verdicts rather than
//     content-dependent ones) or xsd:notation or xsd:redefine. A NON-XSD element
//     at top level closes the false-accept gap above by DECLINING the whole
//     case unless the assembly fails with a grammarRejection: the producer
//     passes over it unreported and unrejected, the open gap #1036 owns. An
//     XSD-namespace name outside that list is admitted instead
//     (#1380), those thirteen being exactly what <schema>'s content model admits
//     and rejectUnmappedTopLevel charging every other — the same permanence
//     argument the misplaced <notation> is admitted on. Within the allowed
//     kinds:
//     - include: always admitted (#242). Its own content model is (annotation?),
//       so it contributes nothing the producer could silently skip; the
//       decidability of the document it POINTS AT is established by the closure
//       gate, which runs this same allowlist over every document the assembly
//       reported reading (and so, transitively, over that document's own
//       <include>s and <import>s) — not by this allowlist entry. src-include
//       (§4.2.3) itself imposes no shape constraint on the included document,
//       only existence and targetNamespace agreement, both of which the assembly
//       decides genuinely. An <include> whose schemaLocation does not resolve
//       yields no document to gate: the assembly reports that
//       (parser.UnfollowedLocationUnresolved) exactly as it does for an <import>
//       that yields no D2, and assembleCase declines only if the parse then fails
//       with the src-resolve rejection that shortfall could have fabricated
//       (#276, narrowed by #404 — see fabricatedRejection).
//     - override: admitted (#183) when every child of it is a decidable source
//       declaration in its own right (overrideDecidable), because §F.2 clause 1
//       makes those children top-level declarations of the OVERRIDDEN document.
//       The document it points at, and the rest of §4.2.5's ·target set·, is
//       gated by the same closure gate that gates an <include>'s target, being in
//       the same reported document set; src-override's own clauses are then
//       enforced genuinely by the assembly.
//     - import: admitted at top level (#182) on the same reasoning. As for
//       include, a directive that yields no D2 (no schemaLocation, or one that
//       does not resolve) is REPORTED as unfollowed, and declines the case only
//       when the parse also fails with the src-resolve rejection the missing
//       components would produce. See the Composition section below,
//       fabricatedRejection and parser.AssemblyReport.Unfollowed.
//     - element: must have no inline <simpleType>/<complexType> child, and every
//       <unique>/<key>/<keyref> child must use the name= form. A bare
//       element (no type=) defaults to xs:anyType (§3.3.2.1 case 4), now seeded as
//       a Complex Type Definition (§3.4.7), so it resolves and is decided
//       genuinely; type= is no longer required. An inline anonymous type is an
//       explicit src-element clause 3 (§3.3.3) rejection that conflates a genuine
//       both-present violation with a mere not-yet-supported inline-only form —
//       indistinguishable here, DECLINED. A name= identity constraint IS produced
//       (#178) — its src-identity-constraint (§3.11.3) and c-props-correct
//       (§3.11.6.1) rejections and its finalize-time keyref resolution
//       (src-resolve clause 1.7) are all genuine — while the ref= form names an
//       existing definition the producer does not yet resolve, so it is DECLINED.
//     - notation: always allowed. Its content is (annotation?) and its whole
//       tableau is decided at produce time (n-props-correct §3.14.6 rejects both
//       identifiers absent), so nothing is silently skipped.
//     - complexType (top-level, or a <complexContent>/<simpleContent> derivation
//       reached transitively): must lie within the producer's decidable subset per
//       complexTypeDecidable — implicit content, either <complexContent>
//       alternant, or <simpleContent> <extension>, whose content model is
//       element/any/sequence/choice/all/<group ref>, whose attributes are local
//       <attribute>/<anyAttribute>/<attributeGroup ref>, whose <openContent> maps to
//       {open content} (#230) and whose <assert> children map to {assertions} (#178),
//       with no <simpleContent> <restriction> and no inline anonymous local type.
//       <simpleContent> <restriction> and the inline forms need a later producer slice,
//       so Produce declines them with a plain limitation error, not a spec verdict. A
//       <group ref>/ <attributeGroup ref> IS produced (#177): its target resolves (or
//       fails src-resolve) genuinely. A <simpleContent>/<complexContent> carrying
//       NEITHER alternant is admitted too (#868): §3.4.2.2 and §3.4.2.3 each require
//       one, and the producer rejects a document carrying neither. A real structural
//       violation inside an admitted shape (src-ct, cos-all-limited, src-wildcard, …)
//       flows through as a genuine rejection.
//     - attributeGroup (top-level, §3.6.2): children only <attribute> (no inline
//       anonymous type), <attributeGroup>, and <anyAttribute> — the shapes the
//       producer folds in (§3.6.2.1/§3.6.2.2). A dangling ref (src-resolve clause
//       1.4) and a circular ref chain (spec-legal, §3.6.2.1) are both decided
//       genuinely.
//     - group (top-level, §3.7.2): a single all/choice/sequence body whose
//       particles are decidable; the body maps to {model group} genuinely (a
//       missing body is the s4s-grammar rejection xs:namedGroup's content model
//       requires, #884).
//     - A <unique>, <key> or <keyref> is admitted under a top-level
//       <attributeGroup>, in a top-level <group> that has no body, and under any
//       <all>/<choice>/<sequence> (#1817): Appendix A admits an identity
//       constraint under an <element> alone, and the producer rejects each of
//       those three positions under §5.1's first bullet, with no rule ID.
//     - Neither of those two requires the DEFINITION form (#1182). A top-level
//       <group>/<attributeGroup> with no name=, or one carrying the ref= that
//       xs:namedGroup/xs:namedAttributeGroup restricts to use="prohibited", is
//       admitted here and REJECTED by the producer under §2.4 clause 1 sd-valid
//       (§5.1's first bullet for schema construction) — an s4s attribute-use
//       fault, which carries no numbered rule ID. The nested reference forms are
//       the mirror image and are admitted on the same terms: a <group> in a
//       content model or an <attributeGroup> in an attribute-declaration tail
//       with no ref=, or carrying the prohibited name=, is the producer's
//       rejection to make, not this gate's to pre-empt.
//     - attribute: an inline <simpleType> child is admitted when its own shape is
//       produced (#733 maps §3.2.2.1 dcl.att.global's tier 1, which dcl.att.local
//       states in the same words), so a type= beside it is a genuine src-attribute
//       clause 4 (§3.2.3) verdict. A bare attribute is FINE too: it defaults to
//       xs:anySimpleType (§3.2.2.1), which builtin.Seed always seeds, so type= is
//       NOT required.
//     - simpleType (top-level or any anonymous inline one reached transitively
//       through a restriction, list, union, element or attribute chain): all
//       three §3.16.2.1 alternatives are admitted. <list> is produced in both its
//       forms (#447) and <union> in all three of its (#738), and every inline
//       <simpleType> item or member child is recursed into. <restriction> is
//       admitted outright: every constraining facet it can carry is produced,
//       the two folded ones included — one assertions facet per restriction
//       (#178, Datatypes §4.3.13) and one enumeration facet per restriction
//       (#740, §4.3.5.2 xr-enumeration) — and an inline <simpleType> base child
//       (§3.16.3 clause 2) is recursed into. A <simpleType> naming NONE of the
//       three is admitted too (#786): simpleTypeBody rejects that document
//       unconditionally, so the rejection is the producer's own and not a
//       limitation. No alternative's member-source arrangement is pre-checked:
//       those ARE the genuine src-simple-type clauses 2, 3 and 4 Produce
//       correctly enforces, so a violation flows through as a real decidable
//       rejection.
//     - annotation: always allowed, no further check.
//  4. Decide. When every document of the closure passes, or the assembly
//     failed with a grammarRejection (step 3), observed = (parser.ParseReport's
//     err == nil): a nil error is genuine evidence of validity (no document of the
//     assembly has any of the violations checked above, so a real one would
//     surface), and a non-nil error is a REAL, implemented rejection (src-include
//     §4.2.3, src-import and src-import-noselfimport §4.2.6.2, sch-props-correct
//     clause 2 duplicate-name §3.17.6.1, src-element §3.3.3, src-attribute §3.2.3,
//     src-simple-type §3.16.3, src-override §4.2.5, src-resolve §3.17.6.2,
//     st-props-correct, src-identity-constraint §3.11.3, c-props-correct
//     §3.11.6.1, n-props-correct §3.14.6, and for the complex-type subset src-ct
//     §3.4.3, cos-all-limited §3.8.6, src-wildcard §3.10.3, p-props-correct
//     §3.9.6, cos-nonambig §3.8.6.4, cos-element-consistent §3.8.6.3,
//     ct-props-correct §3.4.6.1 and derivation-ok-restriction §3.4.6.3), never a
//     fabricated one — the shape allowlist excludes every case whose rejection
//     would be a limitation-in-disguise, and a grammarRejection is a fault of
//     the document's own element tree. The case Passes iff observed agrees with
//     the suite's declared validity.
//
//     Steps 1-3 rule out the non-verdict failure modes ACROSS THE WHOLE CLOSURE:
//     the root was independently confirmed resolvable, readable and
//     <schema>-rooted before the assembly ran, and every document the assembly
//     did take in is reported and shape-gated, the gate yielding only to a
//     grammarRejection. Two of those modes reach this
//     step — a read failure on a COMPOSED document that may be a reader
//     limitation, reported as parser.UnfollowedUnreadable, and a rejection
//     fabricated by components a directive did not bring in — and
//     fabricatedRejection eliminates both, as
//     the ONE site that discriminates on the error, by rule and, for
//     src-resolve, by clause rather than by type. Every OTHER failure this
//     step reads as a verdict, an unruled plain error included, with the
//     exception the GAP below states and the #1883 limitation grammarRejection's
//     GAP states: an <include>/<override> carrying
//     no schemaLocation at all is a §2.4 clause 1 grammar fault no Schema
//     Representation Constraint covers
//     (parse.go's compose), which STYLE E2 requires be charged WITHOUT a rule ID
//     — an unruled error is this processor's spelling of "not valid against the
//     schema for schema documents", not a signal that no verdict was reached
//     (#404).
//
//     GAP(conformance): a THIRD non-verdict mode survives to this step, and
//     reading it as a verdict scores in the ratchet-corrupting direction.
//     parser/parse.go's fetch has TWO error-returning exits and only one of them
//     is a read: a resolver fault that is not loader.ErrNotFound — a permission
//     or transport error, ENOTDIR among them — records
//     parser.UnfollowedLocationUnresolved, NOT parser.UnfollowedUnreadable, and
//     returns an unruled plain Go error. fabricatedRejection therefore finds no
//     unreadable directive and no src-resolve rule, and this step decides
//     "invalid" off a pure I/O failure on a NON-ROOT document — the fault
//     assembleCase's own step 1 declines when it hits the ROOT. #1201 owns the
//     fix, which is parser's rather than this file's; both shapes it may take
//     are stated there. No suite case takes that exit today — every
//     schemaLocation in the corpus was scanned through the loader.Dir open path
//     and none yields a non-ErrNotFound error (#404) — which is exactly why
//     neither the gate nor the ratchet can see it. Do not close it by declining
//     on "unruled AND
//     UnfollowedLocationUnresolved": that predicate is annotB020, this lane's
//     own witness, and it gives back a banked win (#404).
//
// # sch-props-correct clause 2 is per-kind
//
// The duplicate-name rejection (sch-props-correct §3.17.6.1 clause 2) is checked
// PER KIND ({type definitions}, {element declarations}, {attribute declarations}
// are distinct properties, §3.17.1): two simpleTypes sharing an expanded name
// collide, but a simpleType and an element sharing a name do NOT.
// {identity-constraint definitions} is one such property too, and it collects the
// constraints of every <key>/<keyref>/<unique> ANYWHERE in the document
// (§3.17.1), so two identically-named identity constraints under two different
// element declarations DO collide — a genuine clause-2 rejection, not a
// producer artifact. The executor relies on Finalize's per-kind indexByName for
// exactly this, so no cross-kind duplicate check is done here (that would be a
// false-INVALID verdict, a ratchet regression risk).
//
// # Why no false ratchet-corrupting pass is possible
//
// Every "invalid" verdict this lane emits comes from ONE source: parser.Parse
// rejecting an assembly, either one EVERY document of which already passed the
// allowlist or one whose error is a grammarRejection — a §2.4 clause 1 fault
// that no construct the allowlist refuses can fabricate, since one the producer
// skips leads to no unruled error, and one it rejects is charged that fault
// itself. A ReadDocument error produces an "invalid" verdict only on a COMPOSED
// document, and only when it is a well-formedness fault the reader charged
// itself that no reader limitation produces (wellFormednessFault); every other
// one declines (step 1), precisely because it can be a reader limitation rather
// than a real violation, and turning that into "invalid" would fabricate a
// verdict for a well-formed document.
//
// A "valid" verdict coincides only with a truly-valid ground truth: a truly-valid
// assembly (by definition) has none of the checked violations, so Parse
// correctly finds none. An "invalid" verdict coincides only with truly-invalid
// ground truth via a REAL implemented violation — never a fabricated one, since
// the shape allowlist excludes every form (an <override> child carrying no
// name=, an <override> child of a kind <override>'s own content model does not
// admit — both passed over in silence — an <assertions> facet child folding into
// nothing, the produced-but-unjudged extension forms) where the producer's
// rejection would be a limitation rather than a spec violation, or its silence a
// missing rejection. A particle under a <simpleContent> alternant is excluded
// CONSERVATIVELY instead, on neither of those grounds: the producer REJECTS it,
// the s4s order check charging that xs:simpleExtensionType is (annotation?,
// ((attribute | attributeGroup)*, anyAttribute?), assert*)
// (xmlschema11-1.md:1697), which admits no particle in any position — the
// reading schema_closure_test.go's undecidable const records. Declining over an
// explicit rejection forgoes a verdict and cannot fabricate one, so that
// exclusion is a harvest candidate on #1182's reasoning rather than a soundness
// requirement. A suite-invalid case whose only defect is a rule finalize does NOT
// yet check (cos-content-act-restrict — derivation-ok-restriction clause 2.4.2,
// #263 — cos-ns-subset, #265, or the Open Content half of
// derivation-ok-restriction clause 2.4, which xsd.Schema's content-model
// automaton fails open on: xsd/contentrestricts.go's GAP(xsd), live for a
// produced {open content} since #230) is produced cleanly, so the lane observes
// "valid", disagrees with the suite, and records a still-failing gap — never a
// wrong "invalid" pass. The remaining risk the allowlist closes is the VACUOUS
// pass — a document of entirely skipped top-level content that would otherwise
// always "pass" through a producer doing nothing — which is why step 3 confines
// the whole top level of EVERY document in the closure to the processed kinds and
// the decidable complexType subset.
//
// # Composition: <include>, <import> and <override> decided, redefine deferred
//
// <xs:include>, chameleon inclusion included, is DECIDED as of #242,
// <xs:import> as of #182 and <xs:override> as of #183: the assembly follows all
// three closures (#179/#182/#183) and the closure gate covers every document in them,
// so an include/chameleon/import/override case is now decided for the same reason a
// single-document case is, not guessed. A composition directive that yields no D2 —
// an <import> with no schemaLocation, or an <include>, <override> or <import> whose
// schemaLocation does not resolve — is REPORTED as unfollowed
// (parser.AssemblyReport.Unfollowed) and declines the case only when the parse ALSO
// fails src-resolve outside clause 4 (#2054): the missing document's components are
// then absent from the assembly and the reference that wanted them failed src-resolve
// clauses 1-3 at finalize, a FABRICATED "invalid" verdict, the one direction that can
// corrupt the ratchet. That conjunction is the whole hazard (#276): where the parse
// succeeds despite the missing document, §4.2.3 clause 2.4's "not an error ... the
// inclusion must not be performed" is simply in force and the case is decided
// normally — the suite's own cl. 2.4 tests (MS-Schema schD8 and friends) depend on
// that. Where it fails on a rule the shortfall cannot reach, the rejection is the
// document's own and the case is likewise decided (#404, fabricatedRejection). The
// directives are one hazard, not two: src-include clause 2.4 and src-import's "not an
// error" text are parallel, and src-resolve clause 4 (cl.qnr.nsdeclared) licenses a
// same-namespace reference (4.2.1) and a reference into a namespace with a PRESENT
// <import> element (4.2.2) alike, whether or not that import's schemaLocation
// resolved.
//
// One import-adjacent gap is the OTHER direction — fabricated "valid" — which can
// only cost wins, never corrupt: a namespace whose components are genuinely missing
// is not reported as a §5.3 missing component. It makes the lane observe "valid"
// where the suite says "invalid", which records a gap rather than a pass.
// src-resolve clause 4 (cl.qnr.nsdeclared, §3.17.6.2) is enforced per containing
// document (parser's licensedNamespace), and §4.2.6.1 keeps its charge out of
// §5.3, so it decides the case even beside an unfollowed directive (#2054,
// fabricatedRejection).
//
// An <xs:override> is admitted when every one of its children is itself a
// decidable source declaration (overrideDecidable): those children become
// top-level declarations of the overridden document (§F.2 clause 1), so an
// undecidable one would be an undecidable top-level declaration by another route.
// The document it points at, and every document that one <include>s or
// <override>s — §4.2.5's ·target set· — is gated by the same closure gate that
// covers an <include>'s target: all of them are documents the assembly read, so
// all of them are in its report. Verdicts on an admitted override case are
// genuine in both directions: the substituted declarations are really produced (a
// violation among them, such as a simple type left restricting itself, surfaces
// as the rule it breaks), and an unmatched override child is really ignored
// (§4.2.5) rather than added.
//
// <xs:redefine> is DECIDED as of #286, and for ALL FOUR redefinable kinds as of
// #505. The assembly follows it, so the document it names is in the
// closure and gated like any other, and the redefinition's own clauses —
// src-redefine clause 1 (a non-empty <redefine> whose schemaLocation does not
// resolve is an ERROR, unlike <include>), clause 5, clauses 6.1.1/6.1.2, clause
// 7.1, and src-expredef's requirement of a top-level definition item of the
// appropriate name and kind — are enforced genuinely. A redefining
// <simpleType>/<complexType> is paired with the {name}-·absent· original of
// src-expredef clause 1.1, so its self-derivation is a real derivation rather
// than the circularity the pairing exists to avoid, and each redefining child is
// gated by the predicate its own element type is gated by anywhere else
// (redefineDecidable). §F.2 clause 1's "or <redefine>" scope is no longer
// empty either: an <override> in force over a document now substitutes for that
// document's <redefine> children too, restricted to the four element types
// <redefine>'s own content model admits.
//
// # Still deferred
//
// The not-yet-produced complexType forms named above widen in with later slices
// (exactly as the datatypes lane grew across #15/#57/#80, and as a global
// attribute's inline anonymous type did with #733); they stay DECLINED (Fail)
// recorded gaps here, never guessed. UPA and EDC landed with #180,
// derivation-ok-restriction with #262 and cos-ct-extends with #264, so the
// admitted complexType cases those rules reject — restriction and extension
// alike, the latter admitted by #336 — are now decided; the cases still turning
// on cos-content-act-restrict (#263) stay failing gaps rather than wins until
// that lands.
//
// A schemaTest with MORE THAN ONE <ts:schemaDocument> child declares a SET of
// documents to be loaded "one by one, in order" (xsts.xsd, the suite's own
// catalog schema); the runner carries all of them (caseSpec.extraDocs), and this
// lane decides the case on the whole set (#1840). The spec has no notion of
// several roots — §4.2.1's schema(D) is per document — so the set is read as the
// union of each root's schema(D) held to sch-props-correct (§3.17.6.1) as ONE
// schema: parser.ParseSet, which charges clause 2 for one expanded name two roots
// declare as distinct components. A declared document the FIRST one's own
// <include>/<override>/<import>/<redefine> already reaches is not entered as a
// root again (extraRoots), so such a case is just the composition case above.
// Entering it again is not neutral: a document reached as a <redefine> or
// <override> target and then read plainly contributes the very definitions the
// directive replaced, and collides with them.

// newSchemaExec builds the schema lane's executor. The strict backend is built
// once here (mirroring newDatatypesExec's strictBackend := strict.New()): it maps
// all 20 primitives, so parser.Parse's internal builtin.Seed precondition holds
// for every case. Parse seeds from the backend on each call, so no symbol table is
// captured here — the executor only threads the backend and reads the documents.
func newSchemaExec() executor {
	backend := strict.New()
	return func(c caseSpec) Status {
		return execSchemaCase(backend, c)
	}
}

// execSchemaCase decides one schemaTest case, or honestly declines it (Fail).
// The whole decidability gate is assembleCase, over the case's own declared
// document set; what is left here is the one comparison of the assembly's
// outcome with the suite's declared validity.
func execSchemaCase(backend value.Backend, c caseSpec) Status {
	_, _, decidable, perr := assembleCase(backend, c.doc, c.extraDocs)
	if !decidable {
		return Fail()
	}
	return decideAgreement(perr == nil, c.expect.wantsValid())
}

// The words a schema charge probe answers where a decided case carries no rule
// to name. Each is parenthesized so no spec rule ID can collide with it.
// doc.go's GOXSD_DECLINES=1 paragraph owns what chargeAccepted and
// chargeUnruled mean; chargeDeclined never reaches a decided disagreement
// (a decline is never a decided case), so doc.go does not define it.
const (
	chargeAccepted = "(accepted)"
	chargeUnruled  = "(unruled)"
	chargeDeclined = "(declined)"
)

// newSchemaCharge builds the schema lane's charge probe (lane.charge): the rule
// xsderr.RuleOf reads off the assembly's own error, through the same
// assembleCase the executor decides with, so the rule named is the one behind
// the decision and not a second reading of the documents. Like newSchemaExec it
// builds its strict backend once.
func newSchemaCharge() func(caseSpec) string {
	backend := strict.New()
	return func(c caseSpec) string {
		_, _, decidable, perr := assembleCase(backend, c.doc, c.extraDocs)
		if !decidable {
			return chargeDeclined
		}
		if perr == nil {
			return chargeAccepted
		}
		rule, ok := xsderr.RuleOf(perr)
		if !ok {
			return chargeUnruled
		}
		return string(rule)
	}
}

// assembleCase assembles the schema rooted at doc, together with any FURTHER
// documents extraDocs declares, and reports whether the outcome may be read as a
// verdict at all. It is the schema lane's decidability gate and the instance
// lane's precondition alike (conformance/instance.go).
//
// It reads each declared document to check the two preconditions parser.ParseSet
// answers with a plain Go error rather than a validity verdict (rootReadable),
// runs parser.ParseReport from doc, re-assembles with parser.ParseSet when a
// further declared document lies outside that closure (extraRoots), and gates
// the WHOLE <xs:include>/<xs:override>/<xs:import> closure of every root on the
// decidable top-level shape (closureDecidable, which runs schemaShapeDecidable on
// every document the assembly consumed) unless the assembly failed with a
// grammarRejection (assemblyDeclined). The third result, decidable, is false —
// and the caller DECLINES — under any of five conditions: a declared document it
// cannot resolve; one it cannot read (any ReadDocument error, including a
// reader limitation such as an encoding it does not decode); one whose root
// element is not <schema>; a closure holding one document outside the
// producer's decidable subset, where the assembly succeeded or failed with
// anything but a grammarRejection (#2076); and a case whose parse failed with a
// rejection its own unfollowed directives could have fabricated
// (fabricatedRejection, #276/#404). The other three results say nothing then.
//
// Where decidable is true, the fourth result is the assembly's OWN error: nil is
// genuine evidence of validity — no document of the assembly has any of the
// violations the allowlist confines it to, so a real one would surface — and
// non-nil is a REAL implemented rejection, the closure confined to the
// allowlist or the error a grammarRejection. The first result is what the assembly
// built, and the second the parser.AssemblyReport of the documents it read. The
// schema lane reads the error alone; the instance lane needs the schema, and
// takes it only where the error is nil, a schema the assembly rejected being not
// the schema the suite declared — and the report, from which groupSchema reads
// the namespaces the assembly already covers (closureNamespaces).
//
// The resolver is a loader.Dir rooted at doc's own directory, wrapped by
// pinnedResolver, and the root is named by its BASE name, because
// parser.ParseReport reads the root under exactly the location string it is handed
// (readRootDocument in parser/parse.go): passing the full path would give the root
// document a base URI of "…/sunData/SType/x" instead of "x", and every <include>
// in it would then resolve one directory tree away from where the resolver serves.
// The harness's own precondition read below therefore uses the SAME resolver and
// the SAME location string, so it reads byte-identically the document the assembly
// roots at.
func assembleCase(backend value.Backend, doc string, extraDocs []string) (*xsd.Schema, *parser.AssemblyReport, bool, error) {
	return assembleCaseWith(backend, doc, extraDocs, nil)
}

// assembleCaseWith is assembleCase with hints, parser.HintAt roots, entered
// after every root assembleCase enters, through the same resolver and under the
// same gate. The instance lane passes the hints its instance's root carries for
// namespaces the group's own closure leaves uncovered (groupSchema); every other
// caller passes none.
func assembleCaseWith(backend value.Backend, doc string, extraDocs []string, hints []parser.Root) (*xsd.Schema, *parser.AssemblyReport, bool, error) {
	resolver := pinnedResolver{dir: loader.Dir(filepath.Dir(doc))}
	location := filepath.Base(doc)
	if _, ok := rootReadable(resolver, location); !ok {
		return nil, nil, false, nil
	}
	// ParseReport, not Parse: the verdict needs the DOCUMENT SET the assembly
	// consumed, not only its components (#272).
	schema, report, perr := parser.ParseReport(location, parser.WithResolver(resolver), parser.WithBackend(backend))
	roots, ok := extraRoots(report, resolver, doc, extraDocs)
	if !ok {
		return nil, nil, false, nil
	}
	roots = append(roots, hints...)
	if len(roots) > 0 {
		// A declared document the first root's closure did not reach, or a hint:
		// the case is the union of every root's schema(D), assembled as ONE schema so
		// sch-props-correct (§3.17.6.1) clause 2 and src-resolve clause 4 hold
		// across the set (#1840). Its report is a superset of the first root's
		// closure, so the gate below runs over every document either consumed.
		schema, report, perr = parser.ParseSet(append([]parser.Root{parser.RootAt(location)}, roots...),
			parser.WithResolver(resolver), parser.WithBackend(backend))
	}
	if assemblyDeclined(report, perr) {
		return nil, nil, false, nil
	}
	return schema, report, true, perr
}

// assemblyDeclined reports whether an assembly that read the documents report
// lists and returned perr is NO verdict, so its caller DECLINES: assembleCase's
// gate after its precondition reads, shared with assembleHints. It holds in two
// cases.
//
// The closure holds a document outside the producer's decidable subset
// (closureDecidable) and perr is not a grammarRejection. A silently skipped
// representation, in a root or in any composed document, could then make an
// accept vacuous, or leave a component missing that a reference is then charged
// src-resolve for.
//
// Or the rejection is one an unfollowed directive could have fabricated
// (fabricatedRejection). A directive that named no document is only half that
// hazard (#276): the missing components matter solely when something referred to
// them, which shows up as a failed parse whose src-resolve clause 1-3 error the
// spec does not attach to the missing document (§5.3). A parse that SUCCEEDED
// past an unfollowed directive fabricated nothing — §4.2.3 clause 2.4's "not an
// error ... the inclusion must not be performed" is exactly that outcome — so the
// case is still decided, and so is a parse that failed for a reason no
// unfollowed directive could have produced (#404).
func assemblyDeclined(report *parser.AssemblyReport, perr error) bool {
	if !closureDecidable(report) && !grammarRejection(report, perr) {
		return true
	}
	return fabricatedRejection(report, perr)
}

// grammarRejection reports whether perr is a rejection the out-of-subset
// constructs closureDecidable finds cannot have fabricated, so assembleCase may
// read it as "invalid" whatever the closure holds (#2076). It holds when perr is
// non-nil, carries no *xsderr.Error anywhere in its chain, and the assembly left
// no ·inter-schema-document reference· unfollowed.
//
// An error carrying no rule is this processor's spelling of a §2.4 clause 1
// sd-valid fault, restated at §5.1's first bullet (STYLE E2): a property of
// one document's element tree, which a construct the producer passes over
// cannot create, and which makes that document "in error" (§5.1) whatever the
// rest of the closure holds. The rule a SKIPPED construct can reach is
// src-resolve clauses 1-3 (§3.17.6.2), charged for a reference to the component
// the skip left missing (§5.3); it always comes as an *xsderr.Error, so the
// "no rule" test excludes it and everything else ruled with it.
//
// That direction claim quantifies over the decline triggers below, each of
// which either SKIPS the construct (the assembly maps it to nothing and charges
// nothing) or REJECTS it (with an unruled grammar fault of its own):
//
//   - schemaShapeDecidable: a top-level child outside the XSD namespace is
//     SKIPPED by the producer's topLevelDecls (#1036).
//   - overrideDecidable: a child outside the XSD namespace, a nameless child,
//     and a child of an element type §F.2 clause 1 does not match on are all
//     SKIPPED by parser's newOverrideSet.
//   - redefineDecidable: a child outside the XSD namespace is SKIPPED by
//     parser's newRedefineSet; a nameless child and one of an element type
//     §4.2.4's content model does not admit are REJECTED by it.
//   - simpleContentRestrictionDecidable, through silentlyDroppedFacetElement:
//     the plural <assertions> is SKIPPED, folding into no facet.
//   - modelGroupDecidable: every child it declines is REJECTED by parser's
//     unexpectedModelGroupChild.
//
// Every other predicate of the allowlist declines only through one of these.
// A skip leaves a component or a facet missing, and fewer components or facets
// reach no unruled charge. A rejection is the unruled charge itself.
//
// The no-unfollowed condition is #1201's. A resolver fault that is not
// loader.ErrNotFound records parser.UnfollowedLocationUnresolved and returns an
// unruled plain error that is no grammar fault (fabricatedRejection's GAP);
// declining every out-of-subset closure with an unfollowed directive keeps that
// fault declined here, where closureDecidable declined it before this lift.
//
// GAP(conformance): an unruled error is read as a grammar fault here exactly as
// step 4 of "The decidable shape" reads one for an in-subset closure, so the
// unruled LIMITATION parser's baseComponent charges — a derivation reaching
// its named base from inside that base's own content model, refused rather than
// mapped (#1883) — scores "invalid" in an out-of-subset closure too. No skipped
// construct produces it; it is a non-verdict this lift now also reads, and #1883
// owns its retirement.
func grammarRejection(report *parser.AssemblyReport, perr error) bool {
	if perr == nil || len(report.Unfollowed()) > 0 {
		return false
	}
	_, ruled := xsderr.RuleOf(perr)
	return !ruled
}

// The one location pinnedResolver serves from a committed copy, and that copy's
// package-relative path.
const (
	xlinkLocation = "http://www.w3.org/XML/2008/06/xlink.xsd"
	xlinkPinned   = "testdata/xlink/xlink.xsd"
)

// pinnedResolver is the resolver every assembleCase read goes through: dir, the
// case's own loader.Dir, for every location but xlinkLocation, which it serves
// from xlinkPinned and resolves to that on-disk path.
//
// The suite's own catalog schema, common/xsts.xsd, imports the XLink namespace
// from xlinkLocation, which no suite document supplies. src-import (§4.2.6.3)
// makes a failed fetch no error, and §4.2.6.2/§4.3.2 leave location means to the
// processor, so serving a copy of THAT document is sound; serving a subset of
// its declarations is not (#1973). xlinkPinned is the full document, byte for
// byte, CRLF line ends included: 9386 bytes, sha256
// c83df86c7fdc16eb9c862b83dfb53fc1b1a4bcafd6e1d1217199e0188b82f24a, taken from
// xmlresolver/xmlresolverdata src/data/www.w3.org/XML/2008/06/xlink.xsd and
// byte-identical to kusala9/s100-schemas
// schemas/S100/4.0.0/w3c/XML/2008/06/xlink.xsd. Its own import of the XML
// namespace does not resolve and need not: the parser supplies that namespace
// (parser/produce_xmlnamespace.go).
//
// A pinned copy that will not open answers loader.ErrNotFound, so the import is
// merely unfollowed and a rejection resting on it declines (fabricatedRejection)
// rather than deciding.
type pinnedResolver struct {
	dir loader.Resolver
}

// Resolve implements loader.Resolver.
func (r pinnedResolver) Resolve(namespace, location string) (io.ReadCloser, string, error) {
	if location != xlinkLocation {
		return r.dir.Resolve(namespace, location)
	}
	f, err := os.Open(xlinkPinned)
	if err != nil {
		return nil, "", fmt.Errorf("conformance: opening pinned %s for %q: %w: %w", xlinkPinned, location, loader.ErrNotFound, err)
	}
	return f, xlinkPinned, nil
}

// rootReadable checks, for the document a root names at location, the two
// preconditions parser.ParseSet answers with a plain Go error rather than a
// validity verdict, and returns the location the resolver resolved it to. ok is
// false — assembleCase DECLINES — when the document does not resolve, cannot be
// read, or has a root element that is not <schema>. It holds every root of the
// case to them alike, the first and every one extraRoots adds.
func rootReadable(resolver loader.Resolver, location string) (resolved string, ok bool) {
	rc, resolved, err := resolver.Resolve("", location)
	if err != nil {
		// Unreadable document: an honest recorded gap, not a validity verdict.
		return "", false
	}
	defer func() { _ = rc.Close() }() // read-only handle: close error cannot affect the verdict
	root, err := parser.ReadDocument(location, rc)
	if err != nil {
		// A ReadDocument error is DECLINED, never treated as an observed-invalid
		// verdict. The error does not distinguish a genuine XML well-formedness
		// fault from a reader LIMITATION (step 1 of "The decidable shape" at the
		// top of this file), so an "invalid" verdict here could be fabricated for a
		// well-formed document. Declining on ANY ReadDocument error keeps the
		// lane's verdicts honest; a COMPOSED document's read is narrower
		// (fabricatedRejection).
		return "", false
	}
	// §3.17.2 does not require <schema> to be the document root, so a non-schema
	// root is a Parse precondition fault (a plain Go error, not a
	// sch-props-correct rejection), not decidable for this lane — decline.
	return resolved, root.IsSchema()
}

// ruleSrcResolve is the rule the producer charges for a reference the assembled
// schema holds no component for (QName resolution, §3.17.6.2 clauses 1-3). It is
// matched in its BARE catalog form, the clause living in the message text rather
// than in a dotted rule ID — the instance lane's decidableRules idiom.
const ruleSrcResolve xsderr.Rule = "src-resolve"

// fabricatedRejection reports whether perr is a rejection this assembly's own
// unfollowed directives could have FABRICATED: an "invalid" resting on a
// document this processor did not read rather than on the document under test.
// That is the one direction able to corrupt the ratchet, and the whole reason
// assembleCase declines rather than deciding (#276).
//
// The correlation it draws is an ENGINEERING HEURISTIC over the rules THIS
// producer charges, not a spec rule, and there is no spec rule to have: §3.17.6.2
// src-resolve tests a QName against the composed schema's aggregate properties
// and back-references no directive, §4.2.3 clause 2.4 instructs retention of
// unresolved QName references rather than attribution of blame, and §5.3
// describes the ·absent· consequence generically. Nothing in either document
// correlates a specific unfollowed directive with a specific downstream failure,
// so the reasoning below is about this producer's charge sites and is revisited
// whenever they change (#404).
//
// Two directive reasons carry a hazard, and they are different hazards:
//
//   - A location that did not resolve, or a directive naming none at all
//     (parser.UnfollowedLocationUnresolved, parser.UnfollowedNoLocation), leaves
//     the assembly SHORT of components. Having FEWER components surfaces at
//     exactly one rule: the producer charges src-resolve for every reference
//     whose target the schema does not hold, and no other rule it charges can be
//     reached by a shortfall — sch-props-correct clause 2 needs two components to
//     collide, cos-nonambig needs particles that already resolved,
//     src-include/src-import/src-redefine are the directive's OWN faults and so
//     are causes of the shortfall rather than consequences of it, and the §2.4
//     clause 1 grammar faults are properties of one document's element tree.
//   - A location that resolved to a document that could not be READ
//     (parser.UnfollowedUnreadable) is charged src-include/src-import/
//     src-redefine "not well-formed", and that rule alone cannot distinguish a
//     genuine well-formedness fault from a reader LIMITATION. The chain under
//     it can: wellFormednessFault finds a definite fault, and that is a
//     resource that resolved but is no well-formed information set, which
//     src-include clause 1, src-import clause 2, src-redefine clause 2 and
//     src-override clause 1 each make an error (MS-Schema schB4, schE5, schH5,
//     #2067), so it decides. Any other failure alongside such a directive
//     declines, on step 1's reasoning for an unreadable ROOT.
//
// Every other reason fabricates nothing, parser.UnfollowedNoSchemaLocation most
// clearly: §4.2.1 makes that attribute mandatory ("not hints: conforming
// processors must attempt to de-reference"), so the assembly rejects the
// DIRECTIVE ITSELF against the §2.4 clause 1 grammar (STYLE E2's unruled plain
// error) — a genuine rejection of the document under test, naming no missing
// component at all. Reading it as a verdict is what the lane already does for
// every other grammar fault.
//
// The shortfall arm exempts one src-resolve charge no shortfall can produce:
// clause 4 (cl.qnr.nsdeclared), a reference into a namespace its own document
// does not license. Clause 4.2.2 reads the containing document's <import>
// namespace attributes, never whether an import was followed, and §4.2.6.1
// says references into a namespace not imported "are *not* handled as if they
// referred to "missing components"", so a clause 4 charge is the document's own
// fault whatever its directives fetched. The producer charges it eagerly at the
// reference (parser's licensedNamespace), before finalize charges clauses 1-3,
// so it is the error a document holding both returns. The clause is read from
// the error's Msg text (srcResolveClause4): xsderr.Error carries the bare rule
// and no clause, and a structured marker would be an export (#2054). The read
// arm does not take the exemption — an unreadable document that is not a
// wellFormednessFault declines on ANY failure, clause 4 included.
//
// Otherwise the shortfall arm stays SCAN-scoped: ANY unfollowed directive plus
// any other src-resolve failure declines, without asking whether that
// directive is the one that would have supplied the component the reference
// wanted. Narrowing it needs the reference's namespace on the error and the
// directive's namespace on the report, and neither xsderr.Error nor
// parser.UnfollowedDirective carries one — a data-shape change, where this is
// not (#404).
//
// GAP(conformance): one non-verdict failure is invisible to BOTH arms, and the
// caller then scores it "invalid". parser/parse.go's fetch records
// parser.UnfollowedLocationUnresolved and returns an UNRULED plain error for a
// resolver fault that is not loader.ErrNotFound — a permission or transport
// error. The reason is not parser.UnfollowedUnreadable, so the read arm misses
// it; the error is no *xsderr.Error, so it carries no src-resolve rule and the
// shortfall arm misses it too. The two arms above assume every non-verdict a
// composed document can produce is a READ failure; a RESOLVE failure that is not
// mere absence is the one that is not. It cannot be told from the shape this
// predicate reads: parser.UnfollowedLocationUnresolved with an unruled error is
// also annotB020, whose <include> names a document that plain does not exist and
// whose unruled error is an unrelated §2.4 clause 1 grammar fault that must
// DECIDE. #1201 owns the fix and states why it belongs in parser rather than
// here. Zero suite cases reach that exit today, so this predicate's measured
// behavior over the whole corpus is unaffected (#404).
func fabricatedRejection(report *parser.AssemblyReport, perr error) bool {
	if perr == nil || len(report.Unfollowed()) == 0 {
		return false
	}
	for _, u := range report.Unfollowed() {
		if u.Reason == parser.UnfollowedUnreadable && !wellFormednessFault(perr) {
			return true
		}
	}
	var e *xsderr.Error
	if !errors.As(perr, &e) {
		return false
	}
	return e.Rule == ruleSrcResolve && !strings.Contains(e.Msg, srcResolveClause4)
}

// wellFormednessFault reports whether perr carries an XML well-formedness fault
// no reader limitation can produce: the first xsderr.RuleXMLWellFormed
// *xsderr.Error in its chain wraps no cause and is not the unbound-prefix
// charge (unboundPrefixCharge): parser/xmltree's Contract makes such an error a
// definite fault whatever the document's DTD declares. A chain whose first
// xml-wf charge wraps a cause, or that holds none, is no fault here.
// parser.ReadDocument keeps that shape: it passes the reader's errors on as they
// are, wraps a cause around an xml:base it cannot compose, and charges a
// document with no root element with none.
//
// GAP(parser): a composed or hinted schema document whose prefix is genuinely
// unbound — no DTD could have bound it — is declined here rather than decided
// invalid, because the charge does not say whether the document carried a
// DTD. Retiring it needs the reader to apply internal-subset attribute
// defaults (#2073).
//
// The shape is read rather than a marker because a structured one would be an
// export (#2067).
func wellFormednessFault(perr error) bool {
	e, ok := firstWellFormednessCharge(perr)
	return ok && e.Err == nil && !strings.HasPrefix(e.Msg, unboundPrefixCharge)
}

// unboundPrefix reports whether the first xsderr.RuleXMLWellFormed charge in
// perr's chain is the unbound-prefix one: the arm wellFormednessFault's
// GAP(parser) declines, which the instance lane names as
// refuseUnboundPrefix.
func unboundPrefix(perr error) bool {
	e, ok := firstWellFormednessCharge(perr)
	return ok && strings.HasPrefix(e.Msg, unboundPrefixCharge)
}

// firstWellFormednessCharge is the first *xsderr.Error in perr's chain that
// carries xsderr.RuleXMLWellFormed, and false where it holds none.
func firstWellFormednessCharge(perr error) (*xsderr.Error, bool) {
	var e *xsderr.Error
	for err := perr; errors.As(err, &e); err = e.Err {
		if e.Rule == xsderr.RuleXMLWellFormed {
			return e, true
		}
	}
	return nil, false
}

// unboundPrefixCharge opens every message parser/xmltree's reader writes for a
// prefix no in-scope namespace declaration binds — on an element, an end tag or
// an attribute — and no other xml-wf charge it makes. wellFormednessFault reads
// it to keep that charge from deciding; the <!ATTLIST row of
// TestSchemaExecutorReadArmSeparatesFaultFromLimitation drives the real parser,
// so rewording that message fails it.
const unboundPrefixCharge = "unbound namespace prefix"

// srcResolveClause4 is the text parser's licensedNamespace writes into every
// src-resolve clause 4 charge it makes — "src-resolve clause 4.1" for an
// ·absent· namespace, "src-resolve clause 4.2" for a present one — and no other
// src-resolve message carries. fabricatedRejection reads it to tell a clause 4
// charge from every other src-resolve charge (#2054);
// TestSchemaExecutorDecidesUnlicensedNamespaceBesideShortfall drives the real
// parser, so rewording that message fails it.
const srcResolveClause4 = "src-resolve clause 4."

// extraRoots returns a parser.RootAt root for every FURTHER document the case
// declares beyond doc that the assembly rooted at doc did NOT consume, in
// declared order; ok is false when one of them fails rootReadable.
//
// A schemaTest may list several <schemaDocument> children, and the suite's own
// catalog schema defines that as "run as if the schema documents given were
// loaded one by one, in order": the case is the SET, not any member of it. A
// declared document the first one's own <include>/<override>/<import>/<redefine>
// already names is in the assembly already, so it yields no root: entering it
// again would read a <redefine> or <override> target plainly a second time and
// restore the definitions the directive replaced (#1840). Every other declared
// document is a root of its own, which assembleCase enters after doc.
//
// Each extra document is resolved through the SAME resolver the parse used, under
// a location string relative to the same root directory, so the resolved identity
// compared against the report is in the report's own format (closureReached), and
// the root is named by that location string, as doc's own is. ok is false when
// one fails rootReadable, reached or not: each is a declared document of the
// case, held to the preconditions doc is held to, since ParseSet would answer an
// unreadable or non-<schema> root with a plain error the lane would misread as
// "invalid".
func extraRoots(report *parser.AssemblyReport, resolver loader.Resolver, doc string, extraDocs []string) (roots []parser.Root, ok bool) {
	dir := filepath.Dir(doc)
	for _, extra := range extraDocs {
		rel, err := filepath.Rel(dir, extra)
		if err != nil {
			return nil, false
		}
		location := filepath.ToSlash(rel)
		resolved, ok := rootReadable(resolver, location)
		if !ok {
			return nil, false
		}
		if closureReached(report, resolved) {
			continue
		}
		roots = append(roots, parser.RootAt(location))
	}
	return roots, true
}

// decideAgreement Passes iff the observed validity agrees with the suite's
// declared XSD 1.1 expectation. It is the ONE closing comparison every deciding
// executor of the schema and instance lanes reaches, and every decline returns
// before it — which is exactly what the decline census probes (declines.go).
func decideAgreement(observed, expected bool) Status {
	if observed == expected {
		return Pass()
	}
	return Fail()
}

// schemaShapeDecidable reports whether this document contributes no false-accept
// hazard to the assembly that read it: either it is UNCONDITIONALLY REJECTED
// (holdsMisplacedNotation, or the default arm's own grammar verdict below), or
// every top-level child lies within the producer's decidable subset (the step-3
// allowlist documented above). A single out-of-subset child declines an ACCEPTED
// assembly, and a rejected one whose error is not a grammarRejection, since
// Produce would silently skip it — an accept made vacuous, or a component
// missing that a reference is then charged src-resolve for — rather than decide
// it genuinely. A grammarRejection is decided whatever this answers
// (assemblyDeclined, #2076), so false here is not a decline on its own.
//
// The misplaced-<notation> short-circuit comes FIRST because it makes every
// other question moot: a document carrying one is rejected before any producer
// dispatches, so no shape below can be silently skipped into a vacuous accept
// and no shape below can be rejected for a limitation instead. The remaining
// allowlist runs unchanged on every other document, this one included once the
// notation is moved into a slot the grammar admits.
func schemaShapeDecidable(doc *parser.Document) bool {
	if holdsMisplacedNotation(doc.Root()) {
		return true
	}
	for _, child := range doc.Root().Children() {
		el, ok := child.(*parser.Element)
		if !ok {
			continue
		}
		name := el.Name()
		if name.Space() != xsd.XMLSchemaNS {
			return false
		}
		switch name.Local() {
		case "annotation":
			// Harmless, always allowed.
		case "include", "import":
			// Admitted (#242 for include, #182 for import). Neither contributes a
			// component of its own — <include>'s content model is (annotation?) and
			// <import>'s likewise — so there is nothing here for the producer to
			// silently skip. What each POINTS AT is the thing that could be skipped,
			// and that is gated by closureDecidable, which runs this same function
			// over every document the assembly reported reading. src-include's
			// (§4.2.3) and src-import's (§4.2.6.2) own clauses are then enforced
			// genuinely by the assembly. A directive that yields no D2 at all — an
			// <import> with no schemaLocation, or either directive with one that does
			// not resolve — is reported as unfollowed, not judged here, and declines
			// the case only alongside a failed parse: see
			// parser.AssemblyReport.Unfollowed and assembleCase.
		case "element":
			if !elementDecidable(el) {
				return false
			}
		case "attribute":
			if !attributeDecidable(el) {
				return false
			}
		case "simpleType":
			if !simpleTypeDecidable(el) {
				return false
			}
		case "complexType":
			if !complexTypeDecidable(el) {
				return false
			}
		case "group":
			if !groupDecidable(el) {
				return false
			}
		case "attributeGroup":
			if !attributeGroupDecidable(el) {
				return false
			}
		case "notation":
			// Produced (#178) with no undecidable sub-shape: <notation>'s content is
			// (annotation?) and its whole property tableau is settled at produce time
			// (n-props-correct §3.14.6 rejects both identifiers absent), so it is
			// always admitted, like annotation.
		case "override":
			// Admitted (#183). The document it points at is gated by the closure
			// walk, exactly as an <include>'s target is; its own children become top-
			// level declarations of THAT document (§4.2.5, §F.2 clause 1) and are
			// gated here.
			if !overrideDecidable(el) {
				return false
			}
		case "defaultOpenContent":
			// Read (#230) by every complex type of this document that has no
			// <openContent> of its own (§3.4.2.3.3 clause 5.2), so it is no longer
			// silently skipped — and admitted in EVERY shape as of #352, malformed
			// ones included. The two the schema for schema documents forbids (no
			// <any> child; a mode outside interleave|suffix) are now rejected by the
			// producer's pre-produce pass for any document that declares one, so both
			// verdicts are real and unconditional. The narrower arm #230 wrote
			// declined them only because that rejection used to be lazy — reachable
			// solely through a complex type that got as far as clause 5.2 — which
			// would have made the verdict depend on unrelated content.
		case "redefine":
			// Admitted (#286, all four redefinable kinds as of #505). Like
			// <override> the document it points at is gated by the closure walk,
			// and its own children are gated here, each by the predicate its own
			// element type is gated by anywhere else.
			if !redefineDecidable(el) {
				return false
			}
		default:
			// Any other local name: REJECTED by the producer (#1380), on the same
			// permanence argument the misplaced-<notation> short-circuit above
			// stands on. <schema>'s content model (xmlschema11-1.md:4554) admits
			// the thirteen names cased above and no other, with no wildcard arm and
			// no lax position, so rejectUnmappedTopLevel charges every name that
			// reaches here — a §5.1 first-bullet grammar verdict the spec licenses,
			// not a limitation awaiting a slice, so the "invalid" it buys cannot
			// flip. It was declined until then, when the producer skipped the child
			// and a nil verdict would have been vacuous.
		}
	}
	return true
}

// holdsMisplacedNotation reports whether the subtree under parent holds an
// <xs:notation> the schema for schema documents admits nowhere: <notation> is
// declared in the xs:schemaTop group alone (xmlschema11-1.md:4462), and only
// <schema> (:4562) and <override> (:5577) reference that group. <redefine> is
// not a third — its own model reaches xs:redefinable (:5558), which omits
// <notation> (:4465-4477) — so a <notation> child of one is misplaced too, by a
// different production and the same umbrella fault. The fault carries NO rule
// ID: §3.14.3 and §3.14.4 both answer "None as such" (:3409, :3413), so it
// stands on §5.1's first, unnumbered bullet (:4296) directly, and charging a
// src-* verdict here would fabricate one (STYLE E2).
//
// A document holding one is DECIDABLE whatever else it holds (#945), because the
// producer rejects it GENUINELY and UNCONDITIONALLY: parser's rejectS4SFaults
// walks the whole document before any producer dispatches, so no position can
// reach around the charge — and a <notation> under <redefine> is charged earlier
// still, at discovery, by <redefine>'s own content-model guard. Both are grammar
// verdicts the spec licenses, not limitations awaiting a slice, so the "invalid"
// they buy cannot flip once some later slice lands. That permanence is the whole
// admission argument. A non-nil parse error decides an out-of-subset closure on
// its own only as a grammarRejection, which also requires that no directive went
// unfollowed (#2076); this short-circuit admits the document beside an
// unfollowed directive too, leaving that to fabricatedRejection.
//
// The walk MIRRORS rejectS4SFaults' — XSD-namespace elements only, never
// descending into <appinfo> or <documentation>, whose <xs:any
// processContents="lax"> content (:5727, :5740) makes an element merely NAMED
// {XMLSchemaNS}notation ordinary content that no guard governs. The two must
// agree exactly: a shape this predicate claimed and that walk did not would be
// admitted on a rejection that never comes (MS-Notations2006-07-15/notatF009,
// suite-VALID, is precisely such a document).
//
// parent is the element whose children are examined, so the top-level call
// passes the <schema> root and every notation child of it is legal.
func holdsMisplacedNotation(parent *parser.Element) bool {
	name := parent.Name()
	legal := name.Space() == xsd.XMLSchemaNS && (name.Local() == "schema" || name.Local() == "override")
	for _, child := range parent.Children() {
		el, ok := child.(*parser.Element)
		if !ok || el.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		local := el.Name().Local()
		if local == "appinfo" || local == "documentation" {
			continue
		}
		if local == "notation" && !legal {
			return true
		}
		if holdsMisplacedNotation(el) {
			return true
		}
	}
	return false
}

// redefineDecidable reports whether every child of an <xs:redefine> (§4.2.4) is
// a definition the producer decides genuinely once the redefinition has been
// applied (#286). A redefining child becomes a top-level definition of the
// REDEFINING document (src-expredef clause 1.2 / clause 2, §4.2.4 clause 4.1.1),
// so each is gated by the very predicate a top-level child of any document is
// gated by — with one subtraction.
//
// There is no subtraction as of #505: a redefining <complexType> is produced,
// paired with the {name}-·absent· original src-expredef clause 1.1 defines, so it
// is gated by complexTypeDecidable exactly as a top-level one is. The pairing
// puts the original in no by-name index, which narrows nothing this predicate can
// see — every clause a redefinition turns on reads the base COMPONENT, and
// finalize reaches it through the {base type definition} slot. What it does NOT
// reach is the original itself: Phase D's walk enters an owned {base type
// definition} to reach the declarations inside it but charges the base no
// verdict of its own (GAP(xsd), xsd/complexderivation.go, #584). That is an
// under-rejection on a component the redefinition's own verdict already covers
// structurally, the same safe direction as the residue below.
//
// A child with no name= declines rather than being waved through: the pairing is
// keyed on (element type, name), and src-expredef requires a top-level definition
// item of that name and kind in the redefined document, so a nameless child is
// reported as a grammar fault rather than decided.
//
// No fail-open src-redefine clause remains inside an admitted case: clause 7.2.2
// is charged at finalize as of #503 and clause 6.2.2 as of #504, each over the
// pairing the producer records between the redefinition and the original in S2.
// The document the <redefine> POINTS AT is gated by closureDecidable, exactly as
// an <include>'s target is.
//
// ONE fail-CLOSED residue is kept deliberately: a CHAINED <redefine> of a
// <group>/<attributeGroup> — the redefined document redefining a document of its
// own for the same name — is refused under src-expredef's closing requirement, so
// such a case observes "invalid" whatever the suite says. The producer carries the
// marker (parser/redefine.go's chainedOriginal) and #744 owns its retirement,
// which needs an xsd entry point for a redefinition that contributes no component:
// charging both clauses now is NOT enough, because the two suite cases in that
// shape, MS-Schema2006-07-15/schL10 and /schM5, are each invalid at the MIDDLE
// level of their chain, which is the level clause 4.1.2 withholds. So the residue
// is still the whole reason those two agree with the suite. A chained <simpleType>
// or <complexType> composes and is decided genuinely
// (MS-Additional2006-07-15/addB007).
func redefineDecidable(el *parser.Element) bool {
	for _, child := range el.Children() {
		c, ok := child.(*parser.Element)
		if !ok {
			continue
		}
		if c.Name().Space() != xsd.XMLSchemaNS {
			return false
		}
		if c.Name().Local() == "annotation" {
			continue
		}
		if !hasAttr(c, "name") {
			return false
		}
		switch c.Name().Local() {
		case "simpleType":
			if !simpleTypeDecidable(c) {
				return false
			}
		case "complexType":
			if !complexTypeDecidable(c) {
				return false
			}
		case "group":
			if !groupDecidable(c) {
				return false
			}
		case "attributeGroup":
			if !attributeGroupDecidable(c) {
				return false
			}
		default:
			// Every element type §4.2.4's content model does not admit at all.
			return false
		}
	}
	return true
}

// overrideDecidable reports whether every child of an <xs:override> (§4.2.5) is a
// source declaration the producer decides genuinely once ·override
// pre-processing· has substituted it into the overridden document (#183). A child
// that overrides something "will become a top-level declaration" of that document
// (§3.1.2's note on <override> children), so each is gated by the very predicate
// a top-level child of any document is gated by — the substituted declaration is
// produced by the overridden document's producer, through the same code path.
//
// A child with no name= declines rather than being waved through: §F.2 clause 1
// matches on (element type, name), so the parser can only ignore such a child,
// and ignoring a declaration is precisely the vacuous shape this allowlist
// exists to refuse. Any other element type declines for the same reason.
//
// The document the <override> POINTS AT is NOT gated here; that is the closure
// gate's job (closureDecidable), which runs schemaShapeDecidable over it exactly
// as it does for an <include>'s target, both being documents the assembly
// reported reading.
func overrideDecidable(el *parser.Element) bool {
	for _, child := range el.Children() {
		c, ok := child.(*parser.Element)
		if !ok {
			continue
		}
		if c.Name().Space() != xsd.XMLSchemaNS {
			return false
		}
		if c.Name().Local() == "annotation" {
			continue
		}
		if !hasAttr(c, "name") {
			return false
		}
		switch c.Name().Local() {
		case "notation":
			// Always decidable, as at top level.
		case "element":
			if !elementDecidable(c) {
				return false
			}
		case "attribute":
			if !attributeDecidable(c) {
				return false
			}
		case "simpleType":
			if !simpleTypeDecidable(c) {
				return false
			}
		case "complexType":
			if !complexTypeDecidable(c) {
				return false
			}
		case "group":
			if !groupDecidable(c) {
				return false
			}
		case "attributeGroup":
			if !attributeGroupDecidable(c) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// elementDecidable reports whether a TOP-LEVEL <element> is in the form the
// producer decides genuinely. A bare element (no type=) defaults to xs:anyType
// (§3.3.2.1 case 4), which the producer seeds as a Complex Type Definition
// (§3.4.7), so it resolves at finalize and is decided genuinely — type= is not
// required.
//
// Both inline type children ARE produced (§3.3.2.1 dcl.elt.common clause 1) and
// each is admitted when the anonymous type it maps to is itself decidable: a
// <complexType> through complexTypeDecidable, the very predicate a NAMED one
// takes (#340, widened to the explicit-content forms by #1126 — that function's
// doc comment carries why anonymity narrows nothing), and a <simpleType>
// through simpleTypeDecidable, exactly as localElementDecidable gates the local
// form (#229/#442). This function and localElementDecidable are therefore no longer
// asymmetric about tier 1 — the tier is a COMMON mapping rule and the producer
// now maps it on both paths, so a difference here would be the fabrication.
//
// This gate does NOT need a condition for a substitutionGroup= whose head is
// typed by an inline <simpleType>: the producer maps that clause-3 shape to an
// xsd.SubstitutionGroupHeadTypeRef naming the head, like the inline
// <complexType> head beside it, so no member of such a head reaches a limitation
// error (#442; before it, the head's own decline here is what kept the shape out
// of reach).
//
// Its <alternative> children are gated by alternativeTypesDecidable, on the same
// terms and through the same two functions.
//
// Its <unique>/<key>/<keyref> children impose no condition of their own: #178
// produced the name= form and #240 the ref= form, so BOTH are mapped and both
// src-identity-constraint (§3.11.3) and c-props-correct (§3.11.6.1) rejections on
// them are genuine.
func elementDecidable(el *parser.Element) bool {
	if st := childXSD(el, "simpleType"); st != nil && !simpleTypeDecidable(st) {
		return false
	}
	if inline := childXSD(el, "complexType"); inline != nil && !complexTypeDecidable(inline) {
		return false
	}
	return alternativeTypesDecidable(el)
}

// alternativeTypesDecidable reports whether every type an <element>'s
// <alternative> children own is decidable. §3.12.2 declare-ta's inline arm maps
// a <complexType> or <simpleType> child of an <alternative> to a type definition
// the element declaration OWNS (#851); an <alternative> on the type= arm imposes
// no condition, its reference being resolved at finalize like any other with a
// genuine src-resolve clause 1.1 failure.
//
// The <simpleType> child goes through simpleTypeDecidable, the same gate an
// element's own inline one takes.
//
// The <complexType> child goes through complexTypeDecidable, which is where the
// anonymity residual every inline complex type carries is stated once. This site
// took that predicate from the start (#851) and the two element paths joined it
// at #1126; the three no longer differ, so no argument for an asymmetry is owed
// here any more.
//
// It serves both element paths (STYLE T4): §3.3.2.1's {type table} row is a
// COMMON mapping rule, so a top-level and a local <element> carry the same
// children through the same producer.
func alternativeTypesDecidable(el *parser.Element) bool {
	for _, alt := range childrenXSD(el, "alternative") {
		if ct := childXSD(alt, "complexType"); ct != nil && !complexTypeDecidable(ct) {
			return false
		}
		if st := childXSD(alt, "simpleType"); st != nil && !simpleTypeDecidable(st) {
			return false
		}
	}
	return true
}

// complexTypeDecidable reports whether a <complexType> — top-level, ANONYMOUS,
// or a nested derivation reached through <complexContent>/<simpleContent> —
// lies within the producer's decidable subset — the shapes it fully builds AND
// finalize fully judges, so any Produce error on it is a REAL structural violation
// (src-ct/cos-all-limited/src-wildcard/src-attribute/p-props-correct/src-resolve)
// and its silence is a real absence of one. Finalize judges every shape here
// alike with ONE exception, which is anonymity's and is stated in the anonymity
// paragraphs below. It declines:
//
//   - every shape the producer declines with a plain "not yet produced"
//     limitation error — today NONE at this level. The bare <group>/
//     <attributeGroup> lacking a ref was the last one named here and is no
//     longer declined: the producer REJECTS it as the s4s attribute-use fault it
//     is (#1182), so the verdict is genuine and the shape is admitted.
//     <simpleContent> <restriction> was the other one and is no longer declined:
//     #909 built §3.4.2.2 cases 1-2, so the shape goes through
//     simpleContentRestrictionDecidable, which admits what that mapping reads
//     plus every child the s4s order check rejects outright (#1181), leaving the
//     plural <assertions> as its one decline. An inline anonymous <simpleType>
//     on a local element or attribute IS produced (#229) and an inline anonymous
//     <complexType> on a local OR global element is produced too (#340); neither
//     declines here — see localElementDecidable/elementDecidable, which route
//     the anonymous complex-type shape into THIS function, and the anonymity
//     paragraphs below. An <openContent> is produced as of #230 (§3.4.2.3.3
//     clauses 5-6) and is admitted by contentDecidable wherever the schema for
//     schema documents allows one; a MISPLACED one — beside
//     <simpleContent>/<complexContent>, or directly under <complexContent> — is
//     rejected by the producer as the grammar fault it is, so it needs no
//     decline of its own.
//
// A <simpleContent>/<complexContent> carrying NEITHER alternant is ADMITTED
// (#868). §3.4.2.2 and §3.4.2.3 each require one of them, and the producer
// rejects a document that carries neither; that the rejection is a grammar fault
// about the source item rather than a rule verdict does not make it a
// limitation, so it needs no decline any more than a misplaced <openContent>
// does.
//
// Both EXTENSION forms are ADMITTED as of #336. The producer builds them (#228),
// cos-ct-extends (§3.4.6.2) judges them (#264), and its case-1 clauses read only
// folds that are now done — 1.2 over §3.4.2.4 clause 3's {attribute uses}
// (#401), 1.3 over §3.4.2.5 clause 2.2's {attribute wildcard} (#265) and 1.7
// over §3.4.2.1 clause 1's {assertions} (#346). One of its clauses still cannot
// reject: case 1's 1.5 (two-step derivability), proven only for a pure-extension
// chain and otherwise accepted unconditionally, an engine approximation
// (GAP(xsd), xsd/complexextension.go, #392). Case 2 is complete. The file comment
// above carries the full inventory. That clause is an UNDER-rejection, the same
// safe direction as cos-nonambig and ct-props-correct clause 4 already are on the
// admitted <restriction> path, so it bounds what this lane can claim rather than
// letting it fabricate an "invalid".
//
// A <group ref>/<attributeGroup ref> IS produced (#177) and admitted: its target
// resolves genuinely at finalize (or fails src-resolve). Real structural
// violations the producer DOES reject (a nested <all>, a mixed
// mismatch, a both-namespace-forms wildcard, a bad occurrence) are NOT declined:
// admitting them is safe because the producer's rejection is the right reason.
//
// ANONYMITY NARROWS NOTHING HERE, and until #1126 a separate predicate said
// otherwise by admitting the implicit-content form alone. §3.3.2.1 dcl.elt.common
// clause 1 is a COMMON mapping rule, so an <element>'s inline <complexType> maps
// through §3.4.2 exactly as a top-level named one does; §3.4.2.1 makes {name},
// {target namespace} and {context} the only properties anonymity fixes; §3.4.2.4
// opens "This mapping rule is the same for all complex type definitions" and
// §3.4.2.5 says it of its own; and no clause of §3.4.2.2, §3.4.2.3, §3.4.6.1
// (ct-props-correct), §3.4.6.2 (cos-ct-extends) or §3.4.6.3
// (derivation-ok-restriction) reads {name} or {context} at all. A gate narrower
// for the anonymous case would therefore state a distinction the spec does not
// make. All three sites that own an anonymous complex type reach it through this
// one function — elementDecidable and localElementDecidable for §3.3.2.1's tier
// 1 at every nesting depth (contentDecidable → modelGroupDecidable →
// localElementDecidable comes back here), alternativeTypesDecidable for
// §3.12.2's inline arm, which took this predicate from the start (#851).
//
// The residual anonymity used to leave — every finalize pass quantifying over
// {type definitions} reaching NO verdict for a type in no such set (§3.17.2
// scopes the property to the <complexType> children OF <schema>;
// xsd/typedefinition.go records why registering one would be worse) — is GONE
// for both slots this predicate reaches. Since #438 checkContentModelsUnambiguous
// (cos-nonambig), checkElementDeclarationsConsistent (cos-element-consistent)
// and checkComplexDerivations with the ct-props-correct clause 4 and
// cos-ct-extends clause 1.2 checks it drives all walk the owning slots
// (xsd/complextypewalk.go), so an anonymous type here is charged what a named
// one is. What is left uncharged is the redefine ORIGINAL alone (#584, and the
// GAP marker on parser's produceComplexType), which no <element> and no
// <alternative> can seat. That remainder is an UNDER-rejection: this lane can
// report "valid" for a schema a complete processor rejects, never "invalid" for
// a valid one. Nothing an INSTANCE case turns on is approximate on
// either newly admitted alternant — {content type} is computed at PRODUCE time
// (produceSimpleContent for §3.4.2.2, produceComplexContent for §3.4.2.3.3,
// the extension merge of clause 4.2 through xsd.ExtensionContentType), and
// §3.4.2.4 clause 3 and §3.4.2.5 clause 2 reach an owned anonymous type through
// the slot that owns it (xsd/ownedtypefold.go, #414) — so cvc-complex-type
// clause 2 is charged against the spec's own {attribute uses}/{attribute
// wildcard} and clause 3 against the spec's own {simple type definition}.
//
// What admits these shapes is that under-rejection, and NOT §3.4.6.3's
// provisional-accept carve-out, which is scoped to a T whose {content
// type}.{particle}.{term}.{compositor} is all and is keyed to the content
// model's shape rather than to whether T is named.
func complexTypeDecidable(el *parser.Element) bool {
	if sc := childXSD(el, "simpleContent"); sc != nil {
		if ext := childXSD(sc, "extension"); ext != nil {
			return simpleContentExtensionDecidable(ext)
		}
		if r := childXSD(sc, "restriction"); r != nil {
			return simpleContentRestrictionDecidable(r)
		}
		// Neither alternant is a grammar fault the producer rejects genuinely, so
		// it is admitted — see above.
		return true
	}
	if cc := childXSD(el, "complexContent"); cc != nil {
		derivation := childXSD(cc, "restriction")
		if derivation == nil {
			derivation = childXSD(cc, "extension")
		}
		if derivation == nil {
			return true // neither alternant — a genuine rejection, see above
		}
		return contentDecidable(derivation)
	}
	return contentDecidable(el)
}

// simpleContentExtensionDecidable reports whether a <simpleContent> <extension>
// is in the shape the producer decides genuinely. xs:simpleExtensionType
// (xmlschema11-1.md:1697) is (annotation?, ((attribute | attributeGroup)*,
// anyAttribute?), assert*) — the one derivation alternant with no structural
// position at all — so attrDeclsDecidable's vocabulary is the whole of this
// content model and the attribute children the producer folds in (§3.4.2.4) are
// the only ones with a shape to judge.
//
// Every OTHER XSD-namespace child is ADMITTED because the producer REJECTS it
// (#1181), never because it is harmless: checkS4SChildOrder walks this element
// against that model and charges §2.4 clause 1 sd-valid (:615, restated at
// §5.1's first bullet, :4296) for a name no position admits — a
// <sequence>/<choice>/<all>, a <group>, a stray <simpleContent>/<complexContent>
// alike — and an <openContent> is charged earlier still by misplacedOpenContent.
// §3.4.2.2 cases 3-5 do compute {content type} from the resolved base alone, but
// no document carrying such a child reaches that tableau, so NOTHING is dropped
// in silence at this position.
func simpleContentExtensionDecidable(ext *parser.Element) bool {
	for _, child := range ext.Children() {
		el, ok := child.(*parser.Element)
		if !ok || el.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		if decidable, shared := attrDeclsDecidable(el); shared && !decidable {
			return false
		}
	}
	return true
}

// simpleContentRestrictionDecidable reports whether a <simpleContent>
// <restriction> is in the shape the producer decides genuinely (#909). Its
// content model is xs:simpleRestrictionType — (annotation?, (simpleType?,
// facet*)?, ((attribute | attributeGroup)*, anyAttribute?), assert*) — and every
// item of it is mapped: the facet children and the optional inline <simpleType>
// into the anonymous simple type §3.4.2.2 cases 1-2 synthesize, the attribute
// children into {attribute uses}/{attribute wildcard}, the trailing <assert>
// children into the CTD's own {assertions}. So the shared tail goes through
// attrDeclsDecidable unchanged and a facet child is admitted outright, as it
// already is under <simpleType> <restriction> (simpleTypeDecidable) — the two
// sites map facets through the very same restrictionFacets.
//
// A name OUTSIDE that content model is ADMITTED, and the two mechanisms behind
// that must not be conflated (#1181). A model group, a <group>, a stray
// <simpleContent>/<complexContent> or any other name no position of
// xs:simpleRestrictionType admits is REJECTED: checkS4SChildOrder charges §2.4
// clause 1 sd-valid (:615, restated at §5.1's first bullet, :4296) over it, so
// the verdict is the producer's own. The PLURAL <assertions> is the one child
// that really is DROPPED IN SILENCE — it fills the facet position and folds into
// no facet — and it alone still declines here, through
// silentlyDroppedFacetElement. An inline <simpleType> outside the produced
// simple-type subset moves the decline inward through simpleTypeDecidable, as
// every other inline-type site does.
//
// Nothing here narrows for the tableau's case 5 (a base with element-only or
// empty content, or a simple type base, under <restriction>), although that case
// drops the facet children and any inline <simpleType> by the tableau's own
// instruction: every case-5 restriction shape is REJECTED at finalize —
// ct-props-correct clause 2 for a simple type base, derivation-ok-restriction
// clause 2 for the rest — so no verdict rides on what the mapping dropped. Since
// #438 that holds for an anonymous complex type owned by an <alternative> too:
// Phase D reaches it through the owning slot (xsd/complextypewalk.go) and makes
// the same rejection.
func simpleContentRestrictionDecidable(restriction *parser.Element) bool {
	for _, child := range restriction.Children() {
		el, ok := child.(*parser.Element)
		if !ok || el.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		if decidable, shared := attrDeclsDecidable(el); shared {
			if !decidable {
				return false
			}
			continue
		}
		local := el.Name().Local()
		if local == "simpleType" {
			if !simpleTypeDecidable(el) {
				return false
			}
			continue
		}
		if silentlyDroppedFacetElement(local) {
			return false
		}
	}
	return true
}

// silentlyDroppedFacetElement reports whether local names a child the parser's
// s4s order check lets THROUGH the facet position of xs:simpleRestrictionType
// (xmlschema11-1.md:1692) and the mapping then folds into NO facet — the false
// accept this allowlist exists to refuse.
//
// That is exactly the PLURAL <assertions>. §4.3.13 is where the two spellings
// part: the facet is "assertions" (a single facet holding the sequence) while
// the element contributing to it is <assertion>, which is what the facet choice
// spells — no element is named "assertions" anywhere in the s4s grammar. The
// parser's s4sFacetElement nevertheless admits the name into that position,
// because it asks builtin.FacetKindByName and that bridge answers for FACET
// names; its restrictionFacets then matches only the singular and its
// facetKindOf excludes xsd.FacetAssertions, so the child reaches no component
// and no error.
//
// It asks the same bridge, the ONE name→kind bridge the module owns (STYLE
// T4/D3), rather than spelling "assertions" here where the two would drift.
// <assert> is the CTD-level assertion of the same content model's trailing
// position, not a facet at all, and attrDeclsDecidable takes it before this is
// reached.
func silentlyDroppedFacetElement(local string) bool {
	kind, ok := builtin.FacetKindByName(builtin.FacetName(local))
	return ok && kind == xsd.FacetAssertions
}

// attrDeclsDecidable reports whether one child from the tail every complex-type
// derivation alternant ends with — annotation?, (attribute | attributeGroup)*,
// anyAttribute?, assert* (§3.4.2) — is within the producer's decidable subset.
// shared is false when el is none of those names, which leaves the verdict to
// the caller's own site vocabulary: a model group, a <group> or an
// <openContent> for contentDecidable, a facet or an inline <simpleType> for
// simpleContentRestrictionDecidable.
//
// The tail is one vocabulary and is judged in one place, so a site that gains a
// child kind cannot quietly disagree with the others about what an <attribute>
// or an <attributeGroup> may be.
func attrDeclsDecidable(el *parser.Element) (decidable, shared bool) {
	switch el.Name().Local() {
	case "annotation":
		return true, true // harmless
	case "attribute":
		return localAttributeDecidable(el), true
	case "attributeGroup":
		// Admitted in both shapes (#1182). The ref form is produced (#177); the
		// ref-less one is REJECTED by attributeGroupMember, either for carrying
		// the name= xs:attributeGroupRef prohibits or for missing the ref= it
		// requires, so the verdict is the producer's own and nothing is fabricated.
		return true, true
	case "anyAttribute":
		return true, true // an attribute wildcard is produced
	case "assert":
		// Produced (#178) into {assertions}. An Assertion carries only an opaque
		// XPath Expression record with no rejectable state, so admitting it can
		// never turn into a fabricated "invalid" verdict.
		return true, true
	}
	return false, false
}

// contentDecidable reports whether the content-model child and attribute children
// of a <complexType> (implicit content) or of a <complexContent>
// <restriction>/<extension> are all within the producer's decidable subset. A
// <group ref> content child and an <attributeGroup ref> are admitted (produced,
// #177); an <openContent> is admitted (produced, #230); a ref-less <group>/
// <attributeGroup> is admitted too, the producer rejecting it as the s4s
// attribute-use fault it is (#1182). A local <attribute>'s inline anonymous
// <simpleType> is admitted when the inline type itself is decidable — see
// localAttributeDecidable.
//
// The attribute/assert/annotation tail is judged by attrDeclsDecidable, shared
// with simpleContentRestrictionDecidable; what stays here is the vocabulary only
// a complex-content alternant carries.
//
// ITS TWO CALL POSITIONS SHARE ONE GRAMMAR, and that is what lets an unrecognized
// name be admitted rather than declined (#1181). xs:complexTypeModel's
// implicit-content disjunct (:1649), xs:complexRestrictionType (:1718) and
// xs:extensionType (:1723) each end with "openContent?, (group | all | choice |
// sequence)?, ((attribute | attributeGroup)*, anyAttribute?), assert*" behind an
// "annotation?" — exactly the names the arms below spell — so a name reaching
// the default arm fills no position of whichever model the producer ordered this
// element against, and checkS4SChildOrder charges §2.4 clause 1 sd-valid (:615,
// restated at §5.1's first bullet, :4296) over it. A stray <simpleContent>/
// <complexContent> is that fault at BOTH positions and no third one:
// complexTypeDecidable dispatches on either wrapper before it ever calls here,
// so neither name can reach the implicit-content position at all.
func contentDecidable(parent *parser.Element) bool {
	for _, child := range parent.Children() {
		el, ok := child.(*parser.Element)
		if !ok || el.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		if decidable, shared := attrDeclsDecidable(el); shared {
			if !decidable {
				return false
			}
			continue
		}
		switch el.Name().Local() {
		case "sequence", "choice", "all":
			if !modelGroupDecidable(el) {
				return false
			}
		case "group":
			// Produced in the ref form (#177) and REJECTED in the ref-less one
			// (#1182): produceGroupRefParticle charges the name= xs:groupRef
			// prohibits, or the absent ref= it requires. Both verdicts are genuine,
			// so both shapes are admitted.
		case "openContent":
			// Produced (#230) into {open content} (§3.4.2.3.3 clauses 5-6). Its own
			// src-ct clause 3/4 violations and its <any>'s src-wildcard/w-props-correct
			// ones are genuine rejections, so nothing here is limitation-shaped.
		default:
			// Any other name — a stray <simpleContent>/<complexContent> included —
			// fills no position of the model this element is ordered against, so
			// checkS4SChildOrder rejects the document. The verdict is the producer's
			// own, so the shape is admitted (#1181); see above for why the two call
			// positions agree about that.
		}
	}
	return true
}

// modelGroupDecidable reports whether every particle child of a model group
// (<sequence>/<choice>/<all>) is within the producer's decidable subset: nested
// model groups recurse, <element> must be locally decidable
// (localElementDecidable) — its identity constraints impose no condition of
// their own, both forms being produced for local declarations too (#178, #240) —
// <any> is fine, and a <group> is admitted in both shapes — produced in the ref
// form (#177), rejected by the producer in the ref-less one (#1182). A <unique>,
// <key> or <keyref> child is admitted because groupParticles' default arm
// rejects it (#1817). Any other child declines — that arm rejects most of them
// too, but admitting them is a widening with a ratchet measurement of its own.
func modelGroupDecidable(group *parser.Element) bool {
	for _, child := range group.Children() {
		el, ok := child.(*parser.Element)
		if !ok || el.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		switch el.Name().Local() {
		case "annotation", "any":
			// Harmless / produced.
		case "element":
			if !localElementDecidable(el) {
				return false
			}
		case "sequence", "choice", "all":
			if !modelGroupDecidable(el) {
				return false
			}
		case "group":
			// Produced in the ref form (#177) and REJECTED in the ref-less one
			// (#1182), by the same produceGroupRefParticle arms contentDecidable's
			// <group> child reaches.
		case "unique", "key", "keyref":
			// REJECTED by groupParticles' default arm (#1817).
		default:
			// any other child: not produced — decline.
			return false
		}
	}
	return true
}

// groupDecidable reports whether a top-level <group> (§3.7.2) is within the
// producer's decidable subset: its all/choice/sequence body's particles must all
// be decidable. Every other XSD-namespace child is admitted, because the producer
// rejects it against xs:namedGroup's content model on every definition it builds:
// a missing body (#884) and a child in the body's place through
// rejectNamedGroupBody, a child beside the body through checkS4SChildOrder
// (#1876). A child outside the XSD namespace is admitted on the same footing:
// that walk rejects it too (#1982). An <override> child that matches nothing is
// never built, so its out-of-model child, foreign or not, is NOT rejected
// (#1877).
//
// The DEFINITION FORM is not required either (#1182). A top-level <group> with
// no name=, or one carrying the ref= xs:namedGroup restricts to
// use="prohibited", is REJECTED by the producer — parser.topLevelName and
// rejectProhibitedAttrs respectively — under §2.4 clause 1 sd-valid, with no
// numbered rule ID because an s4s attribute-use fault has none. Admitting those
// two shapes therefore fabricates no verdict; it only lets the producer's own
// rejection be observed.
func groupDecidable(el *parser.Element) bool {
	for _, child := range el.Children() {
		c, ok := child.(*parser.Element)
		if !ok || c.Name().Space() != xsd.XMLSchemaNS {
			// A foreign child is REJECTED by the walk against xs:namedGroup
			// (#1982), save under the unmatched <override> the default arm marks
			// (#1877).
			continue
		}
		switch c.Name().Local() {
		case "annotation":
			// Harmless.
		case "all", "sequence", "choice":
			if !modelGroupDecidable(c) {
				return false
			}
		default:
			// REJECTED by the producer, whether or not a body stands beside it
			// (#1876). GAP(conformance): an unmatched <override> child is never
			// built, so this admission, and the foreign child's above, reaches a
			// document the producer accepts (#1877).
		}
	}
	return true
}

// attributeGroupDecidable reports whether a top-level <attributeGroup> (§3.6.2)
// is within the producer's decidable subset: its children must be only
// <attribute> (locally decidable — the group's <attribute> children map to LOCAL
// declarations, §3.6.2.1), <attributeGroup>, or <anyAttribute> — the shapes the
// producer folds in (§3.6.2.1/§3.6.2.2). A dangling or circular ref is decided
// genuinely at producer/finalize time, so it is admitted.
//
// The DEFINITION FORM is not required, on exactly groupDecidable's terms
// (#1182): xs:namedAttributeGroup makes name= use="required" and ref=
// use="prohibited", and the producer charges both faults itself, so a top-level
// <attributeGroup> missing the one or carrying the other is admitted here and
// rejected there.
//
// Every other XSD-namespace child is admitted on the same footing (#1817,
// #1876): the producer's checkS4SChildOrder walk rejects any name
// xs:namedAttributeGroup does not admit on every definition it builds, and so is
// a child outside the XSD namespace (#1982). An <override> child that matches
// nothing is never built, so its out-of-model child, foreign or not, is NOT
// rejected (#1877).
func attributeGroupDecidable(el *parser.Element) bool {
	for _, child := range el.Children() {
		c, ok := child.(*parser.Element)
		if !ok || c.Name().Space() != xsd.XMLSchemaNS {
			// A foreign child is REJECTED by the walk against
			// xs:namedAttributeGroup (#1982), save under the unmatched <override>
			// the default arm marks (#1877).
			continue
		}
		switch c.Name().Local() {
		case "annotation", "anyAttribute":
			// Harmless / produced.
		case "attribute":
			if !localAttributeDecidable(c) {
				return false
			}
		case "attributeGroup":
			// Produced in the ref form (#177) and REJECTED in the ref-less one
			// (#1182), by the same attributeGroupMember arms attrDeclsDecidable's
			// <attributeGroup> child reaches.
		default:
			// REJECTED by the producer's walk against xs:namedAttributeGroup (#1876).
			// GAP(conformance): an unmatched <override> child is never built, so
			// this admission, and the foreign child's above, reaches a document the
			// producer accepts (#1877).
		}
	}
	return true
}

// attributeDecidable reports whether a TOP-LEVEL <attribute> is decidable.
// type= is NOT required — a bare attribute defaults to xs:anySimpleType
// (§3.2.2.1), which builtin.Seed always seeds, so it resolves and is decided
// genuinely.
//
// An inline anonymous <simpleType> is admitted on the same terms as
// localAttributeDecidable admits one, and declined on the same terms too when
// the inline type's own shape is outside the produced subset. The two are no
// longer asymmetric: §3.2.2.1 dcl.att.global states the same three-tier {type
// definition} chain as §3.2.2.2 dcl.att.local word for word, and the producer
// maps both through declaredType (#733), so a difference here would be the
// fabrication.
func attributeDecidable(el *parser.Element) bool {
	inline := childXSD(el, "simpleType")
	return inline == nil || simpleTypeDecidable(inline)
}

// localElementDecidable reports whether a LOCAL <element> (a model group's
// particle child) is within the producer's decidable subset. The asymmetry with
// elementDecidable, its top-level sibling, is deliberate and is #229's whole
// point: the producer maps §3.3.2.1 dcl.elt.common clause 1 for a LOCAL element,
// so an inline anonymous <simpleType> is genuinely produced and admitted here —
// provided the inline type is itself in the produced simple-type subset, since
// otherwise the decline would move inside constructSimpleType and re-shape a
// limitation as a verdict.
//
// An inline <complexType> is produced too as of #340, on this path and on the
// global one alike (§3.3.2.1's tier 1 is a COMMON rule), and is admitted under
// the same proviso — through complexTypeDecidable, which since #1126 judges an
// anonymous type on the same terms as a named one and says there why anonymity
// narrows nothing. Its <alternative> children are gated by
// alternativeTypesDecidable, shared with elementDecidable.
func localElementDecidable(el *parser.Element) bool {
	if ct := childXSD(el, "complexType"); ct != nil && !complexTypeDecidable(ct) {
		return false
	}
	if inline := childXSD(el, "simpleType"); inline != nil && !simpleTypeDecidable(inline) {
		return false
	}
	return alternativeTypesDecidable(el)
}

// localAttributeDecidable reports whether a LOCAL <attribute> — a child of a
// <complexType>/<restriction> or of an <attributeGroup> definition, both of
// which map through §3.2.2.2 dcl.att.local — is within the producer's decidable
// subset. Like localElementDecidable, and like attributeDecidable since #733, it
// admits an inline anonymous <simpleType> whose own shape is produced (#229),
// and for the same reason declines one that is not.
func localAttributeDecidable(el *parser.Element) bool {
	inline := childXSD(el, "simpleType")
	return inline == nil || simpleTypeDecidable(inline)
}

// simpleTypeDecidable reports whether a <simpleType> (top-level, or an anonymous
// inline one reached through a restriction, list, union or element/attribute
// chain) is decidable. Every §3.16.2.1 alternative is, and so is a document
// naming none of them:
//
//   - <list> (#447): produced in both its forms, so it is admitted. Its
//     itemType= is a deferred by-name reference resolved at finalize, whose
//     src-resolve clause 1.1 failure is genuine; its inline <simpleType> child
//     is recursed into with these same checks.
//   - <union> (#738): produced in all three of its forms — memberTypes= alone,
//     inline <simpleType> children alone, and both at once — so it is admitted
//     on the same terms as <list>. Each memberTypes= item is a deferred by-name
//     reference; every inline child is recursed into. The membership-acyclicity
//     verdict a by-name member makes reachable is xsd's, charged at finalize
//     under cos-st-restricts clause 3.3, so a cyclic membership is a genuine
//     rejection here and not a decline.
//   - <restriction>: admitted outright. Every constraining facet it can carry is
//     produced, including the two folded ones — one assertions facet per
//     restriction (#178, Datatypes §4.3.13) and one enumeration facet per
//     restriction (#740, §4.3.5.2 xr-enumeration). An inline <simpleType> base
//     child (§3.16.3 clause 2) is recursed into.
//   - NONE of the three (#786): admitted. simpleTypeBody rejects that document
//     unconditionally — under §5.1's first bullet and no numbered rule, an
//     unfilled alternative position, which is why the rejection carries no
//     rule ID — so the harness observes "invalid" and no verdict is fabricated.
//
// A <simpleType> naming TWO alternatives is admitted through whichever branch is
// tested first, and correctly: the producer rejects that document outright
// (simpleTypeBody), so the verdict is genuine whichever branch let it through.
//
// EVERY return here is therefore true and the recursion only propagates that:
// the predicate is a constant, and no call site of it gates anything. Retiring
// it collapses attributeDecidable and localAttributeDecidable to constants too
// and rewrites the justification every call site carries, so it is a refactor of
// its own rather than part of the widening that made it one (#786).
func simpleTypeDecidable(el *parser.Element) bool {
	if list := childXSD(el, "list"); list != nil {
		inline := childXSD(list, "simpleType")
		return inline == nil || simpleTypeDecidable(inline)
	}
	if union := childXSD(el, "union"); union != nil {
		for _, inline := range childrenXSD(union, "simpleType") {
			if !simpleTypeDecidable(inline) {
				return false
			}
		}
		return true
	}
	restriction := childXSD(el, "restriction")
	if restriction == nil {
		return true
	}
	for _, child := range restriction.Children() {
		r, ok := child.(*parser.Element)
		if !ok {
			continue
		}
		if r.Name().Space() != xsd.XMLSchemaNS {
			continue
		}
		if r.Name().Local() == "simpleType" && !simpleTypeDecidable(r) {
			return false
		}
	}
	return true
}

// hasAttr reports whether el carries the unprefixed (no-namespace) attribute
// local, as XSD schema-element attributes (name, ref, …) carry no namespace. It
// is the presence-only face of parser.Element.Attr, not a second scan of its own
// (STYLE D3) — the lookup itself lives in the parser package, which is the one
// implementation both this harness and the assembly use (#272).
func hasAttr(el *parser.Element, local string) bool {
	_, ok := el.Attr(local)
	return ok
}

// childrenXSD returns every child element of el with expanded name
// {XMLSchemaNS}local, in document order. It is childXSD's repeatable twin, for
// the one gated content model that admits several — <union>'s inline
// <simpleType> children (§3.16.2.4 map.std.union case 1b).
func childrenXSD(el *parser.Element, local string) []*parser.Element {
	var found []*parser.Element
	for _, child := range el.Children() {
		c, ok := child.(*parser.Element)
		if !ok {
			continue
		}
		if name := c.Name(); name.Space() == xsd.XMLSchemaNS && name.Local() == local {
			found = append(found, c)
		}
	}
	return found
}

// childXSD returns el's first child element with expanded name {XMLSchemaNS}local,
// or nil.
func childXSD(el *parser.Element, local string) *parser.Element {
	for _, child := range el.Children() {
		c, ok := child.(*parser.Element)
		if !ok {
			continue
		}
		if name := c.Name(); name.Space() == xsd.XMLSchemaNS && name.Local() == local {
			return c
		}
	}
	return nil
}
