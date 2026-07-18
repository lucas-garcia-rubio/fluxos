package resolve

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

type DispatchSiteID string

type ImplementationCandidate struct {
	ImplementationFQCN string
	Target             MethodHandle
	Kind               ResolutionKind
	Note               string
}

type DispatchSite struct {
	ID           DispatchSiteID
	Caller       ExecutionKey
	ReceiverFQCN string
	Method       string
	Signature    string
	Call         java.CallSite
	Candidates   []ImplementationCandidate
}

func NewDispatchSite(caller ExecutionKey, receiverFQCN, method, signature string, call java.CallSite, candidates []ImplementationCandidate) *DispatchSite {
	site := &DispatchSite{
		Caller: caller, ReceiverFQCN: receiverFQCN, Method: method, Signature: signature,
		Call: cloneCallSite(call), Candidates: append([]ImplementationCandidate(nil), candidates...),
	}
	sort.Slice(site.Candidates, func(i, j int) bool {
		left, right := site.Candidates[i], site.Candidates[j]
		if left.ImplementationFQCN != right.ImplementationFQCN {
			return left.ImplementationFQCN < right.ImplementationFQCN
		}
		if left.Target != right.Target {
			if left.Target.TypeFQCN != right.Target.TypeFQCN {
				return left.Target.TypeFQCN < right.Target.TypeFQCN
			}
			if left.Target.Method != right.Target.Method {
				return left.Target.Method < right.Target.Method
			}
			return left.Target.Signature < right.Target.Signature
		}
		return left.Kind < right.Kind
	})
	site.ID = dispatchSiteID(caller, receiverFQCN, method, call)
	return site
}

func CloneDispatchSite(site *DispatchSite) *DispatchSite {
	if site == nil {
		return nil
	}
	clone := *site
	clone.Call = cloneCallSite(site.Call)
	clone.Candidates = append([]ImplementationCandidate(nil), site.Candidates...)
	return &clone
}

func dispatchSiteID(caller ExecutionKey, receiverFQCN, method string, call java.CallSite) DispatchSiteID {
	parts := []string{"dispatch-site-v1", caller.Method.TypeFQCN, caller.Method.Method,
		caller.Method.Signature, caller.RuntimeTypeFQCN, call.Kind.String(), logicalSourcePath(call.File),
		strconv.FormatUint(uint64(call.StartByte), 10), receiverFQCN, method}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return DispatchSiteID("ds_" + hex.EncodeToString(sum[:6]))
}

func logicalSourcePath(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	for _, marker := range []string{"/src/main/java/", "/src/test/java/"} {
		if index := strings.LastIndex(path, marker); index >= 0 {
			return strings.TrimPrefix(path[index+1:], "/")
		}
	}
	return strings.TrimPrefix(path, "./")
}

func cloneCallSite(call java.CallSite) java.CallSite {
	clone := call
	clone.Args = append([]string(nil), call.Args...)
	if call.TargetType != nil {
		target := *call.TargetType
		clone.TargetType = &target
	}
	return clone
}
