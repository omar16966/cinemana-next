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
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

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
	var arr []SearchItem
	if err := json.Unmarshal(data, &arr); err == nil {
		p.Items = arr
		return nil
	}
	var obj struct {
		Info []SearchItem `json:"info"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	p.Items = obj.Info
	return nil
}

// fetchJSONPage طلب موحّد يعيد عناصر نقطة استعراض واحدة.
func fetchJSONPage(ctx context.Context, client *http.Client, opts ClientOptions, endpoint string) ([]SearchItem, error) {
	var page flexiblePage
	if _, err := getJSON(ctx, client, opts, endpoint, &page); err != nil {
		return nil, err
	}
	return page.Items, nil
}

// fetchLatest يجلب أحدث الأفلام أو المسلسلات من صفحات متتالية ويدمجها
// (مع إزالة التكرار) — latestMovies/latestSeries لا تدعم فلترة لغة،
// فالدمج يتيح الفلترة محلياً بكمية كافية من العناصر.
func fetchLatest(ctx context.Context, client *http.Client, opts ClientOptions, listName string, pages int) ([]SearchItem, error) {
	merged := make([]SearchItem, 0, 48*pages)
	seen := map[string]bool{}
	for p := 0; p < pages; p++ {
		endpoint := strings.TrimRight(opts.BaseURL, "/") +
			"/api/android/" + listName + "/level/" + browseLevel + "/itemsPerPage/48/page/" + strconv.Itoa(p) + "/"
		items, err := fetchJSONPage(ctx, client, opts, endpoint)
		if err != nil {
			if p == 0 {
				return nil, err // الصفحة الأولى ضرورية؛ بقية الصفحات تحسين اختياري
			}
			break
		}
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			if !seen[it.NB] {
				seen[it.NB] = true
				merged = append(merged, it)
			}
		}
	}
	return merged, nil
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

// dedupe إزالة التكرار حسب nb.
func dedupe(items []SearchItem) []SearchItem {
	seen := map[string]bool{}
	out := items[:0]
	for _, it := range items {
		if !seen[it.NB] {
			seen[it.NB] = true
			out = append(out, it)
		}
	}
	return out
}

// =============================================================================
// GetCollection نقطة الدخول: مفتاح قائمة → عناصر خام جاهزة للتطبيع.
// limit هو الحد المطلوب لكل صف (الواجهة تطلب 12-18 عادة).
// =============================================================================
func GetCollection(ctx context.Context, client *http.Client, opts ClientOptions, key CollectionKey, limit int) ([]SearchItem, error) {
	if limit <= 0 {
		limit = 12
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
		merged := dedupe(append(movies, series...))
		if len(merged) > limit {
			merged = merged[:limit]
		}
		return merged, nil

	case ColTopMovies:
		return fetchTopByCategory(ctx, client, opts, categoryAllMovies, "1", limit)

	case ColTopSeries:
		return fetchTopByCategory(ctx, client, opts, categoryAllMovies, "2", limit)

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
		merged := dedupe(append(movies, series...))
		sortByStarsDesc(merged)
		if len(merged) > limit {
			merged = merged[:limit]
		}
		return merged, nil

	default:
		return nil, &usageKeyError{msg: "مفتاح قائمة غير معروف: " + string(key)}
	}
}

// fetchTopByCategory يجلب قائمة تصنيف بترقيم صفحات عبر offset حتى بلوغ
// الحد المطلوب — يدعم صفحات "المزيد" الكاملة (أكثر من 30 عنصراً).
func fetchTopByCategory(ctx context.Context, client *http.Client, opts ClientOptions, categoryID, videoKind string, limit int) ([]SearchItem, error) {
	var all []SearchItem
	for offset := 0; len(all) < limit && offset < 90; offset += 30 {
		endpoint := strings.TrimRight(opts.BaseURL, "/") +
			"/api/android/videosByCategory?categoryID=" + categoryID +
			"&orderby=" + sortTopRated + "&videoKind=" + videoKind +
			"&offset=" + strconv.Itoa(offset) + "&level=" + browseLevel
		items, err := fetchJSONPage(ctx, client, opts, endpoint)
		if err != nil {
			if offset == 0 {
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

// usageKeyError خطأ مفتاح غير معروف (رسالة واضحة في الواجهة).
type usageKeyError struct{ msg string }

func (e *usageKeyError) Error() string { return e.msg }

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
	if f.Page <= 0 {
		f.Page = 1
	}

	// ---- فلترة اللغة: مسار منفصل (videosByCategoryAndLanguage) ----
	// لا تتوفر لغة في AdvancedSearch؛ وهي لا تجمع مع البحث النصي.
	if f.LanguageID != "" && strings.TrimSpace(f.Query) == "" {
		category := f.CategoryID
		if category == "" {
			category = categoryAllMovies // 56: يعمل كـ"كل التصنيفات" في هذه النقطة (مؤكد بالفحص)
		}
		kinds := []string{f.VideoKind}
		if f.VideoKind == "" {
			kinds = []string{"1", "2"} // الكل: ندمج أفلاماً ومسلسلات
		}
		var merged []SearchItem
		for i, kind := range kinds {
			// كل نوع له ترقيم مستقل بoffset=30 لكل صفحة.
			offset := strconv.Itoa((f.Page - 1) * 30)
			endpoint := strings.TrimRight(opts.BaseURL, "/") +
				"/api/android/videosByCategoryAndLanguage?category_id=" + category +
				"&language_id=" + f.LanguageID + "&videoKind=" + kind +
				"&orderby=NEWVIDEOS&level=" + browseLevel + "&offset=" + offset
			items, err := fetchJSONPage(ctx, client, opts, endpoint)
			if err != nil {
				if i == 0 {
					return nil, err
				}
				continue
			}
			merged = append(merged, items...)
		}
		merged = dedupe(merged)
		if len(merged) > limit {
			merged = merged[:limit]
		}
		return merged, nil
	}

	// ---- المسار العام: AdvancedSearch بكل فلاتره ----
	q := url.Values{}
	q.Set("level", browseLevel)
	if t, ok := kindToSearchType(f.VideoKind); ok {
		q.Set("type", t)
	}
	if s := strings.TrimSpace(f.Query); s != "" {
		q.Set("videoTitle", s)
	}
	if c := strings.TrimSpace(f.CategoryID); c != "" {
		q.Set("category_id", c)
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
			to = "2030"
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
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}
