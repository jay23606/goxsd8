package validate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin/strict"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// testBackend is the value.Backend every test here passes to New. It is the
// real strict backend and not a stub: the charges under test are decided by a
// value space, and a stub mapping would pin this package's plumbing while
// leaving the spec question (which lexicals are datatype-valid for xs:integer)
// unasked. Importing it in a TEST does not put it in this package's import
// closure, which imports_test.go pins.
func testBackend() value.Backend { return strict.New() }

// recorder is a slog.Handler that appends one line per record, so a test can
// compare the walk's visits against an expected sequence. It qualifies each
// key with the open groups rather than dropping them, so an expectation here
// keeps describing the log the engine actually emits (STYLE L1).
type recorder struct {
	visits *[]string
	prefix string
}

func (h recorder) Enabled(context.Context, slog.Level) bool { return true }

func (h recorder) Handle(_ context.Context, r slog.Record) error {
	parts := []string{r.Message}
	r.Attrs(func(a slog.Attr) bool {
		parts = append(parts, h.prefix+a.Key+"="+a.Value.String())
		return true
	})
	*h.visits = append(*h.visits, strings.Join(parts, " "))
	return nil
}

// WithAttrs panics rather than recording or dropping: the walk calls no
// logger method that reaches it, so the day one does, the expectations here
// must be rewritten to say what those attributes are.
func (h recorder) WithAttrs([]slog.Attr) slog.Handler {
	panic("validate_test: recorder.WithAttrs: the walk does not log with pre-bound attributes")
}

func (h recorder) WithGroup(name string) slog.Handler {
	h.prefix += name + "."
	return h
}

func recordingLogger() (*slog.Logger, *[]string) {
	visits := new([]string)
	return slog.New(recorder{visits: visits}), visits
}

func emptySchema(t *testing.T) *xsd.Schema {
	t.Helper()
	schema, err := xsd.NewSchemaBuilder().Finalize()
	if err != nil {
		t.Fatalf("finalizing an empty schema: %v", err)
	}
	return schema
}

// topLevelElement is a global element declaration of the given name, with no
// {type definition}: the root dispatch reads {abstract} and nothing else, so
// the type slot is left in the absent state a programmatically built
// declaration starts in.
func topLevelElement(t *testing.T, local string, abstract bool) xsd.ElementDeclaration {
	t.Helper()
	e, err := xsd.NewElementDeclaration(xsderr.Loc{}, xsd.QName{Local: local}, nil, nil,
		xsd.NewGlobalScope(), nil, false, nil, nil, nil, abstract, nil)
	if err != nil {
		t.Fatalf("building the %s element declaration: %v", local, err)
	}
	return e
}

// rootSchema declares the two top-level elements the walk tests drive:
// "root", which sampleTree's root resolves to, and "abstractRoot", which is
// the same declaration with {abstract} = true. It also carries one top-level
// type named "T", in no namespace, so an xsi:type has something to ·resolve· to
// and a prefixed "p:T" has not.
func rootSchema(t *testing.T) *xsd.Schema {
	t.Helper()
	b := xsd.NewSchemaBuilder()
	ct, err := xsd.NewComplexType(xsderr.Loc{}, xsd.QName{Local: "T"}, xsd.QName{}, nil,
		xsd.DerivationRestriction, false, nil, nil, nil, xsd.EmptyContent{}, nil, nil)
	if err != nil {
		t.Fatalf("building the T complex type: %v", err)
	}
	b.AddType(ct)
	b.AddElement(topLevelElement(t, "root", false))
	b.AddElement(topLevelElement(t, "abstractRoot", true))
	schema, err := b.Finalize()
	if err != nil {
		t.Fatalf("finalizing the root schema: %v", err)
	}
	return schema
}

func loc(line, col int) xsderr.Loc {
	return xsderr.Loc{URI: "instance.xml", Line: line, Col: col}
}

// sampleTree is the literal instance every walk test drives, standing for
//
//	<root xmlns:p="urn:p" id="r1" p:lang="en">top
//	  <a>in a</a>between
//	  <b><c/></b>
//	</root>
//
// with one distinct Loc per information item, so a visit is identifiable.
func sampleTree() *testElement {
	c := &testElement{name: xsd.QName{Local: "c"}, loc: loc(5, 6)}
	b := &testElement{
		name: xsd.QName{Local: "b"},
		kids: []Child{ElementChild(c)},
		loc:  loc(5, 3),
	}
	a := &testElement{
		name: xsd.QName{Local: "a"},
		kids: []Child{TextChild(&testText{data: "in a", loc: loc(3, 6)})},
		loc:  loc(3, 3),
	}
	return &testElement{
		name: xsd.QName{Local: "root"},
		attrs: []Attribute{
			&testAttribute{name: xsd.QName{Local: "id"}, value: "r1", loc: loc(1, 23)},
			&testAttribute{name: xsd.QName{Space: "urn:p", Local: "lang"}, value: "en", loc: loc(1, 31)},
		},
		kids: []Child{
			TextChild(&testText{data: "top", loc: loc(2, 3)}),
			ElementChild(a),
			TextChild(&testText{data: "between", loc: loc(4, 3)}),
			ElementChild(b),
		},
		bindings: map[string]string{"p": "urn:p"},
		loc:      loc(1, 1),
	}
}

// wantVisits is sampleTree's every information item, once each, in the order
// cvc-assess-elt (§3.3.4.6) fixes: the element, then its [[attributes]], then
// its [[children]] in document order, recursively. Every key is qualified by
// the "validate" group New installs (STYLE L1). The first and last lines are
// not visits: rootSchema's declaration carries no {type definition}, so the
// root's type is undetermined, which is recorded as it is settled, and its
// ID/IDREF-table decline is recorded as it leaves.
var wantVisits = []string{
	rootTypeDeclined,
	"assessing element validate.name=root validate.loc=instance.xml:1:1",
	"assessing attribute validate.name=id validate.loc=instance.xml:1:23",
	"assessing attribute validate.name={urn:p}lang validate.loc=instance.xml:1:31",
	"assessing text validate.chars=3 validate.loc=instance.xml:2:3",
	"assessing element validate.name=a validate.loc=instance.xml:3:3",
	"assessing text validate.chars=4 validate.loc=instance.xml:3:6",
	"assessing text validate.chars=7 validate.loc=instance.xml:4:3",
	"assessing element validate.name=b validate.loc=instance.xml:5:3",
	"assessing element validate.name=c validate.loc=instance.xml:5:6",
	"assessing ID/IDREF table validate.name=root validate.loc=instance.xml:1:1 validate.rule=cvc-id validate.clause=1 validate.outcome=declined",
}

// rootTypeDeclined is the line the root's undetermined ·selected type
// definition· logs before the root is visited (walk.resolvedSelection).
const rootTypeDeclined = "assessing element validate.name=root validate.loc=instance.xml:1:1 validate.rule=cvc-elt validate.clause=1 validate.outcome=declined"

func TestAssessWalksEveryNodeOnceInDocumentOrder(t *testing.T) {
	log, visits := recordingLogger()
	v, err := New(rootSchema(t), testBackend(), WithLogger(log))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res := v.Assess(sampleTree())

	if !slices.Equal(*visits, wantVisits) {
		t.Errorf("walk visited\n\t%s\nwant\n\t%s",
			strings.Join(*visits, "\n\t"), strings.Join(wantVisits, "\n\t"))
	}
	if got := res.Violations(); len(got) != 0 {
		t.Errorf("Violations() = %v, want none", got)
	}
	if err := res.Err(); err != nil {
		t.Errorf("Err() = %v, want nil for a walk that completed", err)
	}
}

// onlyViolation returns the single violation res carries, failing when it
// carries any other number: every assertion below is about one charge, and a
// second one is a finding of its own rather than an index out of range.
func onlyViolation(t *testing.T, res *Result) *xsderr.Error {
	t.Helper()
	got := res.Violations()
	if len(got) != 1 {
		t.Fatalf("Violations() = %v, want exactly one", got)
	}
	return got[0]
}

// A validation root whose ·expanded name· resolves to no top-level element
// declaration determines neither a ·governing element declaration· nor a
// ·governing type definition·, so cvc-assess-elt clause 1 cannot apply and
// §5.2 strict wildcard validation reports it. The subtree is not walked:
// there is nothing to assess it against.
func TestAssessChargesAnUndeclaredRoot(t *testing.T) {
	tree := sampleTree()
	tree.name = xsd.QName{Space: "urn:p", Local: "undeclared"}

	log, visits := recordingLogger()
	v, err := New(rootSchema(t), testBackend(), WithLogger(log))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res := v.Assess(tree)

	got := onlyViolation(t, res)
	if got.Rule != "cvc-assess-elt" {
		t.Errorf("Rule = %q, want cvc-assess-elt", got.Rule)
	}
	if got.Loc != loc(1, 1) {
		t.Errorf("Loc = %s, want the root's %s", got.Loc, loc(1, 1))
	}
	if !strings.Contains(got.Msg, "{urn:p}undeclared") {
		t.Errorf("Msg = %q, want it to name the expanded name", got.Msg)
	}
	if len(*visits) != 0 {
		t.Errorf("walk visited\n\t%s\nwant nothing", strings.Join(*visits, "\n\t"))
	}
	if err := res.Err(); err != nil {
		t.Errorf("Err() = %v, want nil: an undeclared root is a violation, not a source fault", err)
	}
}

// An xsi:type on an undeclared root determines a ·governing type definition·
// where it ·resolves· (key-governing-type-elem clause 8, §5.2 naming it as one
// of the three determinants), so the charge above is withheld and the walk runs
// against that type alone. An xsi:type that does NOT resolve determines nothing
// and is charged exactly as a bare undeclared root is.
func TestAssessTypesAnUndeclaredRootFromAResolvedXSIType(t *testing.T) {
	typed := func(lexical string) *testElement {
		return &testElement{
			name: xsd.QName{Local: "undeclared"},
			attrs: []Attribute{&testAttribute{
				name:  xsd.QName{Space: xsd.XMLSchemaInstanceNS, Local: "type"},
				value: lexical,
				loc:   loc(1, 40),
			}},
			bindings: map[string]string{"p": "urn:p"},
			loc:      loc(1, 1),
		}
	}
	assess := func(tree *testElement) (*Result, *[]string) {
		log, visits := recordingLogger()
		v, err := New(rootSchema(t), testBackend(), WithLogger(log))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return v.Assess(tree), visits
	}

	// "T" is a top-level type of rootSchema; "p:T" names one in urn:p and
	// resolves to nothing.
	res, visits := assess(typed("T"))
	if got := res.Violations(); len(got) != 0 {
		t.Errorf("Violations() = %v, want none for a root whose xsi:type resolved", got)
	}
	if len(*visits) == 0 {
		t.Error("the walk did not run for a root typed by xsi:type")
	}

	res, _ = assess(typed("p:T"))
	if got := onlyViolation(t, res); got.Rule != "cvc-assess-elt" {
		t.Errorf("Rule = %q, want cvc-assess-elt for a root whose xsi:type resolved to nothing", got.Rule)
	}
}

// cvc-elt clause 2 requires D.{abstract} = false. The walk still runs after
// the charge: ·strictly assessed· clauses 2 and 3 assess [[attributes]] and
// [[children]] whatever clause 1.1.2's evaluation returned.
func TestAssessChargesAnAbstractRoot(t *testing.T) {
	tree := sampleTree()
	tree.name = xsd.QName{Local: "abstractRoot"}

	log, visits := recordingLogger()
	v, err := New(rootSchema(t), testBackend(), WithLogger(log))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res := v.Assess(tree)

	got := onlyViolation(t, res)
	if got.Rule != "cvc-elt" {
		t.Errorf("Rule = %q, want cvc-elt", got.Rule)
	}
	if !strings.Contains(got.Msg, "clause 2") {
		t.Errorf("Msg = %q, want it to name the clause the catalog cannot carry", got.Msg)
	}
	if got.Loc != loc(1, 1) {
		t.Errorf("Loc = %s, want the root's %s", got.Loc, loc(1, 1))
	}
	if !strings.Contains(got.Msg, "abstractRoot") {
		t.Errorf("Msg = %q, want it to name the expanded name", got.Msg)
	}
	want := slices.Clone(wantVisits)
	for _, i := range []int{0, 1, len(want) - 1} {
		want[i] = strings.Replace(want[i], "name=root", "name=abstractRoot", 1)
	}
	if !slices.Equal(*visits, want) {
		t.Errorf("walk visited\n\t%s\nwant\n\t%s",
			strings.Join(*visits, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// A Validator holds no walk state, so assessing twice must not carry the
// first walk into the second.
func TestValidatorIsReusable(t *testing.T) {
	log, visits := recordingLogger()
	v, err := New(rootSchema(t), testBackend(), WithLogger(log))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	first := v.Assess(sampleTree())
	firstVisits := slices.Clone(*visits)
	*visits = nil
	second := v.Assess(sampleTree())

	if !slices.Equal(*visits, firstVisits) {
		t.Errorf("second walk visited\n\t%s\nwant the first walk's\n\t%s",
			strings.Join(*visits, "\n\t"), strings.Join(firstVisits, "\n\t"))
	}
	if first == second {
		t.Error("Assess returned the same *Result twice")
	}
	if err := second.Err(); err != nil {
		t.Errorf("second Err() = %v, want nil", err)
	}
}

// A fault the source reports mid-document stops the walk and surfaces as
// Result.Err, and the subtree past it is not assessed.
func TestAssessStopsAtASourceFault(t *testing.T) {
	fault := errors.New("truncated document")
	faulty := &testElement{name: xsd.QName{Local: "a"}, kidsErr: fault, loc: loc(2, 3)}
	unreached := &testElement{name: xsd.QName{Local: "z"}, loc: loc(3, 3)}
	root := &testElement{
		name: xsd.QName{Local: "root"},
		kids: []Child{ElementChild(faulty), ElementChild(unreached)},
		loc:  loc(1, 1),
	}

	log, visits := recordingLogger()
	v, err := New(rootSchema(t), testBackend(), WithLogger(log))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res := v.Assess(root)

	if !errors.Is(res.Err(), fault) {
		t.Errorf("Err() = %v, want it to wrap %v", res.Err(), fault)
	}
	want := []string{
		rootTypeDeclined,
		"assessing element validate.name=root validate.loc=instance.xml:1:1",
		"assessing element validate.name=a validate.loc=instance.xml:2:3",
	}
	if !slices.Equal(*visits, want) {
		t.Errorf("walk visited\n\t%s\nwant\n\t%s",
			strings.Join(*visits, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// A Child built by neither constructor names no information item. That is an
// adapter bug and not a fault in the source, so it panics — Result.Err is
// reserved for the source faults that stop a walk, and routing both there
// would report a bug in an adapter to a user as a broken document.
func TestAssessPanicsOnZeroChild(t *testing.T) {
	root := &testElement{
		name: xsd.QName{Local: "root"},
		kids: []Child{{}, ElementChild(&testElement{name: xsd.QName{Local: "z"}})},
		loc:  loc(1, 1),
	}

	log, visits := recordingLogger()
	v, err := New(rootSchema(t), testBackend(), WithLogger(log))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Assess did not panic on a Child holding neither arm")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "Child holds neither arm") {
			t.Errorf("Assess panicked with %v, wanted the message to name the empty Child", r)
		}
		// The walk stops at the empty child: root is visited, the sibling
		// past it is not.
		want := []string{rootTypeDeclined, "assessing element validate.name=root validate.loc=instance.xml:1:1"}
		if !slices.Equal(*visits, want) {
			t.Errorf("walk visited\n\t%s\nwant\n\t%s",
				strings.Join(*visits, "\n\t"), strings.Join(want, "\n\t"))
		}
	}()

	v.Assess(root)
}

// The reference tree models xmlns:p the way an adapter must: not as an
// attribute (Appendix D's separate [[namespace attributes]] property), and
// reachable only through LookupPrefix.
func TestNamespaceDeclarationsAreNotAttributes(t *testing.T) {
	root := sampleTree()
	for _, a := range root.Attributes() {
		if a.Name().Local == "xmlns" || strings.HasPrefix(a.Name().Local, "xmlns:") {
			t.Errorf("Attributes() reports the namespace declaration %s", a.Name())
		}
	}
	if uri, ok := root.LookupPrefix("p"); !ok || uri != "urn:p" {
		t.Errorf("LookupPrefix(p) = (%q,%v), want (urn:p,true)", uri, ok)
	}
	if _, ok := root.LookupPrefix("unbound"); ok {
		t.Error("LookupPrefix(unbound) resolved")
	}
}

// A nil backend is a caller bug and not a validity verdict, so it panics
// rather than joining New's one error condition — and it panics BEFORE the nil
// schema is reported, so a caller passing neither learns about the one a
// default could never have supplied.
func TestNewPanicsOnANilBackend(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("New(schema, nil) did not panic")
		}
	}()
	_, _ = New(nil, nil)
}

func TestNewRejectsNilSchema(t *testing.T) {
	v, err := New(nil, testBackend())
	if err == nil {
		t.Fatal("New(nil) = nil error, want one")
	}
	if v != nil {
		t.Errorf("New(nil) returned a %T, want nil", v)
	}
	var xerr *xsderr.Error
	if errors.As(err, &xerr) {
		t.Errorf("New(nil) returned %T: a missing schema is not a validity verdict", xerr)
	}
}

func TestNewDefaultsAndNilLogger(t *testing.T) {
	// The zero option set and an explicit nil logger are both usable: a
	// walk under either runs to completion and logs nothing (STYLE L1).
	for _, opts := range [][]Option{nil, {WithLogger(nil)}} {
		v, err := New(rootSchema(t), testBackend(), opts...)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if res := v.Assess(sampleTree()); res.Err() != nil {
			t.Errorf("Err() = %v, want nil", res.Err())
		}
	}
}

func TestSchemaViewResolvesElements(t *testing.T) {
	schema := emptySchema(t)
	v, err := New(schema, testBackend())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, ok := v.Schema().Element(xsd.QName{Local: "absent"}); ok {
		t.Error("the empty schema resolved an element declaration")
	}
}

func TestResultViolationsAreCopied(t *testing.T) {
	// Violations is a window, not a handle: mutating what it returns must
	// not reach the Result (xsd.Schema.Elements' convention).
	res := &Result{violations: []*xsderr.Error{xsderr.New("cvc-elt.1", loc(1, 1), "sample")}}
	got := res.Violations()
	got[0] = nil
	if res.Violations()[0] == nil {
		t.Error("Violations() shares its backing array with the Result")
	}
	if (&Result{}).Violations() != nil {
		t.Error("Violations() = non-nil for a Result carrying none")
	}
}

// TestResultStanding pins Result.Standing over every shape of its three
// inputs: a violation charges whatever else the Result holds, and an
// unevaluated check or a source fault without one leaves it undecided.
func TestResultStanding(t *testing.T) {
	v := []*xsderr.Error{xsderr.New("cvc-elt.1", loc(1, 1), "sample")}
	u := []Unevaluated{newUnevaluated("cvc-assertion", loc(1, 1), "sample")}
	e := errors.New("source fault")
	for _, s := range []Standing{Charged, Undecided, NotProvenInvalid} {
		if s == 0 {
			t.Errorf("an arm is the zero Standing, which must be no arm")
		}
	}
	for _, c := range []struct {
		name string
		res  Result
		want Standing
	}{
		{"none", Result{}, NotProvenInvalid},
		{"U", Result{unevaluated: u}, Undecided},
		{"E", Result{err: e}, Undecided},
		{"U,E", Result{unevaluated: u, err: e}, Undecided},
		{"V", Result{violations: v}, Charged},
		{"V,U", Result{violations: v, unevaluated: u}, Charged},
		{"V,E", Result{violations: v, err: e}, Charged},
		{"V,U,E", Result{violations: v, unevaluated: u, err: e}, Charged},
	} {
		if got := c.res.Standing(); got != c.want {
			t.Errorf("%s: Standing() = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestAssessRejectsNilRoot(t *testing.T) {
	v, err := New(emptySchema(t), testBackend())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Error("Assess(nil) did not panic")
		}
	}()
	v.Assess(nil)
}

// All FOUR charges that DELEGATE to String Valid (§3.16.4, cvc-simple-type)
// carry the delegated rule's verdict as their wrapped cause, so a consumer reads
// the inner rule ID off the chain instead of scraping the message for it (#914,
// #924). Each is driven here through its own fixture: cvc-attribute clause 3
// (§3.2.4.1), cvc-type clause 3.1.3 (§3.3.4.4), and cvc-complex-type (§3.4.4.2)
// clauses 1.2 and 4. Two rows charge the one cvc-complex-type rule, which is why
// the subtest is keyed on the row's own name and not on the rule ID.
//
// Clause 4 reaches its verdict through xsd.ValueSpace's ValidDefault rather than
// value.ValidateLexical directly, and wraps what that hands back. String Valid
// clause 2 is Datatype Valid (Datatypes §4.1.4), so the inner rule an xs:decimal
// lexical failure carries is cvc-datatype-valid at every one of the four.
func TestDelegatingChargesWrapTheirCause(t *testing.T) {
	decimal := icBuiltin("decimal")
	for _, tc := range []struct {
		name   string
		rule   xsderr.Rule
		charge func(*testing.T) []*xsderr.Error
	}{
		{"cvc-attribute clause 3", "cvc-attribute", func(t *testing.T) []*xsderr.Error {
			return assessTyped(t, valuedRoot("n", "12,50"),
				[]xsd.AttributeUse{typedUse(t, "n", decimal, false, nil, nil)})
		}},
		{"cvc-type clause 3.1.3", "cvc-type", func(t *testing.T) []*xsderr.Error {
			return cAssess(t, simpleTypedSchema(t, decimal, nil, false), cRoot("#12,50"))
		}},
		{"cvc-complex-type clause 1.2", "cvc-complex-type", func(t *testing.T) []*xsderr.Error {
			return cAssess(t, simpleContentSchema(t, decimal), cRoot("#12,50"))
		}},
		{"cvc-complex-type clause 4", "cvc-complex-type", func(t *testing.T) []*xsderr.Error {
			bad := xsd.NewValueConstraint(xsd.ValueDefault, "12,50", nil, nil)
			return assessTyped(t, &testElement{name: xsd.QName{Local: "root"}, loc: loc(1, 1)},
				[]xsd.AttributeUse{typedUse(t, "n", decimal, false, &bad, nil)})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			viol := onlyCharge(t, tc.charge(t), tc.rule)

			inner := errors.Unwrap(viol)
			if inner == nil {
				t.Fatalf("Unwrap() = nil, want the String Valid verdict %s delegates to", tc.rule)
			}
			var datatype *xsderr.Error
			if !errors.As(inner, &datatype) {
				t.Fatalf("errors.As found no *xsderr.Error in the wrapped cause %v", inner)
			}
			if got, ok := xsderr.RuleOf(datatype); !ok || got != "cvc-datatype-valid" {
				t.Errorf("RuleOf(Unwrap()) = %q, %t, want cvc-datatype-valid", got, ok)
			}

			// The verdict is rendered into the message as well: a reader
			// holding only the string has no chain to walk. The wrapped
			// verdict carries no position of its own, so it renders from its
			// fields as "[rule] msg" — never as its Error(), which would put
			// the zero Loc's "?: " inside a line the outer Loc already places
			// (#1844).
			//nolint:errorlint // identity, not chain membership: the rendering covers only a cause that IS the *xsderr.Error.
			if inner != error(datatype) || datatype.Loc != (xsderr.Loc{}) {
				t.Fatalf("wrapped cause = %#v, want itself the *xsderr.Error with the zero Loc this rendering covers", inner)
			}
			want := fmt.Sprintf("[%s] %s", datatype.Rule, datatype.Msg)
			if !strings.HasSuffix(viol.Msg, ": "+want) || strings.HasSuffix(viol.Msg, inner.Error()) {
				t.Errorf("Msg = %q, want it to end with the wrapped verdict rendered as %q, without the zero Loc's placeholder", viol.Msg, want)
			}
			if !strings.Contains(viol.Msg, "cvc-datatype-valid") || !strings.Contains(viol.Msg, `"12,50"`) {
				t.Errorf("Msg = %q, want the inner rule ID and the offending lexical still in it", viol.Msg)
			}
		})
	}
}

// A charge that delegates to nothing wraps nothing: Unwrap is nil rather than
// some placeholder, which is what lets a consumer test the chain for the
// delegation instead of testing it for a sentinel.
func TestNonDelegatingChargeWrapsNothing(t *testing.T) {
	got := cAssess(t, cSchema(t, cSequence(t, false, cParticle(t, "a", 1, 1))), cRoot("b"))

	viol := onlyCharge(t, got, "cvc-complex-content")
	if errors.Unwrap(viol) != nil {
		t.Errorf("Unwrap() = %v, want nil for a charge that delegates to no other rule", errors.Unwrap(viol))
	}
}

// causedBy drops only the zero Loc's placeholder: a cause that is not itself an
// *xsderr.Error, or is one with a position of its own, still renders as %v
// renders it, position included (#1844). Every arm keeps the identical cause
// as the wrapped one.
func TestCausedByRendersOnlyAnUnplacedCauseFromItsFields(t *testing.T) {
	placed := xsderr.New("cvc-datatype-valid", loc(2, 7), "boom")
	for _, tc := range []struct {
		name  string
		cause error
		want  string
	}{
		{"unplaced *xsderr.Error", xsderr.New("cvc-datatype-valid", xsderr.Loc{}, "boom"), "charged: [cvc-datatype-valid] boom"},
		{"placed *xsderr.Error", placed, "charged: " + placed.Error()},
		{"plain error", errors.New("boom"), "charged: boom"},
		{"wrapped unplaced *xsderr.Error", fmt.Errorf("via: %w", xsderr.New("cvc-datatype-valid", xsderr.Loc{}, "boom")), "charged: via: ?: [cvc-datatype-valid] boom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := causedBy("cvc-type", loc(1, 1), tc.cause, "charged")
			if v.Msg != tc.want {
				t.Errorf("Msg = %q, want %q", v.Msg, tc.want)
			}
			//nolint:errorlint // identity, not chain membership: Unwrap must return the very cause passed in.
			if errors.Unwrap(v) != tc.cause {
				t.Errorf("Unwrap() = %v, want the identical cause %v", errors.Unwrap(v), tc.cause)
			}
		})
	}
}
