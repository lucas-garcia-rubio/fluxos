package support;

public final class Validator {
    private Validator() {
    }

    public static String normalize(String value) {
        return require(value).trim();
    }

    public static String require(String value) {
        if (value == null || value.isEmpty()) {
            throw new IllegalArgumentException("value must not be empty");
        }
        return value;
    }

    public static void requireNonNegative(int value) {
        if (value < 0) {
            throw new IllegalArgumentException("value must be non-negative");
        }
    }
}
