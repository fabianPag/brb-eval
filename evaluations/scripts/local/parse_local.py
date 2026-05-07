#!/usr/bin/env python3
import glob
import json
import pandas as pd
from pathlib import Path

script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent
results_dir = repo_root / "evaluations" / "results" / "local"
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
        t = json_data.get("t", 0)
        d = json_data.get("d", 0)

        run = json_data.get("run", -1)
        env = json_data.get("env", "unknown")

        if "node_metrics" in json_data:
            for metrics in json_data["node_metrics"]:
                data.append(
                    {
                        "algorithm": algo,
                        "payload_size": payload_size,
                        "n": n,
                        "t": t,
                        "d": d,
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
                        "cpu_instructions": metrics.get("cpu_instructions", None),
                    }
                )
        else:
            print("Warning: 'node_metrics' key not found")

df = pd.DataFrame(data)
df.to_csv(str(results_file), index=False)
print(f"Parsed {len(df)} entries into metrics.csv")
