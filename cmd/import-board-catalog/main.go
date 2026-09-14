// Command import-board-catalog validates and imports a JSON board catalog.
// It reports by default; --apply inserts pending boards through boardcatalog.Inserter.
// Existing live boards are preserved, making an interrupted import safe to resume.
package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func readCatalog(r io.Reader) ([]boardcatalog.InsertInput, error) {
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	var entries []boardcatalog.InsertInput
	if err := d.Decode(&entries); err != nil {
		return nil, err
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("expected one JSON array, found trailing data")
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("catalog is empty")
	}
	registry := sources.Taxonomy()
	for n := range entries {
		in := &entries[n]
		in.Board = strings.TrimSpace(in.Board)
		// An imported public catalog cannot attribute submissions to local accounts.
		if in.SubmittedBy != nil || (in.Surface != "" && in.Surface != "curator") {
			return nil, fmt.Errorf("entry %d: submission attribution is not supported", n+1)
		}
		if err := boardcatalog.Validate(*in, registry); err != nil {
			return nil, fmt.Errorf("entry %d: %w", n+1, err)
		}
	}
	return entries, nil
}

func run() int {
	apply := flag.Bool("apply", false, "insert pending boards; otherwise only validate")
	flag.Parse()
	if flag.NArg() != 1 {
		log.Print("usage: import-board-catalog [--apply] catalog.json[.gz]")
		return 2
	}
	f, err := os.Open(flag.Arg(0))
	if err != nil {
		log.Print(err)
		return 1
	}
	defer f.Close()
	var reader io.Reader = f
	if strings.HasSuffix(flag.Arg(0), ".gz") {
		z, err := gzip.NewReader(f)
		if err != nil {
			log.Print(err)
			return 1
		}
		defer z.Close()
		reader = z
	}
	entries, err := readCatalog(reader)
	if err != nil {
		log.Printf("catalog: %v", err)
		return 1
	}
	log.Printf("catalog: validated %d entries", len(entries))
	if !*apply {
		log.Print("dry run: pass --apply to insert")
		return 0
	}
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Print(err)
		return 1
	}
	defer cleanup()
	insert := boardcatalog.NewInserter(boardcatalog.NewQueriesRepository(db.New(pool)), sources.Taxonomy())
	added, duplicates := 0, 0
	for n, in := range entries {
		b, err := insert.Insert(ctx, in, boardcatalog.StatusPending)
		if errors.Is(err, boardcatalog.ErrDuplicateBoard) {
			duplicates++
		} else if err != nil {
			log.Printf("entry %d (%s/%s): %v; added=%d duplicates=%d", n+1, in.Provider, in.Board, err, added, duplicates)
			return 1
		} else if b.Status == boardcatalog.StatusRejected {
			log.Printf("entry %d rejected: %s", n+1, b.RejectedReason)
			return 1
		} else {
			added++
		}
		if (n+1)%1000 == 0 {
			log.Printf("catalog: processed=%d/%d added=%d duplicates=%d", n+1, len(entries), added, duplicates)
		}
	}
	log.Printf("catalog complete: added=%d duplicates=%d", added, duplicates)
	return 0
}
