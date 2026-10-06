package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type runtimeConfig struct {
	mode, worker, delivery, addr string
	videos                       bool
	threads                      int
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func readRuntimeConfig() (runtimeConfig, error) {
	r := runtimeConfig{mode: envDefault("APP_MODE", "api"), worker: envDefault("VIDEO_WORKER_MODE", "embedded"), delivery: envDefault("MEDIA_DELIVERY", "direct"), addr: os.Getenv("HTTP_ADDR"), videos: os.Getenv("LOCAL_VIDEO_ENABLED") == "true"}
	if r.mode != "api" && r.mode != "worker" {
		return r, fmt.Errorf("APP_MODE must be api or worker")
	}
	if r.worker != "embedded" && r.worker != "external" {
		return r, fmt.Errorf("VIDEO_WORKER_MODE must be embedded or external")
	}
	if r.delivery != "direct" && r.delivery != "nginx" {
		return r, fmt.Errorf("MEDIA_DELIVERY must be direct or nginx")
	}
	if r.mode == "worker" && !r.videos {
		return r, fmt.Errorf("worker requires LOCAL_VIDEO_ENABLED=true")
	}
	if _, _, err := net.SplitHostPort(r.addr); err != nil {
		return r, fmt.Errorf("HTTP_ADDR must include host and port")
	}
	r.threads = 2
	if text := os.Getenv("FFMPEG_THREADS"); text != "" {
		value, err := strconv.Atoi(text)
		if err != nil || value < 1 || value > 4 {
			return r, fmt.Errorf("FFMPEG_THREADS must be 1..4")
		}
		r.threads = value
	}
	switch envDefault("APP_ENV", "local") {
	case "local":
		if r.videos {
			host, _, _ := net.SplitHostPort(r.addr)
			local := os.Getenv("DB_HOST") == "127.0.0.1" && host == "127.0.0.1"
			compose := os.Getenv("DB_HOST") == "db" && host == "0.0.0.0"
			if !local && !compose {
				return r, fmt.Errorf("local videos require loopback, or the private Compose db network")
			}
		}
	case "production":
		// Validate HTTPS/cookie settings even in the separate worker process.
		if os.Getenv("COOKIE_SECURE") != "true" {
			return r, fmt.Errorf("production requires COOKIE_SECURE=true and HTTPS")
		}
		if _, err := newAuthConfig(os.Getenv("AUTH_ORIGIN"), "true"); err != nil {
			return r, err
		}
		if r.worker != "external" || r.delivery != "nginx" {
			return r, fmt.Errorf("production requires external worker and nginx media delivery")
		}
	default:
		return r, fmt.Errorf("APP_ENV must be local or production")
	}
	return r, nil
}

func checkProcessHealth() error {
	path := "/ready"
	if os.Getenv("APP_MODE") == "worker" {
		path = "/worker/ready"
	}
	_, port, err := net.SplitHostPort(os.Getenv("HTTP_ADDR"))
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://127.0.0.1:" + port + path)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("process not ready")
	}
	return nil
}

func trustedProxyCIDRs(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	var result []string
	for _, item := range strings.Split(value, ",") {
		cidr := strings.TrimSpace(item)
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS must contain explicit CIDRs")
		}
		ones, _ := network.Mask.Size()
		if ones == 0 {
			return nil, fmt.Errorf("cannot trust every address as a reverse proxy")
		}
		result = append(result, cidr)
	}
	return result, nil
}
