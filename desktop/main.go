// main.go (desktop)
// =============================================================================
// نقطة دخول تطبيق سطح المكتب "سينمانا" — مبني بـ Wails v2.
//
// لماذا Wails v2؟
//   - الواجهة الأمامية (Vue 3 + Tailwind) تعمل داخل WebView2 المدمج في
//     ويندوز: خفيفة وسريعة الإقلاع مقارنة بـ Electron (لا نواة كروم مكررة).
//   - منطق الشبكة في Go مباشرة (لا HTTP APIs خارجية ولا سيرفرات جانبية
//     منفصلة): دوال App المصدَّرة أدناه تُربط تلقائياً بالواجهة عبر
//     window.go.main.App داخل الـ WebView.
//
// فصل الطبقات المعماري:
//
//	core/cinemana  → طبقة الخدمة (HTTP، DNS، نقاط النهاية، تحليل الوسائط)
//	desktop/app.go → طبقة ربط تعرض خدمات محددة للواجهة (Bindings)
//	frontend/      → طبقة العرض فقط، لا تعرف شيئاً عن HTTP مباشرة
//
// =============================================================================
package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// assets يدمج ملفات الواجهة المبنية (frontend/dist) داخل الملف التنفيذي
// نفسه — توزيع بملف واحد بلا مجلدات مرافقة.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		// عنوان النافذة وأبعادها: 16:9 تقريباً يناسب شبكة البوسترات.
		Title:            "CinemaNa Next — سينمانا نكست",
		Width:            1360,
		Height:           800,
		MinWidth:         1000,
		MinHeight:        640,
		BackgroundColour: &options.RGBA{R: 7, G: 9, B: 13, A: 255}, // يطابق خلفية الواجهة الداكنة (يمنع وميضاً أبيض عند الإقلاع)
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,  // بناء العميل وتشغيل وكيل البث المحلي
		OnShutdown:       app.shutdown, // إيقاف وكيل البث عند الإغلاق
		Bind: []interface{}{
			// كل الدوال المصدَّرة على App تصبح متاحة في الجافاسكربت عبر
			// window.go.main.App.<MethodName> — هذا هو جسر الربط كله.
			app,
		},
		// وضع داكن لشريط عنوان النافذة ليتناسق مع الواجهة.
		Windows: &windows.Options{
			Theme: windows.Dark,
		},
	})
	if err != nil {
		// في حال فشل إقلاع Wails نفسه (نادر): طباعة ورمز خروج واضحان.
		fmt.Fprintln(os.Stderr, "خطأ إقلاع التطبيق:", err)
		os.Exit(1)
	}
}
