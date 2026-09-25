// client.go
// =============================================================================
// بناء عميل HTTP مخصص لاختبار خدمة "سينمانا" (Shabakaty / Earthlink).
//
// لماذا عميل مخصص بدل http.DefaultClient؟
//  1. تجاوز البروكسي (System Proxy): المتصفح أو النظام قد يوجّه الطلبات عبر
//     بروكسي خارجي/VPN فيفقد الاتصال المباشر بشبكة مزود الخدمة. نضع
//     Proxy: nil لتجاهل أي إعداد بروكسي (متغيرات البيئة أو إعدادات Windows).
//  2. DNS محلي (Plain DNS لا DoH): المتصفحات تستخدم "Secure DNS / DoH"
//     الخارجي (مثل 8.8.8.8 عبر HTTPS) وقد يفشل في حل نطاقات مزود الخدمة
//     أو يعيد عناوين غير مناسبة للشبكة المحلية. نستخدم هنا محلل DNS
//     المدمج في Go (PreferGo) الذي يقرأ خوادم DNS من إعدادات النظام/الراوتر
//     ويستعلمها مباشرة عبر البورت 53 بدون تشفير خارجي، مع إمكانية تحديد
//     خادم DNS يدوياً (--dns) أو تثبيت نطاق على IP مباشرة (--resolve).
//  3. ترويسات تحاكي تطبيق الأندرويد الرسمي: بعض نقاط النهاية (مسار
//     /api/android/) قد تتصرف بشكل مختلف حسب User-Agent، فنرسل ترويسة
//     Dalvik واقعية لجهاز أندرويد مع ترويسات Accept و Connection القياسية.
//
// =============================================================================
package cinemana

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// ClientOptions كل ما يمكن ضبطه في العميل من سطر الأوامر.
type ClientOptions struct {
	BaseURL     string            // العنوان الأساسي للخدمة (نتبع أي إعادة توجيه تلقائياً)
	Timeout     time.Duration     // مهلة كاملة للطلب الواحد
	DNSServer   string            // خادم DNS محلي اختياري مثل 10.0.0.1 أو 10.0.0.1:53 (فارغ = خوادم النظام)
	Resolves    map[string]string // تثبيت نطاق على IP مثل {"cdn.shabakaty.cc": "10.0.0.5"} (مثل --resolve في curl)
	UserAgent   string            // ترويسة User-Agent التي تحاكي تطبيق الأندرويد
	AppID       string            // قيمة X-Requested-With الخاصة بحزمة التطبيق (فارغة = لا تُرسل)
	InsecureTLS bool              // تخطي التحقق من شهادة TLS (للاختبار داخل الشبكة المحلية فقط)
}

// DefaultOptions القيم الافتراضية المعتمدة من ملاحظة سلوك تطبيق الأندرويد.
//
// ملاحظة مهمة اكتُشفت أثناء الفحص: النطاق cinemana.shabakaty.com يعيد
// توجيه 302 إلى cinemana.shabakaty.cc — نحتفظ بـ .com كعنوان أساسي ونترك
// العميل يتبع التحويل تلقائياً، تماماً كما يفعل التطبيق.
func DefaultOptions() ClientOptions {
	return ClientOptions{
		BaseURL: "https://cinemana.shabakaty.com",
		Timeout: 20 * time.Second,
		// UA واقعي لجهاز أندرويد حقيقي: التطبيقات الأصلية المبنية بـ OkHttp/Dalvik
		// ترسل هذا الشكل من ترويسة User-Agent افتراضياً.
		UserAgent: "Dalvik/2.1.0 (Linux; U; Android 13; SM-A536B Build/TP1A.220624.014)",
		// اسم حزمة تطبيق سينمانا/إيرثلينك على جوجل بلاي (com.erthlink.tv).
		// التطبيقات المبنية على WebView ترسلها في X-Requested-With، وإرسالها
		// لا يضر التطبيقات الأصلية ويزيد تشابه الطلب مع الأندرويد.
		AppID: "com.erthlink.tv",
	}
}

// BuildClient يبني http.Client جاهزاً حسب الخيارات أعلاه، ويعيد أيضاً
// الـ resolver لاستخدامه في أمر probe (عرض عناوين IP التي تم حلها).
func BuildClient(opts ClientOptions) (*http.Client, *net.Resolver, error) {
	// ---------------------------------------------------------------------------
	// 1) محلل DNS المحلي
	// ---------------------------------------------------------------------------
	// الافتراضي: محلل نظام التشغيل (net.DefaultResolver). على ويندوز يعني
	// ذلك خدمة DNS Client مع ذاكرة التخزين المؤقتة وسلوك التراجع بين
	// الخوادم — نفس سلوك أي تطبيق سطح مكتب. (ملاحظة صيانة: تجنبنا
	// PreferGo:true افتراضياً لأن المحلل "النقي" في Go يرسل استعلامي
	// A وAAAA معاً، وبعض راوترات مزودي الخدمة تجيب NXDOMAIN لهذا
	// النمط فتفشل عملية الحل رغم أن النطاق موجود — ملاحظة أكدتها
	// الاختبارات الفعلية. كذلك Go لا يستخدم DoH إطلاقاً، فالاتصال
	// DNS محلي عادي عبر البورت 53 في الحالتين.)
	//
	// عند تحديد --dns نستخدم المحلل النقي (PreferGo) لإجبار كل
	// الاستعلامات على الذهاب إلى خادم محدد — مفيد عندما يكون نطاق
	// الخدمة محسوماً فقط على DNS مزود الخدمة.
	var resolver *net.Resolver = net.DefaultResolver
	if opts.DNSServer != "" {
		// إذا حدّد المستخدم خادم DNS محلياً نعيد توجيه كل استعلامات DNS إليه.
		server := opts.DNSServer
		if !strings.Contains(server, ":") {
			server += ":53" // البورت القياسي لـ DNS إن لم يُحدد
		}
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				// المعامل address هنا سيكون خادم DNS من إعدادات النظام؛
				// نتجاهله ونستخدم الخادم المحدد من المستخدم بدلاً منه.
				var d net.Dialer
				d.Timeout = 5 * time.Second
				return d.DialContext(ctx, network, server)
			},
		}
	}

	// ---------------------------------------------------------------------------
	// 2) المكوّن الاتصالي (Dialer + Transport)
	// ---------------------------------------------------------------------------
	dialer := &net.Dialer{
		Timeout:   opts.Timeout,
		KeepAlive: 30 * time.Second,
		Resolver:  resolver,
	}

	transport := &http.Transport{
		// *** تجاوز البروكسي تماماً ***
		// nil تعني: لا تستخدم أي بروكسي، وتجاهل HTTP_PROXY/HTTPS_PROXY
		// وإعدادات بروكسي Windows. هذا يضمن اتصالاً مباشراً بالشبكة المحلية.
		Proxy: nil,

		// نقطة الاتصال المخصصة: نتحقق أولاً من جدول التثبيت (--resolve)
		// فإذا كان للنطاق IP مثبت نتصل به مباشرة مع الحفاظ على اسم النطاق
		// في طبقة TLS (SNI) وإلا لَفشل التحقق من الشهادة.
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				host, port = addr, "80"
			}
			if ip, ok := opts.Resolves[strings.ToLower(host)]; ok {
				addr = net.JoinHostPort(ip, port)
			}
			return dialer.DialContext(ctx, network, addr)
		},

		ForceAttemptHTTP2:     true, // الدخول للمحتوى عبر HTTP/2 عند توفره (كما في تطبيقات الأندرويد الحديثة)
		MaxIdleConns:          20,   // تجمع اتصالات لإعادة الاستخدام (أسرع في الأوامر المتسلسلة)
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig: &tls.Config{
			// خيار للاختبار فقط: بعض الخوادم الداخلية لشبكة المزود تستخدم
			// شهادات موقعة داخلياً لا يثق بها النظام.
			InsecureSkipVerify: opts.InsecureTLS,
		},
	}

	// وعاء كوكيز في الذاكرة: بعض نقاط النهاية تضبط كوكيز جلسة أثناء
	// إعادة التوجيه (من .com إلى .cc)؛ الاحتفاظ بها يزيد التوافق.
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, err
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   opts.Timeout,
		Jar:       jar,
		// نستخدم سياسة التوجيه الافتراضية: تتبع حتى 10 قفزات (302...)،
		// وهذا مطلوب لأن النطاق الأساسي يحوّل إلى shabakaty.cc.
	}
	return client, resolver, nil
}

// DoRequest ينفذ طلب HTTP مع الترويسات التي تحاكي تطبيق الأندرويد الرسمي.
// extra تسمح بإضافة/تعديل ترويسات لحالة معينة (مثل Accept مختلف لملف m3u8).
func DoRequest(ctx context.Context, client *http.Client, opts ClientOptions, method, rawURL string, extra map[string]string) (*http.Response, error) {
	// تنظيف الروابط: بعض حقول الـ JSON تحتوي شرطات مائلة معكوسة "\"
	// (سلوك ملاحظ في التطبيق المرجعي) فنزيلها قبل التحليل.
	rawURL = strings.ReplaceAll(rawURL, "\\", "")

	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}

	// --- الترويسات القياسية التي يرسلها تطبيق الأندرويد ---
	req.Header.Set("User-Agent", opts.UserAgent)
	// Accept: التطبيق يطلب JSON من نقاط api/، مع قبول أي شيء آخر احتياطاً.
	req.Header.Set("Accept", "application/json, text/plain, */*")
	// Connection: HTTP/1.1 الافتراضي هو keep-alive وGo يدير تجمع الاتصالات
	// بنفسه، لكن نرسلها صراحةً لمطابقة سلوك OkHttp في التطبيقات الأصلية.
	req.Header.Set("Connection", "keep-alive")
	if opts.AppID != "" {
		req.Header.Set("X-Requested-With", opts.AppID)
	}

	// ملاحظة صيانة: لا نضبط Accept-Encoding يدوياً. مكتبة Go تضيف "gzip"
	// تلقائياً وتفك الضغط بشفافية؛ لو ضبطناها يدوياً لكان علينا فك
	// الضغط بأنفسنا في كل استجابة.

	for k, v := range extra {
		if v == "" {
			req.Header.Del(k) // تمرير قيمة فارغة = حذف الترويسة الافتراضية
		} else {
			req.Header.Set(k, v)
		}
	}
	return client.Do(req)
}

// ParseResolveFlags تحويل قيم --resolve الممررة كـ "host=ip,host2=ip2" إلى خريطة.
func ParseResolveFlags(values []string) (map[string]string, error) {
	out := make(map[string]string)
	for _, chunk := range values {
		for _, pair := range strings.Split(chunk, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			host, ip, found := strings.Cut(pair, "=")
			if !found {
				return nil, fmt.Errorf("صيغة --resolve يجب أن تكون host=ip: %s", pair)
			}
			if net.ParseIP(strings.TrimSpace(ip)) == nil {
				return nil, fmt.Errorf("عنوان IP غير صالح في --resolve: %s", pair)
			}
			out[strings.ToLower(strings.TrimSpace(host))] = strings.TrimSpace(ip)
		}
	}
	return out, nil
}

// NormalizeBaseURL يضمن أن العنوان الأساسي يحمل مخططاً صالحاً ويُزيل أي مسار زائد.
func NormalizeBaseURL(s string) (string, error) {
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
