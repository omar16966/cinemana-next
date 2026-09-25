// app.go (desktop)
// =============================================================================
// طبقة الربط بين الواجهة (frontend) وطبقة الخدمة (core/cinemana).
//
// كيف تعمل الربطة؟ Wails يصعّد كل دالة عامة (Exported) على بنية App إلى
// الـ WebView بحيث تستدعيها الجافاسكربت هكذا:
//
//	const data = await window.go.main.App.Search("فاست", "movie", 1)
//
// أي خطأ يعاد كـ Promise مرفوض (reject) برسالة نصية — فتعرضه الواجهة
// في شريط التنبيهات بدل انهيار الصمت.
//
// قاعدة معمارية: هذه الدوال لا تعرف شيئاً عن HTML أو CSS، وكل استدعاء
// شبكي حقيقي يحدث في core/cinemana فقط. كما أن روابط الفيديو المرسلة
// للواجهة معاد توجيهها عبر وكيل بث محلي (proxy.go) يضيف الترويسات
// المطلوبة ويحل مشاكل CORS — تفاصيلها في أعلى ذلك الملف.
// =============================================================================
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	cinemana "cinemana-probe/core/cinemana"
)

// App البنية المربوطة بالواجهة.
type App struct {
	ctx      context.Context // سياق Wails (للأحداث إن لزمت مستقبلاً)
	reqCtx   context.Context // سياق طويل الأمد لطلبات الشبكة
	cancel   context.CancelFunc
	client   *http.Client
	opts     cinemana.ClientOptions
	streamer *Streamer
	settings Settings
	mu       sync.Mutex // حماية إعادة بناء العميل عند حفظ الإعدادات
}

// NewApp تُنشئ البنية بقيم افتراضية؛ الإعدادات المحفوظة تُحمَّل في startup.
func NewApp() *App {
	return &App{settings: loadSettings()}
}

// startup تُستدعى مرة واحدة قبل عرض النافذة.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// سياق مستقل عن دورة حياة Wails لطلبات الشبكة الطويلة (البث).
	a.reqCtx, a.cancel = context.WithCancel(context.Background())
	a.rebuild()
	// وكيل البث: يستمع على 127.0.0.1 بمنفذ عشوائي متاح.
	s, err := NewStreamer(a.reqCtx, a.opts, a.client)
	if err != nil {
		// منفذ عشوائي يعني أن الفشل شبه مستحيل؛ إن حدث نبقيه ظاهراً في الكونسول.
		fmt.Println("تحذير: تعذر تشغيل وكيل البث المحلي:", err)
	}
	a.streamer = s
}

// shutdown تنظيف الموارد عند إغلاق النافذة.
func (a *App) shutdown(ctx context.Context) {
	if a.cancel != nil {
		a.cancel()
	}
	if a.streamer != nil {
		a.streamer.Stop()
	}
}

// rebuild يبني عميل HTTP من الإعدادات الحالية (يُستدعى عند الإقلاع
// وبعد أي حفظ للإعدادات ليأخذ التغييرات مفعول فوراً).
func (a *App) rebuild() {
	a.mu.Lock()
	defer a.mu.Unlock()

	base, err := cinemana.NormalizeBaseURL(a.settings.BaseURL)
	if err != nil {
		base = cinemana.DefaultOptions().BaseURL
	}
	a.opts = cinemana.DefaultOptions()
	a.opts.BaseURL = base
	if a.settings.UserAgent != "" {
		a.opts.UserAgent = a.settings.UserAgent
	}
	a.opts.InsecureTLS = a.settings.InsecureTLS

	client, _, err := cinemana.BuildClient(a.opts)
	if err != nil {
		// BuildClient لا يفشل عملياً (لا اتصال هنا)؛ الحفاظ على عميل قديم أفضل من لا شيء.
		return
	}
	a.client = client
}

// =============================================================================
// الدوال المربوطة بالواجهة (Bindings)
// =============================================================================

// Search بحث سريع عن فيلم/مسلسل. mediaType: "all" | "movie" | "series".
func (a *App) Search(query, mediaType string, page int) ([]cinemana.MediaSummary, error) {
	items, _, err := cinemana.Search(a.reqCtx, a.client, a.opts, query, mediaType, page)
	if err != nil {
		return nil, err
	}
	return cinemana.NormalizeSearch(items), nil
}

// GetDetails تفاصيل عمل (عنوان/بوستر/وصف/سنة/ترجمات/تصنيفات).
func (a *App) GetDetails(nb string) (*cinemana.DetailsOutput, error) {
	info, _, err := cinemana.GetVideoInfo(a.reqCtx, a.client, a.opts, nb)
	if err != nil {
		return nil, err
	}
	return cinemana.NormalizeDetails(info), nil
}

// GetEpisodes مواسم مسلسل وحلقاته (لكل حلقة nb يُستخدم مع GetPlayback).
func (a *App) GetEpisodes(seriesNB string) ([]cinemana.Season, error) {
	episodes, _, err := cinemana.GetEpisodes(a.reqCtx, a.client, a.opts, seriesNB)
	if err != nil {
		return nil, err
	}
	return cinemana.NormalizeEpisodes(episodes), nil
}

// GetCollection قائمة استعراض جاهزة للصفوف الأفقية وقوائم Top —
// المفاتيح المتاحة في core/cinemana/browse.go (latest_movies_foreign،
// top_movies، top_anime...). limit عدد العناصر المطلوب في الصف.
// نجلب زيادة عن الحدد ثم ندمج المكررات ونقص إلى الحد المطلوب، حتى لا
// يقلّ عددها الظاهر بعد الدمج.
func (a *App) GetCollection(key string, limit int) ([]cinemana.MediaSummary, error) {
	items, err := cinemana.GetCollection(a.reqCtx, a.client, a.opts, cinemana.CollectionKey(key), limit+8)
	if err != nil {
		return nil, err
	}
	out := cinemana.NormalizeSearch(items)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Browse تصفح بفلاتر لوحة اليمين (نوع/تصنيف/لغة/تقييم/سنة/بحث نصي).
func (a *App) Browse(f cinemana.BrowseFilters) ([]cinemana.MediaSummary, error) {
	// نجلب صفحة أكبر قليلاً ثم ندمج المكررات ونقص إلى 30، ليبقى منطق
	// "المزيد" في الواجهة (صفحة ممتلئة = يوجد المزيد) صحيحاً.
	items, err := cinemana.Browse(a.reqCtx, a.client, a.opts, f, 36)
	if err != nil {
		return nil, err
	}
	out := cinemana.NormalizeSearch(items)
	if len(out) > 30 {
		out = out[:30]
	}
	return out, nil
}

// GetCategories تصنيفات الخدمة (للقائمة المنسدلة في لوحة الفلترة).
func (a *App) GetCategories() ([]cinemana.CategoryItem, error) {
	return cinemana.GetMainCategories(a.reqCtx, a.client, a.opts)
}

// =============================================================================
// تصدير/استيراد بيانات المستخدم (المفضلة والقوائم والمشاهدات) بملف JSON
// =============================================================================

// ExportUserData يعرض حوار حفظ ملف ويكتب فيه ما تمرره الواجهة من بياناتها
// المحلية (المفضلة + القوائم الخاصة + المشاهدات الأخيرة) كنص JSON.
// data هو الـ JSON الجاهز من الواجهة؛ الدالة مسؤولة عن الحوار والملف فقط.
func (a *App) ExportUserData(data string) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "cinemana-next-data.json",
		Filters: []runtime.FileFilter{
			{DisplayName: "ملف JSON (*.json)", Pattern: "*.json"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // المستخدم ألغى الحوار — ليس خطأ
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ImportUserData يعرض حوار اختيار ملف JSON ويعيد محتوى نصياً للواجهة
// لتدمجه في بياناتها المحلية (الدمج والتحقق في الواجهة).
func (a *App) ImportUserData() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "ملف JSON (*.json)", Pattern: "*.json"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // أُلغي الحوار
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// QualityOut جودة جاهزة للتشغيل مع رابطين:
//   - local_url: عبر وكيل البث المحلي (للمشغل المدمج — يضيف الترويسات
//     ويحل CORS ويعيد كتابة قوائم m3u8 تلقائياً).
//   - remote_url: الرابط الموقّع الأصلي (للمشغل الخارجي mpv الذي نمرر
//     له الترويسات كوسيطات سطر أوامر).
type QualityOut struct {
	Resolution string `json:"resolution"`
	Kind       string `json:"kind"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	Bandwidth  int    `json:"bandwidth,omitempty"`
	Codecs     string `json:"codecs,omitempty"`
	LocalURL   string `json:"local_url"`
	RemoteURL  string `json:"remote_url"`
}

// SubtitleOut ترجمة جاهزة: local_url يخدم VTT محوّلاً (حتى ملفات SRT
// تُقبل في مشغل HTML5 الذي لا يفهم SRT مباشرة).
type SubtitleOut struct {
	Language  string `json:"language"`
	LangCode  string `json:"lang_code"`
	Format    string `json:"format"`
	LocalURL  string `json:"local_url"`
	RemoteURL string `json:"remote_url"`
}

// PlaybackInfo كل ما تحتاجه شاشة التشغيل لعمل واحد/حلقة واحدة.
type PlaybackInfo struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Qualities []QualityOut      `json:"qualities"`
	Subtitles []SubtitleOut     `json:"subtitles"`
	HLS       *cinemana.HLSInfo `json:"hls,omitempty"`
	ExpiresOn string            `json:"signed_urls_expire_on,omitempty"`
}

// GetPlayback يجلب جودات الفيديو والترجمات لمعرّف nb (فيلم أو حلقة).
//
// alts: معرّفات النسخ المكررة لنفس العمل (من حقل alts في MediaSummary) —
// الخدمة تحتفظ بنسخ متعددة لبعض الأعمال وبعض نسخها ملفاتها محذوفة/ميتة؛
// إن لم ينتج المعرّف الأساسي أي ملف تشغيل نجرّب البدائل بالترتيب
// تلقائياً بدل إظهار رسالة خطأ.
func (a *App) GetPlayback(nb string, alts []string) (*PlaybackInfo, error) {
	candidates := append([]string{nb}, alts...)
	var lastErr error
	for i, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		pi, err := a.buildPlayback(candidate)
		if err == nil && len(pi.Qualities) > 0 {
			return pi, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("لا توجد ملفات تشغيل لهذا الإصدار")
		}
		// بين المرشحين فاصل قصير حتى لا نضغط على الخدمة.
		if i < len(candidates)-1 {
			time.Sleep(150 * time.Millisecond)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("لا توجد ملفات تشغيل متاحة لهذا العمل")
	}
	return nil, lastErr
}

// buildPlayback يجلب التفاصيل وملفات فيديو معرّف واحد ويبني PlaybackInfo.
// يجلب الاثنين على التوازي لتقليل زمن الانتظار قبل ظهور زر التشغيل.
func (a *App) buildPlayback(nb string) (*PlaybackInfo, error) {
	var (
		info              *cinemana.VideoInfo
		files             []cinemana.VideoFile
		infoErr, filesErr error
		wg                sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		info, _, infoErr = cinemana.GetVideoInfo(a.reqCtx, a.client, a.opts, nb)
	}()
	go func() {
		defer wg.Done()
		files, _, filesErr = cinemana.GetVideoFiles(a.reqCtx, a.client, a.opts, nb)
	}()
	wg.Wait()

	// ملفات فارغة = إصدار ميت: نعتبرها فشلاً ليتولى المرشح التالي.
	if filesErr == nil && len(files) == 0 {
		filesErr = fmt.Errorf("لا توجد ملفات تشغيل لهذا الإصدار")
	}
	if filesErr != nil {
		return nil, filesErr
	}
	// فشل التفاصيل ليس قاتلاً (بدون عنوان/ترجمات احتياطية فقط)؛ لكن نحاول
	// إعادة المحاولة بشكل متزامن إن فشلت مع نجاح الملفات لسبب عابر.
	if infoErr != nil {
		if info, _, infoErr = cinemana.GetVideoInfo(a.reqCtx, a.client, a.opts, nb); infoErr != nil {
			info = nil
		}
	}

	out, err := cinemana.NormalizeVideos(a.reqCtx, a.client, a.opts, nb, files)
	if err != nil {
		return nil, err
	}

	// ترجمات احتياطية من التفاصيل إن لم ترد مع ملفات الفيديو.
	subs := out.Subtitles
	if len(subs) == 0 && info != nil {
		subs = cinemana.NormalizeDetails(info).Subtitles
	}

	pi := &PlaybackInfo{
		ID:        nb,
		Qualities: make([]QualityOut, 0, len(out.Qualities)),
		Subtitles: make([]SubtitleOut, 0, len(subs)),
		HLS:       out.HLS,
	}
	for _, q := range out.Qualities {
		pi.Qualities = append(pi.Qualities, QualityOut{
			Resolution: q.Resolution,
			Kind:       q.Kind,
			Width:      q.Width,
			Height:     q.Height,
			Bandwidth:  q.Bandwidth,
			Codecs:     q.Codecs,
			LocalURL:   a.streamer.StreamURL(q.URL),
			RemoteURL:  q.URL,
		})
	}
	for _, s := range subs {
		pi.Subtitles = append(pi.Subtitles, SubtitleOut{
			Language:  s.Language,
			LangCode:  s.LangCode,
			Format:    s.Format,
			LocalURL:  a.streamer.SubtitleURL(s.URL),
			RemoteURL: s.URL,
		})
	}
	if info != nil {
		pi.Title = info.EnTitle
		if strings.TrimSpace(pi.Title) == "" {
			pi.Title = info.ArTitle
		}
		pi.ExpiresOn = info.ObjectURLExpiration
	}
	return pi, nil
}

// OpenInMPV يفتح الرابط الموقّع مباشرة في مشغل mpv الخارجي بضغطة زر،
// مع تمرير نفس ترويسات المحاكاة (User-Agent وX-Requested-With) كوسيطات،
// وملف الترجمة إن اختير — إذ يدعم mpv جلب الترجمات من روابط http مباشرة.
func (a *App) OpenInMPV(videoURL, subtitleURL, title string) error {
	path := a.findMPV()
	if path == "" {
		return fmt.Errorf("لم يتم العثور على مشغل mpv. ثبّته (مثلاً: winget install mpv) أو حدد مساره من الإعدادات")
	}

	// نفس ترويسات المحاكاة المستخدمة في واجهة الـ API، لئلا يرفض السيرفر
	// طلب الفيديو القادم من مشغل خارجي بم تعريف.
	args := []string{
		"--force-media-title=" + title,
		"--user-agent=" + a.opts.UserAgent,
	}
	if a.opts.AppID != "" {
		args = append(args, "--http-header-fields=X-Requested-With: "+a.opts.AppID)
	}
	if subtitleURL != "" {
		args = append(args, "--sub-file="+subtitleURL)
	}
	// "--" قبل الرابط يمنع تفسيره كخيارات حتى لو بدأ بشرطة.
	args = append(args, "--", videoURL)

	cmd := exec.Command(path, args...)
	// Start وليس Run: لا نريد توقف التطبيق/الواجهة بانتظار المشغل.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("تعذر تشغيل mpv: %w", err)
	}
	// نترك العملية تُجمع تلقائياً بعد الخروج لتفادي عمليات zombie.
	go func() { _ = cmd.Wait() }()
	return nil
}

// findMPV إيجاد مشغل mpv: المسار المحفوظ في الإعدادات أولاً، ثم PATH،
// ثم مواضع التثبيت الشائعة على ويندوز.
func (a *App) findMPV() string {
	if a.settings.MPVPath != "" {
		if p, err := exec.LookPath(a.settings.MPVPath); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("mpv"); err == nil {
		return p
	}
	localAppData := osLocalAppData()
	for _, candidate := range []string{
		filepath.Join(localAppData, "mpv", "mpv.exe"),
		filepath.Join(localAppData, "Programs", "mpv", "mpv.exe"),
		`C:\Program Files\mpv\mpv.exe`,
	} {
		if exists(candidate) {
			return candidate
		}
	}
	return ""
}

// =============================================================================
// الإعدادات
// =============================================================================

// GetSettings قراءة الإعدادات الحالية (تُستدعى عند فتح نافذة الإعدادات).
func (a *App) GetSettings() Settings { return a.settings }

// SaveSettings حفظ الإعدادات وإعادة بناء العميل لتصبح سارية فوراً.
func (a *App) SaveSettings(s Settings) error {
	if strings.TrimSpace(s.BaseURL) == "" {
		s.BaseURL = cinemana.DefaultOptions().BaseURL
	}
	if err := saveSettings(s); err != nil {
		return err
	}
	a.settings = s
	a.rebuild()

	// الوكيل يحمل نسخة من الخيارات للترويسات؛ نحدّثها دون إيقاف الخادم.
	if a.streamer != nil {
		a.streamer.UpdateOptions(a.opts, a.client)
	}
	return nil
}

// =============================================================================
// أدوات صغيرة
// =============================================================================
