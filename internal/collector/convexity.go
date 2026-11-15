package collector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/iriski-app/backend/internal/clients/moex"
	"github.com/iriski-app/backend/internal/risk"
)

type settlementCalendar struct {
	days map[string]bool
	err  error
}

type convexityCalculator struct {
	client    Client
	calendars map[int]settlementCalendar
}

func (c *convexityCalculator) settlementDate(ctx context.Context, tradeDate time.Time) (time.Time, error) {
	if tradeDate.Year() < 2023 {
		return time.Time{}, fmt.Errorf("%w: settlement calendar before 2023 is unsupported", ErrInvalidData)
	}
	for date := tradeDate.AddDate(0, 0, 1); date.Before(tradeDate.AddDate(0, 1, 0)); date = date.AddDate(0, 0, 1) {
		calendar, ok := c.calendars[date.Year()]
		if !ok {
			calendar = c.loadCalendar(ctx, date.Year())
			c.calendars[date.Year()] = calendar
		}
		if calendar.err != nil {
			return time.Time{}, calendar.err
		}
		open := date.Weekday() != time.Saturday && date.Weekday() != time.Sunday
		if exception, ok := calendar.days[date.Format("2006-01-02")]; ok {
			open = exception
		}
		if open {
			return date, nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: settlement day not found", ErrInvalidData)
}

func (c *convexityCalculator) loadCalendar(ctx context.Context, year int) settlementCalendar {
	entries, err := c.client.GetSettlementCalendar(ctx, year)
	if err != nil {
		return settlementCalendar{err: fmt.Errorf("get settlement calendar %d: %w", year, err)}
	}
	days := make(map[string]bool)
	for _, currency := range []string{"-", "RUB"} {
		for _, entry := range entries {
			if strings.TrimSpace(entry.CurrencyID) != currency || entry.StockMarket == nil {
				continue
			}
			date, err := time.Parse("2006-01-02", entry.TradeDate)
			if err != nil || date.Year() != year || (*entry.StockMarket != 0 && *entry.StockMarket != 1) {
				return settlementCalendar{err: fmt.Errorf("%w: invalid settlement calendar entry", ErrInvalidData)}
			}
			days[entry.TradeDate] = *entry.StockMarket == 1
		}
	}
	if len(days) == 0 {
		return settlementCalendar{err: fmt.Errorf("%w: no RUB settlement calendar for %d", ErrInvalidData, year)}
	}
	return settlementCalendar{days: days}
}

func (c *convexityCalculator) calculate(ctx context.Context, instrument moex.Instrument, data moex.MarketData, tradeDate time.Time) (float64, error) {
	if data.YieldClose == nil || !validNumber(*data.YieldClose) || *data.YieldClose <= -100 {
		return 0, fmt.Errorf("%w: missing or invalid yield", ErrInvalidData)
	}
	settlement, err := c.settlementDate(ctx, tradeDate)
	if err != nil {
		return 0, err
	}
	schedule, err := c.client.GetBondization(ctx, data.SECID)
	if err != nil {
		return 0, fmt.Errorf("get bondization: %w", err)
	}
	flows, err := buildCashFlows(instrument, data, schedule, settlement)
	if err != nil {
		return 0, err
	}
	return risk.CountConvexity(settlement, *data.YieldClose/100, flows)
}
