// settings.go (desktop)
// =============================================================================
// إعدادات التطبيق: تُحفظ في مجلد إعدادات ويندوز القياسي
// (%APPDATA%\cinemana-desktop\config.json) وتُقرأ عند الإقلاع.
//
// الإعدادات محدودة عن قصد بما يحتاجه المستخدم فعلاً الآن:
//   - mpv_path: مسار مشغل mpv الخارجي (اختياري — إن ترك فارغاً نبحث في PATH).
//   - base_url: العنوان الأساسي للخدمة (لمن يريد تجربة نقطة نهاية بديلة
//     أو عنوان داخل الشبكة المحلية).
//   - user_agent: تجاوز ترويسة User-Agent (فارغ = الافتراضي الواقعي).
//   - insecure_tls: لتخطي التحقق من الشهادات داخل الشبكات المحلية.
//
// =============================================================================
package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"

	cinemana "cinemana-probe/core/cinemana"
)

// Settings بنية الإعدادات المحفوظة (JSON بسيط تقرؤه الواجهة وتعدله).
type Settings struct {
	MPVPath     string `json:"mpv_path"`
	BaseURL     string `json:"base_url"`
	UserAgent   string `json:"user_agent"`
	InsecureTLS bool   `json:"insecure_tls"`
}

// settingsPath مسار ملف الإعدادات: %APPDATA%\cinemana-desktop\config.json
func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cinemana-desktop", "config.json"), nil
}

// loadSettings قراءة الإعدادات مع قيم افتراضية آمنة عند غياب الملف.
func loadSettings() Settings {
	def := cinemana.DefaultOptions()
	s := Settings{BaseURL: def.BaseURL, UserAgent: def.UserAgent}

	p, err := settingsPath()
	if err != nil {
		return s
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return s // لا ملف بعد = إعدادات افتراضية
	}
	// ندمج فوق الافتراضيات حتى تبقى الحقول الناقصة سليمة. الملف التالف
	// لا يُطبَّق نصفه: نعود للافتراضيات كاملة ونسجل السبب.
	if err := json.Unmarshal(data, &s); err != nil {
		log.Printf("ملف الإعدادات تالف (%v) — استُخدمت الافتراضيات", err)
		s = Settings{BaseURL: def.BaseURL, UserAgent: def.UserAgent}
	}
	if strings.TrimSpace(s.BaseURL) == "" {
		s.BaseURL = def.BaseURL
	}
	if strings.TrimSpace(s.UserAgent) == "" {
		s.UserAgent = def.UserAgent
	}
	return s
}

// saveSettings كتابة الإعدادات مع إنشاء المجلد عند الحاجة.
func saveSettings(s Settings) error {
	p, err := settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(p, data, 0o600)
}

// writeFileAtomic يكتب إلى ملف مؤقت بجوار الهدف ثم يعيد تسميته: انقطاع
// التيار أو الانهيار أثناء الكتابة لا يترك ملفاً مبتوراً.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // لا أثر له بعد نجاح Rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// osLocalAppData مسار %LOCALAPPDATA% مع بديل آمن إن غاب المتغير.
func osLocalAppData() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Local")
}

// exists فحص وجود ملف بسيط.
func exists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
