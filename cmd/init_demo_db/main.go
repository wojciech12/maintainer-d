package main

import (
	"log"
	"maintainerd/db"
)

func main() {
	// Initialize a demo database without external dependencies
	// This creates the schema but doesn't seed with external data
	dbPath := "maintainers.db"
	
	// Call BootstrapSQLite with empty strings for spreadsheet/credentials and seed=false
	// This will create the schema only
	_, err := db.BootstrapSQLite(dbPath, "", "", "", false)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	
	log.Printf("Demo database initialized successfully at %s", dbPath)
	log.Println("Schema created. No seed data loaded.")
}
