package controllers

import (
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"strconv"

	"github.com/gin-gonic/gin"
)

func GetPaletIndex(c *gin.Context) {
	query := c.Query("q")

	var palets []models.Palet

	db := config.DB.
		Preload("PaletSyncApproves").
		Preload("PaletProducts").
		Order("created_at DESC")

	// Jika ada keyword pencarian
	if query != "" {
		db = db.Where(
			config.DB.Where("name_palet LIKE ?", "%"+query+"%").
				Or("category_palet LIKE ?", "%"+query+"%").
				Or("id IN (?)",
					config.DB.
						Select("palet_id").
						Table("palet_products").
						Where("new_name_product LIKE ?", "%"+query+"%").
						Or("new_barcode_product LIKE ?", "%"+query+"%").
						Or("new_category_product LIKE ?", "%"+query+"%").
						Or("old_barcode_product LIKE ?", "%"+query+"%").
						Or("new_tag_product LIKE ?", "%"+query+"%"),
				),
		)
	}

	// ===== PAGINATION =====
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	limit := 20
	offset := (page - 1) * limit

	var total int64
	db.Model(&models.Palet{}).Count(&total)

	err := db.
		Limit(limit).
		Offset(offset).
		Find(&palets).Error

	if err != nil {
		c.JSON(500, gin.H{
			"success": false,
			"message": "Gagal mengambil data palet",
			"error": err.Error(),
		})
		return
	}

	lastPage := int((total + int64(limit) - 1) / int64(limit))
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"success": true,
		"message": "list palet",
		"data": gin.H{
			"data":        palets,
			"current_page": page,
			"total_data":   total,
			"from":         offset + 1,
			"last_page":    lastPage,
			"links":       links,
			"per_page":     limit,
			"to":           offset + len(palets),
		},
	})
}

