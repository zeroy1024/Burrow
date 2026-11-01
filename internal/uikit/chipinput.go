package uikit

import (
	"slices"
	"strings"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type ChipInputProps struct {
	Label, Placeholder string
	Mono               bool
	Validate           func(string) error
	PreserveCommas     bool
	CommitOnBlur       bool
	Disabled, ReadOnly bool
	Handle             *ui.Handle
	Draft              *string
}

type ChipInputResult struct {
	Root, Input ui.Element
	Changed     bool
	Error       string
}

type chipIdentity struct {
	ID    uint64
	Value string
}

type chipIdentities struct {
	Next  uint64
	Items []chipIdentity
}

func (state *chipIdentities) sync(values []string) {
	previous := state.Items
	used := make([]bool, len(previous))
	state.Items = make([]chipIdentity, len(values))
	for index, value := range values {
		id := uint64(0)
		for old, item := range previous {
			if !used[old] && item.Value == value {
				id, used[old] = item.ID, true
				break
			}
		}
		if id == 0 {
			state.Next++
			id = state.Next
		}
		state.Items[index] = chipIdentity{ID: id, Value: value}
	}
}

func ChipInput(c *ui.Context, values *[]string, props ChipInputProps) ChipInputResult {
	t := theme.Current()
	p := t.C.Input
	previous := slices.Clone(*values)
	result := ChipInputResult{Root: ui.Row(c).FillWidth().MinHeight(p.Height).Padding(t.Space.N3, p.PaddingX).
		Gap(t.Space.N4).Wrap().Radius(p.Radius).Background(Color(p.Bg)).Role(ui.RoleGroup).
		Label(props.Label).Disabled(props.Disabled)}
	state := ui.Local(result.Root, "uikit.chips.identities", func() chipIdentities { return chipIdentities{} })
	state.sync(*values)
	pendingChange := ui.Local(result.Root, "uikit.chips.changed", func() bool { return false })
	changedBeforeBuild := *pendingChange
	*pendingChange = false
	entry := props.Draft
	if entry == nil {
		entry = ui.Local(result.Root, "uikit.chips.entry", func() string { return "" })
	}
	previousEntry := *entry
	focused := ui.Local(result.Root, "uikit.chips.focused", func() bool { return false })
	commit := func(value string) {
		if value == "" {
			return
		}
		*values = append(*values, value)
		*pendingChange = true
		*entry = ""
	}
	remove := -1
	result.Root.Children(func() {
		for index, item := range state.Items {
			invalid := false
			if props.Validate != nil {
				if err := props.Validate(item.Value); err != nil {
					invalid = true
				}
			}
			chip := ui.Row(c).Key(item.ID).Height(p.ChipHeight).Padding(0, t.Space.N6).Gap(t.Space.N4).
				Radius(p.ChipRadius).Background(Color(p.ChipBg)).AlignItems(ui.Center)
			if invalid {
				chip.Background(Color(t.Color.Danger.Subtle))
			}
			chip.Children(func() {
				text := ui.Text(c, item.Value).FontSize(p.ChipFontSize).SingleLine().PassThrough()
				if props.Mono {
					text.Font(strings.Join(p.MonoFont, ", "))
				}
				if !props.ReadOnly {
					button := IconButton(c, IconClose, IconButtonProps{Key: "remove", Label: "删除 " + item.Value, Size: SizeXs}).
						Size(t.Size.IconSm, t.Size.IconSm).Shrink(0)
					if button.Clicked() {
						remove = index
					}
				}
			})
		}
		result.Input = ui.TextInputBase(c.Key("entry"), entry).Height(p.ChipHeight).MinWidth(p.NumberWidth).Grow(1).
			FontSize(p.MonoSize).FixedLineHeight(p.LineHeight).Placeholder(props.Placeholder).
			Label(props.Label).ReadOnly(props.ReadOnly)
		if props.Mono {
			result.Input.Font(strings.Join(p.MonoFont, ", "))
		}
		if props.Handle != nil {
			result.Input.Bind(props.Handle)
		}
		if !props.ReadOnly {
			result.Input.OnTextKey(func(key ui.TextKey) bool {
				if key.Mods != 0 {
					return false
				}
				if key.Key == ui.KeyEnter {
					commit(key.Text())
					key.SetText("")
					return true
				}
				if key.Key == ui.KeyBackspace && key.Text() == "" && len(*values) > 0 {
					*values = (*values)[:len(*values)-1]
					*pendingChange = true
					return true
				}
				return false
			})
		}
		if !props.ReadOnly && !props.PreserveCommas && result.Input.Changed() {
			// Character keys arrive as text edits, not OnTextKey events.
			remaining := *entry
			separated := false
			for {
				value, tail, found := strings.Cut(remaining, ",")
				if !found {
					break
				}
				separated = true
				commit(value)
				remaining = tail
			}
			if separated {
				*entry = remaining
				c.Invalidate()
			}
		}
		if props.CommitOnBlur && *focused && !result.Input.Focused() && !props.ReadOnly {
			commit(*entry)
		}
		*focused = result.Input.Focused()
	})
	if remove >= 0 {
		*values = slices.Delete(*values, remove, remove+1)
	}
	if props.Validate != nil {
		for _, value := range *values {
			if err := props.Validate(value); err != nil {
				result.Error = err.Error()
				break
			}
		}
	}
	result.Changed = changedBeforeBuild || *pendingChange || result.Input.Changed() || previousEntry != *entry || !slices.Equal(previous, *values)
	*pendingChange = false
	StyleDisabled(result.Root, props.Disabled)
	styleFormControl(result.Root, result.Error, result.Input.FocusVisible())
	return result
}
