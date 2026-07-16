package app;

import types.*;

import static contract.ContractTasks.interfaceRun;
import static explicit.ExplicitTasks.explicitRun;
import static inherited.ChildTasks.inheritedRun;
import static wildcard.WildcardTasks.*;

public class Workflow {
    private final Helper helper = new Helper();

    public void start() {
        explicitRun();
        explicitRun(helper);
        wildcardRun();
        inheritedRun();
        interfaceRun();
        currentRun();
        helper.work();
    }

    private void currentRun() {}
}
