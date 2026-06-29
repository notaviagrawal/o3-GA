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
