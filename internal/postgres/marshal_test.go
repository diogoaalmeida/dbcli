package postgres

import (
	"testing"
	"time"
)

func TestMarshalValue(t *testing.T) {
	uuidBytes := [16]byte{0x38, 0xf9, 0xd7, 0xd7, 0xd4, 0xed, 0x4a, 0x1e, 0x9c, 0x1a, 0x2f, 0x6b, 0x7c, 0x8d, 0x9e, 0x0f}

	cases := []struct {
		name string
		in   any
		want any
	}{
		{"nil", nil, nil},
		{"int64", int64(42), "42"},
		{"float64", float64(3.5), "3.5"},
		{"bool passes through", true, true},
		{"string passes through", "hello", "hello"},
		{"bytea as base64", []byte{0x01, 0x02}, "AQI="},
		{"timestamp as RFC3339Nano UTC", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), "2026-01-02T03:04:05Z"},
		{"uuid [16]byte as canonical string", uuidBytes, "38f9d7d7-d4ed-4a1e-9c1a-2f6b7c8d9e0f"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := marshalValue(tc.in)
			if got != tc.want {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}
