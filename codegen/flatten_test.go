package codegen

import (
	"path/filepath"
	"regexp"
	"testing"
)

// TestGenerateFlattenGates pins the three gates the package doc's
// "# Particles" states for a flattened field at their boundaries, on element
// e's anonymous complex type: each row's source, which type-checks, matches
// every pattern in want and none in absent. A flattened particle's interface
// is named E plus its compositor and holds one branch per basic particle its
// group reaches, named from that particle, never from a model group; the rows
// that keep an occurrence type show a branch or struct only that shape gives.
func TestGenerateFlattenGates(t *testing.T) {
	const str = `type="xs:string"`
	// slice is the pattern of e's flattened field of compositor word, and
	// ifc that of its interface.
	slice := func(word string) string { return `\t` + word + `\s+\[\]E` + word + `\n` }
	ifc := func(word string) string { return `type E` + word + ` interface \{` }
	// branch is the pattern of the branch E<word><name> holding the one
	// field <name> of Go type typ.
	branch := func(word, name, typ string) string {
		return `type E` + word + name + ` struct \{\n\t` + name + ` ` + regexp.QuoteMeta(typ) + `\n\}`
	}
	tests := []struct {
		name, body   string
		want, absent []string
	}{
		// An emptiable model group alternative flattens its choice, inline or
		// through a <group ref>, whatever the alternative's own {min occurs}
		// and the choice's occurrence, the content particle included.
		{"emptiable group in choice", `<xs:element name="e"><xs:complexType><xs:choice><xs:element name="a" ` + str + `/><xs:sequence><xs:element name="b" minOccurs="0" ` + str + `/></xs:sequence></xs:choice></xs:complexType></xs:element>`,
			[]string{slice("Choice"), ifc("Choice"), branch("Choice", "A", "string"), branch("Choice", "B", "string")}, []string{`EChoiceSequence`}},
		{"emptiable choice in choice", `<xs:element name="e"><xs:complexType><xs:choice><xs:element name="a" ` + str + `/><xs:choice><xs:element name="b" ` + str + `/><xs:element name="c" minOccurs="0" ` + str + `/></xs:choice></xs:choice></xs:complexType></xs:element>`,
			[]string{slice("Choice"), branch("Choice", "A", "string"), branch("Choice", "B", "string"), branch("Choice", "C", "string")}, []string{`EChoiceChoice`}},
		{"emptiable group ref in choice", `<xs:group name="g"><xs:sequence><xs:element name="b" minOccurs="0" ` + str + `/></xs:sequence></xs:group><xs:element name="e"><xs:complexType><xs:choice><xs:element name="a" ` + str + `/><xs:group ref="g"/></xs:choice></xs:complexType></xs:element>`,
			[]string{slice("Choice"), branch("Choice", "A", "string"), branch("Choice", "B", "string")}, nil},
		{"optional emptiable group in choice", `<xs:element name="e"><xs:complexType><xs:choice><xs:element name="a" ` + str + `/><xs:sequence minOccurs="0"><xs:element name="b" minOccurs="0" ` + str + `/></xs:sequence></xs:choice></xs:complexType></xs:element>`,
			[]string{slice("Choice"), branch("Choice", "B", "string")}, nil},
		{"emptiable group alternative in repeated choice", `<xs:element name="e"><xs:complexType><xs:choice maxOccurs="unbounded"><xs:element name="a" ` + str + `/><xs:sequence><xs:element name="b" minOccurs="0" ` + str + `/></xs:sequence></xs:choice></xs:complexType></xs:element>`,
			[]string{slice("Choice"), branch("Choice", "A", "string"), branch("Choice", "B", "string")}, nil},
		{"emptiable group alternative group ref content", `<xs:group name="g"><xs:choice><xs:element name="a" ` + str + `/><xs:sequence><xs:element name="b" minOccurs="0" ` + str + `/></xs:sequence></xs:choice></xs:group><xs:element name="e"><xs:complexType><xs:group ref="g" maxOccurs="1"/></xs:complexType></xs:element>`,
			[]string{slice("Choice"), branch("Choice", "A", "string"), branch("Choice", "B", "string")}, nil},
		// A sequence repeating past 1 whose one occurrence is emptiable is
		// flattened, as the content particle or nested, inline or through a
		// <group ref>, whatever its {min occurs}; with no {particles} its
		// interface has no branch.
		{"repeated emptiable sequence", `<xs:element name="e"><xs:complexType><xs:sequence minOccurs="0" maxOccurs="unbounded"><xs:element name="a" minOccurs="0" ` + str + `/></xs:sequence></xs:complexType></xs:element>`,
			[]string{slice("Sequence"), ifc("Sequence"), branch("Sequence", "A", "string")}, nil},
		{"nested repeated emptiable sequence", `<xs:element name="e"><xs:complexType><xs:sequence><xs:element name="h" ` + str + `/><xs:sequence maxOccurs="2"><xs:element name="a" minOccurs="0" ` + str + `/></xs:sequence></xs:sequence></xs:complexType></xs:element>`,
			[]string{`\tH\s+string\n`, slice("Sequence"), branch("Sequence", "A", "string")}, nil},
		{"repeated empty sequence", `<xs:element name="e"><xs:complexType><xs:sequence><xs:element name="h" ` + str + `/><xs:sequence maxOccurs="2"/></xs:sequence></xs:complexType></xs:element>`,
			[]string{slice("Sequence"), ifc("Sequence")}, []string{`isESequence\(\) \{\}`}},
		{"repeated emptiable sequence group ref", `<xs:group name="g"><xs:sequence><xs:element name="b" minOccurs="0" ` + str + `/></xs:sequence></xs:group><xs:element name="e"><xs:complexType><xs:group ref="g" maxOccurs="unbounded"/></xs:complexType></xs:element>`,
			[]string{slice("Sequence"), branch("Sequence", "B", "string")}, nil},
		{"repeated emptiable sequence over a repeating particle", `<xs:element name="e"><xs:complexType><xs:sequence maxOccurs="unbounded"><xs:element name="b" minOccurs="0" maxOccurs="unbounded" ` + str + `/></xs:sequence></xs:complexType></xs:element>`,
			[]string{slice("Sequence"), branch("Sequence", "B", "string")}, nil},
		// A model group repeating past 1 with a repeating edge is flattened:
		// under a choice and a sequence, through a sequence alternative and a
		// <group ref>, at an element, a wildcard, a sequence and a choice
		// edge.
		{"repeating edge in repeated choice", `<xs:element name="e"><xs:complexType><xs:choice maxOccurs="unbounded"><xs:element name="a" ` + str + `/><xs:element name="b" maxOccurs="unbounded" ` + str + `/></xs:choice></xs:complexType></xs:element>`,
			[]string{slice("Choice"), branch("Choice", "A", "string"), branch("Choice", "B", "string")}, nil},
		{"repeating edge in repeated sequence", `<xs:element name="e"><xs:complexType><xs:sequence maxOccurs="unbounded"><xs:element name="b" maxOccurs="unbounded" ` + str + `/></xs:sequence></xs:complexType></xs:element>`,
			[]string{slice("Sequence"), branch("Sequence", "B", "string")}, nil},
		{"repeating edge in a fixed-count sequence", `<xs:element name="e"><xs:complexType><xs:sequence minOccurs="2" maxOccurs="2"><xs:element name="b" maxOccurs="unbounded" ` + str + `/></xs:sequence></xs:complexType></xs:element>`,
			[]string{slice("Sequence"), branch("Sequence", "B", "string")}, nil},
		{"repeating edge beside an emptiable particle", `<xs:element name="e"><xs:complexType><xs:sequence maxOccurs="unbounded"><xs:element name="b" maxOccurs="unbounded" ` + str + `/><xs:element name="c" minOccurs="0" ` + str + `/></xs:sequence></xs:complexType></xs:element>`,
			[]string{slice("Sequence"), branch("Sequence", "B", "string"), branch("Sequence", "C", "string")}, nil},
		{"repeating edge in a sequence alternative", `<xs:element name="e"><xs:complexType><xs:choice maxOccurs="unbounded"><xs:element name="x" ` + str + `/><xs:sequence><xs:element name="a" maxOccurs="2" ` + str + `/></xs:sequence></xs:choice></xs:complexType></xs:element>`,
			[]string{slice("Choice"), branch("Choice", "X", "string"), branch("Choice", "A", "string")}, []string{`EChoiceSequence`}},
		{"repeating edge through group ref", `<xs:group name="g"><xs:sequence><xs:element name="b" maxOccurs="unbounded" ` + str + `/></xs:sequence></xs:group><xs:element name="e"><xs:complexType><xs:sequence><xs:element name="h" ` + str + `/><xs:sequence maxOccurs="unbounded"><xs:group ref="g"/></xs:sequence></xs:sequence></xs:complexType></xs:element>`,
			[]string{slice("Sequence"), branch("Sequence", "B", "string")}, nil},
		{"repeating sequence edge", `<xs:element name="e"><xs:complexType><xs:sequence maxOccurs="unbounded"><xs:sequence maxOccurs="unbounded"><xs:element name="x" ` + str + `/><xs:element name="y" ` + str + `/></xs:sequence></xs:sequence></xs:complexType></xs:element>`,
			[]string{slice("Sequence"), branch("Sequence", "X", "string"), branch("Sequence", "Y", "string")}, []string{`ESequenceSequence`}},
		{"repeating choice edge", `<xs:element name="e"><xs:complexType><xs:choice maxOccurs="unbounded"><xs:element name="a" ` + str + `/><xs:choice maxOccurs="2"><xs:element name="c" ` + str + `/><xs:element name="d" ` + str + `/></xs:choice></xs:choice></xs:complexType></xs:element>`,
			[]string{slice("Choice"), branch("Choice", "A", "string"), branch("Choice", "C", "string"), branch("Choice", "D", "string")}, []string{`EChoiceChoice`}},
		{"repeating wildcard edge", `<xs:element name="e"><xs:complexType><xs:choice maxOccurs="unbounded"><xs:element name="b" ` + str + `/><xs:any namespace="##other" maxOccurs="2"/></xs:choice></xs:complexType></xs:element>`,
			[]string{slice("Choice"), branch("Choice", "B", "string"), branch("Choice", "Any", "AnyElement")}, nil},
		{"repeating optional alternative", `<xs:element name="e"><xs:complexType><xs:choice maxOccurs="unbounded"><xs:element name="a" ` + str + `/><xs:element name="b" minOccurs="0" maxOccurs="unbounded" ` + str + `/></xs:choice></xs:complexType></xs:element>`,
			[]string{slice("Choice"), branch("Choice", "B", "string")}, nil},
		// A model group meeting no gate keeps its occurrence type.
		{"repeating particle not at both edges", `<xs:element name="e"><xs:complexType><xs:sequence maxOccurs="unbounded"><xs:element name="b" maxOccurs="unbounded" ` + str + `/><xs:element name="c" ` + str + `/></xs:sequence></xs:complexType></xs:element>`,
			[]string{`\tSequence\s+\[\]ESequence\n`, `type ESequence struct \{\n\tB\s+\[\]string\n`}, []string{ifc("Sequence")}},
		{"fixed-count edge particle", `<xs:element name="e"><xs:complexType><xs:sequence maxOccurs="unbounded"><xs:element name="b" minOccurs="2" maxOccurs="2" ` + str + `/></xs:sequence></xs:complexType></xs:element>`,
			[]string{`type ESequence struct \{\n\tB\s+\[\]string\n`}, []string{ifc("Sequence")}},
		{"repeated choice of a sequence with no repeating edge", `<xs:element name="e"><xs:complexType><xs:choice maxOccurs="unbounded"><xs:element name="x" ` + str + `/><xs:sequence><xs:element name="a" ` + str + `/><xs:element name="c" ` + str + `/></xs:sequence></xs:choice></xs:complexType></xs:element>`,
			[]string{`\tChoice\s+\[\]EChoice\n`, `type EChoiceSequence struct \{\n\tA\s+string\n\tC\s+string\n\}`}, []string{`type EChoiceA struct`}},
		{"optional choice with a repeating alternative", `<xs:element name="e"><xs:complexType><xs:choice minOccurs="0"><xs:element name="a" ` + str + `/><xs:element name="b" maxOccurs="unbounded" ` + str + `/></xs:choice></xs:complexType></xs:element>`,
			[]string{`\tChoice\s+EChoice\n`, branch("Choice", "B", "[]string")}, []string{slice("Choice")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := parseString(t, "<xs:schema xmlns:xs=\"http://www.w3.org/2001/XMLSchema\">\n"+tt.body+"\n</xs:schema>\n")
			src := generateOne(t, Target{Schema: schema, Dir: filepath.Join(moduleTempDir(t), "out"), Package: "p"})
			typeCheck(t, src)
			for _, w := range tt.want {
				if !regexp.MustCompile(w).Match(src) {
					t.Errorf("source does not match %q:\n%s", w, src)
				}
			}
			for _, a := range tt.absent {
				if regexp.MustCompile(a).Match(src) {
					t.Errorf("source matches %q:\n%s", a, src)
				}
			}
		})
	}
}
