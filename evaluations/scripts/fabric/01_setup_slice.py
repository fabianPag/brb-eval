#!/usr/bin/env python3
import argparse
import sys
import time
import random
from pathlib import Path
import concurrent.futures
from fabrictestbed_extensions.fablib.fablib import FablibManager
from tqdm import tqdm


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--slice", type=str, required=True, help="Name of the FABRIC slice to create"
    )
    parser.add_argument(
        "--nodes",
        type=int,
        default=30,
        help="Number of nodes to create in the slice (excluding controller)",
    )
    parser.add_argument(
        "--sites", nargs="+", type=str, default=["TACC", "UTAH", "NCSA", "MAX", "MICH"]
    )
    args = parser.parse_args()

    fablib = FablibManager()
    sites = args.sites

    script_dir = Path(__file__).resolve().parent
    repo_root = script_dir.parent.parent.parent
    bin_dir = repo_root / "bin"

    if not (bin_dir / "controller").exists() or not (bin_dir / "node").exists():
        print(f"Error: Could not find 'controller' or 'node' binaries in {bin_dir}")
        sys.exit(1)

    try:
        slice = fablib.get_slice(name=args.slice)
        print(f"Slice '{args.slice}' already exists!")
        sys.exit(0)
    except Exception:
        print(
            f"Creating new slice '{args.slice}' with {args.nodes + 1} VMs ({args.nodes} nodes + controller)..."
        )

    try:
        slice = fablib.new_slice(name=args.slice)
        site_interfaces = {site: [] for site in sites}

        ctrl_node = slice.add_node(
            name="Controller", site=sites[0], image="default_ubuntu_24"
        )
        site_interfaces[sites[0]].append(
            ctrl_node.add_component(model="NIC_Basic", name="net1").get_interfaces()[0]
        )

        for i in range(args.nodes):
            site_name = sites[(i + 1) % len(sites)]
            node = slice.add_node(
                name=f"Node{i}", site=site_name, image="default_ubuntu_24"
            )
            site_interfaces[site_name].append(
                node.add_component(model="NIC_Basic", name="net1").get_interfaces()[0]
            )

        for site, ifaces in site_interfaces.items():
            if len(ifaces) > 0:
                slice.add_l3network(name=f"net_{site}", interfaces=ifaces, type="IPv4")

        print(f"Submitting slice request for '{args.slice}'...")
        slice.submit(wait=False)

        print(
            "\nWaiting for slice to become active... (This can take a long time, leave terminal open)"
        )
        slice.wait(timeout=1800, progress=True)

        print("\nWaiting for SSH access to all nodes...")
        slice.wait_ssh(timeout=1800, progress=True)

        print("\nRunning FABRIC post-boot scripts...")
        slice.post_boot_config()

    except Exception as e:
        print(f"Failed to setup slice: {e}")
        sys.exit(1)

    print("Retrieving networks...")
    networks = {}
    for site in sites:
        try:
            net = slice.get_network(name=f"net_{site}")
            networks[site] = {
                "subnet": net.get_subnet(),
                "gateway": net.get_gateway(),
                "available_ips": net.get_available_ips(),
            }
        except Exception as e:
            print(f"Network for site '{site}' could not be retrieved: {e}")
            sys.exit(1)

    nodes = slice.get_nodes()

    node_assignments = []
    for node in nodes:
        site = node.get_site()
        node_ip = networks[site]["available_ips"].pop(0)
        node_assignments.append((node, site, node_ip))

    def configure_single_node(node_args):
        time.sleep(random.uniform(0.1, 1.0))

        node, site, node_ip = node_args
        iface = node.get_interfaces()[0]

        iface.ip_addr_add(addr=node_ip, subnet=networks[site]["subnet"])
        for other_site, other_net_info in networks.items():
            if other_site != site:
                node.ip_route_add(
                    subnet=other_net_info["subnet"], gateway=networks[site]["gateway"]
                )

        binary_name = "controller" if node.get_name() == "Controller" else "node"
        local_binary_path = str(bin_dir / binary_name)

        node.upload_file(local_binary_path, binary_name)
        node.execute(f"chmod +x ./{binary_name}", quiet=True)

        return node.get_name()

    max_concurrent_workers = min(len(nodes), 10)
    with concurrent.futures.ThreadPoolExecutor(
        max_workers=max_concurrent_workers
    ) as executor:
        future_to_node = {
            executor.submit(configure_single_node, node_arg): node_arg[0].get_name()
            for node_arg in node_assignments
        }

        progress_bar = tqdm(
            concurrent.futures.as_completed(future_to_node),
            total=len(nodes),
            desc="Configuring Nodes",
        )

        for future in progress_bar:
            node_name = future_to_node[future]
            try:
                future.result()
                tqdm.write(f"{node_name} setup complete.")
            except Exception as exc:
                tqdm.write(f"{node_name} setup failed with error: {exc}")

    print("\nSlice ready.")


if __name__ == "__main__":
    main()
