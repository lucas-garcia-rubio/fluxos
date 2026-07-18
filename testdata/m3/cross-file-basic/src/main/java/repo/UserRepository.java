package repo;

import service.User;
import support.Audit;
import support.Formatter;

public class UserRepository {
    public void save(User user) {
        Formatter formatter = new Formatter();
        formatter.format(user);
        Audit.log(user);
    }
}
