// androidqf - Android Quick Forensics
// Copyright (c) 2021-2022 Claudio Guarnieri.
// Use of this software is governed by the MVT License 1.1 that can be found at
//   https://license.mvt.re/1.1/

package adb

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/BARGHEST-ngo/androidqf_mesh/log"
	saveSlice "github.com/botherder/go-savetime/slice"
)

type ADB struct {
	ExePath string
	Serial  string
}

var Client *ADB

// New returns a new ADB instance.
func New(ctx context.Context) (*ADB, error) {
	adb := ADB{}
	err := adb.findExe()
	if err != nil {
		return nil, fmt.Errorf("failed to find a usable adb executable: %v",
			err)
	}
	log.Debugf("ADB found at path: %s", adb.ExePath)
	//In MESH we don't want to kill the server, we assume there is one running
	//Killing the server interferes with the adb pair functionality

	// log.Debug("Killing existing ADB server if running")
	// adb.KillServer()

	// Confirm that we can call "adb devices" without errors
	_, err = adb.Devices(ctx)
	if err != nil {
		return nil, err
	}
	return &adb, nil
}

func (a *ADB) SetSerial(ctx context.Context, serial string) (string, error) {
	devices, err := a.Devices(ctx)
	if err != nil {
		return "", err
	}

	serial = strings.TrimSpace(serial)
	if len(devices) == 0 {
		return "", fmt.Errorf("no devices detected over ADB")
	}

	if serial != "" {
		// Check that the serial match one of the devices
		// Can be replace with the go package slices in 1.21
		if !saveSlice.ContainsNoCase(devices, serial) {
			// Serial is not an existing device
			return "", fmt.Errorf("serial %s not found in the device list", serial)
		}
		a.Serial = serial
	} else {
		// Problem if multiple devices
		if len(devices) > 1 {
			return "", fmt.Errorf("multiple devices connected, please stop AndroidQF and provide a serial number")
		}
		a.Serial = ""
	}
	return a.Serial, nil
}

// List existing devices
func (a *ADB) Devices(ctx context.Context) ([]string, error) {
	var devices []string
	out, err := exec.CommandContext(ctx, a.ExePath, "devices").Output()
	if err = ctxErr(ctx, err); err != nil {
		return devices, fmt.Errorf("failed to use the adb executable: %v",
			err)
	}

	lines := strings.Split(string(out), "\n")
	for _, s := range lines[1:] {
		dev := strings.Fields(s)
		if len(dev) == 2 {
			devices = append(devices, strings.TrimSpace(dev[0]))
			log.Debug("Found new device: ", dev[0])
		}
	}
	return devices, nil
}

// Run a command to the given phone using exec
// Returns string and/or error
func (a *ADB) Exec(ctx context.Context, args ...string) ([]byte, error) {
	if a.Serial != "" {
		args = append([]string{"-s", a.Serial}, args...)
	}
	out, err := exec.CommandContext(ctx, a.ExePath, args...).Output()
	return out, ctxErr(ctx, err)
}

// GetState returns the output of `adb get-state`.
// It is used to check whether a device is connected. If it is not, adb
// will exit with status 1.
func (a *ADB) GetState(ctx context.Context) (string, error) {
	log.Debug("Starting get-state")
	out, err := a.Exec(ctx, "get-state")
	if err != nil {
		log.Debug("get-state failed")
		return "", err
	}

	log.Debug("get-state ok")
	return strings.TrimSpace(string(out)), nil
}

// Shell executes a shell command through adb.
func (a *ADB) Shell(ctx context.Context, cmd ...string) (string, error) {
	fullCmd := append([]string{"shell"}, cmd...)
	out, err := a.Exec(ctx, fullCmd...)
	if err != nil {
		if out == nil {
			return "", err
		}
		// Still return a value because some commands returns 1 but still works.
		return strings.TrimSpace(string(out)), err
	}

	return strings.TrimSpace(string(out)), nil
}

// Pull downloads a file from the device to a local path.
func (a *ADB) Pull(ctx context.Context, remotePath, localPath string) (string, error) {
	out, err := a.Exec(ctx, "pull", remotePath, localPath)
	if err != nil {
		return string(out), err
	}

	return string(out), nil
}

// Push a file on the phone
func (a *ADB) Push(ctx context.Context, localPath, remotePath string) (string, error) {
	out, err := a.Exec(ctx, "push", localPath, remotePath)
	if err != nil {
		return string(out), err
	}

	return string(out), nil
}

// Backup generates a backup of the specified app, or of all.
func (a *ADB) Backup(ctx context.Context, arg string) error {
	cmd := exec.CommandContext(ctx, a.ExePath, "backup", "-nocompress", arg)
	return ctxErr(ctx, cmd.Run())
}

// Bugreport generates a bugreport of the the device
func (a *ADB) Bugreport(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, a.ExePath, "bugreport", "bugreport.zip")
	return ctxErr(ctx, cmd.Run())
}

// check if file exists
func (a *ADB) FileExists(ctx context.Context, path string) (bool, error) {
	out, err := a.Shell(ctx, "[", "-f", path, "] || echo 1")
	if err != nil {
		return false, err
	}
	if out == "1" {
		return false, nil
	}
	return true, nil
}

// List files in a folder using ls, returns array of strings.
func (a *ADB) ListFiles(ctx context.Context, remotePath string, recursive bool) ([]string, error) {
	var remoteFiles []string
	if recursive {
		out, _ := a.Shell(ctx, "find", remotePath, "2>", "/dev/null")
		if out != "" {
			tmpFiles := strings.Split(out, "\n")
			for _, file := range tmpFiles {
				// Remove errors
				if !strings.HasPrefix(file, "find:") {
					remoteFiles = append(remoteFiles, file)
				}
			}
		}
	} else {
		out, err := a.Shell(ctx, "ls", remotePath)
		if err != nil {
			return remoteFiles, err
		}
		if strings.HasPrefix(out, "ls:") {
			// Error
			return remoteFiles, errors.New(out)
		}
		remoteFiles = strings.Split(out, "\n")
	}

	return remoteFiles, nil
}

func (a *ADB) KillServer(ctx context.Context) (string, error) {
	log.Debug("Killing adb server")
	out, err := exec.CommandContext(ctx, a.ExePath, "kill-server").Output()
	if err != nil {
		log.Debug("kill-server failed")
		return "", err
	}

	log.Debug("kill-server ok")
	return strings.TrimSpace(string(out)), nil
}

// ctxErr returns ctx's error in place of err when ctx ended the command,
// so a cancelled command reports context.Canceled rather than "signal: killed"
func ctxErr(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
