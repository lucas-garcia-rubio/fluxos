package kinds;

public enum ModeEnum implements KindContract {
    ACTIVE,
    INACTIVE;

    @Override
    public String kind() {
        return "enum";
    }
}
