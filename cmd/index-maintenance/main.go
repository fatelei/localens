package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"localens/internal/catalog"
	"log"
	"os"
	"os/signal"
	"time"
)

func main() {
	path := flag.String("db", "", "Localens index database (desktop must be stopped for writes)")
	analyze := flag.Bool("analyze", false, "Read-only shared-fragment storage estimate; no migrations or compaction")
	deduplicate := flag.Bool("deduplicate", false, "Migrate text and lexical postings to shared fragments (back up first)")
	verify := flag.String("verify-against", "", "Compare every file body, metadata and semantic chunk with an offline source database")
	flag.Parse()
	if *path == "" {
		log.Fatal("-db is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *analyze {
		result, err := catalog.AnalyzeDatabase(ctx, *path)
		if err != nil {
			log.Fatal(err)
		}
		if err = json.NewEncoder(os.Stdout).Encode(result); err != nil {
			log.Fatal(err)
		}
		return
	}
	started := time.Now()
	store, err := catalog.OpenForMaintenance(*path)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	if *deduplicate {
		fmt.Println("Sharing identical text fragments and lexical postings; preserving file identities and vectors…")
		if err = store.DeduplicateContent(ctx); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Println("Reclaiming unused SQLite pages…")
	if err = store.Compact(ctx); err != nil {
		log.Fatal(err)
	}
	if *verify != "" {
		fmt.Println("Verifying every body, file identity and semantic chunk against source…")
		if err = store.VerifyContentAgainst(ctx, *verify); err != nil {
			log.Fatal(err)
		}
		if err = store.CompareContentSearch(ctx, *verify); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Println("Index optimization complete in", time.Since(started))
}
