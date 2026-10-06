//go:build !linux && !windows

package main

import "fmt"

func diskFree(path string) (int64, error) {
	return 0, fmt.Errorf("disk free check unsupported on this platform")
}
