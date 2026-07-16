package model;

import support.Validator;

public final class ChildValue extends BaseValue {
    public ChildValue(String value) {
        super(Validator.normalize(value));
    }
}
