#!/usr/bin/env python3
import argparse
import shutil
import subprocess
import itertools
import sys
from pathlib import Path
from tqdm import tqdm

parser = argparse.ArgumentParser()
parser.add_argument("-n", "--nodes", nargs="+", type=int, required=True)
parser.add_argument("-p", "--payloads", nargs="+", type=int, required=True)
parser.add_argument("-t", "--byzantine", nargs="+", type=int, default=[0])
parser.add_argument("-d", "--dropped", nargs="+", type=int, default=[0])
args = parser.parse_args()

script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent

sim_dir = repo_root / "evaluations" / "sim"
results_dir = repo_root / "evaluations" / "results" / "sim" / "raw"
config_script = script_dir / "generate_shadow_config.sh"
bin_dir = repo_root / "bin"

algorithms = ["bracha", "afrt", "codedmbrb"]

# Build binaries if they don't exist. Feel free to remove but I thought it would be more complete for a run script
if not (bin_dir / "node").exists() or not (bin_dir / "controller").exists():
    print("Building binaries...")
    subprocess.run(["make", "build"], cwd=repo_root, check=True)

results_dir.mkdir(parents=True, exist_ok=True)
sim_dir.mkdir(parents=True, exist_ok=True)
for algo in algorithms:
    (results_dir / algo).mkdir(exist_ok=True)

configurations = list(
    itertools.product(
        algorithms, args.payloads, args.nodes, args.byzantine, args.dropped
    )
)

try:
    for config in tqdm(configurations, desc="Progress", unit="config"):
        algorithm, payload_size, node_count, byzantine, dropped = config

        tqdm.write(
            f"Simulating: {algorithm.upper()} | N={node_count} | Payload={payload_size}B | t={byzantine} | d={dropped}"
        )

        shadow_data = sim_dir / "shadow.data"

        if shadow_data.exists():
            shutil.rmtree(shadow_data)

        expected_final_state = 0 if byzantine < node_count / 3 else 1

        subprocess.run(
            [
                str(config_script),
                str(node_count),
                str(sim_dir / "shadow.yaml"),
                algorithm,
                str(payload_size),
                str(byzantine),
                str(dropped),
                "false",
                "0",
                "0",
                str(expected_final_state),
            ],
            check=True,
        )

        subprocess.run(
            ["shadow", "shadow.yaml"],
            cwd=sim_dir,
            check=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )

        metrics_file = sim_dir / "shadow.data" / "hosts" / "controller" / "metrics.json"

        if metrics_file.exists():
            save_name = (
                results_dir
                / algorithm
                / f"metrics_{algorithm}_{node_count}nodes_{payload_size}bytes_t{byzantine}_d{dropped}.json"
            )
            shutil.copy(metrics_file, save_name)
        else:
            tqdm.write(
                f"ERROR: Missing metrics for {algorithm} nodes={node_count} payload={payload_size} t={byzantine} d={dropped}"
            )

except KeyboardInterrupt:
    sys.exit(1)

finally:
    if sim_dir.exists():
        shutil.rmtree(sim_dir)
