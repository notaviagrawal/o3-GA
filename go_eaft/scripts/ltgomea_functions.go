package scripts

import (
	"encoding/json"
	"fmt"
	"ga_tuner/utils"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	uuid "github.com/satori/go.uuid"
)

type ltIndividual struct {
	Vector  Vector
	Fitness float64
}

type mixingGroup struct {
	Indices []int
	Name    string
	Source  string
}

type mixingEvent struct {
	Accepted     bool     `json:"accepted"`
	ChangedFlags []string `json:"changed_flags"`
	Delta        *float64 `json:"delta_seconds,omitempty"`
	DonorIndex   int      `json:"donor_index"`
	Evaluation   uint64   `json:"evaluation"`
	Generation   int      `json:"generation"`
	GroupIndices []int    `json:"group_indices"`
	GroupName    string   `json:"group_name"`
	GroupSize    int      `json:"group_size"`
	GroupSource  string   `json:"group_source"`
	NewFitness   float64  `json:"new_fitness"`
	OldFitness   float64  `json:"old_fitness"`
	TargetIndex  int      `json:"target_index"`
	Timestamp    string   `json:"timestamp"`
}

type ltSummary struct {
	Algorithm       string   `json:"algorithm"`
	BestFlags       []string `json:"best_flags"`
	BestFitness     float64  `json:"best_fitness"`
	Budget          int      `json:"budget"`
	Evaluations     uint64   `json:"evaluations"`
	Generations     int      `json:"generations"`
	GroupTries      int      `json:"group_tries"`
	PopulationSize  int      `json:"population_size"`
	Seed            int64    `json:"seed"`
	SearchElapsedMs int64    `json:"search_elapsed_ms"`
}

func LTGOMEARunner() {
	start := time.Now()
	seed := envInt64("LTGOMEA_SEED", 42)
	rng := rand.New(rand.NewSource(seed))
	popSize := envInt("LTGOMEA_POP_SIZE", 40)
	generations := envInt("LTGOMEA_GENERATIONS", 50)
	budget := envInt("LTGOMEA_BUDGET", 1600)
	groupTries := envInt("LTGOMEA_GROUP_TRIES", 4)
	if budget < popSize {
		budget = popSize
	}

	levelTwo := CollectBaseline("O2")
	levelThree := CollectBaseline("O3")
	utils.Notifications = append(utils.Notifications,
		fmt.Sprintf("O2 : %f", levelTwo),
		fmt.Sprintf("O3 : %f", levelThree),
	)

	cache := map[string]float64{}
	population := make([]ltIndividual, popSize)
	best := ltIndividual{Fitness: math.Inf(1)}

	for i := range population {
		vector := randomLTVector(rng)
		fitness := evaluateLTVector(vector, cache)
		population[i] = ltIndividual{Vector: vector, Fitness: fitness}
		if fitness < best.Fitness {
			best = cloneLTIndividual(population[i])
		}
	}

	fmt.Printf(
		"ltgomea init pop=%d generations=%d budget=%d group_tries=%d best=%.6fs elapsed=%.1fs\n",
		popSize,
		generations,
		budget,
		groupTries,
		best.Fitness,
		time.Since(start).Seconds(),
	)

	for generation := 0; generation < generations && int(atomicEvalCount()) < budget; generation++ {
		groups := buildMixingGroups(population, rng)
		for i := range population {
			if int(atomicEvalCount()) >= budget {
				break
			}
			tries := 0
			for _, group := range groups {
				if tries >= groupTries || int(atomicEvalCount()) >= budget {
					break
				}
				donorIndex := rng.Intn(len(population))
				if donorIndex == i && len(population) > 1 {
					donorIndex = (donorIndex + 1) % len(population)
				}
				trial := cloneVector(population[i].Vector)
				changed := copyGroup(trial, population[donorIndex].Vector, group.Indices)
				if !changed {
					continue
				}
				tries++
				oldFitness := population[i].Fitness
				oldVector := cloneVector(population[i].Vector)
				changedFlags := changedFlagNames(oldVector, trial, group.Indices)
				fitness := evaluateLTVector(trial, cache)
				accepted := fitness <= oldFitness
				if accepted {
					population[i] = ltIndividual{Vector: trial, Fitness: fitness}
					if fitness < best.Fitness {
						best = cloneLTIndividual(population[i])
					}
				}
				writeMixingEvent(mixingEvent{
					Accepted:     accepted,
					ChangedFlags: changedFlags,
					Delta:        finiteDelta(oldFitness, fitness),
					DonorIndex:   donorIndex,
					Evaluation:   atomicEvalCount(),
					Generation:   generation + 1,
					GroupIndices: append([]int{}, group.Indices...),
					GroupName:    group.Name,
					GroupSize:    len(group.Indices),
					GroupSource:  group.Source,
					NewFitness:   fitness,
					OldFitness:   oldFitness,
					TargetIndex:  i,
					Timestamp:    time.Now().Format(time.RFC3339Nano),
				})
			}
		}
		sort.Slice(population, func(i, j int) bool { return population[i].Fitness < population[j].Fitness })
		fmt.Printf(
			"ltgomea_generation=%03d best=%.6fs pop_best=%.6fs evals=%d/%d groups=%d elapsed=%.1fs\n",
			generation+1,
			best.Fitness,
			population[0].Fitness,
			atomicEvalCount(),
			budget,
			len(groups),
			time.Since(start).Seconds(),
		)
	}

	bestFlags := vectorToFlagList(best.Vector)
	summary := ltSummary{
		Algorithm:       "LTGOMEA",
		BestFlags:       bestFlags,
		BestFitness:     best.Fitness,
		Budget:          budget,
		Evaluations:     atomicEvalCount(),
		Generations:     generations,
		GroupTries:      groupTries,
		PopulationSize:  popSize,
		Seed:            seed,
		SearchElapsedMs: time.Since(start).Milliseconds(),
	}
	payload, _ := json.MarshalIndent(summary, "", "  ")
	_ = os.WriteFile(filepath.Join(utils.ResultsPath, os.Args[1], "log", "ltgomea_summary.json"), payload, 0666)
	fmt.Printf("ltgomea complete best=%.6fs evals=%d elapsed=%.1fs\n", best.Fitness, atomicEvalCount(), time.Since(start).Seconds())
	fmt.Println("Best flags:", strings.Join(bestFlags, " "))
}

func evaluateLTVector(vector Vector, cache map[string]float64) float64 {
	key := vectorKey(vector)
	if fitness, ok := cache[key]; ok {
		return fitness
	}
	output := uuid.NewV4().String()
	cmd, _ := MatchBinaryWithFlags(vector, "O3")
	cmd = addPolybenchDependencies(cmd, os.Args[1], output)
	fitness := CompileCode(cmd, output, 3)
	cache[key] = fitness
	return fitness
}

func randomLTVector(rng *rand.Rand) Vector {
	return Vector(InitBinaryFloat64(50, 0, 2, rng))
}

func vectorKey(vector Vector) string {
	var b strings.Builder
	for _, value := range vector {
		if value >= 0.5 {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	return b.String()
}

func cloneVector(vector Vector) Vector {
	out := make(Vector, len(vector))
	copy(out, vector)
	return out
}

func cloneLTIndividual(individual ltIndividual) ltIndividual {
	return ltIndividual{Vector: cloneVector(individual.Vector), Fitness: individual.Fitness}
}

func copyGroup(target Vector, donor Vector, group []int) bool {
	changed := false
	for _, index := range group {
		if index < 0 || index >= len(target) || index >= len(donor) {
			continue
		}
		if target[index] != donor[index] {
			target[index] = donor[index]
			changed = true
		}
	}
	return changed
}

func vectorToFlagList(vector Vector) []string {
	flags := []string{"-O3"}
	for i, value := range vector {
		if i >= len(availableFlags) {
			break
		}
		if value >= 0.5 {
			flags = append(flags, "-f"+availableFlags[i])
		} else {
			flags = append(flags, "-fno-"+availableFlags[i])
		}
	}
	return flags
}

func buildMixingGroups(population []ltIndividual, rng *rand.Rand) []mixingGroup {
	dimensions := len(population[0].Vector)
	var groups []mixingGroup
	for i := 0; i < dimensions; i++ {
		groups = append(groups, mixingGroup{Indices: []int{i}, Name: flagName(i), Source: "single"})
	}
	groups = append(groups, seededFlagGroups(dimensions)...)
	groups = append(groups, learnedLinkageTree(population)...)
	groups = dedupeGroups(groups, dimensions)
	rng.Shuffle(len(groups), func(i, j int) {
		groups[i], groups[j] = groups[j], groups[i]
	})
	return groups
}

func seededFlagGroups(dimensions int) []mixingGroup {
	nameToIndex := map[string]int{}
	for i, name := range availableFlags {
		if i >= dimensions {
			break
		}
		nameToIndex[name] = i
	}
	namedGroups := []struct {
		name  string
		flags []string
	}{
		{"fast_math", []string{"fast-math", "associative-math", "finite-math-only", "cx-limited-range", "fp-int-builtin-inexact"}},
		{"gcse_cse", []string{"gcse", "gcse-after-reload", "gcse-las", "gcse-lm", "gcse-sm", "cse-follow-jumps", "cprop-registers"}},
		{"alignment", []string{"align-functions", "align-jumps", "align-labels", "align-loops"}},
		{"branch", []string{"branch-count-reg", "branch-probabilities", "guess-branch-probability"}},
		{"inline_devirt", []string{"early-inlining", "devirtualize", "devirtualize-speculatively"}},
		{"dead_code", []string{"dce", "dse", "delete-dead-exceptions", "delete-null-pointer-checks"}},
		{"loop_control", []string{"finite-loops", "aggressive-loop-optimizations", "delayed-branch"}},
	}
	var groups []mixingGroup
	for _, namedGroup := range namedGroups {
		var group []int
		for _, name := range namedGroup.flags {
			if index, ok := nameToIndex[name]; ok {
				group = append(group, index)
			}
		}
		if len(group) > 1 {
			sort.Ints(group)
			groups = append(groups, mixingGroup{Indices: group, Name: namedGroup.name, Source: "seeded"})
		}
	}
	return groups
}

func learnedLinkageTree(population []ltIndividual) []mixingGroup {
	dimensions := len(population[0].Vector)
	if dimensions <= 1 {
		return nil
	}
	mi := mutualInformationMatrix(population, dimensions)
	clusters := make([][]int, dimensions)
	for i := 0; i < dimensions; i++ {
		clusters[i] = []int{i}
	}
	var groups []mixingGroup
	mergeID := 0
	for len(clusters) > 1 {
		bestI, bestJ := 0, 1
		bestScore := math.Inf(-1)
		for i := 0; i < len(clusters); i++ {
			for j := i + 1; j < len(clusters); j++ {
				score := averageLinkage(mi, clusters[i], clusters[j])
				if score > bestScore {
					bestScore = score
					bestI, bestJ = i, j
				}
			}
		}
		merged := append([]int{}, clusters[bestI]...)
		merged = append(merged, clusters[bestJ]...)
		sort.Ints(merged)
		if len(merged) > 1 && len(merged) < dimensions {
			mergeID++
			groups = append(groups, mixingGroup{
				Indices: merged,
				Name:    fmt.Sprintf("learned_%03d", mergeID),
				Source:  "learned",
			})
		}
		clusters[bestI] = merged
		clusters = append(clusters[:bestJ], clusters[bestJ+1:]...)
	}
	return groups
}

func mutualInformationMatrix(population []ltIndividual, dimensions int) [][]float64 {
	matrix := make([][]float64, dimensions)
	for i := range matrix {
		matrix[i] = make([]float64, dimensions)
	}
	for i := 0; i < dimensions; i++ {
		for j := i + 1; j < dimensions; j++ {
			value := pairMutualInformation(population, i, j)
			matrix[i][j] = value
			matrix[j][i] = value
		}
	}
	return matrix
}

func pairMutualInformation(population []ltIndividual, a int, b int) float64 {
	var counts [2][2]float64
	for _, individual := range population {
		x := 0
		y := 0
		if individual.Vector[a] >= 0.5 {
			x = 1
		}
		if individual.Vector[b] >= 0.5 {
			y = 1
		}
		counts[x][y]++
	}
	total := float64(len(population))
	if total == 0 {
		return 0
	}
	var px [2]float64
	var py [2]float64
	for x := 0; x < 2; x++ {
		for y := 0; y < 2; y++ {
			px[x] += counts[x][y]
			py[y] += counts[x][y]
		}
	}
	mi := 0.0
	for x := 0; x < 2; x++ {
		for y := 0; y < 2; y++ {
			if counts[x][y] == 0 {
				continue
			}
			pxy := counts[x][y] / total
			mi += pxy * math.Log(pxy/((px[x]/total)*(py[y]/total)))
		}
	}
	return mi
}

func averageLinkage(mi [][]float64, a []int, b []int) float64 {
	total := 0.0
	count := 0
	for _, i := range a {
		for _, j := range b {
			total += mi[i][j]
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func dedupeGroups(groups []mixingGroup, dimensions int) []mixingGroup {
	seen := map[string]bool{}
	var out []mixingGroup
	for _, group := range groups {
		clean := make([]int, 0, len(group.Indices))
		for _, index := range group.Indices {
			if index >= 0 && index < dimensions {
				clean = append(clean, index)
			}
		}
		sort.Ints(clean)
		clean = uniqueInts(clean)
		if len(clean) == 0 || len(clean) == dimensions {
			continue
		}
		keyParts := make([]string, len(clean))
		for i, index := range clean {
			keyParts[i] = strconv.Itoa(index)
		}
		key := strings.Join(keyParts, ",")
		if seen[key] {
			continue
		}
		seen[key] = true
		group.Indices = clean
		out = append(out, group)
	}
	return out
}

func uniqueInts(values []int) []int {
	if len(values) == 0 {
		return values
	}
	out := []int{values[0]}
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func changedFlagNames(before Vector, after Vector, group []int) []string {
	var names []string
	for _, index := range group {
		if index < 0 || index >= len(before) || index >= len(after) {
			continue
		}
		if before[index] != after[index] {
			names = append(names, flagName(index))
		}
	}
	return names
}

func flagName(index int) string {
	if index >= 0 && index < len(availableFlags) {
		return availableFlags[index]
	}
	return fmt.Sprintf("flag_%d", index)
}

func finiteDelta(oldFitness float64, newFitness float64) *float64 {
	if math.IsInf(oldFitness, 0) || math.IsNaN(oldFitness) || math.IsInf(newFitness, 0) || math.IsNaN(newFitness) {
		return nil
	}
	delta := newFitness - oldFitness
	return &delta
}

func writeMixingEvent(event mixingEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	path := filepath.Join(utils.ResultsPath, os.Args[1], "log", "mixing_events.jsonl")
	evalLogMu.Lock()
	defer evalLogMu.Unlock()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(string(payload) + "\n")
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envInt64(name string, fallback int64) int64 {
	value, err := strconv.ParseInt(os.Getenv(name), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func atomicEvalCount() uint64 {
	return atomic.LoadUint64(&evalCounter)
}
