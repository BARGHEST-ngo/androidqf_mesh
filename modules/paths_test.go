// Copyright (c) 2021-2023 Claudio Guarnieri.
// Use of this source code is governed by the MVT License 1.1
// which can be found in the LICENSE file.

package modules

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRelativeDeviceChild(t *testing.T) {
	tests := []struct {
		name       string
		deviceRoot string
		devicePath string
		want       string
		wantErr    bool
	}{
		{
			name:       "child with trailing slash root",
			deviceRoot: "/sdcard/Download/Intrusion Logging/",
			devicePath: "/sdcard/Download/Intrusion Logging/logs/file.txt",
			want:       "logs/file.txt",
		},
		{
			name:       "child without trailing slash root",
			deviceRoot: "/data/local/tmp",
			devicePath: "/data/local/tmp/file.txt",
			want:       "file.txt",
		},
		{
			name:       "sibling prefix rejected",
			deviceRoot: "/data/local/tmp",
			devicePath: "/data/local/tmp-evil/file.txt",
			wantErr:    true,
		},
		{
			name:       "parent traversal rejected",
			deviceRoot: "/data/local/tmp",
			devicePath: "/data/local/tmp/../../../host/path",
			wantErr:    true,
		},
		{
			name:       "cleaned child traversal rejected",
			deviceRoot: "/sdcard/Download/Intrusion Logging/",
			devicePath: "/sdcard/Download/Intrusion Logging/../Other/file.txt",
			wantErr:    true,
		},
		{
			name:       "non child rejected",
			deviceRoot: "/sdcard/Download/Intrusion Logging/",
			devicePath: "/sdcard/Download/Other/file.txt",
			wantErr:    true,
		},
		{
			name:       "root rejected",
			deviceRoot: "/data/local/tmp/",
			devicePath: "/data/local/tmp",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := relativeDeviceChild(tt.deviceRoot, tt.devicePath)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("relativeDeviceChild() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("relativeDeviceChild() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("relativeDeviceChild() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreateRootFile(t *testing.T) {
	rootDir := t.TempDir()
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	defer root.Close()

	file, err := createRootFile(root, "nested/file.txt")
	if err != nil {
		t.Fatalf("createRootFile() error = %v", err)
	}
	if _, err := file.WriteString("ok"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(rootDir, "nested", "file.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "ok" {
		t.Fatalf("created file content = %q, want %q", got, "ok")
	}

	file, err = createRootFile(root, "file.txt")
	if err != nil {
		t.Fatalf("createRootFile() root file error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() root file error = %v", err)
	}

	if file, err := createRootFile(root, "../escape"); err == nil {
		file.Close()
		t.Fatal("createRootFile() error = nil, want lexical traversal rejection")
	}
}

func TestCreateRootFileRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires extra privileges on Windows")
	}

	rootDir := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.Symlink(outsideDir, filepath.Join(rootDir, "escape")); err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skipf("symlink creation not permitted: %v", err)
		}
		t.Fatalf("Symlink() error = %v", err)
	}

	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	defer root.Close()

	if file, err := createRootFile(root, "escape/file.txt"); err == nil {
		file.Close()
		t.Fatal("createRootFile() error = nil, want symlink escape rejection")
	}
}

func TestDeviceAbsToLocalRel(t *testing.T) {
	cases := []struct {
		name       string
		devicePath string
		want       string
		wantErr    bool
	}{
		{name: "plain", devicePath: "/data/anr/trace.txt", want: "data/anr/trace.txt"},
		{name: "contains traversal", devicePath: "/data/anr/../../etc/passwd", wantErr: true},
		{name: "escapes root", devicePath: "/../../etc/passwd", wantErr: true},
		{name: "trailing slash", devicePath: "/data/anr/", wantErr: true},
		{name: "double slash", devicePath: "//data/anr/trace.txt", wantErr: true},
		{name: "relative", devicePath: "data/anr/trace.txt", wantErr: true},
		{name: "root", devicePath: "/", wantErr: true},
		{name: "nul byte", devicePath: "/data/anr/\x00trace.txt", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := deviceAbsToLocalRel(tc.devicePath)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("deviceAbsToLocalRel(%q) = %q, want error", tc.devicePath, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("deviceAbsToLocalRel(%q) error = %v", tc.devicePath, err)
			}
			if got != tc.want {
				t.Fatalf("deviceAbsToLocalRel(%q) = %q, want %q", tc.devicePath, got, tc.want)
			}
		})
	}
}

func TestSafeLocalBaseName(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "plain", input: "com.example.app.apk"},
		{name: "separator", input: "../com.example.app.apk", wantErr: true},
		{name: "nested", input: "sub/com.example.app.apk", wantErr: true},
		{name: "parent", input: "..", wantErr: true},
		{name: "current", input: ".", wantErr: true},
		{name: "empty", input: "", wantErr: true},
		{name: "nul byte", input: "com.example\x00.apk", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := safeLocalBaseName(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("safeLocalBaseName(%q) = %q, want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("safeLocalBaseName(%q) error = %v", tc.input, err)
			}
			if got != tc.input {
				t.Fatalf("safeLocalBaseName(%q) = %q, want unchanged", tc.input, got)
			}
		})
	}
}

type fakePuller struct {
	pulled []string
}

func (f *fakePuller) PullToWriter(remotePath string, writer io.Writer) error {
	f.pulled = append(f.pulled, remotePath)
	_, err := io.WriteString(writer, "pulled:"+remotePath)
	return err
}

func TestLogPullStaysInsideRootWhenComponentIsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires extra privileges on Windows")
	}

	rootDir := t.TempDir()
	outsideDir := t.TempDir()

	if err := os.Symlink(outsideDir, filepath.Join(rootDir, "data")); err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skipf("symlink creation not permitted: %v", err)
		}
		t.Fatalf("Symlink() error = %v", err)
	}

	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	defer root.Close()

	rel, err := deviceAbsToLocalRel("/data/anr/trace.txt")
	if err != nil {
		t.Fatalf("deviceAbsToLocalRel() error = %v", err)
	}

	puller := &fakePuller{}
	if err := streamDeviceChildToRoot(root, puller, rel, "/data/anr/trace.txt"); err == nil {
		t.Fatal("streamDeviceChildToRoot() error = nil, want symlink escape rejection")
	}

	entries, err := os.ReadDir(outsideDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("wrote %d entries outside the root, want 0", len(entries))
	}
	if len(puller.pulled) != 0 {
		t.Fatalf("pulled %v, want no pull attempted", puller.pulled)
	}
}

func TestLogPullRejectsNonCanonicalPath(t *testing.T) {
	rootDir := t.TempDir()
	outsideDir := t.TempDir()

	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	defer root.Close()

	devicePath := "/data/anr/../../../../../../" + filepath.Base(outsideDir) + "/escaped.txt"
	if _, err := deviceAbsToLocalRel(devicePath); err == nil {
		t.Fatalf("deviceAbsToLocalRel(%q) error = nil, want rejection", devicePath)
	}

	rel, err := deviceAbsToLocalRel("/data/anr/trace.txt")
	if err != nil {
		t.Fatalf("deviceAbsToLocalRel() error = %v", err)
	}

	if err := streamDeviceChildToRoot(root, &fakePuller{}, rel, "/data/anr/trace.txt"); err != nil {
		t.Fatalf("streamDeviceChildToRoot() error = %v", err)
	}

	if entries, err := os.ReadDir(outsideDir); err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	} else if len(entries) != 0 {
		t.Fatalf("wrote %d entries outside the root, want 0", len(entries))
	}

	if _, err := os.Stat(filepath.Join(rootDir, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("expected the pull to land inside the root: %v", err)
	}
}

func TestPackageCopyNameRejectsTraversal(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	defer root.Close()

	p := &Packages{}
	for _, packageName := range []string{"../../evil", "sub/evil", "/etc/evil", "evil\x00"} {
		if name, err := p.getLocalCopyName(root, packageName, "/data/app/base.apk"); err == nil {
			t.Fatalf("getLocalCopyName(%q) = %q, want error", packageName, name)
		}
	}

	name, err := p.getLocalCopyName(root, "com.example.app", "/data/app/base.apk")
	if err != nil {
		t.Fatalf("getLocalCopyName() error = %v", err)
	}
	if name != "com.example.app.apk" {
		t.Fatalf("getLocalCopyName() = %q, want com.example.app.apk", name)
	}
}

func TestAppendListedFiles(t *testing.T) {
	got := appendListedFiles(nil, "/data/anr/", []string{
		"/data/anr/",
		"/data/anr",
		"/data/anr/trace.txt",
		"/data/anr/sub/",
		"",
	})

	want := []string{"/data/anr/trace.txt", "/data/anr/sub"}
	if len(got) != len(want) {
		t.Fatalf("appendListedFiles() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("appendListedFiles() = %v, want %v", got, want)
		}
	}
}

func TestQuietPullErrorKeepsResolutionFailuresLoud(t *testing.T) {
	if !quietPullError(errors.New("adb: Permission denied")) {
		t.Fatal("quietPullError(Permission denied) = false, want true")
	}
	if !quietPullError(errors.New("cat: /data/anr: Is a directory")) {
		t.Fatal("quietPullError(Is a directory) = false, want true")
	}
	if quietPullError(errors.New(`failed to create destination file "escape/x": no such file or directory`)) {
		t.Fatal("quietPullError(destination refused) = true, want false")
	}
}

type fakeAdb struct {
	pulled   [][2]string
	failWith string
}

func (f *fakeAdb) Pull(remotePath, localPath string) (string, error) {
	f.pulled = append(f.pulled, [2]string{remotePath, localPath})
	if f.failWith != "" {
		return f.failWith, errors.New("exit status 1")
	}
	return "", os.WriteFile(localPath, []byte("pulled:"+remotePath), 0o644)
}

func leftoverParts(t *testing.T, dir string) int {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}

	count := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".androidqf-") {
			count++
		}
	}
	return count
}

func TestPullDeviceChildToRootWritesInsideRoot(t *testing.T) {
	rootDir := t.TempDir()
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	defer root.Close()

	puller := &fakeAdb{}
	if err := pullDeviceChildToRoot(root, puller, "data/anr/trace.txt", "/data/anr/trace.txt"); err != nil {
		t.Fatalf("pullDeviceChildToRoot() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(rootDir, "data", "anr", "trace.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "pulled:/data/anr/trace.txt" {
		t.Fatalf("content = %q, want the pulled bytes", got)
	}

	if local := puller.pulled[0][1]; !strings.HasPrefix(filepath.Base(local), ".androidqf-") {
		t.Fatalf("adb pull destination = %q, want a generated temp name", local)
	}
	if n := leftoverParts(t, rootDir); n != 0 {
		t.Fatalf("%d temp files left behind, want 0", n)
	}
}

func TestPullDeviceChildToRootRejectsSymlinkComponent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires extra privileges on Windows")
	}

	rootDir := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.Symlink(outsideDir, filepath.Join(rootDir, "data")); err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skipf("symlink creation not permitted: %v", err)
		}
		t.Fatalf("Symlink() error = %v", err)
	}

	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	defer root.Close()

	if err := pullDeviceChildToRoot(root, &fakeAdb{}, "data/anr/trace.txt", "/data/anr/trace.txt"); err == nil {
		t.Fatal("pullDeviceChildToRoot() error = nil, want symlink escape rejection")
	}

	entries, err := os.ReadDir(outsideDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("wrote %d entries outside the root, want 0", len(entries))
	}
	if n := leftoverParts(t, rootDir); n != 0 {
		t.Fatalf("%d temp files left behind, want 0", n)
	}
}

func TestPullDeviceChildToRootSurfacesAdbOutput(t *testing.T) {
	rootDir := t.TempDir()
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	defer root.Close()

	puller := &fakeAdb{failWith: "adb: error: remote open failed: Permission denied"}
	err = pullDeviceChildToRoot(root, puller, "data/anr/trace.txt", "/data/anr/trace.txt")
	if err == nil {
		t.Fatal("pullDeviceChildToRoot() error = nil, want failure")
	}
	if !quietPullError(err) {
		t.Fatalf("quietPullError(%v) = false, want the adb message to be recognised", err)
	}
	if _, statErr := os.Stat(filepath.Join(rootDir, "data")); statErr == nil {
		t.Fatal("a failed pull left a destination behind")
	}
	if n := leftoverParts(t, rootDir); n != 0 {
		t.Fatalf("%d temp files left behind, want 0", n)
	}
}
