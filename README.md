# GeoAssist

Каталог мест в Астане: поиск на карте, профили, избранное, оценки и история действий. Go + Gin + MongoDB, модульный монолит с Clean Architecture. Веб-интерфейс на React + TypeScript покрывает весь текущий бизнес-API, включая управление местами для администратора. Сайт, API и MongoDB запускаются через Docker Compose. Следующий этап — collaborative filtering для Final Project.

## Запуск

Нужен Docker Desktop в режиме Linux containers. Из корня проекта:

```powershell
docker compose up --build -d
docker compose ps
docker compose logs --tail=50 api
Invoke-RestMethod http://localhost:8080/health/ready
```

Первый запуск скачивает образы, собирает приложение, ждёт MongoDB, инициализирует replica set `rs0`, создаёт схему и индексы, импортирует демоданные. API начинает принимать запросы после успешного импорта. Health должен вернуть `{"status":"ready"}`.

- **Сайт: [http://localhost:8080](http://localhost:8080).**
- API: `http://localhost:8080/api/v1`.
- MongoDB: `localhost:27018` (порт 27017 был занят существующей службой).
- Остановка с сохранением данных: `docker compose down`.
- Данные остаются в volume `geoassist_mongo_data`; не удаляйте его, если хотите сохранить изменения.

## Автоматический импорт

При первом запуске добавляются 100 синтетических мест Астаны, 50 пользователей, один admin, 400 состояний избранного/оценок и 1200 событий. Категории: кафе, парки, библиотеки, коворкинги и достопримечательности. Координаты расположены около `longitude=71.4304`, `latitude=51.1282`.

Места помечены `(демо)`: названия, адреса и характеристики вымышлены, это не сведения о реальных заведениях. Пересекающиеся группы интересов пользователей подготовлены для будущих рекомендаций.

| Роль | Email | Пароль локального демо |
|---|---|---|
| Admin | `admin@geoassist.local` | `Admin-demo-2026!` |
| User | `student01@geoassist.local` | `Student-demo-2026!` |
| Остальные пользователи | `student02@geoassist.local` … `student50@geoassist.local` | `Student-demo-2026!` |

Все данные и маркер `astana-demo-v1` в `app_meta` импортируются одной транзакцией. Ошибка откатывает весь импорт. Повторный запуск при наличии маркера пропускает seed: нет дубликатов, изменений паролей или перезаписи пользовательских правок. Для непустой базы без маркера отключите `AUTO_SEED` либо выберите отдельную пустую базу. Автоматического reset/drop нет.

Compose предназначен для локальной разработки: порты API и MongoDB доступны только на loopback. Демоаккаунты не предназначены для опубликованного приложения.

## Веб-интерфейс

Гость может искать места по началу названия, категории, тегам и расстоянию от точки, просматривать карточки и карту. После входа доступны профиль, избранное, оценки 1–5, выбор места и история действий. У администратора появляется раздел «Управление»: создание, редактирование и удаление мест с проверкой версии. Регистрация создаёт обычного пользователя; после неё нужно войти.

Открывайте сайт именно через `localhost:8080`: этот адрес должен совпадать с `APP_ORIGIN`. Cookie и CSRF отправляются браузером автоматически, вводить токены не нужно. Геолокация запрашивается только кнопкой «Моё местоположение»; можно ввести координаты или выбрать точку на карте. Карта показывает ту же страницу результатов, что и список. Внешняя подложка OpenStreetMap требует интернет, а список и формы работают при её недоступности.

[Запуск, устройство frontend и проверки](docs/18-frontend-guide.md).

## Разработка frontend

Для изменения интерфейса без пересборки Docker нужен Node.js 22.21.0 (версия закреплена в `web/.node-version`). В корне проекта:

```powershell
$env:APP_ORIGIN = 'http://localhost:5173'
docker compose up -d --force-recreate api
cd web
npm ci
npm run dev
```

Откройте `http://localhost:5173`. Vite проксирует `/api` в Go на 8080 и сохраняет Origin браузера. Одновременно используйте только один origin для изменяющих запросов. Для возврата к готовому сайту остановите Vite, перейдите в корень и выполните:

```powershell
$env:APP_ORIGIN = 'http://localhost:8080'
docker compose up --build -d
```

В контейнере Node не запускается: он нужен только на этапе сборки. Go раздаёт готовый `web/dist` из `/app/web`; API и сайт имеют один origin.

## Postman

Импортируйте коллекцию и local environment из [api/postman](api/postman/README.md). Выберите environment и запускайте запросы по порядку или через Collection Runner. Cookie jar должен быть включён. Коллекция добавляет CSRF-заголовки и сохраняет ID/версию созданного места.

Для изменяющих запросов:

```http
Origin: http://localhost:8080
X-GeoAssist-CSRF: 1
Content-Type: application/json
```

Content-Type нужен при JSON-теле. Авторизация — cookie `geoassist_session`, не Bearer token. Регистрация не выполняет автоматический вход. Только admin меняет каталог. Для просмотра есть отдельный POST `/interactions`: GET карточки историю не изменяет.

Публичный геопоиск:

```text
http://localhost:8080/api/v1/places?category=cafe&tag=wifi&longitude=71.4304&latitude=51.1282&radius_m=2000&limit=20
```

`q` ищет префикс всего названия. Изменение и удаление места требуют `expected_version`; удаление логическое. Полный контракт: [API](docs/04-api.md) и [OpenAPI](api/openapi.yaml).

## Go локально, база в Docker

```powershell
docker compose up -d mongo mongo-init
docker compose logs mongo-init
$env:MONGO_URI = 'mongodb://localhost:27018/?replicaSet=rs0&directConnection=true'
go run ./cmd/api
```

Дождитесь сообщения готовности replica set. Если API уже работает в Docker, сначала `docker compose stop api`, чтобы освободить порт 8080. Go должен соответствовать `go.mod` (1.24.5 или новее). Локальный URI выше также подходит для MongoDB Compass.

Дополнительные команды:

```powershell
go run ./cmd/migrate
go run ./cmd/seed
go run ./cmd/create-admin -email 'teacher@example.com' -name 'Teacher'
```

create-admin запрашивает пароль скрытым вводом и не повышает роль существующего пользователя. seed выполняет тот же одноразовый импорт. Go читает переменные окружения процесса, а не файл `.env`.

## Конфигурация

| Переменная | По умолчанию |
|---|---|
| APP_ENV | development |
| HTTP_ADDR | :8080 |
| APP_ORIGIN | http://localhost:8080 |
| MONGO_URI | localhost:27018, replicaSet=rs0, directConnection=true |
| MONGO_DATABASE | geoassist |
| AUTO_SEED | true |
| COOKIE_SECURE | false |
| WEB_DIR | пусто для локального Go; `/app/web` в Docker |

Compose автоматически читает `.env`; шаблон — [.env.example](.env.example). В контейнере API используется внутренний адрес `mongo:27017`. Production требует HTTPS origin, `COOKIE_SECURE=true`, `AUTO_SEED=false`; текущий Compose является локальным стендом.

## Проверки

```powershell
go test ./cmd/... ./internal/... ./tests/...
go vet ./cmd/... ./internal/... ./tests/...
$env:TEST_MONGO_URI = 'mongodb://localhost:27018/?replicaSet=rs0&directConnection=true'
go test ./cmd/... ./internal/... ./tests/... -count=1
```

Без TEST_MONGO_URI интеграционные тесты явно пропускаются. С ним они создают отдельные базы `geoassist_test_*` и очищают только их. Проверяются авторизация, роли, geo CRUD, версии, транзакции, гонки, дедупликация и seed. Рабочая база не используется.

Postman-коллекцию можно выполнить из терминала при установленном Node.js:

```powershell
npx --yes newman@6.2.1 run api/postman/GeoAssist.postman_collection.json -e api/postman/GeoAssist.local.postman_environment.json
```

Прогон создаёт тестовый аккаунт и место; место в конце логически удаляется, аккаунт и история остаются. Между прогонами учитывайте ограничения частоты запросов, описанные в инструкции Postman.

## Структура и документация

`cmd/` — команды; `internal/identity`, `catalog`, `activity` — бизнес-модули; `internal/bootstrap` — их сборка; `platform` — инфраструктура; `shared` — общие типы; `api` — OpenAPI/Postman; `deploy` — Dockerfile; `tests/integration` — проверки на MongoDB.

`web/src/app` — роуты, сессия, общий интерфейс; `pages` — экраны; `modules` — identity/catalog/activity; `shared` — API-клиент, типы из OpenAPI, формы и карта; `web/e2e` — браузерные тесты с настоящим API.

В каждом бизнес-модуле разделены domain, application, adapters/http и adapters/mongo. HTTP DTO не используются как BSON-документы.

- [План](docs/PLAN.md)
- [Архитектура](docs/01-architecture.md)
- [Домен](docs/02-domain.md)
- [MongoDB](docs/03-database.md)
- [API](docs/04-api.md)
- [Авторизация](docs/05-auth.md)
- [Поиск и рекомендации](docs/06-search-recommendations.md)
- [Этапы реализации](docs/07-delivery.md)
- [Project proposal](docs/08-project-proposal.md)
- [Результаты проверки](docs/09-verification.md)
- [Туториал: устройство проекта и демонстрация API](docs/10-tutorial.md)
- [Отчёт Assignment #6 с местами для скриншотов](docs/11-assignment-6-report.md)
- [Frontend: решения и общий план](docs/12-frontend-plan.md)
- [Frontend: архитектура, папки и сборка с Go](docs/13-frontend-architecture.md)
- [Frontend: экраны и пользовательские сценарии](docs/14-frontend-screens.md)
- [Frontend: API, сессии, ошибки и состояние](docs/15-frontend-api-state.md)
- [Frontend: карта и геопоиск](docs/16-frontend-map-search.md)
- [Frontend: этапы реализации и приёмка](docs/17-frontend-delivery.md)
- [Frontend: руководство по готовому сайту](docs/18-frontend-guide.md)
- [Frontend: результаты проверки](docs/19-frontend-verification.md)
