package scripts

import (
	"encoding/json"
	"fmt"
	"ga_tuner/utils"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/MaxHalford/eaopt"
	uuid "github.com/satori/go.uuid"
)

// psoLogEntry is JSON-safe (no function fields, no NaN).
type psoLogEntry struct {
	Generations uint      `json:"generations"`
	BestID      string    `json:"best_id"`
	Fitness     float64   `json:"fitness"`
	Improvement float64   `json:"improvement"`
	BestVector  []float64 `json:"best_vector,omitempty"`
}

func sanitizeJSONFloat(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

func improvementOverO2(bestFitness, o2Baseline float64) float64 {
	if o2Baseline <= 1e-12 || math.IsNaN(o2Baseline) || math.IsInf(o2Baseline, 0) {
		return 0
	}
	v := 1 - bestFitness/o2Baseline
	return sanitizeJSONFloat(v)
}

func clonePSOVector(g eaopt.Genome) []float64 {
	p, ok := g.(*eaopt.Particle)
	if !ok || p == nil {
		return nil
	}
	// CurrentX can become NaN after Mutate (e.g. ss==0 -> rX/ss); BestX holds the
	// last improving position and stays finite for successful runs.
	src := p.CurrentX
	if len(p.BestX) == len(p.CurrentX) && len(p.BestX) > 0 {
		src = p.BestX
	}
	out := make([]float64, len(src))
	copy(out, src)
	return out
}

// sanitizeFloatSlice replaces NaN/Inf so encoding/json can encode (PSO positions may be NaN).
func sanitizeFloatSlice(s []float64) []float64 {
	if len(s) == 0 {
		return s
	}
	out := make([]float64, len(s))
	for i, v := range s {
		out[i] = sanitizeJSONFloat(v)
	}
	return out
}

func writePSOLogLine(f *os.File, ga *eaopt.GA, levelTwo float64) {
	if ga == nil || len(ga.HallOfFame) == 0 {
		return
	}
	best := ga.HallOfFame[0]
	entry := psoLogEntry{
		Generations: ga.Generations,
		BestID:      best.ID,
		Fitness:     sanitizeJSONFloat(best.Fitness),
		Improvement: improvementOverO2(best.Fitness, levelTwo),
		BestVector:  sanitizeFloatSlice(clonePSOVector(best.Genome)),
	}
	b, err := json.Marshal(entry)
	if err != nil {
		// Avoid log to stderr while termui runs (corrupts display).
		return
	}
	_, _ = f.WriteString(string(b) + "\n")
}

func PSORunner() {
	// Non-zero inertia (W): W=0 drops history term and worsens numerical blow-ups.
	spso, err := eaopt.NewSPSO(10, 10, 0, 1, 0.5, true, nil)
	if err != nil {
		log.Println(err)
		return
	}

	jsonFile := filepath.Join(utils.ResultsPath, os.Args[1], "log", filepath.Base(os.Args[3]))
	f, err := os.OpenFile(jsonFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if err != nil {
		log.Println(err)
		return
	}
	defer f.Close()

	initSnap := struct {
		Phase        string `json:"phase"`
		Algo         string `json:"algo"`
		PopSize      uint   `json:"pop_size"`
		NGenerations uint   `json:"n_generations"`
	}{
		Phase:        "init",
		Algo:         "PSO",
		PopSize:      spso.GA.PopSize,
		NGenerations: spso.GA.NGenerations,
	}
	b0, err := json.Marshal(initSnap)
	if err != nil {
		log.Println(err)
		return
	}
	_, _ = f.WriteString(string(b0) + "\n")

	levelTwo := CollectBaseline("O2")
	levelThree := CollectBaseline("O3")

	utils.Notifications = append(utils.Notifications,
		fmt.Sprintf("O2 : %f", levelTwo),
		fmt.Sprintf("O3 : %f", levelThree),
	)

	spso.GA.RNG = rand.New(rand.NewSource(42))
	spso.GA.Callback = func(ga *eaopt.GA) {
		best := ga.HallOfFame[0]
		fit := sanitizeJSONFloat(best.Fitness)
		imp := improvementOverO2(best.Fitness, levelTwo)
		utils.TextBox = fmt.Sprintf(
			"Best fitness at generation %d: ID: %s, Fitness : %f, Improvement : %f\n",
			ga.Generations, best.ID, fit, imp,
		)
		utils.Progress = (float64(ga.Generations+1) / float64(ga.NGenerations)) * 100
		utils.HallOfFame = fit
		utils.BestOfPops = append(utils.BestOfPops, fit)
		utils.Stats = []float64{
			math.Floor(sanitizeJSONFloat(ga.HallOfFame.FitMin())*100) / 100,
			math.Floor(sanitizeJSONFloat(ga.HallOfFame.FitMax())*100) / 100,
			math.Floor(sanitizeJSONFloat(ga.HallOfFame.FitAvg())*100) / 100,
		}
		writePSOLogLine(f, ga, levelTwo)
	}

	_, _, err = spso.Minimize(FitnessFunction, 50)
	if err != nil {
		log.Println(err)
		return
	}
}

func FitnessFunction(X []float64) (y float64) {
	output := uuid.NewV4().String()
	cmd, _ := MatchBinaryWithFlags(X, "O2")

	cmd = addPolybenchDependencies(cmd, os.Args[1], output)

	total := CompileCode(cmd, output, 3)
	return total
}
