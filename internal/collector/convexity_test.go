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
