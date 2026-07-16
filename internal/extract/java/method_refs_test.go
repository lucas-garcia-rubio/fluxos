package java

import "testing"

func TestExtractMethodAndConstructorReferences(t *testing.T) {
	types := extractJavaSource(t, `
class Base { void inherited() {} }
class Outer {
    static class Inner { Inner() {} void run() {} }
    void refs(Service service) {
        Runnable a = service::run;
        Runnable b = this::own;
        Runnable c = Service::staticRun;
        Runnable d = super::inherited;
        java.util.function.Supplier<Inner> e = Inner::new;
        java.util.function.Supplier<Inner> f = Outer.Inner::new;
    }
    void own() {}
}
class Service { void run() {} static void staticRun() {} }
`)
	method := testMethodByName(t, findTypeBySimpleName(t, types, "Outer"), "refs")
	if len(method.Calls) != 6 {
		t.Fatalf("calls = %+v, want 6 references", method.Calls)
	}
	want := []struct {
		kind      CallKind
		method    string
		receiver  string
		target    string
		qualifier ReferenceQualifierKind
	}{
		{CallMethodReference, "run", "service", "", ReferenceQualifierName},
		{CallMethodReference, "own", "this", "", ReferenceQualifierExpression},
		{CallMethodReference, "staticRun", "Service", "", ReferenceQualifierName},
		{CallMethodReference, "inherited", "super", "", ReferenceQualifierSuper},
		{CallConstructorReference, "<init>", "", "Inner", ReferenceQualifierName},
		{CallConstructorReference, "<init>", "", "Outer.Inner", ReferenceQualifierName},
	}
	for i, expected := range want {
		call := method.Calls[i]
		target := ""
		if call.TargetType != nil {
			target = call.TargetType.Raw
		}
		if call.Kind != expected.kind || call.MethodName != expected.method || call.Receiver != expected.receiver || target != expected.target || call.ReferenceQualifier != expected.qualifier {
			t.Errorf("call %d = %+v, want %+v", i, call, expected)
		}
		if call.Args == nil || call.ArgCount != 0 || call.StartByte >= call.EndByte {
			t.Errorf("call %d metadata = %+v", i, call)
		}
	}
}

func TestMethodReferencesRespectExecutableBoundaries(t *testing.T) {
	types := extractJavaSource(t, `
class Example {
    void run(Service service) {
        Runnable lambda = () -> { Runnable hidden = service::run; };
        class Local { void nested() { Runnable hidden = service::run; } }
        Object object = new Object(service::run) { void nested() { Runnable hidden = service::run; } };
    }
}
class Service { void run() {} }
`)
	method := testMethodByName(t, findTypeBySimpleName(t, types, "Example"), "run")
	refs := 0
	for _, call := range method.Calls {
		if call.Kind == CallMethodReference {
			refs++
		}
	}
	if refs != 1 {
		t.Fatalf("method reference count = %d, calls = %+v", refs, method.Calls)
	}
}

func testMethodByName(t *testing.T, typ *TypeDecl, name string) *MethodDecl {
	t.Helper()
	for i := range typ.Methods {
		if typ.Methods[i].Name == name {
			return &typ.Methods[i]
		}
	}
	t.Fatalf("method %q not found on %s", name, typ.FQCN)
	return nil
}
