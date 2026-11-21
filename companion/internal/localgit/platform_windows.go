//go:build windows

package localgit

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func runOwned(ctx context.Context, cmd *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	var mu sync.Mutex
	defer func() { mu.Lock(); windows.CloseHandle(job); job = 0; mu.Unlock() }()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW}
	if err = cmd.Start(); err != nil {
		return err
	}
	failed := func(err error) error { _ = cmd.Process.Kill(); _ = cmd.Wait(); return err }
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return failed(err)
	}
	err = windows.AssignProcessToJobObject(job, process)
	windows.CloseHandle(process)
	if err != nil {
		return failed(err)
	}
	threads, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return failed(err)
	}
	defer windows.CloseHandle(threads)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	err = windows.Thread32First(threads, &entry)
	resumed := false
	for err == nil {
		if entry.OwnerProcessID == uint32(cmd.Process.Pid) {
			thread, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
			if openErr != nil {
				return failed(openErr)
			}
			_, resumeErr := windows.ResumeThread(thread)
			windows.CloseHandle(thread)
			if resumeErr != nil {
				return failed(resumeErr)
			}
			resumed = true
			break
		}
		err = windows.Thread32Next(threads, &entry)
	}
	if !resumed {
		return failed(errors.New("could not resume owned Git process"))
	}
	stop := context.AfterFunc(ctx, func() {
		mu.Lock()
		if job != 0 {
			_ = windows.TerminateJobObject(job, 1)
		}
		mu.Unlock()
	})
	defer stop()
	return cmd.Wait()
}
