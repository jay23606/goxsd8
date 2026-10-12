// Package xpath is the XPath 2.0 engine serving conditional type
// assignment (CTA) and assertions. Identity-constraint {selector}/{fields}
// (§3.11.6.2/3) are compiled and evaluated by icpath against the restricted
// selector/field path grammar those sections define, never through this
// engine (docs/ARCHITECTURE.md). Full XPath 2.0 is the destination; the
// engine grows outward from the XSD-required subset, tracked by its own
// conformance lane.
//
// # Growth tiers
//
//  1. The CTA restricted subset (the `test` attribute of xs:alternative) — M6.
//     SHIPPED: CompileCTATest parses Structures §3.12.6's productions [8] ta-Test
//     through [18] ta-ConstructorFunction and CTATest.Evaluate decides one against an
//     element's attributes, casts included: [15] ta-CastExpr's `cast as` tail and [18]
//     ta-ConstructorFunction are one node, evaluated through value's facet pipeline,
//     which is what xpath-functions.md §17.1.1 makes a cast. [17] ta-AttrName's
//     NameTest is xpath20.md's [36] whole, wildcards included, so `@*`, `@p:*` and
//     `@*:n` select a sequence of E's attributes and a comparison over one is §3.5.2's
//     existential. THREE shapes inside that grammar compile-time-DECLINE rather than
//     evaluating: a cast whose target's primitive is xs:QName, whose lexical mapping
//     needs a static context this engine has no value for (#888); a cast of an
//     xs:float or xs:double operand, such as `1.5e0`, to a target other than its own
//     type or an ancestor of it, or of an IntegerLiteral or DecimalLiteral to a target
//     outside the xs:string family, xs:float, xs:double, xs:decimal and, for an
//     IntegerLiteral, xs:integer's branch, such as `xs:integer(1.5)`, which F&O
//     §17.1.2, §17.1.3 and §17.1.6 define over the value and not over its canonical
//     lexical where §17.1's casting table marks the target Y or M, and §17.1 makes
//     err:XPTY0004 where it marks N (#1042); and a comparison needing xpath20.md B.1
//     rule 1.1's xs:float-to-xs:double promotion, which value exposes no widening for
//     (#889). So does a cast whose TARGET is not a builtin datatype, which is the
//     required subset's own boundary (§3.12.6 clause 4) rather than a construct of the
//     grammar. CTATestStaticError reports the XPath STATIC errors of the same grammar
//     over the same traversal, which is a different question with a different owner —
//     see below.
//  2. Assertion essentials: axes, predicates, quantified expressions, typed comparisons, the F&O function core — M6. FIRST SLICES SHIPPED: CompileAssertionTest compiles an assertion {test} written in tier 1's grammar over the element's TYPED attributes (AttributeTypes; cvc-assertion clause 1, §3.13.4.1) and TYPED element children and elements below them (ElementTypes, by their path of names below E), and AssertionTest.Evaluate decides it over their
//     typed values (TypedAttributes, ChildElements, ChildElement, Child, TypedValue) — an ·actual value·, or xs:untypedAtomic for an attribute whose type is ·special· (xpath-datamodel §3.3.1.2) and for a child whose ·locally declared type· has mixed content, xs:anyType among them, which is its string-value, its descendants' text included (§6.2.4) — which is cast as tier 1 casts an untyped one. Its grammar is tier 1's plus xpath20.md's value comparisons
//     (§3.5.1, eq/ne/lt/le/gt/ge), `$value` (cvc-assertion clause 2.3; ValueBinding), an abbreviated child-axis step with a QName NameTest (§3.2.1.1), whose unprefixed name takes the {default namespace}, a "/" or "//" opening one or an attribute step, which raises err:XPDY0050 because the instance's root is E and not a document node (§3.2), a relative path of two or more such child steps, `a/b` (§3.2), whose last step may be `*`, `a/*`, every element
//     child (§3.2.1.2), one element step, `a`, `./a` or `.//a` (§3.2.4), or a child step filtered by a conjunction of child steps, `a[b and c]`, each a one of whose children is named each conjunct, so two a splitting the names select nothing (§3.2.2), or by one `preceding::` step, unfiltered or filtered by one child step or fn:not over one, `a[preceding::a[not(b)]]`, each a with an a lacking a b child before it in E's subtree, at any depth, E never
//     among them (§3.2.1.1, cvc-assertion clause 1.3), as the whole operand of fn:exists, fn:empty or an ·effective boolean value·, whose steps are never typed and which selects any node, ·nilled· or not, of any type, at the end of that chain of names, an fn:count call (xpath-functions.md §15.4.1) over `N`, `@N`, either behind `./` or `.//`, or a rooted step, a child step `N` or `./N` in it filtered by one predicate (§3.2.2) — a conjunction of
//     attribute-existence tests, `N[@a and @b]`, or a comparison, or and, or and fn:not over comparisons, of the child's own typed value `.`, `N[. = 'x']`, whose dynamic or type error over any child makes the count raise — and a `|` or `union` of those relative operands but a value-filtered one, each node counted once (§3.3.3), or over an operand that is no path, whose items it counts (`count($value)`, a list's items), xpath20.md §3.4's binary
//     arithmetic (`+`, `-`, `*`, `div`, `idiv`, `mod`) over numeric operands, whose dynamic errors err:FOAR0001 and err:FOAR0002 are charged like any other, F&O's string and sequence core — fn:contains, fn:starts-with and fn:ends-with under the codepoint collation, fn:string-length, fn:normalize-space, fn:string, fn:concat (§7.4.1, each argument cast to xs:string), fn:empty, fn:exists, fn:distinct-values (§15.1.6, by eq, one NaN surviving), fn:true and
//     fn:false — fn:sum over a path through child steps to an attribute below E, `sum(a/b/@p)` (§15.4.5), its attribute typed by AttributeTypes at that path, the static type of the element it ends at, and its values carried by the Tally (Tally.ReadsAttribute, Tally.Attribute), the xs:integer 0 over none, exact over xs:decimal items, and its `instance of` read off the type of the value it returns, and fn:current-date (§16.4), the date of the current
//     dateTime the caller hands Evaluate and FacetAssertions, `()` admitted as an argument, whose xs:string? arguments raise err:XPTY0004 for an item of another type or for two or more items, a constructor function over any operand such an argument may be (xpath20.md §3.1.5), `xs:date(concat(string($value), '!!!'))`, fn:namespace-uri over `.` or with no argument (§14.3), E's [namespace name] as one xs:anyURI, and fn:in-scope-prefixes over `.` (§11.2.6)
//     as one operand of `=` against a StringLiteral or a parenthesized sequence of them, true where one is a prefix of E's [in-scope namespaces], both reading `.` as E's node through the ContextElement Evaluate is handed, which validate.Element satisfies, and never its string value, [47] ContextItemExpr `.`, atomized to one xs:untypedAtomic holding E's string value (cvc-assertion clause 2.3.1's Note) — never `$value` — which ValueBinding carries beside
//     `$value` and which the zero-argument fn:string, fn:string-length and fn:normalize-space read, AssertionTest.ReadsContextItem reporting whether a compiled {test} reads it, so its caller gathers E's string value for no other, as a general comparison's operand, an integer sequence: [11] RangeExpr over IntegerLiterals, bare or parenthesized, or a parenthesized comma sequence of IntegerLiterals and such ranges, `(1 to 10, 20, 30)` (§3.3.1), or a
//     string sequence, a parenthesized comma sequence of StringLiterals, `('a', 'b')`, whose items are xs:string, and §3.8's `if (Expr) then ExprSingle else ExprSingle` wherever an ExprSingle stands whole in a boolean position, evaluating only the branch its test's ·effective boolean value· selects, and there §3.9's `some` and `every` over one in-clause binding a range variable to each child a QName step `N` or `./N` selects, whose body is `$v/@A`, a
//     conjunction of such, or `$v/following-sibling::*[1][self::M]`, the next sibling element where it is named M, or one of those under fn:not, decided off Tally counts with no variable bound at evaluation, or binding a range variable to each child a QName step `N` or `./N` of a type a value step reads TYPED selects, or to each but the last under `N[position() lt last()]` (§3.2.2), whose body is any ExprSingle of this grammar, the range variable a
//     node whose typed value is the step's and `$v/following-sibling::M[1]` the first later child named M however far (§3.2.1.1), decided per child off ChildElements, or binding a range variable to each item of a typed `$value`, `data($value)` or `$value` (xpath-functions.md §2.4), whose body is any ExprSingle of this grammar, the range variable an atomic value of `$value`'s item type in scope in the body alone, decided per item with a decisive item
//     winning over an error, `every` over no child or item true and `some` false, and §3.10.3's `castable as` over a [16] ta-SimpleValue, true exactly where the same `cast as` yields a value and false where that cast raises, an error evaluating its operand raising, and §3.10.1's `instance of` with a builtin atomic SequenceType and an optional occurrence indicator over an operand that is no node or over an fn:data call (xpath-functions.md §2.4), which
//     atomizes its argument — a relative path of two or more QName child steps among them, `data(a/b)`, its last step typed by ElementTypes at that path and its elements' typed values carried by the Tally after each one's subtree (Tally.ReadsElementValue, Tally.ElementValue) — true where the item count is one the indicator admits and each item's type derives from it (§2.5.4), never casting — all of which a Type Alternative's {test} still declines, each
//     outside §3.12.6's subset. AssertionTest.ReadsChild reports which children a compiled {test} reads for their values — a value predicate's children among them, counted over ChildElements, and a quantifier's bound children and the M its body's `following-sibling::M[1]` reads, both in document order — so its caller keeps no other; neither any other fn:count argument nor a path whose existence is asked reads a value, and AssertionTest.Tally makes the
//     Tally its caller reports E's subtree to — each element by its chain of names below E, with the names of its attribute nodes (Tally.Element), in pre-order document order, a report out of that order making Evaluate false, Tally.ReadsAttributesAt telling it at which depths those names can change a count or a value read — and Evaluate reads the counts off, so no counted node is kept. It declines what tier 1 declines, plus a wildcard NameTest but a
//     `*` ending such a child path, an attribute with no fixed atomic type that is not ·special· (a list, a union, an xs:QName or xs:NOTATION primitive), a child read for its value whose ·locally declared type· is ·absent·, ·special·, element-only or empty, or a simple type, or simple content over one, an attribute's would not be admitted as, a path of more than one step in any other position — but a path to an attribute as fn:sum's one argument, and a
//     path of child steps as fn:data's under `instance of` whose last step's type is simple, or has simple content, and is not ·special·, which declines over a mixed, element-only or empty one — or with any other step — so `a/b = 1`, `count(a/b)`, `a/@b`, `a//b`, `a/*/b` and `a/* = 1` decline — an fn:count argument of any other shape, a predicate or a union anywhere else but that child-existence conjunction and that `preceding::` step, a predicate of
//     any other shape — any other axis, `ancestor::` and `preceding-sibling::` among them, `or`, fn:not or a wildcard over an attribute test, an attribute atomized or beside `.`, any other node, a function call, or a bare value such as `N[1]`, which a numeric value would make positional — and a value predicate over a child read under no type a child step would read a TYPED value under, a mixed one among them, a cast from a typed attribute, child or
//     `$value`, or by a constructor function from a typed count, arithmetic or function result, outside the xs:string family to a type it is not derived from (F&O §17.2 case 4's identity cast and §17.3's cast up the hierarchy are admitted, and so is §17.1.2's cast of an xs:date, xs:dateTime or xs:time operand to xs:string itself, fn:string over one included), fn:string or an fn:concat argument over any of those or over a typed count, arithmetic or
//     function result, or a cast of one, outside that family and those primitives, a cast of an xs:float or xs:double operand to a target other than its own type or an ancestor of it, fn:string over such an operand, a collation argument, a function call of the wrong arity, `.` outside a value predicate under a nil content or where it is a node — the whole operand of an ·effective boolean value·, fn:not, fn:exists, fn:empty or fn:count — but as the
//     argument of those two functions, the zero-argument string functions wherever `.` declines, fn:namespace-uri over any other argument, fn:in-scope-prefixes over any other argument or in any other position — free-standing, under fn:count, beside `!=` or `eq` — an integer or string sequence in any other position or over any other operand — a member of another kind, a mix of StringLiterals with the other two, a nested or empty parenthesis, a misplaced
//     comma — or longer than ctaMaxSequenceLength, an arithmetic operand that is not numeric, an xs:float one against xs:double (#889), a unary sign, a `castable as` whose cast declines, over any operand but a SimpleValue or in a value predicate, an `instance of` whose SequenceType is a KindTest, `item()`, `empty-sequence()` or a user-defined or non-atomic type, over a node without fn:data, over a numeric literal or an arithmetic result, over a typed
//     child — fn:distinct-values over one included — against an atomic type its ·locally declared type· does not derive from, which an xsi:type can make the child's own type derive from, so too over a path of child steps read for its values, or in a value predicate, fn:data anywhere but as the operand of `instance of` or a quantifier's binding over `$value`, a quantifier over a statically empty or ·special· `$value`, a nested quantifier, a path or
//     `instance of` over a range variable over `$value`, a quantifier over a child step whose child is of mixed content or of a type a value step would not read, whose binding step carries any other predicate, `<`, `ne` or `[last()]` among them, or whose window is followed by a body decided off the Tally, a path over its range variable but `following-sibling::M[1]`, and a `$value` whose {simple type definition} is classified as such an attribute's type
//     would be, or is a list of a type that would be; a `$value` over ·special· content is xs:untypedAtomic, as such an attribute is. FacetAssertions is the value.AssertionEvaluator for an assertions facet's {test} (Datatypes §4.3.13.3, cvc-assertions-valid), over the same grammar: `$value` is bound to the value under the facet's type, or under its ·active basic member· where that type is a union (dt-xdmrep clause 4), and there is no context item, so
//     `.`, an attribute or child step, an element step, a child path, a filtered child step, a rooted path, a zero-argument string function and fn:namespace-uri and fn:in-scope-prefixes over `.` each raise err:XPDY0002 and fail the facet, and so do `castable as` and fn:data under `instance of` over one of them, and fn:position and fn:last, there being no context position or size either (clause 1.3), which every other façade declines; an fn:count call
//     over a path declines, and so does a quantified expression. A Type Alternative's {test} declines `.` everywhere. Longer paths in other positions, the other axes, predicates in other positions or of other shapes, positional ones among them, unions outside fn:count, quantified expressions beyond that one in-clause over a child step with those body forms and that one predicate or over a typed `$value`, a conditional read as an item rather than for
//     its ·effective boolean value·, `castable as` over other operands, `instance of` beyond an atomic SequenceType over those operands, the collation arguments, fn:in-scope-prefixes in other positions, which needs a listing of E's [in-scope namespaces] that validate.Element lacks, and every other F&O function are PLANNED (#1042).
//  3. The full grammar (docs/specs/md/xpath20.md) and function library
//     (docs/specs/md/xpath-functions.md) — M7 onward, ratcheted.
//     PLANNED.
//
// # One parser, one AST
//
// One lexer and one recursive-descent parser build one AST that serves
// both static analysis (schema-time: syntax errors, type references)
// and evaluation (instance-time). There is never a second, lenient
// parser (STYLE T4).
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
// UNSUPPORTED DOMINATES STATIC. An {expression} outside the required
// subset is declined and never charged, whatever its names resolve to,
// because §3.12.6 clause 2's Note lets a processor decline it and never
// lets one refuse the schema for it. Under-charging is a rejection this
// engine can take later; over-charging is a false reject now.
//
// One static condition is proven today, err:XPST0081 for an unbound
// prefix. The cast-target conditions — err:XPST0051 and err:XPST0080 —
// are not, because proving one takes a parse that runs PAST the failed
// target to the end of a ta-Test and no target stands in for one that
// resolved to nothing; they stay folded into the compile-time withhold
// under ctaTypes.castTarget's marker.
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
//     an unprefixed cast TARGET (xpath20.md §3.10.2). The CTA subset
//     declines every element step, so a cast target is the only thing it
//     consults the default namespace for.
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
