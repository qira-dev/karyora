package knowledgepack

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

// Limits bound every untrusted import (C3-05). All values are Proposed
// pending the recorded owner/measurement approval the plan requires; they
// are deliberately conservative so an import can never exhaust memory or
// time before its checks run.
type Limits struct {
	MaxFileBytes     int64 // per entry
	MaxTotalBytes    int64 // sum over all entries
	MaxEntries       int   // archive entry count
	MaxChunkBytes    int   // retrieval chunk size (index.go)
	ParseTimeoutSecs int   // wall clock per fetch (used via context)
}

// DefaultLimits are the Proposed Phase 3 defaults (C3-05). Numeric limits
// become final only through recorded owner approval.
func DefaultLimits() Limits {
	return Limits{
		MaxFileBytes:     10 << 20, // 10 MiB per entry
		MaxTotalBytes:    64 << 20, // 64 MiB per pack import
		MaxEntries:       2000,     // per archive
		MaxChunkBytes:    4096,     // retrieval chunk bytes
		ParseTimeoutSecs: 30,
	}
}

// QuarantineReason names why an input was rejected or quarantined
// (C3-05/C3-06): the reason is always visible, never silent.
type QuarantineReason string

const (
	QuarantineUnsupportedFormat QuarantineReason = "unsupported_format"
	QuarantineOversize          QuarantineReason = "oversize"
	QuarantineTooManyEntries    QuarantineReason = "too_many_entries"
	QuarantineBadEntryPath      QuarantineReason = "bad_entry_path"
	QuarantineBinaryContent     QuarantineReason = "binary_content"
	QuarantineTraversal         QuarantineReason = "path_traversal"
	QuarantineSymlinkEntry      QuarantineReason = "symlink_entry"
	QuarantineProhibitedTarget  QuarantineReason = "prohibited_fetch_target"
	QuarantineRedirect          QuarantineReason = "redirect_disallowed"
	QuarantineTampered          QuarantineReason = "tampered_content"
	QuarantineUnsupportedVer    QuarantineReason = "unsupported_schema_version"
	QuarantineBrokenReferences  QuarantineReason = "broken_references"
	QuarantineRevokedTrust      QuarantineReason = "revoked_trust"
)

// allowedExtensions is the import allowlist (C3-05): documents only.
// Parsers for other formats are added per format with their own policy —
// never by trusting a supplied MIME value.
var allowedExtensions = map[string]string{
	".md":       "text/markdown",
	".markdown": "text/markdown",
	".txt":      "text/plain",
	".json":     "application/json",
}

// Content is one accepted content item ready to become an immutable Entry.
type Content struct {
	Path        string
	ContentType string
	Data        []byte
}

// ImportDocument accepts one untrusted document with the extension-derived
// allowlist and full content checks (C3-05): allowlist and size checks run
// BEFORE any parsing, and binary/NUL content is rejected rather than
// partially parsed.
func ImportDocument(name string, content []byte, lim Limits) (Content, error) {
	if err := ValidateEntryPath(name); err != nil {
		return Content{}, quarantined(QuarantineBadEntryPath, err)
	}
	ext := strings.ToLower(path.Ext(name))
	ct, ok := allowedExtensions[ext]
	if !ok {
		return Content{}, quarantined(QuarantineUnsupportedFormat, fmt.Errorf("extension %q is not on the import allowlist", ext))
	}
	if int64(len(content)) > lim.MaxFileBytes {
		return Content{}, quarantined(QuarantineOversize, fmt.Errorf("%d bytes exceed the per-file limit %d", len(content), lim.MaxFileBytes))
	}
	if err := checkTextContent(content); err != nil {
		return Content{}, err
	}
	return Content{Path: name, ContentType: ct, Data: content}, nil
}

// ImportArchive accepts an untrusted ZIP archive (C3-05): every entry is
// checked before acceptance — allowlist extension, path rules (no absolute,
// no dot-dot, no backslash), count and size bounds, and an explicit refusal
// of symlink/device entries. A hostile archive rejects or quarantines with
// a visible reason; it can never write outside the pack.
func ImportArchive(zipBytes []byte, lim Limits) ([]Content, error) {
	if int64(len(zipBytes)) > lim.MaxTotalBytes {
		return nil, quarantined(QuarantineOversize, fmt.Errorf("archive of %d bytes exceeds the total limit %d", len(zipBytes), lim.MaxTotalBytes))
	}
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, quarantined(QuarantineUnsupportedFormat, fmt.Errorf("not a readable zip archive: %v", err))
	}
	if len(reader.File) > lim.MaxEntries {
		return nil, quarantined(QuarantineTooManyEntries, fmt.Errorf("%d entries exceed the limit %d", len(reader.File), lim.MaxEntries))
	}
	var total int64
	out := make([]Content, 0, len(reader.File))
	for _, zf := range reader.File {
		if !zf.Mode().IsRegular() {
			return nil, quarantined(QuarantineSymlinkEntry, fmt.Errorf("entry %q is not a regular file", zf.Name))
		}
		if err := ValidateEntryPath(zf.Name); err != nil {
			if strings.Contains(zf.Name, "..") {
				return nil, quarantined(QuarantineTraversal, err)
			}
			return nil, quarantined(QuarantineBadEntryPath, err)
		}
		ext := strings.ToLower(path.Ext(zf.Name))
		ct, ok := allowedExtensions[ext]
		if !ok {
			return nil, quarantined(QuarantineUnsupportedFormat, fmt.Errorf("entry %q has extension %q which is not on the allowlist", zf.Name, ext))
		}
		if int64(zf.UncompressedSize64) > lim.MaxFileBytes {
			return nil, quarantined(QuarantineOversize, fmt.Errorf("entry %q of %d bytes exceeds the per-file limit", zf.Name, zf.UncompressedSize64))
		}
		total += int64(zf.UncompressedSize64)
		if total > lim.MaxTotalBytes {
			return nil, quarantined(QuarantineOversize, fmt.Errorf("expanded archive exceeds the total limit %d", lim.MaxTotalBytes))
		}
		rc, err := zf.Open()
		if err != nil {
			return nil, quarantined(QuarantineUnsupportedFormat, fmt.Errorf("entry %q cannot be opened: %v", zf.Name, err))
		}
		// Read at most MaxFileBytes+1 so an archive that lies about its
		// declared size is still caught.
		content, err := io.ReadAll(io.LimitReader(rc, lim.MaxFileBytes+1))
		_ = rc.Close()
		if err != nil {
			return nil, quarantined(QuarantineUnsupportedFormat, fmt.Errorf("entry %q cannot be read: %v", zf.Name, err))
		}
		if int64(len(content)) > lim.MaxFileBytes {
			return nil, quarantined(QuarantineOversize, fmt.Errorf("entry %q exceeds the per-file limit", zf.Name))
		}
		if err := checkTextContent(content); err != nil {
			return nil, err
		}
		out = append(out, Content{Path: zf.Name, ContentType: ct, Data: content})
	}
	return out, nil
}

// checkTextContent rejects binary payloads: NUL bytes and invalid UTF-8.
// Content-type values supplied by the sender are never trusted (C3-05).
func checkTextContent(content []byte) error {
	if bytes.IndexByte(content, 0) >= 0 {
		return quarantined(QuarantineBinaryContent, fmt.Errorf("content contains a NUL byte"))
	}
	if len(content) > 0 && !utf8.Valid(content) {
		return quarantined(QuarantineBinaryContent, fmt.Errorf("content is not valid UTF-8"))
	}
	return nil
}

// prohibitedIP reports whether ip is a destination an import fetch may
// never reach (C3-05 SSRF constraint): loopback, link-local (including the
// cloud metadata address), unspecified, multicast, and private LAN ranges
// all fail closed. Local sources belong on the filesystem, not behind a URL.
func prohibitedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsPrivate()
}

// FetchSource retrieves one remote document for import under the SSRF
// policy (C3-05): https only, redirects refused (no hop ambiguity), the
// resolved destination checked against prohibited ranges at dial time,
// response size bounded, and the response content checks run like any
// other import.
func FetchSource(ctx context.Context, rawURL string, lim Limits) (Content, error) {
	if !strings.HasPrefix(rawURL, "https://") {
		return Content{}, quarantined(QuarantineUnsupportedFormat, fmt.Errorf("import fetch allows https URLs only"))
	}
	timeout := time.Duration(lim.ParseTimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(DefaultLimits().ParseTimeoutSecs) * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout}
	client := &http.Client{
		Transport: &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			h, p, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("knowledgepack: fetch target is malformed: %v", err)
			}
			// Resolve and verify at dial time: every request re-checks the
			// destination against the prohibited ranges, and the dial goes
			// to the exact verified IP — a DNS rebinding between lookup and
			// dial cannot swap the destination underneath the check.
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, h)
			if err != nil {
				return nil, fmt.Errorf("knowledgepack: fetch target does not resolve: %v", err)
			}
			if len(ips) == 0 {
				return nil, quarantined(QuarantineProhibitedTarget, fmt.Errorf("target %q resolves to no addresses", h))
			}
			for _, ip := range ips {
				if prohibitedIP(ip.IP) {
					return nil, quarantined(QuarantineProhibitedTarget, fmt.Errorf("fetch to %s is prohibited", ip.IP))
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), p))
		}},
		// Redirects are refused for import fetches: every hop would need
		// its own destination revalidation, and import fetches carry no
		// credentials that could leak across origins.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return quarantined(QuarantineRedirect, fmt.Errorf("import fetch refuses redirects (requested %s)", req.URL))
		},
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Content{}, quarantined(QuarantineUnsupportedFormat, fmt.Errorf("URL is malformed: %v", err))
	}
	resp, err := client.Do(req)
	if err != nil {
		return Content{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Content{}, quarantined(QuarantineUnsupportedFormat, fmt.Errorf("fetch returned HTTP %d", resp.StatusCode))
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, lim.MaxFileBytes+1))
	if err != nil {
		return Content{}, fmt.Errorf("knowledgepack: fetch read failed: %w", err)
	}
	if int64(len(content)) > lim.MaxFileBytes {
		return Content{}, quarantined(QuarantineOversize, fmt.Errorf("fetched body exceeds the per-file limit"))
	}
	name := path.Base(req.URL.Path)
	if name == "" || name == "." || name == "/" {
		name = "fetched.txt"
	}
	return ImportDocument(name, content, lim)
}

// BuildEntry turns an accepted content item into an immutable pack Entry
// with its digest (C3-03/C3-05 handoff).
func BuildEntry(res Content) (Entry, error) {
	if err := ValidateEntryPath(res.Path); err != nil {
		return Entry{}, err
	}
	sum := sha256.Sum256(res.Data)
	return Entry{
		Path:        res.Path,
		ContentType: res.ContentType,
		SizeBytes:   int64(len(res.Data)),
		Digest:      "sha256:" + fmt.Sprintf("%x", sum),
	}, nil
}

// quarantined wraps a reason into the error a caller sees; the reason is
// the visible part, the wrapped error is the detail.
func quarantined(reason QuarantineReason, err error) error {
	return fmt.Errorf("knowledgepack: import quarantined (%s): %v", reason, err)
}
