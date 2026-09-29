package postgres

import "testing"

func TestValidateQuery_Allowed(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"simple select", "select 1", "select 1"},
		{"trailing semicolon stripped", "select 1;", "select 1"},
		{"trailing semicolons and whitespace stripped", "select 1;  ;\n", "select 1"},
		{"with cte", "with x as (select 1) select * from x", "with x as (select 1) select * from x"},
		{"explain", "explain select 1", "explain select 1"},
		{"table shorthand", "table vehicles", "table vehicles"},
		{"leading whitespace", "  \n select 1", "select 1"},
		{"line comment stripped", "select 1 -- trailing note\n", "select 1"},
		{"block comment stripped", "select /* mid */ 1", "select   1"},
		{"semicolon inside single-quoted string is not a statement separator", "select 'a;b'", "select 'a;b'"},
		{"escaped quote inside string", "select 'it''s fine'", "select 'it''s fine'"},
		{"semicolon inside dollar-quoted string", "select $$a;b$$", "select $$a;b$$"},
		{"tagged dollar-quote", "select $tag$a;b$tag$", "select $tag$a;b$tag$"},
		{"semicolon inside double-quoted identifier", `select "weird;col" from t`, `select "weird;col" from t`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateQuery(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateQuery_Rejected(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", "   "},
		{"insert", "insert into t values (1)"},
		{"update", "update t set x = 1"},
		{"delete", "delete from t"},
		{"drop table", "drop table t"},
		{"create table", "create table t (id int)"},
		{"multi statement select then delete", "select 1; delete from t"},
		{"multi statement two selects", "select 1; select 2"},
		{"comment-hidden second statement", "select 1; -- comment\ndelete from t"},
		{"trailing content after comment is still a second statement", "select 1 /* x */; drop table t"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ValidateQuery(tc.in); err == nil {
				t.Fatalf("expected error for %q, got none", tc.in)
			}
		})
	}
}

func TestWrapWithLimit(t *testing.T) {
	got := WrapWithLimit("select * from t", 500)
	want := "SELECT * FROM (select * from t) AS dbcli_subquery LIMIT 500"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
