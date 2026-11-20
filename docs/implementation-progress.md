# Linear 对齐实施与验收

目标：实现 `linear-gap-audit.md` 中全部个人本地版差距，并扩展本地 stdio MCP；保留 mygo 原生 UI。用户已授权全部实施。远端账号、团队同步与外部服务仍遵循既定本地版边界，不以占位控件冒充完成。

状态：进行中。只有实现、持久化/MCP（适用时）、自动验证和原生路径验证均充分时才标记完成。

最新剩余差距及本轮实机观察见 [当前剩余差距](linear-remaining-current.md)。复核区分未实现、待验收和预览/文档落后。2026-11-08 用户追加要求去掉顶部 tab，以及模板、草稿、提醒与周期三项导航入口；这些入口及其导航命令和快捷键退出对齐范围，自动保存/恢复与已有模型保留。

| 要求 | 实现 | 验收证据 |
| --- | --- | --- |
| 原生异步保存后的及时反馈 | 前台里程碑、项目更新、就地新建标签及任务文档本轮均自动刷新通过；历史滞后的确切条件仍未确定 | phase7 原生观察；相同旧预览置前复测也通过，不能宣称修复了已确认根因 |
| B1 跨项目搜索上下文与返回/前后切换 | 已实现；自动验证通过 | navigation_test.go：跨项目搜索及历史返回/前进 |
| B2 筛选后隐藏选择不参与批量操作 | 已实现；自动验证通过 | navigation_test.go：隐藏选择清除；batchBar 再校验可见集合 |
| B3 回收站只读详情及恢复 | 已实现；自动验证通过 | navigation_test.go：只读控件、恢复、核心层拒绝编辑 |
| 导航历史、最近访问、列表与详情并排 | 已实现；按用户要求移除顶部 tab 和关联入口，导航控制移至页头 | 旧会话仍恢复当前页面/筛选/历史，应用内多标签不再作为对齐目标 |
| 分组/次级分组/排序/字段/完成项/子任务显示配置 | 已实现；验收进行中 | ui_views.go / issue_groups.go；视图配置与渲染测试；看板细节继续核对 |
| 高级组合筛选、保存视图与持久化 | 已实现；自动验证通过，原生筛选待补 | views_test.go、internal/core/views_test.go、MCP views_test.go |
| 双月日历、相对日期、快捷日期 | 已实现；自动及原生路径通过 | 原生中文输入 7天 → 2026-11-15 → 所有更改已保存；日期测试 |
| 创建展开、可恢复草稿、模板 | 自动保存及恢复保留；模板和草稿独立导航按用户要求移除 | composition_test.go、project_actions_test.go；旧会话不会恢复已移除页面 |
| 正文块操作、附件、粘贴/IME/撤销与焦点 | 创建附件、PNG/文件/HTML 图片、正文内图片、独立副本、H4 与文件块入口已实现；评论/文档/项目更新媒体、段内换行与分隔线已补齐，表格和折叠块待补 | media_test.go、media_transfer_test.go、discussion_media_test.go、phase11–13；原生 HTML 图片及跨重启富文本复制已通过，Finder/系统截图与完整 IME 待补 |
| 周期任务、提醒 | 独立导航按用户要求移除；已有数据、任务/项目操作及 MCP 保留；原生通知待验收 | records_ui_test.go、project_actions_test.go；月末、闰年、DST、重开补建和跨进程去重测试 |
| 关系、复制、转子任务/重新指定父任务/多层子任务 | 新增五类直接关联创建及可恢复草稿，父/子导航、Space 摘要预览已实现；原生子任务与阻塞方向通过 | related_creation_test.go、issue_navigation_test.go、phase7 原生与 stdio；剩余组合验收继续 |
| 任务 ID/URL/分支/prompt 复制和深链接 | 新增标题/Markdown 与相应快捷键；系统 URL 唤起仍待验收 | issue_actions_test.go、issue_navigation_test.go；原生复制标题通过 |
| 活动、修改历史、评论、撤销属性变化 | 线程/嵌套回复及 R 已完成；Enter/⌘Enter 发送、ShiftEnter 换行已按 Linear 更正并原生通过 | 评论组合输入模拟回归通过；真实拼音 IME、完整原生撤销待补 |
| 项目里程碑、文档、更新、依赖、时间线 | 共享模型、MCP 与初版 GUI 已实现；自动回归/原生验收进行中，时间线联动/缩放已实现并进入原生验收 | 项目转换、多层结构/资源转移、依赖循环、备份恢复测试；project_records_test.go |
| 多选批量项目/标签/日期及整体拖拽 | 已实现；自动验证通过，原生待补 | issue_actions_test.go、internal/core/batch_test.go |
| 完整快捷键、导航序列、帮助、上下文命令/搜索分页 | G/O/M/N 序列、P S/L、项目复制/日期/提醒、帮助和分批搜索已实现；编辑器剩余快捷键与快速输入边界待补 | ui_commands.go、ui_shortcuts.go、commands_test.go、project_actions_test.go；不包含已移除导航的快捷键 |
| 选择器当前项定位、就地新建标签、一致菜单 | 当前值定位、翻页键、作用域过滤、就地创建并选中已实现；前台保存反馈本轮通过 | ui_picker.go、organization_ui_test.go；历史后台滞后原因未定 |
| 标签编辑、分组、项目作用域 | 共享模型、GUI、MCP 已实现；自动回归和原生创建通过，其余路径继续验收 | organization_ui_test.go、internal/core/organization_test.go、MCP records_test.go；原生保存名称/颜色/分组/项目作用域 |
| 折叠/调整侧栏、收藏文件夹 | GUI、共享模型、MCP 已实现；宽度及折叠状态重启保持已通过，原生拖入/排序待验收 | organization_ui_test.go 覆盖拖入、折叠、调宽及并发成员保留；原生创建中文文件夹及项目成员通过 |
| 主题、关闭/重开、多窗口、桌面导航 | 浅色/深色/跟随系统、关闭保存后驻留、独立多窗口和按窗口恢复已实现；原生窗口创建、切换、关闭/重开、独立草稿与同任务刷新通过 | phase14/15；desktop_manager_test.go 包含多窗口与外部存储并发、全部窗口退出等待和失败取消；顶部 tab 保持移除 |
| 细化交互：任务预览、时间线联动/缩放、项目提醒/复制/深链接/标签、评论回复 | 项目专属动作已实现；原生项目标签创建/选中/落盘通过，其余项目动作和时间线继续验收 | phase5/6/7/9 验收记录；系统 URL 唤起和实际通知尚未验证 |
| 上述共享模型的 MCP 工具与 stdio 端到端测试 | 已扩展到 71 工具（只读 24），新增 shift_schedule | 实际打包 helper stdio 覆盖排期联动、项目标签/URL、项目提醒/延后并验证落盘 |
| 旧数据升级、备份恢复、GUI/MCP 并发安全 | schema 9 支持模板独立附件及旧引用迁移；原生 v8→v9 升级通过，旧 helper 拒绝新库 | 历史备份兼容、排期原子回滚、克隆隔离及模板删除独立性回归通过 |
| 原生窗口验收、中文操作、发布打包、文档 | 待做 | |

## 当前实施约定

- 筛选变化后清除不可见选择；批量动作再次根据可见集合计算，防止旧选择被误修改。
- 回收站详情只读，恢复后才允许编辑；核心层同样拒绝修改已删除任务。
- 原审计保留为基线；此表跟踪实现，不覆盖原始证据。

## 第一批实现记录（2026-11-08）

- 新增导航、日期、视图/筛选、分组、批量与复制/链接模块；现有三处状态问题已有回归测试。
- 本地数据库升级为 schema 2（views 表）；历史 schema 1 和 backup v1 能读取。当前备份写入 v2，旧版本应用会拒绝打开较新数据库。
- 视图 MCP：list_views、get_view、query_view、save_view、delete_view。递归筛选组采用显式 JSON Schema，避免 SDK 自动推断的循环错误。
- 检查：CGO_ENABLED=0 go test ./... 多轮通过；阶段中途 go test -race ./... 通过；mygo vet 通过；实际打包 helper 的 stdio 新建任务、保存视图、查询视图通过。
- 原生预览：/tmp/flow-alignment-preview/darwin-arm64/Flow.app，数据 .local/alignment-preview；应用包仍标 0.1.1，仅供本轮开发验收，不是新发布。预览没有覆盖源目录最近的筛选文案/看板字段收尾改动，下一轮验证前需重建。
- 原生已操作：创建隔离演示工作区；显示设置切换列表与详情；打开任务，保留左侧列表；中文相对日期 7天 保存成功。旧的 /tmp/flow-delivery-011 预览已正常退出。
- Tester 截图：artifacts/implementation-2026-11-08；截图证明相应 Tester 渲染，不等于所有原生路径已验收。

下一批从共享模型与持久化草稿/历史/关系开始，继续补附件、模板、周期/提醒、多层子任务、项目管理及对应 MCP。上表尚未完成项保持完整范围；最终仍需原生全流程、紧凑窗口、中文输入、升级与签名打包验收。

## 第二批实现记录（2026-11-08）

- 数据库 schema 3 新增 records 表，持久化 12 类记录；备份 v3 包含附件字节，读旧备份 v1/v2。附件 32 MiB/个，备份 512 MiB。迁移、回滚、克隆隔离、完整备份与校验测试通过。
- 创建草稿在编辑停顿、关闭、切换和退出时保存；保存尚未完成又切换到另一草稿的最后一次输入也会继续写入。被 MCP 删除的任务若还有本地文字，转入恢复草稿，不复活已删除任务。
- 共用编辑器支持斜杠菜单、段落样式、段落移动/复制/删除、文字链接、撤销/重做。选择块格式时清除斜杠和格式变化组成单次撤销；菜单退出恢复编辑焦点。新增 editor_test.go。
- 任务详情新增附件、关联/阻塞/重复关系、评论和分页活动历史；多层子任务入口已开放。副本包含后代及附件/文档；转项目保留原任务在回收站。
- 周期任务按日/周/月/年，保存原定月日，月末收缩而后续月份恢复；重开只补最近一期。提醒持久化，支持延后与清除。桌面启动及运行时检查；MCP 可显式 run_scheduled。没有安装后台守护进程，系统通知投递尚需原生验收。
- MCP 当前 68 工具，只读模式 24 工具；新增完整模型往返、只读边界、附件分块与不覆盖导出测试。实际 app 内 helper 的 stdio 新增记录测试通过。
- 第二批原生预览改为 `/tmp/flow-alignment-preview-next/darwin-arm64/Flow.app`，仍标 0.1.1 开发版本；隔离数据 `.local/alignment-preview`。旧预览 schema 2 已正常退出，并备份到 `artifacts/implementation-2026-11-08/pre-schema3-preview.db` 后迁移。
- 原生已完成中文草稿展开、关闭、退出重启、恢复、保存模板、提交；任务 FLW-11 保留多段正文。实际提交本地 Markdown 评论、搜索保存关联关系、添加链接、用系统文件选择器导入 64 字节验收文件。中文通过粘贴验证；不把这些操作写成完整原生拼音 IME 验收。
- 自动验证包括所有 Go 测试、race 和 mygo vet；新测试覆盖草稿队列切换、删除冲突恢复、紧凑窗口、关系循环、历史冲突、评论、附件、调度及块编辑焦点。阶段中再次更新 UI 后按新增改动重跑。

仍保持完整范围：项目资料与时间线 GUI、创建时附件/媒体粘贴、创建关联任务快捷路径、标签管理与就地新建、收藏文件夹/侧栏调整、完整快捷键/上下文搜索、主题/桌面状态/多窗口，以及最终原生全流程与新版发布包。新版没有发布，原 0.1.1 DMG 不代表当前源码。

第二批收尾复验：原生斜杠菜单、键盘过滤和二级标题已验证，SQLite 中保存 heading=2。发现撤销恢复斜杠时菜单重新打开并抢走重做焦点，修复后原生撤销/重做均保留正文焦点；新增回归通过。系统文件选择器导入的 64 字节附件与源文件及 SHA-256 一致，SQLite quick_check=ok。当前预览已重建到此修正版。源码/二进制身份和验证范围记录于 [phase2-verification.json](../artifacts/implementation-2026-11-08/phase2-verification.json)。

## 第三批项目界面与差距复核（2026-11-08）

- 项目导航已连接概览、任务、里程碑、文档、更新与时间线；里程碑可分配/移除任务、完成和排序，项目可设置前置依赖。时间线当前支持月/季度、日期选择和整体移动日期条；联动和更多缩放仍保留为未完成。
- 表单保存按实际改动字段合并最新记录；已修正 Markdown 解析后的格式差异误覆盖 MCP 正文，以及只改旧更新文字却回退最新项目健康状态的问题。
- `project_records_test.go` 三项新增回归通过：资源操作、并发编辑保留/删除不复活、时间线移动/日历日期和依赖循环。全部 Go 测试、race、mygo vet 通过；最终日期宽度调整后项目回归、构建与打包 helper stdio 再次通过。
- 新开发预览 `/tmp/flow-alignment-preview-projects/darwin-arm64/Flow.app`，隔离数据仍 `.local/alignment-preview`，版本仍为开发用 0.1.1。`config/mcp.preview.json` 指向此新 helper。未发布新版 DMG。
- 原生中文里程碑、文档、项目更新落盘，时间线页面可见；SQLite quick_check=ok。发现部分异步保存后的画面需下一次输入才刷新，数据库已经提交；此问题尚未修复，项目原生流程不标记整体通过。
- 新报告 `linear-remaining-current.md` 复用经哈希验证的 REA 证据，并补查 Linear 快捷键帮助和 File 菜单。新明确的任务预览、标签恢复/固定、时间线修饰键联动、项目提醒与链接等差距已加入完整范围。
- 当前源码/二进制哈希、检查和限制见 `artifacts/implementation-2026-11-08/phase3-project-verification.json`。下一轮优先定位原生保存反馈，再继续全部未完成项。

## 第四批标签与收藏组织（2026-11-08）

- 设置页接入标签编辑、颜色、分组和项目作用域；标签选择器按当前项目过滤，并支持搜索无匹配时直接创建、选中。标签改作用域或删除时，同事务清理不兼容的任务、草稿、模板与周期计划引用；GUI 与 MCP 使用相同校验。
- 侧栏接入收藏文件夹的创建/编辑、成员选择、折叠、拖入和上下排序，支持任务、项目及保存视图。删除视图同步清理文件夹引用；文件夹改名保存保留并发新增成员。侧栏支持拖动及键盘调宽，范围 200–380、重置 224。宽度及折叠状态持久化尚未实现。
- 新增核心、GUI、MCP 回归，覆盖标签作用域及并发字段、就地创建、文件夹引用/拖入/折叠/调宽；全套 Go 测试和实际打包 helper stdio 通过。本次复核 race 命中缓存且返回 0，mygo vet 返回 0；日志保存在同阶段 artifacts 目录。
- 新预览 `/tmp/flow-alignment-preview-organize/darwin-arm64/Flow.app`，隔离数据仍 `.local/alignment-preview`；预览 MCP 配置同步指向新 helper。仍为 0.1.1 开发预览，未发布新版 DMG。
- 原生创建“原生收藏验收”文件夹并添加 Flow 1.0；设置页创建中文项目作用域标签，保存名称/颜色/分组均落盘。创建弹窗中直接新建标签也落盘并写入草稿，但再次复现按钮持续禁用、按 Tab 才更新的反馈问题，不能标记整体原生验收完成。SQLite quick_check=ok。
- 为定位异步反馈曾加临时日志及 250ms 延迟，在诊断构建中未稳定复现；诊断代码和延迟均已移除。日志保留，未认定根因或宣称修复。完整剩余范围继续见上表和 `linear-remaining-current.md`。

## 第五批评论线程与输入交互（2026-11-08）

- 评论加入不可移动的 `parentId`，支持嵌套回复；同任务引用和无循环约束在共享核心层验证。编辑保留线程，删除父评论保留后代并显示删除占位。MCP `save_comment` 和 `list_comments` 同步支持，工具数仍 68。
- 评论行提供回复入口和 R 快捷键；输入区支持 ⌘ Enter 发送、Enter 输入换行。主评论和不同回复目标的未发送文字分别保留，异步保存不会清掉期间继续输入的内容。已删除目标不能继续提交回复，文字保留。焦点在当前 Context 请求，避免多个 Tester/窗口绑定同一 Handle 时出错。
- 原生检查发现焦点转入回复区时输入框仍可能位于视口外，已增加显式滚动定位；补充长内容回归，重建后原生截图确认输入框完整可见。线程 Markdown 展示保留原有 Markdown 语义，软换行不等同强制视觉换行。
- 新核心回归覆盖 v3 迁移、v1–v3 备份、跨任务回复拒绝、循环回滚、父评论删除后编辑子回复、完整备份和重启。GUI 回归覆盖 R/⌘ Enter、回复草稿切换、删除并发和滚动；MCP 及真正 stdio 子进程覆盖回复字段落盘。
- 全部 Go 测试、race、mygo vet 通过；最后的禁用状态和滚动调整后重跑受影响 GUI 回归、mygo vet、构建与打包 helper stdio。日志位于同阶段 artifacts 目录。
- 旧预览正常退出并备份为 `pre-schema4-preview.db` 后升级；SQLite user_version=4、quick_check=ok。旧 v3 helper 对新库返回 newer_database，避免旧写入者丢失线程关系。
- 新预览 `/tmp/flow-alignment-preview-comments/darwin-arm64/Flow.app`，仍为 0.1.1 开发版、数据 `.local/alignment-preview`。原生中文回复、Enter 换行、⌘ Enter 提交、R 发起嵌套回复及退出重启保持均通过；预览 MCP 配置已同步。没有发布新版 DMG。
- 保存反馈问题继续保留：临时原子计数和窗口可见性探针的几次样本均完成主线程回调及重建，未稳定复现历史问题，不能认定修复。探针已删除，模块缓存未修改，日志保留。其余全部范围继续有效。

## 第六批桌面会话与原生菜单（2026-11-08）

- 实现固定标签、关闭其他标签、最近 20 个关闭标签的恢复，以及按标签保留页面、筛选和最多 50 步前后历史；固定项保持在前，关闭快捷键不会误关固定项。紧凑窗口新建/切换/恢复时自动滚动到当前标签。
- 同目录 `desktop-session.json` 保存桌面会话、侧栏宽度/隐藏、收藏文件夹折叠。后台原子写入，退出前等待，异步保存不会覆盖期间继续切换后的状态；失败保留旧有效文件和待保存状态。损坏/未知版本文件保留不覆盖，恢复时清理已删除任务/项目引用，不恢复批量选择。
- 原生检查发现默认 File 菜单的 ⌘W 绕过 UI 标签动作并关闭窗口。已替换为自定义原生菜单，所有标签动作调用共享逻辑；弹窗或工作区恢复期间禁用导航菜单。窗口关闭使用 ⌘⇧W，关闭窗口后的驻留与多窗口仍待实现。
- 自动验证包含完整 Go 测试、race、MyGo vet；最后菜单/滚动变化后再次跑全套测试及受影响项 race、MyGo vet。实际打包 helper stdio 通过，模型仍 schema 4，MCP 仍 68 工具。
- 原生隔离预览实测 ⌘T、固定、⌘W 保护/关闭当前项、⌘⇧T 恢复项目页面、⌘⌥W 保留固定项；退出重启恢复固定“设置”、当前“项目”、254 宽侧栏、收藏折叠和侧栏隐藏；重启后的关闭历史仍可恢复。
- 预览改为 `/tmp/flow-alignment-preview-session/darwin-arm64/Flow.app`，MCP 预览配置同步；仍是 0.1.1 开发预览，没有发布新版。完整剩余要求未缩减，见上表和当前差距报告。

## 第七批任务操作、关联草稿和实际前台反馈（2026-11-08）

- 复用 REA 留存证据，当前 Linear 1.32.8 的 ASAR 哈希仍一致。实机 J/Space 显示列表旁只读摘要卡，Enter 打开详情；据此补齐 Flow 列表和看板预览，不改变原路由/选择，外部变化使锚点消失时自动关闭。
- 新增关联、子任务、父任务、被阻塞任务、阻塞任务五种创建流程。草稿保存关系上下文；数据库/备份升级为 5，创建与关联/重新挂接父任务原子提交。目标被删时保留草稿，可解除关系再提交。GUI 和新 MCP 工具 `create_related_issue` 使用同一核心规则。
- 新增任务文档入口、标题/Markdown 复制、父子跳转及相关任务快捷键。评论按实际帮助更正为 Enter/⌘Enter 发送、ShiftEnter 换行；组合输入确认键保护通过 Tester 模拟，尚不等于真实拼音输入法验收。
- 修正异步提交后继续输入被清空的风险：快照完成只清理实际提交版本；后续编辑及解除关系后切换草稿都会继续保存。跨项目模板用于父/子任务时保留目标项目，提交时清理不兼容标签/里程碑。
- 原生复测先置前窗口：旧 phase6 预览的里程碑、项目更新、就地新建标签均在保存后自行刷新；新预览任务/评论/文档也通过。一次切换其他应用后提交的对照同样通过。此前失败证据保留，但不能再把它描述为当前前台必现故障，也尚未证明历史根因就是遮挡。
- 新预览 `/tmp/flow-alignment-preview-actions/darwin-arm64/Flow.app`，隔离数据仍 `.local/alignment-preview`。原生 Space/J/Enter、中文关联草稿关闭/退出/重启/恢复/提交 FLW-12、父子跳转、Enter 评论、任务文档、标题复制及 FLW-13 阻塞方向通过。Linear 只读检查未修改其业务数据。
- 全量 Go 测试、race、mygo vet、Tester 列表/看板截图、打包 helper 实际 stdio 通过；旧 schema4 helper 实测 `newer_database`，数据库 quick_check=ok。证据与限制见 [phase7-actions-verification.json](../artifacts/implementation-2026-11-08/phase7-actions-verification.json)。版本仍为开发用 0.1.1，未发布新版 DMG。

完整待办仍包含媒体/编辑块、创建时周期、完整导航/帮助/上下文搜索、主题与多窗口生命周期、时间线联动、项目专属动作，以及最终原生验收和发布，未标记整个目标完成。


## 第八批：命令导航与用户指定的单页面布局

- 用户追加要求去掉顶部 tab。已移除标签栏、固定/新建/切换/恢复标签入口及对应命令和快捷键；这项明确偏好覆盖此前标签页对齐目标。前进/后退、最近访问、侧栏开关移入页面标题栏；收起侧栏时避开 macOS 窗口按钮。
- 旧桌面会话继续恢复当前页面、筛选、导航历史及侧栏状态。保留旧记录格式的读取兼容，不暴露隐藏标签的切换操作。⌘W 关闭窗口。
- 新增 G/O/M/N 导航和关系序列、P S 项目状态、ShiftP/M 任务项目/里程碑、项目更新快捷键，可搜索帮助，以及当前任务/项目的上下文命令。序列在编辑文字时停用，失效第二键和超时取消。
- 命令搜索不再截断 30 条；50 条分批展示、键盘自动加载后续页，结果使用稳定 ID。同名项目增加 ID 排序，避免同名结果在重建时换位。
- Go 全量、race、mygo vet、构建与打包 helper 实际 stdio 通过。原生检查无 tab 布局、侧栏安全间距、页头前后退、⌘W 退出/重启恢复、⌘/ 中文帮助、G P、O I、编号搜索与上下文命令过滤。一次无等待的 Escape/O/I 自动操作未触发搜索，分步等待焦点稳定后通过；快速连续输入仍需补验，不掩盖这条观察。
- 最新预览：`/tmp/flow-alignment-preview-single/darwin-arm64/Flow.app`，隔离数据 `.local/alignment-preview`；MCP 预览配置同步。模型 schema 5、69 个 MCP 工具，仍为开发用 0.1.1。详情见 [phase8-commands-verification.json](../artifacts/implementation-2026-11-08/phase8-commands-verification.json)。

## 第九批项目动作与导航精简（2026-11-08）

- 按用户要求移除模板、草稿、提醒与周期三个侧栏入口、命令搜索入口和 G D / Alt C 导航快捷键；旧会话中的对应页面回退到所有任务。自动保存及从新建入口恢复草稿保留，已有记录和 MCP API 保留。
- 项目支持独立标签种类、就地多选/创建、设置页管理、提醒、起止日期快捷操作、ID/链接/标题复制及 `flow://project/...` 路由。项目页面不会被遗留的任务焦点抢走动作。
- 数据库及备份升级为 6；项目标签与任务标签不可混用，标签种类创建后不可改变。提醒只允许绑定一个任务或项目；删除期间不触发，恢复后补发，延后保留所属项。MCP 共 70 工具（只读 24）。
- 全量测试、race、mygo vet、构建、打包 helper 实际 stdio 通过。新增项目参数另经 SDK 测试；Tester 覆盖项目复制/日期/提醒/标签、旧页面清理和草稿恢复。原生确认无顶部 tab/三项入口，P L 就地创建中文项目标签并自动选中、清空搜索、写入数据库；保留原先 Flow 1.0 文档页。
- 升级前备份 `pre-schema6-preview.db`；当前 SQLite schema 6、quick_check=ok。旧 schema5 helper 实测拒绝并返回 newer_database。当前预览 `/tmp/flow-alignment-preview-project-actions/darwin-arm64/Flow.app`，MCP 预览配置已同步，仍为开发版 0.1.1。
- 原生项目复制/日期/提醒、系统深链接唤起及通知仍待验收；其他剩余范围继续。证据见 [phase9-project-actions-verification.json](../artifacts/implementation-2026-11-08/phase9-project-actions-verification.json)。

其余个人本地版目标继续执行；顶部 tab 因用户明确要求移除，不再属于待补项。

## 第十批跨项目时间线（2026-11-08）

- 项目列表增加时间线布局；周/月/季/年、缩放按钮/键盘/手势、依赖连线、红色日期冲突、拖动连接点建立依赖、点击线查看及移除关系已接入。里程碑使用菱形，日期刻度在低缩放下保持单行。
- 排期核心操作支持 auto/all/single、多项选择、依赖链和后续里程碑，变更整体提交；跨 DST、重叠选择不重复移动、双向约束传播、错误回滚通过回归。新增 MCP shift_schedule，并以实际打包 helper 子进程验证。
- 上下选择项目，左右选择该项目与里程碑，X 切换选择，Enter 打开里程碑对应任务。新增可保存的 milestone 筛选，schema/backup 7；旧 schema6 helper 拒绝新库，升级前备份 pre-schema7-preview.db。
- 全量测试、race、mygo vet、构建及 stdio 通过。原生默认拖动、依赖点击/移除/重连、W/Y/Q/M、里程碑 Enter、季度布局重启恢复通过；结果落盘并在重启后仍存在。上一批项目标签/项目提醒现在也有真实 stdio 覆盖。
- 原生修饰键拖动、⌘滚动和等号放大仍待复核。MyGo Tester 的指针事件不会携带 HoldModifiers，修饰键分派使用共享动作测试，不能称作真实修饰键拖动验收。最终缩放按钮和刻度修复仅追加 Tester 验收；新版已恢复用户当前 Flow 1.0 任务页。
- 当前预览 `/tmp/flow-alignment-preview-timeline/darwin-arm64/Flow.app`；MCP 配置同步，71 工具（只读 24），仍为开发用 0.1.1。证据见 [phase10-timeline-verification.json](../artifacts/implementation-2026-11-08/phase10-timeline-verification.json)。整体目标仍在进行中。

## 第十一批创建附件与图片（2026-11-08）

- 草稿支持附件归属；保存、清空、提交转移与恢复副本复用同一核心模型，数据库及备份为 8。附件不产生草稿活动；提交后在任务活动中出现。CopyIssue 与恢复草稿副本重映射正文中的附件链接。
- 创建/任务编辑器的附件按钮、⌘⇧U、/file、/insert、PNG/文件剪贴板和系统拖放已接入；图片正文显示，普通文件作为可保存链接。H4 已加入块菜单。单次导入可撤销；异步读取完成时若正文已有新编辑，追加内容而不删除后来输入。
- 保留完整 pinned mygo v0.8.3 本地源码，三处小补丁提供应用粘贴回调和绘制裁剪；详情见 third_party/mygo/FLOW_PATCHES.md。普通富文本与匹配样式粘贴仍由框架处理。
- 自动覆盖草稿/文件重启、复制与链接重绑定、清空/提交归属、无文字但有附件的草稿、错误批次回滚、切换草稿后的异步完成、文字继续输入、图片撤销/重做、系统粘贴命令与拖放。源码全量/race/mygo vet、框架 richtext 测试与打包 helper stdio 均通过。
- 实机通过 ⌘⇧U→文件选择器→中文文本文件入库，Preview 复制 PNG→Flow ⌘V→正文图片，关闭/退出/重启恢复，提交 FLW-15→详情图片及文件列表。两个附件 ID 与哈希保持，旧草稿已消费，schema8 quick_check=ok。原先已有的前台标签反馈草稿没有被覆盖。
- 当前预览 /tmp/flow-alignment-preview-media/darwin-arm64/Flow.app，MCP 配置已同步，仍是开发 0.1.1；MCP 数量保持 71。验收见 [phase11-media-verification.json](../artifacts/implementation-2026-11-08/phase11-media-verification.json)。最终构建原生重开恢复 Flow 1.0 任务页（11 条），顶部 tab 与三个导航入口保持移除。
- 补查框架 Darwin/Linux/Windows arm64 build/vet、production-tag vet、js/wasm vet 均通过；counter-native 与 hello 在三个桌面平台补丁前后体积完全相同。图片渲染回归改为等待加载后的图像尺寸，避免固定延时截到占位符，单项及 race 通过。
- 框架完整 `go test ./...` 未全通过：CLI 测试生成的 replace 路径未引用空格，权限示例未找到预期英文控件。未修改的 v0.8.3 在含空格目录下复现 NativeTemplate 与 Permissions 两类失败，记录在 framework-upstream-baseline 日志；ui/richtext/markdown 及 Flow 自身全套通过。本批未扩大修改到这些上游测试。
- Finder 拖入/复制、系统截图、HTML 图像、跨正文复制和模板引用的独立媒体归属、评论/项目文档媒体，以及其他编辑器、窗口生命周期和最终发布工作继续；未将这些剩余项标记完成。

## 第十二批媒体复制与持久粘贴（2026-11-08）

- 完成 HTML 内嵌/HTTP(S) 图片的本地入库，富文本与 Markdown 粘贴重绑定附件副本，保留格式/次序/单步撤销；延迟完成不覆盖后来编辑，图片空草稿不再被当作空内容。
- 模板附件归属和模板应用复制共用 core 规则；旧数据库及备份迁移可用的共享引用。schema/backup 9，MCP 71 工具（只读 24），旧 schema8 helper 实测 newer_database。升级前备份 pre-schema9-preview.db。
- 新增真实剪贴板命令路径的 Tester 验证、图片空草稿恢复、HTTP fixture 去重/离线保存、非法整批回滚、延迟编辑/切换、模板删除独立性、旧数据库/备份迁移；全量/race/mygo vet 及最终构建 stdio 通过。
- 原生 HTML 粘贴并创建 FLW-16，复制完整正文、退出重启、粘贴并创建 FLW-17，格式/图片均保留。数据库确认两个不同附件 ID 拥有同一 fixture 字节，quick_check=ok。32-html-media.png 为 Tester 渲染；原生截图在 CUA 记录。
- 新预览 /tmp/flow-alignment-preview-transfer/darwin-arm64/Flow.app，MCP 配置同步；恢复用户的 Flow 1.0 概览。验收见 [phase12-media-transfer-verification.json](../artifacts/implementation-2026-11-08/phase12-media-transfer-verification.json)。
- 评论仍为 Markdown 文本框，文档编辑器已有富文本但没有附件目标；两者媒体、复杂块、窗口生命周期等仍在全目标中。本批没有新增 REA/IDA 覆盖，没有发布新版或宣称整体完成。


## 第十三批评论与文档媒体（2026-11-08）

- 评论改用富文本编辑器，接入格式工具、块操作、图片/附件、HTML/PNG/文件粘贴与拖入；保留独立回复草稿、R、Enter/⌘Enter 发送、ShiftEnter 换行及 composition 保护。多段内容保存为 Markdown 段落；模拟输入法覆盖没有降为仅粘贴测试。
- 评论、任务/项目文档及项目更新使用独立编辑文档；附件归属于包含它们的任务或项目。异步导入不会误插进任务描述，继续输入或切换线程也不会覆盖其他编辑器。评论编辑区随段落及图片高度增长到上限后滚动。
- 文档及项目更新在异步保存完成后，仅关闭未继续改动的原表单；继续编辑会保留已保存 ID，下一次保存更新同一记录。切换到另一文档/更新时，旧回调不会关闭新窗口。并发 MCP 字段合并逻辑保留。
- 复制任务时，附属文档的 Markdown 链接与图片重绑定到附件副本；普通文字、行内代码和代码块原文不变。core 测试包含删除源附件后副本仍可读取，真实 stdio 包含媒体评论/文档/项目更新和副本落盘。
- 最终应用全套测试、race、mygo vet、构建及实际打包 helper stdio 通过。`33-comment-media.png` 为已检查的 Tester 渲染。未修改 mygo 框架，未改变 schema/backup 9 或 MCP 71 工具/只读 24。
- 原生在 FLW-17 粘贴加粗标题与 HTML data 图片，Enter 发送；回复使用中文、ShiftEnter 与 ⌘Enter，通过。新建“原生文档媒体验收”及设计验收项目的带图更新，保存后自动显示；退出/重启后评论、回复、文档及更新仍存在，重开文档编辑器仍显示图片。SQLite 验证 3 个独立附件 ID、准确 PNG 字节/SHA、父任务/项目归属，任务正文与 phase12 相同，quick_check=ok。
- 预览更新为 `/tmp/flow-alignment-preview-discussion/darwin-arm64/Flow.app`，MCP 预览配置同步。验收后恢复 Flow 1.0 概览（11 条任务）。原生截图在 CUA 记录内，数据及源码/二进制身份见 `phase13-native-media.json` 与 `phase13-discussion-verification.json`。
- 未发送评论仍只保留在当前进程；附件归属父任务/项目，删除评论或文档不会自动删除父项附件。Finder 原生拖入/文件复制、系统截图、真实拼音候选输入、复杂编辑块、窗口/主题及最终发布继续验收。当前仍为 0.1.1 开发预览，没有发布新版 DMG，完整目标保持进行中。


## 第十四批主题与窗口驻留（2026-11-08）

- 设置加入浅色、深色、跟随系统；本机桌面会话保存选择，旧版本仍默认深色。浅色覆盖侧栏、列表、详情、弹窗和选择器等硬编码背景，原生外观同时切换。
- ⌘W / 原生关闭按钮先刷新未保存内容，再隐藏窗口并保留进程；重新激活恢复原页面和编辑器。⌘Q 完全退出仍等待保存；保存失败取消关闭并保留窗口和草稿。保存期间继续输入会排队保存最新内容。
- 回归覆盖主题持久化、系统外观模拟变化、固定外观不受系统变化影响、关闭期间继续输入、失败重试和关闭/退出区分。全量测试、race、mygo vet、最终构建与打包 helper 实际 stdio 通过。34-light-settings/issues/create 为已检查的 Tester 渲染。
- 原生浅色设置退出重启后保留；选择跟随系统得到当前深色，再恢复显式深色。新建“窗口关闭恢复验收”，关闭后 PID 47164 仍运行，SQLite 已有中文标题/正文；重新激活保留同一进程和编辑器，继续输入并提交 FLW-18。随后 ⌘Q 退出状态 0，显式启动 PID 60672，恢复 Flow 1.0 概览（11 条）。原有前台标签反馈草稿保留，quick_check=ok。
- CUA 读取窗口可能同时激活应用；本次确认关闭后的持久化、进程驻留及激活恢复，没有录下桌面中窗口消失的截图。Dock AX 超时且可选 launch_app 不可用，重开使用 CUA getApp 激活，未冒称验证了 Dock 点击。没有更改全局系统外观，系统变化由 Tester 覆盖。
- 当前预览 `/tmp/flow-alignment-preview-desktop/darwin-arm64/Flow.app`，MCP 配置同步，schema/backup 9、71 工具/只读 24 不变。证据见 [phase14-desktop-verification.json](../artifacts/implementation-2026-11-08/phase14-desktop-verification.json)。顶部 tab 与三个独立导航入口仍保持移除。
- 独立多窗口尚未实现，下一步需隔离窗口编辑状态、草稿与会话写入，并按活动窗口分派原生菜单；复杂编辑块和其余原生验收/最终发布继续。未发布新 DMG，整体目标没有完成。


## 第十五批独立多窗口（2026-11-08）

- ⌘⇧N、文件菜单和命令搜索可新建原生窗口；“在新窗口打开当前页面”只复制导航数据，不共享编辑器、焦点、选中项或列表运行时状态。原生菜单按实际焦点分派，后台重绘不能覆盖前台菜单。
- 每个窗口使用独立 SQLite 连接、保存队列和桌面会话文件，共享工作区记录。主窗口兼容旧草稿 context，新增窗口使用稳定身份隔离创建草稿。desktop-windows.json 保存窗口清单/可见性/当前窗口及统一外观，windows/<id>/desktop-session.json 保存各窗口页面/筛选/侧栏。文件写入原子替换，损坏/未知版本文件保留不覆盖。
- ⌘Q 统一等待所有窗口及清单保存；任一失败取消全部退出，保留待保存内容。关闭仅隐藏自己的窗口，菜单可重开；退出阶段冻结焦点/激活事件，避免已关闭窗口再次收到保存。恢复备份前检查全部窗口，恢复期间阻止其他窗口操作，成功后清除各窗口旧表单。
- 并发回归发现并修正标题保存误覆盖正文：富文本加载会规范化 JSON，原先仅改标题也可能被当成正文编辑。现在记录编辑器正文基线，只有实际正文变化才参与合并；保存期间继续输入仍保留。两个 GUI 窗口的标题/正文与第三个 SQLite 写入者的优先级/收藏同时保存测试通过。
- 全套 Go 测试、race、mygo vet、最终原生构建与实际打包 helper stdio 通过。新增六项回归覆盖焦点分派、独立草稿/会话、并发字段、跨窗口退出失败/重试、损坏清单与退出时原生激活。初次交错 Tester 截图出现未变部分缺绘，改用独立完整渲染记录；35-window-one/two-create 已检查，原生窗口绘制另行验收，未改框架或认定该截图现象的根因。
- 实机新建/切换两个窗口，在同一设计项目填写两份中文草稿，⌘Q 后分别恢复并创建 FLW-19/20。关闭第二窗口后第一窗口编辑器保持，菜单显示已关闭并可重开。文件菜单将 FLW-20 打开到第三个独立窗口；该窗口改正文、原窗口改标题，双方自动刷新且不覆盖。真实 stdio helper 修改优先级，两个任务窗口都显示“高”和 MCP 活动记录。
- 最终构建保留主窗口 Flow 1.0 概览（11 条任务），两个辅助窗口关闭；完全退出/重启后只显示主窗口，窗口菜单保留已关闭标记。原有前台标签反馈草稿保留，两个验收草稿已分别消费，schema/backup 9、quick_check=ok、MCP 71/只读24 不变。
- 预览 `/tmp/flow-alignment-preview-windows/darwin-arm64/Flow.app` 与 MCP 预览配置同步。证据见 [phase15-windows-verification.json](../artifacts/implementation-2026-11-08/phase15-windows-verification.json) 和 phase15-live-stdio.json。本批没有新增 REA/IDA 覆盖或发布新版 DMG；复杂编辑块、媒体/IME、时间线/筛选/拖拽、系统链接/通知及最终交付仍继续。


## 第十六批段内换行与分隔线（2026-11-08）

- 对照 [Linear 编辑器文档](https://linear.app/docs/editor)，接入 Shift Enter 段内换行、`___` 后空格和 `/divider` 分隔线。此前 Shift Enter 会拆分段落；现在保留列表条目、行内格式和段落身份，普通 Enter 仍新建段落/列表项。
- 在本地 mygo 的 `ui/richtext` 扩展正文结构与绘制，分隔线可选择、删除、复制和撤销；文字、富文本粘贴和 IME 提交在其边界插入相邻正文。未把块伪装成外部图片。JSON 持久化结构；Markdown/HTML 导出换行和规则；RTF 的行与段落分别导入。
- Flow 的创建、详情、评论、文档与项目更新共享这些编辑能力。已有评论 Shift Enter 回归更新为段内换行；自动测试覆盖 Unicode 光标、列表、只读、粘贴/撤销、SQLite 草稿重启与提交。实际 stdio 测试新增 create/get/update/重开后的结构验证，工具数量仍为 71，schema/backup 仍为 9。
- 原生在隔离预览创建验收草稿，测试 Shift Enter、`/divider`、一步撤销/重做、列表项内换行、`___` 快捷输入；退出状态 0、重启恢复并提交 FLW-21，详情显示两个分隔线和正确的列表结构。真实 helper 读取同一任务的 Markdown 保留 `<br>` 与 `---`。原有“前台标签反馈草稿”保留。
- Flow 全量测试、race、mygo vet 和构建通过；框架 richtext/Markdown、Darwin/Linux/Windows arm64 build/vet、production 和 wasm vet 通过。完整框架测试仍有含空格路径生成 go.mod 的 CLI 错误和权限示例标签失败，保留日志，不计作全量通过。未使用 richtext 的示例 Darwin 二进制与修改前逐字节相同；Linux/Windows 大小与 phase11 基线相同。
- Tester 图 `36-divider-line-break-draft.png` 已检查；原生截图在 CUA 记录中。连续输入时曾出现截图仅显示部分更新区域，后续详情完整截图正常；原因未证实，不将其作为完整重绘验收通过。
- 当前预览 `/tmp/flow-alignment-preview-blocks-final/darwin-arm64/Flow.app`；MCP 预览配置同步。仍是开发版 0.1.1，没有生成新 DMG。详情见 `phase16-blocks-verification.json`。
- 表格、折叠段落、完整 IME、编辑器其余快捷键、媒体/时间线/高级筛选/拖拽/系统链接/通知及最终发布继续保留在完整目标内。框架默认的部分格式快捷键仍与 Linear 不同，需要统一后再验收。


## 第十七批折叠内容（2026-11-08）

- mygo 富文本增加嵌套 section 元数据和折叠布局。JSON 保留正文，HTML/Markdown 使用 details/summary；代码块中的空行通过字符引用保留。标题支持行内格式，转换为其他块会解除该层折叠并保留子内容。
- `/collapsible section` 或 `>>>` 后空格创建，Enter 进入正文、空段 Enter 退出一层。工具栏和箭头可切换折叠；导航跳过隐藏正文，边界删除先展开，整块操作包括所有子段落。只读展开是视图状态，不写回数据库。
- 回归覆盖嵌套结构、代码空行、图片引用、剪贴板、多段粘贴、光标/删除边界、末尾折叠、标题首部 Enter、撤销、块转换、整块操作及 SQLite 重启恢复。实际打包 helper 的 stdio 创建/读取/更新/重开覆盖嵌套折叠。
- 原生在隔离预览新建“原生折叠内容验收”，使用 slash 创建，粘贴中文和嵌套富文本；折叠状态下真正 Cmd+C/Cmd+V 后 SQLite 正文与原草稿一致。退出成功，重启恢复并提交 FLW-22；详情展开外层/内层不进入编辑、不改变 SQLite。原有前台标签反馈草稿保留，验收草稿已消费，schema/backup 9、71 个工具/只读24 不变，quick_check=ok。
- 中文 typeText 未产生文本，改用系统剪贴板成功；不据此声称真实 IME 通过。原生点击后的 Space 未取得 disclosure 焦点，原生键盘路径不记为通过，Tester 键盘覆盖保留。一次截图仅有更新区域，后续完整详情正常，根因未定。
- Flow 全量、race、mygo vet、三平台编译/vet、production/wasm vet 和构建 stdio 完成；框架全量仍有已记录的空格路径和权限示例失败。未使用富文本的 Darwin counter-native/hello 与改动前字节一致。最新预览与证据见 [phase17-sections-verification.json](../artifacts/implementation-2026-11-08/phase17-sections-verification.json)。表格与其余完整范围继续，未发布新版 DMG，也未新增 REA/IDA 覆盖。
