# Assignment #6

## Designing the data layer and backend for a NoSQL recommendation platform

**Project:** GeoAssist — Geospatial Place Search and Interaction Platform  
**Student:** [Full name]  
**Group:** [Group]  
**Course:** [Course name]  
**Instructor:** [Instructor name]  
**Submission date:** [Date]

> **Подготовка к сдаче:** это текст отчёта с инструкциями для вставки настоящих скриншотов. Заполни титульные поля. Вставь изображения вместо блоков Insert Figure, сохрани английские подписи и удали русские редакторские подсказки. Готовые названия файлов указаны в блоках; пример Markdown-вставки находится в HTML-комментарии под каждым блоком. Сами изображения пока не созданы. Подробный порядок действий — в [туториале](10-tutorial.md).

## 1. Goals and objectives

The goal of Assignment #6 is to design and implement a NoSQL data layer and backend API that can be extended into a recommendation platform. The selected domain must relate to the master's dissertation, and the database must store users, items and their interactions.

For GeoAssist, the objectives are to:

1. Define users, places and interaction types for a geospatial assistant.
2. Select a suitable NoSQL database and justify the data model and indexes.
3. Implement registration, authentication and user profiles.
4. Provide a place catalog with create, read, update and delete operations.
5. Support search by name, category, tags and geographic radius.
6. Store current favorites and ratings, together with an interaction history.
7. Prepare reproducible demonstration data and document the API and startup procedure.
8. Plan a collaborative filtering algorithm for the Final Project.

The implemented scope is the backend and database. Postman is used as the demonstration client. A web interface, collaborative filtering engine and natural-language parser are not implemented in Assignment #6.

## 2. Project proposal and domain

GeoAssist is a platform for finding and selecting places in Astana. The catalog includes cafes, parks, libraries, coworking spaces and attractions. A place has a name, description, category, tags, a display address and geographic coordinates.

| Assignment concept | GeoAssist implementation |
|---|---|
| Users | People searching for places and expressing preferences |
| Items | Places with coordinates and descriptive attributes |
| Interactions | Views, selections, favorites, favorite removals, ratings and rating removals |
| Administrator | A user authorized to manage the catalog |
| Future recommendation output | Places not yet in the user's favorites, selected using patterns in users' current favorites |

The project is implemented as one Go application using Gin and the official MongoDB Go driver. MongoDB runs in Docker. The deployment includes automatic schema preparation and a one-time import of synthetic demonstration data. The following sections describe the database choice, model and planned recommendation algorithm; an extended proposal is also available in [Project Proposal](08-project-proposal.md).

### 2.1 Relevance to the dissertation

The master's dissertation concerns the development of an algorithm for translating natural-language requests into geospatial queries in a messenger. Its central problem is to convert a user message into a structured, valid and executable search request. For example, a message asking for a quiet cafe with Wi-Fi within two kilometres should produce a category, a set of required attributes and a distance constraint. The user's location must be provided separately or obtained through an explicitly defined location input. Evaluating such an algorithm requires a backend that can execute the resulting query and return verifiable results.

GeoAssist provides this execution layer. Its catalog stores coordinates and descriptive attributes, while its API accepts explicit search parameters and returns matching places with distances. A future language-processing module can produce a typed search intent and pass it to the same application services. The database layer does not need to accept arbitrary database code generated from a message. This separation makes the existing backend reusable when the messenger interface is introduced.

The architecture also supports separate evaluation of interpretation and execution. A dissertation experiment can compare the category, tags and radius extracted from a phrase against a reference structured query. The geospatial search can then be checked independently using known points inside and outside the requested radius. Consequently, an incorrect interpretation of a message can be distinguished from a coordinate-handling or query-execution error. This provides a clearer experimental setup than treating the whole assistant as a single component.

User interactions create an additional basis for personalization. Explicit search constraints determine which places are eligible, while a later recommendation component can rank eligible places according to user preferences. A highly recommended place must still satisfy the user's radius and attribute constraints. For the Final Project, collaborative filtering will use patterns of shared favorites between users and places.

The current synthetic Astana dataset supports repeatable functional tests without collecting real users' location histories. It does not establish the quality of natural-language interpretation or recommendations in real usage. The language algorithm, messenger integration and empirical evaluation of those components remain future dissertation work. The contribution of Assignment #6 is a reusable data and execution foundation for that research.

> **Редактору:** этот подраздел рассчитан примерно на 0,5–1 страницу обычного учебного отчёта, но Markdown не фиксирует страницы. После переноса в итоговый шаблон проверь фактический объём по требованию задания.

## 3. NoSQL database selection

MongoDB was selected because the main application query retrieves place documents using geographic and attribute constraints. Its GeoJSON representation and geospatial indexes fit this access pattern. [MongoDB geospatial documentation](https://www.mongodb.com/docs/manual/geospatial-queries/)

| Model | Suitability for this project | Trade-off |
|---|---|---|
| Document model — MongoDB | A place and its attributes form one document; geographic and attribute queries fit the catalog | References and cross-document business rules must be managed explicitly |
| Graph model — Neo4j | Relationships between users and places could be represented as edges | Graph traversal is not the primary operation in the selected design; adopting it would add a different query and modeling approach |
| Key-value model — Redis | A possible future cache for already computed results | A standalone catalog and interaction model would require additional access structures for the selected queries |

Collaborative filtering does not require Neo4j: the planned algorithm can process user–place interaction data in Go. Therefore, MongoDB can remain the database for both Assignment #6 and the Final Project.

## 4. Architecture and data modeling

### 4.1 Modular monolith

The application follows Clean Architecture with three business modules: identity, catalog and activity. Each module separates domain models, application use cases, HTTP adapters and MongoDB adapters. Domain code does not depend on Gin or BSON. Repository interfaces are defined for the application services, and concrete implementations are connected in the bootstrap package.

```text
Postman → HTTP adapter → Application use case → Repository interface
                              ↓                       ↓
                         Domain rules           MongoDB adapter → MongoDB
```

For example, the favorite use case checks the place, loads the current user–place state and writes a state change together with its event. The HTTP layer handles JSON and response codes; the persistence adapter handles BSON and transactions.

### 4.2 Collections

| Collection | Main fields | Purpose |
|---|---|---|
| users | email, password_hash, display_name, role, preferred_categories | Accounts and profile information |
| sessions | user_id, token_hash, expires_at | Server-side authentication sessions |
| places | name, description, category, tags, address, location, version, deleted_at | Searchable catalog |
| user_places | user_id, place_id, favorite, favorited_at, rating | Current preferences for one user–place pair |
| interactions | user_id, place_id, event_key, type, previous_rating, rating, occurred_at | Append-only interaction history through the API |
| app_meta | Dataset marker and import metadata | Technical tracking of the one-time demo import |

User and place references use ObjectID. The history is stored separately from user profiles, avoiding an ever-growing embedded history array. A unique user–place pair identifies the current state, while separate events preserve its changes over time.

Each business collection has a validator for required fields, types and relevant allowed values. Additional validation is performed in Go. MongoDB's flexible document model is therefore used with an explicitly controlled application schema. [MongoDB schema validation](https://www.mongodb.com/docs/manual/core/schema-validation/)

Coordinates are stored as a GeoJSON Point in the order longitude, latitude. The HTTP API uses named coordinate fields, which are converted by the persistence adapter. [MongoDB GeoJSON coordinate conventions](https://www.mongodb.com/docs/manual/geospatial-queries/)

### 4.3 Indexes and query design

| Index | Query or invariant supported |
|---|---|
| users_email: unique email | Login lookup and prevention of duplicate accounts |
| sessions_token; sessions_expiry TTL | Session lookup and cleanup of expired records |
| places_geo: location 2dsphere | Search within a geographic radius |
| places_name; places_category | Active catalog lookup and normalized name-prefix ordering, with optional category |
| user_place_unique: unique user_id/place_id | One current preference state per pair |
| user_favorites | Retrieval of a user's favorites |
| favorite_users | Access to favorite relationships for future collaborative filtering |
| event_idempotency: unique user_id/event_key | Deduplication of repeated event submissions |
| user_history | Retrieval of recent interactions for one user |

Name search is an escaped, normalized prefix search, not arbitrary substring or semantic search. Geographic requests use `$geoNear` with attribute conditions applied before pagination. Index availability does not prove that every combination of filters is optimal; query-plan and scaling measurements are reserved for further evaluation.

### 4.4 Consistency rules

Updating a favorite or rating and appending its event are performed in one transaction. A failure in the event write rolls back the state change. MongoDB transactions provide the atomicity needed for this operation; the local database is configured as a single-node replica set. [MongoDB transactions](https://www.mongodb.com/docs/manual/core/transactions/)

Repeatedly setting the same favorite or rating is a no-op. Views and selections use a client event key: a retry with the same payload returns the original event, while reusing the key for a different payload returns a conflict. Place updates use an expected version to detect concurrent changes. Deletion sets deleted_at instead of erasing the place, preserving historical references.

## 5. Implementation workflow and demonstration

### 5.1 Deployment and test data

The project is started from its root directory using:

```powershell
docker compose up --build -d
docker compose ps
Invoke-RestMethod http://localhost:8080/health/ready
```

The API listens on localhost:8080. MongoDB is exposed on loopback port 27018; the API container connects to mongo:27017 internally. The setup initializes the replica set, applies schema validation and indexes, and imports the demo dataset before accepting API requests.

The initial dataset contains 100 synthetic Astana places, 50 regular users, one administrator, 400 user–place states and 1200 interaction events. The synthetic users have overlapping favorites. The dataset and its import marker are committed together. Normal subsequent startups skip import, preserving changes made during use. Postman demonstrations may increase document counts beyond the initial seed.

> **Insert Figure 1 — `screenshots/01-startup.png`.** PowerShell: покажи `docker compose ps` с healthy для api/mongo и ответ ready. Init-контейнер может иметь Exited (0): это штатное завершение.

<!-- ![Running containers and API readiness](screenshots/01-startup.png) -->

*Figure 1. Docker services and API readiness after initialization.*

> **Insert Figure 2a — `screenshots/02a-place-document.png`.** Compass: geoassist → places, раскрой category, tags, location, version. Используй реально существующий документ.
>
> **Insert Figure 2b — `screenshots/02b-indexes.png`.** Вкладка индексов places: должны быть видны places_geo и тип 2dsphere. По возможности также покажи places_name/places_category.

<!-- ![Place document](screenshots/02a-place-document.png) -->
<!-- ![Place indexes](screenshots/02b-indexes.png) -->

*Figure 2. Place document structure and catalog indexes in MongoDB.*

### 5.2 Registration, authentication and profile

Registration creates a regular user. Login creates a server-side session and returns an HttpOnly cookie. Authenticated profile operations use the identity associated with that cookie; a client cannot choose another user's identity in the request body. Profile responses exclude password hashes and session tokens.

The demonstration uses Postman requests 06–10: registration, login, reading the profile, updating the display name and preferred categories, and logout.

> **Insert Figure 3 — `screenshots/03-registration.png`.** Запрос 06, POST /auth/register, статус 201 и созданный профиль. Пароль на скриншоте показывать не нужно.

<!-- ![Registration result](screenshots/03-registration.png) -->

*Figure 3. Successful user registration.*

> **Insert Figure 4 — `screenshots/04-profile.png`.** После входа 07 выполни запрос 09: PATCH /me, статус 200, обновлённые display_name и preferred_categories.

<!-- ![Profile update](screenshots/04-profile.png) -->

*Figure 4. Updating an authenticated user's profile.*

### 5.3 Place catalog management

Only an administrator can create, update or delete catalog items. The demonstration creates a synthetic cafe using POST /places, reads it using GET /places/{id}, and updates it using PATCH /places/{id}. The request includes a name, description, category, tags, address and coordinates. A successful create returns 201 and version 1. An update supplies expected_version and returns the next version.

> **Insert Figure 5 — `screenshots/05-create-place.png`.** После admin login 11 выполни 12 Create demo place. Покажи существенные поля тела запроса и ответ 201 с id, location и version=1. Запрос 13 подтверждает чтение созданного места; отдельный скриншот для него необязателен.

<!-- ![Creating a catalog item](screenshots/05-create-place.png) -->

*Figure 5. Creating a place in the catalog as an administrator.*

> **Insert Figure 6 — `screenshots/06-update-place.png`.** Запрос 14: expected_version, изменённое описание и версия 2 в ответе. Если у тебя уже другая версия, используй действительные значения и скорректируй пояснение.

<!-- ![Updating a place](screenshots/06-update-place.png) -->

*Figure 6. Updating a place using optimistic version control.*

### 5.4 Search and geographic filtering

The search endpoint combines name prefix, category, required tags and an optional radius. For the demonstration, the query searches for a cafe whose name starts with postman, requires both wifi and quiet tags, and restricts results to 1000 metres around longitude 71.4304 and latitude 51.1282.

```text
GET /api/v1/places?q=postman&category=cafe&tag=wifi&tag=quiet&longitude=71.4304&latitude=51.1282&radius_m=1000
```

The newly created demonstration place meets these conditions. Geographic results include distance_m and are sorted by distance, with ID used to resolve ties. These are spherical distances rather than walking or driving routes. Without geographic parameters, the same endpoint serves as a filtered catalog.

> **Insert Figure 7a — `screenshots/07a-nearby.png`.** Запрос 05: покажи несколько мест с distance_m в пределах 5000 метров.
>
> **Insert Figure 7b — `screenshots/07b-filtered-search.png`.** Запрос 15 после создания/редактирования точки: должны читаться q/category/tags/radius и найденное Postman Demo Coffee. Расстояние этой точки близко к нулю, так как она создана в центре поиска.

<!-- ![Nearby Astana places](screenshots/07a-nearby.png) -->
<!-- ![Combined search filters](screenshots/07b-filtered-search.png) -->

*Figure 7. Geographic search and combined attribute filtering.*

### 5.5 Favorites and ratings

A regular user adds a place to favorites through PUT /me/favorites/{id}. The API returns the current state, including favorite and favorited_at. A repeated identical request returns the same state without a new favorite event or a changed timestamp. Ratings are managed through PUT /me/ratings/{id}, with allowed integer values from 1 to 5.

The demonstration first saves the place and then changes its rating from 5 to 4. The current user_places document contains favorite=true and rating=4; the history preserves both rating changes.

> **Insert Figure 8a — `screenshots/08a-favorite.png`.** После входа student 17 выполни 19: favorite=true и timestamps.
>
> **Insert Figure 8b — `screenshots/08b-favorite-repeat.png`.** Выполни 20: покажи совпадение timestamps с предыдущим ответом. Не выполняй отмену избранного между этими запросами.

<!-- ![Adding a favorite](screenshots/08a-favorite.png) -->
<!-- ![Repeating the favorite request](screenshots/08b-favorite-repeat.png) -->

*Figure 8. Idempotent favorite management.*

> **Insert Figure 9 — `screenshots/09-rating-state.png`.** Выполни 22 и затем 23. На скриншоте 23 покажи rating=4 в запросе и ответе. Это личная оценка, а не агрегированный рейтинг места.

<!-- ![Changing the rating](screenshots/09-rating-state.png) -->

*Figure 9. Updating a user's rating while preserving the current favorite state.*

### 5.6 Interaction history

POST /interactions accepts view and select events. Reading a place with GET does not automatically create a view event. The client supplies an event_key so that a network retry can be distinguished from a new action. The initial event request returns 201; replay of the same event returns 200 and the original event ID.

After a single sequential demonstration of requests 19–26, the history for the created place contains five events: one favorite_added, two rating_set, one view and one select. The duplicate favorite request and duplicate view do not add entries. The history is returned in descending time order by GET /me/interactions.

> **Insert Figure 10a — `screenshots/10a-history.png`.** Запрос 27 после однократного выполнения 19–26: покажи типы пяти событий. Если делал дополнительные действия, повтори демонстрацию с новой точкой либо честно скорректируй количество в тексте.
>
> **Insert Figure 10b — `screenshots/10b-current-state-db.png`.** До шагов 28–29 открой Compass → user_places, фильтр по place_id созданной точки. Покажи favorite=true, rating=4 и ссылки user_id/place_id.

<!-- ![User interaction history](screenshots/10a-history.png) -->
<!-- ![Current user-place state](screenshots/10b-current-state-db.png) -->

*Figure 10. Interaction history and the corresponding current preference state.*

### 5.7 Removing preferences and deleting a place

The user can remove a rating and a favorite without deleting their previous interaction history. An administrator then deletes the place using DELETE /places/{id} with expected_version. The API returns 204. A subsequent public GET returns 404 because the place is no longer active, although its document and historical references remain in the database.

> **Insert Figure 11a — `screenshots/11a-delete.png`.** Выполни 28–31, затем 32: DELETE места, expected_version и статус 204.
>
> **Insert Figure 11b — `screenshots/11b-deleted-not-found.png`.** Запрос 33: GET того же ID и статус 404. Здесь 404 подтверждает ожидаемое поведение, а не неисправность сервера.

<!-- ![Soft-deleting a place](screenshots/11a-delete.png) -->
<!-- ![Deleted place is no longer public](screenshots/11b-deleted-not-found.png) -->

*Figure 11. Logical deletion and exclusion of the place from public access.*

## 6. Verification results

The implementation verification recorded on 7 November 2026 included unit and HTTP tests, integration tests on a real MongoDB replica set, and an end-to-end execution of the Postman collection through Newman. The recorded results are documented in [Verification](09-verification.md).

| Verification | Recorded result |
|---|---|
| Go test suite with TEST_MONGO_URI configured | Passed, including real database integration tests |
| Integration scenarios | 11 passed |
| go vet | No reported issues |
| Postman collection executed through Newman | 34 requests and 105 assertions; 0 failures |
| Restart of the API container | Existing document counts preserved |
| Seed failure simulation | Entire import rolled back; corrected retry succeeded |
| OpenAPI document | Validated against the OpenAPI specification |

The integration tests cover session lifecycle and authorization, geospatial filtering, concurrent version updates, validation failures, transaction rollback, repeated favorites, event replay and seed preservation. During end-to-end verification, an inconsistency between response timestamp precision and BSON Date precision was detected and corrected by using millisecond precision for server-generated timestamps.

These results demonstrate functional behavior in the local environment. They are not a performance benchmark or evidence of recommendation quality. Performance testing and optimization are part of the Final Project.

> **Insert Figure 12 — `screenshots/12-test-summary.png`.** После основной демонстрации запусти Collection Runner один раз или используй команду Newman из README. Покажи фактический итог с количеством запросов/проверок и отсутствием ошибок. Если твоя версия Postman показывает другую группировку счётчиков, не подгоняй изображение под числа: подпиши реальный результат. Для отдельного подтверждения Go-тестов можно добавить Figure 12b с `TEST_MONGO_URI` и результатами `go test ./... -count=1`.

<!-- ![API collection test results](screenshots/12-test-summary.png) -->

*Figure 12. End-to-end API verification results.*

## 7. Planned recommendation algorithm for the Final Project

The planned algorithm is item-based collaborative filtering using current favorites. For each place i, let U(i) be the set of users who currently have that place in favorites. The proposed similarity is Jaccard similarity:

```text
similarity(i, j) = |U(i) intersection U(j)| / |U(i) union U(j)|
score(user, j) = sum of similarity(i, j) over the user's favorite places i
```

An empty union gives similarity zero. Candidate places already in the user's favorites or marked as deleted are excluded. Category, tag and radius conditions remain mandatory filters. If there are no eligible collaborative-filtering candidates, a separately identified popularity fallback is planned.

The similarity is derived from shared user behavior, rather than simply matching categories or selecting the nearest places. This follows the collaborative filtering principle of using relationships in user–item preferences. [Google collaborative filtering overview](https://developers.google.com/machine-learning/recommendation/collaborative/basics)

The Final Project will retain the topic, MongoDB and core data model, and add the recommendation engine, a recommendations API, a web interface, and measured performance evaluation and optimization. No recommendation-quality results are claimed for Assignment #6.

## 8. Conclusion

GeoAssist implements the data layer and backend required for Assignment #6. It stores profiles, place documents, current preferences and interaction history, provides authenticated business operations and geographic search, and supports reproducible local deployment with automatic demo data import. The modular design provides an execution foundation for the dissertation's future natural-language geospatial assistant and a consistent starting point for the Final Project recommendation system.

## 9. References

1. Course handout: *Assignment #6 — Designing the data layer and backend for a NoSQL recommendation platform*. Provided file: `Assignment_6-2.pdf`.
2. Course handout: *Final Project — Recommendation system on a NoSQL database*. Provided file: `Final_Project-2.pdf`.
3. MongoDB. [Geospatial Queries](https://www.mongodb.com/docs/manual/geospatial-queries/).
4. MongoDB. [Schema Validation](https://www.mongodb.com/docs/manual/core/schema-validation/).
5. MongoDB. [Transactions](https://www.mongodb.com/docs/manual/core/transactions/).
6. Google for Developers. [Collaborative filtering](https://developers.google.com/machine-learning/recommendation/collaborative/basics).
7. GeoAssist project. [OpenAPI specification](../api/openapi.yaml), [setup instructions](../README.md) and [recorded verification results](09-verification.md).

External documentation consulted on 7 November 2026.

## Submission checklist — remove this section before submission

- [ ] Replace student, group, course, instructor and date placeholders.
- [ ] Insert real screenshots into Figures 1–12; preserve readable captions.
- [ ] Ensure IDs, versions and timestamps refer to the same demonstration place.
- [ ] Check the Relevance to the dissertation section occupies 0.5–1 page after final formatting.
- [ ] Remove Russian editor notes, unused image comments and this checklist.
- [ ] Apply any additional institutional formatting rules; the supplied handout does not specify font, margins or line spacing.
- [ ] Include the source project, API documentation and setup instructions in the submission package. Include the separate proposal if the submission form expects it as an individual deliverable.
