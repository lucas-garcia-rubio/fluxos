package com.foo;

public class Workflow extends BaseWorkflow {
    private Helper helper;

    public void start() {
        super.initialize();
        prepare();
        helper.work();

        Helper local = new Helper();
        local.work();

        Audit.record();
    }

    private void prepare() {
        finish();
    }

    private void finish() {
        prepare();
    }
}

class BaseWorkflow {
    void initialize() {}
}

class Helper {
    void work() {
        Audit.record();
    }
}

class Audit {
    static void record() {}
}
