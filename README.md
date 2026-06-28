# o3-GA

Correctness-aware GCC optimization flag tuning for PolyBench/C.

This repo searches for flag combinations that beat standard `-O` levels on a single C benchmark. It uses evolutionary and swarm optimizers, validates output correctness, measures PolyBench kernel runtime, and generates Python plots.

The current standalone benchmark is `2mm.c`.

## What It Does

For each candidate flag vector:

1. Compile a correctness binary with `-DPOLYBENCH_DUMP_ARRAYS`.
2. Compare its output against a safe `-O0` reference dump.
3. If correct, compile a timing binary with `-DPOLYBENCH_TIME`.
4. Run timing repeats and average the PolyBench kernel time.
5. Log every candidate to JSONL.
6. Plot search progress.

Hyperfine is used only for final confirmation if it is installed.

## Install

```bash
python3 -m pip install -r py_tuner/requirements.txt
```

You also need GCC. On macOS this was tested with Homebrew `gcc-11`; on Linux, regular `gcc` should work.

## Run

Smoke test:

```bash
python3 -m py_tuner.tune \
  --benchmark 2mm \
  --algorithm random \
  --budget 4 \
  --dataset MINI_DATASET \
  --flags 50 \
  --repeats 1
```

GA run:

```bash
python3 -m py_tuner.tune \
  --benchmark 2mm \
  --algorithm ga \
  --generations 50 \
  --pop-size 40 \
  --dataset SMALL_DATASET \
  --flags 50 \
  --repeats 3
```

PSO run:

```bash
python3 -m py_tuner.tune \
  --benchmark 2mm \
  --algorithm pso \
  --generations 50 \
  --pop-size 40 \
  --dataset SMALL_DATASET \
  --flags 50 \
  --repeats 3
```

If your compiler is named `gcc` instead of `gcc-11`, add:

```bash
--compiler gcc
```

## Plot

After tuning, use the printed run directory:

```bash
python3 -m py_tuner.plot --run-dir py_results/2mm/<run-id>
```

Generated plots include:

- `best_runtime.png`
- `runtime_trace.png`
- `generation_runtime_stats.png`
- `correctness_by_generation.png`

## Final Comparison

```bash
python3 -m py_tuner.final_compare --run-dir py_results/2mm/<run-id> --runs 20 --warmup 3
```

This compares:

- `-O1`
- `-O2`
- `-O3`
- `-Ofast`
- tuned flags

If `hyperfine` is installed, it is used for external process-level benchmarking. Otherwise the script falls back to PolyBench kernel timings.

## Sample Results

`sample_results/2mm-random-smoke` contains a tiny smoke run. It verifies the pipeline, but it is not meant to show meaningful optimization quality.

