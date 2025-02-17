

This file serves as the entry point for the client application. It uses the JSON‑RPC client (implemented in `pkg/rpcclient/client.go`) to communicate with the server. The client sends user instructions to the server to generate a plan (using the LLM) and then executes that plan by invoking the appropriate Docker operations.

## Overview

- **Imports:**  
  Imports necessary packages for JSON encoding/decoding, context management, logging, and the custom packages for shared types (`pkg/mcp`) and the JSON‑RPC client (`pkg/rpcclient`).

- **Main Function:**  
  The `main()` function is the client’s entry point. It performs the following steps:
  1. Creates a new JSON‑RPC client targeting the server endpoint.
  2. Sends a user instruction (e.g., “Generate a plan to create a MySQL container”) to the server’s `CallLLM` method.
  3. Validates and logs the plan JSON received from the server.
  4. Invokes the server’s `ExecutePlan` method to execute the plan.
  5. Unmarshals and prints the execution result.

## Detailed Explanation

- **Creating the RPC Client:**  
  ```go
  client := rpcclient.NewRPCClient("http://localhost:1234/rpc")
  ctx := context.Background()
  ```

A new RPC client is created with a 5‑minute timeout. The ctx variable holds the background context for the RPC calls.

- Calling Server.CallLLM:

```go

userInstructions := "Generate a plan to create a MySQL container"
var planJSON string
if err := client.CallAndParse(ctx, "Server.CallLLM", &planJSON, userInstructions); err != nil {
    log.Fatalf("Error calling Server.CallLLM: %v", err)
}
fmt.Printf("Plan JSON: %s\n", planJSON)
```
The client sends a human-readable instruction to the server. The server’s LLM integration generates a plan in JSON format. This response is parsed into the planJSON string variable.

- Validating the Plan JSON:
```go
if !json.Valid([]byte(planJSON)) {
    log.Fatalf("Invalid JSON received from Server.CallLLM")
}
```
The code verifies that the received JSON is valid.

- Executing the Plan:

```go
var execResp mcp.RPCResponse
if err := client.CallAndParse(ctx, "Server.ExecutePlan", &execResp, planJSON); err != nil {
    log.Fatalf("Error calling Server.ExecutePlan: %v", err)
}
```
The plan is then sent back to the server’s ExecutePlan method. The response (an RPCResponse) is parsed into execResp.

- Processing the Execution Result:

```go
var result map[string]string
if err := json.Unmarshal(execResp.Result, &result); err != nil {
    log.Fatalf("Error unmarshalling result: %v", err)
}
fmt.Printf("Plan execution result: Status=%s, Message=%s\n", result["status"], result["message"])
```
The final step is to unmarshal the JSON result contained in the RPCResponse.Result field and print the status and message. This result indicates whether the plan execution was successful.
