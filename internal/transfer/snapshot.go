package transfer

import (
	"encoding/hex"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/profile"
	"github.com/zeroy1024/Burrow/internal/workspace"
)

type Snapshot struct {
	Version     int               `json:"version"`
	NextBatchID int               `json:"nextBatchId"`
	Batches     []SavedBatch      `json:"batches"`
	NextID      int               `json:"nextId"`
	Sessions    []SavedSession    `json:"sessions"`
	Connections []SavedConnection `json:"connections"`
	Tasks       []SavedTask       `json:"tasks"`
}

type SavedBatch struct {
	ID     int `json:"id"`
	Policy int `json:"policy"`
}

type SavedSession struct {
	ID         int               `json:"id"`
	ViewID     string            `json:"viewId,omitempty"`
	Session    workspace.Session `json:"session"`
	LocalPath  string            `json:"localPath"`
	RemotePath string            `json:"remotePath"`
}

type SavedConnection struct {
	EndpointRef string             `json:"endpointRef,omitempty"`
	StepRefs    []string           `json:"stepRefs,omitempty"`
	ID          int                `json:"id"`
	Endpoint    profile.Endpoint   `json:"endpoint,omitzero"`
	Steps       []profile.Endpoint `json:"steps,omitempty"`
}

type SavedTask struct {
	MCP            *MCPOwner        `json:"mcp,omitempty"`
	ID             int              `json:"id"`
	Batch          int              `json:"batch"`
	Session        int              `json:"session"`
	Entry          fileclient.Entry `json:"entry"`
	State          State            `json:"state"`
	Transferred    int64            `json:"transferred"`
	Started        time.Time        `json:"started,omitempty"`
	Completed      time.Time        `json:"completed,omitempty"`
	Error          string           `json:"error,omitempty"`
	CleanupPending bool             `json:"cleanupPending,omitempty"`
	SourceRemote   bool             `json:"sourceRemote"`
	SourcePath     string           `json:"sourcePath"`
	TargetPath     string           `json:"targetPath"`
	Name           string           `json:"name"`
	Policy         int              `json:"policy"`
	Total          int64            `json:"total"`
	DoneBytes      int64            `json:"doneBytes"`
	Root           *SavedNode       `json:"root,omitempty"`
	Temporary      []string         `json:"temporary,omitempty"`
}

type SavedNode struct {
	StageDirectory string            `json:"stageDirectory,omitempty"`
	Source         string            `json:"source"`
	Destination    string            `json:"destination,omitempty"`
	Temporary      string            `json:"temporary,omitempty"`
	Entry          fileclient.Entry  `json:"entry"`
	Committing     bool              `json:"committing,omitempty"`
	Staged         *fileclient.Entry `json:"staged,omitempty"`
	Partial        *fileclient.Entry `json:"partial,omitempty"`
	Checkpoint     *Checkpoint       `json:"checkpoint,omitempty"`
	Digest         string            `json:"digest,omitempty"`
	Approved       *fileclient.Entry `json:"approved,omitempty"`
	Children       []*SavedNode      `json:"children,omitempty"`
	Total          int64             `json:"total"`
	Done           bool              `json:"done,omitempty"`
	Skipped        bool              `json:"skipped,omitempty"`
}

type MCPOwner struct {
	KeyID, OperationID, ConnectionID, Identity, LocalRoot, Conflict string
	Outcome                                                         string
	CleanupOnly                                                     bool
	CreatedAt                                                       time.Time
	PolicyConnectionID                                              string `json:",omitempty"`
}

type Manifest struct {
	Root                   *SavedNode
	Temporary              []string
	Total, Done            int64
	SourcePath, TargetPath string
}

func SnapshotManifest(job *Job) *Manifest {
	m := &Manifest{Root: snapshotNode(job.Root), Total: job.Total, Done: job.DoneBytes, Temporary: []string{}, SourcePath: job.SourcePath, TargetPath: job.TargetPath}
	for name := range job.Temporary {
		m.Temporary = append(m.Temporary, name)
	}
	return m
}

func snapshotNode(node *Node) *SavedNode {
	if node == nil {
		return nil
	}
	stored := &SavedNode{Source: node.Source, Destination: node.Destination, Temporary: node.Temporary,
		StageDirectory: node.StageDirectory,
		Entry:          node.Entry, Approved: node.Approved, Committing: node.Committing,
		Staged: node.Staged, Partial: node.Partial, Digest: node.Digest,
		Total: node.Total, Done: node.Done, Skipped: node.Skipped}
	for _, pair := range []struct {
		src *fileclient.Entry
		dst **fileclient.Entry
	}{{node.Approved, &stored.Approved}, {node.Staged, &stored.Staged}, {node.Partial, &stored.Partial}} {
		if pair.src != nil {
			value := *pair.src
			*pair.dst = &value
		}
	}
	if node.Checkpoint != nil {
		checkpoint := *node.Checkpoint
		stored.Checkpoint = &checkpoint
	}
	for _, child := range node.Children {
		stored.Children = append(stored.Children, snapshotNode(child))
	}
	return stored
}

func RestoreNode(stored *SavedNode) *Node {
	if stored == nil {
		return nil
	}
	node := &Node{Source: stored.Source, Destination: stored.Destination, Temporary: stored.Temporary,
		StageDirectory: stored.StageDirectory,
		Entry:          stored.Entry, Approved: stored.Approved, Committing: stored.Committing,
		Staged: stored.Staged, Partial: stored.Partial, Digest: stored.Digest,
		Total: stored.Total, Done: stored.Done, Skipped: stored.Skipped}
	if stored.Checkpoint != nil {
		checkpoint := *stored.Checkpoint
		node.Checkpoint = &checkpoint
	}
	for _, child := range stored.Children {
		node.Children = append(node.Children, RestoreNode(child))
	}
	return node
}

func (saved *Snapshot) Validate() error {
	if saved.Version != 4 || saved.NextID < 0 || saved.NextBatchID < 0 {
		return locales.NewError(locales.MessageInvalidTransferHistoryVersionOrCount)
	}
	batches := map[int]SavedBatch{}
	for _, batch := range saved.Batches {
		if batch.ID <= 0 || batch.ID > saved.NextBatchID || batches[batch.ID].ID != 0 || batch.Policy < 0 || batch.Policy > 3 {
			return locales.NewError(locales.MessageInvalidTransferBatch)
		}
		batches[batch.ID] = batch
	}
	groups := map[int]SavedConnection{}
	for _, group := range saved.Connections {
		if group.ID <= 0 || groups[group.ID].ID != 0 {
			return locales.NewError(locales.MessageDuplicateOrInvalidTransferConnectionId)
		}
		if err := profile.ValidateTransferEndpoint(group.Endpoint, false); err != nil {
			return err
		}
		seen := map[string]bool{}
		routeSteps := []profile.Endpoint{}
		if group.Endpoint.GatewayRoute != nil {
			for _, hop := range *group.Endpoint.GatewayRoute {
				if hop.SSH != nil {
					routeSteps = append(routeSteps, *hop.SSH)
				}
			}
		}
		for index, step := range group.Steps {
			if err := profile.ValidateTransferEndpoint(step, false); err != nil {
				return err
			}
			if seen[step.ID] {
				return locales.NewError(locales.MessageTransferJumpHostSnapshotsContainALoop)
			}
			seen[step.ID] = true
			if index < len(group.Steps)-1 && (index >= len(routeSteps) || !profile.SameTransferEndpoint(step, routeSteps[index])) {
				return locales.NewError(locales.MessageInvalidTransferGatewaySnapshotOrder)
			}
		}
		if len(group.Steps) > 0 && len(group.Steps) != len(routeSteps)+1 {
			return locales.NewError(locales.MessageInvalidTransferGatewaySnapshotCount)
		}
		if len(group.Steps) > 0 && !profile.SameTransferEndpoint(group.Endpoint, group.Steps[len(group.Steps)-1]) {
			return locales.NewError(locales.MessageTheTransferConnectionTargetDoesNotMatchItsSnapshot)
		}
		groups[group.ID] = group
	}
	sessions := map[int]SavedSession{}
	for _, session := range saved.Sessions {
		if session.ID <= 0 || sessions[session.ID].ID != 0 {
			return locales.NewError(locales.MessageDuplicateOrInvalidTransferFileSessionId)
		}
		if err := profile.ValidateTransferEndpoint(session.Session.Endpoint, true); err != nil {
			return err
		}
		if strings.TrimSpace(session.ViewID) != session.ViewID {
			return locales.NewError(locales.MessageInvalidFileSessionIdForTransfer)
		}
		if strings.ContainsRune(session.LocalPath+session.RemotePath, '\x00') {
			return locales.NewError(locales.MessageInvalidTransferBrowsingPath)
		}
		if session.Session.ProfileID != "" && session.Session.ProfileID != session.Session.Endpoint.ID {
			return locales.NewError(locales.MessageInvalidTransferSourceSessionId)
		}
		if session.Session.Endpoint.Protocol == "ftp" {
			if session.Session.Connection != 0 {
				return locales.NewError(locales.MessageFtpCannotReferenceAnSshConnection)
			}
		} else {
			group, ok := groups[session.Session.Connection]
			if !ok || !profile.SameTransferEndpoint(session.Session.Endpoint, group.Endpoint) || !profile.SameRoute(session.Session.Endpoint.GatewayRoute, group.Endpoint.GatewayRoute) {
				return locales.NewError(locales.MessageTheSftpConnectionSnapshotDoesNotMatch)
			}
		}
		sessions[session.ID] = session
	}
	ids := map[int]bool{}
	for _, task := range saved.Tasks {
		if task.MCP != nil && !validMCPTransfer(task.MCP, task) {
			return locales.NewError(locales.MessageInvalidMcpTransferIdentity)
		}
		if task.ID <= 0 || task.ID > saved.NextID || ids[task.ID] || sessions[task.Session].ID == 0 ||
			task.State < Queued || task.State > Failed || task.Transferred < 0 || batches[task.Batch].ID == 0 || task.Policy != batches[task.Batch].Policy ||
			task.Total < 0 || task.DoneBytes < 0 || task.Entry.Size < 0 || task.Policy < 0 || task.Policy > 3 ||
			task.CleanupPending && task.State != Failed ||
			!SafeName(task.Name) || !SafeName(task.Entry.Name) ||
			task.SourcePath == "" || task.TargetPath == "" || strings.ContainsRune(task.SourcePath+task.TargetPath, '\x00') {
			return locales.NewError(locales.MessageInvalidTransferTaskRecord)
		}
		ids[task.ID] = true
		owned := map[string]bool{}
		if task.Root == nil {
			if task.Total != 0 || task.DoneBytes != 0 || len(task.Temporary) != 0 || task.State == Done {
				return locales.NewError(locales.MessageATransferTaskWithoutAManifestContainsPartialFiles)
			}
		} else {
			if !canonicalTransferPath(task.SourceRemote, task.SourcePath) || !canonicalTransferPath(!task.SourceRemote, task.TargetPath) || task.Root.Source != task.SourcePath {
				return locales.NewError(locales.MessageInvalidTransferRootDirectoryPath)
			}
			if task.Name != task.Root.Entry.Name || task.Name != Base(task.SourceRemote, task.SourcePath) {
				return locales.NewError(locales.MessageTheTransferSourceNameDoesNotMatchTheManifest)
			}
			expectedName := task.Name
			if task.Root.Destination != "" {
				expectedName = Base(!task.SourceRemote, task.Root.Destination)
			}
			if task.Entry.Name != expectedName {
				return locales.NewError(locales.MessageTheDisplayedTransferDestinationDoesNotMatchTheManifest)
			}
			total, done, err := validateTransferNode(task.Root, task.SourceRemote, task.SourcePath, task.TargetPath, owned, 0)
			if err != nil {
				return err
			}
			if total != task.Total || done != task.DoneBytes {
				return locales.NewError(locales.MessageTransferManifestTotalsDoNotMatch)
			}
			if task.State == Done && !task.Root.Done && !task.Root.Skipped {
				return locales.NewError(locales.MessageACompletedTransferStillHasAnIncompleteManifest)
			}
		}
		if len(owned) != len(task.Temporary) {
			return locales.NewError(locales.MessageTheTransferTemporaryFileSetDoesNotMatch)
		}
		for _, name := range task.Temporary {
			if !owned[name] {
				return locales.NewError(locales.MessageATransferTemporaryFileIsNotInTheManifest)
			}
			delete(owned, name)
		}
	}
	return nil
}

func canonicalTransferPath(remote bool, name string) bool {
	if name == "" || strings.ContainsRune(name, '\x00') {
		return false
	}
	if remote {
		return strings.HasPrefix(name, "/") && Join(true, name, "") == name
	}
	return filepath.IsAbs(name) && filepath.Clean(name) == name
}

func validTransferTemporary(remote bool, name, destination string) bool {
	if name == destination || !canonicalTransferPath(remote, name) || Dir(remote, name) != Dir(remote, destination) {
		return false
	}
	base := Base(remote, name)
	prefix := ".burrow-"
	// Existing saved tasks still own parts created before the rename.
	if strings.HasPrefix(base, ".sshdesk-") {
		prefix = ".sshdesk-"
	}
	if !strings.HasPrefix(base, prefix) || !strings.HasSuffix(base, ".part") {
		return false
	}
	encoded := strings.TrimSuffix(strings.TrimPrefix(base, prefix), ".part")
	value, err := hex.DecodeString(encoded)
	return err == nil && len(value) == 16 && hex.EncodeToString(value) == encoded
}

// Every child's source is the named child of its parent. Every destination is
// one safe component below its parent, and only generated staging names in that
// same directory may be owned/deleted. A journal cannot nominate arbitrary files.
func validateTransferNode(node *SavedNode, sourceRemote bool, source, destinationParent string,
	owned map[string]bool, depth int,
) (int64, int64, error) {
	if node == nil || depth > 256 || node.Source != source || !canonicalTransferPath(sourceRemote, source) ||
		!SafeName(node.Entry.Name) || node.Entry.Symlink || node.Entry.Size < 0 || node.Total < 0 || node.Done && node.Skipped {
		return 0, 0, locales.NewError(locales.MessageInvalidTransferManifestNode)
	}
	if node.Committing &&
		(node.Entry.Directory || node.Done || node.Skipped || node.Temporary == "") {
		return 0, 0, locales.NewError(locales.MessageInvalidTransferCommitPhase)
	}
	if node.Digest != "" {
		decoded, err := hex.DecodeString(node.Digest)
		if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != node.Digest || node.Entry.Directory {
			return 0, 0, locales.NewError(locales.MessageInvalidTransferContentBaseline)
		}
	}
	if node.Staged != nil && (node.Entry.Directory || node.Staged.Directory || node.Staged.Symlink ||
		node.Staged.Size != node.Entry.Size || node.Digest == "") {
		return 0, 0, locales.NewError(locales.MessageInvalidTransferStagingBaseline)
	}
	if checkpoint := node.Checkpoint; checkpoint != nil {
		decoded, err := hex.DecodeString(checkpoint.Digest)
		if node.Entry.Directory || node.Done || node.Skipped || node.Temporary == "" ||
			checkpoint.Offset < 0 || checkpoint.Offset > node.Entry.Size || err != nil ||
			len(decoded) != 32 || hex.EncodeToString(decoded) != checkpoint.Digest {
			return 0, 0, locales.NewError(locales.MessageInvalidTransferPrefixCheckpoint)
		}
	}
	if partial := node.Partial; partial != nil {
		if node.Checkpoint == nil || node.Entry.Directory || node.Done || node.Skipped ||
			node.Temporary == "" || partial.Directory || partial.Symlink ||
			partial.Size < node.Checkpoint.Offset || partial.Size > node.Entry.Size {
			return 0, 0, locales.NewError(locales.MessageInvalidTransferPartialFileBaseline)
		}
	}
	if node.Committing && (node.Staged == nil || node.Digest == "") {
		return 0, 0, locales.NewError(locales.MessageTheTransferCommitIsMissingAVerifiedBaseline)
	}
	targetRemote := !sourceRemote
	if node.Destination != "" {
		if !canonicalTransferPath(targetRemote, node.Destination) || Dir(targetRemote, node.Destination) != destinationParent ||
			!validTransferDestinationName(node.Entry.Name, Base(targetRemote, node.Destination), node.Entry.Directory) {
			return 0, 0, locales.NewError(locales.MessageTheTransferDestinationEscapesItsDirectory)
		}
	}
	if node.StageDirectory != "" {
		if node.Entry.Directory || node.Destination == "" || !validTransferTemporary(targetRemote, node.StageDirectory, node.Destination) || owned[node.StageDirectory] {
			return 0, 0, locales.NewError(locales.MessageThePrivateStagingDirectoryEscapesItsDestination)
		}
		owned[node.StageDirectory] = true
	}
	if node.Temporary != "" {
		valid := validTransferTemporary(targetRemote, node.Temporary, node.Destination)
		if node.StageDirectory != "" {
			valid = node.Temporary == Join(targetRemote, node.StageDirectory, "content")
		}
		if node.Entry.Directory || node.Destination == "" || node.Done || node.Skipped ||
			!valid || owned[node.Temporary] {
			return 0, 0, locales.NewError(locales.MessageInvalidTransferPartialFilePath)
		}
		owned[node.Temporary] = true
	}
	if node.Approved != nil && (node.Destination == "" || node.Approved.Symlink || node.Approved.Directory != node.Entry.Directory || node.Approved.Size < 0) {
		return 0, 0, locales.NewError(locales.MessageInvalidTransferConflictBaseline)
	}
	if !node.Entry.Directory {
		if len(node.Children) != 0 || node.Total != node.Entry.Size {
			return 0, 0, locales.NewError(locales.MessageTheFileManifestContainsChildNodesOrAnIncorrectSize)
		}
		if node.Skipped {
			return 0, 0, nil
		}
		if node.Done {
			if node.Destination == "" {
				return 0, 0, locales.NewError(locales.MessageACompletedFileHasNoDestinationPath)
			}
			return node.Total, node.Total, nil
		}
		return node.Total, 0, nil
	}
	if node.Done && node.Destination == "" {
		return 0, 0, locales.NewError(locales.MessageACompletedDirectoryHasNoDestinationPath)
	}
	if node.Skipped && node.Destination != "" {
		return 0, 0, locales.NewError(locales.MessageASkippedDirectoryAlreadyContainsDestinationState)
	}
	var original, total, done int64
	seen := map[string]bool{}
	for _, child := range node.Children {
		if child == nil || seen[child.Entry.Name] || !SafeName(child.Entry.Name) {
			return 0, 0, locales.NewError(locales.MessageTheTransferManifestContainsDuplicateOrInvalidChildNodes)
		}
		seen[child.Entry.Name] = true
		if node.Skipped && (child.Destination != "" || child.Done || child.Skipped) {
			return 0, 0, locales.NewError(locales.MessageASkippedDirectoryContainsProcessedChildren)
		}
		if child.Destination != "" && node.Destination == "" {
			return 0, 0, locales.NewError(locales.MessageAChildDestinationHasNoParentDirectory)
		}
		childTotal, childDone, err := validateTransferNode(child, sourceRemote, Join(sourceRemote, source, child.Entry.Name), node.Destination, owned, depth+1)
		if err != nil {
			return 0, 0, err
		}
		if child.Total > math.MaxInt64-original || childTotal > math.MaxInt64-total || childDone > math.MaxInt64-done {
			return 0, 0, locales.NewError(locales.MessageTransferManifestSizeOverflow)
		}
		original, total, done = original+child.Total, total+childTotal, done+childDone
		if node.Done && !child.Done && !child.Skipped {
			return 0, 0, locales.NewError(locales.MessageACompletedDirectoryStillHasIncompleteChildren)
		}
	}
	if original != node.Total {
		return 0, 0, locales.NewError(locales.MessageDirectoryManifestTotalSizesDoNotMatch)
	}
	if node.Skipped {
		if done != 0 {
			return 0, 0, locales.NewError(locales.MessageASkippedDirectoryContainsCommittedChildren)
		}
		return 0, 0, nil
	}
	return total, done, nil
}

// Restored MCP ownership is never inferred from a file tab or a display name.
func validMCPTransfer(owner *MCPOwner, task SavedTask) bool {
	if owner.CleanupOnly && (task.SourceRemote || !task.CleanupPending || task.State != Failed) {
		return false
	}
	return owner.KeyID != "" && owner.OperationID != "" && owner.ConnectionID != "" &&
		owner.Identity != "" && filepath.IsAbs(owner.LocalRoot) && !owner.CreatedAt.IsZero() &&
		(task.SourceRemote || task.SourcePath != "")
}

func (task *Task[S]) Snapshot(sessionID int) SavedTask {
	job := task.Job
	var manifest *Manifest
	if task.Run != nil {
		manifest = task.Run.Manifest.Load()
	} else {
		manifest = SnapshotManifest(job)
	}
	entry := task.Entry
	if manifest.Root != nil && manifest.Root.Destination != "" {
		entry.Name = Base(!job.SourceRemote, manifest.Root.Destination)
	}
	state := task.State
	if task.CleanupPending || task.RemoveRequested {
		state = Failed
	}
	stored := SavedTask{ID: task.ID, Session: sessionID, Batch: job.Batch.ID, MCP: task.MCP,
		Entry: entry,
		State: state, Transferred: task.Transferred, Started: task.Started, Completed: task.Completed,
		Error: task.ErrorText.String(), CleanupPending: task.CleanupPending || task.RemoveRequested,
		SourceRemote: job.SourceRemote, SourcePath: manifest.SourcePath, TargetPath: manifest.TargetPath,
		Name: job.Name, Policy: int(job.Batch.Policy.Load()), Total: manifest.Total, DoneBytes: manifest.Done,
		Root: manifest.Root}
	stored.Temporary = append(stored.Temporary, manifest.Temporary...)
	slices.Sort(stored.Temporary)
	return stored
}

func RestoreTask[S any](stored SavedTask, batch *Batch, session, view *S) *Task[S] {
	job := &Job{SourceRemote: stored.SourceRemote, SourcePath: stored.SourcePath,
		TargetPath: stored.TargetPath, Name: stored.Name, Batch: batch,
		Root: RestoreNode(stored.Root), Total: stored.Total, DoneBytes: stored.DoneBytes,
		Temporary: map[string]struct{}{}}
	for _, name := range stored.Temporary {
		job.Temporary[name] = struct{}{}
	}
	entry := stored.Entry
	state := stored.State
	if state != Done && state != Failed {
		state = Paused
	}
	return &Task[S]{ID: stored.ID, Session: session, View: view, MCP: stored.MCP,
		Entry:        entry,
		SourceRemote: stored.SourceRemote, SourcePath: stored.SourcePath, TargetPath: stored.TargetPath,
		State: state, Transferred: stored.Transferred, Total: stored.Total, Started: stored.Started,
		Completed: stored.Completed, ErrorText: locales.Raw(
			stored.Error,
		), CleanupPending: stored.CleanupPending, Job: job, Persisted: true}
}
