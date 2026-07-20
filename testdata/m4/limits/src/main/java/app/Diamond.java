package app;

public class Diamond {
    public void top() {
        left();
        right();
    }
    private void left() { bottom(); }
    private void right() { bottom(); }
    private void bottom() {}
}
