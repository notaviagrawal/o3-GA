from __future__ import annotations

import argparse
import json
import math
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from .evaluator import PolyBenchEvaluator, require_tool
from .flag_sets import DEFAULT_BASE_LEVELS, FlagSpace, discover_optimizer_flags, vector_to_compile_flags
from .logging_utils import append_jsonl
from .optimizers import FitnessBatch, run_deap_ga, run_pymoo_ga, run_pyswarms_pso, run_random_search


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Correctness-aware PolyBench GCC flag tuner")
    parser.add_argument("--benchmark", required=True, help="PolyBench benchmark name, e.g. 2mm")
    parser.add_argument("--algorithm", choices=["random", "ga", "pso", "pymoo-ga"], default="random")
    parser.add_argument("--compiler", default="gcc-11")
    parser.add_argument("--dataset", default="MINI_DATASET", choices=["MINI_DATASET", "SMALL_DATASET", "MEDIUM_DATASET", "LARGE_DATASET", "EXTRALARGE_DATASET"])
    parser.add_argument("--flags", type=int, default=50, help="Number of GCC optimizer flags to use")
    parser.add_argument(
        "--fixed-base",
        choices=["O1", "O2", "O3", "Ofast"],
        help="Force every candidate to use this base optimization level before its -f/-fno flags",
    )
    parser.add_argument("--budget", type=int, default=20, help="Random-search evaluations")
    parser.add_argument("--generations", type=int, default=5)
    parser.add_argument("--pop-size", type=int, default=8)
    parser.add_argument("--repeats", type=int, default=3)
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--results-dir", default="py_results")
    parser.add_argument("--timeout", type=int, default=120)
    parser.add_argument("--workers", type=int, default=1, help="Parallel candidate evaluations per generation/iteration")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    repo_root = Path(__file__).resolve().parents[1]
    compiler = require_tool(args.compiler)
    flags = discover_optimizer_flags(compiler, args.flags)
    base_levels = (args.fixed_base,) if args.fixed_base else DEFAULT_BASE_LEVELS
    space = FlagSpace(base_levels=base_levels, flags=flags)

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
    print(
        f"Building -O0 reference for {args.benchmark} "
        f"({args.dataset}, {args.compiler})...",
        flush=True,
    )
    evaluator.build_reference()

    metadata = {
        "algorithm": args.algorithm,
        "benchmark": args.benchmark,
        "compiler": compiler,
        "dataset": args.dataset,
        "fixed_base": args.fixed_base,
        "flag_count": len(flags),
        "flags": list(flags),
        "base_levels": list(base_levels),
        "run_dir": str(run_dir),
        "seed": args.seed,
        "workers": args.workers,
    }
    (run_dir / "metadata.json").write_text(json.dumps(metadata, indent=2, sort_keys=True), encoding="utf-8")

    best_seen = {"fitness": math.inf, "vector": None}
    eval_count = {"value": 0}
    search_started_at = time.time()
    generation_started_at: dict[int, float] = {}
    generation_eval_count: dict[int, int] = {}
    progress_lock = threading.Lock()

    def fitness_fn(vector: list[int], generation: int, candidate_index: int) -> float:
        with progress_lock:
            generation_started_at.setdefault(generation, time.time())
        result = evaluator.evaluate(
            vector=vector,
            generation=generation,
            algorithm=args.algorithm,
            candidate_index=candidate_index,
            repeats=args.repeats,
        )
        with progress_lock:
            eval_count["value"] += 1
            generation_eval_count[generation] = generation_eval_count.get(generation, 0) + 1
            elapsed_search = time.time() - search_started_at
            elapsed_generation = time.time() - generation_started_at[generation]
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
            append_jsonl(
                run_dir / "progress.jsonl",
                {
                    "candidate_id": result.candidate_id,
                    "candidate_index": candidate_index,
                    "correct": result.correct,
                    "elapsed_generation": elapsed_generation,
                    "elapsed_search": elapsed_search,
                    "eval_count": eval_count["value"],
                    "generation": generation,
                    "generation_eval_count": generation_eval_count[generation],
                    "reason": result.reason,
                    "runtime": result.runtime if math.isfinite(result.runtime) else None,
                    "search_started_at": search_started_at,
                    "timestamp": time.time(),
                },
            )
            if result.correct:
                best_text = f"{best_seen['fitness']:.6f}" if math.isfinite(best_seen["fitness"]) else "n/a"
                print(
                    f"eval={eval_count['value']:04d} gen={generation:03d} "
                    f"cand={candidate_index:03d} ok runtime={result.runtime:.6f}s "
                    f"best={best_text}s elapsed={elapsed_search:.1f}s",
                    flush=True,
                )
            else:
                print(
                    f"eval={eval_count['value']:04d} gen={generation:03d} "
                    f"cand={candidate_index:03d} fail reason={result.reason} "
                    f"elapsed={elapsed_search:.1f}s",
                    flush=True,
                )
        return result.runtime if result.correct else math.inf

    print(
        f"Starting {args.algorithm} search: flags={len(flags)}, "
        f"base_levels={','.join(base_levels)}, "
        f"generations={args.generations}, pop_size={args.pop_size}, "
        f"budget={args.budget}, repeats={args.repeats}, workers={args.workers}",
        flush=True,
    )

    executor = ThreadPoolExecutor(max_workers=args.workers) if args.workers > 1 else None

    def batch_fitness_fn(candidates: FitnessBatch) -> list[float]:
        if executor is None:
            return [fitness_fn(vector, generation, candidate_index) for vector, generation, candidate_index in candidates]
        return list(executor.map(lambda item: fitness_fn(item[0], item[1], item[2]), candidates))

    try:
        batch_fn = batch_fitness_fn if args.workers > 1 else None
        if args.algorithm == "random":
            opt_result = run_random_search(space, args.budget, fitness_fn, args.seed, batch_fn)
        elif args.algorithm == "ga":
            opt_result = run_deap_ga(space, args.generations, args.pop_size, fitness_fn, args.seed, batch_fn)
        elif args.algorithm == "pso":
            opt_result = run_pyswarms_pso(space, args.generations, args.pop_size, fitness_fn, args.seed, batch_fn)
        else:
            if args.workers > 1:
                print("pymoo-ga currently ignores --workers and runs serially.", flush=True)
            opt_result = run_pymoo_ga(space, args.generations, args.pop_size, fitness_fn, args.seed)
    finally:
        if executor is not None:
            executor.shutdown(wait=True)

    best_vector = opt_result.vector
    best_flags = vector_to_compile_flags(best_vector, space)
    summary = {
        **metadata,
        "best_runtime": opt_result.fitness,
        "best_vector": best_vector,
        "best_flags": best_flags,
        "evaluations": eval_count["value"],
        "search_elapsed": time.time() - search_started_at,
    }
    (run_dir / "summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True), encoding="utf-8")
    print(f"Run complete: {run_dir}")
    print(f"Best runtime: {opt_result.fitness:.6f}")
    print("Best flags:", " ".join(best_flags))


if __name__ == "__main__":
    main()
