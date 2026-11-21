package users

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"flypanel/internal/store"
)

var errFakeSync = errors.New("fake sync failure")

type apiUser struct {
	ID             int64     `json:"id"`
	Username       string    `json:"username"`
	UUID           string    `json:"uuid"`
	SubToken       string    `json:"sub_token"`
	Expiry         time.Time `json:"expiry"`
	QuotaBytes     int64     `json:"quota_bytes"`
	QuotaPeriod    string    `json:"quota_period"`
	QuotaResetAt   time.Time `json:"quota_reset_at"`
	DeviceLimit    int       `json:"device_limit"`
	Disabled       bool      `json:"disabled"`
	DisabledReason string    `json:"disabled_reason"`
	CreatedAt      time.Time `json:"created_at"`
	UsedBytes      int64     `json:"used_bytes"`
	OnlineCount    int       `json:"online_count"`
}

func newTestServer(t *testing.T, fc *fakeClient) (*store.Store, http.Handler) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	h := &Handlers{Store: st, Client: fc, PollInterval: 30 * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users", h.List)
	mux.HandleFunc("POST /api/users", h.Create)
	mux.HandleFunc("GET /api/users/{id}", h.Get)
	mux.HandleFunc("PATCH /api/users/{id}", h.Patch)
	mux.HandleFunc("DELETE /api/users/{id}", h.Delete)
	mux.HandleFunc("POST /api/users/{id}/reset-quota", h.ResetQuota)
	return st, mux
}

func seedInbound(t *testing.T, st *store.Store, tag string, enabled bool) {
	t.Helper()
	err := st.InboundCreate(&store.Inbound{
		Tag: tag, Enabled: enabled, ConfigJSON: "{}", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed inbound %q: %v", tag, err)
	}
}

func doJSON(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func createUserViaAPI(t *testing.T, srv http.Handler, body string) apiUser {
	t.Helper()
	rec := doJSON(t, srv, "POST", "/api/users", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user: status %d, body %s", rec.Code, rec.Body.String())
	}
	var u apiUser
	if err := json.Unmarshal(rec.Body.Bytes(), &u); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return u
}

func TestUserCreateDefaults(t *testing.T) {
	fc := &fakeClient{}
	st, srv := newTestServer(t, fc)
	seedInbound(t, st, "in-a", true)

	u := createUserViaAPI(t, srv, `{"username":"alice","expiry":"2027-06-01T00:00:00Z"}`)

	if _, err := uuid.Parse(u.UUID); err != nil {
		t.Fatalf("uuid %q not parseable: %v", u.UUID, err)
	}
	if uuid.MustParse(u.UUID).Version() != 4 {
		t.Fatalf("uuid %q is not v4", u.UUID)
	}
	if len(u.SubToken) != 16 {
		t.Fatalf("sub_token %q: len %d, want 16", u.SubToken, len(u.SubToken))
	}
	if u.DeviceLimit != 0 {
		t.Fatalf("device_limit = %d, want 0", u.DeviceLimit)
	}
	if u.QuotaPeriod != "none" {
		t.Fatalf("quota_period = %q, want none", u.QuotaPeriod)
	}
	if u.Disabled {
		t.Fatal("new user must not be disabled")
	}
	if !u.QuotaResetAt.IsZero() {
		t.Fatalf("quota_reset_at = %s, want zero for period none", u.QuotaResetAt)
	}
	// The new user must be hot-synced onto the enabled inbound.
	if _, ok := fc.members["in-a"]["alice"]; !ok {
		t.Fatal("new user not added to enabled inbound")
	}
}

func TestUserCreateValidation(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"missing username", `{"expiry":"2027-06-01T00:00:00Z"}`},
		{"missing expiry", `{"username":"bob"}`},
		{"invalid expiry", `{"username":"bob","expiry":"not-a-time"}`},
		{"invalid quota period", `{"username":"bob","expiry":"2027-06-01T00:00:00Z","quota_period":"hour"}`},
		{"negative device limit", `{"username":"bob","expiry":"2027-06-01T00:00:00Z","device_limit":-1}`},
		{"bad username charset arrows", `{"username":"bad>>>name","expiry":"2027-06-01T00:00:00Z"}`},
		{"bad username charset space", `{"username":"user name","expiry":"2027-06-01T00:00:00Z"}`},
		{"bad username charset cyrillic", `{"username":"юзер","expiry":"2027-06-01T00:00:00Z"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, srv := newTestServer(t, &fakeClient{})
			rec := doJSON(t, srv, "POST", "/api/users", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestUserCreateUsernameCharset: usernames are the xray stats key; only
// [A-Za-z0-9._@-] survives OnlineUsers extraction, so anything else is
// rejected at the boundary.
func TestUserCreateUsernameCharset(t *testing.T) {
	_, srv := newTestServer(t, &fakeClient{})

	// Charset edge cases that MUST be accepted.
	for _, name := range []string{"alice", "user-1_x.y", "user@example.com", "A0.-_@"} {
		rec := doJSON(t, srv, "POST", "/api/users",
			`{"username":"`+name+`","expiry":"2027-06-01T00:00:00Z"}`)
		if rec.Code != http.StatusCreated {
			t.Errorf("username %q: status = %d, want 201, body %s", name, rec.Code, rec.Body.String())
		}
	}

	// Anything outside the charset is rejected with invalid_username.
	for _, name := range []string{"bad>>>name", "user name", "юзер", "semi;colon", "quo\"te"} {
		b, err := json.Marshal(map[string]string{"username": name, "expiry": "2027-06-01T00:00:00Z"})
		if err != nil {
			t.Fatal(err)
		}
		rec := doJSON(t, srv, "POST", "/api/users", string(b))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("username %q: status = %d, want 400, body %s", name, rec.Code, rec.Body.String())
			continue
		}
		var e struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		if e.Error != "invalid_username" {
			t.Errorf("username %q: error = %q, want invalid_username", name, e.Error)
		}
	}
}

func TestUserListEnriched(t *testing.T) {
	st, srv := newTestServer(t, &fakeClient{})

	u := createUserViaAPI(t, srv,
		`{"username":"bob","expiry":"2027-06-01T00:00:00Z","quota_period":"day"}`)

	// The create handler sets quota_reset_at to the current period start.
	reset := periodStart(time.Now().UTC(), "day")
	if !u.QuotaResetAt.Equal(reset) {
		t.Fatalf("quota_reset_at = %s, want %s", u.QuotaResetAt, reset)
	}

	// One sample before the reset (excluded), two after (summed).
	if err := st.SampleInsert("bob", reset.Add(-2*time.Hour), 999, 999); err != nil {
		t.Fatal(err)
	}
	if err := st.SampleInsert("bob", reset.Add(time.Hour), 100, 10); err != nil {
		t.Fatal(err)
	}
	if err := st.SampleInsert("bob", reset.Add(2*time.Hour), 50, 5); err != nil {
		t.Fatal(err)
	}

	// One fresh device (within 2*PollInterval = 60s) and one stale.
	now := time.Now().UTC()
	if err := st.DeviceUpsert("bob", "203.0.113.10", store.DeviceMeta{HWID: "203.0.113.10"}, now); err != nil {
		t.Fatal(err)
	}
	if err := st.DeviceUpsert("bob", "198.51.100.20", store.DeviceMeta{HWID: "198.51.100.20"}, now.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, srv, "GET", "/api/users", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var list []apiUser
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
	got := list[0]
	if got.UsedBytes != 165 {
		t.Fatalf("used_bytes = %d, want 165", got.UsedBytes)
	}
	if got.OnlineCount != 1 {
		t.Fatalf("online_count = %d, want 1", got.OnlineCount)
	}
}

func TestUserPatchQuotaRaiseEnables(t *testing.T) {
	fc := &fakeClient{}
	st, srv := newTestServer(t, fc)
	seedInbound(t, st, "in-a", true)

	u := createUserViaAPI(t, srv,
		`{"username":"carol","expiry":"2027-06-01T00:00:00Z","quota_bytes":100}`)

	// Simulate the poller disabling the user for quota exhaustion.
	if err := st.UserSetDisabled(u.ID, true, "quota"); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, srv, "PATCH", "/api/users/"+strconv.FormatInt(u.ID, 10), `{"quota_bytes":1000}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var patched apiUser
	if err := json.Unmarshal(rec.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Disabled {
		t.Fatal("raising quota_bytes must re-enable a quota-disabled user")
	}

	db, err := st.UserGet(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if db.Disabled || db.DisabledReason != "" {
		t.Fatalf("db user disabled=%v reason=%q, want enabled with empty reason",
			db.Disabled, db.DisabledReason)
	}
	if _, ok := fc.members["in-a"]["carol"]; !ok {
		t.Fatal("re-enabled user not a member of the enabled inbound")
	}
}

func TestUserDeleteRemovesFromInbounds(t *testing.T) {
	fc := &fakeClient{}
	st, srv := newTestServer(t, fc)
	seedInbound(t, st, "in-a", true)
	seedInbound(t, st, "in-b", false)

	u := createUserViaAPI(t, srv, `{"username":"dave","expiry":"2027-06-01T00:00:00Z"}`)

	rec := doJSON(t, srv, "DELETE", "/api/users/"+strconv.FormatInt(u.ID, 10), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if len(fc.removeCalls) != 1 || fc.removeCalls[0] != "in-a|dave" {
		t.Fatalf("removeCalls = %v, want exactly [in-a|dave]", fc.removeCalls)
	}
	if _, ok := fc.members["in-a"]["dave"]; ok {
		t.Fatal("dave still a member of in-a after delete")
	}
	if _, err := st.UserGet(u.ID); err != store.ErrNotFound {
		t.Fatalf("user not deleted from db: %v", err)
	}
	rec = doJSON(t, srv, "GET", "/api/users/"+strconv.FormatInt(u.ID, 10), "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: status %d, want 404", rec.Code)
	}
}

func TestUsernameImmutable(t *testing.T) {
	_, srv := newTestServer(t, &fakeClient{})
	u := createUserViaAPI(t, srv, `{"username":"erin","expiry":"2027-06-01T00:00:00Z"}`)

	rec := doJSON(t, srv, "PATCH", "/api/users/"+strconv.FormatInt(u.ID, 10), `{"username":"frank"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body %s", rec.Code, rec.Body.String())
	}
}

func TestResetQuota(t *testing.T) {
	fc := &fakeClient{}
	st, srv := newTestServer(t, fc)
	seedInbound(t, st, "in-a", true)

	u := createUserViaAPI(t, srv,
		`{"username":"gina","expiry":"2027-06-01T00:00:00Z","quota_period":"day"}`)
	if err := st.UserSetDisabled(u.ID, true, "quota"); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, srv, "POST", "/api/users/"+strconv.FormatInt(u.ID, 10)+"/reset-quota", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var got apiUser
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Disabled {
		t.Fatal("reset-quota must re-enable a quota-disabled user")
	}
	want := periodStart(time.Now().UTC(), "day")
	if !got.QuotaResetAt.Equal(want) {
		t.Fatalf("quota_reset_at = %s, want %s", got.QuotaResetAt, want)
	}
	if _, ok := fc.members["in-a"]["gina"]; !ok {
		t.Fatal("re-enabled user not a member of the enabled inbound")
	}
}

func TestResetQuotaNonePeriod(t *testing.T) {
	fc := &fakeClient{}
	st, srv := newTestServer(t, fc)
	seedInbound(t, st, "in-a", true)

	// period none + quota: usage is lifetime (reset_at zero), so the
	// poller disabled ivy once the samples crossed the quota.
	u := createUserViaAPI(t, srv,
		`{"username":"ivy","expiry":"2027-06-01T00:00:00Z","quota_bytes":100}`)
	old := time.Now().UTC()
	if err := st.SampleInsert("ivy", old.Add(-time.Hour), 60, 60); err != nil {
		t.Fatal(err)
	}
	if err := st.UserSetDisabled(u.ID, true, "quota"); err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC()
	rec := doJSON(t, srv, "POST", "/api/users/"+strconv.FormatInt(u.ID, 10)+"/reset-quota", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	after := time.Now().UTC()
	var got apiUser
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Disabled {
		t.Fatal("reset-quota must re-enable a quota-disabled user")
	}
	// none has no period start: the window must restart at ~now, not
	// at the zero time (which would keep counting lifetime usage).
	if got.QuotaResetAt.Before(before) || got.QuotaResetAt.After(after) {
		t.Fatalf("quota_reset_at = %s, want within [%s, %s]",
			got.QuotaResetAt, before, after)
	}
	if got.UsedBytes != 0 {
		t.Fatalf("used_bytes = %d, want 0 (pre-reset samples no longer count)", got.UsedBytes)
	}
	if _, ok := fc.members["in-a"]["ivy"]; !ok {
		t.Fatal("re-enabled user not a member of the enabled inbound")
	}

	// The poller recomputes usage since the new reset_at (= 0 here)
	// and must not re-disable the user.
	db, err := st.UserGet(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range Decide(*db, got.UsedBytes, nil, time.Now().UTC()) {
		if a.Type == "disable" {
			t.Fatalf("Decide re-disables after reset: %+v", a)
		}
	}
}

func TestSyncFailureReturns502(t *testing.T) {
	fc := &fakeClient{failAddTag: "in-a"}
	st, srv := newTestServer(t, fc)
	seedInbound(t, st, "in-a", true)

	// The user row is committed, but the response surfaces the sync
	// failure so the admin knows xray did not take the change.
	rec := doJSON(t, srv, "POST", "/api/users",
		`{"username":"hank","expiry":"2027-06-01T00:00:00Z"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body %s", rec.Code, rec.Body.String())
	}
	users, err := st.UserList()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Username != "hank" {
		t.Fatalf("user not committed: %+v", users)
	}
}
