import argparse
import json
import statistics
import shutil
import subprocess
from pathlib import Path

import matplotlib.pyplot as plt
import pandas as pd
import seaborn as sns

from .evaluator import PolyBenchEvaluator
from .flag_sets import FlagSpace


BASELINES = {
    "O1": ["-O1"],
    "O2": ["-O2"],
    "O3": ["-O3"],
    "Ofast": ["-Ofast"],
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Final Hyperfine comparison for a tuner run")
    parser.add_argument("--run-dir", required=True)
    parser.add_argument("--runs", type=int, default=10)
    parser.add_argument("--warmup", type=int, default=2)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    run_dir = Path(args.run_dir).resolve()
    metadata = json.loads((run_dir / "metadata.json").read_text(encoding="utf-8"))
    summary = json.loads((run_dir / "summary.json").read_text(encoding="utf-8"))
    repo_root = Path(__file__).resolve().parents[1]

    space = FlagSpace(tuple(metadata["base_levels"]), tuple(metadata["flags"]))
    evaluator = PolyBenchEvaluator(
        repo_root=repo_root,
        benchmark=metadata["benchmark"],
        compiler=metadata["compiler"],
        flag_space=space,
        run_dir=run_dir,
        dataset=metadata["dataset"],
    )

    binaries: dict[str, Path] = {}
    for label, flags in BASELINES.items():
        binaries[label] = evaluator.build_final_binary(f"final_{label}", flags)
    binaries["tuned"] = evaluator.build_final_binary("final_tuned", summary["best_flags"])

    comparison_dir = run_dir / "final_compare"
    comparison_dir.mkdir(exist_ok=True)
    polybench_records = _polybench_timings(binaries, runs=args.runs, warmup=args.warmup)
    _write_comparison_chart(
        records=polybench_records,
        output_csv=comparison_dir / "polybench_comparison.csv",
        output_png=comparison_dir / "polybench_comparison.png",
        title="Final PolyBench Kernel Runtime",
    )

    hyperfine = shutil.which("hyperfine")
    if hyperfine:
        export_path = comparison_dir / "hyperfine.json"
        cmd = [
            hyperfine,
            "--warmup",
            str(args.warmup),
            "--runs",
            str(args.runs),
            "--export-json",
            str(export_path),
            *[str(path) for path in binaries.values()],
        ]
        subprocess.run(cmd, check=True)
        records = _read_hyperfine(export_path, binaries)
        title = "Final Hyperfine Process Runtime"
    else:
        records = polybench_records
        (comparison_dir / "README.txt").write_text(
            "hyperfine was not found on PATH, so this comparison uses each binary's PolyBench-reported kernel time instead.\n"
            "Install hyperfine and rerun this command for external process-level confirmation.\n",
            encoding="utf-8",
        )
        title = "Final PolyBench Kernel Runtime"

    _write_comparison_chart(
        records=records,
        output_csv=comparison_dir / "comparison.csv",
        output_png=comparison_dir / "comparison.png",
        title=title,
    )
    print(f"Final comparison written to {comparison_dir}")


def _read_hyperfine(export_path: Path, binaries: dict[str, Path]) -> list[dict[str, float | str]]:
    data = json.loads(export_path.read_text(encoding="utf-8"))
    command_to_label = {str(path): label for label, path in binaries.items()}
    rows = []
    for result in data.get("results", []):
        command = result["command"]
        rows.append(
            {
                "label": command_to_label.get(command, Path(command).name),
                "mean_seconds": result["mean"],
                "stddev_seconds": result.get("stddev", 0.0),
                "source": "hyperfine",
            }
        )
    return rows


def _polybench_timings(binaries: dict[str, Path], runs: int, warmup: int) -> list[dict[str, float | str]]:
    rows = []
    for label, binary in binaries.items():
        values = []
        for i in range(warmup + runs):
            value = _run_polybench_binary(label, binary)
            if i >= warmup:
                values.append(value)
        rows.append(
            {
                "label": label,
                "mean_seconds": statistics.mean(values),
                "stddev_seconds": statistics.stdev(values) if len(values) > 1 else 0.0,
                "source": "polybench",
            }
        )
    return rows


def _run_polybench_binary(label: str, binary: Path) -> float:
    proc = subprocess.run([str(binary)], check=True, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    for line in proc.stdout.splitlines():
        try:
            return float(line.strip())
        except ValueError:
            continue
    raise RuntimeError(f"Could not parse PolyBench time for {label}: {proc.stdout!r}")


def _write_comparison_chart(
    records: list[dict[str, float | str]],
    output_csv: Path,
    output_png: Path,
    title: str,
) -> None:
    df = pd.DataFrame(records)
    df.to_csv(output_csv, index=False)

    sns.set_theme(style="whitegrid")
    plt.figure(figsize=(9, 5))
    sns.barplot(data=df, x="label", y="mean_seconds")
    plt.title(title)
    plt.xlabel("Build")
    plt.ylabel("Mean seconds")
    plt.tight_layout()
    plt.savefig(output_png, dpi=160)
    plt.close()


if __name__ == "__main__":
    main()
