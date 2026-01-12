package helpers

import (
	"liquid8/wms/config"
	"liquid8/wms/models"
	"net/url"

	"context"
	"math/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)


func ParseIntOrDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	if err != nil {
		// try float then cast
		var f float64
		_, err2 := fmt.Sscanf(s, "%f", &f)
		if err2 != nil {
			return def
		}
		return int(f)
	}
	return i
}

func StringToInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

func ParseFloatOrDefault(s string, def float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	if err != nil {
		return def
	}
	return f
}

func Float64Ptr(v float64) *float64 {
	return &v
}

func GetFloat64Value(ptr *float64) float64 {
    if ptr != nil {
        return *ptr
    }
    return 0.0
}

type InfeType map[string]interface{}
func LogUserAction(userID uint, name string, action string, page string, info InfeType) error {
	infoJSON, err := json.Marshal(info)
	if err != nil {
		return err 
	}
	
	log := models.UserLog{
		UserID:   userID,
		NameUser: name,
		Action: action,
		Page:     page,
		Info:     string(infoJSON),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return config.DB.Create(&log).Error
}

func ToStringNumber(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case uint:
		return strconv.FormatUint(uint64(t), 10)
	case int:
		return strconv.Itoa(t)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	default:
		return ""
	}
}

func RandomString(n int) string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	ret := make([]byte, n)
	for i := 0; i < n; i++ {
		ret[i] = letters[rand.Intn(len(letters))]
	}
	return string(ret)
}

func GenerateUniqueBarcode(db *gorm.DB, userID uint, custome_barcode string) (string, error) {
	const (
		length   = 5
		maxRetry = 10
	)

	now := time.Now()
	datePart := fmt.Sprintf("%02d%02d", now.Day(), int(now.Month()))

	//ambil MAX(id)
	// var nextID int64
	// err := db.Model(&models.Product{}).
	// 	Select("COALESCE(MAX(id), 0) + 1").
	// 	Scan(&nextID).Error
	// if err != nil {
	// 	return "", err
	// }

	for attempt := 1; attempt <= maxRetry; attempt++ {

		// --- generate random alphanumeric ---
		random := RandomString(length)
		barcode := fmt.Sprintf("%sL%d%s%s", custome_barcode, userID, datePart, random)

		// --- cek apakah barcode sudah ada di DB ---
		var count int64
		err := db.
			WithContext(context.Background()).
			Model(&models.Product{}).
			Where("barcode = ?", barcode).
			Count(&count).
			Error

		if err != nil {
			return "", err
		}

		// --- jika belum digunakan, selesai ---
		if count == 0 {
			return barcode, nil
		}
	}

	// --- jika gagal setelah banyak percobaan ---
	return "", errors.New("failed to generate unique barcode after max retries")
}

func GenerateCodeDocument(db *gorm.DB) (string, error) {
	const (
		length   = 4
		maxRetry = 10
	)

	now := time.Now()
	datePart := fmt.Sprintf("%02d%02d", now.Day(), int(now.Month()))

	//ambil MAX(id)
	// var nextID int64
	// err := db.Model(&models.Product{}).
	// 	Select("COALESCE(MAX(id), 0) + 1").
	// 	Scan(&nextID).Error
	// if err != nil {
	// 	return "", err
	// }

	for attempt := 1; attempt <= maxRetry; attempt++ {

		// --- generate random alphanumeric ---
		random := RandomString(length)
		barcode := fmt.Sprintf("DOC%s%s", datePart, random)

		// --- cek apakah barcode sudah ada di DB ---
		var count int64
		err := db.
			WithContext(context.Background()).
			Model(&models.Document{}).
			Where("code = ?", barcode).
			Count(&count).
			Error

		if err != nil {
			return "", err
		}

		// --- jika belum digunakan, selesai ---
		if count == 0 {
			return barcode, nil
		}
	}

	// --- jika gagal setelah banyak percobaan ---
	return "", errors.New("failed to generate unique code after max retries")
}

func GenerateCodeMigrateRepair(db *gorm.DB) (string, error) {
	const (
		length   = 4
		maxRetry = 5
	)

	now := time.Now()
	datePart := fmt.Sprintf("%02d%02d%04d", now.Day(), int(now.Month()), now.Year())

	//ambil MAX(id)
	// var nextID int64
	// err := db.Model(&models.Product{}).
	// 	Select("COALESCE(MAX(id), 0) + 1").
	// 	Scan(&nextID).Error
	// if err != nil {
	// 	return "", err
	// }

	for attempt := 1; attempt <= maxRetry; attempt++ {

		// --- generate random alphanumeric ---
		random := RandomString(length)
		barcode := fmt.Sprintf("%s%s", datePart, random)

		// --- cek apakah barcode sudah ada di DB ---
		var count int64
		err := db.
			WithContext(context.Background()).
			Model(&models.MigrateRepairDocument{}).
			Where("code = ?", barcode).
			Count(&count).
			Error

		if err != nil {
			return "", err
		}

		// --- jika belum digunakan, selesai ---
		if count == 0 {
			return barcode, nil
		}
	}

	// --- jika gagal setelah banyak percobaan ---
	return "", errors.New("failed to generate unique code after max retries")
}

func GenerateBarcodeBundle(db *gorm.DB) (string, error) {
	const (
		length   = 5
		maxRetry = 10
	)

	for attempt := 1; attempt <= maxRetry; attempt++ {

		// --- generate random alphanumeric ---
		random := RandomString(length)
		barcode := fmt.Sprintf("LQB%s", string(random))

		// --- cek apakah barcode sudah ada di DB ---
		var count int64
		err := db.
			WithContext(context.Background()).
			Model(&models.Bundle{}).
			Where("barcode = ?", barcode).
			Count(&count).
			Error

		if err != nil {
			return "", err
		}

		// --- jika belum digunakan, selesai ---
		if count == 0 {
			return barcode, nil
		}
	}

	// --- jika gagal setelah banyak percobaan ---
	return "", errors.New("failed to generate unique barcode after max retries")
}

func GenerateBarcodeBundleRepair(db *gorm.DB) (string, error) {
	const (
		length   = 5
		maxRetry = 10
	)

	//ambil MAX(id)
	// var nextID int64
	// err := db.Model(&models.Product{}).
	// 	Select("COALESCE(MAX(id), 0) + 1").
	// 	Scan(&nextID).Error
	// if err != nil {
	// 	return "", err
	// }

	for attempt := 1; attempt <= maxRetry; attempt++ {

		// --- generate random alphanumeric ---
		random := RandomString(length)
		barcode := fmt.Sprintf("LR%s", string(random))

		// --- cek apakah barcode sudah ada di DB ---
		var count int64
		err := db.
			WithContext(context.Background()).
			Model(&models.Bundle{}).
			Where("barcode = ?", barcode).
			Count(&count).
			Error

		if err != nil {
			return "", err
		}

		// --- jika belum digunakan, selesai ---
		if count == 0 {
			return barcode, nil
		}
	}

	// --- jika gagal setelah banyak percobaan ---
	return "", errors.New("failed to generate unique barcode after max retries")
}


// ========================= Custom Error =========================
type CustomError struct {
	StatusCode int
	Message    string
	Err        error 
}

func (e *CustomError) Error() string {
	if e.Err != nil {
		// Gabungkan pesan kustom dengan pesan error asli (jika ada)
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func NewCustomError(code int, msg string, err error) *CustomError {
	return &CustomError{
		StatusCode: code,
		Message:    msg,
		Err:        err,
	}
}

func GetToday() string {
	location,_ := time.LoadLocation("Asia/Jakarta")

	nowInJakarta := time.Now().In(location)

	// 3. Truncate waktu ke awal hari (00:00:00) di Jakarta
	startOfDayInJakarta := time.Date(
		nowInJakarta.Year(),
		nowInJakarta.Month(),
		nowInJakarta.Day(),
		0, 0, 0, 0,
		location,
	)

	return startOfDayInJakarta.Format("2006-01-02")
}

func BuildPaginationLinks(
	c *gin.Context,
	currentPage int,
	lastPage int,
) []gin.H {

	links := []gin.H{}

	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}

	baseURL := fmt.Sprintf(
		"%s://%s%s",
		scheme,
		c.Request.Host,
		c.Request.URL.Path,
	)

	existingQueries := c.Request.URL.Query()
	buildURL := func(page int) string {
		params := make(url.Values)
        for k, v := range existingQueries {
			params[k] = v
		}

        // Override atau Set parameter 'page'
        params.Set("page", strconv.Itoa(page))
        
        return baseURL + "?" + params.Encode()
	}

	// PREVIOUS
	links = append(links, gin.H{
		"url":    ternary(currentPage > 1, buildURL(currentPage-1), nil),
		"label":  "&laquo; Previous",
		"active": false,
	})

	window := 2
	start := max(2, currentPage-window)
	end := min(lastPage-1, currentPage+window)

	// FIRST PAGE
	links = append(links, gin.H{
		"url":    buildURL(1),
		"label":  "1",
		"active": currentPage == 1,
	})

	// LEFT DOTS
	if start > 2 {
		links = append(links, gin.H{
			"url": nil,
			"label": "...",
			"active": false,
		})
	}

	// MIDDLE
	for i := start; i <= end; i++ {
		links = append(links, gin.H{
			"url":    buildURL(i),
			"label":  strconv.Itoa(i),
			"active": i == currentPage,
		})
	}

	// RIGHT DOTS
	if end < lastPage-1 {
		links = append(links, gin.H{
			"url": nil,
			"label": "...",
			"active": false,
		})
	}

	// LAST PAGE
	if lastPage > 1 {
		links = append(links, gin.H{
			"url":    buildURL(lastPage),
			"label":  strconv.Itoa(lastPage),
			"active": currentPage == lastPage,
		})
	}

	// NEXT
	links = append(links, gin.H{
		"url":    ternary(currentPage < lastPage, buildURL(currentPage+1), nil),
		"label":  "Next &raquo;",
		"active": false,
	})

	return links
}

func ternary(condition bool, a, b interface{}) interface{} {
	if condition {
		return a
	}
	return b
}