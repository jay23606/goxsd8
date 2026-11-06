package xsderr

// catalog.go is regenerated from the local specs by tools/rulecat (`go tool
// rulecat`). It emits ruleCatalog, the Rule lookup set behind IsValidRule,
// covering every validation rule ID that appears in docs/specs/md. IsValidRule
// itself is hand-written in error.go, not generated; its doc says why and how
// exact membership is. rulecat locates the module root itself, so this
// directive needs no path arguments. Run `go generate ./...` after editing the
// specs or rulecat.
//
// Rule is a bare string type, so nothing in the Go type system stops an
// arbitrary one; the guarantee is enforced by test instead.
// rulecatalog_enforcement_test.go scans the whole module for two things: every
// constant declared with type Rule anywhere in the module must satisfy
// IsValidRule, and in non-test code no string literal may sit in one of four
// syntactic Rule positions — the first argument of New or Wrap, the Rule field
// of an Error composite literal, a Rule-typed result of a return statement, or
// the operand of a Rule(...) conversion. The return position alone admits "",
// the zero Rule; it is a violation in the other three. So a rule the spec
// doesn't define is caught wherever it is written as a Rule constant or as a
// literal in those four positions. That is a syntactic floor, not a dataflow
// proof: a Rule that reaches New/Wrap through a variable or another expression
// is not scanned, and the test's header comment names the residual holes it
// deliberately leaves open.

//go:generate go tool rulecat
