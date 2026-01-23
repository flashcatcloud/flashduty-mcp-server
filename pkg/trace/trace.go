package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

// W3C Trace Context header names
const (
	TraceparentHeader = "traceparent"
	TracestateHeader  = "tracestate"
)

// TraceContext holds W3C Trace Context fields.
// See: https://www.w3.org/TR/trace-context/
type TraceContext struct {
	// TraceID is the 32-character hex-encoded trace identifier
	TraceID string
	// SpanID is the 16-character hex-encoded span/parent identifier
	SpanID string
	// TraceFlags is the trace flags byte (e.g., 0x01 for sampled)
	TraceFlags byte
	// TraceState is the optional vendor-specific trace state
	TraceState string
}

type contextKey struct{}

var traceContextKey = contextKey{}

// FromHTTPHeaders extracts W3C Trace Context from HTTP headers.
// Returns nil if no valid traceparent header is found.
func FromHTTPHeaders(headers http.Header) *TraceContext {
	traceparent := headers.Get(TraceparentHeader)
	if traceparent == "" {
		return nil
	}

	tc, ok := ParseTraceparent(traceparent)
	if !ok {
		return nil
	}

	tc.TraceState = headers.Get(TracestateHeader)
	return tc
}

// ParseTraceparent parses a W3C traceparent header value.
// Format: {version}-{trace-id}-{parent-id}-{trace-flags}
// Example: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
func ParseTraceparent(header string) (*TraceContext, bool) {
	parts := strings.Split(header, "-")
	if len(parts) != 4 {
		return nil, false
	}

	version := parts[0]
	traceID := parts[1]
	spanID := parts[2]
	flagsStr := parts[3]

	// Validate version (currently only 00 is supported)
	if version != "00" {
		return nil, false
	}

	// Validate trace-id (32 hex characters, not all zeros)
	if len(traceID) != 32 || !isValidHex(traceID) || isAllZeros(traceID) {
		return nil, false
	}

	// Validate parent-id (16 hex characters, not all zeros)
	if len(spanID) != 16 || !isValidHex(spanID) || isAllZeros(spanID) {
		return nil, false
	}

	// Validate trace-flags (2 hex characters)
	if len(flagsStr) != 2 || !isValidHex(flagsStr) {
		return nil, false
	}

	flags, err := hex.DecodeString(flagsStr)
	if err != nil || len(flags) != 1 {
		return nil, false
	}

	return &TraceContext{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: flags[0],
	}, true
}

// ToTraceparent formats the TraceContext as a W3C traceparent header value.
func (tc *TraceContext) ToTraceparent() string {
	return fmt.Sprintf("00-%s-%s-%02x", tc.TraceID, tc.SpanID, tc.TraceFlags)
}

// IsSampled returns true if the sampled flag is set.
func (tc *TraceContext) IsSampled() bool {
	return tc.TraceFlags&0x01 != 0
}

// ContextWithTraceContext returns a new context with the TraceContext attached.
func ContextWithTraceContext(ctx context.Context, tc *TraceContext) context.Context {
	return context.WithValue(ctx, traceContextKey, tc)
}

// FromContext extracts TraceContext from context.
// Returns nil if no TraceContext is found.
func FromContext(ctx context.Context) *TraceContext {
	tc, _ := ctx.Value(traceContextKey).(*TraceContext)
	return tc
}

// SetHTTPHeaders sets W3C Trace Context headers on an HTTP request.
func (tc *TraceContext) SetHTTPHeaders(headers http.Header) {
	headers.Set(TraceparentHeader, tc.ToTraceparent())
	if tc.TraceState != "" {
		headers.Set(TracestateHeader, tc.TraceState)
	}
}

// NewTraceContext generates a new TraceContext with random trace ID and span ID.
func NewTraceContext() (*TraceContext, error) {
	// Generate 16 random bytes for trace ID (32 hex characters)
	traceIDBytes := make([]byte, 16)
	if _, err := rand.Read(traceIDBytes); err != nil {
		return nil, fmt.Errorf("failed to generate trace ID: %w", err)
	}
	traceID := hex.EncodeToString(traceIDBytes)

	// Generate 8 random bytes for span ID (16 hex characters)
	spanIDBytes := make([]byte, 8)
	if _, err := rand.Read(spanIDBytes); err != nil {
		return nil, fmt.Errorf("failed to generate span ID: %w", err)
	}
	spanID := hex.EncodeToString(spanIDBytes)

	return &TraceContext{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: 0x01, // Sampled by default
	}, nil
}

// FromHTTPHeadersOrNew extracts W3C Trace Context from HTTP headers, or generates a new one if not found.
func FromHTTPHeadersOrNew(headers http.Header) (*TraceContext, error) {
	tc := FromHTTPHeaders(headers)
	if tc != nil {
		return tc, nil
	}

	// Generate new trace context if not found
	return NewTraceContext()
}

// isValidHex checks if a string contains only valid hexadecimal characters.
func isValidHex(s string) bool {
	for _, c := range s {
		if !isHexDigit(c) {
			return false
		}
	}
	return true
}

// isHexDigit returns true if the rune is a valid hexadecimal digit.
func isHexDigit(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// isAllZeros checks if a hex string represents all zeros.
func isAllZeros(s string) bool {
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return true
}
