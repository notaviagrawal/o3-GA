package main

import (
	"fmt"
	"ga_tuner/scripts"
	"ga_tuner/utils"
	"os"
)

func main() {
	if len(os.Args) < 5 {
		fmt.Println("usage: go run ./cmd/headless <benchmark> <compiler> <log.json> <GA|PSO|LTGOMEA>")
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
	safeFlags := os.Getenv("EAFT_LOCK_SAFE_FLAGS")
	if safeFlags == "" {
		safeFlags = "off"
	}
	fmt.Printf("EAFT_LOCK_SAFE_FLAGS=%s\n", safeFlags)
	penalty := os.Getenv("EAFT_INCORRECT_PENALTY")
	if penalty == "" {
		penalty = "100"
	}
	fmt.Printf("EAFT_INCORRECT_PENALTY=%s\n", penalty)
	dataset := os.Getenv("EAFT_DATASET")
	if dataset == "" {
		dataset = "polybench_default"
	}
	fmt.Printf("EAFT_DATASET=%s\n", dataset)
	earlyStopPatience := os.Getenv("LTGOMEA_EARLY_STOP_PATIENCE")
	if earlyStopPatience == "" {
		earlyStopPatience = "off"
	}
	fmt.Printf("LTGOMEA_EARLY_STOP_PATIENCE=%s\n", earlyStopPatience)
	earlyStopMinDelta := os.Getenv("LTGOMEA_EARLY_STOP_MIN_DELTA")
	if earlyStopMinDelta == "" {
		earlyStopMinDelta = "0.005"
	}
	fmt.Printf("LTGOMEA_EARLY_STOP_MIN_DELTA=%s\n", earlyStopMinDelta)
	warmStart := os.Getenv("LTGOMEA_WARM_START")
	if warmStart == "" {
		warmStart = "on"
	}
	fmt.Printf("LTGOMEA_WARM_START=%s\n", warmStart)
	switch os.Args[4] {
	case "GA":
		scripts.GARunner()
	case "PSO":
		scripts.PSORunner()
	case "LTGOMEA":
		scripts.LTGOMEARunner()
	default:
		fmt.Printf("unknown runner %q, expected GA, PSO, or LTGOMEA\n", os.Args[4])
		os.Exit(1)
	}
}
