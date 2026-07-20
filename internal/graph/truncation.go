package graph

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

// TruncationKind classifica por que uma parte do call graph foi omitida do
// resultado. Apenas maxDepth e maxNodes são produzidos pelo Build; os demais
// kinds fecham o enum para que os renderers saibam formatar todos os casos
// futuros (Passos 7 e 9).
type TruncationKind string

const (
	TruncationMaxDepth       TruncationKind = "maxDepth"
	TruncationMaxNodes       TruncationKind = "maxNodes"
	TruncationMaxImpls       TruncationKind = "maxImpls"
	TruncationDiscoveryLimit TruncationKind = "discoveryLimit"
)

// Truncation é metadata out-of-band sobre uma omissão no grafo. Não entra em
// Graph.Nodes nem conta contra MaxNodes. Omitted reflete quantos targets
// concretos deixaram de ser admitidos naquele call site especificamente.
type Truncation struct {
	Kind    TruncationKind
	Caller  resolve.ExecutionKey
	Call    java.CallSite
	Omitted int
	Note    string
}

// ID produz um identificador estável derivado de Kind, Caller (method +
// runtime) e Call (kind + file lógico + StartByte). Não depende de Omitted ou
// Note — eles são consequência, não identidade.
func (t Truncation) ID() string {
	material := strings.Join([]string{
		"truncation-v1",
		string(t.Kind),
		t.Caller.Method.TypeFQCN,
		t.Caller.Method.Method,
		t.Caller.Method.Signature,
		t.Caller.RuntimeTypeFQCN,
		t.Call.Kind.String(),
		t.Call.File,
		strconv.FormatUint(uint64(t.Call.StartByte), 10),
	}, "\x00")
	sum := sha256.Sum256([]byte(material))
	return "t_" + hex.EncodeToString(sum[:6])
}

// compareTruncations ordena por Kind (ordem do enum), Caller (compareKeys),
// StartByte e por fim target impl FQCN quando aplicável. Truncations dedupadas
// por (caller, call) produzem ordens idênticas mesmo vindindo de ordens de
// planejamento diferentes.
func compareTruncations(a, b Truncation) int {
	if a.Kind != b.Kind {
		return compareTruncationKindOrder(a.Kind, b.Kind)
	}
	if cmp := compareExecutionKeysLocal(a.Caller, b.Caller); cmp != 0 {
		return cmp
	}
	if a.Call.StartByte != b.Call.StartByte {
		return compareIntLocal(int(a.Call.StartByte), int(b.Call.StartByte))
	}
	if a.Call.File != b.Call.File {
		return compareStringLocal(a.Call.File, b.Call.File)
	}
	if a.Omitted != b.Omitted {
		return compareIntLocal(a.Omitted, b.Omitted)
	}
	return compareStringLocal(a.Note, b.Note)
}

func compareTruncationKindOrder(a, b TruncationKind) int {
	order := map[TruncationKind]int{
		TruncationMaxDepth:       0,
		TruncationMaxNodes:       1,
		TruncationMaxImpls:       2,
		TruncationDiscoveryLimit: 3,
	}
	return compareIntLocal(order[a], order[b])
}

func compareExecutionKeysLocal(a, b resolve.ExecutionKey) int {
	if cmp := compareHandlesLocal(a.Method, b.Method); cmp != 0 {
		return cmp
	}
	return compareStringLocal(a.RuntimeTypeFQCN, b.RuntimeTypeFQCN)
}

func compareHandlesLocal(a, b resolve.MethodHandle) int {
	if a.TypeFQCN != b.TypeFQCN {
		return compareStringLocal(a.TypeFQCN, b.TypeFQCN)
	}
	if a.Method != b.Method {
		return compareStringLocal(a.Method, b.Method)
	}
	return compareStringLocal(a.Signature, b.Signature)
}

func compareStringLocal(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func compareIntLocal(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
