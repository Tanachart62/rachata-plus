package main

import (
	"bytes"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestVerifyPassword(t *testing.T) {
	const password = "รหัสผ่านสำหรับทดสอบเท่านั้น"
	encoded, err := hashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(password, encoded) {
		t.Fatal("correct Unicode password rejected")
	}
	if verifyPassword("wrong-password", encoded) {
		t.Fatal("incorrect password accepted")
	}
	for _, malformed := range []string{"", "garbage", strings.Replace(encoded, "m=19456", "m=4294967295", 1), strings.Replace(encoded, "v=19", "v=16", 1), encoded + "$extra"} {
		if verifyPassword(password, malformed) {
			t.Fatalf("accepted malformed hash %q", malformed)
		}
	}
}

func TestAuthConfig(t *testing.T) {
	for _, config := range [][2]string{{"http://example.com", "false"}, {"http://localhost:5173/path", "false"}, {"https://example.com", ""}, {"http://localhost:5173", "true"}, {"", "false"}} {
		if _, err := newAuthConfig(config[0], config[1]); err == nil {
			t.Fatalf("accepted unsafe config %v", config)
		}
	}
	if _, err := newAuthConfig("http://localhost:5173", "false"); err != nil {
		t.Fatal(err)
	}
	if _, err := newAuthConfig("https://example.com", "true"); err != nil {
		t.Fatal(err)
	}
}

func TestWriteProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a, err := newAuthConfig("http://localhost:5173", "false")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(a.protectWrites())
	router.POST("/auth/register", func(c *gin.Context) { c.Status(204) })
	for _, tc := range []struct {
		origin, content, site string
		want                  int
	}{
		{"", "application/json", "", 403},
		{"https://evil.example", "application/json", "", 403},
		{a.origin, "application/json", "cross-site", 403},
		{a.origin, "text/plain", "", 415},
		{a.origin, "application/json", "same-origin", 204},
	} {
		req := httptest.NewRequest("POST", "/auth/register", strings.NewReader("{}"))
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Content-Type", tc.content)
		req.Header.Set("Sec-Fetch-Site", tc.site)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("origin=%q content=%q: got %d want %d", tc.origin, tc.content, w.Code, tc.want)
		}
	}
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("POST", "/auth/register", strings.NewReader("{}"))
		req.Header.Set("Origin", a.origin)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if i == 9 && w.Code != 429 {
			t.Fatalf("rate limit not enforced: %d", w.Code)
		}
	}
}

func TestSessionTokenAndCookie(t *testing.T) {
	token, hash, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := newSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == other || len(token) != 43 {
		t.Fatal("tokens are not random 256-bit tokens")
	}
	expected := sha256.Sum256([]byte(token))
	if !bytes.Equal(hash, expected[:]) {
		t.Fatal("wrong stored digest")
	}
	a := &authConfig{secure: true}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/auth/me", nil)
	a.cookie(c, token, 86400)
	cookie := w.Result().Cookies()[1]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatal("unsafe cookie attributes")
	}
	c.Request.AddCookie(cookie)
	got, ok := sessionHash(c)
	if !ok || !bytes.Equal(got, hash) {
		t.Fatal("cookie digest mismatch")
	}
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	a.cookie(c, "", -1)
	if w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout did not expire cookie")
	}
}

func TestRegisterRejectsInvalidUsernameBeforeDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/auth/register", registerHandler(nil))
	for _, username := range []string{"ab", "invalid-name", "ภาษาไทย", strings.Repeat("a", 31)} {
		req := httptest.NewRequest("POST", "/auth/register", strings.NewReader(`{"username":"`+username+`","name":"Tester","email":"test@example.com","password":"long-enough-test-password"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 400 || !strings.Contains(w.Body.String(), "invalid_username") {
			t.Fatalf("invalid username not rejected: %s", w.Body.String())
		}
	}
}

func TestExpiredRateBucketBeforeSweep(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a, err := newAuthConfig("http://localhost:5173", "false")
	if err != nil {
		t.Fatal(err)
	}
	a.attempts["192.0.2.1"] = attemptWindow{count: 10, reset: time.Now().Add(-time.Second)}
	a.nextSweep = time.Now().Add(time.Minute)
	router := gin.New()
	router.Use(a.protectWrites())
	router.POST("/auth/login", func(c *gin.Context) { c.Status(204) })
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader("{}"))
	req.RemoteAddr = "192.0.2.1:1234"
	req.Header.Set("Origin", a.origin)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatal("expired current bucket must reset before scheduled sweep", w.Code)
	}
}
