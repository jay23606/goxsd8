# goxsd8 Architecture

## Dependency rule

Packages form a strict DAG. `xsderr` (the error currency) is the **pure
leaf**: it imports nothing from this module. `xsd` (the component model)
imports only `xsderr`; every other package builds above the two of them.
Value implementations, parsing, validation, and generation live above them.

```
                 xsderr          (leaf: errors, rule IDs, locations)
                 xsd             (leaf: component model + query/walk APIs; imports xsderr only)
                 internal/...    (leaves: helpers over the standard library, unexportable
                                  because they are nobody's API. Each imports only the
                                  standard library, except internal/xmltok, whose
                                  two module edges, to internal/xmlname and
                                  internal/xmlchar, are the only
                                  internal-on-internal edges:
                                  - internal/schemaloc, the schemaLocation resolver. Its
                                    doc.go still justifies it by the conformance closure
                                    walk #272 deleted (#1286). Its consumers are parser's
                                    assembly and the two copies of the xsi:schemaLocation
                                    hint reader, cmd/goxsd8's and the conformance
                                    harness's. #845's fold-back into parser waits on #755
                                    giving that reader one home in the library.
                                  - internal/xmldecl, a 1.x XMLDecl label read as 1.0
                                    (XML 1.0 §2.8 Note), which parser/xmltree's reader and
                                    the conformance harness's raw re-reads of the same
                                    documents must admit alike.
                                  - internal/xmlenc, byte-order-mark detection, the UTF-16
                                    transcoder and the mark/declaration agreement (XML 1.0
                                    §4.3.3, App. F.1), which the same two readers must
                                    decode alike.
                                  - internal/xmlname, XML 1.0 5e [4] NameStartChar and
                                    [4a] NameChar, which regex's \i/\c,
                                    parser/xmltree's DOCTYPE name checks and
                                    internal/xmltok's name checks read.
                                  - internal/xmlchar, XML 1.0 [2] Char, which
                                    parser/xmltree's character-reference and
                                    markup checks and internal/xmltok's text
                                    check read.
                                  - internal/xmltok, encoding/xml's strict tokenizer
                                    forked to check names against internal/xmlname's
                                    5e tables, which parser/xmltree's reader and
                                    entity replacement-text reader and the
                                    conformance harness's raw re-reads, its
                                    instance hint reader among them, decode with.)
                 value           (value-space contracts, facet pipeline; imports xsd, xsderr, regex)
                 value/backendtest (conformance kit for any backend)
                 builtin         (the generated TypeSpec table and Seed; imports value, xsd, xsderr)
   builtin/strict  builtin/native  <user backends>   (implement value contracts; builtin/strict
                                  also imports builtin and regex, to compile xs:NCName's
                                  generated pattern)
                 regex           (one engine, XSD + F&O flavors)
                 parser/xmltree  (position-tracking XML; imports xsderr,
                                  internal/xmldecl, internal/xmlenc,
                                  internal/xmlname, internal/xmlchar and
                                  internal/xmltok only, and nothing else in the
                                  module — independent of the schema pipeline, not
                                  of the error currency. An internal/ leaf whose
                                  own module edges, if any, are internal/ leaves
                                  is the one kind of further edge it may take;
                                  it never imports regex — see the XML-production
                                  ruling below)
                 loader          (schema resolution interfaces)
                 xpath           (XPath 2.0 engine; imports xsd, value, regex, xsderr)
                 icpath          (the §3.11.6.2/§3.11.6.3 identity-constraint path
                                  subset whole: the grammar, the two SCCs over it
                                  and the streaming matcher; imports xsd, xsderr,
                                  regex. Two consumers in two layers — parser
                                  charges, validate matches)
                 parser          (schema docs -> xsd components; imports xmltree, loader,
                                  xsd, value, builtin, regex, xpath, icpath — and
                                  builtin/strict, as the DEFAULT backend Parse seeds when
                                  the caller supplies none)
                 validate        (instance validation; adapters xmlsrc, jsonsrc, bersrc) [2]
                 codegen  codec  (generation; dataset ser/de)               [1]
                 conformance     (harness + ratchet; repo infrastructure)
                 cmd/goxsd8      (the CLI; help, parse and validate ship, gen does not)
```

**[1] These boxes are DESTINATIONS, not shipped layers.**
`validate/bersrc`, `codegen`, `codec` and
`builtin/native` are `doc.go`-only today: `go doc` renders no exported
identifier for any of them and they import nothing from this module, so the
edges drawn above do not exist yet in `go list -deps`. Read every
present-tense sentence in their sections below as "will", not "does".
`xpath` has left this note: it exports the CTA compile, evaluate and
statically-check entry points, and the assertion compile and evaluate ones,
and imports `xsd`, `value`, `regex` and
`xsderr` for real (its section says which tiers are shipped).

**[2] The infoset seam, the assessment skeleton, the XML adapter and the
first slice of the JSON adapter ship; the BER adapter does not.** `validate` exports the infoset views and
`New`/`Validator`/`Result` and imports `xsd`, `xsderr`, `value`, `xpath` and
`icpath` — no longer `regex` directly, whose NCName class left with the
identity-constraint path lexer for `icpath`, though it stays in the closure
behind `value` for the pattern facets validate compiles — but never a
backend: `New` takes the `value.Backend` as a required parameter, so
`builtin/strict` stays outside the closure exactly as it does for `parser`'s
`Produce` (see its section);
`validate/xmlsrc` exports `Validate`/`Option`/`WithURI` and imports
`parser/xmltree`, `validate`, `xsd` and `xsderr` — and neither `loader` nor
`parser`, which an imports test pins; `validate/jsonsrc` exports
`Validate`/`Option`/`WithURI`, imports `validate`, `xsd`, `xsderr` and
`encoding/json`, and reads the schema through `validate.SchemaView`, the
view `Validator.Schema` returns, to name the root and classify members;
`validate/bersrc` is a destination, on `[1]`'s terms.

Only `xsderr`, `xsd`, `internal/schemaloc`, `internal/xmldecl`,
`internal/xmlenc`, `internal/xmlname`, `internal/xmlchar`,
`internal/xmltok`, `value`, `value/backendtest`, `regex`, `builtin`, `builtin/strict`, `loader`,
`parser`, `parser/xmltree`, `xpath`, `icpath`, `validate`,
`validate/xmlsrc`, `validate/jsonsrc`, `conformance` and `cmd/goxsd8` carry
code today.

**The module has two tiers, and the dependency rules govern the first.**
The **library** is what a consumer imports — the packages above plus
`cmd/goxsd8` — and it is the published surface the warden and steward
guard. **Repo infrastructure** is `conformance` and `tools/*`: code that
exists to build and verify the library, ships to no one, and is outside
the export interface entirely.

Nothing in the library imports infrastructure. Infrastructure may import
the library, and may import other infrastructure — so a tool needing the
expectations file format calls `conformance.LoadExpectations` rather than
becoming a second reader of a format that already has an owner.

**Give code two tools share one home under `tools/internal/<name>`**; do
not copy it into a second `package main`. Go's internal rule confines that
package's importers to `tools/...`, and `go tool surface` skips `tools/`, so
it can reach neither the library nor the published surface — no per-helper
argument for standing it up is owed. `tools/hfnextract/internal` is the
same shape one level down.

**Give an XML 1.0 or Namespaces in XML production two library packages read
one home in a stdlib-only `internal/<name>` leaf**; do not export it from
either reader, and do not add an edge between them. `parser/xmltree` in particular never
imports `regex` to reach one. The Name productions ([4] `NameStartChar`,
[4a] `NameChar`) live in `internal/xmlname`, read by `regex`'s `\i`/`\c`
sets, `parser/xmltree`'s DOCTYPE name checks and `internal/xmltok`'s name
checks. [5] `Name` over those tables is still written twice, as `isName` in
`parser/xmltree/doctype.go` and in `internal/xmltok/xmltok.go`, and #2600
gives it its one home there. Whether that table is generated is
#989's question, asked of `internal/xmlname`. [2] `Char` lives in
`internal/xmlchar`, read by `parser/xmltree`'s character-reference check and
markup check and `internal/xmltok`'s text check (#2187, #2265).

`cmd/goxsd8` is a library CONSUMER, not a place to grow capability. A
capability the CLI needs and the library does not export is a library gap to
file, not CLI code to write. The CLI has taken the other branch for
instance-hint reading, under "Parsing & loading" below (#755). The
`validate` ENGINE imports no source's decoder (`encoding/xml`,
`encoding/json`, BER) — only its adapter does, and `validate/imports_test.go`
pins it. That ban is the engine's and not the library's: `parser/xmltree` is
the library's one XML reader, decoding through `internal/xmltok`, and `parser`
and `validate/xmlsrc` both sit above it.

## Lexical space vs value space

The load-bearing separation of the whole design (Datatypes §2.1–2.3):

- **Lexical space**: strings. Whitespace normalization and `pattern` facets
  operate here, *before* any parsing.
- **Value space**: typed Go values. Ordering, equality, identity, and all
  value-based facets (`minInclusive`, `totalDigits`, `length` on lists,
  `enumeration`, `assertion`) operate here, *after* the lexical mapping.
- The bridge is the pair of **mappings** per type: lexical → value and
  value → canonical lexical. These are defined normatively as function
  definitions ("hfn") in Datatypes Appendix E, and our builtins are
  bootstrapped from those definitions (extracted from
  `docs/specs/md/xmlschema11-2.md` by the hfn tooling), not
  hand-transcribed.

### The facet pipeline

Validation of a literal against a simple type is a fixed pipeline. The two
stage kinds are named as interfaces (`value.LexicalFacet`,
`value.ValueFacet`), but there is **no composition seam yet**: every
implementation is unexported inside `value`, the function that assembles a
type's stages is unexported, and no exported API accepts or returns a stage.
User-composed stages are a destination, not a shipped capability (steward
audit 2026-08-02) — see the note under "Exported surface without a
consumer" below.

```
raw literal
  → whiteSpace normalization        (lexical; from the type's ws facet)
  → pattern facets                  (lexical; every step of the derivation chain)
  → lexical mapping                 (string → value.Value, via the backend)
  → value facets                    (bounds, digits, scale, length, enumeration)
  → assertions                      (XPath, fail-open; per-item for lists,
                                     per-member for unions, at every level)
```

List and union varieties recurse: lists apply the pipeline per item against
the item type before list-level facets; unions try their `{member type
definitions}` (`SimpleType.Members`) in order (not flattened members —
intervening restrictions carry facets, and pattern normalization uses the
*validating member's* whiteSpace).

## Builtin types: generated table + pluggable backends

The builtin type system separates **what the spec says** from **how Go
represents it**:

- **`builtin.TypeSpec` table** (generated, data only): name, base type,
  variety, fundamental facets, applicable constraining facets and their
  defaults, for all 49 builtins **including `precisionDecimal`**. Emitted
  from the hfn definitions and per-type property tables in the local
  Datatypes spec by a deterministic generator; byte-identical on
  regeneration; contains no function values.
- **Primitives are the mandatory floor; derived mappings are optional.**
  Derived builtins (`unsignedShort`, `token`, …) are data by default —
  restrictions of a primitive plus facets from the table, inheriting
  operations — so a minimal backend implements ~25 primitive mappings
  (several share a value space; the Gregorian types ride the temporal
  model). A backend may ALSO map derived builtins to give them narrower
  representations; each type's governing mapping resolves by walking up
  the base chain to the nearest mapped ancestor.
- **The widest-space rule.** A derived type's own mapping governs only
  the value the application receives. Inherited facet checks —
  enumeration and bounds declared anywhere on the derivation chain — are
  compared in the declaring type's space via ITS governing (wider)
  mapping, and schema-build restriction checks always run in the base's
  space: a narrower representation must never distort base-chain
  semantics (overflow, collapsed precision, different ordering). A
  lexical that passes the wide checks but doesn't fit the narrow
  representation is a mapping error on that type, never a false validity
  verdict.
- **`value.Backend`** answers `Mapping(typ)` → a
  `value.Mapping{Parse(lexical, ctx), Canonical(v)}` pair. `Parse` takes a
  context because QName/NOTATION need in-scope namespace bindings.
  Comparison, length, digits, scale, identity are **not** backend methods —
  they are capability interfaces discovered on the returned values.
- **`builtin.Seed(backend)`** composes the generated table with a backend
  at schema-construction time; `value.Override(base, partial)` swaps
  individual types (back only `xs:decimal` with a money type, keep strict
  for the rest).
- Two backends are planned; **one ships today**:
  - `builtin/strict` (ships) — spec-exact: arbitrary-precision
    decimal/integer, `precisionDecimal` (coefficient/scale/sign identity,
    NaN/±INF), the 7-property date/time model, XSD-exact float/double
    behavior. It imports `builtin` for one datum, `builtin.NCNamePattern`,
    which its QName lexical check compiles; `builtin` imports no backend,
    so the edge points toward the leaves like every other.
  - `builtin/native` (**M12 — doc-only contract, exports nothing today**)
    — Go-friendly: `int64`, `float64`, `string`, `time.Time`; documented,
    deliberate deviations from the spec value spaces (range limits,
    timezone folding). Its `doc.go` is the normative contract and already
    marks itself planned.
- **Third-party backends are a supported surface.** `value/backendtest`
  is the public conformance kit: `backendtest.Run(t, backend)` drives
  spec-derived vectors (lexical→value→canonical round-trips, order and
  identity cases, the capability set each type's facets require) plus a
  primitive-coverage check. `builtin/strict` passes it in-repo (that is the
  whole of "our backends" until M12); a custom backend that passes it is
  first-class.

## Component model (`xsd`)

- Components are constructed in **phases** so no traversal ever needs a
  cycle check (STYLE D4): (1) parse schema documents into raw form,
  (2) resolve QName references through a symbol table,
  (3) finalize in dependency order — a component's base/item/member types
  are complete before it is. Spec-forbidden circularities are rejected with
  their named rule, once.

  **Where they are rejected used to be split by per-component
  representation accident, and the simple-type half of that drift is now
  repaired** (steward audits 2026-07-26 and 2026-08-02; tracked as **#271**,
  superseded by **#478** → **#629**/**#636**). Phase 3 holds every reference
  `xsd` defers rather than resolving eagerly: the complex-type base chain
  (`ct-props-correct` cl. 3 — a `TypeDefinitionOrRef` since #505, whose
  by-name arm is the deferred one and whose owned arm is already resolved),
  the SIMPLE-type base chain (`st-props-correct` cl. 2 — a
  `SimpleTypeOrRef` since #636, with the same two-arm shape), the
  `<group ref>` graph (`mg-props-correct` cl. 2), substitution-group
  affiliation (`e-props-correct` cl. 5), and the `<attributeGroup ref>`
  graph (`src-resolve` cl. 1.4 — an `AttributeUseOrGroupRef` since #479,
  folded to its §3.6.2.1/§3.6.2.2 transitive closure in
  `xsd/attributegroupfold.go`), all in `xsd/resolve.go`. Because
  the simple-type base is a reference again, every reader that walks it —
  `Base`, `Variety`, `Primitive`, `Item`, `Members`, `EffectiveFacets` —
  takes an `xsd.TypeResolver` and returns an error, and the packages above
  `xsd` thread that resolver as a parameter and store it nowhere.

  **What remains of the split.** `parser/produce.go`'s `buildComplexType`
  still charges `ct-props-correct` cl. 3 from the producer — the same rule
  with the same verdict `xsd/resolve.go`'s `checkComplexBaseAcyclic`
  charges — because `xsd.NewComplexType` still demands a complete base
  component at construction, so demand-driven eager base construction would
  otherwise not terminate. The simple-type twin died with #636, which also
  removed the producer's `st-props-correct` cl. 2 and `src-resolve` cl. 1.1
  charges for a simple type's base. The attribute-group half died with
  #479, which also removed the producer's `src-resolve` cl. 1.4 splice
  (`collectReferencedGroup`/`spliceAttributeGroup`) and its
  `combineAttributeWildcards` §3.6.2.2 union, both now decided in
  `xsd/attributegroupfold.go`.

  **`ct-props-correct` is not the only rule charged from both packages, and
  the others are deliberate.** `src-resolve` (cl. 1.1 for a COMPLEX base, and
  cl. 4's per-document licensing), `src-redefine`, `e-props-correct` cl. 3 and
  `sch-props-correct` cl. 2 are each charged from `parser` and from `xsd`.
  `parser/produce.go`'s rule const block says for each why the producer's
  charge and the component's are one rule seen from two sides; read that block
  before treating a shared rule ID as drift.

  Moving the complex-type base onto the same footing is the remaining
  pre-1.0 refactor. On the simple-type side the deferral is complete:
  `ListDerivation.Item` and `UnionDerivation.Members` are `SimpleTypeOrRef`
  slots too as of #447, so an `itemType=` or `memberTypes=` naming a
  forward-declared type defers exactly as a `base=` does, and all three are
  resolved by one helper in one phase. The producer maps all three
  §3.16.2.1 alternatives as of #738, and a by-name membership is kept
  acyclic by `cos-st-restricts` cl. 3.3 in the same finalize phase as the
  base chains.
- All child collections are slices in document order. Maps exist only as
  internal indexes and never determine any order.
- Nothing derivable is stored (STYLE D3): no effective-facet caches —
  compute `base.EffectiveFacets(resolver)` overlaid with the declared set on
  demand; no status booleans beside the facts that imply them.
- The model is **read-only** after construction; mutation/editing APIs are
  out of scope. `Finalize` performs a bounded set of mutations before it
  returns — **three**, run in this order from `xsd/resolve.go`'s Phase D:
  `xsd/attributegroupfold.go` materialises the §3.6.2.1 `{attribute uses}`
  union and §3.6.2.2 `{attribute wildcard}` intersection an
  `<attributeGroup ref>` takes part in, into every attribute group
  definition and complex type, consuming every `AttributeGroupRef` (#479);
  then `xsd/attributeusefold.go` materialises §3.4.2.4 clause 3's inherited
  `{attribute uses}` into every complex type (#401); then
  `xsd/attributewildcardfold.go` materialises §3.4.2.5 clause 2's
  `{attribute wildcard}` the same way. Each is a property OVERWRITE, not a
  cache of derivable state (STYLE D3): afterwards the producer's partial
  value is gone rather than kept beside the correct one. Read-only means
  read-only *after* `Finalize`.

  **A FOURTH finalize-time write site is a design change, not an
  increment: file it as a reviewed issue before writing it.** The third
  arrived that way (#479). The attribute-group fold has its own shape — one
  closure computing both properties — and is not a copy of the other two;
  the use and wildcard folds still share near-identical scaffolding (fold
  structs, index-building loops, byte-identical `storeFolded…` bodies,
  `foldOwned…` wrappers around `ownedTypeFold`), which is #1285's STYLE T4
  finding. #414 closed on the anonymous-type gap, not on that duplication.

### Value spaces without a dependency (`xsd.ValueSpace`)

A handful of finalize-time constraints need to know something about a *value*,
not a lexical form: `au-props-correct` (§3.5.6) cl. 3 and `loc-testSubP`
(§3.4.6.4) cl. 4.2/5.2.2 COMPARE two `{value}`s; `a-props-correct` (§3.2.6.1)
cl. 2 and `au-props-correct` cl. 2 ask whether a `{lexical form}` denotes one
at all (Simple Default Valid, §3.2.6.2). Answering any of them needs `value`,
which sits ABOVE `xsd`. Rather than invert the DAG, the whole capability is
taken as an **input**: `xsd.ValueSpace` is a consumer-side interface (STYLE
T3) declared in `xsd`, installed through `SchemaBuilder.FinalizeWith`, and
implemented by `value.NewValueSpace(backend)` — which is what `parser`
installs. Plain `SchemaBuilder.Finalize` installs the unexported
`undecidedValueSpace`, so there is one code path, never a nil check.

Every method may answer **undecided**, and undecided always ACCEPTS
(PRINCIPLES 20's direction applied to value spaces): an ungoverned type, a
lexical the mapping cannot map, two incommensurable value spaces, or a QName
prefix unresolvable in the context the `ValueConstraint` captured. Plain
`Finalize` is therefore the fully fail-open configuration, which is exactly
the behaviour that predated the seam.

The related identity currency is `xsd.ComponentID`: an opaque token minted by
the producer (`NewComponentID`) before either endpoint exists, for the
references a QName cannot carry — anonymous and local components. It is
compared with `==`, never rendered (its underlying value is an address, so any
textual or sorted form would be nondeterministic, D1/D2) and never derived
from position. `Loc` is provenance, not identity.

### Why the finalize machinery lives in `xsd` (a steward ruling, 2026-08-02; re-confirmed 2026-09-06)

`xsd` is by far the largest package, and most of its non-test files —
`resolve.go`, `contentrestricts.go`, `complexextension.go`,
`particleattribution.go`, the two attribute folds and the shared descent
beneath them, and a dozen more — export **nothing at all**. The exact count
is not maintained here and no audit should update it (STYLE D3, #665). That
looks like a candidate for an `xsd/finalize` sub-package. **It is not; do
not propose the split.**

The constraint machinery reads and writes the components' *unexported*
fields — `attributeusefold.go` reads `ComplexType.prohibitedAttributeNames`
and writes `attributeUses` back. Moving it to a sibling package would force
those fields onto the exported surface (or force a mutator API), trading a
big-but-sealed package for a permanently wider public API and a
representable illegal state (STYLE T1). Go's package boundary is exactly the
tool that keeps the component invariants unforgeable, and the zero-export
files are the evidence it is working, not evidence of a missing seam. Judge
`xsd` by its exported surface and its doc.go contract, not by line count.

### Query and walk

Two access styles over the compiled model, one shared core:

- **Query**: direct lookups — element/attribute/type by QName — exposed
  through minimal capability views (STYLE T3): `xsd.ElementResolver`,
  `xsd.AttributeResolver` and `xsd.TypeResolver`, each one lookup method, so
  a consumer that needs only `Element` receives only that.
- **Walk**: traversal of a type's effective content model. The algebra
  ships (type-derivation validity, substitution-group acceptance, wildcard
  admission, attribute-use lookup) — **mostly unexported, with a few
  deliberate entry points the validator needs**; `xsd/doc.go`'s "Walk API"
  section names them and is authoritative on which. Of the two
  drivers over it one ships and one does not:
  - a **pull** driver — `Matcher`, the instance-guided advance of the
    content model one child at a time (the validation consumer) —
    **ships**, as `Schema.ContentMatcher`/`Matcher`/`Attribution`; it
    counts occurrences rather than unfolding them and never backtracks.
    `cos-nonambig` licenses that for which particle takes an item;
    `cvc-accept` clause 3.1's existential over partitions is why the walk
    carries the live partitions — as runs of occurrence counts, not one
    entry each — where a repeating particle can take an item on either side
    of a repeating ancestor's iteration boundary.
  - a **push** driver — `Walker`, the exhaustive, schema-only visitor of
    every particle reachable through sequences/choices/all-groups and
    named-group references (the codegen consumer) — **M9**, not yet.
  Substitution groups are not expanded at walk time (instance-time
  concern), and the Walker will reuse the same algebra rather than
  reimplement it. `go doc ./xsd` is the contract.

## Parsing & loading

- `parser/xmltree`: streaming, bounded-memory XML reader that records
  line/column for every node; the origin of every `xsderr.Loc`. No
  `io.ReadAll` (STYLE P4).
- `loader`: the IO seam. `Resolver` answers "give me the schema document
  for (namespace, location hint)"; helpers provided for files, HTTP, and
  in-memory maps, plus a chaining/catalog resolver. The loader dedupes by
  resolved location. Multi-root assembly is `parser.ParseSet`, whose
  `RootAt` and `HintAt` roots enter one assembly through one `Resolver`
  (#1283). The `xsi:schemaLocation` instance-hint reader is the other
  capability named on this seam, is `parser`'s to export, and does not exist
  there. Two copies stand in for it, both over `internal/schemaloc`:
  `cmd/goxsd8/validate.go`'s `instanceHints`/`hintsOf`, and
  `conformance/instancehints.go`'s. They have already diverged — on an odd
  `xsi:schemaLocation` member count the CLI drops the trailing member where
  the harness declines, and only the harness reads hints below the document
  element and inline `xs:schema` documents — and #755 owns the one home both
  retire into. `loader/doc.go` states the reader in the present tense; that
  sentence is drift (#1286).

- `parser`: the schema-document compiler — the M4 spine, and the only
  writer of `xsd` components. `Parse(location, opts…)` reads the root
  document through one `loader.Resolver`, walks the
  `<xs:include>`/`<xs:import>`/`<xs:override>` closure (§4.2.3, §4.2.6.2,
  §4.2.5) depth-first in document order with a load-once index keyed by
  resolved location, the namespace the document was reached under and the
  override applied to it (document identity, *not* a cycle guard — include
  cycles are spec-legal); the redefinition is deliberately outside that key,
  so a document reached both plainly and as an `<xs:redefine>` target is
  composed once and contributes each definition the redefinition does not
  name once (#1349). It applies chameleon coercion to a
  no-`targetNamespace` included, overridden or redefined document (§F.1),
  carries override pre-processing (§F.2) and redefinition (§4.2.4) alike as
  data beside the effective namespace,
  then produces every document into
  one shared `xsd.SchemaBuilder` and finalizes. `Produce(doc, backend)` is
  the single-document entry point and follows no inter-document reference.
  `<redefine>` is followed (#286) for all four redefinable kinds; a
  redefining `complexType` is paired with the `{name}`-absent original
  `src-expredef` cl. 1.1 defines, which `xsd.ComplexType` holds in the
  `InlineTypeDefinition` arm of its `{base type definition}` slot (#505).
  The assembly-wide symbol table is seeded with the builtins exactly once
  (`builtin.Seed`), which is why seeding is assembly-scoped and not
  per-document: per-document seeding would re-add `xs:string` per included
  document and trip `sch-props-correct` cl. 2.

  Two properties of this seam are worth stating because consumers depend on
  them. First, the backend is a caller-supplied `value.Backend` (`Produce`
  demands it explicitly; only `Parse` defaults it to `builtin/strict`, which
  is the one policy edge from `parser` to a concrete backend). Second, the
  assembled **document set IS reported**, through the entry point:
  `ParseSet(roots, opts…)` assembles several roots — `RootAt` documents and
  `HintAt` schema location hints — into one schema and returns the components
  *and* an `*AssemblyReport` naming every document read, in discovery order
  (`AssembledDocument`), every ·inter-schema-document reference· (§4.2.1)
  the assembly could not follow to one (`UnfollowedDirective`,
  `UnfollowedReason`), and every hint root that named no document
  (`UnfollowedRoots`). `ParseReport(location, opts…)` is `ParseSet` over one
  `RootAt`, and `Parse` its report-free convenience wrapper.

  That closes **#272**, the audit's longest-standing duplication finding.
  The conformance schema lane used to carry its own
  `<include>`/`<import>`/`<override>` walk in order to gate every document in
  a closure; location resolution was de-duplicated first, onto
  `internal/schemaloc.Resolve` (#259), and the walk itself is now gone:
  `conformance/schema_closure.go` holds two functions, `closureDecidable`
  and `closureReached`, that read the report.
  The gated set is the assembled set *by construction* rather than by two
  walks agreeing, so no new composition feature has to be ported twice.

## Regex (`regex`)

One recursive-descent engine translating to Go's RE2, with a **flavor
flag** (PRINCIPLES 10):

- **XSD flavor** (pattern facets): implicitly anchored, `^`/`$` literal,
  non-capturing groups, no flags, `.` excludes `\n` and `\r`.
- **F&O flavor** (`fn:matches`/`fn:replace`/`fn:tokenize`): unanchored,
  real anchors, capturing groups; `i`/`s`/`m` map to RE2 inline flags and
  `x` strips insignificant whitespace before parsing; `q` (undefined in the
  local F&O edition) and any other flag are `err:FORX0001`, and
  back-references — legal F&O grammar but with no RE2 form — are
  `err:FORX0002`, surfaced, never silently accepted.

Character-class handling (`\d \w \p{…}`, subtraction `[a-z-[m]]`) is
shared. The package sits just above the leaves: it imports only `xsderr`
(so its `FORX0001`/`FORX0002`/`src-pattern-value` failures are rule-tagged
per STYLE T2) and `internal/xmlname` (the Name productions behind `\i`/`\c`),
otherwise stdlib.

## XPath (`xpath`)

**Status: the CTA required subset ships, and over the same grammar the
first slices of assertion evaluation; the rest is the destination.**
`xpath/doc.go` owns the exported inventory and the grammar each tier admits;
read it there. Two consumers take the exports in two phases. `parser` calls
`CTATestStaticError` at schema construction, charging `ta-props-correct`
clause 2 over `xpath-valid` clause 2 for a Type Alternative `{test}` with an
XPath static error — wrapping the `*xsderr.Error` this package returns,
which carries the XPath code (`err:XPST0003` or `err:XPST0081`) as its own
rule. `validate`
takes the rest at ·assessment· time: it compiles and evaluates the same
`{test}` against §3.12.6's `ta-Test` grammar; it compiles and evaluates an
assertion `{test}` over the typed values of the element's attributes, of its
element children and of its simple content for `cvc-assertion`, keeping only
the children the compiled test reads; for a test that counts nodes it walks
the element's subtree in `validate/cvcassertion.go`, reporting each element
and attribute to the test's `Tally`, with the typed value of each attribute a
path reads (`ReadsAttribute`, `Attribute`) and, once its subtree is walked,
of each element a path of child steps reads (`ReadsElementValue`,
`ElementValue`), and asking `ReadsAttributesAt` at
which depths an element whose ·governing type definition· was not determined
leaves the count or the value lacking; and it injects `FacetAssertions`, the
`value.AssertionEvaluator` for an assertions facet's `{test}`
(`cvc-assertions-valid`), into `value.ValidateLexical`/`ValidatingType`.
Read the tiers below as "does" for tier 1 and for tier 2's first slices,
and "will" for the rest.

Full XPath 2.0 is the destination; the engine grows outward from the
XSD-required subset:

1. the CTA restricted subset (the `test` attribute of `xs:alternative`) —
   shipped, less the shapes that compile-time-decline, which `xpath/doc.go`
   enumerates,
2. assertion essentials — axes, predicates, quantified expressions, typed
   comparisons, the F&O function core; its first slices ship, and
   `xpath/doc.go`'s forms table enumerates the grammar they admit — `fn:count`
   among it, over `N` or `@N`, bare or behind `./` or `.//`, or over a
   rooted step — and what they decline,
3. the full grammar and function library, tracked by its own conformance
   lane.

One lexer, one parser, one AST — the evaluator walks the same tree the
static analyzer sees. The one other grammar is a tree-less recognizer for
the full xpath20.md Appendix A grammar: it accepts exactly that language, a
superset of the parser's, and is consulted only once the parser declines a
`{test}`, to tell one outside XPath 2.0 (`err:XPST0003`) from legal XPath the
subset merely declines. **Fail-open**: an unsupported construct can never
cause a false rejection; every fallback site is a greppable
`// GAP(xpath): …`. A CTA `{test}` outside the implemented subset is
declined at COMPILE time, and its caller withholds the element's
·governing type definition· rather than reading the test as unmatched; an
assertion `{test}` outside it is declined the same way, and its caller
records it as unevaluated.
Dynamic errors (type mismatch, bad pattern) make an assertion definitively
unsatisfied and a CTA `{test}` definitively false (`key-cta-ta-select`
clause 2) — they are NOT fail-open (PRINCIPLES 20). PRINCIPLES 17 owns
how `$value` binds. F&O regex functions use `regex`'s F&O flavor,
never the pattern-facet flavor.

## Identity-constraint paths (`icpath`)

**Status: ships whole — the grammar, the two SCCs over it and the streaming
matcher.** `go doc ./icpath` renders the exported inventory; read it there.
It owns §3.11.6.2 and §3.11.6.3 — the ·selector subset· and the ·field
subset·, a path grammar over the child and attribute axes — and imports `xsd`,
`xsderr` and `regex` (the NCName class its lexer scans with).

Two consumers in two phases, the shape `xpath` has for §3.12.6's `ta-Test`:
`parser` calls the two `Violation` entry points at schema construction,
charging `c-selector-xpath` / `c-fields-xpaths` at the offending
`<selector>`/`<field>`, and `validate` compiles the same `{expression}` with
`CompileSelector`/`CompileField` and advances the opaque
`Expr`/`Live`/`Selection` at ·assessment· time for `cvc-identity-constraint`.
It is NOT part of `xpath`: this grammar is not a stage of XPath 2.0 — its
lexer borrows XPath 2.0's closed axis vocabulary and `node()` only to classify
a step head, and its matcher evaluates the child and attribute axes alone — and
`validate/doc.go` states these paths are evaluated "directly and never through
the XPath engine".

**Fail-open, in both phases.** Clause 2 of each SCC is a disjunction whose
second arm admits the `child::` (and, for a field, `attribute::`) spelling of
an admitted path; `icpath` compiles those as their abbreviated twins, and a
`Violation` entry point charges only the shapes that fail clause 1 or both
arms of clause 2 and is nil for everything else, `icpath.SelectorViolation`
enumerating which. A path `icpath` cannot read, or the matcher cannot
represent, is declined at compile time and the identity constraint carrying
it charges nothing, under the `GAP(xpath)` marker on `validate`'s
`icFrame.declined`.

## Validation (`validate`)

**Status: the engine ships the seam and charges the cvc- rules
`validate/doc.go` names, at the ·validation root· and at every descendant the
descent types; the XML adapter and the JSON adapter's first slice ship over
it.**
`validate` exports the infoset views (`Element`, `Attribute`, `Text`,
`Children`, `Child`, with `ElementChild`/`TextChild` constructing the sum),
the two optional document-level capabilities an adapter may implement
(`UnparsedEntities`, `DeclarationsProcessed`, narrowed off the root),
plus `New`/`Validator`/`Result`/`Option`/`WithLogger`, `SchemaView` (what
`Validator.Schema` returns: the schema reads an adapter makes) and `Unevaluated`
(`Rule`/`Loc`/`Msg`, read off `Result.Unevaluated`: one check the assessment
reached and did not perform, deliberately not an `error`), and `Assess` walks a
source once. **Read which rules it charges, which clauses of each, and which
are declined in `validate/doc.go`; do not enumerate them here.** A union-governed
value is classified by its ·validating type· (#813). Identity-constraint
`{selector}`/`{fields}` are compiled and evaluated directly as the
restricted path subsets of §3.11.6.2/§3.11.6.3 (`icpath`) and deliberately
NOT through the future XPath engine. **`validate/doc.go` is the authoritative per-rule statement and is
current; do not restate it here beyond this list.** The value-space
charges reach a `value.Backend` through `New`'s required second parameter,
which must be the backend the schema was compiled with.
`validate/xmlsrc` exports `Validate`/`Option`/`WithURI` and drives that walk
over an XML instance decoded by `parser/xmltree`; `validate/jsonsrc`
exports the same three and drives it over a JSON instance, on the arms its
`doc.go` names as implemented (M8, first slice); `validate/bersrc` is
`doc.go`-only (M11). Everything below not covered
by that seam is the destination, stated in present tense because it is a
design contract.

- Abstract infoset via marker interfaces; sources plug in as adapters:
  - `validate/xmlsrc` — XML instances via `parser/xmltree` (first; ships),
  - `validate/jsonsrc` — JSON instances mapped onto the same infoset
    (schema-aware member classification, scalar shorthand for simple
    content, arrays as repeated elements, null as `xsi:nil`),
  - `validate/bersrc` — BER-encoded instances (last; same infoset, TLV
    decode).
  The engine never imports a source's decoder; adapters build infoset
  values and hand them over.
- Content-model matching never backtracks (UPA fixes which particle takes
  an item; a set of live partitions decides which iteration it falls in);
  explicit content beats open-content wildcards at the current state.
- Streaming-oriented; parent element context is threaded from day one
  (ID/IDREF harvesting, EDC's post-`xsi:type` governing type, namespace
  context for identity constraints).
- Every violation is an `xsderr.Error` with a cvc rule ID + instance
  and/or schema location, reported in document order.

## Codegen & codec

**Status: neither package ships anything.** `codegen` (M9) and `codec`
(M10) are `doc.go`-only today — `go doc` renders no exported identifier for
either, and `value.Emitter` does not exist (its API is frozen for M9, not
declared). Everything below is the destination, stated in present tense
because it is a design contract; read it as "will", not "does" (steward
audit 2026-08-02).

- `codegen` emits Go types from a compiled schema, deterministically
  (D1/D2). Multiple schemas map to multiple output directories — one
  package per (schema set, target dir) pairing declared by the caller.
- **Type narrowing in interfaces** is the generated-code idiom:
  - **Choices are sealed interfaces.** An `xs:choice` becomes an interface
    with an unexported marker method; each branch is a concrete type
    implementing it, and consumers use type switches. This is the
    closed-sum exception to STYLE T2: exactly one branch can exist, so
    "N pointer fields, exactly one non-nil" never appears in generated
    code.
  - Generated readers/views expose the narrowest interface a consumer
    needs; optionality and nillability are modeled in types, not comments.
- **Anonymous types get ancestor-context names.** A single namer component
  owns all XSD-name → Go-identifier decisions. Anonymous types are named
  by walking up their schema ancestors to the nearest named declaration
  (element `shipTo` under element `purchaseOrder` → `PurchaseOrderShipTo`),
  extending the path only as far as uniqueness requires; residual
  collisions (case folding, Go keywords, XML-legal-but-Go-illegal names)
  are disambiguated deterministically by document order (D1/D2). Every
  generated type's header comment records its schema Loc + original QName.
- `codec` is the dataset serializer/deserializer: schema-directed decode of
  instance documents into generated (or reflective) Go values and canonical
  encode back out.

### Two decode paths, one semantics

`codec` is built for **minimal allocation**:

- **Runtime path** (always available): the facet pipeline +
  `value.Mapping`, driven by the compiled schema. General, reflective,
  allocation-tolerant.
- **Generated fast path** (planned): backends will export **code emitters**
  (`value.Emitter` — API frozen in M9, **not yet declared**; to be
  implemented by `builtin/strict` and `builtin/native`, and by user
  backends that want it). At codegen time the emitter
  contributes specialized decode/encode code for its types — parsing
  directly from the reader's byte window into the target field, no
  intermediate string, no boxed `value.Value`, facet checks inlined.
  A backend without an emitter simply falls back to the runtime path for
  its types.
- Runtime hot-path APIs will follow the appender convention
  (`AppendCanonical(dst []byte, v) []byte`, `ParseBytes(b []byte)`) so
  even the non-generated path can be allocation-frugal. **Neither method
  exists yet**; the convention is named here and in `codec/doc.go` so the
  first implementation does not invent a second spelling.

The two paths must implement the *same* pipeline stages with the *same*
spec rule IDs, which is what will make them **differentially testable**:
for every type, property tests feed identical input to both paths and
require identical values and identical error rule IDs, and
`testing.AllocsPerRun` benchmarks pin the fast path's allocation budget. A
fast path that disagrees with the runtime path is wrong by definition.
(No such test exists today — there is no second path to differ from.)

### Debuggability of parsing

When a value fails to parse, the error must localize the failure without
a debugger (extending E1–E3). These are **M10 requirements on `codec`**,
not descriptions of shipped behaviour — `GOXSD_DEBUG=codec` traces nothing
today because `codec` is empty:

- every decode error carries the **pipeline stage** that rejected
  (whitespace / pattern / lexical-map / facet / assertion), the type
  QName, the offending input fragment, and the instance Loc + byte offset;
- `GOXSD_DEBUG=codec` traces stage transitions per value through the
  injected slog logger (rule ID, type, input) so an agent can watch one
  value flow through the pipeline;
- generated code preserves this: emitted fast paths report the same
  stage/rule metadata as the runtime path, and generated files map cleanly
  back to the emitting backend and schema construct (a header comment per
  emitted decode function naming type QName + schema Loc).

## Exported surface without a consumer

STYLE T5 is the rule ("export nothing without a consumer"); this is the
standing list of places where the surface currently outruns it, so each
audit re-checks the same set instead of rediscovering it. Everything here
compiles, is documented, and has **zero** callers module-wide.

- **The annotation subsystem** — **deleted (#407); do not re-file.**
  `xsd/doc.go` states that §3.15 `{annotation}` is not modelled, because no
  rule consumes it.
- **`value.LexicalFacet` / `value.ValueFacet`** — the two pipeline-stage
  interfaces. Every implementation is unexported inside `value`, the
  assembling function (`compile`) is unexported, and no exported API takes
  or returns a stage, so no consumer can exist. Either grow the composition
  seam the docs promise, or unexport the interfaces.
- **`loader.FS`/`HTTP`/`Chain`/`Map`** — no non-test consumer, but
  **justified and not to be filed**: the resolver helpers are declared
  library surface in `loader/doc.go` for external users, which is the
  "documented contract it fulfills" half of T5. Re-check, do not re-file.
  `Chain` and `Map` returned to this list when `parser.ParseSet` replaced
  `cmd/goxsd8`'s in-memory wrapper (#1283); `Map` is still every `parser`
  test's resolver. `Dir` has consumers in both tiers, and `ResolverFunc` is
  exercised by `parser`'s tests.
  `regex.FlavorFO` sat here on the same terms until `xpath` and `icpath`
  took it for their NCName prefix scanners (#979; `validate`'s scanner moved
  to `icpath`); the F&O functions it was built for still arrive at M6/M7.
- **`xsd.AttributeResolver`** — whose godoc still names "the instance
  validator (a future consumer)" — has no consumer, `validate` having shipped
  and resolved attributes by another route. `validate.Validator.Schema()`
  left this list when `validate/jsonsrc` became its consumer through
  `validate.SchemaView`, which embeds `xsd.ElementResolver` (#848 re-scope).
- **`parser.Element` / `Node` / `Text` and the raw-tree accessors** — the
  bulk consumer is still `conformance`'s decidability model
  (`conformance/schema.go`'s `schemaShapeDecidable` and its predicates hold
  every `parser.Element` reference outside package `parser` itself;
  `parser.Node` and `Text.Data` have none anywhere). Not a tier violation —
  infrastructure may import the library — but the surface exists because
  that model has to re-derive the producer's coverage from the raw tree.
  **#1029** landed the signal that replaces it
  (`parser.AssembledDocument.Unmapped`, whose one consumer today is
  `conformance/census_test.go`'s one-directional soundness hold) and
  **#1051** is the blocked successor that deletes the model; `cmd/goxsd8`
  reads none of the raw tree since `parser.ParseSet` replaced its wrapper
  (#1283). `Text` has two readers inside `parser` (§4.2.5 override
  canonicalization and the `<xs:notation>` content check), so the annotation
  deletion left it a type with a job, and whether it stays EXPORTED is
  #1051's question alone.
- **`xsd.Occurs.Permits`, `xsd.Namespace.IsAbsent`,
  `xsd.NamespaceConstraint.AllowsNamespace`** — no non-test caller outside
  `xsd`; the first two have none inside it either. `Permits` answers a
  closed `min <= n <= max` question no caller asks, every occurrence site
  reading `Min`/`Max` for a one-sided bound instead; `IsAbsent` is `URI`'s
  second result spelled a second way (D3); `AllowsNamespace` sits, like
  `NamespaceConstraint.AllowsName` beside it, beneath the wildcard-admission
  entry points as the {namespace constraint} property's own accessor —
  `xsd/doc.go`'s Walk API section redirects a caller admitting a name to
  `Schema.ContentMatcher` or `Schema.AllowsAttributeWildcardName` instead of
  either. `AllowsName` stands the same way — in-package callers and
  `parser`'s tests only — and settles with it.
  Filed at the 2026-09-06 audit as #1287. `Notation.SystemIdentifier`,
  `Notation.PublicIdentifier` and `xmltree.CharData.Offset` are callerless
  too and are NOT filed: the first two are §3.14.1 component properties the
  model contracts to expose, and the third is the byte offset `codec`'s M10
  debuggability requirement names.
- **`xsderr.IsValidRule`** — **resolved (#273); re-check, do not re-file.**
  It still has zero non-test consumers, but the module-wide test
  `xsderr/doc.go` claims now exists and passes:
  `xsderr/rulecatalog_enforcement_test.go`'s
  `TestEveryRuleConstantIsInTheCatalog` and `TestNoRuleStringLiterals` scan
  every package. `IsValidRule` is that test's subject, which is T5's
  "documented contract it fulfills". The 63 bare string literals the last
  audit counted (50 in `builtin/strict`, 13 in `value`) are gone — a
  module-wide scan finds **zero** string literals in a `Rule` position.

## Conformance & ratchet

- W3C suite at `testdata/xsdtests` (submodule, pinned).
- Expectations committed at `conformance/testdata/expectations/*.txt`, one
  line per test case, one lane per file (`datatypes`, `schema`, `instance`,
  `xpath`, `json`, `ber`); diffs make regressions obvious and `git blame`
  bisectable.
- `go test ./conformance -run TestConformance -count=1` compares;
  the same command under `GOXSD_RATCHET=1` re-baselines **upward only**.
  A regression fails loudly and must never be committed. The one class
  that deletes a line is a sanctioned applicability removal — a case the
  suite's own `@version` metadata scopes away from an XSD 1.1 processor —
  and only against the arbiter's asserted per-lane count (issue #576;
  rules in `conformance/testdata/expectations/README.md`).

### Cohort isolation is deliberate (a steward ruling, 2026-07-19; re-confirmed 2026-08-16)

`conformance/datatypes.go` claims its cases as separate **cohorts** —
lexical, `<item>`, QName/NOTATION-context, Facets, NOTATION-Facets,
Saxon `PDecimal` — each with its own reader/decoder/executor triple and
its own XML decode structs (`lexicalInstance`, `itemInstance`,
`facetsInstance`, `notationStep`, …). This is isolation-over-DRY **by
design**, not accreted duplication, and future audits should leave it be:

- Each triple decodes a *distinct, static* W3C fixture shape. Those shapes
  are frozen input data, so the triples never have to change together — the
  upkeep coupling that makes duplication expensive is absent.
- All the actual datatype *semantics* funnel into shared library entry
  points (`value.ValidateLexical`, `value.Mapping`); a spec or pipeline
  change flows through the library, not through N decoders. Genuinely
  shared harness machinery (`childBindings`, `nsContext`, `facetChild`,
  `buildOwnFacets`) is already factored out and reused.
- The separation is a **regression firewall** serving the one rule: a new
  cohort (a new issue claiming a new suite directory) adds a triple without
  touching landed ones, so it cannot silently regress the ratchet.

The cost of merging the triples into one "universal" decoder — coupling
unrelated fixture shapes so one cohort's change can break another — exceeds
the cost of the parallel shapes. Re-flag only if a semantic rule ever has
to be edited in two triples to stay correct.

The same ruling covers the *inner* `Restriction{Base, Facets}` decode struct
that mirrors across the cohorts — and the mirror is three anonymous inline
structs plus one already-named type, not four identical anonymous ones, all in
`conformance/datatypes.go`: the inline one inside `lexicalSchema` (which
captures only `base`, that cohort's fixtures carrying no facets), the one
inside `pdecimalSchema`, the one inside `anyURISimpleType`, and the named
`d34Restriction` that `d34SimpleType.Restriction` points at. (Named, not cited
by line: the line numbers this paragraph used to carry were all stale within a
week — cite symbols in prose that outlives an edit.) Each
decodes the frozen `<restriction base=..>` shape of its own cohort's fixtures,
so the upkeep coupling that would make duplication expensive is absent at this
inner level for exactly the reason it is absent at the triple level: one
cohort's fixture handling can change without obliging an edit to another's.
STYLE T4 asks that near-identical structures be unified or the difference
explained — this paragraph is that explanation — and STYLE D3 is not in
tension, because each struct is the sole encoding of its own cohort's fixture
shape, not a second encoding of one shared fact. The section's re-flag
condition applies unchanged and extends to this level: re-flag only if a
semantic rule ever has to be edited in two of these restriction decoders (or
two triples) to stay correct.

## Logging

`log/slog` injected at construction, namespaced groups, silent by default.
The debug level is designed for agents: messages carry rule ID, component
QName, and location so a conformance failure can be localized from logs
alone (`GOXSD_DEBUG=parser,validate` in tests).
