package java

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Extract percorre a AST a partir de tree.RootNode() e devolve todas as
// declarações de tipo encontradas (classes, interfaces, enums, records),
// com Methods preenchidos (Passo 7). Fields ainda não são extraídos.
// filePath é o caminho do arquivo .java no disco — populado em TypeDecl.File
// pra mensagens de erro/warning poderem referenciar arquivo:linha.
func Extract(filePath string, source []byte, tree *sitter.Tree) ([]*TypeDecl, error) {
	root := tree.RootNode()
	pkg := extractPackage(source, root)

	var types []*TypeDecl
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(uint(i))
		switch child.Kind() {
		case "class_declaration":
			types = append(types, extractTypeDecl(filePath, source, child, pkg, TypeKindClass))
		case "interface_declaration":
			types = append(types, extractTypeDecl(filePath, source, child, pkg, TypeKindInterface))
		case "enum_declaration":
			types = append(types, extractTypeDecl(filePath, source, child, pkg, TypeKindEnum))
		case "record_declaration":
			types = append(types, extractTypeDecl(filePath, source, child, pkg, TypeKindRecord))
		}
	}
	return types, nil
}

// extractPackage acha a declaração de package no nível superior e devolve o FQCN.
// Devolve "" se o arquivo está no package default.
func extractPackage(source []byte, root *sitter.Node) string {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(uint(i))
		if child.Kind() != "package_declaration" {
			continue
		}
		// scoped_identifier é filho nomeado posicional (sem field name) — achar por Kind.
		for j := 0; j < int(child.NamedChildCount()); j++ {
			inner := child.NamedChild(uint(j))
			if inner.Kind() == "scoped_identifier" || inner.Kind() == "identifier" {
				return sourceText(source, inner)
			}
		}
	}
	return ""
}

func extractTypeDecl(filePath string, source []byte, node *sitter.Node, pkg string, kind TypeKind) *TypeDecl {
	decl := &TypeDecl{
		Kind: kind,
		File: filePath,
		Line: int(node.StartPosition().Row) + 1, // tree-sitter é 0-indexed; arquivos são 1-indexed
	}

	if nameNode := node.ChildByFieldName("name"); nameNode != nil {
		decl.Name = sourceText(source, nameNode)
	}
	if pkg != "" && decl.Name != "" {
		decl.FQCN = pkg + "." + decl.Name
	} else {
		decl.FQCN = decl.Name
	}

	// Superclass só faz sentido em classe. O nó `superclass` é um wrapper em
	// volta do `type_identifier` (range inclui a keyword "extends") — drilla.
	if kind == TypeKindClass {
		if super := node.ChildByFieldName("superclass"); super != nil {
			if inner := findFirstByKind(super, "type_identifier"); inner != nil {
				decl.SuperClass = sourceText(source, inner)
			}
		}
	}

	// Interfaces: field name varia por Kind (ver TREESITTER.md, "assimetria pegadinha").
	// O wrapper (super_interfaces/extends_interfaces) contém um `type_list`,
	// que por sua vez contém os `type_identifier`s. Drilla dois níveis.
	var ifNode *sitter.Node
	for _, field := range []string{"super_interfaces", "extends_interfaces", "interfaces"} {
		if n := node.ChildByFieldName(field); n != nil {
			ifNode = n
			break
		}
	}
	if ifNode != nil {
		list := ifNode
		if ifNode.Kind() != "type_list" {
			list = findFirstByKind(ifNode, "type_list")
		}
		if list != nil {
			decl.Interfaces = extractTypeList(source, list)
		}
	}

	// Methods + Fields — drilla o body (class_body / interface_body / enum_body).
	if body := node.ChildByFieldName("body"); body != nil {
		decl.Methods = extractMethods(filePath, source, body)
		decl.Fields = extractFields(source, body)
	}

	// Garante slices não-nil pra JSON output ([] em vez de null).
	if decl.Interfaces == nil {
		decl.Interfaces = []string{}
	}
	if decl.Fields == nil {
		decl.Fields = []FieldDecl{}
	}
	if decl.Methods == nil {
		decl.Methods = []MethodDecl{}
	}

	return decl
}

// extractMethods percorre o body de um tipo (class_body, interface_body, etc.)
// e devolve Methods + Constructors. Field declarations, initializers e inner
// classes são ignorados (Fields virão noutra passada; inner classes em v2).
func extractMethods(filePath string, source []byte, body *sitter.Node) []MethodDecl {
	var methods []MethodDecl
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(uint(i))
		switch child.Kind() {
		case "method_declaration", "constructor_declaration":
			methods = append(methods, extractMethod(filePath, source, child))
		}
	}
	return methods
}

// extractFields percorre o body de um tipo e devolve FieldDecls. Cada
// field_declaration pode ter múltiplos declarators (`int x, y;`), então uma
// declaração vira N FieldDecls.
func extractFields(source []byte, body *sitter.Node) []FieldDecl {
	var fields []FieldDecl
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(uint(i))
		if child.Kind() != "field_declaration" {
			continue
		}
		fields = append(fields, extractFieldDecl(source, child)...)
	}
	return fields
}

// extractFieldDecl extrai uma ou mais FieldDecls de uma field_declaration.
// Devolve slice porque `int x, y;` tem múltiplos variable_declarators.
func extractFieldDecl(source []byte, node *sitter.Node) []FieldDecl {
	mods := extractModifiers(source, node)

	typeStr := ""
	if typeNode := node.ChildByFieldName("type"); typeNode != nil {
		typeStr = sourceText(source, typeNode)
	}

	// `declarator` (singular, `int x;`) ou `declarators` (plural, `int x, y;`).
	// variable_declarator_list dentro de `declarators` contém os variable_declarators.
	var declarators []*sitter.Node
	if decl := node.ChildByFieldName("declarator"); decl != nil {
		declarators = append(declarators, decl)
	}
	if decls := node.ChildByFieldName("declarators"); decls != nil {
		for i := 0; i < int(decls.NamedChildCount()); i++ {
			declarators = append(declarators, decls.NamedChild(uint(i)))
		}
	}

	var fields []FieldDecl
	for _, d := range declarators {
		fd := FieldDecl{
			Type:     typeStr,
			Modifier: mods,
		}
		if nameNode := d.ChildByFieldName("name"); nameNode != nil {
			fd.Name = sourceText(source, nameNode)
		}
		if valueNode := d.ChildByFieldName("value"); valueNode != nil {
			fd.Initializer = sourceText(source, valueNode)
		}
		// Garante Modifier não-nil pra JSON output consistente.
		if fd.Modifier == nil {
			fd.Modifier = []string{}
		}
		fields = append(fields, fd)
	}
	return fields
}

// extractMethod popula uma MethodDecl a partir de um method_declaration ou
// constructor_declaration. Constructors não têm field `type` (ReturnType fica "").
// filePath é propagado pra extractCalls popular CallSite.File.
func extractMethod(filePath string, source []byte, node *sitter.Node) MethodDecl {
	m := MethodDecl{
		Modifier: extractModifiers(source, node),
	}
	if nameNode := node.ChildByFieldName("name"); nameNode != nil {
		m.Name = sourceText(source, nameNode)
	}
	// ReturnType — ausente em constructor_declaration (deixa "").
	if typeNode := node.ChildByFieldName("type"); typeNode != nil {
		m.ReturnType = sourceText(source, typeNode)
	}
	if paramsNode := node.ChildByFieldName("parameters"); paramsNode != nil {
		m.Params = extractParams(source, paramsNode)
	}
	m.Calls = extractCalls(source, node, filePath)
	// Garante slices não-nil pra JSON output ([] em vez de null).
	if m.Modifier == nil {
		m.Modifier = []string{}
	}
	if m.Params == nil {
		m.Params = []Param{}
	}
	if m.Calls == nil {
		m.Calls = []CallSite{}
	}
	return m
}

// extractParams percorre o wrapper formal_parameters e devolve cada
// formal_parameter como Param. Spread params, receiver params e afins são
// ignorados (só formal_parameter é processado).
func extractParams(source []byte, paramsNode *sitter.Node) []Param {
	var params []Param
	for i := 0; i < int(paramsNode.NamedChildCount()); i++ {
		child := paramsNode.NamedChild(uint(i))
		if child.Kind() != "formal_parameter" {
			continue
		}
		p := Param{}
		if nameNode := child.ChildByFieldName("name"); nameNode != nil {
			p.Name = sourceText(source, nameNode)
		}
		if typeNode := child.ChildByFieldName("type"); typeNode != nil {
			p.Type = sourceText(source, typeNode)
		}
		params = append(params, p)
	}
	return params
}

// findFirstByKind devolve o primeiro filho nomeado de node com o Kind dado,
// ou nil se não encontrar.
func findFirstByKind(node *sitter.Node, kind string) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(uint(i))
		if c.Kind() == kind {
			return c
		}
	}
	return nil
}

func extractModifiers(source []byte, node *sitter.Node) []string {
	// `modifiers` é filho posicional (sem field name) — achar por Kind.
	modsNode := findFirstByKind(node, "modifiers")
	if modsNode == nil {
		return nil
	}
	// Itera TODOS os children (não só named): keyword tokens como "public",
	// "static" são nós unnamed na AST; annotations são named. ChildCount pega ambos.
	var mods []string
	for i := 0; i < int(modsNode.ChildCount()); i++ {
		child := modsNode.Child(uint(i))
		mods = append(mods, sourceText(source, child))
	}
	return mods
}

func extractTypeList(source []byte, node *sitter.Node) []string {
	var result []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(uint(i))
		result = append(result, sourceText(source, child))
	}
	return result
}

func sourceText(source []byte, node *sitter.Node) string {
	return string(source[node.StartByte():node.EndByte()])
}
