// Package xrayclient talks to the xray-core gRPC API for per-user
// traffic stats, online state and hot user management on an inbound.
package xrayclient

import (
	"context"
	"sort"
	"strings"
	"time"

	handlercmd "github.com/xtls/xray-core/app/proxyman/command"
	statscmd "github.com/xtls/xray-core/app/stats/command"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/vless"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Counter names registered by xray's dispatcher (app/dispatcher/default.go):
//   user>>><email>>>traffic>>>uplink / downlink   byte counters
//   user>>><email>>>online                        per-user online IP map
// Note: separators are three '>' characters, matching xray-core source.

// callTimeout bounds every RPC so a stuck xray cannot hang the poller.
const callTimeout = 10 * time.Second

// Client is a gRPC client to the xray-core API.
type Client struct {
	conn    *grpc.ClientConn
	stats   statscmd.StatsServiceClient
	handler handlercmd.HandlerServiceClient
}

// New dials the xray API at addr (e.g. "127.0.0.1:10085"). Insecure
// transport credentials are intentional: the API inbound listens on
// loopback only.
func New(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:    conn,
		stats:   statscmd.NewStatsServiceClient(conn),
		handler: handlercmd.NewHandlerServiceClient(conn),
	}, nil
}

// Close releases the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), callTimeout)
}

// QueryUserStats returns the uplink/downlink byte counters for one user.
// Counters that do not exist yet (user never transferred) count as 0.
// Counters are read, not reset: Reset_ is always false.
func (c *Client) QueryUserStats(email string) (up, down int64, err error) {
	up, err = c.queryCounter("user>>>" + email + ">>>traffic>>>uplink")
	if err != nil {
		return 0, 0, err
	}
	down, err = c.queryCounter("user>>>" + email + ">>>traffic>>>downlink")
	if err != nil {
		return 0, 0, err
	}
	return up, down, nil
}

func (c *Client) queryCounter(name string) (int64, error) {
	ctx, cancel := ctx()
	defer cancel()
	resp, err := c.stats.QueryStats(ctx, &statscmd.QueryStatsRequest{
		Pattern: name,
		Reset_:  false,
	})
	if err != nil {
		return 0, err
	}
	for _, st := range resp.Stat {
		if st.Name == name {
			return st.Value, nil
		}
	}
	return 0, nil
}

// OnlineUsers returns the emails of all currently online users.
// GetAllOnlineUsers answers with full counter names ("user>>><email>>>online"),
// so the bare email is extracted; malformed entries are skipped.
func (c *Client) OnlineUsers() ([]string, error) {
	ctx, cancel := ctx()
	defer cancel()
	resp, err := c.stats.GetAllOnlineUsers(ctx, &statscmd.GetAllOnlineUsersRequest{})
	if err != nil {
		return nil, err
	}
	emails := make([]string, 0, len(resp.Users))
	for _, u := range resp.Users {
		parts := strings.Split(u, ">>>")
		if len(parts) == 3 && parts[0] == "user" && parts[2] == "online" {
			emails = append(emails, parts[1])
		}
	}
	return emails, nil
}

// OnlineIPs returns the IPs a user is currently connected from, sorted.
// A user with no online map on the server (never connected, or the map
// was reset) yields an empty list, not an error.
func (c *Client) OnlineIPs(email string) ([]string, error) {
	ctx, cancel := ctx()
	defer cancel()
	resp, err := c.stats.GetStatsOnlineIpList(ctx, &statscmd.GetStatsRequest{
		Name: "user>>>" + email + ">>>online",
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	ips := make([]string, 0, len(resp.Ips))
	for ip := range resp.Ips {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	return ips, nil
}

// AddUser adds a VLESS user (level 0) to the inbound with the given tag.
// Adding a user that already exists is a no-op (idempotent), matching
// xray's "User <email> already exists." error on duplicate add.
func (c *Client) AddUser(inboundTag, uuid, email string) error {
	operation := &handlercmd.AddUserOperation{
		User: &protocol.User{
			Level:   0,
			Email:   email,
			Account: serial.ToTypedMessage(&vless.Account{Id: uuid}),
		},
	}
	ctx, cancel := ctx()
	defer cancel()
	_, err := c.handler.AlterInbound(ctx, &handlercmd.AlterInboundRequest{
		Tag:       inboundTag,
		Operation: serial.ToTypedMessage(operation),
	})
	if err != nil && isAlreadyExists(err) {
		return nil
	}
	return err
}

// RemoveUser removes the user with the given email from the inbound.
// Removing a user that is not on the inbound is a no-op (idempotent),
// matching xray's "User <email> not found." error: membership
// reconcile removes every disabled user on every tick, so a
// pass-through error would fail every sync after the first removal.
func (c *Client) RemoveUser(inboundTag, email string) error {
	operation := &handlercmd.RemoveUserOperation{Email: email}
	ctx, cancel := ctx()
	defer cancel()
	_, err := c.handler.AlterInbound(ctx, &handlercmd.AlterInboundRequest{
		Tag:       inboundTag,
		Operation: serial.ToTypedMessage(operation),
	})
	if err != nil && isNotFound(err) {
		return nil
	}
	return err
}

func isAlreadyExists(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "already exists")
}

// isNotFound matches xray's "User <email> not found." AlterInbound
// error, delivered as codes.Unknown so the message text is the only
// signal. Requiring the "user" subject keeps handler/tag-level
// "not found" errors (failed to get handler: <tag>) passing through.
func isNotFound(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "user ") && strings.Contains(msg, " not found")
}
