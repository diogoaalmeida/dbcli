package postgres

import (
	"context"
	"testing"
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
