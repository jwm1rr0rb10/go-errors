# go-errors

[English version](README.md)

Утилиты для работы с ошибками в Go с полноценной поддержкой составных ошибок
(multi-error) и полной совместимостью со стандартным пакетом `errors`.
Никаких зависимостей, кроме стандартной библиотеки.

- `New`, `Errorf`, `Wrap`, `Wrapf`, а также `Is` / `As` / `AsType` / `Unwrap` /
  `ErrUnsupported`, так что пакет может заменить `errors` в импортах
- `AsType[E](err)` — генерик-версия `errors.AsType` из Go 1.26, доступная с
  Go 1.21: без рефлексии и аллокаций, примерно в 7 раз быстрее `errors.As`
- `Wrap` примерно в 5 раз дешевле `fmt.Errorf("ctx: %w", err)`: одна аллокация,
  без разбора формата, без стектрейсов; сообщение строится только при выводе
- Составные ошибки: `Append`, `Join`, `Flatten`, `Prefix`, `AppendMessage`, `Errors`, `Count`
- `Leaves` находит все первопричины на любой глубине, в том числе за обёрнутыми составными ошибками
- `Oneline` выводит любое дерево ошибок в одну строку для текстовых логов
- Работает с `fmt.Errorf("%w")`, `errors.Is`, `errors.As` и значениями `errors.Join`
- Безопасна с любыми типами ошибок, включая несравнимые (слайсы, map)
- Тесты с race-детектором, фаззинг в CI, покрытие 90%+

## Установка

```bash
go get github.com/jwm1rr0rb10/go-errors
```

Нужен Go 1.21+. Совместима с Go 1.27; CI прогоняет тесты на Go 1.21 и на
последней стабильной версии.

## Быстрый старт

```go
package main

import (
	"fmt"

	"github.com/jwm1rr0rb10/go-errors"
)

func main() {
	// Собираем ошибки
	err := errors.Join(errors.New("permission denied"), errors.New("disk full"))
	fmt.Println(err)
	// 2 errors occurred:
	//   - permission denied
	//   - disk full

	// Та же ошибка одной строкой, для текстовых логов
	fmt.Println(errors.Oneline(err))
	// permission denied; disk full

	// Добавляем общий контекст к каждой ошибке
	fmt.Println(errors.Oneline(errors.Prefix(err, "backup failed")))
	// backup failed: permission denied; backup failed: disk full

	// Причинное оборачивание
	dbErr := errors.New("connection refused")
	err = errors.Wrapf(dbErr, "connect to %s:%d", "db.example.com", 5432)
	fmt.Println(err)                   // connect to db.example.com:5432: connection refused
	fmt.Println(errors.Is(err, dbErr)) // true
}
```

## Типобезопасный поиск через AsType

```go
if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
	fmt.Println("failed at path:", pathErr.Path)
}
```

`AsType` ведёт себя так же, как `errors.AsType` из Go 1.26, и работает с
Go 1.21, поэтому код, импортирующий этот пакет вместо `errors`, продолжает
компилироваться. Она обходит дерево ошибок как `errors.As` (обёрнутые ошибки,
составные ошибки, значения `errors.Join`, методы `As(any) bool`), но проверяет
тип на этапе компиляции, не использует рефлексию и не аллоцирует. `go fix` на
Go 1.26+ переписывает простые вызовы `errors.As` на `AsType`; с этим пакетом
переписанный код работает без изменений.

## Errors и Leaves

| Функция | Что возвращает |
|---|---|
| `Errors(err)` | Элементы составной ошибки или значения `errors.Join`, вложенные объединения развёрнуты. Обёрнутая ошибка считается одним элементом и не разворачивается. |
| `Leaves(err)` | Первопричины: проходит и по `%w`-оборачиванию, и по составным ошибкам на любой глубине. |

```go
timeout := errors.New("timeout")
refused := errors.New("connection refused")
err := errors.Wrap(errors.Join(timeout, errors.Wrap(refused, "dial")), "sync failed")

errors.Errors(err)  // [sync failed: 2 errors occurred: ...]  (один обёрнутый элемент)
errors.Leaves(err)  // [timeout, connection refused]
errors.Oneline(err) // "sync failed: timeout; dial: connection refused"
```

`Leaves` удобен для отчётов об ошибках, `Oneline` — для сообщений в логах.
Обе функции безопасны на циклических и патологически больших графах ошибок:
обход ограничен и никогда не зацикливается.

## Сбор ошибок

На горячем пути собирайте ошибки в слайс и объединяйте один раз:

```go
var errs []error
for _, item := range items {
	if e := validate(item); e != nil {
		errs = append(errs, e)
	}
}
return errors.Join(errs...) // nil, если ошибок нет; сама ошибка, если она одна
```

`err = errors.Append(err, e)` в цикле тоже работает и амортизированно O(1):
переиспользуется базовый массив, а результаты добавления к одному и тому же
значению никогда не делят состояние, поэтому добавлять к одной базе из
нескольких горутин безопасно. Но каждый вызов аллоцирует новое значение
составной ошибки, так что на 1000 ошибок получается около 1000 аллокаций
против 13 у варианта «слайс + `Join`» (см. [Производительность](#производительность)).

## API

| Функция | Описание |
|---|---|
| `New(msg) error` | `errors.New` (никогда не nil). |
| `Errorf(format, args...) error` | `fmt.Errorf`; используйте `%w` для оборачивания. |
| `Wrap(err, msg) error` | `"msg: err"`, `err` остаётся причиной. nil → nil. |
| `Wrapf(err, format, args...) error` | `Wrap` с форматированным сообщением. nil → nil. В `format` нельзя `%w` (`go vet` предупредит); для `%` пишите `%%`. |
| `Append(err, errs...) error` | Объединяет ошибки; вложенные составные ошибки разворачиваются, nil пропускаются. Все nil → nil; одна не-nil → она сама. |
| `Join(errs...) error` | То же, что `Append` (отличия от `errors.Join` ниже). |
| `Flatten(err) error` | Единственная вложенная ошибка, если она ровно одна; иначе `err`. |
| `Prefix(err, prefix) error` | Оборачивает каждую вложенную ошибку префиксом `prefix`. |
| `AppendMessage(err, msg) error` | Добавляет `msg` **соседней** ошибкой (составная ошибка из `err` и `msg`). Для контекста используйте `Wrap`. |
| `Errors(err) []error` | Вложенные ошибки (копия, можно менять). |
| `Leaves(err) []error` | Первопричины на любой глубине. |
| `Count(err) int` | `len(Errors(err))` без аллокаций. |
| `Oneline(err) string` | Любое дерево ошибок в одну строку: элементы через `"; "`, обёртки как `"ctx: cause"`. |
| `IsAny(err, targets...) bool` | `errors.Is` для любой из целей. |
| `AsAny(err, targets...) bool` | `errors.As` для первой подходящей цели. |
| `AsType[E](err) (E, bool)` | Генерик-версия `As`: первая ошибка в дереве, подходящая под `E`. То же, что `errors.AsType` (Go 1.26), доступно с Go 1.21, без рефлексии и аллокаций. |
| `Is`, `As`, `Unwrap`, `ErrUnsupported` | Реэкспорт из `errors`. |
| `WithMessage(err, msg) error` | **Устарела**, то же, что `AppendMessage`. В отличие от `pkg/errors.WithMessage` не оборачивает, хотя название на это намекает. |

## Отличия от `errors.Join`

| | `Join` / `Append` | `errors.Join` |
|---|---|---|
| Вложенные объединения | Разворачиваются в один уровень | Вложенность сохраняется |
| Одна не-nil ошибка | Возвращается как есть | Оборачивается в join-значение |
| Дубликаты | Сохраняются (как в `errors.Join`) | Сохраняются |
| `errors.Is` / `errors.As` | Да | Да |
| Сообщение | `N errors occurred:` и по строке `  - msg` на каждую ошибку | Сообщения через перевод строки |

Значения, созданные `errors.Join` (и любым типом с `Unwrap() []error`),
принимаются всеми функциями пакета.

## Форматирование

`%v` и `%s` выводят `Error()`. Все строковые глаголы и флаги работают как для
обычной ошибки: `%q`, `%x`, `%X`, `%-20s`, `%.10s` и так далее. `%+v`
передаётся вложенным ошибкам, поэтому подробные форматы других библиотек
(например, стектрейсы) сохраняются.

`Error()` составной ошибки занимает несколько строк. Для текстовых логов,
алертов и меток метрик используйте `Oneline(err)`. Она также разворачивает
составные ошибки, спрятанные за `Wrap` или `fmt.Errorf("ctx: %w", ...)`, и
заменяет переводы строк в остальных сообщениях на `"; "`.

## Производительность

`make bench`. Go 1.22, linux/amd64. В Go 1.27 подешевели мелкие аллокации
(меньше 80 байт), а под это попадают все значения, которые аллоцирует пакет,
поэтому на новых версиях Go цифры ожидаются такими же или лучше.

| Операция | go-errors | Стандартная библиотека |
|---|---|---|
| Одно оборачивание | 36 ns, 1 alloc | `fmt.Errorf("ctx: %w")`: 210 ns, 2 allocs |
| `Wrapf(err, "user %d", 42)` | 137 ns, 2 allocs | — |
| Цепочка из 5 уровней + проверка `errors.Is` | 221 ns, 6 allocs | 1140 ns, 11 allocs |
| Цепочка из 5 уровней + один вывод | 288 ns, 7 allocs | 1090 ns, 11 allocs |
| `Error()` готовой цепочки из 5 уровней | 85 ns, 1 alloc | 2 ns, 0 allocs (сообщение хранится) |
| `Error()` готовой цепочки из 20 уровней | 205 ns, 1 alloc | 2 ns, 0 allocs |
| `Append(x, y)` | 93 ns, 2 allocs | `errors.Join`: 76 ns, 2 allocs |
| 1000 ошибок, `err = Append(err, e)` | 55 µs, 1010 allocs | — |
| 1000 ошибок, слайс + один `Join` | 25 µs, 13 allocs | — |
| `AsType`, совпадение внутри обёрнутой составной ошибки | 26 ns, 0 allocs | `errors.As`: 188 ns, 1 alloc |
| `Leaves`, цепочка из 3 обёрток | 54 ns, 1 alloc | — |
| `Oneline`, обёрнутая составная ошибка | 259 ns, 3 allocs | — |
| `Count`, `Flatten` | 2–3 ns, 0 allocs | — |

`Wrap` строит сообщение лениво. Для серверов это правильный компромисс:
большинство ошибок только проверяют (`errors.Is`, ретраи, `ErrNotFound`) и
никогда не печатают. Цена в том, что каждый вызов `Error()` собирает строку
заново, за одну аллокацию и линейное время. Если одну и ту же ошибку выводите
много раз, сохраните строку один раз.


## Разработка

```bash
make test   # тесты с race-детектором
make cover  # покрытие
make bench  # бенчмарки
make fuzz   # фаззинг (FUZZTIME=5m make fuzz)
make lint   # staticcheck
```

## Лицензия

[MIT](LICENSE) © Raman Zaitsau [@jwm1rr0rb10](https://github.com/jwm1rr0rb10)
