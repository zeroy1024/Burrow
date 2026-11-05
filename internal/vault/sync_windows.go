package vault

// File.Sync flushes the new key before the database can reference it.
// Opening a directory for FlushFileBuffers is not supported on Windows.
func syncDirectory(string) error { return nil }
