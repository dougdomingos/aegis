package main

import (
	"fmt"
	"log"
	"net/http"

	"dougdomingos.com/aegis/internal/api"
	"dougdomingos.com/aegis/internal/infra"
)

func main() {
	db, err := infra.InitDB("/tmp/aegis.db")
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}

	fmt.Println("starting server on port :8080...")

	router := api.NewRouter(db)
	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatalf("application failed: %v", err)
	}
}
