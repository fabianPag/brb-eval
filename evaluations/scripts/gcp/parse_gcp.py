#!/usr/bin/env python3
import glob
import json
import pandas as pd
import re
from pathlib import Path

script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent
results_dir = repo_root / "evaluations" / "results" / "gcp" / "sim"
results_file = results_dir / "metrics.csv"
raw_dir = results_dir / "raw"

all_files = glob.glob(str(raw_dir / "**" / "*.json"), recursive=True)
data = []

for filepath in all_files:
    filename = Path(filepath).stem
    
    # Extract the run number from the filename (e.g., ..._run1, ..._run2)
    match = re.search(r'run(\d+)', filename)
    run_idx = int(match.group(1)) if match else 1

    with open(filepath, "r") as f:
        json_data = json.load(f)

        algo = json_data.get("algorithm", "unknown")
        payload_size = json_data.get("payload_size", 0)
        n = json_data.get("n", 0)
        t = json_data.get("t", 0)
        d = json_data.get("d", 0)

        for metrics in json_data.get("node_metrics", []):
            first_msg = metrics.get("first_message_time")
            delivery = metrics.get("delivery_time")
            
            latency_ms = None
            if delivery and first_msg:
                t1 = pd.to_datetime(first_msg)
                t2 = pd.to_datetime(delivery)
                
                latency_ms = (t2 - t1).total_seconds() * 1000.0

            data.append({
                "algorithm": algo,
                "payload_size": payload_size,
                "n": n,
                "t": t,
                "d": d,
                "run": run_idx,
                "node_id": metrics.get("node_id", -1),
                "role": metrics.get("role", "unknown"),
                "latency": latency_ms,
                "bytes_sent": metrics.get("bytes_sent", 0),
            })

df = pd.DataFrame(data)
# Create directory if it doesn't exist and save
results_dir.mkdir(parents=True, exist_ok=True)
df.to_csv(str(results_file), index=False)
print(f"Parsed {len(df)} entries into {results_file}")