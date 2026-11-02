package desktop

import (
	"github.com/egoist/mygo/ui"
	"strings"
)

// localize covers the settings and session UI added to the prototype. User names,
// paths, theme names and protocol messages keep their original text.
func localize(c *ui.Context, text string) string { return localizeTag(c.Locale(), text) }
func localizeTag(tag, text string) string {
	if strings.HasPrefix(strings.ToLower(tag), "en") {
		if translated, ok := settingsEnglish[text]; ok {
			return translated
		}
	}
	return text
}

var settingsEnglish = map[string]string{
	"拆分会话":      "Ungroup sessions",
	"添加":        "Add",
	"新建":        "New",
	"连接":        "Connect",
	"编辑":        "Edit",
	"删除":        "Delete",
	"删除分组？":     "Delete group?",
	"添加主机":      "Add host",
	"添加站点":      "Add site",
	"新建分组":      "New group",
	"修改组名":      "Rename group",
	"重命名分组":     "Rename group",
	"分组名称":      "Group name",
	"创建":        "Create",
	"输入新的分组名称。": "Enter a new group name.",
	"此分组名称已存在":  "This group name already exists",
	"将主机拖入分组，整理主机目录。": "Drag hosts into groups to organize the catalog.",
	"删除分组": "Delete group",
	"连接主机": "Connect host",
	"编辑主机": "Edit host",
	"删除主机": "Delete host",
	"连接站点": "Connect site",
	"编辑站点": "Edit site",
	"删除站点": "Delete site",
	"复制地址": "Copy address",

	"删除「%s」分组，连接配置、已打开的会话和传输任务会保留。":                   "Delete group “%s”. Connection profiles, open sessions and transfers will be retained.",
	"删除「%s」分组后，组内 %d 个条目将移至「%s」。连接配置、已打开的会话和传输任务会保留。": "Delete group “%s” and move its %d items to “%s”. Connection profiles, open sessions and transfers will be retained.",

	"搜索当前页面": "Search current page",
	"设置…":    "Settings…",
	"设置":     "Settings", "通用": "General", "外观": "Appearance", "终端": "Terminal", "文件传输": "File Transfer", "快捷键": "Keyboard Shortcuts", "密钥与凭据": "Keys & Credentials", "关于": "About",
	"语言、启动与选项卡": "Language, startup and tabs", "主题、强调色与动态效果": "Theme, accent and motion", "配色、背景、字体与光标 · 修改立即生效": "Colors, background, font and cursor · Changes apply immediately", "目录、传输与同名文件": "Directories, transfers and name conflicts", "点击快捷键进行录制": "Click a shortcut to record a new binding", "私钥、ssh-agent 与已保存的密码": "Private keys, ssh-agent and saved passwords", "SSH Desk 与开源许可": "SSH Desk and open source licenses",
	"搜索设置": "Search settings", "搜索结果": "Search Results", "匹配设置名称与说明 · 修改立即生效": "Matching setting names and descriptions · Changes apply immediately", "没有匹配的设置": "No matching settings", "关闭设置 Esc": "Close settings Esc", "SSH Desk 0.1.0 · 原型": "SSH Desk 0.1.0 · Prototype",
	"语言": "Language", "更改后立即生效": "Changes apply immediately", "跟随系统": "System", "启动与选项卡": "Startup & Tabs", "启动时恢复会话": "Restore sessions on startup", "重新打开上次退出时的选项卡（不会自动输入密码）": "Reopen tabs from the previous session without submitting passwords", "关闭最后一个选项卡时": "When the last tab closes", "保留窗口": "Keep window", "关闭窗口": "Close window", "退出应用": "Quit app", "关闭活动连接前确认": "Confirm before closing an active connection",
	"应用主题": "App Theme", "浅色": "Light", "深色": "Dark", "强调色": "Accent Color", "预览": "Preview", "主按钮": "Primary", "项目一": "One", "项目二": "Two", "开关预览": "Switch preview", "辅助功能": "Accessibility", "减少动态效果": "Reduce motion", "减少位移与缩放，保留透明度和颜色过渡": "Reduce movement and scaling while retaining fades and color transitions", "始终开启": "Always on", "关闭": "Off", "窗口半透明": "Window translucency", "标题栏、玻璃栏的模糊；系统「降低透明度」开启时自动关闭": "Blur in title and glass bars; disabled when the system reduces transparency", "关闭应用内动态效果时，系统减少动态效果仍对 MyGo 内建控件生效。": "System reduced motion still applies to MyGo controls when app reduced motion is off.",
	"蓝色": "Blue", "紫色": "Purple", "粉色": "Pink", "红色": "Red", "橙色": "Orange", "黄色": "Yellow", "绿色": "Green", "石墨": "Graphite",
	"配色方案": "Color Schemes", "导入配色…": "Import Scheme…", "方案": "Mode", "跟随应用明暗": "Follow app appearance", "固定一种": "Fixed scheme", "跟随：按应用当前明暗，在下方两种配色间自动切换": "Follow: switch between the light and dark schemes with app appearance", "浅色外观时": "Light appearance", "深色外观时": "Dark appearance", "配色": "Scheme", "点击卡片：按配色自身明暗放进浅色或深色槽位。导入支持 Ghostty 主题格式。": "Click a card to assign its light or dark slot. Import accepts Ghostty theme files.", "全部 %d 种 ▾": "All %d schemes ▾", "收起 ▴": "Show fewer ▴", "☀ 浅色": "☀ Light", "☾ 深色": "☾ Dark",
	"背景": "Background", "终端背景": "Terminal background", "配色自带（默认）": "Scheme (default)", "与卡片一致": "Match card", "配色自带：任何外观都用配色的背景色。与卡片一致：用卡片底色": "Scheme: use its background in any appearance. Match card: use the card background.", "字体": "Font", "字号": "Font size", "行高": "Line height", "光标": "Cursor", "样式": "Style", "▉ 块": "▉ Block", "▏竖线": "▏Bar", "▁ 下划线": "▁ Underline", "闪烁": "Blink", "光标闪烁": "Cursor blink", "块状光标": "Block cursor", "竖线光标": "Bar cursor", "下划线光标": "Underline cursor", "回滚": "Scrollback", "回滚行数": "Scrollback lines", "行 · 每个会话": "lines · per session", "超过后丢弃最早的输出。": "Discard the oldest output beyond this limit.",
	"目录": "Directories", "默认本地目录": "Default local directory", "展开本地面板时打开的目录": "Directory opened when showing the local pane", "选择本地目录": "Choose local directory", "传输": "Transfers", "同时传输数": "Concurrent transfers", "同名冲突": "Name conflicts", "询问": "Ask", "覆盖": "Overwrite", "跳过": "Skip", "重命名": "Rename",
	"全部恢复默认": "Reset All", "操作": "Action", "恢复默认": "Reset", "恢复默认：": "Reset: ", "新建会话": "New session", "关闭选项卡": "Close tab", "重新打开关闭的选项卡": "Reopen closed tab", "切换到第 1 个选项卡": "Select tab 1", "切换到第 2 个选项卡": "Select tab 2", "切换到第 3 个选项卡": "Select tab 3", "切换到第 4 个选项卡": "Select tab 4", "上一个选项卡": "Previous tab", "下一个选项卡": "Next tab", "显示/隐藏侧栏": "Toggle sidebar", "终端模式": "Terminal mode", "文件传输模式": "File transfer mode", "拆分窗格": "Split pane", "搜索主机": "Search hosts", "会话": "Sessions", "视图": "View", "搜索": "Search", "应用": "App", "未设置": "Unassigned", "按下新的快捷键…": "Press a new shortcut…", "与「%s」冲突": "Conflicts with “%s”", "替换": "Replace",
	"0.1.0 · 原型构建": "0.1.0 · Prototype build", "复制": "Copy", "查看": "View", "开源许可": "Open Source Licenses", "当前为本地原型构建，尚未配置更新与反馈服务。": "This local prototype has no update or feedback service configured.",
	"删除配色": "Delete scheme", "不能删除内置配色": "Built-in schemes cannot be deleted", "全局终端正在使用": "Used by the global terminal", "已保存的会话正在使用": "Used by a saved session", "已打开的会话正在使用": "Used by an open session", "浅色或深色配色槽正在使用": "Used by the light or dark scheme slot",

	"私钥": "Private Keys", "导入…": "Import…", "生成…": "Generate…", "尚未导入私钥。导入后可用于 SSH 或 SFTP 会话。": "No private keys yet. Import a key to use it with SSH or SFTP sessions.",
	"%s · %d 位": "%s · %d bits", "复制私钥指纹": "Copy private key fingerprint", "复制指纹": "Copy fingerprint", "被 %d 个会话使用": "Used by %d sessions", "关闭密钥窗口": "Close key window",
	"密钥操作": "Key actions", "尚未检测 ssh-agent": "ssh-agent not checked", "正在检测 ssh-agent…": "Checking ssh-agent…", "未检测到 ssh-agent": "ssh-agent not detected", "已连接 · %d 把密钥": "Connected · %d keys", "刷新": "Refresh", "添加到 agent": "Add to agent", "使用私钥连接成功后添加到 ssh-agent": "Add a private key to ssh-agent after a successful connection", "ssh-agent 使用说明": "About ssh-agent", "已保存的密码": "Saved Passwords", "%d 个会话保存了密码": "%d sessions have saved passwords", "密码和私钥口令保存在系统钥匙串中": "Passwords and key passphrases are stored in the system credential store", "全部清除…": "Clear All…", "正在处理密钥…": "Processing key…", "重命名…": "Rename…", "复制公钥": "Copy public key", "在文件管理器中显示": "Show in file manager", "删除…": "Delete…", "删除（被 %d 个会话使用）": "Delete (used by %d sessions)", "导入私钥": "Import Private Key", "导入": "Import", "生成 SSH 密钥": "Generate SSH Key", "生成": "Generate", "重命名密钥": "Rename Key", "保存": "Save", "删除密钥": "Delete Key", "移到废纸篓": "Move to Trash", "清除已保存的密码": "Clear Saved Passwords", "全部清除": "Clear All", "清除系统钥匙串中的全部会话密码。下次连接时需要重新输入。": "Clear all session passwords from the system credential store. You will need to enter them again when connecting.", "名称": "Name", "密钥名称": "Key name", "类型": "Type", "密钥类型": "Key type", "注释": "Comment", "密钥注释": "Key comment", "口令": "Passphrase", "私钥口令": "Private key passphrase", "将口令保存在系统钥匙串": "Save passphrase in the system credential store", "正在处理…": "Processing…", "取消": "Cancel", "请填写密钥名称": "Enter a key name", "密钥已不存在": "The key no longer exists", "密钥仍被会话使用，不能删除": "This key is used by a session and cannot be deleted", "确认将「%s」的私钥移到系统废纸篓？": "Move the private key “%s” to the system Trash?",
}
