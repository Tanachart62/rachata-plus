package main

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestRuntimeBoundaries(t *testing.T) {
	for key, value := range map[string]string{"APP_ENV": "local", "APP_MODE": "api", "VIDEO_WORKER_MODE": "external", "MEDIA_DELIVERY": "nginx", "LOCAL_VIDEO_ENABLED": "true", "HTTP_ADDR": "127.0.0.1:8080", "DB_HOST": "127.0.0.1", "FFMPEG_THREADS": "2"} {
		t.Setenv(key, value)
	}
	if _, err := readRuntimeConfig(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DB_HOST", "remote-rds.example")
	if _, err := readRuntimeConfig(); err == nil {
		t.Fatal("local runtime must not accept a remote database")
	}
	t.Setenv("DB_HOST", "db")
	t.Setenv("HTTP_ADDR", "0.0.0.0:8080")
	t.Setenv("APP_MODE", "worker")
	if _, err := readRuntimeConfig(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FFMPEG_THREADS", "100")
	if _, err := readRuntimeConfig(); err == nil {
		t.Fatal("unbounded transcoder threads")
	}
	t.Setenv("FFMPEG_THREADS", "2")
	t.Setenv("APP_ENV", "production")
	t.Setenv("COOKIE_SECURE", "false")
	if _, err := readRuntimeConfig(); err == nil {
		t.Fatal("production accepted insecure cookies")
	}
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("AUTH_ORIGIN", "https://videos.example.com")
	if _, err := readRuntimeConfig(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VIDEO_WORKER_MODE", "embedded")
	if _, err := readRuntimeConfig(); err == nil {
		t.Fatal("production accepted embedded worker")
	}
}

func TestProxyClientIPBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	proxies, err := trustedProxyCIDRs("172.30.84.0/24")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	if err = router.SetTrustedProxies(proxies); err != nil {
		t.Fatal(err)
	}
	router.GET("/", func(c *gin.Context) { c.String(200, c.ClientIP()) })
	for _, item := range []struct{ remote, forwarded, want string }{
		{"203.0.113.9:1234", "198.51.100.8", "203.0.113.9"},
		{"172.30.84.5:1234", "198.51.100.8", "198.51.100.8"},
	} {
		request := httptest.NewRequest("GET", "/", nil)
		request.RemoteAddr = item.remote
		request.Header.Set("X-Forwarded-For", item.forwarded)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Body.String() != item.want {
			t.Fatalf("wrong client IP: %s", response.Body.String())
		}
	}
	for _, value := range []string{"0.0.0.0/0", "0:0:0:0:0:0:0:0/0", "not-a-cidr"} {
		if _, err := trustedProxyCIDRs(value); err == nil {
			t.Fatalf("accepted unbounded/invalid proxy %s", value)
		}
	}
}
