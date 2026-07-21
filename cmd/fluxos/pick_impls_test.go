package main

import (
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

func TestParsePickImplsEmptyReturnsNil(t *testing.T) {
	got, err := parsePickImpls("")
	if err != nil {
		t.Fatalf("parsePickImpls(\"\"): %v", err)
	}
	if got != nil {
		t.Fatalf("got = %+v, want nil", got)
	}
}

func TestParsePickImplsAcceptsNone(t *testing.T) {
	got, err := parsePickImpls("contracts.A=none")
	if err != nil {
		t.Fatalf("parsePickImpls: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got = %+v, want 1 mapping", got)
	}
	choice, ok := got["contracts.A"]
	if !ok || choice.Kind != resolve.FixedChoiceNone {
		t.Fatalf("choice = %+v, want None", choice)
	}
}

func TestParsePickImplsAcceptsAll(t *testing.T) {
	got, err := parsePickImpls("contracts.A=all")
	if err != nil {
		t.Fatalf("parsePickImpls: %v", err)
	}
	if got["contracts.A"].Kind != resolve.FixedChoiceAll {
		t.Fatalf("choice = %+v, want All", got["contracts.A"])
	}
}

func TestParsePickImplsAcceptsExplicitSingle(t *testing.T) {
	got, err := parsePickImpls("contracts.A=app.Impl")
	if err != nil {
		t.Fatalf("parsePickImpls: %v", err)
	}
	choice := got["contracts.A"]
	if choice.Kind != resolve.FixedChoiceExplicit || len(choice.Impls) != 1 || choice.Impls[0] != "app.Impl" {
		t.Fatalf("choice = %+v, want Explicit [app.Impl]", choice)
	}
}

func TestParsePickImplsAcceptsMultipleMappings(t *testing.T) {
	got, err := parsePickImpls("contracts.A=app.Impl,contracts.B=all")
	if err != nil {
		t.Fatalf("parsePickImpls: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got = %+v, want 2 mappings", got)
	}
	if got["contracts.A"].Kind != resolve.FixedChoiceExplicit {
		t.Fatalf("A kind = %v, want Explicit", got["contracts.A"].Kind)
	}
	if got["contracts.B"].Kind != resolve.FixedChoiceAll {
		t.Fatalf("B kind = %v, want All", got["contracts.B"].Kind)
	}
}

func TestParsePickImplsRejectsMissingEquals(t *testing.T) {
	_, err := parsePickImpls("contracts.A")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "expected <receiverFQCN>=<choice>") {
		t.Fatalf("err = %v", err)
	}
	requireUsageError(t, err)
}

func TestParsePickImplsRejectsEmptyReceiverOrChoice(t *testing.T) {
	for _, raw := range []string{"=app.Impl", "contracts.A=", ",contracts.A=all"} {
		_, err := parsePickImpls(raw)
		if err == nil {
			t.Fatalf("parsePickImpls(%q): expected error", raw)
		}
		requireUsageError(t, err)
	}
}

func TestParsePickImplsRejectsDuplicateReceiver(t *testing.T) {
	_, err := parsePickImpls("contracts.A=app.Impl,contracts.A=all")
	if err == nil || !strings.Contains(err.Error(), "duplicate receiver") {
		t.Fatalf("err = %v, want duplicate receiver", err)
	}
}

func TestParsePickImplsRejectsInvalidOrInternallySpacedFQCN(t *testing.T) {
	for _, raw := range []string{
		"contracts..A=app.Impl",
		"contracts .A=app.Impl",
		"contracts.A=app .Impl",
		"contracts.A=app.Impl-extra",
	} {
		_, err := parsePickImpls(raw)
		if err == nil || !strings.Contains(err.Error(), "FQCN") {
			t.Fatalf("parsePickImpls(%q) error = %v, want invalid FQCN", raw, err)
		}
		requireUsageError(t, err)
	}
}

func TestParsePickImplsTrimsWhitespace(t *testing.T) {
	got, err := parsePickImpls(" contracts.A = app.Impl , contracts.B = all ")
	if err != nil {
		t.Fatalf("parsePickImpls: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got = %+v", got)
	}
	if got["contracts.A"].Impls[0] != "app.Impl" {
		t.Fatalf("trims not applied: %+v", got["contracts.A"])
	}
}
