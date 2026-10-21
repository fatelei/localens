package main

import (
	"context"
	"flag"
	"fmt"
	"localens/internal/catalog"
	"log"
)

func main() {
	path := flag.String("db", "", "Localens index database (desktop must be stopped)")
	flag.Parse()
	if *path == "" {
		log.Fatal("-db is required")
	}
	fmt.Println("Migrating duplicate chunk storage; preserving content and vectors…")
	store, err := catalog.Open(*path)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	fmt.Println("Reclaiming unused SQLite pages…")
	if err := store.Compact(context.Background()); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Index optimization complete")
}
