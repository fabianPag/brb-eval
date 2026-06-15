#!/usr/bin/env python3
import argparse
import subprocess
import sys
import signal
import itertools
import json
import re
import shutil
import tempfile
import time
from pathlib import Path
from tqdm import tqdm

parser = argparse.ArgumentParser()
parser.add_argument("-r", "--runs", type=int, default=10)
parser.add_argument("-n", "--nodes", nargs="+", type=int, required=True)
parser.add_argument("-p", "--payloads", nargs="+", type=str, required=True)
parser.add_argument("-t", nargs="+", type=int, default=[-1])
parser.add_argument("-d", nargs="+", type=int, default=[0])
parser.add_argument("--numSilent", nargs="+", type=int, default=[0])
parser.add_argument("--numMsgDrops", nargs="+", type=int, default=[0])
parser.add_argument("--no-perf", action="store_true")
args = parser.parse_args()

evalEnv = "local"
script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent
results_dir = repo_root / "evaluations" / "results" / evalEnv / "raw"

algorithms = ["bracha", "afrt", "codedmbrb"]

results_dir.mkdir(parents=True, exist_ok=True)
for algo in algorithms:
    (results_dir / algo).mkdir(exist_ok=True)

controller_addr = "127.0.0.1:8080"
base_node_port = 9090

configurations = list(
    itertools.product(
        range(1, args.runs + 1),
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

valid_configurations = []

for config in configurations:
    run, algorithm, payload_size, node_count, t, d, num_silent, num_msg_drops = config

    if node_count <= 0:
        continue

    actual_t, actual_d = t, d
    if actual_t == -1 and actual_d == -1:
        continue

    if actual_t == -1:
        actual_t = max((node_count - 2 * actual_d - 1) // 3, 0)
    if actual_d == -1:
        actual_d = max((node_count - 3 * actual_t - 1) // 2, 0)

    if 3 * actual_t + 2 * actual_d >= node_count:
        continue

    if num_silent < 0 or num_silent > node_count - 1:
        continue

    max_possible_drops = max(node_count - num_silent - 1, 0)
    if num_msg_drops < 0 or num_msg_drops > max_possible_drops:
        continue

    valid_configurations.append(
        (
            run,
            algorithm,
            payload_size,
            node_count,
            actual_t,
            actual_d,
            num_silent,
            num_msg_drops,
        )
    )

print(
    f"Skipped {len(configurations) - len(valid_configurations)} invalid combinations."
)
print(f"Starting {len(valid_configurations)} executions...")

try:
    for config in tqdm(valid_configurations, desc="Progress", unit="config"):
        run, algorithm, payload_size, node_count, t, d, num_silent, num_msg_drops = (
            config
        )

        p_bytes = to_bytes(payload_size)
        base_filename = f"{evalEnv}_{algorithm}_n{node_count}_p{p_bytes}_t{t}_d{d}_s{num_silent}_m{num_msg_drops}_run{run}"
        final_json_path = results_dir / algorithm / f"{base_filename}.json"

        if final_json_path.exists():
            continue

        tqdm.write(
            f"Running: {algorithm.upper()} | N={node_count} | Payload={payload_size} | t={t} | d={d} | NumSilent={num_silent} | NumMsgDrops={num_msg_drops} | Run {run}/{args.runs}"
        )

        tmp_run_dir = Path(tempfile.mkdtemp(prefix=f"local_run_{run}_"))
        temp_json_path = tmp_run_dir / "metrics.json"

        ctrl_proc = None
        node_procs = []

        try:
            ctrl_proc = subprocess.Popen(
                [
                    str(repo_root / "bin" / "controller"),
                    f"-addr={controller_addr}",
                    f"-n={node_count}",
                    f"-t={t}",
                    f"-d={d}",
                    f"-algorithm={algorithm}",
                    f"-payloadSize={payload_size}",
                    f"-numSilent={num_silent}",
                    f"-numMsgDrops={num_msg_drops}",
                    f"-metricsFile={str(temp_json_path)}",
                    "-gracePeriod=1s",
                ],
                stdout=subprocess.DEVNULL,
            )

            time.sleep(0.25)

            for i in range(node_count):
                node_port = base_node_port + i
                node_cmd = [
                    str(repo_root / "bin" / "node"),
                    f"-id={i}",
                    f"-addr=127.0.0.1:{node_port}",
                    f"-controller={controller_addr}",
                    "-measureRTT=false",
                ]

                if not args.no_perf:
                    temp_out = tmp_run_dir / f"perf_node{i}.txt"
                    node_cmd = [
                        "perf",
                        "stat",
                        "-e",
                        "instructions:u",
                        "-o",
                        str(temp_out),
                    ] + node_cmd

                p = subprocess.Popen(node_cmd, stdout=subprocess.DEVNULL)
                node_procs.append(p)

            # Wait for controller to finish to indicate algorithm execution done
            ctrl_proc.wait()

        finally:
            if ctrl_proc and ctrl_proc.poll() is None:
                ctrl_proc.terminate()
                ctrl_proc.wait()

            for p in node_procs:
                if p.poll() is None:
                    p.send_signal(signal.SIGINT)
                    p.wait()

            if temp_json_path.exists():
                instructions_map = {}

                if not args.no_perf:
                    for i in range(node_count):
                        perf_file = tmp_run_dir / f"perf_node{i}.txt"
                        if perf_file.exists():
                            content = perf_file.read_text()
                            match = re.search(r"([\d\,\.]+)\s+instructions:u", content)
                            if match:
                                val = match.group(1).replace(",", "").replace(".", "")
                                instructions_map[i] = int(val)

                with open(temp_json_path, "r") as f:
                    data = json.load(f)

                data["env"] = evalEnv
                data["run"] = run

                if not args.no_perf and "node_metrics" in data:
                    for node in data["node_metrics"]:
                        node_id = node.get("node_id")
                        node["cpu_instructions"] = instructions_map.get(node_id)

                with open(final_json_path, "w") as f:
                    json.dump(data, f, indent=2)

            if tmp_run_dir.exists():
                shutil.rmtree(tmp_run_dir)

except KeyboardInterrupt:
    sys.exit(1)
