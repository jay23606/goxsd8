// Package xpath is the XSD processor's internal XPath 2.0 engine, serving
// conditional type assignment (CTA) and assertions, and it offers no
// free-standing Compile(expr) and no evaluation of an expression outside a
// schema component. CompileCTATest, CTATestStaticError and
// CompileAssertionTest take xsd components, an xsd.XPathExpression and an
// xsd.TypeResolver among them, and FacetAssertions returns the
// value.AssertionEvaluator that does. Package parser calls CTATestStaticError
// when it assembles a schema; package validate calls the rest when it assesses
// an instance, and drives the compiled tests and the Tally they return. The
// full grammar and function library are tier 3 below (docs/ARCHITECTURE.md §
// XPath, PLAN M7).
//
// Identity-constraint {selector}/{fields} (§3.11.6.2/3) are compiled and
// evaluated by icpath against the restricted selector/field path grammar those
// sections define, never through this engine (docs/ARCHITECTURE.md).
//
// # Growth tiers
//
// Full XPath 2.0 is the destination; the engine grows outward from the
// XSD-required subset, tracked by its own conformance lane.
//
//  1. The CTA restricted subset (the `test` attribute of xs:alternative),
//     Structures §3.12.6's productions [8] ta-Test through [18]
//     ta-ConstructorFunction — M6. SHIPPED, less the forms the CTA column of
//     the table below declines: CompileCTATest compiles a {test} and
//     CTATest.Evaluate decides it against an element's attributes.
//     CTATestStaticError reports the XPath STATIC errors of the same grammar
//     over the same traversal, and err:XPST0003 for a {test} outside the full
//     XPath 2.0 grammar, which is a different question with a different owner
//     — see below.
//  2. Assertion essentials: axes, predicates, quantified expressions, typed
//     comparisons, the F&O function core — M6. FIRST SLICES SHIPPED: the
//     assert and facet columns of the table below.
//  3. The full grammar (docs/specs/md/xpath20.md) and function library
//     (docs/specs/md/xpath-functions.md) — M7 onward, ratcheted.
//     PLANNED.
//
// # Forms each entry point evaluates
//
// The table answers, for each form, whether CompileCTATest (CTA),
// CompileAssertionTest (assert) and FacetAssertions (facet) evaluate it:
//
//   - yes: compiled and evaluated;
//   - no: declined at compile time, the withhold the fail-open contract below
//     states; a row ending PLANNED is tier 2's to admit;
//   - err: compiled, and raising the dynamic error its row names;
//   - ctx: compiled, and raising err:XPDY0002, which fails the facet. An
//     assertions facet's {test} (Datatypes §4.3.13.3, cvc-assertions-valid)
//     has no context item, position or size (clause 1.3), and binds `$value`
//     to the value under the facet's type, or under its ·active basic member·
//     where that type is a union (dt-xdmrep clause 4).
//
// A row's yes holds of the operands the other rows admit, and another row's no
// narrows it. Over `.` or an attribute or child operand, a facet {test} whose
// row the facet column admits raises err:XPDY0002 instead, and one it
// declines may.
//
// An assertion reads E's attributes TYPED (AttributeTypes; cvc-assertion
// clause 1, §3.13.4.1), and its element children and the elements below them
// TYPED (ElementTypes, by their path of names below E):
// AssertionTest.Evaluate decides over their typed values (TypedAttributes,
// ChildElements, ChildElement, Child, ChildWithAttributes, TypedValue), each
// an ·actual value·, or xs:untypedAtomic for an attribute whose type is
// ·special· (xpath-datamodel §3.3.1.2), for a `$value` over ·special·
// content, and for a child whose ·locally declared type· has mixed content,
// xs:anyType among them, which is its string-value, its descendants' text
// included (§6.2.4), and which is cast as tier 1 casts an untyped one. Which
// children, attributes and subtree nodes a compiled {test} reads, and how its
// caller reports them, is AssertionTest.ReadsChild's,
// AssertionTest.ReadsChildAttributes', AssertionTest.ReadsContextItem's and
// Tally's to state.
//
//	CTA  assert facet  form
//
//	                   Tier 1: Structures §3.12.6's grammar
//	yes  yes    yes    [8] ta-Test through [18] ta-ConstructorFunction; [15]
//	                   ta-CastExpr's `cast as` tail and [18]
//	                   ta-ConstructorFunction are one node, evaluated through
//	                   value's facet pipeline, which is what
//	                   xpath-functions.md §17.1.1 makes a cast
//	yes  yes    ctx    [17] ta-AttrName `@a`, E's attribute
//	yes  no     ctx    [17] ta-AttrName's wildcards, `@*`, `@p:*` and `@*:n`:
//	                   its NameTest is xpath20.md's [36] whole, selecting a
//	                   sequence of E's attributes, and a comparison over one
//	                   is §3.5.2's existential
//	yes  yes    yes    a cast to xs:QName over a StringLiteral, whose prefix
//	                   resolves against the static context
//	                   (xpath-functions.md §5.3)
//	no   no     no     a cast to xs:QName over any other operand but an
//	                   xs:QName, which §17.1 makes err:XPTY0004, or over a
//	                   StringLiteral whose prefix is unbound, err:FONS0004
//	no   no     no     a cast of an xs:float or xs:double operand, such as
//	                   `1.5e0`, to a target other than its own type or an
//	                   ancestor of it, and fn:string over such an operand
//	no   no     no     a cast of an IntegerLiteral or DecimalLiteral to a
//	                   target outside the xs:string family, xs:float,
//	                   xs:double, xs:decimal and, for an IntegerLiteral,
//	                   xs:integer's branch, such as `xs:integer(1.5)`, which
//	                   F&O §17.1.2, §17.1.3 and §17.1.6 define over the value
//	                   and not over its canonical lexical where §17.1's
//	                   casting table marks the target Y or M, and §17.1
//	                   makes err:XPTY0004 where it marks N
//	no   no     no     a comparison, or an arithmetic operand, needing
//	                   xpath20.md B.1 rule 1.1's xs:float-to-xs:double
//	                   promotion, which value exposes no widening for
//	no   no     no     a cast whose TARGET is not a builtin datatype, the
//	                   required subset's own boundary (§3.12.6 clause 4)
//
//	                   Tier 2: every row outside §3.12.6's subset
//	no   yes    yes    the value comparisons eq, ne, lt, le, gt and ge
//	                   (xpath20.md §3.5.1)
//	no   yes    yes    `$value` (cvc-assertion clause 2.3; ValueBinding)
//	no   yes    ctx    [47] ContextItemExpr `.` (cvc-assertion clause 2.3.1's
//	                   Note; Static context below), and fn:string,
//	                   fn:string-length and fn:normalize-space with no
//	                   argument, which read it
//	no   no     ctx    `.` outside a value predicate under a nil content, or
//	                   where it is a node: the whole operand of an ·effective
//	                   boolean value·, fn:not, fn:exists or fn:empty; and the
//	                   zero-argument string functions wherever `.` declines
//	no   no     no     `.` as the whole operand of fn:count
//	no   yes    ctx    fn:namespace-uri over `.` or with no argument (F&O
//	                   §14.3), E's [namespace name] as one xs:anyURI, reading
//	                   `.` as E's node through the ContextElement Evaluate is
//	                   handed, which validate.Element satisfies, and never its
//	                   string value
//	no   no     no     fn:namespace-uri over any other argument
//	no   yes    ctx    fn:in-scope-prefixes over `.` (F&O §11.2.6), reading
//	                   `.` as fn:namespace-uri does, as one operand of `=`
//	                   against a StringLiteral or a parenthesized sequence of
//	                   them: true where one is a prefix of E's [in-scope
//	                   namespaces]
//	no   no     no     fn:in-scope-prefixes over any other argument or in any
//	                   other position, free-standing, under fn:count, beside
//	                   `!=` or `eq`; PLANNED in other positions, which need a
//	                   listing of E's [in-scope namespaces] that
//	                   validate.Element lacks
//	no   no     ctx    fn:position and fn:last but in a quantifier's
//	                   `N[position() lt last()]` below
//	no   yes    ctx    an abbreviated child-axis step with a QName NameTest,
//	                   `N` (§3.2.1.1), whose unprefixed name takes the
//	                   {default namespace}
//	no   yes    ctx    an attribute step behind `./`, `./@a`, E's own `@a`
//	                   (§3.2.4)
//	no   err    ctx    a "/" or "//" opening one child or attribute step,
//	                   which raises err:XPDY0050 because the instance's root
//	                   is E and not a document node (§3.2)
//
//	                   As the whole operand of fn:exists, fn:empty or an
//	                   ·effective boolean value·, whose steps are never typed
//	                   and which selects any node, ·nilled· or not, of any
//	                   type, at the end of its chain of names:
//	no   yes    ctx    a relative path of two or more child steps, `a/b`
//	                   (§3.2), whose last step may be `*`, `a/*`, every
//	                   element child (§3.2.1.2)
//	no   yes    ctx    one element step, `a`, `./a` or `.//a` (§3.2.4)
//	no   yes    ctx    one step whose NodeTest is [58] CommentTest, a [54]
//	                   KindTest, `comment()`, `./comment()` or
//	                   `.//comment()`, which selects the empty sequence
//	                   because cvc-assertion clause 1.2 excludes comments from
//	                   the partial ·PSVI· and this engine offers no user
//	                   option to retain them
//	no   yes    ctx    a child step filtered by a conjunction of child steps,
//	                   `a[b and c]`, each a one of whose children is named
//	                   each conjunct, so two a splitting the names select
//	                   nothing (§3.2.2)
//	no   yes    ctx    a child step filtered by one `preceding::` step,
//	                   unfiltered or filtered by one child step or fn:not
//	                   over one, `a[preceding::a[not(b)]]`, each a with an a
//	                   lacking a b child before it in E's subtree, at any
//	                   depth, E never among them (§3.2.1.1, cvc-assertion
//	                   clause 1.3)
//
//	                   fn:count (xpath-functions.md §15.4.1) over:
//	no   yes    no     `N` or `@N`, bare or behind `./` or `.//`
//	no   err    no     a rooted step, raising as it does
//	no   yes    no     a child step `N` or `./N` filtered by one predicate
//	                   (§3.2.2): a conjunction of attribute-existence tests,
//	                   `N[@a and @b]`, or a comparison, or `and`, `or` and
//	                   fn:not over comparisons, of the child's own typed
//	                   value `.`, `N[. = 'x']`, whose dynamic or type error
//	                   over any child makes the count raise
//	no   yes    no     a `|` or `union` of those relative operands but a
//	                   value-filtered one, each node counted once (§3.3.3)
//	no   yes    yes    an operand that is no path, whose items it counts:
//	                   `count($value)`, a list's items
//	no   no     no     an argument of any other shape, the comment step
//	                   among them
//
//	no   yes    yes    xpath20.md §3.4's binary arithmetic, `+`, `-`, `*`,
//	                   `div`, `idiv` and `mod`, over numeric operands, whose
//	                   dynamic errors err:FOAR0001 and err:FOAR0002 are
//	                   charged like any other
//	no   no     no     an arithmetic operand that is not numeric; a unary
//	                   sign
//	no   yes    yes    F&O's string and sequence core: fn:contains,
//	                   fn:starts-with and fn:ends-with under the codepoint
//	                   collation, fn:string-length, fn:normalize-space,
//	                   fn:string, fn:concat (§7.4.1, each argument cast to
//	                   xs:string), fn:substring-after (§7.5.5), fn:number
//	                   (§14.4, NaN over no item or one whose cast to
//	                   xs:double fails, over an operand whose cast to
//	                   xs:double is admitted), fn:empty, fn:exists,
//	                   fn:distinct-values (§15.1.6, by eq, one NaN
//	                   surviving), fn:true and fn:false; `()` is admitted as
//	                   an argument, and an xs:string? argument raises
//	                   err:XPTY0004 for an item of another type or for two
//	                   or more items
//	no   yes    yes    a constructor function over any operand such an
//	                   argument may be (xpath20.md §3.1.5),
//	                   `xs:date(concat(string($value), '!!!'))`
//	no   yes    ctx    fn:sum over a path through child steps to an attribute
//	                   below E, `sum(a/b/@p)` (§15.4.5), its attribute typed
//	                   by AttributeTypes at that path, the static type of
//	                   the element it ends at, and its values carried by the
//	                   Tally (Tally.ReadsAttribute, Tally.Attribute): the
//	                   xs:integer 0 over none, exact over xs:decimal items,
//	                   and its `instance of` read off the type of the value
//	                   it returns
//	no   yes    yes    fn:current-date (§16.4; Dynamic context below)
//	no   err    no     fn:doc (§15.5.4) over a StringLiteral, which raises
//	                   err:FODC0005 because cvc-xpath clause 8 leaves no
//	                   available document, followed or not by a path of
//	                   QName child or attribute steps that is never evaluated
//	no   yes    yes    fn:doc over `()`, the empty sequence, followed or not
//	                   by such a path
//	no   no     no     a collation argument; PLANNED
//	no   no     no     a function call of the wrong arity
//	no   no     no     every other F&O function; PLANNED
//
//	                   As a general comparison's operand:
//	no   yes    yes    an integer sequence: [11] RangeExpr over
//	                   IntegerLiterals, bare or parenthesized, or a
//	                   parenthesized comma sequence of IntegerLiterals and
//	                   such ranges, `(1 to 10, 20, 30)` (§3.3.1)
//	no   yes    yes    a string sequence: a parenthesized comma sequence of
//	                   StringLiterals, `('a', 'b')`, whose items are
//	                   xs:string
//	no   no     no     an integer or string sequence in any other position or
//	                   over any other operand: a member of another kind, a
//	                   mix of StringLiterals with the other two, a nested or
//	                   empty parenthesis, a misplaced comma, or longer than
//	                   4096 items
//
//	no   yes    yes    §3.8's `if (Expr) then ExprSingle else ExprSingle`
//	                   wherever an ExprSingle stands whole in a boolean
//	                   position, evaluating only the branch its test's
//	                   ·effective boolean value· selects
//	no   no     no     a conditional read as an item rather than for its
//	                   ·effective boolean value·; PLANNED
//
//	                   §3.9's `some` and `every` over one in-clause, `every`
//	                   over no child or item true and `some` false, binding
//	                   a range variable to:
//	no   yes    no     each child a QName step `N` or `./N` selects, whose
//	                   body is `$v/@A`, a conjunction of such, or
//	                   `$v/following-sibling::*[1][self::M]`, the next
//	                   sibling element where it is named M, or one of those
//	                   under fn:not, decided off Tally counts with no
//	                   variable bound at evaluation
//	no   yes    no     each child a QName step `N` or `./N` of a type a value
//	                   step reads TYPED selects, or each but the last under
//	                   `N[position() lt last()]` (§3.2.2), whose body is any
//	                   ExprSingle of this grammar, decided per child off
//	                   ChildElements: the range variable is a node whose
//	                   typed value is the step's, `$v/following-sibling::M[1]`
//	                   the first later child named M however far (§3.2.1.1),
//	                   and `$v/@A` and `$v/following-sibling::M[1]/@A` that
//	                   child's attribute A, typed by AttributeTypes at the
//	                   one-step path of its name and carried on its
//	                   ChildElement (ChildWithAttributes), either one ending
//	                   or not in a function call evaluated with the
//	                   attribute node as its context item,
//	                   `$v/@A/substring-after(., 'val')` (xpath20.md §3.2)
//	no   yes    no     each item of a typed `$value`, `data($value)` or
//	                   `$value` (xpath-functions.md §2.4), whose body is any
//	                   ExprSingle of this grammar, the range variable an
//	                   atomic value of `$value`'s item type in scope in the
//	                   body alone, decided per item with a decisive item
//	                   winning over an error
//	no   no     no     a quantifier over a statically empty or ·special·
//	                   `$value`; a nested quantifier; a path or `instance of`
//	                   over a range variable over `$value`; PLANNED
//	no   no     no     a quantifier over a child step whose child is of mixed
//	                   content or of a type a value step would not read, or
//	                   whose binding step carries any other predicate, `<`,
//	                   `ne` or `[last()]` among them, or whose window is
//	                   followed by a body decided off the Tally; PLANNED
//	no   no     no     a path over its range variable but
//	                   `following-sibling::M[1]`, and an attribute step over
//	                   either: `$v/@*`, an attribute of ·special· type, a
//	                   call after it carrying no type or reading another
//	                   node, any step after that call; PLANNED
//
//	no   yes    yes    §3.10.3's `castable as` over a [16] ta-SimpleValue,
//	                   true exactly where the same `cast as` yields a value
//	                   and false where that cast raises, an error evaluating
//	                   its operand raising
//	no   no     no     a `castable as` whose cast declines, over any operand
//	                   but a SimpleValue or in a value predicate; PLANNED
//	no   yes    yes    §3.10.1's `instance of` with a builtin atomic
//	                   SequenceType and an optional occurrence indicator over
//	                   an operand that is no node, true where the item count
//	                   is one the indicator admits and each item's type
//	                   derives from it (§2.5.4), never casting
//	no   yes    ctx    the same over an fn:data call (xpath-functions.md
//	                   §2.4), which atomizes its argument: a relative path of
//	                   two or more QName child steps, `data(a/b)`, its last
//	                   step typed by ElementTypes at that path and its
//	                   elements' typed values carried by the Tally after each
//	                   one's subtree (Tally.ReadsElementValue,
//	                   Tally.ElementValue), or one typed child step, its axis
//	                   abbreviated or spelled out, `data(child::d)`, filtered
//	                   by one IntegerLiteral predicate or by none,
//	                   `data(d[1])`, the k-th d child in document order or
//	                   none (§3.2.2, §3.2.4)
//	no   yes    no     `instance of` with an AttributeTest over one attribute
//	                   step of E, `@d` or `./@d`, true where the attribute
//	                   count is one the indicator admits and the attribute's
//	                   name and type annotation, the type AttributeTypes
//	                   answers — xs:anySimpleType for one invalid in the
//	                   partial PSVI — match it (§2.5.4.5)
//	no   no     no     an `instance of` whose SequenceType is a KindTest but
//	                   that AttributeTest; an AttributeTest whose TypeName is
//	                   user-defined or xs:anyType; `item()`,
//	                   `empty-sequence()` or a user-defined or non-atomic
//	                   type; PLANNED
//	no   no     no     an `instance of` over a node without fn:data but that
//	                   attribute step, or over a numeric literal or an
//	                   arithmetic result; PLANNED
//	no   no     ctx    an `instance of` over a typed child, fn:distinct-values
//	                   over one included, against an atomic type its ·locally
//	                   declared type· does not derive from, which an xsi:type
//	                   can make the child's own type derive from; so too over
//	                   a path of child steps read for its values, or in a
//	                   value predicate
//	no   no     no     fn:data anywhere but as the operand of `instance of` or
//	                   a quantifier's binding over `$value`
//
//	                   Steps, predicates and the types they read:
//	no   no     no     a wildcard NameTest on a child step but a `*` ending
//	                   a child path
//	no   no     no     a KindTest but the comment step, [59] PITest
//	                   `processing-instruction()` among them, whose NCName or
//	                   StringLiteral argument (err:XPTY0004, xpath20.md
//	                   §3.2.1.2) is not parsed; the comment step behind an
//	                   axis spelled out or in a predicate; PLANNED
//	no   no     no     a path of more than one step in any other position —
//	                   but a path to an attribute as fn:sum's one argument,
//	                   and a path of child steps as fn:data's under `instance
//	                   of` whose last step's type is simple, or has simple
//	                   content, and is not ·special·, which declines over a
//	                   mixed, element-only or empty one — or with any other
//	                   step, so `a/b = 1`, `count(a/b)`, `a/@b`, `a//b`,
//	                   `a/*/b` and `a/* = 1` decline; PLANNED
//	no   no     no     any other axis, `ancestor::` and `preceding-sibling::`
//	                   among them, in a predicate or out of one; PLANNED
//	no   no     no     a predicate or a union anywhere the rows above do not
//	                   admit one, or a predicate of any other shape: `or`,
//	                   fn:not or a wildcard over an attribute test, an
//	                   attribute atomized or beside `.`, any other node, a
//	                   function call, or a bare value such as `N[1]`, which
//	                   a numeric value would make positional; PLANNED
//	no   no     no     a value predicate over a child read under no type a
//	                   child step would read a TYPED value under, a mixed one
//	                   among them
//	no   no     ctx    an attribute with no fixed atomic type that is not
//	                   ·special·: a list, a union, an xs:NOTATION primitive
//	no   no     no     a `$value` whose {simple type definition} is
//	                   classified as such an attribute's type would be, or is
//	                   a list of a type that would be
//	no   no     ctx    a child read for its value whose ·locally declared
//	                   type· is ·absent·, ·special·, element-only or empty,
//	                   or a simple type, or simple content over one, an
//	                   attribute's would not be admitted as
//	no   no     no     a cast from a typed attribute, child or `$value`, or
//	                   by a constructor function from a typed count,
//	                   arithmetic or function result, outside the xs:string
//	                   family to a type it is not derived from: F&O §17.2
//	                   case 4's identity cast and §17.3's cast up the
//	                   hierarchy are admitted, and so is §17.1.2's cast of an
//	                   xs:date, xs:dateTime or xs:time operand to xs:string
//	                   itself, fn:string over one included
//	no   no     no     fn:string or an fn:concat argument over any of those
//	                   or over a typed count, arithmetic or function result,
//	                   or a cast of one, outside that family and those
//	                   primitives
//
// #1042 tracks the PLANNED rows and tier 1's declined casts to xs:QName and of
// literal and floating operands, and #889 the xs:float-to-xs:double promotion.
//
// # One parser, one AST
//
// One lexer and one recursive-descent parser build one AST that serves
// both static analysis (schema-time: name resolution, type references)
// and evaluation (instance-time). There is never a second, lenient
// parser (STYLE T4). Beside it stands one RECOGNIZER for the full
// xpath20.md Appendix A grammar, which builds no tree and answers only
// whether a text the parser declined is XPath 2.0 at all (err:XPST0003).
// It accepts exactly A.1's language, a superset of what the parser
// admits, and is no T4 lenient parser: it is consulted only on a
// decline, and its acceptance compiles nothing. It shares the parser's
// lexical scanners.
//
// # The fail-open contract (PRINCIPLES 20)
//
// An unsupported construct must NEVER cause a false rejection, and each
// consumer takes that in the direction its own rule allows. A CTA
// {test} the implemented subset cannot evaluate is DECLINED at compile
// time, and the caller withholds the element's ·governing type
// definition· rather than assessing it against a guess — never
// "unmatched", which would fall through to another alternative or to
// the {default type definition} and select a type the rule may not have
// selected. An assertion whose {test} falls outside the implemented
// subset is DECLINED at compile time too, and its caller records it as
// unevaluated — never satisfied and never charged. Every fail-open site
// carries a greppable marker:
//
//	// GAP(xpath): <construct>
//
// Direction matters, and the two directions are not the same rule. A
// DYNAMIC error — type mismatch, uncastable value, bad or inexpressible
// regex/flag — is a real verdict, not a decline: it makes an assertion
// definitively UNSATISFIED, and it makes a CTA {test} FALSE, which
// Structures §3.12.4 key-cta-ta-select clause 2 states outright ("the
// {test} is treated as if it had evaluated (without error) to false").
// Confusing that with a decline flips false-accepts into false-rejects
// or vice versa.
//
// # The static-error charge is the assembler's, the code is this engine's
//
// An XPath STATIC error is a third direction: ta-props-correct clause 2
// (§3.12.6) over xpath-valid clause 2 (§3.13.6.2) forbids one outright,
// so it is a Schema Component Constraint decided when the component is
// assembled and independent of any instance. CTATestStaticError proves
// it and returns the fact as an *xsderr.Error carrying the XPath error
// code as its rule; the CHARGE — the schema rule ID and the location —
// is minted by the assembler that owns the constraint (parser), which
// wraps that verdict as its cause so xsderr.RuleOf reads the XPath code
// off one errors.Unwrap and never off a scraped message.
//
// A DECLINE EXCUSES NO PROVEN STATIC ERROR. §3.12.6 clause 2's Note lets a
// processor decline only an expression "containing no static errors", so
// an {expression} outside the required subset is declined uncharged only
// where the subset parse proved no static error in it; one it proved
// before the construct it declines at is charged (#2631).
//
// The static conditions proven today are err:XPST0003 for a {test} no
// derivation of the full grammar (xpath20.md A.1) covers, asked of the
// recognizer once the subset parse found anything wrong and winning over
// every static error that parse met; and otherwise the first in
// expression order the subset parse reaches, whether or not it then
// completes a ta-Test, of err:XPST0081 for an unbound prefix and
// err:XPST0051 or err:XPST0080 for a `cast as` target that is no builtin
// atomic type, xs:untypedAtomic aside — the schema's own types included,
// since xpath-valid clause 2.2.5 puts only the built-ins in scope. The
// parse runs past a failed target on a stand-in cast to xs:string
// (ctaParser.singleType). Only CTATestStaticError reports any of them:
// CompileAssertionTest and FacetAssertions decline every one like any
// other defect. A static error the subset parse never reaches — past the
// construct it declines at, or in one it never resolves, such as
// err:XPST0008 for `$value` — is withheld with the decline, under the
// GAP(xpath) at compileCTATest (#2673).
//
// A constructor call naming such a type is withheld, never charged: it
// names a function, not a cast target. So is err:FONS0004, an unbound
// prefix in the StringLiteral a cast to xs:QName converts, under
// ctaParser.cast's marker.
//
// # Static context
//
//   - $value binds a typed value, never a bare string (PRINCIPLES 17):
//     ValueBinding carries E's ·actual value·, and the compiled tree
//     holds the {simple type definition} it is of, which every comparison and cast
//     reads — or, over a ·special· {simple type definition}, an xs:untypedAtomic
//     value (Datatypes dt-xdmrep clause 1), which XPath's own casting rules type.
//     Beside it ValueBinding carries E's string value, which `.` reads as one
//     xs:untypedAtomic whatever `$value` is: the context item is E, annotated
//     xs:anyType in the partial ·PSVI· (cvc-assertion clause 1.2).
//     It is an ASSERTION binding: ta-props-correct adds no variable, so no CTA
//     {test} sees one.
//   - xpathDefaultNamespace supplies the default ELEMENT/TYPE namespace
//     for unprefixed element steps (never attribute steps) in
//     assertions and IDC selector/field paths (PRINCIPLES 15), and for
//     an unprefixed cast TARGET (xpath20.md §3.10.2), and for the
//     unprefixed StringLiteral a cast to xs:QName converts
//     (xpath-functions.md §5.3). The CTA subset declines every element
//     step, so those two are the only things it consults the default
//     namespace for.
//   - fn:matches / fn:replace / fn:tokenize bind to regex flavor FO,
//     never the pattern-facet flavor.
//
// Numbers follow the XDM model the subset needs; comparisons over typed
// atoms delegate to value capabilities so backend values participate.
// Arithmetic has no value capability to delegate to, so it reads each
// operand's ·canonical representation·, computes, and validates the result's
// lexical against the result type — a datatype validation, as every value
// this engine builds is. Which operand types a comparison operator admits at
// ALL is xpath20.md B.2's answer rather than a capability's, generated from
// the spec into the package (tools/opmapgen) and enforced at compile time.
// Arithmetic's operand admission and result types are B.2's six numeric
// rows, transcribed in ctaTypes.arithmetic and arithmeticResult rather than
// generated: those rows' result cell is the prose rule "numeric" with two
// exceptions, which a generator would have to hard-code as well.
//
// # Dynamic context
//
// The implicit timezone is Z, the zero offset PT0S, for every evaluation of
// either façade (cvc-xpath clause 7 leaves it ·implementation-defined·);
// ctaImplicitTimezone holds it, and F&O §10.4 assumes it on whichever operand
// of a date/time comparison has no timezone of its own.
//
// The current dateTime (xpath20.md §2.1.2) is the caller's: an assertion or
// facet evaluation is handed it as a time.Time (AssertionTest.Evaluate,
// FacetAssertions), which validate reads off the clock once per
// Validator.Assess, constant for the assessment episode (cvc-xpath clause 6).
// fn:current-date is its date in its own UTC offset (F&O §16.4, §17.1.5), not
// in the implicit timezone, or in UTC where no timezoneFrag spells that offset
// (ctaCurrentDateInstant). A Type Alternative's evaluation carries none: its
// grammar calls no function that reads one.
//
// An xs:decimal quotient that does not terminate is rounded to
// ctaDecimalDivisionDigits fractional digits, the precision F&O §6.2 leaves
// ·implementation-defined·; every other xs:decimal and xs:integer result is
// exact.
package xpath
