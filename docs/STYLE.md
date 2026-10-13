# goxsd8 Go Style — Non-Negotiable

Violations are grounds for the arbiter to reject a change even if tests
pass. Each rule has an ID; reviews cite rule IDs. The rationale behind
these rules lives in docs/PRINCIPLES.md.

**Citing rules.** A style rule is cited by its letter ID from this file
(`STYLE S2`, `STYLE T4`) — never by a position in CLAUDE.md's "Style
headlines" list, which is a summary and carries no citable IDs.
`PRINCIPLES N` always means item N of docs/PRINCIPLES.md, whose numbering
is append-only. Both lists start at 1 and disagree about what the early
numbers mean (headline 5 "no cycle checks" is PRINCIPLES.md item 9), so a
positional citation of the headline list silently names the wrong
principle. The module-root `citations_test.go` and `stylecitations_test.go`
check that a cited number or ID exists; neither can check that it is the
rule the sentence means.

**In text no test reads — a verdict, an issue body, a commit message, a LOG
entry — write the rule's headline beside its ID**: `STYLE D3 (one fact, one
encoding)`, `PRINCIPLES 9 (phased construction beats cycle checks)`. A real
but wrong ID then contradicts the words next to it at the moment it is
written (#548). Copy the headline from this file's bold text at that ID,
opened for the purpose: a headline written from memory is the headline of
whichever rule memory supplies (#2569).

## Control flow

**S1. Happy path on the left.** The success path runs down the left margin;
deviations exit early.

**S2. No `else` blocks.** Invert the condition and return/continue early.
`else` after a terminating `if` body is dead weight; `else` after a
non-terminating one usually hides a second function trying to get out.

```go
// BAD
if ok {
    doThing()
} else {
    return err
}

// GOOD
if !ok {
    return err
}
doThing()
```

**S3. Errors are never dropped — especially in loops.** A `for` body that
can fail must either return the error, or accumulate into an explicit
`errs []error` / `errors.Join` that the function returns. `_ =` on an error
value requires a comment proving it cannot matter.

## Errors

**E1. Every error is decorated.** Wrap with what you were doing and to what:
`fmt.Errorf("resolving base type of %s: %w", name, err)`. An error that
surfaces to a user must let them find the schema construct that caused it
without a debugger.

**E2. Errors map to spec validation rules.** Anything that represents a
schema or instance validity violation the spec catalogs is an
`*xsderr.Error` carrying the spec rule ID (`cvc-…`, `cos-…`, `src-…`,
`derivation-ok-…`). One rule ID per error; if you can't name the rule, you
haven't read the spec section yet — unless it is the one violation class the
spec catalogs nowhere: a schema document that is simply not valid against
the schema for schema documents (§2.4 clause 1 `sd-valid`) — a prohibited or
missing attribute, a child its content model does not admit, one repeated
past its maxOccurs, one out of order. Reject that as a plain error naming
the offending item, its location and the Appendix A production it violates;
never as an `*xsderr.Error`, never on `sd-valid` as a rule ID, and never on
a borrowed `src-*`. `xsderr`'s package doc owns the derivation.

**E3. Errors carry location.** Schema errors carry the schema document URI +
line + column (from `parser/xmltree` positions). Instance errors carry the
instance location. `xsderr.Loc` is threaded, not reconstructed.

**E4. A message names its clause inline, spelling the rule ID out with it** —
`"…, but src-simple-type clause 3 allows only one"` — and never as a leading
`"clause 3: …"` label, so the citation survives being read apart from
`Error()`'s rendered `[rule]` prefix and a grep for the full rule-and-clause
finds the site (#759).

**E5. A message leads with its fact, then cites.** Open every `Msg` with the
offending item, its value and what it should have been, and close it with
E4's citation as a clause of the same sentence: `"…, which cvc-type clause
3.1.3 requires"`, `"…, but cvc-pattern-valid requires one to match"`. A spec
term of art goes after the fact or nowhere. Cite by rule ID and clause alone:
no `§` number, and never a trailing parenthetical `(rule, §N)`, which does not
conform to E4. The order binds each message, not the rendered line, so a
delegating message names the item and value in its own fact, and its wrapped
cause follows after `": "` (validate's `causedBy`) as a message of its own,
keeping its own `[rule]` prefix and its own citation. `Error()`'s `[rule]`
prefix and the inline rule ID both stay (#2311). Target shape:

```
<loc>: [cvc-type] element n's value "x3" is not a valid <int>, which cvc-type clause 3.1.3 requires: [cvc-pattern-valid] "x3" matches no member of the pattern facet of <integer>, whose {value} holds <pattern>, but cvc-pattern-valid requires one to match
```

## Data & determinism

**D1. Deterministic output, always.** Identical inputs produce byte-identical
output: generated code, canonical serializations, error lists, iteration
order of reported problems.

**D2. Never iterate a map into output.** Maps are allowed only as internal
lookup indexes. Anything ordered — child components, facets, errors,
generated declarations — lives in a slice, in document order (or a
spec-defined order). If you must drain a map, sort first and justify why a
slice wasn't kept alongside.

**D3. One fact, one encoding — no derivable state.** Do not store what can
be computed from what you already store, and never keep two encodings of
the same fact. No `Primitive bool` next to fundamental facets that already
imply it — a type that defines its own fundamental facets *is* a primitive;
expose `IsPrimitive()` as a derived method if callers need the answer. No
memoized caches without a profile showing a hot path. Two encodings of one
fact will drift; fewer fields, fewer invariants, fewer bugs.

This binds prose as well as fields. A fact stated in a doc comment, a test
comment or a package doc is stated once and the other sites point at it — a
paraphrase is a second encoding, so a verdict that corrects one copy leaves
the reviewer no text to grep for the rest, and the copy that survives
longest is the one inside the test that pins the behaviour (#925). Leave a
package's export inventory to `go doc`: a count of its exported identifiers,
or a list that reads as all of them ("Its top-level exports are …"), is a
second encoding, and prose names an exported identifier only where it states
the consumer that uses it.

**D4. No cycle checks — build in phases.** Structure construction so cycles
cannot exist at traversal time: parse into raw documents, resolve references
via named placeholders, then finalize components in dependency order.
A traversal that needs a `seen` set is a design smell; fix the construction
phase instead. (Where the spec itself permits cycles — e.g. circular
substitution-group or union checks the spec forbids — detect them once at
construction with a named `src-`/`cos-` rule error, then never again.)

**D5. No concurrency.** The parser, validator, and generators are pure
single-threaded transforms: no goroutines, channels, or locks in library
code. Determinism and simplicity outrank parallel speed; revisit only with
a measured, documented need — and then behind a seam, never scattered.

## Types & APIs

**T1. Illegal states unrepresentable.** Unexported fields + constructors
that validate. Closed sets are types with private tag fields, not `string`.
Mutually exclusive fields become a sum-style interface or separate types.
If a comment says "only valid when…", redesign.

**T2. Capabilities are interfaces, not type switches.** Value comparison,
length, digit counting, timezone-awareness etc. are small interfaces
(`value.Ordered`, `value.Lengthed`, …). A `switch v := v.(type)` over
concrete value types outside the defining package is a bug factory —
it silently excludes user-defined types.

*Exception — closed sums:* a set closed by the schema itself (an
`xs:choice` group in generated code, a variety) is a **sealed interface**
(unexported marker method), and consumers type-switch over its branches.
That is the Go sum type and it serves T1: the open/capability rule applies
to *extensible* sets, the sealed/switch rule to *closed* ones. Never mix
them up in either direction.

**T3. Minimal interfaces at boundaries.** Expose the narrowest capability
the consumer needs (a schema view that answers only `ElementByName`), not
the whole object.

**T4. No duplicate structures.** Before adding a type/function that looks
like an existing one, unify or explain in the commit message why they must
differ. Parallel near-identical code paths (two matchers, two resolvers)
rot independently.

**T5. Export nothing without a consumer.** Every exported identifier needs
a justification — a real caller, or a documented contract it fulfills — and
a doc comment (the lint gate enforces the comment; the arbiter reviews the
justification). The exported surface IS the product the library user sees
via `go doc`; every addition is a compatibility promise. Reviews inspect
the exported-surface diff of every change.

**T6. Package-doc status tells the truth about what ships today.** A
package `doc.go`'s status prose — "Current coverage", "Contract (implemented
in M…)", coverage lists, "ships in-repo" claims — describes what `go doc`
renders *now*, not the roadmap. Prose describing surface that does not yet
exist says so in its heading ("Planned contract (M… — not yet implemented)");
never date unshipped prose "implemented". Any diff that changes a package's
exported or implemented surface reconciles that package's status prose in the
*same commit* — render `go doc` for every package you touch and confirm the
coverage claim matches before you report it updated. The forward-looking
design prose is valuable: relabel it planned, don't delete it.

## Spec fidelity

**P1. Stick to the spec.** The local specs in `docs/specs/md/` are ground
truth, not intuition, not other implementations. When behavior is surprising,
quote the clause in the commit message.

**P2. Comment only constraints.** Code comments state what the code cannot:
spec rule being implemented, invariants, why a spec-deviation is deliberate.
Never narrate the next line, and never carry a `Ratchet impact` paragraph:
what a change moved belongs to its commit trailer and `docs/LOG`, and a
comment stating lane effects is stale one landing later (PRINCIPLES 32,
#622).

**P3. Deliberate gaps are tracked, and `GAP(` is the only token that
tracks them.** Every unsupported-construct fallback — fail-open XPath
first, but any deliberate incompleteness anywhere — carries
`// GAP(<area>): <construct>` so gaps are greppable and ratchetable. The
parenthesis is load-bearing: the debt sweep is `grep -rn "GAP("`, so
`// GAP:` without parens, or a marker spelled `PARTIAL`/`TODO`/`XXX`, is
invisible to it and does not count as tracked debt no matter how honest
the prose around it is. `<area>` is the package or spec area
(`GAP(xpath)`, `GAP(xsd)`, `GAP(datatypes)`), never a rule ID or clause
number — rule IDs go in the text after the colon. A gap whose retirement
is owned by an issue names that issue in the text, and the issue must
still be open: a marker pointing at a closed issue is a dead end, so
repoint it in the landing that closes the owner.

Prose that discloses a gap — a decline, a fail-open, an uncharged rule, a
test asserting the incomplete behaviour — sits in the same comment block as
its marker; prose with no marker beside it is untracked however honest it
is (#960). A diff that routes new input through a known defect has widened
that gap: it owes a marker at the site it widened, and a test fixture
chosen to avoid the defect says in its comment that the avoided verdict is
wrong (#1084).

**P3a. A claimed direction names the consumers it quantifies over.** A
marker or comment that asserts a gap's error direction — "fail-open",
"never a false reject", "can only cost a win" — must list the readers of
the value it withholds or adds, by their actual identifiers, and state
which direction each of them charges in. A gap is fail-open only against
the WHOLE consumer set: one reader that charges on an EXTRA member makes
an under-application fail-CLOSED, and one whose rejection condition IS
the withheld value makes a withholding fail-CLOSED. The unenumerated
form — "every consumer of X charges on a missing member" — is not an
acceptable claim, because it is exactly the sentence that gets written
when only the consumers in view were checked. Write the identifiers, or
write no direction at all and say the direction is unestablished. Neither
the gate nor the ratchet can check a direction claim: the corpus does not
contain the shapes (#403).

**P3b. A gap RULED permanent cites the ruling, as `RULED permanent by
#N`.** A ruling is the finding that the gap stays, so it is not a
retirement owner and P3's repointing has no target: the issue that
recorded the ruling closes as completed, and it stays the marker's
citation. Write the phrase immediately before the number — `go tool
gapaudit` reads a citation in that form as tracked whatever the cited
issue's state, and a closed one in any other form as P3's dead end. Dropping
the number instead is not the fix: the ruling's grounding and the
measurements that reopen it live on that thread, and the marker is the
only route to them. Nothing else in the marker is audited once the ruling
retires it, so a second number there is provenance and must read as
provenance (#1376). That is a prose control, not a mechanized one: a second
number's role — provenance beside a ruling, or a stale owner — is read
from the marker's own words, and `go tool gapaudit` has no signal to tell
the two apart (#1619).

**P4. Stream from the start.** Bounded memory on every input path: no
`io.ReadAll`, no whole-document buffering. Position tracking uses an
offset index over the stream, not retained content.

## Enforcement

The machine-checkable subset runs via `.golangci.yml` (`go tool lint`):
errcheck/errorlint/nilerr (S3, E1), revive
early-return/superfluous-else/indent-error-flow/exported (S1, S2, T5),
exhaustive (T1/T2 closed sums), sloglint no-global (L1), forbidigo banning
io.ReadAll and fmt.Print* in library code (P4, L1), plus govet/staticcheck/
unused/ineffassign/bodyclose and gofmt. Everything needing judgment — T4
duplicates, D2 map-iteration-into-output, D3 derivable state, D5, E2 rule
mapping, T5 justification — is the arbiter's and warden's job; keep the
linter set lean rather than approximating those.

## Logging

**L1. `log/slog` only,** through a logger accepted at construction
(`WithLogger` options), never a package-global. Components log under
namespaced groups (`parser`, `validate.cvc`, `xpath`). Debug logs must be
rich enough for an agent to localize a conformance failure without adding
prints: include rule ID, component QName, and location in every message.
Silent by default (`slog.DiscardHandler` when nil).
