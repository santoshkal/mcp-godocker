// pkg/services/git.go

package services

import (
	"context"
	"fmt"
	"os/exec"

	"santoshkal/mcp-godocker/pkg/server"
)

type GitService struct{}

func NewGitService() *GitService {
	return &GitService{}
}

func (gs *GitService) Name() string {
	return "git"
}

func (gs *GitService) RegisterTools(s *server.Server) {
	s.RegisterTool("git_init", "Initialize a new Git repository", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to initialize the repository",
			},
		},
		"required": []string{"path"},
	}, gitInitHandler)

	s.RegisterTool("git_status", "Get Git repository status", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path of the repository",
			},
		},
		"required": []string{"path"},
	}, gitStatusHandler)

	s.RegisterTool("git_add", "Add files to staging", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path of the repository",
			},
			"files": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
				"description": "Files to add",
			},
		},
		"required": []string{"path", "files"},
	}, gitAddHandler)

	s.RegisterTool("git_commit", "Commit changes in Git repository", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path of the repository",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Commit message",
			},
		},
		"required": []string{"path", "message"},
	}, gitCommitHandler)

	s.RegisterTool("git_diff", "Show Git diff", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path of the repository",
			},
		},
		"required": []string{"path"},
	}, gitDiffHandler)
}

func gitInitHandler(ctx context.Context, s *server.Server, parameters map[string]interface{}) error {
	path, _ := parameters["path"].(string)
	if path == "" {
		return fmt.Errorf("missing path for git_init")
	}
	cmd := exec.CommandContext(ctx, "git", "init", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git init error: %s, output: %s", err, string(output))
	}
	return nil
}

func gitStatusHandler(ctx context.Context, s *server.Server, parameters map[string]interface{}) error {
	path, _ := parameters["path"].(string)
	if path == "" {
		return fmt.Errorf("missing path for git_status")
	}
	cmd := exec.CommandContext(ctx, "git", "-C", path, "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git status error: %s, output: %s", err, string(output))
	}
	return nil
}

func gitAddHandler(ctx context.Context, s *server.Server, parameters map[string]interface{}) error {
	path, _ := parameters["path"].(string)
	filesInterface, ok := parameters["files"].([]interface{})
	if !ok || path == "" {
		return fmt.Errorf("missing parameters for git_add")
	}
	var files []string
	for _, f := range filesInterface {
		if fileStr, ok := f.(string); ok {
			files = append(files, fileStr)
		}
	}
	args := append([]string{"-C", path, "add"}, files...)
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git add error: %s, output: %s", err, string(output))
	}
	return nil
}

func gitCommitHandler(ctx context.Context, s *server.Server, parameters map[string]interface{}) error {
	path, _ := parameters["path"].(string)
	message, _ := parameters["message"].(string)
	if path == "" || message == "" {
		return fmt.Errorf("missing parameters for git_commit")
	}
	cmd := exec.CommandContext(ctx, "git", "-C", path, "commit", "-m", message)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git commit error: %s, output: %s", err, string(output))
	}
	return nil
}

func gitDiffHandler(ctx context.Context, s *server.Server, parameters map[string]interface{}) error {
	path, _ := parameters["path"].(string)
	if path == "" {
		return fmt.Errorf("missing path for git_diff")
	}
	cmd := exec.CommandContext(ctx, "git", "-C", path, "diff")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git diff error: %s, output: %s", err, string(output))
	}
	return nil
}
