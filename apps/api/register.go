package main

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type registerRequest struct {
	Username string `json:"username" binding:"required"`
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required"`
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,30}$`)

func registerHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(
			c.Writer,
			c.Request.Body,
			8*1024,
		)

		var req registerRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			var sizeErr *http.MaxBytesError
			if errors.As(err, &sizeErr) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{
					"error": "request_too_large",
				})
				return
			}

			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid_request",
			})
			return
		}

		req.Name = strings.TrimSpace(req.Name)
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))
		req.Username = strings.ToLower(strings.TrimSpace(req.Username))
		if !usernamePattern.MatchString(req.Username) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_username"})
			return
		}

		nameLength := utf8.RuneCountInString(req.Name)
		if nameLength == 0 || nameLength > 100 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_name",
				"message": "Name must contain 1-100 Unicode code points.",
			})
			return
		}

		passwordLength := utf8.RuneCountInString(req.Password)
		if passwordLength < 15 || passwordLength > 128 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_password_length",
				"message": "Password must contain 15-128 Unicode code points.",
			})
			return
		}

		passwordHash, err := hashPassword(req.Password)
		if err != nil {
			log.Printf("register: password hashing failed")

			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "internal_error",
			})
			return
		}

		ctx, cancel := context.WithTimeout(
			c.Request.Context(),
			3*time.Second,
		)
		defer cancel()

		var userID int64

		err = db.QueryRow(
			ctx,
			`
				INSERT INTO users (username, name, email, password_hash, role)
				VALUES ($1, $2, $3, $4, 'user')
				RETURNING id
			`,
			req.Username,
			req.Name,
			req.Email,
			passwordHash,
		).Scan(&userID)

		if err != nil {
			var dbErr *pgconn.PgError

			if errors.As(err, &dbErr) {
				if dbErr.Code == "23505" && dbErr.ConstraintName == "users_username_unique" {
					c.JSON(http.StatusConflict, gin.H{"error": "username_already_exists"})
					return
				}
				if dbErr.Code == "23505" &&
					dbErr.ConstraintName == "users_email_unique" {
					c.JSON(http.StatusConflict, gin.H{
						"error": "email_already_exists",
					})
					return
				}

				log.Printf(
					"register: database error SQLSTATE=%s",
					dbErr.Code,
				)
			} else {
				log.Printf(
					"register: database operation failed (%T)",
					err,
				)
			}

			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "internal_error",
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"account_created": true,
			"user": gin.H{
				"username": req.Username,
				"id":       userID,
				"name":     req.Name,
				"email":    req.Email,
				"role":     "user",
			},
		})
	}
}
