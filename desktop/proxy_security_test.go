// proxy_security_test.go
// =============================================================================
// اختبارات أمان وسلوك وكيل البث المحلي بلا شبكة حقيقية: قائمة النطاقات،
// التحويلات، قيود الطرق وترويسة Host، وتحويل الترجمة وإعادة كتابة m3u8.
// =============================================================================
package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	cinemana "cinemana-probe/core/cinemana"
)

func newTestStreamer(t *testing.T, baseURL string) *Streamer {
	t.Helper()
	opts := cinemana.DefaultOptions()
	if baseURL != "" {
		opts.BaseURL = baseURL
	}
	client, _, err := cinemana.BuildClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s, err := NewStreamer(ctx, opts, client)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAllowedHost(t *testing.T) {
	s := newTestStreamer(t, "")
	allowed := []string{"shabakaty.cc", "cdn.shabakaty.cc", "CINEMANA.SHABAKATY.COM", "a.b.shabakaty.com", "shabakaty.cc."}
	denied := []string{"evilshabakaty.cc", "xshabakaty.com", "shabakaty.cc.evil.com", "example.com", "", "shabakaty", "127.0.0.1"}
	for _, h := range allowed {
		if !s.allowedHost(h) {
			t.Errorf("allowedHost(%q) يجب أن يقبل", h)
		}
	}
	for _, h := range denied {
		if s.allowedHost(h) {
			t.Errorf("allowedHost(%q) يجب أن يرفض", h)
		}
	}
}

func TestAllowedURLScheme(t *testing.T) {
	s := newTestStreamer(t, "")
	for raw, want := range map[string]bool{
		"https://cdn.shabakaty.cc/x.mp4": true,
		"http://cdn.shabakaty.cc/x.mp4":  false, // تخفيض تشفير
		"ftp://cdn.shabakaty.cc/x":       false,
		"https://evilshabakaty.cc/x":     false,
	} {
		u, _ := url.Parse(raw)
		if got := s.allowedURL(u); got != want {
			t.Errorf("allowedURL(%s) = %v، نريد %v", raw, got, want)
		}
	}
}

func get(t *testing.T, method, rawURL, host string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, rawURL, nil)
	if host != "" {
		req.Host = host
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestProxyRejectsForeignTargetsAndHosts(t *testing.T) {
	s := newTestStreamer(t, "")
	if resp := get(t, "GET", s.StreamURL("https://evilshabakaty.cc/x.mp4"), ""); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("نطاق غريب: %d", resp.StatusCode)
	}
	if resp := get(t, "GET", s.StreamURL("https://cdn.shabakaty.cc/x.mp4"), "attacker.example:80"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("Host غريب (DNS rebinding): %d", resp.StatusCode)
	}
	if resp := get(t, "POST", s.StreamURL("https://cdn.shabakaty.cc/x.mp4"), ""); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", resp.StatusCode)
	}
	resp := get(t, "OPTIONS", s.StreamURL("https://cdn.shabakaty.cc/x.mp4"), "")
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Headers") != "Range" {
		t.Errorf("OPTIONS: %d %v", resp.StatusCode, resp.Header)
	}
}

// upstream خادم CDN تجريبي: يُسمح به عبر BaseURL (http + نفس المضيف).
func TestProxyRangeSubtitleAndPlaylist(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/video.mp4"):
			if r.Header.Get("Range") == "bytes=2-5" {
				w.Header().Set("Content-Range", "bytes 2-5/10")
				w.WriteHeader(http.StatusPartialContent)
				w.Write([]byte("2345"))
				return
			}
			w.Write([]byte("0123456789"))
		case strings.HasSuffix(r.URL.Path, "/ok.srt"):
			w.Write([]byte("1\n00:00:01,5 --> 00:00:02,000\n{\\an8}مرحبا\n"))
		case strings.HasSuffix(r.URL.Path, "/missing.srt"):
			http.Error(w, "<html>404</html>", http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/master.m3u8"):
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write([]byte("#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\n#EXT-X-MAP:URI=\"data:text/plain;base64,AAAA\"\n#EXTINF:5,\nseg1.ts\n"))
		case strings.HasSuffix(r.URL.Path, "/redirect"):
			http.Redirect(w, r, "https://example.com/steal", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer up.Close()
	s := newTestStreamer(t, up.URL)

	// Range يُمرَّر ويعود 206.
	req, _ := http.NewRequest("GET", s.StreamURL(up.URL+"/video.mp4"), nil)
	req.Header.Set("Range", "bytes=2-5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || string(body) != "2345" || resp.Header.Get("Content-Range") != "bytes 2-5/10" {
		t.Errorf("Range: %d %q %v", resp.StatusCode, body, resp.Header)
	}

	// ترجمة سليمة → VTT؛ 404 → 502 (لا تتحول صفحة الخطأ إلى ترجمة).
	resp = get(t, "GET", s.SubtitleURL(up.URL+"/ok.srt"), "")
	b, _ := io.ReadAll(resp.Body)
	vtt := string(b)
	if resp.StatusCode != 200 || !strings.HasPrefix(vtt, "WEBVTT") || !strings.Contains(vtt, "00:00:01.500 --> 00:00:02.000") || strings.Contains(vtt, "an8") {
		t.Errorf("SRT→VTT: %d %q", resp.StatusCode, vtt)
	}
	if resp = get(t, "GET", s.SubtitleURL(up.URL+"/missing.srt"), ""); resp.StatusCode != http.StatusBadGateway {
		t.Errorf("ترجمة 404 يجب أن تعيد 502، حصلنا %d", resp.StatusCode)
	}

	// m3u8: المقاطع والمفاتيح تمر عبر الوكيل، و data: URI تبقى كما هي.
	resp = get(t, "GET", s.StreamURL(up.URL+"/master.m3u8"), "")
	b, _ = io.ReadAll(resp.Body)
	pl := string(b)
	if strings.Count(pl, s.baseURL+"/stream?u=") != 2 || !strings.Contains(pl, `URI="data:text/plain;base64,AAAA"`) {
		t.Errorf("إعادة كتابة m3u8: %q", pl)
	}

	// تحويل من نطاق مسموح إلى نطاق غريب يُرفض (لا SSRF عبر open redirect).
	if resp = get(t, "GET", s.StreamURL(up.URL+"/redirect"), ""); resp.StatusCode != http.StatusBadGateway {
		t.Errorf("التحويل لنطاق غريب يجب أن يفشل بـ 502: %d", resp.StatusCode)
	}
}

func TestMediaURLAllowed(t *testing.T) {
	s := newTestStreamer(t, "")
	for raw, want := range map[string]bool{
		"https://cdn.shabakaty.cc/a.mp4":              true,
		s.StreamURL("https://cdn.shabakaty.cc/a.mp4"): true,
		"file:///C:/secret.txt":                       false,
		"ytdl://youtube.com/x":                        false,
		"/etc/passwd":                                 false,
		"https://evilshabakaty.cc":                    false,
		"":                                            false,
	} {
		if got := mediaURLAllowed(s, raw); got != want {
			t.Errorf("mediaURLAllowed(%q) = %v، نريد %v", raw, got, want)
		}
	}
	if mediaURLAllowed(nil, "https://cdn.shabakaty.cc/a.mp4") {
		t.Error("streamer فارغ يجب أن يرفض")
	}
}

func TestValidMPVPath(t *testing.T) {
	if err := validMPVPath(""); err != nil {
		t.Errorf("فارغ مقبول: %v", err)
	}
	for _, bad := range []string{"/bin/sh", `C:\Windows\System32\cmd.exe`, "calc.exe", "/usr/bin/mpv-evil"} {
		if err := validMPVPath(bad); err == nil {
			t.Errorf("validMPVPath(%q) يجب أن يرفض", bad)
		}
	}
}

func TestReadLimited(t *testing.T) {
	if _, err := readLimited(strings.NewReader("12345"), 4); err == nil {
		t.Error("جسم أكبر من الحد يجب أن يفشل بدل القص الصامت")
	}
	if b, err := readLimited(strings.NewReader("1234"), 4); err != nil || string(b) != "1234" {
		t.Errorf("عند الحد تماماً: %q %v", b, err)
	}
}

func TestWriteFileAtomicAndSettingsCorruption(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/out.json"
	if err := writeFileAtomic(p, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(p, []byte(`{"a":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(mustOpen(t, p))
	if string(b) != `{"a":2}` {
		t.Errorf("المحتوى: %q", b)
	}
}

func mustOpen(t *testing.T, p string) io.Reader {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}
