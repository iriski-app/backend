package moex

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGetInstruments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/engines/stock/markets/bonds/boards/TQOB/securities.json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("iss.meta") != "off" || r.URL.Query().Get("iss.only") != "securities" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected Accept header: %s", r.Header.Get("Accept"))
		}
		writeMOEXResponse(t, w, `{
			"securities": {
				"columns": ["ISIN", "FACEVALUE", "SECNAME", "SECID", "FACEUNIT", "COUPONPERIOD", "COUPONPERCENT", "MATDATE", "BOARDID", "CURRENCYID", "BONDTYPE", "COUPON_DETAILS", "FACEVALUE_TYPE", "EXTRA"],
				"data": [
					["RU000A0JS3W6", 1000, "ОФЗ-ПД 26207", "SU26207RMFS9", "SUR", 182, 8.15, "2027-02-03", "TQOB", "SUR", "fixed", "known", "fixed", 123],
					["RU000A0JTK38", null, "ОФЗ-ПД 26212", "SU26212RMFS9", "SUR", null, null, null, "TQOB", "SUR", null, null, null, null]
				]
			}
		}`)
	}))
	defer server.Close()
	client := NewClient(server.URL+"/", time.Second)
	instruments, err := client.GetInstruments(context.Background(), "TQOB")
	if err != nil {
		t.Fatal(err)
	}
	faceValue := 1000.0
	couponPeriod := 182
	couponPercent := 8.15
	expected := []Instrument{
		{
			SECID:         "SU26207RMFS9",
			ISIN:          "RU000A0JS3W6",
			Name:          "ОФЗ-ПД 26207",
			BoardID:       "TQOB",
			CurrencyID:    "SUR",
			FaceUnit:      "SUR",
			FaceValue:     &faceValue,
			CouponPeriod:  &couponPeriod,
			CouponPercent: &couponPercent,
			MaturityDate:  "2027-02-03",
			BondType:      "fixed",
			CouponDetails: "known",
			FaceValueType: "fixed",
		},
		{
			SECID:      "SU26212RMFS9",
			ISIN:       "RU000A0JTK38",
			Name:       "ОФЗ-ПД 26212",
			BoardID:    "TQOB",
			CurrencyID: "SUR",
			FaceUnit:   "SUR",
		},
	}
	if !reflect.DeepEqual(instruments, expected) {
		t.Fatalf("got %+v, want %+v", instruments, expected)
	}
}

func TestGetMarketDataPagination(t *testing.T) {
	var starts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/history/engines/stock/markets/bonds/boards/TQCB/securities.json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("date") != "2026-10-26" || query.Get("limit") != "100" || query.Get("iss.meta") != "off" || query.Get("iss.only") != "history,history.cursor" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		start := query.Get("start")
		starts = append(starts, start)
		switch start {
		case "0":
			writeMOEXResponse(t, w, `{
				"history": {
					"columns": ["CLOSE", "SECID", "TRADEDATE", "BOARDID", "ACCINT", "DURATION", "YIELDCLOSE", "FACEVALUE", "FACEUNIT", "CURRENCYID", "NUMTRADES", "TRADINGSESSION"],
					"data": [
						[97.5, "BOND1", "2026-10-26", "TQCB", 0, 730, 15, 1000, "SUR", "SUR", 12, 3],
						[null, "BOND2", "2026-10-26", "TQCB", null, null, null, null, "USD", "SUR", 0, 3]
					]
				},
				"history.cursor": {"columns": ["TOTAL", "PAGESIZE", "INDEX"], "data": [[3, 2, 0]]}
			}`)
		case "2":
			writeMOEXResponse(t, w, `{
				"history": {
					"columns": ["SECID", "BOARDID", "TRADEDATE", "CLOSE", "ACCINT", "DURATION", "YIELDCLOSE"],
					"data": [["BOND3", "TQCB", "2026-10-26", 101, 12.5, 365, 12]]
				},
				"history.cursor": {"columns": ["INDEX", "TOTAL", "PAGESIZE"], "data": [[2, 3, 2]]}
			}`)
		default:
			t.Errorf("unexpected page start: %s", start)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	client := NewClient(server.URL, time.Second)
	date := time.Date(2026, 10, 26, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	marketData, err := client.GetMarketData(context.Background(), "TQCB", date)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(starts, []string{"0", "2"}) {
		t.Fatalf("unexpected pages: %v", starts)
	}
	if len(marketData) != 3 {
		t.Fatalf("got %d rows, want 3", len(marketData))
	}
	first := marketData[0]
	if first.SECID != "BOND1" || first.TradeDate != "2026-10-26" || first.BoardID != "TQCB" || first.FaceUnit != "SUR" || first.CurrencyID != "SUR" {
		t.Fatalf("unexpected first row: %+v", first)
	}
	if first.Close == nil || *first.Close != 97.5 || first.AccruedInt == nil || *first.AccruedInt != 0 || first.Duration == nil || *first.Duration != 730 || first.YieldClose == nil || *first.YieldClose != 15 {
		t.Fatalf("unexpected numeric values: %+v", first)
	}
	if first.FaceValue == nil || *first.FaceValue != 1000 || first.NumTrades == nil || *first.NumTrades != 12 || first.TradingSession == nil || *first.TradingSession != 3 {
		t.Fatalf("unexpected trading values: %+v", first)
	}
	second := marketData[1]
	if second.Close != nil || second.AccruedInt != nil || second.Duration != nil || second.YieldClose != nil || second.FaceValue != nil {
		t.Fatalf("null values were replaced: %+v", second)
	}
	if second.NumTrades == nil || *second.NumTrades != 0 {
		t.Fatalf("zero number of trades was lost: %+v", second)
	}
	if marketData[2].SECID != "BOND3" {
		t.Fatalf("last page is missing: %+v", marketData[2])
	}
}

func TestGetEmptyData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/history/") {
			writeMOEXResponse(t, w, `{
				"history": {"columns": ["SECID", "BOARDID", "TRADEDATE", "CLOSE", "ACCINT", "DURATION", "YIELDCLOSE"], "data": []},
				"history.cursor": {"columns": ["INDEX", "TOTAL", "PAGESIZE"], "data": [[0, 0, 100]]}
			}`)
			return
		}
		writeMOEXResponse(t, w, `{"securities": {"columns": ["SECID", "ISIN", "SECNAME", "FACEVALUE", "FACEUNIT"], "data": []}}`)
	}))
	defer server.Close()
	client := NewClient(server.URL, time.Second)
	instruments, err := client.GetInstruments(context.Background(), "TQOB")
	if err != nil || instruments == nil || len(instruments) != 0 {
		t.Fatalf("unexpected instruments: %v, error: %v", instruments, err)
	}
	marketData, err := client.GetMarketData(context.Background(), "TQOB", time.Now())
	if err != nil || marketData == nil || len(marketData) != 0 {
		t.Fatalf("unexpected market data: %v, error: %v", marketData, err)
	}
}

func TestClientHTTPError(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				writeMOEXResponse(t, w, "private response body")
			}))
			defer server.Close()
			client := NewClient(server.URL, time.Second)
			_, err := client.GetInstruments(context.Background(), "TQOB")
			var httpErr *HTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != status {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Contains(err.Error(), "private response body") {
				t.Fatalf("response body is included in error: %v", err)
			}
		})
	}
}

func TestGetMarketDataPageError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("start") != "0" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeMOEXResponse(t, w, `{
			"history": {
				"columns": ["SECID", "BOARDID", "TRADEDATE", "CLOSE", "ACCINT", "DURATION", "YIELDCLOSE"],
				"data": [["BOND1", "TQOB", "2026-10-26", 100, 10, 365, 12]]
			},
			"history.cursor": {"columns": ["INDEX", "TOTAL", "PAGESIZE"], "data": [[0, 2, 1]]}
		}`)
	}))
	defer server.Close()
	client := NewClient(server.URL, time.Second)
	marketData, err := client.GetMarketData(context.Background(), "TQOB", time.Now())
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected error: %v", err)
	}
	if marketData != nil {
		t.Fatalf("partial result was returned: %+v", marketData)
	}
}

func TestGetInstrumentsInvalidResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: "{"},
		{name: "missing table", body: "{}"},
		{name: "missing data", body: `{"securities": {"columns": ["SECID"]}}`},
		{name: "missing columns", body: `{"securities": {"data": []}}`},
		{name: "missing required column", body: `{"securities": {"columns": ["SECID"], "data": []}}`},
		{name: "duplicate columns", body: `{"securities": {"columns": ["SECID", "SECID"], "data": []}}`},
		{name: "empty column", body: `{"securities": {"columns": [""], "data": []}}`},
		{name: "short row", body: `{"securities": {"columns": ["SECID", "ISIN", "SECNAME", "FACEVALUE", "FACEUNIT"], "data": [["BOND1"]]}}`},
		{name: "long row", body: `{"securities": {"columns": ["SECID", "ISIN", "SECNAME", "FACEVALUE", "FACEUNIT"], "data": [["BOND1", "ISIN", "Bond", 1000, "SUR", 1]]}}`},
		{name: "invalid number", body: `{"securities": {"columns": ["SECID", "ISIN", "SECNAME", "FACEVALUE", "FACEUNIT"], "data": [["BOND1", "ISIN", "Bond", "wrong", "SUR"]]}}`},
		{name: "extra JSON", body: `{"securities": {"columns": ["SECID", "ISIN", "SECNAME", "FACEVALUE", "FACEUNIT"], "data": []}} {}`},
		{name: "trailing text", body: `{"securities": {"columns": ["SECID", "ISIN", "SECNAME", "FACEVALUE", "FACEUNIT"], "data": []}} wrong`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeMOEXResponse(t, w, tt.body)
			}))
			defer server.Close()
			client := NewClient(server.URL, time.Second)
			instruments, err := client.GetInstruments(context.Background(), "TQOB")
			if !errors.Is(err, ErrInvalidResponse) || instruments != nil {
				t.Fatalf("unexpected instruments: %v, error: %v", instruments, err)
			}
		})
	}
}

func TestGetMarketDataInvalidResponse(t *testing.T) {
	tests := []struct {
		name    string
		history string
		cursor  string
	}{
		{name: "missing history", history: `{}`, cursor: `{"columns": ["INDEX", "TOTAL", "PAGESIZE"], "data": [[0, 0, 100]]}`},
		{name: "missing cursor", history: `{"columns": ["SECID", "BOARDID", "TRADEDATE", "CLOSE", "ACCINT", "DURATION", "YIELDCLOSE"], "data": []}`, cursor: `{}`},
		{name: "empty cursor", cursor: `{"columns": ["INDEX", "TOTAL", "PAGESIZE"], "data": []}`},
		{name: "multiple cursors", cursor: `{"columns": ["INDEX", "TOTAL", "PAGESIZE"], "data": [[0, 1, 1], [0, 1, 1]]}`},
		{name: "wrong index", cursor: `{"columns": ["INDEX", "TOTAL", "PAGESIZE"], "data": [[1, 2, 1]]}`},
		{name: "negative total", cursor: `{"columns": ["INDEX", "TOTAL", "PAGESIZE"], "data": [[0, -1, 1]]}`},
		{name: "zero page size", cursor: `{"columns": ["INDEX", "TOTAL", "PAGESIZE"], "data": [[0, 1, 0]]}`},
		{name: "page exceeds total", cursor: `{"columns": ["INDEX", "TOTAL", "PAGESIZE"], "data": [[0, 0, 1]]}`},
		{name: "page exceeds size", history: `{"columns": ["SECID", "BOARDID", "TRADEDATE", "CLOSE", "ACCINT", "DURATION", "YIELDCLOSE"], "data": [["BOND1", "TQOB", "2026-10-26", 100, 0, 365, 12], ["BOND2", "TQOB", "2026-10-26", 99, 0, 730, 13]]}`, cursor: `{"columns": ["INDEX", "TOTAL", "PAGESIZE"], "data": [[0, 2, 1]]}`},
		{name: "empty page before end", history: `{"columns": ["SECID", "BOARDID", "TRADEDATE", "CLOSE", "ACCINT", "DURATION", "YIELDCLOSE"], "data": []}`, cursor: `{"columns": ["INDEX", "TOTAL", "PAGESIZE"], "data": [[0, 1, 100]]}`},
		{name: "invalid close", history: `{"columns": ["SECID", "BOARDID", "TRADEDATE", "CLOSE", "ACCINT", "DURATION", "YIELDCLOSE"], "data": [["BOND1", "TQOB", "2026-10-26", "wrong", 0, 365, 12]]}`, cursor: `{"columns": ["INDEX", "TOTAL", "PAGESIZE"], "data": [[0, 1, 1]]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := tt.history
			if history == "" {
				history = `{"columns": ["SECID", "BOARDID", "TRADEDATE", "CLOSE", "ACCINT", "DURATION", "YIELDCLOSE"], "data": [["BOND1", "TQOB", "2026-10-26", 100, 0, 365, 12]]}`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeMOEXResponse(t, w, `{"history": `+history+`, "history.cursor": `+tt.cursor+`}`)
			}))
			defer server.Close()
			client := NewClient(server.URL, time.Second)
			marketData, err := client.GetMarketData(context.Background(), "TQOB", time.Now())
			if !errors.Is(err, ErrInvalidResponse) || marketData != nil {
				t.Fatalf("unexpected market data: %v, error: %v", marketData, err)
			}
		})
	}
}

func TestClientCancellation(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.GetInstruments(ctx, "TQOB")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected instruments error: %v", err)
	}
	_, err = client.GetMarketData(ctx, "TQOB", time.Now())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected market data error: %v", err)
	}
}

func TestClientTimeout(t *testing.T) {
	for _, flushHeaders := range []bool{false, true} {
		t.Run(strconv.FormatBool(flushHeaders), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if flushHeaders {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			client := NewClient(server.URL, 100*time.Millisecond)
			_, err := client.GetInstruments(context.Background(), "TQOB")
			var netErr net.Error
			if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &netErr) || !netErr.Timeout() {
				t.Fatalf("unexpected timeout error: %v", err)
			}
		})
	}
}

func TestClientInvalidInput(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", time.Second)
	_, err := client.GetInstruments(context.Background(), "")
	var netErr net.Error
	if err == nil || errors.As(err, &netErr) {
		t.Fatalf("unexpected instruments error: %v", err)
	}
	_, err = client.GetMarketData(context.Background(), "", time.Now())
	if err == nil || errors.As(err, &netErr) {
		t.Fatalf("unexpected board error: %v", err)
	}
	_, err = client.GetMarketData(context.Background(), "TQOB", time.Time{})
	if err == nil || errors.As(err, &netErr) {
		t.Fatalf("unexpected date error: %v", err)
	}
}

func writeMOEXResponse(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write response: %v", err)
	}
}
