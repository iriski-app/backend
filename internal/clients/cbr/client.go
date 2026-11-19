package cbr

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
)

var ErrInvalidResponse = errors.New("invalid CBR response")

type Rate struct {
	Currency string
	Nominal  int
	Value    *big.Rat
	PerUnit  *big.Rat
}

type Rates struct {
	RequestedDate time.Time
	EffectiveDate time.Time
	Items         []Rate
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	if baseURL == "" {
		baseURL = "https://www.cbr.ru/scripts/XML_daily.asp"
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{baseURL: baseURL, httpClient: &http.Client{Timeout: timeout}}
}

func (c *Client) GetRates(ctx context.Context, date time.Time) (Rates, error) {
	if date.IsZero() {
		return Rates{}, errors.New("date is required")
	}
	requested := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return Rates{}, fmt.Errorf("parse CBR URL: %w", err)
	}
	query := u.Query()
	query.Set("date_req", requested.Format("02/01/2006"))
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Rates{}, fmt.Errorf("create CBR request: %w", err)
	}
	req.Header.Set("User-Agent", "iriski-risk-collector/1.0")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Rates{}, fmt.Errorf("request CBR rates: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Rates{}, fmt.Errorf("CBR returned HTTP status %d", resp.StatusCode)
	}
	var payload struct {
		XMLName xml.Name `xml:"ValCurs"`
		Date    string   `xml:"Date,attr"`
		Items   []struct {
			Currency string `xml:"CharCode"`
			Nominal  int    `xml:"Nominal"`
			Value    string `xml:"Value"`
		} `xml:"Valute"`
	}
	decoder := xml.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	decoder.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		if strings.EqualFold(charset, "windows-1251") {
			return charmap.Windows1251.NewDecoder().Reader(input), nil
		}
		return nil, fmt.Errorf("unsupported XML charset %q", charset)
	}
	if err := decoder.Decode(&payload); err != nil {
		return Rates{}, fmt.Errorf("%w: decode XML: %v", ErrInvalidResponse, err)
	}
	effective, err := time.Parse("02.01.2006", payload.Date)
	if err != nil || effective.After(requested) || len(payload.Items) == 0 {
		return Rates{}, fmt.Errorf("%w: invalid date or empty rates", ErrInvalidResponse)
	}
	result := Rates{RequestedDate: requested, EffectiveDate: effective, Items: make([]Rate, 0, len(payload.Items))}
	seen := make(map[string]bool, len(payload.Items))
	for _, item := range payload.Items {
		currency := strings.TrimSpace(item.Currency)
		value, ok := new(big.Rat).SetString(strings.ReplaceAll(strings.TrimSpace(item.Value), ",", "."))
		if len(currency) != 3 || currency[0] < 'A' || currency[0] > 'Z' || currency[1] < 'A' || currency[1] > 'Z' || currency[2] < 'A' || currency[2] > 'Z' || seen[currency] || item.Nominal <= 0 || !ok || value.Sign() <= 0 {
			return Rates{}, fmt.Errorf("%w: invalid rate for %q", ErrInvalidResponse, currency)
		}
		seen[currency] = true
		result.Items = append(result.Items, Rate{Currency: currency, Nominal: item.Nominal, Value: value, PerUnit: new(big.Rat).Quo(value, big.NewRat(int64(item.Nominal), 1))})
	}
	return result, nil
}
