// Copyright 2026 Magnobit, Inc. All rights reserved.

package check

import (
	"strings"
	"testing"

	"github.com/magnobit/quell/internal/parser"
)

type fakeTasks struct {
	status map[string]string
	cancel []string
}

func (f *fakeTasks) Submit(program string) (string, error) {
	if strings.TrimSpace(program) == "" {
		return "", errEmpty{}
	}
	id := "job-1"
	if len(f.status) > 0 {
		id = "job-2"
	}
	if f.status == nil {
		f.status = map[string]string{}
	}
	if _, ok := f.status[id]; ok {
		id = "job-2"
	}
	f.status[id] = "queued"
	return id, nil
}

func (f *fakeTasks) Status(id string) (string, error) {
	s, ok := f.status[id]
	if !ok {
		return "", errEmpty{}
	}
	return s, nil
}

func (f *fakeTasks) Cancel(id string) error {
	f.cancel = append(f.cancel, id)
	f.status[id] = "cancelled"
	return nil
}

type errEmpty struct{}

func (errEmpty) Error() string { return "empty program" }

func TestAsyncAwaitCancel(t *testing.T) {
	prev := TaskHook
	fake := &fakeTasks{}
	TaskHook = fake
	t.Cleanup(func() { TaskHook = prev })

	src := strings.Join([]string{
		`fn status() -> string {`,
		`    let t: task = async("H 0")`,
		`    return await(t)`,
		`}`,
		`fn stop() -> string {`,
		`    let t: task = async("H 0")`,
		`    return cancel(t)`,
		`}`,
		``,
	}, "\n")
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if ds := Check(c); len(ds) > 0 {
		t.Fatal(ds)
	}
	e, err := parser.ParseExpr("status()", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, ds := EvalIn(c, e)
	if len(ds) > 0 {
		t.Fatal(ds)
	}
	if v.Str != "queued" {
		t.Fatalf("status %q", v.Str)
	}
	fake.status["job-1"] = "running"
	cancelExpr, err := parser.ParseExpr("stop()", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, ds := EvalIn(c, cancelExpr); len(ds) > 0 {
		t.Fatal(ds)
	}
	if len(fake.cancel) != 1 || fake.status[fake.cancel[0]] != "cancelled" {
		t.Fatalf("%v %v", fake.cancel, fake.status)
	}
}

func TestAsyncWithoutBridge(t *testing.T) {
	prev := TaskHook
	TaskHook = nil
	t.Cleanup(func() { TaskHook = prev })
	src := "let t: task = async(\"H 0\")\n"
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if ds := Check(c); len(ds) == 0 {
		t.Fatal("missing bridge must be diagnosed")
	}
}

func TestZeroNineNamesStayValid(t *testing.T) {
	src := strings.Join([]string{
		"let all: int = 1",
		"let task: int = 2",
		"fn await() -> int {",
		"    return all + task",
		"}",
		"fn race(timeout: int) -> int {",
		"    let cancel: int = timeout",
		"    return cancel",
		"}",
		"",
	}, "\n")
	c, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if ds := Check(c); len(ds) > 0 {
		t.Fatal(ds)
	}
	e, err := parser.ParseExpr("await()", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, ds := EvalIn(c, e)
	if len(ds) > 0 {
		t.Fatal(ds)
	}
	if v.Int != 3 {
		t.Fatalf("user fn await returned %d", v.Int)
	}
}
