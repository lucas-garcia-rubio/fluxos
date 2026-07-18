# M3 real-project smoke

The final M3 binary was exercised against the public Spring Petclinic project without
vendoring its source into this repository.

```text
project: https://github.com/spring-projects/spring-petclinic
revision: f182358d02e4a68e52bdbabf55ca7800288511e7
target: org.springframework.samples.petclinic.owner.OwnerController.processCreationForm(org.springframework.samples.petclinic.owner.Owner,org.springframework.validation.BindingResult,org.springframework.web.servlet.mvc.support.RedirectAttributes)
command: /tmp/fluxos-m3-final trace 'org.springframework.samples.petclinic.owner.OwnerController.processCreationForm(org.springframework.samples.petclinic.owner.Owner,org.springframework.validation.BindingResult,org.springframework.web.servlet.mvc.support.RedirectAttributes)' /tmp/opencode/spring-petclinic
result: exit 0; 6 nodes, 5 edges, 4 unresolved terminal nodes
observations: the project method and inherited BaseEntity.getId() resolved; Spring/JDK
  dependency receivers and the chained this.owners receiver remained unresolved, as
  expected without classpath and chained-expression inference
mermaid: rendered successfully with @mermaid-js/mermaid-cli to a temporary SVG
```

The binary used for the smoke was built from this workspace immediately before the run:

```bash
go build -o /tmp/fluxos-m3-final ./cmd/fluxos
```

No proprietary source, private path, or third-party output is committed here.
