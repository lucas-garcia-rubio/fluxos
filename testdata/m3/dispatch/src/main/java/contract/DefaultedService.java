package contract;

public interface DefaultedService {
    default void run() {
        // default method body — M3 dispatch should descend into this.
    }
}
