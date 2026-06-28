from __future__ import annotations

import re
import subprocess
from dataclasses import dataclass


DEFAULT_BASE_LEVELS = ("O1", "O2", "O3")


@dataclass(frozen=True)
class FlagSpace:
    base_levels: tuple[str, ...]
    flags: tuple[str, ...]

    @property
    def dimensions(self) -> int:
        return len(self.base_levels) + len(self.flags)


def discover_optimizer_flags(compiler: str, limit: int = 50) -> tuple[str, ...]:
    """Return GCC -f optimizer flags without the leading -f.

    This intentionally preserves GCC's order so `--flags 50` behaves like the
    old Go implementation while making the count configurable.
    """
    proc = subprocess.run(
        [compiler, "--help=optimizers"],
        check=True,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    flags: list[str] = []
    seen: set[str] = set()
    pattern = re.compile(r"^\s+(-f[a-zA-Z0-9][a-zA-Z0-9-]*)\s")
    for line in proc.stdout.splitlines():
        match = pattern.match(line)
        if not match:
            continue
        flag = match.group(1)[2:]
        if flag not in seen:
            flags.append(flag)
            seen.add(flag)
        if len(flags) >= limit:
            break
    if not flags:
        raise RuntimeError(f"No optimizer flags discovered from {compiler}")
    return tuple(flags)


def vector_to_compile_flags(vector: list[int] | tuple[int, ...], space: FlagSpace) -> list[str]:
    if len(vector) != space.dimensions:
        raise ValueError(f"Expected vector length {space.dimensions}, got {len(vector)}")

    base_bits = vector[: len(space.base_levels)]
    if any(base_bits):
        base_index = max(range(len(base_bits)), key=lambda i: base_bits[i])
    else:
        base_index = space.base_levels.index("O3") if "O3" in space.base_levels else 0

    compile_flags = [f"-{space.base_levels[base_index]}"]
    for bit, flag in zip(vector[len(space.base_levels) :], space.flags):
        compile_flags.append(f"-f{flag}" if bit else f"-fno-{flag}")
    return compile_flags


def normalize_vector(values: list[float] | tuple[float, ...], space: FlagSpace) -> list[int]:
    """Convert optimizer output into one-hot base level bits + binary flag bits."""
    if len(values) != space.dimensions:
        raise ValueError(f"Expected vector length {space.dimensions}, got {len(values)}")

    base_raw = values[: len(space.base_levels)]
    base_choice = max(range(len(base_raw)), key=lambda i: base_raw[i])
    out = [0] * len(space.base_levels)
    out[base_choice] = 1
    out.extend(1 if v >= 0.5 else 0 for v in values[len(space.base_levels) :])
    return out

