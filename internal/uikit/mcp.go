package uikit

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type CopyFieldProps struct {
	Key         any
	Label       string
	ButtonLabel string
	Fill        bool
}

type CopyFieldResult struct {
	Root   ui.Element
	Copied bool
}

// CopyField renders a monospace value chip with an integrated copy trigger and
// transient "已复制" feedback.
func CopyField(c *ui.Context, value string, props CopyFieldProps) CopyFieldResult {
	t := theme.Current()
	label := props.Label
	if label == "" {
		label = "复制 " + value
	}
	root := ui.ButtonBase(c).Height(t.Size.ControlSm).Padding(0, t.Space.N8).Gap(t.Space.N6).
		Radius(t.Radius.ControlSm).Border(t.Size.Hairline, Color(t.Color.Border.Control)).
		Background(Color(t.Color.Surface.Sunken)).AlignItems(ui.Center).FocusRing(false).Label(label)
	if props.Key != nil {
		root.Key(props.Key)
	}
	if props.Fill {
		root.FillWidth()
	}
	copiedAt := ui.Local(root, "uikit.copyfield.at", func() time.Time { return time.Time{} })
	copied := !copiedAt.IsZero() && c.Now().Sub(*copiedAt) < 1500*time.Millisecond
	if copied {
		c.AnimationFrame()
	}
	if root.Hovered() {
		root.Background(Color(t.Color.State.Hover))
	}
	if root.FocusVisible() {
		Shadow(root, t.C.Button.FocusShadow)
	}
	result := CopyFieldResult{Root: root}
	if root.Clicked() {
		*copiedAt = c.Now()
		result.Copied = true
	}
	root.Children(func() {
		text := ui.Text(c, value).Font(strings.Join(t.Font.Family.Mono, ", ")).FontSize(t.Font.Size.Footnote).
			TextColor(Color(t.Color.Text.Primary)).SingleLine().Ellipsis("…").PassThrough()
		if props.Fill {
			text.Grow(1).MinWidth(0)
		}
		icon := IconCopy
		ink := t.Color.Icon.Subtle
		if copied {
			icon = IconCheck
			ink = t.Color.Success.Text
		}
		if props.ButtonLabel != "" {
			btnText := props.ButtonLabel
			if copied {
				btnText = "已复制"
			}
			ui.Row(c).Gap(t.Space.N4).AlignItems(ui.Center).PassThrough().Children(func() {
				ui.Icon(c, IconSVG(icon)).Size(t.Size.IconSm, t.Size.IconSm).TextColor(Color(ink)).PassThrough()
				ui.Text(c, btnText).FontSize(t.Font.Size.Caption).TextColor(Color(ink)).SingleLine().PassThrough()
			})
		} else {
			ui.Icon(c, IconSVG(icon)).Size(t.Size.IconSm, t.Size.IconSm).TextColor(Color(ink)).PassThrough()
		}
	})
	return result
}

type FilterChipProps struct {
	Key   any
	Label string
}

// FilterChip renders an inline prefix-labeled dropdown pill ("Secret Key 全部 ▾").
func FilterChip[T comparable](c *ui.Context, prefix string, selected *T, options []Option[T], props FilterChipProps) ui.Element {
	t := theme.Current()
	currentLabel := ""
	for _, opt := range options {
		if opt.Value == *selected {
			currentLabel = opt.Label
			break
		}
	}
	if currentLabel == "" && len(options) > 0 {
		currentLabel = options[0].Label
	}
	aria := props.Label
	if aria == "" {
		aria = prefix + " " + currentLabel
	}
	trigger := ui.ButtonBase(c).Height(t.Size.ControlSm).Padding(0, t.Space.N10).Gap(t.Space.N6).
		Radius(t.Radius.ControlSm).Border(t.Size.Hairline, Color(t.Color.Border.Control)).
		Background(Color(t.Color.Surface.Card)).AlignItems(ui.Center).FocusRing(false).Label(aria)
	if props.Key != nil {
		trigger.Key(props.Key)
	}
	if trigger.Hovered() {
		trigger.Background(Color(t.Color.State.Hover))
	}
	if trigger.FocusVisible() {
		Shadow(trigger, t.C.Button.FocusShadow)
	}
	var entries []MenuEntry
	for _, opt := range options {
		option := opt
		checked := option.Value == *selected
		entries = append(entries, MenuEntry{
			ID:      fmt.Sprintf("%v", option.Value),
			Label:   option.Label,
			Checked: &checked,
			Radio:   true,
			OnChoose: func() {
				*selected = option.Value
			},
		})
	}
	MenuOn(c, trigger, entries)
	trigger.Children(func() {
		ui.Text(c, prefix).FontSize(t.Font.Size.Small).TextColor(Color(t.Color.Text.Secondary)).SingleLine().Shrink(0).PassThrough()
		ui.Text(c, currentLabel).FontSize(t.Font.Size.Small).FontWeight(t.Font.Weight.Semibold).
			TextColor(Color(t.Color.Text.Primary)).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").PassThrough()
		ui.Icon(c, IconSVG(IconChevronDown)).Size(t.Size.IconSm, t.Size.IconSm).
			TextColor(Color(t.Color.Icon.Subtle)).Shrink(0).PassThrough()
	})
	return trigger
}

type PaginationProps struct {
	Total, PageSize int
	Summary         string
}

// Pagination renders a table footer with total count summary and page buttons.
func Pagination(c *ui.Context, page *int, props PaginationProps) ui.Element {
	t := theme.Current()
	pageSize := max(1, props.PageSize)
	totalPages := max(1, (props.Total+pageSize-1)/pageSize)
	if *page < 1 {
		*page = 1
	} else if *page > totalPages {
		*page = totalPages
	}
	summary := props.Summary
	if summary == "" {
		summary = fmt.Sprintf("共 %d 条 · 每页 %d 条", props.Total, pageSize)
	}
	return ui.Row(c).FillWidth().Height(t.Size.StatusBar+t.Space.N8).Shrink(0).
		Padding(0, t.Space.N14).BorderWidth(t.Size.Divider, 0, 0, 0).BorderColor(Color(t.Color.Border.Divider)).
		AlignItems(ui.Center).Gap(t.Space.N8).Children(func() {
		ui.Text(c, summary).FontSize(t.Font.Size.Footnote).TextColor(Color(t.Color.Text.Secondary)).FontFeatures("tnum")
		ui.Spacer(c)
		prev := IconButton(c.Key("prev"), IconArrowLeft, IconButtonProps{Label: "上一页", Size: SizeXs, Disabled: *page <= 1})
		if prev.Clicked() && *page > 1 {
			*page--
		}
		for idx := 1; idx <= totalPages; idx++ {
			if totalPages > 6 && idx > 3 && idx < totalPages && idx != *page {
				if idx == 4 {
					ui.Text(c, "…").FontSize(t.Font.Size.Footnote).TextColor(Color(t.Color.Text.Tertiary)).Padding(0, t.Space.N4)
				}
				continue
			}
			pageNum := idx
			active := *page == pageNum
			btn := ui.ButtonBase(c.Key(pageNum)).Size(t.Size.ControlXs, t.Size.ControlXs).Radius(t.Radius.ControlSm).
				Center().FocusRing(false).Label("第 " + strconv.Itoa(pageNum) + " 页")
			bg, fg := ui.Transparent, Color(t.Color.Text.Secondary)
			if active {
				bg, fg = Color(t.Color.Accent.Default), Color(t.Color.Text.OnAccent)
			} else if btn.Hovered() {
				bg, fg = Color(t.Color.State.Hover), Color(t.Color.Text.Primary)
			}
			btn.Background(bg).TextColor(fg)
			btn.Children(func() {
				ui.Text(c, strconv.Itoa(pageNum)).FontSize(t.Font.Size.Footnote).FontWeight(t.Font.Weight.Semibold).
					FontFeatures("tnum").SingleLine().PassThrough()
			})
			if btn.Clicked() {
				*page = pageNum
			}
		}
		next := IconButton(c.Key("next"), IconArrowRight, IconButtonProps{Label: "下一页", Size: SizeXs, Disabled: *page >= totalPages})
		if next.Clicked() && *page < totalPages {
			*page++
		}
	})
}

type TimelineItem struct {
	Tone                Tone
	Stage, Detail, Time string
	Pending             bool
}

// Timeline renders a vertical sequence of operation lifecycle events.
func Timeline(c *ui.Context, items []TimelineItem) ui.Element {
	t := theme.Current()
	return ui.Column(c).FillWidth().Gap(0).Children(func() {
		for idx, item := range items {
			last := idx == len(items)-1
			ui.Row(c).Key(idx).FillWidth().Gap(t.Space.N10).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Width(t.Space.N14).Shrink(0).AlignItems(ui.Center).Children(func() {
					dotColor := t.Color.Text.Muted
					switch item.Tone {
					case ToneSuccess:
						dotColor = t.Color.Success.Default
					case ToneWarning:
						dotColor = t.Color.Warning.Default
					case ToneDanger:
						dotColor = t.Color.Danger.Default
					case ToneAccent:
						dotColor = t.Color.Accent.Default
					}
					dot := ui.Box(c).Size(t.Size.Dot, t.Size.Dot).Radius(t.Radius.Pill).Margin(t.Space.N4, 0, 0, 0)
					if item.Pending {
						dot.Border(t.Size.Hairline*2, Color(dotColor))
					} else {
						dot.Background(Color(dotColor))
					}
					if !last {
						ui.Box(c).Width(t.Size.Divider).Grow(1).MinHeight(t.Space.N12).Background(Color(t.Color.Border.Subtle))
					}
				})
				ui.Row(c).Grow(1).MinWidth(0).Padding(0, 0, t.Space.N8, 0).Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, item.Stage).FontSize(t.Font.Size.Small).FontWeight(t.Font.Weight.Semibold).
						TextColor(Color(t.Color.Text.Primary)).Shrink(0)
					ui.Text(c, item.Detail).Grow(1).MinWidth(0).FontSize(t.Font.Size.Footnote).
						TextColor(Color(t.Color.Text.Secondary)).SingleLine().Ellipsis("…")
					if item.Time != "" {
						ui.Text(c, item.Time).Font(strings.Join(t.Font.Family.Mono, ", ")).FontSize(t.Font.Size.Caption).
							TextColor(Color(t.Color.Text.Tertiary)).FontFeatures("tnum").Shrink(0)
					}
				})
			})
		}
	})
}

type PermissionMatrixRow struct {
	ID, Name, Subtitle         string
	Warning                    string
	AllowRead, AllowWrite      bool
	AllowExec                  bool
	SupportsFiles, SupportsSSH bool
}

// PermissionMatrix renders the 3-column capability switch table (read/write/exec)
// used in the Secret Key authorization modal.
func PermissionMatrix(c *ui.Context, rows []PermissionMatrixRow, onToggle func(index int, capability string, enabled bool)) ui.Element {
	t := theme.Current()
	colW := float32(118)
	return ui.Column(c).FillWidth().Gap(t.Space.N6).Children(func() {
		ui.Row(c).FillWidth().Height(t.Size.ControlSm).Padding(0, t.Space.N12).
			Radius(t.Radius.ControlSm).Background(Color(t.Color.Surface.Sunken)).
			Border(t.Size.Hairline, Color(t.Color.Border.Subtle)).AlignItems(ui.Center).Gap(t.Space.N8).Children(func() {
			ui.Text(c, "已保存连接").Grow(1).MinWidth(0).FontSize(t.Font.Size.Footnote).
				FontWeight(t.Font.Weight.Medium).TextColor(Color(t.Color.Text.Secondary))
			for _, header := range []string{"文件读取", "文件写入", "命令执行"} {
				ui.Text(c, header).Width(colW).Shrink(0).FontSize(t.Font.Size.Footnote).
					FontWeight(t.Font.Weight.Medium).TextColor(Color(t.Color.Text.Secondary)).TextAlign(ui.Center)
			}
		})
		for index, row := range rows {
			rowIdx := index
			item := row
			ui.Row(c).Key(item.ID).FillWidth().Padding(t.Space.N8, t.Space.N12).
				Radius(t.Radius.Control).Background(Color(t.Color.Surface.Card)).
				Border(t.Size.Hairline, Color(t.Color.Border.Subtle)).AlignItems(ui.Center).Gap(t.Space.N8).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
					ui.Text(c, item.Name).FontSize(t.Font.Size.Small).FontWeight(t.Font.Weight.Semibold).SingleLine().Ellipsis("…")
					sub := item.Subtitle
					tone := ToneTertiary
					if item.Warning != "" {
						sub = item.Subtitle + " · " + item.Warning
						tone = ToneWarning
					}
					Text(c, sub, TextProps{Style: TextCaption, Tone: tone}).SingleLine().Ellipsis("…")
				})
				renderCell := func(capKey, capLabel string, supported, current bool) {
					ui.Box(c).Key(capKey).Width(colW).Shrink(0).Center().Children(func() {
						if !supported {
							ui.Text(c, "—").FontSize(t.Font.Size.Small).TextColor(Color(t.Color.Text.Muted))
							return
						}
						val := current
						sw := Switch(c, &val, ToggleProps{Label: item.Name + " " + capLabel})
						if sw.Changed() && onToggle != nil {
							onToggle(rowIdx, capKey, val)
						}
					})
				}
				renderCell("read", "文件读取", item.SupportsFiles, item.AllowRead)
				renderCell("write", "文件写入", item.SupportsFiles, item.AllowWrite)
				renderCell("exec", "命令执行", item.SupportsSSH, item.AllowExec)
			})
		}
	})
}

type PolicyActionItem struct {
	ID, Label, Hint string
	Deferred        bool
}

// DefaultFilePolicyActions returns the 7 file interception actions defined in the MCP spec.
func DefaultFilePolicyActions() []PolicyActionItem {
	return []PolicyActionItem{
		{ID: "browse", Label: "浏览", Hint: "目录与属性"},
		{ID: "read", Label: "读取", Hint: "正文与传输"},
		{ID: "create", Label: "创建", Hint: "新建与上传"},
		{ID: "write", Label: "写入", Hint: "修改与覆盖"},
		{ID: "rename", Label: "改名", Hint: "工具待上线", Deferred: true},
		{ID: "move", Label: "移动", Hint: "工具待上线", Deferred: true},
		{ID: "delete", Label: "删除", Hint: "工具待上线", Deferred: true},
	}
}

// PolicyActionMatrix renders the 7-action multi-select grid for a file policy path rule.
func PolicyActionMatrix(c *ui.Context, selected map[string]bool, items []PolicyActionItem, onToggle func(id string, checked bool)) ui.Element {
	t := theme.Current()
	return ui.Row(c).FillWidth().Gap(t.Space.N6).AlignItems(ui.Stretch).Children(func() {
		for _, action := range items {
			item := action
			checked := selected[item.ID]
			bg := t.Color.Surface.Card
			border := t.Color.Border.Control
			fg := t.Color.Text.Primary
			hintFg := t.Color.Text.Tertiary
			if checked {
				if item.Deferred {
					bg = t.Color.Warning.Subtle
					border = t.Color.Warning.Default
					fg = t.Color.Warning.TextStrong
					hintFg = t.Color.Warning.Text
				} else {
					bg = t.Color.Danger.Subtle
					border = t.Color.Danger.Default
					fg = t.Color.Danger.Text
					hintFg = t.Color.Danger.Text
				}
			}
			tile := ui.ButtonBase(c.Key(item.ID)).Grow(1).Basis(0).MinWidth(0).Padding(t.Space.N6, t.Space.N6).
				Radius(t.Radius.ControlSm).Background(Color(bg)).Border(t.Size.Hairline, Color(border)).
				Column().AlignItems(ui.Start).Gap(t.Space.N2).FocusRing(false).
				Role(ui.RoleCheckBox).Checked(checked).Label("禁止" + item.Label)
			if tile.Hovered() && !checked {
				tile.Background(Color(t.Color.State.Hover))
			}
			if tile.FocusVisible() {
				Shadow(tile, t.C.Button.FocusShadow)
			}
			tile.Children(func() {
				ui.Row(c).Gap(t.Space.N4).AlignItems(ui.Center).PassThrough().Children(func() {
					if checked {
						ui.Icon(c, IconSVG(IconCheck)).Size(t.Size.IconSm, t.Size.IconSm).TextColor(Color(fg)).PassThrough()
					} else {
						ui.Box(c).Size(t.Size.Dot, t.Size.Dot).Radius(t.Radius.Hair).
							Border(t.Size.Hairline, Color(t.Color.Border.ControlHover)).PassThrough()
					}
					ui.Text(c, item.Label).FontSize(t.Font.Size.Footnote).FontWeight(t.Font.Weight.Semibold).
						TextColor(Color(fg)).SingleLine().PassThrough()
				})
				ui.Text(c, item.Hint).FontSize(t.Font.Size.Caption).TextColor(Color(hintFg)).SingleLine().Ellipsis("…").PassThrough()
			})
			if tile.Clicked() && onToggle != nil {
				onToggle(item.ID, !checked)
			}
		}
	})
}

type DirectoryListEditorProps struct {
	Title, EmptyHint, FooterHint string
	OnAdd                        func()
	OnRemove                     func(index int)
}

// DirectoryListEditor renders a bounded local directory allowlist card with add/remove actions.
func DirectoryListEditor(c *ui.Context, paths []string, props DirectoryListEditorProps) ui.Element {
	t := theme.Current()
	return ui.Column(c).Grow(1).Basis(0).MinWidth(0).Padding(t.Space.N10).Gap(t.Space.N6).
		Radius(t.Radius.Control).Background(Color(t.Color.Surface.Sunken)).
		Border(t.Size.Hairline, Color(t.Color.Border.Subtle)).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N6).Children(func() {
			ui.Text(c, props.Title).Grow(1).MinWidth(0).FontSize(t.Font.Size.Footnote).
				FontWeight(t.Font.Weight.Semibold).TextColor(Color(t.Color.Text.Primary)).SingleLine()
			if Button(c.Key("add"), "+ 添加目录", ButtonProps{Variant: ButtonPlain, Size: SizeSm}).
				TextColor(Color(t.Color.Text.Accent)).Clicked() && props.OnAdd != nil {
				props.OnAdd()
			}
		})
		if len(paths) == 0 {
			ui.Box(c).FillWidth().Padding(t.Space.N8).Radius(t.Radius.ControlSm).
				Background(Color(t.Color.Surface.Card)).Border(t.Size.Hairline, Color(t.Color.Border.Subtle)).
				Children(func() {
					Text(c, props.EmptyHint, TextProps{Style: TextCaption, Tone: ToneTertiary})
				})
		} else {
			for idx, dir := range paths {
				removeIdx := idx
				dirPath := dir
				ui.Row(c).Key(dirPath).FillWidth().MinHeight(t.C.Button.HeightSm+2*t.Space.N4).Shrink(0).
					Padding(t.Space.N4, t.Space.N8).
					Radius(t.Radius.ControlSm).Background(Color(t.Color.Surface.Card)).
					Border(t.Size.Hairline, Color(t.Color.Border.Subtle)).AlignItems(ui.Center).Gap(t.Space.N6).Children(func() {
					ui.Text(c, dirPath).Grow(1).MinWidth(0).Font(strings.Join(t.Font.Family.Mono, ", ")).
						FontSize(t.Font.Size.Footnote).TextColor(Color(t.Color.Text.Primary)).SingleLine().Ellipsis("…")
					if Button(c.Key("rm"), "移除", ButtonProps{Variant: ButtonDangerPlain, Size: SizeSm}).Clicked() && props.OnRemove != nil {
						props.OnRemove(removeIdx)
					}
				})
			}
		}
		if props.FooterHint != "" {
			Text(c, props.FooterHint, TextProps{Style: TextCaption, Tone: ToneTertiary})
		}
	})
}

type OneTimeSecretProps struct {
	KeyName, Secret, ConfigJSON string
	Confirmed                   *bool
	OnCopySecret, OnCopyConfig  func()
}

// OneTimeSecretCard renders the post-creation one-time secret view inside the Key modal.
func OneTimeSecretCard(c *ui.Context, props OneTimeSecretProps) ui.Element {
	t := theme.Current()
	return ui.Column(c).FillWidth().Gap(t.Space.N10).Children(func() {
		Banner(c, "请复制并妥善保存密钥，关闭后无法再次查看明文。", BannerProps{
			Tone: ToneWarning, Icon: IconSVG(IconAlertCircle),
		})
		ui.Column(c).FillWidth().Gap(t.Space.N4).Children(func() {
			Text(c, "密钥明文（"+props.KeyName+"）", TextProps{Style: TextFootnote, Tone: ToneSecondary})
			copyRes := CopyField(c, props.Secret, CopyFieldProps{
				Key: "secret-copy", Label: "复制授权密钥", ButtonLabel: "复制密钥", Fill: true,
			})
			if copyRes.Copied && props.OnCopySecret != nil {
				props.OnCopySecret()
			}
		})
		ui.Column(c).FillWidth().Gap(t.Space.N4).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				Text(c, "客户端配置（请替换密钥占位符）", TextProps{Style: TextFootnote, Tone: ToneSecondary})
				ui.Spacer(c)
				if Button(c.Key("copy-cfg"), "复制配置", ButtonProps{Size: SizeSm, Icon: IconCopy}).Clicked() && props.OnCopyConfig != nil {
					props.OnCopyConfig()
				}
			})
			ui.Box(c).FillWidth().Padding(t.Space.N10).Radius(t.Radius.Control).
				Background(Color(t.Color.Surface.Sunken)).Border(t.Size.Hairline, Color(t.Color.Border.Subtle)).
				Children(func() {
					ui.Text(c, props.ConfigJSON).Font(strings.Join(t.Font.Family.Mono, ", ")).
						FontSize(t.Font.Size.Caption).TextColor(Color(t.Color.Text.Primary))
				})
		})
		if props.Confirmed != nil {
			Checkbox(c, props.Confirmed, "我已复制并妥善保存密钥")
		}
	})
}
