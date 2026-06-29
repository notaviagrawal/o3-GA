package scripts

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"ga_tuner/utils"
	"log"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MaxHalford/eaopt"
)

var evalCounter uint64
var evalLogMu sync.Mutex
var evalPrintMu sync.Mutex
var searchStartedAt = time.Now()
var bestRuntime = math.Inf(1)
var referenceOnce sync.Once
var referenceDump []byte
var referenceErr error
var workerSemaphore = makeWorkerSemaphore()

type evalLogEntry struct {
	Algorithm       string    `json:"algorithm"`
	Benchmark       string    `json:"benchmark"`
	CandidateID     string    `json:"candidate_id"`
	CompileSeconds  float64   `json:"compile_seconds"`
	Command         string    `json:"command"`
	Count           int       `json:"count"`
	Correct         *bool     `json:"correct,omitempty"`
	CorrectSeconds  float64   `json:"correct_seconds"`
	CorrectnessMode string    `json:"correctness_mode"`
	EvalIndex       uint64    `json:"eval_index"`
	IsBaseline      bool      `json:"is_baseline"`
	OutputPath      string    `json:"output_path"`
	Reason          string    `json:"reason,omitempty"`
	RuntimeSeconds  *float64  `json:"runtime_seconds,omitempty"`
	RunSeconds      []float64 `json:"run_seconds"`
	Status          string    `json:"status"`
	Timestamp       string    `json:"timestamp"`
}

func GARunner() {
	var ga, err = eaopt.NewDefaultGAConfig().NewGA()
	if err != nil {
		fmt.Println(err)
		return
	}
	JSON_FILE := filepath.Join(utils.ResultsPath, os.Args[1], "log", os.Args[3])
	f, _ := os.Create(JSON_FILE)
	defer f.Close()
	w := bufio.NewWriter(f)
	fmt.Fprint(w, "")
	w.Flush()

	f, _ = os.OpenFile(JSON_FILE, os.O_APPEND|os.O_WRONLY, 0666)
	defer f.Close()

	var bytes, _ = json.Marshal(ga)
	f.WriteString(string(bytes) + "\n")

	level_two := CollectBaseline("O2")
	level_three := CollectBaseline("O3")

	level_two_notification := fmt.Sprintf("O2 : %f", level_two)
	level_three_notification := fmt.Sprintf("O3 : %f", level_three)

	utils.Notifications = append(utils.Notifications, level_two_notification)
	utils.Notifications = append(utils.Notifications, level_three_notification)

	ga.NGenerations = 50
	ga.ParallelEval = true
	ga.PopSize = 40
	// Add a custom print function to track progress
	ga.Callback = func(ga *eaopt.GA) {
		utils.TextBox = fmt.Sprintf("Best fitness at generation %d: ID:  %s, Fitness : %f, Improvement : %f\n", ga.Generations, ga.HallOfFame[0].ID, ga.HallOfFame[0].Fitness, 1-ga.HallOfFame[0].Fitness/level_two)
		utils.Progress = (float64(ga.Generations+1) / float64(ga.NGenerations)) * float64(100)
		utils.HallOfFame = ga.HallOfFame[0].Fitness
		utils.BestOfPops = append(utils.BestOfPops, ga.HallOfFame[0].Fitness)
		utils.Stats = []float64{math.Floor(ga.HallOfFame.FitMin()*100) / 100, math.Floor(ga.HallOfFame.FitMax()*100) / 100, math.Floor(ga.HallOfFame.FitAvg()*100) / 100}
		var bytes, err = json.Marshal(ga)

		if err != nil {
			fmt.Println(err)
		}
		f.WriteString(string(bytes) + "\n")
		fmt.Printf(
			"generation=%03d best=%.6fs progress=%.1f%% elapsed=%.1fs\n",
			ga.Generations,
			ga.HallOfFame[0].Fitness,
			utils.Progress,
			time.Since(searchStartedAt).Seconds(),
		)
	}

	// Find the minimum
	err = ga.Minimize(VectorFactory)
	if err != nil {
		fmt.Println(err)
		return
	}
}

func MutNormalFloat64(genome []float64, rate float64, rng *rand.Rand) {
	for i := range genome {
		// Flip a coin and decide to mutate or not
		if rng.Float64() < rate {
			//genome[i] += rng.NormFloat64() * genome[i]
			if genome[i] < 0.5 {
				genome[i] = 0
			} else {
				genome[i] = 1
			}
		}
	}
}

// Returns a Binary Vector with just only 0 - 1.
func InitBinaryFloat64(n uint, lower, upper float64, rng *rand.Rand) (floats []float64) {
	floats = make([]float64, n)
	for i := range floats {
		floats[i] = lower + float64(rng.Intn(int(upper)))
	}
	return
}

func CompileCode(cmd string, id string, count int) (Total float64) {
	releaseWorker := acquireWorker()
	defer releaseWorker()

	evalIndex := atomic.AddUint64(&evalCounter, 1)
	app := os.Args[2]
	exec_file := filepath.Join(utils.ResultsPath, os.Args[1], "bin", id)

	correctnessMode := getCorrectnessMode()
	var correct *bool
	correctSeconds := 0.0
	if correctnessMode == "exact" {
		correctStarted := time.Now()
		isCorrect, correctReason := checkCorrectness(cmd, id, app)
		correctSeconds = time.Since(correctStarted).Seconds()
		correct = boolPtr(isCorrect)
		if !isCorrect {
			Total = math.Inf(10)
			writeEvalLog(evalLogEntry{
				Algorithm:       currentAlgorithm(),
				Benchmark:       os.Args[1],
				CandidateID:     id,
				Command:         cmd,
				Count:           count,
				Correct:         correct,
				CorrectSeconds:  correctSeconds,
				CorrectnessMode: correctnessMode,
				EvalIndex:       evalIndex,
				IsBaseline:      isBaselineCommand(cmd),
				OutputPath:      exec_file,
				Reason:          correctReason,
				RuntimeSeconds:  runtimePtr(Total),
				Status:          correctReason,
				Timestamp:       time.Now().Format(time.RFC3339Nano),
			})
			return Total
		}
	}

	// COMPILE
	compileStarted := time.Now()
	command := strings.Split(cmd, " ")
	out_compile, err := exec.Command(app, command...).Output()
	compileSeconds := time.Since(compileStarted).Seconds()
	if err != nil {
		log.Print(string(out_compile))
		Total = math.Inf(10)
		writeEvalLog(evalLogEntry{
			Algorithm:       currentAlgorithm(),
			Benchmark:       os.Args[1],
			CandidateID:     id,
			CompileSeconds:  compileSeconds,
			Command:         cmd,
			Count:           count,
			Correct:         correct,
			CorrectSeconds:  correctSeconds,
			CorrectnessMode: correctnessMode,
			EvalIndex:       evalIndex,
			IsBaseline:      isBaselineCommand(cmd),
			OutputPath:      exec_file,
			Reason:          "compile_failed",
			RuntimeSeconds:  runtimePtr(Total),
			Status:          "compile_failed",
			Timestamp:       time.Now().Format(time.RFC3339Nano),
		})
		return Total
	}

	// EXECUTION
	TotalExecTime := 0.0
	runSeconds := make([]float64, 0, count)
	for i := 0; i < count; i++ {
		command_exec := exec.Command(exec_file)
		var out_exec bytes.Buffer
		// set the output to our variable
		command_exec.Stdout = &out_exec
		start := time.Now()
		err = command_exec.Run()
		elapsed := time.Since(start).Seconds()
		runSeconds = append(runSeconds, elapsed)
		TotalExecTime += elapsed
		if err != nil {
			Total = math.Inf(10)
			writeEvalLog(evalLogEntry{
				Algorithm:       currentAlgorithm(),
				Benchmark:       os.Args[1],
				CandidateID:     id,
				CompileSeconds:  compileSeconds,
				Command:         cmd,
				Count:           count,
				Correct:         correct,
				CorrectSeconds:  correctSeconds,
				CorrectnessMode: correctnessMode,
				EvalIndex:       evalIndex,
				IsBaseline:      isBaselineCommand(cmd),
				OutputPath:      exec_file,
				Reason:          "run_failed",
				RuntimeSeconds:  runtimePtr(Total),
				RunSeconds:      runSeconds,
				Status:          "run_failed",
				Timestamp:       time.Now().Format(time.RFC3339Nano),
			})
			return Total
		}
	}
	// CALC AVERAGE OF TOTAL RUN TIME
	Total = TotalExecTime / float64(count)
	writeEvalLog(evalLogEntry{
		Algorithm:       currentAlgorithm(),
		Benchmark:       os.Args[1],
		CandidateID:     id,
		CompileSeconds:  compileSeconds,
		Command:         cmd,
		Count:           count,
		Correct:         correct,
		CorrectSeconds:  correctSeconds,
		CorrectnessMode: correctnessMode,
		EvalIndex:       evalIndex,
		IsBaseline:      isBaselineCommand(cmd),
		OutputPath:      exec_file,
		RuntimeSeconds:  runtimePtr(Total),
		RunSeconds:      runSeconds,
		Status:          "ok",
		Timestamp:       time.Now().Format(time.RFC3339Nano),
	})
	utils.TotalRunTimes = append(utils.TotalRunTimes, math.Floor(Total*100)/100)
	return
}

func getCorrectnessMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("EAFT_CORRECTNESS")))
	if mode == "" {
		return "off"
	}
	if mode != "off" && mode != "exact" {
		log.Printf("ignoring invalid EAFT_CORRECTNESS=%q, using off", mode)
		return "off"
	}
	return mode
}

func boolPtr(value bool) *bool {
	return &value
}

func makeWorkerSemaphore() chan struct{} {
	raw := os.Getenv("EAFT_WORKERS")
	if raw == "" {
		return nil
	}
	workers, err := strconv.Atoi(raw)
	if err != nil || workers <= 0 {
		log.Printf("ignoring invalid EAFT_WORKERS=%q", raw)
		return nil
	}
	log.Printf("EAFT_WORKERS=%d", workers)
	return make(chan struct{}, workers)
}

func acquireWorker() func() {
	if workerSemaphore == nil {
		return func() {}
	}
	workerSemaphore <- struct{}{}
	return func() {
		<-workerSemaphore
	}
}

func checkCorrectness(cmd string, id string, app string) (bool, string) {
	referenceOnce.Do(func() {
		referenceDump, referenceErr = buildReferenceDump(app)
	})
	if referenceErr != nil {
		log.Print(referenceErr)
		return false, "reference_failed"
	}

	dumpPath := filepath.Join(utils.ResultsPath, os.Args[1], "bin", id+"_dump")
	dumpCommand := commandWithDumpOutput(strings.Split(cmd, " "), dumpPath)
	out, err := exec.Command(app, dumpCommand...).CombinedOutput()
	if err != nil {
		log.Print(string(out))
		return false, "correctness_compile_failed"
	}

	proc := exec.Command(dumpPath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	proc.Stdout = &stdout
	proc.Stderr = &stderr
	if err := proc.Run(); err != nil {
		return false, "correctness_run_failed"
	}
	if !bytes.Equal(stderr.Bytes(), referenceDump) {
		return false, "wrong_output"
	}
	return true, ""
}

func buildReferenceDump(app string) ([]byte, error) {
	referencePath := filepath.Join(utils.ResultsPath, os.Args[1], "bin", "reference_dump")
	args := []string{
		"-O0",
		"-DPOLYBENCH_DUMP_ARRAYS",
		filepath.Join(utils.Files, os.Args[1]+".c"),
		"-I" + utils.Utilities,
		"--include",
		"polybench.c",
		"-lm",
		"-o",
		referencePath,
	}
	out, err := exec.Command(app, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("reference compile failed: %s", string(out))
	}
	proc := exec.Command(referencePath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	proc.Stdout = &stdout
	proc.Stderr = &stderr
	if err := proc.Run(); err != nil {
		return nil, fmt.Errorf("reference run failed: %w", err)
	}
	return stderr.Bytes(), nil
}

func commandWithDumpOutput(command []string, dumpPath string) []string {
	out := make([]string, 0, len(command)+1)
	out = append(out, "-DPOLYBENCH_DUMP_ARRAYS")
	for i := 0; i < len(command); i++ {
		out = append(out, command[i])
		if command[i] == "-o" && i+1 < len(command) {
			out = append(out, dumpPath)
			i++
		}
	}
	return out
}

func currentAlgorithm() string {
	if len(os.Args) > 4 {
		return os.Args[4]
	}
	return ""
}

func isBaselineCommand(cmd string) bool {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return false
	}
	if fields[0] != "-O1" && fields[0] != "-O2" && fields[0] != "-O3" && fields[0] != "-Ofast" {
		return false
	}
	for _, field := range fields[1:] {
		if strings.HasPrefix(field, "-f") {
			return false
		}
	}
	return true
}

func writeEvalLog(entry evalLogEntry) {
	logPath := filepath.Join(utils.ResultsPath, os.Args[1], "log", "evaluations.jsonl")
	payload, err := json.Marshal(entry)
	if err != nil {
		log.Print(err)
		return
	}
	evalLogMu.Lock()
	defer evalLogMu.Unlock()
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(string(payload) + "\n")
	printEvalUpdate(entry)
}

func runtimePtr(value float64) *float64 {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return nil
	}
	return &value
}

func printEvalUpdate(entry evalLogEntry) {
	evalPrintMu.Lock()
	defer evalPrintMu.Unlock()

	bestText := "n/a"
	if entry.Status == "ok" && entry.RuntimeSeconds != nil && (entry.Correct == nil || *entry.Correct) {
		if *entry.RuntimeSeconds < bestRuntime {
			bestRuntime = *entry.RuntimeSeconds
		}
	}
	if !math.IsInf(bestRuntime, 0) && !math.IsNaN(bestRuntime) {
		bestText = fmt.Sprintf("%.6f", bestRuntime)
	}

	runtimeText := "n/a"
	if entry.RuntimeSeconds != nil {
		runtimeText = fmt.Sprintf("%.6f", *entry.RuntimeSeconds)
	}
	label := "candidate"
	if entry.IsBaseline {
		label = "baseline"
	}
	correctText := "unchecked"
	if entry.Correct != nil {
		correctText = fmt.Sprintf("%t", *entry.Correct)
	}
	fmt.Printf(
		"eval=%04d %s status=%s correctness=%s correct=%s runtime=%ss best=%ss elapsed=%.1fs\n",
		entry.EvalIndex,
		label,
		entry.Status,
		entry.CorrectnessMode,
		correctText,
		runtimeText,
		bestText,
		time.Since(searchStartedAt).Seconds(),
	)
}
