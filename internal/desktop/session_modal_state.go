package desktop

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/uikit"
)

type hostDraft struct {
	afterClose                                              func()
	name, address, user, port, group, system                string
	protocol, auth, keyID, password, passphrase             string
	jumpID, keepalive, initCommand, termScheme              string
	remoteDir, localDir, encoding, ftpMode, ftpTLS          string
	env                                                     []string
	envInput, newGroup                                      string
	id                                                      hostID
	editing, deleting, files, portEdited                    bool
	passwordChanged, passphraseChanged, passwordSaved       bool
	advanced, entered, closing, selectedName, creatingGroup bool
	closeStarted                                            time.Time
	errors                                                  map[string]string
	focused                                                 map[string]bool
	handles                                                 map[string]*ui.Handle
	scroll                                                  ui.ScrollState
	advancedHeight                                          float32
	testCancel                                              context.CancelFunc
	testResults                                             <-chan connectionTestResult
	testResult                                              *connectionTestResult
	testVisible                                             bool
	choosingDirectory                                       bool
	listedAgent                                             bool
	agentCount                                              int
	agentError                                              error
	submitted                                               bool
	saving                                                  bool
	agentResults                                            <-chan sessionAgentResult
	generation                                              string
	initialValues                                           [20]string
	initialEnv                                              []string
	initialPortEdited                                       bool
}

type sessionAgentResult struct {
	count int
	err   error
}

var environmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=.*$`)
var hostnamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$`)

func (a *app) startHostDraft() {
	page := a.page()
	page.draft.cancelTest()
	protocol := "SSH"
	if a.section == sectionFiles {
		protocol = "SFTP"
	}
	page.draft = hostDraft{
		port: "22", protocol: protocol, files: a.section == sectionFiles,
		auth: "password", keepalive: "30", encoding: "UTF-8", ftpMode: "passive", ftpTLS: "explicit",
		group: a.catalogGroups()[0].name, advanced: a.modalAdvancedOpen(strings.ToLower(protocol)),
	}
	if page.lastGroup != nil {
		page.draft.group = page.lastGroup.name
	}
	page.draft.initState()
	page.addHostOpen = true
}

func (a *app) startEditHostDraft(id hostID) {
	a.closeSettings()
	h := a.hosts[id]
	page := a.page()
	page.draft.cancelTest()
	protocol := "SSH"
	if h.protocol != "" && h.protocol != "ssh" {
		protocol = "SFTP"
		if h.protocol == "ftp" {
			protocol = "FTP"
		}
	}
	d := hostDraft{
		id: id, editing: true, files: h.protocol != "" && h.protocol != "ssh", protocol: protocol,
		name: h.name, address: h.address, user: h.user, port: strconv.Itoa(h.port),
		system: systems[h.os].name, auth: h.session.Auth, keyID: h.session.KeyID,
		passwordSaved: h.session.PasswordSaved, keepalive: "30", encoding: "UTF-8", ftpMode: "passive", ftpTLS: "explicit",
	}
	if d.auth == "" {
		d.auth = "password"
	}
	for _, group := range a.catalogGroups() {
		if slices.Contains(group.hosts, id) {
			d.group = group.name
			break
		}
	}
	if p := h.session.SSH; p != nil && !d.files {
		d.jumpID, d.keepalive, d.initCommand, d.termScheme = p.JumpHostID, strconv.Itoa(p.KeepAlive), p.InitCommand, p.TermScheme
		d.env = slices.Clone(p.Env)
	}
	if p := h.session.Files; p != nil && d.files {
		d.remoteDir, d.localDir, d.encoding = p.RemoteDir, p.LocalDir, p.Encoding
		if d.encoding == "" {
			d.encoding = "UTF-8"
		}
		if p.FTPMode != "" {
			d.ftpMode = p.FTPMode
		}
		if p.FTPTLS != "" {
			d.ftpTLS = p.FTPTLS
		}
	}
	d.portEdited = h.session.PortEdited || h.port != d.defaultPort()
	d.advanced = a.modalAdvancedOpen(strings.ToLower(protocol)) || d.hasAdvanced()
	d.initState()
	page.draft, page.addHostOpen = d, true
}

func (d *hostDraft) initState() {
	d.errors, d.focused, d.handles = map[string]string{}, map[string]bool{}, map[string]*ui.Handle{}
	d.generation = newSessionID()
	d.initialValues, d.initialEnv, d.initialPortEdited = d.inputValues(), slices.Clone(d.env), d.portEdited
}

func (d *hostDraft) inputValues() [20]string {
	return [20]string{d.name, d.address, d.user, d.port, d.group, d.system,
		d.protocol, d.auth, d.keyID, d.jumpID, d.keepalive, d.initCommand, d.termScheme,
		d.remoteDir, d.localDir, d.encoding, d.ftpMode, d.ftpTLS, d.envInput, d.newGroup}
}

func (d *hostDraft) dirty() bool {
	return d.inputValues() != d.initialValues || !slices.Equal(d.env, d.initialEnv) ||
		d.portEdited != d.initialPortEdited || d.passwordChanged || d.passphraseChanged
}

func (d *hostDraft) defaultPort() int {
	if d.protocol == "FTP" {
		if d.ftpTLS == "implicit" {
			return 990
		}
		return 21
	}
	return 22
}

func (d *hostDraft) cancelTest() {
	if d.testCancel != nil {
		d.testCancel()
		d.testCancel = nil
	}
	d.testResults = nil
}

func (d *hostDraft) changed(field string) {
	d.cancelTest()
	d.testVisible = false
	delete(d.errors, field)
}

func (d *hostDraft) changeProtocol(protocol string) {
	d.protocol = protocol
	if !d.portEdited {
		d.port = strconv.Itoa(d.defaultPort())
	}
	if protocol == "FTP" && (d.auth == "key" || d.auth == "agent") || protocol != "FTP" && d.auth == "anonymous" {
		d.auth = "password"
	}
	d.changed("protocol")
}

func (d *hostDraft) hasAdvanced() bool {
	if d.files {
		return d.remoteDir != "" || d.localDir != "" || d.encoding != "UTF-8" || d.protocol == "FTP" && (d.ftpMode != "passive" || d.ftpTLS != "explicit")
	}
	return d.jumpID != "" || d.keepalive != "30" || d.initCommand != "" || d.termScheme != "" || len(d.env) > 0
}

func (d *hostDraft) advancedSummary(a *app) string {
	if !d.hasAdvanced() {
		return "均为默认"
	}
	parts := []string{}
	if d.files {
		if d.remoteDir != "" {
			parts = append(parts, d.remoteDir)
		}
		if d.localDir != "" {
			parts = append(parts, "本地目录")
		}
		if d.encoding != "UTF-8" {
			parts = append(parts, d.encoding)
		}
		if d.protocol == "FTP" {
			parts = append(parts, map[string]string{"none": "无加密", "explicit": "显式 TLS", "implicit": "隐式 TLS"}[d.ftpTLS])
		}
	} else {
		for _, h := range a.hosts {
			if h.session.ID == d.jumpID && d.jumpID != "" {
				parts = append(parts, "跳板机 "+h.name)
				break
			}
		}
		if d.keepalive != "30" {
			parts = append(parts, "Keepalive "+d.keepalive+" 秒")
		}
		if d.initCommand != "" {
			parts = append(parts, "初始命令")
		}
		if d.termScheme != "" {
			parts = append(parts, strings.TrimPrefix(d.termScheme, "fixed:"))
		}
		if len(d.env) > 0 {
			parts = append(parts, fmt.Sprintf("%d 个环境变量", len(d.env)))
		}
	}
	return strings.Join(parts, " · ")
}

func validSessionHost(value string) bool {
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return false
	}
	if strings.HasPrefix(value, "[") || strings.HasSuffix(value, "]") {
		return strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") && net.ParseIP(value[1:len(value)-1]) != nil
	}
	if net.ParseIP(value) != nil {
		return true
	}
	if len(value) > 253 || strings.ContainsAny(value, ":[]/") {
		return false
	}
	value = strings.TrimSuffix(value, ".")
	for _, label := range strings.Split(value, ".") {
		if len(label) > 63 || !hostnamePattern.MatchString(label) {
			return false
		}
	}
	return true
}

func (a *app) draftError(d *hostDraft, field string) string {
	switch field {
	case "host":
		h := strings.TrimSpace(d.address)
		if h == "" {
			return "请填写主机"
		}
		if !validSessionHost(h) {
			return "主机格式不正确"
		}
	case "port":
		p, err := strconv.Atoi(d.port)
		if err != nil || p < 1 || p > 65535 {
			return "端口需在 1–65535 之间"
		}
	case "key":
		if d.auth == "key" && !slices.ContainsFunc(a.keys, func(k managedKey) bool { return k.ID == d.keyID }) {
			return "请选择私钥"
		}
	case "keepalive":
		if !d.files {
			p, err := strconv.Atoi(d.keepalive)
			if err != nil || p < 0 || p > 3600 {
				return "0–3600 秒"
			}
		}
	case "env":
		if !d.files {
			for _, v := range d.env {
				if !environmentPattern.MatchString(v) {
					return "格式应为 KEY=VALUE"
				}
			}
			if d.envInput != "" && !environmentPattern.MatchString(d.envInput) {
				return "格式应为 KEY=VALUE"
			}
		}
	case "name":
		if d.creatingGroup {
			return ""
		}
		name := strings.TrimSpace(d.name)
		if name == "" {
			name = strings.TrimSpace(d.address)
		}
		for _, group := range a.catalogGroups() {
			if group.name == d.group {
				for _, id := range group.hosts {
					if (!d.editing || id != d.id) && strings.EqualFold(name, a.hosts[id].name) {
						return "此分组已有同名会话"
					}
				}
			}
		}
	case "group":
		if d.creatingGroup {
			if strings.TrimSpace(d.newGroup) == "" {
				return "请填写分组名称"
			}
			for _, group := range a.catalogGroups() {
				if strings.EqualFold(strings.TrimSpace(d.newGroup), group.name) {
					return "此分组名称已存在"
				}
			}
		}
	}
	return ""
}

func (a *app) validateDraft(d *hostDraft) bool {
	d.errors = map[string]string{}
	first := ""
	for _, field := range []string{"name", "host", "port", "key", "group", "keepalive", "env"} {
		if message := a.draftError(d, field); message != "" {
			d.errors[field] = message
			if first == "" {
				first = field
			}
		}
	}
	if first != "" {
		if h := d.handles[first]; h != nil {
			h.Focus()
		}
		return false
	}
	return true
}

func (a *app) draftHost(d *hostDraft) host {
	h := host{os: 0}
	if d.editing {
		h = a.hosts[d.id]
	}
	h.name, h.address, h.user = strings.TrimSpace(d.name), strings.TrimSpace(d.address), strings.TrimSpace(d.user)
	if h.name == "" {
		h.name = h.address
	}
	h.port, _ = strconv.Atoi(d.port)
	h.protocol = ""
	if d.files {
		h.protocol = strings.ToLower(d.protocol)
	}
	h.session.Auth, h.session.KeyID, h.session.PortEdited = d.auth, d.keyID, d.portEdited
	if d.auth != "key" {
		h.session.KeyID = ""
	}
	if d.auth == "anonymous" {
		h.user = "anonymous"
	}
	h.session.SSH, h.session.Files = nil, nil
	if d.files {
		h.session.Files = &fileOptions{RemoteDir: d.remoteDir, LocalDir: d.localDir, Encoding: d.encoding}
		if d.protocol == "FTP" {
			h.session.Files.FTPMode, h.session.Files.FTPTLS = d.ftpMode, d.ftpTLS
		}
	} else {
		keepalive, _ := strconv.Atoi(d.keepalive)
		env := slices.Clone(d.env)
		if d.envInput != "" {
			env = append(env, d.envInput)
		}
		h.session.SSH = &sshOptions{JumpHostID: d.jumpID, KeepAlive: keepalive, InitCommand: d.initCommand, TermScheme: d.termScheme, Env: env}
	}
	return h
}

func (a *app) startDraftTest(c *ui.Context, d *hostDraft) {
	if d.saving || d.closing || strings.TrimSpace(d.address) == "" || d.testResults != nil || !a.validateDraft(d) {
		return
	}
	h := a.draftHost(d)
	password := d.password
	if d.passwordSaved && !d.passwordChanged {
		var err error
		password, err = a.sessionPassword(h)
		if err != nil {
			d.testResult = &connectionTestResult{Summary: "无法读取已保存的密码", Detail: err.Error(), Err: err}
			d.testVisible = true
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), theme.Current().Motion.Duration.Test)
	results := make(chan connectionTestResult, 1)
	d.testCancel, d.testResults, d.testResult, d.testVisible = cancel, results, nil, true
	c.Announce("正在连接…")
	passphrase := d.passphrase
	passphraseEdited := d.passphraseChanged
	snapshot := *a
	snapshot.hosts = slices.Clone(a.hosts)
	snapshot.keys = slices.Clone(a.keys)
	go func() {
		defer cancel()
		results <- snapshot.testConnection(ctx, h, password, passphrase, passphraseEdited)
	}()
	c.AnimationFrame()
}

func (a *app) saveSessionDraft(c *ui.Context, d *hostDraft, connect bool) {
	if d.saving || d.closing || strings.TrimSpace(d.address) == "" || !a.validateDraft(d) {
		return
	}
	d.saving = true
	defer func() { d.saving = false }()
	h := a.draftHost(d)
	initializeSession(&h)
	rollback, release, err := snapshotSessionSecrets(h, d.passwordChanged, d.passphraseChanged)
	if err != nil {
		uikit.Toast(c, uikit.ToastProps{Tone: uikit.ToneDanger, Title: "读取原有凭据失败", Detail: err.Error()})
		return
	}
	defer release()
	if err := a.storeSessionSecrets(&h, d.password, d.passwordChanged, d.passphrase, d.passphraseChanged); err != nil {
		uikit.Toast(c, uikit.ToastProps{Tone: uikit.ToneDanger, Title: "保存凭据失败", Detail: errors.Join(err, rollback()).Error()})
		return
	}
	id := hostID(len(a.hosts))
	if d.editing {
		id = d.id
	}
	err = a.commitCatalogMove(func() {
		if d.editing {
			a.hosts[id] = h
		} else {
			a.hosts = append(a.hosts, h)
		}
		groupName := d.group
		if d.creatingGroup {
			groupName = strings.TrimSpace(d.newGroup)
			a.groups = append(a.groups, a.newCatalogGroup(groupName))
		}
		for _, group := range a.catalogGroups() {
			if group.name == groupName {
				if !slices.Contains(group.hosts, id) {
					group.hosts = append(group.hosts, id)
				}
				a.setGroupCollapsed(group, false)
			} else {
				group.hosts = slices.DeleteFunc(group.hosts, func(candidate hostID) bool { return candidate == id })
			}
		}
	})
	if err != nil {
		uikit.Toast(c, uikit.ToastProps{Tone: uikit.ToneDanger, Title: "保存会话失败", Detail: errors.Join(err, rollback()).Error()})
		return
	}
	a.page().query = ""
	a.closeSessionDraft(d)
	if connect {
		target := sectionSSH
		if d.files {
			target = sectionFiles
		}
		open := func() {
			a.switchSection(target)
			if target == sectionFiles {
				a.openNewFileHost(id)
			} else {
				a.openHost(id)
			}
		}
		if target != a.section {
			d.afterClose = open
		} else {
			open()
		}
	}
}

func (a *app) closeSessionDraft(d *hostDraft) {
	d.cancelTest()
	d.password, d.passphrase = "", ""
	d.closing = true
}

func (a *app) draftOpenTabs(d *hostDraft) int {
	if !d.editing {
		return 0
	}
	n := 0
	for _, tab := range a.tabs {
		if slices.ContainsFunc(tab.root.sessions(), func(s *remoteSession) bool { return s.host == d.id }) {
			n++
		}
	}
	for _, s := range a.fileSessions {
		if s.host == d.id {
			n++
		}
	}
	return n
}

func (a *app) jumpCandidates(d *hostDraft) []hostID {
	ids := []hostID{}
	for index, h := range a.hosts {
		if h.protocol != "" && h.protocol != "ssh" || d.editing && hostID(index) == d.id || h.session.ID == "" {
			continue
		}
		seen := map[string]bool{}
		current := h
		cycle := false
		for current.session.SSH != nil && current.session.SSH.JumpHostID != "" {
			id := current.session.SSH.JumpHostID
			if seen[id] || d.editing && id == a.hosts[d.id].session.ID {
				cycle = true
				break
			}
			seen[id] = true
			found := slices.IndexFunc(a.hosts, func(candidate host) bool { return candidate.session.ID == id })
			if found < 0 {
				break
			}
			current = a.hosts[found]
		}
		if !cycle {
			ids = append(ids, hostID(index))
		}
	}
	return ids
}

func (d *hostDraft) schemeName() string {
	if d.termScheme == "" {
		return "跟随全局"
	}
	return strings.TrimPrefix(d.termScheme, "fixed:")
}
