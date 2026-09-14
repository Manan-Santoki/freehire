package main

import (
	"strings"
	"testing"
)

func TestReadCatalogRejectsInvalidInputBeforeImport(t *testing.T) {
	for _, input := range []string{
		`[]`,
		`null`,
		`[{"provider":"greenhouse","board":"acme","company":"Acme","regoin":"us"}]`,
		`[{"provider":"does-not-exist","board":"acme","company":"Acme"}]`,
		`[{"provider":"greenhouse","company":"Acme"}]`,
		`[{"provider":"remoteok","company":"RemoteOK","SubmittedBy":42}]`,
		`[{"provider":"remoteok","company":"RemoteOK"}] {}`,
		`[{"provider":"remoteok","company":"RemoteOK"}] garbage`,
	} {
		if _, err := readCatalog(strings.NewReader(input)); err == nil {
			t.Errorf("accepted invalid catalog: %s", input)
		}
	}
}

func TestReadCatalogPreservesBoardMetadata(t *testing.T) {
	entries, err := readCatalog(strings.NewReader(`[{"provider":"greenhouse","board":" acme ","company":"Acme","region":"eu","hub":true,"tenants":{"division":"Acme Engineering"}},{"provider":"remoteok","company":"RemoteOK"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Board != "acme" || entries[0].Region != "eu" || !entries[0].Hub || entries[0].Tenants["division"] != "Acme Engineering" || entries[1].Board != "" {
		t.Fatalf("lost catalog metadata: %+v", entries)
	}
}
