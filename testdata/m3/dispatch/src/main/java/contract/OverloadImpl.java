package contract;

public class OverloadImpl implements OverloadContract {
    @Override
    public void run(String value) {}

    @Override
    public void run(Integer value) {}
}
