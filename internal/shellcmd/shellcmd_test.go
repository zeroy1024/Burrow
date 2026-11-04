package shellcmd

import (
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestRuleMatch(t *testing.T) {
	encoded := func(script string) string {
		var data []byte
		for _, unit := range utf16.Encode([]rune(script)) {
			data = append(data, byte(unit), byte(unit>>8))
		}
		return base64.StdEncoding.EncodeToString(data)
	}
	tests := []struct {
		rule, command string
		ignoreCase    bool
		want          bool
		via, fallback string // expected substrings when matched
	}{
		// Plain words that only contain the program name.
		{rule: "rm", command: "format disk", want: false},
		{rule: "rm", command: "docker run --rm alpine ls", want: false},
		{rule: "rm", command: "./rm.sh", want: false},
		{rule: "rm", command: "git rm file", want: false},
		{rule: "rm", command: `echo "rm -rf /"`, want: false},
		{rule: "rm", command: `grep -rn "rm -rf" .`, want: false},
		{rule: "rm", command: "cat <<'EOF' > clean.sh\nrm -rf /tmp/x\nEOF", want: false},
		{rule: "rm", command: "docker rm web", want: false},
		{rule: "rm", command: "command -v rm", want: false},
		{rule: "rm", command: "git ls-files | xargs git rm", want: false},

		// Compound commands and spelling variants.
		{rule: "rm", command: "rm -rf /tmp/x", want: true},
		{rule: "rm", command: "cd /srv && make build; rm -rf dist || true", want: true},
		{rule: "rm", command: "/bin/rm x", want: true},
		{rule: "rm", command: `\rm x`, want: true},
		{rule: "rm", command: "r''m x", want: true},
		{rule: "rm", command: `$'\x72m' x`, want: true},
		{rule: "rm", command: "{rm,-rf,/tmp/x}", want: true},
		{rule: "rm", command: "RM x", want: true},
		{rule: "rm", command: `"$HOME/bin/rm" x`, want: true},
		{rule: "rm", command: "/bin/r? x", want: true},
		{rule: "rm", command: "for f in *.log; do rm \"$f\"; done", want: true},
		{rule: "rm", command: "x=$(rm -rf /tmp/a)", want: true},
		{rule: "rm", command: "diff <(rm a) b", want: true},
		{rule: "rm", command: "if true; then { rm a; }; fi", want: true},
		{rule: "rm", command: "f() { rm \"$1\"; }; f a", want: true},

		// Launchers.
		{rule: "rm", command: "cd /srv && sudo -u app rm -rf build", want: true, via: "sudo"},
		{rule: "rm", command: "sudo -E -n VAR=1 rm x", want: true, via: "sudo"},
		{rule: "rm", command: "env -i PATH=/bin rm x", want: true, via: "env"},
		{rule: "rm", command: "timeout -s KILL 10 nice -n 5 rm x", want: true, via: "timeout ▸ nice"},
		{rule: "rm", command: "ls | xargs -I{} rm -f {}", want: true, via: "xargs"},
		{rule: "rm", command: "find . -name '*.tmp' -exec rm {} +", want: true, via: "find -exec"},
		{rule: "rm", command: `find . -execdir rm -f {} \;`, want: true},
		{rule: "rm", command: "nohup rm -rf /x &", want: true},
		{rule: "rm", command: "busybox rm x", want: true},
		{rule: "rm", command: "docker exec -it web rm -rf /data", want: true, via: "docker exec"},
		{rule: "rm", command: "docker --context prod compose exec web rm x", want: true, via: "docker compose exec"},
		{rule: "rm", command: "kubectl exec -it pod -- rm -rf /data", want: true, via: "kubectl exec"},
		{rule: "rm", command: "sudo --unknown-opt value rm x", want: true},

		// Command strings.
		{rule: "rm", command: "bash -lc 'cd /tmp && rm -rf x'", want: true, via: "bash -c"},
		{rule: "rm", command: `sudo sh -c "rm -rf /var/cache/app"`, want: true, via: "sudo ▸ sh -c"},
		{rule: "rm", command: "ssh -p 22 web1 'rm -rf /var/log/app'", want: true, via: "ssh"},
		{rule: "rm", command: "ssh web1 sudo rm x", want: true},
		{rule: "rm", command: `eval "rm -rf x"`, want: true, via: "eval"},
		{rule: "rm", command: "su - root -c 'rm x'", want: true, via: "su"},
		{rule: "rm", command: "watch -n 5 'rm -f /tmp/lock'", want: true},
		{rule: "rm", command: "flock /tmp/l -c 'rm x'", want: true},
		{rule: "rm", command: `env -S "rm x"`, want: true},
		{rule: "rm", command: `bash -c "bash -c 'rm x'"`, want: true, via: "bash -c ▸ bash -c"},
		{rule: "del", command: `cmd /c "del /s /q C:\tmp"`, want: true, via: "cmd /c"},
		{rule: "remove-item", command: `powershell -NoProfile -ExecutionPolicy Bypass -Command "Remove-Item -Recurse x"`, want: true},
		{rule: "remove-item", command: "pwsh -EncodedCommand " + encoded("Remove-Item -Recurse C:\\x"), want: true},
		{rule: "cmd", command: `C:\Windows\System32\cmd.exe /c dir`, want: true},

		// Scripts on stdin.
		{rule: "rm", command: "bash <<'EOF'\nset -e\nrm -rf /tmp/x\nEOF", want: true, via: "标准输入"},
		{rule: "rm", command: "ssh web1 <<'EOF'\nrm -rf /tmp/x\nEOF", want: true},
		{rule: "rm", command: "ssh web1 bash -s <<EOF\nrm -rf $DIR\nEOF", want: true},
		{rule: "rm", command: "bash <<< 'rm x'", want: true},
		{rule: "rm", command: "echo 'rm -rf x' | sh", want: true},
		{rule: "rm", command: "printf '%s\\n' 'rm x' | bash", want: true},
		{rule: "rm", command: "cat <<'EOF' | sudo bash\nrm x\nEOF", want: true},
		{rule: "rm", command: "sh script.sh", want: false},

		// Argument conditions.
		{rule: "rm -rf", command: "rm -fr build", want: true},
		{rule: "rm -rf", command: "rm -r -f build", want: true},
		{rule: "rm -rf", command: "rm -r build", want: false},
		{rule: "rm -rf", command: "rm -r $FLAGS build", want: false},
		{rule: "rm -rf", command: "rm --recursive --force build", want: false},
		{rule: "rm -rf /", command: "rm -rf /tmp/x", want: false},
		{rule: "rm -rf /", command: "sudo rm -rf /", want: true},
		{rule: "rm /etc/*", command: "rm -f /etc/hosts", want: true},
		{rule: "git push --force", command: "git -C /repo push origin main --force", want: true},
		{rule: "git push --force", command: "git push --force-with-lease", want: false},
		{rule: "git push --force", command: "git push origin --force=true", want: true},
		{rule: "systemctl stop", command: "systemctl --now stop nginx", want: true},
		{rule: "systemctl stop", command: "systemctl status nginx", want: false},
		{rule: "mkfs.*", command: "mkfs.ext4 /dev/sdb1", want: true},
		{rule: "del /s", command: "del /S x", ignoreCase: true, want: true},
		{rule: "del /s", command: "del /S x", want: false},

		// Unresolvable text falls back to keywords.
		{rule: "rm", command: "CMD=rm; $CMD -rf /", want: true, fallback: "命令名"},
		{rule: "rm", command: "$CMD -rf /", want: false},
		{rule: "rm", command: `echo "unterminated; rm x`, want: true, fallback: "无法完整解析"},
		{rule: "rm", command: `echo "unterminated; format x`, want: false},
		{rule: "rm", command: `X="rm -rf /"; bash -c "$X"`, want: true, fallback: "动态"},
		{rule: "git push --force", command: "echo (git push --force)", want: true, fallback: "无法完整解析"},
	}
	for _, tt := range tests {
		rule, err := ParseRule(tt.rule, tt.ignoreCase)
		if err != nil {
			t.Fatalf("ParseRule(%q): %v", tt.rule, err)
		}
		ev, got := rule.Match(Analyze(tt.command))
		if got != tt.want {
			t.Errorf("rule %q on %q: matched = %v, want %v (evidence %+v)", tt.rule, tt.command, got, tt.want, ev)
			continue
		}
		if via := strings.Join(ev.Via, " ▸ "); got && !strings.Contains(via, tt.via) {
			t.Errorf("rule %q on %q: via = %q, want %q", tt.rule, tt.command, via, tt.via)
		}
		if got && !strings.Contains(ev.Fallback, tt.fallback) || tt.fallback == "" && ev.Fallback != "" {
			t.Errorf("rule %q on %q: fallback = %q, want %q", tt.rule, tt.command, ev.Fallback, tt.fallback)
		}
	}
}

func TestParseRuleRejects(t *testing.T) {
	for _, pattern := range []string{"", "rm; ls", "rm && ls", "rm | sh", "rm > x", "X=1 rm", "$CMD", "rm $(ls)", "if rm; then :; fi", `echo "a`} {
		if _, err := ParseRule(pattern, false); err == nil {
			t.Errorf("ParseRule(%q) accepted an invalid rule", pattern)
		}
	}
}

func TestMatchCountsDistinctPlaces(t *testing.T) {
	rule, err := ParseRule("rm", false)
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := rule.Match(Analyze("rm a; ls; rm b; sudo rm c"))
	if !ok || ev.Count != 3 || ev.Text != "rm a" {
		t.Fatalf("evidence = %+v, want first %q and 3 places", ev, "rm a")
	}
}
