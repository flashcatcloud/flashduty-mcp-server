package flashduty

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// getLocalTimezone returns the local timezone location.
// Priority:
// 1. TZ environment variable (if set)
// 2. System local timezone (if not UTC)
// 3. Asia/Shanghai as fallback (for containers without timezone data)
func getLocalTimezone() *time.Location {
	// First, try TZ environment variable
	if tz := os.Getenv("TZ"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}

	// Try to use system local timezone
	loc := time.Local
	// Check if Local() returns UTC (common in containers without timezone data)
	if loc.String() == "UTC" || loc.String() == "Local" {
		// Fallback to Asia/Shanghai for containers without timezone data
		// Go 1.15+ has built-in timezone data, so this should work even in distroless images
		if shanghai, err := time.LoadLocation("Asia/Shanghai"); err == nil {
			return shanghai
		}
	}
	return loc
}

// orderedTextHandler is a custom slog handler that orders fields consistently:
// time level trace_id msg [other fields...]
type orderedTextHandler struct {
	w       io.Writer
	opts    slog.HandlerOptions
	localTZ *time.Location
	attrs   []slog.Attr // Attributes added via WithAttrs
}

// newOrderedTextHandler creates a new orderedTextHandler with local timezone support.
func newOrderedTextHandler(w io.Writer, level slog.Level) slog.Handler {
	return &orderedTextHandler{
		w:       w,
		opts:    slog.HandlerOptions{Level: level},
		localTZ: getLocalTimezone(),
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *orderedTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	minLevel := h.opts.Level.Level()
	return level >= minLevel
}

// Handle processes the log record and writes it with ordered fields.
func (h *orderedTextHandler) Handle(ctx context.Context, r slog.Record) error {
	buf := make([]byte, 0, 1024)

	// 1. Time (always first)
	buf = append(buf, "time="...)
	t := r.Time.In(h.localTZ)
	buf = t.AppendFormat(buf, time.RFC3339Nano)
	buf = append(buf, ' ')

	// 2. Level (always second)
	buf = append(buf, "level="...)
	buf = append(buf, r.Level.String()...)
	buf = append(buf, ' ')

	// 3. Trace ID (always third, extract from attrs or use empty)
	traceID := ""
	var otherAttrs []slog.Attr

	// Collect all attributes, extracting trace_id
	allAttrs := make([]slog.Attr, 0, len(h.attrs)+10)
	allAttrs = append(allAttrs, h.attrs...)

	r.Attrs(func(a slog.Attr) bool {
		allAttrs = append(allAttrs, a)
		return true
	})

	// Process all attributes
	for _, a := range allAttrs {
		if a.Key == "trace_id" {
			if a.Value.Kind() == slog.KindString {
				traceID = a.Value.String()
			}
			continue // Skip, will print separately
		}
		otherAttrs = append(otherAttrs, a)
	}

	// Always include trace_id (empty if not present)
	buf = append(buf, "trace_id="...)
	if traceID != "" {
		buf = append(buf, traceID...)
	} else {
		buf = append(buf, "-"...)
	}
	buf = append(buf, ' ')

	// 4. Message
	buf = append(buf, "msg="...)
	buf = append(buf, r.Message...)
	buf = append(buf, ' ')

	// 5. Other attributes
	for _, a := range otherAttrs {
		buf = append(buf, a.Key...)
		buf = append(buf, '=')
		buf = appendValue(buf, a.Value)
		buf = append(buf, ' ')
	}

	// Remove trailing space and add newline
	if len(buf) > 0 && buf[len(buf)-1] == ' ' {
		buf = buf[:len(buf)-1]
	}
	buf = append(buf, '\n')

	_, err := h.w.Write(buf)
	return err
}

// WithAttrs returns a new handler with the given attributes.
func (h *orderedTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &orderedTextHandler{
		w:       h.w,
		opts:    h.opts,
		localTZ: h.localTZ,
		attrs:   newAttrs,
	}
}

// WithGroup returns a new handler with the given group name.
func (h *orderedTextHandler) WithGroup(name string) slog.Handler {
	return &orderedTextHandler{
		w:       h.w,
		opts:    h.opts,
		localTZ: h.localTZ,
		attrs:   h.attrs,
	}
}

// appendValue appends a value to the buffer in a format similar to slog's text handler.
func appendValue(buf []byte, v slog.Value) []byte {
	switch v.Kind() {
	case slog.KindString:
		buf = append(buf, v.String()...)
	case slog.KindInt64:
		buf = appendInt(buf, v.Int64())
	case slog.KindUint64:
		buf = appendUint(buf, v.Uint64())
	case slog.KindFloat64:
		buf = appendFloat(buf, v.Float64())
	case slog.KindBool:
		if v.Bool() {
			buf = append(buf, "true"...)
		} else {
			buf = append(buf, "false"...)
		}
	case slog.KindDuration:
		buf = append(buf, v.Duration().String()...)
	case slog.KindTime:
		buf = append(buf, v.Time().Format(time.RFC3339Nano)...)
	case slog.KindAny:
		buf = append(buf, fmtAny(v.Any())...)
	case slog.KindGroup:
		// Groups are flattened
		for _, a := range v.Group() {
			buf = append(buf, a.Key...)
			buf = append(buf, '=')
			buf = appendValue(buf, a.Value)
			buf = append(buf, ' ')
		}
		if len(buf) > 0 && buf[len(buf)-1] == ' ' {
			buf = buf[:len(buf)-1]
		}
	}
	return buf
}

// fmtAny formats any value as a string.
func fmtAny(v any) string {
	if v == nil {
		return "nil"
	}
	if err, ok := v.(error); ok {
		return err.Error()
	}
	return fmt.Sprintf("%+v", v)
}

// appendInt appends an int64 to the buffer.
func appendInt(buf []byte, x int64) []byte {
	// Use strconv.AppendInt to avoid integer overflow issues
	return strconv.AppendInt(buf, x, 10)
}

// appendUint appends a uint64 to the buffer.
func appendUint(buf []byte, x uint64) []byte {
	if x == 0 {
		return append(buf, '0')
	}
	var digits [20]byte
	i := len(digits)
	for x > 0 {
		i--
		digits[i] = byte(x%10) + '0'
		x /= 10
	}
	return append(buf, digits[i:]...)
}

// appendFloat appends a float64 to the buffer.
func appendFloat(buf []byte, x float64) []byte {
	return strconv.AppendFloat(buf, x, 'g', -1, 64)
}
