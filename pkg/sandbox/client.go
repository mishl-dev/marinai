package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const (
	sandboxImage    = "sandbox-service:latest"
	sandboxName     = "marinai-sandbox"
	sandboxPort     = "3002"
	containerMemory = 128 * 1024 * 1024 // 128MB limit
)

type Client struct {
	docker    *client.Client
	baseURL   string
	container string
}

type ExecResponse struct {
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
}

type FileInfo struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

func NewClient() (*Client, error) {
	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &Client{
		docker: dockerCli,
	}, nil
}

func (c *Client) ensureRunning(ctx context.Context) error {
	filterArgs := filters.NewArgs()
	filterArgs.Add("name", sandboxName)

	containers, err := c.docker.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})

	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	for _, ctr := range containers {
		if ctr.State == "running" {
			c.baseURL = fmt.Sprintf("http://%s:%s", ctr.ID[:12], sandboxPort)
			c.container = ctr.ID
			return nil
		}
		c.docker.ContainerRemove(ctx, ctr.ID, container.RemoveOptions{Force: true})
	}

	resp, err := c.docker.ContainerCreate(ctx, &container.Config{
		Image: sandboxImage,
		ExposedPorts: nat.PortSet{
			nat.Port(sandboxPort + "/tcp"): {},
		},
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			nat.Port(sandboxPort + "/tcp"): []nat.PortBinding{{HostIP: "0.0.0.0"}},
		},
		Resources: container.Resources{
			Memory:     containerMemory,
			MemorySwap: containerMemory,
		},
		AutoRemove: true,
	}, nil, nil, sandboxName)

	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	if err := c.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	details, err := c.docker.ContainerInspect(ctx, resp.ID)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %w", err)
	}

	bindings := details.NetworkSettings.Ports[nat.Port(sandboxPort+"/tcp")]
	if len(bindings) > 0 {
		c.baseURL = fmt.Sprintf("http://localhost:%s", bindings[0].HostPort)
	} else {
		c.baseURL = fmt.Sprintf("http://%s:%s", resp.ID[:12], sandboxPort)
	}

	c.container = resp.ID

	return c.waitForReady(ctx)
}

func (c *Client) waitForReady(ctx context.Context) error {
	for i := 0; i < 30; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := http.Get(c.baseURL + "/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("sandbox container did not become ready")
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	if err := c.ensureRunning(ctx); err != nil {
		return nil, err
	}

	var reqBody io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (c *Client) Exec(ctx context.Context, command string, timeoutSec int) (*ExecResponse, error) {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	if timeoutSec > 30 {
		timeoutSec = 30
	}

	body, err := c.doRequest(ctx, "POST", "/exec", map[string]interface{}{
		"command": command,
		"timeout": timeoutSec,
	})
	if err != nil {
		return nil, err
	}

	var resp ExecResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Read(ctx context.Context, path string) ([]byte, error) {
	return c.doRequest(ctx, "GET", "/read?path="+path, nil)
}

func (c *Client) Write(ctx context.Context, path, content string) error {
	_, err := c.doRequest(ctx, "POST", "/write", map[string]string{
		"path":    path,
		"content": content,
	})
	return err
}

func (c *Client) List(ctx context.Context, path string) ([]FileInfo, error) {
	body, err := c.doRequest(ctx, "GET", "/list?path="+path, nil)
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	if err := json.Unmarshal(body, &files); err != nil {
		return nil, err
	}
	return files, nil
}

func (c *Client) Close() error {
	return c.docker.Close()
}
