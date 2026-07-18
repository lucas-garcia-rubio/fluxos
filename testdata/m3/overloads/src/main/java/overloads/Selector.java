package overloads;

public class Selector {
    public void byArity() {}

    public void byArity(String value, int count) {}

    public void byLiteral(String value) {}

    public void byLiteral(int value) {}

    public void byObject(Request request) {}

    public void byObject(OtherRequest request) {}

    public void byReference(Request request) {}

    public void byReference(Object request) {}
}
