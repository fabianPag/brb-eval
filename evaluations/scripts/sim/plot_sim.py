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
    "--data-scale",
    type=str,
    choices=["b", "kb", "mb", "gb"],
    default="mb",
    help="Scale for the total data sent axis (default: mb)",
)
args = parser.parse_args()

SCALES = {
    "b": (1, "bytes"),
    "kb": (1_000, "KB"),
    "mb": (1_000_000, "MB"),
    "gb": (1_000_000_000, "GB"),
}

p_div, p_label = SCALES[args.payload_scale]
d_div, d_label = SCALES[args.data_scale]

script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent
results_dir = repo_root / "evaluations" / "results" / "sim"
results_file = results_dir / "metrics.csv"
plots_dir = results_dir / "plots"
plots_dir.mkdir(exist_ok=True)

# Remove old plots to prevent stale data
for old_plot in plots_dir.glob("*.pdf"):
    old_plot.unlink()

df = pd.read_csv(results_file)

df_total = (
    df.groupby(["algorithm", "n", "payload_size", "t", "d"])["bytes_sent"]
    .sum()
    .reset_index()
)

df_total["algorithm"] = df_total["algorithm"].replace(
    {"bracha": "Bracha", "afrt": "AFRT", "codedmbrb": "Coded MBRB"}
)

df_total["plot_payload"] = df_total["payload_size"] / p_div
df_total["plot_data"] = df_total["bytes_sent"] / d_div

color_map = {"Bracha": "tab:blue", "AFRT": "tab:orange", "Coded MBRB": "tab:green"}
algorithm_order = ["Bracha", "AFRT", "Coded MBRB"]

# Total Data Sent vs Payload Size (for each Node count)
for n in sorted(df_total["n"].unique()):
    subset = df_total[df_total["n"] == n]

    if subset["plot_payload"].nunique() < 2:
        print(f"Skipping plot for {n} nodes. Insufficient data.")
        continue

    fig = plt.figure(figsize=(8, 5))
    sns.lineplot(
        data=subset,
        x="plot_payload",
        y="plot_data",
        hue="algorithm",
        marker="o",
        palette=color_map,
        hue_order=algorithm_order,
    )
    plt.legend(title=None)
    plt.title(f"Total Data Sent by Payload Size ({n} Nodes)")
    plt.xlabel(f"Payload Size ({p_label})")
    plt.ylabel(f"Total Data Sent ({d_label})")

    out_file = plots_dir / f"figure_data_sent_vs_payload_{n}_nodes.pdf"
    fig.savefig(out_file, bbox_inches="tight")
    plt.close(fig)
    print(f"Plot saved to {out_file}")

# Total Data Sent vs Node Count (for each Payload size)
for orig_payload in sorted(df_total["payload_size"].unique()):
    subset = df_total[df_total["payload_size"] == orig_payload]

    scaled_val = orig_payload / p_div
    val_str = f"{scaled_val:g}"

    if subset["n"].nunique() < 2:
        print(f"Skipping plot for {val_str} {p_label} payload. Insufficient data.")
        continue

    fig = plt.figure(figsize=(8, 5))
    sns.lineplot(
        data=subset,
        x="n",
        y="plot_data",
        hue="algorithm",
        marker="o",
        palette=color_map,
        hue_order=algorithm_order,
    )
    plt.legend(title=None)
    plt.title(f"Total Data Sent by Node Count (Payload Size={val_str} {p_label})")
    plt.xlabel("Node Count (n)")
    plt.xticks(sorted(subset["n"].unique()))
    plt.ylabel(f"Total Data Sent ({d_label})")

    out_file = (
        plots_dir
        / f"figure_data_sent_vs_nodes_{val_str}_{args.payload_scale}_payload.pdf"
    )
    fig.savefig(out_file, bbox_inches="tight")
    plt.close(fig)
    print(f"Plot saved to {out_file}")
