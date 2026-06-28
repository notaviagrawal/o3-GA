from __future__ import annotations

import argparse
import json
import math
import time
from pathlib import Path

from .evaluator import PolyBenchEvaluator, require_tool
from .flag_sets import DEFAULT_BASE_LEVELS, FlagSpace, discover_optimizer_flags, vector_to_compile_flags
from .logging_utils import append_jsonl
from .optimizers import run_deap_ga, run_pymoo_ga, run_pyswarms_pso, run_random_search


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Correctness-aware PolyBench GCC flag tuner")
    parser.add_argument("--benchmark", required=True, help="PolyBench benchmark name, e.g. 2mm")
    parser.add_argument("--algorithm", choices=["random", "ga", "pso", "pymoo-ga"], default="random")
    parser.add_argument("--compiler", default="gcc-11")
    parser.add_argument("--dataset", default="MINI_DATASET", choices=["MINI_DATASET", "SMALL_DATASET", "MEDIUM_DATASET", "LARGE_DATASET", "EXTRALARGE_DATASET"])
    parser.add_argument("--flags", type=int, default=50, help="Number of GCC optimizer flags to use")
    parser.add_argument("--budget", type=int, default=20, help="Random-search evaluations")
    parser.add_argument("--generations", type=int, default=5)
    parser.add_argument("--pop-size", type=int, default=8)
    parser.add_argument("--repeats", type=int, default=3)
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--results-dir", default="py_results")
    parser.add_argument("--timeout", type=int, default=120)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    repo_root = Path(__file__).resolve().parents[1]
    compiler = require_tool(args.compiler)
    flags = discover_optimizer_flags(compiler, args.flags)
    space = FlagSpace(base_levels=DEFAULT_BASE_LEVELS, flags=flags)

    run_id = time.strftime("%Y%m%d-%H%M%S")
    run_dir = repo_root / args.results_dir / args.benchmark / f"{run_id}-{args.algorithm}"
    run_dir.mkdir(parents=True, exist_ok=True)

    evaluator = PolyBenchEvaluator(
        repo_root=repo_root,
        benchmark=args.benchmark,
        compiler=compiler,
        flag_space=space,
        run_dir=run_dir,
        dataset=args.dataset,
        timeout_seconds=args.timeout,
    )
    evaluator.build_reference()

    metadata = {
        "algorithm": args.algorithm,
        "benchmark": args.benchmark,
        "compiler": compiler,
        "dataset": args.dataset,
        "flag_count": len(flags),
        "flags": list(flags),
        "base_levels": list(DEFAULT_BASE_LEVELS),
        "run_dir": str(run_dir),
        "seed": args.seed,
    }
    (run_dir / "metadata.json").write_text(json.dumps(metadata, indent=2, sort_keys=True), encoding="utf-8")

    best_seen = {"fitness": math.inf, "vector": None}

    def fitness_fn(vector: list[int], generation: int, candidate_index: int) -> float:
        result = evaluator.evaluate(
            vector=vector,
            generation=generation,
            algorithm=args.algorithm,
            candidate_index=candidate_index,
            repeats=args.repeats,
        )
        if result.correct and result.runtime < best_seen["fitness"]:
            best_seen["fitness"] = result.runtime
            best_seen["vector"] = vector
            append_jsonl(
                run_dir / "best.jsonl",
                {
                    "generation": generation,
                    "candidate_index": candidate_index,
                    "runtime": result.runtime,
                    "vector": vector,
                    "compile_flags": result.compile_flags,
                    "candidate_id": result.candidate_id,
                },
            )
        return result.runtime if result.correct else math.inf

    if args.algorithm == "random":
        opt_result = run_random_search(space, args.budget, fitness_fn, args.seed)
    elif args.algorithm == "ga":
        opt_result = run_deap_ga(space, args.generations, args.pop_size, fitness_fn, args.seed)
    elif args.algorithm == "pso":
        opt_result = run_pyswarms_pso(space, args.generations, args.pop_size, fitness_fn, args.seed)
    else:
        opt_result = run_pymoo_ga(space, args.generations, args.pop_size, fitness_fn, args.seed)

    best_vector = opt_result.vector
    best_flags = vector_to_compile_flags(best_vector, space)
    summary = {
        **metadata,
        "best_runtime": opt_result.fitness,
        "best_vector": best_vector,
        "best_flags": best_flags,
    }
    (run_dir / "summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True), encoding="utf-8")
    print(f"Run complete: {run_dir}")
    print(f"Best runtime: {opt_result.fitness:.6f}")
    print("Best flags:", " ".join(best_flags))


if __name__ == "__main__":
    main()

