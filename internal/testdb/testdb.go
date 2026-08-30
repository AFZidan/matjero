package testdb

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"dropshipping/packages/database"
)

var nonIdentifier = regexp.MustCompile(`[^a-z0-9_]+`)

func Open(t testing.TB, dsn string) *database.Pool {
	t.Helper()

	ctx := context.Background()

	adminCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse database url: %v", err)
	}

	adminPool, err := pgxpool.NewWithConfig(ctx, adminCfg)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}

	schema := schemaName(t.Name())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+quotedSchema); err != nil {
		adminPool.Close()
		t.Fatalf("create isolated schema %s: %v", schema, err)
	}

	if _, err := adminPool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pgcrypto`); err != nil {
		adminPool.Close()
		t.Fatalf("ensure pgcrypto extension: %v", err)
	}

	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		if _, err := adminPool.Exec(ctx, `DROP SCHEMA IF EXISTS `+quotedSchema+` CASCADE`); err != nil {
			t.Logf("drop isolated schema %s: %v", schema, err)
		}
		adminPool.Close()
	})

	isolatedCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse database url for isolated pool: %v", err)
	}
	ensureRuntimeParams(isolatedCfg)
	isolatedCfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"

	pool, err = pgxpool.NewWithConfig(ctx, isolatedCfg)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}

	return &database.Pool{Pool: pool}
}

func schemaName(name string) string {
	base := strings.ToLower(name)
	base = nonIdentifier.ReplaceAllString(base, "_")
	base = strings.Trim(base, "_")
	if base == "" {
		base = "testdb"
	}
	return fmt.Sprintf("%s_%d", base, time.Now().UnixNano())
}

func ensureRuntimeParams(cfg *pgxpool.Config) {
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
}
