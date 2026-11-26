package workspace

import (
	"encoding/json"
	"testing"
)

func TestRestoreVersionTwoSplit(t *testing.T) {
	const original = `{"version":2,"tabs":[{"root":{"first":{"session":{"profileId":"a","endpoint":{"name":"A","host":"a","port":22,"id":"a","auth":"agent"}}},"second":{"session":{"profileId":"b","endpoint":{"name":"B","host":"b","port":22,"id":"b","auth":"agent"}}},"stacked":true,"ratio":0.4},"focused":1,"maximized":true}],"section":0,"activeSSH":0,"activeFile":0}`
	var saved Snapshot
	if err := json.Unmarshal([]byte(original), &saved); err != nil {
		t.Fatal(err)
	}
	if err := saved.Validate(nil); err != nil {
		t.Fatal(err)
	}
	tab := RestoreTab(saved.Tabs[0], func(s *Session) *string { return &s.ProfileID })
	if *tab.Focused != "b" || !tab.Maximized || !tab.Root.Stacked || tab.Root.Ratio != .4 {
		t.Fatalf("lost saved split or focus: %+v", tab)
	}
	tab.Root = tab.Root.Remove(tab.Root.FirstSession())
	roundTrip := SaveTab(tab, func(id *string) *Session { return &Session{ProfileID: *id} })
	if roundTrip.Root.Session.ProfileID != "b" || roundTrip.Focused != 0 {
		t.Fatalf("unexpected focus after removing first pane: %+v", roundTrip)
	}
}
