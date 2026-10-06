//go:build linux

package main

import "syscall"

func diskFree(path string) (int64, error) {
	var info syscall.Statfs_t
	err := syscall.Statfs(path, &info)
	return int64(info.Bavail) * int64(info.Bsize), err
}
