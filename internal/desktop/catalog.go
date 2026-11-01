package desktop

import "slices"

type hostGroup struct {
	id        int
	name      string
	hosts     []hostID
	collapsed bool
}

type catalogData struct {
	hosts       map[hostID]host
	groups      []*hostGroup
	nextHostID  hostID
	nextGroupID int
	localPaths  map[hostID]string
}

func demoCatalog() catalogData {
	values := demoHosts()
	groups := demoGroups()
	catalog := catalogData{
		hosts: make(map[hostID]host, len(values)), groups: groups,
		nextHostID: hostID(len(values)), nextGroupID: len(groups), localPaths: map[hostID]string{},
	}
	for id, value := range values {
		value.auth = "agent"
		catalog.hosts[hostID(id)] = value
	}
	return catalog
}

func (a *app) catalog() *catalogData {
	if a.section == sectionFiles {
		return &a.fileCatalog
	}
	return &a.sshCatalog
}

func (a *app) groupCollapsed(group *hostGroup) bool { return group.collapsed }

func (a *app) setGroupCollapsed(group *hostGroup, collapsed bool) { group.collapsed = collapsed }

func (catalog *catalogData) removeHost(id hostID) {
	delete(catalog.hosts, id)
	delete(catalog.localPaths, id)
	for _, group := range catalog.groups {
		group.hosts = slices.DeleteFunc(group.hosts, func(candidate hostID) bool { return candidate == id })
	}
}

type hostDrag struct {
	id    hostID
	group *hostGroup
}

func demoGroups() []*hostGroup {
	return []*hostGroup{
		{id: 0, name: "生产环境", hosts: []hostID{0, 1, 2, 3}},
		{id: 1, name: "测试环境", hosts: []hostID{4, 5, 6}},
		{id: 2, name: "个人", hosts: []hostID{7, 8}},
	}
}

func (a *app) groupNames() []string {
	names := make([]string, len(a.catalog().groups))
	for i, group := range a.catalog().groups {
		names[i] = group.name
	}
	return names
}

func (a *app) moveGroup(source *hostGroup, position int) {
	catalog := a.catalog()
	from := slices.Index(catalog.groups, source)
	catalog.groups = slices.Delete(catalog.groups, from, from+1)
	catalog.groups = slices.Insert(catalog.groups, position, source)
}

// position is the destination index after removing the source from its group.
func moveHost(drag hostDrag, destination *hostGroup, position int) {
	from := slices.Index(drag.group.hosts, drag.id)
	drag.group.hosts = slices.Delete(drag.group.hosts, from, from+1)
	destination.hosts = slices.Insert(destination.hosts, position, drag.id)
}
