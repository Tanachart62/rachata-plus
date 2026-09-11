package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// บนเซิร์ฟเวอร์สามารถใช้ environment โดยไม่ต้องมีไฟล์ .env
	if err := godotenv.Load("../../.env"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load .env: %w", err)
	}

	for _, key := range []string{
		"POSTGRES_USER",
		"POSTGRES_PASSWORD",
		"POSTGRES_DB",
		"POSTGRES_PORT",
		"DB_HOST",
		"DB_SSLMODE",
		"HTTP_ADDR",
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
		// ไม่แสดง connection URL ซึ่งมีรหัสผ่าน
		return fmt.Errorf("cannot initialize database pool: check configuration and CA file")
	}
	defer db.Close()

	router := gin.Default()

	if err := router.SetTrustedProxies(nil); err != nil {
		return err
	}

	router.GET("/health", healthHandler)
	router.GET("/ready", readyHandler(db))

	return router.Run(os.Getenv("HTTP_ADDR"))
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