package terminalwire

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUnixListenerPreservesLiveSocketAndRecoversStaleSocket(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "rck-listen-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "core.sock")
	first, err := Listen("unix:" + path)
	require.NoError(t, err)
	_, err = Listen("unix:" + path)
	require.Error(t, err, "must not replace a live listener")
	conn, err := net.DialTimeout("unix", path, time.Second)
	require.NoError(t, err)
	conn.Close()
	require.NoError(t, first.Close())
	// Reproduce a process crash's leftover socket inode.
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	require.NoError(t, err)
	stale.SetUnlinkOnClose(false)
	require.NoError(t, stale.Close())
	replacement, err := Listen("unix:" + path)
	require.NoError(t, err)
	defer replacement.Close()
	conn, err = net.DialTimeout("unix", path, time.Second)
	require.NoError(t, err)
	conn.Close()
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestUnixListenerRefusesOrdinaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-socket")
	require.NoError(t, os.WriteFile(path, []byte("preserve this"), 0600))
	_, err := Listen("unix:" + path)
	require.Error(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "preserve this", string(data))
}
