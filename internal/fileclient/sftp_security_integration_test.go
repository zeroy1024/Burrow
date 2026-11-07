//go:build integration

package fileclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"path"
	"sync"
	"testing"
	"time"
)

func TestSFTPPrivateStageAtomicCreation(t *testing.T) {
	owner := sftpFixtureConnection(t)
	var observed *sftpObservedStream
	client, err := newSFTP(t.Context(), owner, func(ctx context.Context) (io.ReadWriteCloser, error) {
		stream, err := owner.OpenSubsystem(ctx, "sftp")
		if err != nil {
			return nil, err
		}
		observed = &sftpObservedStream{ReadWriteCloser: stream}
		return observed, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	name := path.Join("/home/burrow", ".burrow-private-"+rand.Text()+".part")
	defer removeSFTPTestFile(t, client, name)
	private := client.(PrivateStageWriter)
	output, created, err := private.OpenPrivateWrite(t.Context(), name, 0, false, true)
	if err != nil || !created {
		t.Fatalf("create private stage: created=%v, error=%v", created, err)
	}
	entry, err := client.Stat(t.Context(), name)
	if err != nil || !entry.ModeKnown || entry.Mode.Perm() != 0600 || entry.Size != 0 {
		output.Close()
		t.Fatalf("stage was not private and empty before writing: %+v, error=%v", entry, err)
	}
	flags, attrs, mode := observed.openRequest(t, name)
	if flags&0x28 != 0x28 || attrs&4 == 0 || mode&07777 != 0600 {
		output.Close()
		t.Fatalf("creation OPEN did not request exclusive 0600: flags=%#x, attrs=%#x, mode=%#o", flags, attrs, mode)
	}
	_, writeErr := io.WriteString(output, "private transfer payload")
	if err := errors.Join(writeErr, output.Close()); err != nil {
		t.Fatal(err)
	}
	sftpTestContent(t, client, name, "private transfer payload")

	conflict, created, err := private.OpenPrivateWrite(t.Context(), name, 0, true, true)
	if conflict != nil {
		conflict.Close()
	}
	if err == nil || created {
		t.Fatalf("exclusive creation claimed an existing file: created=%v, error=%v", created, err)
	}
	sftpTestContent(t, client, name, "private transfer payload")
}

func TestSFTPPrivateStageResume(t *testing.T) {
	for _, test := range []struct {
		name string
		mode fs.FileMode
	}{
		{"public_part", 0644},
		{"read_only_part", 0400},
		{"no_access_part", 0000},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner := sftpFixtureConnection(t)
			client, err := NewSFTP(t.Context(), owner)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := client.Close(); err != nil {
					t.Error(err)
				}
			})
			name := path.Join("/home/burrow", ".burrow-resume-"+rand.Text()+".part")
			defer removeSFTPTestFile(t, client, name)
			output, err := client.OpenWrite(t.Context(), name, 0, true)
			if err != nil {
				t.Fatal(err)
			}
			const prefix = "confirmed-prefix:"
			_, writeErr := io.WriteString(output, prefix)
			if err := errors.Join(writeErr, output.Close()); err != nil {
				t.Fatal(err)
			}
			if err := client.(PermissionSetter).Chmod(t.Context(), name, test.mode); err != nil {
				t.Fatal(err)
			}
			output, created, err := client.(PrivateStageWriter).OpenPrivateWrite(t.Context(), name, int64(len(prefix)), false, false)
			if err != nil || created {
				t.Fatalf("resume private stage: created=%v, error=%v", created, err)
			}
			entry, err := client.Stat(t.Context(), name)
			if err != nil || !entry.ModeKnown || entry.Mode.Perm() != 0600 || entry.Size != int64(len(prefix)) {
				output.Close()
				t.Fatalf("resume did not restrict the existing prefix before writing: %+v, error=%v", entry, err)
			}
			sftpTestContent(t, client, name, prefix)
			_, writeErr = io.WriteString(output, "remaining bytes")
			if err := errors.Join(writeErr, output.Close()); err != nil {
				t.Fatal(err)
			}
			sftpTestContent(t, client, name, prefix+"remaining bytes")
		})
	}
}

type sftpObservedStream struct {
	io.ReadWriteCloser
	mu   sync.Mutex
	wire bytes.Buffer
}

func (s *sftpObservedStream) Write(data []byte) (int, error) {
	n, err := s.ReadWriteCloser.Write(data)
	s.mu.Lock()
	s.wire.Write(data[:n])
	s.mu.Unlock()
	return n, err
}

// Inspect the actual request sent to the fixture, so an initially public file
// followed by chmod cannot satisfy the atomic-creation regression.
func (s *sftpObservedStream) openRequest(t *testing.T, name string) (uint32, uint32, uint32) {
	t.Helper()
	s.mu.Lock()
	data := bytes.Clone(s.wire.Bytes())
	s.mu.Unlock()
	for len(data) > 0 {
		if len(data) < 4 {
			t.Fatal("incomplete outgoing SFTP packet length")
		}
		length := int(binary.BigEndian.Uint32(data))
		if length < 5 || length > len(data)-4 {
			t.Fatal("incomplete outgoing SFTP packet")
		}
		packet := bytes.NewReader(data[4 : 4+length])
		data = data[4+length:]
		kind, _ := packet.ReadByte()
		if kind != 3 { // SSH_FXP_OPEN
			continue
		}
		var id, nameLength, flags, attrs, mode uint32
		for _, value := range []*uint32{&id, &nameLength} {
			if err := binary.Read(packet, binary.BigEndian, value); err != nil {
				t.Fatal(err)
			}
		}
		if uint64(nameLength) > uint64(packet.Len()) {
			t.Fatal("incomplete outgoing SFTP path")
		}
		filename := make([]byte, int(nameLength))
		if _, err := io.ReadFull(packet, filename); err != nil {
			t.Fatal(err)
		}
		if string(filename) != name {
			continue
		}
		for _, value := range []*uint32{&flags, &attrs} {
			if err := binary.Read(packet, binary.BigEndian, value); err != nil {
				t.Fatal(err)
			}
		}
		if attrs&1 != 0 {
			if _, err := packet.Seek(8, io.SeekCurrent); err != nil {
				t.Fatal(err)
			}
		}
		if attrs&2 != 0 {
			if _, err := packet.Seek(8, io.SeekCurrent); err != nil {
				t.Fatal(err)
			}
		}
		if attrs&4 != 0 {
			if err := binary.Read(packet, binary.BigEndian, &mode); err != nil {
				t.Fatal(err)
			}
		}
		return flags, attrs, mode
	}
	t.Fatal("no outgoing OPEN request for the private stage")
	return 0, 0, 0
}

func sftpTestContent(t *testing.T, client Client, name, want string) {
	t.Helper()
	input, err := client.OpenRead(t.Context(), name, 0)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(input)
	if err := errors.Join(readErr, input.Close()); err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("unexpected remote staging contents at %s", name)
	}
}

func removeSFTPTestFile(t *testing.T, client Client, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := client.Remove(ctx, name, false); err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("clean up remote staging file: %v", err)
	}
}
