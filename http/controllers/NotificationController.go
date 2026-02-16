package controllers

import (
	"fmt"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NotifWidget(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User) // biasanya dari middleware JWT
	query := c.Query("q")

	// Base query
	notifQuery := config.DB.
		Model(&models.Notification{}).
		Order("created_at DESC").
		Limit(5)

	// Role filter
	allowedRoles := map[uint]bool{
		1: true,
		2: true,
		5: true,
		8: true,
	}

	if !allowedRoles[user.RoleID] {
		notifQuery = notifQuery.Where("status != ?", "sale")
	}

	// Search filter
	if query != "" {
		notifQuery = notifQuery.Where("status LIKE ?", "%"+query+"%")
	}

	var notifications []models.Notification

	if err := notifQuery.Find(&notifications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"message": "Failed to fetch notifications",
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message": "Notifications",
		"resource":    notifications,
	})
}

func GetNotifications(c *gin.Context) {
	user := c.MustGet("auth_user").(models.User) // biasanya dari middleware JWT
	query := c.Query("q")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 33
	offset := (page - 1) * limit

	// Base query
	notifQuery := config.DB.Model(&models.Notification{})

	// Role filter
	allowedRoles := map[uint]bool{
		1: true,
		2: true,
		5: true,
		8: true,
	}

	if !allowedRoles[user.RoleID] {
		notifQuery = notifQuery.Where("status != ?", "sale")
	}

	// Search filter
	if query != "" {
		notifQuery = notifQuery.Where("status LIKE ?", "%"+query+"%")
	}

	var totalData int64
	if err := notifQuery.Session(&gorm.Session{}).Count(&totalData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"message": "Gagal menghitung total data",
			"error": err.Error(),
		})
		return
	}

	var notifications []models.Notification
	if err := notifQuery.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&notifications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":  false,
			"message": "Failed to fetch notifications",
			"error": err.Error(),
		})
		return
	}

	lastPage := int(math.Ceil(float64(totalData) / float64(limit)))

	// pagination links
	links := helpers.BuildPaginationLinks(c, page, lastPage)

	c.JSON(200, gin.H{
		"data": gin.H{
			"status":  true,
			"message": "Notifications",
			"resource": gin.H{
				"current_page": 	page,
                "total_data":           totalData,
                "data":                 notifications,
				"from":           offset + 1,
				"last_page":      lastPage,
				"links":          links,
				"per_page":       limit,
				"to":             offset + len(notifications),
			},
		},
	})
}

func GetApproveSPV(c *gin.Context) {
	status := c.Param("status")
	notifID := c.Param("notif_id")
	// user := c.MustGet("auth_user").(models.User)

	var notification models.Notification
	if err := config.DB.Where("id = ? AND status = ?", notifID, status).First(&notification).Error; err != nil {
		c.JSON(404, gin.H{
			"success": false,
			"message": "notification not found",
			"error": err.Error(),
		})
		return
	}

	switch status {

	// INVENTORY
	case "inventory":

		var product models.Product
		if err := config.DB.Table("products").
			Select(`
				products.*,
				color_tags.name_color AS color_tag,
				categories.name_category AS category
			`).
			Joins("LEFT JOIN color_tags ON color_tags.id = products.tag_color_id").
			Joins("LEFT JOIN categories ON categories.id = products.category_id").
			Where("products.location_type = ?", "main").
			Where("products.id = ?", notification.ExternalID).
			First(&product).Error; err != nil {

			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "data product not found",
				"error":   err.Error(),
			})
			return
		}

		var approve models.ApproveQueue
		err := config.DB.Preload("User").
			Preload("Category").
			Preload("ColorTag").
			Where("product_id = ? AND type = ? AND status <> ?", notification.ExternalID, "inventory", "0").
			First(&approve).Error

		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "data approve not found",
				"error": err.Error(),
			})
			return
		}

		dataNew := map[string]interface{}{
			"user":       approve.User.Username,
			"product_id":   approve.ProductID,
			"barcode":     product.Barcode,
			"type":  approve.Type,
			"code_document":  approve.CodeDocument,
			"old_price_product":   approve.OldPriceProduct,
			"new_name_product":    approve.NewNameProduct,
			"new_quantity_product": approve.NewQuantityProduct,
			"new_price_product":   approve.NewPriceProduct,
			"new_discount":   approve.NewDiscount,
			"color_tag":       nil,
			"category":        nil,
			"created_at":     approve.CreatedAt,
			"updated_at":     approve.UpdatedAt,
		}

		if approve.ColorTag != nil {
			dataNew["color_tag"] = approve.ColorTag.NameColor
		}
		if approve.Category != nil {
			dataNew["category"] = approve.Category.NameCategory
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "approved edit product",
			"data": gin.H{
				"approve_id": approve.ID,
				"dataOld": product,
				"dataNew": dataNew,
			},
		})
		return

	// SALE
	case "sale":

		var sale models.SaleDocument

		err := config.DB.Where("id = ? AND approved = ?", notification.ExternalID, "1").
			Preload("Sales", func(tx *gorm.DB) *gorm.DB {
				return tx.
					Select(
						"id",
						"sale_document_id",
						"product_name",
						"product_old_price",
						"product_category",
						"product_barcode",
						"bundle_barcode",
						"product_price_sale",
						"product_quantity",
						"total_discount_sale",
						"discount_sale",
						"created_at",
						"base_price",
						"approved",
					)
			}).
			First(&sale).Error

		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Document is not approved",
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "approved invoice discount",
			"data":    sale,
		})
		return

	// STAGING
	case "staging":

		var product models.Product
		if err := config.DB.Table("products").
			Select(`
				products.*,
				color_tags.name_color AS color_tag,
				categories.name_category AS category
			`).
			Joins("LEFT JOIN color_tags ON color_tags.id = products.tag_color_id").
			Joins("LEFT JOIN categories ON categories.id = products.category_id").
			Where("products.location_type = ?", "staging").
			Where("products.id = ?", notification.ExternalID).
			First(&product).Error; err != nil {

			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "data product not found",
				"error":   err.Error(),
			})
			return
		}

		var approve models.ApproveQueue
		err := config.DB.Preload("User").
			Where("product_id = ? AND type = ? AND status <> ?", notification.ExternalID, "staging", "0").
			First(&approve).Error

		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "data approve not found",
				"error": err.Error(),
			})
			return
		}

		dataNew := map[string]interface{}{
			"user":       approve.User.Username,
			"product_id":   approve.ProductID,
			"barcode":     product.Barcode,
			"type":  approve.Type,
			"code_document":  approve.CodeDocument,
			"old_price_product":   approve.OldPriceProduct,
			"new_name_product":    approve.NewNameProduct,
			"new_quantity_product": approve.NewQuantityProduct,
			"new_price_product":   approve.NewPriceProduct,
			"new_discount":   approve.NewDiscount,
			"color_tag":       nil,
			"category":        nil,
			"created_at":     approve.CreatedAt,
			"updated_at":     approve.UpdatedAt,
		}

		if approve.ColorTag != nil {
			dataNew["color_tag"] = approve.ColorTag.NameColor
		}
		if approve.Category != nil {
			dataNew["category"] = approve.Category.NameCategory
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "approved edit product",
			"data": gin.H{
				"approve_id": approve.ID,
				"dataOld": product,
				"dataNew": dataNew,
			},
		})
		return

	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid status",
		})
	}
}

func ApproveEdit(c *gin.Context) {
	approve_id := c.Param("approve_id")

	//start transaction
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
                "success": false, 
                "message": "Internal server error",
                "error": fmt.Sprintf("%v", r),
            })
        }
    }()

	var approve models.ApproveQueue
	if err := tx.First(&approve, approve_id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "approve queue not found",
		})
		return
	}

	// sudah diproses
	if approve.Status == "0" {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "already approved",
		})
		return
	}

	// ambil product
	var product models.Product
	if err := tx.First(&product, approve.ProductID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "product not found",
		})
		return
	}

	var display_price float64
	if approve.NewDiscount != nil {
		display_price = *approve.NewPriceProduct * (1 - *approve.NewDiscount/100)
	}else {
		display_price = *approve.NewPriceProduct
	}

	// update product
	product.OldPriceProduct = *approve.OldPriceProduct
	product.Name = *approve.NewNameProduct
	product.Quantity = int64(*approve.NewQuantityProduct)
	product.Price = *approve.NewPriceProduct
	product.Discount = approve.NewDiscount
	product.TagColorID = approve.TagColorID
	product.CategoryID = approve.CategoryID
	product.DisplayPrice = display_price

	if err := tx.Save(&product).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed update product",
			"error":   err.Error(),
		})
		return
	}

	barcode := product.Barcode

	// update approve queue
	if err := tx.Model(&approve).
		Update("status", "0").Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed update approve queue",
		})
		return
	}

	// update notification
	var notif models.Notification
	err := tx.Where("user_id = ? AND status = ? AND external_id = ?",
		approve.UserID, approve.Type, approve.ProductID).
		First(&notif).Error

	if err == nil {
		tx.Model(&notif).Update("approved", "2")
	}

	// commit
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed commit transaction",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "approved successfully",
		"barcode": barcode,
	})
}

func RejectEdit(c *gin.Context) {
	approve_id := c.Param("approve_id")

	//start transaction
	tx := config.DB.WithContext(c.Request.Context()).Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to start database transaction"})
		return
	}
    
    // Pastikan Rollback jika terjadi panic
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
                "success": false, 
                "message": "Internal server error",
                "error": fmt.Sprintf("%v", r),
            })
        }
    }()

	var approve models.ApproveQueue
	if err := tx.First(&approve, approve_id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "approve queue not found",
		})
		return
	}

	if approve.Status == "0" {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "already approved",
		})
		return
	}

	// hapus approve queue
	if err := tx.Delete(&approve).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed delete approve queue",
			"error":   err.Error(),
		})
		return
	}

	// hapus notification
	tx.Where("user_id = ? AND status = ? AND external_id = ?",
		approve.UserID, approve.Type, approve.ProductID).
		Where("approved = ?", "0").
		Delete(&models.Notification{})

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed commit",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "reject successfully",
	})
}



