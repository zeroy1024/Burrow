# SSH Desk SFTP creation-permission extension

This local module contains the production sources and required internal packages
from `github.com/pkg/sftp v1.13.12`. The module identity and dependency versions
are unchanged. The upstream BSD license and contributors are retained in
`LICENSE` and `CONTRIBUTORS`; tests, fixtures, fuzz targets and developer tools
were not copied.

The only functional patch is `(*Client).OpenFileMode(path, flags, perm)` in
`client.go`. It includes the permission attribute in the same `SSH_FXP_OPEN`
request that creates a file. The existing `OpenFile`, `Open` and `Create` methods
keep their original request attributes and creation policy.

SSH Desk uses `OpenFileMode` with `O_CREATE|O_EXCL` and `0600` for private remote
staging files. This avoids an initially public empty file that another account
could open before a later chmod. The application verifies the returned handle's
permissions before sending file contents and rejects servers that do not honor
the requested restriction.
