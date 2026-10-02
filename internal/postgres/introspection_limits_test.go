package postgres

import (
	"context"
	"testing"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

// Before this test existed, ListSchema/ListSchemas/DescribeTable ran on the
// bare connection with no statement_timeout and no row cap at all — worse
// than Query/Sample, which at least had a server-side timeout. These tests
// prove both are now wired up.

func TestBeginReadOnlyTimeoutTx_EnforcesStatementTimeout(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	tx, err := c.beginReadOnlyTimeoutTx(ctx, 1)
	if err != nil {
		t.Fatalf("beginReadOnlyTimeoutTx: %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "select pg_sleep(2)"); err == nil {
		t.Fatalf("expected statement_timeout to cancel a query slower than the timeout")
	}
}

func TestListSchema_RespectsLimit(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.pgxConn.Exec(ctx, "create table if not exists dbcli_schema_limit_test (id serial primary key)"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_schema_limit_test")
	})

	tables, err := c.ListSchema(ctx, "public", driver.QueryOptions{Limit: 1})
	if err != nil {
		t.Fatalf("ListSchema: %v", err)
	}
	if len(tables) != 1 {
		t.Fatalf("got %d tables, want exactly 1 (limit should cap regardless of how many exist)", len(tables))
	}
}

func TestListSchemas_RespectsLimit(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.pgxConn.Exec(ctx, "create schema if not exists dbcli_schemas_limit_test"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop schema if exists dbcli_schemas_limit_test")
	})

	schemas, err := c.ListSchemas(ctx, driver.QueryOptions{Limit: 1})
	if err != nil {
		t.Fatalf("ListSchemas: %v", err)
	}
	if len(schemas) != 1 {
		t.Fatalf("got %d schemas, want exactly 1 (limit should cap regardless of how many exist)", len(schemas))
	}
}

func TestDescribeTable_RespectsLimit(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.pgxConn.Exec(ctx, "create table if not exists dbcli_describe_limit_test (a int, b int, c int)"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_describe_limit_test")
	})

	desc, err := c.DescribeTable(ctx, "public", "dbcli_describe_limit_test", driver.QueryOptions{Limit: 1})
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}
	if len(desc.Columns) != 1 {
		t.Fatalf("got %d columns, want exactly 1 (limit should cap the columns list too)", len(desc.Columns))
	}
}

func TestDescribeTable_ReverseForeignKeysRespectLimit(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	setup := `
		create table if not exists dbcli_revfk_limit_parent_test (id serial primary key);
		create table if not exists dbcli_revfk_limit_child_a_test (id serial primary key, parent_id integer references dbcli_revfk_limit_parent_test(id));
		create table if not exists dbcli_revfk_limit_child_b_test (id serial primary key, parent_id integer references dbcli_revfk_limit_parent_test(id));
	`
	if _, err := c.pgxConn.Exec(ctx, setup); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_revfk_limit_child_a_test, dbcli_revfk_limit_child_b_test, dbcli_revfk_limit_parent_test")
	})

	desc, err := c.DescribeTable(ctx, "public", "dbcli_revfk_limit_parent_test", driver.QueryOptions{Limit: 1})
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}
	if len(desc.ReferencedBy) != 1 {
		t.Fatalf("got %d referenced_by entries, want exactly 1 (limit should cap reverse foreign keys too), got %+v", len(desc.ReferencedBy), desc.ReferencedBy)
	}
}
