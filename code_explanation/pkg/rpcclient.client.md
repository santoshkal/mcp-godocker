This file provides a reusable JSON‑RPC client for communication with the server. It encapsulates the logic for sending requests, receiving responses, and error handling.

## Overview

- RPCClient Structure:
  Encapsulates an HTTP client and the server endpoint URL.

- NewRPCClient:
  Creates a new RPCClient with a configured HTTP client (with a 5‑minute timeout).

- Call Method:
  This method:

      - Constructs a JSON‑RPC request using the shared RPCRequest type.
      - Sends the request via HTTP POST.
      - Reads and unmarshals the server response into an RPCResponse.
      - Checks for errors in the response and returns the raw result if successful.

- CallAndParse Method:
  A helper that calls Call and then unmarshals the raw JSON result into a user‑provided output structure.
