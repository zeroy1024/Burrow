// Package transfer schedules file copies and runs their resumable manifests.
package transfer

import (
	"context"
	"sync/atomic"

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/locales"
	sshsession "github.com/zeroy1024/Burrow/internal/session"
)

var ErrConflictCanceled = locales.NewError(locales.MessageTransferCancelled)

type ClientResult struct {
	Client fileclient.Client
	Err    error
}

// A Run represents one attempt. Configure worker callbacks before starting it.
// ResolveConflict is required for jobs whose batch policy asks the user.
type Run struct {
	ProgressChanged    func(bytes, total int64, speed float64)
	DestinationChanged func(sourceRemote bool, destination string)
	ResolveConflict    func(context.Context, string, fileclient.Entry) (int, error)

	MCP            *MCPOwner
	Manifest       atomic.Pointer[Manifest]
	Journal        func() error
	Context        context.Context
	Cancel         context.CancelFunc
	Request        *sshsession.Request
	Ready          chan ClientResult
	Bytes          atomic.Int64
	Written        atomic.Int64
	Total          atomic.Int64
	AwaitingClient bool
}

// A job and its manifest have one worker owner at a time. The next attempt only
// starts after the preceding worker has returned and the queue has processed its result.
type Job struct {
	MCPFailConflict        bool
	MCPCheck               func(context.Context, fileclient.Client, fileclient.Client, *Node, string) error
	MCPFinish              func(context.Context, *Node, error) error
	SourceRemote           bool
	SourcePath, TargetPath string
	Name, DestinationKey   string

	Batch            *Batch
	Root             *Node
	Total, DoneBytes int64
	Temporary        map[string]struct{}
}

type Batch struct {
	ID     int
	Policy atomic.Int32
}

type Node struct {
	StageDirectory                 string
	MCPPath, MCPSide, MCPAction    string
	Source, Destination, Temporary string
	Entry                          fileclient.Entry
	Children                       []*Node
	Approved                       *fileclient.Entry
	Staged                         *fileclient.Entry
	Partial                        *fileclient.Entry
	Checkpoint                     *Checkpoint
	Digest                         string
	Committing                     bool
	Total                          int64
	Done, Skipped                  bool
}

// Only a digest and acknowledged offset are persisted, never file bytes or a
// hash implementation's internal buffer. Resume reconstructs the hash while
// verifying the target prefix and then reads the source from this offset.
type Checkpoint struct {
	Offset int64  `json:"offset"`
	Digest string `json:"digest"`
}

func ParsePolicy(policy string) int {
	return map[string]int{"overwrite": 1, "skip": 2, "rename": 3}[policy]
}
