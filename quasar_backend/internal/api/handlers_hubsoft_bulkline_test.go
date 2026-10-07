package api

import (
	"encoding/json"
	"testing"
)

func TestBulkRowLine(t *testing.T) {
	cases := []struct {
		raw  string
		idx  int
		want int
	}{
		{`{"line": 7}`, 0, 7},
		{`{"line": "7"}`, 0, 7}, // o front chegou a mandar texto — antes virava 0 em silêncio
		{`{}`, 3, 5},
		{`{"line": "abc"}`, 0, 2},
		{`{"line": 0}`, 1, 3},
	}
	for _, c := range cases {
		if got := bulkRowLine(json.RawMessage(c.raw), c.idx); got != c.want {
			t.Errorf("bulkRowLine(%s,%d) = %d, want %d", c.raw, c.idx, got, c.want)
		}
	}
}
