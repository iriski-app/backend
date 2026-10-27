package moex

import "encoding/json"

type Instrument struct {
	SECID         string   `json:"SECID"`
	ISIN          string   `json:"ISIN"`
	Name          string   `json:"SECNAME"`
	ShortName     string   `json:"SHORTNAME"`
	BoardID       string   `json:"BOARDID"`
	CurrencyID    string   `json:"CURRENCYID"`
	FaceUnit      string   `json:"FACEUNIT"`
	FaceValue     *float64 `json:"FACEVALUE"`
	CouponPeriod  *int     `json:"COUPONPERIOD"`
	CouponPercent *float64 `json:"COUPONPERCENT"`
	MaturityDate  string   `json:"MATDATE"`
	BondType      string   `json:"BONDTYPE"`
	CouponDetails string   `json:"COUPON_DETAILS"`
	FaceValueType string   `json:"FACEVALUE_TYPE"`
}

type MarketData struct {
	SECID          string   `json:"SECID"`
	BoardID        string   `json:"BOARDID"`
	TradeDate      string   `json:"TRADEDATE"`
	Close          *float64 `json:"CLOSE"`
	AccruedInt     *float64 `json:"ACCINT"`
	YieldClose     *float64 `json:"YIELDCLOSE"`
	Duration       *float64 `json:"DURATION"`
	FaceValue      *float64 `json:"FACEVALUE"`
	FaceUnit       string   `json:"FACEUNIT"`
	CurrencyID     string   `json:"CURRENCYID"`
	NumTrades      *int     `json:"NUMTRADES"`
	TradingSession *int     `json:"TRADINGSESSION"`
	CouponPercent  *float64 `json:"COUPONPERCENT"`
	MaturityDate   string   `json:"MATDATE"`
	BuybackDate    string   `json:"BUYBACKDATE"`
	OfferDate      string   `json:"OFFERDATE"`
	BondType       string   `json:"BONDTYPE"`
	CouponDetails  string   `json:"COUPON_DETAILS"`
	FaceValueType  string   `json:"FACEVALUE_TYPE"`
}

type table struct {
	Columns []string            `json:"columns"`
	Data    [][]json.RawMessage `json:"data"`
}

type cursor struct {
	Index    int `json:"INDEX"`
	Total    int `json:"TOTAL"`
	PageSize int `json:"PAGESIZE"`
}
