package collector

import (
	"math"
	"testing"
	"time"

	"github.com/iriski-app/backend/internal/clients/moex"
)

func TestBuildCashFlows(t *testing.T) {
	settlement := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	instrument := testInstrument()
	data := testMarketData()
	schedule := moex.Bondization{
		Coupons:       []moex.Coupon{{CouponDate: "2026-11-03", Value: testFloat(40)}, {CouponDate: "2027-02-03", Value: testFloat(40)}},
		Amortizations: []moex.Amortization{{AmortDate: "2027-02-03", Value: testFloat(1000)}},
	}
	flows, err := buildCashFlows(instrument, data, schedule, settlement)
	if err != nil {
		t.Fatal(err)
	}
	var total float64
	for _, flow := range flows {
		total += flow.Amount
	}
	if len(flows) != 3 || total != 1080 {
		t.Fatalf("principal or coupons counted incorrectly: %+v", flows)
	}
}

func TestBuildCashFlowsBuyback(t *testing.T) {
	settlement := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	instrument := testInstrument()
	data := testMarketData()
	data.BuybackDate = "2027-01-03"
	schedule := moex.Bondization{
		Coupons:       []moex.Coupon{{CouponDate: "2027-01-03", Value: testFloat(40)}, {CouponDate: "2027-02-03", Value: testFloat(40)}},
		Amortizations: []moex.Amortization{{AmortDate: "2027-01-03", Value: testFloat(200)}},
		Offers:        []moex.Offer{{OfferDate: "2027-01-03", Price: testFloat(98)}},
	}
	flows, err := buildCashFlows(instrument, data, schedule, settlement)
	if err != nil {
		t.Fatal(err)
	}
	var total float64
	for _, flow := range flows {
		total += flow.Amount
		if flow.Date.After(time.Date(2027, 1, 3, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("payment after buyback: %+v", flow)
		}
	}
	if len(flows) != 3 || math.Abs(total-1024) > 1e-9 {
		t.Fatalf("unexpected buyback payments: %+v", flows)
	}
}

func TestBuildCashFlowsRejectsIncompleteData(t *testing.T) {
	settlement := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		change func(*moex.Instrument, *moex.MarketData, *moex.Bondization)
	}{
		{"missing coupon", func(_ *moex.Instrument, _ *moex.MarketData, s *moex.Bondization) { s.Coupons[0].Value = nil }},
		{"missing coupon schedule", func(_ *moex.Instrument, _ *moex.MarketData, s *moex.Bondization) { s.Coupons = nil }},
		{"coupon schedule ends early", func(i *moex.Instrument, _ *moex.MarketData, _ *moex.Bondization) { i.MaturityDate = "2029-02-03" }},
		{"missing face value", func(_ *moex.Instrument, d *moex.MarketData, _ *moex.Bondization) { d.FaceValue = nil }},
		{"excess principal", func(_ *moex.Instrument, _ *moex.MarketData, s *moex.Bondization) {
			s.Amortizations[0].Value = testFloat(1100)
		}},
		{"missing principal repayment", func(_ *moex.Instrument, _ *moex.MarketData, s *moex.Bondization) {
			s.Amortizations = nil
		}},
		{"missing offer price", func(_ *moex.Instrument, d *moex.MarketData, s *moex.Bondization) {
			d.BuybackDate = "2026-12-03"
			s.Offers = []moex.Offer{{OfferDate: d.BuybackDate}}
		}},
		{"ambiguous offer", func(_ *moex.Instrument, _ *moex.MarketData, s *moex.Bondization) {
			s.Offers = []moex.Offer{{OfferDate: "2026-12-03", Price: testFloat(100)}}
		}},
		{"amortization between trade and settlement", func(_ *moex.Instrument, _ *moex.MarketData, s *moex.Bondization) {
			s.Amortizations = append(s.Amortizations, moex.Amortization{AmortDate: "2026-10-27", Value: testFloat(100)})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instrument, data := testInstrument(), testMarketData()
			schedule := moex.Bondization{Coupons: []moex.Coupon{{CouponDate: "2027-02-03", Value: testFloat(40)}}, Amortizations: []moex.Amortization{{AmortDate: "2027-02-03", Value: testFloat(1000)}}}
			tt.change(&instrument, &data, &schedule)
			if _, err := buildCashFlows(instrument, data, schedule, settlement); err == nil {
				t.Fatal("invalid data was accepted")
			}
		})
	}
}
