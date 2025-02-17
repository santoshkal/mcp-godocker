This file defines the shared JSON‑RPC types and tool call argument types. These types are used both by the server and the client to structure requests and responses over JSON‑RPC.

## Overview

- RPCRequest:
  Structure for JSON‑RPC requests. Contains JSON‑RPC version, method, parameters, and a unique ID.

- RPCResponse:
  Structure for JSON‑RPC responses. Contains the version, result (as a raw JSON message), any error information, and the corresponding request ID.

- RPCError:
  Defines the error structure used in JSON‑RPC responses with an error code and message.

- NewError & String Method:
  Helper functions to create new errors and to provide a human‑readable representation of the error.

- ToolCallArgs:
  Represents arguments for directly invoking a tool. Contains a tool name and a map of parameters.
