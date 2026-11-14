//go:build integration

package hostinfo_test

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/hostinfo"
	"github.com/zeroy1024/Burrow/internal/sshclient"

	"golang.org/x/crypto/ssh"
)

// fixtureRunner gives collectors the same command and SFTP access the desktop
// application builds on one SSH transport.
type fixtureRunner struct{ connection *sshclient.Connection }

func (r fixtureRunner) Exec(ctx context.Context, command string) ([]byte, error) {
	return r.connection.Exec(ctx, command)
}

func (r fixtureRunner) Files(ctx context.Context) (hostinfo.Files, error) {
	for {
		if err := r.connection.WaitChannelReady(ctx); err != nil {
			return nil, err
		}
		if err := fileclient.WaitSFTPReady(ctx, r.connection); err != nil {
			return nil, err
		}
		client, err := fileclient.NewSFTP(ctx, r.connection)
		if errors.Is(err, sshclient.ErrChannelPending) || errors.Is(err, fileclient.ErrSFTPInitPending) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return fixtureFiles{client}, nil
	}
}

type fixtureFiles struct{ client fileclient.Client }

func (f fixtureFiles) StatFilesystem(ctx context.Context, name string) (hostinfo.Usage, error) {
	stats, err := f.client.(fileclient.FilesystemStatter).StatFilesystem(ctx, name)
	return hostinfo.Usage{Total: stats.Total, Free: stats.Free}, err
}

func (f fixtureFiles) ReadFile(ctx context.Context, name string, limit int64) ([]byte, error) {
	stream, err := f.client.OpenRead(ctx, name, 0)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(stream, limit))
	return data, errors.Join(readErr, stream.Close())
}

func (f fixtureFiles) Close() error { return f.client.Close() }

func linuxFixture(t *testing.T) fixtureRunner {
	t.Helper()
	address, directory := os.Getenv("BURROW_TEST_ADDRESS"), os.Getenv("BURROW_TEST_DIR")
	if address == "" || directory == "" {
		t.Skip("BURROW_TEST_ADDRESS and BURROW_TEST_DIR are required for the Docker SSH fixture")
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		t.Fatal("the Docker SSH fixture must use a loopback address")
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	// The disposable loopback fixture has no prior identity to compare with.
	connection, err := sshclient.Dial(ctx, sshclient.Config{
		Host: host, Port: port, User: "burrow", Auth: "key",
		KeyPath: filepath.Join(directory, "key_plain"), HostKey: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		cancel()
		t.Fatalf("connect fixture: %v", err)
	}
	// Each command is its own channel; hold the transport between them.
	release, err := connection.Retain()
	if err != nil {
		connection.Close()
		cancel()
		t.Fatalf("retain fixture transport: %v", err)
	}
	t.Cleanup(func() { release(); connection.Close(); cancel(); connection.Wait() })
	return fixtureRunner{connection}
}

func TestLinuxIntegrationFacts(t *testing.T) {
	runner := linuxFixture(t)
	detection, err := hostinfo.Detect(t.Context(), runner, runner.connection.ServerVersion())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if detection.Platform != hostinfo.Linux {
		t.Fatalf("platform = %q, want linux", detection.Platform)
	}
	facts, err := hostinfo.Collect(t.Context(), runner, runner.connection.ServerVersion(), "")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if !facts.Valid() {
		t.Fatalf("facts are not valid: %+v", facts)
	}
	hostname, err := runner.Exec(t.Context(), "hostname")
	if err != nil {
		t.Fatal(err)
	}
	kernel, err := runner.Exec(t.Context(), "uname -r")
	if err != nil {
		t.Fatal(err)
	}
	if facts.Hostname != strings.TrimSpace(string(hostname)) || facts.Kernel != strings.TrimSpace(string(kernel)) {
		t.Fatalf("hostname/kernel = %q/%q, want %q/%q", facts.Hostname, facts.Kernel, hostname, kernel)
	}
	if facts.OSID != "ubuntu" || facts.OSVersion != "24.04" || !strings.Contains(facts.OSName, "Ubuntu") || facts.Arch == "" {
		t.Fatalf("unexpected OS facts for the Ubuntu 24.04 fixture: %+v", facts)
	}
}

func TestLinuxIntegrationSamplerAndRootDisk(t *testing.T) {
	runner := linuxFixture(t)
	sampler := hostinfo.For(hostinfo.Linux).NewSampler(runner)
	first, err := sampler.Sample(t.Context())
	if err != nil {
		t.Fatalf("first Sample: %v", err)
	}
	if first.CPU != nil {
		t.Fatalf("first sample reported a CPU rate without a previous reading: %v", *first.CPU)
	}
	// Generate measurable CPU time between the two readings.
	if _, err := runner.Exec(t.Context(), "i=0; while [ $i -lt 200000 ]; do i=$((i+1)); done"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	second, err := sampler.Sample(t.Context())
	if err != nil {
		t.Fatalf("second Sample: %v", err)
	}
	if second.CPU == nil || *second.CPU < 0 || *second.CPU > 100 {
		t.Fatalf("CPU rate missing or out of range: %v", second.CPU)
	}
	if second.Memory == nil || *second.Memory <= 0 || *second.Memory > 100 ||
		second.MemoryTotal == 0 || second.MemoryUsed == 0 || second.MemoryUsed > second.MemoryTotal {
		t.Fatalf("memory reading inconsistent: %v used=%d total=%d", second.Memory, second.MemoryUsed, second.MemoryTotal)
	}
	if second.Cores < 1 || second.Load == "" || second.Uptime == nil || *second.Uptime <= 0 {
		t.Fatalf("cores/load/uptime missing: %d %q %v", second.Cores, second.Load, second.Uptime)
	}
	if second.Processes == nil || *second.Processes < 1 || len(second.Top) == 0 {
		t.Fatalf("process readings missing: %v %v", second.Processes, second.Top)
	}
	if second.Down == nil || second.Up == nil || *second.Down < 0 || *second.Up < 0 {
		t.Fatalf("network rates missing after two readings: %v %v", second.Down, second.Up)
	}
	if len(second.Unavailable) != 0 {
		t.Fatalf("fixture readings reported unavailable: %v", second.Unavailable)
	}
	if second.DiskKey == "" {
		t.Fatal("root file system was not identified")
	}
	usage, err := sampler.Disk(t.Context(), second.DiskKey)
	if err != nil {
		t.Fatalf("Disk: %v", err)
	}
	if usage.Total == 0 || usage.Free > usage.Total {
		t.Fatalf("root usage inconsistent: %+v", usage)
	}
	// Statistics for another root must not be attributed to this sample.
	if _, err := sampler.Disk(t.Context(), second.DiskKey+"-other"); err == nil {
		t.Fatal("Disk accepted a root identity that does not match the SFTP root")
	}
}

func TestLinuxIntegrationSamplerRecoversAfterCanceledReading(t *testing.T) {
	runner := linuxFixture(t)
	sampler := hostinfo.For(hostinfo.Linux).NewSampler(runner)
	if _, err := sampler.Sample(t.Context()); err != nil {
		t.Fatalf("first Sample: %v", err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := sampler.Sample(canceled); err == nil {
		t.Fatal("Sample succeeded with a canceled context")
	}
	// A failed reading discards the baseline; the next is a first reading again.
	recovered, err := sampler.Sample(t.Context())
	if err != nil {
		t.Fatalf("Sample after cancellation: %v", err)
	}
	if recovered.CPU != nil {
		t.Fatal("rate computed across a failed reading")
	}
}
