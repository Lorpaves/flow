package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
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
	if a.desktop == nil || a.desktop.updater == nil {
		return
	}
	st := a.desktop.updater.snapshot()
	if st.Phase == updateAvailable || st.Phase == updateDownloading || st.Phase == updateReady {
		a.appUpdateOpen = true
		return
	}
	a.awaitingUpdateCheck = true
	a.updateCheckSeq = st.ResultSeq
	a.desktop.updater.checkNow(true)
}

// observeUpdateCheck opens the update dialog once a manual check finishes
// with a new version; up-to-date and error results stay non-intrusive.
func (a *app) observeUpdateCheck() {
	if !a.awaitingUpdateCheck {
		return
	}
	st, ok := a.currentUpdateStatus()
	if !ok || st.Phase == updateChecking || st.ResultSeq == a.updateCheckSeq {
		return
	}
	a.awaitingUpdateCheck = false
	a.updateCheckSeq = st.ResultSeq
	switch st.Phase {
	case updateAvailable, updateReady:
		a.appUpdateOpen = true
	case updateUpToDate:
		if a.page != "settings" {
			a.createdIssue = ""
			a.notice = fmt.Sprintf(a.tr("Flow v%s 已是最新版本", "Flow v%s is up to date"), a.currentAppVersion())
		}
	case updateError:
		if a.page != "settings" {
			a.createdIssue = ""
			a.notice = a.updateErrorText(st.Err)
		}
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
			feedback.Spin(c, feedback.SpinProps{Size: theme.Small}, nil).AlignSelf(ui.Center)
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

// updateSettings builds a compact two-row Software Updates card in Settings.
func (a *app) updateSettings(c *ui.Context) {
	st, ok := a.currentUpdateStatus()
	detail, warn := a.updateStatusText(st, ok)
	a.settingDetail(c, a.tr("软件更新", "Updates"), detail, warn, func() {
		if !ok {
			btn(c, a.tr("检查更新", "Check")).Disabled(true)
			return
		}
		svc := a.desktop.updater
		switch st.Phase {
		case updateChecking:
			btn(c, "").Label(a.tr("检查更新", "Check")).MinWidth(84).Center().Disabled(true).Children(func() {
				feedback.Spin(c, feedback.SpinProps{Size: theme.Small}, nil).AlignSelf(ui.Center)
			})
		case updateAvailable:
			ui.Row(c).Gap(8).Children(func() {
				btn(c, a.tr("查看", "Details")).OnClick(func() { a.openUpdateDialog(false) })
				primary(c, a.tr("更新", "Update")).OnClick(func() {
					a.appUpdateOpen = true
					svc.startDownload()
				})
			})
		case updateDownloading:
			ui.Row(c).Gap(8).Children(func() {
				btn(c, a.tr("进度", "Progress")).OnClick(func() { a.openUpdateDialog(false) })
				btn(c, a.tr("取消", "Cancel")).OnClick(func() { svc.cancelDownload() })
			})
		case updateReady:
			primary(c, a.tr("立即重启", "Restart")).OnClick(func() { svc.relaunch() })
		default:
			btn(c, a.tr("检查更新", "Check")).MinWidth(84).Center().OnClick(func() { a.triggerUpdateCheck() })
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
	a.settingRow(c, a.tr("自动更新", "Automatic updates"), func() {
		mode := 0
		if st.AutoCheck {
			mode = 1
			if st.AutoDownload {
				mode = 2
			}
		}
		a.segmented(c, "update-mode", []string{a.tr("关闭", "Off"), a.tr("提醒", "Notify"), a.tr("自动下载", "Auto")}, mode, false, func(n int) {
			if !ok {
				return
			}
			svc := a.desktop.updater
			switch n {
			case 0:
				svc.setAutoCheck(false)
				svc.setAutoDownload(false)
			case 1:
				svc.setAutoCheck(true)
				svc.setAutoDownload(false)
			case 2:
				svc.setAutoCheck(true)
				svc.setAutoDownload(true)
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
		return "v" + ver, false
	}
	switch st.Phase {
	case updateChecking:
		return a.tr("正在检查…", "Checking…"), false
	case updateAvailable:
		return fmt.Sprintf(a.tr("有新版本 v%s", "v%s available"), st.Version), false
	case updateDownloading:
		return fmt.Sprintf(a.tr("正在下载 v%s…", "Downloading v%s…"), st.Version), false
	case updateReady:
		return fmt.Sprintf(a.tr("v%s 已就绪，重启后生效", "v%s ready to install"), st.Version), false
	case updateError:
		if st.Err == "updates_disabled" {
			return fmt.Sprintf(a.tr("v%s · 当前构建不可自动更新", "v%s · Unavailable in this build"), ver), false
		}
		return a.updateErrorText(st.Err), true
	case updateUpToDate:
		return fmt.Sprintf(a.tr("已是最新 · v%s", "Up to date · v%s"), ver), false
	default:
		if !st.Enabled {
			return "v" + ver, false
		}
		if !st.LastCheck.IsZero() {
			return fmt.Sprintf(a.tr("v%s · 上次检查 %s", "v%s · Checked %s"), ver, st.LastCheck.Format("15:04")), false
		}
		return "v" + ver, false
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
			feedback.Alert(c, a.tr("当前构建不支持自动更新", "Automatic updates unavailable"), feedback.AlertProps{
				Type:        feedback.Info,
				Description: a.tr("请安装正式版本到「应用程序」文件夹。", "Install a signed release in Applications to enable updates."),
			})
			dialogFooter(c, func() {
				primary(c, a.tr("知道了", "OK")).OnClick(func() { a.appUpdateOpen = false })
			})

		case st.Phase == updateChecking:
			ui.Row(c).Padding(16, 4).Gap(12).AlignItems(ui.Center).Children(func() {
				feedback.Spin(c, feedback.SpinProps{}, nil).AlignSelf(ui.Center)
				ui.Text(c, a.tr("正在检查更新…", "Checking for updates…")).FontSize(14).TextColor(a.tk.TextSecondary)
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
				ui.Text(c, fmt.Sprintf(a.tr("Flow v%s 已是最新版本", "Flow v%s is up to date"), a.currentAppVersion())).FontWeight(500)
			})
			dialogFooter(c, func() {
				btn(c, a.tr("重新检查", "Check again")).OnClick(func() { svc.checkNow(true) })
				primary(c, a.tr("完成", "Done")).OnClick(func() { a.appUpdateOpen = false })
			})

		case st.Phase == updateAvailable:
			if strings.TrimSpace(st.Notes) != "" {
				ui.Scroll(c.Key("update-notes")).MaxHeight(240).Padding(12, 14).Radius(a.tk.BorderRadius).Background(a.tk.BgContainer).Border(1, a.hairline()).Children(func() {
					markdown.View(c, st.Notes, &markdown.Options{
						OnLink: func(u string) { mygo.Shell.OpenExternal(u) },
					}).FontSize(13)
				})
			}
			ui.Row(c).Gap(8).Margin(4, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				quietButton(c, a.tr("跳过", "Skip")).OnClick(func() {
					svc.skipCurrent()
					a.appUpdateOpen = false
				})
				ui.Spacer(c)
				btn(c, a.tr("稍后提醒", "Remind me later")).OnClick(func() { a.appUpdateOpen = false })
				primary(c, a.tr("下载并安装", "Download & install")).OnClick(func() { svc.startDownload() })
			})

		case st.Phase == updateDownloading:
			pct := updateProgressPercent(st)
			ui.Column(c).Gap(8).Padding(4, 0).Children(func() {
				feedback.Progress(c, pct, feedback.ProgressProps{
					Status:   feedback.ProgressActive,
					Size:     theme.Large,
					ShowInfo: true,
					Label:    a.downloadProgressLabel(st),
					Format:   func(p float32) string { return fmt.Sprintf("%.0f%%", p) },
				})
			})
			dialogFooter(c, func() {
				btn(c, a.tr("取消", "Cancel")).OnClick(func() { svc.cancelDownload() })
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
