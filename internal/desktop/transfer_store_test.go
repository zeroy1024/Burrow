package desktop

import (
	"strings"
	"testing"

	"sshdesk/internal/fileclient"
)

func TestValidateTransferNodePrivateDirectory(t *testing.T) {
	stage := "/target/.sshdesk-" + strings.Repeat("a", 32) + ".part"
	for _, tt := range []struct {
		name               string
		directory, content string
		valid              bool
	}{
		{"owned private directory and content", stage, stage + "/content", true},
		{"directory outside destination", "/other/.sshdesk-" + strings.Repeat("a", 32) + ".part", stage + "/content", false},
		{"content outside private directory", stage, "/target/other", false},
		{"content traverses private directory", stage, stage + "/../other", false},
		{"unowned directory name", "/target/saved-files", "/target/saved-files/content", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			node := &storedTransferNode{Source: "/source/data.txt", Destination: "/target/data.txt",
				StageDirectory: tt.directory, Temporary: tt.content, Entry: fileclient.Entry{Name: "data.txt", Size: 4}, Total: 4}
			owned := make(map[string]bool)
			_, _, err := validateTransferNode(node, false, node.Source, "/target", owned, 0)
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v: %v", tt.valid, err)
			}
			if tt.valid && (len(owned) != 2 || !owned[tt.directory] || !owned[tt.content]) {
				t.Fatal("private directory and content must both belong to the recoverable task")
			}
		})
	}
}
