package log

import (
	"strings"
	"testing"
)

func TestTruncateBody(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		maxSize     int
		previewSize int
		wantPrefix  string
		wantExact   string
	}{
		{
			name:        "small body not truncated",
			body:        "small body",
			maxSize:     100,
			previewSize: 50,
			wantExact:   "small body",
		},
		{
			name:        "exact size not truncated",
			body:        "exact",
			maxSize:     5,
			previewSize: 3,
			wantExact:   "exact",
		},
		{
			name:        "large body truncated",
			body:        strings.Repeat("a", 200),
			maxSize:     100,
			previewSize: 50,
			wantPrefix:  "[LARGE_BODY: truncated, size: 200 bytes, preview: ",
		},
		{
			name:        "preview size larger than body",
			body:        strings.Repeat("b", 150),
			maxSize:     100,
			previewSize: 200,
			wantPrefix:  "[LARGE_BODY: truncated, size: 150 bytes, preview: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateBody(tt.body, tt.maxSize, tt.previewSize)
			if tt.wantExact != "" {
				if got != tt.wantExact {
					t.Errorf("TruncateBody() = %v, want %v", got, tt.wantExact)
				}
			} else if tt.wantPrefix != "" {
				if !strings.HasPrefix(got, tt.wantPrefix) {
					t.Errorf("TruncateBody() = %v, want prefix %v", got, tt.wantPrefix)
				}
				if !strings.HasSuffix(got, "...]") {
					t.Errorf("TruncateBody() = %v, should end with '...]'", got)
				}
			}
		})
	}
}

func TestTruncateBodyDefault(t *testing.T) {
	// Test with content smaller than default max size
	small := "small content"
	if got := TruncateBodyDefault(small); got != small {
		t.Errorf("TruncateBodyDefault() = %v, want %v", got, small)
	}

	// Test with content larger than default max size (2048)
	large := strings.Repeat("x", 3000)
	got := TruncateBodyDefault(large)
	if !strings.HasPrefix(got, "[LARGE_BODY: truncated, size: 3000 bytes, preview: ") {
		t.Errorf("TruncateBodyDefault() = %v, expected truncated format", got)
	}
}
