package collector

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/iriski-app/backend/internal/clients/moex"
	"github.com/iriski-app/backend/internal/risk"
)

func buildCashFlows(instrument moex.Instrument, data moex.MarketData, schedule moex.Bondization, settlement time.Time) ([]risk.CashFlow, error) {
	if data.FaceValue == nil || !validNumber(*data.FaceValue) || *data.FaceValue <= 0 || instrument.CouponPeriod == nil || *instrument.CouponPeriod <= 0 || *instrument.CouponPeriod > 366 {
		return nil, fmt.Errorf("%w: missing face value or coupon period", ErrInvalidData)
	}
	maturityText := strings.TrimSpace(data.MaturityDate)
	if maturityText == "" || maturityText == "0000-00-00" {
		maturityText = strings.TrimSpace(instrument.MaturityDate)
	}
	maturity, err := time.Parse("2006-01-02", maturityText)
	if err != nil || !maturity.After(settlement) {
		return nil, fmt.Errorf("%w: invalid maturity", ErrInvalidData)
	}
	tradeDate, err := time.Parse("2006-01-02", data.TradeDate)
	if err != nil || settlement.Before(tradeDate) {
		return nil, fmt.Errorf("%w: invalid settlement date", ErrInvalidData)
	}
	horizon := maturity
	buyback := strings.TrimSpace(data.BuybackDate)
	hasBuyback := buyback != "" && buyback != "0000-00-00"
	if hasBuyback {
		horizon, err = time.Parse("2006-01-02", buyback)
		if err != nil || !horizon.After(settlement) || horizon.After(maturity) {
			return nil, fmt.Errorf("%w: invalid buyback date", ErrInvalidData)
		}
	}
	var offerPrice *float64
	for _, offer := range schedule.Offers {
		offerDate := strings.TrimSpace(offer.OfferDate)
		if offerDate == "0000-00-00" && hasBuyback {
			continue
		}
		date, err := time.Parse("2006-01-02", offerDate)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid offer date", ErrInvalidData)
		}
		if hasBuyback && date.Equal(horizon) {
			if offerPrice != nil || offer.Price == nil || !validNumber(*offer.Price) || *offer.Price <= 0 {
				return nil, fmt.Errorf("%w: invalid or ambiguous buyback price", ErrInvalidData)
			}
			offerPrice = offer.Price
		}
		if !hasBuyback && date.After(settlement) && date.Before(maturity) {
			return nil, fmt.Errorf("%w: ambiguous offer horizon", ErrInvalidData)
		}
	}
	if hasBuyback && offerPrice == nil {
		return nil, fmt.Errorf("%w: missing buyback price", ErrInvalidData)
	}
	flows := make([]risk.CashFlow, 0, len(schedule.Coupons)+len(schedule.Amortizations)+1)
	var couponDates []time.Time
	seenCoupons := make(map[time.Time]bool)
	for _, coupon := range schedule.Coupons {
		date, err := time.Parse("2006-01-02", coupon.CouponDate)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid coupon date", ErrInvalidData)
		}
		if !date.After(settlement) || date.After(horizon) {
			continue
		}
		if seenCoupons[date] || coupon.Value == nil || !validNumber(*coupon.Value) || *coupon.Value < 0 {
			return nil, fmt.Errorf("%w: missing or duplicate coupon", ErrInvalidData)
		}
		seenCoupons[date] = true
		couponDates = append(couponDates, date)
		flows = append(flows, risk.CashFlow{Date: date, Amount: *coupon.Value})
	}
	if len(couponDates) == 0 {
		return nil, fmt.Errorf("%w: missing future coupons", ErrInvalidData)
	}
	period := time.Duration(*instrument.CouponPeriod+7) * 24 * time.Hour
	for date := settlement; date.Add(period).Before(horizon); {
		var next time.Time
		for _, coupon := range couponDates {
			if coupon.After(date) && (next.IsZero() || coupon.Before(next)) {
				next = coupon
			}
		}
		if next.IsZero() || next.After(date.Add(period)) {
			return nil, fmt.Errorf("%w: incomplete coupon schedule", ErrInvalidData)
		}
		date = next
	}
	remaining := *data.FaceValue
	seenAmortizations := make(map[time.Time]bool)
	for _, amortization := range schedule.Amortizations {
		date, err := time.Parse("2006-01-02", amortization.AmortDate)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid amortization date", ErrInvalidData)
		}
		if date.After(tradeDate) && !date.After(settlement) {
			return nil, fmt.Errorf("%w: amortization before settlement", ErrInvalidData)
		}
		if !date.After(settlement) || date.After(horizon) {
			continue
		}
		if seenAmortizations[date] || amortization.Value == nil || !validNumber(*amortization.Value) || *amortization.Value < 0 {
			return nil, fmt.Errorf("%w: missing or duplicate amortization", ErrInvalidData)
		}
		seenAmortizations[date] = true
		remaining -= *amortization.Value
		flows = append(flows, risk.CashFlow{Date: date, Amount: *amortization.Value})
	}
	if remaining < -0.01 || math.IsNaN(remaining) || math.IsInf(remaining, 0) {
		return nil, fmt.Errorf("%w: amortizations exceed face value", ErrInvalidData)
	}
	if !hasBuyback && remaining > 0.01 {
		return nil, fmt.Errorf("%w: incomplete principal repayment schedule", ErrInvalidData)
	}
	if remaining > 0 {
		if hasBuyback {
			remaining *= *offerPrice / 100
		}
		flows = append(flows, risk.CashFlow{Date: horizon, Amount: remaining})
	}
	return flows, nil
}
