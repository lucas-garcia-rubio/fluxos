package app;

import contracts.A;
import contracts.B;

public class AlphaA implements A {
    private final B b;

    public AlphaA(B b) {
        this.b = b;
    }

    public void run() {
        b.work();
    }
}
