#!/usr/bin/env python3
import argparse
import pandas as pd
import matplotlib.pyplot as plt
import seaborn as sns
import numpy as np
from pathlib import Path
import sys

sns.set_style("whitegrid")

color_map = {"Bracha": "tab:blue", "AFRT": "tab:orange", "Coded MBRB": "tab:green"}
algorithm_order = ["Bracha", "AFRT", "Coded MBRB"]

PLOT_PRESETS = {
    "latency_vs_payload": {
        "type": "line",
        "agg": "mean",
        "baseline": True,
        "x_col": "payload_size",
        "x_scale": "payload",
        "x_label": "Payload Size",
        "y_col": "global_latency",
        "y_scale": "none",
        "y_label": "Latency (ms)",
        "loop_col": "n",
        "loop_scale": "none",
        "loop_fmt": "n={value:g}",
        "title": "Average Latency",
    },
    "cpu_instructions_vs_payload": {
        "type": "line",
        "agg": "raw",
        "baseline": True,
        "x_col": "payload_size",
        "x_scale": "payload",
        "x_label": "Payload Size",
        "y_col": "cpu_instructions",
        "y_scale": "cpu",
        "y_label": "Average Instructions per Node",
        "loop_col": "n",
        "loop_scale": "none",
        "loop_fmt": "n={value:g}",
        "title": "Average CPU Instruction Count",
    },
    "data_sent_vs_payload": {
        "type": "line",
        "agg": "sum",
        "baseline": True,
        "x_col": "payload_size",
        "x_scale": "payload",
        "x_label": "Payload Size",
        "y_col": "bytes_sent",
        "y_scale": "data",
        "y_label": "Total Data Sent",
        "loop_col": "n",
        "loop_scale": "none",
        "loop_fmt": "n={value:g}",
        "title": "Total Data Sent",
    },
    "peak_heap_vs_payload": {
        "type": "line",
        "agg": "mean",
        "baseline": True,
        "x_col": "payload_size",
        "x_scale": "payload",
        "x_label": "Payload Size",
        "y_col": "peak_heap_bytes",
        "y_scale": "data",
        "y_label": "Average Peak Heap Memory per Node",
        "loop_col": "n",
        "loop_scale": "none",
        "loop_fmt": "n={value:g}",
        "title": "Average Peak Heap Memory",
    },
    "cumulative_alloc_vs_payload": {
        "type": "line",
        "agg": "mean",
        "baseline": True,
        "x_col": "payload_size",
        "x_scale": "payload",
        "x_label": "Payload Size",
        "y_col": "cumulative_alloc_bytes",
        "y_scale": "data",
        "y_label": "Average Cumulative Memory Allocation per Node",
        "loop_col": "n",
        "loop_scale": "none",
        "loop_fmt": "n={value:g}",
        "title": "Average Cumulative Memory Allocation",
    },
    "latency_vs_nodes": {
        "type": "line",
        "agg": "mean",
        "baseline": True,
        "x_col": "n",
        "x_scale": "n",
        "x_label": "Network Size",
        "x_ticks": "unique",
        "y_col": "global_latency",
        "y_scale": "none",
        "y_label": "Latency (ms)",
        "loop_col": "payload_size",
        "loop_scale": "payload",
        "loop_fmt": "Payload={value:g} {suffix}",
        "title": "Average Latency",
    },
    "cpu_instructions_vs_nodes": {
        "type": "line",
        "agg": "raw",
        "baseline": True,
        "x_col": "n",
        "x_scale": "n",
        "x_label": "Network Size",
        "x_ticks": "unique",
        "y_col": "cpu_instructions",
        "y_scale": "cpu",
        "y_label": "Average Instructions per Node",
        "loop_col": "payload_size",
        "loop_scale": "payload",
        "loop_fmt": "Payload={value:g} {suffix}",
        "title": "Average CPU Instruction Count",
    },
    "data_sent_vs_nodes": {
        "type": "line",
        "agg": "sum",
        "baseline": True,
        "x_col": "n",
        "x_scale": "n",
        "x_label": "Network Size",
        "x_ticks": "unique",
        "y_col": "bytes_sent",
        "y_scale": "data",
        "y_label": "Total Data Sent",
        "loop_col": "payload_size",
        "loop_scale": "payload",
        "loop_fmt": "Payload={value:g} {suffix}",
        "title": "Total Data Sent",
    },
    "messages_vs_nodes": {
        "type": "line",
        "agg": "sum",
        "baseline": True,
        "x_col": "n",
        "x_scale": "n",
        "x_label": "Network Size",
        "x_ticks": "unique",
        "y_col": "messages_sent",
        "y_scale": "none",
        "y_label": "Total Messages Sent",
        "loop_col": "payload_size",
        "loop_scale": "payload",
        "loop_fmt": "Payload={value:g} {suffix}",
        "title": "Total Messages Sent",
    },
    "peak_heap_vs_nodes": {
        "type": "line",
        "agg": "mean",
        "baseline": True,
        "x_col": "n",
        "x_scale": "n",
        "x_label": "Network Size",
        "x_ticks": "unique",
        "y_col": "peak_heap_bytes",
        "y_scale": "data",
        "y_label": "Average Peak Heap Memory per Node",
        "loop_col": "payload_size",
        "loop_scale": "payload",
        "loop_fmt": "Payload={value:g} {suffix}",
        "title": "Average Peak Heap Memory",
    },
    "cumulative_alloc_vs_nodes": {
        "type": "line",
        "agg": "mean",
        "baseline": True,
        "x_col": "n",
        "x_scale": "n",
        "x_label": "Network Size",
        "x_ticks": "unique",
        "y_col": "cumulative_alloc_bytes",
        "y_scale": "data",
        "y_label": "Average Cumulative Memory Allocation per Node",
        "loop_col": "payload_size",
        "loop_scale": "payload",
        "loop_fmt": "Payload={value:g} {suffix}",
        "title": "Average Cumulative Memory Allocation",
    },
    "delivering_vs_silent": {
        "type": "bft_bar",
        "baseline": False,
        "x_col": "num_silent",
        "x_label": "Silent Nodes",
        "y_col": "delivering_nodes",
        "y_label": "Delivering Nodes",
        "title": "Delivering Nodes",
    },
    "messages_vs_silent": {
        "type": "bft_bar",
        "baseline": False,
        "x_col": "num_silent",
        "x_label": "Silent Nodes",
        "y_col": "messages_sent",
        "y_label": "Average Messages per Node",
        "title": "Average Messages per Node",
    },
}


def get_scale_config(scale_type, args):
    PAYLOAD_SCALES = {
        "b": (1, "bytes"),
        "kb": (1_000, "KB"),
        "mb": (1_000_000, "MB"),
        "gb": (1_000_000_000, "GB"),
    }
    INSTR_SCALES = {
        "none": (1, ""),
        "thousands": (1_000, "Thousands"),
        "millions": (1_000_000, "Millions"),
        "billions": (1_000_000_000, "Billions"),
    }

    if scale_type == "payload":
        return PAYLOAD_SCALES[args.payload_scale]
    if scale_type == "data":
        return PAYLOAD_SCALES[args.data_scale]
    if scale_type == "cpu":
        return INSTR_SCALES[args.instr_scale]
    if scale_type == "n":
        return 1, "n"

    return 1, ""


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("preset", choices=list(PLOT_PRESETS.keys()))
    parser.add_argument("--env", required=True, choices=["gcp", "local", "sim"])

    parser.add_argument(
        "--payload-scale", choices=["b", "kb", "mb", "gb"], default="mb"
    )
    parser.add_argument("--data-scale", choices=["b", "kb", "mb", "gb"], default="mb")
    parser.add_argument(
        "--instr-scale",
        choices=["none", "thousands", "millions", "billions"],
        default="millions",
    )

    return parser.parse_args()


def main():
    args = parse_args()
    config = PLOT_PRESETS[args.preset]

    script_dir = Path(__file__).resolve().parent
    repo_root = script_dir.parent.parent
    results_dir = repo_root / "evaluations" / "results" / args.env
    plots_dir = results_dir / "plots"
    plots_dir.mkdir(parents=True, exist_ok=True)

    results_file = results_dir / "metrics.csv"
    if not results_file.exists():
        print(f"Error: No data found at {results_file}")
        sys.exit(1)

    df = pd.read_csv(results_file)

    df = df[df["sender_role"] == "honest"]
    if config["baseline"]:
        df = df[
            (df["num_silent"] == 0)
            & (df["num_msg_drops"] == 0)
            & (df["role"] == "honest")
        ]

    iteration_col = "sim_seed" if args.env == "sim" else "run"
    df["algorithm"] = df["algorithm"].replace(
        {"bracha": "Bracha", "afrt": "AFRT", "codedmbrb": "Coded MBRB"}
    )

    if config["type"] == "line":
        x_col, y_col, loop_col = config["x_col"], config["y_col"], config["loop_col"]

        if config["agg"] == "raw":
            df_agg = df.copy()
        else:
            df_agg = (
                df.groupby(["algorithm", "n", "payload_size", iteration_col])[y_col]
                .agg(config["agg"])
                .reset_index()
            )

        x_div, x_suf = get_scale_config(config["x_scale"], args)
        y_div, y_suf = get_scale_config(config["y_scale"], args)
        loop_div, loop_suf = get_scale_config(config["loop_scale"], args)

        df_agg["plot_x"] = df_agg[x_col] / x_div
        df_agg["plot_y"] = df_agg[y_col] / y_div

        for loop_val in sorted(df_agg[loop_col].unique()):
            subset = df_agg[df_agg[loop_col] == loop_val]
            if subset[x_col].nunique() < 2:
                continue

            fig = plt.figure(figsize=(8, 5))
            sns.lineplot(
                data=subset,
                x="plot_x",
                y="plot_y",
                hue="algorithm",
                marker="o",
                palette=color_map,
                hue_order=algorithm_order,
                errorbar="sd",
            )

            x_label = f"{config['x_label']} ({x_suf})" if x_suf else config["x_label"]
            y_label = f"{config['y_label']} ({y_suf})" if y_suf else config["y_label"]

            loop_display_val = loop_val / loop_div

            fmt_suffix = (
                args.payload_scale.upper()
                if config["loop_scale"] == "payload"
                else loop_suf
            )
            title_suffix = (
                config["loop_fmt"]
                .format(value=loop_display_val, suffix=fmt_suffix)
                .strip()
            )

            plt.title(f"{config['title']} by {config['x_label']} ({title_suffix})")
            plt.xlabel(x_label)
            plt.ylabel(y_label)
            plt.legend(title=None)

            if config.get("x_ticks") == "unique":
                plt.xticks(sorted(subset["plot_x"].unique()))

            clean_suffix = fmt_suffix.strip()
            out_name = f"{args.env}_{args.preset}_{loop_col}_{loop_display_val:g}{clean_suffix}.pdf"
            fig.savefig(plots_dir / out_name, bbox_inches="tight")
            plt.close(fig)
            print(f"Generated {out_name}")

    elif config["type"] == "bft_bar":
        if args.preset == "delivering_vs_silent":
            df_seed = (
                df[df["delivery_time"].notna()]
                .groupby(
                    ["algorithm", "n", "payload_size", "num_silent", iteration_col]
                )
                .size()
                .reset_index(name="raw_count")
            )
            df_seed["plot_y"] = df_seed.apply(
                lambda r: (
                    r["n"] - r["num_silent"]
                    if r["raw_count"] > 0 and r["num_silent"] < r["n"] / 3
                    else 0
                ),
                axis=1,
            )
            df_agg = (
                df_seed.groupby(["algorithm", "n", "payload_size", "num_silent"])[
                    "plot_y"
                ]
                .mean()
                .reset_index()
            )

        elif args.preset == "messages_vs_silent":
            df_seed = (
                df.groupby(
                    ["algorithm", "n", "payload_size", "num_silent", iteration_col]
                )["messages_sent"]
                .mean()
                .reset_index()
            )
            df_agg = (
                df_seed.groupby(["algorithm", "n", "payload_size", "num_silent"])[
                    "messages_sent"
                ]
                .mean()
                .reset_index()
            )
            df_agg["plot_y"] = df_agg["messages_sent"] * (
                (df_agg["n"] - df_agg["num_silent"]) / df_agg["n"]
            )

        for n in sorted(df_agg["n"].unique()):
            for payload in sorted(df_agg[df_agg["n"] == n]["payload_size"].unique()):
                subset = df_agg[
                    (df_agg["n"] == n) & (df_agg["payload_size"] == payload)
                ]
                if subset["num_silent"].nunique() < 2:
                    continue

                p_div, p_suf = get_scale_config("payload", args)
                payload_str = f"{payload / p_div:g} {p_suf.upper()}"

                fig, ax = plt.subplots(figsize=(10, 6))

                sns.barplot(
                    data=subset,
                    x="num_silent",
                    y="plot_y",
                    hue="algorithm",
                    palette=color_map,
                    hue_order=[
                        a for a in algorithm_order if a in subset["algorithm"].values
                    ],
                    ax=ax,
                    errorbar=None,
                    saturation=1.0,
                )

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

                ax.set_xlabel(config["x_label"], fontsize=11, labelpad=10)
                ax.set_ylabel(config["y_label"], fontsize=11, labelpad=10)
                ax.set_title(
                    f"Silent Node Threshold: {config['title']} (n={n}, Payload={payload_str})",
                    pad=20,
                )

                ax.legend(loc="upper right", frameon=True)
                ax.grid(
                    axis="y", color="lightgray", linestyle="-", linewidth=0.7, alpha=0.7
                )
                ax.grid(axis="x", visible=False)
                ax.spines["top"].set_visible(False)
                ax.spines["right"].set_visible(False)

                out_name = f"{args.env}_{args.preset}_n{n}_p{payload / p_div:g}{p_suf.upper()}.pdf"
                fig.savefig(plots_dir / out_name, bbox_inches="tight")
                plt.close(fig)
                print(f"Generated {out_name}")


if __name__ == "__main__":
    main()
