package render

import (
	"fmt"
	"strings"
	"unicode"
)

// DiagramNodeLabel returns the presentation label for a Mermaid or DOT node.
// NodeView.Label remains the format-neutral, full-FQCN label used by JSON.
func DiagramNodeLabel(node NodeView, showFQCN, showFQCNParams bool) string {
	label := node.Label
	method := node.Execution.Method
	if method.TypeFQCN != "" && method.Method != "" {
		if formatted, ok := formatMethodLabel(label, method, showFQCN, showFQCNParams); ok {
			label = formatted
		}
	} else {
		if !showFQCN {
			label = shortenOwnerPrefix(label, method.TypeFQCN)
			if label == node.Label {
				label = shortenLabelPrefix(label)
			}
		}
		if !showFQCNParams {
			label = shortenLabelSignature(label)
		}
	}
	if !showFQCN {
		label = shortenRuntimeLabel(label, node.Execution.RuntimeTypeFQCN)
	}
	return label
}

// DiagramTruncationLabel returns a renderer-neutral truncation label. Mermaid
// adds its comment marker separately; DOT uses this value directly.
func DiagramTruncationLabel(truncation TruncationView, showFQCN, showFQCNParams bool) string {
	return fmt.Sprintf("truncation: %s; omitted %d while tracing %s",
		truncationKindLabel(truncation.Kind), truncation.Omitted,
		DiagramExecutionLabel(truncation.Caller, showFQCN, showFQCNParams))
}

// DiagramExecutionLabel formats a caller for a diagram truncation marker.
func DiagramExecutionLabel(execution ExecutionView, showFQCN, showFQCNParams bool) string {
	method := execution.Method
	typeFQCN := cleanSyntheticType(method.TypeFQCN)
	if typeFQCN == "" {
		typeFQCN = "<unknown>"
	}
	if !showFQCN {
		typeFQCN = DisplayTypeName(typeFQCN)
	}
	signature := method.Signature
	if signature == "" {
		signature = "()"
	}
	if !showFQCNParams {
		signature = shortenSignature(signature)
	}
	return typeFQCN + "." + method.Method + signature
}

func formatMethodLabel(label string, method MethodView, showFQCN, showFQCNParams bool) (string, bool) {
	typeFQCN := cleanSyntheticType(method.TypeFQCN)
	signature := method.Signature
	if signature == "" {
		signature = "()"
	}
	oldPrefix := typeFQCN + "." + method.Method + signature
	if !strings.HasPrefix(label, oldPrefix) {
		return label, false
	}
	if !showFQCN {
		typeFQCN = DisplayTypeName(typeFQCN)
	}
	if !showFQCNParams {
		signature = shortenSignature(signature)
	}
	return typeFQCN + "." + method.Method + signature + label[len(oldPrefix):], true
}

func shortenSignature(signature string) string {
	if signature == "" || signature == "()" {
		return signature
	}
	open := strings.IndexByte(signature, '(')
	close := strings.LastIndexByte(signature, ')')
	if open < 0 || close <= open || close != len(signature)-1 {
		return signature
	}
	parameters := strings.Split(signature[open+1:close], ",")
	for i, parameter := range parameters {
		trimmed := strings.TrimSpace(parameter)
		if trimmed == "" {
			continue
		}
		leading := parameter[:len(parameter)-len(strings.TrimLeft(parameter, " \t"))]
		trailing := parameter[len(strings.TrimRight(parameter, " \t")):]
		parameters[i] = leading + DisplayTypeName(trimmed) + trailing
	}
	return signature[:open+1] + strings.Join(parameters, ",") + signature[close:]
}

func shortenLabelSignature(label string) string {
	open := strings.IndexByte(label, '(')
	if open < 0 {
		return label
	}
	closeOffset := strings.IndexByte(label[open:], ')')
	if closeOffset < 0 {
		return label
	}
	close := open + closeOffset
	signature := label[open : close+1]
	return label[:open] + shortenSignature(signature) + label[close+1:]
}

// DisplayTypeName removes the package prefix while retaining the class and
// any nested-class segments in the repository's conventional Java FQCN form.
func DisplayTypeName(typeFQCN string) string {
	typeFQCN = cleanSyntheticType(typeFQCN)
	if typeFQCN == "" {
		return ""
	}
	parts := strings.Split(typeFQCN, ".")
	for i, part := range parts {
		if part != "" && unicode.IsUpper([]rune(part)[0]) {
			return strings.Join(parts[i:], ".")
		}
	}
	return typeFQCN
}

func shortenOwnerPrefix(label, typeFQCN string) string {
	typeFQCN = cleanSyntheticType(typeFQCN)
	if typeFQCN == "" {
		return label
	}
	prefix := typeFQCN + "."
	if !strings.HasPrefix(label, prefix) {
		return label
	}
	return DisplayTypeName(typeFQCN) + label[len(typeFQCN):]
}

func shortenLabelPrefix(label string) string {
	base := label
	suffix := ""
	if separator := strings.Index(label, " ["); separator >= 0 {
		base, suffix = label[:separator], label[separator:]
	}
	parts := strings.Split(base, ".")
	first := -1
	for i, part := range parts {
		if part != "" && unicode.IsUpper([]rune(part)[0]) {
			first = i
			break
		}
	}
	if first < 0 {
		return label
	}
	method := first + 1
	for method < len(parts) && parts[method] != "" && unicode.IsUpper([]rune(parts[method])[0]) {
		method++
	}
	if method == len(parts) {
		return label
	}
	return strings.Join(parts[first:method], ".") + "." + strings.Join(parts[method:], ".") + suffix
}

func shortenRuntimeLabel(label, runtime string) string {
	if runtime == "" {
		const marker = " [runtime: "
		if separator := strings.LastIndex(label, marker); separator >= 0 && strings.HasSuffix(label, "]") {
			runtime = label[separator+len(marker) : len(label)-1]
		}
	}
	if runtime == "" {
		return label
	}
	old := " [runtime: " + runtime + "]"
	if strings.HasSuffix(label, old) {
		return strings.TrimSuffix(label, old) + " [runtime: " + DisplayTypeName(runtime) + "]"
	}
	return label
}

func cleanSyntheticType(typeFQCN string) string {
	if separator := strings.IndexByte(typeFQCN, '#'); separator >= 0 {
		return typeFQCN[:separator]
	}
	return typeFQCN
}

func truncationKindLabel(kind string) string {
	switch kind {
	case "maxDepth":
		return "depth limit"
	case "maxNodes":
		return "node limit"
	case "maxImpls":
		return "implementation limit"
	case "discoveryLimit":
		return "discovery limit"
	default:
		return kind + " limit"
	}
}
