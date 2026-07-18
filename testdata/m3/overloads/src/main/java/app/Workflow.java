package app;

import overloads.OtherRequest;
import overloads.Product;
import overloads.Request;
import overloads.Selector;

public class Workflow {
    private final Selector selector;
    private final String fieldValue;

    public Workflow(Selector selector, String fieldValue) {
        this.selector = selector;
        this.fieldValue = fieldValue;
    }

    public void calls() {
        selector.byArity();
        selector.byArity("text", 2);
        selector.byLiteral("text");
        selector.byLiteral(2);
        selector.byObject(new Request());
        useIdentifiers("parameter");
        Object castValue = fieldValue;
        selector.byLiteral((String) castValue);
        selector.byReference(null);
        selector.byLiteral(makeText());
        new Product("text");
        new Product(2);
        new Product(new Request());
    }

    private void useIdentifiers(String parameter) {
        int localValue = 1;
        selector.byLiteral(parameter);
        selector.byLiteral(localValue);
        selector.byLiteral(fieldValue);
    }

    private String makeText() {
        return "text";
    }

    public void referenceAmbiguity() {
        selector.byReference(new OtherRequest());
    }
}
