package main

import (
	"strings"
	"testing"
)

func TestParseInventoryReturnsRows(t *testing.T) {
	input := "name,slug,url\n" +
		"Acme,acme/careers,https://acme.wd1.myworkdayjobs.com/careers\n" +
		"Beta Inc,beta/jobs,https://boards.greenhouse.io/beta\n"

	rows, err := parseInventory(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseInventory: %v", err)
	}

	want := []inventoryRow{
		{Name: "Acme", Slug: "acme/careers", URL: "https://acme.wd1.myworkdayjobs.com/careers"},
		{Name: "Beta Inc", Slug: "beta/jobs", URL: "https://boards.greenhouse.io/beta"},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		if rows[i] != w {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], w)
		}
	}
}

func TestParseInventoryRejectsMissingURLColumn(t *testing.T) {
	input := "name,slug\nAcme,acme/careers\n"

	_, err := parseInventory(strings.NewReader(input))
	if err == nil {
		t.Fatal("parseInventory: want error for missing url column, got nil")
	}
}

func TestParseInventoryRejectsMalformedCSV(t *testing.T) {
	// The second row has three fields where the header declares two.
	input := "name,url\nAcme,https://acme.example.com,extra\n"

	_, err := parseInventory(strings.NewReader(input))
	if err == nil {
		t.Fatal("parseInventory: want error for inconsistent field count, got nil")
	}
}
