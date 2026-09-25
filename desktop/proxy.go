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
	"context"
	"encoding/base64"
	"fmt"
	"io"
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

// Streamer خادم البث المحلي.
type Streamer struct {
	opts    cinemana.ClientOptions
	client  *http.Client // عميل بث: بلا مهلة كلية (الأفلام طويلة!) مع مهلة رأس استجابة فقط
	mux     *http.ServeMux
	srv     *http.Server
	ln      net.Listener
	baseURL string
	mu      sync.Mutex
}

// NewStreamer يبدأ الاستماع على 127.0.0.1:0 (منفذ عشوائي يختاره النظام).
func NewStreamer(ctx context.Context, opts cinemana.ClientOptions, client *http.Client) (*Streamer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := ln.Addr().(*net.TCPAddr).Port

	// عميل بث مخصص: نفس تجاوز البروكسي وDNS من النواة، لكن بلا مهلة
	// كلية (client.Timeout) لأن تشغيل فيلم قد يستغرق ساعتين؛ نكتفي
	// بمهلة على وصول رأس الاستجابة لاكتشاف تعطل الـ CDN مبكراً.
	streamClient := *client
	streamClient.Timeout = 0
	if t, ok := streamClient.Transport.(*http.Transport); ok {
		tc := t.Clone()
		tc.ResponseHeaderTimeout = 20 * time.Second
		streamClient.Transport = tc
	}

	s := &Streamer{opts: opts, client: &streamClient}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/stream", s.handleStream)
	s.mux.HandleFunc("/sub", s.handleSubtitle)

	s.srv = &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
		// لا WriteTimeout عمداً: النقل قد يستمر طويلاً على روابط بطيئة.
	}
	s.ln = ln
	s.baseURL = fmt.Sprintf("http://127.0.0.1:%d", port) // يُستخدم في StreamURL/SubtitleURL

	go func() {
		// Serve يعيد خطأ عند Stop() — نتجاهله بهدوء.
		_ = s.srv.Serve(ln)
	}()
	_ = ctx
	return s, nil
}

// Stop إيقاف الخادم بلطف (عند إغلاق التطبيق).
func (s *Streamer) Stop() {
	_ = s.srv.Close()
}

// UpdateOptions تحديث الخيارات/العميل بعد حفظ الإعدادات دون إعادة تشغيل.
func (s *Streamer) UpdateOptions(opts cinemana.ClientOptions, client *http.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opts = opts
	if client != nil {
		c := *client
		c.Timeout = 0
		s.client = &c
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
// التطبيق كبروكسي مفتوح لأي موقع آخر).
func (s *Streamer) allowedHost(host string) bool {
	h := strings.ToLower(host)
	if strings.HasSuffix(h, "shabakaty.cc") || strings.HasSuffix(h, "shabakaty.com") {
		return true
	}
	// النطاق الأساسي من الإعدادات (لو غيّره المستخدم لنطاق المزود).
	if base, err := url.Parse(s.opts.BaseURL); err == nil && strings.EqualFold(h, base.Hostname()) {
		return true
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
	if !s.allowedHost(u.Hostname()) {
		return nil, fmt.Errorf("نطاق غير مسموح عبر الوكيل المحلي: %s", u.Hostname())
	}
	return u, nil
}

// handleStream نقطة /stream: وسيط شفاف مع ترويسات + دعم Range + m3u8.
func (s *Streamer) handleStream(w http.ResponseWriter, r *http.Request) {
	target, err := s.decodeTarget(r.URL.Query().Get("u"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		http.Error(w, "طلب غير صالح", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	opts, client := s.opts, s.client
	s.mu.Unlock()

	// ترويسات المحاكاة نفسها المستخدمة مع واجهة الـ API.
	upReq.Header.Set("User-Agent", opts.UserAgent)
	upReq.Header.Set("Accept", "*/*")
	if opts.AppID != "" {
		upReq.Header.Set("X-Requested-With", opts.AppID)
	}
	// identity: لا نريد ضغطاً من الـ CDN حتى تبقى نطاقات Range دقيقة
	// والبحث داخل الفيديو سليماً.
	upReq.Header.Set("Accept-Encoding", "identity")
	// تمرير رأس Range كما هو: عميل الفيديو يقول "أعطني البايتات من X
	// إلى Y" ونمرر الرسالة حرفياً للـ CDN ونعيد 206 للمتصفح.
	if rng := r.Header.Get("Range"); rng != "" {
		upReq.Header.Set("Range", rng)
	}

	resp, err := client.Do(upReq)
	if err != nil {
		http.Error(w, "فشل جلب الوسائط من المزود: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// ---- حالة خاصة: قائمة m3u8 — نعيد كتابتها قبل إعادتها ----
	ct := resp.Header.Get("Content-Type")
	isPlaylist := strings.Contains(strings.ToLower(ct), "mpegurl") || strings.HasSuffix(strings.ToLower(target.Path), ".m3u8")
	if isPlaylist {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			http.Error(w, "فشل قراءة قائمة التشغيل", http.StatusBadGateway)
			return
		}
		rewritten := rewriteM3U8(string(body), target, s.StreamURL)
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write([]byte(rewritten))
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
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(resp.StatusCode)

	if r.Method == http.MethodHead {
		return
	}

	// نسخ الجسم مع Flush دوري: يجعل البداية تصل للمتصفح فوراً بدل
	// انتظار تعبئة مخازن داخلية — إحساس أسرع عند الضغط على تشغيل.
	buf := make([]byte, 64*1024)
	flusher, _ := w.(http.Flusher)
	written := 0
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return // العميل أغلق (أوقف التشغيل مثلاً) — طبيعي
			}
			written += n
			if flusher != nil && written >= 512*1024 {
				flusher.Flush()
				written = 0
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

	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		http.Error(w, "طلب غير صالح", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	opts, client := s.opts, s.client
	s.mu.Unlock()
	upReq.Header.Set("User-Agent", opts.UserAgent)
	upReq.Header.Set("Accept", "*/*")

	resp, err := client.Do(upReq)
	if err != nil {
		http.Error(w, "فشل جلب الترجمة: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
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
	// <track> يتطلب CORS حتماً — ترويسة تجعل الترجمة تعمل حتى لو تغير
	// مصدر الصفحة مستقبلاً.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
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
	data = []byte(strings.TrimPrefix(string(data), "\ufeff"))
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

// stripASSRemovalTags حذف وسوم ASS من نص الترجمة.
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
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
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
