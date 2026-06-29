from __future__ import annotations

import hashlib
import math
import shutil
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path

from .flag_sets import FlagSpace, vector_to_compile_flags
from .logging_utils import append_jsonl


@dataclass(frozen=True)
class EvalResult:
    candidate_id: str
    correct: bool
    runtime: float
    compile_flags: list[str]
    reason: str = ""
    binary_path: str = ""


class PolyBenchEvaluator:
    def __init__(
        self,
        repo_root: Path,
        benchmark: str,
        compiler: str,
        flag_space: FlagSpace,
        run_dir: Path,
        dataset: str = "MINI_DATASET",
        correctness_mode: str = "string",
        timeout_seconds: int = 120,
    ) -> None:
        self.repo_root = repo_root
        self.benchmark = benchmark
        self.compiler = compiler
        self.flag_space = flag_space
        self.run_dir = run_dir
        self.dataset = dataset
        self.correctness_mode = correctness_mode
        self.timeout_seconds = timeout_seconds

        self.source = repo_root / "data" / "Polybench" / "datamining" / f"{benchmark}.c"
        self.utilities = repo_root / "data" / "Polybench" / "utilities"
        self.polybench_c = self.utilities / "polybench.c"
        self.bin_dir = run_dir / "bin"
        self.log_path = run_dir / "evaluations.jsonl"
        self.reference_path = run_dir / "reference.dump"

        if not self.source.exists():
            raise FileNotFoundError(f"Benchmark source not found: {self.source}")

    def ensure_dirs(self) -> None:
        self.bin_dir.mkdir(parents=True, exist_ok=True)

    def compile_binary(
        self,
        compile_flags: list[str],
        output: Path,
        extra_defines: list[str] | None = None,
    ) -> tuple[bool, str]:
        extra_defines = extra_defines or []
        cmd = [
            self.compiler,
            *compile_flags,
            f"-D{self.dataset}",
            *extra_defines,
            str(self.source),
            str(self.polybench_c),
            "-I",
            str(self.utilities),
            "-lm",
            "-o",
            str(output),
        ]
        proc = subprocess.run(
            cmd,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=self.timeout_seconds,
        )
        return proc.returncode == 0, proc.stdout + proc.stderr

    def run_binary(self, binary: Path) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [str(binary)],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=self.timeout_seconds,
        )

    def build_reference(self) -> None:
        self.ensure_dirs()
        reference_bin = self.bin_dir / "reference_dump"
        ok, output = self.compile_binary(
            ["-O0"],
            reference_bin,
            extra_defines=["-DPOLYBENCH_DUMP_ARRAYS"],
        )
        if not ok:
            raise RuntimeError(f"Reference compile failed:\n{output}")
        proc = self.run_binary(reference_bin)
        if proc.returncode != 0:
            raise RuntimeError(f"Reference run failed:\n{proc.stdout}\n{proc.stderr}")
        # PolyBench dumps arrays to stderr.
        self.reference_path.write_text(proc.stderr, encoding="utf-8")

    def parse_polybench_time(self, stdout: str) -> float:
        for line in stdout.splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                value = float(line)
            except ValueError:
                continue
            if math.isfinite(value) and value >= 0:
                return value
        raise ValueError(f"No PolyBench timing value found in stdout: {stdout!r}")

    def candidate_id(self, vector: list[int]) -> str:
        payload = ",".join(str(v) for v in vector).encode("utf-8")
        return hashlib.sha1(payload).hexdigest()[:16]

    def evaluate(
        self,
        vector: list[int],
        generation: int,
        algorithm: str,
        candidate_index: int,
        repeats: int = 3,
    ) -> EvalResult:
        self.ensure_dirs()
        if not self.reference_path.exists():
            self.build_reference()

        compile_flags = vector_to_compile_flags(vector, self.flag_space)
        candidate_id = self.candidate_id(vector)
        binary_prefix = f"g{generation:04d}_c{candidate_index:04d}_{candidate_id}"
        dump_bin = self.bin_dir / f"{binary_prefix}_dump"
        time_bin = self.bin_dir / f"{binary_prefix}_time"

        started = time.time()

        ok, output = self.compile_binary(
            compile_flags,
            dump_bin,
            extra_defines=["-DPOLYBENCH_DUMP_ARRAYS"],
        )
        if not ok:
            result = EvalResult(candidate_id, False, math.inf, compile_flags, "compile_failed")
            self._log(result, vector, generation, algorithm, candidate_index, started, output)
            return result

        proc = self.run_binary(dump_bin)
        if proc.returncode != 0:
            result = EvalResult(candidate_id, False, math.inf, compile_flags, "correctness_run_failed")
            self._log(result, vector, generation, algorithm, candidate_index, started, proc.stdout + proc.stderr)
            return result

        reference = self.reference_path.read_text(encoding="utf-8")
        if proc.stderr != reference:
            result = EvalResult(candidate_id, False, math.inf, compile_flags, "wrong_output")
            self._log(result, vector, generation, algorithm, candidate_index, started, "")
            return result

        ok, output = self.compile_binary(
            compile_flags,
            time_bin,
            extra_defines=["-DPOLYBENCH_TIME"],
        )
        if not ok:
            result = EvalResult(candidate_id, False, math.inf, compile_flags, "timing_compile_failed")
            self._log(result, vector, generation, algorithm, candidate_index, started, output)
            return result

        timings: list[float] = []
        for _ in range(repeats):
            proc = self.run_binary(time_bin)
            if proc.returncode != 0:
                result = EvalResult(candidate_id, False, math.inf, compile_flags, "timing_run_failed")
                self._log(result, vector, generation, algorithm, candidate_index, started, proc.stdout + proc.stderr)
                return result
            try:
                timings.append(self.parse_polybench_time(proc.stdout))
            except ValueError as exc:
                result = EvalResult(candidate_id, False, math.inf, compile_flags, "timing_parse_failed")
                self._log(result, vector, generation, algorithm, candidate_index, started, str(exc))
                return result

        runtime = sum(timings) / len(timings)
        result = EvalResult(candidate_id, True, runtime, compile_flags, binary_path=str(time_bin))
        self._log(result, vector, generation, algorithm, candidate_index, started, "", timings)
        return result

    def build_final_binary(self, name: str, compile_flags: list[str]) -> Path:
        self.ensure_dirs()
        out = self.bin_dir / name
        ok, output = self.compile_binary(compile_flags, out, extra_defines=["-DPOLYBENCH_TIME"])
        if not ok:
            raise RuntimeError(f"Final binary compile failed for {name}:\n{output}")
        return out

    def _log(
        self,
        result: EvalResult,
        vector: list[int],
        generation: int,
        algorithm: str,
        candidate_index: int,
        started: float,
        details: str,
        timings: list[float] | None = None,
    ) -> None:
        append_jsonl(
            self.log_path,
            {
                "algorithm": algorithm,
                "benchmark": self.benchmark,
                "candidate_id": result.candidate_id,
                "candidate_index": candidate_index,
                "compile_flags": result.compile_flags,
                "correct": result.correct,
                "dataset": self.dataset,
                "elapsed_wall": time.time() - started,
                "generation": generation,
                "reason": result.reason,
                "runtime": result.runtime if math.isfinite(result.runtime) else None,
                "timings": timings or [],
                "vector": vector,
            },
        )


def require_tool(name: str) -> str:
    path = shutil.which(name)
    if not path:
        raise RuntimeError(f"Required tool not found on PATH: {name}")
    return path
