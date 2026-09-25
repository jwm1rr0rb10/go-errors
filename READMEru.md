# go-errors

[English version](README.md)

Утилиты для работы с ошибками в Go с полноценной поддержкой составных ошибок
(multi-error) и полной совместимостью со стандартным пакетом `errors`.
Никаких зависимостей, кроме стандартной библиотеки.

- `New`, `Errorf`, `Wrap`, `Wrapf`, а также реэкспорт `Is` / `As` / `Unwrap` /
  `ErrUnsupported`, так что пакет может заменить `errors` в импортах
- Составные ошибки: `Append`, `Join`, `Flatten`, `Prefix`, `WithMessage`, `Errors`, `Count`
- `Leaves` находит все первопричины на любой глубине, в том числе за обёрнутыми составными ошибками
- Работает с `fmt.Errorf("%w")`, `errors.Is`, `errors.As` и значениями `errors.Join`
- Безопасна с любыми типами ошибок, включая несравнимые (слайсы, map)
- `err = Append(err, e)` в цикле — амортизированно O(1)
- Тесты с race-детектором, фаззинг, покрытие 90%+

## Установка

```bash
go get github.com/jwm1rr0rb10/go-errors
```

Нужен Go 1.21+.

## Быстрый старт

```go
package main

import (
	"fmt"

	"github.com/jwm1rr0rb10/go-errors"
)

func main() {
	// Собираем ошибки
	var err error
	err = errors.Append(err, errors.New("permission denied"))
	err = errors.Append(err, errors.New("disk full"))
	fmt.Println(err)
	// 2 errors occurred:
	//   - permission denied
	//   - disk full

	// Добавляем общий контекст к каждой ошибке
	fmt.Println(errors.Prefix(err, "backup failed"))
	// 2 errors occurred:
	//   - backup failed: permission denied
	//   - backup failed: disk full

	// Причинное оборачивание
	dbErr := errors.New("connection refused")
	err = errors.Wrapf(dbErr, "connect to %s:%d", "db.example.com", 5432)
	fmt.Println(err)                    // connect to db.example.com:5432: connection refused
	fmt.Println(errors.Is(err, dbErr)) // true
}
```

## Errors и Leaves

| Функция | Что возвращает |
|---|---|
| `Errors(err)` | Элементы составной ошибки или значения `errors.Join`; вложенные join разворачиваются. Обёрнутая ошибка — один элемент, внутрь она не раскрывается. |
| `Leaves(err)` | Первопричины: проходит и по `%w`-обёрткам, и по составным ошибкам на любой глубине. |

```go
timeout := errors.New("timeout")
refused := errors.New("connection refused")
err := errors.Wrap(errors.Join(timeout, errors.Wrap(refused, "dial")), "sync failed")

errors.Errors(err) // [sync failed: 2 errors occurred: ...]  (один обёрнутый элемент)
errors.Leaves(err) // [timeout, connection refused]
```

Используйте `Leaves` для логирования и отправки ошибок в системы мониторинга.
Циклические и патологически большие графы ошибок обрабатываются: обход
ограничен и никогда не зацикливается.

## Сбор ошибок в цикле

```go
var err error
for _, item := range items {
	if e := validate(item); e != nil {
		err = errors.Append(err, e)
	}
}
return err // nil, если ошибок не было; сама ошибка, если она одна
```

Добавление в составную ошибку переиспользует её внутренний массив, поэтому
каждый вызов амортизированно O(1). Результаты добавления к одному и тому же
значению никогда не разделяют состояние: добавлять к общей базе из нескольких
горутин безопасно.

## API

| Функция | Описание |
|---|---|
| `New(msg) error` | `errors.New` (никогда не nil). |
| `Errorf(format, args...) error` | `fmt.Errorf`; для оборачивания используйте `%w`. |
| `Wrap(err, msg) error` | `"msg: err"`, `err` остаётся причиной. nil → nil. |
| `Wrapf(err, format, args...) error` | `Wrap` с форматированным сообщением. nil → nil. `format` не должен содержать `%w` (`go vet` это ловит); для `%` пишите `%%`. |
| `Append(err, errs...) error` | Объединить ошибки; вложенные составные ошибки разворачиваются, nil пропускаются. Все nil → nil; одна не-nil → она сама. |
| `Join(errs...) error` | То же, что `Append` (отличия от `errors.Join` — ниже). |
| `Flatten(err) error` | Единственная вложенная ошибка, если она одна; иначе `err`. |
| `Prefix(err, prefix) error` | Обернуть каждую вложенную ошибку префиксом `prefix`. |
| `WithMessage(err, msg) error` | Добавить `msg` **соседней** ошибкой (составная ошибка из `err` и `msg`). В отличие от `pkg/errors.WithMessage`, не оборачивает; для контекста используйте `Wrap`. |
| `Errors(err) []error` | Вложенные ошибки (копия, можно менять). |
| `Leaves(err) []error` | Первопричины на любой глубине. |
| `Count(err) int` | `len(Errors(err))` без аллокаций. |
| `IsAny(err, targets...) bool` | `errors.Is` для любой из целей. |
| `AsAny(err, targets...) bool` | `errors.As` для первой подходящей цели. |
| `Is`, `As`, `Unwrap`, `ErrUnsupported` | Реэкспорт из `errors`. |

## Отличия от `errors.Join`

| | `Join` / `Append` | `errors.Join` |
|---|---|---|
| Вложенные join | Разворачиваются в один уровень | Вложенность сохраняется |
| Одна не-nil ошибка | Возвращается как есть | Оборачивается в join |
| Дубликаты | Сохраняются (как в `errors.Join`) | Сохраняются |
| `errors.Is` / `errors.As` | Да | Да |
| Сообщение | `N errors occurred:` и по строке `  - msg` на ошибку | Сообщения через перевод строки |

Значения из `errors.Join` (и любой тип с `Unwrap() []error`) принимаются всеми
функциями пакета.

## Форматирование

`%v` и `%s` выводят `Error()`, `%q` — в кавычках. `%+v` передаётся вложенным
ошибкам, поэтому подробные форматы других библиотек (например, стектрейсы)
сохраняются.

## Производительность

`make bench`. Go 1.26, Intel Xeon 2.1 GHz. v1.0.2 — предыдущая версия.

| Операция | v1.0.2 | v1.1.0 |
|---|---|---|
| `Wrap(err, "ctx")` | 176 ns, 3 allocs | 29 ns, 1 alloc |
| `Wrapf(err, "user %d", 42)` | 256 ns, 4 allocs | 87 ns, 2 allocs |
| `Append(x, y)` | 239 ns, 6 allocs | 68 ns, 2 allocs |
| `Append` в цикле, 1000 ошибок | 50 ms, 74 MB | 48 µs, 67 KB |
| `Leaves`, цепочка из 3 обёрток | 159 ns, 2 allocs | 39 ns, 1 alloc |
| `Leaves`, составная ошибка | 276 ns, 2 allocs | 121 ns, 2 allocs |
| `Count` | 63 ns, 1 alloc | 1.5 ns, 0 allocs |
| `Flatten` | 356 ns, 8 allocs | 2.4 ns, 0 allocs |

Для сравнения: `fmt.Errorf("ctx: %w", err)` — 128 ns, 2 allocs;
`errors.Join(x, y)` — 60 ns, 2 allocs.

## История изменений

### v1.1.0

Исправления:

- **Паника** `hash of unhashable type`, если в `Append`, `Join`, `Flatten` или
  `Leaves` передавалась ошибка несравнимого типа (слайс, как
  `validator.ValidationErrors`, или структура с map), а также в
  `Leaves(Wrap(Join(a, b), ...))`.
- `Append` в цикле был квадратичным (20 000 добавлений — 32 с); теперь
  амортизированно O(1).
- `Leaves` не заглядывал внутрь обёрнутых составных ошибок.
- `%q` не брал составные ошибки в кавычки.

Изменения поведения:

- Нет дедупликации: `Join(err, err)` сохраняет обе, как `errors.Join`. Раньше
  одинаковые ошибки склеивались, и терялось число сбоев.
- Вложенные join разворачиваются на любую глубину (раньше — на один уровень),
  поэтому `Count`, `Errors` и `Join` всегда согласованы.
- `Wrap`/`Wrapf` возвращают внутренний тип вместо `*fmt.wrapError`. Сообщения
  и `errors.Is/As/Unwrap` не изменились.

Добавлено: `ErrUnsupported`, исполняемые примеры, фаззинг произвольных деревьев
ошибок, бенчмарки. Минимальная версия Go снижена с 1.25 до 1.21.

## Разработка

```bash
make test   # тесты с race-детектором
make cover  # покрытие
make bench  # бенчмарки
make fuzz   # фаззинг (FUZZTIME=5m make fuzz)
make lint   # golangci-lint
```

## Лицензия

[MIT](LICENSE) © Raman Zaitsau [@jwm1rr0rb10](https://github.com/jwm1rr0rb10)
