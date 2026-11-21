// Package stats polls xray per-user traffic counters into the store,
// wires the enforcement engine to live data, exposes traffic aggregates
// and performs the daily SQLite backup.
package stats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"flypanel/internal/store"
	"flypanel/internal/users"
)

// StatsSource is the xray stats surface the poller needs. It is
// satisfied by *xrayclient.Client.
type StatsSource interface {
	QueryUserStats(email string) (up, down int64, err error)
	OnlineUsers() ([]string, error)
	OnlineIPs(email string) ([]string, error)
}

// MemberSync is the xray inbound membership surface. It is satisfied
// by *xrayclient.Client.
type MemberSync interface {
	AddUser(tag, uuid, email string) error
	RemoveUser(tag, email string) error
}

const (
	defaultInterval = 30 * time.Second
	pruneEvery      = time.Hour
	samplesKeep     = 90 * 24 * time.Hour
	devicesKeep     = 30 * 24 * time.Hour
)

// ErrBadRange is returned by Traffic for an unknown range name.
var ErrBadRange = errors.New("stats: invalid range (want day, week or month)")

// Poller periodically pulls xray counters, records per-tick deltas,
// enforces quotas/devices/expiry and reconciles inbound membership.
type Poller struct {
	st   *store.Store
	src  StatsSource
	sync MemberSync

	mu       sync.Mutex // guards interval only
	interval time.Duration

	// lastPrune is touched only by tick, which runs on one goroutine.
	lastPrune time.Time
}

// NewPoller builds a poller. A non-positive interval falls back to
// defaultInterval.
func NewPoller(st *store.Store, src StatsSource, sync MemberSync, interval time.Duration) *Poller {
	if interval <= 0 {
		interval = defaultInterval
	}
	return &Poller{st: st, src: src, sync: sync, interval: interval}
}

// SetInterval changes the poll interval; it takes effect after the
// next tick completes.
func (p *Poller) SetInterval(d time.Duration) {
	if d <= 0 {
		return
	}
	p.mu.Lock()
	p.interval = d
	p.mu.Unlock()
}

func (p *Poller) currentInterval() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.interval
}

// Run polls immediately, then once per interval, until ctx is
// cancelled. Cancellation is checked between ticks and between
// per-user RPC batches, because the xray client's own RPCs use fixed
// background contexts.
func (p *Poller) Run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	p.tick(ctx)
	for {
		timer := time.NewTimer(p.currentInterval())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		p.tick(ctx)
	}
}

// tick runs one poll cycle: counter deltas → samples, online devices,
// enforcement, membership reconcile, hourly prune.
func (p *Poller) tick(ctx context.Context) {
	now := time.Now().UTC()
	userList, err := p.st.UserList()
	if err != nil {
		log.Printf("stats: user list: %v", err)
		return
	}

	// 1. Counter deltas; the cursor lives in settings, so it survives
	// panel restarts and never double-counts.
	for _, u := range userList {
		if ctx.Err() != nil {
			return
		}
		curUp, curDown, err := p.src.QueryUserStats(u.Username)
		if err != nil {
			log.Printf("stats: query %q: %v", u.Username, err)
			continue
		}
		cUp, cDown, err := readCursor(p.st, u.Username)
		if err != nil {
			log.Printf("stats: cursor %q: %v", u.Username, err)
			continue
		}
		dUp, dDown := counterDelta(curUp, cUp), counterDelta(curDown, cDown)
		if err := writeCursor(p.st, u.Username, curUp, curDown); err != nil {
			log.Printf("stats: cursor %q: %v", u.Username, err)
			continue
		}
		if dUp == 0 && dDown == 0 {
			continue // idle users do not grow the samples table
		}
		if err := p.st.SampleInsert(u.Username, now, dUp, dDown); err != nil {
			log.Printf("stats: sample %q: %v", u.Username, err)
		}
	}

	// 2. Online users → device sightings. Users absent from the list
	// are left alone so last_seen ages naturally.
	ipsByUser := make(map[string][]string)
	ipsFailed := make(map[string]bool) // OnlineIPs errored for these users
	onlineKnown := true
	online, err := p.src.OnlineUsers()
	if err != nil {
		log.Printf("stats: online users: %v (enforcement skipped this tick)", err)
		onlineKnown = false
	}
	if onlineKnown {
		for _, name := range online {
			if ctx.Err() != nil {
				return
			}
			ips, err := p.src.OnlineIPs(name)
			if err != nil {
				log.Printf("stats: online ips %q: %v (enforcement for %q skipped)", name, err, name)
				ipsFailed[name] = true
				continue
			}
			ipsByUser[name] = ips
			for _, ip := range ips {
				if err := p.st.DeviceUpsert(name, ip, store.DeviceMeta{}, now); err != nil {
					log.Printf("stats: device %q/%q: %v", name, ip, err)
				}
			}
			// За NAT xray видит только один IP на юзера: держим онлайн
			// все его устройства, пока он подключён.
			if len(ips) > 0 {
				if err := p.st.DeviceTouchUser(name, now); err != nil {
					log.Printf("stats: device touch %q: %v", name, err)
				}
			}
		}
	}

	// 3. Enforcement. Users whose online state is unknown are skipped
	// to avoid false device-limit enables on transient RPC failures.
	inbounds, err := p.st.InboundListEnabled()
	if err != nil {
		log.Printf("stats: inbound list: %v", err)
		inbounds = nil
	}
	if onlineKnown {
		for _, u := range userList {
			if ctx.Err() != nil {
				return
			}
			if ipsFailed[u.Username] {
				continue
			}
			// Zero QuotaResetAt (period none) → SamplesSince returns
			// lifetime usage.
			up, down, err := p.st.SamplesSince(u.Username, u.QuotaResetAt)
			if err != nil {
				log.Printf("stats: samples since %q: %v", u.Username, err)
				continue
			}
			cur := u
			for _, a := range users.Decide(cur, up+down, ipsByUser[u.Username], now) {
				if err := p.apply(&cur, a, inbounds); err != nil {
					log.Printf("stats: apply %q %q: %v", u.Username, a.Type, err)
				}
			}
		}
	}

	// 4. Membership reconcile over the full user list (removals cover
	// disabled users; deleted users are handled by the API layer).
	if ctx.Err() != nil {
		return
	}
	if fresh, err := p.st.UserList(); err != nil {
		log.Printf("stats: user list for sync: %v", err)
	} else if err := users.SyncMembership(ctx, p.sync, inbounds, fresh); err != nil {
		log.Printf("stats: membership sync: %v", err)
	}

	// 5. Hourly prune, not every tick.
	if p.lastPrune.IsZero() || now.Sub(p.lastPrune) >= pruneEvery {
		if err := p.st.PruneSamples(now.Add(-samplesKeep)); err != nil {
			log.Printf("stats: prune samples: %v", err)
		}
		if err := p.st.PruneDevices(now.Add(-devicesKeep)); err != nil {
			log.Printf("stats: prune devices: %v", err)
		}
		p.lastPrune = now
	}
}

// apply persists one Decide action and mirrors it in xray membership.
// Decide emits rollover before disable/enable, and the rollover branch
// writes only quota_reset_at (targeted update), so it never rewrites a
// disable applied earlier or a concurrent admin PATCH.
func (p *Poller) apply(u *store.User, a users.Action, inbounds []store.Inbound) error {
	switch a.Type {
	case "rollover":
		u.QuotaResetAt = users.NextBoundary(u.QuotaResetAt, u.QuotaPeriod)
		return p.st.UserSetQuotaResetAt(u.ID, u.QuotaResetAt)
	case "disable":
		if err := p.st.UserSetDisabled(u.ID, true, a.Reason); err != nil {
			return err
		}
		for _, in := range inbounds {
			if err := p.sync.RemoveUser(in.Tag, u.Username); err != nil {
				return err
			}
		}
		return nil
	case "enable":
		if err := p.st.UserSetDisabled(u.ID, false, ""); err != nil {
			return err
		}
		for _, in := range inbounds {
			if err := p.sync.AddUser(in.Tag, u.UUID, u.Username); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("unknown action %q", a.Type)
}

// cursor is the persisted last-seen cumulative counter pair.
type cursor struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

func cursorKey(username string) string { return "stats_cursor_" + username }

// counterDelta returns cur−cursor, or cur when xray restarted and the
// counter dropped below the cursor.
func counterDelta(cur, c int64) int64 {
	if cur < c {
		return cur
	}
	return cur - c
}

func readCursor(st *store.Store, username string) (int64, int64, error) {
	val, err := st.SettingGet(cursorKey(username))
	if errors.Is(err, store.ErrNotFound) {
		return 0, 0, nil // first poll: delta = cur − 0
	}
	if err != nil {
		return 0, 0, err
	}
	var c cursor
	if err := json.Unmarshal([]byte(val), &c); err != nil {
		return 0, 0, fmt.Errorf("parse cursor: %w", err)
	}
	return c.Up, c.Down, nil
}

func writeCursor(st *store.Store, username string, up, down int64) error {
	val, err := json.Marshal(cursor{Up: up, Down: down})
	if err != nil {
		return err
	}
	return st.SettingSet(cursorKey(username), string(val))
}

// Traffic returns summed traffic buckets for username over the range:
// day → hourly buckets for the last 24h, week → daily buckets for the
// last 7 days, month → daily buckets for the last 30 days. username ""
// sums across all users. TrafficSeries filters literally on username,
// so the all-users view is aggregated here, per user.
func Traffic(st *store.Store, username, rng string) ([]store.Bucket, error) {
	now := time.Now().UTC()
	var from time.Time
	var bucket string
	switch rng {
	case "day":
		from, bucket = now.Add(-24*time.Hour), "hour"
	case "week":
		from, bucket = now.AddDate(0, 0, -7), "day"
	case "month":
		from, bucket = now.AddDate(0, 0, -30), "day"
	default:
		return nil, ErrBadRange
	}
	if username != "" {
		return st.TrafficSeries(username, from, now, bucket)
	}
	userList, err := st.UserList()
	if err != nil {
		return nil, err
	}
	totals := make(map[time.Time]store.Bucket)
	for _, u := range userList {
		bs, err := st.TrafficSeries(u.Username, from, now, bucket)
		if err != nil {
			return nil, err
		}
		for _, b := range bs {
			t := totals[b.TS]
			t.TS = b.TS
			t.Up += b.Up
			t.Down += b.Down
			totals[b.TS] = t
		}
	}
	out := make([]store.Bucket, 0, len(totals))
	for _, b := range totals {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TS.Before(out[j].TS) })
	return out, nil
}

// Summary returns lifetime traffic totals and the online user count.
// Online = a device sighting within 2×interval (two poll intervals),
// the same window /api/stats/online uses; the caller passes the live
// poll interval so a longer interval does not undercount.
func Summary(st *store.Store, interval time.Duration) (totalUp, totalDown int64, onlineCount int, err error) {
	userList, err := st.UserList()
	if err != nil {
		return 0, 0, 0, err
	}
	for _, u := range userList {
		up, down, err := st.SamplesSince(u.Username, time.Time{})
		if err != nil {
			return 0, 0, 0, err
		}
		totalUp += up
		totalDown += down
	}
	devices, err := st.DevicesOnline(time.Now().UTC().Add(-2 * interval))
	if err != nil {
		return 0, 0, 0, err
	}
	seen := make(map[string]struct{}, len(devices))
	for _, d := range devices {
		seen[d.Username] = struct{}{}
	}
	return totalUp, totalDown, len(seen), nil
}
