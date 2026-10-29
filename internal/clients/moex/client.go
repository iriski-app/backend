package moex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidResponse = errors.New("invalid MOEX response")

type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("MOEX returned HTTP status %d", e.StatusCode)
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	if baseURL == "" {
		baseURL = "https://iss.moex.com/iss"
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) GetInstruments(ctx context.Context, board string) ([]Instrument, error) {
	if board == "" {
		return nil, errors.New("MOEX board is required")
	}
	path := "/engines/stock/markets/bonds/boards/" + url.PathEscape(board) + "/securities.json"
	params := url.Values{
		"iss.meta": {"off"},
		"iss.only": {"securities"},
	}
	var response struct {
		Securities table `json:"securities"`
	}
	err := c.get(ctx, path, params, &response)
	if err != nil {
		return nil, err
	}
	var instruments []Instrument
	err = decodeTable(response.Securities, &instruments, "SECID", "ISIN", "SECNAME", "FACEVALUE", "FACEUNIT")
	if err != nil {
		return nil, err
	}
	return instruments, nil
}

func (c *Client) GetMarketData(
	ctx context.Context,
	board string,
	date time.Time) ([]MarketData, error) {
	if board == "" {
		return nil, errors.New("MOEX board is required")
	}
	if date.IsZero() {
		return nil, errors.New("MOEX trade date is required")
	}
	path := "/history/engines/stock/markets/bonds/boards/" + url.PathEscape(board) + "/securities.json"
	params := url.Values{
		"iss.meta": {"off"},
		"iss.only": {"history,history.cursor"},
		"date":     {date.Format("2006-01-02")},
		"limit":    {"100"},
	}
	marketData := make([]MarketData, 0)
	start := 0
	for {
		params.Set("start", strconv.Itoa(start))
		var response struct {
			History table `json:"history"`
			Cursor  table `json:"history.cursor"`
		}
		err := c.get(ctx, path, params, &response)
		if err != nil {
			return nil, err
		}
		var page []MarketData
		err = decodeTable(response.History, &page, "SECID", "BOARDID", "TRADEDATE", "CLOSE", "ACCINT", "DURATION", "YIELDCLOSE")
		if err != nil {
			return nil, err
		}
		var cursors []cursor
		err = decodeTable(response.Cursor, &cursors, "INDEX", "TOTAL", "PAGESIZE")
		if err != nil {
			return nil, err
		}
		if len(cursors) != 1 {
			return nil, fmt.Errorf("%w: expected one history cursor", ErrInvalidResponse)
		}
		curCursor := cursors[0]
		if curCursor.Index != start || curCursor.Total < start || curCursor.PageSize <= 0 {
			return nil, fmt.Errorf("%w: invalid history cursor", ErrInvalidResponse)
		}
		if len(page) > curCursor.PageSize || start+len(page) > curCursor.Total {
			return nil, fmt.Errorf("%w: history page exceeds cursor bounds", ErrInvalidResponse)
		}
		if len(page) == 0 && start < curCursor.Total {
			return nil, fmt.Errorf("%w: history page is missing", ErrInvalidResponse)
		}
		marketData = append(marketData, page...)
		start += len(page)
		if start == curCursor.Total {
			return marketData, nil
		}
	}
}

func (c *Client) get(ctx context.Context, path string, params url.Values, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return fmt.Errorf("create MOEX request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send MOEX request: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		return &HTTPError{StatusCode: response.StatusCode}
	}
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(result); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidResponse, err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: unexpected data after JSON", ErrInvalidResponse)
	}
	return nil
}

func decodeTable(data table, result any, requiredColumns ...string) error {
	if len(data.Columns) == 0 || data.Data == nil {
		return fmt.Errorf("%w: table is missing", ErrInvalidResponse)
	}
	columns := make(map[string]bool, len(data.Columns))
	for _, column := range data.Columns {
		if column == "" || columns[column] {
			return fmt.Errorf("%w: invalid table columns", ErrInvalidResponse)
		}
		columns[column] = true
	}
	for _, column := range requiredColumns {
		if !columns[column] {
			return fmt.Errorf("%w: missing column %s", ErrInvalidResponse, column)
		}
	}
	rows := make([]map[string]json.RawMessage, len(data.Data))
	for i, row := range data.Data {
		if len(row) != len(data.Columns) {
			return fmt.Errorf("%w: invalid table row length", ErrInvalidResponse)
		}
		rows[i] = make(map[string]json.RawMessage, len(row))
		for j, column := range data.Columns {
			rows[i][column] = row[j]
		}
	}
	jsonRows, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidResponse, err)
	}
	if err := json.Unmarshal(jsonRows, result); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidResponse, err)
	}
	return nil
}
