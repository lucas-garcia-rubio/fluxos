package resolve

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

// kindToken é o sufixo curto que diferencia terminais no TypeFQCN sintético.
// Não é exibido no label (renderer trunca no primeiro '#'); existe apenas para
// produzir IDs SHA-256 distintos entre kinds no mesmo call site.
func kindToken(kind ResolutionKind) string {
	switch kind {
	case ResolutionUnresolved:
		return "unresolved"
	case ResolutionNoImplementation:
		return "noimpl"
	case ResolutionAmbiguousType:
		return "ambtype"
	case ResolutionAmbiguousOverload:
		return "ambover"
	case ResolutionAmbiguousImplementation:
		return "ambimpl"
	default:
		return "unknown"
	}
}

// TerminalHandle devolve um MethodHandle sintético para um terminal. Terminais
// precisam de handles distintos por call site e por kind para não colidirem
// em Graph.Nodes quando duas linhas diferentes produzem o mesmo nome de
// receiver/method. O TypeFQCN carrega um sufixo determinístico
// ("#<kind>#<hash6>") que diferencia de FQCNs reais enquanto Method e
// Signature permanecem literais para o label.
//
// hash6 cobre (receiverFQCN, methodName, signature, kind-token, call.File,
// call.Line, call.StartByte) — tudo o que distingue dois call sites do mesmo
// nome. Signature pode ser "" quando o overload não foi resolvido.
func TerminalHandle(receiverFQCN, methodName, signature string, kind ResolutionKind, call java.CallSite) MethodHandle {
	token := kindToken(kind)
	material := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%d\x00%d",
		receiverFQCN, methodName, signature, token, call.File, call.Line, call.StartByte)
	sum := sha256.Sum256([]byte(material))
	return MethodHandle{
		TypeFQCN:  fmt.Sprintf("%s#%s#%s", receiverFQCN, token, hex.EncodeToString(sum[:3])),
		Method:    methodName,
		Signature: signature,
	}
}

// TerminalTarget constrói um ResolvedTarget terminal com handle sintético.
// Implementation candidates now live on Resolution.DispatchSite; the final
// argument remains only to keep terminal construction call sites uniform.
func TerminalTarget(kind ResolutionKind, receiverFQCN, methodName, signature string, call java.CallSite, note string, _ []string) ResolvedTarget {
	return ResolvedTarget{
		Key: ExecutionKey{
			Method:          TerminalHandle(receiverFQCN, methodName, signature, kind, call),
			RuntimeTypeFQCN: receiverFQCN,
		},
		Kind: kind,
		Note: note,
	}
}
