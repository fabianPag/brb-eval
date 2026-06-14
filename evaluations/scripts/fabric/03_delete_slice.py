#!/usr/bin/env python3
import argparse
import sys
from fabrictestbed_extensions.fablib.fablib import FablibManager


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--slice", type=str, required=True, help="Name of the FABRIC slice to delete"
    )
    args = parser.parse_args()

    fablib = FablibManager()

    try:
        fablib.get_slice(name=args.slice).delete()
        print("Sent slice deletion request.")
    except Exception as e:
        print(f"Error deleting slice {args.slice}: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
