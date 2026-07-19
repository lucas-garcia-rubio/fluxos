package app;

public final class WorkflowTest {
    public void run(Greeter greeter) {
        new Workflow(greeter).start();
    }
}
