package nested;

public class NestedTypes {
    public interface Contract {
        void run();
    }

    public abstract static class Base implements Contract {}

    public static class Impl extends Base {
        @Override
        public void run() {}
    }

    public class InnerImpl implements Contract {
        @Override
        public void run() {}
    }
}
