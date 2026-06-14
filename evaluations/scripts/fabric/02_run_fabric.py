#!/usr/bin/env python3
import argparse
import itertools
import json
import os
import random
import sys
import time
import concurrent.futures
from pathlib import Path
from tqdm import tqdm
from fabrictestbed_extensions.fablib.fablib import FablibManager


parser = argparse.ArgumentParser()
parser.add_argument("-r", "--runs", type=int, default=10)
parser.add_argument("-n", "--nodes", nargs="+", type=int, required=True)
parser.add_argument("-p", "--payloads", nargs="+", type=str, required=True)
parser.add_argument("-t", nargs="+", type=int, default=[-1])
parser.add_argument("-d", nargs="+", type=int, default=[0])
parser.add_argument("--numSilent", nargs="+", type=int, default=[0])
parser.add_argument("--numMsgDrops", nargs="+", type=int, default=[0])
parser.add_argument(
    "--slice", type=str, required=True, help="Name of the FABRIC slice to run on"
)
args = parser.parse_args()

evalEnv = "fabric"
script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent
results_dir = repo_root / "evaluations" / "results" / evalEnv / "raw"

algorithms = ["bracha", "afrt", "codedmbrb"]

results_dir.mkdir(parents=True, exist_ok=True)
for algo in algorithms:
    (results_dir / algo).mkdir(exist_ok=True)


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
    if num_msg_drops < 0 or num_msg_drops > max(node_count - num_silent - 1, 0):
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
    fablib = FablibManager()
    slice = fablib.get_slice(name=args.slice)
    print(f"Connected to slice: {args.slice}")
except Exception as e:
    print(f"Error connecting to slice {args.slice}: {e}")
    sys.exit(1)

node_map = {node.get_name(): node for node in slice.get_nodes()}

if max(args.nodes) > len(node_map) - 1:
    print(
        f"Tried to run test with {max(args.nodes)} nodes, but slice only has {len(node_map) - 1} nodes."
    )
    sys.exit(1)


ip_cache = {}
nodes = slice.get_nodes()


def fetch_ip(node):
    iface = node.get_interfaces()[0]
    return node.get_name(), str(iface.get_ip_addr())


max_concurrent_workers = min(len(nodes), 10)

with concurrent.futures.ThreadPoolExecutor(
    max_workers=max_concurrent_workers
) as executor:
    future_to_node = {executor.submit(fetch_ip, node): node for node in nodes}

    for future in tqdm(
        concurrent.futures.as_completed(future_to_node),
        total=len(nodes),
        desc="Caching node IPs",
    ):
        try:
            name, ip = future.result()
            ip_cache[name] = ip
        except Exception as e:
            node_name = future_to_node[future].get_name()
            tqdm.write(f"Error fetching IP for {node_name}: {e}")
            sys.exit(1)

ctrl_node = node_map["Controller"]
ctrl_ip = ip_cache["Controller"]


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

        active_nodes = [ctrl_node] + [node_map[f"Node{i}"] for i in range(node_count)]

        def cleanup(node):
            time.sleep(random.uniform(0.1, 1.0))

            node.execute(
                "pkill -f controller; pkill -f node; rm -f metrics.json",
                quiet=True,
            )

        with concurrent.futures.ThreadPoolExecutor(
            max_workers=max_concurrent_workers
        ) as executor:
            list(executor.map(cleanup, active_nodes))

        go_cmd = (
            f"timeout 120 ./controller -addr=0.0.0.0:8080 -n={node_count} -algorithm={algorithm} "
            f"-payloadSize={payload_size} -t={t} -d={d} -numSilent={num_silent} "
            f"-numMsgDrops={num_msg_drops} -metricsFile=metrics.json > /dev/null 2>&1"
        )

        def run_controller():
            ctrl_node.execute(go_cmd, quiet=True)

        def launch_node(i):
            time.sleep(random.uniform(0.1, 1.0))
            node_map[f"Node{i}"].execute(
                f"nohup ./node -id={i} -addr={ip_cache[f'Node{i}']}:{9090 + i} -controller={ctrl_ip}:8080 > /dev/null 2>&1 &",
                quiet=True,
            )

        with concurrent.futures.ThreadPoolExecutor(
            max_workers=max_concurrent_workers
        ) as executor:
            ctrl_future = executor.submit(run_controller)
            list(executor.map(launch_node, range(node_count)))
            ctrl_future.result()

        try:
            temp_download_path = f"/tmp/{base_filename}.json"
            ctrl_node.download_file(temp_download_path, "metrics.json")

            with open(temp_download_path, "r") as f:
                data = json.load(f)

            data["env"] = evalEnv
            data["run"] = run

            with open(final_json_path, "w") as f:
                json.dump(data, f, indent=2)

            os.remove(temp_download_path)

        except Exception as e:
            tqdm.write(f"Error downloading metrics: {e}")

except KeyboardInterrupt:
    sys.exit(1)
