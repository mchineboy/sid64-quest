//go:build linux

package scripting

import "syscall"

func limitWorker() error {
	// Go reserves a large virtual arena; allow that reservation but bound growth.
	for resource, limit := range map[int]uint64{syscall.RLIMIT_AS: 2 << 30, syscall.RLIMIT_CPU: 1, syscall.RLIMIT_CORE: 0} {
		if err := syscall.Setrlimit(resource, &syscall.Rlimit{Cur: limit, Max: limit}); err != nil {
			return err
		}
	}
	return nil
}
