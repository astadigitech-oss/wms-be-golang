package middleware

import (
	"liquid8/wms/models"
	"liquid8/wms/config"

	"net/http"
	"strings"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func AuthCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token tidak ditemukan"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "format token salah"})
			c.Abort()
			return
		}

		jwtSecret := []byte(os.Getenv("JWT_SECRET"))

		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			config.DB.Where("token = ?", tokenString).Delete(&models.UserToken{})
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token tidak valid atau kadaluarsa"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token tidak valid"})
			c.Abort()
			return
		}

		// Cek token di DB
		var userToken models.UserToken
		err = config.DB.Where("token = ?", tokenString).First(&userToken).Error
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token tidak ditemukan di database"})
			c.Abort()
			return
		}

		go func() {
			config.DB.Model(&userToken).Update("last_used_at", time.Now())
		}()

		var user models.User
		if err := config.DB.First(&user, claims["user_id"]).Error; err != nil {
			c.AbortWithStatusJSON(403, gin.H{
				"status": false,
				"message": "user not found",
			})
			return
		}

		c.Set("auth_user", user)
		c.Next()
	}
}
