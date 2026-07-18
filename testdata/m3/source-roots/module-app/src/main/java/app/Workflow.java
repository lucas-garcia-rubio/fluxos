package app;

import api.Service;

public class Workflow {
    private final Service service;

    public Workflow(Service service) {
        this.service = service;
    }

    public void start() {
        service.run();
    }
}
