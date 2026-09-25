// main.go
// =============================================================================
// أداة سطر أوامر (CLI) لاختبار نقاط اتصال خدمة سينمانا — المرحلة الأولى.
//
// منطق الشبكة والـ API نفسه عُزل في الحزمة المشتركة core/cinemana حتى
// يستفيد منه تطبيق سطح المكتب (desktop/) دون تكرار — انظر README.
//
// الاستخدام:
//
//	cinemana-probe <command> [arguments] [flags]
//
// الأوامر:
//
//	probe                  فحص شامل: DNS، الوصول للنطاق، إعادة التوجيه، جرب نقطة بحث
//	search  <عنوان>        البحث عن فيلم/مسلسل            (--type movie|series|all)
//	details <id>           تفاصيل العمل (عنوان، بوستر، وصف، سنة...)
//	videos  <id>           روابط الفيديو المباشرة + الجودات + الترجمات
//	seasons <id>           مواسم المسلسل وحلقاته
//	all     <عنوان>        بحث ثم جلب التفاصيل والروابط للنتيجة الأولى (--index)
//
// كل المخرجات JSON منسق بمسافات بادئة (UTF-8 بلا تهريب لأحرف عربية).
// الأمثلة الكاملة في README.md.
// =============================================================================
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	cinemana "cinemana-probe/core/cinemana"
)

// usageError خطأ في مدخلات المستخدم (يُخرج رمز خروج 2).
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// httpError خطأ بمستوى HTTP نحتفظ فيه برمز الحالة والرابط النهائي للتشخيص.
type httpError struct {
	status   int
	finalURL string
	msg      string
}

func (e *httpError) Error() string { return e.msg }

// Envelope غلاف موحد لكل مخرجات الأداة: نتيجة أو خطأ — يسهّل على أي مستهلك
// لاحق (تطبيق سطح المكتب) قراءة النتائج بشكل موحد.
type Envelope struct {
	OK      bool     `json:"ok"`
	Command string   `json:"command"`
	TookMS  int64    `json:"took_ms"`
	Data    any      `json:"data,omitempty"`
	Error   *ErrInfo `json:"error,omitempty"`
}

// ErrInfo تفاصيل الخطأ للعرض في الـ JSON.
type ErrInfo struct {
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status,omitempty"`
	FinalURL   string `json:"final_url,omitempty"`
}

// printJSON طباعة الغلاف منسقاً. SetEscapeHTML(false) مهمة: تُبقي النصوص
// العربية والأحرف الخاصة مقروءة كما هي بدلاً من تهريبها إلى ‎\u0623...‏
func printJSON(env Envelope) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(env)
}

// cliOptions الخيارات المشتركة بين كل الأوامر (تُقرأ من سطر الأوامر).
type cliOptions struct {
	baseURL    string
	dnsServer  string
	resolves   []string
	userAgent  string
	appID      string
	timeout    time.Duration
	insecure   bool
	mediaType  string
	page       int
	index      int
	noHLSParse bool
	// فلاتر أمر browse:
	filterCategory string
	filterLanguage string
	filterStar     string
	filterYearFrom string
	filterYearTo   string
}

// addFlags تسجيل الخيارات المشتركة على أي FlagSet.
func addFlags(fs *flag.FlagSet, o *cliOptions) {
	def := cinemana.DefaultOptions()
	fs.StringVar(&o.baseURL, "base-url", def.BaseURL, "العنوان الأساسي للخدمة (يتبع التحويل 302 تلقائياً)")
	fs.StringVar(&o.dnsServer, "dns", "", "خادم DNS محلي مثل 10.0.0.1 أو 10.0.0.1:53 (فارغ = خوادم النظام، DNS عادي بلا DoH)")
	fs.Var(&multiValue{&o.resolves}, "resolve", "تثبيت نطاق على IP، يكرر أو يفصل بفواصل: host=ip,host2=ip2")
	fs.StringVar(&o.userAgent, "ua", def.UserAgent, "ترويسة User-Agent (افتراضياً جهاز أندرويد واقعي)")
	fs.StringVar(&o.appID, "app-id", def.AppID, "قيمة X-Requested-With لحزمة التطبيق (فارغة = لا تُرسل)")
	fs.DurationVar(&o.timeout, "timeout", def.Timeout, "مهلة الطلب الواحد")
	fs.BoolVar(&o.insecure, "insecure", false, "تخطي التحقق من شهادة TLS (للاختبار داخل الشبكة المحلية فقط)")
	fs.StringVar(&o.mediaType, "type", "all", "نوع البحث: movie أو series أو all")
	fs.IntVar(&o.page, "page", 1, "رقم صفحة نتائج البحث")
	fs.IntVar(&o.index, "index", 0, "اختيار النتيجة رقم i في أمر all")
	fs.BoolVar(&o.noHLSParse, "no-hls-parse", false, "عدم جلب وتحليل قائمة m3u8 (أسرع عند بطء الشبكة)")
	// فلاتر أمر browse:
	fs.StringVar(&o.filterCategory, "cat", "", "browse: رقم التصنيف (57 أنمي — انظر mainCategories)")
	fs.StringVar(&o.filterLanguage, "lang", "", "browse: لغة المحتوى (7 أجنبي، 9 عربي)")
	fs.StringVar(&o.filterStar, "star", "", "browse: أدنى تقييم (1..9)")
	fs.StringVar(&o.filterYearFrom, "year-from", "", "browse: سنة من")
	fs.StringVar(&o.filterYearTo, "year-to", "", "browse: سنة إلى")
}

// multiValue يسمح بتكرار راية واحدة (--resolve a=1 --resolve b=2).
type multiValue struct{ target *[]string }

func (m *multiValue) String() string { return strings.Join(*m.target, ",") }
func (m *multiValue) Set(s string) error {
	*m.target = append(*m.target, s)
	return nil
}

// reorderArgs يعيد ترتيب معاملات سطر الأوامر بحيث تتقدم الأعلام على
// المعاملات الموضعية. رايات القيم (غير المنطقية) تلتقط القيمة التالية
// لها، و"--" توقف المعالجة وتترك الباقي كما هو.
func reorderArgs(fs *flag.FlagSet, args []string) []string {
	boolFlags := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) {
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			boolFlags[f.Name] = true
		}
	})
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i:]...)
			break
		}
		if len(a) > 1 && strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			name := strings.TrimLeft(a, "-")
			if eq := strings.Index(name, "="); eq >= 0 {
				continue // صيغة --name=value مكتملة
			}
			if !boolFlags[name] && i+1 < len(args) {
				i++
				flags = append(flags, args[i]) // راية ذات قيمة: التقط القيمة
			}
			continue
		}
		pos = append(pos, a)
	}
	return append(flags, pos...)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	command := os.Args[1]
	if command == "-h" || command == "--help" || command == "help" {
		usage()
		return
	}

	var opts cliOptions
	fs := flag.NewFlagSet(command, flag.ExitOnError)
	addFlags(fs, &opts)
	// إعادة ترتيب تلقائية: حزمة flag تتوقف عن قراءة الأعلام عند أول معامل
	// موضعي، فبدونها يُتجاهل --type المكتوب بعد عنوان البحث. نسمح بكتابة
	// الأعلام قبل المعاملات الموضعية أو بعدها.
	_ = fs.Parse(reorderArgs(fs, os.Args[2:]))
	args := fs.Args()

	env := Envelope{Command: command}

	// بناء العميل من الخيارات المشتركة.
	base, err := cinemana.NormalizeBaseURL(opts.baseURL)
	if err != nil {
		env.Error = &ErrInfo{Message: "عنوان أساسي غير صالح: " + err.Error()}
		printJSON(env)
		os.Exit(2)
	}
	def := cinemana.DefaultOptions()
	def.BaseURL = base
	def.Timeout = opts.timeout
	def.UserAgent = opts.userAgent
	def.AppID = opts.appID
	def.InsecureTLS = opts.insecure
	def.DNSServer = opts.dnsServer
	if def.Resolves, err = cinemana.ParseResolveFlags(opts.resolves); err != nil {
		env.Error = &ErrInfo{Message: err.Error()}
		printJSON(env)
		os.Exit(2)
	}

	client, resolver, err := cinemana.BuildClient(def)
	if err != nil {
		env.Error = &ErrInfo{Message: "فشل بناء عميل HTTP: " + err.Error()}
		printJSON(env)
		os.Exit(1)
	}
	// السياق مع مهلة إجمالية سخية (أوامر مثل all تنفذ عدة طلبات متتالية).
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	start := time.Now()
	runErr := dispatch(ctx, command, client, resolver, def, opts, args, &env)
	env.TookMS = time.Since(start).Milliseconds()

	if runErr != nil {
		env.OK = false
		info := &ErrInfo{Message: runErr.Error()}
		var he *httpError
		if errors.As(runErr, &he) {
			info.HTTPStatus = he.status
			info.FinalURL = he.finalURL
		}
		env.Error = info
		printJSON(env)
		var ue *usageError
		if errors.As(runErr, &ue) {
			os.Exit(2)
		}
		os.Exit(1)
	}
	env.OK = true
	printJSON(env)
}

// dispatch توجيه الأمر المناسب حسب أول معامل.
func dispatch(ctx context.Context, command string, client *http.Client, resolver *net.Resolver, def cinemana.ClientOptions, opts cliOptions, args []string, env *Envelope) error {
	switch command {
	case "probe":
		return runProbe(ctx, client, resolver, def, env)
	case "search":
		if len(args) < 1 {
			return &usageError{msg: "الاستخدام: cinemana-probe search \"عنوان العمل\""}
		}
		return runSearch(ctx, client, def, opts, args[0], env)
	case "details":
		if len(args) < 1 {
			return &usageError{msg: "الاستخدام: cinemana-probe details <id>"}
		}
		return runDetails(ctx, client, def, args[0], env)
	case "videos":
		if len(args) < 1 {
			return &usageError{msg: "الاستخدام: cinemana-probe videos <id>"}
		}
		return runVideos(ctx, client, def, args[0], !opts.noHLSParse, env)
	case "seasons":
		if len(args) < 1 {
			return &usageError{msg: "الاستخدام: cinemana-probe seasons <id المسلسل>"}
		}
		return runSeasons(ctx, client, def, args[0], env)
	case "all":
		if len(args) < 1 {
			return &usageError{msg: "الاستخدام: cinemana-probe all \"عنوان العمل\" --type movie|series"}
		}
		return runAll(ctx, client, def, opts, args[0], env)
	case "collection":
		if len(args) < 1 {
			return &usageError{msg: "الاستخدام: cinemana-probe collection <key> [limit] — مثال: top_movies، latest_movies_arabic"}
		}
		return runCollection(ctx, client, def, args[0], env)
	case "browse":
		return runBrowse(ctx, client, def, opts, args, env)
	default:
		usage()
		os.Exit(2)
		return nil
	}
}

// runSearch أمر search: بحث وطباعة النتائج منظمة.
func runSearch(ctx context.Context, client *http.Client, opts cinemana.ClientOptions, o cliOptions, query string, env *Envelope) error {
	items, meta, err := cinemana.Search(ctx, client, opts, query, o.mediaType, o.page)
	if err != nil {
		return wrapMeta(err, meta)
	}
	env.Data = map[string]any{
		"query":   query,
		"type":    o.mediaType,
		"page":    o.page,
		"count":   len(items),
		"results": cinemana.NormalizeSearch(items),
		"request": meta,
	}
	return nil
}

// runDetails أمر details: تفاصيل العمل بصيغة منظمة.
func runDetails(ctx context.Context, client *http.Client, opts cinemana.ClientOptions, nb string, env *Envelope) error {
	info, meta, err := cinemana.GetVideoInfo(ctx, client, opts, nb)
	if err != nil {
		return wrapMeta(err, meta)
	}
	env.Data = map[string]any{"details": cinemana.NormalizeDetails(info), "request": meta}
	return nil
}

// runVideos أمر videos: الجودات + الترجمات + تحليل HLS اختياري.
func runVideos(ctx context.Context, client *http.Client, opts cinemana.ClientOptions, nb string, parseHLS bool, env *Envelope) error {
	files, meta, err := cinemana.GetVideoFiles(ctx, client, opts, nb)
	if err != nil {
		return wrapMeta(err, meta)
	}
	out, err := cinemana.NormalizeVideos(ctx, client, opts, nb, files)
	if err != nil {
		return err
	}
	if !parseHLS {
		out.HLS = nil
	}
	// إن لم تأتِ ترجمات مع ملفات الفيديو نلجأ إلى ترجمات تفاصيل العمل.
	if len(out.Subtitles) == 0 {
		if info, _, err := cinemana.GetVideoInfo(ctx, client, opts, nb); err == nil {
			out.Subtitles = cinemana.NormalizeDetails(info).Subtitles
		}
	}
	env.Data = map[string]any{"videos": out, "request": meta}
	return nil
}

// runSeasons أمر seasons: مواسم مسلسل وحلقاته مجمعة ومرتبة.
func runSeasons(ctx context.Context, client *http.Client, opts cinemana.ClientOptions, seriesNB string, env *Envelope) error {
	episodes, meta, err := cinemana.GetEpisodes(ctx, client, opts, seriesNB)
	if err != nil {
		return wrapMeta(err, meta)
	}
	env.Data = map[string]any{
		"series_id": seriesNB,
		"seasons":   cinemana.NormalizeEpisodes(episodes),
		"total":     len(episodes),
		"request":   meta,
	}
	return nil
}

// runAll أمر all: بحث ثم التفاصيل والروابط معاً للنتيجة المختارة —
// أقرب محاكاة لسير تطبيق سطح المكتب الكامل في أمر واحد.
func runAll(ctx context.Context, client *http.Client, opts cinemana.ClientOptions, o cliOptions, query string, env *Envelope) error {
	items, meta, err := cinemana.Search(ctx, client, opts, query, o.mediaType, o.page)
	if err != nil {
		return wrapMeta(err, meta)
	}
	if len(items) == 0 {
		env.Data = map[string]any{"query": query, "count": 0, "results": []any{}}
		return nil
	}
	if o.index < 0 || o.index >= len(items) {
		return &usageError{msg: fmt.Sprintf("--index=%d خارج النطاق؛ عدد النتائج %d", o.index, len(items))}
	}
	pick := items[o.index]

	details, dmeta, err := cinemana.GetVideoInfo(ctx, client, opts, pick.NB)
	if err != nil {
		return wrapMeta(err, dmeta)
	}
	files, vmeta, err := cinemana.GetVideoFiles(ctx, client, opts, pick.NB)
	if err != nil {
		return wrapMeta(err, vmeta)
	}
	videos, err := cinemana.NormalizeVideos(ctx, client, opts, pick.NB, files)
	if err != nil {
		return err
	}
	if len(videos.Subtitles) == 0 {
		videos.Subtitles = cinemana.NormalizeDetails(details).Subtitles
	}

	data := map[string]any{
		"query":   query,
		"chosen":  cinemana.NormalizeSearch([]cinemana.SearchItem{pick})[0],
		"details": cinemana.NormalizeDetails(details),
		"videos":  videos,
	}
	// المسلسلات: نضيف المواسم تلقائياً لأن kind=2.
	if details.Kind == "2" {
		eps, smeta, err := cinemana.GetEpisodes(ctx, client, opts, pick.NB)
		if err != nil {
			return wrapMeta(err, smeta)
		}
		data["seasons"] = cinemana.NormalizeEpisodes(eps)
	}
	env.Data = data
	return nil
}

// runCollection أمر collection: جلب قائمة استعراض جاهزة (صفوف الرئيسية وTop).
func runCollection(ctx context.Context, client *http.Client, opts cinemana.ClientOptions, key string, env *Envelope) error {
	items, err := cinemana.GetCollection(ctx, client, opts, cinemana.CollectionKey(key), 12)
	if err != nil {
		return err
	}
	out := cinemana.NormalizeSearch(items)
	env.Data = map[string]any{"key": key, "count": len(out), "results": out}
	return nil
}

// runBrowse أمر browse: تصفح بفلاتر لوحة اليمين (نوع/تصنيف/لغة/تقييم/سنة).
func runBrowse(ctx context.Context, client *http.Client, opts cinemana.ClientOptions, o cliOptions, args []string, env *Envelope) error {
	f := cinemana.BrowseFilters{
		Query:      strings.Join(args, " "),
		VideoKind:  kindFlagToVideoKind(o.mediaType),
		CategoryID: o.filterCategory,
		LanguageID: o.filterLanguage,
		MinStar:    o.filterStar,
		YearFrom:   o.filterYearFrom,
		YearTo:     o.filterYearTo,
		Page:       o.page,
	}
	items, err := cinemana.Browse(ctx, client, opts, f, 30)
	if err != nil {
		return err
	}
	out := cinemana.NormalizeSearch(items)
	env.Data = map[string]any{"filters": f, "count": len(out), "results": out}
	return nil
}

// kindFlagToVideoKind تحويل --type إلى videoKind الرقمي المستخدم في الفلترة.
func kindFlagToVideoKind(mediaType string) string {
	switch strings.ToLower(mediaType) {
	case "movie":
		return "1"
	case "series":
		return "2"
	default:
		return ""
	}
}

// wrapMeta إثراء الخطأ ببيانات الاستجابة (رمز الحالة والرابط النهائي)
// إن كانت متاحة، حتى تظهر في حقل error من الـ JSON.
func wrapMeta(err error, meta cinemana.Meta) error {
	var he *httpError
	if errors.As(err, &he) {
		return err // مُوثق مسبقاً
	}
	if meta.Status != 0 {
		return &httpError{status: meta.Status, finalURL: meta.FinalURL, msg: err.Error()}
	}
	return err
}

// =============================================================================
// أمر probe — الفحص الشامل
// =============================================================================

// runProbe فحص تشخيصي شامل يعرض بالترتيب:
//  1. ما حلّه الـ DNS المحلي (أي خادم استخدمنا وأي عناوين IP أعادها).
//  2. هل النطاق الأساسي يستجيب، وإلى أين يحوّل (كشف التحويل .com → .cc).
//  3. هل نقطة البحث تعمل وتقبل ترويسات الأندرويد.
//  4. هل نقطة ملفات الفيديو تعمل بمعرّف حقيقي مأخوذ من نتائج البحث.
func runProbe(ctx context.Context, client *http.Client, resolver *net.Resolver, opts cinemana.ClientOptions, env *Envelope) error {
	type check struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		OK       bool   `json:"ok"`
		Status   int    `json:"http_status,omitempty"`
		FinalURL string `json:"final_url,omitempty"`
		TookMS   int64  `json:"took_ms"`
		Detail   string `json:"detail,omitempty"`
		Error    string `json:"error,omitempty"`
	}
	type probeData struct {
		DNS struct {
			Server      string   `json:"server_used"`
			Note        string   `json:"note"`
			ResolvedIPs []string `json:"resolved_ips,omitempty"`
		} `json:"dns"`
		Checks []check `json:"checks"`
	}
	data := &probeData{}

	// توثيق مصدر DNS المستخدم داخل المخرجات نفسها (يظهر في النتيجة).
	if opts.DNSServer != "" {
		data.DNS.Server = opts.DNSServer
		data.DNS.Note = "خادم DNS محلي محدد يدوياً (DNS عادي عبر البورت 53، بلا DoH)"
	} else {
		data.DNS.Server = "system"
		data.DNS.Note = "محلل نظام التشغيل الافتراضي (على ويندوز: خدمة DNS Client مع ذاكرة مؤقتة، بلا DoH)"
	}

	host := strings.TrimPrefix(strings.TrimPrefix(opts.BaseURL, "https://"), "http://")

	// 1) حل النطاق الأساسي عبر DNS المحلي.
	if ips, err := resolver.LookupHost(ctx, host); err == nil {
		data.DNS.ResolvedIPs = ips
	}

	doCheck := func(name, url string, body func() (int, string, string, error)) check {
		c := check{Name: name, URL: url}
		t0 := time.Now()
		status, final, detail, err := body()
		c.TookMS = time.Since(t0).Milliseconds()
		c.Status = status
		c.FinalURL = final
		c.Detail = detail
		if err != nil {
			c.Error = err.Error()
			c.OK = false
		} else {
			c.OK = true
		}
		return c
	}

	// 2) الوصول للنطاق الأساسي.
	c1 := doCheck("base_reachable", opts.BaseURL, func() (int, string, string, error) {
		resp, err := cinemana.DoRequest(ctx, client, opts, http.MethodGet, opts.BaseURL, nil)
		if err != nil {
			return 0, "", "", err
		}
		defer resp.Body.Close()
		// نتبدد الجسم فقط؛ الهدف قياس الوصول وليس قراءة الصفحة.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		detail := "HTTP " + resp.Status
		return resp.StatusCode, resp.Request.URL.String(), detail, nil
	})
	data.Checks = append(data.Checks, c1)

	// نحل نطاق الوجهة النهائية أيضاً إن اختلف عن الأصلي (مفيد لتشخيص CDN).
	if c1.FinalURL != "" {
		if fh := hostOf(c1.FinalURL); fh != "" && !strings.EqualFold(fh, host) {
			if ips, err := resolver.LookupHost(ctx, fh); err == nil {
				data.DNS.ResolvedIPs = append(data.DNS.ResolvedIPs, ips...)
			}
		}
	}

	// 3) نقطة البحث — نحفظ النتائج أيضاً لاستخدام معرّف حقيقي في الفحص الرابع.
	var searchItems []cinemana.SearchItem
	c2 := doCheck("search_endpoint", strings.TrimRight(opts.BaseURL, "/")+cinemana.PathSearch+"?videoTitle=test&type=movie", func() (int, string, string, error) {
		items, meta, err := cinemana.Search(ctx, client, opts, "test", "movie", 1)
		if err != nil {
			return meta.Status, meta.FinalURL, "", err
		}
		searchItems = items
		return meta.Status, meta.FinalURL, fmt.Sprintf("عدد النتائج: %d", len(items)), nil
	})
	data.Checks = append(data.Checks, c2)

	// 4) نقطة ملفات الفيديو بمعرّف حقيقي من نتائج البحث.
	if c2.OK && len(searchItems) > 0 {
		nb := searchItems[0].NB
		fURL := strings.TrimRight(opts.BaseURL, "/") + cinemana.PathVideoFiles + nb
		c3 := doCheck("video_files_endpoint", fURL, func() (int, string, string, error) {
			files, meta, err := cinemana.GetVideoFiles(ctx, client, opts, nb)
			if err != nil {
				return meta.Status, meta.FinalURL, "", err
			}
			return meta.Status, meta.FinalURL, fmt.Sprintf("عدد الجودات للمعرف %s: %d", nb, len(files)), nil
		})
		data.Checks = append(data.Checks, c3)
	}

	env.Data = data
	return nil
}

// hostOf استخراج النطاق من رابط (تسامحي — للعرض التشخيصي فقط).
func hostOf(raw string) string {
	i := strings.Index(raw, "://")
	if i < 0 {
		return ""
	}
	rest := raw[i+3:]
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		rest = rest[:j]
	}
	if k := strings.LastIndex(rest, "@"); k >= 0 {
		rest = rest[k+1:]
	}
	if k := strings.LastIndex(rest, ":"); k >= 0 && !strings.Contains(rest, "]") {
		rest = rest[:k]
	}
	return rest
}

// usage عرض المساعدة بالعربية.
func usage() {
	fmt.Print(`cinemana-probe — أداة اختبار نقاط اتصال سينمانا (المرحلة الأولى)

الاستخدام:
  cinemana-probe <command> [arguments] [flags]

الأوامر:
  probe                    فحص شامل للاتصال و DNS ونقاط النهاية
  search "عنوان"           البحث عن فيلم/مسلسل (--type movie|series|all --page N)
  details <id>             تفاصيل العمل: العنوان، البوستر، الوصف، السنة، الترجمات
  videos <id>              روابط الفيديو المباشرة مع الجودات والترجمات
  seasons <id>             مواسم المسلسل وحلقاته
  all "عنوان"              بحث ثم تفاصيل وروابط للنتيجة الأولى (--index i)

الخيارات المشتركة:
  --base-url URL           العنوان الأساسي (افتراضياً https://cinemana.shabakaty.com)
  --dns 10.0.0.1           فرض خادم DNS محلي (DNS عادي 53، بلا DoH)
  --resolve host=ip        تثبيت نطاق على IP (يمكن تكراره أو الفصل بفواصل)
  --ua "..."               ترويسة User-Agent بديلة
  --app-id pkg             قيمة X-Requested-With (افتراضي com.erthlink.tv، فارغ لإلغائه)
  --timeout 20s            مهلة الطلب الواحد
  --insecure               تخطي التحقق من شهادة TLS
  --type movie|series|all  نوع البحث (افتراضي all)
  --no-hls-parse           عدم تحليل قوائم m3u8

أمثلة:
  cinemana-probe probe
  cinemana-probe search "fast and furious" --type movie
  cinemana-probe details 5386
  cinemana-probe videos 5386
  cinemana-probe all "game of thrones" --type series
  cinemana-probe search "fast" --dns 10.0.0.1 --resolve cdn.shabakaty.cc=10.10.0.5

كل المخرجات JSON منسق (UTF-8). رمز الخروج: 0 نجاح، 1 خطأ، 2 خطأ استخدام.
`)
}
