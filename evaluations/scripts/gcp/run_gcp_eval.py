#!/usr/bin/env python3
import argparse
import subprocess
import itertools
import sys
import time
import json
import urllib.request
from pathlib import Path
from tqdm import tqdm

parser = argparse.ArgumentParser()
parser.add_argument("-r", "--runs", type=int, default=1)
parser.add_argument("-n", "--nodes", nargs="+", type=int, required=True, help="List of node counts (e.g. 5 7 10)")
parser.add_argument("-p", "--payloads", nargs="+", type=str, required=True)
parser.add_argument("-t", "--byzantine", nargs="+", type=int, default=[0])
parser.add_argument("-d", "--dropped", nargs="+", type=int, default=[0])
parser.add_argument("--zone", type=str, default="europe-north1-a")
args = parser.parse_args()

script_dir = Path(__file__).resolve().parent
repo_root = script_dir.parent.parent.parent
bin_dir = repo_root / "bin"

results_dir = repo_root / "evaluations" / "results" / "gcp" / "raw"

algorithms = ["bracha", "afrt", "codedmbrb"]

def to_bytes(s):
    s = s.upper().strip()
    if s.endswith("MB"): return int(float(s[:-2]) * 1_000_000)
    if s.endswith("KB"): return int(float(s[:-2]) * 1_000)
    if s.endswith("B"):  return int(float(s[:-1]))
    raise ValueError(f"Invalid format: {s}")

# Setup directories
for algo in algorithms:
    (results_dir / algo).mkdir(parents=True, exist_ok=True)

# 1. Build Binaries
print("Building binaries locally...")
subprocess.run(["make", "clean"], cwd=repo_root, check=True, stdout=subprocess.DEVNULL)
subprocess.run(["make", "build"], cwd=repo_root, check=True, stdout=subprocess.DEVNULL)

# 2. GCP Provisioning
max_nodes = max(args.nodes)
print(f"Provisioning 1 Controller and {max_nodes} Nodes on GCP in {args.zone}...")

# Firewall
subprocess.run(["gcloud", "compute", "firewall-rules", "create", "brb-allow-internal", "--allow=tcp,udp", "--source-ranges=10.0.0.0/8", "--quiet"], stderr=subprocess.DEVNULL)
subprocess.run(["gcloud", "compute", "firewall-rules", "create", "brb-allow-controller", "--allow=tcp:8080", "--source-ranges=0.0.0.0/0", "--target-tags=brb-controller", "--quiet"], stderr=subprocess.DEVNULL)
subprocess.run(["gcloud", "compute", "firewall-rules", "create", "allow-ssh-ingress-from-iap", "--allow=tcp:22", "--source-ranges=35.235.240.0/20", "--quiet"], stderr=subprocess.DEVNULL)

# Controller VM
subprocess.run([
    "gcloud", "compute", "instances", "create", "brb-controller",
    f"--zone={args.zone}", "--machine-type=e2-small", "--image-family=debian-12",
    "--image-project=debian-cloud", "--tags=brb-controller", 
    "--metadata=startup-script='#!/bin/bash\napt-get update\napt-get install -y curl'",  # Install curl for metrics download
    "--quiet"
], check=True)

# Node VMs
node_creation_cmd = " & ".join([
    f"gcloud compute instances create brb-node-{i} --zone={args.zone} --machine-type=e2-small --image-family=debian-12 --image-project=debian-cloud --no-address --quiet"
    for i in range(max_nodes)
]) + " & wait"
subprocess.run(node_creation_cmd, shell=True, check=True)

print("Waiting for VMs and SSH to stabilize (45s)...")
time.sleep(45)

# Push binaries
print("Pushing binaries to VMs...")
subprocess.run(["gcloud", "compute", "scp", str(bin_dir / "controller"), f"brb-controller:~/", f"--zone={args.zone}", "--tunnel-through-iap", "--quiet"], check=True)
scp_cmd = " & ".join([f"gcloud compute scp {bin_dir}/node brb-node-{i}:~/ --zone={args.zone} --tunnel-through-iap --quiet" for i in range(max_nodes)]) + " & wait"
subprocess.run(scp_cmd, shell=True, check=True)

# Get IPs
ctrl_ip_res = subprocess.run(["gcloud", "compute", "instances", "describe", "brb-controller", f"--zone={args.zone}", "--format=get(networkInterfaces[0].networkIP)"], capture_output=True, text=True)
controller_internal_ip = ctrl_ip_res.stdout.strip()

ctrl_pub_res = subprocess.run(["gcloud", "compute", "instances", "describe", "brb-controller", f"--zone={args.zone}", "--format=get(networkInterfaces[0].accessConfigs[0].natIP)"], capture_output=True, text=True)
controller_public_ip = ctrl_pub_res.stdout.strip()

node_ips = []
for i in range(max_nodes):
    res = subprocess.run(["gcloud", "compute", "instances", "describe", f"brb-node-{i}", f"--zone={args.zone}", "--format=get(networkInterfaces[0].networkIP)"], capture_output=True, text=True)
    node_ips.append(res.stdout.strip())

# 3. Test Execution Loop
configs = list(itertools.product(range(1, args.runs + 1), algorithms, args.payloads, args.nodes, args.byzantine, args.dropped))

try:
    for config in tqdm(configs, desc="Running Configurations"):
        run_idx, algorithm, p_size, n, t, d = config
        p_bytes = to_bytes(p_size)
        
        tqdm.write(f"--- Running {algorithm.upper()} | N={n} | Payload={p_size} | t={t} | d={d} | Run {run_idx}/{args.runs} ---")

        # Cleanup old processes
        subprocess.run([f"gcloud compute ssh brb-controller --zone={args.zone} --tunnel-through-iap --command='pkill -f controller || true' --quiet"], shell=True, stderr=subprocess.DEVNULL)
        cleanup_nodes = " & ".join([f"gcloud compute ssh brb-node-{i} --zone={args.zone} --tunnel-through-iap --command='pkill -f node || true' --quiet" for i in range(n)]) + " & wait"
        subprocess.run(cleanup_nodes, shell=True, stderr=subprocess.DEVNULL)

        # Start Controller
        subprocess.run([
            "gcloud", "compute", "ssh", "brb-controller", f"--zone={args.zone}", "--tunnel-through-iap", "--quiet",
            "--command", f"nohup ~/controller -addr=0.0.0.0:8080 -n={n} -algorithm={algorithm} -payloadSize={p_size} -t={t} -d={d} -keepControllerAlive=true > ~/controller.log 2>&1 &"
        ])
        time.sleep(3) # Give controller time to bind port

        # Start Nodes
        start_nodes_cmd = " & ".join([
            f"gcloud compute ssh brb-node-{i} --zone={args.zone} --tunnel-through-iap --quiet --command='nohup ~/node -id={i} -addr={node_ips[i]}:{9090+i} -controller={controller_internal_ip}:8080 > ~/node.log 2>&1 &'"
            for i in range(n)
        ]) + " & wait"
        subprocess.run(start_nodes_cmd, shell=True)

        # Poll Controller for completion
        tqdm.write("Waiting for test to complete...")
        status_url = f"http://{controller_public_ip}:8080/status"
        done = False
        attempts = 0
        while not done and attempts < 12: # 1 min timeout (5s * 12)
            time.sleep(5)
            attempts += 1
            try:
                req = urllib.request.Request(status_url)
                with urllib.request.urlopen(req, timeout=5) as response:
                    status = json.loads(response.read().decode())
                    if status.get("allMetricsReceived", False):
                        done = True
            except Exception:
                pass

        if not done:
            tqdm.write("ERROR: Test timed out!")
            continue

        # Download Metrics
        gcp_save_path = results_dir / algorithm / f"metrics_{algorithm}_{n}nodes_{p_bytes}bytes_t{t}_d{d}_run{run_idx}.json"
        try:
            urllib.request.urlretrieve(f"http://{controller_public_ip}:8080/download/metrics", gcp_save_path)
            
            with open(gcp_save_path, "r") as f:
                data = json.load(f)

            data["env"] = "gcp"
            data["run"] = run_idx

            with open(gcp_save_path, "w") as f:
                json.dump(data, f, indent=2)

        except Exception as e:
            tqdm.write(f"Failed to download metrics: {e}")

finally:
    # Delete all VMs since we are not rich :)
    print("\nDeleting VMs and cleaning up...")
    subprocess.run([f"gcloud compute instances delete brb-controller --zone={args.zone} --quiet"], shell=True)
    delete_nodes = " & ".join([f"gcloud compute instances delete brb-node-{i} --zone={args.zone} --quiet" for i in range(max_nodes)]) + " & wait"
    subprocess.run(delete_nodes, shell=True)
    print("Done! Check your evaluations/results/gcp/raw folder.")