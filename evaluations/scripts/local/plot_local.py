#!/usr/bin/env python3
import argparse
import pandas as pd
import matplotlib.pyplot as plt
import seaborn as sns
from pathlib import Path

sns.set_style("whitegrid")

parser = argparse.ArgumentParser()
parser.add_argument(
    "--payload-scale",
    type=str,
    choices=["b", "kb", "mb", "gb"],
    default="mb",
    help="Scale for the payload size axis (default: mb)",
)
parser.add_argument(
    "--instr-scale",
    type=str,
    choices=["none", "thousands", "millions", "billions"],
    default="billions",
    help="Scale for the instructions axis (default: billions)",
)
args = parser.parse_args()

PAYLOAD_SCALES = {
    "b": (1, "bytes"),
    "kb": (1_000, "KB"),
    "mb": (1_000_000, "MB"),
    "gb": (1_000_000_000, "GB"),
}

INSTR_SCALES = {
    "none": (1, "Count"),
    "thousands": (1_000, "Count (Thousands)"),
    "millions": (1_000_000, "Count (Millions)"),
    "billions": (1_000_000_000, "Count (Billions)"),
}

p_div, p_label = PAYLOAD_SCALES[args.payload_scale]
i_div, i_label = INSTR_SCALES[args.instr_scale]

script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent
results_dir = repo_root / "evaluations" / "results" / "local"
results_file = results_dir / "metrics.csv"
plots_dir = results_dir / "plots"
plots_dir.mkdir(exist_ok=True)

# Remove old plots to prevent stale data
for old_plot in plots_dir.glob("*.pdf"):
    old_plot.unlink()

df = pd.read_csv(results_file)

df["algorithm"] = df["algorithm"].replace(
    {"bracha": "Bracha", "afrt": "AFRT", "codedmbrb": "Coded MBRB"}
)

df["plot_payload"] = df["payload_size"] / p_div
df["plot_instructions"] = df["instructions"] / i_div

color_map = {"Bracha": "tab:blue", "AFRT": "tab:orange", "Coded MBRB": "tab:green"}
algorithm_order = ["Bracha", "AFRT", "Coded MBRB"]

# Instructions vs Payload Size (for each Node count)
for n in sorted(df["n"].unique()):
    subset = df[df["n"] == n]

    if subset["plot_payload"].nunique() < 2:
        print(f"Skipping plot for {n} nodes. Insufficient data.")
        continue

    fig = plt.figure(figsize=(8, 5))
    sns.lineplot(
        data=subset,
        x="plot_payload",
        y="plot_instructions",
        hue="algorithm",
        marker="o",
        palette=color_map,
        hue_order=algorithm_order,
    )
    plt.legend(title=None)
    plt.title(f"CPU Instruction Count by Payload Size ({n} Nodes)")
    plt.xlabel(f"Payload Size ({p_label})")
    plt.ylabel(f"Instruction {i_label}")

    out_file = plots_dir / f"figure_instructions_vs_payload_{n}_nodes.pdf"
    fig.savefig(out_file, bbox_inches="tight")
    plt.close(fig)
    print(f"Plot saved to {out_file}")

# Instructions vs Node Count (for each Payload size)
for orig_payload in sorted(df["payload_size"].unique()):
    subset = df[df["payload_size"] == orig_payload]

    scaled_val = orig_payload / p_div
    val_str = f"{scaled_val:g}"

    if subset["n"].nunique() < 2:
        print(f"Skipping plot for {val_str} {p_label} payload. Insufficient data.")
        continue

    fig = plt.figure(figsize=(8, 5))
    sns.lineplot(
        data=subset,
        x="n",
        y="plot_instructions",
        hue="algorithm",
        marker="o",
        palette=color_map,
        hue_order=algorithm_order,
    )
    plt.legend(title=None)
    plt.title(f"CPU Instruction Count by Node Count (Payload Size={val_str} {p_label})")
    plt.xlabel("Node Count (n)")
    plt.xticks(sorted(subset["n"].unique()))
    plt.ylabel(f"Instruction {i_label}")

    out_file = (
        plots_dir
        / f"figure_instructions_vs_nodes_{val_str}_{args.payload_scale}_payload.pdf"
    )
    fig.savefig(out_file, bbox_inches="tight")
    plt.close(fig)
    print(f"Plot saved to {out_file}")
