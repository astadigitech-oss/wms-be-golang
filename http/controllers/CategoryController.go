package controllers

import (
	"liquid8/wms/config"
	"liquid8/wms/models"

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