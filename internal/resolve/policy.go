package resolve

// DispatchPolicy decides what to do with a complete polymorphic dispatch site.
// The site is built before the policy is invoked so policies can use its stable
// ID, canonical candidate order, and caller/runtime metadata as one unit.
type DispatchPolicy interface {
	Apply(site *DispatchSite) PolicyDecision
}

// PolicyDecision is the result of a dispatch policy. Targets contains both
// concrete targets and candidate-specific terminals. Omitted is the number of
// canonical candidates rejected by a policy limit.
type PolicyDecision struct {
	Targets []ResolvedTarget
	Omitted int
}

// ChoiceMode is the selection mode used by site-specific dispatch choices.
type ChoiceMode string

const (
	ChoiceModeNone     ChoiceMode = "none"
	ChoiceModeSelected ChoiceMode = "selected"
	ChoiceModeAll      ChoiceMode = "all"
)

// Short aliases keep the mode names convenient at call sites while retaining
// the type prefix for discoverability in godoc and completions.
const (
	ChoiceNone     = ChoiceModeNone
	ChoiceSelected = ChoiceModeSelected
	ChoiceAll      = ChoiceModeAll
)

// DispatchChoice selects one implementation, all implementations, or no
// implementation for a dispatch site. ImplementationFQCN is meaningful only
// when Mode is ChoiceModeSelected.
type DispatchChoice struct {
	Mode               ChoiceMode
	ImplementationFQCN string
}

// SitePolicy applies choices keyed by the stable dispatch-site ID and delegates
// unmapped sites to Fallback. MaxImpls applies to ChoiceModeAll.
type SitePolicy struct {
	Choices  map[DispatchSiteID]DispatchChoice
	Fallback DispatchPolicy
	MaxImpls int
}

// TerminalPolicy preserves the M3 behavior: a polymorphic dispatch remains a
// single ambiguous-implementation terminal.
type TerminalPolicy struct{}

func (TerminalPolicy) Apply(site *DispatchSite) PolicyDecision {
	if site == nil {
		return PolicyDecision{}
	}
	terminal := TerminalTarget(
		ResolutionAmbiguousImplementation, site.ReceiverFQCN, site.Method,
		"", site.Call,
		"multiple concrete implementations of "+site.ReceiverFQCN, nil,
	)
	return PolicyDecision{Targets: []ResolvedTarget{terminal}}
}

// AllPolicy fans out over canonical candidates. MaxImpls counts candidates
// before their concrete/terminal result is considered: the first N candidates
// are admitted and every later candidate is omitted.
type AllPolicy struct {
	MaxImpls int
}

func (p AllPolicy) Apply(site *DispatchSite) PolicyDecision {
	if site == nil {
		return PolicyDecision{}
	}
	limit := len(site.Candidates)
	if p.MaxImpls > 0 && p.MaxImpls < limit {
		limit = p.MaxImpls
	}

	decision := PolicyDecision{Targets: make([]ResolvedTarget, 0, limit)}
	for index, candidate := range site.Candidates {
		if index >= limit {
			decision.Omitted++
			continue
		}
		decision.Targets = append(decision.Targets, candidateTarget(*site, candidate))
	}
	return decision
}

// FixedChoiceKind classifies the legacy receiver-based choice used by the
// CLI's --pick-impls option.
type FixedChoiceKind int

const (
	FixedChoiceNone FixedChoiceKind = iota
	FixedChoiceAll
	FixedChoiceExplicit
)

// FixedChoice is the receiver-based compatibility form of a dispatch choice.
type FixedChoice struct {
	Kind  FixedChoiceKind
	Impls []string
}

// FixedPolicy applies receiver-FQCN choices. It remains intentionally
// receiver-based for --pick-impls; its policy entry point is nevertheless the
// same complete DispatchSite used by all other policies.
type FixedPolicy struct {
	Choices  map[string]FixedChoice
	Fallback DispatchPolicy
	MaxImpls int
}

func (p FixedPolicy) Apply(site *DispatchSite) PolicyDecision {
	if site == nil {
		return PolicyDecision{}
	}
	choice, ok := p.Choices[site.ReceiverFQCN]
	if !ok {
		return p.fallback(site)
	}

	switch choice.Kind {
	case FixedChoiceNone:
		return TerminalPolicy{}.Apply(site)
	case FixedChoiceAll:
		return AllPolicy{MaxImpls: p.MaxImpls}.Apply(site)
	case FixedChoiceExplicit:
		wanted := make(map[string]struct{}, len(choice.Impls))
		for _, fqcn := range choice.Impls {
			wanted[fqcn] = struct{}{}
		}
		selected := make([]ImplementationCandidate, 0, len(choice.Impls))
		// Site candidates are already canonical. Preserve that order rather than
		// ordering by the declaration order in Impls.
		for _, candidate := range site.Candidates {
			if _, ok := wanted[candidate.ImplementationFQCN]; ok {
				selected = append(selected, candidate)
			}
		}
		selectedSite := *site
		selectedSite.Candidates = selected
		return AllPolicy{MaxImpls: p.MaxImpls}.Apply(&selectedSite)
	default:
		return p.fallback(site)
	}
}

func (p FixedPolicy) fallback(site *DispatchSite) PolicyDecision {
	if p.Fallback == nil {
		return TerminalPolicy{}.Apply(site)
	}
	return p.Fallback.Apply(site)
}

func (p SitePolicy) Apply(site *DispatchSite) PolicyDecision {
	if site == nil {
		return PolicyDecision{}
	}
	choice, ok := p.Choices[site.ID]
	if !ok {
		return p.fallback(site)
	}

	switch choice.Mode {
	case ChoiceModeNone:
		return TerminalPolicy{}.Apply(site)
	case ChoiceModeSelected:
		for _, candidate := range site.Candidates {
			if candidate.ImplementationFQCN == choice.ImplementationFQCN {
				return PolicyDecision{Targets: []ResolvedTarget{candidateTarget(*site, candidate)}}
			}
		}
		// A coordinator normally validates this before resolution. Keep the
		// policy total nevertheless, and do not inspect unrelated candidates.
		candidate := ImplementationCandidate{
			ImplementationFQCN: choice.ImplementationFQCN,
			Kind:               ResolutionUnresolved,
			Note:               "selected implementation is not a dispatch candidate",
		}
		return PolicyDecision{Targets: []ResolvedTarget{candidateTarget(*site, candidate)}}
	case ChoiceModeAll:
		return AllPolicy{MaxImpls: p.MaxImpls}.Apply(site)
	default:
		return p.fallback(site)
	}
}

func (p SitePolicy) fallback(site *DispatchSite) PolicyDecision {
	if p.Fallback == nil {
		return TerminalPolicy{}.Apply(site)
	}
	return p.Fallback.Apply(site)
}

func candidateTarget(site DispatchSite, candidate ImplementationCandidate) ResolvedTarget {
	if candidate.Kind == ResolutionConcrete && candidate.Target.Method != "" {
		return ConcreteTarget(ExecutionKey{
			Method:          candidate.Target,
			RuntimeTypeFQCN: candidate.ImplementationFQCN,
		})
	}

	kind := candidate.Kind
	if kind == ResolutionConcrete {
		kind = ResolutionUnresolved
	}
	return TerminalTarget(
		kind, candidate.ImplementationFQCN, site.Method, "",
		site.Call, candidate.Note, nil,
	)
}
