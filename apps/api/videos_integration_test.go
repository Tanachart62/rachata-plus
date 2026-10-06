package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func TestLocalVideoPipeline(t *testing.T) {
	if os.Getenv("LOCAL_VIDEO_INTEGRATION") != "1" {
		t.Skip("set LOCAL_VIDEO_INTEGRATION=1 with local PostgreSQL and FFmpeg")
	}
	env, err := godotenv.Read("../../.env.local")
	if err != nil || env["DB_HOST"] != "127.0.0.1" {
		t.Fatal("requires loopback .env.local")
	}
	u := url.URL{Scheme: "postgres", User: url.UserPassword(env["POSTGRES_USER"], env["POSTGRES_PASSWORD"]), Host: net.JoinHostPort(env["DB_HOST"], env["POSTGRES_PORT"]), Path: "/" + env["POSTGRES_DB"], RawQuery: "sslmode=disable"}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	adminDB, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal("cannot configure local DB")
	}
	defer adminDB.Close()
	schema := fmt.Sprintf("video_test_%d", time.Now().UnixNano())
	if _, err = adminDB.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := adminDB.Exec(cleanup, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	}()
	config, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, name := range []string{"create_videos.sql", "create_users_and_subscriptions.sql", "add_usernames_and_sessions.sql", "add_local_video_pipeline.sql", "add_account_library.sql", "add_account_consistency.sql", "add_membership_management.sql", "add_account_controls.sql", "add_session_devices.sql"} {
		sql, err := os.ReadFile("../../db/migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	a, err := newAuthConfig("http://127.0.0.1:5174", "false")
	if err != nil {
		t.Fatal(err)
	}
	s, err := newVideoService(db, a, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// The API has no FFmpeg dependency; a distinct worker shares only DB/storage.
	worker, err := newVideoService(db, nil, s.root)
	if err != nil {
		t.Fatal(err)
	}
	if err = worker.initTranscoder(2); err != nil {
		t.Fatal(err)
	}
	cookies := map[string]*http.Cookie{}
	for _, role := range []string{"free", "member", "admin"} {
		actualRole := "user"
		if role == "admin" {
			actualRole = "admin"
		}
		var id int64
		err = db.QueryRow(ctx, `INSERT INTO users(username,name,email,password_hash,role) VALUES($1,$1,$2,'not-used-by-session-tests',$3) RETURNING id`, role, role+"@test.example", actualRole).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		if role == "member" {
			if _, err = db.Exec(ctx, `INSERT INTO subscriptions(user_id,starts_at,expires_at) VALUES($1,NOW(),NOW()+interval '1 day')`, id); err != nil {
				t.Fatal(err)
			}
		}
		token, hash, err := newSessionToken()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,NOW()+interval '1 day')`, hash, id); err != nil {
			t.Fatal(err)
		}
		cookies[role] = &http.Cookie{Name: sessionCookie, Value: token, Path: "/"}
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	s.routes(router)
	request := func(method, path, body, role string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Origin", a.origin)
		req.Header.Set("Content-Type", "application/json")
		if cookie := cookies[role]; cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s (%s): got %d want %d: %s", method, path, role, w.Code, want, w.Body.String())
		}
		return w
	}
	request("GET", "/api/admin/videos", "", "", 401)
	request("GET", "/api/admin/videos", "", "free", 403)
	file := filepath.Join(t.TempDir(), "fixture.mp4")
	fixtureCtx, stop := context.WithTimeout(ctx, 20*time.Second)
	output, err := exec.CommandContext(fixtureCtx, worker.ffmpeg, "-nostdin", "-v", "error", "-f", "lavfi", "-i", "color=c=green:s=320x180:r=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "24", "-c:v", "libx264", "-threads", "2", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac", file).CombinedOutput()
	stop()
	if err != nil {
		t.Fatalf("generate fixture: %v %s", err, output)
	}
	fixture, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	// The guarded worker must reject and remove output that exceeds a job's
	// reservation, while preserving the source for a possible explicit retry.
	limited, err := newVideoService(db, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = limited.initTranscoder(2); err != nil {
		t.Fatal(err)
	}
	budgetKey := "33333333333333333333333333333333"
	dir := filepath.Join(limited.root, budgetKey)
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "source"), fixture, 0600); err != nil {
		t.Fatal(err)
	}
	limited.budget.Job = int64(len(fixture)) + 100
	if _, err = limited.transcode(ctx, budgetKey); !errors.Is(err, errMediaBudget) {
		t.Fatal("output quota was not enforced", err)
	}
	for _, kind := range []string{"full", "preview"} {
		if _, err = os.Stat(filepath.Join(dir, kind)); !os.IsNotExist(err) {
			t.Fatal("over-budget output was retained", err)
		}
	}
	if _, err = os.Stat(filepath.Join(dir, "source")); err != nil {
		t.Fatal("quota failure must preserve source", err)
	}
	limited.releaseMedia(budgetKey)
	upload := func(data []byte, filename, role string, want int) int64 {
		t.Helper()
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		writer.WriteField("title", "HLS test")
		writer.WriteField("category", "ความรู้")
		writer.WriteField("description", "Generated test video")
		part, _ := writer.CreateFormFile("file", filename)
		part.Write(data)
		writer.Close()
		req := httptest.NewRequest("POST", "/api/admin/videos", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Origin", a.origin)
		if cookie := cookies[role]; cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("upload got %d want %d: %s", w.Code, want, w.Body.String())
		}
		if want != 202 {
			return 0
		}
		var response struct {
			Video videoRecord `json:"video"`
		}
		if json.Unmarshal(w.Body.Bytes(), &response) != nil {
			t.Fatal("bad upload response")
		}
		return response.Video.ID
	}
	upload(fixture, "test.mp4", "free", 403)
	upload(fixture, "test.exe", "admin", 400)
	id := upload(fixture, "../../fixture.mp4", "admin", 202)
	base := fmt.Sprintf("/api/admin/videos/%d", id)
	request("PATCH", base, `{"publicationStatus":"published"}`, "admin", 409)
	// Recover a job left in processing by a previous interrupted worker.
	db.Exec(ctx, `UPDATE videos SET status='processing' WHERE id=$1`, id)
	workerCtx, workerStop := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); worker.workQueue(workerCtx) }()
	defer func() { workerStop(); wg.Wait() }()
	await := func(id int64, status string) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			var state string
			if err = db.QueryRow(ctx, `SELECT status FROM videos WHERE id=$1`, id).Scan(&state); err != nil {
				t.Fatal(err)
			}
			if state == status {
				return
			}
			if state == "failed" && status != "failed" {
				t.Fatal("unexpected transcoding failure")
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("timed out waiting for " + status)
	}
	await(id, "ready")
	full := fmt.Sprintf("/media/%d/full/index.m3u8", id)
	preview := fmt.Sprintf("/media/%d/preview/index.m3u8", id)
	request("GET", preview, "", "", 404)
	request("GET", full, "", "", 401)
	request("GET", full, "", "free", 403)
	request("GET", full, "", "member", 404)
	request("GET", full, "", "admin", 200)
	request("PATCH", base, `{"publicationStatus":"published"}`, "admin", 200)
	w := request("GET", full, "", "member", 200)
	if !strings.Contains(w.Body.String(), "#EXT-X-ENDLIST") {
		t.Fatal("invalid HLS output")
	}
	s.nginxDelivery = true
	redirect := request("GET", full, "", "member", 200)
	if !strings.HasPrefix(redirect.Header().Get("X-Accel-Redirect"), "/_hls/") || redirect.Body.Len() != 0 {
		t.Fatal("Nginx delivery must authorize then redirect without sending media bytes")
	}
	for _, role := range []string{"", "free"} {
		status := 401
		if role == "free" {
			status = 403
		}
		if request("GET", full, "", role, status).Header().Get("X-Accel-Redirect") != "" {
			t.Fatal("unauthorized internal redirect")
		}
	}
	s.nginxDelivery = false
	request("HEAD", full, "", "member", 200)
	p := request("GET", preview, "", "", 200)
	if strings.Contains(p.Body.String(), "full/") {
		t.Fatal("preview exposes full asset")
	}
	if !strings.Contains(p.Body.String(), "#EXT-X-ENDLIST") {
		t.Fatal("invalid preview")
	}
	segment := fmt.Sprintf("/media/%d/full/segment_000000.ts", id)
	request("GET", segment, "", "", 401)
	request("GET", segment, "", "free", 403)
	request("GET", segment, "", "member", 200)
	// Even a legacy token left in the DB cannot bypass account suspension.
	if _, err = db.Exec(ctx, `UPDATE users SET account_status='suspended' WHERE username='member'`); err != nil {
		t.Fatal(err)
	}
	request("GET", full, "", "member", 401)
	request("GET", segment, "", "member", 401)
	s.nginxDelivery = true
	request("GET", full, "", "member", 401)
	s.nginxDelivery = false
	if _, err = db.Exec(ctx, `UPDATE users SET account_status='active' WHERE username='member'`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE users SET account_status='suspended' WHERE username='admin'`); err != nil {
		t.Fatal(err)
	}
	request("GET", "/api/admin/videos", "", "admin", 401)
	request("GET", full, "", "admin", 401)
	if _, err = db.Exec(ctx, `UPDATE users SET account_status='active' WHERE username='admin'`); err != nil {
		t.Fatal(err)
	}

	request("GET", fmt.Sprintf("/media/%d/full/source", id), "", "admin", 404)
	req := httptest.NewRequest("GET", segment, nil)
	req.AddCookie(cookies["member"])
	req.Header.Set("Range", "bytes=0-31")
	r := httptest.NewRecorder()
	router.ServeHTTP(r, req)
	if r.Code != 206 || r.Body.Len() != 32 {
		t.Fatal("byte ranges failed")
	}
	list := request("GET", "/api/videos", "", "", 200)
	if !strings.Contains(list.Body.String(), "HLS test") {
		t.Fatal("published video missing")
	}
	request("PATCH", base, `{"publicationStatus":"hidden"}`, "admin", 200)
	request("GET", segment, "", "member", 404)
	request("GET", preview, "", "", 404)
	request("DELETE", base, "", "admin", 204)
	request("GET", full, "", "admin", 404)
	bad := upload([]byte("not a real video"), "broken.mp4", "admin", 202)
	await(bad, "failed")
	request("POST", fmt.Sprintf("/api/admin/videos/%d/retry", bad), "{}", "admin", 202)
	await(bad, "failed")
	request("DELETE", fmt.Sprintf("/api/admin/videos/%d", bad), "", "admin", 204)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(s.root)
		if len(entries) == 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("deleted video files were not cleaned up")
}
