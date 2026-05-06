#!/usr/bin/env python3
import argparse
import pandas as pd
import matplotlib.pyplot as plt
import seaborn as sns
from pathlib import Path

sns.set_style("whitegrid")

parser = argparse.ArgumentParser()
parser.add_argument("--payload-scale", type=str, choices=["b", "kb", "mb"], default="mb")
parser.add_argument("--data-scale", type=str, choices=["kb", "mb", "gb"], default="mb")
args = parser.parse_args()

SCALES = {"b": 1, "kb": 1_000, "mb": 1_000_000, "gb": 1_000_000_000}
p_div = SCALES[args.payload_scale]
d_div = SCALES[args.data_scale]

latency_divisor = 1  # in case we want to convert time units

script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent
results_dir = repo_root / "evaluations" / "results" / "gcp" / "sim"
plots_dir = results_dir / "plots"
plots_dir.mkdir(exist_ok=True)

df = pd.read_csv(results_dir / "metrics.csv")

df["algorithm"] = df["algorithm"].replace({"bracha": "Bracha", "afrt": "AFRT", "codedmbrb": "Coded MBRB"})
color_map = {"Bracha": "tab:blue", "AFRT": "tab:orange", "Coded MBRB": "tab:green"}

df["plot_latency"] = df["latency"] / latency_divisor

df_run = df.groupby(["algorithm", "n", "payload_size", "t", "d", "run"]).agg(
    total_bytes_sent=("bytes_sent", "sum"),
    avg_latency=("plot_latency", "mean")
).reset_index()

df_run["plot_data"] = df_run["total_bytes_sent"] / d_div
df_run["plot_payload"] = df_run["payload_size"] / p_div

# PLOT 1: Latency vs Payload Size
for n in sorted(df_run["n"].unique()):
    subset = df_run[(df_run["n"] == n) & (df_run["t"] == 0) & (df_run["d"] == 0)]
    if subset.empty or subset["payload_size"].nunique() < 2: continue

    fig = plt.figure(figsize=(8, 5))
    # Seaborn automatically averages the 3 runs and draws a confidence interval!
    sns.lineplot(data=subset, x="plot_payload", y="avg_latency", hue="algorithm", marker="o", palette=color_map)
    
    plt.title(f"Average Latency by Payload Size ({n} Nodes)")
    plt.xlabel(f"Payload Size ({args.payload_scale.upper()})")
    plt.ylabel("Latency (ms)")
    fig.savefig(plots_dir / f"gcp_latency_vs_payload_{n}nodes.pdf", bbox_inches="tight")
    plt.close(fig)

# PLOT 2: Total Data Sent vs Payload Size
for n in sorted(df_run["n"].unique()):
    subset = df_run[(df_run["n"] == n) & (df_run["t"] == 0) & (df_run["d"] == 0)]
    if subset.empty or subset["payload_size"].nunique() < 2: continue

    fig = plt.figure(figsize=(8, 5))
    sns.lineplot(data=subset, x="plot_payload", y="plot_data", hue="algorithm", marker="o", palette=color_map)
    
    plt.title(f"Total Network Data by Payload Size ({n} Nodes)")
    plt.xlabel(f"Payload Size ({args.payload_scale.upper()})")
    plt.ylabel(f"Total Data Sent ({args.data_scale.upper()})")
    fig.savefig(plots_dir / f"gcp_data_vs_payload_{n}nodes.pdf", bbox_inches="tight")
    plt.close(fig)

# PLOT 3 & 4: Latency & Data vs Node Count
for payload in sorted(df_run["payload_size"].unique()):
    subset = df_run[(df_run["payload_size"] == payload) & (df_run["t"] == 0) & (df_run["d"] == 0)]
    if subset.empty or subset["n"].nunique() < 2: continue
    
    p_label = f"{payload / p_div:g} {args.payload_scale.upper()}"

    # Latency vs Nodes
    fig = plt.figure(figsize=(8, 5))
    sns.lineplot(data=subset, x="n", y="avg_latency", hue="algorithm", marker="o", palette=color_map)
    plt.title(f"Average Latency by Network Size (Payload={p_label})")
    plt.xlabel("Total Nodes (n)")
    plt.xticks(sorted(subset["n"].unique()))
    plt.ylabel("Latency (ms)")
    fig.savefig(plots_dir / f"gcp_latency_vs_nodes_{payload}bytes.pdf", bbox_inches="tight")
    plt.close(fig)

    # Data vs Nodes
    fig2 = plt.figure(figsize=(8, 5))
    sns.lineplot(data=subset, x="n", y="plot_data", hue="algorithm", marker="o", palette=color_map)
    plt.title(f"Total Network Data by Network Size (Payload={p_label})")
    plt.xlabel("Total Nodes (n)")
    plt.xticks(sorted(subset["n"].unique()))
    plt.ylabel(f"Total Data Sent ({args.data_scale.upper()})")
    fig2.savefig(plots_dir / f"gcp_data_vs_nodes_{payload}bytes.pdf", bbox_inches="tight")
    plt.close(fig2)

# PLOT 5: Latency vs. Byzantine Faults (t)
for n in sorted(df_run["n"].unique()):
    for payload in sorted(df_run["payload_size"].unique()):
        subset = df_run[(df_run["n"] == n) & (df_run["payload_size"] == payload) & (df_run["d"] == 0)]
        
        if subset.empty or subset["t"].nunique() < 2:
            continue

        p_label = f"{payload / p_div:g} {args.payload_scale.upper()}"
        
        fig = plt.figure(figsize=(8, 5))
        sns.lineplot(data=subset, x="t", y="avg_latency", hue="algorithm", marker="o", palette=color_map)
        
        plt.title(f"Impact of Byzantine Faults on Latency (N={n}, Payload={p_label})")
        plt.xlabel("Number of Byzantine Nodes (t)")
        plt.xticks(sorted(subset["t"].unique()))
        plt.ylabel("Latency (ms)")
        
        # Add a vertical red dashed line at the theoretical limit (n > 3t)
        limit = (n - 1) // 3
        plt.axvline(x=limit + 0.5, color='red', linestyle='--', alpha=0.5, label="Theoretical Limit")
        
        fig.savefig(plots_dir / f"gcp_latency_vs_faults_n{n}_{payload}bytes.pdf", bbox_inches="tight")
        plt.close(fig)

# PLOT 6: Latency vs. Dropped Messages (d)
for n in sorted(df_run["n"].unique()):
    for payload in sorted(df_run["payload_size"].unique()):
        # Filter for the specific N and Payload, keeping t=0 to isolate 'd' impact
        subset = df_run[(df_run["n"] == n) & (df_run["payload_size"] == payload) & (df_run["t"] == 0)]
        
        if subset.empty or subset["d"].nunique() < 2:
            continue

        p_label = f"{payload / p_div:g} {args.payload_scale.upper()}"
        
        fig = plt.figure(figsize=(8, 5))
        sns.lineplot(data=subset, x="d", y="avg_latency", hue="algorithm", marker="o", palette=color_map)
        
        plt.title(f"Impact of Dropped Messages on Latency (N={n}, Payload={p_label})")
        plt.xlabel("Number of Dropped Messages (d)")
        plt.xticks(sorted(subset["d"].unique()))
        plt.ylabel("Latency (ms)")
        
        # Add a vertical red dashed line at the theoretical limit (n > 3t + 2d)
        # Assuming t=0 for this plot: n > 2d  => d < n/2
        limit_d = (n - 1) // 2
        plt.axvline(x=limit_d + 0.5, color='red', linestyle='--', alpha=0.5, label="Theoretical Limit")
        
        fig.savefig(plots_dir / f"gcp_latency_vs_dropped_n{n}_{payload}bytes.pdf", bbox_inches="tight")
        plt.close(fig)

print(f"Plots successfully generated in {plots_dir}")