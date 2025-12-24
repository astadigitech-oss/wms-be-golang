package controllers

import (
	"liquid8/wms/config"
	"liquid8/wms/models"

	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

func TagColors(c *gin.Context) {
	query := c.Query("q")

	var color_tags []models.ColorTag

	// Query Category
	config.DB.
		Where("name_color LIKE ?", "%"+query+"%").
		Find(&color_tags)

	// Response
	c.JSON(200, gin.H{
		"success": true,
		"message": "Data categories",
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
		validationErrors := err.(validator.ValidationErrors)

		errors := make(map[string]string)

		for _, e := range validationErrors {
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
		validationErrors := err.(validator.ValidationErrors)

		errors := make(map[string]string)

		for _, e := range validationErrors {
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

