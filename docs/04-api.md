# 04. HTTP API v1

## Общий контракт

Базовый путь `/api/v1`. JSON UTF-8, имена полей snake_case. Время RFC3339 UTC, ID — 24 hex символа. Все примеры относятся к проектируемому API. Cookie и CSRF-правила описаны в [05-auth.md](05-auth.md).

Для POST/PUT/PATCH с телом обязателен `Content-Type: application/json`. Для всех POST/PUT/PATCH/DELETE обязателен `Origin`, точно совпадающий с `APP_ORIGIN`, и `X-GeoAssist-CSRF: 1`. Это относится также к login/register/logout. GET не изменяет данные. Максимальное тело — 64 KiB. Неизвестные поля, параметры и повтор одного скалярного query parameter — ошибка; только `tag` допускается несколько раз.

Ответ одного объекта: `{"data": ...}`. Списки: `data` всегда массив, даже пустой; `pagination` описана ниже. Ответ 204 не имеет тела. Для каждого ответа устанавливаем `X-Request-ID`; ошибки дополнительно содержат его в JSON. Персональные данные и auth-ответы: `Cache-Control: no-store`.

### Пагинация

Все списки Assignment #6 используют offset: `limit` (по умолчанию 20, 1..100), `offset` (по умолчанию 0, 0..10000). Читаем limit+1 для `has_more`, возвращаем не больше limit. `total` не считаем. Следующая страница — offset+limit. На изменяющихся данных возможны сдвиги страниц; snapshot pagination не обещаем.

```json
{
  "data": [],
  "pagination": {"limit": 20, "offset": 0, "has_more": false}
}
```

### Ошибки

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Request validation failed",
    "details": [{"field": "location.latitude", "reason": "out_of_range"}],
    "request_id": "d5c3d028-5398-43fc-9100-2ec3c7d936f4"
  }
}
```

`details` — массив (пустой при отсутствии деталей). Сообщение предназначено человеку, клиент принимает решения по code. Локализация ошибок вне MVP.

| HTTP | code | Когда |
|---|---|---|
| 400 | BAD_REQUEST | Не JSON, лишний JSON после объекта, неизвестное поле/параметр, неверный формат ID или query number |
| 401 | UNAUTHENTICATED | Нет/истекла сессия |
| 401 | INVALID_CREDENTIALS | Неверный email или пароль при входе |
| 403 | FORBIDDEN | Недостаточно прав |
| 403 | CSRF_REJECTED | Не прошли Origin/custom-header проверки |
| 404 | NOT_FOUND | Нет ресурса или место удалено |
| 409 | EMAIL_ALREADY_EXISTS | Email занят |
| 409 | VERSION_CONFLICT | Устаревшая версия места |
| 409 | IDEMPOTENCY_CONFLICT | event_key уже использован с другими данными |
| 413 | PAYLOAD_TOO_LARGE | Тело больше 64 KiB |
| 415 | UNSUPPORTED_MEDIA_TYPE | Неподходящий Content-Type |
| 422 | VALIDATION_ERROR | JSON/числа разобраны, но нарушены правила полей, диапазонов, PATCH |
| 429 | RATE_LIMITED | Превышен лимит, заголовок Retry-After в секундах |
| 500 | INTERNAL_ERROR | Неожиданная серверная ошибка |
| 503 | SERVICE_UNAVAILABLE | БД недоступна, операция не завершилась в срок |

## Таблица маршрутов

| Метод | Путь относительно /api/v1 | Доступ | Успех |
|---|---|---|---|
| POST | /auth/register | public | 201 User |
| POST | /auth/login | public | 200 User + cookie |
| POST | /auth/logout | public/idempotent | 204 + очистка cookie |
| GET | /me | user/admin | 200 User |
| PATCH | /me | user/admin | 200 User |
| GET | /categories | public | 200 список категорий |
| GET | /places | public | 200 список Place |
| GET | /places/:id | public | 200 Place |
| POST | /places | admin | 201 Place |
| PATCH | /places/:id | admin | 200 Place |
| DELETE | /places/:id | admin | 204 |
| GET | /me/places/:id/state | user/admin | 200 UserPlaceState |
| PUT | /me/favorites/:id | user/admin | 200 UserPlaceState |
| DELETE | /me/favorites/:id | user/admin | 200 UserPlaceState |
| GET | /me/favorites | user/admin | 200 список Favorite |
| PUT | /me/ratings/:id | user/admin | 200 UserPlaceState |
| DELETE | /me/ratings/:id | user/admin | 200 UserPlaceState |
| POST | /interactions | user/admin | 201 Interaction, 200 при replay |
| GET | /me/interactions | user/admin | 200 список Interaction |

Администратор также может пользоваться своим избранным и оценками. В путях favorites/ratings/state `:id` — ID места. Чтение чужого профиля/истории не предусмотрено.

## 1. Регистрация, вход и профиль

`POST /auth/register`:

```json
{
  "email": "student@example.com",
  "password": "demo-password-2026",
  "display_name": "Abylai",
  "preferred_categories": ["cafe", "park"]
}
```

Обязательны email/password/display_name; preferred_categories необязателен, по умолчанию `[]`. Успех 201, Location: `/api/v1/me`; этот URL требует входа. Автовхода нет.

Общая схема ответа **User** для register/login/GET me/PATCH me:

```json
{
  "data": {
    "id": "507f1f77bcf86cd799439013",
    "email": "student@example.com",
    "display_name": "Abylai",
    "role": "user",
    "preferred_categories": ["cafe", "park"],
    "created_at": "2026-11-07T10:00:00Z",
    "updated_at": "2026-11-07T10:00:00Z"
  }
}
```

`POST /auth/login`:

```json
{"email": "student@example.com", "password": "demo-password-2026"}
```

Оба поля обязательны. Успех 200 User, а не токен в JSON; Set-Cookie создаёт сессию. `GET /me` без тела возвращает User.

`PATCH /me` принимает хотя бы одно из двух полей:

```json
{"display_name": "Abylai K.", "preferred_categories": []}
```

Email, role, password не принимаются. Пропуск поля сохраняет его, null запрещён. Ответ 200 User с новым updated_at.

`POST /auth/logout` без тела: 204, текущая сессия удаляется, cookie очищается. При отсутствующей/истёкшей сессии тоже 204; CSRF-проверка всё равно обязательна. Сбой удаления в БД — 503, не ложный успех.

## 2. Справочник категорий

`GET /categories` без параметров и тела, 200:

```json
{
  "data": [
    {"code": "cafe", "name": "Кафе"},
    {"code": "park", "name": "Парк"},
    {"code": "library", "name": "Библиотека"},
    {"code": "coworking", "name": "Коворкинг"},
    {"code": "attraction", "name": "Достопримечательность"}
  ]
}
```

Это полный справочник, пагинации нет.

## 3. Создание и чтение мест

`POST /places`:

```json
{
  "name": "Example Coffee",
  "description": "Тихая кофейня с Wi-Fi",
  "category": "cafe",
  "tags": ["wifi", "quiet"],
  "address": "Алматы, демонстрационный адрес 1",
  "location": {"longitude": 76.8897, "latitude": 43.2389}
}
```

Все поля обязательны, кроме tags (по умолчанию `[]`). Ответ 201, Location: `/api/v1/places/507f1f77bcf86cd799439011`.

Общая схема **Place**:

```json
{
  "data": {
    "id": "507f1f77bcf86cd799439011",
    "name": "Example Coffee",
    "description": "Тихая кофейня с Wi-Fi",
    "category": "cafe",
    "tags": ["quiet", "wifi"],
    "address": "Алматы, демонстрационный адрес 1",
    "location": {"longitude": 76.8897, "latitude": 43.2389},
    "version": 1,
    "created_at": "2026-11-07T10:00:00Z",
    "updated_at": "2026-11-07T10:00:00Z",
    "distance_m": null
  }
}
```

Тот же объект возвращает `GET /places/:id` (200, без тела запроса). distance_m всегда присутствует: число в геопоиске, null в остальных ответах. created_by, name_search, deleted_at, schema_version не выдаются клиенту.

## 4. Поиск и каталог

`GET /places?q=example&category=cafe&tag=wifi&tag=quiet&longitude=76.89&latitude=43.24&radius_m=2000&limit=20&offset=0`

| Параметр | Семантика |
|---|---|
| q | Префикс всего названия после нормализации, 1..120 символов; пустое значение запрещено |
| category | Одно значение справочника |
| tag | Повторяемый, до 20 уникальных тегов; все теги должны присутствовать |
| longitude, latitude, radius_m | Передаются только вместе; radius_m — целое 1..50000 |
| limit, offset | Общая пагинация |

Все фильтры соединяются через AND. Без геопараметров сортируем `name_search ASC, _id ASC`; с геопараметрами — `distance ASC, _id ASC`. Произвольного sort в MVP нет. Расстояние сферическое, не длина дорожного маршрута. Геолокация для этого запроса не сохраняется в профиле.

Ответ 200: `data` — массив полных Place из раздела 3, `pagination` — общий объект. В геопоиске distance_m — неотрицательное число метров; округление только при отображении в UI. Пустая выдача — 200 с `data: []`. Тело GET отсутствует.

## 5. Изменение и удаление места

`PATCH /places/:id`:

```json
{
  "expected_version": 1,
  "name": "Example Coffee Updated",
  "tags": ["wifi"]
}
```

Обязательны expected_version (целое >=1) и хотя бы одно изменяемое поле из create. Нельзя менять id/created_at/created_by/version напрямую. Null запрещён. location заменяется целиком. Успех 200 Place с version=2; переданная устаревшая версия — 409 VERSION_CONFLICT. Клиент перечитывает карточку и предлагает применить изменения к новой версии. Даже PATCH с совпадающими значениями считается успешным редактированием и увеличивает версию.

`DELETE /places/:id?expected_version=2`, без тела. При успехе 204, место получает deleted_at и следующую версию. Для уже удалённого/несуществующего места — 404, для конфликта активной версии — 409. Повторный DELETE может вернуть 404: эффект остаётся идемпотентным.

## 6. Избранное и оценки

Общая схема **UserPlaceState**:

```json
{
  "data": {
    "place_id": "507f1f77bcf86cd799439011",
    "favorite": true,
    "favorited_at": "2026-11-07T10:05:00Z",
    "rating": 5,
    "updated_at": "2026-11-07T10:06:00Z"
  }
}
```

favorite обязателен; favorited_at/rating/updated_at обязательны и nullable. Не выдаём ID внутреннего документа и user_id. `GET /me/places/:id/state` возвращает 200 даже при отсутствии состояния (false/null/null/null), если место существует. Для логически удалённого места состояние всё ещё доступно; для никогда не существовавшего — 404.

- `PUT /me/favorites/:id` без тела: установить favorite=true, 200 State.
- `DELETE /me/favorites/:id` без тела: установить favorite=false, 200 State.
- `PUT /me/ratings/:id` с обязательным телом `{"rating": 5}`: 200 State.
- `DELETE /me/ratings/:id` без тела: установить rating=null, 200 State.

Повтор той же операции возвращает текущее состояние и не добавляет событие. Установка требует активного места; отмена допустима для удалённого. Ни один запрос не принимает user_id.

`GET /me/favorites?limit=20&offset=0`: 200 список **Favorite**, сортировка favorited_at DESC, ID состояния DESC. Каждый элемент: `{"place": <полный объект Place>, "favorited_at": <RFC3339>, "rating": <1..5 или null>}`. Обёртка списка общая; `<...>` здесь обозначает ссылку на схему, не буквальный JSON. Только активные места, distance_m=null. Дополнительных фильтров у списка избранного пока нет.

## 7. События и история

`POST /interactions`:

```json
{
  "event_key": "83b6e8d2-9d36-4d77-8538-f352f5c776d2",
  "place_id": "507f1f77bcf86cd799439011",
  "type": "view"
}
```

Все поля обязательны; type только view/select. Новый запрос требует активного места. Сначала проверяем replay: корректный повтор старого события возвращается даже после удаления места. Успех 201; replay с тем же payload — 200; другой payload с тем же ключом — 409.

Схема **Interaction**:

```json
{
  "data": {
    "id": "507f1f77bcf86cd799439014",
    "event_key": "83b6e8d2-9d36-4d77-8538-f352f5c776d2",
    "place_id": "507f1f77bcf86cd799439011",
    "type": "view",
    "previous_rating": null,
    "rating": null,
    "occurred_at": "2026-11-07T10:04:00Z"
  }
}
```

При чтении история также содержит favorite_added/favorite_removed/rating_set/rating_removed. Например, изменение оценки с 4 на 5 имеет previous_rating=4 и rating=5. user_id не выдаётся: это история текущего пользователя.

`GET /me/interactions?type=rating_set&place_id=507f1f77bcf86cd799439011&limit=20&offset=0`

Все параметры необязательны; type — одно из шести событий, place_id — корректный ID. Фильтры AND, сортировка occurred_at DESC, _id DESC. Ответ — список Interaction с общей пагинацией. История включает события удалённых мест. Фильтр по отсутствующему месту даёт пустой список, а не 404.

## 8. Технические endpoints

Вне /api/v1, публичные, без тела запроса:

Health-маршруты служат инфраструктурным проверкам и не применяют общий валидатор query/body; клиентам следует вызывать их без параметров и тела.

- `GET /health/live` → 200 `{"status":"ok"}`: процесс отвечает.
- `GET /health/ready` → 200 `{"status":"ready"}` при готовности БД и схемы; иначе 503 `{"status":"not_ready"}` без секретных подробностей.

## 9. Future: рекомендации

На Assignment #6 маршрут не регистрируем. Предлагаемый контракт для файнала:

`GET /api/v1/me/recommendations?limit=10&category=cafe&longitude=76.89&latitude=43.24&radius_m=5000`

Авторизация обязательна. limit 1..50, по умолчанию 10; геопараметры только вместе, правила те же, что у поиска. Полная выдача top-N, offset отсутствует. Допустимы также повторяемые tag по правилам поиска; q не поддерживается.

Ответ 200: `data` — массив объектов `{place, score, reason}`, где place — полный Place, score — число >=0, reason — `similar_to_favorites` или `popular_nearby` (без геофильтра `popular`). `meta` содержит `algorithm` (`item_cf_jaccard` или `popularity`), `model_built_at` (RFC3339/null) и `fallback_used` (bool). В одном ответе не смешиваем CF- и popularity-баллы. Если есть CF-кандидаты, возвращаем их, даже если меньше limit; fallback — только при полном отсутствии кандидатов. Точные правила в документе 06.
