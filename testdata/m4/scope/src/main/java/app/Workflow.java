package app;

public final class Workflow {
    private final Greeter greeter;

    public Workflow(Greeter greeter) {
        this.greeter = greeter;
    }

    public void start() {
        greeter.greet();
    }
}
