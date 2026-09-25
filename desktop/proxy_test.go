// proxy_test.go
// =============================================================================
// اختبار نهاية-لنهاية لوكيل الترجمة: يشغّل Streamer فعلياً، يجلب رابط
// ترجمة حقيقي من الخدمة، يطلبه عبر /sub، ويتحقق أن الناتج VTT صالح
// وعربي مقروء. يتطلب اتصالاً بشبكة الخدمة (تُتخطى الاختبارات بلا شبكة).
// =============================================================================
package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	cinemana "cinemana-probe/core/cinemana"
)

// TestSubtitleThroughProxy اختبار تحويل SRT→VTT عبر الوكيل المحلي.
func TestSubtitleThroughProxy(t *testing.T) {
	if os.Getenv("CINEMANA_LIVE") == "" {
		t.Skip("اختبار شبكي — شغّله بـ CINEMANA_LIVE=1")
	}

	opts := cinemana.DefaultOptions()
	client, _, err := cinemana.BuildClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// جلب فيلم معروف له ترجمة عربية (5386 = Fast & Furious).
	info, _, err := cinemana.GetVideoInfo(ctx, client, opts, "5386")
	if err != nil {
		t.Skip("لا وصول للخدمة:", err)
	}
	details := cinemana.NormalizeDetails(info)
	if len(details.Subtitles) == 0 {
		t.Fatal("لا توجد ترجمات في تفاصيل الفيلم التجريبي")
	}

	// تشغيل الوكيل وطلب الترجمة عبره.
	s, err := NewStreamer(ctx, opts, client)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	subURL := s.SubtitleURL(details.Subtitles[0].URL)
	resp, err := client.Get(subURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != 200 {
		t.Fatalf("HTTP %d من /sub", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/vtt") {
		t.Errorf("Content-Type = %s، نريد text/vtt", ct)
	}
	if !strings.HasPrefix(string(body), "WEBVTT") {
		t.Errorf("الناتج ليس VTT صالحاً: %.60s", body)
	}
	if !strings.Contains(string(body), "-->") {
		t.Errorf("لا توجد توقيتات في الناتج: %.120s", body)
	}
	t.Logf("VTT سليم (%d بايت) عبر %s", len(body), subURL[:60]+"...")
}

// TestSRTToVTTConversion اختبار تحويل عينة SRT (عربي + ثوانٍ ناقصة + وسوم ASS).
func TestSRTToVTTConversion(t *testing.T) {
	srt := "1\r\n00:00:01,500 --> 00:00:03,250\r\nمرحبا بالعالم\r\n\r\n2\r\n0:00:04,5 --> 0:00:06\r\nوما بدأ في 11 سبتمبر {\\an8}\r\n"
	out := string(srtToVTT([]byte(srt)))
	if !strings.HasPrefix(out, "WEBVTT") {
		t.Errorf("رأس WEBVTT مفقود: %.40s", out)
	}
	if !strings.Contains(out, "00:00:01.500 --> 00:00:03.250") {
		t.Errorf("التوقيت الأول لم يُحول: %s", out)
	}
	if !strings.Contains(out, "00:00:04.500 --> 00:00:06.000") {
		t.Errorf("التوقيت الثاني (ساعات/أجزاء ناقصة) لم يُوحّد: %s", out)
	}
	if !strings.Contains(out, "مرحبا بالعالم") {
		t.Errorf("النص العربي مفقود")
	}
	if strings.Contains(out, `{\an8}`) {
		t.Errorf("وسوم ASS يجب أن تُزال")
	}
	if !strings.Contains(out, "11 سبتمبر") {
		t.Errorf("النص بعد الوسم مفقود")
	}
	// الفاصلة العشرية يجب أن تصبح نقطة في التوقيتات.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "-->") && strings.Contains(line, ",") {
			t.Errorf("فاصلة في التوقيت: %s", line)
		}
	}
}

// TestStreamRangePassthrough تأكيد تمرير رأس Range عبر الوكيل (شبكي).
func TestStreamRangePassthrough(t *testing.T) {
	if os.Getenv("CINEMANA_LIVE") == "" {
		t.Skip("اختبار شبكي — شغّله بـ CINEMANA_LIVE=1")
	}
	opts := cinemana.DefaultOptions()
	client, _, err := cinemana.BuildClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	files, _, err := cinemana.GetVideoFiles(ctx, client, opts, "5386")
	if err != nil || len(files) == 0 {
		t.Skip("لا وصول للخدمة:", err)
	}

	s, err := NewStreamer(ctx, opts, client)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	req, _ := http.NewRequest("GET", s.StreamURL(files[0].VideoURL), nil)
	req.Header.Set("Range", "bytes=0-1023")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// ملاحظة بيئية مؤكدة: عقود فيديو المزود (cdn/cndw*.shabakaty.cc)
	// تعيد 503 من خارج شبكة المزود — لا يمكن التحقق الحي منها من هنا.
	if resp.StatusCode >= 500 {
		t.Skipf("عقد CDN غير متاح من هذه الشبكة (HTTP %d) — طبيعي خارج شبكة المزود", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 206 && resp.StatusCode != 200 {
		t.Fatalf("HTTP %d — نتوقع 206 Partials", resp.StatusCode)
	}
	if len(body) == 0 || len(body) > 4096 {
		t.Errorf("حجم الجسم غير منطقي لنطاق 1KB: %d", len(body))
	}
}
