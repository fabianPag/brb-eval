#!/usr/bin/env python3
import glob
import json
import sys
import pandas as pd
import argparse
from pathlib import Path


def parse_environment(env_name):
    script_dir = Path(__file__).resolve().parent
    repo_root = script_dir.parent
    results_dir = repo_root / "results" / env_name
    results_file = results_dir / "metrics.csv"
    raw_dir = results_dir / "raw"

    if not raw_dir.exists():
        print(f"[{env_name}] Directory {raw_dir} does not exist. Skipping.")
        return

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
            run = json_data.get("run", None)
            sim_seed = json_data.get("sim_seed", None)

            start_time_str = json_data.get("start_time")
            end_time_str = json_data.get("end_time")
            global_latency_ms = None
            if start_time_str and end_time_str:
                start_time = pd.to_datetime(start_time_str)
                end_time = pd.to_datetime(end_time_str)
                global_latency_ms = (end_time - start_time).total_seconds() * 1000.0

            designated_sender = json_data.get("designated_sender", None)
            num_silent = json_data.get("num_silent", None)
            num_msg_drops = json_data.get("num_msg_drops", None)

            sender_sent_hash = None
            sender_role = "honest"
            if "node_metrics" in json_data:
                for m in json_data["node_metrics"]:
                    if m.get("node_id") == designated_sender:
                        sender_role = m.get("role", "honest")
                        sender_sent_hash = m.get("sent_hash")
                        break

            delivered_hashes = set()
            properties_valid = True

            if "node_metrics" in json_data:
                for metrics in json_data["node_metrics"]:
                    node_id = metrics.get("node_id", None)
                    role = metrics.get("role", None)
                    is_delivered = metrics.get("delivered", False)
                    delivery_count = metrics.get("delivery_count", 0)

                    if delivery_count > 1:
                        properties_valid = False
                        print(
                            f"[{env_name}] Property violation (Integrity/No-duplication): Node {node_id} delivered {delivery_count} times!"
                        )

                    if is_delivered:
                        delivered_hashes.add(metrics.get("delivered_hash"))

                    data.append(
                        {
                            "env": env_name,
                            "run": run,
                            "sim_seed": sim_seed,
                            "algorithm": algo,
                            "payload_size": payload_size,
                            "n": n,
                            "t": t,
                            "d": d,
                            "designated_sender": designated_sender,
                            "sender_role": sender_role,
                            "num_silent": num_silent,
                            "num_msg_drops": num_msg_drops,
                            "global_start_time": start_time_str,
                            "global_end_time": end_time_str,
                            "global_latency": global_latency_ms,
                            "node_id": node_id,
                            "role": role,
                            "delivered": is_delivered,
                            "delivery_count": delivery_count,
                            "first_message_time": metrics.get(
                                "first_message_time", None
                            ),
                            "delivery_time": metrics.get("delivery_time", None),
                            "messages_received": metrics.get("messages_received", 0),
                            "bytes_received": metrics.get("bytes_received", 0),
                            "messages_sent": metrics.get("messages_sent", 0),
                            "bytes_sent": metrics.get("bytes_sent", 0),
                            "cpu_instructions": metrics.get("cpu_instructions", None),
                        }
                    )
            else:
                print(
                    f"[{env_name}] Warning: 'node_metrics' key not found in {filepath}"
                )

            if len(delivered_hashes) > 1:
                properties_valid = False
                print(
                    f"[{env_name}] Property violation (No-duplicity): {len(delivered_hashes)} different payloads delivered!"
                )

            elif len(delivered_hashes) == 1:
                only_hash = next(iter(delivered_hashes))

                if sender_role == "honest":
                    if only_hash != sender_sent_hash:
                        properties_valid = False
                        print(
                            f"[{env_name}] Property violation (Validity): Delivered payload does not match sender!"
                        )

            if not properties_valid:
                print(
                    f"[{env_name}] Property violations detected in {filepath}. Exiting."
                )
                sys.exit(1)

    if data:
        df = pd.DataFrame(data)
        df.to_csv(str(results_file), index=False)
        print(f"[{env_name}] Parsed {len(df)} entries into {results_file}")
    else:
        print(f"[{env_name}] No valid metrics found to parse.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--env",
        type=str,
        choices=["gcp", "local", "sim", "all"],
        default="all",
    )
    args = parser.parse_args()

    if args.env == "all":
        for e in ["gcp", "local", "sim"]:
            parse_environment(e)
    else:
        parse_environment(args.env)
