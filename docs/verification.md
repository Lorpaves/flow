# Flow 验收记录

## 0.1.1 交互修正验收（2026-11-08）

对照已安装 Linear 的可见操作后，修正了列表、属性菜单、新建、详情和键盘路径。自动检查在最终源码上通过：10 个界面测试、核心与 MCP 测试、`go test -race ./...`、`go tool mygo vet .`。实际发布辅助程序通过 `FLOW_MCP_BINARY=... go test ./internal/mcpserver -run TestActualStdioTransport -count=1`。最终 staging 与只读挂载 DMG 中的 app 均通过 `codesign --verify --deep --strict`。

新增回归覆盖：

- 列表内修改状态和优先级不进入详情；菜单搜索、数字选择和 Enter 确认。
- 标签连续多选；Esc 逐层关闭菜单、创建窗口和详情；不会误退出背后的页面。
- J/K、上下键、Enter、X、Cmd 多选和 Shift 范围选择；80 条额外任务的离屏定位及手动滚动。
- 详情前后切换、保留筛选与行焦点返回列表；命令面板搜索后用 Enter 执行。
- 连续创建保留原页面；按上下文恢复本次会话内的草稿；中文 composition 和富文本撤销继续通过。
- 纯文本焦点下单字母命令不触发；行内控件以无障碍子控件暴露。

真实 macOS 窗口另行验证：J 聚焦列表、S 打开搜索菜单、数字选择后 Enter 打开详情；J 切到下一条，再用 P 打开优先级菜单；Cmd+N 创建并启用“继续创建”，创建后仍留在原详情；关闭再打开创建窗口，中文草稿完整恢复；L 打开标签，连续选择 Design 和 Feature，Esc 仅关闭菜单；中文正文粘贴、完成编辑及阅读布局；Cmd+K 搜索任务并用 Enter 打开。新增验收数据仅在 `.local/acceptance` 中。中文原生检查采用粘贴，未把自动键盘注入当成真实输入法候选窗验收。

渲染证据包括新增 `09-status-picker.png`、`10-label-picker.png`、`11-create-more.png`、`12-detail-reading.png`，以及更新后的 1000×680 紧凑窗口。已经目视检查；这些仓库图片来自 mygo Tester，实机窗口另在工具会话中观察。

边界：对齐的是上述已观察的核心交互，未验证与 Linear 所有行为完全一致；未新增远程协作、附件或活动日志。新建草稿只在当前应用进程内保留。0.1.1 未改变数据库格式或 stdio MCP 接入方式。

## 0.1.0 基线验收

日期：2026-11-08。平台：本机 macOS / Apple Silicon。依赖：mygo v0.8.3，Go 1.27.3。使用独立 `.local/acceptance` 数据目录。

## 自动检查

- `CGO_ENABLED=0 go test ./...`：通过。
- `CGO_ENABLED=1 go test -race ./...`：通过。
- `go tool mygo vet .`：通过。
- `go tool mygo build -skip-notarize`：生成 app 及 DMG。

核心测试覆盖：数据库关闭重开、父子关系与独立状态、项目删除恢复、备份往返、恢复前快照、未来格式拒绝、注入 SQL 事务失败后磁盘和内存不变、编号不被失败操作消耗、禁止二级子任务、同状态重排、单独删除的子任务不被父任务恢复，以及一万条任务中的中文/正文/编号搜索。

界面测试覆盖：创建、中文正文编辑、自动保存、搜索、文本焦点与快捷键、命令面板、项目创建和概览、中英切换、看板跨列实际拖拽、回收站恢复、输入法 composition 事件、富文本格式撤销、延迟草稿保存不覆盖状态与顺序。

Tester 截图保存在 `artifacts/screenshots/`，包含 1360×880 与 1000×680 窗口。已查看列表、详情、看板和紧凑窗口的渲染结果。截图不是浏览器或 HTML 渲染。

## 原生窗口实测

通过 macOS 应用界面完成以下操作：

1. 打开打包后的 Flow，选择演示工作区。
2. 创建 `原生窗口重启验收`，粘贴中文及英文多行正文。
3. 在属性菜单改为“进行中”，在富文本编辑器追加一行后立即 `⌘ Q` 退出。
4. 重启相同数据目录，从列表重新打开任务，确认标题、状态和包含最后一次编辑的三行正文保留；窗口显示“所有更改已保存”。
5. 通过 macOS 保存面板导出 `Native-Acceptance.flow.json`，确认成功提示。
6. 切换 English，确认侧栏、设置和原生菜单切换。
7. 通过 macOS 打开面板选择备份，确认恢复后显示 11 个任务，语言回到备份中的中文。
8. 读取独立验收数据库：`PRAGMA integrity_check` 为 `ok`；恢复前自动快照确实存在，保留恢复前的英文设置。

实机中文验证采用文本粘贴；输入法候选窗的完整人工输入流程尚未实测。composition 的提交及快捷键隔离由 Tester 覆盖。真实窗口截图在本次工具会话中观察，仓库预览图片来自 Tester。

## 构建与证据边界

本地 MCP 后续验收：官方 Go SDK v1.8.0；18 个工具与只读模式测试通过，真实 stdio 子进程完成创建、读取和退出后落盘验证。两个实际进程同时写入 60 个任务，没有丢失任务或重复编号。桌面端打开 FLW-12 详情时，通过 MCP 修改标题及状态；无需重启，实际窗口自动显示新标题和“审核中”。桌面编辑回写后的读取也通过独立 stdio 请求核验。协议 2025-11-25 握手结果保存在 `artifacts/mcp-smoke.json`。

SQLite 使用官方 mygo release `sqlite-3.53.4-1` 的 darwin-arm64 资源，下载后验证 SHA-256 为 `f78fe493715c8b7b88aaebb275e47bbad831707d780843f078ed4a8f47de14d0`。依赖已随 app 分发。

签名为本地 ad-hoc，未做 Apple Developer ID 公证。测试未改变系统网络设置；离线架构依据本地 SQLite、已打包的运行依赖和无业务网络请求实现，未模拟物理断网。未验证 Intel、Windows、Linux、大规模 UI 帧率、断电恢复或系统磁盘耗尽；事务失败由 SQL 故障注入验证。

最终 DMG 已只读挂载，并对其中的 Flow.app 执行 `codesign --verify --deep --strict`，结果通过。MCP 发布二进制也重新完成 stdio 子进程测试。发布摘要与 SHA-256 见 `artifacts/release-manifest.json`；测试输出保存在 `artifacts/logs/`。源目录位于 iCloud，File Provider 会自动给展开的 `.app` 目录添加 FinderInfo，从而影响该展开副本的 strict 签名检查；独立 staging 与 DMG 内应用检查通过，分发优先使用 DMG。

Linear 调研确认的是已安装桌面壳的入口和可见管理交互，未恢复 Linear 服务端或完整业务源码。Flow 的数据关系和备份逻辑为独立设计。
