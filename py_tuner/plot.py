from __future__ import annotations

import argparse
import json
from pathlib import Path

import matplotlib.pyplot as plt
import pandas as pd
import seaborn as sns

from .logging_utils import read_jsonl


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Plot Python tuner results")
    parser.add_argument("--run-dir", required=True)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    run_dir = Path(args.run_dir).resolve()
    eval_path = run_dir / "evaluations.jsonl"
    if not eval_path.exists():
        raise FileNotFoundError(f"Missing evaluations log: {eval_path}")

    records = read_jsonl(eval_path)
    if not records:
        raise RuntimeError("No evaluation records to plot")

    df = pd.DataFrame(records)
    df["eval_index"] = range(len(df))
    df["runtime_plot"] = pd.to_numeric(df["runtime"], errors="coerce")
    df["best_so_far"] = df["runtime_plot"].cummin()

    plots_dir = run_dir / "plots"
    plots_dir.mkdir(exist_ok=True)
    sns.set_theme(style="whitegrid")

    plt.figure(figsize=(10, 5))
    sns.lineplot(data=df, x="eval_index", y="best_so_far", marker="o")
    plt.title("Best Runtime So Far")
    plt.xlabel("Evaluation")
    plt.ylabel("PolyBench kernel time (s)")
    plt.tight_layout()
    plt.savefig(plots_dir / "best_runtime.png", dpi=160)
    plt.close()

    correct_df = df[df["correct"] == True].copy()
    if not correct_df.empty:
        correct_df["rolling_mean"] = correct_df["runtime_plot"].rolling(window=10, min_periods=1).mean()
        plt.figure(figsize=(10, 5))
        plt.plot(
            correct_df["eval_index"],
            correct_df["runtime_plot"],
            color="#77b7b2",
            alpha=0.28,
            linewidth=1.4,
            label="candidate runtime",
        )
        plt.plot(
            correct_df["eval_index"],
            correct_df["rolling_mean"],
            color="#2c8c83",
            linewidth=2.0,
            label="rolling mean",
        )
        plt.plot(
            df["eval_index"],
            df["best_so_far"],
            color="#9b6a47",
            linewidth=2.0,
            label="best so far",
        )
        plt.title("Runtime Over Search")
        plt.xlabel("Evaluation")
        plt.ylabel("PolyBench kernel time (s)")
        plt.legend()
        plt.tight_layout()
        plt.savefig(plots_dir / "runtime_trace.png", dpi=160)
        plt.close()

    correct_counts = df.groupby(["generation", "correct"]).size().reset_index(name="count")
    plt.figure(figsize=(10, 5))
    sns.barplot(data=correct_counts, x="generation", y="count", hue="correct")
    plt.title("Correctness Outcomes by Generation")
    plt.xlabel("Generation")
    plt.ylabel("Candidates")
    plt.tight_layout()
    plt.savefig(plots_dir / "correctness_by_generation.png", dpi=160)
    plt.close()

    if not correct_df.empty:
        gen_stats = (
            correct_df.groupby("generation")["runtime_plot"]
            .agg(["min", "mean", "max"])
            .reset_index()
        )
        plt.figure(figsize=(10, 5))
        plt.plot(gen_stats["generation"], gen_stats["min"], marker="o", label="min")
        plt.plot(gen_stats["generation"], gen_stats["mean"], marker="o", label="mean")
        plt.plot(gen_stats["generation"], gen_stats["max"], marker="o", label="max")
        plt.title("Population Runtime by Generation")
        plt.xlabel("Generation")
        plt.ylabel("PolyBench kernel time (s)")
        plt.legend()
        plt.tight_layout()
        plt.savefig(plots_dir / "generation_runtime_stats.png", dpi=160)
        plt.close()

    summary_path = run_dir / "summary.json"
    if summary_path.exists():
        summary = json.loads(summary_path.read_text(encoding="utf-8"))
        (plots_dir / "best_flags.txt").write_text(" ".join(summary["best_flags"]) + "\n", encoding="utf-8")

    print(f"Plots written to {plots_dir}")


if __name__ == "__main__":
    main()
