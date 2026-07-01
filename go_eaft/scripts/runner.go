package scripts

import (
	"ga_tuner/utils"
	"ga_tuner/utils/tools"
	"math"
	"math/rand"
	"os"
	"path"
	"strings"

	"github.com/MaxHalford/eaopt"
	uuid "github.com/satori/go.uuid"
)

// A Vector of SingleBench.
type Vector []float64

var availableFlags []string = tools.Flags

var safeFlagOverrides = map[string]float64{
	"allow-store-data-races": 0,
	"associative-math":       0,
	"cx-limited-range":       0,
	"fast-math":              0,
	"finite-math-only":       0,
	"fp-int-builtin-inexact": 0,
}

// TODO : Change static optimization level with Dynamic one.
func MatchBinaryWithFlags(X Vector, OptLevel string) (string, map[string]int) {
	// First collect all available Flags
	cmd := "-" + OptLevel + " "
	// Replace with -f or -fno according to X
	flag_map := map[string]int{}
	for i, v := range X {
		vv := v
		if i < len(availableFlags) {
			vv = constrainedFlagValue(availableFlags[i], vv)
		}
		if math.IsNaN(vv) || math.IsInf(vv, 0) {
			vv = 0
		}
		// GA uses 0/1; PSO uses continuous [0,1]. int(v)==0 wrongly maps every
		// fraction in (0,1) to "off"; threshold matches both encodings.
		on := vv >= 0.5
		if !on {
			cmd += "-fno-" + availableFlags[i] + " "
			flag_map[availableFlags[i]] = 0
		} else {
			cmd += "-f" + availableFlags[i] + " "
			flag_map[availableFlags[i]] = 1
		}
	}

	return cmd, flag_map
}

func addPolybenchDependencies(command string, problem string, out_file string) string {
	if dataset := os.Getenv("EAFT_DATASET"); dataset != "" {
		command += "-D" + dataset + " "
	}
	command += path.Join(utils.Files, problem) + `.c` + ` -I` + utils.Utilities + ` --include ` + `polybench.c` + ` -o ` + path.Join(utils.ResultsPath, os.Args[1], "bin", out_file) + ` -lm`
	return command
}

func constrainedVector(vector Vector) Vector {
	out := make(Vector, len(vector))
	copy(out, vector)
	if !lockSafeFlags() {
		return out
	}
	for i, name := range availableFlags {
		if i >= len(out) {
			break
		}
		if value, ok := safeFlagOverrides[name]; ok {
			out[i] = value
		}
	}
	return out
}

func constrainedFlagValue(name string, value float64) float64 {
	if !lockSafeFlags() {
		return value
	}
	if safeValue, ok := safeFlagOverrides[name]; ok {
		return safeValue
	}
	return value
}

func lockSafeFlags() bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("EAFT_LOCK_SAFE_FLAGS")))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

// Fitness function burasi
func (X Vector) Evaluate() (float64, error) {
	X = constrainedVector(X)
	// Changing Binary Array to GCC command with corresponding open / close flag
	output := uuid.NewV4().String()
	cmd, _ := MatchBinaryWithFlags(X, "O3")

	// Adding some polybench information to run cmd
	cmd = addPolybenchDependencies(cmd, os.Args[1], output)

	// Total is Execution time of Code.
	total := CompileCode(cmd, output, 3)

	return total, nil
}

// Mutate a Vector by resampling each element from a normal distribution with
// probability 0.8.

func (p Vector) Mutate(rng *rand.Rand) {
	MutNormalFloat64(p, 0.8, rng)
}

// TODO : Paper'da olup burada olmayan crossover metodlari neler var ona bak.
// Crossover a Vector with another Vector by applying uniform crossover.
func (X Vector) Crossover(Y eaopt.Genome, rng *rand.Rand) {
	eaopt.CrossGNXFloat64(X, Y.(Vector), 2, rng)
}

// Clone a Vector to produce a new one that points to a different slice.
func (X Vector) Clone() eaopt.Genome {
	var Y = make(Vector, len(X))
	copy(Y, X)
	return Y
}

// VectorFactory returns a random vector by generating 2 values uniformally
// distributed between -10 and 10.
func VectorFactory(rng *rand.Rand) eaopt.Genome {
	// NUMBER_OF_FLAGS := uint(len(availableFlags))
	return Vector(InitBinaryFloat64(50, 0, 2, rng))

}

func CollectBaseline(Baseline string) float64 {
	output := uuid.NewV4().String()
	cmd, _ := MatchBinaryWithFlags(make([]float64, 0), Baseline)
	// Adding some polybench information to run cmd
	cmd = addPolybenchDependencies(cmd, os.Args[1], output)

	// Total is Execution time of Code.
	total := CompileCode(cmd, output, 1)
	return total
}
