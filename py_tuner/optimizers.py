from __future__ import annotations

import math
import random
from dataclasses import dataclass
from typing import Callable, Sequence

import numpy as np

from .flag_sets import FlagSpace, normalize_vector


FitnessFn = Callable[[list[int], int, int], float]
FitnessBatch = Sequence[tuple[list[int], int, int]]
BatchFitnessFn = Callable[[FitnessBatch], list[float]]


@dataclass(frozen=True)
class OptimizerResult:
    vector: list[int]
    fitness: float


def random_vector(space: FlagSpace, rng: random.Random) -> list[int]:
    base = [0] * len(space.base_levels)
    base[rng.randrange(len(base))] = 1
    return base + [rng.randrange(2) for _ in space.flags]


def run_random_search(
    space: FlagSpace,
    budget: int,
    fitness_fn: FitnessFn,
    seed: int,
    batch_fitness_fn: BatchFitnessFn | None = None,
) -> OptimizerResult:
    rng = random.Random(seed)
    best_vector: list[int] | None = None
    best_fitness = math.inf
    candidates = [(random_vector(space, rng), 0, i) for i in range(budget)]
    fitnesses = batch_fitness_fn(candidates) if batch_fitness_fn else [
        fitness_fn(vector, generation, candidate_index)
        for vector, generation, candidate_index in candidates
    ]
    for (vector, _, _), fitness in zip(candidates, fitnesses):
        if fitness < best_fitness:
            best_fitness = fitness
            best_vector = vector
    if best_vector is None:
        raise RuntimeError("Random search did not evaluate any candidates")
    return OptimizerResult(best_vector, best_fitness)


def run_deap_ga(
    space: FlagSpace,
    generations: int,
    pop_size: int,
    fitness_fn: FitnessFn,
    seed: int,
    batch_fitness_fn: BatchFitnessFn | None = None,
) -> OptimizerResult:
    try:
        from deap import base, creator, tools
    except ImportError as exc:
        raise RuntimeError("DEAP is not installed. Run: python3 -m pip install -r py_tuner/requirements.txt") from exc

    rng = random.Random(seed)
    fitness_name = "FitnessMinFlagTuner"
    individual_name = "IndividualFlagTuner"
    if not hasattr(creator, fitness_name):
        creator.create(fitness_name, base.Fitness, weights=(-1.0,))
    if not hasattr(creator, individual_name):
        creator.create(individual_name, list, fitness=getattr(creator, fitness_name))

    toolbox = base.Toolbox()

    def make_individual() -> list[int]:
        return getattr(creator, individual_name)(random_vector(space, rng))

    toolbox.register("individual", make_individual)
    toolbox.register("population", tools.initRepeat, list, toolbox.individual)
    toolbox.register("mate", tools.cxTwoPoint)
    toolbox.register("mutate", _mutate_flag_vector, space=space, rng=rng, indpb=0.08)
    toolbox.register("select", tools.selTournament, tournsize=3)

    population = toolbox.population(n=pop_size)
    best_vector: list[int] | None = None
    best_fitness = math.inf

    for generation in range(generations):
        invalid = [ind for ind in population if not ind.fitness.valid]
        candidates = [(list(ind), generation, idx) for idx, ind in enumerate(invalid)]
        fitnesses = batch_fitness_fn(candidates) if batch_fitness_fn else [
            fitness_fn(vector, generation, candidate_index)
            for vector, generation, candidate_index in candidates
        ]
        for ind, fitness in zip(invalid, fitnesses):
            ind.fitness.values = (fitness,)
            if fitness < best_fitness:
                best_fitness = fitness
                best_vector = list(ind)

        if generation == generations - 1:
            break

        offspring = toolbox.select(population, len(population))
        offspring = list(map(toolbox.clone, offspring))
        for child1, child2 in zip(offspring[::2], offspring[1::2]):
            if rng.random() < 0.7:
                toolbox.mate(child1, child2)
                _repair_base_bits(child1, space, rng)
                _repair_base_bits(child2, space, rng)
                del child1.fitness.values
                del child2.fitness.values
        for mutant in offspring:
            if rng.random() < 0.4:
                toolbox.mutate(mutant)
                del mutant.fitness.values
        population[:] = offspring

    if best_vector is None:
        raise RuntimeError("GA did not evaluate any candidates")
    return OptimizerResult(best_vector, best_fitness)


def _mutate_flag_vector(individual: list[int], space: FlagSpace, rng: random.Random, indpb: float) -> tuple[list[int]]:
    if rng.random() < indpb:
        base_len = len(space.base_levels)
        for i in range(base_len):
            individual[i] = 0
        individual[rng.randrange(base_len)] = 1
    for i in range(len(space.base_levels), len(individual)):
        if rng.random() < indpb:
            individual[i] = 1 - individual[i]
    return (individual,)


def _repair_base_bits(individual: list[int], space: FlagSpace, rng: random.Random) -> None:
    base_len = len(space.base_levels)
    active = [i for i, bit in enumerate(individual[:base_len]) if bit]
    for i in range(base_len):
        individual[i] = 0
    if active:
        individual[rng.choice(active)] = 1
    else:
        individual[rng.randrange(base_len)] = 1


def run_pyswarms_pso(
    space: FlagSpace,
    generations: int,
    pop_size: int,
    fitness_fn: FitnessFn,
    seed: int,
    batch_fitness_fn: BatchFitnessFn | None = None,
) -> OptimizerResult:
    try:
        import pyswarms as ps
    except ImportError as exc:
        raise RuntimeError("PySwarms is not installed. Run: python3 -m pip install -r py_tuner/requirements.txt") from exc

    np.random.seed(seed)
    best_vector: list[int] | None = None
    best_fitness = math.inf
    eval_counter = 0
    current_generation = 0

    def objective(x: np.ndarray) -> np.ndarray:
        nonlocal best_vector, best_fitness, eval_counter, current_generation
        candidates = []
        for i, particle in enumerate(x):
            candidates.append((normalize_vector(particle.tolist(), space), current_generation, eval_counter + i))
        fitnesses = batch_fitness_fn(candidates) if batch_fitness_fn else [
            fitness_fn(vector, generation, candidate_index)
            for vector, generation, candidate_index in candidates
        ]
        eval_counter += len(candidates)
        scores = []
        for (vector, _, _), fitness in zip(candidates, fitnesses):
            if fitness < best_fitness:
                best_fitness = fitness
                best_vector = vector
            scores.append(fitness if math.isfinite(fitness) else 1e12)
        current_generation += 1
        return np.array(scores)

    optimizer = ps.single.GlobalBestPSO(
        n_particles=pop_size,
        dimensions=space.dimensions,
        options={"c1": 1.5, "c2": 1.5, "w": 0.6},
        bounds=(np.zeros(space.dimensions), np.ones(space.dimensions)),
    )
    optimizer.optimize(objective, iters=generations, verbose=False)
    if best_vector is None:
        raise RuntimeError("PSO did not evaluate any candidates")
    return OptimizerResult(best_vector, best_fitness)


def run_pymoo_ga(
    space: FlagSpace,
    generations: int,
    pop_size: int,
    fitness_fn: FitnessFn,
    seed: int,
) -> OptimizerResult:
    """Simple pymoo binary GA backend.

    This gives us the pymoo hook now, even if DEAP remains the default GA.
    """
    try:
        from pymoo.algorithms.soo.nonconvex.ga import GA
        from pymoo.core.problem import ElementwiseProblem
        from pymoo.optimize import minimize
        from pymoo.operators.crossover.pntx import TwoPointCrossover
        from pymoo.operators.mutation.bitflip import BitflipMutation
        from pymoo.operators.sampling.rnd import BinaryRandomSampling
    except ImportError as exc:
        raise RuntimeError("pymoo is not installed. Run: python3 -m pip install -r py_tuner/requirements.txt") from exc

    best_vector: list[int] | None = None
    best_fitness = math.inf
    eval_counter = 0

    class FlagProblem(ElementwiseProblem):
        def __init__(self) -> None:
            super().__init__(n_var=space.dimensions, n_obj=1, xl=0, xu=1, vtype=bool)

        def _evaluate(self, x, out, *args, **kwargs) -> None:
            nonlocal best_vector, best_fitness, eval_counter
            vector = normalize_vector([1.0 if bool(v) else 0.0 for v in x], space)
            generation = eval_counter // max(pop_size, 1)
            fitness = fitness_fn(vector, generation, eval_counter)
            eval_counter += 1
            if fitness < best_fitness:
                best_fitness = fitness
                best_vector = vector
            out["F"] = fitness if math.isfinite(fitness) else 1e12

    algorithm = GA(
        pop_size=pop_size,
        sampling=BinaryRandomSampling(),
        crossover=TwoPointCrossover(),
        mutation=BitflipMutation(),
        eliminate_duplicates=False,
    )
    minimize(FlagProblem(), algorithm, ("n_gen", generations), seed=seed, verbose=False)
    if best_vector is None:
        raise RuntimeError("pymoo GA did not evaluate any candidates")
    return OptimizerResult(best_vector, best_fitness)
