package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

func TestListSchemas_IncludesPublicAndExcludesSystemSchemas(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	schemas, err := c.ListSchemas(ctx, driver.QueryOptions{})
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

	if _, err := c.pgxConn.Exec(ctx, "create table if not exists dbcli_schema_list_test (id serial primary key)"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_schema_list_test")
	})

	tables, err := c.ListSchema(ctx, "public", driver.QueryOptions{})
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
	if _, err := c.pgxConn.Exec(ctx, setup); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_describe_child_test, dbcli_describe_parent_test")
	})

	desc, err := c.DescribeTable(ctx, "public", "dbcli_describe_child_test", driver.QueryOptions{})
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

	if _, err := c.DescribeTable(ctx, "public", "dbcli_does_not_exist_test", driver.QueryOptions{}); err == nil {
		t.Fatalf("expected an error for a nonexistent table")
	}
}

func TestSample_ReturnsRowsAndRespectsLimit(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.pgxConn.Exec(ctx, "create table if not exists dbcli_sample_test (id serial primary key)"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := c.pgxConn.Exec(ctx, "insert into dbcli_sample_test default values"); err != nil {
			t.Fatalf("test fixture insert: %v", err)
		}
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_sample_test")
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

// TestDescribeTable_CompositeForeignKeyPairsColumnsCorrectly is the
// regression test for the original foreignKeysSQL bug: joining
// key_column_usage/constraint_column_usage on (constraint_name,
// table_schema) alone, with no join on ordinal position, silently
// cross-joined a composite FK's columns (2 columns x 2 columns = 4 rows
// with wrong pairings like x->b, y->a, instead of the correct 2 rows).
// The pg_constraint/unnest-WITH-ORDINALITY rewrite pairs by ordinal
// position, so this must produce exactly 2 rows, correctly paired.
func TestDescribeTable_CompositeForeignKeyPairsColumnsCorrectly(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	setup := `
		create table if not exists dbcli_fk_parent_test (a int, b int, primary key (a, b));
		create table if not exists dbcli_fk_child_test (
			x int,
			y int,
			foreign key (x, y) references dbcli_fk_parent_test(a, b)
		);
	`
	if _, err := c.pgxConn.Exec(ctx, setup); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_fk_child_test, dbcli_fk_parent_test")
	})

	desc, err := c.DescribeTable(ctx, "public", "dbcli_fk_child_test", driver.QueryOptions{})
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}

	if len(desc.ForeignKeys) != 2 {
		t.Fatalf("expected exactly 2 foreign key rows (one per column pair), got %d: %+v", len(desc.ForeignKeys), desc.ForeignKeys)
	}

	pairs := map[string]string{}
	for _, fk := range desc.ForeignKeys {
		pairs[fk.Column] = fk.RefColumn
		if fk.RefTable != "dbcli_fk_parent_test" {
			t.Fatalf("expected ref_table dbcli_fk_parent_test, got %q", fk.RefTable)
		}
		if fk.ConstraintName == "" {
			t.Fatalf("expected a non-empty constraint_name, got %+v", fk)
		}
		if len(fk.Columns) != 2 || len(fk.RefColumns) != 2 {
			t.Fatalf("expected the full 2-column constraint arrays on every row, got columns=%v ref_columns=%v", fk.Columns, fk.RefColumns)
		}
	}
	if pairs["x"] != "a" || pairs["y"] != "b" {
		t.Fatalf("expected pairing x->a, y->b, got %+v (this is exactly what the cross-join bug got wrong)", pairs)
	}
}

// TestDescribeTable_CrossSchemaForeignKeyReportsRefSchema covers the other
// half of the original bug: the old query never selected the referenced
// table's schema at all, so a cross-schema FK's target was ambiguous.
func TestDescribeTable_CrossSchemaForeignKeyReportsRefSchema(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	setup := `
		create schema if not exists dbcli_fk_other_schema_test;
		create table if not exists dbcli_fk_other_schema_test.parent_test (id serial primary key);
		create table if not exists dbcli_fk_cross_child_test (
			id serial primary key,
			parent_id integer references dbcli_fk_other_schema_test.parent_test(id)
		);
	`
	if _, err := c.pgxConn.Exec(ctx, setup); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_fk_cross_child_test")
		c.pgxConn.Exec(context.Background(), "drop schema if exists dbcli_fk_other_schema_test cascade")
	})

	desc, err := c.DescribeTable(ctx, "public", "dbcli_fk_cross_child_test", driver.QueryOptions{})
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}

	if len(desc.ForeignKeys) != 1 {
		t.Fatalf("expected exactly 1 foreign key, got %+v", desc.ForeignKeys)
	}
	fk := desc.ForeignKeys[0]
	if fk.RefSchema != "dbcli_fk_other_schema_test" || fk.RefTable != "parent_test" {
		t.Fatalf("expected ref_schema dbcli_fk_other_schema_test and ref_table parent_test, got ref_schema=%q ref_table=%q", fk.RefSchema, fk.RefTable)
	}
}

func TestDescribeTable_ReturnsReferencedBy(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	setup := `
		create table if not exists dbcli_revfk_parent_test (id serial primary key);
		create table if not exists dbcli_revfk_child_test (
			id serial primary key,
			parent_id integer references dbcli_revfk_parent_test(id)
		);
	`
	if _, err := c.pgxConn.Exec(ctx, setup); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_revfk_child_test, dbcli_revfk_parent_test")
	})

	desc, err := c.DescribeTable(ctx, "public", "dbcli_revfk_parent_test", driver.QueryOptions{})
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}

	if len(desc.ReferencedBy) != 1 {
		t.Fatalf("expected exactly 1 referencing foreign key, got %+v", desc.ReferencedBy)
	}
	ref := desc.ReferencedBy[0]
	if ref.Table != "dbcli_revfk_child_test" || len(ref.Columns) != 1 || ref.Columns[0] != "parent_id" || len(ref.RefColumns) != 1 || ref.RefColumns[0] != "id" {
		t.Fatalf("expected dbcli_revfk_child_test.parent_id -> id, got %+v", ref)
	}
}

func TestDescribeTable_ReturnsPrimaryKeyUniqueAndCheckConstraints(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	setup := `
		create table if not exists dbcli_constraints_test (
			id serial,
			email text,
			price numeric,
			primary key (id),
			unique (email),
			check (price > 0)
		);
	`
	if _, err := c.pgxConn.Exec(ctx, setup); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_constraints_test")
	})

	desc, err := c.DescribeTable(ctx, "public", "dbcli_constraints_test", driver.QueryOptions{})
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}

	if len(desc.PrimaryKey) != 1 || desc.PrimaryKey[0] != "id" {
		t.Fatalf("expected primary_key [id], got %v", desc.PrimaryKey)
	}
	if len(desc.UniqueConstraints) != 1 || !strings.Contains(desc.UniqueConstraints[0].Definition, "email") {
		t.Fatalf("expected a unique constraint on email, got %+v", desc.UniqueConstraints)
	}
	if len(desc.CheckConstraints) != 1 || !strings.Contains(desc.CheckConstraints[0].Definition, "price") {
		t.Fatalf("expected a check constraint on price, got %+v", desc.CheckConstraints)
	}

	var pkIndex *driver.IndexInfo
	for i := range desc.Indexes {
		if desc.Indexes[i].Primary {
			pkIndex = &desc.Indexes[i]
		}
	}
	if pkIndex == nil {
		t.Fatalf("expected one index flagged Primary, got %+v", desc.Indexes)
	}
}

// TestDescribeTable_CommentsSurviveADroppedColumn is the regression test
// for using the real pg_attribute attnum (not information_schema's
// ordinal_position) when looking up col_description: ordinal_position
// renumbers to ignore dropped columns, so after dropping a column,
// ordinal_position and attnum diverge for every column after it. Using
// ordinal_position here would point col_description at the wrong physical
// column and silently return the wrong comment (or none).
func TestDescribeTable_CommentsSurviveADroppedColumn(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	setup := `
		create table if not exists dbcli_comment_dropped_test (id serial primary key, drop_me text, name text);
	`
	if _, err := c.pgxConn.Exec(ctx, setup); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	if _, err := c.pgxConn.Exec(ctx, "alter table dbcli_comment_dropped_test drop column drop_me"); err != nil {
		t.Fatalf("drop column: %v", err)
	}
	if _, err := c.pgxConn.Exec(ctx, "comment on table dbcli_comment_dropped_test is 'a test table'"); err != nil {
		t.Fatalf("comment on table: %v", err)
	}
	if _, err := c.pgxConn.Exec(ctx, "comment on column dbcli_comment_dropped_test.name is 'kept column comment'"); err != nil {
		t.Fatalf("comment on column: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_comment_dropped_test")
	})

	desc, err := c.DescribeTable(ctx, "public", "dbcli_comment_dropped_test", driver.QueryOptions{})
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}

	if desc.Comment == nil || *desc.Comment != "a test table" {
		t.Fatalf("expected table comment \"a test table\", got %+v", desc.Comment)
	}

	var nameCol *driver.ColumnInfo
	for i := range desc.Columns {
		if desc.Columns[i].Name == "name" {
			nameCol = &desc.Columns[i]
		}
	}
	if nameCol == nil {
		t.Fatalf("expected a \"name\" column, got %+v", desc.Columns)
	}
	if nameCol.Comment == nil || *nameCol.Comment != "kept column comment" {
		t.Fatalf("expected name column comment \"kept column comment\" (not the dropped column's), got %+v", nameCol.Comment)
	}
}

func TestDescribeTable_ReturnsEnumValuesForEnumColumn(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.pgxConn.Exec(ctx, "create type dbcli_enum_test_status as enum ('pending', 'shipped', 'cancelled')"); err != nil {
		t.Fatalf("create type: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_enum_col_test")
		c.pgxConn.Exec(context.Background(), "drop type if exists dbcli_enum_test_status")
	})
	if _, err := c.pgxConn.Exec(ctx, "create table dbcli_enum_col_test (id serial primary key, status dbcli_enum_test_status)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	desc, err := c.DescribeTable(ctx, "public", "dbcli_enum_col_test", driver.QueryOptions{})
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}

	var statusCol *driver.ColumnInfo
	for i := range desc.Columns {
		if desc.Columns[i].Name == "status" {
			statusCol = &desc.Columns[i]
		}
	}
	if statusCol == nil {
		t.Fatalf("expected a \"status\" column, got %+v", desc.Columns)
	}
	want := []string{"pending", "shipped", "cancelled"}
	if len(statusCol.EnumValues) != len(want) {
		t.Fatalf("got enum_values %v, want %v", statusCol.EnumValues, want)
	}
	for i, v := range want {
		if statusCol.EnumValues[i] != v {
			t.Fatalf("got enum_values %v, want %v", statusCol.EnumValues, want)
		}
	}
}

func TestDescribeTable_ReturnsViewDefinitionForView(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	setup := `
		create table if not exists dbcli_view_base_test (id serial primary key, name text);
		create or replace view dbcli_view_test as select id, name from dbcli_view_base_test;
	`
	if _, err := c.pgxConn.Exec(ctx, setup); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop view if exists dbcli_view_test")
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_view_base_test")
	})

	viewDesc, err := c.DescribeTable(ctx, "public", "dbcli_view_test", driver.QueryOptions{})
	if err != nil {
		t.Fatalf("DescribeTable(view): %v", err)
	}
	if viewDesc.ViewDefinition == nil || !strings.Contains(*viewDesc.ViewDefinition, "dbcli_view_base_test") {
		t.Fatalf("expected view_definition referencing dbcli_view_base_test, got %+v", viewDesc.ViewDefinition)
	}

	tableDesc, err := c.DescribeTable(ctx, "public", "dbcli_view_base_test", driver.QueryOptions{})
	if err != nil {
		t.Fatalf("DescribeTable(table): %v", err)
	}
	if tableDesc.ViewDefinition != nil {
		t.Fatalf("expected view_definition to be nil for an ordinary table, got %+v", *tableDesc.ViewDefinition)
	}
}

func TestListSchema_IncludesPartitionedTable(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	setup := `
		create table if not exists dbcli_partitioned_test (id int, created_at date) partition by range (created_at);
		create table if not exists dbcli_partitioned_test_p1 partition of dbcli_partitioned_test
			for values from ('2020-01-01') to ('2021-01-01');
	`
	if _, err := c.pgxConn.Exec(ctx, setup); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_partitioned_test")
	})

	tables, err := c.ListSchema(ctx, "public", driver.QueryOptions{})
	if err != nil {
		t.Fatalf("ListSchema: %v", err)
	}

	var found *driver.TableInfo
	for i := range tables {
		if tables[i].Name == "dbcli_partitioned_test" {
			found = &tables[i]
		}
	}
	if found == nil {
		t.Fatalf("expected dbcli_partitioned_test in %+v (was being silently dropped by the old relkind filter)", tables)
	}
	if found.Kind != "partitioned_table" {
		t.Fatalf("expected kind \"partitioned_table\", got %q", found.Kind)
	}
}

func TestListSchema_IncludesForeignTable(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.pgxConn.Exec(ctx, "create table if not exists dbcli_fdw_target_test (id int)"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	if _, err := c.pgxConn.Exec(ctx, "create extension if not exists postgres_fdw"); err != nil {
		t.Skipf("postgres_fdw not available, skipping: %v", err)
	}
	setup := `
		create server if not exists dbcli_fdw_test_server
			foreign data wrapper postgres_fdw
			options (host 'localhost', dbname 'postgres', port '5432');
		create user mapping if not exists for current_user
			server dbcli_fdw_test_server
			options (user 'postgres', password 'postgres');
		create foreign table if not exists dbcli_foreign_table_test (id int)
			server dbcli_fdw_test_server
			options (schema_name 'public', table_name 'dbcli_fdw_target_test');
	`
	if _, err := c.pgxConn.Exec(ctx, setup); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop foreign table if exists dbcli_foreign_table_test")
		c.pgxConn.Exec(context.Background(), "drop server if exists dbcli_fdw_test_server cascade")
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_fdw_target_test")
	})

	tables, err := c.ListSchema(ctx, "public", driver.QueryOptions{})
	if err != nil {
		t.Fatalf("ListSchema: %v", err)
	}

	var found *driver.TableInfo
	for i := range tables {
		if tables[i].Name == "dbcli_foreign_table_test" {
			found = &tables[i]
		}
	}
	if found == nil {
		t.Fatalf("expected dbcli_foreign_table_test in %+v (was being silently dropped by the old relkind filter)", tables)
	}
	if found.Kind != "foreign_table" {
		t.Fatalf("expected kind \"foreign_table\", got %q", found.Kind)
	}
}

// TestListSchema_StatsKnownReflectsAnalyzeState covers the stats-known
// signal: a freshly created table has never been vacuumed or analyzed, so
// stats_known must be false (estimated_rows is meaningless, not "confirmed
// empty"). After an explicit ANALYZE, stats_known flips to true and
// estimated_rows reflects the real count. This is deliberately checked via
// pg_stat_user_tables' last_vacuum/last_(auto)analyze columns rather than
// reltuples' sign, since the reltuples == -1 "never analyzed" sentinel is
// PG14+ only; pg_stat_user_tables' columns are accurate back to PG 8.3.
func TestListSchema_StatsKnownReflectsAnalyzeState(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.pgxConn.Exec(ctx, "create table if not exists dbcli_stats_test (id serial primary key)"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_stats_test")
	})
	for i := 0; i < 3; i++ {
		if _, err := c.pgxConn.Exec(ctx, "insert into dbcli_stats_test default values"); err != nil {
			t.Fatalf("test fixture insert: %v", err)
		}
	}

	findTable := func() driver.TableInfo {
		tables, err := c.ListSchema(ctx, "public", driver.QueryOptions{})
		if err != nil {
			t.Fatalf("ListSchema: %v", err)
		}
		for _, tbl := range tables {
			if tbl.Name == "dbcli_stats_test" {
				return tbl
			}
		}
		t.Fatalf("expected dbcli_stats_test in %+v", tables)
		return driver.TableInfo{}
	}

	before := findTable()
	if before.StatsKnown {
		t.Fatalf("expected stats_known=false before any ANALYZE, got %+v", before)
	}

	if _, err := c.pgxConn.Exec(ctx, "analyze dbcli_stats_test"); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	// pg_stat_user_tables is fed by the stats collector asynchronously on
	// older Postgres versions (pre-PG15's shared-memory stats), so
	// last_analyze can lag a moment behind the ANALYZE that set it. Poll
	// with a bounded timeout instead of checking once.
	deadline := time.Now().Add(5 * time.Second)
	var after driver.TableInfo
	for {
		after = findTable()
		if after.StatsKnown {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected stats_known=true after ANALYZE (waited 5s for the stats collector), got %+v", after)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if after.EstimatedRows != 3 {
		t.Fatalf("expected estimated_rows=3 after ANALYZE, got %d", after.EstimatedRows)
	}
}
