package collector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iriski-app/backend/internal/clients/moex"
)

type calendarClient struct {
	Client
	entries map[int][]moex.SettlementCalendar
	calls   int
}

type retryClient struct {
	Client
	calls int
	err   error
}

func (c *retryClient) GetBondization(ctx context.Context, secID string) (moex.Bondization, error) {
	c.calls++
	return moex.Bondization{}, c.err
}

func TestConvexityRetriesTemporaryErrors(t *testing.T) {
	for _, tt := range []struct {
		name  string
		err   error
		calls int
	}{
		{name: "rate limited", err: &moex.HTTPError{StatusCode: 429}, calls: 3},
		{name: "server error", err: &moex.HTTPError{StatusCode: 503}, calls: 3},
		{name: "invalid request", err: &moex.HTTPError{StatusCode: 400}, calls: 1},
		{name: "invalid data", err: moex.ErrInvalidResponse, calls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &retryClient{err: tt.err}
			calculator := convexityCalculator{client: client}
			data := testMarketData()
			data.YieldClose = testFloat(10)
			_, err := calculator.calculate(context.Background(), testInstrument(), data, collectorTradeDate(), nil)
			if !errors.Is(err, tt.err) || client.calls != tt.calls {
				t.Fatalf("calls %d, error %v", client.calls, err)
			}
		})
	}
}

func (c *calendarClient) GetSettlementCalendar(ctx context.Context, year int) ([]moex.SettlementCalendar, error) {
	c.calls++
	return c.entries[year], nil
}

func TestSettlementDate(t *testing.T) {
	closed, open := 0, 1
	client := &calendarClient{entries: map[int][]moex.SettlementCalendar{
		2026: {{TradeDate: "2026-11-02", CurrencyID: "RUB", StockMarket: &closed}},
		2027: {{TradeDate: "2027-01-01", CurrencyID: "RUB", StockMarket: &open}},
	}}
	calc := convexityCalculator{client: client, calendars: make(map[int]settlementCalendar)}
	for _, tt := range []struct{ trade, want string }{
		{"2026-10-30", "2026-11-03"},
		{"2026-10-29", "2026-10-30"},
		{"2026-12-31", "2027-01-01"},
	} {
		trade, err := time.Parse("2006-01-02", tt.trade)
		if err != nil {
			t.Fatal(err)
		}
		got, err := calc.settlementDate(context.Background(), trade)
		if err != nil || got.Format("2006-01-02") != tt.want {
			t.Fatalf("trade %s: got %s, error %v, want %s", tt.trade, got.Format("2006-01-02"), err, tt.want)
		}
	}
	if client.calls != 2 {
		t.Fatalf("calendar requested %d times, want once per year", client.calls)
	}
}

func TestSettlementDateRejectsMissingCalendar(t *testing.T) {
	calc := convexityCalculator{client: &calendarClient{}, calendars: make(map[int]settlementCalendar)}
	_, err := calc.settlementDate(context.Background(), time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrInvalidData) {
		t.Fatalf("got error %v, want invalid calendar", err)
	}
}

func TestSettlementDateUsesCurrencyCalendar(t *testing.T) {
	closed := 0
	client := &calendarClient{entries: map[int][]moex.SettlementCalendar{2026: {
		{TradeDate: "2026-10-01", CurrencyID: "CNY", StockMarket: &closed},
		{TradeDate: "2026-10-02", CurrencyID: "CNY", StockMarket: &closed},
		{TradeDate: "2026-10-05", CurrencyID: "CNY", StockMarket: &closed},
		{TradeDate: "2026-10-06", CurrencyID: "CNY", StockMarket: &closed},
		{TradeDate: "2026-10-07", CurrencyID: "CNY", StockMarket: &closed},
	}}}
	calc := convexityCalculator{client: client, calendars: make(map[int]settlementCalendar), currency: "CNY"}
	got, err := calc.settlementDate(context.Background(), time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC))
	if err != nil || got.Format("2006-01-02") != "2026-10-08" {
		t.Fatalf("got %s, error %v", got, err)
	}
}

func TestYuanSettlementCycleTransition(t *testing.T) {
	closed := 0
	for _, tc := range []struct {
		name     string
		board    string
		trade    string
		want     string
		holidays []moex.SettlementCalendar
	}{
		{name: "T+2", board: "TQOY", trade: "2023-07-24", want: "2023-07-26"},
		{name: "T+2 across weekend", board: "TQOY", trade: "2023-07-28", want: "2023-08-01"},
		{name: "T+1 on transition", board: "TQOY", trade: "2023-07-31", want: "2023-08-01"},
		{name: "T+2 with holiday", board: "TQOY", trade: "2023-07-24", want: "2023-07-27", holidays: []moex.SettlementCalendar{{TradeDate: "2023-07-25", CurrencyID: "CNY", StockMarket: &closed}}},
		{name: "other board unchanged", board: "TQCB", trade: "2023-07-24", want: "2023-07-25"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := append([]moex.SettlementCalendar{{TradeDate: "2023-01-01", CurrencyID: "CNY", StockMarket: &closed}}, tc.holidays...)
			client := &calendarClient{entries: map[int][]moex.SettlementCalendar{2023: entries}}
			calc := convexityCalculator{client: client, calendars: make(map[int]settlementCalendar), currency: "CNY", board: tc.board}
			trade, err := time.Parse("2006-01-02", tc.trade)
			if err != nil {
				t.Fatal(err)
			}
			got, err := calc.settlementDate(context.Background(), trade)
			if err != nil || got.Format("2006-01-02") != tc.want {
				t.Fatalf("got %s, error %v, want %s", got.Format("2006-01-02"), err, tc.want)
			}
		})
	}
}
