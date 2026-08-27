// Copyright (c) 2021-2023 Claudio Guarnieri.
// Use of this source code is governed by the MVT License 1.1
// which can be found in the LICENSE file.

package modules

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/BARGHEST-ngo/androidqf_mesh/acquisition"
	"github.com/BARGHEST-ngo/androidqf_mesh/adb"
	"github.com/BARGHEST-ngo/androidqf_mesh/log"
	"github.com/botherder/go-savetime/text"
)

type Logs struct {
	StoragePath string
	LogsPath    string
}

func NewLogs() *Logs {
	return &Logs{}
}

func (l *Logs) Name() string {
	return "logs"
}

func (l *Logs) InitStorage(storagePath string) error {
	l.StoragePath = storagePath
	l.LogsPath = filepath.Join(storagePath, "logs")

	// Only create directory in traditional mode
	if storagePath != "" {
		err := os.Mkdir(l.LogsPath, 0o755)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("failed to create logs folder: %v", err)
		}
	}

	return nil
}

func appendListedFiles(dst []string, folder string, files []string) []string {
	root := strings.TrimRight(folder, "/")

	for _, file := range files {
		entry := strings.TrimRight(file, "/")
		if entry == "" || entry == root {
			continue
		}
		dst = append(dst, entry)
	}

	return dst
}

func (l *Logs) streamToArchive(acq *acquisition.Acquisition, devicePath, zipPath string) error {
	writer, err := acq.EncryptedWriter.CreateFile(zipPath)
	if err != nil {
		return fmt.Errorf("failed to create zip entry: %v", err)
	}

	if err := acq.StreamingPuller.PullToWriter(devicePath, writer); err != nil {
		return err
	}

	log.Debugf("Streamed log file %s to encrypted archive as %s", devicePath, zipPath)
	return nil
}

func (l *Logs) Run(acq *acquisition.Acquisition, fast bool) error {
	log.Info("Collecting system logs...")

	streaming := acq.StreamingMode && acq.EncryptedWriter != nil
	var localRoot *os.Root
	if !streaming {
		var err error
		localRoot, err = os.OpenRoot(l.LogsPath)
		if err != nil {
			return fmt.Errorf("failed to open logs output root: %v", err)
		}
		defer localRoot.Close()
	}

	staticLogFiles := []string{
		"/data/system/uiderrors.txt",
		"/proc/kmsg",
		"/proc/last_kmsg",
		"/sys/fs/pstore/console-ramoops",
	}

	// FIXME: needed to list files versus pulling folders?
	var deviceLogFiles []string
	for _, logFolder := range []string{"/data/anr/", "/data/log/", "/sdcard/log/"} {
		files, err := adb.Client.ListFiles(logFolder, true)
		if err != nil {
			log.Debugf("Impossible to get files from %s", logFolder)
			continue
		}
		if len(files) == 0 {
			continue
		}

		deviceLogFiles = appendListedFiles(deviceLogFiles, logFolder, files)
		log.Debugf("Files in %s: %s", logFolder, files)
	}

	for _, logFile := range staticLogFiles {
		rel := strings.TrimPrefix(logFile, "/")
		log.Debugf("From: %s", logFile)

		if streaming {
			if err := l.streamToArchive(acq, logFile, path.Join("logs", rel)); err != nil {
				if !quietPullError(err) {
					log.Errorf("Failed to stream log file %s: %v\n", logFile, err)
				}
			}
			continue
		}

		localPath := filepath.Join(l.LogsPath, filepath.FromSlash(rel))
		log.Debugf("To: %s", localPath)

		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			log.Errorf("Failed to create folders for logs %s: %v\n", localPath, err)
			continue
		}

		out, err := adb.Client.Pull(logFile, localPath)
		if err != nil {
			if !text.ContainsNoCase(out, "Permission denied") {
				log.Errorf("Failed to pull log file %s: %s\n", logFile, strings.TrimSpace(out))
			}
			continue
		}
	}

	for _, logFile := range deviceLogFiles {
		rel, err := deviceAbsToLocalRel(logFile)
		if err != nil {
			log.Errorf("Skipping log file with unsafe path %s: %v\n", logFile, err)
			continue
		}

		log.Debugf("From: %s", logFile)

		if streaming {
			if err := l.streamToArchive(acq, logFile, path.Join("logs", rel)); err != nil {
				if !quietPullError(err) {
					log.Errorf("Failed to stream log file %s: %v\n", logFile, err)
				}
			}
			continue
		}

		log.Debugf("To: %s", filepath.Join(l.LogsPath, filepath.FromSlash(rel)))

		if err := pullDeviceChildToRoot(localRoot, adb.Client, rel, logFile); err != nil {
			if !quietPullError(err) {
				log.Errorf("Failed to pull log file %s: %v\n", logFile, err)
			}
			continue
		}
	}

	return nil
}
