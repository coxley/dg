package tui

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	modulePath         = "github.com/coxley/dg"
	updateCheckTimeout = 3 * time.Second
)

type updateStyles struct {
	Normal  lipgloss.Style
	Focused lipgloss.Style
}

type updateState struct {
	current    string
	latest     string
	available  bool
	focused    bool
	dismissed  bool
	installing bool
	styles     updateStyles
}

type updateAvailableMsg struct {
	current string
	latest  string
}

type updateInstallFinishedMsg struct {
	err error
}

func checkForUpdate() tea.Cmd {
	current, ok := installedVersion(debug.ReadBuildInfo())
	if !ok {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()
		command := exec.CommandContext(ctx, "go", "list", "-m", "-json", modulePath+"@latest")
		output, err := command.Output()
		if err != nil {
			return nil
		}
		var result struct {
			Version string
		}
		if err := json.Unmarshal(output, &result); err != nil {
			return nil
		}
		if !newerVersion(current, result.Version) {
			return nil
		}
		return updateAvailableMsg{current: current, latest: result.Version}
	}
}

func installedVersion(info *debug.BuildInfo, ok bool) (string, bool) {
	if !ok || info == nil || info.Main.Path != modulePath || info.Main.Version == "(devel)" {
		return "", false
	}
	if _, valid := parseVersion(info.Main.Version); !valid {
		return "", false
	}
	return info.Main.Version, true
}

func newerVersion(current, latest string) bool {
	currentVersion, currentOK := parseVersion(current)
	latestVersion, latestOK := parseVersion(latest)
	if !currentOK || !latestOK {
		return false
	}
	for i := range currentVersion.numbers {
		if latestVersion.numbers[i] != currentVersion.numbers[i] {
			return latestVersion.numbers[i] > currentVersion.numbers[i]
		}
	}
	return comparePrerelease(currentVersion.prerelease, latestVersion.prerelease) < 0
}

type semanticVersion struct {
	numbers    [3]uint64
	prerelease string
}

func parseVersion(value string) (semanticVersion, bool) {
	value = strings.TrimPrefix(value, "v")
	value, _, _ = strings.Cut(value, "+")
	core, prerelease, _ := strings.Cut(value, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semanticVersion{}, false
	}
	var version semanticVersion
	for i, part := range parts {
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return semanticVersion{}, false
		}
		version.numbers[i] = number
	}
	version.prerelease = prerelease
	return version, true
}

func comparePrerelease(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := range min(len(aParts), len(bParts)) {
		if aParts[i] == bParts[i] {
			continue
		}
		aNumber, aErr := strconv.ParseUint(aParts[i], 10, 64)
		bNumber, bErr := strconv.ParseUint(bParts[i], 10, 64)
		switch {
		case aErr == nil && bErr == nil:
			if aNumber < bNumber {
				return -1
			}
			return 1
		case aErr == nil:
			return -1
		case bErr == nil:
			return 1
		case aParts[i] < bParts[i]:
			return -1
		default:
			return 1
		}
	}
	if len(aParts) < len(bParts) {
		return -1
	}
	return 1
}

func (u *updateState) setStyles(styles updateStyles) {
	u.styles = styles
}

func (u *updateState) show(message updateAvailableMsg) {
	u.current = message.current
	u.latest = message.latest
	u.available = true
	u.dismissed = false
}

func (u *updateState) visible() bool {
	return u.available && !u.dismissed
}

func (u *updateState) focus() {
	if u.visible() {
		u.focused = true
	}
}

func (u *updateState) blur() {
	u.focused = false
}

func (u *updateState) dismiss() {
	u.dismissed = true
	u.focused = false
}

func (u *updateState) view() string {
	if !u.visible() {
		return ""
	}
	style := u.styles.Normal
	if u.focused {
		style = u.styles.Focused
	}
	return style.Render("Update Available")
}

func (u *updateState) install() tea.Cmd {
	if !u.visible() || !u.focused || u.installing {
		return nil
	}
	u.installing = true
	ctx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(ctx, "go", "install", modulePath+"@latest")
	return tea.ExecProcess(command, func(err error) tea.Msg {
		cancel()
		return updateInstallFinishedMsg{err: err}
	})
}
