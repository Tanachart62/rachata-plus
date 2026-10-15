package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func avatarFixture(t *testing.T, width, height int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestAvatarJPEG(t *testing.T) {
	data, err := avatarJPEG(bytes.NewReader(avatarFixture(t, 800, 400)))
	if err != nil {
		t.Fatal(err)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "jpeg" || config.Width != 256 || config.Height != 256 || len(data) > 262144 {
		t.Fatal("invalid stored avatar", config, format, err)
	}
	for _, bad := range [][]byte{nil, []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), bytes.Repeat([]byte("x"), maxAvatarBytes+1), avatarFixture(t, 4097, 1)} {
		if _, err := avatarJPEG(bytes.NewReader(bad)); err == nil {
			t.Fatal("invalid avatar accepted")
		}
	}
}

func testProfileAvatars(t *testing.T, ctx context.Context, db *pgxpool.Pool, a *authConfig) {
	t.Helper()
	var id int64
	err := db.QueryRow(ctx, `INSERT INTO users(username,name,email,password_hash) VALUES('avataruser','Avatar User','avataruser@example.com','unused') RETURNING id`).Scan(&id)
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
	router := gin.New()
	g := router.Group("/auth")
	g.Use(a.protectWrites())
	a.accountRoutes(g, db)
	request := func(method string, owner int64, data []byte, authenticated bool, origin string) *httptest.ResponseRecorder {
		t.Helper()
		var b bytes.Buffer
		writer := multipart.NewWriter(&b)
		if method == "POST" {
			_ = writer.WriteField("expectedUserId", fmt.Sprint(owner))
			file, _ := writer.CreateFormFile("file", "../avatar.png")
			_, _ = file.Write(data)
		}
		_ = writer.Close()
		req := httptest.NewRequest(method, fmt.Sprintf("/auth/avatar?expectedUserId=%d", owner), &b)
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		if authenticated {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	get := func(owner int64, auth bool, status int) []byte {
		t.Helper()
		w := request("GET", owner, nil, auth, a.origin)
		if w.Code != status {
			t.Fatalf("read avatar %d %s", w.Code, w.Body.String())
		}
		if status == 200 && (w.Header().Get("Content-Type") != "image/jpeg" || w.Header().Get("Cache-Control") != "private, no-store") {
			t.Fatal("unsafe avatar headers")
		}
		return w.Body.Bytes()
	}
	get(id, false, 401)
	get(id, true, 404)
	get(id+1, true, 409)
	valid := avatarFixture(t, 400, 300)
	for _, test := range []struct {
		owner  int64
		image  []byte
		auth   bool
		origin string
		status int
	}{
		{id, valid, false, a.origin, 401}, {id, valid, true, "https://other.example", 403},
		{id + 1, valid, true, a.origin, 409}, {id, valid, true, a.origin, 204},
	} {
		w := request("POST", test.owner, test.image, test.auth, test.origin)
		if w.Code != test.status {
			t.Fatalf("upload avatar %d want %d %s", w.Code, test.status, w.Body.String())
		}
	}
	before := append([]byte(nil), get(id, true, 200)...)
	for _, bad := range [][]byte{[]byte(strings.Repeat("x", maxAvatarBytes+1)), []byte("not an image")} {
		w := request("POST", id, bad, true, a.origin)
		if w.Code != 400 {
			t.Fatal("invalid upload accepted", w.Code)
		}
		if !bytes.Equal(before, get(id, true, 200)) {
			t.Fatal("failed upload changed stored avatar")
		}
	}
}
