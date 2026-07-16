package java

import (
	"reflect"
	"testing"
)

func TestExtractCompactConstructorUsesCopiedRecordComponents(t *testing.T) {
	types := extractJavaSource(t, `
record Point(int x, java.util.List<String> labels) {
    public Point { validate(x); }
}
`)

	typ := findTypeBySimpleName(t, types, "Point")
	wantParams := []Param{
		{Name: "x", Type: NewTypeRef("int", false)},
		{Name: "labels", Type: NewTypeRef("java.util.List<String>", false)},
	}
	if !reflect.DeepEqual(typ.RecordComponents, wantParams) {
		t.Fatalf("record components = %+v, want %+v", typ.RecordComponents, wantParams)
	}
	if len(typ.Methods) != 1 {
		t.Fatalf("methods = %+v, want one compact constructor", typ.Methods)
	}
	constructor := &typ.Methods[0]
	if constructor.Kind != MethodCompactConstructor || constructor.Name != "<init>" {
		t.Fatalf("constructor identity = {%s %q}, want compactConstructor <init>", constructor.Kind, constructor.Name)
	}
	if constructor.Signature != "(int,java.util.List)" || !reflect.DeepEqual(constructor.Params, wantParams) {
		t.Fatalf("constructor params/signature = %+v %q", constructor.Params, constructor.Signature)
	}
	if constructor.Synthetic {
		t.Fatal("source compact constructor must not be synthetic")
	}
	if len(constructor.Calls) != 1 || constructor.Calls[0].MethodName != "validate" {
		t.Fatalf("compact constructor calls = %+v, want validate", constructor.Calls)
	}

	constructor.Params[0].Name = "changed"
	if typ.RecordComponents[0].Name != "x" {
		t.Fatal("compact constructor params share their backing array with record components")
	}
}

func TestExtractObjectCreationCallsPreservesPreorderArgsAndAnonymousBoundary(t *testing.T) {
	tree, source := parseJavaSource(t, `
class Foo { Foo(Object first, Object second) {} }
class Bar { Bar() {} }
class Example {
    void create() {
        new Foo(first(), new Bar());
        new Runnable(argumentCall()) {
            public void run() { hidden(); }
        };
    }
}
`)
	types, err := Extract("Constructors.java", source, tree)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	calls := findTypeBySimpleName(t, types, "Example").Methods[0].Calls
	if len(calls) != 5 {
		t.Fatalf("calls = %+v, want five calls", calls)
	}
	wantKinds := []CallKind{CallObjectCreation, CallInvocation, CallObjectCreation, CallObjectCreation, CallInvocation}
	wantNames := []string{"<init>", "first", "<init>", "<init>", "argumentCall"}
	for i := range calls {
		if calls[i].Kind != wantKinds[i] || calls[i].MethodName != wantNames[i] {
			t.Errorf("call %d = {%s %q}, want {%s %q}", i, calls[i].Kind, calls[i].MethodName, wantKinds[i], wantNames[i])
		}
		if calls[i].File != "Constructors.java" || calls[i].Line == 0 || calls[i].StartByte >= calls[i].EndByte {
			t.Errorf("call %d has incomplete location: %+v", i, calls[i])
		}
	}
	outer := calls[0]
	if outer.TargetType == nil || outer.TargetType.Raw != "Foo" || outer.ArgCount != 2 || !reflect.DeepEqual(outer.Args, []string{"first()", "new Bar()"}) {
		t.Fatalf("outer creation = %+v", outer)
	}
	if got := string(source[outer.StartByte:outer.EndByte]); got != "new Foo(first(), new Bar())" {
		t.Fatalf("outer creation range = %q", got)
	}
	if calls[2].TargetType == nil || calls[2].TargetType.Raw != "Bar" {
		t.Fatalf("nested creation target = %+v, want Bar", calls[2].TargetType)
	}
	anonymous := calls[3]
	if !anonymous.Anonymous || anonymous.TargetType == nil || anonymous.TargetType.Raw != "Runnable" || !reflect.DeepEqual(anonymous.Args, []string{"argumentCall()"}) {
		t.Fatalf("anonymous creation = %+v", anonymous)
	}
	for _, call := range calls {
		if call.MethodName == "hidden" {
			t.Fatal("anonymous class body call leaked into enclosing method")
		}
	}
}

func TestExtractExplicitConstructorInvocations(t *testing.T) {
	tree, source := parseJavaSource(t, `
class Base { Base(int value) {} }
class Example extends Base {
    Example() { this(makeValue()); }
    Example(int value) { super(value); }
}
`)
	types, err := Extract("Example.java", source, tree)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	methods := findTypeBySimpleName(t, types, "Example").Methods
	first := methods[0].Calls
	if len(first) != 2 || first[0].Kind != CallThisConstructor || first[0].MethodName != "<init>" || !reflect.DeepEqual(first[0].Args, []string{"makeValue()"}) || first[1].MethodName != "makeValue" {
		t.Fatalf("this constructor calls = %+v", first)
	}
	second := methods[1].Calls
	if len(second) != 1 || second[0].Kind != CallSuperConstructor || second[0].MethodName != "<init>" || !reflect.DeepEqual(second[0].Args, []string{"value"}) {
		t.Fatalf("super constructor calls = %+v", second)
	}
	for _, call := range []CallSite{first[0], second[0]} {
		if call.TargetType != nil || call.File != "Example.java" || call.StartByte >= call.EndByte || call.ArgCount != 1 {
			t.Errorf("explicit constructor metadata = %+v", call)
		}
	}
}

func TestExtractQualifiedSuperConstructorPreservesQualifier(t *testing.T) {
	types := extractJavaSource(t, `
class Outer { class Parent { Parent() {} } }
class Child extends Outer.Parent {
    Child(Outer outer) { outer.super(); }
}
`)

	call := findTypeBySimpleName(t, types, "Child").Methods[0].Calls[0]
	if call.Kind != CallSuperConstructor || call.Receiver != "outer" || call.ArgCount != 0 {
		t.Fatalf("qualified super call = %+v", call)
	}
}

func TestExtractQualifiedObjectCreationPreservesQualifier(t *testing.T) {
	types := extractJavaSource(t, `
class Outer { class Inner {} }
class Example {
    void create(Outer outer) { outer.new Inner(); }
}
`)

	call := findTypeBySimpleName(t, types, "Example").Methods[0].Calls[0]
	if call.Kind != CallObjectCreation || call.Receiver != "outer" || call.TargetType == nil || call.TargetType.Raw != "Inner" {
		t.Fatalf("qualified object creation = %+v", call)
	}
}
