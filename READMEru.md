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

## Installation

If this package lives inside your own module:

```bash
go get -u github.com/jwm1rr0rb10/go-errors
```

---

## Quick Start

```go
package main

import (
    "fmt"
    "yourmodule/errors" // or "github.com/yourusername/errors"
)

func main() {
    err1 := errors.New("permission denied")
    err2 := errors.New("disk full")

    // Combine multiple errors
    err := errors.Append(err1, err2)
    fmt.Println(err)
    // Output:
    // 2 errors occurred:
    //   - permission denied
    //   - disk full

    // Add context to everything
    err = errors.Prefix(err, "backup failed")
    fmt.Println(err)
    // Output:
    // 2 errors occurred:
    //   - backup failed: permission denied
    //   - backup failed: disk full

    // Causal wrapping (the stdlib way)
    dbErr := errors.New("connection refused")
    err = errors.Wrapf(dbErr, "failed to connect to %s:%d", "db.example.com", 5432)
    fmt.Println(err)
    // Output: failed to connect to db.example.com:5432: connection refused

    // Check errors the normal way
    if errors.Is(err, dbErr) {
        fmt.Println("original db error is still in the chain")
    }
}
```

## API Overview
| Function                                             | Description                                                  |
|:-----------------------------------------------------|:-------------------------------------------------------------|
| New(msg string) error                                | Same as errors.New (never returns nil)                       |
| "Errorf(format string, args ...any) error"           | Same as fmt.Errorf                                           |
| "Wrap(err error, msg string) error"                  | Wrap with context (nil → nil)                                |
| "Wrapf(err error, format string, args ...any) error" | Formatted wrap (nil → nil)                                   |
| "Append(err error, errs ...error) error"             | Multi-error builder (nil if all nil)                         |
| Join(errs ...error) error                            | Alias for errors.Join                                        |
| Flatten(err error) error                             | Collapse single-error multi-errors                           |
| "Prefix(err error, prefix string) error"             | Add prefix to every error inside                             |
| "WithMessage(err error, msg string) error"           | Add sibling message (like old Combine)                       |
| Errors(err error) []error,                           | Extract all underlying errors                                |
| "Is, As, Unwrap"                                     | Re-exported stdlib helpers|

---

## Why this library?

- **No dependency hell** — unlike `go-multierror` or `multierr`
- **Modern Go** — fully leverages Go 1.20+ `errors.Join` and `%w` unwrapping
- **Better UX** than both HashiCorp and Uber versions
- **Beautiful output** — multi-errors print cleanly by default
- **Zero surprises** — every function has clear, documented nil-handling

---

## License
[MIT License](https://github.com/jwm1rr0rb10/go-errors/blob/main/LICENSE) – © Raman Zaitsau [@jwm1rrr0rb10](https://github.com/jwm1rr0rb10)

Made with ❤️ for cleaner Go error handling