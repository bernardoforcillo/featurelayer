package dropsstore

import (
	"context"

	"github.com/bernardoforcillo/drops"
	"github.com/bernardoforcillo/drops/pg"
)

// createTable issues CREATE TABLE IF NOT EXISTS for t followed by its
// composite primary key, which drops' CREATE TABLE does not carry (its
// doc scopes it to single-column keys). Every statement is idempotent:
// the ALTER TABLE is wrapped in a plpgsql block that swallows the
// SQLSTATEs PostgreSQL raises when the constraint is already there —
// 42P07 duplicate_table (the key's backing index name is taken), 42710
// duplicate_object and 42P16 invalid_table_definition (the table
// already has a primary key) — since PostgreSQL has no ADD CONSTRAINT
// IF NOT EXISTS. The block adds a missing key and never alters one that
// exists, which keeps CreateSchema honest: it adds, it does not migrate.
func createTable(ctx context.Context, db *pg.DB, t *pg.Table) error {
	if _, err := db.ExecExpr(ctx, pg.CreateTableIfNotExists(t)); err != nil {
		return err
	}
	if pk := t.CompositePrimaryKey(); len(pk) > 0 {
		if _, err := db.ExecExpr(ctx, addPrimaryKey(t, pk)); err != nil {
			return err
		}
	}
	return nil
}

// addPrimaryKey renders one idempotent ALTER TABLE ... ADD CONSTRAINT
// <table>_pkey PRIMARY KEY (cols). Identifiers go through the builder,
// so a table name from WithNames is quoted, never interpolated.
func addPrimaryKey(t *pg.Table, cols []*pg.Column) drops.Expression {
	return drops.ExprFunc(func(b *drops.Builder) {
		b.WriteString("DO $featurelayer$\nBEGIN\n  ALTER TABLE ")
		b.WriteQualified(t.Schema(), t.Name())
		b.WriteString(" ADD CONSTRAINT ")
		b.WriteIdent(t.Name() + "_pkey")
		b.WriteString(" PRIMARY KEY (")
		for i, c := range cols {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteIdent(c.Name())
		}
		b.WriteString(");\nEXCEPTION\n  WHEN duplicate_table OR duplicate_object OR " +
			"invalid_table_definition THEN NULL;\nEND;\n$featurelayer$")
	})
}
