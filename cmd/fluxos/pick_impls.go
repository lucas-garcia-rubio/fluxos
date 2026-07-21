package main

import (
	"strings"
	"unicode"

	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

// parsePickImpls converte o valor cru de --pick-impls em um mapa de
// receiver FQCN -> FixedChoice. Sintaxe:
//
//	<fqcn>=<choice>[,<fqcn>=<choice>...]
//
// onde choice é "none", "all" ou um FQCN de implementation. Validação semântica
// (FQCN existe, receiver é ambíguo) acontece em validatePickImpls depois do
// index carregado.
func parsePickImpls(raw string) (map[string]resolve.FixedChoice, error) {
	if raw == "" {
		return nil, nil
	}
	choices := map[string]resolve.FixedChoice{}
	for _, mapping := range strings.Split(raw, ",") {
		mapping = strings.TrimSpace(mapping)
		if mapping == "" {
			return nil, usageErrorf("invalid --pick-impls: empty mapping")
		}
		eqIdx := strings.IndexByte(mapping, '=')
		if eqIdx <= 0 || eqIdx == len(mapping)-1 {
			return nil, usageErrorf("invalid --pick-impls mapping %q: expected <receiverFQCN>=<choice>", mapping)
		}
		receiver := strings.TrimSpace(mapping[:eqIdx])
		choice := strings.TrimSpace(mapping[eqIdx+1:])
		if receiver == "" || choice == "" {
			return nil, usageErrorf("invalid --pick-impls mapping %q: receiver and choice required", mapping)
		}
		if !validPickFQCN(receiver) {
			return nil, usageErrorf("invalid --pick-impls receiver FQCN %q", receiver)
		}
		parsed, err := parsePickChoice(choice)
		if err != nil {
			return nil, err
		}
		if _, dup := choices[receiver]; dup {
			return nil, usageErrorf("invalid --pick-impls: duplicate receiver %q", receiver)
		}
		choices[receiver] = parsed
	}
	return choices, nil
}

func parsePickChoice(choice string) (resolve.FixedChoice, error) {
	switch choice {
	case "none":
		return resolve.FixedChoice{Kind: resolve.FixedChoiceNone}, nil
	case "all":
		return resolve.FixedChoice{Kind: resolve.FixedChoiceAll}, nil
	}
	if !validPickFQCN(choice) {
		return resolve.FixedChoice{}, usageErrorf("invalid --pick-impls implementation FQCN %q", choice)
	}
	return resolve.FixedChoice{Kind: resolve.FixedChoiceExplicit, Impls: []string{choice}}, nil
}

func validPickFQCN(value string) bool {
	if value == "" {
		return false
	}
	for _, part := range strings.Split(value, ".") {
		if part == "" {
			return false
		}
		for i, r := range part {
			if i == 0 {
				if !unicode.IsLetter(r) && r != '_' && r != '$' {
					return false
				}
				continue
			}
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '$' {
				return false
			}
		}
	}
	return true
}
