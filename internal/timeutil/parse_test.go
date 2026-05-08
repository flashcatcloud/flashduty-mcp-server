package timeutil

import (
	"math"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	type testCase struct {
		name        string
		input       string
		wantExact   int64
		wantApprox  int64
		exactMatch  bool
		approxMatch bool
		wantErr     bool
		tolerance   int64
	}

	now := time.Now().Unix()

	tests := []testCase{
		{name: "relative 5m", input: "5m", wantApprox: now - int64(5*time.Minute/time.Second), approxMatch: true, tolerance: 2},
		{name: "relative 24h", input: "24h", wantApprox: now - int64(24*time.Hour/time.Second), approxMatch: true, tolerance: 2},
		{name: "relative 168h", input: "168h", wantApprox: now - int64(168*time.Hour/time.Second), approxMatch: true, tolerance: 2},
		{name: "relative 30s", input: "30s", wantApprox: now - 30, approxMatch: true, tolerance: 2},
		{name: "absolute date", input: "2026-04-01", wantExact: time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local).Unix(), exactMatch: true},
		{name: "absolute datetime", input: "2026-04-01 10:00:00", wantExact: time.Date(2026, 4, 1, 10, 0, 0, 0, time.Local).Unix(), exactMatch: true},
		{name: "unix timestamp", input: "1712000000", wantExact: 1712000000, exactMatch: true},
		{name: "now keyword", input: "now", wantApprox: now, approxMatch: true, tolerance: 2},
		{name: "empty string", input: "", wantApprox: now, approxMatch: true, tolerance: 2},
		{name: "garbage", input: "garbage", wantErr: true},
		{name: "abc123", input: "abc123", wantErr: true},
		{name: "small number", input: "999", wantErr: true},
		{name: "whitespace 24h", input: "  24h  ", wantApprox: now - int64(24*time.Hour/time.Second), approxMatch: true, tolerance: 2},
		{name: "negative duration", input: "-5m", wantErr: true},
		{name: "boundary 1000000000 rejected", input: "1000000000", wantErr: true},
		{name: "boundary 1000000001 accepted", input: "1000000001", wantExact: 1000000001, exactMatch: true},
		{name: "future +24h", input: "+24h", wantApprox: now + int64(24*time.Hour/time.Second), approxMatch: true, tolerance: 2},
		{name: "future +7d", input: "+7d", wantApprox: now + int64(7*24*time.Hour/time.Second), approxMatch: true, tolerance: 2},
		{name: "past 7d", input: "7d", wantApprox: now - int64(7*24*time.Hour/time.Second), approxMatch: true, tolerance: 2},
		{name: "future garbage", input: "+garbage", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) expected error, got %d", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.input, err)
			}
			if tc.exactMatch && got != tc.wantExact {
				t.Errorf("Parse(%q) = %d, want exactly %d", tc.input, got, tc.wantExact)
			}
			if tc.approxMatch {
				diff := int64(math.Abs(float64(got - tc.wantApprox)))
				if diff > tc.tolerance {
					t.Errorf("Parse(%q) = %d, want approximately %d (tolerance %ds, actual diff %ds)",
						tc.input, got, tc.wantApprox, tc.tolerance, diff)
				}
			}
		})
	}
}
