package cbr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
)

func TestGetRatesLive(t *testing.T) {
	if os.Getenv("TEST_CBR_LIVE") != "1" {
		t.Skip("TEST_CBR_LIVE is not set")
	}
	result, err := NewClient("", 15*time.Second).GetRates(context.Background(), time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.EffectiveDate.After(result.RequestedDate) || len(result.Items) == 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	t.Logf("requested %s, effective %s, currencies %d", result.RequestedDate.Format("2006-01-02"), result.EffectiveDate.Format("2006-01-02"), len(result.Items))
}

func TestGetRates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("date_req") != "15/11/2026" {
			t.Errorf("unexpected date: %s", r.URL.RawQuery)
		}
		body := `<?xml version="1.0" encoding="windows-1251"?><ValCurs Date="14.11.2026"><Valute><CharCode>CNY</CharCode><Nominal>10</Nominal><Name>Юань</Name><Value>125,4321</Value></Valute></ValCurs>`
		encoded, err := charmap.Windows1251.NewEncoder().Bytes([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(encoded)
	}))
	defer server.Close()
	result, err := NewClient(server.URL, time.Second).GetRates(context.Background(), time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.EffectiveDate.Format("2006-01-02") != "2026-11-14" || len(result.Items) != 1 || result.Items[0].Currency != "CNY" || result.Items[0].PerUnit.RatString() != "1254321/100000" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestGetRatesRejectsInvalidResponses(t *testing.T) {
	valid := `<Valute><CharCode>USD</CharCode><Nominal>1</Nominal><Value>90,5</Value></Valute>`
	for _, body := range []string{
		`<ValCurs Date="16.11.2026">` + valid + `</ValCurs>`,
		`<ValCurs Date="15.11.2026"></ValCurs>`,
		`<ValCurs Date="15.11.2026">` + valid + valid + `</ValCurs>`,
		`<ValCurs Date="15.11.2026">` + strings.Replace(valid, "<Nominal>1", "<Nominal>0", 1) + `</ValCurs>`,
		`<ValCurs Date="15.11.2026">` + strings.Replace(valid, "90,5", "-1", 1) + `</ValCurs>`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			_, err := NewClient(server.URL, time.Second).GetRates(context.Background(), time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC))
			if !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("got error %v", err)
			}
		})
	}
}
