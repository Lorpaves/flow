package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
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

// mcpSettings renders a minimal two-row MCP card inside Settings, matching
// the visual rhythm of Appearance and iCloud Sync.
func (a *app) mcpSettings(c *ui.Context) {
	st, ok := a.currentMCPStatus()
	detail, warn := a.mcpSettingsDetail(st)

	a.settingDetail(c, "MCP", detail, warn, func() {
		btn(c, a.tr("配置…", "Configure…")).Label(a.tr("配置 MCP", "Configure MCP")).OnClick(a.openMCPDialog)
		on := st.Enabled
		choice.Switch(c.Key("mcp-enabled"), &on, choice.SwitchProps{}).Label(a.tr("切换 MCP 开关", "Toggle MCP")).Disabled(!ok).OnClick(func() {
			if ok {
				a.desktop.mcp.setEnabled(!st.Enabled)
			}
		})
	})

	a.settingRow(c, a.tr("只读模式", "Read-only"), func() {
		on := st.ReadOnly
		choice.Switch(c.Key("mcp-readonly"), &on, choice.SwitchProps{}).Label(a.tr("切换 MCP 只读模式", "Toggle MCP read-only")).Disabled(!ok || !st.Enabled).OnClick(func() {
			if ok {
				a.desktop.mcp.setReadOnly(!st.ReadOnly)
			}
		})
	})
}

func (a *app) mcpSettingsDetail(st mcpStatus) (string, bool) {
	if !st.Enabled {
		if n := len(st.Sessions); n > 0 {
			return fmt.Sprintf(a.tr("已关闭 · 仍有 %d 个连接", "Off · %d active"), n), false
		}
		return "", false
	}
	if !st.BinaryFound {
		return a.tr("未找到 flow-mcp", "flow-mcp not found"), true
	}
	if st.Testing {
		return a.tr("测试中…", "Testing…"), false
	}
	if st.TestErr != "" {
		return a.mcpErrorText(st.TestErr), true
	}
	if st.Err != "" {
		return st.Err, true
	}
	if n := len(st.Sessions); n > 0 {
		if n == 1 && st.Sessions[0].Client != "" {
			return fmt.Sprintf(a.tr("已连接 · %s", "Connected · %s"), st.Sessions[0].Client), false
		}
		return fmt.Sprintf(a.tr("已连接 %d 个客户端", "%d connected"), n), false
	}
	if st.TestOK && st.TestTools > 0 {
		return fmt.Sprintf(a.tr("正常 · %d 个工具", "Healthy · %d tools"), st.TestTools), false
	}
	if len(st.ConfiguredApps) > 0 {
		return a.tr("就绪 · ", "Ready · ") + strings.Join(st.ConfiguredApps, " / "), false
	}
	return a.tr("就绪", "Ready"), false
}

func (a *app) mcpErrorText(err string) string {
	switch err {
	case "mcp_disabled":
		return a.tr("MCP 已关闭", "MCP is disabled")
	case "mcp_binary_missing":
		return a.tr("未找到 flow-mcp", "flow-mcp not found")
	}
	return a.tr("测试失败：", "Test failed: ") + err
}

func (a *app) mcpDotColor(st mcpStatus) ui.Color {
	switch {
	case !st.Enabled:
		return a.tk.TextQuaternary
	case !st.BinaryFound || st.TestErr != "" || st.Err != "":
		return a.tk.Error
	case len(st.Sessions) > 0 || st.TestOK:
		return a.tk.Success
	default:
		return a.tk.Primary
	}
}

func (a *app) mcpModalStatusLine(st mcpStatus) (string, bool) {
	tools := mcpToolCountFull
	mode := a.tr("读写", "Read & write")
	if st.ReadOnly {
		tools = mcpToolCountRead
		mode = a.tr("只读", "Read-only")
	}
	switch {
	case !st.Enabled:
		return a.tr("已关闭", "Disabled"), false
	case !st.BinaryFound:
		return a.tr("未找到 flow-mcp", "flow-mcp not found"), true
	case st.Testing:
		return a.tr("正在测试连接…", "Testing connection…"), false
	case st.TestErr != "":
		return a.mcpErrorText(st.TestErr), true
	case st.TestOK:
		return fmt.Sprintf(a.tr("正常 · %d 个工具", "Healthy · %d tools"), st.TestTools), false
	case len(st.Sessions) > 0:
		return fmt.Sprintf(a.tr("已连接 %d · %s", "%d connected · %s"), len(st.Sessions), mode), false
	default:
		return fmt.Sprintf(a.tr("就绪 · %s（%d 个工具）", "Ready · %s (%d tools)"), mode, tools), false
	}
}

// mcpDialog is a compact, tabbed MCP status and client setup dialog.
func (a *app) mcpDialog(c *ui.Context) {
	ui.DialogBase(c, &a.mcpOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(520).Padding(20).Gap(14)

		st, ok := a.currentMCPStatus()
		a.dialogTitle(c, "MCP", func() { a.mcpOpen = false })

		// Compact status strip
		statusText, warn := a.mcpModalStatusLine(st)
		ui.Row(c.Key("mcp-modal-status")).Padding(10, 12).Gap(8).Radius(a.tk.BorderRadius).Background(a.tk.BgContainer).AlignItems(ui.Center).Children(func() {
			if st.Testing {
				feedback.Spin(c, feedback.SpinProps{Size: theme.Small}, nil)
			} else {
				ui.Box(c).Size(7, 7).Radius(4).Background(a.mcpDotColor(st)).Shrink(0)
			}
			color := a.tk.Text
			if warn {
				color = a.tk.Error
			}
			ui.Text(c, statusText).FontSize(13).TextColor(color).Grow(1).MinWidth(0).SingleLine()
			if ok {
				quietButton(c, a.tr("测试", "Test")).Label(a.tr("测试 MCP 连接", "Test MCP")).Disabled(st.Testing || !st.Enabled || !st.BinaryFound).OnClick(func() {
					a.desktop.mcp.runTest()
				})
			}
		})

		// Active sessions (shown only when clients are connected)
		if len(st.Sessions) > 0 {
			a.card(ui.Column(c.Key("mcp-sessions"))).Dividers(1, a.hairline()).Border(1, a.hairline()).Children(func() {
				zh := a.data.Preferences.Locale != "en"
				for _, sess := range st.Sessions {
					sess := sess
					ui.Row(c.Key(fmt.Sprintf("mcp-sess-%d", sess.PID))).Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(6, 6).Radius(3).Background(a.tk.Success).Shrink(0)
						name := sess.Client
						if name == "" {
							name = "MCP"
						}
						meta := fmt.Sprintf("PID %d", sess.PID)
						if sess.ReadOnly {
							meta = a.tr("只读", "Read-only") + " · " + meta
						}
						if dur := formatSessionDuration(sess.StartedAt, c.Now(), zh); dur != "" {
							meta += " · " + dur
						}
						ui.Text(c, name).FontSize(13).FontWeight(500).SingleLine()
						ui.Text(c, meta).FontSize(12).TextColor(a.tk.TextTertiary).Grow(1).MinWidth(0).SingleLine()
						if ok {
							quietButton(c, a.tr("断开", "Disconnect")).Label(fmt.Sprintf(a.tr("断开会话 %d", "Disconnect session %d"), sess.PID)).TextColor(a.tk.ErrorText).OnClick(func() {
								a.desktop.mcp.disconnectSession(sess.PID)
							})
						}
					})
				}
			})
		}

		// Tabbed client config (JSON + mainstream agent CLIs)
		tab := a.mcpConfigTab
		if tab < 0 || tab >= len(mcpClientPresets) {
			tab = 0
		}
		preset := mcpClientPresets[tab]
		snippet := preset.build(st.BinaryPath, a.dataDir, st.ReadOnly)
		copyZh, copyEn := "已复制命令", "Copied command"
		if preset.json {
			copyZh, copyEn = "已复制配置", "Copied config"
		}
		labels := make([]string, len(mcpClientPresets))
		for i, p := range mcpClientPresets {
			labels[i] = p.label
		}

		ui.Column(c).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				a.segmented(c, "mcp-tab", labels, tab, true, func(n int) {
					a.mcpConfigTab = n
				})
				ui.Spacer(c)
				if st.BinaryFound && st.BinaryPath != "" {
					iconButton(c, "folder", a.tr("在访达中显示", "Show in Finder")).OnClick(func() {
						mygo.Shell.ShowItemInFolder(st.BinaryPath)
					})
				}
				quietButton(c, a.tr("复制", "Copy")).Label(a.tr("复制 MCP 配置", "Copy MCP config")).OnClick(func() {
					a.copyMCPText(snippet, copyZh, copyEn)
				})
			})
			ui.Box(c.Key("mcp-snippet")).Padding(12, 14).Radius(a.tk.BorderRadius).Background(a.tk.BgContainer).Border(1, a.hairline()).Children(func() {
				ui.Text(c, snippet).Font("monospace").FontSize(12).Selectable()
			})
		})
	})
}
