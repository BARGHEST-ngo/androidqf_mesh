// Copyright (c) 2021-2023 Claudio Guarnieri.
// Use of this source code is governed by the MVT License 1.1
// which can be found in the LICENSE file.

package modules

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/botherder/go-savetime/text"
	"github.com/google/uuid"
)

type pullToWriter interface {
	PullToWriter(remotePath string, writer io.Writer) error
}

func relativeDeviceChild(deviceRoot, devicePath string) (string, error) {
	if deviceRoot == "" {
		return "", fmt.Errorf("device root cannot be empty")
	}
	if strings.ContainsRune(devicePath, 0) {
		return "", fmt.Errorf("unsafe device path %q", devicePath)
	}

	root := path.Clean(deviceRoot)
	child := path.Clean(devicePath)
	if child == root {
		return "", fmt.Errorf("device path %q is the root path %q", devicePath, deviceRoot)
	}

	rootPrefix := root
	if !strings.HasSuffix(rootPrefix, "/") {
		rootPrefix += "/"
	}
	if !strings.HasPrefix(child, rootPrefix) {
		return "", fmt.Errorf("device path %q is outside %q", devicePath, deviceRoot)
	}

	rel := strings.TrimPrefix(child, rootPrefix)
	localRel := filepath.FromSlash(rel)
	if !filepath.IsLocal(localRel) {
		return "", fmt.Errorf("unsafe device path %q", devicePath)
	}

	return rel, nil
}

func deviceAbsToLocalRel(devicePath string) (string, error) {
	if strings.ContainsRune(devicePath, 0) {
		return "", fmt.Errorf("unsafe device path %q", devicePath)
	}

	cleaned := path.Clean(devicePath)
	if !path.IsAbs(cleaned) {
		return "", fmt.Errorf("device path %q is not absolute", devicePath)
	}

	if cleaned != devicePath {
		return "", fmt.Errorf("device path %q is not canonical", devicePath)
	}

	rel := strings.TrimPrefix(cleaned, "/")
	if rel == "" {
		return "", fmt.Errorf("device path %q is the root path", devicePath)
	}

	if !filepath.IsLocal(filepath.FromSlash(rel)) {
		return "", fmt.Errorf("unsafe device path %q", devicePath)
	}

	return rel, nil
}

func safeLocalBaseName(name string) (string, error) {
	if strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("unsafe file name %q", name)
	}

	if strings.ContainsRune(name, '/') || strings.ContainsRune(name, filepath.Separator) {
		return "", fmt.Errorf("unsafe file name %q", name)
	}

	if name == "." || name == ".." {
		return "", fmt.Errorf("unsafe file name %q", name)
	}

	if name != filepath.Base(name) || !filepath.IsLocal(name) {
		return "", fmt.Errorf("unsafe file name %q", name)
	}

	return name, nil
}

func quietPullError(err error) bool {
	if err == nil {
		return true
	}

	msg := err.Error()
	return text.ContainsNoCase(msg, "Permission denied") ||
		text.ContainsNoCase(msg, "Is a directory")
}

type devicePuller interface {
	Pull(remotePath, localPath string) (string, error)
}

func pullDeviceChildToRoot(root *os.Root, puller devicePuller, rel, devicePath string) error {
	localRel := filepath.FromSlash(rel)
	if !filepath.IsLocal(localRel) {
		return fmt.Errorf("unsafe local path %q", rel)
	}

	tmpName := ".androidqf-" + uuid.NewString() + ".part"
	out, err := puller.Pull(devicePath, filepath.Join(root.Name(), tmpName))
	if err != nil {
		root.Remove(tmpName)
		if msg := strings.TrimSpace(out); msg != "" {
			return fmt.Errorf("%v: %s", err, msg)
		}
		return err
	}

	if err := root.MkdirAll(filepath.Dir(localRel), 0o755); err != nil {
		root.Remove(tmpName)
		return fmt.Errorf("failed to create destination folders for %q: %v", rel, err)
	}

	if err := root.Rename(tmpName, localRel); err != nil {
		root.Remove(tmpName)
		return fmt.Errorf("failed to move %q into place: %v", rel, err)
	}

	return nil
}

func createRootFile(root *os.Root, rel string) (*os.File, error) {
	localRel := filepath.FromSlash(rel)
	if !filepath.IsLocal(localRel) {
		return nil, fmt.Errorf("unsafe local path %q", rel)
	}

	if err := root.MkdirAll(filepath.Dir(localRel), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create destination folders for %q: %v", rel, err)
	}

	file, err := root.OpenFile(localRel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination file %q: %v", rel, err)
	}

	return file, nil
}

func streamDeviceChildToRoot(root *os.Root, puller pullToWriter, rel, devicePath string) error {
	file, err := createRootFile(root, rel)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := puller.PullToWriter(devicePath, file); err != nil {
		return err
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination file %q: %v", rel, err)
	}

	return nil
}
