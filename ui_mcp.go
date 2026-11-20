package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func (a *app) currentMCPStatus() (mcpStatus, bool) {
	if a.desktop != nil && a.desktop.mcp != nil {
		return a.desktop.mcp.snapshot(), true
	}
	bin, found := resolveMCPBinary()
	return mcpStatus{Enabled: true, BinaryPath: bin, BinaryFound: found}, false
}

func (a *app) openMCPDialog() {
	a.closePicker()
	a.paletteOpen = false
	a.mcpOpen = true
	if a.desktop != nil && a.desktop.mcp != nil {
		a.desktop.mcp.refreshNow()
	}
}

func (a *app) copyMCPText(text, zhNotice, enNotice string) {
	a.services.WriteClipboard(text)
	a.createdIssue = ""
	a.notice = a.tr(zhNotice, enNotice)
}

// mcpSettings renders the MCP server status & controls card inside Settings.
func (a *app) mcpSettings(c *ui.Context) {
	st, ok := a.currentMCPStatus()
	detail, warn := a.mcpStatusSummary(st, ok)
	badgeText, badgeColor := a.mcpBadge(st, ok)

	ui.Row(c.Key("mcp-status-row")).MinHeight(52).Padding(8, 16).Gap(16).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.tr("MCP 服务", "MCP server"))
				display.Tag(c, badgeText, display.TagProps{Color: badgeColor})
			})
			if detail != "" {
				color := a.tk.TextTertiary
				if warn {
					color = a.tk.Error
				}
				ui.Text(c, detail).FontSize(12).TextColor(color).SingleLine()
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
			if ok && st.Testing {
				feedback.Spin(c, feedback.SpinProps{Size: theme.Small}, nil)
			} else {
				btn(c, a.tr("测试", "Test")).Label(a.tr("测试 MCP 服务", "Test MCP server")).Disabled(!ok || !st.Enabled || !st.BinaryFound).OnClick(func() {
					if ok {
						a.desktop.mcp.runTest()
					}
				})
			}
			btn(c, a.tr("配置…", "Configure…")).Label(a.tr("管理与配置 MCP", "Configure MCP server")).OnClick(a.openMCPDialog)
			on := st.Enabled
			choice.Switch(c.Key("mcp-enabled"), &on, choice.SwitchProps{}).Label(a.tr("切换 MCP 服务开关", "Toggle MCP server")).Disabled(!ok).OnClick(func() {
				if ok {
					a.desktop.mcp.setEnabled(!st.Enabled)
				}
			})
		})
	})

	a.settingDetail(c, a.tr("MCP 只读模式", "MCP read-only mode"), a.tr("仅开放 24 个查询工具，禁止外部 AI 修改或删除数据", "Expose only 24 query tools and block external edits or deletions"), false, func() {
		on := st.ReadOnly
		choice.Switch(c.Key("mcp-readonly"), &on, choice.SwitchProps{}).Label(a.tr("切换 MCP 只读模式", "Toggle MCP read-only mode")).Disabled(!ok || !st.Enabled).OnClick(func() {
			if ok {
				a.desktop.mcp.setReadOnly(!st.ReadOnly)
			}
		})
	})

	binDetail := shortPath(st.BinaryPath)
	binWarn := false
	if !st.BinaryFound {
		binDetail = a.tr("未找到 flow-mcp 组件（开发模式请先运行 make mcp）", "flow-mcp binary not found (run make mcp in development)")
		binWarn = true
	}
	a.settingDetail(c, a.tr("MCP 可执行文件", "MCP binary"), binDetail, binWarn, func() {
		if st.BinaryFound && st.BinaryPath != "" {
			iconButton(c, "folder", a.tr("在访达中显示 flow-mcp", "Show flow-mcp in Finder")).OnClick(func() {
				mygo.Shell.ShowItemInFolder(st.BinaryPath)
			})
		}
		btn(c, a.tr("复制配置", "Copy config")).Label(a.tr("复制 MCP JSON 配置", "Copy MCP JSON config")).OnClick(func() {
			a.copyMCPText(mcpJSONConfig(st.BinaryPath, a.dataDir, st.ReadOnly), "已复制 MCP JSON 配置", "Copied MCP JSON config")
		})
	})
}

func (a *app) mcpBadge(st mcpStatus, ok bool) (string, display.TagColor) {
	switch {
	case !st.Enabled:
		return a.tr("已停用", "Disabled"), display.TagGray
	case !st.BinaryFound:
		return a.tr("未就绪", "Missing"), display.TagError
	case len(st.Sessions) > 0:
		return fmt.Sprintf(a.tr("已连接 %d", "%d active"), len(st.Sessions)), display.TagGreen
	case ok && st.TestErr != "":
		return a.tr("异常", "Error"), display.TagError
	default:
		return a.tr("就绪", "Ready"), display.TagPurple
	}
}

func (a *app) mcpStatusSummary(st mcpStatus, ok bool) (string, bool) {
	if !st.Enabled {
		if n := len(st.Sessions); n > 0 {
			return fmt.Sprintf(a.tr("已停用新连接 · 仍有 %d 个旧会话运行中（可在配置中断开）", "Disabled for new clients · %d existing session(s) still open"), n), false
		}
		return a.tr("已停用 · 外部客户端将无法连接此工作区", "Disabled · external clients cannot connect to this workspace"), false
	}
	if !st.BinaryFound {
		return a.tr("未找到内置 flow-mcp 服务程序", "Bundled flow-mcp binary is missing"), true
	}
	if st.Testing {
		return a.tr("正在进行 stdio 握手与工具列表自检…", "Running stdio handshake and tool check…"), false
	}
	if st.TestErr != "" {
		return a.mcpErrorText(st.TestErr), true
	}
	if st.Err != "" {
		return st.Err, true
	}

	var parts []string
	if n := len(st.Sessions); n > 0 {
		if n == 1 {
			label := a.tr("1 个客户端已连接", "1 client connected")
			if st.Sessions[0].Client != "" {
				label += " (" + st.Sessions[0].Client + ")"
			}
			parts = append(parts, label)
		} else {
			parts = append(parts, fmt.Sprintf(a.tr("%d 个客户端已连接", "%d clients connected"), n))
		}
	} else {
		parts = append(parts, a.tr("本地 stdio · 等待客户端连接", "Local stdio · idle"))
	}

	if st.TestOK && st.TestTools > 0 {
		parts = append(parts, fmt.Sprintf(a.tr("自检通过 · %d 个工具", "Verified · %d tools"), st.TestTools))
	} else if st.ReadOnly {
		parts = append(parts, fmt.Sprintf(a.tr("只读 · %d 个查询工具", "Read-only · %d tools"), mcpToolCountRead))
	} else {
		parts = append(parts, fmt.Sprintf(a.tr("读写 · %d 个工具", "Read & write · %d tools"), mcpToolCountFull))
	}

	if len(st.ConfiguredApps) > 0 {
		parts = append(parts, a.tr("已配置：", "Configured: ")+strings.Join(st.ConfiguredApps, "、"))
	}
	if !ok {
		return strings.Join(parts, " · "), false
	}
	return strings.Join(parts, " · "), false
}

func (a *app) mcpErrorText(err string) string {
	switch err {
	case "mcp_disabled":
		return a.tr("MCP 服务已停用，请先开启开关后再测试。", "MCP server is disabled. Enable it before testing.")
	case "mcp_binary_missing":
		return a.tr("找不到 flow-mcp 可执行文件。", "flow-mcp binary was not found.")
	}
	return a.tr("自检失败：", "Test failed: ") + err
}

// mcpDialog shows live MCP sessions, diagnostics, and copy-ready client setup.
func (a *app) mcpDialog(c *ui.Context) {
	ui.DialogBase(c, &a.mcpOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(540).Padding(22).Gap(14)

		st, ok := a.currentMCPStatus()
		badgeText, badgeColor := a.mcpBadge(st, ok)

		// Header
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Size(34, 34).Radius(10).Center().Background(a.tk.PrimaryBg).Shrink(0).Children(func() {
				ui.Icon(c, lucide.Plug()).FontSize(17).TextColor(a.tk.Primary)
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.tr("MCP 服务管理", "MCP Server")).FontSize(15).FontWeight(600)
					display.Tag(c, badgeText, display.TagProps{Color: badgeColor})
				})
				mode := fmt.Sprintf(a.tr("本地 stdio · 读写模式（%d 个工具）", "Local stdio · Read & write (%d tools)"), mcpToolCountFull)
				if st.ReadOnly {
					mode = fmt.Sprintf(a.tr("本地 stdio · 只读模式（%d 个工具）", "Local stdio · Read-only (%d tools)"), mcpToolCountRead)
				}
				ui.Text(c, mode).FontSize(12).TextColor(a.tk.TextTertiary)
			})
			iconButton(c, "close", a.tr("关闭 MCP 窗口", "Close MCP dialog")).OnClick(func() { a.mcpOpen = false })
		})

		// Self-test status banner when present
		switch {
		case st.Testing:
			ui.Row(c).Padding(10, 12).Gap(10).Radius(a.tk.BorderRadius).Background(a.tk.PrimaryBg).AlignItems(ui.Center).Children(func() {
				feedback.Spin(c, feedback.SpinProps{Size: theme.Small}, nil)
				ui.Text(c, a.tr("正在启动 flow-mcp 执行 stdio 握手测试…", "Running live stdio handshake test…")).FontSize(13)
			})
		case st.TestErr != "":
			feedback.Alert(c, a.tr("MCP 自检未通过", "MCP self-test failed"), feedback.AlertProps{
				Type:        feedback.Error,
				Description: a.mcpErrorText(st.TestErr),
			})
		case st.TestOK:
			ui.Row(c).Padding(10, 12).Gap(10).Radius(a.tk.BorderRadius).Background(a.tk.SuccessBg).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, lucide.ShieldCheck()).FontSize(16).TextColor(a.tk.Success)
				ui.Text(c, fmt.Sprintf(a.tr("stdio 握手正常 · v%s · %d 个工具可用（%s）", "Handshake healthy · v%s · %d tools available (%s)"), st.TestVersion, st.TestTools, st.TestAt.Format("15:04:05"))).FontSize(13).Grow(1).MinWidth(0)
			})
		}

		// Active sessions section
		ui.Column(c).Gap(6).Children(func() {
			a.sectionHeader(c, a.tr("当前连接", "Active sessions"), fmt.Sprintf("%d", len(st.Sessions)), func() {
				if ok {
					quietButton(c, a.tr("测试连接", "Test server")).Disabled(st.Testing || !st.Enabled || !st.BinaryFound).OnClick(func() {
						a.desktop.mcp.runTest()
					})
				}
			})
			if len(st.Sessions) == 0 {
				ui.Box(c).Padding(10, 12).Radius(a.tk.BorderRadius).Background(a.tk.BgContainer).Border(1, a.hairline()).Children(func() {
					ui.Text(c, a.tr("暂无活动连接。MCP 以按需 stdio 子进程运行，外部客户端（Claude、Cursor、Antigravity 等）启动会话后会自动显示在此处。", "No active sessions. MCP runs on demand over stdio and connected clients (Claude, Cursor, Antigravity, etc.) appear here automatically.")).FontSize(12).TextColor(a.tk.TextTertiary)
				})
			} else {
				a.card(ui.Column(c)).Dividers(1, a.hairline()).Border(1, a.hairline()).Children(func() {
					zh := a.data.Preferences.Locale != "en"
					for _, sess := range st.Sessions {
						sess := sess
						ui.Row(c.Key(fmt.Sprintf("mcp-sess-%d", sess.PID))).Padding(8, 12).Gap(10).AlignItems(ui.Center).Children(func() {
							ui.Box(c).Size(7, 7).Radius(4).Background(a.tk.Success).Shrink(0)
							name := sess.Client
							if name == "" {
								name = "MCP Client"
							}
							meta := fmt.Sprintf("PID %d", sess.PID)
							if dur := formatSessionDuration(sess.StartedAt, c.Now(), zh); dur != "" {
								meta += " · " + dur
							}
							if sess.Version != "" {
								meta += " · v" + sess.Version
							}
							ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
								ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
									ui.Text(c, name).FontSize(13).FontWeight(500).SingleLine()
									if sess.ReadOnly {
										display.Tag(c, a.tr("只读", "Read-only"), display.TagProps{Color: "default"})
									} else {
										display.Tag(c, a.tr("读写", "Read & write"), display.TagProps{Color: "purple"})
									}
								})
								ui.Text(c, meta).FontSize(11).TextColor(a.tk.TextTertiary).SingleLine()
							})
							if ok {
								quietButton(c, a.tr("断开", "Disconnect")).Label(fmt.Sprintf(a.tr("断开会话 %d", "Disconnect session %d"), sess.PID)).TextColor(a.tk.ErrorText).OnClick(func() {
									a.desktop.mcp.disconnectSession(sess.PID)
								})
							}
						})
					}
				})
			}
			if len(st.ConfiguredApps) > 0 {
				ui.Row(c).Gap(6).Padding(2, 0, 0, 0).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.tr("本机已检测到配置：", "Detected in:")).FontSize(12).TextColor(a.tk.TextTertiary)
					for _, name := range st.ConfiguredApps {
						display.Tag(c.Key("cfg-"+name), name, display.TagProps{Color: "default"})
					}
				})
			}
		})

		// Client configuration snippets
		jsonSnippet := mcpJSONConfig(st.BinaryPath, a.dataDir, st.ReadOnly)
		cliSnippet := mcpClaudeCLICommand(st.BinaryPath, a.dataDir, st.ReadOnly)

		ui.Column(c).Gap(6).Children(func() {
			a.sectionHeader(c, a.tr("客户端 JSON 配置", "Client JSON config"), "", func() {
				quietButton(c, a.tr("复制 JSON", "Copy JSON")).OnClick(func() {
					a.copyMCPText(jsonSnippet, "已复制 MCP JSON 配置", "Copied MCP JSON config")
				})
			})
			ui.Scroll(c.Key("mcp-json-snippet")).MaxHeight(136).Padding(10, 12).Radius(a.tk.BorderRadius).Background(a.tk.BgContainer).Border(1, a.hairline()).Children(func() {
				ui.Text(c, jsonSnippet).Font("monospace").FontSize(12).Selectable()
			})
		})

		ui.Column(c).Gap(6).Children(func() {
			a.sectionHeader(c, a.tr("Claude Code 命令行接入", "Claude Code CLI"), "", func() {
				quietButton(c, a.tr("复制命令", "Copy command")).OnClick(func() {
					a.copyMCPText(cliSnippet, "已复制 Claude Code 接入命令", "Copied Claude Code command")
				})
			})
			ui.Box(c.Key("mcp-cli-snippet")).Padding(8, 12).Radius(a.tk.BorderRadius).Background(a.tk.BgContainer).Border(1, a.hairline()).Children(func() {
				ui.Text(c, cliSnippet).Font("monospace").FontSize(12).Selectable()
			})
		})

		dialogFooter(c, func() {
			if ok {
				btn(c, a.tr("刷新状态", "Refresh")).OnClick(func() { a.desktop.mcp.refreshNow() })
			}
			primary(c, a.tr("完成", "Done")).OnClick(func() { a.mcpOpen = false })
		})
	})
}
