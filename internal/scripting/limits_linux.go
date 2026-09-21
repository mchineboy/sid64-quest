//go:build linux

package scripting

import "syscall"

func limitWorker() error {
	// Do not set RLIMIT_AS: modern Go reserves a large sparse virtual arena, so
	// address-space caps cause intermittent startup OOMs without bounding RSS.
	// The Go memory target, step budget and parent deadline bound actual work.
	for resource, limit := range map[int]uint64{syscall.RLIMIT_CPU: 2, syscall.RLIMIT_CORE: 0} {
		if err := syscall.Setrlimit(resource, &syscall.Rlimit{Cur: limit, Max: limit}); err != nil {
			return err
		}
	}
	return nil
}
