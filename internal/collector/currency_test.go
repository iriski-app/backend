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
