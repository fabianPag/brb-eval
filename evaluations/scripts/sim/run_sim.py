#!/usr/bin/env python3
import argparse
import shutil
import subprocess
import itertools
import json
import sys
from pathlib import Path
import tempfile
from tqdm import tqdm

parser = argparse.ArgumentParser()
parser.add_argument("-n", "--nodes", nargs="+", type=int, required=True)
parser.add_argument("-p", "--payloads", nargs="+", type=str, required=True)
parser.add_argument("-t", nargs="+", type=int, default=[-1])
parser.add_argument("-d", nargs="+", type=int, default=[0])
parser.add_argument("--numSilent", nargs="+", type=int, default=[0])
parser.add_argument("--numMsgDrops", nargs="+", type=int, default=[0])
args = parser.parse_args()

evalEnv = "sim"
script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent
results_dir = repo_root / "evaluations" / "results" / evalEnv / "raw"
config_script = script_dir / "generate_shadow_config.sh"
bin_dir = repo_root / "bin"

algorithms = ["bracha", "afrt", "codedmbrb"]

# Build binaries if they don't exist. Feel free to remove but I thought it would be more complete for a run script
if not (bin_dir / "node").exists() or not (bin_dir / "controller").exists():
    print("Building binaries...")
    subprocess.run(["make", "build"], cwd=repo_root, check=True)

results_dir.mkdir(parents=True, exist_ok=True)
for algo in algorithms:
    (results_dir / algo).mkdir(exist_ok=True)

configurations = list(
    itertools.product(
        algorithms,
        args.payloads,
        args.nodes,
        args.t,
        args.d,
        args.numSilent,
        args.numMsgDrops,
    )
)


def to_bytes(s):
    s = s.upper().strip()
    try:
        if s.endswith("MB"):
            return int(float(s[:-2]) * 1_000_000)
        if s.endswith("KB"):
            return int(float(s[:-2]) * 1_000)
        if s.endswith("B"):
            return int(float(s[:-1]))
        raise ValueError
    except ValueError:
        raise ValueError(f"Invalid payload format: '{s}'")


for p in args.payloads:
    try:
        to_bytes(p)
    except ValueError as e:
        print(f"Error: {e}")
        sys.exit(1)

try:
    for config in tqdm(configurations, desc="Progress", unit="config"):
        algorithm, payload_size, node_count, t, d, num_silent, num_msg_drops = config

        p_bytes = to_bytes(payload_size)
        base_filename = f"{evalEnv}_{algorithm}_n{node_count}_p{p_bytes}_t{t}_d{d}_s{num_silent}_m{num_msg_drops}"
        final_json_path = results_dir / algorithm / f"{base_filename}.json"

        if final_json_path.exists():
            continue

        tqdm.write(
            f"Simulating: {algorithm.upper()} | N={node_count} | Payload={payload_size} | t={t} | d={d} | NumSilent={num_silent} | NumMsgDrops={num_msg_drops}"
        )

        tmp_path = Path(tempfile.mkdtemp(prefix="sim_"))

        try:
            yaml_path = tmp_path / "shadow.yaml"
            abs_controller = str(repo_root / "bin" / "controller")
            abs_node = str(repo_root / "bin" / "node")

            expected_final_state = 0 if t < node_count / 3 else 1

            subprocess.run(
                [
                    str(config_script),
                    str(node_count),
                    str(yaml_path),
                    algorithm,
                    str(payload_size),
                    str(t),
                    str(d),
                    "false",
                    str(num_silent),
                    str(num_msg_drops),
                    str(expected_final_state),
                    abs_controller,
                    abs_node,
                ],
                check=True,
                stdout=subprocess.DEVNULL,
            )

            subprocess.run(
                ["shadow", "shadow.yaml"],
                cwd=tmp_path,
                check=True,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )

            metrics_source = (
                tmp_path / "shadow.data" / "hosts" / "controller" / "metrics.json"
            )

            if metrics_source.exists():
                with open(metrics_source, "r") as f:
                    data = json.load(f)

                data["env"] = evalEnv

                with open(final_json_path, "w") as f:
                    json.dump(data, f, indent=2)
            else:
                tqdm.write(f"ERROR: Missing metrics for {base_filename}")

        finally:
            if tmp_path.exists():
                shutil.rmtree(tmp_path)

except KeyboardInterrupt:
    sys.exit(1)
