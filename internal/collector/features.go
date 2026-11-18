package collector

import (
	"math"
	"strings"
	"time"

	"github.com/iriski-app/backend/internal/clients/moex"
	"github.com/iriski-app/backend/internal/storage/models"
)

// normalizeBondFeatures uses historical fields from the same market snapshot.
func normalizeBondFeatures(data moex.MarketData, market models.MarketData) models.BondFeatures {
	features := models.BondFeatures{InstrumentID: market.InstrumentID, TradeDate: market.TradeDate, Source: market.Source, LoadedAt: market.LoadedAt, YTM: market.YTM}
	if date, err := time.Parse("2006-01-02", strings.TrimSpace(data.MaturityDate)); err == nil && !date.IsZero() {
		features.MaturityDate = &date
	}
	if data.BuybackDateAvailable {
		buyback := strings.TrimSpace(data.BuybackDate)
		if buyback == "" || buyback == "0000-00-00" {
			features.HasBuyback = new(false)
			features.YieldDate = features.MaturityDate
		} else if date, err := time.Parse("2006-01-02", buyback); err == nil && !date.IsZero() {
			features.HasBuyback = new(true)
			features.YieldDate = &date
		}
	}
	if secID := strings.TrimSpace(data.SECID); secID != "" {
		government := strings.HasPrefix(secID, "SU")
		features.IsGovernment = &government
	}
	if bondType := strings.TrimSpace(data.BondType); bondType != "" {
		structured := bondType == "Структурная облигация"
		features.IsStructured = &structured
	}
	switch strings.TrimSpace(data.BondType) {
	case "Облигация с плавающим купоном":
		features.CouponType = new("floating")
	case "Облигация с фиксированным (известным) купоном":
		features.CouponType = new("fixed")
	}
	if data.CouponPercent != nil && validNumber(*data.CouponPercent) && *data.CouponPercent >= 0 {
		features.CouponPercent = data.CouponPercent
	}
	if data.CouponValue != nil && validNumber(*data.CouponValue) && *data.CouponValue >= 0 {
		features.CouponValue = data.CouponValue
	}
	if data.FaceValue != nil && validNumber(*data.FaceValue) && *data.FaceValue > 0 {
		features.FaceValue = data.FaceValue
	}
	currency := strings.ToUpper(strings.TrimSpace(data.FaceUnit))
	if currency == "SUR" {
		currency = "RUB"
	}
	if len(currency) == 3 {
		features.Currency = &currency
	}
	return features
}

func fillCouponPeriod(features *models.BondFeatures, schedule moex.Bondization, date time.Time) {
	var period *float64
	var latestEnd time.Time
	for _, coupon := range schedule.Coupons {
		start, startErr := time.Parse("2006-01-02", coupon.StartDate)
		end, endErr := time.Parse("2006-01-02", coupon.CouponDate)
		if startErr != nil || endErr != nil || end.After(date) {
			continue
		}
		days := end.Sub(start).Hours() / 24
		if days <= 0 || days > 366 || end.Before(latestEnd) {
			continue
		}
		if end.Equal(latestEnd) && period != nil {
			features.CouponPeriodDays = nil
			features.CouponFreq = nil
			return
		}
		period = &days
		latestEnd = end
	}
	if period != nil {
		features.CouponPeriodDays = period
		freq := math.RoundToEven(365 / *period)
		features.CouponFreq = &freq
	}
}
