# Linear 1.32.8 调研与 Flow 实现借鉴

记录日期：2026-11-08。对象为本机 `/Applications/Linear.app`。本文整理本轮已经取得的 REA、IDA 和只读实机观察，不重新执行目标分析；不保存账号、项目名称、任务正文或其他用户内容。

## 证据身份与范围

| 对象或操作 | 本轮记录 |
| --- | --- |
| Bundle | `com.linear`；`CFBundleVersion` / `CFBundleShortVersionString` 均为 `1.32.8` |
| 主可执行文件 | SHA-256：`f9d244dd7e13ae5d1b3b7e7647020f735eb4ba042cdb059c2447a7ad3b7a2001`；本轮 IDA 分析 arm64 入口 |
| `Contents/Resources/app.asar` | 994,419 bytes，15 个归档条目；SHA-256：`5f13f9e9784b1c5a13b761ec50657879408084e8f8ee91fbfb3c6b1aa80b0dd8` |
| REA JavaScript 分析父证据 | `ev_e3b65fa8e61e63ecd58a5670805e06997cd2866f17754eaf73fded3d9898773f` |
| IDA `analyze_function` 证据 | `ev_1654f60d3355839f1c73d01091b3f44d7052a62ef4b7d5e517262a9dc6528e9f` |

这些 Evidence ID 标识本会话原始观察。会话关闭后，retained 记录不保证仍可通过工具读取；本文是结论及身份索引，不是完整可重放的 Evidence bundle。下列 UI 观察来自实际只读查看，不冒称为 IDA 或静态分析结果。

## 已确认的实现边界

**原生入口进入 Electron。** IDA 在 `start`，地址 `0x100000460`，观察到对 `IsRunAsNodeEnabled` / `ELECTRON_RUN_AS_NODE` 的检查，随后进入 `ElectronInitializeICUandStartNode` 或 `ElectronMain`。这说明本次分析的原生启动入口承担 Electron/Node 启动分流；它没有揭示项目、任务或同步业务的完整实现。依据为上述 IDA 函数证据。

**ASAR 壳配置了在线入口。** REA 在包内 `Constants-DFIMYa3N.js` 发现生产 host 为 `https://linear.app`，`entryUrl` 为 `${m}/auth/desktop`，其中 `m` 对应 host。`out/renderer/index.html` 的记录大小为 0。结合启动入口，可以推断桌面包主要提供 Electron 壳及在线应用入口；这不等于已取得远端前端源码，也不证明具体运行时请求、离线能力或服务器架构。依据为上述 REA 父证据。

**覆盖是部分覆盖。** 本轮成功解析 4 个 JavaScript 文件，分析选择 6 个文件，另有 8 个文件在分析范围之外。部分文件的 semantic nodes 和 unknowns 触及每文件 20,000 上限；动态 IPC 仍有未解析项，应用图标记为 partial。因此不能把“没有在本次图中出现”当作功能不存在，也不能声称恢复了完整业务数据模型、持久化方式、网络协议或调用链。

**三个原生扩展存在声明哈希与观察哈希差异。** `force_press.node`、`haptics.node`、`swipe_navigation.node` 的 companion 声明与实际文件哈希不匹配，分析以 record-and-continue 方式保留该差异。已记录的 `force_press.node` 明细如下：

| 字段 | SHA-256 |
| --- | --- |
| declared | `970cb796246f28317fea8e66227ee9f5dc674b2c409bf2e16cbb83e5e59beb5f` |
| observed | `dba5a0f26d85d0a949f8e1228ce06370648b91cf70422d48ab709a5fe0e0b6e1` |

另外两个扩展在本摘要中只记录“不匹配”结论，不补造未保留的具体哈希。原因未知；签名或 strip 只能作为待验证的可能原因，不能据此判定文件被恶意修改。本轮没有执行这些扩展，也未验证其实际行为。

## 只读实机观察与可借鉴交互

实际查看范围仅包括项目列表、一个任务详情和状态菜单，未变更数据；没有验证服务器实现、协作或同步。

| 实际看到的界面 | Flow 的实现借鉴（产品选择） |
| --- | --- |
| 深色左侧导航：搜索、创建、My issues、Projects、收藏及团队入口；顶部标签；圆角主内容面板 | 保留稳定侧栏、统一搜索/创建入口与主内容区域；个人本地版只显示有实际用途的导航 |
| 项目包含 Overview、Activity、Issues 标签；任务列表按状态分组 | 项目和任务使用统一导航；列表分组与看板共用一套状态及筛选结果 |
| 任务详情切换主内容，而非仅显示侧边抽屉；包含 breadcrumb、标题、属性 pill、正文、子任务和 Activity | 任务详情作为可返回的主页面；状态、优先级、项目和标签集中编辑；正文与子任务层次清晰 |
| 属性区可见状态、priority、assignee、project、label | 首版采用个人本地需要的字段；不因为参考界面出现 assignee 就引入未定义的多人账号系统 |

状态菜单实际列出 7 项，Flow 可保留同一工作流词汇；下列中文用于解释语义，不是从 Linear 内部 schema 提取，内部持久化值由 Flow 自行定义：

| 观察到的菜单文字 | 中文含义 |
| --- | --- |
| Backlog | 待规划 |
| Todo | 待办 |
| In Progress | 进行中 |
| In Review | 待审核 |
| Done | 已完成 |
| Canceled | 已取消 |
| Duplicate | 重复 |

项目、任务、子任务、状态、优先级、标签、列表/看板、搜索筛选、快捷键、备份及中英切换构成 Flow 的独立产品范围。数据库关系、排序方式、备份格式和本地数据是 Flow 的实现设计，不能归因于尚未观测到的 Linear 内部逻辑。

## mygo v0.8.3 技术选型

Flow 选择纯 Go 原生 UI，依赖固定为 `github.com/egoist/mygo v0.8.3`，CLI 通过 Go module 的 `tool` 指令固定在同一模块版本。`ui` 由 mygo 自绘，macOS 使用 Metal；没有 HTML、JavaScript 或 WebView。这里的“原生 UI”不意味着每个控件都是 AppKit 控件，也不意味着复制 Linear 的 Electron 架构。[v0.8.3 原生 UI 文档](https://github.com/egoist/mygo/blob/v0.8.3/docs/ui/README.md)

| 需求 | v0.8.3 已确认的能力与采用方式 |
| --- | --- |
| 长列表及属性列 | `ui.Table` / List 仅构建可见行；表格支持排序、列调宽及重排。保留稳定任务 ID 作为行身份。[Table](https://github.com/egoist/mygo/blob/v0.8.3/docs/ui/table.md) |
| 看板及排序 | `Drag`、`ui.Drop[T]`、`ui.DragOver[T]` 和 `ListState.Reorder` 支持拖放、重排及边缘滚动；业务层负责状态更新和持久化。[Drag and drop](https://github.com/egoist/mygo/blob/v0.8.3/docs/ui/drag-and-drop.md) |
| 任务正文 | `ui/richtext` 提供格式、链接、任务列表、图片、撤销、IME 及 JSON 保存；JSON 保留结构，纯文本可供搜索。编辑器能力仍须在真实窗口验证中文输入和粘贴。[Rich text editor](https://github.com/egoist/mygo/blob/v0.8.3/docs/ui/rich-text-editor.md) |
| 独立本地数据库 | Go 直接使用官方 `plugins/sqlite` 的 `Open`，不注册网页插件；以 `Execute` / `Query` / `Transaction` 实现参数化访问，显式关闭连接。该插件通过 purego 加载随包分发的 SQLite 库，不需要 cgo。[SQLite](https://github.com/egoist/mygo/blob/v0.8.3/docs/plugins/sqlite.md) |
| 中英切换 | 框架支持目录生成消息函数；Flow 首版使用集中中文/英文文案选择，并用 `App.SetLocale` 切换原生菜单语言，语言选择保存在数据库。[Languages](https://github.com/egoist/mygo/blob/v0.8.3/docs/i18n.md) |
| 自动验证 | `ui.NewTester` 可无窗口测试点击、按键、输入、拖拽、滚动、IME composition 和截图；Flow 使用 Tester 与独立的原生实机观察；发布应用未注册 automation 插件。[Testing](https://github.com/egoist/mygo/blob/v0.8.3/docs/ui/testing.md)、[Automation](https://github.com/egoist/mygo/blob/v0.8.3/docs/plugins/automation.md) |

持久化路径使用 `mygo.App.Path(mygo.PathUserData)`，再拼接 `flow.db`。该 API 在 macOS 默认创建 `~/Library/Application Support/<App.Name()>`，按应用名称而非 bundle identifier 隔离；Flow 保持独立应用名称和数据目录，不访问 Linear 数据。Go `sqlite.Open` 要求父目录已经存在。[目录源码](https://github.com/egoist/mygo/blob/v0.8.3/paths.go)、[数据库 API 源码](https://github.com/egoist/mygo/blob/v0.8.3/plugins/sqlite/database.go)

备份需要应用层协调：v0.8.3 的已查公开 API 没有 `Backup` 方法，`Execute` 同样拒绝 `VACUUM INTO`。Flow 实际采用版本化 JSON 逻辑备份：取得跨进程工作区锁后刷新数据，原子写出完整模型；恢复先验证格式和关联关系，保留原数据逻辑备份，再通过 SQLite 单个事务替换数据。不能把正在写入的数据库及 WAL 的随意复制称为一致备份。

工具链由 `go.mod` 的 `go 1.27.3` 声明最低要求；项目开发、测试和打包入口使用 `GOTOOLCHAIN=go1.27.3` 精确指定版本。重复添加同版本 `toolchain` 行没有必要；`go` / `toolchain` 行本身不是阻止使用更高版本的锁。现有 Go 可按需缓存指定工具链，无须修改全局 `go env -w` 设置。[v0.8.3 go.mod](https://github.com/egoist/mygo/blob/v0.8.3/go.mod)、[Go Toolchains](https://go.dev/doc/toolchain)

## 验证边界

本记录确认框架接口存在及本轮观察所支持的产品模式，不代表 Flow 已完成运行验收。验收应另行覆盖：创建与修改任务、父子关系、状态/优先级/标签、列表与看板一致性、搜索筛选、键盘焦点与快捷键、中英切换、中文正文编辑、重启持久化，以及备份恢复。上游性能描述不能代替本应用的实测结果。


## 2026-11-08 原生窗口交互补查

在已安装的 Linear 窗口中，只进行了导航、打开属性选择器和空白新建窗口；未提交或修改 Linear 任务。

- 项目列表点击任务进入主内容详情，头部有当前序号、总数和前后切换按钮。
- 状态菜单包含搜索、状态图标、勾选和 1–7 提示；优先级包含搜索、图标、勾选和 0–4 提示。
- 标签菜单有搜索、颜色标识及多选；菜单显示 S / P / L 提示。
- 新建窗口有标题、正文、属性、关闭按钮和 Create more 开关。Esc 先关属性菜单，再关创建窗口，背后详情保留。
- 正文在完整详情文档中阅读，子任务跟在正文后方。

Flow 0.1.1 按这些观察修正核心操作。Flow 的创建后提示与会话草稿策略为独立实现：未在 Linear 中提交真实任务，因此不声称已确认其提交后的全部行为。附件、协作活动、复杂工作流与 Linear 的完整交互等价性不在本次实现或验证范围内。

## 后续差距审计

详见 [Flow 与 Linear 的剩余差距](linear-gap-audit.md)。后续在 Inbox 观察到列表与详情并排展示，因此本文“任务详情切换主内容”的描述只适用于当时检查的项目列表入口，不能推广为所有入口的统一行为。后续审计保留了新的 REA 静态 evidence bundle，并将实机控件观察与 Flow 合成探针结果分开记录；在线业务 UI 以观察当日为准，不由桌面壳版本单独确定。
