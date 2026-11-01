package desktop

import (
	"fmt"
	"slices"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func (a *app) activateSSHView(session *remoteSession) bool {
	if session.closing {
		return false
	}
	for i, tab := range a.tabs {
		if slices.Contains(tab.root.sessions(), session) {
			a.switchSection(sectionSSH)
			a.activateTab(i)
			a.focusSession(tab, session)
			if session.state != connected && session.state != connecting {
				a.connectSession(session)
			}
			return true
		}
	}
	return false
}

func (a *app) openFilesForSSH(source *remoteSession) {
	target := source.filePeer
	if target == nil {
		for _, candidate := range a.fileSessions {
			if candidate.connection == source.connection && candidate.endpoint.protocol == "sftp" {
				target = candidate
				break
			}
		}
	}
	if target == nil {
		endpoint := source.endpoint
		endpoint.protocol = "sftp"
		target = a.newFileSession(-1, endpoint, true, source.connection)
	}
	source.filePeer, target.sshPeer = target, source
	index := slices.Index(a.fileSessions, target)
	if index < 0 {
		a.fileSessions = append(a.fileSessions, target)
		index = len(a.fileSessions) - 1
		a.reconnectFileSession(target)
	} else if target.state != connected && target.state != connecting {
		a.reconnectFileSession(target)
	}
	a.switchSection(sectionFiles)
	a.activeFile = index
}

func (a *app) openSSHForFiles(source *fileSession) {
	if source.sshPeer != nil {
		source.sshPeer.filePeer = source
		if a.activateSSHView(source.sshPeer) {
			return
		}
	}
	if source.endpoint.protocol == "ftp" {
		if previous := source.sshPeer; previous != nil {
			a.openLinkedShell(source, previous.host, previous.endpoint, previous.temporary, previous.connection)
			return
		}
		// FTP virtual users and ports do not identify an SSH account.
		a.fileSSHLink = source
		return
	}
	for _, tab := range a.tabs {
		for _, candidate := range tab.root.sessions() {
			if candidate.connection == source.connection && !candidate.closing {
				source.sshPeer, candidate.filePeer = candidate, source
				a.activateSSHView(candidate)
				return
			}
		}
	}
	a.openLinkedShell(source, -1, source.endpoint, true, source.connection)
}

func (a *app) openLinkedShell(source *fileSession, profile hostID, endpoint host, temporary bool, connection *sshConnectionGroup) {
	session := a.newShellSession(profile, endpoint, temporary, connection)
	a.nextTab++
	a.tabs = append(a.tabs, &workspaceTab{id: a.nextTab, root: &terminalLayout{session: session}, focused: session})
	source.sshPeer, session.filePeer = session, source
	a.switchSection(sectionSSH)
	a.activateTab(len(a.tabs) - 1)
}

func (a *app) fileSSHLinkDialog(c *ui.Context) {
	source := a.fileSSHLink
	if source == nil {
		return
	}
	open := true
	t := theme.Current()
	modal(c.Key("ftp-ssh-link"), &open, func() {
		ui.Column(c).Width(t.C.Dialog.HostWidth).Gap(t.Space.N12).Children(func() {
			ui.Text(c, "选择 SSH 快捷目标").FontSize(t.C.Dialog.TitleSize).FontWeight(t.Font.Weight.Semibold)
			ui.Text(c, "FTP 使用独立登录。选择已有 SSH 主机后，本次会话可通过按钮双向跳转。").FontSize(t.Font.Size.Small)
			ui.Scroll(c).MaxHeight(t.Size.StatsPanel).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(t.Space.N6).Children(func() {
					for _, group := range a.sshCatalog.groups {
						for _, id := range group.hosts {
							h := a.sshCatalog.hosts[id]
							button(c, fmt.Sprintf("%s · %s@%s:%d", h.name, h.user, h.address, h.port)).Key(id).FillWidth().OnClick(func() {
								for _, tab := range a.tabs {
									for _, candidate := range tab.root.sessions() {
										if !candidate.temporary && !candidate.closing && candidate.host == id && sameSSHIdentity(candidate.endpoint, h) {
											source.sshPeer, candidate.filePeer = candidate, source
											a.fileSSHLink = nil
											a.activateSSHView(candidate)
											return
										}
									}
								}
								a.openHost(id)
								session := a.currentTab().focused
								source.sshPeer, session.filePeer = session, source
								a.fileSSHLink = nil
								a.switchSection(sectionSSH)
							})
						}
					}
				})
			})
			button(c, "取消").OnClick(func() { a.fileSSHLink = nil })
		})
	})
	if !open {
		a.fileSSHLink = nil
	}
}

func sameSSHIdentity(left, right host) bool {
	return left.address == right.address && left.port == right.port && left.user == right.user &&
		left.auth == right.auth && (left.auth != "key" || left.keyPath == right.keyPath)
}
