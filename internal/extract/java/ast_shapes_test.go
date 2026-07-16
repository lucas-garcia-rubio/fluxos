package java

import (
	"reflect"
	"testing"
)

func TestImportDeclarationASTShapes(t *testing.T) {
	tree, source := parseJavaSource(t, `
import com.foo.Service;
import com.foo.*;
import static com.foo.Util.run;
import static com.foo.Util.*;
class Example {}
`)

	nodes := findAllNodesByKind(tree.RootNode(), "import_declaration")
	if len(nodes) != 4 {
		t.Fatalf("import count = %d, want 4", len(nodes))
	}
	wantText := []string{
		"import com.foo.Service;",
		"import com.foo.*;",
		"import static com.foo.Util.run;",
		"import static com.foo.Util.*;",
	}
	wantKinds := [][]string{
		{"scoped_identifier"},
		{"scoped_identifier", "asterisk"},
		{"scoped_identifier"},
		{"scoped_identifier", "asterisk"},
	}
	for i, node := range nodes {
		if got := sourceText(source, node); got != wantText[i] {
			t.Errorf("import %d text = %q, want %q", i, got, wantText[i])
		}
		if got := namedChildKinds(node); !reflect.DeepEqual(got, wantKinds[i]) {
			t.Errorf("import %d named children = %v, want %v", i, got, wantKinds[i])
		}
	}
}

func TestConstructorASTShapes(t *testing.T) {
	tree, source := parseJavaSource(t, `
class Base { Base() {} }
class Example extends Base {
    Example() { this(1); }
    Example(int value) { super(); }
    void create() { new Example(); }
}
record Point(int x, int y) {
    Point { validate(); }
}
`)

	tests := []struct {
		kind string
		want int
	}{
		{kind: "constructor_declaration", want: 3},
		{kind: "compact_constructor_declaration", want: 1},
		{kind: "explicit_constructor_invocation", want: 2},
		{kind: "object_creation_expression", want: 1},
	}
	for _, tt := range tests {
		if got := len(findAllNodesByKind(tree.RootNode(), tt.kind)); got != tt.want {
			t.Errorf("%s count = %d, want %d", tt.kind, got, tt.want)
		}
	}

	invocations := findAllNodesByKind(tree.RootNode(), "explicit_constructor_invocation")
	if got, want := []string{sourceText(source, invocations[0]), sourceText(source, invocations[1])}, []string{"this(1);", "super();"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit constructor invocations = %v, want %v", got, want)
	}
	for i, invocation := range invocations {
		constructor := invocation.ChildByFieldName("constructor")
		arguments := invocation.ChildByFieldName("arguments")
		if constructor == nil || arguments == nil {
			t.Fatalf("explicit constructor invocation %d fields: constructor=%v arguments=%v", i, constructor, arguments)
		}
	}
	creation := findAllNodesByKind(tree.RootNode(), "object_creation_expression")[0]
	if typeNode := creation.ChildByFieldName("type"); typeNode == nil || sourceText(source, typeNode) != "Example" {
		t.Fatalf("object creation type was not exposed through field type")
	}
	if arguments := creation.ChildByFieldName("arguments"); arguments == nil {
		t.Fatal("object creation arguments field is missing")
	}
	record := findAllNodesByKind(tree.RootNode(), "record_declaration")[0]
	if parameters := record.ChildByFieldName("parameters"); parameters == nil || parameters.Kind() != "formal_parameters" {
		t.Fatalf("record parameters field = %v, want formal_parameters", parameters)
	}
	compact := findAllNodesByKind(tree.RootNode(), "compact_constructor_declaration")[0]
	if compact.ChildByFieldName("name") == nil || compact.ChildByFieldName("body") == nil || compact.ChildByFieldName("parameters") != nil {
		t.Fatalf("unexpected compact constructor fields: name=%v body=%v parameters=%v", compact.ChildByFieldName("name"), compact.ChildByFieldName("body"), compact.ChildByFieldName("parameters"))
	}
}

func TestMethodReferenceASTShapes(t *testing.T) {
	tree, source := parseJavaSource(t, `
class Base { void run() {} }
class Service { Service() {} static void staticRun() {} void run() {} }
class Example extends Base {
    Service service;
    void refs() {
        Runnable a = service::run;
        Runnable b = Service::staticRun;
        java.util.function.Supplier<Service> c = Service::new;
        Runnable d = super::run;
    }
}
`)

	nodes := findAllNodesByKind(tree.RootNode(), "method_reference")
	if len(nodes) != 4 {
		t.Fatalf("method reference count = %d, want 4", len(nodes))
	}
	want := []string{"service::run", "Service::staticRun", "Service::new", "super::run"}
	for i, node := range nodes {
		if got := sourceText(source, node); got != want[i] {
			t.Errorf("method reference %d = %q, want %q", i, got, want[i])
		}
		if node.NamedChildCount() == 0 {
			t.Errorf("method reference %d has no named receiver/type child", i)
		}
	}
}

func TestNestedTypeASTShape(t *testing.T) {
	tree, _ := parseJavaSource(t, `
class Outer {
    class Inner { void run() {} }
}
`)

	if got := len(findAllNodesByKind(tree.RootNode(), "class_declaration")); got != 2 {
		t.Fatalf("class declaration count = %d, want 2", got)
	}
	types := extractJavaSource(t, `class Outer { class Inner {} }`)
	if len(types) != 2 || types[1].FQCN != "Outer.Inner" || types[1].EnclosingFQCN != "Outer" {
		t.Fatalf("nested extraction = %+v", types)
	}
}

func TestParameterReceiverCharacterization(t *testing.T) {
	types := extractJavaSource(t, `
class Example {
    void run(Service service) { service.execute(); }
}
`)

	method := findTypeBySimpleName(t, types, "Example").Methods[0]
	if want := []Param{{Name: "service", Type: NewTypeRef("Service", false)}}; !reflect.DeepEqual(method.Params, want) {
		t.Fatalf("params = %v, want %v", method.Params, want)
	}
	if len(method.Calls) != 1 || method.Calls[0].Receiver != "service" || method.Calls[0].MethodName != "execute" {
		t.Fatalf("calls = %+v, want service.execute", method.Calls)
	}
}
