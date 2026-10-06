package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// Opt-in and loopback only. A temporary schema keeps existing local data intact.
func TestLocalAuthIntegration(t *testing.T) {
	if os.Getenv("LOCAL_AUTH_INTEGRATION") != "1" {
		t.Skip("set LOCAL_AUTH_INTEGRATION=1 after starting local PostgreSQL")
	}
	env, err := godotenv.Read("../../.env.local")
	if err != nil {
		t.Fatal("missing .env.local; follow README.md")
	}
	if env["DB_HOST"] != "127.0.0.1" && env["DB_HOST"] != "localhost" && env["DB_HOST"] != "::1" {
		t.Fatal("integration tests allow only loopback DB_HOST")
	}
	dbURL := url.URL{Scheme: "postgres", User: url.UserPassword(env["POSTGRES_USER"], env["POSTGRES_PASSWORD"]), Host: net.JoinHostPort(env["DB_HOST"], env["POSTGRES_PORT"]), Path: "/" + env["POSTGRES_DB"]}
	dbURL.RawQuery = "sslmode=disable"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dbURL.String())
	if err != nil {
		t.Fatal("cannot configure local pool")
	}
	defer admin.Close()
	if err = admin.Ping(ctx); err != nil {
		t.Fatal("local PostgreSQL unavailable; start Docker database first")
	}
	schema := fmt.Sprintf("auth_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("temporary schema cleanup failed: %v", err)
		}
	}()
	config, err := pgxpool.ParseConfig(dbURL.String())
	if err != nil {
		t.Fatal("cannot configure test pool")
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, file := range []string{"create_videos.sql", "create_users_and_subscriptions.sql", "add_usernames_and_sessions.sql", "add_local_video_pipeline.sql", "add_account_library.sql", "add_account_consistency.sql", "add_membership_management.sql", "add_account_controls.sql", "add_session_devices.sql"} {
		sql, err := os.ReadFile("../../db/migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("migration %s: %v", file, err)
		}
	}
	gin.SetMode(gin.TestMode)
	a, err := newAuthConfig("http://localhost:5173", "false")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	group := router.Group("/auth")
	group.Use(a.protectWrites())
	group.POST("/register", registerHandler(db))
	group.POST("/login", a.loginHandler(db))
	group.POST("/logout", a.logoutHandler(db))
	group.GET("/me", a.meHandler(db))
	a.accountRoutes(group, db)
	requestNumber := 0
	request := func(method, path, body string, cookie *http.Cookie, want int) *httptest.ResponseRecorder {
		t.Helper()
		if method == "POST" && path != "/auth/register" && path != "/auth/login" {
			var payload map[string]any
			if json.Unmarshal([]byte(body), &payload) == nil {
				var id int64 = 1
				if cookie != nil {
					hash := sha256.Sum256([]byte(cookie.Value))
					_ = db.QueryRow(ctx, `SELECT user_id FROM sessions WHERE token_hash=$1`, hash[:]).Scan(&id)
				}
				if _, exists := payload["expectedUserId"]; !exists {
					payload["expectedUserId"] = id
				}
				if strings.HasPrefix(path, "/auth/history/") {
					if _, exists := payload["expectedRevision"]; !exists {
						var revision int64
						_ = db.QueryRow(ctx, `SELECT revision FROM watch_history WHERE user_id=$1 AND video_id=1`, id).Scan(&revision)
						payload["expectedRevision"] = revision
					}
				}
				bytes, _ := json.Marshal(payload)
				body = string(bytes)
			}
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		requestNumber++
		req.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", requestNumber)
		req.Header.Set("Origin", a.origin)
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	request("GET", "/auth/me", "", nil, 401)
	signup := `{"username":"tester","name":"Local Tester","email":"test@example.com","password":"correct-test-password"}`
	request("POST", "/auth/register", signup, nil, 201)
	duplicate := request("POST", "/auth/register", signup, nil, 409)
	if !strings.Contains(duplicate.Body.String(), "already_exists") {
		t.Fatal("duplicate not identified")
	}
	request("POST", "/auth/register", strings.Replace(signup, "tester", "other", 1), nil, 409)
	request("POST", "/auth/login", `{"email":"test@example.com","password":"wrong-password"}`, nil, 401)
	request("POST", "/auth/login", `{"email":"missing@example.com","password":"wrong-password"}`, nil, 401)
	login := `{"email":"test@example.com","password":"correct-test-password"}`
	w := request("POST", "/auth/login", login, nil, 200)
	cookie := w.Result().Cookies()[1]
	me := request("GET", "/auth/me", "", cookie, 200)
	if !strings.Contains(me.Body.String(), `"username":"tester"`) || !strings.Contains(me.Body.String(), `"member":false`) {
		t.Fatal("incorrect account or membership")
	}
	w = request("POST", "/auth/login", login, cookie, 200)
	rotated := w.Result().Cookies()[1]
	if rotated.Value == cookie.Value {
		t.Fatal("session token was not rotated")
	}
	request("GET", "/auth/me", "", cookie, 401)
	request("GET", "/auth/me", "", rotated, 200)
	request("POST", "/auth/logout", "{}", rotated, 204)
	request("GET", "/auth/me", "", rotated, 401)
	w = request("POST", "/auth/login", login, nil, 200)
	expired := w.Result().Cookies()[1]
	if _, err = db.Exec(ctx, `UPDATE sessions SET created_at=NOW()-interval '2 days', expires_at=NOW()-interval '1 day'`); err != nil {
		t.Fatal(err)
	}
	request("GET", "/auth/me", "", expired, 401)

	// Account changes require a live session and preserve database identity.
	w = request("POST", "/auth/login", login, nil, 200)
	primary := w.Result().Cookies()[1]
	w = request("POST", "/auth/login", login, nil, 200)
	otherDevice := w.Result().Cookies()[1]
	request("POST", "/auth/profile", `{"name":"Updated Tester","email":"test@example.com"}`, nil, 401)
	request("POST", "/auth/profile", `{"name":"Updated Tester","email":"test@example.com"}`, primary, 200)
	me = request("GET", "/auth/me", "", primary, 200)
	if !strings.Contains(me.Body.String(), `"name":"Updated Tester"`) {
		t.Fatal("profile was not persisted")
	}
	request("POST", "/auth/profile", `{"name":"Updated Tester","email":"new@example.com","currentPassword":"wrong"}`, primary, 400)
	w = request("POST", "/auth/profile", `{"name":"Updated Tester","email":"new@example.com","currentPassword":"correct-test-password"}`, primary, 200)
	changedEmail := w.Result().Cookies()[1]
	request("GET", "/auth/me", "", primary, 401)
	request("GET", "/auth/me", "", otherDevice, 401)
	request("GET", "/auth/me", "", changedEmail, 200)
	request("POST", "/auth/login", login, nil, 401)
	login = `{"email":"new@example.com","password":"correct-test-password"}`
	w = request("POST", "/auth/login", login, nil, 200)
	otherDevice = w.Result().Cookies()[1]
	request("POST", "/auth/password", `{"currentPassword":"wrong","newPassword":"new-correct-test-password"}`, changedEmail, 400)
	w = request("POST", "/auth/password", `{"currentPassword":"correct-test-password","newPassword":"new-correct-test-password"}`, changedEmail, 204)
	passwordChanged := w.Result().Cookies()[1]
	request("GET", "/auth/me", "", changedEmail, 401)
	request("GET", "/auth/me", "", otherDevice, 401)
	request("GET", "/auth/me", "", passwordChanged, 200)
	request("POST", "/auth/login", login, nil, 401)
	login = `{"email":"new@example.com","password":"new-correct-test-password"}`
	request("POST", "/auth/login", login, nil, 200)
	var storedHash string
	if err = db.QueryRow(ctx, `SELECT password_hash FROM users WHERE username='tester'`).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(storedHash, "$argon2id$") || !verifyPassword("new-correct-test-password", storedHash) {
		t.Fatal("new password not hashed correctly")
	}

	request("POST", "/auth/register", `{"username":"another","name":"Another","email":"another@example.com","password":"another-test-password"}`, nil, 201)
	w = request("POST", "/auth/login", `{"email":"another@example.com","password":"another-test-password"}`, nil, 200)
	another := w.Result().Cookies()[1]
	request("POST", "/auth/profile", `{"name":"Should Roll Back","email":"another@example.com","currentPassword":"new-correct-test-password"}`, passwordChanged, 409)
	me = request("GET", "/auth/me", "", passwordChanged, 200)
	if strings.Contains(me.Body.String(), "Should Roll Back") || !strings.Contains(me.Body.String(), "new@example.com") {
		t.Fatal("duplicate email update was not atomic")
	}
	if _, err = db.Exec(ctx, `INSERT INTO videos(title,status,publication_status,duration_seconds) VALUES('Library Test','ready','published',120)`); err != nil {
		t.Fatal(err)
	}
	request("GET", "/auth/library", "", nil, 401)
	request("POST", "/auth/watchlist/1", `{"saved":true}`, passwordChanged, 204)
	request("POST", "/auth/watchlist/1", `{"saved":true}`, passwordChanged, 204)
	w = request("GET", "/auth/library", "", passwordChanged, 200)
	if !strings.Contains(w.Body.String(), `"saved":[1]`) {
		t.Fatal("watchlist not persisted/idempotent")
	}
	w = request("GET", "/auth/library", "", another, 200)
	if !strings.Contains(w.Body.String(), `"saved":[]`) {
		t.Fatal("watchlist leaked across accounts")
	}
	request("POST", "/auth/history/1", `{"position":50}`, passwordChanged, 403)
	if _, err = db.Exec(ctx, `INSERT INTO subscriptions(user_id,starts_at,expires_at) SELECT id,NOW(),NOW()+interval '1 day' FROM users WHERE username='tester'`); err != nil {
		t.Fatal(err)
	}
	request("POST", "/auth/history/1", `{"position":-1}`, passwordChanged, 400)
	request("POST", "/auth/history/1", `{"position":50}`, passwordChanged, 200)
	w = request("GET", "/auth/library", "", passwordChanged, 200)
	if !strings.Contains(w.Body.String(), `"position":50`) {
		t.Fatal("history not persisted")
	}
	w = request("GET", "/auth/library", "", another, 200)
	if !strings.Contains(w.Body.String(), `"history":[]`) {
		t.Fatal("history leaked across accounts")
	}
	request("POST", "/auth/history/1", `{"position":999}`, passwordChanged, 200)
	w = request("GET", "/auth/library", "", passwordChanged, 200)
	if !strings.Contains(w.Body.String(), `"position":120`) {
		t.Fatal("progress not clamped to video duration")
	}
	request("POST", "/auth/watchlist/1", `{"saved":false}`, passwordChanged, 204)
	request("POST", "/auth/watchlist/999", `{"saved":true}`, passwordChanged, 404)
	if _, err = db.Exec(ctx, `UPDATE videos SET publication_status='hidden' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	w = request("GET", "/auth/library", "", passwordChanged, 200)
	if !strings.Contains(w.Body.String(), `"history":[]`) {
		t.Fatal("hidden video visible in library")
	}
	request("POST", "/auth/history/1", `{"position":5}`, passwordChanged, 404)
	request("POST", "/auth/watchlist/1", `{"saved":true}`, passwordChanged, 404)
	// Old account requests must never mutate the current cookie's account.
	request("POST", "/auth/watchlist/1", `{"saved":false,"expectedUserId":99999}`, passwordChanged, 409)
	request("POST", "/auth/profile", `{"name":"Wrong owner","email":"new@example.com","expectedUserId":99999}`, passwordChanged, 409)
	request("POST", "/auth/password", `{"currentPassword":"new-correct-test-password","newPassword":"replacement-password","expectedUserId":99999}`, passwordChanged, 409)
	request("POST", "/auth/logout", `{"expectedUserId":99999}`, passwordChanged, 409)
	request("GET", "/auth/me", "", passwordChanged, 200)
	if _, err = db.Exec(ctx, `UPDATE videos SET publication_status='published' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err = db.QueryRow(ctx, `SELECT revision FROM watch_history WHERE user_id=1 AND video_id=1`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	request("POST", "/auth/history/1", fmt.Sprintf(`{"position":100,"expectedRevision":%d}`, revision), passwordChanged, 200)
	request("POST", "/auth/history/1", fmt.Sprintf(`{"position":40,"expectedRevision":%d}`, revision), passwordChanged, 409)
	request("POST", "/auth/history/1", fmt.Sprintf(`{"position":40,"expectedRevision":%d}`, revision+1), passwordChanged, 200)
	var position int
	if err = db.QueryRow(ctx, `SELECT position_seconds FROM watch_history WHERE user_id=1 AND video_id=1`).Scan(&position); err != nil || position != 40 {
		t.Fatal("backward seek with current revision must work", err, position)
	}
	request("GET", "/auth/library?savedCursor=invalid", "", passwordChanged, 400)
	if _, err = db.Exec(ctx, `INSERT INTO videos(title,status,publication_status,duration_seconds) SELECT 'Page '||n,'ready','published',120 FROM generate_series(1,600)n;
 INSERT INTO watchlist(user_id,video_id,created_at) SELECT 1,id,'2026-01-01' FROM videos;
 INSERT INTO watch_history(user_id,video_id,position_seconds,watched_at) SELECT 1,id,20,'2026-01-01' FROM videos ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	savedSeen := map[int64]bool{}
	historySeen := map[int64]bool{}
	savedCursor, historyCursor := "", ""
	for page := 0; page < 20; page++ {
		w = request("GET", "/auth/library?savedCursor="+savedCursor+"&historyCursor="+historyCursor, "", passwordChanged, 200)
		var data struct {
			Saved       []int64        `json:"saved"`
			History     []historyEntry `json:"history"`
			SavedNext   string         `json:"savedNext"`
			HistoryNext string         `json:"historyNext"`
		}
		if json.Unmarshal(w.Body.Bytes(), &data) != nil {
			t.Fatal("bad library JSON")
		}
		for _, id := range data.Saved {
			if savedSeen[id] {
				t.Fatal("duplicate saved cursor entry")
			}
			savedSeen[id] = true
		}
		for _, e := range data.History {
			if historySeen[e.VideoID] {
				t.Fatal("duplicate history cursor entry")
			}
			historySeen[e.VideoID] = true
		}
		savedCursor, historyCursor = data.SavedNext, data.HistoryNext
		if savedCursor == "" && historyCursor == "" {
			break
		}
	}
	if len(savedSeen) != 601 || len(historySeen) != 601 {
		t.Fatal("pagination silently omitted records", len(savedSeen), len(historySeen))
	}
	// Reproduce the lock/revoke ordering without touching live data.
	token, hash, err := newSessionToken()
	_ = token
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,1,NOW()+interval '1 day')`, hash); err != nil {
		t.Fatal(err)
	}
	blocker, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(blocker)
	if _, err = blocker.Exec(ctx, `UPDATE users SET name=name WHERE id=1;`); err != nil {
		t.Fatal(err)
	}
	if _, err = blocker.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, hash); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		tx, _, err := lockedAccount(ctx, db, hash, 1)
		if tx != nil {
			rollback(tx)
		}
		result <- err
	}()
	waiting := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE '%FOR UPDATE OF u%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("concurrency test did not acquire the waiting lock")
	}
	if err = blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("revoked session survived lock wait", err)
	}
	testMediaReservations(t, ctx, db)
	testMembershipManagement(t, ctx, db, a)

}
