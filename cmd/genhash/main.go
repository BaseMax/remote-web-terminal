// cmd/genhash/main.go
// A small utility to generate bcrypt password hashes for config.json
//
// Usage:
//
//	go run ./cmd/genhash -password yourpassword
//
// Copy the output into config.json as "password_hash".
package main

import (
	"flag"
	"fmt"
	"log"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	pw := flag.String("password", "", "plaintext password to hash")
	cost := flag.Int("cost", 12, "bcrypt cost factor (10-14 recommended)")
	flag.Parse()

	if *pw == "" {
		log.Fatal("provide -password flag")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(*pw), *cost)
	if err != nil {
		log.Fatalf("bcrypt error: %v", err)
	}

	fmt.Println(string(hash))
}
