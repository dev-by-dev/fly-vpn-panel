-- Заголовки клиента (HWID, платформа, версия, модель, UA) приходят
-- только с запросом подписки; xray их не знает.
ALTER TABLE devices ADD COLUMN hwid TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN platform TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN os_version TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN model TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';
