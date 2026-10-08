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

// Regression test: primaryKeySQL used to apply the row-cap LIMIT per
// column, so a 2-column PK under Limit:1 returned only the first column.
// A PK's column list is one unit, not a capped collection, so it must
// come back complete regardless of --limit.
func TestDescribeTable_CompositePrimaryKeyIgnoresLimit(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.pgxConn.Exec(ctx, "create table if not exists dbcli_pk_limit_test (a int, b int, primary key (a, b))"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_pk_limit_test")
	})

	desc, err := c.DescribeTable(ctx, "public", "dbcli_pk_limit_test", driver.QueryOptions{Limit: 1})
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}
	if len(desc.PrimaryKey) != 2 || desc.PrimaryKey[0] != "a" || desc.PrimaryKey[1] != "b" {
		t.Fatalf("got primary_key %v, want [a b] (a composite key must not be truncated by --limit)", desc.PrimaryKey)
	}
}

// Regression test: foreignKeysSQL's LIMIT capped constraints, not the
// rows DescribeTable expands them into, so a 4-column composite FK under
// Limit:1 returned 4 foreign_keys rows instead of 1.
// TestDescribeTable_CompositeForeignKeyRespectsLimit is a regression test
// for a code-review finding that went through two rounds: the first fix
// let a constraint's expansion stop mid-way through under a tight limit,
// which bounded the row count correctly but left the surviving rows
// carrying the full, untruncated Columns/RefColumns of a constraint they
// no longer fully represented, with nothing to signal the cut. The real
// fix treats a constraint as all-or-nothing: a 4-column composite FK
// under a limit it can't fully fit in is dropped entirely, never
// partially included.
func TestDescribeTable_CompositeForeignKeyRespectsLimit(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	setup := `
		create table if not exists dbcli_fk_cap_parent_test (a int, b int, c int, d int, primary key (a, b, c, d));
		create table if not exists dbcli_fk_cap_child_test (
			w int, x int, y int, z int,
			foreign key (w, x, y, z) references dbcli_fk_cap_parent_test(a, b, c, d)
		);
	`
	if _, err := c.pgxConn.Exec(ctx, setup); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_fk_cap_child_test, dbcli_fk_cap_parent_test")
	})

	t.Run("limit too small to fit the whole constraint drops it entirely", func(t *testing.T) {
		desc, err := c.DescribeTable(ctx, "public", "dbcli_fk_cap_child_test", driver.QueryOptions{Limit: 1})
		if err != nil {
			t.Fatalf("DescribeTable: %v", err)
		}
		if len(desc.ForeignKeys) != 0 {
			t.Fatalf("got %d foreign_keys rows, want 0 (a 4-column FK can't fit under limit 1, so it must be dropped whole, not partially included), got %+v", len(desc.ForeignKeys), desc.ForeignKeys)
		}
	})

	t.Run("limit that exactly fits includes the whole constraint with accurate arrays", func(t *testing.T) {
		desc, err := c.DescribeTable(ctx, "public", "dbcli_fk_cap_child_test", driver.QueryOptions{Limit: 4})
		if err != nil {
			t.Fatalf("DescribeTable: %v", err)
		}
		if len(desc.ForeignKeys) != 4 {
			t.Fatalf("got %d foreign_keys rows, want exactly 4, got %+v", len(desc.ForeignKeys), desc.ForeignKeys)
		}
		for _, fk := range desc.ForeignKeys {
			if len(fk.Columns) != 4 || len(fk.RefColumns) != 4 {
				t.Fatalf("expected every row's Columns/RefColumns to be the full 4-element constraint, got %+v", fk)
			}
		}
	})
}

// Regression test: enumValuesSQL had no LIMIT, unlike every other
// introspection query here, so an enum with many labels returned
// unbounded rows.
func TestDescribeTable_EnumValuesRespectLimit(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.pgxConn.Exec(ctx, "create type dbcli_enum_limit_test_status as enum ('a', 'b', 'c')"); err != nil {
		t.Fatalf("create type: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_enum_limit_test")
		c.pgxConn.Exec(context.Background(), "drop type if exists dbcli_enum_limit_test_status")
	})
	if _, err := c.pgxConn.Exec(ctx, "create table dbcli_enum_limit_test (status dbcli_enum_limit_test_status)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	desc, err := c.DescribeTable(ctx, "public", "dbcli_enum_limit_test", driver.QueryOptions{Limit: 1})
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}
	if len(desc.Columns) != 1 {
		t.Fatalf("expected the column limit to also cap to 1 column, got %d", len(desc.Columns))
	}
	if len(desc.Columns[0].EnumValues) != 1 {
		t.Fatalf("got enum_values %v, want exactly 1 label (--limit must cap enum value lookups too)", desc.Columns[0].EnumValues)
	}
}
