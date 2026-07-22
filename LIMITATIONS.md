# Current limitations

`fluxos` performs conservative static analysis of project Java source. It is not a Java
compiler and intentionally leaves uncertain calls unresolved or ambiguous.

## Source discovery and classpath

- Default discovery indexes `src/main/java` roots only; `--scope=all` also indexes
  `src/test/java`.
- With no conventional root, the requested path is treated as one explicit source root.
- Dependencies, JDK symbols, bytecode, Maven/Gradle dependency resolution, annotation
  processors, Lombok, and reflection are not indexed.
- A type outside indexed source does not gain speculative methods or constructors.

## Type and overload inference

- Overload selection is conservative and limited to arity plus obvious literals,
  object creation, lexical identifiers, `this`, and explicit casts.
- Numeric widening, boxing/unboxing, generic inference, and complete varargs ranking are
  not implemented.
- Generic arguments are erased for method identity.
- Unknown expressions preserve all compatible candidates and may produce an ambiguous
  overload terminal.
- Chained expressions and stream pipelines do not have general type inference.

## Polymorphism

- One concrete implementation is followed; zero implementations produce a terminal
  node. Multiple implementations produce a terminal by default.
- `--all-impls` and `--pick-impls` can fan out or select implementations explicitly.
- Full-TTY runs automatically offer an implementation picker unless `--no-prompt`,
  `--all-impls`, or declarative picks are set; non-TTY runs are non-interactive.
- A variable's initializer does not narrow an interface or abstract declared type.
- Sealed `permits` clauses are not used as an implementation source.

## Constructors, references, and lambdas

- Qualified object creation (`outer.new Inner()`), qualified `super(...)`, array
  constructor references, and non-static inner constructor references without an
  enclosing instance are unsupported.
- Complex method-reference receiver expressions may remain unresolved.
- Lambda bodies are executable boundaries but are not represented as synthetic methods,
  so their internal calls are not traced.
- Functional-interface invocation semantics for method references are not modeled.

## Nested, anonymous, and local types

- Named nested types are indexed with their enclosing type name.
- Anonymous and local classes are executable boundaries but are not indexed as normal
  project types.
- Calls inside anonymous or local class bodies are not attributed to the enclosing
  method.

## Output and UX

- Mermaid, DOT, and JSON are available. Mermaid is the default; JSON schema version 1
  exposes ordered nodes, edges, dispatch metadata, and truncations.
- Depth, node, and implementation limits are supported. `--include-unresolved=false` omits
  unresolved terminals and their incident edges; it does not change discovery or graph limits.
- `--all-impls` is bounded by `--max-impls` (default 5; zero is unlimited).
- Mermaid and DOT display truncation notes, but these are presentation only; JSON remains the
  stable structured format. See [JSON schema v1](docs/json-schema.md).
- CLI help is available globally and for each command. Version metadata, release automation,
  and packaged binaries are still pending.
- Persistent indexing caches and advanced Mermaid styling are deferred.
