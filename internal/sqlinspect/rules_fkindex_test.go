package sqlinspect

import (
	"testing"

	"github.com/BhuwanJoshi-01/prodshadow/internal/evidence"
)

func TestPS133ForeignKeyWithoutIndex(t *testing.T) {
	cases := []struct {
		sql  string
		want int
	}{
		{"CREATE TABLE refunds (id int PRIMARY KEY, order_id bigint REFERENCES orders (id));", 1},
		{"CREATE TABLE refunds (id int PRIMARY KEY, order_id bigint REFERENCES orders (id));\nCREATE INDEX refunds_order_idx ON refunds (order_id);", 0},
		{"CREATE TABLE refunds (order_id bigint REFERENCES orders (id), line int, PRIMARY KEY (order_id, line));", 0},
		{"CREATE TABLE lines (order_id bigint, line int, FOREIGN KEY (order_id, line) REFERENCES order_lines (order_id, line));\nCREATE INDEX ON lines (line, order_id, sku);", 0},
		{"CREATE TABLE lines (order_id bigint, line int, FOREIGN KEY (order_id, line) REFERENCES order_lines (order_id, line));\nCREATE INDEX ON lines (line);", 1},
		{"CREATE TABLE refunds (order_id bigint REFERENCES orders (id), note text);\nCREATE INDEX ON refunds (lower(note), order_id);", 1},
		{"ALTER TABLE refunds ADD CONSTRAINT refunds_order_fk FOREIGN KEY (order_id) REFERENCES orders (id) NOT VALID;", 1},
		{"CREATE INDEX CONCURRENTLY refunds_order_idx ON refunds (order_id);\nALTER TABLE refunds ADD CONSTRAINT refunds_order_fk FOREIGN KEY (order_id) REFERENCES orders (id) NOT VALID;", 0},
		{"ALTER TABLE refunds ADD COLUMN order_id bigint REFERENCES orders (id);", 1},
		{"CREATE TABLE notes (id int PRIMARY KEY, body text);", 0},
	}
	for _, tc := range cases {
		if got := findings(t, "PS133", Options{}, tc.sql); len(got) != tc.want {
			t.Errorf("%q: expected %d findings, got %d: %+v", tc.sql, tc.want, len(got), got)
		}
	}
	got := findings(t, "PS133", Options{}, "ALTER TABLE refunds ADD CONSTRAINT fk FOREIGN KEY (order_id) REFERENCES orders (id) NOT VALID;")
	if got[0].Confidence != evidence.Low {
		t.Errorf("an existing table may already have the index: confidence %s", got[0].Confidence)
	}
	got = findings(t, "PS133", Options{}, "CREATE TABLE refunds (order_id bigint REFERENCES orders (id));")
	if got[0].Confidence != evidence.Medium || got[0].Table != "refunds" {
		t.Errorf("a new table cannot have the index yet: %+v", got[0])
	}
}
