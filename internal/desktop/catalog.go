package desktop

import "slices"

type hostGroup struct {
	id        int
	name      string
	hosts     []hostID
	collapsed bool
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
	names := make([]string, len(a.groups))
	for i, group := range a.groups {
		names[i] = group.name
	}
	return names
}

func (a *app) moveGroup(source, destination *hostGroup, after bool) {
	if source == destination {
		return
	}
	from := slices.Index(a.groups, source)
	a.groups = slices.Delete(a.groups, from, from+1)
	to := slices.Index(a.groups, destination)
	if after {
		to++
	}
	a.groups = slices.Insert(a.groups, to, source)
}

func moveHost(drag hostDrag, destination *hostGroup, position int) {
	from := slices.Index(drag.group.hosts, drag.id)
	if drag.group == destination && from < position {
		position--
	}
	drag.group.hosts = slices.Delete(drag.group.hosts, from, from+1)
	destination.hosts = slices.Insert(destination.hosts, position, drag.id)
}
