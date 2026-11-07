# 02. Домен, модели Go и правила

## Общие соглашения

- ID: строковый тип в домене; 24 шестнадцатеричных символа в HTTP; ObjectID в MongoDB.
- Время: `time.Time` в Go, BSON Date в MongoDB, RFC3339 UTC в JSON.
- Сервер назначает ID, timestamps, роль, версии и владельца взаимодействия.
- Длина текста считается по Unicode-кодовым точкам, кроме пароля: для bcrypt ограничиваем UTF-8 байты.
- Строки названий и профиля обрезаются по краям; пароли не нормализуются и не обрезаются.
- Неизвестные поля JSON и query parameters отклоняются. Клиент не передаёт raw MongoDB expressions.

## Бизнес-модели Go

Следующие определения иллюстрируют состав моделей; пакеты и методы реализуются позже.

```go
type ID string
type Role string // "user" | "admin"
type Category string

type User struct {
    ID ID
    Email string
    PasswordHash string
    DisplayName string
    Role Role
    PreferredCategories []Category
    CreatedAt, UpdatedAt time.Time
}

type Session struct {
    ID, UserID ID
    TokenHash string
    CreatedAt, ExpiresAt time.Time
}

type Coordinates struct {
    Longitude float64
    Latitude float64
}

type Place struct {
    ID ID
    Name, Description string
    Category Category
    Tags []string
    Address string
    Location Coordinates
    CreatedBy ID
    Version int64
    CreatedAt, UpdatedAt time.Time
    DeletedAt *time.Time
}

type UserPlace struct {
    ID, UserID, PlaceID ID
    Favorite bool
    FavoritedAt *time.Time
    Rating *int // nil либо 1..5
    CreatedAt, UpdatedAt time.Time
}

type Interaction struct {
    ID, UserID, PlaceID ID
    EventKey string // UUID: клиент для view/select, сервер для изменения состояния
    Type string
    PreviousRating, Rating *int
    OccurredAt time.Time
}

type PlaceHit struct {
    Place Place
    DistanceM *float64 // nil без геопоиска
}

type Actor struct {
    UserID ID
    Role Role
}
```

Вычисляемый `name_search` относится к persistence, а не к публичной модели Place. Секреты User и Session не сериализуются напрямую. DTO определены контрактом API.

## Категории и теги

Для MVP фиксированный справочник категорий: `cafe`, `park`, `library`, `coworking`, `attraction`. API возвращает этот справочник. Новую категорию добавляем согласованным изменением домена, MongoDB validator и seed; отдельный CRUD категорий не нужен.

Теги — произвольные нормализованные slug: `[a-z0-9][a-z0-9_-]{0,31}`. До 20 уникальных тегов, сортировка по алфавиту при записи. Например, `wifi`, `quiet`, `outdoor`. Фильтр нескольких тегов означает наличие всех указанных тегов.

## Валидация сущностей

| Поле | Правило |
|---|---|
| email | 3–254 символа, простой адрес без display name; trim + lowercase как политика проекта; уникальный |
| password | 12–72 UTF-8 байта; не изменять исходную строку |
| display_name | 1–80 символов после trim |
| preferred_categories | массив до 5 уникальных значений справочника; по умолчанию `[]` |
| place.name | 1–120 символов |
| place.description | 1–2000 символов |
| place.address | 1–300 символов; текстовое описание, не источник координат |
| place.category | одно значение справочника |
| place.tags | массив по правилам выше, по умолчанию `[]` |
| longitude | конечное число в диапазоне [-180, 180] |
| latitude | конечное число в диапазоне [-90, 90] |
| rating | целое от 1 до 5 |

На Assignment #6 не добавляем цены, график работы, средний рейтинг, загрузку изображений или автоматический геокодинг адреса. Эти поля потребуют дополнительных правил и не нужны для согласованного MVP.

## Сценарии и инварианты

### Identity

- Register: валидировать → нормализовать email → хешировать пароль → создать User с ролью `user`. Регистрация не создаёт сессию; после неё отдельный Login.
- Login: сравнить пароль → создать случайную сессию на 7 суток. Для неверного email и пароля одинаковый ответ 401.
- Get/Update profile: только собственный профиль. Нельзя менять email, роль, password hash через профиль.
- Logout: отозвать текущую сессию и очистить cookie. Другие сессии сохраняются.

### Catalog

- Читать каталог и карточки может любой посетитель.
- Создание, редактирование и удаление доступны только admin; проверка прав также в application, не только middleware.
- Названия не уникальны: разные реальные места могут называться одинаково.
- Версия при создании — 1. Изменение и удаление увеличивают её на 1.
- Удаление логическое: `deleted_at` устанавливается, обычное чтение возвращает 404.
- Восстановление места и физическое удаление вне MVP.

### Текущее состояние пользователя и места

- Одна пара `(user_id, place_id)` соответствует не более чем одному `UserPlace`.
- При отсутствии документа читаем виртуальное состояние: `favorite=false`, `favorited_at=null`, `rating=null`, `updated_at=null` в API.
- Добавить избранное: установить true и текущее время, создать `favorite_added`.
- Убрать избранное: установить false и `favorited_at=null`, создать `favorite_removed`.
- Повторное добавление/удаление без изменения состояния — успешная операция без нового события и изменения timestamp.
- Выставить оценку: 1–5, событие `rating_set` с предыдущим и новым значением. Изменение на то же значение — no-op.
- Удалить оценку: `rating=null`, событие `rating_removed` с предыдущей оценкой.
- Пустой после отмен документ сохраняем: это упрощает конкуренцию и не требует физического удаления.
- Добавление избранного и установка оценки требуют активного места. Отмена избранного/оценки разрешена для логически удалённого места. Несуществующий ID места — 404.

### История

- Только POST события `view` и `select` принимаются от клиента. `GET` карточки не записывает просмотр автоматически.
- События изменения избранного и оценки создаёт сервер; клиент не может записать их через общий POST.
- Время события серверное. Клиент передаёт UUID `event_key` для дедупликации view/select.
- Повтор с тем же `(user_id, event_key)` и теми же `place_id/type` возвращает исходное событие; другой payload — 409.
- Разные ключи означают разные просмотры. Для будущего CF просмотры пока не используются.
- История неизменяема через API. Удаление места не стирает события.

## PATCH и nullable поля

PATCH — частичное обновление, не JSON Merge Patch. Пропущенное поле не изменяется; `null` для изменяемых полей профиля/места запрещён; `[]` очищает массив. `location`, если передана, содержит обе координаты и заменяется целиком. Пустой PATCH — 422. В Go использовать тип Optional с признаками Present/Null, а не один указатель, который теряет различие отсутствия и null.

## Ошибки application

`Validation`, `NotFound`, `Unauthenticated`, `Forbidden`, `Conflict`, `Unavailable`, `Internal`. Application возвращает тип и безопасные подробности; HTTP adapter выбирает код. Duplicate email превращается в Conflict даже при гонке двух регистраций. BSON/driver errors и stack traces наружу не передаются.
