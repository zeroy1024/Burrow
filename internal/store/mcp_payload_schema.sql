CREATE TABLE mcp_payloads (
 operation_id TEXT PRIMARY KEY REFERENCES mcp_operations(id) ON DELETE CASCADE,
 request TEXT CHECK(request IS NULL OR json_valid(request)),
 response TEXT CHECK(response IS NULL OR json_valid(response))
) STRICT;
PRAGMA user_version=3;
