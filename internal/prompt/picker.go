// Package prompt contains the line-oriented implementation picker. It knows
// how to collect a complete batch, but does not decide when prompting is
// appropriate; that policy belongs to the command package.
package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
	"github.com/lucas-garcia-rubio/fluxos/internal/trace"
)

// ChoiceTokenKind is the syntactic kind of a site choice token.
type ChoiceTokenKind uint8

const (
	ChoiceTokenCandidate ChoiceTokenKind = iota + 1
	ChoiceTokenNone
	ChoiceTokenAll
)

// SelectionToken is the pure-parser representation of one site=choice token.
// Site is the 1-based position shown to the user. Candidate is also 1-based
// when Kind is ChoiceTokenCandidate.
type SelectionToken struct {
	Site      int
	Kind      ChoiceTokenKind
	Candidate int
}

// ParseSelectionTokens parses the line grammar without consulting any sites
// or performing I/O. Site and candidate numbers are positive decimal values;
// none, all, q, and cancel are case-insensitive where applicable.
func ParseSelectionTokens(line string) ([]SelectionToken, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil, errors.New("a seleção está vazia")
	}

	tokens := make([]SelectionToken, 0, len(fields))
	seen := make(map[int]struct{}, len(fields))
	for _, field := range fields {
		if strings.Count(field, "=") != 1 {
			return nil, fmt.Errorf("o token %q deve usar o formato site=opção", field)
		}
		siteText, choiceText, _ := strings.Cut(field, "=")
		if siteText == "" || choiceText == "" {
			return nil, fmt.Errorf("o token %q deve usar o formato site=opção", field)
		}

		site, err := parsePositiveNumber(siteText)
		if err != nil {
			return nil, fmt.Errorf("local %q inválido: %w", siteText, err)
		}
		if _, duplicate := seen[site]; duplicate {
			return nil, fmt.Errorf("o local %d foi informado mais de uma vez", site)
		}
		seen[site] = struct{}{}

		token := SelectionToken{Site: site}
		switch strings.ToLower(choiceText) {
		case "none":
			token.Kind = ChoiceTokenNone
		case "all":
			token.Kind = ChoiceTokenAll
		default:
			candidate, candidateErr := parsePositiveNumber(choiceText)
			if candidateErr != nil {
				return nil, fmt.Errorf("escolha %q inválida para o local %d: informe o número do candidato, none ou all", choiceText, site)
			}
			token.Kind = ChoiceTokenCandidate
			token.Candidate = candidate
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func parsePositiveNumber(value string) (int, error) {
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, errors.New("informe um número positivo")
		}
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, errors.New("informe um número positivo")
	}
	return parsed, nil
}

type validatedBatch struct {
	choices  map[int]resolve.DispatchChoice
	complete bool
}

func validateTokens(tokens []SelectionToken, sites []resolve.DispatchSite) (validatedBatch, error) {
	batch := validatedBatch{choices: make(map[int]resolve.DispatchChoice, len(tokens))}
	for _, token := range tokens {
		if token.Site > len(sites) {
			return validatedBatch{}, fmt.Errorf("o local %d está fora do intervalo; informe um valor entre 1 e %d", token.Site, len(sites))
		}

		choice := resolve.DispatchChoice{}
		switch token.Kind {
		case ChoiceTokenNone:
			choice.Mode = resolve.ChoiceNone
		case ChoiceTokenAll:
			choice.Mode = resolve.ChoiceAll
		case ChoiceTokenCandidate:
			candidates := sites[token.Site-1].Candidates
			if token.Candidate > len(candidates) {
				return validatedBatch{}, fmt.Errorf("o candidato %d está fora do intervalo para o local %d; informe um valor entre 1 e %d", token.Candidate, token.Site, len(candidates))
			}
			choice.Mode = resolve.ChoiceSelected
			choice.ImplementationFQCN = candidates[token.Candidate-1].ImplementationFQCN
		default:
			return validatedBatch{}, fmt.Errorf("tipo de escolha desconhecido para o local %d", token.Site)
		}
		batch.choices[token.Site] = choice
	}
	batch.complete = len(batch.choices) == len(sites)
	return batch, nil
}

// Picker implements trace.ImplementationSelector. The reader is constructed
// once and retained across Select calls so buffered input is not lost between
// discovery rounds.
type Picker struct {
	Reader *bufio.Reader
	ErrOut io.Writer

	// MaxImpls controls only the explanatory label for all. Zero means
	// unlimited, matching the coordinator policy.
	MaxImpls int
}

// NewPicker creates a line-oriented picker with persistent input buffering.
func NewPicker(input io.Reader, stderr io.Writer, maxImpls int) *Picker {
	reader, alreadyBuffered := input.(*bufio.Reader)
	if !alreadyBuffered && input != nil {
		reader = bufio.NewReader(input)
	}
	return &Picker{Reader: reader, ErrOut: stderr, MaxImpls: maxImpls}
}

// Select displays the supplied, already ordered frontier and returns a
// complete batch. No partial or speculative selections escape this method.
func (p *Picker) Select(sites []resolve.DispatchSite) ([]trace.Selection, error) {
	if len(sites) == 0 {
		return []trace.Selection{}, nil
	}
	if p == nil {
		return nil, errors.New("o seletor de prompt é nulo")
	}
	if p.Reader == nil {
		return nil, errors.New("o leitor de entrada do prompt é nulo")
	}

	if err := p.renderSites(sites); err != nil {
		return nil, err
	}

selectionLoop:
	for {
		if err := p.writef("escolhas (exemplo: 1=2 2=none): "); err != nil {
			return nil, err
		}
		line, err := p.readLine()
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(line)
		if isCancellation(trimmed) {
			return nil, selectionCanceled()
		}
		if trimmed == "" {
			if err := p.writef("ajuda: informe local=opção; a opção é o número do candidato, none ou all; q/cancel cancela\n"); err != nil {
				return nil, err
			}
			continue
		}

		tokens, err := ParseSelectionTokens(trimmed)
		if err != nil {
			if writeErr := p.writef("seleção inválida: %v\n", err); writeErr != nil {
				return nil, writeErr
			}
			continue
		}
		batch, err := validateTokens(tokens, sites)
		if err != nil {
			if writeErr := p.writef("seleção inválida: %v\n", err); writeErr != nil {
				return nil, writeErr
			}
			continue
		}

		if !batch.complete {
			missing := missingSiteOrdinals(sites, batch.choices)
			for {
				confirmation, err := p.confirmMissingSites(missing)
				if err != nil {
					return nil, err
				}
				switch confirmation {
				case confirmationInvalid:
					continue
				case confirmationNegative:
					continue selectionLoop
				case confirmationAffirmative:
					// Only an affirmative response may synthesize omitted sites.
					for siteNumber := range sites {
						if _, selected := batch.choices[siteNumber+1]; !selected {
							batch.choices[siteNumber+1] = resolve.DispatchChoice{Mode: resolve.ChoiceNone}
						}
					}
				}
				break
			}
		}

		return selectionsInSiteOrder(sites, batch.choices), nil
	}
}

func (p *Picker) renderSites(sites []resolve.DispatchSite) error {
	if err := p.writef("fluxos: %s\n\n", implementationSitesLabel(len(sites))); err != nil {
		return err
	}
	for siteNumber, site := range sites {
		if err := p.writef("[%d] %s - %s:%d\n", siteNumber+1, formatCaller(site), site.Call.File, site.Call.Line); err != nil {
			return err
		}
		if err := p.writef("    %s\n", formatReceiverMethod(site)); err != nil {
			return err
		}
		for candidateNumber, candidate := range site.Candidates {
			if err := p.writef("    %d) %s\n", candidateNumber+1, candidate.ImplementationFQCN); err != nil {
				return err
			}
		}
		if err := p.writef("    none) none (manter terminal)\n"); err != nil {
			return err
		}
		if p.MaxImpls > 0 {
			if err := p.writef("    all) all (máximo %d)\n\n", p.MaxImpls); err != nil {
				return err
			}
		} else if err := p.writef("    all) all (ilimitado)\n\n"); err != nil {
			return err
		}
	}
	return nil
}

func implementationSitesLabel(count int) string {
	if count == 1 {
		return "1 local de implementação ambíguo"
	}
	return fmt.Sprintf("%d locais de implementação ambíguos", count)
}

type confirmationResult uint8

const (
	confirmationAffirmative confirmationResult = iota + 1
	confirmationNegative
	confirmationInvalid
)

func (p *Picker) confirmMissingSites(missing []int) (confirmationResult, error) {
	if err := p.writef("Confirmar none para os sites não informados %s? [sim/nao] ", formatSiteOrdinals(missing)); err != nil {
		return 0, err
	}
	line, err := p.readLine()
	if err != nil {
		return 0, err
	}
	trimmed := strings.TrimSpace(line)
	if isCancellation(trimmed) {
		return 0, selectionCanceled()
	}
	if isAffirmative(trimmed) {
		return confirmationAffirmative, nil
	}
	if isNegative(trimmed) {
		return confirmationNegative, nil
	}
	if err := p.writef("resposta inválida: informe sim/s/yes/y para confirmar ou nao/não/n para recusar.\n"); err != nil {
		return 0, err
	}
	return confirmationInvalid, nil
}

func (p *Picker) readLine() (string, error) {
	line, err := p.Reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return "", selectionCanceled()
		}
		return "", fmt.Errorf("falha ao ler a seleção: %w", err)
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}

func (p *Picker) writef(format string, args ...any) error {
	if p.ErrOut == nil {
		return errors.New("o escritor de stderr do prompt é nulo")
	}
	if _, err := fmt.Fprintf(p.ErrOut, format, args...); err != nil {
		return fmt.Errorf("falha ao escrever o prompt: %w", err)
	}
	return nil
}

func selectionsInSiteOrder(sites []resolve.DispatchSite, choices map[int]resolve.DispatchChoice) []trace.Selection {
	selections := make([]trace.Selection, 0, len(sites))
	for siteNumber, site := range sites {
		selections = append(selections, trace.Selection{
			SiteID: site.ID,
			Choice: choices[siteNumber+1],
		})
	}
	return selections
}

func missingSiteOrdinals(sites []resolve.DispatchSite, choices map[int]resolve.DispatchChoice) []int {
	missing := make([]int, 0, len(sites)-len(choices))
	for siteNumber := range sites {
		if _, selected := choices[siteNumber+1]; !selected {
			missing = append(missing, siteNumber+1)
		}
	}
	return missing
}

func formatSiteOrdinals(ordinals []int) string {
	values := make([]string, 0, len(ordinals))
	for _, ordinal := range ordinals {
		values = append(values, strconv.Itoa(ordinal))
	}
	return "[" + strings.Join(values, ", ") + "]"
}

func formatCaller(site resolve.DispatchSite) string {
	return formatMethod(site.Caller.Method.TypeFQCN, site.Caller.Method.Method, site.Caller.Method.Signature)
}

func formatReceiverMethod(site resolve.DispatchSite) string {
	return formatMethod(site.ReceiverFQCN, site.Method, site.Signature)
}

func formatMethod(typeName, method, signature string) string {
	qualifiedMethod := method
	if typeName != "" && method != "" {
		qualifiedMethod = typeName + "." + method
	} else if typeName != "" {
		qualifiedMethod = typeName
	}
	return qualifiedMethod + signature
}

func isCancellation(value string) bool {
	return strings.EqualFold(value, "q") || strings.EqualFold(value, "cancel")
}

func isAffirmative(value string) bool {
	switch strings.ToLower(value) {
	case "sim", "s", "yes", "y":
		return true
	default:
		return false
	}
}

func isNegative(value string) bool {
	switch strings.ToLower(value) {
	case "nao", "não", "n":
		return true
	default:
		return false
	}
}

func selectionCanceled() error {
	return trace.ErrSelectionCanceled
}
