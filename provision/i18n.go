package provision

// Lang задаёт язык сообщений. Локализуется не только интерфейс, но и то, что
// печатают сами скрипты: их вывод идёт и в терминал, и в панель djaploy, а у
// панели есть русский и английский инстансы.
type Lang string

const (
	LangEN Lang = "en"
	LangRU Lang = "ru"
)

// text хранит пару переводов. Строки держим рядом с местом использования: для
// такого объёма это надёжнее каталога ключей, где перевод молча теряется.
type text struct{ en, ru string }

func (t text) get(l Lang) string {
	if l == LangRU && t.ru != "" {
		return t.ru
	}
	return t.en
}

func tr(l Lang, en, ru string) string { return text{en, ru}.get(l) }

// Lang возвращает язык, подставляя английский по умолчанию.
func (o Options) Lang() Lang {
	if o.Locale == LangRU {
		return LangRU
	}
	return LangEN
}
