package main

import (
	"fmt"
	"ga_tuner/scripts"
	"ga_tuner/utils"
	"os"
)

func main() {
	if len(os.Args) < 5 {
		fmt.Println("usage: go run ./cmd/headless <benchmark> <compiler> <log.json> <GA|PSO>")
		os.Exit(1)
	}

	utils.Initialization(os.Args[1])
	workers := os.Getenv("EAFT_WORKERS")
	if workers == "" {
		workers = "unbounded"
	}
	fmt.Printf("EAFT_WORKERS=%s\n", workers)
	correctness := os.Getenv("EAFT_CORRECTNESS")
	if correctness == "" {
		correctness = "off"
	}
	fmt.Printf("EAFT_CORRECTNESS=%s\n", correctness)
	switch os.Args[4] {
	case "GA":
		scripts.GARunner()
	case "PSO":
		scripts.PSORunner()
	default:
		fmt.Printf("unknown runner %q, expected GA or PSO\n", os.Args[4])
		os.Exit(1)
	}
}
