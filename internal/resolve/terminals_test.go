package resolve

import (
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

func TestTerminalHandleIsDeterministicPerCallSite(t *testing.T) {
	call := java.CallSite{File: "App.java", Line: 10, StartByte: 100}
	h1 := TerminalHandle("contract.Service", "run", "()", ResolutionNoImplementation, call)
	h2 := TerminalHandle("contract.Service", "run", "()", ResolutionNoImplementation, call)
	if h1 != h2 {
		t.Fatalf("terminal handle not deterministic: %v vs %v", h1, h2)
	}
}

func TestTerminalHandleDistinguishesKindsOnSameCallSite(t *testing.T) {
	call := java.CallSite{File: "App.java", Line: 10, StartByte: 100}
	noImpl := TerminalHandle("contract.Svc", "run", "()", ResolutionNoImplementation, call)
	ambImpl := TerminalHandle("contract.Svc", "run", "()", ResolutionAmbiguousImplementation, call)
	if noImpl == ambImpl {
		t.Fatal("terminal handles should differ across kinds on the same call site")
	}
	if !strings.Contains(noImpl.TypeFQCN, "#noimpl#") {
		t.Fatalf("expected #noimpl# suffix, got %q", noImpl.TypeFQCN)
	}
	if !strings.Contains(ambImpl.TypeFQCN, "#ambimpl#") {
		t.Fatalf("expected #ambimpl# suffix, got %q", ambImpl.TypeFQCN)
	}
}

func TestTerminalHandleDistinguishesCallSitesOnSameMethod(t *testing.T) {
	call1 := java.CallSite{File: "App.java", Line: 10, StartByte: 100}
	call2 := java.CallSite{File: "App.java", Line: 20, StartByte: 200}
	h1 := TerminalHandle("contract.Svc", "run", "()", ResolutionNoImplementation, call1)
	h2 := TerminalHandle("contract.Svc", "run", "()", ResolutionNoImplementation, call2)
	if h1 == h2 {
		t.Fatal("terminal handles should differ across call sites")
	}
	// Method and Signature must remain intact so labels show what was called.
	if h1.Method != "run" || h1.Signature != "()" {
		t.Fatalf("terminal handle lost method/signature: %+v", h1)
	}
}

func TestTerminalHandlePrefixMatchesKindToken(t *testing.T) {
	call := java.CallSite{File: "App.java", Line: 1}
	cases := []struct {
		kind  ResolutionKind
		token string
	}{
		{ResolutionUnresolved, "unresolved"},
		{ResolutionNoImplementation, "noimpl"},
		{ResolutionAmbiguousType, "ambtype"},
		{ResolutionAmbiguousOverload, "ambover"},
		{ResolutionAmbiguousImplementation, "ambimpl"},
	}
	for _, tt := range cases {
		t.Run(tt.token, func(t *testing.T) {
			h := TerminalHandle("contract.Svc", "run", "()", tt.kind, call)
			expected := "contract.Svc#" + tt.token + "#"
			if !strings.HasPrefix(h.TypeFQCN, expected) {
				t.Fatalf("TypeFQCN %q should start with %q", h.TypeFQCN, expected)
			}
		})
	}
}

func TestTerminalTargetCopiesFields(t *testing.T) {
	call := java.CallSite{File: "App.java", Line: 10}
	target := TerminalTarget(ResolutionAmbiguousImplementation, "contract.Svc", "run", "()", call,
		"note", []string{"a.A", "b.B"})
	if target.Descend {
		t.Fatal("terminal target should not descend")
	}
	if target.Kind != ResolutionAmbiguousImplementation {
		t.Fatalf("kind = %v, want AmbiguousImplementation", target.Kind)
	}
	if target.Note != "note" {
		t.Fatalf("note = %q", target.Note)
	}
	if len(target.Candidates) != 2 || target.Candidates[0] != "a.A" {
		t.Fatalf("candidates = %+v", target.Candidates)
	}
	if target.Handle.Method != "run" {
		t.Fatalf("handle method lost: %+v", target.Handle)
	}
}
