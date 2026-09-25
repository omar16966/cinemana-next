// media_test.go
// =============================================================================
// اختبارات وحدة سريعة للدوال الحساسة (تعمل بلا شبكة: go test ./...)
// الهدف: حماية محلل HLS ووظائف التمييز من أي تعديل مستقبلي يكسرها.
// =============================================================================
package cinemana

import (
	"net/url"
	"testing"
)

// TestParseM3U8Master قائمة رئيسية بمستويي جودة + ترجمة مدمجة.
func TestParseM3U8Master(t *testing.T) {
	base, _ := url.Parse("https://cdn.example.com/live/stream.m3u8?sig=abc")
	content := `#EXTM3U
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="arabic",LANGUAGE="ar",URI="subs/ar.vtt"
#EXT-X-STREAM-INF:BANDWIDTH=4000000,RESOLUTION=1920x1080,CODECS="avc1.640028,mp4a.40.2"
1080/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1000000,RESOLUTION=1280x720,CODECS="avc1.64001f"
720/index.m3u8
`
	info := ParseM3U8(content, base)
	if info.PlaylistType != "master" {
		t.Fatalf("PlaylistType = %s, نريد master", info.PlaylistType)
	}
	if len(info.Variants) != 2 {
		t.Fatalf("عدد المستويات = %d، نريده 2", len(info.Variants))
	}
	v := info.Variants[0]
	if v.Bandwidth != 4000000 || v.Resolution != "1920x1080" || v.Codecs != "avc1.640028,mp4a.40.2" {
		t.Errorf("بيانات المستوى الأول غير صحيحة: %+v", v)
	}
	// الرابط النسبي يُحل ضد رابط القائمة وفق معيار RFC 3986: المسار النسبي
	// يُلحق بمجلد القائمة، لكن معاملات التوقيع في رابط القائمة نفسها لا
	// تورّث تلقائياً للروابط الداخلية (سلوك المعيار نفسه الذي تتبعه
	// المشغلات مثل hls.js).
	if v.URL != "https://cdn.example.com/live/1080/index.m3u8" {
		t.Errorf("رابط المستوى لم يُحل بشكل صحيح: %s", v.URL)
	}
	if len(info.Subtitles) != 1 || info.Subtitles[0].Language != "arabic" || info.Subtitles[0].Format != "vtt" {
		t.Errorf("ترجمات القائمة غير صحيحة: %+v", info.Subtitles)
	}
}

// TestParseM3U8Media قائمة مقاطع بجودة واحدة (ليست رئيسية).
func TestParseM3U8Media(t *testing.T) {
	base, _ := url.Parse("https://cdn.example.com/v/720.m3u8")
	content := `#EXTM3U
#EXTINF:10.5,
seg1.ts
#EXTINF:9.5,
seg2.ts
`
	info := ParseM3U8(content, base)
	if info.PlaylistType != "media" || info.Segments != 2 || info.DurationSec != 20 {
		t.Errorf("تحليل قائمة media خاطئ: %+v", info)
	}
	if len(info.Variants) != 0 {
		t.Errorf("قائمة media لا يجب أن تنتج مستويات")
	}
}

// TestSubtitleFormat تمييز الترجمات الحقيقية من الروابط الوهمية.
func TestSubtitleFormat(t *testing.T) {
	cases := map[string]string{
		"https://x/y_ar_transfile.srt?Expires=1&Signature=a%2Bb": "srt",
		"https://x/y.vtt":           "vtt",
		"defaultImages/loading.gif": "", // رابط وهمي — يجب استبعاده
		"https://x/video.mp4":       "",
	}
	for in, want := range cases {
		if got := subtitleFormat(in); got != want {
			t.Errorf("subtitleFormat(%q) = %q، نريد %q", in, got, want)
		}
	}
}

// TestVideoKind تمييز نوع روابط الفيديو حتى مع توقيعات الرابط.
func TestVideoKind(t *testing.T) {
	cases := map[string]string{
		"https://cdn/x/video.mp4?Signature=a": "mp4",
		"https://cdn/x/index.m3u8?token=1":    "hls",
		"https://cdn/x/file.mkv":              "mkv",
		"https://cdn/x/unknown":               "other",
	}
	for in, want := range cases {
		if got := videoKind(in); got != want {
			t.Errorf("videoKind(%q) = %q، نريد %q", in, got, want)
		}
	}
}

// TestParseHLSAttributes قيم مقتبسة تحتوي فواصل (CODECS مثلاً).
func TestParseHLSAttributes(t *testing.T) {
	attrs := parseHLSAttributes(`BANDWIDTH=4000000,RESOLUTION=1920x1080,CODECS="avc1.640028,mp4a.40.2",FRAME-RATE=30`)
	if attrs["BANDWIDTH"] != "4000000" || attrs["RESOLUTION"] != "1920x1080" {
		t.Errorf("تحليل خاطئ: %+v", attrs)
	}
	if attrs["CODECS"] != "avc1.640028,mp4a.40.2" {
		t.Errorf("CODECS داخل الاقتباس يجب أن يبقى كاملاً: %q", attrs["CODECS"])
	}
	if attrs["FRAME-RATE"] != "30" {
		t.Errorf("FRAME-RATE: %q", attrs["FRAME-RATE"])
	}
}
