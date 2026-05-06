#!/usr/bin/env python3
import argparse
import subprocess
import sys
import signal
import itertools
from pathlib import Path
from tqdm import tqdm

parser = argparse.ArgumentParser()
parser.add_argument("-r", "--runs", type=int, default=10)
parser.add_argument("-n", "--nodes", nargs="+", type=int, required=True)
parser.add_argument("-p", "--payloads", nargs="+", type=str, required=True)
parser.add_argument("-t", "--byzantine", nargs="+", type=int, default=[0])
parser.add_argument("-d", "--dropped", nargs="+", type=int, default=[0])
args = parser.parse_args()

script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent
results_dir = repo_root / "evaluations" / "results" / "perf" / "raw"

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
    """Used to store payload size in filenames as bytes."""
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
        raise ValueError(
            f"Invalid payload format: '{s}'. Expected formats like '100B', '1KB', '1MB'."
        )


for p in args.payloads:
    try:
        to_bytes(p)
    except ValueError as e:
        print(f"Error in input arguments: {e}")
        sys.exit(1)

try:
    for config in tqdm(configurations, desc="Progress", unit="config"):
        run, algorithm, payload_size, node_count, byzantine, dropped = config

        tqdm.write(
            f"Running: {algorithm.upper()} | N={node_count} | Payload={payload_size} | t={byzantine} | d={dropped} | Run {run}/{args.runs}"
        )

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
                    "-saveMetrics=false",
                ],
                stdout=subprocess.DEVNULL,
            )

            for i in range(node_count):
                node_port = base_node_port + i
                out_file = (
                    results_dir
                    / algorithm
                    / f"perf_{algorithm}_{node_count}nodes_{to_bytes(payload_size)}bytes_t{byzantine}_d{dropped}_run{run}_node{i}.txt"
                )

                p = subprocess.Popen(
                    [
                        "perf",
                        "stat",
                        "-e",
                        "instructions:u",
                        "-o",
                        str(out_file),
                        str(repo_root / "bin" / "node"),
                        f"-id={i}",
                        f"-addr=127.0.0.1:{node_port}",
                        f"-controller={controller_addr}",
                    ],
                    stdout=subprocess.DEVNULL,
                )
                node_procs.append(p)

            # Wait for controller to finish to indicate algorithm execution done
            ctrl_proc.wait()

        finally:
            # Clean up before next run
            if ctrl_proc and ctrl_proc.poll() is None:
                ctrl_proc.terminate()
                ctrl_proc.wait()

            for p in node_procs:
                if p.poll() is None:
                    p.send_signal(signal.SIGINT)
                    p.wait()


except KeyboardInterrupt:
    sys.exit(1)
