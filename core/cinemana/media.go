// media.go
// =============================================================================
// طبقة "تطبيع" البيانات الخام إلى مخرجات JSON منظمة وسهلة الاستهلاك،
// وإضافة قدرات تحليل الوسائط:
//   - تمييز نوع الرابط: mp4 مباشر أم HLS (.m3u8).
//   - تحليل قائمة HLS الرئيسية (Master Playlist) لاستخراج مستويات الجودة
//     (الدقة، سرعة البت، الترميز) وترجمات HLS المدمجة إن وُجدت.
//   - استخراج ملفات الترجمة .srt / .vtt من حقول translations والحقول المباشرة.
//
// =============================================================================
package cinemana

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// =============================================================================
// هياكل المخرجات النهائية (ما سيطبعه السكربت للمستخدم)
// =============================================================================

// MediaSummary ملخص نتيجة بحث واحدة.
// Alts: معرّفات النسخ المكررة لنفس العمل (الخدمة تحتفظ بنسخ متعددة —
// بعضها ملفاته ميتة). الواجهة تمررها لـ GetPlayback ليجرّب البديل
// تلقائياً إذا فشل المعرف الأساسي.
type MediaSummary struct {
	ID         string   `json:"id"`
	ArTitle    string   `json:"ar_title"`
	EnTitle    string   `json:"en_title"`
	Type       string   `json:"type"` // movie / series
	Year       string   `json:"year"`
	Rating     string   `json:"rating"`
	Poster     string   `json:"poster_url"`
	Thumbnail  string   `json:"thumbnail_url"`
	Categories []string `json:"categories,omitempty"`
	Alts       []string `json:"alts,omitempty"`
}

// SubtitleTrack ترجمة واحدة جاهزة للاستخدام في المشغل.
type SubtitleTrack struct {
	Language string `json:"language"` // الاسم المعروض من الخدمة (arabic...)
	LangCode string `json:"lang_code"`
	Format   string `json:"format"` // srt / vtt
	URL      string `json:"url"`
}

// VideoQuality مستوى جودة واحد.
type VideoQuality struct {
	Resolution string `json:"resolution"`      // 240p..1080p (أو ما تعيده الخدمة)
	Width      int    `json:"width,omitempty"` // تُملأ من قائمة HLS عند توفرها
	Height     int    `json:"height,omitempty"`
	Bandwidth  int    `json:"bandwidth,omitempty"` // سرعة البت لقوائم HLS
	Codecs     string `json:"codecs,omitempty"`
	Kind       string `json:"kind"` // mp4 (ملف مباشر) أو hls (قائمة m3u8)
	URL        string `json:"url"`
}

// HLSVariant مستوى جودة داخل قائمة HLS رئيسية.
type HLSVariant struct {
	Bandwidth  int    `json:"bandwidth"`
	Resolution string `json:"resolution,omitempty"`
	Codecs     string `json:"codecs,omitempty"`
	URL        string `json:"url"`
}

// HLSInfo نتيجة تحليل قائمة HLS (إن وجد رابط m3u8 بين الروابط).
type HLSInfo struct {
	PlaylistType string          `json:"playlist_type"` // master / media / none
	Variants     []HLSVariant    `json:"variants,omitempty"`
	Segments     int             `json:"segments,omitempty"`     // لقوائم المقاطع المفردة
	DurationSec  float64         `json:"duration_sec,omitempty"` // مجموع مدد المقاطع
	Subtitles    []SubtitleTrack `json:"subtitles,omitempty"`    // ترجمات معرفة داخل القائمة
}

// VideosOutput مخرجات أمر videos كاملة.
type VideosOutput struct {
	ID        string          `json:"id"`
	Qualities []VideoQuality  `json:"qualities"`
	HLS       *HLSInfo        `json:"hls,omitempty"`
	Subtitles []SubtitleTrack `json:"subtitles"`
	Notes     []string        `json:"notes,omitempty"`
}

// DetailsOutput مخرجات أمر details كاملة (Title, Poster, Description, Year...).
type DetailsOutput struct {
	ID               string          `json:"id"`
	Title            string          `json:"title"` // العربي إن وجد وإلا الإنجليزي
	ArTitle          string          `json:"ar_title"`
	EnTitle          string          `json:"en_title"`
	Type             string          `json:"type"`
	Year             string          `json:"year"`
	Rating           string          `json:"rating"`
	PosterURL        string          `json:"poster_url"`
	ThumbnailURL     string          `json:"thumbnail_url"`
	DescriptionAr    string          `json:"description_ar"`
	DescriptionEn    string          `json:"description_en"`
	DurationSec      string          `json:"duration_sec"`
	TrailerURL       string          `json:"trailer_url,omitempty"`
	IMDBURL          string          `json:"imdb_url,omitempty"`
	Categories       []string        `json:"categories,omitempty"`
	Subtitles        []SubtitleTrack `json:"subtitles"`
	SubURLsExpireOn  string          `json:"signed_urls_expire_on,omitempty"`
	HasIntroSkipping bool            `json:"has_intro_skipping,omitempty"`
}

// Season موسم مجمّع من قائمة الحلقات.
type Season struct {
	Number   string        `json:"season"`
	Episodes []EpisodeInfo `json:"episodes"`
}

// EpisodeInfo حلقة مختصرة داخل أمر seasons.
type EpisodeInfo struct {
	ID            string `json:"id"`
	EpisodeNumber string `json:"episode_number"`
	Title         string `json:"title"`
	DurationSec   string `json:"duration_sec"`
}

// =============================================================================
// دوال التطبيع
// =============================================================================

// kindToType تحويل حقل kind الرقمي إلى اسم مفهوم: "1" فيلم، "2" مسلسل.
func kindToType(kind string) string {
	switch kind {
	case "1":
		return "movie"
	case "2":
		return "series"
	default:
		if kind != "" {
			return "kind_" + kind
		}
		return "unknown"
	}
}

// videoKind تمييز نوع رابط الفيديو من امتداده.
func videoKind(raw string) string {
	u := strings.ToLower(strings.ReplaceAll(raw, "\\", ""))
	u = strings.SplitN(u, "?", 2)[0] // نتجاهل معاملات الرابط الموقّع
	switch {
	case strings.HasSuffix(u, ".m3u8"):
		return "hls"
	case strings.HasSuffix(u, ".mp4"):
		return "mp4"
	case strings.HasSuffix(u, ".mkv"):
		return "mkv"
	default:
		return "other"
	}
}

// subtitleFormat تمييز امتداد الترجمة؛ يُرجع "" إذا لم تكن ترجمة أصلاً
// (هذا يصفّي الروابط الوهمية مثل defaultImages/loading.gif).
func subtitleFormat(raw string) string {
	u := strings.ToLower(strings.ReplaceAll(raw, "\\", ""))
	u = strings.SplitN(u, "?", 2)[0]
	switch {
	case strings.HasSuffix(u, ".srt"):
		return "srt"
	case strings.HasSuffix(u, ".vtt"):
		return "vtt"
	default:
		return ""
	}
}

// normalizeTranslations تحويل حقل translations الخام إلى قائمة ترجمات مصفّاة.
func normalizeTranslations(list []Translation) []SubtitleTrack {
	out := make([]SubtitleTrack, 0, len(list))
	for _, t := range list {
		if f := subtitleFormat(t.File); f != "" {
			out = append(out, SubtitleTrack{Language: t.Name, LangCode: t.Type, Format: f, URL: t.File})
		}
	}
	return out
}

// NormalizeSearch تحويل نتائج البحث الخام إلى ملخصات منظمة.
// النسخ المكررة (نفس العنوان + السنة + النوع) تُدمج: الأولى تُعرض،
// ومعرّفات البقية تُحفظ في Alts ليجرّبها المشغل تلقائياً عند فشل الأولى.
func NormalizeSearch(items []SearchItem) []MediaSummary {
	out := make([]MediaSummary, 0, len(items))
	byKey := map[string]int{}
	for _, it := range items {
		cats := make([]string, 0, len(it.Categories))
		for _, c := range it.Categories {
			cats = append(cats, c.EnTitle)
		}
		summary := MediaSummary{
			ID:         it.NB,
			ArTitle:    it.ArTitle,
			EnTitle:    it.EnTitle,
			Type:       kindToType(it.Kind),
			Year:       it.Year,
			Rating:     it.Stars,
			Poster:     it.ImgObjUrl,
			Thumbnail:  it.ImgThumbObjUrl,
			Categories: cats,
		}

		// مفتاح الدمج: العنوان (كبير/صغير موحّد) + السنة + النوع.
		title := strings.ToLower(strings.TrimSpace(it.EnTitle))
		if title == "" {
			title = strings.TrimSpace(it.ArTitle)
		}
		key := title + "|" + it.Year + "|" + it.Kind
		if idx, ok := byKey[key]; ok {
			out[idx].Alts = append(out[idx].Alts, it.NB)
			continue
		}
		byKey[key] = len(out)
		out = append(out, summary)
	}
	return out
}

// NormalizeDetails تحويل تفاصيل العمل الخام إلى المخرجات المنظمة.
func NormalizeDetails(v *VideoInfo) *DetailsOutput {
	title := v.ArTitle
	if strings.TrimSpace(title) == "" {
		title = v.EnTitle
	}
	cats := make([]string, 0, len(v.Categories))
	for _, c := range v.Categories {
		cats = append(cats, c.EnTitle)
	}

	// الترجمات: من translations أولاً (الأشمل)، وإن كانت فارغة نلجأ إلى
	// الحقول المباشرة arTranslationFilePath / enTranslationFilePath مع
	// تصفية الامتدادات لاستبعاد الروابط الوهمية.
	subs := normalizeTranslations(v.Translations)
	if len(subs) == 0 {
		for _, raw := range []string{v.ArTranslationFilePath, v.EnTranslationFilePath} {
			if f := subtitleFormat(raw); f != "" {
				subs = append(subs, SubtitleTrack{Format: f, URL: raw})
			}
		}
	}

	return &DetailsOutput{
		ID:               v.NB,
		Title:            title,
		ArTitle:          v.ArTitle,
		EnTitle:          v.EnTitle,
		Type:             kindToType(v.Kind),
		Year:             v.Year,
		Rating:           v.Stars,
		PosterURL:        v.ImgObjUrl,
		ThumbnailURL:     v.ImgThumbObjUrl,
		DescriptionAr:    strings.TrimSpace(v.ArContent),
		DescriptionEn:    strings.TrimSpace(v.EnContent),
		DurationSec:      v.Duration,
		TrailerURL:       v.Trailer,
		IMDBURL:          v.ImdbURLRef,
		Categories:       cats,
		Subtitles:        subs,
		SubURLsExpireOn:  v.ObjectURLExpiration,
		HasIntroSkipping: v.HasIntroSkipping,
	}
}

// NormalizeVideos تحويل قائمة الملفات الخام إلى مخرجات منظمة، مع تحليل
// أي قائمة HLS عند العثور على رابط m3u8.
func NormalizeVideos(ctx context.Context, client *http.Client, opts ClientOptions, nb string, files []VideoFile) (*VideosOutput, error) {
	out := &VideosOutput{
		ID:        nb,
		Qualities: make([]VideoQuality, 0, len(files)),
		Subtitles: []SubtitleTrack{},
	}

	for _, f := range files {
		q := VideoQuality{
			Resolution: f.Resolution,
			Kind:       videoKind(f.VideoURL),
			URL:        f.VideoURL,
		}
		if strings.EqualFold(f.Container, "mp4") && q.Kind == "other" {
			// بعض القيم القديمة قد تحذف الامتداد؛ نعتمد container كخطة بديلة.
			q.Kind = strings.ToLower(f.Container)
		}
		out.Qualities = append(out.Qualities, q)

		// إذا وجدنا قائمة HLS رئيسية نحللها لعرض مستويات الجودة الداخلية.
		if q.Kind == "hls" && out.HLS == nil {
			info, err := FetchAndParseHLS(ctx, client, opts, f.VideoURL)
			if err != nil {
				out.Notes = append(out.Notes, fmt.Sprintf("تعذر تحليل قائمة HLS: %v", err))
				continue
			}
			out.HLS = info
			// ندمج مستويات الجودة من القائمة داخل qualities أيضاً لعرض موحّد.
			for _, v := range info.Variants {
				w, h := parseResolution(v.Resolution)
				out.Qualities = append(out.Qualities, VideoQuality{
					Resolution: v.Resolution,
					Width:      w,
					Height:     h,
					Bandwidth:  v.Bandwidth,
					Codecs:     v.Codecs,
					Kind:       "hls-variant",
					URL:        v.URL,
				})
			}
			if len(info.Subtitles) > 0 {
				out.Subtitles = append(out.Subtitles, info.Subtitles...)
			}
		}
	}

	// ترتيب الجودات تنازلياً: حسب الارتفاع أولاً (1080p قبل 720p) ثم
	// سرعة البت لقوائم HLS التي لا تحمل دقة رقمية.
	sort.SliceStable(out.Qualities, func(i, j int) bool {
		hi, bwi := resRank(out.Qualities[i].Resolution, out.Qualities[i].Bandwidth)
		hj, bwj := resRank(out.Qualities[j].Resolution, out.Qualities[j].Bandwidth)
		if hi != hj {
			return hi > hj
		}
		return bwi > bwj
	})
	return out, nil
}

// resRank إعطاء ترتيب رقمي للدقة "720p" أو سرعة بت بديلة لقوائم HLS.
func resRank(res string, bandwidth int) (height, bw int) {
	h := strings.TrimSuffix(strings.ToUpper(res), "P")
	if n, err := strconv.Atoi(h); err == nil {
		height = n
	}
	return height, bandwidth
}

// parseResolution تحويل "1920x1080" إلى عرض وارتفاع رقميين.
func parseResolution(res string) (int, int) {
	wStr, hStr, ok := strings.Cut(res, "x")
	if !ok {
		return 0, 0
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(wStr))
	h, err2 := strconv.Atoi(strings.TrimSpace(hStr))
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return w, h
}

// NormalizeEpisodes تجميع قائمة الحلقات الخام في مواسم مرتبة.
func NormalizeEpisodes(episodes []Episode) []Season {
	seasonMap := map[string][]EpisodeInfo{}
	for _, e := range episodes {
		title := e.EnTitle
		if strings.TrimSpace(title) == "" {
			title = e.ArTitle
		}
		seasonMap[e.Season] = append(seasonMap[e.Season], EpisodeInfo{
			ID:            e.NB,
			EpisodeNumber: e.EpisodeNummer,
			Title:         title,
			DurationSec:   e.Duration,
		})
	}
	seasons := make([]Season, 0, len(seasonMap))
	for num, eps := range seasonMap {
		sort.SliceStable(eps, func(i, j int) bool {
			return episodeNum(eps[i].EpisodeNumber) < episodeNum(eps[j].EpisodeNumber)
		})
		seasons = append(seasons, Season{Number: num, Episodes: eps})
	}
	sort.SliceStable(seasons, func(i, j int) bool {
		return episodeNum(seasons[i].Number) < episodeNum(seasons[j].Number)
	})
	return seasons
}

// episodeNum تحويل آمن لرقم الحلقة/الموسم النصي إلى عدد للترتيب.
func episodeNum(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// =============================================================================
// تحليل قوائم HLS
// =============================================================================

// FetchAndParseHLS يجلب قائمة m3u8 ويحللها.
// القائمة قد تكون "رئيسية" (تحوي مستويات جودة #EXT-X-STREAM-INF) أو
// "مقاطع" (تحوي #EXTINF فقط = جودة واحدة)؛ نتعامل مع الحالتين.
func FetchAndParseHLS(ctx context.Context, client *http.Client, opts ClientOptions, playlistURL string) (*HLSInfo, error) {
	// للقوائم نطلب */* لأنها ليست JSON.
	resp, err := DoRequest(ctx, client, opts, "GET", playlistURL, map[string]string{"Accept": "*/*"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d أثناء جلب قائمة m3u8", resp.StatusCode)
	}
	// قوائم HLS نصية وصغيرة عادة؛ سقف 2 ميغابايت احتياطاً.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	return ParseM3U8(string(body), resp.Request.URL), nil
}

// ParseM3U8 محلل نصي بسيط لقوائم HLS (بدون مكتبات خارجية عمداً ليسهل صيانته).
// base هو رابط القائمة نفسه لحل الروابط النسبية داخلها.
func ParseM3U8(content string, base *url.URL) *HLSInfo {
	info := &HLSInfo{PlaylistType: "media"}

	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		switch {
		case line == "":
			continue

		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			// سطر يصف مستوى جودة؛ الرابط يأتي في السطر التالي غير التعليقي.
			attrs := parseHLSAttributes(strings.TrimPrefix(line, "#EXT-X-STREAM-INF:"))
			info.Variants = append(info.Variants, HLSVariant{
				Bandwidth:  atoiSafe(attrs["BANDWIDTH"]),
				Resolution: attrs["RESOLUTION"],
				Codecs:     strings.Trim(attrs["CODECS"], `"`),
			})
			info.PlaylistType = "master"

		case strings.HasPrefix(line, "#EXT-X-MEDIA:"):
			// مسارات بديلة داخل القائمة: نستخرج الترجمات فقط (TYPE=SUBTITLES).
			attrs := parseHLSAttributes(strings.TrimPrefix(line, "#EXT-X-MEDIA:"))
			if strings.EqualFold(attrs["TYPE"], "SUBTITLES") && attrs["URI"] != "" {
				info.Subtitles = append(info.Subtitles, SubtitleTrack{
					Language: attrs["NAME"],
					LangCode: attrs["LANGUAGE"],
					Format:   subtitleFormat(attrs["URI"]),
					URL:      resolveAgainst(base, attrs["URI"]),
				})
			}

		case strings.HasPrefix(line, "#EXTINF:"):
			// مقطع في قائمة media: نجمع مدته لعرض المجموع.
			if d, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ","), 64); err == nil {
				info.DurationSec += d
				info.Segments++
			}

		case !strings.HasPrefix(line, "#"):
			// سطر غير تعليقي = رابط. إذا كنا ننتظر رابط مستوى جودة
			// (بعد #EXT-X-STREAM-INF) نربطه بآخر عنصر تمت إضافته.
			if info.PlaylistType == "master" && len(info.Variants) > 0 && info.Variants[len(info.Variants)-1].URL == "" {
				info.Variants[len(info.Variants)-1].URL = resolveAgainst(base, line)
			}
		}
	}

	if info.PlaylistType == "media" && info.Segments == 0 && len(info.Variants) == 0 {
		info.PlaylistType = "none"
	}
	return info
}

// parseHLSAttributes تحليل سطر مثل: BANDWIDTH=800000,RESOLUTION=1280x720,"CODECS"="avc1"
// يدعم القيم المحاطة بعلامات اقتباس والتي تحتوي فواصل.
func parseHLSAttributes(s string) map[string]string {
	out := map[string]string{}
	var key, val strings.Builder
	inQuotes, inVal := false, false
	flush := func() {
		if key.Len() > 0 {
			out[strings.TrimSpace(strings.ToUpper(key.String()))] = strings.TrimSpace(val.String())
		}
		key.Reset()
		val.Reset()
		inVal = false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inQuotes = !inQuotes
		case c == '=' && !inVal:
			inVal = true
		case c == ',' && !inQuotes:
			flush()
		default:
			if inVal {
				val.WriteByte(c)
			} else {
				key.WriteByte(c)
			}
		}
	}
	flush()
	return out
}

// resolveAgainst يحل رابطاً نسبياً ضد رابط القائمة وفق RFC 3986:
// المسارات النسبية تُلحق بمجلد القائمة، ومعاملات استعلام القائمة نفسها
// لا تورّث للروابط الداخلية (نفس سلوك المشغلات القياسية مثل hls.js).
func resolveAgainst(base *url.URL, ref string) string {
	if base == nil || ref == "" {
		return ref
	}
	u, err := url.Parse(strings.ReplaceAll(ref, "\\", ""))
	if err != nil {
		return ref
	}
	return base.ResolveReference(u).String()
}

// atoiSafe تحويل صامت آمن (0 عند الفشل).
func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
