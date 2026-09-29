package postgres

import (
	"testing"
	"time"
)

func TestMarshalValue(t *testing.T) {
	uuidBytes := [16]byte{0x38, 0xf9, 0xd7, 0xd7, 0xd4, 0xed, 0x4a, 0x1e, 0x9c, 0x1a, 0x2f, 0x6b, 0x7c, 0x8d, 0x9e, 0x0f}

	cases := []struct {
		name   string
		in     any
		pgType string
		want   any
	}{
		{"nil", nil, "", nil},
		{"int64", int64(42), "int8", "42"},
		{"float64", float64(3.5), "float8", "3.5"},
		{"bool passes through", true, "bool", true},
		{"string passes through", "hello", "text", "hello"},
		{"bytea as base64", []byte{0x01, 0x02}, "bytea", "AQI="},
		{"timestamptz as RFC3339Nano UTC", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), "timestamptz", "2026-01-02T03:04:05Z"},
		{"timestamp (no tz) as RFC3339Nano UTC", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), "timestamp", "2026-01-02T03:04:05Z"},
		{"date as YYYY-MM-DD, no time component", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "date", "2026-01-02"},
		{"uuid [16]byte as canonical string", uuidBytes, "uuid", "38f9d7d7-d4ed-4a1e-9c1a-2f6b7c8d9e0f"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := marshalValue(tc.in, tc.pgType)
			if got != tc.want {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestMarshalValue_ArrayOfDatesFormatsElementsAsDates(t *testing.T) {
	dates := []time.Time{
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC),
	}

	got, ok := marshalValue(dates, "_date").([]any)
	if !ok {
		t.Fatalf("expected []any, got %#v", got)
	}
	want := []any{"2026-01-02", "2026-03-04"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
