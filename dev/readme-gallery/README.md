# README 原生展示构建

使用当前应用组件和静态 mock 数据，生成独立的 `Burrow Gallery.app`，用于 README 展示图。数据源为 [scene.go.txt](scene.go.txt)，后缀有意避开 `.go`，仅由 [prepare.py](prepare.py) 通过 Go overlay 编入临时应用，不参与普通构建和测试。

运行方式、场景快捷键和截图数据边界见[展示图说明](../../docs/screenshots/readme/README.md)。临时构建与 SDK 配置保留在脚本输出的 `/private/tmp/burrow-readme-gallery-*` 目录中，方便复现；不会替换现有 Burrow 安装或读取真实连接配置。
