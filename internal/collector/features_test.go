package collector

import (
	"testing"
	"time"

	"github.com/iriski-app/backend/internal/clients/moex"
	"github.com/iriski-app/backend/internal/storage/models"
)

func TestHistoricalFeaturesAndCouponPeriod(t *testing.T) {
	date := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	zero := 0.0
	data := moex.MarketData{MaturityDate: "2027-02-03", FaceUnit: "SUR", FaceValue: testFloat(1000), CouponPercent: &zero, CouponValue: &zero, BondType: "Облигация с фиксированным (известным) купоном"}
	features := normalizeBondFeatures(data, models.MarketData{TradeDate: date, Source: models.SourceMoex, LoadedAt: date})
	if features.Currency == nil || *features.Currency != "RUB" || features.CouponPercent == nil || *features.CouponPercent != 0 || features.CouponValue == nil || *features.CouponValue != 0 || features.HasBuyback != nil || features.IsGovernment != nil || features.SettlementCalendarExact != nil {
		t.Fatalf("unexpected features: %+v", features)
	}
	schedule := moex.Bondization{Coupons: []moex.Coupon{{StartDate: "2026-02-04", CouponDate: "2026-08-05"}, {StartDate: "2026-08-05", CouponDate: "2027-02-03"}}}
	fillCouponPeriod(&features, schedule, date)
	if features.CouponPeriodDays == nil || *features.CouponPeriodDays != 182 || features.CouponFreq == nil || *features.CouponFreq != 2 {
		t.Fatalf("unexpected period: %+v", features)
	}
	other := normalizeBondFeatures(data, models.MarketData{TradeDate: date})
	fillCouponPeriod(&other, moex.Bondization{Coupons: []moex.Coupon{{StartDate: "2027-02-03", CouponDate: "2027-08-04"}}}, date)
	if other.CouponPeriodDays != nil {
		t.Fatal("future coupon was used for historical date")
	}
	data.SECID = "SU26207RMFS9"
	data.BuybackDateAvailable = true
	known := normalizeBondFeatures(data, models.MarketData{TradeDate: date})
	if known.IsGovernment == nil || !*known.IsGovernment || known.IsStructured == nil || *known.IsStructured || known.HasBuyback == nil || *known.HasBuyback || known.YieldDate == nil || !known.YieldDate.Equal(*known.MaturityDate) {
		t.Fatalf("unexpected known attributes: %+v", known)
	}
	data.BuybackDate = "2026-12-10"
	known = normalizeBondFeatures(data, models.MarketData{TradeDate: date})
	if known.HasBuyback == nil || !*known.HasBuyback || known.YieldDate == nil || known.YieldDate.Format("2006-01-02") != data.BuybackDate {
		t.Fatalf("buyback attributes: %+v", known)
	}
}
