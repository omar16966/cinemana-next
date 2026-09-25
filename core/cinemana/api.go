// api.go
// =============================================================================
// طبقة الاتصال بنقاط نهاية (Endpoints) سينمانا.
//
// المسارات أدناه مستخرجة من الفحص الفعلي للخدمة ومطابقة لمسارات تطبيق
// الأندرويد الرسمي (/api/android/). كل نقطة تعيد JSON نصياً.
//
// ملاحظة عملية مهمة: روابط الفيديو والصور والترجمات التي تعيدها الخدمة
// "موقعة" (Signed URLs) وتحمل تاريخ انتهاء صلاحية (انظر حقول
// objectUrlExpiration و Expires في الرابط نفسه). لذلك لا تخزّن هذه الروابط
// طويلاً؛ اطلبها من جديد قبل كل تشغيل. هذا أمر طبيعي مع تخزين S3-like.
// =============================================================================
package cinemana

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// مسارات نقاط النهاية (تُلحق بالعنوان الأساسي).
const (
	PathSearch      = "/api/android/AdvancedSearch"       // البحث: ?videoTitle=...&type=movie|series&page=N
	PathVideoInfo   = "/api/android/allVideoInfo/id/"     // تفاصيل العمل: + رقم nb
	PathVideoFiles  = "/api/android/transcoddedFiles/id/" // ملفات الفيديو بجوداتها: + رقم nb
	PathVideoSeason = "/api/android/videoSeason/id/"      // حلقات المسلسل (كل المواسم): + رقم nb المسلسل
)

// Meta بيانات وصفية عن الطلب نفسه — مفيدة لتشخيص التحويلات (302 إلى .cc مثلاً).
type Meta struct {
	Status   int    `json:"http_status"`
	FinalURL string `json:"final_url"`
}

// =============================================================================
// البنى الخام: تطابق حقول JSON كما تعيدها الخدمة حرفياً.
// الحقول كلها تقريباً نصوص حتى الأرقام (سلوك الـ API الفعلي)، والقيم null
// آمنة لأن Go يتعامل مع null كقيمة صفريّة بلا خطأ.
// =============================================================================

// Category تصنيف العمل (أكشن، دراما...).
type Category struct {
	EnTitle string `json:"en_title"`
	ArTitle string `json:"ar_title"`
}

// SearchItem عنصر نتيجة بحث. kind: "1" فيلم، "2" مسلسل.
type SearchItem struct {
	NB                   string     `json:"nb"` // المعرّف المستخدم في بقية النقاط
	ArTitle              string     `json:"ar_title"`
	EnTitle              string     `json:"en_title"`
	OtherTitle           string     `json:"other_title"`
	Stars                string     `json:"stars"` // التقييم (مثل 6.5)
	Year                 string     `json:"year"`
	Kind                 string     `json:"kind"`
	Season               string     `json:"season"`
	EpisodeNummer        string     `json:"episodeNummer"`
	ImgObjUrl            string     `json:"imgObjUrl"`            // البوستر كامل الحجم
	ImgThumbObjUrl       string     `json:"imgThumbObjUrl"`       // صورة مصغّرة
	ImgMediumThumbObjUrl string     `json:"imgMediumThumbObjUrl"` // صورة متوسطة
	ImdbURLRef           string     `json:"imdbUrlRef"`
	Trailer              string     `json:"trailer"`
	Duration             string     `json:"duration"` // بالثواني
	ObjectURLExpiration  string     `json:"objectUrlExpiration"`
	LanguageNb           string     `json:"language_lighttigerNb"` // لغة المحتوى: 7 أجنبي، 9 عربي (تفاصيل browse.go)
	ArTranslationFile    string     `json:"arTranslationFile"`     // اسم ملف ترجمة عربي (بدون مسار)
	EnTranslationFile    string     `json:"enTranslationFile"`
	Categories           []Category `json:"categories"`
}

// Translation ترجمة/ملف ترجمة ضمن حقل translations في تفاصيل العمل.
type Translation struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`      // اسم اللغة المعروض (arabic, english...)
	Type      string `json:"type"`      // رمز اللغة (ar, en...)
	Extention string `json:"extention"` // srt أو vtt (الخطأ الإملائي من الخدمة نفسها)
	File      string `json:"file"`      // رابط موقّع كامل
}

// IntroSkipping نطاق مقدمة الحلقة (لتخطيها في مشغّل المرحلة القادمة).
type IntroSkipping struct {
	Start        string `json:"start"`
	End          string `json:"end"`
	ControlLevel string `json:"control_level"`
}

// VideoInfo تفاصيل عمل كاملة (نقطة allVideoInfo/id/NB).
type VideoInfo struct {
	NB         string `json:"nb"`
	ArTitle    string `json:"ar_title"`
	EnTitle    string `json:"en_title"`
	OtherTitle string `json:"other_title"`
	Stars      string `json:"stars"`
	Year       string `json:"year"`
	Kind       string `json:"kind"`
	Season     string `json:"season"`
	Duration   string `json:"duration"`

	// الوصف بلغتين (المصدر الأساسي لوصف العمل).
	ArContent string `json:"ar_content"`
	EnContent string `json:"en_content"`

	// الصور (روابط موقعة على تخزين المزود).
	ImgObjUrl            string `json:"imgObjUrl"`
	ImgThumbObjUrl       string `json:"imgThumbObjUrl"`
	ImgMediumThumbObjUrl string `json:"imgMediumThumbObjUrl"`
	RatingImg            string `json:"ratingImg"`

	ImdbURLRef string `json:"imdbUrlRef"`
	Trailer    string `json:"trailer"`

	// ملفات الترجمة: أسماء ملفات + روابط موقعة مباشرة، إضافة إلى مصفوفة
	// translations الأشمل (تحمل srt و vtt معاً). ملاحظة: قد يكون
	// enTranslationFilePath رابطاً وهمياً (defaultImages/loading.gif) عند
	// عدم وجود ترجمة إنجليزية — يجب تصفية الامتدادات.
	ArTranslationFile     string        `json:"arTranslationFile"`
	EnTranslationFile     string        `json:"enTranslationFile"`
	ArTranslationFilePath string        `json:"arTranslationFilePath"`
	EnTranslationFilePath string        `json:"enTranslationFilePath"`
	Translations          []Translation `json:"translations"`

	HasIntroSkipping    bool            `json:"hasIntroSkipping"`
	IntroSkipping       []IntroSkipping `json:"introSkipping"`
	ObjectURLExpiration string          `json:"objectUrlExpiration"`
	Categories          []Category      `json:"categories"`
	Likes               string          `json:"Likes"`
	DisLikes            string          `json:"DisLikes"`
}

// VideoFile جودة واحدة من جودات الفيديو (نقطة transcoddedFiles/id/NB).
// container قد يكون mp4 (ملف مباشر) أو أنواع أخرى؛ وفي حال وجدنا رابط
// .m3u8 نتعامل معه كـ HLS ويُحلَّل في media.go.
type VideoFile struct {
	Name                string `json:"name"`                // مثل mp4-720
	Resolution          string `json:"resolution"`          // مثل 720p
	Container           string `json:"container"`           // مثل mp4
	TranscoddedFileName string `json:"transcoddedFileName"` // اسم الملف على التخزين
	VideoURL            string `json:"videoUrl"`            // الرابط الموقّع الكامل
}

// Episode حلقة واحدة من حلقات المسلسل (نقطة videoSeason/id/NB).
// الخدمة تعيد كل الحلقات دفعة واحدة مع رقم الموسم ورقم الحلقة، ونحن
// نجمعها في media.go.
type Episode struct {
	NB             string `json:"nb"` // معرّف الحلقة (يُستخدم مع transcoddedFiles/allVideoInfo)
	ArTitle        string `json:"ar_title"`
	EnTitle        string `json:"en_title"`
	EnContent      string `json:"en_content"`
	Season         string `json:"season"`        // رقم الموسم
	EpisodeNummer  string `json:"episodeNummer"` // رقم الحلقة داخل الموسم
	Duration       string `json:"duration"`
	RootSeries     string `json:"rootSeries"` // معرّف المسلسل الأب
	Year           string `json:"year"`
	Stars          string `json:"stars"`
	ImgThumbObjUrl string `json:"imgThumbObjUrl"`
}

// =============================================================================
// دوال النقاط (Endpoints) — كل واحدة تعيد البيانات الخام + معلومات الطلب
// =============================================================================

// getJSON ينفذ GET ويفك ترميز JSON إلى out، ويعيد Meta للاستجابة.
func getJSON(ctx context.Context, client *http.Client, opts ClientOptions, endpoint string, out any) (Meta, error) {
	resp, err := DoRequest(ctx, client, opts, http.MethodGet, endpoint, nil)
	if err != nil {
		return Meta{}, err
	}
	defer resp.Body.Close()

	meta := Meta{Status: resp.StatusCode, FinalURL: resp.Request.URL.String()}

	// نقرأ الجسم كاملاً ثم نفك الترميز حتى نستطيع عرض رسالة خطأ واضحة
	// عند إرجاع HTML بدل JSON (مثل صفحة خطأ من الشبكة المحلية).
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // سقف 32 ميغابايت احتياطاً
	if err != nil {
		return meta, err
	}
	if resp.StatusCode != http.StatusOK {
		return meta, fmt.Errorf("HTTP %d من %s: %.200s", resp.StatusCode, endpoint, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return meta, fmt.Errorf("تعذر تحليل JSON من %s: %w", resp.Request.URL.String(), err)
	}
	return meta, nil
}

// Search يبحث عن فيلم أو مسلسل بالعنوان.
// mediaType: "movie" أو "series" أو "all" (all = إرسال الطلب بدون معامل type).
func Search(ctx context.Context, client *http.Client, opts ClientOptions, title, mediaType string, page int) ([]SearchItem, Meta, error) {
	q := url.Values{}
	q.Set("videoTitle", title)
	if mediaType != "" && !strings.EqualFold(mediaType, "all") {
		q.Set("type", strings.ToLower(mediaType))
	}
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	endpoint := strings.TrimRight(opts.BaseURL, "/") + PathSearch + "?" + q.Encode()

	var items []SearchItem
	meta, err := getJSON(ctx, client, opts, endpoint, &items)
	return items, meta, err
}

// GetVideoInfo يجلب تفاصيل عمل عبر معرّفه nb.
func GetVideoInfo(ctx context.Context, client *http.Client, opts ClientOptions, nb string) (*VideoInfo, Meta, error) {
	endpoint := strings.TrimRight(opts.BaseURL, "/") + PathVideoInfo + url.PathEscape(nb)
	var info VideoInfo
	meta, err := getJSON(ctx, client, opts, endpoint, &info)
	if err != nil {
		return nil, meta, err
	}
	return &info, meta, nil
}

// GetVideoFiles يجلب روابط الفيديو المباشرة بجوداتها لمعرّف nb.
func GetVideoFiles(ctx context.Context, client *http.Client, opts ClientOptions, nb string) ([]VideoFile, Meta, error) {
	endpoint := strings.TrimRight(opts.BaseURL, "/") + PathVideoFiles + url.PathEscape(nb)
	var files []VideoFile
	meta, err := getJSON(ctx, client, opts, endpoint, &files)
	return files, meta, err
}

// GetEpisodes يجلب كل حلقات مسلسل (مع أرقام المواسم والحلقات).
func GetEpisodes(ctx context.Context, client *http.Client, opts ClientOptions, seriesNB string) ([]Episode, Meta, error) {
	endpoint := strings.TrimRight(opts.BaseURL, "/") + PathVideoSeason + url.PathEscape(seriesNB)
	var episodes []Episode
	meta, err := getJSON(ctx, client, opts, endpoint, &episodes)
	return episodes, meta, err
}
