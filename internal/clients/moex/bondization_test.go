package moex

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetBondization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/securities/SU26238RMFS4/bondization.json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("iss.meta") != "off" || r.URL.Query().Get("limit") != "unlimited" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		writeMOEXResponse(t, w, `{
			"coupons": {"columns": ["value", "coupondate", "startdate", "valueprc", "value_rub"], "data": [[37.4, "2027-05-12", "2026-11-11", 7.5, 37.4], [null, "2027-11-10", null, null, null]]},
			"amortizations": {"columns": ["valueprc", "amortdate", "value", "value_rub"], "data": [[100, "2041-05-15", 1000, 1000]]},
			"offers": {"columns": ["price", "offerdate"], "data": [[null, "2030-01-01"]]}
		}`)
	}))
	defer server.Close()
	client := NewClient(server.URL, time.Second)
	schedule, err := client.GetBondization(context.Background(), " SU26238RMFS4 ")
	if err != nil {
		t.Fatal(err)
	}
	if len(schedule.Coupons) != 2 || len(schedule.Amortizations) != 1 || len(schedule.Offers) != 1 {
		t.Fatalf("unexpected schedule: %+v", schedule)
	}
	if schedule.Coupons[0].CouponDate != "2027-05-12" || schedule.Coupons[0].Value == nil || *schedule.Coupons[0].Value != 37.4 {
		t.Errorf("unexpected coupon: %+v", schedule.Coupons[0])
	}
	if schedule.Coupons[1].Value != nil || schedule.Offers[0].Price != nil {
		t.Error("missing values must remain nil")
	}
	if schedule.Amortizations[0].AmortDate != "2041-05-15" || schedule.Amortizations[0].Value == nil || *schedule.Amortizations[0].Value != 1000 {
		t.Errorf("unexpected amortization: %+v", schedule.Amortizations[0])
	}
}

func TestGetBondizationEmptyAndInvalid(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"empty tables", `{"coupons":{"columns":["coupondate","value"],"data":[]},"amortizations":{"columns":["amortdate","value"],"data":[]},"offers":{"columns":["offerdate","price"],"data":[]}}`, false},
		{"missing table", `{"coupons":{"columns":["coupondate","value"],"data":[]},"amortizations":{"columns":["amortdate","value"],"data":[]}}`, true},
		{"missing column", `{"coupons":{"columns":["coupondate"],"data":[]},"amortizations":{"columns":["amortdate","value"],"data":[]},"offers":{"columns":["offerdate","price"],"data":[]}}`, true},
		{"invalid row", `{"coupons":{"columns":["coupondate","value"],"data":[["2027-01-01"]]},"amortizations":{"columns":["amortdate","value"],"data":[]},"offers":{"columns":["offerdate","price"],"data":[]}}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeMOEXResponse(t, w, tt.body)
			}))
			defer server.Close()
			schedule, err := NewClient(server.URL, time.Second).GetBondization(context.Background(), "BOND")
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidResponse) {
					t.Fatalf("got error %v, want ErrInvalidResponse", err)
				}
				return
			}
			if err != nil || schedule.Coupons == nil || schedule.Amortizations == nil || schedule.Offers == nil {
				t.Fatalf("unexpected schedule: %+v, error: %v", schedule, err)
			}
		})
	}
	if _, err := NewClient("", time.Second).GetBondization(context.Background(), "  "); err == nil {
		t.Fatal("empty SECID must be rejected")
	}
}
