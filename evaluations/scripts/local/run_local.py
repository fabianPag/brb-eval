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
parser.add_argument("-t", "--byzantine", nargs="+", type=int, default=[0])
parser.add_argument("-d", "--dropped", nargs="+", type=int, default=[0])
parser.add_argument("--no-perf", action="store_true")
args = parser.parse_args()

script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent
results_dir = repo_root / "evaluations" / "results" / "local" / "raw"

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
        args.byzantine,
        args.dropped,
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
        run, algorithm, payload_size, node_count, byzantine, dropped = config

        p_bytes = to_bytes(payload_size)
        base_filename = f"metrics_{algorithm}_{node_count}nodes_{p_bytes}bytes_t{byzantine}_d{dropped}_run{run}"
        final_json_path = results_dir / algorithm / f"{base_filename}.json"

        if final_json_path.exists():
            continue

        tqdm.write(
            f"Running: {algorithm.upper()} | N={node_count} | Payload={payload_size} | t={byzantine} | d={dropped} | Run {run}/{args.runs}"
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
                    f"-t={byzantine}",
                    f"-d={dropped}",
                    f"-algorithm={algorithm}",
                    f"-payloadSize={payload_size}",
                    "-gracePeriod=1s",
                    f"-metricsFile={str(temp_json_path)}",
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

                data["env"] = "local"
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
