package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPackagePlaylists(t *testing.T) {
	levels := []rendition{{640, 360, 800000}, {1920, 1080, 5000000}, {2560, 1440, 9000000}, {3840, 2160, 16000000}}
	for _, plan := range []struct {
		name                      string
		height, screens, variants int
	}{{"basic", 1080, 2, 2}, {"standard", 1440, 2, 3}, {"premium", 2160, 4, 4}} {
		t.Run(plan.name, func(t *testing.T) {
			limits := limitsFor(accountResponse{Member: true, Plan: plan.name})
			if limits.Height != plan.height || limits.Screens != plan.screens {
				t.Fatal(limits)
			}
			data, err := filteredPlaylist([]byte(masterPlaylist(levels)), limits.Height, "test-token")
			if err != nil || strings.Count(string(data), "#EXT-X-STREAM-INF:") != plan.variants {
				t.Fatal(string(data), err)
			}
			for _, line := range strings.Split(string(data), "\n") {
				if line != "" && !strings.HasPrefix(line, "#") && !strings.HasSuffix(line, "?playback=test-token") {
					t.Fatal("lease lost in HLS URI", line)
				}
			}
		})
	}
	if limitsFor(accountResponse{Member: true, Plan: "unknown"}).Height != 0 || limitsFor(accountResponse{Plan: "premium"}).Height != 0 {
		t.Fatal("unknown/nonmember privileges")
	}
	if limitsFor(accountResponse{Role: "admin"}).Height != 2160 {
		t.Fatal("admin access")
	}
	wide := []byte(masterPlaylist([]rendition{{2560, 960, 1}, {1920, 720, 1}}))
	for _, name := range []string{"960p.m3u8", "segment_960p_000000.ts"} {
		if renditionAllowed(wide, name, 1080) || !renditionAllowed(wide, name, 1440) {
			t.Fatal("ultrawide dimension bypass", name)
		}
	}
	if !renditionAllowed(wide, "720p.m3u8", 1080) {
		t.Fatal("valid ultrawide rendition denied")
	}
	for _, uri := range []string{"https://example.com/file.ts", "../source", "segment_1080p_000000.ts?forged=1", "#EXT-X-KEY:URI=\"source\""} {
		if _, err := filteredPlaylist([]byte("#EXTM3U\n"+uri+"\n"), 2160, ""); err == nil {
			t.Fatal("untrusted HLS URI accepted", uri)
		}
	}
}

func testPlaybackEntitlements(t *testing.T, ctx context.Context, db *pgxpool.Pool, a *authConfig) {
	s, err := newVideoService(db, a, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := "44444444444444444444444444444444"
	dir := filepath.Join(s.root, key, "full")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	levels := []rendition{{640, 360, 1}, {1920, 1080, 1}, {2560, 1440, 1}, {3840, 2160, 1}}
	if err = os.WriteFile(filepath.Join(dir, "index.m3u8"), []byte(masterPlaylist(levels)), 0600); err != nil {
		t.Fatal(err)
	}
	for _, level := range levels {
		name := fmt.Sprintf("segment_%dp_000000.ts", level.Height)
		if err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%dp.m3u8", level.Height)), []byte("#EXTM3U\n#EXTINF:4,\n"+name+"\n#EXT-X-ENDLIST\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, name), []byte("dummy segment"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var video int64
	if err = db.QueryRow(ctx, `INSERT INTO videos(title,status,publication_status,storage_key) VALUES('playback fixture','ready','published',$1) RETURNING id`, key).Scan(&video); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), `DELETE FROM videos WHERE id=$1`, video)
	router := gin.New()
	s.routes(router)
	perform := func(method, path string, payload any, cookie *http.Cookie) *httptest.ResponseRecorder {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(method, path, strings.NewReader(string(body))).WithContext(ctx)
		req.Header.Set("Origin", a.origin)
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	endpoint := fmt.Sprintf("/api/playback/%d", video)
	for _, plan := range []struct {
		name                      string
		height, screens, variants int
	}{{"basic", 1080, 2, 2}, {"standard", 1440, 2, 3}, {"premium", 2160, 4, 4}} {
		t.Run(plan.name, func(t *testing.T) {
			var user int64
			if err := db.QueryRow(ctx, `INSERT INTO users(username,name,email,password_hash) VALUES($1,$1,$2,'unused') RETURNING id`, "playback_"+plan.name, "playback_"+plan.name+"@example.com").Scan(&user); err != nil {
				t.Fatal(err)
			}
			defer db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, user)
			if _, err := db.Exec(ctx, `INSERT INTO subscriptions(user_id,plan,starts_at,expires_at) VALUES($1,$2,NOW(),NOW()+interval '1 day')`, user, plan.name); err != nil {
				t.Fatal(err)
			}
			login, session, err := newSessionToken()
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,NOW()+interval '1 day')`, session, user); err != nil {
				t.Fatal(err)
			}
			cookie := &http.Cookie{Name: sessionCookie, Value: login}
			payload := func(token string) any { return map[string]any{"expectedUserId": user, "token": token} }
			// More callers than available slots begin together; locking must cap them.
			count := plan.screens + 3
			responses := make([]*httptest.ResponseRecorder, count)
			gate := make(chan struct{})
			var wg sync.WaitGroup
			for i := range count {
				wg.Add(1)
				go func(i int) { defer wg.Done(); <-gate; responses[i] = perform("POST", endpoint, payload(""), cookie) }(i)
			}
			close(gate)
			wg.Wait()
			tokens := []string{}
			urls := []string{}
			for _, w := range responses {
				if w.Code == 200 {
					var data struct{ Token, URL string }
					if err = json.Unmarshal(w.Body.Bytes(), &data); err != nil {
						t.Fatal(err)
					}
					tokens = append(tokens, data.Token)
					urls = append(urls, data.URL)
				} else if w.Code != 409 {
					t.Fatal(w.Code, w.Body.String())
				}
			}
			if len(tokens) != plan.screens {
				t.Fatal("concurrent limit", len(tokens), plan.screens)
			}
			manifest := perform("GET", urls[0], nil, cookie)
			if manifest.Code != 200 || strings.Count(manifest.Body.String(), "#EXT-X-STREAM-INF:") != plan.variants {
				t.Fatal(manifest.Code, manifest.Body.String())
			}
			if w := perform("GET", fmt.Sprintf("/media/%d/full/index.m3u8", video), nil, cookie); w.Code != 403 {
				t.Fatal("lease bypass", w.Code)
			}
			query := "?playback=" + tokens[0]
			for _, height := range []int{360, 1080, 1440, 2160} {
				for _, name := range []string{fmt.Sprintf("%dp.m3u8", height), fmt.Sprintf("segment_%dp_000000.ts", height)} {
					w := perform("GET", fmt.Sprintf("/media/%d/full/%s%s", video, name, query), nil, cookie)
					want := 200
					if height > plan.height {
						want = 403
					}
					if w.Code != want {
						t.Fatal("direct quality bypass", name, w.Code, w.Body.String())
					}
					if w.Code == 200 && strings.HasSuffix(name, ".m3u8") && !strings.Contains(w.Body.String(), query) {
						t.Fatal("segment lease missing")
					}
				}
			}
			otherLogin, otherHash, _ := newSessionToken()
			if plan.name == "premium" {
				if _, err = db.Exec(ctx, `UPDATE subscriptions SET plan='basic' WHERE user_id=$1`, user); err != nil {
					t.Fatal(err)
				}
				allowed := 0
				for _, token := range tokens {
					w := perform("POST", endpoint, payload(token), cookie)
					if w.Code == 200 {
						allowed++
						if w := perform("GET", fmt.Sprintf("/media/%d/full/2160p.m3u8?playback=%s", video, token), nil, cookie); w.Code != 403 {
							t.Fatal("downgrade retained 4K", w.Code)
						}
					} else if w.Code != 409 {
						t.Fatal("unexpected downgrade result", w.Code)
					}
				}
				if allowed != 2 {
					t.Fatal("downgrade retained too many screens", allowed)
				}
				if _, err = db.Exec(ctx, `UPDATE subscriptions SET plan='premium' WHERE user_id=$1`, user); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = db.Exec(ctx, `UPDATE subscriptions SET status='revoked' WHERE user_id=$1`, user); err != nil {
				t.Fatal(err)
			}
			if w := perform("GET", urls[0], nil, cookie); w.Code != 403 {
				t.Fatal("revoked membership still receives media", w.Code)
			}
			if w := perform("POST", endpoint, payload(tokens[0]), cookie); w.Code != 403 {
				t.Fatal("revoked membership renews permit", w.Code)
			}
			if _, err = db.Exec(ctx, `UPDATE subscriptions SET status='active' WHERE user_id=$1`, user); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,NOW()+interval '1 day')`, otherHash, user); err != nil {
				t.Fatal(err)
			}
			if w := perform("GET", urls[0], nil, &http.Cookie{Name: sessionCookie, Value: otherLogin}); w.Code != 403 {
				t.Fatal("permit usable on another session", w.Code)
			}
			if w := perform("DELETE", endpoint, payload(tokens[0]), cookie); w.Code != 204 {
				t.Fatal(w.Code, w.Body.String())
			}
			if w := perform("GET", urls[0], nil, cookie); w.Code != 403 {
				t.Fatal("released lease still usable", w.Code)
			}
			if w := perform("POST", endpoint, payload(""), cookie); w.Code != 200 {
				t.Fatal("released slot not reusable", w.Code, w.Body.String())
			}
			// Offline leases expire and cannot be revived by a late heartbeat.
			h := sha256.Sum256([]byte(tokens[1]))
			if _, err = db.Exec(ctx, `UPDATE playback_leases SET created_at=NOW()-interval '2 minutes',expires_at=NOW()-interval '1 second' WHERE token_hash=$1`, h[:]); err != nil {
				t.Fatal(err)
			}
			if w := perform("POST", endpoint, payload(tokens[1]), cookie); w.Code != 409 {
				t.Fatal("expired heartbeat", w.Code)
			}
			if w := perform("POST", endpoint, payload(""), cookie); w.Code != 200 {
				t.Fatal("expired slot not reusable", w.Code)
			}
			if _, err = db.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, session); err != nil {
				t.Fatal(err)
			}
			var remaining int
			if err = db.QueryRow(ctx, `SELECT COUNT(*) FROM playback_leases WHERE session_hash=$1`, session).Scan(&remaining); err != nil || remaining != 0 {
				t.Fatal("revocation did not cascade", remaining, err)
			}
			if w := perform("GET", urls[1], nil, cookie); w.Code != 401 {
				t.Fatal("revoked session receives media", w.Code)
			}
		})
	}
}
