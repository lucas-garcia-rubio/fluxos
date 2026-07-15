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
	want := []string{"before", "consume", "argumentCall", "after"}
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
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("local vars = %v, want %v", got, want)
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
