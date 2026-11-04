package desktop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"sshdesk/internal/sshclient"
)

type linuxHostFacts struct {
	Hostname    string    `json:"hostname"`
	OSID        string    `json:"osId"`
	OSVersion   string    `json:"osVersion"`
	OSName      string    `json:"osName"`
	Kernel      string    `json:"kernel"`
	CollectedAt time.Time `json:"collectedAt"`
}

type hostFactsState struct {
	values  map[string]linuxHostFacts
	loadErr error
}

type hostFactsProbe struct {
	connection *sshclient.Connection
	pending    bool
	errorText  string
}

// Identity includes the captured jump route, but excludes credentials and
// configuration revisions. Refreshing facts must not change a connection plan.
func hostFactsKey(plan sshDialPlan) string {
	var target strings.Builder
	for _, step := range plan.steps {
		h := step.endpoint
		fmt.Fprintf(
			&target,
			"%q %q %d %q\n",
			h.session.ID,
			h.address,
			h.port,
			h.user,
		)
	}
	sum := sha256.Sum256([]byte(target.String()))
	return hex.EncodeToString(sum[:])
}

func (a *app) loadHostFacts() error {
	a.hostFacts.values = map[string]linuxHostFacts{}
	a.hostFacts.loadErr = a.readHostFacts()
	return a.hostFacts.loadErr
}

func (a *app) readHostFacts() error {
	var data []byte
	found, err := a.store.ReadSetting(a.ctx, "hostFacts", &data)
	if err != nil || !found {
		return err
	}
	values := map[string]linuxHostFacts{}
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if values == nil {
		return errors.New("主机基础信息格式无效")
	}
	for key, facts := range values {
		if key == "" || !facts.valid() {
			return errors.New("主机基础信息记录无效")
		}
	}
	a.hostFacts.values = values
	return nil
}

func (f linuxHostFacts) valid() bool {
	return f.Hostname != "" && f.Kernel != "" && f.OSName != "" && !f.CollectedAt.IsZero()
}

func (f linuxHostFacts) style() systemStyle {
	for _, style := range systems {
		if strings.ToLower(style.name) == f.OSID {
			return style
		}
	}
	return systems[linuxSystem]
}

func (a *app) savedHostFacts(h host) *linuxHostFacts {
	if h.protocol != "" && h.protocol != "ssh" {
		return nil
	}
	plan, err := a.captureSSHPlan(h)
	if err != nil {
		return nil
	}
	facts, ok := a.hostFacts.values[hostFactsKey(plan)]
	if !ok {
		return nil
	}
	return &facts
}

func (a *app) hostSystem(h host) (systemStyle, string) {
	if h.protocol != "" && h.protocol != "ssh" {
		return systems[h.os], h.system
	}
	if facts := a.savedHostFacts(h); facts != nil {
		return facts.style(), facts.OSName
	}
	return systems[linuxSystem], ""
}

func (a *app) rememberHostFacts(key string, facts linuxHostFacts) {
	if previous, ok := a.hostFacts.values[key]; ok && !facts.CollectedAt.After(previous.CollectedAt) {
		return
	}
	a.hostFacts.values[key] = facts
	if a.hostFacts.loadErr != nil {
		a.persistenceNotice("保存主机基础信息失败", a.hostFacts.loadErr)
		return
	}
	data, err := json.Marshal(a.hostFacts.values)
	if err != nil {
		a.persistenceNotice("保存主机基础信息失败", err)
		return
	}
	a.persist(func() error {
		return a.store.SaveSetting(a.ctx, "hostFacts", data)
	}, func(err error) {
		a.persistenceNotice("保存主机基础信息失败", err)
	})
}

// A split reuses its transport's probe. Shell creation has priority; callers
// start this only after the first terminal has attached successfully.
func (a *app) startSSHHostFacts(group *sshConnectionGroup) {
	connection := group.connection
	if group.facts.connection == connection {
		return
	}
	group.facts = hostFactsProbe{connection: connection, pending: true}
	release, err := connection.Retain()
	if err != nil {
		group.facts.pending, group.facts.errorText = false, err.Error()
		return
	}
	lifetime := group.ctx
	key, generation := hostFactsKey(group.plan), group.generation
	a.sshWorkers.Go(func() {
		defer release()
		facts, err := collectLinuxHostFacts(lifetime, connection)
		a.window.Update(func() {
			if group.generation != generation || group.connection != connection || lifetime.Err() != nil {
				return
			}
			group.facts.pending = false
			if err != nil {
				group.facts.errorText = err.Error()
				a.persistenceNotice("获取主机基础信息失败", err)
				return
			}
			a.rememberHostFacts(key, facts)
		})
	})
}

const hostFactsScript = "printf '__BURROW_HOST_FACTS_V1__\\000'\n" + linuxCollectorPrelude + `
emit_file hostname /proc/sys/kernel/hostname 4096
emit_file kernel /proc/sys/kernel/osrelease 4096
emit_file system /etc/os-release 16384
`

var hostFactsCommand = linuxCollectorCommand(hostFactsScript)

func collectLinuxHostFacts(ctx context.Context, connection *sshclient.Connection) (linuxHostFacts, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	collectedAt := time.Now().UTC()
	output, err := connection.Exec(ctx, hostFactsCommand)
	if err != nil {
		return linuxHostFacts{}, err
	}
	sections, err := parseLinuxCollectorOutput(string(output), "__BURROW_HOST_FACTS_V1__")
	if err != nil {
		return linuxHostFacts{}, err
	}
	facts := linuxHostFacts{
		Hostname: strings.TrimSpace(sections["hostname"]), Kernel: strings.TrimSpace(sections["kernel"]),
		OSID: osReleaseValue(sections["system"], "ID"), OSVersion: osReleaseValue(sections["system"], "VERSION_ID"),
		OSName: osReleaseValue(sections["system"], "PRETTY_NAME"), CollectedAt: collectedAt,
	}
	if facts.OSName == "" {
		facts.OSName = "Linux"
	}
	if !facts.valid() {
		return linuxHostFacts{}, errors.New("无法读取 Linux 主机名或内核版本")
	}
	return facts, nil
}

func (a *app) draftHostFacts(d *hostDraft) *linuxHostFacts {
	if d.files {
		return nil
	}
	h := a.draftHost(d)
	plan, err := a.captureSSHPlan(h)
	if err != nil {
		return nil
	}
	result := d.testResult
	if result != nil && result.Err == nil && result.HostFactsKey == hostFactsKey(plan) {
		return result.HostFacts
	}
	return a.savedHostFacts(h)
}
