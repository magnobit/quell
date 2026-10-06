// Copyright 2026 Magnobit, Inc. All rights reserved.

package qtask

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/check"
	"github.com/magnobit/quell/internal/parser"
)

func TestSubmitPersistsSchedulerID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Token") != "tok" {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/v1/orgs/org/scheduler/jobs" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "job-1", "status": "queued"})
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Token: "tok", OrgID: "org", Dir: t.TempDir()}
	h, err := c.Submit("H 0\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	if h.JobID != "job-1" || h.EstimatedCost != nil {
		t.Fatalf("%+v", h)
	}
	loaded, err := Load(c.Dir, h.TaskID)
	if err != nil || loaded.JobID != "job-1" {
		t.Fatal(loaded, err)
	}
}

func TestCancelUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte("provider cancel is unsupported"))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, OrgID: "org"}
	if _, err := c.Cancel("job-1"); err == nil {
		t.Fatal("unsupported cancel must surface the scheduler response")
	}
}

func TestHandleSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/orgs/org/scheduler/jobs" {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "job-9", "status": "queued"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	first := &Client{BaseURL: srv.URL, Token: "tok", OrgID: "org", Dir: dir}
	h, err := first.Submit("H 0\nMEASURE\n")
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Client{BaseURL: srv.URL, Token: "tok", OrgID: "org", Dir: dir}
	status, err := restarted.Status(h.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("status %q", status)
	}
}

func TestAllRaceTimeoutAndPartialFailure(t *testing.T) {
	t.Run("partial", func(t *testing.T) {
		c, hook := scriptedScheduler(t, map[string]string{"H 0": "failed", "X 0": "succeeded"})
		prev := check.TaskHook
		check.TaskHook = hook
		t.Cleanup(func() { check.TaskHook = prev })
		got := evalTask(t, c, `fn both() -> string {
  let a: task = async("H 0\nMEASURE\n")
  let b: task = async("X 0\nMEASURE\n")
  return all(a, b)
}`, "both()")
		if got != "job-h=failed,job-x=succeeded" {
			t.Fatalf("all %q", got)
		}
	})
	t.Run("race", func(t *testing.T) {
		_, hook := scriptedScheduler(t, map[string]string{"H 0": "queued", "X 0": "succeeded"})
		prev := check.TaskHook
		check.TaskHook = hook
		t.Cleanup(func() { check.TaskHook = prev })
		got := evalTask(t, nil, `fn first() -> task {
  let a: task = async("H 0\nMEASURE\n")
  let b: task = async("X 0\nMEASURE\n")
  return race(a, b)
}`, "first()")
		if got != "job-x" {
			t.Fatalf("race %q", got)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		canceled := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/scheduler/jobs"):
				_ = json.NewEncoder(w).Encode(map[string]string{"id": "job-1", "status": "queued"})
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel"):
				canceled = true
				_, _ = w.Write([]byte("cancelled"))
			default:
				_ = json.NewEncoder(w).Encode(map[string]string{"id": "job-1", "status": "queued"})
			}
		}))
		t.Cleanup(srv.Close)
		client := &Client{BaseURL: srv.URL, Token: "tok", OrgID: "org", Dir: t.TempDir()}
		prev := check.TaskHook
		check.TaskHook = Bridge{Client: client}
		t.Cleanup(func() { check.TaskHook = prev })
		got := evalTask(t, nil, `fn limit() -> task {
  let a: task = async("H 0\nMEASURE\n")
  return timeout(a, 1)
}`, "limit()")
		if got != "job-1" || !canceled {
			t.Fatalf("timeout %q canceled %v", got, canceled)
		}
	})
}

func scriptedScheduler(t *testing.T, byProgram map[string]string) (*Client, Bridge) {
	t.Helper()
	jobs := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Token") != "tok" {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/orgs/org/scheduler/jobs" {
			var body struct {
				Program string `json:"program"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			id := "job-h"
			status := "queued"
			if strings.Contains(body.Program, "X 0") {
				id = "job-x"
				status = byProgram["X 0"]
			} else if strings.Contains(body.Program, "H 0") {
				id = "job-h"
				status = byProgram["H 0"]
			}
			jobs[id] = status
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "status": status})
			return
		}
		if r.Method == http.MethodGet {
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			status, ok := jobs[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "status": status})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	c := &Client{BaseURL: srv.URL, Token: "tok", OrgID: "org", Dir: t.TempDir()}
	return c, Bridge{Client: c}
}

func evalTask(t *testing.T, _ *Client, src, expr string) string {
	t.Helper()
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if ds := check.Check(c); len(ds) > 0 {
		t.Fatal(ds)
	}
	e, err := parser.ParseExpr(expr, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, ds := check.EvalIn(c, e)
	if len(ds) > 0 {
		t.Fatal(ds)
	}
	return v.Str
}
