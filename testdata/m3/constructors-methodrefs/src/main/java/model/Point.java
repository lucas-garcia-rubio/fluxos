package model;

import support.Validator;

public record Point(int x, int y) {
    public Point {
        Validator.requireNonNegative(x);
        Validator.requireNonNegative(y);
    }
}
