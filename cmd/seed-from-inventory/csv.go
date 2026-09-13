package main

import (
	"encoding/csv"
	"fmt"
	"io"
)

// inventoryRow is one row of an ATS-company inventory CSV: a company name, the source
// inventory's own slug (unused — freehire derives its own board id from url), and the
// company's public careers URL.
type inventoryRow struct {
	Name string
	Slug string
	URL  string
}

// requiredInventoryColumns are the header columns parseInventory must find. Order in the
// input file does not matter; each is looked up by name.
var requiredInventoryColumns = []string{"name", "slug", "url"}

// parseInventory reads a `name,slug,url` CSV into rows. It returns an error — and no rows
// — when the header is missing a required column or the body is not valid CSV (inconsistent
// field count, unterminated quote).
func parseInventory(r io.Reader) ([]inventoryRow, error) {
	cr := csv.NewReader(r)
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	col := make(map[string]int, len(header))
	for i, name := range header {
		col[name] = i
	}
	for _, name := range requiredInventoryColumns {
		if _, ok := col[name]; !ok {
			return nil, fmt.Errorf("missing required column %q", name)
		}
	}

	var rows []inventoryRow
	for {
		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse row: %w", err)
		}
		rows = append(rows, inventoryRow{
			Name: record[col["name"]],
			Slug: record[col["slug"]],
			URL:  record[col["url"]],
		})
	}
	return rows, nil
}
