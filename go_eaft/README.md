[![CodeFactor](https://www.codefactor.io/repository/github/ghisloine/eaft/badge)](https://www.codefactor.io/repository/github/ghisloine/eaft)
[![Maintainability](https://api.codeclimate.com/v1/badges/f872576a650c17be1544/maintainability)](https://codeclimate.com/github/ghisloine/ga_tuner/maintainability)
# EAFT: Evolutionary Algoritms for GCC Flag Tuning

#### GA Tuner is autotuning tool which uses Genetic Algorithm for solving best flag set for C code compilation. Polybench dataset is used in this project but any other dataset could be used.

#### REQUIREMENTS

GCC-11 required. For better experience, macbook default gcc is not suitable because of optimization flags. Homebrew GCC-11.3.0 version tested. 

#### Headless runners

Run the original EAFT-style GA:

```bash
EAFT_WORKERS=6 go run ./cmd/headless 2mm gcc eaft-ga.json GA
```

Run the seeded linkage-tree GOMEA-style search:

```bash
EAFT_WORKERS=6 LTGOMEA_BUDGET=1600 go run ./cmd/headless 2mm gcc ltgomea.json LTGOMEA
```

By default, search uses EAFT-style unsafe whole-process timing. Use `EAFT_CORRECTNESS=exact` to reject wrong-output candidates immediately, or `EAFT_CORRECTNESS=metric` to log raw runtime while adding `EAFT_INCORRECT_PENALTY` seconds to the candidate fitness when the dump-output check fails.

For exact PolyBench output searches, `EAFT_LOCK_SAFE_FLAGS=1` forces known semantic-risk flags such as `fast-math`, `associative-math`, and `finite-math-only` off. This keeps the search from repeatedly rediscovering very fast but wrong-output floating-point configurations.

LT-GOMEA writes `results/<benchmark>/log/mixing_events.jsonl`, which records each attempted group mix, the donor/target, changed flags, seeded-vs-learned group source, runtime delta, and whether the change was accepted.
