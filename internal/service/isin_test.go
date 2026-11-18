package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/iriski-app/backend/internal/storage/models"
)

func TestValidateISIN(t *testing.T) {
	tests := []struct {
		name  string
		isin  string
		valid bool
	}{
		{name: "US prefix", isin: "US0378331005", valid: true},
		{name: "RU prefix", isin: "RU000A0JX0J2", valid: true},
		{name: "GB prefix", isin: "GB0002634946", valid: true},
		{name: "letters in national code", isin: "AU0000XVGZA3", valid: true},
		{name: "empty", isin: ""},
		{name: "too short", isin: "US037833100"},
		{name: "too long", isin: "US03783310055"},
		{name: "lowercase prefix", isin: "us0378331005"},
		{name: "lowercase national code", isin: "RU000a0JX0J2"},
		{name: "digit in prefix", isin: "U10378331005"},
		{name: "letter as check digit", isin: "US037833100A"},
		{name: "punctuation", isin: "US037833-005"},
		{name: "leading space", isin: " US0378331005"},
		{name: "trailing newline", isin: "US0378331005\n"},
		{name: "non ASCII letters", isin: "РU000A0JX0J2"},
		{name: "wrong check digit", isin: "US0378331004"},
		{name: "changed national code", isin: "US0378331015"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateISIN(tt.isin)
			if tt.valid {
				if err != nil {
					t.Fatalf("ValidateISIN(%q) = %v, want nil", tt.isin, err)
				}
			} else if !errors.Is(err, ErrInvalidISIN) {
				t.Fatalf("ValidateISIN(%q) = %v, want ErrInvalidISIN", tt.isin, err)
			}
		})
	}
}

func TestGetInstrumentRejectsInvalidISINBeforeStorage(t *testing.T) {
	for _, isin := range []string{"invalid", "US0378331004"} {
		t.Run(isin, func(t *testing.T) {
			info, err := NewService(&fakeStore{}).GetInstrument(context.Background(), isin, models.SourceMoex)
			if !errors.Is(err, ErrInvalidISIN) {
				t.Fatalf("error = %v, want ErrInvalidISIN", err)
			}
			if !reflect.DeepEqual(info, InstrumentInfo{}) {
				t.Error("invalid ISIN must return an empty result")
			}
		})
	}
}

func TestGetInstrumentAcceptsValidISIN(t *testing.T) {
	wantErr := errors.New("instrument not found in storage")
	calls := 0
	store := &fakeStore{
		GetInstrumentFunc: func(_ context.Context, isin string) (models.Instrument, error) {
			calls++
			if isin != "US0378331005" {
				t.Errorf("ISIN = %q, want US0378331005", isin)
			}
			return models.Instrument{}, wantErr
		},
	}
	_, err := NewService(store).GetInstrument(context.Background(), "US0378331005", models.SourceMoex)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Errorf("GetInstrument called %d times, want 1", calls)
	}
}
