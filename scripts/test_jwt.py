#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "requests>=2.31.0",
#     "pyjwt>=2.8.0",
#     "rich>=13.7.0",
#     "typer>=0.12.0",
# ]
# ///

"""
JWT authentication testing script for latticelm gateway.

Tests OIDC provider token exchange, JWT validation, and gateway API access.

Usage:
    python test_jwt.py
    python test_jwt.py --auth-url https://auth.example.com --client-id xxx --client-secret xxx
    python test_jwt.py --gateway-url https://gateway.example.com --token <jwt>
"""

import json
import random
import sys
from datetime import datetime
from typing import Annotated, Optional
from urllib.parse import urljoin, urlparse

import jwt
import requests
import typer
from rich.console import Console
from rich.panel import Panel
from rich.syntax import Syntax
from rich.table import Table

app = typer.Typer(help="Test JWT authentication for latticelm")
console = Console()


def get_token(
    auth_url: str,
    client_id: str,
    client_secret: str,
) -> Optional[str]:
    """Exchange client credentials for an access token."""
    # Authentik token endpoint is always at /application/o/token/ under the base domain.
    # Extract the base domain from the issuer URL.
    parsed = urlparse(auth_url)
    base_url = f"{parsed.scheme}://{parsed.netloc}"
    token_url = base_url + "/application/o/token/"
    
    try:
        console.print(f"[dim]Requesting token from: {token_url}[/dim]")
        resp = requests.post(
            token_url,
            data={
                "grant_type": "client_credentials",
                "client_id": client_id,
                "client_secret": client_secret,
            },
            timeout=10,
        )
        resp.raise_for_status()
        data = resp.json()
        return data.get("access_token")
    except Exception as e:
        console.print(f"[red]Failed to get token: {e}[/red]")
        return None


def decode_token_unverified(token: str) -> dict:
    """Decode JWT without verification to inspect claims."""
    try:
        return jwt.decode(token, options={"verify_signature": False})
    except Exception as e:
        console.print(f"[red]Failed to decode token: {e}[/red]")
        return {}


def format_timestamp(ts: int) -> str:
    """Format Unix timestamp to readable date."""
    try:
        return datetime.fromtimestamp(ts).isoformat()
    except:
        return str(ts)


def display_token_info(token: str, issuer_configured: str):
    """Display decoded JWT claims and validation status."""
    claims = decode_token_unverified(token)
    
    if not claims:
        return

    console.print("\n[bold cyan]Token Claims[/bold cyan]")
    
    table = Table(show_header=True, header_style="bold magenta")
    table.add_column("Claim", style="cyan")
    table.add_column("Value", style="green")
    
    for key, value in claims.items():
        if key in ["exp", "iat", "nbf"]:
            display_value = f"{value} ({format_timestamp(value)})"
        else:
            display_value = str(value)
        table.add_row(key, display_value)
    
    console.print(table)
    
    # Validate issuer (with and without trailing slash)
    iss = claims.get("iss", "")
    iss_normalized = iss.rstrip("/")
    issuer_normalized = issuer_configured.rstrip("/")
    
    console.print("\n[bold cyan]Issuer Validation[/bold cyan]")
    iss_table = Table(show_header=True, header_style="bold magenta")
    iss_table.add_column("Field", style="cyan")
    iss_table.add_column("Value", style="green")
    iss_table.add_row("Token iss claim", iss)
    iss_table.add_row("Configured issuer", issuer_configured)
    iss_table.add_row("iss (normalized)", iss_normalized)
    iss_table.add_row("Configured (normalized)", issuer_normalized)
    iss_table.add_row(
        "Match", 
        "[green]✓[/green]" if iss_normalized == issuer_normalized else "[red]✗[/red]"
    )
    console.print(iss_table)
    
    # Check expiration
    exp = claims.get("exp", 0)
    now = datetime.now().timestamp()
    expired = exp < now
    
    console.print("\n[bold cyan]Expiration Status[/bold cyan]")
    exp_table = Table(show_header=False)
    exp_table.add_row("Expires at", format_timestamp(exp))
    exp_table.add_row("Current time", format_timestamp(int(now)))
    exp_table.add_row(
        "Status",
        f"[red]Expired ({(now - exp):.0f}s ago)[/red]" if expired else "[green]Valid[/green]"
    )
    console.print(exp_table)


def test_gateway_endpoint(
    gateway_url: str,
    token: str,
    endpoint: str,
    method: str = "GET",
    data: Optional[dict] = None,
) -> bool:
    """Test gateway endpoint with JWT token."""
    url = urljoin(gateway_url.rstrip("/") + "/", endpoint.lstrip("/"))
    headers = {"Authorization": f"Bearer {token}"}
    
    try:
        console.print(f"[dim]Testing {method} {url}[/dim]")
        if method == "GET":
            resp = requests.get(url, headers=headers, timeout=10)
        else:
            resp = requests.post(url, headers=headers, json=data, timeout=10)
        
        resp.raise_for_status()
        
        # Display response
        console.print(f"[green]✓ {resp.status_code}[/green]")
        try:
            payload = resp.json()
            syntax = Syntax(
                json.dumps(payload, indent=2),
                "json",
                theme="monokai",
                line_numbers=False,
            )
            console.print(syntax)
        except:
            console.print(f"[dim]{resp.text[:200]}[/dim]")
        
        return True
    except requests.exceptions.HTTPError as e:
        console.print(f"[red]✗ HTTP {e.response.status_code}[/red]")
        try:
            error = e.response.json()
            console.print(f"[red]{json.dumps(error, indent=2)}[/red]")
        except:
            console.print(f"[red]{e.response.text[:200]}[/red]")
        return False
    except Exception as e:
        console.print(f"[red]✗ Error: {e}[/red]")
        return False


@app.command()
def main(
    auth_url: Annotated[
        str,
        typer.Option(help="OIDC provider URL"),
    ] = "https://auth.ia-innovacion.work/application/o/lattice-lm/",
    client_id: Annotated[
        Optional[str],
        typer.Option(help="OAuth2 client ID"),
    ] = None,
    client_secret: Annotated[
        Optional[str],
        typer.Option(help="OAuth2 client secret (read from AUTH_CLIENT_SECRET env var if not provided)"),
    ] = None,
    gateway_url: Annotated[
        str,
        typer.Option(help="Gateway URL"),
    ] = "https://latticelm.ia-innovacion.work",
    token: Annotated[
        Optional[str],
        typer.Option(help="Use existing JWT token (skip token exchange)"),
    ] = None,
    skip_gateway_test: Annotated[
        bool,
        typer.Option(help="Skip gateway endpoint tests"),
    ] = False,
):
    """Test JWT authentication with OIDC provider and gateway."""
    
    # Display config
    console.print(Panel.fit(
        "[bold cyan]JWT Authentication Test[/bold cyan]\n"
        f"Auth URL: [green]{auth_url}[/green]\n"
        f"Gateway URL: [green]{gateway_url}[/green]\n"
        f"Client ID: [yellow]{'*' * 8 if client_id else '[not provided]'}[/yellow]",
        title="Config",
        border_style="cyan"
    ))
    
    # Get token if not provided
    if not token:
        if not client_id or not client_secret:
            console.print("[red]Error: --client-id and --client-secret required for token exchange[/red]")
            raise typer.Exit(1)
        
        token = get_token(auth_url, client_id, client_secret)
        if not token:
            raise typer.Exit(1)
        
        console.print("\n[green]✓ Token obtained[/green]")
    
    # Display token info
    display_token_info(token, auth_url)
    
    # Test gateway endpoints
    if skip_gateway_test:
        console.print("\n[dim]Skipping gateway endpoint tests[/dim]")
        raise typer.Exit(0)
    
    console.print("\n[bold cyan]Gateway API Tests[/bold cyan]")
    
    # Fetch models and pick one at random for the responses test
    console.print("\n[bold]GET /v1/models[/bold]")
    models_url = urljoin(gateway_url.rstrip("/") + "/", "v1/models")
    headers = {"Authorization": f"Bearer {token}"}
    model_id = None
    try:
        console.print(f"[dim]Testing GET {models_url}[/dim]")
        resp = requests.get(models_url, headers=headers, timeout=10)
        resp.raise_for_status()
        payload = resp.json()
        console.print(f"[green]✓ {resp.status_code}[/green]")
        syntax = Syntax(json.dumps(payload, indent=2), "json", theme="monokai", line_numbers=False)
        console.print(syntax)
        success_models = True

        models = [m["id"] for m in payload.get("data", [])]
        if models:
            model_id = random.choice(models)
            console.print(f"\n[dim]Selected model: [bold]{model_id}[/bold][/dim]")
    except requests.exceptions.HTTPError as e:
        console.print(f"[red]✗ HTTP {e.response.status_code}[/red]")
        try:
            console.print(f"[red]{json.dumps(e.response.json(), indent=2)}[/red]")
        except Exception:
            console.print(f"[red]{e.response.text[:200]}[/red]")
        success_models = False
    except Exception as e:
        console.print(f"[red]✗ Error: {e}[/red]")
        success_models = False

    # Test /v1/responses with a real model
    console.print("\n[bold]POST /v1/responses[/bold]")
    minimal_req = {
        "model": model_id or "test",
        "input": [{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "Say hello in one sentence."}]}],
    }
    success_responses = test_gateway_endpoint(
        gateway_url, token, "/v1/responses", method="POST", data=minimal_req
    )
    
    # Summary
    console.print("\n[bold cyan]Summary[/bold cyan]")
    summary = Table(show_header=False)
    summary.add_row("Token", "[green]✓ Valid JWT[/green]")
    summary.add_row(
        "Auth",
        "[green]✓ Accepted by gateway[/green]" if (success_models or success_responses) else "[red]✗ Rejected[/red]"
    )
    console.print(summary)


if __name__ == "__main__":
    app()
