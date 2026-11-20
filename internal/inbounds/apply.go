// Package inbounds serves the inbound management and routing-editor API
// and applies generated xray configs atomically.
package inbounds

import (
	"fmt"

	"flypanel/internal/config"
	"flypanel/internal/store"
	"flypanel/internal/xray"
)

// apiPort is the dokodemo-door port xray's gRPC API listens on (spec).
const apiPort = 10811

// defaultRouting is stored/returned when no routing has been set yet.
const defaultRouting = `{"domainStrategy":"AsIs","rules":[]}`

// routingKey is the settings key holding the routing editor JSON.
const routingKey = "routing"

// Applier regenerates the full xray config from the store and applies it:
// generate → validate → atomic write → restart. A validate or write
// failure leaves the current config file and running process untouched.
type Applier struct {
	Store    *store.Store
	Cfg      *config.Config
	Mgr      *xray.Manager
	Validate func(binPath string, cfg []byte) error
}

// NewApplier returns an Applier using the real xray.Validate; tests
// inject a fake by overwriting Validate.
func NewApplier(st *store.Store, cfg *config.Config, mgr *xray.Manager) *Applier {
	return &Applier{Store: st, Cfg: cfg, Mgr: mgr, Validate: xray.Validate}
}

// ConfigPath is the on-disk location of the generated xray config.
func (a *Applier) ConfigPath() string { return a.Cfg.DataDir + "/xray.json" }

// Apply regenerates and applies the config with the given routing JSON
// ("" falls back to xray's default routing). Expiry/quota enforcement is
// the users poller's job; here only the Disabled flag filters users.
func (a *Applier) Apply(routingJSON string) error {
	inbounds, err := a.Store.InboundListEnabled()
	if err != nil {
		return err
	}
	users, err := a.Store.UserList()
	if err != nil {
		return err
	}
	active := make([]store.User, 0, len(users))
	for _, u := range users {
		if !u.Disabled {
			active = append(active, u)
		}
	}
	cfg, err := xray.GenerateConfig(inbounds, active, routingJSON, a.Cfg.EgressIP, apiPort)
	if err != nil {
		return err
	}
	if err := a.Validate(a.Cfg.XrayBin, cfg); err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	if err := xray.ApplyAtomic(a.ConfigPath(), cfg); err != nil {
		return err
	}
	return a.Mgr.Restart()
}
