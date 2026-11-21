package stats

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"flypanel/internal/store"
	"flypanel/internal/users"
)

// fakeStats implements StatsSource with canned xray state.
type fakeStats struct {
	counters map[string][2]int64
	online   []string
	ips      map[string][]string
}

func (f *fakeStats) QueryUserStats(email string) (int64, int64, error) {
	c, ok := f.counters[email]
	if !ok {
		return 0, 0, nil
	}
	return c[0], c[1], nil
}

func (f *fakeStats) OnlineUsers() ([]string, error) { return f.online, nil }

func (f *fakeStats) OnlineIPs(email string) ([]string, error) { return f.ips[email], nil }

// fakeSync implements MemberSync and records calls as "tag|email".
type fakeSync struct {
	added   map[string]bool
	removed map[string]bool
}

func newFakeSync() *fakeSync {
	return &fakeSync{added: map[string]bool{}, removed: map[string]bool{}}
}

func (f *fakeSync) AddUser(tag, uuid, email string) error {
	f.added[tag+"|"+email] = true
	return nil
}

func (f *fakeSync) RemoveUser(tag, email string) error {
	f.removed[tag+"|"+email] = true
	return nil
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func mkUser(t *testing.T, st *store.Store, name string, mutate func(*store.User)) *store.User {
	t.Helper()
	u := &store.User{
		Username:    name,
		UUID:        "11111111-2222-4333-8444-555555555555",
		SubToken:    "token-" + name,
		Expiry:      time.Now().Add(24 * time.Hour),
		QuotaPeriod: "none",
		CreatedAt:   time.Now(),
	}
	if mutate != nil {
		mutate(u)
	}
	if err := st.UserCreate(u); err != nil {
		t.Fatalf("create user %q: %v", name, err)
	}
	return u
}

func mkInbound(t *testing.T, st *store.Store, tag string) {
	t.Helper()
	err := st.InboundCreate(&store.Inbound{
		Tag: tag, Enabled: true, ConfigJSON: "{}", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create inbound %q: %v", tag, err)
	}
}

func setCursor(t *testing.T, st *store.Store, username string, up, down int64) {
	t.Helper()
	val := fmt.Sprintf(`{"up":%d,"down":%d}`, up, down)
	if err := st.SettingSet("stats_cursor_"+username, val); err != nil {
		t.Fatalf("set cursor: %v", err)
	}
}

func mustUser(t *testing.T, st *store.Store, name string) *store.User {
	t.Helper()
	u, err := st.UserGetByUsername(name)
	if err != nil {
		t.Fatalf("get user %q: %v", name, err)
	}
	return u
}

func TestDeltaNormal(t *testing.T) {
	st := openStore(t)
	mkUser(t, st, "alice@example.com", nil)
	setCursor(t, st, "alice@example.com", 100, 100)
	fs := &fakeStats{counters: map[string][2]int64{"alice@example.com": {150, 120}}}
	p := NewPoller(st, fs, newFakeSync(), 30*time.Second)

	p.tick(context.Background())

	up, down, err := st.SamplesSince("alice@example.com", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if up != 50 || down != 20 {
		t.Fatalf("delta sample = (%d, %d), want (50, 20)", up, down)
	}
}

func TestDeltaReset(t *testing.T) {
	st := openStore(t)
	mkUser(t, st, "alice@example.com", nil)
	// xray restarted: cumulative counters dropped below the cursor.
	setCursor(t, st, "alice@example.com", 500, 500)
	fs := &fakeStats{counters: map[string][2]int64{"alice@example.com": {30, 30}}}
	p := NewPoller(st, fs, newFakeSync(), 30*time.Second)

	p.tick(context.Background())

	up, down, err := st.SamplesSince("alice@example.com", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if up != 30 || down != 30 {
		t.Fatalf("delta sample = (%d, %d), want (30, 30) — not (−470, −470)", up, down)
	}
}

func TestCursorSurvivesPanelRestart(t *testing.T) {
	st := openStore(t)
	mkUser(t, st, "alice@example.com", nil)
	fs := &fakeStats{counters: map[string][2]int64{"alice@example.com": {150, 60}}}

	// First panel lifetime: first poll, no cursor → delta = cur.
	p1 := NewPoller(st, fs, newFakeSync(), 30*time.Second)
	p1.tick(context.Background())
	up, down, _ := st.SamplesSince("alice@example.com", time.Time{})
	if up != 150 || down != 60 {
		t.Fatalf("first poll = (%d, %d), want (150, 60)", up, down)
	}

	// Fresh Poller on the same store must read the persisted cursor and
	// not double-count the same cumulative values.
	p2 := NewPoller(st, fs, newFakeSync(), 30*time.Second)
	p2.tick(context.Background())

	up, down, err := st.SamplesSince("alice@example.com", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if up != 150 || down != 60 {
		t.Fatalf("after restart = (%d, %d), want (150, 60) — no double counting", up, down)
	}
	cur, err := st.SettingGet("stats_cursor_alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"up":150,"down":60}`; cur != want {
		t.Fatalf("persisted cursor = %s, want %s", cur, want)
	}
}

func TestEnforcementIntegration(t *testing.T) {
	st := openStore(t)
	mkInbound(t, st, "in1")
	mkUser(t, st, "carol@example.com", func(u *store.User) { u.QuotaBytes = 100 })
	// Quota already exceeded: 120 bytes used.
	err := st.SampleInsert("carol@example.com", time.Now().Add(-time.Hour), 60, 60)
	if err != nil {
		t.Fatal(err)
	}
	sync := newFakeSync()
	p := NewPoller(st, &fakeStats{}, sync, 30*time.Second)

	p.tick(context.Background())

	u := mustUser(t, st, "carol@example.com")
	if !u.Disabled || u.DisabledReason != "quota" {
		t.Fatalf("user = (disabled=%v, reason=%q), want (true, \"quota\")", u.Disabled, u.DisabledReason)
	}
	if !sync.removed["in1|carol@example.com"] {
		t.Fatal("RemoveUser not called for disabled user")
	}
	if sync.added["in1|carol@example.com"] {
		t.Fatal("AddUser must not be called while disabling")
	}

	// Rollover: quota period elapsed → poller persists the new reset_at,
	// Decide emits enable, user is re-added to every enabled inbound.
	u.QuotaPeriod = "day"
	past := time.Now().Add(-48 * time.Hour)
	u.QuotaResetAt = past
	if err := st.UserUpdate(u); err != nil {
		t.Fatal(err)
	}

	p.tick(context.Background())

	u = mustUser(t, st, "carol@example.com")
	if u.Disabled {
		t.Fatal("user should be re-enabled after quota rollover")
	}
	if want := users.NextBoundary(past, "day"); !u.QuotaResetAt.Equal(want) {
		t.Fatalf("quota_reset_at = %v, want %v", u.QuotaResetAt, want)
	}
	if !sync.added["in1|carol@example.com"] {
		t.Fatal("AddUser not called after enable")
	}
}

func TestOnlineDevices(t *testing.T) {
	st := openStore(t)
	mkUser(t, st, "alice@example.com", nil)
	mkUser(t, st, "bob@example.com", nil)
	old := time.Now().Add(-2 * time.Hour)
	// Alice has a device from an earlier sighting; bob is fully offline.
	if err := st.DeviceUpsert("alice@example.com", "203.0.113.7", store.DeviceMeta{HWID: "203.0.113.7"}, old); err != nil {
		t.Fatal(err)
	}
	if err := st.DeviceUpsert("alice@example.com", "203.0.113.8", store.DeviceMeta{HWID: "203.0.113.8"}, old); err != nil {
		t.Fatal(err)
	}
	if err := st.DeviceUpsert("bob@example.com", "198.51.100.9", store.DeviceMeta{HWID: "198.51.100.9"}, old); err != nil {
		t.Fatal(err)
	}
	fs := &fakeStats{
		online: []string{"alice@example.com"},
		ips: map[string][]string{
			"alice@example.com": {"203.0.113.7", "203.0.113.8"},
		},
	}
	p := NewPoller(st, fs, newFakeSync(), 30*time.Second)

	p.tick(context.Background())

	devs, err := st.DevicesForUser("alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 2 {
		t.Fatalf("alice devices = %d, want 2", len(devs))
	}
	for _, d := range devs {
		if d.LastSeen.Before(time.Now().Add(-time.Minute)) {
			t.Errorf("device %s last_seen = %v, want refreshed", d.IP, d.LastSeen)
		}
		if d.IP == "203.0.113.7" && !d.FirstSeen.Equal(old.Truncate(time.Second)) {
			t.Errorf("first_seen = %v, want immutable %v", d.FirstSeen, old)
		}
	}
	bdevs, err := st.DevicesForUser("bob@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(bdevs) != 1 {
		t.Fatalf("bob devices = %d, want 1", len(bdevs))
	}
	if !bdevs[0].LastSeen.Equal(old.Truncate(time.Second)) {
		t.Fatalf("offline user's device was touched: last_seen = %v, want %v",
			bdevs[0].LastSeen, old)
	}
}

func TestPrune(t *testing.T) {
	st := openStore(t)
	mkUser(t, st, "dave@example.com", nil)
	now := time.Now().UTC()
	// One sample and one device past retention, one of each fresh.
	if err := st.SampleInsert("dave@example.com", now.AddDate(0, 0, -91), 100, 100); err != nil {
		t.Fatal(err)
	}
	if err := st.SampleInsert("dave@example.com", now.Add(-time.Hour), 5, 5); err != nil {
		t.Fatal(err)
	}
	if err := st.DeviceUpsert("dave@example.com", "203.0.113.20", store.DeviceMeta{HWID: "203.0.113.20"}, now.AddDate(0, 0, -31)); err != nil {
		t.Fatal(err)
	}
	if err := st.DeviceUpsert("dave@example.com", "203.0.113.21", store.DeviceMeta{HWID: "203.0.113.21"}, now); err != nil {
		t.Fatal(err)
	}
	p := NewPoller(st, &fakeStats{}, newFakeSync(), 30*time.Second)

	p.tick(context.Background()) // first tick prunes

	up, down, err := st.SamplesSince("dave@example.com", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if up != 5 || down != 5 {
		t.Fatalf("samples after prune = (%d, %d), want (5, 5)", up, down)
	}
	devs, err := st.DevicesForUser("dave@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].IP != "203.0.113.21" {
		t.Fatalf("devices after prune = %+v, want only 203.0.113.21", devs)
	}
}

// hookStats wraps fakeStats and runs a one-shot hook inside the first
// QueryUserStats call — simulating a concurrent admin PATCH landing
// mid-tick, after the tick-start user snapshot was taken.
type hookStats struct {
	fakeStats
	hook func()
}

func (h *hookStats) QueryUserStats(email string) (int64, int64, error) {
	if h.hook != nil {
		hook := h.hook
		h.hook = nil
		hook()
	}
	return h.fakeStats.QueryUserStats(email)
}

// TestRolloverPreservesConcurrentPatch: the rollover must write only
// quota_reset_at (targeted update). A full-row UserUpdate from the
// tick-start snapshot clobbers concurrent admin PATCHes, because the
// per-user RPC steps take seconds.
func TestRolloverPreservesConcurrentPatch(t *testing.T) {
	st := openStore(t)
	past := time.Now().Add(-48 * time.Hour)
	mkUser(t, st, "iris@example.com", func(u *store.User) {
		u.QuotaPeriod = "day"
		u.QuotaResetAt = past
		u.QuotaBytes = 100
	})

	fs := &hookStats{fakeStats: fakeStats{}}
	fs.hook = func() {
		// Admin PATCH racing the poll tick: quota raised mid-tick.
		if _, err := st.DB().Exec(
			`UPDATE users SET quota_bytes = 999 WHERE username = ?`, "iris@example.com"); err != nil {
			t.Errorf("concurrent patch: %v", err)
		}
	}
	p := NewPoller(st, fs, newFakeSync(), 30*time.Second)

	p.tick(context.Background())

	u := mustUser(t, st, "iris@example.com")
	if u.QuotaBytes != 999 {
		t.Fatalf("quota_bytes after rollover = %d, want 999 (concurrent PATCH must survive)", u.QuotaBytes)
	}
	if want := users.NextBoundary(past, "day"); !u.QuotaResetAt.Equal(want) {
		t.Fatalf("quota_reset_at = %v, want %v", u.QuotaResetAt, want)
	}
}

func TestTrafficBuckets(t *testing.T) {
	st := openStore(t)
	mkUser(t, st, "erin@example.com", nil)
	mkUser(t, st, "frank@example.com", nil)
	now := time.Now().UTC()
	ins := func(name string, ts time.Time, up int64) {
		t.Helper()
		if err := st.SampleInsert(name, ts, up, 0); err != nil {
			t.Fatal(err)
		}
	}
	ins("erin@example.com", now.Add(-2*time.Hour), 10)   // day + week + month
	ins("erin@example.com", now.AddDate(0, 0, -3), 20)   // week + month
	ins("erin@example.com", now.AddDate(0, 0, -20), 30)  // month
	ins("erin@example.com", now.AddDate(0, 0, -31), 999) // outside every range
	ins("frank@example.com", now.Add(-2*time.Hour), 7)   // for the all-users sum

	sumUp := func(bs []store.Bucket) int64 {
		var s int64
		for _, b := range bs {
			s += b.Up
		}
		return s
	}

	day, err := Traffic(st, "erin@example.com", "day")
	if err != nil {
		t.Fatal(err)
	}
	if len(day) != 1 || day[0].Up != 10 {
		t.Fatalf("day buckets = %+v, want one bucket with Up=10", day)
	}
	if day[0].TS.Minute() != 0 || day[0].TS.Second() != 0 {
		t.Fatalf("day bucket ts %v not hour-aligned", day[0].TS)
	}
	week, err := Traffic(st, "erin@example.com", "week")
	if err != nil {
		t.Fatal(err)
	}
	if sumUp(week) != 30 {
		t.Fatalf("week sum = %d, want 30 (10+20)", sumUp(week))
	}
	for _, b := range week {
		if b.TS.Hour() != 0 {
			t.Fatalf("week bucket ts %v not day-aligned", b.TS)
		}
	}
	month, err := Traffic(st, "erin@example.com", "month")
	if err != nil {
		t.Fatal(err)
	}
	if sumUp(month) != 60 {
		t.Fatalf("month sum = %d, want 60 (10+20+30)", sumUp(month))
	}
	all, err := Traffic(st, "", "day")
	if err != nil {
		t.Fatal(err)
	}
	if sumUp(all) != 17 {
		t.Fatalf("all-users day sum = %d, want 17 (erin 10 + frank 7)", sumUp(all))
	}
	if _, err := Traffic(st, "erin@example.com", "year"); !errors.Is(err, ErrBadRange) {
		t.Fatalf("invalid range err = %v, want ErrBadRange", err)
	}
}

func TestSummary(t *testing.T) {
	st := openStore(t)
	mkUser(t, st, "gina@example.com", nil)
	now := time.Now().UTC()
	if err := st.SampleInsert("gina@example.com", now.Add(-time.Hour), 10, 20); err != nil {
		t.Fatal(err)
	}
	if err := st.DeviceUpsert("gina@example.com", "203.0.113.30", store.DeviceMeta{HWID: "203.0.113.30"}, now); err != nil {
		t.Fatal(err)
	}
	if err := st.DeviceUpsert("gina@example.com", "203.0.113.31", store.DeviceMeta{HWID: "203.0.113.31"}, now); err != nil {
		t.Fatal(err)
	}
	// Stale device outside the online window must not count.
	if err := st.DeviceUpsert("gina@example.com", "203.0.113.32", store.DeviceMeta{HWID: "203.0.113.32"}, now.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}

	up, down, online, err := Summary(st, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if up != 10 || down != 20 {
		t.Fatalf("totals = (%d, %d), want (10, 20)", up, down)
	}
	if online != 1 {
		t.Fatalf("online = %d, want 1 (distinct usernames)", online)
	}
}

// TestSummaryInterval: the online window follows 2× the live poll
// interval, not a hard-coded 60s — a 150s interval must count a device
// seen 3 minutes ago as online.
func TestSummaryInterval(t *testing.T) {
	st := openStore(t)
	mkUser(t, st, "hana@example.com", nil)
	now := time.Now().UTC()
	if err := st.DeviceUpsert("hana@example.com", "203.0.113.40", store.DeviceMeta{HWID: "203.0.113.40"}, now.Add(-3*time.Minute)); err != nil {
		t.Fatal(err)
	}

	// 150s interval → 300s window: the 3-minute-old sighting is online.
	_, _, online, err := Summary(st, 150*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if online != 1 {
		t.Fatalf("online(150s interval) = %d, want 1 (within the 2×150s window)", online)
	}
	// Short interval → narrow window: the same sighting is offline.
	_, _, online, err = Summary(st, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if online != 0 {
		t.Fatalf("online(30s interval) = %d, want 0 (outside the 60s window)", online)
	}
}

func TestRunCancel(t *testing.T) {
	st := openStore(t)
	p := NewPoller(st, &fakeStats{}, newFakeSync(), time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return promptly after ctx cancel")
	}
}
