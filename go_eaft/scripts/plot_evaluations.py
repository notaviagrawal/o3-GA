from __future__ import annotations

import argparse
import json
import math
from pathlib import Path

import matplotlib.pyplot as plt


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Plot EAFT per-evaluation runtime logs")
    parser.add_argument(
        "--log",
        required=True,
        help="Path to results/<benchmark>/log/evaluations.jsonl",
    )
    parser.add_argument(
        "--out-dir",
        help="Output directory for charts. Defaults to a plots folder next to the log.",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    log_path = Path(args.log).resolve()
    out_dir = Path(args.out_dir).resolve() if args.out_dir else log_path.parent / "plots"
    out_dir.mkdir(parents=True, exist_ok=True)

    rows = read_rows(log_path)
    candidate_rows = [
        row
        for row in rows
        if row.get("status") == "ok"
        and not row.get("is_baseline", False)
        and is_finite(row.get("runtime_seconds"))
    ]
    if not candidate_rows:
        raise SystemExit(f"No successful candidate rows found in {log_path}")

    evals = [int(row["eval_index"]) for row in candidate_rows]
    runtimes = [float(row["runtime_seconds"]) for row in candidate_rows]
    best = running_min(runtimes)

    plot_runtime_trace(evals, runtimes, best, out_dir / "runtime_trace.png")
    plot_best_runtime(evals, best, out_dir / "best_runtime.png")
    write_summary(candidate_rows, out_dir / "summary.txt")
    print(f"Wrote plots to {out_dir}")


def read_rows(path: Path) -> list[dict]:
    rows = []
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        rows.append(json.loads(line))
    return rows


def is_finite(value: object) -> bool:
    if value is None:
        return False
    try:
        return math.isfinite(float(value))
    except (TypeError, ValueError):
        return False


def running_min(values: list[float]) -> list[float]:
    best = math.inf
    out = []
    for value in values:
        best = min(best, value)
        out.append(best)
    return out


def plot_runtime_trace(evals: list[int], runtimes: list[float], best: list[float], output: Path) -> None:
    plt.figure(figsize=(11, 6))
    plt.scatter(evals, runtimes, s=13, alpha=0.35, label="candidate runtime")
    plt.plot(evals, best, linewidth=2.0, label="best so far")
    plt.title("EAFT Candidate Runtime")
    plt.xlabel("Evaluation")
    plt.ylabel("Mean process runtime (seconds)")
    plt.legend()
    plt.tight_layout()
    plt.savefig(output, dpi=160)
    plt.close()


def plot_best_runtime(evals: list[int], best: list[float], output: Path) -> None:
    plt.figure(figsize=(11, 5))
    plt.plot(evals, best, linewidth=2.0)
    plt.title("EAFT Best Runtime So Far")
    plt.xlabel("Evaluation")
    plt.ylabel("Best mean process runtime (seconds)")
    plt.tight_layout()
    plt.savefig(output, dpi=160)
    plt.close()


def write_summary(rows: list[dict], output: Path) -> None:
    best = min(rows, key=lambda row: float(row["runtime_seconds"]))
    text = [
        f"evaluations: {len(rows)}",
        f"best_eval_index: {best['eval_index']}",
        f"best_runtime_seconds: {best['runtime_seconds']}",
        f"best_candidate_id: {best['candidate_id']}",
        f"best_command: {best['command']}",
    ]
    output.write_text("\n".join(text) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
