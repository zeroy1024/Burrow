# 历史会话弹窗与设置页截图

2026-11-01 从当时的实现通过 MyGo `ui.NewTester` 的软件渲染器导出，尺寸为 1440 × 960 逻辑像素。当前使用说明见[文档索引](../../README.md)，早期接入记录见[归档](../../archive/session-settings.md)。

| 页面 | 浅色 | 深色 |
| --- | --- | --- |
| SSH 新建会话 | [查看](ssh-modal-light.png) | [查看](ssh-modal-dark.png) |
| FTP/SFTP 新建会话 | [查看](ftp-modal-light.png) | [查看](ftp-modal-dark.png) |
| 设置 · 终端 | [查看](settings-terminal-light.png) | [查看](settings-terminal-dark.png) |
| 设置 · 外观 | [查看](settings-appearance-light.png) | [查看](settings-appearance-dark.png) |

截图使用内存中的演示主机与密钥元数据，不读取真实配置、私钥或系统钥匙串。SFTP 弹窗中的“连接成功 · 48ms”是用于展示结果状态的静态样本。

这些图片用于检查绘制和布局；不包含系统窗口按钮、系统文件选择框或钥匙串交互，也不代表真实网络连接、原生交互和逐像素验收通过。
