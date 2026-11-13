package fixture

import (
	"strings"
	"testing"
)

func TestCheckFixtureStatement(t *testing.T) {
	allowed := []string{
		"CREATE TABLE orders (id bigint PRIMARY KEY)",
		"INSERT INTO orders SELECT g FROM generate_series(1, 1000) g",
		"COPY orders FROM STDIN",
		"create index on orders (id)",
	}
	for _, sql := range allowed {
		if err := checkFixtureStatement(sql); err != nil {
			t.Errorf("%q rejected: %v", sql, err)
		}
	}
	denied := []string{
		"ALTER SYSTEM SET fsync = off",
		"create  role admin superuser",
		"CREATE EXTENSION dblink",
		"COPY orders FROM '/etc/passwd'",
		"COPY orders TO PROGRAM 'curl http://x'",
		"LOAD 'evil'",
		"DROP DATABASE postgres",
	}
	for _, sql := range denied {
		if err := checkFixtureStatement(sql); err == nil {
			t.Errorf("%q should be rejected", sql)
		}
	}
}

func TestHashDistinguishesInputs(t *testing.T) {
	mk := func(schema, seed string, s int64) string {
		h := NewHash()
		h.Add("schema", []byte(schema))
		h.Add("seed", []byte(seed))
		h.AddSeed(s)
		return h.String()
	}
	a := mk("CREATE TABLE t (id int);", "", 1)
	if a != mk("CREATE TABLE t (id int);", "", 1) {
		t.Fatal("hash is not deterministic")
	}
	if a == mk("", "CREATE TABLE t (id int);", 1) || a == mk("CREATE TABLE t (id int);", "", 2) {
		t.Fatal("hash does not distinguish inputs")
	}
	if !strings.HasPrefix(a, "sha256:") || len(a) != 23 {
		t.Fatalf("unexpected format %s", a)
	}
}
