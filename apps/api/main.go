package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
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

	db, err := pgxpool.New(context.Background(), dbURL.String())
	if err != nil {
		return fmt.Errorf("cannot initialize database pool: check configuration and CA file")
	}
	defer db.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	router := gin.Default()

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
