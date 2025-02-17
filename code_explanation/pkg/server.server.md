This file implements the JSON‑RPC server that orchestrates Docker operations based on LLM-generated plans and direct tool calls. It registers tool handlers and provides RPC methods for LLM integration and plan execution.

## Overview

- Tool Registration:
  The server maintains a registry of tools (Docker operations). Each tool has:

      - A name.
      - A description.
      - An input JSON schema.
      - A handler function (which wraps Docker API calls).

- Server Structure:
  The Server struct holds:

      A Docker client.
      An LLM client.
      A registry (map) of tools.

- NewServer:
  Initializes the Docker client, LLM client, and registers the built-in tools (such as create_network, create_container, create_volume, run_container, and pull_image).
  Each tool registration provides a lambda function that calls the corresponding function in pkg/docker.

- RPC Methods:

  - CallLLM:
    - Receives user instructions, builds a list of available tools (for the LLM), constructs a prompt, and invokes the LLM client’s GeneratePlan method. The plan is returned as a JSON string.
  - ExecutePlan:
    - Receives a JSON plan, unmarshals it, and iterates through each action. It looks up the corresponding tool and calls its handler. The method returns an RPCResponse indicating success or failure.
  - CallTool:
    - Allows direct invocation of a specific tool based on provided parameters (useful for testing or specific operations).

- HTTP Adapter & RPC Server Startup:
  The StartRPCServer function sets up an HTTP server on port 1234 that:

      - Registers the server with rpc.RegisterName.
      - Exposes a handler that adapts HTTP requests to the JSON‑RPC codec.
      - Uses a custom httpReadWriteCloser to bridge between HTTP and RPC.
