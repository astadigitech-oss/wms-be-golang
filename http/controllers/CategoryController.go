package controllers

import (
	"liquid8/wms/config"
	"liquid8/wms/models"

	"net/http"

	"github.com/gin-gonic/gin"
)

func Categories(c *gin.Context) {
	query := c.Query("q")

	var categories []models.Category

	// Query Category
	config.DB.
		Model(&models.Category{}).
		Where(
			config.DB.Where("name_category LIKE ?", "%"+query+"%"),
		).Find(&categories)


	// Response
	c.JSON(200, gin.H{
		"success": true,
		"message": "Data categories",
		"resource": categories,
	})
}

func AddCategory(c *gin.Context) {
	type AddCategoryRequest struct {
		NameCategory     string  `json:"name_category" binding:"required"`
		DiscountCategory int `json:"discount_category" binding:"required"`
		MaxPriceCategory float64 `json:"max_price_category" binding:"required"`
	}

	var payload AddCategoryRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": false,
			"error":  err.Error(),
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

	type UpdateCategoryRequest struct {
		NameCategory     string  `json:"name_category" binding:"required"`
		DiscountCategory int `json:"discount_category" binding:"required"`
		MaxPriceCategory float64 `json:"max_price_category" binding:"required"`
	}

	var payload UpdateCategoryRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": false,
			"error":  err.Error(),
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

