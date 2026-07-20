package app;

public class Mixed {
    public void go() {
        step1();
        shortcut();
    }
    private void step1() { step2(); }
    private void step2() { step3(); }
    private void step3() { shared(); }
    private void shortcut() { shared(); }
    private void shared() {}
}
