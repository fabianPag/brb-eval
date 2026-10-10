# BRB-Eval

BRB-Eval is a distributed benchmarking platform for Byzantine Reliable Broadcast (BRB) algorithms written in Go.

It coordinates peer-to-peer nodes to measure network overhead, CPU and memory costs, and latency, both under normal operation and with injected faults.

Implemented algorithms:
* [**Bracha**](https://link.springer.com/book/10.1007/978-3-319-94141-7): Baseline asynchronous Byzantine reliable broadcast (implemented based on Raynal's formulation).
* [**AFRT**](https://www.sciencedirect.com/science/article/abs/pii/S0304397523004231): Signature-based message-adversary-tolerant broadcast (MBRB) using Ed25519.
* [**Coded MBRB**](https://drops.dagstuhl.de/entities/document/10.4230/LIPIcs.OPODIS.2024.14): Communication-efficient MBRB using Reed-Solomon coding, KZG commitments, and threshold signatures.

## Architecture

* **Controller** (`cmd/controller`, `internal/controller`): Coordinates experiments via HTTP barriers (registration, readiness, completion) and collects node metrics.
* **Nodes** (`cmd/node`, `internal/node`):
  * **Transport** (`transport/`): Handles pairwise TCP connections, allowing other layers to address peers by node ID.
  * **Manager** (`manager/`): Raw byte messaging (`Broadcast`, `SendBatch`) and fault-injection handling (silent nodes, message drops, sender equivocation) without modifying algorithm code.
  * **Metrics** (`metrics/`): Atomic counters for messages, bytes, execution timestamps, and memory stats.
  * **Algorithm** (`algorithm/`): Message serialization (gob) and protocol logic (`bracha`, `afrt`, `codedmbrb`), each implementing the `brbAlgorithm` interface.

## Building

Requires **Go 1.26+**.

```bash
make build
```

Compiles `bin/node` and `bin/controller`.

## Running Experiments

Experiments are run using the Python scripts in `evaluations/scripts/` (requires `pandas`, `numpy`, `matplotlib`, `seaborn`, `tqdm`). Raw metrics are saved as JSON in `evaluations/results/<env>/raw/<algorithm>/`.

### Common experiment arguments
The run scripts accept multiple values per flag to run sweeps over:
* `-n`: Number of nodes (e.g. `-n 10 20 30`).
* `-p`: Payload sizes (e.g. `-p 100KB 1MB 8MB`).
* `-t`: Byzantine fault threshold (`-1` to automatically infer maximum tolerated).
* `-d`: Message adversary threshold (`-1` to automatically infer maximum tolerated. -t and -d cannot both be -1 simultaneously).
* `--numSilent`: Number of silent nodes to inject (drops traffic while keeping TCP connections open).
* `--numMsgDrops`: Number of outgoing messages randomly dropped each time a node broadcasts to peers.

### Local execution
Runs nodes on localhost and profiles CPU instructions (via `perf`) and Go runtime memory:

```bash
./evaluations/scripts/local/run_local.py -n 10 -p 100KB 1MB -r 5 -t -1 -d 0
```

Script-specific flags:
* `-r`: Repetitions per configuration (default: `10`).
* `--no-perf`: Disables CPU instruction recording with `perf`.

### Simulation (Shadow)
Runs nodes in the [Shadow](https://shadow.github.io/) network simulator (requires `shadow` installed and available in your `PATH`):

```bash
./evaluations/scripts/sim/run_sim.py -n 10 20 30 -p 1MB -t -1 -d 0 --seed 42
```

Script-specific flags:
* `--seed`: Random seed(s) for simulation reproducibility (default: `42`).
* `--twinsSender`: Enable partition-based equivocation (`true` / `false`, default: `false`).

## Parsing & Plotting

### 1. Parse metrics
Aggregates raw JSON metrics into `evaluations/results/<env>/metrics.csv` and verifies safety properties (checks for duplicate deliveries, conflicting hashes, and invalid payloads).

```bash
./evaluations/scripts/parse_metrics.py --env <env>
```

Flags:
* `--env`: Target environment (`local`, `sim`, `gcp`, `fabric`, or `all` to parse everything; default: `all`).

### 2. Plot results
Generates PDF plots in `evaluations/results/<env>/plots/`:

```bash
./evaluations/scripts/plot.py <preset> --env <env> [options]
```

Flags:
* `--env`: Environment to plot data for (`local`, `sim`, `gcp`, `fabric`).
* `-n`: Filter by node count(s).
* `-p`: Filter by payload size(s).
* `--payload-scale <b|kb|mb|gb>`: Scale factor for payload size axis (default `mb`).
* `--data-scale <b|kb|mb|gb>`: Scale factor for data amount axis (default `mb`).
* `--instr-scale <none|thousands|millions|billions>`: Scale for instruction count axis (default `millions`).
* `--hide-minmax`: Hide min/max overlay lines in line plots.

Available plot presets:
* **Payload scaling** (`*_vs_payload`):  
  `latency_vs_payload`, `cpu_instructions_vs_payload`, `data_sent_vs_payload`, `peak_heap_vs_payload`, `cumulative_alloc_vs_payload`
* **Node scaling** (`*_vs_nodes`):  
  `latency_vs_nodes`, `cpu_instructions_vs_nodes`, `data_sent_vs_nodes`, `messages_vs_nodes`, `peak_heap_vs_nodes`, `cumulative_alloc_vs_nodes`
* **Fault injection** (`*_vs_silent`):  
  `delivering_vs_silent`, `messages_vs_silent_bar`, `messages_vs_silent_box`, `cpu_instructions_vs_silent`, `peak_heap_vs_silent`, `cumulative_alloc_vs_silent`

Examples:
```bash
# Plot CPU instructions vs payload size for 10 local nodes using 100KB and 1MB payloads
./evaluations/scripts/plot.py cpu_instructions_vs_payload --env local -n 10 -p 100KB 1MB

# Plot total network data sent across 10, 20, and 30 simulated nodes with a 1MB payload
./evaluations/scripts/plot.py data_sent_vs_nodes --env sim -n 10 20 30 -p 1MB
```
