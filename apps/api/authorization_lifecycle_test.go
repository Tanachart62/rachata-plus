package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The first read occurs after entry authorization, but before the final write.
// Channels make the revocation ordering deterministic without timing sleeps.
type authorizationBodyGate struct {
	io.Reader
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *authorizationBodyGate) Read(p []byte) (int, error) {
	g.once.Do(func() {
		close(g.entered)
		<-g.release
	})
	return g.Reader.Read(p)
}

// Uses only the opt-in auth test's disposable schema and temporary storage.
// No worker, codec process, real media or external HTTP server is started.
func testAuthorizationLifecycle(t *testing.T, ctx context.Context, db *pgxpool.Pool, a *authConfig) {
	t.Helper()
	password, err := hashPassword("lifecycle-test-password")
	if err != nil {
		t.Fatal(err)
	}
	createUser := func(t *testing.T, name, role string) int64 {
		t.Helper()
		var id int64
		if err := db.QueryRow(ctx, `INSERT INTO users(username,name,email,password_hash,role) VALUES($1,$1,$2,$3,$4) RETURNING id`, name, name+"@example.com", password, role).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	session := func(t *testing.T, id int64) (*http.Cookie, int64) {
		t.Helper()
		token, hash, err := newSessionToken()
		if err != nil {
			t.Fatal(err)
		}
		var sid int64
		if err := db.QueryRow(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,NOW()+interval '1 day') RETURNING session_id`, hash, id).Scan(&sid); err != nil {
			t.Fatal(err)
		}
		return &http.Cookie{Name: sessionCookie, Value: token}, sid
	}
	adminID := createUser(t, "lifecycle_admin", "admin")
	adminCookie, _ := session(t, adminID)
	var sequence atomic.Int64
	newRequest := func(method, path, contentType string, body io.Reader, cookie *http.Cookie) *http.Request {
		req := httptest.NewRequest(method, path, body).WithContext(ctx)
		req.RemoteAddr = fmt.Sprintf("198.51.100.%d:1234", sequence.Add(1))
		req.Header.Set("Origin", a.origin)
		req.Header.Set("Content-Type", contentType)
		req.AddCookie(cookie)
		return req
	}
	perform := func(router *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	check := func(t *testing.T, w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("got %d want %d: %s", w.Code, want, w.Body.String())
		}
	}
	type operation struct {
		name, method string
		admin        bool
		want         int
	}
	operations := []operation{
		{"save", "POST", false, 204},
		{"unsave", "POST", false, 204},
		{"history", "POST", false, 200},
		{"upload", "POST", true, 202},
		{"publish", "PATCH", true, 200},
	}
	for oi, op := range operations {
		transitions := []string{"none", "logout", "own_session", "admin_session", "logout_all", "suspend", "password", "switch_account"}
		if op.name == "history" {
			transitions = append(transitions, "membership")
		}
		for ti, transition := range transitions {
			t.Run(op.name+"/"+transition, func(t *testing.T) {
				role := "user"
				if op.admin {
					role = "admin"
				}
				id := createUser(t, fmt.Sprintf("lifecycle_%d_%d", oi, ti), role)
				cookie, sid := session(t, id)
				other, _ := session(t, id)
				if _, err := db.Exec(ctx, `INSERT INTO subscriptions(user_id,starts_at,expires_at) VALUES($1,NOW(),NOW()+interval '1 day')`, id); err != nil {
					t.Fatal(err)
				}
				var videoID int64
				if err := db.QueryRow(ctx, `INSERT INTO videos(title,status,publication_status,duration_seconds) VALUES('Lifecycle fixture','ready','published',60) RETURNING id`).Scan(&videoID); err != nil {
					t.Fatal(err)
				}
				if op.name == "unsave" {
					if _, err := db.Exec(ctx, `INSERT INTO watchlist(user_id,video_id) VALUES($1,$2)`, id, videoID); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := db.Exec(ctx, `INSERT INTO watch_history(user_id,video_id,position_seconds,revision) VALUES($1,$2,2,1)`, id, videoID); err != nil {
					t.Fatal(err)
				}
				service, err := newVideoService(db, a, t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				// Reservations are accounting only; uploads contain a few dummy bytes.
				service.budget = mediaBudget{Total: 10 << 30, Job: maxVideoBytes + 1, MinFree: 1}
				router := gin.New()
				group := router.Group("/auth")
				group.Use(a.protectWrites())
				group.POST("/logout", a.logoutHandler(db))
				group.POST("/login", a.loginHandler(db))
				group.GET("/me", a.meHandler(db))
				a.accountRoutes(group, db)
				service.routes(router)
				path, contentType := fmt.Sprintf("/auth/watchlist/%d", videoID), "application/json"
				body := []byte(fmt.Sprintf(`{"expectedUserId":%d,"saved":%t}`, id, op.name != "unsave"))
				switch op.name {
				case "history":
					path = fmt.Sprintf("/auth/history/%d", videoID)
					body = []byte(fmt.Sprintf(`{"expectedUserId":%d,"position":12,"expectedRevision":1}`, id))
				case "publish":
					path = fmt.Sprintf("/api/admin/videos/%d", videoID)
					body = []byte(`{"publicationStatus":"hidden"}`)
				case "upload":
					path = "/api/admin/videos"
					var buf bytes.Buffer
					writer := multipart.NewWriter(&buf)
					for name, value := range map[string]string{"title": "Lifecycle upload", "category": "ความรู้"} {
						if err := writer.WriteField(name, value); err != nil {
							t.Fatal(err)
						}
					}
					part, err := writer.CreateFormFile("file", "dummy.mp4")
					if err != nil {
						t.Fatal(err)
					}
					if _, err = part.Write([]byte("dummy fixture; never decoded")); err != nil {
						t.Fatal(err)
					}
					if err = writer.Close(); err != nil {
						t.Fatal(err)
					}
					body, contentType = buf.Bytes(), writer.FormDataContentType()
				}
				var videosBefore, reservationsBefore int
				if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM videos),(SELECT count(*) FROM media_reservations)`).Scan(&videosBefore, &reservationsBefore); err != nil {
					t.Fatal(err)
				}
				gate := &authorizationBodyGate{Reader: bytes.NewReader(body), entered: make(chan struct{}), release: make(chan struct{})}
				var release sync.Once
				resume := func() { release.Do(func() { close(gate.release) }) }
				done := make(chan struct{})
				w := httptest.NewRecorder()
				req := newRequest(op.method, path, contentType, gate, cookie)
				req.ContentLength = int64(len(body))
				requestCtx, cancel := context.WithCancel(req.Context())
				t.Cleanup(func() {
					resume()
					cancel()
					select {
					case <-done:
					case <-time.After(6 * time.Second):
						t.Error("request did not terminate during cleanup")
					}
				})
				go func() { router.ServeHTTP(w, req.WithContext(requestCtx)); close(done) }()
				select {
				case <-gate.entered:
				case <-done:
					t.Fatalf("request failed before body gate: %d %s", w.Code, w.Body.String())
				case <-time.After(3 * time.Second):
					t.Fatal("request did not reach body gate")
				}
				owner := fmt.Sprintf(`{"expectedUserId":%d}`, id)
				switch transition {
				case "logout":
					check(t, perform(router, newRequest("POST", "/auth/logout", "application/json", strings.NewReader(owner), cookie)), 204)
				case "own_session":
					check(t, perform(router, newRequest("POST", fmt.Sprintf("/auth/sessions/%d/revoke", sid), "application/json", strings.NewReader(owner), other)), 200)
				case "admin_session":
					payload := fmt.Sprintf(`{"expectedUserId":%d,"expectedRevision":0,"action":"logout_session","sessionId":"%d"}`, adminID, sid)
					check(t, perform(router, newRequest("POST", fmt.Sprintf("/auth/admin/memberships/%d", id), "application/json", strings.NewReader(payload), adminCookie)), 200)
				case "password":
					payload := fmt.Sprintf(`{"expectedUserId":%d,"currentPassword":"lifecycle-test-password","newPassword":"updated-lifecycle-password"}`, id)
					check(t, perform(router, newRequest("POST", "/auth/password", "application/json", strings.NewReader(payload), other)), 204)
				case "switch_account":
					payload := `{"email":"lifecycle_admin@example.com","password":"lifecycle-test-password"}`
					check(t, perform(router, newRequest("POST", "/auth/login", "application/json", strings.NewReader(payload), cookie)), 200)
				case "logout_all", "suspend", "membership":
					action := transition
					if action == "membership" {
						action = "revoke"
					}
					payload := fmt.Sprintf(`{"expectedUserId":%d,"expectedRevision":0,"action":%q}`, adminID, action)
					check(t, perform(router, newRequest("POST", fmt.Sprintf("/auth/admin/memberships/%d", id), "application/json", strings.NewReader(payload), adminCookie)), 200)
				}
				resume()
				select {
				case <-done:
				case <-time.After(6 * time.Second):
					t.Fatal("delayed request did not finish")
				}
				want := op.want
				if transition != "none" {
					want = 401
					if transition == "membership" {
						want = 403
					}
				}
				check(t, w, want)
				if transition != "none" {
					check(t, perform(router, newRequest(op.method, path, contentType, bytes.NewReader(body), cookie)), want)
					if transition != "membership" {
						check(t, perform(router, newRequest("POST", "/auth/logout", "application/json", strings.NewReader(owner), cookie)), 204)
					}
				}
				var saved bool
				var position int
				var revision int64
				var status string
				if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM watchlist WHERE user_id=$1 AND video_id=$2),h.position_seconds,h.revision,v.publication_status FROM watch_history h JOIN videos v ON v.id=h.video_id WHERE h.user_id=$1 AND h.video_id=$2`, id, videoID).Scan(&saved, &position, &revision, &status); err != nil {
					t.Fatal(err)
				}
				wantSaved, wantPosition, wantRevision, wantStatus := op.name == "unsave", 2, int64(1), "published"
				if transition == "none" {
					switch op.name {
					case "save":
						wantSaved = true
					case "unsave":
						wantSaved = false
					case "history":
						wantPosition, wantRevision = 12, 2
					case "publish":
						wantStatus = "hidden"
					}
				}
				if saved != wantSaved || position != wantPosition || revision != wantRevision || status != wantStatus {
					t.Fatalf("unexpected write: saved=%t position=%d revision=%d status=%s", saved, position, revision, status)
				}
				var videoCount, reservationCount int
				if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM videos),(SELECT count(*) FROM media_reservations)`).Scan(&videoCount, &reservationCount); err != nil {
					t.Fatal(err)
				}
				entries, err := os.ReadDir(service.root)
				if err != nil {
					t.Fatal(err)
				}
				delta := 0
				if op.name == "upload" && transition == "none" {
					delta = 1
				}
				if videoCount != videosBefore+delta || reservationCount != reservationsBefore+delta || len(entries) != delta {
					t.Fatalf("upload cleanup/commit mismatch: videos=%d reservations=%d directories=%d", videoCount-videosBefore, reservationCount-reservationsBefore, len(entries))
				}
				if delta == 1 {
					var leased bool
					if err := db.QueryRow(ctx, `SELECT lease_until IS NOT NULL FROM media_reservations WHERE storage_key=$1`, entries[0].Name()).Scan(&leased); err != nil || leased {
						t.Fatal("accepted upload retained lease", leased, err)
					}
				}
				if transition == "none" && op.name == "history" {
					// CAS remains enforced after introducing the authority transaction.
					check(t, perform(router, newRequest(op.method, path, contentType, bytes.NewReader(body), cookie)), 409)
				}
				if transition == "none" && !op.admin {
					wrong := fmt.Sprintf(`{"expectedUserId":%d,"saved":true,"position":20,"expectedRevision":2}`, adminID)
					check(t, perform(router, newRequest(op.method, path, contentType, strings.NewReader(wrong), cookie)), 409)
				}
				// The other session survives single-session revocation.
				if transition == "logout" || transition == "own_session" || transition == "admin_session" || transition == "switch_account" {
					check(t, perform(router, newRequest("GET", "/auth/me", "application/json", nil, other)), 200)
				}
			})
		}
	}
	t.Run("account-switch-serialization", func(t *testing.T) {
		id := createUser(t, "lifecycle_switch_lock", "user")
		cookie, _ := session(t, id)
		hash := sha256.Sum256([]byte(cookie.Value))
		tx, _, err := lockedAccount(ctx, db, hash[:], id)
		if err != nil {
			t.Fatal(err)
		}
		defer rollback(tx)
		var guardPID int
		if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&guardPID); err != nil {
			t.Fatal(err)
		}
		router := gin.New()
		group := router.Group("/auth")
		group.Use(a.protectWrites())
		group.POST("/login", a.loginHandler(db))
		payload := `{"email":"lifecycle_admin@example.com","password":"lifecycle-test-password"}`
		req := newRequest("POST", "/auth/login", "application/json", strings.NewReader(payload), cookie)
		requestCtx, cancel := context.WithCancel(req.Context())
		done := make(chan struct{})
		w := httptest.NewRecorder()
		t.Cleanup(func() {
			rollback(tx)
			cancel()
			select {
			case <-done:
			case <-time.After(6 * time.Second):
				t.Error("login did not terminate during cleanup")
			}
		})
		go func() { router.ServeHTTP(w, req.WithContext(requestCtx)); close(done) }()
		deadline := time.Now().Add(2 * time.Second)
		waiting := false
		for time.Now().Before(deadline) {
			if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)) AND query LIKE 'DELETE FROM sessions WHERE token_hash=$1%')`, guardPID).Scan(&waiting); err != nil {
				t.Fatal(err)
			}
			if waiting {
				break
			}
			select {
			case <-done:
				t.Fatal("account switch revoked token before the guarded write committed")
			case <-time.After(10 * time.Millisecond):
			}
		}
		if !waiting {
			t.Fatal("login did not wait for the guarded session row")
		}
		var videoID int64
		if err := tx.QueryRow(ctx, `INSERT INTO videos(title,status,publication_status) VALUES('Serialized switch fixture','ready','published') RETURNING id`).Scan(&videoID); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO watchlist(user_id,video_id) VALUES($1,$2)`, id, videoID); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			t.Fatal("account switch did not finish after commit")
		}
		check(t, w, 200)
		var revoked bool
		if err := db.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM sessions WHERE token_hash=$1)`, hash[:]).Scan(&revoked); err != nil || !revoked {
			t.Fatal("account switch failed to revoke old token", revoked, err)
		}
	})
}
