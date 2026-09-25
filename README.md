# CinemaNa Next — سينمانا نكست

عميل سطح مكتب سريع لخدمة **سينمانا** (Earthlink / Shabakaty) بواجهة عربية
داكنة شبيهة بنتفليكس — مبني بـ **Go + Wails v2 + Vue 3 + Tailwind CSS**.

## الميزات

- بحث فوري وشبكة بوسترات بدقة كاملة وصفوف أحدث المحتوى وقسم الأعلى تقييماً
- مشغل مدمج (mp4 + HLS) عبر وكيل بث محلي + ترجمات SRT/VTT قابلة للتنسيق
- استئناف المشاهدة من حيث توقفت + المفضلة وقوائم خاصة + تصدير/استيراد JSON
- لوحة فلترة (نوع/لغة/تصنيف/تقييم/سنة) وأداة CLI لتشخيص نقاط الاتصال

> تشغيل الفيديو يتطلب الاتصال بشبكة مزود الخدمة (Earthlink) — الواجهة والبحث يعملان من أي مكان.

## التحميل

حمّل أحدث نسخة جاهزة من صفحة
[Releases](https://github.com/omar16966/cinemana-next/releases) —
فك الضغط وشغّل `CinemaNaNext.exe` (بلا تثبيت).

## البناء من المصدر

المتطلبات: [Go](https://go.dev) 1.22+، [Node.js](https://nodejs.org) 18+،
[Wails v2](https://wails.io):

```bash
git clone https://github.com/omar16966/cinemana-next.git
cd cinemana-next/desktop
wails build        # الناتج: build/bin/CinemaNaNext.exe
```

أداة التشخيص (اختيارية):

```bash
go build -o cinemana-probe.exe .
cinemana-probe probe
```

## التشغيل أثناء التطوير

```bash
cd desktop
wails dev
```

## الخطوة القادمة

- إضافة ملف ترخيص (LICENSE) ونسخة مثبّت عبر `wails build -nsis`
- دعم تسجيل الدخول لحساب المشترك (قوائم المتابعة والمفضلة السحابية)
- خيار مشغل مدمج بـ libVLC كبديل عن mpv الخارجي

---

التوثيق التقني المفصل (البنية + نقاط النهاية + ملاحظات الفحص):
[docs/TECHNICAL.md](docs/TECHNICAL.md)

@جميع الحقوق محفوظة للمطور — المطور : Om3r-GLM5.3
