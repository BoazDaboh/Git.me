//go:build !windows

package main

func watchSystemSleep() func()            { return func() {} }
func lastSleepMillis() int64              { return 0 }
func systemSleepInterval() (int64, int64) { return 0, 0 }
func systemAwakeMillis() int64            { return 0 }
