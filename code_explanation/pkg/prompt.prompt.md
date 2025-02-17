This file implements dynamic prompt generation for the Docker Compose use case. The prompt is generated based on the current state of Docker resources and input arguments provided by the user.

## Overview

### Data Structures:

- GetPromptResult: Contains one or more prompt messages.
  `PromptMessage`: Represents a single prompt message with a role and content.
  `TextContent`: Contains the type (always "text") and the text of the prompt.
  `DockerComposePromptInput`: Represents the expected input structure when generating a prompt.
  `GetPrompt` Function:

- This function performs the following:

- Validates the input arguments.
- Constructs a project label based on the provided project name.
- Lists Docker containers, volumes, and networks that match the project label using the Docker client.
- Formats each of these resource lists as pretty-printed JSON.
- Builds a multi-line system prompt that includes details of the current state of resources and instructions on how to proceed.
- Returns a GetPromptResult containing a single PromptMessage.
