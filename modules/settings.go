// Copyright (c) 2021-2023 Claudio Guarnieri.
// Use of this source code is governed by the MVT License 1.1
// which can be found in the LICENSE file.

package modules

import (
	"context"
	"fmt"

	"github.com/BARGHEST-ngo/androidqf_mesh/acquisition"
	"github.com/BARGHEST-ngo/androidqf_mesh/adb"
	"github.com/BARGHEST-ngo/androidqf_mesh/log"
)

type Settings struct {
	StoragePath string
}

func NewSettings() *Settings {
	return &Settings{}
}

func (s *Settings) Name() string {
	return "settings"
}

func (s *Settings) InitStorage(storagePath string) error {
	s.StoragePath = storagePath
	return nil
}

func (s *Settings) Run(ctx context.Context, acq *acquisition.Acquisition, fast bool) error {
	log.Info("Collecting device settings...")

	for _, namespace := range []string{"system", "secure", "global"} {
		if err := ctx.Err(); err != nil {
			return err
		}
		out, err := adb.Client.Shell(ctx, fmt.Sprintf("cmd settings list %s", namespace))
		if err != nil {
			return fmt.Errorf("failed to run `cmd settings %s`: %v", namespace, err)
		}

		err = saveStringToAcquisition(acq, fmt.Sprintf("settings_%s.txt", namespace), out)
		if err != nil {
			log.Errorf("Impossible to save settings: %v", err)
		}
	}

	return nil
}
