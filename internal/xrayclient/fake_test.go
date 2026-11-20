package xrayclient

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"

	handlercmd "github.com/xtls/xray-core/app/proxyman/command"
	statscmd "github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// fakeXray implements the two xray API services the client uses,
// mirroring real server semantics (see app/stats/command/command.go
// and app/proxyman/command/command.go in xray-core): QueryStats matches
// by substring, GetStatsOnlineIpList returns NotFound for an unknown
// online map, AlterInbound applies the decoded operation, a
// duplicate AddUser fails with "already exists", and removing an
// absent user fails with "not found.".
type fakeXray struct {
	statscmd.UnimplementedStatsServiceServer
	handlercmd.UnimplementedHandlerServiceServer

	mu        sync.Mutex
	counters  map[string]int64
	online    []string
	ipLists   map[string]map[string]int64
	users     map[string]bool // emails present on the fake inbound
	queryReqs []*statscmd.QueryStatsRequest
	ipReqs    []*statscmd.GetStatsRequest
	alterReqs []*handlercmd.AlterInboundRequest
}

func (f *fakeXray) QueryStats(ctx context.Context, req *statscmd.QueryStatsRequest) (*statscmd.QueryStatsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queryReqs = append(f.queryReqs, req)
	resp := &statscmd.QueryStatsResponse{}
	for name, v := range f.counters {
		if strings.Contains(name, req.Pattern) {
			resp.Stat = append(resp.Stat, &statscmd.Stat{Name: name, Value: v})
		}
	}
	return resp, nil
}

func (f *fakeXray) GetAllOnlineUsers(ctx context.Context, _ *statscmd.GetAllOnlineUsersRequest) (*statscmd.GetAllOnlineUsersResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &statscmd.GetAllOnlineUsersResponse{Users: append([]string(nil), f.online...)}, nil
}

func (f *fakeXray) GetStatsOnlineIpList(ctx context.Context, req *statscmd.GetStatsRequest) (*statscmd.GetStatsOnlineIpListResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ipReqs = append(f.ipReqs, req)
	ips, ok := f.ipLists[req.Name]
	if !ok {
		return nil, status.Error(codes.NotFound, req.Name+" not found.")
	}
	out := make(map[string]int64, len(ips))
	for ip, ts := range ips {
		out[ip] = ts
	}
	return &statscmd.GetStatsOnlineIpListResponse{Name: req.Name, Ips: out}, nil
}

func (f *fakeXray) AlterInbound(ctx context.Context, req *handlercmd.AlterInboundRequest) (*handlercmd.AlterInboundResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alterReqs = append(f.alterReqs, req)
	inst, err := req.Operation.GetInstance()
	if err != nil {
		return nil, err
	}
	switch op := inst.(type) {
	case *handlercmd.AddUserOperation:
		if f.users[op.User.Email] {
			// real xray: proxy/vless/validator.go "User <email> already exists."
			return nil, status.Error(codes.Unknown, "User "+op.User.Email+" already exists.")
		}
		if f.users == nil {
			f.users = map[string]bool{}
		}
		f.users[op.User.Email] = true
	case *handlercmd.RemoveUserOperation:
		if !f.users[op.Email] {
			// real xray: proxy/vless/validator.go "User <email> not found."
			return nil, status.Error(codes.Unknown, "User "+op.Email+" not found.")
		}
		delete(f.users, op.Email)
	}
	return &handlercmd.AlterInboundResponse{}, nil
}

// newBufconnClient serves fakeXray over an in-memory gRPC connection
// and returns a Client wired to it, like New() but with bufconn dialing.
func newBufconnClient(t *testing.T, f *fakeXray) *Client {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	statscmd.RegisterStatsServiceServer(srv, f)
	handlercmd.RegisterHandlerServiceServer(srv, f)
	go srv.Serve(lis)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn.Close()
		srv.Stop()
	})
	return &Client{
		conn:    conn,
		stats:   statscmd.NewStatsServiceClient(conn),
		handler: handlercmd.NewHandlerServiceClient(conn),
	}
}
