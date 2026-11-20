package core

import (
	"cmp"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui/markdown/parser"
)

// remapMarkdownAttachments changes link/image destinations only. Code, prose,
// labels and unrelated Markdown keep their original bytes. Reference links are
// made explicit so a copied document no longer depends on the old destination.
func remapMarkdownAttachments(body string, urls map[string]string) string {
	if len(urls) == 0 || !strings.Contains(body, AttachmentURLPrefix) {
		return body
	}
	type replacement struct {
		start, end int
		text       string
	}
	var changes []replacement
	doc := parser.Parse(body)
	for block := 0; block < doc.Len(); block++ {
		in := doc.Inlines(block)
		for i, n := range in {
			url, ok := urls[n.Dest]
			if !ok || n.Kind != parser.Link && n.Kind != parser.Image {
				continue
			}
			if n.Auto {
				changes = append(changes, replacement{n.Start, n.End, "<" + url + ">"})
				continue
			}
			// Children include code spans and escaped closing brackets; looking
			// after their source ranges finds the real closing label delimiter.
			end := n.Start + 1
			if n.Kind == parser.Image {
				end++
			}
			for j := i + 1; j < n.Next; j++ {
				end = max(end, in[j].End)
			}
			if end > n.End || n.End > len(body) {
				continue
			}
			close := strings.IndexByte(body[end:n.End], ']')
			if close < 0 {
				continue
			}
			start := end + close + 1
			title := ""
			if n.Title != "" {
				title = ` "` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", " ", "\r", " ").Replace(n.Title) + `"`
			}
			changes = append(changes, replacement{start, n.End, "(" + url + title + ")"})
		}
	}
	slices.SortFunc(changes, func(a, b replacement) int { return cmp.Compare(b.start, a.start) })
	for _, change := range changes {
		body = body[:change.start] + change.text + body[change.end:]
	}
	return body
}
