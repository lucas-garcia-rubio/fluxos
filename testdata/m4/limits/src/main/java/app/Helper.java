package app;

public class Helper {
    private final Looper looper;

    public Helper(Looper looper) {
        this.looper = looper;
    }

    public void step() {
        looper.loop();
    }
}
