# Flow MCP · 本地 stdio

Flow MCP 是独立进程，使用官方 [MCP Go SDK v1.8.0](https://github.com/modelcontextprotocol/go-sdk/tree/v1.8.0)。客户端启动进程后，通过 stdin/stdout 交换 MCP JSON-RPC 消息。没有 HTTP 服务、监听端口、账号或 token；运行日志仅写 stderr。

## 直接使用

服务器随安装包放在：

```text
/Applications/Flow.app/Contents/Resources/flow-mcp
```

未安装时，可使用本仓库 `build/darwin-arm64/Flow.app/Contents/Resources/flow-mcp` 的绝对路径。

常见 MCP 客户端的 JSON 配置：

```json
{
  "mcpServers": {
    "flow": {
      "command": "/Applications/Flow.app/Contents/Resources/flow-mcp",
      "args": []
    }
  }
}
```

[config/mcp.local.json](../config/mcp.local.json) 已填入当前项目构建产物的绝对路径；[config/mcp.installed.json](../config/mcp.installed.json) 用于安装到 Applications 后。路径中含空格时直接保留为 JSON 字符串，不额外加 shell 引号。

当前对齐开发预览窗口使用独立演示目录 `.local/alignment-preview`。如需让 MCP 连接这个预览窗口，使用 [config/mcp.preview.json](../config/mcp.preview.json)。以上 local/installed 配置均连接正式默认工作区。

默认读取桌面端的工作区：`~/Library/Application Support/Flow/flow.db`，或在设置中移动后的数据文件夹（按默认文件夹中的 `location.json` 查找；该文件夹不可用时服务拒绝启动，不会另建空工作区）。不要求桌面应用已启动；移动数据文件夹后需重启 MCP 客户端。客户端停止后服务随 stdin 关闭而退出，不需要后台常驻服务。

```sh
# 开发构建
make mcp

# 使用另一工作区
.mygo/flow-mcp --data-dir /absolute/path/to/workspace

# 只暴露 24 个查询工具
.mygo/flow-mcp --read-only

# 版本信息，不启动协议会话
.mygo/flow-mcp --version
```

`FLOW_DATA_DIR` 环境变量也可指定工作区，`--data-dir` 优先。开发版本首次运行可能下载 SQLite 库；安装包中的服务和动态库放在同一 Resources 目录，运行不需要下载。

## 工具

| 类型 | 工具 |
| --- | --- |
| 工作区 | `get_workspace`：计数、语言、状态和优先级定义 |
| 项目查询 | `list_projects`、`get_project` |
| 项目写入 | `create_project`、`update_project`、`trash_project`、`restore_project`、`shift_schedule` |
| 任务查询 | `list_issues`（可按项目、里程碑、状态、优先级、标签筛选）、`get_issue` |
| 任务写入 | `create_issue`、`update_issue`、`move_issue`、`trash_issue`、`restore_issue` |
| 标签 | `list_labels`、`create_label`、`update_label`、`delete_label` |
| 保存视图 | `list_views`、`get_view`、`query_view`、`save_view`、`delete_view` |
| 草稿 | `list_drafts`、`get_draft`、`save_draft`、`submit_draft`、`delete_draft` |
| 模板 | `list_templates`、`get_template`、`save_template`、`delete_template`、`create_from_template` |
| 关系与转换 | `create_related_issue`、`list_relations`、`add_relation`、`remove_relation`、`copy_issue`、`convert_issue_to_project` |
| 评论与历史 | `list_comments`、`save_comment`、`delete_comment`、`list_activities`、`undo_activity`、`restore_description` |
| 附件 | `list_attachments`、`read_attachment`、`add_attachment`、`export_attachment`、`delete_attachment` |
| 项目资料 | `list_milestones`、`save_milestone`、`delete_milestone`、`list_documents`、`save_document`、`delete_document`、`list_project_updates`、`save_project_update`、`delete_project_update` |
| 周期与提醒 | `list_reminders`、`set_reminder`、`snooze_reminder`、`dismiss_reminder`、`list_recurrences`、`save_recurrence`、`delete_recurrence`、`run_scheduled` |
| 收藏文件夹 | `list_favorite_folders`、`save_favorite_folder`、`delete_favorite_folder` |
| 备份 | `create_backup`：把整个工作区写成新的备份文件，不覆盖已有文件，不填路径时放进工作区的 `backups` 文件夹；`restore_backup`：用备份替换整个工作区，先把当前工作区存到 `backups/before-restore-*.flow.json` |

当前源码共 71 个工具，输入包含 JSON Schema，返回同时有 JSON 文本和 `structuredContent`。业务错误以 `isError: true` 返回，失败事务不消耗编号。读写、破坏性修改和本地域的 tool annotations 已标注。`--read-only` 从工具列表及调用入口移除全部写入工具。桌面已移除模板、草稿、提醒与周期的独立导航入口，相关数据和 MCP 工具保留。

另提供 `flow://guide` 只读资源，说明字段、状态、分页和父子关系。

## 示例参数

创建项目：

```json
{"name":"Flow 下一版","status":"active","due":"2026-12-01"}
```

创建任务（`projectId` 使用创建项目返回的 ID）：

```json
{
  "title":"完善搜索体验",
  "projectId":"项目ID",
  "description":"## 验收条件\n\n- [ ] 支持中文\n- [ ] 保持键盘焦点",
  "status":"todo",
  "priority":2
}
```

创建子任务：`{"title":"中文检索测试","parentId":"FLW-12"}`。

修改任务：`{"id":"FLW-12","status":"progress","labelIds":[]}`。未提供字段保持不变；空数组清除标签，空日期清除截止日期。只有顶层任务能直接调整项目，其所有后代自动跟随；`parentId` 可设为另一任务或用空字符串解除父子关系。`milestoneId` 用于指定项目里程碑。任务可用 `start` 和 `due` 排一段日期（`start` 须早于 `due`，否则只保留截止日期），如 `{"id":"FLW-12","start":"2026-11-15","due":"2026-11-18"}`。草稿、模板、周期任务的 `form` 也接受 `start`；周期任务每次生成时保持同样的天数，结束于当次日期。

查询：`{"query":"中文","status":"todo","limit":50,"offset":0}`。支持 `projectId`、`parentId`、`priority`、`labelId`、`favorites`、`trash`、`includeArchived`。分页上限 200；返回 `items`、`total` 和 `nextOffset`（结束为 null）。

项目归档：`update_project` 参数 `{"id":"项目ID","archived":true}`。

标签支持 `name`、`color`（`#RRGGBB`）、`group` 和 `kind`。`kind` 默认为 `issue`，创建后不可改变。任务标签可设 `projectId`：为空表示工作区通用，非空时只适用于该项目的任务。同一种类、作用域和分组内不允许名称大小写重复。修改作用域会同步移除不兼容任务、草稿、模板及周期计划中的标签引用，删除标签同样清理引用。

项目标签使用 `create_label {"kind":"project","name":"发布"}` 创建，不接受 `projectId` 作用域；`list_labels {"kind":"project"}` 查询。省略 `kind` 仍只返回任务标签。`create_project` 和 `update_project` 支持 `labelIds`；更新传空数组清除项目标签，省略则保留。任务和项目不能混用标签种类。`get_project` 返回 `flow://project/项目ID` 的 `url`。

收藏文件夹的 `items` 使用 `{"kind":"issue","id":"FLW-12"}`、`{"kind":"project","id":"项目ID"}` 或 `{"kind":"view","id":"视图ID"}`。`save_favorite_folder` 按提供的完整成员顺序保存；更新前先读取。删除文件夹保留被收藏实体；删除保存视图会同步清理文件夹中的引用。

任务支持 `ID` 或 `FLW-数字`；项目和标签使用实际 ID。日期为 `YYYY-MM-DD`。任务优先级：0 无，1 紧急，2 高，3 中，4 低。

## 与桌面端同时运行

所有 Flow 进程对同一工作区使用 `.flow.lock` 跨进程锁。每次写入先检查 SQLite `data_version` 并刷新模型，再校验和事务提交，避免旧缓存覆盖更新或重复分配任务编号。锁由操作系统在进程退出后释放，不需要删除锁文件。

开启 iCloud 同步后，同步由桌面端在运行时完成：MCP 写入本机数据库，桌面端打开时再同步到其他 Mac；`restore_backup` 的结果也会像普通修改一样同步出去。

桌面端约每秒检查一次外部提交。干净的列表和详情自动更新；未保存的草稿被保留，保存时按修改的字段合并。**同一字段的并发修改采用最后保存者覆盖**，没有多人文档协同编辑或字符级合并。

任务和项目删除进入回收站；标签删除会移除引用且没有标签回收站。服务不提供任意 SQL 或 shell 工具。附件导入允许读取调用者明确指定的绝对文件路径；附件导出只写入不存在的绝对路径。只读模式不提供导入或导出文件工具。它不会自动连接任何 Linear 账号。

## 验证

测试覆盖 SDK 客户端握手、73 个工具发现、参数验证、结构化返回、Markdown、子任务、分页、只读模式和资源读取；真实 stdio 子进程会创建及读取任务、视图、草稿、模板、评论和链接附件并验证落盘。项目标签、项目提醒与延后、项目 URL、排期联动现也通过实际打包 helper 的 stdio 子进程并验证落盘。草稿文件导入、按草稿列出附件、提交转移以及文件字节/MIME 落盘也已通过实际 helper stdio。另有桌面与 MCP 两个进程同时写入 60 个任务的编号和完整性检查。

发布二进制测试：

```sh
GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 \
FLOW_MCP_BINARY="$PWD/build/darwin-arm64/Flow.app/Contents/Resources/flow-mcp" \
go test ./internal/mcpserver -run TestActualStdioTransport -v
```

已额外实测协议 `2025-11-25` 的 stdio 握手，以及 MCP 修改当前打开的详情页后桌面自动刷新。见 [验收记录](verification.md) 和 [协议测试记录](../artifacts/mcp-smoke.json)。

## 新增模型的参数约定

`save_draft`、`save_template` 和 `save_recurrence` 的任务字段放在 `form` 对象中，例如：

```json
{"form":{"title":"未完成的想法","description":"## 背景\n\n文字会保留","status":"backlog"}}
```

保存模板还需 `name`；周期计划还需 `every`、`unit`（day/week/month/year）与 `nextDue`。这些 save 工具替换完整记录，更新前先读取。`submit_draft` 仅需草稿 ID；编辑草稿会按字段合并到原任务。

关系使用 `{"from":"FLW-12","to":"FLW-13","kind":"blocks"}`，kind 支持 related/blocks/duplicate，阻塞和父子关系拒绝循环。副本可设置 `includeChildren:true`。评论使用 `{"issueId":"FLW-12","body":"**本地评论**"}`。

回复评论使用 `{"issueId":"FLW-12","parentId":"评论ID","body":"回复内容"}`。父评论必须存在、未删除且属于同一任务；支持嵌套回复，创建后不能移动线程。编辑只提供 `id`、`issueId` 和 `body` 即可，省略 `parentId` 会保留原线程。删除父评论保留已有回复，桌面显示删除占位；`list_comments` 默认只列未删除评论，传入 `includeDeleted:true` 可读取完整祖先关系。

当前数据库及备份版本为 9，兼容导入 v1–v8 备份和升级旧数据库。附件可归属于任务、项目、草稿或模板；v8 及更旧的 helper 会拒绝新数据库。桌面与 MCP 应使用同一批构建产物，开发预览配置已同步。

`add_attachment` 与 `list_attachments` 接受 `ownerKind: "template"`。`save_template` 会复制正文引用的本地附件到模板；`create_from_template` 再为任务建立独立副本，`delete_template` 清理模板自己的文件。打开旧工作区或导入旧备份时，会复制仍存在的模板源文件；已经丢失的源文件无法凭引用恢复，原正文仍保留。

附件使用 `ownerKind`（issue/project/draft/template）、`ownerId`，并提供 `path` 或 `url`。先用 `save_draft` 获得 ID，即可在任务提交前添加附件。`submit_draft` 在同一事务中转移附件归属并保留附件 ID；`delete_draft` 同时清理草稿的附件。

单文件上限 32 MiB；内容与数据库一起保存、备份。读取返回 base64 分块（最多 1 MiB），导出不会覆盖已有文件。备份上限 512 MiB。

提醒时间支持带时区 RFC3339、本地 `YYYY-MM-DD HH:MM` 和 `2h` 等相对时间。桌面端启动和运行时处理到期计划；单独运行 MCP 时调用 `run_scheduled`。关闭后的周期补建仅生成最近一期，月末保留原定日数并自动缩短至当月最后一天；没有安装后台守护进程。

`set_reminder` 必须在 `issueId` 与 `projectId` 中提供且只提供一个，例如 `{"projectId":"项目ID","at":"2h"}`。`snooze_reminder {"id":"提醒ID","at":"2h"}` 保留所属任务或项目并重新安排时间；已删除的所属项不会触发提醒，恢复后可补发。`list_reminders` 返回所属字段。

历史撤销只回退当时改变的属性，并在后续同字段变更时返回冲突。文件字节不复制到活动历史中；删除附件后需通过工作区备份恢复。

开发预览 0.1.1 的版本标记尚未提升，新增工具以当前源码/重新构建的 helper 为准；旧发布包仍只有此前的能力。

### 直接创建关联任务

`create_related_issue` 同一事务创建任务及关系，失败不会留下孤立任务或消耗编号。例如：

```json
{"form":{"title":"解除当前阻塞","status":"todo"},"relation":{"issueId":"FLW-12","kind":"blocking_issue"}}
```

`kind` 支持 `related`、`sub_issue`、`parent_issue`、`blocked_issue`、`blocking_issue`。`blocking_issue` 表示新任务阻塞目标；`blocked_issue` 表示目标阻塞新任务。创建父/子任务继承目标项目，传入冲突项目会被拒绝；新父任务插入目标及其原父任务之间。

`save_draft` 可带同样的 `relation`，重启后由 `submit_draft` 原子提交。更新草稿时省略 `relation` 保留原关系；`clearRelation:true` 显式解除创建关系，不能同时传 `relation`。目标已删除时提交失败并保留草稿文字，恢复目标或解除关系后可继续提交。

### 时间线排期

`shift_schedule` 与桌面拖动使用同一事务规则。例如：

```json
{"targets":[{"kind":"project","id":"项目ID"}],"days":3,"mode":"auto"}
```

`targets` 支持 project、milestone、issue，使用实际 ID；多项选择会一起移动，重复项只移动一次。`days` 是有符号日历天数，跨夏令时不按本地 24 小时累计。失败不保留部分更改。

- `auto`（默认）：项目里程碑随项目移动；与未归档的 planned 依赖项目发生日期重叠时，消耗原有间隔后调整依赖项目。向后移动推迟后续项目，向前移动提前前置项目；已开始项目保持原位。
- `all`：移动整个未归档依赖连通图，不区分状态；对于里程碑，移动同项目中该日期及之后的有日期里程碑。
- `single`：仅移动显式选中项，关联项目和里程碑保持原日期。

返回 `changed` 项列表（含 kind/id）。没有日期的字段保持未设置。桌面 Shift 拖动对应 all，⌘/Ctrl 对应 single。保存视图的筛选规则新增 `field:"milestone"`，值为里程碑 ID，支持与其他筛选组合。

评论、文档和项目更新的 `body` 仍使用 Markdown。用 `add_attachment` 把文件导入所属任务或项目后，可在正文中引用 `flow-attachment://附件ID`；图片使用 `![说明](...)`，其他文件使用 `[文件名](...)`。桌面富文本编辑和 MCP 共用这些记录，不增加工具或改变 schema 9。删除评论或文档不会删除父任务/项目的附件。

`copy_issue` 复制附属文档时会把其链接/图片目标重绑定到复制出的附件；代码块、行内代码及普通文字中的 URL 保持原文。实际打包 helper 的 stdio 测试已覆盖媒体评论、文档、项目更新以及文档副本落盘。
