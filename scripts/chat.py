#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "rich>=13.7.0",
#     "httpx>=0.27.0",
# ]
# ///

"""
Terminal chat interface for go-llm-gateway.

Usage:
    python chat.py
    python chat.py --url http://localhost:8080
    python chat.py --model gemini-2.0-flash-exp
    python chat.py --token $(gcloud auth print-identity-token)
"""

import argparse
import json
import sys
from typing import Optional

import httpx
from rich.console import Console
from rich.live import Live
from rich.markdown import Markdown
from rich.panel import Panel
from rich.prompt import Prompt
from rich.table import Table


class ChatClient:
    def __init__(self, base_url: str, token: Optional[str] = None):
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.messages = []
        self.console = Console()
        
    def _headers(self) -> dict:
        headers = {"Content-Type": "application/json"}
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        return headers
    
    def chat(self, user_message: str, model: str, stream: bool = True):
        """Send a chat message and get response."""
        # Add user message to history
        self.messages.append({
            "role": "user",
            "content": [{"type": "input_text", "text": user_message}]
        })
        
        payload = {
            "model": model,
            "input": self.messages,
            "stream": stream
        }
        
        if stream:
            return self._stream_response(payload, model)
        else:
            return self._sync_response(payload, model)
    
    def _sync_response(self, payload: dict, model: str) -> str:
        """Non-streaming response."""
        with self.console.status(f"[bold blue]Thinking ({model})..."):
            resp = httpx.post(
                f"{self.base_url}/v1/responses",
                json=payload,
                headers=self._headers(),
                timeout=60.0
            )
            resp.raise_for_status()
        
        data = resp.json()
        assistant_text = ""
        
        for msg in data.get("output", []):
            for block in msg.get("content", []):
                if block.get("type") == "output_text":
                    assistant_text += block.get("text", "")
        
        # Add to history
        self.messages.append({
            "role": "assistant",
            "content": [{"type": "output_text", "text": assistant_text}]
        })
        
        return assistant_text
    
    def _stream_response(self, payload: dict, model: str) -> str:
        """Streaming response with live rendering."""
        assistant_text = ""
        
        with httpx.stream(
            "POST",
            f"{self.base_url}/v1/responses",
            json=payload,
            headers=self._headers(),
            timeout=60.0
        ) as resp:
            resp.raise_for_status()
            
            with Live(console=self.console, refresh_per_second=10) as live:
                for line in resp.iter_lines():
                    if not line.startswith("data: "):
                        continue
                    
                    data_str = line[6:]  # Remove "data: " prefix
                    
                    try:
                        chunk = json.loads(data_str)
                    except json.JSONDecodeError:
                        continue
                    
                    if chunk.get("done"):
                        break
                    
                    delta = chunk.get("delta", {})
                    for block in delta.get("content", []):
                        if block.get("type") == "output_text":
                            assistant_text += block.get("text", "")
                    
                    # Render markdown in real-time
                    live.update(Markdown(assistant_text))
        
        # Add to history
        self.messages.append({
            "role": "assistant",
            "content": [{"type": "output_text", "text": assistant_text}]
        })
        
        return assistant_text
    
    def clear_history(self):
        """Clear conversation history."""
        self.messages = []


def print_models_table():
    """Print available models table."""
    table = Table(title="Available Models", show_header=True, header_style="bold magenta")
    table.add_column("Provider", style="cyan")
    table.add_column("Model ID", style="green")
    table.add_column("Alias", style="yellow")
    
    table.add_row("OpenAI", "gpt-4o", "gpt4")
    table.add_row("OpenAI", "gpt-4o-mini", "gpt4-mini")
    table.add_row("OpenAI", "o1", "o1")
    table.add_row("Anthropic", "claude-3-5-sonnet-20241022", "claude")
    table.add_row("Anthropic", "claude-3-5-haiku-20241022", "haiku")
    table.add_row("Google", "gemini-2.0-flash-exp", "gemini")
    table.add_row("Google", "gemini-1.5-pro", "gemini-pro")
    
    Console().print(table)


def main():
    parser = argparse.ArgumentParser(description="Chat with go-llm-gateway")
    parser.add_argument("--url", default="http://localhost:8080", help="Gateway URL")
    parser.add_argument("--model", default="gemini-2.0-flash-exp", help="Model to use")
    parser.add_argument("--token", help="Auth token (Bearer)")
    parser.add_argument("--no-stream", action="store_true", help="Disable streaming")
    args = parser.parse_args()
    
    console = Console()
    client = ChatClient(args.url, args.token)
    current_model = args.model
    stream_enabled = not args.no_stream
    
    # Welcome banner
    console.print(Panel.fit(
        "[bold cyan]go-llm-gateway Chat Interface[/bold cyan]\n"
        f"Connected to: [green]{args.url}[/green]\n"
        f"Model: [yellow]{current_model}[/yellow]\n"
        f"Streaming: [{'green' if stream_enabled else 'red'}]{stream_enabled}[/]\n\n"
        "Commands:\n"
        "  [bold]/model <name>[/bold] - Switch model\n"
        "  [bold]/models[/bold] - List available models\n"
        "  [bold]/stream[/bold] - Toggle streaming\n"
        "  [bold]/clear[/bold] - Clear conversation\n"
        "  [bold]/quit[/bold] or [bold]/exit[/bold] - Exit\n"
        "  [bold]/help[/bold] - Show this help",
        title="Welcome",
        border_style="cyan"
    ))
    
    # Model aliases
    model_aliases = {
        "gpt4": "gpt-4o",
        "gpt4-mini": "gpt-4o-mini",
        "o1": "o1",
        "claude": "claude-3-5-sonnet-20241022",
        "haiku": "claude-3-5-haiku-20241022",
        "gemini": "gemini-2.0-flash-exp",
        "gemini-pro": "gemini-1.5-pro"
    }
    
    while True:
        try:
            user_input = Prompt.ask("\n[bold blue]You[/bold blue]")
            
            if not user_input.strip():
                continue
            
            # Handle commands
            if user_input.startswith("/"):
                cmd_parts = user_input.split(maxsplit=1)
                cmd = cmd_parts[0].lower()
                
                if cmd in ["/quit", "/exit"]:
                    console.print("[yellow]Goodbye! 👋[/yellow]")
                    break
                
                elif cmd == "/help":
                    console.print(Panel(
                        "[bold]Commands:[/bold]\n"
                        "  /model <name> - Switch model\n"
                        "  /models - List available models\n"
                        "  /stream - Toggle streaming\n"
                        "  /clear - Clear conversation\n"
                        "  /quit - Exit",
                        title="Help",
                        border_style="cyan"
                    ))
                
                elif cmd == "/models":
                    print_models_table()
                
                elif cmd == "/model":
                    if len(cmd_parts) < 2:
                        console.print("[red]Usage: /model <model-name>[/red]")
                        continue
                    
                    new_model = cmd_parts[1]
                    # Check if it's an alias
                    new_model = model_aliases.get(new_model, new_model)
                    current_model = new_model
                    console.print(f"[green]Switched to model: {current_model}[/green]")
                
                elif cmd == "/stream":
                    stream_enabled = not stream_enabled
                    console.print(f"[green]Streaming {'enabled' if stream_enabled else 'disabled'}[/green]")
                
                elif cmd == "/clear":
                    client.clear_history()
                    console.print("[green]Conversation history cleared[/green]")
                
                else:
                    console.print(f"[red]Unknown command: {cmd}[/red]")
                
                continue
            
            # Send message to LLM
            try:
                console.print(f"\n[bold green]Assistant ({current_model})[/bold green]")
                response = client.chat(user_input, current_model, stream=stream_enabled)
                
                if not stream_enabled:
                    # For non-streaming, render markdown
                    console.print(Markdown(response))
                
            except httpx.HTTPStatusError as e:
                console.print(f"[bold red]Error {e.response.status_code}:[/bold red] {e.response.text}")
            except Exception as e:
                console.print(f"[bold red]Error:[/bold red] {e}")
        
        except KeyboardInterrupt:
            console.print("\n[yellow]Use /quit to exit[/yellow]")
        except EOFError:
            break


if __name__ == "__main__":
    main()
