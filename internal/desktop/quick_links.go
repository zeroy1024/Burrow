package desktop

import (
	"fmt"
	"slices"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

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
		a.startFileSession(target, false)
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
	uikit.Modal(c.Key("ftp-ssh-link"), &open, uikit.ModalProps{
		Title: "选择 SSH 快捷目标", Width: t.C.Dialog.HostWidth, Compact: true,
		Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func(parts uikit.ModalParts) {
		ui.Column(c).FillWidth().Gap(t.Space.N12).Children(func() {
			uikit.Text(c, "FTP 使用独立登录。选择已有 SSH 主机后，本次会话可双向跳转。", uikit.TextProps{Style: uikit.TextSmall})
			scroll := ui.Local(parts.Panel, "ssh-link-scroll", func() ui.ScrollState { return ui.ScrollState{} })
			list := ui.Scroll(c).MaxHeight(t.Size.StatsPanel).FillWidth().TrackScroll(scroll).Children(func() {
				ui.Column(c).FillWidth().Gap(t.Space.N6).Children(func() {
					for _, group := range a.groups {
						if group.scope != sectionSSH {
							continue
						}
						for _, id := range group.hosts {
							h := a.hosts[id]
							uikit.Button(c, fmt.Sprintf("%s · %s@%s:%d", h.name, h.user, h.address, h.port), uikit.ButtonProps{}).Key(id).FillWidth().OnClick(func() {
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
			uikit.Scrollbars(c, parts.Panel, list, scroll, uikit.ScrollbarProps{Label: "SSH 快捷目标滚动条"})
			uikit.Button(c, "取消", uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
		})
	})
	if !open {
		a.fileSSHLink = nil
	}
}

func sameSSHIdentity(left, right host) bool {
	return left.address == right.address && left.port == right.port && left.user == right.user &&
		left.session.Auth == right.session.Auth && left.session.KeyID == right.session.KeyID && sameSSHJump(left, right)
}

func sameSSHJump(left, right host) bool {
	jump := func(h host) string {
		if h.session.SSH != nil {
			return h.session.SSH.JumpHostID
		}
		return ""
	}
	return jump(left) == jump(right)
}
