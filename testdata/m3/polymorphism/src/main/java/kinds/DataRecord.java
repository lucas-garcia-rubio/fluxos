package kinds;

public record DataRecord(String value) implements KindContract {
    @Override
    public String kind() {
        return "record";
    }
}
