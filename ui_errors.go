package main

import (
	"github.com/egoist/mygo/ui"
	"strings"
)

func (a *app) recordErrorText(err string) (string, bool) {
	values := map[string][2]string{
		"invalid_timeline_shift":       {"请选择有效的时间线项目和日期范围。", "Choose valid timeline items and a date range."},
		"invalid_creation_relation":    {"请选择有效的创建关系。", "Choose a valid creation relationship."},
		"related_issue_unavailable":    {"关联任务已删除或不可用；草稿已保留，可解除关系继续创建。", "The related issue is unavailable. Your draft is retained; remove the relation to continue."},
		"related_project_conflict":     {"父子任务应属于相同项目。", "Parent and child issues must share a project."},
		"invalid_project_label":        {"请选择项目标签。任务标签不能用于项目。", "Choose project labels. Issue labels cannot be assigned to projects."},
		"immutable_label_kind":         {"标签类型不能更改，请创建新标签。", "A label cannot change its type. Create a new label."},
		"project_not_found":            {"项目已删除或不可用。", "The project is deleted or unavailable."},
		"invalid_label":                {"请填写有效的标签名称和分组。", "Enter a valid label name and group."},
		"invalid_label_scope":          {"请选择可用的项目作用域。", "Choose an available project scope."},
		"duplicate_label":              {"此作用域和分组中已有同名标签。", "This scope and group already contain that label name."},
		"invalid_color":                {"颜色格式应为 #RRGGBB。", "Use a color in #RRGGBB format."},
		"invalid_folder":               {"请填写文件夹名称。", "Enter a folder name."},
		"label_not_found":              {"该标签已被移除。", "This label has been removed."},
		"invalid_favorite":             {"收藏项已不可用，请重新选择。", "A favorite item is unavailable. Choose it again."},
		"project_in_trash":             {"请先恢复项目再编辑。", "Restore the project before editing."},
		"history_conflict":             {"相关属性已再次修改，无法撤销这条历史。", "These properties changed again, so this history entry cannot be undone."},
		"history_not_undoable":         {"这条记录无法撤销。", "This history entry cannot be undone."},
		"relation_cycle":               {"此关系会形成循环，请选择另一条任务。", "This relationship would create a cycle. Choose another issue."},
		"parent_cycle":                 {"父子关系不能形成循环。", "Parent relationships cannot form a cycle."},
		"dependency_cycle":             {"项目依赖不能形成循环。", "Project dependencies cannot form a cycle."},
		"duplicate_target_exists":      {"此任务已标记为另一条任务的重复项。请先移除原关系。", "This issue already duplicates another issue. Remove that relationship first."},
		"invalid_relation":             {"请检查两条任务仍可编辑且不是同一条。", "Both issues must be editable and different."},
		"invalid_project_reference":    {"原项目不可用，请选择一个有效项目。", "The original project is unavailable. Choose a valid project."},
		"invalid_template":             {"请填写模板名称并检查默认属性。", "Enter a template name and check its defaults."},
		"invalid_draft":                {"草稿属性无效，请检查标题长度和日期。", "Check the draft's title length and dates."},
		"invalid_comment":              {"请填写评论，并确认任务尚未删除。", "Enter a comment and check that the issue has not been deleted."},
		"invalid_comment_parent":       {"原评论已删除或不属于此任务。回复草稿已保留。", "The original comment was deleted or belongs to another issue. Your reply draft is retained."},
		"comment_cycle":                {"评论回复不能形成循环。", "Comment replies cannot form a cycle."},
		"invalid_attachment":           {"请填写附件名称，并确认所属任务或项目尚未删除。", "Enter an attachment name and check that its issue or project has not been deleted."},
		"invalid_attachment_url":       {"链接需为有效的 http 或 https 地址。", "Enter a valid http or https URL."},
		"attachment_not_found":         {"原附件已不存在，未插入这次内容。请重新复制文件。", "The source attachment is missing. Nothing was pasted; copy the file again."},
		"invalid_clipboard_image":      {"图片格式不受支持或尺寸过大，未插入这次内容。", "The image format or dimensions are unsupported. Nothing was pasted."},
		"image_import_failed":          {"图片读取失败，未插入这次内容。请重试或先保存图片文件。", "The image could not be loaded. Nothing was pasted; retry or save the image file first."},
		"attachment_too_large":         {"单个附件不能超过 32 MB。", "Each attachment must be 32 MB or smaller."},
		"invalid_attachment_file":      {"请选择普通文件。", "Choose a regular file."},
		"attachment_checksum_mismatch": {"附件内容校验失败，未保存更改。", "Attachment verification failed. Changes were not saved."},
		"invalid_recurrence":           {"请检查标题、周期和下次日期。", "Check the title, repeat interval, and next date."},
		"invalid_reminder":             {"请检查提醒时间以及任务或项目是否仍存在。", "Check the reminder time and whether its issue or project still exists."},
		"invalid_milestone":            {"请填写里程碑名称并检查日期。", "Enter a milestone name and check its date."},
		"invalid_document":             {"请填写文档名称并检查所属项目或任务。", "Enter a document name and check its project or issue."},
		"invalid_project_update":       {"请填写更新内容并选择健康状态。", "Enter an update and select its health status."},
		"not_found":                    {"该内容已不存在，请刷新后重试。", "This item no longer exists. Refresh and try again."},
	}
	best := ""
	for key := range values {
		if strings.HasPrefix(err, key) && len(key) > len(best) {
			best = key
		}
	}
	if best == "" {
		return "", false
	}
	v := values[best]
	return a.tr(v[0], v[1]), true
}
func (a *app) dialogError(c *ui.Context) {
	if a.err != "" {
		ui.Text(c, a.errorText(a.err)).FontSize(12).TextColor(a.theme.Danger)
	}
}
