//go:build !linux

package compute

import "errors"

func limitData(uint64) error { return errors.New("no data limit on this system") }

func offerToOOMKiller() {}
