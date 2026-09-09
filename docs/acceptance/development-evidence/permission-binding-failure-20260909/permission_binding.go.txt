package app

import (
	"errors"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/permissions"
)

// PermissionState is a consistent snapshot. Journals remain owned by the creator
// until all application operations and permission workers have joined.
type PermissionState struct {
	Authority *permissions.Authority
	Legacy    permissions.LegacyMigration
	Digest    string
}

func (c *Controller) PermissionState() PermissionState {
	if c == nil {
		return PermissionState{}
	}
	if c.ReadPermissions != nil {
		return c.ReadPermissions()
	}
	return PermissionState{Authority: c.Authority, Legacy: c.LegacyPermissions}
}
func (c *Controller) preparePermissions(p config.ModelProfile) (func(), error) {
	if c.PreparePermissions != nil {
		commit, err := c.PreparePermissions(p)
		if err != nil {
			return nil, err
		}
		if commit == nil {
			return nil, errors.New("permission preparation returned no commit function")
		}
		return commit, nil
	}
	if c.PermissionState().Authority != nil {
		return nil, errors.New("configuration switching requires scoped authority rebinding")
	}
	return func() {}, nil
}
