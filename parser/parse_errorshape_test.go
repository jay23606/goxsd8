package parser_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kud360/goxsd8/builtin"
	"github.com/kud360/goxsd8/loader"
	"github.com/kud360/goxsd8/parser"
	"github.com/kud360/goxsd8/value"
	"github.com/kud360/goxsd8/xsd"
	"github.com/kud360/goxsd8/xsderr"
)

// noMappings is a value.Backend that maps no datatype at all, so builtin.Seed
// finds every primitive missing.
type noMappings struct{}

func (noMappings) Mapping(xsd.QName) (value.Mapping, bool) { return value.Mapping{}, false }

// faultingResolver resolves through docs, except that a location in
// resolveErr fails to resolve with that error and a location in readErr
// resolves to a reader whose first Read fails with that error.
func faultingResolver(docs map[string]string, resolveErr, readErr map[string]error) loader.Resolver {
	inner := loader.Map(docs)
	return loader.ResolverFunc(func(namespace, location string) (io.ReadCloser, string, error) {
		if err, ok := resolveErr[location]; ok {
			return nil, "", err
		}
		if err, ok := readErr[location]; ok {
			return io.NopCloser(&failFirstReader{err: err}), location, nil
		}
		return inner.Resolve(namespace, location)
	})
}

// TestParseReportErrorShapes pins the shapes ParseReport's doc assigns to each
// error class, by the tests a caller runs: the Rule xsderr.RuleOf reports (or
// that it reports none), the cause errors.Is reaches, and whether errors.As
// reaches a *builtin.MissingPrimitivesError. Each row fails when its producing
// site hands its class back in another shape: an *xsderr.Error where the doc
// says plain, a plain error or another Rule where it says *Error, or a chain
// that drops the cause.
func TestParseReportErrorShapes(t *testing.T) {
	cause := errors.New("boom: the resolver's own cause")
	include := map[string]string{
		"main.xsd": wrap("urn:a", `<xs:include schemaLocation="lib.xsd"/>`),
	}
	cases := []struct {
		name        string
		resolver    loader.Resolver
		opts        []parser.Option
		wantRule    xsderr.Rule // "" means RuleOf must report no *Error
		wantCause   bool
		wantMissing bool
		wantPrefix  string // the opening of the message, naming its class
	}{
		{
			name:      "root reader fails",
			resolver:  faultingResolver(nil, nil, map[string]error{"main.xsd": cause}),
			wantRule:  xsderr.RuleXMLWellFormed,
			wantCause: true,
		},
		{
			name: "root not well-formed",
			resolver: loader.Map(map[string]string{
				"main.xsd": `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">`, // unclosed
			}),
			wantRule: xsderr.RuleXMLWellFormed,
		},
		{
			name:      "included document's reader fails",
			resolver:  faultingResolver(include, nil, map[string]error{"lib.xsd": cause}),
			wantRule:  "src-include",
			wantCause: true,
		},
		{
			name:       "included location's resolver fails other than ErrNotFound",
			resolver:   faultingResolver(include, map[string]error{"lib.xsd": cause}, nil),
			wantCause:  true,
			wantPrefix: "parser: resolving <include> schemaLocation ",
		},
		{
			name: "derivation this parser cannot yet map",
			resolver: loader.Map(map[string]string{
				"main.xsd": wrap("urn:a", `<xs:complexType name="T"><xs:sequence>`+
					`<xs:element name="inner" minOccurs="0"><xs:complexType>`+
					`<xs:complexContent><xs:extension base="tns:T"/></xs:complexContent>`+
					`</xs:complexType></xs:element></xs:sequence></xs:complexType>`),
			}),
			wantPrefix: "parser: the <extension> at ",
		},
		{
			name:        "backend maps no primitive",
			resolver:    loader.Map(map[string]string{"main.xsd": wrap("urn:a", "")}),
			opts:        []parser.Option{parser.WithBackend(noMappings{})},
			wantMissing: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := append([]parser.Option{parser.WithResolver(tc.resolver)}, tc.opts...)
			_, _, err := parser.ParseReport("main.xsd", opts...)
			if err == nil {
				t.Fatal("ParseReport succeeded, want an error")
			}
			rule, ok := xsderr.RuleOf(err)
			if tc.wantRule == "" && ok {
				t.Errorf("RuleOf = %q, want no *xsderr.Error in the chain of %v", rule, err)
			}
			if tc.wantRule != "" && rule != tc.wantRule {
				t.Errorf("RuleOf = %q (found %t), want %q, for %v", rule, ok, tc.wantRule, err)
			}
			if got := errors.Is(err, cause); got != tc.wantCause {
				t.Errorf("errors.Is(err, cause) = %t, want %t, for %v", got, tc.wantCause, err)
			}
			var missing *builtin.MissingPrimitivesError
			if got := errors.As(err, &missing); got != tc.wantMissing {
				t.Errorf("errors.As(err, *builtin.MissingPrimitivesError) = %t, want %t, for %v", got, tc.wantMissing, err)
			}
			if !strings.HasPrefix(err.Error(), tc.wantPrefix) {
				t.Errorf("error = %q, want it to open with %q", err, tc.wantPrefix)
			}
		})
	}
}
