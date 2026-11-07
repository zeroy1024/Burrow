# Shared UI components

[English](uikit.md) | [简体中文](uikit.zh.md)

[internal/uikit](../internal/uikit/) wraps native MyGo appearance, semantics, and UI state for composition by desktop pages. Business work belongs to the app and is not created/destroyed by component mounting.

## Component entry points

| Category | Main files |
|---|---|
| Text, icons, buttons, status | `text.go`, `icon.go`, `button.go`, `badge.go`, `spinner.go` |
| Containers and sections | `card.go`, `section.go`, `divider.go` |
| Inputs, selection, forms | `input.go`, `select.go`, `multiselect.go`, `form.go`, `toggle.go`, `chipinput.go` |
| Dialogs, menus, feedback | `modal.go`, `popover.go`, `menu.go`, `tooltip.go`, `toast.go` |
| Navigation and workspace | `rail.go`, `sider.go`, `tabs.go`, `pane.go`, `sidepanel.go` |
| Files and tables | `table.go`, `sortheader.go`, `filepane.go`, `pathbar.go` |
| Terminal, scrolling, monitoring | `terminalsurface.go`, `scrollbar.go`, `scrollfade.go`, `meter.go`, `sparkline.go` |

Props, state types, and public functions in each file are authoritative. See [themes and design tokens](theme-integration.md) for token sources and generation.

## State and input

Components retain UI state such as focus, hover, popovers, selection presentation, visibility timing, and measurements. The app retains configuration, files, network objects, credentials, and tasks.

Use stable Keys per object and stable Handles for the window lifetime. Sorting and cross-group movement preserve business identity. Cancelling native drag/drop clears the actual gesture state; an animated return offset is not a new drag.

An `Element` is valid only in the current build pass. Exit copies continue running drawing callbacks. Layout observers resolve the current control through an existing `Handle` during drawing and stop after unmount, rather than capturing an old `Element` for geometry queries.

Title bars use the SDK `TitleBar` left/right insets for system window buttons. `ShortcutDisplay` renders platform shortcut labels. Actions still use `ui.Cmd` (Command on macOS, Control elsewhere), never display strings to interpret keys.

Dialogs, menus, and tooltips compose SDK semantics and positioning. Shortcut labels are separate from event bindings. Settings explanatory text uses `SettingsLabel` to place a `HelpTip` question mark beside a group/setting name. It shows a tooltip on hover, is not clickable/focusable, and exposes its text as the icon's accessible name. Unnamed SDK icons are decorative and absent from the accessibility tree. Small current-value details, such as accent names or license types, remain below labels. Closing Settings clears temporary state while retaining reusable handles.

`Modal.Disabled` locks the entire dialog with a transparent modal input shield without stacking disabled opacity. Use `DismissDisabled` to block dismissal while preserving content interaction, such as copying and acknowledging a one-time key. Global busy phases such as saving also use input shields while retaining the current picture.

`Modal` owns entry, exit, and backdrop lifetime. User cancellation uses `RequestDismiss`, confirming unsaved changes first. Successful completion uses `ModalParts.Close` or `ModalController.Close`. Async callbacks request closure on the UI thread and retain the dialog/data until `Dismissed`. The backdrop parent's `AnimatePose.Fade` drives panel fading; the panel scales separately. Exit timing and animation use the same duration. The overlay is removed before focus is restored. SDK redraws animate intermediate frames without rebuilding business UI every frame.

Local file panes, path/search bars, and session advanced sections collapse using SDK exit copies. Exiting contents immediately leave mouse, keyboard, and accessibility interaction trees, without disabled opacity. Existing width, position, and collapse animations still transform layout.

Collapsed transfer lists likewise retain only noninteractive exit copies. Split closure uses `AnimatePose` fade/scale and rebuilds to remove panes only after completion. Key highlights and pane blur reveal advance on the `Painter` clock and request redraw only. Fixed Copied feedback wakes the UI only at expiry. App reduced-motion settings must be passed through controls' `Motion`; drag-cancel animation follows the same policy.

`Select`, `MultiSelect`, and in-window menus show selected checkmarks at row ends. `MultiSelect` chooses from supplied options and displays wrapping selected tags with remove buttons. Its popover supports repeated selection and closes on outside click or Esc.

Dropdown/menu hover and keyboard-focus highlights use neutral backgrounds and body text. Checkmarks indicate selection; dangerous items retain danger feedback.

`Text` styles cover required size/weight combinations; `Mono` changes to monospace at the same size. Pages use these styles and `Tone` instead of assembling typography/colors independently. `IconTile` standardizes icon tiles, radii, and backgrounds: `SizeSm` beside inline text, default in rows, `SizeLg` beside headings.

`Inset` is a bordered panel within dialog/card content, such as rules, tasks, or per-file results. `Tone` gives warning/failure backgrounds; `Compact` is for rows. Use `CodeBlock` for long read-only text and `Banner` for messages instead of assembling parallel inset containers. `Disclosure` creates collapsible sections in panels/drawers with rotating arrows and shared focus shadows; dialog sections use `Section`.

`Count.Max` and `Segmented.CountMax` cap displayed counts. MCP main tabs use 99: zero stays `0`, overflow is `99+`. Unconfigured counters retain full values.

## Tables and scrolling

File DataTable uses real `ui.List(...).Rows(...)` virtualization. Paths identify selection, reveal targets, and retained drag source rows. The app owns directory data, sorting, and file actions; shared UI handles display/input adaptation.

`TableViewport` supplies minimum content widths and two-axis scrolling for custom tables such as MCP. Headers and rows share an explicit width calculated from viewport/minimum width, avoiding percentage widths inside two-axis scrollers. Filter bars and headers remain above rows and use `ScrollWith` for synchronized horizontal movement. Pagination stays outside the viewport. Headers and rows use identical total padding, avoiding collapsed/misaligned columns when details narrow the table. Declare one `Column` set: `TableHeaderRow` draws headers; `ListRow{Table: true}` draws rows; `TableCell` draws cells. Set widths once. Multiline cells grow through `PaddingY`. `TableToolbar` holds filters. An `Empty` state replaces data rows and centers within the visible viewport, independent of widened content. File DataTable empties likewise center between header and footer.

`SidebarDivider`, `DrawerDivider`, and `ResizeGutter` share dragging, arrow-key steps, bounds, and handle visibility. Transfer panels and split dividers keep their own double-click reset, save-on-release, and proportional-resize input, reusing only handle appearance.

`Scrollbars` decorates existing Scroll/List nodes without replacing them or business state. Bars live in non-scrolling frames and appear on actual content movement or pointer-edge proximity. In nested scrolling, outer bars do not appear merely because a wheel event occurred under the pointer; only events caught by `Contain` reveal the inner bars. No overflow means no bar. MyGo sends each wheel event to the innermost container that can still move, without gesture locking. Reaching an inner edge can move the outer container during the same gesture, including inertia. Bounded nested scrollers use `Contain`: gestures beginning inside stop at vertical edges; gestures passing from outside or newly starting on an edge can still go to the outer scroller.

`CodeBlock` supplies bounded read-only selection/copy for complete text, with one heading/copy action per block. Callers do not repeat the heading. Its text field connects shared `Scrollbars` through `TrackScroll`, hides SDK bars, uses shared dimensions/colors/timing, and enables `Contain`. SDK text fields initially scroll to an end-positioned caret. On each new text, `CodeBlock` moves the caret to the start so the first line is shown.

Session tabs retain wheel horizontal movement, reveal after selection, and locating the trailing add button, with scrollbars always hidden. Shortcut groups use Settings rows and natural page scrolling. Fixed-width `ShortcutField` keycaps align recording/reset controls. Leaving the row's focus while recording cancels recording.

The real SSH grid uses its native terminal scrollbar; other app regions use UIKit scrollbars. Parameter differences are documented under [terminal behavior](terminal-themes.md).

## Terminal containers and backdrop

`TerminalSurface` receives session colors and background source, `TerminalChrome` adapts surrounding contrast, and `Pane` presents the container/focus. The app still owns connections and terminal objects.

`ScrollFade` supplies progressive bar blur/tint. Terminal `ViewportBackdrop` and file `TableBackdrop` supply actual adjacent contents. These backgrounds own no extra interaction/accessibility rows and do not enlarge the real input viewport or business grid.

The pinned MyGo stationary GPU path preserves only the innermost rounded clip. Terminal backdrop first limits the grid region, then applies full card radii in the innermost clip, so a later rectangular clip does not remove rounding.

## Interface languages

Component-owned default buttons, statuses, tooltips, and accessibility labels come from `internal/locales`. Business-provided names, paths, and output retain their content; components do not look up arbitrary strings for translation. `ToastProps` title/detail/action use `locales.Text` and can follow language changes while queued. See [interface languages](i18n.md) for typed messages and count plurals.
