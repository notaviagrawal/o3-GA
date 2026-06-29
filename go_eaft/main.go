package main

import (
	"ga_tuner/scripts"
	"ga_tuner/utils"
	"os"
)

func init() {
	utils.Initialization(os.Args[1])
}

func main() {

	// TOOD : Change this side to dynamic. User should have different options for finding optimal
	// Solution for their code. For Example : GA or Particle Swarm or Differential Evolution or OpenAI.
	// Use TERMUI for making Terminal interface. Plotting results and other details will be in there.
	// Instantiate a GA with a GAConfig

	// TODO : Set initial run time of given code. Compare it with best of population.
	// Show improvement with percentage. Ex : Fitness Value : 3 sn. %34 less then normal.

	// Set the number of generations to run for
	// Args[1] => Folder name for results
	// Args[2] => GCC-11	
	// Args[3] => 2mm.json -> Result file.
	// Args[4] => GA or PSO


	// Example full run -> go run main.go 2mm gcc-11 2mm.json GA

	Runner := os.Args[4]
	if Runner == "GA" {
		go scripts.GARunner()
	} else if Runner == "PSO" {
		go scripts.PSORunner()
	}
	utils.CLI()
}
