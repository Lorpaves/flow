package core

import (
	"github.com/egoist/mygo/ui/markdown/parser"
	"strings"
	"testing"
)

func TestCopyDocumentRemapsOnlyMediaDestinations(t *testing.T) {
	s, _ := testStore(t)
	var copiedID, oldURL string
	data, e := s.Update(func(d *Data) error {
		i, e := d.AddIssue("Source", "", "", "todo")
		if e != nil {
			return e
		}
		f, e := d.AddAttachment(Attachment{OwnerKind: "issue", OwnerID: i.ID, Name: "image.png", Data: []byte("independent media")})
		if e != nil {
			return e
		}
		oldURL = AttachmentURLPrefix + f.ID
		body := "prose " + oldURL + "\n\n![image](" + oldURL + ")\n\n[**bold** link](<" + oldURL + "> \"title\")\n\n[reference][r]\n\n[r]: " + oldURL + "\n\n`[code](" + oldURL + ")`\n\n```\n![fenced](" + oldURL + ")\n```\n\n[![nested](" + oldURL + ")](" + oldURL + ")"
		if _, e = d.SaveDocument(Document{OwnerKind: "issue", OwnerID: i.ID, Name: "Media", Body: body}); e != nil {
			return e
		}
		copied, e := d.CopyIssue(i.ID, "Copy", false)
		if e != nil {
			return e
		}
		copiedID = copied.ID
		delete(d.Attachments, f.ID)
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	var body, newURL string
	for _, v := range data.Documents {
		if v.OwnerID == copiedID {
			body = v.Body
		}
	}
	for _, v := range data.Attachments {
		if v.OwnerID == copiedID {
			newURL = AttachmentURLPrefix + v.ID
			if string(v.Data) != "independent media" {
				t.Fatal("copy bytes lost")
			}
		}
	}
	if body == "" || newURL == "" || !strings.Contains(body, "prose "+oldURL) || !strings.Contains(body, "`[code]("+oldURL+")`") || !strings.Contains(body, "```\n![fenced]("+oldURL+")\n```") {
		t.Fatal("literal content changed", body)
	}
	doc := parser.Parse(body)
	links := 0
	for b := 0; b < doc.Len(); b++ {
		for _, n := range doc.Inlines(b) {
			if n.Kind == parser.Link || n.Kind == parser.Image {
				links++
				if n.Dest != newURL {
					t.Fatal("copy still points to deleted original", n.Dest, body)
				}
			}
		}
	}
	if links != 5 {
		t.Fatal("missing links/images", links, body)
	}
}
