// fixes_test.go
// =============================================================================
// اختبارات انحدار (بلا شبكة حقيقية) لإصلاحات المراجعة: التحقق من المدخلات،
// تحليل HLS، الدمج، الذاكرة المؤقتة، وحماية العميل من التحويلات الخطرة.
// =============================================================================
package cinemana

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestNormalizeBaseURL(t *testing.T) {
	ok := map[string]string{
		"cinemana.shabakaty.com":             "https://cinemana.shabakaty.com",
		"http://10.0.0.1:8080/path?x=1":      "http://10.0.0.1:8080",
		"https://user:pw@host.example/a/b#f": "https://host.example",
		"  https://host.example  ":           "https://host.example",
	}
	for in, want := range ok {
		got, err := NormalizeBaseURL(in)
		if err != nil || got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, %v؛ نريد %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"ftp://host", "file:///etc/passwd", "https://", "javascript://x"} {
		if got, err := NormalizeBaseURL(bad); err == nil {
			t.Errorf("NormalizeBaseURL(%q) يجب أن يفشل، لكنه أعاد %q", bad, got)
		}
	}
}

func TestCheckID(t *testing.T) {
	for _, bad := range []string{"", "..", "12/../3", "12a", "1 2", "١٢"} {
		if _, err := checkID(bad); !errors.Is(err, ErrInvalidID) {
			t.Errorf("checkID(%q) يجب أن يرفض", bad)
		}
	}
	if got, err := checkID(" 5386 "); err != nil || got != "5386" {
		t.Errorf("checkID صالح: %q %v", got, err)
	}
}

func TestParseM3U8ExtinfWithTitle(t *testing.T) {
	base, _ := url.Parse("https://cdn.example.com/v/720.m3u8")
	info := ParseM3U8("#EXTM3U\n#EXTINF:10.5,Intro\nseg1.ts\n#EXTINF:9.5,\nseg2.ts\n", base)
	if info.PlaylistType != "media" || info.Segments != 2 || info.DurationSec != 20 {
		t.Errorf("EXTINF بعنوان لم يُحلَّل: %+v", info)
	}
}

func TestQualityRankingUsesHeight(t *testing.T) {
	qs := []VideoQuality{
		{Resolution: "480p"},
		{Resolution: "1920x1080", Height: 1080, Bandwidth: 4000000},
		{Resolution: "720p"},
	}
	if h, _ := resRank(qs[1]); h != 1080 {
		t.Errorf("ارتفاع قائمة HLS يجب أن يُستخدم في الترتيب، حصلنا %d", h)
	}
	if h, _ := resRank(qs[2]); h != 720 {
		t.Errorf("720p → %d", h)
	}
}

func TestNormalizeSearchMerging(t *testing.T) {
	items := []SearchItem{
		{NB: "1", EnTitle: "Dune", Year: "2021", Kind: "1"},
		{NB: "2", EnTitle: "dune ", Year: "2021", Kind: "1"}, // نسخة مكررة
		{NB: "2", EnTitle: "dune", Year: "2021", Kind: "1"},  // نفس المعرف: لا يُضاف لـ Alts
		{NB: "3", Year: "2020", Kind: "1"},                   // بلا عنوان: مستقل
		{NB: "4", Year: "2020", Kind: "1"},                   // بلا عنوان: مستقل
	}
	out := NormalizeSearch(items)
	if len(out) != 3 {
		t.Fatalf("عدد الملخصات = %d، نريد 3: %+v", len(out), out)
	}
	if len(out[0].Alts) != 1 || out[0].Alts[0] != "2" {
		t.Errorf("Alts = %v، نريد [2]", out[0].Alts)
	}
}

func TestDedupeDoesNotMutateInput(t *testing.T) {
	in := []SearchItem{{NB: "1"}, {NB: "1"}, {NB: "2"}, {NB: ""}, {NB: ""}}
	out := dedupe(in)
	if len(out) != 4 {
		t.Errorf("dedupe يجب أن يبقي العناصر بلا معرّف: %d", len(out))
	}
	if in[1].NB != "1" || in[2].NB != "2" {
		t.Errorf("dedupe عدّل الشريحة الأصلية: %+v", in)
	}
}

func TestBrowseRejectsInjectedFilters(t *testing.T) {
	cases := []BrowseFilters{
		{LanguageID: "7&level=0"},
		{CategoryID: "57#"},
		{VideoKind: "3"},
		{VideoKind: "1 or 1"},
	}
	for _, f := range cases {
		_, err := Browse(context.Background(), http.DefaultClient, DefaultOptions(), f, 10)
		var fe *FilterError
		if !errors.As(err, &fe) {
			t.Errorf("Browse(%+v) يجب أن يعيد FilterError، حصلنا %v", f, err)
		}
	}
}

func TestGetCollectionUnknownKey(t *testing.T) {
	_, err := GetCollection(context.Background(), http.DefaultClient, DefaultOptions(), "nope", 5)
	if !errors.Is(err, ErrUnknownCollection) {
		t.Errorf("نريد ErrUnknownCollection، حصلنا %v", err)
	}
}

func TestFlexiblePage(t *testing.T) {
	var p flexiblePage
	if err := p.UnmarshalJSON([]byte(` [{"nb":"1"}] `)); err != nil || len(p.Items) != 1 {
		t.Errorf("مصفوفة: %v %+v", err, p)
	}
	p = flexiblePage{}
	if err := p.UnmarshalJSON([]byte(`{"info":[{"nb":"1"},{"nb":"2"}],"offset":2}`)); err != nil || len(p.Items) != 2 {
		t.Errorf("كائن: %v %+v", err, p)
	}
	if err := (&flexiblePage{}).UnmarshalJSON([]byte(`[{"nb":{}}]`)); err == nil {
		t.Errorf("خطأ نوع داخل المصفوفة يجب أن يظهر كما هو")
	}
}

// TestPageCacheCoalesces طلبات متزامنة على نفس الصفحة = طلب شبكي واحد.
func TestPageCacheCoalesces(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte(`[{"nb":"1"}]`))
	}))
	defer srv.Close()
	opts := DefaultOptions()
	opts.BaseURL = srv.URL
	endpoint := srv.URL + "/api/android/latestMovies/test-cache"

	done := make(chan struct{})
	for i := 0; i < 5; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			if items, err := fetchJSONPage(context.Background(), srv.Client(), opts, endpoint); err != nil || len(items) != 1 {
				t.Errorf("fetchJSONPage: %v %v", items, err)
			}
		}()
	}
	for i := 0; i < 5; i++ {
		<-done
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("عدد الطلبات الشبكية = %d، نريد 1", n)
	}
}

func TestDoRequestRejectsBadScheme(t *testing.T) {
	for _, u := range []string{"file:///etc/passwd", "ftp://x/y", "http://", "javascript:alert(1)"} {
		if _, err := DoRequest(context.Background(), http.DefaultClient, DefaultOptions(), http.MethodGet, u, nil); err == nil {
			t.Errorf("DoRequest(%q) يجب أن يرفض", u)
		}
	}
}

func TestGetJSONHTMLSnippet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><body>Please log in to the ISP portal</body></html>"))
	}))
	defer srv.Close()
	opts := DefaultOptions()
	var out []SearchItem
	_, err := getJSON(context.Background(), srv.Client(), opts, srv.URL, &out)
	if err == nil || !contains(err.Error(), "ISP portal") {
		t.Errorf("الخطأ يجب أن يتضمن مقتطفاً من الصفحة: %v", err)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestRedirectDowngradeRejected(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("[]"))
	}))
	defer plain.Close()
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	defer tlsSrv.Close()

	opts := DefaultOptions()
	opts.InsecureTLS = true // الخادم التجريبي بشهادة موقّعة ذاتياً
	client, _, err := BuildClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	if resp, err := client.Get(tlsSrv.URL); err == nil {
		resp.Body.Close()
		t.Error("تحويل https→http كان يجب أن يُرفض")
	}
}

func TestParseResolveFlagsAndDNSServerIPv6(t *testing.T) {
	opts := DefaultOptions()
	opts.DNSServer = "::1"
	if _, r, err := BuildClient(opts); err != nil || r == nil {
		t.Errorf("BuildClient مع DNS IPv6: %v", err)
	}
}
