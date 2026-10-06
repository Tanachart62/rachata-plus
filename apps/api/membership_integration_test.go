package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Runs inside the existing opt-in integration test's isolated temporary schema.
func testMembershipManagement(t *testing.T, ctx context.Context, db *pgxpool.Pool, a *authConfig) {
	t.Helper()
	password, err := hashPassword("membership-test-password")
	if err != nil {
		t.Fatal(err)
	}
	create := func(username, role string) (int64, *http.Cookie) {
		t.Helper()
		var id int64
		err := db.QueryRow(ctx, `INSERT INTO users(username,name,email,password_hash,role) VALUES($1,$1,$2,$3,$4) RETURNING id`, username, username+"@example.com", password, role).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		token, hash, err := newSessionToken()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,NOW()+interval '1 day')`, hash, id); err != nil {
			t.Fatal(err)
		}
		return id, &http.Cookie{Name: sessionCookie, Value: token}
	}
	actor, adminCookie := create("membershipadmin", "admin")
	target, userCookie := create("membershipuser", "user")
	router := gin.New()
	group := router.Group("/auth")
	group.Use(a.protectWrites())
	a.accountRoutes(group, db)
	group.GET("/me", a.meHandler(db))
	group.POST("/login", a.loginHandler(db))
	var seq atomic.Int64
	perform := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.RemoteAddr = fmt.Sprintf("10.90.0.%d:1234", seq.Add(1))
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0) Chrome/130.0 Safari/537.36 Edg/130.0")
		req.Header.Set("Origin", a.origin)
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	check := func(method, path, body string, cookie *http.Cookie, want int) *httptest.ResponseRecorder {
		t.Helper()
		w := perform(method, path, body, cookie)
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	read := func(w *httptest.ResponseRecorder) accountResponse {
		t.Helper()
		var d struct {
			User accountResponse `json:"user"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
		return d.User
	}
	listPath := "/auth/admin/memberships/accounts"
	check("GET", listPath, "", nil, 401)
	check("GET", listPath, "", userCookie, 403)
	check("GET", listPath+"?cursor=bad", "", adminCookie, 400)
	if _, err = db.Exec(ctx, `INSERT INTO users(username,name,email,password_hash)
      SELECT 'listfixture_'||n,'List Fixture '||n,'listfixture_'||n||'@example.com',$1 FROM generate_series(1,55) n`, password); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Users []accountResponse `json:"users"`
		Next  string            `json:"nextCursor"`
	}
	w := check("GET", listPath+"?q=listfixture_", "", adminCookie, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Users) != 50 || page.Next == "" {
		t.Fatal("first account page", err)
	}
	seen := make(map[int64]bool)
	for _, u := range page.Users {
		if u.Member || seen[u.ID] {
			t.Fatal("invalid listed free account")
		}
		seen[u.ID] = true
	}
	w = check("GET", listPath+"?q=listfixture_&cursor="+page.Next, "", adminCookie, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Users) != 5 || page.Next != "" {
		t.Fatal("second account page", err)
	}
	for _, u := range page.Users {
		if seen[u.ID] {
			t.Fatal("duplicate account page")
		}
		seen[u.ID] = true
	}
	if len(seen) != 55 {
		t.Fatal("account pagination omitted rows")
	}
	w = check("GET", listPath+"?q=MEMBERSHIPUSER", "", adminCookie, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Users) != 1 || page.Users[0].ID != target {
		t.Fatal("case-insensitive account search", err)
	}
	w = check("GET", listPath+"?q=%25", "", adminCookie, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Users) != 0 {
		t.Fatal("search treated percent as wildcard", err)
	}
	path := fmt.Sprintf("/auth/admin/memberships/%d", target)
	body := func(action, plan string, days int, revision int64) string {
		return fmt.Sprintf(`{"expectedUserId":%d,"action":%q,"plan":%q,"days":%d,"expectedRevision":%d}`, actor, action, plan, days, revision)
	}
	lookup := "/auth/admin/memberships?email=membershipuser@example.com"
	check("GET", lookup, "", nil, 401)
	check("GET", lookup, "", userCookie, 403)
	check("POST", path, body("grant", "basic", 30, 0), userCookie, 403)
	check("GET", "/auth/admin/memberships?email=missing@example.com", "", adminCookie, 404)
	initial := read(check("GET", lookup, "", adminCookie, 200))
	if initial.Member || initial.Plan != "" || initial.MembershipRevision != 0 {
		t.Fatal("new user has membership")
	}
	check("POST", path, body("grant", "free", 30, 0), adminCookie, 400)
	check("POST", path, body("grant", "basic", 0, 0), adminCookie, 400)
	check("POST", path, body("grant", "basic", 366, 0), adminCookie, 400)
	staleActor := strings.Replace(body("grant", "basic", 30, 0), fmt.Sprintf(`"expectedUserId":%d`, actor), fmt.Sprintf(`"expectedUserId":%d`, target), 1)
	check("POST", path, staleActor, adminCookie, 409)
	granted := read(check("POST", path, body("grant", "standard", 30, 0), adminCookie, 200))
	if !granted.Member || granted.Plan != "standard" || granted.ExpiresAt < time.Now().Add(29*24*time.Hour).UnixMilli() || granted.MembershipRevision != 1 {
		t.Fatal("incorrect grant", granted)
	}
	me := read(check("GET", "/auth/me", "", userCookie, 200))
	if me.Plan != granted.Plan || me.ExpiresAt != granted.ExpiresAt {
		t.Fatal("me did not expose persisted membership")
	}
	login := read(check("POST", "/auth/login", `{"email":"membershipuser@example.com","password":"membership-test-password"}`, nil, 200))
	if login.Plan != granted.Plan || login.ExpiresAt != granted.ExpiresAt {
		t.Fatal("login metadata missing")
	}
	profileBody := fmt.Sprintf(`{"expectedUserId":%d,"name":"Membership User","email":"membershipuser@example.com"}`, target)
	profile := read(check("POST", "/auth/profile", profileBody, userCookie, 200))
	if profile.Plan != granted.Plan || profile.ExpiresAt != granted.ExpiresAt {
		t.Fatal("profile lost membership")
	}
	renewed := read(check("POST", path, body("grant", "premium", 7, 1), adminCookie, 200))
	if renewed.ExpiresAt != granted.ExpiresAt+7*24*time.Hour.Milliseconds() || renewed.Plan != "premium" {
		t.Fatal("renewal discarded remaining time")
	}
	check("POST", path, body("grant", "basic", 1, 1), adminCookie, 409)
	revoked := read(check("POST", path, body("revoke", "", 0, 2), adminCookie, 200))
	if revoked.Member || revoked.Plan != "" || revoked.ExpiresAt != 0 {
		t.Fatal("revoke did not remove membership")
	}
	progress := fmt.Sprintf(`{"expectedUserId":%d,"position":10,"expectedRevision":0}`, target)
	check("POST", "/auth/history/1", progress, userCookie, 403)
	check("POST", path, body("grant", "basic", 1, 3), adminCookie, 200)
	if _, err = db.Exec(ctx, `UPDATE subscriptions SET starts_at=NOW()-interval '2 days',expires_at=NOW()-interval '1 day' WHERE user_id=$1 AND status='active'`, target); err != nil {
		t.Fatal(err)
	}
	if read(check("GET", "/auth/me", "", userCookie, 200)).Member {
		t.Fatal("expired member remained active")
	}
	check("POST", "/auth/history/1", progress, userCookie, 403)
	// Two concurrent submissions from the same revision must grant once.
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- perform("POST", path, body("grant", "basic", 1, 4), adminCookie).Code }()
	}
	first, second := <-results, <-results
	if !((first == 200 && second == 409) || (first == 409 && second == 200)) {
		t.Fatal("concurrent grants", first, second)
	}
	var count int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM subscriptions WHERE user_id=$1 AND status='active'`, target).Scan(&count); err != nil || count != 1 {
		t.Fatal("overlapping active grants", count, err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM membership_events WHERE user_id=$1 AND actor_id=$2`, target, actor).Scan(&count); err != nil || count != 5 {
		t.Fatal("audit missing or duplicated", count, err)
	}
	// Future subscriptions do not grant early access.
	if _, err = db.Exec(ctx, `UPDATE subscriptions SET starts_at=NOW()+interval '1 hour',expires_at=NOW()+interval '2 hours' WHERE user_id=$1 AND status='active'`, target); err != nil {
		t.Fatal(err)
	}
	if read(check("GET", "/auth/me", "", userCookie, 200)).Member {
		t.Fatal("future membership started early")
	}
	// Suspension preserves membership/data, but invalidates every device.
	if _, err = db.Exec(ctx, `UPDATE subscriptions SET starts_at=NOW()-interval '1 minute',expires_at=NOW()+interval '7 days' WHERE user_id=$1 AND status='active'`, target); err != nil {
		t.Fatal(err)
	}
	before := read(check("GET", lookup, "", adminCookie, 200))
	loginBody := `{"email":"membershipuser@example.com","password":"membership-test-password"}`
	otherLogin := check("POST", "/auth/login", loginBody, nil, 200)
	otherCookie := otherLogin.Result().Cookies()[1]
	check("POST", fmt.Sprintf("/auth/admin/memberships/%d", actor), body("suspend", "", 0, 0), adminCookie, 400)
	check("POST", path, body("suspend", "", 0, 5), userCookie, 403)
	suspended := read(check("POST", path, body("suspend", "", 0, 5), adminCookie, 200))
	if suspended.AccountStatus != "suspended" || suspended.Plan != before.Plan || suspended.ExpiresAt != before.ExpiresAt {
		t.Fatal("suspension changed membership", suspended)
	}
	check("GET", "/auth/me", "", userCookie, 401)
	check("GET", "/auth/me", "", otherCookie, 401)
	check("POST", "/auth/history/1", progress, userCookie, 401)
	check("POST", "/auth/login", loginBody, nil, 403)
	check("POST", "/auth/login", strings.Replace(loginBody, "membership-test-password", "wrong-password", 1), nil, 401)
	check("POST", path, body("grant", "basic", 1, 6), adminCookie, 409)
	if err = db.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1`, target).Scan(&count); err != nil || count != 0 {
		t.Fatal("suspended sessions remain", count, err)
	}
	restored := read(check("POST", path, body("reactivate", "", 0, 6), adminCookie, 200))
	if restored.AccountStatus != "active" || restored.Plan != before.Plan || restored.ExpiresAt != before.ExpiresAt {
		t.Fatal("restore lost metadata")
	}
	check("GET", "/auth/me", "", otherCookie, 401)
	firstDevice := check("POST", "/auth/login", loginBody, nil, 200).Result().Cookies()[1]
	secondDevice := check("POST", "/auth/login", loginBody, nil, 200).Result().Cookies()[1]
	check("POST", path, body("logout_all", "", 0, 7), adminCookie, 200)
	check("GET", "/auth/me", "", firstDevice, 401)
	check("GET", "/auth/me", "", secondDevice, 401)
	check("POST", path, body("logout_all", "", 0, 7), adminCookie, 409)
	check("POST", "/auth/login", loginBody, nil, 200)
	if err = db.QueryRow(ctx, `SELECT count(*) FROM membership_events WHERE user_id=$1 AND actor_id=$2`, target, actor).Scan(&count); err != nil || count != 8 {
		t.Fatal("account control audit", count, err)
	}

	// Individual revocation only affects the chosen session of the target user.
	keepCookie := check("POST", "/auth/login", loginBody, nil, 200).Result().Cookies()[1]
	removeCookie := check("POST", "/auth/login", loginBody, nil, 200).Result().Cookies()[1]
	devicePath := fmt.Sprintf("/auth/admin/memberships/%d/sessions", target)
	check("GET", devicePath, "", nil, 401)
	check("GET", devicePath, "", keepCookie, 403)
	var devices struct {
		Sessions []deviceSession `json:"sessions"`
		Next     string          `json:"nextCursor"`
	}
	w = check("GET", devicePath, "", adminCookie, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &devices); err != nil || len(devices.Sessions) < 2 {
		t.Fatal("session list", err)
	}
	chosen := devices.Sessions[0]
	if chosen.Device != "Edge · Windows" || chosen.Current {
		t.Fatal("incorrect device metadata", chosen)
	}
	if strings.Contains(w.Body.String(), "token_hash") || strings.Contains(w.Body.String(), removeCookie.Value) {
		t.Fatal("session list leaked token")
	}
	own := check("GET", fmt.Sprintf("/auth/admin/memberships/%d/sessions", actor), "", adminCookie, 200)
	var ownDevices struct {
		Sessions []deviceSession `json:"sessions"`
	}
	if err = json.Unmarshal(own.Body.Bytes(), &ownDevices); err != nil || len(ownDevices.Sessions) != 1 || !ownDevices.Sessions[0].Current {
		t.Fatal("current admin session marker", err)
	}
	payload := strings.TrimSuffix(body("logout_session", "", 0, 8), "}") + fmt.Sprintf(`,"sessionId":%q}`, ownDevices.Sessions[0].ID)
	check("POST", path, payload, adminCookie, 404)
	check("GET", "/auth/me", "", adminCookie, 200)
	payload = strings.TrimSuffix(body("logout_session", "", 0, 8), "}") + fmt.Sprintf(`,"sessionId":%q}`, chosen.ID)
	check("POST", path, payload, keepCookie, 403)
	check("POST", path, payload, adminCookie, 200)
	check("GET", "/auth/me", "", removeCookie, 401)
	check("GET", "/auth/me", "", keepCookie, 200)
	check("POST", path, payload, adminCookie, 409)
	payload = strings.TrimSuffix(body("logout_session", "", 0, 9), "}") + fmt.Sprintf(`,"sessionId":%q}`, chosen.ID)
	check("POST", path, payload, adminCookie, 404)
	if _, err = db.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at)
      SELECT decode(md5(n::text)||md5('device_'||n),'hex'),$1,NOW()+interval '1 day' FROM generate_series(1,55) n`, target); err != nil {
		t.Fatal(err)
	}
	w = check("GET", devicePath, "", adminCookie, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &devices); err != nil || len(devices.Sessions) != 50 || devices.Next == "" {
		t.Fatal("device pagination", err)
	}
	check("GET", devicePath+"?cursor="+devices.Next, "", adminCookie, 200)

	// Regular users can list/revoke only their own devices, never another owner.
	check("GET", "/auth/sessions", "", nil, 401)
	selfList := check("GET", "/auth/sessions", "", keepCookie, 200)
	if err = json.Unmarshal(selfList.Body.Bytes(), &devices); err != nil || len(devices.Sessions) != 50 {
		t.Fatal("own session list", err)
	}
	for _, device := range devices.Sessions {
		if device.ID == ownDevices.Sessions[0].ID {
			t.Fatal("own list exposed admin device")
		}
	}
	ownBody := fmt.Sprintf(`{"expectedUserId":%d}`, target)
	check("POST", "/auth/sessions/"+ownDevices.Sessions[0].ID+"/revoke", ownBody, keepCookie, 404)
	check("GET", "/auth/me", "", adminCookie, 200)
	selfChosen := devices.Sessions[0].ID
	check("POST", "/auth/sessions/"+selfChosen+"/revoke", fmt.Sprintf(`{"expectedUserId":%d}`, actor), keepCookie, 409)
	check("POST", "/auth/sessions/"+selfChosen+"/revoke", ownBody, keepCookie, 200)
	check("GET", "/auth/me", "", keepCookie, 200)
	check("POST", "/auth/sessions/"+selfChosen+"/revoke", ownBody, keepCookie, 404)
	var currentDevices struct {
		Sessions []deviceSession `json:"sessions"`
		Next     string          `json:"nextCursor"`
	}
	cursor := ""
	currentID := ""
	for {
		result := check("GET", "/auth/sessions?cursor="+cursor, "", keepCookie, 200)
		if err = json.Unmarshal(result.Body.Bytes(), &currentDevices); err != nil {
			t.Fatal(err)
		}
		for _, device := range currentDevices.Sessions {
			if device.Current {
				currentID = device.ID
			}
		}
		cursor = currentDevices.Next
		if cursor == "" {
			break
		}
	}
	if currentID == "" {
		t.Fatal("missing own current session")
	}
	check("POST", "/auth/sessions/"+currentID+"/revoke", ownBody, keepCookie, 200)
	check("GET", "/auth/me", "", keepCookie, 401)
	// An admin may explicitly sign out their own devices; the response succeeds,
	// subsequent admin requests cannot reuse the old session.
	check("POST", fmt.Sprintf("/auth/admin/memberships/%d", actor), body("logout_all", "", 0, 0), adminCookie, 200)
	check("GET", "/auth/me", "", adminCookie, 401)

}
