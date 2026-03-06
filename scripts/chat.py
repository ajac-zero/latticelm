#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "rich>=13.7.0",
#     "openai>=1.0.0",
# ]
# ///

"""
Terminal chat interface for latticelm.

Usage:
    python chat.py
    python chat.py --url http://localhost:8080
    python chat.py --model gemini-2.0-flash-exp
    python chat.py --token $(gcloud auth print-identity-token)
"""

import argparse
import json
import sys
from datetime import datetime
from typing import Optional, Any

from openai import OpenAI, APIStatusError
from rich.console import Console
from rich.live import Live
from rich.markdown import Markdown
from rich.panel import Panel
from rich.prompt import Prompt
from rich.table import Table


# Define available tools in OpenResponses format
TOOLS = [
    {
        "type": "function",
        "name": "calculator",
        "description": "Perform basic arithmetic operations. Supports addition, subtraction, multiplication, and division.",
        "parameters": {
            "type": "object",
            "properties": {
                "operation": {
                    "type": "string",
                    "enum": ["add", "subtract", "multiply", "divide"],
                    "description": "The arithmetic operation to perform"
                },
                "a": {
                    "type": "number",
                    "description": "The first number"
                },
                "b": {
                    "type": "number",
                    "description": "The second number"
                }
            },
            "required": ["operation", "a", "b"]
        }
    },
    {
        "type": "function",
        "name": "get_current_time",
        "description": "Get the current time in a specified timezone or UTC",
        "parameters": {
            "type": "object",
            "properties": {
                "timezone": {
                    "type": "string",
                    "description": "Timezone name (e.g., 'UTC', 'America/New_York', 'Europe/London'). Defaults to UTC.",
                }
            }
        }
    }
]


def execute_tool(tool_name: str, arguments: dict[str, Any]) -> str:
    """Execute a tool and return the result as a string."""
    if tool_name == "calculator":
        operation = arguments["operation"]
        a = arguments["a"]
        b = arguments["b"]

        if operation == "add":
            result = a + b
        elif operation == "subtract":
            result = a - b
        elif operation == "multiply":
            result = a * b
        elif operation == "divide":
            if b == 0:
                return json.dumps({"error": "Division by zero"})
            result = a / b
        else:
            return json.dumps({"error": f"Unknown operation: {operation}"})

        return json.dumps({"result": result, "operation": operation, "a": a, "b": b})

    elif tool_name == "get_current_time":
        # Simple implementation without pytz
        timezone = arguments.get("timezone", "UTC")
        now = datetime.now()
        return json.dumps({
            "current_time": now.isoformat(),
            "timezone": timezone,
            "note": "Showing local system time (timezone parameter not fully implemented)"
        })

    else:
        return json.dumps({"error": f"Unknown tool: {tool_name}"})


class ChatClient:
    def __init__(self, base_url: str, token: Optional[str] = None):
        self.base_url = base_url.rstrip("/")
        self.client = OpenAI(
            base_url=f"{self.base_url}/v1",
            api_key=token or "no-key",
        )
        self.messages = []
        self.console = Console()
        self.tools_enabled = True
    
    def chat(self, user_message: str, model: str, stream: bool = True):
        """Send a chat message and get response."""
        # Add user message to history as a message-type input item
        self.messages.append({
            "type": "message",
            "role": "user",
            "content": [{"type": "input_text", "text": user_message}]
        })

        if stream:
            return self._stream_response(model)
        else:
            return self._sync_response(model)

    @staticmethod
    def _get_attr(obj: Any, key: str, default: Any = None) -> Any:
        """Access object attributes safely for both SDK objects and dicts."""
        if obj is None:
            return default
        if isinstance(obj, dict):
            return obj.get(key, default)
        return getattr(obj, key, default)

    def _extract_stream_error(self, event: Any) -> str:
        """Extract error message from a response.failed event."""
        response = self._get_attr(event, "response")
        error = self._get_attr(response, "error")
        message = self._get_attr(error, "message")
        if message:
            return str(message)
        return "streaming request failed"

    def _extract_completed_text(self, event: Any) -> str:
        """Extract assistant output text from a response.completed event."""
        response = self._get_attr(event, "response")
        output_items = self._get_attr(response, "output", []) or []

        text_parts = []
        for item in output_items:
            if self._get_attr(item, "type") != "message":
                continue
            for part in self._get_attr(item, "content", []) or []:
                if self._get_attr(part, "type") == "output_text":
                    text = self._get_attr(part, "text", "")
                    if text:
                        text_parts.append(str(text))

        return "".join(text_parts)
    
    def _sync_response(self, model: str) -> str:
        """Non-streaming response with tool support."""
        max_iterations = 10  # Prevent infinite loops
        iteration = 0

        while iteration < max_iterations:
            iteration += 1

            with self.console.status(f"[bold blue]Thinking ({model})..."):
                kwargs = {
                    "model": model,
                    "input": self.messages,
                }
                if self.tools_enabled:
                    kwargs["tools"] = TOOLS

                response = self.client.responses.create(**kwargs)

            # Check if there are tool calls
            tool_calls = []
            assistant_content = []
            text_parts = []

            for item in response.output:
                if item.type == "message":
                    # Extract text from message content
                    for content_block in item.content:
                        if content_block.type == "output_text":
                            text_parts.append(content_block.text)
                            assistant_content.append({"type": "output_text", "text": content_block.text})
                elif item.type == "function_call":
                    # Parse arguments JSON string
                    try:
                        arguments = json.loads(item.arguments)
                    except json.JSONDecodeError:
                        arguments = {}

                    tool_calls.append({
                        "id": item.call_id,
                        "name": item.name,
                        "arguments": arguments
                    })
                    assistant_content.append({
                        "type": "tool_use",
                        "id": item.call_id,
                        "name": item.name,
                        "input": arguments
                    })

            # Add assistant message to history as a message-type input item
            if assistant_content:
                self.messages.append({
                    "type": "message",
                    "role": "assistant",
                    "content": assistant_content
                })

            # If no tool calls, we're done
            if not tool_calls:
                return "\n".join(text_parts) if text_parts else ""

            # Execute tools and add results
            self.console.print(f"[dim]Executing {len(tool_calls)} tool(s)...[/dim]")
            tool_results = []

            for tool_call in tool_calls:
                self.console.print(f"[dim]  → {tool_call['name']}({json.dumps(tool_call['arguments'])})[/dim]")
                result = execute_tool(tool_call["name"], tool_call["arguments"])
                tool_results.append({
                    "type": "function_call_output",
                    "call_id": tool_call["id"],
                    "output": result
                })

            # Add tool results to input
            self.messages.extend(tool_results)

            # Continue the loop to get the next response

        return "[Error: Max iterations reached in tool calling loop]"
    
    def _stream_response(self, model: str) -> str:
        """Streaming response with live rendering and tool support."""
        max_iterations = 10
        iteration = 0

        while iteration < max_iterations:
            iteration += 1
            assistant_text = ""
            stream_error = None
            tool_calls = {}  # Dict to track tool calls by item_id
            tool_calls_list = []  # Final list of completed tool calls
            assistant_content = []

            with Live(console=self.console, refresh_per_second=10) as live:
                kwargs = {
                    "model": model,
                    "input": self.messages,
                    "stream": True,
                }
                if self.tools_enabled:
                    kwargs["tools"] = TOOLS

                stream = self.client.responses.create(**kwargs)

                for event in stream:
                    if event.type == "response.output_text.delta":
                        assistant_text += event.delta
                        live.update(Markdown(assistant_text))
                    elif event.type == "response.completed":
                        # Some providers may emit final text only in response.completed.
                        if not assistant_text:
                            completed_text = self._extract_completed_text(event)
                            if completed_text:
                                assistant_text = completed_text
                                live.update(Markdown(assistant_text))
                    elif event.type == "response.failed":
                        stream_error = self._extract_stream_error(event)
                    elif event.type == "response.output_item.added":
                        if hasattr(event, 'item') and event.item.type == "function_call":
                            # Start tracking a new tool call
                            tool_calls[event.item.id] = {
                                "id": event.item.call_id,
                                "name": event.item.name,
                                "arguments": "",
                                "item_id": event.item.id
                            }
                    elif event.type == "response.function_call_arguments.delta":
                        # Accumulate arguments for the current function call
                        # Find which tool call this belongs to by item_id
                        if hasattr(event, 'item_id') and event.item_id in tool_calls:
                            tool_calls[event.item_id]["arguments"] += event.delta
                    elif event.type == "response.output_item.done":
                        if hasattr(event, 'item') and event.item.type == "function_call":
                            # Function call is complete
                            if event.item.id in tool_calls:
                                tool_call = tool_calls[event.item.id]
                                try:
                                    # Parse the complete arguments JSON
                                    tool_call["arguments"] = json.loads(tool_call["arguments"])
                                    tool_calls_list.append(tool_call)
                                except json.JSONDecodeError:
                                    self.console.print(f"[red]Error parsing tool arguments JSON[/red]")

            if stream_error:
                self.console.print(f"[bold red]Error:[/bold red] {stream_error}")
                return ""

            # Build assistant content
            if assistant_text:
                assistant_content.append({"type": "output_text", "text": assistant_text})

            for tool_call in tool_calls_list:
                assistant_content.append({
                    "type": "tool_use",
                    "id": tool_call["id"],
                    "name": tool_call["name"],
                    "input": tool_call["arguments"]
                })

            # Add to history as a message-type input item
            if assistant_content:
                self.messages.append({
                    "type": "message",
                    "role": "assistant",
                    "content": assistant_content
                })

            # If no tool calls, we're done
            if not tool_calls_list:
                return assistant_text

            # Execute tools
            self.console.print(f"\n[dim]Executing {len(tool_calls_list)} tool(s)...[/dim]")
            tool_results = []

            for tool_call in tool_calls_list:
                self.console.print(f"[dim]  → {tool_call['name']}({json.dumps(tool_call['arguments'])})[/dim]")
                result = execute_tool(tool_call["name"], tool_call["arguments"])
                tool_results.append({
                    "type": "function_call_output",
                    "call_id": tool_call["id"],
                    "output": result
                })

            # Add tool results to input
            self.messages.extend(tool_results)

            # Continue loop for next response

        return "[Error: Max iterations reached in tool calling loop]"
    
    def clear_history(self):
        """Clear conversation history."""
        self.messages = []


def print_models_table(client: OpenAI):
    """Fetch and print available models from the gateway."""
    console = Console()
    try:
        models = client.models.list()
    except Exception as e:
        console.print(f"[red]Failed to fetch models: {e}[/red]")
        return

    table = Table(title="Available Models", show_header=True, header_style="bold magenta")
    table.add_column("Owner", style="cyan")
    table.add_column("Model ID", style="green")

    for model in models:
        table.add_row(model.owned_by, model.id)

    console.print(table)


def print_tools_table():
    """Print available tools."""
    console = Console()
    table = Table(title="Available Tools", show_header=True, header_style="bold magenta")
    table.add_column("Tool Name", style="cyan")
    table.add_column("Description", style="green")

    for tool in TOOLS:
        if tool.get("type") == "function":
            table.add_row(tool["name"], tool["description"])

    console.print(table)


def main():
    parser = argparse.ArgumentParser(description="Chat with latticelm")
    parser.add_argument("--url", default="http://localhost:8080", help="Gateway URL")
    parser.add_argument("--model", default=None, help="Model to use (defaults to first available)")
    parser.add_argument("--token", help="Auth token (Bearer)")
    parser.add_argument("--no-stream", action="store_true", help="Disable streaming")
    args = parser.parse_args()
    
    console = Console()
    client = ChatClient(args.url, args.token)
    
    # Fetch available models and select default
    try:
        available_models = list(client.client.models.list())
    except Exception as e:
        console.print(f"[bold red]Failed to connect to gateway:[/bold red] {e}")
        sys.exit(1)
    
    if not available_models:
        console.print("[bold red]Error:[/bold red] No models are configured on the gateway.")
        sys.exit(1)
    
    if args.model:
        current_model = args.model
    else:
        current_model = available_models[0].id
    stream_enabled = not args.no_stream
    
    # Welcome banner
    console.print(Panel.fit(
        "[bold cyan]latticelm Chat Interface[/bold cyan]\n"
        f"Connected to: [green]{args.url}[/green]\n"
        f"Model: [yellow]{current_model}[/yellow]\n"
        f"Streaming: [{'green' if stream_enabled else 'red'}]{stream_enabled}[/]\n"
        f"Tools: [{'green' if client.tools_enabled else 'red'}]{client.tools_enabled}[/]\n\n"
        "Commands:\n"
        "  [bold]/model <name>[/bold] - Switch model\n"
        "  [bold]/models[/bold] - List available models\n"
        "  [bold]/stream[/bold] - Toggle streaming\n"
        "  [bold]/tools[/bold] - Toggle tool calling\n"
        "  [bold]/listtools[/bold] - List available tools\n"
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
                        "  /tools - Toggle tool calling\n"
                        "  /listtools - List available tools\n"
                        "  /clear - Clear conversation\n"
                        "  /quit - Exit",
                        title="Help",
                        border_style="cyan"
                    ))
                
                elif cmd == "/models":
                    print_models_table(client.client)

                elif cmd == "/listtools":
                    print_tools_table()

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

                elif cmd == "/tools":
                    client.tools_enabled = not client.tools_enabled
                    console.print(f"[green]Tools {'enabled' if client.tools_enabled else 'disabled'}[/green]")

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
                
            except APIStatusError as e:
                console.print(f"[bold red]Error {e.status_code}:[/bold red] {str(e)}")
            except Exception as e:
                console.print(f"[bold red]Error:[/bold red] {e}")
        
        except KeyboardInterrupt:
            console.print("\n[yellow]Use /quit to exit[/yellow]")
        except EOFError:
            break


if __name__ == "__main__":
    main()
