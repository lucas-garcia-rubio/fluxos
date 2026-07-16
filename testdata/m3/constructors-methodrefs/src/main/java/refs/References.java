package refs;

import java.util.function.Function;
import java.util.function.Supplier;
import model.BaseValue;
import model.DefaultValue;
import support.Validator;

public final class References extends BaseValue {
    private final Service service = new Service();

    public References() {
        super("references");
    }

    public void references() {
        Runnable bound = service::run;
        Runnable own = this::own;
        Function<String, String> staticMethod = Validator::normalize;
        Function<BaseValue, String> unbound = BaseValue::value;
        Supplier<String> parent = super::value;
        Supplier<DefaultValue> constructor = DefaultValue::new;
        Supplier<Nested> nestedConstructor = Nested::new;
        Runnable nestedMethod = Nested::run;

        bound.run();
        own.run();
        staticMethod.apply("static");
        unbound.apply(this);
        parent.get();
        constructor.get();
        nestedConstructor.get();
        nestedMethod.run();
    }

    private void own() {
        Validator.require("this");
    }

    private static final class Service {
        private void run() {
            Validator.require("bound");
        }
    }

    public static final class Nested {
        public Nested() {
        }

        public static void run() {
            Validator.require("nested");
        }
    }
}
