package goxsd8

import (
	"errors"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/codec"
	"github.com/kud360/goxsd8/codec/testdata/readme/order"
	"github.com/kud360/goxsd8/codegen"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
)

// TestReadmeFixtures holds each file of testdata/readme that README.md's CLI
// section shows to the text README shows for it (TestReadmeSharedDocument
// holds the files of the two-target gen example): order.xsd and order.json
// byte for byte as a fence's body, and the one-line twice.json, its newline
// dropped, as an inline code span. README's examples run against the
// committed files, so a fence edited without its file, or a file without its
// fence, would show the reader one document and run another.
//
// It holds README's code-generation snippet to readmeCodegen's body, which
// compiles with this file, and the xsd_gen.go README shows to the file that
// running readmeCodegen writes for order.xsd, both byte for byte as a fence's
// body. It holds README's decode snippet to readmeDecode's body the same way,
// and runs it on order.xml.
func TestReadmeFixtures(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		file string
		want func(body string) string
	}{
		{"order.xsd", func(body string) string { return "```xml\n" + body + "```\n" }},
		{"order.json", func(body string) string { return "```json\n" + body + "```\n" }},
		{"twice.json", func(body string) string { return "`" + strings.TrimSuffix(body, "\n") + "`" }},
	} {
		body, err := os.ReadFile(filepath.Join("testdata", "readme", c.file))
		if err != nil {
			t.Errorf("%s: %v", c.file, err)
			continue
		}
		if want := c.want(string(body)); !strings.Contains(string(readme), want) {
			t.Errorf("README.md does not show testdata/readme/%s as\n%s", c.file, want)
		}
	}

	snippet, err := funcBody("readme_test.go", "readmeCodegen")
	if err != nil {
		t.Fatal(err)
	}
	if want := "```go\n" + snippet + "```\n"; !strings.Contains(string(readme), want) {
		t.Errorf("README.md does not show readmeCodegen's body as\n%s", want)
	}
	snippet, err = funcBody("readme_test.go", "readmeDecode")
	if err != nil {
		t.Fatal(err)
	}
	if want := "```go\n" + snippet + "```\n"; !strings.Contains(string(readme), want) {
		t.Errorf("README.md does not show readmeDecode's body as\n%s", want)
	}

	// The program parses order.xsd with testdata/readme as its working
	// directory, so its Loc reads order.xsd:3:3.
	schema, err := parser.Parse("order.xsd", parser.WithResolver(loader.Dir(filepath.Join("testdata", "readme"))))
	if err != nil {
		t.Fatal(err)
	}
	// The program runs in its module's root, one of the directories README
	// states it may run in.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	readmeCodegen(schema)
	gen, err := os.ReadFile(filepath.Join("gen", "order", "xsd_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "```go\n" + string(gen) + "```\n"; !strings.Contains(string(readme), want) {
		t.Errorf("README.md does not show the xsd_gen.go Generate writes for testdata/readme/order.xsd as\n%s", want)
	}

	// The program reads order.xml from its working directory; package order
	// is the golden copy of the file readmeCodegen writes, which
	// codec.TestDecodeReadmeOrder holds to codegen's output.
	t.Chdir(filepath.Join(wd, "testdata", "readme"))
	readmeDecode(schema, strict.New())
}

// TestReadmeSharedDocument holds README's two-target gen example: note.xsd,
// ship.xsd and bill.xsd byte for byte as fences' bodies, and the
// gen/bill/xsd_gen.go README shows to the file Generate writes for the run
// README shows, byte for byte as a fence's body. It takes gen's route: each
// document is parsed by its absolute path under a resolver rooted at the
// filesystem root, and Generate runs in testdata/readme beside the checkout's
// go.mod, each in a copy under a temp directory, so the headers and the import
// path read as gen writes them in the checkout.
func TestReadmeSharedDocument(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), gomod, 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "testdata", "readme")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	docs := map[string]string{}
	for _, file := range []string{"note.xsd", "ship.xsd", "bill.xsd"} {
		body, err := os.ReadFile(filepath.Join("testdata", "readme", file))
		if err != nil {
			t.Fatal(err)
		}
		docs[file] = string(body)
		if err := os.WriteFile(filepath.Join(dir, file), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"note.xsd", "ship.xsd", "bill.xsd"} {
		if want := "```xml\n" + docs[file] + "```\n"; !strings.Contains(string(readme), want) {
			t.Errorf("README.md does not show testdata/readme/%s as\n%s", file, want)
		}
	}

	var targets []codegen.Target
	for _, file := range []string{"ship.xsd", "bill.xsd"} {
		abs := filepath.Join(dir, file)
		schema, err := parser.Parse(abs, parser.WithResolver(loader.Dir(filepath.VolumeName(abs)+string(filepath.Separator))))
		if err != nil {
			t.Fatal(err)
		}
		pkg := strings.TrimSuffix(file, ".xsd")
		targets = append(targets, codegen.Target{Schema: schema, Dir: "./gen/" + pkg, Package: pkg})
	}
	t.Chdir(dir)
	if err := codegen.Generate(targets); err != nil {
		t.Fatal(err)
	}
	ship, err := os.ReadFile(filepath.Join("gen", "ship", "xsd_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ship), "\ntype Note string\n") {
		t.Errorf("gen/ship/xsd_gen.go does not declare type Note string, as README states:\n%s", ship)
	}
	bill, err := os.ReadFile(filepath.Join("gen", "bill", "xsd_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "```go\n" + string(bill) + "```\n"; !strings.Contains(string(readme), want) {
		t.Errorf("README.md does not show the gen/bill/xsd_gen.go Generate writes for testdata/readme's ship.xsd and bill.xsd as\n%s", want)
	}
}

// readmeCodegen is README's code-generation snippet: README shows its body
// byte for byte, each line less one leading tab. schema stands for the
// program's variable of that name.
func readmeCodegen(schema *xsd.Schema) {
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
}

// readmeDecode is README's decode snippet: README shows its body byte for
// byte, each line less one leading tab. schema and backend stand for the
// program's variables of those names, and package order for the package
// readmeCodegen writes.
func readmeDecode(schema *xsd.Schema, backend value.Backend) {
	data, err := os.ReadFile("order.xml")
	if err != nil {
		log.Fatal(err)
	}
	// order.Order is the generated type for order.xml's document element.
	o, err := codec.Decode[order.Order](schema, backend, data, codec.WithURI("order.xml"))
	if err != nil {
		var ue *codec.UndecidedError
		var de *codec.DeclineError
		switch vs := codec.ViolationsOf(err); {
		case vs != nil:
			// Item 3: every violation of a rejected instance.
			for _, v := range vs {
				fmt.Println(v.Loc, v.Rule)
			}
		case errors.As(err, &ue):
			// Item 4: the check the validator left undecided.
			fmt.Println(ue.Unevaluated().Loc(), ue.Unevaluated().Rule())
		case errors.As(err, &de):
			// Items 5 and 7: the construct codec declined.
			fmt.Println(de.Loc(), de.Construct())
		case errors.Is(err, codec.ErrTypeMismatch):
			// Item 6: T does not match the declaration.
			fmt.Println(err)
		default:
			// A source fault, a backend fault, or a nil schema's plain error.
			// For a source fault, xsderr.RuleOf(err) returns its rule.
			fmt.Println(err)
		}
		log.Fatal(err)
	}
	fmt.Println(o.Item, *o.Kind) // [a b] x
}

// funcBody returns the statements of the function name declares in file, the
// lines between its braces with one leading tab dropped from each.
func funcBody(file, name string) (string, error) {
	src, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, file, src, goparser.ParseComments)
	if err != nil {
		return "", err
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != name {
			continue
		}
		body := string(src[fset.Position(fn.Body.Lbrace).Offset+2 : fset.Position(fn.Body.Rbrace).Offset])
		return strings.ReplaceAll("\n"+body, "\n\t", "\n")[1:], nil
	}
	return "", fmt.Errorf("%s declares no func %s", file, name)
}
