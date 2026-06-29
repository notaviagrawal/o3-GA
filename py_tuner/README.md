# Python PolyBench Flag Tuner

This is a clean Python rewrite of the original Go GCC flag tuner.

It keeps the same core idea: search GCC optimization flag choices for a C benchmark, but upgrades evaluation:

- PolyBench `-DPOLYBENCH_DUMP_ARRAYS` output is used for correctness checks.
- PolyBench `-DPOLYBENCH_TIME` internal kernel timing is used for fitness.
- Hyperfine is reserved for final confirmation comparisons when installed.
- Runs are logged as JSONL for later plotting.

## Quick start

Install Python dependencies:

```bash
python3 -m pip install -r py_tuner/requirements.txt
```

Run a small smoke test:

```bash
python3 -m py_tuner.tune --benchmark 2mm --algorithm random --budget 4 --dataset MINI_DATASET --flags 50
```

Run GA:

```bash
python3 -m py_tuner.tune --benchmark 2mm --algorithm ga --budget 80 --generations 10 --pop-size 8 --dataset MINI_DATASET --flags 50
```

Run GA in the original EAFT style, forcing every candidate to start from `-O3`:

```bash
python3 -m py_tuner.tune --benchmark 2mm --algorithm ga --generations 40 --pop-size 40 --dataset LARGE_DATASET --flags 50 --fixed-base O3
```

Run PSO:

```bash
python3 -m py_tuner.tune --benchmark 2mm --algorithm pso --budget 80 --generations 10 --pop-size 8 --dataset MINI_DATASET --flags 50
```

Plot a run:

```bash
python3 -m py_tuner.plot --run-dir py_results/2mm/<run-id>
```

Final Hyperfine comparison, if `hyperfine` is installed:

```bash
python3 -m py_tuner.final_compare --run-dir py_results/2mm/<run-id>
```
