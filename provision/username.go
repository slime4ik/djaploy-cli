package provision

import "errors"

// Ошибки проверки имени. Разделены, чтобы вызывающий мог показать человеку
// внятную причину, а не общее "имя не подходит".
var (
	ErrUserEmpty   = errors.New("имя пустое")
	ErrUserTooLong = errors.New("имя длиннее 32 символов")
	ErrUserStart   = errors.New("имя должно начинаться со строчной буквы или подчёркивания")
	ErrUserChars   = errors.New("допустимы только строчные буквы, цифры, дефис и подчёркивание")
	ErrUserRoot    = errors.New("root уже есть, и уходить от него как раз и есть смысл этого шага")
)

// ValidUsername проверяет имя по правилам useradd: до 32 символов, начинается
// со строчной буквы или подчёркивания, дальше строчные буквы, цифры, дефис,
// подчёркивание.
//
// Проверяем заранее, чтобы человек увидел причину в момент ввода, а не поймал
// невнятную ошибку useradd в середине прогона. От инъекции защищает sq(), это
// отдельная забота.
func ValidUsername(name string) error {
	switch {
	case name == "":
		return ErrUserEmpty
	case len(name) > 32:
		return ErrUserTooLong
	case name == "root":
		return ErrUserRoot
	}

	c := name[0]
	if !(c >= 'a' && c <= 'z') && c != '_' {
		return ErrUserStart
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= '0' && c <= '9',
			c == '_', c == '-':
		default:
			return ErrUserChars
		}
	}
	return nil
}
