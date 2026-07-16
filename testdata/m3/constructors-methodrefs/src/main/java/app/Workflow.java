package app;

import model.BaseValue;
import model.ChildValue;
import model.DefaultValue;
import model.DelegatingValue;
import model.OverloadedValue;
import model.Point;
import support.Validator;

public final class Workflow {
    private Workflow() {
    }

    public static void run() {
        new DefaultValue();
        new OverloadedValue("one");
        new OverloadedValue("two", 2);
        new DelegatingValue("delegated");
        new ChildValue("child");
        new Point(1, 2);
        new OverloadedValue(Validator.normalize("nested"));
        new BaseValue(Validator.normalize("anonymous")) {
            @Override
            public String value() {
                return Validator.normalize("ignored-body");
            }
        };
    }
}
