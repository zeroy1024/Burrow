# README 展示图

这些图片由当前 Burrow 原生 UI 的独立演示构建生成，使用 `Window.CapturePage` 保存窗口内容，分辨率为 2848 × 1800（1424 × 900 逻辑像素，2×）。系统菜单栏、窗口按钮和屏幕共享提示不属于窗口内容，未包含在导出图片中。

| 图片 | 场景 |
|---|---|
| [ssh-dark.png](ssh-dark.png) | 深色 SSH 分屏与服务器状态 |
| [ssh-light.png](ssh-light.png) | 浅色 SSH 分屏与服务器状态 |
| [files.png](files.png) | 本地 / 远程文件面板及三种传输状态 |
| [mcp.png](mcp.png) | MCP 操作审计及命令详情 |

主机、用户名、目录、文件、指标、传输进度和审计记录均来自 [mock 场景](../../../dev/readme-gallery/scene.go.txt)。地址使用文档示例网段 `192.0.2.0/24`；图中的已连接、实时监控、服务运行和传输状态均为静态展示数据，没有连接对应服务器或启动 MCP 监听。这些展示图不作为协议、性能或原生平台功能验收的证据。

演示构建在临时目录使用独立数据库及 SDK 配置，不读取正式版或开发版的站点、凭据和系统钥匙串。正常应用的启动与数据来源不受影响。

## 重新生成

在 macOS 上，准备现有的 `build/darwin-arm64/Burrow.app` 作为原生资源模板，再执行：

```sh
mise exec -- python3 -B dev/readme-gallery/prepare.py
```

如果没有资源模板，可先仅构建一个临时签名的 APP；该 APP 只作为图标和原生资源模板，不需要启动：

```sh
mise exec -- go tool mygo build -skip-dmg -sign -
```

脚本输出临时 `Burrow Gallery.app` 的绝对路径。打开该应用，通过菜单「展示场景」选择界面，待布局稳定后选择「保存当前展示图」（⌘⇧S），图片写入当前目录：

- ⌘1：SSH · 深色
- ⌘2：SSH · 浅色
- ⌘3：文件传输
- ⌘4：MCP 审计；点击第一条记录可展开详情后再保存

构建脚本通过 Go overlay 注入演示入口；不会把 mock 数据接入正常构建，也不修改项目模块锁文件或全局模块缓存。更改展示数据后需要重新构建演示应用。
