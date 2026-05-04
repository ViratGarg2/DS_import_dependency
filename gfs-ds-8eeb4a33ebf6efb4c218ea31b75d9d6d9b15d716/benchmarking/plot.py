#!/usr/bin/env python3
"""
GFS Benchmark Plotter
Reads JSON results produced by benchmark.go and generates matplotlib figures.

Usage:
    python3 plot.py [--results results/] [--out plots/]
"""

import argparse
import json
import math
import os
import sys

try:
    import matplotlib
    matplotlib.use("Agg")
    import matplotlib.pyplot as plt
    import matplotlib.ticker as ticker
except ImportError:
    sys.exit("matplotlib not found — install with: pip install matplotlib")

# ── helpers ───────────────────────────────────────────────────────────────────

def load(results_dir, filename):
    path = os.path.join(results_dir, filename)
    if not os.path.exists(path):
        print(f"  [skip] {filename} not found")
        return None
    with open(path) as f:
        return json.load(f)

def save(out_dir, name):
    os.makedirs(out_dir, exist_ok=True)
    path = os.path.join(out_dir, name)
    plt.savefig(path, dpi=150, bbox_inches="tight")
    print(f"  → {path}")
    plt.close()

COLORS = ["#2563eb", "#16a34a", "#dc2626", "#d97706", "#7c3aed", "#0891b2"]

# ── Plot 1: Aggregate Read Throughput ─────────────────────────────────────────

def plot_read_throughput(data, out_dir):
    if not data:
        return
    fig, ax = plt.subplots(figsize=(7, 4))
    x = [d["concurrency"] for d in data]
    y = [d["throughput_mbps"] for d in data]
    ax.plot(x, y, marker="o", color=COLORS[0], linewidth=2, markersize=7, label="Observed")
    # Ideal linear scaling line
    ideal = [y[0] * (c / x[0]) for c in x]
    ax.plot(x, ideal, linestyle="--", color="gray", linewidth=1.2, label="Linear ideal")
    ax.set_title("Aggregate Read Throughput vs Concurrency\n(GFS §6.2)", fontsize=12)
    ax.set_xlabel("Number of concurrent clients")
    ax.set_ylabel("Aggregate throughput (MB/s)")
    ax.set_xticks(x)
    ax.legend()
    ax.grid(True, alpha=0.3)
    save(out_dir, "1_read_throughput.png")

# ── Plot 2: Aggregate Write Throughput ────────────────────────────────────────

def plot_write_throughput(data, out_dir):
    if not data:
        return
    fig, ax = plt.subplots(figsize=(7, 4))
    x = [d["concurrency"] for d in data]
    y = [d["throughput_mbps"] for d in data]
    ax.plot(x, y, marker="s", color=COLORS[1], linewidth=2, markersize=7, label="Observed")
    ideal = [y[0] * (c / x[0]) for c in x]
    ax.plot(x, ideal, linestyle="--", color="gray", linewidth=1.2, label="Linear ideal")
    ax.set_title("Aggregate Write Throughput vs Concurrency\n(GFS §6.2)", fontsize=12)
    ax.set_xlabel("Number of concurrent clients")
    ax.set_ylabel("Aggregate throughput (MB/s)")
    ax.set_xticks(x)
    ax.legend()
    ax.grid(True, alpha=0.3)
    save(out_dir, "2_write_throughput.png")

# ── Plot: Read & Write Scalability (side-by-side) ────────────────────────────

def plot_scalability(read_data, write_data, out_dir):
    fig, axes = plt.subplots(1, 2, figsize=(13, 5))

    if read_data:
        ax = axes[0]
        x = [d["concurrency"]     for d in read_data]
        y = [d["throughput_mbps"] for d in read_data]
        ax.plot(x, y, marker="o", color="blue", linewidth=2, markersize=7,
                label="Aggregate Throughput")
        ax.set_title("Read Scalability with Concurrent Clients", fontsize=11)
        ax.set_xlabel("Number of Concurrent Clients")
        ax.set_ylabel("Aggregate Throughput (MB/s)")
        ax.set_xticks(x)
        ax.legend(loc="upper left")
        ax.grid(True, alpha=0.3)

    if write_data:
        ax = axes[1]
        x = [d["concurrency"]     for d in write_data]
        y = [d["throughput_mbps"] for d in write_data]
        ax.plot(x, y, marker="s", color="red", linewidth=2, markersize=7,
                label="Aggregate Throughput")
        ax.set_title("Write Scalability with Concurrent Clients", fontsize=11)
        ax.set_xlabel("Number of Concurrent Clients")
        ax.set_ylabel("Aggregate Throughput (MB/s)")
        ax.set_xticks(x)
        ax.legend(loc="upper left")
        ax.grid(True, alpha=0.3)

    plt.tight_layout()
    save(out_dir, "0_scalability.png")

# ── Plot 3a: Append Throughput vs Concurrency ─────────────────────────────────

def plot_append_throughput(data, out_dir):
    if not data:
        return
    fig, ax = plt.subplots(figsize=(7, 4))
    x = [d["concurrency"] for d in data]
    y = [d["ops_per_sec"] for d in data]
    ax.bar(x, y, color=COLORS[2], width=0.5, edgecolor="white")
    ax.set_title("Record Append Throughput vs Concurrency\n(GFS §6.3)", fontsize=12)
    ax.set_xlabel("Number of concurrent clients")
    ax.set_ylabel("Appends per second")
    ax.set_xticks(x)
    ax.grid(True, axis="y", alpha=0.3)
    save(out_dir, "3a_append_throughput.png")

# ── Plot 3b: Append Latency Percentiles ───────────────────────────────────────

def plot_append_latency(data, out_dir):
    if not data:
        return
    fig, ax = plt.subplots(figsize=(7, 4))
    x = [d["concurrency"] for d in data]
    avg = [d["avg_latency_ms"] for d in data]
    p50 = [d["p50_latency_ms"] for d in data]
    p99 = [d["p99_latency_ms"] for d in data]

    width = 0.28
    xs = list(range(len(x)))
    ax.bar([v - width for v in xs], avg, width=width, label="avg", color=COLORS[3])
    ax.bar(xs, p50, width=width, label="p50", color=COLORS[0])
    ax.bar([v + width for v in xs], p99, width=width, label="p99", color=COLORS[2])
    ax.set_title("Record Append Latency Percentiles\n(GFS §6.3)", fontsize=12)
    ax.set_xlabel("Number of concurrent clients")
    ax.set_ylabel("Latency (ms)")
    ax.set_xticks(xs)
    ax.set_xticklabels(x)
    ax.legend()
    ax.grid(True, axis="y", alpha=0.3)
    save(out_dir, "3b_append_latency.png")

# ── Plot 4a: File Size vs Latency ─────────────────────────────────────────────

def plot_filesize_latency(data, out_dir):
    if not data:
        return
    fig, ax = plt.subplots(figsize=(8, 4))
    sizes_kb = [d["file_size_bytes"] / 1024 for d in data]
    wlat = [d["write_latency_ms"] for d in data]
    rlat = [d["read_latency_ms"] for d in data]

    ax.plot(sizes_kb, wlat, marker="o", color=COLORS[1], linewidth=2, markersize=7, label="Write")
    ax.plot(sizes_kb, rlat, marker="^", color=COLORS[0], linewidth=2, markersize=7, label="Read")
    ax.set_xscale("log", base=2)
    ax.set_yscale("log")
    ax.set_title("File Size Impact on Latency\n(GFS §6.1 — fixed master overhead vs bulk transfer)", fontsize=12)
    ax.set_xlabel("File size (KB)")
    ax.set_ylabel("Latency (ms, log scale)")
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda v, _: f"{v:.0f}"))
    ax.legend()
    ax.grid(True, alpha=0.3, which="both")
    save(out_dir, "4a_filesize_latency.png")

# ── Plot 4b: File Size vs Throughput ─────────────────────────────────────────

def plot_filesize_throughput(data, out_dir):
    if not data:
        return
    fig, ax = plt.subplots(figsize=(8, 4))
    sizes_kb = [d["file_size_bytes"] / 1024 for d in data]
    w_mbps = [d["write_mbps"] for d in data]
    r_mbps = [d["read_mbps"] for d in data]

    ax.plot(sizes_kb, w_mbps, marker="o", color=COLORS[1], linewidth=2, markersize=7, label="Write")
    ax.plot(sizes_kb, r_mbps, marker="^", color=COLORS[0], linewidth=2, markersize=7, label="Read")
    ax.set_xscale("log", base=2)
    ax.set_title("Effective Throughput vs File Size\n(amortisation of master overhead)", fontsize=12)
    ax.set_xlabel("File size (KB)")
    ax.set_ylabel("Throughput (MB/s)")
    ax.xaxis.set_major_formatter(ticker.FuncFormatter(lambda v, _: f"{v:.0f}"))
    ax.legend()
    ax.grid(True, alpha=0.3)
    save(out_dir, "4b_filesize_throughput.png")

# ── Plot 5: Chunk Boundary Overhead ───────────────────────────────────────────

def plot_boundary(data, out_dir):
    if not data:
        return
    fig, axes = plt.subplots(1, 2, figsize=(11, 4))
    labels = [d["access_type"].replace("_", "\n") for d in data]
    wlat = [d["write_latency_ms"] for d in data]
    rlat = [d["read_latency_ms"] for d in data]
    xs = list(range(len(labels)))

    for ax, vals, title, color in [
        (axes[0], wlat, "Write Latency", COLORS[1]),
        (axes[1], rlat, "Read Latency",  COLORS[0]),
    ]:
        bars = ax.bar(xs, vals, color=color, edgecolor="white", width=0.55)
        ax.set_title(f"{title} — Within vs Cross-Boundary\n(GFS §6.1 chunk boundary overhead)", fontsize=10)
        ax.set_ylabel("Latency (ms)")
        ax.set_xticks(xs)
        ax.set_xticklabels(labels, fontsize=8)
        ax.grid(True, axis="y", alpha=0.3)
        for bar, v in zip(bars, vals):
            ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + 0.05,
                    f"{v:.1f}", ha="center", va="bottom", fontsize=8)

    fig.suptitle("Chunk Boundary Overhead", fontsize=12)
    plt.tight_layout()
    save(out_dir, "5_chunk_boundary.png")

# ── Plot 6a: Master Ops/sec ───────────────────────────────────────────────────

def plot_master_ops(data, out_dir):
    if not data:
        return
    fig, ax = plt.subplots(figsize=(7, 4))
    ops   = [d["operation"] for d in data]
    tput  = [d["ops_per_sec"] for d in data]
    xs = list(range(len(ops)))
    bars = ax.bar(xs, tput, color=COLORS[4], edgecolor="white", width=0.5)
    ax.set_title("Master Operation Throughput\n(GFS §6.4 — metadata bottleneck)", fontsize=12)
    ax.set_ylabel("Operations / second")
    ax.set_xticks(xs)
    ax.set_xticklabels(ops, rotation=15, ha="right")
    ax.grid(True, axis="y", alpha=0.3)
    for bar, v in zip(bars, tput):
        ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + 0.5,
                f"{v:.0f}", ha="center", va="bottom", fontsize=9)
    save(out_dir, "6a_master_ops.png")

# ── Plot 6b: Master latency percentiles ──────────────────────────────────────

def plot_master_latency(data, out_dir):
    if not data:
        return
    fig, ax = plt.subplots(figsize=(7, 4))
    ops   = [d["operation"] for d in data]
    avg   = [d["avg_latency_ms"] for d in data]
    p99   = [d["p99_latency_ms"] for d in data]
    xs = list(range(len(ops)))
    width = 0.35
    ax.bar([v - width / 2 for v in xs], avg, width=width, label="avg", color=COLORS[0])
    ax.bar([v + width / 2 for v in xs], p99, width=width, label="p99", color=COLORS[2])
    ax.set_title("Master Operation Latency\n(GFS §6.4)", fontsize=12)
    ax.set_ylabel("Latency (ms)")
    ax.set_xticks(xs)
    ax.set_xticklabels(ops, rotation=15, ha="right")
    ax.legend()
    ax.grid(True, axis="y", alpha=0.3)
    save(out_dir, "6b_master_latency.png")

# ── Plot 7: GFS vs Local Filesystem Baseline ─────────────────────────────────

def plot_comparison(data, out_dir):
    if not data:
        return
    ops          = [d["operation"]        for d in data]
    base_mbps    = [d["baseline_mbps"]    for d in data]
    gfs_mbps     = [d["gfs_mbps"]         for d in data]
    base_ms      = [d["baseline_lat_ms"]  for d in data]
    gfs_ms       = [d["gfs_lat_ms"]       for d in data]
    tput_pct     = [d["throughput_pct"]   for d in data]
    lat_overhead = [d["latency_overhead"] for d in data]

    xs    = list(range(len(ops)))

    fig, axes = plt.subplots(2, 2, figsize=(14, 10))

    # ── Top-left: GFS Throughput only ────────────────────────────────────
    ax = axes[0, 0]
    bars = ax.bar(xs, gfs_mbps, color="#f4a582", edgecolor="white", width=0.5)
    ax.set_title("GFS Throughput", fontsize=11)
    ax.set_ylabel("Throughput (MB/s)")
    ax.set_xticks(xs); ax.set_xticklabels(ops, rotation=15, ha="right", fontsize=9)
    ax.grid(True, axis="y", alpha=0.3)
    for bar, v in zip(bars, gfs_mbps):
        if v > 0:
            ax.text(bar.get_x() + bar.get_width()/2, bar.get_height() * 1.02,
                    f"{v:.1f}", ha="center", va="bottom", fontsize=8)

    # ── Top-right: GFS Latency only ───────────────────────────────────────
    ax = axes[0, 1]
    bars = ax.bar(xs, gfs_ms, color="#f4a582", edgecolor="white", width=0.5)
    ax.set_title("GFS Latency", fontsize=11)
    ax.set_ylabel("Latency (ms)")
    ax.set_xticks(xs); ax.set_xticklabels(ops, rotation=15, ha="right", fontsize=9)
    ax.grid(True, axis="y", alpha=0.3)
    for bar, v in zip(bars, gfs_ms):
        if v > 0:
            ax.text(bar.get_x() + bar.get_width()/2, bar.get_height() * 1.02,
                    f"{v:.1f}ms", ha="center", va="bottom", fontsize=8)

    # ── Bottom-left: GFS throughput as % of baseline ──────────────────────
    ax = axes[1, 0]
    bars = ax.bar(xs, tput_pct, color="#f4a582", edgecolor="white", width=0.5)
    ax.axhline(100, linestyle="--", color="gray", linewidth=1.2, label="Baseline (100%)")
    ax.set_title("GFS Throughput as % of Baseline", fontsize=11)
    ax.set_ylabel("Percentage (%)")
    ax.set_xticks(xs); ax.set_xticklabels(ops, rotation=15, ha="right", fontsize=9)
    ax.legend(); ax.grid(True, axis="y", alpha=0.3)
    for bar, v in zip(bars, tput_pct):
        if v > 0:
            ax.text(bar.get_x() + bar.get_width()/2, bar.get_height() + 0.3,
                    f"{v:.1f}%", ha="center", va="bottom", fontsize=8)

    # ── Bottom-right: Latency overhead ratio ──────────────────────────────
    ax = axes[1, 1]
    bars = ax.bar(xs, lat_overhead, color="red", edgecolor="white", width=0.5)
    ax.axhline(1, linestyle="--", color="gray", linewidth=1.2, label="No overhead (1×)")
    ax.set_title("GFS Latency Overhead vs Baseline", fontsize=11)
    ax.set_ylabel("Overhead (× local latency)")
    ax.set_xticks(xs); ax.set_xticklabels(ops, rotation=15, ha="right", fontsize=9)
    ax.legend(); ax.grid(True, axis="y", alpha=0.3)
    for bar, v in zip(bars, lat_overhead):
        if v > 0:
            label = f"{v:.0f}×" if v >= 10 else f"{v:.1f}×"
            ax.text(bar.get_x() + bar.get_width()/2, bar.get_height() * 1.02,
                    label, ha="center", va="bottom", fontsize=8)

    fig.suptitle("GFS vs Local Filesystem Baseline\n(Sequential Write, Sequential Read, Random 4 KB I/O)",
                 fontsize=13)
    plt.tight_layout()
    save(out_dir, "7_gfs_vs_baseline.png")

# ── Plot 8: Sustained Throughput ─────────────────────────────────────────────

def plot_sustained_throughput(data, out_dir):
    if not data:
        return
    writes = [d for d in data if d["operation"] == "write"]
    reads  = [d for d in data if d["operation"] == "read"]

    fig, axes = plt.subplots(1, 2, figsize=(13, 5))

    for ax, samples, label, color in [
        (axes[0], writes, "Sustained Write Throughput", COLORS[1]),
        (axes[1], reads,  "Sustained Read Throughput",  COLORS[0]),
    ]:
        if not samples:
            ax.set_visible(False)
            continue
        t   = [d["elapsed_sec"]      for d in samples]
        bw  = [d["throughput_mbps"]  for d in samples]
        cum = [d["total_mb_transferred"] for d in samples]
        ax.plot(t, bw, marker="o", color=color, linewidth=2, markersize=7, label="MB/s per interval")
        ax2 = ax.twinx()
        ax2.plot(t, cum, marker="x", color="gray", linewidth=1.2, linestyle="--", label="Cumulative MB")
        ax2.set_ylabel("Cumulative MB transferred", color="gray")
        ax2.tick_params(axis="y", labelcolor="gray")
        ax.set_title(f"{label}\n(30 s streaming, sampled every 5 s)", fontsize=11)
        ax.set_xlabel("Elapsed time (s)")
        ax.set_ylabel("Throughput (MB/s)", color=color)
        ax.tick_params(axis="y", labelcolor=color)
        ax.grid(True, alpha=0.3)
        lines1, labels1 = ax.get_legend_handles_labels()
        lines2, labels2 = ax2.get_legend_handles_labels()
        ax.legend(lines1 + lines2, labels1 + labels2, fontsize=8)

    fig.suptitle("Sustained Throughput Over Time", fontsize=13)
    plt.tight_layout()
    save(out_dir, "8_sustained_throughput.png")

# ── Plot 9: Mixed Read/Write Workload ─────────────────────────────────────────

def plot_mixed_workload(data, out_dir):
    if not data:
        return
    labels = [f"R{d['readers']}/W{d['writers']}" for d in data]
    read_bw  = [d["read_mbps"]      for d in data]
    write_bw = [d["write_mbps"]     for d in data]
    agg_bw   = [d["aggregate_mbps"] for d in data]
    xs = list(range(len(labels)))

    fig, axes = plt.subplots(1, 2, figsize=(13, 5))

    # Left: stacked bar — read vs write contribution
    ax = axes[0]
    ax.bar(xs, read_bw,  color=COLORS[0], label="Read MB/s",  edgecolor="white", width=0.5)
    ax.bar(xs, write_bw, color=COLORS[2], label="Write MB/s", edgecolor="white", width=0.5, bottom=read_bw)
    ax.set_title("Read vs Write Throughput by Concurrency Mix", fontsize=11)
    ax.set_xlabel("(Readers / Writers)")
    ax.set_ylabel("Throughput (MB/s)")
    ax.set_xticks(xs)
    ax.set_xticklabels(labels)
    ax.legend()
    ax.grid(True, axis="y", alpha=0.3)

    # Right: aggregate throughput line
    ax = axes[1]
    ax.plot(xs, agg_bw, marker="D", color=COLORS[3], linewidth=2, markersize=8, label="Aggregate MB/s")
    ax.set_title("Aggregate Throughput Under Mixed Workload", fontsize=11)
    ax.set_xlabel("(Readers / Writers)")
    ax.set_ylabel("Aggregate throughput (MB/s)")
    ax.set_xticks(xs)
    ax.set_xticklabels(labels)
    ax.legend()
    ax.grid(True, alpha=0.3)
    for xi, v in zip(xs, agg_bw):
        ax.text(xi, v + max(agg_bw) * 0.02, f"{v:.1f}", ha="center", fontsize=8)

    fig.suptitle("Mixed Read/Write Workload\n(simultaneous clients on distinct files)", fontsize=13)
    plt.tight_layout()
    save(out_dir, "9_mixed_workload.png")

# ── main ──────────────────────────────────────────────────────────────────────

def main():
    p = argparse.ArgumentParser(description="Plot GFS benchmark results")
    # Default looks for results/ next to this script (benchmarking/results/).
    # When run from the project root use: --results benchmarking/results
    default_results = os.path.join(os.path.dirname(os.path.abspath(__file__)), "results")
    default_out     = os.path.join(os.path.dirname(os.path.abspath(__file__)), "plots")
    p.add_argument("--results", default=default_results, help="directory with JSON files")
    p.add_argument("--out", default=default_out, help="output directory for PNGs")
    args = p.parse_args()

    rd = args.results
    od = args.out

    print(f"Reading results from: {rd}/")
    print(f"Writing plots to:     {od}/\n")

    d_read  = load(rd, "read_throughput.json")
    d_write = load(rd, "write_throughput.json")
    plot_scalability(d_read, d_write, od)
    plot_read_throughput(d_read, od)
    plot_write_throughput(d_write, od)
    d_append = load(rd, "append_concurrency.json")
    plot_append_throughput(d_append, od)
    plot_append_latency(d_append, od)
    d_fs = load(rd, "filesize_latency.json")
    plot_filesize_latency(d_fs, od)
    plot_filesize_throughput(d_fs, od)
    plot_boundary(load(rd, "chunk_boundary.json"), od)
    d_master = load(rd, "master_latency.json")
    plot_master_ops(d_master, od)
    plot_master_latency(d_master, od)
    plot_comparison(load(rd, "comparison.json"), od)
    plot_sustained_throughput(load(rd, "sustained_throughput.json"), od)
    plot_mixed_workload(load(rd, "mixed_workload.json"), od)

    print("\nDone.")

if __name__ == "__main__":
    main()
