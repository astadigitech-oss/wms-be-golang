package main

import (
	"liquid8/wms/api"
	"liquid8/wms/config"
	// "liquid8/wms/models"
	"github.com/gin-gonic/gin"
)

func main() {
	config.InitDB()
	// 
	server := gin.Default()

	// Middleware CORS
	server.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "http://localhost:3000") // frontend origin
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true") // optional jika kamu pakai cookies

		// Kalau OPTIONS, langsung balas OK tanpa lanjut handler lain
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	api.RouteHandler(server)

	server.Run(":5001")
}