package desktop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"sshdesk/internal/fileclient"
	"sshdesk/internal/hostinfo"
	"sshdesk/internal/sshclient"
)

type hostFactsState struct {
	values  map[string]hostinfo.Facts
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
	for _, hop := range plan.route {
		if hop.SOCKS != nil {
			fmt.Fprintf(&target, "socks5 %q %d\n", hop.SOCKS.Address, hop.SOCKS.Port)
		}
	}
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
	a.hostFacts.values = map[string]hostinfo.Facts{}
	a.hostFacts.loadErr = a.readHostFacts()
	return a.hostFacts.loadErr
}

func (a *app) readHostFacts() error {
	var data []byte
	found, err := a.store.ReadSetting(a.ctx, "hostFacts", &data)
	if err != nil || !found {
		return err
	}
	values := map[string]hostinfo.Facts{}
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if values == nil {
		return errors.New("主机基础信息格式无效")
	}
	for key, facts := range values {
		// Records saved before platform detection were all collected from Linux.
		if facts.Platform == "" {
			facts.Platform = hostinfo.Linux
			values[key] = facts
		}
		if key == "" || !facts.Valid() {
			return errors.New("主机基础信息记录无效")
		}
	}
	a.hostFacts.values = values
	return nil
}

func factsStyle(f hostinfo.Facts) systemStyle {
	switch f.Platform {
	case hostinfo.Darwin:
		return systems[macOSSystem]
	case hostinfo.Windows:
		return systems[windowsSystem]
	}
	for _, style := range systems {
		if slices.Contains(style.ids, f.OSID) {
			return style
		}
	}
	return systems[linuxSystem]
}

// sshPlatform is the platform the user pinned for an SSH host, or "" for
// automatic detection.
func sshPlatform(h host) hostinfo.Platform {
	if h.session.SSH == nil {
		return ""
	}
	return hostinfo.Platform(h.session.SSH.Platform)
}

// knownPlatform is the pinned platform, else the last detected one, else "".
func (a *app) knownPlatform(h host) hostinfo.Platform {
	if platform := sshPlatform(h); platform != "" {
		return platform
	}
	if facts := a.savedHostFacts(h); facts != nil {
		return facts.Platform
	}
	return ""
}

// groupPlatform chooses the collector for a transport; known is false until
// the platform is pinned or detected.
func (a *app) groupPlatform(group *sshConnectionGroup) (platform hostinfo.Platform, known bool) {
	if platform := sshPlatform(group.endpoint); platform != "" {
		return platform, true
	}
	if group.planErr != nil {
		return "", false
	}
	facts, ok := a.hostFacts.values[hostFactsKey(group.plan)]
	return facts.Platform, ok
}

func (a *app) savedHostFacts(h host) *hostinfo.Facts {
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
	facts := a.savedHostFacts(h)
	if platform := sshPlatform(h); platform != "" && (facts == nil || facts.Platform != platform) {
		return factsStyle(hostinfo.Facts{Platform: platform}), platform.DisplayName()
	}
	if facts != nil {
		return factsStyle(*facts), facts.OSName
	}
	return systems[linuxSystem], ""
}

func (a *app) rememberHostFacts(key string, facts hostinfo.Facts) {
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
	key, generation, platform := hostFactsKey(group.plan), group.generation, sshPlatform(group.endpoint)
	a.sshWorkers.Go(func() {
		defer release()
		facts, err := hostinfo.Collect(lifetime, sshHostRunner{connection}, connection.ServerVersion(), platform)
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

// draftSystemStyle prefers detected facts that agree with a pinned platform.
func (a *app) draftSystemStyle(d *hostDraft) (systemStyle, bool) {
	facts := a.draftHostFacts(d)
	if platform := hostinfo.Platform(d.platform); platform != "" && (facts == nil || facts.Platform != platform) {
		return factsStyle(hostinfo.Facts{Platform: platform}), true
	}
	if facts == nil {
		return systemStyle{}, false
	}
	return factsStyle(*facts), true
}

func (a *app) draftHostFacts(d *hostDraft) *hostinfo.Facts {
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

// sshHostRunner gives collectors command and SFTP access to one transport.
type sshHostRunner struct{ connection *sshclient.Connection }

func (r sshHostRunner) Exec(ctx context.Context, command string) ([]byte, error) {
	return r.connection.Exec(ctx, command)
}

func (r sshHostRunner) Files(ctx context.Context) (hostinfo.Files, error) {
	for {
		if err := r.connection.WaitChannelReady(ctx); err != nil {
			return nil, err
		}
		if err := fileclient.WaitSFTPReady(ctx, r.connection); err != nil {
			return nil, err
		}
		client, err := fileclient.NewSFTP(ctx, r.connection)
		if errors.Is(err, sshclient.ErrChannelPending) || errors.Is(err, fileclient.ErrSFTPInitPending) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return sftpHostFiles{client}, nil
	}
}

type sftpHostFiles struct{ client fileclient.Client }

func (f sftpHostFiles) StatFilesystem(ctx context.Context, name string) (hostinfo.Usage, error) {
	stats, err := f.client.(fileclient.FilesystemStatter).StatFilesystem(ctx, name)
	return hostinfo.Usage{Total: stats.Total, Free: stats.Free}, err
}

func (f sftpHostFiles) ReadFile(ctx context.Context, name string, limit int64) ([]byte, error) {
	stream, err := f.client.OpenRead(ctx, name, 0)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(stream, limit+1))
	if err := errors.Join(readErr, stream.Close()); err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("SFTP 读取 " + name + " 超过采集上限")
	}
	return data, nil
}

func (f sftpHostFiles) Close() error { return f.client.Close() }
