package sqlinspect

import "testing"

func TestPS152IntPrimaryKey(t *testing.T) {
	cases := map[string]int{
		"CREATE TABLE orders (id serial PRIMARY KEY, note text);":                    1,
		"CREATE TABLE orders (id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY);": 1,
		"CREATE TABLE orders (id int, note text, PRIMARY KEY (id));":                 1,
		"CREATE TABLE flags (id smallint PRIMARY KEY);":                              1,
		"CREATE TABLE orders (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY);":  0,
		"CREATE TABLE orders (id bigserial PRIMARY KEY);":                            0,
		"CREATE TABLE orders (id uuid PRIMARY KEY);":                                 0,
		"CREATE TABLE lines (order_id int, line int, PRIMARY KEY (order_id, line));": 0,
		"CREATE TEMPORARY TABLE scratch (id int PRIMARY KEY);":                       0,
	}
	for sql, want := range cases {
		if got := findings(t, "PS152", Options{}, sql); len(got) != want {
			t.Errorf("%s: expected %d findings, got %d", sql, want, len(got))
		}
	}
}
