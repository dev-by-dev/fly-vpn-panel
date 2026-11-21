// Package api wires every HTTP surface of the panel into one stdlib
// router: the public auth and subscription endpoints, the
// JWT-protected admin API, and the embedded SPA.
//
// 502 semantics: the users and inbounds mutation endpoints answer
// 502 {"error":"sync_failed"} or {"error":"apply_failed"} when the
// store row is already committed but pushing the change to the running
// xray failed. In that state the database is the source of truth —
// the stats poller self-heals xray membership on its next ticks — and
// a client must re-read state with GET (or amend it with PATCH)
// instead of retrying the same POST: a retried create would duplicate
// the committed row.
package api

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"flypanel/internal/auth"
	"flypanel/internal/config"
	"flypanel/internal/inbounds"
	"flypanel/internal/stats"
	"flypanel/internal/store"
	"flypanel/internal/sub"
	"flypanel/internal/users"
	"flypanel/internal/xray"
	"flypanel/internal/xrayclient"
	"flypanel/web"
)

// Manager is the xray process surface the API needs. It is satisfied
// by *xray.Manager; tests substitute a fake.
type Manager interface {
	Status() xray.Status
	Start() error
	Stop() error
	Restart() error
}

// Deps carries the router dependencies. Manager is an interface (not
// the concrete *xray.Manager) so tests can fake the xray lifecycle.
type Deps struct {
	Store   *store.Store
	JWT     *auth.JWT
	RL      *auth.RateLimiter
	Client  *xrayclient.Client
	Manager Manager
	Cfg     *config.Config
	Poller  *stats.Poller
}

// NewRouter builds the full panel HTTP handler. Only
// POST /api/auth/login|refresh|logout and GET /sub/{token}[/{format}]
// are public; every other /api/* path sits behind auth.Middleware.
func NewRouter(d Deps) http.Handler {
	authH := &auth.Handlers{
		Store: d.Store, JWT: d.JWT, RL: d.RL,
		SecureCookies: d.Cfg.CookieSecure,
	}
	interval := PollInterval(d.Store, d.Cfg)
	usersH := &users.Handlers{Store: d.Store, Client: d.Client, PollInterval: interval}
	subH := &sub.Handlers{Store: d.Store, EntryIP: d.Cfg.EntryIP}
	apiH := &Handlers{
		Store: d.Store, Mgr: d.Manager, Poller: d.Poller,
		PanelDomain: d.Cfg.PanelDomain, EntryIP: d.Cfg.EntryIP, interval: interval,
		Client:  d.Client,
		DataDir: d.Cfg.DataDir,
	}

	mux := http.NewServeMux()

	// Public: session endpoints and subscription delivery.
	mux.HandleFunc("POST /api/auth/login", authH.Login)
	mux.HandleFunc("POST /api/auth/refresh", authH.Refresh)
	mux.HandleFunc("POST /api/auth/logout", authH.Logout)
	mux.HandleFunc("GET /sub/{token}", subH.Sub)
	mux.HandleFunc("GET /sub/{token}/{format}", subH.Sub)

	// Everything else under /api/ requires a valid fp_access cookie.
	p := http.NewServeMux()
	p.HandleFunc("POST /api/auth/2fa/enroll", authH.Enroll2FA)
	p.HandleFunc("POST /api/auth/2fa/verify", authH.Verify2FA)
	p.HandleFunc("POST /api/auth/2fa/disable", authH.Disable2FA)
	p.HandleFunc("POST /api/auth/password", authH.ChangePassword)

	p.HandleFunc("GET /api/users", usersH.List)
	p.HandleFunc("POST /api/users", usersH.Create)
	p.HandleFunc("GET /api/users/{id}", usersH.Get)
	p.HandleFunc("PATCH /api/users/{id}", usersH.Patch)
	p.HandleFunc("DELETE /api/users/{id}", usersH.Delete)
	p.HandleFunc("POST /api/users/{id}/reset-quota", usersH.ResetQuota)
	p.HandleFunc("GET /api/users/{id}/sub", apiH.UserSub)

	// The inbounds applier needs the concrete *xray.Manager; when a
	// fake manager is injected (tests) these routes stay unmounted and
	// fall through to the JSON 404 below.
	if mgr, ok := d.Manager.(*xray.Manager); ok {
		ib := &inbounds.Handlers{Store: d.Store, Applier: inbounds.NewApplier(d.Store, d.Cfg, mgr)}
		p.HandleFunc("GET /api/inbounds/metrics", apiH.InboundMetrics)
		p.HandleFunc("GET /api/inbounds", ib.List)
		p.HandleFunc("POST /api/inbounds", ib.Create)
		p.HandleFunc("GET /api/inbounds/{id}", ib.Get)
		p.HandleFunc("PATCH /api/inbounds/{id}", ib.Patch)
		p.HandleFunc("DELETE /api/inbounds/{id}", ib.Delete)
		p.HandleFunc("POST /api/inbounds/{id}/keys", ib.RegenKeys)
		p.HandleFunc("GET /api/config", ib.GetConfig)
		p.HandleFunc("GET /api/config/routing", ib.GetRouting)
		p.HandleFunc("PUT /api/config/routing", ib.PutRouting)
	}

	p.HandleFunc("GET /api/templates/{format}", subH.TemplateGet)
	p.HandleFunc("PUT /api/templates/{format}", subH.TemplatePut)

	p.HandleFunc("GET /api/xray/status", apiH.XrayStatus)
	p.HandleFunc("POST /api/xray/start", apiH.XrayStart)
	p.HandleFunc("POST /api/xray/stop", apiH.XrayStop)
	p.HandleFunc("POST /api/xray/restart", apiH.XrayRestart)
	p.HandleFunc("GET /api/stats/traffic", apiH.StatsTraffic)
	p.HandleFunc("GET /api/stats/live", apiH.StatsLive)
	p.HandleFunc("GET /api/stats/online", apiH.StatsOnline)
	p.HandleFunc("GET /api/devices", apiH.Devices)
	p.HandleFunc("GET /api/stats/summary", apiH.StatsSummary)
	p.HandleFunc("GET /api/system", apiH.System)
	p.HandleFunc("GET /api/settings", apiH.SettingsGet)
	p.HandleFunc("PUT /api/settings", apiH.SettingsPut)

	// Unknown /api/* paths answer JSON, never HTML (404 with a valid
	// session; without one the middleware answers 401 first).
	p.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		jsonError(w, http.StatusNotFound, "not found")
	})

	mux.Handle("/api/", auth.Middleware(d.JWT)(p))
	mux.HandleFunc("/", serveSPA)
	return mux
}

// distFS is the embedded SPA tree (web/dist).
var distFS = func() fs.FS {
	sub, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		panic("api: embedded web/dist: " + err.Error())
	}
	return sub
}()

// serveSPA serves the embedded frontend: the exact file when the path
// exists in dist, otherwise index.html (client-side routing fallback).
func serveSPA(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	data, err := fs.ReadFile(distFS, name)
	if err != nil {
		data, err = fs.ReadFile(distFS, "index.html")
		if err != nil {
			http.Error(w, "spa not built", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	} else if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
