#!/usr/bin/env python3
import glob
import json
import pandas as pd
from pathlib import Path

script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent
results_dir = repo_root / "evaluations" / "results" / "gcp"
results_file = results_dir / "metrics.csv"
raw_dir = results_dir / "raw"
all_files = glob.glob(str(raw_dir / "**" / "*.json"), recursive=True)

data = []

for filepath in all_files:
    with open(filepath, "r") as f:
        json_data = json.load(f)

        algo = json_data.get("algorithm", "unknown")
        payload_size = json_data.get("payload_size", 0)
        n = json_data.get("n", 0)

        run = json_data.get("run", -1)
        env = json_data.get("env", "unknown")

        start_time_str = json_data.get("start_time")
        end_time_str = json_data.get("end_time")

        global_latency_ms = None
        if start_time_str and end_time_str:
            start_time = pd.to_datetime(start_time_str)
            end_time = pd.to_datetime(end_time_str)
            global_latency_ms = (end_time - start_time).total_seconds() * 1000.0

        if "node_metrics" in json_data:
            for metrics in json_data["node_metrics"]:
                data.append(
                    {
                        "algorithm": algo,
                        "payload_size": payload_size,
                        "n": n,
                        "designated_sender": json_data.get("designated_sender", 0),
                        "global_start_time": start_time_str,
                        "global_end_time": end_time_str,
                        "global_latency": global_latency_ms,
                        "node_id": metrics.get("node_id", -1),
                        "role": metrics.get("role", "unknown"),
                        "first_message_time": metrics.get("first_message_time"),
                        "delivery_time": metrics.get("delivery_time"),
                        "messages_received": metrics.get("messages_received", 0),
                        "bytes_received": metrics.get("bytes_received", 0),
                        "messages_sent": metrics.get("messages_sent", 0),
                        "bytes_sent": metrics.get("bytes_sent", 0),
                        "run": run,
                        "env": env,
                    }
                )
        else:
            print("Warning: 'node_metrics' key not found")

df = pd.DataFrame(data)
df.to_csv(str(results_file), index=False)
print(f"Parsed {len(df)} entries into {results_file}")