// Package jsonsrc maps JSON documents onto the validate infoset so JSON
// instances can be assessed against a compiled schema set.
//
// The engine never imports a JSON package; this adapter builds infoset
// values over the stdlib streaming json.Decoder token walk — member order
// preserved, the document never held whole (STYLE P4) — and hands them to
// [validate.Validator.Assess] through [Validate]. Verdicts are the engine's,
// with the same cvc-* rule IDs as for the XML document the JSON maps to; the
// json conformance lane pins the mapping with curated cases
// (conformance/testdata/json).
//
// # The validation root
//
// The top-level value is an object with exactly one member. Its key names the
// validation root: it is matched by local name against the top-level element
// declarations ([validate.SchemaView]'s Elements), exactly one of which must
// carry it, and that declaration's expanded name is the root's. Its value is
// the root's, as any member's value is its element's. A top level of any other
// shape, a key no top-level declaration or several carry, and an array or null
// root value decline: there is no no-namespace root to fall back to.
//
// # The mapping
//
// The arms implemented, at every element below the root:
//
//   - A member whose key is the local name of a child element declaration
//     the enclosing element's type contains — directly, indirectly or
//     implicitly, [xsd.Schema.ContainedElementDeclarations] — is that child,
//     under that declaration's expanded name. An object value carries the
//     child's own children; members map in member order.
//   - A scalar value is the element's simple content: {"size":10} ≡
//     <size>10</size>. Its lexical form is the token's source text — a
//     number's own digits, never a float64 round trip; a string's decoded
//     characters; true or false — and the empty string is no text at all,
//     as in <size/>.
//   - An array value is one child per item under the member's key, in item
//     order: an object or scalar item is a child as a member value would be,
//     and an empty array is no child.
//
// The element's default (empty-prefix) namespace is its declaration's target
// namespace, and "xml" is bound; no other prefix is.
//
// Two shapes are not expressible, and the mapping does not reorder to reach
// them. The ORDER TRAP: an array's items are contiguous repeats, so a content
// model interleaving repeats (a, b, a) is reached only through duplicate keys,
// which map in member order like any other members. The NAMESPACE TRAP: a key
// is a local name, while cvc-accept matches expanded names; the namespace
// comes from the matched declaration, so a key two expanded names in one
// content model carry declines. A member is classified under its matched
// declaration's type, which need not be the child's ·governing type
// definition·: where the engine attributes the child to a wildcard instead,
// that type is the engine's to determine — through the top-level declaration
// the child's name resolves to, for one — and it assesses the infoset built
// under the matched declaration.
//
// # Declines
//
// An input the implemented arms do not map is an error from [Validate]
// naming the arm it is not — never a silent drop and never a guessed mapping:
//
//   - a key no contained declaration carries, which could be an attribute or
//     a wildcard's element; a key a wildcard would admit declines with it;
//   - a "$"-prefixed reserved member;
//   - a null, which would be xsi:nil — an empty element would be a mis-map
//     under cvc-elt clause 5.1;
//   - an array item that is itself an array;
//   - a matched declaration carrying a {type table}, whose governing type
//     turns on attributes this mapping does not present;
//   - a {type definition} that does not resolve, under an object value.
//
// # Locations
//
// Every Loc is a line and a byte column from 1, at the member's key for an
// element (at the item for an array's item) and at the scalar token for its
// text.
//
// # Planned arms (later M8 slices — not yet implemented)
//
//   - Object members match a named attribute use as well as a child element
//     declaration; a name declared as both → element wins + a non-fatal
//     warning on the Result.
//   - JSON null ⇒ xsi:nil="true" (nillability itself stays the engine's
//     cvc-elt concern).
//   - Reserved members: "$type" ⇒ xsi:type; "$xmlns" ⇒ {prefix: uri}
//     bindings for QName-valued content, inherited by descendants through an
//     immutable parent-linked scope chain; the element's default
//     (empty-prefix) namespace stays its matched declaration's target
//     namespace and deliberately shadows any "$xmlns" override of "".
package jsonsrc
