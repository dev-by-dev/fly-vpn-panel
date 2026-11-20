// Package users implements the quota/device/expiry enforcement engine
// and the /api/users HTTP handlers.
package users

import (
	"time"

	"flypanel/internal/store"
)

// Action is one enforcement step Decide recommends for a user.
// Type is one of: disable, enable, rollover.
type Action struct {
	Type   string
	Reason string
}

// NextBoundary returns the first period boundary strictly after start:
// day → next 00:00 UTC, week → next Monday 00:00 UTC, month → next 1st
// 00:00 UTC, none → start unchanged. A start exactly on a boundary
// yields the following boundary.
func NextBoundary(start time.Time, period string) time.Time {
	s := start.UTC()
	switch period {
	case "day":
		b := dayStart(s)
		if !b.After(s) {
			return b.AddDate(0, 0, 1)
		}
		return b
	case "week":
		d := dayStart(s)
		offset := (int(d.Weekday()) - int(time.Monday) + 7) % 7 // days since Monday
		mon := d.AddDate(0, 0, -offset)
		if !mon.After(s) {
			return mon.AddDate(0, 0, 7)
		}
		return mon
	case "month":
		first := time.Date(s.Year(), s.Month(), 1, 0, 0, 0, 0, time.UTC)
		if !first.After(s) {
			return first.AddDate(0, 1, 0)
		}
		return first
	default: // "none" (and unknown values): unchanged
		return start
	}
}

// periodStart returns the UTC start of the period containing now; the
// zero time for period none (the storage "unset" convention).
func periodStart(now time.Time, period string) time.Time {
	n := now.UTC()
	switch period {
	case "day":
		return dayStart(n)
	case "week":
		offset := (int(n.Weekday()) - int(time.Monday) + 7) % 7
		return dayStart(n).AddDate(0, 0, -offset)
	case "month":
		return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return time.Time{}
	}
}

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// Decide computes the enforcement actions for one user. It is pure: no
// DB writes, no side effects. Persisting the new QuotaResetAt after a
// rollover is the poller's job (internal/stats). Rule order:
//
//  1. expiry <= now and not disabled → disable(expired)
//  2. period != none and now >= NextBoundary → rollover (usage counted
//     toward the quota restarts at zero — the new period just began)
//  3. quota_bytes > 0 and used >= quota_bytes and not disabled → disable(quota)
//  4. disabled by quota and used < quota_bytes → enable
//  5. device_limit > 0 and unique IPs > device_limit and not disabled → disable(device_limit)
//  6. disabled by device_limit and unique IPs <= device_limit → enable
//  7. disabled by expired and expiry > now → enable
//
// Simultaneous reasons: disable beats enable, priority expired > quota
// > device_limit. A manual disable is never auto-enabled.
func Decide(u store.User, usedBytes int64, onlineIPs []string, now time.Time) []Action {
	var actions []Action

	used := usedBytes
	if u.QuotaPeriod != "none" && !now.Before(NextBoundary(u.QuotaResetAt, u.QuotaPeriod)) {
		actions = append(actions, Action{Type: "rollover", Reason: u.QuotaPeriod})
		used = 0
	}

	expired := !u.Expiry.After(now)
	overQuota := u.QuotaBytes > 0 && used >= u.QuotaBytes
	overDevice := u.DeviceLimit > 0 && countUniqueIPs(onlineIPs) > u.DeviceLimit

	if !u.Disabled {
		switch {
		case expired:
			actions = append(actions, Action{Type: "disable", Reason: "expired"})
		case overQuota:
			actions = append(actions, Action{Type: "disable", Reason: "quota"})
		case overDevice:
			actions = append(actions, Action{Type: "disable", Reason: "device_limit"})
		}
		return actions
	}

	if u.DisabledReason == "manual" {
		return actions
	}
	var cleared bool
	switch u.DisabledReason {
	case "quota":
		cleared = !overQuota
	case "device_limit":
		cleared = !overDevice
	case "expired":
		cleared = !expired
	}
	// A still-active disable condition beats an enable.
	if cleared && !expired && !overQuota && !overDevice {
		actions = append(actions, Action{Type: "enable", Reason: u.DisabledReason})
	}
	return actions
}

// countUniqueIPs counts distinct non-empty IPs, so a NAT'd client with
// several connections from one IP counts as one device.
func countUniqueIPs(ips []string) int {
	seen := make(map[string]struct{}, len(ips))
	for _, ip := range ips {
		if ip != "" {
			seen[ip] = struct{}{}
		}
	}
	return len(seen)
}
