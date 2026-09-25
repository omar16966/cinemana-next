# التوثيق التقني المفصل — CinemaNa Next

هذا الملف يحتفظ بالتفاصيل الكاملة للبنية ونقاط النهاية وملاحظات الفحص.
للنظرة السريعة والبناء والتشغيل ارجع إلى [README.md](../README.md).

## بنية الكود

| المسار | الدور |
|---|---|
| `core/cinemana/client.go` | عميل HTTP المشترك: تجاوز البروكسي، DNS محلي/مفروض، تثبيت النطاقات، ترويسات الأندرويد |
| `core/cinemana/api.go` | نقاط النهاية + البنى الخام المطابقة لردود الخدمة حرفياً |
| `core/cinemana/browse.go` | الاستعراض: أحدث/الأعلى تقييماً/التصنيفات + لوحة الفلترة |
| `core/cinemana/media.go` | تطبيع المخرجات، تمييز أنواع الروابط، محلل HLS، استخراج الترجمات |
| `main.go` | أداة CLI لتشخيص نقاط الاتصال (المرحلة الأولى) |
| `core/cinemana/media_test.go` | اختبارات وحدة للمحلل والدوال الحساسة (بلا شبكة) |
| `desktop/main.go` | نقطة دخول Wails: النافذة وتضمين أصول الواجهة |
| `desktop/app.go` | طبقة الربط: الدوال المنسوجة للواجهة (Search/GetPlayback/OpenInMPV...) |
| `desktop/proxy.go` | وكيل البث المحلي: ترويسات + Range + إعادة كتابة m3u8 + SRT→VTT |
| `desktop/settings.go` | الإعدادات المحفوظة (مسار mpv، العنوان الأساسي، UA) |
| `desktop/frontend/src/` | واجهة Vue 3: مكونات الشبكة/التفاصيل/المشغل + طبقة `services/api.js` |
| `desktop/frontend/src/services/mock.js` | بيانات تجريبية لمعاينة الواجهة في متصفح عادي بلا تطبيق |
| `desktop/frontend/src/assets/fonts/` | خطوط عربية مدمجة (Cairo، Amiri...) — تُحدَّث بـ `desktop/download_fonts.py` |
| `desktop/icons/` + `desktop/update-icon.ps1` | أيقونة التطبيق: ضع `icon.ico` هنا ثم شغّل السكربت وأعد البناء |

قاعدة الفصل المعماري: `core/cinemana` لا يعرف شيئاً عن الواجهات،
و`desktop/frontend` لا يستدعي الشبكة إلا عبر `services/api.js`.

## نقاط النهاية المستخرجة من الفحص الفعلي

| الوظيفة | المسار (على `cinemana.shabakaty.cc/api/android/`) |
|---|---|
| البحث والفلترة | `AdvancedSearch?level=3&videoTitle=&type=movie\|series&star=&year=من,إلى&category_id=&page=` |
| أحدث الأفلام / المسلسلات | `latestMovies` / `latestSeries` + `level/3/itemsPerPage/48/page/N` |
| الأعلى تقييماً | `videosByCategory?categoryID=56&orderby=DESCENDINGIMDBRATE&videoKind=1\|2` |
| الأنمي (رسوم متحركة) | التصنيف `57` عبر `videosByCategory` |
| فلترة اللغة | `videosByCategoryAndLanguage` + حقل `language_lighttigerNb` في العناصر: `7` أجنبي، `9` عربي |
| تفاصيل العمل | `allVideoInfo/id/<nb>` |
| ملفات الفيديو بجوداتها | `transcoddedFiles/id/<nb>` |
| حلقات المسلسل | `videoSeason/id/<nb المسلسل>` |
| تصنيفات الخدمة | `mainCategories?lang=ar` |

المعرّف `nb` يأتي من نتائج البحث. حقول مهمة: `en_title`/`ar_title`، `year`،
`stars` (تقييم)، `kind` (`1` فيلم، `2` مسلسل)، `imgObjUrl` (بوستر)،
`en_content`/`ar_content` (وصف)، `translations` (ترجمات باسم لغة وامتداد ورابط)،
`resolution` + `videoUrl` (جودة الفيديو)، `season` + `episodeNummer` + `nb` (حلقة).

## ملاحظات تقنية مهمة (من الفحص الفعلي)

1. **إعادة توجيه النطاق:** `cinemana.shabakaty.com` يعيد `302` إلى
   `cinemana.shabakaty.cc`. العميل يتبع التحويل تلقائياً ويظهر الوجهة
   النهائية في `request.final_url`.
2. **روابط موقعة مؤقتة:** روابط الفيديو/الصور/الترجمات تحمل
   `AWSAccessKeyId` و`Expires` و`Signature` (تخزين بأسلوب S3)، وحقل
   `objectUrlExpiration` يوضح تاريخ الانتهاء — اطلبها من جديد قبل كل تشغيل.
3. **DNS:** عند فرض `--dns` يستخدم Go محللاً "نقياً" يرسل استعلامي A وAAAA
   معاً؛ بعض راوترات المزودين تجيب NXDOMAIN لهذا النمط. الافتراضي هو محلل
   نظام التشغيل. في كل الأحوال لا DoH إطلاقاً.
4. **الجودات:** الخدمة تعيد ملفات MP4 مباشرة (240p→1080p) على
   `cdn.shabakaty.cc`، وكود تحليل قوائم HLS جاهز مغطى باختبارات.
5. **المسلسلات:** نقطة `videoSeason` تعيد كل الحلقات دفعة واحدة مع
   `season` و`episodeNummer`، ولكل حلقة `nb` خاص يُستخدم مع
   `transcoddedFiles` و`allVideoInfo`.
6. **عقود الفيديو محصورة بشبكة المزود:** `cdn/cndw*.shabakaty.cc` تعيد
   503 من خارج شبكة Earthlink — التشغيل الفعلي يتطلب الاتصال بشبكتها
   (الـ API والترجمات تعمل من أي مكان). اختبارات الشبكة في
   `desktop/proxy_test.go` تتخطى هذه الحالة تلقائياً؛ شغّلها كاملة:
   `CINEMANA_LIVE=1 go test ./desktop/`.

## مرجع أداة CLI (cinemana-probe)

```bash
cinemana-probe probe                              # فحص شامل: DNS + الوصول + التحويل + النقاط
cinemana-probe search "fast and furious" --type movie
cinemana-probe details 5386                       # تفاصيل فيلم
cinemana-probe videos 5386                        # روابط التشغيل + الجودات + الترجمات
cinemana-probe seasons 653                        # مواسم مسلسل وحلقاته
cinemana-probe all "game of thrones" --type series
cinemana-probe collection top_movies              # قائمة استعراض: top_series، top_anime،
                                                  # latest_movies_arabic، latest_series_anime...
cinemana-probe browse --type movie --cat 57 --star 9 --year-from 2020 --lang 9
```

خيارات مفيدة: `--dns 10.0.0.1` (فرض DNS محلي)، `--resolve host=ip`
(تثبيت نطاق)، `--ua` و`--app-id` (ترويسات المحاكاة)، `--insecure`
(تخطي TLS للشبكة المحلية). كل المخرجات JSON منسق، ورموز الخروج:
`0` نجاح، `1` خطأ، `2` خطأ استخدام.
