package agentrunner

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ProcessMonitor periodically samples process RSS bytes until stopped.
type ProcessMonitor struct {
	pid      int
	peakRSS  int64
	stopChan chan struct{}
	mu       sync.Mutex
	done     chan struct{}
}

// StartProcessMonitor starts sampling memory usage for the given pid.
func StartProcessMonitor(pid int, sampleInterval time.Duration) *ProcessMonitor {
	pm := &ProcessMonitor{
		pid:      pid,
		stopChan: make(chan struct{}),
		done:     make(chan struct{}),
	}

	go pm.run(sampleInterval)
	return pm
}

func (pm *ProcessMonitor) run(interval time.Duration) {
	defer close(pm.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.stopChan:
			return
		case <-ticker.C:
			rss := ReadRSS(pm.pid)
			pm.mu.Lock()
			if rss > pm.peakRSS {
				pm.peakRSS = rss
			}
			pm.mu.Unlock()
		}
	}
}

// Stop stops the monitor and returns the peak RSS in bytes.
func (pm *ProcessMonitor) Stop() int64 {
	select {
	case <-pm.stopChan:
		// already closed
	default:
		close(pm.stopChan)
	}
	<-pm.done

	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.peakRSS
}

// ReadRSS reads the resident set size in bytes from /proc/<pid>/statm.
func ReadRSS(pid int) int64 {
	if pid <= 0 {
		return 0
	}
	statmPath := fmt.Sprintf("/proc/%d/statm", pid)
	data, err := os.ReadFile(statmPath)
	if err != nil {
		return 0
	}

	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0
	}

	// field[1] is resident pages
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}

	pageSize := int64(os.Getpagesize())
	return pages * pageSize
}

// KillProcessTree terminates the process group and any lingering child processes.
func KillProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}

	// 1. Gather any known direct child PIDs before killing the parent
	childPids := GetChildPIDs(pid)

	// 2. Send SIGTERM to process group and process
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = syscall.Kill(pid, syscall.SIGTERM)

	// Brief grace period for clean teardown
	time.Sleep(50 * time.Millisecond)

	// 3. Send SIGKILL to the entire process group and process
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)

	// 4. Force kill any individual children that might have detached
	for _, cpid := range childPids {
		_ = syscall.Kill(cpid, syscall.SIGKILL)
	}

	return nil
}

// GetChildPIDs returns child process IDs of a given PID by inspecting /proc/<pid>/task/<pid>/children.
func GetChildPIDs(pid int) []int {
	if pid <= 0 {
		return nil
	}

	path := fmt.Sprintf("/proc/%d/task/%d/children", pid, pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var pids []int
	fields := strings.Fields(string(data))
	for _, f := range fields {
		if cpid, err := strconv.Atoi(f); err == nil && cpid > 0 {
			pids = append(pids, cpid)
			// Recursively fetch grandchildren
			pids = append(pids, GetChildPIDs(cpid)...)
		}
	}
	return pids
}

// IsProcessAlive checks if a process with the given PID is still active in the OS.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// Sending signal 0 performs error checking without actually sending a signal
	err := syscall.Kill(pid, 0)
	return err == nil
}
