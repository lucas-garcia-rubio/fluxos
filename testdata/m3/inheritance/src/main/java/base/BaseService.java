package base;

import support.Helper;

public class BaseService extends GrandparentService {
    protected Helper baseHelper;

    public void inheritedMethod() {
        baseHelper.work();
    }

    public void overloaded(String value) {
        baseHelper.work();
    }

    public void overriddenMethod() {
        baseHelper.work();
    }
}
