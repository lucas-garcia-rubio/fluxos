package app;

import contracts.A;

public class Workflow {
    private final A a;

    public Workflow(A a) {
        this.a = a;
    }

    public void start() {
        a.run();
    }
}
