CREATE TABLE mcp_keys (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, prefix TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('active','disabled','revoked')),
 digest BLOB NOT NULL UNIQUE CHECK(length(digest)=32),
 created_at TEXT NOT NULL, last_used_at TEXT NOT NULL,
 grants TEXT NOT NULL CHECK(json_valid(grants)),
 upload_dirs TEXT NOT NULL CHECK(json_valid(upload_dirs)),
 download_dirs TEXT NOT NULL CHECK(json_valid(download_dirs))
) STRICT;
CREATE TABLE mcp_policies (
 id TEXT PRIMARY KEY, version INTEGER NOT NULL CHECK(version>0),
 enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),
 category TEXT NOT NULL CHECK(category IN ('ssh','files')),
 value TEXT NOT NULL CHECK(json_valid(value))
) STRICT;
CREATE TABLE mcp_operations (
 id TEXT PRIMARY KEY, key_id TEXT NOT NULL, key_name TEXT NOT NULL,
 connection_id TEXT NOT NULL, action TEXT NOT NULL, status TEXT NOT NULL,
 error_code TEXT NOT NULL, error TEXT NOT NULL,
 started_at TEXT NOT NULL, finished_at TEXT NOT NULL,
 target TEXT NOT NULL CHECK(json_valid(target)),
 request TEXT NOT NULL CHECK(json_valid(request)),
 result TEXT NOT NULL CHECK(json_valid(result)),
 hits TEXT NOT NULL CHECK(json_valid(hits)),
 timeline TEXT NOT NULL CHECK(json_valid(timeline)),
 task_id INTEGER NOT NULL DEFAULT 0 CHECK(task_id>=0)
) STRICT;
CREATE INDEX mcp_operations_time ON mcp_operations(started_at DESC,id);
CREATE INDEX mcp_operations_key ON mcp_operations(key_id,started_at DESC);
CREATE INDEX mcp_operations_connection ON mcp_operations(connection_id,started_at DESC);
CREATE TABLE mcp_tasks (
 id INTEGER PRIMARY KEY CHECK(id>0),
 operation_id TEXT NOT NULL UNIQUE REFERENCES mcp_operations(id) ON DELETE CASCADE,
 key_id TEXT NOT NULL, connection_id TEXT NOT NULL,
 state TEXT NOT NULL, error TEXT NOT NULL,
 done INTEGER NOT NULL CHECK(done>=0), total INTEGER NOT NULL CHECK(total>=0),
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL
) STRICT;
CREATE INDEX mcp_tasks_key ON mcp_tasks(key_id,id);
CREATE TABLE mcp_files (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 operation_id TEXT NOT NULL REFERENCES mcp_operations(id) ON DELETE CASCADE,
 path TEXT NOT NULL, side TEXT NOT NULL, action TEXT NOT NULL,
 status TEXT NOT NULL, error TEXT NOT NULL, bytes INTEGER NOT NULL CHECK(bytes>=0),
 at TEXT NOT NULL
) STRICT;
CREATE INDEX mcp_files_operation ON mcp_files(operation_id,id);
PRAGMA user_version=2;
