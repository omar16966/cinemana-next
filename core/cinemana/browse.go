// browse.go
// =============================================================================
// طبقة الاستعراض: الصفوف الأفقية في الصفحة الرئيسية وقوائم "Top".
//
// نقاط النهاية هنا مستخرجة من تحليل تطبيق الويب الرسمي للخدمة (حزمة
// Angular في cinemana.shabakaty.cc) ثم جرّبت واحداً واحداً للتأكد:
//
//	latestMovies/level/{level}/itemsPerPage/48/page/{page}/   أحدث الأفلام (كل اللغات)
//	latestSeries/level/{level}/itemsPerPage/48/page/{page}/   أحدث المسلسلات
//	videosByCategory?categoryID=57&videoKind=1&offset=0       الأنمي (التصنيف 57 = رسوم متحركة)
//	videosByCategoryAndLanguage?category_id=&language_id=...   فلترة لغة + تصنيف
//	videosByCategory?...&orderby=DESCENDINGIMDBRATE            الأعلى تقييماً (للـ Top)
//
// ملاحظات مضمّنة من الفحص الفعلي:
//   - videoKind: 1 = فيلم، 2 = مسلسل (نفس دلالة حقل kind في العناصر).
//   - language_lighttigerNb في كل عنصر: 7 = أجنبي (إنجليزي)، 9 = عربي.
//   - التصنيف 57 = "رسوم متحركة/أنمي" (مؤكد من كود تطبيق الويب نفسه:
//     animationMovies كان يثبّت category_id=57).
//   - استجابة هذه النقاط تأتي بإحدى صيغتين: مصفوفة مباشرة، أو كائن
//     {"info":[...],"offset":N} — flexiblePage أدناه تقبل الاثنتين.
//   - level = مستوى الرقابة الأبوية في الخدمة؛ 3 = إظهار كل المحتوى.
//
// =============================================================================
package cinemana

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// حدود وثوابت الاستعراض.
const (
	pageSize      = 30 // عدد عناصر صفحة offset في نقاط videosByCategory*
	maxLimit      = 200
	latestPerPage = 48
	maxTopOffset  = 90 // لا نتجاوز 4 صفحات (offset 0..90) في قوائم Top
	pageCacheTTL  = 2 * time.Minute
	pageCacheMax  = 64
)

// ErrUnknownCollection مفتاح قائمة غير معروف (يُعامل كخطأ استخدام في CLI).
var ErrUnknownCollection = errors.New("مفتاح قائمة غير معروف")

// numericID قيم الفلاتر الرقمية (تصنيف/لغة/نوع) — تمنع حقن معاملات في الرابط.
var numericID = regexp.MustCompile(`^[0-9]{1,6}$`)

// ثوابت الاستعراض (مستخرجة من فحص تطبيق الويب الرسمي).
const (
	browseLevel       = "3"  // مستوى الرقابة الأبوية: 3 = الكل
	langForeign       = "7"  // أجنبي/إنجليزي
	langArabic        = "9"  // عربي
	categoryAnimation = "57" // رسوم متحركة / أنمي
	categoryAllMovies = "56" // يستخدمه تطبيق الويب كأساس لقوائم "الأعلى تقييماً"
	sortTopRated      = "DESCENDINGIMDBRATE"
)

// CollectionKey مفتاح قائمة استعراض جاهزة (يُمرر من الواجهة).
type CollectionKey string

const (
	ColLatestMoviesForeign CollectionKey = "latest_movies_foreign"
	ColLatestSeriesForeign CollectionKey = "latest_series_foreign"
	ColLatestMoviesAnime   CollectionKey = "latest_movies_anime"
	ColLatestSeriesAnime   CollectionKey = "latest_series_anime"
	ColLatestMoviesArabic  CollectionKey = "latest_movies_arabic"
	ColLatestSeriesArabic  CollectionKey = "latest_series_arabic"
	ColLatestAll           CollectionKey = "latest_all"
	ColTopMovies           CollectionKey = "top_movies"
	ColTopSeries           CollectionKey = "top_series"
	ColTopAnime            CollectionKey = "top_anime"
)

// =============================================================================
// أدوات فك الاستجابات
// =============================================================================

// flexiblePage تقبل صيغتي الاستجابة الملاحظتين: مصفوفة عناصر مباشرة،
// أو كائن {"info":[...],"offset":N} — فلا تنكسر الأدوات إن غيّرت الخدمة
// صيغة أحد المسارات.
type flexiblePage struct {
	Items []SearchItem
}

func (p *flexiblePage) UnmarshalJSON(data []byte) error {
	// نقرر الصيغة من أول محرف فلا تُخفى أخطاء الأنواع داخل المصفوفة.
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return json.Unmarshal(trimmed, &p.Items)
	}
	var obj struct {
		Info []SearchItem `json:"info"`
	}
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return err
	}
	p.Items = obj.Info
	return nil
}

// =============================================================================
// ذاكرة مؤقتة قصيرة الأمد + دمج الطلبات المتزامنة
//
// الصفحة الرئيسية تطلب عدة صفوف دفعة واحدة، وكل صف "latest_*" يجلب نفس
// الصفحات الأربع: بدون هذا نرسل ~24 طلباً بدل ~8. الإدخال ينتهي بعد دقيقتين
// (الروابط الموقّعة تبقى صالحة أطول بكثير) ولا تُخزَّن الأخطاء.
// =============================================================================

type cacheEntry struct {
	done  chan struct{}
	items []SearchItem
	err   error
	at    time.Time
}

var pageCache = struct {
	sync.Mutex
	m map[string]*cacheEntry
}{m: map[string]*cacheEntry{}}

func pruneCacheLocked(now time.Time) {
	for k, e := range pageCache.m {
		select {
		case <-e.done:
			if now.Sub(e.at) > pageCacheTTL {
				delete(pageCache.m, k)
			}
		default:
		}
	}
	for k := range pageCache.m { // سقف الحجم: نُخلي ما يلزم عشوائياً
		if len(pageCache.m) < pageCacheMax {
			break
		}
		delete(pageCache.m, k)
	}
}

// fetchJSONPage طلب موحّد يعيد عناصر نقطة استعراض واحدة.
func fetchJSONPage(ctx context.Context, client *http.Client, opts ClientOptions, endpoint string) ([]SearchItem, error) {
	now := time.Now()
	pageCache.Lock()
	if e, ok := pageCache.m[endpoint]; ok {
		pageCache.Unlock()
		select {
		case <-e.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if e.err == nil && time.Since(e.at) <= pageCacheTTL {
			return append([]SearchItem(nil), e.items...), nil
		}
		// منتهي الصلاحية (أو فشل الرائد): نكمل بجلب جديد دون تخزين.
		return fetchPageUncached(ctx, client, opts, endpoint)
	}
	pruneCacheLocked(now)
	e := &cacheEntry{done: make(chan struct{})}
	pageCache.m[endpoint] = e
	pageCache.Unlock()

	items, err := fetchPageUncached(ctx, client, opts, endpoint)
	e.items, e.err, e.at = items, err, time.Now()
	close(e.done)
	if err != nil {
		pageCache.Lock()
		if pageCache.m[endpoint] == e {
			delete(pageCache.m, endpoint)
		}
		pageCache.Unlock()
		return nil, err
	}
	return append([]SearchItem(nil), items...), nil
}

func fetchPageUncached(ctx context.Context, client *http.Client, opts ClientOptions, endpoint string) ([]SearchItem, error) {
	var page flexiblePage
	if _, err := getJSON(ctx, client, opts, endpoint, &page); err != nil {
		return nil, err
	}
	return page.Items, nil
}

// fetchLatest يجلب أحدث الأفلام أو المسلسلات من صفحات متتالية (بالتوازي)
// ويدمجها مع إزالة التكرار — latestMovies/latestSeries لا تدعم فلترة لغة،
// فالدمج يتيح الفلترة محلياً بكمية كافية من العناصر.
func fetchLatest(ctx context.Context, client *http.Client, opts ClientOptions, listName string, pages int) ([]SearchItem, error) {
	type result struct {
		items []SearchItem
		err   error
	}
	results := make([]result, pages)
	var wg sync.WaitGroup
	for p := 0; p < pages; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			endpoint := strings.TrimRight(opts.BaseURL, "/") +
				"/api/android/" + listName + "/level/" + browseLevel + "/itemsPerPage/" + strconv.Itoa(latestPerPage) +
				"/page/" + strconv.Itoa(p) + "/"
			items, err := fetchJSONPage(ctx, client, opts, endpoint)
			results[p] = result{items, err}
		}(p)
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return nil, err // إلغاء/انتهاء مهلة: لا نعيد بيانات جزئية بصمت
	}
	if results[0].err != nil {
		return nil, results[0].err // الصفحة الأولى ضرورية؛ بقية الصفحات تحسين اختياري
	}
	merged := make([]SearchItem, 0, latestPerPage*pages)
	for _, r := range results {
		if r.err != nil || len(r.items) == 0 {
			break // صفحة فاشلة/فارغة: نتوقف عند آخر متتالية سليمة
		}
		merged = append(merged, r.items...)
	}
	return dedupe(merged), nil
}

// =============================================================================
// الفلاتر المحلية
// =============================================================================

// hasCategory فحص وجود تصنيف بالاسم الإنجليزي داخل عنصر.
func hasCategory(it SearchItem, name string) bool {
	for _, c := range it.Categories {
		if strings.EqualFold(c.EnTitle, name) {
			return true
		}
	}
	return false
}

// isAnime هل العنصر أنمي/رسوم متحركة؟
func isAnime(it SearchItem) bool { return hasCategory(it, "Animation") }

// withLanguage هل العنصر من لغة معينة (حسب language_lighttigerNb)؟
func withLanguage(it SearchItem, lang string) bool {
	return it.LanguageNb == lang
}

// filterLimit يرشح العناصر بشرط حتى بلوغ العدد المطلوب.
func filterLimit(items []SearchItem, limit int, keep func(SearchItem) bool) []SearchItem {
	out := make([]SearchItem, 0, limit)
	for _, it := range items {
		if len(out) >= limit {
			break
		}
		if keep(it) {
			out = append(out, it)
		}
	}
	return out
}

// sortByStarsDesc ترتيب تنازلي حسب التقييم (لدمج قوائم Top).
func sortByStarsDesc(items []SearchItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, _ := strconv.ParseFloat(strings.TrimSpace(items[i].Stars), 64)
		b, _ := strconv.ParseFloat(strings.TrimSpace(items[j].Stars), 64)
		return a > b
	})
}

// dedupe إزالة التكرار حسب nb دون تعديل الشريحة الأصلية. العناصر بلا
// معرّف تُحتفظ بها كما هي (لا تُدمج معاً).
func dedupe(items []SearchItem) []SearchItem {
	seen := make(map[string]bool, len(items))
	out := make([]SearchItem, 0, len(items))
	for _, it := range items {
		if it.NB != "" {
			if seen[it.NB] {
				continue
			}
			seen[it.NB] = true
		}
		out = append(out, it)
	}
	return out
}

// interleave يدمج قائمتين بالتناوب (للعرض المختلط أفلام/مسلسلات).
func interleave(a, b []SearchItem) []SearchItem {
	out := make([]SearchItem, 0, len(a)+len(b))
	for i := 0; i < len(a) || i < len(b); i++ {
		if i < len(a) {
			out = append(out, a[i])
		}
		if i < len(b) {
			out = append(out, b[i])
		}
	}
	return out
}

// truncate يقص إلى limit بأمان (limit موجب مضمون من المستدعي).
func truncate(items []SearchItem, limit int) []SearchItem {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

// =============================================================================
// GetCollection نقطة الدخول: مفتاح قائمة → عناصر خام جاهزة للتطبيع.
// limit هو الحد المطلوب لكل صف (الواجهة تطلب 12-18 عادة).
// =============================================================================
func GetCollection(ctx context.Context, client *http.Client, opts ClientOptions, key CollectionKey, limit int) ([]SearchItem, error) {
	if limit <= 0 {
		limit = 12
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	switch key {
	case ColLatestMoviesForeign, ColLatestMoviesArabic, ColLatestMoviesAnime:
		lang := langForeign
		if key == ColLatestMoviesArabic {
			lang = langArabic
		}
		items, err := fetchLatest(ctx, client, opts, "latestMovies", 4)
		if err != nil {
			return nil, err
		}
		if key == ColLatestMoviesAnime {
			return filterLimit(items, limit, isAnime), nil
		}
		return filterLimit(items, limit, func(it SearchItem) bool { return withLanguage(it, lang) }), nil

	case ColLatestSeriesForeign, ColLatestSeriesArabic, ColLatestSeriesAnime:
		lang := langForeign
		if key == ColLatestSeriesArabic {
			lang = langArabic
		}
		items, err := fetchLatest(ctx, client, opts, "latestSeries", 4)
		if err != nil {
			return nil, err
		}
		if key == ColLatestSeriesAnime {
			return filterLimit(items, limit, isAnime), nil
		}
		return filterLimit(items, limit, func(it SearchItem) bool { return withLanguage(it, lang) }), nil

	case ColLatestAll:
		movies, err := fetchLatest(ctx, client, opts, "latestMovies", 1)
		if err != nil {
			return nil, err
		}
		series, err := fetchLatest(ctx, client, opts, "latestSeries", 1)
		if err != nil {
			return nil, err
		}
		// تناوب أفلام/مسلسلات: الدمج المتتالي كان يملأ القائمة بالأفلام وحدها.
		return truncate(dedupe(interleave(movies, series)), limit), nil

	case ColTopMovies:
		items, err := fetchTopByCategory(ctx, client, opts, categoryAllMovies, "1", limit)
		return truncate(items, limit), err

	case ColTopSeries:
		items, err := fetchTopByCategory(ctx, client, opts, categoryAllMovies, "2", limit)
		return truncate(items, limit), err

	case ColTopAnime:
		// ندمج أفلام ومسلسلات الأنمي ونرتبها تنازلياً بالتقييم.
		movies, err := fetchTopByCategory(ctx, client, opts, categoryAnimation, "1", limit)
		if err != nil {
			return nil, err
		}
		series, err := fetchTopByCategory(ctx, client, opts, categoryAnimation, "2", limit)
		if err != nil {
			return nil, err
		}
		merged := dedupe(append(append([]SearchItem(nil), movies...), series...))
		sortByStarsDesc(merged)
		return truncate(merged, limit), nil

	default:
		return nil, errors.Join(ErrUnknownCollection, errors.New(string(key)))
	}
}

// fetchTopByCategory يجلب قائمة تصنيف بترقيم صفحات عبر offset حتى بلوغ
// الحد المطلوب — يدعم صفحات "المزيد" الكاملة (أكثر من 30 عنصراً).
func fetchTopByCategory(ctx context.Context, client *http.Client, opts ClientOptions, categoryID, videoKind string, limit int) ([]SearchItem, error) {
	var all []SearchItem
	for offset := 0; len(all) < limit && offset <= maxTopOffset; offset += pageSize {
		q := url.Values{}
		q.Set("categoryID", categoryID)
		q.Set("orderby", sortTopRated)
		q.Set("videoKind", videoKind)
		q.Set("offset", strconv.Itoa(offset))
		q.Set("level", browseLevel)
		endpoint := strings.TrimRight(opts.BaseURL, "/") + "/api/android/videosByCategory?" + q.Encode()
		items, err := fetchJSONPage(ctx, client, opts, endpoint)
		if err != nil {
			if offset == 0 || ctx.Err() != nil {
				return nil, err
			}
			break
		}
		if len(items) == 0 {
			break
		}
		all = append(all, items...)
	}
	return dedupe(all), nil
}

// =============================================================================
// لوحة الفلترة (يمين الواجهة): تصفح المحتوى بمعايير متعددة.
//
// الأساس: نقطة AdvancedSearch تدعم فعلياً (مؤكد بالفحص الحي) الفلاتر:
//   videoTitle, type=movie|series, star=<أدنى تقييم>,
//   year=<من>,<إلى>, category_id, page
// وفلترة اللغة تتم عبر videosByCategoryAndLanguage (لغة 7 أجنبي، 9 عربي).
// =============================================================================

// BrowseFilters فلاتر التصفح المرسلة من لوحة اليمين.
type BrowseFilters struct {
	Query      string `json:"query"`       // بحث نصي (اختياري)
	VideoKind  string `json:"video_kind"`  // "" الكل، 1 فيلم، 2 مسلسل
	CategoryID string `json:"category_id"` // "" الكل، وإلا رقم تصنيف من mainCategories
	LanguageID string `json:"language_id"` // "" الكل، 7 أجنبي، 9 عربي
	MinStar    string `json:"min_star"`    // "" أو 1..9 (أدنى تقييم)
	YearFrom   string `json:"year_from"`   // سنة من (اختياري)
	YearTo     string `json:"year_to"`     // سنة إلى (اختياري)
	Page       int    `json:"page"`        // ترقيم يبدأ من 1
}

// CategoryItem تصنيف من mainCategories.
type CategoryItem struct {
	NB    string `json:"nb"`
	Title string `json:"title"`
}

// GetMainCategories قائمة تصنيفات الخدمة بالعربية (للقائمة المنسدلة).
func GetMainCategories(ctx context.Context, client *http.Client, opts ClientOptions) ([]CategoryItem, error) {
	endpoint := strings.TrimRight(opts.BaseURL, "/") + "/api/android/mainCategories?lang=ar"
	var cats []CategoryItem
	if _, err := getJSON(ctx, client, opts, endpoint, &cats); err != nil {
		return nil, err
	}
	return cats, nil
}

// kindToSearchType تحويل videoKind الرقمي إلى قيمة type في AdvancedSearch.
func kindToSearchType(kind string) (string, bool) {
	switch kind {
	case "1":
		return "movie", true
	case "2":
		return "series", true
	}
	return "", false
}

// Browse تنفيذ الفلترة الموحدة: بحث نصي أو تصفح بمعايير، مع دعم اللغة.
func Browse(ctx context.Context, client *http.Client, opts ClientOptions, f BrowseFilters, limit int) ([]SearchItem, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	f.Query = strings.TrimSpace(f.Query)
	f.CategoryID = strings.TrimSpace(f.CategoryID)
	f.LanguageID = strings.TrimSpace(f.LanguageID)
	f.VideoKind = strings.TrimSpace(f.VideoKind)

	// قيم الفلاتر تصل من الواجهة/سطر الأوامر: نقبل الأرقام فقط حتى لا يُحقن
	// معامل إضافي في الرابط (مثل "7&level=0").
	for name, v := range map[string]string{"category_id": f.CategoryID, "language_id": f.LanguageID, "video_kind": f.VideoKind} {
		if v != "" && !numericID.MatchString(v) {
			return nil, &FilterError{Field: name, Value: v}
		}
	}
	if _, ok := kindToSearchType(f.VideoKind); f.VideoKind != "" && !ok {
		return nil, &FilterError{Field: "video_kind", Value: f.VideoKind}
	}

	// ---- فلترة اللغة: مسار منفصل (videosByCategoryAndLanguage) ----
	// لا تتوفر لغة في AdvancedSearch؛ وهي لا تجمع مع البحث النصي (في تلك
	// الحالة نفلتر اللغة محلياً بعد البحث — انظر أسفل).
	if f.LanguageID != "" && f.Query == "" {
		category := f.CategoryID
		if category == "" {
			category = categoryAllMovies // 56: يعمل كـ"كل التصنيفات" في هذه النقطة (مؤكد بالفحص)
		}
		kinds := []string{f.VideoKind}
		if f.VideoKind == "" {
			kinds = []string{"1", "2"} // الكل: ندمج أفلاماً ومسلسلات
		}
		lists := make([][]SearchItem, 0, len(kinds))
		for i, kind := range kinds {
			// كل نوع له ترقيم مستقل: offset = 30 لكل صفحة.
			q := url.Values{}
			q.Set("category_id", category)
			q.Set("language_id", f.LanguageID)
			q.Set("videoKind", kind)
			q.Set("orderby", "NEWVIDEOS")
			q.Set("level", browseLevel)
			q.Set("offset", strconv.Itoa((f.Page-1)*pageSize))
			endpoint := strings.TrimRight(opts.BaseURL, "/") + "/api/android/videosByCategoryAndLanguage?" + q.Encode()
			items, err := fetchJSONPage(ctx, client, opts, endpoint)
			if err != nil {
				if i == 0 || ctx.Err() != nil {
					return nil, err
				}
				continue
			}
			lists = append(lists, items)
		}
		var merged []SearchItem
		if len(lists) == 2 {
			merged = interleave(lists[0], lists[1]) // تناوب: لا تطغى الأفلام على المسلسلات
		} else if len(lists) == 1 {
			merged = lists[0]
		}
		// التقييم والسنة غير مدعومين في هذا المسار: نطبقهما محلياً.
		merged = filterLimit(dedupe(merged), limit, localFilter(f))
		return merged, nil
	}

	// ---- المسار العام: AdvancedSearch بكل فلاتره ----
	q := url.Values{}
	q.Set("level", browseLevel)
	if t, ok := kindToSearchType(f.VideoKind); ok {
		q.Set("type", t)
	}
	if f.Query != "" {
		q.Set("videoTitle", f.Query)
	}
	if f.CategoryID != "" {
		q.Set("category_id", f.CategoryID)
	}
	if s := strings.TrimSpace(f.MinStar); s != "" && s != "0" {
		q.Set("star", s)
	}
	from, to := strings.TrimSpace(f.YearFrom), strings.TrimSpace(f.YearTo)
	if from != "" || to != "" {
		if from == "" {
			from = "1900"
		}
		if to == "" {
			to = strconv.Itoa(time.Now().Year() + 1)
		}
		q.Set("year", from+","+to)
	}
	if f.Page > 1 {
		q.Set("page", strconv.Itoa(f.Page))
	}
	endpoint := strings.TrimRight(opts.BaseURL, "/") + "/api/android/AdvancedSearch?" + q.Encode()

	var items []SearchItem
	if _, err := getJSON(ctx, client, opts, endpoint, &items); err != nil {
		return nil, err
	}
	items = dedupe(items)
	if f.LanguageID != "" { // بحث نصي + لغة: الفلترة المحلية بدل تجاهل اللغة
		items = filterLimit(items, len(items), func(it SearchItem) bool { return withLanguage(it, f.LanguageID) })
	}
	return truncate(items, limit), nil
}

// FilterError قيمة فلتر غير صالحة (رسالة واضحة بدل طلب مشوّه).
type FilterError struct{ Field, Value string }

func (e *FilterError) Error() string {
	return "قيمة غير صالحة للفلتر " + e.Field + ": " + strconv.Quote(e.Value)
}

// localFilter فلترة التقييم الأدنى ونطاق السنوات محلياً (للمسار الذي لا يدعمها).
func localFilter(f BrowseFilters) func(SearchItem) bool {
	minStar, _ := strconv.ParseFloat(strings.TrimSpace(f.MinStar), 64)
	from, _ := strconv.Atoi(strings.TrimSpace(f.YearFrom))
	to, _ := strconv.Atoi(strings.TrimSpace(f.YearTo))
	return func(it SearchItem) bool {
		if minStar > 0 {
			if st, _ := strconv.ParseFloat(strings.TrimSpace(it.Stars), 64); st < minStar {
				return false
			}
		}
		if from > 0 || to > 0 {
			y, err := strconv.Atoi(strings.TrimSpace(it.Year))
			if err != nil || (from > 0 && y < from) || (to > 0 && y > to) {
				return false
			}
		}
		return true
	}
}
