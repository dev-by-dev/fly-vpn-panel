package users

import (
	"context"
	"errors"
	"fmt"

	"flypanel/internal/store"
)

// MemberSync is the xray membership surface the users package needs. It
// is satisfied by *xrayclient.Client; AddUser is idempotent
// (already-exists → nil).
type MemberSync interface {
	AddUser(inboundTag, uuid, email string) error
	RemoveUser(inboundTag, email string) error
}

// SyncMembership reconciles every enabled inbound against the active
// user set: users in active with Disabled cleared are added
// (idempotently), users with Disabled set are removed, so manual and
// enforcement disables take effect immediately instead of waiting for
// the poller. The username is the xray email (the stats key). A user
// deleted from the store is absent from active and must be removed by
// the caller (see Handlers.Delete). An error on one inbound or user
// does not stop the rest; failures are joined with errors.Join.
func SyncMembership(ctx context.Context, c MemberSync, inbounds []store.Inbound, active []store.User) error {
	var errs []error
	for _, in := range inbounds {
		if !in.Enabled {
			continue
		}
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("inbound %q: %w", in.Tag, err))
			break
		}
		for _, u := range active {
			if u.Disabled {
				if err := c.RemoveUser(in.Tag, u.Username); err != nil {
					errs = append(errs, fmt.Errorf("remove %q on %q: %w", u.Username, in.Tag, err))
				}
				continue
			}
			if err := c.AddUser(in.Tag, u.UUID, u.Username); err != nil {
				errs = append(errs, fmt.Errorf("add %q on %q: %w", u.Username, in.Tag, err))
			}
		}
	}
	return errors.Join(errs...)
}
