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
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

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
	startErr error        // سبب تعذّر تشغيل وكيل البث (إن حدث)
	mu       sync.RWMutex // يحمي client/opts/settings: Wails ينفذ الدوال المربوطة بالتوازي
}

// حدود أمان لمدخلات الدوال المربوطة ولملفات الاستيراد/التصدير.
const (
	maxCollectionLimit = 100
	maxUserDataBytes   = 10 << 20
)

// appState نسخة متسقة من حالة التطبيق تُؤخذ تحت القفل ثم تُستخدم بلا قفل.
type appState struct {
	ctx      context.Context
	client   *http.Client
	opts     cinemana.ClientOptions
	settings Settings
}

// wailsCtx سياق Wails (مطلوب لحوارات النظام).
func (a *App) wailsCtx() context.Context {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.ctx
}

// state يأخذ لقطة متسقة (سياق الطلبات + العميل + الخيارات + الإعدادات).
func (a *App) state() appState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return appState{ctx: a.reqCtx, client: a.client, opts: a.opts, settings: a.settings}
}

// NewApp تُنشئ البنية بقيم افتراضية؛ الإعدادات المحفوظة تُحمَّل في startup.
func NewApp() *App {
	return &App{settings: loadSettings()}
}

// startup تُستدعى مرة واحدة قبل عرض النافذة.
func (a *App) startup(ctx context.Context) {
	// سياق مستقل عن دورة حياة Wails لطلبات الشبكة الطويلة (البث).
	reqCtx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.ctx = ctx
	a.reqCtx, a.cancel = reqCtx, cancel
	a.mu.Unlock()
	a.rebuild()

	// وكيل البث: يستمع على 127.0.0.1 بمنفذ عشوائي متاح.
	st := a.state()
	s, err := NewStreamer(reqCtx, st.opts, st.client)
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		// منفذ عشوائي يعني أن الفشل شبه مستحيل؛ نحفظ السبب ليظهر للمستخدم
		// عند أول محاولة تشغيل بدل روابط فارغة صامتة.
		log.Printf("تعذر تشغيل وكيل البث المحلي: %v", err)
		a.startErr = err
		return
	}
	a.streamer = s
}

// shutdown تنظيف الموارد عند إغلاق النافذة.
func (a *App) shutdown(ctx context.Context) {
	a.mu.RLock()
	cancel, streamer := a.cancel, a.streamer
	a.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
	if streamer != nil {
		streamer.Stop()
	}
}

// buildOptions يشتق خيارات العميل من الإعدادات (دالة نقية بلا حالة).
func buildOptions(set Settings) cinemana.ClientOptions {
	opts := cinemana.DefaultOptions()
	if base, err := cinemana.NormalizeBaseURL(set.BaseURL); err == nil {
		opts.BaseURL = base
	}
	if set.UserAgent != "" {
		opts.UserAgent = set.UserAgent
	}
	opts.InsecureTLS = set.InsecureTLS
	return opts
}

// rebuild يبني عميل HTTP من الإعدادات الحالية (يُستدعى عند الإقلاع
// وبعد أي حفظ للإعدادات ليأخذ التغييرات مفعول فوراً). لا يُعدَّل أي شيء
// إذا فشل البناء، فلا تتباعد opts عن client.
func (a *App) rebuild() {
	a.mu.RLock()
	set := a.settings
	a.mu.RUnlock()

	opts := buildOptions(set)
	client, _, err := cinemana.BuildClient(opts)
	if err != nil {
		log.Printf("تعذر بناء عميل HTTP: %v", err)
		return
	}

	a.mu.Lock()
	old := a.client
	a.opts, a.client = opts, client
	a.mu.Unlock()
	if old != nil {
		old.CloseIdleConnections() // لا نترك اتصالات العميل القديم معلّقة
	}
}

// =============================================================================
// الدوال المربوطة بالواجهة (Bindings)
// =============================================================================

// Search بحث سريع عن فيلم/مسلسل. mediaType: "all" | "movie" | "series".
func (a *App) Search(query, mediaType string, page int) ([]cinemana.MediaSummary, error) {
	st := a.state()
	items, _, err := cinemana.Search(st.ctx, st.client, st.opts, query, mediaType, page)
	if err != nil {
		return nil, err
	}
	return cinemana.NormalizeSearch(items), nil
}

// GetDetails تفاصيل عمل (عنوان/بوستر/وصف/سنة/ترجمات/تصنيفات).
func (a *App) GetDetails(nb string) (*cinemana.DetailsOutput, error) {
	st := a.state()
	info, _, err := cinemana.GetVideoInfo(st.ctx, st.client, st.opts, nb)
	if err != nil {
		return nil, err
	}
	return cinemana.NormalizeDetails(info), nil
}

// GetEpisodes مواسم مسلسل وحلقاته (لكل حلقة nb يُستخدم مع GetPlayback).
func (a *App) GetEpisodes(seriesNB string) ([]cinemana.Season, error) {
	st := a.state()
	episodes, _, err := cinemana.GetEpisodes(st.ctx, st.client, st.opts, seriesNB)
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
	// الحد يأتي من الواجهة: نقيّده (القيم السالبة كانت تسبب panic عند القص).
	if limit < 1 {
		limit = 12
	}
	if limit > maxCollectionLimit {
		limit = maxCollectionLimit
	}
	st := a.state()
	items, err := cinemana.GetCollection(st.ctx, st.client, st.opts, cinemana.CollectionKey(key), limit+8)
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
	st := a.state()
	items, err := cinemana.Browse(st.ctx, st.client, st.opts, f, 36)
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
	st := a.state()
	return cinemana.GetMainCategories(st.ctx, st.client, st.opts)
}

// =============================================================================
// تصدير/استيراد بيانات المستخدم (المفضلة والقوائم والمشاهدات) بملف JSON
// =============================================================================

// ExportUserData يعرض حوار حفظ ملف ويكتب فيه ما تمرره الواجهة من بياناتها
// المحلية (المفضلة + القوائم الخاصة + المشاهدات الأخيرة) كنص JSON.
// data هو الـ JSON الجاهز من الواجهة؛ الدالة مسؤولة عن الحوار والملف فقط،
// وتتحقق أن النص JSON صالح وبحجم معقول، وتكتب بشكل ذري (لا ملف نصف مكتوب).
func (a *App) ExportUserData(data string) (string, error) {
	if len(data) > maxUserDataBytes {
		return "", errors.New("البيانات أكبر من الحد المسموح للتصدير")
	}
	if !json.Valid([]byte(data)) {
		return "", errors.New("البيانات المراد تصديرها ليست JSON صالحاً")
	}
	path, err := runtime.SaveFileDialog(a.wailsCtx(), runtime.SaveDialogOptions{
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
	if filepath.Ext(path) == "" {
		path += ".json"
	}
	if err := writeFileAtomic(path, []byte(data), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// ImportUserData يعرض حوار اختيار ملف JSON ويعيد محتوى نصياً للواجهة
// لتدمجه في بياناتها المحلية (الدمج والتحقق في الواجهة). نرفض الملفات
// الضخمة وغير JSON قبل تمريرها عبر الجسر لتفادي تجمّد الواجهة.
func (a *App) ImportUserData() (string, error) {
	path, err := runtime.OpenFileDialog(a.wailsCtx(), runtime.OpenDialogOptions{
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
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if st.Size() > maxUserDataBytes {
		return "", errors.New("الملف أكبر من الحد المسموح (10 ميغابايت)")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !json.Valid(data) {
		return "", errors.New("الملف ليس JSON صالحاً")
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
	st := a.state()
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
			select {
			case <-time.After(150 * time.Millisecond):
			case <-st.ctx.Done():
				return nil, st.ctx.Err()
			}
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
	st := a.state()
	a.mu.RLock()
	streamer, startErr := a.streamer, a.startErr
	a.mu.RUnlock()
	if streamer == nil {
		if startErr == nil {
			startErr = errors.New("وكيل البث غير جاهز")
		}
		return nil, fmt.Errorf("وكيل البث المحلي غير متاح: %w", startErr)
	}

	var (
		info              *cinemana.VideoInfo
		files             []cinemana.VideoFile
		infoErr, filesErr error
		wg                sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		info, _, infoErr = cinemana.GetVideoInfo(st.ctx, st.client, st.opts, nb)
	}()
	go func() {
		defer wg.Done()
		files, _, filesErr = cinemana.GetVideoFiles(st.ctx, st.client, st.opts, nb)
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
		if info, _, infoErr = cinemana.GetVideoInfo(st.ctx, st.client, st.opts, nb); infoErr != nil {
			info = nil
		}
	}

	out, err := cinemana.NormalizeVideos(st.ctx, st.client, st.opts, nb, files)
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
			LocalURL:   streamer.StreamURL(q.URL),
			RemoteURL:  q.URL,
		})
	}
	for _, s := range subs {
		pi.Subtitles = append(pi.Subtitles, SubtitleOut{
			Language:  s.Language,
			LangCode:  s.LangCode,
			Format:    s.Format,
			LocalURL:  streamer.SubtitleURL(s.URL),
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
//
// الروابط تأتي من الواجهة، لذلك نقبل فقط روابط https على نطاقات المزود
// (أو رابط الوكيل المحلي)؛ وإلا لفتح mpv ملفات محلية أو بروتوكولات
// مثل ytdl:// أو سحب ملف ترجمة من مسار تعسفي.
func (a *App) OpenInMPV(videoURL, subtitleURL, title string) error {
	st := a.state()
	a.mu.RLock()
	streamer := a.streamer
	a.mu.RUnlock()
	if !mediaURLAllowed(streamer, videoURL) {
		return errors.New("رابط الفيديو غير مسموح")
	}
	if subtitleURL != "" && !mediaURLAllowed(streamer, subtitleURL) {
		return errors.New("رابط الترجمة غير مسموح")
	}
	path := findMPV(st.settings)
	if path == "" {
		return fmt.Errorf("لم يتم العثور على مشغل mpv. ثبّته (مثلاً: winget install mpv) أو حدد مساره من الإعدادات")
	}

	// تنظيف العنوان من محارف التحكم (أسطر جديدة...) قبل تمريره كوسيط.
	title = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, title)

	// نفس ترويسات المحاكاة المستخدمة في واجهة الـ API، لئلا يرفض السيرفر
	// طلب الفيديو القادم من مشغل خارجي بلا تعريف.
	args := []string{
		"--force-media-title=" + title,
		"--user-agent=" + st.opts.UserAgent,
	}
	if st.opts.AppID != "" {
		args = append(args, "--http-header-fields=X-Requested-With: "+st.opts.AppID)
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

// mediaURLAllowed هل الرابط https على نطاق مزود، أو رابط الوكيل المحلي؟
func mediaURLAllowed(s *Streamer, raw string) bool {
	if s == nil || raw == "" {
		return false
	}
	if strings.HasPrefix(raw, s.baseURL+"/") {
		return true
	}
	u, err := url.Parse(raw)
	return err == nil && s.allowedURL(u)
}

// validMPVPath تحقق أن المسار المحفوظ يشير فعلاً إلى مشغل mpv (اسم الملف
// mpv أو mpv.exe) وليس ملفاً تنفيذياً تعسفياً — الإعداد قابل للتعديل
// من الواجهة.
func validMPVPath(p string) error {
	if p == "" {
		return nil
	}
	base := strings.ToLower(filepath.Base(p))
	base = strings.TrimSuffix(strings.TrimSuffix(base, ".exe"), ".com")
	if base != "mpv" {
		return errors.New("مسار mpv يجب أن يشير إلى الملف mpv أو mpv.exe")
	}
	if _, err := exec.LookPath(p); err != nil {
		return fmt.Errorf("تعذر العثور على mpv في المسار المحدد: %w", err)
	}
	return nil
}

// findMPV إيجاد مشغل mpv: المسار المحفوظ في الإعدادات أولاً، ثم PATH،
// ثم مواضع التثبيت الشائعة على ويندوز.
func findMPV(set Settings) string {
	if set.MPVPath != "" && validMPVPath(set.MPVPath) == nil {
		if p, err := exec.LookPath(set.MPVPath); err == nil {
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
func (a *App) GetSettings() Settings { return a.state().settings }

// SaveSettings حفظ الإعدادات وإعادة بناء العميل لتصبح سارية فوراً.
// نتحقق من المدخلات قبل الحفظ حتى لا يتباعد المحفوظ عن الفعلي.
func (a *App) SaveSettings(s Settings) error {
	s.BaseURL = strings.TrimSpace(s.BaseURL)
	s.UserAgent = strings.TrimSpace(s.UserAgent)
	s.MPVPath = strings.TrimSpace(s.MPVPath)
	if s.BaseURL == "" {
		s.BaseURL = cinemana.DefaultOptions().BaseURL
	}
	base, err := cinemana.NormalizeBaseURL(s.BaseURL)
	if err != nil {
		return fmt.Errorf("العنوان الأساسي غير صالح: %w", err)
	}
	s.BaseURL = base
	if strings.ContainsAny(s.UserAgent, "\r\n") {
		return errors.New("User-Agent لا يجوز أن يحوي أسطراً جديدة")
	}
	if err := validMPVPath(s.MPVPath); err != nil {
		return err
	}
	if err := saveSettings(s); err != nil {
		return err
	}
	a.mu.Lock()
	a.settings = s
	a.mu.Unlock()
	a.rebuild()

	// الوكيل يحمل نسخة من الخيارات للترويسات؛ نحدّثها دون إيقاف الخادم.
	st := a.state()
	a.mu.RLock()
	streamer := a.streamer
	a.mu.RUnlock()
	if streamer != nil {
		streamer.UpdateOptions(st.opts, st.client)
	}
	return nil
}
