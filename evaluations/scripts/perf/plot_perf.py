#!/usr/bin/env python3
import pandas as pd
import matplotlib.pyplot as plt
import seaborn as sns
from pathlib import Path

script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent
results_dir = repo_root / "evaluations" / "results" / "perf"
results_file = results_dir / "metrics.csv"
plots_dir = results_dir / "plots"
plots_dir.mkdir(exist_ok=True)

df = pd.read_csv(results_file)

df["algorithm"] = df["algorithm"].replace(
    {"bracha": "Bracha", "afrt": "AFRT", "codedmbrb": "Coded MBRB"}
)

color_map = {"Bracha": "tab:blue", "AFRT": "tab:orange", "Coded MBRB": "tab:green"}
algorithm_order = ["Bracha", "AFRT", "Coded MBRB"]

for n in sorted(df["n"].unique()):
    subset = df[df["n"] == n]

    fig = plt.figure(figsize=(8, 5))
    sns.lineplot(
        data=subset,
        x="payload_size",
        y="instructions",
        hue="algorithm",
        marker="o",
        palette=color_map,
        hue_order=algorithm_order,
    )
    plt.legend(title=None)
    plt.title(f"CPU Instructions by Payload Size (n={n})")
    plt.xlabel("Payload Size (bytes)")
    plt.ylabel("Instructions (Count)")
    plt.grid(True, linestyle="--", alpha=0.6)
    out_file = plots_dir / f"figure_instructions_vs_payload_n_{n}.pdf"
    fig.savefig(out_file, bbox_inches="tight")
    print(f"Plot saved to {out_file}")

for payload_size in sorted(df["payload_size"].unique()):
    subset = df[df["payload_size"] == payload_size]

    fig = plt.figure(figsize=(8, 5))
    sns.lineplot(
        data=subset,
        x="n",
        y="instructions",
        hue="algorithm",
        marker="o",
        palette=color_map,
        hue_order=algorithm_order,
    )
    plt.legend(title=None)
    plt.title(f"CPU Instructions by Node Count (payload_size={payload_size})")
    plt.xlabel("Node Count (n)")
    plt.ylabel("Instructions (Count)")
    plt.grid(True, linestyle="--", alpha=0.6)
    out_file = plots_dir / f"figure_instructions_vs_nodes_payload_{payload_size}.pdf"
    fig.savefig(out_file, bbox_inches="tight")
    print(f"Plot saved to {out_file}")
