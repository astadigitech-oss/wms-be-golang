package controllers

import (
	"fmt"
	"liquid8/wms/config"
	"liquid8/wms/models"
	"os"
	"path/filepath"
	"strings"

	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/xuri/excelize/v2"
)

func Categories(c *gin.Context) {
	query := c.Query("q")

	var categories []models.Category

	// Query Category
	config.DB.
		Model(&models.Category{}).
		Where("(name_category LIKE ?)", "%"+query+"%").
		Find(&categories)


	// Response
	c.JSON(200, gin.H{
		"success": true,
		"message": "Data categories",
		"resource": categories,
	})
}

func ExportCategory(c *gin.Context) {
	db := config.DB
	// Buat file excel
	f := excelize.NewFile()
	sheet := "Sheet1"
	f.SetSheetName("Sheet1", sheet)

	// Header
	headers := []string{
		"ID", "Nama Category", "Discount", "Max Price Discount",
		"Created At", "Updated At",
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	rowIndex := 2
	limit := 1000
	lastId := 0

	for {
		var categories []models.Category

		err := db.
			Where("id > ?", lastId).
			Order("id ASC").
			Limit(limit).
			Find(&categories).Error

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}

		if len(categories) == 0 {
			break
		}

		for _, category := range categories {

			f.SetCellValue(sheet, fmt.Sprintf("A%d", rowIndex), category.ID)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", rowIndex), category.NameCategory)
			f.SetCellValue(sheet, fmt.Sprintf("C%d", rowIndex), category.DiscountCategory)
			f.SetCellValue(sheet, fmt.Sprintf("D%d", rowIndex), category.MaxPriceCategory)
			f.SetCellValue(sheet, fmt.Sprintf("E%d", rowIndex), category.CreatedAt.Format("2006-01-02 15:05:14"))
			f.SetCellValue(sheet, fmt.Sprintf("F%d", rowIndex), category.UpdatedAt.Format("2006-01-02 15:05:14"))

			rowIndex++
			lastId = int(category.ID)
		}
	}

	// Buat folder exports jika belum ada
	publicPath := "./public/exports"
	err := os.MkdirAll(publicPath, os.ModePerm)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed create directory",
		})
		return
	}

	fileName := "categories.xlsx"
	filePath := filepath.Join(publicPath, fileName)

	// Save file
	if err := f.SaveAs(filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	downloadURL := fmt.Sprintf("%s/public/exports/%s", os.Getenv("APP_URL"), fileName)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "file diunduh",
		"data":    downloadURL,
	})
}

type payloadRequest struct {
	NameCategory     string  `json:"name_category" binding:"required"`
	DiscountCategory int `json:"discount_category" binding:"required"`
	MaxPriceCategory float64 `json:"max_price_category" binding:"required"`
}

func AddCategory(c *gin.Context) {
	var payload payloadRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(400, gin.H{"status": false, "message": "Format JSON tidak valid"})
			return
		}
		errors := make(map[string]string)
		for _, e := range ve {
			field := strings.ToLower(e.Field())

			switch field {
				case "namecategory":
					if e.Tag() == "required" {
						errors["name_category"] = "Nama category wajib diisi"
					}
				case "discountcategory":
					if e.Tag() == "required" {
						errors["discount_category"] = "Discount category wajib diisi"
					}
				case "maxpricecategory":
					if e.Tag() == "required" {
						errors["max_price_category"] = "Max price category wajib diisi"
					}
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"message": "Validasi gagal",
			"errors": errors,
		})
		
		return
	}

	category := models.Category{
		NameCategory:     payload.NameCategory,
		DiscountCategory: payload.DiscountCategory,
		MaxPriceCategory: payload.MaxPriceCategory,
	}

	if err := config.DB.Create(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to create category",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   true,
		"message":  "berhasil tambah category",
		"resource": category,
	})
}

func UpdateCategory(c *gin.Context) {
	id := c.Param("id")

	var payload payloadRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(400, gin.H{"status": false, "message": "Format JSON tidak valid"})
			return
		}
		errors := make(map[string]string)
		for _, e := range ve {
			field := strings.ToLower(e.Field())

			switch field {
				case "namecategory":
					if e.Tag() == "required" {
						errors["name_category"] = "Nama category wajib diisi"
					}
				case "discountcategory":
					if e.Tag() == "required" {
						errors["discount_category"] = "Discount category wajib diisi"
					}
				case "maxpricecategory":
					if e.Tag() == "required" {
						errors["max_price_category"] = "Max price category wajib diisi"
					}
			}
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"status": false,
			"message": "Validasi gagal",
			"errors": errors,
		})
		
		return
	}

	var category models.Category
	if err := config.DB.First(&category, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "Category tidak ditemukan",
		})
		return
	}

	category.NameCategory = payload.NameCategory
	category.DiscountCategory = payload.DiscountCategory
	category.MaxPriceCategory = payload.MaxPriceCategory

	if err := config.DB.Save(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to update category",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   true,
		"message":  "berhasil edit category",
		"resource": category,
	})
}

func DeleteCategory(c *gin.Context) {
	id := c.Param("id")

	var category models.Category
	if err := config.DB.First(&category, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "Data tidak ditemukan",
		})
		return
	}

	if err := config.DB.Delete(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "Gagal menghapus data",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   true,
		"message":  "berhasil hapus category",
		"resource": category,
	})
}

