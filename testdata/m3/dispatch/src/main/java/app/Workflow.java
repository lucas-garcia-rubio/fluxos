package app;

import contract.EmptyService;
import contract.DefaultedService;
import contract.SingleService;
import contract.MultiService;
import contract.OverloadContract;

public class Workflow {
    private final EmptyService empty;
    private final DefaultedService defaulted;
    private final SingleService single;
    private final MultiService multi;
    private final OverloadContract overloads;

    public Workflow(EmptyService empty, DefaultedService defaulted,
                    SingleService single, MultiService multi,
                    OverloadContract overloads) {
        this.empty = empty;
        this.defaulted = defaulted;
        this.single = single;
        this.multi = multi;
        this.overloads = overloads;
    }

    public void start() {
        empty.run();
        defaulted.run();
        single.run();
        multi.run();
        overloads.run(value());
    }

    private String value() {
        return "x";
    }
}
