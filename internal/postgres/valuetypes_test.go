package postgres

import (
	"context"
	"testing"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

// TestQuery_RichColumnTypes runs a query against a real table covering the
// Postgres types most likely to decode into something other than a plain
// Go string/int/bool via pgx — uuid, numeric, jsonb, text[], and bytea all
// have their own codecs. This is the test that would have caught the uuid
// bug (pgx decodes uuid into [16]byte, which marshalValue's early version
// serialized as an array of per-byte numbers instead of a canonical string).
func TestQuery_RichColumnTypes(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	setup := `
		create extension if not exists pgcrypto;
		create table if not exists dbcli_valuetypes_test (
			id uuid primary key default gen_random_uuid(),
			amount numeric(10,2) not null,
			tags text[] not null,
			metadata jsonb not null,
			payload bytea not null,
			is_active boolean not null,
			note text
		);
		truncate dbcli_valuetypes_test;
		insert into dbcli_valuetypes_test (amount, tags, metadata, payload, is_active, note)
		values (1234.56, array['a','b'], '{"k":"v"}'::jsonb, '\xdeadbeef'::bytea, true, null);
	`
	if _, err := c.pool.Exec(ctx, setup); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pool.Exec(context.Background(), "drop table if exists dbcli_valuetypes_test")
	})

	result, err := c.Query(ctx, "select id, amount, tags, metadata, payload, is_active, note from dbcli_valuetypes_test", driver.QueryOptions{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if result.RowCount != 1 {
		t.Fatalf("got %d rows, want 1", result.RowCount)
	}
	row := result.Rows[0]

	id, ok := row["id"].(string)
	if !ok || len(id) != 36 || id[8] != '-' {
		t.Errorf("id: expected a canonical UUID string, got %#v", row["id"])
	}

	if amount, ok := row["amount"].(string); !ok || amount != "1234.56" {
		t.Errorf("amount: expected numeric to be serialized as string \"1234.56\", got %#v", row["amount"])
	}

	tags, ok := row["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("tags: expected [\"a\",\"b\"], got %#v", row["tags"])
	}

	if _, ok := row["metadata"]; !ok {
		t.Errorf("metadata: expected a jsonb value, got nothing")
	}

	if row["is_active"] != true {
		t.Errorf("is_active: expected bool true, got %#v", row["is_active"])
	}

	if row["note"] != nil {
		t.Errorf("note: expected null, got %#v", row["note"])
	}

	if _, ok := row["payload"].(string); !ok {
		t.Errorf("payload: expected bytea to be serialized as a base64 string, got %#v", row["payload"])
	}
}
