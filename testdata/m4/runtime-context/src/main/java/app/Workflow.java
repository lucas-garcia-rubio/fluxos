package app;

public final class Workflow {
    public void start(First first, Second second) {
        first.run();
        second.run();
    }

    public void ambiguous(Contract service) {
        service.run();
    }
}
