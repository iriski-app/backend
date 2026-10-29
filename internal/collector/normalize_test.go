package collector

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/clients/moex"
	"github.com/iriski-app/backend/internal/storage/models"
)

func TestNormalizeInstrument(t *testing.T) {
	data := testInstrument()
	instrument, err := NormalizeInstrument(data)
	if err != nil {
		t.Fatal(err)
	}
	if instrument.ID == uuid.Nil {
		t.Fatal("instrument ID is empty")
	}
	maturityDate := time.Date(2027, 2, 3, 0, 0, 0, 0, time.UTC)
	expected := models.Instrument{
		ID:             instrument.ID,
		ISIN:           "RU000A0JS3W6",
		Name:           "ОФЗ-ПД 26207",
		InstrumentType: "bond",
		Currency:       "RUB",
		FaceValue:      1000,
		CouponType:     "fixed",
		CouponFreq:     2,
		CouponRate:     testFloat(8.15),
		MaturityDate:   &maturityDate,
	}
	if !reflect.DeepEqual(instrument, expected) {
		t.Fatalf("got %+v, want %+v", instrument, expected)
	}
	if !reflect.DeepEqual(data, testInstrument()) {
		t.Fatal("source instrument was changed")
	}
}

func TestNormalizeInstrumentInvalidData(t *testing.T) {
	tests := []struct {
		name   string
		change func(*moex.Instrument)
		want   error
	}{
		{name: "missing SECID", change: func(data *moex.Instrument) { data.SECID = "" }, want: ErrInvalidData},
		{name: "missing ISIN", change: func(data *moex.Instrument) { data.ISIN = "" }, want: ErrInvalidData},
		{name: "invalid ISIN format", change: func(data *moex.Instrument) { data.ISIN = "invalid" }, want: ErrInvalidData},
		{name: "missing names", change: func(data *moex.Instrument) { data.Name = ""; data.ShortName = "" }, want: ErrInvalidData},
		{name: "missing nominal", change: func(data *moex.Instrument) { data.FaceValue = nil }, want: ErrInvalidData},
		{name: "zero nominal", change: func(data *moex.Instrument) { data.FaceValue = testFloat(0) }, want: ErrInvalidData},
		{name: "negative nominal", change: func(data *moex.Instrument) { data.FaceValue = testFloat(-1000) }, want: ErrInvalidData},
		{name: "nominal NaN", change: func(data *moex.Instrument) { data.FaceValue = testFloat(math.NaN()) }, want: ErrInvalidData},
		{name: "nominal infinity", change: func(data *moex.Instrument) { data.FaceValue = testFloat(math.Inf(1)) }, want: ErrInvalidData},
		{name: "foreign nominal", change: func(data *moex.Instrument) { data.FaceUnit = "USD" }, want: ErrUnsupportedCurrency},
		{name: "foreign settlement currency", change: func(data *moex.Instrument) { data.CurrencyID = "USD" }, want: ErrUnsupportedCurrency},
		{name: "missing nominal currency", change: func(data *moex.Instrument) { data.FaceUnit = "" }, want: ErrInvalidData},
		{name: "missing settlement currency", change: func(data *moex.Instrument) { data.CurrencyID = "" }, want: ErrInvalidData},
		{name: "floating coupon", change: func(data *moex.Instrument) { data.BondType = "Облигация с плавающим купоном" }, want: ErrUnsupportedInstrument},
		{name: "unknown future coupons", change: func(data *moex.Instrument) {
			data.CouponDetails = "Фиксированный с неизвестными купонами"
		}, want: ErrUnsupportedInstrument},
		{name: "structured bond", change: func(data *moex.Instrument) { data.BondType = "Структурная облигация" }, want: ErrUnsupportedInstrument},
		{name: "amortizing bond", change: func(data *moex.Instrument) { data.BondType = "Амортизируемая облигация" }, want: ErrUnsupportedInstrument},
		{name: "indexed nominal", change: func(data *moex.Instrument) { data.FaceValueType = "Индексированный" }, want: ErrUnsupportedInstrument},
		{name: "unknown bond type", change: func(data *moex.Instrument) { data.BondType = "" }, want: ErrUnsupportedInstrument},
		{name: "unknown nominal type", change: func(data *moex.Instrument) { data.FaceValueType = "" }, want: ErrUnsupportedInstrument},
		{name: "invalid maturity date", change: func(data *moex.Instrument) { data.MaturityDate = "2027-02-30" }, want: ErrInvalidData},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := testInstrument()
			tt.change(&data)
			instrument, err := NormalizeInstrument(data)
			if !errors.Is(err, tt.want) {
				t.Fatalf("got error %v, want %v", err, tt.want)
			}
			if !reflect.DeepEqual(instrument, models.Instrument{}) {
				t.Fatalf("partial instrument was returned: %+v", instrument)
			}
		})
	}
}

func TestNormalizeInstrumentOptionalData(t *testing.T) {
	data := testInstrument()
	data.ISIN = " RU000A0JS3W6 "
	data.Name = " "
	data.ShortName = " ОФЗ 26207 "
	data.FaceUnit = " rub "
	data.CurrencyID = "RUB"
	data.CouponDetails = ""
	data.CouponPeriod = nil
	data.CouponPercent = nil
	data.MaturityDate = "0000-00-00"
	instrument, err := NormalizeInstrument(data)
	if err != nil {
		t.Fatal(err)
	}
	if instrument.ISIN != "RU000A0JS3W6" || instrument.Name != "ОФЗ 26207" || instrument.Currency != "RUB" {
		t.Fatalf("unexpected instrument: %+v", instrument)
	}
	if instrument.CouponFreq != 0 || instrument.CouponRate != nil || instrument.MaturityDate != nil {
		t.Fatalf("missing optional data was replaced: %+v", instrument)
	}
}

func TestNormalizeInstrumentCoupon(t *testing.T) {
	tests := []struct {
		name        string
		period      *int
		rate        *float64
		wantFreq    int
		wantNilRate bool
	}{
		{name: "monthly", period: testInt(30), rate: testFloat(12), wantFreq: 12},
		{name: "quarterly", period: testInt(91), rate: testFloat(12), wantFreq: 4},
		{name: "annual", period: testInt(366), rate: testFloat(12), wantFreq: 1},
		{name: "zero coupon", period: testInt(182), rate: testFloat(0), wantFreq: 2},
		{name: "missing period", period: nil, rate: testFloat(12), wantFreq: 0},
		{name: "zero period", period: testInt(0), rate: testFloat(12), wantFreq: 0},
		{name: "negative period", period: testInt(-182), rate: testFloat(12), wantFreq: 0},
		{name: "irregular period", period: testInt(730), rate: testFloat(12), wantFreq: 0},
		{name: "missing rate", period: testInt(182), rate: nil, wantFreq: 2, wantNilRate: true},
		{name: "negative rate", period: testInt(182), rate: testFloat(-1), wantFreq: 2, wantNilRate: true},
		{name: "rate NaN", period: testInt(182), rate: testFloat(math.NaN()), wantFreq: 2, wantNilRate: true},
		{name: "rate infinity", period: testInt(182), rate: testFloat(math.Inf(1)), wantFreq: 2, wantNilRate: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := testInstrument()
			data.CouponPeriod = tt.period
			data.CouponPercent = tt.rate
			instrument, err := NormalizeInstrument(data)
			if err != nil {
				t.Fatal(err)
			}
			if instrument.CouponFreq != tt.wantFreq {
				t.Fatalf("frequency %d, want %d", instrument.CouponFreq, tt.wantFreq)
			}
			if tt.wantNilRate {
				if instrument.CouponRate != nil {
					t.Fatalf("unexpected rate: %v", *instrument.CouponRate)
				}
			} else if instrument.CouponRate == nil || *instrument.CouponRate != *tt.rate {
				t.Fatalf("unexpected coupon rate: %v", instrument.CouponRate)
			}
		})
	}
}

func TestNormalizeMarketData(t *testing.T) {
	data := testMarketData()
	instrumentID := uuid.New()
	loadedAt := time.Date(2026, 10, 27, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	marketData, err := NormalizeMarketData(data, instrumentID, loadedAt)
	if err != nil {
		t.Fatal(err)
	}
	if marketData.InstrumentID != instrumentID || marketData.Source != models.SourceMoex || marketData.PriceType != "trade" {
		t.Fatalf("unexpected identity or source: %+v", marketData)
	}
	wantDate := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	if !marketData.TradeDate.Equal(wantDate) || !marketData.LoadedAt.Equal(loadedAt) || marketData.LoadedAt.Location() != time.UTC {
		t.Fatalf("unexpected dates: %+v", marketData)
	}
	assertNumber(t, "price", marketData.PriceClean, 986.4)
	assertNumber(t, "accrued interest", marketData.AccruedInt, 18.76)
	assertNumber(t, "duration", marketData.DurationMod, 1.6)
	assertNumber(t, "yield percent", marketData.YTM, 25)
	if marketData.NumTrades == nil || *marketData.NumTrades != 12 || marketData.Convexity != nil {
		t.Fatalf("unexpected trades or convexity: %+v", marketData)
	}
	if !reflect.DeepEqual(data, testMarketData()) {
		t.Fatal("source market data was changed")
	}
}

func TestNormalizeMarketDataHistoricalNominal(t *testing.T) {
	data := testMarketData()
	data.Close = testFloat(97.5)
	data.FaceValue = testFloat(800)
	data.AccruedInt = testFloat(0)
	marketData, err := NormalizeMarketData(data, uuid.New(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	assertNumber(t, "price", marketData.PriceClean, 780)
	assertNumber(t, "zero accrued interest", marketData.AccruedInt, 0)
}

func TestNormalizeMarketDataMissingValues(t *testing.T) {
	tests := []struct {
		name   string
		change func(*moex.MarketData)
		field  string
	}{
		{name: "missing close", change: func(data *moex.MarketData) { data.Close = nil }, field: "price"},
		{name: "zero close", change: func(data *moex.MarketData) { data.Close = testFloat(0) }, field: "price"},
		{name: "negative close", change: func(data *moex.MarketData) { data.Close = testFloat(-1) }, field: "price"},
		{name: "close NaN", change: func(data *moex.MarketData) { data.Close = testFloat(math.NaN()) }, field: "price"},
		{name: "close infinity", change: func(data *moex.MarketData) { data.Close = testFloat(math.Inf(1)) }, field: "price"},
		{name: "missing nominal", change: func(data *moex.MarketData) { data.FaceValue = nil }, field: "price"},
		{name: "zero nominal", change: func(data *moex.MarketData) { data.FaceValue = testFloat(0) }, field: "price"},
		{name: "negative nominal", change: func(data *moex.MarketData) { data.FaceValue = testFloat(-1) }, field: "price"},
		{name: "nominal NaN", change: func(data *moex.MarketData) { data.FaceValue = testFloat(math.NaN()) }, field: "price"},
		{name: "price overflow", change: func(data *moex.MarketData) {
			data.Close = testFloat(math.MaxFloat64)
			data.FaceValue = testFloat(math.MaxFloat64)
		}, field: "price"},
		{name: "missing accrued interest", change: func(data *moex.MarketData) { data.AccruedInt = nil }, field: "accrued interest"},
		{name: "negative accrued interest", change: func(data *moex.MarketData) { data.AccruedInt = testFloat(-1) }, field: "accrued interest"},
		{name: "accrued interest NaN", change: func(data *moex.MarketData) { data.AccruedInt = testFloat(math.NaN()) }, field: "accrued interest"},
		{name: "accrued interest infinity", change: func(data *moex.MarketData) { data.AccruedInt = testFloat(math.Inf(1)) }, field: "accrued interest"},
		{name: "missing duration", change: func(data *moex.MarketData) { data.Duration = nil }, field: "duration"},
		{name: "zero duration", change: func(data *moex.MarketData) { data.Duration = testFloat(0) }, field: "duration"},
		{name: "negative duration", change: func(data *moex.MarketData) { data.Duration = testFloat(-1) }, field: "duration"},
		{name: "duration NaN", change: func(data *moex.MarketData) { data.Duration = testFloat(math.NaN()) }, field: "duration"},
		{name: "duration infinity", change: func(data *moex.MarketData) { data.Duration = testFloat(math.Inf(1)) }, field: "duration"},
		{name: "missing yield", change: func(data *moex.MarketData) { data.YieldClose = nil }, field: "yield"},
		{name: "invalid yield", change: func(data *moex.MarketData) { data.YieldClose = testFloat(-100) }, field: "yield"},
		{name: "yield NaN", change: func(data *moex.MarketData) { data.YieldClose = testFloat(math.NaN()) }, field: "yield"},
		{name: "yield infinity", change: func(data *moex.MarketData) { data.YieldClose = testFloat(math.Inf(1)) }, field: "yield"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := testMarketData()
			tt.change(&data)
			marketData, err := NormalizeMarketData(data, uuid.New(), time.Now())
			if err != nil {
				t.Fatal(err)
			}
			switch tt.field {
			case "price":
				if marketData.PriceClean != nil {
					t.Fatalf("unexpected price: %v", *marketData.PriceClean)
				}
			case "accrued interest":
				if marketData.AccruedInt != nil {
					t.Fatalf("unexpected accrued interest: %v", *marketData.AccruedInt)
				}
			case "duration":
				if marketData.DurationMod != nil {
					t.Fatalf("unexpected duration: %v", *marketData.DurationMod)
				}
			case "yield":
				if marketData.YTM != nil || marketData.DurationMod != nil {
					t.Fatalf("unexpected yield or duration: %+v", marketData)
				}
			}
			if tt.field != "price" {
				assertNumber(t, "preserved price", marketData.PriceClean, 986.4)
			}
			if tt.field != "accrued interest" {
				assertNumber(t, "preserved accrued interest", marketData.AccruedInt, 18.76)
			}
			if tt.field != "duration" && tt.field != "yield" {
				assertNumber(t, "preserved duration", marketData.DurationMod, 1.6)
			}
		})
	}
}

func TestNormalizeMarketDataInvalidData(t *testing.T) {
	tests := []struct {
		name   string
		change func(*moex.MarketData)
		want   error
	}{
		{name: "missing SECID", change: func(data *moex.MarketData) { data.SECID = "" }, want: ErrInvalidData},
		{name: "missing board", change: func(data *moex.MarketData) { data.BoardID = "" }, want: ErrInvalidData},
		{name: "missing date", change: func(data *moex.MarketData) { data.TradeDate = "" }, want: ErrInvalidData},
		{name: "invalid date", change: func(data *moex.MarketData) { data.TradeDate = "2026-02-30" }, want: ErrInvalidData},
		{name: "unknown date", change: func(data *moex.MarketData) { data.TradeDate = "0000-00-00" }, want: ErrInvalidData},
		{name: "foreign nominal", change: func(data *moex.MarketData) { data.FaceUnit = "USD" }, want: ErrUnsupportedCurrency},
		{name: "foreign settlement currency", change: func(data *moex.MarketData) { data.CurrencyID = "USD" }, want: ErrUnsupportedCurrency},
		{name: "missing currency", change: func(data *moex.MarketData) { data.FaceUnit = "" }, want: ErrInvalidData},
		{name: "floating coupon", change: func(data *moex.MarketData) { data.BondType = "Облигация с плавающим купоном" }, want: ErrUnsupportedInstrument},
		{name: "structured bond", change: func(data *moex.MarketData) { data.BondType = "Структурная облигация" }, want: ErrUnsupportedInstrument},
		{name: "unknown future coupons", change: func(data *moex.MarketData) {
			data.CouponDetails = "Фиксированный с неизвестными купонами"
		}, want: ErrUnsupportedInstrument},
		{name: "indexed nominal", change: func(data *moex.MarketData) { data.FaceValueType = "Индексированный" }, want: ErrUnsupportedInstrument},
		{name: "missing number of trades", change: func(data *moex.MarketData) { data.NumTrades = nil }, want: ErrInvalidData},
		{name: "negative number of trades", change: func(data *moex.MarketData) { data.NumTrades = testInt(-1) }, want: ErrInvalidData},
		{name: "no trades", change: func(data *moex.MarketData) { data.NumTrades = testInt(0) }, want: ErrNoTrades},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := testMarketData()
			tt.change(&data)
			marketData, err := NormalizeMarketData(data, uuid.New(), time.Now())
			if !errors.Is(err, tt.want) {
				t.Fatalf("got error %v, want %v", err, tt.want)
			}
			if !reflect.DeepEqual(marketData, models.MarketData{}) {
				t.Fatalf("partial market data was returned: %+v", marketData)
			}
		})
	}
	_, err := NormalizeMarketData(testMarketData(), uuid.Nil, time.Now())
	if !errors.Is(err, ErrInvalidData) {
		t.Fatalf("unexpected empty instrument ID error: %v", err)
	}
	_, err = NormalizeMarketData(testMarketData(), uuid.New(), time.Time{})
	if !errors.Is(err, ErrInvalidData) {
		t.Fatalf("unexpected empty load time error: %v", err)
	}
}

func TestNormalizedValuesDoNotSharePointers(t *testing.T) {
	data := testInstrument()
	instrument, err := NormalizeInstrument(data)
	if err != nil {
		t.Fatal(err)
	}
	*data.CouponPercent = 99
	assertNumber(t, "coupon rate", instrument.CouponRate, 8.15)
	marketSource := testMarketData()
	marketData, err := NormalizeMarketData(marketSource, instrument.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	*marketSource.AccruedInt = 99
	*marketSource.YieldClose = 99
	*marketSource.NumTrades = 99
	assertNumber(t, "accrued interest", marketData.AccruedInt, 18.76)
	assertNumber(t, "yield", marketData.YTM, 25)
	if *marketData.NumTrades != 12 {
		t.Fatalf("number of trades was changed: %d", *marketData.NumTrades)
	}
}

func testInstrument() moex.Instrument {
	return moex.Instrument{
		SECID:         "SU26207RMFS9",
		ISIN:          "RU000A0JS3W6",
		Name:          "ОФЗ-ПД 26207",
		ShortName:     "ОФЗ 26207",
		BoardID:       "TQOB",
		FaceUnit:      "SUR",
		CurrencyID:    "SUR",
		FaceValue:     testFloat(1000),
		CouponPeriod:  testInt(182),
		CouponPercent: testFloat(8.15),
		MaturityDate:  "2027-02-03",
		BondType:      "Облигация с фиксированным (известным) купоном",
		CouponDetails: "Фиксированный с известными купонами",
		FaceValueType: "Фиксированный",
	}
}

func testMarketData() moex.MarketData {
	return moex.MarketData{
		SECID:         "SU26207RMFS9",
		BoardID:       "TQOB",
		TradeDate:     "2026-10-26",
		Close:         testFloat(98.64),
		AccruedInt:    testFloat(18.76),
		Duration:      testFloat(730),
		YieldClose:    testFloat(25),
		FaceValue:     testFloat(1000),
		FaceUnit:      "SUR",
		CurrencyID:    "SUR",
		NumTrades:     testInt(12),
		BondType:      "Облигация с фиксированным (известным) купоном",
		CouponDetails: "Фиксированный с известными купонами",
		FaceValueType: "Фиксированный",
	}
}

func testFloat(value float64) *float64 {
	return &value
}

func testInt(value int) *int {
	return &value
}

func assertNumber(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is missing", name)
	}
	if !validNumber(*got) || math.Abs(*got-want) > 1e-9 {
		t.Fatalf("%s = %.12f, want %.12f", name, *got, want)
	}
}
