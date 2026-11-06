# goxsd8

An **XSD 1.1 processor for Go**: schema parser, instance validator
(XML, JSON, and BER sources), XPath 2.0 engine, `precisionDecimal`
support, and a code generator that emits allocation-frugal, type-safe
marshalling code.

> **Status: early.** XML and JSON instances validate today; BER is
> planned (M11). The code generator ships Go type declarations for a slice
> of schema shapes, and package `codec` decodes a valid XML instance into
> them for a narrower slice still, with no encoder yet (M10). The XPath engine evaluates a conditional type assignment's
> `{test}` over Structures §3.12.6's restricted subset, less some forms it
> declines (M6), and an assertion's `{test}` in first slices (M6), reporting
> one it declines as unevaluated, never as satisfied; the full XPath 2.0
> grammar and function library are planned (M7 onward). Implementations
> land milestone by milestone (see [docs/PLAN.md](docs/PLAN.md)). Every
> package's `doc.go` states its committed contract — `go doc` is the source
> of truth for what each package will do.

## What it does (the contract)

- **Parse** one or more XSD 1.1 schemas — imports, includes, redefines,
  overrides, chameleon composition — into one immutable, deterministic
  component model with **query** (lookup by name) and **walk**
  (content-model traversal) APIs.
- **Validate** instance documents against the compiled set. Sources are
  adapters over one abstract infoset: XML, JSON, and BER.
  Every violation carries the spec rule ID (`cvc-…`) and an exact
  file:line:column location.
- **XPath 2.0** for assertions and conditional type assignment — the
  required subset first, growing to the full grammar and F&O function
  library, conformance-tracked.
- **Generate Go code**: `xs:choice` becomes a sealed interface (one
  concrete type per branch — no "five pointers, one non-nil" structs),
  values typed by your chosen backend, decode paths specialized for
  minimal allocation. Multiple schemas map to multiple output
  directories.
- **Bring your own value backend.** One ships today — `builtin/strict`
  (spec-exact: arbitrary precision, `precisionDecimal`, seven-property
  temporal model); a second, `builtin/native` (Go-friendly: `int64`,
  `time.Time`, documented deviations), is a fixed planned contract
  (M12 — see its `doc.go`). Backends are composable per type, and
  `value/backendtest` certifies third-party backends.

## Quickstart

```sh
git clone https://github.com/kud360/goxsd8
cd goxsd8
git submodule update --init testdata/xsdtests   # W3C suite, ~215 MB
go build ./... && go test ./...
go install ./cmd/goxsd8                         # needs Go 1.26 or newer, as go.mod sets
export PATH="$(go env GOPATH)/bin:$PATH"
command -v goxsd8                               # prints the binary's path
```

`go build ./...` compiles every package and writes no executable; the
`go install` line is what produces the `goxsd8` binary the CLI section below
invokes. It writes the binary to `go env GOBIN`, or to `$(go env GOPATH)/bin`
when that is empty, and neither is on `$PATH` by default: the `export` line
adds the default one, so put `go env GOBIN` there instead when it is set.

### CLI (contract; `parse`, `validate` and `gen` are implemented)

This block is a **contract**, exactly like the Library one below, and it
lives in one place: `goxsd8 -help` prints its usage block, and
**`go doc github.com/kud360/goxsd8/cmd/goxsd8` renders the whole contract** —
that usage block plus what `-help` does not print: the argument vocabulary,
the shape of a diagnostic line and the CLI's relationship to the library. The
invocations below are worked examples of it.

Every file these examples and the Library ones name is committed in
`testdata/readme/`; run them from there. `gen` writes `./gen/` beside them,
which the checkout ignores. Every `-out` lies in a Go module, here the
checkout's, as the `gen` paragraph of `goxsd8 -help` states.

```sh
cd testdata/readme                              # from the checkout root
goxsd8 parse order.xsd items.xsd                # two compilations, not one set; a summary each, exit 0
goxsd8 validate -schema order.xsd -schema items.xsd order.xml order.json
                                                # one set; exit 0 (codes: the table in goxsd8 -help)
goxsd8 gen -schema order.xsd -out ./gen/order \
           -schema items.xsd -out ./gen/items  # packages order and items; exit 0
```

`order.xsd` declares `{urn:o}order` and `items.xsd` `{urn:i}items`, unrelated
schemas; `order.xml` and `order.json` are the same order. A JSON instance
validates the way an XML one does. `order.xsd` is this schema:

```xml
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
           targetNamespace="urn:o" elementFormDefault="qualified">
  <xs:element name="order">
    <xs:complexType>
      <xs:sequence>
        <xs:element name="item" type="xs:string" maxOccurs="unbounded"/>
      </xs:sequence>
      <xs:attribute name="kind" type="xs:string"/>
    </xs:complexType>
  </xs:element>
</xs:schema>
```

and `order.json` this instance:

```json
{"o:order":{"$xmlns":{"o":"urn:o"},"kind":"x","item":["a","b"]}}
```

```sh
goxsd8 validate -schema order.xsd order.json    # exit 0
```

This is not arbitrary JSON and not JSON Schema: the instance is read as the
XML document `validate/jsonsrc`'s mapping defines, where the top-level
object's one member names the root element and, in an element's object,
every member but `"$xmlns"` and `"$type"` is an attribute or a child. The root
key `"o:order"` is prefixed through `"$xmlns"` so its namespace is spelled,
though an unprefixed `"order"` also matches the top-level declaration of that
local name, targetNamespace or not, unless two top-level declarations carry
it (jsonsrc's "The validation root"). A number's lexical form is its source
digits, never a float64 round trip (`1.0` stays `1.0`, `1e2` stays `1e2`),
and `true` and `false` are those literals, so a JSON boolean is an
`xs:boolean` lexical form.

A child key may repeat in one object: each member is one more child, in
member order, which is how a model interleaving repeats (a, b, a) is
spelled. A repeated attribute key, `"$xmlns"` or `"$type"` is declined
(exit 4). Malformed JSON and a fault reading the instance each print one
`<loc>: [jsonsrc] <message>` line on stdout and exit 1, as malformed XML and
a fault reading an XML instance do; a decline prints
`<loc>: [jsonsrc] undecided: <message>` and exits 4.
`jsonsrc` names the source of the error where a violation's line names its
rule. `twice.json` is
`{"o:order":{"$xmlns":{"o":"urn:o"},"kind":"x","kind":"y","item":["a"]}}`:

```sh
$ goxsd8 validate -schema order.xsd twice.json    # exit 4
twice.json:1:47: [jsonsrc] undecided: attribute member "kind" repeats in the object of {urn:o}order; an element carries one attribute of each expanded name, so give it once
```

`goxsd8 -help` states the JSON shape in brief, and
`go doc github.com/kud360/goxsd8/validate/jsonsrc` renders the full mapping.

### Library

Seeding the builtin datatypes and compiling a schema set both work TODAY.
A program in your own module depends on the library through one line, run in
that module's root, and needs no `replace` directive:

```sh
go get github.com/kud360/goxsd8@main            # in YOUR module, not this checkout
```

The program below and its continuations open the CLI section's files, so run
it with `testdata/readme/` as the working directory. That directory sits
inside the goxsd8 module, so the program needs no `go get`:

```go
package main

import (
	"fmt"
	"log"
	"log/slog"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
)

func main() {
	// A value backend supplies the builtin datatypes' value spaces. builtin.Seed
	// needs one that maps all 20 builtin primitives; builtin/strict maps all 20 on
	// its own, so nothing has to be composed in.
	backend := strict.New()

	// Seed the builtin datatype components. Call this when you want the
	// components yourself — parser.Parse seeds its own from its backend.
	builtins, err := builtin.Seed(backend) // []*xsd.SimpleType, deterministic order
	if err != nil {
		log.Fatal(err)
	}

	// parser.Parse assembles the <xs:include> / <xs:import> / <xs:override>
	// closure of the root document — including chameleon coercion of a
	// no-targetNamespace included (or overridden) document into the including
	// namespace — and returns it finalized. Every option has a default, so
	// parser.Parse("order.xsd") alone is valid too.
	schema, err := parser.Parse("order.xsd",
		parser.WithBackend(backend),          // default: builtin/strict
		parser.WithResolver(loader.Dir(".")), // default: loader.Dir("."), the working directory
		parser.WithLogger(slog.Default()),    // default: silent
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("builtin types:", len(builtins), "top-level elements:", len(schema.Elements()))
}
```

To back one type with your own mapping and inherit the rest, compose:
`value.Override(strict.New(), money)` yields `money`'s mapping for every
type `money` defines and `strict`'s for all the others.

One limit of `parser.Parse` worth knowing up front: it returns only the
FIRST error, not a list of them.

`parser.ParseReport(location string, opts ...Option) (*xsd.Schema,
*AssemblyReport, error)` returns that report alongside the schema, and
`parser.Parse` is it without the report. Reach for it to diagnose an assembly that came back smaller than
expected: `AssemblyReport.Documents()` lists the readings the assembly
performed, and `Unfollowed()` the `xs:include` / `xs:redefine` /
`xs:override` / `xs:import` references that yielded no document, each with
the directive element's position and the reason — a `schemaLocation` that
resolved to nothing, or a bare `xs:import` naming no document at all, which
§4.2.6.2 makes legal. The report is never nil and is populated as far as
assembly got even when an error comes back; `go doc
github.com/kud360/goxsd8/parser AssemblyReport` is its contract. It does not
lift the first-error limit above — `ParseReport` stops at the first error
too, so directives it never reached are neither followed nor reported.

Several schema documents compiled as one set go through
`parser.ParseSet(roots []parser.Root, opts ...Option)`, which returns the
same three values. `parser.RootAt(location)` names a document the caller
vouches for, so failing to read it is an error; `parser.HintAt(namespace,
location)` names a schema location hint whose document may legally be
absent, and is then skipped and listed by
`AssemblyReport.UnfollowedRoots()`. `Parse` and `ParseReport` are
`ParseSet` over one `RootAt`. Sharing a set licenses nothing: a root whose
QNames name a namespace other than its own target namespace (or the XSD and
XSI namespaces) — another root's, or a document another root includes —
must `xs:import` that namespace in its own document, `<xs:import
namespace="urn:b"/>` with no `schemaLocation`, or `ParseSet` charges
`src-resolve`. `go doc github.com/kud360/goxsd8/parser ParseSet` is the
contract.

The component model is also constructible directly, without a schema
document: `xsd.NewSchemaBuilder()` → `Add*` → `Finalize()` returns an
immutable `*xsd.Schema` you query by `xsd.QName` (`Type`, `Element`,
`Attribute`). That builder is PRODUCER surface, not application surface:
every `Add*` takes an already-validated component value, and building one
correctly means honoring every §3 tableau and cross-property invariant its
constructors cannot check — which is `parser.Produce`'s job. An application
that has a schema DOCUMENT calls `parser.Parse` and receives the finalized
`*xsd.Schema`. See `go doc github.com/kud360/goxsd8/xsd SchemaBuilder`, and
`Example_buildFinalizeQuery` in `xsd/example_test.go` for the construct →
`Finalize` → query sequence.

Instance validation runs today: `validate.New` builds the engine and
`validate/xmlsrc` drives an XML document through it. `go doc
github.com/kud360/goxsd8/validate`'s "Contract (M5, landing rule by rule)"
section is the authoritative account of which `cvc-` rules are charged and
what is left undecided — this block only shows the calls. It continues the
program above, with `schema` and `backend` in scope, and adds `os`,
`github.com/kud360/goxsd8/validate` and
`github.com/kud360/goxsd8/validate/xmlsrc` to its imports:

```go
// backend MUST be the one the schema was compiled with: a different value
// space rejects documents the schema admits, and nothing here can detect it.
v, err := validate.New(schema, backend)
if err != nil {
	log.Fatal(err)
}

f, err := os.Open("order.xml")
if err != nil {
	log.Fatal(err)
}
defer f.Close()

// err means no assessment stands: a nil argument, or a document that is not
// well-formed where the walk did not read it — before the document element,
// after it, or in a subtree the walk did not enter. A fault that stopped the
// walk lives in res.Err() alone and is never also returned here.
res, err := xmlsrc.Validate(v, f, xmlsrc.WithURI("order.xml"))
if err != nil {
	log.Fatal(err)
}
// Standing() is the acceptance test, not an empty Violations(): it is Charged
// when a violation was charged, Undecided when none was but a check was left
// unperformed or res.Err() stopped the walk, and NotProvenInvalid otherwise,
// which means not proven invalid rather than valid.
switch res.Standing() {
case validate.Charged:
	fmt.Println("charged")
case validate.Undecided:
	fmt.Println("undecided")
case validate.NotProvenInvalid:
	fmt.Println("not proven invalid")
}
for _, e := range res.Violations() { // []*xsderr.Error, in document order
	fmt.Println(e)
}
if err := res.Err(); err != nil { // the assessment is INCOMPLETE, not invalid
	fmt.Println(err)
}
for _, u := range res.Unevaluated() { // no String method: this is the CLI's form
	fmt.Println(fmt.Sprintf("%s: [%s] undecided: %s", u.Loc(), u.Rule(), u.Msg()))
}
```

A JSON instance validates through the same `v` today: `validate/jsonsrc`
maps it onto the infoset, read as the CLI section's JSON example says, and
hands it to the same engine, so its violations
carry the same `cvc-` rule IDs, located by jsonsrc's line and byte column.
`go doc github.com/kud360/goxsd8/validate/jsonsrc` opens on the JSON shape in
brief. Continuing the program, add `errors` and
`github.com/kud360/goxsd8/validate/jsonsrc` to its imports:

```go
jf, err := os.Open("order.json") // the CLI section's order.json
if err != nil {
	log.Fatal(err)
}
defer jf.Close()

// err is every fault, with no result: a nil argument, malformed JSON or
// content after it, a read fault, or a decline of an input the mapping does
// not map. res.Err() is nil on every result jsonsrc returns, so Standing() is
// Undecided by Unevaluated() alone. Pass WithURI: without it, a decline or a
// violation is located at ?, as in ?:1:2.
res, err = jsonsrc.Validate(v, jf, jsonsrc.WithURI("order.json"))
// A decline is no verdict on the document: report it as undecided, in the
// line the CLI prints for it before exiting 4.
var je *jsonsrc.Error
if errors.As(err, &je) && je.Class() == jsonsrc.Decline {
	fmt.Printf("%s: [jsonsrc] undecided: %s\n", je.Loc(), je.Msg())
	return
}
// Every other error stays fatal. The CLI exits 1 on malformed JSON and on a
// read fault.
if err != nil {
	log.Fatal(err)
}
// Read res.Standing(), res.Violations() and res.Unevaluated() as above.
```

Code generation runs today for a narrow slice of schema shapes:
`codegen.Generate` emits Go type declarations for that slice and declines
every other construct. `go doc github.com/kud360/goxsd8/codegen`'s "Coverage"
section states the slice, and the program's `order.xsd` is inside it, as is
`codegen/testdata/example.xsd`, which `ExampleGenerate` runs. The emitted
structs carry no struct tags and bind no element or attribute QName, so
`encoding/xml` does not round-trip them: do not decode them with
`encoding/xml`, because filling them is package `codec`'s (M10), whose
`codec.Decode` fills the shapes its package doc's "What decodes" states,
README's `Order` among them; `codegen`'s package doc, section "What you get",
states this and the Go shape each construct gets. `Generate` requires each `Dir`,
resolved against the working directory, to lie at or below a go.mod, whose module path
and `Dir`'s path below it give the package's import path (`Target.Dir`'s
doc), so the program runs in its module's root or any directory below it.
Continuing the program, add
`github.com/kud360/goxsd8/codegen` to the imports:

```go
// Writes package order into gen/order.
if errs := codegen.Generate([]codegen.Target{{Schema: schema, Dir: "gen/order", Package: "order"}}); errs != nil {
	// Generate returns one *codegen.TargetError per failure: print each, a
	// decline by its Loc and Construct, any other failure as it reads.
	for _, e := range errs {
		var de *codegen.DeclineError
		if errors.As(e, &de) {
			fmt.Printf("%s: codegen declined %s\n", de.Loc(), de.Construct())
			continue
		}
		fmt.Println(e)
	}
	os.Exit(1)
}
```

For the program's `order.xsd`, `Generate` writes this
`gen/order/xsd_gen.go`:

```go
// Code generated by github.com/kud360/goxsd8/codegen. DO NOT EDIT.

package order

// Order is generated from element {urn:o}order declared at order.xsd:3:3.
type Order struct {
	Item []string
	Kind *string
}
```

A header renders the `Loc` the parser gave the document: as is for a relative
loader key, such as the program's `order.xsd` under `loader.Dir(".")`, and
relative to the target's `Dir` for an absolute path (`codegen`'s package doc,
section "Naming"). `goxsd8 gen` parses each `-schema` by its absolute path, so
its headers name each document by its path relative to the `-out` directory,
climbing with `../` out of it, as `goxsd8 -help` states: the CLI section's
`gen` run writes `declared at ../../order.xsd:3:3`.

`codec.Decode` validates an instance against the schema and decodes it into
the generated type, here `order.Order`; `go doc
github.com/kud360/goxsd8/codec Decode` states every error it returns. Once
`Generate` has written `gen/order`, a program in the same module with
`schema` and `backend` as above decodes `order.xml` into it, importing
`github.com/kud360/goxsd8/codec`, `github.com/kud360/goxsd8/xsderr` and the
generated package, whose import path is the one `Target.Dir`'s doc derives
for the `Dir` that `Generate` was given:

```go
data, err := os.ReadFile("order.xml")
if err != nil {
	log.Fatal(err)
}
// order.Order is the generated type for order.xml's document element.
o, err := codec.Decode[order.Order](schema, backend, data, codec.WithURI("order.xml"))
if err != nil {
	var ue *codec.UndecidedError
	var de *codec.DeclineError
	rule, _ := xsderr.RuleOf(err)
	switch vs := codec.ViolationsOf(err); {
	case vs != nil:
		// Item 3: every violation of a rejected instance.
		for _, v := range vs {
			fmt.Println(v.Loc, v.Rule)
		}
	case rule == xsderr.RuleXMLWellFormed:
		// Item 2: the instance's source fault.
		fmt.Println(err)
	case errors.As(err, &ue):
		// Item 4: the check the validator left undecided.
		fmt.Println(ue.Unevaluated().Loc(), ue.Unevaluated().Rule())
	case errors.As(err, &de):
		// Items 5 and 7: the construct codec declined, charged to an
		// instance Loc.
		loc, _ := de.Loc()
		fmt.Println(loc, de.Construct())
	case errors.Is(err, codec.ErrTypeMismatch):
		// Item 6: T does not match the declaration.
		fmt.Println(err)
	default:
		// Item 1's and item 5's plain errors, and item 8's fault of the
		// backend.
		fmt.Println(err)
	}
	log.Fatal(err)
}
fmt.Println(o.Item, *o.Kind) // [a b] x
```

Two targets can compose one document. `ship.xsd` and `bill.xsd` each import
`note.xsd`:

```xml
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:n">
  <xs:element name="note" type="xs:string"/>
</xs:schema>
```

`ship.xsd` and `bill.xsd` are these schemas:

```xml
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:s">
  <xs:import namespace="urn:n" schemaLocation="note.xsd"/>
  <xs:element name="ship" type="xs:string"/>
</xs:schema>
```

```xml
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:b">
  <xs:import namespace="urn:n" schemaLocation="note.xsd"/>
  <xs:element name="bill" type="xs:string"/>
</xs:schema>
```

```sh
goxsd8 gen -schema ship.xsd -out ./gen/ship \
           -schema bill.xsd -out ./gen/bill    # from testdata/readme; exit 0
```

Package `ship` declares `type Note string`; `gen/bill/xsd_gen.go` imports it
and aliases its type, because the two `-out` directories lie under the same
nearest go.mod and the declarations each would emit for `note.xsd` agree
(`Generate`'s doc owns both conditions; the checkout's go.mod gives the import
path):

```go
// Code generated by github.com/kud360/goxsd8/codegen. DO NOT EDIT.

package bill

import ship "github.com/kud360/goxsd8/testdata/readme/gen/ship"

// Bill is generated from element {urn:b}bill declared at ../../bill.xsd:3:3.
type Bill string

// Note is generated from element {urn:n}note declared at ../../note.xsd:2:3.
type Note = ship.Note
```

Start at `go doc github.com/kud360/goxsd8` and follow the package list;
each package's godoc is its contract. Plain-text `go doc` does not print
the runnable `Example*` funcs; pkg.go.dev
(`https://pkg.go.dev/github.com/kud360/goxsd8`) renders them beside each
package's documentation. For working, tested end-to-end code (seed builtins →
parse a lexical → assert capabilities) in a checkout, read the example tests
directly: `validate/jsonsrc/example_test.go` (`ExampleValidate`),
`validate/example_test.go` (`ExampleValidator_ConditionallySelected`,
`ExampleResult`),
`value/example_test.go` (`ExampleOverride`, `ExampleValidateLexical`),
`builtin/example_test.go` (`ExampleSeed`, `ExampleSeed_missingPrimitive`),
`builtin/strict/example_test.go` (`ExampleNew`),
`codegen/example_test.go` (`ExampleGenerate`),
`codec/example_test.go` (`ExampleDecode`),
`loader/example_test.go` (`Example_chain`), `xsd/example_test.go`
(`Example_termOrRefDiscrimination`, `Example_buildFinalizeQuery`,
`Example_schemaEnumeration`, `Example_contentModelTraversal`,
`Example_modelGroupScopeParent`), and `xsd/namespaceconstraint_test.go`
(`ExampleNamespaceConstraint_AllowsNamespace`).

## Documentation map

| Doc | What it holds |
|---|---|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | package DAG, facet pipeline, backends, codegen design |
| [docs/STYLE.md](docs/STYLE.md) | non-negotiable code rules (cited by ID in reviews) |
| [docs/PRINCIPLES.md](docs/PRINCIPLES.md) | the invariants and spec traps behind the rules |
| [docs/PLAN.md](docs/PLAN.md) | roadmap M0–M12 |
| [docs/WORKFLOW.md](docs/WORKFLOW.md) | the rules every development session obeys |
| [docs/ROUTINES.md](docs/ROUTINES.md) | running the loop on Claude routines |
| [docs/specs/](docs/specs/README.md) | the local W3C specs (ground truth, greppable) |

## Conformance

The W3C XSD test suite (pinned submodule) drives a **ratchet**: expected
outcomes are committed per lane under
`conformance/testdata/expectations/`, regressions fail CI loudly, and
expectations only ever move up. See
[conformance's godoc](conformance/doc.go).

```sh
go test ./conformance -run TestConformance -count=1
```

## How this repo is developed

goxsd8 is built primarily by AI agents — scheduled Claude Code routines
running the slash commands in `.claude/commands/` (`/develop`, `/backlog`,
`/ratchet`, `/retro`, `/story`), with specialized personas in
`.claude/agents/` (implementer, judge, spec oracle, API warden, planner,
architecture steward, historian, and two simulated users who test the
docs you are reading right now). GitHub issues are the project's memory; every change is
judged against [docs/STYLE.md](docs/STYLE.md) and the conformance
ratchet before it lands. Humans are welcome — file issues, or run the
same commands locally.

## License

Apache 2.0 — see [LICENSE](LICENSE).
