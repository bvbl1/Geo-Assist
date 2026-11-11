# Развёртывание

Основной [compose.yaml](../compose.yaml) находится в корне.

- Dockerfile собирает четыре Go-команды; приложение запускается от непривилегированного пользователя.
- init-replica.js создаёт replica set rs0 и ждёт writable primary.
- API при старте применяет схему и одноразовый seed при AUTO_SEED=true.

Команды, порты и демоаккаунты: [README](../README.md).
