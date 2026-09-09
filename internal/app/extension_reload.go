package app

import (
	"context"
	"errors"
	"github.com/sausheong/hand/internal/extensions"
)

func (h *ExtensionHost) ReloadFile(ctx context.Context, path, digest string) (extensions.ReloadReport, error) {
	if h.closing.Load() {
		return extensions.ReloadReport{}, errors.New("extension host closed")
	}
	operation, release, err := h.controller.owner().reserve(ctx, Running)
	if err != nil {
		return extensions.ReloadReport{}, err
	}
	defer release()
	if h.workspace == "" {
		return extensions.ReloadReport{}, errors.New("reviewed host reload is unavailable for this factory")
	}
	selected, err := ReadExtensionStartup(path, digest)
	if err != nil {
		return extensions.ReloadReport{}, err
	}
	identities := map[string]string{}
	for name, identity := range h.identities {
		identities[name] = identity
	}
	for _, review := range selected.Reviews {
		if review.Workspace != h.workspace {
			return extensions.ReloadReport{}, errors.New("extension review belongs to another workspace")
		}
		identity, ok := selected.Identities[review.Specification.Name]
		if !ok {
			return extensions.ReloadReport{}, errors.New("package identity missing")
		}
		if previous, exists := identities[review.Specification.Name]; exists && previous != identity {
			return extensions.ReloadReport{}, errors.New("extension name cannot change package identity during this session")
		}
		identities[review.Specification.Name] = identity
	}
	if len(identities) > 256 {
		return extensions.ReloadReport{}, errors.New("session extension identity limit reached")
	}
	factory, err := reviewedExtensionFactory(selected, h.containerBoundary)
	if err != nil {
		return extensions.ReloadReport{}, err
	}
	bound, err := h.bindFactory(factory, selected.Identities)
	if err != nil {
		return extensions.ReloadReport{}, err
	}
	specs := make([]extensions.Specification, len(selected.Reviews))
	for i, review := range selected.Reviews {
		specs[i] = review.Specification
	}
	report, err := h.manager.ReloadWithFactory(operation, specs, bound)
	if report.Committed {
		h.identities = identities
	}
	return report, err
}
