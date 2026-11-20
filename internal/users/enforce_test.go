package users

import (
	"context"
	"reflect"
	"testing"
	"time"

	"flypanel/internal/store"
)

var testNow = time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)

func baseUser() store.User {
	return store.User{
		Username:    "testuser",
		UUID:        "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		SubToken:    "0123456789abcdef",
		Expiry:      testNow.Add(24 * time.Hour),
		QuotaPeriod: "none",
	}
}

func TestDecide(t *testing.T) {
	cases := []struct {
		name string
		user store.User
		used int64
		ips  []string
		want []Action
	}{
		{
			name: "quota exceeded disables",
			user: func() store.User { u := baseUser(); u.QuotaBytes = 100; return u }(),
			used: 120,
			want: []Action{{Type: "disable", Reason: "quota"}},
		},
		{
			name: "quota still exceeded without rollover stays disabled",
			user: func() store.User {
				u := baseUser()
				u.QuotaBytes = 100
				u.QuotaPeriod = "day"
				u.QuotaResetAt = time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
				u.Disabled = true
				u.DisabledReason = "quota"
				return u
			}(),
			used: 120,
			want: nil,
		},
		{
			name: "rollover resets used and enables quota-disabled user",
			user: func() store.User {
				u := baseUser()
				u.QuotaBytes = 100
				u.QuotaPeriod = "day"
				u.QuotaResetAt = time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
				u.Disabled = true
				u.DisabledReason = "quota"
				return u
			}(),
			used: 120,
			want: []Action{{Type: "rollover", Reason: "day"}, {Type: "enable", Reason: "quota"}},
		},
		{
			name: "device over limit disables",
			user: func() store.User { u := baseUser(); u.DeviceLimit = 2; return u }(),
			ips:  []string{"203.0.113.1", "203.0.113.2", "203.0.113.3"},
			want: []Action{{Type: "disable", Reason: "device_limit"}},
		},
		{
			name: "nat five connections from one ip is one device",
			user: func() store.User { u := baseUser(); u.DeviceLimit = 1; return u }(),
			ips:  []string{"203.0.113.7", "203.0.113.7", "203.0.113.7", "203.0.113.7", "203.0.113.7"},
			want: nil,
		},
		{
			name: "device back under limit enables",
			user: func() store.User {
				u := baseUser()
				u.DeviceLimit = 2
				u.Disabled = true
				u.DisabledReason = "device_limit"
				return u
			}(),
			ips:  []string{"203.0.113.1", "203.0.113.2"},
			want: []Action{{Type: "enable", Reason: "device_limit"}},
		},
		{
			name: "expiry passed disables",
			user: func() store.User { u := baseUser(); u.Expiry = testNow.Add(-time.Hour); return u }(),
			want: []Action{{Type: "disable", Reason: "expired"}},
		},
		{
			name: "expiry extended enables",
			user: func() store.User {
				u := baseUser()
				u.Expiry = testNow.Add(24 * time.Hour)
				u.Disabled = true
				u.DisabledReason = "expired"
				return u
			}(),
			want: []Action{{Type: "enable", Reason: "expired"}},
		},
		{
			name: "manual disabled never auto-enables",
			user: func() store.User {
				u := baseUser()
				u.Disabled = true
				u.DisabledReason = "manual"
				return u
			}(),
			want: nil,
		},
		{
			name: "expiry beats quota when simultaneous",
			user: func() store.User {
				u := baseUser()
				u.Expiry = testNow.Add(-time.Minute)
				u.QuotaBytes = 100
				return u
			}(),
			used: 120,
			want: []Action{{Type: "disable", Reason: "expired"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Decide(tc.user, tc.used, tc.ips, testNow)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Decide() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestDecideRolloverEnables pins the poller's rollover path: a
// quota-disabled user whose period boundary has passed gets BOTH a
// rollover and an enable action.
func TestDecideRolloverEnables(t *testing.T) {
	u := baseUser()
	u.QuotaBytes = 100
	u.QuotaPeriod = "day"
	u.QuotaResetAt = time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	u.Disabled = true
	u.DisabledReason = "quota"

	actions := Decide(u, 120, nil, testNow)
	var hasRollover, hasEnable bool
	for _, a := range actions {
		switch a.Type {
		case "rollover":
			hasRollover = true
		case "enable":
			hasEnable = true
		}
	}
	if !hasRollover || !hasEnable {
		t.Fatalf("actions = %+v, want both a rollover and an enable", actions)
	}
}

func TestNextBoundary(t *testing.T) {
	cases := []struct {
		name   string
		start  time.Time
		period string
		want   time.Time
	}{
		{
			name:   "day",
			start:  time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC),
			period: "day",
			want:   time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "day on boundary goes to next day",
			start:  time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC),
			period: "day",
			want:   time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "day non-utc start normalized",
			start:  time.Date(2026, 11, 20, 15, 0, 0, 0, time.FixedZone("X", 5*3600)),
			period: "day",
			want:   time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "week thursday to monday",
			start:  time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC), // Thursday
			period: "week",
			want:   time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC), // Monday
		},
		{
			name:   "week on monday boundary goes to next monday",
			start:  time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC), // Monday
			period: "week",
			want:   time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "month",
			start:  time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC),
			period: "month",
			want:   time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "month year rollover",
			start:  time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC),
			period: "month",
			want:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "month on first goes to next month",
			start:  time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
			period: "month",
			want:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "none unchanged",
			start:  time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC),
			period: "none",
			want:   time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NextBoundary(tc.start, tc.period)
			if !got.Equal(tc.want) {
				t.Fatalf("NextBoundary(%s, %s) = %s, want %s",
					tc.start.Format(time.RFC3339), tc.period, got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}

// fakeClient is an in-memory MemberSync: adding an existing member is a
// no-op (like xray) and removing a missing member is a no-op too.
type fakeClient struct {
	members     map[string]map[string]string // tag -> email -> uuid
	addCalls    []string                     // "tag|email"
	removeCalls []string                     // "tag|email"
	failAddTag  string
}

func (f *fakeClient) AddUser(tag, uuid, email string) error {
	f.addCalls = append(f.addCalls, tag+"|"+email)
	if tag == f.failAddTag {
		return errFakeSync
	}
	if f.members == nil {
		f.members = map[string]map[string]string{}
	}
	if f.members[tag] == nil {
		f.members[tag] = map[string]string{}
	}
	f.members[tag][email] = uuid
	return nil
}

func (f *fakeClient) RemoveUser(tag, email string) error {
	f.removeCalls = append(f.removeCalls, tag+"|"+email)
	if f.members[tag] != nil {
		delete(f.members[tag], email)
	}
	return nil
}

func TestSyncIdempotent(t *testing.T) {
	fc := &fakeClient{}
	inbounds := []store.Inbound{
		{ID: 1, Tag: "in-a", Enabled: true, ConfigJSON: "{}", CreatedAt: testNow},
		{ID: 2, Tag: "in-b", Enabled: true, ConfigJSON: "{}", CreatedAt: testNow},
		{ID: 3, Tag: "in-off", Enabled: false, ConfigJSON: "{}", CreatedAt: testNow},
	}
	on := baseUser()
	on.Username = "u-on"
	off := baseUser()
	off.Username = "u-off"
	off.Disabled = true

	ctx := context.Background()
	if err := SyncMembership(ctx, fc, inbounds, []store.User{on, off}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if _, ok := fc.members["in-a"]["u-on"]; !ok {
		t.Fatal("u-on not added to in-a")
	}
	if _, ok := fc.members["in-b"]["u-on"]; !ok {
		t.Fatal("u-on not added to in-b")
	}
	if len(fc.members["in-off"]) != 0 {
		t.Fatal("disabled inbound must not be touched")
	}
	if _, ok := fc.members["in-a"]["u-off"]; ok {
		t.Fatal("u-off not removed from in-a")
	}

	// Second pass: re-add of an existing member and remove of a missing
	// member are both no-ops without errors.
	if err := SyncMembership(ctx, fc, inbounds, []store.User{on, off}); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if _, ok := fc.members["in-a"]["u-on"]; !ok {
		t.Fatal("u-on lost after second sync")
	}
}

func TestSyncErrorJoinsAndContinues(t *testing.T) {
	fc := &fakeClient{failAddTag: "in-b"}
	inbounds := []store.Inbound{
		{ID: 1, Tag: "in-a", Enabled: true, ConfigJSON: "{}", CreatedAt: testNow},
		{ID: 2, Tag: "in-b", Enabled: true, ConfigJSON: "{}", CreatedAt: testNow},
	}
	u := baseUser()
	err := SyncMembership(context.Background(), fc, inbounds, []store.User{u})
	if err == nil {
		t.Fatal("expected joined error for the failing inbound")
	}
	if _, ok := fc.members["in-a"][u.Username]; !ok {
		t.Fatal("failing inbound must not stop the other inbounds")
	}
}
