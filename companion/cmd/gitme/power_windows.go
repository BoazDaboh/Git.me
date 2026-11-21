//go:build windows

package main

import (
	"log"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var lastSleep atomic.Int64
var lastResume atomic.Int64
var queryAwake = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryUnbiasedInterruptTime")
var powerDLL = syscall.NewLazyDLL("powrprof.dll")
var registerPower = powerDLL.NewProc("PowerRegisterSuspendResumeNotification")
var unregisterPower = powerDLL.NewProc("PowerUnregisterSuspendResumeNotification")
var powerCallback = syscall.NewCallback(func(_ uintptr, event uintptr, _ uintptr) uintptr {
	if event == 4 {
		lastSleep.Store(time.Now().UnixMilli())
	}
	if event == 7 || event == 18 {
		lastResume.Store(time.Now().UnixMilli())
	}
	return 0
})

type powerSubscription struct {
	Callback uintptr
	Context  uintptr
}

var powerParams = powerSubscription{Callback: powerCallback}

func watchSystemSleep() func() {
	var handle uintptr
	result, _, _ := registerPower.Call(2, uintptr(unsafe.Pointer(&powerParams)), uintptr(unsafe.Pointer(&handle)))
	if result != 0 {
		log.Printf("Sleep notifications unavailable: Windows error %d", result)
		return func() {}
	}
	return func() { unregisterPower.Call(handle) }
}
func lastSleepMillis() int64              { return lastSleep.Load() }
func systemSleepInterval() (int64, int64) { return lastSleep.Load(), lastResume.Load() }
func systemAwakeMillis() int64 {
	var ticks uint64
	result, _, _ := queryAwake.Call(uintptr(unsafe.Pointer(&ticks)))
	if result == 0 {
		return 0
	}
	return int64(ticks / 10000)
}
