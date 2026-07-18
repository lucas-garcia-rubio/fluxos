package service;

import repo.UserRepository;

public class UserService {
    public void create(UserRepository repository) {
        User user = new User();
        repository.save(user);
    }
}
