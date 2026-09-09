package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load("../../.env"); err != nil {
		log.Fatal("Cannot load .env file")
	}

	for _, key := range []string{
		"POSTGRES_USER",
		"POSTGRES_PASSWORD",
		"POSTGRES_DB",
		"POSTGRES_PORT",
	} {
		if os.Getenv(key) == "" {
			log.Fatalf("Missing %s in .env", key)
		}
	}

	dbURL := url.URL{
		Scheme: "postgres",
		User: url.UserPassword(
			os.Getenv("POSTGRES_USER"),
			os.Getenv("POSTGRES_PASSWORD"),
		),
		Host:     "127.0.0.1:" + os.Getenv("POSTGRES_PORT"),
		Path:     "/" + os.Getenv("POSTGRES_DB"),
		RawQuery: "sslmode=disable",
	}

	db, err := pgxpool.New(context.Background(), dbURL.String())
	if err != nil {
		log.Fatal("Invalid database configuration")
	}
	defer db.Close()

	router := gin.Default()

	if err := router.SetTrustedProxies(nil); err != nil {
		log.Print(err)
		return
	}

	router.GET("/health", healthHandler)
	router.GET("/ready", readyHandler(db))

	if err := router.Run("127.0.0.1:8080"); err != nil {
		log.Print(err)
	}
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
			log.Printf("Database connection failed: %v", err)

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