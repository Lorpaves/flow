package core

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

func AttachmentFromFile(path string) (Attachment, error) {
	if !filepath.IsAbs(path) {
		return Attachment{}, errors.New("absolute_path_required")
	}
	f, e := os.Open(path)
	if e != nil {
		return Attachment{}, e
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil {
		return Attachment{}, e
	}
	if !stat.Mode().IsRegular() {
		return Attachment{}, errors.New("invalid_attachment_file")
	}
	if stat.Size() > MaxAttachmentBytes {
		return Attachment{}, errors.New("attachment_too_large")
	}
	data, e := io.ReadAll(io.LimitReader(f, MaxAttachmentBytes+1))
	if e != nil {
		return Attachment{}, e
	}
	if len(data) > MaxAttachmentBytes {
		return Attachment{}, errors.New("attachment_too_large")
	}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return Attachment{Name: filepath.Base(path), MIME: contentType, Data: data}, nil
}
