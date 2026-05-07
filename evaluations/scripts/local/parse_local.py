#!/usr/bin/env python3
import glob
import re
import pandas as pd
from pathlib import Path

script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent
results_dir = repo_root / "evaluations" / "results" / "local"
results_file = results_dir / "metrics.csv"

raw_dir = results_dir / "raw"
all_files = glob.glob(str(raw_dir / "**" / "*.txt"), recursive=True)

data = []

for filepath in all_files:
    filename = Path(filepath).stem
    parts = filename.split("_")

    algo = parts[1]
    nodes = int(parts[2].replace("nodes", ""))
    payload_size = int(parts[3].replace("bytes", ""))
    t = int(parts[4].replace("t", ""))
    d = int(parts[5].replace("d", ""))
    run = int(parts[6].replace("run", ""))
    node_id = int(parts[7].replace("node", ""))

    with open(filepath, "r") as f:
        content = f.read()
        match = re.search(r"([\d,]+)\s+instructions:u", content)

        if match:
            # Remove commas from the instruction count
            instructions = int(match.group(1).replace(",", ""))

            data.append(
                {
                    "algorithm": algo,
                    "payload_size": payload_size,
                    "n": nodes,
                    "t": t,
                    "d": d,
                    "node_id": node_id,
                    "run": run,
                    "instructions": instructions,
                }
            )

df = pd.DataFrame(data)
df.to_csv(str(results_file), index=False)
print(f"Parsed {len(df)} entries into metrics.csv")
