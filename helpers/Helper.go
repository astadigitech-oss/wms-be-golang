package helpers

import (
	"database/sql"
	"liquid8/wms/config"
	"liquid8/wms/models"
	"net/url"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
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

func GenerateCodeSaleDocument(db *gorm.DB, userID uint64) (string, error) {
	const (
		length   = 5
		maxRetry = 10
		prefix   = "LQDSLE"
	)

	var lastCode sql.NullString

	// Ambil code terakhir berdasarkan user
	err := db.
		Model(&models.SaleDocument{}).
		Where("user_id = ?", userID).
		Select("code_document_sale").
		Order("id DESC").
		Limit(1).
		Scan(&lastCode).
		Error
	if err != nil {
		return "", err
	}

	// Default jika belum ada data
	nextID := 1

	// Jika sudah ada code sebelumnya
	if lastCode.Valid {
		code := lastCode.String // contoh: LQDSLE00006

		numPart := code[len(prefix):] // ambil "00006"
		if num, err := strconv.Atoi(numPart); err == nil {
			nextID = num + 1
		}
	}

	// Retry jika ternyata code bentrok
	for attempt := 1; attempt <= maxRetry; attempt++ {

		barcode := fmt.Sprintf("%s%0*d", prefix, length, nextID)

		var count int64
		err := db.
			WithContext(context.Background()).
			Model(&models.SaleDocument{}).
			Where("code_document_sale = ?", barcode).
			Count(&count).
			Error
		if err != nil {
			return "", err
		}

		if count == 0 {
			return barcode, nil
		}

		// Jika sudah ada, naikkan angka & coba lagi
		nextID++
	}

	return "", errors.New("failed to generate unique code after max retries")
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

// ========================== Loyalty Service =====================
func ProcessLoyalty(
	tx *gorm.DB,
	buyer *models.Buyer,
	totalDisplayPrice float64,
) error {

	if totalDisplayPrice < 5000000 {
		return nil
	}

	now := time.Now().In(time.FixedZone("Asia/Jakarta", 7*3600))

	newTransaction := buyer.TransactionCount + 1
	updateData := map[string]interface{}{
		"transaction_count": newTransaction,
	}

	// ===========================
	// GET ELIGIBLE RANK
	// ===========================
	var eligibleRank models.LoyaltyRank
	if buyer.LoyaltyRankID == nil && buyer.TransactionCount == 0 {		
		if err := tx.
			Where("min_transactions <= ?", buyer.TransactionCount).
			Order("min_transactions DESC").
			Limit(1).
			First(&eligibleRank).Error; err != nil {
			return err
		}
	}else {
		if err := tx.
			Where("min_transactions <= ?", newTransaction).
			Order("min_transactions DESC").
			Limit(1).
			First(&eligibleRank).Error; err != nil {
			return err
		}
	}

	// ===========================
	// CHECK RANK CHANGE
	// ===========================
	var previousRankID *uint64

	rankChanged := buyer.LoyaltyRankID == nil ||
		*buyer.LoyaltyRankID != eligibleRank.ID

	if buyer.LoyaltyRankID != nil {
		previousRankID = buyer.LoyaltyRankID
	}

	if eligibleRank.ExpiredWeeks > 0 {
		expire := now.AddDate(0, 0, eligibleRank.ExpiredWeeks*7)
		updateData["expire_date"] = expire
	}

	if rankChanged {
		updateData["loyalty_rank_id"] = eligibleRank.ID
		updateData["last_upgrade_date"] = now
		updateData["loyalty_updated_at"] = now
	}

	// ===========================
	// UPDATE BUYER
	// ===========================
	if err := tx.Model(&models.Buyer{}).
		Where("id = ?", buyer.ID).
		Updates(updateData).Error; err != nil {
		return err
	}

	// ===========================
	// HISTORY (ONLY IF RANK CHANGED)
	// ===========================
	noted := "Rank upgraded to " + eligibleRank.Rank
	if rankChanged {
		if err := tx.Create(&models.BuyerLoyaltyHistory{
			BuyerID:        buyer.ID,
			PreviousRankID: previousRankID,
			CurrentRankID:  &eligibleRank.ID,
			Note:           &noted,
		}).Error; err != nil {
			return err
		}
	}

	return nil
}

type RankInfoResult struct {
	CurrentRank       *models.LoyaltyRank
	NextRank          *models.LoyaltyRank
	TransactionCount  int
	ExpireDate        *time.Time
	DiscountPercent   float64
}

func GetCurrentRankInfo(
	db *gorm.DB,
	buyerID uint,
	currentTransactionDate *time.Time,
) (*RankInfoResult, error) {

	loc, _ := time.LoadLocation("Asia/Jakarta")

	// =========================
	// Load Rank
	// =========================
	var allRanks []models.LoyaltyRank
	if err := db.Order("min_transactions asc").Find(&allRanks).Error; err != nil {
		return nil, err
	}

	// =========================
	// Load Buyer
	// =========================
	var buyer models.Buyer
	if err := db.Preload("Rank").
		First(&buyer, buyerID).Error; err != nil {
		return nil, err
	}

	// =========================
	// 3. Buyer Special
	// =========================
	specialBuyers := map[uint]bool{
		496: true,
	}

	if specialBuyers[buyerID] {
		var currentRank *models.LoyaltyRank
		if buyer.LoyaltyRankID == nil || buyer.Rank == nil {
			currentRank = &allRanks[0]
		} else {
			currentRank = buyer.Rank
		}

		var nextRank *models.LoyaltyRank
		for _, r := range allRanks {
			if r.MinTransactions > currentRank.MinTransactions {
				nextRank = &r
				break
			}
		}

		rankInfoResult := RankInfoResult{
			CurrentRank:      currentRank,
			NextRank:         nextRank,
			TransactionCount: currentRank.MinTransactions,
			ExpireDate: nil,
			DiscountPercent:  currentRank.PercentageDiscount,
		}

		if currentRank.ExpiredWeeks > 0 {
			now := time.Now().In(time.FixedZone("Asia/Jakarta", 7*3600))
			expire := now.AddDate(0, 0, currentRank.ExpiredWeeks*7)
			rankInfoResult.ExpireDate = &expire
		}


		return &rankInfoResult, nil
	}

	// =========================
	// Ambil Transaksi
	// =========================
	query := db.
		Where("buyer_id = ?", buyerID).
		Where("status = ?", "selesai").
		Where("total_display_price >= ?", 5000000).
		Where("created_at >= ?", time.Date(2025, 6, 1, 0, 0, 0, 0, loc))

	if currentTransactionDate != nil {
		query = query.Where("created_at <= ?", *currentTransactionDate)
	}

	var transactions []models.SaleDocument
	if err := query.
		Order("created_at asc").
		Select("created_at").
		Find(&transactions).Error; err != nil {
		return nil, err
	}

	// =========================
	// Buyer Baru
	// =========================
	if len(transactions) == 0 {
		currentRank := allRanks[0]

		var nextRank *models.LoyaltyRank
		if len(allRanks) > 1 {
			nextRank = &allRanks[1]
		}

		return &RankInfoResult{
			CurrentRank:      &currentRank,
			NextRank:         nextRank,
			TransactionCount: 0,
			ExpireDate:       nil,
			DiscountPercent:  currentRank.PercentageDiscount,
		}, nil
	}

	// =========================
	// Simulasi
	// =========================
	currentTransactionCount := 0
	var simulatedExpireDate *time.Time

	storeClosureStart := time.Date(2025, 7, 11, 0, 0, 0, 0, loc)
	storeClosureEnd := time.Date(2025, 9, 19, 23, 59, 59, 0, loc)

	for _, trx := range transactions {
		trxDate := trx.CreatedAt

		// ===== EXPIRED =====
		for currentTransactionCount >= 2 &&
			simulatedExpireDate != nil &&
			trxDate.After(*simulatedExpireDate) {

			//jika rank expired date buyer berda diantara toko tutup
			//maka tambahkan masa expired buyer
			if simulatedExpireDate.After(storeClosureStart) &&
				simulatedExpireDate.Before(storeClosureEnd) {

				sisaHari := int(storeClosureStart.Sub(*simulatedExpireDate).Hours() / 24)
				if sisaHari < 0 {
					sisaHari = 0
				}

				extended := storeClosureEnd.AddDate(0, 0, 1+sisaHari)
				simulatedExpireDate = &extended

				/*jika transaction date tidak lebih besar dari tanggal expired rank
				maka jngan downgrade rank */
				if !trxDate.After(*simulatedExpireDate) {
					break
				}
			}

			// downgrade
			var currentRank *models.LoyaltyRank
			for i := len(allRanks) - 1; i >= 0; i-- {
				if allRanks[i].MinTransactions <= currentTransactionCount {
					currentRank = &allRanks[i]
					break
				}
			}

			var downgradedRank *models.LoyaltyRank
			if currentRank != nil {
				for i := len(allRanks) - 1; i >= 0; i-- {
					if allRanks[i].MinTransactions < currentRank.MinTransactions {
						downgradedRank = &allRanks[i]
						break
					}
				}
			}

			if downgradedRank == nil {
				downgradedRank = &allRanks[0]
			}

			if currentRank != nil && downgradedRank.ID == currentRank.ID {
				currentTransactionCount = downgradedRank.MinTransactions
				simulatedExpireDate = nil
				break
			}

			currentTransactionCount = downgradedRank.MinTransactions

			if downgradedRank.ExpiredWeeks > 0 && simulatedExpireDate != nil {
				newExpire := simulatedExpireDate.AddDate(0, 0, downgradedRank.ExpiredWeeks*7)
				simulatedExpireDate = &newExpire
			} else {
				simulatedExpireDate = nil
			}
		}

		// ===== TRANSAKSI =====
		currentTransactionCount++

		if currentTransactionCount >= 2 {
			effective := currentTransactionCount - 1

			var activeRank *models.LoyaltyRank
			for i := len(allRanks) - 1; i >= 0; i-- {
				if allRanks[i].MinTransactions <= effective {
					activeRank = &allRanks[i]
					break
				}
			}

			if activeRank != nil && activeRank.ExpiredWeeks > 0 {
				exp := trxDate.AddDate(0, 0, activeRank.ExpiredWeeks*7)
				simulatedExpireDate = &exp
			} else {
				simulatedExpireDate = nil
			}
		}
	}

	// =========================
	// Rank Final
	// =========================
	var finalRank *models.LoyaltyRank
	for i := len(allRanks) - 1; i >= 0; i-- {
		if allRanks[i].MinTransactions <= currentTransactionCount {
			finalRank = &allRanks[i]
			break
		}
	}

	if finalRank == nil {
		finalRank = &allRanks[0]
	}

	var nextRank *models.LoyaltyRank
	for _, r := range allRanks {
		if r.MinTransactions > finalRank.MinTransactions {
			nextRank = &r
			break
		}
	}

	return &RankInfoResult{
		CurrentRank:      finalRank,
		NextRank:         nextRank,
		TransactionCount: currentTransactionCount,
		ExpireDate:       simulatedExpireDate,
		DiscountPercent:  finalRank.PercentageDiscount,
	}, nil
}




