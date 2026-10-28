package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := checkProcessHealth(); err != nil {
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	envFile := os.Getenv("ENV_FILE")
	loadEnv := godotenv.Load
	if envFile == "" {
		envFile = "../../.env"
	} else {
		// An explicitly selected local file must override stale RDS variables.
		loadEnv = godotenv.Overload
	}
	if err := loadEnv(envFile); err != nil && (os.Getenv("ENV_FILE") != "" || !os.IsNotExist(err)) {
		return fmt.Errorf("load .env: %w", err)
	}
	runtime, err := readRuntimeConfig()
	if err != nil {
		return err
	}

	for _, key := range []string{
		"POSTGRES_USER",
		"POSTGRES_PASSWORD",
		"POSTGRES_DB",
		"POSTGRES_PORT",
		"DB_HOST",
		"DB_SSLMODE",
	} {
		if os.Getenv(key) == "" {
			return fmt.Errorf("missing environment variable: %s", key)
		}
	}

	sslMode := os.Getenv("DB_SSLMODE")
	rootCert := os.Getenv("DB_SSLROOTCERT")

	if sslMode == "verify-full" || sslMode == "verify-ca" {
		if rootCert == "" {
			return fmt.Errorf("DB_SSLROOTCERT is required for %s", sslMode)
		}

		if _, err := os.Stat(rootCert); err != nil {
			return fmt.Errorf("read database CA certificate: %w", err)
		}
	}

	dbURL := url.URL{
		Scheme: "postgres",
		User: url.UserPassword(
			os.Getenv("POSTGRES_USER"),
			os.Getenv("POSTGRES_PASSWORD"),
		),
		Host: net.JoinHostPort(
			os.Getenv("DB_HOST"),
			os.Getenv("POSTGRES_PORT"),
		),
		Path: "/" + os.Getenv("POSTGRES_DB"),
	}

	query := dbURL.Query()
	query.Set("sslmode", sslMode)

	if rootCert != "" {
		query.Set("sslrootcert", rootCert)
	}

	dbURL.RawQuery = query.Encode()

	poolConfig, err := pgxpool.ParseConfig(dbURL.String())
	if err != nil {
		return fmt.Errorf("invalid database configuration")
	}
	poolConfig.MaxConns = 10
	poolConfig.MinConns = 0
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnLifetimeJitter = 5 * time.Minute
	if value := os.Getenv("DB_MAX_CONNS"); value != "" {
		n, e := strconv.Atoi(value)
		if e != nil || n < 2 || n > 100 {
			return fmt.Errorf("DB_MAX_CONNS must be 2..100")
		}
		poolConfig.MaxConns = int32(n)
	}
	db, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return fmt.Errorf("cannot initialize database pool: check configuration and CA file")
	}
	defer db.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if len(os.Args) >= 2 && os.Args[1] == "import-media" {
		apply := len(os.Args) == 3 && os.Args[2] == "--apply"
		if len(os.Args) > 3 || (len(os.Args) == 3 && !apply) {
			return fmt.Errorf("usage: rachata import-media [--apply]")
		}
		return importMedia(ctx, db, apply)
	}

	router := gin.New()
	// Playback permits are query parameters; never include them in access logs.
	router.Use(gin.LoggerWithFormatter(func(p gin.LogFormatterParams) string {
		return fmt.Sprintf("[HTTP] %s %q %d %s\n", p.Method, p.Request.URL.Path, p.StatusCode, p.Latency)
	}), gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		// Recovery's default request dump can contain cookies or permit URLs.
		log.Printf("[HTTP] panic method=%s path=%q", c.Request.Method, c.Request.URL.Path)
		c.AbortWithStatus(http.StatusInternalServerError)
	}))

	proxies, err := trustedProxyCIDRs(os.Getenv("TRUSTED_PROXY_CIDRS"))
	if err != nil {
		return err
	}
	if err := router.SetTrustedProxies(proxies); err != nil {
		return err
	}

	router.GET("/health", healthHandler)
	router.GET("/ready", readyHandler(db))
	var auth *authConfig
	if runtime.mode == "api" {
		auth, err = newAuthConfig(os.Getenv("AUTH_ORIGIN"), os.Getenv("COOKIE_SECURE"))
		if err != nil {
			return err
		}
		accounts := router.Group("/auth")
		if os.Getenv("APP_ENV") == "production" {
			accounts.Use(auth.sharedWriteLimit(db))
		}
		auth.store, err = newObjectStorage(ctx)
		if err != nil {
			return err
		}
		accounts.Use(auth.protectWrites())
		accounts.POST("/register", registerHandler(db))
		accounts.POST("/login", auth.loginHandler(db))
		accounts.POST("/logout", auth.logoutHandler(db))
		accounts.GET("/me", auth.meHandler(db))
		auth.accountRoutes(accounts, db)
	}
	if runtime.videos {
		videos, err := newVideoService(db, auth, os.Getenv("MEDIA_DIR"))
		if err != nil {
			return err
		}
		videos.nginxDelivery = runtime.delivery == "nginx"
		if videos.store != nil {
			check, stop := context.WithTimeout(ctx, 10*time.Second)
			_, err = db.Exec(check, `SELECT cover_s3_key FROM videos LIMIT 0; SELECT avatar_s3_key FROM users LIMIT 0; SELECT id FROM direct_uploads LIMIT 0; SELECT hits FROM request_limits LIMIT 0`)
			stop()
			if err != nil {
				return fmt.Errorf("AWS storage requires migration 015 and a reachable database")
			}
		}
		if runtime.mode == "api" {
			videos.routes(router)
		}
		if runtime.mode == "worker" || runtime.worker == "embedded" {
			if err := videos.initTranscoder(runtime.threads); err != nil {
				return err
			}
			done := make(chan struct{})
			go func() { defer close(done); videos.worker(ctx) }()
			defer func() { cancel(); <-done }()
			if runtime.mode == "worker" {
				router.GET("/worker/ready", func(c *gin.Context) {
					if !videos.leaseActive.Load() {
						c.Status(503)
						return
					}
					readyHandler(db)(c)
				})
			}
		}
	}
	server := &http.Server{
		Addr: runtime.addr, Handler: router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Minute,
		WriteTimeout:      15 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "rachata-plus-api",
	})
}

func readyHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			log.Printf("Database readiness check failed: %v", err)

			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":   "not_ready",
				"database": "unavailable",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":   "ready",
			"database": "connected",
		})
	}
}
