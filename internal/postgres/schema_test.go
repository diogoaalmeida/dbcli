package postgres

import (
	"context"
	"testing"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

func TestListSchemas_IncludesPublicAndExcludesSystemSchemas(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	schemas, err := c.ListSchemas(ctx)
	if err != nil {
		t.Fatalf("ListSchemas: %v", err)
	}

	var sawPublic bool
	for _, s := range schemas {
		if s.Name == "public" {
			sawPublic = true
		}
		if s.Name == "pg_catalog" || s.Name == "information_schema" {
			t.Fatalf("expected system schema %q to be filtered out, got %+v", s.Name, schemas)
		}
	}
	if !sawPublic {
		t.Fatalf("expected \"public\" schema in result, got %+v", schemas)
	}
}

func TestListSchema_ListsTableInGivenSchema(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.pool.Exec(ctx, "create table if not exists dbcli_schema_list_test (id serial primary key)"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pool.Exec(context.Background(), "drop table if exists dbcli_schema_list_test")
	})

	tables, err := c.ListSchema(ctx, "public")
	if err != nil {
		t.Fatalf("ListSchema: %v", err)
	}

	var found *driver.TableInfo
	for i := range tables {
		if tables[i].Name == "dbcli_schema_list_test" {
			found = &tables[i]
		}
	}
	if found == nil {
		t.Fatalf("expected dbcli_schema_list_test in %+v", tables)
	}
	if found.Kind != "table" {
		t.Fatalf("expected kind \"table\", got %q", found.Kind)
	}
}

func TestDescribeTable_ReturnsColumnsIndexesAndForeignKeys(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	setup := `
		create table if not exists dbcli_describe_parent_test (id serial primary key);
		create table if not exists dbcli_describe_child_test (
			id serial primary key,
			parent_id integer references dbcli_describe_parent_test(id),
			name text not null
		);
	`
	if _, err := c.pool.Exec(ctx, setup); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pool.Exec(context.Background(), "drop table if exists dbcli_describe_child_test, dbcli_describe_parent_test")
	})

	desc, err := c.DescribeTable(ctx, "public", "dbcli_describe_child_test")
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}

	if len(desc.Columns) != 3 {
		t.Fatalf("expected 3 columns, got %+v", desc.Columns)
	}
	if len(desc.Indexes) == 0 {
		t.Fatalf("expected at least the primary key index, got none")
	}
	if len(desc.ForeignKeys) != 1 || desc.ForeignKeys[0].RefTable != "dbcli_describe_parent_test" {
		t.Fatalf("expected one foreign key referencing dbcli_describe_parent_test, got %+v", desc.ForeignKeys)
	}
}

func TestDescribeTable_UnknownTableErrors(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.DescribeTable(ctx, "public", "dbcli_does_not_exist_test"); err == nil {
		t.Fatalf("expected an error for a nonexistent table")
	}
}

func TestSample_ReturnsRowsAndRespectsLimit(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.pool.Exec(ctx, "create table if not exists dbcli_sample_test (id serial primary key)"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := c.pool.Exec(ctx, "insert into dbcli_sample_test default values"); err != nil {
			t.Fatalf("test fixture insert: %v", err)
		}
	}
	t.Cleanup(func() {
		c.pool.Exec(context.Background(), "drop table if exists dbcli_sample_test")
	})

	result, err := c.Sample(ctx, "public", "dbcli_sample_test", driver.QueryOptions{Limit: 2})
	if err != nil {
		t.Fatalf("Sample: %v", err)
	}
	if result.RowCount != 2 {
		t.Fatalf("got %d rows, want 2", result.RowCount)
	}
}

func TestSample_UnknownTableErrors(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.Sample(ctx, "public", "dbcli_does_not_exist_test", driver.QueryOptions{}); err == nil {
		t.Fatalf("expected an error for a nonexistent table")
	}
}
