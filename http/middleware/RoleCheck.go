package middleware

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

// RoleCheck menerima slice of string (roles) yang diizinkan untuk route tertentu
func RoleCheck(allowedRoles []string) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. Ambil role user dari Context
        roleInterface, exists := c.Get("role") 
        if !exists {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "success": false, 
                "message": "User role not found in context. Check authentication middleware.",
            })
            return
        }

        // Konversi interface{} menjadi string
        userRole, ok := roleInterface.(string)
        if !ok {
            c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
                "success": false, 
                "message": "Internal error: Failed to assert user role type.",
            })
            return
        }

        isAllowed := false
        for _, role := range allowedRoles {
            if userRole == role {
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