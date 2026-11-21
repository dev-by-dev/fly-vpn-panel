package inbounds

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"flypanel/internal/store"
)

// apiInbound mirrors inboundResponse for decoding test responses.
type apiInbound struct {
	ID        int64           `json:"id"`
	Tag       string          `json:"tag"`
	Enabled   bool            `json:"enabled"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
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

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return e.Error
}

func decodeInbound(t *testing.T, rec *httptest.ResponseRecorder) apiInbound {
	t.Helper()
	var in apiInbound
	if err := json.Unmarshal(rec.Body.Bytes(), &in); err != nil {
		t.Fatalf("decode inbound response %q: %v", rec.Body.String(), err)
	}
	return in
}

func configField(t *testing.T, in apiInbound, key string) any {
	t.Helper()
	var cfg map[string]any
	if err := json.Unmarshal(in.Config, &cfg); err != nil {
		t.Fatalf("decode inbound config %s: %v", in.Config, err)
	}
	return cfg[key]
}

func TestRoutingNoSanitizer(t *testing.T) {
	env := newTestEnv(t, okValidate)

	routing := `{"domainStrategy":"AsIs","rules":[` +
		`{"type":"field","inboundTag":["api"],"domain":["example.com","*.example.com"],"outboundTag":"direct"},` +
		`{"type":"field","ip":["203.0.113.0/24","198.51.100.7"],"outboundTag":"block"},` +
		`{"type":"field","geoip":"cn","outboundTag":"block"}]}`

	rec := doJSON(t, env.srv, "PUT", "/api/config/routing", routing)
	if rec.Code != http.StatusOK {
		t.Fatalf("put routing: status %d, body %s", rec.Code, rec.Body.String())
	}

	// The stored routing must come back byte-for-byte: no re-marshal, no
	// sanitizing of domain/ip/geoip rules (spec §6.5 full config control).
	rec = doJSON(t, env.srv, "GET", "/api/config/routing", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get routing: status %d", rec.Code)
	}
	if rec.Body.String() != routing {
		t.Fatalf("routing round-trip mismatch:\n got: %s\nwant: %s", rec.Body.String(), routing)
	}

	// The PUT also applied: the on-disk config carries the routing.
	rec = doJSON(t, env.srv, "GET", "/api/config", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get config: status %d, body %s", rec.Code, rec.Body.String())
	}
	if !json.Valid(rec.Body.Bytes()) || !strings.Contains(rec.Body.String(), "example.com") {
		t.Fatalf("applied config missing routing: %s", rec.Body.String())
	}
}

func TestInvalidRouting422(t *testing.T) {
	env := newTestEnv(t, okValidate)
	old := `{"domainStrategy":"AsIs","rules":[]}`
	if rec := doJSON(t, env.srv, "PUT", "/api/config/routing", old); rec.Code != http.StatusOK {
		t.Fatalf("seed routing: status %d, body %s", rec.Code, rec.Body.String())
	}

	rec := doJSON(t, env.srv, "PUT", "/api/config/routing", `{"domainStrategy":`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("broken json: status %d, want 422, body %s", rec.Code, rec.Body.String())
	}
	if errCode(t, rec) != "invalid_json" {
		t.Fatalf("error = %q, want invalid_json", errCode(t, rec))
	}

	// The old value must be intact after the rejected PUT.
	rec = doJSON(t, env.srv, "GET", "/api/config/routing", "")
	if rec.Body.String() != old {
		t.Fatalf("routing after rejected put = %s, want %s", rec.Body.String(), old)
	}
}

func TestRoutingApplyFailureRollsBack(t *testing.T) {
	env := newTestEnv(t, failValidate)
	old := `{"domainStrategy":"IPIfNonEqualMatch","rules":[]}`
	if err := env.st.SettingSet("routing", old); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, env.srv, "PUT", "/api/config/routing", `{"domainStrategy":"AsIs","rules":[]}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422, body %s", rec.Code, rec.Body.String())
	}
	if errCode(t, rec) != "apply_failed" {
		t.Fatalf("error = %q, want apply_failed", errCode(t, rec))
	}

	// Apply failed, so the previous routing value is restored intact.
	rec = doJSON(t, env.srv, "GET", "/api/config/routing", "")
	if rec.Body.String() != old {
		t.Fatalf("routing after failed apply = %s, want old value %s", rec.Body.String(), old)
	}
}

func TestInboundCRUD(t *testing.T) {
	env := newTestEnv(t, okValidate)

	body := `{"tag":"in-a","enabled":true,"config":{"port":443,"protocol":"vless","settings":{}}}`
	rec := doJSON(t, env.srv, "POST", "/api/inbounds", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body %s", rec.Code, rec.Body.String())
	}
	in := decodeInbound(t, rec)
	if in.ID == 0 || in.Tag != "in-a" || !in.Enabled {
		t.Fatalf("created inbound = %+v", in)
	}
	if port, ok := configField(t, in, "port").(float64); !ok || port != 443 {
		t.Fatalf("created config port = %v, want 443", configField(t, in, "port"))
	}
	if tag, _ := configField(t, in, "tag").(string); tag != "in-a" {
		t.Fatalf("created config tag = %q, want in-a (must match store tag)", tag)
	}
	// The create applied the new config to xray.
	waitStarts(t, env.counter, 1, 2*time.Second)

	// Duplicate tag → 400, and nothing extra is persisted.
	rec = doJSON(t, env.srv, "POST", "/api/inbounds", body)
	if rec.Code != http.StatusBadRequest || errCode(t, rec) != "duplicate_tag" {
		t.Fatalf("duplicate tag: status %d, body %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, env.srv, "GET", "/api/inbounds", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status %d, body %s", rec.Code, rec.Body.String())
	}
	var list []apiInbound
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}

	// Patch the port.
	target := "/api/inbounds/" + strconv.FormatInt(in.ID, 10)
	rec = doJSON(t, env.srv, "PATCH", target,
		`{"config":{"port":8443,"protocol":"vless","settings":{}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: status %d, body %s", rec.Code, rec.Body.String())
	}
	patched := decodeInbound(t, rec)
	if port, ok := configField(t, patched, "port").(float64); !ok || port != 8443 {
		t.Fatalf("patched config port = %v, want 8443", configField(t, patched, "port"))
	}
	db, err := env.st.InboundGet(in.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(db.ConfigJSON, `"port":8443`) {
		t.Fatalf("db config after patch = %s, want port 8443", db.ConfigJSON)
	}

	rec = doJSON(t, env.srv, "GET", target, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get: status %d, body %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, env.srv, "DELETE", target, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: status %d, body %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, env.srv, "GET", target, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: status %d, want 404", rec.Code)
	}
	rec = doJSON(t, env.srv, "GET", "/api/inbounds", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("list after delete len = %d, want 0", len(list))
	}
}

func TestKeysRegen(t *testing.T) {
	env := newTestEnv(t, okValidate)

	oldPriv := strings.Repeat("a", 43)
	oldPub := strings.Repeat("b", 43)
	body := `{"tag":"vless-xhttp","enabled":true,"config":{"port":8443,"protocol":"vless","settings":{},` +
		`"streamSettings":{"security":"reality","realitySettings":{"dest":"www.microsoft.com:443",` +
		`"serverNames":["www.microsoft.com"],"privateKey":"` + oldPriv + `",` +
		`"publicKey":"` + oldPub + `","shortIds":[""]}}}}`
	rec := doJSON(t, env.srv, "POST", "/api/inbounds", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body %s", rec.Code, rec.Body.String())
	}
	in := decodeInbound(t, rec)

	rec = doJSON(t, env.srv, "POST", "/api/inbounds/"+strconv.FormatInt(in.ID, 10)+"/keys", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("regen keys: status %d, body %s", rec.Code, rec.Body.String())
	}
	var pair struct {
		PrivateKey string `json:"private_key"`
		PublicKey  string `json:"public_key"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pair); err != nil {
		t.Fatal(err)
	}
	if len(pair.PrivateKey) != 43 || len(pair.PublicKey) != 43 {
		t.Fatalf("key lens = %d/%d, want 43/43", len(pair.PrivateKey), len(pair.PublicKey))
	}
	if pair.PrivateKey == oldPriv || pair.PublicKey == oldPub {
		t.Fatalf("new pair equals old: %q / %q", pair.PrivateKey, pair.PublicKey)
	}
	if pair.PrivateKey == pair.PublicKey {
		t.Fatal("private and public key must differ")
	}

	db, err := env.st.InboundGet(in.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(db.ConfigJSON, pair.PrivateKey) {
		t.Fatalf("new private key not persisted in config_json: %s", db.ConfigJSON)
	}
}

// TestPutNullRouting422: "null" is syntactically valid JSON but not a
// routing object; the PUT must reject it with 422 BEFORE SettingSet, so
// the old value survives and Apply never panics (a panic would skip the
// rollback and brick the panel on every later apply).
func TestPutNullRouting422(t *testing.T) {
	env := newTestEnv(t, okValidate)
	old := `{"domainStrategy":"AsIs","rules":[]}`
	if rec := doJSON(t, env.srv, "PUT", "/api/config/routing", old); rec.Code != http.StatusOK {
		t.Fatalf("seed routing: status %d, body %s", rec.Code, rec.Body.String())
	}

	rec := doJSON(t, env.srv, "PUT", "/api/config/routing", "null")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("put null routing: status %d, want 422, body %s", rec.Code, rec.Body.String())
	}
	if errCode(t, rec) != "invalid_routing" {
		t.Fatalf("error = %q, want invalid_routing", errCode(t, rec))
	}
	// The stored routing must be untouched (no SettingSet, no rollback).
	rec = doJSON(t, env.srv, "GET", "/api/config/routing", "")
	if rec.Body.String() != old {
		t.Fatalf("routing after null put = %s, want %s", rec.Body.String(), old)
	}

	// Any other non-object value gets the same treatment.
	rec = doJSON(t, env.srv, "PUT", "/api/config/routing", `[]`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("put array routing: status %d, want 422, body %s", rec.Code, rec.Body.String())
	}
}

// TestCreateRealityNoKeysAutogen: a freshly created Reality inbound
// without keys would be rejected by `xray run -test` (apply → 502 on
// every UI create); Create must generate the pair before storing.
func TestCreateRealityNoKeysAutogen(t *testing.T) {
	env := newTestEnv(t, okValidate)

	body := `{"tag":"in-reality","enabled":true,"config":{"port":8443,"protocol":"vless","settings":{},` +
		`"streamSettings":{"security":"reality","realitySettings":{"dest":"www.microsoft.com:443",` +
		`"serverNames":["www.microsoft.com"],"shortIds":[""]}}}}`
	rec := doJSON(t, env.srv, "POST", "/api/inbounds", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body %s", rec.Code, rec.Body.String())
	}
	in := decodeInbound(t, rec)
	var cfg map[string]any
	if err := json.Unmarshal(in.Config, &cfg); err != nil {
		t.Fatalf("decode created config: %v", err)
	}
	rs := realitySettings(cfg)
	if rs == nil {
		t.Fatal("realitySettings missing in created config")
	}
	priv, _ := rs["privateKey"].(string)
	pub, _ := rs["publicKey"].(string)
	if len(priv) != 43 || len(pub) != 43 {
		t.Fatalf("auto-generated keys = %q/%q, want a 43-char X25519 pair", priv, pub)
	}
	if priv == pub {
		t.Fatal("private and public key must differ")
	}
	db, err := env.st.InboundGet(in.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(db.ConfigJSON, priv) {
		t.Fatalf("generated key not persisted: %s", db.ConfigJSON)
	}
	// The create applied: xray (fake) started exactly once.
	waitStarts(t, env.counter, 1, 2*time.Second)
}

// TestPatchTagRenormalizesConfig: a tag-only patch must re-normalize
// the stored config under the new tag so store tag == config tag
// (membership sync depends on it).
func TestPatchTagRenormalizesConfig(t *testing.T) {
	env := newTestEnv(t, okValidate)
	rec := doJSON(t, env.srv, "POST", "/api/inbounds",
		`{"tag":"in-old","enabled":true,"config":{"port":443,"protocol":"vless","settings":{}}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status %d, body %s", rec.Code, rec.Body.String())
	}
	in := decodeInbound(t, rec)

	rec = doJSON(t, env.srv, "PATCH", "/api/inbounds/"+strconv.FormatInt(in.ID, 10), `{"tag":"in-new"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch tag: status %d, body %s", rec.Code, rec.Body.String())
	}
	patched := decodeInbound(t, rec)
	if patched.Tag != "in-new" {
		t.Fatalf("tag = %q, want in-new", patched.Tag)
	}
	if tag, _ := configField(t, patched, "tag").(string); tag != "in-new" {
		t.Fatalf("config tag = %q, want in-new (must follow the store tag)", tag)
	}
	db, err := env.st.InboundGet(in.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(db.ConfigJSON, `"tag":"in-new"`) {
		t.Fatalf("db config tag not renormalized: %s", db.ConfigJSON)
	}
}

func TestInboundApplyFailure502(t *testing.T) {
	env := newTestEnv(t, failValidate)

	rec := doJSON(t, env.srv, "POST", "/api/inbounds",
		`{"tag":"in-a","enabled":true,"config":{"port":443,"protocol":"vless","settings":{}}}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("create: status %d, want 502, body %s", rec.Code, rec.Body.String())
	}
	if errCode(t, rec) != "apply_failed" {
		t.Fatalf("error = %q, want apply_failed", errCode(t, rec))
	}
	// The row stays: the DB is the source of truth, no rollback.
	inbounds, err := env.st.InboundList()
	if err != nil {
		t.Fatal(err)
	}
	if len(inbounds) != 1 || inbounds[0].Tag != "in-a" {
		t.Fatalf("inbound row not persisted: %+v", inbounds)
	}

	// Keys regen: the new pair is persisted, apply fails → 502, DB keeps
	// the new keys for the next successful apply.
	oldPriv := strings.Repeat("c", 43)
	reality := `{"tag":"in-r","port":8443,"protocol":"vless","settings":{},` +
		`"streamSettings":{"security":"reality","realitySettings":` +
		`{"dest":"www.microsoft.com:443","privateKey":"` + oldPriv + `","publicKey":"` + oldPriv + `"}}}`
	in := &store.Inbound{
		Tag: "in-r", Enabled: true, ConfigJSON: reality, CreatedAt: time.Now().UTC(),
	}
	if err := env.st.InboundCreate(in); err != nil {
		t.Fatal(err)
	}
	rec = doJSON(t, env.srv, "POST", "/api/inbounds/"+strconv.FormatInt(in.ID, 10)+"/keys", "")
	if rec.Code != http.StatusBadGateway || errCode(t, rec) != "apply_failed" {
		t.Fatalf("regen keys: status %d, body %s", rec.Code, rec.Body.String())
	}
	db, err := env.st.InboundGet(in.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(db.ConfigJSON, oldPriv) {
		t.Fatalf("old key still present after regen: %s", db.ConfigJSON)
	}
	if !strings.Contains(db.ConfigJSON, `"privateKey"`) {
		t.Fatalf("new key not persisted: %s", db.ConfigJSON)
	}
}
