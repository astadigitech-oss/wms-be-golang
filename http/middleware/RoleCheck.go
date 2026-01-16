package middleware

import (
	"liquid8/wms/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RoleCheck menerima slice of string (roles) yang diizinkan untuk route tertentu
func RoleCheck(allowedRoles []string) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. Ambil role user dari Context
        user := c.MustGet("auth_user").(models.User)

        isAllowed := false
        for _, role := range allowedRoles {
            if user.Role.RoleName == role {
                isAllowed = true
                break
            }
        }

        if !isAllowed {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "success": false, 
                "message": "Forbidden. Insufficient permissions for this resource.",
            })
            return
        }
		
        c.Next()
    }
}