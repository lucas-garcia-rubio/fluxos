# M3 limitations

`fluxos` performs conservative static analysis of project Java source. It is not a Java
compiler and intentionally leaves uncertain calls unresolved or ambiguous.

## Source discovery and classpath

- Default discovery indexes `src/main/java` roots only; test sources are excluded.
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

- One concrete implementation is followed; zero or multiple implementations produce a
  terminal node.
- Multiple implementations do not fan out and cannot be selected interactively in M3.
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

- M3 emits Mermaid only; DOT and JSON graph output are not available.
- There is no interactive implementation picker, automatic ambiguity fan-out, depth or
  node limit, direction control, or output-format flag.
- Persistent indexing caches and advanced Mermaid styling are deferred.
