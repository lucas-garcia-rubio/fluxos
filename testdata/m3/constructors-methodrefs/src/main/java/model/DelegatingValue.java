package model;

import support.Validator;

public final class DelegatingValue {
    private final String value;

    public DelegatingValue(String value) {
        this(value, true);
    }

    private DelegatingValue(String value, boolean validate) {
        this.value = validate ? Validator.require(value) : value;
    }

    public String value() {
        return value;
    }
}
