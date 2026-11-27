package measure

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func encodeTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewGray(image.Rect(0, 0, width, height))
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode(%dx%d): %v", width, height, err)
	}
	return buf.Bytes()
}

func writeTestOGFile(t *testing.T, dir, relPath string, data []byte) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func padToSize(t *testing.T, base []byte, size int) []byte {
	t.Helper()
	if len(base) >= size {
		t.Fatalf("base PNG (%d bytes) already >= target %d", len(base), size)
	}
	out := make([]byte, size)
	copy(out, base)
	return out
}

func TestCheckSocialOGAbsent(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	if results := CheckSocialOG(root, "pages/og.png"); len(results) != 0 {
		t.Fatalf("CheckSocialOG on absent file = %+v, want empty", results)
	}
	if results := CheckSocialOG(nil, "pages/og.png"); len(results) != 0 {
		t.Fatalf("CheckSocialOG on nil root = %+v, want empty", results)
	}
}

func TestCheckSocialOGValidAndInvalidFiles(t *testing.T) {
	valid1280 := encodeTestPNG(t, 1280, 640)
	valid1200 := encodeTestPNG(t, 1200, 630)
	wrongDims := encodeTestPNG(t, 800, 600)

	tests := []struct {
		name       string
		outputPath string
		filePath   string
		data       []byte
		wantStatus HealthStatus
		wantPath   string
		wantDetail string
	}{
		{
			name:       "valid 1280x640 default path",
			outputPath: "",
			filePath:   "pages/og.png",
			data:       valid1280,
			wantStatus: Present,
			wantPath:   "pages/og.png",
		},
		{
			name:       "valid 1200x630 bespoke card",
			outputPath: "pages/og.png",
			filePath:   "pages/og.png",
			data:       valid1200,
			wantStatus: Present,
			wantPath:   "pages/og.png",
		},
		{
			name:       "valid 1280x640 one byte below 1 MiB",
			outputPath: "pages/og.png",
			filePath:   "pages/og.png",
			data:       padToSize(t, valid1280, maxSocialOGSize-1),
			wantStatus: Present,
			wantPath:   "pages/og.png",
		},
		{
			name:       "valid custom output path",
			outputPath: "assets/social-card.png",
			filePath:   "assets/social-card.png",
			data:       valid1280,
			wantStatus: Present,
			wantPath:   "assets/social-card.png",
		},
		{
			name:       "wrong dimensions warns",
			outputPath: "pages/og.png",
			filePath:   "pages/og.png",
			data:       wrongDims,
			wantStatus: Warning,
			wantPath:   "pages/og.png",
			wantDetail: "800x600",
		},
		{
			name:       "non-PNG file warns",
			outputPath: "pages/og.png",
			filePath:   "pages/og.png",
			data:       []byte("not a png image at all"),
			wantStatus: Warning,
			wantPath:   "pages/og.png",
			wantDetail: "invalid PNG",
		},
		{
			name:       "exact 1 MiB boundary warns",
			outputPath: "pages/og.png",
			filePath:   "pages/og.png",
			data:       padToSize(t, valid1280, maxSocialOGSize),
			wantStatus: Warning,
			wantPath:   "pages/og.png",
			wantDetail: "exceeds 1 MB",
		},
		{
			name:       "over 1 MiB warns",
			outputPath: "pages/og.png",
			filePath:   "pages/og.png",
			data:       padToSize(t, valid1280, maxSocialOGSize+1024),
			wantStatus: Warning,
			wantPath:   "pages/og.png",
			wantDetail: "exceeds 1 MB",
		},
		{
			name:       "wrong dimensions and over 1 MiB reports both",
			outputPath: "pages/og.png",
			filePath:   "pages/og.png",
			data:       padToSize(t, wrongDims, maxSocialOGSize+10),
			wantStatus: Warning,
			wantPath:   "pages/og.png",
			wantDetail: "800x600",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestOGFile(t, dir, tt.filePath, tt.data)
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()

			results := CheckSocialOG(root, tt.outputPath)
			if len(results) != 1 {
				t.Fatalf("CheckSocialOG() returned %d results (%+v), want 1", len(results), results)
			}
			got := results[0]
			if got.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", got.Path, tt.wantPath)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q (detail=%q)", got.Status, tt.wantStatus, got.Detail)
			}
			if tt.wantDetail != "" && !strings.Contains(got.Detail, tt.wantDetail) {
				t.Errorf("Detail = %q, want substring %q", got.Detail, tt.wantDetail)
			}
			if tt.wantStatus == Present && got.Detail != "" {
				t.Errorf("Present result should have empty Detail, got %q", got.Detail)
			}
		})
	}
}

func TestCheckSocialOGRejectsSymlinksAndDirectories(t *testing.T) {
	t.Run("directory at output path warns", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "pages", "og.png"), 0o755); err != nil {
			t.Fatal(err)
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()

		results := CheckSocialOG(root, "pages/og.png")
		if len(results) != 1 || results[0].Status != Warning {
			t.Fatalf("CheckSocialOG(dir) = %+v, want 1 Warning", results)
		}
	})

	t.Run("symlink at output path warns", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks require elevated privileges on Windows")
		}
		dir := t.TempDir()
		realPNG := filepath.Join(dir, "real.png")
		if err := os.WriteFile(realPNG, encodeTestPNG(t, 1280, 640), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realPNG, filepath.Join(dir, "pages", "og.png")); err != nil {
			t.Skipf("symlink not supported: %v", err)
		}

		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()

		results := CheckSocialOG(root, "pages/og.png")
		if len(results) != 1 || results[0].Status != Warning || !strings.Contains(results[0].Detail, "symlink") {
			t.Fatalf("CheckSocialOG(symlink) = %+v, want symlink Warning", results)
		}
	})

	t.Run("symlink parent directory warns", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks require elevated privileges on Windows")
		}
		dir := t.TempDir()
		writeTestOGFile(t, dir, "realpages/og.png", encodeTestPNG(t, 1280, 640))
		if err := os.Symlink(filepath.Join(dir, "realpages"), filepath.Join(dir, "pages")); err != nil {
			t.Skipf("symlink not supported: %v", err)
		}

		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()

		results := CheckSocialOG(root, "pages/og.png")
		if len(results) != 1 || results[0].Status != Warning || !strings.Contains(results[0].Detail, "symlink") {
			t.Fatalf("CheckSocialOG(parent symlink) = %+v, want symlink Warning", results)
		}
	})
}
