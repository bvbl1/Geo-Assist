# 03. MongoDB: схемы и согласованность

## Подход

Одна база `geoassist`, пять коллекций. MongoDB не требует одинаковых документов автоматически, поэтому приложение и MongoDB validators должны обеспечивать нашу схему. Для каждой коллекции настроить `$jsonSchema`, `validationLevel: strict`, `validationAction: error`, `additionalProperties: false`; разрешить `_id` и все перечисленные ниже поля. Это спецификация валидаторов, их исполняемые определения создаются в миграциях.

Во всех таблицах поля обязательны, если не сказано обратное. Nullable означает обязательное поле, допускающее BSON null. Все документы имеют `schema_version: int = 1`. Date хранится как BSON Date, не строка; ссылки — ObjectID, не вложенные полные документы. Целые значения версий — BSON long, rating/schema_version — BSON int. Double — координаты. Адаптер явно выбирает BSON-типы.

## users

| Поле | BSON-тип / ограничения |
|---|---|
| `_id` | objectId |
| `schema_version` | int, 1 |
| `email` | string, нормализованный, уникальный |
| `password_hash` | string, bcrypt hash |
| `display_name` | string |
| `role` | string: user/admin |
| `preferred_categories` | array<string>, уникальные категории |
| `created_at`, `updated_at` | date |

## sessions

| Поле | BSON-тип / ограничения |
|---|---|
| `_id`, `user_id` | objectId |
| `schema_version` | int, 1 |
| `token_hash` | string: 64 lowercase hex символа SHA-256 |
| `created_at`, `expires_at` | date |

Исходный session token в MongoDB не хранится. Logout физически удаляет текущий документ сессии. Проверяем `expires_at > now` на каждом запросе независимо от TTL-очистки.

## places

| Поле | BSON-тип / ограничения |
|---|---|
| `_id`, `created_by` | objectId |
| `schema_version` | int, 1 |
| `name`, `description`, `address` | string, ограничения из домена |
| `name_search` | string, NFC + trim + Unicode lowercase от name |
| `category` | string, enum категорий |
| `tags` | array<string>, до 20 уникальных slug |
| `location` | object: только type и coordinates |
| `location.type` | string, только Point |
| `location.coordinates` | array<double> ровно из 2 элементов: longitude, latitude |
| `version` | long, >= 1 |
| `created_at`, `updated_at` | date |
| `deleted_at` | date/null, первоначально null |

Пример документа в Extended JSON (иллюстрирует BSON-типы):

```json
{
  "_id": {"$oid": "507f1f77bcf86cd799439011"},
  "schema_version": {"$numberInt": "1"},
  "name": "Example Coffee",
  "name_search": "example coffee",
  "description": "Тихая кофейня с Wi-Fi",
  "category": "cafe",
  "tags": ["quiet", "wifi"],
  "address": "Алматы, демонстрационный адрес 1",
  "location": {
    "type": "Point",
    "coordinates": [{"$numberDouble": "76.8897"}, {"$numberDouble": "43.2389"}]
  },
  "created_by": {"$oid": "507f1f77bcf86cd799439012"},
  "version": {"$numberLong": "1"},
  "created_at": {"$date": "2026-11-07T10:00:00Z"},
  "updated_at": {"$date": "2026-11-07T10:00:00Z"},
  "deleted_at": null
}
```

Диапазоны longitude/latitude проверяются в домене; проверка формы GeoJSON в БД дополняет её. Не считать validator заменой всех бизнес-инвариантов.

## user_places

| Поле | BSON-тип / ограничения |
|---|---|
| `_id`, `user_id`, `place_id` | objectId |
| `schema_version` | int, 1 |
| `favorite` | bool |
| `favorited_at` | date/null |
| `rating` | int 1..5 / null |
| `created_at`, `updated_at` | date |

Уникальная пара user/place. `favorite=true` требует ненулевого `favorited_at`; false — null. Инвариант проверяет домен; при реализации дополнить validator условием `$or`, а не надеяться на типы полей. Это источник текущих предпочтений для CF. Не восстанавливаем текущее избранное пересчётом всей истории.

## interactions

| Поле | BSON-тип / ограничения |
|---|---|
| `_id`, `user_id`, `place_id` | objectId |
| `schema_version` | int, 1 |
| `event_key` | string, UUID |
| `type` | view/select/favorite_added/favorite_removed/rating_set/rating_removed |
| `previous_rating`, `rating` | int 1..5 / null |
| `occurred_at` | date |

Для `rating_set` новое rating ненулевое, previous_rating может быть null. Для `rating_removed` previous_rating ненулевое, rating=null. Для остальных событий оба поля null. Условия проверять в домене и выразить ветвями validator. Сервер генерирует UUID для переходов состояния; для view/select использует клиентский event_key. Уникальность ключа ограничена пользователем.

## Индексы

У каждой коллекции есть стандартный уникальный индекс `_id`. Дополнительно:

| Коллекция | Ключи | Опции / назначение |
|---|---|---|
| users | `{email: 1}` | unique, регистрация и вход |
| sessions | `{token_hash: 1}` | unique, поиск сессии |
| sessions | `{expires_at: 1}` | TTL `expireAfterSeconds: 0` |
| places | `{location: "2dsphere"}` | геопоиск |
| places | `{deleted_at: 1, name_search: 1, _id: 1}` | каталог и префикс имени |
| places | `{deleted_at: 1, category: 1, name_search: 1, _id: 1}` | категория + сортировка каталога |
| user_places | `{user_id: 1, place_id: 1}` | unique, состояние пары |
| user_places | `{user_id: 1, favorite: 1, favorited_at: -1, _id: -1}` | список избранного |
| user_places | `{favorite: 1, place_id: 1, user_id: 1}` | выборка данных CF на файнале |
| interactions | `{user_id: 1, event_key: 1}` | unique, дедупликация |
| interactions | `{user_id: 1, occurred_at: -1, _id: -1}` | история пользователя |

Это начальный набор, не обещание ускорения любого сочетания фильтров. В частности, обычный индекс имени не гарантирует ускорение фильтра имени внутри геозапроса. Индексы по тегам, истории конкретного места и сложным комбинациям добавляем только после `explain("executionStats")` и измерений. CF-индекс можно создать уже на Assignment #6, его полезность проверить на файнале.

## Атомарность и гонки

Изменение избранного/оценки:

1. Проверить пользователя и допустимость операции с местом.
2. В транзакции прочитать текущее user_places.
3. Если состояние уже нужное, вернуть его без записи события.
4. Записать новое состояние и событие.
5. Commit; после него отправить ответ.

Использовать snapshot read concern и majority write concern для транзакции. Уникальный индекс защищает от двух параллельных первых вставок. Write conflict/transient transaction error повторяется ограниченно в пределах request context; duplicate key при гонке создания пары требует повторного чтения и попытки всей операции, а не автоматического 409 клиенту. При исчерпании попыток — 503. MongoDB-операции внутри одной транзакции выполнять последовательно.

Одно изменение Place атомарно: фильтр `_id + deleted_at:null + version`, `$set` нужных полей и `$inc version`. Если matched=0, отдельно различить отсутствующее/удалённое место (404) и конфликт версии (409).

MongoDB не обеспечивает foreign keys. Существование ссылок проверяет application; seed и migration scripts обязаны соблюдать те же правила. История и user_places могут ссылаться на логически удалённое место — это намеренно.

## Списки и удалённые места

Список избранного включает только активные места: `$match` состояния → `$lookup` places → исключение deleted → сортировка → skip/limit. Фильтровать удалённые места нужно до пагинации. История сохраняет все события и place_id; карточка удалённого места может возвращать 404. Не встраиваем меняющееся название места в каждое событие.

## Миграции и запуск

Транзакции требуют replica set или sharded cluster; standalone MongoDB не подходит для выбранной записи состояния и событий. Для локального Compose используем один узел replica set `rs0`. Приложение в Compose подключается к имени узла, объявленному в replica set. Не считать такую конфигурацию отказоустойчивой.

Миграции запускаются отдельной командой: создание коллекций → валидаторы → именованные индексы. Повторный запуск безопасен при той же схеме. Изменения существующей схемы нумеруются; для уже заполненной базы сначала backfill, затем ужесточение validator. Приложение не удаляет и не перестраивает индексы при каждом старте.

## Источники

- [MongoDB schema validation](https://www.mongodb.com/docs/manual/core/schema-validation/)
- [MongoDB transactions](https://www.mongodb.com/docs/manual/core/transactions/)
- [MongoDB TTL indexes](https://www.mongodb.com/docs/manual/core/index-ttl/)
