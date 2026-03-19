#!/usr/bin/env python3
"""
broker-cli — Admin CLI for mini-go-broker
Observability and operational control via HTTP endpoints.
"""

import os
import sys
import json
import click
import requests
from rich.console import Console
from rich.table import Table
from rich import box
from rich.panel import Panel
from rich.text import Text

console = Console()

DEFAULT_HOST = os.getenv("BROKER_HOST", "localhost")
DEFAULT_METRICS_PORT = os.getenv("BROKER_METRICS_PORT", "8080")


def metrics_url(host: str, port: str) -> str:
    return f"http://{host}:{port}/metrics/json"


def fetch_metrics(host: str, port: str) -> dict:
    url = metrics_url(host, port)
    try:
        response = requests.get(url, timeout=5)
        response.raise_for_status()
        return response.json()
    except requests.exceptions.ConnectionError:
        console.print(f"[red]✗ Could not connect to broker at {url}[/red]")
        console.print("[dim]Is the broker running? Check BROKER_HOST and BROKER_METRICS_PORT.[/dim]")
        sys.exit(1)
    except requests.exceptions.Timeout:
        console.print("[red]✗ Request timed out.[/red]")
        sys.exit(1)
    except requests.exceptions.HTTPError as e:
        console.print(f"[red]✗ HTTP error: {e}[/red]")
        sys.exit(1)
    except json.JSONDecodeError:
        console.print("[red]✗ Invalid JSON response from broker.[/red]")
        sys.exit(1)


@click.group()
@click.option("--host", default=DEFAULT_HOST, show_default=True, help="Broker host")
@click.option("--port", default=DEFAULT_METRICS_PORT, show_default=True, help="Metrics port")
@click.pass_context
def cli(ctx, host, port):
    """broker-cli — Admin tool for mini-go-broker."""
    ctx.ensure_object(dict)
    ctx.obj["host"] = host
    ctx.obj["port"] = port


# --------------------------------------------------------------------------- #
# metrics                                                                       #
# --------------------------------------------------------------------------- #

@cli.command()
@click.pass_context
def metrics(ctx):
    """Display broker metrics snapshot."""
    host = ctx.obj["host"]
    port = ctx.obj["port"]
    data = fetch_metrics(host, port)

    # Gauges table
    gauge_table = Table(
        title="Gauges",
        box=box.ROUNDED,
        show_header=True,
        header_style="bold cyan",
    )
    gauge_table.add_column("Metric", style="dim")
    gauge_table.add_column("Value", justify="right")

    gauge_table.add_row("Ready Queue Size", str(data.get("Ready", "N/A")))
    gauge_table.add_row("Inflight Count", str(data.get("Inflight", "N/A")))
    gauge_table.add_row("DLQ Size", str(data.get("DLQ", "N/A")))
    gauge_table.add_row("Avg Processing Latency (ms)", str(data.get("AverageLatencyMillis", "N/A")))

    # Counters table
    counter_table = Table(
        title="Counters",
        box=box.ROUNDED,
        show_header=True,
        header_style="bold cyan",
    )
    counter_table.add_column("Event", style="dim")
    counter_table.add_column("Total", justify="right")

    # Counters
    counter_table.add_row("Published", str(data.get("TotalPublished", "N/A")))
    counter_table.add_row("Acked", str(data.get("TotalAcked", "N/A")))
    counter_table.add_row("Nacked", str(data.get("TotalNacked", "N/A")))
    counter_table.add_row("Redelivered", str(data.get("TotalRedelivered", "N/A")))
    counter_table.add_row("Processed", str(data.get("TotalProcessed", "N/A")))
    counter_table.add_row("DLQ Moves", str(data.get("TotalDLQ", "N/A")))

    console.print()
    console.print(gauge_table)
    console.print()
    console.print(counter_table)
    console.print()


# --------------------------------------------------------------------------- #
# health                                                                        #
# --------------------------------------------------------------------------- #

# Configurable thresholds via env
DLQ_WARN_THRESHOLD = int(os.getenv("HEALTH_DLQ_WARN", "10"))
DLQ_CRIT_THRESHOLD = int(os.getenv("HEALTH_DLQ_CRIT", "50"))
INFLIGHT_WARN_THRESHOLD = int(os.getenv("HEALTH_INFLIGHT_WARN", "100"))
LATENCY_WARN_THRESHOLD = float(os.getenv("HEALTH_LATENCY_WARN_MS", "100.0"))


@cli.command()
@click.pass_context
def health(ctx):
    """Check broker health against thresholds. Exits non-zero if critical."""
    host = ctx.obj["host"]
    port = ctx.obj["port"]
    data = fetch_metrics(host, port)

    issues = []
    warnings = []

    dlq_size = data.get("DLQ", 0)
    inflight = data.get("Inflight", 0)
    latency = data.get("AverageLatencyMillis", 0.0)

    if dlq_size >= DLQ_CRIT_THRESHOLD:
        issues.append(f"DLQ size {dlq_size} ≥ critical threshold {DLQ_CRIT_THRESHOLD}")
    elif dlq_size >= DLQ_WARN_THRESHOLD:
        warnings.append(f"DLQ size {dlq_size} ≥ warning threshold {DLQ_WARN_THRESHOLD}")

    if inflight >= INFLIGHT_WARN_THRESHOLD:
        warnings.append(f"Inflight count {inflight} ≥ warning threshold {INFLIGHT_WARN_THRESHOLD}")

    if latency >= LATENCY_WARN_THRESHOLD:
        warnings.append(f"Avg latency {latency}ms ≥ warning threshold {LATENCY_WARN_THRESHOLD}ms")

    console.print()

    if issues:
        console.print(Panel(
            "\n".join(f"[red]✗ {i}[/red]" for i in issues),
            title="[bold red]CRITICAL[/bold red]",
            border_style="red",
        ))
        for w in warnings:
            console.print(f"[yellow]⚠ {w}[/yellow]")
        console.print()
        sys.exit(2)
    elif warnings:
        console.print(Panel(
            "\n".join(f"[yellow]⚠ {w}[/yellow]" for w in warnings),
            title="[bold yellow]WARN[/bold yellow]",
            border_style="yellow",
        ))
        console.print()
        sys.exit(1)
    else:
        console.print(Panel(
            f"[green]✓ Ready: {data.get('Ready', 0)}  "
            f"Inflight: {inflight}  "
            f"DLQ: {dlq_size}  "
            f"Latency: {latency}ms[/green]",
            title="[bold green]OK[/bold green]",
            border_style="green",
        ))
        console.print()


# --------------------------------------------------------------------------- #
# dlq inspect                                                                   #
# --------------------------------------------------------------------------- #

@cli.command("dlq-inspect")
@click.pass_context
def dlq_inspect(ctx):
    """Inspect Dead Letter Queue status."""
    host = ctx.obj["host"]
    port = ctx.obj["port"]
    data = fetch_metrics(host, port)

    dlq_size = data.get("DLQ", 0)
    total_dlq_moves = data.get("TotalDLQ", 0)

    table = Table(
        title="Dead Letter Queue",
        box=box.ROUNDED,
        show_header=True,
        header_style="bold red",
    )
    table.add_column("Metric", style="dim")
    table.add_column("Value", justify="right")

    table.add_row("Current DLQ Size", str(dlq_size))
    table.add_row("Total DLQ Moves (lifetime)", str(total_dlq_moves))

    console.print()
    console.print(table)

    if dlq_size == 0:
        console.print("[green]✓ DLQ is empty.[/green]")
    elif dlq_size >= DLQ_CRIT_THRESHOLD:
        console.print(f"[red]✗ DLQ size is critical ({dlq_size} ≥ {DLQ_CRIT_THRESHOLD}).[/red]")
    elif dlq_size >= DLQ_WARN_THRESHOLD:
        console.print(f"[yellow]⚠ DLQ size is elevated ({dlq_size} ≥ {DLQ_WARN_THRESHOLD}).[/yellow]")

    console.print()


# --------------------------------------------------------------------------- #
# config validate                                                               #
# --------------------------------------------------------------------------- #

CONFIG_VARS = [
    ("GRPC_PORT",               ":50051",   str,   None),
    ("METRICS_PORT",            ":8080",    str,   None),
    ("MAX_RETRIES",             "3",        int,   lambda v: v > 0),
    ("MAX_DLQ_SIZE",            "100",      int,   lambda v: v > 0),
    ("VISIBILITY_TIMEOUT_SEC",  "5",        int,   lambda v: v >= 1),
    ("DRAIN_TIMEOUT_SEC",       "10",       int,   lambda v: v >= 1),
    ("DEFAULT_PREFETCH",        "1",        int,   lambda v: v >= 1),
]

LOGIC_CHECKS = [
    (
        lambda cfg: cfg["DRAIN_TIMEOUT_SEC"] > cfg["VISIBILITY_TIMEOUT_SEC"],
        "DRAIN_TIMEOUT_SEC should be greater than VISIBILITY_TIMEOUT_SEC — "
        "otherwise inflight messages cannot complete before forced requeue.",
    ),
]


@cli.command("config-validate")
def config_validate():
    """Validate broker environment variable configuration."""
    cfg = {}
    issues = []
    warnings = []

    console.print()

    table = Table(
        title="Configuration",
        box=box.ROUNDED,
        show_header=True,
        header_style="bold cyan",
    )
    table.add_column("Variable", style="dim")
    table.add_column("Value", justify="right")
    table.add_column("Source", justify="center")
    table.add_column("Status", justify="center")

    for var, default, cast, validator in CONFIG_VARS:
        raw = os.getenv(var)
        source = "[green]env[/green]" if raw is not None else "[dim]default[/dim]"
        value = raw if raw is not None else default

        try:
            typed = cast(value)
            cfg[var] = typed
        except (ValueError, TypeError):
            issues.append(f"{var}={value!r} cannot be cast to {cast.__name__}")
            table.add_row(var, str(value), source, "[red]✗ invalid type[/red]")
            continue

        if validator and not validator(typed):
            issues.append(f"{var}={typed} failed validation.")
            table.add_row(var, str(typed), source, "[red]✗ invalid value[/red]")
        else:
            table.add_row(var, str(typed), source, "[green]✓[/green]")

    console.print(table)
    console.print()

    # Cross-variable logic checks
    for check_fn, message in LOGIC_CHECKS:
        try:
            if not check_fn(cfg):
                warnings.append(message)
        except KeyError:
            pass  # skip if a variable failed to parse

    if issues:
        console.print(Panel(
            "\n".join(f"[red]✗ {i}[/red]" for i in issues),
            title="[bold red]INVALID[/bold red]",
            border_style="red",
        ))
        console.print()
        sys.exit(1)
    elif warnings:
        console.print(Panel(
            "\n".join(f"[yellow] {w}[/yellow]" for w in warnings),
            title="[bold yellow]WARNINGS[/bold yellow]",
            border_style="yellow",
        ))
        console.print()
    else:
        console.print("[green]✓ All configuration values are valid.[/green]")
        console.print()


if __name__ == "__main__":
    cli(obj={})