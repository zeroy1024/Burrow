# 参与贡献

欢迎通过 Issue 报告问题、讨论改进，并提交范围明确的 Pull Request。较大的行为或架构变更请先讨论目标和兼容性；安全问题按 [SECURITY.md](SECURITY.md) 私下报告。

## 开发环境

按[开发与构建](docs/development.md)安装 mise 固定的工具链，并运行 `mise install`、`mise exec -- go run .`。开发版与正式版使用独立数据目录；联调只使用自己拥有或已获授权的服务器和隔离测试数据。

## 提交变更

- 保持修改聚焦，复用现有组件和约定，同步更新受影响的使用文档。
- Go 代码保持 `gofmt` 格式；主题令牌修改遵循[生成流程](docs/theme-integration.md)，不要只改生成结果。
- 运行与改动相关的已有测试、静态检查和构建；涉及本地终端扩展时，另行检查其嵌套 module。命令见[已有检查](docs/development.md#已有检查)。
- PR 说明实际问题、修改后的行为、已运行的验证和未覆盖的环境。交叉编译通过不能替代原生系统验收。
- 不提交真实连接配置、数据库、私钥、密码、Token、原始审计或未脱敏截图。报告问题时提供版本、系统/架构、复现步骤和脱敏错误信息。

## 依赖与许可

新增或更新依赖、字体、图标、配色或原生库时，保留其版权与完整许可；同步维护 `internal/desktop/licenses/`、关于页和 `THIRD_PARTY_NOTICES.md`，然后运行：

```sh
python3 scripts/licenses.py
python3 scripts/licenses.py --check
```

提交原创贡献即表示你有权提交该内容，并同意按本项目 [MIT 许可证](LICENSE) 授权；已有第三方内容继续遵循其原许可证。
