package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"

	"github.com/kud360/goxsd8/codegen"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
)

// flatFixture is codegen's own fixture of the shapes its first slice accepts.
// The CLI owns no golden: the file it writes names the schema by its path
// relative to a temporary -out, which climbs to this checkout differently on
// every machine, so the comparand is what codegen.Generate writes for the same
// schema.
const flatFixture = "../../codegen/testdata/flat.xsd"

// absFixture returns path resolved absolute, the location gen hands the parser.
func absFixture(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// generatedFile reads the xsd_gen.go Generate writes into dir.
func generatedFile(t *testing.T, dir string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, "xsd_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// moduleTempDir returns a new temporary directory holding a go.mod declaring
// module example.com/m, so that an -out in it or under it has the import path
// gen requires.
func moduleTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// assertAbsent fails unless nothing exists at path.
func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s exists (lstat: %v), want nothing written there", path, err)
	}
}

// TestGenWritesWhatGenerateWrites pins exit 0: each pair's -out receives the
// file codegen.Generate writes for a Target built from parser.Parse of the
// absolute location gen hands the parser, in the package the -out's last path
// element after filepath.Clean names: a trailing slash and a trailing ..
// included. -q changes nothing. The three pairs share flat.xsd, so the second
// and third files alias the first's types; the CLI and library trees are each
// the root of a module of one path, so the import paths agree. Before #2964
// this run exited 2, reporting gen as not yet implemented.
func TestGenWritesWhatGenerateWrites(t *testing.T) {
	flat := absFixture(t, flatFixture)
	tmp := t.TempDir()
	for _, root := range []string{"cli", "lib"} {
		if err := os.Mkdir(filepath.Join(tmp, root), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, root, "go.mod"), []byte("module example.com/w\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(tmp, "cli", "flat")
	order := filepath.Join(tmp, "cli", "order") + string(filepath.Separator)
	up := filepath.Join(tmp, "cli", "up", "sub") + string(filepath.Separator) + ".."
	var stdout, stderr bytes.Buffer
	code := run([]string{"gen", "-q", "-schema", flatFixture, "-out", out, "-schema", flatFixture, "-out", order, "-schema", flatFixture, "-out", up}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("gen = %d, want %d (stderr %q)", code, exitOK, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("gen stdout = %q, stderr = %q, want both empty", stdout.String(), stderr.String())
	}

	schema, err := parser.Parse(flat, parser.WithResolver(loader.Dir(filesystemRoot(flat))))
	if err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(tmp, "lib")
	if err := codegen.Generate([]codegen.Target{
		{Schema: schema, Dir: filepath.Join(lib, "flat"), Package: "flat"},
		{Schema: schema, Dir: filepath.Join(lib, "order"), Package: "order"},
		{Schema: schema, Dir: filepath.Join(lib, "up"), Package: "up"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"flat", "order", "up"} {
		got := generatedFile(t, filepath.Join(tmp, "cli", dir))
		want := generatedFile(t, filepath.Join(lib, dir))
		if !bytes.Equal(got, want) {
			t.Errorf("gen wrote for -out %s:\n%s\nwant what Generate writes:\n%s", dir, got, want)
		}
		if !bytes.Contains(got, []byte("\npackage "+dir+"\n")) {
			t.Errorf("gen wrote for -out %s no package %s clause:\n%s", dir, dir, got)
		}
		if alias := []byte("\ntype Note = flat.Note\n"); bytes.Contains(got, alias) != (dir != "flat") {
			t.Errorf("gen wrote for -out %s, want the alias %q exactly when the pair is not the first:\n%s", dir, alias, got)
		}
		// The usage block's claim that the comments name each document
		// relative to -out.
		rel, err := filepath.Rel(filepath.Join(tmp, "cli", dir), flat)
		if err != nil {
			t.Fatal(err)
		}
		header := "\n// Note is generated from element {urn:flat}note declared at " + filepath.ToSlash(rel) + ":5:3.\n"
		if !bytes.Contains(got, []byte(header)) {
			t.Errorf("gen wrote for -out %s no line %q:\n%s", dir, header, got)
		}
	}
}

// TestGenIsCheckoutIndependent pins that the generated file names every
// document of the closure relative to -out: one tree laid out alike under two
// absolute roots generates byte-identical files, whatever the working
// directory and however -schema and -out are spelled, and the header of each
// document's element is exactly its relative path.
func TestGenIsCheckoutIndependent(t *testing.T) {
	const flat = "<xs:schema xmlns:xs=\"http://www.w3.org/2001/XMLSchema\">\n" +
		"  <xs:element name=\"a\" type=\"xs:string\"/>\n" +
		"  <xs:include schemaLocation=\"inc.xsd\"/>\n" +
		"</xs:schema>\n"
	const inc = "<xs:schema xmlns:xs=\"http://www.w3.org/2001/XMLSchema\">\n" +
		"  <xs:element name=\"b\" type=\"xs:boolean\"/>\n" +
		"</xs:schema>\n"
	tmp := moduleTempDir(t)
	roots := []string{filepath.Join(tmp, "one"), filepath.Join(tmp, "two", "deeper")}
	for _, root := range roots {
		if err := os.MkdirAll(filepath.Join(root, "s"), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, doc := range []struct{ name, body string }{{"flat.xsd", flat}, {"inc.xsd", inc}} {
			if err := os.WriteFile(filepath.Join(root, "s", doc.name), []byte(doc.body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	want := "// Code generated by github.com/kud360/goxsd8/codegen. DO NOT EDIT.\n\n" +
		"package x\n\n" +
		"// A is generated from element a declared at ../../s/flat.xsd:2:3.\n" +
		"type A string\n\n" +
		"// B is generated from element b declared at ../../s/inc.xsd:2:3.\n" +
		"type B bool\n"
	for _, tc := range []struct {
		name, root, cwd, schema, out string
	}{
		{"first root", roots[0], roots[0], "s/flat.xsd", "gen/x"},
		{"second root", roots[1], roots[1], "s/flat.xsd", "gen/x"},
		{"from the schema directory", roots[0], filepath.Join(roots[0], "s"), "flat.xsd", "../gen/x"},
		{"absolute spellings", roots[1], tmp, filepath.Join(roots[1], "s", "flat.xsd"), filepath.Join(roots[1], "gen", "x") + "/"},
		{"unclean spellings", roots[0], roots[0], "./s/../s/flat.xsd", "gen/./x/../x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(tc.cwd)
			var stdout, stderr bytes.Buffer
			if code := run([]string{"gen", "-schema", tc.schema, "-out", tc.out}, &stdout, &stderr); code != exitOK {
				t.Fatalf("gen = %d, want %d (stderr %q)", code, exitOK, stderr.String())
			}
			if got := string(generatedFile(t, filepath.Join(tc.root, "gen", "x"))); got != want {
				t.Errorf("gen wrote:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// typeDecl matches a top-level type declaration line of a generated file.
var typeDecl = regexp.MustCompile(`(?m)^type `)

// TestGenDeclaresTheComposedSet pins the usage block's sentence on what a
// pair's package declares, each run exiting 0 with empty stdout and stderr: a
// document two pairs' schemas share, whose generated types agree, has its
// element's type declared by the first pair's package, which composes it by
// import, and aliased by the second's, which composes it as its own root,
// importing the first; and a schema declaring no top-level element, only a
// named simple type, gives a file with a package clause and no type
// declaration.
func TestGenDeclaresTheComposedSet(t *testing.T) {
	tmp := moduleTempDir(t)
	simple := filepath.Join(tmp, "types.xsd")
	const body = "<xs:schema xmlns:xs=\"http://www.w3.org/2001/XMLSchema\">\n" +
		"  <xs:simpleType name=\"code\"><xs:restriction base=\"xs:string\"/></xs:simpleType>\n" +
		"</xs:schema>\n"
	if err := os.WriteFile(simple, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	importer, imported := filepath.Join(tmp, "gen", "a"), filepath.Join(tmp, "gen", "b")
	types := filepath.Join(tmp, "gen", "types")
	for _, args := range [][]string{
		{"gen", "-schema", "testdata/imports.xsd", "-out", importer, "-schema", "testdata/imported.xsd", "-out", imported},
		{"gen", "-schema", simple, "-out", types},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != exitOK {
			t.Fatalf("%v = %d, want %d (stderr %q)", args, code, exitOK, stderr.String())
		}
		if stdout.Len() != 0 || stderr.Len() != 0 {
			t.Errorf("%v stdout = %q, stderr = %q, want both empty", args, stdout.String(), stderr.String())
		}
	}
	if got := generatedFile(t, importer); !bytes.Contains(got, []byte("\ntype Carried string\n")) {
		t.Errorf("gen wrote for -out %s no declaration of imported.xsd's element carried:\n%s", importer, got)
	}
	got := generatedFile(t, imported)
	for _, want := range []string{"\nimport a \"example.com/m/gen/a\"\n", "\ntype Carried = a.Carried\n"} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("gen wrote for -out %s no line %q:\n%s", imported, want, got)
		}
	}
	if bytes.Contains(got, []byte("\ntype Carried string\n")) {
		t.Errorf("gen wrote for -out %s a definition of carried, want only its alias:\n%s", imported, got)
	}
	got = generatedFile(t, types)
	if !bytes.Contains(got, []byte("\npackage types\n")) || typeDecl.Match(got) {
		t.Errorf("gen wrote for a schema with no top-level element:\n%s\nwant a package types clause and no type declaration", got)
	}
}

// TestGenRejectedSchemaWritesNothing pins exit 1: a rejected -schema prints its
// first error as parse does, and no pair is generated, the compiling pair
// before it included.
func TestGenRejectedSchemaWritesNothing(t *testing.T) {
	broken := absFixture(t, "testdata/broken.xsd")
	tmp := moduleTempDir(t)
	first, second := filepath.Join(tmp, "first"), filepath.Join(tmp, "second")
	var stdout, stderr bytes.Buffer
	code := run([]string{"gen", "-schema", flatFixture, "-out", first, "-schema", "testdata/broken.xsd", "-out", second}, &stdout, &stderr)
	if code != exitInvalid {
		t.Fatalf("gen = %d, want %d (stderr %q)", code, exitInvalid, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("gen stdout = %q, want empty", stdout.String())
	}
	line := stderr.String()
	if !strings.HasPrefix(line, broken+":") || !strings.Contains(line, ": [src-resolve] ") || strings.Count(line, "\n") != 1 {
		t.Errorf("gen stderr = %q, want one line %s:<line>:<col>: [src-resolve] <message>", line, broken)
	}
	assertAbsent(t, first)
	assertAbsent(t, second)
}

// TestGenReportsEveryPair pins that every pair is compiled and reported before
// the run answers: an unreadable -schema does not hide the next pair's
// rejection, and the run answers with the higher-ranked of the two, which is
// the first.
func TestGenReportsEveryPair(t *testing.T) {
	tmp := moduleTempDir(t)
	var stdout, stderr bytes.Buffer
	code := run([]string{"gen", "-schema", "testdata/nosuch.xsd", "-out", filepath.Join(tmp, "a"), "-schema", "testdata/broken.xsd", "-out", filepath.Join(tmp, "b")}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("gen = %d, want %d (stderr %q)", code, exitUsage, stderr.String())
	}
	for _, want := range []string{"[src-resolve]", "goxsd8: gen: open testdata/nosuch.xsd: no such file or directory"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("gen stderr = %q, want it to contain %q", stderr.String(), want)
		}
	}
}

// TestGenNamesAnUnresolvedDirective pins that gen names a schemaLocation
// resolving to no document as parse does, and still generates.
func TestGenNamesAnUnresolvedDirective(t *testing.T) {
	abs, err := filepath.Abs(shortSchema)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(moduleTempDir(t), "short")
	var stdout, stderr bytes.Buffer
	code := run([]string{"gen", "-schema", shortSchema, "-out", out}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("gen = %d, want %d (stderr %q)", code, exitOK, stderr.String())
	}
	want := "goxsd8: gen: " + abs + ":10:3: this schemaLocation resolved to no document"
	if !strings.HasPrefix(stderr.String(), want) {
		t.Errorf("gen stderr = %q, want it to open %q", stderr.String(), want)
	}
	generatedFile(t, out)
}

// TestGenStrictChargesASkip pins -strict on gen (#1845): a -schema whose
// directive is named resolving to no document fails to compile with exit 4 and
// nothing is written for any pair, the clean one beside it included; a -schema
// with no such line generates.
func TestGenStrictChargesASkip(t *testing.T) {
	mod := moduleTempDir(t)
	short, flat := filepath.Join(mod, "short"), filepath.Join(mod, "flat")
	var stdout, stderr bytes.Buffer
	code := run([]string{"gen", "-strict", "-schema", flatFixture, "-out", flat, "-schema", shortSchema, "-out", short}, &stdout, &stderr)
	if code != exitUndecided {
		t.Fatalf("gen -strict = %d, want %d (stderr %q)", code, exitUndecided, stderr.String())
	}
	want := "goxsd8: gen: " + absFixture(t, shortSchema) + ":10:3: this schemaLocation resolved to no document"
	if !strings.HasPrefix(stderr.String(), want) {
		t.Errorf("gen -strict stderr = %q, want it to open %q", stderr.String(), want)
	}
	assertAbsent(t, filepath.Join(short, "xsd_gen.go"))
	assertAbsent(t, filepath.Join(flat, "xsd_gen.go"))

	stderr.Reset()
	if code := run([]string{"gen", "-strict", "-schema", flatFixture, "-out", flat}, &stdout, &stderr); code != exitOK {
		t.Fatalf("gen -strict, no skip = %d, want %d (stderr %q)", code, exitOK, stderr.String())
	}
	generatedFile(t, flat)
}

// TestGenVerboseLogs pins -v: the parser's assembly is logged at debug level
// on stderr, and the run still generates.
func TestGenVerboseLogs(t *testing.T) {
	out := filepath.Join(moduleTempDir(t), "flat")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"gen", "-v", "-schema", flatFixture, "-out", out}, &stdout, &stderr); code != exitOK {
		t.Fatalf("gen -v = %d, want %d (stderr %q)", code, exitOK, stderr.String())
	}
	if !strings.Contains(stderr.String(), "level=DEBUG") {
		t.Errorf("gen -v stderr = %q, want debug log lines", stderr.String())
	}
	generatedFile(t, out)
}

// packageClause is the remedy closing every refusal of the package name gen
// derives from an -out, spelled out rather than read from packageRemedy so a
// change to the text fails the rows that carry it.
const packageClause = ": gen names the package after the last path element of -out, so name an -out directory such as ./order"

// TestGenUsageAndIOFaults pins exit 2: each row's stderr is exactly its want,
// and nothing is written at the paths it names. A diagnosis of the command
// line's shape ends in genShapeTail; an unreadable -schema, a refused -out and
// a write fault are their one line alone (#3333). Every row runs in a fresh
// working directory, so the -out . row cannot write into the package.
func TestGenUsageAndIOFaults(t *testing.T) {
	flat := absFixture(t, flatFixture)
	type row struct {
		name   string
		args   []string
		want   string
		absent []string
	}
	rows := func(tmp string) []row {
		at := func(p string) string { return filepath.Join(tmp, p) }
		return []row{
			{"no pair", []string{}, "goxsd8: gen: no -schema/-out pair given" + genShapeTail, nil},
			{"-schema without -out", []string{"-schema", flat},
				"goxsd8: gen: -schema " + flat + " stands without its -out" + genShapeTail, nil},
			{"-schema followed by -schema", []string{"-schema", flat, "-schema", flat, "-out", at("d")},
				"goxsd8: gen: -schema " + flat + " stands without its -out" + genShapeTail, []string{at("d")}},
			{"-out without -schema", []string{"-out", at("d")},
				"goxsd8: gen: -out " + at("d") + " stands without a -schema before it" + genShapeTail, []string{at("d")}},
			{"second -out", []string{"-schema", flat, "-out", at("d"), "-out", at("e")},
				"goxsd8: gen: -out " + at("e") + " stands without a -schema before it" + genShapeTail, []string{at("d"), at("e")}},
			{"unreadable -schema", []string{"-schema", "nosuch.xsd", "-out", at("d")},
				"goxsd8: gen: open nosuch.xsd: no such file or directory", []string{at("d")}},
			{"-backend", []string{"-backend", "strict", "-schema", flat, "-out", at("d")},
				"goxsd8: gen: flag provided but not defined: -backend" + genShapeTail, []string{at("d")}},
			{"positional argument", []string{"-schema", flat, "-out", at("d"), "extra.xsd"},
				"goxsd8: gen: extra.xsd: gen takes no positional argument, only -schema/-out pairs" + genShapeTail, []string{at("d")}},
			{"flag after a positional", []string{"extra.xsd", "-out", at("d")},
				fmt.Sprintf(flagAfterPositionalFmt, "gen", "-out") + genShapeTail, []string{at("d")}},
			// A -- the flag package consumed ends gen's options, so the
			// flag-shaped token after it is a positional; a -- that is
			// -out's value ends nothing, and the token after the
			// positional x is misplaced (#3332).
			{"flag-shaped operand after --", []string{"-schema", flat, "-out", at("d"), "--", "-weird.xml"},
				"goxsd8: gen: -weird.xml: gen takes no positional argument, only -schema/-out pairs" + genShapeTail, []string{at("d")}},
			{"-- as -out's value", []string{"-schema", flat, "-out", "--", "x", "-weird.xml"},
				fmt.Sprintf(flagAfterPositionalFmt, "gen", "-weird.xml") + genShapeTail, []string{at("--")}},
			{"valued help", []string{"-help=true"}, fmt.Sprintf(helpNotAFlagValueFmt, "gen") + genShapeTail, nil},
			{"package not an identifier", []string{"-schema", flat, "-out", at("my-pkg")},
				"goxsd8: gen: -schema " + flat + " -out " + at("my-pkg") + `: the package name "my-pkg" is not a Go identifier` + packageClause,
				[]string{at("my-pkg")}},
			{"package a keyword", []string{"-schema", flat, "-out", at("type")},
				"goxsd8: gen: -schema " + flat + " -out " + at("type") + `: the package name "type" is a Go keyword` + packageClause,
				[]string{at("type")}},
			{"package blank", []string{"-schema", flat, "-out", at("_")},
				"goxsd8: gen: -schema " + flat + " -out " + at("_") + `: the package name "_" is the blank identifier` + packageClause,
				[]string{at("_")}},
			{"empty -out", []string{"-schema", flat, "-out", ""},
				"goxsd8: gen: -schema " + flat + " -out : no output directory given",
				[]string{at("xsd_gen.go")}},
			{"package main", []string{"-schema", flat, "-out", at("main")},
				"goxsd8: gen: -schema " + flat + " -out " + at("main") + `: the package name is "main", but the emitted package declares no func main and would not build` + packageClause,
				[]string{at("main")}},
			{"-out .", []string{"-schema", flat, "-out", "."},
				"goxsd8: gen: -schema " + flat + ` -out .: the package name "." is not a Go identifier` + packageClause,
				[]string{at("xsd_gen.go")}},
			// The second pair's -out is refused, so the first is not
			// written either.
			{"duplicate -out", []string{"-schema", flat, "-out", at("d"), "-schema", flat, "-out", at("d") + "/"},
				"goxsd8: gen: -schema " + flat + " -out " + at("d") + "/: the output directory " + at("d") + "/ is also the output directory of -schema " + flat + " -out " + at("d"),
				[]string{at("d")}},
			{"-out a regular file", []string{"-schema", flat, "-out", at("ok"), "-schema", flat, "-out", at("file")},
				"goxsd8: gen: -schema " + flat + " -out " + at("file") + ": the output directory " + at("file") + " exists and is not a directory",
				[]string{at("ok")}},
			{"existing file not generated", []string{"-schema", flat, "-out", at("ok"), "-schema", flat, "-out", at("keep")},
				"goxsd8: gen: -schema " + flat + " -out " + at("keep") + ": " + filepath.Join(at("keep"), "xsd_gen.go") + " exists and its first line does not mark it generated, so it is never overwritten",
				[]string{at("ok")}},
			// Phase 2: the dangling symlink passes every check and fails
			// creating the directory, after the first pair was written.
			{"write fault", []string{"-schema", flat, "-out", at("written"), "-schema", flat, "-out", at("dangling")},
				"goxsd8: gen: -schema " + flat + " -out " + at("dangling") + ": creating " + at("dangling") + ": mkdir " + at("dangling") + ": file exists",
				nil},
		}
	}
	probe := rows("")
	for i := range probe {
		t.Run(probe[i].name, func(t *testing.T) {
			tmp := moduleTempDir(t)
			t.Chdir(tmp)
			if err := os.WriteFile(filepath.Join(tmp, "file"), []byte("not a directory\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(tmp, "keep"), 0o755); err != nil {
				t.Fatal(err)
			}
			kept := []byte("package keep\n")
			if err := os.WriteFile(filepath.Join(tmp, "keep", "xsd_gen.go"), kept, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(tmp, "nowhere"), filepath.Join(tmp, "dangling")); err != nil {
				t.Fatal(err)
			}
			r := rows(tmp)[i]
			var stdout, stderr bytes.Buffer
			code := run(append([]string{"gen"}, r.args...), &stdout, &stderr)
			if code != exitUsage {
				t.Errorf("gen %q = %d, want %d", r.args, code, exitUsage)
			}
			if stdout.Len() != 0 {
				t.Errorf("gen %q stdout = %q, want empty", r.args, stdout.String())
			}
			if want := r.want + "\n"; stderr.String() != want {
				t.Errorf("gen %q stderr = %q, want %q", r.args, stderr.String(), want)
			}
			for _, p := range r.absent {
				assertAbsent(t, p)
			}
			if got, err := os.ReadFile(filepath.Join(tmp, "keep", "xsd_gen.go")); err != nil || !bytes.Equal(got, kept) {
				t.Errorf("keep/xsd_gen.go = %q (%v), want it untouched", got, err)
			}
			if r.name == "write fault" {
				generatedFile(t, filepath.Join(tmp, "written"))
			}
		})
	}
}

// TestGenModuleRefusals pins exit 2 for the three refusals of an -out's Go
// module: an -out with no go.mod at or above it, one whose nearest go.mod
// declares no module, and an -out whose import path repeats an earlier
// pair's, from a second module of the same path. Each prints one stderr line
// naming the pair, with no synopsis or helpPointer under it (#3333), and
// nothing is written.
func TestGenModuleRefusals(t *testing.T) {
	flat := absFixture(t, flatFixture)
	tmp := t.TempDir()
	for _, sub := range []string{"free", "nomodule", "one", "two"} {
		if err := os.Mkdir(filepath.Join(tmp, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []struct{ path, content string }{
		{"nomodule/go.mod", "go 1.22\n"},
		{"one/go.mod", "module example.com/m\n"},
		{"two/go.mod", "module example.com/m\n"},
	} {
		if err := os.WriteFile(filepath.Join(tmp, f.path), []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	at := func(p string) string { return filepath.Join(tmp, p) }
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no go.mod", []string{"-schema", flat, "-out", at("free/gen")},
			"goxsd8: gen: -schema " + flat + " -out " + at("free/gen") + ": no go.mod lies in the output directory " + at("free/gen") + " or any directory above it, so it has no import path: create a go.mod at or above the output directory, as go mod init <module path> does"},
		{"no module directive", []string{"-schema", flat, "-out", at("nomodule/gen")},
			"goxsd8: gen: -schema " + flat + " -out " + at("nomodule/gen") + ": " + at("nomodule/go.mod") + " holds no module directive"},
		{"repeated import path", []string{"-schema", flat, "-out", at("one/gen"), "-schema", flat, "-out", at("two/gen")},
			"goxsd8: gen: -schema " + flat + " -out " + at("two/gen") + ": the output directory " + at("two/gen") +
				" has the import path example.com/m/gen, which is also the import path of -schema " + flat + " -out " + at("one/gen")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(append([]string{"gen"}, tt.args...), &stdout, &stderr); code != exitUsage {
				t.Errorf("gen %q = %d, want %d", tt.args, code, exitUsage)
			}
			if want := tt.want + "\n"; stderr.String() != want {
				t.Errorf("gen %q stderr = %q, want %q", tt.args, stderr.String(), want)
			}
			for _, out := range []string{"free/gen", "nomodule/gen", "one/gen", "two/gen"} {
				assertAbsent(t, at(out))
			}
		})
	}
}

// genRow is one -schema/-out pair of a TestGenDeclines row: the schema's file
// name, the -out directory's name, and the schema's body.
type genRow struct{ file, out, body string }

// TestGenDeclines pins exit 4: each construct outside codegen's slice prints
// one line on stderr naming the pair, the construct and its absolute location,
// with no helpPointer under it, in argument order and then document order, and
// nothing is written. The valueCycle body is codegen/decline_test.go's value
// cycle direct row, a named complex type reaching itself through a local
// element occurring exactly once, and recursion's an element reaching itself
// through an <element ref> particle occurring exactly once.
func TestGenDeclines(t *testing.T) {
	recursion := func(name string) string {
		return `<xs:element name="` + name + `"><xs:complexType><xs:sequence><xs:element ref="` + name + `"/></xs:sequence></xs:complexType></xs:element>`
	}
	const (
		valueCycle = `<xs:complexType name="c"><xs:sequence><xs:element name="x" type="c"/></xs:sequence></xs:complexType><xs:element name="e" type="c"/>`
		cycle      = "local element x reaching complex type c again through elements that each occur exactly once at "
		tail       = " is not supported by codegen, a generator refusal, not a schema fault\n"
	)
	reaching := func(name string) string {
		return "<element ref> particle to element " + name + " reaching element " + name + " again through elements that each occur exactly once at "
	}
	rows := []struct {
		name string
		// pairs holds each pair's schema file name, -out name and schema
		// body, one declaration per line from line 2.
		pairs []genRow
		// want is stderr for the pairs' schema paths and -out paths.
		want func(schemas, outs []string) string
	}{
		{"one decline", []genRow{{"s.xsd", "e", valueCycle}}, func(s, o []string) string {
			return "goxsd8: gen: -schema " + s[0] + " -out " + o[0] + ": " + cycle + s[0] + ":2:39" + tail
		}},
		{"two pairs, three declines", []genRow{{"s.xsd", "e", valueCycle + "\n" + recursion("f")}, {"t.xsd", "g", recursion("g")}}, func(s, o []string) string {
			return "goxsd8: gen: -schema " + s[0] + " -out " + o[0] + ": " + cycle + s[0] + ":2:39" + tail +
				"goxsd8: gen: -schema " + s[0] + " -out " + o[0] + ": " + reaching("f") + s[0] + ":3:1" + tail +
				"goxsd8: gen: -schema " + s[1] + " -out " + o[1] + ": " + reaching("g") + s[1] + ":2:1" + tail
		}},
	}
	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			tmp := moduleTempDir(t)
			var args, schemas, outs []string
			for _, p := range tt.pairs {
				schema := filepath.Join(tmp, p.file)
				doc := "<xs:schema xmlns:xs=\"http://www.w3.org/2001/XMLSchema\">\n" + p.body + "\n</xs:schema>\n"
				if err := os.WriteFile(schema, []byte(doc), 0o644); err != nil {
					t.Fatal(err)
				}
				out := filepath.Join(tmp, p.out)
				schemas, outs = append(schemas, schema), append(outs, out)
				args = append(args, "-schema", schema, "-out", out)
			}
			var stdout, stderr bytes.Buffer
			code := run(append([]string{"gen"}, args...), &stdout, &stderr)
			if code != exitUndecided {
				t.Errorf("gen = %d, want %d", code, exitUndecided)
			}
			if stdout.Len() != 0 {
				t.Errorf("gen stdout = %q, want empty", stdout.String())
			}
			if want := tt.want(schemas, outs); stderr.String() != want {
				t.Errorf("gen stderr = %q, want %q", stderr.String(), want)
			}
			for _, out := range outs {
				assertAbsent(t, out)
			}
		})
	}
}

// TestReportGenerateOSFaultNotDecline pins exit 2 with the line alone, no
// synopsis or helpPointer under it (#3333), for an element of Generate's
// error that answers errors.Is(_, errors.ErrUnsupported) but holds no
// *codegen.DeclineError: a phase-2 OS fault whose errno is ENOTSUP, wrapped
// with %w as Generate wraps it (#3090).
func TestReportGenerateOSFaultNotDecline(t *testing.T) {
	fault := fmt.Errorf("target 0: writing x/xsd_gen.go: %w", syscall.ENOTSUP)
	if !errors.Is(fault, errors.ErrUnsupported) {
		t.Fatalf("errors.Is(%v, errors.ErrUnsupported) = false on this platform; the row tests nothing", fault)
	}
	var stderr bytes.Buffer
	if code := reportGenerate(errors.Join(fault), &stderr); code != exitUsage {
		t.Errorf("reportGenerate = %d, want %d", code, exitUsage)
	}
	if want := "goxsd8: gen: " + fault.Error() + "\n"; stderr.String() != want {
		t.Errorf("reportGenerate stderr = %q, want %q", stderr.String(), want)
	}
}

// TestGenRefusalsOutrankEveryPair pins exit 2 for every refused -out, whatever
// any pair compiles or declines, its own pair's decline included: each refusal
// prints its one line, with no synopsis or helpPointer under it, in argument
// order, before any line a -schema's compilation prints, and nothing is
// written. Every row runs in a fresh working directory holding dec.xsd, a
// schema the generator declines, and its -out paths are relative to it.
func TestGenRefusalsOutrankEveryPair(t *testing.T) {
	flat := absFixture(t, flatFixture)
	broken := absFixture(t, "testdata/broken.xsd")
	refusal := func(schema, out, msg string) string {
		return "goxsd8: gen: -schema " + schema + " -out " + out + ": " + msg + "\n"
	}
	notIdentifier := `the package name "my-pkg" is not a Go identifier` + packageClause
	rows := []struct {
		name string
		args []string
		// refusals is stderr's opening; rejected, when set, is the location of
		// the one rejection line that follows it, and nothing follows when not.
		refusals string
		rejected string
		absent   []string
	}{
		{"behind a rejected schema", []string{"-schema", broken, "-out", "my-pkg"},
			refusal(broken, "my-pkg", notIdentifier), broken, []string{"my-pkg"}},
		{"after a declining pair", []string{"-schema", "dec.xsd", "-out", "gen/x1", "-schema", flat, "-out", "my-pkg"},
			refusal(flat, "my-pkg", notIdentifier), "", []string{"gen", "my-pkg"}},
		{"before a declining pair", []string{"-schema", flat, "-out", "my-pkg", "-schema", "dec.xsd", "-out", "gen/q"},
			refusal(flat, "my-pkg", notIdentifier), "", []string{"gen", "my-pkg"}},
		{"hand-written xsd_gen.go behind its own decline", []string{"-schema", "dec.xsd", "-out", "hw/q"},
			refusal("dec.xsd", "hw/q", "hw/q/xsd_gen.go exists and its first line does not mark it generated, so it is never overwritten"), "", nil},
		{"duplicate -out behind a decline", []string{"-schema", "dec.xsd", "-out", "d", "-schema", flat, "-out", "d/"},
			refusal(flat, "d/", "the output directory d/ is also the output directory of -schema dec.xsd -out d"), "", []string{"d"}},
		{"-out a regular file behind a decline", []string{"-schema", "dec.xsd", "-out", "ok", "-schema", flat, "-out", "file"},
			refusal(flat, "file", "the output directory file exists and is not a directory"), "", []string{"ok"}},
		{"every refusal in argument order", []string{"-schema", "dec.xsd", "-out", "my-pkg", "-schema", flat, "-out", "ok", "-schema", broken, "-out", "type"},
			refusal("dec.xsd", "my-pkg", notIdentifier) + refusal(broken, "type", `the package name "type" is a Go keyword`+packageClause), broken, []string{"my-pkg", "ok", "type"}},
	}
	dec := "<xs:schema xmlns:xs=\"http://www.w3.org/2001/XMLSchema\">\n" +
		`<xs:complexType name="c"><xs:sequence><xs:element name="x" type="c"/></xs:sequence></xs:complexType><xs:element name="e" type="c"/>` +
		"\n</xs:schema>\n"
	kept := []byte("package q\n")
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			tmp := moduleTempDir(t)
			t.Chdir(tmp)
			if err := os.WriteFile("dec.xsd", []byte(dec), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("file", []byte("not a directory\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join("hw", "q"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join("hw", "q", "xsd_gen.go"), kept, 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			code := run(append([]string{"gen"}, r.args...), &stdout, &stderr)
			if code != exitUsage {
				t.Errorf("gen %q = %d, want %d", r.args, code, exitUsage)
			}
			if stdout.Len() != 0 {
				t.Errorf("gen %q stdout = %q, want empty", r.args, stdout.String())
			}
			rest, ok := strings.CutPrefix(stderr.String(), r.refusals)
			if !ok {
				t.Fatalf("gen %q stderr = %q, want it to open %q", r.args, stderr.String(), r.refusals)
			}
			if r.rejected == "" && rest != "" {
				t.Errorf("gen %q stderr after the refusals = %q, want nothing", r.args, rest)
			}
			if r.rejected != "" && (!strings.HasPrefix(rest, r.rejected+":") || !strings.Contains(rest, ": [src-resolve] ") || strings.Count(rest, "\n") != 1) {
				t.Errorf("gen %q stderr after the refusals = %q, want one line %s:<line>:<col>: [src-resolve] <message>", r.args, rest, r.rejected)
			}
			for _, p := range r.absent {
				assertAbsent(t, filepath.Join(tmp, p))
			}
			if got, err := os.ReadFile(filepath.Join("hw", "q", "xsd_gen.go")); err != nil || !bytes.Equal(got, kept) {
				t.Errorf("hw/q/xsd_gen.go = %q (%v), want it untouched", got, err)
			}
		})
	}
}
