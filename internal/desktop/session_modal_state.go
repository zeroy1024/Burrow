package desktop

import (
	"context"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/hostinfo"
	"sshdesk/internal/locales"
	"sshdesk/internal/uikit"
)

type hostDraft struct {
	afterClose                                            func()
	credentialSource                                      *host
	name, address, user, port, group, system              string
	protocol, auth, keyID, password, passphrase           string
	gateway, keepalive, initCommand, termScheme, platform string
	remoteDir, localDir, encoding, ftpMode, ftpTLS        string
	env                                                   []string
	envInput, newGroup                                    string
	id                                                    hostID
	editing, deleting, files, portEdited                  bool
	passwordChanged, passphraseChanged, passwordSaved     bool
	advanced, closing, selectedName, creatingGroup        bool
	modal                                                 uikit.ModalController
	errors                                                map[string]locales.Text
	focused                                               map[string]bool
	handles                                               map[string]*ui.Handle
	scroll                                                ui.ScrollState
	testCancel                                            context.CancelFunc
	testResults                                           <-chan connectionTestResult
	testResult                                            *connectionTestResult
	testVisible                                           bool
	choosingDirectory                                     bool
	listedAgent                                           bool
	agentCount                                            int
	agentError                                            error
	submitted                                             bool
	saving                                                bool
	agentResults                                          <-chan sessionAgentResult
	generation                                            string
	initialValues                                         [21]string
	initialEnv                                            []string
	initialPortEdited                                     bool
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
	groups := a.catalogGroups()
	page.draft = hostDraft{
		port: "22", protocol: protocol, files: a.section == sectionFiles,
		auth: "password", keepalive: "30", encoding: "UTF-8", ftpMode: "passive", ftpTLS: "explicit",
		creatingGroup: len(groups) == 0, advanced: a.modalAdvancedOpen(strings.ToLower(protocol)),
	}
	if len(groups) > 0 {
		page.draft.group = groups[0].name
	}
	if page.lastGroup != nil {
		page.draft.group = page.lastGroup.name
	}
	page.draft.initState()
	page.addHostOpen = true
}

func (a *app) startOtherProtocolDraft(id hostID) {
	source := cloneHostEndpoint(a.hosts[id])
	target := sectionFiles
	if source.protocol == "sftp" {
		target = sectionSSH
	}
	a.switchSection(target)
	a.startHostDraft()
	d := &a.page().draft
	d.name, d.address, d.user = source.name, source.address, source.user
	d.port, d.system = strconv.Itoa(source.port), systems[source.os].name
	d.auth, d.keyID, d.gateway = source.session.Auth, source.session.KeyID, source.session.Gateway
	d.portEdited = source.session.PortEdited || source.port != d.defaultPort()
	d.passwordSaved = d.auth == "password" && source.session.PasswordSaved
	if d.passwordSaved {
		// Read the captured credential only when the user tests or saves the new profile.
		d.credentialSource = &source
	}
	d.initState()
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
		gateway: h.session.Gateway, passwordSaved: h.session.PasswordSaved, keepalive: "30", encoding: "UTF-8", ftpMode: "passive", ftpTLS: "explicit",
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
		d.keepalive, d.initCommand, d.termScheme = strconv.Itoa(p.KeepAlive), p.InitCommand, p.TermScheme
		d.platform = p.Platform
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
	d.errors, d.focused, d.handles = map[string]locales.Text{}, map[string]bool{}, map[string]*ui.Handle{}
	d.generation = newSessionID()
	d.initialValues, d.initialEnv, d.initialPortEdited = d.inputValues(), slices.Clone(d.env), d.portEdited
}

func (d *hostDraft) inputValues() [21]string {
	return [21]string{d.name, d.address, d.user, d.port, d.group, d.system,
		d.protocol, d.auth, d.keyID, d.gateway, d.keepalive, d.initCommand, d.termScheme, d.platform,
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
	if field == "protocol" || field == "auth" {
		delete(d.errors, "user")
	}
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
	return d.keepalive != "30" || d.initCommand != "" || d.termScheme != "" || len(d.env) > 0 || d.platform != ""
}

func (d *hostDraft) advancedSummary(a *app) string {
	if !d.hasAdvanced() {
		return locales.MessageAllDefaults()
	}
	parts := []string{}
	if d.files {
		if d.remoteDir != "" {
			parts = append(parts, d.remoteDir)
		}
		if d.localDir != "" {
			parts = append(parts, locales.MessageLocalDirectories())
		}
		if d.encoding != "UTF-8" {
			parts = append(parts, d.encoding)
		}
		if d.protocol == "FTP" {
			parts = append(parts, map[string]string{"none": locales.MessageUnencrypted(), "explicit": locales.MessageExplicitTls(), "implicit": locales.MessageImplicitTls()}[d.ftpTLS])
		}
	} else {
		if d.keepalive != "30" {
			parts = append(parts, "Keepalive "+d.keepalive+locales.MessageS())
		}
		if d.initCommand != "" {
			parts = append(parts, locales.MessageInitialCommand())
		}
		if d.termScheme != "" {
			parts = append(parts, strings.TrimPrefix(d.termScheme, "fixed:"))
		}
		if len(d.env) > 0 {
			parts = append(parts, locales.SessionEnvironmentCount(len(d.env)))
		}
		if d.platform != "" {
			parts = append(parts, hostinfo.Platform(d.platform).DisplayName())
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

func (a *app) draftError(d *hostDraft, field string) locales.Text {
	switch field {
	case "host":
		h := strings.TrimSpace(d.address)
		if h == "" {
			return locales.Deferred(locales.MessageEnterAHost)
		}
		if !validSessionHost(h) {
			return locales.Deferred(locales.MessageInvalidHostFormat)
		}
	case "port":
		p, err := strconv.Atoi(d.port)
		if err != nil || p < 1 || p > 65535 {
			return locales.Deferred(locales.MessagePortMustBeBetweenAnd)
		}
	case "user":
		if d.files && d.protocol == "FTP" && d.auth != "anonymous" && strings.TrimSpace(d.user) == "" {
			return locales.Deferred(locales.MessageEnterAUsername)
		}
	case "key":
		if d.auth == "key" && !slices.ContainsFunc(a.keys, func(k managedKey) bool { return k.ID == d.keyID }) {
			return locales.Deferred(locales.MessageSelectAPrivateKey)
		}
	case "keepalive":
		if !d.files {
			p, err := strconv.Atoi(d.keepalive)
			if err != nil || p < 0 || p > 3600 {
				return locales.Deferred(locales.MessageSeconds)
			}
		}
	case "env":
		if !d.files {
			for _, v := range d.env {
				if !environmentPattern.MatchString(v) {
					return locales.Deferred(locales.MessageUseTheFormatKeyValue)
				}
			}
			if d.envInput != "" && !environmentPattern.MatchString(d.envInput) {
				return locales.Deferred(locales.MessageUseTheFormatKeyValue)
			}
		}
	case "name":
		if d.creatingGroup {
			return locales.Text{}
		}
		name := strings.TrimSpace(d.name)
		if name == "" {
			name = strings.TrimSpace(d.address)
		}
		for _, group := range a.catalogGroups() {
			if group.name == d.group {
				for _, id := range group.hosts {
					if (!d.editing || id != d.id) && strings.EqualFold(name, a.hosts[id].name) {
						return locales.Deferred(locales.MessageASessionWithThisNameAlreadyExistsInThisGroup)
					}
				}
			}
		}
	case "group":
		if d.creatingGroup {
			if strings.TrimSpace(d.newGroup) == "" {
				return locales.Deferred(locales.MessageEnterAGroupName)
			}
			for _, group := range a.catalogGroups() {
				if strings.EqualFold(strings.TrimSpace(d.newGroup), group.name) {
					return locales.Deferred(locales.MessageThisGroupNameAlreadyExists)
				}
			}
		}
	}
	return locales.Text{}
}

func (a *app) validateDraft(d *hostDraft) bool {
	d.errors = map[string]locales.Text{}
	first := ""
	for _, field := range []string{"name", "host", "port", "user", "key", "group", "keepalive", "env"} {
		if message := a.draftError(d, field); !message.Empty() {
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
	h := host{os: linuxSystem}
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
	h.session.Gateway, h.session.GatewayRoute = d.gateway, nil
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
		h.session.SSH = &sshOptions{KeepAlive: keepalive, InitCommand: d.initCommand, TermScheme: d.termScheme, Env: env, Platform: d.platform}
	}
	return h
}

func (a *app) startDraftTest(c *ui.Context, d *hostDraft) {
	if d.saving || d.closing || strings.TrimSpace(d.address) == "" || d.testResults != nil || !a.validateDraft(d) {
		return
	}
	h := a.draftHost(d)
	password := d.password
	if d.auth == "password" && d.passwordSaved && !d.passwordChanged {
		var err error
		credentialHost := h
		if d.credentialSource != nil {
			credentialHost = *d.credentialSource
		}
		password, err = a.sessionPassword(credentialHost)
		if err != nil {
			d.testResult = &connectionTestResult{Summary: locales.Deferred(
				locales.MessageUnableToReadSavedPassword,
			), Detail: locales.ErrorText(
				err,
			), Err: err}
			d.testVisible = true
			return
		}
	}
	ctx, cancel := context.WithCancel(a.ctx)
	results := make(chan connectionTestResult, 1)
	d.testCancel, d.testResults, d.testResult, d.testVisible = cancel, results, nil, true
	c.Announce(locales.MessageConnecting())
	passphrase := d.passphrase
	passphraseEdited := d.passphraseChanged
	test := a.prepareConnectionTest(h, password, passphrase, passphraseEdited)
	a.sshWorkers.Go(func() {
		defer cancel()
		results <- test(ctx)
	})
	c.AnimationFrame()
}

func (a *app) saveSessionDraft(c *ui.Context, d *hostDraft, connect bool) {
	if d.saving || d.closing || strings.TrimSpace(d.address) == "" || !a.validateDraft(d) {
		return
	}
	d.saving = true
	facts := a.draftHostFacts(d)
	h := a.draftHost(d)
	initializeSession(&h)
	factsKey := ""
	if facts != nil {
		plan, err := a.captureSSHPlan(h)
		if err != nil {
			a.persistenceNotice(locales.MessageUnableToSaveHostInformation, err)
			d.saving = false
			return
		}
		factsKey = hostFactsKey(plan)
	}
	password, passwordChanged := d.password, d.passwordChanged
	if d.credentialSource != nil && d.auth == "password" && d.passwordSaved && !passwordChanged {
		var err error
		password, err = a.sessionPassword(*d.credentialSource)
		if err != nil {
			a.persistenceNotice(locales.MessageUnableToReadTheSourceSessionPassword, err)
			d.saving = false
			return
		}
		passwordChanged = true
	}
	changes, err := a.prepareSessionSecrets(&h, password, passwordChanged, d.passphrase, d.passphraseChanged)
	if err != nil {
		uikit.Toast(
			c,
			uikit.ToastProps{Tone: uikit.ToneDanger, Title: locales.Deferred(
				locales.MessageUnableToSaveCredentials,
			), Detail: locales.ErrorText(
				err,
			)},
		)
		d.saving = false
		return
	}
	id := hostID(len(a.hosts))
	if d.editing {
		id = d.id
	}
	a.commitCatalogSecretsAsync(changes, func() {
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
	}, func(err error) {
		d.saving = false
		if err != nil {
			a.persistenceNotice(locales.MessageUnableToSaveSession, err)
			return
		}
		if facts != nil {
			a.rememberHostFacts(factsKey, *facts)
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
	})
}

func (a *app) closeSessionDraft(d *hostDraft) {
	d.cancelTest()
	d.password, d.passphrase = "", ""
	d.closing = true
	d.modal.Close()
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

func (d *hostDraft) schemeName() string {
	if d.termScheme == "" {
		return ""
	}
	return strings.TrimPrefix(d.termScheme, "fixed:")
}
