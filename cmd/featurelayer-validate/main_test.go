package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestValidFile(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"../../testdata/config.json"}, nil, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "ok — 4 features, 1 segments, 2 flags, 2 plans, 1 addons") || errb.Len() != 0 {
		t.Errorf("stdout=%q stderr=%q", out.String(), errb.String())
	}
	out.Reset()
	if code := run([]string{"-q", "../../testdata/config.json"}, nil, &out, &errb); code != 0 || out.Len() != 0 {
		t.Errorf("-q: exit %d stdout=%q", code, out.String())
	}
}

func TestInvalidFilePrintsEveryProblem(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"../../testdata/invalid.json"}, nil, &out, &errb); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	for _, want := range []string{
		"features[1].key",
		"flags[0].feature",
		"plans[0].extends",
		"limit.period",
		"limit.per",
		"invalid — 5 problem(s)",
	} {
		if !strings.Contains(errb.String(), want) {
			t.Errorf("stderr missing %q:\n%s", want, errb.String())
		}
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty on failure: %q", out.String())
	}
}

func TestStdin(t *testing.T) {
	var out, errb bytes.Buffer
	in := strings.NewReader(`{"features":[{"key":"a","lifecycle":"ga"}]}`)
	if code := run(nil, in, &out, &errb); code != 0 || !strings.HasPrefix(out.String(), "stdin: ok") {
		t.Errorf("stdin: exit %d stdout=%q stderr=%q", code, out.String(), errb.String())
	}
	out.Reset()
	in = strings.NewReader(`{"features":[{"key":"a","lifecycle":"ga"}], "typo": 1}`)
	if code := run([]string{"-"}, in, &out, &errb); code != 1 || !strings.Contains(errb.String(), "typo") {
		t.Errorf("unknown field: exit %d stderr=%q", code, errb.String())
	}
}

func TestUsageErrors(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"a.json", "b.json"}, nil, &out, &errb); code != 2 {
		t.Errorf("two paths: exit %d", code)
	}
	if code := run([]string{"-nope"}, nil, &out, &errb); code != 2 {
		t.Errorf("unknown flag: exit %d", code)
	}
	if code := run([]string{"does-not-exist.json"}, nil, &out, &errb); code != 1 || !strings.Contains(errb.String(), "does-not-exist.json") {
		t.Errorf("missing file: exit %d stderr=%q", code, errb.String())
	}
}
