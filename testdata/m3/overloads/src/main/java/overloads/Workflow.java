package overloads;

public class Workflow {
    public void run(String value) {
        finish(value);
    }

    public void run(int value) {
        finish(value);
    }

    private void finish(String value) {}

    private void finish(int value) {}
}
