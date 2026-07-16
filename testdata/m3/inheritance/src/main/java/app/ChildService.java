package app;

import base.BaseService;
import contract.ChildContract;
import support.Helper;

public class ChildService extends BaseService implements ChildContract {
    private Helper baseHelper;

    @Override
    public void overriddenMethod() {
        baseHelper.work();
    }

    public void callParent() {
        super.inheritedMethod();
    }

    public void callGrandparent() {
        grandparentMethod();
    }

    public void callInheritedOverload() {
        overloaded();
    }

    public void useInheritedField() {
        inheritedHelper.work();
    }

    public void callDefaultMethod() {
        childDefault();
    }
}
