package project

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// UploadsDir is where everything a person attaches lives, inside the
// project library: one folder per upload, so a file can be listed, opened
// and cleaned up on its own, and the Library tab shows it.
const UploadsDir = "uploads"

// MaxAttachments caps the files on one message.
const MaxAttachments = 20

// Attachment is a file a person attached to a message.
type Attachment struct {
	Name string `json:"name"`
	// Path is relative to the library root, forward slashes, always under uploads/.
	Path string `json:"path"`
	// Type is the MIME type.
	Type string `json:"type"`
	Size int64  `json:"size"`
}

// SaveAttachment stores src as a new upload of the project. limit caps its
// size (0 for none).
func (l Layout) SaveAttachment(projectID, name string, src io.Reader, limit int64) (Attachment, error) {
	name = cleanUploadName(name)
	var id [6]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Attachment{}, err
	}
	rel := UploadsDir + "/" + hex.EncodeToString(id[:]) + "/" + name
	dst, err := l.LibraryPath(projectID, rel)
	if err != nil {
		return Attachment{}, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return Attachment{}, err
	}
	out, err := os.Create(dst)
	if err != nil {
		return Attachment{}, err
	}
	if limit > 0 {
		src = io.LimitReader(src, limit+1)
	}
	n, err := io.Copy(out, src)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && limit > 0 && n > limit {
		err = fmt.Errorf("%s is larger than %d MB", name, limit>>20)
	}
	if err != nil {
		_ = os.RemoveAll(filepath.Dir(dst))
		return Attachment{}, err
	}
	return Attachment{Name: name, Path: rel, Type: sniffType(dst, name), Size: n}, nil
}

// ResolveAttachments checks that each path is an existing upload of the
// project and returns what is on disk, so a request cannot name a file
// outside uploads/ or lie about its size.
func (l Layout) ResolveAttachments(projectID string, paths []string) ([]Attachment, error) {
	if len(paths) > MaxAttachments {
		return nil, fmt.Errorf("at most %d attachments per message", MaxAttachments)
	}
	out := make([]Attachment, 0, len(paths))
	for _, rel := range paths {
		rel = filepath.ToSlash(filepath.Clean(rel))
		if !strings.HasPrefix(rel, UploadsDir+"/") {
			return nil, fmt.Errorf("invalid attachment %q", rel)
		}
		abs, err := l.LibraryPath(projectID, rel)
		if err != nil {
			return nil, err
		}
		st, err := os.Stat(abs)
		if err != nil || st.IsDir() {
			return nil, fmt.Errorf("attachment %q not found", rel)
		}
		name := filepath.Base(rel)
		out = append(out, Attachment{Name: name, Path: rel, Type: sniffType(abs, name), Size: st.Size()})
	}
	return out, nil
}

// AttachmentsPayload is the message payload that records attachments.
func AttachmentsPayload(atts []Attachment) string {
	if len(atts) == 0 {
		return ""
	}
	return jsonString(map[string]any{"attachments": atts})
}

// ParseAttachments reads attachments back from a message payload.
func ParseAttachments(payload string) []Attachment {
	if payload == "" {
		return nil
	}
	var pl struct {
		Attachments []Attachment `json:"attachments"`
	}
	_ = json.Unmarshal([]byte(payload), &pl)
	return pl.Attachments
}

// AttachmentNote tells a model where the attached files are on disk. A file
// that was removed since is still listed, as removed.
func AttachmentNote(libraryDir string, atts []Attachment) string {
	if len(atts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[The user attached these files; read them from disk]\n")
	for _, a := range atts {
		abs := filepath.Join(libraryDir, filepath.FromSlash(a.Path))
		if _, err := os.Stat(abs); err != nil {
			fmt.Fprintf(&b, "- %s (removed)\n", a.Name)
			continue
		}
		fmt.Fprintf(&b, "- %s (%s, %s)\n", abs, a.Type, humanSize(a.Size))
	}
	return strings.TrimRight(b.String(), "\n")
}

// withAttachments appends the attachment note to a message's text.
func withAttachments(text, libraryDir string, atts []Attachment) string {
	note := AttachmentNote(libraryDir, atts)
	if note == "" {
		return text
	}
	if strings.TrimSpace(text) == "" {
		return note
	}
	return text + "\n\n" + note
}

func cleanUploadName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == '/' || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	return name
}

func sniffType(abs, name string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
		return strings.SplitN(t, ";", 2)[0]
	}
	f, err := os.Open(abs)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	return strings.SplitN(http.DetectContentType(buf[:n]), ";", 2)[0]
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
