package main

import (
	"log"
	"net/http"
	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	if err := router.SetTrustedProxies(nil); err != nil {
		log.Fatal(err)
	}

	router.GET("/health", healthHandler)

	if err := router.Run("127.0.0.1:8080"); err != nil {
		log.Fatal(err)
	}
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "rachata-plus-api",
	})
}
