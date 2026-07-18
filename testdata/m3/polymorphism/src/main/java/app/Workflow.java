package app;

import base.AbstractBase;
import multi.MultiService;
import nested.NestedTypes;
import transitive.RootContract;

public class Workflow {
    private final RootContract transitive;
    private final AbstractBase abstractBase;
    private final MultiService multiple;
    private final NestedTypes.Contract nested;
    private final SoloBase solo;

    public Workflow(RootContract transitive, AbstractBase abstractBase,
                    MultiService multiple, NestedTypes.Contract nested,
                    SoloBase solo) {
        this.transitive = transitive;
        this.abstractBase = abstractBase;
        this.multiple = multiple;
        this.nested = nested;
        this.solo = solo;
    }

    public void start() {
        transitive.run();
        abstractBase.run();
        multiple.run();
        nested.run();
        solo.run();
    }

    public abstract static class SoloBase {
        public abstract void run();
    }

    public static class SoloImplementation extends SoloBase {
        @Override
        public void run() {}
    }
}
