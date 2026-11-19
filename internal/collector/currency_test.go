package collector

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNormalizeYuanBond(t *testing.T) {
	instrument := testInstrument()
	instrument.FaceUnit, instrument.CurrencyID = "CNY", "CNY"
	instrument.BondType = "Валютная облигация"
	instrument.CouponDetails = "Фиксированный с известными купонами"
	gotInstrument, err := NormalizeInstrument(instrument)
	if err != nil || gotInstrument.Currency != "CNY" {
		t.Fatalf("instrument: %+v, %v", gotInstrument, err)
	}
	data := testMarketData()
	data.FaceUnit, data.CurrencyID = "CNY", "CNY"
	data.BondType, data.CouponDetails = instrument.BondType, instrument.CouponDetails
	data.FaceValue, data.Close, data.AccruedInt = testFloat(1000), testFloat(100.0212), testFloat(25.09)
	got, err := NormalizeMarketData(data, uuid.New(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if *got.PriceCurrency != "CNY" || *got.AccruedIntCurrency != "CNY" || math.Abs(*got.PriceClean-1000.212) > 1e-9 || math.Abs(*got.AccruedInt-25.09) > 1e-9 {
		t.Fatalf("unexpected result: %+v", got)
	}
	data.CurrencyID = "RUB"
	if _, err := NormalizeMarketData(data, uuid.New(), time.Now()); !errors.Is(err, ErrUnsupportedCurrency) {
		t.Fatalf("mixed currency error: %v", err)
	}
	instrument.CouponDetails = "Фиксированный с неизвестными купонами"
	if _, err := NormalizeInstrument(instrument); !errors.Is(err, ErrUnsupportedInstrument) {
		t.Fatalf("unknown coupons error: %v", err)
	}
}

func TestNormalizeForeignNominalWithRubleSettlement(t *testing.T) {
	for _, tt := range []struct {
		nominal string
		close   float64
		accrued float64
		price   float64
	}{
		{nominal: "USD", close: 99.198, accrued: 406.7955, price: 991.98},
		{nominal: "EUR", close: 99.4598, accrued: 213.3958, price: 994.598},
	} {
		t.Run(tt.nominal, func(t *testing.T) {
			instrument := testInstrument()
			instrument.BoardID, instrument.FaceUnit, instrument.CurrencyID = "TQCB", tt.nominal, "SUR"
			normalized, err := NormalizeInstrument(instrument)
			if err != nil || normalized.Currency != tt.nominal {
				t.Fatalf("instrument: %+v, %v", normalized, err)
			}
			data := testMarketData()
			data.BoardID, data.FaceUnit, data.CurrencyID = "TQCB", tt.nominal, "SUR"
			data.FaceValue, data.Close, data.AccruedInt = testFloat(1000), &tt.close, &tt.accrued
			got, err := NormalizeMarketData(data, normalized.ID, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if *got.PriceCurrency != tt.nominal || *got.AccruedIntCurrency != "RUB" {
				t.Fatalf("unexpected currencies: %s, %s", *got.PriceCurrency, *got.AccruedIntCurrency)
			}
			assertNumber(t, "price", got.PriceClean, tt.price)
			assertNumber(t, "accrued interest", got.AccruedInt, tt.accrued)
			data.BoardID = "TQOD"
			if _, err := NormalizeMarketData(data, normalized.ID, time.Now()); !errors.Is(err, ErrUnsupportedCurrency) {
				t.Fatalf("unsupported board: %v", err)
			}
		})
	}
}
