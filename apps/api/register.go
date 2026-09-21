package main

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"unicode/utf8"
)

type registerRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func registerHandler(c *gin.Context) {
	var req registerRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request",
		})
		return
	}

	req.Name = strings.TrimSpace(req.Name)

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

	c.JSON(http.StatusOK, gin.H{
		"status":          "validated",
		"account_created": false,
	})
}
