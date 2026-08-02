package render

import (
	"fmt"
	"strings"
	"unicode"
)

// DiagramNodeLabel returns the presentation label for a Mermaid or DOT node.
// NodeView.Label remains the format-neutral, full-FQCN label used by JSON.
func DiagramNodeLabel(node NodeView, showFQCN bool) string {
	if showFQCN {
		return node.Label
	}

	label := shortenOwnerPrefix(node.Label, node.Execution.Method.TypeFQCN)
	if label == node.Label {
		label = shortenLabelPrefix(label)
	}
	return shortenRuntimeLabel(label, node.Execution.RuntimeTypeFQCN)
}

// DiagramTruncationLabel returns a renderer-neutral truncation label. Mermaid
// adds its comment marker separately; DOT uses this value directly.
func DiagramTruncationLabel(truncation TruncationView, showFQCN bool) string {
	return fmt.Sprintf("truncation: %s; omitted %d while tracing %s",
		truncationKindLabel(truncation.Kind), truncation.Omitted,
		DiagramExecutionLabel(truncation.Caller, showFQCN))
}

// DiagramExecutionLabel formats a caller for a diagram truncation marker.
func DiagramExecutionLabel(execution ExecutionView, showFQCN bool) string {
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
	return typeFQCN + "." + method.Method + signature
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
