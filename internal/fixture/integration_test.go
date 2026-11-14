//go:build integration

package fixture

import (
	"context"
	"testing"

	"github.com/BhuwanJoshi-01/Open-Source-1/internal/isolation"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/pgtest"
	"github.com/BhuwanJoshi-01/Open-Source-1/internal/sqlinspect"

	"go.yaml.in/yaml/v3"
)

const allTypes = `version: 1
tables:
  - name: items
    rows: 2000
    columns:
      - {name: id, generator: sequence}
      - {name: ref, generator: uuid}
      - {name: label, generator: pattern, format: "item-{n}"}
      - {name: note, generator: text, min_length: 0, max_length: 40, null_ratio: 0.3}
      - {name: price, generator: float_range, min: 1, max: 100}
      - {name: weight, generator: float_range, min: 0, max: 5}
      - {name: active, generator: bool, true_ratio: 0.8}
      - {name: kind, generator: choice, values: [a, b, c]}
      - {name: owner_id, generator: zipf, min: 1, max: 100, s: 1.3}
      - {name: created_at, generator: timestamp_range, from: "2025-01-01T00:00:00Z", to: "2025-12-31T00:00:00Z"}
`

func TestGenerateAllColumnTypes(t *testing.T) {
	ctx := context.Background()
	_, srv := pgtest.Start(t)
	if err := srv.Bootstrap(ctx, nil); err != nil {
		t.Fatal(err)
	}
	admin, _ := srv.Connect(ctx, srv.Admin(), isolation.ConnOptions{})
	if _, err := admin.Exec(ctx, "CREATE DATABASE fx OWNER ps_app"); err != nil {
		t.Fatal(err)
	}
	app, _ := srv.App()
	conn, err := srv.Connect(ctx, app, isolation.ConnOptions{Database: "fx"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	schema := sqlinspect.Source{Path: "schema.sql", Content: `CREATE TABLE items (
  id bigint PRIMARY KEY, ref uuid NOT NULL UNIQUE, label text NOT NULL, note text,
  price numeric(10,2) NOT NULL, weight double precision NOT NULL, active boolean NOT NULL,
  kind text NOT NULL, owner_id integer NOT NULL, created_at timestamptz NOT NULL);`}
	if n, err := ApplySQL(ctx, conn, schema); err != nil || n != 1 {
		t.Fatalf("apply schema: %d %v", n, err)
	}
	if _, err := ApplySQL(ctx, conn, sqlinspect.Source{Path: "bad.sql", Content: "SELECT 1;\nCOPY items TO '/tmp/x';"}); err == nil {
		t.Fatal("server-side COPY must be rejected before execution")
	}

	var gf GeneratorFile
	if err := yaml.Unmarshal([]byte(allTypes), &gf); err != nil {
		t.Fatal(err)
	}
	res, err := Generate(ctx, conn, &gf, 7)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Rows != 2000 {
		t.Fatalf("rows = %d", res[0].Rows)
	}
	var nulls, distinctRefs int
	var minOwner int
	if err := conn.QueryRow(ctx, "SELECT count(*) FILTER (WHERE note IS NULL), count(DISTINCT ref), min(owner_id) FROM items").Scan(&nulls, &distinctRefs, &minOwner); err != nil {
		t.Fatal(err)
	}
	if nulls < 450 || nulls > 750 || distinctRefs != 2000 || minOwner != 1 {
		t.Fatalf("unexpected data: nulls=%d refs=%d minOwner=%d", nulls, distinctRefs, minOwner)
	}

	// The same seed must reproduce identical data.
	var before string
	if err := conn.QueryRow(ctx, "SELECT md5(string_agg(items::text, ',' ORDER BY id)) FROM items").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "TRUNCATE items"); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(ctx, conn, &gf, 7); err != nil {
		t.Fatal(err)
	}
	var after string
	if err := conn.QueryRow(ctx, "SELECT md5(string_agg(items::text, ',' ORDER BY id)) FROM items").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("generated data is not deterministic")
	}
}
