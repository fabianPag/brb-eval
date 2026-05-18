#!/usr/bin/env python3
import argparse
import pandas as pd
import matplotlib.pyplot as plt
import seaborn as sns
import numpy as np
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

df = df[df["sender_role"] == "honest"]

df_seed_totals = (
    df.groupby(["algorithm", "n", "payload_size", "t", "d", "sim_seed"])["bytes_sent"]
    .sum()
    .reset_index()
)

df_seed_totals["algorithm"] = df_seed_totals["algorithm"].replace(
    {"bracha": "Bracha", "afrt": "AFRT", "codedmbrb": "Coded MBRB"}
)

df_seed_totals["plot_payload"] = df_seed_totals["payload_size"] / p_div
df_seed_totals["plot_data"] = df_seed_totals["bytes_sent"] / d_div

color_map = {"Bracha": "tab:blue", "AFRT": "tab:orange", "Coded MBRB": "tab:green"}
algorithm_order = ["Bracha", "AFRT", "Coded MBRB"]

# Total Data Sent vs Payload Size (for each Node count)
for n in sorted(df_seed_totals["n"].unique()):
    subset = df_seed_totals[df_seed_totals["n"] == n]

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
for orig_payload in sorted(df_seed_totals["payload_size"].unique()):
    subset = df_seed_totals[df_seed_totals["payload_size"] == orig_payload]

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


def generate_bft_bar_chart(df_subset, y_column, y_label, title, out_filename):
    if df_subset["num_silent"].nunique() < 2:
        print(f"Skipping plot for {title}. Insufficient data.")
        return

    fig, ax = plt.subplots(figsize=(10, 6))
    width = 0.18

    for i, algo in enumerate(algorithm_order):
        algo_data = df_subset[df_subset["algorithm"] == algo].sort_values("num_silent")
        if not algo_data.empty:
            offset = (i - 1) * width
            ax.bar(
                algo_data["num_silent"] + offset,
                algo_data[y_column],
                width,
                label=algo,
                color=color_map[algo],
            )

    n = df_subset["n"].iloc[0]
    ax.set_xticks(np.arange(0, n))
    ax.set_xlim(-0.6, n - 0.4)

    threshold = n / 3
    ax.axvline(x=threshold, color="gray", linestyle="--", linewidth=1.5)
    ax.text(
        threshold,
        ax.get_ylim()[1] * 1.02,
        "Silent < n/3",
        color="gray",
        ha="center",
        va="bottom",
        fontsize=11,
        style="italic",
    )

    ax.set_xlabel("Silent Nodes", fontsize=11, labelpad=10)
    ax.set_ylabel(y_label, fontsize=11, labelpad=10)
    ax.set_title(title, pad=20)
    ax.legend(loc="upper right", frameon=True)
    ax.grid(axis="y", color="lightgray", linestyle="-", linewidth=0.7, alpha=0.7)
    ax.grid(axis="x", visible=False)
    ax.spines["top"].set_visible(False)
    ax.spines["right"].set_visible(False)

    fig.savefig(out_filename, bbox_inches="tight")
    plt.close(fig)


df_delivering_per_seed = (
    df[df["delivery_time"].notna()]
    .groupby(["algorithm", "n", "payload_size", "num_silent", "sim_seed"])
    .size()
    .reset_index(name="raw_count")
)

df_delivering_per_seed["delivering_nodes"] = df_delivering_per_seed.apply(
    lambda row: (
        row["n"] - row["num_silent"]
        if row["raw_count"] > 0 and row["num_silent"] < row["n"] / 3
        else 0
    ),
    axis=1,
)

df_delivering = (
    df_delivering_per_seed.groupby(["algorithm", "n", "payload_size", "num_silent"])[
        "delivering_nodes"
    ]
    .mean()
    .reset_index()
)
df_delivering["algorithm"] = df_delivering["algorithm"].replace(
    {"bracha": "Bracha", "afrt": "AFRT", "codedmbrb": "Coded MBRB"}
)

df_avg_msgs_per_seed = (
    df.groupby(["algorithm", "n", "payload_size", "num_silent", "sim_seed"])[
        "messages_sent"
    ]
    .mean()
    .reset_index()
)

df_avg_msgs = (
    df_avg_msgs_per_seed.groupby(["algorithm", "n", "payload_size", "num_silent"])[
        "messages_sent"
    ]
    .mean()
    .reset_index()
)

df_avg_msgs["messages_sent"] = df_avg_msgs["messages_sent"] * (
    (df_avg_msgs["n"] - df_avg_msgs["num_silent"]) / df_avg_msgs["n"]
)
df_avg_msgs["algorithm"] = df_avg_msgs["algorithm"].replace(
    {"bracha": "Bracha", "afrt": "AFRT", "codedmbrb": "Coded MBRB"}
)

for n in sorted(df_delivering["n"].unique()):
    for payload_size in sorted(
        df_delivering[df_delivering["n"] == n]["payload_size"].unique()
    ):
        payload_size_mb = payload_size / 1_000_000
        payload_str = f"{payload_size_mb:g}"

        subset_delivering = df_delivering[
            (df_delivering["n"] == n) & (df_delivering["payload_size"] == payload_size)
        ]
        generate_bft_bar_chart(
            df_subset=subset_delivering,
            y_column="delivering_nodes",
            y_label="Delivering Nodes",
            title=f"Silent Node Threshold: Delivering Nodes (n={n}, Payload Size={payload_str} MB)",
            out_filename=plots_dir
            / f"figure_delivering_nodes_vs_silent_{n}nodes_{payload_size}bytes.pdf",
        )

        subset_msgs = df_avg_msgs[
            (df_avg_msgs["n"] == n) & (df_avg_msgs["payload_size"] == payload_size)
        ]
        generate_bft_bar_chart(
            df_subset=subset_msgs,
            y_column="messages_sent",
            y_label="Messages Sent (Avg. per Node)",
            title=f"Silent Node Threshold: Message Overhead (n={n}, Payload Size={payload_str} MB)",
            out_filename=plots_dir
            / f"figure_avg_messages_vs_silent_{n}nodes_{payload_size}bytes.pdf",
        )
