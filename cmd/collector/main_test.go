package main

import (
	"errors"
	"flag"
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want time.Time
	}{
		{
			name: "separate value",
			args: []string{"--date", "2026-10-26"},
			want: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "equals value",
			args: []string{"--date=2026-10-27"},
			want: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "leap day",
			args: []string{"--date", "2024-02-29"},
			want: time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			date, err := parseDate(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if !date.Equal(tt.want) || date.Location() != time.UTC {
				t.Fatalf("got date %v, want %v", date, tt.want)
			}
		})
	}
}

func TestParseDateInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing date"},
		{name: "missing value", args: []string{"--date"}},
		{name: "empty value", args: []string{"--date="}},
		{name: "invalid format", args: []string{"--date", "26.10.2026"}},
		{name: "short month", args: []string{"--date", "2026-1-26"}},
		{name: "invalid day", args: []string{"--date", "2026-02-30"}},
		{name: "non leap day", args: []string{"--date", "2026-02-29"}},
		{name: "timestamp", args: []string{"--date", "2026-10-26T12:00:00Z"}},
		{name: "zero date", args: []string{"--date", "0001-01-01"}},
		{name: "zero year", args: []string{"--date", "0000-01-01"}},
		{name: "unknown flag", args: []string{"--unknown"}},
		{name: "positional date", args: []string{"2026-10-26"}},
		{name: "extra argument", args: []string{"--date", "2026-10-26", "extra"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			date, err := parseDate(tt.args)
			if err == nil || !date.IsZero() {
				t.Fatalf("got date %v, error %v, want invalid input", date, err)
			}
		})
	}
}

func TestParseDateHelp(t *testing.T) {
	date, err := parseDate([]string{"--help"})
	if !errors.Is(err, flag.ErrHelp) || !date.IsZero() {
		t.Fatalf("got date %v, error %v, want help", date, err)
	}
}
