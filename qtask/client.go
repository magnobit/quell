// Copyright 2026 Magnobit, Inc. All rights reserved.

// Package qtask submits work to the existing QubitLabs scheduler.
// It does not own credentials, routing, or a second job database.
// A handle file only remembers the ids the scheduler returned.
package qtask

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Handle is enough to reconnect after the CLI exits.
// Unknown costs stay nil. They are never stored as zero.
type Handle struct {
	TaskID        string    `json:"taskId"`
	JobID         string    `json:"jobId"`
	ProviderJobID string    `json:"providerJobId,omitempty"`
	Provider      string    `json:"provider,omitempty"`
	Backend       string    `json:"backend,omitempty"`
	Status        string    `json:"status,omitempty"`
	Submitted     time.Time `json:"submitted"`
	EstimatedCost *float64  `json:"estimatedCost,omitempty"`
	ActualCost    *float64  `json:"actualCost,omitempty"`
}

// Client calls the QubitLabs scheduler HTTP API.
type Client struct {
	BaseURL string
	Token   string
	OrgID   string
	HTTP    *http.Client
	Dir     string
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// Submit posts a program to the scheduler and persists the returned job id.
func (c *Client) Submit(program string) (Handle, error) {
	body, _ := json.Marshal(map[string]string{
		"program":  program,
		"language": "quell",
	})
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/v1/orgs/"+c.OrgID+"/scheduler/jobs", bytes.NewReader(body))
	if err != nil {
		return Handle{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("X-Auth-Token", c.Token)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return Handle{}, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return Handle{}, fmt.Errorf("scheduler submit: %s: %s", resp.Status, stringsTrim(data))
	}
	var parsed struct {
		ID              string `json:"id"`
		Status          string `json:"status"`
		SelectedBackend string `json:"selectedBackend"`
		ExecutionID     string `json:"executionId"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Handle{}, err
	}
	if parsed.ID == "" {
		return Handle{}, fmt.Errorf("scheduler submit returned no job id")
	}
	h := Handle{
		TaskID:        parsed.ID,
		JobID:         parsed.ID,
		ProviderJobID: parsed.ExecutionID,
		Backend:       parsed.SelectedBackend,
		Status:        parsed.Status,
		Submitted:     time.Now().UTC(),
	}
	if err := c.Save(h); err != nil {
		return Handle{}, err
	}
	return h, nil
}

// Status reads the scheduler job. If the network call fails, a persisted
// handle is used so a restarted process can still see the last status.
// Costs on the handle stay nil when the scheduler did not report them.
func (c *Client) Status(jobID string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/api/v1/orgs/"+c.OrgID+"/scheduler/jobs/"+jobID, nil)
	if err != nil {
		return "", err
	}
	if c.Token != "" {
		req.Header.Set("X-Auth-Token", c.Token)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return c.savedStatus(jobID, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return c.savedStatus(jobID, fmt.Errorf("scheduler status: %s: %s", resp.Status, stringsTrim(data)))
	}
	var parsed struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return c.savedStatus(jobID, err)
	}
	if c.Dir != "" {
		if h, err := Load(c.Dir, jobID); err == nil {
			h.Status = parsed.Status
			_ = c.Save(h)
		}
	}
	return parsed.Status, nil
}

func (c *Client) savedStatus(jobID string, cause error) (string, error) {
	if c.Dir == "" {
		return "", cause
	}
	h, err := Load(c.Dir, jobID)
	if err != nil {
		return "", cause
	}
	return h.Status, nil
}

// Bridge is the language hook. It does not store jobs of its own.
type Bridge struct {
	Client *Client
}

func (b Bridge) Submit(program string) (string, error) {
	if b.Client == nil {
		return "", fmt.Errorf("task bridge has no scheduler client")
	}
	h, err := b.Client.Submit(program)
	if err != nil {
		return "", err
	}
	return h.JobID, nil
}

func (b Bridge) Status(id string) (string, error) {
	if b.Client == nil {
		return "", fmt.Errorf("task bridge has no scheduler client")
	}
	return b.Client.Status(id)
}

func (b Bridge) Cancel(id string) error {
	if b.Client == nil {
		return fmt.Errorf("task bridge has no scheduler client")
	}
	_, err := b.Client.Cancel(id)
	return err
}

// Cancel asks the scheduler to cancel. The provider may report that cancel
// is unsupported; that response is returned unchanged.
func (c *Client) Cancel(jobID string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/v1/orgs/"+c.OrgID+"/scheduler/jobs/"+jobID+"/cancel", nil)
	if err != nil {
		return "", err
	}
	if c.Token != "" {
		req.Header.Set("X-Auth-Token", c.Token)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("scheduler cancel: %s: %s", resp.Status, stringsTrim(data))
	}
	return stringsTrim(data), nil
}

// Save writes the handle so another process can await the same job.
func (c *Client) Save(h Handle) error {
	if c.Dir == "" {
		return nil
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.Dir, h.TaskID+".json"), data, 0o644)
}

// Load reads a handle written by Save.
func Load(dir, taskID string) (Handle, error) {
	data, err := os.ReadFile(filepath.Join(dir, taskID+".json"))
	if err != nil {
		return Handle{}, err
	}
	var h Handle
	err = json.Unmarshal(data, &h)
	return h, err
}

func stringsTrim(b []byte) string {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return string(b)
}
