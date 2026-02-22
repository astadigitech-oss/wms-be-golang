package controllers

import (
	"liquid8/wms/config"
	"liquid8/wms/models"
	"strconv"

	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
)

func TagColors(c *gin.Context) {
	query := c.Query("q")

	var color_tags []models.ColorTag

	// Query Category
	config.DB.
		Where("(name_color LIKE ?)", "%"+query+"%").
		Find(&color_tags)

	// Response
	c.JSON(200, gin.H{
		"success": true,
		"message": "List Tag Color",
		"resource": color_tags,
	})
}

type tagColorRequest struct {
	HexaCodeColor     string  `json:"hexa_code_color" binding:"required"`
	NameColor     string  `json:"name_color" binding:"required"`
	MinPriceColor  float64  `json:"min_price_color" binding:"required"`
	MaxPriceColor  float64  `json:"max_price_color" binding:"required"`
	FixedPriceColor float64 `json:"fixed_price_color" binding:"required"`
}

func AddTagColor(c *gin.Context) {
	var payload tagColorRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(400, gin.H{"status": false, "message": "Format JSON tidak valid"})
			return
		}
		errors := make(map[string]string)
		for _, e := range ve {
			field := e.Field()

			switch field {
				case "HexaCodeColor":
					if e.Tag() == "required" {
						errors["hexa_code_color"] = "Kode warna wajib diisi"
					}
				case "NameColor":
					if e.Tag() == "required" {
						errors["name_color"] = "Nama warna wajib diisi"
					}
				case "MinPriceColor":
					if e.Tag() == "required" {
						errors["min_price_color"] = "Harga minimum wajib diisi"
					}
				case "MaxPriceColor":
					if e.Tag() == "required" {
						errors["max_price_color"] = "Harga maksimum wajib diisi"
					}
				case "FixedPriceColor":
					if e.Tag() == "required" {
						errors["fixed_price_color"] = "Harga tetap wajib diisi"
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

	// =========================
	// CEK HEXA CODE 
	var existing models.ColorTag
	err := config.DB.
		Where("hexa_code_color = ?", payload.HexaCodeColor).
		First(&existing).Error

	if err == nil {
		// data ditemukan → hex sudah ada
		c.JSON(http.StatusConflict, gin.H{
			"status": false,
			"message": "Kode warna sudah digunakan",
		})
		return
	}

	if err != gorm.ErrRecordNotFound {
		// error DB selain not found
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": false,
			"message": "Gagal mengecek kode warna",
			"error": err.Error(),
		})
		return
	}

	color_tag := models.ColorTag{
		NameColor:     payload.NameColor,
		HexaCodeColor: payload.HexaCodeColor,
		MinPriceColor: payload.MinPriceColor,
		MaxPriceColor: payload.MaxPriceColor,
		FixedPriceColor: payload.FixedPriceColor,
	}

	if err := config.DB.Create(&color_tag).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to create color tag",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   true,
		"message":  "berhasil tambah color tag",
		"resource": color_tag,
	})
}

func UpdateTagColor(c *gin.Context) {
	id := c.Param("id")

	var payload tagColorRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		ve, ok := err.(validator.ValidationErrors)
		if !ok {
			c.JSON(400, gin.H{"status": false, "message": "Format JSON tidak valid"})
			return
		}
		errors := make(map[string]string)
		for _, e := range ve {
			field := e.Field()
			switch field {
				case "HexaCodeColor":
					if e.Tag() == "required" {
						errors["hexa_code_color"] = "Kode warna wajib diisi"
					}
				case "NameColor":
					if e.Tag() == "required" {
						errors["name_color"] = "Nama warna wajib diisi"
					}
				case "MinPriceColor":
					if e.Tag() == "required" {
						errors["min_price_color"] = "Harga minimum wajib diisi"
					}
				case "MaxPriceColor":
					if e.Tag() == "required" {
						errors["max_price_color"] = "Harga maksimum wajib diisi"
					}
				case "FixedPriceColor":
					if e.Tag() == "required" {
						errors["fixed_price_color"] = "Harga tetap wajib diisi"
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

	var color_tag models.ColorTag
	if err := config.DB.First(&color_tag, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "Color Tag tidak ditemukan",
		})
		return
	}

	// CEK DUPLIKASI HEXA CODE
	// =========================
	var duplicate models.ColorTag
	err := config.DB.
		Where("hexa_code_color = ? AND id <> ?", payload.HexaCodeColor, color_tag.ID).
		First(&duplicate).Error

	if err == nil {
		// hex dipakai record lain
		c.JSON(http.StatusConflict, gin.H{
			"status": false,
			"message": "Kode warna sudah digunakan",
			"errors": gin.H{
				"hexa_code_color": "Kode warna sudah terdaftar",
			},
		})
		return
	}

	if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": false,
			"message": "Gagal validasi kode warna",
			"error": err.Error(),
		})
		return
	}

	color_tag.NameColor = payload.NameColor
	color_tag.MinPriceColor = payload.MinPriceColor
	color_tag.MaxPriceColor = payload.MaxPriceColor
	color_tag.FixedPriceColor = payload.FixedPriceColor
	color_tag.HexaCodeColor = payload.HexaCodeColor

	if err := config.DB.Save(&color_tag).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "failed to update color tag",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   true,
		"message":  "berhasil edit color tag",
		"resource": color_tag,
	})
}

func DeleteTagColor(c *gin.Context) {
	id := c.Param("id")

	var color_tag models.ColorTag
	if err := config.DB.First(&color_tag, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "Color tag tidak ditemukan",
		})
		return
	}

	if err := config.DB.Delete(&color_tag).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "Gagal menghapus data",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   true,
		"message":  "berhasil hapus color tag",
		"resource": color_tag,
	})
}

func GetColorTagByPrice(c *gin.Context) {
	query := c.DefaultQuery("old_price", "0")
	price, err := strconv.ParseFloat(query, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "success": false, 
            "message": "Format harga tidak valid",
        })
        return
    }

	if price >= 100000 {
		var categories []models.Category

		// Query Category
		config.DB.Model(&models.Category{}).Find(&categories)

		// Response
		c.JSON(200, gin.H{
			"success": true,
			"message": "Data categories",
			"resource": gin.H{
				"category": categories,
				"warna": nil,
			},
		})

		return
	}else {
		var colors []models.ColorTag
		if err := config.DB.Where("min_price_color <= ?", price).
			Where("max_price_color >= ?", price).
			Find(&colors).Error; err != nil {
			c.JSON(500, gin.H{"success": false, "message": "Gagal mengambil data tag color", "error": err.Error()})
			return
		}
	
		c.JSON(200, gin.H{
			"success": true,
			"message": "List data color",
			"resource": gin.H{
				"category": nil,
				"warna": colors,
			},	
		})

		return
	}

}
