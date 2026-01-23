package trace

import (
	"context"
	"net/http"
	"testing"
)

func TestParseTraceparent(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   *TraceContext
		wantOk bool
	}{
		{
			name:   "valid traceparent",
			header: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			want: &TraceContext{
				TraceID:    "0af7651916cd43dd8448eb211c80319c",
				SpanID:     "b7ad6b7169203331",
				TraceFlags: 0x01,
			},
			wantOk: true,
		},
		{
			name:   "valid traceparent not sampled",
			header: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00",
			want: &TraceContext{
				TraceID:    "0af7651916cd43dd8448eb211c80319c",
				SpanID:     "b7ad6b7169203331",
				TraceFlags: 0x00,
			},
			wantOk: true,
		},
		{
			name:   "invalid version",
			header: "01-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			wantOk: false,
		},
		{
			name:   "invalid trace-id length",
			header: "00-0af7651916cd43dd8448eb211c8031-b7ad6b7169203331-01",
			wantOk: false,
		},
		{
			name:   "invalid span-id length",
			header: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b71692033-01",
			wantOk: false,
		},
		{
			name:   "all zeros trace-id",
			header: "00-00000000000000000000000000000000-b7ad6b7169203331-01",
			wantOk: false,
		},
		{
			name:   "all zeros span-id",
			header: "00-0af7651916cd43dd8448eb211c80319c-0000000000000000-01",
			wantOk: false,
		},
		{
			name:   "invalid hex in trace-id",
			header: "00-0af7651916cd43dd8448eb211c8031zz-b7ad6b7169203331-01",
			wantOk: false,
		},
		{
			name:   "too few parts",
			header: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331",
			wantOk: false,
		},
		{
			name:   "empty string",
			header: "",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseTraceparent(tt.header)
			if ok != tt.wantOk {
				t.Errorf("ParseTraceparent() ok = %v, wantOk %v", ok, tt.wantOk)
				return
			}
			if tt.wantOk {
				if got.TraceID != tt.want.TraceID {
					t.Errorf("TraceID = %v, want %v", got.TraceID, tt.want.TraceID)
				}
				if got.SpanID != tt.want.SpanID {
					t.Errorf("SpanID = %v, want %v", got.SpanID, tt.want.SpanID)
				}
				if got.TraceFlags != tt.want.TraceFlags {
					t.Errorf("TraceFlags = %v, want %v", got.TraceFlags, tt.want.TraceFlags)
				}
			}
		})
	}
}

func TestTraceContext_ToTraceparent(t *testing.T) {
	tc := &TraceContext{
		TraceID:    "0af7651916cd43dd8448eb211c80319c",
		SpanID:     "b7ad6b7169203331",
		TraceFlags: 0x01,
	}
	want := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	if got := tc.ToTraceparent(); got != want {
		t.Errorf("ToTraceparent() = %v, want %v", got, want)
	}
}

func TestTraceContext_IsSampled(t *testing.T) {
	tests := []struct {
		name       string
		traceFlags byte
		want       bool
	}{
		{"sampled", 0x01, true},
		{"not sampled", 0x00, false},
		{"sampled with other flags", 0x03, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := &TraceContext{TraceFlags: tt.traceFlags}
			if got := tc.IsSampled(); got != tt.want {
				t.Errorf("IsSampled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFromHTTPHeaders(t *testing.T) {
	t.Run("with traceparent", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
		headers.Set("tracestate", "vendor=value")

		tc := FromHTTPHeaders(headers)
		if tc == nil {
			t.Fatal("expected TraceContext, got nil")
		}
		if tc.TraceID != "0af7651916cd43dd8448eb211c80319c" {
			t.Errorf("TraceID = %v, want %v", tc.TraceID, "0af7651916cd43dd8448eb211c80319c")
		}
		if tc.TraceState != "vendor=value" {
			t.Errorf("TraceState = %v, want %v", tc.TraceState, "vendor=value")
		}
	})

	t.Run("without traceparent", func(t *testing.T) {
		headers := http.Header{}
		tc := FromHTTPHeaders(headers)
		if tc != nil {
			t.Errorf("expected nil, got %v", tc)
		}
	})

	t.Run("with invalid traceparent", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("traceparent", "invalid")
		tc := FromHTTPHeaders(headers)
		if tc != nil {
			t.Errorf("expected nil, got %v", tc)
		}
	})
}

func TestContextWithTraceContext(t *testing.T) {
	tc := &TraceContext{
		TraceID:    "0af7651916cd43dd8448eb211c80319c",
		SpanID:     "b7ad6b7169203331",
		TraceFlags: 0x01,
	}

	ctx := context.Background()
	ctx = ContextWithTraceContext(ctx, tc)

	got := FromContext(ctx)
	if got == nil {
		t.Fatal("expected TraceContext, got nil")
	}
	if got.TraceID != tc.TraceID {
		t.Errorf("TraceID = %v, want %v", got.TraceID, tc.TraceID)
	}
}

func TestFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	tc := FromContext(ctx)
	if tc != nil {
		t.Errorf("expected nil, got %v", tc)
	}
}

func TestTraceContext_SetHTTPHeaders(t *testing.T) {
	tc := &TraceContext{
		TraceID:    "0af7651916cd43dd8448eb211c80319c",
		SpanID:     "b7ad6b7169203331",
		TraceFlags: 0x01,
		TraceState: "vendor=value",
	}

	headers := http.Header{}
	tc.SetHTTPHeaders(headers)

	if got := headers.Get("traceparent"); got != "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01" {
		t.Errorf("traceparent = %v", got)
	}
	if got := headers.Get("tracestate"); got != "vendor=value" {
		t.Errorf("tracestate = %v", got)
	}
}

func TestNewTraceContext(t *testing.T) {
	tc, err := NewTraceContext()
	if err != nil {
		t.Fatalf("NewTraceContext() error = %v", err)
	}
	if tc == nil {
		t.Fatal("NewTraceContext() returned nil")
	}
	if len(tc.TraceID) != 32 {
		t.Errorf("TraceID length = %d, want 32", len(tc.TraceID))
	}
	if len(tc.SpanID) != 16 {
		t.Errorf("SpanID length = %d, want 16", len(tc.SpanID))
	}
	if !isValidHex(tc.TraceID) {
		t.Errorf("TraceID = %v, should be valid hex", tc.TraceID)
	}
	if !isValidHex(tc.SpanID) {
		t.Errorf("SpanID = %v, should be valid hex", tc.SpanID)
	}
	if isAllZeros(tc.TraceID) {
		t.Errorf("TraceID should not be all zeros")
	}
	if isAllZeros(tc.SpanID) {
		t.Errorf("SpanID should not be all zeros")
	}
	if tc.TraceFlags != 0x01 {
		t.Errorf("TraceFlags = %v, want 0x01", tc.TraceFlags)
	}
}

func TestFromHTTPHeadersOrNew(t *testing.T) {
	t.Run("with existing traceparent", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
		tc, err := FromHTTPHeadersOrNew(headers)
		if err != nil {
			t.Fatalf("FromHTTPHeadersOrNew() error = %v", err)
		}
		if tc.TraceID != "0af7651916cd43dd8448eb211c80319c" {
			t.Errorf("TraceID = %v, want %v", tc.TraceID, "0af7651916cd43dd8448eb211c80319c")
		}
	})

	t.Run("without traceparent, generates new", func(t *testing.T) {
		headers := http.Header{}
		tc, err := FromHTTPHeadersOrNew(headers)
		if err != nil {
			t.Fatalf("FromHTTPHeadersOrNew() error = %v", err)
		}
		if tc == nil {
			t.Fatal("expected TraceContext, got nil")
		}
		if len(tc.TraceID) != 32 {
			t.Errorf("TraceID length = %d, want 32", len(tc.TraceID))
		}
		if len(tc.SpanID) != 16 {
			t.Errorf("SpanID length = %d, want 16", len(tc.SpanID))
		}
	})
}
