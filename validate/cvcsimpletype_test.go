package validate

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// These fixtures drive String Valid (§3.16.4) clause 3, "every ·ENTITY value·
// in V is a ·declared entity name·", through each charge that delegates to
// String Valid. A root that presents [unparsedEntities] is an entityRoot; a
// bare testElement is the source that does not, whose every ·ENTITY value·
// Appendix D fails.

// entityRoot is a validation root whose source presents [unparsedEntities]:
// declared are the [name]s of its unparsed entity information items.
type entityRoot struct {
	*testElement
	declared []string
}

func (e entityRoot) HasUnparsedEntity(name string) bool { return slices.Contains(e.declared, name) }

// withEntities presents root's document as declaring the unparsed entities
// named.
func withEntities(root *testElement, declared ...string) entityRoot {
	return entityRoot{testElement: root, declared: declared}
}

// wantClauseThree fails unless got is exactly one charge under rule whose
// wrapped cause is a String Valid clause 3 verdict naming value, and returns
// that cause's message.
func wantClauseThree(t *testing.T, got []*xsderr.Error, rule xsderr.Rule, value string) string {
	t.Helper()
	viol := onlyCharge(t, got, rule)
	var cause *xsderr.Error
	if !errors.As(errors.Unwrap(viol), &cause) {
		t.Fatalf("Unwrap(%v) holds no *xsderr.Error, want the String Valid clause 3 verdict", viol)
	}
	if cause.Rule != ruleCvcSimpleType {
		t.Fatalf("wrapped Rule = %q, want %q", cause.Rule, ruleCvcSimpleType)
	}
	prefix := "value " + `"` + value + `"`
	if !strings.HasPrefix(cause.Msg, prefix) || !strings.Contains(cause.Msg, "cvc-simple-type clause 3") {
		t.Fatalf("wrapped Msg = %q, want it to open %q and cite cvc-simple-type clause 3", cause.Msg, prefix)
	}
	return cause.Msg
}

// An xs:ENTITY attribute naming a declared unparsed entity is ·valid·; one
// naming no unparsed entity is not a ·declared entity name· (key-vde), which
// cvc-attribute clause 3 charges with the clause 3 verdict as its cause, at the
// attribute.
func TestAnEntityAttributeMustNameADeclaredUnparsedEntity(t *testing.T) {
	uses := []xsd.AttributeUse{typedUse(t, "ent", icBuiltin("ENTITY"), false, nil, nil)}

	wantSilence(t, assessTyped(t, withEntities(valuedRoot("ent", "pic"), "pic"), uses), "pic is a declared entity name")

	got := assessTyped(t, withEntities(valuedRoot("ent", "ghost"), "pic"), uses)
	msg := wantClauseThree(t, got, ruleCvcAttribute, "ghost")
	if !strings.Contains(msg, "names no unparsed entity in the document's [unparsedEntities], so it is not a ·declared entity name·") {
		t.Errorf("wrapped Msg = %q, want the undeclared arm's ·declared entity name· (key-vde) reason", msg)
	}
	if got[0].Loc != loc(1, 10) {
		t.Errorf("Loc = %s, want the attribute's %s", got[0].Loc, loc(1, 10))
	}
}

// A source that does not implement [UnparsedEntities] supports no
// [unparsedEntities], and Appendix D fails every ·ENTITY value· it presents —
// the name need not be undeclared to fail. A value of any other type is
// untouched by the absence.
func TestASourceWithoutUnparsedEntitiesFailsEveryEntityValue(t *testing.T) {
	uses := []xsd.AttributeUse{typedUse(t, "ent", icBuiltin("ENTITY"), false, nil, nil)}

	msg := wantClauseThree(t, assessTyped(t, valuedRoot("ent", "pic"), uses), ruleCvcAttribute, "pic")
	if !strings.Contains(msg, "presents no [unparsedEntities]") {
		t.Errorf("wrapped Msg = %q, want the capability-absent arm", msg)
	}

	ints := []xsd.AttributeUse{typedUse(t, "n", integerType(), false, nil, nil)}
	wantSilence(t, assessTyped(t, valuedRoot("n", "42"), ints), "an xs:integer holds no ·ENTITY value·")
}

// An xs:ENTITIES value holds one ·ENTITY value· per list item (key-vtype
// clause 2), each checked on its own: the verdict names the undeclared item
// and not its declared neighbours.
func TestEachEntitiesItemMustBeDeclared(t *testing.T) {
	uses := []xsd.AttributeUse{typedUse(t, "ents", icBuiltin("ENTITIES"), false, nil, nil)}

	wantSilence(t, assessTyped(t, withEntities(valuedRoot("ents", "a b"), "a", "b"), uses), "both items are declared")
	wantClauseThree(t, assessTyped(t, withEntities(valuedRoot("ents", "a ghost b"), "a", "b"), uses),
		ruleCvcAttribute, "ghost")
}

// A union's ·ENTITY values· are the ones its ENTITY member validated
// (key-TYPE-value): union(integer, ENTITY) reads "42" as an xs:integer and so
// asks nothing of [unparsedEntities], even of a source that has none, and
// reads "pic" as an ·ENTITY value·.
func TestAUnionChecksOnlyWhatItsEntityMemberValidated(t *testing.T) {
	union := icUnion(t, "IntOrEntity", icBuiltin("integer"), icBuiltin("ENTITY"))
	uses := []xsd.AttributeUse{icUseOf(t, xsd.QName{Local: "u"}, xsd.QName{Local: "IntOrEntity"})}

	wantSilence(t, assessTyped(t, valuedRoot("u", "42"), uses, union), "the integer member validated 42")
	wantClauseThree(t, assessTyped(t, valuedRoot("u", "pic"), uses, union), ruleCvcAttribute, "pic")
	wantSilence(t, assessTyped(t, withEntities(valuedRoot("u", "pic"), "pic"), uses, union), "pic is declared")
}

// The ·validating type· classification the [ID/IDREF table] reads is shared,
// and an ·ENTITY value· is no ·IDREF value·: union(ENTITY, IDREF) reads "pic"
// as an ·ENTITY value· through its first member, so the table gets no entry for
// cvc-id clause 1 to charge as unresolved.
func TestAnEntityValueIsNoIDREFValue(t *testing.T) {
	union := icUnion(t, "EntityOrRef", icBuiltin("ENTITY"), icBuiltin("IDREF"))
	uses := []xsd.AttributeUse{icUseOf(t, xsd.QName{Local: "u"}, xsd.QName{Local: "EntityOrRef"})}

	wantSilence(t, assessTyped(t, withEntities(valuedRoot("u", "pic"), "pic"), uses, union),
		"pic is a declared ·ENTITY value· and references no id")
}

// Sharing the classification does not widen ID candidacy: a ·defaulted
// attribute· of type ENTITY is outside §3.17.5.2's ID closure, so it declines
// nothing and an unresolved IDREF beside it is still charged under cvc-id
// clause 1.
func TestAnEntityTypeIsNoIDCandidate(t *testing.T) {
	pic := xsd.NewValueConstraint(xsd.ValueDefault, "pic", nil, nil)
	uses := []xsd.AttributeUse{
		typedUse(t, "ref", icBuiltin("IDREF"), false, nil, nil),
		typedUse(t, "ent", icBuiltin("ENTITY"), false, &pic, nil),
	}

	if got := onlyCharge(t, assessTyped(t, withEntities(valuedRoot("ref", "ghost"), "pic"), uses), ruleCvcID); got.Loc != loc(1, 10) {
		t.Errorf("Loc = %s, want the unresolved IDREF's %s", got.Loc, loc(1, 10))
	}
}

// An element's ·initial value· goes through the same String Valid: cvc-type
// clause 3.1.3 for an ENTITY-governed element, and cvc-complex-type clause 1.2
// for simple content of that type, each charged at the element.
func TestAnEntityInitialValueMustBeDeclared(t *testing.T) {
	for _, tc := range []struct {
		name   string
		schema *xsd.Schema
		rule   xsderr.Rule
	}{
		{"cvc-type clause 3.1.3", simpleTypedSchema(t, icBuiltin("ENTITY"), nil, false), ruleCvcType},
		{"cvc-complex-type clause 1.2", simpleContentSchema(t, icBuiltin("ENTITY")), ruleCvcComplexType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantSilence(t, cAssess(t, tc.schema, withEntities(cRoot("#pic"), "pic")), "pic is declared")
			got := cAssess(t, tc.schema, withEntities(cRoot("#ghost"), "pic"))
			wantClauseThree(t, got, tc.rule, "ghost")
			if got[0].Loc != loc(1, 1) {
				t.Errorf("Loc = %s, want the element's %s", got[0].Loc, loc(1, 1))
			}
		})
	}
}

// A ·defaulted attribute·'s {lexical form} is String Valid under
// cvc-complex-type clause 4 too, and whether the ·ENTITY value· it supplies is
// declared is the DOCUMENT's to say.
func TestADefaultedEntityAttributeMustBeDeclared(t *testing.T) {
	ghost := xsd.NewValueConstraint(xsd.ValueDefault, "ghost", nil, nil)
	uses := []xsd.AttributeUse{typedUse(t, "ent", icBuiltin("ENTITY"), false, &ghost, nil)}
	bare := func() *testElement { return &testElement{name: xsd.QName{Local: "root"}, loc: loc(1, 1)} }

	wantSilence(t, assessTyped(t, withEntities(bare(), "ghost"), uses), "the default names a declared entity")
	wantClauseThree(t, assessTyped(t, withEntities(bare(), "pic"), uses), ruleCvcComplexType, "ghost")
}

// processedRoot is an entityRoot whose source also presents [all declarations
// processed].
type processedRoot struct {
	entityRoot
	processed bool
}

func (p processedRoot) AllDeclarationsProcessed() bool { return p.processed }

// [all declarations processed] words the miss and never the verdict: a source
// reporting its DTD not fully read has an undeclared name charged under the
// same clause 3 as one reporting every declaration processed, but the message
// says the DTD was not fully read. A source that does not implement
// [DeclarationsProcessed] is taken to have processed every declaration, and a
// declared name passes whatever the source reports.
func TestAnUnreadDTDWordsTheMissButNotTheVerdict(t *testing.T) {
	uses := []xsd.AttributeUse{typedUse(t, "ent", icBuiltin("ENTITY"), false, nil, nil)}
	const unread = "the document's DTD was not fully read"

	wantSilence(t, assessTyped(t, processedRoot{withEntities(valuedRoot("ent", "pic"), "pic"), false}, uses),
		"pic is declared, though the DTD was not fully read")
	for _, tc := range []struct {
		why    string
		root   Element
		unread bool
	}{
		{"declarations unread", processedRoot{withEntities(valuedRoot("ent", "ghost"), "pic"), false}, true},
		{"every declaration processed", processedRoot{withEntities(valuedRoot("ent", "ghost"), "pic"), true}, false},
		{"no DeclarationsProcessed", withEntities(valuedRoot("ent", "ghost"), "pic"), false},
	} {
		msg := wantClauseThree(t, assessTyped(t, tc.root, uses), ruleCvcAttribute, "ghost")
		if got := strings.Contains(msg, unread); got != tc.unread {
			t.Errorf("%s: wrapped Msg = %q, says %q: %t, want %t", tc.why, msg, unread, got, tc.unread)
		}
	}
}
