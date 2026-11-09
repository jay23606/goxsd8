package parser

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// rosterNames returns the roster keys in sorted order, so every table-driven test
// below reports in a fixed order rather than the map's (STYLE D2).
func rosterNames() []string {
	names := make([]string, 0, len(s4sAttrRosters))
	for local := range s4sAttrRosters {
		names = append(names, local)
	}
	sort.Strings(names)
	return names
}

// TestS4SAttrRostersCarryID pins the extension chain the whole roster hangs on:
// every s4s production is built on xs:annotated (xmlschema11-1.md:4426), which
// contributes id, EXCEPT <appinfo> and <documentation>, whose types are bare
// mixed complex types on no named base at all (:5720, :5733). A roster that lost
// id would reject id on the element it lost it from; one that gained id on
// <appinfo> would admit a name that production declares nowhere.
func TestS4SAttrRostersCarryID(t *testing.T) {
	for _, local := range rosterNames() {
		t.Run(local, func(t *testing.T) {
			got := slices.Contains(s4sAttrRosters[local].declares, "id")
			want := local != "appinfo" && local != "documentation"
			if got != want {
				t.Errorf("<%s> roster contains id = %v, want %v (%s, %s)",
					local, got, want, s4sAttrRosters[local].grammar, s4sAttrRosters[local].spec)
			}
		})
	}
}

// TestS4SAttrRostersAreWellFormed pins that no roster repeats a name and none is
// empty: a repeat would print twice in the diagnostic's declares list, and an
// empty roster would reject every unprefixed attribute on that element.
func TestS4SAttrRostersAreWellFormed(t *testing.T) {
	for _, local := range rosterNames() {
		r := s4sAttrRosters[local]
		if len(r.declares) == 0 {
			t.Errorf("<%s> declares nothing, which would reject every unprefixed attribute on it", local)
		}
		seen := map[string]bool{}
		for _, a := range r.declares {
			if seen[a] {
				t.Errorf("<%s> declares %s twice", local, a)
			}
			seen[a] = true
		}
		if r.grammar == "" || r.spec == "" {
			t.Errorf("<%s> names production %q at line %q, want both", local, r.grammar, r.spec)
		}
	}
}

// attrReaders are the four call shapes that read a NO-NAMESPACE schema
// attribute by a literal name, and the argument index the name is at. attr is
// Element's own primitive and Attr its value-only wrapper (tree.go); attrOr and
// boolAttr are the two helpers that funnel into them (produce_complex.go).
// versioningAttr is deliberately absent — it reads the vc: namespace
// (conditional.go), which xs:openAttrs' ##other wildcard admits and no roster
// governs.
var attrReaders = map[string]int{"Attr": 0, "attr": 0, "attrOr": 1, "boolAttr": 1}

// TestS4SAttrRostersCoverEveryAttributeThisPackageReads is the guard against the
// one defect rejectUndeclaredAttrs can introduce: a roster missing a name
// Appendix A really declares turns a valid schema document into a rejected one.
// Every attribute name this package reads off a schema element is a name the
// grammar declares somewhere, so it must appear in some roster — and a name read
// by no roster's element fails here rather than in the corpus.
//
// It scans the package's own source, so a new attribute read added without a
// roster entry fails on the commit that adds it. The scan is deliberately narrow:
// a reader shape it does not know can only weaken this test, never make it pass
// wrongly.
func TestS4SAttrRostersCoverEveryAttributeThisPackageReads(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var files []*ast.File
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := goparser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("ParseFile %s: %v", name, err)
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		t.Fatal("no non-test source files found, so this test asserts nothing")
	}
	var union []string
	for _, r := range s4sAttrRosters {
		union = append(union, r.declares...)
	}
	// A slice of findings rather than an immediate report, so the failures come out
	// sorted rather than in file-walk order (STYLE D2).
	var missing []string
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			at, ok := attrReaderIndex(call)
			if !ok || at >= len(call.Args) {
				return true
			}
			lit, ok := call.Args[at].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			name, err := strconv.Unquote(lit.Value)
			if err != nil || slices.Contains(union, name) {
				return true
			}
			missing = append(missing, name+" ("+fset.Position(lit.Pos()).String()+")")
			return true
		})
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("this package reads the attribute %s, which no s4sAttrRosters entry declares", m)
	}
}

// attrReaderIndex reports which argument of call holds the attribute name, for
// the four shapes attrReaders names, and whether call is one of them at all.
func attrReaderIndex(call *ast.CallExpr) (int, bool) {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		at, ok := attrReaders[fn.Sel.Name]
		return at, ok
	case *ast.Ident:
		at, ok := attrReaders[fn.Name]
		return at, ok
	}
	return 0, false
}
