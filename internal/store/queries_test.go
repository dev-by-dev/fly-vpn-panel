package store

import (
	"errors"
	"testing"
	"time"
)

func TestUserCRUD(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer s.Close()

	u := &User{
		Username:     "alice",
		UUID:         "11111111-1111-4111-8111-111111111111",
		SubToken:     "sub-alice-001",
		Expiry:       time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC),
		QuotaBytes:   1024,
		QuotaPeriod:  "month",
		QuotaResetAt: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		DeviceLimit:  3,
		CreatedAt:    time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
	}
	if err := s.UserCreate(u); err != nil {
		t.Fatalf("UserCreate(): %v", err)
	}
	if u.ID == 0 {
		t.Fatal("UserCreate(): did not set ID")
	}

	got, err := s.UserGet(u.ID)
	if err != nil {
		t.Fatalf("UserGet(%d): %v", u.ID, err)
	}
	if got.Username != u.Username || got.UUID != u.UUID || got.SubToken != u.SubToken {
		t.Errorf("UserGet() string fields = %+v", got)
	}
	if !got.Expiry.Equal(u.Expiry) || !got.QuotaResetAt.Equal(u.QuotaResetAt) || !got.CreatedAt.Equal(u.CreatedAt) {
		t.Errorf("UserGet() times = expiry %v reset %v created %v", got.Expiry, got.QuotaResetAt, got.CreatedAt)
	}
	if got.QuotaBytes != u.QuotaBytes || got.QuotaPeriod != u.QuotaPeriod || got.DeviceLimit != u.DeviceLimit {
		t.Errorf("UserGet() quota/device = %+v", got)
	}
	if got.Disabled {
		t.Error("new user Disabled = true, want false")
	}

	if _, err := s.UserGetByToken(u.SubToken); err != nil {
		t.Errorf("UserGetByToken(): %v", err)
	}
	if _, err := s.UserGetByUsername(u.Username); err != nil {
		t.Errorf("UserGetByUsername(): %v", err)
	}
	if _, err := s.UserGet(999); !errors.Is(err, ErrNotFound) {
		t.Errorf("UserGet(999) err = %v, want ErrNotFound", err)
	}

	dup := &User{
		Username:    u.Username,
		UUID:        "22222222-2222-4222-8222-222222222222",
		SubToken:    "sub-dup-001",
		Expiry:      u.Expiry,
		QuotaPeriod: "none",
		CreatedAt:   u.CreatedAt,
	}
	if err := s.UserCreate(dup); err == nil {
		t.Error("UserCreate(duplicate username): want error, got nil")
	}

	got.QuotaBytes = 4096
	got.DeviceLimit = 5
	got.Expiry = time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := s.UserUpdate(got); err != nil {
		t.Fatalf("UserUpdate(): %v", err)
	}
	after, err := s.UserGet(u.ID)
	if err != nil {
		t.Fatalf("UserGet() after update: %v", err)
	}
	if after.QuotaBytes != 4096 || after.DeviceLimit != 5 || !after.Expiry.Equal(got.Expiry) {
		t.Errorf("after update = %+v", after)
	}

	list, err := s.UserList()
	if err != nil {
		t.Fatalf("UserList(): %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("UserList() len = %d, want 1", len(list))
	}
	if list[0].Username != u.Username {
		t.Errorf("UserList()[0].Username = %q, want %q", list[0].Username, u.Username)
	}

	if err := s.UserDelete(u.ID); err != nil {
		t.Fatalf("UserDelete(): %v", err)
	}
	if _, err := s.UserGet(u.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("UserGet() after delete err = %v, want ErrNotFound", err)
	}
}

func TestUserSetDisabled(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer s.Close()

	u := &User{
		Username:    "bob",
		UUID:        "33333333-3333-4333-8333-333333333333",
		SubToken:    "sub-bob-001",
		Expiry:      time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		QuotaPeriod: "none",
		CreatedAt:   time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
	}
	if err := s.UserCreate(u); err != nil {
		t.Fatalf("UserCreate(): %v", err)
	}

	if err := s.UserSetDisabled(u.ID, true, "policy: abuse"); err != nil {
		t.Fatalf("UserSetDisabled(true): %v", err)
	}
	got, err := s.UserGet(u.ID)
	if err != nil {
		t.Fatalf("UserGet(): %v", err)
	}
	if !got.Disabled || got.DisabledReason != "policy: abuse" {
		t.Errorf("after disable: disabled = %v, reason = %q", got.Disabled, got.DisabledReason)
	}

	if err := s.UserSetDisabled(u.ID, false, ""); err != nil {
		t.Fatalf("UserSetDisabled(false): %v", err)
	}
	got, err = s.UserGet(u.ID)
	if err != nil {
		t.Fatalf("UserGet(): %v", err)
	}
	if got.Disabled || got.DisabledReason != "" {
		t.Errorf("after enable: disabled = %v, reason = %q", got.Disabled, got.DisabledReason)
	}

	if err := s.UserSetDisabled(999, true, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("UserSetDisabled(999) err = %v, want ErrNotFound", err)
	}
}

func TestBackupCodeOneTime(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer s.Close()

	if err := s.BackupCodesReplace([]string{"hash-a", "hash-b", "hash-c"}); err != nil {
		t.Fatalf("BackupCodesReplace(): %v", err)
	}
	ok, err := s.BackupCodeConsume("hash-a")
	if err != nil || !ok {
		t.Fatalf("first consume = %v, %v; want true, nil", ok, err)
	}
	ok, err = s.BackupCodeConsume("hash-a")
	if err != nil || ok {
		t.Errorf("second consume = %v, %v; want false, nil", ok, err)
	}
	ok, err = s.BackupCodeConsume("missing")
	if err != nil || ok {
		t.Errorf("consume unknown = %v, %v; want false, nil", ok, err)
	}
	ok, err = s.BackupCodeConsume("hash-c")
	if err != nil || !ok {
		t.Errorf("consume other code = %v, %v; want true, nil", ok, err)
	}

	// Replacing wipes the old set.
	if err := s.BackupCodesReplace([]string{"hash-d"}); err != nil {
		t.Fatalf("BackupCodesReplace() again: %v", err)
	}
	ok, err = s.BackupCodeConsume("hash-b")
	if err != nil || ok {
		t.Errorf("consume old code after replace = %v, %v; want false, nil", ok, err)
	}
	ok, err = s.BackupCodeConsume("hash-d")
	if err != nil || !ok {
		t.Errorf("consume new code = %v, %v; want true, nil", ok, err)
	}
}

func TestAdminSingle(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer s.Close()

	if err := s.AdminCreate("paneladmin", "hash-1"); err != nil {
		t.Fatalf("AdminCreate(): %v", err)
	}
	if err := s.AdminCreate("second", "hash-2"); err == nil {
		t.Error("second AdminCreate: want error, got nil")
	}

	a, err := s.AdminGet()
	if err != nil {
		t.Fatalf("AdminGet(): %v", err)
	}
	if a.ID != 1 || a.Username != "paneladmin" || a.PasswordHash != "hash-1" || a.TOTPEnabled {
		t.Errorf("AdminGet() = %+v", a)
	}
	if a.CreatedAt.IsZero() {
		t.Error("Admin CreatedAt is zero")
	}

	if err := s.AdminUpdatePassword("hash-2"); err != nil {
		t.Fatalf("AdminUpdatePassword(): %v", err)
	}
	if err := s.AdminSetTOTP("totp-secret", true); err != nil {
		t.Fatalf("AdminSetTOTP(): %v", err)
	}
	a, err = s.AdminGet()
	if err != nil {
		t.Fatalf("AdminGet() after updates: %v", err)
	}
	if a.PasswordHash != "hash-2" || a.TOTPSecret != "totp-secret" || !a.TOTPEnabled {
		t.Errorf("AdminGet() after updates = %+v", a)
	}
}

func TestSamplesSince(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer s.Close()

	base := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	samples := []struct {
		off      time.Duration
		up, down int64
	}{
		{-2 * time.Hour, 100, 200}, // before cutoff, excluded
		{-1 * time.Hour, 10, 20},   // after cutoff
		{0, 1, 2},                  // after cutoff
	}
	for _, sm := range samples {
		if err := s.SampleInsert("alice", base.Add(sm.off), sm.up, sm.down); err != nil {
			t.Fatalf("SampleInsert(%v): %v", sm.off, err)
		}
	}

	since := base.Add(-90 * time.Minute)
	up, down, err := s.SamplesSince("alice", since)
	if err != nil {
		t.Fatalf("SamplesSince(): %v", err)
	}
	if up != 11 || down != 22 {
		t.Errorf("SamplesSince() = up %d, down %d; want 11, 22", up, down)
	}

	up, down, err = s.SamplesSince("nobody", since)
	if err != nil || up != 0 || down != 0 {
		t.Errorf("SamplesSince(nobody) = %d, %d, %v; want 0, 0, nil", up, down, err)
	}
}

func TestSamplesSeries(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer s.Close()

	base := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	// Same ts twice: two users sampled in one tick must collapse into one bucket.
	if err := s.SampleInsert("alice", base, 10, 20); err != nil {
		t.Fatalf("SampleInsert(alice): %v", err)
	}
	if err := s.SampleInsert("bob", base, 1, 2); err != nil {
		t.Fatalf("SampleInsert(bob): %v", err)
	}
	if err := s.SampleInsert("alice", base.Add(-time.Hour), 999, 999); err != nil {
		t.Fatalf("SampleInsert(old): %v", err)
	}

	bs, err := s.SamplesSeries(base.Add(-time.Minute))
	if err != nil {
		t.Fatalf("SamplesSeries(): %v", err)
	}
	if len(bs) != 1 {
		t.Fatalf("SamplesSeries() len = %d; want 1", len(bs))
	}
	if bs[0].Up != 11 || bs[0].Down != 22 || !bs[0].TS.Equal(base) {
		t.Errorf("SamplesSeries()[0] = %+v; want ts %v, up 11, down 22", bs[0], base)
	}
}

func TestTrafficSeries(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer s.Close()

	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	inserts := []struct {
		ts       time.Time
		up, down int64
	}{
		{base.Add(5 * time.Minute), 10, 100},
		{base.Add(40 * time.Minute), 5, 50},
		{base.Add(65 * time.Minute), 1, 1},
		{base.Add(24 * time.Hour), 100, 200},
	}
	for _, in := range inserts {
		if err := s.SampleInsert("alice", in.ts, in.up, in.down); err != nil {
			t.Fatalf("SampleInsert(): %v", err)
		}
	}

	hourly, err := s.TrafficSeries("alice", base, base.Add(48*time.Hour), "hour")
	if err != nil {
		t.Fatalf("TrafficSeries(hour): %v", err)
	}
	wantHour := []Bucket{
		{TS: base, Up: 15, Down: 150},
		{TS: base.Add(time.Hour), Up: 1, Down: 1},
		{TS: base.Add(24 * time.Hour), Up: 100, Down: 200},
	}
	if len(hourly) != len(wantHour) {
		t.Fatalf("hourly buckets = %d, want %d", len(hourly), len(wantHour))
	}
	for i, w := range wantHour {
		if !hourly[i].TS.Equal(w.TS) || hourly[i].Up != w.Up || hourly[i].Down != w.Down {
			t.Errorf("hourly[%d] = %+v, want %+v", i, hourly[i], w)
		}
	}

	daily, err := s.TrafficSeries("alice", base, base.Add(48*time.Hour), "day")
	if err != nil {
		t.Fatalf("TrafficSeries(day): %v", err)
	}
	wantDay := []Bucket{
		{TS: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), Up: 16, Down: 151},
		{TS: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), Up: 100, Down: 200},
	}
	if len(daily) != len(wantDay) {
		t.Fatalf("daily buckets = %d, want %d", len(daily), len(wantDay))
	}
	for i, w := range wantDay {
		if !daily[i].TS.Equal(w.TS) || daily[i].Up != w.Up || daily[i].Down != w.Down {
			t.Errorf("daily[%d] = %+v, want %+v", i, daily[i], w)
		}
	}

	if _, err := s.TrafficSeries("alice", base, base.Add(time.Hour), "week"); err == nil {
		t.Error("TrafficSeries(bucket=week): want error, got nil")
	}
}

func TestDeviceUpsert(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer s.Close()

	t1 := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(2 * time.Hour)
	if err := s.DeviceUpsert("alice", "203.0.113.10", DeviceMeta{}, t1); err != nil {
		t.Fatalf("DeviceUpsert(): %v", err)
	}
	if err := s.DeviceUpsert("alice", "203.0.113.10", DeviceMeta{}, t2); err != nil {
		t.Fatalf("DeviceUpsert() again: %v", err)
	}

	devs, err := s.DevicesForUser("alice")
	if err != nil {
		t.Fatalf("DevicesForUser(): %v", err)
	}
	if len(devs) != 1 {
		t.Fatalf("devices = %d, want 1", len(devs))
	}
	d := devs[0]
	if d.Username != "alice" || d.IP != "203.0.113.10" {
		t.Errorf("device = %q/%q", d.Username, d.IP)
	}
	if !d.FirstSeen.Equal(t1) || !d.LastSeen.Equal(t2) {
		t.Errorf("device times = first_seen %v, last_seen %v; want %v, %v (first_seen must be immutable)",
			d.FirstSeen, d.LastSeen, t1, t2)
	}

	if err := s.DeviceUpsert("alice", "203.0.113.11", DeviceMeta{}, t2); err != nil {
		t.Fatalf("DeviceUpsert() new ip: %v", err)
	}
	online, err := s.DevicesOnline(t2.Add(-time.Minute))
	if err != nil {
		t.Fatalf("DevicesOnline(): %v", err)
	}
	if len(online) != 2 {
		t.Errorf("DevicesOnline(t2-1m) = %d, want 2", len(online))
	}
	online, err = s.DevicesOnline(t2.Add(time.Minute))
	if err != nil {
		t.Fatalf("DevicesOnline(): %v", err)
	}
	if len(online) != 0 {
		t.Errorf("DevicesOnline(t2+1m) = %d, want 0", len(online))
	}

	if err := s.PruneDevices(t2.Add(time.Minute)); err != nil {
		t.Fatalf("PruneDevices(): %v", err)
	}
	devs, err = s.DevicesForUser("alice")
	if err != nil {
		t.Fatalf("DevicesForUser() after prune: %v", err)
	}
	if len(devs) != 0 {
		t.Errorf("devices after prune = %d, want 0", len(devs))
	}
}

func TestRefreshTokenLifecycle(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer s.Close()

	future := time.Now().UTC().Add(time.Hour)
	past := time.Now().UTC().Add(-time.Hour)
	if err := s.RefreshTokenInsert("tok-a", future); err != nil {
		t.Fatalf("RefreshTokenInsert(): %v", err)
	}
	if err := s.RefreshTokenInsert("tok-b", past); err != nil {
		t.Fatalf("RefreshTokenInsert(): %v", err)
	}

	if ok, err := s.RefreshTokenValid("tok-a"); err != nil || !ok {
		t.Errorf("valid(tok-a) = %v, %v; want true, nil", ok, err)
	}
	if ok, err := s.RefreshTokenValid("tok-b"); err != nil || ok {
		t.Errorf("valid(expired tok-b) = %v, %v; want false, nil", ok, err)
	}

	if err := s.RefreshTokenRevoke("tok-a"); err != nil {
		t.Fatalf("RefreshTokenRevoke(): %v", err)
	}
	if ok, err := s.RefreshTokenValid("tok-a"); err != nil || ok {
		t.Errorf("valid(revoked tok-a) = %v, %v; want false, nil", ok, err)
	}

	if err := s.RefreshTokenInsert("tok-c", future); err != nil {
		t.Fatalf("RefreshTokenInsert() tok-c: %v", err)
	}
	if err := s.RefreshTokenRevokeAll(); err != nil {
		t.Fatalf("RefreshTokenRevokeAll(): %v", err)
	}
	if ok, err := s.RefreshTokenValid("tok-c"); err != nil || ok {
		t.Errorf("valid(tok-c after revoke-all) = %v, %v; want false, nil", ok, err)
	}
}

func TestSettingsRoundtrip(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer s.Close()

	if _, err := s.SettingGet("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SettingGet(missing) err = %v, want ErrNotFound", err)
	}
	if err := s.SettingSet("site_name", "FlyPanel"); err != nil {
		t.Fatalf("SettingSet(): %v", err)
	}
	if err := s.SettingSet("site_name", "FlyPanel v2"); err != nil {
		t.Fatalf("SettingSet() overwrite: %v", err)
	}
	v, err := s.SettingGet("site_name")
	if err != nil || v != "FlyPanel v2" {
		t.Errorf("SettingGet() = %q, %v; want %q, nil", v, err, "FlyPanel v2")
	}
	if err := s.SettingSet("maintainer", "ops"); err != nil {
		t.Fatalf("SettingSet() second key: %v", err)
	}

	all, err := s.SettingsAll()
	if err != nil {
		t.Fatalf("SettingsAll(): %v", err)
	}
	if len(all) != 2 || all["site_name"] != "FlyPanel v2" || all["maintainer"] != "ops" {
		t.Errorf("SettingsAll() = %v", all)
	}
}
