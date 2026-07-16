package app;

public class Workflow {
    private ChildService service;

    public void start() {
        service.inheritedMethod();
        service.overriddenMethod();
        service.callParent();
        service.callGrandparent();
        service.callInheritedOverload();
        service.useInheritedField();
        service.callDefaultMethod();
    }
}
