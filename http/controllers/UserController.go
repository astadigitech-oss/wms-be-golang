package controllers

import (
	"fmt"
	"liquid8/wms/config"
	"liquid8/wms/helpers"

	// "liquid8/wms/helpers"
	"liquid8/wms/models"

	// "errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	// "gorm.io/gorm"
	"golang.org/x/crypto/bcrypt"
	"github.com/go-playground/validator/v10"
)

func GetUsers(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	var users []models.User
	var totalData int64

	//inisialisasi query
	query := config.DB.Model(&models.User{}).Preload("Role")

	// Searching (misalnya, mencari berdasarkan nama atau email)
	if q != "" {
		searchPattern := "%" + q + "%"
		query = query.Where("name LIKE ? OR email LIKE ?", searchPattern, searchPattern)
	}

	// Menghitung total data yang sesuai dengan filter/search sebelum diterapkan limit/offset
	if err := query.Count(&totalData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal menghitung total data pengguna", "error": err.Error()})
		return
	}

	err := query.
		Limit(limit).
		Offset(offset).
		Order("created_at desc"). // Sorting data terbaru di atas
		Find(&users).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type ScanAgg struct {
		UserID    uint
		TotalScanToday int64
		TotalScan int64
	}

	var scanAggs []ScanAgg

	today := helpers.GetToday()
	if err := config.DB.
		Model(&models.UserScanWeb{}).
		Select(`
			user_id,
			SUM(CASE WHEN scan_date = ? THEN total_scans ELSE 0 END) AS total_scan_today,
			SUM(total_scans) AS total_scan
		`, today).
		Group("user_id").
		Scan(&scanAggs).Error;  err != nil {
		
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// MAP AGGREGASI → USER
	// =========================
	scanMap := make(map[uint]ScanAgg)
	for _, s := range scanAggs {
		scanMap[s.UserID] = s
	}
	
	// BUILD RESPONSE DATA
	// =========================
	type UserResponse struct {
		models.User
		TotalScanToday int64 `json:"total_scan_today"`
		TotalScan int64 `json:"total_scan"`
	}

	var result []UserResponse
	for _, u := range users {
		agg := scanMap[u.ID]

		result = append(result, UserResponse{
			User:      u,
			TotalScanToday: agg.TotalScanToday,
			TotalScan: agg.TotalScan,
		})
	}


	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))

	baseURL := c.Request.Host + c.Request.URL.Path
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	fullURL := scheme + "://" + baseURL

	// pagination links
	links := []gin.H{
		{
			"url":    nil,
			"label":  "&laquo; Previous",
			"active": false,
		},
	}

	if lastPage <= 8 {
        // Jika total halaman 10 atau kurang, tampilkan semua
        for i := 1; i <= lastPage; i++ {
            links = append(links, gin.H{
                "url":    fmt.Sprintf("%s?page=%d&q=%s", fullURL, i, q),
                "label":  strconv.Itoa(i),
                "active": i == page,
            })
        }
    } else {
        for i := 1; i <= 8; i++ {
            links = append(links, gin.H{
                "url":    fmt.Sprintf("%s?page=%d&q=%s", fullURL, i, q),
                "label":  strconv.Itoa(i),
                "active": i == page,
            })
        }

        // Tambahkan separator "..."
        links = append(links, gin.H{
            "url":    nil,
            "label":  "...",
            "active": false,
        })

        for i := lastPage - 1; i <= lastPage; i++ {
            links = append(links, gin.H{
                "url":    fmt.Sprintf("%s?page=%d&q=%s", fullURL, i, q),
                "label":  strconv.Itoa(i),
                "active": i == page,
            })
        }
    }

	links = append(links, gin.H{
		"url":    nil,
		"label":  "Next &raquo;",
		"active": false,
	})

	var nextPageURL interface{} = nil
	var prevPageURL interface{} = nil

	if page < lastPage {
		nextPageURL = fmt.Sprintf("%s?page=%d&q=%s", fullURL, page+1, q)
	}
	if page > 1 {
		prevPageURL = fmt.Sprintf("%s?page=%d&q=%s", fullURL, page-1, q)
	}

	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Users",
			"resource": gin.H{
				"current_page":   page,
				"data":           result,
				"first_page_url": fmt.Sprintf("%s?page=1&q=%s", fullURL, q),
				"from":           offset + 1,
				"last_page":      lastPage,
				"last_page_url":  fmt.Sprintf("%s?page=%d&q=%s", fullURL, lastPage, q),
				"links":          links,
				"next_page_url":  nextPageURL,
				"path":           fullURL,
				"per_page":       limit,
				"prev_page_url":  prevPageURL,
				"to":             offset + len(users),
				"total":          totalData,
			},
		},
	})
}

func GetRoles(c *gin.Context) {
	var roles []models.Role

	// Menghitung total data yang sesuai dengan filter/search sebelum diterapkan limit/offset
	if err := config.DB.Model(&models.Role{}).Find(&roles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "List Roles",
			"resource": roles,
		},
	})
}

func CreateUser(c *gin.Context) {
	type CreateUserRequest struct {
		Name     string `json:"name" binding:"required"`
		Username     string `json:"username" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=6"`
		RoleID   uint   `json:"role_id" binding:"required"`
	}

	var req CreateUserRequest

	// VALIDASI REQUEST
	// =========================
	if err := c.ShouldBindJSON(&req); err != nil {
		validationErrors := err.(validator.ValidationErrors)

		errors := make(map[string]string)

		for _, e := range validationErrors {
			field := strings.ToLower(e.Field())

			switch field {
			case "name":
				if e.Tag() == "required" {
					errors["name"] = "Nama wajib diisi"
				}

			case "email":
				if e.Tag() == "required" {
					errors["email"] = "Email wajib diisi"
				}
				if e.Tag() == "email" {
					errors["email"] = "Format email tidak valid"
				}

			case "password":
				if e.Tag() == "required" {
					errors["password"] = "Password wajib diisi"
				}
				if e.Tag() == "min" {
					errors["password"] = "Password minimal 6 karakter"
				}

			case "roleid":
				errors["role_id"] = "Role wajib dipilih"
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"message": "Validasi gagal",
			"errors": errors,
		})
		
		return
	}

	// CEK EMAIL SUDAH ADA
	// =========================
	var count int64
	if err := config.DB.
		Model(&models.User{}).
		Where("email = ?", req.Email).
		Count(&count).Error; err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "Email sudah terdaftar",
		})
		return
	}

	// HASH PASSWORD
	// =========================
	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(req.Password),
		bcrypt.DefaultCost,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// CREATE USER
	// =========================
	user := models.User{
		Name:     req.Name,
		Username:     req.Username,
		Email:    req.Email,
		Password: string(hashedPassword),
		RoleID:   req.RoleID,
	}

	if err := config.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "Gagal membuat user",
			"error":   err.Error(),
		})
		return
	}

	// LOAD RELATION
	// =========================
	if err := config.DB.Preload("Role").First(&user, user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// RESPONSE
	// =========================
	c.JSON(http.StatusCreated, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "User berhasil dibuat",
			"resource": user,
		},
	})
}

func UpdateUser(c *gin.Context) {
	userID := c.Param("id")

	type payload struct {
		Name     string `json:"name" binding:"required"`
		Username string `json:"username" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"omitempty,min=6"`
		RoleID   uint   `json:"role_id" binding:"required"`
	}

	var req payload

	// =========================
	// VALIDASI REQUEST
	// =========================
	if err := c.ShouldBindJSON(&req); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "Payload tidak valid",
			})
			return
		}

		errors := make(map[string]string)

		for _, e := range ve {
			field := strings.ToLower(e.Field())

			switch field {
			case "name":
				errors["name"] = "Nama wajib diisi"

			case "username":
				errors["username"] = "Username wajib diisi"

			case "email":
				if e.Tag() == "email" {
					errors["email"] = "Format email tidak valid"
				} else {
					errors["email"] = "Email wajib diisi"
				}

			case "password":
				errors["password"] = "Password minimal 6 karakter"

			case "roleid":
				errors["role_id"] = "Role wajib dipilih"
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "Validasi gagal",
			"errors":  errors,
		})
		return
	}

	// =========================
	// AMBIL USER EXISTING
	// =========================
	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "User tidak ditemukan",
		})
		return
	}

	// =========================
	// CEK EMAIL UNIK (KECUALI DIRI SENDIRI)
	// =========================
	var count int64
	if err := config.DB.
		Model(&models.User{}).
		Where("email = ? AND id != ?", req.Email, userID).
		Count(&count).Error; err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "Email sudah digunakan",
		})
		return
	}

	// =========================
	// UPDATE DATA USER
	// =========================
	user.Name = req.Name
	user.Username = req.Username
	user.Email = req.Email
	user.RoleID = req.RoleID

	// Password hanya diupdate kalau diisi
	if req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword(
			[]byte(req.Password),
			bcrypt.DefaultCost,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		user.Password = string(hashedPassword)
	}

	if err := config.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "Gagal mengupdate user",
			"error":   err.Error(),
		})
		return
	}

	// LOAD RELATION
	// =========================
	if err := config.DB.Preload("Role").First(&user, user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// =========================
	// RESPONSE
	// =========================
	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "User berhasil diupdate",
		"data":    user,
	})
}

func DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	// =========================
	// AMBIL USER
	// =========================
	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "User tidak ditemukan",
		})
		return
	}

	// =========================
	// DELETE USER
	// =========================
	if err := config.DB.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "Gagal menghapus user",
			"error":   err.Error(),
		})
		return
	}

	// =========================
	// RESPONSE
	// =========================
	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "User berhasil dihapus",
	})
}





