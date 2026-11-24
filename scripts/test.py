#!/usr/bin/env python3
"""Go test orchestrator with per-package colored output.

Runs `go test` package-by-package (never `go test ./...` as one blob so
per-package output stays visible), then renders a TUI-style report: one
section per package with PASS/FAIL/NO-TESTS status in color, test counts,
and the package's raw output below the header.

Usage:
    python3 scripts/test.py [--verbose] [--pkg SUBSTRING]

Exit codes:
    0   every package passed
    1   one or more packages failed
    2   `go` binary not found on PATH
    130 interrupted (Ctrl-C)

Stdlib only — raw ANSI escapes, no third-party imports.
"""

import argparse
import os
import shutil
import subprocess
import sys
import time

# --- ANSI colors -----------------------------------------------------------
RESET = "\033[0m"
BOLD = "\033[1m"
DIM = "\033[2m"
GREEN = "\033[32m"
RED = "\033[31m"
YELLOW = "\033[33m"
CYAN = "\033[36m"


def color(text, *codes):
    return "".join(codes) + text + RESET


def repo_root():
    # scripts/ is one level below the repo root; resolve from the script
    # location, not the CWD.
    return os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def discover_packages(root, pkg_filter):
    proc = subprocess.run(
        ["go", "list", "./..."],
        cwd=root,
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        sys.stderr.write(color("error: `go list ./...` failed:\n", RED, BOLD))
        sys.stderr.write(proc.stderr)
        sys.exit(1)
    pkgs = [line.strip() for line in proc.stdout.splitlines() if line.strip()]
    if pkg_filter:
        pkgs = [p for p in pkgs if pkg_filter in p]
    return pkgs


def run_package(root: str, pkg: str) -> tuple[str, tuple[int, int], str, float]:
    """Run `go test` for one package.

    Returns (status, counts, output, elapsed_seconds).
    """
    started = time.monotonic()
    try:
        proc = subprocess.run(
            ["go", "test", "-count=1", pkg],
            cwd=root,
            capture_output=True,
            text=True,
        )
        output = proc.stdout + proc.stderr
    except subprocess.TimeoutExpired:
        return "FAIL", (0, 0), "go test timed out\n", time.monotonic() - started
    elapsed = time.monotonic() - started
    if "no test files" in output:
        return "SKIP", (0, 0), output, elapsed
    # Parse "ok  pkg  0.074s" or leading "FAIL" per go test conventions.
    if proc.returncode == 0 and output.lstrip().startswith("ok"):
        return "PASS", parse_counts(output), output, elapsed
    return "FAIL", parse_counts(output), output, elapsed


def parse_counts(output):
    """Extract (passed, failed) test counts from go test -v-style output.

    Non-verbose `go test` does not print per-test lines, so counts are
    best-effort: (0, 0) when nothing parseable is present.
    """
    passed = failed = 0
    for line in output.splitlines():
        line = line.strip()
        if line.startswith("--- PASS:"):
            passed += 1
        elif line.startswith("--- FAIL:"):
            failed += 1
    return passed, failed


STATUS_STYLE = {
    "PASS": ("PASS", (GREEN, BOLD)),
    "FAIL": ("FAIL", (RED, BOLD)),
    "SKIP": ("NO TESTS", (YELLOW, BOLD)),
}


def render(pkg, status, counts, output, elapsed, verbose):
    label, codes = STATUS_STYLE[status]
    header = " ".join(
        [
            color(label, *codes),
            color(pkg, BOLD, CYAN),
            color(f"({elapsed:.2f}s)", DIM),
        ]
    )
    if status == "PASS" and not verbose:
        # Collapsed: header only, counts when parseable.
        counts_str = (
            f"  [{counts[0]} passed]" if counts[0] or counts[1] else ""
        )
        print(header + color(counts_str, DIM))
    else:
        print(header)
        body = output.rstrip("\n")
        if body:
            for line in body.splitlines():
                print("    " + line)
        print()


def main():
    parser = argparse.ArgumentParser(
        description="Per-package Go test runner with colored output."
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="show full output for every package, including PASS",
    )
    parser.add_argument(
        "--pkg",
        metavar="SUBSTRING",
        default=None,
        help="only test packages whose path contains SUBSTRING",
    )
    args = parser.parse_args()

    if shutil.which("go") is None:
        sys.stderr.write(
            color("error: `go` not found on PATH; install Go or fix PATH\n",
                  RED, BOLD)
        )
        sys.exit(2)

    root = repo_root()
    pkgs = discover_packages(root, args.pkg)
    if not pkgs:
        print(color("no packages matched", YELLOW))
        sys.exit(0)

    print(color(f"Testing {len(pkgs)} package(s) under {root}\n", BOLD))

    wall_start = time.monotonic()
    stats = {"PASS": 0, "FAIL": 0, "SKIP": 0}
    failures = []
    try:
        for pkg in pkgs:
            status, counts, output, elapsed = run_package(root, pkg)
            stats[status] += 1
            render(pkg, status, counts, output, elapsed, args.verbose)
            if status == "FAIL":
                failures.append(pkg)
    except KeyboardInterrupt:
        wall = time.monotonic() - wall_start
        print(
            color(
                f"\nInterrupted after {wall:.2f}s — "
                f"{stats['PASS']} passed, {stats['FAIL']} failed, "
                f"{stats['SKIP']} skipped",
                YELLOW,
            )
        )
        sys.exit(130)
    wall = time.monotonic() - wall_start

    # Summary line.
    parts = [
        color(f"{len(pkgs)} package(s)", BOLD),
        color(f"{stats['PASS']} passed", GREEN),
        color(f"{stats['FAIL']} failed", RED if stats["FAIL"] else DIM),
        color(f"{stats['SKIP']} skipped", YELLOW if stats["SKIP"] else DIM),
        color(f"{wall:.2f}s total", DIM),
    ]
    print("\n" + " | ".join(parts))
    if failures:
        print(color("failed: " + ", ".join(failures), RED))
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    main()
