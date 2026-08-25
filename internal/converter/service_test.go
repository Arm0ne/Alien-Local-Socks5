package converter

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type checkerFunc func(context.Context, string, string, []Node) error

func (function checkerFunc) Check(ctx context.Context, xrayPath, configPath string, nodes []Node) error {
	return function(ctx, xrayPath, configPath, nodes)
}

func TestConvertFileReplacesOutputOnlyAfterSuccessfulCheck(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "nodes.txt")
	outputPath := filepath.Join(directory, "config.json")
	xrayPath := filepath.Join(directory, "xray.exe")
	mustWrite(t, inputPath, []byte(testLink(1, nil)))
	mustWrite(t, outputPath, []byte("old-valid-config"))
	mustWrite(t, xrayPath, []byte("test placeholder"))

	checker := checkerFunc(func(_ context.Context, actualXrayPath, configPath string, nodes []Node) error {
		if actualXrayPath != xrayPath {
			t.Fatalf("xray path = %s", actualXrayPath)
		}
		data, err := os.ReadFile(configPath)
		if err != nil {
			return err
		}
		var parsed map[string]any
		return json.Unmarshal(data, &parsed)
	})

	result, err := ConvertFile(context.Background(), ConvertOptions{
		InputPath: inputPath, OutputPath: outputPath, StartPort: DefaultStartPort,
	}, checker)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 1 {
		t.Fatalf("node count = %d", len(result.Nodes))
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) || strings.Contains(string(data), "old-valid-config") {
		t.Fatalf("output was not replaced with JSON: %s", data)
	}
}

func TestConvertFilePreservesExistingOutputOnFailedCheck(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "nodes.txt")
	outputPath := filepath.Join(directory, "config.json")
	xrayPath := filepath.Join(directory, "xray.exe")
	mustWrite(t, inputPath, []byte(testLink(1, nil)))
	mustWrite(t, outputPath, []byte("old-valid-config"))
	mustWrite(t, xrayPath, []byte("test placeholder"))

	_, err := ConvertFile(context.Background(), ConvertOptions{
		InputPath: inputPath, OutputPath: outputPath, StartPort: DefaultStartPort,
	}, checkerFunc(func(context.Context, string, string, []Node) error {
		return errors.New("xray rejected config")
	}))
	if err == nil {
		t.Fatal("expected check failure")
	}
	data, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "old-valid-config" {
		t.Fatalf("existing output changed: %q", data)
	}
	temporaryFiles, globErr := filepath.Glob(filepath.Join(directory, ".reality-config-*.json"))
	if globErr != nil || len(temporaryFiles) != 0 {
		t.Fatalf("temporary files remain: %v, %v", temporaryFiles, globErr)
	}
}

func TestConvertFileRequiresAdjacentXray(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "nodes.txt")
	mustWrite(t, inputPath, []byte(testLink(1, nil)))
	_, err := ConvertFile(context.Background(), ConvertOptions{
		InputPath: inputPath, OutputPath: filepath.Join(directory, "config.json"), StartPort: DefaultStartPort,
	}, checkerFunc(func(context.Context, string, string, []Node) error { return nil }))
	if err == nil || !strings.Contains(err.Error(), "xray.exe") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRedactSensitiveValues(t *testing.T) {
	node, err := parseLink(testLink(1, nil), 1)
	if err != nil {
		t.Fatal(err)
	}
	input := node.RawLink + "\n" + node.UUID + "\n" + node.PublicKey
	redacted := Redact(input, []Node{node})
	for _, secret := range []string{node.RawLink, node.UUID, node.PublicKey} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redacted output contains secret %q", secret)
		}
	}
}

func TestGeneratedConfigurationsWithXray(t *testing.T) {
	xrayPath := os.Getenv("XRAY_TEST_EXE")
	if xrayPath == "" {
		t.Skip("XRAY_TEST_EXE is not set")
	}
	for _, count := range []int{1, 5, 8, 12} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			var links []string
			for index := 1; index <= count; index++ {
				links = append(links, testLink(index, nil))
			}
			result, err := ParseText(strings.Join(links, "\n"), DefaultStartPort)
			if err != nil {
				t.Fatal(err)
			}
			data, err := BuildConfig(result)
			if err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(t.TempDir(), "config.json")
			mustWrite(t, configPath, data)
			if err := (ProcessChecker{}).Check(context.Background(), xrayPath, configPath, result.Nodes); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
