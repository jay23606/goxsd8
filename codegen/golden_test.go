package codegen

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/kud360/goxsd8/loader"
	xsdparser "github.com/kud360/goxsd8/parser"
)

var update = flag.Bool("update", false, "rewrite codegen/testdata/*.golden from Generate's output")

// TestGenerateGolden pins Generate's output for each fixture. The fixtures are
// parsed by a relative location, so the Loc in every header is portable.
func TestGenerateGolden(t *testing.T) {
	for _, name := range []string{"flat", "nested", "collision", "named", "expanded", "symbolspaces", "choice", "attributes", "integers", "identity", "empty", "lists", "namedlocal", "floats", "decimals", "binary", "temporal", "refs", "anytype", "restriction", "wildcard", "occurs", "simplecontent", "spliced", "extension", "groupref", "unions", "anyattribute", "nestedgroups", "special", "valueconstraint", "choicewildcard", "all", "substitution", "qname", "assertions", "extendshared", "extendsharedfirst", "redefine", "emptiablechoice", "emptiablesequence", "choiceattributes", "typetable", "typetableerror", "typetablemember", "mixed", "nillable", "abstract", "opencontent", "defaultopencontent", "precisiondecimal", "emptychoice", "abstracttype", "flatedge", "flatsequence", "flatchoice", "competition", "shortening", "claimorder"} {
		t.Run(name, func(t *testing.T) {
			schema, err := xsdparser.Parse("testdata/" + name + ".xsd")
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			first := generateOne(t, Target{Schema: schema, Dir: filepath.Join(moduleTempDir(t), "a"), Package: name})
			second := generateOne(t, Target{Schema: schema, Dir: filepath.Join(moduleTempDir(t), "b"), Package: name})
			if !bytes.Equal(first, second) {
				t.Fatalf("two runs differ:\n%s\n---\n%s", first, second)
			}
			typeCheck(t, first)
			golden := filepath.Join("testdata", name+".golden")
			if *update {
				if err := os.WriteFile(golden, first, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first, want) {
				t.Errorf("output differs from %s (rerun with -update to accept):\n%s", golden, first)
			}
		})
	}
}

// TestGenerateAssertionsInert pins the package doc's "# Complex types"
// sentences on {assertions}: the assertions fixture, each <assert> blanked to
// spaces of equal length so that every header's Loc is unchanged, generates
// the bytes of assertions.golden, which TestGenerateGolden pins for the
// fixture as written.
func TestGenerateAssertionsInert(t *testing.T) {
	const location = "testdata/assertions.xsd"
	src, err := os.ReadFile(location)
	if err != nil {
		t.Fatal(err)
	}
	blanked := regexp.MustCompile(`<xs:assert [^>]*/>`).ReplaceAllFunc(src, func(m []byte) []byte {
		return bytes.Repeat([]byte(" "), len(m))
	})
	if bytes.Equal(blanked, src) || bytes.Contains(blanked, []byte("assert")) {
		t.Fatalf("blanking left %s with its <assert> elements, or had none to blank", location)
	}
	resolve := loader.ResolverFunc(func(_, loc string) (io.ReadCloser, string, error) {
		if loc != location {
			return nil, "", fmt.Errorf("no document %q", loc)
		}
		return io.NopCloser(bytes.NewReader(blanked)), loc, nil
	})
	schema, err := xsdparser.Parse(location, xsdparser.WithResolver(resolve))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := generateOne(t, Target{Schema: schema, Dir: filepath.Join(moduleTempDir(t), "assertions"), Package: "assertions"})
	want, err := os.ReadFile(filepath.Join("testdata", "assertions.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output without the <assert> elements differs from assertions.golden:\n%s", got)
	}
}

// TestGenerateExampleTypeChecks type-checks the file ExampleGenerate prints,
// generated from the same schema into the same package. The example's Output
// block is the one committed copy of those bytes, so no golden row pins them.
func TestGenerateExampleTypeChecks(t *testing.T) {
	schema, err := xsdparser.Parse("testdata/example.xsd")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	typeCheck(t, generateOne(t, Target{Schema: schema, Dir: filepath.Join(moduleTempDir(t), "order"), Package: "order"}))
}

// generateOne runs Generate on the one target and returns the file it wrote.
func generateOne(t *testing.T, target Target) []byte {
	t.Helper()
	if err := Generate([]Target{target}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	src, err := os.ReadFile(filepath.Join(target.Dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	return src
}

// moduleTempDir returns a new temporary directory holding a go.mod declaring
// module example.com/m, so that a Target's Dir in it or under it has an import
// path, which Generate requires (Target.Dir).
func moduleTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module example.com/m\n")
	return dir
}

// typeCheck fails unless src is a package that compiles, resolving its imports
// through the default importer; the emitted package imports math/big at most.
func typeCheck(t *testing.T, src []byte) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, fileName, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing the emitted source: %v", err)
	}
	if _, err := (&types.Config{Importer: importer.Default()}).Check(file.Name.Name, fset, []*ast.File{file}, nil); err != nil {
		t.Fatalf("type-checking the emitted source: %v", err)
	}
}

// TestGenerateImportsBig pins the math/big import for each site that sets it:
// a schema whose one element has the row's type, so that the row's Go type
// alone decides the import, emits a package that type-checks only when the
// import is emitted exactly when a declaration names math/big, since a missing
// and an unused import are both type errors. The xs:gMonthDay, xs:gDay and
// xs:gMonth rows, whose datatype structs "# Simple types" gives no math/big
// field, pin the import's absence; every other row pins its presence.
func TestGenerateImportsBig(t *testing.T) {
	for _, typ := range []string{"xs:integer", "xs:decimal",
		"xs:dateTime", "xs:dateTimeStamp", "xs:time", "xs:date", "xs:gYearMonth", "xs:gYear",
		"xs:gMonthDay", "xs:gDay", "xs:gMonth", "xs:duration", "xs:dayTimeDuration", "xs:yearMonthDuration", "xs:precisionDecimal"} {
		t.Run(typ, func(t *testing.T) {
			schema := parseString(t, `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"><xs:element name="e" type="`+typ+`"/></xs:schema>`)
			typeCheck(t, generateOne(t, Target{Schema: schema, Dir: filepath.Join(moduleTempDir(t), "out"), Package: "p"}))
		})
	}
}
