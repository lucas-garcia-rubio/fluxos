package app;

import repo.UserRepository;
import service.UserService;

public class Workflow {
    private final UserService service;

    public Workflow(UserService service) {
        this.service = service;
    }

    public void start() {
        prepare();
        UserRepository repository = new UserRepository();
        service.create(repository);
    }

    private void prepare() {
        Startup.check();
    }
}
