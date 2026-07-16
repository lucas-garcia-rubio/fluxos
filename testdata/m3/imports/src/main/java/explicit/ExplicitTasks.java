package explicit;

import types.Helper;

public final class ExplicitTasks {
    private ExplicitTasks() {}

    public static void explicitRun() {}

    public static void explicitRun(Helper helper) {
        helper.work();
    }
}
