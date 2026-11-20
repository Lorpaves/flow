# Linear 对齐复核：当前还缺什么

最新状态请看 [当前剩余差距](linear-remaining-current.md)；本文保留为历史检查点。

复核日期：2026-11-08。对象为当前 Flow 源码和本机 Linear 1.32.8。本文更新[原始审计](linear-gap-audit.md)的状态判断；原始审计保留为实施前基线。完整实施范围仍见[实施验收表](implementation-progress.md)。本轮为复核，没有改应用代码或重新发布。

> 本文保留复核时的状态快照。随后已实施共享模型、68 个 MCP 工具、草稿模板、任务关系/历史/附件、周期提醒与块编辑器；最新实现及待验收项以 [实施验收表第二批](implementation-progress.md) 为准，下面的“缺少模型/23 工具”不是当前源码状态。

## 结论

Flow 已补上导航、可配置视图、日期和批量操作的主要骨架；剩余差距集中在创建与编辑、任务之间的关系、历史记录、周期提醒和项目资料。这些能力大多还缺共享数据模型，不能仅靠增加按钮完成。也不能把已经实现但尚待原生验收的功能再次记为缺失。

## 证据范围

- **REA 静态证据复用并校验身份**：当前 `app.asar` SHA-256 仍为 `5f13f9e9784b1c5a13b761ec50657879408084e8f8ee91fbfb3c6b1aa80b0dd8`；REA `inspect_analysis_view` 从已保存的 [Evidence bundle](../artifacts/linear-audit-2026-11-08/rea-static-evidence.json)成功读取摘要。父证据 `ev_a563070e76a5b48f1c5d640753a0355e680a16e77a2d6f0cecf7c352994a5ea4`，摘要证据 `ev_0eb8ef0ea6abe38c4fd7fd6843b531af395b37bed22cd3cd7477175ff4956df5`。没有重复跑相同 ASAR 分析。
- **覆盖限制保持不变**：5 个 JavaScript 文件解析成功不等于完整业务恢复；应用图/语义图仍为 partial，有动态 IPC、语义截断和三个原生扩展的声明/实际哈希差异。桌面壳身份相同，也不保证在线业务 UI 永远不变。
- **REA 原生观察受限**：绑定主程序后，`observe_native_ui` 返回 `capability_unavailable / accessibility-denied`，要求 REA 宿主具有辅助功能权限。没有更改系统权限；改用已有 CUA 原生窗口工具观察。REA 目标已关闭。本轮没有重跑 IDA 反编译，也不把窗口观察称作 REA/IDA 恢复结果。
- **本轮实机操作**：查看项目概览和菜单；打开空白新建表单、展开/收起、周期设置、正文 `/` 菜单；查看现有任务的操作菜单和提醒选项。只在空白表单暂时输入 `/` 并清空，清除了临时周期设置、放弃未提交表单；未提交任务、评论、附件或修改现有任务属性。最后返回原项目概览。报告不复制账号、真实任务正文或评论内容。
- **Flow 证据**：重读当前源码，复跑现有定向回归测试，并查看正在运行的隔离开发预览。源码测试与原生验收分别计数。

## 仍未对齐的主要流程

| 领域 | Linear 观察 | Flow 当前缺口与代码依据 |
| --- | --- | --- |
| 创建、草稿、模板 | 本轮确认新建的 Expand/Collapse；任务菜单可见 Convert to → Template。关闭本轮未提交表单时出现 Discard 确认 | 新建编辑区固定 112 高，无展开入口；草稿只存在进程内，没有持久草稿/草稿列表/模板。`ui_projects.go:114`、`:131`；`app.go:67`、`:338`。Linear 的重启草稿恢复语义仍未实测 |
| 正文编辑、附件 | 本轮截图确认 `/` 菜单：Heading 1/2/3、Bulleted list、Numbered list、Checklist、Insert media、Insert screenshot、Attach files；新建与详情均有附件入口 | 有富文本、Markdown 和基础工具条，但没有对应斜杠菜单、块操作及附件导入/管理。详情编辑区固定 260 高并切换阅读/编辑态。`ui_issues.go:515`、`:526`、`:530`；`ui_projects.go:131`。现有中文 composition 和编辑器 undo 测试通过，不能写成两者未实现 |
| 周期任务、提醒 | 本轮新建周期设置有 First due、间隔数、day/week/month/year；提醒有相对时间搜索、1 小时/3 小时/今晚/明天/下周/一月后及 Custom | 只有截止日期，没有周期计划、提醒实体、调度与提醒入口。`internal/core/model.go:29`；`main.go:73`。未创建真实周期任务或提醒，因此生成时点、时区和服务端执行语义未验证 |
| 关系、转换、复制任务 | 本轮菜单搜索列出 Create related → Issue/Sub-issue/Parent issue/Blocked issue/Blocking issue；Mark as → Parent of/Sub-issue of/Related to/Blocked by/Blocking/Duplicate of；Convert to → Project/Template/Recurring issue；还有 Make a copy | 无关系实体和相应操作；子任务仍限制一级、同项目；不能将既有任务重新挂接父任务、复制成新任务或转换成项目/模板。`internal/core/model.go:165`、`:179`；`ui_issues.go:298`、`:548`；`internal/mcpserver/server.go:87`。已有复制 ID/URL/分支/prompt，不能与复制任务混淆。Linear 层级上限、跨项目规则未实测 |
| 活动、历史、评论 | 本轮详情看到 Activity、属性变化记录、评论区；任务与项目菜单均有 Show description history | 无活动/评论/版本对象和持久化，无属性级撤销。CreatedAt/UpdatedAt 不能代替历史记录。`internal/core/model.go:29`、`:61`；`internal/core/store.go:48`；`ui_issues.go:466` |
| 项目管理 | 本轮概览有 Overview/Activity/Issues、Resources、Add document or link、Write first project update、Milestone，属性还包括优先级；原审计已见 Timeline/Start date/Dependencies 显示入口 | 项目仍以名称、纯文本说明、状态、颜色、截止日期、收藏/归档为主；缺里程碑、文档/链接、项目更新、依赖、起始日期、时间线、项目优先级。`internal/core/model.go:17`；`ui_projects.go:60`、`:172`。这些入口的远端写入效果未验证 |
| 命令、键盘、选择器 | 本轮任务操作菜单可搜索并能搜索到下级动作；可见 Shift+D、Shift+H 等提示；原审计记录 G→E/A/B 导航提示 | 命令面板仍只有 4 个固定动作加项目/任务结果，任务截断 30 条；缺任务上下文动作、导航序列、快捷键帮助和搜索分页。标签选择器不能就地新建；项目状态等仍使用另一套菜单。`ui_shell.go:398`、`:420`；`ui_picker.go:218`；`ui_projects.go:183` |
| 组织与桌面状态 | 本轮仍可见收藏文件夹、新标签和侧栏折叠入口；其余桌面细节见原审计待测项 | 侧栏可折叠但固定 224 宽；无收藏文件夹；固定深色、单原生窗口，关闭窗口退出；标签/最近访问/侧栏状态未持久化。标签 UI 仅新增/删除，缺改名/改色、分组、作用域。`navigation.go:29`、`:177`；`ui_shell.go:242`、`:509`；`app.go:86`；`main.go:47`；`internal/core/model.go:50`。Linear 主题/关闭重开/多窗口行为本轮未做实机等价验证 |

上述菜单观察证明动作存在，不代表已验证每个动作的提交、撤销、冲突或同步规则。

## 已有实现，仍需验收的部分

以下不再列为“没有”：

- 跨项目搜索上下文、筛选后隐藏选择、回收站只读：原 B1/B2/B3 已修复，现有回归测试通过。
- 历史后退/前进、最近访问、应用内多标签、列表与详情并排。
- 分组/次级分组/排序/显示字段/完成项/子任务显示策略；AND/OR 组合筛选、保存视图和持久化。
- 双月日历、相对日期、快捷日期；批量项目/标签/日期及所选任务整体拖拽。
- 复制任务 ID/URL/分支/prompt 与 `flow://issue/...` 路由。
- 选择器定位当前项和 PageUp/PageDown。

仍需覆盖的原生路径包括：高级筛选与保存视图、批量拖动、看板细节、紧凑窗口、系统级 URL 唤起，以及最终发布包的完整流程。测试通过不等于这些原生路径已经全部验证。

本轮定向测试实际命令：

```sh
GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 go test ./... -run '^(TestSearchNavigationUsesGlobalContextAndHistory|TestFilteringPrunesHiddenSelectionAndTrashIsReadOnly|TestTabsKeepIndependentNavigationAndFilters|TestDatePickerCancelRelativeAndCalendarSave|TestDisplayFilterSavedViewAndRestart|TestBatchDateProjectAndSelectedDrag|TestClipboardAndDeepLinkRouting|TestChineseCompositionAndRichTextUndo|TestCreateMoreDraftResumeAndModalEscape|TestMCPViewsUseSharedQueryAndValidateNestedFilters|TestSavedViewFiltersPersistenceAndCloneIsolation|TestUpgradeVersionOneDatabaseAndBackup|TestRelativeDatesAndInvalidInput)$' -count=1 -v
```

结果为 exit code 0、13/13 PASS，无跳过：Flow/Tester 9 项、core 3 项、MCP 1 项。该命令没有覆盖实际 stdio 子进程、系统 URL 唤起、原生粘贴或新发布包；此前的相关证据仍按实施验收表单独记录。

## MCP 和当前预览的差距

当前源码为 **23 个 stdio 工具**：原 18 个加 `list_views/get_view/query_view/save_view/delete_view`。共享模型尚无草稿、模板、附件、关系、评论/历史、周期提醒、里程碑、文档和项目更新，MCP 也尚无这些工具。标签修改是例外：MCP 已有 `update_label`，GUI 还没有对应编辑界面。依据：`internal/mcpserver/server.go:132`。

本轮没有连接 Linear MCP 做协议或参数逐项对照；不能宣称两者工具完全等价。Flow 保留本地 stdio。

还有三处交付状态尚未收尾：

1. 当前运行的预览位于 `/tmp/flow-alignment-preview/darwin-arm64/Flow.app`，主程序修改时间为 2026-11-08 09:57:52 GMT；`ui_filter_values.go`、`ui_shell.go`、`ui_issues.go` 在 10:00–10:04 仍有后续修改，不能把该预览当作最新源码的完整验收。
2. `README.md:25`、`docs/mcp.md:61` 仍写 18 个工具；`README.md:73` 仍写 schema 1，而当前已是 23 工具、schema 2。文档需要随最终包更新。
3. `mygo.json` 仍标 0.1.1；当前开发预览不构成新版本发布。旧版验证报告的通过项不自动覆盖新增功能。

账号、多人协作、团队同步、Cycles、远端通知和外部集成仍属于既定个人本地版范围之外；本文没有把这些云端功能算作已设计或已完成。

## 后续顺序

先补可恢复草稿/模板、编辑器和附件，再补历史/评论与任务关系、周期提醒、项目资料；每批同步扩展共享模型和 stdio MCP。最后集中对照键盘/菜单、原生窗口与发布包。当前对齐工作保持进行中。
