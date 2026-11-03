-- Version one is a fresh application model. Legacy JSON files are not read.
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL CHECK(json_valid(value))) STRICT;
CREATE TABLE managed_keys (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, kind TEXT NOT NULL, fingerprint TEXT NOT NULL,
 path TEXT NOT NULL UNIQUE, comment TEXT NOT NULL, bits INTEGER NOT NULL, created_at TEXT NOT NULL,
 public_key BLOB NOT NULL CHECK(length(public_key)>0)
) STRICT;
CREATE TABLE catalog_groups (
 id TEXT PRIMARY KEY, scope TEXT NOT NULL CHECK(scope IN ('ssh','files')),
 name TEXT NOT NULL, position INTEGER NOT NULL CHECK(position>=0), collapsed INTEGER NOT NULL CHECK(collapsed IN (0,1)),
 UNIQUE(scope,name)
) STRICT;
CREATE TABLE profiles (
 id TEXT PRIMARY KEY, group_id TEXT NOT NULL REFERENCES catalog_groups(id) DEFERRABLE INITIALLY DEFERRED,
 position INTEGER NOT NULL CHECK(position>=0), key_id TEXT REFERENCES managed_keys(id) DEFERRABLE INITIALLY DEFERRED,
 jump_id TEXT REFERENCES profiles(id) DEFERRABLE INITIALLY DEFERRED,
 value TEXT NOT NULL CHECK(json_valid(value)), UNIQUE(group_id,position)
) STRICT;
CREATE TABLE endpoints (
 id TEXT PRIMARY KEY, key_id TEXT REFERENCES managed_keys(id) DEFERRABLE INITIALLY DEFERRED,
 value TEXT NOT NULL CHECK(json_valid(value))
) STRICT;
CREATE TABLE workspace (
 id INTEGER PRIMARY KEY CHECK(id=1), version INTEGER NOT NULL CHECK(version=1),
 layout TEXT NOT NULL CHECK(json_valid(layout))
) STRICT;
CREATE TABLE workspace_endpoints (
 endpoint_id TEXT PRIMARY KEY REFERENCES endpoints(id)
) STRICT;
CREATE TABLE transfer_batches (id INTEGER PRIMARY KEY, policy INTEGER NOT NULL CHECK(policy BETWEEN 0 AND 3)) STRICT;
CREATE TABLE transfer_sessions (
 id INTEGER PRIMARY KEY, value TEXT NOT NULL CHECK(json_valid(value))
) STRICT;
CREATE TABLE transfer_connections (
 id INTEGER PRIMARY KEY, value TEXT NOT NULL CHECK(json_valid(value))
) STRICT;
CREATE TABLE transfer_tasks (
 id INTEGER PRIMARY KEY, batch_id INTEGER NOT NULL REFERENCES transfer_batches(id) DEFERRABLE INITIALLY DEFERRED,
 session_id INTEGER NOT NULL REFERENCES transfer_sessions(id) DEFERRABLE INITIALLY DEFERRED,
 value TEXT NOT NULL CHECK(json_valid(value))
) STRICT;
CREATE INDEX transfer_tasks_batch ON transfer_tasks(batch_id);
CREATE INDEX transfer_tasks_session ON transfer_tasks(session_id);
CREATE TABLE transfer_endpoints (endpoint_id TEXT PRIMARY KEY REFERENCES endpoints(id)) STRICT;
CREATE TABLE transfer_state (id INTEGER PRIMARY KEY CHECK(id=1), next_id INTEGER NOT NULL, next_batch_id INTEGER NOT NULL) STRICT;
CREATE TABLE themes (name TEXT PRIMARY KEY, filename TEXT NOT NULL UNIQUE, source BLOB NOT NULL) STRICT;
CREATE TABLE host_keys (
 host TEXT NOT NULL, port INTEGER NOT NULL CHECK(port BETWEEN 1 AND 65535), algorithm TEXT NOT NULL,
 public_key BLOB NOT NULL, revoked INTEGER NOT NULL DEFAULT 0 CHECK(revoked IN (0,1)),
 created_at TEXT NOT NULL, PRIMARY KEY(host,port,algorithm,public_key)
) STRICT;
CREATE INDEX revoked_host_keys ON host_keys(public_key) WHERE revoked=1;
PRAGMA application_id=1112692529;
PRAGMA user_version=1;
