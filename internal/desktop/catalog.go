package desktop

import "slices"

type hostGroup struct {
	id        int
	scope     section
	name      string
	hosts     []hostID
	collapsed bool
}

func (a *app) groupCollapsed(group *hostGroup) bool {
	return group.collapsed
}

func (a *app) setGroupCollapsed(group *hostGroup, collapsed bool) {
	group.collapsed = collapsed
}

type hostDrag struct {
	id    hostID
	group *hostGroup
}

func demoGroups() []*hostGroup {
	return []*hostGroup{
		{id: 0, scope: sectionSSH, name: "生产环境", hosts: []hostID{0, 1, 2, 3}},
		{id: 1, scope: sectionSSH, name: "测试环境", hosts: []hostID{4, 5, 6}},
		{id: 2, scope: sectionSSH, name: "个人", hosts: []hostID{7, 8}},
		{id: 3, scope: sectionFiles, name: "生产环境", hosts: []hostID{}},
		{id: 4, scope: sectionFiles, name: "测试环境", hosts: []hostID{}},
		{id: 5, scope: sectionFiles, name: "个人", hosts: []hostID{}},
	}
}

func (a *app) catalogGroups() []*hostGroup {
	var groups []*hostGroup
	for _, group := range a.groups {
		if group.scope == a.section {
			groups = append(groups, group)
		}
	}
	return groups
}

func (a *app) newCatalogGroup(name string) *hostGroup {
	return &hostGroup{id: len(a.groups), scope: a.section, name: name}
}

func (a *app) groupNames() []string {
	groups := a.catalogGroups()
	names := make([]string, len(groups))
	for i, group := range groups {
		names[i] = group.name
	}
	return names
}

func (a *app) catalogModeMatches(h host) bool {
	if a.section == sectionFiles {
		return h.protocol == "sftp" || h.protocol == "ftp"
	}
	return a.section == sectionSSH && h.protocol == ""
}

func (a *app) catalogModeCount(ids []hostID) int {
	count := 0
	for _, id := range ids {
		if a.catalogModeMatches(a.hosts[id]) {
			count++
		}
	}
	return count
}

func (a *app) moveGroup(source *hostGroup, position int) {
	groups := a.catalogGroups()
	from := slices.Index(groups, source)
	groups = slices.Delete(groups, from, from+1)
	groups = slices.Insert(groups, position, source)
	visible := 0
	for i, group := range a.groups {
		if group.scope == a.section {
			a.groups[i] = groups[visible]
			visible++
		}
	}
}

// position is the destination index after removing the source from its group.
func moveHost(drag hostDrag, destination *hostGroup, position int) {
	from := slices.Index(drag.group.hosts, drag.id)
	drag.group.hosts = slices.Delete(drag.group.hosts, from, from+1)
	destination.hosts = slices.Insert(destination.hosts, position, drag.id)
}
