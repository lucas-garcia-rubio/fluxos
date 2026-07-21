package trace

import (
	"errors"

	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

// Selection is one site-specific implementation choice returned by an
// ImplementationSelector.
type Selection struct {
	SiteID resolve.DispatchSiteID
	Choice resolve.DispatchChoice
}

// ImplementationSelector chooses exactly one choice for every site in the
// frontier passed to Select.
type ImplementationSelector interface {
	Select([]resolve.DispatchSite) ([]Selection, error)
}

// ErrSelectionCanceled indicates that selection was deliberately canceled.
var ErrSelectionCanceled = errors.New("implementation selection canceled")
