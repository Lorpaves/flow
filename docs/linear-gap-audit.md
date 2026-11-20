# Flow 0.1.1 与 Linear 的剩余差距

审计日期：2026-11-08。对象：当前 Flow 工作区与本机 `/Applications/Linear.app`（桌面壳 1.32.8）。结论：核心 CRUD、列表/看板、属性选择和基础快捷键已有，但导航、可配置视图、编辑和项目管理仍有明显交互差距。本轮只做审计，未修复下列问题。

## 证据与边界

- **REA 静态证据**：重新分析本机 `app.asar`，SHA-256 `5f13f9e9784b1c5a13b761ec50657879408084e8f8ee91fbfb3c6b1aa80b0dd8`。父证据 `ev_a563070e76a5b48f1c5d640753a0355e680a16e77a2d6f0cecf7c352994a5ea4`。解析 5 个 JavaScript 文件，0 个解析失败；应用图与语义图仍为 partial，动态 IPC 未解析，部分语义提取达到上限。
- **实机观察**：用原生窗口的辅助功能树和截图，只读检查 Inbox 详情、任务菜单、日期选择、全部任务列表、Display options、筛选入口、项目列表/概览及空白新建弹窗。未提交任务、评论、附件或属性修改；临时打开的空高级筛选已移除。以下 UI 结论来自实机，不归因于静态 REA 或 IDA。
- **Flow 源码与探针**：核对 UI、核心模型、MCP；三个问题用 `ui.Tester` 和合成内存数据复现，未连接用户数据库。临时测试文件已清理。
- 桌面包主要提供 Electron 壳与在线入口，具体业务交互需要运行时证据。旧调研中的 IDA 启动入口分析未在本轮重跑；本轮也没有恢复远端业务源码、同步协议或服务端实现。
- **修正旧观察的适用范围**：项目列表打开任务可进入主内容详情；本轮在 Inbox 看到列表与详情并排。因此不能把“详情一律替换主内容”当成 Linear 的统一规则。

原始静态证据：[REA bundle](../artifacts/linear-audit-2026-11-08/rea-static-evidence.json)（约 86 MB）；便于检查的 [REA 摘要](../artifacts/linear-audit-2026-11-08/rea-summary.json)；[Flow 合成探针输出](../artifacts/linear-audit-2026-11-08/flow-probes.txt)。报告只保留通用控件文字，不保存 Linear 账号、真实任务正文或项目内容。

## 应先处理的 Flow 问题

| 编号 | 已复现行为 | 影响及建议 | 源码位置 |
| --- | --- | --- | --- |
| B1 | 从项目 A 按 ⌘K 打开项目 B 的任务，任务正确，但 `projectID/pageTitle` 仍是 A；任务在原列表的索引为 -1，上一条/下一条控件消失 | 明确全局搜索的导航上下文与返回目标；头部应显示真实来源/所属项目，前后切换应基于定义好的结果集 | `ui_shell.go:397`、`app.go:202`、`ui_picker.go:257` |
| B2 | 选中任务后改变筛选，使它隐藏；批量修改仍作用于该隐藏任务 | **行为已证实，是否算缺陷取决于选择策略。** 目前只显示总选择数，缺少隐藏选择说明；应清除不可见选择，或显式显示并允许查看隐藏选择。本轮未验证 Linear 在同一情形下的策略 | `ui_issues.go:39`、`:50`、`:61`、`:339` |
| B3 | 回收站任务打开详情后仍可编辑并保存标题；同时出现“添加子任务”，提交却返回 `invalid_parent` | 回收站详情需要统一的操作限制及恢复入口，避免可点击但必然失败的操作 | `ui_issues.go:416`、`:422`、`:464`；`internal/core/model.go:153` |

探针命令：`GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 go test -run '^TestLinearAuditProbe' -count=1 -v .`。3/3 PASS 的含义是断言成功复现上述行为，**不表示问题已经修复**；也不替代原生窗口的视觉验收。

## 已确认的主要交互差距

表中的 Linear 一栏仅描述已看到的控件或行为；未执行的服务端动作不计作端到端验证。

| 优先级 | 场景 | Linear 实机证据 | Flow 当前状态与缺口 |
| --- | --- | --- | --- |
| P1 | 导航与上下文 | 顶部有最近访问、后退/前进、多个标签页、Open new tab；Inbox 可保留列表并展示详情 | 单窗口、单页面状态；`navigate` 重置查询、选择、列表状态。只有任务返回栈和当前详情序列，缺少通用历史、独立标签与上下文展示。`app.go:185`、`ui_picker.go:243`、`main.go:20` |
| P1 | 列表/看板设置 | Display options 有 Grouping、Sub-grouping、Ordering、排序方向、Completed issues、Show sub-issues、Nested sub-issues、Show empty groups、Display properties | 主要是列表/看板开关；分组和排序固定，缺少字段选择、次级分组、嵌套子任务策略及视图持久化。`ui_issues.go:116`、`:289`；`internal/core/model.go:323`、`:55` |
| P1 | 筛选与自定义视图 | Add filter 包含 Advanced filter、Dates、Relations、Content、Links 等入口；界面有 Views 和 Add new view；高级筛选可打开条件编辑容器 | 只有搜索及单个状态/优先级/标签条件，固定 AND 组合；没有日期/内容字段级条件、排除条件、保存视图模型。`ui_issues.go:28`；`internal/core/model.go:301`。本轮没有提交保存 Linear 视图，也未逐项验证其布尔运算语义 |
| P1 | 截止日期 | Shift+D 打开相对日期输入，提示 `24h, 7 days, Feb 9`；有 Tomorrow、End of this week、In one week；Custom 打开双月日历、翻月和明确保存/取消 | Flow 是 `YYYY-MM-DD` 文本输入，无日历、相对日期或快捷日期。`ui_issues.go:422`、`ui_projects.go:139` |
| P1 | 创建与正文编辑 | 新建有 Expand、More actions、Attach images/files/videos；扩展菜单有 Make recurring、Add link、Add sub-issue；已有正文有 Heading actions | Flow 有基础富文本及 Markdown 阅读；新建正文固定 112 高、详情编辑固定 260 高，缺少展开创建窗口、附件入口、周期任务和正文块操作界面。`ui_projects.go:131`、`ui_issues.go:427`。Linear 的斜杠菜单、粘贴转换、拖拽附件与草稿重启恢复尚未验证 |
| P1 | 任务动作与关系 | Issue options 有 Create related、Mark as、Convert to、Make a copy、Add document、Add link、Remind me、Show description history；详情有 Open related issues | Flow 菜单只有基础属性、收藏、子任务和回收站；模型没有关系、提醒、正文版本或附件实体。Duplicate 只是一个状态值。`ui_issues.go:260`、`internal/core/model.go:29`。本轮未展开所有关系子菜单，因此不声称已验证每种依赖方向或转换语义 |
| P1 | 复制与外部打开 | 详情可见 Copy issue URL、Copy issue ID、Copy branch name、Copy as prompt | Flow 没有相应复制动作或任务深链接路由；从外部回到某条任务的工作流尚缺。`ui_issues.go:260`、`main.go:20` |
| P2 | 活动、评论与附件 | 详情有 Activity、变更记录、评论输入、订阅、附件入口 | Flow 只有标题、属性、正文、子任务，没有事件记录或评论/附件模型。即使保持单人本地模式，修改历史和本地附件仍可独立补齐。`ui_issues.go:390`、`internal/core/model.go:59` |
| P2 | 项目管理 | 项目有 Overview / Activity / Issues；概览有 Add document or link、Write first project update、Milestone；项目 Display options 有 Timeline、Start date、Dependencies 等 | Flow 概览主要是名称、纯文本说明、状态、截止日期和任务进度；没有里程碑、项目文档/更新、依赖和时间线。`ui_projects.go:60`、`internal/core/model.go:17` |
| P2 | 键盘与命令 | 本轮观察到 G→E / G→A / G→B 导航提示、Shift+D 日期，以及任务菜单的动作快捷键 | Flow 已有 C、⌘N、⌘K、J/K、方向键、Enter、X、S/P/L；没有成套导航序列、日期/项目动作快捷键和快捷键帮助。命令面板只有 4 个固定动作加项目/任务搜索，任务截断至 30 条。`ui_shell.go:86`、`:376` |
| P2 | 侧栏与收藏组织 | Collapse navigation sidebar 和 Create new folder for favorites 可见，顶部标签可独立切换 | Flow 侧栏固定 224 宽，没有折叠、收藏文件夹或标签页。`ui_shell.go:234` |

## Flow 已确认有限制、Linear 对应细节仍需补测

这些不能与上面的实机确认差异混写成“已完整对照”。

| 领域 | Flow 源码事实 | 后续需在隔离环境验证的 Linear 行为 |
| --- | --- | --- |
| 多选与拖拽 | 批量仅状态、优先级、回收站；拖拽负载只有一个任务 ID，无批量改项目/标签/日期。`ui_issues.go:12`、`:339` | Shift/⌘范围选择、跨筛选保留策略、选中多任务整体拖动、批量属性菜单及撤销 |
| 子任务层级 | 只允许一层且父子同项目；没有重新指定父任务、解除父子或挂接既有任务入口。`internal/core/model.go:135`、`:153`；MCP `UpdateIssueInput` 无 ParentID | Linear 的层级上限、跨项目限制、转子任务/取消父子时的数据规则 |
| 草稿与模板 | 新建草稿按上下文保存在内存，无持久化草稿实体或草稿列表；无模板模型。`app.go:65`、`:322` | Linear 取消/重开、重启恢复、多草稿入口、模板套用和创建后的焦点/提示策略 |
| 选择器细节 | 打开时高亮索引总是 0；标签选择器不能就地新建；批量属性和项目状态仍用另一套菜单。`ui_picker.go:35`、`:326`；`ui_issues.go:346` | Linear 是否默认定位当前值、筛选无结果的创建行为、菜单翻页、边界键盘与关闭规则 |
| 标签管理 | Flow UI 只有新建/删除，未提供重命名/改色；MCP 已有 `update_label`，能力不一致。`ui_shell.go:487`；`internal/mcpserver/server.go:148` | Linear 标签分组、作用域、重复名称规则和管理操作 |
| 桌面细节 | Flow 固定深色；窗口关闭走退出流程，无通用多窗口状态。`app.go:79`、`main.go:34` | Linear 关闭最后窗口、重开、主题跟随、触控板返回、触觉/Force Touch 的实际行为。ASAR 中出现原生扩展名称不等于已经验证其交互 |

## MCP 与当前范围

Flow 本地 stdio 服务已有 18 个工具及 `flow://guide`，覆盖项目、任务、标签、移动和回收恢复；这不是缺失项。它与 GUI 共用的模型还没有 saved views、关系、评论、附件、里程碑、项目文档/更新、周期任务，因此这些能力在 MCP 也不可用。重新指定父任务目前同样没有更新参数。依据：`internal/mcpserver/server.go:87`、`:128`。

本轮没有对接或调用 Linear MCP，不能据此声称两个 MCP 的协议和工具参数已做逐项等价核对。本地 stdio 是用户明确要求的传输方式，应保留；补能力应扩展共享数据模型与工具，而不是改变传输方案。

账号、团队成员、多人指派、同步、通知服务、Cycles 和外部集成目前属于本地个人版之外的产品范围。Linear 实机能看到相应部分入口，但本轮未验证远端功能，不能把它们当作已经完成的技术设计。

## 建议实施次序

1. 先修 B1 / B3，明确并处理 B2 的隐藏选择策略，建立一致的导航与选择状态。
2. 补核心交互：日期选择器、显示设置、筛选/保存视图、批量操作、复制和任务深链接。
3. 补文档体验：展开创建、正文块操作、本地附件、可恢复草稿、历史记录；同时验证 IME、粘贴、撤销和焦点。
4. 再补关系、里程碑、项目文档/更新与时间线，并同步扩展 stdio MCP。

这是依据当前 Flow 缺口提出的优先级，不是 Linear 内部实现的恢复结果。未修改应用代码、重新打包或声称交互已经全面对齐。
