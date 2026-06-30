package com.foo;

public class User extends BaseModel implements Auditable, Serializable {

    public String getName() {
        return "unnamed";
    }

    @Override
    public void audit() {
        System.out.println("audited");
    }
}
