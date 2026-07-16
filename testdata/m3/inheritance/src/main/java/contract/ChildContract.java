package contract;

public interface ChildContract extends RootContract {
    default void childDefault() {
        rootDefault();
    }
}
