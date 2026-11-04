package hostinfo

import (
	"context"
	"errors"
	"strings"
	"time"
)

// darwinFactsScript prints NUL-delimited records like the Linux collector.
// sw_vers and uname ship with every macOS release.
const darwinFactsScript = `printf '__BURROW_DARWIN_FACTS_V1__\000'
LC_ALL=C
export LC_ALL
if [ "$(uname -s 2>/dev/null)" != Darwin ]; then
  printf 'error\000不是 macOS 主机\000'
  exit 0
fi
for record in name:-productName version:-productVersion; do
  printf '%s\000%s\000' "${record%%:*}" "$(sw_vers "${record#*:}" 2>/dev/null)"
done
printf 'hostname\000%s\000kernel\000%s\000arch\000%s\000' "$(uname -n)" "$(uname -r)" "$(uname -m)"
`

var darwinFactsCommand = shellCommand(darwinFactsScript)

type darwinCollector struct{}

func (darwinCollector) Facts(ctx context.Context, r Runner, _ Detection) (Facts, error) {
	collectedAt := time.Now().UTC()
	output, err := r.Exec(ctx, darwinFactsCommand)
	if err != nil {
		return Facts{}, err
	}
	sections, err := parseRecords(string(output), "__BURROW_DARWIN_FACTS_V1__")
	if err != nil {
		return Facts{}, err
	}
	facts := Facts{Platform: Darwin, OSID: "macos", OSName: sections["name"], OSVersion: sections["version"],
		Hostname: sections["hostname"], Kernel: sections["kernel"], Arch: sections["arch"], CollectedAt: collectedAt}
	if facts.OSName == "" {
		facts.OSName = Darwin.DisplayName()
	}
	if facts.OSVersion != "" {
		facts.OSName = strings.TrimSpace(facts.OSName + " " + facts.OSVersion)
	}
	if !facts.Valid() {
		return Facts{}, errors.New("无法读取 macOS 主机名或内核版本")
	}
	return facts, nil
}

func (darwinCollector) NewSampler(Runner) Sampler { return unsupportedSampler{} }
