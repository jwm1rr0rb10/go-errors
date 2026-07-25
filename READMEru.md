# errors

**Современные утилиты для работы с ошибками на Go без внешних зависимостей** — с первоклассной поддержкой составных ошибок, совместимостью с `errors.Is`/`As`/`Unwrap` и понятным API.

Полноценная замена стандартному `errors` + `go-multierror` / `multierr`, но **без внешних зависимостей**.

---

## Особенности

- `New`, `Errorf`, `Wrap`, **и** недостающий `Wrapf`
- Multi-error через компактный тип `multiError`
- `Append`, `Join`, `Flatten`, `Prefix`, `WithMessage`, `Errors()` и `Leaves()`
- Совместимость с `fmt.Errorf("%w", ...)`, `errors.Is`, `errors.As` и stdlib `errors.Join`
- Красивый вывод `%v` / `%+v`
- Примеры в godoc, table-тесты и fuzz-тесты

## Установка

```bash
go get -u github.com/jwm1rr0rb10/libraries/backend/golang/errors
```

---

## Быстрый старт

```go
package main

import (
    "fmt"
    "github.com/jwm1rr0rb10/libraries/backend/golang/errors"
)

func main() {
    err1 := errors.New("permission denied")
    err2 := errors.New("disk full")

    err := errors.Append(err1, err2)
    fmt.Println(err)

    err = errors.Prefix(err, "backup failed")
    fmt.Println(err)

    dbErr := errors.New("connection refused")
    err = errors.Wrapf(dbErr, "failed to connect to %s:%d", "db.example.com", 5432)
    fmt.Println(err)

    if errors.Is(err, dbErr) {
        fmt.Println("original db error is still in the chain")
    }
}
```

---

## Errors vs Leaves

| Функция | Что возвращает |
|---------|----------------|
| `Errors(err)` | Верхний уровень siblings в multi-error / stdlib join. Цепочка `%w` — **один внешний wrapper**. |
| `Leaves(err)` | Корневая причина каждого sibling — проходит по `%w` до конца. |

```go
root := errors.New("connection refused")
wrapped := errors.Wrap(root, "dial failed")
multi := errors.Append(wrapped, errors.New("disk full"))

errors.Errors(multi) // [dial failed: connection refused, disk full]
errors.Leaves(multi) // [connection refused, disk full]
```

---

## API

| Функция | Описание |
|---------|----------|
| `Join(errs ...error) error` | Как `Append`, но **разворачивает вложенные join** (в отличие от stdlib `errors.Join`) |
| `Errors(err) []error` | Siblings верхнего уровня |
| `Leaves(err) []error` | Корневые причины |
| `Wrapf` | `format` не должен содержать `%w`; `%` экранируется как `%%` |

Полная таблица — в [README.md](README.md).

---

## Join vs stdlib `errors.Join`

| | `Join` этого пакета | stdlib `errors.Join` |
|--|---------------------|----------------------|
| Вложенные join | Разворачиваются в один уровень | Сохраняют вложенность |
| `errors.Is` / `As` | Да | Да |

Ошибки из stdlib `errors.Join` можно передавать в `Append`, `Errors`, `Leaves`.

---

## License

[MIT License](https://github.com/jwm1rr0rb10/libraries/blob/main/backend/golang/LICENSE) – © Raman Zaitsau [@jwm1rr0rb10](https://github.com/jwm1rr0rb10)