package jobs

import (
	"fmt"
	"liquid8/wms/config"
	"liquid8/wms/models"
	"time"

	"github.com/sirupsen/logrus"
)

func RunExpirdBuyerLoyalty(log *logrus.Logger) {
	log.Info("===== Process expired buyer loyalty is started")
	db := config.DB
	loc, _ := time.LoadLocation("Asia/Jakarta")
	startDate := time.Date(2025, 6, 1, 0, 0, 0, 0, loc)
	now := time.Now().In(loc)

	// Ambil semua rank
	var ranks []models.LoyaltyRank
	if err := db.Order("min_transactions asc").Find(&ranks).Error; err != nil {
		log.WithError(err).Error("Gagal mengambil data ranks")
		return
	}
	if len(ranks) == 0 {
		log.Error("Tidak ada data ranks")
		return
	}

	lowestRank := ranks[0]

	// Ambil semua transaksi eligible
	var transactions []models.SaleDocument
	err := db.
		Where("created_at >= ?", startDate).
		Where("status = ?", "selesai").
		Where("total_display_price >= ?", 5000000).
		Order("buyer_id asc").
		Order("created_at asc").
		Find(&transactions).Error

	if err != nil {
		log.WithError(err).Error("Gagal mengambil data transactions")
		return
	}

	if len(transactions) == 0 {
		log.WithError(err).Error("Tidak ada data transaction")
		return
	}

	// Group transaksi per buyer
	grouped := make(map[uint64][]models.SaleDocument)
	for _, trx := range transactions {
		grouped[trx.BuyerID] =
			append(grouped[trx.BuyerID], trx)
	}

	// Hitung loyalty per buyer (di memory)
	for buyerID, trxList := range grouped {

		currentCount := 0
		currentRank := lowestRank
		var expireDate *time.Time

		for _, trx := range trxList {

			trxDate := trx.CreatedAt.In(loc)

			// downgrade sebelum transaksi
			for currentCount > 0 && expireDate != nil && trxDate.After(*expireDate) {

				downgraded := getLowerRank(ranks, currentRank.MinTransactions)

				if downgraded == nil || downgraded.MinTransactions == 0 {
					currentRank = lowestRank
					currentCount = 0
					expireDate = nil
					break
				}

				currentRank = *downgraded
				currentCount = downgraded.MinTransactions

				if downgraded.ExpiredWeeks > 0 {
					newExpire := expireDate.AddDate(0, 0, downgraded.ExpiredWeeks*7)
					expireDate = checkAndExtendGracePeriod(&newExpire, &trxDate)
				} else {
					expireDate = nil
				}
			}

			// transaksi baru
			currentCount++

			newRank := getRankByTransactionCount(ranks, currentCount)
			currentRank = newRank

			if currentRank.ExpiredWeeks > 0 {
				newExpire := trxDate.AddDate(0, 0, currentRank.ExpiredWeeks*7)
				expireDate = checkAndExtendGracePeriod(&newExpire, &trxDate)
			} else {
				expireDate = nil
			}
		}

		// final downgrade sampai sekarang
		for currentCount > 0 && expireDate != nil && now.After(*expireDate) {
			downgraded := getLowerRank(ranks, currentRank.MinTransactions)

			if downgraded == nil || downgraded.MinTransactions == 0 {
				currentRank = lowestRank
				currentCount = 0
				expireDate = nil
				break
			}

			currentRank = *downgraded
			currentCount = downgraded.MinTransactions

			if downgraded.ExpiredWeeks > 0 {
				newExpire := expireDate.AddDate(0, 0, downgraded.ExpiredWeeks*7)
				expireDate = checkAndExtendGracePeriod(&newExpire, nil)
			} else {
				expireDate = nil
			}
		}

		// Update langsung ke buyer
		err := db.Model(&models.Buyer{}).
			Where("id = ?", buyerID).
			Updates(map[string]interface{}{
				"loyalty_rank_id":  currentRank.ID,
				"transaction_count": currentCount,
				"last_upgrade_date": now,
				"expire_date":       expireDate,
			}).Error

		if err != nil {
			log.WithError(err).Error(fmt.Sprintf("Gagal update data buyer | Buyer ID : %d", buyerID))
			return
		}
	}

	log.Info("===== Expire buyer loyaltyc completed successfully")
}

func getRankByTransactionCount(ranks []models.LoyaltyRank, count int) models.LoyaltyRank {
	selected := ranks[0]
	for _, r := range ranks {
		if r.MinTransactions <= count {
			selected = r
		}
	}
	return selected
}

func getLowerRank(ranks []models.LoyaltyRank, currentMin int) *models.LoyaltyRank {
	var lower *models.LoyaltyRank
	for i := range ranks {
		if ranks[i].MinTransactions < currentMin {
			lower = &ranks[i]
		}
	}
	return lower
}

type closurePeriod struct {
	Start time.Time
	End   time.Time
}

func getClosurePeriods() []closurePeriod {

	loc, _ := time.LoadLocation("Asia/Jakarta")

	return []closurePeriod{
		{
			Start: time.Date(2025, 7, 11, 0, 0, 0, 0, loc),
			End:   time.Date(2025, 9, 19, 23, 59, 59, 0, loc),
		},
		{
			Start: time.Date(2026, 2, 1, 0, 0, 0, 0, loc),
			End:   time.Date(2026, 2, 11, 23, 59, 59, 0, loc),
		},
	}
}

func checkAndExtendGracePeriod(
	expireDate *time.Time,
	transactionDate *time.Time,
) *time.Time {

	periods := getClosurePeriods()
	if expireDate == nil {
		return nil
	}

	finalDate := *expireDate

	var trxDate *time.Time
	if transactionDate != nil {
		t := transactionDate.Truncate(24 * time.Hour)
		trxDate = &t
	}

	for _, period := range periods {

		start := period.Start.Truncate(24 * time.Hour)
		end := period.End.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

		// Jika closure selesai sebelum transaksi → skip
		if trxDate != nil && end.Before(*trxDate) {
			continue
		}

		// Jika closure mulai setelah expire → skip
		if start.After(finalDate) {
			continue
		}

		duration := int(end.Sub(start).Hours()/24) + 1
		finalDate = finalDate.AddDate(0, 0, duration)
	}

	return &finalDate
}

