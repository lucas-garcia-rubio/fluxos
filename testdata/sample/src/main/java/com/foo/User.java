package com.foo;

public class User extends BaseModel implements Auditable, Serializable {

    private String name;
    private static final int COUNT = 10;
    protected List<String> tags;

    public String getName() {
        return "unnamed";
    }

    @Override
    public void audit() {
        System.out.println("audited");
    }
}
