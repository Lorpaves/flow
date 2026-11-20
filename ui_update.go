package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func (a *app) currentAppVersion() string {
	if a.desktop != nil && a.desktop.updater != nil {
		if v := a.desktop.updater.snapshot().Current; v != "" {
			return v
		}
	}
	return flowVersion()
}

func (a *app) currentUpdateStatus() (updateStatus, bool) {
	if a.desktop != nil && a.desktop.updater != nil {
		return a.desktop.updater.snapshot(), true
	}
	return updateStatus{Current: flowVersion(), AutoCheck: true}, false
}

func (a *app) updateHasBadge() bool {
	st, ok := a.currentUpdateStatus()
	return ok && (st.Phase == updateAvailable || st.Phase == updateReady)
}

func (a *app) openUpdateDialog(check bool) {
	a.closePicker()
	a.paletteOpen = false
	a.appUpdateOpen = true
	if check && a.desktop != nil && a.desktop.updater != nil {
		st := a.desktop.updater.snapshot()
		if st.Phase != updateAvailable && st.Phase != updateDownloading && st.Phase != updateReady {
			a.desktop.updater.checkNow(true)
		}
	}
}

func (a *app) triggerUpdateCheck() {
	a.closePicker()
	a.paletteOpen = false
	a.appUpdateOpen = true
	if a.desktop != nil && a.desktop.updater != nil {
		a.desktop.updater.checkNow(true)
	}
}

// updateBanner shows a non-intrusive in-app notice strip at the top of the
// workspace tile whenever an update is available, downloading in the
// background, or ready to install on restart.
func (a *app) updateBanner(c *ui.Context) {
	st, ok := a.currentUpdateStatus()
	if !ok {
		return
	}
	svc := a.desktop.updater
	switch st.Phase {
	case updateAvailable:
		if st.BannerDismiss {
			return
		}
		ui.Row(c.Key("update-banner")).Padding(8, 12, 8, 16).Gap(10).Background(a.tk.PrimaryBg).InsetShadow(0, -1, 0, 0, a.hairline()).Children(func() {
			ui.Icon(c, lucide.Sparkles()).FontSize(14).TextColor(a.tk.Primary)
			ui.Text(c, fmt.Sprintf(a.tr("发现新版本 Flow v%s", "Flow v%s is available"), st.Version)).FontSize(13).FontWeight(500).Grow(1).MinWidth(0).SingleLine()
			quietButton(c, a.tr("查看更新", "View details")).OnClick(func() { a.openUpdateDialog(false) })
			primary(c, a.tr("立即更新", "Update now")).Height(26).PaddingX(12).FontSize(12).OnClick(func() {
				a.appUpdateOpen = true
				svc.startDownload()
			})
			iconButton(c, "close", a.tr("关闭更新提示", "Dismiss update banner")).OnClick(func() { svc.dismissBanner() })
		})
	case updateDownloading:
		if a.appUpdateOpen {
			return
		}
		pct := updateProgressPercent(st)
		label := fmt.Sprintf(a.tr("正在下载 Flow v%s…", "Downloading Flow v%s…"), st.Version)
		if st.Total > 0 {
			label = fmt.Sprintf(a.tr("正在下载 Flow v%s · %.0f%%", "Downloading Flow v%s · %.0f%%"), st.Version, pct)
		}
		ui.Row(c.Key("update-banner")).Padding(8, 12, 8, 16).Gap(10).Background(a.tk.PrimaryBg).InsetShadow(0, -1, 0, 0, a.hairline()).Children(func() {
			feedback.Spin(c, feedback.SpinProps{Size: theme.Small}, nil)
			ui.Text(c, label).FontSize(13).FontWeight(500).Grow(1).MinWidth(0).SingleLine()
			quietButton(c, a.tr("查看进度", "View progress")).OnClick(func() { a.openUpdateDialog(false) })
		})
	case updateReady:
		if st.BannerDismiss {
			return
		}
		ui.Row(c.Key("update-banner")).Padding(8, 12, 8, 16).Gap(10).Background(a.tk.SuccessBg).InsetShadow(0, -1, 0, 0, a.hairline()).Children(func() {
			ui.Icon(c, lucide.CircleCheck()).FontSize(14).TextColor(a.tk.Success)
			ui.Text(c, fmt.Sprintf(a.tr("Flow v%s 已准备就绪，重启后生效", "Flow v%s is ready to install — restart to update"), st.Version)).FontSize(13).FontWeight(500).Grow(1).MinWidth(0).SingleLine()
			primary(c, a.tr("立即重启", "Restart now")).Height(26).PaddingX(12).FontSize(12).OnClick(func() { svc.relaunch() })
			iconButton(c, "close", a.tr("稍后重启", "Dismiss restart banner")).OnClick(func() { svc.dismissBanner() })
		})
	}
}

// updateSettings builds the Software Updates card in the Settings page.
func (a *app) updateSettings(c *ui.Context) {
	st, ok := a.currentUpdateStatus()
	detail, warn := a.updateStatusText(st, ok)
	a.settingDetail(c, a.tr("软件更新", "Software updates"), detail, warn, func() {
		if !ok {
			btn(c, a.tr("检查更新", "Check for updates")).Disabled(true)
			return
		}
		svc := a.desktop.updater
		switch st.Phase {
		case updateChecking:
			ui.Row(c).Gap(8).Children(func() {
				feedback.Spin(c, feedback.SpinProps{Size: theme.Small}, nil)
				btn(c, a.tr("检查中…", "Checking…")).Disabled(true)
			})
		case updateAvailable:
			ui.Row(c).Gap(8).Children(func() {
				btn(c, a.tr("查看更新", "View update")).OnClick(func() { a.openUpdateDialog(false) })
				primary(c, a.tr("立即更新", "Update now")).OnClick(func() {
					a.appUpdateOpen = true
					svc.startDownload()
				})
			})
		case updateDownloading:
			ui.Row(c).Gap(8).Children(func() {
				btn(c, a.tr("查看进度", "View progress")).OnClick(func() { a.openUpdateDialog(false) })
				btn(c, a.tr("取消", "Cancel")).OnClick(func() { svc.cancelDownload() })
			})
		case updateReady:
			primary(c, a.tr("重启以更新", "Restart to update")).OnClick(func() { svc.relaunch() })
		default:
			btn(c, a.tr("检查更新", "Check for updates")).OnClick(func() { a.triggerUpdateCheck() })
		}
	})
	if ok && st.Phase == updateDownloading {
		pct := updateProgressPercent(st)
		ui.Column(c.Key("update-settings-progress")).Padding(0, 16, 12, 16).Children(func() {
			feedback.Progress(c, pct, feedback.ProgressProps{
				Status:   feedback.ProgressActive,
				ShowInfo: true,
				Label:    a.downloadProgressLabel(st),
				Format:   func(p float32) string { return fmt.Sprintf("%.0f%%", p) },
			})
		})
	}
	a.settingRow(c, a.tr("自动检查更新", "Check for updates automatically"), func() {
		on := st.AutoCheck
		choice.Switch(c.Key("update-auto-check"), &on, choice.SwitchProps{}).Label(a.tr("切换自动检查更新", "Toggle automatic update checks")).Disabled(!ok).OnClick(func() {
			if ok {
				a.desktop.updater.setAutoCheck(!st.AutoCheck)
			}
		})
	})
	a.settingDetail(c, a.tr("自动下载更新", "Download updates automatically"), a.tr("发现新版本后在后台下载并准备安装，重启后生效", "Download new versions in the background so they install on restart"), false, func() {
		on := st.AutoDownload
		choice.Switch(c.Key("update-auto-download"), &on, choice.SwitchProps{}).Label(a.tr("切换自动下载更新", "Toggle automatic update downloads")).Disabled(!ok || !st.AutoCheck).OnClick(func() {
			if ok {
				a.desktop.updater.setAutoDownload(!st.AutoDownload)
			}
		})
	})
}

func (a *app) updateStatusText(st updateStatus, running bool) (string, bool) {
	ver := st.Current
	if ver == "" {
		ver = flowVersion()
	}
	if !running {
		return fmt.Sprintf(a.tr("当前版本 v%s", "Version v%s"), ver), false
	}
	switch st.Phase {
	case updateChecking:
		return a.tr("正在检查更新…", "Checking for updates…"), false
	case updateAvailable:
		return fmt.Sprintf(a.tr("当前版本 v%s · 有新版本 v%s 可用", "Version v%s · v%s available"), ver, st.Version), false
	case updateDownloading:
		return fmt.Sprintf(a.tr("正在下载 v%s…", "Downloading v%s…"), st.Version), false
	case updateReady:
		return fmt.Sprintf(a.tr("v%s 已准备就绪，重启 Flow 后生效", "v%s is ready — restart Flow to finish updating"), st.Version), false
	case updateError:
		if st.Err == "updates_disabled" {
			return fmt.Sprintf(a.tr("当前版本 v%s · 此构建不支持自动更新", "Version v%s · Auto-update unavailable in this build"), ver), false
		}
		return a.updateErrorText(st.Err), true
	case updateUpToDate:
		if !st.LastCheck.IsZero() {
			return fmt.Sprintf(a.tr("当前已是最新版本 v%s · 上次检查 %s", "Up to date (v%s) · Last checked %s"), ver, st.LastCheck.Format("15:04")), false
		}
		return fmt.Sprintf(a.tr("当前已是最新版本 v%s", "Up to date (v%s)"), ver), false
	default:
		if !st.Enabled {
			return fmt.Sprintf(a.tr("当前版本 v%s · 开发或只读构建", "Version v%s · Development or read-only build"), ver), false
		}
		if !st.LastCheck.IsZero() {
			return fmt.Sprintf(a.tr("当前版本 v%s · 上次检查 %s", "Version v%s · Last checked %s"), ver, st.LastCheck.Format("15:04")), false
		}
		return fmt.Sprintf(a.tr("当前版本 v%s", "Version v%s"), ver), false
	}
}

// updateDialog renders Flow's Lobe-styled in-app software update modal.
func (a *app) updateDialog(c *ui.Context) {
	ui.DialogBase(c, &a.appUpdateOpen, func(backdrop, panel ui.Element) {
		a.dialogChrome(backdrop, panel)
		panel.Width(480).Padding(22).Gap(14)

		st, ok := a.currentUpdateStatus()
		var svc *updateService
		if ok {
			svc = a.desktop.updater
		}

		// Header: brand icon + title + close button.
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			a.brand(c, 34)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				title := a.tr("软件更新", "Software Update")
				switch st.Phase {
				case updateAvailable:
					title = a.tr("发现新版本", "Update Available")
				case updateDownloading:
					title = a.tr("正在更新 Flow", "Updating Flow")
				case updateReady:
					title = a.tr("更新已准备就绪", "Ready to Restart")
				case updateUpToDate:
					title = a.tr("已是最新版本", "You’re Up to Date")
				}
				ui.Text(c, title).FontSize(15).FontWeight(600)
				sub := fmt.Sprintf(a.tr("当前版本 v%s", "Current version v%s"), a.currentAppVersion())
				if st.Version != "" && (st.Phase == updateAvailable || st.Phase == updateDownloading || st.Phase == updateReady) {
					sub = fmt.Sprintf("v%s → v%s", a.currentAppVersion(), st.Version)
					if !st.Date.IsZero() {
						sub += " · " + st.Date.Local().Format("2006-01-02")
					}
				}
				ui.Text(c, sub).FontSize(12).TextColor(a.tk.TextTertiary)
			})
			iconButton(c, "close", a.tr("关闭更新窗口", "Close update dialog")).OnClick(func() { a.appUpdateOpen = false })
		})

		switch {
		case !ok || st.Phase == updateError && st.Err == "updates_disabled" || (!st.Enabled && st.Phase == updateIdle):
			feedback.Alert(c, a.tr("此构建不支持自动更新", "Automatic updates are unavailable"), feedback.AlertProps{
				Type:        feedback.Info,
				Description: a.tr("当前为开发构建，或应用所在目录不可写。安装正式签名的发布版本到「应用程序」文件夹后即可自动更新。", "This is a development build or its install folder isn’t writable. Install a signed release in Applications to enable automatic updates."),
			})
			dialogFooter(c, func() {
				primary(c, a.tr("知道了", "OK")).OnClick(func() { a.appUpdateOpen = false })
			})

		case st.Phase == updateChecking:
			ui.Row(c).Padding(16, 4).Gap(12).AlignItems(ui.Center).Children(func() {
				feedback.Spin(c, feedback.SpinProps{}, nil)
				ui.Text(c, a.tr("正在检查新版本…", "Checking for new versions…")).FontSize(14).TextColor(a.tk.TextSecondary)
			})
			dialogFooter(c, func() {
				btn(c, a.tr("取消", "Cancel")).OnClick(func() {
					svc.cancelDownload()
					a.appUpdateOpen = false
				})
			})

		case st.Phase == updateUpToDate:
			ui.Row(c).Padding(12, 14).Gap(10).Radius(a.tk.BorderRadius).Background(a.tk.SuccessBg).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, lucide.CircleCheck()).FontSize(18).TextColor(a.tk.Success)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, fmt.Sprintf(a.tr("Flow v%s 已是最新版本", "Flow v%s is up to date"), a.currentAppVersion())).FontWeight(500)
					if !st.LastCheck.IsZero() {
						ui.Text(c, fmt.Sprintf(a.tr("检查时间：%s", "Last checked at %s"), st.LastCheck.Format("15:04"))).FontSize(12).TextColor(a.tk.TextSecondary)
					}
				})
			})
			dialogFooter(c, func() {
				btn(c, a.tr("重新检查", "Check again")).OnClick(func() { svc.checkNow(true) })
				primary(c, a.tr("完成", "Done")).OnClick(func() { a.appUpdateOpen = false })
			})

		case st.Phase == updateAvailable:
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				display.Tag(c, "v"+st.Version, display.TagProps{Color: "purple"})
				ui.Text(c, a.tr("已通过签名校验配置，支持增量更新", "Signed release with delta update support")).FontSize(12).TextColor(a.tk.TextTertiary)
			})
			if strings.TrimSpace(st.Notes) != "" {
				ui.Column(c).Gap(6).Children(func() {
					ui.Text(c, a.tr("更新内容", "Release notes")).FontSize(12).FontWeight(500).TextColor(a.tk.TextSecondary)
					ui.Scroll(c.Key("update-notes")).MaxHeight(240).Padding(12, 14).Radius(a.tk.BorderRadius).Background(a.tk.BgContainer).Border(1, a.hairline()).Children(func() {
						markdown.View(c, st.Notes, &markdown.Options{
							OnLink: func(u string) { mygo.Shell.OpenExternal(u) },
						}).FontSize(13)
					})
				})
			} else {
				ui.Text(c, a.tr("包含最新功能改进与问题修复。", "Includes the latest improvements and bug fixes.")).FontSize(13).TextColor(a.tk.TextSecondary)
			}
			ui.Row(c).Gap(8).Margin(4, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				quietButton(c, a.tr("跳过此版本", "Skip this version")).OnClick(func() {
					svc.skipCurrent()
					a.appUpdateOpen = false
				})
				ui.Spacer(c)
				btn(c, a.tr("稍后提醒", "Remind me later")).OnClick(func() { a.appUpdateOpen = false })
				primary(c, a.tr("下载并安装", "Download & install")).OnClick(func() { svc.startDownload() })
			})

		case st.Phase == updateDownloading:
			pct := updateProgressPercent(st)
			ui.Column(c).Gap(10).Padding(4, 0).Children(func() {
				feedback.Progress(c, pct, feedback.ProgressProps{
					Status:   feedback.ProgressActive,
					Size:     theme.Large,
					ShowInfo: true,
					Label:    a.downloadProgressLabel(st),
					Format:   func(p float32) string { return fmt.Sprintf("%.0f%%", p) },
				})
				ui.Text(c, a.tr("下载与签名校验在后台进行，你可以关闭此窗口继续工作。", "Download and signature verification run in the background. You can close this dialog and keep working.")).FontSize(12).TextColor(a.tk.TextTertiary)
			})
			dialogFooter(c, func() {
				btn(c, a.tr("取消下载", "Cancel download")).OnClick(func() { svc.cancelDownload() })
				btn(c, a.tr("后台继续", "Hide")).OnClick(func() { a.appUpdateOpen = false })
			})

		case st.Phase == updateReady:
			ui.Row(c).Padding(12, 14).Gap(10).Radius(a.tk.BorderRadius).Background(a.tk.SuccessBg).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, lucide.CircleCheck()).FontSize(18).TextColor(a.tk.Success)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, fmt.Sprintf(a.tr("Flow v%s 已安装就绪", "Flow v%s is ready"), st.Version)).FontWeight(500)
					ui.Text(c, a.tr("重启 Flow 即可切换到新版本，未保存的更改会在退出前自动保存。", "Restart Flow to launch the new version. Unsaved edits are saved before quitting.")).FontSize(12).TextColor(a.tk.TextSecondary)
				})
			})
			dialogFooter(c, func() {
				btn(c, a.tr("稍后重启", "Restart later")).OnClick(func() { a.appUpdateOpen = false })
				primary(c, a.tr("立即重启并更新", "Restart & update")).OnClick(func() { svc.relaunch() })
			})

		case st.Phase == updateError:
			feedback.Alert(c, a.tr("更新失败", "Update failed"), feedback.AlertProps{
				Type:        feedback.Error,
				Description: a.updateErrorText(st.Err),
			})
			dialogFooter(c, func() {
				btn(c, a.tr("关闭", "Close")).OnClick(func() { a.appUpdateOpen = false })
				primary(c, a.tr("重试", "Try again")).OnClick(func() {
					if st.Version != "" {
						svc.startDownload()
					} else {
						svc.checkNow(true)
					}
				})
			})

		default:
			ui.Text(c, a.tr("点击下方按钮检查 Flow 是否有新版本可用。", "Check whether a newer version of Flow is available.")).FontSize(13).TextColor(a.tk.TextSecondary)
			dialogFooter(c, func() {
				btn(c, a.tr("关闭", "Close")).OnClick(func() { a.appUpdateOpen = false })
				primary(c, a.tr("立即检查", "Check now")).OnClick(func() { svc.checkNow(true) })
			})
		}
	})
}

func updateProgressPercent(st updateStatus) float32 {
	if st.Total <= 0 {
		return 0
	}
	pct := float32(st.Downloaded) * 100 / float32(st.Total)
	return min(max(pct, 0), 100)
}

func (a *app) downloadProgressLabel(st updateStatus) string {
	if st.Total > 0 {
		return fmt.Sprintf(a.tr("正在下载 v%s（%s / %s）", "Downloading v%s (%s / %s)"), st.Version, formatBytes(st.Downloaded), formatBytes(st.Total))
	}
	return fmt.Sprintf(a.tr("正在下载并校验 v%s…", "Downloading and verifying v%s…"), st.Version)
}

func (a *app) updateErrorText(err string) string {
	lower := strings.ToLower(err)
	switch {
	case err == "updates_disabled":
		return a.tr("当前构建不支持自动更新。", "Automatic updates are not enabled for this build.")
	case strings.Contains(lower, "no such host"), strings.Contains(lower, "timeout"), strings.Contains(lower, "connection refused"), strings.Contains(lower, "network is unreachable"):
		return a.tr("无法连接到更新服务器，请检查网络后重试。", "Couldn’t reach the update server. Check your internet connection and try again.")
	case strings.Contains(lower, "signature"):
		return a.tr("更新包签名校验失败，已拒绝安装以保护应用安全。", "Update signature verification failed. The download was rejected for security.")
	case strings.Contains(lower, "cannot write"), strings.Contains(lower, "permission denied"):
		return a.tr("没有权限写入应用安装目录，请将 Flow 移动到「应用程序」后重试。", "No permission to update the app bundle. Move Flow to Applications and try again.")
	case strings.Contains(lower, "404"):
		return a.tr("尚未发布当前平台的更新清单，请稍后再试。", "No update manifest is published for this platform yet.")
	}
	return err
}

func formatBytes(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.0f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
