package app;

public class Looper {
    private final Helper helper;

    public Looper(Helper helper) {
        this.helper = helper;
    }

    public void loop() {
        helper.step();
    }
}
