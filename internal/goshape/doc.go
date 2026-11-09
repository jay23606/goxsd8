// Package goshape holds the one code answering the decisions that map an XSD
// component to the shape of the Go declaration codegen emits for it, for
// every library package that must reach the same answer: codegen, which
// renders its declarations from it, and codec, which binds and checks a
// caller's T against it.
//
// # Contract
//
//	func SubstitutionSum(schema *xsd.Schema, decl xsd.ElementDeclaration) (members []xsd.ElementDeclaration, sum bool)
//	func BasicMembers(r xsd.TypeResolver, st *xsd.SimpleType) ([]*xsd.SimpleType, error)
//
// The rules are those codegen's package doc states, each in its own section
// ("# Simple types", "# Substitution groups"): those sections own them, and
// this package is their one implementation.
//
// It imports xsd and the standard library.
package goshape
