package xrayclient

import (
	"testing"

	handlercmd "github.com/xtls/xray-core/app/proxyman/command"
	"github.com/xtls/xray-core/proxy/vless"
)

const testUUID = "66ad4540-b58c-4ad2-9926-ea63445a9b57"

func TestQueryUserStats(t *testing.T) {
	f := &fakeXray{counters: map[string]int64{
		"user>>>user1@example.com>>>traffic>>>uplink":   1111,
		"user>>>user1@example.com>>>traffic>>>downlink": 2222,
	}}
	c := newBufconnClient(t, f)

	up, down, err := c.QueryUserStats("user1@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if up != 1111 || down != 2222 {
		t.Fatalf("up=%d down=%d, want 1111/2222", up, down)
	}

	// Both exact patterns must be queried exactly once, without reset.
	if len(f.queryReqs) != 2 {
		t.Fatalf("got %d QueryStats calls, want 2", len(f.queryReqs))
	}
	seen := map[string]bool{}
	for _, r := range f.queryReqs {
		switch r.Pattern {
		case "user>>>user1@example.com>>>traffic>>>uplink",
			"user>>>user1@example.com>>>traffic>>>downlink":
			if seen[r.Pattern] {
				t.Errorf("pattern %q queried twice", r.Pattern)
			}
			seen[r.Pattern] = true
		default:
			t.Errorf("unexpected pattern %q", r.Pattern)
		}
		if r.Reset_ {
			t.Errorf("pattern %q: Reset_ = true, want false", r.Pattern)
		}
	}
	for p, ok := range seen {
		if !ok {
			t.Errorf("pattern %q never queried", p)
		}
	}

	// Missing counters (user never transferred) must yield 0, not an error.
	up, down, err = c.QueryUserStats("nobody@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if up != 0 || down != 0 {
		t.Fatalf("missing counter: up=%d down=%d, want 0/0", up, down)
	}
}

func TestOnlineUsers(t *testing.T) {
	f := &fakeXray{online: []string{"user1@example.com", "user2@example.com"}}
	c := newBufconnClient(t, f)

	users, err := c.OnlineUsers()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 {
		t.Fatalf("got %d users, want 2: %v", len(users), users)
	}
	for _, want := range []string{"user1@example.com", "user2@example.com"} {
		found := false
		for _, u := range users {
			if u == want {
				found = true
			}
		}
		if !found {
			t.Errorf("online user %q missing from %v", want, users)
		}
	}
}

func TestOnlineIPs(t *testing.T) {
	f := &fakeXray{ipLists: map[string]map[string]int64{
		"user>>>user1@example.com>>>online": {
			"203.0.113.10": 1730000000,
			"198.51.100.7": 1730000001,
		},
	}}
	c := newBufconnClient(t, f)

	ips, err := c.OnlineIPs("user1@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 2 || ips[0] != "198.51.100.7" || ips[1] != "203.0.113.10" {
		t.Fatalf("ips = %v, want [198.51.100.7 203.0.113.10] (sorted)", ips)
	}
	if len(f.ipReqs) != 1 || f.ipReqs[0].Name != "user>>>user1@example.com>>>online" {
		t.Fatalf("GetStatsOnlineIpList requests = %+v, want name user>>>user1@example.com>>>online", f.ipReqs)
	}

	// Never-connected user: no online map on the server (NotFound),
	// which must surface as an empty list, not an error.
	ips, err = c.OnlineIPs("offline@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 0 {
		t.Fatalf("offline user ips = %v, want empty", ips)
	}
}

func TestAddRemoveUser(t *testing.T) {
	f := &fakeXray{}
	c := newBufconnClient(t, f)

	if err := c.AddUser("vpn-in", testUUID, "user1@example.com"); err != nil {
		t.Fatal(err)
	}
	// Re-adding a user that already exists is idempotent: no error.
	if err := c.AddUser("vpn-in", testUUID, "user1@example.com"); err != nil {
		t.Fatalf("duplicate add: %v", err)
	}
	if err := c.RemoveUser("vpn-in", "user1@example.com"); err != nil {
		t.Fatal(err)
	}

	if len(f.alterReqs) != 3 {
		t.Fatalf("got %d AlterInbound calls, want 3 (add, dup add, remove)", len(f.alterReqs))
	}

	addReq := f.alterReqs[0]
	if addReq.Tag != "vpn-in" {
		t.Errorf("add: tag = %q, want vpn-in", addReq.Tag)
	}
	inst, err := addReq.Operation.GetInstance()
	if err != nil {
		t.Fatal(err)
	}
	addOp, ok := inst.(*handlercmd.AddUserOperation)
	if !ok {
		t.Fatalf("add operation = %T, want *AddUserOperation", inst)
	}
	if addOp.User.Email != "user1@example.com" {
		t.Errorf("add: email = %q, want user1@example.com", addOp.User.Email)
	}
	if addOp.User.Level != 0 {
		t.Errorf("add: level = %d, want 0", addOp.User.Level)
	}
	acctInst, err := addOp.User.Account.GetInstance()
	if err != nil {
		t.Fatal(err)
	}
	acct, ok := acctInst.(*vless.Account)
	if !ok {
		t.Fatalf("add: account = %T, want *vless.Account", acctInst)
	}
	if acct.Id != testUUID {
		t.Errorf("add: vless id = %q, want %q", acct.Id, testUUID)
	}

	rmReq := f.alterReqs[2]
	if rmReq.Tag != "vpn-in" {
		t.Errorf("remove: tag = %q, want vpn-in", rmReq.Tag)
	}
	inst, err = rmReq.Operation.GetInstance()
	if err != nil {
		t.Fatal(err)
	}
	rmOp, ok := inst.(*handlercmd.RemoveUserOperation)
	if !ok {
		t.Fatalf("remove operation = %T, want *RemoveUserOperation", inst)
	}
	if rmOp.Email != "user1@example.com" {
		t.Errorf("remove: email = %q, want user1@example.com", rmOp.Email)
	}
}

// Removing a user that is not on the inbound must be a no-op: xray
// answers "User <email> not found." (codes.Unknown), and membership
// reconcile removes every disabled user on every tick, so a
// pass-through error would fail every sync after the first removal.
func TestRemoveUserMissingIsNoOp(t *testing.T) {
	f := &fakeXray{}
	c := newBufconnClient(t, f)

	if err := c.RemoveUser("vpn-in", "ghost@example.com"); err != nil {
		t.Fatalf("remove of missing user: %v", err)
	}

	// The RPC was still issued (the server did the rejecting).
	if len(f.alterReqs) != 1 || f.alterReqs[0].Tag != "vpn-in" {
		t.Fatalf("alterReqs = %+v, want exactly one remove on vpn-in", f.alterReqs)
	}
}
