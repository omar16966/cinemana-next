// proxy.go (desktop)
// =============================================================================
// وكيل البث المحلي — حل مشكلتي CORS والـ Headers عند تشغيل الفيديو.
//
// المشكلة: عناصر <video> و<hls.js> داخل الـ WebView تطلب روابط الفيديو
// مباشرة من CDN المزود. أثناء ذلك:
//  1. لا يمكن للـ WebView إرسال ترويسات المحاكاة (User-Agent الدالّ
//     على تطبيق الأندرويد) مع طلب الوسائط.
//  2. لو استخدمنا hls.js لجلب مقاطع m3u8 عبر XHR لَاصطدمنا بحجب CORS
//     (الـ CDN لا يرسل ترويسات Access-Control-Allow-Origin).
//  3. ملفات الترجمة عبر <track> تتطلب CORS حتماً.
//
// الحل: خادم HTTP محلي على 127.0.0.1 (منفذ عشوائي) يعمل وسطاً:
//
//	/stream?u=<رابط مشفّر>  يطلب الوسائط من الـ CDN بنفس ترويسات المحاكاة،
//	                        يمرّر رأس Range (التقديم/التأخير والبحث داخل
//	                        الفيديو يعملان طبيعياً)، ويعيد كتابة قوائم
//	                        m3u8 بحيث تشير مقاطعها الداخلية إلى الوكيل نفسه.
//	/sub?u=<رابط مشفّر>     يجلب ملف الترجمة ويحوله إلى VTT إن كان SRT
//	                        (مشغل HTML5 لا يفهم SRT مباشرة)، مع معالجة
//	                        الترميز العربي windows-1256 إن لم يكن UTF-8.
//
// الوكيل يستمع على الواجهة المحلية فقط (127.0.0.1) ويرفض أي نطاق هدف
// خارج نطاقات المزود — فلن يتحول إلى بروكسي مفتوح.
// =============================================================================
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	cinemana "cinemana-probe/core/cinemana"
)

// allowedDomains نطاقات المزود المسموح بها عبر الوكيل (النطاق نفسه أو أي
// نطاق فرعي له — بحد نقطة فاصل، فلا يُقبل مثل evilshabakaty.cc).
var allowedDomains = []string{"shabakaty.cc", "shabakaty.com"}

// maxPlaylistBytes / maxSubtitleBytes سقف أحجام الاستجابات المحوّلة في الذاكرة.
const (
	maxPlaylistBytes = 4 << 20
	maxSubtitleBytes = 10 << 20
)

// copyBufPool مخازن نسخ مُعاد استخدامها بدل تخصيص 64KB لكل طلب.
var copyBufPool = sync.Pool{New: func() any { b := make([]byte, 64*1024); return &b }}

// Streamer خادم البث المحلي.
type Streamer struct {
	opts    cinemana.ClientOptions
	client  *http.Client // عميل بث: بلا مهلة كلية (الأفلام طويلة!) مع مهلة رأس استجابة فقط
	srv     *http.Server
	baseURL string
	port    int
	mu      sync.RWMutex // يحمي opts وclient عند تحديث الإعدادات
}

// makeStreamClient يبني عميل بث من العميل العادي: نفس تجاوز البروكسي وDNS
// من النواة، لكن بلا مهلة كلية (client.Timeout) لأن تشغيل فيلم قد
// يستغرق ساعتين؛ نكتفي بمهلة على وصول رأس الاستجابة لاكتشاف تعطل الـ CDN
// مبكراً. كما تُفحص كل عملية إعادة توجيه ضمن قائمة النطاقات المسموحة حتى
// لا يتحول تحويل مفتوح على نطاق المزود إلى SSRF.
func (s *Streamer) makeStreamClient(client *http.Client) *http.Client {
	c := *client
	c.Timeout = 0
	if t, ok := c.Transport.(*http.Transport); ok {
		tc := t.Clone()
		tc.ResponseHeaderTimeout = 20 * time.Second
		c.Transport = tc
	}
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("تحويلات كثيرة")
		}
		if !s.allowedURL(req.URL) {
			return fmt.Errorf("تحويل إلى نطاق غير مسموح: %s", req.URL.Hostname())
		}
		return nil
	}
	return &c
}

// NewStreamer يبدأ الاستماع على 127.0.0.1:0 (منفذ عشوائي يختاره النظام)
// ويتوقف تلقائياً عند إلغاء ctx.
func NewStreamer(ctx context.Context, opts cinemana.ClientOptions, client *http.Client) (*Streamer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := ln.Addr().(*net.TCPAddr).Port

	s := &Streamer{opts: opts, port: port}
	s.client = s.makeStreamClient(client)

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", s.guard(s.handleStream))
	mux.HandleFunc("/sub", s.guard(s.handleSubtitle))

	s.srv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// لا WriteTimeout عمداً: النقل قد يستمر طويلاً على روابط بطيئة.
	}
	s.baseURL = fmt.Sprintf("http://127.0.0.1:%d", port) // يُستخدم في StreamURL/SubtitleURL

	go func() {
		// Serve يعيد خطأ عند Stop() — نتجاهله بهدوء.
		_ = s.srv.Serve(ln)
	}()
	if ctx != nil {
		go func() {
			<-ctx.Done()
			s.Stop()
		}()
	}
	return s, nil
}

// Stop إيقاف الخادم بلطف (عند إغلاق التطبيق).
func (s *Streamer) Stop() {
	_ = s.srv.Close()
}

// UpdateOptions تحديث الخيارات/العميل بعد حفظ الإعدادات دون إعادة تشغيل.
func (s *Streamer) UpdateOptions(opts cinemana.ClientOptions, client *http.Client) {
	var sc *http.Client
	if client != nil {
		sc = s.makeStreamClient(client)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opts = opts
	if sc != nil {
		s.client = sc
	}
}

// snapshot نسخة متسقة من الخيارات والعميل الحاليين.
func (s *Streamer) snapshot() (cinemana.ClientOptions, *http.Client) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.opts, s.client
}

// guard يقيّد الطلبات: GET/HEAD فقط، وترويسة Host يجب أن تكون عنوان
// الحلقة المحلية (يمنع DNS rebinding)، ويجيب على OPTIONS (CORS preflight).
func (s *Streamer) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil || (host != "127.0.0.1" && host != "localhost") {
			http.Error(w, "مضيف غير مسموح", http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Content-Length, Accept-Ranges")
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			next(w, r)
		case http.MethodOptions:
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Range")
			w.WriteHeader(http.StatusNoContent)
		default:
			w.Header().Set("Allow", "GET, HEAD, OPTIONS")
			http.Error(w, "طريقة غير مسموحة", http.StatusMethodNotAllowed)
		}
	}
}

// StreamURL يبني رابط بث محلي من رابط موقّع أصلي (b64url لتفادي مشاكل
// معاملات الاستعلام الطويلة والتوقيعات داخل الرابط).
func (s *Streamer) StreamURL(remote string) string {
	if s == nil || remote == "" {
		return ""
	}
	return s.baseURL + "/stream?u=" + base64.RawURLEncoding.EncodeToString([]byte(remote))
}

// SubtitleURL مثل StreamURL لكن لنقطة /sub (تحويل إلى VTT).
func (s *Streamer) SubtitleURL(remote string) string {
	if s == nil || remote == "" {
		return ""
	}
	return s.baseURL + "/sub?u=" + base64.RawURLEncoding.EncodeToString([]byte(remote))
}

// allowedHost فحص أن النطاق الهدف ضمن نطاقات المزود (منع استخدام
// التطبيق كبروكسي مفتوح لأي موقع آخر). المطابقة بحد نقطة فاصل.
func (s *Streamer) allowedHost(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if h == "" {
		return false
	}
	for _, d := range allowedDomains {
		if h == d || strings.HasSuffix(h, "."+d) {
			return true
		}
	}
	// النطاق الأساسي من الإعدادات (لو غيّره المستخدم لنطاق المزود).
	opts, _ := s.snapshot()
	if base, err := url.Parse(opts.BaseURL); err == nil && strings.EqualFold(h, base.Hostname()) {
		return true
	}
	return false
}

// allowedURL فحص المخطط والنطاق معاً. http مسموح فقط لنطاق الإعدادات
// الأساسي إن كان المستخدم قد حدده بـ http (شبكات محلية)، وإلا https فقط.
func (s *Streamer) allowedURL(u *url.URL) bool {
	if u == nil || !s.allowedHost(u.Hostname()) {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		opts, _ := s.snapshot()
		base, err := url.Parse(opts.BaseURL)
		return err == nil && base.Scheme == "http" && strings.EqualFold(u.Hostname(), base.Hostname())
	}
	return false
}

// decodeTarget فك الرابط الأصلي من معامل u والتحقق من سلامته.
func (s *Streamer) decodeTarget(raw string) (*url.URL, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("رابط غير صالح")
	}
	u, err := url.Parse(string(b))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("رابط غير صالح")
	}
	if !s.allowedURL(u) {
		return nil, fmt.Errorf("نطاق غير مسموح عبر الوكيل المحلي: %s", u.Hostname())
	}
	return u, nil
}

// newUpstreamRequest يبني طلباً للـ CDN بترويسات المحاكاة نفسها المستخدمة
// مع واجهة الـ API.
func (s *Streamer) newUpstreamRequest(r *http.Request, target *url.URL, forwardRange bool) (*http.Request, *http.Client, error) {
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	opts, client := s.snapshot()
	upReq.Header.Set("User-Agent", opts.UserAgent)
	upReq.Header.Set("Accept", "*/*")
	if opts.AppID != "" {
		upReq.Header.Set("X-Requested-With", opts.AppID)
	}
	if forwardRange {
		// identity: لا نريد ضغطاً من الـ CDN حتى تبقى نطاقات Range دقيقة
		// والبحث داخل الفيديو سليماً.
		upReq.Header.Set("Accept-Encoding", "identity")
		// تمرير رأس Range كما هو: عميل الفيديو يقول "أعطني البايتات من X
		// إلى Y" ونمرر الرسالة حرفياً للـ CDN ونعيد 206 للمتصفح.
		if rng := r.Header.Get("Range"); rng != "" {
			upReq.Header.Set("Range", rng)
		}
	}
	return upReq, client, nil
}

// readLimited يقرأ حتى limit بايت ويفشل إن زاد الجسم عنه (بدل القص الصامت).
func readLimited(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("الاستجابة أكبر من الحد المسموح (%d بايت)", limit)
	}
	return body, nil
}

// handleStream نقطة /stream: وسيط شفاف مع ترويسات + دعم Range + m3u8.
func (s *Streamer) handleStream(w http.ResponseWriter, r *http.Request) {
	target, err := s.decodeTarget(r.URL.Query().Get("u"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	upReq, client, err := s.newUpstreamRequest(r, target, true)
	if err != nil {
		http.Error(w, "طلب غير صالح", http.StatusBadRequest)
		return
	}

	resp, err := client.Do(upReq)
	if err != nil {
		// التفاصيل (تحوي الرابط الموقّع) تُسجَّل ولا تُعاد للمتصفح.
		log.Printf("proxy: فشل جلب الوسائط: %v", err)
		http.Error(w, "فشل جلب الوسائط من المزود", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// ---- حالة خاصة: قائمة m3u8 — نعيد كتابتها قبل إعادتها ----
	ct := resp.Header.Get("Content-Type")
	isPlaylist := strings.Contains(strings.ToLower(ct), "mpegurl") || strings.HasSuffix(strings.ToLower(target.Path), ".m3u8")
	if isPlaylist {
		if resp.StatusCode != http.StatusOK {
			http.Error(w, fmt.Sprintf("المزود أعاد HTTP %d لقائمة التشغيل", resp.StatusCode), http.StatusBadGateway)
			return
		}
		body, err := readLimited(resp.Body, maxPlaylistBytes)
		if err != nil {
			log.Printf("proxy: قائمة التشغيل: %v", err)
			http.Error(w, "فشل قراءة قائمة التشغيل", http.StatusBadGateway)
			return
		}
		// بعد التحويلات قد يختلف الرابط النهائي: نحل الروابط النسبية ضده.
		rewritten := rewriteM3U8(string(body), resp.Request.URL, s.StreamURL)
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(rewritten))
		}
		return
	}

	// ---- الوسائط العادية (mp4 وغيرها): تمرير شفاف ----
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Cache-Control"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	// نوع افتراضي منطقي إن أهمل الـ CDN ذكره (بعض المشغلات تتطلبه).
	if w.Header().Get("Content-Type") == "" {
		if strings.HasSuffix(strings.ToLower(target.Path), ".mp4") {
			w.Header().Set("Content-Type", "video/mp4")
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
	}
	w.WriteHeader(resp.StatusCode)

	if r.Method == http.MethodHead {
		return
	}

	// نسخ الجسم مع Flush: الدفعة الأولى تُدفع فوراً (إحساس أسرع عند الضغط
	// على تشغيل) ثم كل 512KB تقريباً.
	bufp := copyBufPool.Get().(*[]byte)
	defer copyBufPool.Put(bufp)
	buf := *bufp
	flusher, _ := w.(http.Flusher)
	written, first := 0, true
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return // العميل أغلق (أوقف التشغيل مثلاً) — طبيعي
			}
			written += n
			if flusher != nil && (first || written >= 512*1024) {
				flusher.Flush()
				written, first = 0, false
			}
		}
		if err != nil {
			return // EOF أو قطع اتصال — انتهينا
		}
	}
}

// handleSubtitle نقطة /sub: يجلب الترجمة ويضمن إخراجها VTT صالحاً.
func (s *Streamer) handleSubtitle(w http.ResponseWriter, r *http.Request) {
	target, err := s.decodeTarget(r.URL.Query().Get("u"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	upReq, client, err := s.newUpstreamRequest(r, target, false)
	if err != nil {
		http.Error(w, "طلب غير صالح", http.StatusBadRequest)
		return
	}

	resp, err := client.Do(upReq)
	if err != nil {
		log.Printf("proxy: فشل جلب الترجمة: %v", err)
		http.Error(w, "فشل جلب الترجمة", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// صفحة خطأ (404/403 HTML) يجب ألا تتحول إلى "ترجمة" صالحة.
	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("المزود أعاد HTTP %d للترجمة", resp.StatusCode), http.StatusBadGateway)
		return
	}
	body, err := readLimited(resp.Body, maxSubtitleBytes)
	if err != nil {
		log.Printf("proxy: الترجمة: %v", err)
		http.Error(w, "فشل قراءة الترجمة", http.StatusBadGateway)
		return
	}

	lower := strings.ToLower(target.Path)
	var out []byte
	switch {
	case strings.HasSuffix(lower, ".vtt") || strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "webvtt"):
		out = normalizeVTT(body)
	default: // srt وغيره: نحوله إلى VTT
		out = srtToVTT(body)
	}

	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(out)
	}
}

// =============================================================================
// تحويل SRT إلى VTT
// =============================================================================

// timingRegex يلتقط أزواج التوقيت بصيغة SRT: ‎00:01:02,500 --> ‎00:01:05,000
// مع تسامح واسع: ساعات برقم واحد وأجزاء ثانية ناقصة أو مفقودة تماماً
// (ملفات الترجمة الواقعية تتنوع كثيراً — اكتُشف بالاختبارات الفعلية).
var timingRegex = regexp.MustCompile(`(?m)(\d{1,2}):(\d{2}):(\d{2})(?:[,.](\d{1,3}))?(\s*-->\s*)(\d{1,2}):(\d{2}):(\d{2})(?:[,.](\d{1,3}))?`)

// padMS توحيد أجزاء الثانية إلى 3 خانات: "5"→"500"، "25"→"250"،
// "" (مفقود)→"000" — تفسير كسري صحيح للأرقام الناقصة.
func padMS(s string) string {
	for len(s) < 3 {
		s += "0"
	}
	return s
}

// pad2 إكمال رقم واحد إلى رقمين (ساعة "0" → "00").
func pad2(s string) string {
	for len(s) < 2 {
		s = "0" + s
	}
	return s
}

// decodeSubtitleBytes معالجة الترميز: لو لم يكن النص UTF-8 صالحاً نفترض
// أنه windows-1256 (الترميز العربي الشائع في ملفات SRT القديمة).
func decodeSubtitleBytes(data []byte) string {
	// إزالة BOM إن وُجد.
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	if utf8.Valid(data) {
		return string(data)
	}
	dec := charmap.Windows1256.NewDecoder()
	out, err := dec.Bytes(data)
	if err != nil {
		return string(data) // آخر الحلول: نمرر النص كما هو
	}
	return string(out)
}

// assTagRegex يزيل وسوم تنسيق ASS الموروثة من بعض ملفات SRT مثل
// {\an8} (تموضع) و{\i1} (مائل) — عرضها حرفياً يشوّه النص.
var assTagRegex = regexp.MustCompile(`\{\\[^}]*\}`)

// stripASSTags حذف وسوم ASS من نص الترجمة.
func stripASSTags(text string) string {
	return assTagRegex.ReplaceAllString(text, "")
}

// srtToVTT التحويل الأساسي: توحيد التوقيتات + رأس WEBVTT + إزالة وسوم ASS.
// بنية SRT (أرقام أسطر الكتلة + النص) تبقى صالحة في VTT لأن السطر
// الرقمي يُفسَّر كمعرّف كتلة اختياري — لا حاجة لحذفه.
func srtToVTT(data []byte) []byte {
	text := decodeSubtitleBytes(data)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = timingRegex.ReplaceAllStringFunc(text, func(m string) string {
		p := timingRegex.FindStringSubmatch(m)
		return fmt.Sprintf("%s:%s:%s.%s%s%s:%s:%s.%s",
			pad2(p[1]), p[2], p[3], padMS(p[4]),
			p[5],
			pad2(p[6]), p[7], p[8], padMS(p[9]))
	})
	text = stripASSTags(text)
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "WEBVTT") {
		text = "WEBVTT\n\n" + text
	}
	return []byte(text + "\n")
}

// normalizeVTT تمرير VTT مع تنظيف خفيف (BOM/نهايات أسطر/وسوم ASS) —
// يُستخدم للملفات التي هي أصل VTT.
func normalizeVTT(data []byte) []byte {
	text := decodeSubtitleBytes(data)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = stripASSTags(text)
	return []byte(strings.TrimSpace(text) + "\n")
}

// =============================================================================
// إعادة كتابة قوائم m3u8
// =============================================================================

// uriAttrRegex يلتقط URI="..." داخل أسطر التوجيه (EXT-X-MEDIA / EXT-X-KEY / EXT-X-MAP).
var uriAttrRegex = regexp.MustCompile(`URI="([^"]+)"`)

// rewriteM3U8 يعيد كتابة كل روابط القائمة (مستويات الجودة، المقاطع،
// مفاتيح التشفير، الترجمات) لتشير إلى الوكيل المحلي: هكذا تمر كل طلبات
// hls.js عبر الوكيل فلا تصطدم بحجب CORS ولا تفقد ترويسات المحاكاة.
func rewriteM3U8(content string, baseURL *url.URL, wrap func(string) string) string {
	resolve := func(ref string) string {
		u, err := url.Parse(strings.ReplaceAll(ref, "\\", ""))
		if err != nil {
			return ref
		}
		return baseURL.ResolveReference(u).String()
	}
	content = strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case strings.HasPrefix(trimmed, "#"):
			// أسطر التوجيه: نعيد كتابة قيمة URI="..." فقط إن وجدت.
			if uriAttrRegex.MatchString(trimmed) {
				lines[i] = uriAttrRegex.ReplaceAllStringFunc(trimmed, func(m string) string {
					inner := uriAttrRegex.FindStringSubmatch(m)[1]
					if strings.HasPrefix(strings.ToLower(inner), "data:") {
						return m // مفتاح/ملف مضمّن: لا يُمرَّر عبر الوكيل
					}
					return `URI="` + wrap(resolve(inner)) + `"`
				})
			}
		default:
			// سطر رابط (مستوى جودة أو مقطع): نغلفه بالوكيل.
			lines[i] = wrap(resolve(trimmed))
		}
	}
	return strings.Join(lines, "\n")
}
