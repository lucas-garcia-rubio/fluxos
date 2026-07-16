package java

import (
	"reflect"
	"testing"
)

func TestExtractCallsStopsAtNestedExecutables(t *testing.T) {
	types := extractJavaSource(t, `
class Example {
    void outer() {
        before();
        Runnable lambda = () -> insideLambda();
        class Local {
            void run() { insideLocalClass(); }
        }
        Runnable anonymous = new Runnable() {
            public void run() { insideAnonymous(); }
        };
        consume(argumentCall(), new Runnable() {
            public void run() { nestedArgumentBody(); }
        });
        after();
    }
}
`)

	method := findTypeBySimpleName(t, types, "Example").Methods[0]
	var got []string
	for _, call := range method.Calls {
		got = append(got, call.MethodName)
	}
	want := []string{"before", "<init>", "consume", "argumentCall", "<init>", "after"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
}

func TestExtractLocalVarsStopsAtNestedExecutables(t *testing.T) {
	types := extractJavaSource(t, `
class Example {
    void outer() {
        String outer;
        Runnable lambda = () -> {
            String insideLambda;
        };
        class Local {
            void run() { String insideLocal; }
        }
        Runnable anonymous = new Runnable() {
            public void run() { String insideAnonymous; }
        };
    }
}
`)

	got := findTypeBySimpleName(t, types, "Example").Methods[0].LocalVars
	want := map[string]string{
		"outer":     "String",
		"lambda":    "Runnable",
		"anonymous": "Runnable",
	}
	if len(got) != len(want) {
		t.Fatalf("local vars count = %d, want %d: %+v", len(got), len(want), got)
	}
	for _, decl := range got {
		wantType, ok := want[decl.Name]
		if !ok {
			t.Errorf("unexpected local var %q", decl.Name)
			continue
		}
		if decl.Type.Raw != wantType {
			t.Errorf("local var %q type = %q, want %q", decl.Name, decl.Type.Raw, wantType)
		}
	}
	// Confirma boundary: lambda/local class/anonymous não contaminam o método externo.
	for _, decl := range got {
		if decl.Name == "insideLambda" || decl.Name == "insideLocal" || decl.Name == "insideAnonymous" {
			t.Errorf("nested executable var leaked into outer method: %q", decl.Name)
		}
	}
}

func TestExtractCallKindAndArgCount(t *testing.T) {
	types := extractJavaSource(t, `class Example { void run() { target(first, second); } }`)
	call := findTypeBySimpleName(t, types, "Example").Methods[0].Calls[0]
	if call.Kind != CallInvocation {
		t.Fatalf("call kind = %s, want invocation", call.Kind)
	}
	if call.ArgCount != 2 || len(call.Args) != 2 {
		t.Fatalf("arg count = %d and args = %v, want 2", call.ArgCount, call.Args)
	}
}
