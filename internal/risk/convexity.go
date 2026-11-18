package risk

import (
	"errors"
	"math"
	"time"
)

var ErrInvalidConvexity = errors.New("invalid convexity parameters")

func CountConvexity(settlementDate time.Time, yield float64, cashFlows []CashFlow) (float64, error) {
	year, month, day := settlementDate.Date()
	settlementDay := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	var price, weightedSum float64
	for _, flow := range cashFlows {
		year, month, day := flow.Date.Date()
		paymentDay := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		if !paymentDay.After(settlementDay) {
			continue
		}
		years := float64((paymentDay.Unix()-settlementDay.Unix())/86400) / 365
		discount := math.Pow(1+yield, years)
		presentValue := flow.Amount / discount
		price += presentValue
		weightedSum += years * (years + 1) * presentValue
	}
	if !(price > 0) {
		return 0, ErrInvalidConvexity
	}
	convexity := weightedSum / price / (1 + yield) / (1 + yield)
	if math.IsNaN(convexity) || math.IsInf(convexity, 0) {
		return 0, ErrInvalidConvexity
	}
	return convexity, nil
}
