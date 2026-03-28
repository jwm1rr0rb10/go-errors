# errors

**Современные утилиты для работы с ошибками на Go без внешних зависимостей** — с первоклассной поддержкой составных ошибок, идеальной совместимостью с `errors.Is`/`As`/`Unwrap` и элегантным API.

Созданы как полноценная замена стандартному пакету `errors` в сочетании с `github.com/hashicorp/go-multierror` (или `go.uber.org/multierr`), но **без каких-либо внешних зависимостей**.

---

## Особенности

- `New`, `Errorf`, `Wrap`, **а также** тот самый недостающий `Wrapf`, которого все так ждали
- Полноценная поддержка множественных ошибок (multi-error) благодаря компактному внутреннему типу `multiError`
- `Append`, `Join`, `Flatten`, `Prefix`, `WithMessage` и `Errors()`
- 100% совместимость с `fmt.Errorf("%w", ...)`, `errors.Is`, `errors.As` и `errors.Join`
- Превосходное форматирование ошибок (вывод через `%v` и `%+v` выглядит отлично)
- Полный набор примеров в godoc и поведение, тщательно проверенное табличными тестами
- Никаких неожиданностей с выделением памяти во время выполнения (runtime)

## Установка

Если этот пакет находится внутри вашего собственного модуля:

```bash
go get -u github.com/jwm1rr0rb10/go-errors
```

---

## Быстрый старт

```go
package main

import (
    "fmt"
    "yourmodule/errors" // or "github.com/yourusername/errors"
)

func main() {
    err1 := errors.New("permission denied")
    err2 := errors.New("disk full")
	
	// Объединение нескольких ошибок
    err := errors.Append(err1, err2)
    fmt.Println(err)
	// Вывод:
	// Произошло 2 ошибки:
	//   - отказано в доступе
	//   - диск заполнен

	// Добавить контекст ко всему
    err = errors.Prefix(err, "backup failed")
    fmt.Println(err)
	// Вывод:
	// Произошло 2 ошибки:
	//   - сбой резервного копирования: отказано в доступе
	//   - сбой резервного копирования: диск заполнен

	// Оборачивание с указанием причины (подход стандартной библиотеки)
    dbErr := errors.New("connection refused")
    err = errors.Wrapf(dbErr, "failed to connect to %s:%d", "db.example.com", 5432)
    fmt.Println(err)
	// Вывод: не удалось подключиться к db.example.com:5432: соединение отклонено

	// Проверка ошибок стандартным способом
    if errors.Is(err, dbErr) {
        fmt.Println("original db error is still in the chain")
    }
}
```

## API Overview
| Function                                             | Description                            |
|:-----------------------------------------------------|:---------------------------------------|
| New(msg string) error                                | Same as errors.New (never returns nil) |
| "Errorf(format string, args ...any) error"           | Same as fmt.Errorf                     |
| "Wrap(err error, msg string) error"                  | Wrap with context (nil → nil)          |
| "Wrapf(err error, format string, args ...any) error" | Formatted wrap (nil → nil)             |
| "Append(err error, errs ...error) error"             | Multi-error builder (nil if all nil)   |
| Join(errs ...error) error                            | Alias for errors.Join                  |
| Flatten(err error) error                             | Collapse single-error multi-errors     |
| "Prefix(err error, prefix string) error"             | Add prefix to every error inside       |
| "WithMessage(err error, msg string) error"           | Add sibling message (like old Combine) |
| Errors(err error) []error,                           | Extract all underlying errors          |
| "Is, As, Unwrap"                                     | Re-exported stdlib helpers             |

---

## Почему именно эта библиотека?

- **Никакого «ада зависимостей»** — в отличие от `go-multierror` или `multierr`
- **Современный Go** — в полной мере использует возможности Go 1.20+, включая `errors.Join` и распаковку ошибок через `%w`
- **Улучшенный UX** — превосходит по удобству использования аналоги от HashiCorp и Uber
- **Эстетичный вывод** — по умолчанию составные ошибки выводятся в консоль в аккуратном, читаемом формате
- **Никаких сюрпризов** — поведение каждой функции при обработке `nil` четко определено и задокументировано

---

## License
[MIT License](https://github.com/jwm1rr0rb10/go-errors/blob/main/LICENSE) – © Raman Zaitsau [@jwm1rrr0rb10](https://github.com/jwm1rr0rb10)

Made with ❤️ for cleaner Go error handling