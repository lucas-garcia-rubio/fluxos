package model;

import support.Validator;

public final class OverloadedValue {
    private final String value;

    public OverloadedValue(String value) {
        this.value = Validator.require(value);
    }

    public OverloadedValue(String value, int repetitions) {
        this.value = Validator.require(value.repeat(repetitions));
    }

    public String value() {
        return value;
    }
}
