# Changelog

## 0.1.3

- macOS 正式版本已通过 Apple Developer ID 代码签名与 Apple 公证（Notarization）及票据装订，安装和自动更新不再触发 Gatekeeper 安全拦截。
- `flow-mcp` 内置助手升级为 Apple Silicon + Intel 双架构 Universal 二进制。

## 0.1.2

- 新增应用内自动更新（基于 MyGo Ed25519 签名校验与增量更新）：支持后台定时检查、顶部更新提示条、设置页更新管理、应用内更新日志与下载进度弹窗，以及菜单栏与命令面板快速检查。
- 支持自动后台下载更新、跳过指定版本，并在重启更新前安全保存所有未保存草稿。
- 开源发布至 GitHub (`Lorpaves/flow`)，附带 GitHub Actions 自动构建与发布工作流。

## 0.1.1

- 首个本地打包版本：离线任务与项目管理、富文本编辑、时间线、看板、iCloud 同步与本地 stdio MCP 服务。
