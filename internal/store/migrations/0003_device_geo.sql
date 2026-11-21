-- Страна и провайдер по IP: заполняются лениво через Team Cymru DNS.
ALTER TABLE devices ADD COLUMN country TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN asn TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN provider TEXT NOT NULL DEFAULT '';
