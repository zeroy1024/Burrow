# File management and transfers

[English](file-transfer.md) | [简体中文](file-transfer.zh.md)

## Connect and browse

The file page has local and remote panes. Remote connections support SFTP, FTP, and FTPS. Sites and groups are independent of the SSH catalog. One site can have several file tabs.

Navigate using the path bar, history, and directory entries. Filter, sort, preview metadata, create directories, rename, and delete entries. After editing a saved site's initial directory, an existing tab applies that change on explicit reconnect. If the initial directory is unchanged, it retains the current browsing location.

FTP supports no TLS, explicit TLS, implicit TLS, and active/passive mode. TLS requires a trusted certificate; active mode requires the server to connect back to this computer. Filename encodings include UTF-8, GBK, GB18030, Big5, Shift_JIS, EUC-KR, and ISO-8859-1. This option does not convert file contents.

## Upload and download

Drag files or directories between panes, or select sources with the file picker. The transfer panel shows the current tab's tasks, actual bytes, speed, and progress, with pause, resume, cancel, and retry actions.

Directory transfers first build a real recursive manifest. Empty directories and zero-byte files do not fabricate transferred bytes. Symbolic links and special-file contents are excluded. Directory links can be entered explicitly, but recursive transfers and deletion do not follow links to their targets.

## Name conflicts

Choose Ask, Overwrite, Skip, or Keep both. Directory overwrite merges entries individually. A file/directory type mismatch cannot be overwritten directly.

**Apply this choice to all conflicts** affects the same batch and its recursive children. Cancelling conflict resolution ends that batch's unfinished tasks while retaining completed files. Other batches continue.

Writes use a task-owned `.part` staging file and are marked complete only after closing the contents and committing the destination successfully. Missing resume, permission, or replacement capabilities produce an error; the original destination is not deleted in advance. FTP cannot provide an atomic conditional rename against other clients; see [known limitations](limitations.md).

Windows local permissions are shown as ACLs rather than Unix mode bits. New staging files receive private ACLs at creation. Before overwriting a local file, its DACL is copied and verified. No-overwrite commits use native rename associated with an open handle. File sharing, cross-volume, or permission failures remain explicit. Unsupported Windows filenames, including reserved names and trailing dots/spaces, are rejected in the manifest rather than silently renamed or rewritten.

## Pause, resume, and cancel

| Action | Behavior |
|---|---|
| Pause | Stop current I/O; retain the manifest, partial files, and confirmed checkpoint |
| Resume | Verify the source baseline and destination partial content, then continue at the confirmed offset; directories also verify completed children |
| Cancel | Clean up partial files owned by the task; retain committed files; report cleanup failures and allow retry |
| Retry | Rerun a failed task under the same endpoint, conflict, and content-verification rules |

On disconnection, the server may have completed a rename without returning its result. Recovery checks staging files, final destinations, and digests. If the result remains uncertain, it reports that uncertainty instead of retransmitting blindly and creating duplicate copies.

## Tabs and background tasks

Queues are isolated by file tab. Another tab for the same site starts with an empty queue. Switching pages/tabs or closing a browsing tab does not cancel background tasks. Offline tabs can still display their own tasks.

Tasks retain the endpoint, jump route, and credential version captured when queued. Editing a saved site does not redirect existing tasks. If an old credential is unavailable, the app asks for the credential required by that task.

When tasks have no associated open tab, **Recover tasks** reopens an offline tab using the original identity. It only associates and displays tasks; it does not log in or resume them automatically.

## Quit and restore

Manifests, confirmed offsets, and digests are saved in SQLite during operation. New tasks are saved before starting, and commit intent is saved before final rename. Normal shutdown pauses active tasks, waits for workers to stop, and saves final checkpoints. If startup restoration is enabled and the original file tab is restored, tasks reassociate with it offline. Resume them manually.

With startup restoration disabled, new tabs do not inherit old tasks. Old checkpoints without tab identities remain available but are not assigned by guessing a hostname or address. Use **Recover tasks**. Forced termination or power loss does not guarantee that the latest progress was saved.

See [data and credentials](storage.md) for files and format versions, and [architecture and lifecycle](architecture.md) for connection and commit policies.
