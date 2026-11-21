package processes

import (
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type child struct {
	cmd *exec.Cmd
	mu  sync.Mutex
	job windows.Handle
}

func launch(folder, command string, output io.Writer) (*child, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	shell := filepath.Join(system, "cmd.exe")
	cmd := exec.Command(shell)
	cmd.Args = nil
	cmd.Dir = folder
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.WaitDelay = 2e9
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW, CmdLine: `"` + shell + `" /d /s /c "` + command + `"`}
	if err = cmd.Start(); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	failed := func(err error) (*child, error) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		windows.CloseHandle(job)
		return nil, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return failed(err)
	}
	err = windows.AssignProcessToJobObject(job, process)
	windows.CloseHandle(process)
	if err != nil {
		return failed(err)
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return failed(err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	err = windows.Thread32First(snapshot, &entry)
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
		err = windows.Thread32Next(snapshot, &entry)
	}
	if !resumed {
		return failed(errors.New("could not resume the owned process"))
	}
	return &child{cmd: cmd, job: job}, nil
}
func (c *child) pid() int      { return c.cmd.Process.Pid }
func (c *child) wait() error   { return c.cmd.Wait() }
func (c *child) exitCode() int { return c.cmd.ProcessState.ExitCode() }
func (c *child) kill() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.job == 0 {
		return nil
	}
	return windows.TerminateJobObject(c.job, 1)
}
func (c *child) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.job != 0 {
		windows.CloseHandle(c.job)
		c.job = 0
	}
}
