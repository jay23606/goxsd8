// Command decodecensus measures codec over the W3C suite: of the instance
// cases whose schema codegen generates, how many codec.Decode decodes into the
// type codegen emitted for the document element's declaration, how many it
// declines, counted by the first construct each declines and largest first,
// and how many fail any other way. It is M10's measure, as `go tool
// gencensus` is M9's.
//
// # Population and assembly
//
// The population is the instance lane's: every instanceTest entry
// conformance.Catalog reads from the suite at the root and does not mark
// withheld. A case's schema is its test group's <schemaDocument> list,
// assembled by assess.Assemble: the first document through loader.Dir over
// its own directory, then each further one its closure did not read. The
// instance lane assembles more widely than that, as a caveat the report prints
// states. A group's schema generates when it assembles and codegen.Generate
// emits it, as one target, a package of its own in a temporary module, with no
// error; a case of a group declaring other than one schemaTest has no schema
// and does not generate.
//
// # Decoding
//
// codec.Decode fills a compiled T, so the census compiles what it generated:
// the temporary module, at the path driverModule names, requires this module
// by a replace directive on the module root `go env GOMOD` names from the
// working directory, so that its generated main packages may import package
// assess. Each batch of up to batchSize packages builds one driver
// with `go build`, which calls assess.Drive with one assess.DecoderFor
// Decoder per type the batch's cases decode into, bound to one
// codec.WithBranches per interface of that type's package, so that the census
// supplies every package's branch sets (sealedInterfaces). The command therefore
// runs inside this module's tree, with the go command on PATH and the module
// cache holding this module's requirements, as GOFLAGS=-mod=mod builds offline
// from it.
//
// A case's T is the type the generated file declares, by its header comment
// and its name, for the top-level element declaration with {abstract} false
// that the instance's document element names: ownType states the match. A
// case whose document element is not well-formed, names no such declaration,
// or names one ownType matches no single type for, decodes into a placeholder
// struct instead, and the report counts those cases in a caveat.
//
// The driver classifies each result as package assess states, and validates
// the instance itself, so that "decoded where validate rejects" reads its own
// validation, never Decode's error.
//
// Usage:
//
//	go tool decodecensus [dir]
//
// dir defaults to the suite at [suiteroot.Default]. An absent suite is a
// supported mode, not a failure: the submodule is absent in a fresh container
// (#659), so the tool says so, names the command that initializes it, and
// exits 0. It exits 0 for a completed census whatever it finds, and 2 for an
// operational error.
package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/codegen"
	"github.com/kud360/goxsd8/conformance"
	"github.com/kud360/goxsd8/internal/goname"
	"github.com/kud360/goxsd8/parser/xmltree"
	"github.com/kud360/goxsd8/tools/decodecensus/internal/assess"
	"github.com/kud360/goxsd8/tools/internal/suiteroot"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// usage is printed for any argument the tool cannot act on.
const usage = "usage: decodecensus [dir]"

// driverModule is the temporary module's path. It lies under this module's
// path, so that Go's internal rule lets the drivers import package assess, and
// no directory of this module's tree holds it, so that no import is ambiguous.
const driverModule = "github.com/kud360/goxsd8/tools/decodecensus/driver"

// batchSize bounds the generated packages one driver links: 3000 were measured
// to build in one binary at grounding (#3292).
const batchSize = 2500

func main() {
	if err := run(os.Stdout, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "decodecensus: %v\n", err)
		os.Exit(2)
	}
}

// run drives the census end to end: read the catalog, generate and resolve
// each group, build and run the drivers, render the report. It takes its
// destination and arguments so a test can drive the whole command without a
// subprocess of its own.
func run(stdout io.Writer, args []string) (err error) {
	flags := flag.NewFlagSet("decodecensus", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%s", err, usage)
	}
	if flags.NArg() > 1 {
		return errors.New(usage)
	}
	root := suiteroot.Default
	if flags.NArg() == 1 {
		root = flags.Arg(0)
	}
	index := filepath.Join(root, "suite.xml")
	if _, serr := os.Stat(index); errors.Is(serr, fs.ErrNotExist) {
		return suiteroot.PrintAbsent(stdout, "decodecensus", "no suite index at "+index, "census")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("making the suite root %s absolute: %w", root, err)
	}
	entries, err := conformance.Catalog(abs)
	if err != nil {
		return fmt.Errorf("reading the catalog: %w", err)
	}
	module, err := moduleRoot()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "decodecensus-")
	if err != nil {
		return fmt.Errorf("creating the driver module: %w", err)
	}
	defer func() {
		if rerr := os.RemoveAll(tmp); rerr != nil {
			err = errors.Join(err, fmt.Errorf("removing the driver module %s: %w", tmp, rerr))
		}
	}()
	if err := writeModule(tmp, module); err != nil {
		return err
	}
	rep, batches, err := plan(abs, root, instanceCases(entries), tmp)
	if err != nil {
		return err
	}
	for i, b := range batches {
		sum, err := drive(tmp, i, b)
		if err != nil {
			return err
		}
		rep.merge(sum)
	}
	return printReport(stdout, rep)
}

// instanceCases returns the entries the population holds, in catalog order:
// each instanceTest entry not withheld.
func instanceCases(entries []conformance.CatalogEntry) []conformance.CatalogEntry {
	var cases []conformance.CatalogEntry
	for _, e := range entries {
		if e.Withheld || !isInstanceID(e.ID) {
			continue
		}
		cases = append(cases, e)
	}
	return cases
}

// isInstanceID reports whether id, `<testSet>/<testGroup>/<kind>/<test-name>`,
// names an instanceTest.
func isInstanceID(id string) bool {
	segments := strings.Split(id, "/")
	return len(segments) >= 4 && segments[len(segments)-2] == "instance"
}

// moduleRoot returns the directory of this module's go.mod, as `go env GOMOD`
// names it from the working directory, and its go directive's version.
func moduleRoot() (module, error) {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return module{}, fmt.Errorf("running go env GOMOD: %w", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		return module{}, errors.New("decodecensus runs inside the github.com/kud360/goxsd8 module, and the working directory lies in no module")
	}
	data, err := os.ReadFile(gomod)
	if err != nil {
		return module{}, fmt.Errorf("reading %s: %w", gomod, err)
	}
	m := module{dir: filepath.Dir(gomod)}
	var path string
	for line := range strings.Lines(string(data)) {
		if p, ok := strings.CutPrefix(line, "module "); ok {
			path = strings.TrimSpace(p)
		}
		if v, ok := strings.CutPrefix(line, "go "); ok {
			m.goVersion = strings.TrimSpace(v)
		}
	}
	if path != "github.com/kud360/goxsd8" {
		return module{}, fmt.Errorf("decodecensus runs inside the github.com/kud360/goxsd8 module, and %s declares %q", gomod, path)
	}
	return m, nil
}

// module is this module's root directory and its go directive's version.
type module struct {
	dir, goVersion string
}

// writeModule writes the driver module's go.mod into dir, requiring this
// module by a replace directive on m's directory, and copies m's go.sum
// beside it.
func writeModule(dir string, m module) error {
	gomod := fmt.Sprintf("module %s\n\ngo %s\n\nrequire github.com/kud360/goxsd8 v0.0.0\n\nreplace github.com/kud360/goxsd8 => %s\n",
		driverModule, m.goVersion, m.dir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		return fmt.Errorf("writing the driver module's go.mod: %w", err)
	}
	sum, err := os.ReadFile(filepath.Join(m.dir, "go.sum"))
	if err != nil {
		return fmt.Errorf("reading the module's go.sum: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0o644); err != nil {
		return fmt.Errorf("writing the driver module's go.sum: %w", err)
	}
	return nil
}

// batch is one driver's share of the census: the request it reads, the
// generated packages it imports, and the decoders it compiles, each a
// qualified type, `p<N>.<Name>`, at the index a case's assess.Case.Decoder
// names.
type batch struct {
	req      assess.Request
	packages []genPackage
	decoders []string
}

// genPackage is one generated package a driver imports: its name, p<N>, and
// its generated file's sealed interfaces, in file order.
type genPackage struct {
	name   string
	sealed []sealedDecl
}

// sealedDecl is one interface type a generated file declares, by name, and
// its branches, the names of the types the file gives its marker method, in
// file order.
type sealedDecl struct {
	name     string
	branches []string
}

// plan generates each group's package into dir, resolves each case's type,
// and returns the report's counts so far with the batches that drive the
// rest. abs is the suite root, absolute, and root as the caller named it.
func plan(abs, root string, cases []conformance.CatalogEntry, dir string) (report, []batch, error) {
	rep := report{root: root, considered: len(cases)}
	backend := strict.New()
	var batches []batch
	var cur *batch
	// groupOf maps a schema-document list, joined, to its group's position in
	// groups, for lookup only; nothing ranges over it (STYLE D2).
	groupOf := map[string]int{}
	var groups []groupCases
	for _, c := range cases {
		if len(c.SchemaDocs) == 0 {
			continue
		}
		key := strings.Join(c.SchemaDocs, "\x00")
		i, seen := groupOf[key]
		if !seen {
			i = len(groups)
			groupOf[key] = i
			groups = append(groups, groupCases{schema: c.SchemaDocs})
		}
		groups[i].instances = append(groups[i].instances, c.Docs[0])
	}
	for n, g := range groups {
		// A schema that does not assemble or generate is the census's data,
		// its cases counted as not generating, never an operational error:
		// the suite is full of schemas that are invalid on purpose.
		schema, err := assess.Assemble(abs, g.schema, backend)
		if err != nil {
			continue
		}
		pkg := "p" + strconv.Itoa(n)
		pkgDir := filepath.Join(dir, pkg)
		if errs := codegen.Generate([]codegen.Target{{Schema: schema, Dir: pkgDir, Package: pkg, Name: pkg}}); errs != nil {
			continue
		}
		file, err := parseGenerated(filepath.Join(pkgDir, "xsd_gen.go"))
		if err != nil {
			return report{}, nil, err
		}
		types := generatedTypes(file)
		if cur == nil || len(cur.packages) == batchSize {
			batches = append(batches, batch{req: assess.Request{Root: abs}})
			cur = &batches[len(batches)-1]
		}
		ag := assess.Group{Schema: g.schema}
		used := false
		for _, inst := range g.instances {
			rep.generates++
			name, err := documentElement(filepath.Join(abs, filepath.FromSlash(inst)))
			if err != nil {
				return report{}, nil, err
			}
			typ, decl, ok := resolveCase(schema, types, name, pkgDir)
			if !ok {
				rep.placeholder++
				if decl {
					rep.unresolved++
				}
				ag.Cases = append(ag.Cases, assess.Case{Instance: inst, Decoder: -1})
				continue
			}
			used = true
			ag.Cases = append(ag.Cases, assess.Case{Instance: inst, Decoder: cur.decoder(pkg + "." + typ)})
		}
		cur.req.Groups = append(cur.req.Groups, ag)
		if used {
			cur.packages = append(cur.packages, genPackage{name: pkg, sealed: sealedInterfaces(file)})
		}
	}
	return rep, batches, nil
}

// groupCases is one test group's schema documents and its cases' instance
// documents, in catalog order.
type groupCases struct {
	schema    []string
	instances []string
}

// decoder returns the index of the decoder into typ, a qualified type,
// adding it when b has none.
func (b *batch) decoder(typ string) int {
	if i := slices.Index(b.decoders, typ); i >= 0 {
		return i
	}
	b.decoders = append(b.decoders, typ)
	return len(b.decoders) - 1
}

// resolveCase returns the name of the type ownType finds for the top-level
// element declaration of schema the document element name names, ok false
// when there is none; decl reports, for ok false, that the declaration exists
// with {abstract} false, so that only ownType failed to find a single type.
// A zero name, a document element that did not read, has no declaration.
func resolveCase(schema *xsd.Schema, types []typeDecl, name xsd.QName, dir string) (typ string, decl, ok bool) {
	if name == (xsd.QName{}) {
		return "", false, false
	}
	d, found := schema.Element(name)
	if !found || d.Abstract() {
		return "", false, false
	}
	typ, ok = ownType(types, d, dir)
	return typ, true, ok
}

// documentElement returns the expanded name of the document element of the
// instance at path, or the zero QName when the document is not well-formed
// before it.
func documentElement(path string) (xsd.QName, error) {
	f, err := os.Open(path)
	if err != nil {
		return xsd.QName{}, fmt.Errorf("reading instance %s: %w", path, err)
	}
	defer func() { _ = f.Close() }() // read-only handle: close error cannot change what was read
	r := xmltree.NewReader(path, f)
	for {
		tok, err := r.Token()
		if err != nil {
			return xsd.QName{}, nil
		}
		if start, ok := tok.(*xmltree.StartElement); ok {
			return xsd.QName{Space: start.Name().Space(), Local: start.Name().Local()}, nil
		}
	}
}

// typeDecl is one type declaration of a generated file: its name and the
// first line of its doc comment, the header codegen's package doc "Naming"
// states.
type typeDecl struct {
	name, header string
}

// parseGenerated parses the generated file at path, its comments included.
func parseGenerated(path string) (*ast.File, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing generated file %s: %w", path, err)
	}
	return file, nil
}

// generatedTypes returns the type declarations of the generated file, in
// file order.
func generatedTypes(file *ast.File) []typeDecl {
	var types []typeDecl
	for _, d := range file.Decls {
		gen, ok := d.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE || gen.Doc == nil {
			continue
		}
		for _, spec := range gen.Specs {
			types = append(types, typeDecl{name: spec.(*ast.TypeSpec).Name.Name, header: gen.Doc.List[0].Text})
		}
	}
	return types
}

// sealedInterfaces returns each interface type the generated file declares
// with no type parameters, in file order, with its branches. A generic
// interface, such as codegen's Nillable, is no type a WithBranches can name
// uninstantiated, and Decode declines the nillable element Nillable is
// emitted for before item 6. The branches are each type B the file gives a method of a
// single value receiver of type B, named "is" plus the interface's name, with
// no parameters or results, the marker codegen's package doc "# Complex
// types" states, in file order.
func sealedInterfaces(file *ast.File) []sealedDecl {
	var sealed []sealedDecl
	for _, d := range file.Decls {
		gen, ok := d.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts := spec.(*ast.TypeSpec)
			if _, isInterface := ts.Type.(*ast.InterfaceType); isInterface && ts.TypeParams == nil {
				sealed = append(sealed, sealedDecl{name: ts.Name.Name})
			}
		}
	}
	for _, d := range file.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || fn.Type.Params.NumFields() != 0 || fn.Type.Results.NumFields() != 0 {
			continue
		}
		recv, ok := fn.Recv.List[0].Type.(*ast.Ident)
		if !ok {
			continue
		}
		i := slices.IndexFunc(sealed, func(s sealedDecl) bool { return fn.Name.Name == "is"+s.name })
		if i >= 0 {
			sealed[i].branches = append(sealed[i].branches, recv.Name)
		}
	}
	return sealed
}

// ownProvenances are the texts a header renders the declaration a top-level
// element's own type is generated from as, before the element's name: plain,
// and, for a sealed interface over its governing types, through its {type
// table} or its {type definition} (codegen's package doc, "Element
// declarations").
var ownProvenances = []string{"element ", "the {type table} of element ", "the {type definition} of element "}

// ownType returns the name of the type codegen emitted for the top-level
// element declaration decl into dir, among types, ok false unless exactly one
// type matches. A type matches when its name is decl's local name's
// goname.Identifier, alone or with a numeric suffix from 2, codegen's package
// doc "Naming" rule for a type named from an element, and its header records
// decl, by its expanded name and its Loc rendered as that section states for
// dir, as one of ownProvenances. Every other type a header records decl for is
// named by a path or a branch of names that extends the identifier with
// further words, each of which opens with an upper-case letter, so no other
// name has that shape.
func ownType(types []typeDecl, decl xsd.ElementDeclaration, dir string) (string, bool) {
	base := goname.Identifier(decl.Name().Local)
	loc := headerLoc(dir, decl.Loc())
	var found []string
	for _, t := range types {
		if !namedFrom(t.name, base) {
			continue
		}
		for _, p := range ownProvenances {
			if t.header == "// "+t.name+" is generated from "+p+decl.Name().String()+" declared at "+loc+"." {
				found = append(found, t.name)
			}
		}
	}
	if len(found) != 1 {
		return "", false
	}
	return found[0], true
}

// namedFrom reports whether name is base, or base followed by a numeric
// suffix goname.Namer gives a collision: a decimal from 2, with no leading
// zero.
func namedFrom(name, base string) bool {
	suffix, ok := strings.CutPrefix(name, base)
	if !ok {
		return false
	}
	if suffix == "" {
		return true
	}
	n, err := strconv.Atoi(suffix)
	return err == nil && n >= 2 && strconv.Itoa(n) == suffix
}

// headerLoc renders loc as codegen's package doc "Naming" states a header
// renders it for a target whose Dir is dir.
func headerLoc(dir string, loc xsderr.Loc) string {
	if !filepath.IsAbs(loc.URI) {
		return loc.String()
	}
	base, err := filepath.Abs(dir)
	if err != nil {
		return loc.String()
	}
	rel, err := filepath.Rel(base, loc.URI)
	if err != nil {
		return loc.String()
	}
	loc.URI = filepath.ToSlash(rel)
	return loc.String()
}

// drive writes batch b's driver into the module at dir as package b<n>,
// builds it, runs it on b's request, and returns its summary.
func drive(dir string, n int, b batch) (assess.Summary, error) {
	name := "b" + strconv.Itoa(n)
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		return assess.Summary{}, fmt.Errorf("creating driver %s: %w", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name, "main.go"), driverSource(b), 0o644); err != nil {
		return assess.Summary{}, fmt.Errorf("writing driver %s: %w", name, err)
	}
	bin := filepath.Join(dir, name, name)
	build := exec.Command("go", "build", "-o", bin, "./"+name)
	build.Dir = dir
	build.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		return assess.Summary{}, fmt.Errorf("building driver %s: %w\n%s", name, err, out)
	}
	req, err := json.Marshal(b.req)
	if err != nil {
		return assess.Summary{}, fmt.Errorf("encoding driver %s's request: %w", name, err)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin)
	cmd.Stdin = bytes.NewReader(req)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return assess.Summary{}, fmt.Errorf("running driver %s: %w\n%s", name, err, stderr.Bytes())
	}
	var sum assess.Summary
	if err := json.Unmarshal(stdout.Bytes(), &sum); err != nil {
		return assess.Summary{}, fmt.Errorf("reading driver %s's summary: %w", name, err)
	}
	return sum, nil
}

// driverSource returns the main package of b's driver: an import of each of
// b's packages; for each package declaring a sealed interface, the option
// list p<N>Branches, one codec.WithBranches per interface, each branch's zero
// value as *new(B), so that a branch that is no struct compiles too; and a
// decoder per qualified type, in b's order, bound to its package's list.
func driverSource(b batch) []byte {
	var s strings.Builder
	s.WriteString("// Code generated by decodecensus; DO NOT EDIT.\n\npackage main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n\n")
	if slices.ContainsFunc(b.packages, func(p genPackage) bool { return len(p.sealed) > 0 }) {
		s.WriteString("\t\"github.com/kud360/goxsd8/codec\"\n")
	}
	s.WriteString("\t\"github.com/kud360/goxsd8/tools/decodecensus/internal/assess\"\n")
	for _, p := range b.packages {
		fmt.Fprintf(&s, "\t%s %q\n", p.name, path.Join(driverModule, p.name))
	}
	s.WriteString(")\n")
	for _, p := range b.packages {
		if len(p.sealed) == 0 {
			continue
		}
		fmt.Fprintf(&s, "\nvar %sBranches = []codec.Option{\n", p.name)
		for _, sd := range p.sealed {
			fmt.Fprintf(&s, "\tcodec.WithBranches[%s.%s](", p.name, sd.name)
			for i, br := range sd.branches {
				if i > 0 {
					s.WriteString(", ")
				}
				fmt.Fprintf(&s, "*new(%s.%s)", p.name, br)
			}
			s.WriteString("),\n")
		}
		s.WriteString("}\n")
	}
	s.WriteString("\nvar decoders = []assess.Decoder{\n")
	for _, d := range b.decoders {
		pkg, _, _ := strings.Cut(d, ".")
		bound := ""
		if i := slices.IndexFunc(b.packages, func(p genPackage) bool { return p.name == pkg }); i >= 0 && len(b.packages[i].sealed) > 0 {
			bound = pkg + "Branches..."
		}
		fmt.Fprintf(&s, "\tassess.DecoderFor[%s](%s),\n", d, bound)
	}
	s.WriteString("}\n\nfunc main() {\n\tif err := assess.Drive(os.Stdin, os.Stdout, decoders); err != nil {\n" +
		"\t\tfmt.Fprintln(os.Stderr, err)\n\t\tos.Exit(1)\n\t}\n}\n")
	return []byte(s.String())
}

// report is what the census found. The declined count is not stored: it is
// the sum of the tallies' (STYLE D3).
type report struct {
	root       string
	considered int
	generates  int
	// placeholder counts the cases decoded into the placeholder, and
	// unresolved those of them whose document element names a top-level
	// declaration with {abstract} false.
	placeholder, unresolved int
	sum                     assess.Summary
}

// merge adds one driver's summary to r. A construct's tallies from two
// drivers merge by its String, the key assess.Drive's doc states across
// processes, every driver linking the one codec.
func (r *report) merge(s assess.Summary) {
	r.sum.Decoded += s.Decoded
	r.sum.Undecided += s.Undecided
	r.sum.Rejected += s.Rejected
	r.sum.FailedOther += s.FailedOther
	r.sum.Differential += s.Differential
	r.sum.PlaceholderMismatch += s.PlaceholderMismatch
	for _, t := range s.Declines {
		i := slices.IndexFunc(r.sum.Declines, func(u assess.Tally) bool { return u.Construct == t.Construct })
		if i < 0 {
			r.sum.Declines = append(r.sum.Declines, assess.Tally{Construct: t.Construct})
			i = len(r.sum.Declines) - 1
		}
		r.sum.Declines[i].Count += t.Count
	}
}

// declined is how many cases Decode declined.
func (r report) declined() int {
	n := 0
	for _, t := range r.sum.Declines {
		n += t.Count
	}
	return n
}

// ranked returns the tallies largest first, then by the construct's String.
func ranked(tallies []assess.Tally) []assess.Tally {
	r := slices.Clone(tallies)
	slices.SortStableFunc(r, func(a, b assess.Tally) int {
		if c := cmp.Compare(b.Count, a.Count); c != 0 {
			return c
		}
		return strings.Compare(a.Construct, b.Construct)
	})
	return r
}

// caveats are the limits on reading any figure the census prints; the
// report adds the placeholder's, which carries its counts.
var caveats = []string{
	"Defects: \"rejected codegen's own struct\" and \"decoded where validate rejects\" are defects in codec, and any count other than 0 is one to file.",
	"\"failed other\" is no defect count: it holds every Decode error that is no decline, no undecided validation and no type mismatch on codegen's struct: a source fault, every instance validate charges, as the suite's invalid instances are meant to be, and an error of Decode's second read after validation.",
	"One decline per case: Decode returns its first decline, so a case counts once, under that construct.",
	"Cases, not documents: each instance case counts once, however many cases name its instance document or share its schema.",
	"Assembly: the instance lane's schema is wider. It also serves the suite's own XLink schema, declines an assembly outside the parser's decidable subset, widens the group's schema by the hints on the instance's document element for namespaces it leaves uncovered, and builds a schema from the instance's hints or the built-ins for a group declaring other than one schemaTest, whose cases the census counts as not generating.",
	"The decline table keys a construct on its codec.Construct inside each driver, and across drivers on the Construct's String, which codec makes distinct for each Construct.",
	"No count is a yield: lifting a construct exposes each of its cases' next decline, so its count bounds from above the cases lifting it alone makes decode.",
}

// printReport renders the census: the counts, the declined constructs largest
// first, and the caveats. A bufio.Writer keeps its first write error and Flush
// returns it, so checking Flush checks every write.
func printReport(dst io.Writer, rep report) error {
	w := bufio.NewWriter(dst)
	_, _ = fmt.Fprintf(w, "decodecensus: %d instance case(s) under %s\n", rep.considered, rep.root)
	_, _ = fmt.Fprintf(w, "  schema generates: %d\n", rep.generates)
	_, _ = fmt.Fprintf(w, "  decoded: %d\n", rep.sum.Decoded)
	_, _ = fmt.Fprintf(w, "  declined: %d\n", rep.declined())
	_, _ = fmt.Fprintf(w, "  undecided: %d\n", rep.sum.Undecided)
	_, _ = fmt.Fprintf(w, "  rejected codegen's own struct: %d\n", rep.sum.Rejected)
	_, _ = fmt.Fprintf(w, "  failed other: %d\n", rep.sum.FailedOther)
	_, _ = fmt.Fprintf(w, "  decoded where validate rejects: %d\n", rep.sum.Differential)
	order := ranked(rep.sum.Declines)
	_, _ = fmt.Fprintf(w, "\n=== First decline, by construct: %d construct(s) ===\n", len(order))
	for _, t := range order {
		_, _ = fmt.Fprintf(w, "  %6d  %s\n", t.Count, t.Construct)
	}
	_, _ = fmt.Fprintln(w, "\n=== Caveats ===")
	for _, c := range caveats {
		_, _ = fmt.Fprintf(w, "  - %s\n", c)
	}
	_, _ = fmt.Fprintf(w, "  - %s\n", placeholderCaveat(rep))
	if err := w.Flush(); err != nil {
		return fmt.Errorf("writing the report: %w", err)
	}
	return nil
}

// placeholderCaveat is the caveat on the cases decoded into the placeholder.
func placeholderCaveat(rep report) string {
	return fmt.Sprintf("Placeholder: %d case(s) decode into a placeholder struct, not codegen's type: their document element is not well-formed, "+
		"names no top-level declaration with {abstract} false, or, for %d of them, names one the census found no single generated type for. "+
		"Decode's ErrTypeMismatch on the placeholder, %d case(s), counts under \"failed other\", never under \"rejected codegen's own struct\".",
		rep.placeholder, rep.unresolved, rep.sum.PlaceholderMismatch)
}
