CREATE TABLE vault_meta (
 id INTEGER PRIMARY KEY CHECK(id=1),
 state TEXT NOT NULL CHECK(state IN ('pending_init','pending_migration','ready')),
 value TEXT NOT NULL CHECK(json_valid(value))
) STRICT;
CREATE TABLE vault_items (
 secret_id TEXT PRIMARY KEY,
 kind TEXT NOT NULL CHECK(kind IN ('session_password','private_key','key_passphrase')),
 revision TEXT NOT NULL,
 crypto_epoch INTEGER NOT NULL CHECK(crypto_epoch>0),
 nonce BLOB NOT NULL CHECK(length(nonce)=24),
 ciphertext BLOB NOT NULL CHECK(length(ciphertext)>=16)
) STRICT;
ALTER TABLE managed_keys RENAME COLUMN path TO secret_id;
PRAGMA user_version=4;
