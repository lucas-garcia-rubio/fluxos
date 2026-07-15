# TREESITTER — Referência para o projeto fluxos

Documento de consulta rápida sobre tree-sitter e a grammar Java. Use quando esquecer "qual o
field name pra X?" ou "que tipo de nó representa Y?".

## O que é tree-sitter

Biblioteca de parsing incremental, escrita em C, com bindings pra várias linguagens. Produz
uma AST (Abstract Syntax Tree) a partir de source code. Cada linguagem tem sua "grammar"
definindo os tipos de nó.

No fluxos, usamos:
- **`github.com/tree-sitter/go-tree-sitter`** — binding Go do runtime C.
- **`github.com/tree-sitter/tree-sitter-java/bindings/go`** — grammar Java compilada.

CGO é necessário porque o runtime é C.

## API Go essencial (go-tree-sitter v0.25)

### Tipos principais

| Tipo | O que é | Como se cria |
|---|---|---|
| `*sitter.Parser` | Estado do parser (caro de criar) | `sitter.NewParser()` |
| `*sitter.Language` | Gramática carregada | `sitter.NewLanguage(tree_sitter_java.Language())` |
| `*sitter.Tree` | AST retornada pelo parser | `parser.Parse(source, nil)` |
| `*sitter.Node` | Nó individual da árvore | `tree.RootNode()`, `node.Child(i)` |

### Ciclo de vida (importante)

Tudo que é recurso C precisa de `Close()` explícito:

```go
parser := sitter.NewParser()
defer parser.Close()

tree := parser.Parse(source, nil)
defer tree.Close()
```

O GC do Go não gerencia a memória C. Esquecer `Close()` vaza memória.

### Métodos de Node mais usados

| Método | Retorna | Uso |
|---|---|---|
| `Kind()` | `string` | Tipo do nó (ex.: `"class_declaration"`) |
| `ChildCount()` | `uint32` | Total de filhos (named + unnamed) |
| `NamedChildCount()` | `uint32` | Só filhos nomeados (sem pontuação) |
| `Child(i)` | `*Node` | Filho por índice (inclui pontuação) |
| `NamedChild(i)` | `*Node` | Filho nomeado por índice |
| `ChildByFieldName(name)` | `*Node` | Filho pelo field name; **retorna nil se não existir** |
| `FieldNameForChild(i)` | `string` | Nome do campo que o filho ocupa no pai (índice de `Child`, não `NamedChild`) |
| `IsNamed()` | `bool` | True se é nó nomeado (não pontuação) |
| `StartByte()` / `EndByte()` | `uint32` | Intervalo no source |
| `StartPosition()` / `EndPosition()` | `sitter.Point` | Posição (Row, Column) — 0-indexed |
| `ToSexp()` | `string` | S-expression da subárvore (debug) |

### Padrão de parse

```go
source, _ := os.ReadFile(path)
parser := sitter.NewParser()
defer parser.Close()
parser.SetLanguage(sitter.NewLanguage(tree_sitter_java.Language()))
tree := parser.Parse(source, nil)
defer tree.Close()
root := tree.RootNode()  // sempre é "program" pra Java
```

## Java grammar — tipos de nó principais

### Raiz

- **`program`** — sempre. Unidade de compilação (um arquivo `.java`).

### Cabeçalho do arquivo

- **`package_declaration`** — filho nomeado de `program`. Contém `scoped_identifier` (ex.: `com.foo`).
  - `scoped_identifier` tem fields `scope` e `name`, ambos `identifier`.
  - Sem `package` no arquivo → package default (string vazia).

- **`import_declaration`** — filho nomeado de `program`. Não possui fields semânticos:
  o nome é filho `identifier`/`scoped_identifier`, wildcard é filho `asterisk`, e
  `static` é token unnamed. Para distinguir os quatro formatos, combinar children e
  source text.

### Declarações de tipo (vira `TypeDecl`)

| Java | tree-sitter node | `TypeKind` |
|---|---|---|
| `class Foo { ... }` | `class_declaration` | `TypeKindClass` |
| `interface Foo { ... }` | `interface_declaration` | `TypeKindInterface` |
| `enum Foo { ... }` | `enum_declaration` | `TypeKindEnum` |
| `record Foo(...) { ... }` | `record_declaration` | `TypeKindRecord` |

### Fields das declarações de tipo

**⚠️ Assimetria pegadinha** — os field names **diferem** entre class e interface:

| Java construct | Field name na grammar | Em qual nó | Tipo do filho |
|---|---|---|---|
| Nome da classe (`Foo`) | `name` | `class_declaration` / `interface_declaration` / etc. | `identifier` |
| Modificadores (`public`, `private`, etc.) | **posicional, sem field name** | qualquer declaração | nó `modifiers` — ver "modifiers pegadinha" abaixo |
| `extends Base` (em **classe**) | **`superclass`** | `class_declaration` | wrapper `superclass` com qualquer `_type` |
| `implements X, Y` (em **classe**) | **`interfaces`** | `class_declaration` | wrapper `super_interfaces` com `type_list` |
| `extends X, Y` (em **interface**) | **posicional, sem field name** | `interface_declaration` | wrapper `extends_interfaces` com `type_list` |

**⚠️ Wrapper nesting pegadinha** — `superclass`, `super_interfaces` e `extends_interfaces`
**não são o nó folha**; são wrappers em volta de outros nós. O range do wrapper inclui keywords
(`extends`, `implements`) e vírgulas. Pra extrair o texto limpo do tipo, drilla um nível:

| Field | Wrapper | Nó folha dentro | Como extrair texto limpo |
|---|---|---|---|
| `superclass` | nó `superclass` | qualquer `_type` | primeiro named child; preserva qualified/generic type |
| `super_interfaces` | nó `super_interfaces` | `type_list` | achar `type_list` por Kind e iterar filhos |
| `extends_interfaces` | nó `extends_interfaces` posicional | `type_list` | achar wrapper por Kind, depois iterar `type_list` |

Sem drillar, `sourceText` no wrapper pega "extends BaseModel" (com keyword) ou
"Auditable, Serializable" (vírgula inclusiva). Bug silencioso — compila, roda, mas dados errados.

**⚠️ Modifiers pegadinha** — diferente de `name`, `body`, `type`, etc., o nó `modifiers`
**não tem field name**. É filho posicional (Nomeado como Kind `"modifiers"`, mas sem o
atributo field). Então `ChildByFieldName("modifiers")` retorna `nil`. Achar com
`findFirstByKind(node, "modifiers")`.

Pior: dentro de `modifiers`, os keyword tokens (`public`, `static`, `final`, etc.) são
**filhos unnamed** (literais, não nós nomeados). Iterar `NamedChildCount()` retorna 0.
Tem que iterar `ChildCount()` (todos os filhos, incluindo unnamed) e pegar o texto de cada
um. Annotations (`@Override`) são named (`marker_annotation`), aparecem nos dois métodos.

```go
// Padrão correto:
modsNode := findFirstByKind(node, "modifiers")   // não ChildByFieldName
for i := 0; i < int(modsNode.ChildCount()); i++ {  // não NamedChildCount
    child := modsNode.Child(uint(i))               // não NamedChild
    mods = append(mods, sourceText(source, child))
}
```
| Corpo da classe | `body` | `class_declaration` | `class_body` |
| Corpo da interface | `body` | `interface_declaration` | `interface_body` |

Nota: enum e record têm variações. Enum pode ter `interfaces` (implements); record tem
`parameters` (header do record). Confirme empiricamente com o walker do Passo 4.

### Corpo — methods e fields

Dentro de `class_body`:

- **`field_declaration`** — um field. O field `declarator` possui `multiple: true` e
  aparece repetido para `int x, y, z`; não existe wrapper `declarators`. Iterar named
  children e filtrar `variable_declarator`. Cada declarator tem fields `name` e `value`.

- **`method_declaration`** — um método. Ver tabela abaixo.

- **`constructor_declaration`** — construtor. Mesma estrutura de method, sem `type`.

- **`class_declaration`**, **`interface_declaration`**, etc. — classes internas.

### Method declarations (vira `MethodDecl`)

| Java | tree-sitter field | Conteúdo |
|---|---|---|
| Nome do método (`foo`) | `name` | `identifier` |
| Modificadores (`public static`) | `modifiers` | nó `modifiers` com filhos |
| Tipo de retorno (`void`, `String`, `List<X>`) | `type` | varia: `void_type`, `type_identifier`, `generic_type`, etc. |
| Parâmetros `(String[] args, int x)` | `parameters` | `formal_parameters` contendo vários `formal_parameter` |
| Corpo `{ ... }` | `body` | `block` |

### Parâmetros (vira `Param`)

Cada `formal_parameter` (dentro de `formal_parameters`):

| Java | tree-sitter field | Conteúdo |
|---|---|---|
| Nome (`args`, `x`) | `name` | `identifier` |
| Tipo (`String[]`, `int`) | `type` | varia: `type_identifier`, `array_type`, `integral_type`, etc. |

`array_type` tem fields `element` e `dimensions`.

Varargs usam `spread_parameter`, não `formal_parameter`. O tipo e o declarator são
filhos nomeados posicionais; `variable_declarator.name` contém o nome do parâmetro.
No modelo atual, `Param.Variadic` preserva essa informação e a signature provisória
normaliza `T...` como `T[]`.

As signatures de M3 são raw-erased e determinísticas: removem espaços e argumentos
genéricos aninhados, mas ainda preservam o nome de tipo como escrito no source. Exemplos:
`()`; `(String,int)`; `(java.util.List)`; `(String[])`. A canonicalização para FQCN
depende do índice e dos imports e fica para os passos seguintes.

### Chamadas de método (M2 — preview)

`method_invocation` — uma chamada `obj.method(args)`:

| Parte | tree-sitter field |
|---|---|
| Receptor (`obj`) | `object` (pode ser `identifier`, `field_access`, etc.) |
| Nome do método chamado | `name` (`identifier`) |
| Argumentos | `arguments` (`argument_list`) |

`object_creation_expression` — `new Foo(args)`:

| Parte | tree-sitter field |
|---|---|
| Tipo construído (`Foo`) | `type` |
| Argumentos | `arguments` (`argument_list`) |

### Construtores e method references

- `constructor_declaration` — construtor comum, com fields `name`, `parameters` e `body`.
- `compact_constructor_declaration` — construtor compacto de record, com `name` e `body`.
- `explicit_constructor_invocation` — chamadas `this(...)` e `super(...)`.
- `method_reference` — filhos sem field names; receiver/type e membro final precisam ser
  interpretados pela ordem/source text. Cobre `obj::method`, `Type::method`, `super::method`
  e `Type::new`.

### Executable boundaries

Walkers iniciados no body de um método não devem atravessar automaticamente:

- `lambda_expression`;
- `class_body` de classe local ou anônima;
- `interface_body`, `enum_body` e `annotation_type_body` aninhados.

Calls e local vars dentro desses nós pertencem a outra unidade executável. Em object
creation anônima, continuar percorrendo `argument_list`, mas pular somente o `class_body`.

## Padrões de extração

### Achar nó por Kind (switch em string)

```go
switch node.Kind() {
case "class_declaration":
    // tratar como classe
case "interface_declaration":
    // tratar como interface
default:
    // ignora
}
```

### Pegar field por nome

```go
nameNode := classDecl.ChildByFieldName("name")
if nameNode == nil {
    return "", fmt.Errorf("class sem name?")
}
name := string(source[nameNode.StartByte():nameNode.EndByte()])
```

### Iterar lista (modifiers, interfaces)

```go
interfacesNode := classDecl.ChildByFieldName("super_interfaces")
if interfacesNode == nil {
    // sem implements; default é slice vazio
    return nil
}
// type_list está dentro; iterar
var result []string
for i := 0; i < int(interfacesNode.NamedChildCount()); i++ {
    child := interfacesNode.NamedChild(uint32(i))
    result = append(result, string(source[child.StartByte():child.EndByte()]))
}
```

### Iterar todos os filhos (debug/geral)

```go
for i := 0; i < int(node.ChildCount()); i++ {
    child := node.Child(uint32(i))
    if !child.IsNamed() {
        continue  // pula pontuação/keywords
    }
    fieldName := node.FieldNameForChild(uint32(i))
    // ... usa child e fieldName
}
```

### Walk recursivo (debug)

```go
func walk(node *sitter.Node, depth int, myField string) {
    indent := strings.Repeat("  ", depth)
    if myField != "" {
        fmt.Printf("%s%s: %s\n", indent, myField, node.Kind())
    } else {
        fmt.Printf("%s%s\n", indent, node.Kind())
    }
    for i := 0; i < int(node.ChildCount()); i++ {
        child := node.Child(uint32(i))
        if child.IsNamed() {
            walk(child, depth+1, node.FieldNameForChild(uint32(i)))
        }
    }
}
```

## node-types.json — referência completa

A grammar define todos os tipos de nó e fields num arquivo JSON. Consulte pra casos não
cobertos aqui:

- <https://github.com/tree-sitter/tree-sitter-java/blob/master/src/node-types.json>

Cada entrada lista: nome do nó, fields (nome + tipo), filhos possíveis.

## Erros comuns

| Sintoma | Causa provável |
|---|---|
| `fmt.Printf("%v", node)` imprime `&{{[0 0 0 0] 0x...}` | Node é struct opaca com fields unexported; use os métodos (`Kind()`, `ToSexp()`, etc.) em vez de `%v` |
| `cannot refer to unexported name` | Tentou acessar field lowercase de outro pacote |
| `undefined reference to tree_sitter_java` | Esqueceu `go get` da grammar ou `go mod tidy` |
| `gcc: command not found` | CGO desabilitado sem compilador C; verifique `go env CC` |
| Memory leak crescente em run | Esqueceu `tree.Close()` (e/ou `parser.Close()`) |
| `defer tree.Close()` dentro de loop não libera | `defer` é por-função, não por-iteração; use `tree.Close()` explícito |

## Links

- **go-tree-sitter** — <https://github.com/tree-sitter/go-tree-sitter>
- **tree-sitter-java grammar** — <https://github.com/tree-sitter/tree-sitter-java>
- **node-types.json** (referência completa) — <https://github.com/tree-sitter/tree-sitter-java/blob/master/src/node-types.json>
- **tree-sitter docs gerais** — <https://tree-sitter.github.io/tree-sitter/>
- **Go tree-sitter API** — <https://pkg.go.dev/github.com/tree-sitter/go-tree-sitter>
