package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"gopkg.in/yaml.v3"
)

type specDoc struct {
	OpenAPI string `yaml:"openapi"`
	Info    struct {
		Version string `yaml:"version"`
	} `yaml:"info"`
	Servers []any                     `yaml:"servers"`
	Paths   map[string]map[string]any `yaml:"paths"`
}

func parseSpec(t *testing.T, spec []byte) specDoc {
	t.Helper()

	var doc specDoc
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		t.Fatalf("spec is not valid YAML: %v", err)
	}
	return doc
}

func TestBuildSpec(t *testing.T) {
	spec, err := buildSpec()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	doc := parseSpec(t, spec)

	if doc.OpenAPI == "" {
		t.Error("expected an openapi version field")
	}
	if doc.Info.Version != httpapi.Version {
		t.Errorf("expected info.version %q, got %q", httpapi.Version, doc.Info.Version)
	}
	if len(doc.Servers) != 0 {
		t.Errorf("expected no servers in the committed spec, got %v", doc.Servers)
	}
	for _, want := range []struct{ path, method string }{
		{"/auth/usos/start", "get"},
		{"/auth/usos/callback", "get"},
		{"/auth/exchange", "post"},
	} {
		if _, ok := doc.Paths[want.path][want.method]; !ok {
			t.Errorf("%s %s is missing from the spec", want.method, want.path)
		}
	}
}

func TestBuildSpec_IsDeterministic(t *testing.T) {
	// CI diffs the regenerated spec against the committed one, so two runs must match byte for byte.
	first, err := buildSpec()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	for range 5 {
		next, err := buildSpec()
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !bytes.Equal(first, next) {
			t.Fatal("expected buildSpec to produce identical output on every run")
		}
	}
}

func TestRun(t *testing.T) {
	prev := httpapi.Version
	t.Cleanup(func() { httpapi.Version = prev })

	t.Run("writes the spec to stdout", func(t *testing.T) {
		var stdout bytes.Buffer
		if err := run(nil, &stdout); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if len(parseSpec(t, stdout.Bytes()).Paths) == 0 {
			t.Error("expected paths in the spec written to stdout")
		}
	})

	t.Run("-o writes the spec to a file", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "openapi.yaml")

		var stdout bytes.Buffer
		if err := run([]string{"-o", out}, &stdout); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if stdout.Len() != 0 {
			t.Errorf("expected nothing on stdout with -o, got %q", stdout.String())
		}
		spec, err := os.ReadFile(out)
		if err != nil {
			t.Fatalf("expected the spec file to exist: %v", err)
		}
		if len(parseSpec(t, spec).Paths) == 0 {
			t.Error("expected paths in the spec file")
		}
	})

	t.Run("-version sets info.version", func(t *testing.T) {
		var stdout bytes.Buffer
		if err := run([]string{"-version", "abc1234"}, &stdout); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if got := parseSpec(t, stdout.Bytes()).Info.Version; got != "abc1234" {
			t.Errorf("expected info.version %q, got %q", "abc1234", got)
		}
	})

	t.Run("rejects unknown flags", func(t *testing.T) {
		if err := run([]string{"-nope"}, &bytes.Buffer{}); err == nil {
			t.Error("expected an error for an unknown flag, got nil")
		}
	})
}
