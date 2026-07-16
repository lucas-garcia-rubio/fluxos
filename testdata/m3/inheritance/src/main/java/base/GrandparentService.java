package base;

import support.Helper;

public class GrandparentService {
    protected Helper inheritedHelper;

    public void grandparentMethod() {
        inheritedHelper.work();
    }

    public void overloaded() {
        inheritedHelper.work();
    }

    void packageOnlyMethod() {}

    private void privateMethod() {}
}
